// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"strconv"
	"strings"
)

// `declare -p` and `typeset -p`, which write a declaration back as input the
// shell could read again. It is how scripts serialize state, and how
// `declare -p v` is used to ask whether a name is set at all.
//
// The three shells with the option produce three different texts for
// identical state, and the differences are not one format with options — the
// command word, the flag layout and the array shape all move together. So the
// shape is one enum, the way a `select` menu's layout is, and the value
// spelling is a ListingQuotingStyle of its own, because the engine that
// single-quotes its aliases and traps double-quotes its declarations.
// docs/spec/semantics.md, "Listing a declaration back", has the measurements.

// DeclarationListingForm is the shape of a listed declaration.
type DeclarationListingForm int

const (
	// DeclarationListingUnspecified is no answer, and is refused like any
	// other.
	DeclarationListingUnspecified DeclarationListingForm = iota
	// DeclareListingClustered writes `declare` — whichever name invoked it —
	// then one clustered flag word, with `--` standing where there is no
	// attribute: `declare -- v="1"`, `declare -irx n="5"`. Array elements
	// always carry their subscript, each element of an associative listing
	// is followed by one space, and an associative array with no elements
	// lists with no value at all.
	DeclareListingClustered
	// DeclareListingExportSpelled writes `typeset`, except an exported
	// scalar, which is spelled `export` with the `x` dropped from the
	// cluster — `export -r r=''` — while an exported array stays
	// `typeset -ax`. Array values are wrapped `( x y )` with padding spaces
	// and carry no subscripts: this engine's arrays are dense, so a gap is
	// an empty element.
	//
	// It is also the one form that says **where the declaration would
	// land**, because it is the one whose command word decides that: written
	// from inside a function, a global takes a `-g` word ahead of the
	// cluster and a local exported name is spelled `local` rather than
	// `export`. See exportSpelledDeclaration for the measured table. That is
	// part of the shape rather than an axis of its own — the other forms'
	// shells write one text for a local and a global alike, so they have
	// nothing to disagree with here.
	DeclareListingExportSpelled
	// DeclareListingBareAssignments writes `typeset` with each flag a word
	// of its own — `typeset -x -r -i n=5` — and a name with no attributes as
	// a bare `v=1` with no command word at all. Indexed elements carry
	// subscripts only when the array has gaps.
	DeclareListingBareAssignments
	// DeclareListingCommandWord repeats the listing builtin's own word and
	// nothing else: `export -p` writes `export V='1'` and `readonly -p`
	// writes `readonly R='2'`, with no flag cluster — the word is the flag.
	DeclareListingCommandWord
	// DeclareListingPlainAssignment writes the assignment and nothing else:
	// `V='a b'`, `R=2`. No shell's `-p` writes this — a listing with no
	// command word could not be read back as a declaration — and it is what
	// ksh93 and zsh write for the *bare* `export` and `readonly`, which is
	// why it is reachable only through BareDeclarationListing.
	DeclareListingPlainAssignment
)

func (f DeclarationListingForm) String() string {
	switch f {
	case DeclareListingClustered:
		return "DeclareListingClustered"
	case DeclareListingExportSpelled:
		return "DeclareListingExportSpelled"
	case DeclareListingBareAssignments:
		return "DeclareListingBareAssignments"
	case DeclareListingCommandWord:
		return "DeclareListingCommandWord"
	case DeclareListingPlainAssignment:
		return "DeclareListingPlainAssignment"
	}
	return "DeclarationListingUnspecified"
}

// declaration is one name's state, gathered for listing.
type declaration struct {
	name  string
	value string
	// hasValue tells a value of "" apart from no value at all: a name can
	// carry an attribute and no value, and it lists without the `=`.
	hasValue bool
	arr      Array
	isArr    bool
	assoc    AssocArray
	isAssoc  bool
	// compoundVar is a ksh93 compound variable — `typeset -C`. A fourth kind
	// beside the scalar and the two arrays, and the only one whose value is
	// not in this struct: a compound's members are names of their own and are
	// read off the store when the listing is built. See
	// interp/compoundvariable.go.
	compoundVar bool
	integer     bool
	// floatExponent says the float attribute came from the `E` letter and
	// not from `F`, which is what the listing writes back. See
	// interp/floatformat.go.
	floatExponent bool
	// precision is the number that came with the float letter, which the
	// listing form that writes a number as a word of its own needs — ksh93's
	// `typeset -E 3 a=3.14`. Zero is the letter's default and is written by
	// nobody, the same shape the integer base has.
	precision int
	// isNameref says the name is a **reference** to another parameter, and
	// nameref is the name it points at. What the listing writes is the
	// reference itself and never what it reaches — `declare -n r="v"` — so
	// this is gathered from the reference table directly rather than through
	// the resolution every other reader goes through. See interp/nameref.go.
	isNameref bool
	nameref   string
	readonly  bool
	exported  bool
	lower     bool
	upper     bool
	capital   bool
	// hidden says the `-H` attribute is on the name. What it *does* is the
	// dialect's — see Semantics.DeclareHideValueLetter — so this field is
	// the record and hidesTheValue below is one of the two readings of it.
	//
	// The other reading is ksh93's, where the attribute is inert and is said
	// back as a letter: BareAssignments writes `-H` for it, between the kind
	// letter and the case letters. Clustered never sees it — bash has no
	// `-H` at all.
	hidden bool
	// hidesTheValue is the reading under which `-H` withholds the value: the
	// name is declared, holds what it holds and reads back exactly as it
	// would without the letter, and only a listing that would have written
	// `=value` writes the bare name instead. The letter itself is never
	// written back under this reading.
	//
	// Only the forms that shell reaches honor it — ExportSpelled, its
	// `typeset -p` and `readonly -p`; CommandWord, its `export -p`;
	// PlainAssignment, its bare `export` and `readonly`; and the bare `set`
	// listing in setlisting.go.
	//
	// Read rather than derived at each site, because the same struct is
	// rendered by five forms and a policy consulted five times is a policy
	// four of them will eventually stop consulting.
	hidesTheValue bool
	// tied is the tie this name is half of — `typeset -T` — and holds when
	// either half is being listed. The entry is not the ordinary one: it
	// names *both* parameters, always writes the array's elements as the
	// value whichever half was asked for, and ends with the separator where
	// that is not the default. Measured: `typeset -T SCA sca=( p q )` for
	// the scalar and `typeset -aT SCA sca=( p q )` for the array, and
	// `typeset -T C1 c1=( 1 2 ) '#'` when the separator is not `:`.
	tied   tie
	hasTie bool
	// unique is `typeset -U`, which the one shell with the attribute writes
	// back as a letter — last of them all, after export.
	//
	// Only the ExportSpelled form writes it, which is where that shell's
	// `typeset -p` and `readonly -p` land. Its `export -p` writes letters
	// too — `export -i n=5` — but the CommandWord form this engine gives
	// that builtin writes none at all, for `-i` as much as for `-U`, so
	// spelling `U` there alone would be one letter of a listing that is
	// missing the rest. Clustered and BareAssignments are the shells with
	// no such attribute.
	unique bool
	// traced is `typeset -t`, which every shell that spells the letter
	// writes back and none of them lets change a value. Where it sits among
	// the other letters is the one thing that differs, and each listing form
	// spells its own order — see flagLetters and exportSpelledDeclaration.
	traced bool
	// base is the output base an integer name renders in — `typeset -i16`.
	// Zero where none was named, which is every name in a dialect without
	// the feature. The two forms that write it write it differently and one
	// of them decodes the value: see exportSpelledDeclaration and
	// bareAssignmentDeclaration.
	base int
	// unset says the name carries a compound attribute and holds nothing —
	// the state a valueless `local -a q` leaves where a declared name
	// without a value is *unset* rather than empty. The two facts are wanted
	// at once and only one of them is a value: `${q-UNSET}` fires its
	// default, and `declare -p q` still writes the attribute the
	// declaration recorded.
	//
	// Only reachable where DeclaredNameWithoutValueIsEmpty is no. The
	// dialect that sets a declared name empty has an empty array to print
	// and never arrives here, which is why `typeset -ar a=(  )` keeps its
	// parentheses (#1664, #1554).
	unset bool
	// float is `typeset -F`, the float attribute, whose letter this writes
	// and whose *precision* it does not: measured 2026-09-07, zsh lists
	// `typeset -F 3 x=3.14159` back as `typeset -F x=3.142` — the places are
	// in the value and the number is not written again. ksh93 writes it as a
	// word of its own, `typeset -F 3 x=3.142`, and does not have the
	// attribute here — see #1461.
	float bool
	// silent is the produced parameter one shell keeps out of a listing while
	// still knowing the name: nothing is written and the status is 0, which
	// is neither a row nor a refusal. See ProducedDeclaration.Silent.
	silent bool
	// width is the width attribute — `typeset -L 5 s` and its two
	// neighbors — carrying which of the three letters was written and how
	// wide. hasWidth says the name carries one at all. Unlike the float
	// precision, the number *is* written back here: measured 2026-09-12, zsh
	// lists `typeset -L 5 a=ab` as `typeset -L5 a=ab`, and the value it
	// writes is the raw text rather than the padded one, because that shell
	// pads on the read. See fieldwidth.go.
	width    fieldWidth
	hasWidth bool
	// inAFunction and localHere are not attributes of the name at all: they
	// are where the listing is being written *from*, and a form that claims
	// to be re-executable needs them. Inside a function, a declaration lands
	// on a local unless it says otherwise, so the same text that recreates a
	// global at the top level shadows it here — which is the one case the
	// promise breaks. See exportSpelledDeclaration, the one form measured to
	// say it.
	//
	// localHere is the *innermost* scope's and not "some scope has it": a
	// local of a calling function is not this function's to redeclare, and
	// the listing writes it as a global. Measured 2026-09-12, zsh 5.9.2 —
	// `g(){ typeset -p L }; f(){ local L=1; g }; f` writes `typeset -g L=1`.
	inAFunction bool
	localHere   bool
	// declaredOnly says an empty array or table came from a declaration's
	// letters with nothing written to it, which one listing tells apart from
	// an emptied one and the others do not. Meaningless where the value has
	// elements. See compounddeclaredonly.go.
	declaredOnly bool
}

