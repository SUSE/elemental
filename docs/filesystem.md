# Filesystem Layout and Data Persistence

This document explains how the filesystem and partition layout works in Elemental 3 systems and how data persists across
updates.

## Filesystem Modes

| Path         | Initramfs | Booted System |
|--------------|-----------|---------------|
| `/etc`       | RW        | RW            |
| `/var`       | RW        | RW            |
| `/root`      | RW        | RW            |
| `/run`       | RW        | RW            |
| `/tmp`       | RW        | RW            |
| `/home`      | RO        | RW            |
| `/opt`       | RO        | RW            |
| `/srv`       | RO        | RW            |
| `/usr`       | RO*       | RO            |
| `/usr/local` | RO        | RW            |

\* The `/usr` filesystem is permanently mounted as Read-Only (RO) by design. All other filesystems listed as RO are
unavailable during initramfs and can be optionally mounted as RW, if necessary, for Ignition (via Butane syntax) or for
custom scripts execution on firstboot.

## Partition Layout

The default deployment creates the following partition structure:

| Partition | Label      | Filesystem | Mount Point | Size          | Required | Purpose                        |
|-----------|------------|------------|-------------|---------------|----------|--------------------------------|
| EFI       | `EFI`      | vfat       | `/boot/efi` | Fixed         | Yes      | Bootloader                     |
| Recovery  | `RECOVERY` | ext4       | N / A       | Fixed         | Yes      | Install and restore operations |
| System    | `SYSTEM`   | btrfs      | `/`         | All remaining | Yes      | System and user data           |
| Config    | `CONFIG`   | ext4       | N / A       | Variable      | No       | Firstboot configuration        |

## Btrfs Subvolume Layout

The system partition uses btrfs with the following subvolume structure:

- **Root**: Mounted read-only (`ro=vfs`), contains the immutable OS image
- **RW Volumes**: Separate btrfs subvolumes for mutable data

| Path         | Snapshotted | NoCopyOnWrite | Mounted in Initramfs |
|--------------|-------------|---------------|----------------------|
| `/etc`       | Yes         | No            | Yes                  |
| `/var`       | No          | Yes           | Yes                  |
| `/root`      | No          | No            | Yes                  |
| `/home`      | No          | No            | No                   |
| `/opt`       | No          | No            | No                   |
| `/srv`       | No          | No            | No                   |
| `/usr/local` | No          | No            | No                   |

### Subvolume Properties

- **Snapshotted** (`/etc`): This subvolume is included in btrfs snapshots, allowing configuration to be versioned
  alongside the OS.
- **NoCopyOnWrite** (`/var`): Disables copy-on-write for this subvolume, which is recommended for directories containing
  databases, logs, and container storage.
- **Mounted in Initramfs** (`x-initrd.mount`): These subvolumes are mounted early in the boot process.

## How Upgrades Work

OS upgrades use a btrfs snapper snapshots layout:

1. The root filesystem is an immutable snapshot
2. `/etc` is a nested and snapshotted subvolume
3. Mutable parts of the system (`/var`, `/root`, `/home`, `/opt`, `/srv`, `/usr/local`) are shared btrfs subvolumes
4. A new boot entry is generated for each snapshot
5. Bootloader, kernel, and initrd are installed to the ESP

### Upgrade Process

During an upgrade:

1. A new transaction (snapshot) is initialized
2. The new OS image content is synced to the transaction
3. RW volumes are merged into the new snapshot
4. The fstab is updated for the new snapshot
5. The snapshot is locked (made immutable)
6. A new boot entry is created pointing to the new snapshot
7. The transaction is committed

If an upgrade fails at any point, the transaction is rolled back and the system remains on the previous snapshot.

## Data Persistence Across Updates

Because RW volumes are **shared btrfs subvolumes** (not part of the root snapshot), data in these locations persists
across updates:

| Directory    | Persists Across Updates | Notes                                       |
|--------------|-------------------------|---------------------------------------------|
| `/etc`       | Yes (merged)            | Snapshotted; changes merged to new snapshot |
| `/var`       | Yes                     | Shared subvolume; not affected by updates   |
| `/root`      | Yes                     | Shared subvolume; not affected by updates   |
| `/home`      | Yes                     | Shared subvolume; not affected by updates   |
| `/opt`       | Yes                     | Shared subvolume; not affected by updates   |
| `/srv`       | Yes                     | Shared subvolume; not affected by updates   |
| `/usr/local` | Yes                     | Shared subvolume; not affected by updates   |
| `/usr`       | No                      | Part of immutable root; replaced on update  |

### Example: `/var/lib/rancher`

Container and Kubernetes data stored in `/var/lib/rancher` persists because `/var` is a shared btrfs subvolume with
copy-on-write disabled. When an upgrade occurs:

1. A new root snapshot is created from the new OS image
2. The existing `/var` subvolume (containing `/var/lib/rancher`) is mounted into the new snapshot
3. After reboot, the same `/var` data is accessible from the new OS

### The `/etc` Directory

The `/etc` directory is handled specially:

- It is a **nested subvolume** within the root
- It is **snapshotted** along with the root filesystem
- During upgrades, changes are **merged** into the new snapshot

This means:

- System configuration is versioned with OS snapshots
- Local configuration changes persist across updates via the merge process
- Rolling back the OS also rolls back `/etc` to match that OS version

#### 3 Way Merge on Snapshotted Directories

The three way merge concept is common in distributed version control systems such as git. It essentially describes
the process of merging two branches. The same concept can easily be applied to directory trees. In Elemental the
essential focus is around the contents of `/etc` in a running system.

The merge consists on applying two independent deltas over the same origin. There are the stock default contents of
the current image in which there are some customization applied on top (e.g. users configurations, ssh keys, etc.), this
is the customizations delta. On the other side there is the delta between the stock default contents of the current image
versus the defaults of the new image, the defaults delta.

The merging process applies both deltas over the original content with preference for the customizations delta.
This is a two step process, first applies the defaults delta and then applies the customizations delta. Deltas are
defined at file granularity, meaning the entire file is copied when the delta is applied. This way customized files are
always kept during upgrades, having preference over the defaults delta in case of overlaps. Customized files always win.

> **NOTE:** SE Linux extended attributes are not merged, snapshotted directories are always relabelled according to the current
> SE Linux policy.

#### Merge combinations

Customized files win over new default files. The following matrix aims to illustrate a variety of combinations
and the expected outcome in each case.

| Original defaults | New defaults     | Customized state | Merge result     |
| ----------------- | ---------------- | ---------------- | ---------------- |
| exists            | modified         | not modified     | new default file |
| exists            | modified         | modified         | customized file  |
| exists            | modified         | deleted          | file deleted     |
| exists            | not modified     | modified         | customized file  |
| exists            | not modified     | deleted          | file deleted     |
| exists            | deleted          | not modified     | file deleted     |
| exists            | deleted          | modified         | customized file  |
| missing           | missing          | user created     | user created     |
| missing           | newly added      | user created     | user created     |
| missing           | newly added      | missing          | newly added      |


## Configuring Additional Disks

Since Elemental 3 supports Butane input, additional disks can be configured via Ignition on firstboot.

This configuration is processed during the firstboot phase before the system becomes operational.

## Rollback

Because each upgrade creates a new btrfs snapshot with its own boot entry:

- Previous system states remain available
- The bootloader can boot any available snapshot
- Rolling back means selecting a previous snapshot to boot
- Shared subvolumes (`/var`, `/home`, etc.) are **not** rolled back—they always contain the latest data
