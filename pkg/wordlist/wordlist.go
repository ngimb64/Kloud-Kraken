package wordlist

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
)


func CatAndDelete(catFiles *[]string, catPath string,
                  stringMap map[string]struct{}) error {
    cmdArgs := []string{}
    // Append the file paths to be run via cat
    cmdArgs = append(cmdArgs, *catFiles...)
    // Append the rest of the command args
    cmdArgs = append(cmdArgs, "2>/dev/null", ">", catPath)

    // Format the unique merging command with current file to output file
    cmd := exec.Command("cat", cmdArgs...)
    // Execute the command and wait until it is complete
    err := cmd.Run()
    if err != nil {
        return err
    }

    // Iterate through the files run via cat
    for _, filePath := range *catFiles {
        // Delete the current file being iterated
        err := os.Remove(filePath)
        if err != nil {
            return err
        }

        // Delete the current file from string map
        delete(stringMap, filePath)
    }

    // Reset the cat files list
    *catFiles = nil

    return nil
}


func DuplicutAndDelete(srcPath string, destPath string, maxFileSize int64,
                       stringMap map[string]struct{}) (int32, int64) {
    // Format duplicut command to be executed
    cmd := exec.Command("../../duplicut/duplicut", srcPath, "-o", destPath,
                        "1>/dev/null", "2>/dev/null")
    // Execute the command and wait until it is complete
    err := cmd.Run()
    if err != nil {
        log.Fatalf("Error running duplicut:  %v", err)
    }

    // Delete the source file after duplicut
    err = os.Remove(srcPath)
    if err != nil {
        log.Fatalf("Error deleting %s:  %v", srcPath, err)
    }

    // Delete the source path from string map
    delete(stringMap, srcPath)

    // Get the size of resulting output file
    destPathInfo, err := os.Stat(destPath)
    if err != nil {
        log.Fatalf("Error getting file info:  %v", err)
    }

    // Get the output file size
    outfileSize := destPathInfo.Size()

    // If the output file size is less than max
    if outfileSize < maxFileSize {
        return 0, outfileSize
    // If the output file size is equal max
    } else if outfileSize == maxFileSize {
        return 1, outfileSize
    // If the output file size is greater than max
    } else {
        return 2, outfileSize
    }
}


func FileShaveDD (filterPath string, shavePath string, blockSize int, maxFileSize int64) error {
    // Format the dd command to be executed
    cmd := exec.Command("dd", fmt.Sprintf("if=%s", filterPath),
                        fmt.Sprintf("of=%s", shavePath),
                        fmt.Sprintf("bs=%d", blockSize),
                        fmt.Sprintf("skip=%d", maxFileSize))
    // Execute the dd command
    err := cmd.Run()
    if err != nil {
        return err
    }

    return nil
}


func FileShaveSplit (filterPath string, shavePath string, maxFileSize string) error {
    // Format the cut command to be executed
    cmd := exec.Command("cut", "-b", maxFileSize, filterPath, shavePath)
    // Execute the cut command
    err := cmd.Run()
    if err != nil {
        return err
    }

    return nil
}


func MergeWordlistDir(dirPath string, maxFileSize int64, maxRange float64) {
    catFiles := []string{}
    outFilesMap := make(map[string]struct{})
    fileNameMap := make(map[string]struct{})

    // Get the recommended block size for if dd is utilized
    blockSize, err := disk.GetBlockSize()
    if err != nil {
        log.Fatalf("Error getting recommended block size:  %v", err)
    }

    // Iterate through the contents of the directory and any subdirectories
    err = filepath.Walk(dirPath, func(path string, itemInfo os.FileInfo, walkErr error) error {
        if walkErr != nil {
            return walkErr
        }

        // If the item is a dir, skip to next
        if itemInfo.IsDir() {
            return nil
        }

        // If current file exists in the out files map, skip to next
        _, exists := outFilesMap[path]
        if exists {
            return nil
        }

        // Append the current file path to cat files list
        catFiles = append(catFiles, path)

        // If there is less than 2 files in the cat files list, skip to next
        if len(catFiles) < 2 {
            return nil
        }

        // Create random file for cat command output
        catPath := disk.CreateRandFile(dirPath, globals.RAND_STRING_SIZE, fileNameMap)

        // Cat files in cat list into result deleting originals
        walkErr = CatAndDelete(&catFiles, catPath, fileNameMap)
        if walkErr != nil {
            return walkErr
        }

        // Create a new file for final duplicut command output
        filterPath := disk.CreateRandFile(dirPath, globals.RAND_STRING_SIZE, fileNameMap)

        // Run the oversized file via duplicut to output file, deleting original file
        sizeComparison, destFileSize := DuplicutAndDelete(catPath, filterPath,
                                                          maxFileSize, fileNameMap)
        // If the size of the dest file is equal to max
        // OR resides within the top 15 percent of the max
        if sizeComparison == 1 || (sizeComparison == 0 &&
        data.IsWithinPercentageRange(float64(maxFileSize), float64(destFileSize), maxRange)) {
            // Add the resulting path to out files map
            outFilesMap[filterPath] = struct{}{}
            return nil
        // If the size of the dest file is less than max, skip to next
        } else if sizeComparison == 0 {
            // Add the output file to cat files list for further processing
            catFiles = append(catFiles, filterPath)
            return nil
        }

        // Create a new file for final duplicut command output
        shavePath := disk.CreateRandFile(dirPath, globals.RAND_STRING_SIZE, fileNameMap)

        // For file greater than 75 GB, dd is optimal for resource scalability
        if destFileSize > (75 * globals.GB) {
            // Shaves any data large than excess size into new file
            walkErr = FileShaveDD(filterPath, shavePath, blockSize, maxFileSize)
        // For files less than 75 GB, split is optimal
        } else {
            // Shaves any data large than excess size into new file
            walkErr = FileShaveSplit(filterPath, shavePath, strconv.Itoa(int(maxFileSize)))
        }

        if walkErr != nil {
            return walkErr
        }

        // Add the maxed out file to the out files map
        outFilesMap[filterPath] = struct{}{}
        // Add the file with extra shaved data to cat files list
        catFiles = append(catFiles, shavePath)

        return nil
    })

    if err != nil {
        log.Fatalf("Error merging wordlists:  %v", err)
    }
}
