package fips

import (
	"context"

	"github.com/suse/elemental/v3/pkg/chroot"
	"github.com/suse/elemental/v3/pkg/sys"
)

func Enable(ctx context.Context, s *sys.System) error {
	stdOut, err := s.Runner().RunContext(ctx, "/usr/bin/fips-mode-setup", "--enable", "--no-bootcfg")
	s.Logger().Debug("fips-mode-setup: %s", string(stdOut))
	return err
}

func ChrootedEnable(ctx context.Context, s *sys.System, rootDir string) error {
	callback := func() error { return Enable(ctx, s) }
	return chroot.ChrootedCallback(s, rootDir, nil, callback)
}
