package kloudlogs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	cwl "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwlTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger interface defines logging methods
type Logger interface {
    GetMemoryLog () string
    Debug(msg string, field ...zap.Field)
    Info(msg string, fields ...zap.Field)
    Warn(msg string, fields ...zap.Field)
    Error(msg string, fields ...zap.Field)
    DPanic(msg string, fields ...zap.Field)
    Panic(msg string, fields ...zap.Field)
    Fatal(msg string, fields ...zap.Field)
}

// LoggerManager manages multiple loggers (local, CloudWatch)
type LoggerManager struct {
    LocalLogger Logger
    CloudLogger Logger
}

// NewLoggerManager initializes local and CloudWatch loggers based on the flag.
//
// @Parameters
// - logDestination:  Where the logs will be stored (local, cloudwatch, both)
// - localLogFile:  Path where the logs will be stored locally on file
// - awsConfig:  The initialized AWS configuration instance
// - group:  The CloudWatch logging group
// - logToMemory:  Boolean toggler whether to log to memory or not
//
// @Returns
// - The initialzed logging manager
// - Error if it occurs, otherwise nil on success
//
func NewLoggerManager(logDestination, localLogFile string, awsConfig aws.Config,
                      group string, logToMemory bool) (*LoggerManager, error) {
    var localLogger Logger
    var cloudLogger Logger
    var err error

    // Initialize file-based local logger with optional memory logging
    if logDestination == "local" || logDestination == "both" {
        localLogger, err = NewZapLogger(localLogFile, logToMemory)
        if err != nil {
            return nil, err
        }
    }

    // Initialize CloudWatch logger if needed
    if logDestination == "cloudwatch" || logDestination == "both" {
        cloudLogger, err = NewCloudWatchLogger(awsConfig, group)
        if err != nil {
            return nil, err
        }
    }

    return &LoggerManager{
        LocalLogger:  localLogger,
        CloudLogger: cloudLogger,
    }, nil
}

// Gets the log from the logging instance and
// returns it be stored in memory variable.
//
// @Returns
// - The string JSON log from the zap logging instance
//
func (logMan *LoggerManager) GetLog() string {
    return logMan.LocalLogger.GetMemoryLog()
}

// Parses the variable length args  based on data type into different lists.
//
// @Parameters
// - manager:  The logger manager for zap and CloudWatch instances
// - level:  The level of logging
// - message:  The message to be logged, supports printf format with below args
// - args:  Variadic length list of args with zap.Fields and regular data types
//          supporting printf format
//
func (manager *LoggerManager) LogMessage(level string, message string, args ...any) {
    argList := []any{}
    zapFields := []zap.Field {}
    formattedMessage := ""

    // Iterate through passed in arg list
    for _, arg := range args {
        // Case logic based on arg data type
        switch argType := arg.(type) {
        // If the arg type is a zap field, add it to the zap field list
        case zap.Field:
            zapFields = append(zapFields, argType)
        // For other arg types, add it to the arg list
        default:
            argList = append(argList, argType)
        }
    }

    // If there are any non-zap args to format into the message
    if len(argList) > 0 {
        formattedMessage = fmt.Sprintf(message, argList)
    } else {
        formattedMessage = message
    }

    // Log based on the level (info, error, warn) and include the fields
    switch level {
    case "debug":
        manager.LogDebug(formattedMessage, zapFields...)
    case "info":
        manager.LogInfo(formattedMessage, zapFields...)
    case "warn":
        manager.LogWarn(formattedMessage, zapFields...)
    case "error":
        manager.LogError(formattedMessage, zapFields...)
    case "dpanic":
        manager.LogDPanic(formattedMessage, zapFields...)
    case "panic":
        manager.LogPanic(formattedMessage, zapFields...)
    case "fatal":
        manager.LogFatal(formattedMessage, zapFields...)
    default:
        log.Fatalf("[*] Error: Unknown logging level specified %v", level)
    }
}

// Logs info message using both local and CloudWatch loggers
func (logMan *LoggerManager) LogDebug(msg string, fields ...zap.Field) {
    if logMan.LocalLogger != nil {
        logMan.LocalLogger.Debug(msg, fields...)
    }

    if logMan.CloudLogger != nil {
        logMan.CloudLogger.Debug(msg, fields...)
    }
}

