package wordlist

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

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
                 blockSize int64, maxFileSize int64) (int64, error) {
    // Divide the max file size by the block size to get
    // the number of blocks to skip
    skipSize := float64(maxFileSize) / float64(blockSize)
    countSize := skipSize
    blockSizeStr := strconv.FormatInt(blockSize, 10)
    // If the max file size divided by the block size has a remainder
    if maxFileSize % blockSize != 0 {
        skipSize += 1
    }

    // Format the dd command to copy exceeding data to new file
    cmd := exec.Command("dd", "if=" + filterPath, "of=" + shavePath, "bs=" + blockSizeStr,
                        "skip=" + strconv.FormatFloat(skipSize, 'f', -1, 64))
    // Execute the dd command
    err := cmd.Run()
    if err != nil {
        return -1, err
    }

    // Format the dd command to copy original data to new file
    cmd = exec.Command("dd", "if=" + filterPath, "of=" + originalPath, "bs=" + blockSizeStr,
                       "count=" + strconv.FormatFloat(countSize, 'f', -1, 64))
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
    fileInfo, err := os.Stat(shavePath)
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
    maxFileSizeStr := strconv.FormatInt(maxFileSize, 10)
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

    var outerCount uint64 = 0
    var innerCount uint64 = 0
    maxSizeFloat := float64(maxFileSize)

    for {
        // Format the current count on the end of the output path
        outPath := shavePath + strconv.FormatUint(innerCount, 10) + strconv.FormatUint(outerCount, 10)

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


// Gets the optimal block size for file spliting based on the size of the file.
//
// @Parameters
// - fileSize:  The size of the file which will be used to calculate optimal block size
//
// @Returns
// - The calculated optimal block size
// - Error if it occurs, otherwise nil on success
//
func GetOptimalBlockSize(fileSize int64) (int64, error) {
    // Start with a 4 KiB buffer
    bufSize := int64(4096)

    // Define a practical upper limit
    const maxBufSize = 8 * globals.MB

    // Double the buffer size until it is >= file size or we hit the cap.
    for bufSize < fileSize && bufSize < maxBufSize {
        bufSize *= 2
    }

    // If doubling went over the cap, clamp to maxBufSize.
    if bufSize > maxBufSize {
        bufSize = maxBufSize
    }

    return bufSize, nil
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

    // Iterate through the contents of the directory and any subdirectories, merging wordlists
    err := filepath.Walk(dirPath, func(path string, itemInfo os.FileInfo, walkErr error) error {
        return MergeWordlists(dirPath, maxFileSize, maxRange, maxCutSize, &catFiles,
                              outFilesMap, path, itemInfo, walkErr)
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
// - path:  Path to the currently selected item in merge directory
// - itemInfo:  The info of currently seleted item
// - err:  Error if it occurs during walk, otherwise nil on success
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func MergeWordlists(dirPath string, maxFileSize int64, maxRange float64, maxCutSize int64,
                    catFiles *[]string, outFilesMap map[string]struct{}, path string,
                    itemInfo os.FileInfo, err error) error {
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

    // Create a new file for filtered output
    filterPath, _, err := disk.CreateRandFile(dirPath, globals.RAND_STRING_SIZE,
                                              "kloudkraken-data-", "txt", false)
    if err != nil {
        return err
    }

    destFileSize := itemInfo.Size()
    // If the current file size is not within 15% of the max size
    if destFileSize < maxFileSize &&
    !data.IsInPercentRange(float64(maxFileSize), float64(destFileSize), 15.0) {
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
        // If the size of the dest file is less than max
        } else if sizeComp == 0 {
            // Add the output file to cat files list for further processing
            *catFiles = append(*catFiles, filterPath)
            return nil
        }
    }

    // Create a new file for file shaving process
    shavePath, _, err := disk.CreateRandFile(dirPath, globals.RAND_STRING_SIZE,
                                             "kloudkraken-data-", "txt", false)
    if err != nil {
        return err
    }

    // For file greater than threshold, dd is optimal for resource scalability
    if destFileSize > maxCutSize {
        // Get the optimal block size for file shaving operation based on the file size
        blockSize, err := GetOptimalBlockSize(destFileSize)
        if err != nil {
            return err
        }

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

                // Reset the optimal block size based on size of result of first dd operation
                blockSize, err = GetOptimalBlockSize(shaveFileSize)
                if err != nil {
                    return err
                }

                continue
            }

            // Add the file with extra shaved data to cat files slice
            *catFiles = append(*catFiles, shavePath)
            break
        }
    // For files less than threshold, split is optimal parsing entries line by line
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
