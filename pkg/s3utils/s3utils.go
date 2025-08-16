package s3utils

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
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
//  - bucketName:  The name of the S3 bucket to check existence
//  - callTime:  The length of time the API call is allowed to execute
//
// @Returns
//  - Boolean toggle whether the bucket exists or not
//  - Error if it occurs, otherwise nil on success
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
//  - bucketName:  The name of the bucket to be created
//  - callTime:  The length of time the API call is allowed to execute
//
// @Returns
//  - Error if it occurs, otherwise nil on success
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
//  - bucketName:  The name of the bucket where the object will be retrieved
//  - key:  The key in bucket used to identify the object to retrieve
//  - callTime:  The length of time the API call is allowed to execute
//
// @Returns
//  - The retrieved S3 object as a byte slice
//  - Error if it occurs, otherwise nil on success
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
//  - bucketName:  The name of the S3 bucket where the object will be stored
//  - key:  The key in bucket used to identify where the object will be stored
//  - data:  The data to be stored associated with the key of in the S3 bucket
//  - callTime:  The length of time the API call is allowed to execute
//
// @Returns
//  - The final key name that is used
//  - Error if it occurs, otherwise nil on success
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
