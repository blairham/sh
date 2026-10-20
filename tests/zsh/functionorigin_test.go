// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// **`whence -v` and `type` name the file a function was defined in** (#1706).
//
// The sentence was the fixed string `%s is a shell function from zsh`, which
// is the answer for a function the shell itself defined given to all of them.
// Nothing noticed because every test that defines its function in the snippet
// it runs is on exactly that row.
//
// Measured 2026-09-12 on zsh 5.9.2. Four origins and three shapes:
//
//	autoload -Uz qf; qf; whence -v qf   qf is a shell function from /…/qf
//	source lib.zsh; whence -v sf        sf is a shell function from /…/lib.zsh
//	g(){ :; }; whence -v g              g is a shell function from zsh
//	(the same, with the program on stdin)  g is a shell function
//
// The third is the shell's own *name in a diagnostic* rather than `$0`:
// measured through a symlink, `./xyzzy -c 'g(){ :; }; whence -v g'` still says
// `from zsh`. The fourth has no origin at all and drops the clause.

func TestAFunctionDefinedInThisCommandStringComesFromTheShell(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`g(){ :; }; whence -v g`, "g is a shell function from zsh\n"},
		{`g(){ :; }; type g`, "g is a shell function from zsh\n"},
		// `eval` at the top level is still the shell defining it.
		{`eval 'e(){ :; }'; whence -v e`, "e is a shell function from zsh\n"},
		// And a function defined by *calling* one the command string
		// defined: the origin follows the file the definition was read in,
		// which here is none.
		{
			`outer(){ inner(){ :; }; }; outer; whence -v inner`,
			"inner is a shell function from zsh\n",
		},
	} {
		out, st := runZsh(t, t.TempDir(), c.src)
		if out != c.want || st != 0 {
			t.Errorf("%s: out %q status %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// A sourced file is the origin for what it defines, and the *script* is not:
// the two rows together are what say the origin follows the definition rather
// than the run.
func TestAFunctionDefinedInASourcedFileNamesThatFile(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib.zsh")
	if err := os.WriteFile(lib, []byte("sf() { :; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A definition written by an `eval` *inside* the file is the file's too,
	// which is the row that says the origin is where the text was read and
	// not where the parser happened to be called from.
	lib2 := filepath.Join(dir, "lib2.zsh")
	if err := os.WriteFile(lib2, []byte("eval 'ef() { :; }'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st := runZsh(t, dir, `source `+lib+`; source `+lib2+`; own(){ :; }
whence -v sf
whence -v ef
whence -v own`)
	want := "sf is a shell function from " + lib + "\n" +
		"ef is a shell function from " + lib2 + "\n" +
		"own is a shell function from zsh\n"
	if out != want || st != 0 {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}

// An autoloaded function names the file it was read from, which is the origin
// the definition seam recorded nowhere before this: `autoload` defines through
// text, and text carries no file. A stub that has not been called yet is still
// the other sentence entirely.
func TestAnAutoloadedFunctionNamesItsFile(t *testing.T) {
	dir := t.TempDir()
	fns := filepath.Join(dir, "fns")
	if err := os.MkdirAll(fns, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(fns, "qf")
	if err := os.WriteFile(file, []byte("print -r -- ran\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st := runZsh(t, dir, `fpath=(`+fns+`); autoload -Uz qf
whence -v qf
qf
whence -v qf
type qf`)
	want := "qf is an autoload shell function\nran\n" +
		"qf is a shell function from " + file + "\n" +
		"qf is a shell function from " + file + "\n"
	if out != want || st != 0 {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}

// A script file is the origin for what it defines, named as the script was
// named rather than resolved.
func TestAFunctionDefinedInAScriptNamesTheScript(t *testing.T) {
	out := runZshScriptFile(t, "g(){ :; }\nwhence -v g\n", "s1.zsh")
	if want := "g is a shell function from s1.zsh\n"; out != want {
		t.Errorf("out %q, want %q", out, want)
	}
}

// **A program on standard input has no origin at all**, and the clause is
// absent rather than naming the shell — which is the row that makes the
// fallback a measurement instead of a guess.
func TestAFunctionDefinedOnStandardInputNamesNothing(t *testing.T) {
	f, err := syntax.Parse("g(){ :; }\nwhence -v g\n", zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	sem, dg, dl := zsh.Semantics(), zsh.Diagnostics(), zsh.Dialect()
	r := &interp.Runner{
		Semantics: &sem, Diagnostics: &dg,
		Stdout: &out, Stderr: &out,
		Name: "zsh", Dialect: &dl, Route: interp.RouteStandardInput,
	}
	zsh.Apply(r)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if want := "g is a shell function\n"; out.String() != want {
		t.Errorf("out %q, want %q", out.String(), want)
	}
}
