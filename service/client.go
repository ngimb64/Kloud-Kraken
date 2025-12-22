package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"github.com/ngimb64/Kloud-Kraken/pkg/ec2utils"
	"github.com/ngimb64/Kloud-Kraken/pkg/hashcat"
	"github.com/ngimb64/Kloud-Kraken/pkg/kloudlogs"
	"github.com/ngimb64/Kloud-Kraken/pkg/netio"
	"github.com/ngimb64/Kloud-Kraken/pkg/ssmutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/tlsutils"
	"go.uber.org/zap"
)

// Package level variables
var BufferMutex = &sync.Mutex{}    // Mutex for message buffer synchronization
var DataPath string                // Path where data dirs will be stored
var Ec2SecurityGroupId string      // ID for Security Group for EC2 clients
var HashcatArgs = &hashcat.HashcatArgs{}  // Initialze where hashcat args are stored
var HashFilePath string  // Stores hash file path when received
var HashesPath string    // Path where hash files are stored
var HasRuleset bool      // Toggle for specifying whether ruleset is in use
var LogPath = "/tmp/KloudKraken.log"  // Stores log file to be returned to client
var MaxTransfers atomic.Int32         // Number of file transfers allowed simultaniously
var MaxTransfersInt32 int32           // Stores converted int maxTransfers arg
var RulesetFilePath string            // Stores ruleset file when received
var RulesetPath string                // Path where ruleset files are stored
var TlsMan = &tlsutils.TlsManager{}  // Struct for managing TLS certs, keys, etc.
var WordlistPath string              // Path where wordlists are stored


// Ensure the final cracked hashes file exists and has a message informing
// the user no hashes were cracked.
//
// @Parmeters
//  - lootFile:  The file path where final cracked hashes are stored
//
// @ Returns
//  - Error if it occurs, otherwise nil on success
//
func createFailureResult(lootPath string) error {
    // Open the final cracked hashes file or create if it does not exist
    hashesHandle, err := os.OpenFile(lootPath, os.O_RDWR|os.O_CREATE, 0644)
    if err != nil {
        return err
    }

    // Close the opened cracked hashes file on local exit
    defer func() {
        cerr := hashesHandle.Close()
        if cerr != nil {
            err = errors.Join(err, fmt.Errorf("closing final hashes file - %w", cerr))
        }
    }()

    // Write a message letting user know that no hashes were cracked
    _, err = hashesHandle.Write([]byte("No available cracked hashses after processing"))
    if err != nil {
        return err
    }

    return nil
}


// Lock mutux for messaging connection and related buffer, sends the processing
// complete message.
//
// @Parameters
//  - connection:  network socket connection where procesing complete message is sent
//  - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func sendProcessingComplete(connection net.Conn,
                            logMan *kloudlogs.LoggerManager) {
    // Lock the mutex and ensure it unlocks on local exit
    BufferMutex.Lock()
    defer BufferMutex.Unlock()

    // Send the processing complete message
    _, err := netio.WriteHandler(connection, globals.PROCESSING_COMPLETE,
                                 len(globals.PROCESSING_COMPLETE))
    if err != nil {
        logMan.LogMessage("error", "Error sending processing complete message:  %v", err)
        return
    }
}


