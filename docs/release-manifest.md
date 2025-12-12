# Release Manifest Guide

The `Release Manifest` serves as a component-level descriptor of a product's system. It specifies the underlying components, their specific versions and pull locations, and bundles all this into a single manifest that can be versioned by consumers and leveraged by users to deploy as a unified, single version.

Ultimately, there are two types of release manifests:

* [Product Release Manifest](#product-release-manifest)
* [Core Platform Release Manifest](#core-platform-release-manifest)

## Product Release Manifest

> **NOTE:** Elemental is in active development and the Product manifest API may change over time.

> **IMPORTANT:** The Product Release Manifest is intended to be created, maintained and supported by the consumer.

Enables consumers to extend a specific `Core Platform` release with additional components tailored to their product, bundling everything into a single versioned file called a `Product Release Manifest`. Users will utilize this manifest to describe a new image base at build time, or upgrade a target during day 2 operations.

### Product Release Manifest API

Consumers who wish to create a release manifest for their product should refer to the below API reference for information.

```yaml
metadata:
  name: "SUSE Product"
  version: "4.2.0"
  creationDate: "2025-07-10"
corePlatform:
  image: "registry.suse.com/uc/release-manifest:0.0.1"
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
  * `version` - Required; Version of the product release that this manifest describes.
  * `creationDate` - Optional; Defines the release date for the specified version.
* `corePlatform` - Required; Defines the `Core Platform` release version that this product wishes to be based upon and extend.
  * `image` - Required; Container image pointing to the desired `Core Platform` release manifest.
* `components` - Optional; Components with which to extend the `Core Platform`.
  * `helm` - Optional; Defines Helm components with which to extend the `Core Platform`.
    * `charts` - Required; Defines a list of Helm charts to be deployed alongside any `Core Platform` defined Helm charts.
      * `chart` - Required; Name of the Helm chart, as seen in the repository.
      * `version` - Required; Version of the Helm chart, as seen in the repository.
      * `repository` - Optional if running an OCI chart; Name of the source repository that this chart can be retrieved from.
      * `name` - Optional; Pretty name of the Helm chart.
      * `namespace` - Optional; Namespace where the Helm chart will be deployed. Defaults to the `default` namespace.
      * `values` - Optional; Custom Helm chart values.
      * `dependsOn` - Optional; Defines any chart dependencies that this chart has. Any dependency charts will be deployed before the actual chart.
      * `images` - Optional; Defines images that this chart utilizes.
        * `name` - Required; Reference name for the specified image.
        * `image` - Required; Location of the container image that this chart utilizes.
    * `repositories` - Required; Source repositories for Helm charts.
      * `name` - Required; Defines the name for this repository. This name doesn't have to match the name of the actual repository, but must correspond with the `repository` field of one or more charts.
      * `url` - Required; Defines the source URL where this repository can be accessed.

### Bundle into an OCI image

As mentioned in the [release.yaml](configuration-directory.md#releaseyaml) configuration file, consumers can refer to a `Product Release Manifest` from an OCI image. This section outlines the minimum steps needed for consumers and/or users to set up said image, while also outlining any caveats and recommendations for the process.

*Steps:*
1. Create a product release manifest YAML file by using the [Product Release Manifest API](#product-release-manifest-api) reference. **Make sure you provide only components relevant to your product and remove the example components from the reference.**
2. Using your build tool of choice, build your image with the created manifest copied inside of it.
   * **Caveat:** To be able to find the release manifest, Elemental's tooling requires that the copied manifest's name conforms to the `release_manifest*.yaml` glob pattern and that it is copied either under the root of the OS (`/`), or under `/etc`. 
   * **Recommendation:** Since this image will only hold this file, it is advisable for the image to be as small as possible. Consider using base images such as [scratch](https://hub.docker.com/_/scratch), or similar for your OCI image.

## Core Platform Release Manifest

> **NOTE:** Elemental is in active development and the Core Platform manifest API may change over time.

> **IMPORTANT:** This manifest is maintained and provided by the `Elemental` team and is intended to act as a base for all `Product Release Manifests`.

Defines the set of components that make up a specific `Core Platform` release version.

### Core Platform Release Manifest API

> **IMPORTANT:** This section is for informational purposes only. Consumers should always refer to a Core Platform release manifest provided by the `Elemental` team.

```yaml
# The values shown in this example are for illustrative purposes only
# and should not be used directly
metadata:
  name: "SUSE Core Platform"
  version: "0.0.2"
  creationDate: "2025-07-14"
components:
  operatingSystem:
    image:
      base: "registry.suse.com/uc/uc-base-os-kernel-default:0.0.1"
      iso: "registry.suse.com/uc/uc-base-kernel-default-iso:0.0.1"
  systemd:
    extensions:
    - name: rke2
      image: registry.suse.com/uc/rke2:1.34_6.3-2.20
      required: false
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
  * `operatingSystem` - Operating system related components.
    * `image` - Location to different operating system container images.
      * `base` - Location to the base container image from which all other images defined here are built.
      * `iso` - Location to the installer media ISO that is used during the customization process.
  * `systemd` - Systemd related components.
    * `extensions` - List of systemd extension images.
      * `name` - Name by which the extension can be identified and possibly later enabled from the [product release reference](./configuration-directory.md#product-release-reference).
      * `image` - Location to the extension image itself.
      * `required` - Whether this extension should be included by default or not. If omitted defaults to `false`.
