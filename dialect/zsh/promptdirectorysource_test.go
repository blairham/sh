// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// `%~` here reads the directory the shell is **in**, not the `PWD`
// parameter — #4540.
//
// Measured 2026-09-26 on zsh 5.9.2, run `-f`; `go version -m` says *not a Go
// executable* for it:
//
//	cd <d>; PWD=/bogus; print -r -- ${(%):-%~}    <d>
//	cd <d>;             print -r -- ${(%):-%~}    <d>
//
// The second line is the control and it is the point: both answers are
// reachable, and only the assignment tells them apart. bash is the other way
// round on the same shape, which is why this is a field on the style rather
// than a rule in the walker — see interp.PromptStyle.CwdIsTheShellsOwnDirectory
// and dialect/bash for the other column.
//
// `%d`, which does no abbreviating, is read the same way.
func TestAPromptReadsTheDirectoryTheShellIsIn(t *testing.T) {
	if !zsh.PromptStyle().CwdIsTheShellsOwnDirectory {
		t.Fatal("CwdIsTheShellsOwnDirectory is off, so a written PWD would move this prompt")
	}
	dir := t.TempDir()
	for _, tc := range []struct{ name, src string }{
		{"%~ after a written PWD", `PWD=/bogus; print -rP -- '%~'`},
		{"%d after a written PWD", `PWD=/bogus; print -rP -- '%d'`},
		// The control: with nothing written over it the same line draws the
		// same directory, so the rows above are about the assignment alone.
		{"%~ with nothing written", `print -rP -- '%~'`},
		{"%d with nothing written", `print -rP -- '%d'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != dir+"\n" || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, dir+"\n")
			}
		})
	}
	// And the parameter really did hold the other answer, so the rows above
	// are not a shell that ignored the assignment.
	if out, st := runZsh(t, dir, `PWD=/bogus; print -r -- "$PWD"`); out != "/bogus\n" || st != 0 {
		t.Errorf(`$PWD after the assignment = %q (status %d), want "/bogus\n"`, out, st)
	}
}

// `%~` abbreviates against the named directories `hash -d` recorded, and the
// same shortening reaches `jobs -d` — #4541.
//
// Measured on zsh 5.9.2, 2026-09-26, run `-f`. The rows without a `hash -d`
// are the controls: the same lines draw the path, so the probe can produce
// both answers.
func TestANamedDirectoryShortensAPrompt(t *testing.T) {
	dir, _, _ := jobsDirTree(t)
	below := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(below, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, src, want string }{
		{"the directory itself", `hash -d jd=` + dir + `; print -rP -- '%~'`, "~jd"},
		{"below it", `hash -d jd=` + dir + `; cd ` + below + `; print -rP -- '%~'`, "~jd/a/b"},
		{"the last component of it", `hash -d jd=` + dir + `; print -rP -- '%c'`, "~jd"},
		{"a count reads the marker as one unit", `hash -d jd=` + dir + `; cd ` + below + `; print -rP -- '%-1~'`, "~jd"},
		// The controls, which are the same lines with the table left empty:
		// both draw the path, so the probe can produce either answer.
		{"nothing named", `print -rP -- '%~'`, dir},
		{"nothing over this one", `hash -d jd=` + below + `; print -rP -- '%~'`, dir},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want+"\n" || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want+"\n")
			}
		})
	}
	// The listing shares the shortening with the prompt rather than writing
	// its own, which is the surface this was found on (#4507).
	out, st := runZsh(t, dir, `hash -d jd=`+dir+"\n/bin/sleep 0.3 &\njobs -d\nwait")
	if st != 0 {
		t.Fatalf("jobs -d: %q (status %d)", out, st)
	}
	if got := pwdLines(out); len(got) != 1 || got[0] != "~jd" {
		t.Errorf("jobs -d wrote %q, want the directory as ~jd —\n%s", got, out)
	}
}