// Periodically attempts to select a received file from the wordlist path until
// signal in channel takes the received filename and passes it into command
// execution method for processing, the result is parsed and logged to kloudlogs.
//
// @Parameters
//  - connection:  Socket connection for reading data to be stored and processed
//  - hashcatOptChannel:  Channel to signal when hash and ruleset files are received
//  - transferChannel:  Channel to transmit filenames after transfer to initiate
//                      data processing
//  - waitGroup:  Acts as a barrier for the Goroutines running
//  - transferManager:  Manages calculating the amount of data being transferred
//  - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func processingHandler(connection net.Conn, hashcatOptChannel chan struct{},
                       transferChannel chan struct{}, waitGroup *sync.WaitGroup,
                       transferManager *data.TransferManager,
                       logMan *kloudlogs.LoggerManager) {
    completed := false
    var err error
    // Set the message buffer size
    buffer := make([]byte, globals.MESSAGE_BUFFER_SIZE)
    // Decrements the wait group counter upon local exit
    defer waitGroup.Done()

    defer func() {
        // Lock the mutex and ensure it unlocks on defered function exit
        BufferMutex.Lock()
        defer BufferMutex.Unlock()

        // Transfer the log file to server
        err = netio.UploadFile(connection, buffer, LogPath, globals.LOG_TRANSFER_PREFIX)
        if err != nil {
            logMan.LogMessage("error", "Error occured sending the log file to server:  %v", err)
        }
    } ()

    charsets := []string{HashcatArgs.CharSet1, HashcatArgs.CharSet2,
                         HashcatArgs.CharSet3, HashcatArgs.CharSet4}
    cmdOptions := []string{}

    // Format the path for temp & permanent cracked hashes files
    crackedPath := path.Join(HashesPath, "cracked.txt")
    lootPath := filepath.Join(HashesPath, "loot.txt")

    // If GPU optimization is to be applied, append it to options slice
    if HashcatArgs.ApplyOptimization {
        cmdOptions = append(cmdOptions, "-O")
    }

    // Wait for signal that hash and ruleset files are received
    <-hashcatOptChannel

    // Append command args used by all attack modes
    cmdOptions = append(cmdOptions, "--remove", "-o", crackedPath, "-a",
                        HashcatArgs.CrackingMode, "-m", HashcatArgs.HashType,
                        "-w", HashcatArgs.Workload, HashFilePath)

    // If a ruleset is in use and it has a path
    if HasRuleset && RulesetFilePath != "" {
        // Append it to the command args
        cmdOptions = append(cmdOptions, "-r", RulesetFilePath, "--loopback")
    }

    for {
        // Attempt to get the next available wordlist
        fileName, fileSize, err := disk.CheckDirFiles(WordlistPath)
        if err != nil {
            logMan.LogMessage("error", "Error retrieving wordlist from wordlist dir:  %v",
                              err, zap.String("wordlist directory", WordlistPath))
            return
        }

        select {
        // Poll channel for complete signal
        case <-transferChannel:
            // Set outer boolean toggle
            completed = true

            // Try again to get the next available wordlist to ensure no data is missed
            fileName, fileSize, err = disk.CheckDirFiles(WordlistPath)
            if err != nil {
                logMan.LogMessage("error", "Error retrieving wordlist from wordlist dir:  %v",
                                  err, zap.String("wordlist directory", WordlistPath))
                return
            }
        default:
            // If there was no wordlist available in designated directory
            if fileName == "" {
                // Sleep a bit and re-iterate to see if wordlist is available
                time.Sleep(3 * time.Second)
                continue
            }
        }

        // If the receiving handler routine is complete and
        // there are no more files to be processed
        if completed && fileName == "" {
            // Send the processing complete message to server
            sendProcessingComplete(connection, logMan)
            break
        }

        // Format the path to the wordlist
        filePath := filepath.Join(WordlistPath, fileName)

        var cmdArgs []string

        switch HashcatArgs.CrackingMode {
        case "3":
            // Appened incremental mode and available charsets for hash mask
            cmdArgs = append(cmdOptions, "--incremental")
            hashcat.AppendCharsets(&cmdArgs, charsets)
            // Append the hash mask
            cmdArgs = append(cmdArgs, HashcatArgs.HashMask)
        case "6":
            // Appened incremental mode and available charsets for hash mask
            cmdArgs = append(cmdOptions, "--incremental")
            hashcat.AppendCharsets(&cmdArgs, charsets)
            // Append the wordlist path then the hash mask
            cmdArgs = append(cmdArgs, filePath, HashcatArgs.HashMask)
        case "7":
            // Appened incremental mode and available charsets for hash mask
            cmdArgs = append(cmdOptions, "--incremental")
            hashcat.AppendCharsets(&cmdArgs, charsets)
            // Append the hash mask then the wordlist path
            cmdArgs = append(cmdArgs, HashcatArgs.HashMask, filePath)
        default:
            // For straight mode (0), just append the wordlist path
            cmdArgs = append(cmdOptions, filePath)
        }

        // Execute the hashcat command with populated arg list
        output, err := exec.Command("hashcat", cmdArgs...).CombinedOutput()
        // If the error was an exit type error
        if exitErr, ok := err.(*exec.ExitError); ok {
            code := exitErr.ExitCode()

            // If the code is not exhausted
            if code != 1 {
                logMan.LogMessage("error", "Error executing command:  %v", output)
                return
            }
        }

        // Check to see if cracked hashes file exits after hashcat after processing
        exists, isDir, hasData, err := disk.PathExists(crackedPath)
        if err != nil {
            logMan.LogMessage("error", "Error checking cracked hashes file existence:  %v", err)
            return
        }

        // If cracked hashes file exists and has data
        if exists && !isDir && hasData {
            // If there is data in cracked user hash file prior to processing,
            // append it to the final loot file
            err = disk.AppendFile(crackedPath, lootPath)
            if err != nil {
                logMan.LogMessage("error", "Error appending data to file:  %v", err,
                                  zap.String("source file", "cracked.txt"),
                                  zap.String("destination file", lootPath))
                return
            }
        }

        // Parse the hashcat output
        logArgs, err := hashcat.ParseHashcatOutput(output, []byte("=>"))
        if err != nil {
            logMan.LogMessage("error", "Error parsing hashcat output:  %v", err)
        }

        // Log the hashcat output with kloudlogs
        logMan.LogMessage("info", "Hashcat processing results", logArgs...)

        // Delete the processed file
        os.Remove(filePath)
        // Remove the file size from transfer manager after deletion
        transferManager.RemoveTransferSize(fileSize)
    }

    // Check to see if final cracked hashes file exits before sending back to server
    exists, _, hasData, err := disk.PathExists(lootPath)
    if err != nil {
        logMan.LogMessage("error", "Error checking final cracked hashes file existence:  %v", err)
        return
    }

    // If final cracked hashes does not exist or is empty
    if !exists || !hasData {
        // Ensure final cracked hashes files exists with a message
        // that says cracking attempts were unsuccessful
        err = createFailureResult(lootPath)
        if err != nil {
            logMan.LogMessage("error", "Error creating unsuccessful attempt " +
                              "message for clint:  %v", err)
            return
        }
    }

    // Lock the mutex and ensure it unlocks on local exit
    BufferMutex.Lock()
    defer BufferMutex.Unlock()

    // Transfer the final cracked user hash file to server
    err = netio.UploadFile(connection, buffer, lootPath, globals.LOOT_TRANSFER_PREFIX)
    if err != nil {
        logMan.LogMessage("error", "Error occured sending the cracked hashes to server:  %v", err)
        return
    }
}


