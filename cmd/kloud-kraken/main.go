package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/ngimb64/Kloud-Kraken/internal/color"
	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/internal/validate"
	"github.com/ngimb64/Kloud-Kraken/internal/vpcsetup"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"github.com/ngimb64/Kloud-Kraken/pkg/display"
	"github.com/ngimb64/Kloud-Kraken/pkg/ec2utils"
	"github.com/ngimb64/Kloud-Kraken/pkg/iamutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/kloudlogs"
	"github.com/ngimb64/Kloud-Kraken/pkg/netio"
	"github.com/ngimb64/Kloud-Kraken/pkg/s3utils"
	"github.com/ngimb64/Kloud-Kraken/pkg/ssmutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/tlsutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/tui"
	"github.com/ngimb64/Kloud-Kraken/pkg/wordlist"
	"github.com/ngimb64/Kloud-Kraken/pkg/yamlutils"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

// Package level variables
var CurrentConnections atomic.Int32	 // Tracks current active connections
var ReceivedDir = "/tmp/received"    // Path where cracked hashes & client logs are stored
var TlsMan = &tlsutils.TlsManager{}  // Struct for managing TLS certs, keys, etc.


// Select next available file for transfer, if there are no more available send
// the end transfer message to client. Format the transfer reply with the file
// name and size, get the IP address of the current connection and read the port
// from the socket to format the dialer for the new connection for file transfer.
// Finally pass the connection with other args goroutine to transfer file.
//
// @Parameters
//  - connection:  Network socket connection for handling messaging
//  - buffer:  The buffer storing network messaging
//  - waitGroup:  Used to synchronize the Goroutines running
//  - appConfig:  The configuration struct with loaded yaml program data
//  - logMan:  The kloudlogs logger manager for local logging
//  - ipAddr:  The IP address of the remote client connected to the server
//  - t:  The tui interface for displaying output
//
func handleTransfer(connection net.Conn, buffer []byte,
                    waitGroup *sync.WaitGroup,
                    appConfig *conf.AppConfig,
                    logMan *kloudlogs.LoggerManager,
                    ipAddr string, t *tui.TUI) {
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

    // Get random available port as a listener
    listener, port := netio.GetAvailableListener()

    // Format transfer reply to inform client of selected file name and size
    sendLength, err := netio.FormatTransferReply(filePath, fileSize, port, &buffer,
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

    // Set up context handler for TLS listener
    ctx, cancel := context.WithCancel(context.Background())
    // Setup up TLS listener from existing raw TCP listener
    tlsListener, err := TlsMan.SetupTlsListenerHandler(TlsMan.TlsCertificate,
                                                       TlsMan.CaCertPool, ctx,
                                                       "", port, listener)
    if err != nil {
        logMan.LogMessage("error", "Error setting TLS listener on client:  %v", err)
    }

    // Wait for an incoming connection
    transferConn, err := tlsListener.Accept()
    if err != nil {
        logMan.LogMessage("error", "Error accepting server connection:  %v", err)

        // Ensure TLS listener is closed
        err = tlsListener.Close()
        if err != nil {
            logMan.LogMessage("Error", "Error closing TLS listener:  %v", err)
        }

        // Call cancel function to ensure raw TCP socket is closed
        cancel()
        return
    }

    // Display the remote client connected for file transfer in left panel
    t.LeftPanelCh <- display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                            color.LightCyan, "!"), "",
                                        color.NeonAzure, "Connected ",
                                        color.RadiantAmethyst, ipAddr,
                                        color.NeonAzure, " on port ",
                                        color.KrakenGlowGreen, strconv.Itoa(int(port)))

    // Display the file name to be transfered in right panel
    t.RightPanelCh <- display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                             color.LightCyan, "!"), "",
                                         color.RadiantAmethyst, filepath.Base(filePath),
                                         color.NeonAzure, " transfering to ",
                                         color.RadiantAmethyst, ipAddr)

    logMan.LogMessage("info", "Connected remote client %s on port %d, %s to be transfered",
                      ipAddr, port, filePath)

    // Get the IP address of the remote connection
    remoteIp := strings.Split(transferConn.RemoteAddr().String(), ":")[0]
    // Increment waitgroup counter
    waitGroup.Add(1)

    go func() {
        defer func() {
            // Close the transfer connection
            err = transferConn.Close()
            if err != nil {
                logMan.LogMessage("Error", "Error closing file transfer connection %s:  %v",
                                  remoteIp, err)
            }

            // Close the TLS listener
            err = tlsListener.Close()
            if err != nil {
                logMan.LogMessage("Error", "Error closing the TLS listener:  %v", err)
            }

            // Call cancel function to close raw TCP socket
            cancel()

            // Decrement waitgroup counter
            waitGroup.Done()
        }()

        // Transfer the file to client
        err = netio.TransferFile(transferConn, filePath, fileSize)
        if err != nil {
            logMan.LogMessage("error", "Error occured transfering file to client %s:  %v",
                              remoteIp, err)
        }

        // Display the file path to be transfered in right panel
        t.RightPanelCh <- display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                                 color.LightCyan, "$"), "",
                                             color.RadiantAmethyst, filepath.Base(filePath),
                                             color.NeonAzure, " transfer completed to ",
                                             color.RadiantAmethyst, ipAddr)
    } ()
}


