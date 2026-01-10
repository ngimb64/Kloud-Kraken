package vpcsetup

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/ngimb64/Kloud-Kraken/internal/color"
	"github.com/ngimb64/Kloud-Kraken/pkg/display"
	"github.com/ngimb64/Kloud-Kraken/pkg/s3utils"
)

// Handler function for setting up S3 bucket.
//
// @Parameters
//  - stateConfig:  Pointer to config struct for state file
//  - yamlUpdates:  The map used for updating output YAML data
//  - outStruct:  Pointer to struct used for managing vcpsetup outputs
//  - awsConfig:  The AWS configuration instance
//
// @Returns
//  - Error if it occurs, otherwise nil on success
//
func SetupS3BucketHandler(stateConfig *AwsEnv,
                          yamlUpdates map[string]string,
                          outStruct *VpcBootstrapOutput,
                          awsConfig aws.Config) error {
    tags := map[string]string{
        "kloud-kraken": "true",
        "Name": "kloud-kraken-s3",
    }

    fmt.Println(display.CtextMulti(display.CtextPrefix(color.KrakenPurple,
                                                       color.LightCyan, "!"), "",
                                   color.NeonAzure, "Launching S3 bucket provisioner"))

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

        fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "S3 bucket was created"))
    // Otherwise use the one from YAML since it was found
    } else {
        bucketName = stateConfig.AwsEnv.S3BucketName

        fmt.Println(display.CtextMulti(color.FoamWhite, "  \\-->",
                                       display.CtextPrefix(color.KrakenPurple,
                                                           color.LightCyan, "$"), "",
                                       color.NeonAzure, "S3 bucket already exists"))
    }

    outStruct.S3BucketName = bucketName
    return nil
}
