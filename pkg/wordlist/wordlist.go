package wordlist

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

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


func FileShaveCut () error {

    return nil
}


func FileShaveDD () error {

    return nil
}


func MergeWordlistDir(dirPath string, maxFileSize int64, maxRange float64) {
    catFiles := []string{}
    outFilesMap := make(map[string]struct{})
    nameMap := make(map[string]struct{})
    // Create random file for duplicut command output per iteration
    dupePath := disk.CreateRandFile(dirPath, globals.RAND_STRING_SIZE, nameMap)
    // Create a new file for final duplicut command output
    filterPath := disk.CreateRandFile(dirPath, globals.RAND_STRING_SIZE, nameMap)

    // Get the recommended block size for if dd is utilized
    blockSize, err := disk.GetBlockSize()
    if err != nil {
        log.Fatalf("Error getting recommended block size:  %v", err)
    }

    // Iterate through the contents of the directory and any subdirectories
    err = filepath.Walk(dirPath, func(_ string, itemInfo os.FileInfo, walkErr error) error {
        if walkErr != nil {
            return walkErr
        }

        // If the item is a dir, skip to next
        if itemInfo.IsDir() {
            return nil
        }

        // Format the current file name with path
        currentFile := fmt.Sprintf("%s/%s", dirPath, itemInfo.Name())
        // If current file exists in the out files map, skip to next
        _, exists := outFilesMap[currentFile]
        if exists {
            return nil
        }

        // Append the current file path to cat files list
        catFiles = append(catFiles, currentFile)

        // If there is less than 2 files in the cat files list, skip to next
        if len(catFiles) < 2 {
            return nil
        }

        // Create random file for cat command output
        catPath := disk.CreateRandFile(dirPath, globals.RAND_STRING_SIZE, nameMap)

        // Cat files in cat list into result deleting originals
        walkErr = CatAndDelete(&catFiles, catPath, nameMap)
        if walkErr != nil {
            return walkErr
        }

        // Run the cat merge file via duplicut to output file, deleting original file
        sizeComparison, destFileSize := DuplicutAndDelete(catPath, dupePath,
                                                          maxFileSize, nameMap)
        // If the size of the dest file is equal to max
        // OR resides within the top 15 percent of the max
        if sizeComparison == 1 || (sizeComparison == 0 &&
        data.IsWithinPercentageRange(float64(maxFileSize), float64(destFileSize), maxRange)) {
            // Add the resulting path to out files map
            outFilesMap[dupePath] = struct{}{}
            // Create a new random file for duplicut command output
            dupePath = disk.CreateRandFile(dirPath, globals.RAND_STRING_SIZE, nameMap)
        // If the size of the dest file is less than max, skip to next
        } else if sizeComparison == 0 {
            return nil
        }

        // Run the oversized file via duplicut to output file, deleting original file
        sizeComparison, destFileSize = DuplicutAndDelete(dupePath, filterPath,
                                                         maxFileSize, nameMap)
        // If the size of the dest file is equal to max
        // OR resides within the top 15 percent of the max
        if sizeComparison == 1 || (sizeComparison == 0 &&
        data.IsWithinPercentageRange(float64(maxFileSize), float64(destFileSize), maxRange)) {
            // Add the resulting path to out files map
            outFilesMap[filterPath] = struct{}{}
            // Create a new random file for duplicut command output
            filterPath = disk.CreateRandFile(dirPath, globals.RAND_STRING_SIZE, nameMap)
        // If the size of the dest file is less than max, skip to next
        } else if sizeComparison == 0 {
            return nil
        }

        // For files less than 75 GB, cut is optimal
        if destFileSize < (75 * globals.GB) {
            // Shaves any data large than excess size into new file
            walkErr = FileShaveCut()
        // For file greater than 75 GB, dd is optimal for resource scalability
        } else {
            // Shaves any data large than excess size into new file
            walkErr = FileShaveDD()
        }

        if walkErr != nil {
            return walkErr
        }

        return nil
    })

    if err != nil {
        log.Fatalf("Error merging wordlists:  %v", err)
    }
}
