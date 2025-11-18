package s3utils

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
)

// Struct for managing S3 bucket operations
type S3Manager struct {
    client     *s3.Client
}

// Establishes connection to EC2 service and generates EC2 manager struct
//
// @Parameters
//  - awsConfig:  The AWS credential configuration for connecting to service
//
// @Returns
//  - The initialized S3 manager with client reference
//
func S3NewManager(config aws.Config) *S3Manager {
    // Set up a new S3 client
    s3Client := s3.NewFromConfig(config)

    return &S3Manager{
        client:     s3Client,
    }
}

// Create an S3 bucket.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - bucketName:  The name of the bucket to be created
//  - tags:  String map of tag key-values to configure
//
// @Returns
//  - Name of the created S3 bucket
//  - Error if it occurs, otherwise nil on success
//
func (S3Man *S3Manager) s3BucketCreate(callTime time.Duration,
                                       bucketName string,
                                       region string,
                                       tags map[string]string) (
                                       string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &s3.CreateBucketInput{
        Bucket: aws.String(bucketName),
    }

    // If the region is not the default, set location restraint
    if region != "us-east-1" {
        callInput.CreateBucketConfiguration = &s3types.CreateBucketConfiguration{
            LocationConstraint: s3types.BucketLocationConstraint(region),
        }
    }

    // Create the bucket based on the bucket name in S3 manager
    out, err := S3Man.client.CreateBucket(ctx, callInput)
    // If the bucket was successfully created
    if err != nil {
        var apiErr smithy.APIError

        // If an API error occured
        if errors.As(err, &apiErr) {
            // Get the error code
            errCode := apiErr.ErrorCode()
            // If the error code signals the bucket already exists
            if errCode == "BucketAlreadyExists" ||
            errCode == "BucketAlreadyOwnedByYou" {
                return "", errors.New("S3 bucket already exists")
            }
        }

        // If a non API related error occured during request
        return "", fmt.Errorf("bucket create - %w", err)
    }

    // If no bucket was created
    if out == nil {
        return "", errors.New("S3 bucket failed to create")
    }

    waiterCallInput := s3.HeadBucketInput{
        Bucket: &bucketName,
    }

    // Allocate waiter and wait until the VPC exists
    waiter := s3.NewBucketExistsWaiter(S3Man.client)
    err = waiter.Wait(ctx, &waiterCallInput, callTime)
    if err != nil {
        return bucketName, err
    }

    if len(tags) > 0 {
        putBucketTagInput := &s3.PutBucketTaggingInput{
            Bucket: aws.String(bucketName),
            Tagging: &s3types.Tagging{
                TagSet: awsutils.BuildS3Tags(tags),
            },
        }

        // Tag the bucket after creation if there are tags
        _, err = S3Man.client.PutBucketTagging(ctx, putBucketTagInput)
        if err != nil {
            return bucketName, err
        }
    }

    return bucketName, nil
}

// Checks whether passed in S3 bucket name exists.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - bucketName:  The name of the S3 bucket to check existence
//
// @Returns
//  - Toggle for whether the bucket already exists or not
//  - Error if it occurs, otherwise nil on success
//
func (S3Man *S3Manager) S3BucketExists(callTime time.Duration,
                                       bucketName string) (
                                       bool, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &s3.HeadBucketInput{
        Bucket: aws.String(bucketName),
    }

    // Check if the bucket exists and get information
    out, err := S3Man.client.HeadBucket(ctx, callInput)
    if err != nil {
        var apiErr smithy.APIError

        // If an API error occured
        if errors.As(err, &apiErr) {
            // Get the error code
            errCode := apiErr.ErrorCode()
            // If the error code signals the buck does not exist
            if errCode == "EndpointNotFound" || errCode == "NoSuchBucket" {
                return false, nil
            }
        }

        // If a non API related error occured during request
        return false, fmt.Errorf("bucket exists - %w", err)
    }

    // If no bucket exists
    if out == nil || len(*out.BucketRegion) == 0 {
        return false, nil
    }

    return true, nil
}

// Retrieve object from S3 bucket.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - bucketName:  The name of the bucket where the object will be retrieved
//  - key:  The key in bucket used to identify the object to retrieve
//
// @Returns
//  - The retrieved S3 object as a byte slice
//  - Error if it occurs, otherwise nil on success
//
func (S3Man *S3Manager) S3GetObject(callTime time.Duration,
                                    bucketName string, key string) (
                                    _ []byte, err error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    callInput := &s3.GetObjectInput{
        Bucket: aws.String(bucketName),
        Key:    aws.String(key),
    }

    // Retrieve the object from S3 storage
    resp, err := S3Man.client.GetObject(ctx, callInput)
    if err != nil {
        return nil, err
    }

    // Close response body on local exit
    defer func() {
        cerr := resp.Body.Close()
        if cerr != nil {
            err = errors.Join(err, fmt.Errorf("closing S3 Get request - %w", err))
        }
    }()

    // Read all the data from the request
    rawData, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, err
    }

    return rawData, nil
}

