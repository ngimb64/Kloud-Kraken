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

// Generic creator for Gateway endpoints (S3).
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) gatewayEndpointCreate(callTime time.Duration,
                                               policyDocument string,
                                               vpcId string,
                                               serviceName string,
                                               routeTableIds []string,
                                               tagName string) (
                                               string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.CreateVpcEndpointInput{
        PolicyDocument:  aws.String(policyDocument),
        RouteTableIds:   routeTableIds,
        ServiceName:     aws.String(serviceName),
        VpcEndpointType: ec2types.VpcEndpointTypeGateway,
        VpcId:           aws.String(vpcId),
    }

    if tagName != "" {
        callInput.TagSpecifications = []ec2types.TagSpecification{
         {
            ResourceType: ec2types.ResourceTypeVpcEndpoint,
            Tags: []ec2types.Tag{
                {
                    Key: aws.String("Name"),
                    Value: aws.String(tagName),
                },
            },
         },
        }
    }

    // Create the gateway VPC endpoint
    out, err := Ec2Man.client.CreateVpcEndpoint(ctx, callInput)
    if err != nil {
        return "", fmt.Errorf("create gateway vpc endpoint (%s) - %w",
                              serviceName, err)
    }

    // If VPC endpoint creation failed to return output
    if out == nil || out.VpcEndpoint == nil ||
    out.VpcEndpoint.VpcEndpointId == nil {
        return "", fmt.Errorf("createVpcEndpoint returned nil for %s",
                              serviceName)
    }

    return aws.ToString(out.VpcEndpoint.VpcEndpointId), nil
}

// Generic creator for Interface endpoints (SSM, ssmmessages, ec2messages, etc.).
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) interfaceEndpointCreate(callTime time.Duration,
                                                 policyDocument string,
                                                 vpcId string,
                                                 serviceName string,
                                                 subnetIds []string,
                                                 securityGroupIds []string,
                                                 tagName string) (
                                                 string, error) {
    // Ensure API calls do not hang for than longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.CreateVpcEndpointInput{
        PolicyDocument:    aws.String(policyDocument),
        PrivateDnsEnabled: aws.Bool(true),
        SecurityGroupIds:  securityGroupIds,
        ServiceName:       aws.String(serviceName),
        SubnetIds:         subnetIds,
        VpcEndpointType:   ec2types.VpcEndpointTypeInterface,
        VpcId:             aws.String(vpcId),
    }

    if tagName != "" {
        callInput.TagSpecifications = []ec2types.TagSpecification{
         {
            ResourceType: ec2types.ResourceTypeVpcEndpoint,
            Tags: []ec2types.Tag{
                {
                    Key: aws.String("Name"),
                    Value: aws.String(tagName),
                },
            },
         },
        }
    }

    // Create the interface VPC endpoint
    out, err := Ec2Man.client.CreateVpcEndpoint(ctx, callInput)
    if err != nil {
        return "", fmt.Errorf("create interface vpc endpoint (%s) - %w",
                              serviceName, err)
    }

    // If VPC endpoint creation failed to return output
    if out == nil || out.VpcEndpoint == nil ||
    out.VpcEndpoint.VpcEndpointId == nil {
        return "", fmt.Errorf("createVpcEndpoint returned nil for %s",
                              serviceName)
    }

    return aws.ToString(out.VpcEndpoint.VpcEndpointId), nil
}

// S3 provisioner: tiny function that reuses vpcEndpointExists + gatewayEndpointCreate.
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) S3EndpointProvision(callTime time.Duration,
                                             endpointId string,
                                             region string, vpcId string,
                                             policyDocument string,
                                             routeTableIds []string,
                                             tagName string) (
                                             string, error) {
    // Ensure required args are present
    if vpcId == "" || region == "" || policyDocument == "" {
        return "", errors.New("vpcId or region or policyDocument is missing")
    }

    // Ensure a route table ID was passed in
    if len(routeTableIds) == 0 {
        return "", errors.New("routeTableIds is missing entries")
    }

    // Set the service name for VPC Endpoint
    serviceName := "com.amazonaws." + region + ".s3"

    // If VPC Endpoint ID is present in state file
    if endpointId != "" {
        // Check to see if it exists in AWS environment
        exists, epId, err := Ec2Man.VpcEndpointExists(callTime, endpointId,
                                                      vpcId, serviceName)
        if err != nil {
            return "", err
        }

        // If the VPC Endpoint ID already exists, exit early
        if exists {
            return epId, nil
        }
    }

    return Ec2Man.gatewayEndpointCreate(callTime, policyDocument, vpcId,
                                        serviceName, routeTableIds, tagName)
}

