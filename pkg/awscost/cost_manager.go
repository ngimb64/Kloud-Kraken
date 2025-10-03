package awscost

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"
)

// Manages AWS cost resources and formulas.
//
type AwsCostManager struct {
    awsCostResources []AwsCostResource
    costTable        map[string]float64
    formulas         map[string]string
    priceManager     *PriceManager
    totalCost        float64
}

// Constructs an initialized cost manager.
//
// @Parameters
//  - priceMan:  The AWS price manager instance
//  - addFormulas:  map of formulas to add to default map
//
// @Returns
//  - The initialized AWS cost manager
//
func NewAwsCostManager(priceMan *PriceManager,
                       addFormulas map[string]string,
                       ) *AwsCostManager {
    formulas := map[string]string{
        // NAT Gateway hourly + data transfer
        "nat_gateway":       "price * hours + price_data * gb_transfer",

        // S3 storage
        "s3_bucket":         "price * gb_months",

        // EC2 instance usage
        "ec2_instance":      "price * hours",

        // Elastic IP (if applicable, hourly)
        "elastic_ip":        "price * hours",

        // VPC endpoints (S3, SSM, etc.), hourly or per-GB pricing
        "vpc_endpoint_s3":   "price * hours",
        "vpc_endpoint_ssm":  "price * hours",

        // VPC Flow Logs (charged per GB of logs ingested)
        "vpc_flow_log":      "price * gb_ingested",
    }


    // If there are formulas to add, add them to map
    if len(addFormulas) > 0 {
        maps.Copy(formulas, addFormulas)
    }

    return &AwsCostManager{
        formulas:     formulas,
        costTable:    make(map[string]float64),
        priceManager: priceMan,
    }
}

// Creates a resource entry and returns a pointer to it so caller can set usage.
//
// @Parameters
//  - serviceName:  The name of the AWS service resource to add to manager
//  - filters:  Filters related to retrieving price for resource
//
// @Returns
//  - Pointer to created resource in slice
//  - Error if it occurs, otherwise nil on success
//
func (costMan *AwsCostManager) AddCostResourceToManager(serviceName string,
                                                        filters map[string]string) (
                                                        *AwsCostResource, error) {
    // Retrieve formula from map based on AWS servie name
    formula, ok := costMan.formulas[serviceName]
    if !ok {
        return nil, fmt.Errorf("service name not found in formula map - %q", serviceName)
    }

    if costMan.priceManager == nil || costMan.priceManager.provider == nil {
        return nil, errors.New("price manager has no registered live provider; " +
                               "register AWSPricingProvider before adding resources")
    }

    // Check the AWS price API for price based on service name and filters
    price, err := costMan.priceManager.GetPrice(context.Background(),
                                                serviceName, filters)
    if err != nil {
        return nil, fmt.Errorf("failed to obtain live price for %s - %w",
                               serviceName, err)
    }

    var priceData float64
    priceVal := price.Value

    // If the price is found in returned metadata
    priceMetadata, ok := price.Metadata["data_unit_price"]
    if ok {
        // Parse the price data from metadata
        parsed, err := parseFloatSafe(priceMetadata)
        if err == nil {
            priceData = parsed
        }
    }

    startTime := time.Now()

    resource := AwsCostResource{
        formula:     formula,
        price:       priceVal,
        priceData:   priceData,
        serviceName: serviceName,
        startTime:   startTime,
    }

    // Append and return pointer to element
    costMan.awsCostResources = append(costMan.awsCostResources, resource)
    idx := len(costMan.awsCostResources) - 1
    return &costMan.awsCostResources[idx], nil
}

// Iterates all resources and accumulates costs into costTable and adds sum.
//
func (costMan *AwsCostManager) CalculateTotalCost() {
    costMan.totalCost = 0
    costMan.costTable = make(map[string]float64)
    var err error

    // Iterate through resources in cost manageer list
    for _, resource := range costMan.awsCostResources {
        // Calculate the cost of the current AWS resource
        cost, cerr := resource.CalculateResourceTotal()
        if err != nil {
            err = errors.Join(err, fmt.Errorf("calculating AWS resource - %w", cerr))
        }

        // Save the cost calulcation result in cost table
        costMan.costTable[resource.serviceName] = cost
        costMan.totalCost += cost
    }
}

// Returns a short summary of costs.
//
func (costMan *AwsCostManager) SummaryString() string {
    return fmt.Sprintf("costs=%+v total=%.6f", costMan.costTable, costMan.totalCost)
}


// parseFloatSafe is a tiny helper to parse strings (used for metadata).
//
// @Parameters
//  - data:  The string to parse the float value from
//
// @Returns
//  - Parsed float value from string
//  - Error if it occurs, otherwise nil on success
//
func parseFloatSafe(data string) (float64, error) {
    var parsedFloat float64

    if data == "" {
        return 0, errors.New("empty data passed in to parse float")
    }

    _, err := fmt.Sscan(data, &parsedFloat)
    return parsedFloat, err
}
