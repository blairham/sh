// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Single-dialect divergences, each an axis asked at the disagreement.

func axisRun(t *testing.T, src string, set func(*Semantics)) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		sem := CoreSemantics()
		// The scalar reading an array's bare name yields is its own axis,
		// answered so these tests reach the question they are about.
		sem.ArrayScalarIsTheWholeArray = No
		set(&sem)
		r.Semantics = &sem
	})
}

func TestArrayLengthWithoutASubscriptIsAnAxis(t *testing.T) {
	out, _ := axisRun(t, `a=(hello by z); echo "${#a}"`, func(s *Semantics) {
		s.ArrayLengthWithoutSubscriptIsCount = Yes
	})
	if strings.TrimSpace(out) != "3" {
		t.Errorf("got %q, want the element count", out)
	}
	out, _ = axisRun(t, `a=(hello by z); echo "${#a}"`, func(s *Semantics) {
		s.ArrayLengthWithoutSubscriptIsCount = No
	})
	if strings.TrimSpace(out) != "5" {
		t.Errorf("got %q, want the scalar's length", out)
	}
}

func TestAnEmptyArrayQuotedAtIsAnAxis(t *testing.T) {
	out, _ := axisRun(t, `a=(); set -- "${a[@]}"; echo "n=$#"`, func(s *Semantics) {
		s.EmptyArrayAtIsOneEmptyField = Yes
	})
	if !strings.Contains(out, "n=1") {
		t.Errorf("got %q, want one empty field", out)
	}
	out, _ = axisRun(t, `a=(); set -- "${a[@]}"; echo "n=$#"`, func(s *Semantics) {
		s.EmptyArrayAtIsOneEmptyField = No
	})
	if !strings.Contains(out, "n=0") {
		t.Errorf("got %q, want none", out)
	}
	// A non-empty array asks nothing and keeps its fields either way.
	out, _ = axisRun(t, `a=(x y); set -- "${a[@]}"; echo "n=$#"`, func(s *Semantics) {})
	if !strings.Contains(out, "n=2") {
		t.Errorf("got %q, want the fields kept without a question", out)
	}
}

func TestANegativeSubstringLengthIsAnAxis(t *testing.T) {
	out, _ := axisRun(t, `x=abcdef; echo "[${x:1:-2}]"`, func(s *Semantics) {
		s.SubstringNegativeLengthIsEmpty = Yes
	})
	if !strings.Contains(out, "[]") {
		t.Errorf("got %q, want nothing at all", out)
	}
	out, _ = axisRun(t, `x=abcdef; echo "[${x:1:-2}]"`, func(s *Semantics) {
		s.SubstringNegativeLengthIsEmpty = No
	})
	if !strings.Contains(out, "[bcd]") {
		t.Errorf("got %q, want the count from the end", out)
	}
}

func TestLinenoInAFunctionIsAnAxis(t *testing.T) {
	src := "f(){\necho $LINENO\n}\nf"
	out, _ := axisRun(t, src, func(s *Semantics) { s.LinenoCountsFromTheFunction = Yes })
	if strings.TrimSpace(out) != "1" {
		t.Errorf("got %q, want the function-relative line", out)
	}
	out, _ = axisRun(t, src, func(s *Semantics) { s.LinenoCountsFromTheFunction = No })
	if strings.TrimSpace(out) != "2" {
		t.Errorf("got %q, want the file line", out)
	}
}

// The three brace-range axes, each asked only when a range reaches its
// disagreement: what an endpoint's leading zeros mean, and what a written
// step's sign means.

func TestBraceRangePaddingIsAnAxis(t *testing.T) {
	src := `echo {01..3}`
	out, _ := axisRun(t, src, func(s *Semantics) {
		s.BraceExpansion = Yes
		s.BraceRangePadsToEndpointWidth = Yes
	})
	if strings.TrimSpace(out) != "01 02 03" {
		t.Errorf("got %q, want the range padded to the widest endpoint", out)
	}
	out, _ = axisRun(t, src, func(s *Semantics) {
		s.BraceExpansion = Yes
		s.BraceRangePadsToEndpointWidth = No
	})
	if strings.TrimSpace(out) != "1 2 3" {
		t.Errorf("got %q, want the padding stripped", out)
	}
	// Unanswered, the padded endpoint is a refusal — but only in a dialect
	// whose braces expand at all: with them off the word is a literal and
	// the zeros are never a question.
	if _, st := axisRun(t, src, func(s *Semantics) { s.BraceExpansion = Yes }); st != 2 {
		t.Errorf("status %d, want the unanswered axis refused", st)
	}
	out, st := axisRun(t, src, func(s *Semantics) { s.BraceExpansion = No })
	if st != 0 || strings.TrimSpace(out) != "{01..3}" {
		t.Errorf("got %q status %d, want the literal word without a question", out, st)
	}
	// An unpadded range asks nothing.
	out, st = axisRun(t, `echo {1..3}`, func(s *Semantics) { s.BraceExpansion = Yes })
	if st != 0 || strings.TrimSpace(out) != "1 2 3" {
		t.Errorf("got %q status %d, want the range expanded without a question", out, st)
	}
}

