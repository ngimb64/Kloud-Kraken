package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"flag"
	"log"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"github.com/ngimb64/Kloud-Kraken/pkg/hashcat"
	"github.com/ngimb64/Kloud-Kraken/pkg/kloudlogs"
	"github.com/ngimb64/Kloud-Kraken/pkg/netio"
	"github.com/ngimb64/Kloud-Kraken/pkg/tlsutils"
	"go.uber.org/zap"
)

// Package level variables
const HashesPath = "/tmp/hashes"       // Path where hash files are stored
const RulesetPath = "/tmp/rulesets"    // Path where ruleset files are stored
const WordlistPath = "/tmp/wordlists"  // Path where wordlists are stored
var BufferMutex = &sync.Mutex{}        // Mutex for message buffer synchronization
var HashFilePath string  // Stores hash file path when received
var HasRuleset bool      // Toggle for specifying whether ruleset is in use
var LogPath string       // Stores log file to be returned to client
var MaxTransfers atomic.Int32  // Number of file transfers allowed simultaniously
var MaxTransfersInt32 int32    // Stores converted int maxTransfers arg
var RulesetFilePath string     // Stores ruleset file when received
var Loot = filepath.Join(HashesPath, "loot.txt")  // Path to cracked hashes stored permanently
var HashcatArgsStruct = new(hashcat.HashcatArgs)  // Initialze struct where hashcat options stored
var TlsMan = new(tlsutils.TlsManager)             // Struct for managing TLS certs, keys, etc.


// Ensure the final cracked hashes file exists and has a message informing
// the user no hashes were cracked.
//
// @ Returns
// - Error if it occurs, otherwise nil on success
//
func createFailureResult() error {
    // Open the final cracked hashes file or create if it does not exist
    hashesHandle, err := os.OpenFile(Loot, os.O_RDWR|os.O_CREATE, 0644)
    if err != nil {
        return err
    }

    // Close the opened cracked hashes file on local exit
    defer hashesHandle.Close()

    // Write a message letting user know that no hashes were cracked
    _, err = hashesHandle.Write([]byte("No available cracked hashses after processing"))
    if err != nil {
        return err
    }

    return nil
}


// Lock mutux for messaging connection and related buffer, send the processing complete message.
//
// @Parameters
// - connection:  network socket connection where procesing complete message is sent
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func sendProcessingComplete(connection net.Conn, logMan *kloudlogs.LoggerManager) {
    // Lock the mutex and ensure it unlocks on local exit
    BufferMutex.Lock()
    defer BufferMutex.Unlock()

    // Send the processing complete message
    _, err := netio.WriteHandler(connection, globals.PROCESSING_COMPLETE,
                                 len(globals.PROCESSING_COMPLETE))
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                             "Error sending processing complete message:  %v", err)
        return
    }
}


