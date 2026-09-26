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

// A command whose words all expanded away reports the status of the last
// command substitution it ran (#4589).
//
// **Core, not a dialect split.** Measured 2026-09-26 from a script file
// across the whole panel — bash 5.3.20 (`/opt/homebrew/bin/bash`), bash
// 3.2.57 (`/bin/bash`), ksh93u+ 2012-08-01, dash 0.5.12, zsh 5.9.2 and
// BusyBox ash 1.37.0 in the digest-pinned alpine the oracle names — which
// answer every row below identically. So there is no axis here and the rule
// lives in the substrate; the one row the panel *does* split on has a
// redirection in it and is zsh's null command, which is a different rule with
// a home of its own in Runner.nullCommand.
//
// The noun is **the last substitution the command ran**, and two things it is
// not:
//
//   - not the last one *written*. `X=$(exit 2) $(exit 3)` is 2 in every
//     column, because a command's words expand before its assignments do.
//   - not the last one that *produced* anything. `$(exit 3) $(echo)` is 0 in
//     every column, the substitution that decided it having printed nothing.
//
// Every row is written with a `false` in front of it. That is not decoration:
// this shell answered 0 for all of them, so a row whose right answer is 0
// cannot tell a working rule from no rule at all unless `$?` is something
// else when the line starts.
func TestACommandThatExpandedAwayReportsItsLastSubstitution(t *testing.T) {
	for _, c := range []struct{ name, src, want, why string }{
		{
			"one substitution, and it failed",
			`false; $(exit 3); echo "st=$?"`,
			"st=3",
			"the command ran nothing, so the substitution is what reported",
		},
		{
			"the last one is zero",
			`false; $(exit 3) $(exit 0); echo "st=$?"`,
			"st=0",
			"the last substitution decides, and a zero from it is a zero",
		},
		{
			"the last one is not zero",
			`false; $(exit 0) $(exit 3); echo "st=$?"`,
			"st=3",
			"the same rule with the two swapped, which is what says it is " +
				"the last one rather than the worst one",
		},
		{
			"two in one word",
			`false; $(exit 3)$(exit 4); echo "st=$?"`,
			"st=4",
			"the word they are in is not the unit; the substitution is",
		},
		{
			"the deciding one printed nothing",
			`false; $(exit 3) $(echo); echo "st=$?"`,
			"st=0",
			"it is the last substitution and not the last one to produce a " +
				"word: an empty one still decides",
		},
		{
			"an empty parameter is not a substitution",
			`false; e=; $e; echo "st=$?"`,
			"st=0",
			"the control — a command that expanded away having run nothing " +
				"at all reports success, which is what keeps this from " +
				"being `an empty command keeps $?`",
		},
		{
			"an empty parameter after the substitution",
			`false; e=; $(exit 3) $e; echo "st=$?"`,
			"st=3",
			"a word that carried no substitution does not reset the answer",
		},
		{
			"an assignment expands after the words",
			`false; X=$(exit 2) $(exit 3); echo "st=$? X=[$X]"`,
			"st=2 X=[]",
			"the noun is the last substitution *run*, and the assignment's " +
				"runs last — this is the row that falsifies `the last one " +
				"written`",
		},
		{
			"and its zero wins over the word's failure",
			`false; X=$(exit 0) $(exit 3); echo "st=$?"`,
			"st=0",
			"the same pair with the statuses swapped, so the row above " +
				"cannot be passing because 2 happens to be first",
		},
		{
			"the last assignment among several",
			`false; X=$(exit 2) Y=$(exit 4); echo "st=$?"`,
			"st=4",
			"assignments among themselves run in the order written",
		},
		{
			"an assignment prefix with no substitution in it",
			`false; X=1 $(exit 3); echo "st=$? X=[$X]"`,
			"st=3 X=[1]",
			"the prefix ran and reported nothing, so the word's " +
				"substitution is still the last one that did — and it is a " +
				"bare assignment, so it outlives the command it prefixed",
		},
		{
			"a plain assignment still succeeds",
			`false; X=1; echo "st=$?"`,
			"st=0",
			"the second control: nothing in the command reported, so the " +
				"command reports success",
		},
		{
			"the older spelling",
			"false; `exit 3`; echo \"st=$?\"",
			"st=3",
			"backquotes are the same substitution and take the same route",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := strings.TrimSpace(expandedAwayScript(t, c.src)); got != c.want {
				t.Errorf("wrote %q, want %q — %s", got, c.want, c.why)
			}
		})
	}
}

