package vpcsetup

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/internal/policies"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/iamutils"
)

// Handler function for generating permissions and trust policy for setting
// up client IAM role.
//
// @Parameters
//  - iamClient:  Pointer to IAM service client management struct
//  - stateConfig:  Pointer to config struct for state file
//  - appConfig:  Pointer to program config instance from YAML data
//  - yamlUpdates:  The map used for updating output YAML data
//  - outStruct:  Pointer to struct used for managing vcpsetup outputs
//  - bucketName:  Name of the S3 bucket used
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func SetupClientIamRoleHander(iamClient *iamutils.IamManager,
                              stateConfig *AwsEnv,
                              appConfig *conf.AppConfig,
                              yamlUpdates map[string]string,
                              outStruct *VpcBootstrapOutput,
                              bucketName string) error {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-iam-client",
    }

    // Generate the EC2 clients trust and permissions policy templates
    trustPolicy := policies.ClientTrustPolicyGen()
    permissionsPolicy := policies.ClientPermPolicyGen(bucketName,
                                                      appConfig.ClientConfig.Region,
                                                      outStruct.AccountId,
                                                      "/kloud-kraken/tls-cert",
                                                      "kloud-kraken")
    // Create and apply the EC2 client role
    clientArn, err := iamClient.IamRoleProvision(5 * time.Minute,
                                                 stateConfig.AwsEnv.IamArnClient,
                                                 "KloudKrakenClientRole", trustPolicy,
                                                 "KloudKrakenClientPerms",
                                                 permissionsPolicy, tags, true)
    if err != nil {
        return err
    }

    // If IAM ARN for client was created, add name to yaml updates map
    if clientArn != "" {
        yamlUpdates["aws_env.iam_arn_client"] = clientArn
    // Otherwise use the one from YAML since it was found
    } else {
        clientArn = stateConfig.AwsEnv.IamArnClient
    }

    return nil
}


// Handler function for generating permissions and trust policy for setting
// up server IAM role.
//
// @Parameters
//  - iamClient:  Pointer to IAM service client management struct
//  - stateConfig:  Pointer to config struct for state file
//  - appConfig:  Pointer to program config instance from YAML data
//  - yamlUpdates:  The map used for updating output YAML data
//  - outStruct:  Pointer to struct used for managing vcpsetup outputs
//  - bucketName:  Name of the S3 bucket used
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func SetupServerIamRoleHandler(iamClient *iamutils.IamManager,
                               stateConfig *AwsEnv,
                               appConfig *conf.AppConfig,
                               yamlUpdates map[string]string,
                               outStruct *VpcBootstrapOutput,
                               bucketName string) error {
    var err error
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-iam-server",
    }

    // Generate the servers trust and permissions policy templates
    trustPolicy := policies.ServerTrustPolicyGen(outStruct.AccountId,
                                                appConfig.LocalConfig.IamUsername)
    permissionsPolicy := policies.ServerPermPolicyGen(appConfig.LocalConfig.Region,
                                                     outStruct.AccountId,
                                                     "/kloud-kraken/tls-cert",
                                                     bucketName, "KloudKrakenClientRole")
    // Create and apply role for local server permissions
    outStruct.ServerArn, err = iamClient.IamRoleProvision(5 * time.Minute,
                                                          stateConfig.AwsEnv.IamArnServer,
                                                          "KloudKrakenServerRole", trustPolicy,
                                                          "KloudKrakenServerPerms",
                                                          permissionsPolicy,
                                                          tags, false)
    if err != nil {
        return err
    }

    // If IAM ARN for server was created, add name to yaml updates map
    if outStruct.ServerArn != "" {
        yamlUpdates["aws_env.iam_arn_server"] = outStruct.ServerArn
    // Otherwise use the one from YAML since it was found
    } else {
        outStruct.ServerArn = stateConfig.AwsEnv.IamArnServer
    }

    return nil
}


// Handler function for generating permissions and trust policy for setting
// up VPC flow logs IAM role.
//
// @Parameters
//  - iamClient:  Pointer to IAM service client management struct
//  - stsClient:  The STS service client management struct
//  - stateConfig:  Pointer to config struct for state file
//  - appConfig:  Pointer to program config instance from YAML data
//  - yamlUpdates:  The map used for updating output YAML data
//  - outStruct:  Pointer to struct used for managing vcpsetup outputs
//
// @Returns
//  - VPC flow logs role ARN
//  - Error if it occurs, otherwise nil on success
//
func SetupVpcFlowLogsIamRoleHandler(iamClient *iamutils.IamManager,
                                    stsClient sts.Client,
                                    stateConfig *AwsEnv,
                                    appConfig *conf.AppConfig,
                                    yamlUpdates map[string]string,
                                    outStruct *VpcBootstrapOutput) (
                                    string, error) {
    var err error
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-iam-vpc-flow-logs",
    }

    // Get the account ID associated with API credentials
    outStruct.AccountId, err = awsutils.GetAccountID(1 * time.Minute, stsClient)
    if err != nil {
        return "", err
    }

    // Generate the VPC Flow Logs trust and permissions policy templates
    trustPolicy := policies.VpcFlowLogsTrustPolicyGen()
    permissionsPolicy := policies.VpcFlowLogsPermPolicyGen(appConfig.LocalConfig.Region,
                                                           outStruct.AccountId,
                                                           "kloud-kraken-vpc-flow-logs")
    // Create and appy the VPC flow logs role
    vpcFlowLogArn, err := iamClient.IamRoleProvision(5 * time.Minute,
                                                     stateConfig.AwsEnv.IamArnVpcFlowLogs,
                                                     "KloudKrakenVpcFlowLogsRole", trustPolicy,
                                                     "KloudKrakenVpcFlowLogPerms",
                                                     permissionsPolicy, tags, false)
    if err != nil {
        return vpcFlowLogArn, err
    }

    // If IAM ARN for VPC Flow Logs was created, add name to yaml updates map
    if vpcFlowLogArn != "" {
        yamlUpdates["aws_env.iam_arn_vpc_flow_logs"] = vpcFlowLogArn
    // Otherwise use the one from YAML since it was found
    } else {
        vpcFlowLogArn = stateConfig.AwsEnv.IamArnVpcFlowLogs
    }

    return vpcFlowLogArn, nil
}
