// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"
)

// printfAlternatePrefixField lays out a hexadecimal conversion whose `#` wrote
// a `0x` and whose `0` flag has a fill to place, which is the one arrangement
// where Go's `fmt` and C disagree about how wide the answer is.
//
// C counts the prefix as part of the field: the fill is the width less the
// `0x` and less the digits, so `printf '%#05x' 7` is `0x007` and is five
// characters. `fmt` pads the *digits* to the width and writes the prefix past
// it, so the same call is `0x00007` and is seven. Both put the fill between
// the prefix and the digits — the disagreement is the arithmetic and not the
// placement — and it is a reading rather than a defect, because ksh93u+ takes
// `fmt`'s side of it (#3066). See
// Semantics.PrintfZeroFillCountsTheAlternatePrefix.
//
// The conditions below are each a measurement and not a guard bolted on:
//
//   - **`%x` and `%X` only.** An octal's alternate form is a leading `0`,
//     which is indistinguishable from the fill it would be counted against:
//     `printf '%#06o' 255` is `000377` in all seven columns. So an octal has
//     no reading to have, and asking a dialect about one would invent a
//     divergence.
//   - **The `0` flag, and no `-`.** A *space* fill goes in front of the whole
//     field in every column — `printf '%#8x' 7` is unanimous — and `-` voids
//     a zero fill outright, which C says and all seven do.
//   - **No precision.** A precision written on an integer conversion ignores
//     the `0` flag in C, in `fmt` and in six of the seven columns, so there
//     is no fill here to place. ksh93 is the seventh and honors it, which is
//     a second reading with its own report (#3067) and not this one.
//   - **A nonzero value.** Whether there is a prefix at all at a nought is
//     PrintfAlternateFormAsksTheValue, and the two never both bite: the
//     columns that keep a `0x` there are the columns that do not count it.
//     printfNoughtField has already answered that row.
//   - **A width wider than the digits.** This is where the two readings part
//     and it is not where the *prefix* would overflow: `printf '%#05x' 65535`
//     is `0xffff` under both, because C's fill is the width less six and
//     `fmt`'s is the width less four and both come out empty. They differ
//     from the first character the width asks for past the digits — the
//     fill is two shorter under C and never shorter than nothing — so
//     `width > len(digits)` is exactly the set that has a reading, and
//     `printf '%#3x' 7` asks no dialect anything.
//
// spec has already had its sign flags taken out by printfWithoutSignFlags,
// which is what makes a hexadecimal's field a prefix and digits and nothing
// else.
func (r *Runner) printfAlternatePrefixField(spec string, verb byte, n int64) (string, bool) {
	if verb != 'x' && verb != 'X' {
		return "", false
	}
	flags, width, prec := printfSpecParts(spec)
	if !strings.ContainsRune(flags, '#') || !strings.ContainsRune(flags, '0') ||
		strings.ContainsRune(flags, '-') || prec >= 0 {
		return "", false
	}
	// The operand's bit pattern and not its magnitude, as everywhere an
	// unsigned conversion reads one: `printf '%#050x' -1` is sixteen `f`s
	// and the fill around them, not a negative number.
	v := uint64(n)
	if v == 0 {
		return "", false
	}
	digits := strconv.FormatUint(v, 16)
	prefix := "0x"
	if verb == 'X' {
		digits, prefix = strings.ToUpper(digits), "0X"
	}
	if width <= len(digits) {
		return "", false
	}
	// C's fill, which the prefix has already been taken out of. It goes
	// empty two characters before `fmt`'s does rather than going negative.
	fill := max(0, width-len(prefix)-len(digits))
	if !r.ask(r.sem().PrintfZeroFillCountsTheAlternatePrefix,
		"`printf` counting the `0x` C's `#` wrote against the width a `0` flag fills") {
		return "", false
	}
	return prefix + strings.Repeat("0", fill) + digits, true
}

// printfPadWideField lays a rendered field out to a width `fmt` refused,
// putting a zero fill *inside* an alternate prefix the same way the
// conversion just below the ceiling does.
//
// The two sides of a ceiling have to agree, and they did not: past it the
// width was taken out of the spec, `fmt` wrote a bare `0x7`, and the fill
// went in front of the whole thing — `printf '%#010000020x' 7` was ten
// million zeros and then `0x7`, which is neither reading of
// PrintfZeroFillCountsTheAlternatePrefix. Both of those put the fill between
// the prefix and the digits and differ only in the arithmetic, so what this
// needs from the dialect is the same answer printfAlternatePrefixField asks
// for (#3089).
func (r *Runner) printfPadWideField(field string, verb byte, width int, left, zero bool) string {
	if !zero || left || (verb != 'x' && verb != 'X') || len(field) < 2 ||
		field[0] != '0' || (field[1] != 'x' && field[1] != 'X') {
		return printfPadToWidth(field, width, left, zero)
	}
	prefix, digits := field[:2], field[2:]
	// C counts the prefix against the width and the other reading pads the
	// digits to it, which is two characters wider. The narrow path answers
	// the same question; asking it here is what makes the ceiling invisible.
	fill := width - len(digits)
	if r.ask(r.sem().PrintfZeroFillCountsTheAlternatePrefix,
		"`printf` counting the `0x` C's `#` wrote against the width a `0` flag fills") {
		fill = width - len(prefix) - len(digits)
	}
	if fill <= 0 {
		return field
	}
	return prefix + strings.Repeat("0", fill) + digits
}