// declarationOf gathers what the runner knows about a name. The second result
// reports whether it knows anything at all — an attribute with no value is
// still a declaration, and `declare -p` is how scripts ask.
func (r *Runner) declarationOf(name string) (declaration, bool) {
	d := declaration{
		name:    name,
		integer: r.integer[name],
		// A name restricted mode froze is readonly to the machinery and not to
		// the listing, in the dialect that does not call the mode's freeze a
		// readonly: measured, `readonly -p` in a restricted ksh93 lists none
		// of them where bash lists `declare -r ENV`. See
		// Semantics.RestrictedFreezeIsAReadonly and Runner.restrictedFreeze.
		readonly:    r.readonly[name] && !r.restrictedFreeze(name),
		exported:    r.isExported(name),
		lower:       r.lowered[name],
		upper:       r.uppered[name],
		capital:     r.capitalized[name],
		hidden:      r.hidden[name],
		unique:      r.unique[name],
		traced:      r.traced[name],
		base:        r.integerBase[name],
		inAFunction: len(r.scopes) > 0,
		localHere:   r.localInTheInnermostScope(name),

		declaredOnly: r.declaredOnlyCompound[name],
	}
	d.nameref, d.isNameref = r.nameref[name]
	d.compoundVar = r.isCompoundVariable(name)
	_, d.float = r.floatPrecision[name]
	d.floatExponent = r.floatExponent[name]
	d.precision = r.floatPrecision[name]
	d.width, d.hasWidth = r.fieldWidth[name]
	d.tied, d.hasTie = r.tieOf(name)
	// Which of the two things `-H` is here. The attribute is recorded the
	// same way for both readings — see interp/declarehide.go — and this is
	// where the dialect is asked what it stands for, once, for every listing
	// form that renders this struct.
	d.hidesTheValue = d.hidden &&
		r.sem().DeclareHideValueLetter == DeclareHideValueLetterHidesTheValue
	attributed := d.integer || d.float || d.readonly || d.exported || d.lower ||
		d.upper || d.capital || d.hidden || d.unique || d.traced || d.hasWidth
	producedIsAnArray, producedListsElements := false, false
	if d.isNameref {
		// A reference lists as itself — `declare -n r="v"` — and the tables
		// below are the *target's*, not this name's. Ahead of every one of
		// them because asking them would resolve: assocFor and the rest go
		// through the reference, which is right for a read and wrong for a
		// listing that is supposed to say where the reference points.
		//
		// The value is the target's name, and it is written even when the
		// reference has nothing to point at — `typeset -n r` lists as
		// `declare -n r` with no `=`, which is what hasValue being false
		// gives it.
		d.value, d.hasValue = d.nameref, d.nameref != ""
		return d, true
	}
	if pd, ok := r.producedDeclaration(name); ok {
		// A produced parameter the dialect has said how to list. Its letters
		// are stated rather than read off an attribute table, because there
		// is no entry in one to read — see SetDynamicDeclaration — and they
		// are taken *over* whatever the tables happened to hold, so that one
		// answer to "how does this name list" cannot come from two places.
		d.integer, d.base, d.float = pd.Integer, pd.Base, pd.Float
		d.silent = pd.Silent
		// See ProducedDeclaration.Array: the operand-less listing withholds a
		// *reading* and an array's elements are not one, so the two produced
		// branches below take this rather than the withholding on its own.
		producedIsAnArray = pd.Array
		producedListsElements = pd.ListsItsElements
		attributed = true
	}
	if r.removed[name] {
		// The name holds nothing — `unset` took the value away, or a
		// declaration hid it — but the compound attribute a declaration
		// recorded outlives the value, and it is the store that carries it.
		// Both facts go out together: `unset` on the declaration and the
		// kind beside it. See declaration.unset.
		if _, ok := r.AssocArrays[name]; ok {
			d.isAssoc, d.unset = true, true
			return d, true
		}
		if _, ok := r.Arrays[name]; ok {
			d.isArr, d.unset = true, true
			return d, true
		}
		// Only a surviving attribute keeps a name with neither listable —
		// or a bare declaration made *after* the removal, which is a name
		// brought back into being rather than one that survived it:
		// measured 2026-09-15, `declare xyz; unset xyz; declare xyz;
		// declare -p xyz` writes `declare -- xyz` in bash 5.3.20 where
		// `declare xyz; xyz=v; unset xyz` is `xyz: not found`. `unset`
		// clears the record with the attributes, so what is read here can
		// only be a later declaration's.
		return d, attributed || r.bareDeclarationListed(name)
	}
	// A compound variable, ahead of every table because it is in none of
	// them: its value is the members stored under it, and markCompoundVariable
	// took the old scalar or array away when the name became one.
	if d.compoundVar {
		return d, true
	}
	// The array tables answer ahead of Vars, which mirrors an array's first
	// element — the same order every read follows.
	if a, ok := r.assocFor(name); ok {
		d.assoc, d.isAssoc = a, true
		return d, true
	}
	if a, ok := r.Arrays[name]; ok {
		d.arr, d.isArr = a, true
		return d, true
	}
	// A produced array, asked after the stored one for the reason arrayElems
	// gives: a script that has assigned to the name gets its own value back.
	//
	// Here so that the *letter* is right. Until this existed, the one dialect
	// that marks a produced array readonly listed `typeset -r keymaps` where
	// the shell it models writes `typeset -ar keymaps`. The produced
	// association a line above had the same shape and was already right,
	// which is why the array reading was the one that went missing.
	//
	// **Only when an attribute has already put the name in a listing**, which
	// is what `attributed` says. A produced name is in none of the tables
	// declarableNames walks, so a listing reaches one only through an
	// attribute — and a produced array with no attributes must stay
	// undeclared, which is the answer `$funcstack` gives and had before this.
	if produce, ok := r.DynamicArrays[name]; ok && attributed &&
		(!r.listingDrawsNoReading || producedIsAnArray) {
		elems := produce(r)
		if elems == nil {
			// **Nil is absent and empty is empty**, which is the contract a
			// producer already answers an expansion under — bash's FUNCNAME
			// returns nil outside a call because the parameter does not exist
			// there, and returns no elements for a call with none. A listing
			// has to say the same two things, and it has the shapes for it:
			// `declare -a FUNCNAME` with no `=` against `declare -a
			// BASH_ARGV=()`, measured 2026-09-18 on bash 5.3.20 (#3099).
			d.isArr, d.unset = true, true
			return d, true
		}
		if r.listingDrawsNoReading && !producedListsElements {
			// The kind and no elements, which is what the operand-less
			// listing writes for a produced array it withholds: the name
			// exists — so there is an `=` — and what it holds is not written.
			// See ProducedDeclaration.ListsItsElements for the one that is.
			d.isArr = true
			return d, true
		}
		a := make(Array, len(elems))
		for i, v := range elems {
			a[i] = Scalar(v)
		}
		d.arr, d.isArr = a, true
		return d, true
	}
	// The pipeline record, which is produced by a path of its own rather than
	// through DynamicArrays — the core keeps it and the dialect only names it
	// (see interp/pipestatus.go). The listing has to reach it separately for
	// that reason, and on the same `attributed` terms as the two branches
	// above: `declare -p PIPESTATUS` is `declare -a PIPESTATUS=([0]="0")` in
	// bash 5.3.20 and was `PIPESTATUS: not found` here, from a name the same
	// shell had just expanded an element of (#3099).
	if elems, produced := r.pipelineStatuses(name); produced && attributed &&
		(!r.listingDrawsNoReading || producedIsAnArray) {
		a := make(Array, len(elems))
		for i, v := range elems {
			a[i] = Scalar(v)
		}
		d.arr, d.isArr = a, true
		return d, true
	}
	if v, ok := r.Vars[name]; ok {
		d.value, d.hasValue = v, true
		return d, true
	}
	// A produced *scalar*, on the same terms as the produced array above and
	// asked after the stored value for the same reason. The two branches are
	// one rule read twice, and the scalar half was the one still missing: a
	// dialect that marks a produced scalar readonly put the name into the
	// listing through the attribute and then had nothing to print beside it,
	// so `$ARGC` — which is `$#` and was three at the time — listed as
	// `readonly ARGC=''`. An empty value is not a smaller answer than the
	// real one, it is a different and false one, and a script reading the
	// listing back would set the name to nothing.
	//
	// Guarded by `attributed` exactly as the array is, which is also what
	// keeps a producer with a side effect out of a listing: `RANDOM` carries
	// no attribute, so no listing reaches it and no listing draws a number
	// from it that the next read would not repeat.
	//
	// The guard had a cost and it was on record here for a while: a listing
	// that *named* an unattributed producer found nothing either, so
	// `typeset -p SECONDS` was `no such variable` where zsh writes
	// `typeset -i10 SECONDS=0`. That was #1687, and the note is kept because
	// the way out of it is the thing to reach for next time rather than the
	// obvious one. Lifting the guard was never the answer — it would have let
	// a bare listing draw a number out of every producer on the runner, and
	// it still could not have written the `-i10`, which is an integer
	// attribute and a base. What closed it instead was giving a producer
	// something to *carry*: SetDynamicDeclaration and ProducedDeclaration
	// (#2510, #2552, #2728), after which `SECONDS` and `RANDOM` are
	// attributed like any other name and reach this branch through the guard
	// rather than around it.
	if produce, ok := r.Dynamic[name]; ok && attributed && !r.listingDrawsNoReading {
		d.value, d.hasValue = produce(r), true
		return d, true
	}
	if _, ok := r.Dynamic[name]; ok && attributed {
		// The listing has said it will not write a drawn reading, so the
		// producer is not asked — see listedDeclarationOf, and note that a
		// producer is not always free to ask: bash's generator advances on
		// every draw, so a listing that drew one would change the sequence a
		// script afterwards sees. The row is still known; what it carries is
		// the caller's to fill in.
		return d, true
	}
	if r.bareDeclarationListed(name) {
		// Declared with no letters and no value, so there is neither a
		// value to write nor a letter to write it with: the row is the name
		// alone. Last, because every table above it holds something this
		// one does not — an assignment after the declaration gives the row
		// its value back, and the record is what is left when none of them
		// answer. See baredeclaration.go.
		return d, true
	}
	if v, ok := r.inheritedValue(name); ok {
		// Born in the environment and never assigned to, so the value is
		// still the one that came in. Whether it is exported was settled
		// above: it is, unless something took the attribute off.
		d.value, d.hasValue = v, true
		return d, true
	}
	return d, attributed
}

