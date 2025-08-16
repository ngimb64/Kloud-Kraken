package ec2utils

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// Create a NAT gateway in the specified subnet using the provided EIP allocation ID.
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) natGatewayCreateAndWait(callTime time.Duration, subnetID string,
                                                 eipId string, nameTag string) (
                                                 string, error) {
    // Ensure required args are present
    if subnetID == "" {
        return "", fmt.Errorf("subnetID is required")
    }

    if eipId == "" {
        return "", fmt.Errorf("eipId is required")
    }

    // Ensure AWS API calls do not hang for longer than the provided timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)

    createIn := &ec2.CreateNatGatewayInput{
        SubnetId:     aws.String(subnetID),
        AllocationId: aws.String(eipId),
    }

    createOut, err := Ec2Man.client.CreateNatGateway(ctx, createIn)
    cancel()
    if err != nil {
        return "", fmt.Errorf("create nat gateway - %w", err)
    }

    if createOut == nil || createOut.NatGateway == nil ||
    createOut.NatGateway.NatGatewayId == nil {
        return "", fmt.Errorf("create nat gateway returned empty id")
    }

    newNatID := aws.ToString(createOut.NatGateway.NatGatewayId)

    // Tag the NAT gateway name if provided
    if nameTag != "" {
        // Ensure AWS API calls do not hang for longer specified timeout
        ctxTag, cancelTag := context.WithTimeout(context.Background(), callTime)

        _, _ = Ec2Man.client.CreateTags(ctxTag, &ec2.CreateTagsInput{
            Resources: []string{newNatID},
            Tags: []ec2types.Tag{
                {Key: aws.String("Name"), Value: aws.String(nameTag)},
            },
        })
        cancelTag()
    }

    // Ensure AWS API calls do not hang for longer than the provided timeout
    ctxWait, cancelWait := context.WithTimeout(context.Background(), callTime)
    defer cancelWait()

    waitCallInput := &ec2.DescribeNatGatewaysInput{
        NatGatewayIds: []string{newNatID},
    }

    waiter := ec2.NewNatGatewayAvailableWaiter(Ec2Man.client)
    // Wait until the NAT gateway becomes available
    err = waiter.Wait(ctxWait, waitCallInput, callTime)
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
func (Ec2Man *Ec2Manger) NatGatewayExists(callTime time.Duration, natId string) (
                                          bool, error) {
    // Ensure AWS API calls do not hang for longer than the provided timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    describeCallInput := &ec2.DescribeNatGatewaysInput{
        Filter: []ec2types.Filter{
            {Name: aws.String("nat-gateway-id"), Values: []string{natId}},
        },
    }

    // Get the NAT gateways in passed in subnet ID
    out, err := Ec2Man.client.DescribeNatGateways(ctx, describeCallInput)
    if err != nil {
        return false, fmt.Errorf("describe nat gateways - %w", err)
    }

    // No NAT gateways found in the subnet
    if len(out.NatGateways) == 0 {
        return false, nil
    }

    // Gateway found
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
    // If nat_id is present in yaml
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
    natId, err := Ec2Man.natGatewayCreateAndWait(callTime, subnetId,
                                                 eipId, nameTag)
    if err != nil {
        return "", err
    }

    return natId, nil
}
