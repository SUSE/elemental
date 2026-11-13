# Hardening example

Add the following to `install.yaml` and copy `tailoring.xml` to `hardening/tailoring.xml`
in the configuration directory:

```yaml
cryptoPolicy: fips
hardening:
  tailoring: hardening/tailoring.xml
  datastream: /usr/share/xml/scap/ssg/content/ssg-sle16-ds.xml
```

`tailoring.xml` derives a profile from ANSSI-BP-028 (high) that deselects the bootloader password rule and
changes the `slub_debug` value. Adjust the `benchmark` reference and the base profile to the
datastream shipped in your OS image.

The OS image must already contain `openscap-utils` and `scap-security-guide`: Elemental runs
`oscap` from inside the image, and image-based systems cannot install packages at build time.
Check which datastreams and profiles your image ships with:

```shell
ls /usr/share/xml/scap/ssg/content/ssg-*-ds.xml
oscap info /usr/share/xml/scap/ssg/content/ssg-sle16-ds.xml
```

## RKE2 nodes

Several hardening rules conflict with running RKE2 on an Elemental node. `rke2-tailoring.xml`
defines an RKE2 variant of each profile shipped for SLES 16, each deselecting the same rules,
with the reason for every exception recorded in the file:

| Profile in `install.yaml`        | Based on                                  |
|----------------------------------|-------------------------------------------|
| `base_rke2`                      | `base` (general system security profile)  |
| `anssi_bp28_minimal_rke2`        | ANSSI-BP-028 (minimal)                    |
| `anssi_bp28_intermediary_rke2`   | ANSSI-BP-028 (intermediary)               |
| `anssi_bp28_enhanced_rke2`       | ANSSI-BP-028 (enhanced)                   |
| `anssi_bp28_high_rke2`           | ANSSI-BP-028 (high)                       |
| `pci-dss-4_rke2`                 | PCI-DSS v4.0.1                            |
| `hipaa_rke2`                     | HIPAA                                     |

Copy it to `hardening/rke2-tailoring.xml` in the configuration directory and select a variant:

```yaml
hardening:
  tailoring: hardening/rke2-tailoring.xml
  profile: anssi_bp28_intermediary_rke2
  datastream: /usr/share/xml/scap/ssg/content/ssg-sle16-ds.xml
```

The deselected rules are:

* `mount_option_opt_nosuid`, `mount_option_var_nosuid`: RKE2 runs from `/opt/rke2` (`/usr` is
  read-only) and `/var/lib/rancher/rke2`; `nosuid` prevents the SELinux domain transition of its
  binaries, which then run as `init_t` and are denied network access.
* `mount_option_var_noexec`: RKE2 runs containerd, runc and the kubelet from `/var/lib/rancher/rke2`.
* `sysctl_net_ipv4_ip_forward`: pod networking requires IPv4 forwarding.
* `sysctl_net_ipv4_conf_all_arp_ignore`: `arp_ignore=2` on all interfaces breaks Calico pod
  networking (/32 pod addresses). Set `net.ipv4.conf.all.arp_ignore = 1`, and `2` on the physical
  interface, with a `sysctl.d` drop-in from the OS image or the first boot configuration instead.
* `no_files_unowned_by_user`, `file_permissions_ungroupowned`: container image layers contain files
  owned by IDs that do not exist on the host.
* `restrict_serial_port_logins`: keeps root logins on serial and virtual consoles (e.g. `hvc0`).
* On aarch64 only, `bios_enable_execution_restrictions`: the check looks for the x86 `nx` CPU flag.

The `bios_enable_execution_restrictions` exception only applies to aarch64 nodes: on x86_64, remove
it from each profile so the check still runs. The variants have been validated with RKE2 for
`anssi_bp28_intermediary` and `base`; stricter profiles may need further exceptions.
