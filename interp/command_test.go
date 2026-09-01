// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `command -v` says what would run without running it.
func TestCommandVSaysWhatWouldRun(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`command -v echo`, "echo"},
		{`f() { :; }; command -v f`, "f"},
		{`command -v if`, "if"},
		{`command -v while`, "while"},
		// An external is named by its path, which is the part a script
		// cannot work out for itself.
		{`command -v sh`, "/bin/sh"},
		// Nothing found prints nothing at all — the silence is what makes
		// `command -v x >/dev/null` the usual spelling.
		{`command -v nosuchthing_at_all`, ""},
		// A function is named even though `command` without -v refuses to
		// run it.
		{`f() { :; }; command -v f; command f 2>/dev/null; echo "st=$?"`, "f\nst=127"},
	} {
		if out, _ := run(t, tc.src, nil); strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
		}
	}
}

// The whole reason `command` exists: a function may wrap the thing it is
// named after without calling itself.
func TestCommandBypassesAFunction(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo() { echo overridden; }; command echo hi`, "hi"},
		{`echo() { command echo wrapped "$@"; }; echo hi`, "wrapped hi"},
		// And the function is still there afterwards — bypassed for the one
		// command, not removed.
		{`greet() { printf fn; }; command echo hi; greet`, "hi\nfn"},
	} {
		if out, _ := run(t, tc.src, nil); strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
		}
	}
}

// What `command -v` reports when the name is nothing at all.
func TestCommandNotFoundStatusIsAnAxis(t *testing.T) {
	answer := func(a Answer) func(*Runner) {
		return func(r *Runner) {
			s := *r.Semantics
			s.CommandNotFoundStatusIsNotFound = a
			r.Semantics = &s
		}
	}
	for _, tc := range []struct {
		a    Answer
		want int
	}{
		{Yes, 127},
		{No, 1},
	} {
		_, status := run(t, `command -v nosuchthing_at_all`, answer(tc.a))
		if status != tc.want {
			t.Errorf("%v: status %d, want %d", tc.a, status, tc.want)
		}
	}
	// Finding one needs no dialect: the axis is only about the answer when
	// there is none.
	if _, status := run(t, `command -v echo`, answer(Unspecified)); status != 0 {
		t.Errorf("status %d, want 0", status)
	}
}

// `builtin` runs a builtin and only a builtin.
func TestBuiltinRunsOnlyABuiltin(t *testing.T) {
	for _, tc := range []struct {
		src    string
		want   string
		status int
	}{
		{`builtin echo hi`, "hi", 0},
		{`echo() { echo o; }; builtin echo hi`, "hi", 0},
		// Not a builtin is refused rather than run as an external.
		{`builtin ls 2>&1`, "not a shell builtin", 1},
		{`builtin nosuchthing 2>&1`, "not a shell builtin", 1},
		// With nothing to run it does nothing, successfully.
		{`builtin`, "", 0},
	} {
		out, status := run(t, tc.src, nil)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s = %q, want it to contain %q", tc.src, out, tc.want)
		}
		if status != tc.status {
			t.Errorf("%s: status %d, want %d", tc.src, status, tc.status)
		}
	}
}
