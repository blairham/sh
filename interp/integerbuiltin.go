// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strconv"
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

// takesIntegerBase is whether `-i` reads an output base in this dialect —
// `typeset -i 16 n=255` and its attached spelling `-i16` — so that the word
// after the letter is a base and not a name.
//
// Where the dialect's `-i` takes none — bash, whose `-i16` is an invalid
// option and whose bare `16` is not a valid identifier — nothing is consumed
// and the word is left to mean what it meant.
func (r *Runner) takesIntegerBase() bool {
	takes := r.ask(r.sem().IntegerAttributeTakesABase, "`-i` reading an output base")
	if r.unspecified {
		// An unanswered axis refuses rather than picking a shell, and the
		// status it set is the one that stands.
		return false
	}
	return takes
}

// integerBaseAccepted reports whether the dialect will take this base,
// having complained if it will not.
//
// One shell complains and leaves the name with nothing — `invalid base (must
// be 2 to 36 inclusive): 64`, status 1, and the script carries on — and the
// other takes any base in silence and renders plain what it cannot spell. So
// the refusal is the presence of a wording rather than a second field: with
// none, an unspellable base is simply a base nothing is rendered in, which is
// also what base 10 is.
func (r *Runner) integerBaseAccepted(builtin string, base int, written string) bool {
	if r.validIntegerBase(base) {
		return true
	}
	wording := r.diag().IntegerBadBase
	if wording == "" {
		// Taken and rendered plain: measured, ksh93 prints `5` for both
		// `typeset -i1 f=5` and `typeset -i0 f=5` and says nothing.
		return true
	}
	r.diagf("%s\n", Wording(wording, "%[1]s: invalid base: %[2]s",
		r.builtinComplaintName(builtin), written))
	r.status = 1
	return false
}

// integerBaseTenIsNone reports whether ten is the absence of an output base
// in this dialect rather than a base of its own.
//
// Asked only where the answer changes something: where ten is written down,
// and where the integer letter arrives bare over a name that already has a
// base. Every other declaration reads the same either way, and the common
// path — `typeset -i n` on a name with no base — must not meet a question it
// does not need.
func (r *Runner) integerBaseTenIsNone() bool {
	return r.ask(r.sem().IntegerBaseTenIsNoBase,
		"base ten as the absence of an output base")
}

// spellsIntegerBase reports whether the dialect has digits for this base.
// Base 10 is spelled by every one of them and is the base nothing is written
// in, so it is not one of these.
func (r *Runner) spellsIntegerBase(base int) bool {
	// Base 10 is valid everywhere and marks nothing: measured, `typeset -i10
	// e=255` is `255` and `typeset -i10 e; e=0x1f` is `31` in both shells
	// that have the feature. So the two questions are not one — a base the
	// dialect *takes* and a base it writes a value in — and joining them
	// made the one shell that refuses a bad base refuse base ten as well.
	return base != 10 && r.validIntegerBase(base)
}

// validIntegerBase reports whether the dialect takes this base at all, which
// is the range its alphabet covers.
func (r *Runner) validIntegerBase(base int) bool {
	return base >= 2 && base <= len(r.sem().IntegerBaseDigits)
}

// integerRendered is a number written in a name's output base — `16#ff` for
// 255 under `-i16` — or in plain decimal where the name has no base, which is
// every name in a dialect without the feature and every name under base 10.
//
// The rendered text is what is *stored*, not a way of printing what is: every
// read sees these characters, `${#h}` counts them, a child is told them, and
// arithmetic parses them back. Measured in both shells that have the feature.
func (r *Runner) integerRendered(name string, v int) string {
	base := r.integerBase[name]
	if !r.spellsIntegerBase(base) {
		return itoa(v)
	}
	text, ok := r.baseRendered(v, base, true, 0)
	if !ok {
		return itoa(v)
	}
	return text
}

// baseRendered writes a value in a base, with the `base#` in front where
// prefixed and a `_` every group digits where grouping is on.
//
// One renderer for the two constructs that need it — the integer attribute
// and an arithmetic expression's output format — because they are the same
// spelling and disagreed when they were two: the attribute learns a base back
// out of the text it stored, so `(( x = [#16] 255 ))` only reads as base 16
// afterwards if the digits it wrote are the digits `typeset -i16` writes.
//
// A false means the dialect has no answer, which is the unspecified axis
// coming back from the negative question; the caller writes plain decimal.
func (r *Runner) baseRendered(v, base int, prefixed bool, group int) (string, bool) {
	digits := r.sem().IntegerBaseDigits
	sign := ""
	u := uint64(v)
	if v < 0 {
		if !r.ask(r.sem().IntegerBaseNegativeIsTwosComplement,
			"a negative integer rendered in its output base as a bit pattern") {
			if r.unspecified {
				return "", false
			}
			// The sign in front of the magnitude, outside the base mark.
			sign = "-"
			u = uint64(-v)
		}
	}
	var out []byte
	for {
		out = append([]byte{digits[u%uint64(base)]}, out...)
		u /= uint64(base)
		if u == 0 {
			break
		}
	}
	mark := ""
	if prefixed {
		mark = itoa(base) + "#"
	}
	return sign + mark + groupDigits(string(out), group), true
}

