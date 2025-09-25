package ec2utils

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// Create an association between a route table and subnet.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - routeTableId:  The ID of the route table to associate
//  - subnetId:  The ID of the subnet to associate
//
// @Returns
//  - The ID of route table to subnet association
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) associateRouteTableToSubnet(callTime time.Duration,
                                                     routeTableId string,
                                                     subnetId string) (
                                                     string, error) {
    // Ensure required args are present
    if routeTableId == "" || subnetId == "" {
        return "", errors.New("routeTableId or subnetId is missing")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    associateCallInput := &ec2.AssociateRouteTableInput{
        RouteTableId: aws.String(routeTableId),
        SubnetId:     aws.String(subnetId),
    }

    // Associate the route table to the subnet
    out, err := Ec2Man.client.AssociateRouteTable(ctx, associateCallInput)
    if err != nil {
        return "", fmt.Errorf("associate route table %s to subnet %s - %w",
                              routeTableId, subnetId, err)
    }

    // If there was not output or it is missing Association ID
    if out == nil || out.AssociationId == nil {
        return "", errors.New("associate route table failed to return association id")
    }

    return aws.ToString(out.AssociationId), nil
}

// Check whether the route table is associated to the given subnet.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - associationId:  The association ID to check for existence
//  - routeTableId:  The ID of the route table to check association
//  - subnetId:  The ID of the subnet to check association
//
// @Returns
//  - Toggle for whether route table association already exists or not
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) RouteTableAssociationExists(callTime time.Duration,
                                                     associationId string,
                                                     routeTableId string,
                                                     subnetId string) (
                                                     bool, error) {
    // Ensure required args are present
    if associationId == "" || routeTableId == "" || subnetId == "" {
        return false, errors.New("associationId or routeTableId or " +
                                 "subnetId is missing")
    }

    // Ensure API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    input := &ec2.DescribeRouteTablesInput{
        Filters: []ec2types.Filter{
            {
                Name:   aws.String("association.route-table-association-id"),
                Values: []string{associationId},
            },
        },
    }

    // Get the route tables associated with the passed in Association ID
    out, err := Ec2Man.client.DescribeRouteTables(ctx, input)
    if err != nil {
        return false, fmt.Errorf("describing route tables - %w", err)
    }

    // If no route tables were returned
    if len(out.RouteTables) == 0 {
        return false, nil
    }

    // Iterate through the retrieved route tables
    for _, rt := range out.RouteTables {
        // Ensure the route table ID matches arg
        if rt.RouteTableId != nil && *rt.RouteTableId != routeTableId {
            continue
        }

        // Iterate through the route tables associations
        for _, assoc := range rt.Associations {
            // If associations ID is present and matches arg
            if assoc.RouteTableAssociationId != nil &&
            *assoc.RouteTableAssociationId == associationId {
                // If associations subnet ID is present and matches arg
                if assoc.SubnetId != nil && *assoc.SubnetId == subnetId {
                    return true, nil
                }
            }
        }
    }

    return false, nil
}

// Provision a association exists between route table and subnet by checking for
// existence and creating one if missing.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - associationId:  The route table and subnet association ID
//  - routeTableId:  The route table ID to be associated
//  - subnetID:  The subnet ID to be associated
//
// @Returns
//  - Route table association ID if the resource is created, "" if it already exists
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) RouteTableAssociationProvision(callTime time.Duration,
                                                        associationId string,
                                                        routeTableId string,
                                                        subnetId string) (
                                                        string, error) {
    // Ensure required args are present
    if routeTableId == "" || subnetId == "" {
        return "", errors.New("routeTableId or subnetId is missing")
    }

    // If the route table associate ID is present in state file
    if associationId != "" {
        // Check to see if it exists in AWS environment
        exists, err := Ec2Man.RouteTableAssociationExists(callTime, associationId,
                                                          routeTableId, subnetId)
        if err != nil {
            return "", err
        }

        // If the route table association exists
        if exists {
            return "", nil
        }
    }

    // Create a new association from route table to subnet
    return Ec2Man.associateRouteTableToSubnet(callTime, routeTableId, subnetId)
}

// Disassociates the route table and subnet.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - associationId:  The route table and subnet association ID
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man Ec2Manger) RouteTableAssociateTerminator(callTime time.Duration,
                                                      associationId string) error {
    // Ensure required arg is present
    if associationId == "" {
        return errors.New("associationId is missing")
    }

    // Ensure API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    disassocCallInput := &ec2.DisassociateRouteTableInput{
        AssociationId: aws.String(associationId),
    }

    // Disassociate subnet from route table
    _, err := Ec2Man.client.DisassociateRouteTable(ctx, disassocCallInput)
    if err != nil {
        return fmt.Errorf("failed to disassociate route table (%s) - %w",
                          associationId, err)
    }

    return nil
}
