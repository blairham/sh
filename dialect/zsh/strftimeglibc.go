// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strconv"
	"strings"
	"time"
)

// The C library's half of zsh's strftime, as the GNU C library writes it.
//
// zsh hands every conversion it does not own to the C library's strftime, so
// what a flag, a width or a modifier does is the library's answer and not the
// shell's — and the two builds of zsh 5.9.2 this repository grades against
// carry two different libraries. The macOS build writes most of the extensions
// as text; the Debian build in the suite's image (`ghcr.io/blairham/sh/zsh@
// sha256:aab8255c…`) has the GNU set. #5160 chose the GNU set, because that
// image is what CI grades the zsh column against: the coordinator's call.
//
// Everything here was measured on that image, `LC_ALL=C`, over a grid of every
// letter under each of the flags `-`, `_`, `0`, `^` and `#`, the widths 1, 5
// and 12 and the modifiers `E` and `O` — 4144 specs at four instants, and the
// time zones UTC, America/New_York and Asia/Kolkata for `%z` and `%Z`. The
// rules below are that table stated as rules; strftimeglibc_test.go holds rows
// of it.
//
// A spec is `%`, then any of the flags, then a width, then a modifier, then the
// conversion:
//
//   - `_`, `-` and `0` choose the padding and the last one written wins:
//     spaces, none, zeros. `-` removes only the *natural* padding — `%-d` is
//     `6` — and a width still pads, with spaces: `%-5d` is `    6`.
//   - `^` upper-cases the result. `#` swaps the case of the names — `%#a` is
//     `WED`, `%#p` is `am`, `%#Z` is `utc` — and wins over `^` where the two
//     disagree.
//   - A width pads on the left: with zeros for a number unless a flag said
//     otherwise, with spaces for text unless `0` did. A result already as
//     long as the width is written whole.
//   - `E` and `O` are accepted where the C locale has an alternative, which
//     it writes the same as the plain conversion, and make the spec text
//     anywhere else. Two modifiers are always text.
//   - A spec the library does not know is written as the text it was —
//     `%q` is `%q`, `%Ed` is `%Ed` — with `^` upper-casing that text and a
//     width padding it, which is why `%012Ed` is `000000%012Ed`.
//   - A spec the format ends inside, `%5` or `%E` at the very end, is a `%`.
func glibcStrftime(spec string, flags glibcFlags, t time.Time) string {
	if flags.conv == 0 {
		return "%"
	}
	if flags.mods > 1 || (flags.mod != 0 && !glibcModifierTaken(flags.mod, flags.conv)) {
		return glibcText(spec, flags, true)
	}
	switch flags.conv {
	case 'a':
		return glibcName(t.Weekday().String()[:3], flags, true)
	case 'A':
		return glibcName(t.Weekday().String(), flags, true)
	case 'b', 'h':
		return glibcName(t.Month().String()[:3], flags, true)
	case 'B':
		return glibcName(t.Month().String(), flags, true)
	case 'p':
		return glibcName(meridiem(t, true), flags, false)
	case 'P':
		// Always lower case: `%^P` is `am` as well, the one name neither flag
		// moves.
		return glibcName(meridiem(t, false), glibcFlags{pad: flags.pad, width: flags.width, widthWrote: flags.widthWrote}, false)
	case 'Z':
		zone, _ := t.Zone()
		return glibcName(zone, flags, false)
	case 'c':
		return glibcText(glibcComposite("%a %b %e %H:%M:%S %Y", t), flags, false)
	case 'D', 'x':
		return glibcText(glibcComposite("%m/%d/%y", t), flags, false)
	case 'F':
		return glibcText(glibcComposite("%Y-%m-%d", t), flags, false)
	case 'R':
		return glibcText(glibcComposite("%H:%M", t), flags, false)
	case 'T', 'X':
		return glibcText(glibcComposite("%H:%M:%S", t), flags, false)
	case 'r':
		return glibcText(glibcComposite("%I:%M:%S %p", t), flags, false)
	case 'n':
		return glibcText("\n", flags, false)
	case 't':
		return glibcText("\t", flags, false)
	case '%':
		return glibcText("%", flags, false)
	case 'z':
		return glibcZone(t, flags)
	case 's':
		return glibcNumber(strconv.FormatInt(t.Unix(), 10), 1, ' ', flags)
	}
	if n, width, pad, ok := glibcNumeric(flags.conv, t); ok {
		return glibcNumber(strconv.Itoa(n), width, pad, flags)
	}
	return glibcText(spec, flags, true)
}

