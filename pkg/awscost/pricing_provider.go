package awscost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	pricing "github.com/aws/aws-sdk-go-v2/service/pricing"
	pricetypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
)

// Implements PriceProvider using AWS Pricing GetProducts API.
// Returns the first OnDemand USD price it can parse (best-effort).
//
type AWSPricingProvider struct {
    region              string
    serviceNameToSkuMap map[string]string
}

// Constructs a provider, defaults to "us-east-1" if blank.
//
// @Parameters
//  - region:  The region to use for querying pricing API
//
// @Returns
//  - Pointer to AWS pricing provider
//
func NewAWSPricingProvider(region string) *AWSPricingProvider {
    if region == "" {
        region = "us-east-1"
    }

    // Maps friendly names to AWS SKU service codes
    serviceNameToSkuMap := map[string]string{
        "ec2_instance":    "AmazonEC2",

        "s3_egress":       "AmazonS3",
        "s3_get_requests": "AmazonS3",
        "s3_put_requests": "AmazonS3",
        "s3_storage":      "AmazonS3",

        "vpc_endpoint_ssm_data":   "AmazonEC2",
        "vpc_endpoint_ssm_hourly": "AmazonEC2",
    }

    return &AWSPricingProvider{
        region: region,
        serviceNameToSkuMap: serviceNameToSkuMap,
    }
}

// Implements PriceProvider by querying AWS Pricing GetProducts and extracting
// pricePerUnit->USD.
//
// @Parameters
//  - ctx:  The context handler for SDK API call
//  - serviceOrCode:  The service name or its SKU code for price lookup
//  - filters:  Filters related to retrieving price for resource
//
// @Returns
//  - AWS pricing struct
//  - Error if it occurs, otherwise nil on success
//
func (pricingProvider *AWSPricingProvider) GetPrice(ctx context.Context,
                                                    serviceOrCode string,
                                                    filters map[string]string) (
                                                    Price, error) {
    region := pricingProvider.region
    serviceCode := serviceOrCode

    // Attempt to get SKU code from map by service name
    code, ok := pricingProvider.serviceNameToSkuMap[serviceOrCode]
    if ok {
        serviceCode = code
    }

    // Load AWS config (use pricing endpoint region)
    cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
    if err != nil {
        return Price{}, fmt.Errorf("loading aws config - %w", err)
    }

    // Establish client to pricing API
    client := pricing.NewFromConfig(cfg)
    var sdkFilters []pricetypes.Filter

    // ensure service code is set for safety
    sdkFilters = append(sdkFilters, pricetypes.Filter{
        Field: aws.String("servicecode"),
        Type:  pricetypes.FilterTypeTermMatch,
        Value: aws.String(serviceCode),
    })

    // Iterate through the map of filters
    for key, value := range filters {
        if key == "" || value == "" {
            continue
        }

        // Add filter to filters slice
        sdkFilters = append(sdkFilters, pricetypes.Filter{
            Field: aws.String(key),
            Type:  pricetypes.FilterTypeTermMatch,
            Value: aws.String(value),
        })
    }

    input := &pricing.GetProductsInput{
        ServiceCode:   aws.String(serviceCode),
        Filters:       sdkFilters,
        FormatVersion: aws.String("aws_v1"),
        MaxResults:    aws.Int32(100),
    }

    // Get full pricing list in paginator fashion
    paginator := pricing.NewGetProductsPaginator(client, input)

    // While there are more pages in paginator
    for paginator.HasMorePages() {
        // Get the next page in paginator
        out, err := paginator.NextPage(ctx)
        if err != nil {
            return Price{}, fmt.Errorf("pricing GetProducts page - %w", err)
        }

        // Iterate through products in price list
        for _, product := range out.PriceList {
            var data map[string]any
            meta := make(map[string]string)

            // Unmarshall the JSON data to map
            err := json.Unmarshal([]byte(product), &data)
            if err != nil {
                continue
            }

            // Check to see if the product node exists
            productNode, ok := data["product"].(map[string]any)
            if ok {
                // Check to see if the SKU exists in product node
                sku, ok := productNode["sku"].(string)
                if ok {
                    meta["sku"] = sku
                }

                // Check to see if attributes exist in product node
                attributes, ok := productNode["attributes"].(map[string]any)
                if ok {
                    // Iterate through key-values in attributes map
                    for key, value := range attributes {
                        // Assert value as string
                        strVal, ok := value.(string)
                        if ok {
                            meta[key] = strVal
                        }
                    }
                }
            }

            // Check to see if terms exist
            terms, ok := data["terms"].(map[string]any)
            if ok {
                // Check to see if On-Demand pricing entries exist in terms
                onDemand, ok := terms["OnDemand"].(map[string]any)
                if ok {
                    // Iterate through the On-Demand pricing entries
                    for _, skuTerm := range onDemand {
                        // Type assert on SKU in term
                        skuMap, ok := skuTerm.(map[string]any)
                        if ok {
                            // Type assert on price dimenstions in SKU map
                            priceDims, ok := skuMap["priceDimensions"].(map[string]any)
                            if ok {
                                // Iterate through the price dimensions
                                for _, priceDim := range priceDims {
                                    // Type assert on price dimension
                                    pdMap, ok := priceDim.(map[string]any)
                                    if ok {
                                        // Type assert on price per unit in price dimension
                                        ppu, ok := pdMap["pricePerUnit"].(map[string]any)
                                        if ok {
                                            // Type assert on dollar value
                                            usdStr, ok := ppu["USD"].(string)
                                            if ok && usdStr != "" {
                                                var vfloat float64

                                                // Convert dollar string to float
                                                _, err := fmt.Sscan(usdStr, &vfloat)
                                                if err != nil || vfloat <= 0 {
                                                    continue
                                                }

                                                unit := ""

                                                // Type assert on unit in price dimension
                                                unitIf, ok := pdMap["unit"].(string)
                                                if ok {
                                                    unit = unitIf
                                                }

                                                return Price{
                                                    Value:     vfloat,
                                                    Unit:      unit,
                                                    Currency:  "USD",
                                                    Metadata:  meta,
                                                    Retrieved: time.Now(),
                                                }, nil
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    return Price{}, errors.New("no usable OnDemand price found for given filters" +
                               " (try adding more precise filters)")
}
