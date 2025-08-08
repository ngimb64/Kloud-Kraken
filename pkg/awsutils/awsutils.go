package awsutils

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/rand"
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

// Package level varaibles
var AzIndex = 0


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
// - region:  The AWS region wherer the API credential are to be utilized
// - callTime:  The length of time the API call is allowed to execute
//
// @Returns:
// - The initialized AWS credentials config
// - The AWS access key id
// - The AWS secret access key
// - Error if it occurs, otherwise nil on success
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


// Struct for managing EC2 operations
type Ec2Manger struct {
    client      *ec2.Client
    runResult   *ec2.RunInstancesOutput
}

// Establishes connection to EC2 service and generates EC2 manager struct
//
// @Parameters
// - awsConfig:  The AWS credential configuration for connecting to service
//
// @Returns
// - The initialized EC2 manager with populated data
//
func NewEc2Manager(awsConfig aws.Config) *Ec2Manger {
    // Setup a new EC2 client
    ec2Client := ec2.NewFromConfig(awsConfig)

    return &Ec2Manger{
        client:     ec2Client,
    }
}

// Launches EC2 instances based on passed in config args.
//
// @Parameters
// - callTime:  The length of time the API call is allowed to execute
// - userData:   The user data to be fed into each EC2 and executed
// - ami:  The Amazon Machine Image that the EC2 instances will be using
// - instanceType:  The type of instance to be used
// - count:  The number of instances to be spawned
// - roleName:  The name of the IAM role to be utilized
// - name:  The name of the service to be tagged for easy reference
// - securityGroupIds:  List of security group IDs to apply
// - securityGroups:  List of security group names to apply
// - subnetId:  The subnet ID to apply
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) CreateEc2Instances(callTime time.Duration, userData []byte, ami string,
                                            instanceType string, count int, roleName string,
                                            name string, securityGroupIds []string,
                                            securityGroups []string, subnetId string) (error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Base64 encode the user data script
    encodedUserData := base64.StdEncoding.EncodeToString(userData)

    // Prepare the RunInstances input
    input := &ec2.RunInstancesInput{
        ImageId:      aws.String(ami),
        InstanceType: ec2types.InstanceType(instanceType),
        MinCount:     aws.Int32(int32(count)),
        MaxCount:     aws.Int32(int32(count)),
        UserData:     aws.String(encodedUserData),
        IamInstanceProfile: &ec2types.IamInstanceProfileSpecification{
            Name: aws.String(roleName),
        },
        // Tag instances on creation
        TagSpecifications: []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeInstance,
                Tags: []ec2types.Tag{
                    {Key: aws.String("Service"), Value: aws.String(name)},
                },
            },
        },
    }

    // If there security groups IDs to apply
    if len(securityGroupIds) > 0 {
        input.SecurityGroupIds = securityGroupIds
    }

    // If there are security group names to apply
    if len(securityGroups) > 0 {
        input.SecurityGroups = securityGroups
    }

    // If there is specified subnet to apply
    if subnetId != "" {
        input.SubnetId = &subnetId
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

//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) FetchAvailableAZs(callTime time.Duration) (
                                           []string, error) {
    // Set context timeout for API call
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.DescribeAvailabilityZonesInput{
        Filters: []ec2types.Filter{
            {
                Name:   aws.String("state"),
                Values: []string{"available"},
            },
        },
    }

    // Retrieve the list of avail
    output, err := Ec2Man.client.DescribeAvailabilityZones(ctx, callInput)
    if err != nil {
        return nil, fmt.Errorf("failed to describe AZs:  %w", err)
    }

    azs := []string{}

    // Iterate through AZs and add their names to the slice
    for _, az := range output.AvailabilityZones {
        azs = append(azs, *az.ZoneName)
    }

    // If no az names were parsed, something is wrong
    if len(azs) == 0 {
        return nil, fmt.Errorf("no available AZs found")
    }

    return azs, nil
}

