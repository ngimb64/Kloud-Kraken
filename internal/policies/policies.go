package policies

import "fmt"

// Generates permissions policy for the client EC2.
//
// @Parameters
//  - bucketName:  The name of the S3 bucket where actions will be performed
//  - region:  The AWS region where actions will be performed
//  - accountId:  The AWS account ID where actions will be performed
//  - paramPath:  The path where the certificate is stored in SSM param store
//  - logGroup:  The name of the CloudWatch group being utilized
//
// @Returns
//  - The generated permissions policy with args formatted into it
//
func ClientPermPolicyGen(bucketName string, region string,
                         accountId string, paramPath string,
                         logGroup string) string {
    return fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "S3DownloadBinary",
      "Effect": "Allow",
      "Action": [
        "s3:GetObject"
      ],
      "Resource": "arn:aws:s3:::%s/*",
      "Condition": {
        "StringEquals": {
          "aws:ResourceTag/kloud-kraken": "true"
        }
      }
    },
    {
      "Sid": "SSMFetchParameters",
      "Effect": "Allow",
      "Action": [
        "ssm:GetParameter",
        "ssm:GetParameters",
        "ssm:GetParametersByPath"
      ],
      "Resource": "arn:aws:ssm:%s:%s:parameter%s*",
      "Condition": {
        "StringEquals": {
          "aws:ResourceTag/kloud-kraken": "true"
        }
      }
    },
    {
      "Sid": "CloudWatchLogging",
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents",
        "logs:DescribeLogStreams",
        "logs:PutRetentionPolicy",
        "logs:CreateTags"
      ],
      "Resource": "arn:aws:logs:%s:%s:log-group:/%s*",
      "Condition": {
        "StringEquals": {
          "aws:ResourceTag/kloud-kraken": "true"
        }
      }
    },
    {
      "Sid": "ManageSecurityGroupEgress",
      "Effect": "Allow",
      "Action": [
        "ec2:AuthorizeSecurityGroupEgress",
        "ec2:RevokeSecurityGroupEgress",
        "ec2:DescribeSecurityGroups"
      ],
      "Resource": "arn:aws:ec2:%s:%s:security-group/*",
      "Condition": {
        "StringEquals": {
          "aws:ResourceTag/kloud-kraken": "true"
        }
      }
    }
  ]
}`, bucketName, region, accountId, paramPath, region,
    accountId, logGroup, region, accountId)
}


// Generates trust policy for the client.
//
// @Returns
//  - The generated trust policy
//
func ClientTrustPolicyGen() string {
    return `{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect":    "Allow",
    "Principal": { "Service": "ec2.amazonaws.com" },
    "Action":    "sts:AssumeRole"
  }]
}`
}


// Generate permissions policy for the local server.
//
// @Parameters
//  - region:  The AWS region where actions will be performed
//  - accountId:  The AWS account ID where actions will be performed
//  - ssmParam:  The path where the certificate is stored in SSM param store
//  - bucketName:  The name of the S3 bucket where actions will be performed
//  - clientRoleName:  The name of IAM role the client will be using
//
// @Returns
//  - The generated permissions policy with args formatted into it
//
func ServerPermPolicyGen(region string, accountId string,
                         ssmParam string, bucketName string,
                         clientRoleName string) string {
    return fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "SSMUploadClientCert",
      "Effect": "Allow",
      "Action": [
        "ssm:PutParameter",
        "ssm:DeleteParameter",
        "ssm:AddTagsToResource"
      ],
      "Resource": "arn:aws:ssm:%s:%s:parameter%s*"
    },
    {
      "Sid": "S3Operations",
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:PutObjectAcl",
        "s3:ListBucket",
        "s3:DeleteObject",
        "s3:DeleteBucket"
      ],
      "Resource": [
        "arn:aws:s3:::%s",
        "arn:aws:s3:::%s/*"
      ]
    },
    {
      "Sid": "EC2LifecycleControl",
      "Effect": "Allow",
      "Action": [
        "ec2:RunInstances",
        "ec2:TerminateInstances",
        "ec2:DescribeInstances",
        "ec2:CreateTags",
        "ec2:DeleteNatGateway",
        "ec2:DescribeNatGateways",
        "ec2:ReleaseAddress",
        "ec2:DescribeAddresses",
        "ec2:DeleteVpcEndpoints",
        "ec2:DescribeVpcEndpoints"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "ec2:ResourceTag/kloud-kraken": "true"
        }
      }
    },
    {
      "Sid": "EC2PassRoleForInstanceProfile",
      "Effect": "Allow",
      "Action": [
        "iam:PassRole"
      ],
      "Resource": "arn:aws:iam::%s:role/%s"
    }
  ]
}`, region, accountId, ssmParam, bucketName,
    bucketName, accountId, clientRoleName)
}


