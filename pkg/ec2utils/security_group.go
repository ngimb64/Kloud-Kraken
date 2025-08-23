package ec2utils

import (
	"context"
	"errors"
	"fmt"
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
func (Ec2Man *Ec2Manger) securityGroupCreate(callTime time.Duration,
                                             vpcId string,
                                             groupName string,
                                             description string) (
                                             string, error) {
    // Ensure API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    input := &ec2.CreateSecurityGroupInput{
        GroupName:   aws.String(groupName),
        Description: aws.String(description),
        VpcId:       aws.String(vpcId),
    }

    // Optionally attach a Name tag if specified
    if groupName != "" {
        input.TagSpecifications = []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeSecurityGroup,
                Tags: []ec2types.Tag{
                    {
                        Key: aws.String("Name"), Value: aws.String(groupName),
                    },
                },
            },
        }
    }

    // Create the Security Group
    out, err := Ec2Man.client.CreateSecurityGroup(ctx, input)
    if err != nil {
        return "", fmt.Errorf("create security group - %w", err)
    }

    // If the call is missing output or a Security Group ID
    if out == nil || out.GroupId == nil {
        return "", fmt.Errorf("create security group failed to return GroupId")
    }

    callInput := &ec2.RevokeSecurityGroupEgressInput{
        GroupId: aws.String(*out.GroupId),
        IpPermissions: []ec2types.IpPermission{
            {
                IpProtocol: aws.String("-1"),
                IpRanges: []ec2types.IpRange{
                    {
                        CidrIp: aws.String("0.0.0.0/0"),
                    },
                },
            },
        },
    }

    // Revoke all outbound traffic access
    _, err = Ec2Man.client.RevokeSecurityGroupEgress(ctx, callInput)
    if err != nil {
        return *out.GroupId, fmt.Errorf("revoking inital outbound traffic - %w", err)
    }

    return *out.GroupId, nil
}

//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) SecurityGroupExists(callTime time.Duration,
                                             sgId string) (
                                             bool, error) {
    // Ensure required args are present
    if sgId == "" {
        return false, fmt.Errorf("sgId is missing")
    }

    // Ensure API calls do not hang for longer than specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.DescribeSecurityGroupsInput{
        GroupIds: []string{sgId},
    }

    // Get the security group based on passed in ID
    out, err := Ec2Man.client.DescribeSecurityGroups(ctx, callInput)
    if err != nil {
        var apiErr smithy.APIError

        // If API error says the security group does not exist
        if errors.As(err, &apiErr) &&
        apiErr.ErrorCode() == "InvalidGroup.NotFound" {
            return false, nil
        }

        // If a non API related error occured during request
        return false, fmt.Errorf("describe security groups - %w", err)
    }

    // If no security groups were retrieved
    if out == nil || len(out.SecurityGroups) == 0 {
        return false, nil
    }

    return true, nil
}

//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) SecurityGroupProvision(callTime time.Duration, vpcId string,
                                                sgId string, groupName string,
                                                description string) (
                                                string, error) {
    // If Securityy Group IP ID is present in YAML
    if sgId != "" {
        exists, err := Ec2Man.SecurityGroupExists(callTime, sgId)
        if err != nil {
            return "", err
        }

        // If the Security Group already exists, exit early
        if exists {
            return sgId, nil
        }
    }

    // Create a new Security Group
    newSgId, err := Ec2Man.securityGroupCreate(callTime, vpcId,
                                               groupName, description)
    if err != nil {
        return "", err
    }

    return newSgId, nil
}
