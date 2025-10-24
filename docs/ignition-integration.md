# Elemental and Ignition Integration

This section provides an overview of how to configure the OS at firstboot with Ignition.

## The runtime context

By default, `elemental3ctl` creates two partitions for its operating system images: an ESP partition
(bootloader, kernel and initrd) and a Linux partition (the OS itself). The Linux partition is a btrfs
file system including several subvolumes where there is the default read-only subvolume mounted as the
root device and a list of read-write subvolumes which are mounted in paths that are expected or required
to be read-write, such as `/etc` or `/var`. The default subvolume is a btrfs read-only subvolume so
regardless of mounting it with or without read-write capabilities, the filesystem will prevent any write
in there causing a failure in any attempt.

Default read-write subvolumes for `elemental3ctl` installations are:

* `/var`
* `/root`
* `/etc`
* `/opt`
* `/srv`
* `/home`

The only subvolumes that are mounted in early boot inside initrd before switching root are `/etc`, `/root`
and `/var`. This is relevant for first boot configuration as these are the subvolumes that tools such as
Ignition are capable of modifying at boot time. Those are the subvolumes that include the `x-initrd.mount`
option in `/etc/fstab` file, stating those are mounted before switching root.

`elemental3ctl` can create additional partitions and different subvolumes beyond the default settings.
`elemental3ctl` supports installation parameters provided by a yaml file, the default disk setup is
equivalent to the following one:

```yaml
disks:
- partitions:
  - label: EFI
    role: efi
    mountPoint: /boot
    size: 1024
    mountOpts: ['defaults', 'x-systemd.automount']
  - label: SYSTEM
    role: system
    mountPoint: /
    size: 0
    filesystem: btrfs
    rwVolumes:
    - path: /var
      noCopyOnWrite: true
      mountOpts: ['x-initrd.mount']
    - path: /root
      mountOpts: ['x-initrd.mount']
    - path: /etc
      snapshotted: true
      mountOpts: ['x-initrd.mount']
    - path: /opt
    - path: /srv
    - path: /home
```

Which could be adapted to include an additional partition by adding another item into the partitions
list. Adding an Ignition partition of 512MiB could done with:

```yaml
disks:
- target: /dev/loop0
  partitions:
  - label: EFI
    role: efi
    mountPoint: /boot
    size: 1024
    mountOpts: ['defaults', 'x-systemd.automount']
  - label: IGNITION
    role: data
    mountPoint: /firstboot
    size: 512
    filesystem: btrfs
    mountOpts: ['defaults', 'x-systemd.automount']
  - label: SYSTEM
    role: system
    mountPoint: /
    size: 0
    filesystem: btrfs
    rwVolumes:
    - path: /var
      noCopyOnWrite: true
      mountOpts: ['x-initrd.mount']
    - path: /root
      mountOpts: ['x-initrd.mount']
    - path: /etc
      snapshotted: true
      mountOpts: ['x-initrd.mount']
    - path: /opt
    - path: /srv
    - path: /home
```

## Configuring via Ignition

SUSE Linux Micro's Ignition comes with certain constraints when this is used in conjunction to an image based
(also referred as immutable) OS. Most noticeable aspect is that the root volume, despite ignition attempts to
remount it as a read-write, is still sealed and operating in read-only mode. In practice this essentially
means that changes over the RO areas of the system are forbidden and any attempt to write there is leading to
an ignition failure which translates into a non booting system.

The Ignition configuration file can be provided in a variety of ways depending on the platform (e.g. some
public cloud provider) the system is running, however a common and easy way to provide the configuration is by using a
volume with a filesystem labeled `IGNITION` (not key sensitive), containing a configuration file stored at the
`/ignition/config.ign` path. This can be achieved by adding an extra block device,
such as a USB stick, to the machine, or by installing a system including a partition with a filesystem labeled as `IGNITION`.

More specific details of the SUSE Linux Micro's Ignition can be found in the [downstream repository](https://src.opensuse.org/SLFO-pool/ignition#readme).

### Specific non supported Ignition features

There are some Ignition non supported features in Elemental based systems.

* `kernelArguments` configuration is not functional as it requires a specific grub2 configuration which is not
  aligned with today's bootlodaer configuration provided by `elemental3ctl`. Adaptations on the bootloader setup and/or in
  `ignition-kargs-helper` script of Ignition's package would be required.

### Ignition configuration example

The configuration example below demonstrates the common use cases of Ignition, such as configuring users, dropping
files into the system and handling systemd services. Find full documentation about Ignition at the
[official website](https://coreos.github.io/ignition/) of the project.

```json
{
  "ignition": { "version": "3.5.0" },
  "passwd": {
    "users": [
      {
        "name": "root",
        "passwordHash": "$6$6RmQVwdZ3p0TLx8t$e1RdFSCTNIJRe9fDGnssaQIrTblBbApEE8PCu4FGS/1/PToT9g/GDT05RSF.Ijm6wKs8m3mApYPMw/.oUc0MS0",
        "sshAuthorizedKeys": ["ssh-rsa veryLongRSAPublicKey"]
      },
      {
        "name": "pipo",
        "passwordHash": "$6$6RmQVwdZ3p0TLx8t$e1RdFSCTNIJRe9fDGnssaQIrTblBbApEE8PCu4FGS/1/PToT9g/GDT05RSF.Ijm6wKs8m3mApYPMw/.oUc0MS0",
        "system": false
      }
    ]
  },
  "systemd": {
    "units": [
      {
        "name": "example.service",
        "enabled": true,
        "contents": "[Service]\nType=oneshot\nExecStart=/usr/bin/echo Hello World\n\n[Install]\nWantedBy=multi-user.target"
      }
    ]
  },
  "storage": {
    "files": [
      {
        "path": "/var/test.txt",
        "mode": 420,
        "contents": {
          "source": "data:,testcontents"
        },
        "overwrite": true
      },
      {
        "path": "/etc/subdir/test.txt",
        "mode": 420,
        "contents": {
          "source": "data:,test%20contents%20in%20etc"
        },
        "overwrite": true
      }
    ],
    "filesystems": [
      {
        "path": "/home",
        "device": "/dev/disk/by-label/SYSTEM",
        "format": "btrfs",
        "mountOptions": ["subvol=/@/home"],
        "wipeFilesystem": false
      }
    ]
  }
}
```

Note the `pipo` user is added as a regular user, hence Ignition will create a `/home/pipo` folder. This is would not be possible
without additional configuration because by default `/home` subvolume is no including the `x-initrd.mount` option in fstab
and so that it is not mounted in initrd phases. To workaround this issue without customizing the installation, you can instruct
Ignition to mount filesystems, this is in fact, the purpose of the `filesystems` list in the above example. Note that under
`filesystems` the `/home` mount point is defined with the appropriate mount options.
