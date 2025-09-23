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

//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) createFlowLogToCloudWatch(callTime time.Duration,
                                                   vpcID string,
                                                   logGroupName string,
                                                   roleArn string,
                                                   tagName string) (
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

    if tagName != "" {
        callInput.TagSpecifications = []ec2types.TagSpecification{
            {
                ResourceType: ec2types.ResourceTypeVpcFlowLog,
                Tags: []ec2types.Tag{
                    {
                        Key: aws.String("Name"),
                        Value: aws.String(tagName + "-flow-logs"),
                    },
                },
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

// Checks if flow log group exists via resource-id filter.
//
// @Parameters
//
//
// @Returns
//
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

// Creates a CloudWatch Logs log group if not present.
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man *Ec2Manger) VpcFlowLogProvision(callTime time.Duration,
                                             flowLogId string, vpcId string,
                                             cwlClient *cloudwatchlogs.Client,
                                             logGroupName string, roleArn string,
                                             retentionDays int32, tags map[string]string) (
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
        callInput.Tags = tags
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
                                                      roleArn,
                                                      tags["Name"])
    if err != nil {
        return "", fmt.Errorf("creating flow log to CloudWatch - %w", err)
    }

    return flowLogId, nil
}


//
//
// @Parameters
//
//
// @Returns
//
//
func (Ec2Man Ec2Manger) VpcFlowLogTerminate(callTime time.Duration,
                                            flowLogIDs []string) error {
    // Ensure required arg is present
    if len(flowLogIDs) == 0 {
        return fmt.Errorf("DeleteFlowLogs: no flow log IDs provided")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    deleteFlowLogsInput := &ec2.DeleteFlowLogsInput{
        FlowLogIds: flowLogIDs,
    }

    // Delete the VPC flow logs
    _, err := Ec2Man.client.DeleteFlowLogs(ctx, deleteFlowLogsInput)
    if err != nil {
        return fmt.Errorf("deleting VPC Flow Logs - %w", err)
    }

    return nil
}
