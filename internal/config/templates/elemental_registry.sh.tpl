#!/bin/bash

set -euo pipefail

REGISTRY_DIR="{{ .RegistryDir }}"
HAULER="${REGISTRY_DIR}/hauler"
STORE="${REGISTRY_DIR}/store"
BACKEND="${REGISTRY_DIR}/registry"

rm -rf "${STORE}" "${BACKEND}"

for archive in "${REGISTRY_DIR}"/*-{{ .ArchiveSuffix }}; do
  [ -f "${archive}" ] && "${HAULER}" store load --filename "${archive}" --store "${STORE}" --tempdir "${REGISTRY_DIR}"
done

exec "${HAULER}" store serve registry --port {{ .Port }} --store "${STORE}" --directory "${BACKEND}"
