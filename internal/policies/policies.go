package policies

import "fmt"

// Generates permissions policy for the client EC2.
//
// @Parameters
//  - region:  The AWS region where actions will be performed
//  - accountId:  The AWS account ID where actions will be performed
//
// @Returns
//  - The generated permissions policy with args formatted into it
//
func ClientPermPolicyGen(region string, accountId string) string {
    return fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "S3DownloadBinary",
      "Effect": "Allow",
      "Action": [
        "s3:GetObject"
      ],
      "Resource": "arn:aws:s3:::kloud-kraken-s3/*"
    },

    {
      "Sid": "SSMFetchParameters",
      "Effect": "Allow",
      "Action": [
        "ssm:GetParameter",
        "ssm:GetParameters",
        "ssm:GetParametersByPath"
      ],
      "Resource": "arn:aws:ssm:%s:%s:parameter/kloud-kraken/tls-cert*"
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
        "logs:TagLogGroup",
        "logs:TagResource"
      ],
      "Resource": "arn:aws:logs:%s:%s:log-group:/kloud-kraken*"
    },

    {
      "Sid": "ManageSecurityGroupEgress",
      "Effect": "Allow",
      "Action": [
        "ec2:AuthorizeSecurityGroupEgress",
        "ec2:RevokeSecurityGroupEgress",
        "ec2:DescribeSecurityGroups"
      ],
      "Resource": "arn:aws:ec2:%s:%s:security-group/*"
    }
  ]
}`, region, accountId, region, accountId, region, accountId)
}


// Generates trust policy for the client.
//
// @Returns
//  - The generated trust policy
//
func ClientTrustPolicyGen() string {
    return `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect":    "Allow",
      "Principal": { "Service": "ec2.amazonaws.com" },
      "Action":    "sts:AssumeRole"
    }
  ]
}`
}


// Generate permissions policy for the local server.
//
// @Parameters
//  - region:  The AWS region where actions will be performed
//  - accountId:  The AWS account ID where actions will be performed
//
// @Returns
//  - The generated permissions policy with args formatted into it
//
func ServerPermPolicyGen(region string, accountId string) string {
    return fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "SSMUploadClientCert",
      "Effect": "Allow",
      "Action": [
        "ssm:PutParameter",
        "ssm:DeleteParameter",
        "ssm:AddTagsToResource",
      ],
      "Resource": [
        "arn:aws:ssm:%s:%s:parameter/kloud-kraken/tls-cert*",
      ]
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
        "arn:aws:s3:::kloud-kraken-s3",
        "arn:aws:s3:::kloud-kraken-s3/*"
      ]
    },

    {
      "Sid": "EC2LifecycleControl",
      "Effect": "Allow",
      "Action": [
        "ec2:RunInstances",
        "ec2:TerminateInstances",
        "ec2:DescribeInstances",
        "ec2:DescribeInstanceStatus",
        "ec2:DescribeImages",
        "ec2:CreateTags",
        "ec2:DeleteVpcEndpoints",
        "ec2:DescribeVpcEndpoints"
      ],
      "Resource": "*"
    },

    {
      "Sid": "PricingGetProducts",
      "Effect": "Allow",
      "Action": [
        "pricing:GetProducts"
      ],
      "Resource": "*"
    },

    {
      "Sid": "EC2PassRoleForInstanceProfile",
      "Effect": "Allow",
      "Action": [
        "iam:PassRole"
      ],
      "Resource": "arn:aws:iam::%s:role/KloudKrakenClientRole"
    }
  ]
}`, region, accountId, accountId)
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
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::%s:user/%s"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}`, accountId, iamUser)
}


// Generate permissions policy for VPC Flow Logs.
//
// @Parameters
//  - region:  The AWS region where the VPC Flow Logs are utilized
//  - accountId:  The AWS account ID number
//
// @Returns
//  - The generated permissions policy with args formatted into it
//
func VpcFlowLogsPermPolicyGen(region string, accountId string) string {
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
        "arn:aws:logs:%s:%s:log-group:kloud-kraken-vpc-flow-logs",
        "arn:aws:logs:%s:%s:log-group:kloud-kraken-vpc-flow-logs:*",
        "arn:aws:logs:%s:%s:log-group:/aws/vpc-flow-logs/kloud-kraken-vpc-flow-logs",
        "arn:aws:logs:%s:%s:log-group:/aws/vpc-flow-logs/kloud-kraken-vpc-flow-logs:*"
      ]
    }
  ]
}`, region, accountId, region, accountId, region, accountId, region, accountId)
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
//  - accountId:  The ID of the account to format in the policy
// @Returns
//  - The generated permissions policy
//
func VpcS3EndpointPolicyGen(accountId string) string {
    return fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowKKUserToKKBucketOnly",
      "Effect": "Allow",
      "Principal": "*",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::kloud-kraken-s3",
        "arn:aws:s3:::kloud-kraken-s3/*"
      ],
      "Condition": {
        "StringLike": {
          "aws:PrincipalArn": "arn:aws:sts::%s:assumed-role/KloudKrakenClientRole/*"
        }
      }
    }
  ]
}`, accountId)
}


// Generate permissions policy for the SSM VPC Endpoint.
//
// @Parameters
//  - accountId:  The AWS account ID number
//
// @Returns
//  - The generated permissions policy with args formatted into it
//
func VpcSsmEndpointPolicyGen(accountId string) string {
    return fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowKKUserSSMParams",
      "Effect": "Allow",
      "Principal": "*",
      "Action": [
        "ssm:GetParameter",
        "ssm:GetParameters",
        "ssm:GetParametersByPath",
        "ssm:PutParameter",
        "ssm:DeleteParameter"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "aws:PrincipalAccount": "%s"
        }
      }
    }
  ]
}`, accountId)
}
