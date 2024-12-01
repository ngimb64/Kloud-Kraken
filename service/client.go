package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"github.com/ngimb64/Kloud-Kraken/pkg/kloudlogs"
	"github.com/ngimb64/Kloud-Kraken/pkg/netio"
)

// Package level variables
const StoragePath = "/tmp"	// Path where received files are stored


// Format the result of data processing, send the formatted result of the data processing to the
// remote brain server, and delete the data prior to processing.
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - filePath:  The path to the file to remove after the results are transfered back
// - output:  The raw output of the data processing to formatted and transferred
// - fileSize:  The size of the to be stored on disk from read socket data
// - transferManager:  Manages calculating the amount of data being transferred locally
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func sendAndRemove(connection net.Conn, filePath string, output []byte, fileSize int64,
				   transferManager *data.TransferManager, logMan *kloudlogs.LoggerManager) {
	// TODO:  add code to format the command output to optimal format

	// Send the processing result
	_, err := netio.WriteHandler(connection, &output)
	if err != nil {
		kloudlogs.LogMessage(logMan, "error", "Error sending processing result back:  %v", err)
		return
	}

	// Delete the processed file
	os.Remove(filePath)
	// Remove the file size from transfer manager after deletion
	transferManager.RemoveTransferSize(fileSize)
}


// Reads data (filename or end transfer message) from channel connected to reader Goroutine,
// takes the received filename and passes it into command execution method for processing,
// and the result is formatted and sent back to the brain server.
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - channel:  Channel to transmit filenames after transfer to initiate data processing
// - waitGroup:  Acts as a barrier for the Goroutines running
// - transferManager:  Manages calculating the amount of data being transferred locally
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func processData(connection net.Conn, channel chan []byte, waitGroup *sync.WaitGroup,
				 transferManager *data.TransferManager, logMan *kloudlogs.LoggerManager) {
	// Decrements the wait group counter upon local exit
	defer waitGroup.Done()

	for {
		// Get the filename of the data to process from channel
		channelData := <-channel
		// Parse file name/size from received data via channel from processing routine
		fileName, fileSize, err := netio.GetFileInfo(channelData)
		if err != nil {
			kloudlogs.LogMessage(logMan, "error", "Error parsing file name and size from channel data:  %v", err)
			continue
		}

		// Format the filename in to the path where data is stored
		filePath := path.Join(StoragePath, string(fileName))

		// If the channel contains end transfer message
		if bytes.Contains(fileName, globals.END_TRANSFER_MARKER) {
			// Sleep a bit to ensure all processing results have been sent
			time.Sleep(10 * time.Second)

			// Send the processing complete message
			_, err := netio.WriteHandler(connection, &globals.PROCESSING_COMPLETE)
			if err != nil {
				kloudlogs.LogMessage(logMan, "error", "Error sending processing complete message:  %v", err)
				return
			}
			break
		}

		// TODO:  put actual command syntax below

		// Register a command with file path gathered from channel
		cmd := exec.Command("command", "-flag", filePath)
		// Execute and save the command stdout and stderr output
		output, err := cmd.CombinedOutput()
		if err != nil {
			kloudlogs.LogMessage(logMan, "error", "Error executing command:  %v", err)
			return
		}

		// In a separate goroutine, which will format the output, send it,
		// and delete the processed data
		go sendAndRemove(connection, filePath, output, fileSize, transferManager, logMan)
	}
}


