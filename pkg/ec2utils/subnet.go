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
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
)

// Creates a subnet with CIDR block in specified availability zone.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - vpcId:  The VPC where the subnet will be provisioned
//  - cidrBlock:  The CIDR network the subnet will apply to
//  - az:  The availability zone where the subnet will be provisioned
//  - tags:  String map of tag key-values to configure
//  - isPublic:  Toggle for whether the subnet maps a public IP on launch
//
// @Returns
//  - The Subnet ID of created resource
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) subnetCreate(callTime time.Duration, vpcId string,
                                      cidrBlock string, az string,
                                      tags map[string]string,
                                      isPublic bool) (string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    createCallInput := &ec2.CreateSubnetInput{
        AvailabilityZone: aws.String(az),
        CidrBlock:        aws.String(cidrBlock),
        VpcId:            aws.String(vpcId),
    }

    if len(tags) > 0 {
        createCallInput.TagSpecifications = []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeSubnet,
                Tags: awsutils.BuildEc2Tags(tags),
            },
        }
    }

    // Create the subnet
    createOut, err := Ec2Man.client.CreateSubnet(ctx, createCallInput)
    if err != nil {
        return "", fmt.Errorf("unable to create subnet - %w", err)
    }

    subnetID := aws.ToString(createOut.Subnet.SubnetId)

    waiterCallInput := &ec2.DescribeSubnetsInput{
        SubnetIds: []string{subnetID},
    }

    // Allocate waiter and wait until the subnet is available
    waiter := ec2.NewSubnetAvailableWaiter(Ec2Man.client)
    err = waiter.Wait(ctx, waiterCallInput, callTime)
    if err != nil {
        return subnetID, err
    }

    modifyCallInput := &ec2.ModifySubnetAttributeInput{
        SubnetId: aws.String(subnetID),
        MapPublicIpOnLaunch: &ec2types.AttributeBooleanValue{
            Value: aws.Bool(isPublic),
        },
    }

    // Configure to map to public IP address on launch
    _, err = Ec2Man.client.ModifySubnetAttribute(ctx, modifyCallInput)
    if err != nil {
        return "", fmt.Errorf("unable map subnet to public IP on launch - %w", err)
    }

    return subnetID, nil
}

// Checks whether passed in subnet ID exists.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - subnetId:  The ID of the subnet
//
// @Returns
//  - Toggle for whether Subnet already exists or not
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) SubnetExists(callTime time.Duration,
                                      subnetId string) (
                                      bool, error) {
    // Ensure required args are present
    if subnetId == "" {
        return false, errors.New("subnetId is missing")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Format the input for the subnet description call
    describeInput := &ec2.DescribeSubnetsInput{
        Filters: []ec2types.Filter{
            {
                Name: aws.String("subnet-id"),
                Values: []string{subnetId},
            },
        },
    }

    // Describe the subnet by passed in ID
    out, err := Ec2Man.client.DescribeSubnets(ctx, describeInput)
    if err != nil {
        var apiErr smithy.APIError

        // If Subnet ID was not found
        if errors.As(err, &apiErr) &&
        apiErr.ErrorCode() == "InvalidSubnet" {
            return false, nil
        }

        // If a non API related error occured during request
        return false, fmt.Errorf("DescribeSubnets failed - %w", err)
    }

    // If there was a result and it matches the intended subnet ID
    if out == nil || len(out.Subnets) == 0 {
        return false, nil
    }

    return true, nil
}

// Provision subnet by checking for existence and creating if missing.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - subnetId:  The subnet ID
//  - vpcId:  The VPC where the subnet will be provisioned
//  - az:  The availability zone where the subnet will be provisioned
//  - tags:  String map of tag key-values to configure
//  - isPublic:  Toggle for whether the subnet maps a public IP on launch
//
// @Returns
//  - Subnet ID if the resource is created, "" if it already exists
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) SubnetProvision(callTime time.Duration, subnetId string,
                                         vpcID string, cidrBlock string,
                                         az string, tags map[string]string,
                                         isPublic bool) (
                                         string, error) {
    // Ensure required args are present
    if vpcID == "" || cidrBlock == "" || az == "" {
        return "", errors.New("vpcId or cidrBlock or az is missing")
    }

    // If subnet ID is present in YAML
    if subnetId != "" {
        // Check to see if it exists in AWS enviroment
        subnetExists, err := Ec2Man.SubnetExists(callTime, subnetId)
        if err != nil {
            return "", err
        }

        // If the subnet exists, exit early
        if subnetExists {
            return "", nil
        }
    }

    // Create new subnet
    return Ec2Man.subnetCreate(callTime, vpcID, cidrBlock,
                               az, tags, isPublic)
}


// Deletes the subnet.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - subnetId:  The subnet ID
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man Ec2Manger) SubnetTerminator(callTime time.Duration,
                                         subnetId string) error {
    // Ensure required arg is present
    if subnetId == "" {
        return errors.New("subnetId is missing")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    deleteCallInput := &ec2.DeleteSubnetInput{
        SubnetId: aws.String(subnetId),
    }

    // Delete the subnet by passed in ID
    _, err := Ec2Man.client.DeleteSubnet(ctx, deleteCallInput)
    if err != nil {
        return fmt.Errorf("failed to delete subnet (%s) - %w",
                          subnetId, err)
    }

    return nil
}
