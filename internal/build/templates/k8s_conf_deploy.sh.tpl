#!/bin/bash

set -uo pipefail

declare -A hosts

{{- range .Nodes }}
hosts[{{ .Hostname }}]={{ .Type }}
{{- end }}

HOSTNAME=$(cat /etc/hostname)
if [ ! "$HOSTNAME" ]; then
    HOSTNAME=$(cat /proc/sys/kernel/hostname)
    if [ ! "$HOSTNAME" ] || [ "$HOSTNAME" = "localhost.localdomain" ]; then
        echo "ERROR: Could not identify whether the host is an RKE2 server or agent due to missing hostname"
        exit 1
    fi
fi

NODETYPE="${hosts[$HOSTNAME]:-none}"
if [ "$NODETYPE" = "none" ]; then
    echo "ERROR: Could not identify whether host '$HOSTNAME' is an RKE2 server or agent"
    exit 1
fi

CONFIGFILE=/etc/rancher/rke2/$NODETYPE.yaml
cp $CONFIGFILE /etc/rancher/rke2/config.yaml

{{- if and .APIVIP4 .APIHost }}
echo "{{ .APIVIP4 }} {{ .APIHost }}" >> /etc/hosts
{{- end }}

{{- if and .APIVIP6 .APIHost }}
echo "{{ .APIVIP6 }} {{ .APIHost }}" >> /etc/hosts
{{- end }}

