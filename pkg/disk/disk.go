package disk

import (
	"bytes"
	"fmt"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

const GB = 1024 * 1024 * 1024
var COLON_DELIMITER = []byte(":")
var START_TRANSFER_PREFIX = []byte("<START_TRANSFER:")
var START_TRANSFER_SUFFIX = []byte(">")


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
	readData = bytes.TrimPrefix(readData, START_TRANSFER_PREFIX)
	readData = bytes.TrimSuffix(readData, START_TRANSFER_SUFFIX)
	// Split the string by the colon delimiter
	dataBits := bytes.Split(readData, COLON_DELIMITER)
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


func DiskCheck() (uint64) {
	// Reserved space for the OS (10GB)
	var OSReservedSpace uint64 = 10 * GB

	// Get the total and available disk space
	total, free, err := GetDiskSpace()
	if err != nil {
		fmt.Println("Error getting disk space:", err)
		os.Exit(1)
	}

	fmt.Printf("Total disk space: %d GB\n", total/GB)
	fmt.Printf("Free disk space: %d GB\n", free/GB)

	// Subtract reserved space (for OS) from total space
	remainingSpace := total - OSReservedSpace

	return remainingSpace
}
