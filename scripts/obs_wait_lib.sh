#!/bin/bash

set -uo pipefail

: "${BUILD_ENDPOINT:=https://build.opensuse.org/public/build}"
: "${DOCKER:=docker}"
: "${TIMEOUT:=3600}"
: "${SLEEP:=60}"
: "${ARCH:=$(uname -m)}"

is_pkg_built_complete() {
    local proj="$1"
    local repo="$2"
    local pkg="$3"
    local pkg_url="$BUILD_ENDPOINT/$proj/_result?repository=$repo&arch=$ARCH&package=$pkg"

    local status result
    result="$(curl -fsSL "$pkg_url")" || {
        echo "Failed retrieving result for '$proj/$ARCH/$repo/$pkg' package from $pkg_url"
        return 1
    }

    status="$(xmllint --xpath 'string(//status/@code)' - <<<"$result")" || {
        echo "Failed retrieving status of '$proj/$ARCH/$repo/$pkg' package from $pkg_url"
        return 1
    }

    echo "$proj/$ARCH/$repo/$pkg status: $status"
    [[ "$status" =~ ^(succeeded|excluded|disabled)$ ]]
}

is_repo_clean() {
    local proj="$1"
    local repo="$2"
    local repository_url="$BUILD_ENDPOINT/$proj/_result?repository=$repo&arch=$ARCH"
    
    local result dirty
    result="$(curl -fsSL "$repository_url")" || {
        echo "Failed retrieving result for '$proj/$ARCH/$repo' repository from $repository_url"
        return 1
    }

    dirty="$(xmllint --xpath 'string(//result/@dirty)' - <<<"$result")" || {
        echo "Failed validating dirty state for '$proj/$ARCH/$repo' repository from $repository_url"
        return 1
    }

    [[ "$dirty" == "true" ]] && {
        echo "Repo '$proj/$ARCH/$repo' is in a dirty state"
        return 1
    }
    return 0
}

is_repo_published() {
    local proj="$1"
    local repo="$2"
    local repository_url="$BUILD_ENDPOINT/$proj/_result?repository=$repo&arch=$ARCH"

    local result code state
    result="$(curl -fsSL "$repository_url")" || {
        echo "Failed retrieving result for '$proj/$ARCH/$repo' repository from $repository_url"
        return 1
    }

    code="$(xmllint --xpath 'string(//result/@code)' - <<<"$result")" || {
        echo "Failed retrieving result code for '$proj/$ARCH/$repo' repository from $repository_url"
        return 1
    }

    state="$(xmllint --xpath 'string(//result/@state)' - <<<"$result")" || {
        echo "Failed retrieving result state for '$proj/$ARCH/$repo' repository from $repository_url"
        return 1
    }

    echo "Repository '$repo' - code: $code / state: $state"
    [[ "$code" == "published" && "$state" == "published" ]]
}

compare_image_to_source_pkg() {
    local image="$1"       
    local source_obs_pkg="$2"
    local obs_proj="${source_obs_pkg%/*}"
    local obs_pkg_name="${source_obs_pkg##*/}"

    local oci_arch
    case "$ARCH" in
        x86_64)  oci_arch=amd64 ;;
        aarch64) oci_arch=arm64 ;;
        *) echo "Unsupported ARCH: $ARCH"; return 1;;
    esac

    local manifest_digest
    manifest_digest=$(
        $DOCKER manifest inspect "$image" |
            jq -er --arg oci_arch "$oci_arch" '
                first(
                .manifests[]?
                | select(.platform.os == "linux")
                | select(.platform.architecture == $oci_arch)
                | .digest
                )
            '
    ) || return 1
    
    if [[ -z "$manifest_digest" || "$manifest_digest" == "null" ]]; then
        echo "Missing manifest digest for $oci_arch in: $image"
        return 1
    fi

    local img_wo_tag="${image%:*}"
    local registry="${img_wo_tag%%/*}"
    local repo_path="${img_wo_tag#*/}"
    local img_manifest="https://${registry}/v2/${repo_path}/manifests/${manifest_digest}"
    local img_id
    if ! img_id="$(curl -fsSL "$img_manifest" | jq -r '.config.digest')"; then
        echo "Could not fetch image id for $img_manifest manifest"
        return 1
    fi

    if [[ -z "$img_id" || "$img_id" == "null" ]]; then
        echo "No image id in manifest: $img_manifest" 
        return 1
    fi

    local img_id_in_obs="_blob.${img_id}"
    local obs_container_url="$BUILD_ENDPOINT/$obs_proj/containers/$ARCH/$obs_pkg_name"
    if curl -fsSL "$obs_container_url" | grep -Fq "filename=\"${img_id_in_obs}\""; then
        echo "Image '$image' matches state of '$source_obs_pkg' source package. State: '$img_id'"
        return 0
    fi

    echo "Image '$image' state '$img_id' does not match source package state"
    return 1
}

wait_for() {
    local cmd=( "$@" )
    local start_time now elapsed
    
    echo "Running '${cmd[*]}' with timeout $TIMEOUT and retry every $SLEEP seconds.."
    start_time=$(date +%s)
    while :; do
        "${cmd[@]}" && return 0

        now=$(date +%s)
        elapsed=$(( now - start_time ))
        if (( elapsed >= TIMEOUT )); then
            echo "Timeout waiting for: ${cmd[*]}"
            return 1
        fi

        echo "Retrying in ${SLEEP}s.."
        sleep "$SLEEP"
    done
}

wait_for_repo_pkg_build() {
    local proj="$1"
    local repo="$2"
    local repository_url="$BUILD_ENDPOINT/$proj/_result?repository=$repo&arch=$ARCH"
    
    wait_for is_repo_clean "$proj" "$repo" || return 1

    while IFS= read -r pkg; do
        wait_for is_pkg_built_complete "$proj" "$repo" "$pkg" || return 1
    done < <(
        curl -fsSL "$repository_url" \
        | xmllint --xpath '//status/@package' - \
        | cut -d '=' -f2 \
        | tr -d '"'
    )
}

wait_for_repo_publish() {
    local proj="$1"
    local repo="$2"
    local repository_url="$BUILD_ENDPOINT/$proj/_result?repository=$repo&arch=$ARCH"

    wait_for is_repo_clean "$proj" "$repo" || return 1
    wait_for is_repo_published "$proj" "$repo"
}

wait_image_to_source_match() {
    local image="$1"       
    local source="$2"
    local start_time=$(date +%s)

    wait_for compare_image_to_source_pkg "$image" "$source"
}
