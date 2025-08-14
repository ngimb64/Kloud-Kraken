package cidrutils

import (
	"encoding/binary"
	"fmt"
	"net"
)

// AllocateNextSubnet finds the first unused subnet of prefixLen inside vpcCIDR,
// records it into the allocated map (key = CIDR string), and returns it.
//
// allocated is mutated in-place. Use map[string]struct{}{} as the map type.
// Example:
//   alloc := map[string]struct{}{}
//   cidr, err := AllocateNextSubnet("10.0.0.0/24", alloc, 25)

//
//
// @Parameters
//
//
// @Returns
//
//
func AllocateNextSubnet(vpcCIDR string, allocated map[string]struct{}, prefixLen int) (string, error) {
    _, vpcNet, err := net.ParseCIDR(vpcCIDR)
    if err != nil {
        return "", fmt.Errorf("invalid vpc cidr - %w", err)
    }

    vpcOnes, _ := vpcNet.Mask.Size()
    if prefixLen < vpcOnes || prefixLen > 32 {
        return "", fmt.Errorf("prefixLen must be between %d and 32", vpcOnes)
    }

    // helper: IPv4 only
    base := ipToUint32(vpcNet.IP.Mask(vpcNet.Mask))
    subnetSize := uint32(1) << (32 - prefixLen)
    numSubnets := 1 << (prefixLen - vpcOnes) // safe since prefixLen >= vpcOnes

    // Parse allocated entries into nets for overlap checks.
    allocatedNets := make([]*net.IPNet, 0, len(allocated))
    for k := range allocated {
        _, anet, perr := net.ParseCIDR(k)
        if perr != nil {
            // ignore malformed entries in the map
            continue
        }

        allocatedNets = append(allocatedNets, anet)
    }

    for i := range numSubnets {
        candidateIP := uint32(base) + uint32(i)*subnetSize
        candidate := &net.IPNet{
            IP:   uint32ToIP(candidateIP),
            Mask: net.CIDRMask(prefixLen, 32),
        }

        // sanity: make sure candidate is inside VPC network (should be, but double-check)
        if !vpcNet.Contains(candidate.IP) {
            continue
        }

        // check overlap with allocated
        overlap := false

        for _, anet := range allocatedNets {
            if cidrOverlap(candidate, anet) {
                overlap = true
                break
            }
        }

        if overlap {
            continue
        }

        // found available subnet — record and return
        cidrStr := candidate.String()
        allocated[cidrStr] = struct{}{}
        return cidrStr, nil
    }

    return "", fmt.Errorf("no available /%d subnets left inside %s", prefixLen, vpcCIDR)
}


// cidrOverlap returns true if a and b overlap in address space.

//
//
// @Parameters
//
//
// @Returns
//
//
func cidrOverlap(a, b *net.IPNet) bool {
    aStart := ipToUint32(a.IP.Mask(a.Mask))
    aEnd := aStart + uint32(1<<(32-maskOnes(a.Mask))) - 1
    bStart := ipToUint32(b.IP.Mask(b.Mask))
    bEnd := bStart + uint32(1<<(32-maskOnes(b.Mask))) - 1

    return !(aEnd < bStart || bEnd < aStart)
}


//
//
// @Parameters
//
//
// @Returns
//
//
func ipToUint32(ip net.IP) uint32 {
    ip = ip.To4()

    if ip == nil {
        return 0
    }

    return binary.BigEndian.Uint32(ip)
}


//
//
// @Parameters
//
//
// @Returns
//
//
func maskOnes(m net.IPMask) int {
    ones, _ := m.Size()
    return ones
}


// PrefixFromCIDR returns the prefix length (e.g. 24 for "10.0.0.0/24").
// Works for both IPv4 and IPv6 (IPv6 prefixes up to 128).

//
//
// @Parameters
//
//
// @Returns
//
//
func PrefixFromCidr(cidr string) (int, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0, fmt.Errorf("invalid cidr %q: %w", cidr, err)
	}

    ones, _ := ipnet.Mask.Size()
	return ones, nil
}


//
//
// @Parameters
//
//
// @Returns
//
//
func uint32ToIP(n uint32) net.IP {
    b := make([]byte, 4)
    binary.BigEndian.PutUint32(b, n)
    return net.IP(b)
}
