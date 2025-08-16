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
func (Ec2Man *Ec2Manger) internetGatewayCreateAndAttach(callTime time.Duration,
                                                        vpcId string,
                                                        nameTag string) (
                                                        string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)

    createCallInput := &ec2.CreateInternetGatewayInput{
        TagSpecifications: []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeInternetGateway,
                Tags: []ec2types.Tag{
                    {Key: aws.String("Name"), Value: aws.String(nameTag)},
                },
            },
        },
    }

    // Create the internet gateway
    createOut, err := Ec2Man.client.CreateInternetGateway(ctx, createCallInput)
    cancel()
    if err != nil {
        return "", fmt.Errorf("create internet gateway - %w", err)
    }

    // If the create internet gateway call failed to return an ID
    if createOut.InternetGateway == nil || createOut.InternetGateway.InternetGatewayId == nil {
        return "", fmt.Errorf("create internet gateway returned empty id")
    }

    igwId := *createOut.InternetGateway.InternetGatewayId

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel = context.WithTimeout(context.Background(), callTime)
    defer cancel()

    attachCallInput := &ec2.AttachInternetGatewayInput{
        InternetGatewayId: aws.String(igwId),
        VpcId:             aws.String(vpcId),
    }

    // Attach the created internet gateway to the associated VPC
    _, err = Ec2Man.client.AttachInternetGateway(ctx, attachCallInput)
    if err != nil {
        return "", fmt.Errorf("attach internet gateway %s to vpc %s - %w", igwId, vpcId, err)
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
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.DescribeInternetGatewaysInput{
        Filters: []ec2types.Filter{
            {
                Name:   aws.String("attachment.vpc-id"),
                Values: []string{vpcId},
            },
        },
    }

    // Get informations on any internet gateways in the VPC
    out, err := Ec2Man.client.DescribeInternetGateways(ctx, callInput)
    if err != nil {
        return false, fmt.Errorf("describe internet gateways - %w", err)
    }

    // Iterate through retrieved IGW IDs
    for _, igw := range out.InternetGateways {
        // If the current IGW ID is equal to arg passed in
        if igw.InternetGatewayId == &igwId {
            return true, nil
        }
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
func (Ec2Man *Ec2Manger) InternetGatewayProvision(callTime time.Duration, igwId string,
                                                  vpcId string) (string, error) {
    // If IGW ID is present in YAML
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
    subnetId, err := Ec2Man.internetGatewayCreateAndAttach(callTime, vpcId,
                                                           "Kloud-Kraken-IGW")
    if err != nil {
        return "", err
    }

    return subnetId, nil
}
