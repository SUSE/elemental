# Cluster shape, intended to be shared throughout every provider.
locals {
  profiles = {
    single-node = [{ name = "single-node", role = "server" }]
    multi-node = [
      { name = "control-plane1", role = "server" },
      { name = "control-plane2", role = "server" },
      { name = "control-plane3", role = "server" },
      { name = "worker1", role = "agent" },
    ]
  }

  nodes = local.profiles[var.profile]
}
