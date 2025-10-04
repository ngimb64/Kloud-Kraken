package vpcsetup

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/internal/policies"
	"github.com/ngimb64/Kloud-Kraken/pkg/awscost"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/cidrutils"
	"github.com/ngimb64/Kloud-Kraken/pkg/ec2utils"
	"github.com/ngimb64/Kloud-Kraken/pkg/s3utils"
)

//
//
// @Parameters
//
//
// @Returns
//
//
func SetupEc2SecurityGroupHandler(ec2Client ec2utils.Ec2Manger,
                                  stateConfig AwsEnv,
                                  appConfig conf.AppConfig,
                                  yamlUpdates map[string]string,
                                  outStruct *VpcBootstrapOutput,
                                  vpcId string) (string, error) {
    tags := map[string]string{
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
        return ec2SgId, err
    }

    // If the security group was created, add ID to yaml updates map
    if ec2SgId != "" {
        yamlUpdates["aws_env.ec2_security_group_id"] = ec2SgId
    // Otherwise use the one from YAML since it was found
    } else {
        ec2SgId = stateConfig.AwsEnv.Ec2SecurityGroupId
    }

    outStruct.Ec2SgId = ec2SgId
    return ec2SgId, nil
}


func SetupEc2SecurityGroupRulesHandler(ec2Client ec2utils.Ec2Manger,
                                       stateConfig AwsEnv,
                                       appConfig conf.AppConfig,
                                       yamlUpdates map[string]string,
                                       ec2SgId string) error {
    // Get the DNS address from the CIDR (Ex: 192.168.0.0/24 => 192.168.0.2/32)
    dnsAddr, err := ec2Client.VpcResolverForCidr(appConfig.LocalConfig.CidrBlock)
    if err != nil {
        return err
    }

    // Configure UDP rule in security group for DNS
    err = ec2Client.SecurityGroupRuleProvision(1 * time.Minute, ec2SgId, dnsAddr,
                                               "udp", "egress", 53, 53)
    if err != nil {
        return err
    }

    // Configure TCP rule in security group for DNS
    err = ec2Client.SecurityGroupRuleProvision(1 * time.Minute, ec2SgId, dnsAddr,
                                               "tcp", "egress", 53, 53)
    if err != nil {
        return err
    }

    // Configure TCP rule in security group for HTTP
    err = ec2Client.SecurityGroupRuleProvision(1 * time.Minute, ec2SgId, "0.0.0.0/0",
                                               "tcp", "egress", 80, 80)
    if err != nil {
        return err
    }

    // Configure TCP rule in security group for HTTPS
    err = ec2Client.SecurityGroupRuleProvision(1 * time.Minute, ec2SgId, "0.0.0.0/0",
                                               "tcp", "egress", 443, 443)
    if err != nil {
        return err
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
func SetupElasticIpHandler(ec2Client ec2utils.Ec2Manger,
                           stateConfig AwsEnv,
                           appConfig conf.AppConfig,
                           yamlUpdates map[string]string,
                           outStruct *VpcBootstrapOutput,
                           location string, costErr *error,
                           costMan *awscost.AwsCostManager) (
                           string, error) {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-elastic-ip",
    }

    // Check to see if Elastic IP exists, otherwise create one
    eipId, err := ec2Client.ElasticIpProvision(1 * time.Minute,
                                               stateConfig.AwsEnv.EipId,
                                               tags)
    if err != nil {
        return "", err
    }

    // If a Elastic IP was created, add ID to yaml updates map
    if eipId != "" {
        yamlUpdates["aws_env.eip_id"] = eipId
    // Otherwise use the one from YAML since it was found
    } else {
        eipId = stateConfig.AwsEnv.EipId
    }

    outStruct.EipId = eipId

    filterMap := map[string]string{
        "location": location,
    }

    // Add the elastic IP to cost manager
    _ = costMan.AddCostResourceToManagerHandler("elastic_ip", filterMap, costErr)
    return eipId, nil
}


//
//
// @Parameters
//
//
// @Returns
//
//
func SetupInternetGatewayHandler(ec2Client ec2utils.Ec2Manger,
                                 stateConfig AwsEnv,
                                 appConfig conf.AppConfig,
                                 yamlUpdates map[string]string,
                                 vpcId string) (
                                 string, error) {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-internet-gateway",
    }

    // Check to see if IGW exists, otherwise create & attach one
    igwId, err := ec2Client.InternetGatewayProvision(5 * time.Minute,
                                                     stateConfig.AwsEnv.IgwId,
                                                     vpcId, tags)
    if err != nil {
        return "", err
    }

    // If a Internet Gateway was created, add ID to yaml updates map
    if igwId != "" {
        yamlUpdates["aws_env.igw_id"] = igwId
    // Otherwise use the one from YAML since it was found
    } else {
        igwId = stateConfig.AwsEnv.IgwId
    }

    return igwId, nil
}


