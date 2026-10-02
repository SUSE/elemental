output "node_names" {
  description = "Name of each node for the specific profile."
  value       = module.profile.node_names
}

output "target_groups" {
  description = "List of target groups against which instances need to be registered."
  value = {
    for k, tg in local.tgs : k => {
      arn   = aws_lb_target_group.this[k].arn
      port  = tg.port
      nodes = tg.nodes
    }
  }
}

output "api_vip" {
  description = "Private address AWS assigned to the internal control-plane load balancer."
  value       = data.aws_network_interface.control.private_ip
}

output "api_host" {
  description = "DNS name of the internal control-plane load balancer."
  value       = aws_lb.control.dns_name
}

output "rancher_hostname" {
  description = "Hostname Rancher is served on."
  value       = "rancher.${aws_eip.ingress.public_ip}.sslip.io"
}

output "subnet_id" {
  description = "Subnet to be used for resource creation."
  value       = data.aws_subnet.default.id
}

output "node_sg_id" {
  description = "Security group to be used for resource creation."
  value       = aws_security_group.node.id
}
