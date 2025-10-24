# Building a Linux Virtual Machine Image with Elemental

This section provides an overview of how you build a Linux image that can include additional extensions using Elemental and the `elemental3ctl` command-line interface. The image can be used to boot a virtual machine and run a Linux operating system, such as `openSUSE Tumbleweed`, with custom configurations and extensions.

## Prerequisites

* A server or virtual machine running Tumbleweed, Leap 16.0, SLES 16 or SUSE Linux Micro 6.2, with a minimum x86_64-v2 instruction set.

## Prepare the installation target

1. Create a `qcow2` disk with a size of `20GB`:

    ```shell
    qemu-img create -f qcow2 example.qcow2 20G
    ```

2. Associate the created virtual disk with a block device:

    ```shell
    sudo modprobe nbd && sudo qemu-nbd -c /dev/nbd0 example.qcow2
    ```

3. Check for the block device:

    ```shell
    sudo lsblk /dev/nbd0
    ```

## Prepare basic configuration

`elemental3ctl` can apply basic configuration and extensions at deployment time in the following ways:

* Through a [system extension image](#configuring-through-a-system-extension-image)
* Through a [configuration script](#configuring-through-a-configuration-script)

### Configuring through a system extension image

System extension images can be disk image files or simple folders that get loaded by the `systemd-sysext.service`. They allow you to dynamically extend the operating system. For more information, refer to the [man systemd-sysext](https://www.freedesktop.org/software/systemd/man/latest/systemd-sysext.html) documentation.

Using Elemental's toolset, you can wrap any number of these extension images inside a tarball and provide that tarball during [OS installation](#install-operating-system-on-a-target-device).

> **IMPORTANT:** To be compliant with Elemental's standards, system extension images should always be added under the `/var/lib/extensions` directory of the underlying operating system.

#### Example system extension image

You have multiple options to create a system extension image. Below are some common methods, with the `mkosi` tool being the most prevalent. This tool allows you to build an image from a set of configuration files.

You can create a system extension from a binary or from a set of packages available in the distribution. The following example demonstrates how to create a system extension from a binary.

##### Embed a binary in a system extension image

This example demonstrates how you can create a system extension image and wrap it inside a tarball that will be later provided during OS installation.

The following builds an extension image for the `elemental3ctl` command-line interface.

> **NOTE:** The below steps use the `mkosi` tool. For more information on the tool, refer to the [upstream repository](https://github.com/systemd/mkosi).

*Prepare the `elemental3ctl` extension image:*

1. Create the root extension directory:

    ```shell
    mkdir example-extension
    ```

2. Prepare a configuration file called `mkosi.conf` that the `mkosi` tool will follow:

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
    Output=elemental3ctl-3.0.%a
    END
    ```

3. Prepare the `mkosi.extra` directory inside the `example-extension`:

    * Create the directory structure for `elemental3ctl`:

        ```shell
        mkdir -p example-extension/mkosi.extra/usr/local/bin
        ```

    * Copy the `elemental3ctl` binary from the `build/` directory of the `SUSE/elemental` repository:

        > **NOTE:** If you have not yet built your binaries, run the `make all` command from the root of the `SUSE/elemental` repository.

        ```shell
        cp <path_to_elemental_repo>/build/elemental3ctl <path_to_example_extension>/example-extension/mkosi.extra/usr/local/bin
        ```

4. Create the extension image from the `example-extension` directory:
   > **NOTE:** Make sure you have `mkosi` installed on your system. If not, you can install it using `zypper -n install mkosi`.

    ```shell
    mkosi -C example-extension
    ```

5. Your final directory structure should look similar to:

    ```shell
    example-extension/
    ├── mkosi.conf
    ├── mkosi.extra
    │   └── usr
    │       └── local
    │           └── bin
    │               └── elemental3ctl
    └── mkosi.output
        ├── elemental3ctl-3.0.x86-64 -> elemental3ctl-3.0.x86-64.raw
        └── elemental3ctl-3.0.x86-64.raw
    ```

6. The `mkosi.output/elemental3ctl-3.0.x86-64.raw` file is the system extension image that can be used during the OS installation process following the steps in [Prepare the system extension image as an overlay](#prepare-the-system-extension-image-as-an-overlay).


##### Install RPMs in a system extension image

There are 3 `mkosi.conf` configurations needed:

* [mkosi.conf in the base directory](../examples/tools-sysext/mkosi.conf)
* [base/mkosi.conf defining the base layer](../examples/tools-sysext/mkosi.images/base/mkosi.conf)
* [tools/mkosi.conf defining the tools layer](../examples/tools-sysext/mkosi.images/tools/mkosi.conf)

Creating the tools system extension requires "subtracting" the tools layer from the base layer. The base layer hence needs to include any of the files that are already available on the host operating system, and the tools definition defines the extensions over that. This approach ensures that the tools layer does not overwrite any files on the operating system.

You can build the system extension by invoking `mkosi` in the `examples/tools-sysext` directory. This will create a base image and a tools image, and then assemble them into a system extension.

```shell
cd examples/tools-sysext
mkosi --directory $PWD
```

This will produce the base and extension images and assemble it into a system extension:

```text
Block level copying and synchronization of partition 0 complete in 4.776ms.
Adding new partition 0 to partition table.
Writing new partition table.
All done.
Running post-output script mkosi.images/tools/mkosi.postoutput…
mkosi.postoutput tools-1.0
tools-sysext/mkosi.output/tools-1.0.x86-64.raw size is 11.0M, consumes 1.6M.
```

The resulting system extension will be available in the `mkosi.output/` directory as `tools-1.0.x86-64.raw`.

This system extension can be used as an overlay during the OS installation process, following the steps in the next section.


#### Prepare the system extension image as an overlay

The following steps prepare the example `elemental3ctl-3.0.x86-64.raw` extension image as an overlay:

1. On the same level as `example-extension/`, create an `overlays/var/lib/extensions` directory:

    ```shell
    mkdir -p overlays/var/lib/extensions
    ```

2. Move the `elemental3ctl-3.0.x86-64.raw` extension image to this directory:

    ```shell
    mv example-extension/mkosi.output/elemental3ctl-3.0.x86-64.raw overlays/var/lib/extensions
    ```

3. Create an archive from the overlay directory:

    ```shell
    tar -cavzf overlays.tar.gz -C overlays .
    ```

You have now prepared an archive containing a system extension image for use during the installation process. This adds the `elemental3ctl` binary to the operating system after boot.

### Configuring through a configuration script

The OS installation supports configurations through a script that will run in a `chroot` on the unpacked operating system after expanding the provided overlays archives.

#### Example configuration script

This configuration script applies the following set of configurations on the built image:

1. Configures the password for the `root` user to `linux`.
2. Sets up a `oneshot` type `systemd.service` that will list the contents of the `/var/lib/extensions/` directory.

*Steps:*

1. Create configuration script:

    ```shell
    cat <<- EOF > config.sh
    #!/bin/bash

    set -e

    # Set root user password
    echo "linux" | passwd root --stdin

    # Configure example systemd service
    cat <<- END > /etc/systemd/system/example-oneshot.service
    [Unit]
    Description=Example One-Shot Service

    [Service]
    Type=oneshot
    ExecStart=/bin/ls -alh /var/lib/extensions/

    [Install]
    WantedBy=multi-user.target
    END

    systemctl enable example-oneshot.service
    EOF
    ```

2. Make `config.sh` executable:

    ```shell
    chmod +x config.sh
    ```

## Install operating system on a target device

Once you run the below command, the virtual disk created as part of the [Prepare the Installation Target](#prepare-the-installation-target) section now holds a ready to boot image that will run `openSUSE Tumbleweed` and will be configured as described in the [Prepare Basic Configuration](#prepare-basic-configuration) section.

```shell
sudo elemental3ctl install \
  --overlay tar://overlays.tar.gz \
  --config config.sh \
  --os-image registry.opensuse.org/devel/unifiedcore/tumbleweed/containers/uc-base-os-kernel-default:latest \
  --target /dev/nbd0 \
  --cmdline "root=LABEL=SYSTEM console=ttyS0"
```

Note that:

* The `overlays.tar.gz` tarball came from the system extension image [example configuration](#example-system-extension-image).
* The `config.sh` script came from the [configuration script example](#example-configuration-script).
* `/dev/nbd0` is the chosen block device from the `qemu-nbd -c` command in the [Prepare the Installation Target](#prepare-the-installation-target) section.

> **NOTE:** `elemental3ctl` also supports a `--local` flag that can be used in combination with the `DOCKER_HOST=unix:///run/podman/podman.sock` environment variable to allow for referring to locally pulled OS images.

In case you encounter issues with the process, make sure to enable the `--debug` flag for more information. If the issue persists and you are not aware of the problem, feel free to raise a GitHub Issue.

## Mandatory cleanup before booting the image

Since you attached a block device to the virtual disk created in the [Prepare the Installation Target](#prepare-the-installation-target) section, detach the block device before booting the image:

```shell
sudo qemu-nbd -d /dev/nbd0
```

## Starting the virtual machine image

To boot the image in a virtual machine, you can use either QEMU or libvirt utilities for creating the VM.

*Using QEMU:*
> **NOTE:** Make sure you have `qemu` installed on your system. If not, you can install it using `zypper -n install qemu-x86`.

> **NOTE:** If you are using a different architecture, make sure to adjust the `qemu-system-x86_64` command accordingly.

> **NOTE:** If you haven't configured your user to be in the `kvm` group, you can run the command with `sudo` to allow QEMU to access the KVM acceleration.

```shell
qemu-system-x86_64 -m 8G \
         -accel kvm \
         -cpu host \
         -hda example.qcow2 \
         -bios /usr/share/qemu/ovmf-x86_64.bin \
         -nographic
```

You should see the bootloader prompting you to start `openSUSE Tumbleweed`.


### Explore virtual machine

1. Login with the root user and password as configured in the [config.sh](#example-configuration-script) script.

2. Check you are running the expected operating system:

    ```shell
    cat /etc/os-release
    ```

3. Check that `example-oneshot.service` has run successfully:

    * View service status:

        ```shell
        systemctl status example-oneshot.service
        ```

    * View service logs:

        ```shell
        journalctl -u example-oneshot.service
        ```

4. Check that `elemental3ctl` binary is available and working:

    * Check logs for the `systemd-sysext.service`:

        ```shell
        journalctl -u systemd-sysext.service
        ```

    * Try calling the command:

        ```shell
        elemental3ctl version
        ```

## Create an Installer ISO

To create a self installer ISO, you should prepare and include a specific set of configuration assets. These include:

1. A configuration script
2. Extensions to the installer media


### Configure the Live Installer

The ISO supports configurations through a script which will run in late initramfs in a writeable system root.


#### Example live configuration script

In this example, we are going to prepare a configuration script that will set three aspects:

* Autologin so the live ISO does not require a root password
* An elemental-autoinstaller service to run the installation at boot
* A link between the extensions in the ISO filesystem and `/run/extensions` so that they are loaded at boot

Create the script and make it executable:

```shell
cat <<- END > config-live.sh
#!/bin/bash

# Set autologin for the Live ISO
mkdir -p /etc/systemd/system/serial-getty@ttyS0.service.d

cat > /etc/systemd/system/serial-getty@ttyS0.service.d/override.conf << EOF
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin root --noclear %I $TERM
EOF

mkdir -p /etc/systemd/system/getty@tty1.service.d

cat > /etc/systemd/system/getty@tty1.service.d/override.conf << EOF
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin root --noclear %I $TERM
EOF

# Ensure extensions included in ISO's /extensions folder are loaded at boot
# ISO filesystem is mounted at /run/initramfs/live folder
rm -rf /run/extensions
ln -s /run/initramfs/live/extensions /run/extensions

# Set the elemental-autoinstall.service
cat > /etc/systemd/system/elemental-autoinstall.service << EOF
[Unit]
Description=Elemental Autoinstall
Wants=network-online.target
After=network-online.target
ConditionPathExists=/run/initramfs/live/Install/install.yaml
ConditionFileIsExecutable=/usr/local/bin/elemental3ctl

[Service]
Type=oneshot
ExecStart=/usr/local/bin/elemental3ctl --debug install
ExecStartPost=reboot

[Install]
WantedBy=multi-user.target
EOF

systemctl enable elemental-autoinstall.service
END

chmod +x config-live.sh
```

#### Include Extensions in the Installer Media

The provided OS does not include the `elemental3ctl` required to run the installation to the target disk. The `elemental3ctl` is delivered through a systemd extension image.
To ensure it is available at ISO boot, it has to be included in the ISO filesystem and either copied or linked to `/run/extensions`.

This example shows how to prepare the ISO overlay directory tree and the configuration script to ensure the `elemental3ctl` extensions are
available and loaded at boot.

1. Create an `iso-overlay/extensions` directory:

    ```shell
    mkdir -p iso-overlay/extensions
    ```

2. Create the [elemental3ctl](#example-system-extension-image) extension image and move it to this directory:

    ```shell
    mv example-extension/mkosi.output/elemental3ctl-3.0.x86-64.raw iso-overlay/extensions
    ```

3. Make sure the live configuration script links the `extensions` folder at `/run/extensions`

### Build the Installer ISO

The command below creates an ISO image inside the `build` output directory.
It will be using an `openSUSE Tumbleweed` image and will be configured to automatically self install to the target device (e.g. `dev/sda`) at boot.

```shell
sudo elemental3ctl --debug build-iso \
    --output build \
    --os-image registry.opensuse.org/devel/unifiedcore/tumbleweed/containers/uc-base-os-kernel-default:latest \
    --overlay dir://iso-overlay \
    --cmdline "root=live:CDLABEL=LIVE rd.live.overlay.overlayfs=1 console=ttyS0" \
    --config config-live.sh \
    --install-target /dev/sda \
    --install-overlay tar://overlays.tar.gz \
    --install-config config.sh \
    --install-cmdline "root=LABEL=SYSTEM console=ttyS0"
```

Note that:
* The `overlays.tar.gz` tarball came from the system extension image [example configuration](#example-system-extension-image).
* The `config.sh` script came from the [configuration script example](#example-configuration-script).
* The `/dev/sda` is the target device you want the ISO to install to.
* The `iso-overlay` is the directory tree [including extensions](#include-extensions-in-the-installer-media) that will be included in the ISO filesystem of the built image.
* The `config-live.sh` script came from the live [configuration script example](#example-live-configuration-script).

### Booting a Live Installer Image

> **NOTE:** Make sure you have `qemu` installed on your system. If not, you can install it using `zypper -n install qemu-x86`.
> If you are using a different architecture, ensure the package name and respective command below are adjusted accordingly.

Launch a virtual machine to boot the installer ISO and verify the automated installation:

```shell
qemu-system-x86_64 -m 8G \
         -accel kvm \
         -cpu host \
         -hda disk.img \
         -cdrom build/installer.iso \
         -drive if=pflash,format=raw,readonly,file=/usr/share/qemu/ovmf-x86_64-code.bin \
         -drive if=pflash,format=raw,file=ovmf-x86_64-vars.bin \
         -nographic
```

Note that:
* EFI devices are included in the command. There is a code device for the EFI firmware and a local copy of the EFI variable store to persist any new EFI entry included during the installation.
* The `disk.img` can be an empty disk image file created with the `qemu-img create` command.


## Upgrading the OS of a Booted Image

Suppose the image that you created as part of the previous sections has been running for a while and now you want to upgrade its operating system to include the latest available package versions.

You can do this through the `elemental3ctl` command line tool, by executing the following command:

```shell
elemental3ctl upgrade --os-image registry.opensuse.org/devel/unifiedcore/tumbleweed/containers/uc-base-os-kernel-default:latest
```

After command completion, a new snapshot will be created:

```shell
localhost:~ # snapper list
 # | Type   | Pre # | Date                     | User | Used Space | Cleanup | Description                             | Userdata
---+--------+-------+--------------------------+------+------------+---------+-----------------------------------------+---------
0  | single |       |                          | root |            |         | current                                 |
1- | single |       | Wed Jul 16 12:57:23 2025 | root |  12.28 MiB |         | first root filesystem, snapshot 1       |
2+ | single |       | Wed Jul 16 13:00:13 2025 | root |  12.28 MiB | number  | snapshot created from parent snapshot 1 |
```

What's left is to reboot the OS and select the latest snapshot from the grub menu. After the reboot, your snapshots should look similar to this:

```shell
localhost:~ # snapper list
 # | Type   | Pre # | Date                     | User | Used Space | Cleanup | Description                             | Userdata
---+--------+-------+--------------------------+------+------------+---------+-----------------------------------------+---------
0  | single |       |                          | root |            |         | current                                 |
1  | single |       | Wed Jul 16 12:57:23 2025 | root |  12.28 MiB |         | first root filesystem, snapshot 1       |
2* | single |       | Wed Jul 16 13:00:13 2025 | root |  12.28 MiB | number  | snapshot created from parent snapshot 1 |
```

The latest snapshot will be running on the latest version of the `registry.opensuse.org/devel/unifiedcore/tumbleweed/containers/uc-base-os-kernel-default` image and will still hold any previously defined configurations and/or extensions.
