package main

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
)

// Package level variables
var MAX_CONNECTIONS = 5
var CURRENT_CONNECTIONS int32
var TRANSFER_REQUEST_MARKER = []byte("<TRANSFER_REQUEST>")


func handleConnection(connection net.Conn, waitGroup *sync.WaitGroup) {
	// Decrement counter and close connectionn when Goroutine completes
	defer waitGroup.Done()
	defer connection.Close()

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
		if bytes.Contains(buffer, TRANSFER_REQUEST_MARKER) {
			break
		}
	}

	// Decrement the active connection count
	atomic.AddInt32(&CURRENT_CONNECTIONS, -1)

	fmt.Printf("[!] Connection handled, active connections: %d\n",
			   atomic.LoadInt32(&CURRENT_CONNECTIONS))
}


func startServer() {
	// Start listening on specified port
	listener, err := net.Listen("tcp", ":6969")
	// If there was an starting TCP listener
	if err != nil {
		log.Fatal("Error starting server:", err)
	}

	// Close listener exiting function
	defer listener.Close()
	// Establish wait group for Goroutine synchronization
	var waitGroup sync.WaitGroup

	fmt.Println("[+] Server started, waiting for connections ..")

	for {
		// If the current number of connection is greater than or equal to the allowed max
		if atomic.LoadInt32(&CURRENT_CONNECTIONS) >= int32(MAX_CONNECTIONS) {
			fmt.Println("[*] Maximum number of connections reached .. no more new connections")
			break
		}

		// Wait for an incoming connection
		connection, err := listener.Accept()
		if err != nil {
			log.Println("[*] Error accepting connection:", err)
			continue
		}

		// Increment the active connection count
		atomic.AddInt32(&CURRENT_CONNECTIONS, 1)

		fmt.Printf("[+] New connection accepted, active connections: %d\n",
				   atomic.LoadInt32(&CURRENT_CONNECTIONS))

		// Increment wait group and handle connection in separate Goroutine
		waitGroup.Add(1)
		go handleConnection(connection, &waitGroup)
	}

	// Wait for all active Goroutines to finish before shutting down the server
	waitGroup.Wait()

	fmt.Println("[!] All connections handled .. server shutting down")
}


func main() {
	startServer()
}
