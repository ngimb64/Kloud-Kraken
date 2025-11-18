package vpcsetup

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/internal/globals"
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
    Ec2SecurityGroupId string `yaml:"ec2_security_group_id"`
    FlowLogId          string `yaml:"flow_log_id"`
    IamArnClient       string `yaml:"iam_arn_client"`
    IamArnServer       string `yaml:"iam_arn_server"`
    IamArnVpcFlowLogs  string `yaml:"iam_arn_vpc_flow_logs"`
    IgwId              string `yaml:"igw_id"`
    Region             string `yaml:"region"`
    RouteAssociationId string `yaml:"route_association_id"`
    RouteTableId       string `yaml:"route_table_id"`
    S3BucketName       string `yaml:"s3_bucket_name"`
    S3VpcEndpointId    string `yaml:"s3_vpc_endpoint_id"`
    SsmVpcEndpointId   string `yaml:"ssm_vpc_endpoint_id"`
    SsmSecurityGroupId string `yaml:"ssm_security_group_id"`
    SubnetId           string `yaml:"subnet_id"`
    VpcId              string `yaml:"vpc_id"`
}

type VpcBootstrapOutput struct {
    AccountId        string
    Ec2Client        *ec2utils.Ec2Manger
    Ec2SgId	         string
    SubnetId         string
    S3BucketName     string
    S3Client         *s3utils.S3Manager
    SsmClient        *ssmutils.SsmManager
    ServerArn        string
    SsmVpcEndpointId string
}


