package main

import (
	"fmt"
	"log"
	"os"
	"time"

	cwl "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/ngimb64/Kloud-Kraken/internal/vpcsetup"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/cloudwatchutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/ec2utils"
	"github.com/ngimb64/Kloud-Kraken/pkg/iamutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/s3utils"
	"github.com/ngimb64/Kloud-Kraken/pkg/ssmutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/yamlutils"
	"gopkg.in/yaml.v2"
)

func main() {
    hadError := false
    var stateConfig vpcsetup.AwsEnv
    var stateData []byte
    stateFilePath := "../.kraken-state.yml"
    var userInput string
    var yamlUpdates map[string]string

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
                                              stateConfig.AwsEnv.Region)
    if err != nil {
        log.Fatalf("Error loading AWS configuration:  %v", err)
    }

	// Establish clients to various services
	ec2Client := ec2utils.Ec2NewManager(awsConfig)
	iamClient := iamutils.IamNewManager(awsConfig)
    s3Client := s3utils.S3NewManager(awsConfig)
    ssmClient := ssmutils.SsmNewManager(awsConfig)

    if stateConfig.AwsEnv.S3BucketName != "" {
        // Delete the S3 bucket and its contents
        err = s3Client.S3TerminateBucket(5 * time.Minute,
                                         stateConfig.AwsEnv.S3BucketName)
        if err != nil {
            log.Printf("Error deleting S3 bucket and its contents:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.s3_bucket_name"] = ""
        }
    }

    // Delete all the client TLS certificate from SSM Parameter store
    err = ssmClient.SsmDeleteAllParams(1 * time.Minute,
                                       "/kloud-kraken/tls-cert")
    if err != nil {
        log.Printf("Error deleting parameters from SSM Param Store:  %v", err)
        hadError = true
    }

    // Setup client to CloudWatch
    cwlClient := cwl.NewFromConfig(awsConfig)

    if stateConfig.AwsEnv.FlowLogId != "" {
        // Delete the VPC Flow Logs
        err = ec2Client.VpcFlowLogTerminate(1 * time.Minute,
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
    err = cloudwatchutils.TerminateCloudWatchLogger(1 * time.Minute, cwlClient,
                                                    "kloud-kraken-logs", []string{})
    if err != nil {
        log.Printf("Error deleting CloudWatch logging stream and group:  %v", err)
    }

    if stateConfig.AwsEnv.S3VpcEndpointId != "" {
        // Terminate S3 bucket VPC Endpoint
        err = ec2Client.VpcEndpointsTerminate(1 * time.Minute,
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
        err = ec2Client.VpcEndpointsTerminate(1 * time.Minute,
                                              []string{stateConfig.AwsEnv.SsmVpcEndpointId})
        if err != nil {
            log.Printf("Error deleting SSM Parameter Store VPC Endpoint:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.ssm_vpc_endpoint_id"] = ""
        }
    }

    if stateConfig.AwsEnv.NatGatewayId != "" {
        // Terminate the NAT Gateway
        err = ec2Client.NatGatewayTerminate(10 * time.Minute,
                                            stateConfig.AwsEnv.NatGatewayId)
        if err != nil {
            log.Printf("Error deleting NAT Gateway:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.nat_gateway_id"] = ""
        }
    }

    if stateConfig.AwsEnv.EipId != "" {
        // Release the Elastic IP
        err = ec2Client.ElasticIpTerminate(1 * time.Minute,
                                           stateConfig.AwsEnv.EipId)
        if err != nil {
            log.Printf("Error releasing Elastic IP:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.eip_id"] = ""
        }
    }

    if stateConfig.AwsEnv.PrivateAssociationId != "" {
        // Disassociate private route table <-> subnet
        err = ec2Client.RouteTableAssociateTerminate(1 * time.Minute,
                                                     stateConfig.AwsEnv.PrivateAssociationId)
        if err != nil {
            log.Printf("Error disassociating private route table <-> subnet:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.private_association_id"] = ""
        }
    }

    if stateConfig.AwsEnv.PublicAssociationId != "" {
        // Disassociate public route table <-> public subnet
        err = ec2Client.RouteTableAssociateTerminate(1 * time.Minute,
                                                     stateConfig.AwsEnv.PublicAssociationId)
        if err != nil {
            log.Printf("Error disassociating public route table <-> subnet:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.public_association_id"] = ""
        }
    }

    if stateConfig.AwsEnv.PrivateRouteId != "" {
        // Delete the private route table
        err = ec2Client.RouteTableTerminate(1 * time.Minute,
                                            stateConfig.AwsEnv.PrivateRouteId)
        if err != nil {
            log.Printf("Error deleting Private Route Table:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.private_route_id"] = ""
        }
    }

    if stateConfig.AwsEnv.PublicRouteId != "" {
        // Delete the public route table
        err = ec2Client.RouteTableTerminate(1 * time.Minute,
                                            stateConfig.AwsEnv.PublicRouteId)
        if err != nil {
            log.Printf("Error deleting Public Route Table:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.public_route_id"] = ""
        }
    }

    if stateConfig.AwsEnv.IgwId != "" {
        // Detach and delete the Internet Gateway
        err = ec2Client.InternetGatewayTerminate(2 * time.Minute,
                                                 stateConfig.AwsEnv.IgwId,
                                                 stateConfig.AwsEnv.VpcId)
        if err != nil {
            log.Printf("Error deleting Internet Gateway:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.igw_id"] = ""
        }
    }

    if stateConfig.AwsEnv.PrivateSubnetId != "" {
        // Delete the private subnet
        err = ec2Client.SubnetTerminate(1 * time.Minute,
                                        stateConfig.AwsEnv.PrivateSubnetId)
        if err != nil {
            log.Printf("Error deleting private subnet:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.private_subnet_id"] = ""
        }
    }

    if stateConfig.AwsEnv.PublicSubnetId != "" {
        // Delete the public subnet
        err = ec2Client.SubnetTerminate(1 * time.Minute,
                                        stateConfig.AwsEnv.PublicSubnetId)
        if err != nil {
            log.Printf("Error deleting public subnet:  %v", err)
            hadError = true
        } else {
            yamlUpdates["aws_env.public_subnet_id"] = ""
        }
    }

    if stateConfig.AwsEnv.Ec2SecurityGroupId != "" {
        // Delete the EC2 security group
        err = ec2Client.SecurityGroupTerminate(1 * time.Minute,
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
        err = ec2Client.SecurityGroupTerminate(1 * time.Minute,
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
        err = ec2Client.VpcTerminate(5 * time.Minute,
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
