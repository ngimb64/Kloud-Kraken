package main

import (
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Package level variables
var maxConnections = 5
var currentConnections int32


// handleConnection processes each connection.
func handleConnection(conn net.Conn, wg *sync.WaitGroup) {
	// Decrement counter and close connectionn when Goroutine completes
	defer wg.Done()
	defer conn.Close()

	for {
		// Example: simulate some work with the connection (e.g., a simple echo server)
		fmt.Println("Handling connection:", conn.RemoteAddr())

		// Simulate doing something with the connection
		time.Sleep(2 * time.Second)

		// Write a response (optional)
		conn.Write([]byte("Hello from server!\n"))

		break
	}

	// Decrement the active connection count
	atomic.AddInt32(&currentConnections, -1)

	fmt.Printf("[!] Connection handled, active connections: %d\n", atomic.LoadInt32(&currentConnections))
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
	var wg sync.WaitGroup

	fmt.Println("[+] Server started, waiting for connections ..")

	for {
		// Check if we reached the max connection limit
		if atomic.LoadInt32(&currentConnections) >= int32(maxConnections) {
			fmt.Println("[*] Maximum number of connections reached. Not accepting new connections")
			break
		}

		// Wait for an incoming connection
		conn, err := listener.Accept()
		if err != nil {
			log.Println("[*] Error accepting connection:", err)
			continue
		}

		// Increment the active connection count
		atomic.AddInt32(&currentConnections, 1)

		fmt.Printf("[+] New connection accepted, active connections: %d\n", atomic.LoadInt32(&currentConnections))

		// Increment wait group and handle connection in separate Goroutine
		wg.Add(1)
		go handleConnection(conn, &wg)
	}

	// Wait for all active Goroutines to finish before shutting down the server
	wg.Wait()

	fmt.Println("[!] All connections handled .. server shutting down")
}


func main() {
	startServer()
}
