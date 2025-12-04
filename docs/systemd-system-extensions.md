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
level and helps in, among other things, extending the functionality of such immutable OS by installing sysext image(s)
on such immutable OS.

As a matter of fact, `elemental3ctl` itself is installed on the immutable OS as a sysext. Another sysext installed 
out of the box is RKE2, thus making the OS a perfect environment to develop and deploy Kubernetes applications on.