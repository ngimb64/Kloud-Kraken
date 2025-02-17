package data

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"golang.org/x/exp/rand"
)

// Packagre level variables
const LetterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"


// Populate passed in buffer with random bytes of data.
//
// @Parameters
// - buffer:  The buffer where the random bytes of data will be written
// - maxBytes:  The max amount of bytes of data to store in buffer
//
func GenerateRandomBytes(buffer []byte, maxBytes int) {
    // Seed the random number generator to ensure unique results
    rand.Seed(uint64(time.Now().UnixNano()))

    for i := range buffer {
        buffer[i] = byte(rand.Intn(maxBytes))
    }
}


// Check if the size is within the percentage range of the max size.
//
// @Parameters
// - maxSize:  The max allowed file size
// - currentSize:  The current size to compare to max range
// - percent:  The upper allowed percentange within max size
//
// @Returns
// - true/false boolean whether the size is within upper max range
//
func IsInPercentRange(maxSize float64, currentSize float64,
                      percent float64) bool {
    // Calculate the margin based on the percentage
    margin := (percent / 100) * maxSize

    // Calculate the lower and upper bounds
    lowerBound := maxSize - margin
    upperBound := maxSize

    // Check if the file size is within the range
    return currentSize >= lowerBound && currentSize <= upperBound
}


// Split the file size from its unit and return to different variables
//
// @Parameters
// - unitFileSize:  The file size and unit as one string
//
// @Returns
// - The parsed file size converted to float type
// - The parsed file size unit
// - Error if it occurs, otherwise nil on success
//
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
                return 0, "", fmt.Errorf("error converting string to float64 - %w", err)
            }

            return size, unit, nil
        }
    }

    // If no units were found return error indicating unusual behavior, as this function
    // should have not been called without file units present in arg string
    return 0, "", fmt.Errorf("no valid unit found in arg file size string")
}


// Creates buffer and populates it with random bytes and returns as string.
//
// @Parameters
// - numberChars:  The number of random character to create and set buffer size
//
// @Returns
// - The string of random characters converted from bytes
//
func RandStringBytes(numberChars int) string {
    byteSlice := make([]byte, numberChars)
    // Seed the random number generator with the current Unix timestamp
    rand.Seed(uint64(time.Now().UnixNano()))

    for index := range byteSlice {
        byteSlice[index] = LetterBytes[rand.Intn(len(LetterBytes))]
    }

    return string(byteSlice)
}


// Checks to see if element in slice contains the target string.
//
// @Parameters
// - slice:  String slice to check if value is contained in an entry
// - target:  Target string to check if contained in slice items
//
// @Returns
// - true/false boolean depnding on whether target is in slice or not
//
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


// Checks to see if element in slice is equal to the target string.
//
// @Parameters
// - slice:  String slice to check if value is equal to an entry
// - target:  Target string to check if equal to slice items
//
// @Returns
// - true/false boolean depnding on whether target is in slice or not
//
func StringSliceHasItem(slice []string, target string) bool {
    // Iterate over the copied slice and check for the target value
    for _, item := range slice {
        // If the current unit is in the target string
        if target == item {
            return true
        }
    }

    return false
}


// Function to convert different size units to bytes and return as int64.
//
// @Parameters
// - size:  The size of unit to be converted
// - unit:  The unit to be converted to raw bytes
//
// @Returns
// - The converted file size as raw bytes
//
func ToBytes(size float64, unit string) int64 {
    var byteSize float64

    // Convert the size to bytes based on the unit
    switch unit {
    // Kilobytes
    case "KB":
        byteSize = size * globals.KB
    // Megabytes
    case "MB":
        byteSize = size * globals.MB
    // Gigabytes
    case "GB":
        byteSize = size * globals.GB
    // Invalid unit
    default:
        return -1
    }

    // Convert the result to int64 and return
    return int64(byteSize)
}


// TransferManager tracks the size of all ongoing transfers.
type TransferManager struct {
    OngoingTransfersSize int64
}

// NewTransferManager initializes and returns a new TransferManager instance.
func NewTransferManager() *TransferManager {
    return &TransferManager{}
}

// AddTransferSize adds the specified size to the ongoing transfers.
func (tm *TransferManager) AddTransferSize(size int64) {
    atomic.AddInt64(&tm.OngoingTransfersSize, size)
}

// GetOngoingTransfersSize returns the current total size of ongoing transfers.
func (tm *TransferManager) GetOngoingTransfersSize() int64 {
    return atomic.LoadInt64(&tm.OngoingTransfersSize)
}

// RemoveTransferSize subtracts the specified size from the ongoing transfers.
func (tm *TransferManager) RemoveTransferSize(size int64) {
    atomic.AddInt64(&tm.OngoingTransfersSize, -size)
}


// Trims after the last occurance of specified delimiter.
//
// @Parameters
// - input:  Input to parsed based on last delimiter
// - delimiter:  The delimiter to specify where input should be parsed
//
// @Returns
// - The parsed byte slice output
// - Error if it occurs, otherwise nil on success
//
func TrimAfterLast(input []byte, delimiter []byte) ([]byte, error) {
    // Find the last occurance of the delimiter
    position := bytes.LastIndex(input, delimiter)
    if position == -1 {
        return input, fmt.Errorf("delimiter not found in input")
    }

    return input[position+len(delimiter):], nil
}

// Trims any number of specified byte char from the end of the byte slice.
//
// @Parameters
// - buffer:  The buffer storing the data to end trim
// - char:  The character to shave off the end of the buffer
//
// @Returns
// - The parsed byte slice without any chars on the end
//
func TrimEndChars(buffer []byte, char byte) []byte {
    // Get the index of the last element
    index := len(buffer) - 1

    // Loop backwards to find first instance of non-specified char
    for index >= 0 && buffer[index] == char {
        index--
    }

    // Return the slice up to last valid character
    return buffer[:index+1]
}
