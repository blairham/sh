// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// How much of a leading dash-word `eval` reads as options — #3216.
//
// Three answers over seven columns, measured 2026-09-16 from a script file
// under `env -i`, both streams separately, the status after each line and a
// marker after that:
//
//	column               eval -q echo hi              eval -- echo hi       continued
//	bash 5.3.20          invalid option + usage, 2    `hi`, 0               yes
//	bash as `sh`         the same, 2                  `hi`, 0               no
//	bash 3.2.57          the same, 2                  `hi`, 0               yes
//	ksh93u+ 2012-08-01   unknown option + usage, 2    `hi`, 0               no
//	zsh 5.9.2            `command not found: -q`, 127 `hi`, 0               yes
//	dash 0.5.12          `-q: not found`, 127         `--: not found`, 127  yes
//	BusyBox ash 1.37.0   the same, 127                the same, 127         yes
//
// The two columns that end the script do so because a usage error in a POSIX
// special builtin is fatal where the shell is in that mode — the fatality the
// refusal machinery already carries, not a second question here.
func TestHowMuchOfADashWordEvalReads(t *testing.T) {
	for _, c := range []struct {
		name    string
		reading EvalOptionReading
		src     string
		want    string
	}{
		// The marker. Read by two of the three answers, and the one that
		// does not read it runs it.
		{"the marker ends the options", EvalReadsOptions, "eval -- echo hi\n", "hi\n"},
		{"and under the marker-only reading too", EvalTakesTheEndMarkerOnly, "eval -- echo hi\n", "hi\n"},
		{"where nothing is read it is a command", EvalReadsNoOptions, "eval -- echo hi\n", "not found"},

		// A letter this builtin has not got. Refused by one answer and text
		// under the other two — which is what makes them three answers and
		// not a boolean.
		{"an unknown letter is refused", EvalReadsOptions, "eval -q echo hi\n", "invalid option"},
		{"the marker-only reading runs it", EvalTakesTheEndMarkerOnly, "eval -q echo hi\n", "not found"},
		{"and so does reading nothing", EvalReadsNoOptions, "eval -q echo hi\n", "not found"},

		// The text behind a refused letter does not run, in the answer that
		// refuses. That is the half that makes this a wrong action rather
		// than a wrong message.
		{"and the text behind it does not run", EvalReadsOptions, "eval -q echo hi\n", ""},

		// Behind the marker a dash-word is a command again, which is what
		// separates "the marker ended the options" from "the marker was
		// dropped".
		{"behind the marker a dash-word is a command", EvalReadsOptions, "eval -- -q\n", "not found"},
		{"under the marker-only reading as well", EvalTakesTheEndMarkerOnly, "eval -- -q\n", "not found"},

		// A word with no dash reaches none of it.
		{"a plain word is the text", EvalReadsOptions, "eval echo plain\n", "plain\n"},
		{"in every reading", EvalReadsNoOptions, "eval echo plain\n", "plain\n"},
	} {
		t.Run(c.name+" ("+c.reading.String()+")", func(t *testing.T) {
			out, _ := evalOptRun(t, c.src, c.reading)
			if c.want == "" {
				if strings.Contains(out, "hi") {
					t.Errorf("said %q, want the text never run", out)
				}
				return
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("said %q, want %q in it", out, c.want)
			}
		})
	}
}

// A lone `-` is outside the reading in every answer: it is the first word of
// the text, and the text runs it as a command. Measured — a reading that ate
// it would have made `eval - -- echo hi` run `echo hi`, and no column does.
func TestALoneDashIsNotEvalsMarker(t *testing.T) {
	for _, reading := range []EvalOptionReading{
		EvalReadsOptions, EvalTakesTheEndMarkerOnly, EvalReadsNoOptions,
	} {
		t.Run(reading.String(), func(t *testing.T) {
			out, st := evalOptRun(t, "eval - echo hi\n", reading)
			if strings.Contains(out, "hi") || st == 0 {
				t.Errorf("said %q at status %d, want the dash run as a command", out, st)
			}
		})
	}
}

// Every word was a marker and nothing is left: the same silent success a bare
// `eval` reports. Measured on bash 5.3.20, bash 3.2.57, zsh 5.9.2 and ksh93u+
// — every column that reads the marker at all.
func TestEvalWithNothingLeftAfterTheMarker(t *testing.T) {
	for _, reading := range []EvalOptionReading{EvalReadsOptions, EvalTakesTheEndMarkerOnly} {
		t.Run(reading.String(), func(t *testing.T) {
			out, st := evalOptRun(t, "false\neval --\n", reading)
			if out != "" || st != 0 {
				t.Errorf("said %q at status %d, want nothing at 0", out, st)
			}
		})
	}
}

// The ask is narrow, and this is the half that says so. A preset with no
// answer runs every `eval` that is not the disagreement, and refuses only the
// shape the panel splits over.
func TestAnUnansweredPresetOnlyRefusesADashWord(t *testing.T) {
	for _, c := range []struct {
		name    string
		src     string
		refuses bool
	}{
		{"a plain word", "eval echo hi\n", false},
		{"a quoted command", "eval 'echo hi'\n", false},
		{"nothing at all", "eval\n", false},
		// A lone `-` is a command word in every column, so it is not the
		// disagreement either.
		{"a lone dash", "eval - echo hi\n", false},
		{"the marker", "eval -- echo hi\n", true},
		{"a letter", "eval -q echo hi\n", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := evalOptRun(t, c.src, EvalOptionReadingUnspecified)
			if refused := strings.Contains(out, "no dialect was chosen"); refused != c.refuses {
				t.Errorf("said %q, want refused=%v", out, c.refuses)
			}
		})
	}
}

func evalOptRun(t *testing.T, src string, reading EvalOptionReading) (string, int) {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.EvalOptions = reading
	sem.LoneDashIsAnOption = No
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
	})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return buf.String(), st
}
