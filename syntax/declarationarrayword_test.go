// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// arrayOperandOf reports whether the only command of src took a `name=( … )`
// operand as an array literal, and whether it parsed at all.
func arrayOperandOf(t *testing.T, src string, d syntax.Dialect) (array, parsed bool) {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		return false, false
	}
	cmd, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd)
	if !ok {
		t.Fatalf("%q is not a simple command", src)
	}
	for _, a := range cmd.Assigns {
		if a.Operand && a.IsArray {
			return true, true
		}
	}
	return false, true
}

// A `name=( … )` word is an array literal where a *declaration utility's*
// operand stands and an ordinary word everywhere else, and nothing in the word
// says which — the command's name does.
//
// Measured 2026-09-15 on zsh 5.9.2, each probe in a script file of its own
// with `nonomatch` set: `print -r -- x=(a|b)c` is one word there, `print -r --
// x=(a)` is a word with a glob qualifier on it, and `local a=(x y)` and
// `typeset -a b=(p q r)` are arrays of two and three. The array reading used
// to be offered wherever an assignment could not stand, which was too wide by
// one position: an argument got it too, and both of the first two lines were
// `parse error near `('` (#3087).
func TestAnArrayOperandNeedsADeclarationUtilityInFrontOfIt(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	for _, tc := range []struct {
		name, src     string
		array, parsed bool
	}{
		{name: "a declaration's operand", src: "typeset a=(x y)", array: true, parsed: true},
		{name: "a second operand of the same command", src: "typeset a=(1) b=(2)", array: true, parsed: true},
		{name: "a prefix assignment in front of it", src: "x=1 typeset a=(y z)", array: true, parsed: true},

		// An ordinary command's argument is not one. With no pattern
		// alternation in the dialect there is nothing for the `(` to be, so
		// the word ends at it and the parenthesis is refused behind a word —
		// which is bash 5.3's answer to the same line.
		{name: "an ordinary command's argument", src: "echo x=(a|b)c", array: false, parsed: false},
		// And an argument of the declaration utility that is not an operand
		// at all keeps its own reading: the flag says the *position* may hold
		// an array, not that every word does.
		{name: "an operand with no parenthesis", src: "typeset a=1", array: false, parsed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			array, parsed := arrayOperandOf(t, tc.src, d)
			if parsed != tc.parsed {
				t.Fatalf("%q parsed = %v, want %v", tc.src, parsed, tc.parsed)
			}
			if array != tc.array {
				t.Errorf("%q read an array operand = %v, want %v", tc.src, array, tc.array)
			}
		})
	}

	// And where the dialect has a pattern group, the argument gets one: the
	// `(a|b)` is folded into the word rather than ending it, so
	// `print -r -- x=(a|b)c` is the single word zsh 5.9.2 prints back.
	alt := syntax.Core()
	alt.PatternAlternation = true
	f, err := syntax.Parse("print -r -- x=(a|b)c", alt)
	if err != nil {
		t.Fatalf("`print -r -- x=(a|b)c` with pattern alternation: %v", err)
	}
	cmd, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd)
	if !ok {
		t.Fatal("not a simple command")
	}
	if len(cmd.Assigns) != 0 {
		t.Errorf("read %d assignments, want none", len(cmd.Assigns))
	}
	if n := len(cmd.Args); n != 4 {
		t.Fatalf("read %d words, want 4", n)
	}
	if got := cmd.Args[3].Literal(); got != "x=(a|b)c" {
		t.Errorf("the last word is %q, want %q", got, "x=(a|b)c")
	}
}

// DeclarationArrayFromTheCommandWord decides how the command word has to be
// *written* for the operand behind it to be an array.
//
// The core wants one unquoted literal word, so `'typeset' a=(x y)` is not a
// declaration and the `(` behind `a=` is whatever the dialect makes of a
// parenthesis after a word. The other reading keeps it through quoting and
// loses it only to an expansion. Measured 2026-09-16 (#3351).
func TestTheDeclarationWordIsReadAsTheDialectSaysItIsWritten(t *testing.T) {
	t.Parallel()
	literal := syntax.Core()
	written := syntax.Core()
	written.DeclarationArrayFromTheCommandWord = syntax.DeclarationArrayFromAWrittenWord

	for _, tc := range []struct {
		name, src                string
		fromLiteral, fromWritten bool
	}{
		{"the bare word", "typeset a=(x y)", true, true},
		{"a single-quoted word", "'typeset' a=(x y)", false, true},
		{"a backslash in the word", "\\typeset a=(x y)", false, true},
		{"a partly quoted word", "type\"set\" a=(x y)", false, true},
		// An expansion takes the reading away under both, which is the row
		// that keeps the two readings from being one.
		{"an expanded word", "$cmd a=(x y)", false, false},
		// And a word that is no declaration utility never had it.
		{"a word that is not one", "print a=(x y)", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := arrayOperandOf(t, tc.src, literal); got != tc.fromLiteral {
				t.Errorf("%q from an unquoted literal word = %v, want %v", tc.src, got, tc.fromLiteral)
			}
			if got, _ := arrayOperandOf(t, tc.src, written); got != tc.fromWritten {
				t.Errorf("%q from a written word = %v, want %v", tc.src, got, tc.fromWritten)
			}
		})
	}
}

// An element of a declaration's array is still inside the declaration, and
// the flag is restored at the closing parenthesis rather than left on. Both
// halves matter: an element that opens another literal keeps the reading, and
// the argument *after* the array does not get it from having been beside one.
func TestAnArrayElementKeepsTheReadingAndTheFlagIsPutBack(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	for _, src := range []string{
		"typeset a=( [k]=v )",
		"typeset a=( x=y )",
		"typeset a=(1) b=(2)",
	} {
		if _, err := syntax.Parse(src, d); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
	// The next command starts over, so a word that looked like an operand
	// beside the declaration is an ordinary argument again.
	if _, err := syntax.Parse("typeset a=(1); echo x=(y)", d); err == nil {
		t.Error("`typeset a=(1); echo x=(y)` parsed, want the `(` refused behind a word")
	}
}
