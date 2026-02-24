<div align="center" style="font-family: monospace">

# Kloud Kraken

> Cost-optimized, cloud hash cracking — without the AWS headache.

> Kloud Kraken automates secure AWS infrastructure, high-performance data transfer, and wordlist management so Hashcat never sits idle and you never pay for resources you don’t need.

![alt text](https://github.com/ngimb64/Kloud-Kraken/blob/main/docs/images/KloudKrakenTextLogo.jpeg?raw=true)
![alt text](https://github.com/ngimb64/Kloud-Kraken/blob/main/docs/images/KloudKrakenLogo.jpeg?raw=true)
</div>
<br>


## Why Kloud Kraken

Fast & efficient data flow — TLS-encrypted, direct transfers to EC2 instance-store with concurrent file streams to keep Hashcat working nonstop

Minimal AWS cost & cleanup — isolated VPC, VPC endpoints, least-privilege IAM, and automatic teardown when cracking is complete

Zero-friction setup — automated scripts, YAML configs, built-in wordlist merging, deduplication, and intelligent file splitting
<br>


## Table of Contents

- [Features](#Features)
- [AWS Services Featured](#AWS-Services-Featured)
- [Flowcharts](#Flowcharts)
- [Installation](#Installation)
- [Usage](#Usage)
- [Instance Types](#Instance-Types)
- [Contributing or Issues](#Contributing-or-Issues)
- [License](#License)
<br>


## Features

- Easy setup with automated script
- Easy configuration with YAML templates
- Built-in wordlist merging with flexibility to skip files that exceed specified size
    - Merging process using `cat` -> `deduplicut` until within percentage range of max file size (15% by default)
    - If the file goes over max file size, excess data is shaved with `cut` into a new file
<br>

- Custom TLS based file transfer service using SSM Parameter Store to transfer certificates
    - Service continually transfers data requested by clients based on allowed max file size
    - Server continues transferring until the load directory has been completely processed
    - Client continues requesting data based on available disk space until instance store is full then sleeps until more space is available
    - By using this process Kloud Kraken can handle as much data as desired regardless of available storage on instance store
    - Files are transferred directly to the local EC2 instance-store
    - Facilitates multiple file transfers per EC2 client simultaneously
<br>

- Designed to setup isolated VPC in AWS environment
    - Features public subnet setup with Internet Gateway for EC2 internet access
    - VPC Endpoints for S3 bucket & SSM Parameter Store operations
    - Security groups for ensuring only outbound traffic occurs on EC2
    - Minimalist IAM role utilization featuring bootstrap role for creating and destroying AWS resources
    - Automatically assumes role for server operations with the Security Token Service
    - Client IAM role is created with associated instance profile
<br>

- Cleans up AWS resources that incur cost over time when processing is complete
- Features internal state file for tracking resources for intelligent creation if they do not already exist and a full teardown program that destroys all created resources
- CLI features colorized TUI interface
- Custom logging system with CloudWatch and local backup
<br>


## AWS Services Featured

- CloudWatch
- EC2
- IAM
- S3 Buckets
- Security Token Service
- SSM Parameter Store
<br>


## Demonstration Output

####  AWS Setup
![alt text](https://github.com/ngimb64/Kloud-Kraken/blob/main/docs/images/aws_setup.png?raw=true)
<br><br>

#### Main Operation
![alt text](https://github.com/ngimb64/Kloud-Kraken/blob/main/docs/images/main_operation.png?raw=true)
<br><br>

#### Cleanup and Results
![alt text](https://github.com/ngimb64/Kloud-Kraken/blob/main/docs/images/cleanup_results.png?raw=true)
<br><br>


## Installation

- Download the project
    - `git clone https://github.com/ngimb64/Kloud-Kraken.git`

- Run the installation script
    - `./setup.sh`

### Cloud Setup

- Start by ensuring an AWS account is created and log in as the root user
- In the search bar, search `budgets` which will find the budgets feature in "Billing and Cost Management"
- Create a budget an set a monetary limit based on the intended budget
- Run the policy generator program to generate policy for bootstrap role
    - `./bin/policygen <account_id>`
- Search `iam` to access the IAM services, go to user groups sectionon the left and create a user group with no permissions
	- Click on the created user group, go to the permissions, click add permissions and select "create inline policy"
	- Switch from visual to JSON for policy editor and paste the permissions policy previously generated on command line
- Create a user and assign them to the created user group with IAM permissions
- Generate and store access keys for the newly created user in their security credentials tab
	- Select the "create access key" option in the tab and ensure to select "Command Line Interface" as the use case
	- Configure API access credentials locally before running with `aws configure --profile kloud-kraken`
    - It is critical to set the credentials under the kloud-kraken profile as the program searches for that specific one when loading the AWS config
- By default, 0 vCPUs are allowed for for G and P-series EC2 instances meaning service a quota request must be made for on-demand EC2 G-series based on the number of desired vCPUs to use (add them up if using multiple instances)
    - Search `service quota` to access the service for making requests
    - Keep in mind if your account does not have extensive history the request will be automatically denied initially
        - It is better to start with a single instance and gradually ramp up quota over a few billing cycles, they will limit excessive requests anyway
        - After it is denied explain the purpose of using Kloud Kraken so they can confirm you are legit and not intending to abuse the GPU instances for things like crypto mining, feel free to provide them with a link to the projects GitHub page
        - While writing the information in the message area is a good idea, they **must** be called to get the request process going
    - The service quota request **needs** to be for the default us-east-1 region region as that is the only region the pricing API supports for cost calculation, the tool will likely work in other regions but expect cost calculation to fail
    - Supported instance families can be found at [Instance Types](#Instance-Types)
    - AWS Doc on recommended GPU instances - https://docs.aws.amazon.com/dlami/latest/devguide/gpu.html
    - AWS Doc on setting EC2 service quotas - https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-resource-limits.html

### Local Setup

- Before running the program it is also very helpful to prepare wordlist data ahead of time
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
- For supported instance families [Instance Types](#Instance-Types)
- Despite the tool not supporting Hashcat combinator mode (1), it can be easily achieved locally and combined with other wordlist data using the usual straight mode (0)
  - `hashcat --stdout -a 1 <left_wordlist> <right_wordlist> > combinator_out.txt`

Run the project:
```
./bin/kloud-kraken-server ./config/<yaml_config>
```

**Note**:  If an error occurs during the AWS environment setup about STS credential cache failing to reset, it is not an error to worry about and the program simply needs to be rerun to refresh it. I tried adding additional code to do this but the issue seems to be on the AWS end of things and out of my control to fix at this point in time. If the program fails to connect, exits early, and the log mentions not being able to find the parameter; this issue is caused by SSM Parameter Store VPC Endpoints and is fixed by running the teardown program & rerun to build the AWS resources again.

If at any point the project needs to be rebuilt:
```
make clean && make all
```

To delete the project from AWS environment:
```
./bin/kloud-kraken-teardown
```
<br>


## Instance Types

**Note**: Pricing can be found in the Instance Types tab in the Instances subsection of EC2 service (search g4, g5, etc.)

- g4dn.*
- g5.*
- g6.*
- g6e.*
- gr6.*
- p3.*
- p3dn.*
- p4d.*
- p4de.*
- p5.*
- p5en.*
- p6-b200.*

My personal recommendation for most powerful cost effective selection is to use multiple instances of an affordable type like g4dn.xlarge and let Kloud Kraken optimize by distributing data among multiple EC2 instances. P-series are incredible machines, but they also can be very **EXPENSIVE**. Keep in mind even if the machine is only used 5 minutes a full hour rate will still be charged. The instance type selection really depends on the amount of data as the P-series are intended for processing insane amounts of data for high power computing. Even if the GPUs perform better the cost of G-series is **substantially** less even with multiple instances which combined can achieve similar results than one very expensive instance.
<br>


## Flowcharts

#### Local Server
![alt text](https://github.com/ngimb64/Kloud-Kraken/blob/main/docs/flowcharts/Local-Server.svg?raw=true)
<br><br>

#### AWS Setup
![alt text](https://github.com/ngimb64/Kloud-Kraken/blob/main/docs/flowcharts/AWS-Setup.svg?raw=true)
<br><br>

#### Client
![alt text](https://github.com/ngimb64/Kloud-Kraken/blob/main/docs/flowcharts/Client.svg?raw=true)
<br><br>


## Contributing or Issues

[Contributing Documentation](CONTRIBUTING.md)
<br>


## License

The program is licensed under [PolyForm Noncommercial License 1.0.0](LICENSE.md)
<br>
