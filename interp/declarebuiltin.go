// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strconv"
	"strings"

	"github.com/blairham/sh/syntax"
)

// `declare` and `typeset`, which give a name an attribute as well as a value.
//
// Which of the two a shell has is a dialect's answer and not an axis: dash
// has neither, ksh93 has `typeset` alone, and bash and zsh have both under one
// implementation. So they are registered through the extension seam, the same
// way ksh93 registers `source` and takes `local` away.
//
// The attributes are the interesting half. `-r` and `-x` are `readonly` and
// `export` spelled differently and share their machinery, but `-i` is a
// property of the *name* that changes what a later assignment means:
//
//	declare -i n; n=5+2   → 7, because the value is evaluated
//	n=5+2                 → the four characters, because it is not
//
// That is why an attribute has to be recorded against the name rather than
// applied once at the point of declaring it.

// declareFlags is what a declaration asks for.
type declareFlags struct {
	integer bool
	// base is the output base `-i16` or `-i 16` named, and baseNamed says
	// one was written at all — 0 is a base a shell will take and render
	// plain, so the two cannot be one field.
	base      int
	baseNamed bool
	// float is `-F` where that letter is a float's precision rather than
	// bash's function listing, and precision is the number written after it
	// — `typeset -F 3 x` — with precisionNamed saying one was written at
	// all. Three digits and no digits are different declarations and zero is
	// a number a script may write, so the two cannot be one field, the same
	// way base and baseNamed cannot.
	float bool
	// floatExponent is which of the two float letters was written: `E`
	// rather than `F`. A field of its own because the letter chooses a
	// *format* and not only a number — see interp/floatformat.go — and the
	// two letters otherwise declare the same attribute.
	floatExponent  bool
	precision      int
	precisionNamed bool
	// widthLetter is `L`, `R` or `Z` where the dialect gives those letters a
	// width, and width/widthNamed are its number under the same split
	// precision and precisionNamed are under. The letter is kept rather than
	// three bools because a name carries one of the three and the listing
	// has to write back which — see fieldwidth.go.
	//
	// widthZeroFill is the `Z` letter where it **rides on** one of the other
	// two rather than being a third of them, which is the reading
	// Semantics.DeclareZeroFillLetter records for ksh93: there a name
	// carries a justification and a fill at once and the listing writes both
	// — `typeset -Z 4 -R 4 d=0007`.
	widthLetter   byte
	widthZeroFill bool
	width         int
	widthNamed    bool
	readonly      bool
	export        bool
	assoc         bool
	array         bool
	// compoundVar is `-C`, ksh93's compound-variable letter: a fourth kind
	// beside the scalar and the two arrays. Its own field rather than a
	// third value of a container enum because it is not a container — the
	// name holds no elements, it holds *names*, and what the letter declares
	// is how the parent reads, lists and enumerates. See
	// interp/compoundvariable.go.
	compoundVar bool
	lower       bool
	upper       bool
	global      bool
	hidden      bool
	// hide is the sign of the last `h` letter written and hideNamed says one
	// was written at all — the hide-in-scope attribute, which is a tri-state
	// and not a bool: `-h` sets it, `+h` takes it off, and a declaration with
	// neither inherits whatever the name it shadows already carried. Absent
	// and off are different declarations, so the two cannot be one field.
	// See hideinscope.go.
	hide      bool
	hideNamed bool
	// hideString and hideStringNamed are the *other* reading of the same
	// letter: a string argument this shell records nowhere, which may ride
	// on the letter or arrive as the next word. hideString says the letter
	// was written under that reading and hideStringNamed says its argument
	// has already been found. See Semantics.DeclareHideInScopeLetter.
	hideString      bool
	hideStringNamed bool
	unique          bool
	// traced is the `t` letter: an attribute the name carries, that every
	// listing writes back, and that nothing else in this engine reads. See
	// Runner.traced.
	traced bool
	// inherit is the `I` letter: this declaration's fresh binding takes the
	// value and the attributes of the name at the enclosing scope rather
	// than starting empty. The per-declaration spelling of what one shell's
	// `localvar_inherit` asks for wholesale — see
	// Runner.LocalInheritsTheOuterValue, which is the same request and is
	// read beside this.
	//
	// One bool and no sign, which is measured rather than an omission: that
	// shell reads `+I` as the same request, so `local +I v` inherits in a
	// shell with the option off exactly as `-I` does. There is no spelling
	// that turns inheritance *off* for one declaration.
	inherit bool
	// nameref is the `n` letter: the name being declared is a **reference**
	// to another parameter rather than a parameter of its own, and the value
	// on the operand is the name it points at. Its own field rather than a
	// value of a container enum for the reason compoundVar has one — what
	// the letter declares is not a kind of store but where every later read
	// and write lands. See interp/nameref.go.
	nameref bool
	// namerefOff records the sign of the *last* `n` letter the command
	// carried, which `nameref` alone cannot say: under a plus the letter
	// takes the reference **off**, which is a declaration of its own and not
	// the absence of one. Same split readonlyOff already draws for `r`.
	//
	// What it does is measured and unanimous — see namerefAttributeRemoved.
	namerefOff bool
	tie        bool
	// typeName is the name that rode on the `T` letter in the dialect that
	// reads it as a *type* — `typeset -TPt v` — and typeNamed says one was
	// written at all. The two cannot be one field for the reason
	// mappingName and mappingNamed cannot: an attached name and a name taken
	// from the first operand are indistinguishable afterwards, and only one
	// of them shortens the operand list. See interp/declaretype.go.
	typeName  string
	typeNamed bool
	function  bool
	// functionOff is the sign of the `f` letter, which decides what the
	// function form *writes* rather than whether it is the function form at
	// all: `typeset -f +m 'p*'` writes bodies and `typeset +f -m 'p*'`
	// writes names, so the sign of the word the `m` rode in on decides
	// nothing here. See declareMatching.
	functionOff bool
	// funcNamesOff is the same for the `F` letter, and it exists for the
	// same reason: under a plus that letter is not a listing at all in the
	// one dialect that spells it, it is the function attribute being taken
	// off — which leaves the bare word. See the plus branch in declareNames.
	funcNamesOff bool
	funcNames    bool
	remove       bool
	print        bool
	// matching is the `m` letter — the operands are patterns rather than
	// names — and matchNames is its sign, which chooses between the two
	// listings it has: `-m` writes each match's value and `+m` writes each
	// match's attributes and name. Recorded with its sign for the reason
	// `hide` is: the two signs are different commands rather than one
	// command and its undo. See declarematching.go.
	matching   bool
	matchNames bool
	// mapping is the `M` letter in the dialect that reads it as a *character
	// mapping* — see interp/declaremapping.go and
	// Semantics.DeclareMappingLetter. mappingName is the name, and
	// mappingNamed says it was written attached to the letter rather than
	// left to the first operand: the two are indistinguishable afterwards
	// and the operand list has to be shortened for one and not the other.
	mapping      bool
	mappingName  string
	mappingNamed bool
	// endedOptions records a `--` word. The mapping letter is the one reading
	// that has to know: `typeset -M -- mf` takes no name from the operands
	// there, so the name is missing and the line is refused, where the same
	// line without the dashes reads `mf` as the mapping.
	endedOptions bool
	// added records that some letter was written in a *minus* word, which is
	// what tells a declaration apart from a listing under `-m`. zsh's rule is
	// per letter and not per word — `typeset +mx 'p*'` lists the exported
	// names matching and `typeset -m +x 'p*'` takes the attribute off them —
	// so the sign of the last option word cannot answer it and `remove`
	// is not the field to ask.
	added bool
	// integerForced records that the *name* the command was called by is
	// what asked for the integer attribute, so a plus form on the same line
	// cannot take it off again. It is `integer` in ksh93, where the word
	// carries the type itself — see Semantics.IntegerNameForcesTheAttribute,
	// which is where the other reading lives.
	integerForced bool
	// exportForced records that the *name* the command was called by is what
	// asked for the export attribute, so a plus word on the same line cannot
	// take it off again. It is `export` in the dialect whose `export` reads
	// the declaration letters — the word carries the attribute itself, the
	// way `integer` carries the type — and the letter that spells it is one
	// that dialect refuses outright, so nothing else could have written it.
	// See Semantics.ExportOptions and integerForced just above, which is the
	// same reading for the same reason.
	exportForced bool
	// readonlyOff records the sign of the *last* `r` letter the command
	// wrote, rather than the sign of its last option word, because those are
	// not the same question and the letter is the one that decides.
	// `typeset -r +x n` freezes n in all three shells that spell both
	// letters — measured 2026-09-07 — and reading the word's sign made it a
	// request to *unfreeze*, which is a refusal where the shells say nothing.
	//
	// Mixed signs on the letter itself are a third question and the panel
	// gives it three answers: `typeset -r +r n` leaves n writable in bash
	// and zsh and freezes it in ksh93, and `typeset +r -r n` freezes it in
	// zsh alone. Last occurrence wins here, which is zsh's rule exactly and
	// bash's in the shape a script would write; the disagreement is recorded
	// rather than modeled, as no script writes both signs of one letter on
	// one line.
	readonlyOff bool
	// integerOff records that an `i` was written in a *plus* word, as
	// against a plus word that carried some other letter. Only the explicit
	// spelling may cancel a forced attribute, so the two have to be told
	// apart: `integer +x n` is still an integer in both shells that have the
	// word, and `integer +i n` is one in only one of them.
	integerOff bool
	// signDecided records that the *builtin's name* has already settled
	// whether this line removes the integer attribute, so the letters must
	// not be read again. `integer +i n` reaches it: the plus form removes
	// nothing under that name in one dialect, which is a fact about the word
	// and not about the sign the letter carries — see
	// Semantics.IntegerPlusFormTakesAttributesOff.
	signDecided bool
	// inert records that a letter out of Semantics.DeclareOptionsWithoutEffect
	// was read. Nothing consults it as an attribute; it exists so that a
	// declaration carrying only such a letter is not mistaken for the bare
	// word, whose listing is a different command entirely — `typeset -F`
	// answering with the whole variable table is the one thing worse than
	// refusing the letter (#1037).
	inert bool
	// letters is every option letter the line carried, in the order it was
	// written and with the `f` that names the function table left out.
	//
	// Only one reader, and it is a seam rather than an attribute: a `-f`
	// declaration whose letters mark a name to be defined later hands them
	// straight to the dialect, because which of them mean anything there is
	// that shell's vocabulary and not this package's. See
	// Runner.SetFunctionMarkedUndefined and
	// Semantics.FunctionLettersThatMarkUndefined.
	//
	// Cleared by withoutMatching, with the rest of "what else did this line
	// say": every caller of that comparison is asking whether anything but
	// the sign was written, and a record of the letters is not a second
	// answer to it.
	letters string
	// letterSigns is the sign each letter of `letters` was written with,
	// one byte per letter and in the same order: `-` or `+`.
	//
	// A parallel string rather than a map because declareFlags is compared
	// with `==` — see withoutMatching — and a map field would make it
	// uncomparable. Appended in lockstep with `letters` in the one place
	// that grows either, so the *n*th rune of one is the *n*th byte of the
	// other however wide the rune is.
	//
	// It exists because a letter's own last sign is a question the sign of
	// the *word* cannot answer, and two readers need it: which marking
	// letters a `-f` line refuses under a plus (see
	// Diagnostics.MarkingLettersUnderPlus) and whether a narrowed function
	// listing names its functions rather than writing them out. Measured
	// 2026-09-12 on zsh 5.9.2, `typeset -fu +u nm` is `invalid option(s)`
	// where `typeset +fu -u nm` marks — the same two words the other way
	// round — and `typeset -f +U` names the U-marked functions where
	// `typeset -fU` writes their bodies.
	letterSigns string
}

// lastSign reports the sign the letter was last written with and whether it
// was written at all. Last occurrence wins, which is the rule `readonlyOff`
// already records for the `r` letter and is measured here too.
func (f declareFlags) lastSign(c rune) (plus, written bool) {
	i := 0
	for _, got := range f.letters {
		if got == c && i < len(f.letterSigns) {
			plus, written = f.letterSigns[i] == '+', true
		}
		i++
	}
	return plus, written
}

// letterMissingOnAFunctionLine reports the first letter of `refused` this
// line wrote under a **minus**, which is the sign that asks for the facility
// rather than taking it away. Zero when none was.
//
// The mirror of markingLetterUnderPlus, and beside it rather than folded into
// it: the two ask about opposite signs and the answer to one is not the
// negation of the other, a letter that was never written at all being neither.
// See Diagnostics.UnimplementedOptionLettersOnAFunctionLine.
func (f declareFlags) letterMissingOnAFunctionLine(refused string) (rune, bool) {
	for _, c := range refused {
		if plus, written := f.lastSign(c); written && !plus {
			return c, true
		}
	}
	return 0, false
}

// markingLetterUnderPlus reports the first letter of `refused` whose own last
// sign was a plus. Empty when none was, which is every ordinary line.
func (f declareFlags) markingLetterUnderPlus(refused string) (rune, bool) {
	for _, c := range refused {
		if plus, written := f.lastSign(c); written && plus {
			return c, true
		}
	}
	return 0, false
}

// markingLettersWritten is the marking letters this line carried, in the
// order they were written and each at most once, together with whether any of
// them was written under a plus.
//
// The set is what narrows a function listing with no operands, and the sign
// is what decides between the two shapes it has. Measured 2026-09-12 on zsh
// 5.9.2 with `f1` autoloaded plainly and `f2` with `-U`:
//
//	functions -u      both bodies      functions -U      f2's body
//	functions +U      the name f2      functions -uU     both bodies
//	functions -u +U   both names       typeset -f +U     the name f2
//
// So the letters are a union and any one of them under a plus makes the
// listing a list of names — neither of which the sign of the last option
// word answers, since `functions +U -u` names them too.
func (f declareFlags) markingLettersWritten(marking string) (letters string, plus bool) {
	for _, c := range marking {
		gotPlus, written := f.lastSign(c)
		if !written {
			continue
		}
		letters += string(c)
		plus = plus || gotPlus
	}
	return letters, plus
}

// declareOptionLetters is the set `declare` and `typeset` read where the
// dialect has not answered — the letters the substrate implemented before
// they were a question.
const declareOptionLetters = "aAiprx"

// parseDeclareFlags reads the leading option words of a declaration builtin,
// against the letters the dialect gives it. The parse is the one these
// builtins have always had rather than the shared reader's, because `+i`
// removes what `-i` adds and no other builtin spells an option with a plus.
//
// A letter outside the set goes through the shared refusal, so a letter the
// dialect has and this shell does not is named as missing rather than as
// unknown, in the dialect's words.
func (r *Runner) parseDeclareFlags(name string, args []string, known string) (rest []string, f declareFlags, code int) {
	i := 0
	// pending is the letter an option word left waiting for a number,
	// because the number may arrive as the next word — `typeset -i 16 n=255`
	// and `typeset -F 3 x=1.5` — where it is indistinguishable from a name
	// until this says otherwise. Zero is no letter waiting, which is what a
	// number-taking letter having been satisfied means as much as one never
	// having been written: `typeset -F 3 4 x` is `not an identifier: 4` in
	// both shells with the attribute, so one number ends the wait.
	var pending byte
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			i++
			f.endedOptions = true
			break
		}
		if a == "-" || a == "+" {
			// A sign on its own is an option word carrying no letters, and
			// it is a *listing* rather than a declaration: measured
			// 2026-09-10, `typeset -` writes the whole parameter table with
			// values and `typeset +` writes the same table's attribute words
			// and names, which is exactly what the sign means everywhere
			// else on this builtin. `functions +` is the same word reaching
			// the function table (#1576).
			//
			// Read here rather than in the letter loop because the loop
			// walks `a[1:]`, which is empty: a word with no letters would
			// leave the sign unread and fall through to the operands, where
			// `+` becomes a name nobody may declare. That is bash's answer
			// and it is bash's for a reason — see
			// Semantics.SignAloneIsAnOptionWord, which the three shells with
			// the builtin do not agree on.
			if !r.ask(r.sem().SignAloneIsAnOptionWord, "a bare sign as an option word") {
				if r.unspecified {
					return nil, f, r.status
				}
				break
			}
			pending = 0
			f.remove = a == "+"
			if f.function {
				// And it reaches the function listing where the `f` letter
				// has already been read, which is the same sign meaning the
				// same thing one word later: measured 2026-09-18,
				// `f(){ :; }; typeset -f +` writes `f()` on ksh93u+ and `f`
				// on zsh 5.9.2 — the name alone in both — where
				// `typeset -f -` writes the body. This shell read the sign
				// off the letter alone, so `typeset +f` named the function
				// and `typeset -f +` wrote it out (#3371).
				//
				// The last sign written wins, which is what the two spellings
				// being one question means.
				f.functionOff = f.remove
			}
			continue
		}
		if len(a) < 2 || (a[0] != '-' && a[0] != '+') {
			if pending != 0 && isAllDigits(a) {
				// The detached spelling: the word after the letter is its
				// number and not the first name. Consumed here, which is
				// also what keeps it away from the operand name check — a
				// `3` that is a precision was never an operand, and reading
				// it as one is what made a plugin loader's `typeset -F 3
				// SECONDS=0` complain once per plugin (#1453).
				if !r.readOptionNumber(name, &f, pending, a) {
					return nil, f, r.status
				}
				pending = 0
				continue
			}
			if r.unspecified {
				return nil, f, r.status
			}
			break
		}
		attached := false
		if letter, digits, ok := r.attachedOptionNumber(a, known); ok {
			// `-i16` and `-F3`, where the number rides on the letter. Read
			// here rather than in the letter loop, which would otherwise
			// reach the `1` and call it an unknown option — a true statement
			// about a letter the script never wrote.
			if !r.readOptionNumber(name, &f, letter, digits) {
				return nil, f, r.status
			}
			a, attached = a[:len(a)-len(digits)], true
		}
		if r.unspecified {
			return nil, f, r.status
		}
		pending = 0
		// `+i` removes the attribute where `-i` adds it, which is the one
		// place a shell spells an option with a plus.
		f.remove = a[0] == '+'
	letters:
		for at, c := range a[1:] {
			if !strings.ContainsRune(known, c) {
				return nil, f, r.refuseOption(name, a, known)
			}
			f.added = f.added || !f.remove
			if c != 'f' {
				f.letters += string(c)
				sign := byte('-')
				if f.remove {
					sign = '+'
				}
				f.letterSigns += string(sign)
			}
			if strings.ContainsRune(r.sem().DeclareOptionsWithoutEffect, c) {
				f.inert = true
				// The dialect spells the letter and this engine models
				// nothing it does, so it is taken in silence and decides
				// nothing — see Semantics.DeclareOptionsWithoutEffect for
				// which lie that is and why it is the smaller one.
				continue
			}
			switch c {
			case 'i':
				f.integer = true
				if f.remove {
					// The explicit `+i`, which is the only spelling allowed
					// to cancel a forced attribute. See declareFlags.
					f.integerOff = true
				}
			case 'r':
				f.readonly = true
				f.readonlyOff = f.remove
			case 'x':
				f.export = true
			case 'C':
				// The compound-variable attribute — `typeset -C c`, which
				// the one dialect that has it also spells `compound c`.
				// Recorded like `-A` and for the same reason: it changes
				// what a later `c=( … )` and a later `${c.a}` mean.
				f.compoundVar = true
			case 'A':
				// The associative attribute, and unlike `-a` it must be
				// recorded: it changes what a later subscript *means*, the
				// way `-i` changes what a later assignment means.
				f.assoc = true
			case 'l':
				f.lower = true
			case 'u':
				f.upper = true
			case 'g':
				// Global rather than local: the assignment reaches the
				// global cell however deep the function stack is.
				f.global = true
			case 'T':
				// The tie: the operands are a scalar, an array and an
				// optional separator rather than a list of names. Recorded
				// here and read by biDeclare, which takes its own path for
				// them.
				f.tie = true
				if r.sem().DeclareTypeLetter != DeclareTypeLetterNamesAType {
					break
				}
				// Under the *type* reading the rest of the word is the
				// type's name rather than more letters — `-TPt v` declares
				// `v` of type `Pt` — so this is the second case that stops
				// the walk, exactly as `M` is. Asked of the dialect for the
				// same reason: the other shell that spells the letter reads
				// `typeset -TU S s` as the tie plus the unique attribute,
				// and swallowing the `U` as a type name would take a line
				// that shell runs and read it as another one.
				if rest := a[at+2:]; rest != "" {
					f.typeName, f.typeNamed = rest, true
					break letters
				}
			case 'n':
				// A name *reference*: what the operand carries is the name
				// this one stands for, and every later read, write and
				// `unset` through it lands there. Recorded rather than acted
				// on here, because what the letter does depends on the
				// operand — see declareNameref.
				//
				// Under a plus it is the reference being taken away, which
				// is its own declaration rather than the absence of one —
				// see namerefAttributeRemoved.
				f.nameref, f.namerefOff = !f.remove, f.remove
			case 'I':
				// Inherit: the cell this declaration makes starts from what
				// the enclosing scope's name holds instead of from nothing.
				// Recorded rather than acted on here, because what it means
				// depends on whether a scope is taken at all and on whether
				// the operand carries a value — see declareEmpty.
				//
				// Both signs, which is measured: `+I` asks for the same
				// thing. So the sign is read and discarded here rather than
				// kept the way `-h`'s is.
				f.inherit = true
			case 't':
				// The trace attribute. Recorded and listed and otherwise
				// inert, which is measured rather than a shortcut: on all
				// three shells with the letter the name reads back, expands
				// and assigns exactly as it would without it, and the only
				// reader is the DEBUG trap. So this is the same shape `-U`
				// and `-H` have — a property of the *name* that every later
				// listing consults — and `+t` takes it off again.
				f.traced = true
			case 'U':
				// Keep only the first occurrence of each element. Like
				// `-i` and the case attributes it is a property of the
				// *name* rather than of this assignment, so it is
				// recorded and consulted by every later write.
				f.unique = true
			case 'h':
				if r.sem().DeclareHideInScopeLetter == DeclareHideInScopeLetterTakesAString {
					// The other reading: a string argument this shell
					// records nowhere. It may ride on the letter, and where
					// it does the rest of the word is that string and not
					// more letters — `typeset -hx s q=1` leaves `q`
					// *unexported*, the `x` having been the argument. Same
					// shape the `M` letter's mapping name has, and the same
					// reason this is one of the cases that stop the walk.
					f.hideString = true
					if rest := a[at+2:]; rest != "" {
						f.hideStringNamed = true
						break letters
					}
					break
				}
				// Hide *in scope*: a local declaration of a name with this
				// attribute is an ordinary parameter rather than the special
				// one it is spelled like, and the tie the name is half of
				// goes on without it. Recorded with its sign because `+h`
				// takes off an attribute an outer declaration set, which is
				// the one spelling that changes an answer — see
				// hideinscope.go.
				f.hide, f.hideNamed = !f.remove, true
			case 'H':
				// Hide the value from listings. The name is declared, holds
				// what it holds and reads back exactly as it would without
				// the letter — only a listing that would have written
				// `=value` writes the bare name instead. Recorded rather
				// than acted on at declaration time, because it is a
				// property of the name that a later listing consults, the
				// same shape `-i` has.
				f.hidden = true
			case 'f':
				f.function = true
				f.functionOff = f.remove
			case 'm':
				// The operands are patterns. Recorded rather than acted on
				// here, because what it does depends on every other letter
				// on the line — see declarematching.go.
				f.matching = true
				f.matchNames = f.remove
			case 'M':
				// A character mapping, whose name may ride on the letter —
				// `-Mtolower` — or arrive as the first operand. Where it
				// rides, the rest of the word is that name and not more
				// letters, which is why this is the one case that stops the
				// walk. See interp/declaremapping.go.
				//
				// Asked of the dialect rather than taken from the letter,
				// because the other shell that spells `M` reads the rest of
				// the word as more letters: `functions -Mt` is a refusal of
				// `-t` there, and swallowing the `t` as a mapping name would
				// have taken a refused line and run it. That reading also
				// takes the letter off this parser's hands entirely, before
				// the declaration is reached at all — see biFunctions.
				if r.sem().DeclareMappingLetter == DeclareMappingLetterRegistersAMathFunction {
					break
				}
				f.mapping = true
				if rest := a[at+2:]; rest != "" {
					f.mappingName, f.mappingNamed = rest, true
					break letters
				}
			case 'E':
				// The float attribute again, under the letter that carries
				// a *format* with it. Guarded against an integer letter
				// already read for the reason `F` is, and the two float
				// letters settle between themselves the same way: the first
				// written wins, so `-EF 3` is the exponent form and `-FE 3`
				// the plain one.
				//
				// A dialect that spells the letter and not as a float
				// format falls through to nothing, which is right: it is
				// whatever else the letter is there — today, nothing this
				// engine models, and DeclareOptions and
				// UnimplementedOptionLetters have settled that between them
				// before the loop reached here.
				if r.declareOptionTakesANumber('E') && !f.integer && !f.float {
					f.float, f.floatExponent = true, true
				}
			case 'F':
				if r.declareOptionTakesANumber('F') {
					// The letter is a float's precision in this dialect
					// rather than bash's function listing, and taking a
					// number is that fact rather than a second one: the
					// function listing takes none anywhere it is spelled.
					// See Semantics.DeclareOptionsTakingANumber.
					//
					// The two attributes cannot both stand and the first
					// letter written wins: `typeset -Fi 3` and `-iF 3` are
					// settled by the word ending at the first of them, and
					// `typeset -i -F 3` — two words, where both letters are
					// really read — by this. There is no guard on the
					// integer letter to match, because applyAttributes lets
					// the float branch speak last and one there would decide
					// nothing; a mutant removing it changed no answer.
					if !f.integer {
						f.float = true
					}
					break
				}
				f.funcNames = true
				f.funcNamesOff = f.remove
			case 'L', 'R', 'Z':
				if !r.declareOptionTakesANumber(byte(c)) {
					// The dialect spells the letter but not as a width, so
					// it is whatever else it is there — today, nothing this
					// engine models, which DeclareOptions and
					// UnimplementedOptionLetters have already settled
					// between them before the loop reached here.
					break
				}
				// What `Z` is and which justification survives a pair of
				// them are both dialect answers — see recordWidthLetter,
				// where the two axes are asked.
				r.recordWidthLetter(&f, byte(c))
			case 'p':
				// Print rather than declare. `+p` prints too — measured in
				// both shells that spell the option at all.
				f.print = true
			case 'a':
				// The indexed-array attribute, which is what `-A` is for
				// tables. An array is otherwise dynamic here — `typeset -a
				// arr` followed by `arr[0]=x` worked before the letter was
				// recorded at all — so what the attribute adds is the one
				// thing that cannot be inferred from a store with nothing in
				// it: that the name is an *array* holding nothing rather
				// than a scalar holding nothing. `a[i]=(p q)` is the
				// construct that has to tell those apart, since it splices
				// into the first and is refused on the second (#1330).
				f.array = true
			}
			if attached || f.remove ||
				!r.numberEndsTheWord(byte(c), a[at+2:], args[i+1:]) {
				continue
			}
			// The next word is this letter's number, so the word ends here
			// and whatever else was written in it is the letter's argument
			// rather than more options — see numberEndsTheWord.
			//
			// A word that already carried its number attached is not this
			// shape and takes no second one: `typeset -F3 4 x=1.5` is `not
			// an identifier: 4` in that shell, so the `4` stays an operand.
			// Neither is a plus word: `typeset +F 3 v=1.5` declares two
			// names there, the letter taking nothing back.
			pending = byte(c)
			break
		}
	}
	r.blockALaterPlus(&f)
	r.rankTheNumericLetters(&f)
	rest = args[i:]
	if code := r.refuseTheReferenceLetterInCompany(name, f); code != 0 {
		return nil, f, code
	}
	if code := r.refuseAFunctionLineLetter(name, f, rest); code != 0 {
		return nil, f, code
	}
	return rest, f, 0
}

