package disk

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"golang.org/x/sys/unix"
)

// Package level variables
var SelectedFiles sync.Map		  // Global map to track selected files
var FileSelectionLock sync.Mutex  // Mutex for synchronizing the file selection


// AppendFile appends the contents of srcFile to destFile if the source file has data.
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

    return nil
}


func CheckDirFiles(path string) (string, int64, error) {
    var fileCount int
    var fileName string
    var fileSize int64

    // Read the contents of the directory
    files, err := os.ReadDir(path)
    if err != nil {
        return "", -1, err
    }

    // Loop over the directory contents
    for _, file := range files {
        // If the current item is a file
        if !file.IsDir() {
            fileCount++

            // If the first file is selected
            if fileCount == 1 {
                // Get the file name and size
                info, err := file.Info()
                if err != nil {
                    return "", -1, err
                }

                fileName = info.Name()
                fileSize = info.Size()
            }
        }
    }

    // If no files detected, return empty string
    if fileCount == 0 {
        return "", 0, nil
    } else if fileCount == 1 {
        // If there is one file, return the name and size
        return fileName, fileSize, nil
    } else {
        // If there is more than one file in dir
        return "<MULTI_FILE>", 0, nil
    }
}


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


// GetDiskSpace gets the total and available space on the root disk.
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


// Checks if the file or directory exists and ensure directories
// have contents in them based on file size or entries in dir.
//
// Parameters:
// - The path to check for existence
//
// Returns:
// - Boolean for the item existing and having content
// - Boolean for if the item is a directory
// - Error return handler
//
func PathExists(filePath string) (bool, bool, error) {
    // Get item info on passed in path
    itemInfo, err := os.Stat(filePath)
    if err != nil {
        // If the item does not exist
        if os.IsNotExist(err) {
            return false, false, nil
        }
        // If unexpected error getting item info
        return false, false, fmt.Errorf("error checking file existence - %w", err)
    }

    // If the path is a file, and has data in it
    if !itemInfo.IsDir() && itemInfo.Size() > 0 {
        return true, false, nil
    }

    // Open the directory
    dir, err := os.Open(filePath)
    if err != nil {
        return false, true, fmt.Errorf("error opening directory - %w", err)
    }
    // Close the directory on local exit
    defer dir.Close()

    // Attempt to read the first entry in the dir
    entries, err := dir.ReadDir(1)
    if err != nil {
        return false, true, fmt.Errorf("error reading directory - %w", err)
    }

    // If there is an entry, the dir is not empty
    if len(entries) > 0 {
        return true, true, nil
    }

    // If no entries the directory is empty
    return false, true, nil
}


// Function for each goroutine to walk the directory and select a file.
func SelectFile(loadDir string, maxFileSizeInt64 int64) (string, int64, error) {
    var returnPath string
    var returnSize int64
    done := false

    // Iterate through the file and folders in the load directory
    err := filepath.Walk(loadDir, func(path string, itemInfo os.FileInfo, err error) error {
        if err != nil {
            return err
        }

        // If a file has been already selected, skip to next iteration
        if done {
            return nil
        }

        // If the current item is not a directory, meaning a file
        if !itemInfo.IsDir() {
            // Lock selection process to ensure a single goroutine selects the file
            FileSelectionLock.Lock()
            // Unlock selection process on local exit
            defer FileSelectionLock.Unlock()

            // If the current file size is greater than the max file size set in YAML
            if itemInfo.Size() > maxFileSizeInt64 {
                return nil
            }

            // Check if the file has already been selected by another goroutine,
            // otherwise store the file path in the sync map
            _, loaded := SelectedFiles.LoadOrStore(path, true)
            // The file was already selected, so skip it
            if loaded {
                return nil
            }

            // Set the current file path as return path
            returnPath = path
            // Set the current file size as return size
            returnSize = itemInfo.Size()
            // Set the complete flag to true
            done = true
        }

        return nil
    })

    if err != nil {
        return "", 0, err
    }

    return returnPath, returnSize, nil
}
