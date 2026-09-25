terraform {
  required_version = ">= 1.6.1"
  backend "local" {}

  required_providers {
    aws  = { source = "hashicorp/aws", version = "6.66.0" }
    time = { source = "hashicorp/time", version = "0.14.2" }
  }
}

provider "aws" {
  region = var.region

  # Include default tags, so that orphaned resources due to unexpected
  # deletion errors can be found and handled.
  default_tags {
    tags = {
      "elemental:managed-by" = "terraform"
      "elemental:stack"      = "cluster"
      "elemental:id"         = var.id
    }
  }
}

# Read the infra state so that the infra and cluster states do not drift apart.
# The path is passed in rather than derived, since each cluster id keeps its own.
data "terraform_remote_state" "infra" {
  backend = "local"
  config  = { path = var.infra_state_path }
}

# Store the identity that Terraform is authenticated as.
data "aws_caller_identity" "current" {}

locals {
  # Store outputs from infra module as locals.
  infra = data.terraform_remote_state.infra.outputs

  # Mark first node as init. Nodes are created in order as defined in the multi-node profile,
  # so the first node is always a server node.
  init    = local.infra.node_names[0]
  joiners = slice(local.infra.node_names, 1, length(local.infra.node_names))

  attachments = merge([
    for tg_name, tg in local.infra.target_groups : {
      for node in tg.nodes : "${tg_name}-${node}" => {
        arn  = tg.arn
        port = tg.port
        node = node
      }
    }
  ]...)

  # Ensures that the init node is registered first before any joining nodes.
  init_attachments = { for k, a in local.attachments : k => a if a.node == local.init }
  join_attachments = { for k, a in local.attachments : k => a if a.node != local.init }

  architecture = { "linux/amd64" = "x86_64", "linux/arm64" = "arm64" }[var.platform]

  # Hash used to differentiate between image builds.
  image_hash = try(
    split(" ", trimspace(file("${var.customized_img_path}.sha256")))[0],
    filemd5(var.customized_img_path),
    "unavailable",
  )
}

# Setup S3 bucket.
resource "aws_s3_bucket" "images" {
  bucket        = "ele-images-${data.aws_caller_identity.current.account_id}-${var.id}"
  force_destroy = true
}


# Setup IAM role for disk image import/export.
resource "aws_iam_role" "vmimport" {
  name = "ele-vmimport-${var.id}"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "vmie.amazonaws.com" }
      Action    = "sts:AssumeRole"
      Condition = { StringEquals = { "sts:ExternalId" = "vmimport" } }
    }]
  })
}

# Setup role policy for the vmimport IAM role.
resource "aws_iam_role_policy" "vmimport" {
  name = "ele-vmimport-s3"
  role = aws_iam_role.vmimport.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = ["s3:GetBucketLocation", "s3:ListBucket"]
        Resource = aws_s3_bucket.images.arn
      },
      {
        Effect   = "Allow"
        Action   = ["s3:GetObject"]
        Resource = "${aws_s3_bucket.images.arn}/*"
      },
      {
        Effect   = "Allow"
        Action   = ["ec2:ModifySnapshotAttribute", "ec2:CopySnapshot", "ec2:RegisterImage", "ec2:Describe*"]
        Resource = "*"
      },
    ]
  })
}

# Delay after IAM policy creation. IAM is eventually consistent, so without this
# the import can fail with a confusing "service role does not exist".
resource "time_sleep" "iam" {
  depends_on      = [aws_iam_role_policy.vmimport]
  create_duration = "20s"
}

# Upload the customized raw image in S3.
resource "aws_s3_object" "raw" {
  bucket = aws_s3_bucket.images.id
  key    = "images/${local.image_hash}-${basename(var.customized_img_path)}"
  source = var.customized_img_path
}

# Import the customized raw image and convert it into an EBS snapshot.
resource "aws_ebs_snapshot_import" "this" {
  role_name  = aws_iam_role.vmimport.name
  depends_on = [time_sleep.iam]

  disk_container {
    format = "RAW"
    user_bucket {
      s3_bucket = aws_s3_object.raw.bucket
      s3_key    = aws_s3_object.raw.key
    }
  }

  timeouts { create = "90m" }
}

# Create AMI from which instances will be spun.
resource "aws_ami" "this" {
  name                = "ele-${aws_ebs_snapshot_import.this.id}"
  architecture        = local.architecture
  virtualization_type = "hvm"
  ena_support         = true
  boot_mode           = "uefi"
  imds_support        = "v2.0"
  root_device_name    = "/dev/xvda"

  ebs_block_device {
    device_name           = "/dev/xvda"
    snapshot_id           = aws_ebs_snapshot_import.this.id
    volume_size           = var.volume_size
    volume_type           = "gp3"
    delete_on_termination = true
  }

  timeouts { create = "60m" }
}

# Spin init node.
resource "aws_instance" "init" {
  ami                         = aws_ami.this.id
  instance_type               = var.machine_type
  subnet_id                   = local.infra.subnet_id
  vpc_security_group_ids      = [local.infra.node_sg_id]
  associate_public_ip_address = true
  user_data                   = file("${var.ignition_dir}/${local.init}.ign")
  user_data_replace_on_change = true

  tags = { Name = "ele-${var.id}-${local.init}" }
}

# Spin joining nodes.
resource "aws_instance" "join" {
  for_each   = toset(local.joiners)
  depends_on = [aws_instance.init, aws_lb_target_group_attachment.init]

  ami                         = aws_ami.this.id
  instance_type               = var.machine_type
  subnet_id                   = local.infra.subnet_id
  vpc_security_group_ids      = [local.infra.node_sg_id]
  associate_public_ip_address = true
  user_data                   = file("${var.ignition_dir}/${each.key}.ign")
  user_data_replace_on_change = true

  tags = { Name = "ele-${var.id}-${each.key}" }
}

# Register the init node, using the address AWS handed it.
resource "aws_lb_target_group_attachment" "init" {
  for_each = local.init_attachments

  target_group_arn = each.value.arn
  target_id        = aws_instance.init.private_ip
  port             = each.value.port
}

# Register the joining nodes.
resource "aws_lb_target_group_attachment" "join" {
  for_each = local.join_attachments

  target_group_arn = each.value.arn
  target_id        = aws_instance.join[each.value.node].private_ip
  port             = each.value.port
}
