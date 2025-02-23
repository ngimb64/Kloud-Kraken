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
    // Get available listener and what port it is on
    listener, listenerPort := netio.GetAvailableListener()
    // Close listener on local exit
    defer listener.Close()

    isComplete := make(chan bool)

    go func() {
        // Create the input file and return handle
        outFilePath, outFile := disk.CreateRandFile(".", globals.RAND_STRING_SIZE,
                                                    "output_test", "txt", true)
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
    inFilePath, inFile := disk.CreateRandFile(".", globals.RAND_STRING_SIZE,
                                              "input_test", "txt", true)
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


// func TestFileToSocketHandler(t *testing.T) {
//     // Make reusable assert instance
//     assert := assert.New(t)
// }


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

    // Get the next avail able listener on random port
    testListener, port := netio.GetAvailableListener()
    // Ensure the port is a non-privileged
    assert.Greater(port, int32(1001))

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
    resFileName, resFileSize, err := netio.GetFileInfo(buffer, globals.START_TRANSFER_PREFIX)
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

    // Format listener port with parsed YAML data
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
    connectAddr := ":" + strconv.Itoa(int(listenerPort))

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
        {512 * globals.KB, 0, 4 * globals.KB},
        {50 * globals.MB, 0, 64 * globals.KB},
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


// func TestHandleTransferRecv(t *testing.T) {
//     // Make reusable assert instance
//     assert := assert.New(t)
// }


// func TestReadHandler(t *testing.T) {
//     // Make reusable assert instance
//     assert := assert.New(t)
// }


// func TestReceiveFile(t *testing.T) {
//     // Make reusable assert instance
//     assert := assert.New(t)
// }


func TestSocketToFileCopy(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    testFiles := []string{}
    // Get available listener and what port it is on
    listener, listenerPort := netio.GetAvailableListener()
    // Close listener on local exit
    defer listener.Close()

    isComplete := make(chan bool)

    go func() {
        // Create the input file and return handle
        outFilePath, outFile := disk.CreateRandFile(".", globals.RAND_STRING_SIZE,
                                                    "output_test", "txt", true)
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
        err = netio.SocketToFileCopy(outFile, clientConn, receiveBuffer)
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
    inFilePath, inFile := disk.CreateRandFile(".", globals.RAND_STRING_SIZE,
                                              "input_test", "txt", true)
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


// func TestSocketToFileHandler(t *testing.T) {
//     // Make reusable assert instance
//     assert := assert.New(t)
// }


// func TestTransferFile(t *testing.T) {
//     // Make reusable assert instance
//     assert := assert.New(t)
// }


// func TestUploadFile(t *testing.T) {
//     // Make reusable assert instance
//     assert := assert.New(t)
// }


// func TestWriteHandler(t *testing.T) {
//     // Make reusable assert instance
//     assert := assert.New(t)
// }
