// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// This shell ships four preset aliases it will not report, and they are
// exactly its **declaration words**. Everything else about them is untouched:
// `alias` lists them, the word expands, and `unalias` removes them.
//
// Measured 2026-09-18 on ksh93u+ 2012-08-01, a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with standard input on /dev/null (#3642).
func TestTheDeclarationAliasesAreNotReported(t *testing.T) {
	for _, name := range []string{"compound", "float", "integer", "nameref"} {
		for _, form := range []string{
			"whence " + name,
			"whence -v " + name,
			"whence -a " + name,
			"command -v " + name,
			"command -V " + name,
			"type " + name,
		} {
			out, st := runKshWithPrelude(t, form+"\n")
			if st != 1 {
				t.Errorf("%s: status %d, want 1 (%q)", form, st, out)
			}
			if strings.Contains(out, "is an alias for") || strings.Contains(out, "typeset") {
				t.Errorf("%s: said %q, want it unreported", form, out)
			}
		}
		// `-q` is silent either way, so the status is the whole answer.
		if _, st := runKshWithPrelude(t, "whence -q "+name+"\n"); st != 1 {
			t.Errorf("whence -q %s: status %d, want 1", name, st)
		}
	}
	// And every other preset alias is reported in full, which is the control
	// that keeps this from reading as "`whence` lost its alias route".
	for _, name := range []string{"autoload", "source", "times", "hash", "functions"} {
		out, st := runKshWithPrelude(t, "whence -v "+name+"\n")
		if st != 0 || !strings.Contains(out, "is an alias for") {
			t.Errorf("whence -v %s: %q at %d, want the alias sentence at 0", name, out, st)
		}
	}
}

// The alias itself is exactly as it was, which is what makes the mark a
// *report* and not a removal. The expansion half is asserted through the
// alias-aware route below, because a snippet parsed before the prelude has
// run never sees the table — see dialecttest.Preset.CombinedThroughTheAliases.
func TestADeclarationAliasStillExpandsAndStillLists(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the listing holds it", "alias integer\n", "integer='typeset -li'"},
		{"and the whole listing does", "alias\n", "nameref='typeset -n'"},
	} {
		out, st := runKshWithPrelude(t, tc.src)
		if st != 0 || !strings.Contains(out, tc.want) {
			t.Errorf("%s: %q at %d, want %q", tc.name, out, st, tc.want)
		}
	}
}

// The mark is on the name and not on the value: a new value keeps it and a
// removal takes it off. Measured the same day — `alias integer='echo hi';
// whence -v integer` is still `not found` at 1, while `unalias float; alias
// float=ls; whence -v float` is `float is an alias for ls` at 0.
func TestTheMarkSurvivesAReassignmentAndDiesOnAnUnalias(t *testing.T) {
	out, st := runKshWithPrelude(t, "alias integer='echo hi'\nwhence -v integer\n")
	if st != 1 || !strings.Contains(out, "whence: integer: not found") {
		t.Errorf("after a reassignment: %q at %d, want the refusal at 1", out, st)
	}
	out, st = runKshWithPrelude(t, "unalias float\nalias float=ls\nwhence -v float\n")
	if st != 0 || !strings.Contains(out, "float is an alias for ls") {
		t.Errorf("after an unalias: %q at %d, want the sentence at 0", out, st)
	}
}

// The name a `whence` sentence says back is written shell-quoted where it is
// not a word this shell could write bare — the same spelling its own trace
// uses, measured character for character (#3666).
func TestWhenceQuotesAnOperandThatIsNotAPlainName(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`whence -v nosuchcmd`, "whence: nosuchcmd: not found"},
		{`whence -v "a b"`, "whence: 'a b': not found"},
		{`whence -v "]]"`, "whence: ']]': not found"},
		{`whence -v "x]"`, "whence: 'x]': not found"},
		{`whence -v "=ab"`, "whence: '=ab': not found"},
		// The position rule carries over: a trailing `=` is bare.
		{`whence -v "ab="`, "whence: ab=: not found"},
		{`whence -pv "a b"`, "whence: 'a b': not found"},
		{`whence -av "a b"`, "whence: 'a b': not found"},
		{`command -V "a b"`, "command: 'a b': not found"},
		// And the 127 a command word gets is *not* this sentence: that one
		// is written bare in the same shell.
		{`"a b"`, ": a b: not found"},
	} {
		out, _ := runKshWithPrelude(t, tc.src+"\n")
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: said %q, want %q", tc.src, out, tc.want)
		}
	}
}

// And the word still expands, which is the half a snippet parsed before the
// prelude has run cannot see: `integer zz=3` declares exactly as it did.
func TestADeclarationAliasStillExpands(t *testing.T) {
	out, st, err := preset.CombinedThroughTheAliases(t, dialecttest.Base{Dir: t.TempDir()},
		"integer zz=3\necho \"[$zz]\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if st != 0 || !strings.Contains(out, "[3]") {
		t.Errorf("integer zz=3: %q at %d, want [3] at 0", out, st)
	}
}
