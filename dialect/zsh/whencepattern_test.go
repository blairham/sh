// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `whence -m` and `type -m`, the pattern lookup (#5230). Every row below was
// measured on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f` on a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, 2026-09-30) over a tree of the same
// shape, and is the line that shell wrote with the scratch directory's name
// swapped for this one's.

// patternTree is two PATH directories: p2 ahead of p1, a name in both, a
// directory and a file without the execute bit in p1, and a link to a file.
func patternTree(t *testing.T) (p1, p2 string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p1, p2 = filepath.Join(root, "p1"), filepath.Join(root, "p2")
	for _, dir := range []string{p1, p2, filepath.Join(p1, "qqdir")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(dir, name string, mode os.FileMode) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), mode); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"qqab", "qqbar", "qqbaz"} {
		write(p2, name, 0o700)
	}
	write(p1, "qqfoo", 0o700)
	write(p1, "qqbar", 0o700)
	write(p1, "qqnoexec", 0o600)
	if err := os.Symlink("qqfoo", filepath.Join(p1, "qqlink")); err != nil {
		t.Fatal(err)
	}
	return p1, p2
}

func runPattern(t *testing.T, p1, p2, src string) (string, int) {
	t.Helper()
	const setup = "alias qqbar=ls\nalias -g qqg=x\nqqbaz() { :; }\nqqa() { :; }\nfo() { :; }\n"
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir:  filepath.Dir(p1),
		Vars: map[string]string{"PATH": p2 + ":" + p1},
	}, setup+src+"\n")
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

func TestWhencePatternAnswersFromEveryTable(t *testing.T) {
	p1, p2 := patternTree(t)
	lines := func(ls ...string) string { return strings.Join(ls, "\n") + "\n" }
	for _, c := range []struct {
		name, src, want string
		st              int
	}{
		// The tables in turn — aliases (the global one with them, by name),
		// functions sorted rather than in definition order, then the command
		// table, which is the directories' listing: the directory and the
		// file that will not run are in it, and `qqbar` is p2's alone.
		{"every table", `whence -m 'qq*'`, lines(
			"ls", "x", "qqa", "qqbaz",
			p2+"/qqab", p2+"/qqbar", p2+"/qqbaz",
			p1+"/qqdir", p1+"/qqfoo", p1+"/qqlink", p1+"/qqnoexec"), 0},
		// `-a` turns the command rows into a search: both `qqbar`s.
		{"-a searches", `whence -am 'qqba?'`, lines(
			"ls", "qqbaz", p2+"/qqbar", p1+"/qqbar", p2+"/qqbaz"), 0},
		// `-p` keeps the command rows and nothing else.
		{"-p is the command table", `whence -pm 'qq*'`, lines(
			p2+"/qqab", p2+"/qqbar", p2+"/qqbaz",
			p1+"/qqdir", p1+"/qqfoo", p1+"/qqlink", p1+"/qqnoexec"), 0},
		// Reserved words ahead of functions.
		{"reserved words", `whence -m 'fo*'`, lines("for", "foreach", "fo"), 0},
		// The shapes reach every row.
		{"-w", `whence -wm 'qq[gf]*'`, lines("qqg: global alias", "qqfoo: command"), 0},
		{"type is whence -v", `type -m 'qqba?'`, lines(
			"qqbar is an alias for ls", "qqbaz is a shell function from zsh",
			"qqbar is "+p2+"/qqbar", "qqbaz is "+p2+"/qqbaz"), 0},
		{"type -f writes the body", `type -fm qqa`, "qqa () {\n\t:\n}\n", 0},
		// A pattern matching twice is answered twice.
		{"twice", `whence -m 'qqf*' 'qqfo*'`, lines(p1+"/qqfoo", p1+"/qqfoo"), 0},
		// A miss is silence in every shape, and the status counts operands
		// that matched — one is enough.
		{"a miss", `whence -m 'nos*'`, "", 1},
		{"a miss under -v", `whence -vm 'nos*'`, "", 1},
		{"a miss under -w", `whence -wm nosuch`, "", 1},
		{"one operand matched", `whence -vm 'nos*' qqfoo`, lines("qqfoo is " + p1 + "/qqfoo"), 0},
		// A table match is a match even where the search writes nothing.
		{"matched, nothing to run", `whence -am qqdir`, "", 0},
		// `hash` puts a name in the table and not on PATH.
		{"hashed", "hash qqman=/bin/ls\nwhence -m 'qqm*'", lines("/bin/ls"), 0},
		{"hashed, searched", "hash qqman=/bin/ls\nwhence -am 'qqm*'", "", 0},
		// The table's rows are written as it holds them; only the search
		// resolves a link.
		{"-s on the table", `whence -sm qqlink`, lines(p1 + "/qqlink"), 0},
		{"-s on the search", `whence -asm qqlink`, lines(p1 + "/qqlink -> " + p1 + "/qqfoo"), 0},
		// A disabled builtin is not in the table.
		{"disabled", "disable echo\nwhence -m 'ech?'", "", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runPattern(t, p1, p2, c.src)
			if out != c.want || st != c.st {
				t.Errorf("%s:\n got %q at %d\nwant %q at %d", c.src, out, st, c.want, c.st)
			}
		})
	}
}

// `-f` on a function is the body whatever shape was asked for, and only on a
// function: measured, `whence -vf qqa` writes the definition and `whence -vf
// echo` the builtin's sentence. The second row is the control that says the
// letter did not simply replace `-v`.
func TestWhenceFunctionLetterOutranksTheShape(t *testing.T) {
	p1, p2 := patternTree(t)
	for _, c := range []struct{ src, want string }{
		{"whence -vf qqa", "qqa () {\n\t:\n}\n"},
		{"whence -fv qqa", "qqa () {\n\t:\n}\n"},
		{"whence -wf qqa", "qqa () {\n\t:\n}\n"},
		{"whence -vf echo", "echo is a shell builtin\n"},
		{"whence -v qqa", "qqa is a shell function from zsh\n"},
	} {
		if out, st := runPattern(t, p1, p2, c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q at %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}
