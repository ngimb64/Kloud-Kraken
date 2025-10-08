package vpcsetup

import (
	"time"

	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
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
func SetupSubnetHandler(ec2Client *ec2utils.Ec2Manger,
                        stateConfig *AwsEnv,
                        appConfig *conf.AppConfig,
                        yamlUpdates map[string]string,
                        outStruct *VpcBootstrapOutput,
                        vpcId string) (
                        string, error) {
    // Get the slice of availability zones based on region
    azs, err := ec2Client.FetchAvailableAZs(1 * time.Minute)
    if err != nil {
        return "", err
    }

    // Pick random AZ from slice of AZ names
    az := awsutils.PickAzRandom(azs)

    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-subnet",
    }

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
    // Otherwise use the one from YAML since it was found
    } else {
        subnetId = stateConfig.AwsEnv.SubnetId
    }

    outStruct.SubnetId = subnetId
    return subnetId, nil
}
