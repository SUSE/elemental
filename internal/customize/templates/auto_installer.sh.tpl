#!/bin/bash

mkdir -p /etc/systemd/system/serial-getty@ttyS0.service.d

cat > /etc/systemd/system/serial-getty@ttyS0.service.d/override.conf << EOF
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin root --noclear %I $TERM
EOF

# Show the system journal on the first virtual terminal, so that the installation
# progress is visible right away, and keep a root shell on the second one.
mkdir -p /etc/systemd/system/getty@tty1.service.d

cat > /etc/systemd/system/getty@tty1.service.d/override.conf << EOF
[Service]
ExecStart=
ExecStart=-/usr/bin/journalctl --follow --no-pager
EOF

mkdir -p /etc/systemd/system/getty@tty2.service.d

cat > /etc/systemd/system/getty@tty2.service.d/override.conf << EOF
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin root --noclear %I $TERM
EOF

systemctl enable getty@tty2.service

# Ensure extensions included in ISO's /extensions folder are loaded at boot
# ISO filesystem is mounted at /run/initramfs/live folder
rm -rf /run/extensions
ln -s /run/initramfs/live/extensions /run/extensions

cat > /etc/systemd/system/elemental-autoinstall.service << EOF
[Unit]
Description=Elemental Autoinstall
After=multi-user.target
ConditionPathExists=/run/initramfs/live/Install/install.yaml
ConditionFileIsExecutable=/usr/bin/elemental3ctl
{{- if eq .MediaType "raw" }}
ConditionKernelCommandLine=elm.recovery
ConditionKernelCommandLine=elm.reset
{{- end }}
OnSuccess=reboot.target
StartLimitIntervalSec=600
StartLimitBurst=3

[Service]
Type=oneshot
{{- if eq .MediaType "iso" }}
ExecStart=/usr/bin/elemental3ctl --debug install
{{- else }}
ExecStart=/usr/bin/elemental3ctl --debug reset
{{- end }}
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl enable elemental-autoinstall.service