// refuseTheReferenceLetterInCompany refuses a declaration that wrote the `n`
// letter beside another one, where the dialect will not read the pair at all.
//
// See Semantics.NamerefLetterStandsAlone for the panel. The complaint is the
// **bare** usage block: none of the sentence an option the builtin does not
// have gets in front of it, which is the control that says the refusal is
// about the combination rather than about a letter. The status is the one a
// bad option leaves, and it travels the same road out of the parse — so it
// ends the script exactly where a bad option does, which is what the dialect
// that answers Yes measures.
//
// Asked only where an `n` letter really stood beside another, so a dialect
// with no reference at all is never asked. The sign is not read: `+n -i` is
// refused there as squarely as `-ni`, which is what says the parser is
// looking at the letters and not at what they would have done.
func (r *Runner) refuseTheReferenceLetterInCompany(name string, f declareFlags) int {
	if !f.nameref && !f.namerefOff {
		return 0
	}
	if strings.IndexFunc(f.letters, func(c rune) bool { return c != 'n' }) < 0 {
		return 0
	}
	if !r.ask(r.sem().NamerefLetterStandsAlone, "the reference letter refusing company") {
		if r.unspecified {
			return r.status
		}
		return 0
	}
	r.builtinUsageLine(name)
	return orDefault(r.diag().BuiltinBadOptionStatus, 2)
}

// refuseAFunctionLineLetter refuses a letter this shell has on a variable
// line and has not got on a function one, and answers 0 for every other line.
//
// It is here rather than in the letter loop's refuseOption because the letter
// is *known*: the parser took it and gave it its other meaning, so nothing
// downstream would ever ask whether it was missing. And it is here rather
// than in declareNames — where it was first written — because the *fatality*
// hangs off this function's status: Semantics.TypesetBadOptionFatal is asked
// only of a non-zero code from the option parse, so a refusal raised further
// in would report the same sentence and let the script carry on, where the
// sibling letter beside it in Diagnostics.UnimplementedOptionLetters stops it.
// One refusal, two exits, is exactly the drift this repository keeps finding.
//
// **The table is what says the letter is missing**, and the bare word is
// refused with the named one. It used to require operands, on the reading
// that ksh93's `typeset -fu` with no names is the *listing* of the functions
// the letter marks — which is true and is not this table's business: `u` is
// not in ksh93's row here, precisely because the letter is built. A letter
// that really is missing has no listing to write either, so requiring an
// operand left the bare form falling through to the whole function table —
// `typeset -ft` writing every body in the shell where the letter asks for the
// traced ones (#3101).
func (r *Runner) refuseAFunctionLineLetter(name string, f declareFlags, rest []string) int {
	if code := r.refuseAVariableOnlyLetter(name, f, rest); code != 0 {
		return code
	}
	if !f.function || f.functionOff {
		return 0
	}
	c, missing := f.letterMissingOnAFunctionLine(
		r.diag().UnimplementedOptionLettersOnAFunctionLine[name])
	if !missing {
		return 0
	}
	// The sentence is UnimplementedOptionLetters' own, said in the one place
	// that says it, so the two lists cannot come to word the same fact
	// differently.
	r.complainAboutOption(name, "%s: -%c is not implemented yet\n",
		r.builtinComplaintName(name), c)
	return orDefault(r.diag().BuiltinBadOptionStatus, 2)
}

// refuseAVariableOnlyLetter refuses a letter that makes a *kind of variable*
// on a line that names functions — see
// Diagnostics.VariableOnlyLettersOnAFunctionLine for the measurement.
//
// The first such letter the line wrote under a minus, in the order it was
// written: `declare -f -i -a f` names `-i`. Operands are required, and a `-p`
// makes the line a listing again, which is measured — `declare -f -a` alone
// and `declare -f -p -a f` are both 0.
func (r *Runner) refuseAVariableOnlyLetter(name string, f declareFlags, rest []string) int {
	if w := r.diag().DeclareMakesNoFunction; w != "" && (f.function || f.funcNames) &&
		!f.functionOff && !f.funcNamesOff && !f.print {
		for _, operand := range rest {
			if strings.Contains(operand, "=") {
				r.complainAboutOption(name, "%s\n", Wording(w, "", r.builtinComplaintName(name)))
				return 1
			}
		}
	}
	refused := r.diag().VariableOnlyLettersOnAFunctionLine[name]
	if refused == "" || (!f.function && !f.funcNames) || f.functionOff || f.funcNamesOff ||
		f.print || len(rest) == 0 {
		return 0
	}
	for i, c := range f.letters {
		if strings.ContainsRune(refused, c) && i < len(f.letterSigns) && f.letterSigns[i] != '+' {
			r.complainAboutOption(name, "%s: -%c: invalid option\n", r.builtinComplaintName(name), c)
			return 1
		}
	}
	return 0
}

// rankTheNumericLetters settles a declaration that wrote more than one of the
// three numeric type letters, where the dialect ranks them rather than taking
// the first one written.
//
// Measured 2026-09-14 on ksh93u+ 2012-08-01, each read back with `typeset -p`:
//
//	typeset -iF 3 a=1.5     typeset -F 3 a=1.500
//	typeset -Fi 3 a=1.5     typeset -F 3 a=1.500
//	typeset -i -F 3 a=1.5   typeset -F 3 a=1.500
//	typeset -F -i a=1.5     typeset -F a=1.5000000000
//	typeset -EF 3 a=1.5     typeset -E 3 a=1.5
//	typeset -FE 3 a=1.5     typeset -E 3 a=1.5
//
// So the rank is `E` over `F` over `i`, whichever order they stand in and
// whether they share a word or not — where zsh takes the first letter written
// and answers the pairs four different ways. See
// Semantics.NumericTypeLetterPrecedence.
//
// `-iE` is not one of these and never reaches here: that pair is refused
// outright in the column that ranks, which is
// NumericTypeLettersAreExclusive's question and is asked later.
//
// Read off the letters as written for the reason numericTypeLetterCompany is:
// the loop above has already discarded a loser by *order*, so the flags can no
// longer say what the line held. And gated on more than one of them being
// written, so a declaration with a single numeric letter meets no question at
// all.
func (r *Runner) rankTheNumericLetters(f *declareFlags) {
	if numericLetterCount(f.letters) < 2 {
		return
	}
	if r.sem().NumericTypeLetterPrecedence != NumericLetterFloatOutranksTheInteger {
		return
	}
	switch {
	case strings.ContainsRune(f.letters, 'E') && r.declareOptionTakesANumber('E'):
		f.float, f.floatExponent = true, true
	case strings.ContainsRune(f.letters, 'F') && r.declareOptionTakesANumber('F'):
		f.float, f.floatExponent = true, false
	default:
		return
	}
	if f.integer {
		f.integer = false
		// The number goes with the attribute. Whichever numeric letter the
		// word's *last* one was, it read the following digits as its own —
		// `typeset -Fi 3 a=1.5` is the `i` taking a base and `typeset -iF 16
		// a=255` is the `F` taking a precision — and the rank then hands the
		// name to the float letter. Measured on ksh93u+ 2012-08-01: the
		// first lists `typeset -F 3 a=1.500` and the second `typeset -F 16
		// a=255.0000000000000000`, so the number the losing letter read is
		// the winner's, at whatever it means there.
		if f.baseNamed && !f.precisionNamed {
			f.precision, f.precisionNamed = f.base, true
		}
		f.base, f.baseNamed = 0, false
	}
}

// numericLetterCount is how many of the three a declaration's letters hold.
func numericLetterCount(letters string) int {
	n := 0
	for _, c := range "EFi" {
		if strings.ContainsRune(letters, c) {
			n++
		}
	}
	return n
}

// blockALaterPlus is one dialect's reading of a declaration that writes both
// signs: a plus word that follows a minus word takes nothing off.
//
// `typeset -x e=1; typeset +x e` unexports in every shell that has the
// letters, and `typeset -i +x e` leaves `e` exported in ksh93 — the `-i` has
// made the line a declaration, and the plus word rides along as part of it.
// Order is the whole of it: `typeset +x -i e` *does* unexport there, so this
// is a minus **before** a plus and not simply both being present.
//
// See Semantics.EarlierDeclarationLetterBlocksALaterPlus, which is where the
// panel is and why the field names an order.
func (r *Runner) blockALaterPlus(f *declareFlags) {
	minus := strings.IndexByte(f.letterSigns, '-')
	if minus < 0 || strings.LastIndexByte(f.letterSigns, '+') < minus {
		return
	}
	if !r.ask(r.sem().EarlierDeclarationLetterBlocksALaterPlus,
		"a plus word after a minus word taking nothing off") {
		return
	}
	f.remove, f.integerOff, f.readonlyOff, f.functionOff = false, false, false, false
	f.hide, f.matchNames = f.hideNamed, false
	f.letterSigns = strings.ReplaceAll(f.letterSigns, "+", "-")
}

// numberEndsTheWord reports whether a letter just read is about to take the
// next word as its number, and so is the last letter its own word can carry.
//
// A number-taking letter does *not* end its word on its own: measured
// 2026-09-07 in zsh 5.9.2, `typeset -ix n=255` exports and `typeset -ir n=255`
// freezes, so the letters behind it are ordinary options when no number
// follows. What discards them is a number actually being taken — `typeset -ix
// 16 n=255` leaves `n` unexported and based 16, `typeset -Fx 3 v=1.5` leaves
// `v` unexported at three places, and `-iH n=5` keeps the hiding that `-iH 5
// n` would lose. So the lookahead is the whole rule: a following word of
// digits is what turns the rest of this word into the letter's own argument.
//
// That is also what keeps the dialect from being asked where a script asked
// nothing. Semantics.IntegerAttributeTakesABase refuses when it has not been
// answered, and a plain `typeset -i n` or `typeset -irx v=1` must meet no
// question at all — neither has a number in it to have a question about.
//
// A malformed attached number keeps its word: `-F3g` and `-i16x` reach here
// with a rest that begins in a digit, which attachedOptionNumber has already
// declined, and the letter loop goes on to refuse the digit as an option. zsh
// refuses those two as well and says `bad precision value: 3g` where this
// says the digit is a bad option, so the shapes agree on the refusal and
// disagree on the sentence — recorded rather than modeled, since no script
// writes either.
func (r *Runner) numberEndsTheWord(c byte, rest string, later []string) bool {
	if !declareOptionMayTakeANumber(c, r.sem().DeclareOptionsTakingANumber) {
		return false
	}
	if rest != "" && rest[0] >= '0' && rest[0] <= '9' {
		return false
	}
	if len(later) == 0 || !isAllDigits(later[0]) {
		return false
	}
	if !r.declareOptionTakesANumber(c) {
		return false
	}
	if rest == "" {
		// The letter ends its word, which every dialect with a
		// number-taking letter reads as taking the number.
		return true
	}
	// A letter with more word behind it. One dialect takes the number here
	// and discards the rest of the word; the other leaves the number an
	// operand. Asked last, so a dialect whose letter takes no number at all
	// — and a word with no following digits — never meets the question.
	return !r.ask(r.sem().DeclareNumberDetachedOnlyAtTheWordEnd,
		"a detached number reaching a letter that does not end its option word")
}

func biDeclare(r *Runner, _ context.Context, args []string) int {
	name := r.inBuiltin
	if name == "" {
		name = "declare"
	}
	known := r.sem().DeclareOptions
	if known == "" {
		known = declareOptionLetters
	}
	args, f, code := r.parseDeclareFlags(name, args, known)
	if code != 0 {
		// A bad option ends the script where the dialect counts `typeset`
		// among its special builtins — ksh93, where any of the builtin's
		// failures is fatal, so the honest refusal of a letter it has and
		// this shell does not stops the script the same way.
		if r.ask(r.sem().TypesetBadOptionFatal, "a bad `typeset` option ending the script") {
			r.status = code
			r.fatalUsageQuiet()
		}
		return code
	}
	return r.declareNames(name, args, f)
}

