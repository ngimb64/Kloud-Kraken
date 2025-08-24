package ec2utils

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) securityGroupRuleCreate(callTime time.Duration,
                                                 sgId string, cidr string,
                                                 direction string,
                                                 minPort int32,
                                                 maxPort int32) error {
	// Ensure required args are present
	if sgId == "" || cidr == "" {
		return fmt.Errorf("sgId or cidr is missing")
	}

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    ipPerm := ec2types.IpPermission{
        IpProtocol: aws.String("tcp"),
        FromPort:   aws.Int32(minPort),
        ToPort:     aws.Int32(maxPort),
        IpRanges: []ec2types.IpRange{
            {CidrIp: aws.String(cidr)},
        },
    }

    switch direction {
    case "egress":
        callInput := &ec2.AuthorizeSecurityGroupEgressInput{
            GroupId: aws.String(sgId),
            IpPermissions: []ec2types.IpPermission{ipPerm},
        }

        // Add new egress rule to security group
        _, err := Ec2Man.client.AuthorizeSecurityGroupEgress(ctx, callInput)
        if err != nil {
            return fmt.Errorf("authorize security group egress - %w", err)
        }

    case "ingress":
        callInput := &ec2.AuthorizeSecurityGroupIngressInput{
            GroupId: aws.String(sgId),
            IpPermissions: []ec2types.IpPermission{ipPerm},
        }

        // Add new ingress rule to security group
        _, err := Ec2Man.client.AuthorizeSecurityGroupIngress(ctx, callInput)
        if err != nil {
            return fmt.Errorf("authorize security group egress - %w", err)
        }

    default:
        return fmt.Errorf("improper direction specified, use egress or ingress")
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
func (Ec2Man *Ec2Manger) SecurityGroupRuleExists(callTime time.Duration,
                                                 sgId string, cidr string,
                                                 proto string, minPort int32,
                                                 maxPort int32,) (
                                                 bool, error) {
    // Ensure required args are present
    if sgId == "" || cidr == "" || proto == "" {
        return false, fmt.Errorf("sgId or cidr or protocol is missing")
    }

    // Ensure protocol is all lowercase
    proto = strings.ToLower(proto)
    // If the protocol is not proper format
    if proto != "tcp" && proto != "udp" && proto != "-1" && proto != "all" {
        return false, fmt.Errorf("unsupported protocol - %q", proto)
    }

    // Validate TCP/UDP ports
    if proto == "tcp" || proto == "udp" {
        // Ensure min/max ports are above 0 and the min is above max
        if minPort <= 0 || maxPort <= 0 || minPort > maxPort {
            return false, fmt.Errorf("invalid port range: %d-%d",
                                     minPort, maxPort)
        }
    }

    // Ensure API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.DescribeSecurityGroupsInput{
        GroupIds: []string{sgId},
    }

    // Get the Security Group based on passed in ID
    out, err := Ec2Man.client.DescribeSecurityGroups(ctx, callInput)
    if err != nil {
        var apiErr smithy.APIError

        // If the Security Group ID was not found
        if errors.As(err, &apiErr) &&
        apiErr.ErrorCode() == "InvalidGroup.NotFound" {
            return false, nil
        }

        return false, fmt.Errorf("describe security groups - %w", err)
    }

    // If no security groups were retrieved
    if out == nil || len(out.SecurityGroups) == 0 {
        return false, nil
    }

    // Iterate through security groups1
    for _, sg := range out.SecurityGroups {
        // Iterate through rules in each security group
        for _, perm := range sg.IpPermissions {
            // If the protocol is empty, skip to next rule
            if perm.IpProtocol == nil {
                continue
            }

            permProto := *perm.IpProtocol

            // If permission is "all" (-1) treat as match if CIDR present
            if (permProto == "-1" || permProto == "all") &&
            (proto == "-1" || proto == "all" || proto == "tcp" || proto == "udp") {
                // Iterate through the IP ranges in the outbound rules
                for _, ipr := range perm.IpRanges {
                    // If the network CIDR is present and matches arg
                    if ipr.CidrIp != nil && *ipr.CidrIp == cidr {
                        return true, nil
                    }
                }

                continue
            }

            // Require protocol to match arg
            if permProto != proto {
                continue
            }

            // Ensure to and from ports are present
            if perm.FromPort == nil || perm.ToPort == nil {
                continue
            }

            from := *perm.FromPort
            to := *perm.ToPort

            // If the to and from ports are within IP range
            if from <= minPort && to >= maxPort {
                // Iterate through outbound rules IP ranges
                for _, ipr := range perm.IpRanges {
                    // If the CIDR IP is present and matches args
                    if ipr.CidrIp != nil && *ipr.CidrIp == cidr {
                        return true, nil
                    }
                }
            }
        }
    }

    return false, nil
}


//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) SecurityGroupRuleProvision(callTime time.Duration,
                                                    sgId string, cidr string,
                                                    proto string, direction string,
                                                    minPort int32,
                                                    maxPort int32) error {
    // If Security Group ID is present in YAML
    if sgId != "" {
        exists, err := Ec2Man.SecurityGroupRuleExists(callTime, sgId, cidr,
                                                      proto, minPort, maxPort)
        if err != nil {
            return err
        }

        // If the Security Group already exists, exit early
        if exists {
            return nil
        }
    }

    // Create egress security group
    err := Ec2Man.securityGroupRuleCreate(callTime, sgId, cidr,
                                          direction, minPort, maxPort)
    if err != nil {
        return err
    }

    return nil
}