// Upload the hash and ruleset files (if optional ruleset applied). Goes into
// continual loop where data is read from the message sockets connection-buffer,
// checks for a processing complete message which signals exiting the loop,
// finally after the loop received cracked hash and log file.
//
// @Parameters
//  - connection:  Network socket connection for handling messaging
//  - waitGroup:  Used to synchronize the Goroutines running
//  - appConfig:  The configuration struct with loaded yaml program data
//  - logMan:  The kloudlogs logger manager for local logging
//  - remoteAddr:  IP address to remote client that has connected
//  - t:  The tui interface for displaying output
//
func handleConnection(connection net.Conn,
                      waitGroup *sync.WaitGroup,
                      appConfig *conf.AppConfig,
                      logMan *kloudlogs.LoggerManager,
                      remoteAddr string, t *tui.TUI) {
    var buffer []byte
    var err error

    defer func() {
        // Close the connection
        err = connection.Close()
        if err != nil {
            logMan.LogMessage("Error", "Error closing client connection %s:  %v",
                              connection.RemoteAddr(), err)
        }

        // Decrement the active connection count
        CurrentConnections.Add(-1)

        // Display the connection termination information in the left tui panel
        t.LeftPanelCh <- display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                                color.LightCyan, "-"), "",
                                            color.NeonAzure, "Connection closed for ",
                                            color.RadiantAmethyst, remoteAddr)

        logMan.LogMessage("info", "Connection processing handled",
                        zap.Int32("remaining connections", CurrentConnections.Load()))

        // Decrement waitGroup counter
        waitGroup.Done()
    } ()

    defer func () {
        // Receive log file from client
        _, err = netio.ReceiveFile(connection, buffer, ReceivedDir,
                                   globals.LOG_TRANSFER_PREFIX)
        if err != nil {
            logMan.LogMessage("error", "Error receiving log file:  %v", err)
            return
        }

        // Notify the log file has been received in the tui right panel
        t.RightPanelCh <- display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                                color.LightCyan, "$"), "",
                                             color.NeonAzure, "Log file received from client ",
                                             color.RadiantAmethyst, remoteAddr)
    } ()

    // Reset buffer to messaging size
    buffer = make([]byte, globals.MESSAGE_BUFFER_SIZE)

    // Upload the hash file to connection client
    err = netio.UploadFile(connection, buffer, appConfig.LocalConfig.HashFilePath,
                           globals.HASHES_TRANSFER_PREFIX)
    if err != nil {
        logMan.LogMessage("error", "Error sending the hash file to client:  %v", err)
        return
    }

    // Notify the hash file has been sent in the tui right panel
    t.RightPanelCh <- display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                             color.LightCyan, "$"), "",
                                         color.NeonAzure, "Hash file sent to client ",
                                         color.RadiantAmethyst, remoteAddr)

    // If a ruleset path was specified
    if appConfig.LocalConfig.RulesetPath != "" {
        // Upload the ruleset file to connection client
        err = netio.UploadFile(connection, buffer, appConfig.LocalConfig.RulesetPath,
                               globals.RULESET_TRANSFER_PREFIX)
        if err != nil {
            logMan.LogMessage("error", "Error sending the ruleset to server:  %v", err)
            return
        }

        // Notify the ruleset file has been sent in the tui right panel
        t.RightPanelCh <- display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                                 color.LightCyan, "$"), "",
                                             color.NeonAzure, "Ruleset file sent to client ",
                                             color.RadiantAmethyst, remoteAddr)
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
            handleTransfer(connection, buffer, waitGroup,
                           appConfig, logMan, remoteAddr, t)
        }
    }

    // Receive cracked user hash file from client
    _, err = netio.ReceiveFile(connection, buffer, ReceivedDir,
                               globals.LOOT_TRANSFER_PREFIX)
    if err != nil {
        logMan.LogMessage("error", "Error receiving cracked user hashes:  %v", err)
        return
    }

    // Notify the cracked hashes file has been received in the tui right panel
    t.RightPanelCh <- display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                             color.LightCyan, "$"), "",
                                         color.NeonAzure, "Cracked hashes received from client ",
                                         color.RadiantAmethyst, remoteAddr)
}


