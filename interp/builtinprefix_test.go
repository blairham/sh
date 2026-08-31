// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// Naming a builtin inside a diagnostic's location is one dialect's habit, so
// these name the *flag* and never the shell.

// prefixRun runs src and returns stderr, which is where diagnostics go.
func prefixRun(t *testing.T, dir, src string, dg Diagnostics) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out, errs bytes.Buffer
	sem := permissive()
	r := &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
	}
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return errs.String()
}

// TestTheBuiltinIsNamedInTheLocationOnlyWhenAsked covers the flag in both
// directions. Off is the default and is what three of the four dialects want.
func TestTheBuiltinIsNamedInTheLocationOnlyWhenAsked(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.sh")

	off := prefixRun(t, dir, `. `+missing, Diagnostics{
		Location: LocationTightLine, DotCannotOpen: ".: cannot open %[1]s",
	})
	if strings.Contains(off, ":.:") {
		t.Errorf("off: %q should not name the builtin in the location", off)
	}

	on := prefixRun(t, dir, `. `+missing, Diagnostics{
		Location: LocationTightLine, NamesBuiltinInLocation: true,
		DotCannotOpen: "cannot open %[1]s",
	})
	if !strings.Contains(on, "testsh:.:1:") {
		t.Errorf("on: %q should read testsh:.:1:", on)
	}
}

// TestOnlyABuiltinsOwnDiagnosticIsNamed is the rule, and the distinction is
// "whose diagnostic is this" rather than "where did it happen". A command that
// could not be found is the shell's failure, not a builtin's.
func TestOnlyABuiltinsOwnDiagnosticIsNamed(t *testing.T) {
	dg := Diagnostics{Location: LocationTightLine, NamesBuiltinInLocation: true}
	out := prefixRun(t, t.TempDir(), `nosuchcmd-xyz`, dg)
	// On the *prefix*, not a colon count: the message carries colons of its
	// own, so counting them says nothing about the location.
	if !strings.HasPrefix(out, "testsh:1:") {
		t.Errorf("output = %q, want it to open with the plain location", out)
	}
}

// TestTheNameIsNotSaidTwice is the reason the stripping exists.
//
// Sixteen diagnostics in this package open with the builtin's own name, which
// is what every other dialect prints — `cd: /x: no such directory`. A dialect
// that also puts the name in the location would say it twice, so the message
// gives it up. Stripped centrally rather than at sixteen sites.
func TestTheNameIsNotSaidTwice(t *testing.T) {
	out := prefixRun(t, t.TempDir(), `cd /no/such/dir-xyz`, Diagnostics{
		Location: LocationTightLine, NamesBuiltinInLocation: true,
	})
	if strings.Count(out, "cd") != 1 {
		t.Errorf("output %q names cd %d times, want once",
			out, strings.Count(out, "cd"))
	}
	if !strings.Contains(out, "testsh:cd:1:") {
		t.Errorf("output = %q, want the name in the location", out)
	}
}

// TestABorrowedScriptIsNotTheBuiltinsDiagnostic is the trap this had to avoid.
//
// `.` and `eval` run text that is not theirs. What that text reports belongs
// to it, not to the builtin that read it — measured, and the dialect that
// names builtins agrees: an unset parameter inside a sourced file names no
// builtin at all. Leaving the marker set would have put one on every line the
// script produced.
func TestABorrowedScriptIsNotTheBuiltinsDiagnostic(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(script, []byte("set -u\necho \"$NOPE\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dg := Diagnostics{Location: LocationTightLine, NamesBuiltinInLocation: true}

	for _, tc := range []struct{ name, src string }{
		{"a sourced file", `. ` + script},
		{"eval", `set -u; eval "echo \"$NOPE\""`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := prefixRun(t, dir, tc.src, dg)
			if strings.Contains(out, ":.:") || strings.Contains(out, ":eval:") {
				t.Errorf("output %q claims the builtin reported what the script did", out)
			}
			if !strings.Contains(out, "parameter not set") {
				t.Fatalf("setup: expected the unset-parameter diagnostic, got %q", out)
			}
		})
	}
}

// TestTheBuiltinsOwnFailureIsStillNamedAfterRunningAScript — clearing the
// marker for borrowed text must not clear it for the builtin's own errors,
// which happen before any of that text runs.
func TestTheBuiltinsOwnFailureIsStillNamedAfterRunningAScript(t *testing.T) {
	dir := t.TempDir()
	out := prefixRun(t, dir, `. `+filepath.Join(dir, "absent.sh"), Diagnostics{
		Location: LocationTightLine, NamesBuiltinInLocation: true,
	})
	if !strings.Contains(out, "testsh:.:1:") {
		t.Errorf("output = %q: `.` failing to open is `.`'s own diagnostic", out)
	}
}

// TestANestedBuiltinRestoresTheOuterName — a builtin can run another one, so
// the marker is saved and put back rather than cleared.
func TestANestedBuiltinRestoresTheOuterName(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "inner.sh")
	// The sourced file runs a builtin that fails, and then `.` itself fails on
	// a second, missing operand. The second diagnostic must still say `.`.
	if err := os.WriteFile(script, []byte("cd /no/such/dir-xyz\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := prefixRun(t, dir,
		`. `+script+`; . `+filepath.Join(dir, "absent.sh"),
		Diagnostics{Location: LocationTightLine, NamesBuiltinInLocation: true})

	if !strings.Contains(out, "testsh:cd:") {
		t.Errorf("output = %q, want the inner builtin named", out)
	}
	if !strings.Contains(out, "testsh:.:") {
		t.Errorf("output = %q, want the outer builtin named again afterwards", out)
	}
}
