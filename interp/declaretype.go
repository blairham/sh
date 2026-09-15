// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"
	"strings"
)

// `typeset -T`, where the letter names a **type** rather than tying a scalar
// to an array.
//
// The third letter of a declaration builtin that two shells spell alike and
// neither means what the other does — `-m` and `-M` are the other two, in
// interp/declarematching.go and interp/declaremapping.go — so it is an axis
// and not a letter one dialect has. zsh's `-T` ties two names together (see
// interp/tiedscalar.go); ksh93's declares a type, and the two share no
// operand grammar at all.
//
// Measured 2026-09-13 on ksh93u+ 2012-08-01, `env -i` with a scratch HOME and
// no startup files.
//
// **The first operand is the type and the rest are variables of it, and no
// variable is created.** `typeset -T TS ts` is a silent 0 after which
// `typeset -p TS ts` writes nothing, `TS=a:b:c` leaves `ts` with no elements
// and `ts=(x y z)` leaves `$TS` empty. The two halves never meet, which is
// the whole of what the tie reading does and the whole of what this one does
// not.
//
// **The type name is remembered even with no definition.** `typeset -T ts`
// followed by `typeset -T` writes `typeset -T ts` back, and `typeset -T a b
// c` writes only `typeset -T a` — so the first operand is registered and the
// rest are not. A type name is not a parameter: `$ts` is empty, `typeset -p
// ts` is silent, and a bare `typeset` does not list it.
//
// **A bare `-T` is that listing, under either sign**: `typeset -T` and
// `typeset +T` write the same thing.
//
// **The name may ride on the letter instead.** `typeset -TPt v` registers
// `Pt` and takes `v` as a variable of it, so the rest of the option word is a
// name and not more letters — which is why `typeset -Tl TS ts` is a silent 0
// declaring a type called `l`, and not the lower-case attribute.
//
// **The letter tolerates no other, with one exception.** Because the rest of
// the word is the type's name, a second letter can only arrive in a word of
// its own or in front of the `T` — and `typeset -T -l TS ts`, `typeset -l -T
// TS ts` and `typeset -xT TS ts` are each the builtin's usage block at 2 with
// no complaint above it, which is the same shape the *move* letter's company
// gets. The exception is `p`: `typeset -pT` is a silent 0.
//
// **A type is defined by a compound assignment and by nothing else.** An
// operand carrying an ordinary value is `<name>: type definition requires
// compound assignment` at 1, and — `typeset` being one of that shell's
// special builtins — the script ends there. The complaint names the *first*
// operand that carries one, first position or last: `typeset -T TS=1 ts`
// blames `TS` and `typeset -T TS ts=1 tt=2` blames `ts`.
//
// **The first operand is the type and the rest are variables of it**, which
// is visible only in how a bad one is blamed. A first operand that is not an
// identifier is reported through the compound namespace that shell keeps its
// types in — `typeset -T ':' ts` is `typeset: .sh.type.:: no parent` — and a
// later one gets the ordinary refusal a declaration gives any bad name,
// `typeset -T TS ':'` and `typeset -T TS ts ':'` alike being `typeset: ::
// invalid variable name`. Both are 1 and both end the script.
//
// **`integer` does not take the letter**, and it falls out of the rule above
// rather than needing one of its own: that word is `typeset -li` in this
// shell, so `integer -T TS ts` is a `-T` with two letters in front of it and
// answers the same usage block at 2 that `typeset -li -T TS ts` does.

// DeclareTypeLetterPolicy is what the `T` letter of a declaration builtin
// means to a dialect that spells it.
//
// The two readings share no operand grammar: one takes a scalar, an array and
// a separator, the other takes a list of type names. There is no reading that
// is nearly right, so the unanswered value is refused by name rather than
// given one shell's.
type DeclareTypeLetterPolicy int

const (
	// DeclareTypeLetterUnspecified is no answer, which bash and dash hold and
	// never reach: neither spells the letter.
	DeclareTypeLetterUnspecified DeclareTypeLetterPolicy = iota
	// DeclareTypeLetterTiesAScalarAndAnArray is zsh's `typeset -T SCALAR
	// array [sep]`: see interp/tiedscalar.go, which is where that reading
	// lives.
	DeclareTypeLetterTiesAScalarAndAnArray
	// DeclareTypeLetterNamesAType is ksh93's: the operands name types, and a
	// type is defined by a compound assignment.
	DeclareTypeLetterNamesAType
)

func (p DeclareTypeLetterPolicy) String() string {
	switch p {
	case DeclareTypeLetterTiesAScalarAndAnArray:
		return "ties a scalar and an array"
	case DeclareTypeLetterNamesAType:
		return "names a type"
	}
	return "unspecified"
}

