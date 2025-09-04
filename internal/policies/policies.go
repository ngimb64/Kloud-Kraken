package policies

import "fmt"

// Generates permission policy for the client EC2.
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
      "Resource": "arn:aws:s3:::%s/*"
    },
    {
      "Sid": "SSMFetchParameters",
      "Effect": "Allow",
      "Action": [
        "ssm:GetParameter",
        "ssm:GetParameters",
        "ssm:GetParametersByPath"
      ],
      "Resource": [
        "arn:aws:ssm:%s:%s:parameter%s*"
      ]
    },
    {
      "Sid": "CloudWatchLogging",
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents"
      ],
      "Resource": "arn:aws:logs:%s:%s:log-group:/%s*"
    }
  ]
}`, bucketName, region, accountId, paramPath, region, accountId, logGroup)
}


// Generates trust policy for the client.
//
// @Returns
//  - The generated trust policy with args formatted into it
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


// Generates permission policy for the local server.
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
        "ssm:PutParameter"
      ],
      "Resource": "arn:aws:ssm:%s:%s:parameter%s*"
    },
    {
      "Sid": "S3UploadClientBinary",
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:PutObjectAcl"
      ],
      "Resource": "arn:aws:s3:::%s/*"
    },
    {
      "Sid": "EC2LifecycleControl",
      "Effect": "Allow",
      "Action": [
        "ec2:RunInstances",
        "ec2:TerminateInstances",
        "ec2:DescribeInstances",
        "ec2:CreateTags"
      ],
      "Resource": [
        "arn:aws:ec2:%s:%s:instance/*",
        "arn:aws:ec2:%s:%s:subnet/*",
        "arn:aws:ec2:%s:%s:security-group/*"
      ]
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
}`, region, accountId, ssmParam, bucketName, region, accountId, region,
    accountId, region, accountId, accountId, clientRoleName)
}


// Generates trust policy for the server.
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


//
//
// @Parameters
//
//
// @Returns
//
//
func VpcFlowLogsPermPolicyGen() string {
	return `{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {
      "Service": "vpc-flow-logs.amazonaws.com"
    },
    "Action": "sts:AssumeRole"
  }]
}`
}


//
//
// @Parameters
//
//
// @Returns
//
//
func VpcFlowLogsTrustPolicyGen() string {
	return `{
  "Version":"2012-10-17",
  "Statement":[{
    "Effect":"Allow",
    "Action":[
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:PutLogEvents",
      "logs:DescribeLogGroups",
      "logs:DescribeLogStreams"
    ],
    "Resource":"*"
  }]
}`
}


//
//
// @Parameters
//
//
// @Returns
//
//
func VpcS3EndpointPolicyGen(bucketName string, vpcId string) string {
    return fmt.Sprintf(`{
  "Statement": [
    {
      "Principal": "*",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:ListBucket"
      ],
      "Effect": "Allow",
      "Resource": [
        "arn:aws:s3:::%s",
        "arn:aws:s3:::%s/*"
      ],
      "Condition": {
        "StringEquals": {
          "aws:SourceVpc": "%s"
        }
      }
    },
    {
      "Principal": "*",
      "Action": "s3:*",
      "Effect": "Deny",
      "Resource": "*",
      "Condition": {
        "StringNotEquals": {
          "aws:SourceVpc": "%s"
        }
      }
    }
  ]
}`, bucketName, bucketName, vpcId, vpcId)
}


//
//
// @Parameters
//
//
// @Returns
//
//
func VpcSsmEndpointPolicyGen(accountId string, region string,
                             vpcId string) string {
    return fmt.Sprintf(`{
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::%s:root"
      },
      "Action": [
        "ssm:GetParameter",
        "ssm:GetParameters",
        "ssm:GetParametersByPath",
        "ssm:PutParameter",
        "ssm:DeleteParameter"
      ],
      "Resource": "arn:aws:ssm:%s:%s:parameter/kloud-kraken/*",
      "Condition": {
        "StringEquals": {
          "aws:SourceVpc": "%s"
        }
      }
    }
  ]
}`, accountId, region, accountId, vpcId)
}
