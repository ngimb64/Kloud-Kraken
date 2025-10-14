package vpcsetup

import (
	"time"

	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/internal/policies"
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
func SetupS3VpcGatewayEndpointHandler(ec2Client *ec2utils.Ec2Manger,
                                      stateConfig *AwsEnv,
                                      appConfig *conf.AppConfig,
                                      yamlUpdates map[string]string,
                                      bucketName string, vpcId string,
                                      routeId string, location string,
                                      costErr *error,
                                      costMan *awscost.AwsCostManager) error {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-s3-vpc-endpoint",
    }

    // Generate policy document for S3 VPC Endpoint
    policyDocument := policies.VpcS3EndpointPolicyGen(bucketName, vpcId)

    // Create VPC endpoint for S3 if it does not exist
    s3VpcEndPointId, err := ec2Client.S3EndpointProvision(10 * time.Minute,
                                                          stateConfig.AwsEnv.S3VpcEndpointId,
                                                          appConfig.LocalConfig.Region,
                                                          vpcId, policyDocument,
                                                          []string{routeId}, tags)
    if err != nil {
        return err
    }

    // If S3 VPC endpoint created, add name to yaml updates map
    if s3VpcEndPointId != "" {
        yamlUpdates["aws_env.s3_vpc_endpoint_id"] = s3VpcEndPointId
    // Otherwise use the one from YAML since it was found
    } else {
        s3VpcEndPointId = stateConfig.AwsEnv.S3VpcEndpointId
    }

    return  nil
}


//
//
// @Parameters
//
//
// @Returns
//
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
    policyDocument := policies.VpcSsmEndpointPolicyGen(outStruct.AccountId,
                                                      appConfig.LocalConfig.Region,
                                                      vpcId)

    // Create VPC endpoint for SSM if it does not exist
    ssmVpcEndpointId, err := ec2Client.SsmEndpointProvision(10 * time.Minute,
                                                            stateConfig.AwsEnv.SsmVpcEndpointId,
                                                            appConfig.LocalConfig.Region, vpcId,
                                                            policyDocument, []string{subnetId},
                                                            []string{ssmSgId}, tags)
    if err != nil {
        return err
    }

    // If SSM VPC endpoint was created, add name to yaml updates map
    if ssmVpcEndpointId != "" {
        yamlUpdates["aws_env.ssm_vpc_endpoint_id"] = ssmVpcEndpointId
    // Otherwise use the one from YAML since it was found
    } else {
        ssmVpcEndpointId = stateConfig.AwsEnv.SsmVpcEndpointId
    }

    outStruct.SsmVpcEndpointId = ssmVpcEndpointId

    filterMap := map[string]string{
        "location": location,
    }

    // Add the elastic IP to cost manager
    _ = costMan.AddCostResourceToManagerHandler("vpc_endpoint_ssm_hourly",
                                                filterMap, true, costErr)
    return nil
}
