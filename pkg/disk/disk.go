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


// Checks if the file or directory exists and ensure directories
// have contents in them based on file size or entries in dir
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
		return false, false, fmt.Errorf("error checking file existence: %v", err)
	}

	// If the path is a file, and has data in it
	if !itemInfo.IsDir() && itemInfo.Size() > 0 {
		return true, false, nil
	}

	// Open the directory
	dir, err := os.Open(filePath)
	if err != nil {
		return false, true, fmt.Errorf("Error opening directory: %v", err)
	}
	// Close the directory on local exit
	defer dir.Close()

	// Attempt to read the first entry in the dir
	entries, err := dir.ReadDir(1)
	if err != nil {
		return false, true, fmt.Errorf("Error reading directory: %v", err)
	}

	// If there is an entry, the dir is not empty
	if len(entries) > 0 {
		return true, true, nil
	}

	// If no entries the directory is empty
	return false, true, nil
}


// Function for each goroutine to walk the directory and select a file
func SelectFile(rootDir string) (string, int64, error) {
	var returnPath string
	var returnSize int64
	done := false

	// Walking through the directory tree
	err := filepath.Walk(rootDir, func(path string, itemInfo os.FileInfo, err error) error {
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
			fileSelectionLock.Lock()
			// Unlock selection process on local exit
			defer fileSelectionLock.Unlock()

			// Check if the file has already been selected by another goroutine,
			// otherwise store the file path in the sync map
			_, loaded := selectedFiles.LoadOrStore(path, true)
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
