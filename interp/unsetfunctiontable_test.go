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

// A plain `unset NAME` — no `-f`, no `-v` — and whether it reaches the
// *function* table when the name holds no parameter.
//
// One column against six. Measured 2026-09-16 with each script ending in a
// call whose status is printed, both streams captured separately and a marker
// after it so the file says whether the script continued: bash 5.3.20, the
// same binary under argv[0] `sh` and bash 3.2.57 all answer 127 to the call
// after `unset b`, while zsh 5.9.2, ksh93u+ 2012-08-01, dash 0.5.12 and
// BusyBox ash 1.37.0 all run the function and answer 0. Every column
// continued, so the status is the whole of the answer.
//
// Semantics.UnsetReachesTheFunctionTable, and it is asked at the
// disagreement: only where the name has a function and holds no parameter.
func TestPlainUnsetReachingTheFunctionTable(t *testing.T) {
	const fn = "f() { echo ran; }\n"
	for _, c := range []struct {
		name    string
		reaches Answer
		src     string
		ran     bool
	}{
		{"reaching it", Yes, fn + "unset f\nf\n", false},
		{"not reaching it", No, fn + "unset f\nf\n", true},

		// The parameter table first, one table per call: with a variable of
		// the same name the first `unset` takes the variable and the
		// function is still there.
		{"the variable takes the first turn", Yes, fn + "f=v\nunset f\nf\n", true},
		{"and the function the second", Yes, fn + "f=v\nunset f\nunset f\nf\n", false},

		// `-v` names the parameter namespace and reaches no function in any
		// column: `f() { :; }; unset -v f` leaves `f` callable in bash,
		// where the plain spelling does not.
		{"-v reaches no function", Yes, fn + "unset -v f\nf\n", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := unsetTableRun(t, c.src, c.reaches)
			if got := strings.Contains(out, "ran"); got != c.ran {
				t.Errorf("said %q, want the function %s",
					out, map[bool]string{true: "still there", false: "gone"}[c.ran])
			}
		})
	}
}

// The ask is narrow, and this is the half that says so. A preset with no
// answer runs every `unset` that is not the disagreement — which is nearly
// all of them — and refuses only the one shape the panel splits over.
func TestAnUnansweredPresetOnlyRefusesTheDisagreement(t *testing.T) {
	for _, c := range []struct {
		name    string
		src     string
		refuses bool
	}{
		{"an ordinary parameter", "v=1\nunset v\necho \"[${v-gone}]\"\n", false},
		{"a name holding nothing at all", "unset nosuch\necho done\n", false},
		{"`unset -f` over a function", "f() { :; }\nunset -f f\necho done\n", false},
		{"`unset -v` over a function", "f() { :; }\nunset -v f\necho done\n", false},
		// A function and no parameter: the one shape the columns disagree
		// about, and the only one an unanswered preset may not run.
		{"a function and no parameter", "f() { :; }\nunset f\necho done\n", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := unsetTableRun(t, c.src, 0)
			refused := strings.Contains(out, "no dialect was chosen")
			if refused != c.refuses {
				t.Errorf("said %q, want refused=%v", out, c.refuses)
			}
		})
	}
}

// A name this shell provides is not the script's to remove. `unset pushd`
// reaches a prelude function standing in for a builtin, and a builtin is not
// what a plain `unset` removes in any column.
func TestAPreludeFunctionIsNotRemovedByAPlainUnset(t *testing.T) {
	out, st := unsetTableRun(t, "unset pushd\necho \"st=$?\"\ntype pushd\n", 0)
	if st == 2 && strings.Contains(out, "unset") {
		t.Errorf("said %q at status %d, want no question asked about a name the shell provides", out, st)
	}
}

func unsetTableRun(t *testing.T, src string, reaches Answer) (string, int) {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.UnsetReachesTheFunctionTable = reaches
	sem.UnsetOptions = "vf"
	sem.LoneDashIsAnOption = No
	sem.FunctionAttributeLetters = "rx"
	sem.ReadonlyOptions = "fp"
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
	})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return buf.String(), st
}