// Sends transfer message to server, waits for transfer reply with file name and
// size or the end transfer message. Gets an available port and sends it to the
// server, and waits for an incoming connection from the server and uses that
// new connection to initiate file transfer routine.
//
// @Parameters
//  - connection:  Socket connection for reading data to be stored and processed
//  - buffer:  The buffer used for processing socket messaging
//  - waitGroup:  Used to synchronize the Goroutines running
//  - transferManager:  Manages calculating the amount of data being transferred
//  - transferComplete:  Toggle to signify when all files have been transfered
//  - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func processTransfer(connection net.Conn, buffer []byte,
                     waitGroup *sync.WaitGroup,
                     transferManager *data.TransferManager,
                     transferComplete *bool,
                     logMan *kloudlogs.LoggerManager) {
    // Lock the mutex and ensure it unlocks on local exit
    BufferMutex.Lock()
    defer BufferMutex.Unlock()

    // Get random available port as a listener
    listener, port := netio.GetAvailableListener()

    closeListener := func() {
        cerr := listener.Close()
        if cerr != nil {
            logMan.LogMessage("error", "Error closing raw TCP socket - %v", cerr)
        }
    }

    // Format the transfer request with listener port to connect to
    sendLength, err := netio.FormatTransferRequest(port, &buffer, globals.TRANSFER_REQUEST_PREFIX)
    if err != nil {
        logMan.LogMessage("error", "Error formatting transfer request:  %v", err)
        closeListener()
        return
    }

    // Send the transfer request message to initiate file transfer
    _, err = netio.WriteHandler(connection, buffer, sendLength)
    if err != nil {
        logMan.LogMessage("error", "Error sending the transfer request to server:  %v", err)
        closeListener()
        return
    }

    // Wait to receive the start transfer message from the server
    bytesRead, err := netio.ReadHandler(connection, &buffer)
    if err != nil {
        logMan.LogMessage("error", "Error start transfer message from server:  %v", err)
        closeListener()
        return
    }

    // Slice off any unused bytes in buffer
    readBuffer := buffer[:bytesRead]

    // If the server has completed transferring all data
    if bytes.Contains(readBuffer, globals.END_TRANSFER_MARKER) {
        *transferComplete = true
        closeListener()
        return
    }

    // If the read data does not start with special delimiter or end with closed bracket
    if !bytes.HasPrefix(readBuffer, globals.START_TRANSFER_PREFIX) ||
    !bytes.HasSuffix(readBuffer, globals.TRANSFER_SUFFIX) {
        logMan.LogMessage("error", "Unusual format in receieved start transfer message",
                          zap.String("transfer message", string(readBuffer)))
        closeListener()
        return
    }

    // Extract the file name and size from the start transfer message
    fileName, fileSize, err := netio.ParseStartTransfer(buffer,
                                                        globals.START_TRANSFER_PREFIX,
                                                        bytesRead)
    if err != nil {
        logMan.LogMessage("error", "Error extracting file name and " +
                          "size from start transfer message:  %v", err)
        closeListener()
        return
    }

    // Set up context handler for TLS listener
    ctx, cancel := context.WithCancel(context.Background())
    // Setup up TLS listener from existing raw TCP listener
    tlsListener, err := TlsMan.SetupTlsListenerHandler(TlsMan.TlsCertificate, ctx,
                                                       "0.0.0.0", port, listener)
    if err != nil {
        logMan.LogMessage("error", "Error setting TLS listener on client:  %v", err)
        cancel()
        closeListener()
        return
    }

    // Wait for an incoming connection
    transferConn, err := tlsListener.Accept()
    if err != nil {
        logMan.LogMessage("error", "Error accepting server connection:  %v", err)

        // Ensure TLS listener is closed
        err = tlsListener.Close()
        if err != nil {
            logMan.LogMessage("Error", "Error closing TLS listener:  %v", err)
        }

        // Call cancel function to ensure raw TCP socket is closed
        cancel()
        return
    }

    waitGroup.Add(1)
    MaxTransfers.Add(1)
    // Add the file size of the file to be transfered to transfer manager
    transferManager.AddTransferSize(fileSize)

    go func() {
        defer func() {
            // Close the transfer connection
            err = transferConn.Close()
            if err != nil {
                logMan.LogMessage("Error", "Error closing transfer connection:  %v", err)
            }

            // Close the TLS listener
            err = tlsListener.Close()
            if err != nil {
                logMan.LogMessage("Error", "Error closing the TLS listener:  %v", err)
            }

            // Call cancel function to close raw TCP socket
            cancel()

            // Decrement the waitgroup
            waitGroup.Done()
        } ()

        // Receive the file from remote server
        _, err = netio.HandleTransferRecv(transferConn, WordlistPath, fileName, fileSize)
        if err != nil {
            logMan.LogMessage("error", "Error during file transfer:  %v", err)
        }

        MaxTransfers.Add(-1)
        // Subtract the file size of the file transfer that is complete
        transferManager.RemoveTransferSize(fileSize)
    }()
}


