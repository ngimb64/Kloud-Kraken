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

// Create security group rule in passed in security group ID.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - sgId:  The security group that the rule will be applied to
//  - proto:  The network protocol the security group rule will apply to
//  - cidr:  The CIDR network that the security group rule will apply to
//  - direction:  The network traffic direction the security group rule will control
//  - minPort:  The starting point in the range of ports to apply
//  - maxPort:  The end point in the range of ports to apply
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) securityGroupRuleCreate(callTime time.Duration,
                                                 sgId string, proto string,
                                                 cidr string, direction string,
                                                 minPort int32, maxPort int32,
                                                 ) error {
    // Ensure required args are present
    if sgId == "" || proto == "" || cidr == "" {
        return errors.New("sgId or proto or cidr is missing")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    ipPerm := ec2types.IpPermission{
        IpProtocol: aws.String(proto),
        IpRanges: []ec2types.IpRange{
            {
                CidrIp: aws.String(cidr),
            },
        },
        FromPort:   aws.Int32(minPort),
        ToPort:     aws.Int32(maxPort),
    }

    switch direction {
    case "egress":
        callInput := &ec2.AuthorizeSecurityGroupEgressInput{
            GroupId: aws.String(sgId),
            IpPermissions: []ec2types.IpPermission{ipPerm},
        }

        // Add new egress rule to security group
        _, err := Ec2Man.Client.AuthorizeSecurityGroupEgress(ctx, callInput)
        if err != nil {
            return fmt.Errorf("authorize security group egress - %w", err)
        }

    case "ingress":
        callInput := &ec2.AuthorizeSecurityGroupIngressInput{
            GroupId: aws.String(sgId),
            IpPermissions: []ec2types.IpPermission{ipPerm},
        }

        // Add new ingress rule to security group
        _, err := Ec2Man.Client.AuthorizeSecurityGroupIngress(ctx, callInput)
        if err != nil {
            return fmt.Errorf("authorize security group ingress - %w", err)
        }

    default:
        return errors.New("improper direction specified, use egress or ingress")
    }

    return nil
}


// Check whether security group rule exists.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - sgId:  The security group that the rule will be applied to
//  - cidr:  The CIDR network that the security group rule will apply to
//  - proto:  The network protocol the security group rule will apply to
//  - minPort:  The starting point in the range of ports to apply
//  - maxPort:  The end point in the range of ports to apply
//
// @Returns
//  - Toggle for whether Route Table already exists or not
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) SecurityGroupRuleExists(callTime time.Duration,
                                                 sgId string, cidr string,
                                                 proto string, minPort int32,
                                                 maxPort int32) (
                                                 bool, error) {
    // Ensure required args are present
    if sgId == "" || cidr == "" || proto == "" {
        return false, errors.New("sgId or cidr or proto is missing")
    }

    // Ensure protocol is all lowercase
    proto = strings.ToLower(strings.TrimSpace(proto))
    if proto == "all" {
        proto = "-1"
    }

    // Ensure supported protocol is present
    if proto != "tcp" && proto != "udp" && proto != "-1" {
        return false, fmt.Errorf("unsupported protocol - %q", proto)
    }

    // Ensure TCP/UDP ports are in proper range
    if proto == "tcp" || proto == "udp" {
        if minPort <= 0 || maxPort <= 0 || minPort > maxPort {
            return false, fmt.Errorf("invalid port range: %d-%d", minPort, maxPort)
        }
    }

    // Ensure API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Get the Security Group based on passed in ID
    out, err := Ec2Man.Client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
        GroupIds: []string{sgId},
    })
    if err != nil {
        var apiErr smithy.APIError

        if errors.As(err, &apiErr) && apiErr.ErrorCode() == "InvalidGroup.NotFound" {
            return false, nil
        }

        return false, fmt.Errorf("describe security groups - %w", err)
    }

    if out == nil || len(out.SecurityGroups) == 0 {
        return false, nil
    }

    // Detect if cidr is IPv6
    isV6 := strings.Contains(cidr, ":")

    // Compare IP ranges inside a permission
    matchIPRanges := func(p ec2types.IpPermission) bool {
        if isV6 {
            for _, r := range p.Ipv6Ranges {
                if r.CidrIpv6 != nil && *r.CidrIpv6 == cidr {
                    return true
                }
            }
        } else {
            for _, r := range p.IpRanges {
                if r.CidrIp != nil && *r.CidrIp == cidr {
                    return true
                }
            }
        }

        return false
    }

    // Check slice of permissions (works for ingress or egress)
    checkPerms := func(perms []ec2types.IpPermission) bool {
        for _, perm := range perms {
            if perm.IpProtocol == nil {
                continue
            }

            permProto := strings.ToLower(*perm.IpProtocol)

            // permProto "-1" (all protocols) should match tcp/udp/all requests
            if permProto == "-1" {
                // if user wants any protocol or TCP/UDP, an "all" permission matches
                if proto == "-1" || proto == "tcp" || proto == "udp" {
                    if matchIPRanges(perm) {
                        return true
                    }
                }

                continue
            }

            // Direct protocol match required
            if permProto != proto {
                continue
            }

            // For tcp/udp ensure ports present and encompass requested range
            if proto == "tcp" || proto == "udp" {
                if perm.FromPort == nil || perm.ToPort == nil {
                    continue
                }

                from := *perm.FromPort
                to := *perm.ToPort

                if from <= minPort && to >= maxPort {
                    if matchIPRanges(perm) {
                        return true
                    }
                }

                continue
            }
        }

        return false
    }

    for _, sg := range out.SecurityGroups {
        // Check ingress permissions
        if checkPerms(sg.IpPermissions) {
            return true, nil
        }

        // Check egress permissions
        if checkPerms(sg.IpPermissionsEgress) {
            return true, nil
        }
    }

    return false, nil
}


