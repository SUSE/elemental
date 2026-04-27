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

package containerd_test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/suse/elemental/v3/pkg/containerd"
	"github.com/suse/elemental/v3/pkg/log"
	"github.com/suse/elemental/v3/pkg/sys"
	sysmock "github.com/suse/elemental/v3/pkg/sys/mock"
	"github.com/suse/elemental/v3/pkg/sys/vfs"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/cmd/containerd/server"
	srvconfig "github.com/containerd/containerd/v2/cmd/containerd/server/config"
	"github.com/containerd/containerd/v2/defaults"
	ctrdsys "github.com/containerd/containerd/v2/pkg/sys"
	"github.com/containerd/containerd/v2/version"

	_ "github.com/containerd/containerd/v2/plugins/content/local/plugin"
	_ "github.com/containerd/containerd/v2/plugins/diff/walking/plugin"
	_ "github.com/containerd/containerd/v2/plugins/events"
	_ "github.com/containerd/containerd/v2/plugins/gc"
	_ "github.com/containerd/containerd/v2/plugins/leases"
	_ "github.com/containerd/containerd/v2/plugins/metadata"
	_ "github.com/containerd/containerd/v2/plugins/services/content"
	_ "github.com/containerd/containerd/v2/plugins/services/diff"
	_ "github.com/containerd/containerd/v2/plugins/services/events"
	_ "github.com/containerd/containerd/v2/plugins/services/images"
	_ "github.com/containerd/containerd/v2/plugins/services/introspection"
	_ "github.com/containerd/containerd/v2/plugins/services/leases"
	_ "github.com/containerd/containerd/v2/plugins/services/namespaces"
	_ "github.com/containerd/containerd/v2/plugins/services/snapshots"
	_ "github.com/containerd/containerd/v2/plugins/snapshots/native/plugin"
	_ "github.com/containerd/containerd/v2/plugins/snapshots/overlay/plugin"
)

func TestContainerdSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Containerd test suite")
}

// PullImg pulls and unpacks the given image reference from the local containerd daemon
func PullImg(ctx context.Context, sock, imgRef string) error {
	cli, err := client.New(sock)
	if err != nil {
		return err
	}
	_, err = cli.Pull(ctx, imgRef, client.WithPullUnpack)
	return err
}

// StartEmbeddedDaemon launches a storage-only containerd server in a goroutine.
func StartEmbeddedDaemon(ctx context.Context, s *sys.System, rootDir, stateDir, socketPath string) error {
	cfg := &srvconfig.Config{
		Version: version.ConfigVersion,
		Root:    rootDir,
		State:   stateDir,
		Debug: srvconfig.Debug{
			Level: "debug",
		},
		GRPC: srvconfig.GRPCConfig{
			Address:        socketPath,
			MaxRecvMsgSize: defaults.DefaultMaxRecvMsgSize,
			MaxSendMsgSize: defaults.DefaultMaxSendMsgSize,
		},
		// Explicitly disable everything related to running containers
		DisabledPlugins: []string{
			"io.containerd.grpc.v1.cri",
			"io.containerd.runtime.v1.linux",
			"io.containerd.runtime.v2.task",
			"io.containerd.monitor.v1.cgroups",
			"io.containerd.internal.v1.restart",
		},
		RequiredPlugins: []string{
			"io.containerd.grpc.v1.leases",
			"io.containerd.grpc.v1.snapshots",
			"io.containerd.grpc.v1.content",
			"io.containerd.content.v1.content",
		},
	}

	srv, err := server.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize embedded server: %w", err)
	}

	l, err := ctrdsys.GetLocalListener(cfg.GRPC.Address, cfg.GRPC.UID, cfg.GRPC.GID)
	if err != nil {
		return fmt.Errorf("failed to create socket listener: %w", err)
	}

	go func() {
		// blocks until the context is cancelled
		if err := srv.ServeGRPC(l); err != nil {
			s.Logger().Warn("Embedded containerd stopped: %v", err)
		}
	}()

	return nil
}

const (
	alpineImageRef = "docker.io/library/alpine:3.21.3"
	missingImgRef  = "invalid.registry.org/some/image:latest"
	imageID        = "aded1e1a5b37"
	ctrdSock       = "/run/containerd/containerd.sock"
	ctrdRoot       = "/run/containerd/root"
	ctrdState      = "/run/containerd/state"
	ctrdNamespace  = "test"
)

var _ = Describe("Containerd", Label("containerd", "rootlesskit"), func() {
	var s *sys.System
	var tfs vfs.FS
	var cleanup func()
	var err error
	var buffer *bytes.Buffer
	var ctx context.Context
	var cancel context.CancelFunc
	var ctrdWr *containerd.Wrapper

	BeforeEach(func() {
		buffer = &bytes.Buffer{}
		tfs, cleanup, err = sysmock.TestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		s, err = sys.NewSystem(
			sys.WithFS(tfs),
			sys.WithLogger(log.New(log.WithBuffer(buffer))),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(vfs.MkdirAll(tfs, ctrdRoot, vfs.DirPerm)).To(Succeed())
		Expect(vfs.MkdirAll(tfs, ctrdState, vfs.DirPerm)).To(Succeed())
		ctx = context.Background()
		ctx = containerd.AddNamespace(ctx, ctrdNamespace)
		ctx, cancel = context.WithCancel(ctx)

		root, err := tfs.RawPath(ctrdRoot)
		Expect(err).NotTo(HaveOccurred())

		state, err := tfs.RawPath(ctrdState)
		Expect(err).NotTo(HaveOccurred())

		sock, err := tfs.RawPath(ctrdSock)
		Expect(err).NotTo(HaveOccurred())

		Expect(StartEmbeddedDaemon(ctx, s, root, state, sock)).To(Succeed())

		ctrdWr, err = containerd.NewWrapper(s, sock)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cancel()
		cleanup()
	})

	It("finds an unpacked image from contianerd daemon", func() {
		sock, err := tfs.RawPath(ctrdSock)
		Expect(err).NotTo(HaveOccurred())

		Expect(PullImg(ctx, sock, alpineImageRef)).To(Succeed())

		callback := func(rootfs string) error {
			return vfs.CopyFile(tfs, filepath.Join(rootfs, "/etc/os-release"), "/run/containerd")
		}
		img, err := ctrdWr.FindUnpackedImage(ctx, alpineImageRef)
		Expect(err).NotTo(HaveOccurred())
		Expect(img.Digest).To(ContainSubstring(imageID))
		Expect(ctrdWr.RunOnMountedROSnapshot(ctx, img, callback)).To(Succeed())
		ok, _ := vfs.Exists(tfs, "/run/containerd/os-release")
		Expect(ok).To(BeTrue())

		callback = func(rootfs string) error {
			return fmt.Errorf("callback error")
		}
		Expect(ctrdWr.RunOnMountedROSnapshot(ctx, img, callback)).To(MatchError(ContainSubstring("callback error")))
	})

	It("doesn't find a non existing image", func() {
		_, err := ctrdWr.FindUnpackedImage(ctx, missingImgRef)
		Expect(err).To(MatchError(ContainSubstring("getting image from containerd")))
	})
})
