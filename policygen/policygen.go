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
      "Sid": "STSAssumeServerRole",
      "Effect": "Allow",
      "Action": [
        "sts:AssumeRole"
      ],
      "Resource": "arn:aws:iam::%s:role/KloudKrakenServerRole"
    },

    {
      "Sid": "STSGetCallerId",
      "Effect": "Allow",
      "Action": [
        "sts:GetCallerIdentity"
      ],
      "Resource": "*"
    },

    {
      "Sid": "ManageS3KloudKrakenBucket",
      "Effect": "Allow",
      "Action": [
        "s3:CreateBucket",
        "s3:DeleteBucket",
        "s3:DeleteObject",
        "s3:ListBucket",
        "s3:PutBucketTagging"
      ],
      "Resource": [
        "arn:aws:s3:::kloud-kraken-s3",
        "arn:aws:s3:::kloud-kraken-s3/*"
      ]
    },

    {
      "Sid": "ManageCloudWatchLogsForFlowAndBootstrap",
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:DeleteLogGroup",
        "logs:DeleteLogStream",
        "logs:PutLogEvents",
        "logs:PutRetentionPolicy",
        "logs:TagResource"
      ],
      "Resource": "arn:aws:logs:%s:%s:log-group:kloud-kraken*"
    },

    {
      "Sid": "CloudWatchLoggingDescribe",
      "Effect": "Allow",
      "Action": [
        "logs:DescribeLogGroups",
        "logs:DescribeLogStreams"
      ],
      "Resource": "*"
    },

    {
      "Sid": "EC2NetworkingAndVpcLifecycle",
      "Effect": "Allow",
      "Action": [
        "ec2:AssociateRouteTable",
        "ec2:AttachInternetGateway",
        "ec2:AuthorizeSecurityGroupEgress",
        "ec2:AuthorizeSecurityGroupIngress",
        "ec2:CreateFlowLogs",
        "ec2:CreateInternetGateway",
        "ec2:CreateRoute",
        "ec2:CreateRouteTable",
        "ec2:CreateSecurityGroup",
        "ec2:CreateSubnet",
        "ec2:CreateTags",
        "ec2:CreateVpc",
        "ec2:CreateVpcEndpoint",
        "ec2:DescribeAvailabilityZones",
        "ec2:DeleteFlowLogs",
        "ec2:DeleteInternetGateway",
        "ec2:DeleteRouteTable",
        "ec2:DeleteSecurityGroup",
        "ec2:DeleteSubnet",
        "ec2:DeleteVpc",
        "ec2:DeleteVpcEndpoints",
        "ec2:DescribeFlowLogs",
        "ec2:DescribeInternetGateways",
        "ec2:DescribeInstances",
        "ec2:DescribeRouteTables",
        "ec2:DescribeSecurityGroups",
        "ec2:DescribeSubnets",
        "ec2:DescribeTags",
        "ec2:DescribeVpcs",
        "ec2:DescribeVpcEndpoints",
        "ec2:DetachInternetGateway",
        "ec2:DisassociateRouteTable",
        "ec2:ModifySubnetAttribute",
        "ec2:ModifyVpcAttribute",
        "ec2:RevokeSecurityGroupEgress",
        "ec2:RevokeSecurityGroupIngress",
        "ec2:RunInstances",
        "ec2:TerminateInstances"
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
      "Sid": "ManageIamRolesAndInstanceProfiles",
      "Effect": "Allow",
      "Action": [
        "iam:AddRoleToInstanceProfile",
        "iam:AttachRolePolicy",
        "iam:CreateInstanceProfile",
        "iam:CreateRole",
        "iam:DeleteInstanceProfile",
        "iam:DeleteRole",
        "iam:DeleteRolePolicy",
        "iam:DetachRolePolicy",
        "iam:GetRole",
        "iam:ListAttachedRolePolicies",
        "iam:ListInstanceProfilesForRole",
        "iam:ListRolePolicies",
        "iam:PassRole",
        "iam:PutRolePolicy",
        "iam:RemoveRoleFromInstanceProfile",
        "iam:TagInstanceProfile",
        "iam:TagRole"
      ],
      "Resource": [
        "arn:aws:iam::%s:role/KloudKrakenClientRole",
        "arn:aws:iam::%s:role/KloudKrakenServerRole",
        "arn:aws:iam::%s:role/KloudKrakenVpcFlowLogsRole",
        "arn:aws:iam::%s:instance-profile/KloudKrakenClientRole"
      ]
    }
  ]
}`, accountId, region, accountId, accountId,
    accountId, accountId, accountId)
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
