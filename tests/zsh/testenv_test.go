// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"testing"

	"github.com/blairham/sh/internal/testenv"
)

// This suite starts shells, so it runs in a home of its own. A shell reads
// startup files and writes history from the environment it was handed, and a
// suite that hands it the developer's environment is measuring the developer's
// dotfiles — green on a runner whose home is empty, red on the machine the
// work is done on (#1984, #1987). internal/testenv is the guard; its own
// package comment is the argument for its shape.
func TestMain(m *testing.M) {
	// And the module helper the same way, which is re-entered by wrapper
	// scripts a shell under test runs with its own small environment — one
	// with no SH_TEST_HOME in it, so Assembled cannot see it. Run made it a
	// fresh home under the system temporary directory, and the helper ends in
	// os.Exit, so Run never removed it: a dozen empty directories in /tmp per
	// package run (#5454). The helper starts no shell and reads no home, so
	// there is nothing for a home of its own to guard.
	if testenv.Assembled() || os.Getenv(helperMode) != "" {
		// A copy of this binary re-executed as a helper by a test above. Its
		// environment was assembled by the parent and then aimed by the test
		// that started it, so scrubbing it here would erase the question being
		// asked.
		os.Exit(m.Run())
	}
	os.Exit(testenv.Run("dialect/zsh", m))
}
