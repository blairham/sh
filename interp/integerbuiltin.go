// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strconv"
	"strings"

	"github.com/blairham/sh/syntax"
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
// zsh's own `add-zsh-hook` is why it exists. Run under a shell that has no
// `integer`, the function fails on its own opening declaration, so a shell
// without the word cannot install a hook — which is the whole of what a zsh
// startup file uses that function for. Measured by running it, which is the
// only way this tree learns what another shell's code does; see CLEANROOM.md
// on citing observations rather than source files (#2167).
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
			r.fatalUsageQuiet()
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
	// And the letter it asked for, which is what the *company* rules read:
	// this word is `typeset -i` under another name, so a float letter
	// written beside it is the pair one dialect refuses — measured,
	// `integer -E 3 a=1.5` is typeset's usage block in ksh93u+. Recorded
	// here rather than in the parse because the letter was never written;
	// without it the pair is invisible under this spelling and visible under
	// the other, which is one rule with two answers.
	if !strings.ContainsRune(f.letters, 'i') {
		f.letters += "i"
		f.letterSigns += "-"
	}
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
		f.remove, f.signDecided = off, true
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
		if alphabet := len(r.sem().IntegerBaseDigits); alphabet > 0 && base > alphabet {
			// A base past the end of the alphabet is *kept* and only the
			// rendering falls back — to ten, with the mark still on.
			// Measured 2026-09-12 on ksh93u+: `typeset -i65 d=100` reads
			// `10#100` and lists as `typeset -i 65 d=10#100`, and a later
			// `d=200` is `10#200`, so the 65 is stored and every render
			// goes through this. 2 through 64 spell themselves there, and
			// 65, 66, 70, 100, 128, 256 and 1000 all give `10#`.
			//
			// Not an axis: the only other dialect with the attribute
			// refuses everything outside 2 to 36 by name, so nothing can
			// hold a base this reading would reach and a field for it would
			// have one value nobody could take. See
			// Semantics.IntegerBaseDigits, whose length is the alphabet —
			// and a dialect with no alphabet at all has no base to be past
			// the end of, so it renders plain as it always did.
			return "10#" + itoa(v)
		}
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
		m, ok := r.baseMark(base)
		if !ok {
			return "", false
		}
		mark = m
	}
	return sign + mark + groupDigits(string(out), group), true
}

