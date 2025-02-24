package netio

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"strconv"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
)

// Handle reading data from the passed in file descriptor and write to
// the socket to client.
//
// @Parameters
// - connection:  The active TCP socket connection to transmit data
// - file:  A pointer to the open file descriptor
// - transferBuffer:  The buffer used to store file data that is transferred
//
// @Returns
// - Error if it occurs, otherwise nil on success
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


// Format the transfer reply in buffer the file path and size sent to the client.
//
// @Parameters
// - filePath:  The path to the file to be transfered
// - fileSize:  The size of the file to be transfered
// - buffer:  The buffer where the transfer reply is formatted
// - prefix:  The prefix used on the message
//
// @Returns
// - Return the length of the formatted transfer reply
// - Error if it occurs, otherwise nil on success
//
func FormatTransferReply(filePath string, fileSize int64, buffer *[]byte,
                         prefix []byte) (int, error) {
    byteFilePath := []byte(filePath)
    byteFileSize := []byte(strconv.FormatInt(fileSize, 10))
    // Grab the file name from the end of the path
    fileName, err := data.TrimAfterLast(byteFilePath, []byte("/"))
    if err != nil {
        return -1, err
    }

    // Clear the buffer for sending transfer reply
    copy(*buffer, make([]byte, len(*buffer)))
    // Append the transfer request piece by piece in buffer
    *buffer = append(prefix, fileName...)
    *buffer = append(*buffer, globals.COLON_DELIMITER...)
    *buffer = append(*buffer, byteFileSize...)
    *buffer = append(*buffer, globals.TRANSFER_SUFFIX...)
    // Calculate the len of the transfer reply message
    sendLength := len(prefix) + len(fileName) + len(globals.COLON_DELIMITER) +
                  len(byteFileSize) + len(globals.TRANSFER_SUFFIX)
    return sendLength, nil
}


// In a continuous loop, attempt to find a port to establish a listener.
// If there is an error it will re-iterate until a listener is found and
// returned with its corresponding port number.
//
// @Returns
// - The established listener
// - The port number the listener is established on
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


// Parse file name:size from buffer data based on colon separator.
//
// @Parameters
// - buffer:  The data read from socket buffer to be parsed
// - prefix:  The message prefix format
// - bytesRead:  The number of bytes read into the buffer
//
// @Returns
// - The byte slice with the file name
// - A integer file size
// - Error if it occurs, otherwise nil on success
//
func GetFileInfo(buffer []byte, prefix []byte, bytesRead int) ([]byte, int64, error) {
    // Trim the delimiters around the file info
    buffer = buffer[len(prefix):bytesRead-1]

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
    fileSize, err := strconv.ParseInt(fileSizeStr, 10, 64)
    if err != nil {
        return fileName, fileSize, err
    }

    return fileName, fileSize, nil
}


