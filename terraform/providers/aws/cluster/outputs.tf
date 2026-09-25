output "rancher_url" {
  description = "URL the Rancher UI is served on."
  value       = "https://${local.infra.rancher_hostname}"
}

output "api_endpoint" {
  description = "Kubernetes API endpoint, reachable from inside the VPC."
  value       = "https://${local.infra.api_vip}:6443"
}

output "resource_tag" {
  description = "Tag carried by every resource created by this module."
  value       = "elemental:id=${var.id}"
}

output "node_public_ips" {
  description = "Map of node name to its public address, for SSH when ssh_cidr is set."
  value = merge(
    { (local.init) = aws_instance.init.public_ip },
    { for k, i in aws_instance.join : k => i.public_ip },
  )
}
