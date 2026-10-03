// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The padding flags, `${(l:expr::string1::string2:)x}` and its `r` mirror:
// each word is laid out in a field expr units wide, padded on the left or on
// the right. One grammar in the panel has the construct, so what it means
// here is that shell's answer, measured and recorded in
// docs/spec/grammar/parameter-expansion.md.
//
// Where the step sits is **measured rather than read off the rule numbers**,
// and one row of that is the reason this file exists rather than three lines
// beside the ordering. The vendor manual lists padding at rule 22, after the
// re-evaluation at 21; the shell does the opposite. With `e='ab$(echo XY)'`,
// twelve characters as written and four once read again:
//
//	${(el:20::x:)e}   xxxxxxxxabXY   padded to 20 first, then re-read
//	                                 (a re-reading that ran first would
//	                                 have padded `abXY` to 20 and left
//	                                 sixteen `x`)
//
// Everything else about the placement agrees with the numbers, and each was
// measured on zsh 5.9.2 with a probe that separates the two orders:
//
//	${(l:5::x:)#v}      xxxx2      after the length, which is the count
//	${(Ul:4::x:)u}      xxAB       after the case conversion
//	${(%l:6::x:)m}      the path   after the prompt escapes, `m=%x`
//	${(ql:6::x:)q}      xxa\ b     after the quoting, `q='a b'`
//	${(Ql:8::x:)qq}     xxxxxa b   and after the unquoting
//	${(@ol:3::x:)b}     xxa xbb    after the ordering, `b=(bb a)`
//	${(s.:.l:3::x:)v}   xxa xxb    and after the splitting, per word
//	"${(l:5::-:)a}"     three      but after the quoted join too, which is
//	                               what makes this the *joined* word rather
//	                               than three padded elements
//
// The last is the one that says padding is not elementwise by nature: it is
// applied to each word the group has *by the time it runs*, and the quoted
// join at rule 5 has already turned an array into one of them.

// padApplies reports whether the group asks for padding at all.
func padApplies(e *syntax.ParamExpr) bool {
	return strings.ContainsAny(e.Flags, "lr")
}

// padFlagged lays each word out in the field the group asked for, and is the
// identity where no padding flag was written.
//
// ok is false when the width would not evaluate, which has been reported.
func (r *Runner) padFlagged(e *syntax.ParamExpr, words []string) ([]string, bool) {
	left := strings.ContainsRune(e.Flags, 'l')
	right := strings.ContainsRune(e.Flags, 'r')
	if !left && !right {
		return words, true
	}
	var lw, rw int
	if left {
		w, ok := r.padWidth(e.PadLeft.Width)
		if !ok {
			return nil, false
		}
		lw = w
	}
	if right {
		w, ok := r.padWidth(e.PadRight.Width)
		if !ok {
			return nil, false
		}
		rw = w
	}
	// A width of zero is the flag doing nothing, which is measured and is
	// not the same as truncating to nothing: `${(l:0:)v}` on `ab` is `ab`,
	// where `${(l:1:)v}` is `b`. It reaches the pair as well — with
	// `abcd=abcd`, `${(l:0::L:r:6::R:)abcd}` is `abcdRR`, so a zero side is
	// out of the way entirely rather than halving the word for the other.
	lp := r.paddingArgs(e, 'l', e.PadLeft)
	rp := r.paddingArgs(e, 'r', e.PadRight)
	// An empty element a nested expansion handed back is one unit wide here,
	// though it prints as nothing. See Runner.padAGhost.
	ghost := r.flagEmptiesAreGhosts(e, words)
	out := make([]string, len(words))
	for i, w := range words {
		if col := columnPadding(e); col != nil && r.countsCharacters(w+lp.fill+lp.insert+rp.fill+rp.insert) {
			out[i] = r.padInColumns(w, lw, rw, lp, rp, col)
			continue
		}
		if ghost && w == "" {
			out[i] = r.padAGhost(lw, rw, lp, rp)
			continue
		}
		switch {
		case lw > 0 && rw > 0:
			// Both flags: the manual's rule, and measured on zsh 5.9.2 with
			// `(l:4::L:r:4::R:)` over `a`, `ab`, `abc`, `abcd` and `abcde` —
			// `LLLLaRRR`, `LLLabRRR`, `LLLabcRR`, `LLabcdRR`, `LLabcdeR`.
			// The word is cut in half, the first half padded on the left in
			// a field lw wide and the second padded on the right in a field
			// rw wide, and an odd word puts the extra unit in the *second*
			// half — which is what "the extra padding is applied on the
			// left" comes to, the left field then having one unit less of
			// word to hold.
			u := r.units(w)
			half := len(u) / 2
			out[i] = r.padLeftTo(strings.Join(u[:half], ""), lw, lp) +
				r.padRightTo(strings.Join(u[half:], ""), rw, rp)
		case lw > 0:
			out[i] = r.padLeftTo(w, lw, lp)
		case rw > 0:
			out[i] = r.padRightTo(w, rw, rp)
		default:
			out[i] = w
		}
	}
	return out, true
}

