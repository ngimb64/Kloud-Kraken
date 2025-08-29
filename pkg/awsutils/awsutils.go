package awsutils

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// Package level varaibles
var AzIndex = 0


// Attempts to load AWS access and secret keys from the default keychain.
//
// @Parameters
//  - region:  The AWS region wherer the API credential are to be utilized
//  - callTime:  The length of time the API call is allowed to execute
//
// @Returns
//  - The AWS credentials config
//  - The AWS API access key ID
//  - The AWS API secret access key
//  - Boolean indicating whether the credentials exist or not in default keychain
//
func AttemptLoadDefaultCredChain(region string, callTime time.Duration) (
                                 aws.Config, string, string, bool) {
    // Load the local credential chain (env, ~/.aws, etc.)
    cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
    if err != nil {
        return cfg, "", "", false
    }

    // Retrieve credentials with a deadline
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Retreive the credentials from the credentials provider
    creds, err := cfg.Credentials.Retrieve(ctx)
    if err != nil {
        return cfg, "", "", false
    }

    return cfg, creds.AccessKeyID, creds.SecretAccessKey, true
}


// Set up the AWS config with credentials and region stored in passed in app config.
//
// @Paramters
//  - region:  The AWS region wherer the API credential are to be utilized
//  - callTime:  The length of time the API call is allowed to execute
//
// @Returns:
//  - The initialized AWS credentials config
//  - The AWS access key id
//  - The AWS secret access key
//  - Error if it occurs, otherwise nil on success
//
func AwsConfigSetup(region string, callTime time.Duration) (
                    aws.Config, string, string, error) {
    // Attempt to load credentials from default credential chain
    cfg, accessKey, secretKey, exists := AttemptLoadDefaultCredChain(region, callTime)
    if exists {
        return cfg, accessKey, secretKey, nil
    }

    // Get AWS access and secret key environment variables
    accessKey = os.Getenv("AWS_ACCESS_KEY")
    secretKey = os.Getenv("AWS_SECRET_KEY")
    // If AWS access and secret key are present
    if accessKey == "" || secretKey == "" {
        return aws.Config{}, "", "", fmt.Errorf("missing either the access (%s) or " +
                                                "secret key (%s) for AWS",
                                                accessKey, secretKey)
    }

    // Set the AWS credentials provider
    awsCreds := credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")

    // Load default config and override with custom credentials and region
    cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region),
                                         config.WithCredentialsProvider(awsCreds))
    if err != nil {
        return cfg, "", "", err
    }

    return cfg, accessKey, secretKey, nil
}


// Gets the account ID from STS client.
//
// @Parameters
// - ctx:  Context that manages timeout for API calls
// - stsClient:  Initialized client to the Security Token Service
//
// @Returns
// - The retrieved account ID
//  - Error if it occurs, otherwise nil on success
//
func GetAccountID(callTime time.Duration, stsClient sts.Client) (string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

	out, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", err
	}

	return *out.Account, nil
}


// Select next availability zone in a round robin fashion.
//
// @Parameters
// - azs:  Slice of availability zones to select from
//
// @Returns
// - The chosen availability zone
//
func PickAzRoundRobin(azs []string) string {
    chosen := azs[AzIndex%len(azs)]
    // Increment package level variable
    AzIndex++
    return chosen
}


// Select next availability zone in a random fashion.
//
// @Parameters
// - azs:  Slice of availability zones to select from
//
// @Returns
// - The chosen availability zone
//
func PickAzRandom(azs []string) string {
    // Seed the random number generator to ensure unique results
    rand.New(rand.NewSource(time.Now().UnixNano()))
    return azs[rand.Intn(len(azs))]
}
