package netio_test

import (
	"io"
	"net"
	"os"
	"strconv"
	"testing"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"github.com/ngimb64/Kloud-Kraken/pkg/netio"
	"github.com/stretchr/testify/assert"
)

func TestFileToSocketCopy(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    testFiles := []string{}
    // Get available listener and its corresponding port
    listener, listenerPort := netio.GetAvailableListener()
    // Close listener on local exit
    defer listener.Close()

    isComplete := make(chan bool)

    go func() {
        // Create the input file and return handle
        outFilePath, outFile, err := disk.CreateRandFile(".", globals.RAND_STRING_SIZE,
                                                         "output_test", "txt", true)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
        // Close the output file on local exit
        defer outFile.Close()

        // Add the created file to slice for later removal
        testFiles = append(testFiles, outFilePath)

        // Wait for an incoming connection
        clientConn, err := listener.Accept()
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
        // Close connection on local exit
        defer clientConn.Close()

        // Make 64KB buffer for receiving file transfer
        receiveBuffer := make([]byte, 64 * globals.KB)

        // Transfer received data from connection to file
        bytesWrote, err := io.CopyBuffer(outFile, clientConn, receiveBuffer)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
        // Ensure the bytes wrote equals the expected target
        assert.Equal(int64(20 * globals.MB), bytesWrote)

        // Send complete signal via channel
        isComplete <- true
    } ()

    // Format connection address for testing
    connectAddr := ":" + strconv.Itoa(int(listenerPort))

    // Make a connection to the remote brain server
    serverConn, err := net.Dial("tcp", connectAddr)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Create the input file and return handle
    inFilePath, inFile, err := disk.CreateRandFile(".", globals.RAND_STRING_SIZE,
                                                   "input_test", "txt", true)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Add the created file to slice for later removal
    testFiles = append(testFiles, inFilePath)

    // Make buffer to hold random data and write random data to it
    writeBuffer := make([]byte, 20 * globals.MB)
    data.GenerateRandomBytes(writeBuffer, 20 * globals.MB)
    // Write the buffer of random data to file
    bytesWrote, err := inFile.Write(writeBuffer)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the number of bytes wrote equals the buffer size
    assert.Equal(20 * globals.MB, bytesWrote)

    // Reset the file pointer to begining of file for transfer
    _, err = inFile.Seek(int64(0), 0)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Make 64KB buffer to transfer file
    transferBuffer := make([]byte, 64 * globals.KB)

    // Transfer file directly through the socket
    err = netio.FileToSocketCopy(serverConn, inFile, transferBuffer)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Close the connection after copying
    serverConn.Close()

    // Wait for the channel to send complete signal
    <-isComplete

    // Get the size of the input file
    inFileInfo, err := os.Stat(testFiles[1])
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Get the size of the output file
    outFileInfo, err := os.Stat(testFiles[0])
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Ensure the input and output files are the same size
    assert.Equal(inFileInfo.Size(), outFileInfo.Size())

    // Iterate though create files and delete them
    for _, testFile := range testFiles {
        err = os.Remove(testFile)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
    }
}


func TestFormatTransferReply(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    filePath := "/test/path.txt"
    fileSize := int64(13 * globals.MB)
    byteFilePath := []byte(filePath)
    byteFileSize := []byte(strconv.FormatInt(fileSize, 10))
    buffer := make([]byte, 256)

    // Grab the file name from the end of the path
    fileName, err := data.TrimAfterLast(byteFilePath, []byte("/"))
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Format the transfer reply in passed in buffer
    sendLength, err := netio.FormatTransferReply(filePath, fileSize, &buffer,
                                                 globals.START_TRANSFER_PREFIX)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Ensure message in buffer begins with the start transfer prefix
    assert.Equal(globals.START_TRANSFER_PREFIX,
                 buffer[:len(globals.START_TRANSFER_PREFIX)])
    // Ensure the filname is properly formatted
    assert.Equal(fileName,
                 buffer[len(globals.START_TRANSFER_PREFIX):sendLength-len(byteFileSize)-2])
    // Ensure the colon is properly formatted
    assert.Equal(globals.COLON_DELIMITER,
                 buffer[len(globals.START_TRANSFER_PREFIX)+len(fileName):sendLength-len(byteFileSize)-1])
    // Ensure the file size is properly formatted
    assert.Equal(byteFileSize,
                 buffer[len(globals.START_TRANSFER_PREFIX)+len(fileName)+1:sendLength-1])
    // Ensure the transfer suffix is properly formatted
    assert.Equal(globals.TRANSFER_SUFFIX,
                 buffer[sendLength-len(globals.TRANSFER_SUFFIX):])
}


