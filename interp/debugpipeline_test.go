// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// How a *pipeline* fires the DEBUG trap, which the heads table cannot answer:
// a pipeline is neither a simple command nor one of the compound heads that
// axis enumerates, so this engine fired nothing at all for one and a traced
// script skipped every `cmd | cmd` it had (#2797). See
// interp.DebugTrapPipeline for the panel these are written from.
//
// The action writes to **stderr**, which is deliberate twice over. It is not
// the pipe, so a count taken there is every firing wherever it happened —
// stdout belongs to the next element and a firing made inside one writes into
// a pipe nobody reads. And an action writing to stdout would *race*: the
// element downstream of `:` has exited by the time the action runs, so the
// write fails and takes the rest of the action with it, which counts as a
// missing firing and is nothing of the kind.
//
// Every row is run under both answers to whether the trap is carried into a
// subshell, because the two questions are coupled and the coupling is the
// reason the axis exists: a reading that leaves each element to fire for
// itself fires *nothing* in a shell that does not carry the trap into one,
// which is the shape of the bug, and the shell in that position is exactly
// the one whose firing moved into the shell instead.

// pipeSem answers everything a pipeline's firing needs, leaving the axis and
// the subshell question to the caller.
func pipeSem(how DebugTrapPipeline, subshells, lastHere Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.TrapHasDebugCondition = Yes
		s.DebugTrapRunsBeforeTheCommand = Yes
		s.DebugTrapRunsInsideCalls = No
		s.DebugTrapRunsInSubshells = subshells
		s.DebugTrapRefiresOnEnteringAFunction = No
		// No head fires of its own accord, so every firing a row counts is
		// one this axis decided or one a simple command made.
		s.DebugTrapCompoundHeads = DebugTrapHeadsNone
		s.DebugTrapPipelines = how
		s.LastPipelineElementInCurrentShell = lastHere
	}
}

// pipeDs runs src under one reading and counts the firings, wherever they
// happened.
func pipeDs(t *testing.T, src string, how DebugTrapPipeline, subshells, lastHere Answer) int {
	t.Helper()
	const act = "trap 'echo D >&2' DEBUG\n"
	out, errs, _ := trapRun(t, act+src, pipeSem(how, subshells, lastHere), Diagnostics{})
	if strings.Contains(out, "D") {
		t.Fatalf("ran %q: the action wrote to stdout: %q", src, out)
	}
	return strings.Count(errs, "D\n")
}

func TestHowAPipelineFiresADebugTrap(t *testing.T) {
	for _, c := range []struct {
		name, src string
		// Firings under each reading with the trap kept out of subshells,
		// and then the same three with it carried into them.
		eachOut, perOut, onceOut int
		eachIn, perIn, onceIn    int
	}{
		{
			// Two simple elements, and the row that separates all three
			// readings on its own in the left-hand column: nothing, one per
			// element, one for the pipeline.
			name: "two simple elements", src: ": | :",
			eachOut: 0, perOut: 2, onceOut: 1,
			eachIn: 2, perIn: 2, onceIn: 1,
		},
		{
			// Four, which is what says the first two readings count
			// *elements* rather than adding one firing to a pipeline, and
			// that the third does not count them at all.
			name: "four simple elements", src: ": | : | : | :",
			eachOut: 0, perOut: 4, onceOut: 1,
			eachIn: 4, perIn: 4, onceIn: 1,
		},
		{
			// An element that is not a simple command, which is where the
			// per-element reading is narrower than it sounds: it fires for
			// the `:` element and not for the group beside it.
			name: "a compound element first", src: "{ :; } | :",
			eachOut: 0, perOut: 1, onceOut: 1,
			eachIn: 2, perIn: 2, onceIn: 2,
		},
		{
			// The same with the group last, so the answer is about what the
			// element *is* and not about where it sits.
			name: "a compound element last", src: ": | { :; }",
			eachOut: 0, perOut: 1, onceOut: 1,
			eachIn: 2, perIn: 2, onceIn: 2,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, r := range []struct {
				how             DebugTrapPipeline
				wantOut, wantIn int
			}{
				{DebugTrapPipelineInEachElement, c.eachOut, c.eachIn},
				{DebugTrapPipelinePerSimpleElement, c.perOut, c.perIn},
				{DebugTrapPipelineOnceForThePipeline, c.onceOut, c.onceIn},
			} {
				if got := pipeDs(t, c.src, r.how, No, No); got != r.wantOut {
					t.Errorf("%v over %q, trap not carried into a subshell: %d firings, want %d",
						r.how, c.src, got, r.wantOut)
				}
				if got := pipeDs(t, c.src, r.how, Yes, No); got != r.wantIn {
					t.Errorf("%v over %q, trap carried into a subshell: %d firings, want %d",
						r.how, c.src, got, r.wantIn)
				}
			}
		})
	}
}