// glibcFlags is one spec, read.
type glibcFlags struct {
	pad        byte // '_', '-', '0', or 0 for the conversion's own
	upper      bool // `^`
	swap       bool // `#`
	width      int  // -1 where none was written
	mod        byte // 'E', 'O' or 0
	mods       int  // how many modifiers were written
	conv       byte // 0 where the format ended inside the spec
	widthWrote bool
}

// readGlibcSpec reads the spec starting at format[i], which is the `%`, and
// answers it and the index of its last byte.
func readGlibcSpec(format string, i int) (glibcFlags, int) {
	f := glibcFlags{width: -1}
	j := i + 1
	for ; j < len(format); j++ {
		switch c := format[j]; c {
		case '_', '-', '0':
			f.pad = c
		case '^':
			f.upper = true
		case '#':
			f.swap = true
		default:
			goto width
		}
	}
width:
	if j < len(format) && format[j] >= '1' && format[j] <= '9' {
		w := 0
		for ; j < len(format) && format[j] >= '0' && format[j] <= '9'; j++ {
			if w < 1<<20 {
				w = w*10 + int(format[j]-'0')
			}
		}
		f.width, f.widthWrote = w, true
	}
	for ; j < len(format) && (format[j] == 'E' || format[j] == 'O'); j++ {
		f.mod = format[j]
		f.mods++
	}
	if j >= len(format) {
		return f, len(format) - 1
	}
	f.conv = format[j]
	return f, j
}

// glibcModifierTaken is whether the C locale has an alternative for this
// conversion under this modifier — measured letter by letter, since the set is
// neither POSIX's nor the same for the two.
func glibcModifierTaken(mod, conv byte) bool {
	if mod == 'E' {
		return strings.IndexByte("cnprstuxyzCPRTXYZ%", conv) >= 0
	}
	return strings.IndexByte("bdeghjklmnprstuwyzBCGHIMPRSTUVWZ%", conv) >= 0
}

