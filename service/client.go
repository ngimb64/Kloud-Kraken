package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"github.com/ngimb64/Kloud-Kraken/pkg/kloudlogs"
	"github.com/ngimb64/Kloud-Kraken/pkg/netio"
	"go.uber.org/zap"
)

// Package level variables
const WordlistPath = "/tmp/wordlists"  // Path where wordlists are stored
const HashesPath = "/tmp/hashes"       // Path where hash files are stored
const RulesetPath = "/tmp/rulesets"    // Path where ruleset files are stored
var BufferMutex = &sync.Mutex{}        // Mutex for message buffer synchronization
var Cracked = filepath.Join(HashesPath, "cracked.txt")  // Path to cracked hashes stored post processing
var Loot = filepath.Join(HashesPath, "loot.txt")        // Path to cracked hashes stored permanently
var CrackingMode = ""        // Stores cracking mode arg
var HashFilePath = ""        // Stores hash file path when received
var LogPath = ""             // Stores log file to be returned to client
var RulesetFilePath = ""     // Stores ruleset file when received
var HashType = ""            // Stores hash type to be cracked
var MessagePort32 int32 = 0  // Initial port for messaging communication
var HasRuleset bool          // Toggle for specifying whether ruleset is in use


// Format the result of data processing, send the formatted result of the data processing to the
// remote brain server, and delete the data prior to processing.
//
// Parameters:
// - filePath:  The path to the file to remove after the results are transfered back
// - output:  The raw output of the data processing to formatted and transferred
// - fileSize:  The size of the to be stored on disk from read socket data
// - transferManager:  Manages calculating the amount of data being transferred locally
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
// - waitGroup:  Acts as a barrier for the Goroutines running
//
func logAndRemove(filePath string, output []byte, fileSize int64, transferManager *data.TransferManager,
                  logMan *kloudlogs.LoggerManager, waitGroup *sync.WaitGroup) {
    // Decrement wait group on local exit
    waitGroup.Done()


    // TODO:  parse the hashcat output to kloudlogs


    // Delete the processed file
    os.Remove(filePath)

    // Remove the file size from transfer manager after deletion
    transferManager.RemoveTransferSize(fileSize)
}


func sendProcessingComplete(connection net.Conn, logMan *kloudlogs.LoggerManager) {
    // Lock the mutex and ensure it unlocks on local exit
    BufferMutex.Lock()
    defer BufferMutex.Unlock()

    // Send the processing complete message
    _, err := netio.WriteHandler(connection, &globals.PROCESSING_COMPLETE)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error sending processing complete message:  %w", err)
        return
    }
}


