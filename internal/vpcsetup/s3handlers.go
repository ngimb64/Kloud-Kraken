package vpcsetup

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/ngimb64/Kloud-Kraken/internal/conf"
	"github.com/ngimb64/Kloud-Kraken/pkg/ec2utils"
	"github.com/ngimb64/Kloud-Kraken/pkg/s3utils"
)

//
//
// @Parameters
//
//
// @Returns
//
//
func SetupS3BucketHandler(ec2Client *ec2utils.Ec2Manger,
                          stateConfig *AwsEnv,
                          appConfig *conf.AppConfig,
                          yamlUpdates map[string]string,
                          outStruct *VpcBootstrapOutput,
                          awsConfig aws.Config) (
                          string, error) {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-s3",
    }

    // Set up client to S3 service
    s3Client := s3utils.S3NewManager(awsConfig)
    // Create a S3 bucket if it does not exist
    bucketName, err := s3Client.S3BucketProvision(5 * time.Minute,
                                                  stateConfig.AwsEnv.S3BucketName,
                                                  "kloud-kraken-s3", tags)
    if err != nil {
        return bucketName, err
    }

    // If S3 buccket created, add name to yaml updates map
    if bucketName != "" {
        yamlUpdates["aws_env.s3_bucket_name"] = bucketName
    // Otherwise use the one from YAML since it was found
    } else {
        bucketName = stateConfig.AwsEnv.S3BucketName
    }

    outStruct.S3BucketName = bucketName
    return bucketName, nil
}
