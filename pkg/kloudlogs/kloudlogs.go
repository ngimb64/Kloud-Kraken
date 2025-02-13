package kloudlogs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
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
    localLogger  Logger
    cloudLogger Logger
}

// NewLoggerManager initializes local and CloudWatch loggers based on the flag
func NewLoggerManager(logDestination, localLogFile string, awsConfig aws.Config,
                      logToMemory bool) (*LoggerManager, error) {
    var localLogger Logger
    var cloudLogger Logger
    var err error

    // Initialize local logger (file-based) with optional memory logging
    if logDestination == "local" || logDestination == "both" {
        localLogger, err = NewZapLogger(localLogFile, logToMemory)
        if err != nil {
            return nil, err
        }
    }

    // Initialize CloudWatch logger if needed
    if logDestination == "cloudwatch" || logDestination == "both" {
        cloudLogger, err = NewCloudWatchLogger(awsConfig)
        if err != nil {
            return nil, err
        }
    }

    return &LoggerManager{
        localLogger:  localLogger,
        cloudLogger: cloudLogger,
    }, nil
}

// Get the zap log and stores it in memory
func (logMan *LoggerManager) GetLog() string {
    return logMan.localLogger.GetMemoryLog()
}

// Logs info message using both local and CloudWatch loggers
func (logMan *LoggerManager) LogDebug(msg string, fields ...zap.Field) {
    if logMan.localLogger != nil {
        logMan.localLogger.Debug(msg, fields...)
    }

    if logMan.cloudLogger != nil {
        logMan.cloudLogger.Debug(msg, fields...)
    }
}

// Logs info message using both local and CloudWatch loggers
func (logMan *LoggerManager) LogInfo(msg string, fields ...zap.Field) {
    if logMan.localLogger != nil {
        logMan.localLogger.Info(msg, fields...)
    }

    if logMan.cloudLogger != nil {
        logMan.cloudLogger.Info(msg, fields...)
    }
}

// Logs warning message using both local and CloudWatch loggers
func (logMan *LoggerManager) LogWarn(msg string, fields ...zap.Field) {
    if logMan.localLogger != nil {
        logMan.localLogger.Warn(msg, fields...)
    }

    if logMan.cloudLogger != nil {
        logMan.cloudLogger.Warn(msg, fields...)
    }
}

// Logs error message using both local and CloudWatch loggers
func (logMan *LoggerManager) LogError(msg string, fields ...zap.Field) {
    if logMan.localLogger != nil {
        logMan.localLogger.Error(msg, fields...)
    }

    if logMan.cloudLogger != nil {
        logMan.cloudLogger.Error(msg, fields...)
    }
}

// Logs developer panic message using both local and CloudWatch loggers
func (logMan *LoggerManager) LogDPanic(msg string, fields ...zap.Field) {
    if logMan.localLogger != nil {
        logMan.localLogger.DPanic(msg, fields...)
    }

    if logMan.cloudLogger != nil {
        logMan.cloudLogger.DPanic(msg, fields...)
    }
}

// Logs panic message using both local and CloudWatch loggers
func (logMan *LoggerManager) LogPanic(msg string, fields ...zap.Field) {
    if logMan.localLogger != nil {
        logMan.localLogger.Panic(msg, fields...)
    }

    if logMan.cloudLogger != nil {
        logMan.cloudLogger.Panic(msg, fields...)
    }
}

// Logs fatal message using both local and CloudWatch loggers
func (logMan *LoggerManager) LogFatal(msg string, fields ...zap.Field) {
    if logMan.cloudLogger != nil {
        logMan.cloudLogger.Fatal(msg, fields...)

        // If only CloudWatch logging is active
        if logMan.localLogger == nil {
            os.Exit(1)
        }
    }

    if logMan.localLogger != nil {
        logMan.localLogger.Fatal(msg, fields...)
    }
}


// ZapLogger implements Logger interface using file
// and optional memory logging
type ZapLogger struct {
    logger       *zap.Logger
    memoryBuffer *bytes.Buffer
}

// NewZapLogger creates a new zap logger instance with either file logging or memory logging
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

// Get the zap log and stores it in memory
func (zapLog *ZapLogger) GetMemoryLog() string {
    if zapLog.memoryBuffer != nil {
        return zapLog.memoryBuffer.String()
    }
    return ""
}

// Logs an debug message to zap logger
func (zapLog *ZapLogger) Debug(msg string, fields ...zap.Field) {
    zapLog.logger.Debug(msg, fields...)
}

// Logs an info message to zap logger
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
    client *cloudwatchlogs.Client
}

// NewCloudWatchLogger creates a CloudWatch logger instance
func NewCloudWatchLogger(awsConfig aws.Config) (Logger, error) {
    client := cloudwatchlogs.NewFromConfig(awsConfig)
    // Create and return CloudWatch logger
    return &CloudWatchLogger{client: client}, nil
}

// Get the zap log and stores it in memory
func (cwLogger *CloudWatchLogger) GetMemoryLog() string {
    return ""
}

func (cwLogger *CloudWatchLogger) Debug(msg string, fields ...zap.Field) {
    // TODO:  implement CloudWatch code
    fmt.Println("CloudWatch DEBUG:", msg)
}

func (cwLogger *CloudWatchLogger) Info(msg string, fields ...zap.Field) {
    // TODO:  implement CloudWatch code
    fmt.Println("CloudWatch INFO:", msg)
}

func (cwLogger *CloudWatchLogger) Warn(msg string, fields ...zap.Field) {
    // TODO:  implement CloudWatch code
    fmt.Println("CloudWatch WARN:", msg)
}

func (cwLogger *CloudWatchLogger) Error(msg string, fields ...zap.Field) {
    // TODO:  implement CloudWatch code
    fmt.Println("CloudWatch ERROR:", msg)
}

func (cwLogger *CloudWatchLogger) DPanic(msg string, fields ...zap.Field) {
    // TODO:  implement CloudWatch code
    fmt.Println("CloudWatch ERROR:", msg)
}

func (cwLogger *CloudWatchLogger) Panic(msg string, fields ...zap.Field) {
    // TODO:  implement CloudWatch code
    fmt.Println("CloudWatch ERROR:", msg)
}

func (cwLogger *CloudWatchLogger) Fatal(msg string, fields ...zap.Field) {
    // TODO:  implement CloudWatch code
    fmt.Println("CloudWatch ERROR:", msg)
}


func LogToMap(jsonStr string) (map[string]interface{}, error) {
    var logMap map[string]interface{}

    // Store the json string data as key-values in log map
    err := json.Unmarshal([]byte(jsonStr), &logMap)
    if err != nil {
        return nil, fmt.Errorf("failed to unmarshal JSON log: %w", err)
    }

    return logMap, nil
}


func LogMessage(manager *LoggerManager, level string, message string, args ...any) {
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
