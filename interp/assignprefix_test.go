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

// An assignment prefixed to a builtin is in effect while the builtin runs and
// is taken back afterward. These pin both halves — the visibility that makes
// `IFS=: read x y` split, and the restore that keeps the prefix transient —
// and the special-builtin persistence axis in both directions.

func prefixAssignRun(t *testing.T, src string, sem Semantics) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem,
		Dir: t.TempDir(), Name: "testsh",
	})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v\nstderr: %s", src, rerr, errs.String())
	}
	return out.String()
}

// prefixAssignStreams is the same run with both streams handed back, for the
// rows whose subject is a diagnostic that was or was not written. A refusal of
// an unanswered axis goes to standard error and leaves standard output as it
// was, so a helper that returns only the first of them cannot tell an axis
// that was never asked from one that was asked and refused.
func prefixAssignStreams(t *testing.T, src string, sem Semantics) (string, string) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem,
		Dir: t.TempDir(), Name: "testsh",
	})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v\nstderr: %s", src, rerr, errs.String())
	}
	return out.String(), errs.String()
}

func TestAPrefixIsVisibleToTheBuiltinItPrefixes(t *testing.T) {
	sem := permissive()
	sem.LastPipelineElementInCurrentShell = Yes
	out := prefixAssignRun(t,
		`echo "a:b" | { IFS=: read x y; echo "[$x][$y]"; }`, sem)
	if !strings.Contains(out, "[a][b]") {
		t.Errorf("output = %q, want the read split on the prefixed IFS", out)
	}
}

func TestAPrefixOnABuiltinIsTakenBackAfterward(t *testing.T) {
	sem := permissive()
	sem.LastPipelineElementInCurrentShell = Yes
	// The probe stays in the same group as the read: the restore is what
	// makes the later unquoted expansion split on whitespace again.
	out := prefixAssignRun(t,
		`echo "a:b" | { IFS=: read x y; v="p q"; set -- $v; echo "n=$#"; }`, sem)
	if !strings.Contains(out, "n=2") {
		t.Errorf("output = %q, want IFS back to whitespace after the read", out)
	}
}

func TestAPrefixedNameThatWasUnsetIsUnsetAgainAfterward(t *testing.T) {
	out := prefixAssignRun(t, `unset v; v=1 read -r _ignored </dev/null; echo "${v-absent}"`,
		permissive())
	if !strings.Contains(out, "absent") {
		t.Errorf("output = %q, want the name unset again after the builtin", out)
	}
}

func TestAPrefixOnASpecialBuiltinFollowsTheAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"persists when the dialect says so", Yes, "[2]"},
		{"is taken back when it says not", No, "[1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := permissive()
			sem.AssignmentPrefixPersistsOnSpecialBuiltin = tc.answer
			out := prefixAssignRun(t, `x=1; x=2 export y=3; echo "[$x]"`, sem)
			if !strings.Contains(out, tc.want) {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
		})
	}
}