// Sets up messaging buffer, receives the hash and ruleset files (if optional
// ruleset applied). Goes into continual loop where it checks the disk space
// and the size on the ongoing file transfers where the combined information
// is used to decide whether there is a proper amount of disk space to initiate
// the transfer (if not there is a brief sleep to reiterate). After the loop
// concludes the cracked hashes and log files are sent back to the server.
//
// @Parameters
//  - connection:  Socket connection for reading data to be stored and processed
//  - hashcatOptChannel:  Channel to signal when hash and ruleset files are received
//  - transferChannel:  Channel to transmit file names after transfer to
//                      initiate data processing
//  - waitGroup:  Used to synchronize the Goroutines running
//  - transferManager:  Manages calculating the amount of data being transferred
//  - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//  - maxFileSize:  The maximum allowed size for a file to be transferred
//
func receivingHandler(connection net.Conn, hashcatOptChannel chan struct{},
                      transferChannel chan struct{}, waitGroup *sync.WaitGroup,
                      transferManager *data.TransferManager,
                      logMan *kloudlogs.LoggerManager, maxFileSizeInt64 int64) {
    // Decrements wait group counter upon local exit
    defer waitGroup.Done()
    var err error
    transferComplete := false

    // Make buffer to messaging size
    buffer := make([]byte, globals.MESSAGE_BUFFER_SIZE)

    // Receive the hash file from the server
    HashFilePath, err = netio.ReceiveFile(connection, buffer, HashesPath,
                                           globals.HASHES_TRANSFER_PREFIX)
    if err != nil {
        logMan.LogMessage("error", "Error receiving hash file:  %v", err)
        return
    }

    // If a rule set was specified
    if HasRuleset {
        // Receive the ruleset from the server
        RulesetFilePath, err = netio.ReceiveFile(connection, buffer, RulesetPath,
                                                 globals.RULESET_TRANSFER_PREFIX)
        if err != nil {
            logMan.LogMessage("error", "Error receiving ruleset file:  %v", err)
            return
        }
    }

    // Send signal to other routine that hash and ruleset file has been received
    hashcatOptChannel <- struct{}{}

    for {
        // Get the remaining available and total disk space
        remainingSpace, total, err := disk.GetDiskSpace(DataPath)
        if err != nil {
            logMan.LogMessage("error", "Error checking disk space on client:  %v", err)
            return
        }

        logMan.LogMessage("info", "Client disk statistics queried",
                          zap.Int64("remaining space", remainingSpace),
                          zap.Int64("total space", total))
        // Get the ongoing transfer size from transfer manager
        ongoingTransferSize := transferManager.GetOngoingTransfersSize()

        // If the remaining space minus the ongoing file transfers is greater than or
        // equal to the max file size AND number of transfers is less than allowed max
        if (remainingSpace - ongoingTransferSize) >= maxFileSizeInt64 &&
        MaxTransfers.Load() != MaxTransfersInt32 {
            // Process the transfer of a file and return file size for the next
            processTransfer(connection, buffer, waitGroup, transferManager,
                            &transferComplete, logMan)
            // If all the transfers are complete exit the data receiving loop
            if transferComplete {
                // Sleep to ensure other routine has time to poll for wordlists
                time.Sleep(5 * time.Second)
                // Send finished inidicator to other goroutine processData()
                transferChannel <- struct{}{}
                break
            }

            continue
        }

        // Sleep to avoid excessive syscalls during idle activity
        time.Sleep(5 * time.Second)
    }
}


