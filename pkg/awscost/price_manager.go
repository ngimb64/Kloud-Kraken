package awscost

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

// Price is the canonical price returned by lookups.
//
type Price struct {
    Currency  string
    Metadata  map[string]string
    Retrieved time.Time
    Unit      string
    Value     float64
}

// Pluggable provider interface for live price lookup.
//
type PriceProvider interface {
    GetPrice(ctx context.Context, service string,
             filters map[string]string) (Price, error)
}

// A minimal sequential-only price lookup façade.
//
type PriceManager struct {
    cache    map[string]Price
    ttl      time.Duration
    provider PriceProvider
}

// Creates price manager, ttl is the cache TTL.
//
// @Parameters
//  - ttl:  The Time To Live duration
//
// @Returns
//  - The initialize price manager
//
func NewPriceManager(ttl time.Duration) *PriceManager {
    return &PriceManager{
        cache: make(map[string]Price),
        ttl:   ttl,
    }
}

// Registers a PriceProvider that will be used on cache misses.
//
// @Parameters
//  - priceProvider:  The price provider to register
//
func (priceMan *PriceManager) RegisterProvider(priceProvider PriceProvider) {
    priceMan.provider = priceProvider
}

// Deterministically builds a key from service and filters for caching.
//
// @Parameters
//  - service:  The name of service used in the key
//  - filters:  Filters related to retrieving price for resource
//
// @Returns
//  - The created key for cahce
//
func buildKey(service string, filters map[string]string) string {
    // If there are no filters
    if len(filters) == 0 {
        return service + "|"
    }

    keys := make([]string, 0, len(filters))

    // Iterate through filters and add keys to list
    for key := range filters {
        keys = append(keys, key)
    }

    // Sort the list of keys
    sort.Strings(keys)
    parts := make([]string, 0, len(keys)+1)
    // Format the beginning of the key
    parts = append(parts, service)

    // Iterate through list of keys and build filter key
    for _, key := range keys {
        parts = append(parts, key + "=" + filters[key])
    }

    // Join the parts into one string by delimiter
    return strings.Join(parts, "|")
}

// Checks cache then calls the registered provider (if any) for a live price. If
// no provider is registered, or provider returns an error, this returns an error.
//
// @Parameters
//  - ctx:  The context handler for GetPrice request
//  - service:  The name of the service to query
//  - filters:  Filters related to retrieving price for resource
//
// @Returns
//  - The retrieved resource price
//  - Error if it occurs, otherwise nil on success
//
func (priceMan *PriceManager) GetPrice(ctx context.Context, service string,
                                 filters map[string]string) (
                                 Price, error) {
    // Build the cache key
    key := buildKey(service, filters)

    // Cache hit and fresh
    price, ok := priceMan.cache[key]
    if ok {
        if time.Since(price.Retrieved) < priceMan.ttl {
            return price, nil
        }
    }

    // Ensure provider for live lookup is present
    if priceMan.provider == nil {
        return Price{}, errors.New("no price provider registered (register " +
                                   "AWSPricingProvider to fetch live prices)")
    }

    // Ask provider for live price
    price, err := priceMan.provider.GetPrice(ctx, service, filters)
    if err != nil {
        return Price{}, err
    }

    // Cache and return
    price.Retrieved = time.Now()
    priceMan.cache[key] = price
    return price, nil
}
