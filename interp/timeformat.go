// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"strings"
	"time"
)

// $TIMEFORMAT, which is how a script asks the `time` keyword for a shape
// other than the default one.
//
// Two shells read it — bash and ksh93 — and they read it identically, which
// is why this is one implementation behind Diagnostics.TimeFormatVariable
// rather than two. zsh has a variable for the same job under another name and
// a different vocabulary; dash has none. See the field's own note.
//
// It matters more than its size suggests. The default report is three lines
// with a blank one in front, and a script that wants one machine-readable
// line sets this — so a shell that ignored the variable answered every such
// script with four lines of something else, at status 0. That is the shape
// the difference took here: the variable was not read at all.
//
// Measured 2026-09-12 on bash 5.3.15 and ksh93u+, byte for byte:
//
//	TIMEFORMAT='|%R|%0R|%1R|%2R|%3R|%4R|'   |0.129|0|0.1|0.13|0.129|0.1292|
//	TIMEFORMAT='|%lR|%0lR|%1lR|%3lR|'       |1m1.003s|1m1s|1m1.0s|1m1.003s|
//	TIMEFORMAT='|%U|%S|%P|%%|'              |0.001|0.001|71.00|%|
//	TIMEFORMAT='a\nb %R'                    a\nb 0.001
//	TIMEFORMAT=''                           nothing at all
//	TIMEFORMAT='%Q'                         a complaint, and no report
//
// The fourth row is the one worth pinning: backslash escapes are **not**
// expanded. A format wanting a newline in it is written `$'\n…'`, which the
// word does before the variable ever holds it — and the default value is
// spelled that way for exactly that reason.

// timeFormatDirectives is the set a `%` may be followed by, after an optional
// precision digit and an optional `l`.
const timeFormatDirectives = "RUS"

// timeFormatReport renders one report through a format, and reports whether
// the format was usable. A format with a directive nobody has produces a
// complaint and *no* report, which is what both shells do.
func (r *Runner) timeFormatReport(format string, elapsed, user, sys time.Duration) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' {
			b.WriteByte(c)
			continue
		}
		// A `%` at the very end is a literal `%`, in both shells: measured,
		// `TIMEFORMAT='trailing %'` prints `trailing %`.
		if i+1 >= len(format) {
			b.WriteByte('%')
			break
		}
		i++
		if format[i] == '%' {
			b.WriteByte('%')
			continue
		}
		// The precision, then the long-form marker, then the directive. Both
		// are optional and the order is fixed — `%l3R` is not a directive in
		// either shell.
		precision, long := -1, false
		if format[i] >= '0' && format[i] <= '9' {
			precision = int(format[i] - '0')
			if precision > timeFormatMaxPrecision {
				// Capped rather than refused: measured, `%9R` and `%6R`
				// print the same six decimals.
				precision = timeFormatMaxPrecision
			}
			i++
		}
		if i < len(format) && format[i] == 'l' {
			long = true
			i++
		}
		if i >= len(format) || !strings.ContainsRune(timeFormatDirectives+"P", rune(format[i])) {
			what := ""
			if i < len(format) {
				what = string(format[i])
			}
			r.diagf("%s\n", Wording(r.diag().TimeFormatBadDirective,
				"%[1]s: `%[2]s': invalid format character",
				r.diag().TimeFormatVariable, what))
			return "", false
		}
		if format[i] == 'P' {
			// The CPU percentage, two decimals, and zero where no time
			// passed — a division that has nothing to divide by.
			pct := 0.0
			if elapsed > 0 {
				pct = float64(user+sys) / float64(elapsed) * 100
			}
			fmt.Fprintf(&b, "%.2f", pct)
			continue
		}
		d := elapsed
		switch format[i] {
		case 'U':
			d = user
		case 'S':
			d = sys
		}
		b.WriteString(timeFormatDuration(d, precision, long))
	}
	return b.String(), true
}

// timeFormatMaxPrecision is where the digit stops meaning anything: `%7R` and
// `%9R` both print the six decimals `%6R` does.
const timeFormatMaxPrecision = 6

// timeFormatDuration is one figure, in the plain form or the long one.
//
// The plain form is seconds and nothing else, so a minute past the hour is
// `61.002` rather than a clock reading. The long form always writes the
// minutes, `0m0.510s` for half a second, which is what makes the default
// report's `1m1.003s` and its `0m0.001s` the same shape.
func timeFormatDuration(d time.Duration, precision int, long bool) string {
	if precision < 0 {
		precision = 3
	}
	secs := d.Seconds()
	if !long {
		return fmt.Sprintf("%.*f", precision, secs)
	}
	minutes := int(secs) / 60
	return fmt.Sprintf("%dm%.*fs", minutes, precision, secs-float64(minutes*60))
}

// timeFormat is the format in force, and whether the dialect has one to read
// at all.
//
// Unset is not the empty string: an unset variable leaves the dialect's
// default layout, and a variable *set* to the empty string prints nothing —
// measured, a TIMEFORMAT assigned an empty word writes not even a newline.
func (r *Runner) timeFormat() (string, bool) {
	name := r.diag().TimeFormatVariable
	if name == "" {
		return "", false
	}
	v, ok := r.getVar(name)
	if !ok {
		return "", false
	}
	return v, true
}
