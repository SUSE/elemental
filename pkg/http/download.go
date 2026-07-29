/*
Copyright © 2025-2026 SUSE LLC
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

//revive:disable:var-naming
package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/suse/elemental/v3/pkg/sys/vfs"
)

type Downloader struct{}

func (d Downloader) File(ctx context.Context, fs vfs.FS, url, path string) error {
	r, err := d.URLBody(ctx, url)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	file, err := fs.Create(path)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}

	_, err = io.Copy(file, r)
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("copying file contents: %w", err)
	}

	if err = file.Close(); err != nil {
		return fmt.Errorf("closing file: %w", err)
	}

	return nil
}

func (Downloader) URLBody(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpClient := &http.Client{Timeout: 90 * time.Second}
	resp, err := httpClient.Do(req) // #nosec G704 -- url is assumed to be trusted.
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return resp.Body, nil
}