// padWidth evaluates a field width, which is an arithmetic expression rather
// than a number: `${(r:longest+1:: :)x}` and `${(pl:$((COLUMNS-1))::-:)x}`
// are both in reach on this machine, and a bare name is read as a name there
// exactly as it is inside `$(( ))`.
//
// A negative width is its own absolute value, measured: `${(l:-3:)v}` on `ab`
// is ` ab`, the same three-wide field `${(l:3:)v}` gives, and `${(l:-1:)v}`
// truncates to `b` exactly as `${(l:1:)v}` does. An empty expression is zero
// and no error, which is the same answer `$(( ))` gives here.
//
// The shell being modeled refuses a width past what a 32-bit count can hold —
// `${(l:2147483648:)v}` is an error in the flags there, and `${(l:4294967296:)v}`
// is the value unchanged, the count having wrapped to zero. That is an
// artifact of one implementation's internal width rather than a behavior, so
// it is a **measured gap** here rather than something reproduced: a width this
// side cannot serve is an allocation neither shell can serve.
func (r *Runner) padWidth(text string) (int, bool) {
	tree, text, perr := r.arithTreeOver(nil, text, arithTextArrived)
	if perr != nil {
		// A failure to *read* the expression, which can only happen once it
		// has been expanded, so it is reported here rather than by the
		// parser — the same door interp/expand.go's arithSpanValue leaves by.
		r.diagf("%s\n", r.diag().ParseFailure(perr))
		r.expandErr = true
		return 0, false
	}
	n, err := r.evalArith(tree)
	if r.unspecified {
		// An axis inside the expression went unanswered and `ask` has
		// reported it. Padding to a width invented out of a refused question
		// would be a plausible answer at status 0.
		r.expandErr = true
		return 0, false
	}
	if err != nil {
		r.diagf("%s\n", r.arithFailure(text, err))
		r.expandErr = true
		return 0, false
	}
	if n < 0 {
		n = -n
	}
	if n < 0 {
		// The one value whose negation is itself. Zero is the flag doing
		// nothing, which is the honest answer for a width no field can have.
		n = 0
	}
	return n, true
}

// padding is one side's two fill strings, resolved.
type padding struct {
	// fill is `string1`, repeated as often as needed.
	fill string
	// insert is `string2`, laid once directly against the word.
	insert string
}

// paddingArgs resolves one side's fills.
//
// Three answers per slot, and they are all measured — see syntax.ParamPad for
// the probes. A fill the group did not write pads with spaces whatever `$IFS`
// holds; one written empty uses `$IFS`'s first character; one written with
// text uses that text, read through the `(p)` flag's escapes where a `p`
// stands in front of the padding letter, which is what makes
// `${(pl.3..\n.)}` three newlines.
//
// A `string2` that was not written is inserted nowhere, which is why its
// absent answer is the empty string where `string1`'s is a space.
func (r *Runner) paddingArgs(e *syntax.ParamExpr, flag rune, p syntax.ParamPad) padding {
	return padding{
		fill:   r.padArg(e, flag, p.Fill, p.FillSet, " "),
		insert: r.padArg(e, flag, p.Insert, p.InsertSet, ""),
	}
}

