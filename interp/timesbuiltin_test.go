// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// The figures `times` prints are real timings and cannot be asserted on. The
// shape can, and the shape is what diverges — so every test here masks the
// digits, exactly as the corpus cases do.

var digits = regexp.MustCompile(`[0-9]+`)

func shape(s string) string { return digits.ReplaceAllString(s, "N") }

// timesRun runs src with the given vectors and returns the two streams apart,
// because which one `times` writes to is itself a measured fact.
func timesRun(t *testing.T, src string, sem Semantics, dg Diagnostics) (out, errs string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return o.String(), e.String(), st
}

func TestTimesIsASpecialBuiltinThatExists(t *testing.T) {
	// The gap this closes: the table called it special before it existed.
	if !IsSpecialBuiltin("times") {
		t.Error("times should be a special builtin")
	}
	if _, ok := newTestRunner(t, &Runner{}).Builtin("times"); !ok {
		t.Error("times is called special and does not exist")
	}
}

// TestTimesPrintsTwoLinesToStdout pins the parts that are unanimous: two
// lines, on standard output, status 0.
//
// The stream is worth its own assertion because a pipeline makes it look
// otherwise — `times 2>&1 >/dev/null | wc -l` suggests one shell uses stderr,
// and writing each stream to its own file shows all four use stdout. Keeping
// the streams apart here is the same discipline.
func TestTimesPrintsTwoLinesToStdout(t *testing.T) {
	out, errs, st := timesRun(t, `times`, permissive(), Diagnostics{})
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	if errs != "" {
		t.Errorf("stderr = %q, want nothing there", errs)
	}
	if got := strings.Count(out, "\n"); got != 2 {
		t.Errorf("lines = %d, want 2 (output %q)", got, out)
	}
}

func TestTimesDefaultLayoutIsSelfThenChildren(t *testing.T) {
	out, _, _ := timesRun(t, `times`, permissive(), Diagnostics{})
	if got := shape(out); got != "NmN.Ns NmN.Ns\nNmN.Ns NmN.Ns\n" {
		t.Errorf("shape = %q, want two lines of two figures", got)
	}
}

// TestTimesLayoutIsADialectsChoice covers the shape one dialect uses instead,
// which carries *less* information rather than the same information
// rearranged: labeled lines, one figure each, and no children's times at all.
func TestTimesLayoutIsADialectsChoice(t *testing.T) {
	out, _, _ := timesRun(t, `times`, permissive(),
		Diagnostics{TimesLayout: TimesUserAndSystem, TimesDecimals: 2})
	if got := shape(out); got != "user\tNmN.Ns\nsys\tNmN.Ns\n" {
		t.Errorf("shape = %q, want labeled, tab-separated lines", got)
	}
}

// TestTimesDecimalsAreADialectsChoice — a four-way split nobody would think to
// ask about, so the count is asserted exactly rather than by shape.
func TestTimesDecimalsAreADialectsChoice(t *testing.T) {
	for _, tc := range []struct {
		decimals int
		want     int
	}{
		{0, 3}, // zero means the substrate's own
		{2, 2},
		{3, 3},
		{6, 6},
	} {
		out, _, _ := timesRun(t, `times`, permissive(), Diagnostics{TimesDecimals: tc.decimals})
		first, _, _ := strings.Cut(out, "\n")
		field, _, _ := strings.Cut(first, " ")
		_, frac, ok := strings.Cut(field, ".")
		if !ok {
			t.Fatalf("no decimal point in %q", field)
		}
		frac = strings.TrimSuffix(frac, "s")
		if len(frac) != tc.want {
			t.Errorf("TimesDecimals %d gave %q (%d places), want %d",
				tc.decimals, field, len(frac), tc.want)
		}
	}
}

// TestTimesArgumentsAreADialectsChoice covers the axis in both directions.
// Two shells ignore what they are given and one refuses it; a third makes it a
// syntax error, which never reaches a builtin and is not this question.
func TestTimesArgumentsAreADialectsChoice(t *testing.T) {
	for _, tc := range []struct {
		name    string
		axis    Answer
		status  int
		printed bool
	}{
		{"ignored", No, 0, true},
		{"refused", Yes, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := permissive()
			sem.TimesRejectsArguments = tc.axis
			out, errs, st := timesRun(t, `times foo`, sem,
				Diagnostics{TimesArguments: "times: too many arguments"})
			if st != tc.status {
				t.Errorf("status = %d, want %d", st, tc.status)
			}
			if got := out != ""; got != tc.printed {
				t.Errorf("printed = %v, want %v (out %q, err %q)", got, tc.printed, out, errs)
			}
			if !tc.printed && !strings.Contains(errs, "too many arguments") {
				t.Errorf("stderr = %q, want the dialect's refusal", errs)
			}
		})
	}
}

// TestTimesRefusesWhenNoDialectAnswered — an unanswered axis is refused rather
// than guessed, the same as everywhere else in the vector.
func TestTimesRefusesWhenNoDialectAnswered(t *testing.T) {
	sem := permissive()
	sem.TimesRejectsArguments = Unspecified
	_, errs, st := timesRun(t, `times foo`, sem, Diagnostics{})
	if !strings.Contains(errs, "no dialect was chosen") {
		t.Errorf("stderr = %q, want the axis named", errs)
	}
	if st == 0 {
		t.Error("an unanswered axis must not report success")
	}
}

// TestTimesWithNoArgumentsNeverConsultsTheAxis is the other half: `times` on
// its own is unanimous, so a dialect that has answered nothing still works.
func TestTimesWithNoArgumentsNeverConsultsTheAxis(t *testing.T) {
	sem := permissive()
	sem.TimesRejectsArguments = Unspecified
	out, errs, st := timesRun(t, `times`, sem, Diagnostics{})
	if st != 0 {
		t.Errorf("status = %d, want 0 (stderr %q)", st, errs)
	}
	if strings.Contains(errs, "no dialect was chosen") {
		t.Errorf("plain `times` must not reach the argument axis: %q", errs)
	}
	if shape(out) != "NmN.Ns NmN.Ns\nNmN.Ns NmN.Ns\n" {
		t.Errorf("output = %q", out)
	}
}

// TestMinutesAreNotWrapped — the format is minutes and seconds, and a shell
// that has burned two hours prints 120m rather than starting an hours column
// the format has no room for.
func TestMinutesAreNotWrapped(t *testing.T) {
	// Reached through the exported surface: a runner cannot be made to have
	// burned an hour, so this checks the rendering directly through the one
	// path that formats a duration — a large TimesDecimals keeps it readable.
	out, _, _ := timesRun(t, `times`, permissive(), Diagnostics{})
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		for _, field := range strings.Fields(line) {
			if !strings.HasSuffix(field, "s") || !strings.Contains(field, "m") {
				t.Errorf("field %q is not the NmN.Ns shape", field)
			}
		}
	}
}
