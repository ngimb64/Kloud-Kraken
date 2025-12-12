package ec2utils

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
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
        createInput.UserData = aws.String(encodedUserData)
    }

    // Execute call to run the EC2 instance
    runOutput, err := Ec2Man.Client.RunInstances(ctx, createInput)
    if err != nil {
        return err
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
func (Ec2Man *Ec2Manger) Ec2FetchAvailableAZs(callTime time.Duration) (
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

// Handles retrieving the AMI ID by first attempting via SSM Parameter
// Store then resorts to using DescribeImages call as a backup.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - arch:  The system architecture supported by the AMI
//  - amiNamePattern:  The text pattern of AMI to search for
//  - owners:  The owner IDs of the AMI
//
// @Returns
//  - The retrieved AMI ID if successfull
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) Ec2GetAmiId(callTime time.Duration, arch string,
                                     amiNamePattern string, owners []string) (
                                     string, error) {
    // Ensure required args are present
    if arch  == "" || amiNamePattern == "" {
        return "", errors.New("arch or namePattern is missing")
    }

    var err error
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    describeImagesInput := &ec2.DescribeImagesInput{
        Filters: []ec2types.Filter{
            {Name: aws.String("architecture"), Values: []string{arch}},
            {Name: aws.String("name"), Values: []string{amiNamePattern}},
            {Name: aws.String("state"), Values: []string{"available"}},
        },
    }

    // If owners are specified add them to call input
    if len(owners) > 0 {
        describeImagesInput.Owners = owners
    }

    // Get the AMI images by specified filters
    out, err := Ec2Man.Client.DescribeImages(ctx, describeImagesInput)
    if err != nil {
        return "", err
    }

    if len(out.Images) == 0 {
        return "", fmt.Errorf("no images matched pattern %q", amiNamePattern)
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

// Gets the public IP address(es) of passed in instance IDs. If no instance IDs
// are passed in it will attempt to parse them out of last RunInstances result.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - ec2Ids:  Optional slice of EC2 instance IDs, if nil RunInstances result instead
//
// @Returns
//  - Slice of public IPs retrieved based off instance IDs
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) Ec2GetPublicIps(callTime time.Duration, ec2Ids []string) (
                                         []string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    var instanceIds []string

    // If no instance IDs were passed in, use run instances result
    if len(ec2Ids) < 1 {
        // Iterate through launched instances & save IDs to slice
        for _, instance := range Ec2Man.RunResult.Instances {
            if instance.InstanceId != nil {
                instanceIds = append(instanceIds, *instance.InstanceId)
            }
        }
    // Otherwise use passed in instance IDs
    } else {
        instanceIds = ec2Ids
    }

    // If no instance IDs exist, exit early
    if len(instanceIds) < 1 {
        return nil, errors.New("no instance IDs to retrieve public IPs")
    }

    var describeErr error
    publicIps := make(map[string]struct{})

    // Retry until public IPs are ready
    for {
        // respect context cancellation/timeouts
        select {
        case <-ctx.Done():
            if describeErr != nil {
                return nil, fmt.Errorf("timed out waiting for public IPs" +
                                       " (last describe error: %w): %v",
                                       describeErr, ctx.Err())
            }

            return nil, fmt.Errorf("timed out waiting for public IPs: %w", ctx.Err())
        default:
        }

        describeInput := &ec2.DescribeInstancesInput{
            InstanceIds: instanceIds,
        }

        // Get the instance information where the public IP is stored
        descOut, err := Ec2Man.Client.DescribeInstances(ctx, describeInput)
        if err != nil {
            describeErr = errors.Join(describeErr,
                                      fmt.Errorf("DescribeInstances - %w", err))
            continue
        }

        allHaveIPs := true

        // Iterate through instance reservations
        for _, reservation := range descOut.Reservations {
            // Iterate through specific instance informatiion
            for _, instance := range reservation.Instances {
                // If public IP is missing, set toggle to continue loop
                if instance.PublicIpAddress == nil {
                    allHaveIPs = false
                    continue
                }

                // Check to see if public IP exists in map
                _, exists := publicIps[*instance.PublicIpAddress]
                // If the public IP does not exist in map
                if !exists {
                    // Add it to the public IPs map
                    publicIps[*instance.PublicIpAddress] = struct{}{}
                }
            }
        }

        if allHaveIPs {
            break
        }

        time.Sleep(5 * time.Second)
    }

    index := 0
    resultIps := make([]string, len(publicIps))

    // Iterate through keys of public IP map and assign in slice
    for key := range publicIps {
        resultIps[index] = key
        index++
    }

    return resultIps, nil
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

// Waits and polls the instance status unit it is OK.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) Ec2WaiterStatusOk(callTime time.Duration) error {
    var instanceIDs []string

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Add the instance IDs from run output to list
    for _, instance := range Ec2Man.RunResult.Instances {
        instanceIDs = append(instanceIDs, *instance.InstanceId)
    }

    waiterCallInput := &ec2.DescribeInstanceStatusInput{
        InstanceIds: instanceIDs,
    }

    // Allocate waiter and wait until EC2 instances are spawned
    waiter := ec2.NewInstanceStatusOkWaiter(Ec2Man.Client)
    err := waiter.Wait(ctx, waiterCallInput, callTime)
    if err != nil {
        return err
    }

    return nil
}
