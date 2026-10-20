// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// preset is this package's vectors in one value, and the route the tests here
// take to an interp.Runner. One route rather than one per helper: a hand-built
// Runner that forgets Runner.Dialect asserts the core's answers under this
// dialect's name, which is #860 and is what dialecttest.Preset cannot do.
var preset = dialecttest.Preset{
	Name:        "ash",
	Dialect:     ash.Dialect,
	Semantics:   ash.Semantics,
	Diagnostics: ash.Diagnostics,
	Apply:       ash.Apply,
	Prelude:     ash.Prelude,
}

// presetDialect is a fresh, addressable copy of this dialect's grammar, for a
// Runner literal that needs a field the shared builder does not carry. Fresh
// per call: a runner owns what Dialect points at, so a shared value would let
// one test's change reach the next.
func presetDialect() *syntax.Dialect {
	d := ash.Dialect()
	return &d
}

var _ = presetDialect