// Reads data (filename or end transfer message) from channel connected to reader Goroutine,
// takes the received filename and passes it into command execution method for processing,
// and the result is formatted and sent back to the brain server.
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - waitGroup:  Acts as a barrier for the Goroutines running
// - transferManager:  Manages calculating the amount of data being transferred locally
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func processingHandler(connection net.Conn, channel chan bool, waitGroup *sync.WaitGroup,
                       transferManager *data.TransferManager, logMan *kloudlogs.LoggerManager) {
    var cmd *exec.Cmd
    exitLoop := false
    // Decrements the wait group counter upon local exit
    defer waitGroup.Done()
    // Format file path arg where cracked hashes are stored post processing
    crackedOutfile := fmt.Sprintf("-o=%s", Cracked)

    for {
        // Attempt to get the next available wordlist
        filePath, fileSize, err := disk.CheckDirFiles(WordlistPath)
        if err != nil {
            kloudlogs.LogMessage(logMan, "error", "Error retrieving wordlist from wordlist dir:  %w", err,
                                 zap.String("wordlist directory", WordlistPath))
            return
        }

        select {
        // Poll channel for transfer complete message
        case isComplete := <-channel:
            // If transfers are complete and there is no wordlist in designated directory
            if isComplete && filePath == "" {
                // Send the processing complete message to server
                sendProcessingComplete(connection, logMan)
                exitLoop = true
            }
        default:
            // If there was no wordlist available in designated directory
            if filePath == "" {
                // Sleep a bit and re-iterate to see if wordlist is available
                time.Sleep(3 * time.Second)
                continue
            }
        }

        if exitLoop {
            break
        }

        // If a ruleset is in use and it has a path
        if HasRuleset && RulesetFilePath != "" {
            // Register a command with selected file path
            cmd = exec.Command("hashcat", "-O", "--machine-readable", "--remove", crackedOutfile,
                               "-a", CrackingMode, "-m", HashType, "-r", RulesetFilePath, "-w", "3",
                               HashFilePath, filePath)
        } else {
            // Register a command with selected file path
            cmd = exec.Command("hashcat", "-O", "--machine-readable", "--remove", crackedOutfile,
                               "-a", CrackingMode, "-m", HashType, "-w", "3", HashFilePath, filePath)
        }
        // Execute and save the command stdout and stderr output
        output, err := cmd.CombinedOutput()
        if err != nil {
            kloudlogs.LogMessage(logMan, "error", "Error executing command:  %w", err)
            return
        }

        // If there is data in cracked user hash file prior to processing,
        // append it to the final loot file
        err = disk.AppendFile(Cracked, Loot)
        if err != nil {
            kloudlogs.LogMessage(logMan, "error", "Error appending data to file:  %w", err,
                                 zap.String("source file", Cracked),
                                 zap.String("destination file", Loot))
            return
        }

        // Increment wait group
        waitGroup.Add(1)
        // In a separate goroutine, which will log the output and delete processed data
        go logAndRemove(filePath, output, fileSize, transferManager, logMan, waitGroup)
    }
}


// Sends transfer message to the brain server, waits for transfer reply with file name and
// size, and proceeds to call handle transfer method.
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - buffer:  The buffer used for processing socket messaging
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
    _, err := netio.WriteHandler(connection, &globals.TRANSFER_REQUEST_MARKER)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error sending the transfer request to brain server:  %w", err)
        return
    }

    // Wait for the brain server to send the start transfer message
    _, err = netio.ReadHandler(connection, &buffer)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error start transfer message from server:  %w", err)
        return
    }

    // If the brain has completed transferring all data
    if bytes.Contains(buffer, globals.END_TRANSFER_MARKER) {
        *transferComplete = true
        return
    }

    // If the read data does not start with special delimiter or end with closed bracket
    if !bytes.HasPrefix(buffer, globals.START_TRANSFER_PREFIX) ||
    !bytes.HasSuffix(buffer, globals.TRANSFER_SUFFIX) {
        kloudlogs.LogMessage(logMan, "error", "Unusual format in receieved start transfer message")
        return
    }

    // Extract the file name and size from the stripped initial transfer message
    fileName, fileSize, err := netio.GetFileInfo(buffer, globals.START_TRANSFER_PREFIX,
                                                 globals.TRANSFER_SUFFIX)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                             "Error extracting the file name and size from start transfer message:  %w", err)
        return
    }

    // Format the wordlist file path based on received file name
    filePath := fmt.Sprintf("%s/%s", WordlistPath, fileName)

    // Make a small int32 buffer
    int32Buffer := make([]byte, 4)
    // Get random available port as a listener
    listener, port := netio.GetAvailableListener(logMan)

    // Convert int32 port to bytes and write it into the buffer
    err = binary.Write(bytes.NewBuffer(int32Buffer), binary.BigEndian, port)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error occurred converting int32 port to byte array:  %w", err)
        return
    }

    // Send the converted port bytes to server to notify open port to connect for transfer
    _, err = netio.WriteHandler(connection, &int32Buffer)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error occurred sending converted int32 port to server:  %w", err)
        return
    }

    // Wait for an incoming connection
    transferConn, err := listener.Accept()
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error accepting server connection:  %w", err)
        return
    }

    // Increment wait group
    waitGroup.Add(1)
    // Add the file size of the file to be transfered to transfer manager
    transferManager.AddTransferSize(fileSize)
    // Now synchronized messages are complete, handle transfer with new connection in routine
    go netio.HandleTransfer(transferConn, filePath, fileSize, MessagePort32, logMan, waitGroup)
}


