<div align="center" style="font-family: monospace">

# Kloud Kraken

> A cloud based hash cracking beast that supports distributed workloads among multiple EC2 instances utilizing a built-in TLS protected file transfer service that supports multiple transfers per node simultaneously

![alt text](https://github.com/ngimb64/Kloud-Kraken/blob/main/images/KloudKrakenTextLogo.jpeg?raw=true)
![alt text](https://github.com/ngimb64/Kloud-Kraken/blob/main/images/KloudKrakenLogo.jpeg?raw=true)
<br>
</div>


## Table of Contents

- [Features](#Features)
- [Installation](#Installation)
- [Usage](#Usage)
- [Contributing or Issues](#Contributing-or-Issues)
- [License](#License)


## Features

- Easy configuration with YAML templates
- Built-in wordlist merging with flexibility to skip larger files
  - Merging process using `cat` -> `deduplicut`
  - If the file goes over max file size, excess data is shaved with `cut` or `dd` depending on its size
- Custom TLS based file transfer service using SSM Parameter Store to transfer certificates
  - Service continually transfers data requested by clients based on allowed max file size until the load directory has been completely processed
  - Files are transfered directly to the local EC2 instance-store which features multiple drives combined in a RAID 0 configuration for performance
- Supports hash cracking distributed workloads among multiple EC2
- CLI features colorized TUI interface
<br>


## AWS Services Featured

- CloudWatch
- EC2
- IAM
- S3 Buckets
- SSM Parameter Store
<br>

## Installation

- Begin by downloading the project with `git clone https://github.com/ngimb64/Kloud-Kraken.git`

### Cloud Setup

- Start by ensuring an AWS account is created and log in as the root user
- In the search bar, search "budgets" which will find the budgets feature in "Billing and Cost Management"
- Create a budget an set a monetary limit based on the intended budget
- Search IAM to access the IAM services, create a user with the following permission in the policy editor
```
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "iam:CreateRole",
        "iam:GetRole",
        "iam:PutRolePolicy",
        "iam:CreateInstanceProfile",
        "iam:AddRoleToInstanceProfile",
        "sts:AssumeRole"
      ],
      "Resource": "*"
    }
  ]
}
```
- Create a user and assign them to the created user group with IAM permissions
- Generate access keys for the newly created user
- Remain logged into account as information will be needed when filling out the YAML configuration file

### Local Setup

- This project uses duplicut for de-duplicating wordlists
    - Ensure the binary has executable permissions with `ls -la duplicut`
        - If not set them with `chmod +x duplicut/duplicut`
<br>

- Ensure Go is installed `sudo apt install -y golang`
    - Add these to shell rc file (usually .zshrc or .bashrc, echo $SHELL to find out)
        ```
        export GOROOT=/usr/lib/go
        export GOPATH=$HOME/go
        export PATH=$GOPATH/bin:$GOROOT/bin:$PATH
        ```
    - Reload rc file `source ~/.zshrc` (zsh example)
<br>

- Install Go packages with `go get ./...`
- Ensure any missing external dependencies are resolved `go mod tidy -e`
- Run the test cases in root directory of project `go test ./...`
<br>

- When running the program in full mode with AWS environment there are two options for credential setup
    - Configure API access credentials locally before running with `aws configure`
    - OR set the environment variables  AWS_ACCESS_KEY & AWS_SECRET_KEY
<br>


## Usage

- Make a copy of the `config.yml` file in the config folder to
- Ensure there is wordlist data in the load_dir, a hash_file_path for the hash file to crack, an account_id is added and any other needed components specified in the config.yml file (ensure to use `instructions.yml` as a reference)

Make sure the server and client binaries are compiled:
```
make all
```

If at any point the project needs to be rebuilt:
```
make clean && make all
```

Run the project:
```
./bin/kloud-kraken-server ./config/<yaml_config>
```
<br>


## Contributing or Issues

[Contributing Documentation](CONTRIBUTING.md)
<br>


## License

The program is licensed under [PolyForm Noncommercial License 1.0.0](LICENSE.md)
<br>