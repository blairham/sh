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

// A produced association may answer only *some* of the keys its name is asked
// for, and the ones it has no answer for refuse by name rather than reading
// empty.
//
// The parameter half of this rule is SetAbsentParameter and the shape is the
// same one level down, for a reason that is about the caller and not about how
// far along an implementation is: the thing being viewed can legitimately have
// nothing under a key, so an empty string is a plausible answer to a different
// question and reaches a script as data. A name is refused for having no
// answer; there is no way for the caller to read that as "the thing has
// nothing there".
//
// Asserted with a read *after* the refusal, because the refusal is fatal to
// the expansion and the command it was for: a mechanism that wrote the
// sentence and carried on would print the second `printf` and pass a test
// that only compared the diagnostic.
func TestAKeyAProducedAssociationDoesNotAnswerRefusesByName(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.SetDynamicAssoc("partial", func(*Runner) AssocArray { return AssocArray{"here": "yes"} })
	r.SetAbsentElements("partial", "no answer for that one")
	runSeam(t, r, `printf '[%s]' "${partial[here]}"
printf '[%s]' "${partial[nowhere]}"
printf '[%s]' "after"`)
	if got, want := out.String(), "[yes]"; got != want {
		t.Errorf("output = %q, want %q — the refused expansion's command must not run, and neither must the one after it", got, want)
	}
	if got, want := errs.String(), "testsh:2: partial[nowhere]: no answer for that one\n"; got != want {
		t.Errorf("diagnostic = %q, want %q", got, want)
	}
}

