// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

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
	if testenv.Assembled() {
		// A copy of this binary re-executed as a helper by a test above. Its
		// environment was assembled by the parent and then aimed by the test
		// that started it, so scrubbing it here would erase the question being
		// asked.
		os.Exit(m.Run())
	}
	os.Exit(testenv.Run("dialect/dash", m))
}
