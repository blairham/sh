// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package interp

import (
	"io/fs"
	"time"
)

// fileChangeTime and fileBlocks have no answer where the platform's stat
// cannot be decomposed, and what they then sort by is the name — the same
// bargain fileAccessTime strikes for `test -N`.
func fileChangeTime(fs.FileInfo) (time.Time, bool) { return time.Time{}, false }

func fileBlocks(fs.FileInfo) (int64, bool) { return 0, false }
