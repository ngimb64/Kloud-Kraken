package vpcsetup

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwl "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/ngimb64/Kloud-Kraken/internal/color"
	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/pkg/display"
	"github.com/ngimb64/Kloud-Kraken/pkg/ec2utils"
)

// Handler function for setting up the VPC flow logs.
//
// @Parameters
//  - ec2Client:  Pointer to EC2 service client management struct
//  - stateConfig:  Pointer to config struct for state file
//  - appConfig:  Pointer to program config instance from YAML data
//  - yamlUpdates:  The map used for updating output YAML data
//  - awsConfig:  The AWS configuration instance
//  - vpcId:  The ID of the VPC where flow logs will be applied
//  - vpcFlowLogArn:  The VPC flow logs ARN
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func SetupVpcFlowLogsHandler(ec2Client *ec2utils.Ec2Manger,
                             stateConfig *AwsEnv,
                             appConfig *conf.AppConfig,
                             yamlUpdates map[string]string,
                             awsConfig aws.Config, vpcId string,
                             vpcFlowLogArn string) error {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-vpc-flow-logs",
    }

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Launching VPC flow logs provisioner"))

    // Set up client to CloudWatch Logs
    cwlClient := cwl.NewFromConfig(awsConfig)
    // Create and enable the VPC Flow Logs via CloudWatch if it does not exist
    flowLogId, err := ec2Client.VpcFlowLogProvision(5 * time.Minute,
                                                    stateConfig.AwsEnv.FlowLogId,
                                                    vpcId, cwlClient,
                                                    "kloud-kraken-vpc-flow-logs",
                                                    vpcFlowLogArn, 1, tags)
    if err != nil {
        return err
    }

    // If VPC Flow Logs group was created, add ID to yaml updates map
    if flowLogId != "" {
        yamlUpdates["aws_env.flow_log_id"] = flowLogId

        fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "VPC flow logs was created"))
    // If VPC flow logs already exists
    } else {
        fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "VPC flow logs already exists"))
    }

    return nil
}


// Handler function for setting up the VPC.
//
// @Parameters
//  - ec2Client:  Pointer to EC2 service client management struct
//  - stateConfig:  Pointer to config struct for state file
//  - appConfig:  Pointer to program config instance from YAML data
//  - yamlUpdates:  The map used for updating output YAML data
//
// @Returns
//  - The VPC ID
//  - Error if it occurs, otherwise nil on success
//
func SetupVpcHandler(ec2Client *ec2utils.Ec2Manger,
                     stateConfig *AwsEnv,
                     appConfig *conf.AppConfig,
                     yamlUpdates map[string]string) (
                     string, error) {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-vpc",
    }

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Launching VPC provisioner"))

    // Check to see if the VPC exists, otherwise create one
    vpcId, err := ec2Client.VpcProvision(10*time.Minute,
                                         stateConfig.AwsEnv.VpcId,
                                         appConfig.LocalConfig.CidrBlock,
                                         tags)
    if err != nil {
        return "", err
    }

    // If a VPC was created, add ID to yaml updates map
    if vpcId != "" {
        yamlUpdates["aws_env.vpc_id"] = vpcId

        fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "VPC was created"))
    // If VPC already exists, use the existing ID
    } else {
        vpcId = stateConfig.AwsEnv.VpcId

        fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "VPC already exists"))
    }

    return vpcId, nil
}
