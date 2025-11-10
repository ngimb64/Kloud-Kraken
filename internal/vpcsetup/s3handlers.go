package vpcsetup

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/pkg/awscost"
	"github.com/ngimb64/Kloud-Kraken/pkg/ec2utils"
	"github.com/ngimb64/Kloud-Kraken/pkg/s3utils"
)

// Handler function for setting up S3 bucket.
//
// @Parameters
//  - ec2Client:  Pointer to EC2 service client management struct
//  - stateConfig:  Pointer to config struct for state file
//  - appConfig:  Pointer to program config instance from YAML data
//  - yamlUpdates:  The map used for updating output YAML data
//  - outStruct:  Pointer to struct used for managing vcpsetup outputs
//  - location:  The human readable version of region
//  - costErr:  Pointer to error instance for cost manager
//  - costMan:  Pointer to AWS cost manager instance
//  - awsConfig:  The AWS configuration instance
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func SetupS3BucketHandler(ec2Client *ec2utils.Ec2Manger,
                          stateConfig *AwsEnv,
                          appConfig *conf.AppConfig,
                          yamlUpdates map[string]string,
                          outStruct *VpcBootstrapOutput,
                          location string, costErr *error,
                          costMan *awscost.AwsCostManager,
                          awsConfig aws.Config) error {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-s3",
    }

    // Set up client to S3 service
    s3Client := s3utils.S3NewManager(awsConfig)
    // Create a S3 bucket if it does not exist
    bucketName, err := s3Client.S3BucketProvision(5 * time.Minute,
                                                  stateConfig.AwsEnv.S3BucketName,
                                                  "kloud-kraken-s3",
                                                  awsConfig.Region, tags)
    if err != nil {
        return err
    }

    // If S3 buccket created, add name to yaml updates map
    if bucketName != "" {
        yamlUpdates["aws_env.s3_bucket_name"] = bucketName
    // Otherwise use the one from YAML since it was found
    } else {
        bucketName = stateConfig.AwsEnv.S3BucketName
    }

    outStruct.S3BucketName = bucketName

    return nil
}