// declarableNames is every name the listing with no operands walks: the
// runner's own tables, the attribute records, and the environment the shell
// was born with. Sorted, because two of the three shells with `-p` sort and
// the third promises nothing. Produced parameters are deliberately absent —
// a value that is made up on each read is not state a listing could carry.
func (r *Runner) declarableNames() []string {
	seen := map[string]bool{}
	for name := range r.Vars {
		seen[name] = true
	}
	for name := range r.Arrays {
		seen[name] = true
	}
	for name := range r.AssocArrays {
		seen[name] = true
	}
	for name, on := range r.exported {
		if on {
			seen[name] = true
		}
	}
	for name := range r.readonly {
		if r.restrictedFreeze(name) {
			// A name restricted mode froze, in the dialect that does not call
			// that readonly: measured, `readonly -p` in a restricted ksh93
			// lists nothing where bash lists `declare -r ENV`. The mark is
			// shared machinery and the listing is the observable, so this is
			// the one place the two part company. See
			// Semantics.RestrictedFreezeIsAReadonly.
			continue
		}
		seen[name] = true
	}
	for name := range r.integer {
		seen[name] = true
	}
	for name := range r.floatPrecision {
		seen[name] = true
	}
	for name := range r.fieldWidth {
		seen[name] = true
	}
	for name := range r.lowered {
		seen[name] = true
	}
	for name := range r.uppered {
		seen[name] = true
	}
	for name := range r.hidden {
		seen[name] = true
	}
	for name := range r.unique {
		seen[name] = true
	}
	for name := range r.traced {
		seen[name] = true
	}
	for name := range r.declaredBare {
		// A name with no value and no attribute is in none of the tables
		// above, so the bare listing reaches it only from here.
		//
		// Asked rather than added, which is not the economy it looks like:
		// the record is kept for every dialect, so a dialect whose listing
		// has no row for it would collect the name here, find nothing in
		// declarationOf, and reach the *missing name* path — which is a
		// second axis, unanswered in the two dialects that have no
		// declaration listing at all. Measured: `f(){ local x; export -p;
		// }` in the dash dialect complained about a name nobody had asked
		// about.
		if r.bareDeclarationListed(name) {
			seen[name] = true
		}
	}
	for name := range r.unsetLeftItDeclared {
		// The same record reached the other way, and in the walk for the
		// same reason: measured 2026-09-21 on bash 5.3.20, a whole-table
		// `declare -p` inside `f() { local v; unset v; … }` writes the row
		// `declare -- v`, and so does a bare `local`. Asked through the
		// same gate, so a dialect with no row for it collects nothing.
		if r.bareDeclarationListed(name) {
			seen[name] = true
		}
	}
	for name := range r.nameref {
		// A reference is a name the shell has rather than a value it stored,
		// so it is in none of the value tables and in none of the attribute
		// ones either — the `n` letter is recorded here and nowhere else.
		// Without this the full listing reached a nameref only when the cell
		// under the name happened to be holding something, which is the state
		// a `-n` declaration is now measured *not* to leave (#3084): bash
		// 5.3.20 writes `declare -n r="v"` for `v=1; declare -n r=v` whether
		// or not `r` held a value before the line, and this shell wrote it
		// only in the second case.
		seen[name] = true
	}
	for name, on := range r.compoundVariable {
		if on {
			seen[name] = true
		}
	}
	for k := range r.inheritedEnv {
		// isNameLike keeps the entries that are variables: an exported
		// function travels in the environment under a decorated name no
		// shell lists as one.
		if isNameLike(k) {
			seen[k] = true
		}
	}
	for name := range r.absentParams {
		// A parameter the dialect names and this shell has not got is not a
		// declaration this shell can list, whatever attribute record it
		// carries. It has one now — the absent names are frozen against a
		// write, which is what the shell being modeled does (#1604) — and
		// the readonly table is one of the tables this walk collects from,
		// so without this the bare `readonly` listing would have grown
		// fifteen rows naming parameters that are not there. Measured: zsh's
		// own `readonly` writes none of them.
		delete(seen, name)
	}
	for name := range seen {
		// A compound's members are listed *inside* it and not beside it:
		// `c=(a=1 b=2); typeset -p` writes one line there, `typeset -C
		// c=(a=1;b=2)`, and never a `c.a=1` of its own. Named explicitly they
		// still list — `typeset -p c.a` is `c.a=1` — which is why this is a
		// filter on the walk rather than a rule about the name.
		if r.memberOfACompoundVariable(name) && !r.listsBesideItsElement(name) {
			delete(seen, name)
		}
		// And the namespace of a compound an *element* holds is not a row
		// either, for the same reason read the other way round: the array's
		// own row already writes that value whole, so `a[1]` beside
		// `typeset -a a=([1]=(p=1;q=2))` would write it twice. Named
		// explicitly it still lists — `typeset -p a[1]` is `typeset -C
		// a[1]=(p=1;q=2)` — which is the filter-on-the-walk shape again.
		// See interp/subcompound.go.
		if isElementNamespace(name) {
			delete(seen, name)
		}
	}
	for name := range seen {
		// A name whose value was taken away is a row only where something of
		// the declaration outlived it. Asked of declarationOf rather than of
		// the attribute tables directly, which is the same question it
		// already answers for `-p` — and asking it twice is what left the
		// *compound* half out: a valueless `local -a q` keeps its kind and
		// no scalar attribute, so the list this built dropped it while
		// `declare -p q` wrote `declare -a q` from the same state (#1868).
		if !r.removed[name] {
			continue
		}
		if _, known := r.declarationOf(name); !known {
			delete(seen, name)
		}
	}
	return sortedNames(seen)
}