// Provision S3 bucket by checking for existence and creating if missing.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - bucketName:  The S3 bucket name
//  - defaultBucketName:  The default S3 bucket name used for creation
//  - tags:  String map of tag key-values to configure
//
// @Returns
//  - S3 bucket name if the resource is created, "" if it already exists
//  - Error if it occurs, otherwise nil on success
//
func (S3Man *S3Manager) S3BucketProvision(callTime time.Duration,
                                          bucketName string,
                                          defaultBucketName string,
                                          region string,
                                          tags map[string]string) (
                                          string, error) {
    // If the bucket name is present in state file
    if bucketName != "" {
        // Check to see if it exists in AWS environment
        bucketExists, err := S3Man.S3BucketExists(callTime, bucketName)
        if err != nil {
            return "", err
        }

        // If the S3 bucket exists, exit early
        if bucketExists {
            return "", nil
        }
    }

    // Create S3 bucket with default name
    return S3Man.s3BucketCreate(callTime, defaultBucketName, region, tags)
}

// Put an object into a S3 bucket.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - bucketName:  The name of the S3 bucket where the object will be stored
//  - key:  The key in bucket used to identify where the object will be stored
//  - data:  The data to be stored associated with the key of in the S3 bucket
//
// @Returns
//  - The final key name that is used
//  - Error if it occurs, otherwise nil on success
//
func (S3Man *S3Manager) S3PutObject(callTime time.Duration,
                                    bucketName string,
                                    key string, data []byte) (
                                    string, error) {
    // Keep attemping key with number added until unused is found
    for i := 1; ; i++ {
        // Add number to end of key name
        candidate := key + "-" + strconv.Itoa(i)
        // Ensure AWS API calls do not hang for longer specified timeout
        ctx, cancel := context.WithTimeout(context.Background(), callTime)

        callInput := &s3.PutObjectInput{
            Bucket:      aws.String(bucketName),
            Key:         aws.String(candidate),
            Body:        bytes.NewReader(data),
            IfNoneMatch: aws.String("*"),
        }

        // Put the object in S3 storage based on key
        _, err := S3Man.client.PutObject(ctx, callInput)
        // Cancel context per API call
        cancel()
        if err != nil {
            var apiErr smithy.APIError

            // If the object already exists
            if errors.As(err, &apiErr) &&
            apiErr.ErrorCode() == "PreconditionFailed" {
                continue
            }

            return "", err
        }

        return candidate, nil
    }
}


// Deletes all objects (handles pagination) then deletes S3 bucket.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - bucketName:  The name of the S3 bucket to be deleted
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (S3Man *S3Manager) S3BucketTerminator(callTime time.Duration,
                                           bucketName string) error {
    var token *string

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    for {
        listCallInput := &s3.ListObjectsV2Input{
            Bucket:            aws.String(bucketName),
            ContinuationToken: token,
        }

        // Get a list of up to 1,000 object in bucket (pagination limits)
        listOut, err := S3Man.client.ListObjectsV2(ctx, listCallInput)
        if err != nil {
            return fmt.Errorf("listing S3 bucket objects - %w", err)
        }

        // If there is objects in the S3 bucket
        if len(listOut.Contents) > 0 {
            var objects []s3types.ObjectIdentifier

            // Iterate through the retrived objects and add them to objects list
            for _, object := range listOut.Contents {
                objects = append(objects, s3types.ObjectIdentifier{
                    Key: object.Key,
                })
            }

            // Delete all the S3 objects added to the objects list
            _, err = S3Man.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
                Bucket: aws.String(bucketName),
                Delete: &s3types.Delete{Objects: objects},
            })
            if err != nil {
                return fmt.Errorf("delete objects: %w", err)
            }
        }

        // If the last of pagination results is met
        if !*listOut.IsTruncated {
            break
        }

        token = listOut.NextContinuationToken
    }

    // Once objects in bucket are deleted, delete bucket itself
    _, err := S3Man.client.DeleteBucket(ctx, &s3.DeleteBucketInput{
        Bucket: aws.String(bucketName),
    })
    if err != nil {
        return fmt.Errorf("deleting S3 bucket - %w", err)
    }

    return nil
}
