package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ngimb64/Kloud-Kraken/internal/config"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

	// Get the current working directory
	path, err := os.Getwd()
	// Ensure the error is nil meaning successful operation
	assert.Equal(nil, err)

	testDir := fmt.Sprintf("%s/testdir", path)
	// Create the test directory for the data loaded by the config file
	err = os.Mkdir(testDir, os.ModePerm)
	// Ensure the error is nil meaning successful operation
	assert.Equal(nil, err)

    testFiles := []string{filepath.Join(testDir, "hashes"),
						  filepath.Join(testDir, "ruleset")}

    // Iterate through slice of test files
    for _, fileName := range testFiles {
        // Create the current file
        file, err := os.Create(fileName)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)

		// Make a byte buffer based off iteration index
		buffer := make([]byte, 64)
		// Fill the buffer up with random data
		data.GenerateRandomBytes(buffer, 64)
        // Write the test strings to current file
        bytesWrote, err := file.Write(buffer)
		// Ensure the error is nil meaning successful operation
		assert.Equal(nil, err)
		// Ensure the buffer of random data matches bytes wrote
		assert.Equal(len(buffer), bytesWrote)
        // Close the file after data is written
        file.Close()
    }

	yamlPath := "testdata.yml"
	testData := fmt.Sprintf(`
local_config:
  region: "us-east-1"
  listener_port: 6969
  number_instances: 3
  load_dir: "%s"
  hash_file_path: "%s"
  ruleset_path: "%s"
  max_size_range: 25.0
  log_path: "KloudKraken.log"

client_config:
  region: "us-west-1"
  max_file_size: "100MB"
  cracking_mode: "3"
  hash_type: "1000"
  apply_optimization: true
  workload: "4"
  char_set1: "charset1"
  char_set2: "charset2"
  char_set3: "charset3"
  char_set4: "charset4"
  hash_mask: "charset5"
  max_transfers: 2
  log_mode: "local"
  log_path: "KloudKraken.log"
`, testDir, testFiles[0], testFiles[1])
	// Writing the YAML string to a file
	err = os.WriteFile(yamlPath, []byte(testData), 0644)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

	// Load the config into AppConfig struct
	config := config.LoadConfig(yamlPath)

	// Validate local config fields to original data
	assert.Equal("us-east-1", config.LocalConfig.Region)
	assert.Equal(int32(6969), config.LocalConfig.ListenerPort)
	assert.Equal(int32(3), config.LocalConfig.NumberInstances)
	assert.Equal(testDir, config.LocalConfig.LoadDir)
	assert.Equal(testFiles[0], config.LocalConfig.HashFilePath)
	assert.Equal(testFiles[1], config.LocalConfig.RulesetPath)
	assert.Equal(25.0, config.LocalConfig.MaxSizeRange)
	assert.Equal("KloudKraken.log", config.LocalConfig.LogPath)

	// Validate client config fields to original data
	assert.Equal("us-west-1", config.ClientConfig.Region)
	assert.Equal("100MB", config.ClientConfig.MaxFileSize)
	assert.Equal("3", config.ClientConfig.CrackingMode)
	assert.Equal("1000", config.ClientConfig.HashType)
	assert.True(config.ClientConfig.ApplyOptimization)
	assert.Equal("4", config.ClientConfig.Workload)
	assert.Equal("charset1", config.ClientConfig.CharSet1)
	assert.Equal("charset2", config.ClientConfig.CharSet2)
	assert.Equal("charset3", config.ClientConfig.CharSet3)
	assert.Equal("charset4", config.ClientConfig.CharSet4)
	assert.Equal(int32(2), config.ClientConfig.MaxTransfers)
	assert.Equal("local", config.ClientConfig.LogMode)
	assert.Equal("KloudKraken.log", config.ClientConfig.LogPath)

	// Append the yaml data file to test file for deletion
	testFiles = append(testFiles, yamlPath)

	// Iterate through test files to be deleted
	for _, file := range testFiles {
		// Delete the current file
		err = os.Remove(file)
		// Ensure the error is nil meaning successful operation
		assert.Equal(nil, err)
	}

	// Delete the test dir where test files reside
	err = os.Remove(testDir)
	// Ensure the error is nil meaning successful operation
	assert.Equal(nil, err)
}
