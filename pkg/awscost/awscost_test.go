package awscost_test

import (
	"math"
	"testing"
	"time"

	"github.com/ngimb64/Kloud-Kraken/pkg/awscost"
)

// tiny epsilon for float comparisons
func almostEqual(a, b float64) bool {
    return math.Abs(a-b) <= 1e-9
}


// Single test for bytes -> GB/GiB conversion
func TestBytesConversion(t *testing.T) {
	var b int64 = 5_000_000_000 // 5 billion bytes
	gb := awscost.BytesToGB(b)
	gib := awscost.BytesToGiB(b)

	if !almostEqual(gb, 5.0) {
		t.Fatalf("BytesToGB: expected 5.0 got %v", gb)
	}

	expectedGiB := float64(b) / (1024.0*1024.0*1024.0)
	if !almostEqual(gib, expectedGiB) {
		t.Fatalf("BytesToGiB: expected %v got %v", expectedGiB, gib)
	}
}


// ec2_instance: price * hours
func TestCalculateResourceTotal_EC2Hours(t *testing.T) {
	r := &awscost.AwsCostResource{
		Formula:     "price * hours",
		Price:       0.1,
		ServiceName: "ec2_instance",
		StartTime:   time.Now().Add(-3 * time.Hour),
	}

	cost, err := r.CalculateResourceTotal()
	if err != nil {
		t.Fatalf("ec2 calc error: %v", err)
	}

	if !almostEqual(cost, 0.3) {
		t.Fatalf("ec2 cost: expected ~0.3 got %v", cost)
	}
}


// s3_storage: storage_price * gb_months
func TestCalculateResourceTotal_S3Storage(t *testing.T) {
	r := &awscost.AwsCostResource{
		Formula:     "storage_price * gb_months",
		Price:       0.025,
		ServiceName: "s3_storage",
		Gb:          10.0,
		StartTime:   time.Now().Add(-30 * 24 * time.Hour),
	}

	cost, err := r.CalculateResourceTotal()
	if err != nil {
		t.Fatalf("s3 storage calc error: %v", err)
	}

	if !almostEqual(cost, 0.25) {
		t.Fatalf("s3 storage cost: expected 0.25 got %v", cost)
	}
}


// s3_put_requests: put_requests * price_put (metadata)
func TestCalculateResourceTotal_S3PutRequests(t *testing.T) {
	r := &awscost.AwsCostResource{
		Formula:     "put_requests * price_put",
		ServiceName: "s3_put_requests",
		PutRequests: 1000,
		Metadata:    map[string]string{"price_put": "0.005"},
		StartTime:   time.Now().Add(-1 * time.Hour),
	}

	cost, err := r.CalculateResourceTotal()
	if err != nil {
		t.Fatalf("s3 put calc error: %v", err)
	}

	if !almostEqual(cost, 5.0) {
		t.Fatalf("s3 put cost: expected 5.0 got %v", cost)
	}
}


// s3_get_requests: get_requests * price_get (metadata)
func TestCalculateResourceTotal_S3GetRequests(t *testing.T) {
	r := &awscost.AwsCostResource{
		Formula:     "get_requests * price_get",
		ServiceName: "s3_get_requests",
		GetRequests: 2000,
		Metadata:    map[string]string{"price_get": "0.0004"},
		StartTime:   time.Now().Add(-1 * time.Hour),
	}

	cost, err := r.CalculateResourceTotal()
	if err != nil {
		t.Fatalf("s3 get calc error: %v", err)
	}

	if !almostEqual(cost, 0.8) {
		t.Fatalf("s3 get cost: expected 0.8 got %v", cost)
	}
}


// s3_egress: data_out_gb * price_egress
func TestCalculateResourceTotal_S3Egress(t *testing.T) {
	r := &awscost.AwsCostResource{
		Formula:     "data_out_gb * price_egress",
		ServiceName: "s3_egress",
		GbTransfer:  5.0,
		PriceData:   0.09,
		StartTime:   time.Now().Add(-1 * time.Hour),
	}

	cost, err := r.CalculateResourceTotal()
	if err != nil {
		t.Fatalf("s3 egress calc error: %v", err)
	}

	if !almostEqual(cost, 0.45) {
		t.Fatalf("s3 egress cost: expected 0.45 got %v", cost)
	}
}


// vpc_endpoint_ssm_hourly: price_hour * hours
func TestCalculateResourceTotal_VPCEndpointHourly(t *testing.T) {
	r := &awscost.AwsCostResource{
		Formula:     "price_hour * hours",
		Price:       0.01,
		ServiceName: "vpc_endpoint_ssm_hourly",
		StartTime:   time.Now().Add(-24 * time.Hour),
	}

	cost, err := r.CalculateResourceTotal()
	if err != nil {
		t.Fatalf("vpc hourly calc error: %v", err)
	}

	if !almostEqual(cost, 0.24) {
		t.Fatalf("vpc hourly cost: expected 0.24 got %v", cost)
	}
}


// vpc_endpoint_ssm_data: price_gb * gb_processed
func TestCalculateResourceTotal_VPCEndpointData(t *testing.T) {
	r := &awscost.AwsCostResource{
		Formula:     "price_gb * gb_processed",
		Price:       0.01,
		ServiceName: "vpc_endpoint_ssm_data",
		GbTransfer:  1.0,
		StartTime:   time.Now().Add(-1 * time.Hour),
	}

	cost, err := r.CalculateResourceTotal()
	if err != nil {
		t.Fatalf("vpc data calc error: %v", err)
	}

	if !almostEqual(cost, 0.01) {
		t.Fatalf("vpc data cost: expected 0.01 got %v", cost)
	}
}
