// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A `#` at the front of an expansion is the length prefix or the parameter
// `$#`, and what decides it is the operator behind it.
//
// Measured 2026-09-07 with `set -- p q`, so `$#` is 2, against bash 5.3.15,
// bash 3.2.57, that build invoked as `sh`, dash, ksh93u+ and zsh 5.9.2. The
// rows below are the ones where all six agree, which is most of them — the
// issue this closes read the shape as one shell's and it is core.
//
// The two readings are not separated by anything as tidy as "an operator
// follows": a `-` or a `?` behind the `#` is a *name*, so `${#-}` and `${#?}`
// are lengths and `${#-w}` is a length with a stray word after it. What can
// never begin a name — `=`, `+`, a `:` operator, and a `%`, `/` or `#` with
// an operand — leaves the `#` as the parameter.
func TestAHashIsTheParameterOrTheLengthPrefix(t *testing.T) {
	d := Core()
	for _, tc := range []struct {
		src    string
		name   string
		op     ParamOp
		length bool
	}{
		// The issue's own row: an assignment on `$#`, and neither a length
		// nor — in the grammar that has the flag — the split run.
		{`echo ${#=w}`, "#", ParamAssign, false},
		{`echo ${#:=w}`, "#", ParamAssign, false},
		{`echo ${#+w}`, "#", ParamAlternate, false},
		{`echo ${#:+w}`, "#", ParamAlternate, false},
		{`echo ${#:?w}`, "#", ParamError, false},
		// A trim, a replacement and a substring on the *value* of `$#`.
		{`echo ${##2}`, "#", ParamTrimPrefix, false},
		{`echo ${#%2}`, "#", ParamTrimSuffix, false},
		{`echo ${#/2/X}`, "#", ParamReplace, false},
		{`echo ${#:0:1}`, "#", ParamSubstring, false},

		// And the lengths. `${##}` is the same two characters as `${##2}`
		// resolved the other way — unanimously the length of `$#` — which is
		// why an operand is what the trim reading requires.
		{`echo ${##}`, "#", ParamNone, true},
		{`echo ${#?}`, "?", ParamNone, true},
		{`echo ${#-}`, "-", ParamNone, true},
		{`echo ${#@}`, "@", ParamNone, true},
		{`echo ${#*}`, "*", ParamNone, true},
		{`echo ${#$}`, "$", ParamNone, true},
		{`echo ${#0}`, "0", ParamNone, true},
		{`echo ${#v}`, "v", ParamNone, true},
	} {
		e := firstParam(t, tc.src, d)
		if e.Bad {
			t.Errorf("%s: read as a bad substitution, want name %q", tc.src, tc.name)
			continue
		}
		if e.Name != tc.name || e.Op != tc.op || e.Length != tc.length {
			t.Errorf("%s: name=%q op=%v length=%v, want %q %v %v",
				tc.src, e.Name, e.Op, e.Length, tc.name, tc.op, tc.length)
		}
	}
}

// The shapes that stay unreadable, and each is unreadable for a reason a
// reader can check.
//
// `${#%}` is a bad substitution in five of the six — the trim reading needs an
// operand and the name reading has no name — and zsh alone answers `$#`.
// `${#-w}` and `${#?w}` are a length over `$-` and `$?` with a stray word
// after them, which is zsh's refusal exactly; the other five read the `#` as
// the parameter there and answer 2, and taking their side would hand zsh a
// plausible number in place of a refusal. `${#:-w}` is a third reading again:
// 2 in five shells and 1 in zsh, where it is the length of the nameless
// `${:-w}` this grammar does not have. All three are filed rather than guessed.
func TestAHashWithNoReadingStaysABadSubstitution(t *testing.T) {
	d := Core()
	for _, src := range []string{
		`echo ${#%}`,
		`echo ${#-w}`,
		`echo ${#?w}`,
		`echo ${#:-w}`,
	} {
		if e := firstParam(t, src, d); !e.Bad {
			t.Errorf("%s: read as %+v, want a bad substitution", src, e)
		}
	}
}
