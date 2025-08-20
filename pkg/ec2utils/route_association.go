package ec2utils

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// Create an association between a route table and a subnet and return the association id.
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) AssociateRouteTableToSubnet(callTime time.Duration,
                                                     routeTableId string,
                                                     subnetId string) (
                                                     string, error) {
    // Ensure required args are present
    if routeTableId == "" || subnetId == "" {
        return "", fmt.Errorf("routeTableId and subnetId are required")
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

    if out == nil || out.AssociationId == nil {
        return "", fmt.Errorf("associate route table returned empty association id")
    }

    return aws.ToString(out.AssociationId), nil
}

// Check whether the route table is associated to the given subnet and
// return the association id if present.
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) AssociationExists(callTime time.Duration,
                                           associationId string,
                                           routeTableId string,
                                           subnetId string) (
                                           bool, error) {
    // Ensure required args are present
    if routeTableId == "" || subnetId == "" {
        return false, fmt.Errorf("routeTableId and subnetId are required")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    input := &ec2.DescribeRouteTablesInput{
        RouteTableIds: []string{routeTableId},
    }

    // Describe the route table and inspect associations
    out, err := Ec2Man.client.DescribeRouteTables(ctx, input)
    if err != nil {
        return false, fmt.Errorf("describe route tables - %w", err)
    }

    if len(out.RouteTables) == 0 {
        return false, nil
    }

    // Iterate through the identified route tables
    for _, rt := range out.RouteTables {
        // Iterate through the route table associates
        for _, assoc := range rt.Associations {
            // If the association subnet ID and association ID matches args
            if *assoc.SubnetId == subnetId &&
            *assoc.RouteTableAssociationId == associationId {
                return true, nil
            }
        }
    }

    return false, nil
}

// Ensure a route table association exists between routeTableId and subnetId.
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) RouteTableAssociationProvision(callTime time.Duration,
                                                        associationId string,
                                                        routeTableId string,
                                                        subnetId string) (
                                                        string, error) {
    // If the route table associate ID is present in YAML
    if associationId != "" {
        // If association already exists return it
        exists, err := Ec2Man.AssociationExists(callTime, associationId,
                                                routeTableId, subnetId)
        if err != nil {
            return "", err
        }

        if exists {
            return "", nil
        }
    }

    // Create a new association
    associationId, err := Ec2Man.AssociateRouteTableToSubnet(callTime, routeTableId,
                                                             subnetId)
    if err != nil {
        return "", err
    }

    return associationId, nil
}
