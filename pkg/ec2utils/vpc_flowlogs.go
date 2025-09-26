package ec2utils

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
)

// Creates VPC Flow Logs to CloudWatch group.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - vpcId:  The ID of the VPC where the Flow Logs will be applied
//  - logGroupName:  Name of the CloudWatch Logs group VPC Flow Logs uses
//  - roleArn:  The IAM ARN of the VPC Flow Logs role
//  - tags:  String map of tag key-values to configure
//
// @Returns
//  - The VPC Flow Logs ID of created resource
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) createFlowLogToCloudWatch(callTime time.Duration,
                                                   vpcID string,
                                                   logGroupName string,
                                                   roleArn string,
                                                   tags map[string]string) (
                                                   string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.CreateFlowLogsInput{
        ResourceType:             ec2types.FlowLogsResourceTypeVpc,
        ResourceIds:              []string{vpcID},
        TrafficType:              ec2types.TrafficTypeAll,
        LogDestinationType:       ec2types.LogDestinationTypeCloudWatchLogs,
        LogGroupName:             aws.String(logGroupName),
        DeliverLogsPermissionArn: aws.String(roleArn),
    }

    if len(tags) > 0 {
        flowLogsTags := tags

        // If the name tag exists in tags map
        if name, ok := tags["Name"]; ok {
            flowLogsTags["Name"] = name + "-flow-logs"
        }

        callInput.TagSpecifications = []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeVpcFlowLog,
                Tags: awsutils.BuildEc2Tags(flowLogsTags),
            },
        }
    }

    // Create flow logs for passed in VPC ID in log group name
    out, err := Ec2Man.client.CreateFlowLogs(ctx, callInput)
    if err != nil {
        return "", err
    }

    if len(out.FlowLogIds) > 0 {
        return out.FlowLogIds[0], nil
    }

    return "", fmt.Errorf("CreateFlowLogs succeeded but returned no flow-log-id")
}

// Checks if VPC Flow Logs group exists via resource-id filter.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - vpcId:  ID of the VPC where Flow Logs are applied
//
// @Returns
//  - Toggle for whether VPC Flow Logs exists in VPC or not
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) VpcFlowLogExists(callTime time.Duration,
                                          vpcID string) (
                                          bool, error) {
    // Ensure required arg is present
    if vpcID == "" {
        return false, errors.New("vpcId is missing")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &ec2.DescribeFlowLogsInput{
        Filter: []ec2types.Filter{
            {
                Name:   aws.String("resource-id"),
                Values: []string{vpcID},
            },
        },
    }

    // Get the flow logs based off VPC ID
    out, err := Ec2Man.client.DescribeFlowLogs(ctx, callInput)
    if err != nil {
        return false, err
    }

    return len(out.FlowLogs) > 0, nil
}

// Provision VPC Flow Logs by checking for existence and creating if missing.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - flowLogId:  The VPC Flow Logs ID
//  - vpcId:  The ID of the VPC where the Flow Logs will be applied
//  - cwlClient:  The client to the CloudWatch service
//  - logGroupName:  Name of the CloudWatch Logs group VPC Flow Logs uses
//  - roleArn:  The IAM ARN of the VPC Flow Logs role
//  - retentionDays:  The number of days VPC Flow Logs are retained in CloudWatch
//  - tags:  String map of tag key-values to configure
//
// @Returns
//  - VPC Flow Logs ID if the resource is created, "" if it already exists
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man *Ec2Manger) VpcFlowLogProvision(callTime time.Duration,
                                             flowLogId string, vpcId string,
                                             cwlClient *cloudwatchlogs.Client,
                                             logGroupName string, roleArn string,
                                             retentionDays int32,
                                             tags map[string]string) (
                                             string, error) {
    // Ensure required args are present
    if vpcId == "" || cwlClient == nil || logGroupName == "" || roleArn == "" {
        return "", errors.New("vpcId or cwlClient or logGroupName or roleArn is missing")
    }

    // If the flow log Id is present in state file
    if flowLogId != "" {
        // Check see if flow logs are already set up for VPC
        exists, err := Ec2Man.VpcFlowLogExists(callTime, vpcId)
        if err != nil {
            return "", fmt.Errorf("checking VPC flow logs existence - %w", err)
        }

        // If the VPC flow logs already exist
        if exists {
            return "", nil
        }
    }

    callInput := &cloudwatchlogs.CreateLogGroupInput{
        LogGroupName: aws.String(logGroupName),
    }

    if len(tags) > 0 {
        logGroupTags := tags

        // If the name tag exists in tags map
        if name, ok := tags["Name"]; ok {
            logGroupTags["Name"] = name + "-log-group"
        }

        callInput.Tags = logGroupTags
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)

    // Create the Cloudwatch log group
    _, err := cwlClient.CreateLogGroup(ctx, callInput)
    cancel()
    if err != nil {
        var alreadyExists *cwtypes.ResourceAlreadyExistsException
        // If the error is not related to log group already existing
        if !errors.As(err, &alreadyExists) {
            return "", err
        }
    }

    // Set the VPC Flow Logs group retention period
    err = awsutils.SetRetentionForLogGroup(1 * time.Minute, cwlClient,
                                           logGroupName, retentionDays)
    if err != nil {
        return "", err
    }

    // Create the flow logs from VPC to CloudWatch
    flowLogId, err = Ec2Man.createFlowLogToCloudWatch(callTime, vpcId,
                                                      logGroupName,
                                                      roleArn, tags)
    if err != nil {
        return "", fmt.Errorf("creating flow log to CloudWatch - %w", err)
    }

    return flowLogId, nil
}


// Delete VPC Flow Logs with support for multiple IDs.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - flowLogIds:  List of VPC Flow Logs IDs to delete
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (Ec2Man Ec2Manger) VpcFlowLogTerminator(callTime time.Duration,
                                             flowLogIds []string) error {
    // Ensure required arg is present
    if len(flowLogIds) == 0 {
        return fmt.Errorf("DeleteFlowLogs: no flow log IDs provided")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    deleteFlowLogsInput := &ec2.DeleteFlowLogsInput{
        FlowLogIds: flowLogIds,
    }

    // Delete the VPC flow logs
    _, err := Ec2Man.client.DeleteFlowLogs(ctx, deleteFlowLogsInput)
    if err != nil {
        return fmt.Errorf("deleting VPC Flow Logs - %w", err)
    }

    return nil
}
