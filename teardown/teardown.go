package main

import (
	"fmt"
	"log"
	"os"
	"time"

	cwl "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/ngimb64/Kloud-Kraken/internal/globals"
	"github.com/ngimb64/Kloud-Kraken/internal/vpcsetup"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/cloudwatchutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"github.com/ngimb64/Kloud-Kraken/pkg/ec2utils"
	"github.com/ngimb64/Kloud-Kraken/pkg/iamutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/s3utils"
	"github.com/ngimb64/Kloud-Kraken/pkg/yamlutils"
	"gopkg.in/yaml.v2"
)

// Reads the state file and terminates any existing resources in it.
//
func main() {
    hadError := false
    var stateConfig vpcsetup.AwsEnv
    var stateData []byte
    var userInput string
    var yamlUpdates = map[string]string{}

    globals.ROOT_DIR = disk.GetProjectRootDir()
    stateFilePath := globals.ROOT_DIR + "/.kraken-state.yml"

    fmt.Print("[!] This program is designed to delete all created AWS " +
              "resources by Kloud Kraken, to proceed enter yes:  ")
    fmt.Scanln(&userInput)

    if userInput != "yes" {
        fmt.Print("[*] Input to continue (yes) not detected .. exiting program")
        return
    }

    // Read the data from yaml state file
    stateData, err := os.ReadFile(stateFilePath)
    if err != nil {
        log.Fatalf("Error reading state file for cleanup:  %v", err)
    }

    // Decode raw bytes into StateConfig struct
    err = yaml.Unmarshal(stateData, &stateConfig)
    if err != nil {
        log.Fatalf("Error unmarshaling state file YAML data into state struct:  %v", err)
    }

    if stateConfig.AwsEnv.Region == "" {
        log.Fatal("Missing region from state file, teardown operation likely not needed")
    }

    defer func() {
        // If all resources were deleted without errors
        if !hadError {
            // Delete the kraken state file
            err = os.Remove(stateFilePath)
            if err != nil {
                log.Fatalf("Error deleting state file prior to teardown:  %v", err)
            }

            return
        }

        // If there are no values in YAML file to be updated
        if len(yamlUpdates) == 0 {
            log.Print("No items in YAML updates map when there should be")
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

    // Set up the AWS credentials based on local chain or environment variables
    awsConfig, err := awsutils.AwsConfigSetup(1 * time.Minute,
                                              stateConfig.AwsEnv.Region,
                                              "kloud-kraken")
    if err != nil {
        log.Fatalf("Error loading AWS configuration:  %v", err)
    }

    // Establish clients to various services
    cwlClient := cwl.NewFromConfig(awsConfig)
    ec2Client := ec2utils.Ec2NewManager(awsConfig)
    iamClient := iamutils.IamNewManager(awsConfig)
    s3Client := s3utils.S3NewManager(awsConfig)

    if stateConfig.AwsEnv.S3BucketName != "" {
        // Delete the S3 bucket and its contents
        err = s3Client.S3BucketTerminator(5 * time.Minute,
                                          stateConfig.AwsEnv.S3BucketName)
        if err != nil {
            log.Printf("Error deleting S3 bucket and its contents:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.s3_bucket_name"] = ""
        }
    }

    if stateConfig.AwsEnv.FlowLogId != "" {
        // Delete the VPC Flow Logs
        err = ec2Client.VpcFlowLogTerminator(1 * time.Minute,
                                             []string{stateConfig.AwsEnv.FlowLogId})
        if err != nil {
            log.Printf("Error deleting VPC Flow Logs:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.flow_log_id"] = ""
        }

        // Delete the VPC flow Logs log group
        err = cloudwatchutils.DeleteLogGroup(1 * time.Minute, cwlClient,
                                            "kloud-kraken-vpc-flow-logs")
        if err != nil {
            log.Printf("Error deleting VPC Flow Logs log group:  %v", err)
        }
    }

    // Delete the CloudWatch logging stream and group for EC2 clients
    err = cloudwatchutils.CloudWatchLoggerTerminator(1 * time.Minute, cwlClient,
                                                     "kloud-kraken-logs", []string{})
    if err != nil {
        log.Printf("Error deleting CloudWatch logging stream and group:  %v", err)
    }

    if stateConfig.AwsEnv.S3VpcEndpointId != "" {
        // Terminate S3 bucket VPC Endpoint
        err = ec2Client.VpcEndpointsTerminator(1 * time.Minute,
                                               []string{stateConfig.AwsEnv.S3VpcEndpointId})
        if err != nil {
            log.Printf("Error deleting S3 VPC Endpoint:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.s3_vpc_endpoint_id"] = ""
        }
    }

    if stateConfig.AwsEnv.SsmVpcEndpointId != "" {
        // Terminate SSM Parameter Store VPC Endpoint
        err = ec2Client.VpcEndpointsTerminator(1 * time.Minute,
                                               []string{stateConfig.AwsEnv.SsmVpcEndpointId})
        if err != nil {
            log.Printf("Error deleting SSM Parameter Store VPC Endpoint:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.ssm_vpc_endpoint_id"] = ""
        }
    }

    if stateConfig.AwsEnv.RouteAssociationId != "" {
        // Disassociate private route table <-> subnet
        err = ec2Client.RouteTableAssociateTerminator(1 * time.Minute,
                                                      stateConfig.AwsEnv.RouteAssociationId)
        if err != nil {
            log.Printf("Error disassociating route table <-> subnet:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.route_association_id"] = ""
        }
    }

    if stateConfig.AwsEnv.RouteTableId != "" {
        // Delete the private route table
        err = ec2Client.RouteTableTerminator(1 * time.Minute,
                                             stateConfig.AwsEnv.RouteTableId)
        if err != nil {
            log.Printf("Error deleting Route Table:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.route_table_id"] = ""
        }
    }

    if stateConfig.AwsEnv.IgwId != "" {
        // Detach and delete the Internet Gateway
        err = ec2Client.InternetGatewayTerminator(2 * time.Minute,
                                                  stateConfig.AwsEnv.IgwId,
                                                  stateConfig.AwsEnv.VpcId)
        if err != nil {
            log.Printf("Error deleting Internet Gateway:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.igw_id"] = ""
        }
    }

    if stateConfig.AwsEnv.SubnetId != "" {
        // Delete the public subnet
        err = ec2Client.SubnetTerminator(1 * time.Minute,
                                         stateConfig.AwsEnv.SubnetId)
        if err != nil {
            log.Printf("Error deleting subnet:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.subnet_id"] = ""
        }
    }

    if stateConfig.AwsEnv.Ec2SecurityGroupId != "" {
        // Delete the EC2 security group
        err = ec2Client.SecurityGroupTerminator(1 * time.Minute,
                                                stateConfig.AwsEnv.Ec2SecurityGroupId)
        if err != nil {
            log.Printf("Error deleting EC2 seecurity group:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.ec2_security_group_id"] = ""
        }
    }

    if stateConfig.AwsEnv.SsmSecurityGroupId != "" {
        // Delete the SSM Parameter Store security group
        err = ec2Client.SecurityGroupTerminator(1 * time.Minute,
                                                stateConfig.AwsEnv.SsmSecurityGroupId)
        if err != nil {
            log.Printf("Error deleting SSM security group:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.ssm_security_group_id"] = ""
        }
    }

    if stateConfig.AwsEnv.VpcId != "" {
        // Delete the VPC
        err = ec2Client.VpcTerminator(5 * time.Minute,
                                      stateConfig.AwsEnv.VpcId)
        if err != nil {
            log.Printf("Error deleting VPC:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.vpc_id"] = ""
        }
    }

    if stateConfig.AwsEnv.IamArnVpcFlowLogs != "" {
        // Delete the VPC Flow Logs IAM role
        err = iamClient.IamRoleTerminator(5 * time.Minute,
                                          stateConfig.AwsEnv.IamArnVpcFlowLogs,
                                          false)
        if err != nil {
            log.Printf("Error deleting VPC Flow Logs IAM role:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.iam_arn_vpc_flow_logs"] = ""
        }
    }

    if stateConfig.AwsEnv.IamArnClient != "" {
        // Delete the client IAM role
        err = iamClient.IamRoleTerminator(5 * time.Minute,
                                          stateConfig.AwsEnv.IamArnClient,
                                          true)
        if err != nil {
            log.Printf("Error deleting client IAM role:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.iam_arn_client"] = ""
        }
    }

    if stateConfig.AwsEnv.IamArnServer != "" {
        // Delete the server IAM role
        err = iamClient.IamRoleTerminator(5 * time.Minute,
                                          stateConfig.AwsEnv.IamArnServer,
                                          false)
        if err != nil {
            log.Printf("Error deleting server IAM role:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.iam_arn_server"] = ""
        }
    }
}
