- This project uses duplicut for de-duplicating wordlists
    - Ensure the binary has executable permissions with `ls -la duplicut`
        - If not set them with `chmod +x duplicut/duplicut`

- Ensure Go is installed `sudo apt install -y golang`
    - Add these to shell rc file (usually .zshrc or .bashrc, echo $SHELL to find out)
        ```
        export GOROOT=/usr/lib/go
        export GOPATH=$HOME/go
        export PATH=$GOPATH/bin:$GOROOT/bin:$PATH
        ```
    - Reload rc file `source ~/.zshrc` (zsh example)

- Install Go packages with `go get ./...`
- Ensure any missing external dependencies are resolved `go mod tidy -e`
- Run the test cases in root directory of project `go test ./...`
<br>

- When running the program in full mode with AWS environment there are two options for credential setup
    - Configure API access credentials locally before running with `aws configure`
    - OR set the environment variables  AWS_ACCESS_KEY & AWS_SECRET_KEY


AWS Services Featured
---
- CloudWatch
- EC2
- IAM
- S3 Buckets
- SSM Parameter Store


- Start out by creating an IAM user group with below permissions attached using the policy editor
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
