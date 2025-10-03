package awscost

import (
	"fmt"
	"time"

	"github.com/expr-lang/expr"
)

// AwsCostResource represents a tracked resource and usage fields.
//
type AwsCostResource struct {
    formula     string
    price       float64  // primary price (e.g., $/hour or $/GB-month)
    priceData   float64  // optional second price (e.g., $/GB transfer)
    serviceName string
    startTime   time.Time
    timeUsed    time.Duration

    // Usage fields
    Gb         float64  // GB stored
    gbMonths   float64  // precomputed GB-months
    GbTransfer float64  // GB transferred
    Requests   float64  // number of requests
}

// CalculateResourceTotal computes the resource cost using expr formulas.
//
// @Returns
//	- Returns resource total cost unless it fails, which defaults to 0
//  - Error if it occurs, otherwise nil on success
//
func (resource *AwsCostResource) CalculateResourceTotal() (float64, error) {
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
        "price":       resource.price,
        "price_data":  resource.priceData,
        "hours":       hours,
        "gb_months":   gbMonths,
        "gb_transfer": resource.GbTransfer,
        "requests":    resource.Requests,
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

    // Ensure the cost calc output is float data type
    resourceCost, ok := costCalcOut.(float64)
    if ok {
        return resourceCost, nil
    }

    return 0, fmt.Errorf("unexpected formula result type for %s - %T",
                         resource.serviceName, costCalcOut)
}