// The three exemptions, and each is a question rather than a read.
//
// The four conditional operators are a script saying what to use when the key
// is not there, and the set test asks whether it is there at all — so all five
// are answered, at status 0, with nothing written. A guard that stopped the
// shell would be worse than no guard: every caller that checks before reading
// would stop at the check.
func TestAskingAboutAKeyAProducedAssociationLacksIsAnswered(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.SetDynamicAssoc("partial", func(*Runner) AssocArray { return AssocArray{"here": "yes"} })
	r.SetAbsentElements("partial", "no answer for that one")
	// The set test needs the grammar that has it, which is one dialect's —
	// named as a flag rather than as a shell, which is the rule for a test in
	// this package.
	d := syntax.Core()
	d.ParamSetTestFlag = true
	f, err := syntax.Parse(`printf '[%s][%s][%s][%s][%s][%s]' "${partial[nowhere]-d}" "${partial[nowhere]:-d}" `+
		`"${partial[nowhere]+p}" "${partial[here]+p}" "${+partial[nowhere]}" "${+partial[here]}"`+"\n", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := out.String(), "[d][d][][p][0][1]"; got != want {
		t.Errorf("asking about a key = %q, want %q", got, want)
	}
	if errs.String() != "" {
		t.Errorf("asking about a key wrote %q, want silence", errs.String())
	}
}

// The whole table is neither refused nor emptied by the rule: no key was
// named, so there is no key to refuse.
//
// Three spellings, because the whole-array subscript is decided on the
// subscript as written and the refusal is placed where a *key* is known — so
// each of these could have reached it with `@`, `*` or the empty string
// standing in for one.
func TestTheWholeOfAPartialAssociationIsStillReadable(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.SetDynamicAssoc("partial", func(*Runner) AssocArray { return AssocArray{"here": "yes"} })
	r.SetAbsentElements("partial", "no answer for that one")
	runSeam(t, r, `printf '[%s]' "${partial[@]}" "${partial[*]}" "${#partial[@]}"`+"\n")
	if got, want := out.String(), "[yes][yes][1]"; got != want {
		t.Errorf("the whole table = %q, want %q", got, want)
	}
	if errs.String() != "" {
		t.Errorf("reading the whole table wrote %q, want silence", errs.String())
	}
}

// A name with no absent-elements reason registered is untouched by any of
// this: a key it has no value for reads as nothing, at status 0, and the
// command runs.
//
// The control this pair of mechanisms needs. Every other test here registers
// the reason, so a refusal that fired for *every* produced association would
// pass all of them.
func TestAProducedAssociationWithoutTheReasonStillReadsEmpty(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.SetDynamicAssoc("plain", func(*Runner) AssocArray { return AssocArray{"here": "yes"} })
	runSeam(t, r, `printf '[%s]' "${plain[nowhere]}"; printf '[%s]' "after"`+"\n")
	if got, want := out.String(), "[][after]"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if errs.String() != "" {
		t.Errorf("a produced association with no reason registered wrote %q, want silence", errs.String())
	}
	if reason, ok := r.AbsentElements("plain"); ok {
		t.Errorf("AbsentElements(plain) = %q, true; want nothing registered", reason)
	}
}

// The one-key reading of a produced association, which is the second half of
// the seam above and exists because the first half is the wrong unit for a
// lookup. A parameter whose values have to be *made* — rendered, searched
// for, computed — pays for every key it holds on a read that wanted one, and
// on a real startup that was the difference between a shell that starts and
// one that appears to hang. See SetDynamicAssocElement.

// A single-key read takes the keyed route, and the whole-table producer is
// not run at all. The counter is the assertion: an answer that happened to be
// right would not say which of the two produced it.
func TestAKeyedProducerAnswersWithoutBuildingTheTable(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	whole := 0
	r.SetDynamicAssoc("view", func(*Runner) AssocArray {
		whole++
		return AssocArray{"a": "1", "b": "2"}
	})
	r.SetDynamicAssocElement("view", func(_ *Runner, key string) (string, bool) {
		v, ok := AssocArray{"a": "1", "b": "2"}[key]
		return v, ok
	})
	runSeam(t, r, `printf '[%s][%s][%s]' "${view[a]}" "${view[b]}" "${view[nope]}"`)
	if got, want := out.String(), "[1][2][]"; got != want {
		t.Errorf("three keyed reads = %q, want %q", got, want)
	}
	if whole != 0 {
		t.Errorf("whole-table producer ran %d times, want 0 — the keyed route was not taken", whole)
	}
}

// And the two readings have to agree about *absence* as much as about value,
// because which one a read gets is what decides whether the default applies.
// A keyed producer saying not-found is the table having no such key.
func TestAKeyedProducerSayingNotFoundIsAnAbsentElement(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.SetDynamicAssoc("view", func(*Runner) AssocArray { return AssocArray{"here": ""} })
	r.SetDynamicAssocElement("view", func(_ *Runner, key string) (string, bool) {
		return "", key == "here"
	})
	// `here` is set and empty, `gone` is not set at all, and the default is
	// what tells them apart — the same distinction the stored path draws.
	runSeam(t, r, `printf '[%s][%s]' "${view[here]:-D}" "${view[gone]:-D}"`)
	if got, want := out.String(), "[D][D]"; got != want {
		t.Errorf("`:-` over a keyed producer = %q, want %q", got, want)
	}
	runSeam(t, r, `printf '[%s][%s]' "${view[here]-D}" "${view[gone]-D}"`)
	if got, want := out.String(), "[D][D][][D]"; got != want {
		t.Errorf("`-` over a keyed producer = %q, want %q", got, want)
	}
}

// The whole-array spellings still need the whole table, and they are told
// apart before the keyed route is taken — `@` and `*` are the two shapes a
// single lookup cannot finish.
func TestTheWholeArraySpellingsStillBuildTheTable(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	whole := 0
	r.SetDynamicAssoc("view", func(*Runner) AssocArray {
		whole++
		return AssocArray{"a": "1"}
	})
	r.SetDynamicAssocElement("view", func(_ *Runner, key string) (string, bool) {
		v, ok := AssocArray{"a": "1"}[key]
		return v, ok
	})
	runSeam(t, r, `printf '[%s]' "${view[@]}" "${view[*]}"`)
	if got, want := out.String(), "[1][1]"; got != want {
		t.Errorf("the whole-array spellings = %q, want %q", got, want)
	}
	if whole != 2 {
		t.Errorf("`@` and `*` ran the whole-table producer %d times, want 2", whole)
	}
	// And a plain key beside them still does not.
	runSeam(t, r, `printf '[%s]' "${view[a]}"`)
	if whole != 2 {
		t.Errorf("a plain key ran the whole-table producer, total %d, want 2", whole)
	}
}

// A name with absent elements declared refuses by name on the keyed route
// too. The refusal is about a key the producer has no answer for, so moving
// where the key is looked up must not move where it is refused — see
// absentparam.go.
func TestAKeyedProducerStillRefusesAnAbsentElementByName(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.SetDynamicAssoc("view", func(*Runner) AssocArray { return AssocArray{"here": "yes"} })
	r.SetDynamicAssocElement("view", func(_ *Runner, key string) (string, bool) {
		return "yes", key == "here"
	})
	r.SetAbsentElements("view", "no answer for that one")
	// The refusal is fatal to the expansion, and what is asserted is the
	// sentence: which status a fatal error carries is an axis, and this
	// runner answers it only so the refusal does not draw a second
	// diagnostic about the unanswered one.
	f, err := syntax.Parse(`printf '[%s]' "${view[gone]}"`, syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, _ = r.Run(context.Background(), f)
	if out.String() != "" {
		t.Errorf("a refused element still printed %q", out.String())
	}
	if want := "view[gone]: no answer for that one"; !strings.Contains(errs.String(), want) {
		t.Errorf("refusal = %q, want it to contain %q", errs.String(), want)
	}
}

// A *stored* table shadows the keyed producer exactly as it shadows the
// whole-table one. Not a nicety: assocFor asks the stored table first, so a
// keyed producer that answered anyway would leave `${m[k]}` reading past a
// write that `${(k)m}` could still see.
func TestAStoredTableShadowsTheKeyedProducerToo(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.SetDynamicAssoc("view", func(*Runner) AssocArray { return AssocArray{"a": "produced"} })
	r.SetDynamicAssocElement("view", func(*Runner, string) (string, bool) {
		return "keyed", true
	})
	// No writer registered, so the assignment lands in a stored table — the
	// shadowing SetDynamicAssocWriter exists to prevent. What is asserted
	// here is only that both readings are shadowed the same way.
	runSeam(t, r, `view[b]=2
printf '[%s][%s]' "${view[a]}" "${view[b]}"`)
	if got, want := out.String(), "[][2]"; got != want {
		t.Errorf("reads after a write with no writer = %q, want %q", got, want)
	}
}

// Asking whether a name is an association produces nothing. Every caller of
// that predicate throws the table away, and on a real startup it was more
// than a quarter of the renderings of the costliest view in the shell:
// `m[k]=v` and `unset "m[k]"` each asked, and each built the lot to find out.
func TestAskingWhetherANameIsAnAssociationProducesNothing(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	whole := 0
	table := map[string]string{"a": "1"}
	r.SetDynamicAssoc("view", func(*Runner) AssocArray {
		whole++
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
		table[key] = value
	})
	runSeam(t, r, "view[b]=2")
	if whole != 0 {
		t.Errorf("an assignment produced the table %d times, want 0", whole)
	}
	// And the write still arrived, so nothing was skipped along with the
	// production.
	runSeam(t, r, `printf '[%s]' "${view[b]}"`)
	if got, want := out.String(), "[2]"; got != want {
		t.Errorf("the element written = %q, want %q", got, want)
	}
}

// One element of a *stored* association, read and written without the whole
// table passing through the caller's hands. The keyed counterpart of
// SetAssoc and GetAssoc, which copy — and the copying is the point: a
// dialect keeping a set of names in an association asked about one name per
// lookup and added one name per write, so both halves were linear in the set
// and filling it was quadratic.
func TestOneElementOfAStoredAssociationIsReadAndWrittenOnItsOwn(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.Register("mark", func(rr *Runner, _ context.Context, args []string) int {
		rr.SetAssocElement(args[0], args[1], args[2])
		return 0
	})
	r.Register("ask", func(rr *Runner, _ context.Context, args []string) int {
		v, ok := rr.AssocElement(args[0], args[1])
		if !ok {
			_, _ = rr.Out().Write([]byte("[absent]"))
			return 0
		}
		_, _ = rr.Out().Write([]byte("[" + v + "]"))
		return 0
	})
	// The name does not exist until the first write, which is the case a
	// "read the table, add to it, write it back" helper gets for free and a
	// keyed write has to do itself.
	runSeam(t, r, `ask set a
mark set a one
mark set b two
ask set a
ask set b
ask set c
ask nosuchname a`)
	if got, want := out.String(), "[absent][one][two][absent][absent]"; got != want {
		t.Errorf("keyed reads and writes = %q, want %q", got, want)
	}
	// And what was written is one association, visible as one — not a
	// private table beside the name.
	runSeam(t, r, `printf '[%s]' "${set[a]}" "${set[b]}"`)
	if got, want := out.String(), "[absent][one][two][absent][absent][one][two]"; got != want {
		t.Errorf("the same name read as a shell parameter = %q, want %q", got, want)
	}
}

// A sparse array reads exactly as it did before the dense shortcut was put
// in front of readArray, under *both* answers to the axis that decides what
// a gap is. The shortcut claims only subscripts 0 to n-1 with none missing,
// and the two readings of anything else are what it must not quietly
// replace: one yields the assigned elements, the other walks the whole
// extent and pads the gap with empties.
func TestASparseArrayReadsTheSameUnderTheDenseShortcut(t *testing.T) {
	for _, tc := range []struct {
		name   string
		sparse Answer
		want   string
	}{
		{"assigned only", Yes, "[x][y][z][q]"},
		{"the whole extent", No, "[x][y][z][][][][][q]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errs strings.Builder
			dg := Diagnostics{Location: LocationTightLine, BuiltinLocation: LocationTightLine}
			sem := Semantics{FatalErrorStatusIsOne: Yes, ArraysAreSparse: tc.sparse, ArrayBaseIsZero: Yes}
			r := newTestRunner(t, &Runner{
				Stdout: &out, Stderr: &errs, Diagnostics: &dg, Semantics: &sem, Name: "testsh",
			})
			runSeam(t, r, "a=(x y z)\na[7]=q\nprintf '[%s]' \"${a[@]}\"")
			if out.String() != tc.want {
				t.Errorf("a gap at 3..6 = %q, want %q (stderr %q)", out.String(), tc.want, errs.String())
			}
		})
	}
}

// And a dense one is unchanged, which is the case the shortcut does claim.
func TestADenseArrayReadsTheSameUnderTheDenseShortcut(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	runSeam(t, r, "a=(x y z)\nprintf '[%s]' \"${a[@]}\"")
	if got, want := out.String(), "[x][y][z]"; got != want {
		t.Errorf("a dense array = %q, want %q", got, want)
	}
}