// declarePrint is the `-p` of `declare` and `typeset`: the named
// declarations, or every one the runner knows when no name is given.
func (r *Runner) declarePrint(names []string) int {
	return r.declarePrintForm(names, r.sem().DeclareListing, true, nil)
}

// attributeFiltered says a filtered walk is one an *attribute letter* asked
// for — `declare -a`, `declare -A`, `declare -i` — rather than one a builtin
// is for, which is `export -p` and `readonly -p`.
//
// The difference decides whether the produced parameters are in the walk at
// all, and it is a measurement rather than a convenience. 2026-09-20, one
// letter per script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`, bash
// 5.3.20 in a shell that has read nothing:
//
//	declare -a   BASH_ARGC BASH_ARGV BASH_LINENO BASH_SOURCE DIRSTACK FUNCNAME GROUPS
//	declare -A   BASH_ALIASES BASH_CMDS
//	declare -i   BASHPID RANDOM SRANDOM
//	declare -x   no produced name
//	declare -r   no produced name
//
// So the letters that select a *kind* reach them and the two builtins do not,
// which is what keeps `export -p` and `readonly -p` from asking
// Semantics.ProducedParameterListing — an axis whose answer neither of them
// would use.
type attributeFiltered bool

// declarePrintForm lists declarations in the given form, walking only the
// names the filter admits when no operands narrow it — which is how
// `export -p` lists the exported names alone.
func (r *Runner) declarePrintForm(names []string, form DeclarationListingForm, dashP bool, keep func(declaration) bool) int {
	return r.declarePrintFiltered(names, form, dashP, keep, false)
}

// declarePrintFiltered is declarePrintForm with [attributeFiltered] stated:
// byLetter says the filter came from an attribute letter, so the produced
// parameters are in the walk and the filter decides which of them stay.
func (r *Runner) declarePrintFiltered(names []string, form DeclarationListingForm, dashP bool,
	keep func(declaration) bool, byLetter attributeFiltered,
) int {
	if form == DeclarationListingUnspecified {
		r.diagf("%s\n", r.unanswered("how a declaration is listed back"))
		r.status = 2
		r.unspecified = true
		return 2
	}
	status := 0
	filtered := false
	// The produced parameters this listing added on top of the walk, and
	// whether the dialect wants their reading written. See
	// Semantics.ProducedParameterListing: the value is a per-dialect fact
	// and the *names* are the registering dialect's, so a shell that has
	// registered none reaches none of this and is never asked the axis.
	var produced map[string]bool
	var listing ProducedListing
	if len(names) == 0 {
		// No operand, so this is the shape that answers differently about the
		// running command's own assignment prefix — see
		// interp/prefixlisting.go.
		defer r.walkingTheWholeTable()()
		names = r.declarableNames()
		filtered = keep != nil
		if !filtered || bool(byLetter) {
			// The unfiltered `-p`, and the walks an attribute *letter* asked
			// for. `export -p` and `readonly -p` come through here with a
			// filter and without the letter, and no produced parameter
			// carries either attribute in any column of the panel — so they
			// are left out rather than admitted and then rejected, which
			// would make the axis a question those two builtins ask and
			// never use.
			//
			// A letter is the other way round: `declare -a` writes seven
			// produced names in bash 5.3.20, `declare -A` two and `declare
			// -i` three, and this engine wrote none of them while its own
			// unfiltered `declare -p` wrote every one — see
			// [attributeFiltered] for the measurement.
			if add := r.producedListingNames(names); len(add) > 0 {
				listing = r.producedListing()
				if r.unspecified {
					return r.status
				}
				produced = make(map[string]bool, len(add))
				seen := make(map[string]bool, len(names)+len(add))
				for _, name := range names {
					seen[name] = true
				}
				for _, name := range add {
					seen[name], produced[name] = true, true
				}
				names = sortedNames(seen)
			}
		}
	}
	for _, name := range names {
		d, known := r.listedDeclarationOf(name, produced[name], listing)
		if known && r.listingIsALocalsOwn {
			// The `local` word's own listing, in the dialect where it answers
			// for the running call rather than for the shell: a name this
			// call did not declare is not there to be written, so it takes
			// the missing-name route below rather than a second refusal of
			// its own. See interp/localbuiltin.go.
			//
			// The refusal check is inside this branch rather than after it,
			// because the ask only happens here: a listing no `local`
			// asked for must not be ended by a flag some earlier line set.
			skip := r.localListingSkipsAName(name)
			if r.unspecified {
				return r.status
			}
			if skip {
				known = false
			}
		}
		if known && filtered && !keep(d) && !keep(r.prefixCountsAsExportedInAFilteredListing(name, d)) {
			continue
		}
		if !known {
			if r.listingWalksTheWholeTable {
				// Nobody asked for this name: the walk collected it and the
				// prefix reading then said the shell has no such variable —
				// `z=9 export -p` in the column that answers from the shell's
				// own table. A row that is not there is not a missing operand
				// and is written as nothing, in silence.
				continue
			}
			if r.valuelessRecordIsStillAName(name) {
				// The name is one this shell *has* — a local whose value an
				// `unset` took, in a dialect whose listing writes no row for
				// it — so it is written as nothing at 0 rather than reported
				// as missing. Ahead of the refusal below, because that one
				// is about a name the shell has never heard of. See
				// interp/baredeclaration.go.
				continue
			}
			if r.unspecified {
				return r.status
			}
			// A missing name is reported or passed over in silence, and the
			// silence is measured rather than a shortcut: one shell prints
			// nothing and answers 0 however many names were missing.
			if r.ask(r.sem().DeclarePrintReportsAMissingName, "`-p` reporting a name that is not there") {
				r.diagf("%s: %s\n", r.inBuiltin,
					Wording(r.diag().DeclareNoSuchVariable, "%[1]s: not found", name))
				status = 1
			}
			if r.unspecified {
				return r.status
			}
			continue
		}
		if d.silent && dashP {
			// Known to the listing and written by it as nothing, at 0 — see
			// ProducedDeclaration.Silent. Ahead of the row rather than
			// inside the renderer, because there is no row: a form that
			// wrote an empty line would be a shape no shell produces.
			//
			// The silence belongs to the **`-p` word** and not to the name,
			// and it is asked of the word rather than of the shape that came
			// back because two dialects give the two forms one shape. zsh
			// 5.9.2, 2026-09-12, one run, `ARGC`:
			//
			//	typeset -p ARGC   nothing, status 0
			//	typeset -p        no row
			//	readonly -p       no row
			//	typeset -r        ARGC=0
			//	readonly          ARGC=0
			//	typeset           integer 10 readonly ARGC=0
			//
			// `LINENO` answers the same six ways. So a name silent to `-p`
			// is still in the listing the bare word writes, and the
			// bare-assignment form is the one this engine renders those with.
			continue
		}
		r.printf("%s\n", r.listedDeclaration(form, d))
	}
	return status
}

func (r *Runner) listedDeclaration(form DeclarationListingForm, d declaration) string {
	switch form {
	case DeclareListingExportSpelled:
		return r.exportSpelledDeclaration(d)
	case DeclareListingBareAssignments:
		return r.bareAssignmentDeclaration(d, ListedValueAlone)
	case DeclareListingCommandWord:
		return r.commandWordDeclaration(d)
	case DeclareListingPlainAssignment:
		return r.plainAssignmentDeclaration(d)
	}
	return r.clusteredDeclaration(d)
}

