// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `$!` before a background command has been started answers two questions at
// once, and the panel puts three combinations on them (#937, #3011).
//
// The first is what it *reads*: nothing in bash, dash, BusyBox ash and ksh93,
// and `0` in zsh — a number nothing ever had. The second is whether it is
// *set*, which `${!-word}`, `${!+word}` and `set -u` can each see and a bare
// `$!` cannot.
//
// This was a pair of Answer fields, and the pair had a spelling for a state no
// shell is in — set and empty, quiet — and none at all for ksh93's, which is
// unset to the operators and quiet under `set -u`. A form is what holds all
// three; see LastBackgroundPidPolicy.
func TestWhatTheLastBackgroundPidReadsBeforeAnyJob(t *testing.T) {
	for _, w := range []struct {
		name   string
		policy LastBackgroundPidPolicy
		want   string
	}{
		{"unspecified is set and empty", LastBackgroundPidUnspecified, "[]\n[]\n[set]\n"},
		{"a recorded zero, which is zsh", LastBackgroundPidZero, "[0]\n[0]\n[set]\n"},
		{"unset, which is bash, dash and ash", LastBackgroundPidUnset, "[]\n[unset]\n[]\n"},
		{"unset and quiet, which is ksh93", LastBackgroundPidUnsetButNotRefused, "[]\n[unset]\n[]\n"},
	} {
		t.Run(w.name, func(t *testing.T) {
			sem := permissive()
			sem.LastBackgroundPid = w.policy
			const src = `printf '[%s]\n' "$!" "${!-unset}" "${!+set}"`
			if got, st := run(t, src, withSem(sem)); got != w.want || st != 0 {
				t.Errorf("got %q/%d, want %q/0", got, st, w.want)
			}
		})
	}
}

// And whether `set -u` stops the script over it, which is the second question
// and the one the two unset answers part on.
//
// A bare `$!` cannot tell those two apart — both expand to nothing — so the
// only case that separates them is this one.
func TestWhetherNounsetRefusesTheLastBackgroundPidBeforeAnyJob(t *testing.T) {
	const src = `set -u; echo "[$!]"; echo after`

	refuses := permissive()
	refuses.LastBackgroundPid = LastBackgroundPidUnset
	got, st := run(t, src, withSem(refuses))
	if strings.Contains(got, "after") || st == 0 {
		t.Errorf("the refusing answer should have stopped the script, got %q/%d", got, st)
	}
	if !strings.Contains(got, "!") {
		t.Errorf("the refusal should name the parameter, got %q", got)
	}

	for _, w := range []struct {
		name   string
		policy LastBackgroundPidPolicy
		want   string
	}{
		{"unset and quiet", LastBackgroundPidUnsetButNotRefused, "[]\nafter\n"},
		{"a set zero", LastBackgroundPidZero, "[0]\nafter\n"},
		{"set and empty", LastBackgroundPidUnspecified, "[]\nafter\n"},
	} {
		t.Run(w.name, func(t *testing.T) {
			sem := permissive()
			sem.LastBackgroundPid = w.policy
			if got, st := run(t, src, withSem(sem)); got != w.want || st != 0 {
				t.Errorf("got %q/%d, want %q/0", got, st, w.want)
			}
		})
	}
}

// And once a job has been started the parameter is set under every answer,
// which is what says the whole question is about nothing having run rather
// than about `$!`.
//
// Measured that way too: with one job behind it, `set -u` has nothing to say
// about `$!` in any of the six columns.
func TestTheLastBackgroundPidIsSetOnceAJobHasRun(t *testing.T) {
	for _, policy := range []LastBackgroundPidPolicy{
		LastBackgroundPidUnspecified,
		LastBackgroundPidZero,
		LastBackgroundPidUnset,
		LastBackgroundPidUnsetButNotRefused,
	} {
		t.Run(policy.String(), func(t *testing.T) {
			sem := permissive()
			sem.LastBackgroundPid = policy
			src := `set -u; /bin/sleep 0 & wait; x=$!; echo "[${x:+yes}] after"`
			if got, st := run(t, src, withSem(sem)); got != "[yes] after\n" || st != 0 {
				t.Errorf("got %q/%d, want %q/0", got, st, "[yes] after\n")
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
	sem.LastBackgroundPid = LastBackgroundPidUnset

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
	sem.LastBackgroundPid = LastBackgroundPidUnset
	sem.IndirectionYieldsName = No
	enable := func(d *syntax.Dialect) { d.ParamIndirection = true }
	if got, st := runGrammar(t, `y=hello; x=y; echo "[${!x}]"`, enable, withSem(sem)); got != "[hello]\n" || st != 0 {
		t.Errorf("an indirection through a set name: got %q/%d, want %q/0", got, st, "[hello]\n")
	}
}
