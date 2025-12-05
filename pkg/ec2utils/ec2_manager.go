package ec2utils

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
)

// Struct for storing Ec2CreateInstances call input
type Ec2CreateInstancesInput struct {
    AMI              string
    EbsSize          int32              // Optional
    HasWaiter        bool               // Optional
    InstanceType     string             // Optional
    MaxCount         int32
    MinCount         int32
    RoleName         string             // Optional
    SecurityGroupIds []string           // Optional
    SubnetId         string             // Optional
    Tags             map[string]string  // Optional
    UserData         []byte             // Optional
}

// Struct for managing EC2 operations
type Ec2Manger struct {
    Client      *ec2.Client
    RunResult   *ec2.RunInstancesOutput
}

// Establishes connection to EC2 service and generates EC2 manager struct
//
// @Parameters
//  - awsConfig:  The AWS credential configuration for connecting to service
//
// @Returns
//  - The initialized EC2 manager with populated data
//
func Ec2NewManager(awsConfig aws.Config) *Ec2Manger {
    // Setup a new EC2 client
    ec2Client := ec2.NewFromConfig(awsConfig)

    return &Ec2Manger{
        Client:     ec2Client,
    }
}

// Launches EC2 instances based on passed in config args.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - callInput:  Input struct Ec2CreateInstancesInput that stores inputs
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) Ec2CreateInstances(callTime time.Duration,
                                            callInput *Ec2CreateInstancesInput) (
                                            error) {
    // Ensure required call inputs are present in struct
    if callInput.AMI == "" {
        return errors.New("AMI is missing from ec2CreateInstancesInput struct")
    }

    // Ensure the min and max counts are greater than one and
    // the max is greater than or equal to the min count
    if callInput.MaxCount < 1 || callInput.MinCount < 1 ||
    callInput.MinCount > callInput.MaxCount {
        return errors.New("max & min counts should be greater than 0 and " +
                          "the max should be greater than or equal to the min")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    createInput := &ec2.RunInstancesInput{
        ImageId:      aws.String(callInput.AMI),
        MaxCount:     aws.Int32(callInput.MaxCount),
        MinCount:     aws.Int32(callInput.MinCount),
    }

    // If non-default EBS volume size is set, set the block device mapping
    if callInput.EbsSize > 0 {
        createInput.BlockDeviceMappings = []ec2types.BlockDeviceMapping{
            {
                DeviceName: aws.String("/dev/xvda"),
                Ebs: &ec2types.EbsBlockDevice{
                    VolumeSize:          aws.Int32(callInput.EbsSize),
                    VolumeType:          ec2types.VolumeTypeGp3,
                    DeleteOnTermination: aws.Bool(true),
                },
            },
        }
    }

    // If there is an instance type to apply
    if callInput.InstanceType != "" {
        createInput.InstanceType = ec2types.InstanceType(callInput.InstanceType)
    }

    // If there is a role name to apply to IAM instance profile
    if callInput.RoleName != "" {
        createInput.IamInstanceProfile = &ec2types.IamInstanceProfileSpecification{
            Name: aws.String(callInput.RoleName),
        }
    }

    // If there security groups IDs to apply
    if len(callInput.SecurityGroupIds) > 0 {
        createInput.SecurityGroupIds = callInput.SecurityGroupIds
    }

    // If there is specified subnet to apply
    if callInput.SubnetId != "" {
        createInput.SubnetId = aws.String(callInput.SubnetId)
    }

    // If there is tags to be applied
    if len(callInput.Tags) > 0 {
        createInput.TagSpecifications = []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeInstance,
                Tags: awsutils.BuildEc2Tags(callInput.Tags),
            },
        }
    }

    // If there is user data to be encoded and applied
    if callInput.UserData != nil {
        // Base64 encode the user data script
        encodedUserData := base64.StdEncoding.EncodeToString(callInput.UserData)
        createInput.UserData = &encodedUserData
    }

    // Execute call to run the EC2 instance
    runOutput, err := Ec2Man.Client.RunInstances(ctx, createInput)
    if err != nil {
        return err
    }

    // If there is a status OK waiter
    if callInput.HasWaiter {
        var instanceIDs []string

        // Add the instance IDs from run output to list
        for _, inst := range runOutput.Instances {
            instanceIDs = append(instanceIDs, *inst.InstanceId)
        }

        waiterCallInput := &ec2.DescribeInstanceStatusInput{
            InstanceIds: instanceIDs,
        }

        // Allocate waiter and wait until EC2 instances are spawned
        waiter := ec2.NewInstanceStatusOkWaiter(Ec2Man.Client)
        err = waiter.Wait(ctx, waiterCallInput, callTime)
        if err != nil {
            return err
        }
    }

    // Assign run API call to EC2 manager struct
    Ec2Man.RunResult = runOutput
    return nil
}

// Retrieves a list of availability zones for region used for AWS config.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//
// @Returns
//  - List of retrieved availability zones for region
//  - Error if it occurs, otherwise nil on success
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

    // Retrieve the list of available availibility zones
    output, err := Ec2Man.Client.DescribeAvailabilityZones(ctx, callInput)
    if err != nil {
        return nil, fmt.Errorf("failed to describe AZs - %w", err)
    }

    azs := []string{}

    // Iterate through AZs and add their names to the slice
    for _, az := range output.AvailabilityZones {
        azs = append(azs, *az.ZoneName)
    }

    // If no az names were parsed, something is wrong
    if len(azs) == 0 {
        return nil, errors.New("no available AZs found")
    }

    return azs, nil
}

// Terminates the EC2 instances by ID's collected from creation method result.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//
// @Returns
//  - The output from the EC2 termination API call
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) Ec2TerminateInstances(callTime time.Duration) (
                                               *ec2.TerminateInstancesOutput,
                                               error) {
    var ids []string
    var termOutput *ec2.TerminateInstancesOutput

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

    // If no instances were found, return early
    if len(ids) == 0 {
        return termOutput, nil
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
