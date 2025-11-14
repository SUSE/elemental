#!/bin/bash

set -uo pipefail

KUBE_SYSTEM_NS="kube-system"

kubectl_cmd() {
  KUBECONFIG=/etc/rancher/rke2/rke2.yaml /var/lib/rancher/rke2/bin/kubectl "$@"
}

retryKubectlCreate() {
  local resource="$1"
  local retries="$2"
  local sleep="$3"
  local failed=true
  for i in $(seq 1 "$retries"); do
    output=$(kubectl_cmd create -f "$resource" 2>&1)
    if [[ $? -eq 0 || "$output" == *AlreadyExists* ]]; then
      echo "$output"
      failed=false
      break
    fi
    echo "Creation for resource '$resource' failed with error '$output'. Retrying in $sleep seconds.."
    sleep $sleep
  done

  if [ "$failed" = true ]; then
    echo "Creation for resource '$resource' failed.."
    return 1
  fi

  return 0
}

waitForHelmChart() {
  local name="$1"
  local namespace="${2:-$KUBE_SYSTEM_NS}"
  local wait="${3:-900s}"

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
  if ! kubectl_cmd wait --for=condition=complete --timeout="$wait" job/"$chart_job" -n "$namespace"; then
    echo "Job $chart_job failed to complete on time"
    return 1
  fi

  return 0
}

{{- if .HelmCharts }}
deployHelmCharts() {
  local helmCharts=(
{{- range .HelmCharts }}
"{{ . }}"
{{- end }}
)
  local failed=false
  echo "Deploying HelmCharts.."
  for chart in "${helmCharts[@]}"; do
    chart_name=$(kubectl_cmd create --dry-run=client -f "$chart" -o jsonpath='{.metadata.name}')
    retryKubectlCreate "$chart" 6 10
    if [[ $? -ne 0 ]]; then
      failed=true
    fi

    echo "Waiting for $chart_name HelmChart to be available.."
    if ! waitForHelmChart "$chart_name" "$KUBE_SYSTEM_NS"; then
        failed=true
    fi
  done

  if [ "$failed" = true ]; then
    exit 1
  fi
}
{{- end }}

{{- if .ManifestsDir }}
deployManifests() {
  local failed=false
  echo "Deploying Kubernetes manifests.."
  for manifest in {{ .ManifestsDir }}/*.yaml; do
    retryKubectlCreate "$manifest" 6 10
    if [[ $? -ne 0 ]]; then
      failed=true
    fi
  done

  if [ "$failed" = true ]; then
    exit 1
  fi
}
{{- end }}

waitForCoreRKE2Charts() {
  # A running rke2-server.service does not mean that the Helm Controller is ready.
  # Wait for the Helm Controller to start creating the core RKE2 HelmChart resources.
  until [[ $(kubectl_cmd get helmcharts -n "$KUBE_SYSTEM_NS" --no-headers 2>/dev/null | wc -l) -gt 0 ]]; do
    sleep 10
  done

  local rke2_manifests_dir="/var/lib/rancher/rke2/server/manifests"
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
      exit 1
    fi
  done
}

waitForCoreRKE2Charts

{{- if .HelmCharts }}
deployHelmCharts
{{- end }}

{{- if .ManifestsDir }}
deployManifests
{{- end }}
