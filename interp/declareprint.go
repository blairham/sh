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
	integer  bool
	readonly bool
	exported bool
	lower    bool
	upper    bool
	// hidden keeps the value out of the listing — `typeset -H`. Not a flag
	// letter in any listed form: the shell that has the attribute says it by
	// writing no value, and never writes an `H` back.
	//
	// Only the forms that shell reaches honor it — ExportSpelled, its
	// `typeset -p` and `readonly -p`; CommandWord, its `export -p`; and
	// PlainAssignment, its bare `export` and `readonly`. Clustered and
	// BareAssignments are left alone on purpose rather than for want of
	// effort: bash has no `-H` at all, and the letter ksh93 does have is a
	// different attribute whose listing keeps the value — `typeset -H h=v`
	// lists back as `typeset -H h=v` there. Hiding in those forms would be
	// output no shell in the panel produces.
	hidden bool
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
	// base is the output base an integer name renders in — `typeset -i16`.
	// Zero where none was named, which is every name in a dialect without
	// the feature. The two forms that write it write it differently and one
	// of them decodes the value: see exportSpelledDeclaration and
	// bareAssignmentDeclaration.
	base int
	// float is `typeset -F`, the float attribute, whose letter this writes
	// and whose *precision* it does not: measured 2026-09-07, zsh lists
	// `typeset -F 3 x=3.14159` back as `typeset -F x=3.142` — the places are
	// in the value and the number is not written again. ksh93 writes it as a
	// word of its own, `typeset -F 3 x=3.142`, and does not have the
	// attribute here — see #1461.
	float bool
}

