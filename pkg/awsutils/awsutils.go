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
	cwl "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// Package level varaibles
var AzIndex = 0


// Attempts to load AWS access and secret keys from the default keychain.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - region:  The AWS region wherer the API credential are to be utilized
//
// @Returns
//  - The AWS credentials config
//  - Toggle for whether the credentials exist or not in default keychain
//
func AttemptLoadDefaultCredChain(callTime time.Duration, region string) (
                                 aws.Config, bool) {
    // Load the local credential chain (env, ~/.aws, etc.)
    cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
    if err != nil {
        return cfg, false
    }

    // Retrieve credentials with a deadline
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Retreive the credentials from the credentials provider
    _, err = cfg.Credentials.Retrieve(ctx)
    if err != nil {
        return cfg, false
    }

    return cfg, true
}


// Set up AWS config with credentials and region stored in app config.
//
// @Paramters
//  - callTime:  The length of time the API call is allowed to execute
//  - region:  The AWS region wherer the API credential are to be utilized
//
// @Returns:
//  - The initialized AWS credentials config
//  - Error if it occurs, otherwise nil on success
//
func AwsConfigSetup(callTime time.Duration, region string) (
                    aws.Config, error) {
    // Attempt to load credentials from default credential chain
    cfg, exists := AttemptLoadDefaultCredChain(callTime, region)
    if exists {
        return cfg, nil
    }

    // Get AWS access and secret key environment variables
    accessKey := os.Getenv("AWS_ACCESS_KEY")
    secretKey := os.Getenv("AWS_SECRET_KEY")
    // If AWS access and secret key are present
    if accessKey == "" || secretKey == "" {
        return aws.Config{}, fmt.Errorf("missing either the access (%s) or " +
                                        "secret key (%s) for AWS",
                                        accessKey, secretKey)
    }

    // Set the AWS credentials provider
    awsCreds := credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")

    // Load default config and override with custom credentials and region
    cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region),
                                         config.WithCredentialsProvider(awsCreds))
    if err != nil {
        return cfg, err
    }

    return cfg, nil
}


// Build AWS resource tags based on the passed in map.
//
// @Parameters
//  - tagMap:  Map used to store the tag key-value entries
//
// @Returns
//  - The populated EC2 tag slice
//
func BuildEc2Tags(tagMap map[string]string) []ec2types.Tag {
    tags := make([]ec2types.Tag, 0, len(tagMap))

    // Iterate through the tags map
    for key, value := range tagMap {
        // Add the key-value in map to tags slice
        tags = append(tags, ec2types.Tag{
            Key:   aws.String(key),
            Value: aws.String(value),
        })
    }

    return tags
}


// Build AWS resource tags based on the passed in map.
//
// @Parameters
//  - tagMap:  Map used to store the tag key-value entries
//
// @Returns
//  - The populated IAM tag slice
//
func BuildIamTags(tagMap map[string]string) []iamtypes.Tag {
	tags := make([]iamtypes.Tag, 0, len(tagMap))

    // Iterate through the tags map
	for k, v := range tagMap {
        // Add the key-value in map to tags slice
		tags = append(tags, iamtypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	return tags
}


// Build AWS resource tags based on the passed in map.
//
// @Parameters
//  - tagMap:  Map used to store the tag key-value entries
//
// @Returns
//  - The populated S3 tag slice
//
func BuildS3Tags(tagMap map[string]string) []s3types.Tag {
    tags := make([]s3types.Tag, 0, len(tagMap))

    // Iterate through the tags map
    for key, value := range tagMap {
        // Add the key-value in map to tags slice
        tags = append(tags, s3types.Tag{
            Key:   aws.String(key),
            Value: aws.String(value),
        })
    }

    return tags
}


// Build AWS resource tags based on the passed in map.
//
// @Parameters
//  - tagMap:  Map used to store the tag key-value entries
//
// @Returns
//  - The populcated SSM tag slice
//
func BuildSsmTags(tagMap map[string]string) []ssmtypes.Tag {
    tags := make([]ssmtypes.Tag, 0, len(tagMap))

    // Iterate through the tags map
    for key, value := range tagMap {
        // Add the key-value in map to tags slice
        tags = append(tags, ssmtypes.Tag{
            Key:   aws.String(key),
            Value: aws.String(value),
        })
    }

    return tags
}


// Gets the account ID from STS client.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - stsClient:  Initialized client to the Security Token Service
//
// @Returns
//  - The retrieved account ID
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
//  - azs:  Slice of availability zones to select from
//
// @Returns
//  - The chosen availability zone
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
//  - azs:  Slice of availability zones to select from
//
// @Returns
//  - The chosen availability zone
//
func PickAzRandom(azs []string) string {
    // Seed the random number generator to ensure unique results
    rand.New(rand.NewSource(time.Now().UnixNano()))
    return azs[rand.Intn(len(azs))]
}


// Sets retention in days for the provided log group.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - client:  The CloudWatch logs client
//  - logGroupName:  The CloudWatch log group name
//  - retentionDays:  Number of days to retain the logs in CloudWatch
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func SetRetentionForLogGroup(callTime time.Duration, client *cwl.Client,
                             logGroupName string, retentionDays int32) error {
    if retentionDays < 1 {
        return fmt.Errorf("retentionDays must be 1 or greater")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &cwl.PutRetentionPolicyInput{
        LogGroupName:    aws.String(logGroupName),
        RetentionInDays: aws.Int32(retentionDays),
    }

    // Put the retention policy for passed in log group
    _, err := client.PutRetentionPolicy(ctx, callInput)
    if err != nil {
        return fmt.Errorf("failed to set retention for %q - %w",
                          logGroupName, err)
    }

    return nil
}
