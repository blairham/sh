// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"os"
	"testing"

	"github.com/blairham/sh/internal/childguard"
	"github.com/blairham/sh/internal/testenv"
	"github.com/blairham/sh/internal/treeguard"
)

// This file is the suite's environment, and the reason it exists is that the
// front end starts *shells* — a shell reads a person's startup files, writes a
// person's history file and appends to a person's block store, and it does all
// three by reading the environment it was handed. A test that hands it the
// developer's environment has aimed those writes at the developer's home.
//
// Measured before the guard was written, `go test ./driver/` appended 12,394
// records to `~/.local/state/sh/blocks/index.jsonl` — 3.0 MB, each record
// naming the command and the working directory it ran in — and wrote five
// lines into whatever `$HISTFILE` pointed at, which for anyone whose shell
// exports it is their real history (#890).
//
// The guard itself is internal/testenv, which is where the argument for its
// shape lives. It was two near-copies — this file and repl's — until a third
// package needed it and found #1984 by failing on a developer's machine
// instead.
func TestMain(m *testing.M) {
	// A copy of this binary re-executed as a shell, for the background-job
	// tests. Here rather than at the top of the test, which is where the
	// signal tests put theirs: those shells are killed and never return, so
	// they never reach an os.Exit inside a test — and Go 1.26 panics on one
	// that does. Measured the hard way: two of the four cases passed anyway,
	// because their evidence was a child process that survived the panic.
	bgRunningAsAShell()
	if testenv.Assembled() {
		// A shell half re-executed by a test above. Its environment was
		// assembled by the parent and then aimed by the test that started it,
		// so touching it here would overwrite the question being asked.
		os.Exit(m.Run())
	}
	// And a temporary directory of its own, guarded. This package starts
	// shells as programs, which is where the leak in #1284 lived: a process
	// substitution's directory was removed by nothing, so every invocation
	// left one in /tmp for good. interp's own tests could not have caught
	// it — each hands its Runner a TMPDIR the framework takes away again —
	// and treeguard.Run could not either, since what is left is an *empty*
	// directory and Run counts files. treeguard.Temp is that case.
	os.Exit(testenv.Run("driver", treeguard.Temp(childguard.Wrap(m, childguard.PipeMarker))))
}

// scratchHome gives one test a home of its own, and a history file under it.
func scratchHome(t *testing.T) string {
	t.Helper()
	return testenv.Scratch(t)
}

// TestTheSuiteRunsInAHomeOfItsOwn asserts the assembly above actually
// happened, from inside the run it assembled.
//
// internal/testenv makes the same assertion about itself; this one is here
// because the claim worth pinning in *this* package is that its TestMain still
// reaches the guard — a `bgRunningAsAShell` that returned early, or a
// re-ordering of the two branches above, would leave the suite running against
// a person's home with every other test still green.
func TestTheSuiteRunsInAHomeOfItsOwn(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" || home != os.Getenv(testenv.TripwireHome) {
		t.Fatalf("HOME = %q, want the assembled scratch home %q", home, os.Getenv(testenv.TripwireHome))
	}
	if left := testenv.EntriesUnder(home); len(left) > 0 {
		t.Errorf("the suite's home already holds %v; it is meant to stay empty", left)
	}
	// Set and empty, which is not the same as absent: absent means "use the
	// default", and the default is a file under the home.
	if v, ok := os.LookupEnv("HISTFILE"); !ok || v != "" {
		t.Errorf("HISTFILE = %q, %v; want it set and empty, which is recording off", v, ok)
	}
	// Everything the startup surface reads and this suite is not about. Each
	// one aims a read or a write at a real directory when it survives:
	// ZDOTDIR and $ENV name startup files, BASH_ENV names the one a
	// non-interactive shell reads, XDG_STATE_HOME and SH_BLOCKS_DIR name the
	// block store, FPATH names the directories an autoload searches, and
	// SHELLOPTS turns options on before a line has run.
	for _, name := range []string{
		"ZDOTDIR", "ENV", "BASH_ENV", "SHELLOPTS", "FPATH",
		"XDG_STATE_HOME", "SH_BLOCKS_DIR", "SH_BLOCKS_OUTPUT",
	} {
		if v, ok := os.LookupEnv(name); ok {
			t.Errorf("%s = %q survived the scrub", name, v)
		}
	}
}