// Reads data from the socket and write it to the passed in open file descriptor until end
// of file has been reached or error occurs with socket operation.
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - channel:  Channel to transmit filenames after transfer to initiate data processing
// - sendChannel:  boolean toggle that is to signify whether the file name should be sent
//				   through the channel prior to completion of data processing
// - transferBuffer:  Buffer allocated for file transfer based on file size
// - fileName:  The name of the file to be stored on disk from read socket data
// - fileSize:  The size of the to be stored on disk from read socket data
// - file:  The open file descriptor of where the data to be processed will be stored
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func socketToFileHander(connection net.Conn, channel chan []byte, sendChannel bool,
						transferBuffer []byte, fileName string, fileSize int64, file *os.File,
						logMan *kloudlogs.LoggerManager) {
	// Close file on local exit
	defer file.Close()
	// Make a local buffer for formatting the file name and size to be sent via channel
	channelBuffer := make([]byte, globals.MESSAGE_BUFFER_SIZE)

	for {
		// Read data into the buffer
		_, err := netio.ReadHandler(connection, &transferBuffer)
		// If the EOF has been reached
		if err == io.EOF {
			// If toggle specified, send file name and size through channel for processing
			if sendChannel {
				// Format the channel message like "<fileName>:<fileSize>"
				channelBuffer = []byte(fileName)
				channelBuffer = append(channelBuffer, byte(':'))
				channelBuffer = append(channelBuffer, []byte(strconv.FormatInt(fileSize, 10))...)
				channel <- channelBuffer
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
// - channel:  Channel to transmit filenames after transfer to initiate data processing
// - fileName:  The name of the file to be stored on disk from read socket data
// - fileSize:  The size of the to be stored on disk from read socket data
// - sendChannel:  boolean toggle that is to signify whether the file name should be sent
//				   through the channel prior to completion of data processing
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func handleTransfer(connection net.Conn, channel chan []byte, fileName string,
					fileSize int64, sendChannel bool, logMan *kloudlogs.LoggerManager) {
	// Set a file path with the received file name
	filePath := path.Join(StoragePath, fileName)
	//  Create buffer to optimal size based on expected file size
	transferBuffer := make([]byte, netio.GetOptimalBufferSize(fileSize))

	kloudlogs.LogMessage(logMan, "info",
						 "Filename to be tranfered:  %s | File size to be transfered:  %d", fileName, fileSize)

	// Open the file for writing
	file, err := os.Create(filePath)
	if err != nil {
		kloudlogs.LogMessage(logMan, "error", "Error creating the file %s:  %v", fileName, err)
		return
	}

	// Read data from the socket and write to the file path
	socketToFileHander(connection, channel, sendChannel, transferBuffer,
					   fileName, fileSize, file, logMan)
}


// Sends transfer message to the brain server, waits for transfer reply with file name and
// size, and proceeds to call handle transfer method.
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - channel:  Channel to transmit filenames after transfer to initiate data processing
// - buffer:  The buffer used for processing socket messaging
// - transferManager:  Manages calculating the amount of data being transferred locally
// - transferComplete:  boolean toggle that is to signify when all files have been transfered
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func processTransfer(connection net.Conn, channel chan []byte, buffer []byte,
					 transferManager *data.TransferManager, transferComplete *bool,
					 logMan *kloudlogs.LoggerManager) {
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
		// Send finished message to other Goroutine
		channel <- globals.END_TRANSFER_MARKER
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

	// Add the file size of the file to be transfered to transfer manager
	transferManager.AddTransferSize(fileSize)
	// Now that synchronized messages are complete, handle transfer in routine
	go handleTransfer(connection, channel, string(fileName), fileSize, true, logMan)
}


// Receives the file of hash to be cracked from the brain server.
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - channel:  Channel to transmit filenames after transfer to initiate data processing
// - buffer:  The buffer used for processing socket messaging
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func receiveHashFile(connection net.Conn, channel chan []byte, buffer []byte,
					 logMan *kloudlogs.LoggerManager) {
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

	// Now that synchronized messages are complete, handle transfer in routine
	go handleTransfer(connection, channel, string(fileName), fileSize, false, logMan)
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
func receiveData(connection net.Conn, channel chan []byte, waitGroup *sync.WaitGroup,
				 transferManager *data.TransferManager, logMan *kloudlogs.LoggerManager,
				 maxFileSizeInt64 int64) {
	// Decrements wait group counter upon local exit
	defer waitGroup.Done()

	transferComplete := false
	// Set the message buffer size
	buffer := make([]byte, globals.MESSAGE_BUFFER_SIZE)
	// Receive the hash file from the server
	receiveHashFile(connection, channel, buffer, logMan)

	for {
		// Get the remaining available and total disk space
		remainingSpace, total := disk.DiskCheck()
		// Get the ongoing transfer size from transfer manager
		ongoingTransferSize := transferManager.GetOngoingTransfersSize()

		kloudlogs.LogMessage(logMan, "info", "Remaining space left:  %d | Total space left:  %d",
							 remainingSpace, total)

		// If the remaining space minus the ongoing file transfers
		// is greater than or equal to the max file size
		if (remainingSpace - ongoingTransferSize) >= maxFileSizeInt64 {
			// Process the transfer of a file and return file size for the next
			processTransfer(connection, channel, buffer, transferManager, &transferComplete, logMan)
			// If the transfer is complete exit the data receiving loop
			if transferComplete {
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
func handleConnection(connection net.Conn, logMan *kloudlogs.LoggerManager, maxFileSizeInt64 int64) {
	// Initialize a transfer mananager used to track the size of active file transfers
	transferManager := data.TransferManager{}

	// Create a channel for the goroutines to communicate
	channel := make(chan []byte)
	// Establish a wait group
	var waitGroup sync.WaitGroup
	// Add two goroutines to the wait group
	waitGroup.Add(2)

	// Start the goroutine to write data to the file
	go receiveData(connection, channel, &waitGroup, &transferManager, logMan, maxFileSizeInt64)
	// Start the goroutine to process the file
	go processData(connection, channel, &waitGroup, &transferManager, logMan)

	// Wait for both goroutines to finish
	waitGroup.Wait()
}


// Take the IP address & port argument and establish a connection to
// remote brain server, then pass the connection to Goroutine handler.
//
// Parameters:
// - ipAddr:  The ip address of the remote brain server
// - port:  The TCP port to connect to on remote brain server
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
// - maxFileSize:  The maximum allowed size for a file to be transferred
//
func connectRemote(ipAddr string, port int, logMan *kloudlogs.LoggerManager, maxFileSizeInt64 int64) {
	// Define the address of the server to connect to
	serverAddress := fmt.Sprintf("%s:%s", ipAddr, port)

	// Make a connection to the remote brain server
	connection, err := net.Dial("tcp", serverAddress)
	if err != nil {
		kloudlogs.LogMessage(logMan, "fatal", "Error connecting to remote server:  %v", err)
		return
	}

	kloudlogs.LogMessage(logMan, "info", "Connected remote server at %s on port %d", ipAddr, port)

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
	// Parse the command line flags
	flag.Parse()

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
	connectRemote(ipAddr, port, logMan, maxFileSizeInt64)
}
