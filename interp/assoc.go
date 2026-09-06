// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"sort"
	"strings"

	"github.com/blairham/sh/syntax"
)

// AssocArray is an associative array: string keys to string values.
//
// A separate kind from Array rather than a widening of it, because the two
// read the same subscript differently: `m[1+1]` names the element at 2 in an
// indexed array and the element whose key is the three characters `1+1` in an
// associative one — measured, and the whole reason `declare -A` exists. The
// attribute has to be on the *name*, set by `declare -A` or `typeset -A`,
// because the subscript's meaning is decided before any value is looked at.
type AssocArray map[string]string

// keys returns the assigned keys, sorted.
//
// Sorted because the shells promise no order at all: the same three
// assignments come back in three different orders from the three shells that
// have the feature, and bash's own order moves between versions. A promise
// nobody makes is not worth imitating, and a deterministic one is worth
// having.
func (a AssocArray) keys() []string {
	out := make([]string, 0, len(a))
	for k := range a {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// values returns the values in key order — the reading `${m[@]}` yields.
func (a AssocArray) values() []string {
	keys := a.keys()
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, a[k])
	}
	return out
}

// assocDeclared reports whether the name carries the associative attribute.
func (r *Runner) assocDeclared(name string) bool {
	_, ok := r.AssocArrays[name]
	return ok
}

// markAssoc gives a name the associative attribute, which is what
// `declare -A` and `typeset -A` do. Declaring twice keeps the elements.
func (r *Runner) markAssoc(name string) {
	if r.AssocArrays == nil {
		r.AssocArrays = map[string]AssocArray{}
	}
	if _, ok := r.AssocArrays[name]; ok {
		return
	}
	r.AssocArrays[name] = AssocArray{}
	// A name is one kind of array at a time. Two of the three shells with
	// the attribute let `-A` take over a name that held an indexed array
	// (the third refuses); what none of them does is keep both readings
	// alive at once.
	delete(r.Arrays, name)
}

// setAssocElem assigns one element. The key is any string, the empty one
// included — it is the subscript as written, never a number.
func (r *Runner) setAssocElem(name, key, value string) {
	a := r.AssocArrays[name]
	if a == nil {
		a = AssocArray{}
		if r.AssocArrays == nil {
			r.AssocArrays = map[string]AssocArray{}
		}
		r.AssocArrays[name] = a
	}
	a[key] = value
}

// unsetAssocElem removes one key, which is `unset m[k]` on a declared name.
func (r *Runner) unsetAssocElem(name, key string) {
	delete(r.AssocArrays[name], key)
}

// assocSubscript answers `${m[k]}`, `${m[@]}` and `${m[*]}` for a declared
// associative name. The subscript is a key, not an expression: `${m[1+1]}` is
// the element stored under the three characters, measured unanimous in the
// shells that have the attribute.
func (r *Runner) assocSubscript(a AssocArray, e *syntax.ParamExpr) []string {
	switch key := r.subscriptText(e.Subscript()); key {
	case "@", "*":
		// Non-nil even when empty: the array exists, so `${m[@]:-d}` on an
		// empty one is zero fields rather than the default — the same answer
		// an empty indexed array gives.
		return a.values()
	default:
		if v, ok := a[key]; ok {
			return []string{v}
		}
		// nil says the element was not there, exactly as the indexed path
		// does: a value of "" is set and `${m[k]:-d}` has to tell the two
		// apart.
		return nil
	}
}

// assocScalar is what a plain `$m` gives on an associative array.
//
// The same axis a plain `$a` on an indexed array asks, wearing the shape
// associative keys force on it: the shell that answers "the whole array"
// joins every value, and the shells that answer "one element" read the
// element whose key is `0` — not the first key there is, because "first" is
// an order and an associative array has none. Measured: with `m[a]=1` alone,
// `$m` is empty in the two and `1` in the one.
func (r *Runner) assocScalar(a AssocArray) (string, bool) {
	if len(a) == 0 {
		return "", false
	}
	if v, ok := a["0"]; ok && len(a) == 1 {
		// The two answers agree, so nothing is asked — the join of one value
		// is that value.
		return v, true
	}
	if r.ask(r.sem().ArrayScalarIsTheWholeArray, "a plain `$a` giving the whole array") {
		return strings.Join(a.values(), " "), true
	}
	v, ok := a["0"]
	return v, ok
}

