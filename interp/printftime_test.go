// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
)

// `printf '%(fmt)T'`: an epoch through a date format, with the format written
// inside the conversion.
//
// The corpus pins a fixed epoch, because the two operands that are not epochs
// — now, and when the shell started — cannot be recorded twice with the same
// answer. They are pinned here instead, through the Clock hook, which is what
// the hook is for.

// timeSemantics answers the one axis these tests are about.
func timeSemantics(has Answer) Semantics {
	sem := CoreSemantics()
	sem.PrintfTimeConversion = has
	// The epoch reading, which is what every case in this file is about.
	// printfdate_test.go asks the other one.
	sem.PrintfTimeOperandIsADateString = No
	return sem
}

func printfTime(t *testing.T, src string, has Answer, adjust func(*Runner)) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		sem := timeSemantics(has)
		r.Semantics = &sem
		r.Vars = map[string]string{"TZ": "UTC"}
		if adjust != nil {
			adjust(r)
		}
	})
}

func TestPrintfTimeConversion(t *testing.T) {
	t.Run("a fixed epoch through a format", func(t *testing.T) {
		out, st := printfTime(t, `printf '%(%Y-%m-%dT%H:%M:%S)T\n' 1000000000`, Yes, nil)
		if st != 0 || out != "2001-09-09T01:46:40\n" {
			t.Errorf("out = %q status %d, want the epoch in UTC", out, st)
		}
	})

	t.Run("an empty format is the time of day", func(t *testing.T) {
		out, _ := printfTime(t, `printf '%()T\n' 1000000000`, Yes, nil)
		if out != "01:46:40\n" {
			t.Errorf("out = %q, want the C locale's time of day", out)
		}
	})

	t.Run("the width belongs to the result", func(t *testing.T) {
		out, _ := printfTime(t, `printf '[%10(%Y)T][%-10(%Y)T]\n' 1000000000 1000000000`, Yes, nil)
		if out != "[      2001][2001      ]\n" {
			t.Errorf("out = %q, want the year padded both ways", out)
		}
	})

	t.Run("the format runs once per operand", func(t *testing.T) {
		out, _ := printfTime(t, `printf '%(%Y)T ' 1000000000 1100000000; echo`, Yes, nil)
		if out != "2001 2004 \n" {
			t.Errorf("out = %q, want a year per operand", out)
		}
	})

	t.Run("a dialect without it calls the conversion unknown", func(t *testing.T) {
		out, st := printfTime(t, `printf '%(%Y)T\n' 1000000000`, No, nil)
		if !strings.Contains(out, "invalid directive") || st == 0 {
			t.Errorf("out = %q status %d, want the conversion refused", out, st)
		}
	})

	t.Run("unanswered, the core refuses rather than choosing", func(t *testing.T) {
		out, _ := printfTime(t, `printf '%(%Y)T\n' 1000000000`, Unspecified, nil)
		if !strings.Contains(out, "writing a date") {
			t.Errorf("out = %q, want a refusal naming the axis", out)
		}
	})

	t.Run("a format with no %( asks nothing", func(t *testing.T) {
		out, st := printfTime(t, `printf '%s\n' hi`, Unspecified, nil)
		if st != 0 || out != "hi\n" {
			t.Errorf("out = %q status %d, want no question where no date was asked for", out, st)
		}
	})

	t.Run("an operand that is not an epoch", func(t *testing.T) {
		out, st := printfTime(t, `printf '%(%Y)T\n' abc`, Yes, func(r *Runner) {
			sem := timeSemantics(Yes)
			sem.PrintfReportsBadNumber = Yes
			r.Semantics = &sem
			r.Vars = map[string]string{"TZ": "UTC"}
		})
		if !strings.Contains(out, "invalid number") || !strings.Contains(out, "1970") || st == 0 {
			t.Errorf("out = %q status %d, want the complaint and the epoch zero", out, st)
		}
	})
}

// -1 is now and -2 is when the shell started, and a missing operand is now as
// well. The Clock hook is what makes that sayable twice.
func TestPrintfTimeNowAndShellStart(t *testing.T) {
	fixed := time.Date(2019, 3, 4, 5, 6, 7, 0, time.UTC)
	for _, tc := range []struct {
		name, src string
	}{
		{"minus one is now", `printf '%(%Y-%m-%d)T\n' -1`},
		{"no operand at all is now too", `printf '%(%Y-%m-%d)T\n'`},
		{"minus two is when the shell started", `printf '%(%Y-%m-%d)T\n' -2`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := printfTime(t, tc.src, Yes, func(r *Runner) {
				sem := timeSemantics(Yes)
				r.Semantics = &sem
				r.Vars = map[string]string{"TZ": "UTC"}
				r.Clock = func() time.Time { return fixed }
			})
			if st != 0 || out != "2019-03-04\n" {
				t.Errorf("out = %q status %d, want the clock's answer", out, st)
			}
		})
	}

	// And an epoch that really is -1 is a second before the epoch, not now:
	// the two spellings are told apart by the operand rather than by sign.
	t.Run("a genuine epoch is still an epoch", func(t *testing.T) {
		out, _ := printfTime(t, `printf '%(%Y)T\n' 100000`, Yes, nil)
		if out != "1970\n" {
			t.Errorf("out = %q, want the epoch's own year", out)
		}
	})
}

