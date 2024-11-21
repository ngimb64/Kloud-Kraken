package disk

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"golang.org/x/sys/unix"
)

// Package level variables
var selectedFiles sync.Map		  // Global map to track selected files
var fileSelectionLock sync.Mutex  // Mutex for synchronizing the file selection


func DiskCheck() (uint64) {
	// Reserved space for the OS (10GB)
	var OSReservedSpace uint64 = 10 * globals.GB

	// Get the total and available disk space
	total, free, err := GetDiskSpace()
	if err != nil {
		fmt.Println("Error getting disk space:", err)
		os.Exit(1)
	}

	fmt.Printf("Total disk space: %d GB\n", total/globals.GB)
	fmt.Printf("Free disk space: %d GB\n", free/globals.GB)

	// Subtract reserved space (for OS) from total space
	remainingSpace := total - OSReservedSpace

	return remainingSpace
}


// GetDiskSpace gets the total and available space on the root disk.
func GetDiskSpace() (total uint64, free uint64, err error) {
    var statfs unix.Statfs_t

    // Get the stats of the root filesystem ("/")
    err = unix.Statfs("/", &statfs)
    if err != nil {
        return 0, 0, fmt.Errorf("failed to get disk space: %v", err)
    }

    // Total space is (blocks * block size)
    total = statfs.Blocks * uint64(statfs.Bsize)
    // Free space is (free blocks * block size)
    free = statfs.Bfree * uint64(statfs.Bsize)

    return total, free, nil
}


// Parse out the file name and size from the delimiter
// sent from remote brain server.
//
// Parameters:
// - readData:  The data read from socket buffer to be parsed.
//
// Returns:
// - The byte slice with the file name
// - A integer file size
// - Either nil on success or a string error message on failure
//
func GetFileInfo(readData []byte) ([]byte, int, any) {
	// Trim the delimiters around the file info
	readData = bytes.TrimPrefix(readData, globals.START_TRANSFER_PREFIX)
	readData = bytes.TrimSuffix(readData, globals.START_TRANSFER_SUFFIX)
	// Split the string by the colon delimiter
	dataBits := bytes.Split(readData, globals.COLON_DELIMITER)
	// Extract the filename and size from bits
	fileName := dataBits[0]
	fileSizeStr := dataBits[1]

	// Convert the size string to an integr
	fileSize, err := strconv.Atoi(string(fileSizeStr))
	// If the string integer failed to convert back to its native type
	if err != nil {
		fmt.Println("Error converting size string to int:", err)
		return fileName, fileSize, "Error occured during file size coversion"
	}

	return fileName, fileSize, nil
}


// Function for each goroutine to walk the directory and select a file
func SelectFile(rootDir string) (any, error) {
	var returnPath string
	done := false

	// Walking through the directory tree
	err := filepath.Walk(rootDir, func(path string, fileInfo os.FileInfo, err error) error {
		// If there is an error walking an item in the path
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
			if loaded {
				// The file was already selected, skip this file
				return nil
			}

			// Set the current path the return path
			returnPath = path
			// Set the complete flag to true
			done = true
		}

		return nil
	})

	// If an error occurred calling Walk function
	if err != nil {
		return nil, err
	}

	return returnPath, nil
}