// The right-hand column above is where the two readings that fire in the
// shell have to withhold the element's own firing, and the count cannot say
// where the withholding stops. This does: a command *inside* an element is a
// command in its own right and goes on firing, so the group contributes its
// own `:` and nothing for the group.
//
// The last element runs in the current shell here, which is the case the
// withholding is load-bearing for under any reading — that element is the
// shell rather than a copy, so a flag that did not reach it would fire twice
// however the trap is carried.
func TestAPipelineElementFiresNothingOfItsOwnWhereThePipelineFired(t *testing.T) {
	for _, c := range []struct {
		how  DebugTrapPipeline
		src  string
		want int
	}{
		// The pipeline fires for both elements; neither fires again, even
		// with the trap carried into a subshell and the last element being
		// the shell itself.
		{DebugTrapPipelinePerSimpleElement, ": | :", 2},
		// One firing for the pipeline, and the group's own command still
		// fires inside it — a withheld head, not a silenced element.
		{DebugTrapPipelineOnceForThePipeline, ": | { :; }", 2},
		// The control: with nothing fired in the shell, the same two
		// firings are the element's own and the group's own.
		{DebugTrapPipelineInEachElement, ": | { :; }", 2},
	} {
		if got := pipeDs(t, c.src, c.how, Yes, Yes); got != c.want {
			t.Errorf("%v over %q with the last element here: %d firings, want %d",
				c.how, c.src, got, c.want)
		}
	}
}

// A firing the pipeline makes happens in the shell running it, which is what
// lets the action's own assignments outlive the pipeline — and is the half of
// the question no count of output lines can see, since an element's streams
// and an element's variables are two different boundaries.
//
// `: | :` is two elements and a `trap -` that fires once itself, so the
// reading that fires in the shell counts three and the one that fires inside
// the elements counts one: everything an element's action assigned went with
// the element.
func TestAPipelineFiresInTheShellOrInTheElement(t *testing.T) {
	const src = "n=0\ntrap 'n=$((n+1))' DEBUG\n: | :\ntrap - DEBUG\necho n=$n\n"
	for _, c := range []struct {
		how  DebugTrapPipeline
		want string
	}{
		{DebugTrapPipelineInEachElement, "n=1\n"},
		{DebugTrapPipelinePerSimpleElement, "n=3\n"},
		{DebugTrapPipelineOnceForThePipeline, "n=2\n"},
	} {
		out, errs, _ := trapRun(t, src, pipeSem(c.how, Yes, No), Diagnostics{})
		if errs != "" {
			t.Fatalf("%v: stderr %q", c.how, errs)
		}
		if out != c.want {
			t.Errorf("%v: %q, want %q", c.how, out, c.want)
		}
	}
}

// TestADialectWithNoDebugConditionFiresNothingForAPipeline is the verdict
// Semantics.DebugTrapPipelines names for the two dialects the corpus cannot
// pin it in: both refuse `trap … DEBUG` outright, so no reading of this axis
// can reach a firing there and every value behaves alike.
//
// Asserted rather than reasoned, because "the condition is refused" and "the
// pipeline fires nothing" are two claims and only the first is obvious: a
// firing site that read the axis before the condition would fire for a shell
// that had refused to set the trap at all.
func TestADialectWithNoDebugConditionFiresNothingForAPipeline(t *testing.T) {
	const src = "trap 'echo D >&2' DEBUG\n: | :\n: | { :; }\necho done\n"
	for _, how := range []DebugTrapPipeline{
		DebugTrapPipelineInEachElement,
		DebugTrapPipelinePerSimpleElement,
		DebugTrapPipelineOnceForThePipeline,
	} {
		out, errs, _ := trapRun(t, src, func(s *Semantics) {
			pipeSem(how, Yes, No)(s)
			s.TrapHasDebugCondition = No
		}, Diagnostics{})
		if strings.Contains(errs, "D\n") {
			t.Errorf("%v with no DEBUG condition: fired, stderr %q", how, errs)
		}
		if !strings.Contains(out, "done\n") {
			t.Errorf("%v with no DEBUG condition: the script did not finish, stdout %q", how, out)
		}
	}
}

