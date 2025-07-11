#!/bin/bash
set -euo pipefail

MARKER_FILE="/var/lib/elemental/.network-configuration-attempted"

if [ -f "$MARKER_FILE" ]; then
    echo "Marker file '$MARKER_FILE' found. Script already executed, exiting." >&2
    exit 0
fi

/usr/bin/nmc apply --config-dir {{ .ConfigDir }} || {
echo "WARNING: Failed to apply static network configurations." >&2
}

echo "Reloading NetworkManager connection files..."
nmcli connection reload

touch "$MARKER_FILE"

echo "Network configuration attempt completed."
