package awsutils

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	cwl "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// Package level varaibles
var AzIndex = 0
// Maps AWS region codes to the human-friendly
// "location" strings used by the Pricing API
var RegionCodeToLocation = map[string]string{
    // US
    "us-east-1":      "US East (N. Virginia)",
    "us-east-2":      "US East (Ohio)",
    "us-west-1":      "US West (N. California)",
    "us-west-2":      "US West (Oregon)",

    // Canada
    "ca-central-1":   "Canada (Central)",

    // South America
    "sa-east-1":      "South America (Sao Paulo)",

    // Europe
    "eu-central-1":   "EU (Frankfurt)",
    "eu-west-1":      "EU (Ireland)",
    "eu-west-2":      "EU (London)",
    "eu-west-3":      "EU (Paris)",
    "eu-north-1":     "EU (Stockholm)",
    "eu-south-1":     "EU (Milan)",

    // Middle East / Africa
    "me-south-1":     "Middle East (Bahrain)",
    "af-south-1":     "Africa (Cape Town)",

    // Asia Pacific
    "ap-northeast-1": "Asia Pacific (Tokyo)",
    "ap-northeast-2": "Asia Pacific (Seoul)",
    "ap-northeast-3": "Asia Pacific (Osaka-Local)",
    "ap-southeast-1": "Asia Pacific (Singapore)",
    "ap-southeast-2": "Asia Pacific (Sydney)",
    "ap-south-1":     "Asia Pacific (Mumbai)",

    // China
    "cn-north-1":     "China (Beijing)",
    "cn-northwest-1": "China (Ningxia)",

    // GovCloud
    "us-gov-west-1":  "AWS GovCloud (US-West)",
    "us-gov-east-1":  "AWS GovCloud (US-East)",
}


// Attempts to load AWS access and secret keys from the default keychain.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - region:  The AWS region wherer the API credential are to be utilized
//  - profileName:  The name of the AWS config profile to load from credentials file
//
// @Returns
//  - The AWS credentials config
//  - Toggle for whether the credentials exist or not in default keychain
//
func AttemptLoadDefaultCredChain(callTime time.Duration, region string,
                                 profileName string) (aws.Config, bool) {
    // Load the local credential chain (env, ~/.aws, etc.)
    cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region),
                                         config.WithSharedConfigProfile(profileName))
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
//  - profileName:  The name of the AWS config profile to load from credentials file
//
// @Returns:
//  - The initialized AWS credentials config
//  - Error if it occurs, otherwise nil on success
//
func AwsConfigSetup(callTime time.Duration, region string, profileName string) (
                    aws.Config, error) {
    if profileName == "" {
        profileName = "default"
    }

    // Attempt to load credentials from default credential chain
    cfg, exists := AttemptLoadDefaultCredChain(callTime, region, profileName)
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
    for key, value := range tagMap {
        // Add the key-value in map to tags slice
        tags = append(tags, iamtypes.Tag{
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


// Gets the AMI ID's by specified filters and gets the most recently created.
//
// @Parameters
//  - ctx:  Context handler for AWS API call duration
//  - ec2Client:  Established client to EC2 service
//  - arch:  The system architecture supported by the AMI
//  - namePattern:  The name pattern to filter in DescribeImages
//  - owners:  The owner IDs of the AMI
//
// @Returns
//  - The retrieved AMI ID if successful
//  - Error if it occurs, otherwise nil on success
//
func findImageByName(ctx context.Context, ec2Client *ec2.Client,
                     arch string, namePattern string, owners []string) (
                     string, error) {
    if arch  == "" || namePattern == "" {
        return "", errors.New("arch or namePattern is missing")
    }

    describeImagesInput := &ec2.DescribeImagesInput{
        Filters: []ec2types.Filter{
            {Name: aws.String("architecture"), Values: []string{arch}},
            {Name: aws.String("name"), Values: []string{namePattern}},
            {Name: aws.String("state"), Values: []string{"available"}},
        },
    }

    // If owners are specified add them to call input
    if len(owners) > 0 {
        describeImagesInput.Owners = owners
    }

    // Get the AMI images by specified filters
    out, err := ec2Client.DescribeImages(ctx, describeImagesInput)
    if err != nil {
        return "", err
    }

    if len(out.Images) == 0 {
        return "", fmt.Errorf("no images matched pattern %q", namePattern)
    }

    // Sort list of AMIs by descending order by creation date
    sort.Slice(out.Images, func(i int, j int) bool {
        ai := out.Images[i].CreationDate
        aj := out.Images[j].CreationDate

        // If both nil -> consider equal (stable)
        if ai == nil && aj == nil {
            return false
        }

        // Push nils to the end (so non-nil come first)
        if ai == nil {
            return false
        }

        if aj == nil {
            return true
        }

        // CreationDate uses ISO-8601 so lexicographic comparison is valid
        return *ai > *aj
    })

    // search for first non-nil ImageId
    for _, img := range out.Images {
        if img.ImageId != nil {
            return aws.ToString(img.ImageId), nil
        }
    }

    return "", errors.New("no image with valid ImageId found")
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


// Handles retrieving the AMI ID by first attempting via SSM Parameter
// Store then resorts to using DescribeImages call as a backup.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - ec2Client:  Established client to EC2 service
//  - arch:  The system architecture supported by the AMI
//  - amiNamePattern:  The text pattern of AMI to search for
//  - owners:  The owner IDs of the AMI
//
// @Returns
//  - The retrieved AMI ID if successfull
//  - Error if it occurs, otherwise nil on success
//
func GetAmiId(callTime time.Duration, ec2Client *ec2.Client,
              arch string, amiNamePattern string,
              owners []string) (string, error) {
    var err error
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Backup method using DescribeImages if SSM method fails
    amiID, ferr := findImageByName(ctx, ec2Client, arch, amiNamePattern, owners)
    if ferr != nil {
        err = errors.Join(err, fmt.Errorf("fallback DescribeImages failed - %w", ferr))
        return "", err
    }

    return amiID, nil
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


// Retrieves the human readable location mapped to region
// for AWS pricing API.
//
// @Parameters
//  - region:  The region to fetch its human readable location
//
// @Returns
//  - The retrieved human readable location of region
//  - Toggle for whether region exists in map or not
//
func RegionToLocation(region string) (string, bool) {
    if region == "" {
        return "", false
    }

    // Ensure the region is lowercase format
    lowerRegion := strings.ToLower(strings.TrimSpace(region))
    // Retrieve human readable location from region key
    loc, ok := RegionCodeToLocation[lowerRegion]
    return loc, ok
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


// Validates whether the passed in region is in region map.
//
// @Parameters
//  - region:  The region name to be validated
//
// @Returns
//  - Toggle for whether the region exists in map or not
//
func ValidateRegion(region string) bool {
    _, ok := RegionToLocation(region)
    return ok
}
