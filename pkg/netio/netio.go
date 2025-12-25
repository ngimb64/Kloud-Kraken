package netio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"strconv"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
)

// Copy data directly to network socket using specified kernel buffer.
//
// @Parameters
//  - connection:  The active TCP socket connection to transmit data
//  - file:  A pointer to the open file descriptor
//  - transferBuffer:  The buffer used to store file data that is transferred
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func FileToSocketCopy(connection net.Conn, file *os.File,
                      transferBuffer []byte) error {
    // Close the file on local exit
    defer file.Close()

    // Transfer data from open file to connection
    _, err := io.CopyBuffer(connection, file, transferBuffer)
    if err != nil {
        return err
    }

    return nil
}


// Format start transfer message in buffer the file path and size sent to the client.
//
// @Parameters
//  - filePath:  The path to the file to be transfered
//  - fileSize:  The size of the file to be transfered
//  - buffer:  The buffer where the transfer reply is formatted
//  - prefix:  The prefix used on the message
//
// @Returns
//  - Return the length of the formatted transfer reply
//  - Error if it occurs, otherwise nil on success
//
func FormatStartTransfer(filePath string, fileSize int64,
                         buffer *[]byte, prefix []byte) (
                         int, error) {
    byteFilePath := []byte(filePath)
    byteFileSize := []byte(strconv.FormatInt(fileSize, 10))
    // Grab the file name from the end of the path
    fileName, err := data.TrimAfterLast(byteFilePath, []byte("/"))
    if err != nil {
        return -1, err
    }

    // Set the buffer pointer to beginning
    *buffer = (*buffer)[:0]
    // Append the transfer request piece by piece in buffer
    *buffer = append(*buffer, prefix...)
    *buffer = append(*buffer, fileName...)
    *buffer = append(*buffer, globals.COLON_DELIMITER...)
    *buffer = append(*buffer, byteFileSize...)
    *buffer = append(*buffer, globals.TRANSFER_SUFFIX...)
    // Calculate the len of the transfer reply message
    return len(*buffer), nil
}


// Format the transfer reply in buffer the file path and size sent to the client.
//
// @Parameters
//  - port:  port number to add to message, must be a
//           non-negative integer above 1
//  - buffer:  The buffer where the transfer reply is formatted
//  - prefix:  The prefix used on the message
//
// @Returns
//  - Return the length of the formatted transfer reply
//  - Error if it occurs, otherwise nil on success
//
func FormatTransferRequest(port int, buffer *[]byte,
                           prefix []byte) (int, error) {
    bytePort := []byte(strconv.Itoa(port))

    // Set the buffer pointer to beginning
    *buffer = (*buffer)[:0]
    // Append the transfer request piece by piece in buffer
    *buffer = append(*buffer, prefix...)
    *buffer = append(*buffer, bytePort...)
    *buffer = append(*buffer, globals.TRANSFER_SUFFIX...)
    // Calculate the len of the transfer reply message
    return len(*buffer), nil
}


// In a continuous loop, attempt to find a port to establish a listener.
// If there is an error it will re-iterate until a listener is found and
// returned with its corresponding port number.
//
// @Returns
//  - The established listener
//  - The port number the listener is established on
//
func GetAvailableListener() (net.Listener, int) {
    var minPort int = 1001
    var maxPort int = 65535

    for {
        // Select a random port inside min-max range
        port := rand.Intn(maxPort - minPort+1) + minPort

        // Attempt to establish a local listener for incoming connect
        testListener, err := net.Listen("tcp", ":" + strconv.Itoa(port))
        // If the listener not was succefully established
        if err != nil {
            continue
        }

        return testListener, port
    }
}


// Get the IP address and port of the passed in connection.
//
// @Parameters
//  - connection:  The connection to get the IP and port
//
// @Returns
//  - The parsed IP address
//  - The parsed port
//  - Error if it occurs, otherwise nil on success
//
func GetIpPort(connection net.Conn) (string, int, error) {
    // Get the ip:port adress of the connected client
    stringAddr := connection.RemoteAddr().String()
    if stringAddr == "" {
        return "", -1, fmt.Errorf("unable to retrieve client address from connection")
    }

    // Split the IP and port from address, saving IP in variable
    ipAddr, strPort, err := net.SplitHostPort(stringAddr)
    if err != nil {
        return "", -1, err
    }

    // Convert the parsed string port to integer
    port, err := strconv.Atoi(strPort)
    if err != nil {
        return "", -1, err
    }

    return ipAddr, port, nil
}


// Adjust buffer to optimal size based on file size to be received.
//
// @Parameters
//  - fileSize:  The size of the file to be received
//
// @Returns
//  - An optimal integer buffer size
//
func GetOptimalBufferSize(fileSize int64) int {
    switch {
    // If the file is less than or equal to 1 MB
    case fileSize <= 1 * globals.MB:
        // 4 KB buffer
        return 8 * globals.KB
    // If the file is less than or equal to 100 MB
    case fileSize <= 100 * globals.MB:
        // 64 KB buffer
        return 128 * globals.KB
    // If the file is less than or equal to 1 GB
    case fileSize <= 1 * globals.GB:
        // 1 MB buffer
        return 1 * globals.MB
    // If the file is greater than 1 GB
    default:
        // 4 MB buffer
        return 4 * globals.MB
    }
}


