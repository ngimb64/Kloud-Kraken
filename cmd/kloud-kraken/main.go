package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/internal/validate"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"github.com/ngimb64/Kloud-Kraken/pkg/display"
	"github.com/ngimb64/Kloud-Kraken/pkg/kloudlogs"
	"github.com/ngimb64/Kloud-Kraken/pkg/netio"
	"github.com/ngimb64/Kloud-Kraken/pkg/tlsutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/wordlist"
	"go.uber.org/zap"
)

// Package level variables
var CurrentConnections atomic.Int32		// Tracks current active connections
var ReceivedDir = "/tmp/received"       // Path where cracked hashes & client logs are stored


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
        kloudlogs.LogMessage(logMan, "error",
                             "Error selecting the next available file to transfer:  %v", err)
        return
    }

    // If there are no more files available to be transfered
    if filePath == "" {
        // Send the end transfer message then exit function
        _, err = netio.WriteHandler(connection, globals.END_TRANSFER_MARKER,
                                    len(globals.END_TRANSFER_MARKER))
        if err != nil {
            kloudlogs.LogMessage(logMan, "error", "Error sending the end transfer message:  %v", err)
        }

        return
    }

    // Format transfer reply to inform client of selected file name and size
    sendLength, err := netio.FormatTransferReply(filePath, fileSize, &buffer,
                                                 globals.START_TRANSFER_PREFIX)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                             "Error formatting transfer reply:  %v", err)
        return
    }

    // Send the transfer reply with file name and size
    _, err = netio.WriteHandler(connection, buffer, sendLength)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error sending the transfer reply:  %v", err)
        return
    }

    // Get the IP address from the ip:port host address
    ipAddr, _, err := netio.GetIpPort(connection)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                             "Error occcurred spliting host address to get IP/port:  %v", err)
        return
    }

    var port uint16
    // Receive bytes of port of client port to connect to for file transfer
    err = binary.Read(connection, binary.LittleEndian, &port)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error receiving client listener port:  %v", err)
        return
    }

    // Format remote address with IP and port
    remoteAddr := ipAddr + ":" + strconv.Itoa(int(port))


    // TODO:  add TLS logic once initial connection is finished


    // Make a connection to the remote brain server
    transferConn, err := net.Dial("tcp", remoteAddr)
    if err != nil {
        kloudlogs.LogMessage(logMan, "fatal", "Error connecting to remote client for transfer:  %v", err)
        return
    }

    kloudlogs.LogMessage(logMan, "info", "Connected remote client at %s on port %d", ipAddr, port)
    // Increment waitgroup counter
    waitGroup.Add(1)

    go func() {
        // Close transfer connection and decrement waitgroup counter on local exit
        defer transferConn.Close()
        defer waitGroup.Done()

        // Transfer the file to client
        err = netio.TransferFile(transferConn, filePath, fileSize)
        if err != nil {
            kloudlogs.LogMessage(logMan, "error",
                                 "Error occured transfering file to client:  %v", err)
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

    // Set message buffer size
    buffer := make([]byte, globals.MESSAGE_BUFFER_SIZE)


    // TODO:  receive the client certificate and add it to the servers cert pool


    // Upload the hash file to connection client
    err := netio.UploadFile(connection, buffer, appConfig.LocalConfig.HashFilePath,
                            globals.HASHES_TRANSFER_PREFIX)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error",
                             "Error occured sending the hash file to client:  %v", err)
        return
    }

    // If a ruleset path was specified
    if appConfig.LocalConfig.RulesetPath != "" {
        // Upload the ruleset file to connection client
        err = netio.UploadFile(connection, buffer, appConfig.LocalConfig.RulesetPath,
                               globals.RULESET_TRANSFER_PREFIX)
        if err != nil {
            kloudlogs.LogMessage(logMan, "error",
                                 "Error occured sending the ruleset to server:  %v", err)
            return
        }
    }

    for {
        // Read data from connected client
        bytesRead, err := netio.ReadHandler(connection, &buffer)
        if err != nil {
            kloudlogs.LogMessage(logMan, "error",
                                 "Error occurred reading data from socket:  %v", err)
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
        kloudlogs.LogMessage(logMan, "error", "Error receiving cracked user hashes:  %v", err)
        return
    }

    // Receive log file from client
    _, err = netio.ReceiveFile(connection, buffer, ReceivedDir,
                               globals.LOG_TRANSFER_PREFIX)
    if err != nil {
        kloudlogs.LogMessage(logMan, "error", "Error receiving log file:  %v", err)
        return
    }

    // Decrement the active connection count
    CurrentConnections.Add(-1)

    kloudlogs.LogMessage(logMan, "info", "Connection processing handled",
                         zap.Int32("remaining connections", CurrentConnections.Load()))
}


