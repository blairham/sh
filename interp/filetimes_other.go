// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package interp

import (
	"io/fs"
	"time"
)

// A platform whose stat fields were not looked up, where the two qualifiers
// answer for nothing rather than answering wrongly: a test that cannot be
// taken keeps no file, which is what `false` says here.
func fileTime(fs.FileInfo, byte) (time.Time, bool) { return time.Time{}, false }
