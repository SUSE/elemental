# Image Customization

This section provides an overview of how the `elemental3` command-line interface enables users to customize and extend an image that is based on a specific set of components defined in a [release manifest](release-manifest.md).

For general information about the customization process, refer to the [Customization Process](#customization-process) section.

For information on how to boot a customized image, refer to the [Booting a customized image](#booting-a-customized-image) section.

For real-life examples, refer to the deployment scenario examples in the [Cookbook](../docs/cookbook-first-time-use.md#scope-and-audience) documentation. 

## Customization Process

The customization process is executed through the `elemental3 customize` command.

As part of the command, users are expected to provide:

1. **Specifications for the image** - these are defined as options to the command itself. Available specification options can be viewed by calling the command's help message: `elemental3 customize -h`.
2. **Information about the desired state of the image** - this is done through the definition of a configuration directory that the user creates beforehand. For more information on the directory itself, refer to the [Configuration Directory Guide](configuration-directory.md).

Please familiarize yourself with both points before attempting to customize your first image.

For further understanding of the customization process, you can also refer to the following sections:

- [Limitations](#limitations) - for any limitations that the process might have.
- [Overview](#overview) - for a high-level description of the steps that the customization process goes through.
- [Execution](#execution) - for supported methods to trigger the customization process.

### Limitations

Currently, the image customization process has the following limitations:

1. Supports customizing images only for `x86_64` platforms.
1. Supports customizing images for connected (non air-gapped) environments only.

Elemental is in active development, and these limitations **will** be addressed as part of the product roadmap.

### Overview

> **NOTE**: The user is able to customize Linux-only images by **excluding** Kubernetes resources and deployments from the configuration directory (regardless of whether this is under `kubernetes/manifests`, `kubernetes/cluster.yaml` or `release.yaml`). This is currently an implicit process, but it is possible that an explicit option for it (e.g. a flag) is added at a later stage.

This section provides a high-level overview of the steps that Elemental's tooling goes through in order to produce a customized and extended image.

*Steps:*
1. Parse the user provided image specifications.
1. Parse the configuration directory that the user has defined in the image specification.
1. Parse the [solution release manifest](release-manifest.md#solution-release-manifest) that the user has defined as a [release reference](configuration-directory.md#solution-release-reference) in the `release.yaml` file of the configuration directory.
1. Pull and parse the [core platform release manifest](release-manifest.md#core-platform-release-manifest) that the aforementioned solution manifest extends.
1. Begin the customization process:
   1. Unpack the pre-built installer ISO that is defined in the `core platform` release manifest.
   1. Prepare for Kubernetes cluster creation and resource deployment:
      1. Prepare Helm charts and Kubernetes manifests
      1. Download RKE2 artifacts (tarball, images, checksums) and install script for air-gapped installation.
   1. Prepare any other overlays or firstboot configurations based on what was defined in the configuration directory.
   1. Produce a description of the desired installation state and merge it with the installer ISO description.
   1. Produce the final desired image type.
1. Mark customization as completed.

![image](images/customize-process.png)

### Execution

Starting the customization process can be done either by directly working with the `elemental3` binary, or by using the `elemental3` container image. Below you can find the minimum set of options for running both use cases.

#### Binary

```shell
sudo elemental3 customize --type <raw/iso> --config-dir <path>
```

> **IMPORTANT:** The above process is long running, as it involves pulling multiple component images over the network.
> For a quicker execution, use locally pulled container images in combination with the `--local` flag.

Unless configured otherwise, after execution, the resulting ready-to-boot image will reside in the configuration directory path and use the `image-<timestamp>.<image-type>` naming format.

> **NOTE:** You can specify another path for the output using the `--output (-o)` option, however, be mindful if running Elemental 3 from a container,
> as it would require including the mounted configuration directory as a prefix (e.g. --output /config/<desired-path>).

#### Container image

> **NOTE:** This section assumes you have pulled the `elemental3` container image and referenced it in the `ELEMENTAL_IMAGE` variable.

Run the customization process:

* For a RAW disk image:
   ```shell
   podman run -it -v <PATH_TO_CONFIG_DIR>:/config $ELEMENTAL_IMAGE customize --type raw
   ```

* For ISO media:
   ```shell
   podman run -it -v <PATH_TO_CONFIG_DIR>:/config $ELEMENTAL_IMAGE customize --type iso
   ```

> **IMPORTANT:** The above process is long running, as it involves pulling multiple component images over the network.
> For a quicker execution, use locally pulled container images in combination with mounting the podman socket to the `elemental3` container image and specifying the `--local` flag:
>
> 1. Start Podman socket:
>   ```shell
>   systemctl enable --now podman.socket
>   ```
> 2. Run the `elemental3` container with the mounted podman socket:
>   ```shell
>   podman run -it -v <PATH_TO_CONFIG_DIR>:/config -v /run/podman/podman.sock:/var/run/docker.sock $ELEMENTAL_IMAGE customize --type <raw/iso> --local
>   ```

Unless configured otherwise, the above process will produce a customized RAW or ISO image under the specified `<PATH_TO_CONFIG_DIR>` directory.

## Booting a customized image

> **NOTE:** The below RAM and vCPU resources are just reference values, feel free to tweak them based on what your environment needs.

The customized image can be booted as any other regular image. Below you can find an example of how this can be done by using Libvirt to setup a virtual machine from a customized image that has a static network configured for machines with the `FE:C4:05:42:8B:01` MAC address.

* RAW disk image:

   ```shell
   virt-install --name customized-raw \
               --ram 16000 \
               --vcpus 10 \
               --disk path="<customized-image-path>",format=raw \
               --osinfo detect=on,name=sle-unknown \
               --graphics none \
               --console pty,target_type=serial \
               --network network=default,model=virtio,mac=FE:C4:05:42:8B:01 \
               --virt-type kvm \
               --import \
               --boot uefi
   ```

* ISO media:

   * Create an empty `disk.img` disk image that will be used as a storage device:

     ```shell
     truncate -s 20G disk.img
     ```

   * Boot a VM using the previously created resources, namely the `customized.iso`, `disk.img` and local EFI store:

    ```shell
    virt-install --name customized-iso \
                --ram 16000 \
                --vcpus 10 \
                --import \
                --disk path=disk.img,format=raw \
                --cdrom "customized.iso" \
                --boot uefi \
                --graphics none \
                --console pty,target_type=serial \
                --network network=default,model=virtio,mac=FE:C4:05:42:8B:01 \
                --osinfo detect=on,name=sle-unknown \
                --virt-type kvm
    ```
