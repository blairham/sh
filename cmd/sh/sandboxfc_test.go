// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `fc`'s file letters, from the outside, through the flags a person types —
// including the one that makes the shell a session.
//
// This was the last row on `make sandbox`'s inert ledger, and the only one of
// the four that needed the *instrument* fixed as well as the shell: zsh writes
// a history file only when the shell is interactive, so a route run through
// `-c` could not have gone green however faithfully the letter was written.
// #2283 gave `sandboxcheck.Route` an `Args` field for exactly this row.
//
// So every case here passes `-i`, and the pair in
// TestFcWritesNothingFromAScriptWhateverThePolicySays is what keeps that
// honest: without it, a gate that refused everything and a shell that wrote
// nothing would look the same.

// fcSession runs a script as an interactive shell with the given policy.
func fcSession(t *testing.T, ws, policy, script string) outcome {
	t.Helper()
	return sandboxed(t, "zsh", "-policy", policy, "-i", "-c", "cd "+ws+"\n"+script+"\n")
}

func TestTheFcFileLettersAreInsideTheBoundary(t *testing.T) {
	t.Parallel()
	const seed = "SAVEHIST=10\nprint -s 'echo remembered'\n"
	cases := []struct {
		name   string
		make   func(t *testing.T, at string)
		script string // %s is the path outside the workspace
		held   func(at string) bool
		want   string
	}{{
		name:   "-W to a denied path",
		make:   func(*testing.T, string) {},
		script: seed + `HISTFILE=%s` + "\n" + `fc -W`,
		held:   func(at string) bool { return !exists(at) },
		want:   "no history file was created outside",
	}, {
		name:   "-W with an operand",
		make:   func(*testing.T, string) {},
		script: seed + `fc -W %s`,
		held:   func(at string) bool { return !exists(at) },
		want:   "an operand is the same write as $HISTFILE",
	}, {
		name:   "-W over something",
		make:   func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS") },
		script: seed + `fc -W %s`,
		held:   func(at string) bool { return untouchedByFc(at) },
		want:   "the file still holds what it held",
	}, {
		name:   "-A to a denied path",
		make:   func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS") },
		script: seed + `fc -A %s`,
		held:   func(at string) bool { return untouchedByFc(at) },
		want:   "an append opens the same path on a different letter",
	}, {
		name:   "-R from a denied path",
		make:   func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS") },
		script: `fc -R %s` + "\n" + `fc -l`,
		held:   func(at string) bool { return exists(at) },
		want:   "the contents did not reach the list",
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			outside, ws, policy := filesFixture(t)
			at := filepath.Join(outside, "target")
			c.make(t, at)
			got := fcSession(t, ws, policy, strings.ReplaceAll(c.script, "%s", at))
			if !c.held(at) {
				t.Errorf("%s escaped the boundary: want %s\nerrs = %q",
					c.name, c.want, got.errs)
			}
			if strings.Contains(got.out, "PRECIOUS") {
				t.Errorf("out = %q: the contents of a denied file reached the script", got.out)
			}
		})
	}
}

func untouchedByFc(at string) bool {
	data, err := os.ReadFile(at)
	return err == nil && string(data) == "PRECIOUS"
}

// The other half, without which every case above passes for a shell that
// writes no history file at all — which, for `-c`, is the correct behaviour
// and is exactly the trap.
//
// So the sibling is run the same way, `-i` included, into the workspace the
// policy allows. If this did not write, nothing above would be evidence.
func TestTheFcFileLettersStillWorkWhereThePolicyAllowsThem(t *testing.T) {
	t.Parallel()
	_, ws, policy := filesFixture(t)

	got := fcSession(t, ws, policy, strings.Join([]string{
		`SAVEHIST=10`,
		`HISTFILE=saved`,
		`print -s 'echo one'`,
		`fc -W`,
		`print -s 'echo two'`,
		`fc -A other`,
		`fc -R saved`,
		`fc -l`,
	}, "\n"))

	for _, want := range []string{"echo one", "echo two"} {
		if !strings.Contains(got.out, want) {
			t.Errorf("out = %q, want it to contain %q\nerrs = %q", got.out, want, got.errs)
		}
	}
	if _, err := os.Stat(filepath.Join(ws, "saved")); err != nil {
		t.Errorf("the allowed history file was not written: %v\nerrs = %q", err, got.errs)
	}
}

// A script writes no history file wherever it points, and that is zsh's
// answer rather than the policy's.
//
// This is the row's whole history in one case. `make sandbox` ran every route
// through `-c` and graded this one `inert` for it, with a note saying the
// letter was not accepted — which was true and was not the reason. The claim
// here is the one that made the route need an `Args` field: **the same script
// that writes under `-i` writes nothing without it, even when the policy
// allows the path outright.**
func TestFcWritesNothingFromAScriptWhateverThePolicySays(t *testing.T) {
	t.Parallel()
	_, ws, policy := filesFixture(t)
	const script = "SAVEHIST=10\nHISTFILE=saved\nprint -s 'echo x'\nfc -W\n"

	quiet := sandboxed(t, "zsh", "-policy", policy, "-c", "cd "+ws+"\n"+script)
	if _, err := os.Stat(filepath.Join(ws, "saved")); err == nil {
		t.Errorf("a script wrote a history file into an allowed directory, "+
			"which zsh does not\nerrs = %q", quiet.errs)
	}

	// And the same script under `-i`, into the same allowed directory, does.
	// Without this line the one above passes for a shell whose `fc` is broken.
	session := fcSession(t, ws, policy, script)
	if _, err := os.Stat(filepath.Join(ws, "saved")); err != nil {
		t.Errorf("a session did not write into an allowed directory: %v\nerrs = %q",
			err, session.errs)
	}
}
