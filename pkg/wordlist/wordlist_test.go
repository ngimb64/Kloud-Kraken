package wordlist_test

import (
	"os"
	"testing"

	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
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

    testData := []byte("test\nstring\nfile\nmmmk\nfoo\nbar\nsham\nshamar\n")
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
    assert.Equal(int64(len(testData)), size)

    // Delete the result file after test complete
    err = os.Remove(duplicutOutFile.Name())
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
}


func TestReduceBlockSize(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get the recommended block size
    blockSize, err := disk.GetBlockSize()
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Reduce the block size to a reusable size
    blockSize = wordlist.ReduceBlockSize(int64(10), int64(blockSize))
    // Ensure the block size is is reduced to nearest binary chunk
    assert.Equal(8, blockSize)
}


// func TestFileShaveDD(t *testing.T) {
//     // Make reusable assert instance
//     assert := assert.New(t)

//     // Create a random test file for input data to shave
//     inFile, err := os.CreateTemp("", "testfile")
//     // Ensure the error is nil meaning successful operation
//     assert.Equal(nil, err)

//     // Set the random data size
//     randomDataSize := 20 * globals.MB
//     // Make a byte buffer based off iteration index
//     buffer := make([]byte, randomDataSize)
//     // Fill the buffer up with random data
//     data.GenerateRandomBytes(buffer, randomDataSize)
//     // Write the random data to the output file
//     bytesWrote, err := inFile.Write(buffer)
//     // Ensure the error is nil meaning successful operation
//     assert.Equal(nil, err)
//     // Close the file after data has been written
//     inFile.Close()
//     // Ensure the bytes wrote matches the buffer size
//     assert.Equal(bytesWrote, randomDataSize)

//     // Create a random test file for output shaved data
//     outFile, err := os.CreateTemp("", "testfile")
//     // Ensure the error is nil meaning successful operation
//     assert.Equal(nil, err)
//     // Close the output file
//     outFile.Close()

//     // Get the recommended block size
//     blockSize, err := disk.GetBlockSize()
//     // Ensure the error is nil meaning successful operation
//     assert.Equal(nil, err)
//     // Shave exceeding half of wordlist into new file
//     err = wordlist.FileShaveDD(inFile.Name(), outFile.Name(),
//                          blockSize, int64((10 * globals.MB)/blockSize))
//     // Ensure the error is nil meaning successful operation
//     assert.Equal(nil, err)

//     // Get the file info of input file
//     inFileInfo, err := os.Stat(inFile.Name())
//     // Ensure the error is nil meaning successful operation
//     assert.Equal(nil, err)

//     // Get the file info of the resulting output file
//     outFileInfo, err := os.Stat(outFile.Name())
//     // Ensure the error is nil meaning successful operation
//     assert.Equal(nil, err)

//     // Ensure the input file is shaved down to half size
//     assert.Equal(inFileInfo.Size(), int64(10 * globals.MB))
//     // Ensure the input and output file sizes are equal
//     assert.Equal(inFileInfo.Size(), outFileInfo.Size())
// }
