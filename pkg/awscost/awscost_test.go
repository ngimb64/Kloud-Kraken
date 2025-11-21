package awscost_test

import (
	"math"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/ngimb64/Kloud-Kraken/pkg/awscost"
	"github.com/stretchr/testify/assert"
)

// Tiny epsilon for float comparisons
//
func almostEqual(a, b float64) bool {
    return math.Abs(a-b) <= 1e-6
}


func TestConversionHelpers(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    var b int64 = 5_000_000_000 // 5 billion bytes
    gb := awscost.BytesToGB(b)
    gib := awscost.BytesToGiB(b)
    assert.True(almostEqual(gb, 5.0))

    expectedGiB := float64(b) / (1024.0*1024.0*1024.0)
    assert.True(almostEqual(gib, expectedGiB))
}


func TestCalculateResourceTotal_EC2Hours(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    resource := &awscost.AwsCostResource{
        Formula:     "price * hours",
        Price:       0.1,
        ServiceName: "ec2_instance",
        StartTime:   time.Now().Add(-3 * time.Hour),
    }

    cost, err := resource.CalculateResourceTotal()
    assert.Equal(nil, err)

    assert.True(almostEqual(cost, 0.3))
}


func TestCalculateResourceTotal_S3Storage(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    resource := &awscost.AwsCostResource{
        Formula:     "storage_price * gb_months",
        Price:       0.025,
        ServiceName: "s3_storage",
        Gb:          10.0,
        StartTime:   time.Now().Add(-30 * 24 * time.Hour),
    }

    cost, err := resource.CalculateResourceTotal()
    assert.Equal(nil, err)

    assert.True(almostEqual(cost, 0.25))
}


func TestCalculateResourceTotal_S3PutRequests(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    resource := &awscost.AwsCostResource{
        Formula:     "put_requests * price_put",
        ServiceName: "s3_put_requests",
        PutRequests: 1000,
        Metadata:    map[string]string{"price_put": "0.005"},
        StartTime:   time.Now().Add(-1 * time.Hour),
    }

    cost, err := resource.CalculateResourceTotal()
    assert.Equal(nil, err)

    assert.True(almostEqual(cost, 5.0))
}


func TestCalculateResourceTotal_S3GetRequests(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    resource := &awscost.AwsCostResource{
        Formula:     "get_requests * price_get",
        ServiceName: "s3_get_requests",
        GetRequests: 2000,
        Metadata:    map[string]string{"price_get": "0.0004"},
        StartTime:   time.Now().Add(-1 * time.Hour),
    }

    cost, err := resource.CalculateResourceTotal()
    assert.Equal(nil, err)

    assert.True(almostEqual(cost, 0.8))
}


func TestCalculateResourceTotal_S3Egress(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    resource := &awscost.AwsCostResource{
        Formula:     "data_out_gb * price_egress",
        ServiceName: "s3_egress",
        GbTransfer:  5.0,
        PriceData:   0.09,
        StartTime:   time.Now().Add(-1 * time.Hour),
    }

    cost, err := resource.CalculateResourceTotal()
    assert.Equal(nil, err)

    assert.True(almostEqual(cost, 0.45))
}


func TestCalculateResourceTotal_VPCEndpointHourly(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    resource := &awscost.AwsCostResource{
        Formula:     "price_hour * hours",
        Price:       0.01,
        ServiceName: "vpc_endpoint_ssm_hourly",
        StartTime:   time.Now().Add(-24 * time.Hour),
    }

    cost, err := resource.CalculateResourceTotal()
    assert.Equal(nil, err)

    assert.True(almostEqual(cost, 0.24))
}


func TestCalculateResourceTotal_VPCEndpointData(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    resource := &awscost.AwsCostResource{
        Formula:     "price_gb * gb_processed",
        Price:       0.01,
        ServiceName: "vpc_endpoint_ssm_data",
        GbTransfer:  1.0,
        StartTime:   time.Now().Add(-1 * time.Hour),
    }

    cost, err := resource.CalculateResourceTotal()
    assert.Equal(nil, err)

    assert.True(almostEqual(cost, 0.01))
}


func TestCalculateTotalCost(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Create a PriceManager with a 1 hour cache TTL
	priceMan := awscost.NewPriceManager(1 * time.Hour)
	priceMan.RegisterProvider(awscost.NewAWSPricingProvider(*aws.NewConfig()))
    // Create the AwsCostManager but use static pricing
    costMan := awscost.NewAwsCostManager(priceMan, nil)

    resource := &awscost.AwsCostResource{
        Formula:     "price * hours",
        Price:       0.1,
        ServiceName: "ec2_instance",
        StartTime:   time.Now().Add(-3 * time.Hour),
    }

    costMan.AwsCostResources = append(costMan.AwsCostResources, *resource)

    resource = &awscost.AwsCostResource{
        Formula:     "storage_price * gb_months",
        Price:       0.025,
        ServiceName: "s3_storage",
        Gb:          10.0,
        StartTime:   time.Now().Add(-30 * 24 * time.Hour),
    }

    costMan.AwsCostResources = append(costMan.AwsCostResources, *resource)

    resource = &awscost.AwsCostResource{
        Formula:     "data_out_gb * price_egress",
        ServiceName: "s3_egress",
        GbTransfer:  5.0,
        PriceData:   0.09,
        StartTime:   time.Now().Add(-1 * time.Hour),
    }

    costMan.AwsCostResources = append(costMan.AwsCostResources, *resource)

    resource = &awscost.AwsCostResource{
        Formula:     "price_hour * hours",
        Price:       0.01,
        ServiceName: "vpc_endpoint_ssm_hourly",
        StartTime:   time.Now().Add(-24 * time.Hour),
    }

    costMan.AwsCostResources = append(costMan.AwsCostResources, *resource)

    // Calculate total cost of all AWS resource in cost manager
    err := costMan.CalculateTotalCost()
    assert.Equal(nil, err)

    assert.True(almostEqual(costMan.TotalCost, 1.24))
}