//
//
// @Parameters
//
//
// @Returns
//
//
func SetupNatGatewayHandler(ec2Client ec2utils.Ec2Manger,
                            stateConfig AwsEnv,
                            appConfig conf.AppConfig,
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
    _ = costMan.AddCostResourceToManagerHandler("nat_gateway", filterMap, costErr)
    return natGatewayId, nil
}


//
//
// @Parameters
//
//
// @Returns
//
//
func SetupRouteTableAssociationsHandler(ec2Client ec2utils.Ec2Manger,
                                        stateConfig AwsEnv,
                                        appConfig conf.AppConfig,
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
func SetupRouteTablesHandler(ec2Client ec2utils.Ec2Manger,
                             stateConfig AwsEnv,
                             appConfig conf.AppConfig,
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


//
//
// @Parameters
//
//
// @Returns
//
//
func SetupS3BucketHandler(ec2Client ec2utils.Ec2Manger,
                          stateConfig AwsEnv,
                          appConfig conf.AppConfig,
                          yamlUpdates map[string]string,
                          outStruct *VpcBootstrapOutput,
                          awsConfig aws.Config) (
                          string, error) {
    tags := map[string]string{
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
        return bucketName, err
    }

    // If S3 buccket created, add name to yaml updates map
    if bucketName != "" {
        yamlUpdates["aws_env.s3_bucket_name"] = bucketName
    // Otherwise use the one from YAML since it was found
    } else {
        bucketName = stateConfig.AwsEnv.S3BucketName
    }

    outStruct.S3BucketName = bucketName
    return bucketName, nil
}


//
//
// @Parameters
//
//
// @Returns
//
//
func SetupS3VpcGatewayEndpointHandler(ec2Client ec2utils.Ec2Manger,
                                      stateConfig AwsEnv,
                                      appConfig conf.AppConfig,
                                      yamlUpdates map[string]string,
                                      bucketName string, vpcId string,
                                      privateRouteId string,
                                      location string, costErr *error,
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
                                                          []string{privateRouteId}, tags)
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

    filterMap := map[string]string{
        "location": location,
    }

    // Add the elastic IP to cost manager
    _ = costMan.AddCostResourceToManagerHandler("vpc_endpoint_s3", filterMap, costErr)
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
func SetupSsmSecurityGroupHandler(ec2Client ec2utils.Ec2Manger,
                                  stateConfig AwsEnv,
                                  appConfig conf.AppConfig,
                                  yamlUpdates map[string]string,
                                  vpcId string) (string, error) {
    tags := map[string]string{
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
        return ssmSgId, err
    }

    // If the security group was created, add ID to yaml updates map
    if ssmSgId != "" {
        yamlUpdates["aws_env.ssm_security_group_id"] = ssmSgId
    // Otherwise use the one from YAML since it was found
    } else {
        ssmSgId = stateConfig.AwsEnv.SsmSecurityGroupId
    }

    return ssmSgId, nil
}


//
//
// @Parameters
//
//
// @Returns
//
//
func SetupSsmSecurityGroupRuleHandler(ec2Client ec2utils.Ec2Manger,
                                      ssmSgId string) error {
    // Configure TCP rule in security group for HTTPS
    return ec2Client.SecurityGroupRuleProvision(1 * time.Minute, ssmSgId, "0.0.0.0/0",
                                                "tcp", "ingress", 443, 443)
}


//
//
// @Parameters
//
//
// @Returns
//
//
func SetupSsmVpcInterfaceEndpointHandler(ec2Client ec2utils.Ec2Manger,
                                         stateConfig AwsEnv,
                                         appConfig conf.AppConfig,
                                         yamlUpdates map[string]string,
                                         outStruct *VpcBootstrapOutput,
                                         vpcId string, privSubnetId string,
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
                                                            appConfig.LocalConfig.Region,
                                                            vpcId, policyDocument,
                                                            []string{privSubnetId},
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
    _ = costMan.AddCostResourceToManagerHandler("vpc_endpoint_ssm", filterMap, costErr)
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
func SetupSubnetsHandler(ec2Client ec2utils.Ec2Manger,
                         stateConfig AwsEnv,
                         appConfig conf.AppConfig,
                         yamlUpdates map[string]string,
                         outStruct *VpcBootstrapOutput,
                         vpcId string) (
                         string, string, error) {
    // Get the slice of availability zones based on region
    azs, err := ec2Client.FetchAvailableAZs(1 * time.Minute)
    if err != nil {
        return "", "", err
    }

    // Pick random AZ from slice of AZ names
    az := awsutils.PickAzRandom(azs)

    // Set up map for ensuring unique subnet allocation
    subnetMap := map[string]struct{}{}

    // Parse the prefix length from CIDR
    prefixLength, err := cidrutils.PrefixFromCidr(appConfig.LocalConfig.CidrBlock)
    if err != nil {
        return "", "", err
    }

    // Allocate first available subnet in CIDR block for public subnet
    pubCidr, err := cidrutils.AllocateNextSubnet(appConfig.LocalConfig.CidrBlock,
                                                 subnetMap, prefixLength + 1)
    if err != nil {
        return "", "", err
    }

    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-public-subnet",
    }

    // Create public subnet if it does not exist
    pubSubnetId, err := ec2Client.SubnetProvision(5 * time.Minute,
                                                  stateConfig.AwsEnv.PublicSubnetId,
                                                  vpcId, pubCidr, az, tags, true)
    if err != nil {
        return pubSubnetId, "", err
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
        return pubSubnetId, "", err
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
        return pubSubnetId, privSubnetId, err
    }

    // If a private subnet was created, add ID to yaml updates map
    if privSubnetId != "" {
        yamlUpdates["aws_env.private_subnet_id"] = privSubnetId
    // Otherwise use the one from YAML since it was found
    } else {
        privSubnetId = stateConfig.AwsEnv.PrivateSubnetId
    }

    outStruct.PrivSubnetId = privSubnetId
    return pubSubnetId, privSubnetId, nil
}


//
//
// @Parameters
//
//
// @Returns
//
//
func SetupVpcHandler(ec2Client ec2utils.Ec2Manger,
                     stateConfig AwsEnv,
                     appConfig conf.AppConfig,
                     yamlUpdates map[string]string) (
                     string, error) {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name":         "kloud-kraken-vpc",
    }

    // Check to see if the VPC exists, otherwise create one
    vpcId, err := ec2Client.VpcProvision(10*time.Minute,
                                         stateConfig.AwsEnv.VpcId,
                                         appConfig.LocalConfig.CidrBlock,
                                         tags)
    if err != nil {
        return "", err
    }

    // If a VPC was created, add ID to yaml updates map
    if vpcId != "" {
        yamlUpdates["aws_env.vpc_id"] = vpcId
        // Otherwise use the one from YAML since it was found
    } else {
        vpcId = stateConfig.AwsEnv.VpcId
    }

    return vpcId, nil
}