// Sets up file to be received by allocating an optimal buffer size based on expected
// file size and creating an empty file before proceeding to the file to socket handler.
//
// @Parameters
//  - connection:  Active socket connection for reading data to be stored and processed
//  - storePath:  The directory where read socket data will be stored as files
//  - fileName:  The name of the file to store
//  - fileSize:  The size of the to be stored on disk from read socket data
//
// @Returns
//  - The file path where the file was received
//  - Error if it occurs, otherwise nil on success
//
func HandleTransferRecv(connection net.Conn, storePath string,
                        fileName string, fileSize int64) (
                        string, error) {
    var file *os.File
    var err error
    //  Create buffer to optimal size based on expected file size
    transferBuffer := make([]byte, GetOptimalBufferSize(fileSize))
    // Format the path where the file will be stored
    filePath := storePath + "/" + fileName

    for {
        // Open the file for writing
        file, err = os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
        // If a file with the same name already exists
        if os.IsExist(err) {
            // Add random characters to beginning of name, then try again
            filePath = storePath + "/" + data.RandStringBytes(8) + "_" + fileName
            continue
        } else if err != nil {
            return "", err
        }

        break
    }

    // Read data from the socket and write to the file path
    err = SocketToFileCopy(file, connection, transferBuffer, fileSize)
    if err != nil {
        return "", err
    }

    return filePath, nil
}


// Parse file name:size from buffer data based on colon separator.
//
// @Parameters
//  - buffer:  The data read from socket buffer to be parsed
//  - prefix:  The message prefix format
//  - bytesRead:  The number of bytes read into the buffer
//
// @Returns
//  - The file name
//  - A integer file size
//  - Error if it occurs, otherwise nil on success
//
func ParseStartTransfer(buffer []byte, prefix []byte, bytesRead int) (
                        string, int64, error) {
    // Trim the delimiters around the file info
    buffer = buffer[len(prefix):bytesRead-1]
    // Split the buffer message into slice by colon delimiters
    parsedEntries := bytes.Split(buffer, []byte(":"))
    // If neither of expected number of entries were found or none
    if len(parsedEntries) != 2 {
        return "", -1, fmt.Errorf("unexpected number of entries" +
            " parsed in transfer reply - %d", len(parsedEntries))
    }

    // Convert the size to bytes -> string -> integr
    fileSize, err := strconv.ParseInt(string(parsedEntries[1]), 10, 64)
    if err != nil {
        return "", -1, err
    }

    return string(parsedEntries[0]), fileSize, nil
}


// Parse file name:size from buffer data based on colon separator.
//
// @Parameters
//  - buffer:  The data read from socket buffer to be parsed
//  - prefix:  The message prefix format
//  - bytesRead:  The number of bytes read into the buffer
//
// @Returns
//  - A network port to connect to
//  - Error if it occurs, otherwise nil on success
//
func ParseTransferRequest(buffer []byte, prefix []byte, bytesRead int) (
                          int, error) {
    // Trim the delimiters around the file info
    buffer = buffer[len(prefix):bytesRead-1]
    // Convert network port back to int
    port, err := strconv.Atoi(string(buffer))
    if err != nil {
        return -1, fmt.Errorf("error converting port back to int - %w", err)
    }

    return port, nil
}


// Handler for network socket read operations.
//
// @Parameters
//  - connection:  Network connection where data will be read from
//  - buffer:  The buffer used for processing socket messaging
//  - suffix:  Pattern at end of meesage signaling its end
//
// @Returns
//  - The number of bytes read into the buffer
//  - Error if it occurs, otherwise nil on success
//
func ReadHandler(connection net.Conn, buffer *[]byte,
                 suffix []byte) ([]byte, error) {
    var message []byte
    // Use full capacity for reading
    *buffer = (*buffer)[:cap(*buffer)]

    for {
        // Read data from connection into temp buffer
        bytesRead, err := connection.Read(*buffer)
        if err != nil {
            return nil, err
        }

        // Add the read data in temp buffer to message buffer
        message = append(message, (*buffer)[:bytesRead]...)

        // If the message of read(s) has end suffix
        if bytes.HasSuffix(message, suffix) {
            break
        }
    }

    return message, nil
}


