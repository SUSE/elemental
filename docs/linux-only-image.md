# Building a Linux-only Image

This section provides an overview of how users wishing to build a Linux-only image can leverage the `elemental3-toolkit` command-line client to install an operating system on a target device. 

## Prerequisites

* A server or virtual machine running Tumbleweed, Leap 16.0, SLES 16 or SUSE Linux Micro 6.2, with a minimum x86_64-v2 instruction set.

## Prepare the Installation Target

1. Create a `RAW` disk with a size of `10GB`:

    ```bash
    qemu-img create -f raw example.img 10G
    ```

2. Associate the created disk with a loop device:

    > **IMPORTANT:** Make sure to save the output of the below command, as it will be used later.

    ```bash
    losetup -f --show example.img
    ```

## Prepare Basic Configuration

To build a Linux-only image, users of the `elemental3-toolkit` can apply their basic configurations at installation time in the following ways:

* Through a [system extension image](#configuring-through-a-system-extension-image)
* Through a [configuration script](#configuring-through-a-configuration-script)

### Configuring through a System Extension Image

System extension images can be disk image files or simple folders that get loaded by the `systemd-sysext.service`. They allow users to dynamically extend the operating system's directory hierarchies with additional files. For more information, refer to the [man systemd-sysext](https://www.freedesktop.org/software/systemd/man/latest/systemd-sysext.html) documentation.

Using Elemental's toolset, users can wrap any number of these extension images inside a tarball and provide that tarball during [OS installation](#install-os-on-target-device).

> **IMPORTANT:** To be compliant with Elemental's standards, system extension images should always be added under the `/var/lib/extensions` directory of the underlying operating system.

#### Example System Extension Image

This example demonstrates how users can create a system extension image and wrap it inside a tarball that will be later provided during OS installation.  

The following builds an extension image for the `elemental3-toolkit` command line client.

> **NOTE:** The below steps use the `mkosi` tool. For more information on the tool, refer to the [upstream repository](https://github.com/systemd/mkosi).

*Prepare the `elemental3-toolkit` extension image:*

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
    Output=elemental3-toolkit-3.0.%a
    END
    ```

3. Prepare the `mkosi.extra` directory inside the `example-extension`:

    * Create the directory structure for `elemental3-toolkit`:

        ```shell
        mkdir -p example-extension/mkosi.extra/usr/local/bin
        ```

    * Copy the `elemental3-toolkit` binary from the `build/` directory of the `SUSE/elemental` repository:

        > **NOTE:** If you have not yet built your binaries, run the `make all` command from the root of the `SUSE/elemental` repository.

        ```shell
        cp <path_to_elemental_repo>/build/elemental3-toolkit <path_to_example_extension>/example-extension/mkosi.extra/usr/local/bin
        ```

4. Create the extension image from the `example-extension` directory:

    ```shell
    mkosi -C example-extension
    ```

5. Your final directory structure should look similar to:

    ```shell
    example-extension/
    ├── mkosi.conf
    ├── mkosi.extra
    │   └── usr
    │       └── local
    │           └── bin
    │               └── elemental3-toolkit
    └── mkosi.output
        ├── elemental3-toolkit-3.0.x86-64 -> elemental3-toolkit-3.0.x86-64.raw
        └── elemental3-toolkit-3.0.x86-64.raw
    ```

*Prepare the `elemental3-toolkit-3.0.x86-64.raw` extension image as an overlay:*

1. On the same level as `example-extension/`, create an `overlays/var/lib/extensions` directory:

    ```shell
    mkdir -p overlays/var/lib/extensions
    ```

2. Move the `elemental3-toolkit-3.0.x86-64.raw` extension image to this directory:

    ```shell
    mv example-extension/mkosi.output/elemental3-toolkit-3.0.x86-64.raw overlays/var/lib/extensions
    ```

3. Create an archive from the overlay directory:

    ```bash
    tar cavf overlays.tar.gz -C overlays .
    ```

You have successfully prepared an archive containing a system extension image for use during the installation process. This adds the `elemental3-toolkit` binary to the operating system after boot.

### Configuring through a configuration script

The OS installation supports configurations through a script that will be executed in a `chroot` on the unpacked operating system.

> **NOTE:** The script is executed after any user provided overlays archive are expanded.

#### Example Config Script

In this example we are going to setup a configuration script that will apply the following set of configurations on the built image:

1. Configure a password for the `root` user.
2. Setup a `oneshot` type `systemd.service` that will list the contents of the `/var/lib/extensions/` directory.

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

## Install OS on Target Device

Once you run the below command, the RAW disk that was created as part of the [Prepare the Installation Target](#prepare-the-installation-target) section will now hold a ready to boot image that will run `openSUSE Tumbleweed` and will be configured as described in the [Prepare Basic Configuration](#prepare-basic-configuration) section.

```shell
sudo elemental3-toolkit install \
  --overlay tar://overlays.tar.gz \
  --config config.sh \
  --os-image registry.opensuse.org/devel/unifiedcore/tumbleweed/containers/uc-base-os-kernel-default:latest \
  --target /dev/loop0 \
  --cmdline "root=LABEL=SYSTEM console=ttyS0"
```

Note that:
* The `overlays.tar.gz` tarball came from the system extension image [example configuration](#example-system-extension-image).
* The `config.sh` script came from the [configuration script example](#example-config-script).
* The `/dev/loop0` came from the output of the `losetup` command in the [Prepare the Installation Target](#prepare-the-installation-target) section.

> **NOTE:** `elemental3-toolkit` also supports a `--local` flag that can be used in combination with the `DOCKER_HOST=unix:///run/podman/podman.sock` environment variable to allow for referring to locally pulled OS images.

In case you encounter issues with the process, make sure to enable the `--debug` flag for more information. If the issue persists and you are not aware of the problem, feel free to raise a GitHub Issue.

## Cleanup

Since a loop device was attached to the RAW disk that was created as part of the [Prepare the Installation Target](#prepare-the-installation-target) section, you should deallocate the loop device:

```shell
losetup -d /dev/loop0
```

## Booting the Image

To boot the image in a virtual machine you can use either QEMU or libvirt utilities for creating the VM. Keep in mind that the emulated CPU (or vCPU) has to be at least `x86_64-v2` compliant.

*Using QEMU:*
```shell
qemu-kvm -m 8190 \
         -cpu host \
         -hda example.img \
         -bios /usr/share/qemu/ovmf-x86_64.bin \
         -nographic
```

You should see the bootloader prompting you to start `openSUSE Tumbleweed`.

### Validate Booted Image

1. Login with the root user and password as configured in the [config.sh](#example-config-script) script.

2. Validate you are running the expected operating system:

    ```shell
    cat /etc/os-release
    ```

3. Validate that the configured `example-oneshot.service` is created:

    * View service status:

        ```shell
        systemctl status example-oneshot.service
        ```

    * View service logs:

        ```shell
        journalctl -u example-oneshot.service
        ```

4. Validate that the `elemental3-toolkit` binary was successfully added to the system through an extension image:

    * Check logs for the `systemd-sysext.service`:

        ```shell
        journalctl -u systemd-sysext.service
        ```

    * Try calling the command:

        ```shell
        elemental3-toolkit version
        ```

## Create an Installer ISO

To create a self installer ISO, users must prepare and include a specific set of configuration assets. These include:

1. A configuration script
2. A directory tree to include additional files into the ISO filesystem

### Configure the Live Installer

The ISO supports configurations through a script which will be executed in late initramfs just before switching root.
The script is executed chrooting to the system root.

#### Example Live Config Script

In this example we are going to setup a configuration script that will set three aspects:

1. Autologin, so the live ISO does not require a root password
2. Add an elemental-autoinstaller service to run the installation at boot
3. Ensure the extensions added in the ISO filesystem are linked to `/run/extesions` so they are loaded at boot

*Steps:*

1. Create configuration script:

    ```shell
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
    ConditionFileIsExecutable=/usr/bin/elemental3-toolkit


    [Service]
    Type=oneshot
    ExecStart=/usr/bin/elemental3-toolkit --debug install --description /run/initramfs/live/Install/install.yaml
    ExecStartPost=reboot
    Restart=on-failure
    RestartSec=5

    [Install]
    WantedBy=multi-user.target
    EOF

    systemctl enable elemental-autoinstall.service
    ```

2. Make `config.sh` executable:

    ```shell
    chmod +x config.sh
    ```

#### Include Extensions in the Installer Media

The provided OS does not include the `elemental3-toolkit` required to run the installation to the target disk. `elemental3-toolkit` is provided as
a systemd extension so to make it available at ISO boot it has to be present in the ISO filesystem and copied or linked to `/run/extensions`.

This example shows how to prepare the ISO overlay directory tree and the configuration script to ensure the `elemental3-toolkit` extensions are
available and loaded at boot.

1. Create an `iso-overlay/extensions` directory:

    ```shell
    mkdir -p iso-overlay/extensions
    ```

2. Move the `elemental3-toolkit-3.0.x86-64.raw` extension image to this directory:

    ```shell
    mv example-extension/mkosi.output/elemental3-toolkit-3.0.x86-64.raw iso-overlay/extensions
    ```

3. Make sure the live configuration script links the `extensions` folder at `/run/extensions`

### Build the Installer ISO

To create an installer live ISO image process is similar to the [install step](#install-os-on-target-device) described above. The command below
will create an ISO image inside the `build` output folder using an `openSUSE Tumbleweed` image and it will be configured to automatically self
install to the target device (e.g. `/dev/sda`) at boot.

```shell
sudo elemental3-toolkit --debug build-iso \
    --output build \
    --os-image registry.opensuse.org/devel/unifiedcore/tumbleweed/containers/uc-base-os-kernel-default:latest \
    --overlay dir://iso-overlay \
    --config config-live.sh \
    --install-target /dev/sda \
    --install-overlay tar://overlays.tar.gz \
    --install-config config.sh
```

Note that:
* The `overlays.tar.gz` tarball came from the system extension image [example configuration](#example-system-extension-image).
* The `config.sh` script came from the [configuration script example](#example-config-script).
* The `/dev/sda` is the target device you want the ISO to install to.
* The `iso-overlay` is the directory tree [including extensions](#include-extensions-in-the-installer-media) that will be included in the ISO filesystem of the built image.
* The `config-live.sh` script came from the live [configuration script example](#example-live-config-script).

### Booting a Live Installer Image

In order to boot the installer ISO and test the automatic install a simple QEMU virtual machine can be launched with the following command:

```shell
qemu-kvm -m 8190 \
         -cpu host \
         -hda disk.img \
         -cdrom build/installer.iso \
         -drive if=pflash,format=raw,readonly,file=/usr/share/qemu/ovmf-x86_64-code.bin \
         -drive if=pflash,format=raw,file=ovmf-x86_64-vars.bin
```

Note that:
* EFI devices are included to the command. There is a code device for the EFI firmware and a local copy of the EFI variable store to persist any new EFI entry included during the installation.
* The `disk.img` can be an empty disk image file created with the `qemu-img create` command.

## Upgrading the OS of a Booted Image

Suppose the image that you created as part of the previous sections has been running for a while and now you want to upgrade its operating system to include the latest available package versions.

You can do this through the `elemental3-toolkit` command line tool, by executing the following command:

```shell
elemental3-toolkit upgrade --os-image registry.opensuse.org/devel/unifiedcore/tumbleweed/containers/uc-base-os-kernel-default:latest
```

After command completion a new snapshot will be created:

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
