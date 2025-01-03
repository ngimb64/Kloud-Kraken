package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
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
const HashesPath = "/tmp/hashes"       // Path where hash file is stored
var BufferMutex = &sync.Mutex{}        // Mutex for message buffer synchronization
var Cracked = filepath.Join(HashesPath, "cracked.txt")  // Path cracked hashes are temporarily stored post processing
var Loot = filepath.Join(HashesPath, "loot.txt")        // Path cracked hashes are stored permanently
var CrackingMode = ""  // Stores cracking mode arg
var HashFilePath = ""  // Stores hash file path when received
var HashType = ""      // Stores hash type to be cracked
var Port32 int32 = 0   // Initial port for messaging communication


// Format the result of data processing, send the formatted result of the data processing to the
// remote brain server, and delete the data prior to processing.
//
// Parameters:
// - filePath:  The path to the file to remove after the results are transfered back
// - output:  The raw output of the data processing to formatted and transferred
// - fileSize:  The size of the to be stored on disk from read socket data
// - transferManager:  Manages calculating the amount of data being transferred locally
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func logAndRemove(filePath string, output []byte, fileSize int64, transferManager *data.TransferManager,
                  logMan *kloudlogs.LoggerManager) {

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
        kloudlogs.LogMessage(logMan, "error", "Error sending processing complete message:  %v", err)
        return
    }
}


