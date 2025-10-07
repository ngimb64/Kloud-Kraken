package awscost

import (
	"fmt"
	"strings"
	"time"

	"github.com/expr-lang/expr"
)

// AwsCostResource represents a tracked resource and usage fields.
//
type AwsCostResource struct {
    formula     string
    Metadata    map[string]string
    price       float64  // Primary price (e.g., $/hour or $/GB-month)
    priceData   float64  // Optional second price (e.g., $/GB transfer)
    serviceName string
    startTime   time.Time
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
    if resource.startTime.IsZero() {
        resource.startTime = time.Now()
    }

    resource.timeUsed = time.Since(resource.startTime)

    // Get hours used and calculate out of a month
    hours := resource.timeUsed.Hours()
    months := hours / (24.0 * 30.0)

    gbMonths := resource.gbMonths
    // If there are no GB month, calculate them
    if gbMonths <= 0 {
        gbMonths = resource.Gb * months
    }

    env := map[string]any{
        "price":        resource.price,      // Default primary price
        "price_data":   resource.priceData,  // Secondary price (if any)
        "hours":        hours,               // Hours of uptime
        "months":       months,
        "gb_months":    gbMonths,
        "gb":           resource.Gb,
        "gb_transfer":  resource.GbTransfer,
        "data_out_gb":  resource.GbTransfer, // Alias used in some formulas
        "requests":     resource.Requests,
        "put_requests": resource.PutRequests,
        "get_requests": resource.GetRequests,
    }

    // Ensure all formula aliases are set
    env["storage_price"] = resource.price
    env["price_hour"] = resource.price
    env["price_gb"] = resource.price
    env["price_egress"] = resource.priceData
    env["price_data_unit"] = resource.priceData

    // Add parsed numeric metadata (if present). We add both raw string and parsed numeric
    // as underscored keys, e.g. meta_put_price -> 0.005
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

    // Compile the cost formula to be evaluated
    prog, err := expr.Compile(resource.formula, expr.AsFloat64())
    if err != nil {
        return 0, fmt.Errorf("failed to compile formula for %s - %v (formula: %q)",
                             resource.serviceName, err, resource.formula)
    }

    // Run the cost forumla to generate resource cost
    costCalcOut, err := expr.Run(prog, env)
    if err != nil {
        return 0, fmt.Errorf("failed to run formula for %s - %v (formula: %q)",
                             resource.serviceName, err, resource.formula)
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
                             resource.serviceName, costCalcOut)
    }
}

//  Begin recording time resource is in usage.
//
func (resource *AwsCostResource) StartResourceTimer() {
    resource.startTime = time.Now()
}
