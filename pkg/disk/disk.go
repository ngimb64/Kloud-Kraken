package disk

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"golang.org/x/sys/unix"
)

// Package level variables
var (
    FileSelectionLock sync.Mutex  // Mutex for synchronizing the file selection
    InitOnce          sync.Once   // Ensures function is only called one time
    ProjectRoot       string      // Path to project root dir
    SelectedFiles     sync.Map	  // Global map to track selected files
)

// AppendFile appends contents of srcFile to destFile if source file has data.
//
// @Parameters
//  - sourceFilePath:  The source file whose data will be appended to the dest
//  - destFilePath:  The destination file where source data will be appended
//
// @Returns
//  - Error if it occurs, otherwise nil on success
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


// CalcReserveBytes returns how many bytes should be reserved on a disk
// to avoid filling it completely.
//
// @Parameters
//  - diskSize:  The total size of the entire disk in bytes
//
// @Returns
//  - The calulcated reserved bytes size based on disk size
//
func CalcReserveBytes(diskSize int64) int64 {
    if diskSize <= 0 {
        return 0
    }

    const (
        gb         = 1024 * 1024 * 1024
        smallGB    = 50.0                // <= this size → 10%
        largeGB    = 500.0               // >= this size → 1%
        maxPct     = 0.10                // 10%
        minPct     = 0.01                // 1%
        minReserve = 100 * 1024 * 1024   // 100 MB
    )

    var pct float64
    totalGB := float64(diskSize) / float64(gb)

    switch {
    case totalGB <= smallGB:
        pct = maxPct
    case totalGB >= largeGB:
        pct = minPct
    default:
        // Perform linear interpolation
        ratio := (totalGB - smallGB) / (largeGB - smallGB) // 0..1
        pct = maxPct - ratio * (maxPct - minPct)
    }

    // Get the reserve size
    reserve := int64(math.Ceil(pct * float64(diskSize)))

    // If the reserve is less than the minimum, set the minimum
    if reserve < minReserve {
        reserve = minReserve
    }

    return reserve
}


// Reads the passed in path to dir and attempts to get the first file,
// returning its name and size.
//
// @Parameters
//  - path:  The path to the directory to attempt to read a file
//
// @Returns
//  - The name of the retrieved file
//  - The size of the retrieved file
//  - Error if it occurs, otherwise nil on success
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


// Creates a random text file based on length of name and extension. Provides
// boolean toggle to specify whether file handle should stay open and returned
// or be closed and not be returned.
//
// @Parameters
//  - dirPath:  The path to the directory where the file will be created
//  - nameLen:  The number of random characters for the name
//  - baseName:  The base of file name to append random chars to
//  - externsion:  The file extension to use (ex: "txt" leave out the .)
//  - retHandler:  Boolean used to return the open file descriptor or not
//
// @Returns
//  - The formatted path to the newly create random file
//  - The open file handler of create file is retHandler is true
//  - Error if it occurs, otherwise nil on success
//
func CreateRandFile(dirPath string, nameLen int, baseName string,
                    extension string, retHandler bool) (
                    string, *os.File, error) {
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

        // Attempt to open the generated file path
        file, err := os.OpenFile(randoPath, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
        // If the file exists, close it and skip
        if os.IsExist(err) {
            err = file.Close()
            if err != nil {
                return randoPath, nil, err
            }

            continue

        } else if err != nil {
            return "", nil, err
        }

        // If the return handler is true, return open file
        if retHandler {
            return randoPath, file, nil
        }

        // Close the file descriptor since not in use
        err = file.Close()
        if err != nil {
            return randoPath, nil, err
        }

        return randoPath, nil, nil
    }
}


// Handler for resolving symlink paths to ensure absolute is returned.
//
// @Parameters
//  - symPath:  Symlink path to be evaluated
//
// @Returns
//  - The resolved symlink path
//
func evalSymlinks(symPath string) string {
    resolved, err := filepath.EvalSymlinks(symPath)
    if err != nil {
        return symPath
    }

    return resolved
}


// Gets the total and available space on the root disk.
//
// @Parameters
//  - path:  path to location on disk where size will be queried
//  - reservedSpace:  The amount of space reserved for the OS
//
// @Returns
//  - The total space on disk
//  - the available free space
//  - Error if it occurs, otherwise nil on success
//
func GetDiskSpace(path string) (
                  total int64, free int64, err error) {
    var statfs unix.Statfs_t

    // Get the stats of the passed in path
    err = unix.Statfs(path, &statfs)
    if err != nil {
        return -1, -1, err
    }

    // Total space is (blocks * block size)
    total = int64(statfs.Blocks) * statfs.Bsize
    // Free space is (free blocks * block size)
    free = int64(statfs.Bfree) * statfs.Bsize
    // Subtract the reserved  OS space from from available
    remaining := free - CalcReserveBytes(total)

    return remaining, total, nil
}


