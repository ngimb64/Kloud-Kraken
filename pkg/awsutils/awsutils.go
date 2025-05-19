package awsutils

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
)

// TODO:  add doc
func AttemptLoadDefaultCredChain(region string, callTime time.Duration) (
                                 aws.Config, bool) {
    // Load the local credential chain (env, ~/.aws, etc.)
    cfg, err := config.LoadDefaultConfig(context.TODO(),
        config.WithRegion(region),
    )
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
    // Attempt to load credentials from default credential chain
    awsConfig, exists := AttemptLoadDefaultCredChain(region, 30*time.Second)
    if exists {
        return awsConfig, nil
    }

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
    awsConfig, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region),
                                               config.WithCredentialsProvider(awsCreds))
    if err != nil {
        return awsConfig, err
    }

    return awsConfig, nil
}


// Struct for managing EC2 instance creation
type Ec2Manger struct {
    AMI          string
    Client       *ec2.Client
    Config       aws.Config
    Count        int
    InstanceType string
    Name         string
    RunResult    *ec2.RunInstancesOutput
    UserData     []byte
}

// TODO:  add doc
func NewEc2Manager(ami string, config aws.Config, count int, instanceType string,
                   name string, userData []byte) *Ec2Manger {
    return &Ec2Manger{
        AMI:          ami,
        Config:       config,
        Count:        count,
        InstanceType: instanceType,
        Name:         name,
        UserData:     userData,
    }
}

// Launches EC2 instances based on passed in count, pausing between each based on
// based in delay.
//
// TODO: finish doc
//
func (Ec2Man *Ec2Manger) CreateEc2Instances(callTime time.Duration,
                                            ec2Client *ec2.Client) (error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // If no EC2 client exists, make one
    if ec2Client == nil {
        ec2Client = ec2.NewFromConfig(Ec2Man.Config)
    }

    // Save reference to client for other methods
    Ec2Man.Client = ec2Client
    // Base64 encode the user data script
    encodedUserData := base64.StdEncoding.EncodeToString(Ec2Man.UserData)

    // Prepare the RunInstances input
    input := &ec2.RunInstancesInput{
        ImageId:      aws.String(Ec2Man.AMI),
        InstanceType: ec2types.InstanceType(Ec2Man.InstanceType),
        MinCount:     aws.Int32(int32(Ec2Man.Count)),
        MaxCount:     aws.Int32(int32(Ec2Man.Count)),
        UserData:     aws.String(encodedUserData),
        // Tag instances on creation
        TagSpecifications: []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeInstance,
                Tags: []ec2types.Tag{
                    {Key: aws.String("Service"), Value: aws.String(Ec2Man.Name)},
                },
            },
        },
    }

    // Execute call to run the EC2 instance
    runOutput, err := ec2Client.RunInstances(ctx, input)
    if err != nil {
        return fmt.Errorf("RunInstances failed: %w", err)
    }

    // Assign run API call to EC2 manager struct
    Ec2Man.RunResult = runOutput
    return nil
}

// TODO:  add doc
func (Ec2Man *Ec2Manger) TerminateEc2Instances(callTime time.Duration) (
                                               *ec2.TerminateInstancesOutput, error) {
    var ids []string

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Iterate through instances from result output
    for _, instance := range Ec2Man.RunResult.Instances {
        // If the instance ID is present add to ids slice
        if instance.InstanceId != nil {
            ids = append(ids, *instance.InstanceId)
        }
    }

    // build termination input with parsed id's
    terminateInput := &ec2.TerminateInstancesInput{
        InstanceIds: ids,
    }

    // call the API
    termOutput, err := Ec2Man.Client.TerminateInstances(ctx, terminateInput)
    if err != nil {
        return nil, fmt.Errorf("failed to terminate instances: %w", err)
    }

    return termOutput, nil
}


type S3Manager struct {
    BucketName string
}

func NewS3Manager(bucketName string) *S3Manager {
    return &S3Manager{
        BucketName: bucketName,
    }
}

// TODO: add doc
func BucketExists(cfg aws.Config, bucketName string, timeout time.Duration) (bool, error) {
    client := s3.NewFromConfig(cfg)
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
        Bucket: aws.String(bucketName),
    })
    if err == nil {
        // 200 OK → bucket exists and is accessible
        return true, nil
    }

    // Unwrap the SDK error to see if it's a “not found” case
    var apiErr smithy.APIError
    if errors.As(err, &apiErr) {
        code := apiErr.ErrorCode()
        switch code {
        case "NotFound", "NoSuchBucket":
            // bucket genuinely does not exist
            return false, nil
        }
    }

    // any other error (403 Forbidden, network, etc)
    return false, fmt.Errorf("checking bucket %q existence: %w", bucketName, err)
}

