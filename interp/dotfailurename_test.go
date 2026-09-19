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

// One dialect diagnoses a dot script that would not open under the *operand's*
// name rather than under the running script's. These name the flag and never
// the shell.

// dotFailureRun runs src as a script file called s.sh in dir and returns
// stderr.
func dotFailureRun(t *testing.T, dir, src string, names bool) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out, errs bytes.Buffer
	sem := permissive()
	dg := Diagnostics{
		Location: LocationTightLine, NamesBuiltinInLocation: true,
		LocationNamesTheCurrentFile:          true,
		LocationNamesTheFunction:             true,
		LocationNamesTheEvalText:             true,
		EvalSourceName:                       "(eval)",
		DotCannotOpen:                        ".: %[1]s: %[2]s",
		DotFailureNamesTheFileItCouldNotOpen: names,
	}
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "s.sh", Route: RouteScriptFile,
		Vars: map[string]string{"PATH": dir},
	})
	r.SetScriptFile("s.sh")
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return errs.String()
}

// The flag in both directions, on the shape it is measured on: a `.` written
// at the top level of the script the shell was given.
func TestAFailedDotIsNamedAfterItsOperandOnlyWhenAsked(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name  string
		names bool
		want  string
	}{
		{"off, the running script keeps the slot", false, "s.sh:.:1:"},
		{"on, the file that would not open takes it", true, "./nope_zz:.:1:"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := dotFailureRun(t, dir, ". ./nope_zz\n", c.names)
			if !strings.HasPrefix(out, c.want) {
				t.Errorf("output = %q, want it to open %q", out, c.want)
			}
		})
	}
}

// The rename replaces the *script's* name and nothing else. A name already
// standing in that slot for another reason keeps it — measured over a
// function, `eval` and a file the script sourced — which is what says this is
// a rename rather than a rule about the message.
func TestAFailedDotDoesNotTakeANameAnotherFrameHasPut(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "nest.sh")
	if err := os.WriteFile(nested, []byte(". ./nope_zz\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		src  string
		want string
	}{
		{"inside a function, the function's", "f(){ . ./nope_zz; }\nf\n", "f:"},
		{"inside eval, eval's", "eval '. ./nope_zz'\n", "(eval):"},
		{"inside a sourced file, that file's", ". " + nested + "\n", nested + ":"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := dotFailureRun(t, dir, c.src, true)
			if !strings.HasPrefix(out, c.want) {
				t.Errorf("output = %q, want it to open %q", out, c.want)
			}
			if strings.HasPrefix(out, "./nope_zz") {
				t.Errorf("output = %q, the operand took a slot it does not own", out)
			}
		})
	}
}

// And a subshell boundary is not a frame: the same line inside `( … )` is
// still the top level of the script, which is the control that says the gate
// is the call stack rather than "the outermost statement".
func TestAFailedDotInASubshellIsStillTheScriptsOwnLine(t *testing.T) {
	out := dotFailureRun(t, t.TempDir(), "( . ./nope_zz )\n", true)
	if !strings.HasPrefix(out, "./nope_zz:.:1:") {
		t.Errorf("output = %q, want it to open %q", out, "./nope_zz:.:1:")
	}
}

// The name moves in the diagnostic and nowhere else: `$0` on the next line is
// the script's, measured on the reference.
func TestAFailedDotLeavesDollarZeroAlone(t *testing.T) {
	dir := t.TempDir()
	f, err := syntax.Parse(". ./nope_zz\nprintf 'next=%s\\n' \"$0\"\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	sem := permissive()
	dg := Diagnostics{
		Location: LocationTightLine, NamesBuiltinInLocation: true,
		LocationNamesTheCurrentFile:          true,
		LocationNamesTheFunction:             true,
		LocationNamesTheEvalText:             true,
		EvalSourceName:                       "(eval)",
		DotCannotOpen:                        ".: %[1]s: %[2]s",
		DotFailureNamesTheFileItCouldNotOpen: true,
	}
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "s.sh", Route: RouteScriptFile,
		Vars: map[string]string{"PATH": dir},
	})
	r.SetScriptFile("s.sh")
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatal(rerr)
	}
	if got, want := out.String(), "next=s.sh\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}
