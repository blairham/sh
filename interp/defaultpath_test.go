// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/internal/testenv"
)

// decoyOnPath writes an executable of the given name into a directory of its
// own and answers with the directory and the path.
//
// A decoy rather than a fixture, because what these tests need is a PATH that
// holds *the wrong* answer: a `-p` search that reported it would be searching
// the caller's PATH under another name.
func decoyOnPath(t *testing.T, name, body string) (dir, exe string) {
	t.Helper()
	dir = t.TempDir()
	exe = filepath.Join(dir, name)
	if err := testenv.WriteExecutable(exe, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, exe
}

// `command -p` searches a default path that holds the standard utilities,
// which is the whole of what the letter is for: a script reaches for it when
// PATH is the thing it cannot trust.
//
// The assertion is "not the decoy, and something that exists" rather than a
// path, because where a machine keeps its shell is not this shell's business
// — it is /bin/sh on a Mac and /usr/bin/sh on Ubuntu, and both are on the
// default path.
func TestCommandDashPSearchesTheDefaultPathAndNotTheCallers(t *testing.T) {
	dir, decoy := decoyOnPath(t, "sh", "exit 7")
	onDecoyPath := func(r *Runner) { r.Env = []string{"PATH=" + dir} }

	// The fixture first: without the letter, the decoy is what the search
	// finds. A test whose decoy was never reachable would pass for a shell
	// that found nothing at all.
	if out, st := run(t, `command -v sh`, onDecoyPath); strings.TrimSpace(out) != decoy || st != 0 {
		t.Fatalf("command -v sh = %q status %d, want the decoy %q", out, st, decoy)
	}

	for _, tc := range []struct {
		name  string
		setup func(*Runner)
	}{
		{"a PATH holding the wrong answer", onDecoyPath},
		// The case the letter exists for, and the one #2933 was filed on.
		{"a PATH holding nothing", func(r *Runner) { r.Env = []string{"PATH="} }},
		{"a PATH pointing nowhere", func(r *Runner) { r.Env = []string{"PATH=/nonexistent_zz"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, `command -pv sh`, tc.setup)
			got := strings.TrimSpace(out)
			if st != 0 || got == "" {
				t.Fatalf("command -pv sh = %q status %d, want a standard utility", out, st)
			}
			if got == decoy {
				t.Errorf("command -pv sh = %q, want the default path rather than the caller's", got)
			}
			if info, err := os.Stat(got); err != nil || !info.Mode().IsRegular() {
				t.Errorf("command -pv sh named %q, which is not a program here (%v)", got, err)
			}
		})
	}
}

// And it really runs it, not only reports it — the two are one question asked
// twice, and a script that tested with `-pv` and ran with `-p` would be
// testing the wrong path if they parted.
func TestCommandDashPRunsWhatItReports(t *testing.T) {
	empty := func(r *Runner) { r.Env = []string{"PATH="} }
	if _, st := run(t, `command -p sh -c 'exit 3'`, empty); st != 3 {
		t.Errorf("command -p sh -c 'exit 3' left %d, want 3", st)
	}
	// The control: the same command without the letter cannot be found at
	// all, so the run above went through the default path and not through
	// some other door.
	if _, st := run(t, `command sh -c 'exit 3' 2>/dev/null`, empty); st != 127 {
		t.Errorf("without -p the status is %d, want 127", st)
	}
}

// The child's environment is the caller's. `-p` is a search path and never an
// assignment — measured across the panel, `command -p sh -c 'echo $PATH'`
// prints what the script set.
func TestCommandDashPLeavesTheEnvironmentAlone(t *testing.T) {
	dir, _ := decoyOnPath(t, "unused", ":")
	setup := func(r *Runner) { r.Env = []string{"PATH=" + dir} }
	out, st := run(t, `command -p sh -c 'printf "%s" "$PATH"'`, setup)
	if st != 0 || strings.TrimSpace(out) != dir {
		t.Errorf("the child saw PATH=%q status %d, want %q", out, st, dir)
	}
}

// The letter reaches the word `command` named and nothing that word goes on
// to run. Measured: `command -p eval '…'` looks the inner command up on the
// caller's PATH in every column of the panel.
func TestCommandDashPDoesNotReachInsideTheBuiltinItRan(t *testing.T) {
	dir, _ := decoyOnPath(t, "onlyhere_zz", "exit 0")
	setup := func(r *Runner) { r.Env = []string{"PATH=" + dir} }
	if _, st := run(t, `command -p eval 'onlyhere_zz'`, setup); st != 0 {
		t.Errorf("command -p eval 'onlyhere_zz' left %d, want the caller's PATH to have found it", st)
	}
	// And with the caller's PATH empty it is not found, which is what says
	// the success above came from that PATH rather than from anywhere else.
	empty := func(r *Runner) { r.Env = []string{"PATH="} }
	if _, st := run(t, `command -p eval 'onlyhere_zz' 2>/dev/null`, empty); st == 0 {
		t.Errorf("with an empty PATH it still ran, so the default path reached inside the eval")
	}
}

// What a default-path search found is not remembered in four of the five
// columns, because the hash is a memo of the *caller's* PATH and this search
// deliberately did not use it. Remembering it lets a later bare name run a
// program the script's own PATH cannot reach, which is bash's answer and is
// Semantics.DefaultPathSearchIsRemembered.
func TestADefaultPathSearchIsNotRemembered(t *testing.T) {
	empty := func(r *Runner) {
		r.Env = []string{"PATH="}
		sem := *r.Semantics
		sem.DefaultPathSearchIsRemembered = No
		r.Semantics = &sem
	}
	out, st := run(t, `command -pv sh >/dev/null; command -v sh; echo "st=$?"`, empty)
	if st != 0 {
		t.Fatalf("the script itself failed: %q status %d", out, st)
	}
	if strings.TrimSpace(out) != "st=1" && strings.TrimSpace(out) != "st=127" {
		t.Errorf("after `command -pv sh` a plain lookup said %q, want it still not found", out)
	}
}

// And the other answer, which is bash's: the path the default search resolved
// goes into the hash, so a later bare name finds it even though the caller's
// PATH never held it.
//
// The two suites are the same snippet under the two answers, which is the
// whole of what the axis is — a table that answered the same either way would
// be a field nothing reads.
func TestADefaultPathSearchIsRememberedWhereTheAxisSaysSo(t *testing.T) {
	setup := func(r *Runner) {
		r.Env = []string{"PATH="}
		sem := *r.Semantics
		sem.DefaultPathSearchIsRemembered = Yes
		r.Semantics = &sem
	}
	out, st := run(t, `command -pv sh >/dev/null; command -v sh >/dev/null; echo "st=$?"`, setup)
	if st != 0 {
		t.Fatalf("the script itself failed: %q status %d", out, st)
	}
	if strings.TrimSpace(out) != "st=0" {
		t.Errorf("after `command -pv sh` a plain lookup said %q, want it remembered at 0", out)
	}
}