func TestGetAvailableListener(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get available listener and its corresponding port
    testListener, port := netio.GetAvailableListener()
    // Ensure the port is a non-privileged
    assert.Greater(port, 1001)

    // Close the established listener on random port
    err := testListener.Close()
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
}


func TestGetFileInfo(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    filePath := "/test/path.txt"
    fileSize := int64(13 * globals.MB)
    byteFileSize := []byte(strconv.FormatInt(fileSize, 10))
    buffer := make([]byte, 256)

    // Format the transfer reply in passed in buffer
    sendLength, err := netio.FormatTransferReply(filePath, fileSize, &buffer,
                                                 globals.START_TRANSFER_PREFIX)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Ensure the send length adds up to its intended components
    assert.Equal(sendLength, len(globals.START_TRANSFER_PREFIX)+(len(filePath)-6)+1+len(byteFileSize)+1)

    // Parse the file name and size from the transfer reply message in buffer
    resFileName, resFileSize, err := netio.GetFileInfo(buffer, globals.START_TRANSFER_PREFIX, sendLength)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the parsed file name is correct
    assert.Equal([]byte("path.txt"), resFileName)
    // Ensure the parsed file size is correct
    assert.Equal(fileSize, resFileSize)
}


func TestGetIpPort(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get available listener and its corresponding port
    listener, listenerPort := netio.GetAvailableListener()
    // Close listener on local exit
    defer listener.Close()

    isComplete := make(chan bool)

    go func() {
        // Wait for an incoming connection
        clientConn, err := listener.Accept()
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
        // Close connection on local exit
        defer clientConn.Close()

        // Send complete signal via channel
        isComplete <- true
    } ()

    // Format connection address for testing
    connectAddr := ":" + strconv.Itoa(listenerPort)

    // Make a connection to the remote brain server
    serverConn, err := net.Dial("tcp", connectAddr)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close connection on local exit
    defer serverConn.Close()

    // Get the IP address and port from established connection
    ipAddr, port, err := netio.GetIpPort(serverConn)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Ensure the IP address is the local host
    assert.Equal("127.0.0.1", ipAddr)
    // Ensure the port is the listener port established by server
    assert.Equal(listenerPort, port)

    // Wait for the channel to send complete signal
    <-isComplete
}


func TestGetOptimalBufferSize(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    tests := []struct {
        fileSize int64
        output   int
        expected int
    } {
        {512 * globals.KB, 0, 8 * globals.KB},
        {50 * globals.MB, 0, 128 * globals.KB},
        {500 * globals.MB, 0, 1 * globals.MB},
        {2 * globals.GB, 0, 4 * globals.MB},
    }

    // Iterate through test case slice
    for _, test := range tests {
        // Get the optimal buffer size based on passed in file size
        test.output = netio.GetOptimalBufferSize(test.fileSize)
        // Ensure the returned buffer matches the expected
        assert.Equal(test.expected, test.output)
    }
}


