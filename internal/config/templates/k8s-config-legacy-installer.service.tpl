# This is a legacy template to support OSes builds that are
# including Elemental < v3.1. For those, rke2-selinux package and
# SE Linux management in Elemental are not well aligned and
# require a manual rke2 modules installation at boot time.

[Unit]
Description=Kubernetes Installation and Configuration
ConditionFirstBoot=true
Requires=network-online.target

[Service]
Type=oneshot
TimeoutSec=900
Restart=on-failure
RestartSec=60
EnvironmentFile=-{{ .RuntimeEnvPath }}

# Install SE Linux modules hack
ExecStartPre=/bin/sh -c "semodule -i /usr/share/selinux/packages/rke2.pp"

ExecStart=/bin/bash "{{ .ConfigDeployScript }}"
ExecStartPost=/bin/sh -c "systemctl disable k8s-config-installer.service"
ExecStartPost=/bin/sh -c "rm -rf /etc/systemd/system/k8s-config-installer.service"

[Install]
WantedBy=multi-user.target
