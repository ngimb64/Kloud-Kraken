package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/ngimb64/Kloud-Kraken/internal/config"
	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"github.com/ngimb64/Kloud-Kraken/pkg/netio"
)

// Package level variables
var CurrentConnections atomic.Int32		// Tracks current active connections


func fileToSocketHandler(connection net.Conn, transferBuffer []byte, file *os.File) {
	// Close the file on local exit
	defer file.Close()

	for {
		// Read buffer size from file
		_, err := file.Read(transferBuffer)
		if err != nil {
			// If the error was not EOF
			if err != io.EOF {
				fmt.Println("Error reading file:", err)
			}
			break
		}

		// Write the read bytes to the client
		_, err = netio.WriteHandler(connection, &transferBuffer)
		if err != nil {
			fmt.Println("Error sending data:", err)
			break
		}
	}
}


func transferFile(connection net.Conn, filePath string, fileSize int64) {
	// Create buffer to optimal size based on expected file size
	transferBuffer := make([]byte, netio.GetOptimalBufferSize(fileSize))

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("Error opening the file to be transferred:", err)
		return
	}

	// Read the file chunk by chunk and send to client
	fileToSocketHandler(connection, transferBuffer, file)

	// Delete the transfered file
	err = os.Remove(filePath)
	if err != nil {
		fmt.Println("Error deleting the file:", err)
		return
	}
}


func handleTransfer(connection net.Conn, buffer *[]byte, appConfig *config.AppConfig) {
	// Select the next avaible file in the load dir from YAML data
	filePath, fileSize, err := disk.SelectFile(appConfig.LocalConfig.LoadDir,
											   appConfig.ClientConfig.MaxFileSizeInt64)
	if err != nil {
		fmt.Println("Error selecting the next avaible file for transfer:", err)
		return
	}

	// If there are no more files available to be transfered
	if filePath == "" {
		// Send the end transfer message then exit function
		_, err = netio.WriteHandler(connection, &globals.END_TRANSFER_MARKER)
		if err != nil {
			fmt.Println("Error sending transfer message:", err)
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
		fmt.Println("Error sending the transfer reply:", err)
		return
	}

	go transferFile(connection, filePath, fileSize)
}


func uploadHashFile(connection net.Conn, buffer *[]byte, appConfig *config.AppConfig) {
	filePath := appConfig.LocalConfig.HashFilePath

	// Get the hash file size based on saved path in config
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
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
		fmt.Println("Error sending the transfer reply:", err)
		os.Exit(1)
	}

	go transferFile(connection, filePath, fileSize)
}


func handleConnection(connection net.Conn, waitGroup *sync.WaitGroup,
					  appConfig *config.AppConfig) {
	// Close connection and decrement waitGroup counter on local exit
	defer connection.Close()
	defer waitGroup.Done()

	// Set message buffer size
	buffer := make([]byte, globals.MESSAGE_BUFFER_SIZE)
	// Upload the hash file to connection client
	uploadHashFile(connection, &buffer, appConfig)

	for {
		// Read data from connected client
		_, err := netio.ReadHandler(connection, &buffer)
		if err != nil {
			fmt.Print("[*] Error occured reading data from socket")
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
			handleTransfer(connection, &buffer, appConfig)
		}
	}

	// Decrement the active connection count
	CurrentConnections.Add(-1)

	fmt.Printf("[!] Connection handled, active connections: %d\n",
			   CurrentConnections.Load())
}


func startServer(appConfig *config.AppConfig) {
	// Format listener port with parsed YAML data
	listenerPort := fmt.Sprint(":%s", appConfig.LocalConfig.ListenerPort)
	// Start listening on specified port
	listener, err := net.Listen("tcp", listenerPort)
	if err != nil {
		log.Fatal("Error starting server:", err)
	}

	// Close listener on local exit
	defer listener.Close()
	// Establish wait group for Goroutine synchronization
	var waitGroup sync.WaitGroup

	fmt.Println("[+] Server started, waiting for connections ..")

	for {
		// If the current number of connection is greater than or equal to the allowed max
		if CurrentConnections.Load() >= int32(appConfig.LocalConfig.MaxConnections) {
			fmt.Println("[!] All remote clients are connected")
			break
		}

		// Wait for an incoming connection
		connection, err := listener.Accept()
		if err != nil {
			log.Println("[*] Error accepting connection:", err)
			continue
		}

		// Increment the active connection count
		CurrentConnections.Add(1)

		fmt.Printf("[+] New connection accepted, active connections: %d\n",
				   CurrentConnections.Load())

		// Increment wait group and handle connection in separate Goroutine
		waitGroup.Add(1)
		go handleConnection(connection, &waitGroup, appConfig)
	}

	// Wait for all active Goroutines to finish before shutting down the server
	waitGroup.Wait()

	fmt.Println("[!] All connections handled .. server shutting down")
}


func main() {
	configFilePath := "config.yaml"
	// Load the configuration from the YAML file
	appConfig, err := config.LoadConfig(configFilePath)
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	// TODO:  set up a local logger to file

	startServer(appConfig)
}
