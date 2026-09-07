// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"
)

// `integer`, which is the declaration builtin under a second name with the
// integer attribute already decided.
//
// ksh93 and zsh have the word and bash and dash do not, which makes it a
// dialect's answer rather than an axis — the same shape `declare` has, and it
// is registered through the extension seam for the same reason. What it is
// *not* is a second implementation: an integer declaration is
// `Runner.declareNames` here as much as `typeset -i` is, so the attribute, the
// arithmetic a later assignment means, the function shadow, the readonly
// refusal and every option letter's meaning come from the one place. A copy
// would have drifted the first time one of those was measured again.
//
// zsh's own `add-zsh-hook` is why it exists: line 26 of that function file is
// `integer del list help`, so a shell without the word cannot run the thing
// every zsh startup file uses to install a hook.
//
// Three things the second name decides for itself, and each of them is a
// dialect's table rather than a branch here:
//
//   - which letters it takes, which is *narrower* than the declaration's in
//     both shells and narrowed differently — Semantics.IntegerOptions;
//   - whether `integer +i n` can take the attribute back off the name, which
//     the two shells answer opposite ways —
//     Semantics.IntegerNameForcesTheAttribute;
//   - what its refusals call it, which follows the invoked name through
//     r.inBuiltin the way every other builtin's do.

// IntegerBuiltin is the `integer` builtin, for a dialect that has the word to
// register.
//
// Exported for the same reason Builtin is: a dialect package cannot reach an
// unexported function, and the alternative — reimplementing the declaration
// in `dialect/ksh` and again in `dialect/zsh` — is two copies of the thing
// this package exists to hold once.
func IntegerBuiltin() Builtin { return biInteger }

func biInteger(r *Runner, _ context.Context, args []string) int {
	name := r.inBuiltin
	if name == "" {
		name = "integer"
	}
	known := r.sem().IntegerOptions
	args, f, code := r.parseDeclareFlags(name, args, known)
	if code != 0 {
		// The same fatality `typeset` has, and for the same reason: ksh93
		// counts its declaration builtins among the special ones, so an
		// honest refusal of a letter it spells and this shell does not stops
		// the script exactly where a bad option to `typeset` stops it.
		if r.ask(r.sem().TypesetBadOptionFatal, "a bad `typeset` option ending the script") {
			r.status = code
			r.fatalQuiet()
		}
		return code
	}
	if len(args) == 0 {
		// No names at all is a *listing* of the integer variables in both
		// shells that have the word, and the two do not even agree on which
		// names are in it — ksh93 writes the ones a script declared and zsh
		// includes its own specials. It is the filtered listing `typeset -i`
		// with no names is, which this engine does not build either, so it
		// refuses by name rather than falling through to the bare
		// declaration listing and answering with the whole variable table.
		r.diagf("%s: a listing is not implemented yet\n", r.builtinComplaintName(name))
		return 2
	}
	// The name asked for the attribute, so it is an addition however the
	// letters were written: `integer +x n` is still an integer declaration in
	// both shells, because the plus belongs to the export letter.
	f.integer = true
	if f.remove {
		// A plus word is the one place the two readings part company, and it
		// is the *word* rather than the `i` in it: in the shell where the
		// name carries the type, `integer +x n` leaves the export alone as
		// well. So the whole removal stands or falls together rather than the
		// integer half of it being singled out.
		off := r.ask(r.sem().IntegerPlusFormTakesAttributesOff,
			"a plus word on `integer` taking an attribute off the name")
		if r.unspecified {
			return r.status
		}
		f.remove = off
		// And where a plus form does remove, it reaches the attribute the
		// *name* asked for only when `i` is the letter that was written.
		f.integerForced = off && !f.integerOff
	}
	return r.declareNames(name, args, f)
}

// refuseIntegerBase is what `-i` with an output base says where the dialect
// gives the letter one.
//
// This engine records that a name is an integer and evaluates what is
// assigned to it; it has nowhere to keep a *base*, which is a property of how
// the value reads back out. So the base is named as missing instead of being
// read and dropped. Dropping it was the previous answer and was the silent
// kind of wrong: `typeset -i 16 n=255` reported 0 and left `255` standing
// where ksh93 prints `16#ff` and zsh prints `16#FF`, and the `16` went on to
// become a variable of its own.
//
// Where the dialect's `-i` takes no base — bash, whose `-i16` is an invalid
// option and whose bare `16` is not a valid identifier — nothing is refused
// and the word is left to mean what it meant.
func (r *Runner) refuseIntegerBase(builtin string) int {
	takes := r.ask(r.sem().IntegerAttributeTakesABase, "`-i` reading an output base")
	if r.unspecified {
		// An unanswered axis refuses rather than picking a shell, and the
		// status it set is the one that stands.
		return r.status
	}
	if !takes {
		return 0
	}
	r.diagf("%s: an output base is not implemented yet\n", r.builtinComplaintName(builtin))
	return 2
}

// hasAttachedIntegerBase is `-i16`: the integer letter with a base riding on
// it. The letter has to be one the dialect spells, or the word is somebody
// else's bad option and not a base at all.
func hasAttachedIntegerBase(word, known string) bool {
	if !strings.ContainsRune(known, 'i') {
		return false
	}
	at := strings.IndexByte(word[1:], 'i')
	if at < 0 {
		return false
	}
	return isAllDigits(word[at+2:])
}

// isAllDigits is a non-empty run of decimal digits, which is what an output
// base looks like and what a name may not be.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