// declareNames is the declaration itself, once the letters have been read.
//
// Its own function because `integer` is the same declaration under a second
// name with the integer attribute already decided, and a second copy of this
// is the thing that would drift. See integerbuiltin.go.
func (r *Runner) declareNames(name string, args []string, f declareFlags) int {
	if f.function || f.funcNames {
		// A marking letter written under a plus, in the one shell that
		// refuses it. Ahead of everything because it is a refusal of the
		// *line* rather than of anything it went on to ask for: measured
		// 2026-09-12 on zsh 5.9.2, `functions +u`, `typeset +fu` and
		// `typeset +fu 'p*'` are all `invalid option(s)` at 1, with and
		// without operands alike. See Diagnostics.MarkingLettersUnderPlus
		// for why the refused set is written out rather than derived from
		// the letters that mark.
		if refusal := r.diag().MarkingUnderPlusRefusal; refusal != "" {
			if _, under := f.markingLetterUnderPlus(r.diag().MarkingLettersUnderPlus); under {
				r.diagf("%s: %s\n", r.builtinComplaintName(name), refusal)
				return 1
			}
		}
	}
	if f.mapping {
		switch r.sem().DeclareMappingLetter {
		case DeclareMappingLetterNamesACharacterMapping:
			return r.declareMapping(name, args, f)
		case DeclareMappingLetterUnspecified:
			r.errf("%s\n", r.diag().Report(r.name(), r.line,
				r.unanswered("the `M` letter of a declaration")))
			r.status, r.unspecified = 2, true
			return r.status
		}
	}
	if f.hideString && !f.hideStringNamed {
		// The string arrives as the next word where it did not ride on the
		// letter, and a letter with nothing behind it at all is refused with
		// this shell's own sentence and its usage block. Ahead of every
		// reading below because the word is consumed either way: a line read
		// without taking it would declare a name that shell never saw.
		if len(args) == 0 {
			r.diagf("%s\n", Wording(r.diag().DeclareHideStringMissing,
				"%[1]s: -h: string argument expected", r.builtinComplaintName(name)))
			return r.refuseWithUsage(name)
		}
		args = args[1:]
	}
	if f.matching {
		switch r.sem().DeclareMatchingLetter {
		case DeclareMatchingLetterMoves:
			// The letter *moves* a parameter here, and the operands are
			// `new=old` rather than patterns — a different command with the
			// same spelling. Ahead of the empty-operand branch below because
			// this reading has nothing to fall back to: measured, a bare
			// `typeset -m` writes nothing at all where the pattern reading's
			// bare form writes the whole table. See interp/declaremove.go.
			return r.declareMove(name, args, f)
		case DeclareMatchingLetterUnspecified:
			// A dialect that spells the letter and has not said which of the
			// two it means. Refused by name rather than given one shell's
			// reading, the way every unanswered axis is: the two share no
			// operand grammar, so a guess would read a script's patterns as
			// renames or the other way about.
			r.errf("%s\n", r.diag().Report(r.name(), r.line,
				r.unanswered("the `m` letter of a declaration")))
			r.status, r.unspecified = 2, true
			return r.status
		}
		if len(args) == 0 {
			// `-m` with nothing to match is *ignored*, which is measured
			// rather than assumed: `typeset -m` writes the whole parameter
			// table exactly as the bare word does, and `typeset +m` writes
			// it too. So the letter is dropped here and whatever else was
			// written decides, which is how a bare `typeset +m` reaches the
			// bare listing rather than a filtered one.
			f.matching, f.matchNames = false, false
			if withoutMatching(f) == (declareFlags{}) {
				return r.bareDeclarationListing()
			}
		} else {
			return r.declareMatching(name, args, f)
		}
	}
	if f.function && len(args) > 0 && r.markingLetters(f.letters) {
		// Not a listing at all: this line *makes* the names functions whose
		// bodies are read the first time they are called. See
		// Runner.SetFunctionMarkedUndefined for the seam and
		// Semantics.FunctionLettersThatMarkUndefined for which letters say
		// so.
		//
		// Ahead of the listing branch because it is the same letters read
		// the other way: `typeset -fu nm` writes nothing and leaves `nm`
		// waiting, where this engine answered with a silent listing of a
		// function that did not exist (#1753).
		//
		// Both signs come here and the hook is told which, because the two
		// are the same record written and unwritten: `typeset +ft f` takes
		// the tracing mark off in ksh93u+, measured, where `typeset +fu nm`
		// is `invalid option(s)` in zsh. The refusing dialect never reaches
		// this line — Diagnostics.MarkingUnderPlusRefusal is raised at the
		// top of this function — so routing the plus form here costs it
		// nothing and gives the shell that has one somewhere to put it.
		return r.markUndefinedFunctions(r, args, f.letters, f.remove)
	}
	if f.function || f.funcNames {
		// The function table rather than the variables, and the *sign* of
		// the `f` letter picks the shape: `-f` writes the functions
		// themselves and `+f` writes only their names. `-F` is a third
		// spelling of the second, in the one dialect where that letter is
		// not a float's precision. `-p` alongside changes nothing — the
		// flags already mean print, and `typeset +fp` is the name (#1576).
		//
		// The same reading declareMatching already makes of the same field:
		// `typeset +fm '_*'` names its matches and `typeset -fm '_*'` writes
		// their bodies. Asking `f.funcNames` alone here is what left the
		// plus form printing bodies while the pattern form printed names.
		namesOnly := f.funcNames && !f.funcNamesOff
		if f.functionOff || f.funcNamesOff {
			if !r.ask(r.sem().FunctionNamesUnderPlus, "`typeset +f` naming its functions") {
				if r.unspecified {
					return r.status
				}
				// The sign is not a request for names here: it takes the
				// function attribute *off*, and a name that is no longer a
				// function is nothing this builtin has to say. What is left
				// is the bare word, whose listing is a different command
				// entirely.
				//
				// Measured 2026-09-12 on bash 5.3.15: `declare +f` with no
				// operands is byte-identical to `declare` — `diff <(declare
				// +f) <(declare)` is empty — and `declare +F` is the same
				// line again. With operands it is a silent 0 that leaves the
				// function defined, which `declare -F` afterwards shows. So
				// the letter reaches the listing rather than writing one of
				// its own (#1754).
				if len(args) == 0 {
					return r.bareDeclarationListing()
				}
				return 0
			}
			if r.unspecified {
				return r.status
			}
			namesOnly = true
		}
		if r.unspecified {
			return r.status
		}
		narrowed := false
		if len(args) == 0 {
			// A listing carrying a marking letter and no operands is not the
			// whole table: it is the functions holding that mark, which is
			// the same set a bare `autoload` writes out in the shell with
			// the notion. See Runner.SetMarkedFunctions for the measurements
			// and for why the letters are a union.
			//
			// Only with no operands. With them the letters are attributes
			// being *set* on the names rather than a filter over them —
			// measured, `functions -U g` gives the ordinary function `g` the
			// U mark, and `typeset -f +U f2` takes it off `f2` and writes
			// nothing whatever `f2` holds. The minus half of that is the
			// marking branch above; the plus half is not modeled here.
			// And the same rule for the letters that are *attributes* of a
			// function rather than marks on one: with no operands they are
			// a filter over the table and a union, so `declare -Frx` is the
			// functions holding either. See
			// Semantics.FunctionAttributeLetters, and the operand form just
			// below, which sets instead.
			if attrs := r.functionAttributeLettersWritten(f); attrs != "" {
				args, narrowed = r.functionsHoldingAttributes(attrs), true
			} else if minus, _ := r.functionLineNamesNoFunction(name, f); minus {
				// A letter no function can hold: the population is empty
				// and nothing is
				// written. Measured 2026-09-16 on bash 5.3.20, `declare -f
				// -a` and `declare -F -i` are each silent at 0 with functions
				// defined, where `declare -F +i` is the whole listing.
				args, narrowed = nil, true
			}
			if marked, narrow, plus := r.markedFunctionListing(f); narrow {
				args, narrowed = marked, true
				if plus {
					// A marking letter under a plus names its functions
					// where a minus writes them out, and it does so however
					// the `f` letter was signed: `typeset -f +U` is the name
					// alone and `functions +U -u` names them too, so neither
					// the `f` letter's sign nor the last option word's
					// answers this.
					namesOnly = true
				}
			}
		}
		// With operands the same letters *set* the attribute rather than
		// choosing a population: measured 2026-09-16 on bash 5.3.20,
		// `declare -fr b`, `declare -Fr b`, `declare -rf b` and `typeset -fr
		// b` all freeze `b` and write nothing, where without a letter the
		// same line is a listing. Behind the narrowing above, which only
		// happens with no operands, so the two readings cannot both fire.
		if !narrowed && len(args) > 0 {
			attrs, removed := r.functionAttributeLettersWritten(f), r.functionAttributeLettersRemoved(f)
			if attrs != "" || removed != "" {
				return r.setFunctionAttributes(name, args, attrs, removed)
			}
			if _, written := r.functionLineNamesNoFunction(name, f); written && !f.print {
				// Only a plus of a letter no function holds is left here —
				// the minus was refused as the options were read — and it
				// takes away what was never there: `declare -f +a f` is a
				// silent 0 in bash 5.3.20, not the listing it would be
				// without the letter.
				return 0
			}
		}
		// `-p` alongside asks for the reissuable shape, so it is also where
		// extended debugging's location does *not* go: measured on bash
		// 5.3.15, `shopt -s extdebug; declare -F g` is `g 1 ./lib2.sh` and
		// `declare -Fp g` is `declare -f g` with no location in it at all.
		return r.declareFunctions(args, narrowed, namesOnly, f.funcNames,
			r.LocatesFunctions() && !f.print, f.print)
	}

	if f.tie {
		switch r.sem().DeclareTypeLetter {
		case DeclareTypeLetterNamesAType:
			// The letter names a *type* here and the operands are type
			// names rather than a scalar and an array — a different command
			// with the same spelling. Ahead of every branch below because
			// this reading has nothing in common with the tie's: a bare
			// `typeset -T` is a listing under both readings and the two
			// list different things, and the operand form shares no
			// grammar at all. See interp/declaretype.go.
			return r.declareType(name, args, f)
		case DeclareTypeLetterUnspecified:
			// A dialect that spells the letter and has not said which of
			// the two it means. Refused by name rather than given one
			// shell's reading, the way every unanswered axis is: a guess
			// would tie two of a script's type names together or declare a
			// scalar and an array as types.
			r.errf("%s\n", r.diag().Report(r.name(), r.line,
				r.unanswered("the `T` letter of a declaration")))
			r.status, r.unspecified = 2, true
			return r.status
		}
	}

	if numericTypeLetterCompany(f) &&
		r.ask(r.sem().NumericTypeLettersAreExclusive, "the integer and float letters on one declaration") {
		// The two numeric type letters cannot both stand here, and the
		// refusal is the builtin's usage block rather than a complaint about
		// either letter — measured, both orders answer the same way. See the
		// axis; the other reading lets the first letter written win, which
		// the parse has already settled by the time this is reached.
		return r.refuseWithUsage(name)
	}
	if widthLetterCompany(f) && r.declareOptionTakesANumber('L') &&
		r.ask(r.sem().WidthLettersExcludeTheIntegerLetter,
			"the integer and width letters on one declaration") {
		// The integer letter and a width letter cannot both stand here, and
		// the refusal is the builtin's usage block rather than a complaint
		// about either letter — measured, every order answers the same way.
		// See the axis; the other reading lets the first letter written win,
		// which the parse has already settled by the time this is reached.
		//
		// Gated on the dialect having the letters as widths at all, so a
		// shell where `L` means something else never meets the question: a
		// bare `strings.ContainsAny` would ask it of a letter this engine
		// read as a width nowhere.
		return r.refuseWithUsage(name)
	}
	if r.unspecified {
		return r.status
	}
	if f.hidden {
		switch r.sem().DeclareHideValueLetter {
		case DeclareHideValueLetterUnspecified:
			// A dialect that spells the `H` letter and has not said which of
			// the two things it is. Refused by name rather than given one
			// shell's reading, the way every unanswered axis is: a guess
			// either withholds a value a script's listing is meant to carry
			// or writes a letter into one that never had it. See
			// interp/declarehide.go.
			r.errf("%s\n", r.diag().Report(r.name(), r.line,
				r.unanswered("the `H` letter of a declaration")))
			r.status, r.unspecified = 2, true
			return r.status
		case DeclareHideValueLetterIsAnInertAttribute:
			if hideValueLetterCompany(f) {
				// The letter and the integer attribute are exclusive under
				// this reading: measured, `typeset -iH n=5` and `integer -H
				// n=5` are both the builtin's usage block at 2 with nothing
				// above it. zsh takes the pair and lists `typeset -i n`, so
				// this belongs to the reading rather than to the letter.
				return r.refuseWithUsage(name)
			}
		}
	}

	if len(args) == 0 && !f.tie {
		if code, answered := r.declarationListing(f); answered {
			return code
		}
	}

	if f.tie && len(args) == 0 {
		// `typeset -T` with nothing to tie lists the ties there are, both
		// halves of each — measured, and it is the only filtered listing
		// this builtin has: every other attribute letter with no names is a
		// filter this engine does not build. This one is built because a
		// tie is the one attribute whose *listing* is how a script finds the
		// pairs at all.
		//
		// The sign picks the shape here as it does everywhere else on this
		// builtin: `typeset +T` writes the tied names and no values.
		return r.tieListing(f.remove)
	}
	if f.tie && !f.remove && len(args) > 0 {
		// The operands of `-T` are not a list of names: they are a scalar,
		// an array and — where a third is given — the separator. Ahead of
		// `-p` because `typeset -pT` with names is not a shape any shell
		// measured has, and behind the bare listing because `typeset -T`
		// alone *is* a listing there.
		return r.declareTie(name, args, f)
	}
	if f.print {
		if len(args) == 0 && f.attributeLetterWritten() {
			// `-p` beside an attribute letter and no operand is the
			// *filtered* listing, and the filter is built: it is the one
			// `export -p` already walks this very renderer with. The comment
			// that used to stand here said it was not built, and so this fell
			// through to the whole table — `declare -pi` wrote every variable
			// the shell had, at status 0, where the letter asked for the
			// integer ones.
			//
			// Unanimous across the panel, measured 2026-09-15 over a table
			// holding one integer, one readonly, one export and one plain
			// scalar, each shell counting the rows of its own `typeset -p -i`
			// against its own bare `typeset -i`:
			//
			//	bash 5.3.20   9 filtered   (bare -i: 9)
			//	bash 3.2      8 filtered   (bare -i: 8)
			//	zsh 5.9.2    19 filtered   (bare -i: 30)
			//	ksh93        10 filtered   (bare -i: 10)
			//
			// So it is the core's and not an axis, and all three of our
			// dialects had it: bash wrote 23 rows where the letter asked for
			// 1, zsh 62 and ksh 13.
			//
			// Through declarePrintForm rather than through the bare listing,
			// because `-p` picks the *shape* as well as the set: zsh and
			// ksh93 both write `n1=1` for a bare `typeset -i` and `typeset -i
			// n1=1` for `typeset -p -i`, so routing this to the bare walk
			// would have filtered the rows and then written them in the wrong
			// form in two columns of three.
			//
			// Only with no operand. `declare -pi name` is not a filter in any
			// of them — bash writes `declare -- p1="plain"` at 0 for a name
			// carrying no integer attribute — so the operand form is left to
			// declarePrint below, which already answers it.
			keep, answered := r.attributeFilter(f)
			if !answered {
				return r.status
			}
			if keep == nil {
				// A letter this dialect spells and this engine records
				// nothing for. It is still an attribute to select on and no
				// name carries it, so the listing is empty rather than whole
				// — the same answer declarationListing gives.
				return 0
			}
			return r.declarePrintFiltered(nil, r.sem().DeclareListing, true, keep, true)
		}
		// The operands of this listing are its own to perform first, in the
		// two columns that perform them: `typeset -p s=5` declares and shows
		// in one line, and what it shows is what it has just stored. See
		// interp/declareprintoperand.go, which also holds the listing back
		// until Runner.assignOperands has landed the parenthesized ones.
		if code, took := r.declarePrintPerformsItsOperands(name, args, f); took {
			return code
		}
		// A letter alongside `-p` that no listing filters on decides nothing,
		// and refusing the combination would break the plain use to be honest
		// about the rare one.
		return r.declarePrint(args)
	}

	// The operands are names, and this is where that is finally asked.
	//
	// `export`, `readonly`, `local`, `unset` and `set -A` all go through
	// builtinNames and refuse a bad operand in the dialect's words; this
	// builtin split each operand at `=` and got on with declaring whatever
	// was on the left, so `typeset ':'` created a parameter called `:` and
	// reported success where the shells it copies refuse — two of them
	// fatally. Only the `-T` path asked, which is what made the wordings
	// exist without anything else reading them (#1096).
	//
	// Below `-f`, `-F` and `-p` on purpose: those take function names and
	// listing operands, which are not this rule — measured, `typeset -f 1x`
	// is a silent 1 in every shell that has the word and `typeset -p 1x`
	// gets a complaint about the *listing* rather than about the name.
	//
	// The complaint name rather than the invoked one, because one dialect
	// renames this builtin in its own diagnostics: ksh93's `integer 1x` says
	// `typeset: 1x: invalid variable name`, where zsh's says `integer`. That
	// is Diagnostics.BuiltinComplaintName doing the job it already does for
	// an unknown option.
	// A fatal refusal comes back as `ended` rather than as a control flag,
	// because the operands that *were* names are declared first and the
	// script stops after them — real zsh's `. f` over `typeset ":" ok=1`
	// leaves `ok` at 1, and this left it empty until the give-up was held
	// back (#1211). The flag is raised below the loop.
	// Kept before the loop, whose own `name` is the operand: a per-operand
	// refusal that has to say which word was written cannot reach the
	// parameter any more once the shadowing begins.
	complaintName := r.builtinComplaintName(name)
	// The two letters that carry `export`'s own refusals onto whatever word
	// the line was written with — the export letter and the reference
	// letter, under either sign. One condition, because the two refusals
	// travel together and a line that earns one earns the other.
	if f.export || f.nameref || f.namerefOff {
		// The **reason** a bad name is refused for: measured 2026-09-14 on
		// ksh93u+ 2012-08-01, `typeset -x 3` is `typeset: 3: is not an
		// identifier` where `typeset -r 3`, `typeset -l 3` and a bare
		// `typeset 3` are all `invalid variable name`. The builtin still
		// names *itself* in every one of them, so only the reason moves —
		// which is why this re-keys the wording and does not rename the
		// builtin. See Diagnostics.ExportLetterTakesExportsBadName.
		//
		// The reference letter says the same, under either sign: `typeset -n
		// 1x`, `typeset -n 1x=v` and `typeset +n 1x` are all `is not an
		// identifier` there. It was one row and recorded rather than modeled
		// while the only way to reach it was a name beginning with a digit;
		// the dotted operand below is what gave it three.
		if r.diag().ExportLetterTakesExportsBadName {
			outer := r.badNameWordedAs
			r.badNameWordedAs = "export"
			defer func() { r.badNameWordedAs = outer }()
		}
		// And **which** names are bad: neither letter will take one with a
		// dot in it, where every other letter on this word takes a compound
		// member. The company the letter keeps does not matter either —
		// `-xr` refuses as `-x` does. See Runner.dottedBuiltinName.
		outer := r.dottedOperandRefusedByALetter
		r.dottedOperandRefusedByALetter = true
		defer func() { r.dottedOperandRefusedByALetter = outer }()
	}
	args, code, ended := r.builtinNames(complaintName, args, false)
	if r.unspecified {
		// An unanswered axis inside the name check is not a refusal to carry
		// past: nothing was decided, so nothing is declared.
		return code
	}
	status := code

	// The freeze exemption one operand may be granted, put back at the end of
	// the builtin however it returns. Set per operand below rather than here,
	// because two names on one line are two separate questions — `readonly f
	// s; typeset -i f s` retypes both and `readonly f; typeset -i f s` only
	// the one that was frozen. See numericTypeLetterRetypesFrozen.
	outerRetyping := r.retypingFrozen
	defer func() { r.retypingFrozen = outerRetyping }()
	// And the name a refusal through a **reference** speaks of, which is
	// this operand's own where the dialect says so — see
	// Runner.declarationRefusalNamesTheOperand. Restored rather than cleared
	// for the reason above it: a declaration can be written inside one.
	outerSpokenAs := r.refusalSpokenAs
	defer func() { r.refusalSpokenAs = outerSpokenAs }()
	// A `-g` declaration written inside a call that was given an assignment
	// prefix writes the shell's own cell, *underneath* the binding the prefix
	// put on the name — so the temporary bindings come off for the length of
	// the line and go back on after it, with whatever the declaration left
	// underneath them recorded as what the outermost call gives back. See
	// interp/globalunderacallprefix.go.
	//
	// Around the whole line rather than around the store, because every step
	// of the declaration lands on that cell and not the value alone: measured
	// 2026-09-18 on bash 5.3.20, `declare -gx e=3` under a prefix leaves
	// `declare -x e="3"` behind and `declare -gr f=3` leaves `declare -r
	// f="3"`.
	if f.global {
		var held []string
		for _, a := range args {
			name, _, _, _ := declarationOperand(a)
			if name == "" {
				continue
			}
			held = append(held, name)
			if r.literalOperands[name] {
				// And the same letter recorded for the operand assignment
				// that runs after this builtin has returned, which is where
				// an array literal is really written — the seam
				// indexedLetterHere exists for, and for the same reason: the
				// parser hands the utility a bare name and the assignment
				// cannot see the letters. See interp/globaloperand.go.
				if r.globalLetterHere == nil {
					r.globalLetterHere = map[string]bool{}
				}
				r.globalLetterHere[name] = true
			}
		}
		// The running command's own prefix first, because it is the
		// innermost of the three stacks a `-g` write goes under: lifting it
		// reveals whatever the *call's* prefix put there, which is what the
		// lift below then has to see. The two undos run in the other order,
		// which is what a pair of defers gives. See
		// interp/globalunderitsownprefix.go.
		defer r.globalDeclarationRunsUnderItsOwnPrefix(held)()
		defer r.globalDeclarationRunsOnTheShellsOwnCell(held)()
	}

	// See biReadonly: a refusal made anywhere in an operand whose parentheses
	// the quoting hid names the builtin, and the claim belongs to the operand
	// being read rather than to the call.
	defer func(was bool) { r.rereadingAQuotedLiteral = was }(r.rereadingAQuotedLiteral)
	for _, a := range args {
		name, value, hasValue, appends := declarationOperand(a)
		r.rereadingAQuotedLiteral = hasValue && r.operandHidesALiteral(name, value, f, false)
		// A **member path** whose base is a reference is a member of the name
		// the reference points at, and the whole operand is about that cell.
		// Here rather than beside attributeFollowsTheReference below, which
		// runs *after* the scope is taken: that rule is keyed on whole names
		// and never saw a dotted one, and redirecting after the shadow would
		// leave the copy standing over `c.b` — a binding nothing reads, with
		// this line's letter recorded on it and given back at the return. See
		// Runner.compoundMemberThroughAReference.
		name = r.compoundMemberThroughAReference(name)
		if r.typeLetterOverAnArrayLiteralRefused(name, f) {
			// Ahead of everything else this operand would do, because the
			// shell that refuses declares nothing: the name is not brought
			// into being, no attribute is recorded, and the script stops.
			return r.status
		}
		if r.unspecified {
			return r.status
		}
		// The letters this operand is declared under, which are the line's
		// plus whatever the *name* makes of them: the export letter asks for
		// `-g` too in one dialect, and that is a question about this name's
		// scope rather than about the line. A copy per operand, because the
		// answer differs between two names on one line — `typeset -x a b`
		// where the function has already made `a` local.
		df := f
		if !df.global && r.exportLetterDeclaresAGlobal(name, df) {
			df.global = true
		}
		if r.unspecified {
			return r.status
		}
		if base, subs, subscripted := r.operandSubscripts(r.inBuiltin, name); subscripted &&
			hasValue && r.letterDropsTheSubscript(value, df) {
			// The array letter is on this very line and the value is
			// parenthesized, and there the subscript decides nothing: not
			// where the value goes, not whether it appends, and not whether
			// the value is re-read. The operand is the base name with the
			// text, which is what the branch below this one already does
			// with it. See interp/subscriptedcompound.go.
			name, appends = base, false
		} else if subscripted {
			sub := subs[len(subs)-1]
			if hasValue {
				// A parenthesized value under a subscript is the one shape
				// the re-read excludes, and one column says so before
				// storing the characters. Ahead of the store because it is
				// about the operand as written. See
				// interp/subscriptedcompound.go.
				r.warnQuotedCompoundAtASubscript(base, sub, value, appends, df)
				// The operand names an element, so the attributes and the
				// scope are about `a` and the value is about `a[1]`.
				// Splitting at the `=` and handing `a[1]` to the variable
				// store made the whole line a no-op at status 0 — see
				// declareelement.go.
				r.declareElement(base, subs[:len(subs)-1], sub, value, appends, df, true)
				if r.unspecified || r.operandGaveUpTheBuiltin() {
					return r.status
				}
				continue
			}
			// A subscripted operand carrying no value declares the *name*
			// as an array and writes no element in one column, reads the
			// brackets first in another, and writes the element in the
			// third: see valuelessSubscriptedOperand, which owns all three
			// and hands back the name and the letters to declare. Measured
			// 2026-09-12, `typeset a[3]` leaves `declare -a a` in bash and
			// `${#a[@]}` is 0 in bash and ksh93 alike, and an array already
			// standing is left as it is. It used to declare a variable
			// literally named `a[3]` — invisible to `${a[3]}` and to
			// `typeset -p a`, at status 0 (#1380).
			n, letters, done := r.valuelessSubscriptedOperand(base, subs, df, true)
			if done {
				if r.unspecified || r.operandGaveUpTheBuiltin() {
					return r.status
				}
				continue
			}
			if r.unspecified {
				return r.status
			}
			name, df = n, letters
		}
		if r.exportRefusesACompound(name, df) {
			// The export letter over a compound, which one shell will not
			// have at all. Ahead of the shadow and of every attribute
			// because nothing about the operand is to happen: the letter
			// does not land, the value is not stored, and the script ends.
			return r.status
		}
		// Read before the attributes are applied, because `-x` on this very
		// declaration would otherwise answer a question asked about the name
		// it shadows. See shadowedExport.
		wasExported := r.isExported(name)
		if r.declarationShadowRefused(name) {
			// Reported, and the next operand still declared: bash's
			// `local y=1 x=5 z=2` over a frozen `x` leaves y and z local.
			if r.unspecified || r.ctl == controlExit {
				// Unless the refusal was fatal, where there is no next
				// operand and no status of the builtin's either: falling
				// through to the `return 1` below overwrote the status the
				// fatal error had set, and dash exits 2 for one of those.
				return r.status
			}
			r.assignFailed = true
			continue
		}
		// Declaring inside a function declares a local, which is unanimous
		// among the three shells that have the name — subject to ksh93's
		// rule about which functions have a scope at all.
		//
		// `-g` is the one thing that changes that: it takes no shadow, so
		// the attributes and the value land on the global cell and `typeset
		// -g x=new` inside a function survives its return even where a
		// `local x` is standing in front of the name.
		//
		// Not taking the shadow is the *whole* of what the letter means, and
		// every other step of the declaration is the same either way.
		// Writing it as an early exit from the loop said otherwise: it made
		// `-g` a second and shorter declaration that dropped every step
		// below the exit. The associative attribute was one of them, and
		// putting `markAssoc` back inside the exit fixed that one letter
		// while leaving the shape that lost it — a valueless `typeset -g n`
		// still brought no name into being, which is the whole of a
		// `typeset -gA a b c` setup line (#989).
		fresh := false
		if !df.global {
			fresh = r.shadowTypeset(name)
		}
		// And a name that is already a **reference** is not what the rest of
		// this operand is about: the attributes, the value and the freeze all
		// belong to what it points at. **After** the shadow, because a shadow
		// this line has just taken has dropped the reference with every other
		// attribute — so a fresh binding is no longer one to follow, which is
		// what leaves `local -i s` over a caller's reference a declaration of
		// the function's own `s`. See interp/namerefattribute.go.
		// The `n` letter under a plus, which is about the **reference** and
		// not about what it points at — so it is answered ahead of the
		// redirect below, which would otherwise carry the rest of the
		// declaration off to the target of a reference this line is removing.
		if df.namerefOff && r.namerefAttributeRemoved(name) {
			continue
		}
		r.refusalSpokenAs = outerSpokenAs
		// Where the attributes and the value part company, which is a
		// reference aimed at an element and nothing else. Read before the
		// redirect below replaces the name, since afterwards the subscript
		// the value belongs to is gone. See Runner.referenceValueTarget.
		valueTarget := r.referenceValueTarget(name, df)
		if target, follows := r.attributeFollowsTheReference(name, df); follows {
			// The refusal the redirect leads to may still be spoken of under
			// the name the script wrote, which is one column's answer and
			// only for a declaration carrying a value. Recorded before the
			// name is replaced, because that is the only place it is still
			// here. See Runner.declarationRefusalNamesTheOperand.
			if hasValue && r.declarationRefusalNamesTheOperand() {
				r.refusalSpokenAs = name
			}
			if r.unspecified {
				return r.status
			}
			name = target
			// Re-read for the redirected name, since the question it answers
			// — what the *shadowed* name carried before this line — is about
			// the cell the attributes are going to land on.
			wasExported = r.isExported(name)
			if !df.global {
				// And the binding is the target's, for the reason
				// aDeclarationThroughAReferenceIsTheTargets gives. Behind
				// the global letter, which is the one spelling that makes no
				// binding at all — `typeset -g v=4` through a reference
				// really does write the shell's own cell, in bash and here
				// alike.
				fresh = r.declarationThroughAReferenceShadowsTheTarget(name, fresh, r.shadowTypeset)
			}
		}
		// Attributes after the shadow, and ahead of the value: `-i` changes
		// what the assignment on the same line *means*, so it cannot wait
		// until the end the way readonly does — `declare -r c=1` sets c and
		// then freezes it, and applying both up front made the declaration
		// refuse its own value and leave the name empty. What the shadow adds
		// is the other end of the same ordering: it has just taken the outer
		// name's attributes off, so these are the local's own and the caller
		// gets its own back on return. Applied before it, they were saved as
		// the outer name's and outlived the call (#1673).
		if r.numericTypeLetterRetypesFrozen(name, df) {
			// The whole operand is exempt and not only its assignment: a
			// valueless `typeset -gi q` over a frozen scalar re-reads the
			// standing text through applyAttributes and declareEmpty, which
			// reach setVarAs the same way the assignment below does — so a name
			// let past one of them met the refusal at the other and printed the
			// sentence anyway. Put back at the top of the next iteration.
			r.retypingFrozen = name
		} else {
			r.retypingFrozen = outerRetyping
			// And where the dialect does *not* exempt it, a value-shaping
			// letter over a frozen name may be refused before any of the
			// declaration happens. Here rather than at the store, because
			// what the panel refuses is the whole operand: the attribute has
			// to not land either, and a gate inside the store leaves a name
			// carrying a letter the shell being imitated never gave it
			// (#2561). See attributeOverFrozenRefused.
			if r.attributeOverFrozenRefused(name, df, hasValue) {
				if r.unspecified || r.ctl == controlExit {
					return r.status
				}
				// Reported, and the next operand still declared — the shape
				// every other per-operand refusal in this loop takes.
				r.assignFailed = true
				continue
			}
		}
		// What this name carries before the letters land, for the operand
		// to put back if it is refused: a refusal bash reports leaves the
		// name exactly as it found it. Read here rather than at the top of
		// the operand because `name` is the redirected one by now — the cell
		// the attributes are about to reach is the cell that has to go back.
		// See declarationtakenback.go.
		held := r.holdTheDeclaration(name)
		r.applyAttributes(name, df)
		if hasValue && !fresh && r.compoundNameHolds(name) && r.declaresAType(name) {
			// An attribute **arriving** over a standing array or table reaches
			// the elements already there, in the column that answers
			// CompoundAttribute that way, and it arrives whether or not the
			// same word carries a value. Only the valueless form asked —
			// declareEmpty reaches rereadStandingValue and a valued one never
			// does — and the column that folds at arrival got its answer from
			// the *store*, which used to re-fold whatever array it was handed.
			// With the store folding only what a write introduces (#3888) that
			// accident is gone, so the question is asked where it belongs.
			//
			// Measured 2026-09-20 with `b=(p q r)` in front of each:
			// `typeset -i b=5` is `5 0 0` on ksh93u+ and `5 q r` on bash
			// 5.3.20, which is each column's *valueless* answer with the
			// value written over element 0 — so it is one rule and not two.
			//
			// Ahead of the value, for the same reason the attributes are:
			// the fold is about the elements the name was holding, and the
			// value replaces one of them afterwards.
			r.compoundMeetingAnAttribute(name)
			if r.unspecified {
				return r.status
			}
		}
		if !df.global {
			r.localExportAttribute(name, df.export)
			if r.unspecified {
				// See biLocal: an unanswered axis refuses the declaration
				// rather than making it one way and saying so.
				return r.status
			}
			r.shadowedExport(name, wasExported)
		}
		// After the shadow, which is what lets the scope put back the outer
		// name's attribute rather than the one this line just gave it.
		r.setHideInScope(name, df)
		if !r.markDeclaredCompound(name, fresh, df, hasValue) {
			// One kind of array declared over the other, and the dialect
			// will not have it: the operand is refused and the next one is
			// still declared, which is the shape every other per-operand
			// refusal in this loop takes.
			if r.unspecified || r.ctl == controlExit {
				return r.status
			}
			r.assignFailed = true
			continue
		}
		if df.readonly && df.readonlyOff {
			if code := r.removeReadonly(name, hasValue); code != 0 {
				return code
			}
			if r.unspecified || r.ctl == controlExit {
				return r.status
			}
		}
		if hasValue && r.inconsistentTypeRefused(name, fresh, df) {
			return r.status
		}
		if r.unspecified {
			return r.status
		}
		if df.nameref && !r.namerefLetterIsDropped(df) {
			// The `n` letter makes the name a **reference**, and what the
			// operand carries is the name it points at rather than a value
			// to store. Ahead of every store below because none of them
			// applies: nothing is written into the parameter tables at all.
			//
			// Behind the shadow and the attributes, which is load-bearing
			// for `local -n`: the reference belongs to this binding, so the
			// caller's own name of the same spelling is untouched and gets
			// itself back on return. See interp/nameref.go.
			// A **frozen name** takes no reference, and a frozen reference
			// is not re-aimed: measured 2026-09-16 on bash 5.3.20, `v=1;
			// w=2; declare -rn r=v` then `declare -n r=w` and a bare
			// `declare -n r` both answer `declare: r: readonly variable` at
			// 1 and leave the reference aimed where it was.
			//
			// Core rather than an axis, because bash is the only column that
			// can make a frozen reference: ksh93u+ answers `typeset -rn`
			// with its usage block at 2, and no other shell on the panel
			// spells a reference at all. Asking a dialect here would be
			// asking a question only one of them can be measured on — see
			// "ask only where it matters".
			//
			// Carried *into* declareNameref rather than tested here, because
			// its order against the other two refusals is measured and the
			// bad target goes first. See there.
			frozen := r.readonly[name] && !df.readonlyOff
			if code := r.declareNameref(complaintName, name, value, df,
				hasValue, frozen, !fresh, held, fresh); code != 0 {
				status = code
				if r.ctl == controlExit {
					return r.status
				}
				r.assignFailed = true
				continue
			}
			// The readonly letter, which is marked at the *bottom* of this
			// loop — see the comment above applyAttributes, where the
			// ordering is explained — and which this branch's `continue`
			// walked past. So the one letter a reference could not carry was
			// the one whose line was below the branch, which is the shape
			// #3106 found for the export attribute a call writes.
			//
			// It is the *reference* that is frozen and not what it points
			// at, measured 2026-09-16 on bash 5.3.20: `v=1; declare -rn r=v`
			// lists `declare -nr r="v"`, refuses a re-aim with `declare: r:
			// readonly variable` at 1 and an `unset -n r` with `unset: r:
			// cannot unset: readonly variable` at 1, while `v=2` goes through
			// at 0 and `declare -p v` keeps `declare -- v="1"`. Ours wrote no
			// `r` in the letters, took the re-aim and took the unset — three
			// silent wrong answers from one missing line.
			if df.readonly && !df.readonlyOff {
				r.markReadonly(name)
			}
			continue
		}
		switch {
		case hasValue && r.arrayLiteralHiddenByQuoting(name, value, appends):
			// The operand's value came out as `( … )`, and in one column a
			// declaration reads that text again as a literal rather than
			// storing it. Ahead of every store below because it is the same
			// operand read as a different thing: the branches there would put
			// the characters in element 0 and the name would look populated
			// while holding one string. See interp/quotedarrayliteral.go.
			//
			// operandGaveUpTheBuiltin and not controlExit alone: a text that
			// reached the re-read and would not parse is refused there and
			// gives up the rest of the line, so the operands behind it are
			// never reached. Reading only the exit ran on to the next one.
			if r.unspecified || r.operandGaveUpTheBuiltin() {
				return r.status
			}
			r.declarationAssignmentExport(name, df.export)
			if r.unspecified {
				return r.status
			}
		case hasValue && df.compoundVar:
			// The value on a `-C` operand is the *name of a variable to copy
			// from* rather than a value to store, and the kind travels with
			// it: `typeset -C d=c` is a compound, `typeset -C d=x` over a
			// scalar `x` is a plain scalar, and a name nobody set leaves the
			// target unset. Ahead of the append branch because that spelling
			// is not one this shell reaches — `typeset -C d+=c` is `d+:
			// invalid variable name` there. See interp/compoundcopy.go.
			r.compoundDeclarationValue(name, value)
			if r.unspecified || r.ctl == controlExit {
				return r.status
			}
		case hasValue && appends:
			// `declare a+=2` joins what the name is holding, through
			// whatever its attributes make of the join — see
			// declarationAppend. The axis that says this operand is read at
			// all was answered in builtinNames.
			if !r.declarationAppend(r.orName(valueTarget, name), value, df.global, fresh) {
				return r.status
			}
			if r.unspecified || r.ctl == controlExit {
				return r.status
			}
			if !df.global {
				r.declarationAssignmentExport(name, df.export)
				if r.unspecified {
					return r.status
				}
			}
		case hasValue && df.global:
			r.setGlobalVar(r.orName(valueTarget, name), value)
			if r.unspecified || r.ctl == controlExit {
				return r.status
			}
		case hasValue:
			r.setVarAs(r.orName(valueTarget, name), value, assignedByDeclaration)
			if r.ctl == controlExit {
				// See biExport: the failure's status is the one that stands.
				return r.status
			}
			r.declarationAssignmentExport(name, df.export)
			if r.unspecified {
				return r.status
			}
		default:
			if r.valuelessDeclarationLists(name, f, fresh) {
				// Said back and nothing more: the value is unchanged, which
				// is why this does not return — the branch below still runs
				// and still decides nothing here.
				r.listStandingDeclaration(name)
			}
			if r.unspecified {
				return r.status
			}
			// `-g` never takes a shadow, so the cell it declares into is
			// never a fresh one — which is exactly the reading that leaves a
			// standing value alone and brings only an absent name into
			// being.
			// No guard on r.unspecified beside this call. The declaration is
			// still refused as a whole — the *next* operand's
			// localExportAttribute check sees the flag and returns 2 with
			// nothing assigned — so a guard here decided nothing and was a
			// line no mutant could kill. What matters is that the refusal
			// reaches the operands after it, and
			// TestAnUnansweredInheritedTypeAxisRefusesTheNamesAfterItToo
			// pins that rather than the guard that appeared to do it.
			if !r.declarationCarriesAnArrayLiteral(name) {
				r.declareEmpty(name, fresh, df.export || df.readonly,
					withoutMatching(df) != (declareFlags{}),
					df.inherit || r.LocalInheritsTheOuterValue(), false)
			}
		}
		if df.readonly && !df.readonlyOff {
			r.markReadonly(name)
		}
	}
	if ended {
		// The operands that were names have been declared; now the script
		// stops, which is what the refusal above asked for.
		return r.endAfterABadName(status)
	}
	if r.assignFailed {
		// See biExport.
		return 1
	}
	// The refused operand's status, where one was refused and the dialect did
	// not end the script over it: bash reports each bad name, declares the
	// well-formed ones beside them and exits 1.
	return status
}

// markDeclaredCompound gives a name the array attribute its declaration named,
// and it is one function because there are three declaration loops and they
// disagreed about it.
//
// `-a` and `-A` name two kinds of array and a name is one kind at a time, so
// the indexed mark goes first and a line carrying both letters is the table —
// which is the order the stores themselves enforce, since markIndexed leaves a
// declared table alone. `+a` and `+A` do nothing rather than removing: two of
// the three shells with the attribute refuse to take it off a name, the same
// shape `+r` already has.
//
// Called *after* the shadow, which is what makes `typeset -A m` inside a
// function declare a local table and the caller's absence come back when it
// returns — and, under `-g`, after the shadow that was deliberately not taken,
// which is what leaves the table on the global cell where the function's
// return cannot reach it.
//
// Being that step, it is also where a fresh cell gives up the compound the
// outer name held — see freshcell.go. That has to happen before the letters
// are read and not after: markIndexed leaves a name that is already an array
// alone, so `local -a arr` over a caller's array marked nothing and kept the
// caller's elements, and a drop afterwards would have taken away the very
// array the letter had just declared.
//
// **`local` had the table's half of this and not the array's** (#1535), and
// that is why it is a function now rather than two `if`s copied into each
// loop. A valueless declaration that never marked the name went on to
// declareEmpty, which sets a *scalar* empty — so `local -a opts` left `opts`
// holding the empty string where `typeset -a opts` on the very next line left
// an array. Nothing said so: `${#opts}` is 0 either way and `${opts[@]}` is
// one empty element against none, so the name a script had declared as an
// array was a string for the rest of the function and every later read of it
// was a plausible answer to the wrong question. The panel is unanimous that
// it is not — `f() { local -a a; echo ${#a[@]}; }` is 0 in bash 5.3.15,
// bash 3.2.57 and zsh 5.9.2, and the ksh93 spelling `typeset -a a` is 0 too,
// where a one-element scalar answers 1.
func (r *Runner) markDeclaredCompound(name string, fresh bool, f declareFlags, hasValue bool) bool {
	// Ahead of the `remove` return, because a fresh cell holds nothing
	// whatever the declaration's letters say: `local +a arr` is still a
	// declaration into a cell this call made.
	// `!declarationInherits` because a declaration that asked for the
	// enclosing binding is not building an empty cell: the compound it would
	// drop is the one it was handed. Measured 2026-09-17 on bash 5.3.20 with
	// `localvar_inherit` on — `declare -a a=(x y); f(){ declare -A a; }` is
	// `cannot convert indexed to associative array` there, which is the
	// check one line below reading the array this would otherwise have taken
	// away. See localinherit.go.
	r.dropTheOuterCompound(name, fresh && !r.declarationInherits(f))
	if f.remove {
		return true
	}
	if r.typeLetterTakesTheCompoundLetter(f) {
		// The declaration wrote both a numeric type letter and a container
		// one, and in this dialect the type wins: the name is a scalar of
		// that type and no compound is declared at all.
		return true
	}
	if r.unspecified {
		return true
	}
	if !r.compoundKindChanged(name, f, hasValue) {
		return false
	}
	if r.unspecified {
		return true
	}
	if f.compoundVar {
		// Ahead of both container letters because it is neither of them, and
		// because it takes whatever the name held: measured 2026-09-13 on
		// ksh93u+, `c=1; typeset -C c` lists `typeset -C c=()` — the scalar
		// is discarded rather than becoming a first member, which is the
		// answer no ScalarUnderACompound policy spells. So this asks no axis:
		// one dialect has the letter and it has one answer.
		r.markCompoundVariable(name)
		return true
	}
	if f.array {
		if r.literalOperands[name] {
			// The indexed letter and an array literal on one command, which
			// is the shape whose subscripts go back to being expressions —
			// see Runner.indexedLetterHere. Recorded here rather than
			// inferred later because the operand assignment cannot see the
			// letters: the parser hands the utility the bare name.
			if r.indexedLetterHere == nil {
				r.indexedLetterHere = map[string]bool{}
			}
			r.indexedLetterHere[name] = true
		}
		v, p := r.declaredCompoundOverAScalar(name, f, r.sem().ScalarUnderAnArrayDeclaration,
			"an array declaration over a name already holding a scalar")
		switch p {
		case ScalarUnderACompoundBecomesTheFirstElement:
			r.markIndexed(name)
			r.storeArray(name, Array{0: Scalar(v)})
		case ScalarUnderACompoundDiscardsIt:
			r.markIndexed(name)
		case ScalarUnderACompoundStaysAScalar:
			// The declaration records nothing and converts nothing: the name
			// is the scalar it was. Not the same as promoting it, even where
			// a scalar answers `${b[0]}` and `${#b[@]}` as an array of one
			// would — a bare `typeset -a` afterwards does not list the name.
		}
	}
	if f.assoc {
		if r.literalOperands[name] {
			// The table letter and a literal on one command — the shape one
			// dialect refuses a bare-word literal in outright, where the
			// same literal written as a plain assignment converts the name.
			// Recorded here for the same reason indexedLetterHere is: the
			// operand assignment is handed the bare name and cannot see the
			// letters. See Runner.tableLetterHere.
			if r.tableLetterHere == nil {
				r.tableLetterHere = map[string]bool{}
			}
			r.tableLetterHere[name] = true
		}
		v, p := r.declaredCompoundOverAScalar(name, f, r.sem().ScalarUnderATableDeclaration,
			"a table declaration over a name already holding a scalar")
		switch p {
		case ScalarUnderACompoundBecomesTheFirstElement:
			r.markAssoc(name)
			// Under the key `0`, which is a key like any other and is what
			// both promoting shells write — `a=1; typeset -A a` lists as
			// `declare -A a=([0]="1" )` in bash and `typeset -A a=([0]=1)` in
			// ksh93. The array base plays no part: a table has no positions.
			r.setAssocElem(name, "0", v)
		case ScalarUnderACompoundDiscardsIt:
			r.markAssoc(name)
		case ScalarUnderACompoundStaysAScalar:
			// No dialect measured answers the table letter this way; the
			// value exists because the two letters share one policy and
			// ksh93 answers them differently.
		}
	}
	return true
}

