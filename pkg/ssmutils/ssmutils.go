package ssmutils

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// Struct for managing S3 bucket operations
type SsmManager struct {
    client    *ssm.Client
}

// Establishes connection to SSM service and generates SSM manager struct
//
// @Parameters
//  - awsConfig:  The AWS credential configuration for connecting to service
//
// @Returns
//  - The initialized SSM manager with client reference
//
func NewSsmManager(config aws.Config) *SsmManager {
    // Set up a new SSM client
    ssmClient := ssm.NewFromConfig(config)

    return &SsmManager{
        client:    ssmClient,
    }
}

// Retrieve value from AWS SSM Parameter Store.
//
// @Parameters
//  - parameter:  name of the parameter to retrieve
//  - callTime:  The length of time the API call is allowed to execute
//
// @Returns
//  - The retrieved parameter from param store
//  - Error if it occurs, otherwise nil on success
//
func (SsmMan *SsmManager) GetSsmParameter(parameter string, callTime time.Duration) (
                                          string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Get parameter from AWS SSM Parameter Store
    output, err := SsmMan.client.GetParameter(ctx, &ssm.GetParameterInput{
        Name:           aws.String(parameter),
        WithDecryption: aws.Bool(true),
    })
    if err != nil {
        return "", err
    }

    return aws.ToString(output.Parameter.Value), nil
}

// Put value into AWS SSM Parameter Store.
//
// @Parameters
//  - parameter:  name of the parameter to retrieve
//  - data:  The data to store with associated parameter
//  - callTime:  The length of time the API call is allowed to execute
//
// @Returns
//  - The path where the parameter is stored in param store
//  - Error if it occurs, otherwise nil on success
//
func (SsmMan *SsmManager) PutSsmParameter(parameter string, data string,
                                          callTime time.Duration) (
                                          string, error) {
    var existsErr *ssmtypes.ParameterAlreadyExists

    // Keep attemping parameters with number added until unused is found
    for i := 1;; i++ {
        // Add number to end of parameter name
        candidate := parameter + "-" + strconv.Itoa(i)
        // Ensure AWS API calls do not hang for longer specified timeout
        ctx, cancel := context.WithTimeout(context.Background(), callTime)

        // Put parameter into AWS SSM Parameter Store
        _, err := SsmMan.client.PutParameter(ctx, &ssm.PutParameterInput{
            Name:      aws.String(candidate),
            Value:     aws.String(data),
            Type:      ssmtypes.ParameterTypeSecureString,
            Overwrite: aws.Bool(false),
        })
        // Cancel context per API call
        cancel()

        if err != nil {
            // If the parameter already exists in SSM Parameter Store
            if errors.As(err, &existsErr) {
                continue
            }

            // For all other errors
            return "", err
        }

        return candidate, nil
    }
}
