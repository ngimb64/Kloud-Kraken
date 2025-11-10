package ec2utils

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
)

// Creates and waits for the VPC to be created.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - cidrBlock:  The network CIDR block of IP address space to allocate in VPC
//  - tags:  String map of tag key-values to configure
//
// @Returns
//  - The ID of the created VPC
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) vpcCreate(callTime time.Duration, cidrBlock string,
                                   tags map[string]string) (string, error) {
    // Ensure API calls do not hang for than longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    createCallInput := &ec2.CreateVpcInput{
        CidrBlock: aws.String(cidrBlock),
    }

    if len(tags) > 0 {
        createCallInput.TagSpecifications = []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeVpc,
                Tags: awsutils.BuildEc2Tags(tags),
            },
        }
    }

    // Create a new VPC since no valid ID was provided
    createOut, err := Ec2Man.client.CreateVpc(ctx, createCallInput)
    if err != nil {
        return "", err
    }

    vpcId := *createOut.Vpc.VpcId

    waiterCallInput := &ec2.DescribeVpcsInput{
        VpcIds: []string{vpcId},
    }

    // Allocate waiter and wait until the VPC exists
    waiter := ec2.NewVpcExistsWaiter(Ec2Man.client)
    err = waiter.Wait(ctx, waiterCallInput, callTime)
    if err != nil {
        return vpcId, err
    }

    // Enable DNS support so interface endpoints with Private DNS work
    _, err = Ec2Man.client.ModifyVpcAttribute(ctx, &ec2.ModifyVpcAttributeInput{
        VpcId: aws.String(vpcId),
        EnableDnsSupport: &ec2types.AttributeBooleanValue{Value: aws.Bool(true)},
    })
    if err != nil {
        return vpcId, fmt.Errorf("failed to enable DNS support - %w", err)
    }

    // Enable DNS hostnames VPC attribute
    _, err = Ec2Man.client.ModifyVpcAttribute(ctx, &ec2.ModifyVpcAttributeInput{
        VpcId: aws.String(vpcId),
        EnableDnsHostnames: &ec2types.AttributeBooleanValue{Value: aws.Bool(true)},
    })
    if err != nil {
        return vpcId, fmt.Errorf("failed to enable DNS hostnames - %w", err)
    }

    return vpcId, nil
}

// Checks to see if the VP ID exists.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - vpcID:  The ID of the VPC to ensure exists
//
// @Returns
//  - Toggle for whether VPC already exists or not
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) VpcExists(callTime time.Duration, vpcId string) (
                                   bool, error) {
    // Ensure required arg is present
    if vpcId == "" {
        return false, errors.New("vpcId is missing")
    }

    // Ensure API calls do not hang for than longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.DescribeVpcsInput{
        VpcIds: []string{vpcId},
    }

    // Check to see if the VPC exists based on ID
    out, err := Ec2Man.client.DescribeVpcs(ctx, callInput)
    if err != nil {
        var apiErr smithy.APIError

        // If the VPC ID was not found
        if errors.As(err, &apiErr) &&
        apiErr.ErrorCode() == "InvalidVpcID.NotFound" {
            return false, nil
        }

        // If a non API related error occured during request
        return false, fmt.Errorf("describe VPC - %w", err)
    }

    // If the ID was identified, exit early
    if out == nil || len(out.Vpcs) == 0 {
        return false, nil
    }

    return true, nil
}

// Provision VPC by checking for existence and creating if missing.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - vpcID:  The ID of the VPC to ensure exists
//  - cidrBlock:  The network CIDR block of IP address space to allocate in VPC
//  - tags:  String map of tag key-values to configure
//
// @Returns
//  - The ID of VPC if created, otherwise nil
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) VpcProvision(callTime time.Duration, vpcId string,
                                      cidrBlock string, tags map[string]string) (
                                      string, error) {
    // Ensure required args are present
    if cidrBlock == "" {
        return "", errors.New("cidrBlock is missing")
    }

    // If VPC ID is present in state file
    if vpcId != "" {
        // Check to see if it exists in AWS enviroment
        vpcExists, err := Ec2Man.VpcExists(callTime, vpcId)
        if err != nil {
            return "", err
        }

        // If the VPC already exists, exit early
        if vpcExists {
            return "", nil
        }
    }

    // Wait until VPC is created
    return Ec2Man.vpcCreate(callTime, cidrBlock, tags)
}


// Returns the VPC resolver IP for the given IPv4 CIDR which for AWS is commonly
// network base + 2 (returned as /32).
//
// Examples:
//   "10.1.0.0/16"  -> "10.1.0.2/32"
//   "192.168.0.0/24" -> "192.168.0.2/32"
//
// @Parameters
//  - cidr:  Network CIDR to calculate VPC resolver
//
// @Returns
//  - The network CIDR of the VPC resolver (always /32)
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) VpcResolverForCidr(cidr string) (string, error) {
    // Parse the IP and network address with CIDR
    ip, ipnet, err := net.ParseCIDR(cidr)
    if err != nil {
        return "", fmt.Errorf("parse cidr %q - %w", cidr, err)
    }

    // Converts IP address to 4 byte representation
    ip4 := ip.To4()
    if ip4 == nil {
        return "", fmt.Errorf("only IPv4 CIDRs supported - %s", cidr)
    }

    // Get the CIDR size
    ones, bits := ipnet.Mask.Size()
    if bits != 32 {
        return "", fmt.Errorf("unexpected mask size for %s", cidr)
    }

    // Ensure CIDR is less than /30
    if ones > 30 {
        return "", fmt.Errorf("cidr %s is too small (/%d) to compute resolver" +
                              " (needs prefix <= /30)", cidr, ones)
    }

    // Convert network base to uint32
    networkStart := binary.BigEndian.Uint32(ip4)

    // Compute resolver address in network
    resolver := networkStart + 2

    // Compute broadcast for sanity check
    mask := ^uint32(0) << (32-ones)
    broadcast := networkStart | ^mask

    if resolver > broadcast {
        return "", fmt.Errorf("computed resolver address outside CIDR %s", cidr)
    }

    // Put resolver result in buffer big endian
    resIP := make(net.IP, 4)
    binary.BigEndian.PutUint32(resIP, resolver)

    return resIP.String() + "/32", nil
}


// Deletes the VPC.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - vpcId:  The VPC ID
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man Ec2Manger) VpcTerminator(callTime time.Duration,
                                      vpcId string) error {
    // Ensure required arg is present
    if vpcId == "" {
        return errors.New("vpcId is missing")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    deleteCallInput := &ec2.DeleteVpcInput{
        VpcId: aws.String(vpcId),
    }

    // Delete the VPC by passed in ID
    _, err := Ec2Man.client.DeleteVpc(ctx, deleteCallInput)
    if err != nil {
        return fmt.Errorf("failed to delete VPC %s - %w", vpcId, err)
    }

    return nil
}
