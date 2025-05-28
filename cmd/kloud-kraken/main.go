package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/internal/validate"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"github.com/ngimb64/Kloud-Kraken/pkg/display"
	"github.com/ngimb64/Kloud-Kraken/pkg/kloudlogs"
	"github.com/ngimb64/Kloud-Kraken/pkg/netio"
	"github.com/ngimb64/Kloud-Kraken/pkg/tlsutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/wordlist"
	"go.uber.org/zap"
)

// Package level variables
var CurrentConnections atomic.Int32	   // Tracks current active connections
var ReceivedDir = "/tmp/received"      // Path where cracked hashes & client logs are stored
var TlsMan = new(tlsutils.TlsManager)  // Struct for managing TLS certs, keys, etc.


// Select next available file for transfer, if there are no more available send the end transfer
// message to client. Format the transfer reply with the file name and size, get the IP address
// of the current connection and read the port from the socket to format the dialer for the new
// connection for file transfer. Finally pass the connection with other args into TransferFile().
//
// @Parameters
// - connection:  Network socket connection for handling messaging
// - buffer:  The buffer storing network messaging
// - waitGroup:  Used to synchronize the Goroutines running
// - appConfig:  The configuration struct with loaded yaml program data
// - logMan:  The kloudlogs logger manager for local logging
//
func handleTransfer(connection net.Conn, buffer []byte, waitGroup *sync.WaitGroup,
                    appConfig *conf.AppConfig, logMan *kloudlogs.LoggerManager) {
    // Select the next avaible file in the load dir from YAML data
    filePath, fileSize, err := disk.SelectFile(appConfig.LocalConfig.LoadDir,
                                               appConfig.ClientConfig.MaxFileSizeInt64)
    if err != nil {
        logMan.LogMessage("error", "Error selecting the next available file to transfer:  %v", err)
        return
    }

    // If there are no more files available to be transfered
    if filePath == "" {
        // Send the end transfer message then exit function
        _, err = netio.WriteHandler(connection, globals.END_TRANSFER_MARKER,
                                    len(globals.END_TRANSFER_MARKER))
        if err != nil {
            logMan.LogMessage("error", "Error sending the end transfer message:  %v", err)
        }

        return
    }

    // Format transfer reply to inform client of selected file name and size
    sendLength, err := netio.FormatTransferReply(filePath, fileSize, &buffer,
                                                 globals.START_TRANSFER_PREFIX)
    if err != nil {
        logMan.LogMessage("error", "Error formatting transfer reply:  %v", err)
        return
    }

    // Send the transfer reply with file name and size
    _, err = netio.WriteHandler(connection, buffer, sendLength)
    if err != nil {
        logMan.LogMessage("error", "Error sending the transfer reply:  %v", err)
        return
    }

    // Get the IP address from the ip:port host address
    ipAddr, _, err := netio.GetIpPort(connection)
    if err != nil {
        logMan.LogMessage("error", "Error occcurred spliting host address to get IP/port:  %v", err)
        return
    }

    var port uint16
    // Receive bytes of port of client port to connect to for file transfer
    err = binary.Read(connection, binary.LittleEndian, &port)
    if err != nil {
        logMan.LogMessage("error", "Error receiving client listener port:  %v", err)
        return
    }

    // Format remote address with IP and port
    remoteAddr := ipAddr + ":" + strconv.Itoa(int(port))

    // Make a connection to the remote brain server
    transferConn, err := tls.Dial("tcp", remoteAddr,
                                  tlsutils.NewClientTLSConfig(TlsMan.CaCertPool, ipAddr))
    if err != nil {
        logMan.LogMessage("fatal", "Error connecting to remote client for transfer:  %v", err)
        return
    }

    logMan.LogMessage("info", "Connected remote client at %s on port %d", ipAddr, port)
    // Increment waitgroup counter
    waitGroup.Add(1)

    go func() {
        // Decrement waitgroup counter and close transfer connection on local exit
        defer waitGroup.Done()
        defer transferConn.Close()

        // Transfer the file to client
        err = netio.TransferFile(transferConn, filePath, fileSize)
        if err != nil {
            logMan.LogMessage("error", "Error occured transfering file to client:  %v", err)
        }
    } ()
}


