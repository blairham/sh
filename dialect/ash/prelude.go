// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash

// Prelude is the part of the dialect written as shell rather than as Go.
//
// Not empty, and the one line in it is measured rather than decorative: a
// bare `set` in this shell lists `BB_ASH_VERSION='1.37.0'` first, so a script
// asking which shell it is under has an answer here where dash gives it none.
// It is the only variable this shell names itself in — there is no `$ASH_VERSION`
// and no `${.sh.version}` — and a script that tests for it is testing for this
// shell.
func Prelude() string { return "BB_ASH_VERSION=" + Version + "\n" }

// Version is what this dialect reports as the BusyBox ash it implements.
//
// The number is the build every answer in this package was measured against —
// BusyBox v1.37.0, the `alpine:3` image's — so a reader who finds a
// disagreement knows which binary to re-run. The tag says whose shell this is,
// the way the other dialects' version strings do.
const Version = "1.37.0-blairham"
