package logging

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"go.uber.org/zap"
)

// Logger interface defines logging methods
type Logger interface {
	Info(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
}

// ZapLogger implements Logger interface using zap
type ZapLogger struct {
	logger *zap.Logger
}

// Info logs an info message to zap logger
func (z *ZapLogger) Info(msg string, fields ...zap.Field) {
	z.logger.Info(msg, fields...)
}

// Error logs an error message to zap logger
func (z *ZapLogger) Error(msg string, fields ...zap.Field) {
	z.logger.Error(msg, fields...)
}

// Warn logs a warning message to zap logger
func (z *ZapLogger) Warn(msg string, fields ...zap.Field) {
	z.logger.Warn(msg, fields...)
}


// NewZapLogger creates a new zap logger instance with file logging
func NewZapLogger(logFile string) (Logger, error) {
	// Create a production config
	cfg := zap.NewProductionConfig()

	// Override the default output paths to include the opened log file path
	cfg.OutputPaths = []string{"stdout", logFile} // Log to both stdout and file

	// Optionally, you can also set ErrorOutputPaths to control where error logs go
	cfg.ErrorOutputPaths = []string{"stderr"} // Logs errors to stderr

	// Build the logger with the config
	logger, err := cfg.Build() // No need to open the file manually
	if err != nil {
		return nil, fmt.Errorf("could not create zap logger: %w", err)
	}

	return &ZapLogger{logger: logger}, nil
}


// CloudWatchLogger implements Logger interface for CloudWatch
type CloudWatchLogger struct {
	client *cloudwatchlogs.Client
}

func (cw *CloudWatchLogger) Info(msg string, fields ...zap.Field) {
	// For demonstration, just log to the console (you can send to CloudWatch here)
	fmt.Println("CloudWatch INFO:", msg)
}

func (cw *CloudWatchLogger) Error(msg string, fields ...zap.Field) {
	// For demonstration, just log to the console (you can send to CloudWatch here)
	fmt.Println("CloudWatch ERROR:", msg)
}

func (cw *CloudWatchLogger) Warn(msg string, fields ...zap.Field) {
	// For demonstration, just log to the console (you can send to CloudWatch here)
	fmt.Println("CloudWatch WARN:", msg)
}

// NewCloudWatchLogger creates a CloudWatch logger instance
func NewCloudWatchLogger(awsConfig aws.Config) (Logger, error) {
	client := cloudwatchlogs.NewFromConfig(awsConfig)
	// Create and return CloudWatch logger (For demo, just a placeholder)
	return &CloudWatchLogger{client: client}, nil
}


// LoggerManager manages multiple loggers (local, CloudWatch)
type LoggerManager struct {
	localLogger  Logger
	cloudLogger Logger
}

// NewLoggerManager initializes local and CloudWatch loggers based on the flag
func NewLoggerManager(logDestination, localLogFile string, awsConfig aws.Config) (*LoggerManager, error) {
	var localLogger Logger
	var cloudLogger Logger
	var err error

	// Initialize local logger (file-based)
	if logDestination == "local" || logDestination == "both" {
		localLogger, err = NewZapLogger(localLogFile)
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

// LogInfo logs info message using both local and CloudWatch loggers
func (lm *LoggerManager) LogInfo(msg string, fields ...zap.Field) {
	if lm.localLogger != nil {
		lm.localLogger.Info(msg, fields...)
	}
	if lm.cloudLogger != nil {
		lm.cloudLogger.Info(msg, fields...)
	}
}

// LogError logs error message using both local and CloudWatch loggers
func (lm *LoggerManager) LogError(msg string, fields ...zap.Field) {
	if lm.localLogger != nil {
		lm.localLogger.Error(msg, fields...)
	}
	if lm.cloudLogger != nil {
		lm.cloudLogger.Error(msg, fields...)
	}
}

// LogWarn logs warning message using both local and CloudWatch loggers
func (lm *LoggerManager) LogWarn(msg string, fields ...zap.Field) {
	if lm.localLogger != nil {
		lm.localLogger.Warn(msg, fields...)
	}
	if lm.cloudLogger != nil {
		lm.cloudLogger.Warn(msg, fields...)
	}
}
