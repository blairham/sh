// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"slices"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// TestDescribingAProducedTableDoesNotAskItToDescribeItself is the trap this
// accessor has and nothing else in the package does.
//
// The table a dialect builds out of these answers is itself a produced
// association, so it appears in its own name list. Asking `assocFor` what
// kind that name is *produces* the value — which runs the dialect's view,
// which asks again. Through the shell it hung, which is the worst way for it
// to be wrong: no error, no output, no end.
//
// The fix is to test the two tables for membership instead, and this is the
// test that says so. Restoring the loop fails here in about four seconds —
// as a stack overflow rather than as an assertion, so it is a crash and not
// a `--- FAIL` line — and the row also asserts the answer, so a fix that
// stopped the loop by reporting the wrong kind would still be caught.
func TestDescribingAProducedTableDoesNotAskItToDescribeItself(t *testing.T) {
	out, _ := runGrammar(t, `echo "[${self[self]}][${self[mine]}]"`, nil, func(rr *Runner) {
		rr.SetVar("mine", "1")
		rr.SetDynamicAssoc("self", func(inner *Runner) AssocArray {
			table := AssocArray{}
			for _, name := range inner.ParameterNames() {
				if a, ok := inner.ParameterAttributes(name); ok {
					table[name] = kindWord(a.Kind)
				}
			}
			return table
		})
	})
	const want = "[association][scalar]"
	if got := strings.TrimSpace(out); got != want {
		t.Errorf("the table describes itself as %q, want %q", got, want)
	}
}

// TestParameterKindPutsTheContainerFirst is the precedence, which is one kind
// per name rather than a set.
//
// Measured in zsh 5.9.2, the shell that publishes these: `typeset -ia ia;
// ia=(1 2)` describes as `array` and says nothing about integers, and the
// float spelling does the same. So a numeric attribute is only ever a
// scalar's, and a dialect switching on the kind cannot be handed two.
//
// The integer and float rows are dialect/zsh's rather than this file's,
// because the attribute arrives through `typeset -i` and the core has no
// declaration utility to put it on with. They are asserted there against the
// real shell, which is the stronger statement anyway.
func TestParameterKindPutsTheContainerFirst(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*Runner)
		want  ParameterKind
	}{
		{"a scalar", func(r *Runner) { r.SetVar("v", "1") }, ScalarParameter},
		{"an array", func(r *Runner) { r.SetArray("v", []string{"1"}) }, ArrayParameter},
		{"an association", func(r *Runner) { r.SetAssoc("v", map[string]string{"k": "x"}) }, AssocParameter},
		{
			name:  "a produced array",
			setup: func(r *Runner) { r.SetDynamicArray("v", func(*Runner) []string { return nil }) },
			want:  ArrayParameter,
		},
		{
			name:  "a produced association",
			setup: func(r *Runner) { r.SetDynamicAssoc("v", func(*Runner) AssocArray { return nil }) },
			want:  AssocParameter,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var r *Runner
			runGrammar(t, `:`, nil, func(rr *Runner) { r = rr; tc.setup(rr) })
			a, ok := r.ParameterAttributes("v")
			if !ok {
				t.Fatal("the name is not there")
			}
			if a.Kind != tc.want {
				t.Errorf("kind = %v, want %v", a.Kind, tc.want)
			}
		})
	}
}

// TestParameterNamesCoversTheProducedOnesToo is the difference from
// declarableNames, which is a *listing* and leaves produced parameters out on
// the grounds that a value made up on each read is not state a listing could
// carry. That is right for a listing and wrong for a report on what the shell
// has: `$funcstack` is as real to the script asking as `$PATH` is.
func TestParameterNamesCoversTheProducedOnesToo(t *testing.T) {
	var r *Runner
	runGrammar(t, `:`, nil, func(rr *Runner) {
		r = rr
		rr.SetVar("mine", "1")
		rr.SetDynamicArray("made", func(*Runner) []string { return nil })
		rr.SetDynamic("scalarmade", func(*Runner) string { return "" })
	})
	names := r.ParameterNames()
	for _, want := range []string{"mine", "made", "scalarmade"} {
		if !slices.Contains(names, want) {
			t.Errorf("%q is missing from the names", want)
		}
	}
	if slices.Contains(names, "nosuchvar") {
		t.Error("a name nothing made is in the names")
	}
	if !slices.IsSorted(names) {
		t.Error("the names are not sorted")
	}
	if _, ok := r.ParameterAttributes("nosuchvar"); ok {
		t.Error("a name nothing made reports attributes")
	}
}

func kindWord(k ParameterKind) string {
	switch k {
	case AssocParameter:
		return "association"
	case ArrayParameter:
		return "array"
	case IntegerParameter:
		return "integer"
	case FloatParameter:
		return "float"
	default:
		return "scalar"
	}
}

// ParameterIsNamed is the per-name half of ParameterNames, and the two have
// to agree for every name or a dialect's `${parameters[x]}` answers about a
// different shell from its `${(k)parameters}`.
//
// The union is over seven tables, so the failure to guard against is a
// helper that forgot one. **Each name is put into exactly one table, by
// hand**, and that is the whole design of this test: the obvious version —
// SetVar, SetArray, SetAssoc and ask about the result — cannot see a missing
// branch at all, because storeArray keeps a scalar view of every array, so
// the Vars check answers for a name the Arrays check was supposed to. It
// passed against a build with `r.Arrays` deleted from the union.
func TestTheTwoReadingsOfParameterNamesAgree(t *testing.T) {
	var r *Runner
	runGrammar(t, `:`, nil, func(rr *Runner) {
		r = rr
		rr.Env = append(rr.Env, "ONLYENV=fromenv")
		rr.SetAbsentParameter("refuser", "not implemented yet")
	})
	// Straight into one table each, past the setters, so that no name is
	// reachable through a second branch.
	r.Vars["onlyscalar"] = "1"
	r.Arrays = map[string]Array{"onlyarray": {0: "x"}}
	r.AssocArrays = map[string]AssocArray{"onlyassoc": {"k": "v"}}
	r.Dynamic = map[string]func(*Runner) string{"onlymade": func(*Runner) string { return "" }}
	r.DynamicArrays = map[string]func(*Runner) []string{"onlymadearray": func(*Runner) []string { return nil }}
	r.DynamicAssocs = map[string]func(*Runner) AssocArray{"onlymadeassoc": func(*Runner) AssocArray { return nil }}

	names := r.ParameterNames()
	// Every table is represented, or the loop below proves nothing about the
	// branch that table stands for.
	for _, want := range []string{
		"onlyscalar", "onlyarray", "onlyassoc", "ONLYENV",
		"onlymade", "onlymadearray", "onlymadeassoc",
	} {
		if !slices.Contains(names, want) {
			t.Fatalf("%q is not in ParameterNames — this test cannot check its branch", want)
		}
	}
	for _, name := range names {
		if !r.ParameterIsNamed(name) {
			t.Errorf("ParameterIsNamed(%q) = false, but the name is in the list", name)
		}
	}
	// And the other direction, over the shapes that could answer yes
	// wrongly. An *absent* parameter is the one that matters: it has
	// attributes and is deliberately not a name the listing yields, so a
	// predicate that asked ParameterAttributes instead would report a
	// parameter `${(k)parameters}` says is not there.
	for _, name := range []string{"refuser", "nosuchvar", ""} {
		if r.ParameterIsNamed(name) {
			t.Errorf("ParameterIsNamed(%q) = true, but it is not in the list", name)
		}
		if slices.Contains(names, name) {
			t.Errorf("%q is in the list, which this test assumed it was not", name)
		}
	}
}