// Upload the hash and ruleset files (if optional ruleset applied). Goes into continual loop
// where data is read from the message sockets connection-buffer, checks for a processing complete
// message which signals exiting the loop, finally after the loop received cracked hash and log file.
//
// @Parameters
// - connection:  Network socket connection for handling messaging
// - waitGroup:  Used to synchronize the Goroutines running
// - appConfig:  The configuration struct with loaded yaml program data
// - logMan:  The kloudlogs logger manager for local logging
//
func handleConnection(connection net.Conn, waitGroup *sync.WaitGroup,
                      appConfig *conf.AppConfig, logMan *kloudlogs.LoggerManager) {
    // Close connection and decrement waitGroup counter on local exit
    defer connection.Close()
    defer waitGroup.Done()

    // Set buffer to receive client PEM certificate
    buffer := make([]byte, 2 * globals.KB)

    // Receive the client PEM certificate bytes
    bytesRead, err := netio.ReadHandler(connection, &buffer)
    if err != nil {
        logMan.LogMessage("error", "Error reading client PEM cert:  %v", err)
        return
    }

    // Add the read client PEM cert to the cert pool
    TlsMan.AddCACert(buffer[:bytesRead])

    // Reset buffer to messaging size
    buffer = make([]byte, globals.MESSAGE_BUFFER_SIZE)

    // Upload the hash file to connection client
    err = netio.UploadFile(connection, buffer, appConfig.LocalConfig.HashFilePath,
                           globals.HASHES_TRANSFER_PREFIX)
    if err != nil {
        logMan.LogMessage("error", "Error sending the hash file to client:  %v", err)
        return
    }

    // If a ruleset path was specified
    if appConfig.LocalConfig.RulesetPath != "" {
        // Upload the ruleset file to connection client
        err = netio.UploadFile(connection, buffer, appConfig.LocalConfig.RulesetPath,
                               globals.RULESET_TRANSFER_PREFIX)
        if err != nil {
            logMan.LogMessage("error", "Error sending the ruleset to server:  %v", err)
            return
        }
    }

    for {
        // Read data from connected client
        bytesRead, err := netio.ReadHandler(connection, &buffer)
        if err != nil {
            logMan.LogMessage("error", "Error reading data from socket:  %v", err)
            return
        }

        // Save read content into isolated buffer
        readBuffer := buffer[:bytesRead]

        // If the read data contains the processing complete message
        if bytes.Contains(readBuffer, globals.PROCESSING_COMPLETE) {
            break
        }

        // If the read data contains transfer request message
        if bytes.Contains(readBuffer, globals.TRANSFER_REQUEST_MARKER) {
            // Call method to handle file transfer based
            handleTransfer(connection, buffer, waitGroup, appConfig, logMan)
        }
    }

    // Receive cracked user hash file from client
    _, err = netio.ReceiveFile(connection, buffer, ReceivedDir,
                               globals.LOOT_TRANSFER_PREFIX)
    if err != nil {
        logMan.LogMessage("error", "Error receiving cracked user hashes:  %v", err)
        return
    }

    // Receive log file from client
    _, err = netio.ReceiveFile(connection, buffer, ReceivedDir,
                               globals.LOG_TRANSFER_PREFIX)
    if err != nil {
        logMan.LogMessage("error", "Error receiving log file:  %v", err)
        return
    }

    // Decrement the active connection count
    CurrentConnections.Add(-1)

    logMan.LogMessage("info", "Connection processing handled",
                      zap.Int32("remaining connections", CurrentConnections.Load()))
}