// Waits for the start transfer message and parses the file name and size from it.
// The file name is appended to the current path and passed into the receive handler.
//
// @Parameters
//  - connection:  Active socket connection for receiving data
//  - buffer:  The buffer used for processing socket messaging
//  - storePath:  The path where the received file will be stored
//  - prefix:  The expected prefix for the transfer reply
//
// @Returns
//  - The formatted file path with the received file name
//  - Error if it occurs, otherwise nil on success
//
func ReceiveFile(connection net.Conn, buffer []byte,
                 storePath string, prefix []byte) (
                 string, error) {
    // Wait for the transfer reply with file name and size
    readBuffer, err := ReadHandler(connection, &buffer, []byte(">"))
    if err != nil {
        return "", err
    }

    // If read data does not start with delimiter or end with closed bracket
    if !bytes.HasPrefix(readBuffer, prefix) ||
    !bytes.HasSuffix(readBuffer, globals.TRANSFER_SUFFIX) {
        return "", fmt.Errorf("improper prefix or suffix in transfer reply")
    }

    // Extract the file name and size from the initial transfer message
    fileName, fileSize, err := ParseStartTransfer(readBuffer, prefix,
                                                  len(readBuffer))
    if err != nil {
        return "", err
    }

    // Send the transfer initiated message to receiver to ensure synchronization
    _, err = WriteHandler(connection, globals.TRANSFER_INITIATED_MARKER,
                          len(globals.TRANSFER_INITIATED_MARKER))
    if err != nil {
        return "", err
    }

    // Receive the file from server
    filePath, err := HandleTransferRecv(connection, storePath,
                                        fileName, fileSize)
    if err != nil {
        return "", err
    }

    return filePath, nil
}


// Reads data from the socket and write it to the passed in open file descriptor
// until end of expected file size has been reached or error occurs with socket
// operation.
//
// @Parameters
//  - file:  Open file descriptor where the data to be processed will be stored
//  - connection:  Socket connection for reading data to be stored and processed
//  - transferBuffer:  Buffer allocated for file transfer based on file size
//  - fileSize:  The size of the file to be received
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func SocketToFileCopy(file *os.File, connection net.Conn,
                      transferBuffer []byte, fileSize int64) error {
    // Close file on local exit
    defer file.Close()

    // Set up limited reader to prevent connection from hanging after copy
    limitedReader := &io.LimitedReader{R: connection, N: fileSize}

    // Transfer data from connection to open file
    _, err := io.CopyBuffer(file, limitedReader, transferBuffer)
    if err != nil {
        return err
    }

    return nil
}


// Gets the IP address and port, sets up optimal buffer based on expected file
// size, opens the file and calls method to send the file via network socket.
// After the transfer is complete the file is deleted from disk.
//
// @Parameters
//  - connection:  The network connection where the file will be sent
//  - filePath:  The path to the file to be transfered
//  - fileSize:  The size of the file to be transfered
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func TransferFile(connection net.Conn, filePath string, fileSize int64) error {
    // Create buffer to optimal size based on expected file size
    transferBuffer := make([]byte, GetOptimalBufferSize(fileSize))

    // Open the file
    file, err := os.Open(filePath)
    if err != nil {
        return err
    }

    // Read the file chunk by chunk and send to client
    err = FileToSocketCopy(connection, file, transferBuffer)
    if err != nil {
        return err
    }

    return nil
}


// Gets the file size, formats and sends the transfer reply, and calls
// transfer method.
//
// @Parameters
//  - connection:  The network connection where the file will be sent
//  - buffer:  The buffer used for processing socket messaging
//  - filePath:  The path to the file to be uploaded
//  - prefix:  The prefix of the transfer reply
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func UploadFile(connection net.Conn, buffer []byte,
                filePath string, prefix []byte) error {
    // Get the file size based on saved path in config
    fileInfo, err := os.Stat(filePath)
    if err != nil {
        return err
    }

    // Get the size of the file for transfer reply
    fileSize := fileInfo.Size()

    // Format the transfer reply
    sendLength, err := FormatStartTransfer(filePath, fileSize, &buffer, prefix)
    if err != nil {
        return err
    }

    // Send the file transfer reply with file name and size
    _, err = WriteHandler(connection, buffer, sendLength)
    if err != nil {
        return err
    }

    // Receive transfer initiated message from client to ensure synchronization
    readBuffer, err := ReadHandler(connection, &buffer, []byte(">"))
    if err != nil {
        return err
    }

    // If the transfer initiated message format is invalid
    if !bytes.Contains(readBuffer, globals.TRANSFER_INITIATED_MARKER) {
        return errors.New("transfer initiated message format invalid")
    }

    // Transfer the file to client
    err = TransferFile(connection, filePath, fileSize)
    if err != nil {
        return err
    }

    return nil
}


// Handler for network socket write operations.
//
// @Parameters
//  - connection:  The network connection where data will be wrote to
//  - buffer:  The buffer where the data will be wrote to
//  - writeBytes:  The number of bytes into the buffer to write
//
// @Returns
//  - The number of bytes wrote from the buffer
//  - Error if it occurs, otherwise nil on success
//
func WriteHandler(connection net.Conn, buffer []byte,
                  writeBytes int) (int, error) {
    // Perform write operation via passed in connection
    bytesWrote, err := connection.Write(buffer[:writeBytes])
    if err != nil {
        return 0, err
    }

    return bytesWrote, nil
}
