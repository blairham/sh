// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// `typeset -H`, the fourth letter of a declaration builtin that two shells
// spell alike and neither means what the other does — `-m`, `-M` and `-T` are
// the other three, in interp/declarematching.go, interp/declaremapping.go and
// interp/declaretype.go. zsh's withholds a name's *value* from a listing;
// ksh93's is an attribute that is recorded, written back with its letter, and
// changes nothing else at all.
//
// Measured 2026-09-13 on ksh93u+ 2012-08-01 and zsh 5.9.2, `env -i` with a
// scratch HOME and no startup files.
//
// **The value is never withheld.** `typeset -H h=hid` reads back as `[hid]`
// in both shells, which is the half they agree on. They part at every
// listing: `typeset -p h` is `typeset -H h=hid` in ksh93 and `typeset h` in
// zsh, and a bare `set` writes `zzh=hid` there against zsh's bare `zzh`. So
// one shell says the attribute by *naming the letter* and the other by
// *omitting the value*, and neither ever does the other's.
//
// **`+H` takes the attribute off and there was never anything to give back.**
// `typeset -H h=hid; typeset +H h; typeset -p h` is `h=hid` — the plain form,
// because a ksh93 name with no attribute left lists without the word.
//
// **The letter is the name's and outlives a write.** `typeset -H h=1; h=2;
// typeset -p h` is `typeset -H h=2`, and a name with the attribute and no
// value lists as `typeset -H q`. It goes away with the name: after `unset h`,
// `typeset -p h` writes nothing and reports 0.
//
// **Where the letter stands among the others**, measured a pair at a time:
// `typeset -x -H h=1`, `typeset -r -H h=1`, `typeset -a -H h=(1 2)`, `typeset
// -A -H m=([k]=v)`, `typeset -H -l h=ab`, `typeset -H -u h=AB` and all of them
// at once as `typeset -x -r -H -u h=AB`. So it follows the export, readonly
// and kind letters and precedes the case letters — exactly where this engine's
// bare-assignment listing already has a gap, between `-A` and `-l`. The
// spelling on the *input* side decides nothing: `-Hl` and `-lH` list the same.
//
// **The letter and the integer attribute are exclusive.** `typeset -iH n=5` is
// that builtin's usage block at 2 with no complaint above it, and — `typeset`
// being one of that shell's special builtins — the script ends there. `integer
// -H n=5` is the same block by the same route, because that word is `typeset
// -li` in this shell and the conflict is with the attribute rather than with
// the second name. zsh takes `-iH` and lists `typeset -i n`, so the
// exclusivity belongs to this reading and not to the letter.
//
// **Not modeled, and recorded rather than left to be rediscovered**: in ksh93
// the letter shares a bit with the width and float letters, so `typeset -H
// -F2 h=1` lists as `typeset -E 2 h=1` and `typeset -H -L4 h=ab` drops the
// `H`. Those letters are still refused by name here (see
// Diagnostics.UnimplementedOptionLetters), so no line can reach the
// combination; #2419 is where they are implemented, and this is the note that
// says the combination is not a second `-H` question.

// DeclareHideValueLetterPolicy is what the `H` letter of a declaration
// builtin means to a dialect that spells it.
//
// The two readings agree about the value the name holds and disagree about
// every listing of it, so there is no reading that is nearly right: the
// unanswered value is refused by name rather than given one shell's.
type DeclareHideValueLetterPolicy int

const (
	// DeclareHideValueLetterUnspecified is no answer, which bash, dash and
	// ash hold and never reach: none of them spells the letter.
	DeclareHideValueLetterUnspecified DeclareHideValueLetterPolicy = iota
	// DeclareHideValueLetterHidesTheValue is zsh's: the name is declared and
	// holds what it holds, and only a listing that would have written
	// `=value` writes the bare name instead. The letter itself is never
	// written back.
	DeclareHideValueLetterHidesTheValue
	// DeclareHideValueLetterIsAnInertAttribute is ksh93's: the attribute is
	// recorded on the name and said back as `-H` in a listing that has words
	// for its attributes, value and all. Nothing else about the name changes.
	DeclareHideValueLetterIsAnInertAttribute
)

func (p DeclareHideValueLetterPolicy) String() string {
	switch p {
	case DeclareHideValueLetterHidesTheValue:
		return "hides the value from a listing"
	case DeclareHideValueLetterIsAnInertAttribute:
		return "an inert attribute, listed with its letter"
	}
	return "unspecified"
}

// DeclareHideInScopeLetterPolicy is what the lower-case `h` letter of a
// declaration builtin means to a dialect that spells it.
//
// The fifth letter two shells spell alike and read as two unrelated things,
// and the only one of the five whose readings differ about how many *words*
// the command has: one takes an argument and the other does not, so a line
// read the wrong way declares a name the other never saw.
type DeclareHideInScopeLetterPolicy int

const (
	// DeclareHideInScopeLetterUnspecified is no answer, which bash, dash and
	// ash hold and never reach: none of them spells the letter.
	DeclareHideInScopeLetterUnspecified DeclareHideInScopeLetterPolicy = iota
	// DeclareHideInScopeLetterHidesInScope is zsh's: a local declaration of
	// a name carrying the attribute is an ordinary parameter rather than the
	// special one it is spelled like, and the tie the name is half of goes
	// on without it. The letter takes no argument. See hideinscope.go.
	DeclareHideInScopeLetterHidesInScope
	// DeclareHideInScopeLetterTakesAString is ksh93's: the letter takes a
	// string — `[-h string]` in that shell's own usage block — and records
	// nothing at all. Measured 2026-09-14 on ksh93u+ 2012-08-01:
	//
	//	typeset -h "a string" q=1; typeset -p q    q=1
	//	typeset -h s q=1 r=2; typeset -p q r       q=1 and r=2
	//	typeset -h"s" q=1; typeset -p q            q=1
	//	typeset -hx s q=1; typeset -p q            q=1 — the `x` is the string
	//	typeset -h                                 `-h: string argument expected`
	//
	// The fourth row is what says the string may ride on the letter the way
	// a mapping name rides on `-M`: `-hx` is the letter with `x` behind it,
	// so the `x` is the argument rather than the export letter, and `q` is
	// **not** exported. The last is the refusal when nothing follows it.
	DeclareHideInScopeLetterTakesAString
)

func (p DeclareHideInScopeLetterPolicy) String() string {
	switch p {
	case DeclareHideInScopeLetterHidesInScope:
		return "hides a special name's specialness in scope"
	case DeclareHideInScopeLetterTakesAString:
		return "takes a string argument and records nothing"
	}
	return "unspecified"
}

// hideValueLetterCompany reports whether this declaration asks for the
// integer attribute beside the `H` letter, which the inert reading refuses
// with the builtin's usage block.
//
// The flag is asked rather than the letter, which is what makes `integer -H
// n=5` the same refusal as `typeset -iH n=5`: that word carries the attribute
// itself, so it reaches here with no `i` written anywhere. The same shape
// typeLetterCompany has beside it, and asking the flag is what keeps the rule
// in one place.
func hideValueLetterCompany(f declareFlags) bool { return f.integer }