// commandWordDeclaration is DeclareListingCommandWord — see the constant.
//
// The value is listedDeclarationValue's rather than declareQuoted's, which is
// the same correction plainAssignmentDeclaration took in #1868 and for the
// same reason: a compound has elements to write and this wrote a bare name.
// Measured 2026-09-12 on ksh93u+ — `export m=([k]=v)`, `export g=([3]=x)`,
// `readonly A=(1 2)` — where this engine wrote `export m`, and `export
// h=16#ff` for a based integer where it wrote the `#` quoted. Both are the
// value half of that shell's own `-p`, which is what a listing claims to be.
func (r *Runner) commandWordDeclaration(d declaration) string {
	head := r.inBuiltin + " " + d.name
	if d.hidesTheValue || d.unset {
		// Nothing to write a value from: `-H` withholds it under the reading
		// that does, and a typed name whose value was taken away has none.
		return head
	}
	if !d.hasValue && !d.isArr && !d.isAssoc {
		return head
	}
	return head + "=" + r.listedDeclarationValue(d)
}

// plainAssignmentDeclaration is DeclareListingPlainAssignment — see the
// constant. A name carrying an attribute and no value still lists, and lists
// as a bare name, because there is no word left to say the attribute with.
//
// A *compound* lists its elements, which had gone missing: this wrote a bare
// `ea` for an exported array where zsh writes `ea=( 1 2 )` and ksh93 writes
// `g=([3]=x)`. Measured from a bare `export` and a bare `readonly` in both,
// and it is the same row `typeset -a` writes there (#1868) — the two reach
// this function by different routes and there is one row.
//
// The value is listedDeclarationValue's rather than declareQuoted's for the
// same reason: a based integer lists as the decimal it stands for in one of
// these shells and as the based text it holds in the other, and that
// difference is already written down once.
func (r *Runner) plainAssignmentDeclaration(d declaration) string {
	if d.hidesTheValue || d.unset {
		// Nothing to write a value from: `-H` withholds it under the reading
		// that does, and a typed name whose value was taken away has none.
		// Measured on the second —
		// ksh93 writes a bare `u` for `typeset -a u; typeset -a`, where the
		// shell that declares a name *empty* instead has an empty array to
		// print and writes `u=(  )`.
		return d.name
	}
	if !d.hasValue && !d.isArr && !d.isAssoc {
		return d.name
	}
	return d.name + "=" + r.listedDeclarationValue(d)
}

// listedDeclarationValue spells a declaration's value alone — no name, no
// command word — in the shape this dialect's `declare -p` gives a compound
// one. For a listing whose attributes are words rather than flags, and for
// the one that has no room for them at all: see plainAssignmentDeclaration.
//
// The dialect is asked, which it was not while the only callers were one
// shell's. Measured 2026-09-10 from `typeset -a` and `typeset -i` with no
// names: zsh writes `g=( ” ” x )` and `h=255` for a based name, where
// ksh93 writes `g=([3]=x)` and `h=16#ff` — each of them the value half of
// that shell's own `-p`, which is what this claims to be.
func (r *Runner) listedDeclarationValue(d declaration) string {
	if r.sem().DeclareListing == DeclareListingBareAssignments {
		value, _ := r.bareAssignmentValue(d, ListedValueAlone)
		return value
	}
	switch {
	case d.isAssoc:
		if len(d.assoc) == 0 {
			return "( )"
		}
		return "( " + strings.Join(r.quotedTablePairs(d, r.clusteredKey), " ") + " )"
	case d.isArr:
		return "( " + strings.Join(r.quotedArrayElems(d), " ") + " )"
	case d.base != 0:
		// A based integer lists here as the plain decimal number it stands
		// for, and bare: measured on zsh 5.9.2, `typeset -i16 h=255` writes
		// `integer 16 h=255` and `typeset -i8 o=8` writes `integer 8 o=8`,
		// where the *value* the name holds is `16#FF` and `8#10` — which is
		// what `echo $h` gives back. The base is already a word of the
		// attributes; repeating it in the value quoted the `#` and wrote
		// `h='16#FF'`. Negatives keep their sign, `n=-255`.
		//
		// The same decoding `typeset -p` makes, and deliberately the same
		// function: the two forms differ in the quoting around the number
		// and not in the number.
		return r.listedInDecimal(d)
	}
	return r.declareQuoted(d.value, ListedValueAlone)
}

// declareQuoted spells one listed value in the dialect's declaration style,
// which its other listings do not decide. Where the value stands is the
// caller's to say — see ListedValuePlace, and listedAssignmentHead for the one
// thing it changes.
func (r *Runner) declareQuoted(v string, place ListedValuePlace) string {
	return r.quoteListedValue(r.sem().DeclareValueQuoting, "`declare -p`", v, place)
}

// clusteredDeclaration is DeclareListingClustered — see the constant.
func (r *Runner) clusteredDeclaration(d declaration) string {
	flags := d.flagLetters()
	head := "declare -" + flags
	if flags == "" {
		head = "declare --"
	}
	head += " " + d.name
	switch {
	case d.unset:
		// The letters and nothing else: the name is typed and holds no
		// value, so there is no `=` to write. Ahead of the two compound
		// branches because it is those the name is typed as.
		return head
	case d.isAssoc:
		if len(d.assoc) == 0 && d.declaredOnly {
			// A table the letters declared and nothing has written to, which
			// this engine writes with no value at all. An *emptied* one is
			// `=()` and falls through — the distinction is the assignment
			// rather than the emptiness, and the two spellings are not
			// interchangeable when they are read back: `declare -A m`
			// re-declares where `declare -A m=()` empties. See
			// compounddeclaredonly.go.
			return head
		}
		var b strings.Builder
		b.WriteString(head)
		b.WriteString("=(")
		for _, k := range r.assocKeys(d.name, d.assoc) {
			// Sorted keys are this implementation's choice: the shells
			// promise no order at all, and a deterministic listing is worth
			// having. See AssocArray.keys.
			b.WriteString("[" + r.clusteredKey(k) + "]=" + r.listedElement(d.assoc[k], ListedValueAlone) + " ")
		}
		b.WriteString(")")
		return b.String()
	case d.isArr:
		if len(d.arr) == 0 && d.declaredOnly {
			// The indexed half of the same distinction, and it moves the
			// other way: `declare -a q` was listing as `declare -a q=()`
			// here. See the association above.
			return head
		}
		elems := make([]string, 0, len(d.arr))
		for _, i := range d.arr.subscripts() {
			elems = append(elems, fmt.Sprintf("[%d]=%s", i, r.listedElement(d.arr[i], ListedValueAlone)))
		}
		return head + "=(" + strings.Join(elems, " ") + ")"
	case d.hasValue:
		return head + "=" + r.declareQuoted(d.value, ListedValueAlone)
	}
	return head
}

// listedInDecimal is a declaration's value with an output base decoded away.
// Its own function because one listing form writes the number and the other
// writes the text the name is holding, and both need to say which.
func (r *Runner) listedInDecimal(d declaration) string {
	if d.base == 0 {
		return d.value
	}
	sign, text := "", d.value
	if rest, cut := strings.CutPrefix(text, "-"); cut {
		sign, text = "-", rest
	}
	digits, ok := digitsAfterTheBaseMark(text, d.base)
	if !ok {
		return d.value
	}
	v, err := strconv.ParseInt(digits, d.base, 64)
	if err != nil {
		return d.value
	}
	return sign + itoa(int(v))
}

// digitsAfterTheBaseMark takes the mark off a rendered value, whichever of the
// spellings it was written in.
//
// Written to read all of them rather than the one the shell would write now,
// because the two are not the same question: `typeset -i16 a=108` under
// `unsetopt c_bases` stores `16#6C`, and a `typeset -p a` after a later
// `setopt c_bases` still has to decode the text that is there (#4502). See
// Semantics.IntegerBaseMarkIsCSpelled.
func digitsAfterTheBaseMark(text string, base int) (string, bool) {
	if at := strings.IndexByte(text, '#'); at >= 0 {
		return text[at+1:], true
	}
	switch base {
	case 16:
		if rest, cut := strings.CutPrefix(text, "0x"); cut {
			return rest, true
		}
		if rest, cut := strings.CutPrefix(text, "0X"); cut {
			return rest, true
		}
	case 8:
		// A leading zero, which is base eight's C spelling and needs a digit
		// after it — `00` is zero and `0` on its own is a value with no mark.
		if len(text) > 1 && text[0] == '0' {
			return text[1:], true
		}
	}
	return "", false
}

