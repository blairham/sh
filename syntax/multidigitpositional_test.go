// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// multiDigit is the core plus the flag. The flag is named here and the shell
// that sets it is not.
func multiDigit() syntax.Dialect {
	d := syntax.Core()
	d.MultiDigitPositional = true
	return d
}

// The whole of the flag: `$10` is one expansion where it is on and `$1`
// followed by the character `0` where it is off.
//
// Checked on the spans rather than on a result, because the word boundary is
// the entire difference — once the spans are cut, nothing downstream can tell
// the two readings apart, and both readings produce a word that expands
// without complaint.
func TestAMultiDigitPositionalIsOneExpansion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		src   string
		on    string // the expansion's inner text with the flag
		onAft string // the literal text left over with the flag
		off   string // the same, without it
		offAf string
	}{
		{"two digits", "echo $10", "10", "", "1", "0"},
		{"three digits", "echo $123", "123", "", "1", "23"},
		{
			// The run is kept exactly as written. Whoever resolves the name
			// reads the number, which is what makes `$01` the first
			// parameter without a rule about zeros in the lexer.
			name: "a leading zero", src: "echo $01",
			on: "01", onAft: "", off: "0", offAf: "1",
		},
		{"every digit zero", "echo $00", "00", "", "0", "0"},
		{
			// The run still stops at the first character that is not a
			// digit, which is the half the flag does not move.
			name: "a letter after the run", src: "echo $10a",
			on: "10", onAft: "a", off: "1", offAf: "0a",
		},
		{"one digit is unchanged", "echo $1", "1", "", "1", ""},
		{
			// A special is exactly one character either way: the flag is
			// about digits and `$@` is not one.
			name: "a special then a digit", src: "echo $@1",
			on: "@", onAft: "1", off: "@", offAf: "1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, on := range []bool{true, false} {
				d := syntax.Core()
				if on {
					d = multiDigit()
				}
				var value, after string
				for _, s := range spansOf(t, tc.src, d) {
					if s.Kind == syntax.ParamExp {
						value = s.Value
					} else {
						after += s.Value
					}
				}
				want, wantAfter := tc.off, tc.offAf
				if on {
					want, wantAfter = tc.on, tc.onAft
				}
				if value != want || after != wantAfter {
					t.Errorf("%q with the flag %v: expansion %q with %q after, want %q and %q",
						tc.src, on, value, after, want, wantAfter)
				}
			}
		})
	}
}

// The braced spelling is the flag's control: `${10}` is the tenth parameter
// in every shell in the panel, so the flag must not be what decides it. A
// change that made the braced form conditional would pass every unbraced row
// above and still be wrong.
func TestTheBracedSpellingDoesNotNeedTheFlag(t *testing.T) {
	t.Parallel()
	for _, on := range []bool{true, false} {
		d := syntax.Core()
		if on {
			d = multiDigit()
		}
		spans := spansOf(t, "echo ${10}", d)
		if len(spans) != 1 || spans[0].Kind != syntax.ParamExp || spans[0].Value != "10" {
			t.Errorf("with the flag %v: spans %+v, want one expansion of 10", on, spans)
		}
	}
}

// A run of digits takes no subscript, with the flag as without it: the
// parameters that carry a bare `[ … ]` are the names, `@` and `*`, and a
// wider run of digits does not join them.
func TestAMultiDigitPositionalStillTakesNoSubscript(t *testing.T) {
	t.Parallel()
	d := multiDigit()
	d.BareSubscript = true
	spans := spansOf(t, "echo $10[2]", d)
	if len(spans) != 2 || spans[0].Value != "10" || spans[1].Value != "[2]" {
		t.Errorf("spans %+v, want the expansion 10 and the text [2]", spans)
	}
}

// The length sigil reads the same run, which falls out of the two flags
// rather than being a rule of its own: `$#10` is the length of the tenth
// parameter where a dialect has both.
func TestALengthReadsTheWholeRun(t *testing.T) {
	t.Parallel()
	d := multiDigit()
	d.BareSubscript = true
	spans := spansOf(t, "echo $#10", d)
	if len(spans) != 1 || spans[0].Value != "#10" {
		t.Errorf("spans %+v, want one expansion of #10", spans)
	}
}