// compoundKindChanged is a declaration naming one kind of array over a name
// that is already the *other* kind, and it reports whether the declaration may
// go on — see Semantics.TableUnderAnArrayDeclaration and
// ArrayUnderATableDeclaration, where the four answers and the panel are.
//
// **Three shapes and two of them are asked here.** A declaration carrying no
// value of its own is what TableUnderAnArrayDeclaration and
// ArrayUnderATableDeclaration answer; a declaration carrying an **array
// literal** is a second question the panel splits differently, and it is
// TableUnderAnArrayLiteralDeclaration and ArrayUnderATableLiteralDeclaration's
// — ksh93 ends the script over the valueless array letter and converts the
// literal without a word, so no one field could hold both of its answers.
// bash's sentence for the literal form names no builtin, which is the tell
// that the complaint comes from the assignment rather than from the utility,
// and its refusal gives up the rest of the command list where the valueless
// one runs the next command on it (#2287).
//
// The third shape is a declaration carrying a **scalar** value — `typeset -a
// h=x` over a table — and it is deliberately left where it was: the columns
// move again under it (zsh calls it an inconsistent type, ksh93 ends the
// script, bash refuses under the builtin's name), so it is a third field's
// question and not this one's.
//
// A name that is neither compound, or is already the kind being declared,
// asks nothing: `typeset -A m; typeset -A m` is a redeclaration in every
// column and `typeset -a b` over a scalar is ScalarUnderAnArrayDeclaration.
func (r *Runner) compoundKindChanged(name string, f declareFlags, hasValue bool) bool {
	// literalOperands beside hasValue, because an *array literal* operand is
	// not a `name=value` word: `typeset -a h=(x)` reaches the loop as the
	// bare name with the parentheses held aside, so the string would say the
	// operand carried nothing.
	if r.literalOperands[name] {
		return r.compoundKindChangedByALiteral(name, f)
	}
	if hasValue {
		return true
	}
	switch {
	case f.array && r.assocDeclared(name):
		return r.changeCompoundKind(name, r.sem().TableUnderAnArrayDeclaration,
			"an array declaration over a name already declared a table",
			compoundKindRefusal{
				atTheAssignment: r.diag().CannotConvertTableToArray,
				fallback:        "%[2]s: %[1]s: cannot convert associative to indexed array",
			},
			func() { r.tableBecomesAnArray(name) })
	case f.assoc && r.arrayDeclared(name):
		return r.changeCompoundKind(name, r.sem().ArrayUnderATableDeclaration,
			"a table declaration over a name already holding an array",
			compoundKindRefusal{
				atTheAssignment: r.diag().CannotConvertArrayToTable,
				fallback:        "%[2]s: %[1]s: cannot convert indexed to associative array",
			},
			func() { r.arrayBecomesATable(name) })
	}
	return true
}

// compoundKindChangedByALiteral is compoundKindChanged for the shape whose
// value is a declaration's own array literal, which the panel answers
// differently — see TableUnderAnArrayLiteralDeclaration for the rows.
//
// The converting answer **empties** the name in both directions and does not
// carry the old elements across the way the valueless table letter does in one
// column: the literal is an assignment and it replaces what it lands on.
// Measured 2026-09-12, `typeset -a a=(x y); typeset -A a=([k]=v)` lists
// `typeset -A a=([k]=v)` in ksh93 and zsh alike, where the valueless `typeset
// -A a` lists `typeset -A a=([0]=x [1]=y)` in ksh93.
//
// The conversion happens here and the literal is stored by the branch below,
// so this only has to take the old kind away: leaving the table standing is
// what made `typeset -A h; typeset -a h=(x)` store the literal's *word* as a
// key and report success, which is nobody's answer.
func (r *Runner) compoundKindChangedByALiteral(name string, f declareFlags) bool {
	switch {
	case f.array && r.assocDeclared(name):
		return r.changeCompoundKind(name, r.sem().TableUnderAnArrayLiteralDeclaration,
			"an array literal declaration over a name already declared a table",
			compoundKindRefusal{
				atTheAssignment: r.diag().CannotConvertTableToArrayAtTheAssignment,
				fallback:        "%[1]s: cannot convert associative to indexed array",
				// The builtin's own sentence behind the assignment's, for
				// the one shape that writes both — see changeCompoundKind.
				underTheBuiltin: r.diag().CannotConvertTableToArray,
				builtinFallback: "%[2]s: %[1]s: cannot convert associative to indexed array",
			},
			func() { r.compoundKindEmptied(name, false) })
	case f.assoc && r.arrayDeclared(name):
		return r.changeCompoundKind(name, r.sem().ArrayUnderATableLiteralDeclaration,
			"a table literal declaration over a name already holding an array",
			compoundKindRefusal{
				atTheAssignment: r.diag().CannotConvertArrayToTableAtTheAssignment,
				fallback:        "%[1]s: cannot convert indexed to associative array",
				underTheBuiltin: r.diag().CannotConvertArrayToTable,
				builtinFallback: "%[2]s: %[1]s: cannot convert indexed to associative array",
			},
			func() { r.compoundKindEmptied(name, true) })
	}
	return true
}

// compoundKindEmptied takes the compound a name is holding away and marks it
// the other kind, with nothing under it for the literal to land beside.
//
// One function for both directions, unlike tableBecomesAnArray and
// arrayBecomesATable, because the literal form has no elements question to
// part them: neither converting column keeps anything.
func (r *Runner) compoundKindEmptied(name string, toTable bool) {
	delete(r.Arrays, name)
	delete(r.AssocArrays, name)
	if toTable {
		r.markAssoc(name)
		return
	}
	r.markIndexed(name)
}

// compoundKindRefusal is what a refused conversion writes. Two sentences,
// because the *literal* form's refusal is two inside a function and the
// valueless form's is one everywhere — see changeCompoundKind's
// CompoundKindChangeAbandonsTheLine branch, which is the only reader of the
// second pair.
//
// A struct rather than four more parameters, because the two pairs are the
// same shape and a caller passing them positionally is a caller that can swap
// them silently: what it costs is a sentence under the wrong name, which is
// exactly the class of mistake the wordings exist to prevent.
type compoundKindRefusal struct {
	// atTheAssignment is the sentence the *assignment* raises, and fallback
	// is the substrate's own wording for it.
	atTheAssignment, fallback string
	// underTheBuiltin is the sentence the *builtin* raises afterwards, which
	// the valueless form has as its only sentence and the literal form has
	// as its second one. Empty where there is no second sentence to write.
	underTheBuiltin, builtinFallback string
}

// changeCompoundKind resolves one of the two axes and does what it says,
// reporting whether the declaration survives it.
func (r *Runner) changeCompoundKind(name string, p CompoundKindChangePolicy,
	what string, say compoundKindRefusal, convert func(),
) bool {
	wording, fallback := say.atTheAssignment, say.fallback
	switch p {
	case CompoundKindChangeRefused:
		r.diagf("%s\n", Wording(wording, fallback, name, r.inBuiltin))
		// The refusal costs the operand and the builtin's status, and
		// nothing else: measured, `declare -A h; declare -a h; echo A`
		// prints the sentence, then `A`, and `$?` is 1 in between.
		r.assignFailed = true
		return false
	case CompoundKindChangeEndsTheScript:
		r.fatal("%s\n", Wording(wording, fallback, name, r.inBuiltin))
		return false
	case CompoundKindChangeAbandonsTheLine:
		if fn := r.inFunc; fn != "" && say.underTheBuiltin != "" {
			// **Inside a function it is two sentences and the line is not
			// given up.** Measured 2026-09-21, `env -i PATH=/usr/bin:/bin
			// LC_ALL=C` with a scratch HOME, from a script file, on bash
			// 5.3.20, with `declare -a u=(1 2)` already in place:
			//
			//	declare -A u=([k]=v)                one sentence, no word in
			//	                                    front, the line given up
			//	eee() { declare -gA u=([k]=v); }    eee: u: cannot convert …
			//	                                    declare: u: cannot convert …
			//	                                    and `echo` behind the `;`
			//	                                    runs, at 1
			//	the same from a nested call          the **innermost**
			//	                                    function's name
			//	the same inside `( … )`              the same two sentences
			//
			// So the word in front of the assignment's sentence is the
			// running function's — the same slot the builtin's name fills
			// in the valueless form's wording, which is why one template
			// serves both — and the builtin then raises its own refusal
			// behind it. The line is given up at the top level and not in a
			// function, which #4049 recorded the other way round: it read
			// the `echo` behind the `;` as not running in either shell, and
			// it runs in bash.
			r.diagf("%s: %s\n", fn, Wording(wording, fallback, name, r.inBuiltin))
			r.diagf("%s\n", Wording(say.underTheBuiltin, say.builtinFallback, name, r.inBuiltin))
			r.status, r.assignFailed = 1, true
			return false
		}
		// The refusal costs the operand, the status and the rest of the
		// command list — and not the input. Measured 2026-09-12, the same
		// four commands after this one print nothing when they are separated
		// by `;` and all of it when they are separated by newlines, by both
		// invocation routes; see CompoundKindChangeAbandonsTheLine, and
		// refuseReadonly, which reaches controlAbandon for the same reading.
		r.diagf("%s\n", Wording(wording, fallback, name, r.inBuiltin))
		r.status, r.assignFailed = 1, true
		r.abandonTheCommand()
		return false
	case CompoundKindChangeKeepsTheElements, CompoundKindChangeEmptiesTheName:
		convert()
		return true
	}
	r.diagf("%s\n", r.unanswered(what))
	r.status = 2
	r.unspecified = true
	return false
}

// tableBecomesAnArray is the converting answer for `typeset -a` over a table.
//
// Only the emptying column reaches it — no shell measured carries a table's
// values over to an indexed array, there being no order to carry them in —
// so the table goes and an empty array takes its place.
func (r *Runner) tableBecomesAnArray(name string) {
	delete(r.AssocArrays, name)
	r.markIndexed(name)
}

// arrayBecomesATable is the converting answer for `typeset -A` over an array,
// and the two converting columns part over what happens to the elements.
//
// ksh93 carries them across under the keys `0`, `1`, … — measured
// 2026-09-12, `typeset -a a=(x y); typeset -A a` lists `typeset -A a=([0]=x
// [1]=y)` and `${a[0]}` reads `x` afterwards, so they are really there and
// not merely printed. zsh takes them away.
//
// The keys are the *subscripts* the array held and not a run from zero, so a
// sparse array keeps its gaps as the keys it had.
func (r *Runner) arrayBecomesATable(name string) {
	kept := r.sem().ArrayUnderATableDeclaration == CompoundKindChangeKeepsTheElements
	held := r.Arrays[name]
	delete(r.Arrays, name)
	r.markAssoc(name)
	if !kept {
		return
	}
	for idx, v := range held {
		r.setAssocElem(name, itoa(idx), v.scalar())
	}
}

// arrayDeclared reports whether the name is holding an indexed array, which
// is the question assocDeclared asks about the other kind.
func (r *Runner) arrayDeclared(name string) bool {
	if _, produced := r.DynamicArrays[name]; produced {
		return true
	}
	_, ok := r.Arrays[name]
	return ok
}

// typeLetterTakesTheCompoundLetter reports whether a declaration writing both
// a numeric type letter and a container letter declares no container at all —
// see Semantics.NumericAttributeReplacesTheArrayAttribute.
//
// Asked only where both were written, which is the narrowest point the
// answers differ: `typeset -a arr` and `typeset -i n` raise no question
// between them.
//
// Both container letters, because the shell that says yes says it of both:
// `typeset -ia z` and `typeset -iA m` are each `typeset -i …=0` there.
func (r *Runner) typeLetterTakesTheCompoundLetter(f declareFlags) bool {
	if !f.array && !f.assoc {
		return false
	}
	if !f.integer && !f.float {
		return false
	}
	return r.ask(r.sem().NumericAttributeReplacesTheArrayAttribute,
		"a numeric type letter taking the array letter off the same declaration")
}

// declaredCompoundOverAScalar resolves ScalarUnderAnArrayDeclaration or
// ScalarUnderATableDeclaration for one name, and hands back the value the
// promoting answer keeps.
//
// The dialect is asked only where there is a scalar to make something of, and
// that is not a shortcut: an *unset* name is unanimous — `unset b; typeset -a
// b` is an array of no elements in bash, ksh93 and zsh alike — and so is a
// name already holding an array or a table, which every column leaves standing
// exactly as it is. Asking anyway would refuse `typeset -a opts` under a
// dialect that has no answer to a question `typeset -a opts` does not raise.
//
// A *produced* array is not a scalar to be converted either: it has the
// attribute already, and markIndexed leaves it to its producer.
//
// Nor is a value standing in a cell this scope has made *local*. A local
// declaration builds the array cell rather than converting one, and that is
// measured rather than assumed: bash promotes at the top level and through
// `-g` — `b=1; typeset -a b` and `b=1; f() { typeset -ga b; }; f` both list
// `declare -a b=([0]="1")` — and empties under every local spelling, with
// `f() { local b=1; local -a b; }`, `local b=1; declare -a b` and
// `local b=1; typeset -a b` all `declare -a b=()`. The shadow being *fresh*
// is not what decides it: `local b=1` has already taken the copy, and the
// second declaration on the next line empties the cell anyway. Nor is it the
// letter's doing in general — `local b=1; local -i b` keeps the `1` — so what
// is new is the array cell in particular.
//
// No dialect is asked, because none of them disagrees here: zsh discards a
// held scalar wherever it finds one, and ksh93's `typeset` inside a plain
// function declares nothing local at all, so the two shells with an answer of
// their own reach the same place by their own route.
func (r *Runner) declaredCompoundOverAScalar(name string, f declareFlags, p ScalarUnderACompoundPolicy, what string) (string, ScalarUnderACompoundPolicy) {
	if _, ok := r.Arrays[name]; ok {
		return "", ScalarUnderACompoundDiscardsIt
	}
	if r.assocDeclared(name) {
		return "", ScalarUnderACompoundDiscardsIt
	}
	if _, produced := r.DynamicArrays[name]; produced {
		return "", ScalarUnderACompoundDiscardsIt
	}
	if r.isCompoundVariable(name) {
		// ksh93's fourth kind is not a scalar either, and a container letter
		// over one discards it whole: measured 2026-09-13, `c=(a=1); typeset
		// -a c` is `typeset -a c` there with no members left, and `typeset -A
		// c` is `typeset -A c=()`. Ahead of getVar, which is what makes this
		// a line rather than a fall-through — a compound *does* answer a
		// value, its whole tree laid out over several lines, and reading it
		// here would have made that text the new array's first element.
		return "", ScalarUnderACompoundDiscardsIt
	}
	// `!f.global` because a `-g` declaration is not a local one whatever the
	// scope has shadowed. It is deliberately unpinned: the only shape that
	// tells it from `r.localCell(name)` alone is a `-g` letter written where
	// the name is *also* locally shadowed, and that shape already answers
	// wrong here for a reason of its own — `b=1; f() { local b=2; typeset -ga
	// b; }; f` leaves bash's *global* an array of `1` and its local the
	// scalar `2` untouched, where this engine writes the local cell and never
	// reaches the global array at all. A test would have to pin that wrong
	// answer to reach the term. See the mutation note in the pull request.
	// Unless this very declaration asked for the enclosing binding, in which
	// case the cell is *not* being built empty and the scalar standing in it
	// is the one it was handed. Measured 2026-09-17 on bash 5.3.20: with
	// `localvar_inherit` on, `declare -i n=5; f(){ local -a n; declare -p n;
	// }` is `declare -ai n=([0]="5")`, where the same line with the option
	// off is `declare -ai n=()`. So the sentence above — "a local
	// declaration builds the array cell rather than converting one" — holds
	// exactly where the cell is built, and this is the one request that says
	// it is not. See localinherit.go.
	if !f.global && r.localCell(name) && !r.declarationInherits(f) {
		return "", ScalarUnderACompoundDiscardsIt
	}
	// getVar rather than Runner.Vars, for the reason appendedOverAScalar
	// gives: a name the script was *started* with reads back through the
	// environment, and it is a value the name is holding like any other.
	v, held := r.getVar(name)
	if !held {
		return "", ScalarUnderACompoundDiscardsIt
	}
	return v, r.scalarUnderACompound(p, what)
}

// localCell reports whether the innermost scope has already taken the name
// over, so the cell a declaration is about to write is this function's own.
//
// shadow's `fresh` answers a narrower question — whether the copy was taken
// on *this* line — and a name declared twice in one function needs the wider
// one.
func (r *Runner) localCell(name string) bool {
	if len(r.scopes) == 0 {
		return false
	}
	_, saved := r.scopes[len(r.scopes)-1].saved[name]
	return saved
}

// inconsistentTypeRefused reports whether a declaration's plain word is
// refused for landing on a cell that is really holding an array or a keyed
// table, having said so and ended the script — see
// Semantics.ScalarOverACompoundIsAnInconsistentType.
//
// `fresh` is the shadow's answer and is what separates the two shapes that
// look alike: a declaration inside a function writes a cell it has just made,
// which holds nothing whatever the caller left, so `f(){ local b=q; }` over a
// caller's array is taken in every column. What is refused is a declaration
// reaching a cell that is *really* compound, which is what the top level and
// `-g` have in common.
//
// One gate for `typeset`, `local`, `readonly` and `export` alike, because the
// shell that refuses refuses all four in the same words with its own name in
// the location.
//
// **A numeric type letter is not a plain word**, and that is measured rather
// than reasoned: 2026-09-12 on zsh 5.9.2, `typeset -a q=(a); typeset -gi q=4`
// lists `typeset -i q=4` and the `-F` spelling lists `typeset -F
// q=4.0000000000`, where the identical line with `-x`, `-l`, `-r` or `-U` on
// it is `q: inconsistent type for assignment` and so is the letterless
// `typeset -g q=4`. A table converts the same way. So what the refusal is
// about is a declaration that says nothing about the name's *type* landing on
// a compound cell — a letter that names one is a retype and is taken.
//
// This is the same rule the freeze half records from the other side, and it is
// why the two are not one field: see numericTypeLetterRetypesFrozen, whose
// `typeset -ar q=(a); typeset -gi q=4` row reaches this gate first and would
// have been refused here with the freeze already stood down.
func (r *Runner) inconsistentTypeRefused(name string, fresh bool, f declareFlags) bool {
	if fresh || !r.compoundCell(name) {
		return false
	}
	if f.namesANumericType(r) {
		return false
	}
	if !r.ask(r.sem().ScalarOverACompoundIsAnInconsistentType,
		"a plain word declared over a name holding an array") {
		return false
	}
	return r.refuseInconsistentType(name)
}

// typeLetterOverAnArrayLiteralRefused reports whether a declaration is refused
// for naming a *type* — the integer or the float letter — while assigning an
// array literal to the same name, having said so and ended the script. See
// Semantics.TypeLetterAndAnArrayLiteralIsAnInconsistentType.
//
// The letter on this line and not the attribute the name is carrying, which is
// what the measurement says: `typeset -i z; typeset z=(1 2)` is taken in the
// shell that refuses, so a check against r.integer would refuse a line that
// shell writes an array for.
//
// One gate for `typeset`, `local`, `readonly` and `export` alike, because the
// shell that refuses refuses all four in the same words with its own name in
// the location — the same shape inconsistentTypeRefused already has, and they
// share the sentence for the same reason.
func (r *Runner) typeLetterOverAnArrayLiteralRefused(name string, f declareFlags) bool {
	if f.remove || !r.literalOperands[name] {
		return false
	}
	if !f.integer && !f.float {
		return false
	}
	if !r.ask(r.sem().TypeLetterAndAnArrayLiteralIsAnInconsistentType,
		"a type letter on a declaration whose value is an array literal") {
		return false
	}
	return r.refuseInconsistentType(name)
}

// refuseInconsistentType writes the one sentence two questions reach and ends
// the script. Folded out of the two rather than written twice: the wording is
// the dialect's and there is one of it.
func (r *Runner) refuseInconsistentType(name string) bool {
	r.fatal("%s\n", Wording(r.diag().InconsistentType,
		"%s: inconsistent type for assignment", name))
	return true
}

// compoundCell reports whether the name is really holding an array or a keyed
// table right now — not merely carrying the attribute, and not a name a
// declaration has hidden.
func (r *Runner) compoundCell(name string) bool {
	if r.removed[name] {
		return false
	}
	if _, ok := r.Arrays[name]; ok {
		return true
	}
	_, ok := r.AssocArrays[name]
	return ok
}

// exportLetterDeclaresAGlobal reports whether the `x` letter on this
// declaration also asks for `-g` — see
// Semantics.ExportLetterDeclaresAGlobal.
//
// Asked at the narrowest point the two shells part company, which is three
// conditions deep and each of them was measured:
//
//   - Inside a function. At the top level a declaration reaches the global
//     cell under either answer, so there is nothing to choose between.
//   - Where the letter was written. `typeset -i v=1` and the bare word take
//     the scope every shell gives them.
//   - Where the name is not *already* local to this scope. The shell that
//     answers yes exempts one it has been told about: `f(){ local m=1;
//     typeset -x m; }` leaves the caller's m alone there, so the letter says
//     where a declaration lands and not what it does to a name already here.
//
// `local` never comes through here, which is the fourth condition and is
// structural rather than a test: biLocal has its own loop, and the shell that
// answers yes exempts that word by name.
func (r *Runner) exportLetterDeclaresAGlobal(name string, f declareFlags) bool {
	if !f.export || len(r.scopes) == 0 {
		return false
	}
	if r.localCell(name) {
		return false
	}
	return r.ask(r.sem().ExportLetterDeclaresAGlobal,
		"the export letter on a declaration reaching past the function")
}

// integerComesOff is whether this declaration's letters ask for the integer
// attribute to come off.
//
// The sign of the last `i` *letter* rather than of the last option word, which
// is the distinction readonlyOff already draws and for the same reason:
// `typeset -i +x n` is a declaration of an integer that is not exported, and
// reading the word's sign made it a request to stop being one.
//
// A `-i` before a `+i` is one dialect's exception and is asked of the dialect.
// See Semantics.EarlierIntegerLetterKeepsTheAttribute, where the panel is.
func (f declareFlags) integerComesOff(r *Runner) bool {
	if f.signDecided {
		return f.remove
	}
	first, last := byte(0), byte(0)
	for i := 0; i < len(f.letters) && i < len(f.letterSigns); i++ {
		if f.letters[i] != 'i' {
			continue
		}
		if first == 0 {
			first = f.letterSigns[i]
		}
		last = f.letterSigns[i]
	}
	if last == 0 {
		// The letter rode in on a name rather than on a word — `integer +x n`
		// — so there is no `i` to read a sign from and the word's is all
		// there is.
		return f.remove
	}
	return last == '+'
}