// Provision a secuurity group rule by checking for existence and creating
// one if missing.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - sgId:  The security group that the rule will be applied to
//  - cidr:  The CIDR network that the security group rule will apply to
//  - proto:  The network protocol the security group rule will apply to
//  - direction:  The network traffic direction the security group rule will control
//  - minPort:  The starting point in the range of ports to apply
//  - maxPort:  The end point in the range of ports to apply
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) SecurityGroupRuleProvision(callTime time.Duration,
                                                    sgId string, cidr string,
                                                    proto string, direction string,
                                                    minPort int32,
                                                    maxPort int32) error {
    // Ensure required args are present
    if cidr == "" || proto == "" || direction == "" {
        return errors.New("sgId or cidr or proto is missing")
    }

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
    return Ec2Man.securityGroupRuleCreate(callTime, sgId, proto, cidr,
                                          direction, minPort, maxPort)
}


// Revokes security group rule based on specified direction.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - sgId:  The security group that the rule will be applied to
//  - proto:  The network protocol the security group rule will apply to
//  - cidr:  The CIDR network that the security group rule will apply to
//  - direction:  The network traffic direction the security group rule will control
//  - minPort:  The starting point in the range of ports to apply
//  - maxPort:  The end point in the range of ports to apply
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man Ec2Manger) RevokeSecurityGroupRule(callTime time.Duration,
                                                sgId string, proto string,
                                                cidr string, direction string,
                                                minPort int32, maxPort int32,
                                                ) error {
    // Ensure required arg is present
    if sgId == "" || proto == "" || cidr == "" {
        return fmt.Errorf("groupID or proto or cidr is missing")
    }

    // Ensure API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    ipPerm := ec2types.IpPermission{
        IpProtocol: aws.String(proto),
        IpRanges: []ec2types.IpRange{
            {
                CidrIp: aws.String(cidr),
            },
        },
        FromPort:   aws.Int32(minPort),
        ToPort:     aws.Int32(maxPort),
    }

    switch direction {
    case "egress":
        revokeCallInput := &ec2.RevokeSecurityGroupEgressInput{
            GroupId:       aws.String(sgId),
            IpPermissions: []ec2types.IpPermission{ipPerm},
        }

        // Revoke the egress security group rule
        _, err := Ec2Man.Client.RevokeSecurityGroupEgress(ctx, revokeCallInput)
        if err != nil {
            return fmt.Errorf("revoke egress on %s failed - %w", sgId, err)
        }

    case "ingress":
        revokeCallInput := &ec2.RevokeSecurityGroupIngressInput{
            GroupId:       aws.String(sgId),
            IpPermissions: []ec2types.IpPermission{ipPerm},
        }

        // Revoke the ingress security group rule
        _, err := Ec2Man.Client.RevokeSecurityGroupIngress(ctx, revokeCallInput)
        if err != nil {
            return fmt.Errorf("revoke ingress on %s failed - %w", sgId, err)
        }

        return nil
    default:
        return errors.New("improper direction specified, use egress or ingress")
    }

    return nil
}
