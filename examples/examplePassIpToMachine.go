package main

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go/aws/session"
)

// GetPublicIP fetches the public IP of the executing machine by querying ipify.org
func getPublicIP() (string, error) {
	response, err := http.Get("https://api.ipify.org")
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	ip, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	return string(ip), nil
}


// Function to create an EC2 instance with user data that includes the public IP
func createEC2Instance(publicIP string) (*ec2.Instance, error) {
	// Initialize a new AWS session
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-west-2")}, // Replace with your desired AWS region
	)
	if err != nil {
		return nil, err
	}

	// Create an EC2 service client
	svc := ec2.New(sess)

	// User data script to pass the public IP to the EC2 instance
	userData := fmt.Sprintf(`#!/bin/bash
echo "Public IP of the local machine: %s" > /opt/provisioning/ip_address.txt
/opt/provisioning/Client --ip %s
`, publicIP, publicIP)

	// Run the EC2 instance with user data
	runResult, err := svc.RunInstances(&ec2.RunInstancesInput{
		ImageId:      aws.String("ami-0abcdef1234567890"), // Replace with your AMI ID
		InstanceType: aws.String("t2.micro"),              // Adjust instance type as needed
		MinCount:     aws.Int64(1),
		MaxCount:     aws.Int64(1),
		UserData:     aws.String(userData),
	})
	if err != nil {
		return nil, err
	}

	// Return the created instance
	return runResult.Instances[0], nil
}


func test() {
	// Get the public IP of the local machine
	publicIP, err := getPublicIP()
	if err != nil {
		log.Fatal(err)
	}

	// Print the public IP (You will pass it to the EC2 instance)
	fmt.Println("Public IP:", publicIP)

	// Create the EC2 instance and pass the public IP in the user data
	instance, err := createEC2Instance(publicIP)
	if err != nil {
		log.Fatalf("Failed to create EC2 instance: %v", err)
	}

	fmt.Printf("Created EC2 instance with ID: %s\n", *instance.InstanceId)
}
