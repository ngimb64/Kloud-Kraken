package vpcsetup

import (
	"time"

	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/pkg/awscost"
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
func SetupNatGatewayHandler(ec2Client *ec2utils.Ec2Manger,
                            stateConfig *AwsEnv,
                            appConfig *conf.AppConfig,
                            yamlUpdates map[string]string,
                            outStruct *VpcBootstrapOutput,
                            pubSubnetId string, eipId string,
                            location string, costErr *error,
                            costMan *awscost.AwsCostManager) (
                            string, error) {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-nat-gateway",
    }

    // Create NAT gateway in public subnet if it does not exist
    natGatewayId, err := ec2Client.NatGatewayProvision(15 * time.Minute,
                                                       stateConfig.AwsEnv.NatGatewayId,
                                                       pubSubnetId, eipId, tags)
    if err != nil {
        return "", err
    }

    // If a NAT Gateway was created, add ID to yaml updates map
    if natGatewayId != "" {
        yamlUpdates["aws_env.nat_gateway_id"] = natGatewayId
    // Otherwise use the one from YAML since it was found
    } else {
        natGatewayId = stateConfig.AwsEnv.NatGatewayId
    }

    outStruct.NatGatewayId = natGatewayId

    filterMap := map[string]string{
        "location": location,
    }

    // Add the elastic IP to cost manager
    _ = costMan.AddCostResourceToManagerHandler("nat_gateway", filterMap,
                                                true, costErr)
    return natGatewayId, nil
}