// Get the IP address and port of the passed in connection.
//
// @Parameters
// - connection:  The connection to get the IP and port
//
// @Returns
// - The parsed IP address
// - The parsed port
// - Error if it occurs, otherwise nil on success
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
// - fileSize:  The size of the file to be received
//
// @Returns
// - An optimal integer buffer size
//
func GetOptimalBufferSize(fileSize int64) int {
    switch {
    // If the file is less than or equal to 1 MB
    case fileSize <= 1 * globals.MB:
        // 4 KB buffer
        return 4 * globals.KB
    // If the file is less than or equal to 100 MB
    case fileSize <= 100 * globals.MB:
        // 64 KB buffer
        return 64 * globals.KB
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
// - connection:  Active socket connection for reading data to be stored and processed
// - storePath:  The directory where read socket data will be stored as files
// - fileName:  The name of the file to store
// - fileSize:  The size of the to be stored on disk from read socket data
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func HandleTransferRecv(connection net.Conn, storePath string, fileName string,
                        fileSize int64) (string, error) {
    var file *os.File
    var err error
    //  Create buffer to optimal size based on expected file size
    transferBuffer := make([]byte, GetOptimalBufferSize(fileSize))

    filePath := storePath + "/" + fileName

    for {
        // Open the file for writing
        file, err = os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
        // If a file with the same name already exists
        if os.IsExist(err) {
            // Add some some random characters to the beginning of the name
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


// Handler for network socket read operations.
//
// @Parameters
// - connection:  The network connection where data will be read from
// - buffer:  The buffer where the read data will be stored
//
// @Returns
// - The number of bytes read into the buffer
// - Error if it occurs, otherwise nil on success
//
func ReadHandler(connection net.Conn, buffer *[]byte) (int, error) {
    // Perform read operation via passed in connection
    bytesRead, err := connection.Read(*buffer)
    if err != nil {
        return bytesRead, fmt.Errorf("error reading from connection - %w", err)
    }

    return bytesRead, nil
}


// Waits for the start transfer message and parses the file name and size from it.
// The file name is appended to the current path and passed into the receive handler.
//
// @Parameters
// - connection:  Active socket connection for receiving data
// - buffer:  The buffer used for processing socket messaging
// - storePath:  The path where the received file will be stored
// - prefix:  The expected prefix for the transfer reply
//
// @Returns
// - The formatted file path with the received file name
// - Error if it occurs, otherwise nil on success
//
func ReceiveFile(connection net.Conn, buffer []byte, storePath string,
                 prefix []byte) (string, error) {
    // Wait for the start transfer message
    bytesRead, err := ReadHandler(connection, &buffer)
    if err != nil {
        return "", err
    }

    // If read data does not start with delimiter or end with closed bracket
    if !bytes.HasPrefix(buffer, prefix) ||
    !bytes.HasSuffix(buffer[:bytesRead], globals.TRANSFER_SUFFIX) {
        return "", fmt.Errorf("improper prefix or suffix in transfer message")
    }

    // Extract the file name and size from the initial transfer message
    fileName, fileSize, err := GetFileInfo(buffer, prefix, bytesRead)
    if err != nil {
        return "", err
    }

    // Receive the file from server
    filePath, err := HandleTransferRecv(connection, storePath,
                                        string(fileName), fileSize)
    if err != nil {
        return "", err
    }

    return filePath, nil
}


// Reads data from the socket and write it to the passed in open file descriptor until end
// of expected file size has been reached or error occurs with socket operation.
//
// @Parameters
// - file:  The open file descriptor of where the data to be processed will be stored
// - connection:  Active socket connection for reading data to be stored and processed
// - transferBuffer:  Buffer allocated for file transfer based on file size
// - fileSize:  The size of the file to be received
//
// @Returns
// - Error if it occurs, otherwise nil on success
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


// Gets the IP address and port, sets up optimal buffer based on expected file size, opens
// the file and calls method to send the file via network socket. After the transfer is
// complete the file is deleted from disk.
//
// @Parameters
// - connection:  The network connection where the file will be sent
// - filePath:  The path to the file to be transfered
// - fileSize:  The size of the file to be transfered
//
// @Returns
// - Error if it occurs, otherwise nil on success
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

    // Delete the transfered file
    err = os.Remove(filePath)
    if err != nil {
        return err
    }

    return nil
}


// Gets the file size, formats and sends the transfer reply, and calls transfer method.
//
// @Parameters
// - connection:  The network connection where the file will be sent
// - buffer:  The buffer used for server-client messaging
// - filePath:  The path to the file to be uploaded
// - prefix:  The prefix of the transfer reply
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func UploadFile(connection net.Conn, buffer []byte, filePath string,
                prefix []byte) error {
    // Get the file size based on saved path in config
    fileInfo, err := os.Stat(filePath)
    if err != nil {
        return err
    }

    // Get the size of the file for transfer reply
    fileSize := fileInfo.Size()

    // Format the transfer reply
    sendLength, err := FormatTransferReply(filePath, fileSize, &buffer, prefix)
    if err != nil {
        return err
    }

    // Send the file transfer reply with file name and size
    _, err = WriteHandler(connection, buffer, sendLength)
    if err != nil {
        return err
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
// - connection:  The network connection where data will be wrote to
// - buffer:  The buffer where the data will be wrote to
// - writeBytes:  The number of bytes into the buffer to write
//
// @Returns
// - The number of bytes wrote from the buffer
// - Error if it occurs, otherwise nil on success
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
