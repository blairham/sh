// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// plainParamScript puts an unquoted `$name`, as a command's argument and as an
// assignment's value, in front of every state the shortcut must either answer
// exactly as the full road does or leave to it: separators in IFS, values
// that split or glob, unset and empty names, arrays of each kind, references,
// produced parameters with a side effect on every read, attributes folded on
// read, a function's local, noglob, and an unset name under `set -u`.
//
// Shapes a vector refuses are run through `command eval`, so a refusal is
// part of the output both runs must agree on and the script carries on.
const plainParamScript = `p() { printf '[%s]' "$@"; echo; }
i=5; p $i ${i}; x=$i; p "$x"
i=10; IFS=1; p $i; x=$i; p "$x"; [[ $i == 10 ]] && echo d10
IFS=; i=105; p $i
unset IFS; i=a.b_c-d+e,f:g/h; p $i
i='a b'; p $i; x=$i; p "$x"
i='/dev/nul?'; p $i
i='[a]'; p $i; i='x]'; p $i; i='['; p $i
i=; p $i; x=$i; p "$x"
unset i; p $i; x=$i; p "$x"
command eval 'a=(1 2 3)'; p $a; x=$a; p "$x"
command eval 'b=(7)'; p $b; x=$b; p "$x"
command eval 'typeset -A m; m[k]=v; m[j]=w'; p $m; x=$m; p "$x"
IFS=; p $a; p $m; x=$a; p "$x"; unset IFS
i=4; command eval 'typeset -n ref=i'; p $ref; x=$ref; p "$x"
command eval 'typeset -n el="a[1]"'; p $el; x=$el; p "$x"
command eval 'typeset -n whole="a[@]"'; p $whole
p $COUNT $COUNT; x=$COUNT; p "$x" $COUNT
p $SPACED; x=$SPACED; p "$x" $SPACED
p $LIST; x=$LIST; p "$x"
IFS=; p $LIST; x=$LIST; p "$x"; unset IFS
f() { local i=3; p $i; x=$i; p "$x"; }; f
command eval 'typeset -u up=abc'; p $up; x=$up; p "$x"
command eval 'typeset -i n=3+4'; p $n
set -f; i='/dev/nul?'; p $i; set +f
i=9; [ $i -lt 10 ] && echo lt; [[ $i -lt 10 ]] && echo dlt; [[ $i == 9 ]] && echo deq
i='/dev/nul?'; [[ $i == /dev/null ]] && echo dpat; [[ /dev/null == $i ]] && echo dglob
i=1; command eval 'a=(x y z)'; p ${a[$i]}; s=abcdef; p ${s:$i:2}; command eval 'p $(( a[$i] ))'
(set -u; p $nosuch); echo "<$?>"
set -- pos5 'a b' '' '/dev/nul?' '[x]'; p $1 ${1} $2 $3 $4 $5 $6; x=$1; p "$x"; x=$2; p "$x"; x=$6; p "$x"
IFS=o; p $1; x=$1; p "$x"; unset IFS
g() { local a=$1 b=$2 c=$3 d=$9; p "$a" "$b" "$c" "$d"; typeset e=$1; p "$e"; }; g pos1 'q r' ''
(set -u; p $7); echo "<$?>"
`

// plainParamVectors are the answers the script is run under: arrays read as
// their first element, and arrays read as the whole list, which is the pair of
// readings that decides whether a bare array name is a scalar at all.
func plainParamVectors() map[string]Semantics {
	base := testSemantics()
	// References and the case letters, which the substrate's own set leaves
	// out and the script reads through.
	base.DeclareOptions = "aAilnprux"
	first := base
	first.ArrayScalarIsTheWholeArray = No
	first.ArrayNameWithoutSubscriptIsTheList = No
	whole := base
	whole.ArrayScalarIsTheWholeArray = Yes
	whole.ArrayNameWithoutSubscriptIsTheList = Yes
	return map[string]Semantics{"first element": first, "whole array": whole}
}

