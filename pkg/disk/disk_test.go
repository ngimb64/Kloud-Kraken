package disk_test

import (
	"fmt"
	"os"
	"path/filepath"
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
        assert.Equal(nil, err)
        // Write the test strings to current file
        file.Write([]byte(testData[index]))
        // Close the file after data is written
        file.Close()
    }

    // Append the data in the source file to the destination
    err := disk.AppendFile(testFiles[0], testFiles[1])
    assert.Equal(nil, err)

    // Open the resulting file to read the data
    resultFile, err := os.Open(testFiles[1])
    assert.Equal(nil, err)

    readBuffer := make([]byte, 64)

    // Read the data from the resulting file
    bytesRead, err := resultFile.Read(readBuffer)
    assert.Equal(nil, err)
    // Close the resulting file
    resultFile.Close()
    // Ensure the message in the buffer is equal to the two files appended
    assert.Equal(readBuffer[:bytesRead],
                []byte("for testing purposes onlyThese strings are"))

    // Delete the resulting file
    err = os.Remove(testFiles[1])
    assert.Equal(nil, err)
}


func TestCheckDirFiles(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get the current working directory
    path, err := os.Getwd()
    assert.Equal(nil, err)

    testPath := fmt.Sprintf("%s/../data", path)
    // Get the first file name and size if there are files
    fileName, fileSize, err := disk.CheckDirFiles(testPath)
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
    assert.Equal(nil, err)

    // Create a random file and return the path
    filePath, _, err := disk.CreateRandFile(path, globals.RAND_STRING_SIZE,
                                            "kloudkraken-data", "", false)
    assert.Equal(nil, err)
    // Check to see if the file exists
    exists, isDir, hasData, err := disk.PathExists(filePath)
    assert.Equal(nil, err)
    // Ensure the file path exists
    assert.True(exists)
    // Ensure the path is not a directory
    assert.False(isDir)
    // Ensure the dummy file is empty
    assert.False(hasData)

    // Delete the file after testing
    err = os.Remove(filePath)
    assert.Equal(nil, err)
}


func TestGetDiskSpace(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get the total and free disk space
    total, free, err := disk.GetDiskSpace("/")
    assert.Equal(nil, err)
    // Ensure the total size is greater than 0
    assert.Less(int64(0), total)
    // Ensure the free size is greater than 0
    assert.Less(int64(0), free)
}


func TestGetProjectRootDir(t *testing.T) {
    // Make reusable assert instance
	assert := assert.New(t)

	// Create temp root directory
	tmpRoot := t.TempDir()
	// Create marker file (go.mod) in tmpRoot so it will be recognized as root
	markerPath := filepath.Join(tmpRoot, "go.mod")
	err := os.WriteFile(markerPath, []byte("module example.com/fake"), 0644)
	assert.Nil(err)
	// Create a nested working directory under tmpRoot and chdir into it
	nested := filepath.Join(tmpRoot, "cmd", "kloud-kraken")
	err = os.MkdirAll(nested, 0755)
	assert.Nil(err)

	// Save current working directory and restore at the end of the test
	origWd, err := os.Getwd()
	assert.Nil(err)
	defer func() {
		_ = os.Chdir(origWd)
	}()

	// Change to the nested directory
	err = os.Chdir(nested)
	assert.Nil(err)

	// Ensure KRAKEN_ROOT env var won't override detection
	origEnv := os.Getenv("KRAKEN_ROOT")
	_ = os.Unsetenv("KRAKEN_ROOT")
	defer func() {
		// restore original env var (empty or previous)
		_ = os.Setenv("KRAKEN_ROOT", origEnv)
	}()

	projectRoot := disk.GetProjectRootDir()

	// Expected absolute path of the temp root
	expectedRoot, err := filepath.Abs(tmpRoot)
	assert.Nil(err)

	// Assert the detection found the tmpRoot
	assert.Equal(expectedRoot, projectRoot)
}


func TestMakeDirs(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Get the current working directory
    path, err := os.Getwd()
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
        assert.Equal(nil, err)
    }
}


func TestPathExists(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    existentFile := "../data/data.go"
    // Check to see if the path exists, dir or file, and if it has data
    exists, isDir, hasData, err := disk.PathExists(existentFile)
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
    assert.Equal(nil, err)
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
    assert.Equal(nil, err)

    // Format test dir in current work dir path
    realDirPath := fmt.Sprintf("%s/testdir", path)
    // Create the test dir where test files will be made
    err = os.Mkdir(realDirPath, os.ModePerm)
    assert.Equal(nil, err)

    testFiles := []string{"test1.txt", "test2.txt", "test3.txt",
                          "test4.txt", "test5.txt"}
    bufferSizes := []int{256, 512, 1024, 2046, 4096}
    selectedNames := make(map[string]bool)

    // Iterate through slice of test file names
    for index, testFile := range testFiles {
        // Format the file path with test dir
        filePath := fmt.Sprintf("%s/%s", realDirPath, testFile)
        // Open the file with write permissions
        file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)
        assert.Equal(nil, err)

        // Make a byte buffer based off iteration index
        buffer := make([]byte, bufferSizes[index])
        // Fill the buffer up with random data
        data.GenerateRandomBytes(buffer, bufferSizes[index])
        // Write the random data to the output file
        bytesWrote, err := file.Write(buffer)
        assert.Equal(nil, err)
        // Close the file after data has been written
        file.Close()
        // Ensure the bytes wrote matches the buffer size
        assert.Equal(bytesWrote, bufferSizes[index])
    }

    for range testFiles {
        // Attempt to select a file with proper max size
        filePath, fileSize, err := disk.SelectFile(realDirPath,
                                                   int64(100*globals.MB))
        assert.Equal(nil, err)

        // Ensure a file was selected
        assert.NotEqual("", filePath)
        assert.Less(int64(0), fileSize)

        // Extract just the filename
        name := filepath.Base(filePath)
        // Ensure filename has NOT already been returned
        _, exists := selectedNames[name]
        assert.False(exists, "duplicate filename selected - %s", name)
        // If the file name is not in map, add it
        selectedNames[name] = true
    }

    // Delete the testdir and its contents
    err = os.RemoveAll(realDirPath)
    assert.Equal(nil, err)
}
