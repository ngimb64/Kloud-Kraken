package main

// Built-in packages
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
const StoragePath = "/tmp"


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
			break
		}

		// Register a command with file path gathered from channel
		cmd := exec.Command("command", "-flag", filePath)
		// Execute and save the command stdout and stderr output
		output, err := cmd.CombinedOutput()
		// If there was an error exe
		if err != nil {
			fmt.Printf("Error executing command: %v\n", err)
			return
		}

		// TODO:  add code to format the command output to optimal format

		// Send the processing result
		_, err = connection.Write(output)
		// If there was an error sending the result to brain
		if err != nil {
			fmt.Printf("Error sending command result: %v\n", err)
			return
		}

		// TODO: add code to delete the file after processing is complete
	}
}


func processTransfer(connection net.Conn, channel chan []byte, buffer []byte,
					 transferComplete *bool) {
	var fileName []byte
	var file *os.File

	// Send the transfer request message to initiate file transfer
	_, err := connection.Write(globals.TRANSFER_REQUEST_MARKER)
	// If there was an error sending the transfer request
	if err != nil {
		println("Error sending the transfer request to brain server")
		return
	}

	// Wait for the brain server to send the start transfer message
	_, err = connection.Read(buffer)
	// If there was an error reading data from the socket
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

	// If the read data starts with special delimiter and ends with a closed bracket
	if bytes.HasPrefix(buffer, globals.START_TRANSFER_PREFIX)&&
	bytes.HasSuffix(buffer, globals.START_TRANSFER_SUFFIX) {
		// Extract the file name and size from the initial transfer message
		fileName, fileSize, err := disk.GetFileInfo(buffer)
		// If there was an error converting the file size to int
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Set a file path with the received file name
		filePath := path.Join(StoragePath, string(fileName))
		// Reset the buffer to optimal size based on expected file size
		buffer = make([]byte, netio.GetOptimalBufferSize(fileSize))

		// Open the file for writing
		file, err := os.Create(filePath)
		// If the file failed to open
		if err != nil {
			fmt.Println("Error creating file:", err)
			return
		}

		// Close file when the function exits
		defer file.Close()
	// If unexpected behavior occurred
	} else {
		fmt.Println("[*] Unusual behavior detected with processTransfer method")
		return
	}

	for {
		// Read data into the buffer
		bytesRead, err := connection.Read(buffer)
		// If the EOF has been reached
		if err == io.EOF {
			// Send the file name through a channel to process it
			channel <- fileName
			break
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
				 maxFileSizeInt uint64) {
	// Decrements the wait group counter upon local exit
	defer waitGroup.Done()

	transferComplete := false
	// Set the initial buffer size
	buffer := make([]byte, 512)

	// Read data from the connection in chunks and write to the file
	for {
		// Get the remaining available disk space
		remainingSpace := disk.DiskCheck()

		// If there is enough disk space to store the max file size
		if remainingSpace >= maxFileSizeInt {
			// Call function to process the transfer of a file
			processTransfer(connection, channel, buffer, &transferComplete)
		}

		// If the transfer is complete exit the data receiving loop
		if transferComplete {
			break
		}

		// Reset buffer to smallest size for message processing after transfer
		buffer = make([]byte, 512)
		// Sleep to avoid excessive syscalls in checking disk size
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
func handleConnection(connection net.Conn, maxFileSizeInt uint64) {
	// Create a channel for the goroutines to communicate
	channel := make(chan []byte)

	// Establish a wait group
	var waitGroup sync.WaitGroup
	// Add two goroutines to the wait group
	waitGroup.Add(2)

	// Start the goroutine to write data to the file
	go receiveData(connection, channel, &waitGroup, maxFileSizeInt)
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
func connectRemote(ipAddr string, port int, maxFileSizeInt uint64) {
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

	handleConnection(connection, maxFileSizeInt)
}


func main() {
	var ipAddr string
	var port int
	var maxFileSizeInt uint64

	// Define command line flags with default values and descriptions
	flag.StringVar(&ipAddr, "ipAddr", "localhost",
				   "IP address of brain server to connect to")
	flag.IntVar(&port, "port", 6969, "TCP port to connect to on brain server")
	flag.Uint64Var(&maxFileSizeInt, "maxFileSizeInt", 0,
				  "The max size for file to be transmitted at once")
	// Parse the command line flags
	flag.Parse()

	// Connect to remote server to begin receiving data for processing
	connectRemote(ipAddr, port, maxFileSizeInt)
}
