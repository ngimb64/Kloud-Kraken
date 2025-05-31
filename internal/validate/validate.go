package validate

import (
	"errors"
	"fmt"
	"net"
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

// Package level variables
var ReAccountId = regexp.MustCompile(`^\d{12}$`)
var ReIamUsername = regexp.MustCompile(`^[\w+=,.@-]{1,64}$`)
var ReSecurityGroupId = regexp.MustCompile(`^sg-[0-9a-f]{8,}$`)
var ReSecurityGroupName = regexp.MustCompile(
    `^[A-Za-z0-9\s\.\_\-\:\/\(\)\#\,\@\[\]\+\=\&\;\{\}\!\$\*]{1,255}$`,
)
var ReSubnetId = regexp.MustCompile(`^subnet-[0-9a-f]{8,}$`)


// Ensure the AWS account ID is of proper format.
//
// @Parameters
// - accountId:  The ID number for the AWS account
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func ValidateAccountId(accountId string) error {
    // If the account ID is not 12 digits
    if !ReAccountId.MatchString(accountId) {
        return errors.New("invalid AWS account ID, must be exactly 12 digits")
    }

    return nil
}


// Ensures the S3 bucket name is of proper format.
//
// @Parameters
// -name:  The name of the S3 bucket to be validated
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func ValidateBucketName(name string) error {
    if name == "" {
        return nil
    }

    length := len(name)
    // Ensure the bucket is of proper length
    if length < 3 || length > 63 {
        return fmt.Errorf("bucket name must be 3 to 63 characters; got %d", length)
    }

    // Allows lowercase letters, numbers, dots & hyphens and must start and
    // end with letter or number
    validName := regexp.MustCompile(`^[a-z0-9][a-z0-9\.-]{1,61}[a-z0-9]$`)
    if !validName.MatchString(name) {
        return errors.New(
            "bucket name must start and end with a lowercase letter or number, " +
            "and contain only lowercase letters, numbers, dots (.) or hyphens (-)",
        )
    }

    // Ensure there are no IP addresses in the name
    if ip := net.ParseIP(name); ip != nil {
        return errors.New("bucket name must not be formatted as an IP address")
    }

    return nil
}


// Ensures that if there is a char set that is present and the proper cracking
// mode that supports a hash mask with custom charsets is present.
//
// @Parameters
// - crackingMode:  The hashcat cracking mode
// - hashMask:  The hashcat hash mask
// - args:  A iterator of 4 custom character sets
//
// @Returns
// - true/false boolean depending on if valid hashmask and charsets are present
//
func ValidateCharsets(crackingMode string, hashMask string, args ...string) bool {
    if hashMask == "" {
        return true
    }

    supportedModes := []string{"3", "6", "7"}
    // Check to see if passed in cracking mode is in supported modes
    isSupported := data.StringSliceHasItem(supportedModes, crackingMode)

    // Iterate through charset args
    for _, charset := range args {
        // If the custom charset is present and mode is not supported
        if charset != "" && !isSupported  {
            return false
        }
    }

    return true
}


// In a continous loop, the input is gathered and tested to see if the path
// exists that is a yaml file with data inside it.
//
// @Parameters
// - configFilePath:  The path to the configuration to attempt to load
//
func ValidateConfigPath(configFilePath *string) {
    for {
        if *configFilePath == "" {
            fmt.Print("Enter the path of the YAML config file to use:  ")
            // Read the YAML file path from user input
            _, err := fmt.Scanln(configFilePath)
            if err != nil {
                fmt.Println("Error occurred reading user input path: ", err)
                // Sleep for a few seconds and clear screen before re-prompt
                display.ClearScreen(3)
                // Reset the config file path
                *configFilePath = ""
                continue
            }
        }

        // Check to see if the input path exists and is a file or dir
        exists, isDir, hasData, err := disk.PathExists(*configFilePath)
        if err != nil {
            fmt.Println("Error checking input path existence: ", err)
            // Sleep for a few seconds and clear screen before re-prompt
            display.ClearScreen(3)
            // Reset the config file path
            *configFilePath = ""
            continue
        }

        // If the path does not exist OR is a dir OR does not have data OR is not YAML file
        if !exists || isDir || !hasData || !strings.HasSuffix(*configFilePath, ".yml") {
            fmt.Println("Input path does not exist,is a dir, or not YAML file type: ",
                        configFilePath)
            // Sleep for a few seconds and clear screen before re-prompt
            display.ClearScreen(3)
            // Reset the config file path
            *configFilePath = ""
            continue
        }

        break
    }
}


// Validate the hashcat cracking mode to ensure it is supported.
//
// @Parameters
// - hashMode:  The hashcat mode to validate
//
// @Returns
// - A true/false boolean depending on whether the mode is supported or not
//
func ValidateCrackingMode(hashMode string) bool {
    hashModes := []string{"0", "3", "6", "7"}

    // Check to see if arg hash mode is in the allowed hash modes
    return data.StringSliceHasItem(hashModes, hashMode)
}


// Ensure the passed in directory path exists and is a dir that has data.
//
// @Parameters
// - dirPath:  The path to the directory to validate
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func ValidateDir(dirPath string) error {
    // Check to see if the load directory exists and has files in it
    exists, isDir, hasData, err := disk.PathExists(dirPath)
    if err != nil {
        return err
    }

    // If the load dir path does not exist OR is not a directory OR does not have data
    if !exists || !isDir || !hasData {
        return fmt.Errorf("load dir path does not exist or is a file or" +
                          " does not have data in it")
    }

    return nil
}


// Ensure the passed in file path exists and is a file that has data.
//
// @Parameters
// - filePath:  The path to the file to validate
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func ValidateFile(filePath string) error {
    // Check to see if the hash file exists
    exists, isDir, hasData, err := disk.PathExists(filePath)
    if err != nil {
        return err
    }

    // If hash file path does not exist OR is a directory OR does not have data
    if !exists || isDir || !hasData {
        return fmt.Errorf("hash file path does not exist or is a directory or" +
                          " does not have data in it")
    }

    return nil
}


// Validate the path to the hash file and the file itself via ValidateFile().
//
// @Parameters
// - filePath:  The path to the hash file to validate
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func ValidateHashFile(filePath string) error {
    // Validate the hash file path
    validPath, err := ValidatePath(filePath)
    if err != nil {
        return fmt.Errorf("improper hash_file_path specified in local config - %w", err)
    }

    // Validate the hash file
    err = ValidateFile(validPath)
    if err != nil {
        return fmt.Errorf("error validating hash file based on %s path - %w", validPath, err)
    }

    return nil
}


// Ensure the hash mask is present while a supported cracking mode is selcted.
//
// @Parameters
// - crackingMode:  The hashcat cracking mode
// - hashMask:  The hashcat mask to validate
//
// @Returns
// - true/false value depending on whether the hash mask is present
//   with a supported cracking mode
//
func ValidateHashMask(crackingMode string, hashMask string) bool {
    if hashMask == "" {
        return true
    }

    supportedModes := []string{"3", "6", "7"}
    // Check to see if passed in cracking mode is in supported modes and hashmask is present
    return data.StringSliceHasItem(supportedModes, crackingMode) && hashMask != ""
}


// Ensure the passed in hash type is a supported hashcat hash type.
//
// @Parameters
// - hashType:  the hash type to validate
//
// @Returns
// - true/false boolean depending on whether hash type is valid or not
//
func ValidateHashType(hashType string) bool {
    hashTypes := []string{"0", "10", "11", "12", "20", "21", "23", "30", "40", "50",
                          "60", "100", "101", "110", "111", "112", "120", "121", "122",
                          "123", "124", "130", "131", "132", "133", "140", "141", "150",
                          "160", "200", "300", "400", "500", "900", "1000", "1100", "1400",
                          "1410", "1420", "1421", "1430", "1431", "1440", "1441", "1450",
                          "1460", "1600", "1700", "1710", "1711", "1720", "1722", "1730",
                          "1731", "1740", "1750", "1760", "1800", "2400", "2410", "2500",
                          "2600", "2611", "2612", "2711", "2811", "3200", "3300", "3500",
                          "3610", "3710", "3711", "3720", "3721", "3800", "3910", "4010",
                          "4110", "4210", "4300", "4400", "4500", "4600", "4700", "4800",
                          "4900", "5000", "5100", "5200", "5300", "5400", "5500", "5600",
                          "5700", "5800", "6300", "6400", "6500", "6700", "6900", "7000",
                          "7100", "7200", "7300", "7400", "7600", "7900", "8400", "8900",
                          "9200", "9300", "9800", "10000", "10200", "10300", "11000",
                          "11100", "11200", "11400", "99999"}

    // Check to see if arg hash type is in the allowed hash types
    return data.StringSliceHasItem(hashTypes, hashType)
}


// Ensures the AWS IAM username is of proper format.
//
// @Parameters
// - iamUsername:  The username of the IAM to be validated
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func ValidateIamUsername(iamUsername string) error {
    // If the IAM username is not of proper format
    if !ReIamUsername.MatchString(iamUsername) {
        return errors.New("invalid IAM username, must be 1-64 characters and " +
                          "only contain alphanumeric characters and +=,.@-_")
    }

    return nil
}


// Ensures the passed in instance type is in the supported slice.
//
// @Parameters
// -instanceType:  The EC2 instance type to be used
//
// @Returns
// - true/false boolean depending on whether instance type is valid or not
//
func ValidateInstanceType(instanceType string) bool {
    var supportedInstances = []string{
        // === G4dn instances ===
        "g4dn.xlarge", "g4dn.2xlarge", "g4dn.4xlarge",
        "g4dn.8xlarge", "g4dn.12xlarge", "g4dn.16xlarge",

        // === G5 instances ===
        "g5.2xlarge", "g5.4xlarge", "g5.8xlarge",
        "g5.12xlarge", "g5.16xlarge", "g5.24xlarge",
        "g5.48xlarge",

        // === G6 instances ===
        "g6.xlarge", "g6.2xlarge", "g6.4xlarge",
        "g6.8xlarge", "g6.12xlarge", "g6.16xlarge",
        "g6.24xlarge", "g6.48xlarge",

        // === Gr6 instances ===
        "gr6.4xlarge", "gr6.8xlarge",

        // === G6e instances ===
        "g6e.xlarge", "g6e.2xlarge", "g6e.4xlarge",
        "g6e.8xlarge", "g6e.12xlarge", "g6e.16xlarge",
        "g6e.24xlarge", "g6e.48xlarge",

        // === P4d and P4de instances ===
        "p4d.24xlarge", "p4de.24xlarge",

        // === P5 and P5e instances ===
        "p5.48xlarge", "p5e.48xlarge",

        // === P6-B200 instances ===
        "p6-b200.48xlarge",
    }

    return data.StringSliceHasItem(supportedInstances, instanceType)
}


// Ensure the listener is above a non-privileged TCP port (over 1000).
//
// @Parameters
// - listenerPort:  The port to be validated
//
// @Returns
// - true/false boolean depending on whether the port is above 1000 or not
//
func ValidateListenerPort(listenerPort int) bool {
    return listenerPort > 1000
}


// Ensure the load dir path is valid and validate the load dir.
//
// @Paramters
// - dirPath:  Path to the load directory to be validated
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func ValidateLoadDir(dirPath string) error {
    // Validate the load directory path
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


// Ensure the passed in log mode is supported.
//
// @Parameters
// - logMode:  The log mode to be validated
//
// @Returns
// - true/false depending on whether log mode is supported or not
//
func ValidateLogMode(logMode string) bool {
    logModes := []string{"local", "cloudwatch", "both"}

    // Check to see if arg logging mode is in allowed modes
    return data.StringSliceHasItem(logModes, logMode)
}


// Ensure the passed in max file size is of raw bytes format or in
// unit format (KB, MB, GB). If in raw bytes it is simply converted to
// int64, but for unit format a conversion to raw bytes then int64.
//
// @Parameters
// - maxFileSize:  The max file size prior to parse and calculation/conversion
//
// @Returns
// - The converted int64 max file size as raw bytes
// - Error if it occurs, otherwise nil on success
//
func ValidateFileSize(maxFileSize string) (int64, error) {
    var byteSize int64
    var err error

    // Save string max file size to local variable ensuring
    // any units are uppercase (KB, MB, GB)
    maxFileSize = strings.ToUpper(maxFileSize)
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


// Ensure the passed in max size range is 50 percent or below.
//
// @Parameters
// - percentage:  The float percentage to validate
//
// @Returns
// - true/false boolean depending on whether the percentage
//   less than or equal to 50 or not
//
func ValidateMaxSizeRange(percentage float64) bool {
    return percentage <= 50.0
}


// Ensure the passed in max transfers is greater than zero.
//
// @Parameters
// - maxTransfers:  The number of allowed file transfer simultaniously
//
// @Returns
// - true/false boolean depending on whether the max transfers
//   is greater than 0 or not
func ValidateMaxTransfers(maxTransfers int32) bool {
    return maxTransfers > 0
}


// Ensure the passed in number instances is greater than zero.
//
// @Parameters
// - maxTransfers:  The number instances to allocate
//
// @Returns
// - true/false boolean depending on whether the number instances
//   is greater than 0 or not
func ValidateNumberInstances(numberInstances int) bool {
    return numberInstances > 0
}


// Cleans the passed in path and ensures it is of proper format.
//
// @Parameters
// - path:  The path to be validated
//
// @Returns
// - The validated path
// - Error if it occurs, otherwise nil on success
//
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

    // Validate path format with regex
    validPath := regexp.MustCompile(`^[a-zA-Z0-9\._\-\/]+$`).MatchString(cleanedPath)
    if !validPath {
        return "", fmt.Errorf("path %s contains invalid characters", path)
    }

    return cleanedPath, nil
}


// Ensure the passed in region is a valid AWS region.
//
// @Parameters
// - region:  The AWS region to be validated
//
// @Returns
// - true/false boolean depending on whether the AWS region is valid or not
//
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


// Validate the path to the ruleset file and the file itself via ValidateFile().
//
// @Parameters
// - filePath:  The path to the ruleset file to validate
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func ValidateRulesetFile(filePath string) error {
    // If the ruleset path is empty return early
    if filePath == "" {
        return nil
    }

    // Validate the ruleset file path
    validPath, err := ValidatePath(filePath)
    if err != nil {
        return fmt.Errorf("improper ruleset_path specified in local config - %w", err)
    }

    // Validate the ruleset file
    err = ValidateFile(validPath)
    if err != nil {
        return fmt.Errorf("error validating ruleset file based on %s path - %w", validPath, err)
    }

    return nil
}


// Ensures any security group IDs are of proper format.
//
// @Parameters
// - securityGroupIds:  Slice of security group IDs to validate
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func ValidateSecurityGroupIds(securityGroupIds []string) error {
    // Iterate through passed in list of security group IDs
    for _, sg := range securityGroupIds {
        // If the current security group is not of proper format
        if !ReSecurityGroupId.MatchString(sg) {
            return fmt.Errorf("invalid security group ID - %q", sg)
        }
    }

    return nil
}


// Ensures any security group names are of proper format.
//
// @Parameters
// - securityGroups:  Slice of security group names to validate
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func ValidateSecurityGroups(securityGroups []string) error {
    // Iterate through passed in list of security group names
    for _, name := range securityGroups {
        // If the security group name is not of proper length
        if len(name) == 0 || len(name) > 255 {
            return fmt.Errorf("invalid security group name: %q (must be 1–255 characters)", name)
        }

        // If the security group has "sg-" prefix reserve for IDs
        if len(name) >= 3 && name[:3] == "sg-" {
            return fmt.Errorf("invalid security group name: %q (cannot start with \"sg-\")", name)
        }

        // If the security group name is not of proper format
        if !ReSecurityGroupName.MatchString(name) {
            return fmt.Errorf("invalid security group name - %q", name)
        }
    }

    return nil
}


// Ensures the AWS subnet ID is of proper format.
//
// @Parameters
// - subnetId:  Subnet ID to validate
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func ValidateSubnetId(subnetId string) error {
    // Ensure the AWS subnet ID is of proper format
	if !ReSubnetId.MatchString(subnetId) {
		return fmt.Errorf("invalid subnet ID - %q", subnetId)
	}

	return nil
}


// Ensure the passed in workload is suppported by hashcat.
//
// @Parameters
// - workload:  The hashcat workload to be validated
//
// @Returns
// - true/false boolean depending on whether the workload is supported or not
//
func ValidateWorkload(workload string) bool {
    workloads := []string{"1", "2", "3", "4"}

    // Ensure the passed in workload is in workload slice
    return data.StringSliceHasItem(workloads, workload)
}
