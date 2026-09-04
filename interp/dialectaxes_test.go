// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Four single-dialect divergences, each an axis asked at the disagreement.

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
