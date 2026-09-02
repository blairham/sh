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

// `type` says what a name would run, and every part of the sentence is a
// dialect's own words — down to whether a missing name gets the shell's name
// in front of it.
func TestTypeSaysWhatANameWouldRun(t *testing.T) {
	dg := Diagnostics{
		TypeKeyword:  "%[1]s is a reserved word",
		TypeFunction: "%[1]s is a shell function",
		TypeExternal: "%[1]s is a tracked alias for %[2]s",
	}
	for _, tc := range []struct{ name, src, want string }{
		{"a builtin", `type cd`, "cd is a shell builtin"},
		{"a keyword", `type if`, "if is a reserved word"},
		{"a function", `f(){ :; }; type f`, "f is a shell function"},
		{"an external", `type /bin/ls`, "/bin/ls is a tracked alias for /bin/ls"},
		// A function wins over a builtin of the same name, which is the
		// order a command is actually resolved in.
		{"a function shadowing a builtin", `cd(){ :; }; type cd`, "cd is a shell function"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, func(r *Runner) {
				sem := CoreSemantics()
				sem.TypePrintsFunctionBody = No
				r.Semantics, r.Diagnostics = &sem, &dg
			})
			if st != 0 {
				t.Fatalf("status %d: %s", st, out)
			}
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("out = %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// A name it cannot account for: four wordings, two statuses, and two of the
// four print the line with nothing in front of it.
func TestTypeReportsANameThatIsNothing(t *testing.T) {
	for _, tc := range []struct {
		name       string
		dg         Diagnostics
		wantStatus int
	}{
		{"a plain failure", Diagnostics{TypeNotFound: "type: %[1]s: not found"}, 1},
		{"or a missing command's", Diagnostics{TypeNotFound: "%[1]s: not found", TypeNotFoundStatus: 127}, 127},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, `type nope`, func(r *Runner) {
				sem := CoreSemantics()
				r.Semantics, r.Diagnostics = &sem, &tc.dg
			})
			if st != tc.wantStatus {
				t.Errorf("status %d, want %d", st, tc.wantStatus)
			}
			if !strings.Contains(out, "nope") {
				t.Errorf("out = %q, want the name in it", out)
			}
		})
	}
}

// One name failing does not stop the rest, and the failure is what the whole
// command reports.
func TestTypeAnswersEveryNameAndReportsTheFailure(t *testing.T) {
	out, st := run(t, `type cd nope ls`, func(r *Runner) {
		sem := CoreSemantics()
		dg := Diagnostics{TypeNotFound: "type: %[1]s: not found"}
		r.Semantics, r.Diagnostics = &sem, &dg
	})
	for _, want := range []string{"cd is a shell builtin", "nope", "ls is /"} {
		if !strings.Contains(out, want) {
			t.Errorf("out = %q, want it to contain %q", out, want)
		}
	}
	if st == 0 {
		t.Error("status 0, want the one failure reported")
	}
}

// The same guard `command -v` has: there is an executable called
// /usr/bin/umask and this shell will not run it, so `type` must not say where
// it is. A script that asks before using it would be told yes and then fail.
func TestTypeWillNotNameAReservedBuiltinOnPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alias"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	out, st := run(t, `type alias`, func(r *Runner) {
		sem := CoreSemantics()
		dg := Diagnostics{TypeNotFound: "type: %[1]s: not found"}
		r.Semantics, r.Diagnostics = &sem, &dg
		r.Vars = map[string]string{"PATH": dir}
	})
	if strings.Contains(out, dir) {
		t.Errorf("out = %q, want the shell not to name a PATH hit it would refuse to run", out)
	}
	if st == 0 {
		t.Error("status 0, want it reported as not found")
	}
}

// bash follows the sentence with the function itself, reformatted. That needs
// a printer for the syntax tree and there is not one, so it refuses rather
// than printing the sentence and dropping the half that was asked for.
func TestTypeRefusesAFunctionBodyItCannotPrint(t *testing.T) {
	out, st := run(t, `f(){ echo hi; }; type f`, func(r *Runner) {
		sem := CoreSemantics()
		sem.TypePrintsFunctionBody = Yes
		r.Semantics = &sem
	})
	if !strings.Contains(out, "not implemented yet") {
		t.Errorf("out = %q, want an honest refusal", out)
	}
	if strings.Contains(out, "is a function") {
		t.Errorf("out = %q, want no half answer before it", out)
	}
	if st == 0 {
		t.Error("status 0, want the refusal reported")
	}
}
