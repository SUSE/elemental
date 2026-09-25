variable "profile" {
  description = "Layout of nodes by use-case: single-node or multi-node."
  type        = string
  validation {
    condition     = contains(["single-node", "multi-node"], var.profile)
    error_message = "profile must be 'single-node' or 'multi-node'."
  }
}
