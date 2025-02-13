package conf

import (
	"fmt"
	"log"
	"os"

	"github.com/ngimb64/Kloud-Kraken/internal/validate"
	"gopkg.in/yaml.v3"
)

// AppConfig is a wrapper that ties the local and client yaml configs
type AppConfig struct {
    LocalConfig  LocalConfig  `yaml:"local_config"`
    ClientConfig ClientConfig `yaml:"client_config"`
}

// LocalConfig contains the yaml configuration for local server settings
type LocalConfig struct {
    Region          string  `yaml:"region"`
    ListenerPort    int32   `yaml:"listener_port"`
    NumberInstances int32   `yaml:"number_instances"`
    LoadDir	   	    string  `yaml:"load_dir"`
    HashFilePath    string  `yaml:"hash_file_path"`
    RulesetPath     string  `yaml:"ruleset_path"`
    MaxSizeRange    float64 `yaml:"max_size_range"`
    LogPath         string  `yaml:"log_path"`
}

// ClientConfig contains the yaml configuration for the client settings
type ClientConfig struct {
    Region            string `yaml:"region"`
    MaxFileSize       string `yaml:"max_file_size"`
    MaxFileSizeInt64  int64  `yaml:"-"`              // Parsed later
    CrackingMode      string `yaml:"cracking_mode"`
    HashType          string `yaml:"hash_type"`
    ApplyOptimization bool   `yaml:"apply_optimization"`
    Workload          string `yaml:"workload"`
    CharSet1          string `yaml:"char_set1"`
    CharSet2          string `yaml:"char_set2"`
    CharSet3          string `yaml:"char_set3"`
    CharSet4          string `yaml:"char_set4"`
    HashMask          string `yaml:"hash_mask"`
    MaxTransfers      int32  `yaml:"max_transfers"`
    LogMode           string `yaml:"log_mode"`
    LogPath           string `yaml:"log_path"`
}


// LoadConfig reads the YAML file and unmarshals it into AppConfig struct in
// memory, then validates the parsed data from local and client sections of yaml.
//
// @Returns
// - The initialized AppConfig struct loaded with validated data
//
func LoadConfig(filePath string) *AppConfig {
    // Open the YAML file
    file, err := os.Open(filePath)
    if err != nil {
        log.Fatalf("Could not open YAML file:  %v", err)
    }
    // Close file on local exit
    defer file.Close()

    // Create a new AppConfig instance
    var config AppConfig

    // Decode YAML into AppConfig struct
    decoder := yaml.NewDecoder(file)
    err = decoder.Decode(&config)
    if err != nil {
        log.Fatalf("Could not decode YAML into AppConfig:  %v", err)
    }

    // Validate local config section of YAML data
    err = ValidateLocalConfig(&config.LocalConfig)
    if err != nil {
        log.Fatalf("Invalid local config:  %v", err)
    }

    // Validate client config section of YAML data
    err = ValidateClientConfig(&config.ClientConfig)
    if err != nil {
        log.Fatalf("Invalid client config:  %v", err)
    }

    return &config
}


// Takes the parsed data in LocalConfig struct and passes each
// struct member into its corresponding validation routine.
//
// @Parameters
// - localConfig:  The LocalConfig section of the parsed yaml data
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func ValidateLocalConfig(localConfig *LocalConfig) error {
    // Ensure a proper region was specified in the local config
    if !validate.ValidateRegion(localConfig.Region) {
        return fmt.Errorf("improper region specified")
    }

    // If the listerner port is less than 1000
    if !validate.ValidateListenerPort(localConfig.ListenerPort) {
        return fmt.Errorf("listener_port must greater than 1000")
    }

    // If the number of instances is less than one
    if !validate.ValidateNumberInstances(localConfig.NumberInstances) {
        return fmt.Errorf("number_instances must be a positive integer")
    }

    // Ensure the load directory exists and has files in it
    err := validate.ValidateLoadDir(localConfig.LoadDir)
    if err != nil {
        return err
    }

    // Ensure the hash file path exists
    err = validate.ValidateHashFile(localConfig.HashFilePath)
    if err != nil {
        return err
    }

    // Ensure the ruleset file path exists
    err = validate.ValidateRulesetFile(localConfig.RulesetPath)
    if err != nil {
        return err
    }

    // Ensure the max size range is less or equal to 50 percent
    if !validate.ValidateMaxSizeRange(localConfig.MaxSizeRange) {
        return fmt.Errorf("max_size_range greater than 50 percent")
    }

    // Ensure log path is proper format and reset ruleset path with validated
    localConfig.LogPath, err = validate.ValidatePath(localConfig.LogPath)
    if err != nil {
        return fmt.Errorf("improper log_path specified - %w", err)
    }

    return nil
}


// Takes the parsed data in ClientConfig struct and passes each
// struct member into its corresponding validation routine.
//
// @Parameters
// - clientConfig:  The ClientConfig section of the parsed yaml data
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func ValidateClientConfig(clientConfig *ClientConfig) error {
    var err error

    // If an improper region was specified in client config
    if !validate.ValidateRegion(clientConfig.Region) {
        return fmt.Errorf("improper region specified")
    }

    // Parse and convert the max file size to raw bytes from any units
    clientConfig.MaxFileSizeInt64, err = validate.ValidateMaxFileSize(clientConfig.MaxFileSize)
    if err != nil {
        return fmt.Errorf("improper max_file_size - %w", err)
    }

    // If the cracking mode was not in supported modes
    if !validate.ValidateCrackingMode(clientConfig.CrackingMode) {
        return fmt.Errorf("improper cracking_mode specified")
    }

    // If the hash type was not in supported types
    if !validate.ValidateHashType(clientConfig.HashType) {
        return fmt.Errorf("improper hash_type specified")
    }

    // If the workload was not in supported profiles
    if !validate.ValidateWorkload(clientConfig.Workload) {
        return fmt.Errorf("improper workload specified")
    }

    // If the there are custom charsets but missing hash masks or improper mode
    if !validate.ValidateCharsets(clientConfig.CrackingMode, clientConfig.HashMask,
                                  clientConfig.CharSet1, clientConfig.CharSet2,
                                  clientConfig.CharSet3, clientConfig.CharSet4) {
        return fmt.Errorf("custom charsets specified with either missing hash mask or " +
                          "mode that does not support hash masks")
    }

    // If the hash mask is present but not supported by cracking mode
    if !validate.ValidateHashMask(clientConfig.CrackingMode, clientConfig.HashMask) {
        return fmt.Errorf("hash_mask specified but not supported by cracking mode")
    }

    // If the max_transfers was less than one
    if !validate.ValidateMaxTransfers(clientConfig.MaxTransfers) {
        return fmt.Errorf("improper max_transfers specified")
    }

    // If an improper region was specified in client config
    if !validate.ValidateLogMode(clientConfig.LogMode) {
        return fmt.Errorf("improper log_mode specified")
    }

    // Ensure log path is of proper format
    clientConfig.LogPath, err = validate.ValidatePath(clientConfig.LogPath)
    if err != nil {
        return fmt.Errorf("improper log_path specified - %w", err)
    }

    return nil
}
