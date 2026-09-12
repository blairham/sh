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
// Asked of the *tables* rather than through assocFor, which is the same
// question and not the same work: assocFor produces, and every caller here
// throws the table away. Measured on a real startup, ninety-two of the three
// hundred and forty-one renderings of `$functions` were this predicate —
// `functions[f]=…` and `unset "functions[f]"` each asking whether the name
// was keyed and building nineteen hundred function bodies to find out.
func (r *Runner) assocDeclared(name string) bool {
	if _, stored := r.AssocArrays[name]; stored {
		return true
	}
	_, produced := r.DynamicAssocs[name]
	return produced
}

// markAssoc gives a name the associative attribute, which is what
// `declare -A` and `typeset -A` do. Declaring twice keeps the elements.
func (r *Runner) markAssoc(name string) {
	if _, produced := r.DynamicAssocs[name]; produced {
		// A produced association already has the attribute — assocDeclared
		// answers from this very table — and giving it a stored table would put an empty one
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
	// What the name's attributes make of the value, the same fold an array's
	// elements get in storeArray and a scalar gets in setVarAs — asked for
	// rather than assumed, because an element is where the panel splits. See
	// compoundElemsFolded.
	if r.attributeWouldChange(name, value) {
		if r.ask(r.sem().CompoundElementsGoThroughTheAttribute,
			"an element written to an attributed name going through the attribute") {
			v, ok := r.attributeFolded(name, value)
			if !ok {
				return
			}
			value = v
		}
		if r.unspecified {
			return
		}
	}
	a[key] = value
	// See nameIsBack: a keyed table is the one store that does not keep a
	// scalar view, so it is the one that has to lift the mark itself. An
	// indexed array keeps `$a` answering through setVar and lifts it there.
	r.nameIsBack(name)
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
	key := r.assocKey(e.Subscript())
	r.reportEmptyAssocKeyRead(e.Name, key)
	if v, ok := a[key]; ok {
		return []string{v}
	}
	return r.absentAssocElement(e, key)
}

// reportEmptyAssocKeyRead says that a read's key came out empty, where the
// dialect says so, and leaves the read to answer as it would have.
//
// The other face of assocAssignKey, and deliberately not the same code: the
// store refuses, names the subscript as it was *written* and reports 1, while
// this reports the **name** alone and the expansion carries on with the empty
// string at status 0. One column does both and words them differently, which
// is what says they are two questions — see
// Semantics.EmptyAssociativeKeyIsReportedWhenRead.
//
// Nothing here sets the failed-expansion flag. Measured 2026-09-12,
// `"[${m[$w]}]${m[$w]}"` writes the sentence once per read and still prints
// `[]`, so the word is completed rather than abandoned.
func (r *Runner) reportEmptyAssocKeyRead(name, key string) {
	if key != "" {
		return
	}
	if !r.ask(r.sem().EmptyAssociativeKeyIsReportedWhenRead,
		"a read whose key on a keyed table came out empty") {
		return
	}
	r.diagf("%s\n", Wording(r.diag().EmptyAssociativeKeyRead,
		"%[1]s: bad array subscript", name))
}

// assocSubscriptOfTheName is a subscript on a name that reads as an
// association, and what it decides is how much of the table the read needs.
//
// One key wants one key. Where the name has a keyed producer — see
// SetDynamicAssocElement — that is the whole of the work, and the
// whole-table producer is never run; every other shape goes the long way,
// which is the same answer arrived at by walking. The two spellings of the
// whole array are the shapes that need the table, and they are told apart
// here rather than inside the keyed route so that the route is only ever
// taken by a read a single lookup can finish.
func (r *Runner) assocSubscriptOfTheName(e *syntax.ParamExpr) []string {
	if produce, keyed := r.assocElementProducer(e.Name); keyed {
		// Decided on the subscript as written and before any reading of
		// quotes, for the reason assocSubscript gives: a *quoted* `@` is an
		// ordinary key, and asking the produced key instead would collapse
		// the two spellings into one.
		if w := r.searchOperand(e.Subscript()); w != "@" && w != "*" {
			key := r.assocKey(e.Subscript())
			r.reportEmptyAssocKeyRead(e.Name, key)
			if v, ok := produce(r, key); ok {
				return []string{v}
			}
			return r.absentAssocElement(e, key)
		}
	}
	a, _ := r.assocFor(e.Name)
	return r.assocSubscript(a, e)
}

// absentAssocElement is what a key an association has not got reads as, by
// either route to it.
func (r *Runner) absentAssocElement(e *syntax.ParamExpr, key string) []string {
	if r.refuseAbsentElement(e, key) {
		// A produced association that answers only some of the keys its name
		// is asked for, asked for one of the others. Refused by name here
		// rather than at either word path, because this is where the key is
		// known — see absentparam.go.
		return nil
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

// assocAssignKey is assocKey for a caller that is about to *store* under the
// key, and it is the one place an empty one is refused.
//
// Splitting the store from the read is what the panel asks for rather than a
// convenience: one column refuses to store under an empty key and still
// *reads* one, with a different sentence and a different subject — `m[""]:
// bad array subscript` on the way in and `m: bad array subscript` on the way
// out. The second is #1972 and is not asked here, so a read of a key nothing
// holds stays the ordinary miss it is in the other two columns.
//
// The subject is the subscript **as it was written**, which is the printer's
// job and not an expansion's: bash names `m[$w]` and not the empty text that
// word came to, so both of the renderings a *key* has — quotes off, quotes
// kept — are the wrong one here. syntax.PrintWord is the one that remembers
// the spelling, and a diagnostic about how a line was typed is what it is
// for.
func (r *Runner) assocAssignKey(name string, w *syntax.Word) (string, bool) {
	key := r.assocKey(w)
	if key != "" {
		return key, true
	}
	if !r.ask(r.sem().EmptyAssociativeKeyIsAnError, "an empty key on a keyed table") {
		return key, true
	}
	r.diagf("%s\n", Wording(r.diag().BadArraySubscript,
		"%[1]s[%[2]s]: bad array subscript", name, syntax.PrintWord(w)))
	// assignFailed and not the status alone: an assignment statement decides
	// its own status after the right-hand sides have run, and writes 0 over
	// anything set here unless it is told the assignment did not happen.
	// Measured, `declare -A m; m[""]=4; echo $?` is 1 in the column that
	// refuses.
	r.status, r.assignFailed = 1, true
	// And the rest of the *list* is given up, which is the same thing a
	// refused reassignment does in the one column that reaches either:
	// measured 2026-09-12, `declare -A m` then `m[""]=4; echo A` on one line
	// prints no `A` and the line after it runs. No axis, because only one
	// column refuses at all — see refuseReadonly, where the same two fields
	// carry the same behavior for the neighboring refusal.
	r.ctl, r.abandonLine = controlAbandon, r.line
	return "", false
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
	// No early return for an empty table. Which of the two readings a bare
	// name takes decides set-ness as well as the value, and the two shells
	// disagree about an empty one in the same direction they disagree about
	// an empty array: measured, zsh answers `typeset -A h; ${+h}` with 1 and
	// `${h+SET}` with SET, where bash's `declare -A h; [[ -v h ]]` is false.
	// Short-circuiting here answered bash's way for both, and did it before
	// the axis was reached — so no dialect could say otherwise. See
	// bareArrayReading, which is the same shape for an array.
	if v, ok := a["0"]; ok && len(a) == 1 {
		// The two answers agree, so nothing is asked — the join of one value
		// is that value.
		return v, true
	}
	if r.ask(r.sem().ArrayScalarIsTheWholeArray, "a plain `$a` giving the whole array") {
		return strings.Join(a.values(), " "), true
	}
	if r.ask(r.sem().KeyedTableScalarIsTheFirstValue,
		"a plain `$m` on a keyed table giving the first value rather than the one keyed `0`") {
		// Where the table keeps an order, "one element" is the first of
		// them. An empty table has no first value and is still set, which is
		// the same answer the no-early-return above protects.
		vs := a.values()
		if len(vs) == 0 {
			return "", true
		}
		return vs[0], true
	}
	v, ok := a["0"]
	return v, ok
}

// assignAssocLiteral is `m=([k]=v …)` on a declared name — and `m+=(…)`,
// which keeps the elements already there where `=` starts over.
func (r *Runner) assignAssocLiteral(name string, elems []*syntax.Word, appendTo bool) {
	parsed, ok := r.literalElems(elems)
	if !ok {
		// A failed element list costs the whole table, exactly as it costs
		// the whole indexed array — see literalElems.
		return
	}
	r.assignAssocElems(name, parsed, appendTo)
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

// SetDynamicAssocElement says how a produced association answers *one* key,
// for the names where producing the whole table to read one entry is the
// wrong shape of work.
//
// Optional, and the default is right for most: a producer whose table is a
// map the runner already holds costs nothing to run whole, and registering a
// second entry point for it would be two answers to maintain where one would
// do. It earns its place where a key's value has to be *made* — `$functions`
// renders each body through the printer, so producing all of them to hand
// back one was measured at thirty milliseconds a read on a real startup's
// nineteen hundred functions, against a hundred and fifty microseconds for
// the one.
//
// # The contract is that the two readings agree
//
// This is a faster route to the same answer and not a second opinion. For
// every key, the element producer must say found with the value the whole
// table would hold, and not-found exactly where the table would have no such
// key — because which of the two it says is what decides whether
// `${m[k]:-d}` takes the default, and, for a name with absent elements
// declared, whether the read is refused by name. A producer that disagreed
// with its own table would make `${m[k]}` and `${(k)m}` describe different
// shells.
//
// A *stored* table shadows both readings, the same way and for the same
// reason it shadows the whole-table producer — see assocFor.
func (r *Runner) SetDynamicAssocElement(name string, value func(r *Runner, key string) (string, bool)) {
	if r.dynamicAssocElements == nil {
		r.dynamicAssocElements = map[string]func(*Runner, string) (string, bool){}
	}
	r.dynamicAssocElements[name] = value
}

// assocElementProducer is the one-key reading a name has, if it has one and
// nothing stored is standing in front of it.
//
// The stored-table check is not an optimisation but the shadowing rule:
// assocFor asks AssocArrays first, so a name something has written to reads
// out of that table, and a keyed producer that answered anyway would make the
// write invisible to `${m[k]}` while `${(k)m}` still showed it.
func (r *Runner) assocElementProducer(name string) (func(*Runner, string) (string, bool), bool) {
	if _, stored := r.AssocArrays[name]; stored {
		return nil, false
	}
	produce, ok := r.dynamicAssocElements[name]
	return produce, ok
}

// assocElemCurrent is what `m[k]` holds at this moment, by the same route a
// *read* of it takes.
//
// It exists for `m[k]+=v`, which has to join the value that is there — and
// which read the stored table directly until #2260, so on a **produced**
// association it always joined onto nothing. A produced name has no stored
// table, so `functions[f]+=…` replaced the function it meant to extend and
// `mapfile[p]+=…` truncated the file it meant to append to, each silently and
// each looking exactly like a correct append of an empty original.
//
// The order is assocFor's order and must stay it: a stored table shadows a
// producer, so a name something has already written to joins onto its own
// value, and the keyed producer is preferred over the whole-table one for the
// reason SetDynamicAssocElement gives — building every function body to read
// one is what that seam exists to avoid, and an append is a read.
func (r *Runner) assocElemCurrent(name, key string) string {
	if a, stored := r.AssocArrays[name]; stored {
		return a[key]
	}
	if produce, ok := r.dynamicAssocElements[name]; ok {
		v, _ := produce(r, key)
		return v
	}
	if produce, ok := r.DynamicAssocs[name]; ok {
		return produce(r)[key]
	}
	return ""
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
