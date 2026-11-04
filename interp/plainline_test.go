// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// plainLineScript is the lines a plain declaration line's route draws its
// edges at, beside the empty shadow's own script: a line that stops being
// plain partway through — at a frozen name, a set one, the shell's own, a
// name the line already declared or the call declared before — and hands the
// rest to the full road; a literal, a quoted value, a positional holding an
// `=`, an empty value; nested calls declaring the same name; a return from
// inside, early and late; the trace, `set -u`, `set -a`, the DEBUG and ERR
// traps; a prefix on the call and on the word itself; `command` in front of
// the word; `--`, a bad name and an array literal on the line; letters on an
// earlier line; and the status a line leaves behind a failure.
//
// Shapes a vector may refuse are run through `command eval` or a subshell,
// so a refusal is part of the output both runs must agree on.
const plainLineScript = `p() { printf '[%s]' "$@"; echo; }
show() { for n; do typeset -p "$n" 2>&1 || echo "$n:absent"; done; }
command eval 'readonly fro=r'; fo=outer
mid() { local m1 m2=$1 fo m3 m4=lit; p "<${m1-unset}>" "<$m2>" "<${fo-unset}>" "<${m3-unset}>" "<$m4>"; show m1 m2 m3 fo m4; }
mid one; echo "mid $?"; show m1 m2 m3 fo m4
(ro() { local r1=$1 fro r2; echo "ro $?"; show r1 r2; }; ro x); echo "after ro $?"
dup() { local d1 d1=$1 d2=x d2; show d1 d2; }; dup v; show d1 d2
again() { local g1; local g2=$1 g1 g3; show g1 g2 g3; }; again w; show g1 g2 g3
lit() { local l1=$1 l2=lit l3="$2" l4= l5=$3; p "$l1" "$l2" "$l3" "<$l4>" "$l5"; }; lit 'a b' 'c  d' 'x=y'
nest() { local n1=$1 n2; inner; p "$n1" "<${i1-unset}>" "<${n2-unset}>"; }
inner() { local i1=in n1; n1=changed; n2=reached; p "<$n1>" "$i1"; }
nest out; show n1 n2 i1
ret() { local r1=$1 r2; r2=set; return 3; }; ret x; echo "ret $? <${r1-unset}><${r2-unset}>"
early() { local e1=one; if [ "$1" = stop ]; then return 4; fi; local e2=two; p "$e1" "$e2"; }
early stop; echo "early $?"; early go; show e1 e2
traced() { set -x; local x1 x2=$1; set +x; p "$x2"; }; traced tv 2>&1
unsetu() { set -u; local u1 u2=$1; p "<$u2>"; set +u; }; unsetu uv
allexp() { set -a; local ax1 ax2=$1; set +a; case "$(export -p)" in *ax2=*) echo ax2-exported;; *) echo ax2-not;; esac; }
allexp av; show ax1 ax2
special() { local s1 IFS s2 OPTIND s3 PATH; show s1 s2 s3; p "<$IFS>"; }; special
typed() { command eval 'local -i ti1=3+4'; local ti1 tp1; show ti1 tp1; }; typed
(bad() { local b1 1bad b2; echo "bad $?"; show b1 b2; }; bad); echo "after bad $?"
st() { false; local s4 s5; echo "status $?"; (exit 3); local s6=$?; echo "kept $s6"; }; st
(dbg() { local db1 db2=$1; p "$db2"; }; command eval 'trap "echo dbg" DEBUG'; dbg dv; command eval 'trap - DEBUG')
(command eval 'trap "echo err" ERR'; et() { local et1=$(false) et2; p "<$et1>"; }; et)
prefixed() { local pf1 pf2; show pf1 pf2; pf1=in; }; pf1=pre pf2=two prefixed; show pf1 pf2
withprefix() { pw=1 local pq pw pr; show pq pw pr; }; withprefix; show pw
cmdlocal() { command local cl1=c cl2; show cl1 cl2; }; cmdlocal; show cl1 cl2
dashdash() { local -- dd1 dd2=v; show dd1 dd2; }; dashdash
arr() { command eval 'local ar1=(1 2) ar2'; show ar1 ar2; }; arr; show ar1 ar2
words() { w='1x'; v='wz=1'; local $v "$w" 2>&1; echo "words $?"; show wz; }; (words)
evald() { eval 'local ev1=$1 ev2'; show ev1 ev2; }; evald ee; show ev1 ev2
(opts() { local o1 -x o2; echo "opts $?"; show o1 o2; }; opts); echo "after opts $?"
quoted() { local qa='(x y)' qb="(z)"; show qa qb; }; quoted
(command eval 'set -r'; rs() { local rs1 rs2=$1 PATH rs3; echo "rs $?"; show rs1 rs2 rs3; }; rs x); echo "after rs $?"
p plain-end
`

// plainLineRows are lines plainLineScript writes from its first call to its
// last, which the full road must write for a comparison to have compared it.
var plainLineRows = []string{"[<one>]", "ret 3 <unset><unset>\n", "early 4\n[one][two]\n", "[out]", "[plain-end]\n"}