// Set up listener and enter loop where the amount of active connections is checked
// until the specified number of instances is equal to the active connections the
// listener will wait until a connection is accepted. Increment the active connections
// counter and waitgroup, and pass the connection with other args into handler goroutine.
//
// @Parameters
// - appConfig:  The configuration struct with loaded yaml program data
// - logMan:  The kloudlogs logger manager for local logging
//
func startServer(appConfig *conf.AppConfig, logMan *kloudlogs.LoggerManager) {
    // Establish wait group for Goroutine synchronization
    var waitGroup sync.WaitGroup

    // Set up context handler for TLS listener
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    // Set up the TLS listener to accept incoming connections
    tlsListener, err := tlsutils.SetupTlsListenerHandler(TlsMan.TlsCertificate, TlsMan.CaCertPool, ctx,
                                                         "", appConfig.LocalConfig.ListenerPort, nil)
    if err != nil {
        logMan.LogMessage("fatal", "Error setting up TLS listener:  %v", err)
    }

    // Close the TLS listener on local exit
    defer tlsListener.Close()

    logMan.LogMessage("info", "Server started, waiting for connections ..")

    for {
        // If current number of connection is greater than or equal to number of instances
        if CurrentConnections.Load() >= int32(appConfig.LocalConfig.NumberInstances) {
            logMan.LogMessage("info", "All remote clients are connected")
            break
        }

        // Wait for an incoming connection
        connection, err := tlsListener.Accept()
        if err != nil {
            logMan.LogMessage("error", "Error accepting client connection:  %v", err)
            return
        }

        // Increment the active connection count
        CurrentConnections.Add(1)

        logMan.LogMessage("info", "Connection accepted to remote client",
                          zap.Int32("active connections", CurrentConnections.Load()))

        // Increment wait group and handle connection in separate Goroutine
        waitGroup.Add(1)
        go handleConnection(connection, &waitGroup, appConfig, logMan)
    }

    // Wait for all active Goroutines to finish before shutting down the server
    waitGroup.Wait()

    logMan.LogMessage("info", "All connections handled .. server shutting down")
}


// TODO:  document when finished
//
func ec2UserDataGen(appConf *conf.AppConfig, accessKey string, secretKey string,
                    keyName string, ipAddrs []string, ssmParam string) (
                    string, error) {
    var hasRuleset bool
    // Convert the slice of IP addresses to CSV string
    ipAddrsCsv, err := data.SliceToCsv(ipAddrs)
    if err != nil {
        return "", err
    }

    // If a ruleset path was specified
    if appConf.LocalConfig.RulesetPath != "" {
        hasRuleset = true
    } else {
        hasRuleset = false
    }

    data := fmt.Sprintf(`#!/bin/bash
export AWS_ACCESS_KEY_ID=%s
export AWS_SECRET_ACCESS_KEY=%s

apt update && apt upgrade -y && apt install -y hashcat

CWD=$(pwd)
aws s3 cp s3://%s/%s $CWD/client --region %s --no-progress
chmod +x $CWD/client
$CWD/client -applyOptimization=%t \
            -awsAccessKey=%s \
            -awsRegion=%s \
            -awsSecretKey=%s \
            -certSsmParam=%s \
            -charSet1=%s \
            -charSet2=%s \
            -charSet3=%s \
            -charSet4=%s \
            -crackingMode=%s \
            -hashMask=%s \
            -hashType=%s \
            -hasRuleset=%t \
            -ipAddrs=%s \
            -isTesting=%t \
            -logMode=%s \
            -logPath=%s \
            -maxFileSizeInt64=%d \
            -maxTransfers=%d \
            -port=%d \
            -workload=%s
`, accessKey, secretKey, appConf.LocalConfig.BucketName, keyName, appConf.ClientConfig.Region, true,
   accessKey, appConf.ClientConfig.Region, secretKey, ssmParam, appConf.ClientConfig.CharSet1,
   appConf.ClientConfig.CharSet2, appConf.ClientConfig.CharSet3, appConf.ClientConfig.CharSet4,
   appConf.ClientConfig.CrackingMode, appConf.ClientConfig.HashMask, appConf.ClientConfig.HashType,
   hasRuleset, ipAddrsCsv, false, appConf.ClientConfig.LogMode, appConf.ClientConfig.LogPath,
   appConf.ClientConfig.MaxFileSizeInt64, appConf.ClientConfig.MaxTransfers,
   appConf.LocalConfig.ListenerPort, appConf.ClientConfig.Workload)

    return data, nil
}


