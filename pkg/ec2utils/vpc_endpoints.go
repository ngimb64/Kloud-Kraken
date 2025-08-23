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
func (Ec2Man *Ec2Manger) gatewayEndpointCreate(callTime time.Duration, vpcId string,
                                               routeTableIds []string, region string) (
                                               string, error) {
    if vpcId == "" {
        return "", fmt.Errorf("vpcId is empty")
    }

    if len(routeTableIds) == 0 {
        return "", fmt.Errorf("routeTableIds required for gateway endpoint")
    }

    if region == "" {
        return "", fmt.Errorf("region is empty")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    serviceName := "com.amazonaws." + region + ".s3"
    in := &ec2.CreateVpcEndpointInput{
        VpcId:           aws.String(vpcId),
        ServiceName:     aws.String(serviceName),
        VpcEndpointType: ec2types.VpcEndpointTypeGateway,
        RouteTableIds:   routeTableIds,
    }

    out, err := Ec2Man.client.CreateVpcEndpoint(ctx, in)
    if err != nil {
        return "", fmt.Errorf("create gateway vpc endpoint (%s) - %w", serviceName, err)
    }

    if out == nil || out.VpcEndpoint == nil || out.VpcEndpoint.VpcEndpointId == nil {
        return "", fmt.Errorf("createVpcEndpoint returned nil for %s", serviceName)
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
func (Ec2Man *Ec2Manger) interfaceEndpointCreate(callTime time.Duration, vpcId string,
                                                 serviceName string, subnetIds []string,
                                                 securityGroupIds []string, privateDns bool) (
                                                 string, error) {
    if vpcId == "" {
        return "", fmt.Errorf("vpcId is empty")
    }

    if serviceName == "" {
        return "", fmt.Errorf("serviceName is empty")
    }

    if len(subnetIds) == 0 {
        return "", fmt.Errorf("subnetIds required for interface endpoint")
    }

    if len(securityGroupIds) == 0 {
        return "", fmt.Errorf("securityGroupIds required for interface endpoint")
    }

    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.CreateVpcEndpointInput{
        VpcId:             aws.String(vpcId),
        ServiceName:       aws.String(serviceName),
        VpcEndpointType:   ec2types.VpcEndpointTypeInterface,
        SubnetIds:         subnetIds,
        SecurityGroupIds:  securityGroupIds,
        PrivateDnsEnabled: aws.Bool(privateDns),
    }

    out, err := Ec2Man.client.CreateVpcEndpoint(ctx, callInput)
    if err != nil {
        return "", fmt.Errorf("create interface vpc endpoint (%s) - %w",
                              serviceName, err)
    }

    if out == nil || out.VpcEndpoint == nil || out.VpcEndpoint.VpcEndpointId == nil {
        return "", fmt.Errorf("createVpcEndpoint returned nil for %s", serviceName)
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
func (Ec2Man *Ec2Manger) S3EndpointProvision(callTime time.Duration, region string,
                                             vpcId string, routeTableIds []string) (
                                             string, error) {
    if region == "" {
        return "", fmt.Errorf("region is empty")
    }

    serviceName := "com.amazonaws." + region + ".s3"

    exists, epId, err := Ec2Man.vpcEndpointExists(callTime, vpcId, serviceName)
    if err != nil {
        return "", err
    }

    if exists {
        return epId, nil
    }

    return Ec2Man.gatewayEndpointCreate(callTime, vpcId, routeTableIds, region)
}

// SSM provisioner: tiny function that reuses vpcEndpointExists + interfaceEndpointCreate.
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) SsmEndpointProvision(callTime time.Duration, region string,
                                              vpcId string, subnetIds []string,
                                              securityGroupIds []string) (
                                              string, error) {
    if region == "" {
        return "", fmt.Errorf("region is empty")
    }

    serviceName := "com.amazonaws." + region + ".ssm"

    exists, epId, err := Ec2Man.vpcEndpointExists(callTime, vpcId, serviceName)
    if err != nil {
        return "", err
    }

    if exists {
        return epId, nil
    }

    // Private DNS enabled for SSM
    return Ec2Man.interfaceEndpointCreate(callTime, vpcId, serviceName,
                                          subnetIds, securityGroupIds, true)
}

// Generic existence checker usable by S3, SSM, etc.
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) vpcEndpointExists(callTime time.Duration, vpcId string,
                                           serviceName string) (
                                           bool, string, error) {
    if vpcId == "" {
        return false, "", fmt.Errorf("vpcId is empty")
    }

    if serviceName == "" {
        return false, "", fmt.Errorf("serviceName is empty")
    }

    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    in := &ec2.DescribeVpcEndpointsInput{
        Filters: []ec2types.Filter{
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

    out, err := Ec2Man.client.DescribeVpcEndpoints(ctx, in)
    if err != nil {
        var apiErr smithy.APIError
        // No stable NotFound code for DescribeVpcEndpoints; treat API errors as "not exists"
        if errors.As(err, &apiErr) {
            return false, "", nil
        }

        return false, "", fmt.Errorf("describe vpc endpoints - %w", err)
    }

    if out == nil || len(out.VpcEndpoints) == 0 {
        return false, "", nil
    }

    return true, aws.ToString(out.VpcEndpoints[0].VpcEndpointId), nil
}