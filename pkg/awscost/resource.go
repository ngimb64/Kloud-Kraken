package awscost

import (
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/expr-lang/expr"
)

// AwsCostResource represents a tracked resource and usage fields.
//
type AwsCostResource struct {
    Formula     string
    Metadata    map[string]string
    Price       float64  // Primary price (e.g., $/hour or $/GB-month)
    PriceData   float64  // Optional second price (e.g., $/GB transfer)
    ServiceName string
    StartTime   time.Time
    timeUsed    time.Duration

    // Usage fields
    Gb          float64  // GB stored
    gbMonths    float64  // Precomputed GB-months
    GetRequests float64  // Number of GET requests
    GbTransfer  float64  // GB transferred
    PutRequests float64  // Number of PUT requests
    Requests    float64  // General number of requests
}

// CalculateResourceTotal computes the resource cost using expr formulas.
//
// @Returns
//	- Returns resource total cost unless it fails, which defaults to 0
//  - Error if it occurs, otherwise nil on success
//
func (resource *AwsCostResource) CalculateResourceTotal() (float64, error) {
    // If timer was not called ensure it is to prevent errors
    if resource.StartTime.IsZero() {
        resource.StartTime = time.Now()
    }

    resource.timeUsed = time.Since(resource.StartTime)

    // Get hours used and calculate out of a month
    hours := resource.timeUsed.Hours()
    months := hours / (24.0 * 30.0)

    gbMonths := resource.gbMonths
    // If there are no GB month, calculate them
    if gbMonths <= 0 {
        gbMonths = resource.Gb * months
    }

    defaults := map[string]any{
        "price":         0.0,  // Default primary price
        "price_data":    0.0,  // Secondary price (if any)
        "price_egress":  0.0,
        "price_gb":      0.0,
        "price_hour":    0.0,
        "price_put":     0.0,
        "price_get":     0.0,
        "storage_price": 0.0,

        "data_out_gb":  resource.GbTransfer,
        "gb":           resource.Gb,
        "gb_months":    gbMonths,
        "gb_processed": resource.GbTransfer,
        "gb_transfer":  resource.GbTransfer,
        "get_requests": resource.GetRequests,
        "hours":        hours,
        "months":       months,
        "put_requests": resource.PutRequests,
        "requests":     resource.Requests,
    }

    // Start with the defaults
    env := make(map[string]any, len(defaults))
    maps.Copy(env, defaults)

    // Ensure all formula aliases are set
    env["price"] = resource.Price
    env["price_data"] = resource.PriceData
    env["storage_price"] = resource.Price       // alias
    env["price_egress"] = resource.PriceData    // alias
    env["price_hour"] = resource.Price
    env["price_gb"] = resource.Price
    env["price_data_unit"] = resource.PriceData

    if resource.Metadata != nil {
        // Add parsed numeric metadata (if present), add both raw string and
        // parsed numeric as underscored keys, e.g. meta_put_price -> 0.005
        for key, value := range resource.Metadata {
            // Make canonical lowercased key with underscores for spaces/dashes
            normKey := strings.ToLower(key)
            normKey = strings.ReplaceAll(normKey, " ", "_")
            normKey = strings.ReplaceAll(normKey, "-", "_")

            // Put raw string metadata entry in env under "meta_<key>"
            env["meta_" + normKey] = value

            // Try to parse numeric values and add them as
            // float env entries (if numeric)
            parsed, err := parseFloatSafe(value)
            if err == nil {
                // Add with and without price_ prefix
                // if key suggests pricing
                env[normKey] = parsed
                env["price_"+normKey] = parsed
            }
        }
    }

    // Compile the cost formula to be evaluated
    prog, err := expr.Compile(resource.Formula, expr.AsFloat64())
    if err != nil {
        return 0, fmt.Errorf("failed to compile formula for %s - %v (formula: %q)",
                             resource.ServiceName, err, resource.Formula)
    }

    // Run the cost forumla to generate resource cost
    costCalcOut, err := expr.Run(prog, env)
    if err != nil {
        return 0, fmt.Errorf("failed to run formula for %s - %v (formula: %q)",
                             resource.ServiceName, err, resource.Formula)
    }

    // Accept numeric types convertible to float64
    switch value := costCalcOut.(type) {
    case float64:
        return value, nil
    case float32:
        return float64(value), nil
    case int:
        return float64(value), nil
    case int64:
        return float64(value), nil
    case uint64:
        return float64(value), nil
    default:
        return 0, fmt.Errorf("unexpected formula result type for %s - %T",
                             resource.ServiceName, costCalcOut)
    }
}

//  Begin recording time resource is in usage.
//
func (resource *AwsCostResource) StartResourceTimer() {
    resource.StartTime = time.Now()
}
