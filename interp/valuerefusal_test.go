// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// TestARefusedValueTakesTheFreezesRoute pins RefuseValues' one promise: a
// value the name does not take is refused exactly where, and at exactly the
// cost, a freeze on the same name would be — only the sentence differs. Each
// route is run twice, once over a frozen name and once over a name refusing
// the value, and the two transcripts must agree once the sentences are
// swapped. Every door a store comes through is a row, because a door that
// forgets to ask stores the value and the transcripts part.
func TestARefusedValueTakesTheFreezesRoute(t *testing.T) {
	t.Parallel()
	const frozen = "N: readonly variable"
	const refused = "no: x"
	for _, src := range []string{
		`N=x; echo "after $?"`,
		`export N=x; echo "after $?"`,
		`N=x true; echo "after $?"`,
		`N=x /usr/bin/true; echo "after $?"`,
		`f() { echo "in"; }; N=x f; echo "after $?"`,
		`for N in x; do echo body; done; echo "after $?"`,
		`eval N=x; echo "after $?"`,
		"read N <<E\nx\nE\necho \"after $?\"",
		`getopts a N -a; echo "after $?"`,
		`(N=x); echo "after $?"`,
		`: $((N=0)); echo "after $?"`,
	} {
		sem := PosixSemantics()
		// The prefix axes the frozen rows reach, answered as the one dialect
		// that registers a refusal answers them.
		sem.PrefixToAFrozenNameIsCheckedFirst = FrozenPrefixCheckedWithTheCommand
		sem.FrozenPrefixIsCheckedBeforeItsValue = No
		sem.PrefixExportAtABuiltin = PrefixExportAtABuiltinUnchanged
		freeze, freezeSt := run(t, src, func(r *Runner) { r.Semantics = &sem; r.MarkReadonly("N") })
		value, valueSt := run(t, src, func(r *Runner) {
			r.Semantics = &sem
			r.RefuseValues("N", func(v string) (string, bool) {
				if v == "x" || v == "a" || v == "0" {
					return "no: " + v, true
				}
				return "", false
			})
		})
		value = strings.ReplaceAll(strings.ReplaceAll(value, "no: a", refused), "no: 0", refused)
		if !strings.Contains(freeze, frozen) {
			t.Fatalf("%s: the freeze was not refused, so the row compares nothing: %q", src, freeze)
		}
		if want := strings.ReplaceAll(freeze, frozen, refused); value != want || valueSt != freezeSt {
			t.Errorf("%s\n got %q (%d)\nwant %q (%d)", src, value, valueSt, want, freezeSt)
		}
	}
}

// TestAValueTheNameTakesIsStored is the control: a value the check passes is
// stored on every one of those routes.
func TestAValueTheNameTakesIsStored(t *testing.T) {
	t.Parallel()
	out, st := run(t, `N=5; echo "[$N]"; export N=6; echo "[$N]"; read N <<E
7
E
echo "[$N]"`, func(r *Runner) {
		r.RefuseValues("N", func(v string) (string, bool) { return "no", v == "x" })
	})
	if out != "[5]\n[6]\n[7]\n" || st != 0 {
		t.Errorf("got %q (%d)", out, st)
	}
}
