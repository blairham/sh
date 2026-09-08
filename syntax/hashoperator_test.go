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
		// `:-` reads the same way *here*, and only here, because a grammar
		// with no nameless expansion has nothing for the `#` to be a length
		// of: `${#:-w}` is `$#` with a default that never fires, and the
		// five shells without the form answer 2. The one that has it reads a
		// length instead — TestANamelessExpansionIsALengthsOperand below.
		{`echo ${#:-w}`, "#", ParamDefault, false},
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
// plausible number in place of a refusal. Separating those two needs a
// backtracking parse — `${#-}` is a length and `${#-w}` is not — so they are
// still filed rather than guessed.
//
// `${#:-w}` used to sit here as a third unreadable shape and no longer does:
// it is decided by NamelessParamExpansion, in both directions, and the two
// tests either side of this one hold the two answers.
func TestAHashWithNoReadingStaysABadSubstitution(t *testing.T) {
	d := Core()
	for _, src := range []string{
		`echo ${#%}`,
		`echo ${#-w}`,
		`echo ${#?w}`,
	} {
		if e := firstParam(t, src, d); !e.Bad {
			t.Errorf("%s: read as %+v, want a bad substitution", src, e)
		}
	}
}

// With a nameless expansion in the grammar, `${#:-w}` is a *length* over it:
// the `#` is the prefix, the name is absent, and the operand is what gets
// measured. Measured 2026-09-08 with `set -- p q`: zsh 5.9.2 answers 1 where
// the other five answer 2, and `${#:-abcd}` is 4 there against 2.
//
// The one operator that moves. `${#:+w}`, `${#:=w}` and `${#:?w}` read the
// `#` as the parameter in all six, and the row above holds them for the
// dialect without the form; this one holds them again *with* it, because a
// rule about the colon rather than about `:-` would break exactly here and
// would still pass the other test.
func TestANamelessExpansionIsALengthsOperand(t *testing.T) {
	d := Core()
	d.NamelessParamExpansion = true
	// The length and its operator are one node here — Length with Op set —
	// so the shape needs the flag that lets a length carry an operator as
	// well. Not an accident of this test: the one shell with the nameless
	// form is also the one shell whose length takes an operator, and a
	// grammar with the first and not the second has no reading for these
	// characters at all.
	d.ParamLengthTakesAnOperator = true
	for _, tc := range []struct {
		src    string
		name   string
		op     ParamOp
		length bool
	}{
		{`echo ${#:-w}`, "", ParamDefault, true},
		{`echo ${#:+w}`, "#", ParamAlternate, false},
		{`echo ${#:=w}`, "#", ParamAssign, false},
		{`echo ${#:?w}`, "#", ParamError, false},
		{`echo ${#=w}`, "#", ParamAssign, false},
		{`echo ${#}`, "#", ParamNone, false},
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
