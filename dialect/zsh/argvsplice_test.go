// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `argv[N]=(p q)` splices the positional parameters, exactly as the same
// spelling splices a named array.
//
// `argv` is this shell's name for the parameters and an ordinary array in
// every other respect — see dialect/zsh/argv.go — so an array literal through
// a subscript on it is the construct #1411 built, reached through the one name
// whose elements are produced rather than stored. It refused, at status 1 and
// with `attempt to assign array value to non-array`, which is the sentence a
// *scalar* gets: the question "is this name an array" was asked of the array
// table, where a produced name has nothing.
//
// It is not a corner. zsh's own `ztst.zsh` normalizes `tail -1` into `tail -n
// 1` with `argv[$argi]=(-n ${argv[$argi][2,-1]})`, so every `tail` its test
// suite runs wrote the diagnostic and produced nothing (#4614).
//
// Measured 2026-09-26 against zsh 5.9.2 (aarch64-apple-darwin25.4.0), run
// `-f`; `go version -m` says *not a Go executable* for it.
func TestAnArrayLiteralThroughASubscriptOnTheParametersSplices(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The four rows the report was filed with, and the count with each,
		// because the count is what makes this a splice rather than a write.
		{`set -- x y z; argv[2]=(p q); print -r -- "st=$? n=$# [$argv[*]]"`, "st=0 n=4 [x p q z]"},
		{`set -- x y z; argv[2]=(p); print -r -- "st=$? n=$# [$argv[*]]"`, "st=0 n=3 [x p z]"},
		{`set -- x y z; argv[2]=(); print -r -- "st=$? n=$# [$argv[*]]"`, "st=0 n=2 [x z]"},
		{`set -- x y z; argv[2,3]=(p q r); print -r -- "st=$? n=$# [$argv[*]]"`, "st=0 n=4 [x p q r]"},
		// A range that grows and one that shrinks, since a span whose ends
		// come to one subscript is the single subscript and would hide both.
		{`set -- x y z; argv[2,2]=(a b c); print -r -- "n=$# [$argv[*]]"`, "n=5 [x a b c z]"},
		{`set -- x y z; argv[1,3]=(q); print -r -- "n=$# [$argv[*]]"`, "n=1 [q]"},
		{`set -- x y z; argv[2,3]=(); print -r -- "n=$# [$argv[*]]"`, "n=1 [x]"},
		// Past the end pads with empty parameters and then places, which is
		// what the scalar spelling already does.
		{`set -- x y z; argv[5]=(p q); print -r -- "n=$# [$argv[4]][$argv[5]]"`, "n=6 [][p]"},
		{`set -- x y z; argv[6,7]=(p); print -r -- "n=$# [$argv[6]]"`, "n=6 [p]"},
		// A negative subscript counts back from the last parameter, and past
		// the first puts the words in front of every one there is.
		{`set -- x y z; argv[-1]=(p q); print -r -- "n=$# [$argv[*]]"`, "n=4 [x y p q]"},
		{`set -- x y z; argv[-4]=(p); print -r -- "n=$# [$argv[*]]"`, "n=4 [p x y z]"},
		// `+=` keeps the parameter and puts the words after it, which is the
		// distinction a subscript draws against `argv+=(p q)`.
		{`set -- x y z; argv[2]+=(p q); print -r -- "n=$# [$argv[*]]"`, "n=5 [x y p q z]"},
		// The subscript is an expression, and `ztst.zsh`'s own line is one.
		{`set -- -1 foo; argi=1; argv[$argi]=(-n ${argv[$argi][2,-1]}); print -r -- "n=$# [$argv[*]]"`, "n=3 [-n 1 foo]"},
		// Inside a function, where the parameters are the call's own and the
		// caller's are untouched by the write.
		{`f() { argv[2]=(p q); print -r -- "in n=$# [$argv[*]]" }; set -- A B; f x y z; print -r -- "out n=$# [$argv[*]]"`, "in n=4 [x p q z]\nout n=2 [A B]"},
		// A declaration's operand reaches the same place.
		{`set -- x y z; typeset argv[2]=(p q); print -r -- "n=$# [$argv[*]]"`, "n=4 [x p q z]"},
		// And the *scalar* spelling of a range, which is the same splice
		// carrying one word. It is here rather than in a file of its own
		// because the two spellings answering differently is the whole shape
		// of this defect: a range over a produced name that was not read as
		// an array became a span of **characters** of the joined parameters.
		{`set -- x y z; argv[2,3]=X; print -r -- "n=$# [$argv[*]]"`, "n=2 [x X]"},
		{`set -- x y z; argv[1,2]=X; print -r -- "n=$# [$argv[*]]"`, "n=2 [X z]"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

// `$@` and `$argv` are two spellings of one array, so a write through the
// subscripted one is visible through every reading of the other.
//
// Worth its own rows because the splice writes through a producer's writer
// rather than into the array table: a write that had landed in the table would
// read back correctly under `$argv`, which the producer answers ahead of, and
// not at all under `$@`, `$1` or `$#`.
func TestASpliceOfTheParametersIsVisibleThroughEverySpellingOfThem(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`set -- x y z; argv[2]=(p q); print -r -- "[$@] [$*] 1=$1 2=$2 3=$3 4=$4"`, "[x p q z] [x p q z] 1=x 2=p 3=q 4=z"},
		{`set -- x y z; argv[2]=(p q); print -r -- "[${@[2]}][${@[3]}] [${argv[2]}]"`, "[p][q] [p]"},
		{`set -- x y z; argv[2]=(p q); shift; print -r -- "n=$# [$argv[*]]"`, "n=3 [p q z]"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}

// The boundaries, which the parameters share with a named array because they
// are the same construct: subscripts count from one, so zero is below the
// first and refused in the same sentence, whatever `$0` is.
//
// `$0` is not one of the parameters — `set -- a b` leaves `$#` at 2 — so there
// is nothing at subscript zero for a literal to replace. The refusal is the
// one `a[0]=(p q)` gets and not the `attempt to assign array value to
// non-array` this used to give, which said the wrong thing about the name.
func TestASpliceOfTheParametersRefusesASubscriptBelowTheFirst(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`set -- x y z; argv[0]=(p q); echo after`, "argv: assignment to invalid subscript range"},
		{`set -- x y z; argv[0,0]=(p); echo after`, "argv: assignment to invalid subscript range"},
	} {
		out, _ := answersRun(t, tc.src)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s:\n  said %q\n  want it to contain %q", tc.src, out, tc.want)
		}
		if strings.Contains(out, "after") {
			t.Errorf("%s: the rest of the line ran; the refusal should end the script", tc.src)
		}
	}
}

