package cidrutils

import (
	"encoding/binary"
	"fmt"
	"net"
)

// Finds the first unused subnet of prefix length inside passed in CIDR and
// records it to prevent trying to reallocate it in further invocations.
//
// Example - allocates two subnets splitting /24 CIDR in half:
//   alloc := map[string]struct{}{}
//   subnet1, err := AllocateNextSubnet("192.168.0.0/24", alloc, 25)
//   subnet2, err := AllocateNextSubnet("192.168.0.0/24", alloc, 25)
//
// @Parameters
//  - vpcCidr:  The base CIDR network of the VPC
//  - allocated:  Map used to ensure unique CIDR allocation for subnets
//  - prefixLen:  The length the CIDR prefix of subnet to allocate
//
// @Returns
//  - The next available subnet to allocate
//  - Error if it occurs, otherwise nil on success
//
func AllocateNextSubnet(vpcCidr string, allocated map[string]struct{},
                        prefixLen int) (string, error) {

    // Parse and validate the base VPC CIDR
    _, vpcNet, err := net.ParseCIDR(vpcCidr)
    if err != nil {
        return "", fmt.Errorf("invalid vpc cidr - %w", err)
    }

    // Ensure the requested prefix fits inside the VPC range
    vpcOnes, _ := vpcNet.Mask.Size()
    if prefixLen < vpcOnes || prefixLen > 32 {
        return "", fmt.Errorf("prefixLen must be between %d and 32", vpcOnes)
    }

    // Compute numeric base address and per-subnet size
    base := ipToUint32(vpcNet.IP.Mask(vpcNet.Mask))
    subnetSize := uint32(1) << (32 - prefixLen)
    // Get the number of candidate subnets inside the base
    numSubnets := 1 << (prefixLen - vpcOnes)

    // Convert existing allocations into parsed nets for overlap checks
    allocatedNets := make([]*net.IPNet, 0, len(allocated))
    for k := range allocated {
        // Parse the network CIDR
        _, anet, perr := net.ParseCIDR(k)
        if perr != nil {
            // Skip malformed entries so they do not block allocation
            continue
        }
        allocatedNets = append(allocatedNets, anet)
    }

    // Iterate candidate subnets in sequence from the base address
    for i := range numSubnets {
        candidateIP := base + uint32(i)*subnetSize
        candidate := &net.IPNet{
            IP:   uint32ToIP(candidateIP),
            Mask: net.CIDRMask(prefixLen, 32),
        }

        // Ensure candidate lies inside the VPC CIDR boundary
        if !vpcNet.Contains(candidate.IP) {
            continue
        }

        overlap := false
        // Check candidate against all allocated subnets for overlap
        for _, anet := range allocatedNets {
            if cidrOverlap(candidate, anet) {
                overlap = true
                break
            }
        }
        if overlap {
            continue
        }

        // Reserve and return the first available candidate
        cidrStr := candidate.String()
        allocated[cidrStr] = struct{}{}
        return cidrStr, nil
    }

    // No suitable subnet found inside the provided VPC CIDR
    return "", fmt.Errorf("no available /%d subnets left inside %s", prefixLen, vpcCidr)
}


// Returns true if subnet A and B overlap in address space.
//
// @Parameters
// - subnetA:  The first subnet to compare for overlap
// - subnetB:  The second subnet to compare for overlap
//
// @Returns
// - True/False whether the subnet overlaps or not
//
func cidrOverlap(subnetA *net.IPNet, subnetB *net.IPNet) bool {
    // Convert subnet A network address to an IPv4 uint32 value
    aStart := ipToUint32(subnetA.IP.Mask(subnetA.Mask))
    // Compute the last IP of subnet A as start plus size minus one
    aEnd := aStart + uint32(1<<(32-maskOnes(subnetA.Mask))) - 1
    // Convert subnet B network address to an IPv4 uint32 value
    bStart := ipToUint32(subnetB.IP.Mask(subnetB.Mask))
    // Compute the last IP of subnet B as start plus size minus one
    bEnd := bStart + uint32(1<<(32-maskOnes(subnetB.Mask))) - 1

    // Return true when the two IP ranges overlap and false when they do not
    return !(aEnd < bStart || bEnd < aStart)
}


// IP address to convert to 4 byte representation.
//
// @Parameters
//  - ip:  IP address to convert
//
// @Returns
//  - Converted 4 byte represntation of IP address
//
func ipToUint32(ip net.IP) uint32 {
    // Convert IP address to 4 byte representation
    ip = ip.To4()

    // If the passed in IP is not of IP format
    if ip == nil {
        return 0
    }

    return binary.BigEndian.Uint32(ip)
}


// Get the number of leading ones in the subnet mask.
//
// @Parameters
//  - mask:  The subnet mask to get the number of leading ones
//
// @Returns
//  - The number of leading ones in subnet mask
//
func maskOnes(mask net.IPMask) int {
    ones, _ := mask.Size()
    return ones
}


// Parses and returns the prefix length for both IPv4 & IPv6.
//
// Example - parse 24 from 192.168.0.0/24:
//   prefixLen, err := PrefixFromCidr("192.168.0.0/24")
//
// @Parameters
//  - cidr:  CIDR to parse prefix length from
//
// @Returns
//  - Prefix length parsed from input CIDR
//  - Error if it occurs, otherwise nil on success
//
func PrefixFromCidr(cidr string) (int, error) {
    // Parse the network CIDR
    _, ipnet, err := net.ParseCIDR(cidr)
    if err != nil {
        return 0, fmt.Errorf("invalid cidr %q - %w", cidr, err)
    }

    // Get the number of leading ones in the subnet mask
    ones := maskOnes(ipnet.Mask)
    return ones, nil
}


// Convert uint32 to IP address.
//
// @Parameters
//  - num:  uint32 to be converted
//
// @Returns
//  - Converted IP address
//
func uint32ToIP(num uint32) net.IP {
    ip := make([]byte, 4)
    binary.BigEndian.PutUint32(ip, num)
    return net.IP(ip)
}
