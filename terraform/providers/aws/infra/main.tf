terraform {
  required_version = ">= 1.6.1"
  backend "local" {}

  required_providers {
    aws = { source = "hashicorp/aws", version = "6.66.0" }
  }
}

provider "aws" {
  region = var.region

  # Include default tags, so that orphaned resources due to unexpected
  # deletion errors can be found and handled.
  default_tags {
    tags = {
      "elemental:managed-by" = "terraform"
      "elemental:stack"      = "infra"
      "elemental:id"         = var.id
    }
  }
}

# Use the generic profile from modules/ and instantiate it
# here under the name "profile".
module "profile" {
  source  = "../../../modules/profiles"
  profile = var.profile
}

locals {
  # Prefix to be appended on every created resource.
  name = "ele-${var.id}"

  # Target groups mapping which port needs to be mapped to which node.
  tgs = {
    api   = { port = 6443, nodes = module.profile.server_names }
    sup   = { port = 9345, nodes = module.profile.server_names }
    http  = { port = 80, nodes = module.profile.node_names }
    https = { port = 443, nodes = module.profile.node_names }
  }
}

# Use the default VPC from the AWS account.
data "aws_vpc" "default" {
  default = true
}

# Use the default subnet of the first available availability zone.
data "aws_subnet" "default" {
  vpc_id            = data.aws_vpc.default.id
  availability_zone = data.aws_availability_zones.available.names[0]
  default_for_az    = true
}

# Retrieves the availability zones in the configured region that are in an
# 'available' state.
data "aws_availability_zones" "available" {
  state = "available"
}

# Setup a security group for the RKE2 nodes.
resource "aws_security_group" "node" {
  name        = "${local.name}-nodes"
  description = "RKE2 nodes"
  vpc_id      = data.aws_vpc.default.id
}

# Setup a security group for the internal control-plane NLB.
resource "aws_security_group" "control" {
  name        = "${local.name}-control-nlb"
  description = "RKE2 control NLB"
  vpc_id      = data.aws_vpc.default.id
}

# Setup a security group for the public facing NLB that will be used
# for application ingress access.
resource "aws_security_group" "ingress" {
  name        = "${local.name}-ingress-nlb"
  description = "RKE2 ingress NLB"
  vpc_id      = data.aws_vpc.default.id
}

# Configure egress access for all nodes.
resource "aws_vpc_security_group_egress_rule" "all" {
  for_each = {
    node    = aws_security_group.node.id
    control = aws_security_group.control.id
    ingress = aws_security_group.ingress.id
  }

  security_group_id = each.value
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

# Ensure all inbound traffic between resources referring to the same
# security group.
resource "aws_vpc_security_group_ingress_rule" "node_mesh" {
  security_group_id            = aws_security_group.node.id
  referenced_security_group_id = aws_security_group.node.id
  ip_protocol                  = "-1"
}

# If the ssh_cidr has been provided, ensure SSH access to the nodes.
resource "aws_vpc_security_group_ingress_rule" "ssh" {
  count             = var.ssh_cidr == null ? 0 : 1
  security_group_id = aws_security_group.node.id
  cidr_ipv4         = var.ssh_cidr
  ip_protocol       = "tcp"
  from_port         = 22
  to_port           = 22
}

# Allow all resources within the VPC CIDR to reach the internal control-plane NLB
# on the RKE2 API and supervisor ports.
resource "aws_vpc_security_group_ingress_rule" "control_in" {
  for_each          = toset(["6443", "9345"])
  security_group_id = aws_security_group.control.id
  cidr_ipv4         = data.aws_vpc.default.cidr_block
  ip_protocol       = "tcp"
  from_port         = tonumber(each.key)
  to_port           = tonumber(each.key)
}

# Allow the internal control-plane NLB to forward RKE2 API and supervisor
# traffic to the cluster nodes.
resource "aws_vpc_security_group_ingress_rule" "node_from_control" {
  for_each                     = toset(["6443", "9345"])
  security_group_id            = aws_security_group.node.id
  referenced_security_group_id = aws_security_group.control.id
  ip_protocol                  = "tcp"
  from_port                    = tonumber(each.key)
  to_port                      = tonumber(each.key)
}

# Allow public access to the ingress NLB on HTTP and HTTPS.
resource "aws_vpc_security_group_ingress_rule" "ingress_in" {
  for_each          = toset(["80", "443"])
  security_group_id = aws_security_group.ingress.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "tcp"
  from_port         = tonumber(each.key)
  to_port           = tonumber(each.key)
}

# Allow the public ingress NLB to send HTTP and HTTPS traffic to the cluster nodes.
resource "aws_vpc_security_group_ingress_rule" "node_from_ingress" {
  for_each                     = toset(["80", "443"])
  security_group_id            = aws_security_group.node.id
  referenced_security_group_id = aws_security_group.ingress.id
  ip_protocol                  = "tcp"
  from_port                    = tonumber(each.key)
  to_port                      = tonumber(each.key)
}

# Prepare load balancer target groups as defined by local.tgs.
resource "aws_lb_target_group" "this" {
  for_each = local.tgs

  name                 = "${local.name}-${each.key}"
  port                 = each.value.port
  protocol             = "TCP"
  vpc_id               = data.aws_vpc.default.id
  target_type          = "ip"
  deregistration_delay = 30

  health_check { protocol = "TCP" }
}

# Allocate an EIP that will be used by the ingress NLB.
resource "aws_eip" "ingress" {
  domain = "vpc"
}

# Create ingress NLB.
resource "aws_lb" "ingress" {
  name               = "${local.name}-ingress"
  load_balancer_type = "network"
  internal           = false
  security_groups    = [aws_security_group.ingress.id]

  subnet_mapping {
    subnet_id     = data.aws_subnet.default.id
    allocation_id = aws_eip.ingress.id
  }
}

# Create internal control NLB.
resource "aws_lb" "control" {
  name               = "${local.name}-control"
  load_balancer_type = "network"
  internal           = true
  security_groups    = [aws_security_group.control.id]

  subnet_mapping {
    subnet_id = data.aws_subnet.default.id
  }
}

# Find the ENI created for the control NLB by filtering it
# by security group.
data "aws_network_interface" "control" {
  filter {
    name   = "group-id"
    values = [aws_security_group.control.id]
  }

  depends_on = [aws_lb.control]
}

# Configure control NLB listeners for the API and supervisor ports.
resource "aws_lb_listener" "control" {
  for_each          = { api = 6443, sup = 9345 }
  load_balancer_arn = aws_lb.control.arn
  protocol          = "TCP"
  port              = each.value

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.this[each.key].arn
  }
}

# Configure ingress NLB listeners for HTTP and HTTPS ports.
resource "aws_lb_listener" "ingress" {
  for_each          = { http = 80, https = 443 }
  load_balancer_arn = aws_lb.ingress.arn
  protocol          = "TCP"
  port              = each.value

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.this[each.key].arn
  }
}