// Handle the TCP connection between Goroutine with a channel
// connecting routines to pass messages to signal data to process.
//
// @Parameters
//  - connection:  The network socket connection for handling messaging
//  - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//  - maxFileSize:  The maximum allowed size for a file to be transferred
//
func handleConnection(connection net.Conn,
                      logMan *kloudlogs.LoggerManager,
                      maxFileSizeInt64 int64) {
    // Initialize a transfer mananager used to track the size of active file transfers
    transferManager := data.NewTransferManager()

    // Create channels for the goroutines to communicate
    hashcatOptChannel := make(chan struct{})
    transferChannel := make(chan struct{})
    // Establish a wait group
    var waitGroup sync.WaitGroup
    // Add two goroutines to the wait group
    waitGroup.Add(2)

    // Start the goroutine to write data to the file
    go receivingHandler(connection, hashcatOptChannel, transferChannel, &waitGroup,
                        transferManager, logMan, maxFileSizeInt64)
    // Start the goroutine to process the file
    go processingHandler(connection, hashcatOptChannel, transferChannel, &waitGroup,
                         transferManager, logMan)

    // Wait for both goroutines to finish
    waitGroup.Wait()
}


// Take the IP address & port argument and establish a connection to
// remote brain server, then pass the connection to Goroutine handler.
//
// @Parameters
//  - port:  The port of the remote server
//  - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//  - maxFileSize:  The maximum allowed size for a file to be transferred
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func connectRemote(port int, logMan *kloudlogs.LoggerManager,
                   maxFileSizeInt64 int64) {
    // Set up context handler for TLS listener
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    // Set up the TLS listener to accept incoming connections
    tlsListener, err := TlsMan.SetupTlsListenerHandler(TlsMan.TlsCertificate,
                                                       ctx, "0.0.0.0", port, nil)
    if err != nil {
        logMan.LogMessage("fatal", "Error setting up TLS listener:  %v", err)
    }

    // Close the TLS listener on local exit
    defer func() {
        err = tlsListener.Close()
        if err != nil {
            logMan.LogMessage("error", "Error closing TLS listener:  %v", err)
        }
    } ()

    logMan.LogMessage("info", "Listening for connections on port %d", port)

    // Wait for an incoming connection
    connection, err := tlsListener.Accept()
    if err != nil {
        logMan.LogMessage("error", "Error accepting client connection:  %v", err)
        return
    }

    // Close socket connection on local exit
    defer func() {
        err = connection.Close()
        if err != nil {
            logMan.LogMessage("error", "Error closing server connection:  %v", err)
        }
    } ()

    // Set up goroutines for receiving and processing data
    handleConnection(connection, logMan, maxFileSizeInt64)
}


