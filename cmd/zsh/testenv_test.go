// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"testing"

	"github.com/blairham/sh/driver"

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
	os.Exit(testenv.Run("cmd/zsh", m))
}

// scratchShell is this binary's own shell value with the machine's system-wide
// startup directory replaced by an empty one belonging to this test.
//
// The shipped binary reads `/etc`, which is the whole of #1717 and is pinned
// by TestTheBinaryNamesTheMachinesStartupDirectory below. A *test* that read it
// would be measuring the runner it happened to be on — `/etc/profile` sets
// `$PATH` and sources `/etc/profile.d` on a Linux runner and runs `path_helper`
// on macOS — which is the same failure internal/testenv exists to prevent, and
// the one a scratch `HOME` cannot reach because no environment variable stands
// between a shell and `/etc`.
func scratchShell(t *testing.T) driver.Shell {
	t.Helper()
	sh := shell()
	sh.SystemStartupDirectory = t.TempDir()
	return sh
}

// TestTheBinaryNamesTheMachinesStartupDirectory keeps the line above honest.
// Every test here runs with the directory moved, so nothing else would notice
// if the shipped value went missing.
func TestTheBinaryNamesTheMachinesStartupDirectory(t *testing.T) {
	if got := shell().SystemStartupDirectory; got != "/etc" {
		t.Errorf("SystemStartupDirectory = %q, want /etc", got)
	}
}
