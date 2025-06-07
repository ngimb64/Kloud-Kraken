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
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
)

// Attempts to load AWS access and secret keys from the default keychain.
//
// @Parameters
// - region:  The AWS region wherer the API credential are to be utilized
// - callTime:  The length of time the API call is allowed to execute
//
// @Returns
// - The AWS credentials config
// - The AWS API access key ID
// - The AWS API secret access key
// - Boolean indicating whether the credentials exist or not in default keychain
//
func AttemptLoadDefaultCredChain(region string, callTime time.Duration) (
                                 aws.Config, string, string, bool) {
    // Load the local credential chain (env, ~/.aws, etc.)
    config, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
    if err != nil {
        return config, "", "", false
    }

    // Retrieve credentials with a deadline
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Retreive the credentials from the credentials provider
    creds, err := config.Credentials.Retrieve(ctx)
    if err != nil {
        return config, "", "", false
    }

    return config, creds.AccessKeyID, creds.SecretAccessKey, true
}


// Set up the AWS config with credentials and region stored in passed in app config.
//
// @Paramters
// - region:  The AWS region wherer the API credential are to be utilized
// - callTime:  The length of time the API call is allowed to execute
//
// @Returns:
// - The initialized AWS credentials config
// - The AWS access key id
// - The AWS secret access key
// - Error if it occurs, otherwise nil on success
//
func AwsConfigSetup(region string, callTime time.Duration) (aws.Config, string, string, error) {
    // Attempt to load credentials from default credential chain
    awsConfig, accessKey, secretKey, exists := AttemptLoadDefaultCredChain(region, callTime)
    if exists {
        return awsConfig, accessKey, secretKey, nil
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
    awsConfig, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region),
                                               config.WithCredentialsProvider(awsCreds))
    if err != nil {
        return awsConfig, "", "", err
    }

    return awsConfig, accessKey, secretKey, nil
}


// Struct for managing EC2 operations
type Ec2Manger struct {
    ami              string
    client           *ec2.Client
    count            int
    instanceType     string
    name             string
    roleName         string
    runResult        *ec2.RunInstancesOutput
    securityGroupIds []string
    securityGroups   []string
    subnetId         string
    userData         []byte
}

// Establishes connection to EC2 service and generates EC2 manager struct
//
// @Parameters
// - ami:  The Amazon Machine Image that the EC2 instances will be using
// - awsConfig:  The AWS credential configuration for connecting to service
// - count:  The number of instances to be spawned
// - instanceType:  The type of instance to be used
// - name:  The name of the service to be tagged for easy reference
// - roleName:  The name of the IAM role to be utilized
// - securityGroupIds:  List of security group IDs to apply
// - securityGroups:  List of security group names to apply
// - subnetId:  The subnet ID to apply
// - userData:   The user data to be fed into each EC2 and executed
//
// @Returns
// - The initialized EC2 manager with populated data
//
func NewEc2Manager(ami string, awsConfig aws.Config, count int, instanceType string,
                   name string, roleName string, securityGroupIds []string,
                   securityGroups []string, subnetId string, userData []byte) *Ec2Manger {
    // Setup a new EC2 client
    ec2Client := ec2.NewFromConfig(awsConfig)

    return &Ec2Manger{
        ami:              ami,
        client:           ec2Client,
        count:            count,
        instanceType:     instanceType,
        name:             name,
        roleName:         roleName,
        securityGroupIds: securityGroupIds,
        securityGroups:   securityGroups,
        subnetId:         subnetId,
        userData:         userData,
    }
}

