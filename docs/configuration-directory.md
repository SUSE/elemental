# Configuration Directory Guide

The configuration directory is the place where users define the desired state of the image they intend to build by using the [elemental3 build](image-build-and-customization.md#elemental3-build) command.

Generally, the available configuration areas that this directory supports are the following:

* [Product Release Reference](#product-release-reference)
* [Operating System](#operating-system)
* [Kubernetes](#kubernetes)
* [Network](#network)

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
core:
  helm:
    - chart: foo
      valuesFile: foo.yaml
product:
  helm:
    - chart: bar
      valuesFile: bar.yaml
```

* `name` - Optional; Name of the product that all other configurations will be based on.
* `manifestURI` - Required; URI to a release manifest for the Core Platform or the Product that will be used as base. For more information, refer to the [Release Manifest](./release-manifest.md) guide. Supports both local file (file://) and OCI image (oci://) definitions.
* `core` - Optional; Components to explicitly enable from the Core Platform base.
  * `helm` - Required; List of Helm chart components that need to be enabled from the Core Platform base.
    * `chart` - Required; The actual chart that needs to be enabled, as seen in the Core Platform release manifest.
    * `valuesFile` - Optional; The name of the [Helm values file](https://helm.sh/docs/chart_template_guide/values_files/) (not including the path) that will be applied to this chart. The values file must be placed under `kubernetes/helm/values` for the specified chart.
* `product` - Optional; Components to explicitly enable from the desired product base. Applicable only if the manifest specified under `manifestURI` is a [product manifest](./release-manifest.md#product-release-manifest).
  * `helm` - Required; List of Helm chart components that need to be enabled from the product base.
    * `chart` - Required; The actual chart that needs to be enabled, as seen in the product release manifest.
    * `valuesFile` - Optional; The name of the [Helm values file](https://helm.sh/docs/chart_template_guide/values_files/) (not including the path) that will be applied to this chart. The values file must be placed under `kubernetes/helm/values` for the specified chart.

## Operating System

Users can provide configurations related to the operating system through the `install.yaml` and `os.yaml` files.

### install.yaml

The `install.yaml` file enables users to configure the OS installation process by introducing the following API:

```yaml
bootloader: grub
kernelCmdLine: "root=LABEL=SYSTEM console=ttyS0"
```

* `bootloader` - Required; Specifies the bootloader that will load the operating system.
* `kernelCmdLine` - Required; Parameters to add to the kernel when the operating system boots up.

### os.yaml

The `os.yaml` file enables users to configure the actual operating system by introducing the following API:

```yaml
diskSize: 35G
users:
  - username: root
    password: linux
```

* `diskSize` - Optional; Defines the size of the disk for the image that will be built. Defaults to `10G`.
* `users` - Required to have at least one; Defines users to be added to the operating system.
  * `username` - Required; Name of the user that needs to be added.
  * `password` - Required; Password for the user that needs to be added.

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

### Kubernetes Directory

The `kubernetes/` directory enables users to configure custom Helm chart values and/or further extend the Kubernetes cluster with locally defined manifests.

The directory's structure is as follows:

```shell
.
└── kubernetes
    ├── helm
    │   └── values
    │       └── rancher.yaml
    └── manifests
        └── local-manifest.yaml
```

* `helm` - Optional; Contains locally provided Helm chart configurations
  * `values` - Optional; Contains [Helm values files](https://helm.sh/docs/chart_template_guide/values_files/). Helm charts that require specified values must have a values file included in this directory.
* `manifests` - Optional; Contains locally provided Kubernetes manifests which will be applied to the cluster. Can be used separately or in combination with the manifests provided in the `kubernetes.yaml` file.

## Network

Network configuration can be declaratively applied through the `network/` directory in one of two ways:

1. Via [nmstate configuration files](#configuring-the-network-via-nmstate-files).
1. Via a [user-defined network script](#configuring-the-network-via-a-user-defined-script).

> **NOTE:** If the `network/` directory is missing, the system will implicitly fallback to DHCP.

> **IMPORTANT:** Elemental does not support mixing `nmstate` configuration files and a `user-defined` script within the same `network/` directory.

### Configuring the network via nmstate files 

You can define your desired network state by providing `nmstate` configuration files, in YAML format, within the `network/` directory. 

These files will be processed by the NetworkManager Configurator (`nmc`), a CLI tool that leverages the functionality provided by the `nmstate` library and enables users to easily define the desired state of their network.

Configurations for multiple hosts can be defined by creating files named after the hostname that would be set. This allows for multiple different nodes to be spawned from the same built image, with each node self-identifying during the first boot process based on MAC address matching of the network card(s). 

Examples for this type of configurations can be viewed under the [examples](../examples/elemental/build/network/) directory.

For more information on the `nmstate` library, refer to the [upstream documentation](https://nmstate.io).

For more information on `nmc`, refer to the [upstream repository](https://github.com/suse-edge/nm-configurator).

### Configuring the network via a user-defined script

For use cases where configuring the network through `nmstate` files is not applicable, you have the option to define a custom script that will do the actual network configuration.

This script will be executed on first boot during the `initrd` phase and needs to be provided under `network/configure-network.sh`:

```shell
.
├── ..
├── kubernetes/
└── network/
    └── configure-network.sh
```

> **NOTE:** If available, the default network is setup before the `configure-network.sh` runs. This ensures that the script is able to retrieve relevant configurations over the network if needed.

> **IMPORTANT:** The `configure-network.sh` script will run in a restricted environment. To apply the desired network state, you **must** provide your configurations through a set of helper tools available to the `configure-network.sh` script during execution. For a complete list of the avaiable tools, see the [Helper tools](#helper-tools) section. 

#### Helper tools

This section lists the tools that are available to the `configure-network.sh` script during its execution.

##### NetworkManager Configurator

The NetworkManager Configurator (`nmc`) is made available to the `configure-network.sh` script. You can retrieve your `nmstate` configuration files in whatever way best suits your use case, and then use `nmc` to generate and apply the desired network state.

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

`set_conf_d` is a shell function that can be called through the `configure-network.sh` script to automatically set configuration snippets in the NetworkManager's `conf.d` directory. As arguments, the function accepts either multiple files or a single directory.

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

`set_dispatcher_d` is a shell function that can be called through the `configure-network.sh` script to automatically set dispatcher scripts in the NetworkManager's `dispatcher.d` directory. As arguments, the function accepts either multiple files or a single directory.

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

`set_sys_conn` is a shell function that can be called through the `configure-network.sh` script to automatically set network connection profiles in the NetworkManager's `system-connections` directory. As arguments, the function accepts either multiple files or a single directory.

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

`set_hostname` is a shell function that can be called through the `configure-network.sh` script to automatically set the node's hostname. As arguments, the function accepts a single string literal.

*`configure-network.sh` example:*

```bash
#!/bin/bash
...
set_hostname "foo"
```

##### disable_wired_conn

`disable_wired_conn` is a shell function that can be called through the `configure-network.sh` script. It removes any existing wired connections and configures `no-auto-default=*` in the NetworkManager's `conf.d` directory.

*`configure-network.sh` example:*

```bash
#!/bin/bash
...
disable_wired_conn
```