// TestAPipelineWithholdsAnElementsHeadAndNothingDeeper is the half of the
// withholding the counts above cannot see, because they run with no compound
// head firing at all: where a dialect *does* write a head for a `{ }`, an
// element that is one must not write it where the pipeline has already fired
// — and the command inside the group must go on firing, because it is a
// command in its own right and not the element's head.
//
// Measured in zsh 5.9.2, the column that writes a head for every compound:
// `trap 'echo d' DEBUG; { echo a; } | sed 's/^/1:/'` writes `d`, `1:d`, `1:a`
// — one firing for the pipeline, none for the group, and one for the `echo`
// inside it. Three firings there would be the group's head fired twice over.
func TestAPipelineWithholdsAnElementsHeadAndNothingDeeper(t *testing.T) {
	const src = ": | { :; }"
	for _, c := range []struct {
		how      DebugTrapPipeline
		lastHere Answer
		want     int
	}{
		// One firing for the pipeline, none for the group beside it, and
		// the group's own `:` still fires.
		{DebugTrapPipelineOnceForThePipeline, No, 2},
		// One for the `:` element — the group is not a simple command, so
		// it is not fired for and its head is withheld all the same.
		{DebugTrapPipelinePerSimpleElement, No, 2},
		// The same two with the group running on the shell itself rather
		// than on a copy, which is the element a flag left in the runner
		// would reach differently.
		{DebugTrapPipelineOnceForThePipeline, Yes, 2},
		{DebugTrapPipelinePerSimpleElement, Yes, 2},
		// The control: with nothing fired for the pipeline, the group
		// writes its own head and the count is one higher.
		{DebugTrapPipelineInEachElement, No, 3},
		{DebugTrapPipelineInEachElement, Yes, 3},
	} {
		const act = "trap 'echo D >&2' DEBUG\n"
		_, errs, _ := trapRun(t, act+src, func(s *Semantics) {
			pipeSem(c.how, Yes, c.lastHere)(s)
			s.DebugTrapCompoundHeads = DebugTrapHeadsEveryCompound
		}, Diagnostics{})
		if got := strings.Count(errs, "D\n"); got != c.want {
			t.Errorf("%v over %q, last element here %v: %d firings, want %d",
				c.how, src, c.lastHere, got, c.want)
		}
	}
}

// TestAnActionThatUnwindsStopsThePipelineBeforeItStarts is what the firing's
// *position* buys, and the only place a count cannot show it: the firings a
// pipeline makes all happen before any element starts, so an action that
// exits at one of them leaves the whole pipeline unrun rather than half of
// it — and the elements after the one it fired for are not fired for either.
//
// Measured in bash 5.3.15, 2026-09-14, `env -i PATH=/usr/bin:/bin`: over
// `: | : | :; echo after`, an action exiting at the nth firing writes exactly
// f1…fn and leaves 9, for every n from 1 to 4. This shell answers the same at
// every position.
//
// Each element writes to stderr rather than down the pipe, so an element that
// ran despite the unwinding is visible rather than swallowed. The elements
// run at the same time as each other, so the row that lets them run asks
// which of them wrote and never in what order.
func TestAnActionThatUnwindsStopsThePipelineBeforeItStarts(t *testing.T) {
	const src = "n=0\ntrap 'n=$((n+1)); echo f$n >&2; [ $n = 2 ] && exit 9; true' DEBUG\n" +
		"echo A >&2 | echo B >&2 | echo C >&2\necho after >&2\n"
	// f1 and f2 are the first two elements; the third is not fired for, no
	// element runs, and the command after the pipeline is never reached.
	_, errs, code := trapRun(t, src, pipeSem(DebugTrapPipelinePerSimpleElement, No, No), Diagnostics{})
	if errs != "f1\nf2\n" || code != 9 {
		t.Errorf("DebugTrapPipelinePerSimpleElement: stderr %q status %d, want %q and 9",
			errs, code, "f1\nf2\n")
	}
	// The control from the other reading: one firing for the pipeline, so
	// the unwinding one is the `echo after` and every element did run.
	_, errs, code = trapRun(t, src, pipeSem(DebugTrapPipelineOnceForThePipeline, No, No), Diagnostics{})
	for _, want := range []string{"f1\n", "A\n", "B\n", "C\n", "f2\n"} {
		if !strings.Contains(errs, want) {
			t.Errorf("DebugTrapPipelineOnceForThePipeline: stderr %q has no %q", errs, want)
		}
	}
	if strings.Contains(errs, "after") || code != 9 {
		t.Errorf("DebugTrapPipelineOnceForThePipeline: stderr %q status %d, want no `after` and 9",
			errs, code)
	}
}
