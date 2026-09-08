// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// The `(~)` expansion flag, which marks the string arguments of the flags
// written behind it in the same group.
//
// It is **not** the flag-group spelling of `${~name}`, and that is the thing
// these tests exist to hold: the written tilde makes the whole substituted
// string a pattern, where this one makes only the separator the group
// *inserts* one and leaves the value between the separators literal. Reading
// it as the other would pass a row asking whether anything matched and fail
// every row below that asks *what* matched.
//
// The grammar and the answers are named as axes rather than as a shell.
// GlobExpansionResults is No because that is where the two readings differ at
// all: where every expansion result is already a pattern there is nothing for
// the flag to exempt, which the last test asserts from the other side.

func tildeGroupRun(t *testing.T, src string, glob Answer) (string, string, int) {
	t.Helper()
	d := syntax.Core()
	d.ParamExpansionFlags = true
	d.ArrayLiteral = true
	d.ArraySubscript = true
	// A bare `( … )` in a pattern is what gives an inserted `|` somewhere to
	// mean something. Without groups the flag has no observable effect at
	// all, which is itself an answer and is why the grammar is named here.
	d.PatternAlternation = true
	// The written `~` and `=` that share the slot behind the group: without
	// them a `${(…)=name}` is read as an assignment operator rather than as
	// the split it is, and the row about a split beside the mark would be
	// measuring the wrong refusal.
	d.ParamTildeFlag = true
	d.ParamSplitFlag = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.SplitParamExpansion = No
	sem.GlobExpansionResults = glob
	sem.FatalErrorStatusIsOne = Yes
	sem.DeclaredNameWithoutValueIsEmpty = Yes
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Dialect: &d, Semantics: &sem, Name: "testsh",
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

// The separator the group inserts is a pattern and the words around it are
// not, which is one string that neither of the two ordinary answers can be.
func TestTheTildeGroupFlagMarksTheSeparatorAndNotTheValue(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The manual's own example, asked from both sides. `?` is an
		// element, so the alternation matches it; `q` is not, so a `?`
		// read as a live metacharacter would match it and must not.
		{
			"the inserted separator is an alternation",
			`a=('?' 'x'); case 'z?' in (z(${(~j.|.)a})) printf hit;; *) printf miss;; esac`,
			"hit",
		},
		{
			"and the element beside it is literal",
			`a=('?' 'x'); case zq in (z(${(~j.|.)a})) printf hit;; *) printf miss;; esac`,
			"miss",
		},
		// The control: without the flag the whole joined text is one
		// literal, so it matches itself and nothing else.
		{
			"without the flag the join is text",
			`a=('?' 'x'); case 'z?|x' in (z(${(j.|.)a})) printf hit;; *) printf miss;; esac`,
			"hit",
		},
		{
			"and matches no alternative",
			`a=('?' 'x'); case 'z?' in (z(${(j.|.)a})) printf hit;; *) printf miss;; esac`,
			"miss",
		},
		// The row that separates this flag from `${~name}`: a star *in an
		// element* stays a star. `${~name}` answers hit here.
		{
			"a metacharacter in an element stays one character",
			`b=('a*' 'x'); case zabc in (z(${(~j.|.)b})) printf hit;; *) printf miss;; esac`,
			"miss",
		},
		{
			"and matches itself",
			`b=('a*' 'x'); case 'za*' in (z(${(~j.|.)b})) printf hit;; *) printf miss;; esac`,
			"hit",
		},
		// And the same from the other end: with no flag behind it to mark,
		// the tilde marks nothing whatever the value holds.
		{
			"a tilde with nothing behind it marks nothing",
			`p='a|b'; case za in (z(${(~)p})) printf hit;; *) printf miss;; esac`,
			"miss",
		},
		{
			"and leaves the value matching itself",
			`p='a|b'; case 'za|b' in (z(${(~)p})) printf hit;; *) printf miss;; esac`,
			"hit",
		},
		// Quoting does not suppress it, which is the opposite of what the
		// written tilde does and had to be measured rather than carried
		// over: a quoted `${~name}` has no answer of its own at all.
		{
			"quoting does not suppress the mark",
			`a=('?' 'x'); case 'z?' in (z("${(~j.|.)a}")) printf hit;; *) printf miss;; esac`,
			"hit",
		},
		// The tilde *half* of `${~name}` is not this flag's. A value whose
		// leading tilde became a home directory here would be the written
		// flag's implementation reached by the wrong route.
		{
			"the flag does not expand a tilde out of a value",
			`t='~/zz'; printf "[%s]" "${(~)t}"`,
			"[~/zz]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, _ := tildeGroupRun(t, tc.src, No)
			if errs != "" {
				t.Fatalf("stderr = %q, want none", errs)
			}
			if out != tc.want {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
		})
	}
}

