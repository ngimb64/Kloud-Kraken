package wordlist_test

import (
	"os"
	"testing"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"github.com/ngimb64/Kloud-Kraken/pkg/wordlist"
	"github.com/stretchr/testify/assert"
)

func TestCatAndDelete(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    file1Data := []byte("test\nstring\nfile\nmmmk\n")
    file2Data := []byte("foo\nbar\nsham\nshamar")

    // Create a random test file
    file1, err := os.CreateTemp("", "testfile")
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

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
    err = wordlist.CatAndDelete(&catFiles, catOutfile.Name())
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

    // Create a random test file
    file1, err := os.CreateTemp("", "testfile")
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

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
                                                      int64(104857600))
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


func TestGetBlockSize(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get the recommended block size
    blockSize, err := wordlist.GetBlockSize()
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the block size is greater than 0
    assert.Less(int(0), blockSize)
}


func TestReduceBlockSize(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get the recommended block size
    blockSize, err := wordlist.GetBlockSize()
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Reduce the block size to a reusable size
    blockSize = wordlist.ReduceBlockSize(int64(10), int64(blockSize))
    // Ensure the block size is is reduced to nearest binary chunk
    assert.Equal(8, blockSize)
}


func TestFileShaveDD(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Create a random test file for input data to shave
    inFile, err := os.CreateTemp("", "testfile")
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Set the size of random data
    randomDataSize := 20 * globals.MB
    // Make a byte buffer based off random data size
    buffer := make([]byte, randomDataSize)
    // Fill the buffer up with random data
    data.GenerateRandomBytes(buffer, randomDataSize)
    // Write the random data to the output file
    bytesWrote, err := inFile.Write(buffer)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close the file after data has been written
    inFile.Close()
    // Ensure the bytes wrote matches the buffer size
    assert.Equal(bytesWrote, randomDataSize)

    // Create a random test file for output shaved data
    shaveFile, err := os.CreateTemp("", "testfile")
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close the output file
    shaveFile.Close()

    // Create a random test file for original data
    originFile, err := os.CreateTemp("", "testfile")
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close the output file
    originFile.Close()

    // Get the recommended block size
    blockSize, err := wordlist.GetBlockSize()
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Shave exceeding half of wordlist into new file
    shaveFileSize, err := wordlist.FileShaveDD(inFile.Name(), shaveFile.Name(),
                                               originFile.Name(), blockSize,
                                               int64((10 * globals.MB)))
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Get the file info of orgiginal data before exceeding was shaved
    originFileInfo, err := os.Stat(originFile.Name())
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Ensure the input file is shaved down to half size
    assert.Equal(originFileInfo.Size(), int64(10 * globals.MB))
    // Ensure the input and output file sizes are equal
    assert.Equal(originFileInfo.Size(), shaveFileSize)

    deleteFiles := []string{shaveFile.Name(), originFile.Name()}

    // Iterate through resulting files and delete them
    for _,file := range deleteFiles {
        err = os.Remove(file)
        assert.Equal(nil, err)
    }
}


func TestFileShaveSplit(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Create a random test file for input data to shave
    inFile, err := os.CreateTemp("", "testfile")
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Set the size of random data
    randomDataSize := 21 * globals.MB
    // Make a byte buffer based off random data size
    buffer := make([]byte, randomDataSize)
    // Fill the buffer up with random data
    data.GenerateRandomBytes(buffer, randomDataSize)
    // Write the random data to the output file
    bytesWrote, err := inFile.Write(buffer)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close the file after data has been written
    inFile.Close()
    // Ensure the bytes wrote matches the buffer size
    assert.Equal(bytesWrote, randomDataSize)

    catFiles := []string{}
    outFilesMap := make(map[string]struct{})

    // Cut file into 10 2MB files and a 1MB overflow file
    wordlist.FileShaveSplit(inFile.Name(), "/tmp/testfile",
                            int64(2 * globals.MB),
                            &catFiles, outFilesMap)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Ensure proper number of output files
    assert.Equal(10, len(outFilesMap))
    // Ensure proper number of file to pass back into cat
    assert.Equal(1, len(catFiles))

    // Delete the file in cat files slice
    err = os.Remove(catFiles[0])
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Iterate through output files and delete them
    for fileName := range outFilesMap {
        err = os.Remove(fileName)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
    }
}


func TestMergeWordlistDir(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    dirPath := "testdir"
    // Create the test directory
    err := os.Mkdir(dirPath, os.ModePerm)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Copy the directory with test wordlist data to test dir
    err = os.CopyFS(dirPath, os.DirFS("../../testdata"))
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    maxFileSize := int64(20 * globals.MB)
    // Merge the created wordlists in the wordlist dir
    err = wordlist.MergeWordlistDir(dirPath, maxFileSize, 15.0,
                                    int64(75 * globals.GB))
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    dirItems, err := os.ReadDir(dirPath)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    fullFiles := []string{}
    shaveFiles := []string{}

    // Iterate through the items in the test dir
    for _, item := range dirItems {
        if item.IsDir() {
            continue
        }

        itemPath := dirPath + "/" + item.Name()

        // Get the current file info
        itemInfo, err := os.Stat(itemPath)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)

        // Get the current file size and ensure it is less than max
        fileSize := itemInfo.Size()
        assert.Less(fileSize, maxFileSize)

        // If the file is within 5 percent or equal to the max file size
        if data.IsInPercentRange(float64(maxFileSize), float64(fileSize), 5.0) ||
        fileSize == maxFileSize {
            fullFiles = append(fullFiles, itemPath)
        } else {
            shaveFiles = append(shaveFiles, itemPath)
        }
    }

    // Ensure there are 5 full files
    assert.Equal(5, len(fullFiles))
    // Ensure there is one leftover file
    assert.Equal(1, len(shaveFiles))

    // Get the size of the leftover file
    fileInfo, err := os.Stat(shaveFiles[0])
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the leftover file is expected size
    assert.Equal(int64(6369586), fileInfo.Size())

    // Delete test directory and its contents after test
    err = os.RemoveAll(dirPath)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
}


func TestRemoveMergeSubdirs(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    dirPath := "testdir"
    // Create the test directory
    err := os.Mkdir(dirPath, os.ModePerm)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Copy the directory with test wordlist data to test dir
    err = os.CopyFS(dirPath, os.DirFS("../../testdata"))
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    testDirs := []string{dirPath + "/" + "testdir1",
                         dirPath + "/" + "testdir2",
                         dirPath + "/" + "testdir3",
                         dirPath + "/" + "testdir4"}

    // Iterate through the test dirs and create them
    for _, testDir := range testDirs {
        err = os.Mkdir(testDir, os.ModePerm)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
    }

    // Delete any subdirectories just created leaving only files
    err = wordlist.RemoveMergeSubdirs(dirPath)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Read the items in the wordlist merge dir
    dirItems, err := os.ReadDir(dirPath)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Iterate through the items in the test dir and
    // ensure there are no subdirectories
    for _, item := range dirItems {
        assert.False(item.IsDir())
    }

    // Delete test directory and its contents after test
    err = os.RemoveAll(dirPath)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
}
