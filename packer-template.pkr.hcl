variable "aws_access_key" {
    description = "AWS Access Key"
    type        = string
    sensitive   = true
}

variable "aws_secret_key" {
    description = "AWS Secret Key"
    type        = string
    sensitive   = true
}

variable "aws_region" {
    description = "AWS Region"
    type        = string
}

variable "aws_instance_type" {
    description = "AWS Instance Type"
    type        = string
}

variable "aws_subnet_id" {
    description = "AWS Subnet ID"
    type        = string
    default     = null
}

variable "aws_security_group_id" {
    description = "AWS Security Group ID"
    type        = string
    default     = null
}

# Instance store based EC2 builder with latest Alpine Linux image
builder "amazon-instance" {
    access_key        = var.aws_access_key
    secret_key        = var.aws_secret_key
    region            = var.aws_region
    instance_type     = var.aws_instance_type
    subnet_id         = lookup(var, "aws_subnet_id", null)
    security_group_id = lookup(var, "aws_security_group_id", null)
    ami_name          = "alpine-image-${timestamp()}"
    ssh_username      = "root"

    # Filter for latest Alpine Linux AMI
    source_ami_filter {
        filters = {
            virtualization-type = "hvm"
            name                = "*alpine-ami-3.18-x86_64*"
            root-device-type    = "instance-store"
        }
        owners      = ["951157211495"]  # Owner ID for Alpine Linux
        most_recent = true              # Ensures most recent version is used
    }
}

# Inital shell provisioner to update and setup image
provisioner "shell" {
    inline = [
        "apk update && apk upgrade",         # Update and upgrade Alpines package manager
        "mkdir /opt/provisioning"            # Create provisioning folder
    ]
}

# File upload provisioner for custom Go service
provisioner "file" {
    source      = "service"            # Local files to be uploaded to image
    destination = "/opt/provisioning"  # Location in image where files are stored
}

# Shell provisioner for running initialization script of all the tools to be installed
provisioner "shell" {
    script            = "scripts/init.sh"    # Local script location
    environment_vars  = ["HOSTNAME=test"]    # Enviroment variables used in script
    remote_folder     = "/opt/provisioning"  # Folder where the script will reside on the AMI
    skip_clean        = true                 # File will not be removed by packer
}