// baseMark is the text written in front of the digits to say what base they
// are in — `16#`, or C's own spelling of that base in the dialect that
// borrows it.
//
// Only two bases have a C spelling to borrow. Sixteen's is `0x` and is this
// axis on its own. Eight's is a leading zero, and it is only C's spelling in a
// shell that *reads* a leading zero as octal, so it is this axis and
// ArithLeadingZeroIsOctal together — measured, zsh 5.9.2 writes `8#10` under
// `C_BASES` alone and `010` under `C_BASES` and `OCTAL_ZEROES` both.
//
// A false is the unspecified axis coming back, handled the way baseRendered
// handles the negative question's.
func (r *Runner) baseMark(base int) (string, bool) {
	if base != 16 && base != 8 {
		// Nothing else in the alphabet has a C literal to be spelled as, so
		// the question is never put for it.
		return itoa(base) + "#", true
	}
	if !r.ask(r.sem().IntegerBaseMarkIsCSpelled,
		"an output base written in C's spelling rather than as `base#`") {
		if r.unspecified {
			return "", false
		}
		return itoa(base) + "#", true
	}
	if base == 16 {
		return "0x", true
	}
	if !r.ask(r.sem().ArithLeadingZeroIsOctal,
		"a leading zero read as octal, which is what makes it base eight's C spelling") {
		if r.unspecified {
			return "", false
		}
		// C-spelled marks, but a leading zero means nothing to this shell,
		// so base eight has no spelling here to borrow and keeps `8#`.
		return "8#", true
	}
	return "0", true
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
	if letter == 'L' || letter == 'R' || letter == 'Z' {
		// A width rather than a precision, and one field pair each so the
		// listing knows which letter to write the number back onto.
		f.width, f.widthNamed = n, true
		return true
	}
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

// octalNumeral is a zero-padded numeral whose digits are all octal ones,
// which is the only shape a bare leading zero names a base in.
func octalNumeral(text string) bool {
	if !zeroPadded(text) {
		return false
	}
	for i := 0; i < len(text); i++ {
		if text[i] == '8' || text[i] == '9' {
			return false
		}
	}
	return true
}

// radixInsideExpression is the base a radix literal *inside* an assignment's
// text names, and whether a bare leading zero stands anywhere in it — the
// same pair radixWritten gives the arithmetic route, over a text this route
// has only as characters.
//
// Parsed here rather than handed down from the evaluation that follows,
// which is the other way it could have arrived. Three routes learn from a
// text — an assignment folded through an attribute, an append, and the
// re-read a name already holding a value takes when `-i` arrives — and the
// tree is of no use to any of them but this question, so handing one down
// would have widened three signatures to narrow one. Nothing is observable
// either way: the ordering of the learn against the evaluation cannot be
// measured, because a text that fails to evaluate ends the shell before
// anything can read the name's base back.
//
// A text that is only a numeral never reaches the parser. That is nearly
// every assignment an integer name ever takes, and it is already answered
// above by the literal reading; parsing it again here would put a parse on
// the hot path to learn nothing. plainNumeral is the test builtin's, reused
// rather than copied: it already means "digits with at most a sign, and no
// leading zero in front of another digit", and the leading zero it excludes
// is one this route must still look at.
func (r *Runner) radixInsideExpression(text string) (base int, padded bool) {
	text = strings.TrimSpace(text)
	if text == "" || plainNumeral(text) {
		return 0, false
	}
	p := syntax.NewParser("", r.dialect())
	e := p.ParseArithFor(text, syntax.Pos{})
	if p.Err() != nil {
		// Text that will not parse is not this function's to report: the
		// evaluation that follows parses it again and says so, in the
		// wording and with the status the dialect gives an assignment.
		return 0, false
	}
	return radixWritten(e)
}

// learnIntegerBase takes a name's output base from a radix written in the
// text being assigned to it, where the dialect learns one and the name has
// none already.
//
// A radix at the front of the text is read out of the characters; a radix
// standing anywhere else is an expression's, and radixInsideExpression goes
// and finds it.
//
// Asked only when the text actually holds a radix, so a plain `n=5` under an
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
	// A **bare leading zero** names a base too, where the dialect reads one
	// as octal: measured 2026-09-17 on zsh 5.9.2 under `setopt octal_zeroes`,
	// `typeset -i d=010` reads back `8#10` — eight, written in the base it
	// was read in — where `typeset -i h=0x10` has always been `16#10` here.
	// The prefix spellings this function knows are not the only way to write
	// a radix down; they are only the two that carry it in the text (#3520).
	// Trimmed the way integerBaseOfLiteral trims, and measured: `typeset -i
	// s=" 010 "` is `8#10` on zsh 5.9.2, so the spaces around a literal are
	// not what tells a number from an expression here.
	//
	// And the digits have to *be* octal, which is not tidiness: `08` is a
	// zero-padded numeral that no leading-zero reading makes a base, since
	// the column that reads one refuses the numeral outright — `setopt
	// octal_zeroes; typeset -i x=08` is `bad math expression` on zsh 5.9.2 —
	// while the columns that do not read one have a decimal 8 with no base
	// in it. Without the check this asked the learning question about every
	// padded number there is, which is a question a dialect may not have
	// answered: `a=08; typeset -i a` met a refusal in a suite that had
	// nothing to do with bases.
	padded := base == 0 && octalNumeral(strings.TrimSpace(text))
	if base == 0 && !padded {
		// The text is not a numeral, so it is an **expression**, and an
		// expression carries its radix wherever the radix stands rather than
		// at its front: measured 2026-09-18 on zsh 5.9.2, `typeset -i a=1+0x1f`
		// reads back `16#20` and `typeset -i t=1+010` under `setopt
		// octal_zeroes` reads back `8#11`, exactly as `(( a = 1 + 0x1f ))`
		// does. Reading the text as a literal can only ever find a radix that
		// begins it, so every row with an operator in it taught nothing
		// (#3670).
		//
		// radixWritten is the same walk the arithmetic route learns by, so
		// the two routes now answer one question over one representation —
		// the point of the fix, and why this is not a second reader standing
		// beside the literal one. The literal reading above is a fast path
		// over the same leaves and not a second answer: radixWritten's
		// numeral case is integerBaseOfLiteral and octalNumeral.
		base, padded = r.radixInsideExpression(text)
	}
	if base == 0 && !padded {
		return
	}
	if !r.ask(r.sem().IntegerBaseComesFromTheValueAssigned,
		"an integer name taking its output base from the value assigned to it") {
		return
	}
	if padded {
		// Behind the learning question, so a dialect that takes no base from
		// a value is never asked what a leading zero means — which is every
		// column but one, and two of them read a leading zero as octal while
		// carrying no base at all. Asking in front of it would put a
		// question to `typeset -i e=010` in bash, whose answer is a plain 8
		// either way.
		if !r.octalLeadingZero() {
			return
		}
		base = 8
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
