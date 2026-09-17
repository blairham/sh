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

// The names `${!prefix@}` comes to include the ones held in a **compound**
// table, and one shape of those is an axis. Tests name the axis, never a
// shell — the measurements are in Semantics.PrefixListingNamesADeclaredOnly-
// Compound and in the two dialect suites that answer it.
//
// An array has a scalar cell beside it and a keyed table has none at all, so
// a listing built from the scalar table alone knew a name everywhere a
// lookup, an export or `declare -p` asks and did not know it here. What a
// caller does with nothing is the damage rather than the empty listing:
// `declare -p ${!m@}` with nothing to substitute is `declare -p` with no
// operands, which prints the whole shell.

// prefixCompoundRun runs src in a dialect that has both `${!prefix@}` and a
// declaration word able to bring a compound into being, and reports stdout,
// stderr and status.
func prefixCompoundRun(t *testing.T, src string, set func(*Semantics)) (string, string, int) {
	t.Helper()
	d := syntax.Core()
	d.ParamIndirection = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.DeclareOptions = "aAfgilmprux"
	sem.DeclaredNameWithoutValueIsEmpty = No
	if set != nil {
		set(&sem)
	}
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Dialect: &d,
		Dir: t.TempDir(), Name: "testsh",
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return strings.TrimSpace(out.String()), errs.String(), st
}

// A name whose only home is a compound table is a name the listing offers.
// The keyed table is the shape that had nowhere else to be found; the
// indexed one is here beside it so that a store which stops writing a scalar
// cell cannot reopen this quietly.
func TestAPrefixListingNamesACompoundTable(t *testing.T) {
	written := func(s *Semantics) { s.PrefixListingNamesADeclaredOnlyCompound = No }
	for _, tc := range []struct{ name, src, want string }{
		{"a keyed table", `typeset -A zqm=([k]=v); echo "[${!zq@}]"`, "[zqm]"},
		{"an indexed array", `zqa[3]=x; echo "[${!zq@}]"`, "[zqa]"},
		{"both, sorted", `typeset -A zqm=([k]=v); zqa[3]=x; echo "[${!zq@}]"`, "[zqa zqm]"},
		// The star spelling is the same listing joined differently, exactly
		// as `$@` and `$*` are.
		{"the star spelling", `typeset -A zqm=([k]=v); zqa[3]=x; printf "[%s]" "${!zq*}"`, "[zqa zqm]"},
		// A scalar beside them, so the answer is not "everything this shell
		// holds" arriving by another route.
		{"a name outside the prefix", `typeset -A zqm=([k]=v); other=1; echo "[${!zq@}]"`, "[zqm]"},
		// And `unset` takes a compound name out of the listing the way it
		// takes a scalar out: what cannot be read is not named.
		{"unset takes it away", `typeset -A zqm=([k]=v); unset zqm; echo "[${!zq@}]"`, "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := prefixCompoundRun(t, tc.src, written)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.src, out, errs, st, tc.want)
			}
		})
	}
}

// The axis: a table a declaration brought into being and nothing has written
// to. It is the *assignment* that moves the name rather than the emptiness,
// so `=()` behind the same declaration is listed under both answers and an
// element written and then unset is too.
func TestADeclaredOnlyCompoundIsListedOnlyWhereTheAxisSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		yes, no   string
	}{
		{"a keyed table", `typeset -A zqm; echo "[${!zq@}]"`, "[zqm]", "[]"},
		{"an indexed array", `typeset -a zqa; echo "[${!zq@}]"`, "[zqa]", "[]"},
		// Assigned an empty compound, which is the control: the same
		// emptiness, the other side of the record, listed either way.
		{"assigned empty", `typeset -A zqm=(); echo "[${!zq@}]"`, "[zqm]", "[zqm]"},
		// Written to and emptied again, which is the same control reached
		// from the other direction.
		{"written then emptied", `typeset -A zqm; zqm[k]=v; unset "zqm[k]"; echo "[${!zq@}]"`, "[zqm]", "[zqm]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, side := range []struct {
				answer Answer
				want   string
			}{{Yes, tc.yes}, {No, tc.no}} {
				out, errs, st := prefixCompoundRun(t, tc.src, func(s *Semantics) {
					s.PrefixListingNamesADeclaredOnlyCompound = side.answer
				})
				if out != side.want || errs != "" || st != 0 {
					t.Errorf("%s under %v = %q (stderr %q, status %d), want %q",
						tc.src, side.answer, out, errs, st, side.want)
				}
			}
		})
	}
}

// Unanswered refuses rather than guessing, and it refuses **only** where such
// a name is a candidate: a shell that has written to everything it declared
// never reaches the axis, so the listing beside it is silent and at 0.
func TestTheDeclaredOnlyAxisIsAskedOnlyWhereItDecides(t *testing.T) {
	out, errs, st := prefixCompoundRun(t, `typeset -A zqm=([k]=v); echo "[${!zq@}]"`, nil)
	if out != "[zqm]" || errs != "" || st != 0 {
		t.Errorf("a written table = %q (stderr %q, status %d), want the name and silence", out, errs, st)
	}
	out, errs, st = prefixCompoundRun(t, `typeset -A zqm; echo "[${!zq@}]"`, nil)
	if !strings.Contains(errs, "declaration brought into being") || st == 0 {
		t.Errorf("a declared-only table = %q (stderr %q, status %d), want a refusal naming the axis", out, errs, st)
	}
	// And a declared-only name outside the prefix decides nothing, so it is
	// not asked either.
	out, errs, st = prefixCompoundRun(t, `typeset -A other; zqa[3]=x; echo "[${!zq@}]"`, nil)
	if out != "[zqa]" || errs != "" || st != 0 {
		t.Errorf("outside the prefix = %q (stderr %q, status %d), want the name and silence", out, errs, st)
	}
}
