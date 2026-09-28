// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"strings"
	"time"
)

// The **second** vocabulary a `time` format can be written in.
//
// Two shells read a format variable in the vocabulary interp/timeformat.go
// implements — `%[p][l]R`, `%U`, `%S`, `%P` — and a third reads one that
// shares only `%P` and `%%` with it. Sharing a reader between them was never
// possible: `%U` means the same thing in both and *renders* differently,
// `%R` is not a directive in this one at all, and the digit that is a
// precision there is literal text here. So this is a reader of its own
// behind Diagnostics.TimeFormatVerbs, and the field's two values are the two
// vocabularies rather than the shells.
//
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh`, `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)` — `go version -m` says *not a Go executable*
// for it, so the reference is a different program from ours — script files
// run `-f` under `env -i PATH=/usr/bin:/bin`, `time sleep 0.123456` in one
// run:
//
//	%E  0.13s      %*E  0.134      %mE  134ms   %uE  134015us
//	%U  0.00s      %*U  0.000      %mU  0ms     %uU  412us
//	%S  0.00s      %*S  0.001      %P   0%      %J   sleep 0.123456
//
// So the bare letter is **seconds to two places with an `s` after it**, `*`
// is a plain decimal to three places with no unit, `m` is whole
// milliseconds and `u` whole microseconds — all four rounded rather than
// truncated, which `%mE` of 3600µs answering `4ms` in the same run is what
// says.
//
// # An unrecognized directive is not a complaint
//
// The other vocabulary refuses the whole report over one bad directive and
// names it. This one prints the `%` and carries on from the character after
// it, so the text comes out as written — measured in the same run:
//
//	%x   %x       %Q   %Q       %9E  %9E      %lE  %lE
//	%*J  %*J      %mP  %mP      %*P  %*P      %%   %
//
// The last four rows are what make this a re-scan rather than an "echo the
// run": `*`, `m` and `u` are modifiers only in front of `E`, `U` and `S`, so
// `%*P` is a `%` that matched nothing followed by the literal `*P`. Reading
// it as "skip to the letter" would have printed the same thing here and
// something different for a format with a modifier and no letter at all.
//
// And a `%` at the very **end** of the format writes nothing, where the
// other vocabulary writes a literal `%`: measured, `TIMEFMT='trailing %'`
// is `trailing ` and a newline.

// timeFormatWithUnits renders one report line in that vocabulary.
//
// It cannot fail, which is the other half of the difference from
// timeFormatReport: there is no bad directive to refuse over.
//
// `text` is what `%J` writes — the pipeline element as it was written, or
// the word a bare `time`'s own two lines are labeled with.
func timeFormatWithUnits(format, text string, elapsed, user, sys time.Duration) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' {
			b.WriteByte(c)
			continue
		}
		if i+1 >= len(format) {
			// A `%` with nothing after it is dropped rather than written.
			break
		}
		rest := format[i+1:]
		if rest[0] == '%' {
			b.WriteByte('%')
			i++
			continue
		}
		if rest[0] == 'J' {
			b.WriteString(text)
			i++
			continue
		}
		if rest[0] == 'P' {
			// Whole percent with the sign after it, and nought where no
			// time passed — a division with nothing to divide by. Truncated
			// rather than rounded, which is the reading the default format
			// already agreed with byte for byte.
			pct := 0
			if elapsed > 0 {
				pct = int(float64(user+sys) / float64(elapsed) * 100)
			}
			fmt.Fprintf(&b, "%d%%", pct)
			i++
			continue
		}
		// A modifier, then the figure it applies to. The modifier is
		// optional and is only a modifier in front of one of the three
		// letters — see the note above, where `%*P` is measured.
		modifier, letter := byte(0), rest[0]
		if letter == '*' || letter == 'm' || letter == 'u' {
			if len(rest) < 2 {
				b.WriteByte('%')
				continue
			}
			modifier, letter = rest[0], rest[1]
		}
		d, ok := elapsed, true
		switch letter {
		case 'E':
		case 'U':
			d = user
		case 'S':
			d = sys
		default:
			ok = false
		}
		if !ok {
			// Nothing matched. The `%` goes out and the scan carries on
			// from the character after it, so a modifier that led nowhere
			// is literal text.
			b.WriteByte('%')
			continue
		}
		b.WriteString(timeFigureWithUnits(d, modifier))
		i += 1
		if modifier != 0 {
			i++
		}
	}
	return b.String()
}

// timeFigureWithUnits is one figure in the unit the modifier asked for.
func timeFigureWithUnits(d time.Duration, modifier byte) string {
	switch modifier {
	case '*':
		return fmt.Sprintf("%.3f", d.Seconds())
	case 'm':
		return fmt.Sprintf("%dms", int64(float64(d)/float64(time.Millisecond)+0.5))
	case 'u':
		return fmt.Sprintf("%dus", int64(float64(d)/float64(time.Microsecond)+0.5))
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

// TimeFormatVerbs is which vocabulary a `time` format variable is written in.
//
// Two of them, and they are vocabularies rather than shells: what decides is
// the set of directives and how a figure renders, not which column asked. See
// the note at the top of this file for the measurement that separates them.
type TimeFormatVerbs uint8

const (
	// TimeFormatVerbsRealUserSys is `%%`, `%P` and `%[precision][l]` before
	// `R`, `U` or `S` — bash and ksh93, which read it identically. A
	// directive outside the set refuses the whole report and is named.
	TimeFormatVerbsRealUserSys TimeFormatVerbs = iota
	// TimeFormatVerbsElapsedWithUnits is `%%`, `%P`, `%J` and
	// `%[*|m|u]` before `E`, `U` or `S` — zsh, where the bare letter carries
	// its unit, the elapsed figure is `E` rather than `R`, and a directive
	// outside the set is written out rather than refused.
	TimeFormatVerbsElapsedWithUnits
)

// String names the vocabulary, so a failing test says which one it meant.
func (v TimeFormatVerbs) String() string {
	if v == TimeFormatVerbsElapsedWithUnits {
		return "TimeFormatVerbsElapsedWithUnits"
	}
	return "TimeFormatVerbsRealUserSys"
}