func TestHandleTransferRecv(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    testFiles := []string{}
    // Get available listener and its corresponding port
    listener, listenerPort := netio.GetAvailableListener()
    // Close listener on local exit
    defer listener.Close()

    isComplete := make(chan bool)

    go func() {
        // Wait for an incoming connection
        clientConn, err := listener.Accept()
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
        // Close connection on local exit
        defer clientConn.Close()

        // Read data from the socket and write to the file path
        outFilePath, err := netio.HandleTransferRecv(clientConn, "./", "output_test.txt",
                                                     int64(20 * globals.MB))
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
        // Add the created file to slice for later removal
        testFiles = append(testFiles, outFilePath)

        // Send complete signal via channel
        isComplete <- true
    } ()

    // Format connection address for testing
    connectAddr := ":" + strconv.Itoa(listenerPort)

    // Make a connection to the remote brain server
    serverConn, err := net.Dial("tcp", connectAddr)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close connection on local exit
    defer serverConn.Close()

    // Create the input file and return handle
    inFilePath, inFile, err := disk.CreateRandFile(".", globals.RAND_STRING_SIZE,
                                                   "input_test", "txt", true)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Add the created file to slice for later removal
    testFiles = append(testFiles, inFilePath)

    // Make buffer to hold random data and write random data to it
    writeBuffer := make([]byte, 20 * globals.MB)
    data.GenerateRandomBytes(writeBuffer, 20 * globals.MB)
    // Write the buffer of random data to file
    bytesWrote, err := inFile.Write(writeBuffer)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the number of bytes wrote equals the buffer size
    assert.Equal(20 * globals.MB, bytesWrote)

    // Reset the file pointer to begining of file for transfer
    _, err = inFile.Seek(int64(0), 0)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Create buffer for file transfer
    transferBuffer := make([]byte, 64 * globals.KB)

    // Transfer the file to the client
    err = netio.FileToSocketCopy(serverConn, inFile, transferBuffer)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Wait for the channel to send complete signal
    <-isComplete

    // Get the size of the input file
    inFileInfo, err := os.Stat(testFiles[1])
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Get the size of the output file
    outFileInfo, err := os.Stat(testFiles[0])
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Ensure the input and output files are the same size
    assert.Equal(inFileInfo.Size(), outFileInfo.Size())

    // Iterate though create files and delete them
    for _, testFile := range testFiles {
        err = os.Remove(testFile)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
    }
}


func TestReadHandler(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get available listener and its corresponding port
    listener, listenerPort := netio.GetAvailableListener()
    // Close listener on local exit
    defer listener.Close()

    isComplete := make(chan bool)
    testMessage := []byte("This is a test message that is used for testing purposes")
    testMessage2 := []byte("This is another test message that is used for testing")

    go func() {
        // Wait for an incoming connection
        clientConn, err := listener.Accept()
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
        // Close connection on local exit
        defer clientConn.Close()

        // Make buffer for receiving data
        receiveBuffer := make([]byte, 64)
        // Read the message from server and store into buffer
        bytesRead, err := netio.ReadHandler(clientConn, &receiveBuffer)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)

        // Ensure bytes read equals the expected message
        assert.Equal(len(testMessage), bytesRead)
        // Ensure content in the buffer matches expected message
        assert.Equal(testMessage, receiveBuffer[:len(testMessage)])

        // Perform write operation via passed in connection
        bytesWrote, err := clientConn.Write(testMessage2)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
        // Ensure bytes wrote equals the expected message
        assert.Equal(len(testMessage2), bytesWrote)

        // Send complete signal via channel
        isComplete <- true
    } ()

    // Format connection address for testing
    connectAddr := ":" + strconv.Itoa(int(listenerPort))

    // Make a connection to the remote brain server
    serverConn, err := net.Dial("tcp", connectAddr)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close connection on local exit
    defer serverConn.Close()

    // Perform write operation via passed in connection
    bytesWrote, err := serverConn.Write(testMessage)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure bytes wrote equals the expected message
    assert.Equal(len(testMessage), bytesWrote)

    // Make buffer to receive data
    buffer := make([]byte, 64)
    // Read the message from server and store into buffer
    bytesRead, err := netio.ReadHandler(serverConn, &buffer)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Ensure bytes read equals the expected message length
    assert.Equal(len(testMessage2), bytesRead)
    // Ensure content in the buffer matches expected message
    assert.Equal(testMessage2, buffer[:len(testMessage2)])

    // Wait for the channel to send complete signal
    <-isComplete
}


