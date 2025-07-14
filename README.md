# Elemental

[![golangci-lint](https://github.com/suse/elemental/actions/workflows/golangci-lint.yaml/badge.svg)](https://github.com/suse/elemental/actions/workflows/golangci-lint.yaml)
[![CodeQL](https://github.com/SUSE/elemental/actions/workflows/github-code-scanning/codeql/badge.svg)](https://github.com/SUSE/elemental/actions/workflows/github-code-scanning/codeql)
[![Unit Tests](https://github.com/SUSE/elemental/actions/workflows/unit_tests.yaml/badge.svg)](https://github.com/SUSE/elemental/actions/workflows/unit_tests.yaml)


# Description

Elemental is a tool for installing, configuring and updating operating system images from an OCI registry.

## Features

*   **Image Management:** Manage and version your OS images.
*   **Deployment:** Deploy an OS image to bare metal or virtual machines.
*   **Updates:** Update an existing OS installation from a newer image.
*   **Extensibility:** Extend the OS installation image with extensions.

## Quick-start

Elemental can be used to convert an OCI image into a bootable disk-image. This requires that the OCI image actually contains a bootloader, kernel, initrd and init-system. In this quick-start we will use the tumbleweed example.

```sh
$ make # build elemental binaries
$ qemu-img create -f raw build/elemental-tumbleweed.img 10G
$ sudo losetup --show -f build/elemental-tumbleweed.img # make a note of the device-name
$ sudo ./build/elemental3-toolkit install --os-image=registry.opensuse.org/devel/unifiedcore/tumbleweed/containers/uc-base-os-kernel-default:0.0.1 --config=examples/tumbleweed/config.sh --target /dev/loopX # use the loopback-device printed in previous step.
$ sudo losetup -d /dev/loopX
```

After the image is built we can boot it using QEMU:

```sh
qemu-kvm -m 4096 -hda build/elemental-tumbleweed.img -bios /usr/share/qemu/ovmf-x86_64.bin -cpu host
```

## Contribution

For contributing to Elemental, please create a fork of the repository and send a Pull Request (PR). A number of GitHub Actions will be triggered on the PR and they need to pass.

Before opening a Pull Request, use `golangci-lint fmt` to format the code and `golangci-lint run` to execute linting steps that are configured in `/.golangci.yml` in the base directory of the repository.

Please make sure to follow these guidelines with regards to logging and error-handling:
* Avoid logging the very same error in multiple places on error-return
* Error logging must include at least one piece of detail, never a log without details
* Prefer logging in multiple lines rather than wrapping it into a single line

PRs will be reviewed by the maintainers and require two reviews without outstanding change-request to pass and become mergable.