// Launches EC2 instances based on passed in count, pausing between each based on
// based in delay.
//
// @Parameters
// - callTime:  The length of time the API call is allowed to execute
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) CreateEc2Instances(callTime time.Duration) (error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Base64 encode the user data script
    encodedUserData := base64.StdEncoding.EncodeToString(Ec2Man.userData)

    // Prepare the RunInstances input
    input := &ec2.RunInstancesInput{
        ImageId:      aws.String(Ec2Man.ami),
        InstanceType: ec2types.InstanceType(Ec2Man.instanceType),
        MinCount:     aws.Int32(int32(Ec2Man.count)),
        MaxCount:     aws.Int32(int32(Ec2Man.count)),
        UserData:     aws.String(encodedUserData),
        IamInstanceProfile: &ec2types.IamInstanceProfileSpecification{
            Name: aws.String(Ec2Man.roleName),
        },
        // Tag instances on creation
        TagSpecifications: []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeInstance,
                Tags: []ec2types.Tag{
                    {Key: aws.String("Service"), Value: aws.String(Ec2Man.name)},
                },
            },
        },
    }

    // If there security groups IDs to apply
    if len(Ec2Man.securityGroupIds) > 0 {
        input.SecurityGroupIds = Ec2Man.securityGroupIds
    }

    // If there are security group names to apply
    if len(Ec2Man.securityGroups) > 0 {
        input.SecurityGroups = Ec2Man.securityGroups
    }

    // If there is specified subnet to apply
    if Ec2Man.subnetId != "" {
        input.SubnetId = &Ec2Man.subnetId
    }

    // Execute call to run the EC2 instance
    runOutput, err := Ec2Man.client.RunInstances(ctx, input)
    if err != nil {
        return err
    }

    // Assign run API call to EC2 manager struct
    Ec2Man.runResult = runOutput
    return nil
}

// Terminates the EC2 instances by ID's collected from creation method result.
//
// @Parameters
// - callTime:  The length of time the API call is allowed to execute
//
// @Returns
// - The output from the EC2 termination API call
// - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) TerminateEc2Instances(callTime time.Duration) (
                                               *ec2.TerminateInstancesOutput, error) {
    var ids []string

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Iterate through instances from result output
    for _, instance := range Ec2Man.runResult.Instances {
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
    termOutput, err := Ec2Man.client.TerminateInstances(ctx, terminateInput)
    if err != nil {
        return nil, err
    }

    return termOutput, nil
}


// Creates an IAM role with the passed in JSON policy data applied.
//
// @Parameters
// - iamClient:  The client to the IAM service
// - callTime:  The length of time the API call is allowed to execute
// - roleName:  The IAM Role to attach to
// - trustPolicyJson:  The JSON trust policy
// - permPolicyName:  An identifier name for permissions policy
// - permPolicyJSON:  The JSON permissions policy
// - createProfile:  Toggle to set whether instance profiles are created or not
//
// @Returns
// - The ARN of the existing or created role
// - Error if it occurs, otherwise nil on success
//
func IamRoleCreation(iamClient *iam.Client, callTime time.Duration, roleName string,
                     trustPolicyJson string, permPolicyName string,
                     permPolicyJson string, createProfile bool) (string, error) {
    var roleArn string
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Check if the IAM role exists
    getOut, err := iamClient.GetRole(ctx, &iam.GetRoleInput{
        RoleName: aws.String(roleName),
    })
    if err != nil {
        var notFound *iamtypes.NoSuchEntityException

        // If the IAM role does not exist
        if ok := errors.As(err, &notFound); ok {
            // Create the IAM role
            createOut, err := iamClient.CreateRole(ctx, &iam.CreateRoleInput{
                RoleName:                 aws.String(roleName),
                AssumeRolePolicyDocument: aws.String(trustPolicyJson),
            })
            if err != nil {
                return "", fmt.Errorf("CreateRole failed: %w", err)
            }

            // Set the role ARN from output
            roleArn = aws.ToString(createOut.Role.Arn)
        } else {
            return "", fmt.Errorf("GetRole failed: %w", err)
        }
    } else {
        // Role existed, grab its ARN
        roleArn = aws.ToString(getOut.Role.Arn)
    }

    // Attach or overwrite the inline permissions policy
    _, err = iamClient.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
        RoleName:       aws.String(roleName),
        PolicyName:     aws.String(permPolicyName),
        PolicyDocument: aws.String(permPolicyJson),
    })
    if err != nil {
        return "", fmt.Errorf("PutRolePolicy failed: %w", err)
    }

    if createProfile {
        // Create the instance profile
        _, err = iamClient.CreateInstanceProfile(ctx, &iam.CreateInstanceProfileInput{
            InstanceProfileName: aws.String(roleName),
        })
        if err != nil {
            var entityExists *iamtypes.EntityAlreadyExistsException

            // If the error is not that the instance profile already exists
            if !errors.As(err, &entityExists) {
                return "", fmt.Errorf("CreateInstanceProfile failed: %w", err)
            }
        }

        // Add role to the instance profile
        _, err = iamClient.AddRoleToInstanceProfile(ctx, &iam.AddRoleToInstanceProfileInput{
            InstanceProfileName: aws.String(roleName),
            RoleName:            aws.String(roleName),
        })
        if err != nil {
            return "", fmt.Errorf("AddRoleToInstanceProfile failed: %w", err)
        }
    }

    return roleArn, nil
}