// Periodically attempts to select a received file from the wordlist path until signal in channel
// takes the received filename and passes it into command execution method for processing, and
// the result is parse and logged via kloudlogs.
//
// @Parameters
// - connection:  Active socket connection for reading data to be stored and processed
// - hashcatOptChannel:  Channel to signal when the hash and ruleset files has been received
// - transferChannel:  Channel to transmit filenames after transfer to initiate data processing
// - waitGroup:  Acts as a barrier for the Goroutines running
// - transferManager:  Manages calculating the amount of data being transferred locally
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func processingHandler(connection net.Conn, hashcatOptChannel chan struct{}, transferChannel chan struct{},
                       waitGroup *sync.WaitGroup, transferManager *data.TransferManager,
                       logMan *kloudlogs.LoggerManager) {
    completed := false
    // Decrements the wait group counter upon local exit
    defer waitGroup.Done()

    charsets := []string{HashcatArgsStruct.CharSet1, HashcatArgsStruct.CharSet2,
                         HashcatArgsStruct.CharSet3, HashcatArgsStruct.CharSet4}
    cmdOptions := []string{}

    // Get the current working directory
    cwd, err := os.Getwd()
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error getting current dir:  %v", err)
        return
    }

    // Format the path for temp cracked hashes file
    crackedPath := path.Join(cwd, "cracked.txt")

    // If GPU optimization is to be applied, append it to options slice
    if HashcatArgsStruct.ApplyOptimization {
        cmdOptions = append(cmdOptions, "-O")
    }

    // Wait for signal that hash and ruleset files are received
    <-hashcatOptChannel

    // Append command args used by all attack modes
    cmdOptions = append(cmdOptions, "--remove", "-o", crackedPath, "-a",
                        HashcatArgsStruct.CrackingMode, "-m", HashcatArgsStruct.HashType,
                        "-w", HashcatArgsStruct.Workload, HashFilePath)

    // If a ruleset is in use and it has a path
    if HasRuleset && RulesetFilePath != "" {
        // Append it to the command args
        cmdOptions = append(cmdOptions, "-r", RulesetFilePath, "--loopback")
    }

    for {
        // Attempt to get the next available wordlist
        fileName, fileSize, err := disk.CheckDirFiles(WordlistPath)
        if err != nil {
            kloudlogs.LogMessage(logMan, "error", "Error retrieving wordlist from wordlist dir:  %v",
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
                kloudlogs.LogMessage(logMan, "error", "Error retrieving wordlist from wordlist dir:  %v",
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

        switch HashcatArgsStruct.CrackingMode {
        case "3":
            // Appened incremental mode and available charsets for hash mask
            cmdArgs = append(cmdOptions, "--incremental")
            hashcat.AppendCharsets(&cmdArgs, charsets)
            // Append the hash mask
            cmdArgs = append(cmdArgs, HashcatArgsStruct.HashMask)
        case "6":
            // Appened incremental mode and available charsets for hash mask
            cmdArgs = append(cmdOptions, "--incremental")
            hashcat.AppendCharsets(&cmdArgs, charsets)
            // Append the wordlist path then the hash mask
            cmdArgs = append(cmdArgs, filePath, HashcatArgsStruct.HashMask)
        case "7":
            // Appened incremental mode and available charsets for hash mask
            cmdArgs = append(cmdOptions, "--incremental")
            hashcat.AppendCharsets(&cmdArgs, charsets)
            // Append the hash mask then the wordlist path
            cmdArgs = append(cmdArgs, HashcatArgsStruct.HashMask, filePath)
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
                kloudlogs.LogMessage(logMan, "error", "Error executing command:  %v", output)
                return
            }
        }

        // Check to see if cracked hashes file exits after hashcat after processing
        exists, isDir, hasData, err := disk.PathExists(crackedPath)
        if err != nil {
            kloudlogs.LogMessage(logMan, "error",
                                 "Error checking cracked hashes file existence:  %v", err)
            return
        }

        // If cracked hashes file exists and has data
        if exists && !isDir && hasData {
            // If there is data in cracked user hash file prior to processing,
            // append it to the final loot file
            err = disk.AppendFile(crackedPath, Loot)
            if err != nil {
                kloudlogs.LogMessage(logMan, "error", "Error appending data to file:  %v", err,
                                     zap.String("source file", "cracked.txt"),
                                     zap.String("destination file", Loot))
                return
            }
        }

        // Parse the hashcat output
        logArgs := hashcat.ParseHashcatOutput(output, []byte("=>"))
        // Log the hashcat output with kloudlogs
        kloudlogs.LogMessage(logMan, "info", "Hashcat processing results", logArgs...)

        // Delete the processed file
        os.Remove(filePath)
        // Remove the file size from transfer manager after deletion
        transferManager.RemoveTransferSize(fileSize)
    }

    // Set the message buffer size
    buffer := make([]byte, globals.MESSAGE_BUFFER_SIZE)

    // Lock the mutex and ensure it unlocks on local exit
    BufferMutex.Lock()
    defer BufferMutex.Unlock()

    // Check to see if final cracked hashes file exits before sending back to server
    exists, _, hasData, err := disk.PathExists(Loot)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                             "Error checking final cracked hashes file existence:  %v", err)
        return
    }

    // If final cracked hashes does not exist or is empty
    if !exists || !hasData {
        // Ensure final cracked hashes files exists with a message
        // that says cracking attempts were unsuccessful
        err = createFailureResult()
        if err != nil {
            kloudlogs.LogMessage(logMan, "error",
                                 "Error creating unsuccessful attempt message for clint:  %v", err)
            return
        }
    }

    // Transfer the final cracked user hash file to server
    err = netio.UploadFile(connection, buffer, Loot, globals.LOOT_TRANSFER_PREFIX)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                            "Error occured sending the cracked hashes to server:  %v", err)
        return
    }

    // Transfer the log file to server
    err = netio.UploadFile(connection, buffer, LogPath, globals.LOG_TRANSFER_PREFIX)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                             "Error occured sending the log file to server:  %v", err)
        return
    }
}