// Create the required dirs for program operation.
//
func makeServerDirs() {
    // Set the program directories
    programDirs := []string{ReceivedDir}
    // Create needed directories
    disk.MakeDirs(programDirs)
}


// Parses command line args (path to yaml config file), if args not present
// or invalid then proceeds to user input until valid yaml file is specified.
//
// @Returns
// - Pointer to AppConfig struct populated from yaml data
//
func parseArgs() *conf.AppConfig {
    var configFilePath string

    // If the config file path was not passed in
    if len(os.Args) < 2 {
        // Prompt the user until proper path is passed in
        validate.ValidateConfigPath(&configFilePath)
    // If the config file path arg was passed in
    } else {
        // Set the provided arg as the config file path
        configFilePath = os.Args[1]

        // Check to see if the input path exists and is a file or dir
        exists, isDir, hasData, err := disk.PathExists(configFilePath)
        if err != nil {
            log.Fatal("Error checking config file path existence: ", err)
        }

        // If the path does not exist OR is a dir OR does not have data OR is not YAML file
        if !exists || isDir || !hasData || !strings.HasSuffix(configFilePath, ".yml") {
            fmt.Println("Provided YAML config file path invalid: ", configFilePath)
            // Sleep for a few seconds and clear screen
            display.ClearScreen(3)
            // Prompt the user until proper path is passed in
            validate.ValidateConfigPath(&configFilePath)
        }
    }

    // Load the configuration from the YAML file
    return conf.LoadConfig(configFilePath)
}


