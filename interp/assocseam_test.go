// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Three things a registered builtin needs and could not reach: a table of
// *named* values, the same read back, and the value of an expression it did
// not parse. Each is here because a dialect builtin found it missing —
// `zparseopts` writes an association and `zformat` compares an expression
// with a number — and each is about something this package owns: the
// associative attribute decides how a subscript is read, and the arithmetic
// is the shell's own.

func seamRunner(t *testing.T, out, errs *strings.Builder) *Runner {
	t.Helper()
	dg := Diagnostics{Location: LocationTightLine, BuiltinLocation: LocationTightLine}
	// One axis answered, because one of these tests reaches a *fatal* error
	// and the status a fatal error carries is a thing the shells disagree
	// about — an unanswered axis reports itself, which would be a second
	// diagnostic on a test asserting on the first.
	sem := Semantics{FatalErrorStatusIsOne: Yes}
	return newTestRunner(t, &Runner{
		Stdout: out, Stderr: errs, Diagnostics: &dg, Semantics: &sem, Name: "testsh",
	})
}

func runSeam(t *testing.T, r *Runner, src string) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
}

// An **empty** table still declares the name, which is the case a "nothing to
// write, so write nothing" shortcut gets wrong. An association that exists and
// is empty and one that was never created are different things — the second is
// not an array at all — and a builtin whose parse matched nothing has to leave
// the first.
func TestSetAssocDeclaresTheNameEvenWithNothingInIt(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.Register("fill", func(rr *Runner, _ context.Context, args []string) int {
		rr.SetAssoc(args[0], nil)
		return 0
	})
	runSeam(t, r, "fill empty\n")
	got, ok := r.GetAssoc("empty")
	if !ok {
		t.Fatal("GetAssoc(empty) = not there, want a declared empty association")
	}
	if len(got) != 0 {
		t.Errorf("GetAssoc(empty) = %v, want no elements", got)
	}
	if _, ok := r.GetAssoc("never"); ok {
		t.Error("GetAssoc(never) = there, want not there")
	}
}

// The subscript's meaning is what the attribute decides, so a table written
// through this seam has to read back by *key* and not by index: `m[1+1]` is
// the three characters here and would be the element at 2 in an indexed array.
// This is the assertion that a seam reaching into the map directly would fail.
func TestSetAssocGivesTheNameTheAssociativeAttribute(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.Register("fill", func(rr *Runner, _ context.Context, _ []string) int {
		rr.SetAssoc("m", map[string]string{"1+1": "key", "2": "index"})
		return 0
	})
	runSeam(t, r, "fill\nprintf '[%s]' \"${m[1+1]}\"\n")
	if out.String() != "[key]" {
		t.Errorf("${m[1+1]} = %q, want %q", out.String(), "[key]")
	}
}

// GetAssoc hands back a copy. A caller that reads a table, adds to it and
// writes it back — which is exactly what a builtin preserving existing
// elements does — must not change the shell's state halfway through deciding
// what to write, because a failure after that point would leave half a write
// behind.
func TestGetAssocIsACopyAndNotTheShellsOwnTable(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.Register("meddle", func(rr *Runner, _ context.Context, _ []string) int {
		table, _ := rr.GetAssoc("m")
		table["added"] = "no"
		delete(table, "kept")
		return 0
	})
	runSeam(t, r, "typeset -A m\nm[kept]=yes\nmeddle\nprintf '[%s][%s]' \"${m[kept]}\" \"${m[added]}\"\n")
	if out.String() != "[yes][]" {
		t.Errorf("the shell's table after meddling = %q, want %q", out.String(), "[yes][]")
	}
}

// ArithValue is the shell's own arithmetic and not a number parse: `1+1` is 2
// and a bare name is the variable's value, an unset one being zero.
func TestArithValueIsTheShellsOwnArithmetic(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.Register("value", func(rr *Runner, _ context.Context, args []string) int {
		n, ok := rr.ArithValue(args[0])
		_, _ = out.WriteString(strconv.Itoa(n) + ":" + boolText(ok) + " ")
		return 0
	})
	runSeam(t, r, "x=7\nvalue 1+1\nvalue x\nvalue nosuchname\nvalue '(1+2)*2'\n")
	want := "2:ok 7:ok 0:ok 6:ok "
	if out.String() != want {
		t.Errorf("ArithValue = %q, want %q", out.String(), want)
	}
}

