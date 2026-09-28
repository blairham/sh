// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package repl

import (
	"io/fs"
	"time"
)

// accessTime is not available here, so no mailbox is ever reported about.
// See the unix file beside this one.
func accessTime(fs.FileInfo) (time.Time, bool) { return time.Time{}, false }