// clusteredKey spells a subscript in the clustered form: bare when it is
// plain, `$'...'` when it holds a control character, double-quoted otherwise —
// the same reach the form's values make, without the always.
//
// Bare by the *declaration* rule and not the plainest one, because the shell
// that writes this form judges a key exactly as it judges a scalar's value in
// a bare `set`: `[a#b]` and `[a~b]` are bare there where `["#a"]` and
// `["~b"]` are quoted, and the position rules behind that are
// Semantics.ListedHashIsBareUnlessItOpensTheValue and
// Semantics.ListedTildeIsBareWhereItCannotExpand. Reading listedValueIsBare here
// quoted both of the bare ones (#2298).
func (r *Runner) clusteredKey(k string) string {
	switch {
	case hasControl(k):
		return r.dollarQuoted(k)
	case wholeArraySubscriptAsAKey(k):
		return doubleQuoted(k)
	case r.valueListsBare(k):
		return k
	}
	return doubleQuoted(k)
}

// wholeArraySubscriptAsAKey reports whether a key is one of the two words that
// mean *the whole array* when they stand alone in brackets.
//
// Both are ordinary characters in a key and both list bare without this, which
// is worse than cosmetic: a listing is meant to be read back, and `[@]=at` fed
// to the shell again is the whole-array subscript rather than the key `@` the
// table is holding. Measured 2026-09-14, bash 5.3.15 — the one column that
// will store such a key at all, ksh93 and zsh each refusing the assignment by
// name — writes `declare -A r=(["@"]="at" )` (#2749).
//
// The whole key and not a character in it: `a@b` and `a*b` are bare and quoted
// respectively there for the ordinary reasons, and neither is this.
func wholeArraySubscriptAsAKey(k string) bool {
	return k == "@" || k == "*"
}

// numberedLetterEndsTheWord breaks a cluster after the letter carrying a
// number, because that is how the shell that writes numbers this way reads
// them back.
//
// A number-taking letter ends its option word in that shell — the rule
// Semantics.DeclareOptionsTakingANumber is about — so a cluster with the rest
// of the letters trailing the digits is not the declaration it claims to be:
// `-i16r` re-read gives the base `16r`, not a frozen name in base 16.
// Measured 2026-09-12 on zsh 5.9.2, `typeset -ri 16 v=255` lists as
// `typeset -i16 -r v=255` and `typeset -rL 3 a=abcd` as `typeset -L3 -r
// a=abcd`; this engine wrote `typeset -i16r v=255` (#1461).
//
// at is one past the last digit, and zero means no letter carried a number —
// which is every listing in every other dialect and nearly every one in this.
func numberedLetterEndsTheWord(flags string, at int) string {
	if at <= 0 || at >= len(flags) {
		return flags
	}
	return flags[:at] + " -" + flags[at:]
}

// exportSpelledDeclaration is DeclareListingExportSpelled — see the constant.
func (r *Runner) exportSpelledDeclaration(d declaration) string {
	word := "typeset"
	// This engine slots the case letters between the integer letter and the
	// readonly one, where the clustered engine puts them after export —
	// measured from the same state in both. `U` comes after export and `T`
	// after that, measured: `typeset -arxU`, `export -iU`, `typeset -lU`,
	// `export -UT`, `typeset -aUT`, `typeset -arT`.
	// `F` sits where `i` does, which the two attributes being exclusive
	// makes unambiguous, and before the letters measured after it: zsh lists
	// `typeset -Fr x=1.500`, `typeset -FU x=1.500` and `export -F x=1.500`.
	// `L`, `R` and `Z` sit where `i` and `F` do — measured, `typeset -aL 3 c`
	// lists as `typeset -aL3 c=(  )`, so the container letters come first —
	// and the letters measured after them follow: `typeset -rL 3 a=abcd` is
	// `typeset -L3 -r a=abcd` and `typeset -lL 4 d=ABCD` is
	// `typeset -L4 -l d=ABCD`. A name carries one of the three, so their
	// order among themselves decides nothing.
	flags := d.letters("naAiEFLRZlurtxUT")
	numbered := 0
	if d.base != 0 {
		// The base rides on the letter here — `typeset -i16 h=255` — where
		// the other listed form writes it as a word of its own. Measured in
		// the one shell with this arrangement.
		flags = strings.Replace(flags, "i", "i"+itoa(d.base), 1)
		numbered = strings.Index(flags, "i") + 1 + len(itoa(d.base))
	}
	if d.hasWidth && d.width.width != 0 {
		// The same arrangement, and the number is always written: a width
		// learned from the first value is written back exactly as one the
		// letter named — `typeset -L f=xy` lists as `typeset -L2 f=xy`.
		l := string(d.width.letter)
		flags = strings.Replace(flags, l, l+itoa(d.width.width), 1)
		numbered = strings.Index(flags, l) + 1 + len(itoa(d.width.width))
	}
	// Where the declaration would *land* is part of this form, because the
	// form's promise is that the text recreates the state it describes.
	// Inside a function a bare `typeset` declares a local, so a global needs
	// the letter that says so and a local needs none. Measured 2026-09-12 on
	// zsh 5.9.2 with `env -i`, `-f`, from inside a one-line function:
	//
	//	global scalar          typeset -g s=plain
	//	global array           typeset -g -a g=( a b )
	//	global readonly        typeset -g -r rr=1
	//	global based integer   typeset -g -i10 n=5
	//	global tie             typeset -g -T TT tt=( a b )
	//	global exported scalar export q=2
	//	global exported array  typeset -g -ax A=( 1 2 )
	//	local scalar           typeset g=2
	//	local array            typeset -a la=( 1 2 )
	//	local exported scalar  local -x e=9
	//	local exported array   local -ax la=( 1 2 )
	//	local exported+frozen  local -rx lxr=1
	//
	// so the letter is a word of its own ahead of the cluster, and it is
	// written only beside `typeset` — the two words that already say where
	// they land carry it in the word instead. `local` is the third command
	// word of this form and keeps `x` where `export` drops it, which is what
	// makes the pair legible: `export` *is* the export letter and `local` is
	// not.
	//
	// At the top level every name is global and the letter would say
	// nothing, which is why this asks whether there is a scope at all rather
	// than only whether the name is in one.
	local := d.inAFunction && d.localHere
	switch {
	case d.exported && local:
		// A local that is also exported: the word says the scope and the
		// cluster keeps every letter, `x` included.
		word = "local"
	case d.exported && !d.isArr && !d.isAssoc:
		// Only a scalar earns the `export` spelling; an exported array keeps
		// the word and the letter.
		//
		// The letter goes from wherever it stands rather than off the end:
		// `U` is written after it, so `export -U x1=v` was coming out
		// `export -xU x1=v` while `x` was the last letter there was.
		word = "export"
		flags = strings.ReplaceAll(flags, "x", "")
	case d.inAFunction && !d.localHere:
		// A global written from inside a function, under the one word that
		// would otherwise declare a local. Without this the listing is not
		// the declaration it read: pasting it into another function creates
		// a local and leaves the global alone (#2041).
		word += " -g"
	}
	head := word
	if flags != "" {
		head += " -" + numberedLetterEndsTheWord(flags, numbered)
	}
	head += " " + d.name
	if d.hasTie {
		// Both names, and then the array's own elements whichever half was
		// asked for — the value a tie has is one value. The separator
		// follows where it is not the default, quoted the way a listed value
		// is: `'#'`, `' '`, `''`, and a bare `-`. The NUL is the one byte
		// that quoting cannot write, and tieSeparatorWord holds why the
		// empty word is its spelling rather than an escape.
		head = strings.TrimSuffix(head, " "+d.name) + " " + d.tied.scalar
		elems, _ := r.arrayElems(d.tied.array)
		quoted := make([]string, len(elems))
		for i, v := range elems {
			quoted[i] = r.declareQuoted(v, ListedValueInAList)
		}
		out := head + " " + d.tied.array + "=( " + strings.Join(quoted, " ") + " )"
		if d.tied.sep != defaultTieSeparator {
			out += " " + r.tieSeparatorWord(d.tied.sep)
		}
		return out
	}
	if d.hidesTheValue {
		// The whole of what `-H` does under this reading: the attributes
		// still speak, the value does not — a scalar's, an array's and a table's alike. Measured
		// `typeset -A C`, `typeset -a A`, `typeset -i n`, `typeset -r r` and
		// `export e` back from names that all held values.
		return head
	}
	switch {
	case d.isAssoc:
		if len(d.assoc) == 0 {
			return head + "=( )"
		}
		pairs := make([]string, 0, len(d.assoc))
		for _, k := range r.assocKeys(d.name, d.assoc) {
			// Keys never reach `$'...'` in this engine even where its values
			// do, which is the trap listing's style rather than the alias
			// one — measured, not assumed.
			key := r.quoteListedValue(ListingQuoteWhenNeededPlain, "`typeset -p`", k, ListedValueAlone)
			pairs = append(pairs, "["+key+"]="+r.listedElement(d.assoc[k], ListedValueAlone))
		}
		return head + "=( " + strings.Join(pairs, " ") + " )"
	case d.isArr:
		// The dialect's own reading: dense, so a gap lists as an empty
		// element — which is also why no subscripts are written.
		elems := r.readArray(d.arr)
		quoted := make([]string, len(elems))
		for i, v := range elems {
			quoted[i] = r.declareQuoted(v, ListedValueInAList)
		}
		return head + "=( " + strings.Join(quoted, " ") + " )"
	case d.hasValue:
		// A based value is decoded back to decimal for this listing, which
		// is the one nuance of it: `typeset -i16 h=255` back from a name
		// holding `16#FF`, and `typeset -i16 b=16` from one that learned its
		// base from an `0x10`. What the *shell* holds is the based text —
		// every read sees it and a child is told it — and only this listing
		// writes the number.
		return head + "=" + r.declareQuoted(r.listedInDecimal(d), ListedValueAlone)
	}
	return head
}

