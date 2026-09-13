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
// **Only zsh has them here**, exactly as only zsh has the float attribute,
// and for the same unresolved reason rather than for want of measuring:
// ksh93 reads a detached number only where the letter ends its option word —
// `typeset -Lx 5 c=ab` is `5: is not an identifier` there, where zsh reads
// the `5` as the width and discards the `x` — so giving ksh the letters needs
// a per-spelling half of Semantics.DeclareOptionsTakingANumber that #1453
// deferred with three disagreements of its own. ksh93 also stores the padded
// text where zsh keeps the raw, which is the split
// Semantics.CaseAttributeFoldsWhenRead already records for `-l` and `-u` and
// answers the same way for these; that half needs no new axis and is ready
// for the preset that wants it. See #1461.
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
	// letter is 'L', 'R' or 'Z'. A name carries at most one: the letters are
	// exclusive and the *earlier* one in an option word wins, measured —
	// `typeset -RZ 5 l=7` is `    7` and lists as `typeset -R5`, while
	// `typeset -LZ 5 m=7` is `7    `.
	letter byte
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

// widthOf is the width attribute a name carries, and whether it carries one.
func (r *Runner) widthOf(name string) (fieldWidth, bool) {
	w, ok := r.fieldWidth[name]
	return w, ok
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
	if w.letter == 'Z' && value != "" && value[0] >= '0' && value[0] <= '9' {
		// Zeros only where the value begins with a digit, so a sign or a
		// word is blank-filled: `typeset -Z 5 g=-7` is `   -7` and
		// `typeset -Z 4 d=abc` is ` abc`. The test is the first character
		// and not whether the whole value is a number — `1.5` fills with
		// zeros, `001.5`.
		pad = strings.Repeat("0", n-len(value))
	}
	if w.letter == 'L' {
		return value + pad
	}
	return pad + value
}
