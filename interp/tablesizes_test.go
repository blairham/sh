// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/blairham/sh/interp"
)

// A reservation is invisible to a script, so what it is for has to be seen
// in what it costs: registering as many names as were reserved allocates
// nothing past the reservation itself, where registering them into tables
// made empty grows each table several times on the way.
func TestAReservedTableDoesNotGrow(t *testing.T) {
	names := make([]string, 100)
	for i := range names {
		names[i] = fmt.Sprintf("name%d", i)
	}
	builtin := func(*interp.Runner, context.Context, []string) int { return 0 }
	fill := func(r *interp.Runner) {
		for _, n := range names {
			r.Register(n, builtin)
			r.MarkShellOwnParameter(n)
			r.SetVar(n, "")
		}
	}
	plain := testing.AllocsPerRun(20, func() { fill(emptyRunner()) })
	reserved := testing.AllocsPerRun(20, func() {
		r := emptyRunner()
		r.ReserveTables(interp.TableSizes{Builtins: len(names), ShellOwn: len(names), Parameters: len(names)})
		fill(r)
	})
	if reserved >= plain {
		t.Errorf(
			"registering %d names allocated %.0f times into reserved tables and %.0f into empty ones",
			len(names),
			reserved,
			plain,
		)
	}
	got := emptyRunner().TableSizes()
	if got != (interp.TableSizes{}) {
		t.Errorf("a runner nothing has written holds %+v", got)
	}
	r := emptyRunner()
	fill(r)
	if got := r.TableSizes(); got.Builtins != len(names) || got.ShellOwn != len(names) || got.Parameters < len(names) {
		t.Errorf("after %d registrations the tables hold %+v", len(names), got)
	}
}

// A reservation made after a table exists leaves it alone, so a name already
// in it is still there.
func TestAReservationKeepsWhatATableHolds(t *testing.T) {
	t.Parallel()
	r := emptyRunner()
	r.Register("early", func(*interp.Runner, context.Context, []string) int { return 7 })
	r.SetVar("early", "kept")
	r.ReserveTables(interp.TableSizes{Builtins: 64, Parameters: 64})
	if _, ok := r.Builtin("early"); !ok {
		t.Error("the builtin registered before the reservation is gone")
	}
	if v, _ := r.GetVar("early"); v != "kept" {
		t.Errorf("the parameter stored before the reservation reads %q", v)
	}
}

// emptyRunner is a runner nothing has written, which is what a reservation is
// made into.
func emptyRunner() *interp.Runner {
	// testrunner:bare — nothing here runs a command, and the subject is how
	// much a runner's own tables allocate: the helper's directory and cleanup
	// would be counted by AllocsPerRun on both sides of the comparison.
	return &interp.Runner{}
}
