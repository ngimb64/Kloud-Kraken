package wordlist

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"unicode"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
)

// Performs the Linux cat command on a slice of files to the passed in
// output path. Prior to executing the command the original source file
// is deleted and the cat file slice is reset for the next execution.
//
// @Parameters
// - catFiles:  Slice of the file paths of files to be concatenated via cat
// - catPath:  The path to the resulting output file of the cat command
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func CatAndDelete(catFiles *[]string, catPath string) error {
    catCmd := "cat"
    // Iterate through the file path and apppend them
    for _, file := range *catFiles {
        catCmd += " " + file
    }

    // Append the rest of the command args
    catCmd += " 2>/dev/null > " + catPath

    // Format the unique merging command with current file to output file
    cmd := exec.Command("sh", "-c", catCmd)
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
    }

    // Reset the cat files list
    *catFiles = nil

    return nil
}


// Runs the source file through duplicut with the resulting output written
// to the destination file and comparing its size to the max file size.
//
// @Parameters
// - srcPath:  The path to the source file that needs de-deplication
// - destPath:  The path to the resulting output file of duplicut
// - maxFileSize:  The max allowed file size for wordlists
//
// @Returns
// - numerical indicator of output size comparison to max file size
//   0 = less than, 1 = equal to, 2 = greater than
// - The size of the duplicut output file
//
func DuplicutAndDelete(srcPath string, destPath string,
                       maxFileSize int64) (int32, int64, error) {
    // Format duplicut command to be executed
    duplicutCmd := "../../duplicut/duplicut " + srcPath + " -o " +
                   destPath + " 1>/dev/null 2>/dev/null"
    cmd := exec.Command("sh", "-c", duplicutCmd)
    // Execute the command and wait until it is complete
    err := cmd.Run()
    if err != nil {
        return -1, -1, err
    }

    // Delete the source file after duplicut
    err = os.Remove(srcPath)
    if err != nil {
        return -1, -1, err
    }

    // Get the size of resulting output file
    destPathInfo, err := os.Stat(destPath)
    if err != nil {
        return -1, -1, err
    }

    // Get the output file size
    outfileSize := destPathInfo.Size()

    // If the output file size is less than max
    if outfileSize < maxFileSize {
        return 0, outfileSize, nil
    // If the output file size is equal max
    } else if outfileSize == maxFileSize {
        return 1, outfileSize, nil
    // If the output file size is greater than max
    } else {
        return 2, outfileSize, nil
    }
}


// Takes the file that is over the max allowed size and move
// any data over that max into a new file via dd command.
//
// @Parameters
// - filterPath:  The source file that is over the max size that needs
//                excess data to be filtered
// - shavePath:  The destination file there the excess data is written to
// - originalPath:  Path to original file data after excess filtered
// - blockSize:  The size of the block of data for dd to send at a time
// - maxFileSize:  The max allowed size for wordlist file
//
// @Returns
// - The size of resulting file where extra data is shaved
// - Error if it occurs, otherwise nil on success
//
func FileShaveDD(filterPath string, shavePath string, originalPath string,
                 blockSize int, maxFileSize int64) (int64, error) {
    // Get the file info of the source file
    fileInfo, err := os.Stat(filterPath)
    if err != nil {
        return -1, err
    }

    srcSize := fileInfo.Size()
    blockSize64 := int64(blockSize)

    // If the source file is smaller than default block size
    if srcSize < blockSize64 {
        // Cut block size in half until smaller than source size
        blockSize = ReduceBlockSize(srcSize, blockSize64)
        // If the source size was less than 2 (lowest binary)
        if blockSize == -1 {
            return -1, fmt.Errorf("source size was less than 2 or not binary - %d", srcSize)
        }
    }

    // Divide the max file size by the block size to get
    // the number of blocks to skip
    skipSize := float64(maxFileSize) / float64(blockSize)
    countSize := skipSize
    blockSizeStr := strconv.Itoa(blockSize)
    // If the max file size divided by the block size has a remainder
    if maxFileSize % int64(blockSize) != 0 {
        skipSize += 1
    }

    // Format the dd command to copy exceeding data to new file
    cmd := exec.Command("dd", "if=" + filterPath, "of=" + shavePath, "bs=" + blockSizeStr,
                        "skip=" + strconv.Itoa(int(skipSize)))
    // Execute the dd command
    err = cmd.Run()
    if err != nil {
        return -1, err
    }

    // Format the dd command to copy original data to new file
    cmd = exec.Command("dd", "if=" + filterPath, "of=" + originalPath, "bs=" + blockSizeStr,
                       "count=" + strconv.Itoa(int(countSize)))
    // Execute the dd command
    err = cmd.Run()
    if err != nil {
        return -1, err
    }

    // Delete the original file once process is complete
    err = os.Remove(filterPath)
    if err != nil {
        return -1, err
    }

    // Get the file info for the shave path
    fileInfo, err = os.Stat(shavePath)
    if err != nil {
        return -1, err
    }

    return fileInfo.Size(), nil
}


