# Deploying Elemental on Harvester

> **NOTE:** This document is a platform-specific extension of [Deploying Elemental on the Cloud](cloud-enablement.md). It only covers what is different or additional for [Harvester](https://harvesterhci.io/). Read the generic document first, as everything described there also applies here.

Harvester runs Elemental nodes as KubeVirt virtual machines. The workflow is the same as for any other cloud provider:

| Generic step | On Harvester | Section |
|---|---|---|
| Prepare infrastructure resources | Prepare a VLAN and, optionally, a load balancer | [Infrastructure Resources Preparation](#infrastructure-resources-preparation) |
| Build a customized image | Build a RAW image and upload it as a `VirtualMachineImage` | [Preparing the Image](#preparing-the-image) |
| Upload the image and spin up machines | Create a Secret, a PersistentVolumeClaim and a VirtualMachine per node | [Spinning the Elemental Machines](#spinning-the-elemental-machines) |

There are two deployment methods:

* **Disk image** (default) - A RAW or qcow2 disk image is uploaded to Harvester and used as the node's disk. This is the flow described in the main sections of this document.
* **ISO installer** - An ISO is published as a containerDisk and attached to each virtual machine to install the system onto an empty disk. It only differs from the disk image flow in a few steps, see [Alternative: Installing from an ISO](#alternative-installing-from-an-iso).

## Prerequisites

This document has been validated with Elemental v3.1 and Harvester v1.8.2. Older Elemental versions cannot receive additional Ignition configuration at
runtime, hence Harvester is considered an unsupported platform before Elemental v3.1.

## Infrastructure Resources Preparation

> Complements [Infrastructure Resources Preparation](cloud-enablement.md#infrastructure-resources-preparation).

Prepare the following in Harvester before building the image:

1. A VLAN network that the Elemental virtual machines will be attached to.
1. [Optional] A load balancer that forwards traffic to the RKE2 API (6443) and supervisor (9345) on each of the cluster nodes.

> **NOTE:** If no external load balancer is available, MetalLB can be run on the virtual machines instead. See [Preparing the Image](#preparing-the-image).

## Preparing the Image

> Complements [Preparing the Image](cloud-enablement.md#preparing-the-image).

### Configuration Directory

Create the [configuration directory](configuration-directory.md) following the requirements of the [generic document](cloud-enablement.md#preparing-the-image). In `kubernetes/cluster.yaml`, set the `network` section according to the infrastructure prepared in the previous section.

The `install.yaml` file must include the Ignition platform identifier. This is done by adding an extra parameter to the kernel command line. Add
`ignition.platform.id=openstack` to the `kernelCmdLine` field in `install.yaml`, see the example below:

```yaml
schema: v0
bootloader: grub
kernelCmdLine: "console=ttyS0 quiet loglevel=3 ignition.platform.id=openstack"
iso:
  device: /dev/vda # Only used by the ISO. The device the ISO installs to; the kernel usually names virtio disks as /dev/vd[a-z]
```

With the `openstack` platform, Ignition looks for an attached volume labeled `config-2`. Harvester can automatically attach such a volume from a
Kubernetes Secret, see [Ignition Secret](#ignition-secret).

#### Using MetalLB

If no external load balancer is available, set `network.apiVIPMode: managed` in `kubernetes/cluster.yaml`, overriding the generic guide's `external` setting, and set `network.apiVIP` to an unused address on the VM VLAN. This enables MetalLB and the endpoint-copier operator and generates the manifests for the cluster API virtual IP. If ingress load balancing is also needed, include the ingress IP pool and L2 advertisement from the [multi-node example](../examples/elemental/customize/multi-node/), adapting their addresses to the VLAN defined in [Infrastructure Resources Preparation](#infrastructure-resources-preparation).

### Building and Uploading the Image

Build the customized image as a disk (RAW or qcow2), following the [customization process](image-customization.md#customization-process).

> **NOTE:** To install from an ISO instead of a disk image, see [Alternative: Installing from an ISO](#alternative-installing-from-an-iso).

Then upload it to Harvester by creating a `VirtualMachineImage` resource. See the example:

```yaml
apiVersion: harvesterhci.io/v1beta1
kind: VirtualMachineImage
metadata:
  name: custom-image
  namespace: elemental
spec:
  displayName: <name displayed in the UI>
  sourceType: download
  url: <image URL>
```

Adapt the following fields to your environment:

* `metadata.name` - Name of the image resource. It is also used to retrieve the generated `storageClassName` below.
* `metadata.namespace` - Namespace where the image is created. Use the same namespace for all the resources of the deployment.
* `spec.displayName` - Name shown in the Harvester UI.
* `spec.url` - URL that the Harvester cluster downloads the image from. It must be reachable from Harvester itself, see [Making the Image Reachable](#making-the-image-reachable).

#### Making the Image Reachable

With `sourceType: download`, Harvester fetches the image from the given `url`. If you do not have a place to host the image, you can serve it from your workstation and expose it temporarily with a [cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/do-more-with-tunnels/trycloudflare/) quick tunnel, which creates a public, transient URL with a random hostname and requires no Cloudflare account.

1. Serve the directory that contains the image over local HTTP. Use a directory holding only the image:

    ```shell
    cd <directory containing the image>
    python3 -m http.server 8080
    ```

1. In another terminal, start the tunnel pointing at the local service:

    ```shell
    cloudflared tunnel --url http://localhost:8080
    ```

1. Cloudflared prints a URL like `https://<random-words>.trycloudflare.com`. Use it in `spec.url`, followed by the image file name, for example `https://<random-words>.trycloudflare.com/<image-file>`.
1. Once the `VirtualMachineImage` is reported as imported, stop both processes. The URL stops working and a new one is generated on each run.

> **WARNING:** Anyone who knows the URL can download the image for as long as the tunnel is running. Make sure the image does not contain secrets, and stop the tunnel as soon as the upload completes. This approach is meant for testing and development, not for production.

Once the image is uploaded, Harvester generates a StorageClass for it. Take note of its name (`storageClassName`), as it is needed when creating the node volumes. Get it with:

```bash
kubectl get virtualmachineimage custom-image -n elemental -o jsonpath='{.status.storageClassName}{"\n"}'
```

## Spinning the Elemental Machines

> Complements [Spinning the Elemental Machines](cloud-enablement.md#spinning-the-elemental-machines).

Each node requires three resources, all created in the same namespace:

1. A Secret that carries the ignition configuration.
1. A PersistentVolumeClaim that holds the node's disk.
1. A KubeVirt VirtualMachine that ties the previous two together with the VLAN.

### Ignition Secret

Create an `Opaque` Kubernetes Secret with `stringData.userdata` set to the literal ignition configuration. As described in [Runtime Ignition Creation](cloud-enablement.md#runtime-ignition-creation), the configuration must define the node's hostname and node type (`runtime.env`).

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: node1-ignition
  namespace: elemental
type: Opaque
stringData:
  userdata: |
    Here goes the literal ignition JSON setting the hostname, NODETYPE and, optionally, IS_INIT_NODE variables
```

See the [Creating the Configuration](cloud-enablement.md#creating-the-configuration) section for instructions on how to generate the ignition configuration.

Adapt the following fields to your environment:

* `metadata.name` - Name of the Secret. It is referenced by the VirtualMachine (`secretRef.name`), so keep them in sync.
* `metadata.namespace` - Namespace of the deployment, the same as the rest of the node resources.
* `stringData.userdata` - The ignition configuration of this specific node: its hostname, `NODETYPE` and, for the initializer server, `IS_INIT_NODE=true`. This is different for each node.

### Volume Claim

Create a PersistentVolumeClaim that references the `storageClassName` generated when the image was uploaded. Set the requested size to the expected disk size of the node, which must be bigger than the uploaded image.

> **NOTE:** When installing from an ISO the volume claim is different, see [Volume Claim for the ISO](#volume-claim-for-the-iso).

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: node1-disk-0
  namespace: elemental
  annotations:
    harvesterhci.io/imageId: elemental/custom-image
spec:
  accessModes:
    - ReadWriteMany
  volumeMode: Block
  storageClassName: <storageClassName from the custom-image>
  resources:
    requests:
      storage: 50Gi
```

Adapt the following fields to your environment:

* `metadata.name` - Name of the volume claim. It is referenced by the VirtualMachine (`claimName`).
* `metadata.namespace` - Namespace of the deployment, the same as the rest of the node resources.
* `metadata.annotations.harvesterhci.io/imageId` - Reference to the uploaded image in `<namespace>/<name>` format, matching the `VirtualMachineImage` created before.
* `spec.storageClassName` - The StorageClass generated for the uploaded image, as retrieved in [Building and Uploading the Image](#building-and-uploading-the-image).
* `spec.resources.requests.storage` - Disk size of the node. It must be bigger than the uploaded image.

### Virtual Machine

Create the KubeVirt VirtualMachine resource, referencing the VLAN network, the volume claim and the ignition secret created above.

```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: node1
  namespace: elemental
spec:
  runStrategy: RerunOnFailure
  template:
    metadata:
      labels:
        harvesterhci.io/vmName: node1
    spec:
      domain:
        cpu:
          cores: 4
          sockets: 1
          threads: 1
        memory:
          guest: 8Gi
        resources:
          limits:
            cpu: "4"
            memory: 8Gi
        firmware:
          bootloader:
            efi:
              secureBoot: false
        devices:
          disks:
            - name: disk-0
              bootOrder: 1
              disk:
                bus: virtio
            - name: cloudinitdisk
              disk:
                bus: virtio
          interfaces:
            - name: default
              bridge: {}
              model: virtio
      networks:
        - name: default
          multus:
            networkName: elemental/elemental-vlan
      volumes:
        - name: disk-0
          persistentVolumeClaim:
            claimName: node1-disk-0
        - name: cloudinitdisk
          cloudInitConfigDrive:
            secretRef:
              name: node1-ignition
```

Adapt the following fields to your environment:

* `metadata.name` and `spec.template.metadata.labels.harvesterhci.io/vmName` - Name of the virtual machine. Both must match.
* `metadata.namespace` - Namespace of the deployment, the same as the rest of the node resources.
* `spec.template.spec.domain.cpu`, `domain.memory` and `domain.resources.limits` - CPU and memory sizing of the node. Keep the `limits` consistent with the CPU and memory values.
* `spec.template.spec.networks[].multus.networkName` - The VLAN network prepared in [Infrastructure Resources Preparation](#infrastructure-resources-preparation), in `<namespace>/<name>` format.
* `spec.template.spec.volumes[].persistentVolumeClaim.claimName` - Must match the name of the volume claim of this node.
* `spec.template.spec.volumes[].cloudInitConfigDrive.secretRef.name` - Must match the name of the ignition Secret of this node.

> **NOTE:** The `firmware.bootloader.efi` setting is required, as Elemental images boot using EFI.

> **NOTE:** When installing from an ISO the virtual machine needs an additional volume, see [Virtual Machine for the ISO](#virtual-machine-for-the-iso).

### Organizing the Resources

The Secret, PersistentVolumeClaim and VirtualMachine of a node are tied to each other. It is good practice to keep them together in a single YAML file and to prepare one file per virtual machine:

```shell
kubectl apply -f node-1.yaml -f node-2.yaml -f node-3.yaml
```

## Alternative: Installing from an ISO

Instead of uploading a disk image, the system can be installed from an ISO. The ISO can be published as a [containerDisk](https://kubevirt.io/user-guide/storage/disks_and_volumes/#containerdisk) (an OCI image in a registry) and attached to each virtual machine, which installs the system onto an empty disk. A containerDisk suits
the installation ISO workflow as it is exposed to the VM as a non-persistent, ephemeral volume.

Everything not mentioned in this section is unchanged: [Infrastructure Resources Preparation](#infrastructure-resources-preparation), the [Configuration Directory](#configuration-directory), the [Ignition Secret](#ignition-secret) and [Organizing the Resources](#organizing-the-resources) work exactly as in the disk image flow. Do not create a `VirtualMachineImage` resource, as a containerDisk is used instead.

In this scenario the virtual machines are created with a persistent empty volume as the main drive, a containerDisk set as a cdrom drive and a cloudInitConfigDrive
set as an additional drive.

The ISO boots as a live cdrom, installs itself to the empty drive and reboots. Then, on the first boot of the installed system, Ignition runs the configuration stored in
the cloudInitConfigDrive. The containerDisk can be referenced simultaneously by multiple VMs.

### Building the ISO

Build the ISO with the same configuration directory, following the [customization process](image-customization.md#customization-process) and setting the `--type iso` flag.

### Publishing the ISO as a containerDisk

A containerDisk is a `scratch`-based OCI image that can be defined and built with the following simple Dockerfile:

```bash
cat << END > Dockerfile
FROM scratch
ADD --chown=107:107 nodes.iso /disk/
END

docker build -t example.registry.com/installer-iso:v1 .
docker push example.registry.com/installer-iso:v1
```

Adapt the name of the ISO file and of the image to your environment. The image must be published to a registry that Harvester can access.

### Volume Claim for the ISO

The volume claim is the same as in [Volume Claim](#volume-claim) with these changes:

* Remove the `harvesterhci.io/imageId` annotation, as there is no uploaded image. The volume starts empty.
* Set `spec.storageClassName` to a regular Harvester StorageClass such as `harvester-longhorn`, instead of the one generated for an image.
* `spec.resources.requests.storage` is now the disk size the system is installed on.

Following the previous example, the volume claim becomes:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: node1-disk-0
  namespace: elemental
spec:
  accessModes:
    - ReadWriteMany
  volumeMode: Block
  storageClassName: harvester-longhorn
  resources:
    requests:
      storage: 50Gi
```

### Virtual Machine for the ISO

The virtual machine is the same as in [Virtual Machine](#virtual-machine) with these additions:

* A containerDisk volume that references the published ISO image.
* A CD-ROM disk entry for that volume, with a boot order so that the ISO is used for the installation.

The volumes list including the containerDisk volume:

```yaml
# under spec.template.spec
volumes:
- name: disk-0
  persistentVolumeClaim:
    claimName: node1-disk-0
- name: iso-volume
  containerDisk:
    image: example.registry.com/installer-iso:v1
    imagePullPolicy: IfNotPresent
- name: cloudinitdisk
  cloudInitConfigDrive:
    secretRef:
      name: node1-ignition
```

The device list including the cdrom:

```yaml
# under spec.template.spec.domain
devices:
  disks:
  - name: disk-0
    bootOrder: 1
    disk:
      bus: virtio
  - name: iso-volume
    bootOrder: 2
    cdrom:
      bus: sata
  - name: cloudinitdisk
    disk:
      bus: virtio
```

Adapt the following fields to your environment:

* `spec.template.spec.volumes[].containerDisk.image` - Reference of the ISO containerDisk image published in [Publishing the ISO as a containerDisk](#publishing-the-iso-as-a-containerdisk).
