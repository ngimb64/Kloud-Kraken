package disk_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"github.com/stretchr/testify/assert"
)


func TestAppendFile(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    testData := []string{"These strings are", "for testing purposes only"}
    testFiles := []string{"test1.txt", "test2.txt"}

    // Iterate through slice of test files
    for index, fileName := range testFiles {
        // Create the current file
        file, err := os.Create(fileName)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
        // Write the test strings to current file
        file.Write([]byte(testData[index]))
        // Close the file after data is written
        file.Close()
    }

    // Append the data in the source file to the destination
    err := disk.AppendFile(testFiles[0], testFiles[1])
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Open the resulting file to read the data
    resultFile, err := os.Open(testFiles[1])
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    readBuffer := make([]byte, 64)

    // Read the data from the resulting file
    bytesRead, err := resultFile.Read(readBuffer)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close the resulting file
    resultFile.Close()
    // Ensure the message in the buffer is equal to the two files appended
    assert.Equal(readBuffer[:bytesRead],
                []byte("for testing purposes onlyThese strings are"))

    // Delete the resulting file
    err = os.Remove(testFiles[1])
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
}


func TestCheckDirFiles(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get the current working directory
    path, err := os.Getwd()
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    testPath := fmt.Sprintf("%s/../data", path)
    // Get the first file name and size if there are files
    fileName, fileSize, err := disk.CheckDirFiles(testPath)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the file has a name
    assert.NotEqual("", fileName)
    // Ensure the file has a size
    assert.Less(int64(0), fileSize)
}


func TestCreateRandFile(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get the current working directory
    path, err := os.Getwd()
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Create a random file and return the path
    filePath, _ := disk.CreateRandFile(path, globals.RAND_STRING_SIZE,
                                       "kloudkraken-data", "", false)

    // Check to see if the file exists
    exists, isDir, hasData, err := disk.PathExists(filePath)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the file path exists
    assert.True(exists)
    // Ensure the path is not a directory
    assert.False(isDir)
    // Ensure the dummy file is empty
    assert.False(hasData)

    // Delete the file after testing
    err = os.Remove(filePath)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
}


func TestDiskCheck(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get the remaining and total space on disk
    remaining, total, err := disk.DiskCheck()
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the remaining and total are greater than 0
    assert.Less(int64(0), remaining)
    assert.Less(int64(0), total)
}


func TestGetBlockSize(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get the recommended block size
    blockSize, err := disk.GetBlockSize()
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the block size is greater than 0
    assert.Less(int(0), blockSize)
}


func TestGetDiskSpace(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get the total and free disk space
    total, free, err := disk.GetDiskSpace()
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the total size is greater than 0
    assert.Less(int64(0), total)
    // Ensure the free size is greater than 0
    assert.Less(int64(0), free)
}


func TestMakeDirs(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get the current working directory
    path, err := os.Getwd()
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    testDirs := []string{fmt.Sprintf("%s/%s", path, "testdir1"),
                         fmt.Sprintf("%s/%s", path, "testdir2"),
                         fmt.Sprintf("%s/%s", path, "testdir3")}
    // Create each dir in slice
    disk.MakeDirs(testDirs)

    // Iterate through the slice of dirs
    for _, dir := range testDirs {
        // Check to see if the dir path exists
        exists, isDir, hasData, _ := disk.PathExists(dir)
        // Ensure dir exists
        assert.True(exists)
        // Ensure dir is a dir
        assert.True(isDir)
        // Ensure dir has no data
        assert.False(hasData)
        // Delete the dir after testing
        err = os.Remove(dir)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
    }
}


func TestPathExists(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    existentFile := "../data/data.go"
    // Check to see if the path exists, dir or file, and if it has data
    exists, isDir, hasData, err := disk.PathExists(existentFile)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure the file exists
    assert.True(exists)
    // Ensure the file is not a dir
    assert.False(isDir)
    // Ensure the file has data as it is part of the project
    assert.True(hasData)

    nonExistentFile := "skdlvskldnld"
    // Check to see if the path exists, dir or file, and if it has data
    exists, isDir, hasData, err = disk.PathExists(nonExistentFile)
    // Ensure error is present since path does not exist
    assert.NotEqual(nil, err)
    // Ensure the file does not exist
    assert.False(exists)
    // The file is neither a file or a dir
    assert.False(isDir)
    // The file does not have data
    assert.False(hasData)
}


func TestSelectFile(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    fakeDir := "dkvskdnvsdkvk"
    // Set max file size to 100 MB
    maxFileSize := int64(104857600)

    // Attempt to select from non-existent dir
    _, _, err := disk.SelectFile(fakeDir, maxFileSize)
    // Ensure the error present since dir path is fake
    assert.NotEqual(nil, err)

    // Get the current working directory
    path, err := os.Getwd()
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Format test dir in current work dir path
    realDirPath := fmt.Sprintf("%s/testdir", path)
    // Create the test dir where test files will be made
    err = os.Mkdir(realDirPath, os.ModePerm)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    testFiles := []string{"test1.txt", "test2.txt", "test3.txt",
                          "test4.txt", "test5.txt"}
    bufferSizes := []int{256, 512, 1024, 2046, 4096}

    // Iterate through slice of test file names
    for index, testFile := range testFiles {
        // Format the file path with test dir
        filePath := fmt.Sprintf("%s/%s", realDirPath, testFile)
        // Open the file with write permissions
        file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)

        // Make a byte buffer based off iteration index
        buffer := make([]byte, bufferSizes[index])
        // Fill the buffer up with random data
        data.GenerateRandomBytes(buffer, bufferSizes[index])
        // Write the random data to the output file
        bytesWrote, err := file.Write(buffer)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
        // Close the file after data has been written
        file.Close()
        // Ensure the bytes wrote matches the buffer size
        assert.Equal(bytesWrote, bufferSizes[index])
    }

    // Attempt to select a file with proper max size
    filePath, fileSize, err := disk.SelectFile(realDirPath, int64(100 * globals.MB))
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Ensure a file path was selected
    assert.NotEqual("", filePath)
    // Ensure the file size is greater than zero
    assert.Less(int64(0), fileSize)

    // Delete the testdir and its contents
    err = os.RemoveAll(realDirPath)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
}
