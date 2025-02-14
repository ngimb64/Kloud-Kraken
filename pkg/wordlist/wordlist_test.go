package wordlist_test

import (
	"os"
	"testing"

	"github.com/ngimb64/Kloud-Kraken/pkg/wordlist"
	"github.com/stretchr/testify/assert"
)

func TestCatAndDelete(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    file1Data := []byte("test\nstring\nfile\nmmmk\n")
    file2Data := []byte("foo\nbar\nsham\nshamar")
    fileNameMap := make(map[string]struct{})

    // Create a random test file
    file1, err := os.CreateTemp("", "testfile")
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Add the file name to unique map
    fileNameMap[file1.Name()] = struct{}{}

    // Write the data to the file
    bytesWrote, err := file1.Write(file1Data)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the bytes wrote matches the data length
    assert.Equal(len(file1Data), bytesWrote)
    // Close the file
    file1.Close()

    // Create a random test file
    file2, err := os.CreateTemp("", "testfile")
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Add the file name to unique map
    fileNameMap[file2.Name()] = struct{}{}

    // Write the data to the file
    bytesWrote, err = file2.Write(file2Data)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the bytes wrote matches the data length
    assert.Equal(len(file2Data), bytesWrote)
    // Close the file
    file2.Close()

    // Add the created files to the cat files
    catFiles := []string{file1.Name(), file2.Name()}

    // Create output file for cat command
    catOutfile, err := os.CreateTemp("", "catout")
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close the file as it will be handled by cat
    catOutfile.Close()

    // Execute the cat command that deletes the input files
    err = wordlist.CatAndDelete(&catFiles, catOutfile.Name(), fileNameMap)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Read the output from the output file
    output, err := os.ReadFile(catOutfile.Name())
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Delete the file after the data is read in memory
    err = os.Remove(catOutfile.Name())
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    assert.Equal([]byte("test\nstring\nfile\nmmmk\nfoo\nbar\nsham\nshamar"), output)
}


func TestDuplicutAndDelete(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    testData := []byte("test\nstring\nfile\nmmmk\nfoo\nbar\nsham\nshamar")
    fileNameMap := make(map[string]struct{})

    // Create a random test file
    file1, err := os.CreateTemp("", "testfile")
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Add the file name to unique map
    fileNameMap[file1.Name()] = struct{}{}

    // Write the data to the file
    bytesWrote, err := file1.Write(testData)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the bytes wrote matches the data length
    assert.Equal(len(testData), bytesWrote)

    // Write the same data to the file
    bytesWrote, err = file1.Write(testData)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the bytes wrote matches the data length
    assert.Equal(len(testData), bytesWrote)

    // Write the same data to the file
    bytesWrote, err = file1.Write(testData)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the bytes wrote matches the data length
    assert.Equal(len(testData), bytesWrote)
    // Close the file
    file1.Close()

    // Create output file for cat command
    duplicutOutFile, err := os.CreateTemp("", "duplicutout")
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close the file as it will be handled by duplicut
    duplicutOutFile.Close()

    // Execute the cat command that filters duplicates in files
    sizeComp, size, err := wordlist.DuplicutAndDelete(file1.Name(), duplicutOutFile.Name(),
                                                      int64(104857600), fileNameMap)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the size comparison is equal to less than indicator
    assert.Equal(int32(0), sizeComp)
    // Ensure the size is equal to the expected data
    assert.Equal(int64(53), size)
    // Delete the result file after test complete
    os.Remove(duplicutOutFile.Name())
}
