// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// `typeset -m` where the letter *moves* a parameter — the other of the two
// readings the `m` letter has, and nothing the first one does. See
// interp/declarematching.go for the pattern reading and for
// Semantics.DeclareMatchingLetter, which is the switch between them.
//
// Measured 2026-09-13 on ksh93u+ 2012-08-01, the shell that reads it this
// way, with a scrubbed environment and no startup files.
//
// **The operand is `new=old` and the parameter moves.** `qa=1; typeset -m
// qb=qa` leaves `qb` holding 1 and `qa` unset, and the kind travels with the
// value: an array arrives as an array and a table as a table. A source
// nobody defined is not an error — it *unsets* the destination, which is the
// same statement from the other side.
//
// **An operand with no `=` names the destination and reads the source out of
// its value.** `typeset qa=hello; typeset -m qa` moves `hello` to `qa`. That
// is why `qa=1; typeset -m qa` is `1: invalid variable name`: the source name
// is the *value* 1, not the operand. A name whose value is a based integer
// blames the based text — `typeset -i16 qa=255; typeset -m qa` is `16#ff` —
// which is what says the value is read the way a script reads it rather than
// off the store.
//
// **Both halves must be identifiers, and they are blamed differently.** A bad
// destination quotes the *whole operand* back, `q*=9: invalid variable name`;
// a bad source quotes only the source, `qa=1bad` is `1bad: invalid variable
// name`. Either is status 1 and, since this is a special builtin in that
// shell, it ends the script — so the operands after it never run, which is
// measured: `typeset -m '1bad=qa' 'qb=qa'` leaves `qb` unset.
//
// **The letter tolerates no other, with one exception.** Every attribute
// letter beside it — `-mx`, `-mi`, `-mf`, `-m -x` in two words, and the `-mi`
// that `integer -m` really is — answers the builtin's usage block at 2 with
// no complaint above it, which is the same shape a bad option gets there. The
// exception is `p`: `typeset -pm 'q*'` and `typeset -pm 'q*'=9` are a silent
// 0 that assigns nothing and lists nothing, where the same lines without the
// `m` assign and list. So `-p` does not conflict and does not act.
//
// **Attributes do not travel.** Measured: `typeset -x qa=1; typeset -m qb=qa`
// leaves `qb` unexported, and `typeset -i qa=7; typeset -m qb=qa` lists `qb`
// back with no integer word. The array *kind* does travel, which is the one
// property of the name that the value cannot be read without.

// declareMove is `typeset -m` under Semantics.DeclareMatchingLetter's second
// reading. The letters are settled first because they can refuse the line
// before an operand is looked at.
func (r *Runner) declareMove(name string, operands []string, f declareFlags) int {
	if r.moveLetterConflicts(f) {
		return r.refuseWithUsage(name)
	}
	if f.print {
		// `-p` beside the letter: measured a silent 0, whatever the operands
		// say. Not a listing and not a move — the one letter that neither
		// conflicts nor acts.
		return 0
	}
	fatal, _ := r.nameRules(name)
	for _, operand := range operands {
		to, from, written := strings.Cut(operand, "=")
		if !isPlainName(to) {
			// The whole operand, not the half that is wrong: `q*=9` is
			// blamed as `q*=9` and `1bad=qa` as `1bad=qa`.
			return r.badBuiltinName(r.builtinComplaintName(name), operand, to, fatal)
		}
		if !written {
			// The source is this name's *value*, read the way a script
			// reads it so that a based integer is blamed as the script
			// would see it.
			from, _ = r.getVar(to)
		}
		if !isPlainName(from) {
			// And here only the source is quoted back, which is the other
			// half of the measurement above.
			return r.badBuiltinName(r.builtinComplaintName(name), from, from, fatal)
		}
		if code := r.moveParameter(to, from); code != 0 {
			return code
		}
	}
	return 0
}

// moveLetterConflicts reports whether another letter on the line refuses the
// move outright. `p` is the exception and `m` is the letter itself; every
// other letter, and the `f` that is recorded apart from them, conflicts.
func (r *Runner) moveLetterConflicts(f declareFlags) bool {
	if f.function || f.funcNames {
		return true
	}
	return strings.ContainsFunc(f.letters, func(c rune) bool {
		return c != 'm' && c != 'p'
	})
}

// refuseWithUsage writes the builtin's usage block with no complaint above it
// and answers the bad-option status, ending the script where a special
// builtin's failure ends one.
//
// The shape a refusal takes when the *combination* is what is wrong rather
// than any one word: there is no letter to name, so naming one would point at
// a word the script wrote correctly.
func (r *Runner) refuseWithUsage(name string) int {
	r.builtinUsageLine(name)
	status := orDefault(r.diag().BuiltinBadOptionStatus, 2)
	// The same fatality a letter this builtin does not have gets, read from
	// the same axis: what ends the script is `typeset` failing, and the
	// combination is a failure of the option word as much as an unknown
	// letter is. Reached here rather than at parseDeclareFlags because the
	// letters parsed fine and only their company is wrong.
	if r.ask(r.sem().TypesetBadOptionFatal, "a bad `typeset` option ending the script") {
		r.status = status
		r.fatalQuiet()
	}
	return status
}

// moveParameter is the move itself: the value and the array kind travel, the
// attributes do not, and the source is left unset.
//
// A source that does not exist unsets the destination rather than being
// refused, which is measured — `qb=2; typeset -m qb=nosuch` leaves `qb`
// unset — and is the reading that makes a move of a moved name work.
func (r *Runner) moveParameter(to, from string) int {
	// The *source* is asked first and named in the refusal, which is
	// measured: `typeset -r qa=7; typeset -m qb=qa` is `typeset: qa: is read
	// only` and leaves `qb` alone. A move takes the source away, so a frozen
	// source refuses the whole operand and not only half of it.
	if r.refuseReadonly(from, assignedByDeclaration) || r.refuseReadonly(to, assignedByDeclaration) {
		return 1
	}
	switch {
	case r.AssocArrays[from] != nil:
		table := r.AssocArrays[from]
		r.unsetName(to)
		if r.AssocArrays == nil {
			r.AssocArrays = map[string]AssocArray{}
		}
		r.AssocArrays[to] = table
	case r.Arrays[from] != nil:
		a := r.Arrays[from]
		r.unsetName(to)
		r.storeArray(to, a)
	default:
		value, set := r.getVar(from)
		r.unsetName(to)
		if set {
			r.setVar(to, value)
		}
	}
	r.unsetName(from)
	return 0
}
