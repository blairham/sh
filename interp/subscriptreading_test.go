// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runReading runs src with the two subscript-reading axes answered, and with
// the array base answered too — a case about which element a subscript names
// turns on all three.
func runReading(t *testing.T, zeroBased, comma, character Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.SpecialParamSubscript = true
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.ArrayBaseIsZero = zeroBased
		sem.SubscriptCommaIsARange = comma
		sem.ScalarSubscriptIsACharacter = character
		r.Semantics = &sem
	})
}

// One subscript, two readings. The comma is either the separator of a range or
// the arithmetic operator whose value is its right operand — and the two
// answers are two different values with no diagnostic between them, which is
// what makes it a conflict rather than a construct one grammar has.
func TestASubscriptPairIsAnAxis(t *testing.T) {
	const src = `a=(w x y z); printf "[%s]" "${a[1,3]}"`
	if out, st := runReading(t, No, Yes, No, src); out != "[w x y]" || st != 0 {
		t.Errorf("as a range = %q (status %d), want %q at 0", out, st, "[w x y]")
	}
	// The other reading evaluates the whole text and uses the result as one
	// subscript, which is the third element where the first is 1.
	if out, st := runReading(t, No, No, No, src); out != "[y]" || st != 0 {
		t.Errorf("as an operator = %q (status %d), want %q at 0", out, st, "[y]")
	}
}

// The endpoints are subscripts, so they are counted from the base — which is
// what keeps `${a[2,2]}` and `${a[2]}` naming the same element under either
// answer to the base.
func TestARangeCountsFromTheBase(t *testing.T) {
	const src = `a=(w x y z); printf "[%s]" "${a[1,2]}"`
	if out, _ := runReading(t, No, Yes, No, src); out != "[w x]" {
		t.Errorf("one-based = %q, want %q", out, "[w x]")
	}
	if out, _ := runReading(t, Yes, Yes, No, src); out != "[x y]" {
		t.Errorf("zero-based = %q, want %q", out, "[x y]")
	}
}

// The axis is asked only where the two readings differ. A pair naming one
// element names it under either reading, so nothing has to be chosen — which
// is what keeps a core that has chosen no dialect from refusing it.
func TestAPairNamingOneElementAsksNothing(t *testing.T) {
	out, st := runReading(t, No, Unspecified, No, `a=(w x y z); printf "[%s]" "${a[2,2]}"`)
	if out != "[x]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[x]")
	}
}

// And where they do differ, an unanswered axis is refused by name rather than
// guessed at.
func TestAPairThatDiffersRefusesAnUnansweredAxis(t *testing.T) {
	out, st := runReading(t, No, Unspecified, No, `a=(w x y z); printf "[%s]" "${a[1,3]}"`)
	want := "sh: `${a[1,3]}` naming a range rather than one subscript: " +
		"the shells disagree here and no dialect was chosen\n"
	if !strings.HasPrefix(out, want) {
		t.Errorf("output = %q, want it to open with %q", out, want)
	}
	if st != 2 {
		t.Errorf("status %d, want the unanswered axis refused at 2", st)
	}
}

// A subscript on a plain string is either one of its characters or an element
// of the one-element array a scalar reads as. Both answer, neither reports.
func TestAScalarSubscriptIsAnAxis(t *testing.T) {
	const src = `s=hello; printf "[%s][%s]" "${s[1]}" "${s[2]}"`
	if out, st := runReading(t, No, No, Yes, src); out != "[h][e]" || st != 0 {
		t.Errorf("as characters = %q (status %d), want %q at 0", out, st, "[h][e]")
	}
	if out, st := runReading(t, No, No, No, src); out != "[hello][]" || st != 0 {
		t.Errorf("as one element = %q (status %d), want %q at 0", out, st, "[hello][]")
	}
}

// A string has no end to count back from, so under the element reading a
// negative subscript names nothing: bash and ksh93 both answer empty, and bash
// also warns `s: bad array subscript` where ksh93 is silent — the value is
// what is unanimous.
//
// It answered the whole string, because the string went to the array path and
// a one-element array's last element is its first. A plausible value with no
// diagnostic near it.
func TestANegativeSubscriptOnAStringNamesNothing(t *testing.T) {
	for _, src := range []string{
		`s=hello; printf "[%s]" "${s[-1]}"`,
		`s=hello; printf "[%s]" "${s[-9]}"`,
	} {
		if out, st := runReading(t, Yes, No, No, src); out != "[]" || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", src, out, st, "[]")
		}
		// And the base still names the string itself, which is the reading
		// this is the edge of rather than a refusal of the whole shape.
		if out, _ := runReading(t, Yes, No, No, `s=hello; printf "[%s]" "${s[0]}"`); out != "[hello]" {
			t.Errorf("the base = %q, want %q", out, "[hello]")
		}
	}
}

