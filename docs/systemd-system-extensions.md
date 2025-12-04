# System extensions (systemd-sysext) in Elemental

This chapter covers what a system extension (a.k.a. sysext) is, why it's relevant for Elemental, and how Elemental 
uses it to provide extensibility in the image mode OS image.

## What is a system extension?

[System extension images (or sysext images)](https://www.freedesktop.org/software/systemd/man/latest/systemd-sysext.html) 
can be disk image files or simple folders that get loaded by
`systemd-sysext.service`. They provide a way to dynamically extend the `/usr`
and `/opt` directory hierarchies with additional files. When one or more system
extension images are activated, their `/usr` and `/opt` hierarchies are
combined via "overlayfs" with the same hierarchies of the host OS. This causes
"merging" (or overmounting) of the `/usr` and `/opt` contents of the system
extension image with that of the underlying host system.

When a system extension image is deactivated, the `/usr` and `/opt` mountpoints are disassembled, thus revealing the 
unmodified original host version of the filesystem hierarchy.

Merging or activating makes the system extension's resources appear below `/usr` and `/opt` as if they were 
included in the base OS image itself. Unmerging or deactivating makes them disappear again, leaving in place only 
the files that were shipped with the base OS image itself.

Note that files and directories contained in a system extension image outside the `/usr` and `/opt` hierarchies are 
not merged. E.g., files in `/etc` and `/var` included in a system extension image will not appear under the 
respective hierarchies after activation.

To learn more about system extension images, refer to the 
[official documentation](https://www.freedesktop.org/software/systemd/man/latest/systemd-sysext.html) about it.

## When are system extensions required?

A system extension is useful when working with an OS with an immutable base. Such an OS is usually shipped as an 
image that contains all the essential software: bootloader, kernel and userspace utilities. However, it doesn't have 
a package manager like `zypper` which can be used to install additional packages. System extensions help extend the 
functionality and usability of those image mode OSes.

## Creating system extension images

There are multiple tools to create a system extension image, e.g., `mksquashfs`, `mkerofs`, or `mkosi`. In this guide we 
will use `mkosi`, which is a higher level tool than the other two. It builds bootable OS images, system 
extension images, CPIO archives, and more. The way it differs from the other two is that `mkosi` has a wrapper 
around popular package managers that helps install and setup things without needing to install a package manager.

In this guide, we will see two approaches to create a system extension image:
1. By embedding a binary
2. By installing an RPM

### Embedding a binary in a system extension image

In this section, we will install the `kubectl` binary as a system extension on an existing ISO package as OCI image.

1. Create the root extension directory:
    ```shell
    mkdir example-extension
    ```
1. Prepare a configuration file called `mkosi.conf` that the `mkosi` tool will follow:
    ```shell
    cat <<- END > example-extension/mkosi.conf
    [Distribution]
    Distribution=opensuse
    Release=tumbleweed
    
    [Build]
    Environment=SYSTEMD_REPART_OVERRIDE_FSTYPE_ROOT=squashfs
    
    [Output]
    Format=sysext
    OutputDirectory=mkosi.output
    Output=kubectl-3.0.%a
    END
   ```
   - The `Distribution` section defines the Linux distribution to be installed in the image.
   - `SYSTEMD_REPART_OVERRIDE_FSTYPE_ROOT=squashfs` specifies the root filesystem type for the resulting disk image.
   - The `Output` section defines various values for the result produced by `mkosi`. Besides `sysext`, it could 
     generate various other types of output.
1. Prepare the `mkosi.extra` directory inside the `example-extension` directory:
   - Create the directory structure for kubectl:
        ```shell
        mkdir -p example-extension/mkosi.extra/usr/bin
        ```       
   - Get the `kubectl` binary by following the steps mentioned
     [here](https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/), make it executable, and copy it to the 
     directory created above:
        ```shell
        chmod +x kubectl
        cp kubectl example-extension/mkosi.extra/usr/bin
        ```
1. Create the extension image from the `example-extension` directory:
    > **NOTE**: Make sure you have `mkosi` installed on your system. If not, install it using `zypper install mkosi`.

    ```shell
    mkosi -C example-extension
    ```
1. Your final directory structure should look like:
    ```shell
    example-extension
    ├── mkosi.conf
    ├── mkosi.extra
    │   └── usr
    │       └── bin
    │           └── kubectl
    └── mkosi.output
        ├── kubectl.x86-64 -> kubectl.x86-64.raw
        └── kubectl.x86-64.raw
    ```
We now have a system extension image named `kubectl.x86-64.raw` ready under the `mkosi.output/` directory.

### Installing RPMs in a system extension image

The Elemental project's repository contains an [example](../examples/tools-sysext) that can be run directly to 
understand this. Check out the directory structure of [`examples/tools-sysext`](../examples/tools-sysext).

```shell
tree examples/tools-sysext
examples/tools-sysext
├── mkosi.conf
└── mkosi.images
    ├── base
    │   └── mkosi.conf
    └── tools
        ├── mkosi.conf
        └── mkosi.finalize
```
The presence of the `mkosi.images` directory indicates that the configuration is meant for multiple images.

Here there are three `mkosi.conf` files, relative to `examples/tools-sysext`. Below is a brief summary of the purpose of each of 
these files:
- `mkosi.conf`: This is the global `mkosi` configuration file which contains distribution-level configuration. 
- `base/mkosi.conf`: This file is for the base layer that's used to install package(s) upon.
- `tools/mkosi.conf`: This is the configuration for the tools layer.

Creating the tools system extension requires "subtracting" the tools layer from the base layer. The base layer hence 
needs to include any of the files that are already available on the host operating system, and the tools definition 
defines the extensions over that. This approach ensures that the tools layer does not overwrite any files on the 
operating system.

You can build the system extension by invoking `mkosi` in the `examples/tools-sysext` directory. This will create a base 
image and a tools image, and then assemble them into a system extension.
```shell
cd examples/tools-sysext
mkosi --directory $PWD
```
The resulting system extension will be available in the `mkosi.output/` directory as `tools-1.0_1.0_x86-64.raw`.

Such systemd extensions can be included as an overlay in the Elemental customization process.

Further steps to build the image can be found in the document for
[Building Linux image](./building-linux-image.md#preparing-the-system-extension-image-as-an-overlay).

## How are systemd extensions used in Elemental?

The elemental project mainly consists of two binaries:
- `elemental3`
- `elemental3ctl`

`elemental3` is a higher-level tool that takes as its input an OCI image containing an ISO artifact, adds payloads
such as system extensions, Kubernetes definitions, first-boot configs, and generates an ISO or RAW file which can be
used to boot a VM.

`elemental3ctl` is a lower-level tool that can do various things like installing an OS (packaged as an OCI image) on a
target system, upgrading such OS from an OCI image, manage kernel modules on a system, unpack an OCI image, build
an installation media (generally an ISO file) from an OS image (packaged as OCI image), and more.

`elemental3ctl` is a runtime management tool that helps deploy an OS image on disk, as well as helps manage such an 
installation by performing upgrades, managing kernel modules, perform factory reset, etc. `elemental3` complements 
it by building and configuring an OS image that could have additional artifacts and
capabilities, making it a platform to build and deploy cloud-native applications.

`elemental3ctl` is provided on an image mode OS as a system extension. Another system extension
installed out of the box is RKE2, thus making the OS a perfect environment to develop and deploy Kubernetes
applications on.