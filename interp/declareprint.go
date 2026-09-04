// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
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
	integer  bool
	readonly bool
	exported bool
}

// declarationOf gathers what the runner knows about a name. The second result
// reports whether it knows anything at all — an attribute with no value is
// still a declaration, and `declare -p` is how scripts ask.
func (r *Runner) declarationOf(name string) (declaration, bool) {
	d := declaration{
		name:     name,
		integer:  r.integer[name],
		readonly: r.readonly[name],
		exported: r.exported[name],
	}
	attributed := d.integer || d.readonly || d.exported
	if r.removed[name] {
		// `unset` took the value away; only a surviving attribute keeps the
		// name listable.
		return d, attributed
	}
	// The array tables answer ahead of Vars, which mirrors an array's first
	// element — the same order every read follows.
	if a, ok := r.AssocArrays[name]; ok {
		d.assoc, d.isAssoc = a, true
		return d, true
	}
	if a, ok := r.Arrays[name]; ok {
		d.arr, d.isArr = a, true
		return d, true
	}
	if v, ok := r.Vars[name]; ok {
		d.value, d.hasValue = v, true
		return d, true
	}
	for _, kv := range r.environ() {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			// Born in the environment and never assigned to: the value is
			// real and the name is exported, which is what carrying it in
			// the environment means.
			d.value, d.hasValue, d.exported = v, true, true
			return d, true
		}
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
		seen[name] = true
	}
	for name := range r.integer {
		seen[name] = true
	}
	for _, kv := range r.environ() {
		// isNameLike keeps the entries that are variables: an exported
		// function travels in the environment under a decorated name no
		// shell lists as one.
		if k, _, ok := strings.Cut(kv, "="); ok && isNameLike(k) {
			seen[k] = true
		}
	}
	for name := range seen {
		if r.removed[name] && !r.readonly[name] && !r.integer[name] && !r.exported[name] {
			delete(seen, name)
		}
	}
	return sortedNames(seen)
}

// declarePrint is the `-p` of `declare` and `typeset`: the named
// declarations, or every one the runner knows when no name is given.
func (r *Runner) declarePrint(names []string) int {
	return r.declarePrintForm(names, r.sem().DeclareListing, nil)
}

// declarePrintForm lists declarations in the given form, walking only the
// names the filter admits when no operands narrow it — which is how
// `export -p` lists the exported names alone.
func (r *Runner) declarePrintForm(names []string, form DeclarationListingForm, keep func(declaration) bool) int {
	if form == DeclarationListingUnspecified {
		r.diagf("how a declaration is listed back: the shells disagree here and no dialect was chosen\n")
		r.status = 2
		r.unspecified = true
		return 2
	}
	status := 0
	filtered := false
	if len(names) == 0 {
		names = r.declarableNames()
		filtered = keep != nil
	}
	for _, name := range names {
		d, known := r.declarationOf(name)
		if known && filtered && !keep(d) {
			continue
		}
		if !known {
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
		r.printf("%s\n", r.listedDeclaration(form, d))
	}
	return status
}

func (r *Runner) listedDeclaration(form DeclarationListingForm, d declaration) string {
	switch form {
	case DeclareListingExportSpelled:
		return r.exportSpelledDeclaration(d)
	case DeclareListingBareAssignments:
		return r.bareAssignmentDeclaration(d)
	case DeclareListingCommandWord:
		return r.commandWordDeclaration(d)
	}
	return r.clusteredDeclaration(d)
}

// commandWordDeclaration is DeclareListingCommandWord — see the constant.
func (r *Runner) commandWordDeclaration(d declaration) string {
	head := r.inBuiltin + " " + d.name
	if d.hasValue {
		return head + "=" + r.declareQuoted(d.value)
	}
	return head
}

// declareQuoted spells one listed value in the dialect's declaration style,
// which its other listings do not decide.
func (r *Runner) declareQuoted(v string) string {
	return r.quoteListedValue(r.sem().DeclareValueQuoting, "`declare -p`", v)
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
	case d.isAssoc:
		if len(d.assoc) == 0 {
			// The attribute is the whole of what an empty table has to say,
			// and this engine says it with no value at all.
			return head
		}
		var b strings.Builder
		b.WriteString(head)
		b.WriteString("=(")
		for _, k := range d.assoc.keys() {
			// Sorted keys are this implementation's choice: the shells
			// promise no order at all, and a deterministic listing is worth
			// having. See AssocArray.keys.
			b.WriteString("[" + clusteredKey(k) + "]=" + r.declareQuoted(d.assoc[k]) + " ")
		}
		b.WriteString(")")
		return b.String()
	case d.isArr:
		elems := make([]string, 0, len(d.arr))
		for _, i := range d.arr.subscripts() {
			elems = append(elems, fmt.Sprintf("[%d]=%s", i, r.declareQuoted(d.arr[i])))
		}
		return head + "=(" + strings.Join(elems, " ") + ")"
	case d.hasValue:
		return head + "=" + r.declareQuoted(d.value)
	}
	return head
}

