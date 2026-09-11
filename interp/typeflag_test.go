// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// typeFlagRun is flagsRun with a vocabulary supplied, which is what the `(t)`
// flag needs before it carries at all.
//
// The wording here is this test's and not a shell's: the words are joined
// with a `+` and the kinds are spelled out in full, so a row that passed by
// accidentally reaching some other describer would not read as this one's.
// What the flag owes the dialect is the *facts*; the spelling is the
// dialect's, and that separation is the whole of SetParameterTypeWord.
func typeFlagRun(t *testing.T, src string) (string, string, int) {
	t.Helper()
	d := syntax.Core()
	d.ParamExpansionFlags = true
	// The three constructs the rows reach beside the flag: `${(t)+v}` needs
	// the set test between the braces, `${(t)${s}}` an expansion where a name
	// would be, and the character subscript needs the axis that makes
	// `${s[2]}` a character at all.
	d.ParamSetTestFlag = true
	d.NestedParamExpansion = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.SplitParamExpansion = No
	sem.GlobExpansionResults = No
	sem.FatalErrorStatusIsOne = Yes
	sem.DeclaredNameWithoutValueIsEmpty = Yes
	sem.ArrayBaseIsZero = No
	sem.SubscriptCommaIsARange = Yes
	sem.ArrayScalarIsTheWholeArray = Yes
	sem.ScalarSubscriptIsACharacter = Yes
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &errs, Dialect: &d, Semantics: &sem, Name: "testsh"})
	r.SetParameterTypeWord(func(a ParameterAttributes) string {
		words := []string{"KIND" + itoaKind(a.Kind)}
		for _, attr := range []struct {
			on   bool
			word string
		}{
			{a.Local, "local"},
			{a.Tied, "tied"},
			{a.Readonly, "readonly"},
			{a.Exported, "export"},
			{a.Unique, "unique"},
		} {
			if attr.on {
				words = append(words, attr.word)
			}
		}
		return strings.Join(words, "+")
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

func itoaKind(k ParameterKind) string {
	switch k {
	case ArrayParameter:
		return "array"
	case AssocParameter:
		return "assoc"
	case IntegerParameter:
		return "integer"
	case FloatParameter:
		return "float"
	default:
		return "scalar"
	}
}

// `(t)` puts the *type* of the name in place of its value, and everything
// else in the group then runs on that word.
//
// That is the whole rule, and every row below is one consequence of it. This
// shell had no way to ask a name its type at all, which is the shape that
// hides bugs: a fix whose own tests must fall back to a listing says less,
// and says it in a form that changes when the printer changes (#1657).
//
// Named for the flag rather than for a shell, per the rule in AGENTS.md; the
// words are this test's own, for the reason typeFlagRun gives. Measured on
// zsh 5.9.2, the only shell with the flag; the corpus row is
// `param/expansion-flags-parameter-type`.
func TestTheTypeFlagDescribesTheNameAndNotItsValue(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a scalar", `v=abc; printf "[%s]" "${(t)v}"`, "[KINDscalar]"},
		{"an array", `w=(a b); printf "[%s]" "${(t)w}"`, "[KINDarray]"},
		{"an attribute rides on the kind", `typeset -rx v=1; printf "[%s]" "${(t)v}"`, "[KINDscalar+readonly+export]"},
		{"the letters transform the word", `v=abc; printf "[%s]" "${(Ut)v}"`, "[KINDSCALAR]"},
		{"the length measures it", `v=abc; printf "[%s]" "${(t)#v}"`, "[10]"},
		{"an operator applies to it", `v=abc; printf "[%s]" "${(t)v#KIND}"`, "[scalar]"},
		{"whose test does not fire on a name that is set", `v=abc; printf "[%s]" "${(t)v:-D}"`, "[KINDscalar]"},
		{"a subscript reads the word's characters", `w=(a b); printf "[%s]" "${(t)w[5]}" "${(t)w[5,9]}"`, "[a][array]"},
		{"a whole-array subscript is the whole word", `w=(a b); printf "[%s]" "${(t)w[@]}"`, "[KINDarray]"},
		{"the set test still answers about the name", `v=abc; unset u; printf "[%s]" "${(t)+v}" "${(t)+u}"`, "[1][0]"},
		{"behind an indirection it is the name that was resolved", `w=(a b); h=w; printf "[%s]" "${(Pt)h}"`, "[KINDarray]"},
		{"a nested inner is a value, which has no name to describe", `s=hello; printf "[%s]" "${(t)${s}}"`, "[hello]"},

		// An unset name is the empty word and *unset* with it, which is the
		// row a flag that always described something gets wrong: the
		// colon-less test has to fire.
		{"an unset name is empty", `unset u; printf "[%s]" "${(t)u}"`, "[]"},
		{"and unset", `unset u; printf "[%s]" "${(t)u-D}"`, "[D]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := typeFlagRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q status %d (stderr %q), want %q at 0", tc.src, out, st, errs, tc.want)
			}
		})
	}
}

// A special parameter is refused by name rather than answered empty.
//
// The shell has `$@` and `$0`; what it does not have is the facts they would
// be described from. Empty is the word for a name the shell does **not**
// have, so answering it here would make `${(t)0-D}` substitute `D` for a
// parameter every shell in the panel provides — a plausible word at status 0.
func TestTheTypeFlagRefusesASpecialParameterByName(t *testing.T) {
	for _, src := range []string{
		`printf "[%s]" "${(t)0}"`,
		`set -- a; printf "[%s]" "${(t)1}"`,
		`printf "[%s]" "${(t)@}"`,
		`printf "[%s]" "${(t)#}"`,
	} {
		out, errs, st := typeFlagRun(t, src)
		if st == 0 {
			t.Errorf("%s = %q at status 0, want a refusal", src, out)
		}
		if !strings.Contains(errs, "the (t) expansion flag is not implemented for a special parameter") {
			t.Errorf("%s: stderr = %q, want the flag and the shape named", src, errs)
		}
	}
}
