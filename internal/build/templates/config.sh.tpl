#!/bin/bash -x

set -e

# Setting users
{{ range .Users -}}
useradd -m {{ .Username }} || true
echo '{{ .Username }}:{{ .Password }}' | chpasswd
{{ end }}

{{- if .NetworkScript }}
# Configuring network

cat <<- EOF > /etc/systemd/system/first-boot-network.service
[Unit]
Description=Configure network on first boot
Before=first-boot-complete.target
Wants=first-boot-complete.target

Before=rke2-server.service rke2-agent.service

ConditionPathExists=!/var/lib/elemental/.network-configuration-attempted
After=network.target NetworkManager.service

[Service]
Type=oneshot
TimeoutStartSec=30
ExecStart=/bin/bash "{{ .NetworkScript }}"

[Install]
WantedBy=multi-user.target
EOF

systemctl enable first-boot-network.service
{{- end }}

# Enabling systemd extensions
cat <<- END > /etc/systemd/system/ensure-sysext.service
[Unit]
BindsTo=systemd-sysext.service
After=systemd-sysext.service
DefaultDependencies=no
# Keep in sync with systemd-sysext.service
ConditionDirectoryNotEmpty=|/etc/extensions
ConditionDirectoryNotEmpty=|/run/extensions
ConditionDirectoryNotEmpty=|/var/lib/extensions
ConditionDirectoryNotEmpty=|/usr/local/lib/extensions
ConditionDirectoryNotEmpty=|/usr/lib/extensions
[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/bin/systemctl daemon-reload
ExecStart=/usr/bin/systemctl restart --no-block sockets.target timers.target multi-user.target
[Install]
WantedBy=sysinit.target
END

systemctl enable ensure-sysext.service

{{- if and .KubernetesDir .ManifestDeployScript }}
# Deploying Kubernetes resources

cat << EOF > /etc/systemd/system/k8s-resource-installer.service
[Unit]
Description=Kubernetes Resources Installer
Requires=rke2-server.service
After=rke2-server.service
ConditionPathExists=/var/lib/rancher/rke2/bin/kubectl
ConditionPathExists=/etc/rancher/rke2/rke2.yaml

[Install]
WantedBy=multi-user.target

[Service]
Type=oneshot
TimeoutSec=900
Restart=on-failure
RestartSec=60
ExecStartPre=/bin/sh -c 'until [ "\$(systemctl show -p SubState --value rke2-server.service)" = "running" ]; do sleep 10; done'
ExecStart=/bin/bash "{{ .ManifestDeployScript }}" 
ExecStartPost=/bin/sh -c "systemctl disable k8s-resource-installer.service"
ExecStartPost=/bin/sh -c "rm -rf /etc/systemd/system/k8s-resource-installer.service"
ExecStartPost=/bin/sh -c 'rm -rf "{{ .KubernetesDir }}"'
EOF

systemctl enable k8s-resource-installer.service
{{- end }}
