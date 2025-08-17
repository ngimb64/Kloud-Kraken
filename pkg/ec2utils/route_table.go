package ec2utils

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// Create a route table in the VPC add a 0.0.0.0/0 route to either
// the IGW or NAT depending on arguments.
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) routeTableCreateAndAttach(callTime time.Duration, vpcId string,
                                                   igwId string, natId string,
                                                   subnetId string, nameTag string) (
                                                   string, error) {
    // Ensure required arg is present
    if vpcId == "" {
        return "", fmt.Errorf("vpcId is required")
    }

    if (igwId == "" && natId == "") || (igwId != "" && natId != "") {
        return "", fmt.Errorf("exactly one of igwId or natId must be provided")
    }

    createRouteCallInput := &ec2.CreateRouteTableInput{
        VpcId: aws.String(vpcId),
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)

    createOut, err := Ec2Man.client.CreateRouteTable(ctx, createRouteCallInput)
    cancel()
    if err != nil {
        return "", fmt.Errorf("create route table - %w", err)
    }

    if createOut == nil || createOut.RouteTable == nil ||
    createOut.RouteTable.RouteTableId == nil {
        return "", fmt.Errorf("create route table returned empty id")
    }

    rtID := aws.ToString(createOut.RouteTable.RouteTableId)

    // Build and create the default route to the chosen target
    routeIn := &ec2.CreateRouteInput{
        RouteTableId:         aws.String(rtID),
        DestinationCidrBlock: aws.String("0.0.0.0/0"),
    }
    if igwId != "" {
        routeIn.GatewayId = aws.String(igwId)
    } else {
        routeIn.NatGatewayId = aws.String(natId)
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctxRoute, cancelRoute := context.WithTimeout(context.Background(), callTime)

    _, err = Ec2Man.client.CreateRoute(ctxRoute, routeIn)
    cancelRoute()
    if err != nil {
        return "", fmt.Errorf("create route to target on rt %s - %w", rtID, err)
    }

    // Tag the route table name if provided
    if nameTag != "" {
        // Ensure AWS API calls do not hang for longer specified timeout
        ctxTag, cancelTag := context.WithTimeout(context.Background(), callTime)

        _, _ = Ec2Man.client.CreateTags(ctxTag, &ec2.CreateTagsInput{
            Resources: []string{rtID},
            Tags: []ec2types.Tag{
                {Key: aws.String("Name"), Value: aws.String(nameTag)},
            },
        })
        cancelTag()
    }

    // Associate the route table to the provided subnet if given
    if subnetId != "" {
        // Ensure AWS API calls do not hang for longer specified timeout
        ctxAssoc, cancelAssoc := context.WithTimeout(context.Background(), callTime)

        associateCallInput := &ec2.AssociateRouteTableInput{
            RouteTableId: aws.String(rtID),
            SubnetId:     aws.String(subnetId),
        }

        _, err = Ec2Man.client.AssociateRouteTable(ctxAssoc, associateCallInput)
        cancelAssoc()
        if err != nil {
            return "", fmt.Errorf("associate route table %s to subnet %s - %w",
                                  rtID, subnetId, err)
        }
    }

    return rtID, nil
}

// Check whether a route table exists in the VPC with a 0.0.0.0/0 route to the
// provided IGW or NAT/ Returns exists true when such a route table is present
// id of the route table and a short state string state values are either
// "has-route" or "has-route-and-association" or empty when not found.
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) RouteTableExists(callTime time.Duration,
                                          rtId string) (
                                          bool, error) {
    if rtId == "" {
        return false, fmt.Errorf("rtId is empty")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.DescribeRouteTablesInput{
        Filters: []ec2types.Filter{
            {Name: aws.String("route-table-id"), Values: []string{rtId}},
        },
    }

    // Describe route tables based on passed in ID
    out, err := Ec2Man.client.DescribeRouteTables(ctx, callInput)
    if err != nil {
        return false, fmt.Errorf("describe route tables - %w", err)
    }

    // If there were no route tables found
    if len(out.RouteTables) == 0 {
        return false, nil
    }

    return true, nil
}


// Provision a route table that routes 0.0.0.0/0 to either the IGW or NAT and
// associate it to a subnet when missing.
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) RouteTableProvision(callTime time.Duration, rtId string,
                                             vpcId string, igwId string,
                                             natId string, subnetId string,
                                             nameTag string) (
                                             string, error) {
    // If route_table_id is present in YAML
    if rtId != "" {
        // If a matching route table exists return it
        exists, err := Ec2Man.RouteTableExists(callTime, rtId)
        if err != nil {
            return "", err
        }

        // If the route table exists
        if exists {
            return "", nil
        }
    }

    // Create a new route table with the chosen target and optionally associate it
    rtId, err := Ec2Man.routeTableCreateAndAttach(callTime, vpcId, igwId,
                                                   natId, subnetId, nameTag)
    if err != nil {
        return "", err
    }

    return rtId, nil
}
