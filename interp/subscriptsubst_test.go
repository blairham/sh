// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// runSubscriptSubst is run() with the grammar these rows need: a subscript
// holding a substitution, and the operators that read the value behind it.
func runSubscriptSubst(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ParamSubstitution = true
		d.ParamSubstring = true
		d.ParamIndirection = true
	}, nil)
}

// The counter, and why it is a command substitution with a side effect rather
// than an arithmetic one.
//
// #3244 established that an arithmetic counter **cannot see a reader that
// evaluates without moving**: `i++` counts the evaluations that increment, and
// a reader asking the same brackets a fifth time without incrementing is
// invisible to it. A substitution that writes a byte is seen by every reader,
// because running it *is* the side effect. Both are asserted here — the marks
// below and `i` in the arithmetic rows further down — because they fail for
// different reasons and #3122 records a shell where the two part company.
const subscriptMark = `$(printf . >&2; echo 1)`

// A subscript is expanded **once** per expansion of the brackets, not once per
// reader of them.
//
// #3104 gave a subscript one arithmetic evaluation per expansion, and the
// *word* expansion of the same brackets was not part of that: subscriptText,
// subscriptAsWritten and subscriptValue are separate roads to it and each
// reader walked one, so a command substitution written into the brackets ran
// four times for the plain reading and ten for the conditional ones — the
// values agreeing throughout, which left the count as the only tell (#3240).
//
// Measured 2026-09-16, `env -i` with a scratch HOME and no startup files, from
// a script file, with `a=(x y z)` and the subscript above. The number is the
// marks one expansion writes:
//
//	source          bash 5.3.20  bash 3.2.57  ksh93u+ 2012  zsh 5.9.2  before  after
//	${a[S]}                   1            1             1          1       4      1
//	${a[S]#y}                 1            2             1          1       6      1
//	${a[S]-D}                 1            1             1          1      10      1
//	${a[S]:-D}                1            1             1          1      10      1
//	${a[S]+X}                 1            1             1          1       4      1
//	${#a[S]}                  1            1             1          1       4      1
//	${a[S]/y/Z}               1            2             1          1       6      1
//	${a[S]:0:1}               1            2             1          1       7      1
//
// bash 3.2.57 is the one column that is not flatly 1, and the three rows it
// says 2 on are the ones that read the value a second time; that is a
// question of its own and not this one. dash and BusyBox ash have no arrays
// and refuse the line.
func TestASubscriptRunsItsSubstitutionOncePerExpansion(t *testing.T) {
	for _, tc := range []struct{ name, spec, want string }{
		{"a plain element", `${a[S]}`, "<y>"},
		{"a trim over the element", `${a[S]#y}`, "<>"},
		{"a word for an element that is there", `${a[S]-D}`, "<y>"},
		{"a word for an element that is there or empty", `${a[S]:-D}`, "<y>"},
		{"a word for an element that is set", `${a[S]+X}`, "<X>"},
		{"the length of the element", `${#a[S]}`, "<1>"},
		{"a replacement over the element", `${a[S]/y/Z}`, "<Z>"},
		{"a slice of the element", `${a[S]:0:1}`, "<y>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := strings.Replace(tc.spec, "S", subscriptMark, 1)
			out, st := runSubscriptSubst(t, `a=(x y z); printf '<%s>' "`+spec+`"`)
			marks := strings.Count(out, ".")
			if got := strings.ReplaceAll(out, ".", ""); got != tc.want {
				t.Errorf("%s came to %q, want %q", tc.spec, got, tc.want)
			}
			if marks != 1 {
				t.Errorf("%s ran its substitution %d times, want 1 — %q", tc.spec, marks, out)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// The other direction, which is the one a hold breaks: an expansion that
// happens again runs it again.
//
// A `for` loop expands one node once per pass, and every reference shell runs
// the substitution once per pass — three passes, three marks. A hold kept past
// the end of its expansion would answer the second pass with the first pass's
// value, which is the silent shape rather than a slow one.
func TestASubscriptRunsItsSubstitutionAgainOnEachExpansion(t *testing.T) {
	src := `a=(x y z); for k in 1 2 3; do printf '<%s>' "${a[` + subscriptMark + `]}"; done`
	out, st := runSubscriptSubst(t, src)
	if got := strings.ReplaceAll(out, ".", ""); got != "<y><y><y>" {
		t.Errorf("the loop came to %q, want %q", got, "<y><y><y>")
	}
	if marks := strings.Count(out, "."); marks != 3 {
		t.Errorf("three passes ran the substitution %d times, want 3 — %q", marks, out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// Two sets of brackets in one word are two expansions and two runs. The hold
// is keyed on the brackets, so a second expansion beside the first must not
// read the first one's answer — which is the same wrong-value failure the loop
// row guards, in the shape a single command can hold.
func TestTwoSubscriptsInOneWordRunTheirOwnSubstitutions(t *testing.T) {
	src := `a=(x y z); b=(P Q R); printf '<%s><%s>' "${a[` + subscriptMark +
		`]}" "${b[` + subscriptMark + `]}"`
	out, st := runSubscriptSubst(t, src)
	if got := strings.ReplaceAll(out, ".", ""); got != "<y><Q>" {
		t.Errorf("two subscripts came to %q, want %q", got, "<y><Q>")
	}
	if marks := strings.Count(out, "."); marks != 2 {
		t.Errorf("two subscripts ran %d substitutions, want 2 — %q", marks, out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The arithmetic half, which is where the repeated expansion was not only a
// count.
//
// `$(( … ))` written into a subscript is a span of the subscript word, so it
// was evaluated once per reader like any other — and an expression that moves
// left the variable somewhere no shell leaves it and then read the wrong
// element with it. Measured the same day: bash 5.3.20 and ksh93u+ 2012 both
// answer `x` with `i` at 1 for every row below, and this shell answered `D`
// with `i` at **10** for the first of them.
//
// The counter here is the variable rather than a mark, which is the pair
// #3244's lesson asks for: a reader that evaluates without moving is invisible
// to this one and visible to the marks above, and a reader that moves without
// running a command is the other way round.
func TestAnArithmeticSubstitutionInASubscriptIsEvaluatedOnce(t *testing.T) {
	for _, tc := range []struct{ name, spec, want string }{
		{"a word for an element that is there", `${a[$((i++))]-D}`, "<x>"},
		{"a plain element", `${a[$((i++))]}`, "<x>"},
		{"a trim over the element", `${a[$((i++))]#x}`, "<>"},
		{"a word for an element that is set", `${a[$((i++))]+X}`, "<X>"},
		{"the length of the element", `${#a[$((i++))]}`, "<1>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `a=(x y z); i=0; printf '<%s>' "` + tc.spec + `"; printf ' i=%s' "$i"`
			out, st := runSubscriptSubst(t, src)
			want := tc.want + " i=1"
			if out != want {
				t.Errorf("%s: got %q, want %q", tc.spec, out, want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// A substitution in a subscript that itself expands brackets with a
// substitution in them: two runs, one for each set, and neither reading the
// other's hold. The inner expansion opens a hold of its own and gives the
// outer one back, which is the only thing keeping the outer brackets from
// re-running once the inner had finished.
func TestASubscriptNestedInsideASubscriptRunsItsOwnSubstitution(t *testing.T) {
	src := `a=(x y z); b=(P Q R); printf '<%s>' "${a[$(printf . >&2; echo "${b[` +
		subscriptMark + `]:+1}")]}"`
	out, st := runSubscriptSubst(t, src)
	if got := strings.ReplaceAll(out, ".", ""); got != "<y>" {
		t.Errorf("the nested subscript came to %q, want %q", got, "<y>")
	}
	if marks := strings.Count(out, "."); marks != 2 {
		t.Errorf("the nesting ran %d substitutions, want 2 — %q", marks, out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}
