// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"path/filepath"
	"strings"
)

// PathHitSpelling is how a shell writes back a command it found by searching
// PATH: the entry it was found in and the name, put together one of three
// ways. See Semantics.PathHitSpelled for the measurements.
//
// What runs is the absolute path in every reading; this is only the report —
// `command -v`, `type`, `whence`, `hash` — and what the command hash keeps,
// which is why a report made after the command has run is the same as one
// made before.
type PathHitSpelling uint8

const (
	// PathHitJoinedOnce is the entry and the name with exactly one separator
	// between them, and an empty entry written as `.`: `PATH=/bin/` gives
	// `/bin/ls`, `PATH=` and `PATH=.` give `./ls`. Nothing else is tidied, so
	// `/usr/../bin` stays as it is.
	PathHitJoinedOnce PathHitSpelling = iota

	// PathHitAsWritten is the entry, a slash and the name, with nothing
	// tidied at all — `/bin//ls`, `.//ls` — and an empty entry as the name
	// alone.
	PathHitAsWritten

	// PathHitFromTheWorkingDirectory is PathHitJoinedOnce with a relative
	// entry put under the working directory: `PATH=.` and an empty entry
	// give `$PWD/ls`, `PATH=./` gives `$PWD/./ls`, `PATH=sub` `$PWD/sub/ls`.
	PathHitFromTheWorkingDirectory
)

// spelledPathHit is a PATH hit written back the way this dialect writes one,
// from the entry exactly as PATH holds it.
func (r *Runner) spelledPathHit(dir, name string) string {
	switch r.sem().PathHitSpelled {
	case PathHitAsWritten:
		return writtenPathHit(dir, name)
	case PathHitFromTheWorkingDirectory:
		switch {
		case dir == "" || dir == ".":
			dir = r.Dir
		case !filepath.IsAbs(dir) && r.Dir != "":
			dir = r.Dir + "/" + dir
		}
		if strings.HasSuffix(dir, "/") {
			return dir + name
		}
		return dir + "/" + name
	}
	// A **prefix** join and not filepath.Join, which is the distinction
	// Runner.reportedPath draws and for the same reason: Join cleans, so a
	// `.` entry would come back as the bare name and name nothing. One
	// trailing slash is dropped and exactly one is written — measured
	// 2026-09-22 on bash 5.3.20 over a scratch tree, `PATH=.` and `PATH=`
	// both answer `./e`, `PATH=sub` and `PATH=sub/` both answer `sub/e`, and
	// `PATH=./sub` keeps its dot.
	if dir == "" {
		dir = "."
	}
	return strings.TrimSuffix(dir, "/") + "/" + name
}
