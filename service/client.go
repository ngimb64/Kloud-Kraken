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
	"sync"
	"time"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"github.com/ngimb64/Kloud-Kraken/pkg/netio"
)

// Package level variables
const StoragePath = "/tmp"	// Path where received files are stored


func sendAndRemove(connection net.Conn, filePath string, output []byte) {
	// TODO:  add code to format the command output to optimal format

	// Send the processing result
	_, err := netio.WriteHandler(connection, &output)
	if err != nil {
		fmt.Printf("Error sending command result: %v\n", err)
		return
	}

	// Delete the processed file
	os.Remove(filePath)
}


// Reads data (filename or end transfer message) from channel connected to reader Goroutine,
// takes the received filename and passes it into command execution method for processing,
// and the result is formatted and sent back to the brain server.
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - channel:  Channel to transmit filenames after transfer to initiate data processing
// - waitGroup:  Acts as a barrier for the Goroutines running
//
func processData(connection net.Conn, channel chan []byte, waitGroup *sync.WaitGroup) {
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
		go sendAndRemove(connection, filePath, output)
	}
}


func handleTransfer(connection net.Conn, channel chan []byte, fileName string,
					fileSize int64, sendChannel bool) {
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

	// Close file when the function exits
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


func processTransfer(connection net.Conn, channel chan []byte, buffer []byte,
					 transferComplete *bool) int64 {
	// Send the transfer request message to initiate file transfer
	_, err := netio.WriteHandler(connection, &globals.TRANSFER_REQUEST_MARKER)
	if err != nil {
		println("Error sending the transfer request to brain server")
		return 0
	}

	// Wait for the brain server to send the start transfer message
	_, err = netio.ReadHandler(connection, &buffer)
	if err != nil {
		fmt.Println("Error start transfer message from server:", err)
		return 0
	}

	// If the brain has completed transferring all data
	if bytes.Contains(buffer, globals.END_TRANSFER_MARKER) {
		// Send finished message to other Goroutine
		channel <- globals.END_TRANSFER_MARKER
		*transferComplete = true
		return 0
	}

	// If the read data does not start with special delimiter or end with closed bracket
	if !bytes.HasPrefix(buffer, globals.START_TRANSFER_PREFIX) ||
	!bytes.HasSuffix(buffer, globals.START_TRANSFER_SUFFIX) {
		fmt.Println("Unusual behavior detected with processTransfer method")
		return 0
	}

	// Extract the file name and size from the initial transfer message
	fileName, fileSize, err := netio.GetFileInfo(buffer, globals.START_TRANSFER_PREFIX,
										   		 globals.START_TRANSFER_SUFFIX)
	if err != nil {
		fmt.Println(err)
		return 0
	}

	// Now that synchronized messages are complete, handle transfer in routine
	go handleTransfer(connection, channel, string(fileName), fileSize, true)

	return fileSize
}


func receiveHashFile(connection net.Conn, channel chan []byte, buffer []byte) {
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
	go handleTransfer(connection, channel, string(fileName), fileSize, false)
}


// Concurrently reads data from TCP socket connection until entire file has been
// transferred. Afterwards the file name is passed through a channel to the process
// data Goroutine to load the file into data processing.
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - channel:  Channel to transmit filenames after transfer to initiate data processing
// - waitGroup:  Used to synchronize the Goroutines running
// - appConfig:  Pointer to the program configuration struct
//
func receiveData(connection net.Conn, channel chan []byte, waitGroup *sync.WaitGroup,
				 maxFileSizeInt64 int64) {
	// Decrements the wait group counter upon local exit
	defer waitGroup.Done()

	var lastTransferSize int64 = 0
	transferComplete := false
	// Set the message buffer size
	buffer := make([]byte, globals.MESSAGE_BUFFER_SIZE)
	// Receive the hash file from the server
	receiveHashFile(connection, channel, buffer)

	// Read data from the connection in chunks and write to the file
	for {
		// Get the remaining available disk space
		remainingSpace := disk.DiskCheck()

		// If there is enough disk space for the first transfer or there is enough disk
		// space with included calculation of the file size from the last transfer
		if ((remainingSpace >= maxFileSizeInt64) && (lastTransferSize == 0)) ||
		((remainingSpace - lastTransferSize) >= maxFileSizeInt64) {
			// Process the transfer of a file and return file size for the next
			lastTransferSize = processTransfer(connection, channel, buffer, &transferComplete)
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
// - appConfig:  Pointer to the program configuration struct
//
func handleConnection(connection net.Conn, maxFileSizeInt64 int64) {
	// Create a channel for the goroutines to communicate
	channel := make(chan []byte)
	// Establish a wait group
	var waitGroup sync.WaitGroup
	// Add two goroutines to the wait group
	waitGroup.Add(2)

	// Start the goroutine to write data to the file
	go receiveData(connection, channel, &waitGroup, maxFileSizeInt64)
	// Start the goroutine to process the file
	go processData(connection, channel, &waitGroup)

	// Wait for both goroutines to finish
	waitGroup.Wait()
}


// Take the IP address & port argument and establish a connection to
// remote brain server, then pass the connection to Goroutine handler.
//
// Parameters:
// - appConfig:  Pointer to the program configuration struct
//
func connectRemote(ipAddr string, port int, maxFileSizeInt64 int64) {
	// Define the address of the server to connect to
	serverAddress := fmt.Sprintf("%s:%s", ipAddr, port)

	// Make a connection to the remote brain server
	connection, err := net.Dial("tcp", serverAddress)
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		return
	}

	// Close connection on local exit
	defer connection.Close()
	// Set up goroutines for receiving and processing data
	handleConnection(connection, maxFileSizeInt64)
}


func main() {
	var ipAddr string
	var port int
	var maxFileSizeInt64 int64

	// Define command line flags with default values and descriptions
	flag.StringVar(&ipAddr, "ipAddr", "localhost",
				   "IP address of brain server to connect to")
	flag.IntVar(&port, "port", 6969, "TCP port to connect to on brain server")
	flag.Int64Var(&maxFileSizeInt64, "maxFileSizeInt64", 0,
				  "The max size for file to be transmitted at once")
	// Parse the command line flags
	flag.Parse()

	// Connect to remote server to begin receiving data for processing
	connectRemote(ipAddr, port, maxFileSizeInt64)
}
