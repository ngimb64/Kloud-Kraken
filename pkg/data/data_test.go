package data_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"github.com/stretchr/testify/assert"
)


func TestGenerateRandomBytes(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Create buffer and generate half with random bytes
    buffer := make([]byte, 128)
    data.GenerateRandomBytes(buffer, 128)

    //Get the current working directory
    path, err := os.Getwd()
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Create a random file for testing
    testFile, file, err := disk.CreateRandFile(path, globals.RAND_STRING_SIZE,
                                               "kloudkraken-data", "", true)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Write the random bytes to file
    bytesWrote, err := file.Write(buffer)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the bytes wrote equals random generated in buffer
    assert.Equal(128, len(buffer[:bytesWrote]))
    // Close the file
    file.Close()

    // Get the size of the file
    fileInfo, err := os.Stat(testFile)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the file size equals random generate in buffer
    assert.Equal(int64(128), fileInfo.Size())
    // Delete the file
    err = os.Remove(testFile)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
}


func TestIsInPercentRange(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    tests := []struct {
        maxSize	 float64
        fileSize float64
        percent  float64
    } {
        {100.0 * globals.GB, 87.0 * globals.GB, 20.0},
        {512.0 * globals.MB, 480.0 * globals.MB, 25.0},
        {56.0 * globals.KB, 54.0 * globals.KB, 15.0},
    }

    // Iterate through slice of struct and pass its members into function
    for _, test := range tests {
        assert.True(data.IsInPercentRange(test.maxSize,
                                          test.fileSize,
                                          test.percent))
    }

    tests = []struct {
        maxSize	 float64
        fileSize float64
        percent  float64
    } {
        {256.0 * globals.MB, 123.0 * globals.MB, 10.0},
        {64.0 * globals.KB, 16.0 * globals.KB, 20.0},
        {480.0 *globals.GB, 10.0 * globals.GB, 35.0},
    }

    // Iterate through slice of struct and pass its members into function
    for _, test := range tests {
        assert.False(data.IsInPercentRange(test.maxSize,
                                           test.fileSize,
                                           test.percent))
    }
}


func TestParseFileSizeType(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)
    sizes := []int{256, 512, 64}

    // Iterate through sizes slice
    for index, size := range sizes {
        // Format size with size unit appended
        arg := fmt.Sprintf("%d%s", size, globals.FILE_SIZE_TYPES[index])
        // Run test with current args
        size, unit, _ := data.ParseFileSizeType(arg)
        // Ensure the result size and unit match the original
        assert.Equal(float64(sizes[index]), size)
        assert.Equal(globals.FILE_SIZE_TYPES[index], unit)
    }
}


func TestRandStringBytes(t *testing.T) {
    stringLen := 12
    // Ensure a dozen random bytes are returned as a string
    assert.Equal(t, stringLen, len(data.RandStringBytes(stringLen)))
}


func TestSliceToCsv(t *testing.T) {
    testSlice := []string{"foo", "bar", "shazam", "shamar"}
    // Make reusable assert instance
    assert := assert.New(t)
    // Convert the test data to CSV string
    resultCsv, err := data.SliceToCsv(testSlice)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the resulting CSV string is of proper format
    assert.Equal("foo,bar,shazam,shamar", resultCsv)
}


func TestStringSliceContains(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)
    testSlice := []string{"fee", "fi", "fo", "fum"}

    // Set true values and test them in loop
    trues := []string{"blahfiblah", "blahfum", "feeblah"}

    for _, truth := range trues {
        assert.True(data.StringSliceContains(testSlice, truth))
    }

    // Set false values and test them in loop
    falses := []string{"dxrz4", "flapjacks", "doug funny"}

    for _, falacy := range falses {
        assert.False(data.StringSliceContains(testSlice, falacy))
    }
}


func TestStringSliceHasItem(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)
    testSlice := []string{"fee", "fi", "fo", "fum"}

    // Set true values and test them in loop
    trues := []string{"fi", "fum", "fee"}

    for _, truth := range trues {
        assert.True(data.StringSliceHasItem(testSlice, truth))
    }

    // Set false values and test them in loop
    falses := []string{"dxrz4", "flapjacks", "doug funny"}

    for _, falacy := range falses {
        assert.False(data.StringSliceHasItem(testSlice, falacy))
    }
}


func TestToBytes(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)
    // Set up data slices
    sizes := []float64{4192.0, 1024.0, 64.0}
    units := []string{"KB", "MB", "GB"}
    results := []float64{globals.KB, globals.MB, globals.GB}

    // Iterate sizes slice using index to access corresponding items of other slices
    for index, size := range sizes {
        assert.Equal(data.ToBytes(size, units[index]), int64(size * results[index]))
    }
}


func TestTransferManager(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)
    // Create and initialize new transfer manager
    tMan := data.NewTransferManager()
    // Add data to the transfer manager
    tMan.AddTransferSize(4096)
    // Ensure the size of size matches prior added data
    assert.Equal(int64(4096), tMan.GetOngoingTransfersSize())
    // Remove half the size of the data added
    tMan.RemoveTransferSize(2048)
    // Ensure the size matches half the size of original data
    assert.Equal(int64(2048), tMan.GetOngoingTransfersSize())
    // Remove the remaining data
    tMan.RemoveTransferSize(2048)
    // Ensure the transfer manage is empty
    assert.Equal(int64(0), tMan.GetOngoingTransfersSize())
}


func TestTrimAfterLast(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    tests := []struct {
        input     []byte
        delimiter []byte
        output    []byte
    } {
        {[]byte("=> test => string => result"), []byte("=> "), []byte("result")},
        {[]byte("test-string-hyphen-delimited"), []byte("-"), []byte("delimited")},
        {[]byte("test<SPACE>string<SPACE>result"), []byte("<SPACE>"), []byte("result")},
    }

    // Iterate through slice of test structs
    for _, test := range tests {
        // Use struct members as input to call function and trim after last delimiter
        output, _ := data.TrimAfterLast(test.input, test.delimiter)
        // Compare expected output to function output
        assert.Equal(test.output, output)
    }
}


func TestTrimEndChars(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    tests := []struct {
        buffer []byte
        char   byte
        output []byte
    } {
        {[]byte("test.."), byte('.'), []byte("test")},
        {[]byte("string."), byte('.'), []byte("string")},
        {[]byte("foo...."), byte('.'), []byte("foo")},
    }

    // Iterate through slice of test structs
    for _, test := range tests {
        // Use struct members as input to call function and trim chars on end
        output := data.TrimEndChars(test.buffer, test.char)
        // Compare expected output to function output
        assert.Equal(test.output, output)
    }
}
