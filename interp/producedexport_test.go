// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A produced parameter a script exports reaches a child with the value the
// name answers *now* — #4159.
//
// It is answered by a function rather than stored, so it is in the variable
// table only if something assigned to it and in the inherited environment only
// if it arrived there: neither walk that builds a child's environment could
// see one, and every such name reached every child as nothing at all.
//
// Measured 2026-09-22 against bash 5.3.20: `export SECONDS`, `export RANDOM`
// and `export BASH_ARGV0=x` each put the name in a child's environment there,
// and this shell put none of the three. #4196 had closed exactly this hole for
// the bound option records; the rule is the same one and lives in one place.
func TestAnExportedProducedParameterReachesAChild(t *testing.T) {
	out, status := run(t, `export P; /usr/bin/env | grep '^P=' || echo "(none)"`,
		func(r *Runner) {
			r.SetDynamic("P", func(*Runner) string { return "produced" })
		})
	if status != 0 || out != "P=produced\n" {
		t.Errorf("got %q status %d, want the child told what the name answers", out, status)
	}
}

// And one nothing exported reaches no child, which is the half that keeps this
// from handing every command the whole parameter table.
func TestAProducedParameterNobodyExportedReachesNoChild(t *testing.T) {
	out, status := run(t, `/usr/bin/env | grep '^P=' || echo "(none)"`,
		func(r *Runner) {
			r.SetDynamic("P", func(*Runner) string { return "produced" })
		})
	if status != 0 || out != "(none)\n" {
		t.Errorf("got %q status %d, want the name kept to this shell", out, status)
	}
}

// A produced name that says it is not *there* is not handed down either — the
// same answer an unset name gets everywhere else. See SetDynamicPresence.
func TestAProducedParameterThatIsNotThereReachesNoChild(t *testing.T) {
	out, status := run(t, `export P; /usr/bin/env | grep '^P=' || echo "(none)"`,
		func(r *Runner) {
			r.SetDynamic("P", func(*Runner) string { return "produced" })
			r.SetDynamicPresence("P", func(*Runner) bool { return false })
		})
	if status != 0 || out != "(none)\n" {
		t.Errorf("got %q status %d, want a name that is not there handed down as nothing", out, status)
	}
}
