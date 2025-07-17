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

## Guides

* [Building a Linux-only Image](./docs/linux-only-image.md) - for customers and/or consumers interested in building Linux-only images.
* [Image Build and Customization](./docs/image-build-and-customization.md) - for customers and/or consumers interested in building images that are customized, extended and based on a specific product.
* [Release Manifest Guide](./docs/release-manifest.md) - for consumers interested in creating a release manifest for their product.

## Building from Source

```shell
make all
```

This will produce a `build/` directory containing the `elemental3` and `elemental3-toolkit` command-line clients.

## Contribution

For contributing to Elemental, please create a fork of the repository and send a Pull Request (PR). A number of GitHub Actions will be triggered on the PR and they need to pass.

Before opening a Pull Request, use `golangci-lint fmt` to format the code and `golangci-lint run` to execute linting steps that are configured in `/.golangci.yml` in the base directory of the repository.

Please make sure to follow these guidelines with regards to logging and error-handling:
* Avoid logging the very same error in multiple places on error-return
* Error logging must include at least one piece of detail, never a log without details
* Prefer logging in multiple lines rather than wrapping it into a single line

PRs will be reviewed by the maintainers and require two reviews without outstanding change-request to pass and become mergable.