// Activates TUI interface and sets up TLS listener then enters loop where the
// amount of active connections is checked until the specified number of
// instances is equal to the active connections the listener will wait until a
// connection is accepted. Increment the active connections counter and
// waitgroup, and pass the connection with other args into handler goroutine.
//
// @Parameters
//  - appConfig:  The configuration struct with loaded yaml program data
//  - logMan:  The kloudlogs logger manager for local logging
//
func startServer(appConfig *conf.AppConfig,
                 logMan *kloudlogs.LoggerManager) {
    // Establish wait group for Goroutine synchronization
    var waitGroup sync.WaitGroup

    // Setup TUI interface for and ensure it closes on local exit
    t := tui.NewTUI(100, "Connections", 500 * time.Millisecond, 3, "File Transfers")
    go t.Start(color.SkyBlue, color.BrightMagenta, color.BrightMint)
    defer t.Stop()

    // Set up context handler for TLS listener
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    // Set up the TLS listener to accept incoming connections
    tlsListener, err := TlsMan.SetupTlsListenerHandler(TlsMan.TlsCertificate,
                                                       TlsMan.CaCertPool, ctx, "",
                                                       appConfig.LocalConfig.ListenerPort, nil)
    if err != nil {
        logMan.LogMessage("fatal", "Error setting up TLS listener:  %v", err)
    }

    // Close the TLS listener on local exit
    defer func() {
        err = tlsListener.Close()
        if err != nil {
            logMan.LogMessage("error", "Error closing TLS listener:  %v", err)
        }
    } ()

    // Display port TLS listener is on in the left panel
    t.LeftPanelCh <- display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                            color.LightCyan, "!"), "",
                                        color.NeonAzure, "Listening on port ",
                                        color.KrakenGlowGreen,
                                        strconv.Itoa(appConfig.LocalConfig.ListenerPort))

    logMan.LogMessage("info", "Listening for connections on port %d ..",
                      appConfig.LocalConfig.ListenerPort)

    for {
        // If number of connection is greater than or equal to number of instances
        if CurrentConnections.Load() >= appConfig.LocalConfig.NumberInstances {
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

        // Get the remote IP address for output/logging
        remoteAddr := connection.RemoteAddr().String()

        // Display the connection spawning information in the left tui panel
        t.LeftPanelCh <- display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                                color.LightCyan, "+"), "",
                                            color.NeonAzure, "Accepted ",
                                            color.RadiantAmethyst, remoteAddr)

        logMan.LogMessage("info", "Connection accepted from %s", remoteAddr,
                          zap.Int32("active connections", CurrentConnections.Load()))

        // Increment wait group and handle connection in separate Goroutine
        waitGroup.Add(1)
        go handleConnection(connection, &waitGroup, appConfig, logMan, remoteAddr, t)
    }

    // Wait for all active Goroutines to finish before shutting down the server
    waitGroup.Wait()

    // Sleep for a few seconds so information can be displayed before tui is stopped
    time.Sleep(5 * time.Second)
}