// declarationOf gathers what the runner knows about a name. The second result
// reports whether it knows anything at all — an attribute with no value is
// still a declaration, and `declare -p` is how scripts ask.
func (r *Runner) declarationOf(name string) (declaration, bool) {
	d := declaration{
		name:     name,
		integer:  r.integer[name],
		readonly: r.readonly[name],
		exported: r.isExported(name),
		lower:    r.lowered[name],
		upper:    r.uppered[name],
		hidden:   r.hidden[name],
		unique:   r.unique[name],
		base:     r.integerBase[name],
	}
	_, d.float = r.floatPrecision[name]
	d.tied, d.hasTie = r.tieOf(name)
	attributed := d.integer || d.float || d.readonly || d.exported || d.lower ||
		d.upper || d.hidden || d.unique
	if r.removed[name] {
		// `unset` took the value away; only a surviving attribute keeps the
		// name listable.
		return d, attributed
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
	if v, ok := r.Vars[name]; ok {
		d.value, d.hasValue = v, true
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
		seen[name] = true
	}
	for name := range r.integer {
		seen[name] = true
	}
	for name := range r.floatPrecision {
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
	for k := range r.inheritedEnv {
		// isNameLike keeps the entries that are variables: an exported
		// function travels in the environment under a decorated name no
		// shell lists as one.
		if isNameLike(k) {
			seen[k] = true
		}
	}
	for name := range seen {
		_, isFloat := r.floatPrecision[name]
		if r.removed[name] && !r.readonly[name] && !r.integer[name] && !isFloat &&
			!r.exported[name] && !r.lowered[name] && !r.uppered[name] &&
			!r.hidden[name] && !r.unique[name] {
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
		r.diagf("%s\n", r.unanswered("how a declaration is listed back"))
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
	case DeclareListingPlainAssignment:
		return r.plainAssignmentDeclaration(d)
	}
	return r.clusteredDeclaration(d)
}

// commandWordDeclaration is DeclareListingCommandWord — see the constant.
func (r *Runner) commandWordDeclaration(d declaration) string {
	head := r.inBuiltin + " " + d.name
	if d.hasValue && !d.hidden {
		return head + "=" + r.declareQuoted(d.value)
	}
	return head
}

// plainAssignmentDeclaration is DeclareListingPlainAssignment — see the
// constant. A name carrying an attribute and no value still lists, and lists
// as a bare name, because there is no word left to say the attribute with.
func (r *Runner) plainAssignmentDeclaration(d declaration) string {
	if !d.hasValue || d.hidden {
		return d.name
	}
	return d.name + "=" + r.declareQuoted(d.value)
}

// listedDeclarationValue spells a declaration's value alone — no name, no
// command word — in the shape this dialect's `declare -p` gives a compound
// one. For a listing whose attributes are words rather than flags.
func (r *Runner) listedDeclarationValue(d declaration) string {
	switch {
	case d.isAssoc:
		if len(d.assoc) == 0 {
			return "( )"
		}
		return "( " + strings.Join(r.quotedTablePairs(d, r.clusteredKey), " ") + " )"
	case d.isArr:
		return "( " + strings.Join(r.quotedArrayElems(d), " ") + " )"
	}
	return r.declareQuoted(d.value)
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
			b.WriteString("[" + r.clusteredKey(k) + "]=" + r.declareQuoted(d.assoc[k]) + " ")
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
	at := strings.IndexByte(text, '#')
	if at < 0 {
		return d.value
	}
	v, err := strconv.ParseInt(text[at+1:], d.base, 64)
	if err != nil {
		return d.value
	}
	return sign + itoa(int(v))
}

// clusteredKey spells a subscript in the clustered form: bare when it is
// plain, `$'...'` when it holds a control character, double-quoted otherwise —
// the same reach the form's values make, without the always.
func (r *Runner) clusteredKey(k string) string {
	switch {
	case hasControl(k):
		return r.dollarQuoted(k)
	case listedValueIsBare(k):
		return k
	}
	return doubleQuoted(k)
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
	flags := d.letters("aAiFlurxUT")
	if d.base != 0 {
		// The base rides on the letter here — `typeset -i16 h=255` — where
		// the other listed form writes it as a word of its own. Measured in
		// the one shell with this arrangement.
		flags = strings.Replace(flags, "i", "i"+itoa(d.base), 1)
	}
	if d.exported && !d.isArr && !d.isAssoc {
		// Only a scalar earns the `export` spelling; an exported array keeps
		// the word and the letter.
		//
		// The letter goes from wherever it stands rather than off the end:
		// `U` is written after it, so `export -U x1=v` was coming out
		// `export -xU x1=v` while `x` was the last letter there was.
		word = "export"
		flags = strings.ReplaceAll(flags, "x", "")
	}
	head := word
	if flags != "" {
		head += " -" + flags
	}
	head += " " + d.name
	if d.hasTie {
		// Both names, and then the array's own elements whichever half was
		// asked for — the value a tie has is one value. The separator
		// follows where it is not the default, quoted the way a listed value
		// is: `'#'`, `' '`, `''`, and a bare `-`.
		head = strings.TrimSuffix(head, " "+d.name) + " " + d.tied.scalar
		elems, _ := r.arrayElems(d.tied.array)
		quoted := make([]string, len(elems))
		for i, v := range elems {
			quoted[i] = r.declareQuoted(v)
		}
		out := head + " " + d.tied.array + "=( " + strings.Join(quoted, " ") + " )"
		if d.tied.sep != defaultTieSeparator {
			out += " " + r.declareQuoted(d.tied.sep)
		}
		return out
	}
	if d.hidden {
		// The whole of what `-H` does: the attributes still speak, the value
		// does not — a scalar's, an array's and a table's alike. Measured
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
		// A based value is decoded back to decimal for this listing, which
		// is the one nuance of it: `typeset -i16 h=255` back from a name
		// holding `16#FF`, and `typeset -i16 b=16` from one that learned its
		// base from an `0x10`. What the *shell* holds is the based text —
		// every read sees it and a child is told it — and only this listing
		// writes the number.
		return head + "=" + r.declareQuoted(r.listedInDecimal(d))
	}
	return head
}

// bareAssignmentDeclaration is DeclareListingBareAssignments — see the
// constant.
func (r *Runner) bareAssignmentDeclaration(d declaration) string {
	var flags []string
	// Not the clustered order: this engine leads with what the name *is
	// for* — export first, then readonly — and follows with what it is.
	if d.exported {
		flags = append(flags, "-x")
	}
	if d.readonly {
		flags = append(flags, "-r")
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
	if d.lower {
		flags = append(flags, "-l")
	}
	if d.upper {
		flags = append(flags, "-u")
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
		value = d.value
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
// the attributes. The two clustering engines share the order of the letters
// they had before the case attributes arrived, and part ways over where
// those go — measured, `-irxl` against `-ilr` for the same state — which is
// why the order is the caller's to spell.
func (d declaration) flagLetters() string { return d.letters("aAirxlu") }

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
			on = d.float
		case 'r':
			on = d.readonly
		case 'x':
			on = d.exported
		case 'l':
			on = d.lower
		case 'u':
			on = d.upper
		case 'U':
			on = d.unique
		case 'T':
			on = d.hasTie
		}
		if on {
			b.WriteRune(c)
		}
	}
	return b.String()
}