// Sends transfer message to server, waits for transfer reply with file name and size or
// the end transfer message. Gets an available port and sends it to the server, and
// waits for an incoming connection from the server and uses that new connection to
// initiate file transfer routine.
//
// @Parameters
// - connection:  Active socket connection for reading data to be stored and processed
// - buffer:  The buffer used for processing socket messaging
// - waitGroup:  Used to synchronize the Goroutines running
// - transferManager:  Manages calculating the amount of data being transferred locally
// - transferComplete:  boolean toggle that is to signify when all files have been transfered
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func processTransfer(connection net.Conn, buffer []byte, waitGroup *sync.WaitGroup,
                     transferManager *data.TransferManager, transferComplete *bool,
                     logMan *kloudlogs.LoggerManager) {
    // Lock the mutex and ensure it unlocks on local exit
    BufferMutex.Lock()
    defer BufferMutex.Unlock()

    // Send the transfer request message to initiate file transfer
    _, err := netio.WriteHandler(connection, globals.TRANSFER_REQUEST_MARKER,
                                 len(globals.TRANSFER_REQUEST_MARKER))
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                             "Error sending the transfer request to brain server:  %v", err)
        return
    }

    // Wait to receive the start transfer message from the server
    bytesRead, err := netio.ReadHandler(connection, &buffer)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error start transfer message from server:  %v", err)
        return
    }

    // Slice off any unused bytes in buffer
    readBuffer := buffer[:bytesRead]

    // If the server has completed transferring all data
    if bytes.Contains(readBuffer, globals.END_TRANSFER_MARKER) {
        *transferComplete = true
        return
    }

    // If the read data does not start with special delimiter or end with closed bracket
    if !bytes.HasPrefix(readBuffer, globals.START_TRANSFER_PREFIX) ||
    !bytes.HasSuffix(readBuffer, globals.TRANSFER_SUFFIX) {
        kloudlogs.LogMessage(logMan, "error", "Unusual format in receieved start transfer message")
        return
    }

    // Extract the file name and size from the stripped initial transfer message
    fileName, fileSize, err := netio.GetFileInfo(buffer, globals.START_TRANSFER_PREFIX, bytesRead)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                             "Error extracting file name and size from start transfer message:  %v", err)
        return
    }

    // Make buffer for int port bytes
    intBuffer := make([]byte, 2)
    // Get random available port as a listener
    listener, port := netio.GetAvailableListener()

    // Convert int port to bytes and write it into the buffer
    binary.LittleEndian.PutUint16(intBuffer, uint16(port))

    // Send the converted port bytes to server to notify open port to connect for transfer
    _, err = netio.WriteHandler(connection, intBuffer, len(intBuffer))
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                             "Error occurred sending converted int32 port to server:  %v", err)
        return
    }

    // Set up context handler for TLS listener
    ctx, cancel := context.WithCancel(context.Background())
    // Setup up TLS listener from existing raw TCP listener
    tlsListener, err := tlsutils.SetupTlsListenerHandler(TlsMan.TlsCertificate, TlsMan.CaCertPool,
                                                         ctx, "", port, listener)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error setting TLS listener on client:  %v", err)
    }

    // Wait for an incoming connection
    transferConn, err := tlsListener.Accept()
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error accepting server connection:  %v", err)
        cancel()
        return
    }

    waitGroup.Add(1)
    MaxTransfers.Add(1)
    // Add the file size of the file to be transfered to transfer manager
    transferManager.AddTransferSize(fileSize)

    go func() {
        // Close TLS listener and transfer connection on local exit
        defer cancel()
        defer tlsListener.Close()
        defer transferConn.Close()
        // Decrement the waitgroup on local exit
        defer waitGroup.Done()

        // Receive the file from remote server
        _, err = netio.HandleTransferRecv(transferConn, WordlistPath, string(fileName), fileSize)
        if err != nil {
            kloudlogs.LogMessage(logMan, "error", "Error during file transfer:  %v", err)
        }

        MaxTransfers.Add(-1)
        // Subtract the file size of the file transfer that is complete
        transferManager.RemoveTransferSize(fileSize)
    }()
}


