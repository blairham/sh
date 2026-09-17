// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// The run-time half of syntax.Dialect.DoubleBracketIsACommand: `[[` as a
// builtin that reads `test`'s language with `&&` and `||` for connectives,
// and a pattern on the right of `=`, `==` and `!=` whatever it was quoted
// with.

func doubleBracketCommand(d *syntax.Dialect) {
	d.DoubleBracket = false
	d.DoubleBracketIsACommand = true
}

// statuses runs each line of src and reports the status each one left, so a
// refusal on one line does not hide the answers of the rest.
func statuses(t *testing.T, enable func(*syntax.Dialect), lines ...string) string {
	t.Helper()
	var b strings.Builder
	for _, l := range lines {
		b.WriteString("(" + l + ") 2>/dev/null; printf '%s ' $?\n")
	}
	out, _ := runGrammar(t, b.String(), enable, nil)
	return strings.TrimSpace(out)
}

func TestACommandBracketMatchesAQuotedPattern(t *testing.T) {
	t.Parallel()
	got := statuses(t, doubleBracketCommand,
		`[[ abc == "a*" ]]`,
		`[[ abc = 'a*' ]]`,
		`[[ abc != "a*" ]]`,
		`[[ "a*" == abc ]]`,
		// The control: `[` in the same runner compares strings.
		`[ abc = "a*" ]`,
	)
	if want := "0 0 1 1 1"; got != want {
		t.Errorf("statuses %q, want %q", got, want)
	}
	// And under the keyword the quotes protect the pattern, which is what
	// says the rows above are about the flag.
	if got := statuses(t, nil, `[[ abc == "a*" ]]`); got != "1" {
		t.Errorf("keyword reading: status %q, want 1", got)
	}
}

func TestACommandBracketSplitsItsOperands(t *testing.T) {
	t.Parallel()
	got := statuses(t, doubleBracketCommand,
		`s="two words"; [[ $s == "two words" ]]`,
		`e=; [[ $e == "" ]]`,
		`[[ ]]`,
		`[[ 1+2 -eq 3 ]]`,
		`[[ 010 -eq 8 ]]`,
	)
	if want := "2 2 1 2 1"; got != want {
		t.Errorf("statuses %q, want %q", got, want)
	}
}

func TestACommandBracketsConnectivesAreItsOwn(t *testing.T) {
	t.Parallel()
	got := statuses(t, doubleBracketCommand,
		`[[ -n x && -z "" ]]`,
		`[[ -z x || -z y ]]`,
		`[[ a && "" ]]`,
		`[[ a = a || a = b && a = b ]]`,
		// Quoting the connective changes nothing: the builtin sees a word.
		`[[ a = a '&&' a = b ]]`,
		// And test's own spellings are not connectives here.
		`[[ a = a -a b = b ]]`,
		`[[ a = b -o a = a ]]`,
		`[ a = a -a b = b ]`,
		`[[ a = a || ]]`,
	)
	if want := "0 1 1 0 1 2 2 0 2"; got != want {
		t.Errorf("statuses %q, want %q", got, want)
	}
}

func TestACommandBracketMatchesARegex(t *testing.T) {
	t.Parallel()
	got := statuses(t, doubleBracketCommand,
		`[[ abc =~ a.c ]]`,
		`[[ abc =~ "^b" ]]`,
		`[[ a.c =~ "a.c" && x ]]`,
		`[ abc =~ a.c ]`,
	)
	if want := "0 1 0 2"; got != want {
		t.Errorf("statuses %q, want %q", got, want)
	}
	// A regex that will not compile is a status and no sentence.
	out, _ := runGrammar(t, `[[ abc =~ "(b" ]]; echo "st=$?"`, doubleBracketCommand, nil)
	if out != "st=2\n" {
		t.Errorf("an uncompilable regex: output %q, want only the status", out)
	}
}

func TestACommandBracketIsACommandOnlyUnderTheFlag(t *testing.T) {
	t.Parallel()
	src := `v='[['; $v -n x ]] 2>/dev/null; echo "run=$?"; command -v '[['; ` +
		`[[ a = a; echo "unclosed=$?"; [[ a ]] ]] 2>/dev/null; echo "late=$?"; ` +
		`[[ a "]]"; echo "quoted=$?"`
	out, _ := runGrammar(t, src, doubleBracketCommand, nil)
	for _, want := range []string{"run=0", "\n[[\n", "unclosed=2", "late=2", "quoted=0"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q lacks %q", out, want)
		}
	}
	// A keyword names no command, so running the word finds nothing.
	out, _ = runGrammar(t, `v='[['; $v -n x ]] 2>/dev/null; echo "run=$?"`, nil, nil)
	if !strings.Contains(out, "run=127") {
		t.Errorf("keyword reading: output %q, want run=127", out)
	}
}
