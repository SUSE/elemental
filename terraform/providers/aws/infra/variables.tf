variable "id" {
  description = "Unique identifier to be used for all created resources."
  type        = string
  validation {
    condition     = can(regex("^[a-z0-9]([a-z0-9-]{0,18}[a-z0-9])?$", var.id))
    error_message = "id must be between 1 and 20 lowercase alphanumeric or dash characters, and must not start or end with a dash."
  }
}

variable "profile" {
  description = "Deployment layout profile. Supported values: single-node (1 control-plane), multi-node (3 control-planes and 1 worker)"
  type        = string
  validation {
    condition     = contains(["single-node", "multi-node"], var.profile)
    error_message = "profile must be 'single-node' or 'multi-node'."
  }
}

variable "region" {
  description = "Region where resources will be created."
  type        = string
  default     = "eu-central-1"
}

variable "ssh_cidr" {
  description = "Allow SSH access on port 22 for the specified source IP range. Leave unset to disable SSH."
  type        = string
  default     = null
}