// `ksharrays` moves where the splice lands and nothing else about it, because
// the letter answers what a subscript *means* and the splice asks that at the
// edge where a script wrote a number.
//
// Measured in the same run. The parameters are indexed from zero under the
// option — `argv[0]` is `$1` — so every row is the unoptioned one shifted by a
// subscript, and the refused subscript moves with it: below the first is `-4`
// rather than `0`, and zero is an ordinary parameter.
func TestKshArraysMovesWhereASpliceOfTheParametersLands(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`setopt ksharrays; set -- x y z; argv[0]=(p q); print -r -- "n=$# [${argv[*]}]"`, "n=4 [p q y z]"},
		{`setopt ksharrays; set -- x y z; argv[1]=(p q); print -r -- "n=$# [${argv[*]}]"`, "n=4 [x p q z]"},
		{`setopt ksharrays; set -- x y z; argv[2]=(p q); print -r -- "n=$# [${argv[*]}]"`, "n=4 [x y p q]"},
		{`setopt ksharrays; set -- x y z; argv[0,1]=(p q r); print -r -- "n=$# [${argv[*]}]"`, "n=4 [p q r z]"},
		// A negative subscript counts back from the end and asks nothing
		// about the base, so this row is the same under either.
		{`setopt ksharrays; set -- x y z; argv[-1]=(p q); print -r -- "n=$# [${argv[*]}]"`, "n=4 [x y p q]"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s:\n  said %q\n  want %q", tc.src, got, tc.want)
		}
	}
}
