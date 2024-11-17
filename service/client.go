package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"strconv"
	"sync"
)

// Package level variables
var STORAGE_PATH = "/tmp"
var COLON_DELIMITER = []byte(":")
var START_TRANSFER_PREFIX = []byte("<START_TRANSFER:")
var START_TRANSFER_SUFFIX = []byte(">")
var END_TRANSFER_MARKER = []byte("<END_TRANSFER>")


// Reads data from channel connected to reader Goroutine. ADD MORE
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - channel:  Channel to transmit filenames after transfer to initiate data processing
// - waitGroup:  Used to synchronize the Goroutines running
func processData(connection net.Conn, channel chan []byte, waitGroup *sync.WaitGroup) {
	// Decrements the wait group counter upon function exit
	defer waitGroup.Done()

	for {
		// Get the filename of the data to process from channel
		fileName := <-channel
		// Format the filename in to the path where data is stored
		filePath := path.Join(STORAGE_PATH, string(fileName))

		// If the channel contains end transfer message
		if bytes.Contains(fileName, END_TRANSFER_MARKER) {
			break
		}

		// Execute command as background process
		cmd := exec.Command("command", "-flag", filePath)
		// Save the command stdout and stderr output
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Error executing command: %v\n", err)
			return
		}

		// Send the processing result
		_, err = connection.Write(output)
		// If there was an error sending the result to brain
		if err != nil {
			fmt.Printf("Error sending command result: %v\n", err)
			return
		}
	}
}


// Adjust buffer to optimal size based on file size to be received.
//
// Parameters:
// - fileSize:  The size of the file to be received
//
// Returns:
// - An optimal integer buffer size
func getOptimalBufferSize(fileSize int) int {
	switch {
	// If the file is less than or equal to 1 MB
	case fileSize <= 1 * 1024 * 1024:
		// 512 byte buffer
		return 512
	// If the file is less than or equal to 100 MB
	case fileSize <= 100 * 1024 * 1024:
		// 8 KB buffer
		return 8 * 1024
	// If the file is greater than 100 MB
	default:
		// 1 MB buffer
		return 1024 * 1024
	}
}


// Parse out the file name and size from the delimiter
// sent from remote brain server.
//
// Parameters:
// - readData:  The data read from socket buffer to be parsed.
//
// Returns:
// - The byte slice with the file name
// - A integer file size
// - Either nil on success or a string error message on failure
func getFileInfo(readData []byte) ([]byte, int, any) {
	// Trim the delimiters around the file info
	readData = bytes.TrimPrefix(readData, START_TRANSFER_PREFIX)
	readData = bytes.TrimSuffix(readData, START_TRANSFER_SUFFIX)
	// Split the string by the colon delimiter
	dataBits := bytes.Split(readData, COLON_DELIMITER)
	// Extract the filename and size from bits
	fileName := dataBits[0]
	fileSizeStr := dataBits[1]

	// Convert the size string to an integr
	fileSize, err := strconv.Atoi(string(fileSizeStr))
	// If the string integer failed to convert back to its native type
	if err != nil {
		fmt.Println("Error converting size string to int:", err)
		return fileName, fileSize, "Error occured during file size coversion"
	}

	return fileName, fileSize, nil
}


// Concurrently reads data from TCP socket connection until entire file has been
// transferred. Afterwards the file name is passed through a channel to the process
// data Goroutine to load the file into data processing.
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - channel:  Channel to transmit filenames after transfer to initiate data processing
// - waitGroup:  Used to synchronize the Goroutines running
func receiveData(connection net.Conn, channel chan []byte, waitGroup *sync.WaitGroup) {
	// Decrements the wait group counter upon function exit
	defer waitGroup.Done()

	var file *os.File
	var fileName []byte
	// Set the initial buffer size
	buffer := make([]byte, 512)

	// Read data from the connection in chunks and write to the file
	for {
		// Read data into the buffer
		bytesRead, err := connection.Read(buffer)
		// If the EOF has been reached
		if err == io.EOF {
			// Close the file descriptor
			file.Close()
			// Send the file name through a channel to process it
			channel <- fileName
		} else if err != nil {
			// Otherwise print error
			fmt.Println("Error reading data from server:", err)
			return
		}

		// If the brain has completed transferring all data
		if bytes.Contains(buffer, END_TRANSFER_MARKER) {
			// Send finished message to other Goroutine
			channel <- END_TRANSFER_MARKER
			break
		}

		// If the read data starts with special delimiter and ends with a closed bracket
		if bytes.HasPrefix(buffer, START_TRANSFER_PREFIX)&&
		bytes.HasSuffix(buffer, START_TRANSFER_SUFFIX) {
			// Extract the file name and size from the initial transfer message
			fileName, fileSize, err := getFileInfo(buffer)
			// If there was an error converting the file size to int
			if err != nil {
				fmt.Println(err)
				return
			}

			// Set a file path with the received file name
			filePath := path.Join("/tmp", string(fileName))
			// Reset the buffer to optimal size based on expected file size
			buffer = make([]byte, getOptimalBufferSize(fileSize))

			// Open the file for writing
			file, err := os.Create(filePath)
			// If the file failed to open
			if err != nil {
				fmt.Println("Error creating file:", err)
				return
			}

			continue
		}

		// Write the data to the file
		_, err = file.Write(buffer[:bytesRead])
		// If there was an error writig data to the file
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return
		}
	}
}


// Handle the TCP connection between Goroutine with a channel
// connecting routines to pass messages to signal data to process.
//
// Parameters:
// - connection:  The TCP socket connection utilized for transferring data
func handleConnection(connection net.Conn) {
	defer connection.Close()

	// Create a channel for the goroutines to communicate
	channel := make(chan []byte)

	// Establish a wait group
	var waitGroup sync.WaitGroup
	// Add two goroutines to the wait group
	waitGroup.Add(2)

	// Start the goroutine to write data to the file
	go receiveData(connection, channel, &waitGroup)
	// Start the goroutine to process the file (this can start immediately)
	go processData(connection, channel, &waitGroup)

	// Wait for both goroutines to finish
	waitGroup.Wait()
}


// Take the IP address & port argument and establish a connection to
// remote brain server, then pass the connection to Goroutine handler.
//
// Parameters:
// - address:  The IP address and port formatted like "<ip_address>:<port>"
func connectRemote(address string) {
	connection, err := net.Dial("tcp", address)
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		return
	}
	defer connection.Close()

	handleConnection(connection)
}


func main() {
	var ipAddr string
	var port string

	// Define command line flags with default values and descriptions
	flag.StringVar(&ipAddr, "ipAddr", "localhost", "IP address of brain server to connect to")
	flag.StringVar(&port, "port", "6969", "TCP port to connect to")
	// Parse the command line flags
	flag.Parse()

	// Define the address of the server to connect to
	serverAddress := fmt.Sprintf("%s:%s", ipAddr, port)
	// Connect to remote server to begin receiving data for processing
	connectRemote(serverAddress)
}
