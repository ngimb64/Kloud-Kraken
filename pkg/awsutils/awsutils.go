package awsutils

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// Set up the AWS config with credentials and region stored in passed in app config.
//
// @Paramters
// - region:  The AWS region wherer the API credential are to be utilized
//
// @Returns:
// - The initialized AWS credentials config
// - Error if it occurs, otherwise nil on success
//
func AwsConfigSetup(region string) (aws.Config, error) {
    // Get the AWS access and secret key environment variables
    awsAccessKey := os.Getenv("AWS_ACCESS_KEY")
    awsSecretKey := os.Getenv("AWS_SECRET_KEY")
    // If AWS access and secret key are present
    if awsAccessKey == "" || awsSecretKey == "" {
        return aws.Config{}, fmt.Errorf("missing either the access (%s) or " +
                                        "secret key (%s) for AWS",
                                           awsAccessKey, awsSecretKey)
    }

    // Set the AWS credentials provider
    awsCreds := credentials.NewStaticCredentialsProvider(awsAccessKey, awsSecretKey, "")

    // Load default config and override with custom credentials and region
    awsConfig, err := config.LoadDefaultConfig(
        context.TODO(),
        config.WithRegion(region),
        config.WithCredentialsProvider(awsCreds),
    )

    if err != nil {
        return awsConfig, err
    }

    return awsConfig, nil
}


// Retrieve value from AWS SSM Parameter Store.
//
// @Parameters
// - awsConfig:  The AWS configuration instance with credentials
// - name:  name of the parameter to retrieve
//
// @Returns
// - The retrieved parameter from Parameter Store
// - Error if it occurs, otherwise nil on success
//
func GetBytesParameter(awsConfig aws.Config, name string) (string, error) {
    // Ensure AWS API calls do not hang for longer than 30 seconds
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // Establish an SSM parameter store client
    ssmClient := ssm.NewFromConfig(awsConfig)

    // Get parameter from AWS SSM Parameter Store
    out, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
        Name:           aws.String(name),
        WithDecryption: aws.Bool(true),
    })

    if err != nil {
        return "", err
    }

    return aws.ToString(out.Parameter.Value), nil
}


// Put value into AWS SSM Parameter Store.
//
// @Parameters
// - awsConfig:  The AWS configuration instance with credentials
// - name:  name of the parameter to retrieve
// - data:  The data to store with associated parameter
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func PutBytesParameter(awsConfig aws.Config, name string, data string) error {
    // Ensure AWS API calls do not hang for longer than 30 seconds
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // Establish an SSM parameter store client
    ssmClient := ssm.NewFromConfig(awsConfig)

    // Put parameter into AWS SSM Parameter Store
    _, err := ssmClient.PutParameter(ctx, &ssm.PutParameterInput{
        Name:      aws.String(name),
        Value:     aws.String(data),
        Type:      types.ParameterTypeSecureString,
        Overwrite: aws.Bool(true),
    })

    if err != nil {
        return err
    }

    return nil
}