// A substitution with nothing in its body reports success, whatever the shell
// holding it was doing (#4589).
//
// Its own rule rather than a corollary of the one above, and it has to be:
// the clone a substitution's body runs on carries the caller's status in, so
// that `$?` read *inside* the body names the command before the substitution
// — measured, `false; echo $(echo $?)` writes 1 in every column of the panel
// — and a body with no command in it has nothing to overwrite that with. So
// an empty body used to hand the borrowed status straight back out, which was
// invisible while an expanded-away command reported 0 regardless and became
// `false; ``` answering 1 the moment it did not.
//
// Unanimous across the same six columns, 2026-09-26.
func TestAnEmptySubstitutionBodyReportsSuccess(t *testing.T) {
	for _, c := range []struct{ name, src, want, why string }{
		{
			"nothing but spaces, in a word",
			"false; $( ); echo \"st=$?\"",
			"st=0",
			"the body ran no command, so it succeeded — and the command " +
				"holding it reports what the substitution reported",
		},
		{
			"nothing but spaces, on the right of an assignment",
			`false; X=$( ); echo "st=$?"`,
			"st=0",
			"the same substitution reached through the assignment rule, " +
				"which is the route that showed this at all",
		},
		{
			"nothing but a newline",
			"false; X=$(\n); echo \"st=$?\"",
			"st=0",
			"a body is empty by holding no command, not by holding no text",
		},
		{
			"nothing but a comment",
			"false; X=`# nothing here\n`; echo \"st=$?\"",
			"st=0",
			"the same, through the older spelling and with the body a " +
				"comment rather than blank",
		},
		{
			"a body that runs something still reads the caller's status",
			`false; echo "st=$(echo $?)"`,
			"st=1",
			"the control, and the reason the rule above is guarded on the " +
				"body being empty: `$?` inside a body is still the command " +
				"before the substitution, so zeroing the clone outright " +
				"would answer 0 here",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := strings.TrimSpace(expandedAwayScript(t, c.src)); got != c.want {
				t.Errorf("wrote %q, want %q — %s", got, c.want, c.why)
			}
		})
	}
}

// What `set -e` does with a command that expanded away, which is the rule
// above seen from the place a wrong 0 is most expensive: a script that meant
// to stop carried on.
//
// bash 5.3.20, bash 3.2.57, ksh93u+, dash, zsh 5.9.2 and BusyBox ash all end
// the shell at `$(exit 3)` and none of them reaches the line after it. This
// reached it, because the status it was deciding on was 0.
func TestErrExitStopsOnACommandThatExpandedAway(t *testing.T) {
	out := &strings.Builder{}
	r := expandedAwayRunner(t, out)
	runExpandedAway(t, r, "set -e\n$(exit 3)\necho after\n")
	if got := out.String(); strings.Contains(got, "after") {
		t.Errorf("wrote %q, want the shell stopped before `after`", got)
	}
	if got, want := r.Exited(), true; got != want {
		t.Errorf("Exited() is %v, want %v — the option has to see the status", got, want)
	}
}

// expandedAwayScript runs src on a core shell and hands back what it wrote.
//
// The status is read back through `$?` written into the output rather than
// from the Runner, because these rows are about what the *command* reported
// and the Runner's status is the script's — the trailing `echo` would be the
// last thing to set it.
func expandedAwayScript(t *testing.T, src string) string {
	t.Helper()
	out := &strings.Builder{}
	r := expandedAwayRunner(t, out)
	runExpandedAway(t, r, src)
	return out.String()
}

// expandedAwayRunner is a shell with no dialect in it, which is the claim
// this file makes: the panel is unanimous, so the rule belongs to the
// substrate and a preset must not be needed to reach it.
func expandedAwayRunner(t *testing.T, out *strings.Builder) *Runner {
	t.Helper()
	sem := PosixSemantics()
	return newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: out,
	})
}

// runExpandedAway parses src as core shell and runs it, tolerating the one
// error a run here is allowed to end on: `set -e` stopping the script is the
// subject of a test in this file, and Runner.Run reports it.
func runExpandedAway(t *testing.T, r *Runner, src string) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil && !r.Exited() {
		t.Fatal(err)
	}
}
