package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/ngimb64/Kloud-Kraken/internal/color"
	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/internal/validate"
	"github.com/ngimb64/Kloud-Kraken/internal/vpcsetup"
	"github.com/ngimb64/Kloud-Kraken/pkg/awscost"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"github.com/ngimb64/Kloud-Kraken/pkg/display"
	"github.com/ngimb64/Kloud-Kraken/pkg/ec2utils"
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
var AssumedAwsConfig aws.Config      // Assumed server AWS config
var Ec2Client *ec2utils.Ec2Manger    // Client manager for EC2
var Ec2SecurityGroupId string        // Security group ID for EC2
var ParamsToDelete []string          // SSM parameters to delete at end of program
var ReceivedDir = "/tmp/received"    // Path where cracked hashes & client logs are stored
var SsmMan *ssmutils.SsmManager      // Client manager for SSM
var TlsMan = &tlsutils.TlsManager{}  // Struct for managing TLS certs, keys, etc.


// Select next available file for transfer, if there are no more available send
// the end transfer message to client. Format the transfer reply with the file
// name and size, get the IP address of the current connection and read the port
// from the socket to format the dialer for the new connection for file transfer.
// Finally pass the connection with other args goroutine to transfer file.
//
// @Parameters
//  - connection:  Network socket connection for handling messaging
//  - readBuffer:  The buffer storing read data from connection
//  - transferWaitGroup:  Used to sync Goroutines running file transfers
//  - appConfig:  The configuration struct with loaded yaml program data
//  - logMan:  The kloudlogs logger manager for local logging
//  - ipAddr:  The IP address of the remote client connected to the server
//  - t:  The tui interface for displaying output
//
func handleTransfer(connection net.Conn, readBuffer []byte,
                    transferWaitGroup *sync.WaitGroup,
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

    // Parse the TCP port to connect to from the transfer request
    port, err := netio.ParseTransferRequest(readBuffer,
                                            globals.TRANSFER_REQUEST_PREFIX,
                                            len(readBuffer))
    if err != nil {
        logMan.LogMessage("error", "Error parsing port from transfer request:  %v", err)
        return
    }

    // Format transfer reply to inform client of selected file name and size
    buffer, err := netio.FormatStartTransfer(filePath, fileSize,
                                             globals.MESSAGE_BUFFER_SIZE,
                                             globals.START_TRANSFER_PREFIX)
    if err != nil {
        logMan.LogMessage("error", "Error formatting transfer reply:  %v", err)
        return
    }

    // Send start transfer message with file name and size
    _, err = netio.WriteHandler(connection, buffer, len(buffer))
    if err != nil {
        logMan.LogMessage("error", "Error sending the transfer reply:  %v", err)
        return
    }

    port32 := int32(port)

    revokeSecurityGroup := func() {
        // Remove rule from security group that allows
        // outbound port to connect to server
        err = Ec2Client.RevokeSecurityGroupRule(1 * time.Minute,
                                                Ec2SecurityGroupId,
                                                "tcp", "0.0.0.0/0",
                                                "ingress", port32, port32)
        if err != nil {
            logMan.LogMessage("Error", "Error revoking EC2 security group",
                              zap.Int32("Port", port32))
        }
    }

    // Add rule to security group to allow outbound port to connect to server
    err = Ec2Client.SecurityGroupRuleProvision(1 * time.Minute, Ec2SecurityGroupId, "0.0.0.0/0",
                                                "tcp", "ingress", port32, port32)
    if err != nil {
        logMan.LogMessage("Error", "Error provisioning security group rule for file transfer",
                          zap.Int32("Port", port32))
        return
    }

    // Format client ip:port to connect to
    clientAddr := net.JoinHostPort(ipAddr, strconv.Itoa(port))

    maxRetries := 3
    tlsConfig := tlsutils.NewClientTLSConfig(TlsMan.CertPool, ipAddr)
    var transferConn *tls.Conn

    for range 3 {
        // Make a connection to the client for file transfer
        transferConn, err = tls.Dial("tcp", clientAddr, tlsConfig)
        if err == nil {
            break
        }

        logMan.LogMessage("error", "Error connecting to remote client for transfer:  %v", err)
        // Sleep for a bit and try again on failure
        time.Sleep(5 * time.Second)
        maxRetries -= 1
    }

    if maxRetries == 0 {
        logMan.LogMessage("error", "Max connection attempt failures exhausted")
        // Remove added security group rule
        revokeSecurityGroup()
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

    // Increment waitgroup counter
    transferWaitGroup.Add(1)

    go func() {
        var err error

        defer func() {
            // Close the transfer connection
            cerr := transferConn.Close()
            if cerr != nil {
                logMan.LogMessage("Error", "Error closing transfer connection %d:  %v",
                                  port, cerr)
            }

            // Remove added security group rule
            revokeSecurityGroup()
            // Decrement waitgroup counter
            transferWaitGroup.Done()
        }()

        // Transfer the file to client
        err = netio.TransferFile(transferConn, filePath, fileSize)
        if err != nil {
            logMan.LogMessage("error", "Error occured transfering file to client %s:  %v",
                              ipAddr, err)
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
//  - appConfig:  The configuration struct with loaded yaml program data
//  - logMan:  The kloudlogs logger manager for local logging
//  - mainWaitGroup:  Wait group for main server goroutines
//  - remoteAddr:  IP address to remote client that has connected
//  - t:  The tui interface for displaying output
//
func handleConnection(connection net.Conn,
                      appConfig *conf.AppConfig,
                      logMan *kloudlogs.LoggerManager,
                      mainWaitGroup *sync.WaitGroup,
                      remoteAddr string, t *tui.TUI) {
    var err error

    defer func() {
        // Close connection to remote server
        err = connection.Close()
        if err != nil {
            logMan.LogMessage("error", "Error closing client connection:  %v", err)
        }

        // Display the connection termination information in the left tui panel
        t.LeftPanelCh <- display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                                color.LightCyan, "-"), "",
                                            color.NeonAzure, "Connection closed for ",
                                            color.RadiantAmethyst, remoteAddr)
        // Decrement waitGroup counter
        mainWaitGroup.Done()
    } ()

    defer func () {
        // Receive log file from client
        _, err = netio.ReceiveFile(connection, globals.MESSAGE_BUFFER_SIZE,
                                   ReceivedDir, globals.LOG_TRANSFER_PREFIX)
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

    // Upload the hash file to connection client
    err = netio.UploadFile(connection, globals.MESSAGE_BUFFER_SIZE,
                           appConfig.LocalConfig.HashFilePath,
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
        err = netio.UploadFile(connection, globals.MESSAGE_BUFFER_SIZE,
                               appConfig.LocalConfig.RulesetPath,
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

    var transferWaitGroup sync.WaitGroup

    for {
        // Read data from connected client
        readBuffer, err := netio.ReadHandler(connection,
                                             globals.MESSAGE_BUFFER_SIZE,
                                             []byte(">"))
        if err != nil {
            logMan.LogMessage("error", "Error reading data from socket:  %v", err)
            return
        }

        // If the read data contains the processing complete message
        if bytes.Contains(readBuffer, globals.PROCESSING_COMPLETE) {
            break
        }

        // If the read data contains transfer request message
        if bytes.Contains(readBuffer, globals.TRANSFER_REQUEST_PREFIX) {
            // Call method to handle file transfer based
            handleTransfer(connection, readBuffer, &transferWaitGroup,
                           appConfig, logMan, remoteAddr, t)
        }
    }

    // Wait for any file transfer goroutines to complete
    transferWaitGroup.Wait()

    // Receive cracked user hash file from client
    _, err = netio.ReceiveFile(connection, globals.MESSAGE_BUFFER_SIZE,
                               ReceivedDir, globals.LOOT_TRANSFER_PREFIX)
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
    var mainWaitGroup sync.WaitGroup

    // Get the EC2 public IPs based on instance IDs from run output
    ec2Ips, err := Ec2Client.Ec2GetPublicIps(1 * time.Minute, nil)
    if err != nil {
        logMan.LogMessage("fatal", "Error retreiving run output public IPs:  %v", err)
    }

    // Establish client to SSM
    SsmMan = ssmutils.SsmNewManager(AssumedAwsConfig)

    // Setup TUI interface for and ensure it closes on local exit
    t := tui.NewTUI(100, "Connections", 500 * time.Millisecond, 3, "File Transfers")
    go t.Start(color.SkyBlue, color.BrightMagenta, color.BrightMint)
    defer t.Stop()

    // Iterate through instances in result from SDK call
    for _, ipAddr := range ec2Ips {
        // If there is no public IP address, skip it
        if ipAddr == "" {
            continue
        }

        // Get instance ID based on its public IP
        instanceId, err :=  Ec2Client.Ec2GetInstanceIdByPublicIp(1 * time.Minute, ipAddr)
        if err != nil {
            logMan.LogMessage("error", "Error getting EC2 instance ID by public IP:  %v", err)
            continue
        }

        // Format SSM parameter and add to list to delete at end of program
        param := "/kloud-kraken/" + instanceId + "/tls-cert"
        ParamsToDelete = append(ParamsToDelete, param)

        // Retrieve the server TLS cert from SSM param store
        certPemString, err := SsmMan.SsmGetParameter(1 * time.Minute, param)
        if err != nil {
            logMan.LogMessage("error", "Error getting server TLS cert via SSM Param Store:  %v", err)
            continue
        }

        // Add client certificate to cert pool
        err = TlsMan.AddCertToPool([]byte(certPemString))
        if err != nil {
            logMan.LogMessage("error", "Error adding TLS certificate to pool:  %v", err)
            continue
        }

        // Define the address of the server to connect to
        serverAddress := net.JoinHostPort(ipAddr, strconv.Itoa(appConfig.LocalConfig.ListenerPort))

        // Make a connection to the remote server
        connection, err := tls.Dial("tcp", serverAddress,
                                    tlsutils.NewClientTLSConfig(TlsMan.CertPool, ipAddr))
        if err != nil {
            logMan.LogMessage("error", "Error connecting to remote client:  %v", err)
            continue
        }

        // Display connection attempt in the left panel
        t.LeftPanelCh <- display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                                color.LightCyan, "!"), "",
                                            color.NeonAzure, "Connected to ",
                                            color.RadiantAmethyst, serverAddress)

        // Increment wait group and handle connection in separate Goroutine
        mainWaitGroup.Add(1)
        go handleConnection(connection, appConfig, logMan,
                            &mainWaitGroup, ipAddr, t)
    }

    // Wait for all active Goroutines to finish before shutting down server
    mainWaitGroup.Wait()
    // Sleep a few seconds so information is displayed before tui is stopped
    time.Sleep(5 * time.Second)
}


// Takes passed in args and formats into user data generated for EC2 creation.
//
// @Parameters
//  - appConf:  The configuration instance that stores program YAML data
//  - bucketName:  Name of S3 bucket where client binary is stored
//  - keyName:  The name of the key of the S3 bucket
//  - ec2SgId:  The ID for the security group for client EC2 instances
//
// @Returns
//  - The generated EC2 user data with args formatted into it
//  - Error if it occurs, otherwise nil on success
//
func ec2UserDataGen(appConf *conf.AppConfig, bucketName string,
                    keyName string, ec2SgId string) (
                    string, error) {
    var hasRuleset bool

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

ROOT_PART=$(findmnt -no SOURCE /)
ROOT_DISK=$(lsblk -no PKNAME "$ROOT_PART")

# Get the NVMe instance store device names
mapfile -t DEVICES < <(lsblk -d -n -o NAME,TYPE |
    awk -v root="$ROOT_DISK" '$2=="disk" && $1 ~ /^nvme/ && $1 != root {print "/dev/" $1}')

# If no NVMe instance store drives are found, log error and exit
if (( ${#DEVICES[@]} < 1 )); then
    echo "ERROR: no NVMe instance-store devices found"
    exit 1
fi

# Use the first NVMe instance store device
STORAGE_DEVICE="${DEVICES[0]}"
# Detect if the device is already mounted
EXISTING_MOUNT=$(lsblk -no MOUNTPOINT "$STORAGE_DEVICE" | tr -d ' ')

if [[ -n "$EXISTING_MOUNT" ]]; then
    echo "Instance store already mounted at $EXISTING_MOUNT"
    INSTANCE_STORE_PATH="$EXISTING_MOUNT"
else
    echo "Instance store not mounted yet — mounting to /mnt/instance-store"
    mkdir -p /mnt/instance-store

    # Only create filesystem if device truly has none
    if ! blkid "$STORAGE_DEVICE" &>/dev/null; then
        mkfs.ext4 -F "$STORAGE_DEVICE"
    fi

    mount "$STORAGE_DEVICE" /mnt/instance-store
    INSTANCE_STORE_PATH="/mnt/instance-store"
fi

# Quick test to ensure instance role credentials are available
if ! aws sts get-caller-identity --output text >/dev/null 2>&1; then
    echo "ERROR: instance profile credentials unavailable (check IAM role perms)"
    exit 2
fi

# Environment variables for non-interactive installs
export DEBIAN_FRONTEND=noninteractive
export NEEDRESTART_S=no                # Prevent "services need restart" prompts
export APT_LISTCHANGES_FRONTEND=none   # Suppress changelog prompts

# Hashcat installation
apt install -qy \
  -o Dpkg::Options::="--force-confdef" \
  -o Dpkg::Options::="--force-confold" \
  hashcat

# Ensure nvidia-smi output ensures GPU is working
if ! DRIVER_ERR=$(nvidia-smi 2>&1); then
    printf "ERROR: nvidia-smi - %%s\n" "$DRIVER_ERR"
fi

DIR=/opt
# Ensure /opt directory exists
mkdir -p $DIR
# Copy binary to instance from S3 and set executable permissions
aws s3 cp s3://%s/%s $DIR/client --region %s --no-progress
test -x $DIR/client || chmod +x $DIR/client

# Create run script to launch client
cat > $DIR/run-client.sh <<-__EOF__
#!/bin/bash
set -euo pipefail

exec $DIR/client \
    -applyOptimization=%t \
    -awsRegion="%s" \
    -charSet1="%s" \
    -charSet2="%s" \
    -charSet3="%s" \
    -charSet4="%s" \
    -crackingMode="%s" \
    -dataPath="$INSTANCE_STORE_PATH" \
    -ec2SecurityGroupId="%s" \
    -hashMask="%s" \
    -hasRuleset=%t \
    -hashType="%s" \
    -logMode="%s" \
    -maxFileSizeInt64=%d \
    -maxTransfers=%d \
    -port=%d \
    -workload="%s"
__EOF__
chmod +x $DIR/run-client.sh

# Create systemd unit file that runs script to launch client
cat > /etc/systemd/system/client.service <<-__EOF__
[Unit]
Description=Kloud Kraken Client
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$DIR/run-client.sh
TimeoutStartSec=360
Restart=on-failure
RestartSec=10
StandardOutput=file:/var/log/client.log
StandardError=file:/var/log/client.log

[Install]
WantedBy=multi-user.target
__EOF__

# Set up the client service to execute
systemctl daemon-reload
systemctl enable --now client.service
`, bucketName, keyName, "us-east-1", true, "us-east-1",
   appConf.ClientConfig.CharSet1, appConf.ClientConfig.CharSet2,
   appConf.ClientConfig.CharSet3, appConf.ClientConfig.CharSet4,
   appConf.ClientConfig.CrackingMode, ec2SgId, appConf.ClientConfig.HashMask,
   hasRuleset, appConf.ClientConfig.HashType, appConf.ClientConfig.LogMode,
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
//
// @Returns
//  - The initialized AWS configuration instance
//  - The VPC bootstrap output struct
//  - AWS cost manager struct
//  - Errors associated with cost manager
//  - Error if it occurs, otherwise nil on success
//
func awsSetup(appConfig *conf.AppConfig) (aws.Config, *vpcsetup.VpcBootstrapOutput,
                                          *awscost.AwsCostManager, error, error) {
    // Set up AWS credentials based on local chain or environment variables
    baseAwsConfig, err := awsutils.AwsConfigSetup(1 * time.Minute, "us-east-1",
                                                  "kloud-kraken")
    if err != nil {
        return baseAwsConfig, nil, nil, nil, err
    }

    // Get human readable location string based off region for cost calculation
    location, exists := awsutils.RegionToLocation(baseAwsConfig.Region)
    if !exists {
        return baseAwsConfig, nil, nil, nil, err
    }

    // Establish client to Security Token Service
    stsClient := sts.NewFromConfig(baseAwsConfig)

    // Set up Kloud Kraken VPC and its associated components
    bootstrapOut, costMan, costErr, err := vpcsetup.VpcBootstrap(appConfig, baseAwsConfig,
                                                                 location, *stsClient)
    if err != nil {
        return baseAwsConfig, bootstrapOut, costMan, costErr, err
    }

    Ec2SecurityGroupId = bootstrapOut.Ec2SgId

    // Create a provider that will call STS AssumeRole under the covers
    assumeProvider := stscreds.NewAssumeRoleProvider(stsClient, bootstrapOut.ServerArn)
    // Make a shallow copy of base AWS config instance
    AssumedAwsConfig = baseAwsConfig.Copy()
    // Swap in credentials of assumed role
    AssumedAwsConfig.Credentials = aws.NewCredentialsCache(assumeProvider)

    // Ensure STS token is refreshed per execution
    _, err = AssumedAwsConfig.Credentials.Retrieve(context.Background())
    if err != nil {
        // Sleep for a bit and try again
        time.Sleep(5 * time.Second)
        _, err = AssumedAwsConfig.Credentials.Retrieve(context.Background())
    }

    if err != nil {
        return AssumedAwsConfig, bootstrapOut, costMan, costErr, err
    }

    // Read the client binary into memory
    binData, err := os.ReadFile(globals.BIN_DIR + "/kloud-kraken-client")
    if err != nil {
        return AssumedAwsConfig, bootstrapOut, costMan, costErr, err
    }

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Uploading client binary to S3 bucket"))

    // Re-establish client to S3 with new API key set
    s3Client := s3utils.S3NewManager(AssumedAwsConfig)
    // Upload the client binary to S3 Bucket
    keyName, err := s3Client.S3PutObject(5 * time.Minute,
                                         bootstrapOut.S3BucketName,
                                         "client", binData)
    if err != nil {
        return AssumedAwsConfig, bootstrapOut, costMan, costErr, err
    }

    bootstrapOut.S3Client = s3Client

    fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                   display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "Uploaded client binary to S3 bucket ",
                                   color.RadiantAmethyst, bootstrapOut.S3BucketName))

    // Generate user data script to set up client program in EC2
    userData, err := ec2UserDataGen(appConfig, bootstrapOut.S3BucketName,
                                    keyName, bootstrapOut.Ec2SgId)
    if err != nil {
        return AssumedAwsConfig, bootstrapOut, costMan, costErr, err
    }

    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-ec2-client",
    }

    // Re-setup new client to EC2 service with newly assumed role
    Ec2Client = ec2utils.Ec2NewManager(AssumedAwsConfig)
    bootstrapOut.Ec2Client = Ec2Client
    // Get the latest AMI for the ubuntu deep learning
    amiId, err := Ec2Client.Ec2GetAmiId(1 * time.Minute, "x86_64", "Deep Learning " +
                                        "Base AMI with Single CUDA (Ubuntu 22.04) *",
                                        []string{"amazon"})
    if err != nil {
        return AssumedAwsConfig, bootstrapOut, costMan, costErr, err
    }

	// Always set number of instances to 1 until distributed is further tested
	appConfig.LocalConfig.NumberInstances = 1

    ec2CreateInstancesInput := &ec2utils.Ec2CreateInstancesInput{
        AMI:              amiId,
        InstanceType:     appConfig.LocalConfig.InstanceType,
        MaxCount:         appConfig.LocalConfig.NumberInstances,
        MinCount:         appConfig.LocalConfig.NumberInstances,
        RoleName:         "KloudKrakenClientRole",
        SecurityGroupIds: []string{bootstrapOut.Ec2SgId},
        SubnetId:         bootstrapOut.SubnetId,
        Tags:             tags,
        UserData:         []byte(userData),
    }

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Creating EC2 instance(s)"))

    // Create number of EC2 instances based on passed in data
    err = Ec2Client.Ec2CreateInstances(5 * time.Minute, ec2CreateInstancesInput)
    if err != nil {
        return AssumedAwsConfig, bootstrapOut, costMan, costErr, err
    }

    filterMap := map[string]string{
        "capacitystatus":  "Used",
        "instanceType":    appConfig.LocalConfig.InstanceType,
        "location":        location,
        "operatingSystem": "Linux",
        "preInstalledSw":  "NA",
        "tenancy":         "Shared",
    }

    // Add the Ec2 instances to the cost manager
    _ = costMan.AddCostResourceToManager("ec2_instance", filterMap, true, &costErr)

    fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                   display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "EC2 instance creation completed"))

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Generating x509 certificate pool"))

    // Generate a TLS x509 certificate and cert pool
    err = TlsMan.CertPoolGen()
    if err != nil {
        log.Fatalf("Error generating TLS certificate:  %v", err)
    }

    fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                   display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "X509 cerificate pool generated"))

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Waiting for status OK to " +
                                   "connect to the EC2 instances"))

    // Wait until the EC2 instance status is OK
    err = Ec2Client.Ec2WaiterStatusOk(15 * time.Minute)
    if err != nil {
        return AssumedAwsConfig, bootstrapOut, costMan, costErr, err
    }

    return AssumedAwsConfig, bootstrapOut, costMan, costErr, nil
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


// Set important paths for project to be globally accessable.
//
func setProjectPaths() {
    globals.ROOT_DIR = disk.GetProjectRootDir()
    globals.BIN_DIR = globals.ROOT_DIR + "/bin"
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
    defer func() {
        fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "Total runtime:  ",
                                       color.KrakenGlowGreen,
                                       time.Since(startTime).String()))
    }()

    // Handle selecting the YAML file if no arg provided
    // and load YAML data into struct configuration class
    appConfig := parseArgs()
    // Set paths to project dirs
    setProjectPaths()
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

    var logMan *kloudlogs.LoggerManager

    // Call handler function that sets up AWS IAM user permissions,
    // transfers client binary via S3, set TLS certificate via SSM
    // parameter store, and launches EC2 instances
    awsConfig, bootstrapOut, costMan, costErr, err := awsSetup(appConfig)
    // If error occured during AWS resource cost calculation
    if costErr != nil {
        defer func() {
            if logMan != nil {
                logMan.LogMessage("error", "Error occured during AWS cost" +
                                    " calulcation:  %v", costErr)
            } else {
                log.Printf("Error occured during AWS cost calulcation:  %v", costErr)
            }
        }()
    }

    if err != nil {
        log.Fatalf("Error with AWS setup:  %v", err)
    }

    // AWS cost optimization resource deletion cleanup routine
    defer func() {
        var stateConfig vpcsetup.AwsEnv
        var stateData []byte
        stateFilePath := globals.ROOT_DIR + "/.kraken-state.yml"
        var yamlUpdates = map[string]string{}

        // Read the data from yaml state file
        stateData, err = os.ReadFile(stateFilePath)
        if err != nil {
            log.Printf("Error reading state file for cleanup:  %v", err)
        }

        // Decode raw bytes into StateConfig struct
        err = yaml.Unmarshal(stateData, &stateConfig)
        if err != nil {
            log.Printf("Error unmarshaling state file YAML data into " +
                        "state struct:  %v", err)
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

        fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "!"), "",
                                       color.NeonAzure, "Deleting EC2 instances"))

        // Terminate the EC2 instances
        termOutput, err := bootstrapOut.Ec2Client.Ec2TerminateInstances(10 * time.Minute)
        if err != nil {
            log.Printf("Error terminating EC2 instances:  %v", err)
        }

        // Iterate through list of terminated instance ids and log them
        for _, instance := range termOutput.TerminatingInstances {
            if logMan != nil {
                logMan.LogMessage("info", "Instance state for %s: %s -> %s\n",
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

        fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "!"), "",
                                       color.NeonAzure, "Deleting messaging port security group rule"))

        // Revoke the security group rule for listener port set by client
        err = bootstrapOut.Ec2Client.RevokeSecurityGroupRule(1 * time.Minute, bootstrapOut.Ec2SgId,
                                                             "tcp", "0.0.0.0/0", "ingress",
                                                             int32(appConfig.LocalConfig.ListenerPort),
                                                             int32(appConfig.LocalConfig.ListenerPort))
        if err != nil {
            if logMan != nil {
                logMan.LogMessage( "error", "Error revoking security group rule:  %v", err)
            } else {
                log.Printf("Error revoking security group rule:  %v", err)
            }
        }

        fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "!"), "",
                                       color.NeonAzure, "Deleting VPC Endpoints"))

        // Terminate SSM Parameter Store VPC Interface Endpoint
        err = bootstrapOut.Ec2Client.VpcEndpointsTerminator(1 * time.Minute,
                                                            []string{bootstrapOut.SsmVpcEndpointId})
        if err != nil {
            if logMan != nil {
                logMan.LogMessage("error", "Error deleting SSM Parameter Store" +
                                  " VPC Interface Endpoint:  %v", err)
            } else {
                log.Printf("Error deleting SSM Parameter Store VPC" +
                           " Interface Endpoint:  %v", err)
            }
        } else {
            yamlUpdates["aws_env.ssm_vpc_endpoint_id"] = ""
        }

        fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "!"), "",
                                       color.NeonAzure, "Emptying and deleting S3 bucket"))

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

        fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "!"), "",
                                       color.NeonAzure, "Deleting TLS certificates" +
                                       " from SSM Param Store"))

        // Iterate through SSM parameters and delete them
        for _, param := range ParamsToDelete {
            // Delete all the client TLS certificate from SSM Parameter store
            err = SsmMan.SsmDeleteParameter(1 * time.Minute, param)
            if err != nil {
                if logMan != nil {
                    logMan.LogMessage("error", "Error deleting parameters from" +
                                      " SSM Param Store:  %v", err)
                } else {
                    log.Printf("Error deleting parameters from SSM Param Store:  %v", err)
                }
            }
        }

        // Calculate the total cost of AWS resources
        err = costMan.CalculateTotalCost()
        if err != nil {
            if logMan != nil {
                logMan.LogMessage("error", "Error calculating total AWS " +
                                  "resource cost:  %v", err)
            } else {
                log.Printf("Error calculating total AWS resource cost:  %v", err)
            }
        }

        // Visible column widths
        const serviceColWidth = 24
        const costColWidth = 10

        // Print table header
        header := display.Ctext(color.LightCyan, "\n        AWS Cost Table\n")
        fmt.Print(header)
        // Dynamic separator width (| + service + | + cost + |)
        totalWidth := 1 + serviceColWidth + 1 + costColWidth + 1
        fmt.Println(strings.Repeat("-", totalWidth))

        // column titles
        leftSep := display.Ctext(color.KrakenPurple, "|")
        serviceTitle := display.PadRightColored(display.Ctext(color.NeonAzure, "Service Name"),
                                                serviceColWidth)
        costTitle := display.PadLeftColored(display.Ctext(color.BrightLime, "Cost"), costColWidth)

        fmt.Printf("%s%s%s%s%s\n", leftSep, serviceTitle, display.Ctext(color.KrakenPurple, "|"),
                   costTitle, display.Ctext(color.KrakenPurple, "|"))
        fmt.Println(strings.Repeat("-", totalWidth))

        // Iterate through contents of the service table
        for service, price := range costMan.CostTable {
            serviceColored := display.Ctext(color.NeonAzure, service)

            // Format price as decimal with 4 digits after decimal
            priceStr := fmt.Sprintf("%.4f", price)
            priceColored := display.Ctext(color.BrightLime, priceStr)

            serviceField := display.PadRightColored(serviceColored, serviceColWidth)
            costField := display.PadLeftColored(priceColored, costColWidth)

            fmt.Printf("%s%s%s%s%s\n", display.Ctext(color.KrakenPurple, "|"),
                       serviceField, display.Ctext(color.KrakenPurple, "|"),
                       costField, display.Ctext(color.KrakenPurple, "|"))
            }

            fmt.Println(strings.Repeat("-", totalWidth))
    } ()

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