// Create the required dirs for program operation.
//
func makeClientDirs() {
    // Set the program directories
    programDirs := []string{WordlistPath, HashesPath}

    // If there is a ruleset, append its path to program dirs
    if HasRuleset {
        programDirs = append(programDirs, RulesetPath)
    }

    // Create needed directories
    err := disk.MakeDirs(programDirs)
    if err != nil {
        log.Fatalf("Error creating client dirs:  %v", err)
    }
}


// Parse the command like flags into local and package level variables, make any
// required dirs for program operation. Set up the AWS access config with key and
// secret, set up logging manager, and set up connection with server.
//
func main() {
    var awsRegion string
    var logMode string
    var maxFileSizeInt64 int64
    var maxTransfers int
    var port int

    // Define command line flags with default values and descriptions
    flag.BoolVar(&HashcatArgs.ApplyOptimization, "applyOptimization", false,
                 "Apply the -O flag for GPU optimization")
    flag.StringVar(&awsRegion, "awsRegion", "us-east-1", "The AWS region to deploy EC2 instances")
    flag.StringVar(&HashcatArgs.CharSet1, "charSet1", "", "Custom character set 1 for masks")
    flag.StringVar(&HashcatArgs.CharSet2, "charSet2", "", "Custom character set 2 for masks")
    flag.StringVar(&HashcatArgs.CharSet3, "charSet3", "", "Custom character set 3 for masks")
    flag.StringVar(&HashcatArgs.CharSet4, "charSet4", "", "Custom character set 4 for masks")
    flag.StringVar(&HashcatArgs.CrackingMode, "crackingMode", "0", "Hashcat cracking mode")
    flag.StringVar(&DataPath, "dataPath", "", "Path to directory where program dirs are created")
    flag.StringVar(&Ec2SecurityGroupId, "ec2SecurityGroupId", "", "ID for Security Group for EC2 clients")
    flag.StringVar(&HashcatArgs.HashMask, "hashMask", "", "Mask to apply to hash cracking attempts")
    flag.BoolVar(&HasRuleset, "hasRuleset", false, "Toggle to specify if ruleset is in use")
    flag.StringVar(&HashcatArgs.HashType, "hashType", "1000", "Hashcat hash type to crack")
    flag.StringVar(&logMode, "logMode", "local",
                   "The mode of logging, which support local, CloudWatch, or both")
    flag.Int64Var(&maxFileSizeInt64, "maxFileSizeInt64", 0,
                  "The max size for file to be transmitted at once")
    flag.IntVar(&maxTransfers, "maxTransfers", 2, "Maximum number of files to transfer simultaniously")
    flag.IntVar(&port, "port", 7003, "TCP port to connect to on brain server")
    flag.StringVar(&HashcatArgs.Workload, "workload", "4", "Workload profile number to apply")

    // Parse the command line flags
    flag.Parse()

    // Ensure the max transfers is proper data type
    MaxTransfersInt32 = int32(maxTransfers)

    // Join the base path to the data folders to be created
    HashesPath = path.Join(DataPath, "hashes")
    RulesetPath = path.Join(DataPath, "rulesets")
    WordlistPath = path.Join(DataPath, "wordlists")

    // Create directories for client
    makeClientDirs()

    // Load instance-profile credentials vie metadata service
    awsConfig, err := config.LoadDefaultConfig(context.TODO(),
                                               config.WithRegion(awsRegion))
    if err != nil {
        log.Fatalf("Error loading AWS config:  %v", err)
    }

    // Establish client to EC2 service
    ec2Client := ec2utils.Ec2NewManager(awsConfig)
    // Get public IP from metadata service
    publicIp, err := ec2Client.Ec2GetPublicIpMetadata(false, "120")
    if err != nil {
        log.Fatalf("Error getting EC2 public IP from metadata service:  %v", err)
    }

    // Generate clients TLS PEM certificate and key and save in TLS manager
    err = TlsMan.PemCertAndKeyGen("Kloud Kraken", false, publicIp)
    if err != nil {
        log.Fatalf("Error creating TLS PEM certificate and key:  %v", err)
    }

    // Get instance ID from metadata service
    instanceId, err := ec2Client.Ec2GetInstanceIdMetadata(true, "120")
    if err != nil {
        log.Fatalf("Error getting instance ID from metadata:  %v", err)
    }

    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-ssm-tls-cert",
    }

    // Setup client to SSM
    ssmClient := ssmutils.SsmNewManager(awsConfig)
    // Push the servers certificate PEM into SSM parameter store
    _, err = ssmClient.SsmPutParameter(1 * time.Minute,
                                       "/kloud-kraken/" + instanceId + "/tls-cert",
                                       string(TlsMan.CertPemBlock), true, tags)
    if err != nil {
        log.Fatalf("Error putting TLS certificate in SSM Param Store:  %v", err)
    }

    tags = map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-logs",
    }

    // Initialize the LoggerManager based on the flags
    logMan, err := kloudlogs.NewLoggerManager(logMode, LogPath, awsConfig,
                                              "kloud-kraken-logs", 1,
                                              tags, false)
    if err != nil {
        log.Fatalf("Error initializing logger manager:  %v", err)
    }

    // Connect to remote server to begin receiving data for processing
    connectRemote(port, logMan, maxFileSizeInt64)
}