// plainParamRunner is a runner under sem with three produced parameters a
// reading can be seen through: a count that moves on every read, one whose
// value splits, and a produced list.
func plainParamRunner(t *testing.T, sem Semantics, out *strings.Builder) *Runner {
	t.Helper()
	dg := PosixDiagnostics()
	r := newTestRunner(t, &Runner{Stdout: out, Stderr: out, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	count := 0
	r.SetDynamic("COUNT", func(*Runner) string { count++; return strings.Repeat("1", count) })
	r.SetDynamic("SPACED", func(*Runner) string { count++; return "s " + strings.Repeat("1", count) })
	r.SetDynamicArray("LIST", func(*Runner) []string { return []string{"l1", "l2"} })
	r.SetDynamicAssoc("TABLE", func(*Runner) AssocArray { return AssocArray{"0": {Str: "t"}} })
	return r
}

// TestAPlainParameterWordAnswersAsTheFullRoadDoes runs plainParamScript with
// the plain `$name` shortcut on and off, and requires the same output and
// status (#6353).
func TestAPlainParameterWordAnswersAsTheFullRoadDoes(t *testing.T) {
	f, err := syntax.Parse(plainParamScript, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	for name, sem := range plainParamVectors() {
		t.Run(name, func(t *testing.T) {
			run := func(on bool) (string, int) {
				// Both shortcuts at once: the full road is the chain with
				// neither the plain word nor the store's one-lookup read.
				was := SetPlainParamShortcutForTest(on)
				defer SetPlainParamShortcutForTest(was)
				wasStored := SetStoredScalarShortcutForTest(on)
				defer SetStoredScalarShortcutForTest(wasStored)
				var out strings.Builder
				r := plainParamRunner(t, sem, &out)
				st, err := r.Run(context.Background(), f)
				if err != nil {
					t.Fatalf("shortcut %v: %v", on, err)
				}
				return out.String(), st
			}
			full, fullStatus := run(false)
			short, shortStatus := run(true)
			if short != full || shortStatus != fullStatus {
				t.Errorf("with the shortcut:\n%s(status %d)\nwithout it:\n%s(status %d)",
					short, shortStatus, full, fullStatus)
			}
			// The rows the comparison leans on, so a script that stopped
			// early or a vector that refused the arrays compares nothing.
			for _, row := range []string{"[5][5]\n", "[][0]\n", "d10\n", "[1][11]\n", "[ABC]\n", "dlt\n", "deq\n", "[bc]\n", "parameter not set", "[pos5][pos5][a][b][/dev/null][[x]]", "[pos1][q r][][]\n"} {
				if !strings.Contains(full, row) {
					t.Errorf("the script did not reach %q:\n%s", row, full)
				}
			}
		})
	}
}

// TestThePlainParameterShortcutIsReached says the comparison above compares
// something: a plain scalar is answered by the shortcut, and the states it
// must decline are declined.
func TestThePlainParameterShortcutIsReached(t *testing.T) {
	t.Parallel()
	for name, sem := range plainParamVectors() {
		t.Run(name, func(t *testing.T) {
			var out strings.Builder
			r := plainParamRunner(t, sem, &out)
			word := func(src string) *syntax.Word {
				f, err := syntax.Parse(": "+src, syntax.Core())
				if err != nil {
					t.Fatal(err)
				}
				return f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd).Args[1]
			}
			setup := func(src string) {
				f, err := syntax.Parse(src, syntax.Core())
				if err != nil {
					t.Fatal(err)
				}
				if st, err := r.Run(context.Background(), f); st != 0 || err != nil {
					t.Fatalf("%q: %d %v %s", src, st, err, out.String())
				}
			}
			setup("i=5")
			setup("LATE=5; true")
			r.SetPipelineStatus("STATUSES")
			r.SetDeferredParameter("LATE")
			for _, w := range []string{"$i", "${i}"} {
				if v, ok := PlainParamWordForTest(r, word(w)); !ok || v != "5" {
					t.Errorf("%s holding 5: got %q, %v; want the shortcut's \"5\"", w, v, ok)
				}
			}
			// A positional parameter is read off the frame's list.
			setup("set -- 5 6 7 8 9 10 11 12 13 14")
			for _, w := range []string{"$1", "${1}", "${10}"} {
				want := "5"
				if w == "${10}" {
					want = "14"
				}
				if v, ok := PlainParamWordForTest(r, word(w)); !ok || v != want {
					t.Errorf("%s holding %s: got %q, %v; want the shortcut's %q", w, want, v, ok, want)
				}
			}
			for _, declined := range []struct{ setup, word string }{
				{"IFS=5", "$i"},
				{"unset IFS; i='a b'", "$i"},
				{"i='*'", "$i"},
				{"i=", "$i"},
				{"unset i", "$i"},
				{"i=5", "${i-x}"},
				{"i=5", "$i$i"},
				{"i=5", `"$i"`},
				{"i=5", "${#i}"},
				{"set -- 5", "$2"},
				{"set -- 5", "$0"},
				{"set -- 5", "${00}"},
				{"set -- 5", "$#"},
				{"set -- 'a b'", "$1"},
				{"set -- '*'", "$1"},
				{"set -- ''", "$1"},
				{"set -- 5", "${1-x}"},
				{"", "$COUNT"},
				{"", "$LIST"},
				{"", "$TABLE"},
				{"", "$STATUSES"},
				{"", "$LATE"},
				{"a=(1)", "$a"},
				{"typeset -A m; m[0]=1", "$m"},
				{"typeset -n ref=i", "$ref"},
			} {
				setup(declined.setup)
				if v, ok := PlainParamWordForTest(r, word(declined.word)); ok {
					t.Errorf("after %q, %s was answered %q by the shortcut", declined.setup, declined.word, v)
				}
			}
		})
	}
}
