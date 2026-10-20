// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"regexp"
	"strings"
	"testing"
)

// diagnosticLocation is the part of a diagnostic in front of its sentence,
// which these tests do not grade: the rows are about which lines are written
// and in what order.
var diagnosticLocation = regexp.MustCompile(`(?m)^[^+\n][^\n]*?: (warning: |[*@]: invalid)`)

func withoutLocations(s string) string {
	return diagnosticLocation.ReplaceAllString(s, "$1")
}

// A table literal under `set -x` is traced as the element writes it performs,
// and each line is written once its own element is stored, so the empty-key
// warning sits in front of that element's line and a refusal partway leaves
// the lines of the elements before it. Measured 2026-10-15 against ksh93u+
// 2012-08-01 from script files; every expected output is that shell's, less
// the location (#6683, #6684, #6686).
func TestATableLiteralIsTracedAsEachElementIsStored(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			"the empty key between two others",
			"typeset -A a\nset -x\na=([\"a b\"]=1 [\"\"]=3 [x]=y)",
			"+ a['a b']=1\nwarning: adding empty subscript\n+ a['']=3\n+ a[x]=y\n", 0,
		},
		{
			"the append spelling",
			"typeset -A a\nset -x\na+=([x]=1 [\"\"]=2)",
			"+ a[x]=1\nwarning: adding empty subscript\n+ a['']=2\n", 0,
		},
		{
			"a literal that makes the table",
			"set -x\nc=([\"\"]=1 [y]=2)",
			"warning: adding empty subscript\n+ c['']=1\n+ c[y]=2\n", 0,
		},
		{
			"beside a scalar on the same line",
			"typeset -A a\nset -x\nx=1 a=([\"\"]=2) y=3",
			"+ x=1\nwarning: adding empty subscript\n+ a['']=2\n+ y=3\n", 0,
		},
		{
			"an element holding an array",
			"typeset -A a\nset -x\na=([k]=(x y) [\"\"]=(p) [m]=1)",
			"+ a[k]=( x y )\nwarning: adding empty subscript\n+ a[]=( p )\n+ a[m]=1\n", 0,
		},
		{
			"the whole-array key refused partway",
			"typeset -A a\nset -x\na=([k]=1 [*]=s)\necho after",
			"+ a[k]=1\n*: invalid subscript in assignment\n", 1,
		},
		{
			// A literal whose keys are positions is placed whole, and its
			// lines follow the placement.
			"positions",
			"set -x\nq=(1)\nq=([2]=c [0]=a)",
			"+ q=( 1 )\n+ q[2]=c\n+ q[0]=a\n", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if got := withoutLocations(out); got != c.want || st != c.status {
				t.Errorf("got %q at %d\nwant %q at %d", got, st, c.want, c.status)
			}
		})
	}
}

// An element written with an array for its value is traced, from what was
// stored: the key as it resolved and unquoted, and the words quoted as values.
// Measured 2026-10-15 against ksh93u+ 2012-08-01 from a script file (#6684).
func TestAnElementHoldingAnArrayIsTraced(t *testing.T) {
	src := strings.Join([]string{
		"typeset -A a",
		"set -x",
		"a[k]=(x y)",
		"a[\"\"]=(x y)",
		"a[\"p q\"]=(\"1 2\" z)",
		"a[m]=()",
		"a[n]+=(w)",
		"a[n]+=(v)",
		"a[s]=str",
		"a[s]+=(q)",
		"x=(1)",
		"x[3]=(p q)",
		"typeset -p a x",
	}, "\n")
	want := "+ a[k]=( x y )\n" +
		"warning: adding empty subscript\n" +
		"+ a[]=( x y )\n" +
		"+ a[p q]=( '1 2' z )\n" +
		"+ a[n]+=( w )\n" +
		"+ a[n]+=( v )\n" +
		"+ a[s]=str\n" +
		"+ a[s]+=( q )\n" +
		"+ x=( 1 )\n" +
		"+ x[3]=( p q )\n" +
		"+ typeset -p a x\n" +
		"typeset -A a=(['']=(x y) [k]=(x y) [m]=() [n]=(w v) ['p q']=('1 2' z) [s]=(str q) )\n" +
		"typeset -a x=([0]=1 [3]=(p q) )\n"
	out, st := runKsh(t, t.TempDir(), src)
	if got := withoutLocations(out); got != want || st != 0 {
		t.Errorf("got %q at %d\nwant %q", got, st, want)
	}
}

// `[*]=` and `[@]=` written as a table literal's key end the script here, as
// `m[@]=Z` does; a key that only comes out `*` is an ordinary one. Measured
// 2026-10-15 against ksh93u+ 2012-08-01 from script files (#6686).
func TestAWholeArrayKeyInATableLiteralIsRefused(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"star", "typeset -A a\na=([k]=1 [*]=s)\necho after", "*: invalid subscript in assignment\n"},
		{"at", "typeset -A a\na=([k]=1 [@]=s)\necho after", "@: invalid subscript in assignment\n"},
		{"a declaration's literal", "typeset -A a=([*]=s)\necho after", "*: invalid subscript in assignment\n"},
		{"an append", "typeset -A a\na+=([*]=s)\necho after", "*: invalid subscript in assignment\n"},
		{"a literal that makes the table", "a=([1]=1 [*]=2)\necho after", "*: invalid subscript in assignment\n"},
		{"over an indexed name", "typeset -a a\na=([*]=2)\necho after", "*: invalid subscript in assignment\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if got := withoutLocations(out); got != c.want || st != 1 {
				t.Errorf("got %q at %d, want %q at 1", got, st, c.want)
			}
		})
	}
	for _, c := range []struct{ name, src, want string }{
		{"quoted", "typeset -A a\na=([\"*\"]=s [x]=1)\ntypeset -p a", "typeset -A a=(['*']=s [x]=1)\n"},
		{"expanded", "typeset -A a\nk='*'\na=([$k]=s)\ntypeset -p a", "typeset -A a=(['*']=s)\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q at %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// A literal that fails partway under `set -x` keeps what it stored before the
// failure, as the same literal untraced does. Measured 2026-10-15 against
// ksh93u+ 2012-08-01: `typeset -A a; a=([k]=1 [x]=$((1/0)))` leaves `[k]=1`
// traced or not, where the traced one here left an empty value under the
// empty key (#6683).
func TestAFailedTableLiteralKeepsWhatItStoredUnderTracing(t *testing.T) {
	out, _ := runKsh(t, t.TempDir(),
		"typeset -A a\ntrap 'set +x; typeset -p a' EXIT\nset -x\na=([k]=1 [x]=$((1/0)))")
	if !strings.HasSuffix(out, "typeset -A a=([k]=1)\n") || strings.Contains(out, "warning") {
		t.Errorf("got %q, want the listing `typeset -A a=([k]=1)` and no warning", out)
	}
}
