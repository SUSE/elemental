[Unit]
Description=Kubernetes Resources Installer
After=k8s-config-installer.service
{{- if .InitNode.Hostname }}
ConditionHost={{ .InitNode.Hostname }}
{{- end }}
StartLimitIntervalSec=1h
StartLimitBurst=5

[Service]
Type=oneshot
# The deployment script bounds its own waits and continues past failed resources,
# so do not limit the time to deploy all resources as a whole.
TimeoutStartSec=infinity
Restart=on-failure
RestartSec=60
{{- if not .InitNode.Hostname }}
EnvironmentFile={{ .RuntimeEnvPath }}
ExecCondition=/bin/sh -c '[ "$${IS_INIT_NODE}" = "true" ]'
{{- end }}
ExecStartPre=/usr/bin/timeout 900 /bin/sh -c 'until [ "$(systemctl show -p SubState --value rke2-server.service)" = "running" ]; do sleep 10; done'
ExecStart=/bin/bash "{{ .ManifestDeployScript }}" 
ExecStartPost=/bin/sh -c "systemctl disable k8s-resource-installer.service"
ExecStartPost=/bin/sh -c "rm -rf /etc/systemd/system/k8s-resource-installer.service"

[Install]
WantedBy=multi-user.target
