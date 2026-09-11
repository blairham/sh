// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"testing"

	"github.com/blairham/sh/internal/childguard"
	"github.com/blairham/sh/internal/testenv"
	"github.com/blairham/sh/internal/treeguard"
)

// This file is the suite's environment, and it is here for the reason #890 put
// one in driver: this package *is* the thing that writes a person's history.
// It opens HISTFILE, it appends to the block store under $HOME/.local/state,
// and a session it starts does both without being asked. driver's suite spent
// a day writing 3.0 MB of records into whoever ran it, with a green board the
// whole time, and the only thing standing between this package and the same
// outcome is that every test here happens to build its Runner with an explicit
// Env.
//
// "Happens to" is the problem. Nothing under repl/ reads the process
// environment — a Runner holds its own — so without this the isolation is a
// property of newTestRunner rather than of the suite. One test that hands a
// session the process's environment, or one seam that learns to read a
// variable, and the property is gone with nothing to say so. The tripwire in
// internal/testenv is what says so.
func TestMain(m *testing.M) {
	// And a temporary directory of its own, guarded. This package starts
	// shells as programs, which is where the leak in #1284 lived: a process
	// substitution's directory was removed by nothing, so every invocation
	// left one in /tmp for good. interp's own tests could not have caught
	// it — each hands its Runner a TMPDIR the framework takes away again —
	// and treeguard.Run could not either, since what is left is an *empty*
	// directory and Run counts files. treeguard.Temp is that case.
	os.Exit(testenv.Run("repl", treeguard.Temp(childguard.Wrap(m, childguard.PipeMarker))))
}

// TestTheSuiteRunsInAHomeOfItsOwn asserts the assembly happened, from inside
// the run it assembled.
//
// internal/testenv makes the same assertion about itself. This one is the
// claim about *this* package: that its TestMain still goes through the guard,
// which a re-ordering of the wrappers above could quietly take away while
// everything else stayed green.
func TestTheSuiteRunsInAHomeOfItsOwn(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" || home != os.Getenv(testenv.TripwireHome) {
		t.Fatalf("HOME = %q, want the assembled scratch home %q", home, os.Getenv(testenv.TripwireHome))
	}
	if left := testenv.EntriesUnder(home); len(left) > 0 {
		t.Errorf("the suite's home already holds %v; it is meant to stay empty", left)
	}
	if v, ok := os.LookupEnv("HISTFILE"); !ok || v != "" {
		t.Errorf("HISTFILE = %q, %v; want it set and empty, which is recording off", v, ok)
	}
	// Everything the startup and history surface reads and this suite is not
	// about. Each aims a read or a write at a real directory when it survives.
	for _, name := range []string{
		"ZDOTDIR", "ENV", "BASH_ENV", "SHELLOPTS", "FPATH",
		"HISTSIZE", "HISTFILESIZE",
		"XDG_STATE_HOME", "SH_BLOCKS_DIR", "SH_BLOCKS_OUTPUT",
	} {
		if v, ok := os.LookupEnv(name); ok {
			t.Errorf("%s = %q survived the scrub", name, v)
		}
	}
}
