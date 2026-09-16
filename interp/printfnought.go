// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
)

// printfNoughtField lays out an integer conversion of **nought** here, in the
// rows where Go's `fmt` and C disagree about it, and answers false in every
// other row so that `fmt` goes on doing the work.
//
// The disagreement is a small, closed set and it is bounded by a measurement
// rather than by a reading of the two specifications.
// TestTheWidePrecisionRenderersAgreeWithFmt grades a renderer written from C
// against `fmt` over every flag, width, precision and value the integer band
// can carry, and the only rows it has to skip are a value of nought carrying
// one of `+`, ` ` or `#` (#3024). So those are the rows this takes, and
// taking any other would be changing an answer nothing said was wrong.
//
// There are two of them and they are not the same kind of thing:
//
//   - **The sign at a precision of nought.** C says a precision of 0 with a
//     value of 0 produces *no digits*, and says separately that `+` and the
//     space flag write a sign for a signed conversion. Both hold at once, so
//     `printf '%+.0d' 0` is one character and not none. Every column on the
//     panel writes it — bash 5.3.20, bash as sh, bash 3.2.57, zsh 5.9.2,
//     ksh93u+, dash and BusyBox ash, measured 2026-09-15 — and `fmt` writes
//     the empty string, which is this shell alone against seven.
//
//   - **The alternate form at nought**, which is an axis and not a defect:
//     see Semantics.PrintfAlternateFormAsksTheValue. Six columns read C and
//     ksh93 does not, and ours already answered as ksh93 does — so a blanket
//     "match C" would have broken the one column that was passing.
//
// Nothing here writes a `0x`, and that is load-bearing rather than a
// coincidence: the C reading takes the prefix *off* at nought, and the ksh93
// reading — which keeps it — never reaches this function, so the field laid
// out below is a sign and digits and printfPadToWidth can place a `0` fill
// without knowing about a prefix it will never see. Where the padding would
// have to land inside a `0x` the answer is still `fmt`'s, which is a separate
// question with a separate divergence of its own (#3066).
//
// spec is the conversion as the caller will hand it to `fmt`: the unsigned
// conversions have already had their sign flags taken out by
// printfWithoutSignFlags, which is why the sign below can be read straight
// off the flags without asking whether the verb has one.
func (r *Runner) printfNoughtField(spec string, verb byte, n int64) (string, bool) {
	if n != 0 {
		return "", false
	}
	flags, width, prec := printfSpecParts(spec)
	sign := ""
	switch {
	case strings.ContainsRune(flags, '+'):
		sign = "+"
	case strings.ContainsRune(flags, ' '):
		sign = " "
	}
	// `#` is C's alternate form, and it means something only on the three
	// conversions that have one. A `%d` is not one of them, so a `%#d` of
	// nought is left where it was rather than asked about.
	alternate := strings.ContainsRune(flags, '#') &&
		(verb == 'o' || verb == 'x' || verb == 'X')

	switch {
	case alternate:
		if !r.ask(r.sem().PrintfAlternateFormAsksTheValue,
			"`printf` reading C's `#` off the value at a value of nought") {
			return "", false
		}
	case sign != "" && prec == 0:
		// The row above: the digits are erased and the sign is not.
	default:
		return "", false
	}

	// C's digits for a nought: none at all where the precision is nought,
	// the precision's worth of zeros where it states one, and the single
	// digit the value has where the spec states no precision.
	digits := "0"
	switch {
	case prec == 0:
		digits = ""
	case prec > 0:
		digits = strings.Repeat("0", prec)
	}

	prefix := ""
	if alternate && verb == 'o' && !strings.HasPrefix(digits, "0") {
		// C's `#` on an octal raises the precision until there is a leading
		// zero. A run of zeros already has one; an *empty* run does not, and
		// that is the whole of `printf '%#.0o' 0` being `0` and not nothing.
		prefix = "0"
	}

	// The `0` flag is ignored where an integer conversion states a precision,
	// which is C and is unanimous on the panel: `printf '%+05.0d' 0` pads
	// with blanks in six of the seven columns. ksh93 is the seventh and pads
	// with zeros there whatever the precision says, which is a reading of its
	// own and is filed rather than modeled (#3067).
	zero := prec < 0 && strings.ContainsRune(flags, '0')
	return printfPadToWidth(sign+prefix+digits, width,
		strings.ContainsRune(flags, '-'), zero), true
}