// The zone is the Runner's `$TZ`, not the process's — the same rule that
// keeps PATH out of os/exec.
func TestPrintfTimeReadsTheRunnersZone(t *testing.T) {
	for _, tc := range []struct{ tz, want string }{
		{"UTC", "01:46 UTC\n"},
		{"America/New_York", "21:46 EDT\n"},
		// An empty zone, and a name no database has, are both UTC.
		{"", "01:46 UTC\n"},
		{"nonsense", "01:46 UTC\n"},
	} {
		t.Run(tc.tz, func(t *testing.T) {
			out, _ := printfTime(t, `printf '%(%H:%M %Z)T\n' 1000000000`, Yes, func(r *Runner) {
				sem := timeSemantics(Yes)
				r.Semantics = &sem
				r.Vars = map[string]string{"TZ": tc.tz}
			})
			if out != tc.want {
				t.Errorf("TZ=%q gave %q, want %q", tc.tz, out, tc.want)
			}
		})
	}

	// Unexported is enough: a shell variable decides, not the environment.
	t.Run("an unexported assignment decides", func(t *testing.T) {
		out, _ := printfTime(t, `TZ=UTC; printf '%(%H:%M %Z)T\n' 1000000000`, Yes, func(r *Runner) {
			sem := timeSemantics(Yes)
			r.Semantics = &sem
			r.Vars = map[string]string{"TZ": "America/New_York"}
		})
		if out != "01:46 UTC\n" {
			t.Errorf("out = %q, want the assignment to have decided", out)
		}
	})
}

// The conversion specifications themselves, against a Sunday in September
// 2001 — every one measured from a live shell in the C locale.
func TestPrintfTimeConversionSpecifications(t *testing.T) {
	for _, tc := range []struct{ spec, want string }{
		{"%a", "Sun"},
		{"%A", "Sunday"},
		{"%b", "Sep"},
		{"%B", "September"},
		{"%h", "Sep"},
		{"%c", "Sun Sep  9 01:46:40 2001"},
		{"%C", "20"},
		{"%d", "09"},
		{"%D", "09/09/01"},
		{"%e", " 9"},
		{"%F", "2001-09-09"},
		{"%g", "01"},
		{"%G", "2001"},
		{"%H", "01"},
		{"%I", "01"},
		{"%j", "252"},
		{"%k", " 1"},
		{"%l", " 1"},
		{"%m", "09"},
		{"%M", "46"},
		{"%p", "AM"},
		{"%r", "01:46:40 AM"},
		{"%R", "01:46"},
		{"%s", "1000000000"},
		{"%S", "40"},
		{"%T", "01:46:40"},
		{"%u", "7"},
		{"%U", "36"},
		{"%V", "36"},
		{"%w", "0"},
		{"%W", "36"},
		{"%x", "09/09/01"},
		{"%X", "01:46:40"},
		{"%y", "01"},
		{"%Y", "2001"},
		{"%z", "+0000"},
		{"%Z", "UTC"},
		{"%%", "%"},
		// A letter no conversion has keeps itself and loses the `%`.
		{"%i", "i"},
		{"%n", "\n"},
		{"%t", "\t"},
	} {
		t.Run(tc.spec, func(t *testing.T) {
			out, _ := printfTime(t, `printf '[%(`+tc.spec+`)T]' 1000000000`, Yes, nil)
			if out != "["+tc.want+"]" {
				t.Errorf("%s gave %q, want %q", tc.spec, out, "["+tc.want+"]")
			}
		})
	}

	// A Tuesday in November, so the weekday conversions are pinned somewhere
	// other than a Sunday — where %u and %w disagree the most.
	t.Run("a midweek date", func(t *testing.T) {
		out, _ := printfTime(t, `printf '%(%a %u %w %U %V %W %j)T\n' 1700000000`, Yes, nil)
		if out != "Tue 2 2 46 46 46 318\n" {
			t.Errorf("out = %q, want the midweek answers", out)
		}
	})
}
