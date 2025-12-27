package cloudwatchutils

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwl "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// Get CloudWatch log streams associated with passed in log group is none
// are already present. Delete the log streams associated with log group,
// then finally delete the log group.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - client:  Established client to CloudWatch services
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func CloudWatchLoggerTerminator(callTime time.Duration, cwlClient *cwl.Client,
                                logGroupName string, streams []string) error {
    // Ensure required args are present
    if logGroupName == "" {
        return errors.New("logGroupName is missing")
    }

    deleteStreams := false
    var err error

    // If there are no streams currently to be deleted
    if len(streams) == 0 {
        // Get log streams associated with log group name
        streams, err = GetLogStreams(1 * time.Minute, cwlClient,
                                     logGroupName)
        if err != nil {
            var resourceNotFound *cwltypes.ResourceNotFoundException

            // If error is other than log group does not exist
            if !errors.As(err, &resourceNotFound) {
                return fmt.Errorf("GetLogStreams %s - %w", logGroupName, err)
            }
        }

        // If log streams were retrieved
        if len(streams) > 0 {
            deleteStreams = true
        }
    } else {
        deleteStreams = true
    }

    if deleteStreams {
        // Delete the CloudWatch log stream
        err = DeleteLogStreams(1 * time.Minute, cwlClient,
                               logGroupName, streams)
        if err != nil {
            return fmt.Errorf("deleting stream associated with log group %s - %w",
                            logGroupName, err)
        }
    }

    // Delete the CloudWatch log group
    err = DeleteLogGroup(1 * time.Minute, cwlClient, logGroupName)
    if err != nil {
        return fmt.Errorf("deleting log group %s - %w", logGroupName, err)
    }

    return nil
}


// Delete the CloudWatch log group.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - client:  Established client to CloudWatch services
//  - logGroupName:  The name of the log group to be deleted
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func DeleteLogGroup(callTime time.Duration, client *cwl.Client,
                    logGroupName string) error {
    // Ensure required arg is present
    if logGroupName == "" {
        return nil
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    deleteLogGroupInput := &cwl.DeleteLogGroupInput{
        LogGroupName: aws.String(logGroupName),
    }

    // Delete the CloudWatch log group
    _, err := client.DeleteLogGroup(ctx, deleteLogGroupInput)
    if err != nil {
        var resourceNotFound *cwltypes.ResourceNotFoundException

        // If error is other than log group does not exist
        if !errors.As(err, &resourceNotFound) {
            return fmt.Errorf("DeleteLogGroup %s - %w", logGroupName, err)
        }
    }

    return nil
}


// Deletes list of log streams associated with specified log group.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - client:  Established client to CloudWatch services
//  - logGroupName:  The name of the log group where the streams are present
//  - streams:  List of streams to be deleted
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func DeleteLogStreams(callTime time.Duration, client *cwl.Client,
                      logGroupName string, streams []string) error {
    // Ensure required args are present
    if logGroupName == "" || len(streams) == 0 {
        return errors.New("logGroupName is missing or stream slice is empty")
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Iterate through list of CloudWatch streams
    for _, stream := range streams {
        deleteLogStreamInput := &cwl.DeleteLogStreamInput{
            LogGroupName:  aws.String(logGroupName),
            LogStreamName: aws.String(stream),
        }

        // Delete the current log stream in list
        _, err := client.DeleteLogStream(ctx, deleteLogStreamInput)
        if err != nil {
            var resourceNotFound *cwltypes.ResourceNotFoundException

            // If error is other than log group does not exist
            if !errors.As(err, &resourceNotFound) {
                return fmt.Errorf("deleting the log stream %s/%s - %w",
                                  logGroupName, stream, err)
            }
        }
    }

    return nil
}


// Gets the log streams associated with specified log group name.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - client:  Established client to CloudWatch services
//  - logGroupName:  The name of the log group to retrieve its streams
//
// @Returns
//  - List of retrieved log stream names
//  - Error if it occurs, otherwise nil on success
//
func GetLogStreams(callTime time.Duration, cwlClient *cwl.Client,
                   logGroupName string) (
                   []string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    var nextToken *string
    toDelete := make([]string, 0)

    // Loop until all log streams have been processed
    for {
        describeLogStreamsInput := &cwl.DescribeLogStreamsInput{
            LogGroupName: aws.String(logGroupName),
            NextToken:    nextToken,
        }

        // Get up to 25 CloudWatch log streams associated with log group
        describeOut, err := cwlClient.DescribeLogStreams(ctx, describeLogStreamsInput)
        if err != nil {
            return toDelete, fmt.Errorf("describing log streams %s - %w",
                                        logGroupName, err)
        }

        // Iterate through the list of retrieve log streams
        for _, logStream := range describeOut.LogStreams {
            // If the log stream name is present, add it to return list
            if logStream.LogStreamName != nil {
                toDelete = append(toDelete, *logStream.LogStreamName)
            }
        }

        // If next token is not present in output, all streams have been processed
        if describeOut.NextToken == nil || *describeOut.NextToken == "" {
            break
        }

        nextToken = describeOut.NextToken
    }

    return toDelete, nil
}
