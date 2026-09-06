// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// `++` and `--` write through a subscript, exactly as `=` and `+=` do.
//
// They did not: the increment took a bare name and nothing else, so
// `(( m[k]++ ))` was `++ needs a variable` in every dialect while
// `(( m[k] += 1 ))` was fine. Measured on bash 5.3, ksh93u+ and zsh 5.9.2:
// every one of these is the same answer in all three.
func TestAnOperatorWritesThroughASubscript(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"postfix on a key", `typeset -A m; m[k]=1; (( m[k]++ )); echo "${m[k]}"`, "2"},
		{"prefix on a key", `typeset -A m; m[k]=1; (( ++m[k] )); echo "${m[k]}"`, "2"},
		{"postfix decrement", `typeset -A m; m[k]=5; (( m[k]-- )); echo "${m[k]}"`, "4"},
		{"prefix decrement", `typeset -A m; m[k]=5; (( --m[k] )); echo "${m[k]}"`, "4"},
		// The two spellings differ in what they evaluate to, not in what
		// they do — the same distinction a bare name already made.
		{"postfix yields the old value", `typeset -A m; m[k]=1; echo "$(( m[k]++ )) ${m[k]}"`, "1 2"},
		{"prefix yields the new value", `typeset -A m; m[k]=1; echo "$(( ++m[k] )) ${m[k]}"`, "2 2"},
		// An indexed element takes the same operators, and the subscript
		// there is still an expression.
		{"postfix on an index", `a=(3 4 5); (( a[1]++ )); echo "${a[1]}"`, "5"},
		{"an expression subscript", `a=(3 4 5); i=0; (( a[i+1]++ )); echo "${a[1]}"`, "5"},
		// The element that is not there yet starts from nothing, which is
		// what a version counter written as `(( c[k]++ ))` relies on.
		{"an absent key starts at zero", `typeset -A m; (( m[q]++ )); echo "[${m[q]}]"`, "[1]"},
		{"an absent index starts at zero", `a=(3 4); (( a[5]++ )); echo "[${a[5]}]"`, "[1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := run(t, tc.src, nil); strings.TrimSpace(out) != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// An expression that names no storage still cannot be incremented: the
// operator reaching an element must not make it reach a literal.
func TestAnIncrementStillNeedsSomethingToWriteTo(t *testing.T) {
	for _, src := range []string{
		`(( 1++ ))`,
		`(( ++1 ))`,
	} {
		out, _ := run(t, src+`; echo "st=$?"`, nil)
		if !strings.Contains(out, "needs a variable") || !strings.Contains(out, "st=1") {
			t.Errorf("%s = %q, want a refusal at status 1", src, out)
		}
	}
}

// A declared associative name's subscript is a key inside an expression, the
// same reading `${m[k]}` takes.
//
// It was an arithmetic index there, so `$(( m[k] ))` read whatever element
// `k` evaluated to — 0 for an unset name — and an assignment through it
// stored under that number. Both were silent: the wrong element came back
// with no diagnostic at all.
func TestAnAssociativeSubscriptIsAKeyInsideAnExpression(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The case that tells the two readings apart: with a value under `k`
		// and a different one under `0`, an index would answer 99.
		{"a key is not an index", `typeset -A m; m[k]=7; m[0]=99; k=0; echo "$(( m[k] ))"`, "7"},
		{"the write lands on the key", `typeset -A m; m[k]=1; k=0; (( m[k]++ )); echo "k=${m[k]} zero=[${m[0]}]"`, "k=2 zero=[]"},
		{"a plain assignment", `typeset -A m; m[k]=1; (( m[k] = 9 )); echo "${m[k]}"`, "9"},
		{"a compound assignment", `typeset -A m; m[k]=1; (( m[k] += 5 )); echo "${m[k]}"`, "6"},
		{"a key that is an expression", `typeset -A m; m[1+1]=7; echo "$(( m[1+1] ))"`, "7"},
		// Whitespace is part of a key, in an expression exactly as in
		// `${m[ k ]}` — unanimous in the three shells with the attribute.
		{"whitespace belongs to the key", `typeset -A m; m[k]=7; echo "$(( m[ k ] ))"`, "0"},
		{"an absent key reads as zero", `typeset -A m; m[k]=7; echo "$(( m[nope] + 1 ))"`, "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := run(t, tc.src, nil); strings.TrimSpace(out) != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// An element reads as an operand by the rule a plain name reads by.
//
// It did not: a name holding a name was chased to a number and an element
// holding one was refused, so `y=5; a=(y); echo $(( a[0] ))` errored where
// bash and ksh93 answer 5. The storage an operand came out of is not what
// decides how it reads.
func TestAnElementIsReadAsAnOperandLikeAName(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an indexed element holding a name", `y=5; a=(y); echo "$(( a[0] ))"`, "5"},
		{"a key holding a name", `typeset -A m; y=5; m[k]=y; echo "$(( m[k] ))"`, "5"},
		{"an element holding a word that is no name", `a=(abc); echo "$(( a[0] ))"`, "0"},
		{"incrementing through the chase", `a=(abc); (( a[0]++ )); echo "[${a[0]}]"`, "[1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := run(t, tc.src, nil); strings.TrimSpace(out) != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
			}
		})
	}
}