// groupDigits puts a `_` every group digits, counted from the right, which is
// what `[#16_4] 1048575` being `16#F_FFFF` says. A group of zero is no
// grouping at all, and is how a specifier turns it back off.
func groupDigits(s string, group int) string {
	if group <= 0 || len(s) <= group {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%group == 0 {
			out = append(out, '_')
		}
		out = append(out, c)
	}
	return string(out)
}

// integerBaseOfLiteral is the base the text of an assignment names, for the
// dialect that learns one from it — 16 from `0x10`, 8 from `8#7`, and none
// from a leading zero or from a value that arrived already evaluated.
//
// 0 when the text names none, which is every text in the dialect that does
// not learn.
func integerBaseOfLiteral(text string) int {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "-")
	text = strings.TrimPrefix(text, "+")
	if len(text) > 2 && text[0] == '0' && (text[1] == 'x' || text[1] == 'X') {
		return 16
	}
	if at := strings.IndexByte(text, '#'); at > 0 && isAllDigits(text[:at]) {
		n, err := strconv.Atoi(text[:at])
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}

// declareOptionMayTakeANumber is the *syntactic* half of whether a letter's
// argument is a number: whether it is a letter of that shape at all, asked
// before any number has been seen.
//
// Split from the answer so that no dialect question is put where a script
// wrote none — `typeset -i n` names no base and must not meet
// Semantics.IntegerAttributeTakesABase, which is unanswered in a dialect that
// has never been asked and refuses when it is. Taking the letters as a plain
// string argument rather than off the runner is what keeps that guarantee
// checkable by reading this line.
func declareOptionMayTakeANumber(c byte, takingANumber string) bool {
	return c == 'i' || strings.IndexByte(takingANumber, c) >= 0
}

// declareOptionTakesANumber is the answer, and the one place that decides it
// for every letter — asked only once digits are in hand.
//
// `-i` reads its own axis rather than Semantics.DeclareOptionsTakingANumber
// because that axis decides a second thing besides the parse, whether the
// base is *recorded*, and the `integer` builtin asks it too. Two fields both
// claiming the integer letter takes a number could disagree; one helper with
// two sources cannot.
func (r *Runner) declareOptionTakesANumber(c byte) bool {
	if c == 'i' {
		return r.takesIntegerBase()
	}
	return strings.IndexByte(r.sem().DeclareOptionsTakingANumber, c) >= 0
}

// attachedOptionNumber is `-i16` and `-F3`: a number riding on the letter
// that takes one, rather than arriving as the word after it.
//
// The number is the trailing run of digits and its letter is the character
// in front of it, which is the only reading either spelling has — a letter
// after the digits would put them in the middle of the word, and no shell
// spells one there. The letter has to be one the dialect claims, or the word
// is somebody else's bad option and not a number at all.
func (r *Runner) attachedOptionNumber(word, known string) (letter byte, digits string, ok bool) {
	at := -1
	for i := 1; i < len(word); i++ {
		if word[i] >= '0' && word[i] <= '9' {
			at = i
			break
		}
	}
	if at < 2 || !isAllDigits(word[at:]) {
		return 0, "", false
	}
	letter = word[at-1]
	if strings.IndexByte(known, letter) < 0 || !r.declareOptionTakesANumber(letter) {
		return 0, "", false
	}
	// The answer and not the *may* half, which is what every caller wants
	// with digits already in hand — and asking the dialect is right here for
	// the same reason: `-i16` names a base, so a dialect with no answer for
	// whether the letter reads one has been asked something the script did
	// write down.
	return letter, word[at:], true
}

