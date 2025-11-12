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
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
)

// Create a route table in the VPC add the passed in route to either the IGW or
// NAT depending on arguments.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - vpcId:  The VPC ID where the route table is to be created
//  - igwId:  The ID of the Internet Gateway to attach to route table
//  - natId:  The ID of the NAT Gateway to attach to route table
//  - destCidr:  The CIDR netork that the route table routes to
//  - tags:  String map of tag key-values to configure
//
// @Returns
//  - The Route Table ID of created resource
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) routeTableCreateAndAttach(callTime time.Duration,
                                                   vpcId string, igwId string,
                                                   natId string, destCidr string,
                                                   tags map[string]string) (
                                                   string, error) {
    // Ensure required arg is present
    if vpcId == "" || destCidr == "" {
        return "", errors.New("vpcId or destCidr is missing")
    }

    // Ensure either one of the two args are set
    if (igwId == "") == (natId == "") {
        return "", errors.New("either igwId or natId must be provided")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    createRouteTableCallInput := &ec2.CreateRouteTableInput{
        VpcId: aws.String(vpcId),
    }

    // Tag the route table name if provided
    if len(tags) > 0 {
        createRouteTableCallInput.TagSpecifications = []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeRouteTable,
                Tags: awsutils.BuildEc2Tags(tags),
            },
        }
    }

    // Create route table in passed in VPC ID
    createOut, err := Ec2Man.Client.CreateRouteTable(ctx, createRouteTableCallInput)
    if err != nil {
        return "", fmt.Errorf("create route table - %w", err)
    }

    if createOut == nil || createOut.RouteTable == nil ||
    createOut.RouteTable.RouteTableId == nil {
        return "", errors.New("create route table failed to return route table id")
    }

    rtID := aws.ToString(createOut.RouteTable.RouteTableId)

    createRouteCallInput := &ec2.CreateRouteInput{
        DestinationCidrBlock: aws.String(destCidr),
        RouteTableId:         aws.String(rtID),
    }
    if igwId != "" {
        createRouteCallInput.GatewayId = aws.String(igwId)
    } else {
        createRouteCallInput.NatGatewayId = aws.String(natId)
    }

    // Create route to the chosen target
    _, err = Ec2Man.Client.CreateRoute(ctx, createRouteCallInput)
    if err != nil {
        return "", fmt.Errorf("create route to target on rt %s - %w", rtID, err)
    }

    return rtID, nil
}

// Check whether route table exists in the VPC.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - rtId:  The route table ID
//
// @Returns
//  - Toggle for whether Route Table already exists or not
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) RouteTableExists(callTime time.Duration,
                                          rtId string) (
                                          bool, error) {
    // Ensure required args are present
    if rtId == "" {
        return false, errors.New("rtId is missing")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.DescribeRouteTablesInput{
        Filters: []ec2types.Filter{
            {
                Name: aws.String("route-table-id"),
                Values: []string{rtId},
            },
        },
    }

    // Describe route tables based on passed in ID
    out, err := Ec2Man.Client.DescribeRouteTables(ctx, callInput)
    if err != nil {
        var apiErr smithy.APIError

        // If the Route Table ID was not found
        if errors.As(err, &apiErr) &&
        apiErr.ErrorCode() == "InvalidRouteTableID.NotFound" {
            return false, nil
        }

        // If a non API related error occured during request
        return false, fmt.Errorf("describe route tables - %w", err)
    }

    // If there were no route tables found
    if len(out.RouteTables) == 0 {
        return false, nil
    }

    // Iterate through returned route tables
    for _, rt := range out.RouteTables {
        if rt.RouteTableId != nil || *rt.RouteTableId == rtId {
            return true, nil
        }
    }

    return false, nil
}


// Provision a route table that routes passed in network CIDR to either
// the IGW or NAT and associate it to a subnet when missing. Either the
// igwId or the natId is specified depending the desired destination.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - rtId:  The route table ID
//  - vpcId:  The VPC ID where the route table resides
//  - igwId:  If specified, the Internet Gateway the route table is attached to
//  - natId:  If specified, the NAT Gateway ID the route table is attached to
//  - destCidr:  The CIDR netork that the route table routes to
//  - tags:  String map of tag key-values to configure
//
// @Returns
//  - Route table ID if the resource is created, "" if it already exists
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) RouteTableProvision(callTime time.Duration, rtId string,
                                             vpcId string, igwId string,
                                             natId string, destCidr string,
                                             tags map[string]string) (
                                             string, error) {
    // Ensure required arg is present
    if vpcId == "" || destCidr == "" {
        return "", errors.New("vpcId or destCidr is missing")
    }

    // Ensure either one of the two args are set
    if (igwId == "") == (natId == "") {
        return "", errors.New("either igwId or natId must be provided")
    }

    // If route_table_id is present in state file
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

    // Create a new route table with chosen target
    return Ec2Man.routeTableCreateAndAttach(callTime, vpcId, igwId,
                                            natId, destCidr, tags)
}


// Deletes the route table.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - routeTableId:  The ID of the route table to terminate
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man Ec2Manger) RouteTableTerminator(callTime time.Duration,
                                             routeTableId string) error {
    // Ensure required arg is present
    if routeTableId == "" {
        return errors.New("routeTableId is missing")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    deleteRouteCallInput := &ec2.DeleteRouteTableInput{
        RouteTableId: aws.String(routeTableId),
    }

    _, err := Ec2Man.Client.DeleteRouteTable(ctx, deleteRouteCallInput)
    if err != nil {
        return fmt.Errorf("failed to delete route table (%s) - %w",
                          routeTableId, err)
    }

    return nil
}