//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) InternetGatewayCreateAndAttach(callTime time.Duration,
                                                        vpcId string,
                                                        nameTag string) (string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)

    createCallInput := &ec2.CreateInternetGatewayInput{
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeInternetGateway,
				Tags: []ec2types.Tag{
					{Key: aws.String("Name"), Value: aws.String(nameTag)},
				},
			},
		},
	}

    // Create the internet gateway
	createOut, err := Ec2Man.client.CreateInternetGateway(ctx, createCallInput)
    cancel()
	if err != nil {
		return "", fmt.Errorf("create internet gateway:  %w", err)
	}

    // If the create internet gateway call failed to return an ID
	if createOut.InternetGateway == nil || createOut.InternetGateway.InternetGatewayId == nil {
		return "", fmt.Errorf("create internet gateway returned empty id")
	}

	igwId := *createOut.InternetGateway.InternetGatewayId

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel = context.WithTimeout(context.Background(), callTime)
    defer cancel()

    attachCallInput := &ec2.AttachInternetGatewayInput{
		InternetGatewayId: aws.String(igwId),
		VpcId:             aws.String(vpcId),
    }

    // Attach the created internet gateway to the associated VPC
	_, err = Ec2Man.client.AttachInternetGateway(ctx, attachCallInput)
    if err != nil {
		return "", fmt.Errorf("attach internet gateway %s to vpc %s:  %w", igwId, vpcId, err)
	}

	return igwId, nil
}

//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) InternetGatewayExists(callTime time.Duration,
                                               vpcId string, igwId string) (
                                               bool, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.DescribeInternetGatewaysInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("attachment.vpc-id"),
				Values: []string{vpcId},
			},
		},
	}

    // Get informations on any internet gateways in the VPC
	out, err := Ec2Man.client.DescribeInternetGateways(ctx, callInput)
	if err != nil {
		return false, fmt.Errorf("describe internet gateways:  %w", err)
	}

    // Iterate through retrieved IGW IDs
	for _, igw := range out.InternetGateways {
        // If the current IGW ID is equal to arg passed in
		if igw.InternetGatewayId == &igwId {
			return true, nil
		}
	}

	return false, nil
}

//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) InternetGatewayProvisioner(callTime time.Duration, igwId string,
                                                    vpcId string) (string, error) {
    // If IGW ID is present in YAML
    if igwId != "" {
        // Check to see if it exists in AWS enviroment
        igwExists, err := Ec2Man.InternetGatewayExists(callTime, vpcId, igwId)
        if err != nil {
            return "", err
        }

        // If the IGW exists, exit early
        if igwExists {
            return "", nil
        }
    }

    // Create new internet gateway
    subnetId, err := Ec2Man.InternetGatewayCreateAndAttach(callTime, vpcId,
                                                           "Kloud-Kraken-IGW")
    if err != nil {
        return "", err
    }

    return subnetId, nil
}

//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) SubnetCreate(callTime time.Duration, vpcId, cidrBlock,
                                      az string, isPublic bool) (string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)

    // Create the subnet
    createOut, err := Ec2Man.client.CreateSubnet(ctx, &ec2.CreateSubnetInput{
        VpcId:            aws.String(vpcId),
        CidrBlock:        aws.String(cidrBlock),
        AvailabilityZone: aws.String(az),
    })
    cancel()
    if err != nil {
        return "", fmt.Errorf("unable to create subnet:  %w", err)
    }

    subnetID := aws.ToString(createOut.Subnet.SubnetId)

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel = context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Configure to map to public IP address on launch
    _, err = Ec2Man.client.ModifySubnetAttribute(ctx, &ec2.ModifySubnetAttributeInput{
        SubnetId: aws.String(subnetID),
        MapPublicIpOnLaunch: &ec2types.AttributeBooleanValue{
            Value: aws.Bool(isPublic),
        },
    })
    if err != nil {
        return "", fmt.Errorf("unable map subnet to public IP on launch:  %w", err)
    }

    return subnetID, nil
}

//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) SubnetExists(callTime time.Duration, vpcId string,
                                      cidrBlock string, subnetId string,
                                      az string) (bool, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Format the input for the subnet description call
    describeInput := &ec2.DescribeSubnetsInput{
        Filters: []ec2types.Filter{
            {Name: aws.String("vpc-id"), Values: []string{vpcId}},
            {Name: aws.String("cidr-block"), Values: []string{cidrBlock}},
            {Name: aws.String("availability-zone"), Values: []string{az}},
        },
    }

    // Get description of input subnet to see if it exists
    out, err := Ec2Man.client.DescribeSubnets(ctx, describeInput)
    if err != nil {
        return false, fmt.Errorf("DescribeSubnets failed: %w", err)
    }

    // If there was a result and it matches the intended subnet ID
    if len(out.Subnets) > 0 && out.Subnets[0].SubnetId == &subnetId {
        return true, nil
    }

    return false, nil
}

