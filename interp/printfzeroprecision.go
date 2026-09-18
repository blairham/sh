// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// printfZeroFillShape reports whether this conversion is the one arrangement
// the `0` flag and a precision can disagree about — the flag, a width, a
// precision, an integer verb, and no `-`, which voids a zero fill everywhere
// — and answers the spec with its *width* taken out.
//
// The width is removed rather than the precision, and the axis's last row is
// what says that is right: `%08.10d` of 7 is the same ten digits in every
// column, so the column that keeps the flag applies the precision and then
// fills the *field*. Rendering the narrow spec and padding what comes back is
// therefore the whole of it, through the same printfPadToWidth every other
// laid-out field uses, which is what keeps a sign in front of the fill.
//
// It asks no dialect anything: whether there is a fill to place is not known
// until the field has been rendered, and the two readings coincide where
// there is none. See Runner.printfZeroFlagSurvivesAPrecision, which is the
// question, and Semantics.PrintfZeroFlagSurvivesAPrecision.
func printfZeroFillShape(spec string, verb byte) (narrow string, width int, ok bool) {
	flags, width, prec := printfSpecParts(spec)
	if width == 0 || prec < 0 || strings.ContainsRune(flags, '-') ||
		!strings.ContainsRune(flags, '0') {
		return "", 0, false
	}
	switch verb {
	case 'd', 'i', 'o', 'u', 'x', 'X':
	default:
		return "", 0, false
	}
	i := 1 + len(flags)
	j := i + printfFieldRun(spec, i)
	return spec[:i] + spec[j:], width, true
}

// printfZeroFlagSurvivesAPrecision is the axis, asked of a field that has a
// fill to place.
func (r *Runner) printfZeroFlagSurvivesAPrecision() bool {
	return r.ask(r.sem().PrintfZeroFlagSurvivesAPrecision,
		"`printf` keeping the `0` flag's fill where a precision is written on an integer conversion")
}

// printfZeroFlagIsIgnored reports whether C's rule applies to a field the
// wide-width path is about to lay out: the `0` flag is ignored where a
// precision is written on an integer conversion.
//
// The wide path read the flag at face value, so `printf '%010000020.3d' 7`
// was zero-filled in every dialect where six of the seven columns fill it
// with blanks — the same question one band up (#3067).
func (r *Runner) printfZeroFlagIsIgnored(flags string, prec int, verb byte) bool {
	if prec < 0 || !strings.ContainsRune(flags, '0') || strings.ContainsRune(flags, '-') {
		return false
	}
	switch verb {
	case 'd', 'i', 'o', 'u', 'x', 'X':
	default:
		return false
	}
	return !r.printfZeroFlagSurvivesAPrecision()
}
