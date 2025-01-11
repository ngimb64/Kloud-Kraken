package netio

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"strconv"
	"sync"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/kloudlogs"
	"go.uber.org/zap"
)

// Handle reading data from the passed in file descriptor and write to the socket to client.
//
// Params:
// - connection:  The active TCP socket connection to transmit data
// - transferBuffer:  The buffer used to store file data that is transferred
// - file:  A pointer to the open file descriptor
// - logMan:  The kloudlogs logger manager for local logging
//
func FileToSocketHandler(connection net.Conn, transferBuffer []byte, file *os.File,
                         logMan *kloudlogs.LoggerManager) {
    // Close the file on local exit
    defer file.Close()

    for {
        // Read buffer size from file
        _, err := file.Read(transferBuffer)
        if err != nil {
            // If the error was not the end of file
            if err != io.EOF {
                kloudlogs.LogMessage(logMan, "error", "Error reading file:  %w", err)
            }
            break
        }

        // Write the read bytes to the client
        _, err = WriteHandler(connection, &transferBuffer)
        if err != nil {
            kloudlogs.LogMessage(logMan, "error", "Error sending data in socket:  %w", err)
            break
        }
    }
}


func GetAvailableListener(logMan *kloudlogs.LoggerManager) (net.Listener, int32) {
    var minPort int32 = 1001
    var maxPort int32 = 65535

    for {
        // Select a random port inside min-max range
        port := rand.Int31n(maxPort - minPort+1) + minPort

        // Attempt to establish a local listener for incoming connect
        testListener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
        // If the listener was succefully established
        if err == nil {
            return testListener, port
        }

        kloudlogs.LogMessage(logMan, "info", "Unable to obtain listener port:  %w", err,
                             zap.Int32("listener port", port))
    }
}


// Parse file name/size from buffer data based on colon separator
//
// Parameters:
// - buffer:  The data read from socket buffer to be parsed
//
// Returns:
// - The byte slice with the file name
// - A integer file size
// - Either nil on success or a string error message on failure
//
func GetFileInfo(buffer []byte, prefix []byte, suffix []byte) ([]byte, int64, error) {
    // Trim the delimiters around the file info
    buffer = buffer[len(prefix) : len(buffer) - len(suffix)]
    // Get the position of the colon delimiter
    colonPos := bytes.IndexByte(buffer, ':')
    // If the colon separator is missing
    if colonPos == -1 {
        return []byte(""), 0, fmt.Errorf("invalid message structure, colon missing")
    }

    // Extract the file path and size
    fileName := buffer[:colonPos]
    fileSizeStr := string(buffer[colonPos+1:])

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
        return "", -1, fmt.Errorf("unable to split IP and port from client address - %w", err)
    }

    // Convert the parsed string port to integer
    port, err := strconv.Atoi(strPort)
    if err != nil {
        return "", -1, fmt.Errorf("unable to convert string address %s to port - %w", strPort, err)
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


// Takes the passed in file name and parses it to the file path to create the file where the
// resulting file will be stored to lated be used for processing.
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - filePath:  The path of the file to be stored on disk from read socket data
// - fileSize:  The size of the to be stored on disk from read socket data
// - messagingPort:  The original port where the server-client messaging occurs
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
// - waitGroup:  Used to synchronize the Goroutines running
//
func HandleTransfer(connection net.Conn, filePath string, fileSize int64, messagingPort int32,
                    logMan *kloudlogs.LoggerManager, waitGroup *sync.WaitGroup) {
    // If a waitgroup was passed in
    if waitGroup != nil {
        // Decrement wait group on local exit
        defer waitGroup.Done()
    }

    // Get the IP address from the ip:port host address
    _, port, err := GetIpPort(connection)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                             "Error occcurred spliting host address to get IP/port:  %w", err)
        return
    }

    // If the parsed port of the passed in connection does not
    // match the original port used to manage messaging
    if port != messagingPort {
        // Ensure the transfer connection is closed upon local exit
        defer connection.Close()
    }

    //  Create buffer to optimal size based on expected file size
    transferBuffer := make([]byte, GetOptimalBufferSize(fileSize))

    kloudlogs.LogMessage(logMan, "info", "File transfer initiated", zap.String("file path", filePath),
                         zap.Int64("file size", fileSize))

    // Open the file for writing
    file, err := os.Create(filePath)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error creating the file %s:  %w", filePath, err)
        return
    }

    // Read data from the socket and write to the file path
    SocketToFileHander(connection, transferBuffer, file, logMan)
}


// ReadWrapper clears the buffer before reading
func ReadHandler(conn net.Conn, buffer *[]byte) (int, error) {
    // Clear the buffer to avoid leftover data
    *buffer = (*buffer)[:0]

    // Perform the read operation
    bytesRead, err := conn.Read(*buffer)
    if err != nil {
        return bytesRead, fmt.Errorf("error reading from connection - %w", err)
    }

    return bytesRead, nil
}


