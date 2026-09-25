# Deploying Elemental on the Cloud

To deploy an Elemental cluster on the cloud, you need to:
1. Prepare the necessary infrastructure resources.
1. Build a customized image for the specific cloud provider.
1. Upload the image and spin the desired machines.

In the sections below you will find general information on what is necessary for Elemental to be deployed on a specific cloud provider. 

For plug-and-play examples, refer to the [single-node](cookbook-first-time-use.md#recipe-4-single-node-kubernetes-cluster-on-aws) and [multi-node](cookbook-first-time-use.md#recipe-5-multi-node-kubernetes-cluster-on-aws) AWS examples in the Cookbook document.

## Infrastructure Resources Preparation

To facilitate the RKE2 cluster loadbalancing, before building the customized image, ensure you have the following available on your provider:

1. Internal loadbalancer that forwards traffic to the RKE2 API (6443) and supervisor (9345) on each of the cluster nodes - The private IP and hostname of this resource is going to be needed during the image customization phase.
1. [Optional] A second loadbalancer that forwards HTTP (80) and HTTPS (443) traffic to the RKE2 ingress controller - For applications that need to be accessesed from outside the cluster (e.g. Rancher UI).

> **NOTE:** This section describes only the expected end state of the infrastructure. It does not cover every resource that may be required to achieve that state, as the specific resources and implementation details will vary between cloud providers.

## Preparing the Image

Customizing an Elemental image for the cloud is as simple as creating a [configuration directory](./configuration-directory.md), populating it with desired configurations and starting the [customization process](image-customization.md#customization-process).

The only mandatory requirements for the configuration directory are the following:
1. Under `kernelCmdLine` in `install.yaml` you **must** provide the [ignition platform identifier](https://coreos.github.io/ignition/supported-platforms/) for the cloud provider you will be deploying on - Necessary so that ignition knows from where to retrieve runtime configurations.
1. Under `kubernetes/cluster.yaml` make sure to provide the following configurations:
   * `network.apiVIP` - The private address of the internal loadbalancer created in the previous section.
   * `network.apiHost` - The host of the internal loadbalancer created in the previous section.
   * `network.apiVIPMode: "external"` - To avoid the default MetalLB loadbalancing that is done for non-cloud clusters.
1. Do not define any static node role configuration under the `kubernetes/cluster.yaml` file - It is hard to match static configurations with the dynamic nature of cloud providers. As such, node role configuration will be provided during the machine creation through ignition.
1. Do not define any static network configuration under the `network/` directory - Any network configuration should happen on cloud provider level.

Apart from the above requirements, feel free to make any configurations necessary for your environment. For example,adding an SSH key or another user, writing a file on the filesystem, including additional Kubernetes manfiests and charts, or anything else!

An example of a configuration directory for AWS can be found [here](../examples/elemental/customize/aws/).

## Spinning the Elemental Machines

> **NOTE:** This section assumes you have uploaded your customized image to the desired cloud provider and created the necessary resources so that a machine can be spun from that image.

Up to now, you have customized an image that has no notion of what type of RKE2 node will it be booted as. This is configured at runtime by passing an ignition configuration to the machine's metadata (e.g. user-data for AWS).

At a minimum, this ignition configuration needs to be shipping the following:
1. A `runtime.env` file created at `/var/lib/elemental/` - This file contains runtime configurations that the Elemental tooling parses at boot time.
1. A defined hostname under the `/etc/hostname` file - So that nodes can be differentiated between one another.

Below you can find information on how to create this ignition configuration. 

### Runtime Ignition Creation

#### runtime.env

To determine the node's role in the cluster, usually Elemental relies on a statically defined network, host and node information within the customized image. 

However, this is not the right approach when it comes to cloud providers. As such, Elemental now supports runtime configurations through a `/var/lib/elemental/runtime.env` file. This file is provided at boot time on each node through an additional user defined ignition configuration.

This is a standard `.env` file that supports the following environment variables:

* `NODETYPE` - Type of the node that this machine will represent. Supported values: server, agent.
* `IS_INIT_NODE` - Applies only to `server` nodes; describes whether this is the initialiser server node. Default: false.

> **NOTE:** Configurations here will be extended in the future based on user feedback and feature development.

#### Creating the Configuration

> **NOTE:** For specific information on how ignition integrates with  Elemental, refer to the [Elemental and Ignition Integration](ignition-integration.md) document.

The easiest way to create the ignition configuration is to use [butane](https://coreos.github.io/butane/). 

For a single-node cluster, create a directory with the following contents:

1. A `runtime.env` file with `NODETYPE=server` and `IS_INIT_NODE=true`.
1. A `butane.yaml` file with the following configuration:
    ```yaml
    variant: fcos
    version: 1.6.0

    storage:
    files:
        - path: /var/lib/elemental/runtime.env
        mode: 0644
        overwrite: true
        contents:
            local: runtime.env
        - path: /etc/hostname
        mode: 0644
        overwrite: true
        contents:
            inline: |
            single-node-example.com
    ```

Then run butane against that directory:

```shell
butane --strict --pretty --files-dir <dir> butane.yaml > config.ign
```

This will produce the necessary ignition configuration for a single-node cluster. For multi node clusters there needs to be a directory for each node.

For more information, refer to the available examples for [single-node](../examples/elemental/runtime-configs/single-node/) and [multi-node](../examples/elemental/runtime-configs/multi-node/) runtime configurations.