//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) SubnetProvision(callTime time.Duration, subnetId string,
                                         vpcID string, cidrBlock string,
                                         az string, isPublic bool) (
                                         string, error) {
    // If subnet ID is present in YAML
    if subnetId != "" {
        // Check to see if it exists in AWS enviroment
        subnetExists, err := Ec2Man.SubnetExists(callTime, vpcID, cidrBlock, subnetId, az)
        if err != nil {
            return "", err
        }

        // If the subnet exists, exit early
        if subnetExists {
            return "", nil
        }
    }

    // Create new subnet
    subnetId, err := Ec2Man.SubnetCreate(callTime, vpcID, cidrBlock, az, isPublic)
    if err != nil {
        return "", err
    }

    return subnetId, nil
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

// Creates and waits for the VPC to be created.
//
// @Parameters
// - callTime:  The length of time the API call is allowed to execute
// - cidrBlock:  The network CIDR block of IP address space to allocate in VPC
//
// @Returns
// - The ID of the created VPC
// - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) VpcCreate(callTime time.Duration,
                                   cidrBlock string) (
                                   string, error) {
    // Set context timeout for API call
    ctx, cancel := context.WithTimeout(context.Background(), callTime)

    // Format input for CreateVpc call
    createCallInput := &ec2.CreateVpcInput{
        CidrBlock: aws.String(cidrBlock),
        TagSpecifications: []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeVpc,
                Tags: []ec2types.Tag{
                {
                    Key: aws.String("Name"), Value: aws.String("Kloud-Kraken-VPC"),
                },
            },
        }},
    }

    // Create a new VPC since no valid ID was provided
    createOut, err := Ec2Man.client.CreateVpc(ctx, createCallInput)
    cancel()
    if err != nil {
        return "", err
    }

    vpcId := *createOut.Vpc.VpcId

    // Format input for NewVpcExistsWaiter call
    waiterCallInput := &ec2.DescribeVpcsInput{
        VpcIds: []string{vpcId},
    }

    // Set context timeout for API call
    ctx, cancel = context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Allocate waiter and wait until the VPC is available
    waiter := ec2.NewVpcExistsWaiter(Ec2Man.client)
    err = waiter.Wait(ctx, waiterCallInput, 5 * time.Minute)
    if err != nil {
        return vpcId, err
    }

    return vpcId, nil
}

// Checks to see if the VPC exists.
//
// @Parameters
// - callTime:  The length of time the API call is allowed to execute
// - vpcID:  The ID of the VPC to ensure exists
//
// @Returns
// - Boolean to notify whether bucket exists or not
// - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) VpcExists(callTime time.Duration, vpcId string) (bool, error) {
    // Set context timeout for API call
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Check to see if the VPC exists
    out, err := Ec2Man.client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
        VpcIds: []string{vpcId},
    })

    // If the ID was identified, exit early
    if err == nil && len(out.Vpcs) == 1 {
        return true, nil
    }

    var apiErr smithy.APIError
    // If the error is not API related
    // OR the API error suggests the VPC exists
    if !errors.As(err, &apiErr) ||
    apiErr.ErrorCode() != "InvalidVpcID.NotFound" {
        return true, err
    }

    // The VPC was not found
    return false, nil
}

// Returns VPC ID if it exists, or creates it using supplied CIDR.
//
// @Parameters
// - callTime:  The length of time the API call is allowed to execute
// - vpcID:  The ID of the VPC to ensure exists
// - cidrBlock:  The network CIDR block of IP address space to allocate in VPC
//
// @Returns
// - The ID of VPC if created, otherwise nil
// - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) VpcProvision(callTime time.Duration, vpcId string,
                                      cidrBlock string) (string, error) {
    // If VPC ID is present in YAML
    if vpcId != "" {
        // Check to see if it exists in AWS enviroment
        vpcExists, err := Ec2Man.VpcExists(callTime, vpcId)
        if err != nil {
            return "", err
        }

        // If the VPC already exists, exit early
        if vpcExists {
            return "", nil
        }
    }

    // Create and wait until VPC is created
    vpcId, err := Ec2Man.VpcCreate(callTime, cidrBlock)
    if err != nil {
        return vpcId, err
    }

    return vpcId, nil
}


