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
func (Ec2Man *Ec2Manger) internetGatewayCreateAndAttach(callTime time.Duration,
                                                        vpcId string,
                                                        tagName string) (
                                                        string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    createCallInput := &ec2.CreateInternetGatewayInput{}

    if tagName != "" {
        createCallInput.TagSpecifications = []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeInternetGateway,
                Tags: []ec2types.Tag{
                    {
                        Key: aws.String("Name"),
                        Value: aws.String(tagName),
                    },
                },
            },
        }
    }

    // Create the internet gateway
    createOut, err := Ec2Man.client.CreateInternetGateway(ctx, createCallInput)
    if err != nil {
        return "", fmt.Errorf("create internet gateway - %w", err)
    }

    // If the create internet gateway call failed to return an ID
    if createOut.InternetGateway == nil ||
    createOut.InternetGateway.InternetGatewayId == nil {
        return "", errors.New("create internet gateway returned empty id")
    }

    igwId := *createOut.InternetGateway.InternetGatewayId

    waiterCallInput := &ec2.DescribeInternetGatewaysInput{
      InternetGatewayIds: []string{igwId},
    }

    // Allocate waiter and wait until the Internet Gateway is available
    waiter := ec2.NewInternetGatewayExistsWaiter(Ec2Man.client)
    err = waiter.Wait(ctx, waiterCallInput, callTime)
    if err != nil {
        return igwId, err
    }

    attachCallInput := &ec2.AttachInternetGatewayInput{
        InternetGatewayId: aws.String(igwId),
        VpcId:             aws.String(vpcId),
    }

    // Attach the created internet gateway to the associated VPC
    _, err = Ec2Man.client.AttachInternetGateway(ctx, attachCallInput)
    if err != nil {
        return "", fmt.Errorf("attach internet gateway %s to vpc %s - %w",
                              igwId, vpcId, err)
    }

    return igwId, nil
}

//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) InternetGatewayExists(callTime time.Duration,
                                               vpcId string, igwId string) (
                                               bool, error) {
    // Ensure required args are present
    if vpcId == "" || igwId == "" {
        return false, errors.New("vpcId or igwId is missing")
    }

    // Ensure API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    input := &ec2.DescribeInternetGatewaysInput{
        InternetGatewayIds: []string{igwId},
    }

    // Get the Internet Gateway based on passed in Gateway ID
    out, err := Ec2Man.client.DescribeInternetGateways(ctx, input)
    if err != nil {
        var apiErr smithy.APIError

        // If the Gateway ID was not found
        if errors.As(err, &apiErr) &&
        apiErr.ErrorCode() == "InvalidInternetGatewayID.NotFound" {
            return false, nil
        }

        // If a non API related error occured during request
        return false, fmt.Errorf("describe internet gateways - %w", err)
    }

    // If no Internet Gatewys were found
    if len(out.InternetGateways) == 0 {
        return false, nil
    }

    // Iterate through retrieved Internet Gateways
    for _, igw := range out.InternetGateways {
        // If the IGW ID is present and matches arg
        if igw.InternetGatewayId != nil &&
        *igw.InternetGatewayId == igwId {
            // Iterate through the IGW attachments
            for _, attachment := range igw.Attachments {
                // If the attached VPC ID is present and matches arg
                if attachment.VpcId != nil &&
                *attachment.VpcId == vpcId {
                    return true, nil
                }
            }
        }
    }

    // IGW exists but is not attached to the provided VPC
    return false, fmt.Errorf("igw exist but is not attached to %s", vpcId)
}

//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) InternetGatewayProvision(callTime time.Duration,
                                                  igwId string,
                                                  vpcId string,
                                                  nameTag string) (
                                                  string, error) {
    // Ensure required args are present
    if vpcId == "" {
        return "", errors.New("vpcId is missing")
    }

    // If IGW ID is present in state file
    if igwId != "" {
        // Check to see if it exists in AWS enviroment
        igwExists, err := Ec2Man.InternetGatewayExists(callTime, vpcId, igwId)
        if err != nil {
            return "", err
        }

        // If the IGW exists, exit early
        if igwExists {
            return "", nil
        }
    }

    // Create new internet gateway
    return Ec2Man.internetGatewayCreateAndAttach(callTime, vpcId, nameTag)
}


//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man Ec2Manger) InternetGatewayTerminate(callTime time.Duration,
                                                 igwId string,
                                                 vpcId string) error {
    // Ensure required args are present
    if igwId == "" || vpcId == "" {
        return errors.New("igwId or vpcId is missing")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    detachCallInput := &ec2.DetachInternetGatewayInput{
        InternetGatewayId: aws.String(igwId),
        VpcId:             aws.String(vpcId),
    }

    // Detach the Internet Gateway from specified VPC
    _, err := Ec2Man.client.DetachInternetGateway(ctx, detachCallInput)
    if err != nil {
        return fmt.Errorf("failed to detach internet gateway %s from VPC %s - %w",
                          igwId, vpcId, err)
    }

    deleteCallInput := &ec2.DeleteInternetGatewayInput{
        InternetGatewayId: aws.String(igwId),
    }

    // Delete the Internet Gateway
    _, err = Ec2Man.client.DeleteInternetGateway(ctx, deleteCallInput)
    if err != nil {
        return fmt.Errorf("failed to delete internet gateway %s - %w",
                          igwId, err)
    }

    return nil
}
