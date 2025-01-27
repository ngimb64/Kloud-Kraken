package validate

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"github.com/ngimb64/Kloud-Kraken/pkg/display"
)


func ValidateConfigPath(configFilePath *string) {
    for {
        fmt.Print("Enter the path of the YAML config file to use:  ")
        // Read the YAML file path from user input
        _, err := fmt.Scanln(configFilePath)
        if err != nil {
            fmt.Println("Error occurred reading user input path: ", err)
            // Sleep for a few seconds and clear screen before re-prompt
            display.ClearScreen(3)
            continue
        }

        // Check to see if the input path exists and is a file or dir
        exists, isDir, err := disk.PathExists(*configFilePath)
        if err != nil {
            fmt.Println("Error checking input path existence: ", err)
            // Sleep for a few seconds and clear screen before re-prompt
            display.ClearScreen(3)
            continue
        }

        // If the path does not exist or is a dir or is not YAML file
        if !exists || isDir || !strings.HasSuffix(*configFilePath, ".yml") {
            fmt.Println("Input path does not exist or is a dir or not of YAML file type: ", configFilePath)
            // Sleep for a few seconds and clear screen before re-prompt
            display.ClearScreen(3)
            continue
        }

        break
    }
}


func ValidateDir(dirPath string) error {
    // Check to see if the load directory exists and has files in it
    exists, isDir, err := disk.PathExists(dirPath)
    if err != nil {
        return err
    }

    // If the load dir path does not exist or is not a directory
    if !exists || !isDir {
        return fmt.Errorf("load dir path does not exist or is a file")
    }

    return nil
}


func ValidateFile(filePath string) error {
    // Check to see if the hash file exists
    exists, isDir, err := disk.PathExists(filePath)
    if err != nil {
        return err
    }

    // If the hash file path does not exist or is a directory
    if !exists || isDir {
        return fmt.Errorf("hash file path does not exist or is a directory")
    }

    return nil
}


func ValidateHashFile(filePath string) error {
    validPath, err := ValidatePath(filePath)
    if err != nil {
        return fmt.Errorf("improper hash_file_path specified in local config - %w", err)
    }

    err = ValidateFile(validPath)
    if err != nil {
        return fmt.Errorf("error validating hash file based on %s path - %w", validPath, err)
    }

    return nil
}


func ValidateListenerPort(listenerPort int32) bool {
    return listenerPort > 1000
}


func ValidateLoadDir(dirPath string) error {
    validPath, err := ValidatePath(dirPath)
    if err != nil {
        return fmt.Errorf("improper load_dir specified in local config - %w", err)
    }

    // Ensure the load directory exists and has files in it
    err = ValidateDir(validPath)
    if err != nil {
        return fmt.Errorf("error validating load directory - %w", err)
    }

    return nil
}


func ValidateLogMode(logMode string) bool {
    logModes := []string{"local", "cloudwatch", "both"}

    // Check to see if the passed in mode is in preset list
    return data.StringSliceContains(logModes, logMode)
}


func ValidateMaxFileSize(maxFileSize string) (int64, error) {
    var byteSize int64
    var err error

    // Save string max file size to local variable ensuring
    // any units are lowercase (MB, GB, etc.)
    maxFileSize = strings.ToLower(maxFileSize)
    // Check to see if the max files size contains a conversion unit
    sliceContains := data.StringSliceContains(globals.FILE_SIZE_TYPES, maxFileSize)

    // If the slice contains a data unit to be converted to raw bytes
    if sliceContains {
        // Split the size from the unit type
        size, unit, err := data.ParseFileSizeType(maxFileSize)
        if err != nil {
            return -1, fmt.Errorf("error parsing file size unit - %w", err)
        }
        // Pass the size and unit to calculate to raw bytes
        byteSize = data.ToBytes(size, unit)
    // If the file size seems to already be in bytes
    } else {
        // Attempt to convert it straight to int64
        byteSize, err = strconv.ParseInt(maxFileSize, 10, 64)
        if err != nil {
            return -1, fmt.Errorf("error converting string to int64 - %w", err)
        }
    }

    // If the converted max file size is less than or equal to 0
    if byteSize <= 0 {
        return -1, fmt.Errorf("converted max file size is less than or equal to 0")
    }

    return byteSize, nil
}


func ValidateMaxTransfers(maxTransfers int32) bool {
    return maxTransfers > 0
}


func ValidateNumberInstances(numberInstances int32) bool {
    return numberInstances > 0
}


func ValidatePath(path string) (string, error) {
    // Ensure the path is not empty
    if path == "" {
        return "", fmt.Errorf("passed in path cannot be empty")
    }

    // Clean the path (removes redundant slashes, etc.)
    cleanedPath := filepath.Clean(path)
    // Check if the cleaned path contains any invalid characters
    if strings.Contains(cleanedPath, "//") {
        return "", fmt.Errorf("path %s contains double slashes", path)
    }

    // Ensure the path does not end with slash unless its root directory
    if cleanedPath != "/" && strings.HasSuffix(cleanedPath, "/") {
        return "", fmt.Errorf("path %s cannot end with a slash unless it is the root directory", path)
    }

    // Validate path format with regex
    validPath := regexp.MustCompile(`^[a-zA-Z0-9\._\-\/]+$`).MatchString(cleanedPath)
    if !validPath {
        return "", fmt.Errorf("path %s contains invalid characters", path)
    }

    return cleanedPath, nil
}


func ValidateRegion(region string) bool {
    // Iterate through the endpoint partitions
    for _, currPartitions := range endpoints.DefaultPartitions() {
        // Iterate through the regions in the current partition
        for _, currRegion := range currPartitions.Regions() {
            // It the current region ID matches arg string
            if currRegion.ID() == region {
                return true
            }
        }
    }

    return false
}


func ValidateRulesetFile(filePath string) error {
    validPath, err := ValidatePath(filePath)
    if err != nil {
        return fmt.Errorf("improper ruleset_path specified in local config - %w", err)
    }

    err = ValidateFile(validPath)
    if err != nil {
        return fmt.Errorf("error validating ruleset file based on %s path - %w", validPath, err)
    }

    return nil
}
