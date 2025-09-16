package ssmutils

import (
	"context"
	"errors"
	"fmt"
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
func SsmNewManager(config aws.Config) *SsmManager {
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
func (SsmMan *SsmManager) SsmGetParameter(callTime time.Duration,
                                          parameter string) (
                                          string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ssm.GetParameterInput{
        Name:           aws.String(parameter),
        WithDecryption: aws.Bool(true),
    }

    // Get parameter from AWS SSM Parameter Store
    output, err := SsmMan.client.GetParameter(ctx, callInput)
    if err != nil {
        return "", err
    }

    return aws.ToString(output.Parameter.Value), nil
}

// Put value into AWS SSM Parameter Store.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - parameter:  name of the parameter to retrieve
//  - data:  The data to store with associated parameter
//  - tagName:  Name to tag the AWS resource
//
// @Returns
//  - The path where the parameter is stored in param store
//  - Error if it occurs, otherwise nil on success
//
func (SsmMan *SsmManager) SsmPutParameter(callTime time.Duration,
                                          parameter string,
                                          data string,
                                          tagName string) (
                                          string, error) {
    var existsErr *ssmtypes.ParameterAlreadyExists

    // Keep attemping parameters with number added until unused is found
    for i := 1;; i++ {
        // Add number to end of parameter name
        candidate := parameter + "-" + strconv.Itoa(i)
        // Ensure AWS API calls do not hang for longer specified timeout
        ctx, cancel := context.WithTimeout(context.Background(), callTime)

        callInput := &ssm.PutParameterInput{
            Name:      aws.String(candidate),
            Value:     aws.String(data),
            Type:      ssmtypes.ParameterTypeSecureString,
            Overwrite: aws.Bool(false),
        }

        // If tag was specified, add it to input
        if tagName != "" {
            callInput.Tags = []ssmtypes.Tag{
                {
                    Key: aws.String("Name"),
                    Value: aws.String(tagName),
                },
            }
        }

        // Put parameter into AWS SSM Parameter Store
        _, err := SsmMan.client.PutParameter(ctx, callInput)
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


//
//
// @Parameters
//
//
// @Returns
//
//
func (SsmMan *SsmManager) SsmDeleteParameter(callTime time.Duration,
                                             paramName string) error {
    // Ensure required arg is present
    if paramName == "" {
        return fmt.Errorf("paramName is missing")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    deleteCallInput := &ssm.DeleteParameterInput{
        Name: aws.String(paramName),
    }

    // Delete the passed in parameter from SSM Parameter Store
    _, err := SsmMan.client.DeleteParameter(ctx, deleteCallInput)
    if err != nil {
        return err
    }

    return nil
}


//
//
// @Parameters
//
//
// @Returns
//
//
func (SsmMan SsmManager) SsmDeleteAllParams(callTime time.Duration,
                                            baseName string) error {
    for i := 1;; i++ {
        // Format the parameter name per iteration
        paramName := baseName + "-" + strconv.Itoa(i)
        // Attempt to delete the parameter
        err := SsmMan.SsmDeleteParameter(callTime, paramName)
        if err != nil {
            var paramNotFound *ssmtypes.ParameterNotFound

            // If the parameter was not found, operation is complete
            if errors.As(err, &paramNotFound) {
                break
            }

            return fmt.Errorf("failed to delete param %q - %w", paramName, err)
        }
    }

    return nil
}
