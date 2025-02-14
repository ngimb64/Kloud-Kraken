package kloudlogs_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/ngimb64/Kloud-Kraken/pkg/kloudlogs"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestLogToMap(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    testJsonStr := "{\"key1\":\"value1\",\"key2\":\"value2\"," +
                   "\"key3\":\"value3\",\"key4\":\"value4\"}"
    jsonMap, err := kloudlogs.LogToMap(testJsonStr)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    for counter := 1; counter <= 4; counter++ {
        // Compared the formatted value to the return value of
        // the map based on the key used to access the value
        assert.Equal(fmt.Sprintf("%s%d", "value", counter),
                     jsonMap[fmt.Sprintf("%s%d", "key", counter)])
    }
}


func TestLogMessage(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    region := "test-region"
    awsAccessKey := "blah"
    awsSecretKey := "blah"
    awsCreds := credentials.NewStaticCredentialsProvider(awsAccessKey, awsSecretKey, "")

    // Load default config and override with custom credentials and region
    awsConfig, err := config.LoadDefaultConfig(
        context.TODO(),
        config.WithRegion(region),
        config.WithCredentialsProvider(awsCreds),
    )
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    logFile := "testlog.log"
    // Initialize the LoggerManager based on the flags
    logMan, err := kloudlogs.NewLoggerManager("local", logFile, awsConfig, false)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    logArgs := []any{zap.String("key1", "value1"), zap.String("key2", "value2"),
                     zap.String("key3", "value3"), zap.String("key4", "value4")}
    // Log the hashcat output with kloudlogs
    kloudlogs.LogMessage(logMan, "info", "TestLogMessage test message", logArgs...)

    // Get the file info
    fileInfo, err := os.Stat(logFile)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Delete the test log file
    err = os.Remove(logFile)
    // Ensure the error is nil meaning successful operation
    assert.Equal(nil, err)

    // Get the file size
    logFileSize := fileInfo.Size()
    var expectedSize int64

    if logFileSize == int64(178) {
        expectedSize = 178
    } else if logFileSize == int64(179) {
        expectedSize = 179
    } else {
        t.Fatal("Unexpected log size in TestLogMessage")
    }

    // Ensure the log file size is 178 or 179 bytes
    // (usually 179 but on rare occasion 178)
    assert.Equal(expectedSize, logFileSize)
}
