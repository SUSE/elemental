# Release Manifest Guide

The `Release Manifest` acts as a descriptor of the system's desired state from a component-level perspective. It specifies the underlying components, their specific versions and pull locations, and bundles all this into a single manifest that can be versioned by consumers and leveraged by users to deploy as a unified, single version.

Ultimately, there are two types of release manifests:

* [Product Release Manifest](#product-release-manifest)
* [Core Platform Release Manifest](#core-platform-release-manifest)

## Product Release Manifest

> **NOTE:** Elemental is in active development and the Product manifest API may change over time.

> **IMPORTANT:** The Product Release Manifest is intended to be created, maintained and supported by the consumer.

Enables consumers to extend a specific `Unified Core` release with additional components tailored to their product, bundling everything into a single versioned file called a `Product Release Manifest`. Users will utilize this manifest to describe a new image base at build time, or upgrade a target during day 2 operations.

### Product Release Manifest API

Consumers who wish to create a release manifest for their product should refer to the below API reference for information.

```yaml
metadata:
  name: "SUSE Product"
  version: "4.2.0"
  upgradePathsFrom:
  - "4.1.9"
  creationDate: "2025-07-10"
corePlatform:
  image: "registry.suse.com/unifiedcore/release-manifest"
  version: "0.0.1"
components:
  helm:
    charts:
    - chart: "cert-manager"
      version: "v1.17.2"
      namespace: "cert-manager"
      repository: "jetstack"
      values:
        crds:
          enabled: true
    - chart: "rancher"
      version: "2.11.1"
      namespace: "cattle-system"
      repository: "rancher"
      values:
        replicas: 1
      dependsOn:
      - "cert-manager"
      images:
      - name: "rancher"
        image: "registry.rancher.com/rancher/rancher:v2.11.1"
    repositories:
    - name: "rancher"
      url: "https://releases.rancher.com/server-charts/stable"
    - name: "jetstack"
      url: "https://charts.jetstack.io"
```

* `metadata` - Optional; General information about the product version that this manifest describes.
  * `name` - Required; Name of the product that this manifest describes.
  * `version` - Required; Release version of the product that this manifest describes.
  * `upgradePathsFrom` - Optional; Previous versions from which and upgrade to this release manifest version is supported.
  * `creationDate` - Optional; Defines the release date for the specified version.
* `corePlatform` - Required; Defines the `Unified Core` release version that this product wishes to be based upon and extend.
  * `image` - Required; Container image pointing to the desired `Unified Core` release manifest.
  * `version` - Required; Version of the release manifest that you wish to use. The version of the manifest matches the version of the `Unified Core` platform.
* `components` - Optional; Components with which to extend the `Unified Core` platform.
  * `helm` - Optional; Defines Helm components with which to extend the `Unified Core` platform.
    * `charts` - Required; Defines a list of Helm charts to be deployed along side any `Unified Core` defined Helm charts.
      * `chart` - Required; Name of the Helm chart, as seen in the repository.
      * `version` - Required; Version of the Helm chart, as seen in the repository.
      * `repository` - Optional if running an OCI chart; Name of the source repository that this chart can be retrieved from.
      * `name` - Optional; Pretty name of the Helm chart.
      * `namespace` - Optional; Namespace where the Helm chart will be deployed. Defaults to the `default` namespace.
      * `values` - Optional; Custom Helm chart values.
      * `dependsOn` - Optional; Defines any chart dependencies that this chart haves. Any dependency charts will be deployed before the actual chart.
      * `images` - Optional; Defines images that this chart utilizes.
        * `name` - Required; Reference name for the specified image.
        * `image` - Required; Location of the container image that this chart utilizes.
  * `repositories` - Required; Source repositories for to Helm charts.

### Bundle into an OCI image

As mentioned in the [release.yaml](configuration-directory.md#releaseyaml) configuration file, consumers can refer to a `Product Release Manifest` from an OCI image. This section outlines the minimum steps needed for consumers and/or users to setup said image, while also outlining any caveats and recommendations for the process.

*Steps:*
1. Create a product release manifest YAML file by using the [Product Release Manifest API](#product-release-manifest-api) reference. **Make sure you provide only components relevant to your product and remove the example components from the reference.**
2. Using your build tool of choice, build your image with the created manifest copied inside of it.
   * **Caveat:** To be able to find the release manifest, Elemental's tooling requires that the copied manifest's name conforms to the `release_manifest*.yaml` glob pattern and that it is copied either under the root of the OS (`/`), or under `/etc`. 
   * **Recommendation:** Since this image will only hold this file, it is advisable for the image to be as small as possible. Consider using base images such as [scratch](https://hub.docker.com/_/scratch), or similar for your OCI image.

## Core Platform Release Manifest

> **NOTE:** Elemental is in active development and the Core Platform manifest API may change over time.

> **IMPORTANT:** This manifest is maintained and provided by the `Unified Core` team and is intended to act as a base for all `Product Release Manifests`.

Defines the set of components that make up a specific `Unified Core` platform release version.

### Core Platform Release Manifest API

> **IMPORTANT:** This section is for informational purposes only. Consumers should always refer to a Core Platform release manifest provided by the `Unified Core` team.

```yaml
# The values shown in this example are for illustrative purposes only
# and should not be used directly
metadata:
  name: "SUSE Unified Core Platform"
  version: "0.0.2"
  upgradePathsFrom: 
  - "0.0.1"
  creationDate: "2025-07-14"
components:
  operatingSystem:
    version: "6.2"
    image: "registry.suse.com/unifiedcore/uc-base-os-kernel-default:0.0.1"
  kubernetes:
    rke2:
      version: "1.32"
      image: "https://download.foo.com/unifiedcore/rke2-1.32.x86-64.raw"
  helm:
    charts:
    - name: "MetalLB"
      chart: "metallb"
      version: "0.15.0"
      namespace: "metallb-system"
      repository: "metallb-repo"
    repositories:
    - name: "metallb-repo"
      url: "https://metallb.github.io/metallb"
```

The manifest's structure is similar to that of the [Product Release Manifest](#product-release-manifest-api), with the key difference being the inclusion of components unique to the Core Platform (e.g. `operatingSystem` and `kubernetes`). 

This reference focuses only on the unique to the Core Platform component APIs. Any components not mentioned here share the same description as those in the `Product Release Manifest`.

* `components` - Components described by the Core Platform release manifest.
  * `operatingSystem` - Describes the base operating system version and location.
    * `version` - Version of the base operating system.
    * `image` - Location for the container image hosting the base operating system.
  * `kubernetes` - Describes the Kubernetes distributions that are supported with this Core Platform release.
    * `rke2` - Describes the RKE2 Kubernetes distribution version and location.
      * `version` - Version for the RKE2 Kubernetes distribution.
      * `image` - Location for the `systemd-sysext` image that hosts the RKE2 Kubernetes distribution. **Currently this property refers to the RAW image file location, but the end goal is for it to refer to a container image.**
