package disk

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"golang.org/x/sys/unix"
)

// Package level variables
var selectedFiles sync.Map		  // Global map to track selected files
var fileSelectionLock sync.Mutex  // Mutex for synchronizing the file selection


func DiskCheck() (int64) {
	// Reserved space for the OS (10GB)
	var OSReservedSpace int64 = 10 * globals.GB

	// Get the total and available disk space
	total, free, err := GetDiskSpace()
	if err != nil {
		fmt.Println("Error getting disk space:", err)
		os.Exit(1)
	}

	// TODO:  log this instead of print it
	fmt.Printf("Total disk space: %d GB\n", total/globals.GB)
	fmt.Printf("Free disk space: %d GB\n", free/globals.GB)

	// Subtract reserved space (for OS) from free space
	remainingSpace := free - OSReservedSpace

	return remainingSpace
}


// GetDiskSpace gets the total and available space on the root disk.
func GetDiskSpace() (total int64, free int64, err error) {
    var statfs unix.Statfs_t

    // Get the stats of the root filesystem ("/")
    err = unix.Statfs("/", &statfs)
    if err != nil {
        return 0, 0, fmt.Errorf("failed to get disk space: %v", err)
    }

    // Total space is (blocks * block size)
    total = statfs.Blocks * statfs.Bsize
    // Free space is (free blocks * block size)
    free = statfs.Bfree * statfs.Bsize

    return total, free, nil
}


// Function for each goroutine to walk the directory and select a file
func SelectFile(rootDir string) (string, int64, error) {
	var returnPath string
	var returnSize int64
	done := false

	// Walking through the directory tree
	err := filepath.Walk(rootDir, func(path string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// If a file has been already selected, skip to next iteration
		if done {
			return nil
		}

		// If the current item is not a directory, meaning a file
		if !fileInfo.IsDir() {
			// Lock selection process to ensure a single goroutine selects the file
			fileSelectionLock.Lock()
			// Unlock selection process on local exit
			defer fileSelectionLock.Unlock()

			// Check if the file has already been selected by another goroutine
			_, loaded := selectedFiles.LoadOrStore(path, true)
			// The file was already selected, so skip it
			if loaded {
				return nil
			}

			// Set the current file path as return path
			returnPath = path
			// Set the current file size as return size
			returnSize = fileInfo.Size()
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
