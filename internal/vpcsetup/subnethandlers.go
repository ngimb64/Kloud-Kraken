package vpcsetup

import (
	"time"

	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/cidrutils"
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
func SetupSubnetsHandler(ec2Client *ec2utils.Ec2Manger,
                         stateConfig *AwsEnv,
                         appConfig *conf.AppConfig,
                         yamlUpdates map[string]string,
                         outStruct *VpcBootstrapOutput,
                         vpcId string) (
                         string, string, error) {
    // Get the slice of availability zones based on region
    azs, err := ec2Client.FetchAvailableAZs(1 * time.Minute)
    if err != nil {
        return "", "", err
    }

    // Pick random AZ from slice of AZ names
    az := awsutils.PickAzRandom(azs)

    // Set up map for ensuring unique subnet allocation
    subnetMap := map[string]struct{}{}

    // Parse the prefix length from CIDR
    prefixLength, err := cidrutils.PrefixFromCidr(appConfig.LocalConfig.CidrBlock)
    if err != nil {
        return "", "", err
    }

    // Allocate first available subnet in CIDR block for public subnet
    pubCidr, err := cidrutils.AllocateNextSubnet(appConfig.LocalConfig.CidrBlock,
                                                 subnetMap, prefixLength + 1)
    if err != nil {
        return "", "", err
    }

    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-public-subnet",
    }

    // Create public subnet if it does not exist
    pubSubnetId, err := ec2Client.SubnetProvision(5 * time.Minute,
                                                  stateConfig.AwsEnv.PublicSubnetId,
                                                  vpcId, pubCidr, az, tags, true)
    if err != nil {
        return pubSubnetId, "", err
    }

    // If a public subnet was created, add ID to yaml updates map
    if pubSubnetId != "" {
        yamlUpdates["aws_env.public_subnet_id"] = pubSubnetId
    // Otherwise use the one from YAML since it was found
    } else {
        pubSubnetId = stateConfig.AwsEnv.PublicSubnetId
    }

    // Allocate next available subnet in CIDR block for private subnet
    privCidr, err := cidrutils.AllocateNextSubnet(appConfig.LocalConfig.CidrBlock,
                                                  subnetMap, prefixLength + 1)
    if err != nil {
        return pubSubnetId, "", err
    }

    tags = map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-private-subnet",
    }

    // Create private subnet if it does not exist
    privSubnetId, err := ec2Client.SubnetProvision(5 * time.Minute,
                                                   stateConfig.AwsEnv.PrivateSubnetId,
                                                   vpcId, privCidr, az, tags, false)
    if err != nil {
        return pubSubnetId, privSubnetId, err
    }

    // If a private subnet was created, add ID to yaml updates map
    if privSubnetId != "" {
        yamlUpdates["aws_env.private_subnet_id"] = privSubnetId
    // Otherwise use the one from YAML since it was found
    } else {
        privSubnetId = stateConfig.AwsEnv.PrivateSubnetId
    }

    outStruct.PrivSubnetId = privSubnetId
    return pubSubnetId, privSubnetId, nil
}
