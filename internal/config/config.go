package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// AppConfig is a wrapper for the configuration
type AppConfig struct {
	LocalConfig  LocalConfig  `yaml:"local_config"`
	ClientConfig ClientConfig `yaml:"client_config"`
}

// LocalConfig contains the configuration for local server settings
type LocalConfig struct {
	ListenerPort   int    `yaml:"listener_port"`
	MaxConnections int    `yaml:"max_connections"`
	LoadDir	   	   string `yaml:"load_dir"`
}

// ClientConfig contains the configuration for the client settings
type ClientConfig struct {
	MaxFileSize    string `yaml:"max_file_size"`
	MaxFileSizeInt uint64 `yaml:"-"`			 // Parsed later
}


// NewAppConfig creates and returns a new AppConfig nested structure
func NewAppConfig(listenerPort int, maxConnections int, clientPort int,
				  ipAddress string, maxFileSize string, loadDir string) *AppConfig {
	return &AppConfig{
		LocalConfig: LocalConfig{
			ListenerPort:   listenerPort,
			MaxConnections: maxConnections,
			LoadDir:		loadDir,
		},
		ClientConfig: ClientConfig{
			MaxFileSize: 	maxFileSize,
			MaxFileSizeInt: 0,
		},
	}
}


// LoadConfig reads the YAML file and unmarshals it into AppConfig
func LoadConfig(filePath string) (*AppConfig, error) {
	// Open the YAML file
	file, err := os.Open(filePath)
	// If there is an error opening the YAML file
	if err != nil {
		return nil, fmt.Errorf("could not open YAML file: %v", err)
	}
	// Close file on local exit
	defer file.Close()

	// Create a new AppConfig instance
	var config AppConfig

	// Decode YAML into AppConfig struct
	decoder := yaml.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("could not decode YAML into AppConfig: %v", err)
	}

	// Validate local config section of YAML data
	err = ValidateLocalConfig(&config.LocalConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid local config: %v", err)
	}

	// Validate client config section of YAML data
	err = ValidateClientConfig(&config.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid client config: %v", err)
	}

	return &config, nil
}


func ValidateLocalConfig(localConfig *LocalConfig) error {
	// If the listener port is less than or equal to 1000
	if localConfig.ListenerPort <= 1000 {
		return fmt.Errorf("listener_port must greater than 1000")
	}

	// If the max connections is less than or equal to 0
	if localConfig.MaxConnections <= 0 {
		return fmt.Errorf("max_connections must be a positive integer")
	}

	// Add validation logic for load_dir to ensure it exists and has files

	return nil
}


func ValidateClientConfig(clientConfig *ClientConfig) error {
	// Save string max file size to local variable
	maxFileSize := clientConfig.MaxFileSize

	// TODO:  add logic to validate MaxFile Size and implement system to support
	//		  raw bytes, KB, MB, and GB (convert all to raw bytes)

	// TODO:  after above logic is implemented, adjust below string-int conversion
	//		  to only occur is the data type is string

	// Convert the max file size string to integer
	numberConversion, err := strconv.ParseUint(maxFileSize, 10, 64)
	// If there is an error converting string number to unsigned integer
	if err != nil {
		fmt.Println("Error converting max_file_size:", err)
		os.Exit(1)
	}

	// If the converted max file size is less than or equal to 0
	if numberConversion <= 0 {
		fmt.Println("Error converting")
		os.Exit(1)
	}

	// Assign the converted max file size to struct key
	clientConfig.MaxFileSizeInt = numberConversion

	return nil
}
