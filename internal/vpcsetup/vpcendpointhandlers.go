package vpcsetup

import (
	"fmt"
	"time"

	"github.com/ngimb64/Kloud-Kraken/internal/color"
	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/internal/policies"
	"github.com/ngimb64/Kloud-Kraken/pkg/awscost"
	"github.com/ngimb64/Kloud-Kraken/pkg/display"
	"github.com/ngimb64/Kloud-Kraken/pkg/ec2utils"
)

// Handler function for setting up S3 VPC Gateway Endpoint.
//
// @Parameters
//  - ec2Client:  Pointer to EC2 service client management struct
//  - stateConfig:  Pointer to config struct for state file
//  - appConfig:  Pointer to program config instance from YAML data
//  - yamlUpdates:  The map used for updating output YAML data
//  - vpcId:  The VPC ID where the endpoint will be deployed
//  - routeId:  The ID of the route table associated with the endpoint
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func SetupS3VpcGatewayEndpointHandler(ec2Client *ec2utils.Ec2Manger,
                                      stateConfig *AwsEnv,
                                      appConfig *conf.AppConfig,
                                      yamlUpdates map[string]string,
                                      vpcId string, routeId string,
                                      accountId string) error {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-s3-vpc-endpoint",
    }

    // Generate policy document for S3 VPC Endpoint
    policyDocument := policies.VpcS3EndpointPolicyGen(accountId)

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Launching VPC S3 endpoint provisioner"))

    // Create VPC endpoint for S3 if it does not exist
    s3VpcEndPointId, err := ec2Client.S3EndpointProvision(10 * time.Minute,
                                                          stateConfig.AwsEnv.S3VpcEndpointId,
                                                          "us-east-1", vpcId, policyDocument,
                                                          []string{routeId}, tags)
    if err != nil {
        return err
    }

    // If S3 VPC endpoint created, add name to yaml updates map
    if s3VpcEndPointId != "" {
        yamlUpdates["aws_env.s3_vpc_endpoint_id"] = s3VpcEndPointId

        fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "S3 VPC endpoint was created"))
    // If S3 VPC endpoint already exists
    } else {
        fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "S3 VPC endpoint already exists"))
    }

    return  nil
}


// Handler function for setting up SSM VPC Interface Endpoint.
//
// @Parameters
//  - ec2Client:  Pointer to EC2 service client management struct
//  - stateConfig:  Pointer to config struct for state file
//  - appConfig:  Pointer to program config instance from YAML data
//  - yamlUpdates:  The map used for updating output YAML data
//  - outStruct:  Pointer to struct used for managing vcpsetup outputs
//  - vpcId:  The VPC ID where the endpoint will be deployed
//  - subnetId:  The subnet ID where endpoint will be deployed
//  - ssmSgId:  The security group ID associated with the endpoint
//  - location:  The human readable version of region
//  - costErr:  Pointer to error instance for cost manager
//  - costMan:  Pointer to AWS cost manager instance
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func SetupSsmVpcInterfaceEndpointHandler(ec2Client *ec2utils.Ec2Manger,
                                         stateConfig *AwsEnv,
                                         appConfig *conf.AppConfig,
                                         yamlUpdates map[string]string,
                                         outStruct *VpcBootstrapOutput,
                                         vpcId string, subnetId string,
                                         ssmSgId string, location string,
                                         costErr *error,
                                         costMan *awscost.AwsCostManager) error {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-ssm-vpc-endpoint",
    }

    // Generate policy document for SSM VPC Endpoint
    policyDocument := policies.VpcSsmEndpointPolicyGen(outStruct.AccountId)

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Launching VPC SSM endpoint provisioner"))

    // Create VPC endpoint for SSM if it does not exist
    ssmVpcEndpointId, err := ec2Client.SsmEndpointProvision(10 * time.Minute,
                                                            stateConfig.AwsEnv.SsmVpcEndpointId,
                                                            "us-east-1", vpcId, policyDocument,
                                                            []string{subnetId}, []string{ssmSgId}, tags)
    if err != nil {
        return err
    }

    // If SSM VPC endpoint was created, add name to yaml updates map
    if ssmVpcEndpointId != "" {
        yamlUpdates["aws_env.ssm_vpc_endpoint_id"] = ssmVpcEndpointId

        fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "SSM VPC endpoint was created"))
    // If SSM VPC endpoint already exists, use the existing ID
    } else {
        ssmVpcEndpointId = stateConfig.AwsEnv.SsmVpcEndpointId

        fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "SSM VPC endpoint already exists"))
    }

    outStruct.SsmVpcEndpointId = ssmVpcEndpointId

    filterMap := map[string]string{
        "endpointType":  "PrivateLink",
        "location":      location,
        "productFamily": "VpcEndpoint",
        "usagetype":     "USE1-VpcEndpoint-Hours",
    }

    // Add the elastic IP to cost manager
    _ = costMan.AddCostResourceToManager("vpc_endpoint_ssm_hourly",
                                         filterMap, true, costErr)
    return nil
}
