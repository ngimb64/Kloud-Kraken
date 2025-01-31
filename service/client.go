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
	"sort"
	"sync"
	"sync/atomic"
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


type HashcatArgs struct {
    CrackingMode      string
    HashType          string
    ApplyOptimization bool
    Workload          string
    CharSet1          string
    CharSet2          string
    CharSet3          string
    CharSet4          string
    HashMask          string
}


// Package level variables
const HashesPath = "/tmp/hashes"       // Path where hash files are stored
const RulesetPath = "/tmp/rulesets"    // Path where ruleset files are stored
const WordlistPath = "/tmp/wordlists"  // Path where wordlists are stored
var Cracked = filepath.Join(HashesPath, "cracked.txt")  // Path to cracked hashes stored post processing
var Loot = filepath.Join(HashesPath, "loot.txt")        // Path to cracked hashes stored permanently
var BufferMutex = &sync.Mutex{}        // Mutex for message buffer synchronization
var HashcatArgsStruct = HashcatArgs{}  // Initialze struct where hashcat options stored
var HashFilePath string        // Stores hash file path when received
var HasRuleset bool            // Toggle for specifying whether ruleset is in use
var LogPath string             // Stores log file to be returned to client
var MaxTransfers atomic.Int32  // Number of file transfers allowed simultaniously
var MaxTransfersInt32 int32    // Stores converted int maxTransfers arg
var MessagePort32 int32        // Initial port for messaging communication
var RulesetFilePath string     // Stores ruleset file when received


func parseHashcatOutput(output []byte, logMan *kloudlogs.LoggerManager) {
    var keys []string
    var logArgs []any
    // Make a map to store parsed data
    outputMap := make(map[string]string)

    // Trim up to the end section with result data
    parsedOutput, err := data.TrimBeforeLast(output, []byte("=>"))
    if err != nil {
        log.Fatalf("Error pre-trimming:  %v", err)
    }

    // Split the byte slice into lines base on newlines
    lines := bytes.Split(parsedOutput, []byte("\n"))

    // Iterate through slice of byte slice lines
    for _, line := range lines {
        // Find the first occurance of the colon separator
        index := bytes.Index(line, []byte(":"))
        // If the line does not contain the index, skip it
        if index == -1 {
            continue
        }

        // Extract the key/value based on the colon separator
        key := bytes.TrimSpace(line[:index])
        value := bytes.TrimSpace(line[index+1:])
        keyStr := string(key)

        // Append the key to the keys string slice
        keys = append(keys, keyStr)
        // Store the key and value as strings in map
        outputMap[keyStr] = string(value)
    }

    // Sort the keys by alphabetical order
    sort.Strings(keys)

    // Iterate through the sorted keys
    for _, key := range keys {
        // Add the key/value from output map based on sorted key value
        logArgs = append(logArgs, zap.String(key, outputMap[key]))
    }

    kloudlogs.LogMessage(logMan, "info", "Hashcat processing results", logArgs...)
}


// Format the result of data processing, send the formatted result of the data processing to the
// remote brain server, and delete the data prior to processing.
//
// @Parameters
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

    // Parse and log the hashcat output
    parseHashcatOutput(output, logMan)
    // Delete the processed file
    os.Remove(filePath)

    // Remove the file size from transfer manager after deletion
    transferManager.RemoveTransferSize(fileSize)
}


func appendCharsets(cmdOptions *[]string) {
    charsets := []string{HashcatArgsStruct.CharSet1, HashcatArgsStruct.CharSet2,
                         HashcatArgsStruct.CharSet3, HashcatArgsStruct.CharSet4}
    var counter int32 = 1

    // Iterate through hashcat charsets
    for _, charset := range charsets {
        // Exit loop is charset is empty or counter is greater than max charset
        if charset == "" || counter > 4 {
            break
        }

        // Append the formated charset flag and corresponding charset
        *cmdOptions = append(*cmdOptions, fmt.Sprintf("-%d", counter), charset)
        counter += 1
    }
}


func sendProcessingComplete(connection net.Conn, logMan *kloudlogs.LoggerManager) {
    // Lock the mutex and ensure it unlocks on local exit
    BufferMutex.Lock()
    defer BufferMutex.Unlock()

    // Send the processing complete message
    _, err := netio.WriteHandler(connection, globals.PROCESSING_COMPLETE,
                                 len(globals.PROCESSING_COMPLETE))
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error sending processing complete message:  %w", err)
        return
    }
}


