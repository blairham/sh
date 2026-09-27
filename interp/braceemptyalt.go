// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A brace alternative that came to nothing is a field in two of the four
// columns that have braces at all, and is removed in the other two.
//
// `f(){ printf '%d |' $#; for x in "$@"; do printf ' [%s]' "$x"; done; }` over
// `-c`, measured 2026-09-27 against /opt/homebrew/bin/zsh 5.9.2 run `-f`,
// /bin/ksh `Version AJM 93u+ 2012-08-01`, /opt/homebrew/bin/bash 5.3.20 and
// /bin/bash 3.2.57 — `go version -m` says *not a Go executable* for each:
//
//	                       bash 5.3 / 3.2     ksh93u+ / zsh 5.9.2
//	f {a,b}     (control)  2 | [a] [b]        2 | [a] [b]
//	f {a,}                 1 | [a]            2 | [a] []
//	f {,a}                 1 | [a]            2 | [] [a]
//	f {,}                  0 |                2 | [] []
//	unset u; f {a,$u}      1 | [a]            2 | [a] []
//	unset u; f {$u,a}      1 | [a]            2 | [] [a]
//	unset u; f {a,$u,b}    2 | [a] [b]        3 | [a] [] [b]
//	f {a,$(true)}          1 | [a]            2 | [a] []
//
// #4800 filed the row as ksh93's alone, on a probe set that reached only the
// *written* half; zsh answers it the same way and does so on the produced half
// too, so the axis has the two columns every other structural brace question
// here has. The control says the probe can see the columns agree, and it saw
// these seven not agree.
//
// **The question is about the alternative and not about the word**, which two
// rows hold apart. `unset u; f {a,b}$u` is `2 | [a] [b]` in every column —
// the alternatives are text, so nothing came to nothing. And `a=(); f
// {p,q}${^a}` is **no word at all** in zsh, where a distributive span with no
// elements takes the word with it: the alternative is still text there and the
// word went away for a reason of its own, so this rule must not put it back.
// That is why the shape asked about is a name whose *whole* expansion came to
// a single field that nothing reached — the only way to arrive at which is for
// the alternative to have been the empty part — rather than "a name that
// produced no field", which would answer that zsh row `2 | [p] [q]` and be
// wrong about it.
//
// `f {a,""}` is `2 | [a] []` in every column including this one already, and
// is the mechanism rather than the exception: a quoted null is a field the
// word keeps wherever it stands. What the axis adds is that in two columns an
// alternative that came to nothing is that same quoted null, written or not.
//
// See Semantics.BraceEmptyAlternativeIsAField.

// braceNameCameToNothing reports whether the word being expanded is a name a
// brace fan made whose expansion came to a single field that nothing reached
// — and, where it did, puts the axis.
//
// It is asked at the disagreement and nowhere else: a name that produced a
// field of any kind, and every word that is not a brace name at all, never
// reach it, so a vector that has not answered is not refused over a word no
// column disagrees about.
func (r *Runner) braceNameCameToNothing(b *wordFields, armed bool) bool {
	if !armed || b.noFields || len(b.all) != 1 || b.all[0] != "" {
		return false
	}
	return r.askBrace(r.sem().BraceEmptyAlternativeIsAField,
		"a brace alternative that came to nothing being a field of its own")
}
