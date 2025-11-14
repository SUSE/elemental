[Unit]
Description=Kubernetes Config Installer
ConditionPathExists=!/etc/rancher/rke2/config.yaml
Requires=network-online.target

[Service]
Type=oneshot
TimeoutSec=900
Restart=on-failure
RestartSec=60
# TODO (atanasdinov): Figure out a declarative, non-hardcoded approach for installing selinux modules
ExecStartPre=/bin/sh -c "semodule -i /usr/share/selinux/packages/rke2.pp"
ExecStart=/bin/bash "{{ .ConfigDeployScript }}"
ExecStartPost=/bin/sh -c "systemctl disable k8s-config-installer.service"
ExecStartPost=/bin/sh -c "rm -rf /etc/systemd/system/k8s-config-installer.service"

[Install]
WantedBy=multi-user.target
