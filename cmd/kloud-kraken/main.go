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
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/ngimb64/Kloud-Kraken/internal/color"
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
	"github.com/ngimb64/Kloud-Kraken/pkg/tui"
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
// - ipAddr:  The IP address of the remote client connected to the server
// - t:  The tui interface for displaying output
//
func handleTransfer(connection net.Conn, buffer []byte, waitGroup *sync.WaitGroup,
                    appConfig *conf.AppConfig, logMan *kloudlogs.LoggerManager,
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

    var port uint16
    // Receive bytes of port of client port to connect to for file transfer
    err = binary.Read(connection, binary.LittleEndian, &port)
    if err != nil {
        logMan.LogMessage("error", "Error receiving client listener port:  %v", err)
        return
    }

    // Strip the original port used for connection from address
    ipAddr = strings.Split(ipAddr, ":")[0]
    // Format remote address with IP and  received port for transfer
    remoteAddr := ipAddr + ":" + strconv.Itoa(int(port))

    // Make a connection to the remote brain server
    transferConn, err := tls.Dial("tcp", remoteAddr,
                                  tlsutils.NewClientTLSConfig(TlsMan.CaCertPool, ipAddr))
    if err != nil {
        logMan.LogMessage("fatal", "Error connecting to remote client for transfer:  %v", err)
        return
    }

    // Display the remote client connected for file transfer in left panel
    t.LeftPanelCh <- display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                            color.LightCyan, "!"), "",
                                        color.NeonAzure, "Connected remote client ",
                                        color.RadiantAmethyst, ipAddr,
                                        color.NeonAzure, " on port ",
                                        color.KrakenGlowGreen, strconv.Itoa(int(port)))

    // Display the file path to be transfered in right panel
    t.RightPanelCh <- display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                             color.LightCyan, "!"), "",
                                         color.RadiantAmethyst, filePath,
                                         color.NeonAzure, " to be transfered")

    logMan.LogMessage("info", "Connected remote client %s on port %d, %s to be transfered",
                      ipAddr, port, filePath)
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

        // Display the file path to be transfered in right panel
        t.RightPanelCh <- display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                                 color.LightCyan, "$"), "",
                                             color.RadiantAmethyst, filePath,
                                             color.NeonAzure, " transfer completed")
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
// - remoteAddr:  IP address to remote client that has connected
// - t:  The tui interface for displaying output
//
func handleConnection(connection net.Conn, waitGroup *sync.WaitGroup,
                      appConfig *conf.AppConfig, logMan *kloudlogs.LoggerManager,
                      remoteAddr string, t *tui.TUI) {
    // Close connection and decrement waitGroup counter on local exit
    defer waitGroup.Done()
    defer connection.Close()

    // Set buffer to receive client PEM certificate
    buffer := make([]byte, 2 * globals.KB)

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

    // Decrement the active connection count
    CurrentConnections.Add(-1)

    // Display the connection termination information in the left tui panel
    t.LeftPanelCh <- display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                            color.LightCyan, "-"), "",
                                        color.NeonAzure, "Connection closed for ",
                                        color.RadiantAmethyst, remoteAddr)

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
    defer tlsListener.Close()

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
                                            color.NeonAzure, "Connection accepted from ",
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
    time.Sleep(3 * time.Second)
}