// The mark reaches the flags written *behind* the tilde, and the count is
// parity — the same reading `${~~name}` has, arrived at independently.
func TestTheTildeGroupFlagMarksOnlyWhatFollowsIt(t *testing.T) {
	// One arm, one flag group substituted into it, so the rows differ in
	// nothing but the thing they are about.
	arm := func(flags string) string {
		return `a=('?' 'x'); case 'z?' in (z(${(` + flags +
			`)a})) printf on;; *) printf off;; esac`
	}
	for _, tc := range []struct{ name, flags, want string }{
		{"in front of the flag", `~j.|.`, "on"},
		{"with another flag between them", `U~j.|.`, "on"},
		{"behind the flag it marks nothing", `j.|.~`, "off"},
		{"two tildes are none", `~~j.|.`, "off"},
		{"three are one again", `~~~j.|.`, "on"},
		{"and the count is taken where the flag stands", `~j.|.~`, "on"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, _ := tildeGroupRun(t, arm(tc.flags), No)
			if errs != "" {
				t.Fatalf("stderr = %q, want none", errs)
			}
			if out != tc.want {
				t.Errorf("flags (%s): output = %q, want %q", tc.flags, out, tc.want)
			}
		})
	}
}

// Where the join stands among the other steps, asserted as text.
//
// The shell being modeled joins and then marks the separator so every later
// step steps over it; this one holds the join back instead, which is the same
// answer wherever the step rewrites text one character at a time. Each half
// has a measurement: `${(~qj.|.)b}` on `('a b' 'c')` is `a\ b|c` under either
// order, `q` marking one character at a time and never the bar; and
// `${(~oj.|.)c}` on `(x a)` is `x|a`, unsorted, because the sort has one word
// by then — which is also what the same group without the tilde answers, and
// what `(u)` answers for the same reason.
func TestTheTildeGroupFlagJoinsAfterQuotingAndBeforeOrdering(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the quoting flag runs over the words", `b=('a b' c); printf "[%s]" "${(~qj.|.)b}"`, `[a\ b|c]`},
		{"and the tilde may stand behind it", `b=('a b' c); printf "[%s]" "${(q~j.|.)b}"`, `[a\ b|c]`},
		{"the ordering flag sees the join", `c=(x a); printf "[%s]" "${(~oj.|.)c}"`, "[x|a]"},
		{"and does not sort without the tilde either", `c=(x a); printf "[%s]" "${(oj.|.)c}"`, "[x|a]"},
		{"the dedup sees it too", `e=(p p q); printf "[%s]" "${(~uj.|.)e}"`, "[p|p|q]"},
		{"exactly as it does without the tilde", `e=(p p q); printf "[%s]" "${(uj.|.)e}"`, "[p|p|q]"},
		{"a scalar inserts no separator", `s='a|b'; case za in (z(${(~j.|.)s})) printf hit;; *) printf miss;; esac`, "miss"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, _ := tildeGroupRun(t, tc.src, No)
			if errs != "" {
				t.Fatalf("stderr = %q, want none", errs)
			}
			if out != tc.want {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
		})
	}
}

