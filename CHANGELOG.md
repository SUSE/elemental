# Changelog

## v3.1.0-alpha.20260528

Changes since `v3.0.0-alpha.20251212`.

### Highlights

- RKE2 installation was reworked to use the standard install script and tarball workflow delivered through an OCI image instead of shipping RKE2 as a systemd extension ([#399](https://github.com/SUSE/elemental/pull/399)).
- Release manifests now distinguish Core Platform manifests from Solution manifests, replacing the previous Product terminology across APIs, examples, docs, and resolver code ([#462](https://github.com/SUSE/elemental/pull/462)).
- `elemental3 release-info` was added for inspecting release manifests from local files or OCI images, including readable table and Markdown output ([#392](https://github.com/SUSE/elemental/pull/392), [#407](https://github.com/SUSE/elemental/pull/407)).
- Configuration parsing is now schema-aware and validates image, deployment, Kubernetes, Helm, and release manifest definitions at parse time ([#375](https://github.com/SUSE/elemental/pull/375), [#379](https://github.com/SUSE/elemental/pull/379), [#421](https://github.com/SUSE/elemental/pull/421), [#360](https://github.com/SUSE/elemental/pull/360), [#365](https://github.com/SUSE/elemental/pull/365)).
- Kubernetes customization now supports `registries.yaml`, authenticated Helm repositories, safer config installation, and more robust resource deployment ([#429](https://github.com/SUSE/elemental/pull/429), [#387](https://github.com/SUSE/elemental/pull/387), [#443](https://github.com/SUSE/elemental/pull/443), [#439](https://github.com/SUSE/elemental/pull/439), [#411](https://github.com/SUSE/elemental/pull/411)).
- Local OCI extraction can use a running containerd daemon for already-unpacked images when `--local` and `CONTAINERD_SOCK` are both set ([#422](https://github.com/SUSE/elemental/pull/422)).

### Breaking Changes

- Configuration directories must declare the v0 schema in `install.yaml` ([#375](https://github.com/SUSE/elemental/pull/375)).
- Kubernetes cluster configuration moved from top-level `kubernetes.yaml` to `kubernetes/cluster.yaml` ([#370](https://github.com/SUSE/elemental/pull/370)).
- `release.yaml` no longer accepts the `name` field ([#436](https://github.com/SUSE/elemental/pull/436)).
- Product release manifest terminology was renamed to Solution. Example files and APIs now use names such as `suse-solution-manifest.yaml` and `pkg/manifest/api/solution` ([#462](https://github.com/SUSE/elemental/pull/462)).
- Kubernetes is now a first-class release-manifest component enabled with `components.kubernetes: {}`; RKE2 is no longer selected or represented as a systemd extension ([#399](https://github.com/SUSE/elemental/pull/399)).
- `elemental3 release-info` no longer provides YAML or JSON output modes. Use the source manifest YAML for machine-readable inspection, or table/Markdown output for readable summaries ([#407](https://github.com/SUSE/elemental/pull/407)).

### Schema And Definition Changes

Configuration directories now carry an explicit schema version ([#375](https://github.com/SUSE/elemental/pull/375)):

```yaml
# install.yaml
schema: v0
bootloader: grub
```

Kubernetes cluster configuration moved into the `kubernetes` directory ([#370](https://github.com/SUSE/elemental/pull/370)):

```yaml
# Before
# kubernetes.yaml
nodes:
  - hostname: node1.example
    type: server

# After
# kubernetes/cluster.yaml
nodes:
  - hostname: node1.example
    type: server
```

Release references changed from Product wording and RKE2 systemd extension selection to Solution wording and explicit Kubernetes selection ([#399](https://github.com/SUSE/elemental/pull/399), [#436](https://github.com/SUSE/elemental/pull/436), [#462](https://github.com/SUSE/elemental/pull/462)):

```yaml
# Before
name: suse-product
manifestURI: file://./suse-product-manifest.yaml
components:
  systemd:
    - extension: rke2
  helm:
    - chart: rancher

# After
manifestURI: file://./suse-solution-manifest.yaml
components:
  kubernetes: {}
  helm:
    - chart: rancher
```

Core release manifests now describe the Kubernetes distribution as a first-class component ([#399](https://github.com/SUSE/elemental/pull/399)):

```yaml
# Before
components:
  systemd:
    extensions:
      - name: rke2
        image: registry.example.com/rke2:1.34

# After
components:
  kubernetes:
    version: v1.35.0+rke2r1
    image: registry.example.com/rke2:1.35_1.0
```

Helm credentials can now be provided for release-selected charts and user-defined Kubernetes Helm repositories ([#387](https://github.com/SUSE/elemental/pull/387), [#443](https://github.com/SUSE/elemental/pull/443)):

```yaml
components:
  helm:
    - chart: endpoint-copier-operator
      credentials:
        username: release-user
        password: release-pass

kubernetes:
  helm:
    repositories:
      - name: private-charts
        url: https://charts.example.com
        credentials:
          username: user
          password: pass
        insecureSkipTLSVerify: true
```

Kubernetes configuration can include registry mirror settings ([#429](https://github.com/SUSE/elemental/pull/429)):

```yaml
# kubernetes/config/registries.yaml
mirrors:
  registry.suse.com:
    endpoint:
      - https://registry.suse.com/v2
```

### Features

- Added `elemental3 init` and configuration writing support to scaffold a default configuration directory with `install.yaml`, `release.yaml`, `butane.yaml`, `network/`, and `kubernetes/` ([#384](https://github.com/SUSE/elemental/pull/384)).
- Added `elemental3 release-info` for local file and OCI release manifest inspection, including Markdown output ([#392](https://github.com/SUSE/elemental/pull/392), [#407](https://github.com/SUSE/elemental/pull/407)).
- Added schema-aware parsing and validation for v0 configuration files, release manifests, deployment definitions, Kubernetes, and Helm inputs ([#375](https://github.com/SUSE/elemental/pull/375), [#379](https://github.com/SUSE/elemental/pull/379), [#421](https://github.com/SUSE/elemental/pull/421), [#360](https://github.com/SUSE/elemental/pull/360), [#365](https://github.com/SUSE/elemental/pull/365)).
- Added Kubernetes artifact unpacking from the Core Platform release manifest image ([#399](https://github.com/SUSE/elemental/pull/399)).
- Added `registries.yaml` support under `kubernetes/config` ([#429](https://github.com/SUSE/elemental/pull/429)).
- Added Helm authentication credentials for release-selected charts and user-defined Kubernetes Helm charts, with generated auth secrets for HTTP and OCI chart sources ([#387](https://github.com/SUSE/elemental/pull/387), [#443](https://github.com/SUSE/elemental/pull/443)).
- Added priority manifest deployment so generated auth secrets and other priority resources are applied before HelmChart resources ([#387](https://github.com/SUSE/elemental/pull/387)).
- Added containerd-backed local OCI extraction for images already unpacked in a running containerd daemon ([#422](https://github.com/SUSE/elemental/pull/422)).
- Added progress bars for OCI extraction during customization ([#353](https://github.com/SUSE/elemental/pull/353)).
- Added support for reading local OS images in the Makefile integration flow ([#380](https://github.com/SUSE/elemental/pull/380)).
- Added Docker image targets for `elemental3` and `elemental3ctl` runners ([#324](https://github.com/SUSE/elemental/pull/324)).
- Added custom Discoverable Partitions Specification identifiers for recovery and config partitions, plus architecture-specific root partition identifiers for systemd-repart ([#313](https://github.com/SUSE/elemental/pull/313)).
- Added riscv64 EFI boot file handling ([#380](https://github.com/SUSE/elemental/pull/380)).

### Changes

- Switched default image and artifact examples to `registry.suse.com` ([#400](https://github.com/SUSE/elemental/pull/400)).
- Removed `elemental3ctl` system extension image references from release manifests and examples ([#415](https://github.com/SUSE/elemental/pull/415)).
- Changed release manifest resolution and data structures from Product extension to Solution extension ([#462](https://github.com/SUSE/elemental/pull/462)).
- Changed release-info digest reporting to use image config digests instead of image digests ([#410](https://github.com/SUSE/elemental/pull/410)).
- Changed local OCI extraction to stream from local container storage and use containerd-specific extraction only when both `--local` and `CONTAINERD_SOCK` are set ([#402](https://github.com/SUSE/elemental/pull/402), [#422](https://github.com/SUSE/elemental/pull/422)).
- Changed Kubernetes first-boot installation and configuration to unpack release-provided artifacts, preserve existing RKE2 `config.yaml` content, and fail early when Kubernetes is enabled but absent from the Core Platform manifest ([#399](https://github.com/SUSE/elemental/pull/399), [#439](https://github.com/SUSE/elemental/pull/439)).
- Changed Kubernetes resource creation retries to distinguish real failures from `AlreadyExists` responses ([#411](https://github.com/SUSE/elemental/pull/411)).
- Changed reset/install partitioning to stop forcing a sector size for systemd-repart ([#463](https://github.com/SUSE/elemental/pull/463)).
- Changed recovery and config partitions to use ext4 by default and adjusted installer-media recovery partition sizing ([#337](https://github.com/SUSE/elemental/pull/337)).
- Changed deployment partition role naming from `data` to `generic`, added a dedicated `config` role, and set the default crypto policy explicitly ([#313](https://github.com/SUSE/elemental/pull/313), [#365](https://github.com/SUSE/elemental/pull/365)).
- Changed overlay handling so customization only sets an overlay tree when an overlay directory exists ([#442](https://github.com/SUSE/elemental/pull/442)).
- Changed SELinux upgrade relabeling to distinguish immutable, snapshotted, and shared persistent paths; shared persistent paths are excluded from forced relabeling and trigger first-boot autorelabeling when needed ([#450](https://github.com/SUSE/elemental/pull/450), [#453](https://github.com/SUSE/elemental/pull/453), [#455](https://github.com/SUSE/elemental/pull/455)).
- Changed CLI plumbing from `urfave/cli/v2` to `urfave/cli/v3` and consolidated common flag names/descriptions ([#350](https://github.com/SUSE/elemental/pull/350)).
- Changed build and test targets to use explicit runner images and explicit `ROOTLESSKIT=yes` for rootlesskit tests ([#324](https://github.com/SUSE/elemental/pull/324), [#434](https://github.com/SUSE/elemental/pull/434)).
- Removed deprecated `elemental3 build` references from docs and workflows ([#366](https://github.com/SUSE/elemental/pull/366)).

### Fixes

- Fixed Helm authentication for HTTP chart repositories ([#443](https://github.com/SUSE/elemental/pull/443)).
- Fixed split-mode overlay directory handling ([#442](https://github.com/SUSE/elemental/pull/442)).
- Fixed `elemental3 init` so it avoids overwriting existing configuration and accepts the target directory as a positional argument ([#398](https://github.com/SUSE/elemental/pull/398)).
- Fixed RKE2 installation flow to exit early when the install script fails ([#399](https://github.com/SUSE/elemental/pull/399)).
- Fixed kernel command-line handling so the same value is not added more than once ([#361](https://github.com/SUSE/elemental/pull/361)).
- Fixed riscv64 bootloader support where shim is not available ([#380](https://github.com/SUSE/elemental/pull/380)).
- Fixed first-run behavior in arm64 containers by creating loop device nodes when needed ([#406](https://github.com/SUSE/elemental/pull/406)).
- Fixed local OCI unpack tests so they do not depend on static tag digests ([#402](https://github.com/SUSE/elemental/pull/402)).
- Fixed logger calls that attempted unsupported error wrapping ([#425](https://github.com/SUSE/elemental/pull/425)).
- Fixed mocked runner debug messages and formatting support ([#426](https://github.com/SUSE/elemental/pull/426), [#427](https://github.com/SUSE/elemental/pull/427)).
- Fixed integer conversion risks by using `uint64` for disk size conversions and MiB values ([#464](https://github.com/SUSE/elemental/pull/464)).
- Fixed typos and codespell findings across code, docs, examples, and comments, including Podman command syntax in documentation ([#328](https://github.com/SUSE/elemental/pull/328), [#329](https://github.com/SUSE/elemental/pull/329), [#362](https://github.com/SUSE/elemental/pull/362), [#454](https://github.com/SUSE/elemental/pull/454), [#468](https://github.com/SUSE/elemental/pull/468)).

### Documentation

- Added first-time-use cookbook, documentation index, troubleshooting guide, toolbox documentation, and systemd system extension documentation ([#390](https://github.com/SUSE/elemental/pull/390), [#391](https://github.com/SUSE/elemental/pull/391), [#320](https://github.com/SUSE/elemental/pull/320), [#270](https://github.com/SUSE/elemental/pull/270)).
- Expanded filesystem layout documentation and replaced the old filesystem modes page ([#376](https://github.com/SUSE/elemental/pull/376)).
- Updated release manifest, configuration directory, and image customization docs for Solution terminology, `schema: v0`, `kubernetes/cluster.yaml`, Kubernetes release manifest components, Helm credentials, `registries.yaml`, and the revamped RKE2 installation workflow ([#370](https://github.com/SUSE/elemental/pull/370), [#375](https://github.com/SUSE/elemental/pull/375), [#387](https://github.com/SUSE/elemental/pull/387), [#399](https://github.com/SUSE/elemental/pull/399), [#421](https://github.com/SUSE/elemental/pull/421), [#429](https://github.com/SUSE/elemental/pull/429), [#462](https://github.com/SUSE/elemental/pull/462)).
- Updated single-node and multi-node customization examples, network addresses, and registry mirror examples ([#399](https://github.com/SUSE/elemental/pull/399), [#429](https://github.com/SUSE/elemental/pull/429)).
- Clarified the purpose and audience of the Ignition integration documentation ([#435](https://github.com/SUSE/elemental/pull/435)).
- Removed the Integration Tests badge from README/status surfaces ([#420](https://github.com/SUSE/elemental/pull/420)).

### Testing And CI

- Added OBS pull request workflows, local OBS wait actions, and PR-only integration tests that wait for OBS artifacts before running ([#319](https://github.com/SUSE/elemental/pull/319), [#351](https://github.com/SUSE/elemental/pull/351), [#357](https://github.com/SUSE/elemental/pull/357), [#358](https://github.com/SUSE/elemental/pull/358)).
- Added customize ISO/RAW integration tests and configuration-directory test data ([#331](https://github.com/SUSE/elemental/pull/331)).
- Reworked openQA job groups around Elemental container, ISO image, OS image, release manifest, and RKE2 container validation ([#359](https://github.com/SUSE/elemental/pull/359), [#377](https://github.com/SUSE/elemental/pull/377), [#393](https://github.com/SUSE/elemental/pull/393), [#408](https://github.com/SUSE/elemental/pull/408), [#412](https://github.com/SUSE/elemental/pull/412)).
- Added or updated openQA coverage for release manifest validation, customize ISO/RAW generation, recovery testing, FIPS testing, single-node Kubernetes, multi-node Kubernetes, newer Rancher versions, and RKE2 validation; RKE2-oriented tests now use 8 GiB RAM ([#377](https://github.com/SUSE/elemental/pull/377), [#408](https://github.com/SUSE/elemental/pull/408), [#412](https://github.com/SUSE/elemental/pull/412), [#428](https://github.com/SUSE/elemental/pull/428)).
- Added a `lint` Makefile target ([#367](https://github.com/SUSE/elemental/pull/367)).
- Pinned GitHub Actions to commit SHAs, disabled persistent checkout credentials, and disabled Go action caching where credentials could persist unexpectedly ([#467](https://github.com/SUSE/elemental/pull/467)).
- Switched workflow package installation commands from `apt` to `apt-get` ([#348](https://github.com/SUSE/elemental/pull/348)).
- Added rootlesskit-gated unit test execution ([#434](https://github.com/SUSE/elemental/pull/434)).
- Added unit tests for `init`, `release-info`, configuration validation, release manifest validation, `registries.yaml`, containerd extraction, partition handling, SELinux relabeling, and Kubernetes setup ([#384](https://github.com/SUSE/elemental/pull/384), [#392](https://github.com/SUSE/elemental/pull/392), [#360](https://github.com/SUSE/elemental/pull/360), [#379](https://github.com/SUSE/elemental/pull/379), [#429](https://github.com/SUSE/elemental/pull/429), [#434](https://github.com/SUSE/elemental/pull/434), [#313](https://github.com/SUSE/elemental/pull/313), [#455](https://github.com/SUSE/elemental/pull/455), [#399](https://github.com/SUSE/elemental/pull/399)).
- Moved test assets under `tests/testdata` ([#331](https://github.com/SUSE/elemental/pull/331)).

### Dependencies

- Updated Go from 1.25 to 1.26.3 ([#452](https://github.com/SUSE/elemental/pull/452)).
- Updated `github.com/containerd/containerd/v2` to v2.3.1 and added `github.com/containerd/platforms` ([#321](https://github.com/SUSE/elemental/pull/321), [#385](https://github.com/SUSE/elemental/pull/385), [#422](https://github.com/SUSE/elemental/pull/422), [#466](https://github.com/SUSE/elemental/pull/466)).
- Updated `github.com/coreos/butane` to v0.28.0 ([#343](https://github.com/SUSE/elemental/pull/343), [#374](https://github.com/SUSE/elemental/pull/374), [#461](https://github.com/SUSE/elemental/pull/461)).
- Updated `github.com/coreos/ignition/v2` to v2.26.0 ([#317](https://github.com/SUSE/elemental/pull/317), [#323](https://github.com/SUSE/elemental/pull/323), [#461](https://github.com/SUSE/elemental/pull/461)).
- Updated `github.com/google/go-containerregistry` to v0.21.6 ([#369](https://github.com/SUSE/elemental/pull/369), [#373](https://github.com/SUSE/elemental/pull/373), [#381](https://github.com/SUSE/elemental/pull/381), [#413](https://github.com/SUSE/elemental/pull/413), [#444](https://github.com/SUSE/elemental/pull/444)).
- Added `github.com/go-playground/validator/v10` ([#360](https://github.com/SUSE/elemental/pull/360), [#365](https://github.com/SUSE/elemental/pull/365), [#379](https://github.com/SUSE/elemental/pull/379), [#409](https://github.com/SUSE/elemental/pull/409)).
- Added `github.com/olekukonko/tablewriter` for release-info output ([#392](https://github.com/SUSE/elemental/pull/392), [#407](https://github.com/SUSE/elemental/pull/407)).
- Added `github.com/schollz/progressbar/v3` for OCI extraction progress ([#353](https://github.com/SUSE/elemental/pull/353)).
- Updated `github.com/urfave/cli` from v2 to v3.9.0 ([#350](https://github.com/SUSE/elemental/pull/350), [#356](https://github.com/SUSE/elemental/pull/356), [#372](https://github.com/SUSE/elemental/pull/372), [#405](https://github.com/SUSE/elemental/pull/405), [#446](https://github.com/SUSE/elemental/pull/446)).
- Updated `github.com/onsi/ginkgo/v2` to v2.29.0 and `github.com/onsi/gomega` to v1.40.0 ([#314](https://github.com/SUSE/elemental/pull/314), [#316](https://github.com/SUSE/elemental/pull/316), [#334](https://github.com/SUSE/elemental/pull/334), [#336](https://github.com/SUSE/elemental/pull/336), [#345](https://github.com/SUSE/elemental/pull/345), [#354](https://github.com/SUSE/elemental/pull/354), [#355](https://github.com/SUSE/elemental/pull/355), [#423](https://github.com/SUSE/elemental/pull/423), [#432](https://github.com/SUSE/elemental/pull/432), [#445](https://github.com/SUSE/elemental/pull/445)).
- Updated `github.com/sirupsen/logrus` to v1.9.4 ([#344](https://github.com/SUSE/elemental/pull/344)).
- Updated `golang.org/x/crypto` to v0.52.0 ([#318](https://github.com/SUSE/elemental/pull/318), [#335](https://github.com/SUSE/elemental/pull/335), [#364](https://github.com/SUSE/elemental/pull/364), [#386](https://github.com/SUSE/elemental/pull/386), [#414](https://github.com/SUSE/elemental/pull/414), [#441](https://github.com/SUSE/elemental/pull/441), [#459](https://github.com/SUSE/elemental/pull/459)).
- Updated `golang.org/x/sys` to v0.45.0 ([#440](https://github.com/SUSE/elemental/pull/440)).
- Updated `k8s.io/mount-utils` to v0.36.1 ([#315](https://github.com/SUSE/elemental/pull/315), [#322](https://github.com/SUSE/elemental/pull/322), [#363](https://github.com/SUSE/elemental/pull/363), [#371](https://github.com/SUSE/elemental/pull/371), [#397](https://github.com/SUSE/elemental/pull/397), [#447](https://github.com/SUSE/elemental/pull/447)).
- Updated transitive Docker, OpenTelemetry, gRPC, protobuf, Kubernetes, and containerd-related dependencies through the Go module graph ([#452](https://github.com/SUSE/elemental/pull/452), [#465](https://github.com/SUSE/elemental/pull/465), [#466](https://github.com/SUSE/elemental/pull/466)).

### Maintenance

- Bumped license years to 2026 and added a license bump script ([#325](https://github.com/SUSE/elemental/pull/325)).
- Added constants based on goconst linter findings ([#433](https://github.com/SUSE/elemental/pull/433)).
- Reduced noisy sync logging and improved progress reporting for customization ([#353](https://github.com/SUSE/elemental/pull/353)).
- Updated platform library usage and image build logic for the new runner targets ([#422](https://github.com/SUSE/elemental/pull/422), [#324](https://github.com/SUSE/elemental/pull/324)).
- Moved OBS wait logic into local reusable actions and scripts, and removed pull-request-specific shell logic ([#351](https://github.com/SUSE/elemental/pull/351), [#358](https://github.com/SUSE/elemental/pull/358), [#357](https://github.com/SUSE/elemental/pull/357)).
- Improved wording, comments, and error messages across CLI, containerd, bootloader, installer, repartitioning, SELinux, and test code ([#328](https://github.com/SUSE/elemental/pull/328), [#329](https://github.com/SUSE/elemental/pull/329), [#362](https://github.com/SUSE/elemental/pull/362), [#425](https://github.com/SUSE/elemental/pull/425), [#468](https://github.com/SUSE/elemental/pull/468)).
