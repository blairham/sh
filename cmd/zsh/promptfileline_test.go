// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTheFileLineEscapeCountsInTheFile is `%I`: the line being read, counted
// in the file `%x` names rather than from the start of the unit `%i` counts
// in. Every row measured 2026-10-01 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`
// over a script file); the trace row is the shape zsh's own E02xtrace asks
// about (#5156).
func TestTheFileLineEscapeCountsInTheFile(t *testing.T) {
	rows := []struct{ name, src, want string }{
		{"top level and a function body", "print -P A:%I\n\nfn() {\n  print -P B:%I\n}\nfn\n", "A:1\nB:4\n"},
		{"a function written on one line", ":\nfn() { print -P cf:%I; }; fn\n", "cf:2\n"},
		{"a subshell and a substitution", "(print -P C:%I)\nx=$(print -P D:%I)\nprint $x\n", "C:1\nD:2\n"},
		{"an anonymous function", ":\n() { print -P anon:%I }\n", "anon:2\n"},
		{"eval at the top", ":\n:\neval \"print -P ev:%I\nprint -P ev2:%I\"\n", "ev:4\nev2:5\n"},
		{"eval inside a function", "fn() {\n  :\n  eval \"print -P E:%I\"\n}\nfn\n", "E:4\n"},
		{
			"eval inside eval",
			":\n:\neval \"print -P a:%I\n  eval \\\"print -P b:%I\\\"\n  eval \\\":\nprint -P c:%I\\\"\"\n",
			"a:4\nb:5\nc:7\n",
		},
		{"a function called from eval", "f() {\n  print -P f:%I\n}\neval \":\nf\"\n", "f:2\n"},
		{"eval in a function called from eval", "f() {\n  eval \"print -P g:%I\"\n}\n:\neval \":\nf\"\n", "g:3\n"},
		{
			"the trace prefix",
			"PS4=\"+%x:%I> \"\nfn() {\n  print This is fn.\n}\n:\nfn\n",
			"This is fn.\n",
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			dir := t.TempDir()
			script := filepath.Join(dir, "fnfile")
			if err := os.WriteFile(script, []byte(row.src), 0o600); err != nil {
				t.Fatal(err)
			}
			out, errs, code := runZsh(t, "-f", script)
			if code != 0 || out != row.want {
				t.Errorf("status %d, out %q, stderr %q; want out %q", code, out, errs, row.want)
			}
		})
	}
}

// TestTheFileLineEscapeTracesTheDefiningLine is the trace half: with
// `PS4='+%x:%I> '` a function's body line is traced at its line in the file.
// Measured as the row above.
func TestTheFileLineEscapeTracesTheDefiningLine(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fnfile")
	src := "PS4=\"+%x:%I> \"\nfn() {\n  print This is fn.\n}\n:\nfn\n"
	if err := os.WriteFile(script, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	_, errs, code := runZsh(t, "-fx", script)
	want := "+" + script + ":1> PS4='+%x:%I> ' \n" +
		"+" + script + ":5> :\n" +
		"+" + script + ":6> fn\n" +
		"+" + script + ":3> print This is fn.\n"
	if code != 0 || errs != want {
		t.Errorf("status %d, stderr\n%q\nwant\n%q", code, errs, want)
	}
}