// plainLineVectors are the empty shadow's vectors, the base with the DEBUG and
// ERR traps a line runs under, with `command` in front of the word answered
// both ways, and with a valueless declaration's empty left open.
func plainLineVectors() map[string]Semantics {
	v := emptyShadowVectors()
	traps := v["base"]
	traps.TrapHasDebugCondition, traps.TrapHasErrCondition = Yes, Yes
	traps.DebugTrapRunsBeforeTheCommand, traps.ErrTrapRunsInSubshells = Yes, Yes
	v["traps"] = traps
	// `command` in front of the word, answered both ways the panel splits.
	for name, nothing := range map[string]Answer{"command declares": No, "command declares nothing": Yes} {
		c := v["base"]
		c.CommandPrefixKeepsADeclaration, c.LocalThroughCommandDeclaresNothing = Yes, nothing
		v[name] = c
	}
	// And a declaration whose empty is a question left open, which the first
	// valueless operand asks and stops the line on.
	open := v["base"]
	open.DeclaredNameWithoutValueIsEmpty = Unspecified
	v["empty open"] = open
	return v
}

// plainLineBar is how many operands the route must take under the vector
// named, and the rows the full road must write: none taken where locals
// start as the outer binding, which the empty shadow leaves to the full road;
// and where a valueless declaration's empty is open, one taken and nothing
// written past the line it stops, since the first valueless operand stops the
// script there.
func plainLineBar(vector string, rows []string) (int, []string) {
	switch vector {
	case "inherits outer":
		return 0, rows
	case "empty open":
		return 1, nil
	}
	return 20, rows
}

// plainLineCompare runs prog with the route on and off and requires the same
// output and status, the route to have declared at least atLeast operands with
// it on (none where atLeast is 0) and none with it off, and the full road's
// output to hold every row.
func plainLineCompare(
	t *testing.T, prog *syntax.File, sem Semantics, inheritsOuter bool, atLeast int, rows []string,
) {
	t.Helper()
	run := declarationRouteRun(t, prog, sem, inheritsOuter, SetPlainLineShortcutForTest,
		func(r *Runner) (int, int) { return PlainLineOperandsForTest(r), 0 })
	full, fullStatus, fullTaken, _ := run(false)
	short, shortStatus, taken, _ := run(true)
	if short != full || shortStatus != fullStatus {
		t.Errorf("with the route:\n%s(status %d)\nwithout it:\n%s(status %d)", short, shortStatus, full, fullStatus)
	}
	switch {
	case fullTaken != 0:
		t.Errorf("the route took %d operands with it off; want none", fullTaken)
	case atLeast == 0 && taken != 0:
		t.Errorf("the route took %d operands; want none", taken)
	case taken < atLeast:
		t.Errorf("the route took %d operands; want at least %d", taken, atLeast)
	}
	for _, row := range rows {
		if !strings.Contains(full, row) {
			t.Errorf("the script did not reach %q:\n%s", row, full)
		}
	}
}

// TestAPlainLineAnswersAsTheFullRoadDoes runs the empty shadow's script and
// plainLineScript — `local`, then `typeset` and `declare` in both forms of
// function — with the route for a plain declaration line on and off, under
// every vector the empty shadow's own tests run, and requires byte-identical
// output and status, the route taken wherever a line makes locals and nowhere
// else (#6373).
func TestAPlainLineAnswersAsTheFullRoadDoes(t *testing.T) {
	scripts := []struct {
		name, src string
		rows      []string
	}{
		{"shadow", emptyShadowScript + emptyShadowGlobalLines, emptyShadowRows},
		{"line", plainLineScript, plainLineRows},
	}
	for _, sc := range scripts {
		local := declarationsSpelled(t, sc.src, "local", false)
		for name, sem := range plainLineVectors() {
			t.Run(sc.name+"/local/"+name, func(t *testing.T) {
				atLeast, rows := plainLineBar(name, sc.rows)
				plainLineCompare(t, local, sem, name == "inherits outer", atLeast, rows)
			})
		}
	}
	for _, sc := range scripts {
		for _, word := range []string{"typeset", "declare"} {
			for _, keyword := range []bool{false, true} {
				plainTypesetCompare(t, sc.name, sc.src, word, keyword, sc.rows)
			}
		}
	}
}

// plainTypesetCompare is TestAPlainLineAnswersAsTheFullRoadDoes for one script
// spelled with word, in one form of function: under every empty-shadow vector
// with a function of either form making locals, then with only a
// `function`-form body making them, and with that question left open.
func plainTypesetCompare(t *testing.T, name, src, word string, keyword bool, rows []string) {
	t.Helper()
	prog := declarationsSpelled(t, src, word, keyword)
	form := map[bool]string{false: "f()", true: "function f"}[keyword]
	for vname, sem := range plainLineVectors() {
		sem.DeclareOptions = "aAgilnprux"
		sem.TypesetLocalNeedsKeywordFunction = No
		t.Run(name+"/"+word+"/"+form+"/"+vname, func(t *testing.T) {
			atLeast, rows := plainLineBar(vname, rows)
			plainLineCompare(t, prog, sem, vname == "inherits outer", atLeast, rows)
		})
	}
	for _, kw := range []struct {
		name   string
		answer Answer
	}{{"keyword functions only", Yes}, {"keyword axis open", Unspecified}} {
		sem := emptyShadowVectors()["base"]
		sem.DeclareOptions = "aAgilnprux"
		sem.TypesetLocalNeedsKeywordFunction = kw.answer
		t.Run(name+"/"+word+"/"+form+"/"+kw.name, func(t *testing.T) {
			// Only a `function`-form body makes locals here, and an
			// open axis is left to the full road whichever form asks.
			atLeast := 0
			if keyword && kw.answer == Yes {
				atLeast = 20
			}
			rows := rows[:1]
			if !keyword && kw.answer == Unspecified {
				rows = []string{"needing a keyword-defined function to declare a local"}
			}
			plainLineCompare(t, prog, sem, false, atLeast, rows)
		})
	}
}
