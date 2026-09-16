// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/internal/testenv"
)

// writeEchoOnPath is a directory holding an executable called `echo`, so that
// the file row of a listing is one this test made rather than whatever the
// machine has.
func writeEchoOnPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := testenv.WriteExecutable(filepath.Join(dir, "echo"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// `type -a` and the two letters that compose with it.
//
// The listing letter had no alias row at all and no path row: it walked the
// function, builtin, keyword and PATH tables and nothing else, so a name that
// was only an alias came back `not found` under `-a` one line after plain
// `type` had named it, and `-ap` printed the whole listing where three shells
// print paths. Measured 2026-09-16 on bash 5.3.20, zsh 5.9.2 and ksh93u+.

// TestTypeAllNamesAnAliasFirst: the alias row leads the listing and the walk
// carries on past it, in every column that has the letter.
//
//	                     bash 5.3.20                zsh 5.9.2
//	type -a nope         nope is aliased to `true'  nope is an alias for true
//	type -a echo         the alias, the builtin and the file, in that order
func TestTypeAllNamesAnAliasFirst(t *testing.T) {
	for _, c := range []struct {
		dialect, alone string
	}{
		{"bash", "nope is aliased to `true'\n"},
		{"zsh", "nope is an alias for true\n"},
	} {
		t.Run(c.dialect, func(t *testing.T) {
			p := presets[c.dialect]
			const defs = "shopt -s expand_aliases 2>/dev/null; alias nope='true'; alias echo='echo -n'; "
			out, _, err := p.Combined(t, dialecttest.Base{}, defs+"type -a nope")
			if err != nil {
				t.Fatal(err)
			}
			if out != c.alone {
				t.Errorf("type -a of an alias said %q, want %q", out, c.alone)
			}
			out, _, err = p.Combined(t, dialecttest.Base{}, defs+"type -a echo")
			if err != nil {
				t.Fatal(err)
			}
			first, _, _ := strings.Cut(out, "\n")
			if !strings.Contains(first, "alias") {
				t.Errorf("type -a echo began %q, want the alias row first", first)
			}
			if !strings.Contains(out, "builtin") {
				t.Errorf("type -a echo said %q, want the builtin row after the alias", out)
			}
		})
	}
}

// TestTypeAllWithAPathLetterPrintsPaths: `-p` and `-P` narrow the listing to
// its file rows rather than being dropped.
//
// The status is the half worth having, because it is the axis rather than a
// new rule: where the shell's own answer counts for `-p`
// (TypePSearchesPathPastTheShell, bash), a name the shell can answer for is
// found even though this letter prints none of it; where it does not (zsh,
// ksh93), the same name is not found.
//
//	              bash 5.3.20      zsh 5.9.2
//	-ap f         silence, 0       `f not found`, 1
//	-ap echo      /bin/echo, 0     `echo is /bin/echo`, 0
func TestTypeAllWithAPathLetterPrintsPaths(t *testing.T) {
	for _, c := range []struct {
		dialect, echoRow, funcRow string
	}{
		{"bash", "", "f=0\n"},
		{"zsh", "echo is ", "f=1\n"},
	} {
		t.Run(c.dialect, func(t *testing.T) {
			p := presets[c.dialect]
			dir := writeEchoOnPath(t)
			base := dialecttest.Base{Dir: dir, Env: []string{"PATH=" + dir}}
			out, _, err := p.Combined(t, base, "type -ap echo")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(out, "/echo\n") || !strings.HasPrefix(out, c.echoRow) ||
				strings.Contains(out, "builtin") {
				t.Errorf("type -ap echo said %q, want the path row alone, opening %q", out, c.echoRow)
			}
			out, _, err = p.Combined(t, base, "f() { :; }\ntype -ap f >/dev/null 2>&1; echo f=$?")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(out, c.funcRow) {
				t.Errorf("type -ap of a function answered %q, want %q", out, c.funcRow)
			}
		})
	}
}

// TestTypePathIsSilentForAnAlias: the same table the listing was missing, in
// the plain letter. bash's `-p` speaks only where the plain answer would have
// been a file, and an alias is not one — `type -p ls` with `alias ls=...` set
// is silence and 0 there, and named the file here.
func TestTypePathIsSilentForAnAlias(t *testing.T) {
	p := presets["bash"]
	dir := writeEchoOnPath(t)
	if err := testenv.WriteExecutable(filepath.Join(dir, "tool"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	base := dialecttest.Base{Dir: dir, Env: []string{"PATH=" + dir}}
	// Three rows, because no two of them alone say it. The control names the
	// file, so the two below are the alias table being consulted rather than
	// `-p` having gone quiet; the shadowed name separates the *output*, and
	// the name that is only an alias separates the *status* — 1 without the
	// table, where bash answers 0.
	for _, tc := range []struct{ src, want string }{
		{"type -p tool; echo st=$?", "/tool\nst=0\n"},
		{"alias tool='true'\ntype -p tool; echo st=$?", "st=0\n"},
		{"alias only='true'\ntype -p only; echo st=$?", "st=0\n"},
	} {
		out, _, err := p.Combined(t, base, "shopt -s expand_aliases\n"+tc.src)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(out, tc.want) {
			t.Errorf("%q said %q, want it to end %q", tc.src, out, tc.want)
		}
	}
}