// Takes the file that is over the max allowed size and move
// any data over that max into a new file via cut command.
//
// @Parameters
// - filterPath:  The source file that is over the max size that
//                needs excess data to be filtered
// - shavePath:  The destination file there the excess data is written to
// - maxFileSize:  The max allowed size for wordlist file
// - catFiles:  The slice of file paths to pass into CatAndDelete()
// - outFilesMap:  The map used to ensure only files that have not been
//                 processed are selected
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func FileShaveSplit(filterPath string, shavePath string, maxFileSize int64,
                    catFiles *[]string, outFilesMap map[string]struct{}) error {
    // Convert the max file size to string
    maxFileSizeStr := strconv.Itoa(int(maxFileSize))
    // Format the cut command to be executed
    cmd := exec.Command("split", "-d", "-C", maxFileSizeStr, filterPath, shavePath)
    // Execute split command
    err := cmd.Run()
    if err != nil {
        return err
    }

    // Delete the original file after split
    err = os.Remove(filterPath)
    if err != nil {
        return err
    }

    outerCount := 0
    innerCount := 0
    maxSizeFloat := float64(maxFileSize)

    for {
        // Format the current count on the end of the output path
        outPath := shavePath + strconv.Itoa(innerCount) + strconv.Itoa(outerCount)

        // Get the current file info
        fileInfo, err := os.Stat(outPath)
        // If the output file does not exist
        if errors.Is(err, os.ErrNotExist) {
            break
        } else if err != nil {
            return err
        }

        // If file is within the top 5% of max file size meaning its full
        if data.IsInPercentRange(maxSizeFloat, float64(fileInfo.Size()), 5.0) {
            // Add the current file to map for managing output files
            outFilesMap[outPath] = struct{}{}
        } else {
            // Add the current file to the cat files list
            *catFiles = append(*catFiles, outPath)
        }

        if outerCount >= 9 {
            innerCount += 1
            outerCount = 0
            continue
        }

        outerCount += 1
    }

    return nil
}


// Get the recommended IO block size and convert it to int.
//
// @Returns
// - The recommended block size
// - Error if it occurs, otherwise nil on success
//
func GetBlockSize() (int, error) {
    var blockSize int

    // Format command to get recommended block size
    cmd := exec.Command("sh", "-c", "stat / | grep 'IO Block:' | cut -d':' -f4 | cut -d' ' -f2")

    // Execute command to get block size
    byteBlockSize, err := cmd.Output()
    if err != nil {
        return 0, err
    }

    // Iterate through the range of bytes in slice
    for _, b := range byteBlockSize {
        // If the byte rune is a digit
        if unicode.IsDigit(rune(b)) {
            // Convert from byte ('0' to '9') to int
            blockSize = blockSize*10 + int(b-'0')
        }
    }

    return blockSize, nil
}


// Sets up the cat files slice and out files map, gets the block size, and
// call filepath walk with closure function above until complete.
//
// @Parameters
// - dirPath:  The path to the directory where wordlist merging occurs
// - maxFileSize:  The maximum size a wordlist should be
// - maxRange:  The range within the max that makes a file register as full
// - maxCutSize:  The max size threshold where dd is utilized instead of cut
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func MergeWordlistDir(dirPath string, maxFileSize int64,
                      maxRange float64, maxCutSize int64) error {
    catFiles := []string{}
    outFilesMap := make(map[string]struct{})

    // Get the recommended block size for if dd is utilized
    blockSize, err := GetBlockSize()
    if err != nil {
        return err
    }

    // Iterate through the contents of the directory and any subdirectories, merging wordlists
    err = filepath.Walk(dirPath, func(path string, itemInfo os.FileInfo, walkErr error) error {
        return MergeWordlists(dirPath, maxFileSize, maxRange, maxCutSize, &catFiles,
                              outFilesMap, blockSize, path, itemInfo, walkErr)
    })

    if err != nil {
        return err
    }

    return nil
}


