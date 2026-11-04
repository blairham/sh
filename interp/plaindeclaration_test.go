// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// declarationCommandScript is the lines a declaration line taken at the command
// by plainBuiltinCommand draws its edges at, beside the plain line's and the
// empty shadow's own scripts: a bare word, which lists; a letter and a word
// that is no name; a function, an alias of the line and an `eval` of it; the
// top level, where the word refuses or declares a global; a frozen name; a
// name already declared, which lists in the column that lists it; `$_`
// behind the line; `set -e` and the ERR and DEBUG traps over a refusal; a
// pipeline; and output held around a listing.
//
// Shapes a vector may refuse are run through `command eval` or a subshell,
// so a refusal is part of the output both runs must agree on.
const declarationCommandScript = `p() { printf '[%s]' "$@"; echo; }
show() { for n; do typeset -p "$n" 2>&1 || echo "$n:absent"; done; }
command eval 'readonly frz=r'; pd=outer
one() { local pa pb pc; p "<${pa-unset}>" "<${pd-unset}>"; show pa pb pc; }; one; show pa pb pc
under() { local ua ub; p "$_"; }; under
(top() { :; }; local ta tb; echo "top $?"; show ta tb); echo "after top $?"
bare() { local; echo "bare $?"; }; (bare) >/dev/null; echo "after bare $?"
letter() { local -i li lj; li=3+4; p "$li"; }; letter
(fro() { local fa frz fb; echo "fro $?"; show fa fb; }; fro) 2>&1; echo "after fro $?"
twice() { local tw; local tw; echo "twice $?"; }; twice 2>&1
listed() { local la=1; local la lb; echo "listed $?"; }; listed 2>&1
(set -e; se() { local sa frz; echo "se $?"; }; se; echo "after se") 2>&1; echo "set -e $?"
(command eval 'trap "echo err" ERR'; er() { local ea frz; :; }; er) 2>&1
(command eval 'trap "echo dbg" DEBUG'; db() { local da db; p "$da"; }; db; command eval 'trap - DEBUG')
piped() { local pp pq | cat; echo "piped $?"; }; piped
evald() { eval 'local ev1 ev2'; show ev1 ev2; }; evald
nested() { local na; inner; p "<${na-unset}><${ia-unset}>"; }
inner() { local ia na; na=in; ia=x; }; nested; show na ia
(local() { echo "fn local $*"; }; fl() { local fa fb; }; fl; unset -f local); echo "after fn $?"
ret() { local ra rb; return 5; }; ret; echo "ret $?"
out() { local oa ob; p held; }; out | cat
p decl-end
`

// declarationCommandRows are lines declarationCommandScript writes, which the
// full road must write for a comparison to have compared it.
var declarationCommandRows = []string{"[7]\n", "[held]\n", "[decl-end]\n"}

// declarationCommandVectors are the plain line's vectors, with `$_` tracked by
// the command and by the input line, and exit values reported.
func declarationCommandVectors() map[string]Semantics {
	v := plainLineVectors()
	under := v["base"]
	under.UnderscoreTracksTheLastArgument, under.UnderscoreIsAParameterAtAll = Yes, Yes
	under.UnderscoreHoldsTheCallAcrossEvalAndSource = Yes
	v["underscore"] = under
	input := under
	input.UnderscoreMovesOnlyBetweenInputCommands = Yes
	v["input underscore"] = input
	return v
}

// declarationCommandCompare runs prog with plainBuiltinCommand on and off, with
// the plain line's own route as line says, and requires the same output and
// status, at least atLeast declaration lines run by the route with it on and
// none with it off, and the full road's output to hold every row.
func declarationCommandCompare(
	t *testing.T, prog *syntax.File, sem Semantics, inheritsOuter, line bool, atLeast int, rows []string,
) {
	t.Helper()
	was := SetPlainLineShortcutForTest(line)
	defer SetPlainLineShortcutForTest(was)
	run := declarationRouteRun(t, prog, sem, inheritsOuter, SetPlainBuiltinShortcutForTest,
		func(r *Runner) (int, int) { return PlainDeclarationLinesForTest(r), 0 })
	full, fullStatus, fullTaken, _ := run(false)
	short, shortStatus, taken, _ := run(true)
	if short != full || shortStatus != fullStatus {
		t.Errorf("with the route:\n%s(status %d)\nwithout it:\n%s(status %d)", short, shortStatus, full, fullStatus)
	}
	if fullTaken != 0 {
		t.Errorf("the route ran %d declaration lines with it off; want none", fullTaken)
	}
	if taken < atLeast {
		t.Errorf("the route ran %d declaration lines; want at least %d", taken, atLeast)
	}
	for _, row := range rows {
		if !strings.Contains(full, row) {
			t.Errorf("the script did not reach %q:\n%s", row, full)
		}
	}
}

// TestAPlainDeclarationLineAnswersAsSimpleCommandDoes runs the empty shadow's
// script, the plain line's and declarationCommandScript — `local`, then
// `typeset` and `declare` in both forms of function — with plainBuiltinCommand
// taking a declaration line of plain names and with simpleCommand taking
// every line, under every vector the plain line's own test runs and three of
// `$_`'s, with the utility's plain line route on and off, and requires
// byte-identical output and status, and the route taken (#6373).
func TestAPlainDeclarationLineAnswersAsSimpleCommandDoes(t *testing.T) {
	// least is how many of each script's lines are plain declarations the
	// route must run, counted: most lines of the plain line's script hold a
	// value, which the route leaves to simpleCommand.
	scripts := []struct {
		name, src string
		rows      []string
		least     int
	}{
		{"shadow", emptyShadowScript + emptyShadowGlobalLines, emptyShadowRows, 17},
		{"line", plainLineScript, plainLineRows, 4},
		{"command", declarationCommandScript, declarationCommandRows, 9},
	}
	for _, sc := range scripts {
		for _, word := range []string{"local", "typeset", "declare"} {
			for _, keyword := range []bool{false, true} {
				if word == "local" && keyword {
					continue
				}
				prog := declarationsSpelled(t, sc.src, word, keyword)
				form := map[bool]string{false: "f()", true: "function f"}[keyword]
				for vname, sem := range declarationCommandVectors() {
					sem.DeclareOptions = "aAgilnprux"
					sem.TypesetLocalNeedsKeywordFunction = No
					for _, line := range []bool{true, false} {
						lname := map[bool]string{true: "line route", false: "full road"}[line]
						t.Run(sc.name+"/"+word+"/"+form+"/"+vname+"/"+lname, func(t *testing.T) {
							atLeast, rows := sc.least, sc.rows
							if vname == "empty open" {
								// The first valueless operand stops the
								// plain line's script on the question.
								atLeast, rows = min(atLeast, 1), nil
							}
							declarationCommandCompare(t, prog, sem, vname == "inherits outer", line, atLeast, rows)
						})
					}
				}
			}
		}
	}
}
