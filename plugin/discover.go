// Copyright IBM Corp. 2016, 2025
// SPDX-License-Identifier: MPL-2.0

package core

import (
	"path/filepath"

	"github.com/hashicorp/go-plugin/internal/pathx"
)

// Discover discovers plugins that are in a given directory.
//
// The directory doesn't need to be absolute. For example, "." will work fine.
//
// This currently assumes any file matching the glob is a core.
// In the future this may be smarter about checking that a file is
// executable and so on.
//
// TODO: test
func Discover(glob, dir string) ([]string, error) {
	var err error

	// Make the directory absolute if it isn't already
	if !filepath.IsAbs(dir) {
		dir, err = pathx.Abs(dir)
		if err != nil {
			return nil, err
		}
	}

	return pathx.Glob(pathx.Join(dir, glob))
}
