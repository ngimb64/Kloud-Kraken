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

// Create a route table in the VPC add the passed in route to either
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
                                                   destCidr string, nameTag string,
                                                   subnetId string) (
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

    createRouteCallInput := &ec2.CreateRouteTableInput{
        VpcId: aws.String(vpcId),
    }

    // Create route table in passed in VPC ID
    createOut, err := Ec2Man.client.CreateRouteTable(ctx, createRouteCallInput)
    if err != nil {
        return "", fmt.Errorf("create route table - %w", err)
    }

    if createOut == nil || createOut.RouteTable == nil ||
    createOut.RouteTable.RouteTableId == nil {
        return "", errors.New("create route table failed to return route table id")
    }

    rtID := aws.ToString(createOut.RouteTable.RouteTableId)

    routeIn := &ec2.CreateRouteInput{
        RouteTableId:         aws.String(rtID),
        DestinationCidrBlock: aws.String(destCidr),
    }
    if igwId != "" {
        routeIn.GatewayId = aws.String(igwId)
    } else {
        routeIn.NatGatewayId = aws.String(natId)
    }

    // Create route to the chosen target
    _, err = Ec2Man.client.CreateRoute(ctx, routeIn)
    if err != nil {
        return "", fmt.Errorf("create route to target on rt %s - %w", rtID, err)
    }

    // Tag the route table name if provided
    if nameTag != "" {
        createTagsCallInput := &ec2.CreateTagsInput{
            Resources: []string{rtID},
            Tags: []ec2types.Tag{
                {
                    Key: aws.String("Name"), Value: aws.String(nameTag),
                },
            },
        }

        _, _ = Ec2Man.client.CreateTags(ctx, createTagsCallInput)
    }

    // Associate the route table to the provided subnet if given
    if subnetId != "" {
        associateCallInput := &ec2.AssociateRouteTableInput{
            RouteTableId: aws.String(rtID),
            SubnetId:     aws.String(subnetId),
        }

        _, err = Ec2Man.client.AssociateRouteTable(ctx, associateCallInput)
        if err != nil {
            return "", fmt.Errorf("associate route table %s to subnet %s - %w",
                                  rtID, subnetId, err)
        }
    }

    return rtID, nil
}

// Check whether a route table exists in the VPC with passed in route to the
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
                Name: aws.String("route-table-id"), Values: []string{rtId},
            },
        },
    }

    // Describe route tables based on passed in ID
    out, err := Ec2Man.client.DescribeRouteTables(ctx, callInput)
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
// the IGW or NAT and associate it to a subnet when missing.
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
                                             destCidr string, nameTag string) (
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

    // Create a new route table with the chosen target and optionally associate it
    return Ec2Man.routeTableCreateAndAttach(callTime, vpcId, igwId, natId,
                                            destCidr, nameTag, subnetId)
}
