// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A chain of subscripts read as a walk **into** a nested compound —
// `${a[1][2]}` where element 1 holds an array of its own.
//
// The other reading of the same text is the one syntax.Dialect.ChainedSubscript
// was written for: there a subscript counts *characters* when it is handed one
// string and *elements* when it is handed a list, so `a=(one two three);
// ${a[1][2]}` is `n`. Giving this dialect that reading would answer a
// plausible wrong character at status 0, which is the failure this repository
// has the axis mechanism for — it answers the same row with **empty**, because
// element 0 holds a string and a second subscript on it reaches a nested array
// that is not there.
//
// Measured 2026-09-15 on ksh93u+ 2012-08-01, `env -i PATH=/usr/bin:/bin` with
// a scratch HOME, from a script file, with `a[1]=(p q); a[2]=plain`:
//
//	${a[1][0]}     p          the nested array, indexed
//	${a[1][1]}     q
//	${a[1][9]}     empty      and unset: `${a[1][9]-none}` is `none`
//	${a[1][@]}     p, q       two fields
//	${a[1][*]}     `p q`      one
//	${#a[1][@]}    2
//	${a[2][0]}     plain      a string element answers at the base and
//	${a[2][1]}     empty      nowhere else — `${a[2][1]-none}` is `none`
//	${a[2][@]}     nothing    zero fields, where the *name* `${s[@]}` on a
//	                          scalar is one
//	${a[9][0]}     empty      a link that named nothing ends the walk
//
// and with `c[1][2][3]=v`, which the write half of this already builds:
//
//	${c[1][2][3]}  v          so the chain is a walk of any depth
//	${c[1][2]}     empty      a nested element read as one string is its own
//	${c[1]}        empty      first element, and element 0 is not there
//	${c[1][2][@]}  v
//
// and with `typeset -A m; m[k]=(x y)`, which is the row that says only the
// first subscript can be a key:
//
//	${m[k][1]}     y
//	${#m[k][@]}    2
//
// The write half landed in #2491 and is interp/chainassign.go; the value it
// builds was reachable only through `typeset -p` until this (#2830).

// nestedChainSubscript walks the chain and answers the last subscript.
//
// nil rather than an empty slice is what says the expansion is *unset*, which
// is the distinction `${a[9][0]-none}` turns on; an empty slice is a whole-
// array subscript that named no elements, which is set and has none.
func (r *Runner) nestedChainSubscript(e *syntax.ParamExpr) ([]string, bool) {
	held, there := r.nestedChainRoot(e)
	for i := 1; i < len(e.Leading) && there; i++ {
		idx, ok := r.chainLinkIndex(r.chainLink(e, i))
		if !ok {
			return nil, true
		}
		held, there = r.nestedElementAt(held, idx)
	}
	if !there {
		return nil, true
	}
	if r.wholeArrayIndex(e) {
		// `[@]` and `[*]` name the whole of the nested array. The join that
		// separates them is the caller's, exactly as it is for a name.
		return r.nestedElements(held), true
	}
	idx, ok := r.chainLinkIndex(e)
	if !ok {
		return nil, true
	}
	last, found := r.nestedElementAt(held, idx)
	if !found {
		return nil, true
	}
	return []string{r.elemText(last)}, true
}

// nestedChainRoot resolves the first subscript against the *name*, which is
// the one link that is not an element of an array.
//
// The same split the write half makes, and for the same reason: a declared
// table takes its subscript as a key, expanded and never evaluated, and every
// link below it indexes an array whatever the name's attribute is. Measured,
// `typeset -A m; m[k]=(x y); ${m[k][1]}` is `y`.
//
// A name holding a plain string answers at the base and nowhere else, which is
// scalarElemAt's rule one level up: `s=abc; ${s[0][0]}` reaches the string
// through the base and `${s[1][0]}` reaches nothing.
func (r *Runner) nestedChainRoot(e *syntax.ParamExpr) (Element, bool) {
	name := r.throughNamerefName(e.Name)
	first := r.chainLink(e, 0)
	first.Name = name
	if r.assocDeclared(name) {
		tbl, ok := r.assocFor(name)
		if !ok {
			return Element{}, false
		}
		held, there := tbl[r.joinWord(first.Index)]
		return held, there
	}
	idx, ok := r.chainLinkIndex(first)
	if !ok {
		return Element{}, false
	}
	if arr, is := r.Arrays[name]; is {
		held, there := arr[idx]
		return held, there
	}
	if v, held := r.getVar(name); held {
		if s, ok := scalarElemAt(v, idx, r.arrayBase()); ok {
			return Scalar(s), true
		}
	}
	return Element{}, false
}

// chainLinkIndex evaluates one link's subscript as arithmetic.
//
// The same three calls subscriptOver makes for a subscript written on a name,
// so a complaint about an unreadable one quotes back the text a plain
// `${a[b c]}` would — the expression reader is where that wording lives, and a
// second reading of a subscript here is how the two would come to disagree.
func (r *Runner) chainLinkIndex(e *syntax.ParamExpr) (int, bool) {
	written := r.subscriptTextAsWritten(e.Subscript())
	text := trimSubscript(written)
	return r.subscriptIndexAsWritten(r.writtenSubscript(e, text), written)
}

// nestedElementAt is one step of the walk: what index i of an element holds.
//
// An element holding an array is indexed. An element holding a *string* is the
// string at the base and nothing anywhere else — measured, `a[2]=plain` gives
// `plain` for `${a[2][0]}` and an unset expansion for `${a[2][1]}` — which is
// scalarElemAt's rule, reached one level down from where a subscript on a
// scalar name reaches it.
func (r *Runner) nestedElementAt(held Element, idx int) (Element, bool) {
	if held.Nested != nil {
		el, there := held.Nested[idx]
		return el, there
	}
	// The element's own text at the base and nowhere else, which is the rule
	// a *name* holding a string already follows — and a compound answers by
	// the same rule with the text it renders: measured on ksh93u+
	// 2012-08-01, `a[1]=(p=1 q=2)` then `${a[1][0]}` is the whole compound
	// and `${a[1][1]}` is nothing, exactly as `a[1]=plain` answers `plain`
	// and nothing. Str is a namespace rather than a value for that kind, so
	// the text has to come from elemText. See interp/subcompound.go.
	if s, ok := scalarElemAt(r.elemText(held), idx, r.arrayBase()); ok {
		return Scalar(s), true
	}
	return Element{}, false
}

// nestedElements is the whole of what an element holds, for `[@]` and `[*]`.
//
// An element holding a string has **no** elements here, which is measured and
// is not the same answer a scalar *name* gives: `a[2]=plain; ${#a[2][@]}` is 0
// where `s=abc; ${#s[@]}` is 1. An empty slice and not nil, because the
// expansion is set and has nothing in it rather than being unset.
func (r *Runner) nestedElements(held Element) []string {
	if held.Nested == nil {
		return []string{}
	}
	subs := held.Nested.subscripts()
	out := make([]string, 0, len(subs))
	for _, i := range subs {
		out = append(out, held.Nested[i].scalar())
	}
	return out
}
