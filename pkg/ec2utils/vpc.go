package ec2utils

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// Creates and waits for the VPC to be created.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - cidrBlock:  The network CIDR block of IP address space to allocate in VPC
//
// @Returns
//  - The ID of the created VPC
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) vpcCreate(callTime time.Duration,
                                   cidrBlock string) (
                                   string, error) {
    // Set context timeout for API call
    ctx, cancel := context.WithTimeout(context.Background(), callTime)

    // Format input for CreateVpc call
    createCallInput := &ec2.CreateVpcInput{
        CidrBlock: aws.String(cidrBlock),
        TagSpecifications: []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeVpc,
                Tags: []ec2types.Tag{
                {
                    Key: aws.String("Name"), Value: aws.String("Kloud-Kraken-VPC"),
                },
            },
        }},
    }

    // Create a new VPC since no valid ID was provided
    createOut, err := Ec2Man.client.CreateVpc(ctx, createCallInput)
    cancel()
    if err != nil {
        return "", err
    }

    vpcId := *createOut.Vpc.VpcId

    // Format input for NewVpcExistsWaiter call
    waiterCallInput := &ec2.DescribeVpcsInput{
        VpcIds: []string{vpcId},
    }

    // Set context timeout for API call
    ctx, cancel = context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Allocate waiter and wait until the VPC is available
    waiter := ec2.NewVpcExistsWaiter(Ec2Man.client)
    err = waiter.Wait(ctx, waiterCallInput, 5 * time.Minute)
    if err != nil {
        return vpcId, err
    }

    return vpcId, nil
}

// Checks to see if the VPC exists.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - vpcID:  The ID of the VPC to ensure exists
//
// @Returns
//  - Boolean to notify whether bucket exists or not
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) VpcExists(callTime time.Duration, vpcId string) (
                                   bool, error) {
    // Set context timeout for API call
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Check to see if the VPC exists
    out, err := Ec2Man.client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
        VpcIds: []string{vpcId},
    })
    if err != nil {
        return false, fmt.Errorf("describe VPC - %w", err)
    }

    // If the ID was identified, exit early
    if len(out.Vpcs) == 0 {
        return false, nil
    }

    return true, nil
}

// Returns VPC ID if it exists, or creates it using supplied CIDR.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - vpcID:  The ID of the VPC to ensure exists
//  - cidrBlock:  The network CIDR block of IP address space to allocate in VPC
//
// @Returns
//  - The ID of VPC if created, otherwise nil
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) VpcProvision(callTime time.Duration, vpcId string,
                                      cidrBlock string) (string, error) {
    // If VPC ID is present in YAML
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

    // Create and wait until VPC is created
    vpcId, err := Ec2Man.vpcCreate(callTime, cidrBlock)
    if err != nil {
        return vpcId, err
    }

    return vpcId, nil
}


// VPCResolverForCIDR returns the VPC resolver IP for the given IPv4 CIDR.
// AWS convention: resolver is network base + 2 (returned as /32).
//
// Examples:
//   "10.1.0.0/16"  -> "10.1.0.2/32"
//   "192.168.0.0/24" -> "192.168.0.2/32"
//
// Returns error if:
//  - cidr is invalid
//  - it's not IPv4
//  - the prefix is too small to contain a +2 host (prefix > 30)
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