// declareType is `typeset -T` under the type reading. It answers the whole
// line: either the types are registered and their variables are names this
// engine has nothing to give them, or the line is refused, and there is no
// path through it that goes on to declare anything the ordinary way.
func (r *Runner) declareType(builtin string, operands []string, f declareFlags) int {
	if typeLetterCompany(f) {
		// Any other option *word* beside the letter is the usage block, and
		// it is the word rather than the letter: `typeset -Tl TS ts` is a
		// silent 0 because `l` is the type's *name*, while `typeset -T -l`,
		// `typeset -xT` and `typeset -l -T` are each the block at 2. `-p` is
		// the exception and rides along deciding nothing, which is the same
		// exception the move letter beside it has.
		return r.refuseWithUsage(builtin)
	}
	name, named, operands := f.typeName, f.typeNamed, operands
	if !named && len(operands) > 0 && !r.literalOperands[operands[0]] &&
		!strings.Contains(operands[0], "=") {
		// With nothing on the letter the *first* operand is the type and
		// the rest are variables of it, which is visible in what each is
		// blamed for: see refuseTypeNameHasNoParent.
		name, named, operands = operands[0], true, operands[1:]
		if !isPlainName(name) && name != "" {
			return r.refuseTypeNameHasNoParent(builtin, name)
		}
	}
	for _, operand := range operands {
		if r.literalOperands[operand] {
			// The compound assignment, which is the one form that does
			// something — it *defines* the type. A compound value is
			// #2620's work and this engine has no representation for one,
			// so the letter goes on saying so for this shape alone rather
			// than taking the line and quietly leaving an array behind.
			// Reached before the operand assignments run, so the array the
			// parser lifted out is never stored.
			return r.refuseTypeLetter(builtin)
		}
		if variable, _, valued := strings.Cut(operand, "="); valued {
			return r.refuseTypeNeedsACompound(builtin, variable)
		}
		if _, takes := r.nameRules(builtin); r.isBuiltinName(builtin, operand, takes) {
			continue
		}
		if r.unspecified {
			return 2
		}
		return r.badBuiltinName(builtin, operand, operand,
			r.sem().BadNameToDeclarationFatal)
	}
	if !named {
		// A bare letter is the listing of the types there are, under either
		// sign — measured, `typeset -T` and `typeset +T` write the same
		// thing. A type with no definition is written back as the bare
		// declaration that made it.
		for _, t := range r.declaredTypes {
			r.printf("typeset -T %s\n", t)
		}
		return 0
	}
	// A type this engine cannot define is a type it has nothing to record
	// beyond its name, and a name is not a variable: measured, `typeset -T
	// ts` leaves `$ts` empty, `typeset -p ts` silent and a bare `typeset`
	// without it. So the variables of the type are checked and dropped, and
	// only the type's own name is kept — for the listing above, which is the
	// one thing that can see it.
	if name != "" && !slices.Contains(r.declaredTypes, name) {
		r.declaredTypes = append(r.declaredTypes, name)
	}
	return 0
}

// typeLetterCompany reports whether anything was asked for beside the `T`
// letter that the type reading will not have.
//
// `p` is the one that rides along, measured: `typeset -pT` is a silent 0
// where every other letter beside the `T` is the usage block.
//
// The integer attribute is asked for as well as the letters, and that is the
// second builtin rather than a second rule: `integer` is this declaration
// under a word that carries the type itself, so `integer -T TS ts` reaches
// here with no `i` written anywhere and is the same conflict `typeset -i -T
// TS ts` is. Measured — both are the usage block at 2 — and asking the flag
// rather than the letter is what keeps the rule in one place.
func typeLetterCompany(f declareFlags) bool {
	if f.integer {
		return true
	}
	for i := 0; i < len(f.letters); i++ {
		if c := f.letters[i]; c != 'T' && c != 'p' {
			return true
		}
	}
	return false
}

// refuseTypeLetter says the letter is not implemented for the one shape that
// would have done something, in the same sentence an unimplemented letter
// gets everywhere else.
//
// The wording is Diagnostics.UnimplementedOptionLetters' own and is said in
// the one place that says it, so a reader meeting `-T` in a definition and
// `-T` in a bundle is told the same fact the same way.
func (r *Runner) refuseTypeLetter(builtin string) int {
	r.complainAboutOption(builtin, "%s: -T is not implemented yet\n",
		r.builtinComplaintName(builtin))
	status := orDefault(r.diag().BuiltinBadOptionStatus, 2)
	if r.ask(r.sem().TypesetBadOptionFatal, "a bad `typeset` option ending the script") {
		r.status = status
		r.fatalUsageQuiet()
	}
	return status
}

// refuseTypeNeedsACompound is the refusal of an operand carrying an ordinary
// value.
//
// The builtin is taken out of the location for it, which is measured rather
// than tidied: `typeset -T TS=1` is `<shell>: TS: type definition requires
// compound assignment` where `typeset -T TS ':'` two characters away is
// `<shell>: typeset: :: invalid variable name`. One word, two shapes, and
// badBuiltinName already does the same thing for the same reason.
func (r *Runner) refuseTypeNeedsACompound(builtin, name string) int {
	outer := r.inBuiltin
	r.inBuiltin = ""
	r.diagf("%s\n", Wording(r.diag().DeclareTypeNeedsACompoundAssignment,
		"%[1]s: type definition requires compound assignment", name))
	r.inBuiltin = outer
	return r.endDeclarationAfterARefusal(1)
}

// refuseTypeNameHasNoParent is the refusal of a first operand that is not an
// identifier, which the shell with the letter reports through the compound
// namespace it keeps its types in rather than as a bad variable name.
func (r *Runner) refuseTypeNameHasNoParent(builtin, operand string) int {
	r.diagf("%s\n", Wording(r.diag().DeclareTypeNameHasNoParent,
		"%[1]s: .sh.type.%[2]s: no parent",
		r.builtinComplaintName(builtin), operand))
	return r.endDeclarationAfterARefusal(1)
}