func (r *Runner) padArg(e *syntax.ParamExpr, flag rune, text string, set bool, absent string) string {
	if !set {
		return absent
	}
	if text == "" {
		// Measured with `IFS=.` and `v=ab`: `${(l:5:::)v}` is `...ab` and
		// `${(l:5::x:::)v}` is `xx.ab`, so an empty argument is the first
		// character of IFS in either slot. An IFS with no first character
		// leaves the slot contributing nothing, which is measured too:
		// `IFS=; ${(l:5:::)v}` is `ab`, unpadded.
		return r.ifsFirst(r.ifs())
	}
	return r.flagArgument(e, flag, text)
}

// padLeftTo lays w out in a field width units wide, padded on the left.
//
// A word already that wide or wider is **truncated from the far end**, which
// is measured and is the half a symmetric implementation gets wrong:
// `${(l:1:)v}` on `ab` is `b` and `${(l:3:)w}` on `abcdef` is `def`, where the
// `r` mirror keeps the other end.
func (r *Runner) padLeftTo(w string, width int, p padding) string {
	u := r.units(w)
	if len(u) >= width {
		return strings.Join(u[len(u)-width:], "")
	}
	room := width - len(u)
	// `string2` goes once directly against the word and is itself truncated
	// where it does not fit — measured, `${(l:3::x::yz:)v}` on `ab` is `zab`
	// and `${(l:3::x::yzw:)v}` is `wab`, so it keeps the end nearest the word.
	ins := r.units(p.insert)
	if len(ins) > room {
		ins = ins[len(ins)-room:]
	}
	room -= len(ins)
	return r.repeatFill(p.fill, room, true) + strings.Join(ins, "") + w
}

// padRightTo is padLeftTo's mirror: the word keeps its *near* end when it is
// truncated, and both fills are laid out on the other side.
func (r *Runner) padRightTo(w string, width int, p padding) string {
	u := r.units(w)
	if len(u) >= width {
		return strings.Join(u[:width], "")
	}
	room := width - len(u)
	ins := r.units(p.insert)
	if len(ins) > room {
		ins = ins[:room]
	}
	room -= len(ins)
	return w + strings.Join(ins, "") + r.repeatFill(p.fill, room, false)
}

// repeatFill produces n units of fill, taken from the end nearest the word:
// the *last* n units of the repetition for a left pad and the first n for a
// right one.
//
// Which end matters as soon as the fill is more than one unit long, and it is
// measured on zsh 5.9.2 with `v=ab`: `${(l:7::ab:)v}` is `bababab` — five
// units of padding spelled `babab`, the tail of the repetition — where
// `${(r:7::ab:)v}` is `abababa`, its head. Truncating both from the front
// answers `ababaab` for the first, which is a plausible seven characters.
//
// A fill that came to nothing contributes nothing rather than looping.
func (r *Runner) repeatFill(fill string, n int, keepRight bool) string {
	if n <= 0 || fill == "" {
		return ""
	}
	u := r.units(fill)
	start := 0
	if keepRight {
		start = (len(u) - n%len(u)) % len(u)
	}
	var b strings.Builder
	for i := range n {
		b.WriteString(u[(start+i)%len(u)])
	}
	return b.String()
}

// padAGhost pads the empty element a nested expansion hands back, which
// takes a unit of the field while printing as nothing. Measured 2026-10-01
// and 2026-10-02 on zsh 5.9.2 under `LC_ALL=C` with `b=(x ” y)`:
// `"${(@l:1:)${b[@]}}"` leaves it empty, `"${(@l:3:)${b[@]}}"` pads it to
// two blanks, `"${(@r:2::-:)${b[@]}}"` to one `-`, and
// `"${(@l:2::L:r:2::R:)${b[@]}}"` is `LLR` — the unit is the word's second
// half. Which empties these are is Runner.nestedEmptiesAreGhosts, the rule
// the sort key and a length follow too (#5345, #5412): a nested *value* that
// is empty is not one, and `"${(l:3:)${b[2]}}"` is three blanks.
func (r *Runner) padAGhost(lw, rw int, lp, rp padding) string {
	switch {
	case lw > 0 && rw > 0:
		return r.padLeftTo("", lw, lp) + r.padRightTo("", rw-1, rp)
	case lw > 0:
		return r.padLeftTo("", lw-1, lp)
	case rw > 0:
		return r.padRightTo("", rw-1, rp)
	}
	return ""
}

