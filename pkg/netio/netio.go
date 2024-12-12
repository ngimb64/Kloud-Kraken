package netio

import (
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"strconv"

	"github.com/ngimb64/Kloud-Kraken/pkg/kloudlogs"
)


func GetAvailableListener(logMan *kloudlogs.LoggerManager) (net.Listener, int32) {
	var minPort int32 = 1001
	var maxPort int32 = 65535

	for {
		// Select a random port inside min-max range
		port := rand.Int31n(maxPort-minPort+1) + minPort

		// Attempt to establish a local listener for incoming connect
		testListener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		// If the listener was succefully established
		if err == nil {
			return testListener, port
		}

		kloudlogs.LogMessage(logMan, "info", "Port %d is not available .. attempting next port", port)
	}
}


// Parse file name/size from buffer data based on colon separator
//
// Parameters:
// - dataBuffer:  The data read from socket buffer to be parsed
//
// Returns:
// - The byte slice with the file name
// - A integer file size
// - Either nil on success or a string error message on failure
//
func GetFileInfo(dataBuffer []byte) ([]byte, int64, error) {
	// Get the position of the colon delimiter
	colonPos := bytes.IndexByte(dataBuffer, ':')
	// If the colon separator is missing
	if colonPos == -1 {
		return []byte(""), 0, fmt.Errorf("invalid message structure, colon missing")
	}

	// Extract the file path and size
	fileName := dataBuffer[:colonPos]
	fileSizeStr := string(dataBuffer[colonPos+1:])

	// Convert the size string to an 64 bit integr
	fileSize, err := strconv.ParseInt(string(fileSizeStr), 10, 64)
	if err != nil {
		return fileName, fileSize, err
	}

	return fileName, fileSize, nil
}


func GetIpPort(connection net.Conn) (string, int32, error) {
	// Get the ip:port adress of the connected client
	stringAddr := connection.RemoteAddr().String()
	if stringAddr == "" {
		return "", -1, fmt.Errorf("unable to retrieve client address from connection")
	}

	// Split the IP and port from address, saving IP in variable
	ipAddr, strPort, err := net.SplitHostPort(stringAddr)
	if err != nil {
		return "", -1, fmt.Errorf("unable to split IP and port from client address:  %v", err)
	}

	// Convert the parsed string port to integer
	port, err := strconv.Atoi(strPort)
	if err != nil {
		return "", -1, fmt.Errorf("unable to convert string address %s to port:  %v", strPort, err)
	}

	// Cast int port conversion to int32
	port32 := int32(port)

	return ipAddr, port32, nil
}


// Adjust buffer to optimal size based on file size to be received.
//
// Parameters:
// - fileSize:  The size of the file to be received
//
// Returns:
// - An optimal integer buffer size
//
func GetOptimalBufferSize(fileSize int64) int {
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


// ReadWrapper clears the buffer before reading
func ReadHandler(conn net.Conn, buffer *[]byte) (int, error) {
	// Clear the buffer to avoid leftover data
	*buffer = (*buffer)[:0]

	// Perform the read operation
	bytesRead, err := conn.Read(*buffer)
	if err != nil {
		return bytesRead, fmt.Errorf("error reading from connection: %w", err)
	}

	return bytesRead, nil
}


// WriteWrapper writes data to the connection
func WriteHandler(conn net.Conn, buffer *[]byte) (int, error) {
	// Perform the write operation
	bytesWrote, err := conn.Write(*buffer)
	if err != nil {
		return 0, fmt.Errorf("error writing to connection: %w", err)
	}

	// Clear the buffer to avoid leftover data
	*buffer = (*buffer)[:0]

	return bytesWrote, nil
}