func TestBraceRangeStepSignIsAnAxis(t *testing.T) {
	// A sign pointing away from the far endpoint: honored, the range holds
	// its first element alone; ignored, the endpoints set the direction and
	// the step contributes magnitude.
	for _, tc := range []struct{ src, honored, ignored string }{
		{`echo {10..1..3}`, "10", "10 7 4 1"},
		{`echo {a..e..-1}`, "a", "a b c d e"},
	} {
		out, _ := axisRun(t, tc.src, func(s *Semantics) {
			s.BraceExpansion = Yes
			s.BraceRangeStepSignHonored = Yes
		})
		if strings.TrimSpace(out) != tc.honored {
			t.Errorf("%s honored: got %q, want %q", tc.src, out, tc.honored)
		}
		out, _ = axisRun(t, tc.src, func(s *Semantics) {
			s.BraceExpansion = Yes
			s.BraceRangeStepSignHonored = No
			s.BraceRangeNegativeStepReverses = No
		})
		if strings.TrimSpace(out) != tc.ignored {
			t.Errorf("%s ignored: got %q, want %q", tc.src, out, tc.ignored)
		}
	}
	if _, st := axisRun(t, `echo {10..1..3}`, func(s *Semantics) { s.BraceExpansion = Yes }); st != 2 {
		t.Errorf("status %d, want the unanswered axis refused", st)
	}
	// A step whose sign agrees with the endpoints asks nothing of this axis.
	out, st := axisRun(t, `echo {1..10..3}`, func(s *Semantics) { s.BraceExpansion = Yes })
	if st != 0 || strings.TrimSpace(out) != "1 4 7 10" {
		t.Errorf("got %q status %d, want the stride without a question", out, st)
	}
}

func TestBraceRangeNegativeStepReversalIsAnAxis(t *testing.T) {
	// `{3..1..-1}` is the corner where the sign agrees with the endpoints,
	// so the sign axis is not asked and only the reversal question remains.
	out, _ := axisRun(t, `echo {3..1..-1}`, func(s *Semantics) {
		s.BraceExpansion = Yes
		s.BraceRangeNegativeStepReverses = Yes
	})
	if strings.TrimSpace(out) != "1 2 3" {
		t.Errorf("got %q, want the walk reversed", out)
	}
	out, _ = axisRun(t, `echo {3..1..-1}`, func(s *Semantics) {
		s.BraceExpansion = Yes
		s.BraceRangeNegativeStepReverses = No
	})
	if strings.TrimSpace(out) != "3 2 1" {
		t.Errorf("got %q, want the endpoints' order kept", out)
	}
	// The walk is reversed rather than the endpoints swapped: `{1..10..-4}`
	// is `1 5 9` backwards, not the `10 6 2` of `{10..1..4}`.
	out, _ = axisRun(t, `echo {1..10..-4}`, func(s *Semantics) {
		s.BraceExpansion = Yes
		s.BraceRangeStepSignHonored = No
		s.BraceRangeNegativeStepReverses = Yes
	})
	if strings.TrimSpace(out) != "9 5 1" {
		t.Errorf("got %q, want the endpoint walk reversed", out)
	}
	// Honored, the sign never reaches the reversal question.
	out, _ = axisRun(t, `echo {1..10..-4}`, func(s *Semantics) {
		s.BraceExpansion = Yes
		s.BraceRangeStepSignHonored = Yes
	})
	if strings.TrimSpace(out) != "1" {
		t.Errorf("got %q, want the honored sign to answer first", out)
	}
	if _, st := axisRun(t, `echo {3..1..-1}`, func(s *Semantics) { s.BraceExpansion = Yes }); st != 2 {
		t.Errorf("status %d, want the unanswered axis refused", st)
	}
}

func TestALocalInheritingTheExportAttributeIsAnAxis(t *testing.T) {
	// Read through a real child rather than through a listing: what the axis
	// decides is what a command is told, and a shell can hold the attribute
	// and still hand the entry over, or the other way about.
	const shadow = `export FOO=bar; f() { local FOO=baz; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`
	out, _ := axisRun(t, shadow, func(s *Semantics) { s.LocalInheritsTheExportAttribute = Yes })
	if !strings.Contains(out, "FOO=baz") {
		t.Errorf("got %q, want the child told the local's value", out)
	}
	out, _ = axisRun(t, shadow, func(s *Semantics) { s.LocalInheritsTheExportAttribute = No })
	if !strings.Contains(out, "(none)") {
		t.Errorf("got %q, want the child told nothing under the name", out)
	}
	// Taking the attribute off is the local's, and it goes back with the
	// value when the function returns.
	out, _ = axisRun(t, `export FOO=bar; f() { local FOO=baz; }; f; /usr/bin/env | grep '^FOO=' || echo "(none)"`,
		func(s *Semantics) { s.LocalInheritsTheExportAttribute = No })
	if !strings.Contains(out, "FOO=bar") {
		t.Errorf("got %q, want the outer name exported again", out)
	}
	// A local naming the attribute itself answers the question outright.
	out, _ = axisRun(t, `export FOO=bar; f() { local -x FOO=baz; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`,
		func(s *Semantics) {
			s.LocalInheritsTheExportAttribute = No
			s.LocalOptions = "x"
		})
	if !strings.Contains(out, "FOO=baz") {
		t.Errorf("got %q, want the letter to say so outright", out)
	}
	// A name nothing exported asks nothing, so an unanswered axis is not
	// reached and the child is told nothing either way.
	out, st := axisRun(t, `FOO=bar; f() { local FOO=baz; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f`, func(*Semantics) {})
	if st != 0 || !strings.Contains(out, "(none)") {
		t.Errorf("got %q status %d, want no question and no entry", out, st)
	}
	// And where it is exported, an unanswered axis is refused rather than
	// guessed at: the declaration is not made at all.
	if _, st := axisRun(t, `export FOO=bar; f() { local FOO=baz; }; f`, func(*Semantics) {}); st != 2 {
		t.Errorf("status %d, want the unanswered axis refused", st)
	}
}