// Reads data (filename or end transfer message) from channel connected to reader Goroutine,
// takes the received filename and passes it into command execution method for processing,
// and the result is formatted and sent back to the brain server.
//
// @Parameters
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

    cmdOptions := []string{}

    // If GPU optimization is to be applied, append it to options slice
    if HashcatArgsStruct.ApplyOptimization {
        cmdOptions = append(cmdOptions, "-O")
    }

    // Append command args used by all attack modes
    cmdOptions = append(cmdOptions, "--remove", fmt.Sprintf("-o=%s", Cracked), "-a",
                        HashcatArgsStruct.CrackingMode, "-m", HashcatArgsStruct.HashType,
                        "-w", HashcatArgsStruct.Workload, HashFilePath)

    // If a ruleset is in use and it has a path
    if HasRuleset && RulesetFilePath != "" {
        // Append it to the command args
        cmdOptions = append(cmdOptions, "-r", RulesetFilePath)
    }

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

        var cmdArgs []string

        switch HashcatArgsStruct.CrackingMode {
        case "3":
            // Appened incremental mode and available charsets for hash mask
            cmdArgs = append(cmdOptions, "--incremental")
            appendCharsets(&cmdArgs)
            // Append the hash mask
            cmdArgs = append(cmdArgs, HashcatArgsStruct.HashMask)
        case "6":
            // Appened incremental mode and available charsets for hash mask
            cmdArgs = append(cmdOptions, "--incremental")
            appendCharsets(&cmdArgs)
            // Append the wordlist path then the hash mask
            cmdArgs = append(cmdArgs, filePath, HashcatArgsStruct.HashMask)
        case "7":
            // Appened incremental mode and available charsets for hash mask
            cmdArgs = append(cmdOptions, "--incremental")
            appendCharsets(&cmdArgs)
            // Append the hash mask then the wordlist path
            cmdArgs = append(cmdArgs, HashcatArgsStruct.HashMask, filePath)
        default:
            // For straight mode (0), append the loopback mode and wordlist path
            cmdArgs = append(cmdOptions, "--loopback", filePath)
        }

        // Register a command with selected file path
        cmd = exec.Command("hashcat", cmdArgs...)
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
// @Parameters
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
    _, err := netio.WriteHandler(connection, globals.TRANSFER_REQUEST_MARKER,
                                 len(globals.TRANSFER_REQUEST_MARKER))
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                             "Error sending the transfer request to brain server:  %w", err)
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
    fileName, fileSize, err := netio.GetFileInfo(buffer, globals.START_TRANSFER_PREFIX)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                             "Error extracting file name and size from start transfer message:  %w", err)
        return
    }

    // Format the wordlist file path based on received file name
    filePath := fmt.Sprintf("%s/%s", WordlistPath, fileName)

    // Make a small int32 buffer
    int32Buffer := make([]byte, 4)
    // Get random available port as a listener
    listener, port := netio.GetAvailableListener(logMan)

    // Convert int32 port to bytes and write it into the buffer
    err = binary.Write(bytes.NewBuffer(int32Buffer), binary.LittleEndian, port)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                             "Error occurred converting int32 port to byte array:  %w", err)
        return
    }

    // Send the converted port bytes to server to notify open port to connect for transfer
    _, err = netio.WriteHandler(connection, int32Buffer, len(int32Buffer))
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                             "Error occurred sending converted int32 port to server:  %w", err)
        return
    }

    // Wait for an incoming connection
    transferConn, err := listener.Accept()
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error accepting server connection:  %w", err)
        return
    }

    // Increment wait group and max transfers counter
    waitGroup.Add(1)
    MaxTransfers.Add(1)
    // Add the file size of the file to be transfered to transfer manager
    transferManager.AddTransferSize(fileSize)

    // Now synchronized messages finished, handle transfer with new connection and
    // decrement counter when complete
    go func() {
        netio.HandleTransferRecv(transferConn, filePath, fileSize, MessagePort32, logMan, waitGroup)
        MaxTransfers.Add(-1)
    }()
}


// Concurrently reads data from TCP socket connection until entire file has been
// transferred. Afterwards the file name is passed through a channel to the process
// data Goroutine to load the file into data processing.
//
// @Parameters
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
                                     globals.HASHES_TRANSFER_PREFIX)
    // If a rule set was specified
    if HasRuleset {
        // Receive the ruleset from the server
        RulesetFilePath = netio.ReceiveFile(connection, buffer, MessagePort32, logMan, RulesetPath,
                                            globals.RULESET_TRANSFER_PREFIX)
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

        // If the remaining space minus the ongoing file transfers is greater than or
        // equal to the max file size AND the current number of transfers is less than
        if (remainingSpace - ongoingTransferSize) >= maxFileSizeInt64 &&
        MaxTransfers.Load() != MaxTransfersInt32 {
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
    netio.UploadFile(connection, buffer, MessagePort32, logMan,
                     Loot, globals.LOOT_TRANSFER_PREFIX)
    // Transfer the log file to server
    netio.UploadFile(connection, buffer, MessagePort32, logMan,
                     LogPath, globals.LOG_TRANSFER_PREFIX)
}


// Handle the TCP connection between Goroutine with a channel
// connecting routines to pass messages to signal data to process.
//
// @Parameters
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
// @Parameters
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


func makeClientDirs() {
    // Set the program directories
    programDirs := []string{WordlistPath, HashFilePath}

    // If there is a ruleset, append its path to program dirs
    if HasRuleset {
        programDirs = append(programDirs, RulesetPath)
    }

    // Create needed directories
    disk.MakeDirs(programDirs)
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
    var maxTransfers int

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
    flag.StringVar(&LogPath, "logPath", "KloudKraken.log", "Path to the log file")

    // Parse the command line flags
    flag.Parse()

    // Parsed int args are int32
    MessagePort32 = int32(port)
    MaxTransfersInt32 = int32(maxTransfers)

    // Create directories for client
    makeClientDirs()

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