// glibcNumeric is a numeric conversion's value, its natural width and its
// natural padding.
func glibcNumeric(conv byte, t time.Time) (n, width int, pad byte, ok bool) {
	isoYear, isoWeek := t.ISOWeek()
	switch conv {
	case 'C':
		return floorDiv(t.Year(), 100), 2, '0', true
	case 'd':
		return t.Day(), 2, '0', true
	case 'e':
		return t.Day(), 2, ' ', true
	case 'g':
		return ((isoYear % 100) + 100) % 100, 2, '0', true
	case 'G':
		return isoYear, 1, '0', true
	case 'H':
		return t.Hour(), 2, '0', true
	case 'I':
		return twelveHour(t.Hour()), 2, '0', true
	case 'j':
		return t.YearDay(), 3, '0', true
	case 'k':
		return t.Hour(), 2, ' ', true
	case 'l':
		return twelveHour(t.Hour()), 2, ' ', true
	case 'm':
		return int(t.Month()), 2, '0', true
	case 'M':
		return t.Minute(), 2, '0', true
	case 'S':
		return t.Second(), 2, '0', true
	case 'u':
		wd := int(t.Weekday())
		if wd == 0 {
			wd = 7
		}
		return wd, 1, '0', true
	case 'U':
		return (t.YearDay() + 6 - int(t.Weekday())) / 7, 2, '0', true
	case 'V':
		return isoWeek, 2, '0', true
	case 'w':
		return int(t.Weekday()), 1, '0', true
	case 'W':
		return (t.YearDay() + 6 - (int(t.Weekday())+6)%7) / 7, 2, '0', true
	case 'y':
		return ((t.Year() % 100) + 100) % 100, 2, '0', true
	case 'Y':
		return t.Year(), 1, '0', true
	}
	return 0, 0, 0, false
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// glibcNumber pads a number's digits: to its natural width with its natural
// padding, or to a written width.
func glibcNumber(digits string, natural int, pad byte, f glibcFlags) string {
	neg := strings.HasPrefix(digits, "-")
	if neg {
		digits = digits[1:]
	}
	width := natural
	switch f.pad {
	case '-':
		width = 0
		pad = ' '
	case '_':
		pad = ' '
	case '0':
		pad = '0'
	}
	// A written width is a floor and not a size: `%1d` is still `06`. Only
	// `-` takes the natural width away, so `%-1d` is `6`.
	if f.widthWrote && f.width > width {
		width = f.width
	}
	if neg {
		width--
	}
	if n := width - len(digits); n > 0 {
		if pad == '0' {
			digits = strings.Repeat("0", n) + digits
			if neg {
				digits = "-" + digits
			}
			return digits
		}
		if neg {
			digits = "-" + digits
		}
		return strings.Repeat(" ", n) + digits
	}
	if neg {
		digits = "-" + digits
	}
	return digits
}

// glibcName is a name — a weekday, a month, a meridiem, a zone — with the case
// flags applied and then a width. upperBySwap says which way `#` turns it:
// up for the day and month names, down for the meridiem and the zone.
func glibcName(s string, f glibcFlags, upperBySwap bool) string {
	switch {
	case f.swap && upperBySwap:
		s = strings.ToUpper(s)
	case f.swap:
		s = strings.ToLower(s)
	case f.upper:
		s = strings.ToUpper(s)
	}
	return glibcPad(s, f)
}

// glibcText is text with `^` applied and then a width. literal says it is the
// spec itself being written back, where `#` is measured to do nothing.
func glibcText(s string, f glibcFlags, literal bool) string {
	// Written back as text, the spec still takes the case its conversion
	// would have, or most of it: `%^Ed` is `%^ED`, and `%#Eb` is `%#EB`
	// where `%#Ea` and `%#Ed` stay as they were — measured, and the two month
	// letters are the only ones `#` reaches here.
	if f.upper || (literal && f.swap && (f.conv == 'b' || f.conv == 'h')) {
		s = strings.ToUpper(s)
	}
	return glibcPad(s, f)
}

// glibcPad pads text to a written width: with zeros where `0` was the padding
// flag, with spaces otherwise — `-` included, since text has no natural
// padding for it to take away.
func glibcPad(s string, f glibcFlags) string {
	if !f.widthWrote || len(s) >= f.width {
		return s
	}
	pad := " "
	if f.pad == '0' {
		pad = "0"
	}
	return strings.Repeat(pad, f.width-len(s)) + s
}

// glibcComposite writes one of the conversions that stand for several.
func glibcComposite(format string, t time.Time) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			b.WriteByte(format[i])
			continue
		}
		f, end := readGlibcSpec(format, i)
		b.WriteString(glibcStrftime(format[i:end+1], f, t))
		i = end
	}
	return b.String()
}

// glibcZone is `%z`, which pads twice over when a width is written: measured,
// `%5z` at UTC is `    +00000` — the width less one in front of the sign, and
// the digits padded to the width behind it — while `%z` is `+0000`, `%-z`
// `+0` and `%_z` `+   0`.
func glibcZone(t time.Time, f glibcFlags) string {
	_, off := t.Zone()
	sign := "+"
	if off < 0 {
		sign, off = "-", -off
	}
	hhmm := (off/3600)*100 + (off%3600)/60
	digits := strconv.Itoa(hhmm)
	width, pad := 4, byte('0')
	switch f.pad {
	case '-':
		width, pad = 0, ' '
	case '_':
		pad = ' '
	}
	lead := ""
	if f.widthWrote {
		if f.width > width || f.pad == '-' {
			width = f.width
		}
		fill := " "
		if f.pad == '0' {
			fill = "0"
		}
		if f.width > 1 {
			lead = strings.Repeat(fill, f.width-1)
		}
	}
	if n := width - len(digits); n > 0 {
		digits = strings.Repeat(string(pad), n) + digits
	}
	return lead + sign + digits
}

// meridiem is AM or PM, in upper case or lower.
func meridiem(t time.Time, upper bool) string {
	m := "AM"
	if t.Hour() >= 12 {
		m = "PM"
	}
	if !upper {
		m = strings.ToLower(m)
	}
	return m
}
