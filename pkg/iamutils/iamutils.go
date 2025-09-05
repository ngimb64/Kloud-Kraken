package iamutils

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
    defer cancel()

    createCallInput := &iam.CreateInstanceProfileInput{
        InstanceProfileName: aws.String(roleName),
    }

    // Create the instance profile
    _, err := IamMan.client.CreateInstanceProfile(ctx, createCallInput)
    if err != nil {
        var entityExists *iamtypes.EntityAlreadyExistsException

        // If the error is not that the instance profile already exists
        if !errors.As(err, &entityExists) {
            return fmt.Errorf("instance profile already exists - %w", err)
        }
    }

    addRoleCallInput := &iam.AddRoleToInstanceProfileInput{
        InstanceProfileName: aws.String(roleName),
        RoleName:            aws.String(roleName),
    }

    // Add role to the instance profile
    _, err = IamMan.client.AddRoleToInstanceProfile(ctx, addRoleCallInput)
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
func (IamMan *IamManager) iamRoleCreation(callTime time.Duration, roleName string,
                                          trustPolicyJson string, permPolicyName string,
                                          permPolicyJson string, createProfile bool) (
                                          string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Create the IAM role
    createOut, err := IamMan.client.CreateRole(ctx, &iam.CreateRoleInput{
        RoleName:                 aws.String(roleName),
        AssumeRolePolicyDocument: aws.String(trustPolicyJson),
    })
    if err != nil {
        return "", fmt.Errorf("CreateRole failed - %w", err)
    }

    // Set the role ARN from output
    roleArn := aws.ToString(createOut.Role.Arn)

    // Attach or overwrite the inline permissions policy
    _, err = IamMan.client.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
        RoleName:       aws.String(roleName),
        PolicyName:     aws.String(permPolicyName),
        PolicyDocument: aws.String(permPolicyJson),
    })
    if err != nil {
        return roleArn, fmt.Errorf("PutRolePolicy failed -  %w", err)
    }

    // If specified, create instance profile and attach role to it
    if createProfile {
        err = IamMan.createInstanceProfile(callTime, roleName)
        if err != nil {
            return roleArn, fmt.Errorf("creating EC2 instace profile - %w", err)
        }
    }

    return roleArn, nil
}

// Checks whether an IAM role exists by name.
//
// @Parameters
//
//
// @Returns
//
//
func (IamMan *IamManager) IamRoleExists(callTime time.Duration,
                                        roleName string) (
                                        bool, error) {
    // Ensure required arg is present
    if roleName == "" {
        return false, fmt.Errorf("roleName is missing")
    }

    // Ensure API calls do not hang for than longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Check if the IAM role exists
    _, err := IamMan.client.GetRole(ctx, &iam.GetRoleInput{
        RoleName: aws.String(roleName),
    })
    if err != nil {
        var noSuch *iamtypes.NoSuchEntityException

        // If the IAM role does not exist
        if errors.As(err, &noSuch) {
            return false, nil
        }

        return false, fmt.Errorf("GetRole failed - %w", err)
    }

    return true, nil
}

//
//
// @Parameters
//
//
// @Returns
//
//
func (IamMan *IamManager) IamRoleProvision(callTime time.Duration,
                                           roleArn string,
                                           defaultRoleName string,
                                           trustPolicyJson string,
                                           permPolicyName string,
                                           permPolicyJson string,
                                           createProfile bool) (
                                           string, error) {
    // Ensure required args are present
    if defaultRoleName == "" || trustPolicyJson == "" ||
    permPolicyName == "" || permPolicyJson == "" {
        return "", fmt.Errorf("defaultRoleName or trustPolicyJson or" +
                              " permPolicyName or permPolicyJson is missing")
    }

    // If the IAM role ARN is present in state file
    if roleArn != "" {
        // Parse the role name from ARN to check existence
        roleName, err := roleNameFromArn(roleArn)
        if err != nil {
            return "", fmt.Errorf("parsing role arn: %w", err)
        }

        // Check to see if the IAM role already exists
        exists, err := IamMan.IamRoleExists(callTime, roleName)
        if err != nil {
            return "", err
        }

        // If the IAM role already exists, exist early
        if exists {
            return "", nil
        }
    }

    // Create the IAM role using the default assigned name
    return IamMan.iamRoleCreation(callTime, defaultRoleName, trustPolicyJson,
                                  permPolicyName, permPolicyJson, createProfile)
}

// Extracts the role name from a role ARN.
//
// Examples:
//   arn:aws:iam::123456789012:role/path/to/MyRole -> MyRole
//
// @Parameters
//
//
// @Returns
//
//
func roleNameFromArn(roleArn string) (string, error) {
    // Ensure required arg is present
    if roleArn == "" {
        return "", fmt.Errorf("roleArn is missing")
    }

    // ARN should contain ":role/" and the role name is after the last '/'
    if !strings.Contains(roleArn, ":role/") {
        return "", fmt.Errorf("invalid role ARN (missing ':role/') - %s", roleArn)
    }

    // Get the last slash index in role ARN
    idx := strings.LastIndex(roleArn, "/")

    // If the index was not found or there is nothing after it
    if idx == -1 || idx == len(roleArn)-1 {
        return "", fmt.Errorf("invalid role ARN format - %s", roleArn)
    }

    return roleArn[idx+1:], nil
}
