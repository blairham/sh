// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A literal whose subscripts are keys, standing in one element's place:
// `typeset -A a; a[k]=([u]=1 [v]=2)`.
//
// The column that nests an element literal makes this element a keyed table
// of its own, which is what the same literal makes of a name — `b=([u]=1
// [v]=2)` declares `b` a table there — one level down. Measured 2026-11-02 on
// ksh93u+ 2012-08-01 (`/bin/ksh`), from script files under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME (#6778):
//
//	typeset -A a; a[k]=([u]=1 [v]=2)    typeset -A a=([k]=([u]=1 [v]=2) )
//	                                    traced `+ a[k][u]=1`, `+ a[k][v]=2`
//	  then ${a[k][u]} ${#a[k][@]} ${!a[k][@]} ${a[k][@]}    1 2 u v 1 2
//	  then a[k][w]=3                    ([u]=1 [v]=2 [w]=3), ${a[k][0]} empty
//	  then a[k]+=([w]=3)                ([u]=1 [v]=2 [w]=3), traced `+ a[k][w]=3`
//	typeset -a a; a[1]=([u]=1)          typeset -a a=([1]=([u]=1) )
//	typeset -A a; a[k]=([0]=x [2]=y); a[k][1+1]=z
//	                                    ([0]=x [1+1]=z [2]=y): keys, numbers or not
//	x=5; a[k]=([$x]=1 [u v]=2)          traced `+ a[k][5]=1`, `+ a[k]['u v']=2`
//	typeset -A a; a[k]=([v]=2 [u]=1)    traced in that order, v first
//	typeset -A a=([k]=([u]=1 [v]=2))    the same table, through the whole literal
//	typeset -A a; a[k]=(1 2)            ([k]=(1 2) ) — the plain list, unmoved
//
// This read every subscript as arithmetic, so `u` and `v`, unset, were both 0
// and the element kept the last value alone: `([k]=(2) )`, traced `+ a[k]=( 2
// )`. A chained write is not the answer, though the trace reads like one: a
// lone `a[k][u]=1` makes the element an *indexed* array and stores under 0 in
// both shells. It is the literal that makes the element a table, and only
// then do the keys land as keys.
//
// Not modeled, all measured on the same build:
//
//   - A keyed literal over an element already holding an indexed array writes
//     into that array as arithmetic and keeps the rest: `a[k]=(p q);
//     a[k]=([u]=1)` is `([k]=(1 q) )` there, and a table here.
//   - `a[k]+=([u]=1)` over an indexed array appends the word `[u]=1`, and
//     over nothing at all is `([0]='' [u]=1)`.
//   - What the element reads as one string, `${a[k]}`: `2` after `[u]=1
//     [v]=2`, `3` after `[a]=1 [b]=2 [c]=3`, and empty after `[v]=2 [u]=1`,
//     which follows the reference's own table and nothing a script can see.
//     This answers the value under the last key, which agrees on the first
//     two rows.
//
// bash refuses the literal before it reads the subscript, on the
// SubscriptedArrayLiteralRefused road, so nothing here reaches it.

// nestedTable is the table a literal standing in an element's place builds,
// where its subscripts are keys: every element subscripted, and the reading
// Semantics.ArrayLiteralSubscriptIsAKey gives a name's literal saying keys.
// False for every other literal, which nests an indexed array as before.
func (r *Runner) nestedTable(parsed []literalElem) (Element, bool) {
	if len(parsed) == 0 {
		return Element{}, false
	}
	for _, e := range parsed {
		if !e.subscripted || e.members != nil {
			return Element{}, false
		}
	}
	if !r.literalSubscriptsAreKeys(r.sem().ArrayLiteralSubscriptIsAKey, parsed) {
		return Element{}, false
	}
	t := make(AssocArray, len(parsed))
	for _, e := range parsed {
		switch {
		case e.nested != nil:
			t[e.sub] = nestedAppended(t[e.sub], *e.nested, e.appendValue)
		case e.appendValue:
			t[e.sub] = Scalar(t[e.sub].scalar() + e.value)
		default:
			t[e.sub] = Scalar(e.value)
		}
	}
	if r.expanded != nil && r.expanded.tracesAfterTheStore {
		// The pairs as written, for the trace: it writes them in the order
		// the literal did, which a table does not keep.
		r.expanded.nestedPairs = parsed
	}
	return Element{Table: t}, true
}

// scalar is what a nested table reads as one string: the value under the last
// key in this implementation's order. See the rows above for why that is an
// approximation.
func (a AssocArray) scalar() string {
	last, ok := a.lastElement()
	if !ok {
		return ""
	}
	return last.scalar()
}

