[Unit]
Description=Kubernetes Config Installer
ConditionPathExists=!/etc/rancher/rke2/config.yaml
Requires=network-online.target

[Service]
Type=oneshot
TimeoutSec=900
Restart=on-failure
RestartSec=60
# TODO (atanasdinov): Fix context of files and directories instead of only forcing a rebuild and reload of policies
ExecStartPre=/bin/sh -c "semodule -B"
ExecStart=/bin/bash "{{ .ConfigDeployScript }}"
ExecStartPost=/bin/sh -c "systemctl disable k8s-config-installer.service"
ExecStartPost=/bin/sh -c "rm -rf /etc/systemd/system/k8s-config-installer.service"

[Install]
WantedBy=multi-user.target
