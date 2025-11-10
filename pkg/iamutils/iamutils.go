package iamutils

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/ngimb64/Kloud-Kraken/pkg/awsutils"
)

// Regex to match IAM ARN
var ReRoleArn = regexp.MustCompile(`^arn:aws(-[\w]+)?:iam::\d{12}:role/.+$`)


// Extracts the role name from a role ARN.
//
// Examples:
//   arn:aws:iam::123456789012:role/path/to/MyRole -> MyRole
//
// @Parameters
//  - The IAM role ARN to be parsed
//
// @Returns
//  - The role name parsed from ARN
//  - Error if it occurs, otherwise nil on success
//
func getRoleNameFromArn(roleArn string) (string, error) {
    // Ensure required arg is present
    if roleArn == "" {
        return "", errors.New("roleArn is missing")
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


// Struct for managing IAM operations
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
//  - tags:  String map of tag key-values to configure
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (IamMan *IamManager) createInstanceProfile(callTime time.Duration,
                                                roleName string,
                                                tags map[string]string) error {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    createInput := &iam.CreateInstanceProfileInput{
        InstanceProfileName: aws.String(roleName),
    }

    if len(tags) > 0 {
        instanceProfileTags := tags

        // If the name tag exists in tags map
        if name, ok := tags["Name"]; ok {
            instanceProfileTags["Name"] = name + "-instance-profile"
        }

        createInput.Tags = awsutils.BuildIamTags(instanceProfileTags)
    }

    // Create the instance profile
    _, err := IamMan.client.CreateInstanceProfile(ctx, createInput)
    if err != nil {
        var entityExists *iamtypes.EntityAlreadyExistsException

        // If the error is not that the instance profile already exists
        if !errors.As(err, &entityExists) {
            return fmt.Errorf("error creating instance profile - %w", err)
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
//  - tags:  String map of tag key-values to configure
//  - createProfile:  Toggle whether an instance profile is created or not
//
// @Returns
//  - The ARN of the created role
//  - Error if it occurs, otherwise nil on success
//
func (IamMan *IamManager) iamRoleCreation(callTime time.Duration, roleName string,
                                          trustPolicyJson string, permPolicyName string,
                                          permPolicyJson string, tags map[string]string,
                                          createProfile bool) (
                                          string, error) {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    createInput := &iam.CreateRoleInput{
        AssumeRolePolicyDocument: aws.String(trustPolicyJson),
        RoleName:                 aws.String(roleName),
    }

    if len(tags) > 0 {
        createInput.Tags = awsutils.BuildIamTags(tags)
    }

    // Create the IAM role
    createOut, err := IamMan.client.CreateRole(ctx, createInput )
    if err != nil {
        return "", fmt.Errorf("CreateRole failed - %w", err)
    }

    // Set the role ARN from output
    roleArn := aws.ToString(createOut.Role.Arn)

    // Attach or overwrite the inline permissions policy
    _, err = IamMan.client.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
        PolicyDocument: aws.String(permPolicyJson),
        PolicyName:     aws.String(permPolicyName),
        RoleName:       aws.String(roleName),
    })
    if err != nil {
        return roleArn, fmt.Errorf("PutRolePolicy failed -  %w", err)
    }

    // If specified, create instance profile and attach role to it
    if createProfile {
        err = IamMan.createInstanceProfile(callTime, roleName, tags)
        if err != nil {
            return roleArn, fmt.Errorf("creating EC2 instace profile - %w", err)
        }
    }

    return roleArn, nil
}

// Checks whether an IAM role exists by name.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - roleName:  The name of the IAM role to check existence
//
// @Returns
//  - Toggle for whether IAM role already exists or not
//  - Error if it occurs, otherwise nil on success
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

// Provision IAM role by checking for existence and creating if missing.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - roleArn:  The IAM role ARN
//  - defaultRoleName:  The name applied to role upon creation
//  - trustPolicyJson:  JSON trust policy
//  - permPolicyName:  The name of the permissions policy
//  - permPolicyJson:  JSON permissions policy
//  - tags:  String map of tag key-values to configure
//  - createProfile:  Toggle for whether an IAM instance profile should be made
//
// @Returns
//  - IAM role ARN if the resource is created, "" if it already exists
//  - Error if it occurs, otherwise nil on success
//
func (IamMan *IamManager) IamRoleProvision(callTime time.Duration,
                                           roleArn string,
                                           defaultRoleName string,
                                           trustPolicyJson string,
                                           permPolicyName string,
                                           permPolicyJson string,
                                           tags map[string]string,
                                           createProfile bool) (
                                           string, error) {
    // Ensure required args are present
    if defaultRoleName == "" || trustPolicyJson == "" ||
    permPolicyName == "" || permPolicyJson == "" {
        return "", errors.New("defaultRoleName or trustPolicyJson or" +
                              " permPolicyName or permPolicyJson is missing")
    }

    // If the IAM role ARN is present in state file
    if roleArn != "" {
        // Parse the role name from ARN to check existence
        roleName, err := getRoleNameFromArn(roleArn)
        if err != nil {
            return "", fmt.Errorf("parsing role arn - %w", err)
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
    return IamMan.iamRoleCreation(callTime, defaultRoleName,
                                  trustPolicyJson, permPolicyName,
                                  permPolicyJson, tags, createProfile)
}

// Handles the full deletion of an IAM role and its associate components.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - roleName:  The IAM Role used for operations, also supports ARNs
//  - deleteProfiles:  Toggle for specifying whether instance profiles
//                     associated with role should be deleted
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (IamMan IamManager) IamRoleTerminator(callTime time.Duration,
                                           roleName string,
                                           deleteProfiles bool) error {
    // Ensure required arg is present
    if roleName == "" {
        return errors.New("roleName is missing")
    }

    var err error

    // If the passed in rolename is a ARN, extract the rolename from it
    if ReRoleArn.MatchString(roleName) {
        // Parse the role name from ARN to check existence
        roleName, err = getRoleNameFromArn(roleName)
        if err != nil {
            return fmt.Errorf("parsing role arn - %w", err)
        }
    }

    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    profileNames := []string{}
    listInstanceProfilesInput := &iam.ListInstanceProfilesForRoleInput{
        RoleName: aws.String(roleName),
    }

    // Get list of instance profiles associated with role name
    instanceProfilePaginator := iam.NewListInstanceProfilesForRolePaginator(IamMan.client,
                                                                            listInstanceProfilesInput)

    // While the instance profile paginator has pages left
    for instanceProfilePaginator.HasMorePages() {
        // Get the next page for processing
        page, err := instanceProfilePaginator.NextPage(ctx)
        // If there was a error other than the entity does not exist
        if err != nil {
            if !strings.Contains(err.Error(), "NoSuchEntity") {
                return fmt.Errorf("list instance profiles for role - %w", err)
            }

            break
        }

        // Iterate through list of retrieved instance profiles
        for _, instanceProfile := range page.InstanceProfiles {
            // If the instance profile name is present, add it to profile names list
            if instanceProfile.InstanceProfileName != nil {
                profileNames = append(profileNames,
                                      aws.ToString(instanceProfile.InstanceProfileName))
            }
        }
    }

    // Remove the role from associated instance profiles
    err = IamMan.iamTerminateRoleFromProfiles(callTime, roleName)
    if err != nil {
        return fmt.Errorf("removing role %s from profiles - %w", roleName, err)
    }

    // Delete any inline policies the role has
    err = IamMan.iamTerminateAllInlinePolicies(callTime, roleName)
    if err != nil {
        return fmt.Errorf("deleting inline policies from role - %w", err)
    }

    // Detach any managed policies the role has
    err = IamMan.iamTerminateAllManagedPolicies(callTime, roleName)
    if err != nil {
        return fmt.Errorf("detaching managed polcies from role - %w", err)
    }

    // Delete the role itself
    err = IamMan.iamTerminateRole(callTime, roleName)
    if err != nil {
        return fmt.Errorf("deleting the IAM role %s - %w", roleName, err)
    }

    // If instance profiles associated with role are to deleted
    if deleteProfiles {
        // Iterate through instance profile names
        for _, profileName := range profileNames {
            if profileName == "" {
                continue
            }

            // Delete the IAM instance profile
            err = IamMan.iamTerminateInstanceProfile(callTime, roleName)
            if err != nil {
                return fmt.Errorf("deleting instance profile %s - %w",
                                  profileName, err)
            }
        }
    }

    return nil
}

// Deletes all inline policies associated with IAM role name.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - roleName:  The role name where the inline policies are applied
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (IamMan IamManager) iamTerminateAllInlinePolicies(callTime time.Duration,
                                                       roleName string) error {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    listRolePoliciesInput := &iam.ListRolePoliciesInput{
        RoleName: aws.String(roleName),
    }

    // Get list of inline policies with paginator support
    listRolePoliciesPaginator := iam.NewListRolePoliciesPaginator(IamMan.client,
                                                                  listRolePoliciesInput)

    // Iterate through paginator while there are more pages to be processed
    for listRolePoliciesPaginator.HasMorePages() {
        // Get the next page in paginator to process
        out, err := listRolePoliciesPaginator.NextPage(ctx)
        if err != nil {
            return fmt.Errorf("list inline policies - %w", err)
        }

        // Iterate through the retrieved policies in the page
        for _, policyName := range out.PolicyNames {
            deleteRolePolicyInput :=&iam.DeleteRolePolicyInput{
                PolicyName: aws.String(policyName),
                RoleName:   aws.String(roleName),
            }

            // Delete the role policy associated with role name
            _, err := IamMan.client.DeleteRolePolicy(ctx, deleteRolePolicyInput)
            // If there was a error other than the entity does not exist
            if err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
                return fmt.Errorf("delete inline policy %s - %w", policyName, err)
            }
        }
    }

    return nil
}


// Deletes all managed policies associated with IAM role name.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - roleName:  The role name where the managed policies are applied
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (IamMan *IamManager) iamTerminateAllManagedPolicies(callTime time.Duration,
                                                         roleName string) error {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    listAttachedRoleInput := &iam.ListAttachedRolePoliciesInput{
        RoleName: aws.String(roleName),
    }

    // Get list of managed role policies with paginator support
    attachedRolePaginator := iam.NewListAttachedRolePoliciesPaginator(IamMan.client,
                                                                      listAttachedRoleInput)

    // While the attached role paginator has pages left
    for attachedRolePaginator.HasMorePages() {
        // Get the next page in paginator to process
        page, err := attachedRolePaginator.NextPage(ctx)
        if err != nil {
            return fmt.Errorf("list attached policies - %w", err)
        }

        // Iterate through the retrieved policies in the page
        for _, attachedPolicy := range page.AttachedPolicies {
            detachRolePolicyInput := &iam.DetachRolePolicyInput{
                PolicyArn: attachedPolicy.PolicyArn,
                RoleName:  aws.String(roleName),
            }

            // Detach policy from associated role name
            _, err := IamMan.client.DetachRolePolicy(ctx, detachRolePolicyInput)
            // If there was a error other than the entity does not exist
            if err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
                return fmt.Errorf("detach policy %s - %w",
                                  aws.ToString(attachedPolicy.PolicyArn), err)
            }
        }
    }

    return nil
}


// Delete the instance profile by name.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - profileName:  Name of instance profile to delete
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (IamMan IamManager) iamTerminateInstanceProfile(callTime time.Duration,
                                                     profileName string) error {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    deleteInstanceProfileInput := &iam.DeleteInstanceProfileInput{
        InstanceProfileName: aws.String(profileName),
    }

    // Delete the instance profile
    _, err := IamMan.client.DeleteInstanceProfile(ctx, deleteInstanceProfileInput)
    // If there was a error other than the entity does not exist
    if err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
        return fmt.Errorf("delete instance profile %s - %w", profileName, err)
    }

    return nil
}


// Delete the IAM role by name.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - roleName:  Name of the IAM role to be deleted
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (IamMan IamManager) iamTerminateRole(callTime time.Duration,
                                          roleName string) error {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    // Delete the IAM role
    _, err := IamMan.client.DeleteRole(ctx, &iam.DeleteRoleInput{
        RoleName: aws.String(roleName),
    })
    // If there was a error other than the entity does not exist
    if err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
        return fmt.Errorf("delete role %s - %w", roleName, err)
    }

    return nil
}


// Remove IAM role from all associated instance profiles.
//
// @Parameters
//  - callTime:  The length of time the API call is allowed to execute
//  - roleName:  The role name to remove from instance profiles
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func (IamMan IamManager) iamTerminateRoleFromProfiles(callTime time.Duration,
                                                      roleName string) error {
    // Ensure AWS API calls do not hang for longer specified timeout
    ctx, cancel := context.WithTimeout(context.Background(), callTime)
    defer cancel()

    listInstanceProfilesInput := &iam.ListInstanceProfilesForRoleInput{
        RoleName: aws.String(roleName),
    }

    // List the instance profiles that are associated with the role
    listOut, err := IamMan.client.ListInstanceProfilesForRole(ctx,
                                                              listInstanceProfilesInput)
    if err != nil {
        if strings.Contains(err.Error(), "NoSuchEntity") {
            return nil
        }

        return fmt.Errorf("list instance profiles for %s - %w", roleName, err)
    }

    // Iterate through the list of instance profiles
    for _, ip := range listOut.InstanceProfiles {
        removeRoleInput := &iam.RemoveRoleFromInstanceProfileInput{
            InstanceProfileName: ip.InstanceProfileName,
            RoleName:            aws.String(roleName),
        }

        // Remove the role from the instance profile
        _, err := IamMan.client.RemoveRoleFromInstanceProfile(ctx, removeRoleInput)
        // If there was a error other than the entity does not exist
        if err != nil && !strings.Contains(err.Error(), "NoSuchEntity") {
            return fmt.Errorf("remove role %s from profile %s - %w", roleName,
                              aws.ToString(ip.InstanceProfileName), err)
        }
    }

    return nil
}
