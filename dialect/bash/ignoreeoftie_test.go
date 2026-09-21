// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `IGNOREEOF` and `ignoreeof` are two spellings of one state, and every row
// below moves one of them and reads the other back.
//
// Measured 2026-09-21 against bash 5.3.20 under `env -i PATH=/usr/bin:/bin
// LC_ALL=C` with a scratch HOME, from a script file, reading the option out
// of `set -o` (#4047). It is `set -o` and not `shopt`: `shopt ignoreeof` is
// `invalid shell option name` in that shell and in this one.
func TestTheEndOfFileCountAndItsOptionAreOneState(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, wantOption, wantValue string }{
		{"neither is set to start with", "", "off", "unset"},
		// Any assignment turns it on, so the tie reads the assignment and
		// not the value: the empty one is the row that says so.
		{"an assignment turns the option on", "IGNOREEOF=10\n", "on", "10"},
		{"an empty assignment does too", "IGNOREEOF=\n", "on", ""},
		{"and one that is not a count", "IGNOREEOF=abc\n", "on", "abc"},
		{"and a zero", "IGNOREEOF=0\n", "on", "0"},
		// And the removal turns it off rather than leaving it on with an
		// absent count.
		{"unsetting turns it off", "IGNOREEOF=10\nunset IGNOREEOF\n", "off", "unset"},
		// The other direction writes a value of its own, and it overwrites
		// one already there rather than leaving it.
		{"the option writes the count", "set -o ignoreeof\n", "on", "10"},
		{"and writes over a count already there", "IGNOREEOF=3\nset -o ignoreeof\n", "on", "10"},
		{"turning it off takes the name away", "set -o ignoreeof\nset +o ignoreeof\n", "off", "unset"},
		// The name is not exported by any of that, which the last row
		// checks separately below.
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, errs := runOptionScript(t,
				c.src+"set -o\necho \"v=${IGNOREEOF-unset}\"\n")
			if errs != "" {
				t.Fatalf("complained: %q", errs)
			}
			if got := listedOption(t, out, "ignoreeof"); got != c.wantOption {
				t.Errorf("the option reads %q, want %q (out = %q)", got, c.wantOption, out)
			}
			if want := "v=" + c.wantValue + "\n"; !strings.HasSuffix(out, want) {
				t.Errorf("out = %q, want it to end with %q", out, want)
			}
		})
	}
}

// The option writing the parameter does not export it, and the parameter
// writing the option does not send the write back round: the value a script
// assigned is the value that stands.
//
// The second half is what one reentry flag is for. Without it the assignment
// would turn the option on, the option would write its own count over the
// assignment, and the script's value would be the one that lost.
func TestTheTieDoesNotExportOrOverwriteWhatWasAssigned(t *testing.T) {
	t.Parallel()
	out, errs := runOptionScript(t, "IGNOREEOF=3\necho \"v=$IGNOREEOF\"\nexport -p\n")
	if errs != "" {
		t.Fatalf("complained: %q", errs)
	}
	if !strings.HasPrefix(out, "v=3\n") {
		t.Errorf("out = %q, want the assigned value to stand", out)
	}
	if strings.Contains(out, "IGNOREEOF") {
		t.Errorf("out = %q, want the name unexported", out)
	}
}

// And a name the environment carried is not an assignment: the value shows
// through and the option stays off, measured the same day on the same shell.
func TestAnInheritedCountDoesNotTurnTheOptionOn(t *testing.T) {
	t.Parallel()
	out, st := runBashWithEnv(t, map[string]string{"IGNOREEOF": "4"},
		"set -o\necho \"v=${IGNOREEOF-unset}\"\n")
	if got := listedOption(t, out, "ignoreeof"); got != "off" || st != 0 {
		t.Errorf("the option reads %q at %d, want off at 0 (out = %q)", got, st, out)
	}
	if !strings.HasSuffix(out, "v=4\n") {
		t.Errorf("out = %q, want the inherited value to show through", out)
	}
}

// runBashWithEnv runs a script with names the *environment* carried, which is
// not an assignment: the variable table starts holding them and nothing this
// shell does is a write.
func runBashWithEnv(t *testing.T, vars map[string]string, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir(), Vars: vars}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// listedOption is one row of a `set -o` listing, read back by name. In Go
// rather than through a pipeline, because these runs have no `grep` on their
// PATH — and a missing program would read as an option that was off.
func listedOption(t *testing.T, listing, name string) string {
	t.Helper()
	for _, line := range strings.Split(listing, "\n") {
		if rest, ok := strings.CutPrefix(line, name); ok {
			return strings.TrimSpace(rest)
		}
	}
	t.Fatalf("no %s row in %q", name, listing)
	return ""
}