// Concurrently reads data from TCP socket connection until entire file has been
// transferred. Afterwards the file name is passed through a channel to the process
// data Goroutine to load the file into data processing.
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - channel:  Channel to transmit filenames after transfer to initiate data processing
// - waitGroup:  Used to synchronize the Goroutines running
// - transferManager:  Manages calculating the amount of data being transferred locally
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
// - maxFileSize:  The maximum allowed size for a file to be transferred
//
func receivingHandler(connection net.Conn, channel chan bool, waitGroup *sync.WaitGroup,
                      transferManager *data.TransferManager, logMan *kloudlogs.LoggerManager,
                      maxFileSizeInt64 int64) {
    // Decrements wait group counter upon local exit
    defer waitGroup.Done()

    transferComplete := false
    // Set the message buffer size
    buffer := make([]byte, globals.MESSAGE_BUFFER_SIZE)
    // Receive the hash file from the server
    HashFilePath = netio.ReceiveFile(connection, buffer, MessagePort32, logMan, HashesPath,
                                     globals.HASHES_TRANSFER_PREFIX,
                                     globals.TRANSFER_SUFFIX)
    // If a rule set was specified
    if HasRuleset {
        // Receive the ruleset from the server
        RulesetFilePath = netio.ReceiveFile(connection, buffer, MessagePort32, logMan, RulesetPath,
                                            globals.RULESET_TRANSFER_PREFIX,
                                            globals.TRANSFER_SUFFIX)
    }

    for {
        // Get the remaining available and total disk space
        remainingSpace, total, err := disk.DiskCheck()
        if err != nil {
            kloudlogs.LogMessage(logMan, "error",
                                 "Error checking disk space on client:  %w", err)
        }

        kloudlogs.LogMessage(logMan, "info", "Client disk statistics queried",
                             zap.Int64("remaining space", remainingSpace),
                             zap.Int64("total space", total))
        // Get the ongoing transfer size from transfer manager
        ongoingTransferSize := transferManager.GetOngoingTransfersSize()

        // If the remaining space minus the ongoing file transfers
        // is greater than or equal to the max file size
        if (remainingSpace - ongoingTransferSize) >= maxFileSizeInt64 {
            // Process the transfer of a file and return file size for the next
            processTransfer(connection, buffer, waitGroup, transferManager,
                            &transferComplete, logMan)
            // If all the transfers are complete exit the data receiving loop
            if transferComplete {
                // Send finished inidicator to other goroutine processData()
                channel <- true
                break
            }
            continue
        }

        // Sleep to avoid excessive syscalls during idle activity
        time.Sleep(5 * time.Second)
    }

    // Transfer the cracked user hash file to server
    netio.UploadFile(connection, &buffer, MessagePort32, logMan, Loot)
    // Transfer the log file to server
    netio.UploadFile(connection, &buffer, MessagePort32, logMan, LogPath)
}


// Handle the TCP connection between Goroutine with a channel
// connecting routines to pass messages to signal data to process.
//
// Parameters:
// - connection:  The TCP socket connection utilized for transferring data
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
// - maxFileSize:  The maximum allowed size for a file to be transferred
//
func handleConnection(connection net.Conn, logMan *kloudlogs.LoggerManager,
                      maxFileSizeInt64 int64) {
    // Initialize a transfer mananager used to track the size of active file transfers
    transferManager := data.TransferManager{}

    // Create a channel for the goroutines to communicate
    channel := make(chan bool)
    // Establish a wait group
    var waitGroup sync.WaitGroup
    // Add two goroutines to the wait group
    waitGroup.Add(2)

    // Start the goroutine to write data to the file
    go receivingHandler(connection, channel, &waitGroup, &transferManager, logMan,
                        maxFileSizeInt64)
    // Start the goroutine to process the file
    go processingHandler(connection, channel, &waitGroup, &transferManager, logMan)

    // Wait for both goroutines to finish
    waitGroup.Wait()
}