// And an expression that will not *evaluate* is not a zero. It is reported the
// way `$(( ))` reports one — located as the shell rather than as the builtin,
// because the expression failed and not the command holding it — and it stops
// the script, so nothing after it runs and the caller is told before it writes
// anything.
func TestArithValueReportsAndStopsWhenAnExpressionWillNotEvaluate(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.Register("value", func(rr *Runner, _ context.Context, args []string) int {
		if _, ok := rr.ArithValue(args[0]); !ok {
			_, _ = out.WriteString("refused ")
			return 1
		}
		_, _ = out.WriteString("took ")
		return 0
	})
	runSeam(t, r, "value 1/0\nvalue 1+1\n")
	if out.String() != "refused " {
		t.Errorf("what ran = %q, want %q — the second line must not run", out.String(), "refused ")
	}
	if errs.String() != "testsh:1: division by zero\n" {
		t.Errorf("diagnostic = %q, want %q", errs.String(), "testsh:1: division by zero\n")
	}
}

func boolText(b bool) string {
	if b {
		return "ok"
	}
	return "no"
}

// A produced association is a **view**: the producer is asked at the moment
// the parameter is read, so three reads around two mutations give three
// answers. A snapshot taken at any single instant cannot satisfy this, which
// is the point of writing it as read-mutate-read rather than as
// mutate-then-read (#1060).
func TestAProducedAssociationIsAViewAndNotASnapshot(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	table := map[string]string{"a": "1"}
	r.SetDynamicAssoc("view", func(*Runner) AssocArray {
		copied := make(AssocArray, len(table))
		for k, v := range table {
			copied[k] = v
		}
		return copied
	})
	r.Register("mutate", func(*Runner, context.Context, []string) int {
		table["b"] = "2"
		delete(table, "a")
		return 0
	})
	runSeam(t, r, `printf '[%s][%s]' "${view[a]}" "${view[b]}"
mutate
printf '[%s][%s]' "${view[a]}" "${view[b]}"
mutate
printf '[%s][%s]' "${view[a]}" "${view[b]}"`)
	// The producer's answer moves under the reads. A table filled in once
	// would print the first pair three times.
	want := "[1][][][2][][2]"
	if out.String() != want {
		t.Errorf("three reads around two mutations = %q, want %q", out.String(), want)
	}
}

// And it cannot be turned back into a snapshot by writing to it. A stored
// table is what a read finds first, so an assignment that landed in one would
// shadow the producer for good — with nothing about the name's shape changing
// and no diagnostic written. The writer is where an assignment goes instead.
func TestAWriteToAProducedAssociationCannotShadowIt(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	table := map[string]string{"a": "1"}
	r.SetDynamicAssoc("view", func(*Runner) AssocArray {
		copied := make(AssocArray, len(table))
		for k, v := range table {
			copied[k] = v
		}
		return copied
	})
	r.SetDynamicAssocWriter("view", func(_ *Runner, key, value string, set bool) {
		if !set {
			delete(table, key)
			return
		}
		// Deliberately not what it was given, so the test can tell a write
		// that went through the hook from one that went into a stored table.
		table[key] = "through:" + value
	})
	// Assignment only: `unset m[k]` reaches the same hook, but which
	// subscript spellings `unset` reads at all is a dialect's answer and this
	// runner has chosen no shell. The removing half is asserted where a
	// dialect has answered it — see dialect/zsh's `unset "functions[h]"`.
	runSeam(t, r, `view[b]=2
printf '[%s][%s]' "${view[a]}" "${view[b]}"
view[a]=9
printf '[%s][%s]' "${view[a]}" "${view[b]}"`)
	want := "[1][through:2][through:9][through:2]"
	if out.String() != want {
		t.Errorf("writes through a produced association = %q, want %q", out.String(), want)
	}
}

// Declaring the name is not a way in either: `typeset -A` on a produced
// association would otherwise put an empty stored table in front of the
// producer, which is the same shadowing by a route that writes no element at
// all.
func TestDeclaringAProducedAssociationDoesNotShadowIt(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.SetDynamicAssoc("view", func(*Runner) AssocArray {
		return AssocArray{"a": "1"}
	})
	runSeam(t, r, "typeset -A view\nprintf '[%s]' \"${view[a]}\"\n")
	if out.String() != "[1]" {
		t.Errorf("after `typeset -A` = %q, want %q", out.String(), "[1]")
	}
}

