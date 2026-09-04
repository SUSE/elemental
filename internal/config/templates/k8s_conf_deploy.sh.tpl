#!/bin/bash

set -uo pipefail

declare -A hosts

{{- range .Nodes }}
hosts[{{ .Hostname }}]={{ .Type }}
{{- end }}

# This is to support both static and DHCP configurations
HOSTNAME=$(</etc/hostname)
[[ -z "${HOSTNAME}" ]] \
  && HOSTNAME=$(</proc/sys/kernel/hostname)

NODETYPE="${hosts[${HOSTNAME}]:-server}"
CONFIGFILE="{{ .KubernetesDir }}/${NODETYPE}.yaml"
REGFILE="{{ .KubernetesDir }}/registries.yaml"

{{- if .InitNode.Hostname }}
if [[ "${HOSTNAME}" = "{{ .InitNode.Hostname }}" ]]; then
  echo "Setting up init node"
  CONFIGFILE={{ .KubernetesDir }}/init.yaml
fi
{{- end }}

# Better to append if a file exist
# Useful if some custom configuration are done at boot
mkdir -p /etc/rancher/rke2
echo "Copying RKE2 config file ${CONFIGFILE}"
cat ${CONFIGFILE} >> /etc/rancher/rke2/config.yaml

if [[ -e "${REGFILE}" ]]; then
  cp "${REGFILE}" /etc/rancher/rke2/registries.yaml
fi

{{- if and .APIVIP4 .APIHost }}
grep -q "{{ .APIVIP4 }} {{ .APIHost }}" /etc/hosts \
  || echo "{{ .APIVIP4 }} {{ .APIHost }}" >> /etc/hosts
{{- end }}

{{- if and .APIVIP6 .APIHost }}
grep -q "{{ .APIVIP6 }} {{ .APIHost }}" /etc/hosts \
  || echo "{{ .APIVIP6 }} {{ .APIHost }}" >> /etc/hosts
{{- end }}

echo "Installing RKE2 from embedded artifacts..."

export INSTALL_RKE2_ARTIFACT_PATH="{{ .InstallPath }}"
export INSTALL_RKE2_TAR_PREFIX=/opt/rke2

if ! sh "{{ .InstallScript }}"; then
  echo "Error: RKE2 installation failed" >&2
  exit 1
fi

systemctl enable --now rke2-${NODETYPE}.service