// Generate trust policy for the server.
//
// @Parameters
//  - accountId:  The AWS account ID where actions will be performed
//  - iamUser:  The IAM user that the policy will apply to
//
// @Returns
//  - The generated trust policy with args formatted into it
//
func ServerTrustPolicyGen(accountId string, iamUser string) string {
    return fmt.Sprintf(`{
  "Version":"2012-10-17",
  "Statement":[{
    "Effect":"Allow",
    "Principal":{
      "AWS":"arn:aws:iam::%s:user/%s"
    },
    "Action":"sts:AssumeRole"
  }]
}`, accountId, iamUser)
}


// Generate permissions policy for VPC Flow Logs.
//
// @Parameters
//  - region:  The AWS region where the VPC Flow Logs are utilized
//  - accountId:  The AWS account ID number
//  - logGroupName:  The CloudWatch log group name where the logs are stored
//
// @Returns
//  - The generated permissions policy with args formatted into it
//
func VpcFlowLogsPermPolicyGen(region string, accountId string,
                              logGroupName string) string {
    return fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowCreateAndWriteToKloudKrakenFlowLogs",
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents",
        "logs:DescribeLogGroups",
        "logs:DescribeLogStreams"
      ],
      "Resource": [
        "arn:aws:logs:%s:%s:log-group:%s",
        "arn:aws:logs:%s:%s:log-group:%s:*",
        "arn:aws:logs:%s:%s:log-group:/aws/vpc-flow-logs/%s",
        "arn:aws:logs:%s:%s:log-group:/aws/vpc-flow-logs/%s:*"
      ]
    }
  ]
}`, region, accountId, logGroupName,
    region, accountId, logGroupName,
    region, accountId, logGroupName,
    region, accountId, logGroupName)
}


// Generate trust policy for the VPC Flow Logs.
//
// @Returns
//  - The generated trust policy with args formatted into it
//
func VpcFlowLogsTrustPolicyGen() string {
    return `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "vpc-flow-logs.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}`
}


// Generate permissions policy for the S3 VPC Endpoint.
//
// @Parameters
//  - bucketName:  The name of the S3 bucket
//  - iamArn:  The IAM ARN of S3 VPC Endpoint
//
// @Returns
//  - The generated permissions policy with args formatted into it
//
func VpcS3EndpointPolicyGen(bucketName string, iamArn string) string {
    return fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowKKUserToKKBucketOnly",
      "Effect": "Allow",
      "Principal": {
        "AWS": "%s"
      },
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::%s",
        "arn:aws:s3:::%s/*"
      ]
    }
  ]
}`, iamArn, bucketName, bucketName)
}


// Generate permissions policy for the SSM VPC Endpoint.
//
// @Parameters
//  - accountId:  The AWS account ID number
//  - iamArn:  The IAM ARN of the SSM VPC Endpoint
//
// @Returns
//  - The generated permissions policy with args formatted into it
//
func VpcSsmEndpointPolicyGen(accountId, region, iamArn string) string {
    return fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowKKUserSSMParams",
      "Effect": "Allow",
      "Principal": {
        "AWS": "%s"
      },
      "Action": [
        "ssm:GetParameter",
        "ssm:GetParameters",
        "ssm:GetParametersByPath",
        "ssm:PutParameter",
        "ssm:DeleteParameter"
      ],
      "Resource": "arn:aws:ssm:%s:%s:parameter/kloud-kraken/*"
    }
  ]
}`, iamArn, region, accountId)
}

