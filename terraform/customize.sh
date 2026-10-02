#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PLATFORM="${PLATFORM:-linux/amd64}"
ELEMENTAL_BIN="${ELEMENTAL_BIN:-$ROOT/build/elemental3}"

PROVIDER="${1:?usage: customize.sh <provider> <profile> <work-dir>}"
PROFILE="${2:?usage: customize.sh <provider> <profile> <work-dir>}"
WORK_DIR="${3:?usage: customize.sh <provider> <profile> <work-dir>}"

CONFIG_EXAMPLE="$ROOT/examples/elemental/customize/$PROVIDER"
RUNTIME_EXAMPLE="$ROOT/examples/elemental/runtime-configs/$PROFILE"

WORK_CONFIG_DIR="$WORK_DIR/config"
OUTPUTS="$WORK_DIR/infra.json"
CLUSTER_YAML="$WORK_CONFIG_DIR/kubernetes/cluster.yaml"
RANCHER_YAML="$WORK_CONFIG_DIR/kubernetes/helm/values/rancher.yaml"

[[ -d "$CONFIG_EXAMPLE" ]] || {
    echo "error: no example provided for $PROVIDER under $CONFIG_EXAMPLE" >&2
    exit 1
}

[[ -f "$OUTPUTS" ]] || {
    echo "error: missing infrastructure outputs at $OUTPUTS, run 'make infra' first" >&2
    exit 1
}

API_VIP=$(jq -re .api_vip.value "$OUTPUTS")
API_HOST=$(jq -re .api_host.value "$OUTPUTS")
RANCHER_HOSTNAME=$(jq -re .rancher_hostname.value "$OUTPUTS")

[[ -n "${ELEMENTAL_IMAGE:-}" || -x "$ELEMENTAL_BIN" ]] || {
    echo "error: set ELEMENTAL_IMAGE to an elemental3 container image, or build $ELEMENTAL_BIN with 'make -C $ROOT'" >&2
    exit 1
}

SUDO=""
[[ "$(id -u)" -eq 0 ]] || SUDO="sudo"

if [[ "$(uname)" == "Darwin" ]]; then
    SED_I=(sed -i '')
else
    SED_I=(sed -i)
fi

rm -rf "$WORK_CONFIG_DIR"
cp -a "$CONFIG_EXAMPLE" "$WORK_CONFIG_DIR"

# Sets the value of a key in a specific file.
# usage: set_key <file> <key> <value>
set_key() {
    local file="$1" key="$2" value="$3"

    [[ -f "$file" ]] || {
        echo "error: missing file: $file" >&2
        exit 1
    }

    # Ensure that the key exists in the file and is
    # first in the line preceded only space/tabs.
    grep -qE "^[[:space:]]*$key:" "$file" || {
        echo "error: no '$key' key in $file" >&2
        exit 1
    }

    # Rewrite the key line with the correct value.
    "${SED_I[@]}" -E "s|^([[:space:]]*$key:).*|\1 \"$value\"|" "$file"
}

set_key "$CLUSTER_YAML" apiVIP   "$API_VIP"
set_key "$CLUSTER_YAML" apiHost  "$API_HOST"

[[ -f "$RANCHER_YAML" ]] && set_key "$RANCHER_YAML" hostname "$RANCHER_HOSTNAME"

if [[ -n "${ELEMENTAL_IMAGE:-}" ]]; then
    echo "Customizing image using $ELEMENTAL_IMAGE"
    podman run --rm \
        -v "$WORK_CONFIG_DIR:/config" \
        "$ELEMENTAL_IMAGE" customize \
        --type raw \
        --platform "$PLATFORM" \
        --config-dir /config \
        --output /config/customized.raw
else
    echo "Customizing image using $ELEMENTAL_BIN"
    $SUDO "$ELEMENTAL_BIN" customize \
        --type raw \
        --platform "$PLATFORM" \
        --config-dir "$WORK_CONFIG_DIR" \
        --output "$WORK_CONFIG_DIR/customized.raw"
    $SUDO chown -R "$(id -u):$(id -g)" "$WORK_CONFIG_DIR"
fi

rm -rf "$WORK_DIR/ignition"
mkdir -p "$WORK_DIR/ignition"

echo "Converting butane configurations to ignition"
for name in $(jq -re '.node_names.value[]' "$OUTPUTS"); do
    # If no sub-dir is present, then assume single-node setup.
    dir="$RUNTIME_EXAMPLE/$name"
    [[ -d "$dir" ]] || dir="$RUNTIME_EXAMPLE"

    ign_out=$WORK_DIR/ignition/$name.ign
    butane --strict --files-dir "$dir" --output "$ign_out" "$dir/butane.yaml"
done
