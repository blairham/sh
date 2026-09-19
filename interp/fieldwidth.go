// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// The width attributes — `typeset -L`, `-R` and `-Z`.
//
// Three letters and one rule, which is why they are one file and one field on
// the runner: each names a width, and what the name holds is presented in
// that many characters. `-L` keeps the left of the value and pads on the
// right, `-R` and `-Z` keep the right and pad on the left, and `-Z` fills
// with zeros instead of blanks where the value begins with a digit.
//
// Measured 2026-09-12 against zsh 5.9.2 and ksh93u+, `env -i` with a scratch
// HOME, ZDOTDIR and HISTFILE, from a script file. Both shells agree on every
// value below:
//
//	typeset -L 5 a=ab          [ab   ]
//	typeset -R 5 b=ab          [   ab]
//	typeset -Z 4 c=7           [0007]
//	typeset -Z 4 d=abc         [ abc]     no leading digit, so blanks
//	typeset -L 5 e=abcdefgh    [abcde]    truncated from the right
//	typeset -R 3 f=abcdefgh    [fgh]      truncated from the left
//	typeset -Z 5 g=-7          [   -7]    a sign is not a digit
//	typeset -Z 5 h=1.5         [001.5]
//	typeset -L 3 i="  x"       [x  ]      leading blanks go before the pad
//	typeset -L 4 j; j=xy       [xy  ]     the attribute outlives its own line
//
// **Two shells have them, and they do not agree about what `Z` is.** Measured
// 2026-09-18 on ksh93u+ 2012-08-01 and zsh 5.9.2, a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME and standard input on
// /dev/null. Every *value* above is the same in both columns; these are not:
//
//	written              ksh93 value  ksh93 listing         zsh value  zsh listing
//	typeset -Z 4 d=7     0007         typeset -Z 4 -R 4 d   0007       typeset -Z4 d
//	typeset -ZR 5 p=7    00007        typeset -Z 5 -R 5 p   00007      typeset -Z5 p
//	typeset -ZL 5 q=7    7·····       typeset -Z 5 -L 5 q   00007      typeset -Z5 q
//	typeset -LZ 5 r=7    7·····       typeset -Z 5 -L 5 r   7·····     typeset -L5 r
//	typeset -LZ 5 i=0012 12····       typeset -Z 5 -L 5 i   0012·      typeset -L5 i
//	typeset -LR 5 t=7    ····7        typeset -R 5 t        7·····     typeset -L5 t
//	typeset -RL 5 u=7    7·····       typeset -L 5 u        ····7      typeset -R5 u
//
// (`·` for a blank, so a trailing pad can be seen.) Three separate facts, and
// each is a field on the semantics vector rather than a branch here:
//
//   - `Z` is a **zero-fill flag riding on a justification** in ksh93, `R` by
//     default, and a *third exclusive letter* in zsh — see
//     Semantics.DeclareZeroFillLetter. The listing follows: ksh93 writes the
//     justification the flag rode in on beside it, which is where the
//     `-Z 4 -R 4` pair comes from.
//   - Where the flag rides on `L` there is no left-hand pad to fill, and
//     ksh93 spends it the other way: the value's **leading zeros come off**.
//     Measured a value at a time — `0012` is `12`, `00ab` is `ab`, `0.5` is
//     `.5`, `0` and `000` are nothing at all, and `-07` is untouched because
//     it does not begin with one.
//   - Where a declaration writes both justification letters, the **last**
//     wins in ksh93 and the **first** in zsh, across words as well as inside
//     one — `typeset -L -R 5 a=7` is `····7` there. See
//     Semantics.WidthJustificationPrecedence.
//
// ksh93 also stores the padded text where zsh keeps the raw, which is the
// split Semantics.CaseAttributeFoldsWhenRead already records for `-l` and
// `-u` and answers the same way for these; that half needed no new axis. See
// #1461 and #2859.
//
// `-E` is deliberately not here. It is a *format* rather than a width and the
// two shells do not share one: `typeset -E 3 y=3.14159` is `3.14e+00` in zsh
// and `3.14` in ksh93, which is `%.*e` with n-1 places against `%.*g` with n.
// That is an axis and a second rendering, and it belongs with ksh's float
// letters rather than with these three.

