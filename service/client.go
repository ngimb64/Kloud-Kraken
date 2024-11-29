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


func sendAndRemove(connection net.Conn, filePath string, output []byte,
				   transferManager *data.TransferManager, logMan *kloudlogs.LoggerManager) {
	// TODO:  add code to format the command output to optimal format

	// Send the processing result
	_, err := netio.WriteHandler(connection, &output)
	if err != nil {
		fmt.Printf("Error sending command result: %v\n", err)
		return
	}

	// Get the file size for transfer manager subtraction after removal
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		println("Error occurred getting file information:", err)
		os.Exit(1)
	}

	// Delete the processed file
	os.Remove(filePath)
	// Remove the file size from transfer manager after deletion
	transferManager.RemoveTransferSize(fileInfo.Size())
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
		fileName := <-channel
		// Format the filename in to the path where data is stored
		filePath := path.Join(StoragePath, string(fileName))

		// If the channel contains end transfer message
		if bytes.Contains(fileName, globals.END_TRANSFER_MARKER) {
			// Sleep a bit to ensure all processing results have been sent
			time.Sleep(10 * time.Second)

			// Send the processing complete message
			_, err := netio.WriteHandler(connection, &globals.PROCESSING_COMPLETE)
			if err != nil {
				fmt.Println("Error sending processing complete message:", err)
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
			fmt.Printf("Error executing command: %v\n", err)
			return
		}

		// In a separate goroutine, format the output, send it,
		// and delete the processed data
		go sendAndRemove(connection, filePath, output, transferManager, logMan)
	}
}


func socketToFileHander(connection net.Conn, channel chan []byte, sendChannel bool,
						transferBuffer []byte, file *os.File, logMan *kloudlogs.LoggerManager) {
	// Close file on local exit
	defer file.Close()

	for {
		// Read data into the buffer
		_, err := netio.ReadHandler(connection, &transferBuffer)
		// If the EOF has been reached
		if err == io.EOF {
			// If specified with toggle, send file name through
			// the channel for processing
			if sendChannel {
				channel <- transferBuffer
			}
			break
		}

		// Write the data to the file
		_, err = file.Write(transferBuffer)
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return
		}
	}
}


func handleTransfer(connection net.Conn, channel chan []byte, fileName string,
					fileSize int64, sendChannel bool, logMan *kloudlogs.LoggerManager) {
	// Set a file path with the received file name
	filePath := path.Join(StoragePath, fileName)
	//  Create buffer to optimal size based on expected file size
	transferBuffer := make([]byte, netio.GetOptimalBufferSize(fileSize))

	// Open the file for writing
	file, err := os.Create(filePath)
	if err != nil {
		fmt.Println("Error creating file:", err)
		return
	}

	// Read data from the socket and write to the file path
	socketToFileHander(connection, channel, sendChannel, transferBuffer, file, logMan)
}


func processTransfer(connection net.Conn, channel chan []byte, buffer []byte,
					 transferManager *data.TransferManager, transferComplete *bool,
					 logMan *kloudlogs.LoggerManager) {
	// Send the transfer request message to initiate file transfer
	_, err := netio.WriteHandler(connection, &globals.TRANSFER_REQUEST_MARKER)
	if err != nil {
		println("Error sending the transfer request to brain server")
		return
	}

	// Wait for the brain server to send the start transfer message
	_, err = netio.ReadHandler(connection, &buffer)
	if err != nil {
		fmt.Println("Error start transfer message from server:", err)
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
		fmt.Println("Unusual behavior detected with processTransfer method")
		return
	}

	// Extract the file name and size from the initial transfer message
	fileName, fileSize, err := netio.GetFileInfo(buffer, globals.START_TRANSFER_PREFIX,
										   		 globals.START_TRANSFER_SUFFIX)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Add the file size of the file to be transfered to transfer manager
	transferManager.AddTransferSize(fileSize)

	// Now that synchronized messages are complete, handle transfer in routine
	go handleTransfer(connection, channel, string(fileName), fileSize, true, logMan)
}


func receiveHashFile(connection net.Conn, channel chan []byte, buffer []byte,
					 logMan *kloudlogs.LoggerManager) {
	// Wait for the brain server to send the start transfer message
	_, err := netio.ReadHandler(connection, &buffer)
	if err != nil {
		fmt.Println("Error start transfer message from server:", err)
		os.Exit(1)
	}

	// If the read data does not start with special delimiter or end with closed bracket
	if !bytes.HasPrefix(buffer, globals.HASHES_TRANSFER_PREFIX) ||
	!bytes.HasSuffix(buffer, globals.HASHES_TRANSFER_SUFFIX) {
		fmt.Println("Error: unusual behavior detected with receiveHashFile method")
		os.Exit(1)
	}

	// Extract the file name and size from the initial transfer message
	fileName, fileSize, err := netio.GetFileInfo(buffer, globals.HASHES_TRANSFER_PREFIX,
												 globals.HASHES_TRANSFER_SUFFIX)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
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

	// Read data from the connection in chunks and write to the file
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
func connectRemote(ipAddr string, port int, maxFileSizeInt64 int64, logMan *kloudlogs.LoggerManager) {
	// Define the address of the server to connect to
	serverAddress := fmt.Sprintf("%s:%s", ipAddr, port)

	// Make a connection to the remote brain server
	connection, err := net.Dial("tcp", serverAddress)
	if err != nil {
		kloudlogs.LogMessage(logMan, "error", "Error connecting to remote server:  %v", err)
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
	flag.StringVar(&logPath, "logPath", "KloudKraken.log", " File path to the log file")
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
	connectRemote(ipAddr, port, maxFileSizeInt64, logMan)
}
