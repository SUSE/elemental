# Elemental

Elemental is a tool for installing, configuring and updating operating system images from an OCI registry.

## Features

*   **Image Management:** Manage and version your OS images.
*   **Deployment:** Deploy an OS image to bare metal or virtual machines.
*   **Updates:** Update an existing OS installation from a newer image.
*   **Extensibility:** Extend the OS installation image with extensions.

## Elemental binaries

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

## Guides

* [Using Elemental for the first time](cookbook-first-time-use.md) - First use guide for the "UC" build of Elemental
* [Building a Linux Image](building-linux-image.md) - for users and/or consumers interested in building Linux images.
* [Image Customization](image-customization.md) - for users and/or consumers interested in customizing images that are based on a specific release.
* [Release Manifest Guide](release-manifest.md) - for consumers interested in creating a release manifest for their product.
* [Elemental and Ignition Integration](ignition-integration.md) - for consumers interested in understanding the nuances and capabilities of Ignition in the scope of Elemental.
* [Troubleshooting Guide](troubleshooting.md) - guide for users and consumers in troubleshooting a running system.
