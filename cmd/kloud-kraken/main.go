package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/ngimb64/Kloud-Kraken/internal/config"
	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/internal/validate"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"github.com/ngimb64/Kloud-Kraken/pkg/display"
	"github.com/ngimb64/Kloud-Kraken/pkg/kloudlogs"
	"github.com/ngimb64/Kloud-Kraken/pkg/netio"
)

// Package level variables
var CurrentConnections atomic.Int32		// Tracks current active connections


// Handle reading data from the passed in file descriptor and write to the socket to client.
//
// Params:
// - connection:  The active TCP socket connection to transmit data
// - transferBuffer:  The buffer used to store file data that is transferred
// - file:  A pointer to the open file descriptor
// - logMan:  The kloudlogs logger manager for local logging
//
func fileToSocketHandler(connection net.Conn, transferBuffer []byte, file *os.File,
						 logMan *kloudlogs.LoggerManager) {
	// Close the file on local exit
	defer file.Close()

	for {
		// Read buffer size from file
		_, err := file.Read(transferBuffer)
		if err != nil {
			// If the error was not the end of file
			if err != io.EOF {
				kloudlogs.LogMessage(logMan, "error", "Error reading file:  %v", err)
			}
			break
		}

		// Write the read bytes to the client
		_, err = netio.WriteHandler(connection, &transferBuffer)
		if err != nil {
			kloudlogs.LogMessage(logMan, "error", "Error sending data in socket:  %v", err)
			break
		}
	}
}


func transferFile(connection net.Conn, filePath string, fileSize int64,
				  logMan *kloudlogs.LoggerManager) {
	// Create buffer to optimal size based on expected file size
	transferBuffer := make([]byte, netio.GetOptimalBufferSize(fileSize))

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		kloudlogs.LogMessage(logMan, "error", "Error opening the file to be transfered:  %v", err)
		return
	}

	// Read the file chunk by chunk and send to client
	fileToSocketHandler(connection, transferBuffer, file, logMan)

	// Delete the transfered file
	err = os.Remove(filePath)
	if err != nil {
		kloudlogs.LogMessage(logMan, "error", "Error deleting the file:  %v", err)
		return
	}
}


func handleTransfer(connection net.Conn, buffer *[]byte, appConfig *config.AppConfig,
					logMan *kloudlogs.LoggerManager) {
	// Select the next avaible file in the load dir from YAML data
	filePath, fileSize, err := disk.SelectFile(appConfig.LocalConfig.LoadDir,
											   appConfig.ClientConfig.MaxFileSizeInt64)
	if err != nil {
		kloudlogs.LogMessage(logMan, "error", "Error selecting the next available file to transfer:  %v", err)
		return
	}

	// If there are no more files available to be transfered
	if filePath == "" {
		// Send the end transfer message then exit function
		_, err = netio.WriteHandler(connection, &globals.END_TRANSFER_MARKER)
		if err != nil {
			kloudlogs.LogMessage(logMan, "error", "Error sending the end transfer message:  %v", err)
		}
		return
	}

	// Clear the buffer before building transfer reply
	*buffer = (*buffer)[:0]
	// Append the transfer reply piece by piece in buffer
	*buffer = append(globals.START_TRANSFER_PREFIX, []byte(filePath)...)
	*buffer = append(*buffer, globals.COLON_DELIMITER...)
	*buffer = append(*buffer, []byte(strconv.FormatInt(fileSize, 10))...)
	*buffer = append(*buffer, globals.START_TRANSFER_SUFFIX...)

	// Send the transfer reply with file name and size
	_, err = netio.WriteHandler(connection, buffer)
	if err != nil {
		kloudlogs.LogMessage(logMan, "error", "Error sending the transfer reply:  %v", err)
		return
	}

	// Get the IP address from the ip:port host address
	ipAddr, _, err := netio.GetIpPort(connection)
	if err != nil {
		kloudlogs.LogMessage(logMan, "error", "Error occcurred spliting host address to get IP/port:  %v", err)
		return
	}

	var port int32
	// Receive bytes of port of client port to connect to for file transfer
	err = binary.Read(connection, binary.BigEndian, &port)
	if err != nil {
		kloudlogs.LogMessage(logMan, "error", "Error receiving client listener port:  %v", err)
		return
	}

	// Format remote address with IP and port
	remoteAddr := fmt.Sprintf("%s:%d", ipAddr, port)

	// Make a connection to the remote brain server
	transferConn, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		kloudlogs.LogMessage(logMan, "fatal", "Error connecting to remote client for transfer:  %v", err)
		return
	}

	kloudlogs.LogMessage(logMan, "info", "Connected remote client at %s on port %d", ipAddr, port)

	go transferFile(transferConn, filePath, fileSize, logMan)
}


func uploadHashFile(connection net.Conn, buffer *[]byte, appConfig *config.AppConfig,
					logMan *kloudlogs.LoggerManager) {
	filePath := appConfig.LocalConfig.HashFilePath

	// Get the hash file size based on saved path in config
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		kloudlogs.LogMessage(logMan, "fatal", "Error getting file size:  %v", err)
	}

	fileSize := fileInfo.Size()

	// Clear the buffer before building transfer reply
	*buffer = (*buffer)[:0]
	// Append the hash file transfer request piece by piece in buffer
	*buffer = append(globals.HASHES_TRANSFER_PREFIX, []byte(filePath)...)
	*buffer = append(*buffer, globals.COLON_DELIMITER...)
	*buffer = append(*buffer, []byte(strconv.FormatInt(fileSize, 10))...)
	*buffer = append(*buffer, globals.HASHES_TRANSFER_SUFFIX...)

	// Send the hash file transfer request with file name and size
	_, err = netio.WriteHandler(connection, buffer)
	if err != nil {
		kloudlogs.LogMessage(logMan, "fatal", "Error sending the hash file name and size:  %v", err)
	}

	// Transfer the hash file to client
	transferFile(connection, filePath, fileSize, logMan)
}


