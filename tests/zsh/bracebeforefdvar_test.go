// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// Where a command begins, a `{` is the reserved word before it is anything
// else, so `{v}<f` is the group `{ v }` with its input from f and not a
// descriptor the shell picks for v (#6616). Measured 2026-10-15 on zsh 5.9.2
// at /opt/homebrew/bin/zsh, `-fc`, a script file and standard input; the table
// is on syntax.Dialect.OpenBraceNeedsNoBlank.
func TestABraceWhereACommandBeginsIsNotAnFdVariable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f1"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		src, want string
		notFound  int
	}{
		{"{v}<f1; echo v=$v", "v=", 1},
		{"true; {v}<f1; echo v=$v", "v=", 1},
		{"! {v}<f1; echo v=$v", "v=", 1},
		{"true | {v}<f1; echo v=$v", "v=", 1},
		{">/dev/null {v}<f1; echo v=$v", "v=", 1},
		{"if true; then {v}<f1; fi; echo v=$v", "v=", 1},
		{"set -- a b; for i {v}<f1; echo i=$i", "i=b", 2},
		{"for i in a b; {v}<f1; echo i=$i", "i=b", 2},
		{"repeat 2 {v}<f1; echo r", "r", 2},
		// The controls: an argument's place keeps the descriptor reading.
		{"exec {v}<f1; echo v=$v", "v=1", 0},
		{"true {v}<f1; echo v=$v", "v=1", 0},
	} {
		out, _ := runZsh(t, dir, c.src)
		if got := strings.Count(out, "command not found: v"); got != c.notFound {
			t.Errorf("%q: %d command-not-found lines, want %d\n%s", c.src, got, c.notFound, out)
		}
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if last := lines[len(lines)-1]; !strings.HasPrefix(last, c.want) ||
			(c.want == "v=" && last != "v=") {
			t.Errorf("%q: last line %q, want %q", c.src, last, c.want)
		}
	}
}

// The redirection behind `for i {v}` belongs to the loop, so a file that is
// not there is refused once and the loop does not run, whatever the
// positional parameters hold.
func TestALoopsBraceBodyRedirectionOpensOnce(t *testing.T) {
	dir := t.TempDir()
	for _, src := range []string{
		"for i {v}<nofile; echo i=$i",
		"set -- a b; for i {v}<nofile; echo i=$i",
	} {
		out, _ := runZsh(t, dir, src)
		if n := strings.Count(out, "no such file or directory: nofile"); n != 1 {
			t.Errorf("%q: refused %d times, want once\n%s", src, n, out)
		}
		if !strings.HasSuffix(out, "i=\n") {
			t.Errorf("%q: %q, want the loop variable unset", src, out)
		}
	}
}

// The parse errors the group reading makes: an assignment cannot stand in
// front of a group, and a word cannot stand behind one.
func TestABraceWhereACommandBeginsRefusesWhatAGroupWould(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"x=1 {v}<f1", "{"},
		{"{v}<f1 :", ":"},
		{"{v}<f1 {w}<f1", "{"},
		{"nocorrect {v}<f1", "{"},
		{"set -- a b; for i {v}<f1 print $i", "print"},
	} {
		_, err := syntax.Parse(c.src, zsh.Dialect())
		var se *syntax.Error
		if !errors.As(err, &se) {
			t.Errorf("%q: err %v, want a parse error", c.src, err)
			continue
		}
		if se.Token != c.want {
			t.Errorf("%q: blamed %q, want %q", c.src, se.Token, c.want)
		}
	}
}

// A loop's variable stands where the lexer splits a bare `{` off, and the
// refusal names the whole word the brace begins, as it does behind `fi`
// (#6615). Measured 2026-10-15 on zsh 5.9.2, `-fc`, a script file and
// standard input.
func TestALoopVariableInBracesIsNamedWhole(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"for {v} in a; do :; done", "{v}"},
		{"for {v}x in a; do :; done", "{v}x"},
		{"select {v} in a; do break; done", "{v}"},
		{"for {v}&>x", "{v}"},
		{"for {v}</dev/null", "{v}"},
		{"select {v}</dev/null", "{v}"},
		{"for {v}x</dev/null", "{v}x"},
		{"for {a,b} in a; do :; done", "{a,b}"},
		{"for {v", "{v"},
		// The control: a brace with a blank behind it is the brace alone.
		{"for { v } in a; do :; done", "{"},
	} {
		_, err := syntax.Parse(c.src, zsh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", c.src)
			continue
		}
		want := "parse error near `" + c.want + "'"
		if got := diagTable(zsh.Diagnostics()).ParseFailure(err); got != want {
			t.Errorf("%q:\n got %q\nwant %q", c.src, got, want)
		}
	}
}
