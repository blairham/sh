// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// An arithmetic command standing where the grammar has no command is quoted
// back two ways, and neither of them is a description of the construct.
//
// Measured 2026-09-11 on a script holding `x=1` and `(( 1 )) (( 2 ))`: bash
// 5.3 and zsh name the expression it held with its blanks, ` 2 `, and ksh93
// names the `((` that opened it. Ours named it `arithmetic command`, which is
// [syntax.Kind.String]'s prose standing where a diagnostic quotes what it read
// — a sentence nobody in the panel writes (#2013).

// arithCmdDialect is the grammar this needs: `(( ))` as a command.
func arithCmdDialect(d *syntax.Dialect) { d.ArithCommand = true }

func unexpectedIn(t *testing.T, src string, d Diagnostics) string {
	t.Helper()
	dial := syntax.Core()
	arithCmdDialect(&dial)
	_, err := syntax.Parse(src, dial)
	if err == nil {
		t.Fatalf("%s: parsed cleanly, want a refusal", src)
	}
	return d.ParseFailure(err)
}

func TestAnUnexpectedArithmeticCommandIsNamedByItsText(t *testing.T) {
	d := Diagnostics{SyntaxUnexpected: "near `%[1]s'"}
	got := unexpectedIn(t, "x=1\n(( 1 )) (( 2 ))", d)
	if !strings.Contains(got, "near ` 2 '") {
		t.Errorf("= %q, want the expression with its blanks", got)
	}
	if strings.Contains(got, "arithmetic command") {
		t.Errorf("= %q, want no description of the construct", got)
	}
}

// The other value of the same question, which is what makes it the dialect's
// answer rather than one spelling for everyone.
func TestADialectMayNameTheOperatorThatOpenedIt(t *testing.T) {
	d := Diagnostics{SyntaxUnexpected: "near `%[1]s'", SyntaxUnexpectedNamesTheOpener: true}
	got := unexpectedIn(t, "x=1\n(( 1 )) (( 2 ))", d)
	if !strings.Contains(got, "near `(('") {
		t.Errorf("= %q, want the opening operator", got)
	}
}

// And the flag reaches only the construct that has two spellings: an ordinary
// token has no opener, so asking for one changes nothing.
func TestNamingTheOpenerLeavesEveryOtherTokenAlone(t *testing.T) {
	plain := Diagnostics{SyntaxUnexpected: "near `%[1]s'"}
	opener := plain
	opener.SyntaxUnexpectedNamesTheOpener = true
	src := "case x in\n;;\nesac"
	if a, b := unexpectedIn(t, src, plain), unexpectedIn(t, src, opener); a != b {
		t.Errorf("= %q with the flag and %q without it, want no difference", b, a)
	}
}
