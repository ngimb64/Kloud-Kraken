package vpcsetup

import (
	"time"

	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/pkg/ec2utils"
)

//
//
// @Parameters
//
//
// @Returns
//
//
func SetupInternetGatewayHandler(ec2Client *ec2utils.Ec2Manger,
                                 stateConfig *AwsEnv,
                                 appConfig *conf.AppConfig,
                                 yamlUpdates map[string]string,
                                 vpcId string) (
                                 string, error) {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-internet-gateway",
    }

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
    // Otherwise use the one from YAML since it was found
    } else {
        igwId = stateConfig.AwsEnv.IgwId
    }

    return igwId, nil
}
