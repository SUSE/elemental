#!/bin/bash

set -uo pipefail

KUBE_SYSTEM_NS="kube-system"

# Resources that failed to deploy. Deployment continues past failures, so that
# resources not depending on a failed one are still deployed, and fails at the end.
FAILED_RESOURCES=()

kubectl_cmd() {
  KUBECONFIG=/etc/rancher/rke2/rke2.yaml /var/lib/rancher/rke2/bin/kubectl "$@"
}

retryKubectlCreate() {
  local resource="$1"
  local retries="$2"
  local sleep="$3"

  for i in $(seq 1 "$retries"); do
    output=$(kubectl_cmd create -f "$resource" 2>&1)
    status=$?

    if [ $status -eq 0 ]; then
      echo "$output"
      return 0
    fi

    local failed=false
    while IFS= read -r line; do
      if [[ -n "$line" && "$line" != *"AlreadyExists"* ]]; then
        failed=true
        break
      fi
    done <<< "$output"

    if [ "$failed" = false ]; then
      echo "$output"
      return 0 # Only "AlreadyExists" errors found, consider it success
    fi

    echo "Attempt $i/$retries: Creation for resource '$resource' failed with error '$output'. Retrying in $sleep seconds.."
    sleep "$sleep"
  done

  echo "ERROR: Resource '$resource' failed to deploy after $retries attempts."
  return 1
}

jobConditions() {
  kubectl_cmd get job "$1" -n "$2" -o jsonpath='{range .status.conditions[?(@.status=="True")]}{.type}{"\n"}{end}'
}

waitForHelmChart() {
  local name="$1"
  local namespace="${2:-$KUBE_SYSTEM_NS}"

  chart_job=""
  for i in {1..10}; do
    chart_job=$(kubectl_cmd get helmcharts "$name" -n "$namespace" -o go-template='{{"{{.status.jobName}}"}}')
    if [ "$chart_job" != "<no value>" ] && [ -n "$chart_job" ]; then
      break
    fi

    echo "[$i/10] '.status.jobName' is not yet present in $name HelmChart. Retrying.."
    sleep 3
  done

  if [ -z "$chart_job" ]; then
    echo "Could not get Job for $name HelmChart"
    return 1
  fi

  echo "Waiting for $chart_job HelmChart Job.."
  local conditions errors=0
  while true; do
    if conditions=$(jobConditions "$chart_job" "$namespace" 2>&1); then
      errors=0
      grep -qx Complete <<< "$conditions" && return 0
      if grep -qx Failed <<< "$conditions"; then
        echo "Job $chart_job failed, Helm Controller gave up installing $name HelmChart"
        return 1
      fi
    elif (( ++errors >= 10 )); then
      echo "Could not get Job $chart_job of $name HelmChart: $conditions"
      return 1
    fi
    sleep 10
  done
}

{{- if .HelmCharts }}
deployHelmCharts() {
  local helmCharts=(
{{- range .HelmCharts }}
"{{ . }}"
{{- end }}
)
  echo "Deploying HelmCharts.."
  for chart in "${helmCharts[@]}"; do
    chart_name=$(kubectl_cmd create --dry-run=client -f "$chart" -o jsonpath='{.metadata.name}')
    if ! retryKubectlCreate "$chart" 6 10; then
      FAILED_RESOURCES+=("HelmChart $chart")
      continue
    fi

    echo "Waiting for $chart_name HelmChart to be available.."
    if ! waitForHelmChart "$chart_name" "$KUBE_SYSTEM_NS"; then
      FAILED_RESOURCES+=("HelmChart $chart")
    fi
  done
}
{{- end }}

{{- if .ManifestsDir }}
deployManifests() {
  echo "Deploying Kubernetes manifests.."
  for manifest in {{ .ManifestsDir }}/*.yaml; do
    [[ -e "$manifest" ]] || continue
    [[ "$manifest" == *-priority.yaml ]] && continue
    if ! retryKubectlCreate "$manifest" 6 10; then
      FAILED_RESOURCES+=("manifest $manifest")
    fi
  done
}
{{- end }}

{{- if .ManifestsDir }}
deployPriorityManifests() {
  echo "Deploying Kubernetes priority manifests.."
  for manifest in {{ .ManifestsDir }}/*-priority.yaml; do
    [[ -e "$manifest" ]] || continue
    if ! retryKubectlCreate "$manifest" 6 10; then
      FAILED_RESOURCES+=("manifest $manifest")
    fi
  done
}
{{- end }}

waitForCoreRKE2Charts() {
  # A running rke2-server.service does not mean that the Helm Controller is ready.
  # Wait for the Helm Controller to start creating the core RKE2 HelmChart resources.
  # Nothing can be deployed without it, so give up on the whole deployment if it never does.
  local deadline=$((SECONDS + 900))
  until [[ $(kubectl_cmd get helmcharts -n "$KUBE_SYSTEM_NS" --no-headers 2>/dev/null | wc -l) -gt 0 ]]; do
    if (( SECONDS >= deadline )); then
      echo "ERROR: The Helm Controller did not create any HelmChart resources in time."
      exit 1
    fi
    sleep 10
  done

  local rke2_manifests_dir="{{ .RKE2ManifestsDir }}"
  local rke2_chart_names=""
  for rke2_file in $rke2_manifests_dir/*.yaml; do
    # Make sure file is a valid K8s resource
    if kubectl_cmd create --dry-run=client -f "$rke2_file" > /dev/null 2>&1; then
      kind=$(kubectl_cmd create --dry-run=client -f "$rke2_file" -o jsonpath="{.kind}" 2>&1)
      name=$(kubectl_cmd create --dry-run=client -f "$rke2_file" -o jsonpath="{.metadata.name}" 2>&1)
      if [ "$kind" = "HelmChart" ]; then
          rke2_chart_names="$rke2_chart_names $name"
      fi
    fi
  done

  echo "Waiting for RKE2 core helm charts"
  for name in $rke2_chart_names; do
    if ! waitForHelmChart "$name" "$KUBE_SYSTEM_NS"; then
      FAILED_RESOURCES+=("RKE2 HelmChart $name")
    fi
  done
}

waitForCoreRKE2Charts

{{- if .ManifestsDir }}
deployPriorityManifests
{{- end }}

{{- if .HelmCharts }}
deployHelmCharts
{{- end }}

{{- if .ManifestsDir }}
deployManifests
{{- end }}

if [ ${#FAILED_RESOURCES[@]} -gt 0 ]; then
  echo "ERROR: The following resources failed to deploy:"
  printf '  - %s\n' "${FAILED_RESOURCES[@]}"
  exit 1
fi
