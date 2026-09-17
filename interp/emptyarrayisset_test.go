// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Whether an array that exists and holds no elements answers the `-`/`+`
// test as a set parameter — Semantics.EmptyArrayIsSet. Tests name the axis
// and never a shell; the measurements are on the field and in the three
// dialect suites that answer it.
//
// The question is **existence**, asked of a name that was declared. An array
// holding one empty string is set under both answers, and it joins to the
// same nothing an empty one does, so a test written against the value could
// not tell the two apart.

func emptyArrayRun(t *testing.T, src string, answer Answer) (string, string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.EmptyArrayIsSet = answer
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem,
		Dir: t.TempDir(), Name: "testsh",
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return strings.TrimSpace(out.String()), errs.String(), st
}

func TestAnArrayWithNoElementsUnderTheSetTest(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		yes, no   string
	}{
		{"the plus test", `e=(); echo "[${e[@]+S}]"`, "[S]", "[]"},
		{"the minus test", `e=(); echo "[${e[@]-D}]"`, "[]", "[D]"},
		// The star spelling answers with the at spelling: the join is what
		// differs between them and existence is not about the join.
		{"the star spelling, plus", `e=(); echo "[${e[*]+S}]"`, "[S]", "[]"},
		{"the star spelling, minus", `e=(); echo "[${e[*]-D}]"`, "[]", "[D]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, side := range []struct {
				answer Answer
				want   string
			}{{Yes, tc.yes}, {No, tc.no}} {
				out, errs, st := emptyArrayRun(t, tc.src, side.answer)
				if out != side.want || errs != "" || st != 0 {
					t.Errorf("%s under %v = %q (stderr %q, status %d), want %q",
						tc.src, side.answer, out, errs, st, side.want)
				}
			}
		})
	}
}

// The three guards, each a place the panel agrees, so the axis decides none
// of them and both answers give the same line.
func TestTheEmptyArrayAxisIsAskedOnlyWhereItDecides(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// An element holding the empty string is set, which is what makes
		// this a count rather than a reading of the joined value.
		{"one empty element", `e=(""); echo "[${e[@]+S}]"`, "[S]"},
		{"one empty element, minus", `e=(""); echo "[${e[@]-D}]"`, "[]"},
		// A single element is not the whole array.
		{"an element subscript", `e=(); echo "[${e[0]+S}]"`, "[]"},
		// A name nothing ever declared is unset under either answer.
		{"a name nothing declared", `echo "[${zqnosuch[@]+S}]"`, "[]"},
		// An array with elements is set under either answer.
		{"a name with elements", `e=(x); echo "[${e[@]+S}]"`, "[S]"},
		// And the colon form fires on the empty value whichever way the
		// set-ness goes, so it never reaches the axis.
		{"the colon form", `e=(); echo "[${e[@]:-D}]"`, "[D]"},
		{"the colon plus form", `e=(); echo "[${e[@]:+S}]"`, "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, answer := range []Answer{Yes, No, Unspecified} {
				out, errs, st := emptyArrayRun(t, tc.src, answer)
				if out != tc.want || errs != "" || st != 0 {
					t.Errorf("%s under %v = %q (stderr %q, status %d), want %q",
						tc.src, answer, out, errs, st, tc.want)
				}
			}
		})
	}
}

// Unanswered refuses rather than guessing, and only where the axis decides.
func TestTheEmptyArrayAxisRefusesWhenNoShellWasChosen(t *testing.T) {
	out, errs, st := emptyArrayRun(t, `e=(); echo "[${e[@]+S}]"`, Unspecified)
	if !strings.Contains(errs, "array with no elements is a set parameter") || st == 0 {
		t.Errorf("unanswered = %q (stderr %q, status %d), want a refusal naming the axis", out, errs, st)
	}
}
