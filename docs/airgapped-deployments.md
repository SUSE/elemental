# Air-gapped Deployments

Elemental can build images that create their Kubernetes cluster and deploy their workloads without any network access. Everything the cluster needs at first boot is collected while the image is customized and embedded into the image itself.

This guide covers:
* [How air-gapped images work](#how-it-works)
* [Requirements](#requirements)
* [What gets embedded](#what-gets-embedded)
* [Configuring an air-gapped image](#configuring-an-air-gapped-image)
* [Building an air-gapped image](#building-an-air-gapped-image), including [building without network access](#building-without-network-access)
* [Verifying a deployment](#verifying-a-deployment)
* [Troubleshooting](#troubleshooting) and [limitations](#limitations)

## How it works

Passing the `--airgap` option to `elemental3 customize` changes how the Kubernetes artifacts are delivered.

During customization, Elemental:
1. Pulls every configured Helm chart and embeds it into its HelmChart resource as `chartContent`, so the chart is installed without contacting its repository.
1. Collects the container images needed by RKE2, the Helm charts, and the Kubernetes manifests. See [What gets embedded](#what-gets-embedded) for the full list of sources.
1. Saves the collected images with [Hauler](https://hauler.dev) into a registry that is embedded into the image.
1. Configures RKE2 to use the embedded registry as a mirror for every registry the images come from.

At boot, `elemental-registry.service` serves the embedded images on `localhost:6545` with the Hauler binary shipped by the OS image. The service starts before `k8s-config-installer.service` and the RKE2 services. RKE2 pulls its own images and the workload images through the mirror, so no external registry needs to be reachable.

## Requirements

* **Kubernetes enabled** - `--airgap` only affects the Kubernetes artifacts. Images without Kubernetes are already self-contained.
* **Helm and Hauler** - The `elemental3` container image ships both. When running the `elemental3` binary directly, `helm` and `hauler` must be available in `PATH`.
* **A Core Platform for Elemental 3.1 or later** - The `kubernetes.image` of the Core Platform release manifest must ship the RKE2 image lists, named `rke2-images-<set>.linux-<arch>.txt`, which Elemental reads to know which RKE2 images to embed. Core Platforms for Elemental 3.1 (RKE2 1.37 and later) ship them. The Core Platforms for Elemental 3.0 (RKE2 1.35 and 1.36) do not, so they cannot be used for air-gapped images.
* **Any architecture** - Air-gapped images can be customized for any supported `--platform`, regardless of the architecture of the customization host. The Hauler binary that serves the embedded registry is shipped by the OS image.
* **Access to the artifacts** - The customization host needs access to the registries and Helm repositories that provide the artifacts, or a [prepared cache](#building-without-network-access).

## What gets embedded

| Artifact | Source | Location on the node |
| --- | --- | --- |
| RKE2 installation artifacts | `kubernetes.image` of the Core Platform release manifest | `/opt/k8s/install` |
| RKE2 images | Image lists in `kubernetes.image` | Embedded registry |
| Helm charts | Chart repositories of the release manifests and `kubernetes/cluster.yaml` | `chartContent` of the HelmChart resources |
| Workload images | Helm charts, Kubernetes manifests, release manifests, and `embeddedRegistry` | Embedded registry |

> **NOTE:** RKE2 installation artifacts and [systemd extensions](systemd-system-extensions.md) are embedded into every image, with or without `--airgap`.

The RKE2 images are taken from the following image lists:
* `core` - Always included.
* The CNI plugins listed under `cni` in `kubernetes/config/server.yaml`. Defaults to `canal`. The value `none` adds nothing.
* The ingress controllers listed under `ingress-controller` in `kubernetes/config/server.yaml`, unless it is the default for the RKE2 version (`traefik` for RKE2 1.36 and later, `ingress-nginx` before). The default ingress controller is part of the `core` list.

Workload images are collected from:
* The `images` listed for the enabled Helm charts in the release manifests.
* The output of `helm template` for every Helm chart, rendered with the configured values.
* The local manifests under `kubernetes/manifests` and the remote manifests listed in `kubernetes/cluster.yaml`.
* The images listed under `embeddedRegistry.containerImages` in `kubernetes/cluster.yaml`.

In rendered charts and manifests, Elemental collects every `image` field of `Pod`, `Deployment`, `StatefulSet`, `DaemonSet`, `ReplicaSet`, `Job`, and `CronJob` resources, including resources wrapped in a `List`.

## Configuring an air-gapped image

An air-gapped image uses the regular [configuration directory](configuration-directory.md). In most cases no extra configuration is needed. The `embeddedRegistry` section of `kubernetes/cluster.yaml` covers the remaining cases:

```yaml
# kubernetes/cluster.yaml
embeddedRegistry:
  containerImages:
    - name: registry.rancher.com/rancher/shell:v0.5.0
    - name: nginx:stable@sha256:b03c8dfc241047d827e1e14d69533205b387d476d97ef7efce58605a6c3acb84
  registries:
    - uri: registry.example.com
      credentials:
        username: user
        password: pass
```

* `containerImages` - Images that are not discovered automatically, e.g. images deployed by an operator at runtime or referenced outside of a workload resource.
* `registries` - Credentials for private registries that provide any of the embedded images. The `uri` is the registry host, e.g. `registry.example.com`. The credentials are only used during customization and are not written to the image. When running the `elemental3` binary directly, Hauler stores them in the container registry authentication configuration of the user running it.

Credentials for private Helm chart repositories are configured with the chart repositories, as for images that are not air-gapped. See [cluster.yaml](configuration-directory.md#clusteryaml).

Configuring `embeddedRegistry` without passing `--airgap` fails the customization.

If a Helm chart renders resources based on API versions that the Kubernetes version of the Core Platform does not provide, list them under `apiVersions` for the chart. They are passed to `helm template` so that image discovery sees the same resources as the cluster:

```yaml
# kubernetes/cluster.yaml
helm:
  charts:
    - name: cert-manager-fips
      version: "1.19.3"
      targetNamespace: cmf-system
      repositoryName: application-collection
      apiVersions:
        - batch/v1
        - apps/v1/Deployment
```

For the full reference of these options, see [cluster.yaml](configuration-directory.md#clusteryaml).

## Building an air-gapped image

Add `--airgap` to the regular [customization command](image-customization.md#execution):

```shell
podman run -it \
  -v <PATH_TO_CONFIG_DIR>:/config \
  -v <PATH_TO_CACHE_DIR>:/cache \
  $ELEMENTAL_IMAGE \
  customize --type <raw/iso/qcow2> \
  --airgap
```

With the default `auto` cache mode, every downloaded artifact, including the saved container images, is stored in the [cache](image-customization.md#cache-configurations). Later builds reuse it instead of downloading the artifacts again. Without a directory mounted at `/cache`, the cache is created under `<PATH_TO_CONFIG_DIR>/cache`.

> **NOTE:** The embedded images become part of the installation media, so the size of the ISO, RAW, or qcow2 image grows by the total size of the embedded images. Every installed node keeps the images in `/var/lib/elemental/registry` and serves them with its own `elemental-registry.service`. The recovery partition, which holds a copy of the installation media, contains them as well. Plan the disk size of the nodes accordingly.

### Building without network access

The cache also allows building air-gapped images on a host that has no network access:

1. On a host with network access, run the air-gapped customization once to populate the cache:
   ```shell
   podman run -it \
     -v <PATH_TO_CONFIG_DIR>:/config \
     -v <PATH_TO_CACHE_DIR>:/cache \
     $ELEMENTAL_IMAGE \
     customize --type <raw/iso/qcow2> \
     --airgap
   ```
1. Transfer the configuration directory, the cache directory, and the `elemental3` container image to the disconnected host, e.g. with `podman save -o elemental3.tar $ELEMENTAL_IMAGE` on the connected host and `podman load -i elemental3.tar` on the disconnected one.
1. On the disconnected host, run the customization from the cache only:
   ```shell
   podman run -it --network none \
     -v <PATH_TO_CONFIG_DIR>:/config \
     -v <PATH_TO_CACHE_DIR>:/cache \
     $ELEMENTAL_IMAGE \
     customize --type <raw/iso/qcow2> \
     --airgap \
     --cache offline
   ```

With `--cache offline`, the customization fails on the first artifact that is missing from the cache instead of trying to download it.

The cache only holds the artifacts of the configurations it was populated with. Populate it with the same configuration directory, `--type`, and `--platform` as the offline build, and populate it again after changing any of them, e.g. after adding a Helm chart or an image.

> **NOTE:** Images are cached by reference. A mutable tag, such as `latest`, is served from the cache until it is refreshed with `--cache refresh`.

## Verifying a deployment

Install and boot the customized image as usual. Once the node is up, the following checks show whether the embedded registry is in use.

Check that the embedded registry is running:
```shell
systemctl status elemental-registry.service
curl --fail http://localhost:6545/v2/
```

List the embedded repositories:
```shell
curl http://localhost:6545/v2/_catalog
```

Check that RKE2 uses the embedded registry as a mirror. `http://localhost:6545` should be the first endpoint of the mirrors for `docker.io` and for every registry the embedded images come from:
```shell
cat /etc/rancher/rke2/registries.yaml
```

Check that the cluster workloads are running, on a server node:
```shell
export KUBECONFIG=/etc/rancher/rke2/rke2.yaml
export PATH=$PATH:/var/lib/rancher/rke2/bin
kubectl get pods -A
```

## Troubleshooting

**`kubernetes image set "<set>" is not shipped by <image>: rke2-images-<set>.linux-<arch>.txt not found, available: ...`**

If no sets are available, the Core Platform does not support air-gapped images; use a Core Platform for Elemental 3.1 or later. Otherwise, the Core Platform's RKE2 image does not provide images for a CNI plugin or ingress controller configured in `kubernetes/config/server.yaml`. Use one of the listed sets.

**`embeddedRegistry is configured but the --airgap flag is not present`**

Pass `--airgap`, or remove the `embeddedRegistry` section from `kubernetes/cluster.yaml`.

**`image <image> (<platform>) is not cached and downloads are disabled`** or **`file '<key>' is not cached and downloads are disabled`**

An artifact needed by the configuration is missing from the cache. Populate the cache again with network access, using the same configuration directory and `--platform`.

**A pod fails with `ErrImagePull` or `ImagePullBackOff` on the node**

Check that `elemental-registry.service` is running (`journalctl -u elemental-registry.service`) and that the image is listed in the embedded registry. If it is not, the image was not discovered during customization. Add it to `embeddedRegistry.containerImages` and build the image again.

## Limitations

* The embedded registry only contains the images collected during customization. Images for workloads added after the deployment, and for upgrades, must be made available separately, e.g. from a registry that the cluster can reach.
* Image discovery only covers the sources listed in [What gets embedded](#what-gets-embedded). Images that operators deploy at runtime, or that are referenced outside of workload resources (e.g. in ConfigMaps, custom resources, or environment variables), must be listed under `embeddedRegistry.containerImages`.
* `elemental-registry.service` listens on port 6545 on all network interfaces. The registry is read-only, but it does not use authentication or TLS, so anyone who can reach the port can pull the embedded images. Restrict access to the port with a firewall if needed.
* Helm charts are embedded inline into their HelmChart resources, so they count towards the Kubernetes object size limit. Very large charts may exceed it.
