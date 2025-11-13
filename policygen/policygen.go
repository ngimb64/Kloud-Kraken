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
//  - region:  AWS region (e.g. "us-east-1")
//
// @Returns
//  - The generated permissions policy with args formatted into it
//
func bootstrapPolicyGen(accountId string, region string) string {
    return fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AssumeServerRole",
      "Effect": "Allow",
      "Action": ["sts:AssumeRole"],
      "Resource": "arn:aws:iam::%s:role/KloudKrakenServerRole"
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
        "arn:aws:s3:::kloud-kraken-s3",
        "arn:aws:s3:::kloud-kraken-s3/*"
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
      "Resource": "arn:aws:ssm:%s:%s:parameter/kloud-kraken/tls-cert*"
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
      "Resource": "arn:aws:logs:%s:%s:log-group:kloud-kraken*"
    },

    {
      "Sid": "EC2NetworkingAndVpcLifecycle",
      "Effect": "Allow",
      "Action": [
        "ec2:CreateVpc",
        "ec2:ModifyVpcAttribute",
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
        "ec2:DescribeFlowLogs",
        "ec2:DescribeAvailabilityZones",
        "ec2:RunInstances",
        "ec2:TerminateInstances",
        "ec2:DescribeInstances"
      ],
      "Resource": "*"
    },

    {
      "Sid": "EC2HelperForS3ListAndMultipart",
      "Effect": "Allow",
      "Action": [
        "s3:ListBucket"
      ],
      "Resource": "arn:aws:s3:::kloud-kraken-s3"
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
        "iam:TagInstanceProfile",
        "iam:TagRole",
        "iam:PassRole"
      ],
      "Resource": [
        "arn:aws:iam::%s:role/KloudKrakenClientRole",
        "arn:aws:iam::%s:role/KloudKrakenServerRole",
        "arn:aws:iam::%s:role/KloudKrakenVpcFlowLogsRole",
        "arn:aws:iam::%s:instance-profile/KloudKrakenClientRole"
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
}`, accountId, region, accountId, region, accountId,
    accountId, accountId, accountId, accountId)
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
    bootPol := bootstrapPolicyGen(accountId, region)

    // Write the generated permissions policy to output file
    err := os.WriteFile("policy-out.json", []byte(bootPol), os.ModePerm)
    if err != nil {
        log.Fatalf("Error writing policy to file:  %v", err)
    }

    fmt.Println("[!] Bootstrap permissions policy generated ..")
}
