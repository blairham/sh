// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"
	"time"
)

// One shell's `%T` takes a **date string** where the other's takes a number
// of seconds, and it is the same letter (#602).
//
// `%(fmt)T` was built for bash in #481, where the operand is an epoch. ksh93
// spells the same conversion the same way and reads its operand through the
// date parser its `date` and `touch` use, so `printf '%(%Y)T' 2020-01-02`
// answers 2020 there and a *number* earns a warning and the current time.
// The plain `%T` with no parentheses is that conversion too, which bash does
// not have at all.
//
// Measured 2026-09-12, ksh93u+ 2012-08-01, TZ=UTC:
//
//	#1000000000            2001-09-09 01:46:40   an epoch, behind a `#`
//	now, exact             the current time
//	today                  midnight today
//	tomorrow, yesterday    midnight, a day either way
//	noon, midnight         today at that hour
//	2020-01-02             that date, at the current time of day
//	2020-01-02 03:04:05    that date and time
//	2020-01-02T03:04:05    the same, ISO-separated
//	03:04, 03:04:05        today at that time
//	abc, 1000000000        warning, and the current time, status 1
//
// **A subset, deliberately, and the boundary is written down rather than
// hidden.** That parser also takes `Jan 2 2020`, the `[[CC]YY]MMDDhhmm` form
// `touch` has, and a bare `-1`; those are its own legacy spellings rather
// than anything a portable script writes, and they meet the warning here.
// The warning is that shell's own answer for an operand it cannot read, so
// what a script sees for one of them is a sentence it would recognize and a
// status of 1 — not a wrong date presented as a right one.

// kshDate reads one of the date strings above, or reports that it could not.
//
// now is passed in rather than read here so that the whole conversion sees
// one instant: `printf '%(%S)T %(%S)T' now now` must not straddle a second.
func kshDate(s string, now time.Time) (time.Time, bool) {
	s = strings.TrimSpace(s)
	switch s {
	case "", "now", "exact":
		return now, true
	case "today":
		return midnightOf(now), true
	case "tomorrow":
		return midnightOf(now).AddDate(0, 0, 1), true
	case "yesterday":
		return midnightOf(now).AddDate(0, 0, -1), true
	case "noon":
		return midnightOf(now).Add(12 * time.Hour), true
	case "midnight":
		return midnightOf(now), true
	}
	if rest, ok := strings.CutPrefix(s, "#"); ok {
		// The one spelling that *is* an epoch, and the reason a script can
		// still hand this conversion a number: `#` in front says so.
		n, err := strconv.ParseInt(rest, 10, 64)
		if err != nil {
			return now, false
		}
		return time.Unix(n, 0).In(now.Location()), true
	}
	// A date, a time, or a date and a time separated by a space or a `T`.
	date, clock, split := strings.Cut(s, " ")
	if !split {
		date, clock, split = strings.Cut(s, "T")
	}
	if !split {
		if t, ok := clockOf(s, now); ok {
			return t, true
		}
		return dateOf(s, now)
	}
	day, ok := dateOf(date, now)
	if !ok {
		return now, false
	}
	at, ok := clockOf(clock, day)
	if !ok {
		return now, false
	}
	return at, true
}

// midnightOf is the start of the day a time falls in, in its own zone.
func midnightOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// dateOf reads `YYYY-MM-DD`, keeping the time of day it is handed — which is
// what makes a bare date answer with the current clock, measured.
func dateOf(s string, at time.Time) (time.Time, bool) {
	d, err := time.ParseInLocation("2006-01-02", s, at.Location())
	if err != nil {
		return at, false
	}
	h, min, sec := at.Clock()
	return time.Date(d.Year(), d.Month(), d.Day(), h, min, sec, 0, at.Location()), true
}

// clockOf reads `HH:MM` or `HH:MM:SS` on the day it is handed.
func clockOf(s string, on time.Time) (time.Time, bool) {
	layout := "15:04"
	if strings.Count(s, ":") == 2 {
		layout = "15:04:05"
	}
	t, err := time.ParseInLocation(layout, s, on.Location())
	if err != nil {
		return on, false
	}
	y, m, d := on.Date()
	return time.Date(y, m, d, t.Hour(), t.Minute(), t.Second(), 0, on.Location()), true
}
