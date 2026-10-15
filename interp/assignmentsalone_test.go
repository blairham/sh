// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	. "github.com/blairham/sh/interp"
)

// assignmentsAloneScript runs commands made of assignments alone through
// everything simpleCommand answers for one: the status a substitution leaves,
// `$_`, a function's locals, an array, `set -x`, the DEBUG and ERR traps,
// `set -e`, a readonly name, a value that will not expand, and a loop. A
// refusal is made inside a subshell, so it is part of the output both runs
// must agree on and the script carries on.
const assignmentsAloneScript = `p() { printf '<%s>' "$@"; echo; }
false; x=1; p "$?"
false; x=$(exit 3); p "$?"
false; X=$(exit 2) Y=$(exit 4); p "$?" "$X$Y"
echo a b >/dev/null; x=1; p "$_"
i=0; i=$((i + 1)); j=$((i * 5)); p "$i" "$j"
f() { local v; v=$1; w=$((v + 1)); p "$v" "$w"; }; f 4; p "$w"
a=(1 2 3); p "${a[@]}"
set -x; tx=1 ty=$((tx + 1)); set +x
trap 'echo dbg' DEBUG; dx=1; trap - DEBUG
trap 'echo err' ERR; ex=$(false); trap - ERR
(set -e; sx=$(false); echo no); p "$?"
(readonly ro=1; ro=2; echo after); p "$?"
(bx=$(( } )); echo after); p "$?"
(nx=$((1 / 0)); echo after); p "$?"
x=1 && p and; x=$(false) || p or
cnt=0; while [ $cnt -lt 3 ]; do cnt=$((cnt + 1)); done; p "$cnt"
p end
`

// TestACommandOfAssignmentsAloneAnswersAsSimpleCommandDoes runs
// assignmentsAloneScript with assignmentsAloneCommand on and off, and requires
// the same output and status (#6496).
func TestACommandOfAssignmentsAloneAnswersAsSimpleCommandDoes(t *testing.T) {
	for _, p := range []dialecttest.Preset{
		{Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics, Diagnostics: bash.Diagnostics, Apply: bash.Apply},
		{Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics, Diagnostics: zsh.Diagnostics, Apply: zsh.Apply},
	} {
		t.Run(p.Name, func(t *testing.T) {
			run := func(on bool) (string, int) {
				was := SetAssignmentsAloneShortcutForTest(on)
				defer SetAssignmentsAloneShortcutForTest(was)
				out, st, err := p.Combined(t, dialecttest.Base{Dir: t.TempDir()}, assignmentsAloneScript)
				if err != nil {
					t.Fatalf("shortcut %v: %v", on, err)
				}
				return out, st
			}
			full, fullStatus := run(false)
			short, shortStatus := run(true)
			if short != full || shortStatus != fullStatus {
				t.Errorf("with the shortcut:\n%s(status %d)\nwithout it:\n%s(status %d)", short, shortStatus, full, fullStatus)
			}
			// The rows the comparison leans on, so a script that stopped
			// early compares nothing.
			for _, row := range []string{"<0>\n<3>\n<4><>\n", "<1><5>\n", "<4><5>\n<5>\n", "<1><2><3>\n", "dbg\n", "err\n", "<and>\n<or>\n<3>\n<end>\n"} {
				if !strings.Contains(full, row) {
					t.Errorf("the script did not reach %q:\n%s", row, full)
				}
			}
		})
	}
}

// theRoadToNoWords is every method and field simpleCommand reaches on the
// runner outside its loops over the words and before it hands a command with
// no words to storeTheAssignmentsAlone — the statements a command of
// assignments alone passes through — as assignmentsAloneCommand was checked
// against it. A call added there is one assignmentsAloneCommand does not make:
// when this test fails, decide whether the new call can do anything for a
// command with no words, redirections or modifiers, make it in
// assignmentsAloneCommand if so, and then update the list.
var theRoadToNoWords = []string{
	"arrayOperandName", "arrayOperands", "beginGlobUnit",
	"ctl", "dashPrecommand", "declarationOperands", "declinesRmStar", "expandErr",
	"failedExpansion", "failedExpansionStatus", "fatal", "diag", "lastArg", "lastArgSet",
	"lexedSubscriptOperands", "loopDepth", "noteInputLevelArgument", "nullCommand", "operandWasRejoined",
	"precommands", "rejoinedArrayOperand", "rejoinsArrayOperand", "scopeProcSubs", "sem",
	"sourceClosedSubscriptOperands", "startSimpleCommand", "status", "storeTheAssignmentsAlone",
	"traceEmptyCommand", "underscoreBeforeADeclarationsAssignment", "unspecified", "leadingModifierWords",
}

// TestTheAssignmentsAloneRoadIsTheOneItWasCheckedAgainst fails when what
// simpleCommand reaches around its word loop changes. See theRoadToNoWords.
func TestTheAssignmentsAloneRoadIsTheOneItWasCheckedAgainst(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "runner.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var body []ast.Stmt
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv != nil && fn.Name.Name == "simpleCommand" {
			body = fn.Body.List
		}
	}
	// The loops over the words, which a command with none never enters,
	// and the statement that hands such a command to
	// storeTheAssignmentsAlone; everything else up to it is the road.
	loops, handOff := map[int]bool{}, -1
	for i, s := range body {
		if rs, ok := s.(*ast.RangeStmt); ok {
			if sel, ok := rs.X.(*ast.SelectorExpr); ok && sel.Sel.Name == "Args" {
				loops[i] = true
			}
		}
		if is, ok := s.(*ast.IfStmt); ok && handOff < 0 && strings.Contains(nodeText(is.Body), "storeTheAssignmentsAlone") {
			handOff = i
		}
	}
	if len(loops) != 2 || handOff < 0 {
		t.Fatalf("found %d loops over the words and the hand-off at %d in simpleCommand, want 2 and one: the guard is looking in the wrong place", len(loops), handOff)
	}
	calls := map[string]bool{}
	for i, s := range body[:handOff+1] {
		if loops[i] {
			continue
		}
		ast.Inspect(s, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "r" {
					calls[sel.Sel.Name] = true
				}
			}
			return true
		})
	}
	var got []string
	for name := range calls {
		got = append(got, name)
	}
	slices.Sort(got)
	want := slices.Clone(theRoadToNoWords)
	slices.Sort(want)
	if added, removed := difference(got, want), difference(want, got); len(added) > 0 || len(removed) > 0 {
		t.Errorf("simpleCommand's road to a command with no words now reaches [%s] and no longer [%s]: read "+
			"assignmentsAloneCommand against the change, then update theRoadToNoWords",
			strings.Join(added, ", "), strings.Join(removed, ", "))
	}
}

// nodeText is the names an AST node mentions, enough to recognize a call.
func nodeText(n ast.Node) string {
	var b strings.Builder
	ast.Inspect(n, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			b.WriteString(id.Name)
			b.WriteByte(' ')
		}
		return true
	})
	return b.String()
}
