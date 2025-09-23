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
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/cidrutils"
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


//
//
// @Parameters
//
//
// @Returns
//
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

    // VPC setup
    //-----------
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-vpc",
    }

    // Check to see if the VPC exists, otherwise create one
    vpcId, err := ec2Client.VpcProvision(10 * time.Minute,
                                         stateConfig.AwsEnv.VpcId,
                                         appConfig.LocalConfig.CidrBlock,
                                         tags)
    if err != nil {
        return outStruct, err
    }

    // If a VPC was created, add ID to yaml updates map
    if vpcId != "" {
        yamlUpdates["aws_env.vpc_id"] = vpcId
    // Otherwise use the one from YAML since it was found
    } else {
        vpcId = stateConfig.AwsEnv.VpcId
    }

    // Internet Gateway setup
    //------------------------
    tags = map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-internet-gateway",
    }

    // Check to see if IGW exists, otherwise create & attach one
    igwId, err := ec2Client.InternetGatewayProvision(5 * time.Minute,
                                                     stateConfig.AwsEnv.IgwId,
                                                     vpcId, tags)
    if err != nil {
        return outStruct, err
    }

    // If a Internet Gateway was created, add ID to yaml updates map
    if igwId != "" {
        yamlUpdates["aws_env.igw_id"] = igwId
    // Otherwise use the one from YAML since it was found
    } else {
        igwId = stateConfig.AwsEnv.IgwId
    }

    // Elastic IP setup
    //------------------
    tags = map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-elastic-ip",
    }

    // Check to see if Elastic IP exists, otherwise create one
    eipId, err := ec2Client.ElasticIpProvision(1 * time.Minute,
                                               stateConfig.AwsEnv.EipId,
                                               tags)
    if err != nil {
        return outStruct, err
    }

    // If a Elastic IP was created, add ID to yaml updates map
    if eipId != "" {
        yamlUpdates["aws_env.eip_id"] = eipId
    // Otherwise use the one from YAML since it was found
    } else {
        eipId = stateConfig.AwsEnv.EipId
    }

    outStruct.EipId = eipId

    // Public and Private Subnets setup
    //----------------------------------

    // Get the slice of availability zones based on region
    azs, err := ec2Client.FetchAvailableAZs(1 * time.Minute)
    if err != nil {
        return outStruct, err
    }

    // Pick random AZ from slice of AZ names
    az := awsutils.PickAzRandom(azs)

    // Set up map for ensuring unique subnet allocation
    subnetMap := map[string]struct{}{}

    // Parse the prefix length from CIDR
    prefixLength, err := cidrutils.PrefixFromCidr(appConfig.LocalConfig.CidrBlock)
    if err != nil {
        return outStruct, err
    }

    // Allocate first available subnet in CIDR block for public subnet
    pubCidr, err := cidrutils.AllocateNextSubnet(appConfig.LocalConfig.CidrBlock,
                                                 subnetMap, prefixLength + 1)
    if err != nil {
        return outStruct, err
    }

    tags = map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-public-subnet",
    }

    // Create public subnet if it does not exist
    pubSubnetId, err := ec2Client.SubnetProvision(5 * time.Minute,
                                                  stateConfig.AwsEnv.PublicSubnetId,
                                                  vpcId, pubCidr, az, tags, true)
    if err != nil {
        return outStruct, err
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
        return outStruct, err
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
        return outStruct, err
    }

    // If a private subnet was created, add ID to yaml updates map
    if privSubnetId != "" {
        yamlUpdates["aws_env.private_subnet_id"] = privSubnetId
    // Otherwise use the one from YAML since it was found
    } else {
        privSubnetId = stateConfig.AwsEnv.PrivateSubnetId
    }

    outStruct.PrivSubnetId = privSubnetId

    // NAT Gateway setup
    //-------------------
    tags = map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-nat-gateway",
    }

    // Create NAT gateway in public subnet if it does not exist
    natGatewayId, err := ec2Client.NatGatewayProvision(15 * time.Minute,
                                                       stateConfig.AwsEnv.NatGatewayId,
                                                       pubSubnetId, eipId, tags)
    if err != nil {
        return outStruct, err
    }

    // If a NAT Gateway was created, add ID to yaml updates map
    if natGatewayId != "" {
        yamlUpdates["aws_env.nat_gateway_id"] = natGatewayId
    // Otherwise use the one from YAML since it was found
    } else {
        natGatewayId = stateConfig.AwsEnv.NatGatewayId
    }

    outStruct.NatGatewayId = natGatewayId

    // Public & Private Route Table setup
    //------------------------------------
    tags = map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-public-route-table",
    }

    // Create route table for subnets to internet gateway if does not exist
    publicRouteId, err := ec2Client.RouteTableProvision(1 * time.Minute,
                                                        stateConfig.AwsEnv.PublicRouteId,
                                                        vpcId, igwId, "", pubSubnetId,
                                                        "0.0.0.0/0", tags)
    if err != nil {
        return outStruct, err
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
                                                         privSubnetId, "0.0.0.0/0", tags)
    if err != nil {
        return outStruct, err
    }

    // If the private route table was created, add ID to yaml updates map
    if privateRouteId != "" {
        yamlUpdates["aws_env.private_route_id"] = privateRouteId
    // Otherwise use the one from YAML since it was found
    } else {
        privateRouteId = stateConfig.AwsEnv.PrivateRouteId
    }

    // Public & Private Route Table association
    //------------------------------------------

    // Ensure public route tables are associated to subnet
    publicAssocId, err := ec2Client.RouteTableAssociationProvision(1 * time.Minute,
                                                                   stateConfig.AwsEnv.PublicAssociationId,
                                                                   publicRouteId, pubSubnetId)
    if err != nil {
        return outStruct, err
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
        return outStruct, err
    }

    // If the private association occured, add ID to yaml updates map
    if privateAssocId != "" {
        yamlUpdates["aws_env.private_association_id"] = privateAssocId
    // Otherwise use the one from YAML since it was found
    } else {
        privateAssocId = stateConfig.AwsEnv.PrivateAssociationId
    }

    // EC2 Security Group setup
    //--------------------------
    tags = map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-ec2-security-group",
    }

    // Create EC2 security group if it does not exist
    ec2SgId, err := ec2Client.SecurityGroupProvision(5 * time.Minute,
                                                     stateConfig.AwsEnv.Ec2SecurityGroupId,
                                                     vpcId, "kloud-kraken-ec2-security-group",
                                                     "Security group for Kloud" +
                                                     " Kraken EC2 instances", tags)
    if err != nil {
        return outStruct, err
    }

    // If the security group was created, add ID to yaml updates map
    if ec2SgId != "" {
        yamlUpdates["aws_env.ec2_security_group_id"] = ec2SgId
    // Otherwise use the one from YAML since it was found
    } else {
        ec2SgId = stateConfig.AwsEnv.Ec2SecurityGroupId
    }

    outStruct.Ec2SgId = ec2SgId

    // EC2 Security Group Rules setup
    //--------------------------------

    // Get the DNS address from the CIDR (Ex: 192.168.0.0/24 => 192.168.0.2/32)
    dnsAddr, err := ec2Client.VpcResolverForCidr(appConfig.LocalConfig.CidrBlock)
    if err != nil {
        return outStruct, err
    }

    // Configure UDP rule in security group for DNS
    err = ec2Client.SecurityGroupRuleProvision(1 * time.Minute, ec2SgId, dnsAddr,
                                               "udp", "egress", 53, 53)
    if err != nil {
        return outStruct, err
    }

    // Configure TCP rule in security group for DNS
    err = ec2Client.SecurityGroupRuleProvision(1 * time.Minute, ec2SgId, dnsAddr,
                                               "tcp", "egress", 53, 53)
    if err != nil {
        return outStruct, err
    }

    // Configure TCP rule in security group for HTTP
    err = ec2Client.SecurityGroupRuleProvision(1 * time.Minute, ec2SgId, "0.0.0.0/0",
                                               "tcp", "egress", 80, 80)
    if err != nil {
        return outStruct, err
    }

    // Configure TCP rule in security group for HTTPS
    err = ec2Client.SecurityGroupRuleProvision(1 * time.Minute, ec2SgId, "0.0.0.0/0",
                                               "tcp", "egress", 443, 443)
    if err != nil {
        return outStruct, err
    }

    // SSM Security Group setup
    //--------------------------
    tags = map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-ssm-security-group",
    }

    // Create SSM Parameter Store security group if it does not exist
    ssmSgId, err := ec2Client.SecurityGroupProvision(5 * time.Minute,
                                                     stateConfig.AwsEnv.SsmSecurityGroupId,
                                                     vpcId, "kloud-kraken-ssm-security-group",
                                                     "Security group for Kloud " +
                                                     "Kraken SSM parameter store" +
                                                     " VPC endpoint", tags)
    if err != nil {
        return outStruct, err
    }

    // If the security group was created, add ID to yaml updates map
    if ssmSgId != "" {
        yamlUpdates["aws_env.ssm_security_group_id"] = ssmSgId
    // Otherwise use the one from YAML since it was found
    } else {
        ssmSgId = stateConfig.AwsEnv.SsmSecurityGroupId
    }

    // SSM Security Group Rules setup
    //--------------------------------

    // Configure TCP rule in security group for HTTPS
    err = ec2Client.SecurityGroupRuleProvision(1 * time.Minute, ssmSgId, "0.0.0.0/0",
                                               "tcp", "ingress", 443, 443)
    if err != nil {
        return outStruct, err
    }

    // S3 Bucket setup
    //-----------------
    tags = map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-s3",
    }

    // Set up client to S3 service
    s3Client := s3utils.S3NewManager(awsConfig)
    // Create a S3 bucket if it does not exist
    bucketName, err := s3Client.S3BucketProvision(5 * time.Minute,
                                                  stateConfig.AwsEnv.S3BucketName,
                                                  "kloud-kraken-s3", tags)
    if err != nil {
        return outStruct, err
    }

    // If S3 buccket created, add name to yaml updates map
    if bucketName != "" {
        yamlUpdates["aws_env.s3_bucket_name"] = bucketName
    // Otherwise use the one from YAML since it was found
    } else {
        bucketName = stateConfig.AwsEnv.S3BucketName
    }

    outStruct.S3BucketName = bucketName

    // S3 VPC Gateway Endpoint setup
    //-------------------------------
    tags = map[string]string{
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
                                                          []string{privateRouteId}, tags)
    if err != nil {
        return outStruct, err
    }

    // If S3 VPC endpoint created, add name to yaml updates map
    if s3VpcEndPointId != "" {
        yamlUpdates["aws_env.s3_vpc_endpoint_id"] = s3VpcEndPointId
    // Otherwise use the one from YAML since it was found
    } else {
        s3VpcEndPointId = stateConfig.AwsEnv.S3VpcEndpointId
    }

    // SSM VPC Interface Endpoint setup
    //----------------------------------
    tags = map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-ssm-vpc-endpoint",
    }

    // Generate policy document for SSM VPC Endpoint
    policyDocument = policies.VpcSsmEndpointPolicyGen(outStruct.AccountId,
                                                      appConfig.LocalConfig.Region,
                                                      vpcId)

    // Create VPC endpoint for SSM if it does not exist
    ssmVpcEndpointId, err := ec2Client.SsmEndpointProvision(10 * time.Minute,
                                                            stateConfig.AwsEnv.SsmVpcEndpointId,
                                                            appConfig.LocalConfig.Region,
                                                            vpcId, policyDocument,
                                                            []string{privSubnetId},
                                                            []string{ssmSgId}, tags)
    if err != nil {
        return outStruct, err
    }

    // If SSM VPC endpoint was created, add name to yaml updates map
    if ssmVpcEndpointId != "" {
        yamlUpdates["aws_env.ssm_vpc_endpoint_id"] = ssmVpcEndpointId
    // Otherwise use the one from YAML since it was found
    } else {
        ssmVpcEndpointId = stateConfig.AwsEnv.SsmVpcEndpointId
    }

    outStruct.SsmVpcEndpointId = ssmVpcEndpointId

    // VPC Flow Logs IAM Role setup
    //------------------------------
    tags = map[string]string{
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
                                                     "vpc-flow-logs-role", trustPolicy,
                                                     "vpc-flow-log-permissions",
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
                                                 "client-role", trustPolicy,
                                                 "client-permissions",
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
                                                     bucketName, "client-role")
    // Create and apply role for local server permissions
    outStruct.ServerArn, err = iamClient.IamRoleProvision(5 * time.Minute,
                                                          stateConfig.AwsEnv.IamArnServer,
                                                          "server-role", trustPolicy,
                                                          "server-permissions",
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
