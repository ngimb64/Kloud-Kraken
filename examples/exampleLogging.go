package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/credentials"
	"github.com/yourusername/yourrepo/logger" // Adjust import path to your project
)

// logMessage demonstrates logging from multiple goroutines.
func logMessage(manager *logger.LoggerManager, message string) {
	manager.LogInfo(message) // Uses the LogInfo method to log messages
}

func main() {
	// Command-line flags for specifying log destinations
	logDestination := flag.String("logDestination", "local", "Log destination: local, cloudwatch, or both")
	localLogFile := flag.String("localLogFile", "app.log", "Path to the local log file")
	flag.Parse()

	// AWS config for CloudWatch logging
	awsConfig := aws.Config{
		Region:      "us-west-2", // Replace with your AWS region
		Credentials: credentials.NewStaticCredentialsProvider("AWS_ACCESS_KEY", "AWS_SECRET_KEY", ""),
	}

	// Initialize the LoggerManager based on the flags
	manager, err := logger.NewLoggerManager(*logDestination, *localLogFile, awsConfig)
	if err != nil {
		log.Fatalf("Error initializing logger manager: %v", err)
	}

	// Simulate concurrent logging from multiple goroutines
	go logMessage(manager, "Goroutine 1 logging message")
	go logMessage(manager, "Goroutine 2 logging message")
	go logMessage(manager, "Goroutine 3 logging message")

	// Allow time for goroutines to complete logging
	time.Sleep(1 * time.Second)

	fmt.Println("Logging completed!")
}