// Sets up messaging buffer, receives the hash and ruleset files (if optional ruleset applied).
// Goes into continual loop where it checks the disk space and the size on the ongoing file
// transfers where the combined information is used to decide whether there is a proper amount
// of disk space to initiate the transfer (if not there is a brief sleep to reiterate). After
// the loop concludes the cracked hashes and log files are sent back to the server.
//
// @Parameters
// - connection:  Active socket connection for reading data to be stored and processed
// - hashcatOptChannel:  Channel to signal when the hash and ruleset files has been received
// - transferChannel:  Channel to transmit filenames after transfer to initiate data processing
// - waitGroup:  Used to synchronize the Goroutines running
// - transferManager:  Manages calculating the amount of data being transferred locally
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
// - maxFileSize:  The maximum allowed size for a file to be transferred
//
func receivingHandler(connection net.Conn, hashcatOptChannel chan struct{}, transferChannel chan struct{},
                      waitGroup *sync.WaitGroup, transferManager *data.TransferManager,
                      logMan *kloudlogs.LoggerManager, maxFileSizeInt64 int64) {
    // Decrements wait group counter upon local exit
    defer waitGroup.Done()
    transferComplete := false

    // Upload the client TLS PEM cert to the server to be added to its cert pool
    _, err := netio.WriteHandler(connection, TlsMan.CertPemBlock, len(TlsMan.CertPemBlock))
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error sending client PEM certificate:  %v", err)
        return
    }

    // Make buffer to messaging size
    buffer := make([]byte, globals.MESSAGE_BUFFER_SIZE)

    // Receive the hash file from the server
    HashFilePath, err = netio.ReceiveFile(connection, buffer, HashesPath,
                                          globals.HASHES_TRANSFER_PREFIX)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error receiving hash file:  %v", err)
        return
    }

    // If a rule set was specified
    if HasRuleset {
        // Receive the ruleset from the server
        RulesetFilePath, err = netio.ReceiveFile(connection, buffer, RulesetPath,
                                                 globals.RULESET_TRANSFER_PREFIX)
        if err != nil {
            kloudlogs.LogMessage(logMan, "error", "Error receiving ruleset file:  %v", err)
            return
        }
    }

    // Send signal to other routine that hash and ruleset file has been received
    hashcatOptChannel <- struct{}{}

    for {
        // Get the remaining available and total disk space
        remainingSpace, total, err := disk.DiskCheck()
        if err != nil {
            kloudlogs.LogMessage(logMan, "error",
                                 "Error checking disk space on client:  %v", err)
            return
        }

        kloudlogs.LogMessage(logMan, "info", "Client disk statistics queried",
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
// - connection:  The network socket connection for handling messaging
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
// - maxFileSize:  The maximum allowed size for a file to be transferred
//
func handleConnection(connection net.Conn, logMan *kloudlogs.LoggerManager,
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
// - ipAddr:  The ip address of the remote server
// - port:  The port of the remote server
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
// - maxFileSize:  The maximum allowed size for a file to be transferred
//
func connectRemote(ipAddr string, port int, logMan *kloudlogs.LoggerManager,
                   maxFileSizeInt64 int64) {
    var err error

    // Define the address of the server to connect to
    serverAddress := ipAddr + ":" + strconv.Itoa(port)

    // Make a connection to the remote server
    connection, err := tls.Dial("tcp", serverAddress,
                                tlsutils.NewClientTLSConfig(TlsMan.TlsCertificate,
                                                            TlsMan.CaCertPool, ipAddr))
    if err != nil {
        kloudlogs.LogMessage(logMan, "fatal", "Error connecting to remote server:  %v", err)
        return
    }

    // Close connection on local exit
    defer connection.Close()

    kloudlogs.LogMessage(logMan, "info", "Connected to remote server",
                         zap.String("ip address", ipAddr), zap.Int("port", port))

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
    disk.MakeDirs(programDirs)
}


// Parse the command like flags into local and package level variables, make any
// required dirs for program operation. Set up the AWS access config with key and
// secret, set up logging manager, and set up connection with server.
//
func main() {
    var ipAddr string
    var port int
    var maxFileSizeInt64 int64
    var logMode string
    var awsRegion string
    var awsAccessKey string
    var awsSecretKey string
    var maxTransfers int
    var isTesting bool
    var testPemCert string

    // Define command line flags with default values and descriptions
    flag.StringVar(&ipAddr, "ipAddr", "localhost", "IP address of brain server to connect to")
    flag.IntVar(&port, "port", 6969, "TCP port to connect to on brain server")
    flag.StringVar(&awsRegion, "awsRegion", "us-east-1", "The AWS region to deploy EC2 instances")
    flag.StringVar(&awsAccessKey, "awsAccessKey", "", "The access key for AWS programmatic access")
    flag.StringVar(&awsSecretKey, "awsSecretKey", "", "The secret key for AWS programmatic access")
    flag.Int64Var(&maxFileSizeInt64, "maxFileSizeInt64", 0,
                  "The max size for file to be transmitted at once")
    flag.StringVar(&HashcatArgsStruct.CrackingMode, "crackingMode", "0", "Hashcat cracking mode")
    flag.StringVar(&HashcatArgsStruct.HashType, "hashType", "1000", "Hashcat hash type to crack")
    flag.BoolVar(&HashcatArgsStruct.ApplyOptimization, "applyOptimization", false,
                 "Apply the -O flag for GPU optimization")
    flag.StringVar(&HashcatArgsStruct.Workload, "workload", "3", "Workload profile number to apply")
    flag.StringVar(&HashcatArgsStruct.CharSet1, "charSet1", "", "Custom character set 1 for masks")
    flag.StringVar(&HashcatArgsStruct.CharSet2, "charSet2", "", "Custom character set 2 for masks")
    flag.StringVar(&HashcatArgsStruct.CharSet3, "charSet3", "", "Custom character set 3 for masks")
    flag.StringVar(&HashcatArgsStruct.CharSet4, "charSet4", "", "Custom character set 4 for masks")
    flag.StringVar(&HashcatArgsStruct.HashMask, "hashMask", "", "Mask to apply to hash cracking attempts")
    flag.BoolVar(&HasRuleset, "hasRuleset", false, "Toggle to specify if ruleset is in use")
    flag.IntVar(&maxTransfers, "maxTransfers", 3, "Maximum number of files to transfer simultaniously")
    flag.StringVar(&logMode, "logMode", "local",
                   "The mode of logging, which support local, CloudWatch, or both")
    flag.StringVar(&LogPath, "logPath", "/tmp/KloudKraken.log", "Path to the log file")
    flag.BoolVar(&isTesting, "isTesting", false, "Toggle to enable testing mode")
    flag.StringVar(&testPemCert, "testPemCert", "", "Path to TLS PEM certificate file for local testing")

    // Parse the command line flags
    flag.Parse()

    MaxTransfersInt32 = int32(maxTransfers)
    // Create directories for client
    makeClientDirs()

    var awsConfig aws.Config
    var err error
    var serverCertPemBlock []byte

    // If the program is being run in full mode (not testing)
    if !isTesting {
        // If AWS access and secret key are present
        if awsAccessKey == "" || awsSecretKey == "" {
            log.Fatalf("AWS access key or secret key missing and not in testing mode")
        }

        // Set the AWS credentials provider
        awsCreds := credentials.NewStaticCredentialsProvider(awsAccessKey, awsSecretKey, "")

        // Load default config and override with custom credentials and region
        awsConfig, err = config.LoadDefaultConfig(
            context.TODO(),
            config.WithRegion(awsRegion),
            config.WithCredentialsProvider(awsCreds),
        )

        if err != nil {
            log.Fatalf("Error loading client AWS config:  %v", err)
        }

        // Retrieve the server TLS cert from AWS param store
        certPemString, err := awsutils.GetBytesParameter(awsConfig, "/kloud-kraken/tls/cert")
        if err != nil {
            log.Fatalf("Error getting server TLS cert via SSM Parameter Store:  %v", err)
        }

        // Convert retrieved TLS cert PEM block to bytes
        serverCertPemBlock = []byte(certPemString)

    // If the program is being run in testing mode
    } else {
        // Load the servers TLS certifcate PEM block
        serverCertPemBlock, err = os.ReadFile(testPemCert)
        if err != nil {
            log.Fatalf("Error reading TLS certificate PEM file:  %v", err)
        }
    }

    // Generate the servers TLS PEM certificate and key and save in TLS manager
    TlsMan.CertPemBlock, TlsMan.KeyPemBlock, err = tlsutils.PemCertAndKeyGenHandler("Kloud Kraken", false)
    if err != nil {
        log.Fatalf("Error creating TLS PEM certificate and key:  %v", err)
    }

    // Generate a TLS x509 certificate and cert pool
    TlsMan.TlsCertificate, TlsMan.CaCertPool, err = tlsutils.CertGenAndPool(TlsMan.CertPemBlock,
                                                                            TlsMan.KeyPemBlock,
                                                                            TlsMan.CaCertPemBlocks)
    if err != nil {
        log.Fatalf("Error generating TLS certificate:  %v", err)
    }

    // Append the client TLS cert PEM block to management list
    TlsMan.AddCACert(serverCertPemBlock)

    // Initialize the LoggerManager based on the flags
    logMan, err := kloudlogs.NewLoggerManager(logMode, LogPath, awsConfig, false)
    if err != nil {
        log.Fatalf("Error initializing logger manager:  %v", err)
    }

    // Connect to remote server to begin receiving data for processing
    connectRemote(ipAddr, port, logMan, maxFileSizeInt64)
}
