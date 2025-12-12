package vpcsetup

import (
	"fmt"
	"time"

	"github.com/ngimb64/Kloud-Kraken/internal/color"
	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/display"
	"github.com/ngimb64/Kloud-Kraken/pkg/ec2utils"
)

// Handler function for setting up subnet in specified VPC.
//
// @Parameters
//  - ec2Client:  Pointer to EC2 service client management struct
//  - stateConfig:  Pointer to config struct for state file
//  - appConfig:  Pointer to program config instance from YAML data
//  - yamlUpdates:  The map used for updating output YAML data
//  - outStruct:  Pointer to struct used for managing vcpsetup outputs
//  - vpcId:  The ID of the VPC where the subnet will be setup
//
// @Returns
//  - The subnet ID
//  - Error if it occurs, otherwise nil on success
//
func SetupSubnetHandler(ec2Client *ec2utils.Ec2Manger,
                        stateConfig *AwsEnv,
                        appConfig *conf.AppConfig,
                        yamlUpdates map[string]string,
                        outStruct *VpcBootstrapOutput,
                        vpcId string) (
                        string, error) {
    // Get the slice of availability zones based on region
    azs, err := ec2Client.Ec2FetchAvailableAZs(1 * time.Minute)
    if err != nil {
        return "", err
    }

    // Pick random AZ from slice of AZ names
    az := awsutils.PickAzRandom(azs)

    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-subnet",
    }

    fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                   display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Launching subnet provisioner"))

    // Create public subnet if it does not exist
    subnetId, err := ec2Client.SubnetProvision(5 * time.Minute,
                                               stateConfig.AwsEnv.SubnetId, vpcId,
                                               appConfig.LocalConfig.CidrBlock,
                                               az, tags, true)
    if err != nil {
        return subnetId, err
    }

    // If a public subnet was created, add ID to yaml updates map
    if subnetId != "" {
        yamlUpdates["aws_env.subnet_id"] = subnetId

        fmt.Println(display.CtextMulti(color.FoamWhite, "      \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "Subnet was created"))
    // If subnet already exists, use the existing ID
    } else {
        subnetId = stateConfig.AwsEnv.SubnetId

        fmt.Println(display.CtextMulti(color.FoamWhite, "      \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "Subnet already exists"))
    }

    outStruct.SubnetId = subnetId
    return subnetId, nil
}
