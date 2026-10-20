// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// preset is tests/zsh's, for the one external test that stays here:
// compdump_test.go, which needs the hooks compdumphooks_test.go exports from
// inside the package, and only a test in this directory can see those.
var preset = dialecttest.Preset{
	Name:        "zsh",
	Dialect:     zsh.Dialect,
	Semantics:   zsh.Semantics,
	Diagnostics: zsh.Diagnostics,
	Apply:       zsh.Apply,
	Prelude:     zsh.Prelude,
}