func handleConnection(connection net.Conn, waitGroup *sync.WaitGroup,
					  appConfig *config.AppConfig, logMan *kloudlogs.LoggerManager) {
	// Close connection and decrement waitGroup counter on local exit
	defer connection.Close()
	defer waitGroup.Done()

	// Set message buffer size
	buffer := make([]byte, globals.MESSAGE_BUFFER_SIZE)
	// Upload the hash file to connection client
	uploadHashFile(connection, &buffer, appConfig, logMan)

	for {
		// Read data from connected client
		_, err := netio.ReadHandler(connection, &buffer)
		if err != nil {
			kloudlogs.LogMessage(logMan, "error", "Error occurred reading data from socket:  %v", err)
			return
		}

		// If the read data contains the processing complete message
		if bytes.Contains(buffer, globals.PROCESSING_COMPLETE) {
			break
		}

		// TODO:  add logic to handle report data in Goroutine if detected

		// If the read data contains transfer request message
		if bytes.Contains(buffer, globals.TRANSFER_REQUEST_MARKER) {
			// Call method to handle file transfer based
			handleTransfer(connection, &buffer, appConfig, logMan)
		}
	}

	// Decrement the active connection count
	CurrentConnections.Add(-1)

	kloudlogs.LogMessage(logMan, "info", "Connection handled, active connections:  %v",
						 CurrentConnections.Load())
}


func startServer(appConfig *config.AppConfig, logMan *kloudlogs.LoggerManager) {
	// Format listener port with parsed YAML data
	listenerPort := fmt.Sprint(":%s", appConfig.LocalConfig.ListenerPort)
	// Start listening on specified port
	listener, err := net.Listen("tcp", listenerPort)
	if err != nil {
		kloudlogs.LogMessage(logMan, "fatal", "Error starting server:  %v", err)
	}

	// Close listener on local exit
	defer listener.Close()
	// Establish wait group for Goroutine synchronization
	var waitGroup sync.WaitGroup

	kloudlogs.LogMessage(logMan, "info", "Server started, waiting for connections ..")

	for {
		// If current number of connection is greater than or equal to number of instances
		if CurrentConnections.Load() >= int32(appConfig.LocalConfig.NumberInstances) {
			kloudlogs.LogMessage(logMan, "info", "All remote clients are connected")
			break
		}

		// Wait for an incoming connection
		connection, err := listener.Accept()
		if err != nil {
			kloudlogs.LogMessage(logMan, "error", "Error accepting client connection:  %v", err)
			continue
		}

		// Increment the active connection count
		CurrentConnections.Add(1)

		kloudlogs.LogMessage(logMan, "info", "Connection accepted, active connections:  %d",
							 CurrentConnections.Load())

		// Increment wait group and handle connection in separate Goroutine
		waitGroup.Add(1)
		go handleConnection(connection, &waitGroup, appConfig, logMan)
	}

	// Wait for all active Goroutines to finish before shutting down the server
	waitGroup.Wait()

	kloudlogs.LogMessage(logMan, "info", "All connections handled .. server shutting down")
}


func parseArgs() *config.AppConfig {
	var configFilePath string

	// If the config file path was not passed in
	if len(os.Args) < 2 {
		// Prompt the user until proper path is passed in
		validate.ValidateConfigPath(&configFilePath)
	// If the config file path arg was passed in
	} else {
		// Set the provided arg as the config file path
		configFilePath = os.Args[1]

		// Check to see if the input path exists and is a file or dir
		exists, isDir, err := disk.PathExists(configFilePath)
		if err != nil {
			log.Fatal("Error checking config file path existence: ", err)
		}

		// If the path does not exist or is a dir or is not YAML file
		if !exists || isDir || !strings.HasSuffix(configFilePath, ".yml") {
			fmt.Println("Provided YAML config file path invalid: ", configFilePath)
			// Sleep for a few seconds and clear screen
			display.ClearScreen(3)
			// Prompt the user until proper path is passed in
			validate.ValidateConfigPath(&configFilePath)
		}
	}

	// Load the configuration from the YAML file
	return config.LoadConfig(configFilePath)
}


func main() {
	var awsConfig aws.Config

	// Handle selecting the YAML file if no arg provided
	// and load YAML data into struct configuration class
	appConfig := parseArgs()

	// Get the AWS access and secret key environment variables
	awsAccessKey := os.Getenv("AWS_ACCESS_KEY")
	awsSecretKey := os.Getenv("AWS_SECRET_KEY")

	// If AWS access and secret key are present
	if awsAccessKey == "" || awsSecretKey == "" {
		log.Fatal("Missing either the access or the secret key for AWS")
	}

	// Set AWS config for CloudWatch logging
	awsConfig = aws.Config {
		Region:      appConfig.LocalConfig.Region,
		Credentials: credentials.NewStaticCredentialsProvider(awsAccessKey, awsSecretKey, ""),
	}

	// Initialize the LoggerManager based on the flags
	logMan, err := kloudlogs.NewLoggerManager("local", appConfig.LocalConfig.LogPath, awsConfig)
	if err != nil {
		log.Fatalf("Error initializing logger manager:  %v", err)
	}

	// TODO:  After local testing add function calls for setting up AWS, packer AMI build (if not exist already),
	//		  and spawning EC2 with user data executing service with params passed in

	startServer(appConfig, logMan)
}