// CommandsOnPath is the listing half of the lookup, so it follows the same
// rules: the **first** PATH entry holding a name is the one it resolves to,
// and a file that could not be run is not a command at all.
func TestCommandsOnPathFollowsTheLookupsOwnRules(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	// The same name in both, so the answer says which entry won.
	for _, dir := range []string{first, second} {
		if err := os.WriteFile(filepath.Join(dir, "both"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// And one that is there but cannot be run.
	if err := os.WriteFile(filepath.Join(second, "notexec"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.Register("report", func(rr *Runner, _ context.Context, _ []string) int {
		found := rr.CommandsOnPath()
		_, _ = out.WriteString("both=" + found["both"] + " ")
		if _, listed := found["notexec"]; listed {
			_, _ = out.WriteString("notexec=listed")
			return 0
		}
		_, _ = out.WriteString("notexec=absent")
		return 0
	})
	r.Vars = map[string]string{"PATH": first + ":" + second}
	runSeam(t, r, "report\n")
	want := "both=" + filepath.Join(first, "both") + " notexec=absent"
	if out.String() != want {
		t.Errorf("CommandsOnPath = %q, want %q", out.String(), want)
	}
}

// AliasTable hands back a copy, for the reason GetAssoc's does: a caller
// reading the table, changing it and writing it back must not move the shell's
// state halfway through deciding what to write.
func TestAliasTableIsACopy(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.Register("meddle", func(rr *Runner, _ context.Context, _ []string) int {
		table := rr.AliasTable()
		table["added"] = "no"
		delete(table, "kept")
		return 0
	})
	r.SetAlias("kept", "yes")
	runSeam(t, r, "meddle\n")
	if v, ok := r.LookupAlias("kept"); !ok || v != "yes" {
		t.Errorf("alias kept = %q (%v), want %q", v, ok, "yes")
	}
	if _, ok := r.LookupAlias("added"); ok {
		t.Error("meddling with the copy added an alias to the shell")
	}
}

// A producer with nothing to say still gives a table. An empty association and
// an absent one are different things — `${#m}` is 0 for both, but the second
// is not an array at all, so `${m[k]:-d}` and a declaration listing answer
// differently — and a producer that has simply not been given anything yet has
// not stopped existing.
func TestAProducerWithNothingToSayStillHasATable(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.SetDynamicAssoc("empty", func(*Runner) AssocArray { return nil })
	// The subscript is what tells the two apart. With the attribute on,
	// `a b` is a key and reads as nothing; without it, the same brackets are
	// an arithmetic expression and `a b` will not evaluate. So this asserts
	// on the *stderr* as much as on the value.
	runSeam(t, r, "printf '[%s][%s]' \"${empty[k]:-DEFAULT}\" \"${empty[a b]}\"\n")
	if out.String() != "[DEFAULT][]" || errs.String() != "" {
		t.Errorf("a nil producer = %q (stderr %q), want %q and silence",
			out.String(), errs.String(), "[DEFAULT][]")
	}
	if _, ok := r.GetAssoc("empty"); ok {
		t.Error("GetAssoc found a stored table, which a produced name must not have")
	}
}

// RemoveFunction reports whether the *script* had a function of that name, and
// a prelude's is not the script's to remove: false, and the name still works
// afterwards. That is the same answer `unset -f` gives it (#1082), reached
// through the same predicate.
func TestRemoveFunctionWillNotTakeAPreludeFunction(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.SourcingPrelude(true)
	runSeam(t, r, "shellsown() { echo shell; }\n")
	r.SourcingPrelude(false)
	runSeam(t, r, "scripts() { echo script; }\n")
	if r.RemoveFunction("shellsown") {
		t.Error("RemoveFunction took a prelude function, which is not the script's to remove")
	}
	if !r.RemoveFunction("scripts") {
		t.Error("RemoveFunction refused the script's own function")
	}
	if r.RemoveFunction("neverdefined") {
		t.Error("RemoveFunction claimed to remove a name nothing defined")
	}
	runSeam(t, r, "shellsown\n")
	if out.String() != "shell\n" {
		t.Errorf("after the refused removal = %q, want the prelude's function still callable", out.String())
	}
}
