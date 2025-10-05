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
func SetupEc2SecurityGroupHandler(ec2Client *ec2utils.Ec2Manger,
                                  stateConfig *AwsEnv,
                                  appConfig *conf.AppConfig,
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


func SetupEc2SecurityGroupRulesHandler(ec2Client *ec2utils.Ec2Manger,
                                       stateConfig *AwsEnv,
                                       appConfig *conf.AppConfig,
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
func SetupSsmSecurityGroupHandler(ec2Client *ec2utils.Ec2Manger,
                                  stateConfig *AwsEnv,
                                  appConfig *conf.AppConfig,
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
func SetupSsmSecurityGroupRuleHandler(ec2Client *ec2utils.Ec2Manger,
                                      ssmSgId string) error {
    // Configure TCP rule in security group for HTTPS
    return ec2Client.SecurityGroupRuleProvision(1 * time.Minute, ssmSgId, "0.0.0.0/0",
                                                "tcp", "ingress", 443, 443)
}
