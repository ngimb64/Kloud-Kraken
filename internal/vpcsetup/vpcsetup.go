package vpcsetup

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwl "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/internal/policies"
	"github.com/ngimb64/Kloud-Kraken/pkg/awscost"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/disk"
	"github.com/ngimb64/Kloud-Kraken/pkg/ec2utils"
	"github.com/ngimb64/Kloud-Kraken/pkg/iamutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/s3utils"
	"github.com/ngimb64/Kloud-Kraken/pkg/ssmutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/yamlutils"
	"gopkg.in/yaml.v2"
)

type AwsEnv struct {
    AwsEnv StateConfig `yaml:"aws_env"`
}

type StateConfig struct {
    Ec2SecurityGroupId   string `yaml:"ec2_security_group_id"`
    EipId                string `yaml:"eip_id"`
    FlowLogId            string `yaml:"flow_log_id"`
    IamArnClient         string `yaml:"iam_arn_client"`
    IamArnServer         string `yaml:"iam_arn_server"`
    IamArnVpcFlowLogs    string `yaml:"iam_arn_vpc_flow_logs"`
    IgwId                string `yaml:"igw_id"`
    NatGatewayId         string `yaml:"nat_gateway_id"`
    PrivateAssociationId string `yaml:"private_association_id"`
    PrivateRouteId       string `yaml:"private_route_id"`
    PrivateSubnetId      string `yaml:"private_subnet_id"`
    PublicAssociationId  string `yaml:"public_association_id"`
    PublicRouteId        string `yaml:"public_route_id"`
    PublicSubnetId       string `yaml:"public_subnet_id"`
    Region               string `yaml:"region"`
    S3BucketName         string `yaml:"s3_bucket_name"`
    S3VpcEndpointId      string `yaml:"s3_vpc_endpoint_id"`
    SsmVpcEndpointId     string `yaml:"ssm_vpc_endpoint_id"`
    SsmSecurityGroupId   string `yaml:"ssm_security_group_id"`
    VpcId                string `yaml:"vpc_id"`
}

type VpcBootstrapOutput struct {
    AccountId        string
    Ec2Client        *ec2utils.Ec2Manger
    Ec2SgId	         string
    EipId            string
    NatGatewayId     string
    PrivSubnetId     string
    S3BucketName     string
    S3Client         *s3utils.S3Manager
    SsmClient        *ssmutils.SsmManager
    ServerArn        string
    SsmVpcEndpointId string
}


