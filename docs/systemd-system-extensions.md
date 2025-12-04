# systemd system extensions (sysext) in elemental

This document covers what system extension (a.k.a. sysext) is, why it's relevant for elemental, and how elemental 
uses it to provide extensibility in immutable OS image.

## What is sysext?

systemd system extensions (or systemd sysext) provide a way to dynamically extend the `/usr` and `/opt` directory 
hierarchies with additional files. When one or more sysext images are activated, their `/usr` and `/opt` hierarchies 
are combined via "overlayfs" with the same hierarchies of the host OS. This causes "merging" (or overmounting) of the 
`/usr` and `/opt` contents of the sysext image with that of underlying host system.

When a sysext image is deactivated, the `/usr` and `/opt` mountpoints are disassembled, thus revealing the 
unmodified original host version of hierarchy.

Merging or activating makes the system extension's resources suddenly appear below `/usr` and `/opt` as if they were 
included in the base OS image itself. Unmerging or deactivating makes them disappear again, leaving in place only 
the files that were shipped with the base OS image itself.

Note that files and directories contained in a sysext image outside the `/usr` and `/opt` hierarchies are not merged.
E.g., files in `/etc` and `/var` included in a sysext image will not appear under the respective hierarchies after 
activation.

To learn more about system extension images, refer the [official documentation](https://www.freedesktop.org/software/systemd/man/latest/systemd-sysext.html) about it.

## Where is sysext required?

sysext is useful when working with an OS with an immutable base. Such an OS is usually shipped as an image that 
contains all the essential software: bootloader, kernel and userspace utilities. However, it doesn't have a package 
manager like zypper which can be used to install additional packages. sysext helps extend the functionality and 
usability of such minimal OS.

## elemental project and sysext

The elemental project consists mainly of two binaries:
- `elemental`
- `elemental3ctl`

`elemental` is a higher level tool used for installation and upgrade of an immutable OS. `elemental3ctl` is lower 
level and helps in, among other things, building a bootable OS image of either RAW or ISO foramt. However, 
`elemental3ctl` is not expected to be used directly by most users; it will be used by `elemental` to get stuff done. 
`elemental3ctl` can also be used to install an OCI image on the target system.

As a matter of fact, `elemental3ctl` itself is installed on the immutable OS as a sysext. Another sysext installed 
out of the box is RKE2, thus making the OS a perfect environment to develop and deploy Kubernetes applications on.

## Example sysext image using elemental3ctl

`elemental3ctl` can wrap any number of sysext images inside a tarball and provide that tarball during OS 
installation. The sysext images should always be added under `/var/lib/extensions` directory of the underlying OS.

There are multiple ways to create a sysext image, e.g., `mksquashfs`, `mkerofs`, `mkosi`. In this guide we will use 
`mkosi` which is a higher level tool than the other two. It helps build bootable OS images, sysext images, CPIO 
archives, and more. In this guide we will focus on using it to build sysext images.

A sysext can be created from either a binary or from a set of packages available in the distribution.