// TODO:  delete dummy function after below logic is fixed
func getWordlist(wordlistPath string) (string, int64) {
    return "", 0
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
func processData(connection net.Conn, channel chan bool, waitGroup *sync.WaitGroup,
                 transferManager *data.TransferManager, logMan *kloudlogs.LoggerManager) {
    // Decrements the wait group counter upon local exit
    defer waitGroup.Done()

    for {

        // TODO:  adjust logic to check directory until file exists and if more than one
        //        file exists combine them together with module to fuse multiple wordlists
        //        which will replace below dummy function

        filePath, fileSize := getWordlist(WordlistPath)

        select {
        // Poll channel for transfer complete message
        case isComplete := <-channel:
            // If transfers are complete and there is no wordlist in designated directory
            if isComplete && filePath == "" {
                // Send the processing complete message to server
                sendProcessingComplete(connection, logMan)
                break
            }
        default:
            // If there was no wordlist available in designated directory
            if filePath == "" {
                // Sleep a bit and re-iterate to see if wordlist is available
                time.Sleep(3 * time.Second)
                continue
            }
        }

        // Register a command with selected file path
        cmd := exec.Command("hashcat", "-O", "--machine-readable", "--remove",
                            fmt.Sprintf("-o=%s", Cracked), "-a", CrackingMode,
                            "-m", HashType, "-w", "3", HashFilePath, filePath)
        // Execute and save the command stdout and stderr output
        output, err := cmd.CombinedOutput()
        if err != nil {
            kloudlogs.LogMessage(logMan, "error", "Error executing command:  %v", err)
            return
        }

        // TODO:  after processing take the cracked hash file and append the data to a final hash file
        //        so the data will not be lost next time hashcat runs

        // In a separate goroutine, which will log the output and delete processed data
        go logAndRemove(filePath, output, fileSize, transferManager, logMan)
    }
}


// Reads data from the socket and write it to the passed in open file descriptor until end
// of file has been reached or error occurs with socket operation.
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - transferBuffer:  Buffer allocated for file transfer based on file size
// - file:  The open file descriptor of where the data to be processed will be stored
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func socketToFileHander(connection net.Conn, transferBuffer []byte, file *os.File,
                        logMan *kloudlogs.LoggerManager) {
    // Close file on local exit
    defer file.Close()

    for {
        // Read data into the buffer
        _, err := netio.ReadHandler(connection, &transferBuffer)
        if err != nil {
            // If the error is not End Of File reached
            if err != io.EOF {
                kloudlogs.LogMessage(logMan, "error", "Error reading from socket:  %v", err)
                return
            }

            break
        }

        // Write the data to the file
        _, err = file.Write(transferBuffer)
        if err != nil {
            kloudlogs.LogMessage(logMan, "error", "Error writing to file:  %v", err)
            return
        }
    }
}


// Takes the passed in file name and parses it to the file path to create the file where the
// resulting file will be stored to lated be used for processing.
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - filePath:  The path of the file to be stored on disk from read socket data
// - fileSize:  The size of the to be stored on disk from read socket data
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
// - waitGroup:  Used to synchronize the Goroutines running
//
func handleTransfer(connection net.Conn, filePath string, fileSize int64,
                    logMan *kloudlogs.LoggerManager, waitGroup *sync.WaitGroup) {
    // If a waitgroup was passed in
    if waitGroup != nil {
        // Decrement wait group on local exit
        defer waitGroup.Done()
    }

    // Get the IP address from the ip:port host address
    _, port, err := netio.GetIpPort(connection)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                             "Error occcurred spliting host address to get IP/port:  %v", err)
        return
    }

    // If the parsed port of the passed in connection does not
    // match the original port used to manage messaging
    if port != Port32 {
        // Ensure the transfer connection is closed upon local exit
        defer connection.Close()
    }

    //  Create buffer to optimal size based on expected file size
    transferBuffer := make([]byte, netio.GetOptimalBufferSize(fileSize))

    kloudlogs.LogMessage(logMan, "info", "File transfer initiated", zap.String("file path", filePath),
                         zap.Int64("file size", fileSize))

    // Open the file for writing
    file, err := os.Create(filePath)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error creating the file %s:  %v", filePath, err)
        return
    }

    // Read data from the socket and write to the file path
    socketToFileHander(connection, transferBuffer, file, logMan)
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
        kloudlogs.LogMessage(logMan, "error", "Error sending the transfer request to brain server:  %v", err)
        return
    }

    // Wait for the brain server to send the start transfer message
    _, err = netio.ReadHandler(connection, &buffer)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error start transfer message from server:  %v", err)
        return
    }

    // If the brain has completed transferring all data
    if bytes.Contains(buffer, globals.END_TRANSFER_MARKER) {
        *transferComplete = true
        return
    }

    // If the read data does not start with special delimiter or end with closed bracket
    if !bytes.HasPrefix(buffer, globals.START_TRANSFER_PREFIX) ||
    !bytes.HasSuffix(buffer, globals.START_TRANSFER_SUFFIX) {
        kloudlogs.LogMessage(logMan, "error", "Unusual format in receieved start transfer message")
        return
    }

    // Trim the delimiters around the file info
    buffer = buffer[len(globals.START_TRANSFER_PREFIX) : len(buffer) - len(globals.START_TRANSFER_SUFFIX)]
    // Extract the file name and size from the stripped initial transfer message
    fileName, fileSize, err := netio.GetFileInfo(buffer)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                             "Error extracting the file name and size from start transfer message:  %v", err)
        return
    }

    // Format the wordlist file path based on received file name
    filePath := fmt.Sprint("%s/%s", WordlistPath, fileName)

    // Make a small int32 buffer
    int32Buffer := make([]byte, 4)
    // Get random available port as a listener
    listener, port := netio.GetAvailableListener(logMan)

    // Convert int32 port to bytes and write it into the buffer
    err = binary.Write(bytes.NewBuffer(int32Buffer), binary.BigEndian, port)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error occurred converting int32 port to byte array:  %v", err)
        return
    }

    // Send the converted port bytes to server to notify open port to connect for transfer
    _, err = netio.WriteHandler(connection, &int32Buffer)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error occurred sending converted int32 port to server:  %v", err)
        return
    }

    // Wait for an incoming connection
    transferConn, err := listener.Accept()
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error accepting server connection:  %v", err)
        return
    }

    // Increment wait group
    waitGroup.Add(1)
    // Add the file size of the file to be transfered to transfer manager
    transferManager.AddTransferSize(fileSize)
    // Now synchronized messages are complete, handle transfer with new connection in routine
    go handleTransfer(transferConn, filePath, fileSize, logMan, waitGroup)
}


