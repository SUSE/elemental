[Unit]
Description=Elemental Registry
After=network.target
Before=k8s-config-installer.service

[Service]
Type=simple
User=root
WorkingDirectory={{ .RegistryDir }}
ExecStart=/bin/bash "{{ .Script }}"
TimeoutStartSec=300
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
