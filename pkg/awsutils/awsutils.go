package awsutils

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
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


// Struct for managing EC2 instance creation
type CreateEc2Args struct {
    AMI       string
    Config    aws.Config
    Count     int
    Delay     time.Duration
    Name      string
    PublicIps []string
    SsmParam  string
    UserData  string
}

// Launches EC2 instances based on passed in count, pausing between each based on
// based in delay.
//
// TODO: finish doc
//
func (Ec2Args *CreateEc2Args) CreateEc2Instances() ([]ec2types.Instance, error) {
    var allInstances []ec2types.Instance
    // Ensure AWS API calls do not hang for longer than 15 seconds
    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    // Establish an EC2 client
    ec2Client := ec2.NewFromConfig(Ec2Args.Config)

    // Iterate over number of EC2 instances to be created
    for i := range Ec2Args.Count {
        // Base64 encode the user data script
        encodedUserData := base64.StdEncoding.EncodeToString([]byte(Ec2Args.UserData))

        // Prepare the RunInstances input
        input := &ec2.RunInstancesInput{
            ImageId:      aws.String(Ec2Args.AMI),
            InstanceType: ec2types.InstanceTypeT2Micro,
            MinCount:     aws.Int32(1),
            MaxCount:     aws.Int32(1),
            UserData:     aws.String(encodedUserData),
            // Tag instances on creation
            TagSpecifications: []ec2types.TagSpecification{
                {
                    ResourceType: ec2types.ResourceTypeInstance,
                    Tags: []ec2types.Tag{
                        {Key: aws.String("Service"), Value: aws.String(Ec2Args.Name)},
                        {Key: aws.String("Index"), Value: aws.String(strconv.Itoa(i))},
                    },
                },
            },
        }

        // Execute call to run the EC2 instance
        out, err := ec2Client.RunInstances(ctx, input)
        if err != nil {
            return nil, fmt.Errorf("RunInstances failed on iteration %d: %w", i, err)
        }

        // Add the launched instance to instances slice
        allInstances = append(allInstances, out.Instances...)

        // If not the last iteration, sleep before the next launch
        if i < Ec2Args.Count-1 && Ec2Args.Delay > 0 {
            time.Sleep(Ec2Args.Delay)
        }
    }

    return allInstances, nil
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
    // Ensure AWS API calls do not hang for longer than 15 seconds
    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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


// TODO:  add docs
func NewCreateEc2Args(ami string, config aws.Config, count int, delay time.Duration,
                      name string, publicIps []string,
                      ssmParam string, userData string) *CreateEc2Args {
    return &CreateEc2Args{
        AMI:       ami,
        Config:    config,
        Count:     count,
        Delay:     delay,
        Name:      name,
        PublicIps: publicIps,
        SsmParam:  ssmParam,
        UserData:  userData,
    }
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
func PutBytesParameter(awsConfig aws.Config, name string,
                       data string) (string, error) {
    // Ensure AWS API calls do not hang for longer than 15 seconds
    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    // Establish an SSM parameter store client
    ssmClient := ssm.NewFromConfig(awsConfig)

    // Keep attemping parameters with number added until unused is found
    for i := 0;; i++ {
        // Add number to end of parameter name
        candidate := name + "-" + strconv.Itoa(i)

        // Put parameter into AWS SSM Parameter Store
        _, err := ssmClient.PutParameter(ctx, &ssm.PutParameterInput{
            Name:      aws.String(name),
            Value:     aws.String(data),
            Type:      types.ParameterTypeSecureString,
            Overwrite: aws.Bool(false),
        })

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
