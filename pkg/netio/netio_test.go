package netio_test

import (
	"net"
	"strconv"
	"testing"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"github.com/ngimb64/Kloud-Kraken/pkg/netio"
	"github.com/stretchr/testify/assert"
)

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

    go func() {
        // Wait for an incoming connection
        serverConn, err := listener.Accept()
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
        // Close connection on local exit
        defer serverConn.Close()
    } ()

    // Format connection address for testing
    connectAddr := ":" + strconv.Itoa(int(listenerPort))

    // Make a connection to the remote brain server
    clientConn, err := net.Dial("tcp", connectAddr)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close connection on local exit
    defer clientConn.Close()

    // Get the IP address and port from established connection
    ipAddr, port, err := netio.GetIpPort(clientConn)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Ensure the IP address is the local host
    assert.Equal("127.0.0.1", ipAddr)
    // Ensure the port is the listener port established by server
    assert.Equal(listenerPort, port)
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
