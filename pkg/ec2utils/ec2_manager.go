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

// Struct for managing EC2 operations
type Ec2Manger struct {
    client      *ec2.Client
    runResult   *ec2.RunInstancesOutput
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
        client:     ec2Client,
    }
}

// Launches EC2 instances based on passed in config args.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - userData:   The user data to be fed into each EC2 and executed
//  - ami:  The Amazon Machine Image that the EC2 instances will be using
//  - instanceType:  The type of instance to be used
//  - count:  The number of instances to be spawned
//  - roleName:  The name of the IAM role to be utilized
//  - name:  The name of the service to be tagged for easy reference
//  - securityGroupIds:  List of security group IDs to apply
//  - securityGroups:  List of security group names to apply
//  - subnetId:  The subnet ID to apply
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) Ec2CreateInstances(callTime time.Duration,
                                            userData []byte,
                                            ami string,
                                            instanceType string,
                                            minCount int32,
                                            maxCount int32,
                                            roleName string,
                                            tags map[string]string,
                                            securityGroupIds []string,
                                            subnetId string) (
                                            error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Base64 encode the user data script
    encodedUserData := base64.StdEncoding.EncodeToString(userData)

    // Prepare the RunInstances input
    createInput := &ec2.RunInstancesInput{
        IamInstanceProfile: &ec2types.IamInstanceProfileSpecification{
            Name: aws.String(roleName),
        },
        ImageId:      aws.String(ami),
        InstanceType: ec2types.InstanceType(instanceType),
        MaxCount:     aws.Int32(maxCount),
        MinCount:     aws.Int32(minCount),
        UserData:     aws.String(encodedUserData),
    }

    if len(tags) > 0 {
        createInput.TagSpecifications = []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeInstance,
                Tags: awsutils.BuildEc2Tags(tags),
            },
        }
    }

    // If there security groups IDs to apply
    if len(securityGroupIds) > 0 {
        createInput.SecurityGroupIds = securityGroupIds
    }

    // If there is specified subnet to apply
    if subnetId != "" {
        createInput.SubnetId = aws.String(subnetId)
    }

    // Execute call to run the EC2 instance
    runOutput, err := Ec2Man.client.RunInstances(ctx, createInput)
    if err != nil {
        return err
    }

    var instanceIDs []string

    // Add the instance IDs from run output to list
    for _, inst := range runOutput.Instances {
        instanceIDs = append(instanceIDs, *inst.InstanceId)
    }

    waiterCallInput := &ec2.DescribeInstanceStatusInput{
        InstanceIds: instanceIDs,
    }

    // Allocate waiter and wait until EC2 instances are spawned
    waiter := ec2.NewInstanceStatusOkWaiter(Ec2Man.client)
    err = waiter.Wait(ctx, waiterCallInput, callTime)
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

    // Retrieve the list of available availibility zones
    output, err := Ec2Man.client.DescribeAvailabilityZones(ctx, callInput)
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
    for _, instance := range Ec2Man.runResult.Instances {
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
    termOutput, err := Ec2Man.client.TerminateInstances(ctx, terminateInput)
    if err != nil {
        return nil, err
    }

    return termOutput, nil
}
