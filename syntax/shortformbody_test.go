// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A loop or an `if` whose header ended itself may take its body without the
// words that ordinarily open and close it, and the spellings are two flags
// rather than one: the brace group is [Dialect.ShortForm]'s and the single
// command and the omitted body are [Dialect.ShortFormBody]'s. See the field,
// where the grid is.

// shortFormWithoutTheBody is the reading of the shell whose short-body option
// is off: the family is there and two of its three body spellings are not.
func shortFormWithoutTheBody() Dialect {
	d := Core()
	d.ShortForm = true
	d.ShortFormBody = false
	return d
}

// shortFormWithTheBody differs in that one field.
func shortFormWithTheBody() Dialect {
	d := shortFormWithoutTheBody()
	d.ShortFormBody = true
	return d
}

func TestAShortBodyIsWrittenWithoutBracesWhereTheFlagSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		// refused with the flag off and with it on. The flag moves a row only
		// where the two differ, and that is the whole population it reaches.
		off, on bool
	}{
		// One command, in every construct the family reaches. The separator
		// before it is optional and is not what decides, which is why the
		// same loop is here written both ways.
		{"a for's list and one command", "for i in a b; echo $i\n", true, false},
		{"a for's parenthesized list and one command", "for i (a b) echo $i\n", true, false},
		{"a select's parenthesized list and one command", "select o (a b) :\n", true, false},
		{"an if whose condition ended itself", "if (( 1 )) echo hi\n", true, false},
		{"a while with a separator", "while (( 0 )); :\n", true, false},
		{"a while with none", "while (( 0 )) :\n", true, false},
		{"an until", "until (( 1 )); :\n", true, false},
		// And the body omitted altogether, which is the same production with
		// nothing after the header.
		{"a for with a name and no list", "for i\n", true, false},
		{"a for with a list and no body", "for i in a b\n", true, false},
		{"a while with nothing after it", "while\n", true, false},
		{"a while with a condition and no body", "while true\n", true, false},
		{"an until with nothing after it", "until\n", true, false},
		// The brace spelling, which the flag does not reach: it is the
		// family's own and stays wherever ShortForm left it. A rule about
		// "a body written short" takes these with the rows above and is
		// wrong about every one of them.
		{"an if with a brace body", "if (( 1 )) { echo A; }\n", false, false},
		{"an if with a brace body and an else", "if (( 1 )) { echo A; } else { echo B; }\n", false, false},
		{"an if with a brace body and a redundant fi", "if (( 1 )) { echo A; } fi\n", false, false},
		{"a while with a brace body", "while (( 0 )) { :; }\n", false, false},
		{"an until with a brace body", "until (( 1 )) { :; }\n", false, false},
		{"a select with a brace body", "select o (a b) { :; }\n", false, false},
		{"a for with a parenthesized list and a brace body", "for i (a b) { echo $i; }\n", false, false},
		{"a for with a list and a brace body", "for i in a b; { echo $i; }\n", false, false},
		{"a C-style for with a brace body", "for ((i=0;i<2;i++)) { echo $i; }\n", false, false},
		// Nor does it reach the parenthesized item list, which is reachable
		// with the long body in both states.
		{"a parenthesized list with a keyword body", "for i (a b); do echo $i; done\n", false, false},
		// The controls on either side: the long form is untouched, and a body
		// that is a syntax error stays one, so a flag that had widened the
		// grammar rather than moved this one production would show here.
		{"the long form", "for i in a b; do echo $i; done\n", false, false},
		{"an ordinary command", "echo hi\n", false, false},
		{"a stop word where a command belongs", "{ fi; }\n", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.src, shortFormWithoutTheBody()); (err != nil) != tc.off {
				t.Errorf("with the flag off: refused=%v, want %v (%v)", err != nil, tc.off, err)
			}
			if _, err := Parse(tc.src, shortFormWithTheBody()); (err != nil) != tc.on {
				t.Errorf("with the flag on: refused=%v, want %v (%v)", err != nil, tc.on, err)
			}
		})
	}
}

// And the flag answers nothing at all for a dialect without the family: a
// short body is refused there in both states, which is what says this is a
// narrowing of ShortForm rather than a production of its own.
func TestTheShortBodyFlagNeedsTheFamilyToBeThere(t *testing.T) {
	for _, src := range []string{
		"for i in a b; echo $i\n",
		"while (( 0 )) :\n",
		"if (( 1 )) echo hi\n",
		"while true\n",
	} {
		d := Core()
		d.ShortForm = false
		d.ShortFormBody = true
		if _, err := Parse(src, d); err == nil {
			t.Errorf("%q parsed with ShortForm off, so the flag reaches past the family it narrows", src)
		}
	}
}