// Logs info message using both local and CloudWatch loggers
func (logMan *LoggerManager) LogInfo(msg string, fields ...zap.Field) {
    if logMan.LocalLogger != nil {
        logMan.LocalLogger.Info(msg, fields...)
    }

    if logMan.CloudLogger != nil {
        logMan.CloudLogger.Info(msg, fields...)
    }
}

// Logs warning message using both local and CloudWatch loggers
func (logMan *LoggerManager) LogWarn(msg string, fields ...zap.Field) {
    if logMan.LocalLogger != nil {
        logMan.LocalLogger.Warn(msg, fields...)
    }

    if logMan.CloudLogger != nil {
        logMan.CloudLogger.Warn(msg, fields...)
    }
}

// Logs error message using both local and CloudWatch loggers
func (logMan *LoggerManager) LogError(msg string, fields ...zap.Field) {
    if logMan.LocalLogger != nil {
        logMan.LocalLogger.Error(msg, fields...)
    }

    if logMan.CloudLogger != nil {
        logMan.CloudLogger.Error(msg, fields...)
    }
}

// Logs developer panic message using both local and CloudWatch loggers
func (logMan *LoggerManager) LogDPanic(msg string, fields ...zap.Field) {
    if logMan.LocalLogger != nil {
        logMan.LocalLogger.DPanic(msg, fields...)
    }

    if logMan.CloudLogger != nil {
        logMan.CloudLogger.DPanic(msg, fields...)
    }
}

// Logs panic message using both local and CloudWatch loggers
func (logMan *LoggerManager) LogPanic(msg string, fields ...zap.Field) {
    if logMan.LocalLogger != nil {
        logMan.LocalLogger.Panic(msg, fields...)
    }

    if logMan.CloudLogger != nil {
        logMan.CloudLogger.Panic(msg, fields...)
    }
}

// Logs fatal message using both local and CloudWatch loggers
func (logMan *LoggerManager) LogFatal(msg string, fields ...zap.Field) {
    if logMan.CloudLogger != nil {
        logMan.CloudLogger.Fatal(msg, fields...)

        // If only CloudWatch logging is active
        if logMan.LocalLogger == nil {
            os.Exit(1)
        }
    }

    if logMan.LocalLogger != nil {
        logMan.LocalLogger.Fatal(msg, fields...)
    }
}


// ZapLogger implements Logger interface using file
// and optional memory logging
type ZapLogger struct {
    logger       *zap.Logger
    memoryBuffer *bytes.Buffer
}

// NewZapLogger creates a zap logger instance with either file or memory logging.
//
// @Parameters
// - logFile:  The path for the output log file
// - logToMemory:  Boolean toggle to specify whether to log to memory or not
//
// @Returns
// - Initialzed zap logging instance
// - Error if it occurs, otherwise nil on success
//
func NewZapLogger(logFile string, logToMemory bool) (Logger, error) {
    var logger *zap.Logger
    var err error

    // If logging to memory
    if logToMemory {
        // Create a buffer to capture logs in memory
        memoryBuffer := new(bytes.Buffer)

        // Use zapcore directly for logging to memory
        core := zapcore.NewCore(
            zapcore.NewJSONEncoder(zap.NewProductionConfig().EncoderConfig),
            zapcore.AddSync(memoryBuffer),
            zap.InfoLevel,
        )

        // Create the logger with the custom core
        logger := zap.New(core)

        // Return the logger along with the memory buffer
        return &ZapLogger{
            logger:       logger,
            memoryBuffer: memoryBuffer,
        }, nil
    // Othwise logging to file
    } else {
        // If logging to file
        cfg := zap.NewProductionConfig()
        cfg.OutputPaths = []string{"stdout", logFile}
        cfg.ErrorOutputPaths = []string{"stderr", logFile}

        // Build the file-based logger
        logger, err = cfg.Build()
        if err != nil {
            return nil, fmt.Errorf("could not create file logger: %w", err)
        }

        return &ZapLogger{
            logger:       logger,
            memoryBuffer: nil,
        }, nil
    }
}

// Gets the zap log from the zap logging instance and
// returns it be stored in memory variable.
//
// @Returns
// - The string JSON log from the zap logging instance
//
func (zapLog *ZapLogger) GetMemoryLog() string {
    if zapLog.memoryBuffer != nil {
        return zapLog.memoryBuffer.String()
    }
    return ""
}

