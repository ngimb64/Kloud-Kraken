package conf

import (
	"fmt"
	"os"

	"github.com/ngimb64/Kloud-Kraken/internal/validate"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
	"gopkg.in/yaml.v3"
)

// The root tier of the YAML ties local and client configs
type AppConfig struct {
    ClientConfig ClientConfig `yaml:"client_config"`
    LocalConfig  LocalConfig  `yaml:"local_config"`
}

// Contains the YAML configuration for the client settings
type ClientConfig struct {
    ApplyOptimization bool   `yaml:"apply_optimization"`
    CharSet1          string `yaml:"char_set1"`
    CharSet2          string `yaml:"char_set2"`
    CharSet3          string `yaml:"char_set3"`
    CharSet4          string `yaml:"char_set4"`
    CrackingMode      string `yaml:"cracking_mode"`
    HashMask          string `yaml:"hash_mask"`
    HashType          string `yaml:"hash_type"`
    LogMode           string `yaml:"log_mode"`
    MaxFileSize       string `yaml:"max_file_size"`
    MaxFileSizeInt64  int64  `yaml:"-"`
    MaxTransfers      int32  `yaml:"max_transfers"`
    Workload          string `yaml:"workload"`
}

// Contains the YAML configuration for local server settings
type LocalConfig struct {
    CidrBlock           string   `yaml:"cidr_block"`
    HashFilePath        string   `yaml:"hash_file_path"`
    IamUsername         string   `yaml:"iam_username"`
    InstanceType        string   `yaml:"instance_type"`
    ListenerPort        int      `yaml:"listener_port"`
    LoadDir	   	        string   `yaml:"load_dir"`
    LocalTesting        bool     `yaml:"local_testing"`
    MaxMergingSize      string   `yaml:"max_merging_size"`
    MaxMergingSizeInt64 int64    `yaml:"-"`
    MaxSizeRange        float64  `yaml:"max_size_range"`
    NumberInstances     int32    `yaml:"number_instances"`
    Region              string   `yaml:"region"`
    RulesetPath         string   `yaml:"ruleset_path"`
}


// LoadConfig reads the YAML file and unmarshals it into AppConfig struct in
// memory, then validates the parsed data from local and client sections of yaml.
//
// @Parameters
//  - filePath:  The path to the YAML file to load
//
// @Returns
//  - The initialized AppConfig struct loaded with validated data
//  - Error if it occurs, otherwise nil on success
//
func LoadConfig(filePath string) (*AppConfig, error) {
    // Create a new AppConfig instance
    var config AppConfig

    // Read yaml file contents into memory
    yamlBytes, err := os.ReadFile(filePath)
    if err != nil {
        return nil, fmt.Errorf("reading config file:  %w", err)
    }

    // Decode the raw bytes into AppConfig struct
    err = yaml.Unmarshal(yamlBytes, &config)
    if err != nil {
        return nil, fmt.Errorf("parsing YAML:  %w", err)
    }

    // Validate client config section of YAML data
    err = validateClientConfig(&config.ClientConfig)
    if err != nil {
        return &config, err
    }

    // Validate local config section of YAML data
    err = validateLocalConfig(&config.LocalConfig)
    if err != nil {
        return &config, err
    }

    return &config, nil
}


// Takes the parsed data in ClientConfig struct and passes each struct member
// into its corresponding validation routine.
//
// @Parameters
//  - clientConfig:  The ClientConfig section of the parsed yaml data
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func validateClientConfig(clientConfig *ClientConfig) error {
    var err error

    // If the there are custom charsets but missing hash masks or improper mode
    if !validate.ValidateCharsets(clientConfig.CrackingMode, clientConfig.HashMask,
                                  clientConfig.CharSet1, clientConfig.CharSet2,
                                  clientConfig.CharSet3, clientConfig.CharSet4) {
        return fmt.Errorf("custom charsets specified with either missing hash mask or " +
                          "mode that does not support hash masks")
    }

    // If the cracking mode was not in supported modes
    if !validate.ValidateCrackingMode(clientConfig.CrackingMode) {
        return fmt.Errorf("improper cracking_mode specified")
    }

    // If the hash mask is present but not supported by cracking mode
    if !validate.ValidateHashMask(clientConfig.CrackingMode, clientConfig.HashMask) {
        return fmt.Errorf("hash_mask specified but not supported by cracking mode")
    }

    // If the hash type was not in supported types
    if !validate.ValidateHashType(clientConfig.HashType) {
        return fmt.Errorf("improper hash_type specified")
    }

    // If an improper region was specified in client config
    if !validate.ValidateLogMode(clientConfig.LogMode) {
        return fmt.Errorf("improper log_mode specified")
    }

    // Parse and convert the max file size to raw bytes from any units
    clientConfig.MaxFileSizeInt64, err = validate.ValidateFileSize(clientConfig.MaxFileSize)
    if err != nil {
        return fmt.Errorf("improper max_file_size - %w", err)
    }

    // If the max_transfers was less than one
    if !validate.ValidateMaxTransfers(clientConfig.MaxTransfers) {
        return fmt.Errorf("improper max_transfers specified")
    }

    // If the workload was not in supported profiles
    if !validate.ValidateWorkload(clientConfig.Workload) {
        return fmt.Errorf("improper workload specified")
    }

    return nil
}


// Takes the parsed data in LocalConfig struct and passes each struct member
// into its corresponding validation routine.
//
// @Parameters
//  - localConfig:  The LocalConfig section of the parsed yaml data
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func validateLocalConfig(localConfig *LocalConfig) error {
    // Ensure the CIDR block is of proper format if exists
    err := validate.ValidateCidrBlock(localConfig.CidrBlock)
    if err != nil {
        return err
    }

    // Ensure the hash file path exists
    err = validate.ValidateHashFile(localConfig.HashFilePath)
    if err != nil {
        return err
    }

    // Ensure the IAM username is valid
    err = validate.ValidateIamUsername(localConfig.IamUsername)
    if err != nil {
        return err
    }

    // Ensure instance type is in supported list
    if !validate.ValidateInstanceType(localConfig.InstanceType) {
        return fmt.Errorf("improper instance_type - %q", localConfig.InstanceType)
    }

    // If the listerner port is less than 1000
    if !validate.ValidateListenerPort(localConfig.ListenerPort) {
        return fmt.Errorf("listener_port must greater than 1000")
    }

    // Ensure the load directory exists and has files in it
    err = validate.ValidateLoadDir(localConfig.LoadDir)
    if err != nil {
        return err
    }

    // Parse and convert the max merging size to raw bytes from any units
    localConfig.MaxMergingSizeInt64, err = validate.ValidateFileSize(localConfig.MaxMergingSize)
    if err != nil {
        return fmt.Errorf("improper max_merging_size - %w", err)
    }

    // Ensure the max size range is less or equal to 50 percent
    if !validate.ValidateMaxSizeRange(localConfig.MaxSizeRange) {
        return fmt.Errorf("max_size_range greater than 50 percent")
    }

    // If the number of instances is less than one
    if !validate.ValidateNumberInstances(localConfig.NumberInstances) {
        return fmt.Errorf("number_instances must be a positive integer")
    }

    // Ensure a proper region was specified in the local config
    if !awsutils.ValidateRegion(localConfig.Region) {
        return fmt.Errorf("improper region specified")
    }

    // Ensure the ruleset file path exists
    err = validate.ValidateRulesetFile(localConfig.RulesetPath)
    if err != nil {
        return err
    }

    return nil
}
