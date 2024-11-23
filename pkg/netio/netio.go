package netio

import (
	"fmt"
	"net"
)

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
