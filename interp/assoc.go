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
//
// A *produced* one carries it as much as a stored one: the attribute decides
// how a subscript is read, and `${functions[a b]}` has to be a key wherever
// the table came from.
func (r *Runner) assocDeclared(name string) bool {
	_, ok := r.assocFor(name)
	return ok
}

// markAssoc gives a name the associative attribute, which is what
// `declare -A` and `typeset -A` do. Declaring twice keeps the elements.
func (r *Runner) markAssoc(name string) {
	if _, produced := r.DynamicAssocs[name]; produced {
		// A produced association already has the attribute — assocDeclared
		// asks assocFor — and giving it a stored table would put an empty one
		// in front of the producer, which is the shadowing setAssocElem
		// guards against by another route. `typeset -A functions` is the line
		// that reaches here.
		return
	}
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
//
// A **produced** association is written through its own hook and never into
// the stored table, and that is not a nicety: a stored table shadows the
// producer — assocFor asks it first — so one assignment would turn a live view
// into a snapshot taken at that instant, and every read afterwards would be a
// plausible answer to a question about the past. The name would still be an
// association, still hold the right keys, and never say it had stopped
// tracking. See SetDynamicAssocWriter.
func (r *Runner) setAssocElem(name, key, value string) {
	if write, ok := r.dynamicAssocWriters[name]; ok {
		write(r, key, value, true)
		return
	}
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
//
// Through the producer's hook for a produced one, for the reason setAssocElem
// is: `unset "functions[f]"` undefines the function in the shell this models,
// and a delete from a table that is not the answer would do nothing and say
// nothing.
func (r *Runner) unsetAssocElem(name, key string) {
	if write, ok := r.dynamicAssocWriters[name]; ok {
		write(r, key, "", false)
		return
	}
	delete(r.AssocArrays[name], key)
}

// assocSubscript answers `${m[k]}`, `${m[@]}` and `${m[*]}` for a declared
// associative name. The subscript is a key, not an expression: `${m[1+1]}` is
// the element stored under the three characters, measured unanimous in the
// shells that have the attribute.
func (r *Runner) assocSubscript(a AssocArray, e *syntax.ParamExpr) []string {
	// A bare `@` or `*` is the whole array in every shell in the panel, and a
	// *quoted* one is a key — `${n["@"]}` looks one up and finds nothing.
	// So the whole-array spelling is decided on the subscript as written,
	// before any reading of quotes, and never on the key the axis produced:
	// under the quote-removing answer that key is `@` too, and the two
	// spellings would collapse into one.
	if w := r.searchOperand(e.Subscript()); w == "@" || w == "*" {
		// Non-nil even when empty: the array exists, so `${m[@]:-d}` on an
		// empty one is zero fields rather than the default — the same answer
		// an empty indexed array gives.
		return a.values()
	}
	if v, ok := a[r.assocKey(e.Subscript())]; ok {
		return []string{v}
	}
	// nil says the element was not there, exactly as the indexed path does: a
	// value of "" is set and `${m[k]:-d}` has to tell the two apart.
	return nil
}

// assocKey is the key an associative array's subscript names.
//
// Two readings, and the axis is SubscriptIsAQuotingContext. Where the
// subscript *is* a quoting context the key is the text inside its quotes,
// which is bash's and ksh93's answer; where it is not, the key is the
// subscript exactly as written — substitutions performed, and every other
// character, quotes and backslashes included, kept. That second reading is
// Runner.searchOperand, which already renders a subscript that way for the
// search flags PR #1101 landed; one rule reached from two sides rather than
// two renderings that could drift.
//
// Neither reading trims. The space-trimming this used to do belongs to the
// arithmetic subscript, where the evaluator ignores blanks anyway, and it is
// wrong here in every shell in the panel: with `p[s]=T`, `${p[ s ]}` looks up
// three characters and finds nothing in bash, ksh93 and zsh alike, where this
// found `T`.
//
// Asked only where the two readings differ, so a key with no quote or
// backslash in it — which is nearly every key a script writes — never demands
// a dialect for the question.
func (r *Runner) assocKey(w *syntax.Word) string {
	quoted := r.expandKeyQuoted(w)
	asWritten := r.searchOperand(w)
	if quoted == asWritten {
		return quoted
	}
	if r.ask(r.sem().SubscriptIsAQuotingContext, "an array subscript being a quoting context") {
		return quoted
	}
	return asWritten
}

// expandKeyQuoted is the key under the reading that removes quotes: the
// subscript expanded as a word, joined, and not trimmed.
func (r *Runner) expandKeyQuoted(w *syntax.Word) string {
	if w != nil && len(w.Spans) == 1 && w.Spans[0].Kind == syntax.Literal {
		return w.Spans[0].Value
	}
	return strings.Join(r.expandWordNoSplit(w), "")
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
	if _, produced := r.DynamicAssocs[name]; produced && !appendTo {
		// Replacing a produced association wholesale would mean emptying
		// something this shell does not store — every function at once, every
		// option at once — and the stored table it would leave behind
		// shadows the view for good. Refused by name; the element form
		// beside it goes through the producer's own hook.
		r.diagf("%s: assigning to the whole of a produced association is not implemented yet\n", name)
		return
	}
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

// SetDynamicAssoc registers an associative array whose contents are produced
// when it is read.
//
// The third of the three produced-parameter seams, and the one the other two
// cannot stand in for. `SetDynamic` gives a string and `SetDynamicArray` a
// list; a parameter that answers *which functions exist and what each one's
// body is* is neither, because `${m[key]}` has to reach one of them by name
// and a list of pairs cannot be asked that question.
//
// **It is a view and not a snapshot**, and that distinction is the whole
// reason it is a function rather than a table. A dialect that filled an
// AssocArray once would be right until the first `f() { … }` and then quietly
// wrong — and quietly is the word, because the shape stays plausible: the
// caller reads an association, gets a value or an empty string, and is never
// told the table stopped tracking. The producer is asked on every read, so
// read, mutate, read again in one shell gives three different answers where a
// snapshot gives one.
func (r *Runner) SetDynamicAssoc(name string, value func(*Runner) AssocArray) {
	if r.DynamicAssocs == nil {
		r.DynamicAssocs = map[string]func(*Runner) AssocArray{}
	}
	r.DynamicAssocs[name] = value
}

// SetDynamicAssocWriter says what happens when a script assigns to one element
// of a produced association, or unsets one — `set` false is the unset.
//
// Required rather than optional for any produced association a script may
// write to, and the reason is the failure it prevents. Without it an
// assignment lands in the stored table, the stored table is what a later read
// finds first, and the view has silently become a snapshot of the moment
// somebody wrote to it. Nothing about the name's shape changes and no
// diagnostic is written; the caller reads a perfectly ordinary association
// that stopped tracking.
//
// A produced association a script must *not* write to is marked readonly
// instead, which is a refusal with a sentence rather than a write that goes
// somewhere unhelpful.
func (r *Runner) SetDynamicAssocWriter(name string, write func(r *Runner, key, value string, set bool)) {
	if r.dynamicAssocWriters == nil {
		r.dynamicAssocWriters = map[string]func(*Runner, string, string, bool){}
	}
	r.dynamicAssocWriters[name] = write
}

// assocFor is the associative table a name reads as, produced or stored.
//
// The stored table is asked first, which is the order the produced scalars and
// the produced arrays already follow: a script that has assigned to the name
// gets its own value back. A produced one has no stored table until something
// writes to it, so in practice the second clause is the answer.
func (r *Runner) assocFor(name string) (AssocArray, bool) {
	if a, ok := r.AssocArrays[name]; ok {
		return a, true
	}
	if produce, ok := r.DynamicAssocs[name]; ok {
		// A producer with nothing to say still has a table: an empty
		// association and an absent one are different things, and the name
		// exists either way. A nil map needs no normalizing to say so — every
		// read this package does of one is a length, a range or a lookup, and
		// all three answer for nil — so there is no branch here to get wrong.
		return produce(r), true
	}
	return nil, false
}
