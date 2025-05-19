package validate_test

import (
	"crypto/sha512"
	"fmt"
	"os"
	"testing"

	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/internal/validate"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"github.com/stretchr/testify/assert"
)

func TestValidateBucketName(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    err := validate.ValidateBucketName("test-bucket")
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    err = validate.ValidateBucketName("#3$$3dsslmdv.12mvm_#")
    // Ensure the error occured
    assert.NotEqual(nil, err)

    err = validate.ValidateBucketName("10.10.10.10")
    // Ensure the error occured
    assert.NotEqual(nil, err)
}


func TestValidateCharsets(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Ensure invalid cracking mode fails
    assert.False(validate.ValidateCharsets("9", "?u?l?l?l?l?l?l?l?d",
                                           "testchars1", "testchars2"))
    // Ensure missing hash mask passes
    assert.True(validate.ValidateCharsets("3", "", ""))
    // Ensure proper args pass
    assert.True(validate.ValidateCharsets("3", "?u?l?l?l?l?l?l?l?d",
                                          "testchars1", "testchar2"))
}


func TestValidateConfigPath(t *testing.T) {
    configPath := "../../config/config.yml"
    // Test with the default yaml config file
    validate.ValidateConfigPath(&configPath)
}


func TestValidateCrackingMode(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    truths := []string{"0", "3", "6", "7"}
    // Iterate through slice of truths and test them
    for _, truth := range truths {
        assert.True(validate.ValidateCrackingMode(truth))
    }

    falacies := []string{"-1", "1", "4", "5", "8"}
    // Iterate through slice of truths and test them
    for _, falacy := range falacies {
        assert.False(validate.ValidateCrackingMode(falacy))
    }
}


func TestValidateDir(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    testDir := "testingDir"
    // Create the test directory
    err := os.Mkdir(testDir, os.ModePerm)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    testFile := "testFile.txt"
    // Format the file path with test dir
    filePath := fmt.Sprintf("%s/%s", testDir, testFile)
    // Open the file with write permissions
    file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Make a byte buffer based off iteration index
    buffer := make([]byte, 64)
    // Fill the buffer up with random data
    data.GenerateRandomBytes(buffer, 64)
    // Write the random data to the output file
    bytesWrote, err := file.Write(buffer)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close the file after data has been written
    file.Close()
    // Ensure the bytes wrote matches the buffer size
    assert.Equal(bytesWrote, 64)

    // Validate the created test dir
    err = validate.ValidateDir(testDir)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Delete the test dir after it has been validated
    err = os.RemoveAll(testDir)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
}


func TestValidateFile(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    testDir := "testingDir"
    // Create the test directory
    err := os.Mkdir(testDir, os.ModePerm)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    testFile := "testFile.txt"
    // Format the file path with test dir
    filePath := fmt.Sprintf("%s/%s", testDir, testFile)
    // Open the file with write permissions
    file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Make a byte buffer based off iteration index
    buffer := make([]byte, 64)
    // Fill the buffer up with random data
    data.GenerateRandomBytes(buffer, 64)
    // Write the random data to the output file
    bytesWrote, err := file.Write(buffer)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close the file after data has been written
    file.Close()
    // Ensure the bytes wrote matches the buffer size
    assert.Equal(bytesWrote, 64)

    // Validate the created test file inside the test dir
    err = validate.ValidateFile(filePath)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Delete the test dir after it has been validated
    err = os.RemoveAll(testDir)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
}


func TestValidateHashFile(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    testDir := "testingDir"
    // Create the test directory
    err := os.Mkdir(testDir, os.ModePerm)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    testFile := "testHashFile"
    // Format the file path with test dir
    filePath := fmt.Sprintf("%s/%s", testDir, testFile)
    // Open the file with write permissions
    file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Generate hash for hash file
    hasher := sha512.New()
    hasher.Write([]byte("cassandra"))

    // Write the hash to the hash file
    bytesWrote, err := file.Write(hasher.Sum(nil))
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close the file after data has been written
    file.Close()
    // Ensure the bytes wrote matches the buffer size
    assert.Equal(bytesWrote, 64)

    // Validate the created test file inside the test dir
    err = validate.ValidateHashFile(filePath)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Delete the test dir after it has been validated
    err = os.RemoveAll(testDir)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
}


