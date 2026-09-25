output "node_names" {
  description = "All node names related to a specific profile."
  value       = [for n in local.nodes : n.name]
}

output "server_names" {
  description = "All server nodes related to a specific profile."
  value       = [for n in local.nodes : n.name if n.role == "server"]
}
