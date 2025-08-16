package ec2utils

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) subnetCreate(callTime time.Duration, vpcId string,
                                      cidrBlock string, az string,
                                      isPublic bool) (string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)

    // Create the subnet
    createOut, err := Ec2Man.client.CreateSubnet(ctx, &ec2.CreateSubnetInput{
        VpcId:            aws.String(vpcId),
        CidrBlock:        aws.String(cidrBlock),
        AvailabilityZone: aws.String(az),
    })
    cancel()
    if err != nil {
        return "", fmt.Errorf("unable to create subnet - %w", err)
    }

    subnetID := aws.ToString(createOut.Subnet.SubnetId)

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel = context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Configure to map to public IP address on launch
    _, err = Ec2Man.client.ModifySubnetAttribute(ctx, &ec2.ModifySubnetAttributeInput{
        SubnetId: aws.String(subnetID),
        MapPublicIpOnLaunch: &ec2types.AttributeBooleanValue{
            Value: aws.Bool(isPublic),
        },
    })
    if err != nil {
        return "", fmt.Errorf("unable map subnet to public IP on launch - %w", err)
    }

    return subnetID, nil
}

//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) SubnetExists(callTime time.Duration, vpcId string,
                                      cidrBlock string, subnetId string,
                                      az string) (bool, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Format the input for the subnet description call
    describeInput := &ec2.DescribeSubnetsInput{
        Filters: []ec2types.Filter{
            {Name: aws.String("vpc-id"), Values: []string{vpcId}},
            {Name: aws.String("cidr-block"), Values: []string{cidrBlock}},
            {Name: aws.String("availability-zone"), Values: []string{az}},
        },
    }

    // Get description of input subnet to see if it exists
    out, err := Ec2Man.client.DescribeSubnets(ctx, describeInput)
    if err != nil {
        return false, fmt.Errorf("DescribeSubnets failed - %w", err)
    }

    // If there was a result and it matches the intended subnet ID
    if len(out.Subnets) > 0 && out.Subnets[0].SubnetId == &subnetId {
        return true, nil
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
func (Ec2Man *Ec2Manger) SubnetProvision(callTime time.Duration, subnetId string,
                                         vpcID string, cidrBlock string,
                                         az string, isPublic bool) (
                                         string, error) {
    // If subnet ID is present in YAML
    if subnetId != "" {
        // Check to see if it exists in AWS enviroment
        subnetExists, err := Ec2Man.SubnetExists(callTime, vpcID, cidrBlock,
                                                 subnetId, az)
        if err != nil {
            return "", err
        }

        // If the subnet exists, exit early
        if subnetExists {
            return "", nil
        }
    }

    // Create new subnet
    subnetId, err := Ec2Man.subnetCreate(callTime, vpcID, cidrBlock,
                                         az, isPublic)
    if err != nil {
        return "", err
    }

    return subnetId, nil
}