// Logs a debug message to zap logger
func (zapLog *ZapLogger) Debug(msg string, fields ...zap.Field) {
    zapLog.logger.Debug(msg, fields...)
}

// Logs a info message to zap logger
func (zapLog *ZapLogger) Info(msg string, fields ...zap.Field) {
    zapLog.logger.Info(msg, fields...)
}

// Logs a warning message to zap logger
func (zapLog *ZapLogger) Warn(msg string, fields ...zap.Field) {
    zapLog.logger.Warn(msg, fields...)
}

// Logs a error message to zap logger
func (zapLog *ZapLogger) Error(msg string, fields ...zap.Field) {
    zapLog.logger.Error(msg, fields...)
}

// Logs a developer panic message to zap logger
func (zapLog *ZapLogger) DPanic(msg string, fields ...zap.Field) {
    zapLog.logger.DPanic(msg, fields...)
}

// Logs a panic message to zap logger
func (zapLog *ZapLogger) Panic(msg string, fields ...zap.Field) {
    zapLog.logger.Panic(msg, fields...)
}

// Logs a fatal message to zap logger
func (zapLog *ZapLogger) Fatal(msg string, fields ...zap.Field) {
    zapLog.logger.Fatal(msg, fields...)
}


// CloudWatchLogger implements Logger interface for CloudWatch
type CloudWatchLogger struct {
    client       *cwl.Client
    cwMutex      sync.Mutex
    logGroup     string
    logStream    string
    nextSequence *string
}

// Creates and returns CloudWatch logger instance.
//
// @Parameters
// - awsConfig:  The AWS configuration config struct
// - group:  The CloudWatch logging group
// - stream:  The CloudWatch logging stream
//
// @Returns
// - The initializes CloudWatch logger config instance
// - Error if it occurs, otherwise nil on success
//
func NewCloudWatchLogger(awsConfig aws.Config, group string) (
                         Logger, error) {
    var stream string
    // Establish CloudWatch client and set to run in background
    client := cwl.NewFromConfig(awsConfig)
    ctx := context.Background()

    // Set up client to the EC2 instance metadata service
    metaDataService := imds.NewFromConfig(awsConfig)
    // Get the EC2 insance id based on the AWS config
    metaData, err := metaDataService.GetMetadata(ctx, &imds.GetMetadataInput{Path: "instance-id"})
    if err != nil {
        // Fallback to hostname if failed to retrieve instance id
        stream, err = os.Hostname()
        if err != nil {
            return nil, fmt.Errorf("cannot determine host identity: %w", err)
        }
    } else {
        // Get the EC2 instance id from metadata output
        streamData, err := io.ReadAll(metaData.Content)
        if err != nil {
            return nil, fmt.Errorf("getting instance ID:  %w", err)
        }

        stream = string(streamData)
    }

    // Create the CloudWatch log group
    _, err = client.CreateLogGroup(ctx, &cwl.CreateLogGroupInput{
        LogGroupName: aws.String(group),
    })
    if err != nil {
        var ae *cwlTypes.ResourceAlreadyExistsException

        // If the error is not having to do with group already existing
        if !errors.As(err, &ae) {
            return nil, fmt.Errorf("CreateLogGroup: %w", err)
        }
    }

    // Create the CloudWatch log stream
    _, err = client.CreateLogStream(ctx, &cwl.CreateLogStreamInput{
        LogGroupName:  aws.String(group),
        LogStreamName: aws.String(stream),
    })
    if err != nil {
        return nil, fmt.Errorf("CreateLogStream: %w", err)
    }

    // Describe to grab initial token (nil if fresh)
    res, err := client.DescribeLogStreams(ctx, &cwl.DescribeLogStreamsInput{
        LogGroupName:        aws.String(group),
        LogStreamNamePrefix: aws.String(stream),
    })
    if err != nil {
        return nil, fmt.Errorf("calling DescribeLogStreams: %w", err)
    }

    var token *string
    // If there are log streams retrieved
    if len(res.LogStreams) > 0 {
        // Set the upload sequence token
        token = res.LogStreams[0].UploadSequenceToken
    }

    return &CloudWatchLogger{
        client:       client,
        logGroup:     group,
        logStream:    stream,
        nextSequence: token,
    }, nil
}

