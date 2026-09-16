// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// How a failing pipeline is judged when its last element runs in this shell
// (#2921). See Semantics.FailingPipelineWhoseLastElementRanHere for the
// measurements; each row below is one of them, counted in E lines.

func lastHereSem(j LastElementJudging) Semantics {
	s := errSem()
	s.LastPipelineElementInCurrentShell = Yes
	s.ErrexitSeesPipefailFailure = Yes
	s.PipefailOption = Yes
	s.FailingPipelineWhoseLastElementRanHere = j
	return s
}

var lastElementRows = []struct {
	name, src string
	// E lines under each answer, in the order the constants are declared.
	unless, as, then int
}{
	{"a builtin", `true | false`, 1, 2, 2},
	{"a program on PATH", `true | sh -c 'exit 1'`, 1, 1, 2},
	{"a subshell", `true | ( exit 1 )`, 1, 1, 2},
	{"a group of one", `true | { false; }`, 1, 2, 2},
	{"a group of one on PATH", `true | { sh -c 'exit 1'; }`, 1, 1, 2},
	{"a group of two", `true | { :; false; }`, 1, 1, 2},
	{"a loop", `true | for i in 1; do false; done`, 1, 1, 2},
	{"a test", `true | [[ a == b ]]`, 1, 1, 2},
	{"a group ending in a bang", `true | { ! true; }`, 0, 0, 1},
	{"a group ending in a chain", `true | { false && true; }`, 0, 0, 1},
	{"only pipefail saw it", `set -o pipefail; false | true`, 1, 0, 1},
	{"pipefail and the element", `set -o pipefail; true | false`, 1, 2, 2},
}

func TestAFailingPipelineIsJudgedByItsLastElementsReading(t *testing.T) {
	for _, j := range []LastElementJudging{
		PipelineJudgedUnlessItsLastElementJudgedItself,
		PipelineJudgedAsItsLastElement,
		LastElementJudgedThenThePipeline,
	} {
		for _, row := range lastElementRows {
			want := map[LastElementJudging]int{
				PipelineJudgedUnlessItsLastElementJudgedItself: row.unless,
				PipelineJudgedAsItsLastElement:                 row.as,
				LastElementJudgedThenThePipeline:               row.then,
			}[j]
			out, _ := runGrammar(t, "trap 'echo E' ERR; "+row.src+"; echo done", enableTestAndArith, withSem(lastHereSem(j)))
			if got := strings.Count(out, "E\n"); got != want || !strings.HasSuffix(out, "done\n") {
				t.Errorf("%s under %s: got %q, want %d E", row.name, j, out, want)
			}
		}
	}
}

func TestAFunctionAsTheLastElementIsJudgedAtEachLevel(t *testing.T) {
	// The body's failure, the call, and the pipeline: three where the shell
	// judges a command it ran itself twice.
	s := lastHereSem(PipelineJudgedAsItsLastElement)
	s.ErrTrapRunsInsideFunctions = Yes
	out, _ := run(t, "trap 'echo E' ERR; f() { false; }; true | f; echo done", withSem(s))
	if out != "E\nE\nE\ndone\n" {
		t.Errorf("got %q, want three E", out)
	}
	// And a function whose body went to PATH still ran here.
	out, _ = run(t, "trap 'echo E' ERR; f() { sh -c 'exit 1'; }; true | f; echo done", withSem(s))
	if out != "E\nE\nE\ndone\n" {
		t.Errorf("a body on PATH: got %q, want three E", out)
	}
}

func TestSetEStopsWhereTheElementIsJudged(t *testing.T) {
	for _, j := range []LastElementJudging{
		PipelineJudgedUnlessItsLastElementJudgedItself,
		PipelineJudgedAsItsLastElement,
		LastElementJudgedThenThePipeline,
	} {
		s := lastHereSem(j)
		s.ErrexitSeesPipefailFailure = No
		out, _ := runGrammar(t, "set -e; true | { false && true; }; echo survived", enableTestAndArith, withSem(s))
		want := "survived\n"
		if j == LastElementJudgedThenThePipeline {
			want = ""
		}
		if out != want {
			t.Errorf("under %s: got %q, want %q", j, out, want)
		}
	}
}
