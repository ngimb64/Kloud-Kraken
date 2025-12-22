package vpcsetup

import (
	"fmt"
	"time"

	"github.com/ngimb64/Kloud-Kraken/internal/color"
	"github.com/ngimb64/Kloud-Kraken/pkg/display"
	"github.com/ngimb64/Kloud-Kraken/pkg/ec2utils"
)

// Handler function for setting up the Internet Gateway.
//
// @Parameters
//  - ec2Client:  Pointer to EC2 service client management struct
//  - stateConfig:  Pointer to config struct for state file
//  - yamlUpdates:  The map used for updating output YAML data
//  - vpcId:  The ID of the VPC to setup the IGW in
//
// @Returns
//  - Internet Gateway ID
//  - Error if it occurs, otherwise nil on success
//
func SetupInternetGatewayHandler(ec2Client *ec2utils.Ec2Manger,
                                 stateConfig *AwsEnv,
                                 yamlUpdates map[string]string,
                                 vpcId string) (
                                 string, error) {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-internet-gateway",
    }

    fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                   display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Launching Internet Gateway provisioner"))

    // Check to see if IGW exists, otherwise create & attach one
    igwId, err := ec2Client.InternetGatewayProvision(5 * time.Minute,
                                                     stateConfig.AwsEnv.IgwId,
                                                     vpcId, tags)
    if err != nil {
        return "", err
    }

    // If a Internet Gateway was created, add ID to yaml updates map
    if igwId != "" {
        yamlUpdates["aws_env.igw_id"] = igwId

        fmt.Println(display.CtextMulti(color.FoamWhite, "      \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "Internet Gateway was created"))
    // If Internet Gateway already exists, use the existing ID
    } else {
        igwId = stateConfig.AwsEnv.IgwId

        fmt.Println(display.CtextMulti(color.FoamWhite, "      \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "Internet Gateway already exists"))
    }

    return igwId, nil
}
