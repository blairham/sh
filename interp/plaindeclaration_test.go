// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"slices"
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
// pipeline; output held around a listing; and the operands written
// `name=$param` — a positional, a named scalar, an unset one under `set -u`,
// one holding a separator, an `=` or nothing, one naming an operand of the
// same line, one over a frozen name, a subscripted and a dotted name beside
// them, and `set -k`, which one column takes the operand away for.
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
vn=named; va() { local v1=$1 v2=$vn v3 v4=$2; p "$v1" "$v2" "<${v3-unset}>" "$v4"; show v1 v2 v3 v4; }; va one two
show v1 v2 v3 v4
uv() { local ux=$1 uy; p "$_"; }; uv x
(uu() { set -u; local ux=$2 uy; p "<$ux>"; }; uu x) 2>&1; echo "after uu $?"
sp() { local s1=$1 s2=$2 s3=$3; p "$s1" "$s2" "<$s3>"; }; sp 'a b' 'k=v' ''
ob=outer; same() { local ob=$1 oc=$ob; p "$ob" "$oc"; }; same in
(fv() { local f1=$1 frz=$1 f2=$1; echo "fv $?"; show f1 f2; }; fv x) 2>&1; echo "after fv $?"
(set -k; kv() { local k1=$1 k2; show k1 k2; }; kv x) 2>&1; echo "after kv $?"
tv() { local t1=$1; t1=changed; inner2; p "$t1"; }; inner2() { local t1=$1 t2=$t1; p "$t2"; }; tv first
(su() { local s9=$1 sx[1]=$1 s8=$1; echo "su $?"; show s9 s8; }; su x) 2>&1; echo "after su $?"
(dn() { local d9=$1 d.x=$1 d8=$1; echo "dn $?"; show d9 d8; }; dn x) 2>&1; echo "after dn $?"
p decl-end
`

// declarationCommandRows are lines declarationCommandScript writes, which the
// full road must write for a comparison to have compared it.
var declarationCommandRows = []string{
	"[7]\n", "[held]\n", "[one][named][<", "[in][outer]\n", "[changed]\n", "[decl-end]\n",
}

// declarationCommandVectors are the plain line's vectors, with `$_` tracked by
// the command and by the input line, and with operands stored in turn.
func declarationCommandVectors() map[string]Semantics {
	v := plainLineVectors()
	under := v["base"]
	under.UnderscoreTracksTheLastArgument, under.UnderscoreIsAParameterAtAll = Yes, Yes
	under.UnderscoreHoldsTheCallAcrossEvalAndSource = Yes
	v["underscore"] = under
	input := under
	input.UnderscoreMovesOnlyBetweenInputCommands = Yes
	v["input underscore"] = input
	// And the column that stores a declaration's operands in turn, whose
	// valued lines the route leaves to simpleCommand.
	turn := v["base"]
	turn.DeclarationOperandsAreStoredInTurn = Yes
	v["stored in turn"] = turn
	// And `set -k` taking a declaration's operand for a prefix, which the
	// route leaves to simpleCommand.
	promotes := v["base"]
	promotes.KeywordAssignments, promotes.KeywordPromotesADeclarationsOperand = Yes, Yes
	v["set -k promotes"] = promotes
	return v
}

// declarationCommandCompare runs prog with plainBuiltinCommand on and off, with
// the plain line's own route as line says, and requires the same output and
// status, at least least.lines declaration lines run by the route with it on,
// least.valued of them with a `name=$param` operand, and none with it off, and
// the full road's output to hold every row.
func declarationCommandCompare(
	t *testing.T, prog *syntax.File, sem Semantics, inheritsOuter, line bool, least routeBar, rows []string,
) {
	t.Helper()
	was := SetPlainLineShortcutForTest(line)
	defer SetPlainLineShortcutForTest(was)
	run := declarationRouteRun(t, prog, sem, inheritsOuter, SetPlainBuiltinShortcutForTest,
		func(r *Runner) (int, int) {
			return PlainDeclarationLinesForTest(r), PlainValuedDeclarationLinesForTest(r)
		})
	full, fullStatus, fullTaken, _ := run(false)
	short, shortStatus, taken, valued := run(true)
	if short != full || shortStatus != fullStatus {
		t.Errorf("with the route:\n%s(status %d)\nwithout it:\n%s(status %d)", short, shortStatus, full, fullStatus)
	}
	if fullTaken != 0 {
		t.Errorf("the route ran %d declaration lines with it off; want none", fullTaken)
	}
	if least.valued < 0 && valued != 0 {
		t.Errorf("the route ran %d declaration lines with a value; want none here", valued)
	}
	if taken < least.lines || valued < least.valued {
		t.Errorf("the route ran %d declaration lines, %d with a value; want at least %d and %d",
			taken, valued, least.lines, least.valued)
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
	// route must run, and how many of those carry a `name=$param`, counted:
	// a line holding any other value is left to simpleCommand. names is how
	// many hold names alone, which is what the route runs where operands are
	// stored in turn.
	scripts := []struct {
		name, src string
		rows      []string
		least     routeBar
		names     int
	}{
		{"shadow", emptyShadowScript + emptyShadowGlobalLines, emptyShadowRows, routeBar{18, 1}, 17},
		{"line", plainLineScript, plainLineRows, routeBar{10, 6}, 4},
		{"command", declarationCommandScript, declarationCommandRows, routeBar{13, 4}, 9},
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
							if vname == "stored in turn" {
								// Where an operand is stored in turn, a later
								// one on the line reads it: `oc=$ob` is `in`.
								atLeast = routeBar{sc.names, -1}
								rows = slices.DeleteFunc(slices.Clone(rows), func(r string) bool {
									return r == "[in][outer]\n"
								})
							}
							if vname == "empty open" {
								// The first valueless operand stops the
								// plain line's script on the question.
								atLeast, rows = routeBar{min(atLeast.lines, 1), 0}, nil
							}
							declarationCommandCompare(t, prog, sem, vname == "inherits outer", line, atLeast, rows)
						})
					}
				}
			}
		}
	}
}

// routeBar is how many declaration lines a script must send down the route,
// and how many of those with a `name=$param` operand: -1 for none at all.
type routeBar struct{ lines, valued int }

// TestAReservedWordDeclarationLineAnswersAsSimpleCommandDoes is the same
// comparison over declarationCommandScript parsed where `local`, `typeset` and
// `declare` are reserved words, so a line with an assignment operand is one
// the reserved word runs — and `$_`, where the dialect says so, stops at the
// word in front of that operand (#6373).
func TestAReservedWordDeclarationLineAnswersAsSimpleCommandDoes(t *testing.T) {
	d := syntax.Core()
	d.DeclarationReservedWords = map[string]bool{"local": true, "typeset": true, "declare": true}
	for _, word := range []string{"local", "typeset", "declare"} {
		prog, err := syntax.Parse(strings.ReplaceAll(declarationCommandScript, "local ", word+" "), d)
		if err != nil {
			t.Fatal(err)
		}
		v := declarationCommandVectors()
		stops := v["underscore"]
		stops.UnderscoreStopsAtADeclarationsAssignment = Yes
		for vname, sem := range map[string]Semantics{"base": v["base"], "underscore stops": stops} {
			sem.DeclareOptions = "aAgilnprux"
			sem.TypesetLocalNeedsKeywordFunction = No
			t.Run(word+"/"+vname, func(t *testing.T) {
				declarationCommandCompare(t, prog, sem, false, true, routeBar{13, 4}, declarationCommandRows)
			})
		}
	}
}
