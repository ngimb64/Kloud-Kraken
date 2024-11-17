package main

import (
	"fmt"
	"os"
	"syscall"
)

const GB = 1024 * 1024 * 1024 // 1 GB in bytes

// GetDiskSpace gets the total and available space on the root disk.
func GetDiskSpace() (total uint64, free uint64, err error) {
	var statfs syscall.Statfs_t

	// Get the stats of the root filesystem ("/")
	err = syscall.Statfs("/", &statfs)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get disk space: %v", err)
	}

	// Total space is (blocks * block size)
	total = statfs.Blocks * uint64(statfs.Bsize)
	// Free space is (free blocks * block size)
	free = statfs.Bfree * uint64(statfs.Bsize)

	return total, free, nil
}

// SplitDiskSpace splits the disk space for processing and storage.
func SplitDiskSpace(total uint64, reserved uint64) (processingSpace uint64, storageSpace uint64, err error) {
	if total < reserved {
		return 0, 0, fmt.Errorf("not enough space to reserve %d bytes", reserved)
	}

	// Subtract reserved space (for OS) from total space
	remainingSpace := total - reserved

	// Split the remaining space into two equal parts
	processingSpace = remainingSpace / 2
	storageSpace = remainingSpace / 2

	return processingSpace, storageSpace, nil
}

func main() {
	// Reserved space for the OS (10GB)
	OSReservedSpace := 10 * GB

	// Get the total and available disk space
	total, free, err := GetDiskSpace()
	if err != nil {
		fmt.Println("Error getting disk space:", err)
		os.Exit(1)
	}

	fmt.Printf("Total disk space: %d GB\n", total/GB)
	fmt.Printf("Free disk space: %d GB\n", free/GB)

	// Reserve 10GB for the OS and split the remaining space
	processingSpace, storageSpace, err := SplitDiskSpace(total, OSReservedSpace)
	if err != nil {
		fmt.Println("Error splitting disk space:", err)
		os.Exit(1)
	}

	// Print the calculated spaces
	fmt.Printf("Reserved for OS: %d GB\n", OSReservedSpace/GB)
	fmt.Printf("Processing space: %d GB\n", processingSpace/GB)
	fmt.Printf("Storage space for data: %d GB\n", storageSpace/GB)
}