// Creates TLS x509 certificate and a cert pool which are used to setup the TLS
// configuration instance. After a TLS listener is established and returned.
//
// @Parameters
// - appConfig:  The configuration struct with loaded yaml program data
// - logMan:  The kloudlogs logger manager for local logging
// - ctx:  The context instance for managing original raw listener
//
// @Returns
// - The established TLS listener
//
func setupTlsListener(appConfig *conf.AppConfig, logMan *kloudlogs.LoggerManager,
                      ctx context.Context) net.Listener {
    // Generate certificate base on certificate & key PEM blocks
    cert, err := tls.X509KeyPair(appConfig.LocalConfig.TlsCertPem,
                                 appConfig.LocalConfig.TlsKeyPem)
    if err != nil {
        kloudlogs.LogMessage(logMan, "fatal", "Error generating TLS x509 certificate:  %v", err)
    }

    // Create empty server x509 certificate pool
    serverPool := x509.NewCertPool()

    // Create a TLS configuarion instance
    tlsConfig := &tls.Config{
        Certificates:       []tls.Certificate{cert},
        GetConfigForClient: tlsutils.GetTlsConfig(cert, serverPool),
    }

    // Format listener address with port from parsed YAML data
    listenerAddr := ":" + strconv.Itoa(appConfig.LocalConfig.ListenerPort)
    // Create a TLS server instance
    tlsServer := tlsutils.NewTlsServer(ctx, listenerAddr, tlsConfig)
    // Setup TLS listener from server instance
    tlsListener, err := tlsServer.SetupTlsListener(appConfig.LocalConfig.TlsCertPem,
                                                   appConfig.LocalConfig.TlsKeyPem)
    if err != nil {
        kloudlogs.LogMessage(logMan, "fatal", "Error finalizing TLS listener setup:  %v", err)
    }

    return tlsListener
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
    tlsListener := setupTlsListener(appConfig, logMan, ctx)

    kloudlogs.LogMessage(logMan, "info", "Server started, waiting for connections ..")

    for {
        // If current number of connection is greater than or equal to number of instances
        if CurrentConnections.Load() >= appConfig.LocalConfig.NumberInstances {
            kloudlogs.LogMessage(logMan, "info", "All remote clients are connected")
            break
        }

        // Wait for an incoming connection
        connection, err := tlsListener.Accept()
        if err != nil {
            kloudlogs.LogMessage(logMan, "error", "Error accepting client connection:  %v", err)
            continue
        }

        // Increment the active connection count
        CurrentConnections.Add(1)

        kloudlogs.LogMessage(logMan, "info", "Connection accepted to remote client",
                             zap.Int32("active connections", CurrentConnections.Load()))

        // Increment wait group and handle connection in separate Goroutine
        waitGroup.Add(1)
        go handleConnection(connection, &waitGroup, appConfig, logMan)
    }

    // Wait for all active Goroutines to finish before shutting down the server
    waitGroup.Wait()

    kloudlogs.LogMessage(logMan, "info", "All connections handled .. server shutting down")
}


// Set up the AWS config with credentials and region stored in passed in app config.
//
// @Paramters
// - appConfig:  The configuration struct for application
//
// @Returns:
// - The initialized AWS credentials config
//
func awsConfigSetup(appConfig conf.AppConfig) aws.Config {
    // Get the AWS access and secret key environment variables
    awsAccessKey := os.Getenv("AWS_ACCESS_KEY")
    awsSecretKey := os.Getenv("AWS_SECRET_KEY")
    // If AWS access and secret key are present
    if awsAccessKey == "" || awsSecretKey == "" {
        log.Fatal("Missing either the access or the secret key for AWS")
    }

    // Set the AWS credentials provider
    awsCreds := credentials.NewStaticCredentialsProvider(awsAccessKey, awsSecretKey, "")

    // Load default config and override with custom credentials and region
    awsConfig, err := config.LoadDefaultConfig(
        context.TODO(),
        config.WithRegion(appConfig.LocalConfig.Region),
        config.WithCredentialsProvider(awsCreds),
    )

    if err != nil {
        log.Fatalf("Error loading server AWS config:  %v", err)
    }

    return awsConfig
}


// Generates the TLS certificate & key, saving the result in the appConifg struct.
//
// @Parameters
// - appConfig:  The program configuration instance where TLS cert & key are stored
// - testMode:  boolean toggle for whether PEM file should be generated or not
//
func tlsCertAndKeyGen(appConfig *conf.AppConfig, testMode bool) {
    // Add the localhost to CA hosts list
    hosts := "localhost"

    // Get available usable public/private IP's assigned to network interfaces
    ipAddrs, err := tlsutils.GetUsableIps()
    if err != nil {
        log.Fatalf("Error getting usable IP's from network interfaces:  %v", err)
    }

    // Iterate through the list IP's and add them to CSV string
    for _, ipAddr := range ipAddrs {
        hosts += ("," + ipAddr)
    }

    // Generate the TLS certificate/key and save them in app config
    certPem, keyPem, err := tlsutils.TlsCertAndKeyGen("Kloud Kraken", hosts, testMode)
    if err != nil {
        log.Fatalf("Error creating TLS cert & key pair:  %v", err)
    }

    // Assign the resulting PEM certificate and key to their locations in appConfig struct
    appConfig.LocalConfig.TlsCertPem, appConfig.LocalConfig.TlsKeyPem = certPem, keyPem
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

    // If the program is being run in full mode (not testing)
    if !appConfig.LocalConfig.LocalTesting {

       // TODO:  get the public IP address via IPify API with backup alternatives
       //        if that fails, and pass result into below TLS function


        // Generate the TLS certificate & key and save in appConfig
        tlsCertAndKeyGen(appConfig, false)

        // Set up the AWS credentials based on environment variables
        awsConfig = awsConfigSetup(*appConfig)


        // TODO:  ensure server certificate uploaded via parameter store so client can retrieve it

        // TODO:  add function calls for setting up AWS and spawning EC2
        //        with user data executing service with params passed in


    // If the program is being run in testing mode
    } else {
        // Generate the TLS certificate & key and save in appConfig
        tlsCertAndKeyGen(appConfig, true)

        log.Println("Testing mode:  TLS cert & key generated, transfer cert to client before execution")
    }

    // Initialize the LoggerManager based on the flags
    logMan, err := kloudlogs.NewLoggerManager("local", appConfig.LocalConfig.LogPath, awsConfig, false)
    if err != nil {
        log.Fatalf("Error initializing logger manager:  %v", err)
    }

    startServer(appConfig, logMan)
}
