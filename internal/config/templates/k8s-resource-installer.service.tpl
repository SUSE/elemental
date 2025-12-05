[Unit]
Description=Kubernetes Resources Installer
After=rke2-server.service
ConditionHost={{ .InitHostname }}

[Service]
Type=oneshot
TimeoutSec=900
Restart=on-failure
RestartSec=60
ExecStartPre=/bin/sh -c 'until [ "$(systemctl show -p SubState --value rke2-server.service)" = "running" ]; do sleep 10; done'
ExecStart=/bin/bash "{{ .ManifestDeployScript }}" 
ExecStartPost=/bin/sh -c "systemctl disable k8s-resource-installer.service"
ExecStartPost=/bin/sh -c "rm -rf /etc/systemd/system/k8s-resource-installer.service"
ExecStartPost=/bin/sh -c 'rm -rf "{{ .KubernetesDir }}"'

[Install]
WantedBy=multi-user.target