// Sets up an entire VPC with private-public subnets for allowinf internet
// access while keeping the EC2 instances in their own isolated environment.
// The resulting IDs of created AWS resources are saved to a map then the
// YAML state file is created or updated on subsequent uses.
//
// @Parameters
//  - appConfig:  The program configuration instance with parsed YAML data
//  - awsConfig:  The configuration to access AWS environment
//  - ec2Client:  The EC2 service client management struct
//  - iamClient:  The IAM service client management struct
//  - stsClient:  The STS service client management struct
//
// @Returns
//  - The VPC bootstrap output struct
//  - Error if it occurs, otherwise nil on success
//
func VpcBootstrap(appConfig conf.AppConfig,
                  awsConfig aws.Config,
                  ec2Client ec2utils.Ec2Manger,
                  iamClient iamutils.IamManager,
                  stsClient sts.Client) (
                  *VpcBootstrapOutput, error) {
    outStruct := &VpcBootstrapOutput{}
    var stateConfig AwsEnv
    var stateData []byte
    stateFilePath := "../.kraken-state.yml"
    var yamlUpdates map[string]string

    // Check to see if the yaml state file exists
    exists, isDir, hasData, err := disk.PathExists(stateFilePath)
    if err != nil {
        return outStruct, err
    }

    // If the yaml state file exists and has data
    if exists && !isDir && hasData {
        // Read the data from yaml state file
        stateData, err = os.ReadFile(stateFilePath)
        if err != nil {
            return outStruct, err
        }

        // Decode raw bytes into StateConfig struct
        err = yaml.Unmarshal(stateData, &stateConfig)
        if err != nil {
            return outStruct, err
        }
    }

    defer func() {
        // If there are no values in YAML file to be updated
        if len(yamlUpdates) == 0 {
            return
        }

        // Update the yaml values with values from passed in map
        newYaml, yerr := yamlutils.UpdateYAMLBytes(stateData, yamlUpdates)
        if yerr != nil {
            err = errors.Join(err, fmt.Errorf("updating yaml - %w", yerr))
            return
        }

        // Overwrite the original yaml with the updated data
        werr := os.WriteFile(stateFilePath, newYaml, 0644)
        if werr != nil {
            err = errors.Join(err, fmt.Errorf("writing output yaml - %w", werr))
        }
    }()

    // Check to see if region in the state file matches one from config
    if stateConfig.AwsEnv.Region != appConfig.LocalConfig.Region &&
    stateConfig.AwsEnv.Region != "" {
        return outStruct, errors.New("region in YAML config does not match state file, " +
                                     "run teardown program first before running again")
    }

    // If the region is not present in the state file
    if stateConfig.AwsEnv.Region == "" {
        // Add the region to the updates map for YAML state file
        yamlUpdates["aws_env.region"] = appConfig.LocalConfig.Region
    }

    var costErr error
    // Create a PriceManager with a 1 hour cache TTL
	priceMan := awscost.NewPriceManager(1 * time.Hour)
	priceMan.RegisterProvider(awscost.NewAWSPricingProvider(appConfig.LocalConfig.Region))
	// Create the AwsCostManager using live-only PriceManager
	costMan := awscost.NewAwsCostManager(priceMan, nil)

    // Get the human readable location string based off region for cost calculation
    location, exists := awsutils.RegionToLocation(appConfig.LocalConfig.Region)
    if !exists {
        return outStruct, errors.New("region does not exist in region map in awsutils")
    }

    // Setup the VPC
    vpcId, err := SetupVpcHandler(ec2Client, stateConfig, appConfig, yamlUpdates)
    if err != nil {
        return outStruct, fmt.Errorf("setting up VPC - %w", err)
    }

    // Setup the Internet Gateway
    igwId, err := SetupInternetGatewayHandler(ec2Client, stateConfig, appConfig,
                                              yamlUpdates, vpcId)
    if err != nil {
        return outStruct, fmt.Errorf("setting up Internet Gateway - %w", err)
    }

    // Setup the Elastic IP
    eipId, err := SetupElasticIpHandler(ec2Client, stateConfig, appConfig,
                                        yamlUpdates, outStruct,
                                        location, &costErr, costMan)
    if err != nil {
        return outStruct, fmt.Errorf("setting up Elastic IP - %w", err)
    }

    // Set up the Public and Private Subnets
    pubSubnetId, privSubnetId, err := SetupSubnetsHandler(ec2Client, stateConfig,
                                                          appConfig, yamlUpdates,
                                                          outStruct, vpcId)
    if err != nil {
        return outStruct, fmt.Errorf("setting up subnets - %w", err)
    }

    // Setup the NAT Gateway
    natGatewayId, err := SetupNatGatewayHandler(ec2Client, stateConfig, appConfig,
                                                yamlUpdates, outStruct, pubSubnetId,
                                                eipId, location, &costErr, costMan)
    if err != nil {
        return outStruct, fmt.Errorf("setting up NAT Gateway - %w", err)
    }

    // Setup Public & Private Route Tables
    publicRouteId, privateRouteId, err := SetupRouteTablesHandler(ec2Client, stateConfig,
                                                                  appConfig, yamlUpdates,
                                                                  vpcId, igwId, natGatewayId)
    if err != nil {
        return outStruct, fmt.Errorf("setting up route tables - %w", err)
    }

    // Setup Public & Private Route Table associations
    err = SetupRouteTableAssociationsHandler(ec2Client, stateConfig, appConfig,
                                             yamlUpdates, publicRouteId, pubSubnetId,
                                             privateRouteId, privSubnetId)
    if err != nil {
        return outStruct, fmt.Errorf("setting up route table associations - %w", err)
    }

    // Setup the EC2 Security Group
    ec2SgId, err := SetupEc2SecurityGroupHandler(ec2Client, stateConfig, appConfig,
                                                 yamlUpdates, outStruct, vpcId)
    if err != nil {
        return outStruct, fmt.Errorf("setting up EC2 security group - %w", err)
    }

    // Setup EC2 Security Group Rules
    err = SetupEc2SecurityGroupRulesHandler(ec2Client, stateConfig, appConfig,
                                            yamlUpdates, ec2SgId)
    if err != nil {
        return outStruct, fmt.Errorf("setting up EC2 security group rules - %w", err)
    }

    // Setup the SSM Security Group
    ssmSgId, err := SetupSsmSecurityGroupHandler(ec2Client, stateConfig, appConfig,
                                                 yamlUpdates, vpcId)
    if err != nil {
        return outStruct, fmt.Errorf("setting up SSM security group - %w", err)
    }

    // Setup SSM Security Group Rules
    err = SetupSsmSecurityGroupRuleHandler(ec2Client, ssmSgId)
    if err != nil {
        return outStruct, fmt.Errorf("setting up SSM security group rules - %w", err)
    }

    // Setup the S3 Bucket
    bucketName, err := SetupS3BucketHandler(ec2Client, stateConfig, appConfig,
                                            yamlUpdates, outStruct, awsConfig)
    if err != nil {
        return outStruct, fmt.Errorf("setting up S3 bucket - %w", err)
    }

    // Setup the S3 VPC Gateway Endpoint
    err = SetupS3VpcGatewayEndpointHandler(ec2Client, stateConfig, appConfig,
                                           yamlUpdates, bucketName, vpcId,
                                           privateRouteId, location, &costErr,
                                           costMan)
    if err != nil {
        return outStruct, fmt.Errorf("setting up S3 VPC Gateway Endpoint - %w", err)
    }

    // Setup the SSM VPC Interface Endpoint
    err = SetupSsmVpcInterfaceEndpointHandler(ec2Client, stateConfig, appConfig,
                                              yamlUpdates, outStruct, vpcId,
                                              privSubnetId, ssmSgId, location,
                                              &costErr, costMan)
    if err != nil {
        return outStruct, fmt.Errorf("setting up SSM VPC Interface Endpoint - %w", err)
    }


    // TODO:  finish refactoring below code into handlers


    // VPC Flow Logs IAM Role setup
    //------------------------------
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-iam-vpc-flow-logs",
    }

    // Get the account ID associated with API credentials
    outStruct.AccountId, err = awsutils.GetAccountID(1 * time.Minute, stsClient)
    if err != nil {
        return outStruct, err
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
        return outStruct, err
    }

    // If IAM ARN for VPC Flow Logs was created, add name to yaml updates map
    if vpcFlowLogArn != "" {
        yamlUpdates["aws_env.iam_arn_vpc_flow_logs"] = vpcFlowLogArn
    // Otherwise use the one from YAML since it was found
    } else {
        vpcFlowLogArn = stateConfig.AwsEnv.IamArnVpcFlowLogs
    }

    // VPC Flow Logs setup
    //---------------------
    tags = map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-vpc-flow-logs",
    }

    // Set up client to CloudWatch Logs
    cwlClient := cwl.NewFromConfig(awsConfig)

    // Create and enable the VPC Flow Logs via CloudWatch if it does not exist
    flowLogId, err := ec2Client.VpcFlowLogProvision(5 * time.Minute,
                                                    stateConfig.AwsEnv.FlowLogId,
                                                    vpcId, cwlClient,
                                                    "kloud-kraken-vpc-flow-logs",
                                                    vpcFlowLogArn, 1, tags)
    if err != nil {
        return outStruct, err
    }

    // If VPC Flow Logs group was created, add ID to yaml updates map
    if flowLogId != "" {
        yamlUpdates["aws_env.flow_log_id"] = flowLogId
    // Otherwise use the one from YAML since it was found
    } else {
        flowLogId = stateConfig.AwsEnv.FlowLogId
    }

    // Client IAM Role setup
    //-----------------------
    tags = map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-iam-client",
    }

    // Generate the EC2 clients trust and permissions policy templates
    trustPolicy = policies.ClientTrustPolicyGen()
    permissionsPolicy = policies.ClientPermPolicyGen(bucketName,
                                                     appConfig.ClientConfig.Region,
                                                     outStruct.AccountId,
                                                     "/kloud-kraken/tls-cert",
                                                     "kloud-kraken")
    // Create and apply the EC2 client role
    clientArn, err := iamClient.IamRoleProvision(5 * time.Minute,
                                                 stateConfig.AwsEnv.IamArnClient,
                                                 "KloudKrakenClientRole", trustPolicy,
                                                 "KloudKrakenClientPerms",
                                                 permissionsPolicy,
                                                 tags, true)
    if err != nil {
        return outStruct, err
    }

    // If IAM ARN for client was created, add name to yaml updates map
    if clientArn != "" {
        yamlUpdates["aws_env.iam_arn_client"] = clientArn
    // Otherwise use the one from YAML since it was found
    } else {
        clientArn = stateConfig.AwsEnv.IamArnClient
    }

    // Server IAM Role setup
    //-----------------------
    tags = map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-iam-server",
    }

    // Generate the servers trust and permissions policy templates
    trustPolicy = policies.ServerTrustPolicyGen(outStruct.AccountId,
                                                appConfig.LocalConfig.IamUsername)
    permissionsPolicy = policies.ServerPermPolicyGen(appConfig.LocalConfig.Region,
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
        return outStruct, err
    }

    // If IAM ARN for server was created, add name to yaml updates map
    if outStruct.ServerArn != "" {
        yamlUpdates["aws_env.iam_arn_server"] = outStruct.ServerArn
    // Otherwise use the one from YAML since it was found
    } else {
        outStruct.ServerArn = stateConfig.AwsEnv.IamArnServer
    }

    return outStruct, nil
}
