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

// A failed expansion gives up the line it happened on and the shell carries
// on at the next one, where the dialect says so — and ends the shell where it
// does not.
//
// The failure used to be controlExit in every dialect, which is the same
// answer as controlAbandon for any probe whose commands are on **one line**:
// `echo $((1/0)); echo after` prints nothing more either way. Every probe in
// the tree was that shape, so one unreadable expansion ending a whole file
// looked correct (#1171).
//
// So the shape of every row here is two statements on two lines, and what is
// asserted is the second one running. A row with a `;` in it would pass
// against both readings and is kept below only as the control that says the
// line is the unit.

// expandAbandonRun runs src with the axis set as given and returns what the
// shell wrote and what it exited with.
func expandAbandonRun(t *testing.T, src string, abandons Answer) (string, string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.FailedExpansionAbandonsTheLine = abandons
	var out, errs bytes.Buffer
	dir := t.TempDir()
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem,
		Diagnostics: &Diagnostics{Location: LocationLineWord},
		Dir:         dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

// The failures this axis covers, each in the two-line shape, under both
// answers. One list per failure rather than one row, because "a failed
// expansion" is several failures sharing the expandErr flag and a fix that
// reached only the arithmetic one would pass a division-by-zero row alone.
func TestAFailedExpansionGivesUpTheLineOrTheShell(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a division by zero", "echo pre\necho $((1/0))\necho after"},
		{"an arithmetic expression the parser refused", "echo pre\necho $((1+))\necho after"},
		{"a bad substitution", "echo pre\necho ${x@ZZZ}\necho after"},
		// A bad *subscript* is the same failure and is asserted in the
		// dialect that has arrays: the preset here is the standard's, which
		// has no subscript for the arithmetic to be wrong in.
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := expandAbandonRun(t, tc.src, Yes)
			if !strings.Contains(out, "pre") {
				t.Fatalf("giving up the line: output %q, want the line before it to have run", out)
			}
			if errs == "" {
				t.Fatalf("giving up the line: nothing was reported, so this row is not the failure it names")
			}
			if !strings.Contains(out, "after") {
				t.Errorf("giving up the line: output %q, want the next line to run", out)
			}
			if st != 0 {
				t.Errorf("giving up the line: status %d, want the next line's 0", st)
			}

			out, errs, st = expandAbandonRun(t, tc.src, No)
			if !strings.Contains(out, "pre") || errs == "" {
				t.Fatalf("ending the shell: output %q errs %q, want the failure reached", out, errs)
			}
			if strings.Contains(out, "after") {
				t.Errorf("ending the shell: output %q, want nothing after it", out)
			}
			if st == 0 {
				t.Errorf("ending the shell: status %d, want the failure's", st)
			}
		})
	}
}

// The control that says the unit is the line: with a `;` the rest of the
// *list* is given up under both answers, so this row cannot tell them apart
// and is what every earlier probe looked like.
func TestAFailedExpansionGivesUpTheRestOfItsListUnderEitherAnswer(t *testing.T) {
	for _, abandons := range []Answer{Yes, No} {
		out, _, st := expandAbandonRun(t, "echo pre; echo $((1/0)); echo after", abandons)
		if strings.Contains(out, "after") {
			t.Errorf("abandons=%v: output %q, want the rest of the list given up", abandons, out)
		}
		if !strings.Contains(out, "pre") {
			t.Errorf("abandons=%v: output %q, want the command before it to have run", abandons, out)
		}
		if st == 0 {
			t.Errorf("abandons=%v: status %d, want the failure's", abandons, st)
		}
	}
}

// Giving up the line unwinds past every shape that holds statements and is
// consumed at the top-level statement loop, which is controlAbandon's whole
// definition — and is what a readonly refusal already did. Asserted for each
// shape because a construct that swallowed the give-up would carry on inside
// itself, and a loop is where that shows as a second complaint.
func TestGivingUpTheLineUnwindsPastEveryEnclosingShape(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a loop", "for i in 1 2 3; do echo $((1/0)); echo one; done\necho two"},
		{"a while", "while :; do echo $((1/0)); echo one; break; done\necho two"},
		{"an if", "if true; then echo $((1/0)); echo one; fi\necho two"},
		{"a group", "{ echo $((1/0)); echo one; }\necho two"},
		{"a function body", "f() { echo $((1/0)); echo one; }\nf\necho two"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := expandAbandonRun(t, tc.src, Yes)
			if n := strings.Count(errs, "division"); n != 1 {
				t.Errorf("complained %d times, want once: %q", n, errs)
			}
			if strings.Contains(out, "one") {
				t.Errorf("output %q, want what enclosed it given up", out)
			}
			if !strings.Contains(out, "two") {
				t.Errorf("output %q, want the shell to carry on", out)
			}
			if st != 0 {
				t.Errorf("status %d, want the last command's", st)
			}
		})
	}
}

