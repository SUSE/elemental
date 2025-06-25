/*
Copyright © 2025 SUSE LLC
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

package vm

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck,revive
	. "github.com/onsi/gomega"    //nolint:staticcheck,revive

	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

const (
	Passive     = "passive"
	Active      = "active"
	Recovery    = "recovery"
	LiveCD      = "liveCD"
	UnknownBoot = "unknown"

	TimeoutRawDiskTest = 600 // Timeout to connect for recovery_raw_disk_test
)

type SUT struct {
	Host          string
	Username      string
	Password      string
	SSHKey        []byte
	Timeout       int
	artifactsRepo string
	TestVersion   string
	CDLocation    string
	MachineID     string
	VMPid         int
}

func NewSUT() *SUT {
	var (
		err        error
		user       string
		sshKeyFile string
		sshKey     []byte
		pass       string
		host       string
		vmPid      int
		timeout    = 180
		value      int
	)

	if user = os.Getenv("SSH_USER"); user == "" {
		user = "root"
	}

	if sshKeyFile = os.Getenv("SSH_KEY"); sshKeyFile == "" {
		sshKeyFile = "../assets/testkey"
	}

	// Not useful here to check for a reading error, skip it!
	sshKey, _ = os.ReadFile(sshKeyFile)

	if pass = os.Getenv("SSH_PASS"); pass == "" {
		pass = "uc0@linux"
	}

	if host = os.Getenv("SSH_HOST"); host == "" {
		host = "127.0.0.1:2222"
	}

	vmPidStr := os.Getenv("VM_PID")
	if value, err = strconv.Atoi(vmPidStr); err == nil {
		vmPid = value
	}

	valueStr := os.Getenv("SSH_TIMEOUT")
	if value, err = strconv.Atoi(valueStr); err == nil {
		timeout = value
	}

	return &SUT{
		Host:          host,
		Username:      user,
		Password:      pass,
		SSHKey:        sshKey,
		MachineID:     "test",
		Timeout:       timeout,
		artifactsRepo: "",
		CDLocation:    "",
		VMPid:         vmPid,
	}
}

// BootFrom returns the booting partition of the SUT
// NOTE: passive/recovery are not handled yet!
func (s *SUT) BootFrom() string {
	GinkgoHelper()

	out, err := s.command("cat /proc/cmdline")
	Expect(err).ToNot(HaveOccurred())

	switch {
	case strings.Contains(out, "LABEL=SYSTEM"):
		return Active
	case strings.Contains(out, "live:CDLABEL"):
		return LiveCD
	default:
		return UnknownBoot
	}
}

func (s *SUT) EventuallyBootedFrom(image string) {
	GinkgoHelper()

	Eventually(func() error {
		actual := s.BootFrom()
		if actual != image {
			return fmt.Errorf("expected boot from %s, actual %s", image, actual)
		}

		return nil
	}, time.Duration(60)*time.Second, time.Duration(10)*time.Second).ShouldNot(HaveOccurred())
}

func (s *SUT) GetOSRelease(ss string) string {
	GinkgoHelper()

	out, err := s.Command(fmt.Sprintf("source /etc/os-release && echo $%s", ss))
	Expect(err).ToNot(HaveOccurred())
	Expect(out).ToNot(BeEmpty())

	return strings.TrimSpace(out)
}

func (s *SUT) GetArch() string {
	GinkgoHelper()

	out, err := s.Command("uname -p")
	Expect(err).ToNot(HaveOccurred())
	Expect(out).ToNot(BeEmpty())

	return strings.TrimSpace(out)
}

func (s *SUT) EventuallyConnects(t ...int) {
	GinkgoHelper()

	dur := s.Timeout
	if len(t) > 0 {
		dur = t[0]
	}
	Eventually(func() (string, error) {
		if !s.IsVMRunning() {
			return "", StopTrying("Underlaying VM is no longer running!")
		}
		return s.command("echo -n ping")
	}, time.Duration(time.Duration(dur)*time.Second), time.Duration(5*time.Second)).Should(Equal("ping"))
}

func (s *SUT) EventuallyDisconnects(t ...int) {
	GinkgoHelper()

	dur := s.Timeout
	if len(t) > 0 {
		dur = t[0]
	}
	s.EventuallyConnects(10)
	Eventually(func() (string, error) {
		if !s.IsVMRunning() {
			return "", StopTrying("Underlaying VM is no longer running!")
		}
		out, _ := s.command("sleep 30 && echo -n ping")
		return out, nil
	}, time.Duration(time.Duration(dur)*time.Second), time.Duration(2*time.Second)).ShouldNot(Equal("ping"))
}

func (s *SUT) IsVMRunning() bool {
	if s.VMPid <= 0 {
		// Can't check without a pid, assume it is always running
		return true
	}
	proc, err := os.FindProcess(s.VMPid)
	if err != nil || proc == nil {
		return false
	}

	// On Unix FindProcess does not error out if the process does not
	// exist, so we send a test signal
	return proc.Signal(syscall.Signal(0)) == nil
}

// Command sends a command to the SUIT and waits for reply
func (s *SUT) Command(cmd string) (string, error) {
	if !s.IsVMRunning() {
		return "", fmt.Errorf("VM is not running, doesn't make sense running any command")
	}
	return s.command(cmd)
}

func (s *SUT) command(cmd string) (string, error) {
	client, err := s.connectToHost()
	if err != nil {
		return "", err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmd)
	if err != nil {
		return string(out), errors.Wrap(err, string(out))
	}

	return string(out), err
}

// Reboot reboots the system under test
func (s *SUT) Reboot(t ...int) {
	By("Reboot")
	_, _ = s.command("reboot")
	time.Sleep(10 * time.Second)
	s.EventuallyConnects(t...)
}

func (s *SUT) clientConfig() *ssh.ClientConfig {
	var signer ssh.Signer
	var err error
	auths := []ssh.AuthMethod{}

	if s.SSHKey != nil {
		signer, err = ssh.ParsePrivateKey(s.SSHKey)
		if err != nil {
			log.Fatalf("unable to parse private key: %v", err)
		}
		auths = append(auths, ssh.PublicKeys(signer))
	}

	sshConfig := &ssh.ClientConfig{
		User:            s.Username,
		Auth:            append(auths, ssh.Password(s.Password)),
		Timeout:         15 * time.Second,            // max time to establish connection
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
	}

	return sshConfig
}

func (s *SUT) connectToHost() (*ssh.Client, error) {
	sshConfig := s.clientConfig()

	client, err := ssh.Dial("tcp", s.Host, sshConfig)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// AssertBootedFrom asserts that we booted from the proper type and adds a helpful message
func (s SUT) AssertBootedFrom(b string) {
	GinkgoHelper()

	Expect(s.BootFrom()).To(Equal(b), "Should have booted from: %s", b)
}
