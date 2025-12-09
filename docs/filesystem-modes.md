# Filesystem Modes

| Path          | Initramfs | Booted System | 
|---------------|-----------|---------------|
| /etc          | RW        | RW            |
| /var          | RW        | RW            |
| /root         | RW        | RW            |
| /run          | RW        | RW            |
| /tmp          | RW        | RW            |
| /home         | RO        | RW            |
| /opt          | RO        | RW            |
| /srv          | RO        | RW            |
| /usr          | RO*       | RO            |
| /usr/local    | RO        | RW            |

*The /usr filesystem is permanently mounted as Read-Only (RO) by design. All other filesystems listed as RO are unavailable
during initramfs and can be optionally mounted as RW, if necessary, for Ignition (via Butane syntax) or for custom scripts execution on firstboot.
