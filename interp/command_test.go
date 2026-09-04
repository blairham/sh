// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
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

// An external is named by its path, which is the part a script cannot work
// out for itself. The executable is made here rather than borrowed from the
// host: `sh` is /bin/sh on a Mac and /usr/bin/sh on Ubuntu, so a test that
// named a real one would be asserting where the machine keeps its shell.
func TestCommandVNamesAnExternalByItsPath(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "onlyhere")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	setup := func(r *Runner) { r.Env = []string{"PATH=" + dir} }
	if out, _ := run(t, `command -v onlyhere`, setup); strings.TrimSpace(out) != exe {
		t.Errorf("command -v onlyhere = %q, want %q", strings.TrimSpace(out), exe)
	}
	// Off PATH it is not found, so the path in the answer above came from
	// the search and not from the name.
	setup = func(r *Runner) { r.Env = []string{"PATH="} }
	if out, st := run(t, `command -v onlyhere`, setup); strings.TrimSpace(out) != "" || st == 0 {
		t.Errorf("off PATH: %q status %d, want silence and a failure", out, st)
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

// TestAnOptionCommandDoesNotHaveIsRefusedOrRun. All four read -v and -p;
// what splits them is anything else, and both answers are a real dialect's.
func TestAnOptionCommandDoesNotHaveIsRefusedOrRun(t *testing.T) {
	answer := func(a Answer) func(*Runner) {
		return func(r *Runner) {
			s := *r.Semantics
			s.CommandRejectsUnknownOption = a
			r.Semantics = &s
			dg := Diagnostics{BuiltinBadOption: "command: %[2]s: invalid option"}
			r.Diagnostics = &dg
		}
	}

	out, _ := run(t, `command -x echo hi`, answer(Yes))
	if !strings.Contains(out, "-x: invalid option") {
		t.Errorf("refused: got %q, want it named as an option", out)
	}
	if strings.Contains(out, "hi") {
		t.Errorf("refused: got %q, want the command not to have run", out)
	}

	// Taken as the command instead, which is the other half — and which is
	// why this is not simply "read the options".
	out, _ = run(t, `command -x echo hi`, answer(No))
	if strings.Contains(out, "invalid option") {
		t.Errorf("run: got %q, want no option complaint", out)
	}

	// And the options it does have are read either way, which is the part a
	// single "reads options" flag got wrong.
	for _, a := range []Answer{Yes, No} {
		out, _ = run(t, `command -v echo`, answer(a))
		if !strings.Contains(out, "echo") {
			t.Errorf("%v: got %q, want -v read whatever the answer", a, out)
		}
	}
}

// TestABundleGivenToCommandIsNamedByItsFirstLetter, so `--version` comes back
// as `--` — the rule printf's options here already follow.
func TestABundleGivenToCommandIsNamedByItsFirstLetter(t *testing.T) {
	out, _ := run(t, `command --version`, func(r *Runner) {
		s := *r.Semantics
		s.CommandRejectsUnknownOption = Yes
		r.Semantics = &s
		dg := Diagnostics{BuiltinBadOption: "command: %[2]s: invalid option"}
		r.Diagnostics = &dg
	})
	if !strings.Contains(out, "--: invalid option") {
		t.Errorf("got %q, want the first letter of the bundle", out)
	}
	if strings.Contains(out, "--version: invalid") {
		t.Errorf("got %q, want the whole word not named", out)
	}
}

// TestWhatCommandRunsReportsAsItself, not as the builtin: the dialect that
// names a builtin in the location says `sh:1: command not found: -x` and not
// `sh:command:1:`, which is the rule `.` and `eval` already follow.
func TestWhatCommandRunsReportsAsItself(t *testing.T) {
	out, _ := run(t, `command nosuchcmd-xyz`, func(r *Runner) {
		s := *r.Semantics
		s.CommandRejectsUnknownOption = Yes
		r.Semantics = &s
		dg := Diagnostics{
			Location:               LocationTightLine,
			NamesBuiltinInLocation: true,
			PathNotFound:           "command not found: %[1]s",
		}
		r.Diagnostics = &dg
	})
	if strings.Contains(out, ":command:") {
		t.Errorf("got %q, want the builtin not named in the location", out)
	}
}
