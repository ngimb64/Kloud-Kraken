package ec2utils

import (
	"context"
	"errors"
	"fmt"
	"time"

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
func (Ec2Man *Ec2Manger) elasticIPCreate(callTime time.Duration) (
                                         string, error) {
    // Ensure API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.AllocateAddressInput{
        Domain: ec2types.DomainTypeVpc,
    }

    // Allocate Elastic IP Address
    out, err := Ec2Man.client.AllocateAddress(ctx, callInput)
    if err != nil {
        return "", fmt.Errorf("allocating elasic IP - %w", err)
    }

    // If the Elastic IP creation call failed to return output
    if out == nil {
        return "", fmt.Errorf("elastic IP creation call returned nil")
    }

    // If the output does not contain a Puclic IP
    if out.PublicIp == nil {
        return "", fmt.Errorf("elastic IP creation call missing public IP")
    }

    // If the output is missing a Allocation ID
    if out.AllocationId == nil {
        return "", fmt.Errorf("elastic IP creation call missing allocation ID")
    }

    return *out.AllocationId, nil
}

//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) ElasticIPExists(callTime time.Duration,
                                         eipId string) (
                                         bool, error) {
    // Ensure required args are present
    if eipId == "" {
        return false, fmt.Errorf("eipId is missing")
    }

    // Ensure API calls do not hang for than longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.DescribeAddressesInput{
        AllocationIds: []string{eipId},
    }

    // Get the Elastic IP address based on passed in Allocation ID
    out, err := Ec2Man.client.DescribeAddresses(ctx, callInput)
    if err != nil {
        var apiErr smithy.APIError

        // If Allocation ID was not found
        if errors.As(err, &apiErr) &&
        apiErr.ErrorCode() == "InvalidAllocationID.NotFound" {
            return false, nil
        }

        // If a non API related error occured during request
        return false, fmt.Errorf("describe addresses - %w", err)
    }

    // If no Elatic IPs were retrieved
    if out == nil || len(out.Addresses) == 0 {
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
func (Ec2Man *Ec2Manger) ElasticIpProvision(callTime time.Duration,
                                            eipId string) (
                                            string, error) {
    // If Elastic IP ID is present in state file
    if eipId != "" {
        // Check to see if it exists in AWS enviroment
        eipExists, err := Ec2Man.ElasticIPExists(callTime, eipId)
        if err != nil {
            return "", err
        }

        // If the Elastic IP already exists, exit early
        if eipExists {
            return "", nil
        }
    }

    // Create and wait until VPC is created
    return Ec2Man.elasticIPCreate(callTime)
}
