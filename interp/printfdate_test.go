// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
)

// The other reading of `%T`, where the operand is a date *string* and not a
// number of seconds (#602).
//
// The clock is pinned through the Clock hook, which is what makes `now`,
// `today` and their neighbors assertable at all: a corpus row cannot record
// them twice with the same answer, and a test that read the wall clock would
// be asserting the machine.

var pinned = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

func dateRun(t *testing.T, src string) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		sem := CoreSemantics()
		sem.PrintfTimeConversion = Yes
		sem.PrintfTimeOperandIsADateString = Yes
		r.Semantics = &sem
		r.Vars = map[string]string{"TZ": "UTC"}
		r.Clock = func() time.Time { return pinned }
	})
}

// **The strings this conversion takes, and what each one means.** Every row
// was measured against the shell that has the form; the subset is written
// down in interp/printfdate.go and the rest meet that shell's own warning.
func TestADateStringOperand(t *testing.T) {
	for _, c := range []struct{ operand, want string }{
		// The one spelling that is still an epoch, and the reason a script
		// can hand this conversion a number at all.
		{"#1000000000", "2001-09-09 01:46:40"},
		{"#0", "1970-01-01 00:00:00"},

		{"now", "2026-03-04 05:06:07"},
		{"exact", "2026-03-04 05:06:07"},
		{"", "2026-03-04 05:06:07"},
		{"today", "2026-03-04 00:00:00"},
		{"tomorrow", "2026-03-05 00:00:00"},
		{"yesterday", "2026-03-03 00:00:00"},
		{"noon", "2026-03-04 12:00:00"},
		{"midnight", "2026-03-04 00:00:00"},

		// A bare date keeps the current time of day, and a bare time keeps
		// today — which is the pair that says the two halves are read
		// separately rather than as one timestamp with defaults of zero.
		{"2020-01-02", "2020-01-02 05:06:07"},
		{"05:04", "2026-03-04 05:04:00"},
		{"05:04:03", "2026-03-04 05:04:03"},

		// Both separators between them.
		{"2020-01-02 03:04:05", "2020-01-02 03:04:05"},
		{"2020-01-02T03:04:05", "2020-01-02 03:04:05"},
	} {
		out, st := dateRun(t, `printf '%(%Y-%m-%d %H:%M:%S)T\n' "`+c.operand+`"`)
		if out != c.want+"\n" || st != 0 {
			t.Errorf("%q gave %q status %d, want %q at 0", c.operand, out, st, c.want)
		}
	}
}

// **An operand it cannot read is a warning and the current time, not a
// refusal** — and the status is 1, which is the only part a script can act
// on. A plain number is one of those operands, which is the whole difference
// from the epoch reading.
func TestADateStringItCannotRead(t *testing.T) {
	for _, operand := range []string{"abc", "1000000000", "2020-13-45", "99:99"} {
		out, st := dateRun(t, `printf '%(%Y-%m-%d %H:%M:%S)T\n' "`+operand+`"; echo "st=$?"`)
		const want = "sh: printf: warning: invalid argument of type T\n" +
			"2026-03-04 05:06:07\nst=1\n"
		if out != want || st != 0 {
			t.Errorf("%q gave %q status %d, want %q", operand, out, st, want)
		}
	}
}

// **The bare `%T` is this reading's own spelling**, and it and `%()T` share a
// default format that is the full date line rather than the time of day.
func TestTheBareTimeConversion(t *testing.T) {
	out, st := dateRun(t, `printf '[%T][%()T][%12(%Y)T]\n' "#1000000000" "#1000000000" "#1000000000"`)
	const want = "[Sun Sep  9 01:46:40 UTC 2001][Sun Sep  9 01:46:40 UTC 2001][        2001]\n"
	if out != want || st != 0 {
		t.Errorf("gave %q status %d, want %q", out, st, want)
	}
}

// The whole conversion sees one instant, so a format written twice cannot
// straddle a second. Asserted rather than assumed because the clock is a hook
// and a second read of it would be invisible in a fast test.
func TestOneInstantForTheWholeFormat(t *testing.T) {
	out, _ := dateRun(t, `printf '%(%S)T %(%S)T\n' now now`)
	if fields := strings.Fields(out); len(fields) != 2 || fields[0] != fields[1] {
		t.Errorf("gave %q, want the same second twice", out)
	}
}

// A dialect without this reading is not asked about it, and a dialect with
// neither reading meets the conversion as one it does not have.
func TestTheBareLetterNeedsTheDateReading(t *testing.T) {
	out, st := run(t, `printf '[%T]' 1; echo " st=$?"`, func(r *Runner) {
		sem := CoreSemantics()
		sem.PrintfTimeConversion = Yes
		sem.PrintfTimeOperandIsADateString = No
		r.Semantics = &sem
	})
	if !strings.Contains(out, "T") || st != 0 {
		t.Errorf("gave %q status %d, want the letter refused", out, st)
	}
	if strings.Contains(out, "[19") || strings.Contains(out, "[20") {
		t.Errorf("gave %q, want no date at all from the epoch reading", out)
	}
}
