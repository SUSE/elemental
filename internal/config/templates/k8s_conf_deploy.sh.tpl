#!/bin/bash

set -uo pipefail

: "${K8S_DIR:={{ .KubernetesDir }}}"
: "${INIT_PATH:=${K8S_DIR}/init.yaml}"
: "${REGFILE:=${K8S_DIR}/registries.yaml}"
: "${BASE_CONF_NAME:=00-elemental-base-rke2-config.yaml}"

declare -A hosts

{{- range .Nodes }}
hosts[{{ .Hostname }}]={{ .Type }}
{{- end }}

# This is to support both static and DHCP configurations
HOSTNAME=$(</etc/hostname)
[[ -z "${HOSTNAME}" ]] \
  && HOSTNAME=$(</proc/sys/kernel/hostname)

: "${NODETYPE:=${hosts[$HOSTNAME]:-}}"
[[ -z "${NODETYPE}" ]] && {
  echo "Error: Undeclared 'NODETYPE' for hostname '${HOSTNAME}'" >&2
  exit 1
}

is_init_node=false
{{- if .InitNode.Hostname }}
[[ "${HOSTNAME}" == "{{ .InitNode.Hostname }}" ]] && is_init_node=true
{{- end }}

: "${IS_INIT_NODE:=${is_init_node}}"
CONFIGFILE="${K8S_DIR}/${NODETYPE}.yaml"
[[ "${IS_INIT_NODE}" == "true" ]] && CONFIGFILE="${INIT_PATH}"

RKE2_CFG_DROP_IN_PATH="/etc/rancher/rke2/config.yaml.d"
mkdir -p "${RKE2_CFG_DROP_IN_PATH}"
cp "${CONFIGFILE}" "${RKE2_CFG_DROP_IN_PATH}/${BASE_CONF_NAME}"

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

systemctl enable --now "rke2-${NODETYPE}.service"
