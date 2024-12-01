package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"gopkg.in/yaml.v3"
)

// AppConfig is a wrapper for the configuration
type AppConfig struct {
	LocalConfig  LocalConfig  `yaml:"local_config"`
	ClientConfig ClientConfig `yaml:"client_config"`
}

// LocalConfig contains the configuration for local server settings
type LocalConfig struct {
	Region		   string `yaml:"region"`
	ListenerPort   int    `yaml:"listener_port"`
	MaxConnections int    `yaml:"max_connections"`
	LoadDir	   	   string `yaml:"load_dir"`
	HashFilePath   string `yaml:"hash_file_path"`
	LogPath		   string `yaml:"log_path"`
}

// ClientConfig contains the configuration for the client settings
type ClientConfig struct {
	Region			 string `yaml:"region"`
	MaxFileSize    	 string `yaml:"max_file_size"`
	MaxFileSizeInt64 int64 `yaml:"-"`  			   // Parsed later
	LogMode			 string `yaml:"log_mode"`
	LogPath			 string `yaml:"log_path"`
}


// LoadConfig reads the YAML file and unmarshals it into AppConfig
func LoadConfig(filePath string) (*AppConfig, error) {
	// Open the YAML file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not open YAML file => %v", err)
	}
	// Close file on local exit
	defer file.Close()

	// Create a new AppConfig instance
	var config AppConfig

	// Decode YAML into AppConfig struct
	decoder := yaml.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("could not decode YAML into AppConfig => %v", err)
	}

	// Validate local config section of YAML data
	err = ValidateLocalConfig(&config.LocalConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid local config => %v", err)
	}

	// Validate client config section of YAML data
	err = ValidateClientConfig(&config.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid client config => %v", err)
	}

	return &config, nil
}


func ValidateLocalConfig(localConfig *LocalConfig) error {
	// TODO:  add logic to validate the aws region

	// If the listener port is less than or equal to 1000
	if localConfig.ListenerPort <= 1000 {
		return fmt.Errorf("listener_port must greater than 1000")
	}

	// If the max connections is less than or equal to 0
	if localConfig.MaxConnections <= 0 {
		return fmt.Errorf("max_connections must be a positive integer")
	}

	// Check to see if the load directory exists and has files in it
	exists, isDir, err := disk.PathExists(localConfig.LoadDir)
	if err != nil {
		return err
	}

	// If the load dir path does not exist or is not a directory
	if !exists || !isDir {
		return fmt.Errorf("load dir path does not exist or is a file")
	}

	// Check to see if the hash file exists
	exists, _, err = disk.PathExists(localConfig.HashFilePath)
	if err != nil {
		return err
	}

	// If the hash file path does not exist
	if !exists {
		return fmt.Errorf("hash file path does not exist")
	}

	return nil
}


func ValidateClientConfig(clientConfig *ClientConfig) error {
	// TODO:  add logic to validate the aws region

	var byteSize int64
	var err error
	// Save string max file size to local variable ensuring
	// any units are lowercase (MB, GB, etc.)
	maxFileSize := strings.ToLower(clientConfig.MaxFileSize)
	// Check to see if the max files size contains a conversion unit
	sliceContains := data.StringSliceContains(globals.FILE_SIZE_TYPES, maxFileSize)

	// If the slice contains a data unit to be converted to raw bytes
	if sliceContains {
		// Split the size from the unit type
		size, unit, err := data.ParseFileSizeType(maxFileSize)
		if err != nil {
			log.Fatalf("Error parsing file size unit:  %v", err)
		}
		// Pass the size and unit to calculate to raw bytes
		byteSize = data.ToBytes(size, unit)
	// If the file size seems to already be in bytes
	} else {
		// Attempt to convert it straight to int64
		byteSize, err = strconv.ParseInt(maxFileSize, 10, 64)
		if err != nil {
			log.Fatalf("Error converting string to int64:  %v", err)
		}
	}

	// If the converted max file size is less than or equal to 0
	if byteSize <= 0 {
		fmt.Errorf("Converted max_file_size is less than or equal to 0")
	}

	// Assign the converted max file size to struct key
	clientConfig.MaxFileSizeInt64 = byteSize

	// TODO:  add logic to validate the log mode

	return nil
}