// Struct for managing S3 bucket operations
type IamManager struct {
    client  *iam.Client
}

// Establishes connection to IAM service and generates IAM manager struct
//
// @Parameters
// - awsConfig:  The AWS credential configuration for connecting to service
//
// @Returns
// - The initialized IAM manager with populated data
//
func NewIamManager(awsConfig aws.Config) *IamManager {
    // Setup a new EC2 client
    iamClient := iam.NewFromConfig(awsConfig)

    return &IamManager{
        client:     iamClient,
    }
}


// Creates an instance profile with passed and attaches role to it.
//
// @Parameters
// - callTime:  The length of time the API call is allowed to execute
// - roleName:  The IAM Role used for operations
//
// @Returns
// - Error if it occurs, otherwise nil on success
//
func (IamMan *IamManager) createInstanceProfile(callTime time.Duration,
                                                roleName string) error {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    // Create the instance profile
    _, err := IamMan.client.CreateInstanceProfile(ctx, &iam.CreateInstanceProfileInput{
        InstanceProfileName: aws.String(roleName),
    })
    cancel()
    if err != nil {
        var entityExists *iamtypes.EntityAlreadyExistsException

        // If the error is not that the instance profile already exists
        if !errors.As(err, &entityExists) {
            return fmt.Errorf("instance profile already exists:  %w", err)
        }
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel = context.WithTimeout(context.Background(), callTime)

    // Add role to the instance profile
    _, err = IamMan.client.AddRoleToInstanceProfile(ctx, &iam.AddRoleToInstanceProfileInput{
        InstanceProfileName: aws.String(roleName),
        RoleName:            aws.String(roleName),
    })
    cancel()
    if err != nil {
        return fmt.Errorf("attaching role to instance:  %w", err)
    }

    return nil
}


// Creates an IAM role with the passed in JSON policy data applied.
//
// @Parameters
// - callTime:  The length of time the API call is allowed to execute
// - roleName:  The IAM Role used for operations
// - trustPolicyJson:  The JSON trust policy
// - permPolicyName:  An identifier name for permissions policy
// - permPolicyJSON:  The JSON permissions policy
// - createProfile:  Toggle whether an instance profile is created or not
//
// @Returns
// - The ARN of the existing or created role
// - Error if it occurs, otherwise nil on success
//
func (IamMan *IamManager) IamRoleCreation(callTime time.Duration, roleName string,
                                          trustPolicyJson string, permPolicyName string,
                                          permPolicyJson string, createProfile bool) (
                                          string, error) {
    var roleArn string
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)

    // Check if the IAM role exists
    getOut, err := IamMan.client.GetRole(ctx, &iam.GetRoleInput{
        RoleName: aws.String(roleName),
    })
    cancel()
    if err != nil {
        var notFound *iamtypes.NoSuchEntityException

        // If the IAM role does not exist
        if ok := errors.As(err, &notFound); ok {
            // Create the IAM role
            createOut, err := IamMan.client.CreateRole(ctx, &iam.CreateRoleInput{
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

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel = context.WithTimeout(context.Background(), callTime)

    // Attach or overwrite the inline permissions policy
    _, err = IamMan.client.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
        RoleName:       aws.String(roleName),
        PolicyName:     aws.String(permPolicyName),
        PolicyDocument: aws.String(permPolicyJson),
    })
    cancel()
    if err != nil {
        return "", fmt.Errorf("PutRolePolicy failed:  %w", err)
    }

    // If specified, create instance profile and attach role to it
    if createProfile {
        err = IamMan.createInstanceProfile(callTime, roleName)
        if err != nil {
            return "", fmt.Errorf("creating EC2 instace profile:  %w", err)
        }
    }

    return roleArn, nil
}


//
//
// @Parameters
//
//
// @Returns
//
//
func PickAzRoundRobin(azs []string) string {
    chosen := azs[AzIndex%len(azs)]
    // Increment package level variable
    AzIndex++
    return chosen
}


//
//
// @Parameters
//
//
// @Returns
//
//
func PickAzRandom(azs []string) string {
    // Seed the random number generator to ensure unique results
    rand.New(rand.NewSource(time.Now().UnixNano()))
    return azs[rand.Intn(len(azs))]
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