// Takes passed in args and formats into user data generated for EC2 creation.
//
// @Parameters
//  - appConf:  The configuration instance that stores program YAML data
//  - keyName:  The name of the key of the S3 bucket
//  - ipAddrs:  Slice of IP addresses to be formatted into CSV string
//  - ssmParam:  The path where the certificate is stored in SSM param store
//  - ec2SgId:  The ID for the security group for client EC2 instances
//
// @Returns
//  - The generated EC2 user data with args formatted into it
//  - Error if it occurs, otherwise nil on success
//
func ec2UserDataGen(appConf *conf.AppConfig, bucketName string, keyName string,
                    ipAddrs []string, ssmParam string, ec2SgId string) (
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
# Exit on any failure, error on undefined variables, echo each command, and
# catch failures in any part of a pipeline
set -euxo pipefail
# Captures both STDOUT & STDERR, sending everything to user data log file
exec > >(tee /var/log/user-data.log | logger -t user-data -s 2>/dev/console) 2>&1

# Get the NVMe instance store device names
mapfile -t DEVICES < <(lsblk -d -n -o NAME,TYPE |
    awk '$2=="disk" && $1 ~ /^nvme[0-9]+n1$/ {print "/dev/" $1}')

# If no NVMe instance store drives are found, log error and exit
if (( ${#DEVICES[@]} == 0 )); then
    echo "ERROR: no NVMe instance-store devices found"
    exit 1
fi

retries=0
# Update, upgrade, and install needed packages for hash cracking & RAID configuration
until DEBIAN_FRONTEND=noninteractive apt update && apt upgrade && apt install -y hashcat mdadm; do
    (( retries++ ))
    # If the updates process fails 3 times due to network issues, log error and exit
    (( retries >= 3 )) && { echo "ERROR: apt-get install failed"; exit 1; }
    sleep 5
done

# Create RAID 0 setup with idenified drives if it does not already exist
if ! mdadm --detail /dev/md0 &>/dev/null; then
    yes | mdadm --create /dev/md0 --level=0 --raid-devices=${#DEVICES[@]} "${DEVICES[@]}"
fi

# If filesystem for RAID drive does not exist, make it
if ! blkid /dev/md0 &>/dev/null; then
    mkfs.ext4 -F /dev/md0
fi

# Create mount point for instances store
mkdir -p /mnt/instance-store
# Add mount point to fstab if it is not already in there
grep -q '/mnt/instance-store' /etc/fstab || \
    echo "/dev/md0  /mnt/instance-store  ext4  defaults,nofail  0 2" >> /etc/fstab
# Mount the mount point if it is not already mounted
mountpoint -q /mnt/instance-store || mount /mnt/instance-store

echo "[!] Instance-store ready at /mnt/instance-store"

# Application bootstrap
CWD=$(pwd)
aws s3 cp s3://%s/%s "$CWD"/client --region %s --no-progress
chmod +x "$CWD"/client
$CWD/client -applyOptimization=%t \
            -awsRegion=%s \
            -certSsmParam=%s \
            -charSet1=%s \
            -charSet2=%s \
            -charSet3=%s \
            -charSet4=%s \
            -crackingMode=%s \
            -ec2SecurityGroupId=%s \
            -hashMask=%s \
            -hasRuleset=%t \
            -hashType=%s \
            -ipAddrs=%s \
            -isTesting=%t \
            -logMode=%s \
            -maxFileSizeInt64=%d \
            -maxTransfers=%d \
            -port=%d \
            -workload=%s
`, bucketName, keyName,
   appConf.ClientConfig.Region, true,
   appConf.ClientConfig.Region, ssmParam,
   appConf.ClientConfig.CharSet1, appConf.ClientConfig.CharSet2,
   appConf.ClientConfig.CharSet3, appConf.ClientConfig.CharSet4,
   appConf.ClientConfig.CrackingMode, ec2SgId,
   appConf.ClientConfig.HashMask, hasRuleset, appConf.ClientConfig.HashType,
   ipAddrsCsv, false, appConf.ClientConfig.LogMode,
   appConf.ClientConfig.MaxFileSizeInt64, appConf.ClientConfig.MaxTransfers,
   appConf.LocalConfig.ListenerPort, appConf.ClientConfig.Workload)

    return data, nil
}


// Sets up AWS credentials, then the VPC and all its inner workings are
// provisioned where it checks if the resource exists and creates it if it does
// not. Uses IAM permissions in the credentials to set up client and server
// roles in IAM. Then assumes created server role via STS service. Puts
// generated TLS certificate in SSM parameter store and client binary in S3
// bucket for later retrieval. Concludes by launching EC2 instances and
// ensure proper termination via defer.
//
// @Parameters
//  - appConfig:  The configuration instance with program YAML data
//  - publicIps:  List of public IPs to format into user data template
//
// @Returns
//  - The initialized AWS configuration instance
//  - The VPC bootstrap output struct
//  - Errors associated with cost manager
//  - Error if it occurs, otherwise nil on success
//
func awsSetup(appConfig *conf.AppConfig, publicIps []string) (
              awsConfig aws.Config,
              bootstrapOut *vpcsetup.VpcBootstrapOutput,
              costErr error, err error) {
    // Set up the AWS credentials based on local chain or environment variables
    awsConfig, err = awsutils.AwsConfigSetup(1 * time.Minute,
                                             appConfig.LocalConfig.Region)
    if err != nil {
        return awsConfig, bootstrapOut, nil, err
    }

	// Establish clients to various services
	ec2Client := ec2utils.Ec2NewManager(awsConfig)
	iamClient := iamutils.IamNewManager(awsConfig)
	stsClient := sts.NewFromConfig(awsConfig)

    // Set up the kloud kraken VPC and its associated components
    bootstrapOut, costMan, costErr, err := vpcsetup.VpcBootstrap(appConfig, awsConfig,
                                                                 ec2Client, iamClient,
                                                                 *stsClient)
    if err != nil {
        return awsConfig, bootstrapOut, costErr, err
    }

    bootstrapOut.Ec2Client = ec2Client

    // Create a provider that will call STS AssumeRole under the covers
    assumeProvider := stscreds.NewAssumeRoleProvider(stsClient, bootstrapOut.ServerArn)

    // Create fresh AWS config from new STS provider
    awsConfig, err = config.LoadDefaultConfig(
        context.TODO(),
        config.WithRegion(appConfig.LocalConfig.Region),
        config.WithCredentialsProvider(aws.NewCredentialsCache(assumeProvider)),
    )
    if err != nil {
        return awsConfig, bootstrapOut, costErr, err
    }

    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-ssm-tls-cert",
    }

    // Setup client to SSM
    ssmClient := ssmutils.SsmNewManager(awsConfig)
    // Push the servers certificate PEM into SSM parameter store
    ssmParam, err := ssmClient.SsmPutParameter(1 * time.Minute,
                                               "/kloud-kraken/tls-cert",
                                               string(TlsMan.CertPemBlock),
                                               tags)
    if err != nil {
        return awsConfig, bootstrapOut, costErr, err
    }

    bootstrapOut.SsmClient = ssmClient

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "TLS certificate uploaded to " +
                                   "SSM Parameter Store for client retrieval"))

    // Read the client binary into memory
    binData, err := os.ReadFile("./kloud-kraken-client")
    if err != nil {
        return awsConfig, bootstrapOut, costErr, err
    }

    // Re-establish client to S3 with new API key set
    s3Client := s3utils.S3NewManager(awsConfig)
    // Upload the client binary to S3 Bucket
    keyName, err := s3Client.S3PutObject(5 * time.Minute,
                                         bootstrapOut.S3BucketName,
                                         "client", binData)
    if err != nil {
        return awsConfig, bootstrapOut, costErr, err
    }

    bootstrapOut.S3Client = s3Client

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "Uploaded client binary to S3 bucket ",
                                   color.RadiantAmethyst, bootstrapOut.S3BucketName))

    // Generate user data script to set up client program in EC2
    userData, err := ec2UserDataGen(appConfig, bootstrapOut.S3BucketName,
                                    keyName, publicIps, ssmParam,
                                    bootstrapOut.Ec2SgId)
    if err != nil {
        return awsConfig, bootstrapOut, costErr, err
    }

    tags = map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-ec2-client",
    }

    ec2CreateInstancesInput := &ec2utils.Ec2CreateInstancesInput{
        AMI:              "ami-0eb94e3d16a6eea5f",
        InstanceType:     appConfig.LocalConfig.InstanceType,
        MaxCount:         appConfig.LocalConfig.NumberInstances,
        MinCount:         appConfig.LocalConfig.NumberInstances,
        RoleName:         "KloudKrakenClientRole",
        SecurityGroupIds: []string{bootstrapOut.Ec2SgId},
        SubnetId:         bootstrapOut.SubnetId,
        Tags:             tags,
        UserData:         []byte(userData),
    }

    // Re-setup new client to EC2 service with newly assumed role
    ec2Client = ec2utils.Ec2NewManager(awsConfig)
    // Create number of EC2 instances based on passed in data
    err = ec2Client.Ec2CreateInstances(15 * time.Minute, ec2CreateInstancesInput)
    if err != nil {
        return awsConfig, bootstrapOut, costErr, err
    }

    filterMap := map[string]string{
        "instanceType": appConfig.LocalConfig.InstanceType,
        "location": awsConfig.Region,
		"operatingSystem":"Linux",
    }

    // Add the Ec2 instances to the cost manager
    _ = costMan.AddCostResourceToManagerHandler("ec2_instance", filterMap, true, &costErr)

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "EC2 instance creation completed"))

    return awsConfig, bootstrapOut, costErr, nil
}


// Displays the Kloud Kraken ascii banner.
//
func printBanner() {
    // Print program banner
    fmt.Println(color.MistyAqua + `
          ,.                                     ..
           xO:     c. .                  .    'o0,
           .XM'  l0. .d     co    c.  d  kKdOWMk
           .MM.;NN' xM;   lKKxXx.lMl  MX,oMx,oM0'
           .MM0Ml   OM.   dM. dM'lMc  MN oMl  xMk
           .MMXMx   OM.  'dM. dM'lMc  MN oMl  dMo
           .MM cMO. OM..0'kM' xM,lMd .MW oMl .0M0.
           xMM. :MX,xXNOx:'dN0Kc..cKKNd'.KMXOWK:.
      .;, .'Wl '..cko.,   . .d    . :..;o:..,:. .  ,c'.
        dWd.'lXd:0coO:. .dWl. 'O; lkd:llo0c.d0'  KWd
        .Md dN: oMl'KWd .M0M. ;Mc,Nk. KW';o dM0  OM.
        ,MKNO.  lMx0Xc  xX.Md ;MKMo   KMco  dMN0 0M
        .MNdWO  lMkxWo .MKkNW.;MOdW;  KM;' ,dM'KKXM
        .Mx :Wx oM: oM.OM  'MO:Ml oM: XMldN,xM..0MM.
        lM0  lX0;0.  Oldx   xk.X. .c0d''ok,'Nd. :MMo
        OWo.   ,o,    :.     . .     .,   .o.   .cXX.
       dd.       .                        .        :O.
      ;.                                             :.
    ` + color.AnsiReset)
}


// Create the required dirs for program operation.
//
func makeServerDirs() {
    // Set the program directories
    programDirs := []string{ReceivedDir}
    // Create needed directories
    err := disk.MakeDirs(programDirs)
    if err != nil {
        log.Fatalf("Error creating server dirs:  %v", err)
    }
}


// Parses command line args (path to yaml config file), if args not present
// or invalid then proceeds to user input until valid yaml file is specified.
//
// @Returns
//  - Pointer to AppConfig struct populated from yaml data
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
            log.Fatalf("Error checking config file path existence:  %v", err)
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

    // Load the YAML data into AppConfig struct
    appConfig, err := conf.LoadConfig(configFilePath)
    if err != nil {
        log.Fatalf("Error loading YAML data:  %v", err)
    }

    return appConfig
}


// Parse command line args, make needed directories, merge wordlists and remove remaining
// empty dirs. Set up AWS access config with key and secret, set up AWS resource cleanup
// for certain resources before the program terminates set up logging manager instance,
// set up EC2 code passing command line args via user data, and start server.
//
func main() {
    // Begin recording program timing
    startTime := time.Now()
    // Display the total execution time when program exits
    defer fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                             color.LightCyan, "$"), "",
                                         color.NeonAzure, "Total runtime:  ",
                                         color.KrakenGlowGreen,
                                         time.Since(startTime).String()))

    // Handle selecting the YAML file if no arg provided
    // and load YAML data into struct configuration class
    appConfig := parseArgs()
    // Make the server directories
    makeServerDirs()
    // Display the kloud kraken banner
    printBanner()

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Wordlist merging started, time" +
                                   " varies greatly depending on how much data"))

    // Merge the wordlists in the load dir based on max file size
    err := wordlist.MergeWordlistDir(appConfig.LocalConfig.LoadDir,
                                     appConfig.LocalConfig.MaxMergingSizeInt64,
                                     appConfig.ClientConfig.MaxFileSizeInt64,
                                     appConfig.LocalConfig.MaxSizeRange)
    if err != nil {
        log.Fatalf("Error merging wordlists:  %v", err)
    }

    // Delete any leftover folders in load dir
    err = wordlist.RemoveMergeSubdirs(appConfig.LocalConfig.LoadDir)
    if err != nil {
        log.Fatalf("Error deleting load dir subdirs:  %v", err)
    }

    fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                   display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "Wordlist merging process completed"))

    var awsConfig aws.Config
    var bootstrapOut *vpcsetup.VpcBootstrapOutput
    var logMan *kloudlogs.LoggerManager

    // If the program is being run in full mode (not testing)
    if !appConfig.LocalConfig.LocalTesting {
        // Query IP lookup APIs for public IP addresses
        publicIps, err := tlsutils.GetPublicIps()
        if err != nil {
            log.Fatalf("Error getting public IP addresses:  %v", err)
        }

        fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "Server public IP addresses retrieved"))

        // Generate the servers TLS PEM certificate and key and save in TLS manager
        err = TlsMan.PemCertAndKeyGenHandler("Kloud Kraken", false, publicIps...)
        if err != nil {
            log.Fatalf("Error creating TLS PEM certificate & key:  %v", err)
        }

        fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "Server TLS PEM certificate " +
                                       "and key generated"))
        var costErr error

        // Call handler function that sets up AWS IAM user permissions,
        // transfers client binary via S3, set TLS certificate via SSM
        // parameter store, and launches EC2 instances
        awsConfig, bootstrapOut, costErr, err = awsSetup(appConfig, publicIps)
        if err != nil {
            log.Fatalf("Error with AWS setup:  %v", err)
        }

        // AWS cost optimization resource deletion cleanup routine
        defer func() {
            var stateConfig vpcsetup.AwsEnv
            var stateData []byte
            stateFilePath := "../.kraken-state.yml"
            var yamlUpdates map[string]string

            // Read the data from yaml state file
            stateData, err = os.ReadFile(stateFilePath)
            if err != nil {
                log.Printf("Error reading state file for cleanup:  %v", err)
            }

            // Decode raw bytes into StateConfig struct
            err = yaml.Unmarshal(stateData, &stateConfig)
            if err != nil {
                log.Printf("Error unmarshaling state file YAML data into state struct:  %v", err)
            }

            defer func() {
                // If there are no values in YAML file to be updated
                if len(yamlUpdates) == 0 {
                    return
                }

                // Update the yaml values with values from passed in map
                newYaml, err := yamlutils.UpdateYAMLBytes(stateData, yamlUpdates)
                if err != nil {
                    log.Printf("Error updating state data with entries in map:  %v", err)
                    return
                }

                // Overwrite the original yaml with the updated data
                err = os.WriteFile(stateFilePath, newYaml, 0644)
                if err != nil {
                    log.Printf("Error writing state data to state file:  %v", err)
                }
            }()

            // If error occured during AWS resource cost calculation
            if costErr != nil {
                if logMan != nil {
                    logMan.LogMessage("error", "Error occured during AWS cost" +
                                      " calulcation:  %v", err)
                } else {
                    log.Printf("Error occured during AWS cost calulcation:  %v", err)
                }
            }

            // Terminate the EC2 instances
            termOutput, err := bootstrapOut.Ec2Client.Ec2TerminateInstances(10 * time.Minute)
            if err != nil {
                log.Printf("Error terminating EC2 instances:  %v", err)
            }

            // Iterate through list of terminated instance ids and log them
            for _, instance := range termOutput.TerminatingInstances {
                if logMan != nil {
                    logMan.LogMessage("Instance state for %s: %s -> %s\n",
                                      aws.ToString(instance.InstanceId),
                                      instance.PreviousState.Name,
                                      instance.CurrentState.Name)
                } else {
                    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                                       color.LightCyan, "+"), "",
                                                   color.NeonAzure, "Instance state for ",
                                                   color.RadiantAmethyst,
                                                   aws.ToString(instance.InstanceId),
                                                   color.NeonAzure, ": ", color.KrakenGlowGreen,
                                                   string(instance.PreviousState.Name),
                                                   color.NeonAzure, " -> ", color.KrakenGlowGreen,
                                                   string(instance.CurrentState.Name)))
                }
            }

            // Terminate SSM Parameter Store VPC Interface Endpoint
            err = bootstrapOut.Ec2Client.VpcEndpointsTerminator(1 * time.Minute,
                                                                []string{bootstrapOut.SsmVpcEndpointId})
            if err != nil {
                if logMan != nil {
                    logMan.LogMessage( "error", "Error deleting SSM Parameter Store" +
                                       " VPC Interface Endpoint:  %v", err)
                } else {
                    log.Printf("Error deleting SSM Parameter Store VPC" +
                               " Interface Endpoint:  %v", err)
                }
            } else {
                yamlUpdates["aws_env.ssm_vpc_endpoint_id"] = ""
            }

            // Delete the S3 bucket and its contents
            err = bootstrapOut.S3Client.S3BucketTerminator(5 * time.Minute,
                                                           bootstrapOut.S3BucketName)
            if err != nil {
                if logMan != nil {
                    logMan.LogMessage("error", "Error deleting S3 bucket and " +
                                      "its contents:  %v", err)
                } else {
                    log.Printf("Error deleting S3 bucket and its contents:  %v", err)
                }
            } else {
                yamlUpdates["aws_env.s3_bucket_name"] = ""
            }

            // Delete all the client TLS certificate from SSM Parameter store
            err = bootstrapOut.SsmClient.SsmDeleteAllParams(1 * time.Minute,
                                                            "/kloud-kraken/tls-cert")
            if err != nil {
                if logMan != nil {
                    logMan.LogMessage("error", "Error deleting parameters from" +
                                      " SSM Param Store:  %v", err)
                } else {
                    log.Printf("Error deleting parameters from SSM Param Store:  %v", err)
                }
            }
        } ()

    // If the program is being run in testing mode
    } else {
        // Generate the servers TLS PEM certificate & key and save in TLS manager
        err = TlsMan.PemCertAndKeyGenHandler("Kloud Kraken", true)
        if err != nil {
            log.Fatalf("Error creating TLS PEM certificate and key:  %v", err)
        }

        fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "TESTING"), "",
                                       color.NeonAzure, "PEM cert generated, transfer " +
                                       " to client before execution"))
    }

    // Generate a TLS x509 certificate and cert pool
    err = TlsMan.CertGenAndPool(TlsMan.CertPemBlock, TlsMan.KeyPemBlock,
                                TlsMan.CaCertPemBlocks)
    if err != nil {
        log.Fatalf("Error generating TLS certificate:  %v", err)
    }

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "X509 cerificate pool generated " +
                                   "and server certifcate added to pool"))

    // Initialize the LoggerManager based on the flags
    logMan, err = kloudlogs.NewLoggerManager("local", "KloudKraken.log",
                                             awsConfig, "", -1, nil, false)
    if err != nil {
        log.Fatalf("Error initializing logger manager:  %v", err)
    }

    // Sleep briefly to so output can be read before tui starts
    time.Sleep(5 * time.Second)

    // Listen for incoming client connections and handle them
    startServer(appConfig, logMan)

    // Redisplay banner once processing is complete
    printBanner()

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "All connections handled " +
                                   ".. server shutting down"))

    logMan.LogMessage("info", "All connections handled .. server shutting down")
}
