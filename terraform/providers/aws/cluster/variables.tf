variable "id" {
  description = "Unique identifier to be used for all created resources."
  type        = string
  validation {
    condition     = can(regex("^[a-z0-9]([a-z0-9-]{0,18}[a-z0-9])?$", var.id))
    error_message = "id must be between 1 and 20 lowercase alphanumeric or dash characters, and must not start or end with a dash."
  }
}

variable "infra_state_path" {
  description = "State file the infra stage wrote, read back for its outputs."
  type        = string
}

variable "platform" {
  description = "Platform the customized image was built for, as <os>/<arch>."
  type        = string
  default     = "linux/amd64"
  validation {
    condition     = contains(["linux/amd64", "linux/arm64"], var.platform)
    error_message = "platform must be 'linux/amd64' or 'linux/arm64'."
  }
}

variable "customized_img_path" {
  description = "Disk image produced by `elemental3 customize`."
  type        = string
}

variable "ignition_dir" {
  description = "Directory holding one <node-name>.ign per node."
  type        = string
}

variable "region" {
  description = "Region to create resources in. Must be the same as the infra stage."
  type        = string
  default     = "eu-central-1"
}

variable "machine_type" {
  description = "Type of the EC2 instance that will be used."
  type        = string
  # TODO(ivpe): change this once access to a paid account is available.
  default     = "c7i-flex.large"
}

variable "volume_size" {
  description = "Root volume size in GiB."
  type        = number
  default     = 35
}
