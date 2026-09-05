// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"
	"time"
)

// The date conversions `printf '%(fmt)T'` writes.
//
// Go has no strftime, so this is one: written from the POSIX XCU description
// of the conversion specifications and checked against a live shell, which is
// the rule in CLEANROOM.md — behavior from the standard and from an oracle
// run, never from somebody's C.
//
// The locale is the C locale throughout. Every panel member was measured with
// LC_ALL=C, and this engine has no locale of its own to consult: %c, %x and %X
// are the C locale's forms, and the month and day names are English. A shell
// embedding this and wanting a locale would want a hook, which nothing has
// asked for yet.

// strftime writes t through a POSIX date format.
//
// A conversion this does not have keeps its letter and loses the `%`, which is
// what the platform this was measured on does with one its own strftime does
// not know — `%(%i)T` writes `i`. That is the C library's answer rather than
// the shell's, and it is not the same on every system: glibc has letters this
// does not. Recorded in docs/spec/semantics.md rather than pinned by a case.
func strftime(format string, t time.Time) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			b.WriteByte(format[i])
			continue
		}
		i++
		// `%E` and `%O` are the locale's alternative forms, and POSIX allows
		// a shell to ignore the modifier. The C locale has no alternatives,
		// so the letter behind it is the whole answer.
		if (format[i] == 'E' || format[i] == 'O') && i+1 < len(format) {
			i++
		}
		b.WriteString(strftimeConversion(format[i], t))
	}
	return b.String()
}

// strftimeConversion is one conversion specification.
func strftimeConversion(c byte, t time.Time) string {
	switch c {
	case 'a':
		return t.Format("Mon")
	case 'A':
		return t.Weekday().String()
	case 'b', 'h':
		return t.Format("Jan")
	case 'B':
		return t.Month().String()
	case 'c':
		// The C locale's date and time: the form `date` writes.
		return t.Format("Mon Jan _2 15:04:05 2006")
	case 'C':
		return pad2(t.Year() / 100)
	case 'd':
		return pad2(t.Day())
	case 'D':
		return strftime("%m/%d/%y", t)
	case 'e':
		return space2(t.Day())
	case 'F':
		return strftime("%Y-%m-%d", t)
	case 'g':
		year, _ := t.ISOWeek()
		return pad2(year % 100)
	case 'G':
		year, _ := t.ISOWeek()
		return strconv.Itoa(year)
	case 'H':
		return pad2(t.Hour())
	case 'I':
		return pad2(clockHour(t))
	case 'j':
		return pad(t.YearDay(), 3)
	case 'k':
		return space2(t.Hour())
	case 'l':
		return space2(clockHour(t))
	case 'm':
		return pad2(int(t.Month()))
	case 'M':
		return pad2(t.Minute())
	case 'n':
		return "\n"
	case 'p':
		if t.Hour() < 12 {
			return "AM"
		}
		return "PM"
	case 'r':
		return strftime("%I:%M:%S %p", t)
	case 'R':
		return strftime("%H:%M", t)
	case 's':
		return strconv.FormatInt(t.Unix(), 10)
	case 'S':
		return pad2(t.Second())
	case 't':
		return "\t"
	case 'T', 'X':
		return strftime("%H:%M:%S", t)
	case 'u':
		// Monday is 1 and Sunday is 7, where %w counts Sunday as 0.
		if d := int(t.Weekday()); d != 0 {
			return strconv.Itoa(d)
		}
		return "7"
	case 'U':
		return pad2(weekOfYear(t, time.Sunday))
	case 'V':
		_, week := t.ISOWeek()
		return pad2(week)
	case 'w':
		return strconv.Itoa(int(t.Weekday()))
	case 'W':
		return pad2(weekOfYear(t, time.Monday))
	case 'x':
		return strftime("%m/%d/%y", t)
	case 'y':
		return pad2(t.Year() % 100)
	case 'Y':
		return strconv.Itoa(t.Year())
	case 'z':
		return t.Format("-0700")
	case 'Z':
		return t.Format("MST")
	case '%':
		return "%"
	}
	return string(c)
}

// clockHour is the hour on a twelve-hour clock, where midnight and noon are
// both 12 rather than 0.
func clockHour(t time.Time) int {
	h := t.Hour() % 12
	if h == 0 {
		return 12
	}
	return h
}

// weekOfYear counts weeks from the first `first` of the year, which is what
// %U and %W do: the days before it are week 0.
func weekOfYear(t time.Time, first time.Weekday) int {
	offset := (int(t.Weekday()) - int(first) + 7) % 7
	return (t.YearDay() + 6 - offset) / 7
}

func pad2(n int) string { return pad(n, 2) }

// pad writes n with leading zeros to the given width.
func pad(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// space2 writes n in two columns with a leading space rather than a zero,
// which is what %e, %k and %l do.
func space2(n int) string {
	s := strconv.Itoa(n)
	if len(s) < 2 {
		return " " + s
	}
	return s
}
