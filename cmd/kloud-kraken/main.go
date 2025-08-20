package main

import (
	"bytes"
	"context"
	"errors"
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
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/cidrutils"
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
	"gopkg.in/yaml.v3"
)

// Package level variables
var CurrentConnections atomic.Int32	   // Tracks current active connections
var ReceivedDir = "/tmp/received"      // Path where cracked hashes & client logs are stored
var TlsMan = new(tlsutils.TlsManager)  // Struct for managing TLS certs, keys, etc.

type StateConfig struct {
    BucketName           string `yaml:"bucket_name"`
    EipId                string `yaml:"eip_id"`
    IgwId                string `yaml:"igw_id"`
    NatGatewayId         string `yaml:"nat_gateway_id"`
    PrivateAssociationId string `yaml:"private_association_id"`
    PrivateRouteId       string `yaml:"private_route_id"`
    PrivateSubnetId      string `yaml:"private_subnet_id"`
    PublicAssociationId  string `yaml:"public_association_id"`
    PublicRouteId        string `yaml:"public_route_id"`
    PublicSubnetId       string `yaml:"public_subnet_id"`
    VpcId                string `yaml:"vpc_id"`
}

// Select next available file for transfer, if there are no more available send the end transfer
// message to client. Format the transfer reply with the file name and size, get the IP address
// of the current connection and read the port from the socket to format the dialer for the new
// connection for file transfer. Finally pass the connection with other args into TransferFile().
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


// Upload the hash and ruleset files (if optional ruleset applied). Goes into continual loop
// where data is read from the message sockets connection-buffer, checks for a processing complete
// message which signals exiting the loop, finally after the loop received cracked hash and log file.
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
    // Close the connection on local exit
    defer func() {
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

        // Decrement waitGroup counter on local exit
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

    // Set buffer to receive client PEM certificate
    buffer = make([]byte, 2 * globals.KB)

    // Receive the client PEM certificate bytes
    bytesRead, err := netio.ReadHandler(connection, &buffer)
    if err != nil {
        logMan.LogMessage("error", "Error reading client PEM cert:  %v", err)
        return
    }

    // Add the read client PEM cert to the cert pool
    err = TlsMan.AddCACert(buffer[:bytesRead])
    if err != nil {
        logMan.LogMessage("error", "Error adding PEM cert to pool:  %v", err)
        return
    }

    // Notify TLS cerificate has been received in the tui right panel
    t.RightPanelCh <- display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                             color.LightCyan, "$"), "",
                                         color.NeonAzure, "TLS certificate received from client ",
                                         color.RadiantAmethyst, remoteAddr)

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


