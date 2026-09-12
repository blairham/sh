// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bash's `history`, from the outside, through the flags a person types.
//
// Four of its letters name a file and two of those *write* one, in a shell
// with no terminal anywhere: bash keeps a history list in a script and
// `history -w out` creates the file even when that list is empty. So this is
// an ordinary script writing to a path it chose, which is what `make sandbox`
// graded `builtin/history-write` as `inert` about — "not a builtin yet,
// falls through to a refused exec" — until the builtin and this gate landed
// together.
//
// Every case is decided by the filesystem or by what reached the script, for
// the reason sandboxfiles_test.go gives: a check that cannot tell "refused"
// from "did not work" would let the next escape in the same way. Each denied
// case has an allowed sibling below.

// hist runs a script from inside the workspace with the given policy.
func hist(t *testing.T, ws, policy, script string) outcome {
	t.Helper()
	return sandboxed(t, "bash", "-policy", policy, "-c", "cd "+ws+"\n"+script+"\n")
}

func TestTheHistoryBuiltinIsInsideTheBoundary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		make   func(t *testing.T, at string)
		script string // %s is the path outside the workspace
		held   func(at string) bool
		want   string
	}{{
		name:   "-w to a denied path",
		make:   func(*testing.T, string) {},
		script: `history -s "echo remembered"; history -w %s`,
		held:   func(at string) bool { return !exists(at) },
		want:   "no history file was created outside",
	}, {
		name: "-w through HISTFILE",
		make: func(*testing.T, string) {},
		// The path can arrive from the *variable* rather than from an
		// operand, and a gate placed on the operand alone would miss it
		// entirely — which is the more likely spelling in a real rc file.
		script: `HISTFILE=%s; history -s x; history -w`,
		held:   func(at string) bool { return !exists(at) },
		want:   "the default path is gated like an operand",
	}, {
		name:   "-w over something",
		make:   func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS") },
		script: `history -s x; history -w %s`,
		held:   func(at string) bool { return keeps(at) },
		want:   "the file still holds what it held",
	}, {
		name:   "-a to a denied path",
		make:   func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS") },
		script: `history -s x; history -a %s`,
		held:   func(at string) bool { return keeps(at) },
		want:   "an append is a write and answers to the same rule",
	}, {
		name:   "-r from a denied path",
		make:   func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS") },
		script: `history -r %s; history`,
		held:   func(at string) bool { return exists(at) },
		want:   "the contents did not reach the list",
	}, {
		name:   "-n from a denied path",
		make:   func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS") },
		script: `history -n %s; history`,
		held:   func(at string) bool { return exists(at) },
		want:   "the unread-lines letter is the same read",
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			outside, ws, policy := filesFixture(t)
			at := filepath.Join(outside, "target")
			c.make(t, at)
			got := hist(t, ws, policy, strings.ReplaceAll(c.script, "%s", at))
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

// keeps asks the filesystem whether a denied file still holds what it held.
//
// Not "does it exist": `-w` opens with O_TRUNC, so a gate that ran after the
// open would leave the name in place and the contents gone — which is exactly
// the shape a test asserting existence alone would pass.
func keeps(at string) bool {
	data, err := os.ReadFile(at)
	return err == nil && string(data) == "PRECIOUS"
}

// The other half of every case above, without which they would all pass for a
// builtin that does not work.
//
// One script, because the letters compose the way a caller uses them: a list
// is built, written, added to, appended, cleared and read back.
//
// The whole output is asserted rather than a substring, and the expected
// answer is the one bash 5.3.15 gives for the same seven lines — measured
// side by side, byte for byte. It is worth reading, because the duplicate is
// the point: `-w` puts the whole list down, and the `-a` after it appends
// everything since the last *append*, which was none, so `echo one` lands
// twice. A test asserting "contains echo one" and "contains echo two" would
// pass for a shell that got that interaction wrong in either direction — and
// did, until this expectation was checked against bash rather than guessed.
func TestTheHistoryBuiltinStillWorksWhereThePolicyAllowsIt(t *testing.T) {
	t.Parallel()
	_, ws, policy := filesFixture(t)

	got := hist(t, ws, policy, strings.Join([]string{
		`history -s "echo one"`,
		`history -w saved`,
		`history -s "echo two"`,
		`history -a saved`,
		`history -c`,
		`history -r saved`,
		`history`,
	}, "\n"))

	want := "    1  echo one\n    2  echo one\n    3  echo two\n"
	if got.out != want {
		t.Errorf("out = %q, want %q\nerrs = %q", got.out, want, got.errs)
	}
}

// A refused write says so, in the words a refused redirection uses.
//
// There is no honest way to be quiet about it: a history file that was not
// written is not there, and a builtin that returned 0 would leave a script
// believing it had saved what somebody typed.
func TestARefusedHistoryWriteSaysSo(t *testing.T) {
	t.Parallel()
	outside, ws, policy := filesFixture(t)
	at := filepath.Join(outside, "saved")

	got := hist(t, ws, policy, `history -s x; history -w `+at)
	if !strings.Contains(got.errs, "refused") {
		t.Errorf("errs = %q, want the refusal reported the way a redirection's is", got.errs)
	}
	if got.code == 0 {
		t.Error("status = 0: a write that did not happen reported success")
	}
	if exists(at) {
		t.Error("the file was created")
	}
}

// A denied read tells a script nothing about whether the file is there.
//
// The pair is the claim, with the operand normalized out of both: a path the
// policy covers that holds something and one that holds nothing must answer
// alike, or the refusal is an oracle for what the policy hides.
func TestADeniedHistoryReadSaysNothingAboutWhetherTheFileIsThere(t *testing.T) {
	t.Parallel()
	outside, ws, policy := filesFixture(t)
	secret := filepath.Join(outside, "secret")
	writeFile(t, secret, "PRECIOUS")
	missing := filepath.Join(outside, "nosuch")

	refused := hist(t, ws, policy, `history -r `+secret+`; history`)
	absent := hist(t, ws, policy, `history -r `+missing+`; history`)

	norm := func(s, path string) string { return strings.ReplaceAll(s, path, "PATH") }
	if refused.out != absent.out {
		t.Errorf("refused = %q, absent = %q: the list a denied read leaves differs",
			refused.out, absent.out)
	}
	if got, want := norm(refused.errs, secret), norm(absent.errs, missing); got != want {
		t.Errorf("refused = %q, absent = %q: a script can tell a denied file that exists "+
			"from a denied one that does not", got, want)
	}
}
