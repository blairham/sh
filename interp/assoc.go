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
type AssocArray map[string]Element

// keys returns the assigned keys, sorted.
//
// Sorted because the shells promise no order at all: the same three
// assignments come back in three different orders from the three shells that
// have the feature, and bash's own order moves between versions. A promise
// nobody makes is not worth imitating, and a deterministic one is worth
// having.
//
// **A reading surface wants [Runner.assocKeys] rather than this**, which is
// this and then the one thing a *produced* table may say about its own order.
// One name in the tree does say something — see SetDynamicAssocKeyOrder — and
// a surface that walked the sorted keys straight would read that one backwards
// while reading every other table right, which is the shape this tree keeps
// finding: a helper beside a helper, one of them missing what the other
// carries.
func (a AssocArray) keys() []string {
	out := make([]string, 0, len(a))
	for k := range a {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// assocKeys is the keys of the table read under name, in the order that name
// reads in: sorted, unless it is a produced association that states its own.
//
// Named rather than taken from the table because order is a property of the
// *parameter* and an AssocArray is a Go map, which has none to carry. The
// producer is the one that knows — `$history`'s keys are event numbers and
// zsh reads them newest first, where sorting them as strings puts `10` before
// `9`.
//
// The name has to be a produced one for the order to speak, so that a script
// that unsets the parameter and stores an ordinary table under the same
// spelling gets the ordinary answer back.
func (r *Runner) assocKeys(name string, a AssocArray) []string {
	keys := a.keys()
	name = r.throughNameref(name)
	if _, produced := r.DynamicAssocs[name]; !produced {
		return keys
	}
	order, ok := r.dynamicAssocKeyOrder[name]
	if !ok {
		return keys
	}
	return order(keys)
}

// SetDynamicAssocKeyOrder states the order a produced association's keys are
// read in, where sorted is what every other table in this engine answers.
//
// One name states one, and it is not a preference. `$history`'s keys are
// history event numbers and zsh reads them **newest first**, measured
// 2026-09-24 against zsh 5.9.2 with five entries: `${(k)history}` is `5 4 3 2
// 1` and `typeset -m history` lists `[5]` down to `[1]`. That order is what
// `${history[(r)pat]}` means — the subscript takes the *first* match in scan
// order, which is the most recent command, and it is the whole of what
// zsh-autosuggestions asks the parameter for. Sorted keys would hand back the
// oldest match instead, and as strings they would not even be in history
// order: `"10" < "9"`.
//
// So this is a correctness seam and not a tidiness one, and it is deliberately
// per-name: nothing else in the module has an order anybody can observe, and
// a rule applied to all of them would be a claim about tables the shells make
// no promise about.
func (r *Runner) SetDynamicAssocKeyOrder(name string, order func(keys []string) []string) {
	if r.dynamicAssocKeyOrder == nil {
		r.dynamicAssocKeyOrder = map[string]func([]string) []string{}
	}
	r.dynamicAssocKeyOrder[name] = order
}

// assocValues returns the values in key order — the reading `${m[@]}` yields.
//
// On the runner rather than on the table, for the reason [Runner.denseElems]
// is: an element may hold a compound, whose text is the names under it.
func (r *Runner) assocValues(name string, a AssocArray) []string {
	keys := r.assocKeys(name, a)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, r.elemText(a[k]))
	}
	return out
}

// clone is a copy a write may reach without the original seeing it, to any
// depth — Array.clone's rule for a keyed table, and see it for why the shallow
// copy was not enough.
func (a AssocArray) clone() AssocArray {
	if a == nil {
		return nil
	}
	out := make(AssocArray, len(a))
	for k, v := range a {
		out[k] = v.clone()
	}
	return out
}

// equal reports whether two tables hold the same value under the same keys.
//
// Its own method for Array.equal's reason: an element may hold an array, which
// makes it uncomparable, so maps.Equal does not compile over this type.
func (a AssocArray) equal(b AssocArray) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		w, held := b[k]
		if !held || !v.equal(w) {
			return false
		}
	}
	return true
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
	name = r.throughNameref(name)
	if _, stored := r.AssocArrays[name]; stored {
		return true
	}
	_, produced := r.DynamicAssocs[name]
	return produced
}

// markAssoc gives a name the associative attribute, which is what
// `declare -A` and `typeset -A` do. Declaring twice keeps the elements.
func (r *Runner) markAssoc(name string) {
	name = r.namespaceWriteName(name)
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
	// As for the indexed letter: `c=(a=1); typeset -A c` is `typeset -A c=()`
	// there, with the compound's members gone. See compoundVariableRetyped.
	r.localizeMemberWrite(name)
	r.compoundVariableRetyped(name)
	r.AssocArrays[name] = AssocArray{}
	// Declared and not assigned, which is the state one listing writes
	// without the `=()` — see compounddeclaredonly.go.
	r.compoundDeclaredOnly(name)
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
	r.setAssocElemAs(name, key, value, ElementHoldsItsValue)
}

