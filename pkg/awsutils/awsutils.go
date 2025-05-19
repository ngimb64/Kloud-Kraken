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

// TODO: add doc
func AttemptLoadDefaultCredChain(region string, callTime time.Duration) (
                                 aws.Config, bool) {
    // Load the local credential chain (env, ~/.aws, etc.)
    config, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
    if err != nil {
        return config, false
    }

    // Retrieve credentials with a deadline
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Retreive the credentials from the credentials provider
    _, err = config.Credentials.Retrieve(ctx)
    if err != nil {
        return config, false
    }

    return config, true
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
func AwsConfigSetup(region string, callTime time.Duration) (aws.Config, error) {
    // Attempt to load credentials from default credential chain
    awsConfig, exists := AttemptLoadDefaultCredChain(region, callTime)
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
    // Setup a new EC2 client
    ec2Client := ec2.NewFromConfig(config)

    return &Ec2Manger{
        AMI:          ami,
        Client:       ec2Client,
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
func (Ec2Man *Ec2Manger) CreateEc2Instances(callTime time.Duration) (error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

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
    runOutput, err := Ec2Man.Client.RunInstances(ctx, input)
    if err != nil {
        return err
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

    // Terminate all the collected instance id's
    termOutput, err := Ec2Man.Client.TerminateInstances(ctx, terminateInput)
    if err != nil {
        return nil, err
    }

    return termOutput, nil
}


type S3Manager struct {
    BucketName string
    Client     *s3.Client
}

func NewS3Manager(config aws.Config, bucketName string) *S3Manager {
    // Set up a new S3 client
    s3Client := s3.NewFromConfig(config)

    return &S3Manager{
        BucketName: bucketName,
        Client:     s3Client,
    }
}

// TODO: add doc
func (S3Man *S3Manager) BucketExists(callTime time.Duration) (bool, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Check if the bucket exists and get information
    _, err := S3Man.Client.HeadBucket(ctx, &s3.HeadBucketInput{
        Bucket: aws.String(S3Man.BucketName),
    })
    // If there was no error, bucket exists and is accessible
    if err == nil {
        return true, nil
    }

    var apiErr smithy.APIError

    // If an API error occured
    if errors.As(err, &apiErr) {
        // Get the error code
        errCode := apiErr.ErrorCode()
        // If the error code signals the buck does not exist
        if errCode == "NotFound" || errCode == "NoSuchBucket" {
            return false, nil
        }
    }

    // Any other error (403 Forbidden, network, etc)
    return false, err
}

// TODO: add doc
func (S3Man *S3Manager) CreateBucket(callTime time.Duration) error {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Create the bucket based on the bucket name in S3 manager
    _, err := S3Man.Client.CreateBucket(ctx, &s3.CreateBucketInput{
        Bucket: aws.String(S3Man.BucketName),
    })
    // If the bucket was successfully created
    if err == nil {
        return nil
    }

    var apiErr smithy.APIError

    // If an API error occured
    if errors.As(err, &apiErr){
        // Get the error code
        errCode := apiErr.ErrorCode()
        // If the error code signals the bucket already exists
        if errCode == "BucketAlreadyExists" || errCode == "BucketAlreadyOwnedByYou" {
            return errors.New("S3 bucket already exists")
        }
    }

    // For any other errors
    return err
}

// TODO: add doc
func (S3Man *S3Manager) GetS3Object(key string, callTime time.Duration) (
                                    []byte, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Retrieve the object from S3 storage
    resp, err := S3Man.Client.GetObject(ctx, &s3.GetObjectInput{
        Bucket: aws.String(S3Man.BucketName),
        Key:    aws.String(key),
    })
    if err != nil {
        return nil, err
    }

    // Close response body on local exit
    defer resp.Body.Close()

    // Read all the data from the request
    rawData, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, err
    }

    return rawData, nil
}

// TODO:  add doc
func (S3Man *S3Manager) PutS3Object(keyName string, data []byte,
                                    callTime time.Duration) (string, error) {
    var apiErr smithy.APIError

    // Keep attemping key with number added until unused is found
    for i := 1; ; i++ {
        // Add number to end of parameter name
        candidate := keyName + "-" + strconv.Itoa(i)
        // Ensure AWS API calls do not hang for longer specified timeout
        ctx, cancel := context.WithTimeout(context.Background(), callTime)

        // Put the object in S3 storage based on key
        _, err := S3Man.Client.PutObject(ctx, &s3.PutObjectInput{
            Bucket:      aws.String(S3Man.BucketName),
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

        // If the error is an API error an its code signals object already exists
        if errors.As(err, &apiErr) && apiErr.ErrorCode() == "PreconditionFailed" {
            continue
        }

        // Otherwise an undesired error occured
        return "", err
    }
}


type SsmManager struct {
    Client    *ssm.Client
    Parameter string
}

func NewSsmManager(config aws.Config, parameter string) *SsmManager {
    // Set up a new SSM client
    ssmClient := ssm.NewFromConfig(config)

    return &SsmManager{
        Client:    ssmClient,
        Parameter: parameter,
    }
}

// Retrieve value from AWS SSM Parameter Store.
//
// @Parameters
// - name:  name of the parameter to retrieve
// - callTime:  The length of time the API call is allowed to execute
//
// @Returns
// - The retrieved parameter from param store
// - Error if it occurs, otherwise nil on success
//
func (SsmMan *SsmManager) GetSsmParameter(name string, callTime time.Duration) (
                                          string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Get parameter from AWS SSM Parameter Store
    out, err := SsmMan.Client.GetParameter(ctx, &ssm.GetParameterInput{
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
// - name:  name of the parameter to retrieve
// - data:  The data to store with associated parameter
// - callTime:  The length of time the API call is allowed to execute
//
// @Returns
// - The path where the parameter is stored in param store
// - Error if it occurs, otherwise nil on success
//
func (SsmMan *SsmManager) PutSsmParameter(paramName string, data string,
                                          callTime time.Duration) (
                                          string, error) {
    var existsErr *types.ParameterAlreadyExists

    // Keep attemping parameters with number added until unused is found
    for i := 1;; i++ {
        // Add number to end of parameter name
        candidate := paramName + "-" + strconv.Itoa(i)
        // Ensure AWS API calls do not hang for longer specified timeout
        ctx, cancel := context.WithTimeout(context.Background(), callTime)

        // Put parameter into AWS SSM Parameter Store
        _, err := SsmMan.Client.PutParameter(ctx, &ssm.PutParameterInput{
            Name:      aws.String(candidate),
            Value:     aws.String(data),
            Type:      types.ParameterTypeSecureString,
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