func TestValidateHashMask(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    truths := []string{"3", "6", "7"}
    // Iterate through slice of truths and test them
    for _, truth := range truths {
        assert.True(validate.ValidateHashMask(truth, "?u?l?l?l?l?l?l?l?d"))
    }

    falacies := []string{"-1", "0", "1"}
    // Iterate through slice of falacies and test them
    for _, falacy := range falacies {
        assert.False(validate.ValidateHashMask(falacy, "?u?l?l?l?l?l?l?l?d"))
    }

    truths = []string{"4", "5", "8"}
    // Iterate through slice of truths and test them
    for _, truth := range truths {
        assert.True(validate.ValidateHashMask(truth, ""))
    }
}


func TestValidateHashType(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    truths := []string{"0", "1000", "6400", "10000", "99999"}
    // Iterate through slice of truths and test them
    for _, truth := range truths {
        assert.True(validate.ValidateHashType(truth))
    }

    falacies := []string{"-1", "333", "11580", "10000000"}
    // Iterate through slice of truths and test them
    for _, falacy := range falacies {
        assert.False(validate.ValidateHashType(falacy))
    }
}


func TestValidateInstanceType(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Try test with proper value
    isType := validate.ValidateInstanceType("p3.8xlarge")
    assert.True(isType)

    // Try test with bad value
    isType = validate.ValidateInstanceType("blahblah")
    assert.False(isType)
}


func TestValidateListenerPort(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Test with a port below or equal 1000
    assert.False(validate.ValidateListenerPort(420))
    // Test with a port above 1000
    assert.True(validate.ValidateListenerPort(4444))
}


func TestValidateLoadDir(t *testing.T) {
   // Make reusable assert instance
   assert := assert.New(t)

   testDir := "testingDir"
   // Create the test directory
   err := os.Mkdir(testDir, os.ModePerm)
   // Ensure the error is nil meaning successful operation
   assert.Equal(nil, err)

   testFile := "testFile.txt"
   // Format the file path with test dir
   filePath := fmt.Sprintf("%s/%s", testDir, testFile)
   // Open the file with write permissions
   file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)
   // Ensure the error is nil meaning successful operation
   assert.Equal(nil, err)

   // Make a byte buffer based off iteration index
   buffer := make([]byte, 64)
   // Fill the buffer up with random data
   data.GenerateRandomBytes(buffer, 64)
   // Write the random data to the output file
   bytesWrote, err := file.Write(buffer)
   // Ensure the error is nil meaning successful operation
   assert.Equal(nil, err)
   // Close the file after data has been written
   file.Close()
   // Ensure the bytes wrote matches the buffer size
   assert.Equal(bytesWrote, 64)

   // Validate the created test dir
   err = validate.ValidateLoadDir(testDir)
   // Ensure the error is nil meaning successful operation
   assert.Equal(nil, err)

   // Delete the test dir after it has been validated
   err = os.RemoveAll(testDir)
   // Ensure the error is nil meaning successful operation
   assert.Equal(nil, err)
}


func TestValidateLogMode(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    truths := []string{"local", "cloudwatch", "both"}
    // Iterate through slice of truths and test them
    for _, truth := range truths {
        assert.True(validate.ValidateLogMode(truth))
    }

    falacies := []string{"test", "string", "nonsense"}
    // Iterate through slice of truths and test them
    for _, falacy := range falacies {
        assert.False(validate.ValidateLogMode(falacy))
    }
}


