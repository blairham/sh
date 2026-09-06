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

// The fourth produced-parameter seam, and the only one that produces nothing.
//
// A dialect with modules has to be able to say "this shell has not got that
// parameter" in a way a *script* can hear. Without it the only available
// answer was an empty value at status 0, which is indistinguishable from a
// real answer — and the whole reason a module could not load over a parameter
// nobody in the file touched (#1058, #1146).

// runAbsent runs one line and gives back what went to each stream.
func runAbsent(t *testing.T, src string) (out, errs string, status int) {
	t.Helper()
	var o, e strings.Builder
	r := seamRunner(t, &o, &e)
	r.SetAbsentParameter("nothere", "parameter not implemented yet")
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return o.String(), e.String(), r.ExitStatus()
}

// **Every route that reads a parameter refuses, and each one names it.**
//
// Written route by route because the routes do not share a bottom. The
// subscript one is the reason this is a table and not a single assertion:
// `${m[k]}` on a name nothing holds is answered by the array path with no
// fields at all, so a test placed where a value is fetched never sees it —
// and `${m[k]}` is the spelling a caller of a parameter module writes most.
//
// Each row asserts the *name* is in the diagnostic and that nothing reached
// standard output. A row that checked only the status would pass against a
// shell that expanded to nothing quietly, which is the exact bug this seam
// exists to prevent.
func TestAnAbsentParameterRefusesByNameOnEveryRoute(t *testing.T) {
	for _, src := range []string{
		`printf '[%s]' "$nothere"`,
		`printf '[%s]' "${nothere}"`,
		`printf '[%s]' "${nothere[k]}"`,
		`printf '[%s]' "${nothere[@]}"`,
		`printf '[%s]' "${#nothere}"`,
		`printf '[%s]' ${nothere[k]}`,
		`x="${nothere[k]}"; printf '[%s]' "$x"`,
	} {
		t.Run(src, func(t *testing.T) {
			out, errs, status := runAbsent(t, src)
			if !strings.Contains(errs, "nothere: parameter not implemented yet") {
				t.Errorf("%s: stderr = %q, want the parameter named", src, errs)
			}
			if out != "" {
				t.Errorf("%s: stdout = %q, want nothing handed to the caller", src, out)
			}
			if status == 0 {
				t.Errorf("%s: status 0, want a failure", src)
			}
		})
	}
}

// It is a refusal to read something absent and not a claim on the spelling: a
// script that gave the name a value of its own is answered with it, and the
// conditional operators — which are a script saying what to do when there is
// no value — are left alone, exactly as `set -u` leaves them.
func TestAnAbsentParameterYieldsToTheScriptsOwnAnswer(t *testing.T) {
	out, errs, status := runAbsent(t, `printf '[%s]' "${nothere-d}" "${nothere+set}"
nothere=mine
printf '[%s]' "$nothere" "${#nothere}"`)
	if want := "[d][][mine][4]"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if errs != "" || status != 0 {
		t.Errorf("stderr = %q (status %d), want silence and 0", errs, status)
	}
}

// **A name that refuses is not a name the shell has.** The two questions a
// module loader asks are different ones, and answering the first with the
// second is how "refuses legibly" would quietly become "implemented" — which
// would let a module load over a parameter that reads empty, undoing the rule
// this seam exists to serve.
func TestAnAbsentParameterIsNotADynamicOne(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.SetAbsentParameter("nothere", "parameter not implemented yet")
	r.SetDynamicAssoc("real", func(*Runner) AssocArray { return AssocArray{"k": "v"} })
	if r.DynamicParameter("nothere") {
		t.Error("DynamicParameter(nothere) = true, want false — nothing produces it")
	}
	if !r.AbsentParameter("nothere") {
		t.Error("AbsentParameter(nothere) = false, want true")
	}
	if !r.DynamicParameter("real") || r.AbsentParameter("real") {
		t.Error("a produced parameter answered the absent question, or failed the produced one")
	}
	if r.AbsentParameter("neither") || r.DynamicParameter("neither") {
		t.Error("a name nobody registered answered one of the two questions")
	}
}
