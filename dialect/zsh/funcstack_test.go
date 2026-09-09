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

// show is the expression every case here prints: the whole stack as
// `[a][b]`, with the count beside it.
//
// It is `${#funcstack[@]}` and not `$#funcstack`, which is the shorter and
// more idiomatic spelling, because an unsubscripted read of a *dynamic* array
// answers as though the name were unset — #1600, which is core rather than
// this parameter and which `$FUNCNAME` has identically. When that is fixed
// the shorter spelling starts working here for free; asserting on it now
// would be asserting on somebody else's bug.
const show = `print -r -- "${#funcstack[@]} [${(j:][:)funcstack}]"`

// TestFuncstackNamesTheUnitsInnermostFirst is the ordering, and it is the
// whole of what a script reads this for. Every row was measured against
// zsh 5.9.2.
func TestFuncstackNamesTheUnitsInnermostFirst(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			name: "a nesting is innermost first",
			src:  "outer(){ inner; }; inner(){ " + show + " }; outer",
			want: "2 [inner][outer]\n",
		},
		{
			name: "one function is one frame",
			src:  "g(){ " + show + " }; g",
			want: "1 [g]\n",
		},
		{
			// Not "empty because nothing is implemented" — the row above
			// proves the parameter answers. This one says the top level of
			// `-c` has nothing to name, which is also zsh's answer.
			name: "nothing called is no frames",
			src:  show,
			want: "0 []\n",
		},
		{
			name: "three deep",
			src:  "a(){ b; }; b(){ c; }; c(){ " + show + " }; a",
			want: "3 [c][b][a]\n",
		},
		{
			// A function called from a *subshell* inside another function
			// still sees the frames it is under, because the stack is the
			// runner's and a subshell clones it.
			name: "a subshell keeps the frames it is under",
			src:  "f(){ ( g ); }; g(){ " + show + " }; f",
			want: "2 [g][f]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("funcstack = %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// TestFuncstackHasNoFrameForTheScriptItself is the one place this parameter
// is not bash's `FUNCNAME` under another name, and the reason a translation
// of that one cannot be used.
//
// bash ends `FUNCNAME` with `main`; zsh 5.9.2 has no entry for the script at
// all — `$#funcstack` is 0 at a script's top level and 1, not 2, inside a
// function the script calls. So the frame [interp.Runner.CallStack] appends
// for the script has to be dropped, and only a runner that has actually been
// given a script file can tell whether it was: under `-c` there is no such
// frame to drop, and every case above would pass with the rule missing.
func TestFuncstackHasNoFrameForTheScriptItself(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			name: "the top level of a script is no frames",
			src:  show,
			want: "0 []\n",
		},
		{
			name: "a function in a script is one frame",
			src:  "g(){ " + show + " }; g",
			want: "1 [g]\n",
		},
		{
			name: "a nesting in a script is two",
			src:  "f(){ g; }; g(){ " + show + " }; f",
			want: "2 [g][f]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runZshScriptFile(t, tc.src, "/s/main.zsh")
			if got != tc.want {
				t.Errorf("funcstack = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFuncstackNamesASourcedFileByThePathItWasFoundAt is the second field the
// walk has to choose between, and the two routes disagree only on one of
// them.
//
// `. ./lib.zsh` reports `./lib.zsh` — but the operand and the found path are
// the *same string* there, because a slash in the operand skips the PATH
// search, so that route cannot tell the two fields apart. The bare operand
// found on PATH is the one that discriminates: zsh 5.9.2 answers with the
// full path while the operand stays `lib.zsh`.
func TestFuncstackNamesASourcedFileByThePathItWasFoundAt(t *testing.T) {
	dir := t.TempDir()
	body := "print -r -- \"top ${#funcstack[@]} [${(j:][:)funcstack}]\"\n" +
		"sf(){ print -r -- \"fn  ${#funcstack[@]} [${(j:][:)funcstack}]\"; }\nsf\n"
	if err := os.WriteFile(filepath.Join(dir, "lib.zsh"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	found := filepath.Join(dir, "lib.zsh")

	t.Run("found on PATH, under the path and not the operand", func(t *testing.T) {
		out, st := runZsh(t, dir, ". lib.zsh")
		want := "top 1 [" + found + "]\nfn  2 [sf][" + found + "]\n"
		if out != want || st != 0 {
			t.Errorf("funcstack = %q (status %d), want %q", out, st, want)
		}
	})

	t.Run("a slashed operand is its own path", func(t *testing.T) {
		out, st := runZsh(t, dir, ". ./lib.zsh")
		want := "top 1 [./lib.zsh]\nfn  2 [sf][./lib.zsh]\n"
		if out != want || st != 0 {
			t.Errorf("funcstack = %q (status %d), want %q", out, st, want)
		}
	})
}

// runZshScriptFile runs src as though the shell had been given a script,
// which is the only way to put the frame this parameter must *not* report on
// the stack. See dialect/bash's runWithScriptFile, which is the same thing
// for the dialect that does report it.
func runZshScriptFile(t *testing.T, src, file string) string {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out bytes.Buffer
	sem, dg, dl := zsh.Semantics(), zsh.Diagnostics(), zsh.Dialect()
	r := &interp.Runner{
		Semantics: &sem, Diagnostics: &dg,
		Stdout: &out, Stderr: &out,
		Name: "zsh", Dialect: &dl,
	}
	zsh.Apply(r)
	r.SetScriptFile(file)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out.String()
}