// Method that packages message & fields, sends to CW, and updates token.
//
// @Parameters
// - level:  The level that the log event will be set to
// - msg:  The message of log event
// - fields:  Any additional zap field to be added to log entry
//
func (cloudWatchLog *CloudWatchLogger) log(level string, msg string, fields ...zap.Field) {
    // Build log entry
    entry := map[string]any{
        "timestamp": time.Now().UTC().Format(time.RFC3339Nano),
        "level":     level,
        "message":   msg,
    }

    // Iterate through the the slice of fields
    for _, field := range fields {
        // Add fields in log entry map
        entry[field.Key] = field.Interface
    }

    // Format the data into JSON for transporting to CloudWatch
    payload, err := json.Marshal(entry)
    if err != nil {
        log.Fatalf("marshal log entry: %v\n", err)
    }

    // Set up input log event message
    event := cwlTypes.InputLogEvent{
        Message:   aws.String(string(payload)),
        Timestamp: aws.Int64(time.Now().UnixNano() / int64(time.Millisecond)),
    }

    // Set mutex for logging operation
    cloudWatchLog.cwMutex.Lock()
    defer cloudWatchLog.cwMutex.Unlock()

    inputEvent := &cwl.PutLogEventsInput{
        LogGroupName:  aws.String(cloudWatchLog.logGroup),
        LogStreamName: aws.String(cloudWatchLog.logStream),
        LogEvents:     []cwlTypes.InputLogEvent{event},
        SequenceToken: cloudWatchLog.nextSequence,
    }

    // Upload log entry via the log stream
    resp, err := cloudWatchLog.client.PutLogEvents(context.Background(), inputEvent)
    if err != nil {
        log.Fatalf("PutLogEvents: %v\n", err)
    }

    // Set the next sequence token fron the response
    cloudWatchLog.nextSequence = resp.NextSequenceToken
}

// Current dummy handler to follow interface contract (zap only)
func (cloudWatchLog *CloudWatchLogger) GetMemoryLog() string {
    return ""
}

// Logs a debug message to CloudWatch
func (cloudWatchLog *CloudWatchLogger) Debug(msg string, fields ...zap.Field) {
    cloudWatchLog.log("DEBUG", msg, fields...)
}

// Logs a info message to CloudWatch
func (cloudWatchLog *CloudWatchLogger) Info(msg string, fields ...zap.Field) {
    cloudWatchLog.log("INFO", msg, fields...)
}

// Logs a warn message to CloudWatch
func (cloudWatchLog *CloudWatchLogger) Warn(msg string, fields ...zap.Field) {
    cloudWatchLog.log("WARN", msg, fields...)
}

// Logs a error message to CloudWatch
func (cloudWatchLog *CloudWatchLogger) Error(msg string, fields ...zap.Field) {
    cloudWatchLog.log("ERROR", msg, fields...)
}

// Logs a developer panic message to CloudWatch
func (cloudWatchLog *CloudWatchLogger) DPanic(msg string, fields ...zap.Field) {
    cloudWatchLog.log("DPANIC", msg, fields...)
}

// Logs a panic message to CloudWatch
func (cloudWatchLog *CloudWatchLogger) Panic(msg string, fields ...zap.Field) {
    cloudWatchLog.log("PANIC", msg, fields...)
}

// Logs a fatal message to CloudWatch
func (cloudWatchLog *CloudWatchLogger) Fatal(msg string, fields ...zap.Field) {
    cloudWatchLog.log("FATAL", msg, fields...)
}


// Takes the passed in JSON formatted string and maps into a map via unmarshal.
//
// @Parameters
// - jsonStr:  The JSON string to unmarshal into map
//
// @Returns
// - The map with unmarshaled JSON data
// - Error if it occurs, otherwise nil on success
//
func LogToMap(jsonStr string) (map[string]any, error) {
    var logMap map[string]any

    // Store the json string data as key-values in log map
    err := json.Unmarshal([]byte(jsonStr), &logMap)
    if err != nil {
        return nil, fmt.Errorf("failed to unmarshal JSON log: %w", err)
    }

    return logMap, nil
}