// setAssocElemAs is setAssocElem told what kind of element to leave behind,
// which is the whole of what a valueless declaration needs of it — see
// declareAssocKey. One body rather than two, because the guards above the
// write (a produced table, a compound being retyped, the name's attribute
// folding the value, the declared-only mark being lifted) are the same
// whichever kind is being stored, and a second copy that omitted one of them
// is the failure this file already warns about.
func (r *Runner) setAssocElemAs(name, key, value string, kind ElementKind) {
	name = r.namespaceWriteName(name)
	name = r.throughNameref(name)
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
		// A keyed write over a compound variable replaces it, the same way
		// an indexed one does in storeArray. See compoundVariableRetyped.
		r.localizeMemberWrite(name)
		r.compoundVariableRetyped(name)
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
	a[key] = Element{Str: value, Kind: kind}
	// Written to, so the name leaves the declared-only set — see
	// compounddeclaredonly.go.
	r.compoundWasAssigned(name)
	// See nameIsBack: a keyed table is the one store that does not keep a
	// scalar view, so it is the one that has to lift the mark itself. An
	// indexed array keeps `$a` answering through setVar and lifts it there.
	r.nameIsBack(name)
}

// declareAssocKey is what a **valueless** subscripted declaration leaves under
// a table's key: the key exists, and it holds nothing rather than the empty
// string.
//
// The two states are one byte apart in a listing and identical everywhere
// else, which is why the difference lives on the element rather than in a
// second table. Measured on ksh93u+ 2012-08-01, 2026-09-19:
//
//	typeset -A m; typeset 'm[k]'            typeset -A m=([k]=)
//	typeset -A m; m[k]=                     typeset -A m=([k]='')
//	typeset -A m; m[k]=; typeset 'm[k]'     typeset -A m=([k]='')
//	typeset -A m; typeset 'm[k]'; m[k]=     typeset -A m=([k]='')
//	typeset -A m; typeset 'm[k]'; m[j]=v    typeset -A m=([j]=v [k]=)
//
// The third and fourth rows are why this declines to overwrite: the
// declaration never takes a key that holds a value back down to holding none,
// in either order, so it writes only where there is no key yet.
//
// No axis, for the reason nestTrailingSpace carries none: only the column
// that reads a valueless operand's subscript without writing an element can
// reach this at all — see Semantics.ValuelessSubscriptedOperand — so no other
// dialect can produce the state to be asked about.
//
// A **produced** table is written through its hook with the empty string, as
// every other write to one is: the hook takes a string and there is nowhere
// in it to put a state. See setAssocElem for why a produced table is never
// written into the stored one.
func (r *Runner) declareAssocKey(name, key string) {
	// The **stored** table rather than assocFor's, which answers a produced
	// one first: a produced table is written through its hook by the call
	// below whatever it already holds, exactly as every other write to one
	// is, and a key this declined on its behalf would be a write it never
	// heard about.
	target := r.throughNameref(r.namespaceWriteName(name))
	if _, held := r.AssocArrays[target][key]; held {
		// Already holding something, so the declaration says nothing this
		// table has not already been told.
		return
	}
	r.setAssocElemAs(name, key, "", ElementDeclaredAndEmpty)
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
	// So the whole-array spelling is decided on the subscript as **typed**,
	// before any reading of quotes and before any expansion, and never on the
	// key the axis produced: under the quote-removing answer that key is `@`
	// too, and the two spellings would collapse into one.
	//
	// The expansion is the half this used to leave out, and a table is where
	// it costs the most. Measured 2026-09-20 with `typeset -A m=([k]=v);
	// m[@]=Z` — the key both columns store there — and `K=@`, `${m[$K]}` is
	// that key's value `Z` in bash 5.3.20 and ksh93u+ alike, where this shell
	// joined every value and answered `Z v` at status 0 with nothing said. A
	// subscript that merely comes out `@` is ordinary text: a key here, an
	// expression over an indexed array. See wholeArraySubscriptAsTyped, which
	// is the same reading the indexed side asks and is asked from here so
	// that there is one of it (#3889).
	if _, typed := r.wholeArraySubscriptAsTyped(e); typed {
		// Non-nil even when empty: the array exists, so `${m[@]:-d}` on an
		// empty one is zero fields rather than the default — the same answer
		// an empty indexed array gives.
		values := r.assocValues(e.Name, a)
		if e.Length || e.Indirect {
			// A count and a list of keys, neither of which reads a value —
			// the same pair the indexed whole-array branch leaves alone.
			return values
		}
		// A `.get` per key, in the order the values came out, which is what
		// a.values already walks. Measured on ksh93u+ 2012-08-01,
		// 2026-09-16: `${m[@]}` on a two-key table enters the hook with each
		// key in turn.
		return r.disciplinedElements(e.Name, r.assocKeys(e.Name, a), values)
	}
	key := r.assocKey(e.Subscript())
	if r.reportEmptyAssocKeyRead(e, key) {
		// Refused outright rather than reported and answered, which is the
		// length operator's row alone. Nothing to give back: the word is
		// abandoned and the shell is ending.
		return nil
	}
	if v, ok := a[key]; ok {
		// The key is the subscript a discipline is entered with, exactly as
		// a number is for an indexed array. See interp/discipline.go.
		return []string{r.disciplinedElement(e.Name, key, r.elemText(v))}
	}
	// A key the table has not got fires all the same, and a hook that
	// assigns answers for it — measured, `${m[zz]}` on a table without `zz`
	// enters the hook with `zz` there.
	if v, replaced := r.disciplineElementRead(e.Name, key); replaced {
		return []string{v}
	}
	return r.absentAssocElement(e, key)
}

