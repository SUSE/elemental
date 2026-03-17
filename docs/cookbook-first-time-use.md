# **Cookbook: Deploying UC For The First Time**

# **Introduction**

UC is a SLES-based, immutable, image-oriented Kubernetes-native infrastructure stack designed to serve as the common foundation for the next generation of SUSE solutions. Unlike traditional distributions, UC treats the operating system, Kubernetes layer, and management agents as a single atomic unit, bound together by a "release manifest."

This architecture allows us to streamline initial deployment and lifecycle management by ensuring that the entire stack is versioned, validated, and upgraded as a coherent whole. While it provides a rigorous baseline, UC is built for flexibility; it provides the essential tooling required to customize and extend the system, enabling SUSE products, partners, and customers to tailor the stack for diverse environments and unique solution requirements.

## Release Information

UC comprises a few components, their comprehensive integration and validation, and effective delivery:

| Component | Version | Purpose |
| :---: | :---: | :---: |
| Elemental | 3.0.0-alpha | Customization and installation of OS \+ Kubernetes |
| SUSE Linux Enterprise Server | 16.0 | Source for building a containerized OS |
| RKE2 | 1.34.2 | Certified Kubernetes distribution |
| MetalLB | 0.15.2 | Load balancing/HA capabilities for multi-node Kubernetes clusters (API and services) |
| Endpoint Copier Operator | 0.3.0 |  |

## Scope and Audience

This “cookbook” details three distinct provisioning scenarios:

* **Recipe 1:** Deployment of a “single-node” Kubernetes cluster using an example SUSE solution utilizing UC.

* **Recipe 2:** Deployment of a “multi-node” Kubernetes cluster using an example SUSE solution utilizing UC

