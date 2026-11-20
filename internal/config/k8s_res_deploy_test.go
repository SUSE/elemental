/*
Copyright © 2026 SUSE LLC
SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/internal/butane"
	"github.com/suse/elemental/v3/internal/image"
)

// fakeKubectl emulates the kubectl calls of the deployment script. Creating a resource,
// or waiting for the Job of a HelmChart, fails when the resource file name contains "failing".
const fakeKubectl = `#!/bin/bash
case "$1 $2" in
  "get helmcharts")
    if [ "$3" = "-n" ]; then echo "rke2-coredns"; else echo "helm-install-$3"; fi ;;
  "create --dry-run=client")
    if [[ "$*" == *"{.kind}"* ]]; then echo "HelmChart"; else basename "$4" .yaml; fi ;;
  "create -f")
    if [[ "$3" == *failing* ]]; then echo "error: unauthorized" >&2; exit 1; fi
    echo "$3" >> "$CREATED_LOG"; echo "$3 created" ;;
  "wait --for=condition=complete")
    [[ "$*" != *failing* ]] ;;
esac
`

var _ = Describe("Kubernetes resources deployment script", Label("k8s-res-deploy"), func() {
	var dir, binDir, manifestsDir, createdLog string

	writeFile := func(path, content string, mode os.FileMode) {
		Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
		Expect(os.WriteFile(path, []byte(content), mode)).To(Succeed())
	}

	run := func(helmCharts ...string) (string, error) {
		butaneCfg := &butane.Config{}
		Expect(appendK8sResDeployScript(butaneCfg, manifestsDir, helmCharts)).To(Succeed())
		script := findFileContentsInConfig(butaneCfg, filepath.Join("/", image.KubernetesPath(), k8sResDeployScriptName))
		Expect(script).NotTo(BeNil())

		// Point the script to the fake kubectl and the test core RKE2 manifests
		data := strings.ReplaceAll(*script, "/var/lib/rancher/rke2/bin/kubectl", filepath.Join(binDir, "kubectl"))
		data = strings.ReplaceAll(data, filepath.Join("/", image.RKE2ServerManifestsPath()), filepath.Join(dir, "rke2"))
		scriptPath := filepath.Join(dir, k8sResDeployScriptName)
		writeFile(scriptPath, data, 0o755)

		cmd := exec.Command("bash", scriptPath)
		cmd.Env = append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"), "CREATED_LOG="+createdLog)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	created := func() []string {
		data, err := os.ReadFile(createdLog)
		if os.IsNotExist(err) {
			return nil
		}
		Expect(err).NotTo(HaveOccurred())
		return strings.Fields(string(data))
	}

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		binDir = filepath.Join(dir, "bin")
		manifestsDir = filepath.Join(dir, "manifests")
		createdLog = filepath.Join(dir, "created.log")

		writeFile(filepath.Join(binDir, "kubectl"), fakeKubectl, 0o755)
		// Skip the retry delays
		writeFile(filepath.Join(binDir, "sleep"), "#!/bin/sh\n", 0o755)
		writeFile(filepath.Join(dir, "rke2", "rke2-coredns.yaml"), "kind: HelmChart\n", 0o644)
		writeFile(filepath.Join(manifestsDir, "local-path-provisioner.yaml"), "kind: Deployment\n", 0o644)
	})

	It("deploys all resources and succeeds", func() {
		chart := filepath.Join(dir, "helm", "ollama.yaml")
		writeFile(chart, "kind: HelmChart\n", 0o644)

		out, err := run(chart)
		Expect(err).NotTo(HaveOccurred(), out)
		Expect(created()).To(ConsistOf(chart, filepath.Join(manifestsDir, "local-path-provisioner.yaml")))
		Expect(out).NotTo(ContainSubstring("failed to deploy"))
	})

	It("succeeds without Helm charts and manifests to deploy", func() {
		manifestsDir = ""

		out, err := run()
		Expect(err).NotTo(HaveOccurred(), out)
		Expect(created()).To(BeEmpty())
	})

	It("deploys the resources independent of a failed one and fails at the end", func() {
		failing := filepath.Join(dir, "helm", "failing-gpu-operator.yaml")
		chart := filepath.Join(dir, "helm", "ollama.yaml")
		writeFile(failing, "kind: HelmChart\n", 0o644)
		writeFile(chart, "kind: HelmChart\n", 0o644)
		failingManifest := filepath.Join(manifestsDir, "failing-auth-priority.yaml")
		writeFile(failingManifest, "kind: Secret\n", 0o644)

		out, err := run(failing, chart)
		Expect(err).To(HaveOccurred(), out)
		Expect(created()).To(ConsistOf(chart, filepath.Join(manifestsDir, "local-path-provisioner.yaml")))
		Expect(out).To(ContainSubstring("ERROR: The following resources failed to deploy:\n" +
			"  - manifest " + failingManifest + "\n" +
			"  - HelmChart " + failing + "\n"))
	})
})

var _ = Describe("Kubernetes resources installer unit", Label("k8s-res-deploy"), func() {
	It("does not limit the deployment time as a whole, only the wait for RKE2", func() {
		unit, err := generateK8sResourcesUnit("/var/lib/elemental/kubernetes/k8s_res_deploy.sh", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(unit).To(ContainSubstring("TimeoutStartSec=infinity\n"))
		Expect(unit).NotTo(ContainSubstring("TimeoutSec="))
		Expect(unit).To(ContainSubstring("ExecStartPre=/usr/bin/timeout 900 /bin/sh -c 'until"))
	})
})