// Receives the file from the remote brain server
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - buffer:  The buffer used for processing socket messaging
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func ReceiveFile(connection net.Conn, buffer []byte, messagingPort int32, logMan *kloudlogs.LoggerManager,
                 storePath string, prefix []byte, suffix []byte) string {
    // Wait for the brain server to send the start transfer message
    _, err := ReadHandler(connection, &buffer)
    if err != nil {
        kloudlogs.LogMessage(logMan, "fatal", "Error receiving hash transfer message from server:  %w", err)
        os.Exit(2)
    }

    // If the read data does not start with special delimiter or end with closed bracket
    if !bytes.HasPrefix(buffer, prefix) || !bytes.HasSuffix(buffer, suffix) {
        kloudlogs.LogMessage(logMan, "fatal", "Unusual format in receieved hashes transfer message")
        os.Exit(2)
    }

    // Extract the file name and size from the initial transfer message
    fileName, fileSize, err := GetFileInfo(buffer, prefix, suffix)
    if err != nil {
        kloudlogs.LogMessage(logMan, "fatal",
                             "Error extracting the file name and size from hashes transfer message:  %w", err)
        os.Exit(2)
    }

    // Format the hash file path based on received file name
    filePath := fmt.Sprintf("%s/%s", storePath, fileName)
    // Receive the hash file from server
    HandleTransfer(connection, filePath, fileSize, messagingPort, logMan, nil)

    return filePath
}


// Reads data from the socket and write it to the passed in open file descriptor until end
// of file has been reached or error occurs with socket operation.
//
// Parameters:
// - connection:  Active socket connection for reading data to be stored and processed
// - transferBuffer:  Buffer allocated for file transfer based on file size
// - file:  The open file descriptor of where the data to be processed will be stored
// - logMan:  The kloudlogs logger manager for local and Cloudwatch logging
//
func SocketToFileHander(connection net.Conn, transferBuffer []byte, file *os.File,
                        logMan *kloudlogs.LoggerManager) {
    // Close file on local exit
    defer file.Close()

    for {
        // Read data into the buffer
        _, err := ReadHandler(connection, &transferBuffer)
        if err != nil {
            // If the error is not End Of File reached
            if err != io.EOF {
                kloudlogs.LogMessage(logMan, "error", "Error reading from socket:  %w", err)
                return
            }
            break
        }

        // Write the data to the file
        _, err = file.Write(transferBuffer)
        if err != nil {
            kloudlogs.LogMessage(logMan, "error", "Error writing to file:  %w", err)
            return
        }
    }
}


func TransferFile(connection net.Conn, messagingPort int32, filePath string, fileSize int64,
                  logMan *kloudlogs.LoggerManager) {
    // Get the IP address from the ip:port host address
    _, port, err := GetIpPort(connection)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error occcurred spliting host address to get IP/port:  %w", err)
        return
    }

    // If the parsed port of the passed in connection does not
    // match the original port used to manage messaging
    if port != messagingPort {
        // Ensure the transfer connection is closed upon local exit
        defer connection.Close()
    }

    // Create buffer to optimal size based on expected file size
    transferBuffer := make([]byte, GetOptimalBufferSize(fileSize))

    // Open the file
    file, err := os.Open(filePath)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error opening the file to be transfered:  %w", err)
        return
    }

    // Read the file chunk by chunk and send to client
    FileToSocketHandler(connection, transferBuffer, file, logMan)

    // Delete the transfered file
    err = os.Remove(filePath)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error deleting the file:  %w", err)
        return
    }
}


func UploadFile(connection net.Conn, buffer *[]byte, listenerPort int32,
                logMan *kloudlogs.LoggerManager, filePath string) {
    // Get the hash file size based on saved path in config
    fileInfo, err := os.Stat(filePath)
    if err != nil {
        kloudlogs.LogMessage(logMan, "fatal", "Error getting file size:  %w", err)
    }

    fileSize := fileInfo.Size()

    // Clear the buffer before building transfer reply
    *buffer = (*buffer)[:0]
    // Append the hash file transfer request piece by piece in buffer
    *buffer = append(globals.HASHES_TRANSFER_PREFIX, []byte(filePath)...)
    *buffer = append(*buffer, globals.COLON_DELIMITER...)
    *buffer = append(*buffer, []byte(strconv.FormatInt(fileSize, 10))...)
    *buffer = append(*buffer, globals.TRANSFER_SUFFIX...)

    // Send the hash file transfer request with file name and size
    _, err = WriteHandler(connection, buffer)
    if err != nil {
        kloudlogs.LogMessage(logMan, "fatal", "Error sending the hash file name and size:  %w", err)
    }

    // Transfer the hash file to client
    TransferFile(connection, listenerPort, filePath, fileSize, logMan)
}


// WriteWrapper writes data to the connection
func WriteHandler(conn net.Conn, buffer *[]byte) (int, error) {
    // Perform the write operation
    bytesWrote, err := conn.Write(*buffer)
    if err != nil {
        return 0, fmt.Errorf("error writing to connection - %w", err)
    }

    // Clear the buffer to avoid leftover data
    *buffer = (*buffer)[:0]

    return bytesWrote, nil
}
