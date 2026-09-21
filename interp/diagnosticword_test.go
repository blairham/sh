// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The word of `${x?word}` is read as **fields** of a command line in one
// family and as a **value** in the rest — see Semantics.DiagnosticWordIsFields
// and #3876. Tests name axes and wordings, never shells.
//
// Three things have to hold at once and only the first is the axis:
//
//   - under Yes a `$*` in the word comes out space-separated, because the
//     word was expanded into fields and the sentence made of them;
//   - under No it joins on the first character of IFS, which is what every
//     other place that wants a word as a value does;
//   - the *assigning* word is the value reading under **both**, because the
//     panel is unanimous there. That row is what fails if the question is
//     asked on the helper the two forms share rather than at the operator.
//
// And a fourth that is not an assertion about the axis at all: with the
// default IFS the two readings coincide, because its first character is
// already the space the fields reading supplies. It is kept under both
// answers to say that the rows above earn their `IFS=:` line.

// diagWordRun runs src with the diagnostic-word axis answered and nothing
// else moved. Standard error is in the same buffer, which is where the
// sentence this axis is about comes out.
func diagWordRun(t *testing.T, src string, fields Answer) string {
	t.Helper()
	out, _ := axisRun(t, src, func(s *Semantics) {
		// The fields reading is a word split, so the axis that decides
		// whether an unquoted expansion splits at all has to be answered or
		// nothing reaches the question. Answered the same way under both
		// legs, which is what keeps the rows comparable.
		s.SplitParamExpansion = Yes
		s.UnquotedListJoinsOnIFS = Yes
		// And what a refused expansion costs, which every row here reaches
		// and none of them is about.
		s.FatalErrorStatusIsOne = Yes
		s.DiagnosticWordIsFields = fields
	})
	return out
}

// The headline. One script, one non-default IFS, two readings.
func TestTheDiagnosticWordsFieldReadingIsAnAxis(t *testing.T) {
	const src = `set -- 'a:b' c
IFS=:
unset e1; echo ${e1?$*}`
	if out := diagWordRun(t, src, Yes); !strings.Contains(out, "a b c") {
		t.Errorf("yes: got %q, want the word expanded into fields and joined with a space", out)
	} else if strings.Contains(out, "a:b:c") {
		t.Errorf("yes: got %q, want no IFS join in the sentence", out)
	}
	if out := diagWordRun(t, src, No); !strings.Contains(out, "a:b:c") {
		t.Errorf("no: got %q, want the word taken as a value and joined on IFS", out)
	} else if strings.Contains(out, "a b c") {
		t.Errorf("no: got %q, want no space join in the sentence", out)
	}
}

// The control that keeps the axis off the shared helper: the word an
// *assignment* form stores is the value reading whichever way this axis is
// answered, because the panel does not split there. Asking the question one
// level up passes every row above and fails this one.
func TestTheAssigningWordIsAValueWhicheverWayTheDiagnosticWordIsRead(t *testing.T) {
	const src = `set -- 'a:b' c
IFS=:
unset u; : ${u:=$*}; printf '[%s]' "$u"`
	for _, a := range []Answer{Yes, No} {
		out := diagWordRun(t, src, a)
		if !strings.Contains(out, "[a:b:c]") {
			t.Errorf("%v: got %q, want the assigning word stored as a value", a, out)
		}
	}
}

// The control the measurement itself needs: under the default IFS the two
// readings produce the same string, so a probe that does not set IFS cannot
// tell them apart and proves nothing about either.
func TestTheDefaultSeparatorHidesTheDiagnosticWordsReading(t *testing.T) {
	const src = `set -- 'a:b' c
unset e1; echo ${e1?$*}`
	yes, no := diagWordRun(t, src, Yes), diagWordRun(t, src, No)
	if yes != no {
		t.Errorf("the readings part under the default IFS: %q against %q", yes, no)
	}
	if !strings.Contains(yes, "a:b c") {
		t.Errorf("got %q, want the colon kept inside the first parameter and a space between them", yes)
	}
}
