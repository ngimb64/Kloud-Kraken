package awscost

// BytesToGB converts bytes to decimal gigabytes (1 GB = 1e9 bytes)
// Use when billing uses decimal GB (common in cloud billing).
// Use with cloud pricing calculation.
//
func BytesToGB(b int64) float64 {
    return float64(b) / 1e9
}


// BytesToGiB converts bytes to gibibytes (1 GiB = 1024^3 bytes)
// Use when you need binary GiB conversion.
// Use with local OS calculations
//
func BytesToGiB(b int64) float64 {
    return float64(b) / (1024.0 * 1024.0 * 1024.0)
}
