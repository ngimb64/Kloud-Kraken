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
    AwsCostResources []AwsCostResource
    CostTable        map[string]float64
    Formulas         map[string]string
    priceManager     *PriceManager
    TotalCost        float64
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
        "ec2_instance":    "price * hours",

        "s3_egress":       "data_out_gb * price_egress",
        "s3_get_requests": "get_requests * price_get",
        "s3_put_requests": "put_requests * price_put",
        "s3_storage":      "storage_price * gb_months",

        "vpc_endpoint_ssm_data":   "price_gb * gb_processed",
        "vpc_endpoint_ssm_hourly": "price_hour * hours",
    }

    // If there are formulas to add, add them to map
    if len(addFormulas) > 0 {
        maps.Copy(formulas, addFormulas)
    }

    return &AwsCostManager{
        CostTable:    make(map[string]float64),
        Formulas:     formulas,
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
func (costMan *AwsCostManager) addCostResourceToManager(serviceName string,
                                                        filters map[string]string,
                                                        startNow bool) (
                                                        *AwsCostResource, error) {
    // Retrieve formula from map based on AWS servie name
    formula, ok := costMan.Formulas[serviceName]
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

    var startTime time.Time

    // If the resource is to be started immediately
    if startNow {
        startTime = time.Now()
    }

    resource := AwsCostResource{
        Formula:     formula,
        Metadata:    price.Metadata,
        Price:       priceVal,
        PriceData:   priceData,
        ServiceName: serviceName,
        StartTime:   startTime,
    }

    // Append and return pointer to element
    costMan.AwsCostResources = append(costMan.AwsCostResources, resource)
    idx := len(costMan.AwsCostResources) - 1
    return &costMan.AwsCostResources[idx], nil
}

// Handler for adding cost resource to cost manager, adds any errors that
// occur to error string.
//
// @Parameters
//  - serviceName:  The name of the AWS resource to add to the cost manager
//  - filterMap:  The map used to stored filters applied to cache key
//  - startNow:  Toggler for whether to start resource timer immediately
//  - err:  The error string to add any errors that occur to
//
// @Returns
//  - Pointer to the AWS resource added to cost manager
//  - Error if it occurs, otherwise nil on success
//
func (costMan *AwsCostManager) AddCostResourceToManager(serviceName string,
                                                        filterMap map[string]string,
                                                        startNow bool, err *error) (
                                                        *AwsCostResource) {
    // Add the cost resource to cost manager
    resource, addErr := costMan.addCostResourceToManager(serviceName, filterMap, startNow)
    if addErr != nil {
        *err = errors.Join(*err, fmt.Errorf("adding resource to cost manager - %w", addErr))
    }

    return resource
}

// Iterates all resources and accumulates costs into costTable and adds sum.
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (costMan *AwsCostManager) CalculateTotalCost() error {
    var err error

    // Iterate through resources in cost manageer list
    for _, resource := range costMan.AwsCostResources {
        // Calculate the cost of the current AWS resource
        cost, cerr := resource.CalculateResourceTotal()
        if cerr != nil {
            err = errors.Join(err, fmt.Errorf("calculating AWS resource - %w", cerr))
            cost = -1
        }

        // Save the cost calulcation result in cost table
        costMan.CostTable[resource.ServiceName] = cost
        if cerr != nil {
            continue
        }

        costMan.TotalCost += cost
    }

    return err
}

// Returns a short summary of costs.
//
func (costMan *AwsCostManager) SummaryString() string {
    return fmt.Sprintf("costs=%+v total=%.6f", costMan.CostTable, costMan.TotalCost)
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