// Parse command line args, make needed directories, merge wordlists and remove remaining
// empty dirs. Set up AWS access config with key and secret, set up logging manager
// instance, set up EC2 code passing command line args via user data, and start server.
//
func main() {
    // Handle selecting the YAML file if no arg provided
    // and load YAML data into struct configuration class
    appConfig := parseArgs()
    // Make the server directories
    makeServerDirs()
    // Merge the wordlists in the load dir based on max file size
    err := wordlist.MergeWordlistDir(appConfig.LocalConfig.LoadDir,
                                     appConfig.LocalConfig.MaxMergingSizeInt64,
                                     appConfig.ClientConfig.MaxFileSizeInt64,
                                     appConfig.LocalConfig.MaxSizeRange,
                                     int64(1 * globals.GB))
    if err != nil {
        log.Fatalf("Error merging wordlists:  %v", err)
    }

    // Delete any leftover folders in load dir
    err = wordlist.RemoveMergeSubdirs(appConfig.LocalConfig.LoadDir)
    if err != nil {
        log.Fatalf("Error deleting load dir subdirs:  %v", err)
    }

    var awsConfig aws.Config
    var logMan *kloudlogs.LoggerManager

    // If the program is being run in full mode (not testing)
    if !appConfig.LocalConfig.LocalTesting {
        // Query IP lookup APIs for public IP addresses
        publicIps, err := tlsutils.GetPublicIps()
        if err != nil {
            log.Fatalf("Error getting public IP addresses:  %v", err)
        }

        // Generate the servers TLS PEM certificate and key and save in TLS manager
        TlsMan.CertPemBlock,
        TlsMan.KeyPemBlock, err = tlsutils.PemCertAndKeyGenHandler("Kloud Kraken", false, publicIps...)
        if err != nil {
            log.Fatalf("Error creating TLS PEM certificate and key:  %v", err)
        }

        // Set up the AWS credentials based on local chain or environment variables
        awsConfig, accessKey, secretKey, err := awsutils.AwsConfigSetup(appConfig.LocalConfig.Region,
                                                                        1*time.Minute)
        if err != nil {
            log.Fatalf("Error initializing AWS config:  %v", err)
        }

        // Establish client to SSM
        ssmMan := awsutils.NewSsmManager(awsConfig)
        // Push the servers certificate PEM into SSM parameter store
        param, err := ssmMan.PutSsmParameter("/kloud-kraken/tls/cert", string(TlsMan.CertPemBlock),
                                             1*time.Minute)
        if err != nil {
            log.Fatalf("Error putting TLS PEM certificate in SSM Parameter Store:  %v", err)
        }

        // Establish client to S3
        s3Man := awsutils.NewS3Manager(awsConfig)
        // Check to see if S3 bucket exists
        exists, err := s3Man.BucketExists(appConfig.LocalConfig.BucketName, 1*time.Minute)
        if err != nil {
            log.Fatalf("Error checking S3 bucket existence:  %v", err)
        }

        // If S3 bucket does not exist create one
        if !exists {
            err = s3Man.CreateBucket(appConfig.LocalConfig.BucketName, 1*time.Minute)
            if err != nil {
                log.Fatalf("Error creating S3 bucket:  %v", err)
            }
        }

        // Read the client binary into memory
        binData, err := os.ReadFile("./client")
        if err != nil {
            log.Fatalf("Error reading client binary from disk:  %v", err)
        }

        // Upload the client binary to S3 Bucket
        keyName, err := s3Man.PutS3Object(appConfig.LocalConfig.BucketName, "client",
                                          binData, 1*time.Minute)
        if err != nil {
            log.Fatalf("Error putting object in S3 bucket:  %v", err)
        }

        // Generate user data script to set up client program in EC2
        userData, err := ec2UserDataGen(appConfig, accessKey, secretKey,
                                        keyName, publicIps, param)
        if err != nil {
            log.Fatalf("Error generating user data for EC2:  %v", err)
        }

        // Setup EC2 creation instance with populated args
        ec2Man := awsutils.NewEc2Manager("ami-0eb94e3d16a6eea5f", awsConfig,
                                         appConfig.LocalConfig.NumberInstances,
                                         appConfig.LocalConfig.InstanceType,
                                         "Kloud-Kraken", []byte(userData))
        // Create number of EC2 instances based on passed in data
        err = ec2Man.CreateEc2Instances(20*time.Minute)
        if err != nil {
            log.Fatalf("Error creating EC2 instances:  %v", err)
        }

        defer func() {
            // Terminate the EC2 instances when processing is complete
            termOutput, err := ec2Man.TerminateEc2Instances(time.Minute*10)
            if err != nil {
                log.Fatalf("Error terminating EC2 instances:  %v", err)
            }

            // Iterate through list of terminated instance ids
            for _, instance := range termOutput.TerminatingInstances {
                logMan.LogMessage("Instance state for %s: %s → %s\n",
                                  aws.ToString(instance.InstanceId),
                                  instance.PreviousState.Name,
                                  instance.CurrentState.Name)
            }
        } ()

    // If the program is being run in testing mode
    } else {
        // Generate the servers TLS PEM certificate & key and save in TLS manager
        TlsMan.CertPemBlock,
        TlsMan.KeyPemBlock, err = tlsutils.PemCertAndKeyGenHandler("Kloud Kraken", true)
        if err != nil {
            log.Fatalf("Error creating TLS PEM certificate and key:  %v", err)
        }

        log.Println("Testing mode:  PEM cert & key generated, transfer cert to client before execution")
    }

    // Generate a TLS x509 certificate and cert pool
    TlsMan.TlsCertificate, TlsMan.CaCertPool, err = tlsutils.CertGenAndPool(TlsMan.CertPemBlock,
                                                                            TlsMan.KeyPemBlock,
                                                                            TlsMan.CaCertPemBlocks)
    if err != nil {
        log.Fatalf("Error generating TLS certificate:  %v", err)
    }

    // Initialize the LoggerManager based on the flags
    logMan, err = kloudlogs.NewLoggerManager("local", appConfig.LocalConfig.LogPath, awsConfig,
                                             "Kloud-Kraken", false)
    if err != nil {
        log.Fatalf("Error initializing logger manager:  %v", err)
    }

    startServer(appConfig, logMan)
}