// fieldWidth is the width attribute a name carries: which letter named it and
// how wide.
//
// One struct rather than two maps because the two facts are never wanted
// apart — a letter with no width and a width with no letter are both
// meaningless — and because a shadow has one thing to save and put back.
type fieldWidth struct {
	// letter is 'L', 'R' or 'Z': the justification, and the letter a listing
	// writes back for it. 'Z' stands here only in the column where the
	// zero-fill letter is a justification of its own — see
	// Semantics.DeclareZeroFillLetter — and in the other column the
	// justification is 'L' or 'R' with zeroFill saying the letter was
	// written beside it.
	letter byte
	// zeroFill is the `Z` letter under the reading where it **rides on** a
	// justification rather than being one. Its own field rather than a third
	// value of letter because the two facts are independent there: the
	// listing writes `typeset -Z 5 -L 5` and `typeset -Z 5 -R 5`, so a name
	// carries both at once and a shadow has both to put back.
	zeroFill bool
	// width is how many characters the value is presented in. Zero means the
	// letter was written with no number and none has been learned yet: the
	// width then comes from the first value stored, which both shells record
	// as if it had been written — `typeset -L f=xy` lists as `typeset -L2
	// f=xy` and `typeset -Z h=7` as width 1.
	//
	// A written zero is the same thing rather than a width of nothing:
	// measured, `typeset -L 0 i=abcd` lists as `typeset -L4 i=abcd` and
	// leaves `abcd` whole.
	width int
}

// widthLearned records the width a name's first value teaches it, for the
// letter written with no number of its own.
//
// On the way in rather than on the way out, and that is the whole reason it
// is separate from widthPadded: the width becomes part of the *declaration* —
// `typeset -L f=xy` lists back as `typeset -L2 f=xy` — so a shell that
// computed it at print time would answer that listing right and every later
// read wrong, once the name held something shorter.
func (r *Runner) widthLearned(name, value string) {
	w, ok := r.fieldWidth[name]
	if !ok || w.width != 0 {
		return
	}
	n := len(strings.TrimLeft(value, " \t"))
	if n == 0 {
		// Nothing to learn from, and zero is not a width: `typeset -L 3 j`
		// with no value keeps the 3 it was given, and a letter with neither
		// a number nor a value waits for one. Measured, `typeset -L 3 j;
		// j=abcd` is `abc`.
		return
	}
	w.width = n
	r.fieldWidth[name] = w
}

// widthPadded is what the width attribute makes of one value.
//
// The leading blanks come off before anything else, which is measured and is
// not the same as trimming the result: `typeset -L 3 k="  x"` is `x  ` in
// both shells, where padding the value as written would give `  x`.
func (r *Runner) widthPadded(name, value string) string {
	w, ok := r.fieldWidth[name]
	if !ok || w.width == 0 {
		return value
	}
	value = strings.TrimLeft(value, " \t")
	if w.zeroFill && w.letter == 'L' {
		// The zero-fill letter riding on the *left* justification has no
		// left-hand pad to fill, so it spends itself the other way and the
		// leading zeros come off. Measured 2026-09-18 on ksh93u+ at width
		// six: `0012` reads `12`, `00ab` reads `ab`, `0.5` reads `.5`, `0`
		// and `000` read as nothing at all, and `-07` is untouched, a sign
		// not being a zero. The width is learned before this runs, which is
		// why `typeset -LZ p=0012` is a width of four holding `12`.
		value = strings.TrimLeft(value, "0")
	}
	n := w.width
	switch {
	case len(value) > n && w.letter == 'L':
		// Truncated from the far side of the one that is kept, which is the
		// same rule as the padding: `-L` keeps the left.
		return value[:n]
	case len(value) > n:
		return value[len(value)-n:]
	}
	pad := strings.Repeat(" ", n-len(value))
	if w.zeroFilling() && value != "" && value[0] >= '0' && value[0] <= '9' {
		// Zeros only where the value begins with a digit, so a sign or a
		// word is blank-filled: `typeset -Z 5 g=-7` is `   -7` and
		// `typeset -Z 4 d=abc` is ` abc`. The test is the first character
		// and not whether the whole value is a number — `1.5` fills with
		// zeros, `001.5` — and it is unanimous across the two columns that
		// have the letter, measured a value at a time at width six: `1a`,
		// `1e3`, `0x1f` and `1 2` all fill with zeros, `.5` and `+7` with
		// blanks.
		pad = strings.Repeat("0", n-len(value))
	}
	if w.letter == 'L' {
		return value + pad
	}
	return pad + value
}

// zeroFilling reports whether the pad this width lays down is zeros rather
// than blanks, which the two readings of the letter reach by different roads:
// under one it is the justification itself, and under the other it is a flag
// riding on the right-hand justification. Riding on the left-hand one fills
// nothing — there is no left-hand pad — and strips instead.
func (w fieldWidth) zeroFilling() bool {
	return w.letter == 'Z' || (w.zeroFill && w.letter == 'R')
}

