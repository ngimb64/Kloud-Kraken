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

// Create a NAT gateway in the specified subnet using the provided EIP allocation ID.
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) natGatewayCreateAndWait(callTime time.Duration,
                                                 subnetID string,
                                                 eipId string,
                                                 nameTag string) (
                                                 string, error) {
    // Ensure required args are present
    if subnetID == ""  || eipId == "" {
        return "", errors.New("subnetID or eipId is missing")
    }

    // Ensure AWS API calls do not hang for longer than the provided timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    createCallInput := &ec2.CreateNatGatewayInput{
        SubnetId:     aws.String(subnetID),
        AllocationId: aws.String(eipId),
    }

    // Tag the NAT gateway name if provided
    if nameTag != "" {
        createCallInput.TagSpecifications = []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeNatgateway,
                Tags: []ec2types.Tag{
                    {
                        Key: aws.String("Name"),
                        Value: aws.String(nameTag),
                    },
                },
            },
        }
    }

    // Create the NAT Gateway
    createOut, err := Ec2Man.client.CreateNatGateway(ctx, createCallInput)
    if err != nil {
        return "", fmt.Errorf("create nat gateway - %w", err)
    }

    // If the create call failed to produce output or the NAT
    // Gateway or it's corresponding ID are missing
    if createOut == nil || createOut.NatGateway == nil ||
    createOut.NatGateway.NatGatewayId == nil {
        return "", errors.New("create nat gateway failed to return gateway id")
    }

    newNatID := aws.ToString(createOut.NatGateway.NatGatewayId)

    waitCallInput := &ec2.DescribeNatGatewaysInput{
        NatGatewayIds: []string{newNatID},
    }

    // Allocate waiter and wait until the NAT Gateway is available
    waiter := ec2.NewNatGatewayAvailableWaiter(Ec2Man.client)
    err = waiter.Wait(ctx, waitCallInput, callTime)
    if err != nil {
        return newNatID, fmt.Errorf("waiting for nat gateway %s available status - %w",
                                    newNatID, err)
    }

    return newNatID, nil
}

// Check whether a usable NAT gateway exists in the given subnet and
// return its id and state.
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) NatGatewayExists(callTime time.Duration,
                                          natId string) (
                                          bool, error) {
    // Ensure required args are present
    if natId == "" {
        return false, errors.New("natId is missing")
    }

    // Ensure AWS API calls do not hang for longer than the provided timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    describeCallInput := &ec2.DescribeNatGatewaysInput{
        Filter: []ec2types.Filter{
            {
                Name: aws.String("nat-gateway-id"),
                Values: []string{natId},
            },
        },
    }

    // Describe the NAT Gateway by passed in ID
    out, err := Ec2Man.client.DescribeNatGateways(ctx, describeCallInput)
    if err != nil {
        var apiErr smithy.APIError

        // If the NAT Gateway ID was not found
        if errors.As(err, &apiErr) &&
        apiErr.ErrorCode() == "InvalidNatGatewayID.NotFound" {
            return false, nil
        }

        // If a non API related error occured during request
        return false, fmt.Errorf("describe nat gateways - %w", err)
    }

    // If no NAT Gateways found in the subnet
    if len(out.NatGateways) == 0 {
        return false, nil
    }

    return true, nil
}

// Provision a NAT gateway by checking for existence and creating one if missing.
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) NatGatewayProvision(callTime time.Duration, natId string,
                                             subnetId string, eipId string,
                                             nameTag string) (string, error) {
    // Ensure required args are present
    if subnetId == "" || eipId == "" {
        return "", errors.New("subnetId or eipId is missing")
    }

    // If nat_id is present in state file
    if natId != "" {
        // Check to see if it exists in AWS enviroment
        exists, err := Ec2Man.NatGatewayExists(callTime, natId)
        if err != nil {
            return "", err
        }

        // If the NAT gateway exists
        if exists {
            return "", nil
        }
    }

    // Create and wait for a new NAT gateway
    return Ec2Man.natGatewayCreateAndWait(callTime, subnetId,
                                          eipId, nameTag)
}


// Deletes the NAT Gateway based on passed in ID.
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) NatGatewayTerminate(callTime time.Duration,
                                             natGatewayId string) error {
    // Ensure required arg is present
    if natGatewayId == "" {
        return errors.New("natGatewayId is missing")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.DeleteNatGatewayInput{
        NatGatewayId: aws.String(natGatewayId),
    }

    // Delete the NAT Gateway
    _, err := Ec2Man.client.DeleteNatGateway(ctx, callInput)
    if err != nil {
        return err
    }

    return nil
}