// applyAttributes records what a name has been declared to be.
func (r *Runner) applyAttributes(name string, f declareFlags) {
	if f.integer {
		if r.integer == nil {
			r.integer = map[string]bool{}
		}
		if f.integerComesOff(r) && !f.integerForced {
			delete(r.integer, name)
			// The base goes with the attribute — measured, `typeset +i j`
			// leaves the text the name is holding alone and a *later* `j=3`
			// is plain, so what the plus form takes off is the rendering of
			// what comes next and not what is already there.
			delete(r.integerBase, name)
		} else {
			r.integer[name] = true
			// See the float branch below: the later declaration speaks, and
			// `typeset -F 3 x=1.5; typeset -i x` reads `1`.
			delete(r.floatPrecision, name)
			r.markFloatExponent(name, false)
			r.numericLetterReplacesTheCase(name)
			switch {
			case f.baseNamed && f.base == 10 && r.integerBaseTenIsNone():
				// Ten written down where ten is the letter's default
				// records nothing at all, so a name that had a base loses
				// it: measured, `typeset -i16 h=255; typeset -i10 h` reads
				// `255` in ksh93 and lists back without a base word.
				delete(r.integerBase, name)
				r.rerenderInTheNewBase(name)
			case f.baseNamed && f.base == 0:
				// Zero is not a base and is not "no base" either: it leaves
				// the one the name already has exactly where it is.
				// Measured 2026-09-12 on ksh93u+, `typeset -i16 e=255;
				// typeset -i0 e` still reads `16#ff` — where the same line
				// with `-i1` reads `255`, which is the row that says the two
				// are not one rule.
			case f.baseNamed && f.base < 2:
				// Anything else below two records nothing, so a name that
				// had a base loses it and one that had none never gets one:
				// `typeset -i1 b=5` lists as `typeset -i b=5` and
				// `typeset -i-5 j=100` does too. Same shape as the ten
				// branch above, and reached for the same reason — this is
				// the base that is not written down.
				delete(r.integerBase, name)
				r.rerenderInTheNewBase(name)
			case f.baseNamed:
				if r.integerBase == nil {
					r.integerBase = map[string]int{}
				}
				r.integerBase[name] = f.base
				// The base applies to what the name already holds, not only
				// to what is written next: measured, `typeset -i i=5;
				// typeset -i16 i` reads back `16#5`, and `typeset -i16
				// h=255; typeset -i8 h` re-renders to `8#377`. A declaration
				// that also assigns stores its value after this runs, so the
				// one line covers both spellings — the same shape `-U` has
				// just below.
				r.rerenderInTheNewBase(name)
			case r.integerBase[name] != 0 && r.integerBaseTenIsNone():
				// The letter with no base is the letter naming ten, where
				// ten is what it names by default — so it takes the base off
				// a name that had one and re-renders what is standing there:
				// measured, `typeset -i16 a=255; typeset -i a` is `255` in
				// ksh93 and `16#FF` in zsh, and `integer a` is the same
				// declaration under another word. Another letter is not this
				// question: `typeset -x` over a based name leaves the base
				// alone in both, which is why nothing is asked unless the
				// integer letter itself was written.
				delete(r.integerBase, name)
				r.rerenderInTheNewBase(name)
			}
		}
	}
	if f.widthLetter != 0 {
		if f.remove {
			// `typeset +L e` takes the attribute off and reveals the text
			// the name was holding all along — measured, `typeset -L 3
			// e=abcd; typeset +L e` reads `abcd` and lists as a plain
			// `typeset e=abcd`. Nothing is written back, exactly as `+F`
			// and `+i` write nothing back, because this shell never stored
			// the padded text in the first place.
			//
			// A zero fill is the one part of a presentation that does come
			// back off, in the column that stored one — see
			// widthZerosUnwound, where the measurement is.
			r.widthZerosUnwound(name)
			delete(r.fieldWidth, name)
		} else {
			// The attribute being replaced presented the text the name is
			// holding, and the one arriving has to read the text rather than
			// the presentation — see widthUnwound, where the measurements
			// are.
			r.widthUnwound(name)
			if r.fieldWidth == nil {
				r.fieldWidth = map[string]fieldWidth{}
			}
			w := fieldWidth{
				letter:   f.widthLetter,
				zeroFill: f.widthZeroFill,
				width:    r.fieldWidth[name].width,
			}
			if f.widthNamed {
				// A number written down replaces whatever the name had,
				// including a width it had learned: measured, `typeset -L 3
				// f=abcd; typeset -R 4 f` lists as `typeset -R4 f=abcd`. A
				// written zero names no width and leaves the learning to
				// the value, which is the branch widthLearned takes.
				w.width = f.width
			}
			r.fieldWidth[name] = w
			// The three letters and the float one are not exclusive in the
			// panel — `typeset -lL 4 d=ABCD` is `abcd`, both letters acting
			// — so nothing is taken off here. The integer letter is the one
			// that swallows the number instead of sharing it, and the parse
			// has already settled that: `typeset -iL 3 k=12345` reads the 3
			// as a *base* in zsh and lists `typeset -i3`.
		}
	}
	if f.float {
		if f.remove {
			// `typeset +F x` takes the attribute off and leaves the text the
			// name is holding alone: measured, `typeset -F 3 x=1.5; typeset
			// +F x` reads `1.500` still and lists as a plain `typeset
			// x=1.500`. What the plus form takes off is the rendering of
			// what comes next, the same as `+i`.
			delete(r.floatPrecision, name)
			// And the letter beside it, which is the format rather than the
			// places: `typeset +E x` leaves a plain name and not one that
			// renders as `F`.
			r.markFloatExponent(name, false)
		} else {
			if r.floatPrecision == nil {
				r.floatPrecision = map[string]int{}
			}
			// Which of the two letters declared it. Written whichever way
			// round, so `typeset -E 3 x; typeset -F 3 x` really is the plain
			// rendering afterwards and the attribute is not half of each.
			r.markFloatExponent(name, f.floatExponent)
			// A bare `-F` or `-E` over a name that already has a precision
			// keeps it in one shell and resets it to the letter's default in
			// the other — measured, `typeset -F 3 x=1.5; typeset -F x` is
			// `1.500` in zsh 5.9.2 and `typeset -E 3 a=1.23456789; typeset
			// -E a` reads `1.23` and then `1.23456789` in ksh93u+. That was
			// a note here while zsh was the only dialect given the attribute
			// and is Semantics.BareFloatLetterResetsThePrecision since
			// ksh93 gained the `E` letter (#2559).
			if f.precisionNamed || r.floatPrecision[name] == 0 ||
				r.ask(r.sem().BareFloatLetterResetsThePrecision,
					"a bare float letter resetting a precision the name already had") {
				r.floatPrecision[name] = f.precision
			}
			if r.unspecified {
				return
			}
			// The two attributes cannot both stand and the later
			// *declaration* speaks: measured, `typeset -i x=5; typeset -F 3
			// x` reads `5.000` and the reverse reads `1`. Within one word it
			// is the earlier letter, which the parse settled.
			delete(r.integer, name)
			delete(r.integerBase, name)
			// The precision applies to what the name already holds and not
			// only to what is written next — `v=1.5; typeset -F 3 v` is
			// `1.500` and `typeset -i16 v=255; typeset -F 3 v` is `255.000`
			// — but nothing is written back here. That is
			// rereadStandingValue's job, which declareEmpty reaches once
			// these attributes are recorded, and going through it rather
			// than around it is what makes the re-read meet
			// AttributeRereadsTheValueItFinds and evaluate through the
			// arithmetic. A re-render of its own read the standing text with
			// strconv, which left `16#FF` exactly where it was.
			r.numericLetterReplacesTheCase(name)
		}
	}
	if f.export {
		// A plus word takes the attribute off — unless the *name the command
		// was called by* is what asked for it, which no plus on some other
		// letter may cancel. The same reading `integer` takes for its own
		// type letter: measured 2026-09-12, `export +i q=4` still exports in
		// the shell whose `export` reads the declaration letters, and lists
		// as `export q=4`. See exportForced.
		r.declarationExports(name, !f.remove || f.exportForced)
	}
	// The case attributes fold at assignment here, which is what bash and
	// ksh93 do. zsh stores the raw text and folds on *expansion* — every
	// read agrees with the other two, and only its `typeset -p` betrays the
	// difference by listing the raw value. That listing nuance is
	// deliberately not modeled; the fold every script observes is.
	// A numeric letter on the *same* declaration does not take the value out
	// of the case attribute's hands — `typeset -li i=3+4` is `7` and
	// `typeset -li v=AB` is `0` on ksh93u+, bash 5.3.15 and zsh 5.9.2 alike,
	// in either order of the two letters — but it does not take the
	// *attribute* off either, which is what this used to read it as.
	// Measured 2026-09-12, `typeset -li v=4` lists `typeset -il v=4` in zsh,
	// `declare -il v="4"` in bash and `typeset -l -i v=4` in ksh93, where
	// this shell listed a bare `typeset -i v=4` in all three (#2541). So the
	// letter is recorded and only the two questions below are asked of the
	// dialect.
	//
	// Not the same question as CaseAttributeReplacesTheNumericAttribute,
	// which is about a *later* declaration and where the three disagree:
	// `typeset -i i; typeset -l i` leaves `3+4` in ksh93 and `7` in the
	// other two. One command is not two, and this shell was reading the
	// axis for both — so `integer='typeset -li'`, this shell's own alias
	// for its own builtin, stopped evaluating anything (#2345). That is why
	// caseLetterReplacesTheNumeric is still asked only where no numeric
	// letter shares the line.
	numeric := f.integer || f.float
	canceled := r.caseLettersCancel(name, f)
	if f.lower && !canceled {
		if r.lowered == nil {
			r.lowered = map[string]bool{}
		}
		if f.remove {
			delete(r.lowered, name)
		} else {
			r.lowered[name] = true
			// The two case attributes cannot both stand: the later one
			// speaks, which is what both shells measured do.
			delete(r.uppered, name)
			if !numeric {
				r.caseLetterReplacesTheNumeric(name)
			}
		}
	}
	upperRecords := f.upper && !canceled
	if upperRecords && numeric {
		// One shell writes nothing down for the upper letter beside a
		// numeric type letter, where it writes the lower one down. Asked
		// only here, so a line with no numeric letter on it meets no
		// question — see upperLetterRecordsNothing.
		upperRecords = !r.upperLetterRecordsNothing()
	}
	if upperRecords {
		if r.uppered == nil {
			r.uppered = map[string]bool{}
		}
		if f.remove {
			delete(r.uppered, name)
		} else {
			r.uppered[name] = true
			delete(r.lowered, name)
			if !numeric {
				r.caseLetterReplacesTheNumeric(name)
			}
		}
	}
	if f.unique {
		if r.unique == nil {
			r.unique = map[string]bool{}
		}
		if f.remove {
			// `+U` drops the attribute and leaves the elements that are
			// there alone: measured, `typeset -U c=(1 2 3 2)` is `1 2 3`,
			// and `typeset +U c; c+=(1)` is `1 2 3 1` — the duplicate the
			// append brought stands.
			delete(r.unique, name)
		} else {
			r.unique[name] = true
			// The attribute applies to what the name already holds, not
			// only to what is written next: measured, `b=(1 1 2)` followed
			// by `typeset -U b` reads back `1 2`. A declaration that also
			// assigns has already stored its value by the time this runs,
			// so this one line covers both spellings.
			if a, ok := r.Arrays[name]; ok {
				r.storeArray(name, a)
			}
		}
	}
	if f.traced {
		if r.traced == nil {
			r.traced = map[string]bool{}
		}
		if f.remove {
			// `+t` takes the letter off and leaves everything else standing:
			// measured on all three shells with the attribute, `typeset -t
			// T=1` then `typeset +t T` lists the plain form with the value
			// still there.
			delete(r.traced, name)
		} else {
			r.traced[name] = true
		}
	}
	if f.hidden {
		if r.hidden == nil {
			r.hidden = map[string]bool{}
		}
		if f.remove {
			// `+H` takes the value back out of hiding and leaves everything
			// else alone: measured, `typeset -iH n=5` lists as `typeset -i n`
			// and `typeset +H n` lists as `typeset -i n=5`.
			delete(r.hidden, name)
		} else {
			r.hidden[name] = true
		}
	}
}

// namesANumericType reports whether this declaration actually *adds* the
// integer or the float attribute, as against writing the letter under a plus
// or not writing one at all.
//
// Two readers and they ask it for opposite reasons — inconsistentTypeRefused,
// where naming a type is what makes a declaration something other than a plain
// word over a compound cell, and numericTypeLetterRetypesFrozen, where it is
// the first half of a retype. One function so the two cannot come apart, which
// is the failure a second copy of `f.integer && !f.integerComesOff(r)` invites.
func (f declareFlags) namesANumericType(r *Runner) bool {
	switch {
	case f.integer:
		return !f.integerComesOff(r)
	case f.float:
		return !f.remove
	}
	return false
}

// numericTypeLetterRetypesFrozen reports whether this operand is the second
// shape a frozen name still takes: a declaration whose integer or float letter
// gives the name a numeric type it does not already hold.
//
// The letter half of what frozenScalarRetyped answers for an array literal,
// and its own question rather than the same one reached twice — the two have
// different domains over the same frozen name. A frozen *array* is retyped by
// the letter and is not retyped by the literal, measured 2026-09-12 on zsh
// 5.9.2: `typeset -ar q=(a); typeset -gi q=4` lists `typeset -ir q=4` where
// `readonly q=(a); typeset -g q=(b)` is `read-only variable: q`. See
// Semantics.NumericTypeLetterRetypesAFrozenName for the whole panel.
//
// Four guards before the dialect is reached, and each is a measured
// discriminator:
//
//   - the name is frozen, or there was nothing to be exempt from;
//   - a numeric type letter is actually being *added*, so `typeset +i` and a
//     line with no type letter at all keep the refusal they had;
//   - the type is one the name does not already hold, which is the *retype*
//     half — `typeset -ir q=1; typeset -gi q=4` is refused, and so is the same
//     line with a base written on it, because a base is not a type;
//   - the name is not a module's absent parameter, whose kind this shell does
//     not know. The same guard frozenScalarRetyped carries, for the same
//     measured reason.
//
// The integer and float attributes are the two this engine records, so they
// are the two asked about. `-E` is the float attribute wearing a second
// rendering — measured, `typeset -Fr q=1; typeset -gE q=4` is refused in zsh,
// which says the two letters name one type — and it is not implemented here
// yet, so no third branch would have anything to read.
// namesAValueShapingAttribute reports whether this declaration writes a letter
// that says what the name's values *are* or what kind of cell holds them.
//
// The same list clearTypeAttributes takes off — the integer and float letters,
// the two case letters and a field width — plus the two array letters, which
// say the kind rather than the type. Every one of them changes what a read of
// the name produces or what a write to it means, which is the distinction the
// panel draws: `-x`, `-t`, `-r` and `-g` are permissions and placement and are
// deliberately outside it. See Semantics.AttributeOverAFrozenNameIsRefused for
// where the line was measured.
//
// The **sign is not read**, and that is measured rather than convenient:
// bash 5.3 refuses `typeset +i q` over a frozen `-i` name exactly as it
// refuses `typeset -i q`, so what the refusal is about is the letter being
// written at all. A helper that asked namesANumericType here would answer No
// for the plus form and let it through, which is the second-helper shape this
// tree keeps rediscovering.
func (f declareFlags) namesAValueShapingAttribute() bool {
	return f.integer || f.float || f.widthLetter != 0 ||
		f.lower || f.upper || f.array || f.assoc
}

// namesAValueBearingType is the narrower list one dialect refuses over a
// frozen name that holds **nothing**: the letters that say what a value of
// this name *is*, so that declaring one over a name with no value would have
// to invent the value — an integer zero, a float, a padded field, a compound
// with no members.
//
// Three of namesAValueShapingAttribute's letters are deliberately not here,
// and they are the discriminator rather than an omission: the two case
// letters and the two array letters are taken over the same frozen name in
// that shell, with a value and without. A case letter folds a value that is
// not there yet and an array letter makes the name a container rather than
// giving it one. The compound letter is here because it was measured here.
// See Semantics.TypeLetterOverAFrozenNameWithNoValueIsRefused for the table.
func (f declareFlags) namesAValueBearingType() bool {
	return f.integer || f.float || f.widthLetter != 0 || f.compoundVar
}

// namesAKeyedCell is the other narrow list that one dialect refuses over a
// frozen name, and it is the complement of the one above in both halves: the
// letters that make the name a cell addressed **by name** rather than by
// number, refused where the frozen name *holds* something.
//
// The indexed array letter is deliberately not here and is the
// discriminator: `c=1; readonly c; typeset -a c` is taken in that shell,
// where the two beside it are refused. An indexed array can keep a scalar as
// element zero and a keyed one cannot, which is the shape the measurement
// draws. See Semantics.KeyedLetterOverAFrozenNameHoldingAValueIsRefused.
func (f declareFlags) namesAKeyedCell() bool {
	return f.assoc || f.compoundVar
}

// attributeOverFrozenRefused reports whether this operand is refused for
// naming an attribute over a frozen name, having said so.
//
// It stands in front of applyAttributes and not inside the store, which is the
// ordering #1673 deliberately chose against for the *value* — `declare -r c=1`
// has to set `c` and then freeze it, so applying the attributes up front made
// a declaration refuse its own value. This is a third branch rather than a
// move of that one: the name is frozen *before* this line runs, so nothing on
// this line can be the thing that froze it.
//
// Two guards before the dialect is reached, and both are measured:
//
//   - the name is frozen, or there is nothing to refuse;
//   - a value-shaping letter is written, since the panel takes `-x`, `-t`,
//     `-r`, `-g` and a bare word over a frozen name in every column.
//
// numericTypeLetterRetypesFrozen is asked by the caller first and takes its
// own names out, so a dialect that exempts a retype never arrives here.
//
// **Three axes and not one**, because one dialect's answer is not a constant:
// it turns on what the frozen name holds, and it turns the *opposite* way for
// two different sets of letters.
//
//   - The wide one is asked first and is what bash answers yes to. It covers
//     every value-shaping letter over any frozen name, assigning operand
//     included, which is the shape #2561 is about.
//   - Where that says no, a frozen name **holding nothing** still refuses the
//     letters a value has to be built for — see
//     Semantics.TypeLetterOverAFrozenNameWithNoValueIsRefused (#3937).
//   - And a frozen name **holding something** still refuses the letters whose
//     cell cannot hold what is there — see
//     Semantics.KeyedLetterOverAFrozenNameHoldingAValueIsRefused (#3965).
//
// The last two are complements and cannot both fire, which is what the one
// `holds` below says out loud. Both are worded as an attribute's refusal
// rather than as an assignment's, which is what the form says; the wide one
// keeps the declaration's wording it was measured with.
//
// Both narrow ones are the **valueless operand** only, and `assigns` is what
// says so. An operand carrying a value is decided at the store: measured
// 2026-09-20, `readonly c; typeset -i c=4` is the plain assignment refusal
// there, and `readonly c; typeset -C c=(a=1)` is **taken**, which is the rule
// #3915 records. Refusing here would take that one back.
func (r *Runner) attributeOverFrozenRefused(name string, f declareFlags, assigns bool) bool {
	if !r.readonly[name] {
		return false
	}
	if f.namesAValueShapingAttribute() {
		wide := r.sem().AttributeOverAFrozenNameIsRefused
		if r.ask(wide, "an attribute letter over a frozen name being refused") {
			if r.unspecified {
				return true
			}
			return r.refuseReadonly(name, assignedByDeclaration)
		}
		if wide == Unspecified {
			// It has just reported itself unanswered, and asking the
			// narrower question would complain twice about one declaration.
			return false
		}
	}
	if assigns || r.literalOperands[name] {
		// The operand carries a value, so the store decides it. Both halves
		// of that question, because an array or compound literal is not a
		// `name=value` word: `typeset -C c=(a=1)` reaches the loop as the
		// bare name with the parentheses held aside, so `assigns` alone
		// would read it as carrying nothing — the same pair
		// compoundKindChanged reads, for the same reason.
		return false
	}
	var axis Answer
	var why string
	switch holds := r.nameHoldsSomething(name); {
	case !holds && f.namesAValueBearingType():
		axis = r.sem().TypeLetterOverAFrozenNameWithNoValueIsRefused
		why = "a type letter over a frozen name that holds nothing being refused"
	case holds && f.namesAKeyedCell():
		axis = r.sem().KeyedLetterOverAFrozenNameHoldingAValueIsRefused
		why = "a keyed-cell letter over a frozen name that holds a value being refused"
	default:
		return false
	}
	if !r.ask(axis, why) {
		return false
	}
	if r.unspecified {
		return true
	}
	return r.refuseReadonly(name, attributeRatherThanAValue)
}

func (r *Runner) numericTypeLetterRetypesFrozen(name string, f declareFlags) bool {
	if !r.readonly[name] {
		return false
	}
	if !f.namesANumericType(r) {
		return false
	}
	switch {
	case f.integer && r.integer[name]:
		// Already an integer, so nothing is being retyped and the ordinary
		// refusal stands.
		return false
	case f.float:
		if _, isFloat := r.floatPrecision[name]; isFloat {
			return false
		}
	}
	if r.AbsentParameter(name) {
		return false
	}
	return r.ask(r.sem().NumericTypeLetterRetypesAFrozenName,
		"a numeric type letter retyping a frozen name")
}

// caseLettersCancel reports whether this declaration wrote **both** case
// letters under a minus, which one reading makes a declaration that records
// neither and takes off whichever the name was already carrying — see
// Semantics.TwoCaseLettersOnOneDeclarationCancel.
//
// The removal happens here rather than in the two branches below, because the
// branches are what the cancel switches *off* and a rule that only declined to
// add would leave a standing attribute where two of the three shells take it
// away.
//
// The sign is read per letter and not off the word, which is measured:
// `typeset +l -u z=Ab` lists `typeset -u z=Ab` in zsh and `declare -u z="AB"`
// in bash, so a letter written under a plus is not one of the two that cancel.
// Same reading `readonlyOff` already takes for the `r` letter.
func (r *Runner) caseLettersCancel(name string, f declareFlags) bool {
	if !f.lower || !f.upper {
		return false
	}
	lowerPlus, lowerWritten := f.lastSign('l')
	upperPlus, upperWritten := f.lastSign('u')
	if !lowerWritten || !upperWritten || lowerPlus || upperPlus {
		return false
	}
	if !r.ask(r.sem().TwoCaseLettersOnOneDeclarationCancel,
		"both case letters written on one declaration") {
		return false
	}
	delete(r.lowered, name)
	delete(r.uppered, name)
	return true
}

// upperLetterRecordsNothing reports whether `-u` beside a numeric type letter
// on the same declaration records no attribute, where `-l` beside the same
// letter records one — see
// Semantics.UpperCaseLetterBesideANumericTypeLetterRecordsNothing.
//
// A name is not passed and none is needed: the answer is about the pair of
// letters and not about the cell, and the shell that drops the letter drops it
// for every name.
func (r *Runner) upperLetterRecordsNothing() bool {
	return r.ask(r.sem().UpperCaseLetterBesideANumericTypeLetterRecordsNothing,
		"the upper-case letter written beside a numeric type letter")
}

// numericLetterReplacesTheCase takes a case attribute off a name the integer
// or float letter has just been given, where that letter *replaces* what says
// what the name's values are rather than joining it — see
// Semantics.NumericAttributeReplacesTheCaseAttribute.
//
// Asked only where the name is carrying one, which is the narrowest point the
// two answers differ: a plain `typeset -i n` has no case attribute to lose and
// must meet no question at all.
//
// One function for both letters, called from the integer branch and from the
// float branch, because the two are one family and a second copy is where they
// would come apart.
func (r *Runner) numericLetterReplacesTheCase(name string) {
	if !r.lowered[name] && !r.uppered[name] {
		return
	}
	if !r.ask(r.sem().NumericAttributeReplacesTheCaseAttribute,
		"a numeric letter taking a case attribute off the name") {
		return
	}
	delete(r.lowered, name)
	delete(r.uppered, name)
}

// caseLetterReplacesTheNumeric is the other direction, and its own axis
// because one shell answers the two differently — see
// Semantics.CaseAttributeReplacesTheNumericAttribute.
//
// The base goes with the integer letter and the precision with the float one,
// for the reason `+i` and `+F` already have: what a type letter leaves behind
// is the text the name is holding, and only the rendering of what comes next
// is taken away.
func (r *Runner) caseLetterReplacesTheNumeric(name string) {
	// The float attribute is a precision, and zero is one a script may write
	// — `typeset -F 0 x` — so the presence of the key is the question and not
	// the number under it, the same distinction precisionNamed draws.
	_, float := r.floatPrecision[name]
	if !r.integer[name] && !float {
		return
	}
	if !r.ask(r.sem().CaseAttributeReplacesTheNumericAttribute,
		"a case letter taking the integer or float attribute off the name") {
		return
	}
	delete(r.integer, name)
	delete(r.integerBase, name)
	delete(r.floatPrecision, name)
}

// setGlobalVar assigns to a name's global cell, past any local shadowing it.
//
// In this engine there is one table and a stack of saved outer values, so
// the global cell is either the table itself — no scope saved the name — or
// the copy held by the *oldest* scope that did, which is the value the last
// return will put back. Whether `-g` really reaches past a local is the one
// disagreement here, and it is asked only where a local stands in the way —
// with none, both shells that spell the letter write the global.
func (r *Runner) setGlobalVar(name, value string) {
	for _, sc := range r.scopes {
		if _, saved := sc.saved[name]; !saved {
			continue
		}
		if _, _, under := r.shellsOwnCellUnderACallPrefix(name); under {
			// The shell's own cell is an enclosing call's assignment prefix
			// entry, which is outside this scope — the frame was pushed
			// before it — and it is what the *visible* cell holds for the
			// length of this declaration. So the write goes there and this
			// scope's saved copy is a temporary like any other, restored
			// over the global when the call it belongs to returns. See
			// interp/globalunderacallprefix.go, which asked the axis for
			// this name before the line began.
			break
		}
		if !r.ask(r.sem().DeclareGlobalReachesPastALocal, "`declare -g` writing past a local of the same name") {
			if r.unspecified {
				return
			}
			// The visible cell — the local — which is what a plain
			// assignment would have written.
			break
		}
		sc.saved[name] = value
		sc.existed[name] = true
		if sc.removedBefore != nil {
			sc.removedBefore[name] = false
		}
		return
	}
	r.setVarAs(name, value, assignedByDeclaration)
}

// declarationListing answers a declaration that named nothing to declare,
// which is a listing in both shells that have the word — and *which* listing
// is decided by exactly the letters that decide it under `-m`.
//
// So this is that ladder over the whole table instead of over a pattern's
// matches, walked with the same helpers rather than with a second copy of
// them: see declarationNameListing and declarationFilteredNameListing, which
// declareMatching reaches through matchedListing and matchedNameListing. The
// `-m` half was built first (#1696) and the letters were read only there, so
// every one of these rows wrote nothing at all until now — `typeset +` and
// `typeset +x` were silent where the same letters with a `*` pattern
// answered (#1576).
//
// Measured 2026-09-10 on zsh 5.9.2, `-f` with no startup files and a scrubbed
// environment, against a table holding a scalar, an export, an integer, a
// read-only, an array, an association, a unique array and a case-folded pair:
//
//	typeset      every name, its attribute words and its value
//	typeset -    the same: a sign carrying no letters adds nothing
//	typeset +    every name and its attribute words, and no value
//	typeset +x   the exported names alone, and no attribute words
//	typeset +xi  the exported names *and* the integer ones — either letter
//	typeset +g   every name and its value again
//
// `+g` is the row that says the fallback is the bare listing rather than
// silence: `-g` says where a declaration lands rather than what a name
// carries, so there is nothing for it to filter on and the letter drops out.
// `+gx` writes what `+x` writes, which is the same fact from the other side.
//
// The minus sign carrying an attribute letter is the *valued* half of the
// same ladder, measured on the same day and in the same shells:
//
//	typeset -x   the exported names, each with its value
//	typeset -a   every array, each with its elements
//	typeset -ax  what the two letters together select, which is the one
//	             thing the panel does not agree about — see
//	             Semantics.DeclarationListingFilter
//
// It writes nothing at all until #1868: the letter was read as a filter and
// then given up on, so `declare -x`, the common way to dump an environment as
// declarations, came back empty at status 0.
//
// The bool is whether this is a shape it answers at all. A line whose letters
// ask for something else — a tie, a print — is not a listing and is left to
// the caller.
func (r *Runner) declarationListing(f declareFlags) (int, bool) {
	if withoutListingLetters(f) != (declareFlags{}) {
		return 0, false
	}
	if withoutMatching(f) == (declareFlags{}) {
		// Nothing was written but the sign. Under a minus that is the bare
		// listing — the truly bare word reaches here too, which is the one
		// row that was already right — and under a plus it is the same walk
		// with the values left off.
		if f.remove {
			return r.declarationNameListing(r.declarableNames()), true
		}
		return r.bareDeclarationListing(), true
	}
	if !f.attributeLetterWritten() && !f.inert {
		// Nothing was written that a name could *carry*: `-g` and `+g` say
		// where a declaration lands rather than what a name is, so the
		// letter drops out and the whole table is the answer, values and
		// all. Measured on both signs, and `+gx` writing what `+x` writes
		// is the same fact from the other side.
		return r.bareDeclarationListing(), true
	}
	keep, answered := r.attributeFilter(f)
	if !answered {
		return r.status, true
	}
	if keep == nil {
		// A letter this dialect spells and this engine records nothing for
		// — see Semantics.DeclareOptionsWithoutEffect. It is still an
		// attribute to select on and no name here carries it, so the
		// listing is empty rather than whole: measured 2026-09-10, both
		// `typeset -z` and `typeset +z` are 0 with not one byte, where
		// `typeset -g` on the same table writes every name.
		return 0, true
	}
	// The produced parameters are candidates here exactly as they are under
	// the unfiltered `-p`, and the letter is what decides which of them
	// stay. Measured 2026-09-20, one letter per script file under `env -i
	// PATH=/usr/bin:/bin LC_ALL=C`, bash 5.3.20 in a shell that has read
	// nothing: `declare -a` names BASH_ARGC, BASH_ARGV, BASH_LINENO,
	// BASH_SOURCE, DIRSTACK, FUNCNAME and GROUPS, `declare -A` names
	// BASH_ALIASES and BASH_CMDS, and `declare -i` names BASHPID, RANDOM and
	// SRANDOM — while `declare -x` and `declare -r` name none, because no
	// produced parameter carries either attribute.
	//
	// This engine wrote **one** row for `declare -a` — the stored
	// BASH_VERSINFO — while its own `declare -p` wrote all eight, so the
	// names were registered and the filtered walk was the one place that
	// could not see them.
	names, produced, listing := r.listedNames()
	if r.unspecified {
		return r.status, true
	}
	if f.added {
		// Some letter that names an attribute was written with a *minus*,
		// which is the filtered listing carrying values — `declare -x`
		// writing `declare -x e="1"` and `typeset -a` writing `q=( a b )`.
		// The same names the plus form selects, written as rows instead of
		// as bare names, so the two cannot come to disagree about which
		// names those are (#1868).
		return r.declarationFilteredListing(names, produced, listing, keep), true
	}
	return r.declarationFilteredNameListing(names, produced, listing, keep), true
}