// Walks through passed in dir path appending files to the cat list until
// multiple are available, then performing cat on them while original files
// are deleted. After the cat result is passed into duplicut where the original
// file is deleted again. If the resulting file size is equal to the max file size
// OR is within the specified max range of the file size it will be added to a
// map for managing completed files. If it is less than the bottom of the max range
// it will be added back to the cat file list and re-iterate. If greater then
// if will either use cut (small files) or dd (larger files) to shave the exess
// data into a new file and save the original to the output files list.
//
// @Parameters
// - dirPath:  The path to the directory where wordlist merging occurs
// - maxFileSize:  The maximum size a wordlist should be
// - maxRange:  The range within the max that makes a file register as full
// - maxCutSize:  The max size threshold where dd is utilized instead of cut
// - catFiles:  The slice of file paths to pass into CatAndDelete()
// - outFilesMap:  The map used to ensure only files that have not been
//                 processed are selected
// - blockSize:  The size of the block of data for dd to send at a time
// - path:  Path to the currently selected item in merge directory
// - itemInfo:  The info of currently seleted item
// - err:  Error if it occurs during walk, otherwise nil on success
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func MergeWordlists(dirPath string, maxFileSize int64, maxRange float64, maxCutSize int64,
                    catFiles *[]string, outFilesMap map[string]struct{}, blockSize int,
                    path string, itemInfo os.FileInfo, err error) error {
    if err != nil {
        return err
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

    // Append the current file path to cat files slice
    *catFiles = append(*catFiles, path)

    // If there is less than 2 files in the cat files slice, skip to next
    if len(*catFiles) < 2 {
        return nil
    }

    // Create random file for cat command output
    catPath, _, err := disk.CreateRandFile(dirPath, globals.RAND_STRING_SIZE,
                                           "kloudkraken-data-", "txt", false)
    if err != nil {
        return err
    }

    // Cat files in cat slice into result deleting originals
    err = CatAndDelete(catFiles, catPath)
    if err != nil {
        return err
    }

    // Create a new file for final duplicut command output
    filterPath, _, err := disk.CreateRandFile(dirPath, globals.RAND_STRING_SIZE,
                                              "kloudkraken-data-", "txt", false)
    if err != nil {
        return err
    }

    // Run the oversized file via duplicut to output file, deleting original file
    sizeComp, destFileSize, err := DuplicutAndDelete(catPath, filterPath, maxFileSize)
    if err != nil {
        return err
    }

    // If the size of the dest file is equal to max OR resides within the max range
    if sizeComp == 1 || (sizeComp == 0 &&
    data.IsInPercentRange(float64(maxFileSize), float64(destFileSize), maxRange)) {
        // Add the resulting path to out files map
        outFilesMap[filterPath] = struct{}{}
        return nil
    // If the size of the dest file is less than max, skip to next
    } else if sizeComp == 0 {
        // Add the output file to cat files list for further processing
        *catFiles = append(*catFiles, filterPath)
        return nil
    }

    // Create a new file for file shaving process
    shavePath, _, err := disk.CreateRandFile(dirPath, globals.RAND_STRING_SIZE,
                                             "kloudkraken-data-", "txt", false)
    if err != nil {
        return err
    }

    // For file greater than threshold, dd is optimal for resource scalability
    if destFileSize > maxCutSize {
        for {
            // Create a new file for original file data after excess filtered
            originalPath, _, err := disk.CreateRandFile(dirPath, globals.RAND_STRING_SIZE,
                                                        "kloudkraken-data-", "txt", false)
            if err != nil {
                return err
            }

            // Shaves any data large than excess size into new file
            shaveFileSize, err := FileShaveDD(filterPath, shavePath, originalPath,
                                              blockSize, maxFileSize)
            if err != nil {
                return err
            }

            // Add the maxed out file to the out files map
            outFilesMap[originalPath] = struct{}{}

            // If the shaved file still exceeds max file size
            if shaveFileSize > maxFileSize {
                // Set result path as input and make new shave path for next iteration
                filterPath = shavePath
                shavePath, _, err = disk.CreateRandFile(dirPath, globals.RAND_STRING_SIZE,
                                                        "kloudkraken-data-", "txt", false)
                if err != nil {
                    return err
                }

                continue
            }

            // Add the file with extra shaved data to cat files slice
            *catFiles = append(*catFiles, shavePath)
            break
        }
    // For files less than threshold, split is optimal for efficiency
    } else {
        // Shaves any data large than excess size into new file
        err = FileShaveSplit(filterPath, shavePath, maxFileSize,
                             catFiles, outFilesMap)
        if err != nil {
            return err
        }
    }

    return nil
}


// Divides the block size in half until the source file size exceeds it.
//
// @Parameters
// - srcSize:  The size of the source file for dd operation
// - blockSize:  The block size used for dd operation
//
// @Returns
// - Reduced block size on success, -1 on failure
//
func ReduceBlockSize(srcSize int64, blockSize64 int64) int {
    // If source file is less than smallest binary unit
    // or is not divisble by 2
    if srcSize < 2 || srcSize % 2 != 0 {
        return -1
    }

    for {
        // Divide block size by 2
        blockSize64 /= 2

        // Once the source size exceeds block size, exit loop
        if srcSize > blockSize64 {
            break
        }
    }

    return int(blockSize64)
}


// Deletes any subdirs and their contents in passed in dir path.
//
// @Parameters
// - dirPath:  The path to the directory to delete subdirs
//
// Returns
// - Error if it occurs, otherwise nil on success
//
func RemoveMergeSubdirs(dirPath string) error {
    // Get the contents of the wordlist merge dirs
    dirItems, err := os.ReadDir(dirPath)
    if err != nil {
        return err
    }

    // Iterate through wordlist merge dir contents
    for _, item := range dirItems {
        // Skip if not a dir
        if !item.IsDir() {
            continue
        }

        // Format the dir name onto path
        subdirPath := dirPath + "/" + item.Name()

        // Remove the subdir and any contents
        err = os.RemoveAll(subdirPath)
        if err != nil {
            return err
        }
    }

    return nil
}
