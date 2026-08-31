[Unit]
Description=Kubernetes Resources Installer
After=k8s-config-installer.service
{{- if .InitNode.Hostname }}
ConditionHost={{ .InitNode.Hostname }}
{{- end }}

[Service]
Type=oneshot
TimeoutSec=900
Restart=on-failure
RestartSec=60
{{- if not .InitNode.Hostname }}
EnvironmentFile={{ .RuntimeEnvPath }}
ExecCondition=/bin/sh -c '[ "$${IS_INIT_NODE}" = "true" ]'
{{- end }}
ExecStartPre=/bin/sh -c 'until [ "$(systemctl show -p SubState --value rke2-server.service)" = "running" ]; do sleep 10; done'
ExecStart=/bin/bash "{{ .ManifestDeployScript }}" 
ExecStartPost=/bin/sh -c "systemctl disable k8s-resource-installer.service"
ExecStartPost=/bin/sh -c "rm -rf /etc/systemd/system/k8s-resource-installer.service"

[Install]
WantedBy=multi-user.target