// withoutListingLetters is one declaration's letters with every letter a
// listing knows how to read taken out, so that "this line asks for nothing
// else" is one comparison rather than a list of fields a new attribute would
// silently fall off the end of — the reason withoutMatching is written the
// same way.
//
// What is deliberately *not* cleared is the point of it: `T`, `h`, `p` and
// the function letters stay, so a line carrying one is not a shape this
// answers and falls through to the branch that does.
func withoutListingLetters(f declareFlags) declareFlags {
	f.integer, f.integerOff, f.base, f.baseNamed = false, false, 0, false
	f.float, f.precision, f.precisionNamed = false, 0, false
	f.widthLetter, f.widthZeroFill = 0, false
	f.width, f.widthNamed = 0, false
	f.readonly, f.readonlyOff = false, false
	f.export, f.assoc, f.array = false, false, false
	f.lower, f.upper, f.unique, f.hidden = false, false, false, false
	f.traced = false
	f.global, f.inert = false, false
	// The reference letter is a listing letter too — see attributeFilter,
	// where the measurement is. Without this line `declare -n` with no
	// operand was not a listing at all: it fell past declarationListing into
	// the declaration loop, which had no operand to declare, and wrote
	// nothing at 0 where bash writes every reference it has.
	f.nameref = false
	return withoutMatching(f)
}

// declareFunctions is `declare -f` and `-F` and `typeset +f`: the functions
// themselves, or only their names.
//
// One shell has each of these under `declare` and prints `-F` in its own two
// shapes — `declare -f name` per function when nothing narrows it, the bare
// name when an operand asked — so the shapes are written here the way `-t`'s
// kind words are: there is no second engine to hold a wording for.
//
// asDeclarations is what tells those two shapes apart, and it is the `-F`
// letter's rather than the listing's: measured 2026-09-10, bash 5.3 writes
// `declare -f f` for `declare -F` and `f` for `declare -F f`, while zsh 5.9.2
// writes the bare `f` for `typeset +f` with an operand and without one alike.
// So the letter carries the shape and the operand count only chooses between
// bash's two.
//
// A name is written **raw**, and that is measured rather than inherited from
// listedFunctionName: `function "a b" { :; }; typeset +f` writes `a b` with
// no quotes in zsh 5.9.2, where the same shell's body listing writes
// `'a b' () {`. A names-only listing is a list of names and not a program
// that reads back (#1576).
// narrowed says the caller has already chosen the population — a listing
// filtered by the marks its letters named — so an empty slice is an empty
// listing rather than a request for the whole table.
//
// locates is extended debugging asking for the line and the file after the
// name, and it is the caller's to decide rather than read here because the
// same option is off for the shape `-p` asks for — see LocatesFunctions and
// the call in declareBuiltin.
func (r *Runner) declareFunctions(names []string, narrowed, namesOnly, asDeclarations, locates, print bool) int {
	named := len(names) > 0 || narrowed
	// Whether a *name* asked, which is not the same question and decides two
	// other things: `declare -F f` is the bare name where `declare -F` is a
	// declaration line, and `declare -f f` is the body alone where `declare
	// -f` writes the attribute line after a body that has one. A narrowed
	// listing is the *table* filtered rather than a name asked for, and bash
	// 5.3.20 writes both of those for it — `declare -Fr` is `declare -fr b`
	// and `declare -fr` is the body and then that same line (#3192).
	byOperand := len(names) > 0 && !narrowed
	if !named {
		// The script's own and not the prelude's: this listing is what a
		// state capture reads, and the prelude's functions are the shell's
		// (#1035, scriptFuncNames). A name asked for is still answered,
		// which is why only this branch narrows.
		names = r.scriptFuncNames()
	}
	status := 0
	for _, name := range names {
		fn, ok := r.reportedFunc(name)
		if !ok {
			// Silent, and 1 stands however many other names printed —
			// measured in both shells that can be asked. The **`-p` word**
			// is what may report instead, and it is a question of its own:
			// see Semantics.DeclarePrintReportsAMissingFunctionName, where
			// the panel splits differently from the variable spelling's.
			//
			// Asked only for a name an operand named, because that is where
			// the two readings differ: a whole-table listing collected the
			// names itself and has nobody's operand to report.
			if print && byOperand && r.ask(r.sem().DeclarePrintReportsAMissingFunctionName,
				"`-p` reporting a name that is not there beside the function letter") {
				// The variable spelling's sentence and deliberately the same
				// one, with this builtin's own word in front of it: measured,
				// `declare -pf g nosuch` writes `g`'s body and then the same
				// `declare: nosuch: not found` a `declare -p nosuch` writes.
				r.diagf("%s: %s\n", r.inBuiltin,
					Wording(r.diag().DeclareNoSuchVariable, "%[1]s: not found", name))
			}
			if r.unspecified {
				return r.status
			}
			status = 1
			continue
		}
		switch {
		case !namesOnly:
			r.printf("%s", r.listedFunctionLine(name, fn))
			if !byOperand {
				// And the attributes under the body, where this dialect has
				// them and this function holds some.
				r.printf("%s", r.functionAttributeLine(name))
			}
		case byOperand && asDeclarations && locates:
			// Extended debugging: the name, the line the definition begins
			// on and the file it was read from. See LocatesFunctions — it is
			// what a shell-level debugger needs to put a breakpoint
			// anywhere, and this shell already kept both halves.
			r.printf("%s %d %s\n", name, r.functionDefinitionLine(name, fn),
				r.functionDefinitionFile(name))
		case (byOperand && !print) || !asDeclarations:
			// The bare name, which is what a names-only listing writes for a
			// name an operand asked about — and the `-p` word takes that
			// back, because it asks for the shape the line could be reissued
			// in. Measured 2026-09-21 on bash 5.3.20: `declare -F g` is `g`,
			// `declare -Fp g` is `declare -f g`, and `declare -F` with no
			// operand is `declare -f g` too. The call site below already
			// said so about the location this form leaves out; the branch
			// did not, so `-Fp` wrote the bare name (#4065).
			r.printf("%s\n", r.listedFunctionNameOnly(name, fn))
		default:
			r.printf("declare -f%s %s\n", r.functionAttributes(name), name)
		}
	}
	return status
}

// listedFunctionLine is one row of a body listing, terminator and all — what
// the shell writes for one function and nothing more.
//
// Every caller writes this string as it stands rather than adding a newline,
// because in one dialect there is no newline to add: ksh93 says the
// definition back verbatim and the text already ends with the character that
// ended the statement, so a listing under `-c` ends with a `;` and a bare
// listing of two functions runs them together. See
// Diagnostics.FunctionListingIsSourceText.
//
// A function still waiting for its body is asked about first. The text a
// listing writes in its place is the shell's own sentence about the name —
// see SetUndefinedFunctions — and the declaration it is marked on may well
// carry the source of the body it *used* to have, which is not what that
// shell writes there.
func (r *Runner) listedFunctionLine(name string, fn *syntax.FuncDecl) string {
	if _, undefined := r.undefinedFunction(name); undefined {
		return r.listedFunction(name, fn) + "\n"
	}
	if r.diag().FunctionListingIsSourceText && fn != nil && fn.SourceText != "" {
		return fn.SourceText
	}
	return r.listedFunction(name, fn) + "\n"
}

// listedFunction is a function said back whole, in the dialect's arrangement:
// the header the dialect writes — see Diagnostics.FunctionListingHeader —
// and the body laid out by its function layout.
//
// The header is the `name ()` form for every declaration, including one
// written with the `function` keyword, and that is measured rather than
// overlooked. #1406 is the same question asked of syntax.Print, where the
// answer is the other one: a tree carries FuncDecl.Keyword because ksh93's
// `typeset` needs it, so a *printer* that dropped it handed back a program
// whose locals leak. A listing does not, for two reasons that have to hold
// together — bash 5.3, bash 3.2 and zsh 5.9.2 all write `f () ` back for
// either spelling, which they may because `typeset` declares a local in both
// bodies there; and ksh93, which does write `function f { …; }` back, has no
// `-f` listing in this implementation to reach here at all. If one is added,
// this line is where the keyword has to come back — the body is all that
// goes through the printer, so the printer's fix does not reach it.
// Measured 2026-09-08 on bash 5.3.15, bash 3.2.57, zsh 5.9.2 and ksh93u+.
func (r *Runner) listedFunction(name string, fn *syntax.FuncDecl) string {
	if _, ok := r.undefinedFunction(name); ok {
		if row := r.diag().UndefinedFunctionListing; row != "" {
			// A shell whose rendering of a name still waiting is a
			// *declaration* rather than a body — no header, no braces, and
			// so nowhere for the block form below to go. See
			// Diagnostics.UndefinedFunctionListing.
			return Wording(row, "", listedFunctionName(name))
		}
	}
	body := syntax.PrintWith(fn.Body, r.functionLayout)
	if text, ok := r.undefinedFunction(name); ok {
		// A function whose body has not been read yet does not print its
		// tree: what stands between the braces is the shell saying what the
		// function is, and it is not in the tree to be printed. See
		// SetUndefinedFunctions. The header is still this listing's, so a
		// name that needs quoting gets it here and in one place.
		body = text
	}
	header := r.diag().FunctionListingHeader
	if fn.Keyword && r.diag().FunctionListingKeywordHeader != "" {
		// The word the declaration was written with, put back — see
		// Diagnostics.FunctionListingKeywordHeader. Only the dialect that
		// runs the two bodies differently answers this, and it is the whole
		// of why the header is not one wording: the body goes through
		// syntax.Print, which keeps the keyword, and the header never
		// reaches it.
		header = r.diag().FunctionListingKeywordHeader
	}
	return Wording(header, "%[1]s () \n%[2]s", listedFunctionName(name), body)
}

// listedFunctionNameOnly is one row of a **names-only** function listing.
//
// Raw, and that is measured rather than inherited from listedFunctionName:
// `function "a b" { :; }; typeset +f` writes `a b` with no quotes in zsh
// 5.9.2, where the same shell's body listing writes `'a b' () {`. A listing
// of names is a list and not a program that reads back (#1576).
//
// What the dialect may add is *punctuation that says which declaration this
// was*, which is one shell's answer alone: measured on ksh93u+, `f() { :; };
// function g { :; }; typeset +f` writes `f()` and then `g`. See
// Diagnostics.FunctionNameListing and its keyword half — the same pair the
// body listing's header is, for the same reason.
func (r *Runner) listedFunctionNameOnly(name string, fn *syntax.FuncDecl) string {
	spelling := r.diag().FunctionNameListing
	if fn != nil && fn.Keyword && r.diag().FunctionNameListingKeyword != "" {
		spelling = r.diag().FunctionNameListingKeyword
	}
	return Wording(spelling, "%[1]s", name)
}

// listedFunctionName is the name half of that header, written so the listing
// reads back as the definition it describes.
//
// Only one dialect can hold a name that needs anything done to it — see
// syntax.Dialect.FunctionKeywordNameIsAnyWord, under which the word after the
// keyword is the name whatever is in it — and that dialect quotes such a name
// when it writes one back. Measured 2026-09-08 on zsh 5.9.2 by defining
// `function "$n" { :; }` for each name and reading the first line of
// `functions`:
//
//	a-b   a.b   a!b   x1   1x   _x        bare
//	''    'a b'   'a;b'   'a|b'   'a$b'   quoted
//	'@#%'   'a*b'   'a{b'   'a}b'   'a=b'   'a~b'   'a]b'   'a^b'
//	'a'\''b'                                a quote inside the quotes
//
// So the bare set is the characters a name may hold and still read back as one
// ordinary command word: letters, digits, `_`, and `! % + , - . / : @`, in any
// position — a leading digit and a trailing `-` are both bare. It is measured
// over all thirty-two printable ASCII punctuation marks and is **not**
// syntax.Dialect.FunctionNamePunctuation, which is a different question with a
// different answer: `#`, `]` and `^` are names to the parser and are quoted
// here.
//
// Nor is it interp/minimalquote.go's table, though it looks like it. That one
// answers `${(q-)…}`, where `~` and `=` are special only at a word's start, so
// `a~b` and `PATH=/x` come back bare; here they are quoted wherever they sit,
// a command word being read differently from a value. Two tables because the
// measurements differ, said here so the next reader does not fold them.
func listedFunctionName(name string) string {
	if name != "" && !strings.ContainsFunc(name, functionNameNeedsQuotes) {
		return name
	}
	return "'" + strings.ReplaceAll(name, "'", `'\''`) + "'"
}

// functionNameNeedsQuotes is the bare set above, read one rune at a time.
func functionNameNeedsQuotes(c rune) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return false
	case c == '_':
		return false
	case strings.ContainsRune("!%+,-./:@", c):
		return false
	}
	// Every byte above ASCII goes bare too, which is the same reading
	// syntax.Dialect.FunctionNamePunctuation takes of a name written in
	// another script. Measured: `function ä { :; }; functions` is `ä () {`.
	return c < 0x80
}

// markReadonly freezes a name.
//
// Undone in one dialect: zsh's `typeset +r` takes the attribute back off, and
// bash and ksh93 refuse to. So `+r` is an answered question rather than a
// no-op — see removeReadonly and Semantics.ReadonlyAttributeCanBeRemoved. The
// note that used to stand here said no shell in the panel could unfreeze a
// name, and it had been measured on bash and ksh93 alone (#1168).
//
// A name the running declaration is also assigning to as an operand is frozen
// *after* that assignment rather than now: see the freezing field and
// applyDeferredFreeze. `declare -ar A=(x y)` is one command, and the value it
// carries cannot be refused by the attribute it carries beside it.
func (r *Runner) markReadonly(name string) {
	// The declaration is freezing a name its own command's prefix is holding,
	// in the column where that keeps the prefix's value for the shell. Ahead
	// of the deferred freeze below, because the keeping is about this command
	// either way and the deferral only moves *when* the attribute lands. See
	// Runner.keepThePrefixEntry.
	r.keepThePrefixEntry(name)
	if r.freezing[name] {
		r.freezeAfter = append(r.freezeAfter, name)
		return
	}
	if r.readonly == nil {
		r.readonly = map[string]bool{}
	}
	r.readonly[name] = true
}

// removeReadonly answers a plus form that asked for the readonly attribute
// back off a name — `typeset +r x`.
//
// One dialect grants it and two refuse, and the refusal is the ordinary
// readonly refusal through a declaration rather than a sentence of its own:
// see Semantics.ReadonlyAttributeCanBeRemoved. Nothing is asked about a name
// that is not frozen, which is every `typeset +r` a script writes to make
// sure a name is writable — the plus form on a free name reports 0 and says
// nothing in all three shells that spell it.
//
// hasValue is whether the same operand also carries an assignment, and it
// decides which of the two refusals speaks. `typeset +r x=5` on a frozen name
// is refused as an *assignment* — measured, and it is the shape that tells
// the two apart: ksh93 answers the valueless form in its builtin location
// with the builtin named and this one in the plain line form. So a plus form
// with a value says nothing here and lets the assignment behind it report.
//
// The freeze is dropped before that assignment rather than after, which is
// the whole of what the shell that grants this does: `typeset -r x=1;
// typeset +r x=5` leaves 5 there.
func (r *Runner) removeReadonly(name string, hasValue bool) int {
	if !r.readonly[name] {
		return 0
	}
	if r.ask(r.sem().ReadonlyAttributeCanBeRemoved,
		"the readonly attribute being taken off a name") {
		delete(r.readonly, name)
		return 0
	}
	if r.unspecified || hasValue {
		return r.status
	}
	r.refuseReadonly(name, attributeRatherThanAValue)
	return r.status
}

// applyDeferredFreeze freezes the names markReadonly held back, now that the
// declaration's own operand assignments have landed.
//
// Held as a list rather than a set because nothing here depends on the order
// and a list says so: the same name twice freezes once either way.
func (r *Runner) applyDeferredFreeze() {
	for _, name := range r.freezeAfter {
		if r.readonly == nil {
			r.readonly = map[string]bool{}
		}
		r.readonly[name] = true
	}
	r.freezeAfter = nil
}

// operandNames is the set of names a command assigns to as operands — the
// `a=(x y)` written after a declaration utility's own word.
//
// Nil when there are none, which is every command that is not a declaration
// with an array in it, so the lookup markReadonly makes costs a nil map read.
func operandNames(c *syntax.SimpleCmd) map[string]bool {
	var names map[string]bool
	for _, a := range c.Assigns {
		if !a.Operand {
			continue
		}
		if names == nil {
			names = map[string]bool{}
		}
		names[a.Name] = true
	}
	return names
}

// arrayLiteralOperands is the subset of operandNames whose operand is an
// array literal — `typeset -i z=(1 2)` rather than `typeset -i z=1`.
//
// Beside operandNames rather than folded into it because the two answer
// different questions of the same list: that one is "which names is this
// declaration about to assign", which decides when a freeze may land, and
// this one is "what shape is the value", which decides whether the letters
// and the value are the same kind. See Runner.literalOperands.
func arrayLiteralOperands(c *syntax.SimpleCmd) map[string]bool {
	var names map[string]bool
	for _, a := range c.Assigns {
		if !a.Operand || !a.IsArray {
			continue
		}
		if names == nil {
			names = map[string]bool{}
		}
		names[a.Name] = true
	}
	return names
}

// exportRefusesACompound reports — and refuses — an export letter written over
// a name whose value is a compound variable, which is the one kind the
// environment has no representation of at all.
//
// A correction rather than an axis: one dialect in the panel has the fourth
// kind, so there is no second answer to choose between. Measured 2026-09-14 on
// ksh93u+ 2012-08-01, `env -i PATH=/usr/bin:/bin` with a scratch HOME, over
// `-c`, a script file and standard input alike — status 1 and the script ends
// in each:
//
//	export c=(a=1)               export: c: only simple variables can be exported
//	c=(a=1); export c            the same
//	c=(a=1); typeset -x c        typeset: c: only simple variables can be exported
//	typeset -Cx c=(a=1)          the same
//	typeset -x a=()              typeset: a: only simple variables can be exported
//	c=(a=1); typeset -Cx d=c     typeset: d: only simple variables can be exported
//
// The last row is why the name reported is the operand's and not the value's:
// the copy names `d`. The fifth is why an *empty* parenthesized literal
// belongs here — `a=()` is a compound in that shell, not an empty array, which
// is what its own `typeset -p a` says.
//
// Two neighbors are the control, and both are taken at 0 there: `typeset -x
// a=(p q)` exports an index array and `typeset -Ax m` a table. So it is the
// compound alone and not every parenthesized value, which is what keeps this
// from being a refusal of arrays wearing a compound's name.
//
// The three ways a name is one on this line are all asked, because they are
// three different moments: it is already a compound, the `-C` letter is making
// it one, or the operand carries a compound literal for it to become. A check
// that waited for the store would have let the attribute land first.
func (r *Runner) exportRefusesACompound(name string, f declareFlags) bool {
	if !f.export || f.remove || r.diag().CompoundIsNotExportable == "" {
		return false
	}
	if !r.isCompoundVariable(name) && !f.compoundVar && !r.compoundOperands[name] &&
		// And a **member** of a compound that stands, which is a name the
		// word takes and then refuses in these same words: `typeset
		// zz=(a=1); export zz.a` is `export: zz.a: only simple variables can
		// be exported`, and with a value it names `zz.a` rather than the
		// whole operand. See Runner.scalarMemberStands, where the rows that
		// narrow it are.
		!r.scalarMemberStands(name) {
		return false
	}
	r.fatal("%s\n", Wording(r.diag().CompoundIsNotExportable,
		"%[2]s: %[1]s: only simple variables can be exported",
		name, r.builtinComplaintName(r.inBuiltin)))
	return true
}

// compoundLiteralOperands is arrayLiteralOperands narrowed to the parenthesized
// operands whose items are *assignments* — a compound variable's body — which
// is the shape the parser kept apart as Assign.Members.
//
// Two functions rather than one map with two bits in it, because the two are
// read by different questions and one of them existed first: widening
// literalOperands to carry a kind would have made every caller of it decide
// something none of them was asking.
func compoundLiteralOperands(c *syntax.SimpleCmd) map[string]bool {
	var names map[string]bool
	for _, a := range c.Assigns {
		if !a.Operand || a.Members == nil {
			continue
		}
		if names == nil {
			names = map[string]bool{}
		}
		names[a.Name] = true
	}
	return names
}

// integerValue evaluates an assignment to a name declared integer.
//
// An empty assignment is zero rather than nothing, and a name that is not set
// is zero rather than an error — `n=abc` leaves 0 and says nothing, because
// `abc` is a perfectly good expression whose value happens to be unset. Text
// that will not parse *is* an error, and a fatal one: bash and zsh both stop
// the script rather than store something.
func (r *Runner) integerValue(text string) (string, bool) {
	v, ok := r.integerNumber(text)
	if !ok {
		return "", false
	}
	return itoa(v), true
}

// integerNumber is integerValue before the number is written down.
//
// Its own function because `+=` wants the number rather than the text: an
// attributed name adds, so the value it holds and the expression on the right
// have to meet as integers, and neither of them is the answer on its own.
// Folded out of integerValue rather than copied beside it — the parse, the
// dialect it parses under and the wording of both failures are one thing, and
// a second evaluator here is the shape that has cost this tree seven bugs.
func (r *Runner) integerNumber(text string) (int, bool) {
	if n, ok := r.zeroPaddedInteger(text); ok {
		return n, true
	}
	if r.unspecified {
		return 0, false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, true
	}
	p := syntax.NewParser("", r.dialect())
	e := p.ParseArithFor(text, syntax.Pos{})
	if err := p.Err(); err != nil {
		r.mathFatalf("%s", r.diag().ParseFailure(err))
		return 0, false
	}
	v, err := r.evalArith(e)
	if err != nil {
		// Worded where every other arithmetic failure is worded, so the
		// value is quoted back the way the dialect quotes an expression:
		// `typeset -i b; b=0x` is `0x: arithmetic syntax error` in ksh93,
		// where the raw error said `arithmetic syntax error` with nothing
		// named at all. The blamed text is the assignment's own value, which
		// carries no surrounding blanks — `$(( 0x ))` in the same shell is
		// `` 0x `` with them, from the same wording and a different text
		// (#2420).
		if r.arithNounsetNamedTheParameter {
			// Already fatal and already carrying the status that route gives
			// a `set -u` refusal — 127 from a `-c` string in bash, which
			// Runner.fatal would put its own 1 over. See
			// Runner.arithNounsetRefusal.
			//
			// The refusal is the shell's own rather than the declaration's
			// — the same split `let` makes, and for the same reason — so it
			// keeps diagf and does not go through the naming rule below.
			r.diagf("%s\n", r.arithFailure(text, err))
			return 0, false
		}
		// mathFatalf rather than fatal: this is the evaluator's sentence
		// raised through a builtin, and each column names the builtin for it
		// exactly where it names one for `let`. See Runner.mathFatalf.
		r.mathFatalf("%s", r.arithFailure(text, err))
		return 0, false
	}
	return v, true
}

// declaredNameWithoutValueIsEmpty answers whether a declaration with no value
// sets the name, and decides what an unanswered vector does about it.
//
// The disagreement is one question — see
// Semantics.DeclaredNameWithoutValueIsEmpty — and the *word* it is written on
// decides whether a vector that has not answered refuses or reads. `export
// name` and `readonly name` are POSIX's own spellings, in 2.14, and a script
// that says either is asking for an attribute rather than for a value; every
// shell in the panel runs both. So an unanswered vector reads them the way
// four of the five columns do rather than refusing a line the standard
// mandates, which is the bargain BackgroundJobInput and the `getopts` answers
// in CoreSemantics already strike.
//
// `typeset`, `declare` and `local` are nobody's standard, and they ask: a
// dialect that means to spell a declaration has to say what its declarations
// leave behind.
func (r *Runner) declaredNameWithoutValueIsEmpty(standardWord bool) bool {
	if standardWord {
		return r.sem().DeclaredNameWithoutValueIsEmpty == Yes
	}
	return r.ask(r.sem().DeclaredNameWithoutValueIsEmpty,
		"a declaration without a value setting the name")
}

// declarationOwnsTheStandingEmpty is what a declaration naming an attribute
// does to a name a *previous* declaration left with no value of its own: the
// name now holds the empty string in its own right, and a child is told about
// it.
//
// Only reachable where a valueless declaration sets the name at all — see
// Semantics.DeclaredNameWithoutValueIsEmpty and Runner.declaredEmpty — so it
// is core rather than an axis: the dialects that leave such a name unset have
// no record here to clear.
//
// Measured 2026-09-12 on zsh 5.9.2, `-f`, `env -i`, reading a real child's
// environment with `env`:
//
//	typeset -x A; env                        nothing
//	typeset -x A; typeset -x A; env          A=
//	typeset -x A; export A; env              A=
//	typeset -x A; readonly A; env            A=
//	typeset -x A; typeset +r A; env          A=
//	typeset A;    export A; env              A=
//	typeset -x A; typeset A; env             nothing
//	typeset -x A; typeset -p A; env          nothing
//
// The last two are what the guard is for and what makes this "naming an
// attribute" rather than "a second command": a bare `typeset A` over a name
// that already holds something is a *listing* in that shell rather than a
// declaration, and so is `typeset -p`. Neither changes what the child is
// told; every line that writes a letter, or is spelled with one of the words
// that is an attribute, does.
//
// It is a real difference and not a corner. `typeset -x V` then `export V` is
// how a script declares an exported name it means to fill in later, and the
// child was told nothing about it here — a `make`-style wrapper that exports
// its variables up front handed them all over as absent.
func (r *Runner) declarationOwnsTheStandingEmpty(name string) {
	delete(r.declaredEmpty, name)
}

// zeroPaddedInteger answers a value written as a digit string with a leading
// zero, where the dialect reads one in decimal — see
// Semantics.IntegerAssignmentReadsALeadingZeroAsDecimal. The second result is
// whether that reading applied at all; everything else goes on to the
// arithmetic parse below it.
//
// Asked here rather than inside the arithmetic evaluator because it is the
// *assignment* that splits and not the expression: measured 2026-09-10 on
// ksh93u+, `$((010))` is 8 and `typeset -i d=010` is 10 in the same shell, so
// the two are different readers there and a fix routed through the evaluator
// would move both.
//
// The shape is the whole of what is asked, and each edge of it is measured on
// the shell that reads decimal:
//
//	typeset -i d=010    10   the digits, in decimal
//	typeset -i d=-010  -10   a sign belongs to the number
//	typeset -i d=09      9   an invalid octal digit is just a digit
//	typeset -i d=0       0   one digit is no disagreement at all
//	typeset -i d=" 010 " 8   not a number, so the expression reader has it
//	typeset -i d=010+1   9   an expression, and octal inside it
//	typeset -i d=0x10   16   a prefix both readers agree about
//
// So the text has to be the number and nothing else — no spaces around it, no
// operator in it — which is what tells an assignment of a padded number apart
// from an expression that happens to start with one.
func (r *Runner) zeroPaddedInteger(text string) (int, bool) {
	if !zeroPadded(text) {
		return 0, false
	}
	if r.sem().ArithLeadingZeroIsOctal != Yes {
		// The two readings agree: nothing made the zero octal, so the
		// expression reader already answers ten for `010`. Asking a dialect
		// to choose between two identical answers is the shape that refuses
		// a line no shell disagrees about.
		return 0, false
	}
	if !r.ask(r.sem().IntegerAssignmentReadsALeadingZeroAsDecimal,
		"a zero-padded number assigned to an integer name") {
		return 0, false
	}
	n, err := strconv.Atoi(text)
	if err != nil {
		// Out of range for this machine's int, which the expression reader
		// below reports in its own words rather than this one guessing.
		return 0, false
	}
	return n, true
}