// reportEmptyAssocKeyRead says what a read's key coming out empty draws, where
// the dialect says anything, and reports whether the read was **refused** —
// which only the length operator's row is.
//
// One function for both faces of the emptiness rather than a second helper
// beside it, because the two are chosen between rather than added together:
// every route that reads a key reaches here, and a route that asked only the
// report would silently give the length operator the wrong one of the two.
//
// The plain read is the other face of assocAssignKey, and deliberately not
// the same code: the store refuses, names the subscript as it was *written*
// and reports 1, while this reports the **name** alone and the expansion
// carries on with the empty string at status 0. One column does both and
// words them differently, which is what says they are two questions — see
// Semantics.EmptyAssociativeKeyIsReportedWhenRead.
//
// That face sets no failed-expansion flag. Measured 2026-09-12,
// `"[${m[$w]}]${m[$w]}"` writes the sentence once per read and still prints
// `[]`, so the word is completed rather than abandoned.
func (r *Runner) reportEmptyAssocKeyRead(e *syntax.ParamExpr, key string) bool {
	if key != "" {
		return false
	}
	if e.Length {
		// The length operator asks its own axis and never the read's, which
		// is measured rather than tidy: the shell that reports the plain read
		// of a *declared-only* table says nothing at all about the length of
		// the same element. See refusesTheLengthOfAnEmptyAssocKey.
		return r.refusesTheLengthOfAnEmptyAssocKey(e)
	}
	if !r.ask(r.sem().EmptyAssociativeKeyIsReportedWhenRead,
		"a read whose key on a keyed table came out empty") {
		return false
	}
	r.diagf("%s\n", Wording(r.diag().EmptyAssociativeKeyRead,
		"%[1]s: bad array subscript", e.Name))
	return false
}

