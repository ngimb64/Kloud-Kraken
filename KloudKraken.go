package main

import (
	"fmt"
	"net"
	"os"
	"sync"
)


// Simulate sending a file to a client for processing
func sendFileToClient(clientAddress string, chunk []byte, wg *sync.WaitGroup) {
	defer wg.Done()

	// Connect to the client server
	conn, err := net.Dial("tcp", clientAddress)
	if err != nil {
		fmt.Println("Error connecting to client:", err)
		return
	}
	defer conn.Close()

	// Send the chunk of data
	_, err = conn.Write(chunk)
	if err != nil {
		fmt.Println("Error sending file to client:", err)
		return
	}

	// Wait for the processed result from the client (could be the same chunk or transformed data)
	processedData := make([]byte, len(chunk))
	_, err = conn.Read(processedData)
	if err != nil {
		fmt.Println("Error receiving processed data from client:", err)
		return
	}

	// Do something with the processed data (e.g., store it)
	fmt.Println("Received processed data from client:", processedData)
}


// Distribute chunks to multiple clients concurrently
func distributeChunksToClients(chunks [][]byte, clientAddresses []string) {
	var wg sync.WaitGroup

	// Assume we distribute one chunk to each client
	for i, chunk := range chunks {
		clientAddress := clientAddresses[i%len(clientAddresses)] // Round robin if more chunks than clients
		wg.Add(1)
		go sendFileToClient(clientAddress, chunk, &wg)
	}

	// Wait for all goroutines to finish
	wg.Wait()
}


func main() {
	// Example chunks (these could come from the chunking process you implemented)
	var chunks [][]byte
	for i := 0; i < 5; i++ {
		chunks = append(chunks, []byte(fmt.Sprintf("This is chunk %d", i+1)))
	}

	// Example client addresses (these should be real client IPs or hostnames)
	clientAddresses := []string{"localhost:8081", "localhost:8082"}

	// Distribute the chunks to the clients
	distributeChunksToClients(chunks, clientAddresses)
}
