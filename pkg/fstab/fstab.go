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

package fstab

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"text/tabwriter"

	"strings"

	"github.com/suse/elemental/v3/pkg/sys"
	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

const File = "/etc/fstab"

type Line struct {
	Device     string
	MountPoint string
	FileSystem string
	Options    []string
	DumpFreq   int
	FsckOrder  int
}

// WriteFstab writes an fstab file at the given location including the the given fstab lines
func WriteFstab(s *sys.System, fstabFile string, fstabLines []Line) (err error) {
	fstab, err := s.FS().Create(fstabFile)
	if err != nil {
		s.Logger().Error("could not create fstab file %s", fstabFile)
		return err
	}
	defer func() {
		e := fstab.Close()
		if err == nil && e != nil {
			err = e
		}
	}()

	err = writeFstabLines(fstab, fstabLines)
	if err != nil {
		s.Logger().Error("failed writing fstab lines")
		return err
	}

	return nil
}

// UpdateFstab updates the given fstab file by replacing each oldLine with its newLine.
func UpdateFstab(s *sys.System, fstabFile string, oldLines, newLines []Line) (err error) {
	if len(oldLines) != len(newLines) {
		return fmt.Errorf("length of new and old lines must match")
	}

	fstab, err := s.FS().OpenFile(fstabFile, os.O_RDWR, vfs.FilePerm)
	if err != nil {
		s.Logger().Error("could not open fstab file %s", fstabFile)
		return err
	}
	defer func() {
		e := fstab.Close()
		if err == nil && e != nil {
			err = e
		}
	}()

	scanner := bufio.NewScanner(fstab)

	var fstabLines []Line
	for scanner.Scan() {
		line := scanner.Text()
		fstabLine, err := fstabLineFromFields(strings.Fields(line))
		if err != nil {
			s.Logger().Error("invalid fstab line '%s'", line)
			return err
		}
		fstabLines = append(fstabLines, fstabLine)
	}

	fstabLines = updateFstabLines(fstabLines, oldLines, newLines)

	_, err = fstab.Seek(0, 0)
	if err != nil {
		s.Logger().Error("failed to file fstab file cursor")
		return err
	}

	err = fstab.Truncate(0)
	if err != nil {
		s.Logger().Error("failed truncating fstab file '%s'", fstabFile)
		return err
	}

	err = writeFstabLines(fstab, fstabLines)
	if err != nil {
		s.Logger().Error("failed writing fstab lines")
		return err
	}

	return nil
}

func updateFstabLines(lines []Line, oldLines, newLines []Line) []Line {
	var fstabLines []Line
	for _, line := range lines {
		if i := matchFstabLine(line, oldLines); i >= 0 {
			fstabLines = append(fstabLines, newLines[i])
		} else {
			fstabLines = append(fstabLines, line)
		}
	}
	return fstabLines
}

func writeFstabLines(w io.Writer, fstabLines []Line) (err error) {
	tw := tabwriter.NewWriter(w, 1, 4, 1, ' ', 0)
	for _, fLine := range fstabLines {
		_, err = fmt.Fprintf(
			tw, "%s\t%s\t%s\t%s\t%d\t%d\n",
			fLine.Device, fLine.MountPoint, fLine.FileSystem,
			strings.Join(fLine.Options, ","), fLine.DumpFreq, fLine.FsckOrder,
		)
		if err != nil {
			return err
		}
	}
	err = tw.Flush()
	if err != nil {
		return err
	}

	return nil
}

// matchFstabLine compares device, mountpoint, filesystem and options of the given fstab line, with the
// lines in the match list. If parameters match returns the index of the list or -1 if no match. Any empty
// field in match lines matches any value.
func matchFstabLine(line Line, matchLines []Line) int {
	for i, mLine := range matchLines {
		if mLine.Device != "" && mLine.Device != line.Device {
			continue
		}
		if mLine.MountPoint != "" && mLine.MountPoint != line.MountPoint {
			continue
		}
		if mLine.FileSystem != "" && mLine.FileSystem != line.FileSystem {
			continue
		}
		if len(mLine.Options) > 0 && !slices.Equal(mLine.Options, line.Options) {
			continue
		}
		return i
	}
	return -1
}

func fstabLineFromFields(fields []string) (Line, error) {
	var fstabLine Line
	if len(fields) != 6 {
		return fstabLine, fmt.Errorf("invalid number of fields for fstab line")
	}
	dumpFreq, err := strconv.Atoi(fields[4])
	if err != nil {
		return fstabLine, fmt.Errorf("invalid dump frequency value in fstab line '%s'", fields[4])
	}
	fsckOrder, err := strconv.Atoi(fields[5])
	if err != nil {
		return fstabLine, fmt.Errorf("invalid filesystem check order value in fstab line '%s'", fields[5])
	}
	fstabLine.Device = fields[0]
	fstabLine.MountPoint = fields[1]
	fstabLine.FileSystem = fields[2]
	fstabLine.Options = strings.Split(fields[3], ",")
	fstabLine.DumpFreq = dumpFreq
	fstabLine.FsckOrder = fsckOrder

	return fstabLine, nil
}