// assignAssocLiteral is `m=([k]=v …)` on a declared name — and `m+=(…)`,
// which keeps the elements already there where `=` starts over.
func (r *Runner) assignAssocLiteral(name string, elems []*syntax.Word, appendTo bool) {
	r.assignAssocElems(name, r.literalElems(elems), appendTo)
}

// assignAssocElems places an already-expanded literal into the keyed table.
//
// Taken apart from the expansion because the indexed path reaches it too: a
// dialect that reads a subscript as a key stores `a=([k]=v)` here, and it must
// not expand the elements a second time to do so.
func (r *Runner) assignAssocElems(name string, parsed []literalElem, appendTo bool) {
	if !appendTo {
		if r.AssocArrays == nil {
			r.AssocArrays = map[string]AssocArray{}
		}
		r.AssocArrays[name] = AssocArray{}
	}
	// An element without a `[k]=` head is half of a pair: `m=(a 1 b 2)` is
	// two elements in the two shells that take the form at all — the third
	// refuses it, and refusing the shape both others accept would be the
	// lone answer.
	var pairs []string
	for _, e := range parsed {
		if e.subscripted {
			r.setAssocElem(name, e.sub, e.value)
			continue
		}
		pairs = append(pairs, e.fields...)
	}
	for i := 0; i < len(pairs); i += 2 {
		value := ""
		if i+1 < len(pairs) {
			value = pairs[i+1]
		}
		r.setAssocElem(name, pairs[i], value)
	}
}

// assocElem reads one `[key]=value` element of an associative literal.
//
// The shape is decided on the word as written, before any expansion — the
// same rule assignShaped applies — because expanding first would hand `[k]=v`
// to the pattern matcher, where it is a character class. The key is the
// literal text between the brackets; the value expands as an assignment's,
// which is what keeps `[k]=$x` whole and `[k]=*` a star.
func (r *Runner) assocElem(w *syntax.Word) (key, value string, ok bool) {
	if w == nil || len(w.Spans) == 0 {
		return "", "", false
	}
	head := w.Spans[0]
	if head.Kind != syntax.Literal || head.Quoting != syntax.Unquoted ||
		!strings.HasPrefix(head.Value, "[") {
		return "", "", false
	}
	// The `]=` that closes the key is looked for across the spans, not only
	// in the first: `["c d"]=v` and `[$k]=v` put quoting or an expansion
	// between the brackets, so the key is a word of its own that ends where
	// an *unquoted* `]=` appears — a quoted one is part of the key.
	for i, s := range w.Spans {
		if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
			continue
		}
		text := s.Value
		if i == 0 {
			text = text[1:]
		}
		j := strings.Index(text, "]=")
		if j < 0 {
			continue
		}
		keyWord := syntax.Word{Spans: append([]syntax.Span(nil), w.Spans[:i]...)}
		if i == 0 {
			keyWord.Spans = nil
		} else {
			keyWord.Spans[0].Value = strings.TrimPrefix(keyWord.Spans[0].Value, "[")
		}
		keyWord.Spans = append(keyWord.Spans, syntax.Span{
			Kind: syntax.Literal, Value: text[:j], Quoting: s.Quoting, Pos: s.Pos,
		})
		valueWord := syntax.Word{Spans: append([]syntax.Span{{
			Kind: syntax.Literal, Value: text[j+2:], Quoting: s.Quoting, Pos: s.Pos,
		}}, w.Spans[i+1:]...)}
		return r.expandAssignValue(&keyWord), r.expandAssignValue(&valueWord), true
	}
	return "", "", false
}
