// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// specialSub is the core plus a subscript on a parameter that is not a name.
// The flag is named here and the shell that sets it is not.
func specialSub(on bool) syntax.Dialect {
	d := syntax.Core()
	d.SpecialParamSubscript = on
	return d
}

// paramOf returns the parameter expansion the operand of the command is.
func paramOf(t *testing.T, src string, d syntax.Dialect) *syntax.ParamExpr {
	t.Helper()
	spans := spansOf(t, src, d)
	if len(spans) != 1 || spans[0].Kind != syntax.ParamExp || spans[0].Param == nil {
		t.Fatalf("parse %q: operand is not one parameter expansion, got %d spans", src, len(spans))
	}
	return spans[0].Param
}

// The whole of the flag: a name carries a subscript wherever there are
// subscripts at all, and a parameter that is not a name carries one only here.
//
// Off, the bracket is left unconsumed and the leftover text takes the ordinary
// route an unreadable expansion takes — which is what produces each grammar's
// own refusal without this flag having to word one.
func TestASubscriptOnASpecialParameterNeedsTheFlag(t *testing.T) {
	for _, tc := range []struct {
		name  string
		src   string
		index string // the subscript's text, "" for none read
	}{
		{"the positional list", "echo ${@[1]}", "1"},
		{"the joined list", "echo ${*[2]}", "2"},
		{"a positional parameter", "echo ${1[2]}", "2"},
		{"two digits of one", "echo ${10[2]}", "2"},
		{"the shell's name", "echo ${0[1]}", "1"},
		{"the status", "echo ${?[1]}", "1"},
		{"the option letters", "echo ${-[1]}", "1"},
		{"the process id", "echo ${$[1]}", "1"},
		{"a range of them", "echo ${@[1,3]}", "1,3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			on := paramOf(t, tc.src, specialSub(true))
			if on.Index == nil {
				t.Fatalf("%s with the flag: no subscript read", tc.src)
			}
			if got := syntax.PrintWord(on.Index); got != tc.index {
				t.Errorf("%s with the flag: subscript %q, want %q", tc.src, got, tc.index)
			}
			off := paramOf(t, tc.src, specialSub(false))
			if off.Index != nil {
				t.Errorf("%s without the flag: read a subscript %q", tc.src, syntax.PrintWord(off.Index))
			}
			if !off.Bad {
				t.Errorf("%s without the flag: not marked unreadable", tc.src)
			}
		})
	}
}

// A *name* is unaffected in either direction, which is the claim that keeps
// this flag separate from ArraySubscript: `${a[1]}` is read by four of the six
// shells measured and `${@[1]}` by one, so a single flag could not say both.
func TestASubscriptOnANameIgnoresTheFlag(t *testing.T) {
	for _, src := range []string{
		"echo ${a[1]}", "echo ${a[@]}", "echo ${_x[1,3]}", "echo ${A1[0]}",
	} {
		for _, on := range []bool{true, false} {
			e := paramOf(t, src, specialSub(on))
			if e.Index == nil {
				t.Errorf("%s with the flag %v: no subscript read", src, on)
			}
			if e.Bad {
				t.Errorf("%s with the flag %v: marked unreadable", src, on)
			}
		}
	}
}

// And the grammar that refuses an unreadable expansion while *reading* refuses
// this one there too, rather than deferring it — the same split every other
// unreadable expansion follows.
func TestASubscriptOnASpecialParameterIsRefusedWhileReadingWhereThatIsTheRule(t *testing.T) {
	d := specialSub(false)
	d.BadSubstitutionAtParseTime = true
	if _, err := syntax.Parse("echo ${@[1]}", d); err == nil {
		t.Fatal("parsed ${@[1]} without the flag under a grammar that refuses while reading")
	}
	d.SpecialParamSubscript = true
	if _, err := syntax.Parse("echo ${@[1]}", d); err != nil {
		t.Fatalf("parse ${@[1]} with the flag: %v", err)
	}
}
