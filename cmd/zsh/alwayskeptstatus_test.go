// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// What an `always` half reads, and what the construct ends on, after a failed
// expansion that kept `$?` — by every route, because the routes are where the
// shell used to decide early (#6621).
//
// Measured 2026-10-15 on zsh 5.9.2 under -f, through -c, from a script file and
// from standard input, which agree on every row. The 0 a declaration or a
// `case` keeps is turned into a failure only as the shell ends, so the half
// reads 0 and a bare `exit` in it exits 0; a kept 7 is what the construct ends
// on; and a loop the error leaves sets 1 over whatever was kept, so a half
// inside the loop reads the kept status and one around it reads 1.
func TestAnAlwaysHalfReadsTheStatusAFailureKept(t *testing.T) {
	rows := []struct {
		src, out string
		status   int
	}{
		{`{ true; export x=$((1/0)) } always { echo in=$? }`, "in=0\n", 1},
		{`{ (exit 7); export x=$((1/0)) } always { echo in=$? }`, "in=7\n", 7},
		{`{ true; local x=$((1/0)) } always { echo in=$? }`, "in=0\n", 1},
		{`{ (exit 7); case x in $((1/0))) ;; esac } always { echo in=$? }`, "in=0\n", 1},
		{`{ true; export x=$((1/0)) } always { exit }`, "", 0},
		{`{ true; export x=$((1/0)) } always { TRY_BLOCK_ERROR=0 }; echo after=$?`, "after=1\n", 0},
		{`{ (exit 7); export x=$((1/0)) } always { TRY_BLOCK_ERROR=0 }; echo after=$?`, "after=7\n", 0},
		{`{ { true; export x=$((1/0)) } always { echo a=$? } } always { echo b=$? }`, "a=0\nb=1\n", 1},
		{`f(){ { true; export x=$((1/0)) } always { echo in=$? }; }; f`, "in=0\n", 1},
		// The loop's 1 is set as the error leaves it, not at the failure.
		{`(exit 7); for i in 1; do { export x=$((1/0)) } always { echo in=$? }; done`, "in=7\n", 1},
		{`(exit 7); { for i in 1; do { export x=$((1/0)) } always { echo a=$? }; done } always { echo in=$? }`, "a=7\nin=1\n", 1},
		{`(exit 7); { while :; do export x=$((1/0)); done } always { echo in=$? }`, "in=1\n", 1},
		{`(exit 7); { repeat 1 export x=$((1/0)) } always { echo in=$? }`, "in=1\n", 1},
		{`for i in 1; do { (exit 6); print $((1/0)) } always { echo in=$? }; done`, "in=6\n", 1},
		{`for i in 1; do { (exit 6); print $((1/0)) } always { TRY_BLOCK_ERROR=0 }; echo s=$?; done`, "s=6\n", 0},
		{`for i in 1; do ( (exit 6); print $((1/0)) ); echo s=$?; done`, "s=6\n", 0},
		{`for i in 1; do x=$( (exit 6); print $((1/0)) ); echo s=$?; done`, "s=6\n", 0},
		{`( for i in 1; do (exit 6); print $((1/0)); done ); echo s=$?`, "s=1\n", 0},
		// Controls: a failure that sets its own status, and a construct with
		// no loop around it.
		{`{ true; echo $((1/0)) } always { echo in=$? }`, "in=1\n", 1},
		{`(exit 7); { if true; then export x=$((1/0)); fi } always { echo in=$? }`, "in=0\n", 1},
	}
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	for i, c := range rows {
		file := filepath.Join(dir, "s"+itoaRow(i)+".zsh")
		if err := os.WriteFile(file, []byte(c.src+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		routes := []struct {
			name string
			run  func() (string, string, int)
		}{
			{"-c", func() (string, string, int) { return runZsh(t, "-f", "-c", c.src) }},
			{"file", func() (string, string, int) { return runZsh(t, "-f", file) }},
			{"stdin", func() (string, string, int) { return prompt(t, c.src+"\n", "zsh", "-f") }},
		}
		for _, route := range routes {
			out, errs, status := route.run()
			if out != c.out || status != c.status {
				t.Errorf("%s by %s\n got %q %d\nwant %q %d\n(stderr %q)", c.src, route.name, out, status, c.out, c.status, errs)
			}
		}
	}
}

// What a `${x?word}` failing in an `always` construct's try half does at a
// prompt, where it does not end the shell and is an error condition like any
// other: the half runs, can clear it, and reads the status the failure left
// (#6644).
//
// Measured 2026-10-15 on zsh 5.9.2 run `-f -i`, through a terminal and from
// standard input, one row a line with `echo st=$?` after it. Every one of these
// skipped the half here.
func TestAParamErrorInATryHalfRunsTheAlwaysHalfAtAPrompt(t *testing.T) {
	rows := []struct{ before, line, want string }{
		{"true", "{ x=${q?b} } always { echo in=$? }", "in=5\nst=5\n"},
		{"true", "{ echo ${q?b} } always { echo in=$? }", "in=1\nst=1\n"},
		{"true", "{ : ${q?b}; echo no } always { echo in=$? }", "in=1\nst=1\n"},
		{"true", "f(){ { x=${q?b} } always { echo in=$? }; echo no; }; f", "in=5\nst=5\n"},
		{"true", "{ { x=${q?b} } always { echo in=$? } } always { echo out=$? }", "in=5\nout=5\nst=5\n"},
		{"true", "( { x=${q?b} } always { echo in=$? } ); echo s=$?", "in=5\ns=5\nst=0\n"},
		{"true", "eval '{ echo ${q?b} } always { echo in=$? }'; echo e=$?", "in=1\ne=1\nst=0\n"},
		{"true", "{ x=${q?b} } always { TRY_BLOCK_ERROR=0 }; echo r=$?", "r=5\nst=0\n"},
		{"true", `x=$( { echo ${q?b} } always { echo in=$? } ); echo "[$x]"`, "[in=1]\nst=0\n"},
		// A declaration or a `case` keeps the 0, which the construct ends on
		// as 5; a kept 7 stays.
		{"true", "{ export x=${q?b} } always { echo in=$? }", "in=0\nst=5\n"},
		{"true", "{ typeset -i n=${q?b} } always { echo in=$? }", "in=0\nst=5\n"},
		{"true", "{ case ${q?b} in x) ;; esac } always { echo in=$? }", "in=0\nst=5\n"},
		{"true", "{ export x=${q?b} } always { TRY_BLOCK_ERROR=0 }; echo r=$?", "r=5\nst=0\n"},
		{"(exit 7)", "{ export x=${q?b} } always { echo in=$? }", "in=7\nst=7\n"},
		{"true", "{ export x=$((1/0)) } always { echo in=$? }", "in=0\nst=1\n"},
		// A loop sets 1 as the error leaves it.
		{"true", "for i in 1; do { x=${q?b} } always { echo in=$? }; done", "in=5\nst=1\n"},
		{"true", "for i in 1; do { x=${q?b} } always { TRY_BLOCK_ERROR=0 }; echo s=$?; done", "s=5\nst=0\n"},
		{"(exit 7)", "for i in 1; do { export x=$((1/0)) } always { echo in=$? }; done", "in=7\nst=1\n"},
		{"true", "for i in 1; do ( (exit 7); export x=$((1/0)) ); echo s=$?; done", "s=7\nst=0\n"},
	}
	var typed, want strings.Builder
	for _, c := range rows {
		typed.WriteString(c.before + "\n" + c.line + "\necho st=$?\n")
		want.WriteString(c.want)
	}
	t.Setenv("HOME", t.TempDir())
	out, errs, _ := prompt(t, typed.String(), "zsh", "-f", "-i")
	if out != want.String() {
		t.Errorf("stdout\n%s\nwant\n%s\n(stderr %q)", out, want.String(), errs)
	}

	// An `exit` in the half ends the shell, on the status the half read.
	out, _, code := prompt(t, "{ x=${q?b} } always { exit }\necho alive\n", "zsh", "-f", "-i")
	if out != "" || code != 5 {
		t.Errorf("exit in the half: got %q %d, want \"\" 5", out, code)
	}
}

// The same construct as the first line of an interactive shell's program:
// from a script the error costs that line and the next one runs, and under -c
// the shell ends on it, both after the half has run. Measured 2026-10-15 on
// zsh 5.9.2 run `-f -i` with standard input on /dev/null (#6644).
func TestAParamErrorInATryHalfRunsTheAlwaysHalfInAnInteractiveProgram(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	script := filepath.Join(t.TempDir(), "k.zsh")
	src := "{ x=${q?b} } always { echo in=$? }; echo same\necho after=$?\n"
	if err := os.WriteFile(script, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errs, code := prompt(t, "", "zsh", "-f", "-i", script)
	if out != "in=5\nafter=5\n" || code != 0 {
		t.Errorf("script: got %q %d, want %q 0 (stderr %q)", out, code, "in=5\nafter=5\n", errs)
	}
	out, errs, code = prompt(t, "", "zsh", "-f", "-i", "-c", "{ x=${q?b} } always { echo in=$? }; echo after")
	if out != "in=5\n" || code != 5 {
		t.Errorf("-c: got %q %d, want %q 5 (stderr %q)", out, code, "in=5\n", errs)
	}
}