func TestSocketToFileCopy(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    testFiles := []string{}
    // Get available listener and its corresponding port
    listener, listenerPort := netio.GetAvailableListener()
    // Close listener on local exit
    defer listener.Close()

    isComplete := make(chan bool)

    go func() {
        // Create the input file and return handle
        outFilePath, outFile, err := disk.CreateRandFile(".", globals.RAND_STRING_SIZE,
                                                         "output_test", "txt", true)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
        // Add the created file to slice for later removal
        testFiles = append(testFiles, outFilePath)

        // Wait for an incoming connection
        clientConn, err := listener.Accept()
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)

        // Make 64KB buffer for receiving file transfer
        receiveBuffer := make([]byte, 64 * globals.KB)

        // Transfer received data from connection to file
        err = netio.SocketToFileCopy(outFile, clientConn, receiveBuffer,
                                     int64(20 * globals.MB))
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)

        // Get the size of the output file
        outFileInfo, err := os.Stat(outFile.Name())
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)

        // Ensure the bytes wrote equals the expected target
        assert.Equal(int64(20 * globals.MB), outFileInfo.Size())

        // Send complete signal via channel
        isComplete <- true
    } ()

    // Format connection address for testing
    connectAddr := ":" + strconv.Itoa(int(listenerPort))

    // Make a connection to the remote brain server
    serverConn, err := net.Dial("tcp", connectAddr)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Create the input file and return handle
    inFilePath, inFile, err := disk.CreateRandFile(".", globals.RAND_STRING_SIZE,
                                                   "input_test", "txt", true)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure file hanle closes on local exit
    defer inFile.Close()
    // Add the created file to slice for later removal
    testFiles = append(testFiles, inFilePath)

    // Make buffer to hold random data and write random data to it
    writeBuffer := make([]byte, 20 * globals.MB)
    data.GenerateRandomBytes(writeBuffer, 20 * globals.MB)
    // Write the buffer of random data to file
    bytesWrote, err := inFile.Write(writeBuffer)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the number of bytes wrote equals the buffer size
    assert.Equal(20 * globals.MB, bytesWrote)

    // Reset the file pointer to begining of file for transfer
    _, err = inFile.Seek(int64(0), 0)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Make 64KB buffer to transfer file
    transferBuffer := make([]byte, 64 * globals.KB)

    // Transfer file directly through the socket
    _, err = io.CopyBuffer(serverConn, inFile, transferBuffer)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Wait for the channel to send complete signal
    <-isComplete

    // Close the connection after signal (normally would hang without limited reader)
    serverConn.Close()

    // Get the size of the input file
    inFileInfo, err := os.Stat(testFiles[1])
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Get the size of the output file
    outFileInfo, err := os.Stat(testFiles[0])
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Ensure the input and output files are the same size
    assert.Equal(inFileInfo.Size(), outFileInfo.Size())

    // Iterate though create files and delete them
    for _, testFile := range testFiles {
        err = os.Remove(testFile)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
    }
}


func TestTransferFile(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get available listener and its corresponding port
    listener, listenerPort := netio.GetAvailableListener()
    // Close listener on local exit
    defer listener.Close()

    isComplete := make(chan bool)
    // Create the input file and return handle
    outFilePath, outFile, err := disk.CreateRandFile(".", globals.RAND_STRING_SIZE,
                                                     "output_test", "txt", true)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    go func() {
        // Wait for an incoming connection
        clientConn, err := listener.Accept()
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
        // Close connection on local exit
        defer clientConn.Close()

        // Make 64KB buffer to receive file
        transferBuffer := make([]byte, 64 * globals.KB)

        // Read data from the socket and write to the file path
        err = netio.SocketToFileCopy(outFile, clientConn, transferBuffer,
                                     20 * globals.MB)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)

        // Send complete signal via channel
        isComplete <- true
    } ()

    // Format connection address for testing
    connectAddr := ":" + strconv.Itoa(listenerPort)

    // Make a connection to the remote brain server
    serverConn, err := net.Dial("tcp", connectAddr)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close connection on local exit
    defer serverConn.Close()

    // Create the input file and return handle
    inFilePath, inFile, err := disk.CreateRandFile(".", globals.RAND_STRING_SIZE,
                                                   "input_test", "txt", true)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Make buffer to hold random data and write random data to it
    writeBuffer := make([]byte, 20 * globals.MB)
    data.GenerateRandomBytes(writeBuffer, 20 * globals.MB)
    // Write the buffer of random data to file
    bytesWrote, err := inFile.Write(writeBuffer)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the number of bytes wrote equals the buffer size
    assert.Equal(20 * globals.MB, bytesWrote)
    // Close input file after writing data
    inFile.Close()

    // Transfer the file to the client
    err = netio.TransferFile(serverConn, inFilePath, int64(bytesWrote))
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Wait for the channel to send complete signal
    <-isComplete

    // Get the size of the output file
    outFileInfo, err := os.Stat(outFilePath)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Ensure the input and output files are the same size
    assert.Equal(int64(20 * globals.MB), outFileInfo.Size())

    deleteFiles := []string{inFilePath, outFilePath}

    // Iterate through list of test files and delete them
    for _, file := range deleteFiles {
        err = os.Remove(file)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
    }
}