// Asked only where the readings differ: a one-character string at the
// subscript both readings answer to is itself either way.
func TestAOneCharacterStringAsksNothing(t *testing.T) {
	out, st := runReading(t, No, No, Unspecified, `s=h; printf "[%s]" "${s[1]}"`)
	if out != "[h]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[h]")
	}
	// And a subscript naming nothing under either reading needs no answer.
	if out, st := runReading(t, No, No, Unspecified, `s=hello; printf "[%s]" "${s[9]}"`); out != "[]" || st != 0 {
		t.Errorf("past the end = %q (status %d), want %q at 0", out, st, "[]")
	}
}

// An array is never read as characters, however short its elements are: the
// axis is about a *scalar*, and an array holding one element is not one.
func TestAnArrayIsNotReadAsCharacters(t *testing.T) {
	out, st := runReading(t, No, No, Yes, `a=(hello); printf "[%s]" "${a[1]}"`)
	if out != "[hello]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[hello]")
	}
}

// A character is a character and not a byte where the locale names a
// multibyte encoding, and a byte where it does not — the two halves of
// MultibyteEncodingIsHonored, on the subscript that indexes them.
func TestAStringSubscriptCountsCharacters(t *testing.T) {
	out, st := runReading(t, No, Yes, Yes,
		`LC_ALL=en_US.UTF-8; s=héllo; printf "[%s][%s]" "${s[2]}" "${s[2,3]}"`)
	if out != "[é][él]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[é][él]")
	}
	// The same subscript in a single-byte locale names the second *byte*,
	// which is half of the two-byte character and is not valid UTF-8 on its
	// own — the answer the whole panel gives under LC_ALL=C.
	out, st = runReading(t, No, Yes, Yes,
		`LC_ALL=C; s=héllo; printf "[%s][%s]" "${s[2]}" "${s[2,3]}"`)
	if out != "[\xc3][é]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[\xc3][é]")
	}
}

// `@` and `*` are the positional parameters as a list, where the grammar reads
// a subscript on them at all; every other special parameter is a value, and a
// subscript reaches into it the way it reaches into any other string.
func TestASubscriptOnTheSpecialParameters(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the list", `set -- a b c; printf "[%s][%s]" "${@[1]}" "${*[3]}"`, "[a][c]"},
		{"from the end", `set -- a b c; printf "[%s]" "${@[-1]}"`, "[c]"},
		{"a range of it keeps its fields", `set -- a b c; printf "[%s]" "${@[1,2]}"`, "[a][b]"},
		{"and joins through the star", `set -- a b c; printf "[%s]" "${*[1,2]}"`, "[a b]"},
		{"a parameter is a string", `set -- abcd; printf "[%s]" "${1[2]}"`, "[b]"},
		{"and a range of one", `set -- abcd; printf "[%s]" "${1[2,3]}"`, "[bc]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runReading(t, No, Yes, Yes, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// A subscript on a parameter that is not a name needs the grammar flag: with
// it off the bracket is never consumed, and the leftover text is what makes
// the expansion unreadable.
func TestASubscriptOnASpecialParameterNeedsTheGrammar(t *testing.T) {
	out, st := runGrammar(t, `set -- a b; printf "[%s]" "${@[1]}"; echo " ok"`, nil, nil)
	if strings.Contains(out, "ok") {
		t.Errorf("output %q ran on past the refusal", out)
	}
	if st == 0 {
		t.Errorf("status 0, want the expansion refused")
	}
}

// A range with three parts is a refusal in the reading that has ranges, and
// the other reading's answer everywhere else. Answering the second under the
// first would be the silent kind of wrong: `${a[1,2,3]}` would be an element.
func TestAThreePartSubscript(t *testing.T) {
	const src = `a=(w x y z); printf "[%s]" "${a[1,2,3]}"; echo " ok"`
	out, st := runReading(t, No, Yes, No, src)
	if strings.Contains(out, "ok") || st == 0 {
		t.Errorf("as a range = %q (status %d), want a refusal that stops", out, st)
	}
	if out, st := runReading(t, No, No, No, src); out != "[y] ok\n" || st != 0 {
		t.Errorf("as an operator = %q (status %d), want %q at 0", out, st, "[y] ok\n")
	}
}

// A comma inside nesting is not the range's: `${a[f(1,2)]}` has one endpoint.
func TestANestedCommaIsNotTheRange(t *testing.T) {
	out, st := runReading(t, No, Yes, No, `a=(w x y z); printf "[%s]" "${a[(1,2)]}"`)
	if out != "[x]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[x]")
	}
}
