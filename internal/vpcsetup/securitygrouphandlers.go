package vpcsetup

import (
	"fmt"
	"time"

	"github.com/ngimb64/Kloud-Kraken/internal/color"
	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/pkg/display"
	"github.com/ngimb64/Kloud-Kraken/pkg/ec2utils"
)

// Handler function for setting up security group for EC2 instances.
//
// @Parameters
//  - ec2Client:  Pointer to EC2 service client management struct
//  - stateConfig:  Pointer to config struct for state file
//  - yamlUpdates:  The map used for updating output YAML data
//  - outStruct:  Pointer to struct used for managing vcpsetup outputs
//  - vpcId:  The ID of the VPC wthere the security group will apply
//
// @Returns
//  - EC2 security group ID
//  - Error if it occurs, otherwise nil on success
//
func SetupEc2SecurityGroupHandler(ec2Client *ec2utils.Ec2Manger,
                                  stateConfig *AwsEnv,
                                  yamlUpdates map[string]string,
                                  outStruct *VpcBootstrapOutput,
                                  vpcId string) (string, error) {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-ec2-security-group",
    }

    fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                   display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Launching EC2 security group provisioner"))

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

        fmt.Println(display.CtextMulti(color.FoamWhite, "      \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "EC2 security group was created"))
    // If security group already exists, use the existing ID
    } else {
        ec2SgId = stateConfig.AwsEnv.Ec2SecurityGroupId

        fmt.Println(display.CtextMulti(color.FoamWhite, "      \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "EC2 security group already exists"))
    }

    outStruct.Ec2SgId = ec2SgId
    return ec2SgId, nil
}


// Handler function for setting up security group rules for EC2 security group.
//
// @Parameters
//  - ec2Client:  Pointer to EC2 service client management struct
//  - appConfig:  Pointer to program config instance from YAML data
//  - ec2SgId:  The EC2 security group ID
//  - serverPort:  The port on the server the client initally connects to
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func SetupEc2SecurityGroupRulesHandler(ec2Client *ec2utils.Ec2Manger,
                                       appConfig *conf.AppConfig, ec2SgId string,
                                       serverPort int) error {
    fmt.Println(display.CtextMulti(color.FoamWhite, "      \\-->",
                                   display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Launching EC2 security group rules provisioner"))

    // Configure TCP rule for initial connection to server
    err := ec2Client.SecurityGroupRuleProvision(1 * time.Minute, ec2SgId, "0.0.0.0/0", "tcp",
                                                "ingress", int32(serverPort), int32(serverPort))
    if err != nil {
        return nil
    }

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

    fmt.Println(display.CtextMulti(color.FoamWhite, "          \\-->",
                                   display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "Provisioned EC2 security group rules"))

    return nil
}


// Handler function for setting up security group rules for SSM security group.
//
// @Parameters
//  - ec2Client:  Pointer to EC2 service client management struct
//  - stateConfig:  Pointer to config struct for state file
//  - yamlUpdates:  The map used for updating output YAML data
//  - vpcId:  The ID of the VPC wthere the security group will apply
//
// @Returns
//  - SSM security group ID
//  - Error if it occurs, otherwise nil on success
//
func SetupSsmSecurityGroupHandler(ec2Client *ec2utils.Ec2Manger,
                                  stateConfig *AwsEnv,
                                  yamlUpdates map[string]string,
                                  vpcId string) (string, error) {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-ssm-security-group",
    }

    fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                   display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Launching SSM security group provisioner"))

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

        fmt.Println(display.CtextMulti(color.FoamWhite, "      \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "SSM security group was created"))
    // If security group already exists, use the existing ID
    } else {
        ssmSgId = stateConfig.AwsEnv.SsmSecurityGroupId

        fmt.Println(display.CtextMulti(color.FoamWhite, "      \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "SSM security group already exists"))
    }

    return ssmSgId, nil
}


// Handler function for setting up the security group for SSM access
//
// @Parameters
//  - ec2Client:  Pointer to EC2 service client management struct
//  - ssmSgId:  The ID of the SSM security group
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func SetupSsmSecurityGroupRuleHandler(ec2Client *ec2utils.Ec2Manger,
                                      ssmSgId string) error {
    fmt.Println(display.CtextMulti(color.FoamWhite, "      \\-->",
                                   display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Launching SSM security group rules provisioner"))

    // Configure TCP rule in security group for HTTPS
    err := ec2Client.SecurityGroupRuleProvision(1 * time.Minute, ssmSgId, "0.0.0.0/0",
                                                "tcp", "ingress", 443, 443)
    if err != nil {
        return err
    }

    fmt.Println(display.CtextMulti(color.FoamWhite, "          \\-->",
                                   display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "$"), "",
                                   color.NeonAzure, "Provisioned SSM security group rules"))

    return nil
}