* **Recipe 3:** Deployment of a “single-node” Kubernetes cluster deployment, provisioned via [Cluster API](https://cluster-api.sigs.k8s.io/) and using UC as a base.

This is a guide for **anyone** interested in understanding UC, its goals, and the underlying technology.

## Prerequisites

As this guide will walk you through the full deployment, you will need to ensure that your local workstation (or remote development system) meets the following hardware, software, and networking requirements if you’re to successfully complete the tasks outlined in this cookbook.

### Hardware Requirements

There are two different roles for the hosts needed:

* **Customization Host** \-\> Used for creating the UC images.

* **Hypervisor Host** \-\> Used for provisioning VMs based on the images created above.

Those two roles **can** be the same physical host to simplify the scenario.

**NOTE:** UC also supports bare metal provisioning, so the hypervisor host is only necessary for virtualization purposes as recommended as part of this guide; it’s perfectly possible to use a bare metal system, laptop, or other piece of equipment (e.g. Raspberry Pi) to boot your resulting images.

The requirements will vary depending on whether the machine used for customizing artifacts is the same one that is also going to be used for provisioning virtual machines. If that is the case, multiply the requirements below by the target number of Kubernetes nodes to be deployed.

#### Customization Host

* **CPU Architecture:** x86\_64 (v2 instruction set); aarch64 should work as well.
* **Memory (RAM):** 8 GB or higher
* **Disk Space:**
  * RAW disk: 50 GB for writing a 35 GB RAW disk
  * ISO media: 10 GB for writing a \~1 GB ISO
* **Internet Access** for downloading customization artifacts. Offline / air-gapped capabilities are scheduled for the future (UC 0.6+).

#### Hypervisor Host

* **CPU Architecture:** x86\_64 (v2 instruction set); aarch64 may work but is not comprehensively validated yet.
* **Memory (RAM):** 32 GB or higher (depending on the number of VMs intended to run, each of those are recommended 16GB+, 12 GB minimum)
* **Disk Space:**
  * 100 GB for hosting the VM disks (depending on the number of VMs intended to run, each of those will be 35 GB but if disks are thin provisioned it will be less)
* **Internet Access** for downloading runtime artifacts, container images, etc.

#### Kubernetes Node (VM)

The base requirements per virtual machine (VM), acting as a Kubernetes node, are aligned to the [RKE2 expectations](https://docs.rke2.io/install/requirements). To ensure robust operation, the examples in this guide utilize:

* **CPU cores:** 2 required, recommended at least 4
* **Memory (RAM):** 12 GB (16GB+ recommended, depending on workload)
* **Disk Space:** 35 GB

### Software and Tools

There are a few requirements for the host(s) acting as the entrypoint for customization and hypervisor:

* **Operating System:** Linux.
  * SUSE and openSUSE derivatives are obviously recommended due to our testing, e.g. SLES 16.0, Leap 16.0 or Tumbleweed.
  * Other distributions may work, but none have been officially tested so far.
* **Container Runtime:** Podman (as provided by the operating system)
  * Required for running the customization process.
* **Virtualization Stack:** QEMU / Libvirt.
  * Required for provisioning the Kubernetes nodes. This can reside on a different host than the one used for creating the bootable artifacts.

# Key Concepts

UC is an initiative to build a tightly integrated, opinionated and thoroughly validated stack of core components that serve as the foundation layer, or the lowest common denominator, of what is deployed as the underlying infrastructure platform for SUSE solutions that use RKE2.

In contrast with traditional stacks where updates are managed as individual upgrades, separated by layers, in UC all updates and lifecycles are done top-down, from the container to the OS, and are designed to be reproducible and deterministic.

## Components

 Let’s now cover these in more detail, focusing on their specific role in the infrastructure stack.

### Operating System

The operating system used by UC is designed to be an evolution of the work pioneered with Rancher OS Management. **It is built, maintained and delivered in a container image** (OCI) format using SLES 16.0 as its source and foundation.

The design is specifically tailored to container management scenarios, regardless of whether that involves Kubernetes orchestration or is subject to constrained environments where Podman is the only available tool. This involves immutability, security hardening, and minimalist footprint.

Due to its nature, the operating system does not come with a standard package manager (e.g. zypper). Installation and updates are done using container native workflows and artifacts, not standard system management processes. This keeps “pristinity” as one of its core concepts, prevents tampering, and allows for seamless, atomic, containerized-based updates that favor repeatability.

### Kubernetes

Kubernetes is prevalent in this day and age, and its popularity is only going to grow with time. **UC bundles RKE2 as an enterprise-ready Kubernetes distribution**, that inherits all the benefits of K3s and delivers additional hardening, compliance, and advanced networking capabilities. As part of UC, it is built and delivered as a [*systemd extension*](https://www.freedesktop.org/software/systemd/man/latest/systemd-sysext.html), but can be customized just like any normal RKE2 installation.

### Load Balancing & High Availability

UC uses MetalLB and Endpoint Copier Operator in order to establish reliable load balancing capabilities for the Kubernetes API. This allows for Kubernetes nodes **to form a cluster** **in a secure and fully automated manner,** and for API functionality to be maintained in the event of a node outage, or during lifecycle operations.

### Elemental 3

UC bootstrapped the development of the next generation of Elemental by adding the innovative approach to converge the operating system and Kubernetes distribution. The tooling provides the following capabilities now, and a lot more are expected in the future:

* **Installing and upgrading operating systems.** Building on the success of Elemental 2, Elemental 3 includes and has full control over the installation and upgrade mechanisms for the target systems, ensuring these operations are smooth and fully validated.
* **Customizing prebuilt artifacts.** Elemental 3 has the capability to consume the operating system being delivered as container images and provide the respective ISO and RAW disk installers, vastly simplifying the deployment experience.
* **Customizing prebuilt artifacts.** We consider the live installers as a key deliverable built in-house by the UC team. This is where we use the live installers as a source to customize and tailor bootable artifacts for our consumers. The resulting images may embed Kubernetes, advanced network settings, custom scripts and more.
* **Aligning all of it together.** As you can see, there are a plethora of moving parts and components in the picture. Elemental 3 is the key which allows for a set of components to be pinned and deployed together, covering a wide range of deployment paradigms and scenarios.

## Validation

Finally, one of the most important aspects of UC is that **it includes comprehensive testing and validation between all components** by default. Every release ensures that various different deployment scenarios have been verified and confirmed to be successful across Kubernetes cluster topology, system architecture, hardware variability and more.

# Environment Setup

This section details the steps required to prepare your host system by installing necessary packages, and retrieving the core project files.

## Root Usage

The instructions in this guide assume privileged access (sudo) on the customization host.

Rootless execution is *possible*, but not extensively tested yet. If you want to proceed with it, ensure that:

* Rootless Podman socket is enabled  (via `systemctl --user enable --now podman.socket` which provides `${XDG_RUNTIME_DIR}/podman/podman.sock` on a Linux system).
* All invocations in this cookbook are modified to use `${XDG_RUNTIME_DIR}/podman/podman.sock` instead of `/run/podman/podman.sock.`
* SELinux is in *Permissive* mode. (Use `sestatus` to check the currently active mode).
* setuid and setgid are configured appropriately for your user (See
  [https://rootlesscontaine.rs/getting-started/common/subuid/](https://rootlesscontaine.rs/getting-started/common/subuid/) for details)


## Install Required Packages and Enable Services

Install the necessary dependencies for container management, version control and virtualization, and enable the respective services.

**On the customization host**, execute the following commands:

```shell
sudo zypper install podman \
   git-core
sudo systemctl enable --now podman.socket
```

**On the hypervisor host**, execute the following commands:

```shell
sudo zypper install qemu \
   libvirt \
   virt-install
sudo systemctl enable --now libvirtd
```

**Note:** You have to install all packages and enable all services above if you are using the same host for both customization processes and virtual machine management.

## Clone the Elemental Repository

The [SUSE/Elemental](https://github.com/SUSE/elemental) repository on GitHub serves as the primary location for UC development. It is constantly being updated with the latest examples that will be leveraged in this guide.

Clone the repository to your host system. The examples directory within will be referenced throughout this guide from this point onward.

```shell
mkdir ${HOME}/elemental-cookbook/

git clone --depth 1 \
-b main \
https://github.com/SUSE/elemental.git \
${HOME}/elemental-cookbook/elemental

export ELEMENTAL_PATH="${HOME}/elemental-cookbook/elemental/examples/elemental/customize"
```

## Fetch the Elemental Container Image

Elemental 3 is packaged and delivered in a container image. We will use Podman to fetch the latest version as a final step to the environment setup stage.

```shell
export ELEMENTAL_IMAGE="registry.suse.com/beta/uc/elemental3:latest"

sudo podman pull ${ELEMENTAL_IMAGE}
```

If you receive a certificate validation error, make sure to install the SUSE IBS CA certificates first.

# **Customization Process**

## Example Templates

Before proceeding, let's explore the available configuration templates provided by the Elemental repository and describe what each one entails. These templates cover various deployment use cases which closely align with the recipes in this guide.

You can list the contents of the customization examples directory using the previously defined `$ELEMENTAL_PATH` variable:

```shell
$ ls -1 ${ELEMENTAL_PATH}

linux-only
multi-node
single-node
```

These directories cover the following use cases:

* **“Linux-only”.** This example showcases how to produce a bootable Linux artifact **not** affiliated with Kubernetes deployments at all, demonstrating the base OS capabilities of UC.
* **Single-node clusters.** This is the foundational example, producing a bootable artifact used for bootstrapping a Kubernetes cluster consisting of a single node.
* **Multi-node clusters.** Closely resembles the example above, producing a bootable artifact used for bootstrapping a Kubernetes cluster consisting of multiple nodes.

## Example Overview

Going further, we will explain what an example from the templates above looks like which is going to touch a few key points around what UC allows. The one we will focus on in this section is the single-node cluster, as it forms the basis for the majority of the subsequent recipes.

Let’s examine the contents for this example:

```shell
tree -U ${ELEMENTAL_PATH}/single-node/

single-node/
├── butane.yaml
├── install.yaml
├── kubernetes.yaml
├── kubernetes
│   ├── config
│   │    └── server.yaml
│   ├── helm
│   │    └── values
│   │          └── rancher.yaml
│   └── manifests
│        ├── ip-pool.yaml
│        ├── l2-adv.yaml
│        └── rke2-ingress-config.yaml
├── network
│    └── single-node-example.yaml
├── release.yaml
└── suse-product-manifest.yaml

6 directories, 11 files
```

This output might be confusing and you might be wondering how to read through it. Let’s examine it together\!

* **butane.yaml:** This is where you are able to provide firstboot configuration settings such as users, SSH keys, and system files. The interface language is [Butane](https://coreos.github.io/butane/), which implies that the operating system uses Ignition as its configuration mechanism. The data provided in this file is merged with the one generated by Elemental 3\.
* **install.yaml**: The installation customization for the operating system. This is where you are able to define kernel parameters, crypto policy, target installation device and others.
* **kubernetes.yaml:** This is where you define the number and type of nodes that will form a Kubernetes cluster, alongside workloads in the form of plain manifests and Helm charts.
* **kubernetes/**: This subdirectory allows you to provide RKE2 configurations for cluster nodes, Helm chart customization options (such as values files), as well as additional local Kubernetes manifests.
* **network/**: This subdirectory allows you to provide advanced network settings that can span multiple machines or specialized scripts that are necessary for configuring machines in highly specific use cases.
* **release.yaml:** This is the “bread and butter” capability that Elemental 3 and UC provide. This is where you are able to specify the version of UC or any solution built on top of it, as well as all the components that will be enabled in the final artifact. The components range from solely RKE2 to specialized operating system extensions or key Kubernetes workloads (e.g. NVIDIA GPU Operator for SUSE AI).
* **suse-product-manifest.yaml**: This file serves as an example description, that showcases how any product or solution can use UC as a base, and add any additional components on top of it. You can learn more about the “release manifest” concept [in the repository documentation.](https://github.com/SUSE/elemental/blob/main/docs/release-manifest.md)

# **Recipe 1: Single-node Kubernetes cluster**

## Preparation

Familiarize yourself with the contents within the `single-node/` example directory, and you will notice the following configurations:

* User `root` defined (with ‘`linux`’ as a hashed password value)
* RAW disk size set to 35 GB
* FIPS mode enforced
* RKE2 enabled
* MetalLB and Rancher enabled (from UC and example solution releases respectively)
* NeuVector and Local Path Provisioner enabled (as additional Kubernetes workloads)

Feel free to adjust these values as you see fit, e.g. by adding an SSH key or another user, writing a file on the filesystem, adjusting a Rancher Helm chart value or anything else\!

## Execution

Let’s customize a RAW disk image:

```shell
sudo podman run -it --rm\
--network host \
-v ${ELEMENTAL_PATH}/single-node:/config:Z \
-v /run/podman/podman.sock:/var/run/docker.sock \
${ELEMENTAL_IMAGE} customize --type raw
```


**Note:** The Podman socket from the host is mounted to the Docker socket within the container. This is expected as a large majority of the artifacts that Elemental 3 fetches and works with are
container images, i.e. it will fetch and unpack data from various OCI registries as a source.

**Note:** The Podman socket path on the host is not `/run/podman/podman.sock` if you are using Podman as a regular user with a regular rootless setup. See earlier note in [Environment Setup](#environment-setup) on how to leverage a rootless Podman execution.

As soon as the process completes, you will find your image alongside its SHA256 checksum within the `$ELEMENTAL_PATH/single-node` directory.

```shell
du -hs ${ELEMENTAL_PATH}/single-node/*.raw*

1.2G    /home/xxx/elemental-cookbook/elemental/examples/elemental/customize/single-node/image-2026-01-12T11-06-06.raw
4.0K    /home/xxx/elemental-cookbook/elemental/examples/elemental/customize/single-node/image-2026-01-12T11-06-06.raw.sha256
```

By default the naming scheme is in the following format: `image-<timestamp>.<type>`. You are able to provide your own using the \--output flag in the command above.

The resulting image contains the necessary partition table and contents to boot into a live installer, which will create the initial snapshot, apply the relevant settings and lead into the first boot of the system, where Ignition and additional configurations will be applied.

## Deployment

You’ll use libvirt to create a virtual machine using the built artifact as the disk. The network interface is specifically selected to match the network settings supplied for it under the `network/` configuration directory.

Feel free to adjust the RAM and vCPUs setting to fit your system. If you need to heavily reduce these, please look into disabling Rancher or other Kubernetes resources.

```shell
sudo virt-install --name uc-single-node \
     	            --ram 16000 \
                  --vcpus 10 \
                  --disk path="$(ls -1 ${ELEMENTAL_PATH}/single-node/*.raw)",format=raw \
                  --osinfo detect=on,name=sle-unknown \
                  --graphics none \
                  --console pty,target_type=serial \
                  --network network=default,model=virtio,mac=FE:C4:05:42:8B:AB \
                  --virt-type kvm \
                  --import \
                  --boot uefi,loader=/usr/share/qemu/ovmf-x86_64-ms-code.bin,nvram.template=/usr/share/qemu/ovmf-x86_64-ms-vars.bin
```

After a few seconds, a new VM will be created, booted and the terminal will be connected to the console (to disconnect from the console you can use *ctrl+\]*). You should see the VM booting.

Then the installation will happen. The system will be auto-logged as root and you can see the progress live via *journalctl \-f*.

As soon as the installation and first boot stages conclude, you will be prompted with the login screen where you can use the credentials specified above (root/linux, if you have not modified the base example) to access the system.

Within a couple of minutes, not only RKE2 but Rancher, NeuVector and MetalLB will all have been automatically deployed and running. The logs can be observed using *journalctl \-f* as the root user.

Feel free to verify this yourself by following these instructions:

```shell
export KUBECONFIG=/etc/rancher/rke2/rke2.yaml
ln -s /var/lib/rancher/rke2/data/*/bin/kubectl /root/bin/kubectl

export CRI_CONFIG_FILE=/var/lib/rancher/rke2/agent/etc/crictl.yaml
ln -s /var/lib/rancher/rke2/bin/crictl /root/bin/crictl

crictl ps
kubectl get pods -A
```

**NOTE:** Creating the symlinks and the exports can be done using the current customization capabilities of UC, it is left as an exercise to the reader.

With this, the first recipe comes to an end\! Let’s move over to the more complex scenarios.

## Cleanup

Optionally, you can remove the virtual machine provisioned for this scenario:

```shell
sudo virsh destroy uc-single-node
sudo virsh undefine uc-single-node --nvram

# if you want to destroy both the VM and the raw image:
sudo virsh undefine uc-single-node --nvram --remove-all-storage
```

# **Recipe 2: Multi-node Kubernetes cluster**

## Preparation

Familiarize yourself with the contents within the `multi-node/` example directory, and you will find the following configurations:

* User `root` defined (with ‘`linux`’ as a hashed password value)
* RAW disk size set to 35 GB
* FIPS mode enforced
* Network settings for 4 hosts configured
* RKE2 enabled
* Multiple nodes (3 servers and 1 agent), and virtual IP address configured
* MetalLB and Rancher enabled (from UC and example solution releases respectively)
* NeuVector and Local Path Provisioner enabled (as additional Kubernetes workloads)

As always, feel free to adjust these values as you see fit, e.g. by adding an SSH key or another user, writing a file on the filesystem, adjusting a Rancher Helm chart value or anything else\!

## Execution

Let’s customize a RAW disk image:

```shell
sudo podman run -it \
--network host \
-v ${ELEMENTAL_PATH}/multi-node:/config \
-v /run/podman/podman.sock:/var/run/docker.sock \
${ELEMENTAL_IMAGE} customize --type raw
```

Note that the resulting image in this case will have to be copied several times, so that you are able to create as many virtual machines as the number of preconfigured Kubernetes nodes.

```shell
for i in {1..4}; do cp ${ELEMENTAL_PATH}/multi-node/image*.raw ${ELEMENTAL_PATH}/multi-node/node${i}.example.com.raw; done
```

## Deployment

You will use a similar format for the virtual machine deployment as the one above. Pay close attention to the network interface specification, as that will result in each machine applying the corresponding settings that we preconfigured the image for. The Kubernetes node type is determined by the hostname initially supplied. As soon as the initializer system is up and running, it will form a Kubernetes cluster and all other systems will continuously attempt to connect until successfully joining.

As always, feel free to adjust the RAM and vCPUs setting to fit your system. If you need to heavily reduce these, please look into disabling Rancher or other Kubernetes resources.

```shell

sudo virt-install --name node1.example.com \
             --ram 16000 \
             --vcpus 10 \
             --disk path="${ELEMENTAL_PATH}/multi-node/node1.example.com.raw",format=raw \
             --osinfo detect=on,name=sle-unknown \
             --graphics none \
		 --noautoconsole \
             --console pty,target_type=serial \
             --network network=default,model=virtio,mac=FE:C4:05:42:8B:AB \
             --virt-type kvm \
             --import \
             --boot uefi,loader=/usr/share/qemu/ovmf-x86_64-ms-code.bin,nvram.template=/usr/share/qemu/ovmf-x86_64-ms-vars.bin

sudo virt-install --name node2.example.com \
             --ram 16000 \
             --vcpus 10 \
             --disk path="${ELEMENTAL_PATH}/multi-node/node2.example.com.raw",format=raw \
             --osinfo detect=on,name=sle-unknown \
             --graphics none \
		 --noautoconsole \
             --console pty,target_type=serial \
             --network network=default,model=virtio,mac=FE:C4:05:42:8B:AC \
             --virt-type kvm \
             --import \
             --boot uefi,loader=/usr/share/qemu/ovmf-x86_64-ms-code.bin,nvram.template=/usr/share/qemu/ovmf-x86_64-ms-vars.bin

sudo virt-install --name node3.example.com \
             --ram 16000 \
             --vcpus 10 \
             --disk path="${ELEMENTAL_PATH}/multi-node/node3.example.com.raw",format=raw \
             --osinfo detect=on,name=sle-unknown \
             --graphics none \
		 --noautoconsole \
             --console pty,target_type=serial \
             --network network=default,model=virtio,mac=FE:C4:05:42:8B:AD \
             --virt-type kvm \
             --import \
             --boot uefi,loader=/usr/share/qemu/ovmf-x86_64-ms-code.bin,nvram.template=/usr/share/qemu/ovmf-x86_64-ms-vars.bin

sudo virt-install --name node4.example.com \
             --ram 16000 \
             --vcpus 10 \
             --disk path="${ELEMENTAL_PATH}/multi-node/node4.example.com.raw",format=raw \
             --osinfo detect=on,name=sle-unknown \
             --graphics none \
		 --noautoconsole \
             --console pty,target_type=serial \
             --network network=default,model=virtio,mac=FE:C4:05:42:8B:AE \
             --virt-type kvm \
             --import \
             --boot uefi,loader=/usr/share/qemu/ovmf-x86_64-ms-code.bin,nvram.template=/usr/share/qemu/ovmf-x86_64-ms-vars.bin

```

As soon as all nodes are up and running, you should be seeing a healthy multi-node cluster:

```shell

virsh list
 Id   Name                State
-----------------------------------
 1    node1.example.com   running
 2    node2.example.com   running
 3    node3.example.com   running
 4    node4.example.com   running
```

Verify this yourself by connecting to the console of one of the nodes.

```shell
virsh console node1.example.com
...
kubectl get nodes
```

On to the final recipe in this guide\!

## Cleanup

Optionally, you can remove the virtual machines provisioned for this scenario:

```shell
for i in {1..4}; do
sudo virsh destroy node${i}.example.com
sudo virsh undefine node${i}.example.com --nvram

# if you want to destroy both the VM and the raw image:
sudo virsh undefine node${i}.example.com --nvram --remove-all-storage
done
```

# **Recipe 3: Single-node Kubernetes Cluster using Cluster API**

## Preparation

As you have seen so far, the examples above always include Kubernetes within the image as well as Ignition configurations, network settings and more. This is, however, not going to work for deployments leveraging [Cluster API](https://cluster-api.sigs.k8s.io/) where many (if not all) of these configurations have to come from the respective Cluster API provider. These providers perform all the bootstrapping and infrastructure provisioning steps.

UC leverages Cluster API Provider for RKE2 (CAPRKE2) as our bootstrap and control plane provider.

For the infrastructure provider there are various choices, for example [Cluster API Provider Metal3](https://book.metal3.io/capm3/introduction.html) (CAPM3), which is used in the [SUSE Telco Cloud](https://documentation.suse.com/suse-edge/3.4/html/edge/atip.html) solution. Luckily, there is a simplified demo environment4 provided by Telco Cloud that you can use to mock this provider, and use virtual machines even here. A [demo environment setup](https://github.com/suse-edge/metal3-demo) is maintained by the SUSE Telco team.

We will use the “Linux-only” example as we need to *split* the responsibilities between OS management and Kubernetes installation.

In order to satisfy the infrastructure provider (CAPM3), we will need to set the respective Ignition provider in the `linux-only/install.yaml` file. This is where **you will have to include** the `ignition.platform.id=openstack` value within the `kernelCmdLine` parameter as:

```
bootloader: grub
kernelCmdLine: "console=ttyS0 loglevel=3 ignition.platform.id=openstack"
raw:
 diskSize: 12G
```

That parameter instructs CAPM3 to look for the [Ignition](https://coreos.github.io/ignition/supported-platforms/) configuration within a specific partition on the disk, which serves as a config drive, labeled as *config-2*, included by the internal implementation of the components leveraged by this provider.

In case you were wondering, this is also how you can leverage other CAPI infrastructure providers which are expecting Ignition from a different source\!

Finally, the `diskSize` value can be lowered to reduce the transfer effort.

## Execution

Let’s customize the RAW disk image:

```shell
sudo podman run -it --network host \
-v ${ELEMENTAL_PATH}/linux-only:/config \
-v /run/podman/podman.sock:/var/run/docker.sock \
${ELEMENTAL_IMAGE} customize --type raw --mode split
```

**Note:** The new “`--mode split”` flag *splits* and saves the configuration that would normally be embedded as an Ignition partition within the image, as a separate directory.

This effectively means that the bootable artifact now only contains the operating system, and the applied installation configurations, e.g. the kernel command line and disk size.

```shell
tree -U ${ELEMENTAL_PATH}/linux-only
...
├── image-2026-01-14T16-44-43-config      <- The config folder and assets
│   ├── catalyst
│   │   └── network
│   │       └── example-libvirt.yaml
│   └── ignition
│       └── config.ign
├── image-2026-01-14T16-44-43.raw         <- the OS only image
└── image-2026-01-14T16-44-43.raw.sha256  <- the SHA256 of the image above
```

**Note \#2**: The “`--mode split`” option may also be used to customize an image, and have its respective configuration intentionally separate. By burning the `image-xxx-config` directory as an ISO with an “IGNITION” label (e.g. by using `mkisofs` or `xorrisofs`), we can follow a deployment process largely similar to plain [SUSE Linux Micro today](https://documentation.suse.com/sle-micro/6.1/html/Micro-deployment-selfinstall-images/index.html#deployment-preparing-configuration-device).

## Deployment

Every infrastructure provider is different, but for CAPM3 we are using a `Metal3MachineTemplate` resource to specify the paths for the image and its respective checksum, as received from the Elemental 3 customization process.

Note: The image generated above needs to be specified on the `Metal3MachineTemplate` as [.spec.template.image.url](https://doc.crds.dev/github.com/metal3-io/cluster-api-provider-metal3/infrastructure.cluster.x-k8s.io/Metal3MachineTemplate/v1beta1@v1.3.0#spec-template-spec-image-url) and must then be made available via a webserver, either the media-server container enabled via the Metal3 chart (see [the SUSE Edge docs for more information](https://documentation.suse.com/suse-edge/3.4/html/edge/atip-management-cluster.html#metal3-media-server)) or some other locally accessible server.

Then the final piece to the puzzle is the `RKE2ControlPlane` template, where we will supply additional Butane configuration that will be merged with the one CAPRKE2 generates for installing RKE2.

```
 agentConfig:
   format: ignition
   kubelet:
     extraArgs:
       - provider-id=metal3://BAREMETALHOST_UUID
   additionalUserData:
     config: |
       variant: flatcar
       version: 1.4.0
       systemd:
         units:
           - name: rke2-preinstall.service
             enabled: true
             contents: |
               [Unit]
               Description=rke2-preinstall
               Wants=network-online.target
               Before=rke2-install.service
               ConditionPathExists=!/run/cluster-api/bootstrap-success.complete
               [Service]
               Type=oneshot
               User=root
               ExecStartPre=/bin/sh -c "mount -L config-2 /mnt"
               ExecStart=/bin/sh -c "sed -i \"s/BAREMETALHOST_UUID/$(jq -r .uuid /mnt/openstack/latest/meta_data.json)/\" /etc/rancher/rke2/config.yaml"
               ExecStart=/bin/sh -c "echo \"node-name: $(jq -r .name /mnt/openstack/latest/meta_data.json)\" >> /etc/rancher/rke2/config.yaml"
               ExecStartPost=/bin/sh -c "umount /mnt"
               [Install]
               WantedBy=multi-user.target
       storage:
         filesystems:
           - path: /opt
             device: "/dev/disk/by-partlabel/SYSTEM"
             format: btrfs
             wipe_filesystem: false
             mount_options:
               - "subvol=/@/opt"
```

Some of these fields are definitely *magic* but future iterations of UC will allow us to integrate in a much simpler way… so stay tuned!

This is definitely an advanced use case so don’t hesitate to reach out to us if the above is unclear. We will gladly help you!
