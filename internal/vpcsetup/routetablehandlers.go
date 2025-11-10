package vpcsetup

import (
	"time"

	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/pkg/ec2utils"
)

// Handler function for setting up the route table associations.
//
// @Parameters
//  - ec2Client:  Pointer to EC2 service client management struct
//  - stateConfig:  Pointer to config struct for state file
//  - appConfig:  Pointer to program config instance from YAML data
//  - yamlUpdates:  The map used for updating output YAML data
//  - routeId:  The ID of the route table to associate
//  - subnetId:  The ID of the subnet to associate
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func SetupRouteTableAssociationHandler(ec2Client *ec2utils.Ec2Manger,
                                       stateConfig *AwsEnv,
                                       appConfig *conf.AppConfig,
                                       yamlUpdates map[string]string,
                                       routeId string, subnetId string) (
                                       error) {
    // Ensure route table is associated to subnet
    publicAssocId, err := ec2Client.RouteTableAssociationProvision(1 * time.Minute,
                                                                   stateConfig.AwsEnv.RouteAssociationId,
                                                                   routeId, subnetId)
    if err != nil {
        return err
    }

    // If the public association occured, add ID to yaml updates map
    if publicAssocId != "" {
        yamlUpdates["aws_env.route_association_id"] = publicAssocId
    }

    return nil
}


// Handler function for setting up the route tables.
//
// @Parameters
//  - ec2Client:  Pointer to EC2 service client management struct
//  - stateConfig:  Pointer to config struct for state file
//  - appConfig:  Pointer to program config instance from YAML data
//  - yamlUpdates:  The map used for updating output YAML data
//  - vpcId:  The ID of the vpc to setup the route table in
//  - igwId:  The ID of the internet gateway that the route table points to
//
// @Returns
//  - Route table ID
//  - Error if it occurs, otherwise nil on success
//
func SetupRouteTableHandler(ec2Client *ec2utils.Ec2Manger,
                            stateConfig *AwsEnv,
                            appConfig *conf.AppConfig,
                            yamlUpdates map[string]string,
                            vpcId string, igwId string) (
                            string, error) {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-route-table",
    }

    // Create route table for subnets to internet gateway if does not exist
    routeTableId, err := ec2Client.RouteTableProvision(1 * time.Minute,
                                                       stateConfig.AwsEnv.RouteTableId,
                                                       vpcId, igwId, "", "0.0.0.0/0", tags)
    if err != nil {
        return routeTableId, err
    }

    // If the public route table was created, add ID to yaml updates map
    if routeTableId != "" {
        yamlUpdates["aws_env.route_table_id"] = routeTableId
    // Otherwise use the one from YAML since it was found
    } else {
        routeTableId = stateConfig.AwsEnv.RouteTableId
    }

    return routeTableId, nil
}
