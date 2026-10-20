// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// The `x` and `n` letters will not take a name with a dot in it, which is
// `export`'s own refusal arriving on `typeset`.
//
// Measured 2026-09-20 against AT&T ksh93u+ 2012-08-01 (`/bin/ksh` here),
// `-c` under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on the
// null device. Every refusing row below was status 0 here, with the attribute
// recorded on a member the reference never gives one (#3956).
//
// The refusal is fatal, as every bad name on a declaration is in this shell,
// so each row is the whole of what the script writes.
func TestTheExportAndReferenceLettersRefuseADottedOperand(t *testing.T) {
	const zz = `typeset zz=(a=1); `
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			"the export letter over a member that stands",
			zz + `typeset -x zz.a; print st=$?`,
			"sh: typeset: zz.a: is not an identifier\n", 1,
		},
		{
			"under a plus, which is the same refusal",
			zz + `typeset +x zz.a; print st=$?`,
			"sh: typeset: zz.a: is not an identifier\n", 1,
		},
		{
			"and in company, where one letter is enough",
			zz + `typeset -xr zz.a; print st=$?`,
			"sh: typeset: zz.a: is not an identifier\n", 1,
		},
		{
			"over a name no compound holds",
			`typeset -x nosuch.x; print st=$?`,
			"sh: typeset: nosuch.x: is not an identifier\n", 1,
		},
		{
			"and over a leading-dot name",
			`typeset -x .foo; print st=$?`,
			"sh: typeset: .foo: is not an identifier\n", 1,
		},
		{
			"the reference letter likewise",
			zz + `typeset -n zz.q; print st=$?`,
			"sh: typeset: zz.q: is not an identifier\n", 1,
		},
		{
			"and under a plus",
			zz + `typeset +n zz.a; print st=$?`,
			"sh: typeset: zz.a: is not an identifier\n", 1,
		},
		// The sentence the reference letter carries is `export`'s, which is
		// the letter's and not the word's: the plain declaration says
		// `invalid variable name` to the same operand.
		{
			"the reference letter's reason is export's",
			`typeset -n 1x; print st=$?`,
			"sh: typeset: 1x: is not an identifier\n", 1,
		},
		{
			"the control: the plain declaration's reason is its own",
			`typeset 1x; print st=$?`,
			"sh: typeset: 1x: invalid variable name\n", 1,
		},
		// And a dotted operand carrying a value leaves the builtin out of
		// the sentence, which the valueless form and a valued plain name
		// both keep.
		{
			"a dotted operand with a value names itself alone",
			zz + `typeset -n zz.q=zz; print st=$?`,
			"sh: zz.q=zz: is not an identifier\n", 1,
		},
		{
			"and so does export's own",
			`export .foo=1; print st=$?`,
			"sh: .foo=1: is not an identifier\n", 1,
		},
		{
			"the control: a valued plain name keeps the builtin",
			`export 1x=v; print st=$?`,
			"sh: export: 1x=v: is not an identifier\n", 1,
		},
		{
			"the control: a valueless dotted operand keeps it too",
			`export .foo; print st=$?`,
			"sh: export: .foo: is not an identifier\n", 1,
		},
		// And the operand is refused **as written**, ahead of the reference
		// walk a member path otherwise takes: `c.b` over `typeset -n c=zz`
		// is named `c.b` and not `zz.b`. The name check runs before the
		// operand loop that resolves the path, which is what keeps the two
		// in this order — see Runner.compoundMemberThroughAReference, whose
		// call in that loop the letters never reach.
		{
			"through a reference, the refusal names the path as written",
			`typeset zz=(a=1 b=2); typeset -n c=zz; typeset -x c.b`,
			"sh: typeset: c.b: is not an identifier\n", 1,
		},
		{
			"and export says the same of it, since `c` is no compound",
			`typeset zz=(a=1 b=2); typeset -n c=zz; export c.b`,
			"sh: export: c.b: is not an identifier\n", 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := answersRun(t, c.src)
			if out != c.want || st != c.status {
				t.Errorf("%s = %q at %d, want %q at %d", c.src, out, st, c.want, c.status)
			}
		})
	}
}

// And the letters are the whole of the split: every other declaration takes a
// member path and records its attribute there.
//
// These are the controls for the refusals above and each was already right.
func TestTheOtherLettersTakeADottedOperand(t *testing.T) {
	const zz = `typeset zz=(a=1 b=2); `
	for _, c := range []struct{ name, src, want string }{
		{"a bare declaration", zz + `typeset zz.a; print st=$?`, "st=0\n"},
		{"the case letter", zz + `typeset -u zz.b; zz.b=hi; print -r -- "[${zz.b}]"`, "[HI]\n"},
		{"the integer letter", zz + `typeset -i zz.a; zz.a=3+4; print -r -- "[${zz.a}]"`, "[7]\n"},
		{"the freeze, by letter and by word", zz + `typeset -r zz.a; readonly zz.b; print st=$?`, "st=0\n"},
		// A subscript is not a member path, and the export letter takes one.
		{"the export letter over a subscript", `typeset -x a[1]; print st=$?`, "st=0\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := answersRun(t, c.src)
			if out != c.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// `export` takes a member of a compound that stands, and refuses it in the
// words it keeps for a compound rather than in the words it keeps for a name
// it cannot read.
//
// Measured the same day and the same way. The three rows that do *not* get
// the compound's sentence are what narrow the exception to a member that is
// set and is not itself a compound; all three were already right here, and
// the first two rows were `is not an identifier` (#3956).
func TestExportTakesACompoundMemberAndRefusesItAsACompound(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a member that stands",
			`typeset zz=(a=1); export zz.a`,
			"sh: export: zz.a: only simple variables can be exported\n",
		},
		{
			"and with a value, where it names the member and not the operand",
			`typeset zz=(a=1); export zz.a=v`,
			"sh: export: zz.a: only simple variables can be exported\n",
		},
		{
			"a member that is not there is not a name",
			`typeset zz=(a=1); export zz.nope`,
			"sh: export: zz.nope: is not an identifier\n",
		},
		{
			"a member that is itself a compound is not one either",
			`typeset zz=(a=1 n=(y=1)); export zz.n`,
			"sh: export: zz.n: is not an identifier\n",
		},
		{
			"and neither is a dotted name under a scalar",
			`zz=plain; export zz.a`,
			"sh: export: zz.a: is not an identifier\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := answersRun(t, c.src)
			if out != c.want || st != 1 {
				t.Errorf("%s = %q at %d, want %q at 1", c.src, out, st, c.want)
			}
		})
	}
}