// SSM provisioner: tiny function that reuses vpcEndpointExists + interfaceEndpointCreate.
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) SsmEndpointProvision(callTime time.Duration,
                                              endpointId string, region string,
                                              vpcId string, policyDocument string,
                                              subnetIds []string,
                                              securityGroupIds []string,
                                              tagName string) (
                                              string, error) {
    // Ensure required args are present
    if region == "" || vpcId == "" || policyDocument == "" {
        return "", errors.New("region or vpcId or policyDocument is missing")
    }

    // Ensure Subnet IDs and Security Group IDs have entries
    if len(subnetIds) == 0 || len(securityGroupIds) == 0 {
        return "", errors.New("subnetIds or securityGroupIds is missing entries")
    }

    // Set the service name for VPC Endpoint
    serviceName := "com.amazonaws." + region + ".ssm"

    // If VPC Endpoint ID is present in state file
    if endpointId != "" {
        // Check to see if it exists in AWS environment
        exists, epId, err := Ec2Man.VpcEndpointExists(callTime, endpointId,
                                                      vpcId, serviceName)
        if err != nil {
            return "", err
        }

        // If the VPC Endpoint ID already exists, exit early
        if exists {
            return epId, nil
        }
    }

    // Private DNS enabled for SSM
    return Ec2Man.interfaceEndpointCreate(callTime, policyDocument, vpcId,
                                          serviceName, subnetIds,
                                          securityGroupIds, tagName)
}

// Generic existence checker usable by S3, SSM, etc.
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) VpcEndpointExists(callTime time.Duration,
                                           endpointId string, vpcId string,
                                           serviceName string) (
                                           bool, string, error) {
    // Ensure required args are present
    if endpointId == "" || vpcId == "" || serviceName == "" {
        return false, "", errors.New("endpointId or vpcId or serviceName is missing")
    }

    // Ensure API calls do not hang for than longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.DescribeVpcEndpointsInput{
        Filters: []ec2types.Filter{
            {
                Name: aws.String("vpc-endpoint-id"),
                Values: []string{endpointId},
            },
            {
                Name:   aws.String("vpc-id"),
                Values: []string{vpcId},
            },
            {
                Name:   aws.String("service-name"),
                Values: []string{serviceName},
            },
        },
    }

    // Get VPC endpoint based in specified filters
    out, err := Ec2Man.client.DescribeVpcEndpoints(ctx, callInput)
    if err != nil {
        var apiErr smithy.APIError

        // If the VPC Endpoint was not found
        if errors.As(err, &apiErr) &&
        apiErr.ErrorCode() == "InvalidVpcEndpoint.NotFound" {
            return false, "", nil
        }

        return false, "", fmt.Errorf("describe vpc endpoints - %w", err)
    }

    // If no VPC Endpoints were retrieved
    if out == nil || len(out.VpcEndpoints) == 0 {
        return false, "", nil
    }

    return true, aws.ToString(out.VpcEndpoints[0].VpcEndpointId), nil
}


//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man Ec2Manger) VpcEndpointsTerminate(callTime time.Duration,
                                              vpcEndpointIds []string) error {
    // Ensure a VPC endpoint is present
    if len(vpcEndpointIds) == 0 {
        return fmt.Errorf("vpcEndpointId is missing")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.DeleteVpcEndpointsInput {
        VpcEndpointIds: vpcEndpointIds,
    }

    // Delete the VPC endpoints
    _, err := Ec2Man.client.DeleteVpcEndpoints(ctx, callInput)
    if err != nil {
        return err
    }

    return nil
}