// zeroPadded reports whether text is a decimal digit string, optionally
// signed, whose first digit is a zero standing in front of another — the one
// shape the two readings answer differently.
func zeroPadded(text string) bool {
	digits := strings.TrimPrefix(strings.TrimPrefix(text, "-"), "+")
	if len(digits) < 2 || digits[0] != '0' {
		return false
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return false
		}
	}
	return true
}

// defaultFloatPlaces is how many decimal places `-F` writes with no number
// after it. Ten in both shells with the attribute — measured 2026-09-07,
// `typeset -F x=1.5` is `1.5000000000` in zsh 5.9.2 and ksh93 alike — so it
// is a constant and not a table: a dialect field with one value in every
// dialect records an agreement as though it were a choice.
const defaultFloatPlaces = 10

// floatPlaces is the precision a float name renders in, which is the number
// its letter named or the default where it named none.
//
// Zero is the absence of a number rather than a request for no decimal
// places at all: `typeset -F 0 x=3.9` is `3.9000000000` in zsh, the same as
// the bare letter. ksh93 disagrees and this follows zsh, which is the only
// dialect the attribute is given to — see #1461.
func floatPlaces(prec int) int {
	if prec <= 0 {
		return defaultFloatPlaces
	}
	return prec
}

// floatValue evaluates text as an arithmetic expression and answers the
// float it comes to, which is what a float name stores rather than the
// characters written: `typeset -F 3 x=1+2` is `3.000`.
//
// integerValue's sibling and deliberately not a flag on it: the two differ
// only in which half of an arithNum they keep, and a shared function with a
// bool would put that choice at every call site instead of in the one place
// the attribute is read.
func (r *Runner) floatValue(text string) (float64, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, true
	}
	p := syntax.NewParser("", r.dialect())
	e := p.ParseArithFor(text, syntax.Pos{})
	if err := p.Err(); err != nil {
		r.mathFatalf("%s", r.diag().ParseFailure(err))
		return 0, false
	}
	v, err := r.evalNum(e)
	if err != nil {
		// arithFailure and mathFatalf, as integerNumber does: the float
		// letters reach the same evaluator and the same two questions — which
		// text the sentence quotes back, and which column names the builtin —
		// and answering them differently here left ksh93's `float a=1/0` as a
		// bare `divide by zero` where that shell writes `typeset: 1/0: divide
		// by zero` (#3342).
		r.mathFatalf("%s", r.arithFailure(text, err))
		return 0, false
	}
	return v.asFloat(), true
}

// declareEmpty is what declaring a name without a value does.
//
// The name is now local, or attributed, or both — but whether it also *exists*
// is a dialect's answer, so this is the one place that decides it and both
// `local` and `typeset` come through here.
func (r *Runner) declareEmpty(name string, fresh, keepsTheEnvironmentEntry, namesAnAttribute, inherits, standardWord bool) {
	// A name that already holds a value is not one this declaration is
	// bringing into being, and nothing about being declared empties it:
	// `typeset -x v` on a `v=abc` leaves `abc` alone in all four shells that
	// spell the builtin, and so does `typeset -H h` on an `h=hid`. Emptying
	// it made a declaration that only meant to add an attribute destroy the
	// value it was adding it to.
	//
	// Inside a function the cell a shadow just made is new whatever the
	// caller held, which is what fresh says — measured, `v=5; function f {
	// typeset -i v; echo "[${v-UNSET}]"; }` reads UNSET in bash and ksh93 and
	// `0` in zsh, against `[5]` for the same line at the top.
	// A *second* declaration naming an attribute, over a name the first one
	// left with no value of its own. Read before anything else because the
	// paths below return early on their own — a re-read that changes nothing
	// stops there — and this is true whichever of them the line takes. Not
	// for a fresh cell: the record is keyed by name, so a local shadowing an
	// outer declared-empty name is the first declaration of *its* cell.
	owned := namesAnAttribute && !fresh && r.declaredEmpty[name]
	if owned {
		r.declarationOwnsTheStandingEmpty(name)
	}
	if !fresh && r.declaredNameHolds(name) {
		if r.declarationStartsAnInheritedNameOver(name, keepsTheEnvironmentEntry) {
			// The name is being started over rather than added to, so it
			// falls through to the branches below as though it had been
			// holding nothing — which is what it is now holding.
			if r.unspecified {
				return
			}
		} else {
			if r.unspecified {
				return
			}
			if !r.rereadStandingValue(name) {
				return
			}
			// The one answer that starts the name over too: a compound value
			// discarded so the declaration can bring the name into being
			// afresh as a scalar of the declared type. Same fall-through and
			// for the same reason as above.
			if r.unspecified {
				return
			}
		}
	}
	if r.declaredNameWithoutValueIsEmpty(standardWord) {
		// The array half of a tie is set empty in its own kind — no elements,
		// not an empty string — and the mirror carries that to the scalar. See
		// tielocal.go.
		if r.declaredEmptyTieArray(name) {
			return
		}
		// assignedAsTheCompoundView, because this is not a value the script
		// wrote: it is the empty a declared name holds, and where the letters
		// just declared an array or a table it is that compound's view. A
		// plain scalar store here would ask scalarOverCompound whether to
		// replace the compound the same declaration had brought into being,
		// and in the dialect that replaces, `local -a opts` left a string.
		r.setVarAs(name, "", assignedAsTheCompoundView)
		if owned {
			// Cleared above, and this is the branch that would put it
			// straight back.
			return
		}
		// Set by a declaration and not by an assignment, which the shell's
		// own reads cannot tell apart and a child can: see
		// Runner.declaredEmpty.
		if r.declaredEmpty == nil {
			r.declaredEmpty = map[string]bool{}
		}
		r.declaredEmpty[name] = true
		return
	}
	if r.unspecified {
		return
	}
	if fresh && r.prefixEntryIsInThisCell(name) {
		// The cell this declaration made is the one an assignment prefix put
		// a value in, so there is nothing here to empty: `f(){ local c;
		// declare -p c; }` called as `c=2 f` reads `declare -x c="2"`, and so
		// does the builtin's own prefix in `f(){ b=4 declare -r b; }`.
		// Measured 2026-09-16 in bash 5.3.20, ksh93u+ (`function f { typeset
		// c; … }`, the only form that scopes there) and BusyBox ash 1.37 —
		// the three columns that hide an outer value at all, so it is
		// unanimous and asked of nobody. zsh gives a declared name the empty
		// string and returns above; dash hides nothing, so neither reaches
		// this line. See Runner.prefixEntryShadowed (#3437).
		return
	}
	// The name is unset here, and whether it is *recorded* is the second
	// question — the one bash and ksh93 part on. Noted for every dialect and
	// asked of none: the axis is read where the record is, which is the
	// listing and nothing else, so `local u` — a line every dialect runs —
	// asks nothing. Only for an operand that named no attribute, because a
	// letter is already a record: every shell that has one lists `typeset
	// -i xyz` for a name holding nothing. See baredeclaration.go.
	// The script has asked for the enclosing binding rather than for
	// nothing, either through this declaration's own letter or through the
	// shell-wide name. Ahead of everything below, because all of it is about
	// a cell that holds no value and this one does: the bare-declaration
	// record would make a listing write `declare -- v` over an inherited
	// `v="OUTER"`, the ignore parameter would be taken out of view, and the
	// axis under it asks whether the outer value is *hidden* — which a cell
	// that has just been handed it has no occasion to ask.
	//
	// `fresh` is the same gate everything below keeps: a second declaration
	// of a name its own scope already made has no enclosing binding in front
	// of it. See localinherit.go for the measurement, and for why a scope
	// that held nothing falls through to the ordinary fresh binding.
	if fresh && inherits && r.restoreTheOuterBinding(name) {
		return
	}
	if !namesAnAttribute {
		r.recordBareDeclaration(name)
	}
	// Whether the *outer* value still shows through is a third disagreement,
	// and it only arises where a shadow was actually taken: `declare u` at
	// the top level leaves the global alone in every shell measured.
	//
	// Taken *by this declaration*, which is what `fresh` says and what the
	// question turns on. A second declaration of a name its own scope already
	// declared has no outer value in front of it — the value it would hide is
	// the local the first declaration made — and no shell hides that.
	// Measured 2026-09-06, `env -i`, from a file and through `-c` alike:
	// `f() { local FOO=x; local FOO; echo "[${FOO-UNSET}]"; }` reads `x` in
	// bash 5.3.15, bash as `sh`, bash 3.2.57 and zsh 5.9.2, and the ksh93
	// spelling `typeset FOO=x; typeset FOO` reads `x` in all four including
	// ksh93u+. `typeset -g FOO` after a `local FOO=x` is the same shape and
	// the same answer. It is a *different* function's declaration that hides:
	// `f() { local FOO=x; g; }; g() { local FOO; ... }` reads UNSET, and that
	// is the case this axis is about.
	//
	// Asking the scope instead of the declaration answered the two the same
	// way, so the value a function had just put in its own local was thrown
	// away by a line that only meant to name it again.
	if !fresh {
		return
	}
	// The cell this declaration made holds no value, so the parameter that
	// takes names out of a pathname expansion stops here exactly as `unset`
	// stops it. A *null* value assigned is a different state and leaves it
	// alone — measured, `GLOBIGNORE=a; GLOBIGNORE=` keeps hidden names
	// visible where `f(){ local GLOBIGNORE; echo *; }` does not see one.
	// See interp/ignorednames.go.
	r.ignoredNamesUnset(name)
	if r.ask(r.sem().ValuelessDeclarationHidesTheOuterValue, "a declaration without a value hiding the outer value") {
		r.hideVar(name)
	}
}

// declarationAssignmentExport is what a declaration that *assigns* does to the
// export attribute of the name it assigned to.
//
// One shell resets it: the name keeps the value and no child is told about it
// again, at the top level and in a function whose declarations reach the
// caller alike. The other two leave it alone, so this is a switch and not a
// rule — see Semantics.DeclarationAssignmentClearsTheExportAttribute.
//
// namesTheAttribute is `-x` or `+x` on the declaration itself, which settles
// the question outright and is not this one; `export NAME=value` is the same
// case by another spelling, which is why that builtin never comes here.
//
// Asked only where a scope was *not* taken. Where one was, the question is
// LocalInheritsTheExportAttribute — the same shell's answer from the other
// side, already applied and already undone when the function returns. Both
// firing would take the attribute off for good where a keyword function only
// takes it off for its own duration, which is measurably not what happens.
func (r *Runner) declarationAssignmentExport(name string, namesTheAttribute bool) {
	if namesTheAttribute || !r.isExported(name) {
		return
	}
	if len(r.scopes) > 0 {
		if _, shadowed := r.scopes[len(r.scopes)-1].saved[name]; shadowed {
			return
		}
	}
	if !r.ask(r.sem().DeclarationAssignmentClearsTheExportAttribute,
		"a declaration that assigns taking the export attribute off the name") {
		return
	}
	if r.exported == nil {
		r.exported = map[string]bool{}
	}
	r.exported[name] = false
}

// declarationStartsAnInheritedNameOver reports whether this declaration takes
// the name back to the state a *fresh* one would be in, having done so.
//
// The third answer to the question rereadStandingValue asks, on the one input
// where the two shells that re-read part company — see
// Semantics.InheritedValueSurvivesADeclaredType for the shape and what each
// part of it is guarding against. A shell that says no here discards a value
// the script never assigned rather than reading it back through the attribute
// that just arrived, so both the value and the export attribute go and the
// name reads as one this shell has never held.
//
// Asked *before* attributeWouldChange rather than inside rereadStandingValue,
// because the two questions have different inputs: the re-read is about a
// value the attribute would alter, and this is about where the value lives.
// An inherited `7` meeting `-i` and an inherited `UPPER` meeting `-u` are
// discarded by the shell that discards, and neither is a value any fold would
// touch — so asking behind that predicate would have answered the shape's
// commonest spelling with silence.
func (r *Runner) declarationStartsAnInheritedNameOver(name string, keepsTheEnvironmentEntry bool) bool {
	if keepsTheEnvironmentEntry {
		// `-x` or `-r` on this same command. Measured: `typeset -ix G` and
		// `typeset -ir K` keep an inherited value and re-read it, where
		// `typeset -x P; typeset -i P` — the same two attributes over two
		// commands — discards it, and so does any other extra letter. It is
		// the letters this command carries, not the ones the name has.
		return false
	}
	if !r.declaresAType(name) {
		// A declaration with nothing to say about a value leaves an
		// inherited name entirely alone everywhere: a bare `typeset`, `-x`
		// and `-r` all read the value back and still reach a child.
		return false
	}
	if _, own := r.Vars[name]; own {
		// The script has assigned it, so the value is this shell's rather
		// than the one it was started with — and every shell re-reads or
		// waits over that, including the one that discards. `D=$D; typeset
		// -i D` is 0 in all three, which is what says the question is about
		// where the value lives.
		return false
	}
	if _, inherited := r.inheritedValue(name); !inherited {
		// Not an inherited value at all, so there is nothing here to
		// discard. declaredNameHolds counts an array and a keyed table as
		// something the name is holding, and a keyed table keeps no scalar
		// view for the check above to find — so `typeset -A m; m[k]=v;
		// typeset -i m` arrives here with a table and no value anywhere, and
		// without this it would be marked removed and written off the
		// environment for a value it never had.
		return false
	}
	if r.ask(r.sem().InheritedValueSurvivesADeclaredType,
		"a value the shell was started with surviving a declared type") {
		return false
	}
	if r.unspecified {
		return false
	}
	// Both halves, because the shell that does this drops both: the value,
	// and the export the name had by having arrived in the environment.
	//
	// Nothing is deleted from the scalar table, because by here there is
	// nothing in it: the check above returned already for a name this shell
	// holds a value of its own for. A `delete` beside these two read as
	// though it were doing the work and was a line no mutant could kill.
	//
	// The export record is written off rather than deleted, for the reason
	// unsetName has: deleting it puts the question back to the environment,
	// and the environment is precisely what still names this one. The removal
	// mark goes with it, so the inherited value is not read back by the
	// fallback either — and a later assignment lifts the mark without putting
	// the export back, which is what the shell that discards does.
	if r.exported == nil {
		r.exported = map[string]bool{}
	}
	r.exported[name] = false
	if r.removed == nil {
		r.removed = map[string]bool{}
	}
	r.removed[name] = true
	return true
}

// declaresAType reports whether the name carries one of the attributes that
// has something to say about a *value*: the integer letter and the two case
// letters, which are the three that share one answer wherever they are asked.
//
// Read off the name rather than off the declaration's flags because that is
// where applyAttributes has just put them, and because `integer n` — the
// spelling that carries the type in the word — never sets a letter at all.
func (r *Runner) declaresAType(name string) bool {
	return r.integer[name] || r.lowered[name] || r.uppered[name]
}

// compoundNameHolds reports whether the name is holding an array or a keyed
// table — the two values a scalar re-read has nothing to say about.
func (r *Runner) compoundNameHolds(name string) bool {
	if _, ok := r.assocFor(name); ok {
		return true
	}
	a, ok := r.Arrays[name]
	// An array with nothing in it is not a value to start over from, and that
	// is measured rather than an artifact of how the store is kept: `typeset
	// -ia b=(); b=(5+5 6+6)` folds to `10 12` and keeps the letter in bash
	// 5.3.15, exactly as `typeset -ia b` with no literal at all does. It
	// matters because the array attribute *is* an empty store — see
	// markIndexed — so `typeset -ia b` would otherwise be a name already
	// holding a compound value and lose the letter to its first literal.
	return ok && len(a) > 0
}

// compoundMeetingAnAttribute is what an attribute a declaration has just
// added makes of a compound value the name is already holding.
//
// Three answers where the scalar question has two, and the two shells that
// share the scalar answer disagree with each other about this one — see
// Semantics.CompoundAttributePolicy for the measurements and for why it is a
// second question rather than a widening of the first.
//
// The letter matters for one of the three answers and for that one only. A
// shell that replaces the compound with a scalar does so because the letter
// changed what *kind* of name it is, and a case letter changes no kind: in
// that dialect `arr=(a b); typeset -u arr` still reads `a b` with two
// elements where `typeset -i arr` leaves one. So the replacement is read for
// the type letter and the other two answers for every letter alike.
// The result says the compound value was discarded and the declaration
// should carry on as though the name had been holding nothing.
func (r *Runner) compoundMeetingAnAttribute(name string) (startedOver bool) {
	switch r.compoundAttribute() {
	case CompoundAttributeKeepsTheElements:
		// The elements stand and the attribute waits for the next write,
		// which is the same answer this shell gives a scalar. Written out
		// rather than left to the default so that all three answers are
		// visible here; nothing can tell it from the default, which is what
		// a mutation of the case label showed.
	case CompoundAttributeFoldsEveryElement:
		if a, ok := r.assocFor(name); ok {
			r.foldAssocElems(name, a)
			return false
		}
		// storeArray is not used, deliberately: it asks
		// CompoundElementsGoThroughTheAttribute, which is a question about a
		// *write*, and this is not one. Both fields answer yes in the one
		// dialect that folds here, so routing through it would pass every
		// test and make one field answer for two.
		r.Arrays[name] = r.foldedElems(name, r.Arrays[name])
	case CompoundAttributeReplacesItWithAScalar:
		if !r.integer[name] {
			// A case letter changes no kind, so there is nothing to replace
			// the compound with. Measured in the one dialect that answers
			// this way: the array keeps its length and its elements.
			return false
		}
		r.compoundBecomesAScalar(name)
		return true
	}
	return false
}

// foldAssocElems folds every element of a keyed table in place, under its own
// key. Written out rather than routed through setAssocElem for the reason
// compoundMeetingAnAttribute gives: that path asks the write question.
func (r *Runner) foldAssocElems(name string, a AssocArray) {
	for k, v := range a {
		folded, ok := r.attributeFolded(name, v.scalar())
		if !ok {
			// The evaluation failed and has said so; the table is left as it
			// stands rather than half rewritten.
			return
		}
		a[k] = Scalar(folded)
	}
}

// compoundBecomesAScalar discards a compound value so the declaration can
// bring the name into being afresh as a scalar of the declared type.
//
// Nothing is stored here, which is the whole of why it is right: the value
// the name comes back holding is whatever a *fresh* declaration of it would
// leave, and that is DeclaredNameWithoutValueIsEmpty's to answer. The dialect
// that replaces answers that question yes, so the name comes back holding the
// empty string read through the integer attribute — 0. Measured, and it is 0
// for `(7 8)`, `(x y)` and `(0x10 9)` alike, so it is a fresh name and not a
// fold of anything the array held.
//
// The caller returns straight into those branches, so this only has to clear
// what stands in their way.
func (r *Runner) compoundBecomesAScalar(name string) {
	delete(r.Arrays, name)
	delete(r.AssocArrays, name)
	delete(r.Vars, name)
}

// rereadStandingValue applies an attribute a declaration has just added to the
// value the name was already holding, where the dialect says it reaches back.
//
// The two answers **both lose something**, which is why it is a field rather
// than a rule. A shell that re-reads destroys text: `FOO=bar; typeset -i FOO`
// evaluates `bar` as an expression, an unset name is 0, and 0 is what the name
// holds afterwards — measured in ksh93u+ and zsh 5.9.2, from a file and
// through `-c`, and in every zsh emulation. A shell that does not leaves a
// name declared integer holding text that is not a number — measured in bash
// 5.3.15, bash as `sh` and bash 3.2.57, which all still read `bar`. Neither
// reading keeps both promises.
//
// One question over every attribute that has something to say about a value:
// the shells that re-read `-i` fold `-u` and `-l` on the spot too — `d=MiXeD;
// typeset -u d` is MIXED in ksh93 and zsh and MiXeD in bash — and the shell
// that does not, does not. So the letters share an answer rather than each
// having one.
//
// Asked only where the two readings differ. An attribute with nothing to say
// about a value — `-x`, `-r`, `-a` — folds to itself and never gets here, and
// neither does a value the fold leaves alone, so `a=7; typeset -i a` needs no
// dialect.
//
// Not an assignment, which is why attributeFolded is called rather than
// setVarAs: a readonly name is re-read rather than refused, measured `typeset
// -r r=1; typeset -i r` as 1 at status 0. A scalar only — zsh keeps an array's
// elements as they are under both case letters, measured `a b` from `arr=(a
// b); typeset -u arr`, where ksh93 folds them; that divergence is recorded and
// not modeled.
// The result says whether the name was *started over* rather than read back:
// one of the three compound answers discards the value so that the
// declaration continues as though the name had been holding nothing, which is
// the caller's business and not this function's.
func (r *Runner) rereadStandingValue(name string) (startedOver bool) {
	if r.compoundNameHolds(name) && r.declaresAType(name) {
		// A compound value is a question of its own, with three answers
		// where the scalar one has two — see compoundMeetingAnAttribute. It
		// stands ahead of the scalar read rather than inside it because an
		// array keeps its first element in the scalar table, so a scalar
		// re-read here would quietly rewrite a copy nothing reads back and
		// leave the elements as they were.
		//
		// Asked only where one of the three letters that says something
		// about a value has arrived. Without one, all three answers are
		// "nothing happens" — so a bare `typeset -A m` must not be made to
		// ask, and it was: `markAssoc` puts the empty table there before
		// this runs, so every declaration of a table reached the question.
		return r.compoundMeetingAnAttribute(name)
	}
	// The value the name holds, wherever it is being held. A name the script
	// never assigned is still holding what it was started with, and reading
	// only the table skipped exactly that: `INHERITED=bar sh -c 'typeset -i
	// INHERITED; echo "[$INHERITED]"'` is `[0]` in zsh 5.9.2 and was `[bar]`
	// here, because the name lives in the inherited environment until
	// something writes it. Found by a mutant: dropping the `ok` guard changed
	// nothing any test could see, which is what said the guard was standing
	// in front of a case nothing reached.
	v, ok := r.Vars[name]
	if !ok {
		if v, ok = r.inheritedValue(name); !ok {
			// An exported name with no value anywhere. A compound value has
			// already been answered above, so there is nothing left here for
			// a scalar re-read to say anything about.
			return false
		}
	}
	if !r.attributeWouldChange(name, v) {
		return false
	}
	if !r.ask(r.sem().AttributeRereadsTheValueItFinds,
		"an attribute re-reading the value the name already holds") {
		return false
	}
	if folded, ok := r.attributeFolded(name, v); ok {
		r.Vars[name] = folded
	}
	return false
}

// attributeWouldChange reports whether re-reading a value through the name's
// attributes could give anything other than the value itself — the only case
// the two readings differ in, and so the only case worth a dialect.
//
// It has to answer **without evaluating**, because evaluating is exactly what
// the shell that does not re-read never does, and this engine's evaluation
// complains out loud. `FOO=08; typeset -i FOO` is the shape that proves it:
// the expression is a bad octal digit and the complaint ends the command,
// where bash reads `08` back with nothing said at all. Folding first to find
// out whether to ask made the question's own answer conditional on the
// dialect's, in the one direction that is loud.
//
// So the integer half asks a narrower question than the fold does: is this
// text already the canonical decimal spelling of itself? That is the one shape
// an integer attribute leaves alone, and anything else — `08`, `+7`, `3+4`,
// `bar`, an empty string — reads differently under the two shells whether it
// evaluates or not.
func (r *Runner) attributeWouldChange(name, value string) bool {
	if _, ok := r.floatPrecision[name]; ok {
		// The cheap half, the same shape the integer letter's is: a value
		// already written at the name's precision is one the fold would
		// return unchanged, and anything else — a plain `5`, a based
		// `16#FF`, a word that is no number at all — is not. Deliberately
		// not floatValue, which would run the arithmetic reader for a
		// question that only asks whether to bother, and which complains
		// where this must only answer.
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return true
		}
		// The rendering may itself refuse — a dialect that spells the `E`
		// letter and has not said what it means — and an unanswered axis is
		// not "the value would not change": the caller has to reach the
		// refusal rather than skip past it.
		text, ok := r.floatFormatted(name, v)
		if !ok || text != value {
			return true
		}
	}
	if r.integer[name] {
		n, err := strconv.Atoi(value)
		if err != nil || itoa(n) != value {
			return true
		}
	}
	if _, ok := r.fieldWidth[name]; ok && !r.caseFoldsOnRead() &&
		r.widthPadded(name, value) != value {
		// A width the name already carries, in the column that stores the
		// presentation: a declaration naming a different letter or a
		// different number re-presents what the name is holding. Measured
		// 2026-09-18 on ksh93u+ — `typeset -R 4 v=ab; typeset -L 4 v` reads
		// `ab  `, and `typeset -L 4 e=ab; typeset -L 6 e` reads `ab    ` —
		// while the same second line naming what the name already has
		// answers the value back and asks no dialect at all.
		return true
	}
	if r.caseFoldsOnRead() {
		// The case letters cannot change what is *stored* in this shell —
		// they act on every read instead — so there is nothing here for them
		// to say. Without this the two callers ask their dialect a question
		// whose answer can only be "store the value you already have". See
		// Semantics.CaseAttributeFoldsWhenRead.
		return false
	}
	switch {
	case r.lowered[name]:
		return strings.ToLower(value) != value
	case r.uppered[name]:
		return strings.ToUpper(value) != value
	}
	return false
}

// declaredNameHolds reports whether the name already has something to lose:
// a scalar, either array, or a value it was born with in the environment.
// A name `unset` took away holds nothing, however many attributes survive it.
func (r *Runner) declaredNameHolds(name string) bool {
	if r.removed[name] {
		return false
	}
	if _, ok := r.AssocArrays[name]; ok {
		return true
	}
	if _, ok := r.Arrays[name]; ok {
		return true
	}
	if _, ok := r.Vars[name]; ok {
		return true
	}
	_, ok := r.inheritedValue(name)
	return ok
}

// localExportAttribute answers whether the local a declaration just took
// inherits the export attribute of the name it shadows.
//
// The question is only there when the shadowed name is exported and a scope
// was actually taken; explicit says the declaration named the attribute
// itself, which answers it outright. Taking the attribute off is a change to
// a record the scope has to put back, so it is saved the way the value is.
func (r *Runner) localExportAttribute(name string, explicit bool) {
	if explicit {
		return
	}
	if len(r.scopes) == 0 {
		return
	}
	sc := r.scopes[len(r.scopes)-1]
	if _, shadowed := sc.saved[name]; !shadowed {
		// No scope was taken — a declaration at the top level, or one this
		// dialect gives no scope to — so there is no local to export.
		return
	}
	if !r.isExported(name) {
		// Nothing to inherit, and so nothing to disagree about.
		return
	}
	if r.ask(r.sem().LocalInheritsTheExportAttribute,
		"a local declaration inheriting the export attribute of the name it shadows") {
		return
	}
	if r.unspecified {
		return
	}
	if sc.exportedSpoken == nil {
		sc.exportedSpoken = map[string]bool{}
		sc.savedExported = map[string]bool{}
	}
	if _, seen := sc.exportedSpoken[name]; !seen {
		on, spoken := r.exported[name]
		sc.exportedSpoken[name] = spoken
		sc.savedExported[name] = on
	}
	if r.exported == nil {
		r.exported = map[string]bool{}
	}
	r.exported[name] = false
}

