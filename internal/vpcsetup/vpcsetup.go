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
	"github.com/ngimb64/Kloud-Kraken/pkg/yamlutils"
	"gopkg.in/yaml.v2"
)

type StateConfig struct {
    Ec2SecurityGroupId   string `yaml:"ec2_security_group_id"`
    EipId                string `yaml:"eip_id"`
    FlowLogId            string `yaml:"flow_log_id"`
    IgwId                string `yaml:"igw_id"`
    NatGatewayId         string `yaml:"nat_gateway_id"`
    PrivateAssociationId string `yaml:"private_association_id"`
    PrivateRouteId       string `yaml:"private_route_id"`
    PrivateSubnetId      string `yaml:"private_subnet_id"`
    PublicAssociationId  string `yaml:"public_association_id"`
    PublicRouteId        string `yaml:"public_route_id"`
    PublicSubnetId       string `yaml:"public_subnet_id"`
    S3BucketName         string `yaml:"s3_bucket_name"`
    S3VpcEndpointId      string `yaml:"s3_vpc_endpoint_id"`
    SsmVpcEndpointId     string `yaml:"ssm_vpc_endpoint_id"`
    SsmSecurityGroupId   string `yaml:"ssm_security_group_id"`
    VpcId                string `yaml:"vpc_id"`
}

