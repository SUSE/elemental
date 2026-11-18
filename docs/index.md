# Elemental

Elemental is an image-based operating system management framework that uses OCI registries to distribute, install, and update Linux systems.

It enables complete systems—including the operating system, Kubernetes configuration, and platform components—to be assembled, versioned, deployed, and updated using the same OCI-based workflows that have made containers successful.

Elemental is designed to support cloud-native infrastructure, edge computing, AI platforms, appliance-style products, and other environments that benefit from image-based operating systems and declarative lifecycle management.

## Why Elemental?

Traditional infrastructure often relies on post-installation configuration, package management, and manual lifecycle operations.

Elemental takes an image-based approach where complete system definitions are assembled ahead of time, versioned as OCI artifacts, and deployed consistently across environments.

This approach provides:

- Reproducible deployments
- Atomic and predictable upgrades
- Simplified lifecycle management
- Reduced configuration drift
- Consistent operations across environments
- Versioned and auditable system definitions

## Architecture

Elemental consists of two primary binaries:

### `elemental3`

`elemental3` is the build-time and solution composition tool.

It is responsible for:

- Building deployable installation media and RAW disks
- Customizing operating system images
- Integrating Kubernetes configurations
- Adding extensions and additional payloads
- Managing release descriptors

In practice, `elemental3` is used by platform builders and solution teams to assemble complete, deployable systems.

### `elemental3ctl`

`elemental3ctl` is the runtime lifecycle management tool.

It is responsible for:

- Operating system installation and upgrades
- Recovery and reset operations
- Kernel module management
- OCI image deployment
- Runtime system administration tasks

In practice, `elemental3ctl` runs on target systems and manages their lifecycle throughout deployment and operation.

## Documentation
* [Using Elemental for the first time](cookbook-first-time-use.md) - First use guide for the Elemental 3 project
* [Image Customization](image-customization.md) - for users and/or consumers interested in customizing images that are based on a specific release.
* [Air-gapped Deployments](airgapped-deployments.md) - for users and/or consumers interested in building images that deploy Kubernetes clusters without network access.
* [Release Manifest Guide](release-manifest.md) - for consumers interested in creating a release manifest for their solution.
* [Configuration Directory Guide](configuration-directory.md) - for users and/or consumers interested in checking configration options.
* [Filesystem Layout Guide](filesystem.md) - for users and/or consumers interested in knowing the system layout and the nuances of data persistency across updates.
* [Elemental and Ignition Integration](ignition-integration.md) - for consumers interested in understanding the nuances and capabilities of Ignition in the scope of Elemental.
* [Troubleshooting Guide](troubleshooting.md) - guide for users and consumers in troubleshooting a running system.