// shadowedExport remembers what an exported name held when a declaration took
// a scope in front of it.
//
// The shadowed binding does not stop being exported for having something
// standing in front of it, so a command is told its value for as long as the
// declaration has none of its own: the shell reads `${FOO-UNSET}` as unset
// inside the function and a child is still handed `FOO=bar`. Measured on both
// halves — an exported name and one that arrived in the environment — and on
// two levels, where what a child is told is the *caller's* local rather than
// the global behind it.
//
// Told what the name was rather than asking, because the attributes on this
// very declaration have been applied by the time a scope exists to record
// into: `FOO=bar; f() { local -x FOO; }` would otherwise read its own `-x`
// as the shadowed name's and hand a child a value no shell hands it. Which is
// the difference the measurement turns on — an exported name shadowed by a
// valueless declaration reaches a child, and an unexported one does not,
// whatever the declaration says about the local's own attribute. `+x` is the
// proof that it is the shadowed binding speaking and not the local: it takes
// the attribute off the local outright, and the child is still told the outer
// value.
//
// Reached only where a local carries the attribute of the name it shadows at
// all. The shell that answers LocalInheritsTheExportAttribute no tells a
// child nothing under that name, and it tells it nothing here either — a
// valueless declaration of an exported name is that same question and not a
// second one. The axis is read rather than asked because the declaration
// above has just asked it, and asking twice would report an unanswered one
// twice.
//
// Not written once per scope: a second declaration of the same name hides
// whatever the first one left, and the value that goes to a child is the one
// that was just taken away rather than the one the scope will put back.
func (r *Runner) shadowedExport(name string, exported bool) {
	if !exported || len(r.scopes) == 0 {
		return
	}
	if r.sem().LocalInheritsTheExportAttribute != Yes {
		return
	}
	sc := r.scopes[len(r.scopes)-1]
	if _, shadowed := sc.saved[name]; !shadowed {
		// No scope was taken — a declaration at the top level, or one this
		// dialect gives no scope to — so nothing is standing in front of
		// anything.
		return
	}
	value, ok := r.Vars[name]
	if !ok {
		if value, ok = r.inheritedValue(name); !ok {
			// Exported with no value anywhere — `export FOO` and nothing
			// more — and an exported name with no value reaches no child in
			// any shell measured.
			return
		}
	}
	if sc.exportedShadow == nil {
		sc.exportedShadow = map[string]string{}
	}
	sc.exportedShadow[name] = value
}

// hideVar takes a name out of view entirely — the tables and the environment
// fallback alike — so `${name-UNSET}` fires its default. The caller's shadow
// is what brings the outer value back.
func (r *Runner) hideVar(name string) {
	delete(r.Vars, name)
	if r.removed == nil {
		r.removed = map[string]bool{}
	}
	r.removed[name] = true
}

// declarationShadowRefused reports whether a declaration of a frozen name is
// refused, having said so and having decided what the refusal costs.
//
// Asked before the shadow, because the two answers are about whether the
// shadow happens at all — see Semantics.DeclarationMayShadowAReadonly. Where
// the shadow is allowed the attribute travels with the value and this reports
// nothing; where it is not, the refusal is the one an assignment to a frozen
// name already makes, in the declaration's form so that the builtin's name
// reaches the sentence.
//
// One gate for `local` and for `typeset`/`declare` alike, because bash refuses
// all of their spellings identically and zsh takes all of them. `local` used
// to reach the refusal only through the assignment, which got three separate
// things wrong: the valueless form met no check at all and shadowed the frozen
// name in silence, the message lost the builtin's name, and the failure
// abandoned the rest of the function where bash runs it (#1159).
func (r *Runner) declarationShadowRefused(name string) bool {
	if len(r.scopes) == 0 || !r.readonly[name] {
		// Not a declaration into a scope, or nothing frozen to shadow.
		// Assigning to a frozen name at top level is the ordinary refusal
		// and is not this question.
		return false
	}
	if r.ask(r.sem().DeclarationMayShadowAReadonly, "a declaration shadowing a readonly name") {
		return false
	}
	if r.unspecified {
		return true
	}
	if _, enclosing := r.enclosingShadowOf(name); enclosing {
		// The freeze is a *calling* function's, held by a binding that
		// call's own declaration put there — which the check above cannot
		// see, because it reads the attribute off the name. A second
		// question, asked only here, and the column that refuses a frozen
		// shadow at all still splits on it. See
		// Semantics.DeclarationMayShadowAnEnclosingScopesReadonly.
		if r.ask(r.sem().DeclarationMayShadowAnEnclosingScopesReadonly,
			"a declaration shadowing a freeze a calling function's own declaration holds") {
			return false
		}
		if r.unspecified {
			return true
		}
	}
	return r.refuseReadonly(name, assignedByDeclaration)
}

// shadowTypeset is shadow for `typeset`, which unlike `local` does not always
// get a scope to declare into.
//
// ksh93 is the reason: only a function defined with the `function` word has
// one, and in a POSIX-style function the assignment is ordinary and reaches
// the caller. The axis is asked only when the two answers differ — inside a
// keyword-defined function they do not, so that case needs no dialect.
//
// The result reports whether this call is what took the scope's copy — see
// shadow, and declareEmpty, which is the one caller that needs to know.
func (r *Runner) shadowTypeset(name string) (fresh bool) {
	if len(r.scopes) == 0 {
		return false
	}
	if !r.scopes[len(r.scopes)-1].keyword &&
		r.ask(r.sem().TypesetLocalNeedsKeywordFunction, "`typeset` needing a keyword-defined function to declare a local") {
		return false
	}
	return r.shadow(name)
}

// shadow saves a name in the innermost scope so the function's exit puts it
// back, which is what makes a declaration local.
//
// The result reports whether the copy was taken *here*: with it, the cell the
// declaration is about to write is new and holds nothing, whatever the outer
// name held. A second declaration of the same name in the same scope finds
// the copy already made and is writing over a cell that is its own.
func (r *Runner) shadow(name string) (fresh bool) {
	if len(r.scopes) == 0 {
		return false
	}
	sc := r.scopes[len(r.scopes)-1]
	if _, seen := sc.saved[name]; !seen {
		fresh = true
		// A live assignment prefix is holding this name, so the value the
		// cell below displaces is the prefix's and the declaration is taking
		// it into a scope of its own. Recorded before anything is saved, so
		// the declaration's own attributes reach the new cell rather than the
		// outer one. See Runner.prefixEntryShadowed.
		fromPrefix, tookThePrefixEntry := r.prefixEntryShadowed(name)
		old, existed := r.Vars[name]
		sc.saved[name] = old
		sc.existed[name] = existed
		if sc.removedBefore == nil {
			sc.removedBefore = map[string]bool{}
		}
		sc.removedBefore[name] = r.removed[name]
		// The frozen attribute is displaced with the value, in the dialect
		// that lets a declaration shadow one — the caller checked the axis
		// before getting here. Saved so the outer name is frozen again on
		// return: a function that thawed a readonly for good would be a hole
		// in the whole point of the attribute.
		//
		// Recorded whether or not there was a freeze, because the same entry
		// is what takes back a freeze this *call* adds — see
		// scope.savedReadonly. The delete below is unconditional for the
		// same reason there is no `if` around it: a name with nothing
		// recorded is a delete of a key that is not there.
		if sc.savedReadonly == nil {
			sc.savedReadonly = map[string]bool{}
		}
		sc.savedReadonly[name] = r.readonly[name]
		if !r.freezeSurvivesAShadow(name) {
			delete(r.readonly, name)
		}
		// And the hide-in-scope attribute, for the same reason and with the
		// same two jobs: the outer name gets back what it carried, and an
		// attribute this call added or removed goes away with the call.
		// Recorded whether or not there was one, so absent means this scope
		// never shadowed the name — see scope.savedHideInScope.
		if sc.savedHideInScope == nil {
			sc.savedHideInScope = map[string]bool{}
		}
		sc.savedHideInScope[name] = r.hideInScope[name]
		// And whether *this* shadow is a hidden one, which is a different
		// question from the attribute being saved above: what governs the
		// shadow is what the binding it displaced carried, and the fresh
		// binding carries nothing unless the declaration writes `-h`.
		// Measured — `typeset -h v=1` is `scalar-hide` and a plain `local v`
		// inside a function is `scalar-local`, with no `hide` in the word.
		// So the answer is recorded here and the attribute comes off with
		// every other one; shadowIsHidden is what reads it back.
		if sc.hiddenShadow == nil {
			sc.hiddenShadow = map[string]bool{}
		}
		hidden := r.shadowIsHidden(name)
		sc.hiddenShadow[name] = hidden
		delete(r.hideInScope, name)
		if hidden {
			// And the producer, which is what makes the shadow an ordinary
			// parameter rather than a second view of the shell's own: the
			// cell this declaration is about to write is where the value
			// comes from now (#2586). After freezeSurvivesAShadow above,
			// which asked the outer binding's question and has been answered.
			r.suspendProducer(sc, name)
		}
		// And the rest of the attributes, taken off for the same reason the
		// frozen one is: the cell this declaration writes is a fresh binding
		// and carries nothing the outer name carried. Recorded whether or
		// not there were any, so the entry can put back "none" as well as
		// take back one this call added — see localattributes.go.
		if sc.savedAttrs == nil {
			sc.savedAttrs = map[string]nameAttributes{}
		}
		sc.savedAttrs[name] = r.captureAttributes(name)
		r.dropNameAttributes(name)
		// And the export attribute, which is an attribute like every other
		// one above and was the one this list did not have. Not taken off
		// here — whether a local inherits it is a dialect's answer and
		// localExportAttribute asks it — but *recorded*, so that whatever
		// the declaration then does to the record goes away with the call.
		//
		// Without the record, `-x` written on a declaration inside a
		// function wrote a global fact. Measured 2026-09-15 against bash
		// 5.3.20, bash 3.2 and ksh93, all three of which answer that the
		// attribute is the local's and lasts as long as the call:
		//
		//	f(){ declare -x A=1; }; f; declare -p A     A: not found, 1
		//	A=outer; f(){ declare -x A=1; }; f          A reaches a child
		//	export B=o; f(){ declare +x B; }; f         B reaches none
		//
		// The first left `declare -p` answering 0 for a name that does not
		// exist. The other two are the worse half: a function's own
		// declaration decided what the *caller's* variable put in a child's
		// environment, in both directions, long after the call returned.
		//
		// The dialect that reads `-x` as `-g` never arrives here — the
		// letter takes no shadow there, which is what the letter means — so
		// this is inert for it and its own answer is unchanged.
		if sc.exportedSpoken == nil {
			sc.exportedSpoken = map[string]bool{}
			sc.savedExported = map[string]bool{}
		}
		if _, seen := sc.exportedSpoken[name]; !seen {
			on, spoken := r.exported[name]
			sc.exportedSpoken[name] = spoken
			sc.savedExported[name] = on
		}
		// And what a produced parameter was last assigned, which is a store
		// of its own: a declaration that shadowed Vars alone left the message
		// to the producer standing, so the local's value went on being read
		// after the call had returned. See scope.savedAssigned.
		if sc.assignedSpoken == nil {
			sc.savedAssigned = map[string]string{}
			sc.assignedSpoken = map[string]bool{}
		}
		sc.savedAssigned[name], sc.assignedSpoken[name] = r.assigned[name]
		delete(r.assigned, name)
		if tookThePrefixEntry {
			// The cell is the prefix's now, so what this scope gives back on
			// return is what the *prefix* would have given back and not the
			// prefix's own value — which is the state saved before the
			// command applied it. Written over the entries above rather than
			// instead of them, so every other attribute is still the outer
			// name's. See Runner.prefixEntryShadowed.
			r.scopeTakesOverThePrefixEntry(sc, name, fromPrefix)
		}
		if name == "OPTIND" {
			// And the half of the `getopts` position that is not a
			// parameter. Here rather than beside the builtin because this is
			// what *makes* a name local, and the cursor is local exactly when
			// the number is — see shadowGetoptsCursor.
			r.shadowGetoptsCursor(sc)
		}
		if r.memberNamesInUse {
			// And the names *under* this one, which a declaration displaces
			// with it: a compound variable's members are ordinary names
			// spelled with a dot, so shadowing the bare name alone left them
			// in the caller's scope for the body to write. See
			// compoundlocal.go, and the gate that keeps it inert in the four
			// dialects with no such spelling.
			r.shadowCompoundNamespace(sc, name)
		}
	}
	// Arrays live in a table of their own, so a name has to be saved from
	// both. Saving only the scalar left `f() { local a; a=(x y); }` writing a
	// global array: `local` shadowed nothing an array assignment then wrote
	// to, and the value outlived the function.
	if _, seen := sc.arrayExisted[name]; !seen {
		old, existed := r.Arrays[name]
		if sc.savedArrays == nil {
			sc.savedArrays = map[string]Array{}
			sc.arrayExisted = map[string]bool{}
		}
		// Copied rather than kept: an Array is a map, so saving the value
		// would save a reference to the very table the function is about to
		// write into, and putting it back would put back the changes.
		if existed {
			old = old.clone()
		}
		sc.savedArrays[name] = old
		sc.arrayExisted[name] = existed
	}
	// And a third table for the associative kind, for the same reason as the
	// second — and here the *attribute* is what is being shadowed as much as
	// the value: `typeset -A m` in a function must not leave the caller's
	// `m` reading its subscripts as strings.
	if _, seen := sc.assocExisted[name]; !seen {
		old, existed := r.AssocArrays[name]
		if sc.savedAssoc == nil {
			sc.savedAssoc = map[string]AssocArray{}
			sc.assocExisted = map[string]bool{}
		}
		if existed {
			old = old.clone()
		}
		sc.savedAssoc[name] = old
		sc.assocExisted[name] = existed
	}
	// And how the shadowed table came to be, which travels with the two
	// above: a local declaration marks the name declared-only, and the
	// caller's own table must not inherit that when the call unwinds. Keyed
	// off assocExisted, which the block above has just filled in for this
	// name, so the record is taken exactly once per name per scope.
	if _, seen := sc.declaredOnlyBefore[name]; !seen {
		if sc.declaredOnlyBefore == nil {
			sc.declaredOnlyBefore = map[string]bool{}
		}
		sc.declaredOnlyBefore[name] = r.declaredOnlyCompound[name]
		setBool(&sc.heldAnElementBefore, name, r.compoundHeldAnElement[name])
	}
	// And the other half of a tie the shell made for itself, which is one
	// value under two names and so cannot have one of them saved alone. See
	// tielocal.go.
	r.shadowTiedHalf(name)
	return fresh
}

// declarationUtilities are the commands whose `name=value` arguments are
// assignments rather than ordinary words.
//
// The list is the core's: `export` and `readonly` are POSIX, and `local` and
// `typeset` are here because every shell in the panel that has them treats
// them the same way. A dialect adds its own names with SetDeclaring — bash and
// zsh add `declare` — because the rule has to follow the name into the shell
// that has it and must not apply in the shell that does not, where the same
// word is an ordinary command.
var declarationUtilities = map[string]bool{
	"export": true, "readonly": true, "local": true, "typeset": true,
}

// SetDeclaring makes a command's `name=value` arguments expand as assignments.
func (r *Runner) SetDeclaring(name string) {
	if r.declaring == nil {
		r.declaring = map[string]bool{}
	}
	r.declaring[name] = true
}

func (r *Runner) declares(name string) bool {
	return declarationUtilities[name] || r.declaring[name]
}

// assignShaped reports whether a word begins with a literal `name=` — where
// the name may carry a subscript.
//
// The name has to be literal: `$x=1` is not an assignment in any shell, and
// neither is `"a"=1`. Only what the word says before any expansion counts.
//
// A subscript is part of the name, which is what keeps `typeset a[1]=v` off
// the pattern path: measured with a file literally named `a1=v` on disk, all
// six columns still assign the element and none of them matches the file, so
// the operand is an assignment there as much as `n=3` is. Reading only span
// zero could not see it — `a[$i]=v` reaches this as three spans, and the `=`
// that ends the name is in the last of them.
func assignShaped(w *syntax.Word) bool {
	_, _, ok := assignNameSplit(w)
	return ok
}

// assignNameSplit finds the `=` that ends a declaration operand's name and
// answers where it is: the span it lives in and its offset within that span.
//
// The plain half of declarationNameSplit, which is where the walk and the
// rules it enforces are written down. An **appending** operand is the other
// half and is appendNameSplit: the two cannot be one predicate, because
// assignNameSplit is shared with keywordPromotable — see there.
func assignNameSplit(w *syntax.Word) (span, off int, ok bool) {
	span, off, appends, ok := declarationNameSplit(w)
	return span, off, ok && !appends
}

// declarationNameSplit finds the `=` that ends a declaration operand's name,
// answers where it is — the span it lives in and its offset within that span
// — and whether the character in front of it was the append operator's `+`.
//
// Only an unquoted literal `=` counts, and only one outside the brackets: the
// subscript is part of the name, so `a[k=1]=v` is the element `k=1` of `a`
// rather than a name of `a[k`. Depth is tracked rather than assumed, because
// a subscript is an expression and an expression may index another array.
//
// Everything that is not an unquoted literal is allowed only inside the
// brackets. That is what still refuses `$x=1` and `"a"=1` while taking
// `a[$i]=v` and `m["k"]=v`, which is the split every shell in the panel makes.
//
// The `+` is taken only immediately in front of the `=` and only once, so
// `a[1]+=v` and `x+=v` are appends while `a+b=v`, `x++=v` and `a[1]+x=v` are
// not names at all. It is reported rather than folded into the name because
// the two readings have different callers and a different axis: see
// appendNameSplit.
func declarationNameSplit(w *syntax.Word) (span, off int, appends, ok bool) {
	if w == nil || len(w.Spans) == 0 {
		return 0, 0, false, false
	}
	head := w.Spans[0]
	if head.Kind != syntax.Literal || head.Quoting != syntax.Unquoted {
		return 0, 0, false, false
	}
	depth, closed, plus, name := 0, false, false, ""
	for i, s := range w.Spans {
		if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
			if depth == 0 {
				return 0, 0, false, false
			}
			continue
		}
		for j := 0; j < len(s.Value); j++ {
			c := s.Value[j]
			if depth == 0 && plus && c != '=' {
				// The `+` is the operator only where the `=` follows it.
				// `x+y=v` is a name that is not one and `x++=v` is nothing
				// in any column, so the word goes back to being ordinary.
				return 0, 0, false, false
			}
			switch {
			case c == '[':
				if depth == 0 && closed {
					// One subscript and no more: `a[1][2]=v` is not a name
					// this engine knows, and neither is `a[1]x=v`, so the
					// word goes back to being an ordinary operand.
					return 0, 0, false, false
				}
				depth++
			case c == ']':
				if depth > 0 {
					depth--
					closed = depth == 0
				}
			case c == '+' && depth == 0:
				plus = true
			case c == '=' && depth == 0:
				// The one name test, and it is enough for the bracket as
				// well: nothing is added to `name` once a subscript has
				// closed, so the text judged here is exactly the text in
				// front of the `[`. A second test there could only refuse
				// what this one refuses — `[1]=v` and `a-b[1]=v` both arrive
				// with the same `name` either way — which is why there is
				// not one.
				if !isPlainName(name) {
					return 0, 0, false, false
				}
				return i, j, plus, true
			default:
				if depth == 0 {
					if closed {
						return 0, 0, false, false
					}
					name += string(c)
				}
			}
		}
	}
	return 0, 0, false, false
}

// expandAssignArg expands `name=value` given to a declaration utility,
// expanding the value as an assignment's and the name as the name it is.
//
// The name half is expanded rather than taken as written because a subscript
// may hold one — `typeset a[$i]=v` names an element every shell in the panel
// works out first — and it is expanded as an assignment's *value* is, so the
// brackets are never a pattern and the text is never split.
func (r *Runner) expandAssignArg(w *syntax.Word) string {
	i, j, ok := assignNameSplit(w)
	if !ok {
		// An appending operand, `x+=v`, whose name assignNameSplit refuses
		// and appendNameSplit takes. The `+` stays on the name half, which
		// is what the utility reads the operator off.
		i, j, ok = appendNameSplit(w)
	}
	if !ok {
		// Never, from either caller: both asked one of those two questions.
		return r.expandAssignValue(w)
	}
	name := *w
	name.Spans = append(append([]syntax.Span{}, w.Spans[:i]...), syntax.Span{
		Kind: syntax.Literal, Value: w.Spans[i].Value[:j],
		Quoting: w.Spans[i].Quoting, Pos: w.Spans[i].Pos,
	})
	value := *w
	value.Spans = append([]syntax.Span{{
		Kind: syntax.Literal, Value: w.Spans[i].Value[j+1:],
		Quoting: w.Spans[i].Quoting, Pos: w.Spans[i].Pos,
	}}, w.Spans[i+1:]...)
	return r.expandAssignName(&name) + "=" + r.expandAssignValue(&value)
}

// expandAssignName expands the name half of a declaration's operand.
//
// As an assignment's value is expanded — never split, never matched against
// the filesystem — because a subscript may hold an expansion and everything
// else in the name half is literal by the time it gets here. A fast path for
// the wholly literal name was written first and then removed: it could not be
// told from this, since a literal word expands to itself, so it was a branch
// no mutation could kill.
func (r *Runner) expandAssignName(w *syntax.Word) string {
	return r.expandAssignValue(w)
}

// declarationCarriesAnArrayLiteral reports whether this operand's value is an
// array literal the command machinery is holding aside — `typeset a=(x y)`,
// where the parser hands the builtin the bare name and lands the parentheses
// through Runner.assignOperands.
//
// The declaration is *not* valueless, which is the whole of what this is for.
// The loop reads `hasValue` off a `name=value` word, so an array literal reads
// as nothing there and the name went to declareEmpty — a store of the empty
// string, to a name the very next step assigns an array to. Harmless while
// nothing refused it, and two sentences the moment something did:
// `readonly q; typeset -g q=(b)` wrote `read-only variable: q` once for the
// empty this branch stored and once for the array the operand landed (#2250).
//
// Every other reader of the same fact already spells it this way beside
// hasValue — see compoundKindChanged and typeLetterOverAnArrayLiteralRefused
// — so this is that condition given a name rather than a new rule.
func (r *Runner) declarationCarriesAnArrayLiteral(name string) bool {
	return r.literalOperands[name]
}

// functionDefinitionLine is the line a function's definition begins on, for
// the listing that reports where a function came from.
//
// The tree already carries it, so this is a reading rather than a record: a
// definition the parser read knows its own position, and one a builtin made
// from text was parsed too. Measured on bash 5.3.15, 2026-09-13, with a blank
// first line in the file: `g` defined on line 2 answers 2, so it is the
// definition's own line and not the line the file was sourced from.
func (r *Runner) functionDefinitionLine(name string, fn *syntax.FuncDecl) int {
	if fn == nil {
		return 0
	}
	if text, ok := r.undefinedFunction(name); ok && text != "" {
		// A body that has not been read yet has no position worth naming;
		// the placeholder is the shell's own text. One is what a listing
		// under this option is least likely to be asked about, and zero is
		// what the shell that has the option writes for a function it has
		// no line for.
		return 0
	}
	return int(fn.Pos().Line)
}

// functionDefinitionFile is the file a function was defined in, for the same
// listing.
//
// Runner.funcOrigins is the record and it is already kept for every route that
// defines one. The fallback is the shell's own name, which is measured rather
// than chosen: a function defined in a `-c` string or on standard input is
// reported by bash 5.3.15 against the shell's own path, and a function from a
// script names that script exactly as the invocation spelled it — `./s.sh`
// and `s.sh` are two answers to the same question there.
//
// Not Runner.functionOrigin, which answers the neighboring question for a
// different shell and has a different answer for standard input: that one
// reports *no* origin there, because the sentence it feeds leaves the clause
// off entirely. Folding the two would make one of them wrong.
func (r *Runner) functionDefinitionFile(name string) string {
	if file := r.functionFile(name); file != "" {
		return file
	}
	return r.name()
}

// numericTypeLetterCompany reports whether a declaration wrote the integer
// letter and the **exponent** float letter both.
//
// Read off the letters as written rather than off the flags, because the
// flags are where the parse has already discarded the loser: `-iE` leaves
// f.float false and `-Ei` leaves both set, so a test on the flags would
// answer the two orders differently where the shell that refuses them does
// not.
//
// `F` is deliberately not here, and that is measured rather than an
// oversight. The refusal is the `E` letter's alone: measured 2026-09-14 on
// ksh93u+ 2012-08-01, `typeset -iE 3 a=1.5` and `typeset -Ei 3 a=1.5` are
// both typeset's usage block at 2, where `typeset -iF 3 a=1.5`,
// `typeset -Fi 3 a=1.5` and `typeset -i -F 3 a=1.5` all declare a float and
// list `typeset -F 3 a=1.500`. Reading the pair as "the integer letter and
// *a* float letter" was right for the only float letter that column had
// when the axis was written, and refuses a line it runs now that it has two
// (#2419).
//
// Which letter wins where the pair *is* taken is the parse's and not this
// function's, and it is the same answer either way round: the same run has
// `typeset -EF 3 a=1.5` and `-FE 3 a=1.5` both listing `typeset -E 3 a=1.5`,
// so the exponent letter wins over the other two wherever it stands.
func numericTypeLetterCompany(f declareFlags) bool {
	return strings.ContainsRune(f.letters, 'i') && strings.ContainsRune(f.letters, 'E')
}

// declarationRefusalNamesTheOperand reports whether a refusal a declaration
// reaches **through a reference** is spoken of under the name the script
// wrote rather than the one it landed on.
//
// The redirect itself is not in question — the freeze that refuses is the
// *target's*, and the row that says so is the one where the **reference** is
// the frozen one: measured 2026-09-18 on bash 5.3.20, `u=1; declare -rn s=u;
// declare s=9` writes 9 into `u` at 0, so a frozen reference is walked
// straight past. Only the name in the sentence moves.
//
// And only for a declaration carrying a **value**, which is the pair that
// makes it a rule rather than a wording table. With `u=1; readonly u;
// declare -n s=u`, measured the same day, `env -i` with a scratch HOME, from
// a file:
//
//	declare -i s      declare: u: readonly variable   the target
//	declare s=9       declare: s: readonly variable   the reference
//	declare -i s=9    declare: s: readonly variable   the reference
//	declare -a s=9    declare: s: readonly variable   the reference
//	declare s+=9      declare: s: readonly variable   the reference
//	u=9               u: readonly variable            the target
//	s=9               u: readonly variable            the target
//	export s=9        u: readonly variable            the target
//	readonly s=9      u: readonly variable            the target
//	local s=9         local: u: readonly variable     the target
//
// So it is the `declare`/`typeset` word and the value together; every
// neighbor names the cell the write would have landed in. Every row is status
// 1 in both shells and no write happens in either, so the name is the whole
// of the difference.
//
// Asked only where a declaration with a value really redirected through a
// reference, so a dialect without references never meets it. ksh93 spells one
// and answers the other way — `typeset s=9` through a reference to a frozen
// `u` is `u: is read only` there — which is what makes this a field.
func (r *Runner) declarationRefusalNamesTheOperand() bool {
	return r.ask(r.sem().DeclarationThroughAReferenceNamesTheOperand,
		"a declaration's refusal through a reference naming the operand")
}

// namerefLetterIsDropped reports whether an `n` letter beside `a` or `A` is
// ignored rather than refused.
//
// Measured 2026-09-17 on bash 5.3.20, `env -i` with a scratch HOME, from a
// file — the split is the binding and not the builtin's name:
//
//	declare -an t         at the top level      the array, silently, at 0
//	typeset -an t         at the top level      the same
//	declare -na t=v       at the top level      declare -a t=([0]="v")
//	f(){ declare -g -an d; }    global by -g    the array, silently, at 0
//	f(){ declare -an d; }       local           `d: reference variable cannot
//	f(){ local -an l; }         local            be an array` at 1
//	f(){ typeset -an t; }       local            the same
//
// So a declaration that makes a **local** binding refuses the pair, which is
// what this shell did everywhere, and one that makes a global binding keeps
// the array and drops the reference. The name still ends up carrying the
// array attribute in both columns; what differs is the complaint and the
// status, and — where a value was given — whether it is stored.
//
// Core rather than an axis: bash is the only shell on the panel that spells
// both letters. ksh93 has `nameref` and no `-n`, and its `typeset -an` is its
// usage block at 2.
func (r *Runner) namerefLetterIsDropped(df declareFlags) bool {
	if !df.array && !df.assoc {
		return false
	}
	// A local binding is what refuses: inside a function, and not sent to the
	// global table by `-g`.
	return df.global || len(r.scopes) == 0
}