// DeclareZeroFillLetterPolicy is what the `Z` letter of a declaration **is**.
//
// Two readings, and neither contains the other: under one the letter names a
// justification of its own and cannot stand beside `L` or `R`, under the other
// it names a fill and has to have one of them underneath it. The same line
// comes out a different attribute under each, and a name declared under the
// second carries two facts where the first carries one.
type DeclareZeroFillLetterPolicy int

const (
	// DeclareZeroFillLetterUnspecified is no answer, which the dialects with
	// no width letters at all hold and never reach: a declaration there
	// cannot write the letter.
	DeclareZeroFillLetterUnspecified DeclareZeroFillLetterPolicy = iota

	// DeclareZeroFillLetterIsAJustificationOfItsOwn makes `Z` the third
	// member of an exclusive set with `L` and `R`: it right-justifies, it
	// fills with zeros, and a declaration writing it beside either of the
	// others keeps one letter and drops the rest. zsh 5.9.2, measured
	// 2026-09-18: `typeset -ZL 5 q=7` lists as `typeset -Z5 q=7` and
	// `typeset -LZ 5 r=7` as `typeset -L5 r=7`, so the pair is settled by
	// WidthJustificationPrecedence and never by the letters themselves.
	DeclareZeroFillLetterIsAJustificationOfItsOwn

	// DeclareZeroFillLetterRidesOnTheJustification makes `Z` a fill that
	// needs a justification under it — `R` where the declaration names none.
	// ksh93u+, measured the same day: `typeset -Z 4 d=7` lists as `typeset
	// -Z 4 -R 4 d=0007`, naming both, and `typeset -ZL 5 q=7` as `typeset -Z
	// 5 -L 5 q='7    '`, where the fill has nothing to fill and the value's
	// leading zeros come off instead.
	//
	// The default is what makes this more than a listing difference: under
	// the other reading `-LZ` is `L` alone, and here it is `L` **and** the
	// fill, so `typeset -LZ 5 i=0012` is `12   ` here and `0012 ` there.
	DeclareZeroFillLetterRidesOnTheJustification
)

func (p DeclareZeroFillLetterPolicy) String() string {
	switch p {
	case DeclareZeroFillLetterIsAJustificationOfItsOwn:
		return "a justification of its own"
	case DeclareZeroFillLetterRidesOnTheJustification:
		return "a fill riding on the justification"
	}
	return "unspecified"
}

// WidthJustificationPrecedencePolicy is which width letter a declaration
// writing more than one ends up with.
//
// The same two readings NumericTypeLetterPrecedencePolicy draws for the
// numeric letters, and the opposite answers from the same two shells, which
// is why it is a second field and not a reuse of the first: one shell ranks
// the numeric letters and takes the *last* width letter, and the other takes
// the first of both.
type WidthJustificationPrecedencePolicy int

const (
	// WidthJustificationPrecedenceUnspecified is no answer, held by the
	// dialects that cannot write one width letter, let alone two.
	WidthJustificationPrecedenceUnspecified WidthJustificationPrecedencePolicy = iota

	// WidthJustificationFirstWrittenWins is zsh's, and it is the reading the
	// parse falls into on its own: the letter read first sets the attribute
	// and the ones behind it find it already set. Measured 2026-09-18 on zsh
	// 5.9.2 — `typeset -LR 5 t=7` lists as `typeset -L5 t=7`, `-RL` as
	// `typeset -R5`, `-ZRL` as `typeset -Z5` and `-LRZ` as `typeset -L5`.
	WidthJustificationFirstWrittenWins

	// WidthJustificationLastWrittenWins is ksh93's, and it holds across
	// words as well as inside one. Measured in the same run, each read back
	// with `typeset -p`:
	//
	//	typeset -LR 5 t=7     typeset -R 5 t='    7'
	//	typeset -RL 5 u=7     typeset -L 5 u='7    '
	//	typeset -ZRL 5 c=7    typeset -Z 5 -L 5 c='7    '
	//	typeset -LRZ 5 d=7    typeset -Z 5 -R 5 d=00007
	//	typeset -L -R 5 a=7   typeset -R 5 a='    7'
	//	typeset -R -L 5 b=7   typeset -L 5 b='7    '
	//
	// The last two are what say this is the *order written* and not the
	// order of one word's characters: there both letters are really read,
	// and the later one still wins.
	WidthJustificationLastWrittenWins
)

func (p WidthJustificationPrecedencePolicy) String() string {
	switch p {
	case WidthJustificationFirstWrittenWins:
		return "the first letter written"
	case WidthJustificationLastWrittenWins:
		return "the last letter written"
	}
	return "unspecified"
}

