package config

import (
	"fmt"
	"log"
	"os"

	"github.com/ngimb64/Kloud-Kraken/internal/validate"
	"gopkg.in/yaml.v3"
)

// AppConfig is a wrapper for the configuration
type AppConfig struct {
    LocalConfig  LocalConfig  `yaml:"local_config"`
    ClientConfig ClientConfig `yaml:"client_config"`
}

// LocalConfig contains the configuration for local server settings
type LocalConfig struct {
    Region		    string `yaml:"region"`
    ListenerPort    int32  `yaml:"listener_port"`
    NumberInstances int    `yaml:"number_instances"`
    LoadDir	   	    string `yaml:"load_dir"`
    HashFilePath    string `yaml:"hash_file_path"`
    LogPath		    string `yaml:"log_path"`
}

// ClientConfig contains the configuration for the client settings
type ClientConfig struct {
    Region			 string `yaml:"region"`
    MaxFileSize    	 string `yaml:"max_file_size"`
    MaxFileSizeInt64 int64  `yaml:"-"`              // Parsed later
    LogMode			 string `yaml:"log_mode"`
    LogPath			 string `yaml:"log_path"`
    CrackingMode     string `yaml:"cracking_mode"`
    HashType         string `yaml:"hash_type"`
}


// LoadConfig reads the YAML file and unmarshals it into AppConfig
func LoadConfig(filePath string) *AppConfig {
    // Open the YAML file
    file, err := os.Open(filePath)
    if err != nil {
        log.Fatalf("Could not open YAML file:  %w", err)
    }
    // Close file on local exit
    defer file.Close()

    // Create a new AppConfig instance
    var config AppConfig

    // Decode YAML into AppConfig struct
    decoder := yaml.NewDecoder(file)
    err = decoder.Decode(&config)
    if err != nil {
        log.Fatalf("Could not decode YAML into AppConfig:  %w", err)
    }

    // Validate local config section of YAML data
    err = ValidateLocalConfig(&config.LocalConfig)
    if err != nil {
        log.Fatalf("Invalid local config:  %w", err)
    }

    // Validate client config section of YAML data
    err = ValidateClientConfig(&config.ClientConfig)
    if err != nil {
        log.Fatalf("Invalid client config:  %w", err)
    }

    return &config
}


func ValidateLocalConfig(localConfig *LocalConfig) error {
    // Ensure a proper region was specified in the local config
    if !validate.ValidateRegion(localConfig.Region) {
        return fmt.Errorf("improper region specified in local config")
    }

    // Ensure the listerner port is greater than 1000
    if !validate.ValidateListenerPort(localConfig.ListenerPort) {
        return fmt.Errorf("listener_port must greater than 1000")
    }

    // Ensure the number of instances is a positive integer
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

    // Ensure log path is of proper format
    logPath, err := validate.ValidatePath(localConfig.LogPath)
    if err != nil {
        return fmt.Errorf("improper log_path specified in local config - %w", err)
    }

    // Reset the logging path with validated clean path
    localConfig.LogPath = logPath

    return nil
}


func ValidateClientConfig(clientConfig *ClientConfig) error {
    // If an improper region was specified in client config
    if !validate.ValidateRegion(clientConfig.Region) {
        return fmt.Errorf("improper region specified in client config")
    }

    // Parse and convert the max file size to raw bytes from any units
    fileSize, err := validate.ValidateMaxFileSize(clientConfig.MaxFileSize)
    if err != nil {
        return fmt.Errorf("improper max_file_size in client config - %w", err)
    }

    // Prior to validation set the int64 max file size in client config
    clientConfig.MaxFileSizeInt64 = fileSize

    // If an improper region was specified in client config
    if !validate.ValidateLogMode(clientConfig.LogMode) {
        return fmt.Errorf("improper log_mode specified in client config")
    }

    // Ensure log path is of proper format
    logPath, err := validate.ValidatePath(clientConfig.LogPath)
    if err != nil {
        return fmt.Errorf("improper log_path specified in client config - %w", err)
    }

    // Reset the logging path with validated clean path
    clientConfig.LogPath = logPath

    // TODO:  validation methods for cracking_mode and hash_type


    return nil
}
