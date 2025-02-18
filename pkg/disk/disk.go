package disk

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"unicode"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"golang.org/x/sys/unix"
)

// Package level variables
var SelectedFiles sync.Map		  // Global map to track selected files
var FileSelectionLock sync.Mutex  // Mutex for synchronizing the file selection


// AppendFile appends the contents of srcFile to destFile if the source file has data.
//
// @Parameters
// - sourceFilePath:  The source file whose data will be appended to the dest
// - destFilePath:  The destination file where the source files data will be appended
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func AppendFile(sourceFilePath string, destFilePath string) error {
    // Open the source file for reading
    sourceFile, err := os.Open(sourceFilePath)
    if err != nil {
        return fmt.Errorf("error opening source file - %w", err)
    }
    // Close source file on local exit
    defer sourceFile.Close()

    // Check if the source file is empty
    fileInfo, err := sourceFile.Stat()
    if err != nil {
        return fmt.Errorf("error retrieving file info - %w", err)
    }

    // If the file is empty, ignore appending
    if fileInfo.Size() == 0 {
        return nil
    }

    // Open the destination file for appending
    destFile, err := os.OpenFile(destFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return fmt.Errorf("error opening destination file - %w", err)
    }
    // Close destination file on local exit
    defer destFile.Close()

    // Copy the contents of the source file to the destination file
    _, err = io.Copy(destFile, sourceFile)
    if err != nil {
        return fmt.Errorf("error copying data - %w", err)
    }

    // Delete the original file
    err = os.Remove(sourceFilePath)
    if err != nil {
        return fmt.Errorf("error deleting souce file - %w", err)
    }

    return nil
}


// Reads the passed in path (dir) and attempts to get the first file,
// returning its name and size.
//
// @Parameters
// - path:  The path to the directory to attempt to read a file
//
// @Returns
// - The name of the retrieved file
// - The size of the retrieved file
// - Error if it occurs, otherwise nil on success
//
func CheckDirFiles(path string) (string, int64, error) {
    var fileName string
    var fileSize int64

    // Read the contents of the directory
    items, err := os.ReadDir(path)
    if err != nil {
        return "", -1, err
    }

    // Loop over the directory contents
    for _, item := range items {
        // If the current item is a directory
        if item.IsDir() {
            continue
        }

        // Get the file name and size
        info, err := item.Info()
        if err != nil {
            return "", -1, err
        }

        fileName = info.Name()
        fileSize = info.Size()
        break
    }

    // If no files detected, return empty string
    if fileSize < 1 {
        return "", 0, nil
    }

    // If there is one file, return the name and size
    return fileName, fileSize, nil
}


