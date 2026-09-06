// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A function definition the grammar refuses keeps its node. The name and the
// parentheses were read, the error says what was wrong, and the node is what
// the parser has already handed to whatever asked for a command — so a
// [syntax.FuncDecl] with no body is a normal product of a failed parse.
//
// Two things then measure that node without knowing it, and both used to
// dereference the body it does not have (#911). The tests below are one
// input each, under the flag vector that reaches it, asserting the whole
// rendered diagnostic: the failure being fixed is a *panic where a syntax
// error was promised*, so the assertion has to be that the error arrived,
// intact, with its position on it.
func mustBeSyntaxError(t *testing.T, src string, d syntax.Dialect, want string) {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err == nil {
		t.Fatalf("%q parsed, want %q", src, want)
	}
	if got := err.Error(); got != want {
		t.Errorf("%q:\n got %q\nwant %q", src, got, want)
	}
	// A *syntax.Error and not a bare formatted one: the position is the half
	// of the promise a caller cannot reconstruct, and a plain error carries
	// none.
	if _, ok := err.(*syntax.Error); !ok {
		t.Errorf("%q: error is %T, want *syntax.Error", src, err)
	}
	_ = f
}

// TestACoprocMeasuresNoRefusedDefinition covers the site #911 was reported
// from: with the `coproc` word and no name after it, the word `MY` is the
// start of the command, `MY (` is a definition to a grammar that commits at
// the parenthesis, and the coproc then takes its own end from the command it
// read.
//
// No preset has this pair — bash sets both coproc flags and zsh sets neither
// — which is why it survived. The dialect is built here by hand for that
// reason rather than borrowed from a shell.
func TestACoprocMeasuresNoRefusedDefinition(t *testing.T) {
	d := syntax.Core()
	d.Coproc = true
	d.FuncDefAtParen = true
	mustBeSyntaxError(t, `coproc MY ( cat </dev/null ); echo "n=${#MY[@]}"`, d, `1:13: "cat" unexpected`)
	mustBeSyntaxError(t, `coproc MY ( cat )`, d, `1:13: "cat" unexpected`)
}

// TestAShortFormBodyMeasuresNoRefusedDefinition covers the second site, which
// the issue did not name and which a hand sweep of one flag at a time cannot
// reach: a short-form body takes the end of the statement it read, through
// [syntax.Pipeline] and [syntax.Stmt], and lands on the same missing body.
//
// It needs the same `FuncDefAtParen` and `ShortForm` instead of `Coproc`, and
// no preset has that pair either — the flag belongs to one shell and the
// other two shells that commit at the parenthesis do not have it. Both bodies
// a short form may open are covered, because the extent is taken in one place
// for all of them and a test of one would not say so.
func TestAShortFormBodyMeasuresNoRefusedDefinition(t *testing.T) {
	d := syntax.Core()
	d.ShortForm = true
	d.FuncDefAtParen = true
	mustBeSyntaxError(t, `if [[ -n x ]] echo( A; echo after`, d, `1:21: "A" unexpected`)
	mustBeSyntaxError(t, `while (( i )) echo( A`, d, `1:21: "A" unexpected`)
}

// TestARefusedDefinitionHasAnEmptyExtent is the invariant the fix rests on,
// asserted on the node rather than through a caller of it. Pos is never past
// End, which is what the two measurements above were relying on without
// saying so.
func TestARefusedDefinitionHasAnEmptyExtent(t *testing.T) {
	fn := &syntax.FuncDecl{Name: "f", Start: syntax.Pos{Offset: 3, Line: 2, Col: 4}}
	if got, want := fn.End(), fn.Pos(); got != want {
		t.Errorf("End() = %+v, want %+v", got, want)
	}
}