// bareAssignmentValue is that form's value alone, and whether there is one.
//
// Its own function because the same bytes are what a listing with *no command
// word* writes in this dialect — `g=([3]=x)` for a bare `export` and for
// `typeset -a` — and a second spelling of them beside plainAssignmentDeclaration
// is how the two would come to disagree about a gap or a based number.
func (r *Runner) bareAssignmentValue(d declaration, place ListedValuePlace) (string, bool) {
	switch {
	case d.compoundVar:
		// A compound's members are not in the declaration at all — they are
		// names in the store, spelled with a dot — so the body is gathered
		// here rather than carried. See Runner.compoundVariableBody for the
		// `;` rule, which is measured and is not the one a symmetry argument
		// gives.
		return "(" + r.compoundVariableBody(d.name, true) + ")", true
	case d.isAssoc:
		pairs, _ := r.bareAssignmentElements(d)
		return "(" + strings.Join(pairs, " ") + nestTrailingSpace(d.assoc.lastElement()) + ")", true
	case d.isArr:
		if len(d.arr) == 0 {
			// An indexed array with nothing in it, which this form answers
			// from the array's *history* rather than from its emptiness —
			// the one place the three compound states are told apart.
			// Measured 2026-09-18 on ksh93u+ 2012-08-01, a script file under
			// `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on
			// /dev/null:
			//
			//	typeset -a a;                 typeset -p a   typeset -a a
			//	typeset -a b=();              typeset -p b   typeset -a b
			//	typeset -ia n;                typeset -p n   typeset -a -i n
			//	typeset -a d; d[0]=x; unset 'd[0]'           typeset -a d=([0]=)
			//	typeset -a d; d[2]=x; unset 'd[2]'           typeset -a d=([0]=)
			//	e[5]=q; unset 'e[5]'                         typeset -a e=([0]=)
			//	e=(x y z); unset 'e[0]' 'e[1]' 'e[2]'        typeset -a e=([0]=)
			//
			// So it is always the *first* subscript and never the one the
			// element that went was at, which is what says this is about the
			// array being empty rather than about how a removal is recorded.
			// The control is a hole that is not the whole array: `e=(x y z);
			// unset 'e[1]'` is `([0]=x [2]=z)` there, a skipped subscript and
			// not an empty element, and `e=(x); unset 'e[0]'; e[3]=z` is
			// `([3]=z)` — a real element clears it again.
			//
			// A **table** is `=()` however it got there, which is the branch
			// above and is why the two kinds are answered apart:
			// `typeset -A m; m[k]=1; unset 'm[k]'` is `typeset -A m=()`.
			//
			// This wrote `=()` for every one of these (#3407).
			if !r.compoundHeldAnElement[d.name] {
				return "", false
			}
			return fmt.Sprintf("([%d]=)", r.arrayBase()), true
		}
		elems, _ := r.bareAssignmentElements(d)
		return "(" + strings.Join(elems, " ") + nestTrailingSpace(d.arr.lastElement()) + ")", true
	case d.hasValue && d.base != 0:
		// A based number lists bare here, where an ordinary value carrying a
		// `#` is quoted: measured, `typeset -i 16 a=16#ff` against
		// `b='#lead'`, `c='tail#'` and `d='a#b'` from the same shell. It
		// reads the text as the number it is rather than as a word with a
		// comment character in it.
		//
		// Conditioned on the name's base, which is narrower than what that
		// shell does — it leaves a *plain* name holding `16#ff` bare too —
		// and is the part this change measured. The wider rule is #1271.
		return d.value, true
	case d.hasValue:
		return r.declareQuoted(d.value, place), true
	}
	return "", false
}

// bareAssignmentElements is a parenthesized value's members, one string each,
// and whether the name has a parenthesized value at all.
//
// Split out of bareAssignmentValue because a compound variable's multi-line
// rendering writes the same strings a line apart rather than a blank apart,
// and quoting an element two ways is how the two forms would disagree about a
// key holding a blank.
func (r *Runner) bareAssignmentElements(d declaration) ([]string, bool) {
	switch {
	case d.isAssoc:
		pairs := make([]string, 0, len(d.assoc))
		for _, k := range r.assocKeys(d.name, d.assoc) {
			// Keys quote the way values do here, `$'...'` included.
			pairs = append(pairs, "["+r.declareQuoted(k, ListedValueAlone)+"]="+r.listedElement(d.assoc[k], ListedValueAlone))
		}
		return pairs, true
	case d.isArr:
		subs := d.arr.subscripts()
		elems := make([]string, 0, len(subs))
		for _, i := range subs {
			// Subscripts appear only where they carry information: an array
			// that is contiguous from zero lists its values alone.
			//
			// Measured aside: one engine reads an *empty literal* as a
			// compound variable rather than as an empty array, so
			// `a=(x y); a=()` lists there as `typeset -C a=()`. That is a
			// fact about the literal and is answered where the literal is
			// read — see syntax.Dialect.CompoundVariableDeclarators. An
			// array that became empty by another route keeps `-a` here, and
			// so does one the letter declared: `typeset -a c=()` is an
			// empty array in that shell too.
			if r.arrayHasGaps(d.arr) {
				elems = append(elems, fmt.Sprintf("[%d]=%s", i, r.listedElement(d.arr[i], ListedValueAlone)))
			} else {
				elems = append(elems, r.listedElement(d.arr[i], ListedValueInAList))
			}
		}
		return elems, true
	}
	return nil, false
}

// bareAssignmentDeclaration is DeclareListingBareAssignments — see the
// constant.
func (r *Runner) bareAssignmentDeclaration(d declaration, place ListedValuePlace) string {
	flags := bareAssignmentFlags(d)
	value, hasValue := r.bareAssignmentValue(d, place)
	head := bareAssignmentHead(flags, d.name)
	if len(flags) == 0 {
		// No attributes: a bare assignment, with no command word at all. A
		// name with neither attributes nor value never reaches here — it is
		// the missing-name case, which this engine passes over in silence.
		return head + "=" + value
	}
	if hasValue {
		head += "=" + value
	}
	return head
}