// Set up listener and enter loop where the amount of active connections is checked
// until the specified number of instances is equal to the active connections the
// listener will wait until a connection is accepted. Increment the active connections
// counter and waitgroup, and pass the connection with other args into handler goroutine.
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
//
// @Returns
//  - The generated EC2 user data with args formatted into it
//  - Error if it occurs, otherwise nil on success
//
func ec2UserDataGen(appConf *conf.AppConfig, stateConfig *StateConfig,
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
aws s3 cp s3://%s/%s $CWD/client --region %s --no-progress
chmod +x $CWD/client
$CWD/client -applyOptimization=%t \
            -awsRegion=%s \
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
`, stateConfig.BucketName, keyName,
   appConf.ClientConfig.Region, true,
   appConf.ClientConfig.Region, ssmParam,
   appConf.ClientConfig.CharSet1, appConf.ClientConfig.CharSet2,
   appConf.ClientConfig.CharSet3, appConf.ClientConfig.CharSet4,
   appConf.ClientConfig.CrackingMode, appConf.ClientConfig.HashMask,
   appConf.ClientConfig.HashType, hasRuleset, ipAddrsCsv, false,
   appConf.ClientConfig.LogMode, appConf.ClientConfig.LogPath,
   appConf.ClientConfig.MaxFileSizeInt64, appConf.ClientConfig.MaxTransfers,
   appConf.LocalConfig.ListenerPort, appConf.ClientConfig.Workload)

    return data, nil
}


// Generates permission policy for the server.
//
// @Parameters
//  - region:  The AWS region where actions will be performed
//  - accountId:  The AWS account ID where actions will be performed
//  - ssmParam:  The path where the certificate is stored in SSM param store
//  - bucketName:  The name of the S3 bucket where actions will be performed
//  - clientRoleName:  The name of IAM role the client will be using
//
// @Returns
//  - The generated permissions policy with args formatted into it
//
func serverPermPolicyGen(region string, accountId string,
                         ssmParam string, bucketName string,
                         clientRoleName string) string {
    return fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "SSMUploadClientCert",
      "Effect": "Allow",
      "Action": [
        "ssm:PutParameter"
      ],
      "Resource": "arn:aws:ssm:%s:%s:parameter%s*"
    },
    {
      "Sid": "S3UploadClientBinary",
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:PutObjectAcl"
      ],
      "Resource": "arn:aws:s3:::%s/*"
    },
    {
      "Sid": "EC2LifecycleControl",
      "Effect": "Allow",
      "Action": [
        "ec2:RunInstances",
        "ec2:TerminateInstances",
        "ec2:DescribeInstances",
        "ec2:CreateTags"
      ],
      "Resource": [
        "arn:aws:ec2:%s:%s:instance/*",
        "arn:aws:ec2:%s:%s:subnet/*",
        "arn:aws:ec2:%s:%s:security-group/*"
      ]
    },
    {
      "Sid": "EC2PassRoleForInstanceProfile",
      "Effect": "Allow",
      "Action": [
        "iam:PassRole"
      ],
      "Resource": "arn:aws:iam::%s:role/%s"
    }
  ]
}`, region, accountId, ssmParam, bucketName, region, accountId, region,
    accountId, region, accountId, accountId, clientRoleName)
}


// Generates trust policy for the server.
//
// @Parameters
//  - accountId:  The AWS account ID where actions will be performed
//  - iamUser:  The IAM user that the policy will apply to
//
// @Returns
//  - The generated trust policy with args formatted into it
//
func serverTrustPolicyGen(accountId string, iamUser string) string {
    return fmt.Sprintf(`{
  "Version":"2012-10-17",
  "Statement":[{
    "Effect":"Allow",
    "Principal":{
      "AWS":"arn:aws:iam::%s:user/%s"
    },
    "Action":"sts:AssumeRole"
  }]
}`, accountId, iamUser)
}