// clusteredKey spells a subscript in the clustered form: bare when it is
// plain, `$'...'` when it holds a control character, double-quoted otherwise —
// the same reach the form's values make, without the always.
func clusteredKey(k string) string {
	switch {
	case hasControl(k):
		return dollarQuoted(k)
	case listedValueIsBare(k):
		return k
	}
	return doubleQuoted(k)
}

// exportSpelledDeclaration is DeclareListingExportSpelled — see the constant.
func (r *Runner) exportSpelledDeclaration(d declaration) string {
	word := "typeset"
	flags := d.flagLetters()
	if d.exported && !d.isArr && !d.isAssoc {
		// Only a scalar earns the `export` spelling; an exported array keeps
		// the word and the letter.
		word = "export"
		flags = strings.TrimSuffix(flags, "x")
	}
	head := word
	if flags != "" {
		head += " -" + flags
	}
	head += " " + d.name
	switch {
	case d.isAssoc:
		if len(d.assoc) == 0 {
			return head + "=( )"
		}
		pairs := make([]string, 0, len(d.assoc))
		for _, k := range d.assoc.keys() {
			// Keys never reach `$'...'` in this engine even where its values
			// do, which is the trap listing's style rather than the alias
			// one — measured, not assumed.
			key := r.quoteListedValue(ListingQuoteWhenNeededPlain, "`typeset -p`", k)
			pairs = append(pairs, "["+key+"]="+r.declareQuoted(d.assoc[k]))
		}
		return head + "=( " + strings.Join(pairs, " ") + " )"
	case d.isArr:
		// The dialect's own reading: dense, so a gap lists as an empty
		// element — which is also why no subscripts are written.
		elems := r.readArray(d.arr)
		quoted := make([]string, len(elems))
		for i, v := range elems {
			quoted[i] = r.declareQuoted(v)
		}
		return head + "=( " + strings.Join(quoted, " ") + " )"
	case d.hasValue:
		return head + "=" + r.declareQuoted(d.value)
	}
	return head
}

// bareAssignmentDeclaration is DeclareListingBareAssignments — see the
// constant.
func (r *Runner) bareAssignmentDeclaration(d declaration) string {
	var flags []string
	// Not the clustered order: this engine leads with what the name *is
	// for* — export first — and puts the kind last.
	if d.exported {
		flags = append(flags, "-x")
	}
	if d.readonly {
		flags = append(flags, "-r")
	}
	if d.integer {
		flags = append(flags, "-i")
	}
	if d.isArr {
		flags = append(flags, "-a")
	}
	if d.isAssoc {
		flags = append(flags, "-A")
	}
	value := ""
	hasValue := true
	switch {
	case d.isAssoc:
		pairs := make([]string, 0, len(d.assoc))
		for _, k := range d.assoc.keys() {
			// Keys quote the way values do here, `$'...'` included.
			pairs = append(pairs, "["+r.declareQuoted(k)+"]="+r.declareQuoted(d.assoc[k]))
		}
		value = "(" + strings.Join(pairs, " ") + ")"
	case d.isArr:
		subs := d.arr.subscripts()
		elems := make([]string, 0, len(subs))
		for _, i := range subs {
			// Subscripts appear only where they carry information: an array
			// that is contiguous from zero lists its values alone.
			//
			// Measured aside: the real engine lists an *empty* indexed array
			// as `typeset -C arr=()`, retyping it as a compound variable.
			// That is a fact about its type system, not about `-p`, and it
			// is deliberately not followed — an empty array keeps `-a` here.
			if r.arrayHasGaps(d.arr) {
				elems = append(elems, fmt.Sprintf("[%d]=%s", i, r.declareQuoted(d.arr[i])))
			} else {
				elems = append(elems, r.declareQuoted(d.arr[i]))
			}
		}
		value = "(" + strings.Join(elems, " ") + ")"
	case d.hasValue:
		value = r.declareQuoted(d.value)
	default:
		hasValue = false
	}
	if len(flags) == 0 {
		// No attributes: a bare assignment, with no command word at all. A
		// name with neither attributes nor value never reaches here — it is
		// the missing-name case, which this engine passes over in silence.
		return d.name + "=" + value
	}
	head := "typeset " + strings.Join(flags, " ") + " " + d.name
	if hasValue {
		head += "=" + value
	}
	return head
}

// flagLetters is the clustered spelling of what a name is: kind first, then
// the attributes, in the one order both clustering engines share.
func (d declaration) flagLetters() string {
	var b strings.Builder
	if d.isArr {
		b.WriteByte('a')
	}
	if d.isAssoc {
		b.WriteByte('A')
	}
	if d.integer {
		b.WriteByte('i')
	}
	if d.readonly {
		b.WriteByte('r')
	}
	if d.exported {
		b.WriteByte('x')
	}
	return b.String()
}
