package conf_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/internal/globals"
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


    // TODO:  add security_group_ids, security_groups, and subnet_id


    yamlPath := "testdata.yml"
    testData := fmt.Sprintf(`
local_config:
  account_id: "123456789"
  bucket_name: "test-bucket"
  hash_file_path: "%s"
  iam_username: "doug"
  instance_type: "t2-micro"
  listener_port: 6969
  load_dir: "%s"
  local_testing: true
  log_path: "KloudKraken.log"
  max_merging_size: "50MB"
  max_size_range: 25.0
  number_instances: 3
  region: "us-east-1"
  ruleset_path: "%s"
  security_group_ids: ["sg-01234567", "sg-0a1b2c3d4e5f6a7b8",
                       "sg-abcdef1234567890abcdef]
  security_groups: ["my-security-group", "web.server@frontend"]
  subnet_id: "subnet-0a1b2c3d4e5f6a7b8"


client_config:
  apply_optimization: true
  char_set1: "charset1"
  char_set2: "charset2"
  char_set3: "charset3"
  char_set4: "charset4"
  cracking_mode: "3"
  hash_mask: "?u?l?l?l?l?l?l?l?d"
  hash_type: "1000"
  log_mode: "local"
  log_path: "KloudKraken.log"
  max_file_size: "100MB"
  max_transfers: 2
  region: "us-west-1"
  workload: "4"
`, testFiles[0], testDir, testFiles[1])
    // Writing the YAML string to a file
    err = os.WriteFile(yamlPath, []byte(testData), 0644)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Load the config into AppConfig struct
    config := conf.LoadConfig(yamlPath)

    // Validate local config fields to original data
    assert.Equal("123456789", config.LocalConfig.AccountId)
    assert.Equal("test-bucket", config.LocalConfig.BucketName)
    assert.Equal(testFiles[0], config.LocalConfig.HashFilePath)
    assert.Equal("doug", config.LocalConfig.IamUsername)
    assert.Equal("t2-micro", config.LocalConfig.InstanceType)
    assert.Equal(6969, config.LocalConfig.ListenerPort)
    assert.Equal(testDir, config.LocalConfig.LoadDir)
    assert.True(config.LocalConfig.LocalTesting)
    assert.Equal("KloudKraken.log", config.LocalConfig.LogPath)
    assert.Equal("50MB", config.LocalConfig.MaxMergingSize)
    assert.Equal(int64(50 * globals.MB), config.LocalConfig.MaxMergingSizeInt64)
    assert.Equal(25.0, config.LocalConfig.MaxSizeRange)
    assert.Equal(3, config.LocalConfig.NumberInstances)
    assert.Equal("us-east-1", config.LocalConfig.Region)
    assert.Equal(testFiles[1], config.LocalConfig.RulesetPath)
    assert.Equal(3, len(config.LocalConfig.SecurityGroupIds))
    assert.Equal(2, len(config.LocalConfig.SecurityGroups))
    assert.Equal("subnet-0a1b2c3d4e5f6a7b8", config.LocalConfig.SubnetId)

    // Validate client config fields to original data
    assert.True(config.ClientConfig.ApplyOptimization)
    assert.Equal("charset1", config.ClientConfig.CharSet1)
    assert.Equal("charset2", config.ClientConfig.CharSet2)
    assert.Equal("charset3", config.ClientConfig.CharSet3)
    assert.Equal("charset4", config.ClientConfig.CharSet4)
    assert.Equal("3", config.ClientConfig.CrackingMode)
    assert.Equal("?u?l?l?l?l?l?l?l?d", config.ClientConfig.HashMask)
    assert.Equal("1000", config.ClientConfig.HashType)
    assert.Equal("local", config.ClientConfig.LogMode)
    assert.Equal("KloudKraken.log", config.ClientConfig.LogPath)
    assert.Equal("100MB", config.ClientConfig.MaxFileSize)
    assert.Equal(int64(100 * globals.MB), config.ClientConfig.MaxFileSizeInt64)
    assert.Equal(int32(2), config.ClientConfig.MaxTransfers)
    assert.Equal("us-west-1", config.ClientConfig.Region)
    assert.Equal("4", config.ClientConfig.Workload)

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