func TestWriteHandler(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get available listener and its corresponding port
    listener, listenerPort := netio.GetAvailableListener()
    // Close listener on local exit
    defer listener.Close()

    isComplete := make(chan bool)
    testMessage := []byte("This is a test message that is used for testing purposes")
    testMessage2 := []byte("This is another test message that is used for testing")

    go func() {
        // Wait for an incoming connection
        clientConn, err := listener.Accept()
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
        // Close connection on local exit
        defer clientConn.Close()

        // Make buffer for receiving data
        receiveBuffer := make([]byte, 64)
        // Read the message from server and store into buffer
        bytesRead, err := clientConn.Read(receiveBuffer)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)

        // Ensure bytes read equals the expected message
        assert.Equal(len(testMessage), bytesRead)
        // Ensure content in the buffer matches expected message
        assert.Equal(testMessage, receiveBuffer[:len(testMessage)])

        // Perform write operation via passed in connection
        bytesWrote, err := netio.WriteHandler(clientConn, testMessage2,
                                              len(testMessage2))
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
        // Ensure bytes wrote equals the expected message
        assert.Equal(len(testMessage2), bytesWrote)

        // Send complete signal via channel
        isComplete <- true
    } ()

    // Format connection address for testing
    connectAddr := ":" + strconv.Itoa(int(listenerPort))

    // Make a connection to the remote brain server
    serverConn, err := net.Dial("tcp", connectAddr)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close connection on local exit
    defer serverConn.Close()

    // Perform write operation via passed in connection
    bytesWrote, err := netio.WriteHandler(serverConn, testMessage,
                                          len(testMessage))
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure bytes wrote equals the expected message
    assert.Equal(len(testMessage), bytesWrote)

    // Make buffer to receive data
    buffer := make([]byte, 64)
    // Read the message from server and store into buffer
    bytesRead, err := serverConn.Read(buffer)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Ensure bytes read equals the expected message length
    assert.Equal(len(testMessage2), bytesRead)
    // Ensure content in the buffer matches expected message
    assert.Equal(testMessage2, buffer[:len(testMessage2)])

    // Wait for the channel to send complete signal
    <-isComplete
}


// Tests both the UploadFile and ReceiveFile methods
func TestFileTransfer(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get available listener and its corresponding port
    listener, listenerPort := netio.GetAvailableListener()
    // Close listener on local exit
    defer listener.Close()

    isComplete := make(chan bool)
    var receivedPath string

    go func() {
        // Wait for an incoming connection
        clientConn, err := listener.Accept()
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
        // Close connection on local exit
        defer clientConn.Close()

        messageBuffer := make([]byte, globals.MESSAGE_BUFFER_SIZE)
        // Read data from the socket and write to the file path
        receivedPath, err = netio.ReceiveFile(clientConn, messageBuffer, ".",
                                              globals.LOG_TRANSFER_PREFIX)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)

        // Send complete signal via channel
        isComplete <- true
    } ()

    // Format connection address for testing
    connectAddr := ":" + strconv.Itoa(listenerPort)

    // Make a connection to the remote brain server
    serverConn, err := net.Dial("tcp", connectAddr)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close connection on local exit
    defer serverConn.Close()

    // Create the input file and return handle
    inFilePath, inFile, err := disk.CreateRandFile(".", globals.RAND_STRING_SIZE,
                                                   "input_test", "txt", true)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Make buffer to hold random data and write random data to it
    writeBuffer := make([]byte, 20 * globals.MB)
    data.GenerateRandomBytes(writeBuffer, 20 * globals.MB)
    // Write the buffer of random data to file
    bytesWrote, err := inFile.Write(writeBuffer)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the number of bytes wrote equals the buffer size
    assert.Equal(20 * globals.MB, bytesWrote)
    // Close input file after writing data
    inFile.Close()

    messageBuffer := make([]byte, globals.MESSAGE_BUFFER_SIZE)
    // Transfer the file to the client
    err = netio.UploadFile(serverConn, messageBuffer, inFilePath,
                           globals.LOG_TRANSFER_PREFIX)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Wait for the channel to send complete signal
    <-isComplete

    // Get the size of the output file
    outFileInfo, err := os.Stat(receivedPath)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Ensure the input and output files are the same size
    assert.Equal(int64(20 * globals.MB), outFileInfo.Size())

    deleteFiles := []string{inFilePath, receivedPath}

    // Iterate through list of test files and delete them
    for _, file := range deleteFiles {
        err = os.Remove(file)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
    }
}