// Handler that ensures getProjectRootDir is only called once and
// handles the return value.
//
// @Returns
//  - The root dir of the project
//
func GetProjectRootDir() string {
    InitOnce.Do(func() {
        ProjectRoot = getProjectRootDir()
    })

    return ProjectRoot
}


// Retrieves the root dir of the project by reverse crawling symlinks and
// checking for common files in root directory of project.
//
// @Returns
//  - Path to the projects root directory
//
func getProjectRootDir() string {
    // Check for environment variable override
    envVar := os.Getenv("KRAKEN_ROOT")
    if envVar != "" {
        // Get the absolute path of variable
        abs, err := filepath.Abs(envVar)
        if err != nil {
            return envVar
        }

        return evalSymlinks(abs)
    }

    var searchPoints []string

    // Get the working directory
    wordDir, err := os.Getwd()
    if err == nil {
        // Get the absolute path
        abs, err := filepath.Abs(wordDir)
        if err == nil {
            searchPoints = append(searchPoints, abs)
        } else {
            searchPoints = append(searchPoints, wordDir)
        }
    }

    // Get the path of the executable that was called
    exe, err := os.Executable()
    if err == nil {
        searchPoints = append(searchPoints, filepath.Dir(exe))
    }

    // Search upward for project markers
    seenMap := map[string]struct{}{}

    // Iterate through the slice of search points to reverse crawl symlinks
    for _, searchPoint := range searchPoints {
        // Resolve the symlink for the current search point
        point := evalSymlinks(searchPoint)

        for {
            // Check to see if the point exists in filter map
            _, ok := seenMap[point]
            if ok {
                break
            }

            seenMap[point] = struct{}{}

            // Check to see if point has root files
            if hasRootFile(point) {
                return point
            }

            // Get the dir where point resides
            parent := filepath.Dir(point)
            if parent == point {
                break
            }

            point = parent
        }
    }

    // Fallback to first candidate or "."
    if len(searchPoints) > 0 {
        return searchPoints[0]
    }

    return "."
}


// Checks to see if the directory has a file that indicates it is root.
//
// @Parameters
//  - dir:  The directory to check for files common in root
//
// @Returns
//  - Toggle for whether the directory has a root file or not
//
func hasRootFile(dir string) bool {
    markers := []string{"go.mod", ".git", "Makefile", "README.md"}

    // Iterate through root marker files
    for _, marker := range markers {
        // Check to see if the file has statistics
        _, err := os.Stat(filepath.Join(dir, marker))
        if err == nil {
            return true
        }
    }

    return false
}


// Creates the slice of directories passed in.
//
// @Parameters
//  - programDirs:  The slice of directories to be created
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func MakeDirs(programDirs []string) error {
    // Iterate through slice of dirs
    for _, dir := range programDirs {
        // Create the current dir and any missing parent dirs
        err := os.MkdirAll(dir, os.ModePerm)
        if err != nil {
            return err
        }
    }

    return nil
}


// Checks if the file or directory exists and ensure directories have contents
// in them based on file size or entries in dir.
//
// @Parameters
//  - The path to check for existence
//
// @Returns
//  - Boolean for the item existing and having content
//  - Boolean for if the item is a directory
//  - Boolean for if the file or dir contains data
//  - Error if it occurs, otherwise nil on success
//
func PathExists(filePath string) (bool, bool, bool, error) {
    // Get item info on passed in path
    itemInfo, err := os.Stat(filePath)
    if err != nil {
        // If the file does not exist
        if os.IsNotExist(err) {
            return false, false, false, nil
        }

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
//  - loadDir:  The directory to attempt to select a file
//  - maxFileSizeInt64:  The max file size to ensure violators are not selected
//
// @Returns
//  - Path of the selected file
//  - Size of the selected file
//  - Error if it occurs, otherwise nil on success
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

        // Format the current file path
        itemPath := loadDir + "/" + item.Name()

        // Get the file statistics for the current file
        itemInfo, err := os.Stat(itemPath)
        if err != nil {
            continue
        }

        // If the current file size is greater than the max file size
        // set in YAML OR the current file is empty
        if itemInfo.Size() > maxFileSizeInt64 || itemInfo.Size() == 0 {
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
