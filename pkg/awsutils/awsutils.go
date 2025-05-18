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


// Struct for managing EC2 instance creation
type Ec2Args struct {
    AMI       string
    Config    aws.Config
    Count     int
    Delay     time.Duration
    Name      string
    UserData  string
}

// Launches EC2 instances based on passed in count, pausing between each based on
// based in delay.
//
// TODO: finish doc
//
func (Ec2Args *Ec2Args) CreateEc2Instances(callTime time.Duration,
                                           ec2Client *ec2.Client) ([]ec2types.Instance, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // If no EC2 client exists, make one
    if ec2Client == nil {
        ec2Client = ec2.NewFromConfig(Ec2Args.Config)
    }

    // Base64 encode the user data script
    encodedUserData := base64.StdEncoding.EncodeToString([]byte(Ec2Args.UserData))

    // Prepare the RunInstances input
    input := &ec2.RunInstancesInput{
        ImageId:      aws.String(Ec2Args.AMI),
        InstanceType: ec2types.InstanceTypeT2Micro,
        MinCount:     aws.Int32(int32(Ec2Args.Count)),
        MaxCount:     aws.Int32(int32(Ec2Args.Count)),
        UserData:     aws.String(encodedUserData),
        // Tag instances on creation
        TagSpecifications: []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeInstance,
                Tags: []ec2types.Tag{
                    {Key: aws.String("Service"), Value: aws.String(Ec2Args.Name)},
                },
            },
        },
    }

    // Execute call to run the EC2 instance
    output, err := ec2Client.RunInstances(ctx, input)
    if err != nil {
        return nil, fmt.Errorf("RunInstances failed: %w", err)
    }

    return output.Instances, nil
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


// TODO:  add doc
func NewEc2Args(ami string, config aws.Config, count int,
                delay time.Duration, name string,
                userData string) *Ec2Args {
    return &Ec2Args{
        AMI:       ami,
        Config:    config,
        Count:     count,
        Delay:     delay,
        Name:      name,
        UserData:  userData,
    }
}


// TODO:  add doc
func PutUniqueObjectBytes(cfg aws.Config, bucket, keyName string, data []byte,
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
        var apiErr *smithy.APIError


        // TODO:  research if this works or there is a better way to handle S3 errors


        // If the candiate was successful
        if err == nil {
            return candidate, nil
        // If the object already exists on S3 bucket
        } else if errors.As(err, apiErr) {
            if apiErr.ErrorCode() == "PreconditionFailed" {
                continue
            }
        }

        // Otherwise an undesired error occured
        return "", fmt.Errorf("put %q: %w", candidate, err)
    }
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


// func TerminateEc2Instances() {
//     var ids []string

//     for _, inst := range output.Instances {
//         if inst.InstanceId != nil {
//             ids = append(ids, *inst.InstanceId)
//         }
//     }

//     // build termination input
//     terminateInput := &ec2.TerminateInstancesInput{
//         InstanceIds: ids,
//     }

//     // call the API
//     terminateOutput, err := ec2Client.TerminateInstances(ctx, terminateInput)
//     if err != nil {
//         return fmt.Errorf("failed to terminate instances: %w", err)
//     }
// }
