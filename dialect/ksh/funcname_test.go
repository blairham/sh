// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// A quoted name before `()` is a definition here, where this parser refused
// the line at the parenthesis.
//
// The quoting is the whole of it, and `'q'()` is the control that says so: no
// set of name characters can exclude `q`. This shell and zsh remove the
// quotes before reading the name, and the three bash columns take the word as
// its source text and refuse it — two against four, where a name holding a
// space splits the panel one against five (#1561).
func TestAQuotedNameBeforeTheParensIsADefinition(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`'q'() { echo q; }; q; echo "st=$?"`, "q\nst=0\n"},
		// Called back by the same quoting, which is the half a definition
		// row cannot show: a name that was accepted is a name that calls.
		{`'q'() { echo q; }; 'q'; echo "st=$?"`, "q\nst=0\n"},
		// Either quote, the removal being the same one a loop variable's
		// gets — see Dialect.ForNameMayBeQuoted.
		{`"q"() { echo q; }; q; echo "st=$?"`, "q\nst=0\n"},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s:\n got %q (status %d)\nwant %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// And a quoted word that is not a name once the quotes are off meets the
// check the definition already had — `PunctuatedFunctionNameIsRefused`, which
// this shell answers yes and which ends the script at 1. The name it quotes
// is the **expanded** one: `a\ b()` is `a b: invalid function name`, two
// words and no backslash, where bash names the source text.
//
// `'a;b'` is the row no character class could hold: a bare `;` would have
// ended the word before the parenthesis, so the quoting is what is being
// measured and not a wider set of name characters.
func TestAQuotedNameThatIsNotANameIsRefusedWhereItRuns(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`''() { echo e; }; echo after`, "sh: : invalid function name\n"},
		{`'a b'() { echo ab; }; echo after`, "sh: a b: invalid function name\n"},
		{`a\ b() { echo ab; }; echo after`, "sh: a b: invalid function name\n"},
		{`'a;b'() { echo one; }; echo after`, "sh: a;b: invalid function name\n"},
	} {
		out, st := answersRun(t, tc.src)
		if out != tc.want || st != 1 {
			t.Errorf("%s:\n got %q (status %d)\nwant %q at 1", tc.src, out, st, tc.want)
		}
	}
}

// A bare pattern character is still not a name, quoting being what decides
// it: `a*b()` never reaches the definition here and is a syntax error at the
// parenthesis, exactly as it is in ksh93.
func TestABarePatternBeforeTheParensIsStillASyntaxError(t *testing.T) {
	for _, src := range []string{
		`a*b() { echo s; }`,
		`a?b() { echo s; }`,
	} {
		if _, err := syntax.Parse(src+"\n", ksh.Dialect()); err == nil {
			t.Errorf("%s parsed, where ksh93 answers a syntax error at the paren", src)
		}
	}
	// Quoted, the same characters are ordinary text and the name is read.
	if _, err := syntax.Parse("'a*b'() { echo s; }\n", ksh.Dialect()); err != nil {
		t.Errorf("a quoted pattern refused while parsing: %v", err)
	}
}

// A `function` definition whose name holds an expansion parses here, the
// complaint coming when the definition is reached — and it ends the script,
// which is where this shell parts from bash. Measured 2026-09-10, `w=foo;
// function _p_${w} { echo HI; }; echo after` writes the one line and stops
// at 1, with `after` unreached (#1296).
func TestAFunctionNameHoldingAnExpansionEndsTheScript(t *testing.T) {
	const src = `w=foo; function _p_${w} { echo HI; }; echo after`
	if _, err := syntax.Parse(src+"\n", ksh.Dialect()); err != nil {
		t.Fatalf("refused while parsing: %v — this shell accepts it and `ksh -n` is silent", err)
	}
	out, st := answersRun(t, src)
	const want = "sh: _p_${w}: invalid function name\n"
	if out != want || st != 1 {
		t.Errorf("\n got %q (status %d)\nwant %q at 1", out, st, want)
	}
}

// The word is named **as written** here too, which is the half the sentence
// alone cannot show: the token's literal text is `_p_w`, a perfectly good
// name for a different function, and defining it at status 0 is the answer
// #1256 removed.
func TestTheRefusedFunctionNameIsNamedAsWrittenHere(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"_p_$w", "sh: _p_$w: invalid function name\n"},
		{"_p_${w}", "sh: _p_${w}: invalid function name\n"},
		{"_p_$@", "sh: _p_$@: invalid function name\n"},
	} {
		out, st := answersRun(t, "function "+tc.name+" { echo HI; }; echo after")
		if out != tc.want || st != 1 {
			t.Errorf("function %s:\n got %q (status %d)\nwant %q at 1", tc.name, out, st, tc.want)
		}
	}
}

// A redirection on the definition does not make it survivable, which is
// measured rather than assumed by symmetry with the loop's axis: the same
// shape on a `for` clause *is* survivable here, and this one is not.
func TestARedirectedDefinitionStillEndsTheScript(t *testing.T) {
	out, st := answersRun(t, "function _p_${w} { echo HI; } > mf; echo after")
	if want := "sh: _p_${w}: invalid function name\n"; out != want || st != 1 {
		t.Errorf("\n got %q (status %d)\nwant %q at 1", out, st, want)
	}
}

// The line the complaint names is the definition's own, which a one-line
// snippet cannot show — and it is the reason a script with several such
// definitions can be repaired at all.
func TestTheComplaintNamesTheDefinitionsLine(t *testing.T) {
	out, st := answersRun(t, "w=foo\nfunction _p_${w} { echo HI; }\necho after")
	if want := "sh: line 2: _p_${w}: invalid function name\n"; out != want || st != 1 {
		t.Errorf("\n got %q (status %d)\nwant %q at 1", out, st, want)
	}
}
