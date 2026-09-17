// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"sort"
	"strings"

	"github.com/blairham/sh/syntax"
)

// dotRanged reports whether an expansion's subscript is a `lo..hi` range the
// grammar separated — see syntax.Dialect.SubscriptDotRange. Only the last
// subscript of an expansion with no chain in front of it: a range inside a
// chain has no reading here, and its text is left to the arithmetic that
// refuses it.
func dotRanged(e *syntax.ParamExpr) bool {
	return e != nil && e.IndexDots != nil && len(e.Leading) == 0
}

// dotRangeSubscript answers `${a[lo..hi]}`: the elements whose subscripts lie
// from lo through hi, as a list — one field each when quoted, exactly as
// `"${a[@]}"` is — or their subscripts under `${!a[lo..hi]}`.
//
// Only one shell has the construct, and what it names is measured rather than
// derived, because several of the rules are not the obvious ones. Measured
// 2026-09-16 on ksh93u+ 2012-08-01, `a=(a b c d e)` unless noted:
//
//	${a[1..3]}               b c d      three fields in quotes
//	${a[i..i+2]}  i=1        b c d      the ends are arithmetic
//	${a[..2]}                a b c      an empty end is 0
//	${a[2..]}                c          so this is 2..0
//	${a[3..1]}               d          a reversed range is its first element
//	${a[1..9]}               b c d e    a high end past the last is the last
//	${a[-3..-1]}             c d e      a negative end counts from the end
//	${a[1..-9]}              b          and past the start is below lo
//	${a[-9..1]}              a: subscript out of range, and the line ends
//	${a[5..9]}               a          nothing at or above lo: element 0
//	${!a[5..9]}              0
//	a[2]=c a[5]=f ${a[3..4]} f          the first *set* subscript at or above
//	a[2]=c a[5]=f ${a[0..9]} c f        lo, then every set one through hi
//	a[2]=c a[5]=f ${a[6..9]} (empty)    element 0, which is not set
//	s=hello ${s[0..3]}       hello      a string is an array of one
//	s=hello ${s[1..3]}       (empty)    and has no element 0 fallback
//	${#a[1..3]}              "${#a[1..3]}": bad substitution
//
// So the first element is always taken — the one at or above lo — and the
// high end only decides whether any *more* follow it. That one rule is the
// reversed range, the `2..` spelling and the sparse rows at once.
//
// An association ranges over its keys in order, by the same rule, with the
// ends compared as strings: keys a b c d give `2 3` for `b..c`, `3` for
// `bb..c`, `1 2` for `..b` and nothing for `e..f`.
func (r *Runner) dotRangeSubscript(e *syntax.ParamExpr) []string {
	if e.Length {
		// A count is not something the range has: refused at the run, so a
		// branch never taken says nothing, measured.
		r.reportBadSubstitution(e)
		return nil
	}
	if r.assocDeclared(e.Name) {
		return r.dotRangeOfTable(e)
	}
	lo, ok := r.dotRangeEnd(e.IndexDots.Lo.Text)
	if !ok {
		return nil
	}
	hi, ok := r.dotRangeEnd(e.IndexDots.Hi.Text)
	if !ok {
		return nil
	}
	elems, scalar, ok := r.subscriptTarget(e)
	if !ok {
		return nil
	}
	base := r.arrayBase()
	if scalar {
		// One place, at the base, and only a range that begins there reaches
		// it: `s[..]` is the value and `s[1..3]` and `s[-1..0]` are nothing.
		if lo != base {
			return nil
		}
		if e.Indirect {
			return []string{itoa(base)}
		}
		return []string{elems[0]}
	}
	subs, vals := r.dotRangeElements(e.Name, elems)
	end := base
	if len(subs) > 0 {
		end = subs[len(subs)-1] + 1
	}
	if lo < 0 {
		if lo += end; lo < 0 {
			r.diagf("%s\n", Wording(r.diag().BadArraySubscript,
				"%[1]s[%[2]s]: bad array subscript", e.Name, e.IndexText))
			r.expandErr = true
			return nil
		}
	}
	if hi < 0 {
		hi += end
	}
	limit := -1
	if e.Op == syntax.ParamSubstring {
		// The offset is not counted from the range's first element. Measured
		// on the same build: a non-zero offset *replaces* lo and keeps hi, and
		// finds nothing rather than element 0 when nothing is at or above it;
		// the length counts elements from there.
		//
		//	${a[1..3]:2}     c d       ${a[2..4]:1}     b c d e
		//	${a[1..3]:0}     b c d     ${a[1..3]:4}     e
		//	${a[1..3]:1:2}   b c       ${a[1..3]: -1}   e
		//	${a[1..3]:9}     (empty)   ${a[1..3]:1:0}   (empty)
		off := r.numOf(e.Arg, e, e.Arg2)
		if r.rangeRefused || r.expandErr {
			return nil
		}
		if e.Arg2 != nil {
			if limit = r.numOf(e.Arg2, e, nil); r.rangeRefused || r.expandErr {
				return nil
			}
			if limit <= 0 {
				return nil
			}
		}
		if off != 0 {
			if off < 0 {
				off = max(off+end, 0)
			}
			first := sort.SearchInts(subs, off)
			if first == len(subs) {
				return nil
			}
			return dotRangePick(e, subs, vals, first, hi, limit)
		}
	}
	first := sort.SearchInts(subs, lo)
	if first == len(subs) {
		if e.Indirect {
			return []string{itoa(base)}
		}
		if len(subs) > 0 && subs[0] == base {
			return []string{vals[0]}
		}
		return nil
	}
	return dotRangePick(e, subs, vals, first, hi, limit)
}

