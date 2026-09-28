// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `~(…)` group asking for an **anchor** is read where it stands.
//
// `l` and `r` pin the match to an end of the subject, which is the comparison
// matchTilde already made for a group at the front and tildeHereAnchors now
// makes for one further along. The rows are ksh93u+ 2012-08-01's, measured
// 2026-09-27; nothing here names a shell, because the construct is a grammar
// flag and the letters are that construct's.
func TestATildeAnchorGroupInTheMiddleOfAPattern(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The controls. A group at the front is read — that is the reading
		// this one is being held against — and a letter no shell has cannot
		// match, which is what says a `~(…)` is being *read* here rather
		// than every one of them being stepped over.
		{"an anchor at the front", `[[ zab == ~(l)zab ]] && echo YES || echo NO`, "YES"},
		{"and the other one", `[[ zab == ~(r)zab ]] && echo YES || echo NO`, "YES"},
		{
			"a letter no shell has still cannot match",
			`[[ zab == z~(Z)ab ]] && echo YES || echo NO`, "NO",
		},

		// A whole-subject comparison satisfies both anchors, so what these
		// rows show is the group being **consumed** rather than standing as
		// the four characters it was written with.
		{"one in the middle", `[[ zab == z~(l)ab ]] && echo YES || echo NO`, "YES"},
		{"the right anchor too", `[[ zab == z~(r)ab ]] && echo YES || echo NO`, "YES"},
		{"both at once", `[[ zab == z~(lr)ab ]] && echo YES || echo NO`, "YES"},
		{"turned off", `[[ zab == z~(-l)ab ]] && echo YES || echo NO`, "YES"},
		{"behind a star", `[[ zab == *~(l)ab ]] && echo YES || echo NO`, "YES"},
		{"inside a pattern group", `[[ zab == @(z~(l)ab) ]] && echo YES || echo NO`, "YES"},
		{"and the pattern still has to match", `[[ zabc == z~(l)ab ]] && echo YES || echo NO`, "NO"},

		// A **trim** is where the anchors have something to refuse, because
		// the piece it hands over is a prefix rather than the whole subject.
		// The pair is what makes each row readable: the same pattern one
		// character shorter answers the other way.
		{"a prefix trim reaching the end", `v=abcd; printf "[%s]" "${v#a~(r)bcd}"`, "[]"},
		{"and one that does not", `v=abcd; printf "[%s]" "${v#a~(r)bc}"`, "[abcd]"},
		{"a prefix trim starts the subject", `v=abcd; printf "[%s]" "${v#a~(l)bc}"`, "[d]"},
		{"a substitution refuses the same way", `v=abcd; printf "[%s]" "${v/a~(r)bc/X}"`, "[abcd]"},
		{"and takes the span that reaches the end", `v=abcd; printf "[%s]" "${v/a~(r)bcd/X}"`, "[X]"},

		// **A substitution is where `l` has something to refuse**, because
		// the span it chooses need not start the subject. The three rows are
		// one probe: the span alone is replaced, `l` refuses it, and `-l`
		// takes it back — so each answer is known to be reachable.
		{"a span that does not start the subject", `v=abcd; printf "[%s]" "${v/bc/X}"`, "[aXd]"},
		{"and the left anchor refuses it", `v=abcd; printf "[%s]" "${v/b~(l)c/X}"`, "[abcd]"},
		{"where the sign takes it back", `v=abcd; printf "[%s]" "${v/b~(-l)c/X}"`, "[aXd]"},

		// A letter ksh93 has and this shell does not is refused **by name**
		// wherever the group stands, so there is no answer for a row here to
		// assert: see unhonoredTildeLetter, and dialect/ksh's own test for
		// the sentence and the status (#4914). A letter no ksh93 has is a
		// different question and keeps its row above.
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tildeMid(t, tc.src, nil); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// `~(g)` makes a **prefix trim** take the longest piece its pattern will
// match, and is read where it stands rather than only at the front.
//
// A trim is the one surface that can show it: a whole-subject match has no
// length to choose. Every row is against `v=aXbXc`, where `${v#*X}` is `bXc`
// and `${v##*X}` is `c` — so a row answering `c` is the greed being honored
// and one answering `bXc` is it not being, and neither is the subject
// happening to agree.
func TestATildeGreedGroupInTheMiddleOfATrim(t *testing.T) {
	const v = `v=aXbXc; printf "[%s]" `
	for _, tc := range []struct{ name, src, want string }{
		// The three controls, so that both answers are known to be
		// reachable before any row below is read.
		{"the plain trim", v + `"${v#*X}"`, "[bXc]"},
		{"the doubled one", v + `"${v##*X}"`, "[c]"},
		{"a group at the front", v + `"${v#~(g)*X}"`, "[c]"},

		{"one behind a literal", v + `"${v#a~(g)*X}"`, "[c]"},
		{"behind a bracket expression", v + `"${v#[aX]~(g)*X}"`, "[c]"},
		{"behind a pattern group", v + `"${v#@(a|q)~(g)*X}"`, "[c]"},
		{"behind a negated group", v + `"${v#!(q)~(g)*X}"`, "[c]"},
		{"with the star behind it", v + `"${v#a~(g)*}"`, "[]"},

		// **A bare wildcard in front takes the greed away**, and the two
		// spellings that only look like one are what say the noun is a
		// wildcard rather than the character: `*(q)` and `?(q)` are a
		// group's quantifier, and both stay greedy.
		{"a bare star in front", v + `"${v#*~(g)X}"`, "[bXc]"},
		{"a bare question in front", v + `"${v#?~(g)*X}"`, "[bXc]"},
		{"a star further back", v + `"${v#a*~(g)X}"`, "[bXc]"},
		{"nothing behind it at all", v + `"${v#*X~(g)}"`, "[bXc]"},
		{"a star that is a quantifier", v + `"${v#*(q)~(g)*X}"`, "[c]"},
		{"a question that is one", v + `"${v#?(q)~(g)*X}"`, "[c]"},
		{"a star inside a group's body", v + `"${v#@(a*)~(g)*X}"`, "[c]"},
		{"and one inside a bracket", v + `"${v#[a*]~(g)*X}"`, "[c]"},

		// The sense sign, and a group that says nothing about greed leaving
		// an earlier one's request alone.
		{"turned off where it stands", v + `"${v#a~(-g)*X}"`, "[bXc]"},
		{"turned on and off again", v + `"${v#~(g)a~(-g)*X}"`, "[bXc]"},
		{"a later group that is silent about it", v + `"${v#~(g)a~(i)*X}"`, "[c]"},

		// And a **suffix** trim is pinned at the far end already, so there
		// is nothing for greed to lengthen.
		{"a suffix trim is unmoved", v + `"${v%X~(g)*}"`, "[aXb]"},
		{"as it is without the group", v + `"${v%X*}"`, "[aXb]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tildeMid(t, tc.src, nil); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// Pathname expansion reaches the same letters, which takes one thing more
// than the matcher: the gate at the top of the walk has to read the field as
// a pattern at all. See holdsTildeFoldGroup.
func TestATildeAnchorGroupInTheMiddleOfAGlob(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"za", "zb"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ name, src, want string }{
		// The control: a field with no group in it and no metacharacter is
		// a name rather than a pattern, so a missing one stands as written.
		{"a name that is not there", `printf "[%s]" zq`, `[zq]`},

		{"an anchor in the middle", `printf "[%s]" z~(l)a`, `[za]`},
		{"the other one", `printf "[%s]" z~(r)a`, `[za]`},
		{"the greed letter", `printf "[%s]" z~(g)a`, `[za]`},

		// `N` is consumed here and asks for nothing: it deletes the word at
		// the **front** only, so a field whose group is further along and
		// whose pattern names nothing stands as written rather than
		// vanishing.
		{"the null letter is consumed", `printf "[%s]" z~(N)a`, `[za]`},
		{"and does not delete the word", `printf "[%s]" z~(N)z*`, `[z~(N)z*]`},
		// The word is gone, so `printf` runs its format once with nothing to
		// put in it — `[]` rather than `[~(N)zz*]`, which is what the row
		// above answers.
		{"where one at the front does", `printf "[%s]" ~(N)zz*`, `[]`},

		// A letter this shell declines is still the text it was written as
		// **here**, where the same letter in a condition is refused by name.
		// That is not the two routes disagreeing: a field whose group stands
		// anywhere but the head is not made a pattern at all — see
		// tildeGlobPattern, which asks for a group at the front — so nothing
		// reaches the matcher for the refusal to be raised from. `~(M)zab`
		// at the head does reach it and does stop the script.
		{"a letter this shell declines", `printf "[%s]" z~(M)a`, `[z~(M)a]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tildeMid(t, tc.src, func(r *Runner) { r.Dir = dir })
			if got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
