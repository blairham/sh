// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `$!` before a background command has been started splits the panel twice,
// on two questions that look like one (#937).
//
// The first is what it *reads*: nothing in bash 5.3.15, bash 3.2.57, bash 3.2
// run as `sh`, dash and ksh93u+, and `0` in zsh 5.9.2 — a number nothing ever
// had. That one is pinned as a row of TestSemanticsAxesHaveTwoSides, because
// it is a value and nothing more.
//
// The second is whether it is *set*, which is a different split and the more
// useful one: `set -u` exists to stop exactly this read. bash and dash stop;
// ksh93 and zsh carry on. Neither answer predicts the other — zsh's zero is a
// value and ksh93's empty is a set parameter, so the two quiet shells are
// quiet for different reasons — which is why they are two fields.
func TestTheLastBackgroundPidIsUnsetBeforeAnyJobIsAnAxis(t *testing.T) {
	const src = `set -u; echo "[$!]"; echo after`

	unset := permissive()
	unset.LastBackgroundPidIsUnsetBeforeAnyJob = Yes
	got, st := run(t, src, withSem(unset))
	if strings.Contains(got, "after") || st == 0 {
		t.Errorf("Yes: the script should have stopped, got %q/%d", got, st)
	}
	if !strings.Contains(got, "!") {
		t.Errorf("Yes: the refusal should name the parameter, got %q", got)
	}

	quiet := permissive()
	quiet.LastBackgroundPidIsUnsetBeforeAnyJob = No
	if got, st := run(t, src, withSem(quiet)); got != "[]\nafter\n" || st != 0 {
		t.Errorf("No: got %q/%d, want %q/0", got, st, "[]\nafter\n")
	}
}

// And once a job has been started the parameter is set on both sides, which is
// what says the refusal is about nothing having run rather than about `$!`.
//
// Measured that way too: with one job behind it, `set -u` has nothing to say
// about `$!` in any of the six columns.
func TestTheLastBackgroundPidIsSetOnceAJobHasRun(t *testing.T) {
	for _, w := range []struct {
		name   string
		answer Answer
	}{{"where an unstarted one would be unset", Yes}, {"and where it would not", No}} {
		t.Run(w.name, func(t *testing.T) {
			sem := permissive()
			sem.LastBackgroundPidIsUnsetBeforeAnyJob = w.answer
			src := `set -u; /bin/sleep 0 & wait; x=$!; echo "[${x:+yes}] after"`
			if got, st := run(t, src, withSem(sem)); got != "[yes] after\n" || st != 0 {
				t.Errorf("got %q/%d, want %q/0", got, st, "[yes] after\n")
			}
		})
	}
}

// The two axes are independent, and zsh is the combination that proves it: a
// zero that `set -u` is content with.
//
// Asserting one side of each in isolation would not catch a reading that made
// the zero imply the setness, which is the reading a single field would force.
func TestTheTwoLastBackgroundPidAxesAreIndependent(t *testing.T) {
	for _, w := range []struct {
		name        string
		zero, unset Answer
		want        string
		stops       bool
	}{
		{"zero and content, which is zsh", Yes, No, "[0]\nafter\n", false},
		{"empty and content, which is ksh93", No, No, "[]\nafter\n", false},
		{"empty and unset, which is bash and dash", No, Yes, "", true},
		{"zero and unset, which no shell in the panel is", Yes, Yes, "", true},
	} {
		t.Run(w.name, func(t *testing.T) {
			sem := permissive()
			sem.LastBackgroundPidIsZeroBeforeAnyJob = w.zero
			sem.LastBackgroundPidIsUnsetBeforeAnyJob = w.unset
			got, st := run(t, `set -u; echo "[$!]"; echo after`, withSem(sem))
			if w.stops {
				if strings.Contains(got, "after") || st == 0 {
					t.Errorf("should have stopped, got %q/%d", got, st)
				}
				return
			}
			if got != w.want || st != 0 {
				t.Errorf("got %q/%d, want %q/0", got, st, w.want)
			}
		})
	}
}

// The refusal's words and its status are the dialect's, and they are the same
// two fields an unset positional reads.
//
// One wording for both because it is one measurement: bash writes the `$` back
// and says `$!: unbound variable` exactly as it says `$1: unbound variable`,
// and dash writes the name alone and says `!: parameter not set`. A third
// field would have held a copy of one of these in all four presets.
func TestTheLastBackgroundPidRefusalIsWordedByTheDialect(t *testing.T) {
	withDialect := func(sem Semantics, diag Diagnostics) func(*Runner) {
		return func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &diag }
	}
	sem := permissive()
	sem.LastBackgroundPidIsUnsetBeforeAnyJob = Yes

	sigil := Diagnostics{UnboundPositional: "$%[1]s: measured sigil wording"}
	if got, _ := run(t, `set -u; echo "$!"`, withDialect(sem, sigil)); !strings.Contains(got, "$!: measured sigil wording") {
		t.Errorf("the sigil wording is the one consulted: got %q", got)
	}

	// Empty means "the same line as an unset name", which is what three of
	// the four presets leave it at.
	plain := Diagnostics{UnboundVariable: "%[1]s: measured plain wording"}
	if got, _ := run(t, `set -u; echo "$!"`, withDialect(sem, plain)); !strings.Contains(got, "!: measured plain wording") {
		t.Errorf("the fallback wording names the parameter: got %q", got)
	}
}

// `${!x}` is a different construct that happens to share the character, and
// the axis does not reach it.
//
// It cannot, and the reason is worth writing down rather than guarding: an
// indirection parses with the name `x` and the indirect flag set, so the name
// the axis tests is never `!` for one. A `!e.Indirect` clause was written into
// the test on the assumption that it could, and a mutant that removed it
// survived — which is what said the clause was dead. This case stays as the
// regression guard for the construct, not for that clause.
func TestTheAxisDoesNotReachAnIndirection(t *testing.T) {
	sem := permissive()
	sem.LastBackgroundPidIsUnsetBeforeAnyJob = Yes
	sem.IndirectionYieldsName = No
	enable := func(d *syntax.Dialect) { d.ParamIndirection = true }
	if got, st := runGrammar(t, `y=hello; x=y; echo "[${!x}]"`, enable, withSem(sem)); got != "[hello]\n" || st != 0 {
		t.Errorf("an indirection through a set name: got %q/%d, want %q/0", got, st, "[hello]\n")
	}
}