// Takes passed in args and formats into user data generated for EC2 creation.
//
// @Parameters
// - appConf:  The configuration instance that stores program YAML data
// - keyName:  The name of the key of the S3 bucket
// - ipAddrs:  Slice of IP addresses to be formatted into CSV string
// - ssmParam:  The path where the certificate is stored in SSM param store
//
// @Returns
// - The generated EC2 user data with args formatted into it
// - Error if it occurs, otherwise nil on success
//
func ec2UserDataGen(appConf *conf.AppConfig, keyName string, ipAddrs []string,
                    ssmParam string) (string, error) {
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
set -euxo pipefail
exec > >(tee /var/log/user-data.log | logger -t user-data -s 2>/dev/console) 2>&1

# === NVMe RAID0 instance-store setup ===
mapfile -t DEVICES < <(lsblk -d -n -o NAME,TYPE |
    awk '$2=="disk" && $1 ~ /^nvme[0-9]+n1$/ {print "/dev/" $1}')
if (( ${#DEVICES[@]} == 0 )); then
    echo "ERROR: no NVMe instance‐store devices found"
    shutdown -h now
    exit 1
fi

retries=0
until DEBIAN_FRONTEND=noninteractive apt-get update && apt-get install -y mdadm; do
    ((retries++))
    (( retries>=3 )) && { echo "ERROR: apt-get install failed"; shutdown -h now; exit 1; }
    sleep 5
done

if ! mdadm --detail /dev/md0 &>/dev/null; then
    yes | mdadm --create /dev/md0 --level=0 --raid-devices=${#DEVICES[@]} "${DEVICES[@]}"
fi

mdadm --detail --scan | tee /etc/mdadm/mdadm.conf
update-initramfs -u

if ! blkid /dev/md0 &>/dev/null; then
    mkfs.ext4 -F /dev/md0
fi

mkdir -p /mnt/instance-store
grep -q '/mnt/instance-store' /etc/fstab || \
    echo "/dev/md0  /mnt/instance-store  ext4  defaults,nofail  0 2" >> /etc/fstab
mountpoint -q /mnt/instance-store || mount /mnt/instance-store

echo "✓ Instance-store ready at /mnt/instance-store"

# === Application bootstrap ===
apt update && apt upgrade -y && apt install -y hashcat

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
`, appConf.LocalConfig.BucketName, keyName,
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
// - region:  The AWS region where actions will be performed
// - accountId:  The AWS account ID where actions will be performed
// - ssmParam:  The path where the certificate is stored in SSM param store
// - bucketName:  The name of the S3 bucket where actions will be performed
// - clientRoleName:  The name of IAM role the client will be using
//
// @Returns
// - The generated permissions policy with args formatted into it
//
func serverPermPolicyGen(region string, accountId string, ssmParam string,
                         bucketName string, clientRoleName string) string {
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
// - accountId:  The AWS account ID where actions will be performed
// - iamUser:  The IAM user that the policy will apply to
//
// @Returns
// - The generated trust policy with args formatted into it
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
// - bucketName:  The name of the S3 bucket where actions will be performed
// - region:  The AWS region where actions will be performed
// - accountId:  The AWS account ID where actions will be performed
// - paramPath:  The path where the certificate is stored in SSM param store
// - logGroup:  The name of the CloudWatch group being utilized
//
// @Returns
// - The generated permissions policy with args formatted into it
//
func clientPermPolicyGen(bucketName string, region string, accountId string,
                         paramPath string, logGroup string) string {
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
// - The generated trust policy with args formatted into it
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


// Sets up AWS credentials, uses IAM permissions in the credentials to set up
// client and server roles in IAM. Then assumes created server role via STS
// service. Puts generated TLS certificate in SSM parameter store and client
// binary in S3 bucket for later retrieval. Concludes by launching EC2 instances.
//
// @Parameters
// - appConfig:  The configuration instance with program YAML data
// - publicIps:  List of public IPs to format into user data template
//
// @Returns
// - The initialized AWS configuration instance
// - The EC2 manager instance to utilize for later operations
// - Error if it occurs, otherwise nil on success
//
func awsSetup(appConfig *conf.AppConfig, publicIps []string) (
              aws.Config, *awsutils.Ec2Manger, error) {
    var ec2Man *awsutils.Ec2Manger
    // Set up the AWS credentials based on local chain or environment variables
    awsConfig, _, _, err := awsutils.AwsConfigSetup(appConfig.LocalConfig.Region,
                                                    1 * time.Minute)
    if err != nil {
        return awsConfig, ec2Man, err
    }

    // Setup client to IAM service
    iamClient := iam.NewFromConfig(awsConfig)

    // Generate the EC2 clients trust and permissions policy templates
    trustPolicy := clientTrustPolicyGen()
    permissionsPolicy := clientPermPolicyGen(appConfig.LocalConfig.BucketName,
                                             appConfig.ClientConfig.Region,
                                             appConfig.LocalConfig.AccountId,
                                             "/kloud-kraken/tls/cert", "Kloud-Kraken")
    // Create and apply the EC2 client role
    _, err = awsutils.IamRoleCreation(iamClient, 2 * time.Minute, "ClientRole",
                                      trustPolicy, "ClientPermissions",
                                      permissionsPolicy, true)
    if err != nil {
        return awsConfig, ec2Man, err
    }

    // Generate the servers trust and permissions policy templates
    trustPolicy = serverTrustPolicyGen(appConfig.LocalConfig.AccountId,
                                       appConfig.LocalConfig.IamUsername)
    permissionsPolicy = serverPermPolicyGen(appConfig.LocalConfig.Region,
                                            appConfig.LocalConfig.AccountId,
                                            "/kloud-kraken/tls/cert",
                                            appConfig.LocalConfig.BucketName,
                                            "ClientRole")
    // Create and apply role for local server permissions
    serverArn, err := awsutils.IamRoleCreation(iamClient, 2 * time.Minute, "ServerRole",
                                               trustPolicy, "ServerPermissions",
                                               permissionsPolicy, false)
    if err != nil {
        return awsConfig, ec2Man, err
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
        return awsConfig, ec2Man, err
    }

    // Establish client to SSM
    ssmMan := awsutils.NewSsmManager(awsConfig)
    // Push the servers certificate PEM into SSM parameter store
    param, err := ssmMan.PutSsmParameter("/kloud-kraken/tls/cert",
                                         string(TlsMan.CertPemBlock),
                                         1 * time.Minute)
    if err != nil {
        return awsConfig, ec2Man, err
    }

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "TLS certificate uploaded to " +
                                   "SSM Parameter Store for client retrieval"))

    // Establish client to S3
    s3Man := awsutils.NewS3Manager(awsConfig)
    // Check to see if S3 bucket exists
    exists, err := s3Man.BucketExists(appConfig.LocalConfig.BucketName, 1 * time.Minute)
    if err != nil {
        return awsConfig, ec2Man, err
    }

    // If S3 bucket does not exist create one
    if !exists {
        err = s3Man.CreateBucket(appConfig.LocalConfig.BucketName, 1 * time.Minute)
        if err != nil {
            return awsConfig, ec2Man, err
        }

        fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "Created S3 bucket ",
                                       color.RadiantAmethyst, appConfig.LocalConfig.BucketName))
    }

    // Read the client binary into memory
    binData, err := os.ReadFile("./client")
    if err != nil {
        return awsConfig, ec2Man, err
    }

    // Upload the client binary to S3 Bucket
    keyName, err := s3Man.PutS3Object(appConfig.LocalConfig.BucketName, "client",
                                      binData, 1 * time.Minute)
    if err != nil {
        return awsConfig, ec2Man, err
    }

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "Uploaded client binary to S3 bucket ",
                                   color.RadiantAmethyst, appConfig.LocalConfig.BucketName))

    // Generate user data script to set up client program in EC2
    userData, err := ec2UserDataGen(appConfig, keyName, publicIps, param)
    if err != nil {
        return awsConfig, ec2Man, err
    }

    // Setup EC2 creation instance with populated args
    ec2Man = awsutils.NewEc2Manager("ami-0eb94e3d16a6eea5f", awsConfig,
                                    appConfig.LocalConfig.NumberInstances,
                                    appConfig.LocalConfig.InstanceType,
                                    "Kloud-Kraken", "ClientRole",
                                    appConfig.LocalConfig.SecurityGroupIds,
                                    appConfig.LocalConfig.SecurityGroups,
                                    appConfig.LocalConfig.SubnetId,
                                    []byte(userData))
    // Create number of EC2 instances based on passed in data
    err = ec2Man.CreateEc2Instances(20 * time.Minute)
    if err != nil {
        return awsConfig, ec2Man, err
    }

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "EC2 instance creation completed"))

    return awsConfig, ec2Man, nil
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

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Wordlist merging started, this could " +
                                   "take time depending on the amount of data being processed"))

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

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "Wordlist merging completed"))

    var awsConfig aws.Config
    var ec2Man *awsutils.Ec2Manger
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
    time.Sleep(4 * time.Second)

    // Listen for incoming client connections and handle them
    startServer(appConfig, logMan)

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "All connections handled " +
                                   ".. server shutting down"))

    logMan.LogMessage("info", "All connections handled .. server shutting down")
}