// Receives the file of hash to be cracked from the brain server.
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - buffer:  The buffer used for processing socket messaging
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func receiveHashFile(connection net.Conn, buffer []byte, logMan *kloudlogs.LoggerManager) {
    // Wait for the brain server to send the start transfer message
    _, err := netio.ReadHandler(connection, &buffer)
    if err != nil {
        kloudlogs.LogMessage(logMan, "fatal", "Error receiving hash transfer message from server:  %v", err)
        os.Exit(2)
    }

    // If the read data does not start with special delimiter or end with closed bracket
    if !bytes.HasPrefix(buffer, globals.HASHES_TRANSFER_PREFIX) ||
    !bytes.HasSuffix(buffer, globals.HASHES_TRANSFER_SUFFIX) {
        kloudlogs.LogMessage(logMan, "fatal", "Unusual format in receieved hashes transfer message")
        os.Exit(2)
    }

    // Trim the delimiters around the file info
    buffer = buffer[len(globals.HASHES_TRANSFER_PREFIX) : len(buffer) - len(globals.HASHES_TRANSFER_SUFFIX)]
    // Extract the file name and size from the initial transfer message
    fileName, fileSize, err := netio.GetFileInfo(buffer)
    if err != nil {
        kloudlogs.LogMessage(logMan, "fatal",
                             "Error extracting the file name and size from hashes transfer message:  %v", err)
        os.Exit(2)
    }

    // Format the hash file path based on received file name
    HashFilePath = fmt.Sprintf("%s/%s", HashesPath, fileName)
    // Receive the hash file from server
    handleTransfer(connection, HashFilePath, fileSize, logMan, nil)
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
func receiveData(connection net.Conn, channel chan bool, waitGroup *sync.WaitGroup,
                transferManager *data.TransferManager, logMan *kloudlogs.LoggerManager,
                maxFileSizeInt64 int64) {
    // Decrements wait group counter upon local exit
    defer waitGroup.Done()

    transferComplete := false
    // Set the message buffer size
    buffer := make([]byte, globals.MESSAGE_BUFFER_SIZE)
    // Receive the hash file from the server
    receiveHashFile(connection, buffer, logMan)

    for {
        // Get the remaining available and total disk space
        remainingSpace, total := disk.DiskCheck()

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
                channel <- true
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
    go receiveData(connection, channel, &waitGroup,
                   &transferManager, logMan, maxFileSizeInt64)
    // Start the goroutine to process the file
    go processData(connection, channel, &waitGroup, &transferManager, logMan)

    // Wait for both goroutines to finish
    waitGroup.Wait()

    // TODO:  add logic to transfer cracker user hash file to server

    // TODO:  add logic to transfer log file to server
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
    serverAddress := fmt.Sprintf("%s:%s", ipAddr, Port32)

    // Make a connection to the remote brain server
    connection, err := net.Dial("tcp", serverAddress)
    if err != nil {
        kloudlogs.LogMessage(logMan, "fatal", "Error connecting to remote server:  %v", err)
        return
    }

    kloudlogs.LogMessage(logMan, "info", "Connected remote server at %s on port %d", ipAddr, Port32)

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
    var logPath string
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
    flag.StringVar(&logPath, "logPath", "KloudKraken.log", "Path to the log file")
    flag.StringVar(&CrackingMode, "crackingMode", "0", "Hashcat cracking mode")
    flag.StringVar(&HashType, "hashType", "1000", "Hashcat hash type to crack")
    // Parse the command line flags
    flag.Parse()

    // Ensure parsed port is int32
    Port32 = int32(port)

    // Ensure the wordlist directory exists
    err := os.MkdirAll(WordlistPath, os.ModePerm)
    if err != nil {
        log.Fatalf("Error creating wordlist dir:  %v", err)
    }

    // Ensure the hashes directory exists
    err = os.MkdirAll(HashFilePath, os.ModePerm)
    if err != nil {
        log.Fatalf("Error creating hashes dir:  %v", err)
    }

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
    logMan, err := kloudlogs.NewLoggerManager(logMode, logPath, awsConfig)
    if err != nil {
        log.Fatalf("Error initializing logger manager: %v", err)
    }

    // Connect to remote server to begin receiving data for processing
    connectRemote(ipAddr, logMan, maxFileSizeInt64)
}