// Struct for managing S3 bucket operations
type S3Manager struct {
    client     *s3.Client
}

// Establishes connection to EC2 service and generates EC2 manager struct
//
// @Parameters
// - awsConfig:  The AWS credential configuration for connecting to service
//
// @Returns
// - The initialized S3 manager with client reference
//
func NewS3Manager(config aws.Config) *S3Manager {
    // Set up a new S3 client
    s3Client := s3.NewFromConfig(config)

    return &S3Manager{
        client:     s3Client,
    }
}

// Checks to see if an S3 bucket already exists.
//
// @Parameters
// - bucketName:  The name of the S3 bucket to check existence
// - callTime:  The length of time the API call is allowed to execute
//
// @Returns
// - Boolean toggle whether the bucket exists or not
// - Error if it occurs, otherwise nil on success
//
func (S3Man *S3Manager) BucketExists(bucketName string, callTime time.Duration) (
                                     bool, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Check if the bucket exists and get information
    _, err := S3Man.client.HeadBucket(ctx, &s3.HeadBucketInput{
        Bucket: aws.String(bucketName),
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

// Create an S3 bucket.
//
// @Parameters
// - bucketName:  The name of the bucket to be created
// - callTime:  The length of time the API call is allowed to execute
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func (S3Man *S3Manager) CreateBucket(bucketName string, callTime time.Duration) error {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Create the bucket based on the bucket name in S3 manager
    _, err := S3Man.client.CreateBucket(ctx, &s3.CreateBucketInput{
        Bucket: aws.String(bucketName),
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

// Retrieve object from S3 bucket.
//
// @Parameters
// - bucketName:  The name of the bucket where the object will be retrieved
// - key:  The key in bucket used to identify the object to retrieve
// - callTime:  The length of time the API call is allowed to execute
//
// @Returns
// - The retrieved S3 object as a byte slice
// - Error if it occurs, otherwise nil on success
//
func (S3Man *S3Manager) GetS3Object(bucketName string, key string,
                                    callTime time.Duration) (
                                    []byte, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Retrieve the object from S3 storage
    resp, err := S3Man.client.GetObject(ctx, &s3.GetObjectInput{
        Bucket: aws.String(bucketName),
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

// Put an object into a S3 bucket.
//
// @Parameters
// - bucketName:  The name of the S3 bucket where the object will be stored
// - key:  The key in bucket used to identify where the object will be stored
// - data:  The data to be stored associated with the key of in the S3 bucket
// - callTime:  The length of time the API call is allowed to execute
//
// @Returns
// - The final key name that is used
// - Error if it occurs, otherwise nil on success
//
func (S3Man *S3Manager) PutS3Object(bucketName string, key string, data []byte,
                                    callTime time.Duration) (string, error) {
    var apiErr smithy.APIError

    // Keep attemping key with number added until unused is found
    for i := 1; ; i++ {
        // Add number to end of key name
        candidate := key + "-" + strconv.Itoa(i)
        // Ensure AWS API calls do not hang for longer specified timeout
        ctx, cancel := context.WithTimeout(context.Background(), callTime)

        // Put the object in S3 storage based on key
        _, err := S3Man.client.PutObject(ctx, &s3.PutObjectInput{
            Bucket:      aws.String(bucketName),
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


// Struct for managing S3 bucket operations
type SsmManager struct {
    client    *ssm.Client
}

// Establishes connection to SSM service and generates SSM manager struct
//
// @Parameters
// - awsConfig:  The AWS credential configuration for connecting to service
//
// @Returns
// - The initialized SSM manager with client reference
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
// - parameter:  name of the parameter to retrieve
// - callTime:  The length of time the API call is allowed to execute
//
// @Returns
// - The retrieved parameter from param store
// - Error if it occurs, otherwise nil on success
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
// - parameter:  name of the parameter to retrieve
// - data:  The data to store with associated parameter
// - callTime:  The length of time the API call is allowed to execute
//
// @Returns
// - The path where the parameter is stored in param store
// - Error if it occurs, otherwise nil on success
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
