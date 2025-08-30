package iamutils

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// Struct for managing S3 bucket operations
type IamManager struct {
    client  *iam.Client
}

// Establishes connection to IAM service and generates IAM manager struct
//
// @Parameters
//  - awsConfig:  The AWS credential configuration for connecting to service
//
// @Returns
//  - The initialized IAM manager with populated data
//
func IamNewManager(awsConfig aws.Config) *IamManager {
    // Setup a new EC2 client
    iamClient := iam.NewFromConfig(awsConfig)

    return &IamManager{
        client:     iamClient,
    }
}

// Creates an instance profile with passed and attaches role to it.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - roleName:  The IAM Role used for operations
//
// @Returns
//  - Error if it occurs, otherwise nil on success
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
            return fmt.Errorf("instance profile already exists - %w", err)
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
        return fmt.Errorf("attaching role to instance - %w", err)
    }

    return nil
}


// Creates an IAM role with the passed in JSON policy data applied.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - roleName:  The IAM Role used for operations
//  - trustPolicyJson:  The JSON trust policy
//  - permPolicyName:  An identifier name for permissions policy
//  - permPolicyJSON:  The JSON permissions policy
//  - createProfile:  Toggle whether an instance profile is created or not
//
// @Returns
//  - The ARN of the existing or created role
//  - Error if it occurs, otherwise nil on success
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
                return "", fmt.Errorf("CreateRole failed - %w", err)
            }

            // Set the role ARN from output
            roleArn = aws.ToString(createOut.Role.Arn)
        } else {
            return "", fmt.Errorf("GetRole failed - %w", err)
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
        return "", fmt.Errorf("PutRolePolicy failed -  %w", err)
    }

    // If specified, create instance profile and attach role to it
    if createProfile {
        err = IamMan.createInstanceProfile(callTime, roleName)
        if err != nil {
            return "", fmt.Errorf("creating EC2 instace profile - %w", err)
        }
    }

    return roleArn, nil
}