// refusesTheLengthOfAnEmptyAssocKey is `${#m[$w]}` with `$w` empty, where the
// dialect refuses it, and reports whether it did.
//
// The subject is the subscript **as it was written**, brackets included and
// with no name in front of it, which is neither of the two subjects the
// neighboring shapes use. IndexText is the text between the brackets, so the
// brackets are in the wording — see Diagnostics.EmptyAssociativeKeyLength.
//
// A name the declaration's letters merely brought into being is not this, and
// that carve-out is measured rather than inferred: `typeset -A m; ${#m[$w]}`
// is `0` and silent in the column that refuses `typeset -A m; m=();
// ${#m[$w]}` beside it. It is the *assignment* that moves the name and not
// the emptiness, which is exactly the pair declaredOnlyCompound already keeps
// — so this is the second reader of that set, and the first outside the
// listing.
//
// Unlike the read's report this abandons the word: status 1, nothing
// expanded, and the shell ends. See Semantics.EmptyAssociativeKeyRefusesTheLength.
func (r *Runner) refusesTheLengthOfAnEmptyAssocKey(e *syntax.ParamExpr) bool {
	if r.declaredOnlyCompound[e.Name] {
		return false
	}
	if !r.ask(r.sem().EmptyAssociativeKeyRefusesTheLength,
		"`${#m[$w]}`, the length of a keyed table's element under an empty key") {
		// Either the dialect answers the length as an ordinary absent
		// element — ksh93 and zsh, which give the `0` the refusal stands in
		// front of — or no dialect was chosen and ask has said so.
		return r.unspecified
	}
	r.diagf("%s\n", Wording(r.diag().EmptyAssociativeKeyLength,
		"[%[1]s]: bad array subscript", e.IndexText))
	r.expandErr = true
	return true
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
			if r.reportEmptyAssocKeyRead(e, key) {
				return nil
			}
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
	r.expandSubscriptTilde(w)
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

// expandSubscriptTilde is Semantics.SubscriptKeyExpandsALeadingTilde: the
// leading unquoted `~` of a subscript becomes the home directory before the
// text becomes a key.
//
// Asked here rather than at the store, because a *read* takes it too — with
// the element stored under `$HOME/k`, `${m[~/k]}` finds it in the two columns
// that expand — and this is where the two routes meet. Asked only where the
// two readings differ: a subscript with no leading unquoted `~` is the same
// key either way, which is every subscript a script normally writes.
//
// The word is rewritten in place, as expandTilde does everywhere else. That
// is safe to repeat: the value it leaves no longer starts with a `~`, so a
// subscript inside a loop expands once and reads the same on every pass.
func (r *Runner) expandSubscriptTilde(w *syntax.Word) {
	if !startsWithAnUnquotedTilde(w) {
		return
	}
	if !r.ask(r.sem().SubscriptKeyExpandsALeadingTilde,
		"a subscript's leading tilde expanding to the home directory") {
		return
	}
	r.expandTilde(w)
}

// startsWithAnUnquotedTilde is the guard that keeps the axis from being asked
// about a subscript it cannot change — which is the condition expandTilde
// itself applies, read without performing anything.
func startsWithAnUnquotedTilde(w *syntax.Word) bool {
	if w == nil || len(w.Spans) == 0 {
		return false
	}
	s := w.Spans[0]
	return s.Kind == syntax.Literal && s.Quoting == syntax.Unquoted &&
		strings.HasPrefix(s.Value, "~")
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
	r.abandonTheCommand()
	return "", false
}

// expandKeyQuoted is the key under the reading that removes quotes: the
// subscript expanded as a word, joined, and not trimmed.
//
// The shortcut is for a word that *is* its text — one literal span, nothing to
// expand — and it has to ask how that span was quoted as well as what kind it
// is. `$'…'` is a Literal span whose Value is the source between the quotes
// with its escapes still in it: the decoding is the expansion's, and taking
// the Value as the key skipped it. So `declare -A n; n[$'\t']=tab` stored a key
// spelled backslash-then-t rather than a tab — two characters, reachable only
// by writing the same escape again, and invisible to `${n[$t]}` for a `t`
// holding the character it meant. bash, whose subscript this is, stores the
// tab (#2749).
//
// Every other quoting kind really is its text. Single quotes protect
// everything, a double-quoted literal keeps a backslash that escapes nothing,
// and a backslash-quoted span is one character — so naming the one kind that
// decodes is narrower than routing every quoted span through the expander,
// and it keeps the shortcut doing what it is for.
func (r *Runner) expandKeyQuoted(w *syntax.Word) string {
	if w != nil && len(w.Spans) == 1 && w.Spans[0].Kind == syntax.Literal &&
		w.Spans[0].Quoting != syntax.DollarSingleQuoted {
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
func (r *Runner) assocScalar(name string, a AssocArray) (string, bool) {
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
		return r.elemText(v), true
	}
	if r.ask(r.sem().ArrayScalarIsTheWholeArray, "a plain `$a` giving the whole array") {
		return strings.Join(r.assocValues(name, a), " "), true
	}
	if r.ask(r.sem().KeyedTableScalarIsTheFirstValue,
		"a plain `$m` on a keyed table giving the first value rather than the one keyed `0`") {
		// Where the table keeps an order, "one element" is the first of
		// them. An empty table has no first value and is still set, which is
		// the same answer the no-early-return above protects.
		vs := r.assocValues(name, a)
		if len(vs) == 0 {
			return "", true
		}
		return vs[0], true
	}
	v, ok := a["0"]
	return r.elemText(v), ok
}

// assignAssocLiteral is `m=([k]=v …)` on a declared name — and `m+=(…)`,
// which keeps the elements already there where `=` starts over.
func (r *Runner) assignAssocLiteral(name string, elems []*syntax.ArrayElem, appendTo bool) {
	// Before the elements are expanded, because one reading refuses the shape
	// and a refusal must not run what the literal holds: measured, zsh writes
	// its sentence without the `$(…)` in a later element having run.
	reads, refuseBare, ok := r.tableLiteralShape(elems)
	if !ok {
		return
	}
	parsed, ok := r.literalElems(elems, reads,
		r.bareLiteralElementIsOneValue(name, elems, false))
	if !ok {
		// A failed element list costs the whole table, exactly as it costs
		// the whole indexed array — see literalElems.
		return
	}
	if r.indexArrayIntoATable(name, parsed, appendTo) {
		return
	}
	r.assignAssocElems(name, parsed, appendTo, refuseBare)
}

// tableLiteralShape answers the two questions a table literal's element shapes
// raise, and reports whether the literal survives them.
//
// **reads** is whether an element is read for a `[key]=` head at all, which is
// the parse question every literal asks. **refuseBare** is the store question and
// only a mixed literal asks it: whether a bare element among heads is refused
// rather than paired off. They are separate because the common answers differ —
// an all-bare literal in the dialect that refuses a mixture still *reads* for
// heads, and it has no head for a bare element to be measured against.
//
// Semantics.MixedTableLiteral has the readings and the rows. It is asked only
// where the literal really mixes the two shapes, which is the narrowest point the
// columns part: a literal whose elements all carry a head, and one where none
// does, reaches the same table under every reading.
func (r *Runner) tableLiteralShape(elems []*syntax.ArrayElem) (reads, refuseBare, ok bool) {
	heads, bare := 0, 0
	for _, el := range elems {
		if el.Word == nil {
			continue
		}
		if syntax.SubscriptedElement(el.Word) {
			heads++
			continue
		}
		bare++
	}
	if heads == 0 || bare == 0 {
		// Not mixed. The shape rule the *grammar* carries still applies — see
		// literalShapeReadsSubscripts, which is the reading one dialect refuses
		// this mixture at the read for.
		return r.literalShapeReadsSubscripts(elems), false, true
	}
	switch r.mixedTableLiteral() {
	case MixedTableLiteralFollowsTheFirstElement:
		// The first element chooses. Where it carried a head every later bare
		// element is refused; where it did not, a `[key]=value` word is ordinary
		// text and nothing is refused at all.
		first := elems[0].Word != nil && syntax.SubscriptedElement(elems[0].Word)
		return first, first, true
	case MixedTableLiteralRefused:
		r.fatal("%s\n", Wording(r.diag().MixedTableLiteralRefusal,
			"bad [key]=value syntax for associative array"))
		return false, false, false
	}
	// Unanswered: mixedTableLiteral has said so and stopped the command.
	return false, false, false
}

// indexArrayIntoATable is the refusing answer to a literal of **bare words**
// landing on a table, and reports whether it fired.
//
// See Semantics.BareElementsInATableLiteralEndTheScript for the panel: two
// columns pair the words off as key, value, key, value and one reads the
// parentheses as an *index array* and will not put one in a table.
//
// An element **written** without a `[key]=` head is what decides it, and not
// the fields it came to — which is why it reads literalElem.subscripted
// rather than counting words. Measured: `e=; typeset -A m=($e)` is refused
// there although the element expands to nothing, and only a literal with no
// element written in it at all — `typeset -A m=()` — is accepted.
//
// After the expansion rather than before, for the same measured reason:
// `typeset -A m=($(echo SIDE >&2))` writes `SIDE` and then complains, so the
// elements are expanded and then judged. Nothing has been stored by then —
// assignAssocElems is what stores — so the refusal costs the whole literal.
func (r *Runner) indexArrayIntoATable(name string, parsed []literalElem, appendTo bool) bool {
	if !r.sem().BareElementsInATableLiteralEndTheScript {
		return false
	}
	bare := false
	for _, e := range parsed {
		if !e.subscripted {
			bare = true
			break
		}
	}
	if !bare {
		return false
	}
	if r.tableBecomesAnIndexArray(name, appendTo) {
		r.compoundKindEmptied(name, false)
		if a, ok := r.literalInto(name, Array{}, 0, parsed, true); ok {
			r.storeArray(name, a)
		}
		return true
	}
	r.fatal("%s\n", Wording(r.diag().IndexArrayIntoATable,
		"cannot append index array to associative array %[1]s", name))
	return true
}

// tableBecomesAnIndexArray is the *other* answer the refusing column gives to
// the same literal: the name stops being a table and holds the words as an
// index array, with nothing said about it.
//
// Which of the two happens turns on three measured things, and the surprise
// is the third. Measured 2026-09-13 on ksh93u+ 2012-08-01:
//
//	typeset -A m=([a]=1); m=(x y)        typeset -a m=(x y)
//	typeset -A m=([a]=1); typeset m=(x y)    typeset -a m=(x y)
//	typeset -A m=([a]=1); typeset -A m=(x y) cannot append index array …
//	typeset -A m=([a]=1); m+=(x y)           cannot append index array …
//	typeset -A m;         m=(x y)            cannot append index array …
//	typeset -A m=();      m=(x y)            cannot append index array …
//
// So an **append** never converts — the sentence's verb is the honest one
// there, since there is no index array to append to a table — and the table
// **letter written on the same command** holds the name to its kind, which is
// what Runner.tableLetterHere records. The third is that an **empty** table
// refuses where a table with an element in it converts, which is that shell's
// own and is reproduced rather than explained: a replacing assignment is
// replacing nothing when the name holds no element, and there it complains.
func (r *Runner) tableBecomesAnIndexArray(name string, appendTo bool) bool {
	if appendTo || r.tableLetterHere[name] {
		return false
	}
	return len(r.AssocArrays[name]) > 0
}

// assignAssocElems places an already-expanded literal into the keyed table.
//
// Taken apart from the expansion because the indexed path reaches it too: a
// dialect that reads a subscript as a key stores `a=([k]=v)` here, and it must
// not expand the elements a second time to do so.
func (r *Runner) assignAssocElems(name string, parsed []literalElem, appendTo, refuseBare bool) {
	// Before anything is written, and before the produced-table refusal below
	// it, because the arity is a property of the literal and not of what is
	// behind the name: measured, `aliases=(a 1 b)` earns the same sentence a
	// stored table's does. See tableLiteralPairsOff for what the refusal must
	// leave standing.
	if !r.tableLiteralPairsOff(parsed) {
		return
	}
	_, produced := r.DynamicAssocs[name]
	if produced && !appendTo {
		if _, writable := r.dynamicAssocWriters[name]; !writable {
			// A produced association with no writer has nowhere for the
			// elements to go, and the stored table a fallback would leave
			// behind shadows the view for good. Refused by name; the
			// element form beside it is refused by the same absence.
			r.diagf("%s: assigning to the whole of a produced association is not implemented yet\n", name)
			return
		}
	}
	// Written to, so the name leaves the declared-only set. Here rather than
	// only in setAssocElem below, because an empty literal — `m=()`, the very
	// case the listing tells apart — writes no element and would otherwise
	// still read as declared-only. See compounddeclaredonly.go.
	r.compoundWasAssigned(name)
	// Read before the clear below, because that is what one of the two
	// answers to KeyedLiteralAppendJoinsTheReplacedValue needs and the clear
	// is about to destroy it.
	replaced := r.replacedElems(name, parsed, appendTo)
	switch {
	case appendTo:
	case produced:
		// The producer's own table is what a replacing assignment replaces,
		// and only for the names whose shell empties it first — see
		// SetDynamicAssocEmptiedByReplacement for why that is per-name and
		// not a rule. Never the stored table: writing an empty one here
		// would put a snapshot in front of the view, which is the failure
		// SetDynamicAssocWriter exists to prevent.
		//
		// And only for a literal that names a key. `aliases=()` leaves every
		// alias standing in zsh 5.9.2, as does `e=(); aliases=($e)` — the
		// empty literal hands that shell's parameter no table at all and its
		// set function returns before it clears anything. So the one shape
		// that reads most like "empty this" is the one shape that does not,
		// and it is measured rather than reasoned.
		if r.dynamicAssocEmptied[name] && literalNamesAKey(parsed) {
			r.emptyProducedAssoc(name)
		}
	default:
		if r.AssocArrays == nil {
			r.AssocArrays = map[string]AssocArray{}
		}
		r.AssocArrays[name] = AssocArray{}
	}
	// An element without a `[k]=` head is half of a pair: `m=(a 1 b 2)` is
	// two elements in the two shells that take the form at all — the third
	// refuses it, and refusing the shape both others accept would be the
	// lone answer.
	var pairs, pairWords []string
	for _, e := range parsed {
		if e.subscripted {
			if e.sub == "" && !r.tableLiteralKeyIsThere(name, e.written, true) {
				// An empty key written with a head, in the reading that refuses
				// one. It costs the rest of the literal and leaves what is
				// already stored standing. See Semantics.EmptyKeyInATableLiteral.
				return
			}
			if e.members != nil {
				// The value under the key is a **compound variable's body** —
				// `a=([1]=(p=1 q=2))` — which hangs its members under the
				// element's own spelling. The same two pieces the indexed
				// placement uses, through the same function; see
				// Runner.literalElementCompound.
				value, ok := r.literalElementCompound(name, e.sub, e.members)
				if !ok {
					return
				}
				r.storeAssocElement(name, e.sub, value)
				continue
			}
			if e.nested != nil {
				// The value under the key is a literal of its own —
				// `a=( [0]=(1 2) )` — which the whole-element write stores
				// rather than the string one. See storeAssocElement.
				r.storeAssocElement(name, e.sub, *e.nested)
				continue
			}
			value := e.value
			if e.appendValue {
				v, ok := r.keyedLiteralAppend(name, e.sub, value, replaced)
				if !ok {
					return
				}
				value = v
			}
			r.setAssocElem(name, e.sub, value)
			continue
		}
		if refuseBare {
			// A bare element where the first one carried a head, in the reading
			// that lets the first element choose. The refusal costs the rest of
			// the literal and leaves what is already stored standing, which is
			// what the column that makes it was measured doing: `a=([zero]=5
			// [one]=10 four [two]=2)` keeps `zero` and `one`. See
			// Semantics.MixedTableLiteral.
			r.refuseBareTableLiteralElement(name, e)
			return
		}
		pairs = append(pairs, e.fields...)
		for range e.fields {
			// The element's own text beside each field it made, so a refusal
			// about the *key* can name the word the source wrote. One field per
			// element is the shape the refusing column has — see
			// BareElementsInATableLiteralAreEachOneValue — and where an element
			// made several the field is the nearest thing to a word there is.
			pairWords = append(pairWords, e.written)
		}
	}
	for i := 0; i < len(pairs); i += 2 {
		if pairs[i] == "" {
			written := pairs[i]
			if i < len(pairWords) && pairWords[i] != "" {
				written = pairWords[i]
			}
			if !r.tableLiteralKeyIsThere(name, written, false) {
				// The pair is dropped and the literal carries on, which is what
				// the refusing column was measured doing — and is the half that
				// parts the two shapes. See Semantics.EmptyKeyInATableLiteral.
				continue
			}
		}
		value := ""
		if i+1 < len(pairs) {
			value = pairs[i+1]
		}
		r.setAssocElem(name, pairs[i], value)
	}
}

// tableLiteralPairsOff reports whether a keyed literal's bare elements may be
// paired off as key, value, key, value — and writes the refusal where they may
// not.
//
// The count is over the **fields the elements came to** rather than over the
// elements themselves, because one element may come to any number of fields:
// with `words=(a 1 b)`, `typeset -A m=($words)` is one element and three
// fields, and it is the three that decides. See
// Semantics.BareElementsInATableLiteralMustPairOff, where the pair holding the
// written count fixed at one is.
//
// An element carrying a `[key]=` head contributes no field and is not counted:
// it names where its value goes and has no partner to find. So a literal with
// no bare element in it — `m=([k]=v)`, and the empty `m=()` — counts zero,
// which is even, and the axis is not asked at all. Nor is it asked for an even
// count, since every column pairs one off.
//
// Called before the table is emptied and before the first element is stored,
// which is what the refusing column was measured doing: with `typeset -A
// h=(x 9)` in front of it, `eval "h=(a 1 b)"` leaves `x` at `9`. The expansion
// has already happened by then — the elements are `parsed` — which is the same
// order indexArrayIntoATable is in, and the same order zsh writes a `$(…)`
// element's side effect before its complaint in.
func (r *Runner) tableLiteralPairsOff(parsed []literalElem) bool {
	fields := 0
	for _, e := range parsed {
		if e.subscripted {
			continue
		}
		fields += len(e.fields)
	}
	if fields%2 == 0 {
		return true
	}
	if !r.ask(r.sem().BareElementsInATableLiteralMustPairOff,
		"a keyed literal whose bare elements come to an odd number of fields being refused") {
		// Taken, the last field becoming a key with nothing under it — or
		// unanswered, in which case ask has said so and stopped the command,
		// and there is nothing to store into.
		return !r.unspecified
	}
	r.fatal("%s\n", Wording(r.diag().UnpairedTableLiteralElements,
		"bad set of key/value pairs for associative array"))
	return false
}

// tableLiteralKeyIsThere reports whether an **empty** key a table literal named
// may be stored, and writes the refusal where it may not.
//
// One function for the literal's two shapes because they share the reading and
// part only in what a refusal costs: a bare pair is dropped and the literal goes
// on, a `[""]=` head gives up the rest of it. head says which this is. See
// Semantics.EmptyKeyInATableLiteral for the rows.
func (r *Runner) tableLiteralKeyIsThere(name, written string, head bool) bool {
	if r.emptyKeyInATableLiteral() != EmptyKeyInATableLiteralRefused {
		// Accepted, or unanswered — in which case the axis has already said so
		// and stopped the command, and there is nothing to store into anyway.
		return !r.unspecified
	}
	if !head {
		wording := Wording(r.diag().EmptyKeyInATableLiteralPair,
			"%[1]s: bad array subscript", written)
		r.diagf("%s\n", wording)
		return false
	}
	r.failedSubscript("%s\n", Wording(r.diag().EmptyKeyInATableLiteralElement,
		"%[1]s: bad array subscript", written))
	return false
}

// refuseBareTableLiteralElement is the sentence a bare element earns inside a
// literal whose first element carried a head.
//
// The status and the reach are the ones measured: status 1, and the rest of the
// command list is given up while the input is not — the same shape a bad
// subscript in a literal already has, which is why it goes through the same door.
func (r *Runner) refuseBareTableLiteralElement(name string, e literalElem) {
	wording, fallback := r.diag().BareElementInASubscriptedTableLiteral,
		"%[1]s: %[2]s: must use subscript when assigning associative array"
	verb := e.written
	if e.operand {
		// A declaration's operand, which the shell expanded before the builtin
		// saw it — and the column that refuses names the value rather than the
		// text, in single quotes. See
		// Diagnostics.BareElementInASubscriptedTableLiteralOperand for the rows.
		if operand := r.diag().BareElementInASubscriptedTableLiteralOperand; operand != "" {
			wording = operand
		}
		verb = ""
		if len(e.fields) > 0 {
			verb = e.fields[0]
		}
	}
	// The status is the builtin's as well as the line's: a declaration whose
	// operand was refused reports 1 rather than the 0 its own return would
	// otherwise put there, which is the same door every other refused operand
	// goes through.
	r.assignFailed = true
	r.failedSubscript("%s\n", Wording(wording, fallback, name, verb))
}

// literalNamesAKey reports whether a keyed literal has anything to store —
// an element with a `[k]=` head, or a bare word to pair off. False for `m=()`
// and for `m=($empty)` alike, which is the distinction the empty-literal
// paragraph in assignAssocElems turns on: the second is empty after
// expansion, not in the source, and both leave a produced table alone.
func literalNamesAKey(parsed []literalElem) bool {
	for _, e := range parsed {
		if e.subscripted || len(e.fields) > 0 {
			return true
		}
	}
	return false
}

// replacedElems is what the append elements of a *replacing* keyed literal
// would join if they joined the value the name held before it — read through
// the ordinary element read, so a produced table answers as it would to a
// script, and nil where no element asks the question.
//
// Nil for an appending literal on purpose: `m+=([k]+=x)` keeps the table, so
// the value before the literal and the value built so far are the same thing
// and there is nothing to tell apart. The map is keyed by the key and not by
// position because two elements may name one key, which is exactly the
// spelling the axis below is visible in.
func (r *Runner) replacedElems(name string, parsed []literalElem, appendTo bool) map[string]string {
	if appendTo {
		return nil
	}
	var replaced map[string]string
	for _, e := range parsed {
		if !e.subscripted || !e.appendValue {
			continue
		}
		if replaced == nil {
			replaced = map[string]string{}
		}
		replaced[e.sub] = r.assocElemCurrent(name, e.sub)
	}
	return replaced
}

// keyedLiteralAppend is what a `[k]+=v` element of a keyed literal stores.
//
// The join itself is appendedValue's, so an integer-attributed table adds
// where a plain one concatenates, exactly as `m[k]+=v` on its own line does.
// What is decided here is only *which value is joined to*, and that is a
// measured disagreement — see KeyedLiteralAppendJoinsTheReplacedValue.
//
// Asked only where the two readings part. They agree for every appending
// literal, for a key the name did not hold, and for the first append of a key
// the literal has not already written — which is nearly every line anyone
// writes, and none of them should have to name a shell to run.
func (r *Runner) keyedLiteralAppend(name, key, add string, replaced map[string]string) (string, bool) {
	base := r.assocElemCurrent(name, key)
	if old, asked := replaced[key]; asked && old != base {
		if r.ask(r.sem().KeyedLiteralAppendJoinsTheReplacedValue,
			"a `[k]+=` element of a replacing keyed literal joining the value the name held before it") {
			base = old
		}
		if r.unspecified {
			return "", false
		}
	}
	return r.appendedValue(name, base, add)
}

// assocElem reads one `[key]=value` or `[key]+=value` element of a literal.
//
// The shape is decided on the word as written, before any expansion — the
// same rule assignShaped applies — because expanding first would hand `[k]=v`
// to the pattern matcher, where it is a character class. The key is the
// literal text between the brackets; the value expands as an assignment's,
// which is what keeps `[k]=$x` whole and `[k]=*` a star.
//
// The split itself is [syntax.ElementSubscript]'s rather than this
// function's, and that is the point of the seam. The parser asks the same
// question of the same word — one dialect decides a literal's whole shape on
// whether its *first* element is subscripted — and a second scan here is how
// the two would come to disagree about what a subscripted element is: a
// literal would then parse under one reading and store under the other. What
// is left here is the expansion, which is the half the parser has no business
// doing.
//
// The append spelling is read in exactly the places the plain one is, which
// is what every shell in the panel that reads either does. It was not read at
// all, so `a=(p q r); a+=( [1]+=Z )` left a fourth element holding the seven
// characters `[1]+=Z` where bash and zsh join the value to the element the
// subscript names — silently, at status 0, with an array that is the wrong
// length and looks populated (#2405). In the dialect whose subscripts are
// keys the same word went in as a *key* spelled `[1]+=Z`, which is worse: the
// table grows an entry nothing will ever read.
func (r *Runner) assocElem(w *syntax.Word) (key, value string, appendValue, ok bool) {
	sub, val, appends, ok := syntax.ElementSubscript(w)
	if !ok {
		return "", "", false, false
	}
	return r.expandAssignValue(sub), r.expandAssignValue(val), appends, true
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

// SetDynamicAssocEmptiedByReplacement says that a *replacing* whole-table
// assignment to this produced association — `m=(k v)`, `m=([k]=v)` — empties
// the table it views before the literal's own keys go in, rather than merging
// into it.
//
// Per-name rather than a rule, because the shells do not agree with
// themselves about it. Measured on zsh 5.9.2 and bash 5.3.20, 2026-09-16, by
// writing one key the ordinary way, replacing the table with a literal naming
// a *different* key, and asking for the first one back:
//
//	$commands       kept       $aliases    emptied
//	$functions      kept       $galiases   emptied
//	$options        kept       $saliases   emptied
//	$mapfile        kept       $nameddirs  emptied
//	BASH_CMDS       kept
//	BASH_ALIASES    kept
//
// A probe that replaced the table with a literal naming the *same* key could
// not have told the two apart, which is why the measurement uses two.
//
// The emptying goes through the writer, one `set` false per key the view
// currently lists, so a name registered here needs a writer whose unset
// actually removes the entry — see emptyProducedAssoc.
func (r *Runner) SetDynamicAssocEmptiedByReplacement(name string) {
	if r.dynamicAssocEmptied == nil {
		r.dynamicAssocEmptied = map[string]bool{}
	}
	r.dynamicAssocEmptied[name] = true
}

// emptyProducedAssoc removes every key the view currently lists, through the
// producer's own writer.
//
// Through the writer rather than by reaching into whatever the producer reads
// from, because the writer is the one place that knows what removing an entry
// *means* for this name: an alias leaves the alias table, a named directory
// leaves the named-directory table, and neither is a map interp could clear
// on the dialect's behalf without knowing which.
//
// Sorted, so a writer with a visible order — a diagnostic, a trace — writes
// the same sequence twice for the same table.
func (r *Runner) emptyProducedAssoc(name string) {
	write, ok := r.dynamicAssocWriters[name]
	if !ok {
		return
	}
	produce, ok := r.DynamicAssocs[name]
	if !ok {
		return
	}
	table := produce(r)
	keys := make([]string, 0, len(table))
	for key := range table {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		write(r, key, "", false)
	}
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
// # With one exception, and it has a direction
//
// A view may **read more than it lists, never less**. Where the two differ,
// the element producer is the answer and the whole table is the enumeration,
// and a key it answers for that the table does not carry is allowed.
//
// The asymmetry is not a taste. Everything the contract above protects is
// answered by the *element* reading — `${m[k]}`, `${m[k]:-d}`, `${+m[k]}`,
// the absent-element refusal — so a key that reads and is not listed leaves
// every branch a script can take correct, and costs only that `${(k)m}` is
// narrower than what `${m[k]}` will answer. A key that *lists* and does not
// read is the other thing entirely: it makes `${m[k]:-d}` take the default
// for a name the shell has just enumerated, which is the failure this whole
// paragraph exists to prevent.
//
// The case that earned it is `$terminfo`, and it is what the shell being
// modeled does: `${+terminfo[Se]}` is 1 with the terminal's cursor sequence
// behind it while `Se` is not among the names `${(k)terminfo}` gives, because
// the description's extended section is readable and not enumerated there.
// See dialect/zsh/terminfo.go, which carries the measurement (#2102).
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
		return r.elemText(a[key])
	}
	if produce, ok := r.dynamicAssocElements[name]; ok {
		v, _ := produce(r, key)
		return v
	}
	if produce, ok := r.DynamicAssocs[name]; ok {
		return r.elemText(produce(r)[key])
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
	name = r.throughNameref(name)
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
