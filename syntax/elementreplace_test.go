// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"testing"
)

// wholeElementReplacing is the core plus `${a:/pattern/replacement}`. The flag
// is named and the shell that sets it is not.
func wholeElementReplacing() Dialect {
	d := Core()
	d.ParamWholeElementReplace = true
	return d
}

// The operator parses to a node of its own, with the pattern and the
// replacement split on the first unquoted slash — the replacement's operands
// under the element family's reading.
func TestTheWholeElementReplacementParses(t *testing.T) {
	for _, tc := range []struct {
		name           string
		src            string
		pattern, with  string
		hasReplacement bool
	}{
		{"pattern and replacement", `echo ${a:/foo/Z}`, "foo", "Z", true},
		{"an empty replacement", `echo ${a:/foo/}`, "foo", "", true},
		{"no replacement at all", `echo ${a:/foo}`, "foo", "", false},
		{"a pattern with a metacharacter", `echo ${a:/b*/Z}`, "b*", "Z", true},
		{
			// The pattern stops at the *first* slash and the replacement
			// keeps the rest, however many more there are.
			"slashes after the first belong to the replacement",
			`echo ${a:/foo/a/b}`, "foo", "a/b", true,
		},
		{"an empty pattern", `echo ${a:/}`, "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := firstParam(t, tc.src, wholeElementReplacing())
			if e.Op != ParamElementReplace {
				t.Fatalf("op = %v (%q), want %v (%q)",
					e.Op, e.Op, ParamElementReplace, ParamElementReplace)
			}
			got := ""
			if e.Arg != nil {
				got = e.Arg.Literal()
			}
			if got != tc.pattern {
				t.Errorf("pattern = %q, want %q", got, tc.pattern)
			}
			if (e.Arg2 != nil) != tc.hasReplacement {
				t.Fatalf("replacement present = %v, want %v", e.Arg2 != nil, tc.hasReplacement)
			}
			if e.Arg2 == nil {
				return
			}
			if with := e.Arg2.Literal(); with != tc.with {
				t.Errorf("replacement = %q, want %q", with, tc.with)
			}
		})
	}
}

// The flag decides, and without it the same characters are the substring the
// rest of the panel reads — which is what makes the grammar additive.
func TestWithoutTheFlagTheSlashStartsAnOffset(t *testing.T) {
	for _, src := range []string{`echo ${a:/foo/Z}`, `echo ${a:/foo}`} {
		on := firstParam(t, src, wholeElementReplacing())
		off := firstParam(t, src, Core())
		if on.Op == off.Op {
			t.Errorf("%s: both grammars read %v — the flag decided nothing", src, on.Op)
		}
		if off.Op != ParamSubstring {
			t.Errorf("%s: without the flag op = %v, want the substring", src, off.Op)
		}
	}
}

// The slash has to sit directly against the colon, and only against the
// *first* one. Everything else after a colon is what it always was.
//
// This is the guard the change rests on. A division cannot begin an
// arithmetic expression, so the operator can be read ahead of the substring
// without taking anything from it — but only if the rule stays exactly one
// character wide, and only if the length half of `${v:off:len}` keeps its
// arithmetic.
func TestTheSlashIsReadOnlyDirectlyAfterTheFirstColon(t *testing.T) {
	for _, tc := range []struct {
		src  string
		op   ParamOp
		arg  string
		arg2 string
	}{
		// An offset whose arithmetic is a division: the slash has something
		// in front of it, so nothing here is the new operator.
		{`echo ${v:3/2}`, ParamSubstring, "3/2", ""},
		// A space between them, and the substring takes the whole of it.
		{`echo ${v: /2}`, ParamSubstring, " /2", ""},
		// The second colon opens the length, which stays arithmetic.
		{`echo ${v:2:/1}`, ParamSubstring, "2", "/1"},
		{`echo ${v:2}`, ParamSubstring, "2", ""},
		{`echo ${v:2:2}`, ParamSubstring, "2", "2"},
		{`echo ${v: -2}`, ParamSubstring, " -2", ""},
		{`echo ${v:+set}`, ParamAlternate, "set", ""},
		{`echo ${v:-alt}`, ParamDefault, "alt", ""},
		{`echo ${v:=set}`, ParamAssign, "set", ""},
		{`echo ${v:?msg}`, ParamError, "msg", ""},
		{`echo ${v:}`, ParamSubstring, "", ""},
	} {
		t.Run(tc.src, func(t *testing.T) {
			e := firstParam(t, tc.src, wholeElementReplacing())
			if e.Op != tc.op {
				t.Fatalf("op = %v (%q), want %v (%q)", e.Op, e.Op, tc.op, tc.op)
			}
			arg, arg2 := "", ""
			if e.Arg != nil {
				arg = e.Arg.Literal()
			}
			if e.Arg2 != nil {
				arg2 = e.Arg2.Literal()
			}
			if arg != tc.arg || arg2 != tc.arg2 {
				t.Errorf("operands = %q, %q; want %q, %q", arg, arg2, tc.arg, tc.arg2)
			}
		})
	}
}

// The two flags are independent: a grammar with the selectors and not this one
// still reads `:/` as an offset, and a grammar with this one and not the
// selectors still reads `:#` as one. Neither flag is quietly the other.
func TestTheTwoWholeElementFlagsAreIndependent(t *testing.T) {
	selectorsOnly := Core()
	selectorsOnly.ParamElementSelection = true
	if got := firstParam(t, `echo ${a:/foo/Z}`, selectorsOnly).Op; got != ParamSubstring {
		t.Errorf("with the selectors alone, `:/` = %v, want the substring", got)
	}
	if got := firstParam(t, `echo ${a:#foo}`, wholeElementReplacing()).Op; got != ParamSubstring {
		t.Errorf("with the replacement alone, `:#` = %v, want the substring", got)
	}
}

// The operands are words, so a pattern and a replacement may both be built out
// of expansions — which is what `${a:/(#m)*/<$MATCH>}` needs to be readable at
// all.
func TestTheWholeElementReplacementOperandsAreWords(t *testing.T) {
	e := firstParam(t, `echo ${a:/$p/$MATCH}`, wholeElementReplacing())
	if e.Op != ParamElementReplace {
		t.Fatalf("op = %v, want the whole-element replacement", e.Op)
	}
	for _, tc := range []struct {
		name string
		w    *Word
	}{{"pattern", e.Arg}, {"replacement", e.Arg2}} {
		if tc.w == nil || len(tc.w.Spans) != 1 {
			t.Fatalf("%s = %#v, want one span", tc.name, tc.w)
		}
		if got := tc.w.Spans[0].Kind; got != ParamExp {
			t.Errorf("%s span kind = %v, want a parameter expansion", tc.name, got)
		}
	}
}
