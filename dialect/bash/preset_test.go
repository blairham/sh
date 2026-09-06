// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// preset is this package's four vectors in one value, and the route the tests
// here take to an interp.Runner.
//
// One route rather than four. The helpers this replaced — answersRun,
// refuseInScript, runBash — were the same six lines in each of the four
// dialect packages, and each copy had to remember Runner.Dialect on its own.
// Three of them did not, which is #860; the copy in answers_test.go did not
// either until #849, and #826 found that one only because a pattern happened
// to reach it. dialecttest.Preset.Runner cannot forget the field, and the
// guard in that package fails any Runner literal under dialect/ that does.
var preset = dialecttest.Preset{
	Name:        "bash",
	Dialect:     bash.Dialect,
	Semantics:   bash.Semantics,
	Diagnostics: bash.Diagnostics,
	Apply:       bash.Apply,
}

// presetDialect is a fresh, addressable copy of this dialect's grammar, for
// the Runner literals that are still assembled by hand because the case needs
// a field the shared builder does not carry.
//
// Fresh per call rather than one package-level value: Runner.Dialect is a
// pointer and a runner owns what it points at — cloning a runner for a
// subshell writes a modified copy back through it (interp/matchoption.go), so
// a shared one would let a `setopt` in one test reach the next.
func presetDialect() *syntax.Dialect {
	dl := bash.Dialect()
	return &dl
}