// TODO: add doc
func CreateBucketIfNotExists(cfg aws.Config, bucketName string, timeout time.Duration) error {
    client := s3.NewFromConfig(cfg)
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
        Bucket: aws.String(bucketName),
    })
    if err == nil {
        // created successfully
        return nil
    }

    // If it's an API error, inspect the ErrorCode
    var apiErr smithy.APIError
    if errors.As(err, &apiErr) {
        switch apiErr.ErrorCode() {
        case "BucketAlreadyExists", "BucketAlreadyOwnedByYou":
            // name is taken (by you or someone else) → treat as success
            return nil
        }
    }

    // any other error is real
    return fmt.Errorf("creating bucket %q: %w", bucketName, err)
}

// TODO: add doc
func GetS3Object(cfg aws.Config, bucket, key string, callTime time.Duration,
                 s3Client *s3.Client) ([]byte, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // If no S3 client exists, make one
    if s3Client == nil {
        s3Client = s3.NewFromConfig(cfg)
    }

    // Retrieve the object from S3 storage
    resp, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
        Bucket: aws.String(bucket),
        Key:    aws.String(key),
    })
    if err != nil {
        return nil, fmt.Errorf("s3 GetObject %q/%q: %w", bucket, key, err)
    }

    // Close response body on local exit
    defer resp.Body.Close()

    // Read all the data from the request
    raw, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("reading body for %q/%q: %w", bucket, key, err)
    }

    return raw, nil
}

// TODO:  add doc
func PutS3Object(cfg aws.Config, bucket string, keyName string, data []byte,
                 callTime time.Duration, s3Client *s3.Client) (string, error) {
    // If no S3 client exists, make one
    if s3Client == nil {
        s3Client = s3.NewFromConfig(cfg)
    }

    // Keep attemping key with number added until unused is found
    for i := 1; ; i++ {
        // Add number to end of parameter name
        candidate := keyName + "-" + strconv.Itoa(i)
        // Ensure AWS API calls do not hang for longer specified timeout
        ctx, cancel := context.WithTimeout(context.Background(), callTime)

        // Put the object in S3 storage based on key
        _, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
            Bucket:      aws.String(bucket),
            Key:         aws.String(candidate),
            Body:        bytes.NewReader(data),
            IfNoneMatch: aws.String("*"),
        })
        // Cancel context per API call
        cancel()

        // If the candiate was successful
        if err == nil {
            return candidate, nil
        }

        // If the object already exists on S3 bucket
        if apiErr, ok := err.(smithy.APIError); ok {
            errCode := apiErr.ErrorCode()
            // If the error code signals the bucket already exists
            if errCode == "BucketAlreadyExists" || errCode == "BucketAlreadyOwnedByYou" {
                continue
            }
        }

        // Otherwise an undesired error occured
        return "", fmt.Errorf("put %q: %w", candidate, err)
    }
}


type SsmManager struct {
    Parameter string
}

func NewSsmManager(parameter string) *SsmManager {
    return &SsmManager{
        Parameter: parameter,
    }
}

// Retrieve value from AWS SSM Parameter Store.
//
// @Parameters
// - awsConfig:  The AWS configuration instance with credentials
// - name:  name of the parameter to retrieve
// - callTime:  The length of time the API call is allowed to execute
// - ssmClient:  Established param store session, if nil one is created
//
// @Returns
// - The retrieved parameter from param store
// - Error if it occurs, otherwise nil on success
//
func GetSsmParameter(awsConfig aws.Config, name string, callTime time.Duration,
                     ssmClient *ssm.Client) (string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // If no SSM parameter store client exists, make one
    if ssmClient == nil {
        ssmClient = ssm.NewFromConfig(awsConfig)
    }

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
// - callTime:  The length of time the API call is allowed to execute
// - ssmClient:  Established param store session, if nil one is created
//
// @Returns
// - The path where the parameter is stored in param store
// - Error if it occurs, otherwise nil on success
//
func PutSsmParameter(awsConfig aws.Config, paramName string,
                     data string, callTime time.Duration,
                     ssmClient *ssm.Client) (string, error) {
    // If no SSM parameter store client exists, make one
    if ssmClient == nil {
        ssmClient = ssm.NewFromConfig(awsConfig)
    }

    // Keep attemping parameters with number added until unused is found
    for i := 1;; i++ {
        // Add number to end of parameter name
        candidate := paramName + "-" + strconv.Itoa(i)
        // Ensure AWS API calls do not hang for longer specified timeout
        ctx, cancel := context.WithTimeout(context.Background(), callTime)

        // Put parameter into AWS SSM Parameter Store
        _, err := ssmClient.PutParameter(ctx, &ssm.PutParameterInput{
            Name:      aws.String(candidate),
            Value:     aws.String(data),
            Type:      types.ParameterTypeSecureString,
            Overwrite: aws.Bool(false),
        })
        // Cancel context per API call
        cancel()

        if err != nil {
            var existsErr *types.ParameterAlreadyExists

            // If the parameter already exists in SSM Parameter Store
            if errors.As(err, &existsErr) {
                continue
            }

            return "", fmt.Errorf("put operation failed with %q: %w", candidate, err)
        }

        return candidate, nil
    }
}
