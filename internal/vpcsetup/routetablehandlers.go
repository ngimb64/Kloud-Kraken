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
func SetupRouteTableAssociationsHandler(ec2Client *ec2utils.Ec2Manger,
                                        stateConfig *AwsEnv,
                                        appConfig *conf.AppConfig,
                                        yamlUpdates map[string]string,
                                        publicRouteId string,
                                        pubSubnetId string,
                                        privateRouteId string,
                                        privSubnetId string) (
                                        error) {
    // Ensure public route tables are associated to subnet
    publicAssocId, err := ec2Client.RouteTableAssociationProvision(1 * time.Minute,
                                                                   stateConfig.AwsEnv.PublicAssociationId,
                                                                   publicRouteId, pubSubnetId)
    if err != nil {
        return err
    }

    // If the public association occured, add ID to yaml updates map
    if publicAssocId != "" {
        yamlUpdates["aws_env.public_association_id"] = publicAssocId
    // Otherwise use the one from YAML since it was found
    } else {
        publicAssocId = stateConfig.AwsEnv.PublicAssociationId
    }

    // Ensure private route tables are associated to subnet
    privateAssocId, err := ec2Client.RouteTableAssociationProvision(1 * time.Minute,
                                                                    stateConfig.AwsEnv.PrivateAssociationId,
                                                                    privateRouteId, privSubnetId)
    if err != nil {
        return err
    }

    // If the private association occured, add ID to yaml updates map
    if privateAssocId != "" {
        yamlUpdates["aws_env.private_association_id"] = privateAssocId
    // Otherwise use the one from YAML since it was found
    } else {
        privateAssocId = stateConfig.AwsEnv.PrivateAssociationId
    }

    return nil
}


//
//
// @Parameters
//
//
// @Returns
//
//
func SetupRouteTablesHandler(ec2Client *ec2utils.Ec2Manger,
                             stateConfig *AwsEnv,
                             appConfig *conf.AppConfig,
                             yamlUpdates map[string]string,
                             vpcId string, igwId string,
                             natGatewayId string) (
                             string, string, error) {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-public-route-table",
    }

    // Create route table for subnets to internet gateway if does not exist
    publicRouteId, err := ec2Client.RouteTableProvision(1 * time.Minute,
                                                        stateConfig.AwsEnv.PublicRouteId,
                                                        vpcId, igwId, "", "0.0.0.0/0", tags)
    if err != nil {
        return publicRouteId, "", err
    }

    // If the public route table was created, add ID to yaml updates map
    if publicRouteId != "" {
        yamlUpdates["aws_env.public_route_id"] = publicRouteId
    // Otherwise use the one from YAML since it was found
    } else {
        publicRouteId = stateConfig.AwsEnv.PublicRouteId
    }

    tags = map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-private-route-table",
    }

    // Create route table for subnets to NAT Gateway if it does not exist
    privateRouteId, err := ec2Client.RouteTableProvision(1 * time.Minute,
                                                         stateConfig.AwsEnv.PrivateRouteId,
                                                         vpcId, "", natGatewayId,
                                                         "0.0.0.0/0", tags)
    if err != nil {
        return publicRouteId, privateRouteId, err
    }

    // If the private route table was created, add ID to yaml updates map
    if privateRouteId != "" {
        yamlUpdates["aws_env.private_route_id"] = privateRouteId
    // Otherwise use the one from YAML since it was found
    } else {
        privateRouteId = stateConfig.AwsEnv.PrivateRouteId
    }

    return publicRouteId, privateRouteId, nil
}