// An append written as a prefix joins the name's current value rather than
// replacing it, and still leaves the shell's own value alone afterwards.
//
// Unanimous across the panel, and the reason it is worth pinning separately
// from the row above: the operator is on the assignment either way, so a
// prefix route that expands the value and stores it has already lost the
// append without failing anything. The command sees the tail alone, which for
// the idiom this construct exists for — `PATH+=:/x cmd` — is a PATH with one
// entry in it.
func TestAnAppendPrefixJoinsTheValueThatIsThere(t *testing.T) {
	sem := permissive()
	// Taken back afterwards, so the restore and the join are one row: an
	// implementation that stored the joined value on the shell would pass
	// the first half and fail the second.
	sem.AssignmentPrefixPersistsOnSpecialBuiltin = No
	got := prefixAssignRun(t, `v=1; v+=4; v+=5 eval 'echo "[$v]"'; echo "after=[$v]"`, sem)
	if want := "[145]\nafter=[14]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// And an append in front of a name holding nothing is the value alone, which
// is what says the join reads the name rather than assuming one is there.
func TestAnAppendPrefixOverAnUnsetNameIsTheValue(t *testing.T) {
	sem := permissive()
	sem.AssignmentPrefixPersistsOnSpecialBuiltin = No
	got := prefixAssignRun(t, `unset v; v+=5 eval 'echo "[$v]"'`, sem)
	if want := "[5]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A prefix in front of a **function** call, which is the third kind and is
// the one that used to be discarded outright: the value was expanded on that
// route and never applied, so `v=1; v=9 f` showed the body `1` where all
// seven panel columns show it `9` (#2407).

func TestAPrefixIsVisibleInsideTheFunctionItPrefixes(t *testing.T) {
	// Core, not an axis: unanimous across dash, bash 5.3, bash-as-sh, bash
	// 3.2, ksh93u+, zsh 5.9.2 and BusyBox ash. Both answers of both axes are
	// set, so this asserts the visibility rather than one dialect's reading
	// of what happens afterwards.
	for _, persists := range []Answer{Yes, No} {
		for _, exported := range []Answer{Yes, No} {
			sem := permissive()
			sem.AssignmentPrefixPersistsAfterAFunction = persists
			sem.PrefixToAFunctionIsExported = exported
			got := prefixAssignRun(t, `f(){ echo "[$v]"; }; v=1; v=9 f`, sem)
			if want := "[9]\n"; got != want {
				t.Errorf("persists=%v exported=%v: got %q, want %q", persists, exported, got, want)
			}
		}
	}
}

func TestAPrefixOnAFunctionFollowsThePersistenceAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"persists when the dialect says so", Yes, "[9]\nafter=[9]\n"},
		{"is taken back when it says not", No, "[9]\nafter=[1]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := permissive()
			sem.AssignmentPrefixPersistsAfterAFunction = tc.answer
			got := prefixAssignRun(t, `f(){ echo "[$v]"; }; v=1; v=9 f; echo "after=[$v]"`, sem)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// The same axis decides what a body that *assigned over* the name leaves
// behind, which is the half a row using an untouched name cannot see: under
// the keeping answer the caller reads what the body wrote, not the prefix.
func TestThePersistenceAxisKeepsWhateverTheBodyLeft(t *testing.T) {
	src := `f(){ v=inner; }; v=1; v=9 f; echo "after=[$v]"`
	sem := permissive()
	sem.AssignmentPrefixPersistsAfterAFunction = Yes
	if got := prefixAssignRun(t, src, sem); got != "after=[inner]\n" {
		t.Errorf("Yes side: got %q, want the body's value", got)
	}
	sem.AssignmentPrefixPersistsAfterAFunction = No
	if got := prefixAssignRun(t, src, sem); got != "after=[1]\n" {
		t.Errorf("No side: got %q, want the caller's value back", got)
	}
}

// A name the prefix invented is gone again under the taking-back answer,
// rather than left holding an empty string — which is the shape `${v-absent}`
// is the only probe for.
func TestAPrefixedNameAFunctionInventedIsUnsetAgain(t *testing.T) {
	sem := permissive()
	sem.AssignmentPrefixPersistsAfterAFunction = No
	got := prefixAssignRun(t, `unset v; f(){ :; }; v=9 f; echo "[${v-absent}]"`, sem)
	if want := "[absent]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAPrefixOnAFunctionFollowsTheExportAxis(t *testing.T) {
	// `export -p` rather than a child, because the substrate must not start
	// one to answer a question about an attribute — and matched with `case`
	// rather than `grep`, so the probe starts no program either. The listing
	// is the same evidence: every column that hands the name to a child
	// lists it here. The name is a rare one because the listing is the whole
	// environment, and a pattern of one letter would match somebody else's.
	src := `f(){ case "$(export -p)" in *zqp*) echo YES;; *) echo NO;; esac; }; zqp=1; zqp=9 f`
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"exported for the call when the dialect says so", Yes, "YES\n"},
		{"an ordinary variable when it says not", No, "NO\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := permissive()
			sem.AssignmentPrefixPersistsAfterAFunction = No
			sem.PrefixToAFunctionIsExported = tc.answer
			if got := prefixAssignRun(t, src, sem); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// And the attribute goes back with the value: a name exported only for the
// length of a call is told to no child on the next line.
func TestAnExportedPrefixLosesTheAttributeWithTheCall(t *testing.T) {
	sem := permissive()
	sem.AssignmentPrefixPersistsAfterAFunction = No
	sem.PrefixToAFunctionIsExported = Yes
	got := prefixAssignRun(t,
		`f(){ :; }; zqp=1; zqp=9 f; case "$(export -p)" in *zqp*) echo YES;; *) echo NO;; esac`, sem)
	if want := "NO\n"; got != want {
		t.Errorf("got %q, want %q — the export attribute outlived the call", got, want)
	}
}

// The export answer moves the attribute in *both* directions rather than one
// reading leaving it alone: where the prefix is a plain assignment to this
// shell, a name that was exported before the call loses the attribute for it.
// Measured — `export v=1; f(){ typeset -p v; }; v=9 f` prints a plain `v=9` in
// ksh93 and `declare -x v="9"` in bash, and a child the body starts is told
// `v=9` in six columns and nothing in ksh93.
func TestAPrefixOnAFunctionTakesTheAttributeOffWhereTheDialectSaysSo(t *testing.T) {
	sem := permissive()
	sem.AssignmentPrefixPersistsAfterAFunction = No
	sem.PrefixToAFunctionIsExported = No
	src := `export zqp=1; f(){ case "$(export -p)" in *zqp*) echo YES;; *) echo NO;; esac; }; zqp=9 f`
	if got, want := prefixAssignRun(t, src, sem), "NO\n"; got != want {
		t.Errorf("got %q, want %q — the attribute was left on for the call", got, want)
	}
}

// And the removal is given back with everything else, so a name the caller
// exported is still exported on the next line.
func TestAnAttributeTakenOffForACallComesBackWithIt(t *testing.T) {
	sem := permissive()
	sem.AssignmentPrefixPersistsAfterAFunction = No
	sem.PrefixToAFunctionIsExported = No
	got := prefixAssignRun(t,
		`export zqp=1; f(){ :; }; zqp=9 f; case "$(export -p)" in *zqp*) echo YES;; *) echo NO;; esac`, sem)
	if want := "YES\n"; got != want {
		t.Errorf("got %q, want %q — the caller's export attribute did not come back", got, want)
	}
}

// The two answers ksh93 gives together, which is the combination the whole
// panel's holdout column is: the name keeps the prefix's value afterwards and
// is exported to nobody, before or after.
func TestTheKeepingAndUnexportingAnswersCompose(t *testing.T) {
	sem := permissive()
	sem.AssignmentPrefixPersistsAfterAFunction = Yes
	sem.PrefixToAFunctionIsExported = No
	got := prefixAssignRun(t,
		`export zqp=1; f(){ :; }; zqp=9 f; echo "[$zqp]"; case "$(export -p)" in *zqp*) echo YES;; *) echo NO;; esac`, sem)
	if want := "[9]\nNO\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The persistence axis is not asked where the two readings land in the same
// place, which is what keeps an unanswered vector off an ordinary line: a
// prefix whose value is what the name already holds changes nothing either
// way. Read off **stderr**, because a refusal writes there and leaves the
// line's output alone — a probe that reads standard output alone cannot tell
// an axis that was not asked from one that was asked and refused.
func TestThePersistenceAxisIsNotAskedWhereTheReadingsAgree(t *testing.T) {
	// One case per export answer, because the attribute is part of what
	// giving the name back would put back. Under the un-exporting answer a
	// name nobody exported is where the two readings meet; under the
	// exporting one it is a name the caller had already exported.
	for _, tc := range []struct {
		exported Answer
		src      string
	}{
		{No, `f(){ echo "[$v]"; }; v=9; v=9 f`},
		{Yes, `f(){ echo "[$v]"; }; export v=9; v=9 f`},
	} {
		sem := permissive()
		sem.AssignmentPrefixPersistsAfterAFunction = Unspecified
		sem.PrefixToAFunctionIsExported = tc.exported
		out, errs := prefixAssignStreams(t, tc.src, sem)
		if out != "[9]\n" {
			t.Errorf("exported=%v: stdout = %q, want %q", tc.exported, out, "[9]\n")
		}
		if errs != "" {
			t.Errorf("exported=%v: asked an axis it did not need: %q", tc.exported, errs)
		}
	}
}

// The export axis has no such case, and that is the correction rather than an
// omission. An earlier reading skipped it for a name that was exported
// already, on the premise that such a name reaches every child under either
// answer; the un-exporting column above is what that premise is false in, so
// the axis is asked on every name a prefix stands in front of.
func TestTheExportAxisIsAskedEvenForANameAlreadyExported(t *testing.T) {
	sem := permissive()
	sem.AssignmentPrefixPersistsAfterAFunction = No
	sem.PrefixToAFunctionIsExported = Unspecified
	_, errs := prefixAssignStreams(t, `export v=9; f(){ :; }; v=9 f`, sem)
	if !strings.Contains(errs, "exported for the call") {
		t.Errorf("stderr = %q, want the unanswered export axis named", errs)
	}
}

// And a prefix in front of `command` reaches the command it names, because
// `command` is a precommand word rather than a command of its own.
//
// Unanimous, so it is not an axis: `v=1; v=9 command env` shows the child
// `v=9` in bash 5.3, ksh93u+, zsh 5.9.2 and dash 0.5.12, measured
// 2026-09-12, where this shell showed it nothing (#2408). Setting the name
// is not enough — a child is handed the exported names — so the prefix has
// to carry the export attribute for the length of the command.
func TestAPrefixThroughCommandReachesTheChild(t *testing.T) {
	out, st := run(t,
		`v=1; v=9 command /usr/bin/env | grep '^v=' || echo "(none)"`, nil)
	if st != 0 || out != "v=9\n" {
		t.Errorf("got %q status %d, want the child shown v=9", out, st)
	}
}

// Written twice over, which is the arrangement that distinguishes carrying
// the attribute for the command from handing one child a one-off environment:
// the inner `command` has no prefix of its own to hand on.
func TestAPrefixThroughTwoCommandWordsStillReachesTheChild(t *testing.T) {
	out, st := run(t,
		`v=1; v=9 command command /usr/bin/env | grep '^v=' || echo "(none)"`, nil)
	if st != 0 || out != "v=9\n" {
		t.Errorf("got %q status %d, want the child shown v=9", out, st)
	}
}

// The attribute is the command's and not the shell's afterward. All four
// columns leave `v` holding `1` and leave it unexported, so a later child
// sees nothing — which is what savedVar's export tri-state is for: the name
// goes back to never having been spoken about rather than to a recorded
// `false`.
func TestAPrefixThroughCommandIsNotExportedAfterward(t *testing.T) {
	out, st := run(t,
		`v=1; v=9 command true; echo "read=[$v]"; `+
			`/usr/bin/env | grep '^v=' || echo "(none)"`, nil)
	if st != 0 || out != "read=[1]\n(none)\n" {
		t.Errorf("got %q status %d, want the prefix taken back with the attribute", out, st)
	}
}

// A name the shell had exported keeps its own value for the later child, and
// the prefix reaches only the one command. Measured identically in all four.
func TestAPrefixThroughCommandLeavesAnExportedNameAsItWas(t *testing.T) {
	out, st := run(t,
		`export v=1; v=9 command /usr/bin/env | grep '^v='; `+
			`/usr/bin/env | grep '^v='`, nil)
	if st != 0 || out != "v=9\nv=1\n" {
		t.Errorf("got %q status %d, want v=9 to the command and v=1 after", out, st)
	}
}
