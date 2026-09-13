// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package interp

import (
	"io/fs"
	"time"
)

// fileAccessTime has no answer where the platform's stat cannot be
// decomposed, and a `test -N` that could not tell says false — the same
// bargain `-O` and `-G` strike for an identity they cannot read.
func fileAccessTime(fs.FileInfo) (time.Time, bool) { return time.Time{}, false }
