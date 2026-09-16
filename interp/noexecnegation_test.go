// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `set -n` reads and runs nothing, so the status a `!` in front of a pipeline
// would invert was reported by no command. Six of the seven panel columns
// leave it alone and zsh inverts it anyway — see
// Semantics.UnrunNegationInvertsTheStatus, where the panel is measured.
//
// The whole of this defect was the status, and both streams stayed empty
// throughout it, so a test that read only the output would have passed for
// the year `sh -n` answered 1 to a file with nothing wrong with it (#3179).

func noexecNegationSem(a Answer) Semantics {
	s := permissive()
	s.UnrunNegationInvertsTheStatus = a
	return s
}

func TestUnrunNegationInvertsOnlyWhereTheDialectSays(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		yes  int
		no   int
	}{
		{"the whole file", "set -n\n! true\n", 1, 0},
		{"after a command that did run", "true\nset -n\n! true\n", 1, 0},
		{"a negated pipeline", "set -n\n! true | cat\n", 1, 0},
		// Twice inverts back, so both answers agree here — the row is what
		// says the axis is about the `!` and not about the reading.
		{"twice", "set -n\n! true\n! true\n", 0, 0},
		// And a negation inside an `if` is the clause's business: `set -n`
		// never walks into the compound, so no `!` is reached at all.
		{"inside an if", "set -n\nif ! true; then :; fi\n", 0, 0},
		// Nothing else reachable under `set -n` moves the status, because no
		// command runs to report one.
		{"a bare false", "set -n\nfalse\n", 0, 0},
	} {
		out, st := run(t, tc.src, withSem(noexecNegationSem(Yes)))
		if out != "" || st != tc.yes {
			t.Errorf("%s, inverting: got %q status %d, want %q status %d",
				tc.name, out, st, "", tc.yes)
		}
		out, st = run(t, tc.src, withSem(noexecNegationSem(No)))
		if out != "" || st != tc.no {
			t.Errorf("%s, not inverting: got %q status %d, want %q status %d",
				tc.name, out, st, "", tc.no)
		}
	}
}

// TestUnrunNegationIsAskedOnlyUnderNoExec is what keeps the core usable.
// `! grep -q pat file` is ordinary in a script and no shell disagrees about
// what its `!` does, so asking the vector on the running route would refuse a
// question the panel never posed.
func TestUnrunNegationIsAskedOnlyUnderNoExec(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want int
	}{
		{"! true", 1},
		{"! false", 0},
		{"! true | cat", 1},
		{"true; ! true", 1},
	} {
		out, st := run(t, tc.src, withSem(CoreSemantics()))
		if out != "" || st != tc.want {
			t.Errorf("%s under the core: got %q status %d, want %q status %d",
				tc.src, out, st, "", tc.want)
		}
	}
}

// TestUnrunNegationRefusedNamesItselfAndEndsAtTwo pins the diagnostic and the
// status together, which is the pairing this defect was invisible to: the
// core wrote nothing at all and exited 1, so a check on either half alone
// would have read as a syntax error nobody could see.
//
// The order is load-bearing and is why the status is asserted beside the
// text. The refusal sets 2 and then declines to invert; an implementation
// that inverted first and asked afterwards would answer 2 here as well, but
// one that asked and then inverted anyway would turn the 2 into a 0 and
// report success under its own diagnostic.
func TestUnrunNegationRefusedNamesItselfAndEndsAtTwo(t *testing.T) {
	out, st := run(t, "set -n\n! true\n", withSem(CoreSemantics()))
	if st != 2 {
		t.Errorf("an unanswered axis should end at 2, got status %d with %q", st, out)
	}
	for _, want := range []string{
		"`!` inverting the status of a pipeline `set -n` never ran",
		"no dialect was chosen",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("refusal %q does not say %q", out, want)
		}
	}
}