// bareAssignmentFlags is the letters a bare-assignment listing writes for a
// name, in this engine's own order.
func bareAssignmentFlags(d declaration) []string {
	var flags []string
	if d.isNameref {
		// A **name reference** lists with the letter and nothing else, and
		// the value on it is the name it points at: measured 2026-09-15 on
		// ksh93u+, `v=1; typeset -n r=v; typeset -p r` is `typeset -n r=v`.
		// Ahead of every letter below because none of them can stand with it
		// — the attribute says what the name *is*, and a reference carries
		// none of the target's. See interp/nameref.go.
		return []string{"-n"}
	}
	// Not the clustered order: this engine leads with what the name *is
	// for* — export first, then readonly — and follows with what it is.
	if d.exported {
		flags = append(flags, "-x")
	}
	if d.readonly {
		flags = append(flags, "-r")
	}
	if d.traced {
		// The trace attribute sits behind export and readonly and in front
		// of everything else this form writes, which is measured a letter at
		// a time on ksh93u+ 2012-08-01: `typeset -x -t`, `typeset -x -r -t
		// -l -u -i` from a name carrying all of them, and `typeset -t -a`,
		// `typeset -t -A`, `typeset -t -H`, `typeset -t -i` and `typeset -t
		// -u` from a name carrying one beside it.
		flags = append(flags, "-t")
	}
	// The kind letter comes next and the value letters after it — measured
	// from a name declared with every one: `typeset -x -r -a -u q=(1)` and
	// `typeset -x -A -i w=([k]=1)`.
	//
	// It read `-x -r -l -i -a` here until an array could carry a value
	// letter at all, which was measured on a *scalar* — where the kind
	// letter is absent and any order for it passes. `typeset -a -i` back as
	// `typeset -i -a` is what made the order visible.
	if d.isArr {
		flags = append(flags, "-a")
	}
	if d.isAssoc {
		flags = append(flags, "-A")
	}
	compoundAt := -1
	if d.compoundVar {
		flags = append(flags, "-C")
		compoundAt = len(flags) - 1
	}
	if d.hidden && !d.hidesTheValue {
		// The inert reading of `-H`, which is the only one that reaches this
		// form: the letter is written and the value stays. Measured a pair at
		// a time on ksh93u+ — `typeset -x -H h=1`, `typeset -r -H h=1`,
		// `typeset -a -H h=(1 2)`, `typeset -A -H m=([k]=v)`, `typeset -H -l
		// h=ab` and `typeset -H -u h=AB` — so it stands after the kind letter
		// and before the case letters, which is exactly here. See
		// interp/declarehide.go; the reading that withholds the value writes
		// no letter at all, and the guard says which one this is rather than
		// leaving the two to be told apart by which dialect happened to call.
		flags = append(flags, "-H")
	}
	if d.lower {
		flags = append(flags, "-l")
	}
	if d.upper {
		flags = append(flags, "-u")
	}
	if d.capital {
		flags = append(flags, "-c")
	}
	if d.float {
		// The float letter and its number as a word of its own —
		// `typeset -E 3 a=3.14` — which is where the integer base sits too
		// and is measured from the same state: `typeset -xE 3 a=1.5` lists
		// as `typeset -x -E 3 a=1.5`, so the letter follows export and
		// readonly and carries its number behind it.
		if d.floatExponent {
			flags = append(flags, "-E")
		} else {
			flags = append(flags, "-F")
		}
		if d.precision != 0 {
			flags = append(flags, itoa(d.precision))
		}
	}
	if d.integer {
		flags = append(flags, "-i")
		if d.base != 0 {
			// A word of its own after the letter — `typeset -i 16 h=16#ff` —
			// where the other listed form attaches it. Measured in the one
			// shell with this arrangement.
			flags = append(flags, itoa(d.base))
		}
	}
	if d.hasWidth {
		// The width letters, last of all and each with its number as a word
		// of its own. Measured 2026-09-18 on ksh93u+ 2012-08-01 a letter at
		// a time: `typeset -x -L 3`, `typeset -r -L 3`, `typeset -l -L 4`,
		// `typeset -t -L 3`, `typeset -a -L 3` and `typeset -x -r -t -L 4`,
		// so they follow everything above and the order among the three
		// decides nothing — a name carries one justification.
		//
		// The number is written even where it is zero, which is not the
		// float letters' rule and is measured rather than carried over:
		// `typeset -L k` with no width and no value lists as `typeset -L 0
		// k` and `typeset -Z m` as `typeset -Z 0 -R 0 m`, where a precision
		// nobody wrote is left off the letter entirely.
		if d.width.zeroFill {
			// The fill and the justification it rides on are two letters
			// here and the listing writes both, the fill first: `typeset -Z
			// 4 d=7` lists as `typeset -Z 4 -R 4 d=0007` and `typeset -ZL 5
			// q=7` as `typeset -Z 5 -L 5 q='7    '`. See
			// Semantics.DeclareZeroFillLetter, which is where the other
			// reading writes one letter for the pair.
			flags = append(flags, "-Z", itoa(d.width.width))
		}
		flags = append(flags, "-"+string(d.width.letter), itoa(d.width.width))
	}
	return dropACompoundLetterBesideAnother(flags, compoundAt)
}

// dropACompoundLetterBesideAnother removes the `C` where the listing has some
// other letter to write, which is the only shape it is written in.
//
// A correction rather than an axis: one dialect has compound variables at all,
// so there is no second answer to choose between. Measured 2026-09-14 on
// ksh93u+ 2012-08-01, `env -i PATH=/usr/bin:/bin` with a scratch HOME:
//
//	c=(a=1); typeset -p c                  typeset -C c=(a=1)
//	typeset -C c=(a=1); typeset -p c       typeset -C c=(a=1)
//	readonly c=(a=1); typeset -p c         typeset -r c=(a=1)
//	typeset -rC c=(a=1); typeset -p c      typeset -r c=(a=1)
//	c=(a=1); typeset -t c; typeset -p c    typeset -t c=(a=1)
//	readonly c=(); typeset -p c            typeset -r c=()
//
// Rows two and four are what say this is about the *letter written back* and
// not about which letters the declaration carried: the same `-C` is spelled
// on the command in both, and only the one with nothing beside it survives.
// The parentheses are written either way, so a reader can still see the kind
// — it is the letter that is not repeated.
//
// at is where the `C` was appended, or below zero where none was. It is taken
// rather than searched for because `-C` is also how a *base* would be spelled
// if one were ever 12, and a scan for the string would find that instead.
func dropACompoundLetterBesideAnother(flags []string, at int) []string {
	if at < 0 || len(flags) == 1 {
		return flags
	}
	return append(flags[:at:at], flags[at+1:]...)
}

// bareAssignmentHead is everything a bare-assignment listing writes in front
// of the `=`: the command word and letters where the name carries attributes,
// and the bare name where it carries none.
//
// Its own function because a compound variable's *multi-line* rendering writes
// the same head with the value laid out across lines instead of inside one
// pair of parentheses — `typeset -A h=(` and then a line per element — and a
// second spelling of the letters beside this one is how the two forms would
// come to disagree about an order that was measured.
func bareAssignmentHead(flags []string, name string) string {
	if len(flags) == 0 {
		return name
	}
	return "typeset " + strings.Join(flags, " ") + " " + name
}

// flagLetters is the clustered spelling of what a name is: kind first, then
// the attributes. The two clustering engines share the order of the letters
// they had before the case attributes arrived, and part ways over where
// those go — measured, `-irxl` against `-ilr` for the same state — which is
// why the order is the caller's to spell.
//
// `n` sits **after the container letters and after `i`**, and before
// everything measured after it. It stood first, on the reasoning that the
// letter says what the name *is* rather than what it holds; the one pair that
// can tell says otherwise. Measured 2026-09-17 on bash 5.3.20: `declare -n y;
// declare -i y` lists as `declare -in y`, and a reference that reaches an
// element of an integer array the same way is `declare -inx`. `a` and `A`
// cannot stand with `n` at all — a `-n` declaration over an array is refused,
// see NamerefArrayRefusal — so their side of it is unexercised and they keep
// the place the rest of the order gives them.
//
// `c` sits **last**, behind every other letter including the two it cannot
// stand with. Measured 2026-09-23 on GNU bash 5.3.15: `declare -irtxc v="9"`,
// `declare -axc v=([0]="A")`, `declare -Ac v`, `declare -nc v`, and `-xc`,
// `-rc`, `-tc`, `-ic` for each letter on its own.
func (d declaration) flagLetters() string { return d.letters("aAinrtxluc") }

// letters spells the attributes present in the given order.
func (d declaration) letters(order string) string {
	var b strings.Builder
	for _, c := range order {
		on := false
		switch c {
		case 'a':
			on = d.isArr
		case 'A':
			on = d.isAssoc
		case 'i':
			on = d.integer
		case 'F':
			on = d.float && !d.floatExponent
		case 'E':
			on = d.float && d.floatExponent
		case 'L', 'R', 'Z':
			on = d.hasWidth && d.width.letter == byte(c)
		case 'r':
			on = d.readonly
		case 'x':
			on = d.exported
		case 'l':
			on = d.lower
		case 'u':
			on = d.upper
		case 'c':
			on = d.capital
		case 'U':
			on = d.unique
		case 't':
			on = d.traced
		case 'T':
			on = d.hasTie
		case 'n':
			on = d.isNameref
		}
		if on {
			b.WriteRune(c)
		}
	}
	return b.String()
}
