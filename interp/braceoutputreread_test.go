// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether what brace expansion produced re-enters the word as shell text or as
// the spans the parse cut is an axis, and the two readings are both reachable.
//
// The probe is a bare parameter whose name the produced text continues: under
// one reading the words that come out are the strings `$varx` and `$vary`, and
// under the other the word is still `[$var][x]` and `$var` is the name.
func TestBraceOutputRereadAsTextIsAnAxis(t *testing.T) {
	const src = `var=baz; varx=vx; vary=vy; echo $var{x,y}`
	if out, st := axisRun(t, src, func(s *Semantics) {
		s.BraceExpansion = Yes
		s.BraceOutputRereadAsText = Yes
	}); st != 0 || out != "vx vy\n" {
		t.Errorf("got %q status %d, want the produced text read as part of the name", out, st)
	}
	if out, st := axisRun(t, src, func(s *Semantics) {
		s.BraceExpansion = Yes
		s.BraceOutputRereadAsText = No
	}); st != 0 || out != "bazx bazy\n" {
		t.Errorf("got %q status %d, want the name ended where the span ended", out, st)
	}
	if _, st := axisRun(t, src, func(s *Semantics) { s.BraceExpansion = Yes }); st != 2 {
		t.Errorf("status %d, want the unanswered axis refused", st)
	}
}

// And it is asked **only** where the two readings part, which is nearly never:
// a word whose produced text is inert is the same word either way, so an
// unanswered dialect expands it rather than refusing it.
//
// This is the half that keeps the axis out of every script that writes a
// brace. Without it `echo a{b,c}d` — and every range anybody counts — would
// refuse in a vector that has not answered a question those words cannot tell
// the two answers of.
func TestTheBraceOutputAxisIsNotAskedWhereBothReadingsAgree(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"text on both sides of a list", `echo a{b,c}d`, "abd acd\n"},
		{"a name the braces already ended", `var=baz; echo ${var}{x,y}`, "bazx bazy\n"},
		{"a counted numeric range", `echo {1..4}`, "1 2 3 4\n"},
		{"a padded one", `echo {01..3}`, "01 02 03\n"},
		{"a range beside a list", `echo {1..2}{x,y}`, "1x 1y 2x 2y\n"},
		{"a quoted alternative", `echo x{'a b',c}`, "xa b xc\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := axisRun(t, tc.src, func(s *Semantics) {
				s.BraceExpansion = Yes
				s.BraceRangePadsToEndpointWidth = Yes
			})
			if st != 0 || out != tc.want {
				t.Errorf("%s: got %q status %d, want %q with no question asked", tc.src, out, st, tc.want)
			}
		})
	}
}

// A range whose elements are *not* inert is where the readings part from the
// other end: under the re-reading one a counted backslash is an unquoted
// backslash and quote removal takes it, and under the other it is data the
// word carries.
func TestACountedRangeOfActiveCharactersAsksTheAxis(t *testing.T) {
	const src = `printf '[%s]' {Z..a}`
	if out, st := axisRun(t, src, func(s *Semantics) {
		s.BraceExpansion = Yes
		s.BraceOutputRereadAsText = Yes
	}); st != 0 || out != "[Z][[][][]][^][_][`][a]" {
		t.Errorf("got %q status %d, want the backslash gone to quote removal", out, st)
	}
	if out, st := axisRun(t, src, func(s *Semantics) {
		s.BraceExpansion = Yes
		s.BraceOutputRereadAsText = No
	}); st != 0 || out != "[Z][[][\\][]][^][_][`][a]" {
		t.Errorf("got %q status %d, want the backslash kept as data", out, st)
	}
	if _, st := axisRun(t, src, func(s *Semantics) { s.BraceExpansion = Yes }); st != 2 {
		t.Errorf("status %d, want the unanswered axis refused", st)
	}
}

// Text the re-reading produced that will not read as a word is a **run-time**
// failure belonging to the word: the line is abandoned and the next one runs,
// which is FailedExpansionAbandonsTheLine rather than a parse failure. The
// other reading never meets it, because a word substituted back as spans has
// no text to fail on.
func TestProducedTextThatWillNotReadAbandonsTheLine(t *testing.T) {
	out, st := axisRun(t, "printf '[%s]' x{Z..a}y\necho \"after=$?\"\n", func(s *Semantics) {
		s.BraceExpansion = Yes
		s.BraceOutputRereadAsText = Yes
		s.FailedExpansionAbandonsTheLine = Yes
		s.FatalErrorStatusIsOne = Yes
	})
	if st != 0 || out == "" {
		t.Fatalf("got %q status %d, want a complaint and the next line", out, st)
	}
	if want := "after=1\n"; len(out) < len(want) || out[len(out)-len(want):] != want {
		t.Errorf("got %q, want the line abandoned at 1 with the next one running", out)
	}
}
