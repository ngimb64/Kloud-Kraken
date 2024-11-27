package data

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
)


func ParseFileSizeType(unitFileSize string) (float64, string, error) {
	// Iterate through the string slice of file types
	for _, unit := range globals.FILE_SIZE_TYPES {
		// If the current unit is within passed in string
		if strings.HasSuffix(unitFileSize, unit) {
			// Remove the unit from the end of the string
			sizeStr := strings.TrimSuffix(unitFileSize, unit)

			// Convert the remaining size string to 64-bit float
			size, err := strconv.ParseFloat(sizeStr, 64)
			if err != nil {
				return 0, "", fmt.Errorf("error converting string to float64: %v", err)
			}

			return size, unit, nil
		}
	}

	// If no units were found return error indicating unusual behavior, as this function
	// should have not been called without file units present in arg string
	return 0, "", fmt.Errorf("no valid unit found in arg file size string")
}


// Function that takes a slice, makes it immutable by copying it, and checks for a target
func StringSliceContains(slice []string, target string) bool {
    // Iterate over the copied slice and check for the target value
    for _, item := range slice {
		// If the current unit is in the target string
		if strings.Contains(target, item) {
            return true
        }
    }
    return false
}


// Function to convert different size units to bytes and return as int64
func ToBytes(size float64, unit string) int64 {
    var byteSize float64

    // Convert the size to bytes based on the unit
    switch unit {
    // Kilobytes
    case "KB":
        byteSize = size * 1024
    // Megabytes
    case "MB":
        byteSize = size * 1024 * 1024
    // Gigabytes
    case "GB":
        byteSize = size * 1024 * 1024 * 1024
    // Terabytes
    case "TB":
        byteSize = size * 1024 * 1024 * 1024 * 1024
    // Petabytes
    case "PB":
        byteSize = size * 1024 * 1024 * 1024 * 1024 * 1024
    // Exabytes
    case "EB":
        byteSize = size * 1024 * 1024 * 1024 * 1024 * 1024 * 1024
    // Invalid unit
    default:
        return -1
    }

    // Convert the result to int64 and return
    return int64(byteSize)
}


// TransferManager tracks the size of ongoing transfers.
type TransferManager struct {
	OngoingTransfersSize int64 // Total size of all ongoing transfers
}

// NewTransferManager initializes and returns a new TransferManager instance.
func NewTransferManager() *TransferManager {
	return &TransferManager{}
}

// AddTransferSize adds the specified size to the ongoing transfers.
func (tm *TransferManager) AddTransferSize(size int64) {
	atomic.AddInt64(&tm.OngoingTransfersSize, size)
}

// RemoveTransferSize subtracts the specified size from the ongoing transfers.
func (tm *TransferManager) RemoveTransferSize(size int64) {
	atomic.AddInt64(&tm.OngoingTransfersSize, -size)
}

// GetOngoingTransfersSize returns the current total size of ongoing transfers.
func (tm *TransferManager) GetOngoingTransfersSize() int64 {
	return atomic.LoadInt64(&tm.OngoingTransfersSize)
}
