// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// ageEpoch is what the clock says for every run below, so the fixture's ages
// are exact rather than approximately whatever the suite took to get here.
// Runner.Clock exists for this — see its comment: a value the shell produces
// has to be sayable from outside or nothing depending on it tests twice the
// same.
var ageEpoch = time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

// ageDir holds three files at known ages: half a day, a day and a half, and
// two and a half. Those three are chosen to separate *truncation* from
// comparison — see fileAge — because 1.5 days is more than one day and is
// still not `m+1`.
func ageDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, age := range map[string]time.Duration{
		"h12": 12 * time.Hour,
		"h36": 36 * time.Hour,
		"h60": 60 * time.Hour,
	} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		at := ageEpoch.Add(-age)
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// runAged is runQualified with the clock pinned to ageEpoch.
func runAged(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	d := syntax.Core()
	qualifying(&d)
	return runGrammar(t, src, qualifying, func(r *Runner) {
		r.Dialect = &d
		r.Dir = dir
		r.Clock = func() time.Time { return ageEpoch }
		sem := *r.Semantics
		sem.GlobNoMatchIsError = Yes
		r.Semantics = &sem
	})
}

// The `m` qualifier: select a file by how old its contents are.
//
// Every row is a measurement on zsh 5.9.2, 2026-09-11, against a directory
// holding exactly these three ages.
//
// **The comparison is over a truncated number of units, and that is the whole
// of the rule.** It is not what the sign suggests: `m+1` reports only the
// 2.5-day file, because 1.5 days is *more than* a day and the number it is
// compared against is `1`. All three operators read the same truncated age
// and differ only in the comparison, which is why globCount.holds is reused
// whole (#2094).
func TestTheModificationAgeQualifier(t *testing.T) {
	dir := ageDir(t)
	all := "[h12][h36][h60]"
	for _, tc := range []struct{ name, list, want string }{
		// A plain number is the exact age in whole days.
		{"a number is the whole days", "m0", "[h12]"},
		{"and the next day", "m1", "[h36]"},
		{"and the one after", "m2", "[h60]"},
		// `-` is younger and `+` is older, both over the truncated age.
		{"a minus is younger", "m-1", "[h12]"},
		{"a plus is older", "m+1", "[h60]"},
		{"older than nothing is all but today", "m+0", "[h36][h60]"},
		{"younger than three days is everything", "m-3", all},
		// The unit letter comes between the qualifier and the number.
		{"hours", "mh-24", "[h12]"},
		{"more hours", "mh+24", "[h36][h60]"},
		{"an exact hour count nothing holds", "mh24", ""},
		{"days written out", "md+1", "[h60]"},
		{"seconds", "ms+100", all},
		{"weeks, all within the first", "mw0", all},
		{"months, all within the first", "mM0", all},
		// It turns and unions like every other test.
		{"a caret turns it", "^m0", "[h36][h60]"},
		{"a comma unions two ages", "m0,m2", "[h12][h60]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runAged(t, dir, `printf "[%s]" *(`+tc.list+`)`)
			if tc.want == "" {
				if !containsSub(out, "no matches found: *("+tc.list+")") || st == 0 {
					t.Errorf("*(%s) = %q (status %d), want a fatal miss", tc.list, out, st)
				}
				return
			}
			if out != tc.want || st != 0 {
				t.Errorf("*(%s) = %q (status %d), want %q at 0", tc.list, out, st, tc.want)
			}
		})
	}
}

// An argument with no digits is `number expected`, the same sentence the link
// count gives and for the same reason: the letter was recognized and its
// argument was not. `mx` is the row that matters — `x` is not a unit, so the
// argument begins at a character that is not a digit.
func TestAModificationAgeWithNoNumberIsRefused(t *testing.T) {
	dir := ageDir(t)
	for _, list := range []string{"m", "m+", "m-", "mx-1", "mh", "mh+"} {
		t.Run(list, func(t *testing.T) {
			out, st := runAged(t, dir, `printf "[%s]" *(`+list+`)`)
			if !containsSub(out, "number expected") || st == 0 {
				t.Errorf("*(%s) = %q (status %d), want `number expected` and a failure", list, out, st)
			}
		})
	}
}

// A file written after the clock says now is zero units old rather than a
// negative number — which is the answer `m0` and `m-1` already give for
// anything younger than the unit, and the one that cannot underflow.
func TestAFileFromTheFutureIsNoAgeAtAll(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ahead")
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	at := ageEpoch.Add(48 * time.Hour)
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
	if out, st := runAged(t, dir, `printf "[%s]" *(m0)`); out != "[ahead]" || st != 0 {
		t.Errorf("*(m0) = %q (status %d), want %q at 0", out, st, "[ahead]")
	}
}
