#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
YEAR=""

usage() {
  echo "Usage: $(basename "$0") --year YYYY"
  echo "  --year YYYY   Target copyright year (e.g. 2026)"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --year)
      [[ $# -ge 2 ]] || usage
      YEAR="$2"
      shift 2
      ;;
    -h|--help)
      usage
      ;;
    *)
      echo "Unknown argument: $1"
      usage
      ;;
  esac
done

if [[ -z "${YEAR}" ]]; then
  echo "Undefined '--year' flag"
  usage
  exit 1
fi

if sed --version >/dev/null 2>&1; then
  # Linux specific option
  SED_INPLACE=(-i)
else
  # Mac specific opiton
  SED_INPLACE=(-i '')
fi

git -C "$REPO_ROOT" ls-files '*.go' | while IFS= read -r f; do
  sed "${SED_INPLACE[@]}" -E \
    -e "s/^Copyright © ([0-9]{4})[[:space:]]*-[[:space:]]*[0-9]{4} SUSE LLC$/Copyright © \1-${YEAR} SUSE LLC/" \
    -e "s/^Copyright © ([0-9]{4}) SUSE LLC$/Copyright © \1-${YEAR} SUSE LLC/" \
    "$REPO_ROOT/$f"
done