// recordWidthLetter is one width letter arriving on a declaration.
//
// Three letters, two questions and one field, which is why the whole of it is
// here rather than inline in the option loop: what `Z` is, and which of the
// justifications survives where a declaration writes both.
//
// It reads no number. The width is the option loop's, because a detached one
// is a rule about *words* — see Semantics.DeclareNumberDetachedOnlyAtTheWordEnd
// — and a letter that lost the precedence still consumed its digits.
func (r *Runner) recordWidthLetter(f *declareFlags, c byte) {
	if c == 'Z' && r.sem().DeclareZeroFillLetter == DeclareZeroFillLetterRidesOnTheJustification {
		f.widthZeroFill = true
		if f.widthLetter == 0 {
			// The fill has to have a justification under it and the
			// declaration named none, so it takes the one the shell writes
			// back for it: `typeset -Z 4 d=7` lists as `typeset -Z 4 -R 4`.
			// A justification written *later* replaces this, which is the
			// last-written rule doing the work rather than a second one:
			// `typeset -ZL 5 q=7` is `-Z 5 -L 5`.
			f.widthLetter = 'R'
		}
		return
	}
	if f.widthLetter == 0 ||
		r.sem().WidthJustificationPrecedence == WidthJustificationLastWrittenWins {
		f.widthLetter = c
	}
}

// widthLetterCompany reports whether a declaration wrote the integer letter
// and one of the three width letters both.
//
// Read off the letters as written rather than off the flags, for the reason
// numericTypeLetterCompany is: the parse has already discarded a loser by
// order, and the number a losing letter read went with the winner — `typeset
// -iL 5` leaves an integer with a base of five in the column that takes the
// pair, so a test on the flags could no longer say a width letter had been
// written at all.
//
// The **sign is not read**, which is the same rule attributeOverFrozenRefused
// follows: what the refusal is about is the letters standing on one
// declaration, and a plus is still a letter written.
func widthLetterCompany(f declareFlags) bool {
	return strings.ContainsRune(f.letters, 'i') &&
		strings.ContainsAny(f.letters, "LRZ")
}

// widthUnwound takes the standing width attribute's presentation back off the
// value a name is holding, for the declaration that is about to replace it.
//
// The store is the presentation in this column, so a second declaration that
// read what it found there would be reading its own predecessor's pad.
// Measured 2026-09-18 on ksh93u+ 2012-08-01, each line a declaration and then
// a second one over the same name:
//
//	typeset -L 4 u=ab;    typeset -R 4 u    '  ab'
//	typeset -R 4 v=ab;    typeset -L 4 v    'ab  '
//	typeset -L 4 e=ab;    typeset -L 6 e    'ab    '
//	typeset -L 4 d=ab;    typeset -Z 4 d    '  ab'
//	typeset -L 4 e=12;    typeset -Z 4 e    0012
//	typeset -R 4 f=12;    typeset -Z 4 f    0012
//	typeset -Z 4 a=0012;  typeset -L 4 a    '12  '
//	typeset -Z 4 h=7;     typeset -Z 6 h    000007
//	typeset -LZ 5 c=0012; typeset -R 5 c    '   12'
//	typeset -L 4 t=0012;  typeset -L 4 t    0012
//
// The last line is the control and is why this is an unwind rather than a
// trim: nothing about the value changes, so the re-read has nothing to say
// and the leading zeros this one *did* write stay where they are.
//
// Asked only of the column that stores the presentation. Where the width acts
// on the read instead, the store is the text the assignment carried and there
// is nothing to unwind — see Semantics.CaseAttributeFoldsWhenRead.
func (r *Runner) widthUnwound(name string) {
	w, ok := r.fieldWidth[name]
	if !ok || r.caseFoldsOnRead() {
		return
	}
	v, ok := r.Vars[name]
	if !ok {
		return
	}
	if w.letter == 'L' {
		r.Vars[name] = strings.TrimRight(v, " \t")
		return
	}
	v = strings.TrimLeft(v, " \t")
	if w.zeroFilling() {
		v = strings.TrimLeft(v, "0")
	}
	r.Vars[name] = v
}

// widthZerosUnwound is the narrower half, for the declaration that takes the
// attribute **off** rather than replacing it.
//
// Only the zeros there, and that asymmetry is measured rather than
// convenient. Same run:
//
//	typeset -Z 4 d=7;  typeset +Z d    7        the fill comes off
//	typeset -R 5 b=ab; typeset +R b    '   ab'  the blanks do not
//	typeset -L 5 a=ab; typeset +L a    'ab   '  nor these
//
// A blank pad is left where it stands because nothing is going to lay one
// down again, and the shell has no raw text to fall back on; a zero pad is
// not, because zeros read as part of the value rather than as spacing.
func (r *Runner) widthZerosUnwound(name string) {
	if w, ok := r.fieldWidth[name]; !ok || !w.zeroFilling() {
		return
	}
	r.widthUnwound(name)
}