// Sets up an entire VPC with public subnet, isolated VPC Endpoints, etc.
// The resulting IDs of created AWS resources are saved to a map then the
// YAML state file is created or updated on subsequent uses.
//
// @Parameters
//  - appConfig:  Pointer to program config instance from YAML data
//  - awsConfig:  The configuration to access AWS environment
//  - ec2Client:  Pointer to EC2 service client management struct
//  - iamClient:  Pointer to IAM service client management struct
//  - stsClient:  The STS service client management struct
//
// @Returns
//  - The VPC bootstrap output struct
//  - The AWS cost manager that manages resource costs
//  - Errors associated with cost manager
//  - Error if it occurs, otherwise nil on success
//
func VpcBootstrap(appConfig *conf.AppConfig,
                  awsConfig aws.Config,
                  ec2Client *ec2utils.Ec2Manger,
                  iamClient *iamutils.IamManager,
                  stsClient sts.Client) (
                  *VpcBootstrapOutput,
                  *awscost.AwsCostManager,
                  error, error) {
    outStruct := &VpcBootstrapOutput{}
    var stateConfig AwsEnv
    var stateData []byte
    stateFilePath := globals.ROOT_DIR + "/.kraken-state.yml"
    var yamlUpdates = map[string]string{}

    // Check to see if the yaml state file exists
    exists, isDir, hasData, err := disk.PathExists(stateFilePath)
    if err != nil {
        return outStruct, nil, nil, err
    }

    // If the yaml state file exists and has data
    if exists && !isDir && hasData {
        // Read the data from yaml state file
        stateData, err = os.ReadFile(stateFilePath)
        if err != nil {
            return outStruct, nil, nil, err
        }

        // Decode raw bytes into StateConfig struct
        err = yaml.Unmarshal(stateData, &stateConfig)
        if err != nil {
            return outStruct, nil, nil, err
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
    if stateConfig.AwsEnv.Region != awsConfig.Region &&
    stateConfig.AwsEnv.Region != "" {
        return outStruct, nil, nil,
               errors.New("region in YAML config does not match state file, " +
                          "run teardown program first before running again")
    }

    // If the region is not present in the state file
    if stateConfig.AwsEnv.Region == "" {
        // Add the region to the updates map for YAML state file
        yamlUpdates["aws_env.region"] = awsConfig.Region
    }

    var costErr error
    // Create a PriceManager with a 1 hour cache TTL
	priceMan := awscost.NewPriceManager(1 * time.Hour)
	priceMan.RegisterProvider(awscost.NewAWSPricingProvider(awsConfig.Region))
	// Create the AwsCostManager using live-only PriceManager
	costMan := awscost.NewAwsCostManager(priceMan, nil)

    // Get human readable location string based off region for cost calculation
    location, exists := awsutils.RegionToLocation(awsConfig.Region)
    if !exists {
        return outStruct, costMan, costErr,
               errors.New("region does not exist in region map in awsutils")
    }

    // Setup the VPC
    vpcId, err := SetupVpcHandler(ec2Client, &stateConfig,
                                  appConfig, yamlUpdates)
    if err != nil {
        return outStruct, costMan, costErr,
               fmt.Errorf("setting up VPC - %w", err)
    }

    // Setup the Internet Gateway
    igwId, err := SetupInternetGatewayHandler(ec2Client, &stateConfig,
                                              appConfig, yamlUpdates,
                                              vpcId)
    if err != nil {
        return outStruct, costMan, costErr,
               fmt.Errorf("setting up Internet Gateway - %w", err)
    }

    // Set up the subnet
    subnetId, err := SetupSubnetHandler(ec2Client, &stateConfig, appConfig,
                                        yamlUpdates, outStruct, vpcId)
    if err != nil {
        return outStruct, costMan, costErr,
               fmt.Errorf("setting up subnet - %w", err)
    }

    // Setup the route table
    routeId, err := SetupRouteTableHandler(ec2Client, &stateConfig, appConfig,
                                           yamlUpdates, vpcId, igwId)
    if err != nil {
        return outStruct, costMan, costErr,
               fmt.Errorf("setting up route table - %w", err)
    }

    // Setup route table associations
    err = SetupRouteTableAssociationHandler(ec2Client, &stateConfig, appConfig,
                                            yamlUpdates, routeId, subnetId)
    if err != nil {
        return outStruct, costMan, costErr,
               fmt.Errorf("setting up route table associations - %w", err)
    }

    // Setup the EC2 security group
    ec2SgId, err := SetupEc2SecurityGroupHandler(ec2Client, &stateConfig,
                                                 appConfig, yamlUpdates,
                                                 outStruct, vpcId)
    if err != nil {
        return outStruct, costMan, costErr,
               fmt.Errorf("setting up EC2 security group - %w", err)
    }

    // Setup EC2 security group Rules
    err = SetupEc2SecurityGroupRulesHandler(ec2Client, &stateConfig,
                                            appConfig, yamlUpdates,
                                            ec2SgId)
    if err != nil {
        return outStruct, costMan, costErr,
               fmt.Errorf("setting up EC2 security group rules - %w", err)
    }

    // Setup the SSM security group
    ssmSgId, err := SetupSsmSecurityGroupHandler(ec2Client, &stateConfig,
                                                 appConfig, yamlUpdates,
                                                 vpcId)
    if err != nil {
        return outStruct, costMan, costErr,
               fmt.Errorf("setting up SSM security group - %w", err)
    }

    // Setup SSM Security group rules
    err = SetupSsmSecurityGroupRuleHandler(ec2Client, ssmSgId)
    if err != nil {
        return outStruct, costMan, costErr,
               fmt.Errorf("setting up SSM security group rules - %w", err)
    }

    // Setup the S3 bucket
    err = SetupS3BucketHandler(ec2Client, &stateConfig, appConfig,
                               yamlUpdates, outStruct, awsConfig.Region,
                               &costErr, costMan, awsConfig)
    if err != nil {
        return outStruct, costMan, costErr,
               fmt.Errorf("setting up S3 bucket - %w", err)
    }

    // Setup the S3 VPC Gateway Endpoint
    err = SetupS3VpcGatewayEndpointHandler(ec2Client, &stateConfig, appConfig,
                                           yamlUpdates, vpcId, routeId,
                                           location, &costErr, costMan)
    if err != nil {
        return outStruct, costMan, costErr,
               fmt.Errorf("setting up S3 VPC Gateway Endpoint - %w", err)
    }

    // Setup the SSM VPC Interface Endpoint
    err = SetupSsmVpcInterfaceEndpointHandler(ec2Client, &stateConfig, appConfig,
                                              yamlUpdates, outStruct, vpcId,
                                              subnetId, ssmSgId, location,
                                              &costErr, costMan)
    if err != nil {
        return outStruct, costMan, costErr,
               fmt.Errorf("setting up SSM VPC Interface Endpoint - %w", err)
    }

    // Setup VPC Flow Logs IAM role
    vpcFlowLogArn, err := SetupVpcFlowLogsIamRoleHandler(iamClient, stsClient,
                                                         &stateConfig, appConfig,
                                                         yamlUpdates, outStruct)
    if err != nil {
        return outStruct, costMan, costErr,
               fmt.Errorf("setting up VPC Flow Logs IAM role - %w", err)
    }

    // Setup the VPC Flow Logs
    err = SetupVpcFlowLogsHandler(ec2Client, &stateConfig, appConfig,
                                  yamlUpdates, awsConfig, vpcId,
                                  vpcFlowLogArn)
    if err != nil {
        return outStruct, costMan, costErr,
               fmt.Errorf("setting up VPC Flow Logs - %w", err)
    }

    // Setup Client IAM role
    err = SetupClientIamRoleHander(iamClient, &stateConfig, appConfig,
                                   yamlUpdates, outStruct)
    if err != nil {
        return outStruct, costMan, costErr,
               fmt.Errorf("setting up Client IAM Role - %w", err)
    }

    // Setup Server IAM role
    err = SetupServerIamRoleHandler(iamClient, &stateConfig, appConfig,
                                    yamlUpdates, outStruct)
    if err != nil {
        return outStruct, costMan, costErr,
               fmt.Errorf("setting up Server IAM Role - %w", err)
    }

    return outStruct, costMan, costErr, nil
}
