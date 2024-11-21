package main

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ngimb64/Kloud-Kraken/internal/config"
	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
)

// Package level variables
var CurrentConnections int32	// Tracks current active connections


func handleTransfer(connection net.Conn, transferComplete *bool, buffer []byte,
				    appConfig *config.AppConfig) {
	// Select the next avaible file in the load dir from YAML data
	filePath, err := disk.SelectFile(appConfig.LocalConfig.LoadDir)
	// If there was an error selecting the next avaible file
	if err != nil {
		fmt.Println("Error selecting the next avaible file for transfer:", err)
	}
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
		// If an error occured reading from the socket
		if err != nil {
			fmt.Print("[*] Error occured reading data from socket")
			return
		}

		// If the read data contains transfer request message
		if bytes.Contains(buffer, globals.TRANSFER_REQUEST_MARKER) {
			// Call method to handle file transfer based
			handleTransfer(connection, &transferComplete, buffer, appConfig)

			// If the transfer is complete, exit socket loop
			if transferComplete {
				break
			}
		}

		// Reset buffer to smallest size for message processing after transfer
		buffer = make([]byte, 512)
		// Sleep to avoid excessive polling during idle network activity
		time.Sleep(5 * time.Second)
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
	// If there was an starting TCP listener
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
	// If there was an error loading the YAML config file
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	startServer(appConfig)
}
