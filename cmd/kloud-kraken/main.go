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
var CurrentConnections int32	// Tracks current active connections


func handleTransfer(connection net.Conn, transferComplete *bool, buffer []byte,
				    appConfig *config.AppConfig) {
	// Select the next avaible file in the load dir from YAML data
	filePath, fileSize, err := disk.SelectFile(appConfig.LocalConfig.LoadDir)
	if err != nil {
		fmt.Println("Error selecting the next avaible file for transfer:", err)
	}

	// If there are no more files available to be transfered
	if filePath == "" {
		// Send the end transfer message then exit function
		_, err = connection.Write(globals.END_TRANSFER_MARKER)
		if err != nil {
			fmt.Println("Error sending transfer message:", err)
		}

		return
	}

	// Initial bytes buffer for message
	var byteBuffer bytes.Buffer
	// Format the transfer reply message
	byteBuffer.Write(globals.START_TRANSFER_PREFIX)
	byteBuffer.Write([]byte(filePath))
	byteBuffer.Write(globals.COLON_DELIMITER)
	byteBuffer.Write([]byte(strconv.FormatInt(fileSize, 10)))
	byteBuffer.Write(globals.START_TRANSFER_SUFFIX)

	// Send the transfer request reply with file name and size
	_, err = connection.Write(byteBuffer.Bytes())
	if err != nil {
		fmt.Println("Error sending the transfer reply:", err)
		return
	}

	// Reset the buffer to optimal size based on expected file size
	buffer = make([]byte, netio.GetOptimalBufferSize(fileSize))

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("Error opening the file to be transferred:", err)
		return
	}

	// Close the file on local exit
	defer file.Close()

	for {
		// Read buffer size from file
		bytesRead, err := file.Read(buffer)
		if err != nil {
			// If the error was not EOF
			if err != io.EOF {
				fmt.Println("Error reading file:", err)
			}
			break
		}

		// Write the read bytes to the client
		_, writeErr := connection.Write(buffer[:bytesRead])
		if writeErr != nil {
			fmt.Println("Error sending data:", writeErr)
			break
		}
	}

	// Delete file in separate routine after transfer
	go func (path string)  {
		os.Remove(path)
	}(filePath)
}


func handleConnection(connection net.Conn, waitGroup *sync.WaitGroup,
					  appConfig *config.AppConfig) {
	// Close connection and decrement waitGroup counter on local exit
	defer connection.Close()
	defer waitGroup.Done()

	transferComplete := false
	//Set initial buffer size
	buffer := make([]byte, 512)

	for {
		// Read data from connected client
		_, err := connection.Read(buffer)
		if err != nil {
			fmt.Print("[*] Error occured reading data from socket")
			return
		}

		// TODO: add logic to receive and handle report data Goroutine

		// If the read data contains transfer request message
		if bytes.Contains(buffer, globals.TRANSFER_REQUEST_MARKER) {
			// Call method to handle file transfer based
			handleTransfer(connection, &transferComplete, buffer, appConfig)

			// If all available files have beed transfered, exit socket loop
			if transferComplete {
				break
			}
		}

		// Reset buffer to smallest size for message processing after transfer
		buffer = make([]byte, 512)
	}

	// Decrement the active connection count
	atomic.AddInt32(&CurrentConnections, -1)

	fmt.Printf("[!] Connection handled, active connections: %d\n",
			   atomic.LoadInt32(&CurrentConnections))
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
		if atomic.LoadInt32(&CurrentConnections) >= int32(appConfig.LocalConfig.MaxConnections) {
			fmt.Println("[*] Maximum number of connections reached",
						".. no more new connections")
			break
		}

		// Wait for an incoming connection
		connection, err := listener.Accept()
		if err != nil {
			log.Println("[*] Error accepting connection:", err)
			continue
		}

		// Increment the active connection count
		atomic.AddInt32(&CurrentConnections, 1)

		fmt.Printf("[+] New connection accepted, active connections: %d\n",
				   atomic.LoadInt32(&CurrentConnections))

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

	startServer(appConfig)
}
