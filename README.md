<div align="center" style="font-family: monospace">

# Kloud Kraken

**NOTE:** This project is still a work in progress but at least 99% finished so expect it to be completed in the next few weeks

> A cloud based hash cracking machine that supports distributed workloads among multiple EC2 instances utilizing a built-in TLS protected file transfer service that supports multiple transfers per node simultaneously

![alt text](https://github.com/ngimb64/Kloud-Kraken/blob/main/images/KloudKrakenTextLogo.jpeg?raw=true)
![alt text](https://github.com/ngimb64/Kloud-Kraken/blob/main/images/KloudKrakenLogo.jpeg?raw=true)
</div>
<br>

## Table of Contents

- [Features](#Features)
- [Installation](#Installation)
- [Usage](#Usage)
- [Contributing or Issues](#Contributing-or-Issues)
- [License](#License)
<br>

## Features

- Easy setup with automated script
- Easy configuration with YAML templates
- Supports hash cracking distributed workloads among multiple EC2 instances
- Built-in wordlist merging with flexibility to skip larger files
  - Merging process using `cat` -> `deduplicut`
  - If the file goes over max file size, excess data is shaved with `cut` into a new file
<br>

- Custom TLS based file transfer service using SSM Parameter Store to transfer certificates
  - Service continually transfers data requested by clients based on allowed max file size
    - This process continues until the load directory has been completely processed
  - Files are transferred directly to the local EC2 instance-store
  - Facilitates multiple file transfers per EC2 client simultaneously
<br>

- Designed to setup isolated VPC in AWS environment
  - Features public subnet setup Internet Gateway for EC2 internet access
  - VPC Endpoints for S3 bucket & SSM Parameter Store operations
  - Security groups for ensuring only outbound traffic occurs on EC2
  - Minimalist IAM role utilization featuring bootstrap role for creating and destroying AWS resources
  - Automatically assumes role for server operations with the Security Token Service
  - Client IAM role is created with associated instance profile
<br>

- EC2 clients utilize multiple NVMe drives combined in a RAID 0 configuration for optimized performance of disk operations
- CLI features colorized TUI interface
- Custom logging system with CloudWatch and local backup
- Cleans up AWS resources that incur cost over time when processing is complete
<br>


## AWS Services Featured

- CloudWatch
- EC2
- IAM
- S3 Buckets
- Security Token Service
- SSM Parameter Store
<br>

## Installation

- Begin by downloading the project with `git clone https://github.com/ngimb64/Kloud-Kraken.git`

- Run the installation script
	- `./setup.sh`

### Cloud Setup

- Start by ensuring an AWS account is created and log in as the root user
- In the search bar, search `budgets` which will find the budgets feature in "Billing and Cost Management"
- Create a budget an set a monetary limit based on the intended budget
- Run the policy generator program to generate policy for bootstrap role
  - `./bin/policygen <account_id> <region>`
- Search `iam` to access the IAM services, create a user group with the permissions policy just generated in the policy editor
- Create a user and assign them to the created user group with IAM permissions
- Generate access keys for the newly created user

### Local Setup

- When running the program in full mode with AWS environment there are two options for credential setup
    - Configure API access credentials locally before running with `aws configure` (preferred)
    - OR set the environment variables  AWS_ACCESS_KEY & AWS_SECRET_KEY
    	- Keep in mind these will be stored in command line history unless configuration is done prior
<br>

- Before running the program it is also incredibly important to prepare wordlist data ahead of time
    - Smaller wordlists easily merge but larger ones slow the process down **substantially**
    - In the YAML config it is best to set a reasonable `max_merging_size` (ex: 400MB) to prevent bottlenecks from merging large wordlists
    - It is also ideal to set a reasonable `max_file_size` (ex: 2GB) to prevent extensive delays in network latency as smaller files transfer quicker and distribute better among EC2 clients
    - The following example splits Crackstation's 15GB wordlist into 400MB files:
      `split -C 400M -d --additional-suffix=.txt crackstation.txt ./crack_station_`
    - It is also important the `max_size_range` is at a decent percent as lower percentage will results feeding the same wordlist into the merging process until it is within that range or meets the `max_merging_size`
<br>


## Usage

- Make a copy of the default `config.yml` file in the config folder
- Ensure there is wordlist data in the `load_dir`, a `hash_file_path` for the hash file to crack, and any other needed components specified in your copy of `config.yml`
- Ensure to use `instructions.yml` as a reference when configuring the recently made copy
- For supported regions [Regions](#Regions) and instance families [Instances](#Instances)
- Despite the tool not supporting Hashcat combinator mode (1), it can be easily achieved locally and combined with other wordlist data using the usual straight mode (0)
  - `hashcat --stdout -a 1 <left_wordlist> <right_wordlist> > combinator_out.txt`

Run the project:
```
./bin/kloud-kraken-server ./config/<yaml_config>
```

If at any point the project needs to be rebuilt:
```
make clean && make all
```

To delete the project from AWS environment:
```
./bin/kloud-kraken-teardown
```
<br>


## Regions

// US
"us-east-1", "us-east-2", "us-west-1", "us-west-2"

// Canada
"ca-central-1"

// South America
"sa-east-1"

// Europe
"eu-central-1", "eu-west-1", "eu-west-2", "eu-west-3", "eu-north-1", "eu-south-1"

// Middle East / Africa
"me-south-1", "af-south-1"

// Asia Pacific
"ap-northeast-1", "ap-northeast-2", "ap-northeast-3", "ap-southeast-1", "ap-southeast-2", "ap-south-1"

// China
"cn-north-1", "cn-northwest-1"

// GovCloud
"us-gov-west-1, "us-gov-east-1"
<br>


## Instances

**Note**: Pricing can be found in the Instance Types tab in the Instances subsection of EC2 service (search g4, g5, g6)

- g4ad.*
- g4dn.*
- g5.*
- g5g.*
- g6.*
- g6e.*
- g6f.*
<br>


## Contributing or Issues

[Contributing Documentation](CONTRIBUTING.md)
<br>


## License

The program is licensed under [PolyForm Noncommercial License 1.0.0](LICENSE.md)
<br>
