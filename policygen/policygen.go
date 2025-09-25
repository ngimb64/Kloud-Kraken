package main

import (
	"fmt"
	"log"
	"os"
)

// Generates bootstrap permissions policy JSON for initial setup and teardown.
//
// @Parameters
//  - accountId:  AWS account id (e.g. "123456789012")
//  - bucketName:  S3 bucket name for kloud-kraken (e.g. "kloud-kraken-s3")
//  - region:  AWS region (e.g. "us-east-1")
//  - ssmPrefix:  SSM parameter prefix (e.g. "/kloud-kraken/")
//  - logGroupPrefix:  CloudWatch Logs prefix (e.g. "/kloud-kraken")
//  - roleNamePrefix:  IAM role name prefix allowed for bootstrap to
// 					   create (e.g. "kloud-kraken-")
//
// @Returns
//  - The generated permissions policy with args formatted into it
//
func bootstrapPolicyGen(accountId string, bucketName string,
                        region string, ssmPrefix string,
                        logGroupPrefix string,
                        roleNamePrefix string) string {
    return fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AssumeServerRole",
      "Effect": "Allow",
      "Action": ["sts:AssumeRole"],
      "Resource": "arn:aws:iam::%s:role/KloudKrakenBootstrapRole"
    },

    {
      "Sid": "ManageS3KloudKrakenBucket",
      "Effect": "Allow",
      "Action": [
        "s3:CreateBucket",
        "s3:PutBucketTagging",
        "s3:PutObject",
        "s3:PutObjectAcl",
        "s3:GetObject",
        "s3:ListBucket",
        "s3:ListBucketMultipartUploads",
        "s3:DeleteObject",
        "s3:DeleteObjectVersion",
        "s3:DeleteBucket"
      ],
      "Resource": [
        "arn:aws:s3:::%s",
        "arn:aws:s3:::%s/*"
      ]
    },

    {
      "Sid": "ManageSsmKloudKrakenParameters",
      "Effect": "Allow",
      "Action": [
        "ssm:PutParameter",
        "ssm:GetParameter",
        "ssm:GetParameters",
        "ssm:GetParametersByPath",
        "ssm:DeleteParameter",
        "ssm:AddTagsToResource",
        "ssm:DescribeParameters"
      ],
      "Resource": "arn:aws:ssm:%s:%s:parameter%s*"
    },

    {
      "Sid": "ManageCloudWatchLogsForFlowAndBootstrap",
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents",
        "logs:DescribeLogStreams",
        "logs:PutRetentionPolicy",
        "logs:TagResource",
        "logs:DeleteLogStream",
        "logs:DeleteLogGroup"
      ],
      "Resource": "arn:aws:logs:%s:%s:log-group:%s*"
    },

    {
      "Sid": "EC2NetworkingAndVpcLifecycle",
      "Effect": "Allow",
      "Action": [
        "ec2:CreateVpc",
        "ec2:DescribeVpcs",
        "ec2:CreateTags",
        "ec2:DeleteVpc",
        "ec2:CreateSubnet",
        "ec2:DescribeSubnets",
        "ec2:DeleteSubnet",
        "ec2:ModifySubnetAttribute",
        "ec2:CreateInternetGateway",
        "ec2:AttachInternetGateway",
        "ec2:DescribeInternetGateways",
        "ec2:DetachInternetGateway",
        "ec2:DeleteInternetGateway",
        "ec2:CreateNatGateway",
        "ec2:DescribeNatGateways",
        "ec2:DeleteNatGateway",
        "ec2:AllocateAddress",
        "ec2:DescribeAddresses",
        "ec2:ReleaseAddress",
        "ec2:CreateRouteTable",
        "ec2:DescribeRouteTables",
        "ec2:DeleteRouteTable",
        "ec2:CreateRoute",
        "ec2:AssociateRouteTable",
        "ec2:DisassociateRouteTable",
        "ec2:CreateSecurityGroup",
        "ec2:DescribeSecurityGroups",
        "ec2:DeleteSecurityGroup",
        "ec2:AuthorizeSecurityGroupIngress",
        "ec2:AuthorizeSecurityGroupEgress",
        "ec2:RevokeSecurityGroupIngress",
        "ec2:RevokeSecurityGroupEgress",
        "ec2:CreateVpcEndpoint",
        "ec2:DescribeVpcEndpoints",
        "ec2:DeleteVpcEndpoints",
        "ec2:CreateFlowLogs",
        "ec2:DeleteFlowLogs",
        "ec2:DescribeAvailabilityZones",
        "ec2:RunInstances",
        "ec2:TerminateInstances",
        "ec2:DescribeInstances"
      ],
      "Resource": "*",
      "Condition": {
        "StringEquals": {
          "ec2:ResourceTag/kloud-kraken": "true"
        }
      }
    },

    {
      "Sid": "EC2HelperForS3ListAndMultipart",
      "Effect": "Allow",
      "Action": [
        "s3:ListBucket"
      ],
      "Resource": "arn:aws:s3:::%s"
    },

    {
      "Sid": "ManageIamRolesAndInstanceProfiles",
      "Effect": "Allow",
      "Action": [
        "iam:CreateRole",
        "iam:DeleteRole",
        "iam:PutRolePolicy",
        "iam:DeleteRolePolicy",
        "iam:GetRole",
        "iam:CreateInstanceProfile",
        "iam:DeleteInstanceProfile",
        "iam:AddRoleToInstanceProfile",
        "iam:RemoveRoleFromInstanceProfile",
        "iam:ListInstanceProfilesForRole",
        "iam:ListRolePolicies",
        "iam:ListAttachedRolePolicies",
        "iam:AttachRolePolicy",
        "iam:DetachRolePolicy",
        "iam:PassRole"
      ],
      "Resource": [
        "arn:aws:iam::%s:role/%s*",
        "arn:aws:iam::%s:instance-profile/%s*"
      ]
    },

    {
      "Sid": "UtilityAndReadOnly",
      "Effect": "Allow",
      "Action": [
        "sts:GetCallerIdentity",
        "ec2:DescribeTags"
      ],
      "Resource": "*"
    }
  ]
}`, accountId, bucketName, bucketName,
    region, accountId, ssmPrefix, region,
    accountId, logGroupPrefix, bucketName,
    accountId, roleNamePrefix, accountId,
    roleNamePrefix)
}


// Generates the bootstrap role permissions policy, writes it to JSON file,
// and notifies the user when the process is complete.
//
func main() {
    accountId := os.Args[1]
    region := os.Args[2]

    if accountId == "" || region == "" {
        log.Fatal("Missing required args, usage example:  " +
                  "\n\t./bin/policygen <account_id> <region>")
    }

    // Generate the bootstrap permissions policy
    bootPol := bootstrapPolicyGen(accountId, "kloud-kraken-s3", region,
                                  "/kloud-kraken/tls-cert", "/kloud-kraken",
                                  "KloudKrakenBootstrapRole")

    // Write the generated permissions policy to output file
    err := os.WriteFile("./../policygen/policy-out.json",
                        []byte(bootPol), os.ModePerm)
    if err != nil {
        log.Fatalf("Error writing policy to file:  %v", err)
    }

    fmt.Println("[!] Bootstrap permissions policy generated ..")
}