// dotRangePick takes the element at first and every one after it whose
// subscript is at most hi, up to limit of them where limit is not negative.
func dotRangePick(e *syntax.ParamExpr, subs []int, vals []string, first, hi, limit int) []string {
	var out []string
	for i := first; i < len(subs); i++ {
		if i > first && subs[i] > hi {
			break
		}
		if limit >= 0 && len(out) == limit {
			break
		}
		if e.Indirect {
			out = append(out, itoa(subs[i]))
		} else {
			out = append(out, vals[i])
		}
	}
	return out
}

// dotRangeElements is every set subscript of a list in order, beside its
// value. A stored array answers from its store, so a sparse one keeps its
// gaps; anything else — a produced list, the positional parameters — is
// counted from the base.
func (r *Runner) dotRangeElements(name string, elems []string) ([]int, []string) {
	base := r.arrayBase()
	if a, stored := r.Arrays[name]; stored {
		if _, produced := r.pipelineStatuses(name); !produced {
			keys := r.arrayKeys(a)
			subs := make([]int, len(keys))
			vals := make([]string, len(keys))
			for i, k := range keys {
				subs[i] = k + base
				vals[i] = a[k].scalar()
			}
			return subs, vals
		}
	}
	subs := make([]int, len(elems))
	for i := range elems {
		subs[i] = base + i
	}
	return subs, elems
}

// dotRangeEnd evaluates one end of a range, where an end with nothing in it
// is 0 rather than the empty-subscript refusal: `${a[..2]}` is `a b c`.
func (r *Runner) dotRangeEnd(w *syntax.Word) (int, bool) {
	text := strings.TrimSpace(r.subscriptTextAsWritten(w))
	if text == "" {
		return 0, true
	}
	return r.subscriptIndexAsWritten(text, text)
}

// dotRangeOfTable is the range over an association's keys, compared as
// strings — see dotRangeSubscript for the measurements.
func (r *Runner) dotRangeOfTable(e *syntax.ParamExpr) []string {
	lo := r.assocKey(e.IndexDots.Lo.Text)
	hi := r.assocKey(e.IndexDots.Hi.Text)
	a, _ := r.assocFor(e.Name)
	keys := a.keys()
	first := sort.SearchStrings(keys, lo)
	var out []string
	for i := first; i < len(keys); i++ {
		if i > first && keys[i] > hi {
			break
		}
		if e.Indirect {
			out = append(out, keys[i])
		} else {
			out = append(out, a[keys[i]].scalar())
		}
	}
	return out
}
