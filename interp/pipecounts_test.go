// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// pipeCountsRunner is a bare runner whose vector reads a compound's body for
// the record, with the three axes that reading consults set as given.
func pipeCountsRunner(assignment, arrayAssignment, testAndArith Answer) *Runner {
	r := cacheTestRunner()
	r.Semantics.CompoundPipelineStatusRecord = CompoundPipelineStatusFromTheBody
	r.Semantics.AssignmentUpdatesPipelineStatus = assignment
	r.Semantics.ArrayAssignmentUpdatesPipelineStatus = arrayAssignment
	r.Semantics.TestAndArithmeticUpdatePipelineStatus = testAndArith
	return r
}

// compoundsIn is every compound command in src, nested ones included, which
// are the commands the kept answers are kept for.
func compoundsIn(t *testing.T, src string) []syntax.Command {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	var out []syntax.Command
	var list func([]*syntax.Stmt)
	var expr func(syntax.Expr)
	list = func(stmts []*syntax.Stmt) {
		for _, st := range stmts {
			expr(st.Expr)
		}
	}
	expr = func(e syntax.Expr) {
		switch x := e.(type) {
		case *syntax.BinaryExpr:
			expr(x.X)
			expr(x.Y)
		case *syntax.Pipeline:
			for _, c := range x.Cmds {
				switch c := c.(type) {
				case *syntax.IfClause:
					out = append(out, c)
					list(c.Cond)
					list(c.Then)
					for _, e := range c.Elifs {
						list(e.Cond)
						list(e.Then)
					}
					list(c.Else)
				case *syntax.CaseClause:
					out = append(out, c)
					for _, it := range c.Items {
						list(it.Body)
					}
				case *syntax.LoopClause:
					out = append(out, c)
					list(c.Cond)
					list(c.Body)
				case *syntax.ForClause:
					out = append(out, c)
					list(c.Body)
				case *syntax.Group:
					out = append(out, c)
					list(c.List)
				}
			}
		}
	}
	list(f.Stmts)
	if len(out) == 0 {
		t.Fatalf("%q holds no compound, so it tests nothing", src)
	}
	return out
}

// A kept answer is the reading it stands for: every compound, under every
// pair of answers the three axes can give, asked once to fill the table and
// once more to be answered from it, against a fresh read. The entry is looked
// for before the second ask, so a table that kept nothing cannot pass by
// agreeing with itself.
func TestAKeptCompoundAnswerIsTheReadItStandsFor(t *testing.T) {
	t.Parallel()
	scripts := []string{
		`if [[ a ]]; then (( 1 )); elif [[ b ]]; then x=1; else y=(1); fi`,
		`if [[ a ]]; then (( 1 )); elif [[ b ]]; then x=1; else echo hi; fi`,
		`case $x in a) x=1 ;; b) (( 2 )) ;; c) [[ c ]] ;; esac`,
		`case $x in a) x=1 ;; b) echo hi ;; esac`,
		`while [[ a ]]; do if (( 1 )); then { x=1; }; fi; done`,
		`for w in a b; do x=$w; done >/dev/null`,
		`{ (( 1 )); [[ a ]] && x=2; }`,
		`if [[ a ]]; then (( 1 )) & fi`,
	}
	answers := []Answer{Yes, No}
	for _, assignment := range answers {
		for _, arrayAssignment := range answers {
			for _, testAndArith := range answers {
				r := pipeCountsRunner(assignment, arrayAssignment, testAndArith)
				for _, src := range scripts {
					for _, c := range compoundsIn(t, src) {
						want := r.countsForPipelineStatusRead(c)
						if got := r.countsForPipelineStatus(c); got != want {
							t.Errorf("%q, %T, first ask: %v, read %v", src, c, got, want)
						}
						if _, ok := r.pipeCounts[c]; !ok {
							t.Fatalf("%q, %T: not kept, so the second ask below tests nothing", src, c)
						}
						if got := r.countsForPipelineStatus(c); got != want {
							t.Errorf("%q, %T, kept: %v, read %v", src, c, got, want)
						}
					}
				}
			}
		}
	}
}

// The answer is kept under the axes it was read under, and an axis that moves
// takes the table with it: `[[ … ]]` and `(( … ))` count under one answer and
// not under the other, so the same `if` is two answers on one runner.
func TestAKeptCompoundAnswerMovesWithTheAxes(t *testing.T) {
	t.Parallel()
	r := pipeCountsRunner(No, Yes, No)
	c := compoundsIn(t, `if [[ a ]]; then (( 1 )); fi`)[0]
	if r.countsForPipelineStatus(c) {
		t.Fatal("the `if` counts with neither of its commands counting, so the move below tests nothing")
	}
	r.Semantics.TestAndArithmeticUpdatePipelineStatus = Yes
	if !r.countsForPipelineStatus(c) {
		t.Error("an axis that moved was answered from what was kept before it moved")
	}
}

// Nothing is kept where reading it says something: an unanswered axis is
// refused out loud every time it is reached, and a policy that does not read
// the body decides without a walk at all.
func TestAnUnansweredAxisKeepsNoCompoundAnswer(t *testing.T) {
	t.Parallel()
	r := pipeCountsRunner(No, Yes, Unspecified)
	c := compoundsIn(t, `if [[ a ]]; then (( 1 )); fi`)[0]
	r.countsForPipelineStatus(c)
	if !r.unspecified {
		t.Fatal("the unanswered axis was not reached, so nothing here tests whether it is kept")
	}
	if _, ok := r.pipeCounts[c]; ok {
		t.Error("an answer read under an unanswered axis was kept, so the refusal is said once")
	}
	r = pipeCountsRunner(No, Yes, No)
	r.Semantics.CompoundPipelineStatusRecord = CompoundPipelineStatusFromWhatRan
	r.countsForPipelineStatus(c)
	if len(r.pipeCounts) != 0 {
		t.Error("a policy that does not read the body kept an answer")
	}
}

// The bookkeeping every command does and undoes costs nothing to undo: each
// of these answered a closure, which was an allocation per `(( … ))`, per
// compound and per `case` (#6423). A kept compound answer is a map read.
func TestTheBookkeepingACommandUndoesAllocatesNothing(t *testing.T) {
	r := pipeCountsRunner(No, Yes, No)
	r.Semantics.FailedExpansionInADeclarationOrCaseSetsNoStatus = Yes
	r.Semantics.ACommandHoldsAJobSlot = Yes
	c := compoundsIn(t, `if [[ a ]]; then (( 1 )); fi`)[0]
	r.countsForPipelineStatus(c)
	for _, row := range []struct {
		name string
		f    func()
	}{
		{"an arithmetic command's line", func() { r.inArithCommandText(syntax.Pos{Line: 3}).undo() }},
		{"a failed expansion's status", func() { r.failedExpansionStatusIs(failedExpansionLeavesZero).undo() }},
		{"a command's job slot", func() {
			hold, ok := r.holdACommandsJobSlot(true)
			if !ok {
				t.Fatal("no slot was held, so this row tests nothing")
			}
			hold.release()
		}},
		{"a kept compound answer", func() { r.countsForPipelineStatus(c) }},
	} {
		if n := testing.AllocsPerRun(100, row.f); n != 0 {
			t.Errorf("%s allocated %v times; want 0", row.name, n)
		}
	}
	if r.substFragmentLine != 0 || r.failedExpansionStatus != 0 || r.commandSlotHeld {
		t.Error("an undo did not put back what it took")
	}
}
