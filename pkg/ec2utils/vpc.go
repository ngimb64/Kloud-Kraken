package ec2utils

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// Creates and waits for the VPC to be created.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - cidrBlock:  The network CIDR block of IP address space to allocate in VPC
//
// @Returns
//  - The ID of the created VPC
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) vpcCreate(callTime time.Duration,
                                   cidrBlock string) (
                                   string, error) {
    // Set context timeout for API call
    ctx, cancel := context.WithTimeout(context.Background(), callTime)

    // Format input for CreateVpc call
    createCallInput := &ec2.CreateVpcInput{
        CidrBlock: aws.String(cidrBlock),
        TagSpecifications: []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeVpc,
                Tags: []ec2types.Tag{
                {
                    Key: aws.String("Name"), Value: aws.String("Kloud-Kraken-VPC"),
                },
            },
        }},
    }

    // Create a new VPC since no valid ID was provided
    createOut, err := Ec2Man.client.CreateVpc(ctx, createCallInput)
    cancel()
    if err != nil {
        return "", err
    }

    vpcId := *createOut.Vpc.VpcId

    // Format input for NewVpcExistsWaiter call
    waiterCallInput := &ec2.DescribeVpcsInput{
        VpcIds: []string{vpcId},
    }

    // Set context timeout for API call
    ctx, cancel = context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Allocate waiter and wait until the VPC is available
    waiter := ec2.NewVpcExistsWaiter(Ec2Man.client)
    err = waiter.Wait(ctx, waiterCallInput, 5 * time.Minute)
    if err != nil {
        return vpcId, err
    }

    return vpcId, nil
}

// Checks to see if the VPC exists.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - vpcID:  The ID of the VPC to ensure exists
//
// @Returns
//  - Boolean to notify whether bucket exists or not
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) VpcExists(callTime time.Duration, vpcId string) (
                                   bool, error) {
    // Set context timeout for API call
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Check to see if the VPC exists
    out, err := Ec2Man.client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
        VpcIds: []string{vpcId},
    })
    if err != nil {
        return false, fmt.Errorf("describe VPC - %w", err)
    }

    // If the ID was identified, exit early
    if len(out.Vpcs) == 0 {
        return false, nil
    }

    return true, nil
}

// Returns VPC ID if it exists, or creates it using supplied CIDR.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - vpcID:  The ID of the VPC to ensure exists
//  - cidrBlock:  The network CIDR block of IP address space to allocate in VPC
//
// @Returns
//  - The ID of VPC if created, otherwise nil
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) VpcProvision(callTime time.Duration, vpcId string,
                                      cidrBlock string) (string, error) {
    // If VPC ID is present in YAML
    if vpcId != "" {
        // Check to see if it exists in AWS enviroment
        vpcExists, err := Ec2Man.VpcExists(callTime, vpcId)
        if err != nil {
            return "", err
        }

        // If the VPC already exists, exit early
        if vpcExists {
            return "", nil
        }
    }

    // Create and wait until VPC is created
    vpcId, err := Ec2Man.vpcCreate(callTime, cidrBlock)
    if err != nil {
        return vpcId, err
    }

    return vpcId, nil
}