// Take the IP address & port argument and establish a connection to
// remote brain server, then pass the connection to Goroutine handler.
//
// Parameters:
// - ipAddr:  The ip address of the remote brain server
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
// - maxFileSize:  The maximum allowed size for a file to be transferred
//
func connectRemote(ipAddr string, logMan *kloudlogs.LoggerManager, maxFileSizeInt64 int64) {
    // Define the address of the server to connect to
    serverAddress := fmt.Sprintf("%s:%d", ipAddr, MessagePort32)

    // Make a connection to the remote brain server
    connection, err := net.Dial("tcp", serverAddress)
    if err != nil {
        kloudlogs.LogMessage(logMan, "fatal", "Error connecting to remote server:  %w", err)
        return
    }

    kloudlogs.LogMessage(logMan, "info", "Connected to remote server",
                         zap.String("ip address", ipAddr), zap.Int32("port", MessagePort32))

    // Close connection on local exit
    defer connection.Close()
    // Set up goroutines for receiving and processing data
    handleConnection(connection, logMan, maxFileSizeInt64)
}


func main() {
    var ipAddr string
    var port int
    var maxFileSizeInt64 int64
    var logMode string
    var awsRegion string
    var awsAccessKey string
    var awsSecretKey string
    var awsConfig aws.Config

    // Define command line flags with default values and descriptions
    flag.StringVar(&ipAddr, "ipAddr", "localhost",
                   "IP address of brain server to connect to")
    flag.IntVar(&port, "port", 6969, "TCP port to connect to on brain server")
    flag.Int64Var(&maxFileSizeInt64, "maxFileSizeInt64", 0,
                  "The max size for file to be transmitted at once")
    flag.StringVar(&awsRegion, "awsRegion", "us-east-1", "The AWS region to deploy EC2 instances")
    flag.StringVar(&awsAccessKey, "awsAccessKey", "", "The access key for AWS programmatic access")
    flag.StringVar(&awsSecretKey, "awsSecretKey", "", "The secret key for AWS programmatic access")
    flag.StringVar(&logMode, "logMode", "local",
                   "The mode of logging, which support local, CloudWatch, or both")
    flag.StringVar(&LogPath, "logPath", "KloudKraken.log", "Path to the log file")
    flag.StringVar(&CrackingMode, "crackingMode", "0", "Hashcat cracking mode")
    flag.StringVar(&HashType, "hashType", "1000", "Hashcat hash type to crack")
    flag.BoolVar(&HasRuleset, "hasRuleset", false, "Toggle to specify if ruleset is in use")
    // Parse the command line flags
    flag.Parse()

    // Ensure parsed port is int32
    MessagePort32 = int32(port)

    // Set the program directories
    programDirs := []string{WordlistPath, HashFilePath}

    // If there is a ruleset, append its path to program dirs
    if HasRuleset {
        programDirs = append(programDirs, RulesetPath)
    }

    // Create needed directories
    disk.MakeDirs(programDirs)

    // If AWS access and secret key are present
    if awsAccessKey != "" && awsSecretKey != "" {
        // Set AWS config for CloudWatch logging
        awsConfig = aws.Config {
            Region:      awsRegion,
            Credentials: credentials.NewStaticCredentialsProvider(awsAccessKey, awsSecretKey, ""),
        }
    // Otherwise if the logging is not set to local
    } else if logMode != "local" {
        log.Fatal("Missing AWS API credentials but log mode is not set to local")
    }

    // Initialize the LoggerManager based on the flags
    logMan, err := kloudlogs.NewLoggerManager(logMode, LogPath, awsConfig)
    if err != nil {
        log.Fatalf("Error initializing logger manager:  %v", err)
    }

    // Connect to remote server to begin receiving data for processing
    connectRemote(ipAddr, logMan, maxFileSizeInt64)
}