// readOptionNumber takes the number an option letter named, under either
// spelling, and reports whether the declaration goes on.
func (r *Runner) readOptionNumber(builtin string, f *declareFlags, letter byte, written string) bool {
	if letter == 'i' {
		return r.readIntegerBase(builtin, f, written)
	}
	n, err := strconv.Atoi(written)
	if err != nil {
		// Not a number at all, so not this letter's argument — the callers
		// only offer digits, so this is belt and braces rather than the
		// check that does the work.
		return true
	}
	// The letter itself is not set here. Both spellings that reach this have
	// already written it — the detached one in the word before, the attached
	// one in what is left of this word after the number comes off.
	f.precision, f.precisionNamed = n, true
	return true
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

// readIntegerBase takes the base an option word named, refusing it where the
// dialect will not spell it and says so. The result is whether the
// declaration goes on.
func (r *Runner) readIntegerBase(builtin string, f *declareFlags, written string) bool {
	n, err := strconv.Atoi(written)
	if err != nil {
		// Not a number at all, so not a base — the caller only offers digits,
		// so this is belt and braces rather than the check that does the work.
		return true
	}
	if !r.integerBaseAccepted(builtin, n, written) {
		return false
	}
	// The letter itself is not set here. Both spellings that reach this have
	// already written it — the detached one in the word before, the attached
	// one in what is left of this word after the base comes off — and
	// setting it again covered for the truncation being wrong.
	f.base, f.baseNamed = n, true
	return true
}

// integerRenderedText is integerRendered over the decimal text integerValue
// produced, which is the shape attributeFolded has to hand.
func (r *Runner) integerRenderedText(name, decimal string) string {
	if r.integerBase[name] == 0 {
		// The common case by far: no base was named and none was learned, so
		// there is nothing to render and nothing to parse back.
		return decimal
	}
	v, err := strconv.Atoi(decimal)
	if err != nil {
		return decimal
	}
	return r.integerRendered(name, v)
}

// learnIntegerBase takes a name's output base from the radix prefix of the
// text being assigned to it, where the dialect learns one and the name has
// none already.
//
// Asked only when the text actually names a base, so a plain `n=5` under an
// integer name never reaches the dialect — which matters, because that is
// nearly every assignment there is.
func (r *Runner) learnIntegerBase(name, text string) {
	if r.arithOutput != nil {
		// The text was *rendered* by an expression's output format rather
		// than written by the script, so there is no literal here to learn a
		// base from. Measured: `typeset -i i; (( i = [#16] 255 ))` leaves the
		// name plain, where `(( i = [#16] 0x1f ))` takes 16 — from the `0x1f`
		// and not from the specifier that spelled the answer.
		//
		// Only this route, and the boundary is the point: the same six
		// characters arriving the ordinary way *do* teach a base, because
		// there they are the text an assignment was given —
		// `typeset -i i; i=$(( [#16] 255 ))` is `typeset -i16 i=255`. The
		// field is nil by then, the expansion having finished.
		return
	}
	if r.integerBase[name] != 0 {
		// The name has one, from the letter or from an earlier assignment,
		// and it sticks: measured, a name that learned 16 renders a later
		// plain `5` as `16#5`.
		return
	}
	base := integerBaseOfLiteral(text)
	if base == 0 {
		return
	}
	if !r.ask(r.sem().IntegerBaseComesFromTheValueAssigned,
		"an integer name taking its output base from the value assigned to it") {
		return
	}
	if !r.validIntegerBase(base) {
		// A radix outside what the dialect takes is not a base it can
		// remember; the value still evaluates, which is what the arithmetic
		// reader does with it either way.
		return
	}
	if base == 10 && r.integerBaseTenIsNone() {
		// Ten learned is ten written down, and where ten is the letter's
		// default it records nothing either way. The dialect that learns
		// answers no, and records it: measured, `typeset -i b; b=10#5` lists
		// back as `typeset -i10 b=5` in zsh — so what the learning path
		// keeps is a base the dialect *takes* and not only one it writes a
		// value in, which is why this is validIntegerBase and not
		// spellsIntegerBase.
		return
	}
	if r.integerBase == nil {
		r.integerBase = map[string]int{}
	}
	r.integerBase[name] = base
}

// rerenderInTheNewBase writes a value the name is already holding back out in
// the base a declaration has just given it.
//
// Not a re-read and not an assignment: the number does not change, only the
// characters it is written in, so it meets no dialect and no readonly
// refusal. `typeset -r16 …` is not a spelling any shell has, and a base named
// over a frozen name re-renders in both that do — measured.
func (r *Runner) rerenderInTheNewBase(name string) {
	held, ok := r.Vars[name]
	if !ok {
		return
	}
	v, err := strconv.Atoi(strings.TrimPrefix(held, "-"))
	if err != nil {
		// Already written in some base, so read it back through the
		// arithmetic that understands `16#ff` and write it out in the new
		// one. Silent on failure: the value stands as it is rather than
		// being half converted.
		text, ok := r.integerValue(held)
		if !ok {
			return
		}
		if v, err = strconv.Atoi(text); err != nil {
			return
		}
	} else if strings.HasPrefix(held, "-") {
		v = -v
	}
	r.Vars[name] = r.integerRendered(name, v)
}