// Creates a random text file based on length of name and extension.
// Provides boolean toggle to specify whether file handle should stay open and
// returned or be closed and not be returned.
//
// @Parameters
// - dirPath:  The path to the directory where the file will be created
// - nameLen:  The number of random characters for the name
// - baseName:  The base of file name which random random chars will be appended
// - externsion:  The file extension to use (ex: "txt" leave out the .)
// - retHandler:  Boolean used to return the open file descriptor or not
//
// @Returns
// - The formatted path to the newly create random file
// - The open file handler of create file is retHandler is true
//
func CreateRandFile(dirPath string, nameLen int, baseName string,
                    extension string, retHandler bool) (string, *os.File) {
    var randoPath string
    var randoString string

    for {
        // Re-create a random size string based on passed on length
        randoString = data.RandStringBytes(nameLen)

        // If a base file name is specified
        if baseName != "" {
            // Format randomly generate string appended on base name
            randoString = baseName + randoString
        }

        // If no file extension specified
        if extension == "" {
            // Format generate string into path
            randoPath = dirPath + "/" + randoString
        // If there is a file extension to format
        } else {
            // Format generate string into path
            randoPath = dirPath + "/" + randoString + "." + extension
        }

        // Attempt to open the generated file, skip if it already exists
        file, err := os.OpenFile(randoPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
        if os.IsExist(err) {
            continue
        }

        // If the return handler is true, return open file
        if retHandler {
            return randoPath, file
        }

        // Close the file descriptor since not in use
        file.Close()
        return randoPath, nil
    }
}


// Checks the disk to get the total and available space.
// The reserved space for the OS is subtracted from the free.
//
// @Returns
// - The remaining space (free - OS_RESERVED)
// - The total space
// - Error if it occurs, otherwise nil on success
//
func DiskCheck() (int64, int64, error) {
    // Get the total and available disk space
    total, free, err := GetDiskSpace()
    if err != nil {
        return -1, -1, err
    }

    // Subtract reserved space (for OS) from free space
    remainingSpace := free - globals.OS_RESERVED_SPACE

    return remainingSpace, total, nil
}


// Get the recommended IO block size and convert it to int.
//
// @Returns
// - The recommended block size
// - Error if it occurs, otherwise nil on success
//
func GetBlockSize() (int, error) {
    var blockSize int

    // Format command to get recommended block size
    cmd := exec.Command("sh", "-c", "stat / | grep 'IO Block:' | cut -d':' -f4 | cut -d' ' -f2")

    // Execute command to get block size
    byteBlockSize, err := cmd.Output()
    if err != nil {
        return 0, err
    }

    // Iterate through the range of bytes in slice
    for _, b := range byteBlockSize {
        // If the byte rune is a digit
        if unicode.IsDigit(rune(b)) {
            // Convert from byte ('0' to '9') to int
            blockSize = blockSize*10 + int(b-'0')
        }
    }

    return blockSize, nil
}


// Gets the total and available space on the root disk.
//
// @Returns
// - The total space on disk
// - the available free space
// - Error if it occurs, otherwise nil on success
//
func GetDiskSpace() (total int64, free int64, err error) {
    var statfs unix.Statfs_t

    // Get the stats of the root filesystem ("/")
    err = unix.Statfs("/", &statfs)
    if err != nil {
        return 0, 0, fmt.Errorf("failed to get disk space - %w", err)
    }

    // Total space is (blocks * block size)
    total = int64(statfs.Blocks) * statfs.Bsize
    // Free space is (free blocks * block size)
    free = int64(statfs.Bfree) * statfs.Bsize

    return total, free, nil
}


// Creates the slice of directories passed in.
//
// @Parameters
// - programDirs:  The slice of directories to be created
//
func MakeDirs(programDirs []string) {
    // Iterate through slice of dirs
    for _, dir := range programDirs {
        // Create the current dir and any missing parent dirs
        err := os.MkdirAll(dir, os.ModePerm)
        if err != nil {
            log.Fatalf("Error creating directory:  %v", dir)
        }
    }
}


// Checks if the file or directory exists and ensure directories
// have contents in them based on file size or entries in dir.
//
// @Parameters
// - The path to check for existence
//
// @Returns
// - Boolean for the item existing and having content
// - Boolean for if the item is a directory
// - Boolean for if the file or dir contains data
// - Error if it occurs, otherwise nil on success
//
func PathExists(filePath string) (bool, bool, bool, error) {
    // Get item info on passed in path
    itemInfo, err := os.Stat(filePath)
    if err != nil {
        // If unexpected error getting item info
        return false, false, false, fmt.Errorf("error checking file existence - %w", err)
    }

    // If the path is a file and has data
    if !itemInfo.IsDir() && itemInfo.Size() > 0 {
        return true, false, true, nil
    // If the path is a empty file
    } else if !itemInfo.IsDir() && itemInfo.Size() == 0 {
        return true, false, false, nil
    }

    // Open the directory
    dir, err := os.Open(filePath)
    if err != nil {
        return true, true, false, fmt.Errorf("error opening directory - %w", err)
    }
    // Close the directory on local exit
    defer dir.Close()

    // Attempt to read the first entry in the dir
    _, err = dir.ReadDir(1)
    if err != nil {
        return true, true, false, fmt.Errorf("error reading directory - %w", err)
    }

    // If there is an entry, the dir is not empty
    return true, true, true, nil

}


// Function for each goroutine to walk the directory and select a unique file.
//
// @Parameters
// - loadDir:  The directory to attempt to select a file
// - maxFileSizeInt64:  The max file size to ensure any violators are not selected
//
// @Returns
// - Path of the selected file
// - Size of the selected file
// - Error if it occurs, otherwise nil on success
//
func SelectFile(loadDir string, maxFileSizeInt64 int64) (string, int64, error) {
    var returnPath string
    var returnSize int64

    // Read the contents of the directory
    items, err := os.ReadDir(loadDir)
    if err != nil {
        return "", 0, err
    }

    // Iterate through the items in the load dir
    for _, item := range items {
        if item.IsDir() {
            continue
        }

        // Lock selection process to ensure a single goroutine selects the file
        FileSelectionLock.Lock()
        // Unlock selection process on local exit
        defer FileSelectionLock.Unlock()

        // Format the current file path
        itemPath := loadDir + "/" + item.Name()

        // Get the file statistics for the current file
        itemInfo, err := os.Stat(itemPath)
        if err != nil {
            continue
        }

        // If the current file size is greater than the max file size set in YAML
        if itemInfo.Size() > maxFileSizeInt64 {
            continue
        }

        // Check if the file has already been selected by another goroutine,
        // otherwise store the file path in the sync map
        _, loaded := SelectedFiles.LoadOrStore(itemPath, true)
        // The file was already selected, so skip it
        if loaded {
            continue
        }

        returnPath = itemPath
        returnSize = itemInfo.Size()
        break
    }

    return returnPath, returnSize, nil
}