// columnPadding is the width a character is given when `(m)` is written
// beside the padding pair, and nil where it is not. Measured 2026-10-03 on
// zsh 5.9.2 in a UTF-8 locale, with `w=日本語`, a `日` two columns wide:
//
//	(mr:N:)w   N=1..7  日 日 日本 日本 日本語 日本語 "日本語 "
//	(ml:N:)w   N=1..7  "" 語 語 本語 本語 日本語 " 日本語"
//	(mmr:2:)w  (mml:2:)w   日本  本語    `mm` gives every character one
//	                                     column and a combining one none
//	(mr:7::日:)日本   日本日日   the right fill crosses the edge too
//	(ml:8::日:)日本   日日日本
//	(ml:7::日:)日本   nothing at all, where the left fill cannot land on it
//	(ml:3:r:3:)日本   " 日本 "   each half in its own field
//
// So the right field keeps every character that starts inside it, and the
// left keeps only what fits. In a single-byte locale the letter changes
// nothing, as for the length.
func columnPadding(e *syntax.ParamExpr) func(string) int {
	switch strings.Count(e.Flags, "m") {
	case 0:
		return nil
	case 1:
		return displayColumns
	}
	return func(u string) int {
		if displayColumns(u) == 0 {
			return 0
		}
		return 1
	}
}

// padInColumns is the padding pair measured in columns. See columnPadding.
func (r *Runner) padInColumns(w string, lw, rw int, lp, rp padding, col func(string) int) string {
	switch {
	case lw > 0 && rw > 0:
		u := r.units(w)
		half := len(u) / 2
		return r.padLeftInColumns(u[:half], lw, lp, col) + r.padRightInColumns(u[half:], rw, rp, col)
	case lw > 0:
		return r.padLeftInColumns(r.units(w), lw, lp, col)
	case rw > 0:
		return r.padRightInColumns(r.units(w), rw, rp, col)
	}
	return w
}

// padRightInColumns keeps the characters that start inside the field, and
// fills what is left the same way, a wide fill crossing the edge.
func (r *Runner) padRightInColumns(u []string, width int, p padding, col func(string) int) string {
	var b strings.Builder
	n := 0
	for _, c := range u {
		if n >= width {
			return b.String()
		}
		b.WriteString(c)
		n += col(c)
	}
	for _, c := range r.units(p.insert) {
		if n >= width {
			return b.String()
		}
		b.WriteString(c)
		n += col(c)
	}
	fill := r.units(p.fill)
	for i := 0; n < width && len(fill) > 0; i++ {
		c := fill[i%len(fill)]
		if col(c) == 0 {
			break
		}
		b.WriteString(c)
		n += col(c)
	}
	return b.String()
}

// padLeftInColumns keeps, from the right, only the characters that fit, and
// fills from the right the same way. A fill that cannot land exactly on the
// field's edge leaves nothing at all, which is measured and is that shell's
// own edge: see columnPadding.
func (r *Runner) padLeftInColumns(u []string, width int, p padding, col func(string) int) string {
	kept, n := []string(nil), 0
	for i := len(u) - 1; i >= 0; i-- {
		if n+col(u[i]) > width {
			return strings.Join(reverseUnits(kept), "")
		}
		kept = append(kept, u[i])
		n += col(u[i])
	}
	ins := r.units(p.insert)
	for i := len(ins) - 1; i >= 0; i-- {
		if n+col(ins[i]) > width {
			return strings.Join(reverseUnits(kept), "")
		}
		kept = append(kept, ins[i])
		n += col(ins[i])
	}
	fill := r.units(p.fill)
	for i := 0; n < width && len(fill) > 0; i++ {
		c := fill[len(fill)-1-i%len(fill)]
		if col(c) == 0 {
			break
		}
		if n+col(c) > width {
			return ""
		}
		kept = append(kept, c)
		n += col(c)
	}
	return strings.Join(reverseUnits(kept), "")
}

// reverseUnits is the units in the other order.
func reverseUnits(u []string) []string {
	out := make([]string, len(u))
	for i, c := range u {
		out[len(u)-1-i] = c
	}
	return out
}