type VpcBootstrapOutput struct {
    AccountId    string
    BucketName   string
    Ec2SgId	     string
    PrivSubnetId string
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
    stateFilePath := "../.kraken-state.yml"
    var stateConfig StateConfig
    var stateData []byte
    var yamlUpdates map[string]string
    outStruct := &VpcBootstrapOutput{}

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
            err = errors.Join(err, fmt.Errorf("updating yaml:  %w", yerr))
            return
        }

        // Overwrite the original yaml with the updated data
        werr := os.WriteFile(stateFilePath, newYaml, 0644)
        if werr != nil {
            err = errors.Join(err, fmt.Errorf("writing output yaml:  %w", werr))
        }
    }()

    // Check to see if the VPC exists, otherwise create one
    vpcId, err := ec2Client.VpcProvision(20*time.Minute,
                                         stateConfig.VpcId,
                                         appConfig.LocalConfig.CidrBlock,
                                         "Kloud-Kraken-VPC")
    if err != nil {
        return outStruct, err
    }

    // If a VPC was created, add ID to yaml updates map
    if vpcId != "" {
        yamlUpdates["aws_env.vpc_id"] = vpcId
    // Otherwise use the one from YAML since it was found
    } else {
        vpcId = stateConfig.VpcId
    }

    // Check to see if IGW exists, otherwise create & attach one
    igwId, err := ec2Client.InternetGatewayProvision(10*time.Minute,
                                                     stateConfig.IgwId, vpcId,
                                                     "Kloud-Kraken-IGW")
    if err != nil {
        return outStruct, err
    }

    // If a Internet Gateway was created, add ID to yaml updates map
    if igwId != "" {
        yamlUpdates["aws_env.igw_id"] = igwId
    // Otherwise use the one from YAML since it was found
    } else {
        igwId = stateConfig.IgwId
    }

    // Check to see if Elastic IP exists, otherwise create one
    eipId, err := ec2Client.ElasticIpProvision(10*time.Minute, stateConfig.EipId)
    if err != nil {
        return outStruct, err
    }

    // If a Elastic IP was created, add ID to yaml updates map
    if eipId != "" {
        yamlUpdates["aws_env.eip_id"] = eipId
    // Otherwise use the one from YAML since it was found
    } else {
        eipId = stateConfig.EipId
    }

    // Get the slice of availability zones based on region
    azs, err := ec2Client.FetchAvailableAZs(5 * time.Minute)
    if err != nil {
        return outStruct, err
    }

    // Pick random AZ from slice of AZ names
    az := awsutils.PickAzRandom(azs)

    // Set up map for ensuring unique subnet allocation
    alloc := map[string]struct{}{}

    // Parse the prefix length from CIDR
    prefixLength, err := cidrutils.PrefixFromCidr(appConfig.LocalConfig.CidrBlock)
    if err != nil {
        return outStruct, err
    }

    // Allocate first available subnet in CIDR block for public subnet
    pubCidr, err := cidrutils.AllocateNextSubnet(appConfig.LocalConfig.CidrBlock,
                                                 alloc, prefixLength+1)
    if err != nil {
        return outStruct, err
    }

    // Create public subnet if it does not exist
    pubSubnetId, err := ec2Client.SubnetProvision(5 * time.Minute,
                                                  stateConfig.PublicSubnetId,
                                                  vpcId, pubCidr, az, true)
    if err != nil {
        return outStruct, err
    }

    // If a public subnet was created, add ID to yaml updates map
    if pubSubnetId != "" {
        yamlUpdates["aws_env.public_subnet_id"] = pubSubnetId
    // Otherwise use the one from YAML since it was found
    } else {
        pubSubnetId = stateConfig.PublicSubnetId
    }

    // Allocate next available subnet in CIDR block for private subnet
    privCidr, err := cidrutils.AllocateNextSubnet(appConfig.LocalConfig.CidrBlock,
                                                  alloc, prefixLength+1)
    if err != nil {
        return outStruct, err
    }

    // Create private subnet if it does not exist
    privSubnetId, err := ec2Client.SubnetProvision(5 * time.Minute,
                                                   stateConfig.PrivateSubnetId,
                                                   vpcId, privCidr, az, false)
    if err != nil {
        return outStruct, err
    }

    // If a private subnet was created, add ID to yaml updates map
    if privSubnetId != "" {
        yamlUpdates["aws_env.private_subnet_id"] = privSubnetId
    // Otherwise use the one from YAML since it was found
    } else {
        privSubnetId = stateConfig.PrivateSubnetId
    }

    outStruct.PrivSubnetId = privSubnetId

    // Create NAT gateway in public subnet if it does not exist
    natGatewayId, err := ec2Client.NatGatewayProvision(10 * time.Minute,
                                                       stateConfig.NatGatewayId,
                                                       pubSubnetId, eipId,
                                                       "Kloud-Kraken-NAT-Gateway")
    if err != nil {
        return outStruct, err
    }

    // If a NAT Gateway was created, add ID to yaml updates map
    if natGatewayId != "" {
        yamlUpdates["aws_env.nat_gateway_id"] = natGatewayId
    // Otherwise use the one from YAML since it was found
    } else {
        natGatewayId = stateConfig.NatGatewayId
    }

    // Create route table for subnets to internet gateway if does not exist
    publicRouteId, err := ec2Client.RouteTableProvision(5 * time.Minute,
                                                        stateConfig.PublicRouteId,
                                                        vpcId, igwId, "",
                                                        pubSubnetId, "0.0.0.0/0",
                                                        "Kloud-Kraken-Public-Route")
    if err != nil {
        return outStruct, err
    }

    // If the public route table was created, add ID to yaml updates map
    if publicRouteId != "" {
        yamlUpdates["aws_env.public_route_id"] = publicRouteId
    // Otherwise use the one from YAML since it was found
    } else {
        publicRouteId = stateConfig.PublicRouteId
    }

    // Create route table for subnets to NAT Gateway if it does not exist
    privateRouteId, err := ec2Client.RouteTableProvision(5 * time.Minute,
                                                         stateConfig.PrivateRouteId,
                                                         vpcId, "", natGatewayId,
                                                         privSubnetId, "0.0.0.0/0",
                                                         "Kloud-Kraken-Private-Route")
    if err != nil {
        return outStruct, err
    }

    // If the private route table was created, add ID to yaml updates map
    if privateRouteId != "" {
        yamlUpdates["aws_env.private_route_id"] = privateRouteId
    // Otherwise use the one from YAML since it was found
    } else {
        privateRouteId = stateConfig.PrivateRouteId
    }

    // Ensure public route tables are associated to subnet
    publicAssocId, err := ec2Client.RouteTableAssociationProvision(5 * time.Minute,
                                                                   stateConfig.PublicAssociationId,
                                                                   publicRouteId, pubSubnetId)
    if err != nil {
        return outStruct, err
    }

    // If the public association occured, add ID to yaml updates map
    if publicAssocId != "" {
        yamlUpdates["aws_env.public_association_id"] = publicAssocId
    // Otherwise use the one from YAML since it was found
    } else {
        publicAssocId = stateConfig.PublicAssociationId
    }

    // Ensure private route tables are associated to subnet
    privateAssocId, err := ec2Client.RouteTableAssociationProvision(5 * time.Minute,
                                                                    stateConfig.PrivateAssociationId,
                                                                    privateRouteId, privSubnetId)
    if err != nil {
        return outStruct, err
    }

    // If the private association occured, add ID to yaml updates map
    if privateAssocId != "" {
        yamlUpdates["aws_env.private_association_id"] = privateAssocId
    // Otherwise use the one from YAML since it was found
    } else {
        privateAssocId = stateConfig.PrivateAssociationId
    }

    // Create EC2 security group if it does not exist
    ec2SgId, err := ec2Client.SecurityGroupProvision(5 * time.Minute,
                                                     stateConfig.Ec2SecurityGroupId,
                                                     vpcId, "Kloud-Kraken-EC2-SG",
                                                     "Security group for Kloud" +
                                                     " Kraken EC2 instances")
    if err != nil {
        return outStruct, err
    }

    // If the security group was created, add ID to yaml updates map
    if ec2SgId != "" {
        yamlUpdates["aws_env.ec2_security_group_id"] = ec2SgId
    // Otherwise use the one from YAML since it was found
    } else {
        ec2SgId = stateConfig.Ec2SecurityGroupId
    }

    outStruct.Ec2SgId = ec2SgId

    // Get the DNS address from the CIDR (Ex: 192.168.0.0/24 => 192.168.0.2/32)
    dnsAddr, err := ec2Client.VpcResolverForCidr(appConfig.LocalConfig.CidrBlock)
    if err != nil {
        return outStruct, err
    }

    // Configure UDP rule in security group for DNS
    err = ec2Client.SecurityGroupRuleProvision(5 * time.Minute, ec2SgId, dnsAddr,
                                               "udp", "egress", 53, 53)
    if err != nil {
        return outStruct, err
    }

    // Configure TCP rule in security group for DNS
    err = ec2Client.SecurityGroupRuleProvision(5 * time.Minute, ec2SgId, dnsAddr,
                                               "tcp", "egress", 53, 53)
    if err != nil {
        return outStruct, err
    }

    // Configure TCP rule in security group for HTTP
    err = ec2Client.SecurityGroupRuleProvision(5 * time.Minute, ec2SgId, "0.0.0.0/0",
                                               "tcp", "egress", 80, 80)
    if err != nil {
        return outStruct, err
    }

    // Configure TCP rule in security group for HTTPS
    err = ec2Client.SecurityGroupRuleProvision(5 * time.Minute, ec2SgId, "0.0.0.0/0",
                                               "tcp", "egress", 443, 443)
    if err != nil {
        return outStruct, err
    }

    // Create SSM Parameter Store security group if it does not exist
    ssmSgId, err := ec2Client.SecurityGroupProvision(5 * time.Minute,
                                                     stateConfig.SsmSecurityGroupId,
                                                     vpcId, "Kloud-Kraken-SSM-SG",
                                                     "Security group for Kloud " +
                                                     "Kraken SSM parameter store" +
                                                     " VPC endpoint")
    if err != nil {
        return outStruct, err
    }

    // If the security group was created, add ID to yaml updates map
    if ssmSgId != "" {
        yamlUpdates["aws_env.ssm_security_group_id"] = ssmSgId
    // Otherwise use the one from YAML since it was found
    } else {
        ssmSgId = stateConfig.SsmSecurityGroupId
    }

    // Configure TCP rule in security group for HTTPS
    err = ec2Client.SecurityGroupRuleProvision(5 * time.Minute, ssmSgId, "0.0.0.0/0",
                                               "tcp", "ingress", 443, 443)
    if err != nil {
        return outStruct, err
    }

    // Create VPC endpoint for S3 if it does not exist
    s3VpcEndPointId, err := ec2Client.S3EndpointProvision(5 * time.Minute,
                                                          stateConfig.S3VpcEndpointId,
                                                          appConfig.LocalConfig.Region,
                                                          vpcId, []string{privateRouteId})
    if err != nil {
        return outStruct, err
    }

    // If S3 VPC endpoint created, add name to yaml updates map
    if s3VpcEndPointId != "" {
        yamlUpdates["aws_env.s3_vpc_endpoint"] = s3VpcEndPointId
    // Otherwise use the one from YAML since it was found
    } else {
        s3VpcEndPointId = stateConfig.S3VpcEndpointId
    }

    // Create VPC endpoint for SSM if it does not exist
    ssmVpcEndpointId, err := ec2Client.SsmEndpointProvision(5 * time.Minute,
                                                            stateConfig.SsmVpcEndpointId,
                                                            appConfig.LocalConfig.Region,
                                                            vpcId, []string{privSubnetId},
                                                            []string{ssmSgId})
    if err != nil {
        return outStruct, err
    }

    // If SSM VPC endpoint was created, add name to yaml updates map
    if ssmVpcEndpointId != "" {
        yamlUpdates["aws_env.ssm_vpc_endpoint_id"] = ssmVpcEndpointId
    // Otherwise use the one from YAML since it was found
    } else {
        ssmVpcEndpointId = stateConfig.SsmVpcEndpointId
    }

    // Set up client to S3 service
    s3Client := s3utils.S3NewManager(awsConfig)

    bucketName, err := s3Client.S3BucketProvision(5 * time.Minute,
                                                  stateConfig.S3BucketName,
                                                  "Kloud-Kraken-S3")
    if err != nil {
        return outStruct, err
    }

    // If S3 buccket created, add name to yaml updates map
    if bucketName != "" {
        yamlUpdates["aws_env.s3_bucket_name"] = bucketName
    // Otherwise use the one from YAML since it was found
    } else {
        bucketName = stateConfig.S3BucketName
    }

    outStruct.BucketName = bucketName

    // Get the account ID associated with API credentials
    outStruct.AccountId, err = awsutils.GetAccountID(1 * time.Minute, stsClient)
    if err != nil {
        return outStruct, err
    }

    // Generate the VPC Flow Logs trust and permissions policy templates
    trustPolicy := policies.VpcFlowLogsTrustPolicyGen()
    permissionsPolicy := policies.VpcFlowLogsPermPolicyGen()
    // Create and appy the VPC flow logs role
    vpcFlowLogArn, err := iamClient.IamRoleCreation(5 * time.Minute,
                                                    "VpcFlowLogsRole",
                                                    trustPolicy,
                                                    "VpcFlowLogPermissions",
                                                    permissionsPolicy, false)
    if err != nil {
        return outStruct, err
    }

    // Set up client to CloudWatch Logs
    cwlClient := cwl.NewFromConfig(awsConfig)

    // Create and enable the VPC Flow Logs via CloudWatch if it does not exist
    flowLogId, err := ec2Client.VpcFlowLogProvision(5 * time.Minute,
                                                    stateConfig.FlowLogId,
                                                    vpcId, cwlClient,
                                                    "Kloud-Kraken-VPC-Flow-Logs",
                                                    vpcFlowLogArn)
    if err != nil {
        return outStruct, err
    }

    // If VPC Flow Logs group was created, add ID to yaml updates map
    if flowLogId != "" {
        yamlUpdates["aws_env.flow_log_id"] = flowLogId
    // Otherwise use the one from YAML since it was found
    } else {
        flowLogId = stateConfig.FlowLogId
    }

    return outStruct, nil
}