func TestValidateFileSize(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    tests := []struct {
        input   string
        output  int64
    } {
        {"100mb", int64(100 * globals.MB)},
        {"10GB", int64(10 * globals.GB)},
        {"512kb", int64(512 * globals.KB)},
    }

    // Iterate through slice of test structs
    for _, test := range tests {
        // Use struct member as input to call function
        outputSize, err := validate.ValidateFileSize(test.input)
        // Ensure the error is nil meaning successful operation
        assert.Equal(nil, err)
        // Ensure the expected test size equals function output
        assert.Equal(test.output, outputSize)
    }
}


func TestValidateMaxSizeRange(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Test a range above 50%
    assert.False(validate.ValidateMaxSizeRange(75.0))
    // Test a range at 50%
    assert.True(validate.ValidateMaxSizeRange(50.0))
    // Test a range below 50%
    assert.True(validate.ValidateMaxSizeRange(15.0))
}


func TestValidateMaxTransfers(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Test negative number
    assert.False(validate.ValidateMaxTransfers(-1))
    // Test zero value
    assert.False(validate.ValidateMaxTransfers(0))
    // Test positive value
    assert.True(validate.ValidateMaxTransfers(3))
}


func TestValidateNumberInstances(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Test negative number
    assert.False(validate.ValidateNumberInstances(-1))
    // Test zero value
    assert.False(validate.ValidateNumberInstances(0))
    // Test positive value
    assert.True(validate.ValidateNumberInstances(3))
}


func TestValidatePath(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    testPath := ""
    // Begin with empty test path
    _, err := validate.ValidatePath(testPath)
    // Ensure the error is not nil since no path
    assert.NotEqual(err, nil)

    testPath = "./test//path"
    // Run test with path that needs cleansing
    resultPath, err := validate.ValidatePath(testPath)
    // Ensure the error is nil meaning successful operation
    assert.Equal(err, nil)
    // Ensure the path was properly cleansed
    assert.Equal("test/path", resultPath)

    testPath = "./test/../path"
    // Run test with path that needs cleansing
    resultPath, err = validate.ValidatePath(testPath)
    // Ensure the error is nil meaning successful operation
    assert.Equal(err, nil)
    // Ensure that ../ and before are removed
    assert.Equal("path", resultPath)

    testPath = "\\te!@/#st\\/p$#%345ath/"
    // Run test with path to fail regex validation
    _, err = validate.ValidatePath(testPath)
    // Ensure the error is not nil since slash on end
    assert.NotEqual(err, nil)
}


func TestValidateRegion(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    truths := []string{"us-east-1", "us-east-2", "us-west-2"}
    // Iterate through slice of truths and test them
    for _, truth := range truths {
        assert.True(validate.ValidateRegion(truth))
    }

    falacies := []string{"test", "string", "nonsense"}
    // Iterate through slice of truths and test them
    for _, falacy := range falacies {
        assert.False(validate.ValidateRegion(falacy))
    }
}


func TestValidateRulesetFile(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    testDir := "testingDir"
    // Create the test directory
    err := os.Mkdir(testDir, os.ModePerm)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    testFile := "testFile.txt"
    // Format the file path with test dir
    filePath := fmt.Sprintf("%s/%s", testDir, testFile)
    // Open the file with write permissions
    file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Make a byte buffer based off iteration index
    buffer := make([]byte, 64)
    // Fill the buffer up with random data
    data.GenerateRandomBytes(buffer, 64)
    // Write the random data to the output file
    bytesWrote, err := file.Write(buffer)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
    // Close the file after data has been written
    file.Close()
    // Ensure the bytes wrote matches the buffer size
    assert.Equal(bytesWrote, 64)

    // Validate the created test file inside the test dir
    err = validate.ValidateRulesetFile(filePath)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Delete the test dir after it has been validated
    err = os.RemoveAll(testDir)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)
}


func TestValidateWorkload(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    truths := []string{"1", "2", "3", "4"}
    // Iterate through slice of truths and test them
    for _, truth := range truths {
        assert.True(validate.ValidateWorkload(truth))
    }

    falacies := []string{"5", "6", "0", "-1"}
    // Iterate through slice of truths and test them
    for _, falacy := range falacies {
        assert.False(validate.ValidateWorkload(falacy))
    }
}
