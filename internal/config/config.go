package config

import (
	"fmt"
	"os"
	"strconv"
)

// AppConfig stores the parsed configuration
type AppConfig struct {
	IpAddress      string
	Port           int
	MaxFileSize    string
	MaxFileSizeInt uint64
}


// NewAppConfig creates and returns a new AppConfig
func NewAppConfig(ipAddress string, port int, maxFileSize string) *AppConfig {
	return &AppConfig{
		IpAddress: 	 ipAddress,
		Port:     	 port,
		MaxFileSize: maxFileSize,
	}
}


// Validate checks if the AppConfig is valid
func Validate(config *AppConfig) error {
	// TODO: add logic to validate ip IpAddress

	// Validate Port (must be a positive integer)
	if config.Port <= 1000 || config.Port > 65535 {
		return fmt.Errorf("port must be a valid positive integer between 1000 and 65535")
	}

	// TODO:  add logic to validate MaxFile Size and implement system to support
	//		  raw bytes, KB, MB, and GB (convert all to raw bytes)

	// Convert the max file size string to integer
	numberConversion, err := strconv.ParseUint(config.MaxFileSize, 10, 64)
	// If there is an error converting string number to unsigned integer
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// Assign the converted max file size to struct key
	config.MaxFileSizeInt = numberConversion

	return nil
}