// The compositions this interpreter cannot hold a marked separator through
// are named rather than carried, and the whole rendered line is asserted.
//
// A status is not enough to assert here and the reason is specific: the
// refused expansion and the carried one both leave a `case` at status 0, so a
// test reading the status alone passes on an implementation that ignored the
// flag — which is the failure this flag was found by in the first place.
func TestTheTildeGroupFlagIsRefusedWhereItCannotBeCarried(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an operator would read the join back",
			`a=(p q); printf "[%s]" "${(~j.|.)a:-z}"`,
			"testsh: ${(~j.|.)a:-z}: the (~) expansion flag is not implemented beside an operator\n",
		},
		{
			"and so would a trim",
			`a=(p q); printf "[%s]" "${(~j.|.)a#p}"`,
			"testsh: ${(~j.|.)a#p}: the (~) expansion flag is not implemented beside an operator\n",
		},
		{
			"a split takes the separator out again",
			`a=(p q); printf "[%s]" "${(~fj.|.)a}"`,
			"testsh: ${(~fj.|.)a}: the (~) expansion flag is not implemented beside a split\n",
		},
		{
			"an IFS split beside the group says the same",
			`a=(p q); printf "[%s]" ${(~j.|.)=a}`,
			"testsh: ${(~j.|.)=a}: the (~) expansion flag is not implemented beside a split\n",
		},
		{
			"and where both stand the split is the one named",
			`a=(p q); printf "[%s]" ${(~j.|.)=a:-z}`,
			"testsh: ${(~j.|.)=a:-z}: the (~) expansion flag is not implemented beside a split\n",
		},
		{
			"a second (q) wraps the join rather than each word",
			`d=('a b' c); printf "[%s]" "${(~qqj.|.)d}"`,
			"testsh: ${(~qqj.|.)d}: the (~) expansion flag is not implemented beside the (qq) flag\n",
		},
		{
			// Minimal quoting wraps a whole word, so it is in that family
			// despite being spelled with a single `q` — measured,
			// `${(~q-j.|.)d}` is `'a b|c'`, one pair of quotes round the
			// join, where quoting each word on its own gives `'a b'|c`. The
			// row exists because the count is what names the others, and a
			// count of one is exactly what this group has.
			"and so does the minimal quoting, on one q",
			`d=('a b' c); printf "[%s]" "${(~q-j.|.)d}"`,
			"testsh: ${(~q-j.|.)d}: the (~) expansion flag is not implemented beside the (q-) flag\n",
		},
		{
			// And the extended form, which is the same family and the
			// further case: measured, `${(~q+j.|.)d}` is `'a b|c'` too, and
			// a value it renders would put one `$'…'` round the whole join,
			// which no per-word rewrite could produce at all. It has one
			// `q` like the one above, so a count would carry it.
			"and the extended form of it, on one q as well",
			`d=('a b' c); printf "[%s]" "${(~q+j.|.)d}"`,
			"testsh: ${(~q+j.|.)d}: the (~) expansion flag is not implemented beside the (q+) flag\n",
		},
		{
			"and the unquoting takes a level off the join",
			`k=("'a" "b'"); printf "[%s]" "${(~Qj.|.)k}"`,
			"testsh: ${(~Qj.|.)k}: the (~) expansion flag is not implemented beside the (Q) flag\n",
		},
		{
			"the prompt escapes read across the separator",
			`m=('%%x' y); printf "[%s]" "${(~%j.|.)m}"`,
			"testsh: ${(~%j.|.)m}: the (~) expansion flag is not implemented beside the (%) flag\n",
		},
		{
			"and the split separator is named for itself",
			`v=a-b; printf "[%s]" "${(~s.-.)v}"`,
			"testsh: ${(~s.-.)v}: the (~) expansion flag is not implemented for the (s) separator\n",
		},
		{
			"a tilde behind the split marks nothing and is carried",
			`v=a-b; printf "[%s]" "${(s.-.~)v}"`,
			"",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := tildeGroupRun(t, tc.src, No)
			if errs != tc.want {
				t.Errorf("stderr = %q, want %q", errs, tc.want)
			}
			if tc.want == "" {
				return
			}
			if out != "" {
				t.Errorf("output = %q, want nothing substituted", out)
			}
			if st == 0 {
				t.Errorf("status 0, want the refusal to carry one")
			}
		})
	}
}

// Where the dialect reads every expansion result as a pattern the flag has
// nothing left to exempt, and the answer is the one the option gives.
//
// The row that says the flag is an *override of the escape* rather than a
// mechanism of its own: with the axis at Yes the element's star is live too,
// which the same line at No refuses.
func TestTheTildeGroupFlagExemptsNothingWhereEverythingIsAPattern(t *testing.T) {
	const src = `b=('a*' 'x'); case zabc in (z(${(~j.|.)b})) printf hit;; *) printf miss;; esac`
	if out, _, _ := tildeGroupRun(t, src, Yes); out != "hit" {
		t.Errorf("globbing results: output = %q, want %q", out, "hit")
	}
	if out, _, _ := tildeGroupRun(t, src, No); out != "miss" {
		t.Errorf("not globbing results: output = %q, want %q", out, "miss")
	}
}
