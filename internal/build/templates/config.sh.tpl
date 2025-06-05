#!/bin/bash -x

set -e

# Setting users
{{ range .Users -}}
useradd -m {{ .Username }} || true
echo '{{ .Username }}:{{ .Password }}' | chpasswd
{{ end }}