// With nothing after it the failure's own status stands, which is what makes
// the 0 in the rows above the *next line's* rather than the give-up's.
// The number is FatalErrorStatusIsOne's, which the preset here answers No —
// so 2 rather than 1, and asserting the specific number is the point: a 0
// would mean the give-up had swallowed the failure's status along with the
// rest of the line.
func TestGivingUpTheLineLeavesTheFailuresStatusBehind(t *testing.T) {
	if _, _, st := expandAbandonRun(t, "echo pre\necho $((1/0))", Yes); st != 2 {
		t.Errorf("status %d, want the failure's 2", st)
	}
	// And the same failure ending the shell leaves the same number, which is
	// what says the status and the reach are two questions.
	if _, _, st := expandAbandonRun(t, "echo pre\necho $((1/0))", No); st != 2 {
		t.Errorf("ending the shell: status %d, want the same 2", st)
	}
}

// The route is not the question, which is the correction #1171 turned up:
// the same program answers the same way whether it came from a file or from
// a command-string argument. Both routes with both separators, because the
// two cells that used to be compared were the diagonal of this square.
func TestGivingUpTheLineDoesNotDependOnHowTheShellWasStarted(t *testing.T) {
	for _, route := range []Route{RouteScriptFile, RouteCommandString} {
		for _, tc := range []struct {
			why, src string
			wantNext bool
		}{
			{"separate lines, so the next line runs", "echo pre\necho $((1/0))\necho after", true},
			{"one list, so the rest of it does not", "echo pre; echo $((1/0)); echo after", false},
		} {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse %q: %v", tc.src, err)
			}
			sem := permissive()
			sem.FailedExpansionAbandonsTheLine = Yes
			var out, errs bytes.Buffer
			dir := t.TempDir()
			r := newTestRunner(t, &Runner{
				Stdout: &out, Stderr: &errs, Semantics: &sem, Route: route,
				Diagnostics: &Diagnostics{Location: LocationLineWord},
				Dir:         dir, Name: "testsh", Vars: map[string]string{"PATH": dir},
			})
			if _, rerr := r.Run(context.Background(), f); rerr != nil {
				t.Fatalf("run %q: %v", tc.src, rerr)
			}
			_ = errs
			if got := strings.Contains(out.String(), "after"); got != tc.wantNext {
				t.Errorf("route %v, %s: ran the next command = %v, want %v (output %q)",
					route, tc.why, got, tc.wantNext, out.String())
			}
		}
	}
}

// The rows above vary the *failure*; these vary the **position** the failed
// expansion stood in, which is the other half and the one that was wrong.
//
// Each position is expanded by different code — a command word by the word
// expander, an assignment's right-hand side by the assignment, a `[` operand
// by the word expander inside a compound, and a `for` list before the loop
// has done anything — so the axis is read at three separate sites and a rule
// applied at one of them leaves the others running on. That is what happened:
// a word and a `[` operand stopped while an assignment and a `for` list
// carried on at status 0, in every dialect, so two rows were wrong against
// the whole panel and two against bash alone (#1229).
//
// Written with the line boundary in the source, because that is the only
// place the two answers differ: within one line every shell abandons the rest
// of the list, and the control above says so.
func TestAFailedExpansionReachesTheSameFarInEveryPositionItCanStandIn(t *testing.T) {
	for _, p := range []struct{ name, line string }{
		{"a command word", `printf "[%s]" $((1/0))`},
		{"an assignment", `x=$((1/0))`},
		{"a test operand", `if [ x = $((1/0)) ]; then echo t; else echo f; fi`},
		{"a for list", `for i in $((1/0)); do echo "i=$i"; done`},
	} {
		t.Run(p.name, func(t *testing.T) {
			src := "echo one\n" + p.line + "\necho two"

			out, errs, st := expandAbandonRun(t, src, Yes)
			if !strings.Contains(out, "one") || errs == "" {
				t.Fatalf("giving up the line: output %q errs %q, want the failure reached", out, errs)
			}
			if !strings.Contains(out, "two") {
				t.Errorf("giving up the line: output %q, want the next line to run", out)
			}
			if st != 0 {
				t.Errorf("giving up the line: status %d, want the next line's 0", st)
			}

			out, errs, st = expandAbandonRun(t, src, No)
			if !strings.Contains(out, "one") || errs == "" {
				t.Fatalf("ending the shell: output %q errs %q, want the failure reached", out, errs)
			}
			if strings.Contains(out, "two") {
				t.Errorf("ending the shell: output %q, want nothing after it to have run", out)
			}
			if st == 0 {
				t.Errorf("ending the shell: status 0, want the failure's own")
			}
		})
	}
}

// And the control that says the four positions are positions rather than four
// spellings of a command word: none of them gets as far as producing a value,
// so nothing downstream of the expansion runs. Without it a `for` list that
// expanded to no fields and a `for` list that was abandoned read the same, and
// so do a condition that was skipped and one that came out false.
func TestAFailedExpansionProducesNothingDownstreamOfIt(t *testing.T) {
	for _, tc := range []struct{ src, unwanted string }{
		{"echo one\nif [ x = $((1/0)) ]; then echo t; else echo f; fi\necho two", "f"},
		{"echo one\nfor i in $((1/0)); do echo \"i=$i\"; done\necho two", "i="},
		{"echo one\nx=$((1/0))\necho \"x=[$x]\"", "x=[0]"},
	} {
		out, _, _ := expandAbandonRun(t, tc.src, Yes)
		if strings.Contains(out, tc.unwanted) {
			t.Errorf("%q got %q, want no %q — the expansion never produced a value",
				tc.src, out, tc.unwanted)
		}
	}
}
