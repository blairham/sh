// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A modifier list written straight after an unbraced expansion, which is a
// question about where the **word** ends.
//
// The flag is named here and the shell that sets it is not: what belongs to a
// dialect lives under dialect/. See Dialect.BareParamModifiers for the
// measurement and dialect/zsh for what the letters then do.

// bareMods is the core plus the unbraced modifier list.
func bareMods() syntax.Dialect {
	d := syntax.Core()
	d.BareParamModifiers = true
	return d
}

// The whole of the flag: `$p:t` is one expansion where it is on, and an
// expansion followed by two characters of text where it is off.
//
// Checked on the spans and not on a result, because the boundary is the
// entire difference — off, the `:t` is text by the time anything expands it,
// so the two readings do not even fail the same way.
func TestABareModifierListIsPartOfTheExpansion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		src   string
		value string // the expansion's inner text, as `${ … }` would hold it
		after string // the literal text left over, "" for none
	}{
		{"one modifier", "echo $p:t", "p:t", ""},
		{"a chain", "echo $p:t:r", "p:t:r", ""},
		{"a quoting modifier", "echo $s:q", "s:q", ""},
		// The letter and nothing after it, which is where this differs from
		// the braced list: `${p:h2}` is the head twice and `$p:h2` is the
		// head once with a `2` beside it.
		{"no count follows the letter", "echo $p:h2", "p:h", "2"},
		{"nor anything else", "echo $s:qX", "s:q", "X"},
		// A substitution is the one segment that carries an operand, and it
		// is taken whole.
		{"a substitution", "echo $p:s/a/Z/", "p:s/a/Z/", ""},
		{"text after a substitution", "echo $p:s/a/Z/x", "p:s/a/Z/", "x"},
		{"a substitution with no closing delimiter", "echo $p:s/a/Z", "p:s/a/Z", ""},
		// The word still ends where it would have ended. A `;` after an
		// unterminated substitution is the operator it always was, which is
		// what keeps one from eating the rest of the line.
		{"an operator still ends the word", "echo $p:s/a/Z;", "p:s/a/Z", ""},
		// A colon that begins no modifier is given back, with no complaint.
		{"a letter that names nothing", "echo $s:zz", "s", ":zz"},
		{"a bare colon", "echo $s:", "s", ":"},
		{"a digit is not a modifier", "echo $s:2", "s", ":2"},
		// After a subscript, which is the expansion a completion writes it
		// after — and the two flags are separate, so the subscript one has
		// to be on for the brackets to be part of the expansion at all.
		{"after a subscript", "echo $b:q", "b:q", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spans := spansOf(t, tc.src, bareMods())
			if len(spans) == 0 || spans[0].Kind != syntax.ParamExp {
				t.Fatalf("%q: first span is not an expansion: %+v", tc.src, spans)
			}
			if spans[0].Value != tc.value {
				t.Errorf("%q: expansion is %q, want %q", tc.src, spans[0].Value, tc.value)
			}
			var rest string
			for _, s := range spans[1:] {
				rest += s.Value
			}
			if rest != tc.after {
				t.Errorf("%q: text after is %q, want %q", tc.src, rest, tc.after)
			}
		})
	}
}

// Without the flag the same words end at the name and the colon is text.
//
// The pair is the point, and here it is the stronger half: six of the seven
// shells in the panel have no such modifier, and a grammar that read one
// anyway would quietly change what `$x:$y` means in every portable script
// that writes a path list.
func TestWithoutTheFlagTheColonIsText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, value, after string }{
		{"echo $p:t", "p", ":t"},
		{"echo $s:q", "s", ":q"},
		{"echo $p:s/a/Z/", "p", ":s/a/Z/"},
		{"echo $x:$y", "x", ":"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			spans := spansOf(t, tc.src, syntax.Core())
			if len(spans) == 0 || spans[0].Value != tc.value {
				t.Fatalf("%q: expansion is %+v, want %q", tc.src, spans, tc.value)
			}
			var rest string
			for _, s := range spans[1:] {
				if s.Kind != syntax.Literal {
					break
				}
				rest += s.Value
			}
			if rest != tc.after {
				t.Errorf("%q: text after is %q, want %q", tc.src, rest, tc.after)
			}
		})
	}
}

// The letters the lexer stops on are the ones the evaluator applies, and
// there is one table.
//
// This is the join that keeps the two halves from drifting: the lexer decides
// where the word ends and the evaluator decides what the segment does, and a
// letter in one set and not the other is either a word cut for nothing or a
// modifier nobody can write unbraced. A second copy of the set is this
// repository's standing way of introducing exactly that — see
// syntax/modifier.go.
func TestEveryModifierLetterEndsTheSameWord(t *testing.T) {
	t.Parallel()
	for letter := range syntax.ModifierLetters {
		src := "echo $p:" + string(letter)
		if letter == 's' {
			src += "/a/Z/"
		}
		spans := spansOf(t, src, bareMods())
		if len(spans) != 1 || spans[0].Kind != syntax.ParamExp {
			t.Errorf("%q: %+v, want one expansion — the letter did not join it", src, spans)
		}
	}
	// And a letter that is not in the table does not, which is what says the
	// loop above is reading the table rather than accepting everything.
	for _, letter := range []byte{'z', 'x', 'B', '1', '%'} {
		src := "echo $p:" + string(letter)
		if spans := spansOf(t, src, bareMods()); len(spans) == 1 {
			t.Errorf("%q: one span, want the colon given back", src)
		}
	}
}