// Generates permission policy for the client.
//
// @Parameters
//  - bucketName:  The name of the S3 bucket where actions will be performed
//  - region:  The AWS region where actions will be performed
//  - accountId:  The AWS account ID where actions will be performed
//  - paramPath:  The path where the certificate is stored in SSM param store
//  - logGroup:  The name of the CloudWatch group being utilized
//
// @Returns
//  - The generated permissions policy with args formatted into it
//
func clientPermPolicyGen(bucketName string, region string,
                         accountId string, paramPath string,
                         logGroup string) string {
    return fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "S3DownloadBinary",
      "Effect": "Allow",
      "Action": [
        "s3:GetObject"
      ],
      "Resource": "arn:aws:s3:::%s/*"
    },
    {
      "Sid": "SSMFetchParameters",
      "Effect": "Allow",
      "Action": [
        "ssm:GetParameter",
        "ssm:GetParameters",
        "ssm:GetParametersByPath"
      ],
      "Resource": [
        "arn:aws:ssm:%s:%s:parameter%s*"
      ]
    },
    {
      "Sid": "CloudWatchLogging",
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents"
      ],
      "Resource": "arn:aws:logs:%s:%s:log-group:/%s*"
    }
  ]
}`, bucketName, region, accountId, paramPath, region, accountId, logGroup)
}


// Generates trust policy for the client.
//
// @Returns
//  - The generated trust policy with args formatted into it
//
func clientTrustPolicyGen() string {
    return `{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect":    "Allow",
    "Principal": { "Service": "ec2.amazonaws.com" },
    "Action":    "sts:AssumeRole"
  }]
}`
}


// Sets up AWS credentials, then the VPC and all its inner workings are provisioned
// where it checks if the resource exists and creates it if it does not. Uses IAM
// permissions in the credentials to set up client and server roles in IAM. Then
// assumes created server role via STS service. Puts generated TLS certificate in
// SSM parameter store and client binary in S3 bucket for later retrieval.
// Concludes by launching EC2 instances and ensure proper termination via defer.
//
// @Parameters
//  - appConfig:  The configuration instance with program YAML data
//  - publicIps:  List of public IPs to format into user data template
//
// @Returns
//  - The initialized AWS configuration instance
//  - The EC2 manager instance to utilize for later operations
//  - Error if it occurs, otherwise nil on success
//
func awsSetup(appConfig *conf.AppConfig, publicIps []string) (
              awsConfig aws.Config, ec2Client *ec2utils.Ec2Manger, err error) {
    stateFilePath := "../.kraken-state.yml"
    var stateConfig StateConfig
    var stateData []byte
    var yamlUpdates map[string]string

    // Set up the AWS credentials based on local chain or environment variables
    awsConfig, _, _, err = awsutils.AwsConfigSetup(appConfig.LocalConfig.Region,
                                                   1 * time.Minute)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // Check to see if the yaml state file exists
    exists, isDir, hasData, err := disk.PathExists(stateFilePath)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // If the yaml state file exists and has data
    if exists && !isDir && hasData {
        // Read the data from yaml state file
        stateData, err = os.ReadFile(stateFilePath)
        if err != nil {
            return awsConfig, ec2Client, err
        }

        // Decode raw bytes into StateConfig struct
        err = yaml.Unmarshal(stateData, &stateConfig)
        if err != nil {
            return awsConfig, ec2Client, err
        }
    }

    defer func() {
        // If there are no values in YAML file to be updated
        if len(yamlUpdates) == 0 {
            return
        }

        // Update the yaml values with values from passed in map
        newYaml, yerr := yamlutils.UpdateYAMLBytes(stateData, yamlUpdates)
        if yerr != nil {
            err = errors.Join(err, fmt.Errorf("updating yaml:  %w", yerr))
            return
        }

        // Overwrite the original yaml with the updated data
        werr := os.WriteFile(stateFilePath, newYaml, 0644)
        if werr != nil {
            err = errors.Join(err, fmt.Errorf("writing output yaml:  %w", werr))
        }
    }()

    // Establish client to EC2 service
    ec2Client = ec2utils.NewEc2Manager(awsConfig)

    // Check to see if the VPC exists, otherwise create one
    vpcId, err := ec2Client.VpcProvision(5 * time.Minute,
                                         stateConfig.VpcId,
                                         appConfig.LocalConfig.CidrBlock)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // If a VPC was created, add ID to yaml updates map
    if vpcId != "" {
        yamlUpdates["aws_env.vpc_id"] = vpcId
    // Otherwise use the one from YAML since it was found
    } else {
        vpcId = stateConfig.VpcId
    }

    // Check to see if IGW exists, otherwise create & attach one
    igwId, err := ec2Client.InternetGatewayProvision(1 * time.Minute,
                                                     stateConfig.IgwId,
                                                     vpcId)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // If a Internet Gateway was created, add ID to yaml updates map
    if igwId != "" {
        yamlUpdates["aws_env.igw_id"] = igwId
    // Otherwise use the one from YAML since it was found
    } else {
        igwId = stateConfig.IgwId
    }

    // Check to see if Elastic IP exists, otherwise create one
    eipId, err := ec2Client.ElasticIpProvision(1 * time.Minute,
                                               stateConfig.EipId)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // If a Elastic IP was created, add ID to yaml updates map
    if eipId != "" {
        yamlUpdates["aws_env.eip_id"] = eipId
    // Otherwise use the one from YAML since it was found
    } else {
        eipId = stateConfig.EipId
    }

    // Get the slice of availability zones based on region
    azs, err := ec2Client.FetchAvailableAZs(1 * time.Minute)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // Pick random AZ from slice of AZ names
    az := awsutils.PickAzRandom(azs)

    // Set up map for ensuring unique subnet allocation
    alloc := map[string]struct{}{}

    // Parse the prefix length from CIDR
    prefixLength, err := cidrutils.PrefixFromCidr(appConfig.LocalConfig.CidrBlock)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // Allocate first available subnet in CIDR block for public subnet
    pubCidr, err := cidrutils.AllocateNextSubnet(appConfig.LocalConfig.CidrBlock,
                                                 alloc, prefixLength + 1)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // Create public subnet if it does not exist
    pubSubnetId, err := ec2Client.SubnetProvision(1 * time.Minute,
                                                  stateConfig.PublicSubnetId,
                                                  vpcId, pubCidr, az, true)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // If a public subnet was created, add ID to yaml updates map
    if pubSubnetId != "" {
        yamlUpdates["aws_env.public_subnet_id"] = pubSubnetId
    // Otherwise use the one from YAML since it was found
    } else {
        pubSubnetId = stateConfig.PublicSubnetId
    }

    // Allocate next available subnet in CIDR block for private subnet
    privCidr, err := cidrutils.AllocateNextSubnet(appConfig.LocalConfig.CidrBlock,
                                                  alloc, prefixLength + 1)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // Create private subnet if it does not exist
    privSubnetId, err := ec2Client.SubnetProvision(1 * time.Minute,
                                                   stateConfig.PrivateSubnetId,
                                                   vpcId, privCidr, az, false)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // If a private subnet was created, add ID to yaml updates map
    if privSubnetId != "" {
        yamlUpdates["aws_env.private_subnet_id"] = privSubnetId
    // Otherwise use the one from YAML since it was found
    } else {
        privSubnetId = stateConfig.PrivateSubnetId
    }

    // Create NAT gateway in public subnet if it does not exist
    natGatewayId, err := ec2Client.NatGatewayProvision(5 * time.Minute,
                                                       stateConfig.NatGatewayId,
                                                       pubSubnetId, eipId,
                                                       "Kloud-Kraken-NAT-Gateway")
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // If a NAT Gateway was created, add ID to yaml updates map
    if natGatewayId != "" {
        yamlUpdates["aws_env.nat_gateway_id"] = natGatewayId
    // Otherwise use the one from YAML since it was found
    } else {
        natGatewayId = stateConfig.NatGatewayId
    }

    // Create route table for subnets to internet gateway if does not exist
    publicRouteId, err := ec2Client.RouteTableProvision(1 * time.Minute,
                                                        stateConfig.PublicRouteId,
                                                        vpcId, igwId, "",
                                                        pubSubnetId,
                                                        "Kloud-Kraken-Public-Route")
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // If the public route table was created, add ID to yaml updates map
    if publicRouteId != "" {
        yamlUpdates["aws_env.public_route_id"] = publicRouteId
    // Otherwise use the one from YAML since it was found
    } else {
        publicRouteId = stateConfig.PublicRouteId
    }

    // Create route table for subnets to NAT Gateway if does not exist
    privateRouteId, err := ec2Client.RouteTableProvision(1 * time.Minute,
                                                         stateConfig.PrivateRouteId,
                                                         vpcId, "", natGatewayId,
                                                         privSubnetId,
                                                         "Kloud-Kraken-Private-Route")
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // If the private route table was created, add ID to yaml updates map
    if privateRouteId != "" {
        yamlUpdates["aws_env.private_route_id"] = privateRouteId
    // Otherwise use the one from YAML since it was found
    } else {
        privateRouteId = stateConfig.PrivateRouteId
    }

    // Ensure public route tables are associated to subnet
    publicAssocId, err := ec2Client.RouteTableAssociationProvision(1 * time.Minute,
                                                                   stateConfig.PublicAssociationId,
                                                                   publicRouteId, pubSubnetId)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // If the public association occured, add ID to yaml updates map
    if publicAssocId != "" {
        yamlUpdates["aws_env.public_association_id"] = publicAssocId
    // Otherwise use the one from YAML since it was found
    } else {
        publicAssocId = stateConfig.PublicAssociationId
    }

    // Ensure private route tables are associated to subnet
    privateAssocId, err := ec2Client.RouteTableAssociationProvision(1 * time.Minute,
                                                                    stateConfig.PrivateAssociationId,
                                                                    privateRouteId, privSubnetId)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // If the private association occured, add ID to yaml updates map
    if privateAssocId != "" {
        yamlUpdates["aws_env.private_association_id"] = privateAssocId
    // Otherwise use the one from YAML since it was found
    } else {
        privateAssocId = stateConfig.PrivateAssociationId
    }



    // TODO: VPC network setup
    //     X Create VPC with CIDR block
    //     X Create and attach Internet Gateway (IGW) to the VPC
    //     X Allocate Elastic IP for NAT Gateway
    //     X Create public and private subnets within the VPC
    //     X Create NAT Gateway in public subnet (uses Elastic IP)
    //     X Create route table for public subnets: 0.0.0.0/0 → IGW
    //     X Create route tables for private subnets (per AZ): 0.0.0.0/0 → NAT Gateway
    //     X Associate **public** subnets to public route table
    //     X Associate **private** subnets to private route tables
    //     - Create security groups for EC2 and other services
    //     - Configure Network ACLs for granular subnet-level rules
    //     - Create VPC endpoints (e.g., S3, SSM) for private access
    //     - Create S3 bucket & add bucket policy restricting access to VPC/VPC endpoint
    //     - Enable VPC Flow Logs for traffic monitoring and auditing



    // Setup client to IAM service
    iamClient := iamutils.NewIamManager(awsConfig)

    // Generate the EC2 clients trust and permissions policy templates
    trustPolicy := clientTrustPolicyGen()
    permissionsPolicy := clientPermPolicyGen(stateConfig.BucketName,
                                             appConfig.ClientConfig.Region,
                                             appConfig.LocalConfig.AccountId,
                                             "/kloud-kraken/tls-cert", "Kloud-Kraken")
    // Create and apply the EC2 client role
    _, err = iamClient.IamRoleCreation(2 * time.Minute, "ClientRole",
                                       trustPolicy, "ClientPermissions",
                                       permissionsPolicy, true)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // Generate the servers trust and permissions policy templates
    trustPolicy = serverTrustPolicyGen(appConfig.LocalConfig.AccountId,
                                       appConfig.LocalConfig.IamUsername)
    permissionsPolicy = serverPermPolicyGen(appConfig.LocalConfig.Region,
                                            appConfig.LocalConfig.AccountId,
                                            "/kloud-kraken/tls-cert",
                                            stateConfig.BucketName,
                                            "ClientRole")
    // Create and apply role for local server permissions
    serverArn, err := iamClient.IamRoleCreation(2 * time.Minute, "ServerRole",
                                                trustPolicy, "ServerPermissions",
                                                permissionsPolicy, false)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "IAM server and client roles created"))

    // Set up client to Security Token Service
    stsClient := sts.NewFromConfig(awsConfig)
    // Format role ARN from created role
    roleArn := "arn:aws:iam::" + serverArn + ":role/ServerRole"
    // Create a provider that will call STS AssumeRole under the covers
    assumeProvider := stscreds.NewAssumeRoleProvider(stsClient, roleArn)

    // Create fresh AWS config from new STS provider
    awsConfig, err = config.LoadDefaultConfig(
        context.TODO(),
        config.WithRegion(appConfig.LocalConfig.Region),
        config.WithCredentialsProvider(aws.NewCredentialsCache(assumeProvider)),
    )
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // Setup client to SSM
    ssmClient := ssmutils.NewSsmManager(awsConfig)
    // Push the servers certificate PEM into SSM parameter store
    param, err := ssmClient.PutSsmParameter("/kloud-kraken/tls-cert",
                                            string(TlsMan.CertPemBlock),
                                            1 * time.Minute)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "TLS certificate uploaded to " +
                                   "SSM Parameter Store for client retrieval"))

    // Read the client binary into memory
    binData, err := os.ReadFile("./kloud-kraken-client")
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // Setup client to S3
    s3Client := s3utils.NewS3Manager(awsConfig)

    // Upload the client binary to S3 Bucket
    keyName, err := s3Client.PutS3Object(stateConfig.BucketName, "client",
                                         binData, 1 * time.Minute)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "Uploaded client binary to S3 bucket ",
                                   color.RadiantAmethyst, stateConfig.BucketName))

    // Generate user data script to set up client program in EC2
    userData, err := ec2UserDataGen(appConfig, &stateConfig, keyName, publicIps, param)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    // Re-setup new client to EC2 service with newly assumed role
    ec2Client = ec2utils.NewEc2Manager(awsConfig)
    // Create number of EC2 instances based on passed in data
    err = ec2Client.CreateEc2Instances(20 * time.Minute, []byte(userData),
                                       "ami-0eb94e3d16a6eea5f",
                                       appConfig.LocalConfig.InstanceType,
                                       appConfig.LocalConfig.NumberInstances,
                                       "ClientRole", "Kloud-Kraken-EC2-Client",
                                       appConfig.LocalConfig.SecurityGroupIds,
                                       appConfig.LocalConfig.SecurityGroups,
                                       stateConfig.PrivateSubnetId)
    if err != nil {
        return awsConfig, ec2Client, err
    }

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "EC2 instance creation completed"))

    return awsConfig, ec2Client, nil
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

    // Load the YAML data into AppConfig struct
    appConfig, err := conf.LoadConfig(configFilePath)
    if err != nil {
        log.Fatal("Error loading YAML data: ", err)
    }

    // Load the configuration from the YAML file
    return appConfig
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
    // Display the kloud kraken banner
    printBanner()

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Wordlist merging started, time varies " +
                                   "greatly depending on how much data"))

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

    fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                   display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "Wordlist merging process completed"))

    var awsConfig aws.Config
    var ec2Man *ec2utils.Ec2Manger
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

        // Call handler function that sets up AWS IAM user permissions,
        // transfers client binary via S3, set TLS certificate via SSM
        // parameter store, and launches EC2 instances
        awsConfig, ec2Man, err = awsSetup(appConfig, publicIps)
        if err != nil {
            log.Fatalf("Error with AWS setup:  %v", err)
        }

        defer func() {
            // Terminate the EC2 instances when processing is complete
            termOutput, err := ec2Man.TerminateEc2Instances(time.Minute * 10)
            if err != nil {
                log.Printf("Error terminating EC2 instances:  %v", err)
            }

            // Iterate through list of terminated instance ids
            for _, instance := range termOutput.TerminatingInstances {
                if logMan != nil {
                    logMan.LogMessage("Instance state for %s: %s -> %s\n",
                                      aws.ToString(instance.InstanceId),
                                      instance.PreviousState.Name,
                                      instance.CurrentState.Name)
                } else {
                    log.Println("Instance state for " + aws.ToString(instance.InstanceId) +
                                ": " + string(instance.PreviousState.Name) + " -> " +
                                string(instance.CurrentState.Name))
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
    logMan, err = kloudlogs.NewLoggerManager("local", appConfig.LocalConfig.LogPath,
                                             awsConfig, "Kloud-Kraken", false)
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