// tableJoined is a copy of held with every key of add written over it, for
// `a[k]+=([w]=3)` where the element already holds a table.
func tableJoined(held, add AssocArray) AssocArray {
	out := held.clone()
	for k, v := range add {
		out[k] = v.clone()
	}
	return out
}

// nestedTableListing is a nested table as `typeset -p` writes it: every key
// with its value, quoted as a table's own keys are, and the trailing blank
// the nested array's listing takes under the same rule.
func (r *Runner) nestedTableListing(t AssocArray) string {
	keys := t.keys()
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, "["+r.declareQuoted(k, ListedValueAlone)+"]="+r.listedElement(t[k], ListedValueAlone))
	}
	return "(" + strings.Join(pairs, " ") + nestTrailingSpace(t.lastElement()) + ")"
}

// tracedTableWrites is the lines a nested table's store writes: one element
// write per pair, under target — the element's own name and subscripts,
// already written — in the order pairs gives, or this implementation's key
// order where nothing kept the literal's.
func (r *Runner) tracedTableWrites(target string, t AssocArray, pairs []literalElem, d *Diagnostics) []string {
	if pairs == nil {
		for _, k := range t.keys() {
			v := t[k]
			pairs = append(pairs, literalElem{sub: k, value: v.Str, nested: nestedOrNil(v), subscripted: true})
		}
	}
	quote := func(s string) string { return r.traceQuote(s, d.TraceQuoting, d.TraceMetacharacters) }
	lines := make([]string, 0, len(pairs))
	for _, e := range pairs {
		if e.nested != nil {
			lines = append(lines, r.tracedNestedLines(target, quote(e.sub), e.appendValue, *e.nested, nil, d)...)
			continue
		}
		op := "="
		if e.appendValue {
			op = "+="
		}
		lines = append(lines, target+"["+quote(e.sub)+"]"+op+quote(e.value))
	}
	return lines
}

// tracedNestedLines is an element write whose value is an array or a table of
// its own: one line for an array, and one per pair for a table.
func (r *Runner) tracedNestedLines(
	name, sub string,
	appended bool,
	value Element,
	pairs []literalElem,
	d *Diagnostics,
) []string {
	if value.Table == nil {
		return []string{r.traceNestedElementWrite(name, sub, appended, value, d)}
	}
	return r.tracedTableWrites(name+"["+sub+"]", value.Table, pairs, d)
}

// nestedOrNil is the element itself where it holds an array or a table, and
// nil where it holds a string.
func nestedOrNil(e Element) *Element {
	if e.Nested == nil && e.Table == nil {
		return nil
	}
	return &e
}

// traceNestedTableStore is the lines an element write that stored a table
// writes, and whether it stored one.
func (r *Runner) traceNestedTableStore(a *syntax.Assign, e *expandedAssign, d *Diagnostics) ([]string, bool) {
	if e == nil || e.nested == nil || e.nested.Table == nil {
		return nil, false
	}
	links := e.chain
	if len(links) < 2 {
		links = []string{e.subscript}
	}
	var b strings.Builder
	b.WriteString(a.Name)
	for _, link := range links {
		b.WriteString("[" + r.traceQuote(link, d.TraceQuoting, d.TraceMetacharacters) + "]")
	}
	return r.tracedTableWrites(b.String(), e.nested.Table, e.nestedPairs, d), true
}

// nestedTableKey is a link's subscript read as a key of the table the walk
// has reached: expanded and never evaluated, as a declared table's own.
func (r *Runner) nestedTableKey(link *syntax.ParamExpr) string {
	return r.joinWord(link.Index)
}

// chainWriteIntoATable is chainWrite's last link where the element it reaches
// holds a table: the link is a key and the write is the table's, append and
// attributes as an indexed link has them. Measured 2026-11-02 on ksh93u+
// 2012-08-01, `typeset -A a; a[k]=([u]=1 [v]=2); a[k][w]=3` is `([u]=1 [v]=2
// [w]=3)`, with `${a[k][0]}` empty (#6778).
func (r *Runner) chainWriteIntoATable(name string, held AssocArray, place func(Element),
	links []string, key, value string, appendTo bool, build func() (Element, bool),
) ([]string, bool) {
	tbl := held.clone()
	links = append(links, key)
	if build != nil {
		v, ok := build()
		if !ok {
			return nil, false
		}
		tbl[key] = nestedAppended(tbl[key], v, appendTo)
		place(Element{Table: tbl})
		return links, true
	}
	if appendTo {
		value = tbl[key].scalar() + value
	}
	folded, ok := r.attributeFolded(name, value)
	if !ok {
		return nil, false
	}
	tbl[key] = Scalar(folded)
	place(Element{Table: tbl})
	return links, true
}
