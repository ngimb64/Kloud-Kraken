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
                                         eipId string, err error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Allocate Elastic IP address
    out, err := Ec2Man.client.AllocateAddress(ctx, &ec2.AllocateAddressInput{
        Domain: ec2types.DomainTypeVpc,
    })
    if err != nil {
        return "", fmt.Errorf("allocate address - %w", err)
    }

    if out == nil {
        return "", fmt.Errorf("allocate address returned nil")
    }

    // If the output does not contain a puclic IP
    if out.PublicIp == nil {
        return "", fmt.Errorf("AllocateAddress call missing public IP")
    }

    // If the output contains a allocation ID
    if out.AllocationId != nil {
        eipId = *out.AllocationId
    }

    return eipId, nil
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
    if eipId == "" {
        return false, fmt.Errorf("eipId is empty")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Get the Elastic IP address based on passed in allocation ID
    out, err := Ec2Man.client.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{
        AllocationIds: []string{eipId},
    })
    if err != nil {
        var apiErr smithy.APIError

        // If the allocation was not found
        if errors.As(err, &apiErr) &&
        apiErr.ErrorCode() == "InvalideipId.NotFound" {
            return false, nil
        }

        return false, fmt.Errorf("describe addresses - %w", err)
    }

    if len(out.Addresses) == 0 {
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
func (Ec2Man *Ec2Manger) ElasticIpProvision(callTime time.Duration, eipId string) (
                                            string, error) {
    // If Elastic IP ID is present in YAML
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
    eipId, err := Ec2Man.elasticIPCreate(callTime)
    if err != nil {
        return eipId, err
    }

    return eipId, nil
}
