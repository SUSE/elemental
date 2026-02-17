# Configuration Directory Guide

The configuration directory is the place where users define the desired state of the image they intend to build by using the [`elemental3 customize`](image-customization.md#customization-process) command.

Generally, the available configuration areas that this directory supports are the following:

* [Product Release Reference](#product-release-reference)
* [Operating System](#operating-system)
* [Kubernetes](#kubernetes)
* [Network](#network)
* [Custom Scripts](#custom-scripts)

This document provides an overview of each configuration area, the rationale behind it and its API.

## Product Release Reference

> **NOTE:** Before reviewing this file, make sure you familiarize yourself with the [release manifest](release-manifest.md) concept.

One of Elemental's key features is enabling users to base their image on a set of components that are aligned with a specific product release.

Consumers can use the `release.yaml` file to configure the desired product that they wish to use as base. Furthermore, they can explicitly choose which components from this product to enable based on their specific use case.

### release.yaml

```yaml
name: suse-product
manifestURI: file:///path/to/manifest/suse-product-manifest.yaml
# manifestURI: oci://registry.suse.com/suse-product/release-manifest:0.0.1
components:
  helm:
    - chart: foo
      valuesFile: foo.yaml
  systemd:
    - extension: bar
```

* `name` - Optional; Name of the product that all other configurations will be based on.
* `manifestURI` - Required; URI to a release manifest for the Core Platform or the Product that will be used as base. For more information, refer to the [Release Manifest](./release-manifest.md) guide. Supports both local file (file://) and OCI image (oci://) definitions.
* `components` - Optional; Components to explicitly enable from the Core Platform base.
  * `helm` - Optional; List of Helm chart components that need to be enabled from the Core Platform base.
    * `chart` - Required; The actual chart that needs to be enabled, as seen in the Core Platform release manifest.
    * `valuesFile` - Optional; The name of the [Helm values file](https://helm.sh/docs/chart_template_guide/values_files/) (not including the path) that will be applied to this chart. The values file must be placed under `kubernetes/helm/values` for the specified chart.
  * `systemd` - Optional; List of System extensions that need to be enabled from the product base.
    * `extension` - Required; The actual extension that needs to be enabled, as seen in the product release manifest.

## Operating System

Users can provide configurations related to the operating system through the `install.yaml` and `butane.yaml` files.

### install.yaml

The `install.yaml` file enables users to configure the OS installation process by introducing the following API:

```yaml
bootloader: grub
kernelCmdLine: "console=ttyS0"
raw:
  diskSize: 8G
iso:
  device: "/dev/sda"
```

* `bootloader` - Required; Specifies the bootloader that will load the operating system.
* `kernelCmdLine` - Optional; Parameters to add to the kernel when the operating system boots up. The tool itself defines the essential parameters to boot (e.g. `root=LABEL=SYSTEM`),
   the string provided here is simply concatenated after them in order to provide a mechanism to include additional custom parameters.
* `raw` - Required for RAW images; Specifies RAW disk image configurations.
  * `diskSize` - Required; Specifies the size of the resulting disk image.
* `iso` - Required for ISO images; Specifies ISO image configurations.
  * `device` - Required; Specifies the disk that will be used as the install device.

### butane.yaml

The `butane.yaml` optional file enables users to configure the actual operating system by allowing them to provide their own [Butane](https://coreos.github.io/butane/) configuration.
During the customization processes, this will be translated into an [Ignition](https://coreos.github.io/ignition/) configuration which will be included in the image and executed at first boot.
The example below shows how it can be used to set up users:

```yaml
version: 1.6.0
variant: fcos
passwd:
  users:
  - name: root
    # Hash for 'linux' passwd created with "openssl passwd -6"
    password_hash: "$6$dkiCjuXvS8brdFUA$w1b4wSV.0wQ7BmZ7l/Be6fhqlk8CMEE8NQkhtaXIPjMTFw90JNYfI1lBhSoUILhmqupcmOp681FHIdvIZdbc90"
```

Elemental does not enforce or prefer any specific Butane variant.

Check [Filesystem Modes](./filesystem-modes.md) for more information on the filesystem layout and which paths are writable.

Check [Elemental and Ignition Integration](./ignition-integration.md) for further details about Ignition being used in the scope of Elemental.

> [!NOTE]
> The inclusion of an external Butane configuration file is not considered to be a stable part of the Elemental user interface. Butane configuration
> could be superseded by a native Elemental declaration in the future.

## Kubernetes

Users can provide Kubernetes related configurations through the `kubernetes.yaml` file and/or the `kubernetes/` directory.

### kubernetes.yaml

The `kubernetes.yaml` file enables users to extend the Kubernetes cluster with Helm charts and/or remote Kubernetes manifests by introducing the following API:

```yaml
manifests:
  - https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.31/deploy/local-path-storage.yaml
helm:
  charts:
    - name: "rancher"
      version: "2.11.1"
      targetNamespace: "cattle-system"
      repositoryName: "rancher"
      valuesFile: rancher.yaml
  repositories:
    - name: "rancher"
      url: "https://releases.rancher.com/server-charts/stable"
nodes:
- hostname: node1.example
  type: server
  init: true
- hostname: node2.example
  type: server
- hostname: node3.example
  type: server
- hostname: node4.example
  type: agent
network:
    apiVIP: 192.168.122.100
    apiHost: api.cluster01.example.com
    apiVIP6: fd12:3456:789a::21
```

* `manifests` - Optional; Defines remote Kubernetes manifests to be deployed on the cluster.
* `helm` - Optional; Defines a set of Helm charts and their sources.
  * `charts` - Required; Defines a list of Helm charts to be deployed on the cluster.
    * `name` - Required; Name of the Helm chart, as seen in the repository.
    * `version` - Required; Version of the Helm chart, as seen in the repository.
    * `targetNamespace` - Required; Namespace where the Helm chart will be deployed.
    * `repositoryName` - Required; Name of the source repository that this chart can be retrieved from.
    * `valuesFile` - Optional; The name of the [Helm values file](https://helm.sh/docs/chart_template_guide/values_files/) (not including the path) that will be applied to this chart. The values file must be placed under `kubernetes/helm/values` for the specified chart.
  * `repositories` - Required; Source repositories for the Helm charts.
    * `name` - Required; Defines the name for this repository. This name doesn't have to match the name of the actual
    repository, but must correspond with the `repositoryName` of one or more charts.
    * `url` - Required; Defines the URL where this chart repository can be reached.
* `nodes` - Required for multi-node clusters; Defines a list of all nodes that form the cluster.
  * `hostname` -  Required; Indicates the fully qualified domain name (FQDN) to identify the particular node on which the remainder of these attributes will be applied.
  * `type` - Required; Selects the Kubernetes node type, either server (for control plane nodes) or agent (for worker nodes).
  * `init` - Optional; Indicates which node should function as the cluster initializer. The initializer node is the server node which bootstraps the cluster and allows other nodes to join it. If unset, the first server in the node list will be selected as the initializer.
* `network`:
  * `apiVIP` - Required for multi-node clusters if not using `apiVIP6`; Specifies the IPv4 address which will serve as the cluster LoadBalancer, backed by MetalLB.
  * `apiVIP6` -  Required for multi-node clusters if not using `apiVIP`; Specifies the IPv6 address which will serve as the cluster LoadBalancer, backed by MetalLB.
  * `apiHost` - Optional; Specifies the domain address for accessing the cluster.

### Kubernetes Directory

The `kubernetes/` directory enables users to configure custom Helm chart values and/or further extend the Kubernetes cluster with locally defined manifests.

The directory's structure is as follows:

```text
.
└── kubernetes
    ├── helm
    │   └── values
    │       └── rancher.yaml
    ├── manifests
    │   └── local-manifest.yaml
    └── config
        ├── agent.yaml
        └── server.yaml
```

* `helm` - Optional; Contains locally provided Helm chart configurations
  * `values` - Optional; Contains [Helm values files](https://helm.sh/docs/chart_template_guide/values_files/). Helm charts that require specified values must have a values file included in this directory.
* `manifests` - Optional; Contains locally provided Kubernetes manifests which will be applied to the cluster. Can be used separately or in combination with the manifests provided in the `kubernetes.yaml` file.
* `config` - Optional; Contains locally provided Kubernetes configuration files, `server.yaml` for control-plane nodes and `agent.yaml` for workers.

## Network

Network configuration can be declaratively applied through the `network/` directory in one of two ways:

1. Via [nmstate configuration files](#configuring-the-network-via-nmstate-files).
1. Via a [user-defined network script](#configuring-the-network-via-a-user-defined-script).

> **NOTE:** If the `network/` directory is missing, the system will implicitly fall back to DHCP.

> **IMPORTANT:** Elemental does not support mixing `nmstate` configuration files and a `user-defined` script within the same `network/` directory.

### Configuring the network via nmstate files

You can define your desired network state by providing `nmstate` configuration files, in YAML format, within the `network/` directory.

These files will be processed by the NetworkManager Configurator (`nmc`), a CLI tool that leverages the functionality provided by the `nmstate` library and enables users to easily define the desired state of their network.

You can define the configurations for multiple hosts by creating files named after the hostname that would be set. Thereby allowing multiple different nodes to be spawned from the same built image, with each node self-identifying during the first boot process based on MAC address matching of the network card(s).

Examples for this type of configurations can be viewed under the `examples` directory — [single-node](../examples/elemental/customize/single-node/network) setup and [multi-node](../examples/elemental/customize/multi-node/network) setup.

For more information on the `nmstate` library, refer to the [upstream documentation](https://nmstate.io).

For more information on `nmc`, refer to the [upstream repository](https://github.com/suse-edge/nm-configurator).

### Configuring the network via a user-defined script

For use cases where configuring the network through `nmstate` files is not sufficient, you can define a custom script for the actual network configuration.

A script named `configure-network.sh` will be executed on first boot during the `initrd` phase:

```text
.
├── ..
├── kubernetes/
└── network/
    └── configure-network.sh
```

> **NOTE:** If available, the default network is set up before the `configure-network.sh` runs. This ensures that the script is able to retrieve relevant configurations also from remote locations.

> **IMPORTANT:** The `configure-network.sh` script will run in a restricted environment. To apply the desired network state, you **must** provide your configurations through a set of helper tools available to the `configure-network.sh` script during execution. For a complete list of the available tools, see the [Helper tools](#helper-tools) section.

#### Helper tools

This section lists the tools that are available to the `configure-network.sh` script during its execution.

##### NetworkManager Configurator

The NetworkManager Configurator (`nmc`) is available for the `configure-network.sh` script. You can retrieve your `nmstate` configuration files in whatever way best suits your use case, and then use `nmc` to generate and apply the desired network state.

*`configure-network.sh` example:*

```bash
#!/bin/bash
...
mkdir desired-states
curl -L -o desired-states/my.host.yaml https://example.com/my.host.yaml

mkdir generated
nmc generate --config-dir desired-states --output-dir generated
nmc apply --config-dir generated
```

##### set_conf_d

You can call the `set_conf_d`  shell function to apply configuration snippets into NetworkManager's `conf.d` directory. It accepts either multiple files or a single directory as arguments.

*`configure-network.sh` example:*

```bash
#!/bin/bash
...
mkdir configs
curl -L -o configs/foo.conf https://example.com/foo.conf
curl -L -o configs/bar.conf https://example.com/bar.conf

# example: passing a directory
set_conf_d "configs/"

# example: passing multiple files
set_conf_d "configs/foo.conf" "configs/bar.conf"
```

##### set_dispatcher_d

You can call the `set_dispatcher_d` shell function to set dispatcher scripts in the NetworkManager's `dispatcher.d` directory. It accepts either multiple files or a single directory as arguments.

*`configure-network.sh` example:*

```bash
#!/bin/bash
...
mkdir dispatchers
curl -L -o dispatchers/foo.sh https://example.com/foo.sh
curl -L -o dispatchers/bar.sh https://example.com/bar.sh

# example: passing a directory
set_dispatcher_d "dispatchers/"

# example: passing multiple files
set_dispatcher_d "dispatchers/foo.sh" "dispatchers/bar.sh"
```

##### set_sys_conn

You can call the `set_sys_conn` shell function to set network connection profiles in the NetworkManager's `system-connections` directory. It accepts either multiple files or a single directory as arguments.

> **IMPORTANT:** Using both `set_sys_conn` and `nmc` to configure the network connection profiles may result in unexpected behaviour. Consider using one or the other depending on your use case.

*`configure-network.sh` example:*

```bash
#!/bin/bash
...
mkdir sys-conns
curl -L -o sys-conns/foo.nmconnection https://example.com/foo.nmconnection
curl -L -o sys-conns/bar.nmconnection https://example.com/bar.nmconnection

# example: passing a directory
set_sys_conn "sys-conns/"

# example: passing multiple files
set_sys_conn "sys-conns/foo.nmconnection" "sys-conns/bar.nmconnection"
```

##### set_hostname

You can call the `set_hostname` shell function to set the node's hostname. It accepts a single string literal as an argument.

*`configure-network.sh` example:*

```bash
#!/bin/bash
...
set_hostname "myhostname"
```

##### disable_wired_conn

You can call the `disable_wired_conn` shell function to remove any existing wired connections and configure `no-auto-default=*` in the NetworkManager's `conf.d` directory.

*`configure-network.sh` example:*

```bash
#!/bin/bash
...
disable_wired_conn
```

## Custom Scripts

Elemental can bundle in custom scripts that will be executed during the firstboot phase of provisioning a system.
Additionally, these scripts may require the availability of particular local files which can be embedded into the configuration partition too.

These scripts are executed alphabetically. It is suggested to use a numbered prefix within the 50–99 range (e.g. `60-my-script.sh`).
Elemental may leverage the values in the 00–49 range in the future, so unless necessary, those should be avoided.

Finally, if any of the provided scripts or files is needed beyond the firstboot phase, a script should be included that explicitly copies them to the filesystem.

```text
.
├── ...
└── custom
    ├── files
    │   ├── custom-binary
    │   ├── subdirectory
    │   │   └── some-file.txt
    │   └── custom-script.sh
    └── scripts
        └── 70-manual-configuration.sh
```

* `custom` - May be included to inject files into the configuration partition. Files are organized by subdirectory as follows:
  * `scripts` - If present, all the files in this directory will be included in the built / customized image and automatically
    executed during the firstboot phase.
  * `files` - If present, all the files, directories, and subdirectories in this directory will be available at firstboot on the booted system.

Note that attempting to write to read-only directories (e.g., `/usr`) from a custom script will fail.
Check [Filesystem Modes](./filesystem-modes.md) for more information on the filesystem layout and which paths are writable.

It is crucial to perform cleanup (unmounting) in every script that involves mounting a specific path.
