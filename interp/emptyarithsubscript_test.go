// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A subscript written with nothing between the brackets is a *grammar*
// acceptance and a semantics answer, which is why it is tested here by axis
// and not by shell. `$(( a[$w] ))` with an empty `$w` is `$(( a[] ))` by the
// time the expression exists, because an arithmetic expansion substitutes its
// parameters before it parses — so this reaches far more scripts than write it.
func emptySub(p EmptyArithSubscriptPolicy) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.EmptyArithSubscript = p
		r.Semantics = &s
	}
}

func skipSubOfUnsetName(a Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.ArithSubscriptSkippedWhenNameUnset = a
		r.Semantics = &s
	}
}

// The three answers part on all of the value, the stream and whether the
// expression survives, which is what makes this an axis rather than a wording.
func TestAnEmptyArithmeticSubscriptAnswersByPolicy(t *testing.T) {
	for _, tc := range []struct {
		name   string
		p      EmptyArithSubscriptPolicy
		src    string
		want   string
		status int
	}{
		// The empty expression is zero, so the operand is the element that
		// subscript names — not the number zero. `a[]` on a five-first array
		// is five.
		{
			"the empty expression names element zero",
			EmptyArithSubscriptIsTheEmptyExpression,
			`a=(5 6 7); echo $(( a[] )); echo after`,
			"5\nafter\n", 0,
		},
		// Reported: the subscript is named and the operand is a flat zero,
		// which is the row that tells this policy from the one above.
		{
			"reported, and then a flat zero",
			EmptyArithSubscriptIsReported,
			`a=(5 6 7); echo $(( a[] )); echo after`,
			"sh: a[]: bad array subscript\n0\nafter\n", 0,
		},
		// Invalid: no value at all, and the sentence is the subscript's own
		// rather than the expression parser's.
		{
			"invalid, and the expression produces nothing",
			EmptyArithSubscriptIsInvalid,
			`a=(5 6 7); echo $(( a[] )); echo after`,
			"sh: invalid subscript\n", 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, emptySub(tc.p))
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q (status %d), want %q at %d",
					tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}

// No answer is refused by name. One shell prints and continues, one prints and
// stops, one prints nothing — no two of those can stand in for each other, so
// there is nothing to fall back on.
func TestAnEmptyArithmeticSubscriptUnansweredIsRefused(t *testing.T) {
	out, st := run(t, `echo $(( a[] )); echo after`, emptySub(EmptyArithSubscriptUnspecified))
	if !strings.Contains(out, "a subscript written with nothing in it") ||
		!strings.Contains(out, "no dialect was chosen") {
		t.Errorf("got %q, want the axis named in the refusal", out)
	}
	if strings.Contains(out, "after") || st == 0 {
		t.Errorf("got %q (status %d), want the input abandoned", out, st)
	}
}

// A write through brackets with nothing in them stores nothing unless the
// dialect reads them as the empty expression. Without the guard the write
// falls through to the *bare name*, so `(( a[]++ ))` would quietly overwrite
// the array with a number.
func TestAnEmptyArithmeticSubscriptDoesNotWriteThroughTheBareName(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    EmptyArithSubscriptPolicy
		want string
	}{
		{"the empty expression steps element zero", EmptyArithSubscriptIsTheEmptyExpression, "6 6 7"},
		{"reported leaves it alone", EmptyArithSubscriptIsReported, "5 6 7"},
		{"invalid leaves it alone", EmptyArithSubscriptIsInvalid, "5 6 7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `a=(5 6 7); (( a[]++ )); printf "%s" "${a[*]}"; echo`
			out, _ := run(t, src, emptySub(tc.p))
			if got := lastLine(out); got != tc.want {
				t.Errorf("%s left %q, want %q", src, got, tc.want)
			}
		})
	}
}

// Whether a subscript is read at all when the name in front of it is not set
// is its own axis, and it is only visible where the subscript errors or
// assigns: the value is the same zero either way, which is why a probe on the
// value alone could not tell the two apart.
func TestASubscriptOfAnUnsetNameIsSkippedByPolicy(t *testing.T) {
	for _, tc := range []struct {
		name   string
		a      Answer
		src    string
		want   string
		status int
	}{
		{
			"skipped: the subscript never divides",
			Yes, `echo $(( nodecl[1/0] )); echo after`, "0\nafter\n", 0,
		},
		{
			"skipped: the subscript never steps",
			Yes, `i=0; echo $(( nodecl[i++] )); echo "i=$i"`, "0\ni=0\n", 0,
		},
		{
			"read: the subscript divides",
			No, `echo $(( nodecl[1/0] )); echo after`, "sh: division by zero\n", 2,
		},
		{
			"read: the subscript steps",
			No, `i=0; echo $(( nodecl[i++] )); echo "i=$i"`, "0\ni=1\n", 0,
		},
		// Set-ness and not emptiness: a scalar holding nothing is there, and
		// the brackets are read.
		{
			"a name set to the empty string is there",
			Yes, `e=; echo $(( e[1/0] )); echo after`, "sh: division by zero\n", 2,
		},
		// And so is an array declared with nothing in it, which no plain read
		// of the name would have found.
		{
			"an array declared empty is there",
			Yes, `typeset -a a; echo $(( a[1/0] )); echo after`, "sh: division by zero\n", 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, skipSubOfUnsetName(tc.a))
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q (status %d), want %q at %d",
					tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}

// The two axes meet on the row the whole thing was found on: an empty
// subscript on a name nothing declared. Where the name is looked up first the
// brackets are never reached, so the operand is the plain unset zero and the
// empty-subscript answer never runs — which is what a probe expecting zero
// cannot tell from an answer that reported and then produced zero anyway.
func TestAnEmptySubscriptOnAnUnsetNameIsTheUnsetOperand(t *testing.T) {
	setup := func(r *Runner) {
		s := *r.Semantics
		s.ArithSubscriptSkippedWhenNameUnset = Yes
		s.EmptyArithSubscript = EmptyArithSubscriptIsInvalid
		r.Semantics = &s
	}
	for _, src := range []string{
		`echo $(( nodecl[] )); echo after`,
		`echo $(( nodecl[] + 1 - 1 )); echo after`,
	} {
		out, st := run(t, src, setup)
		if !strings.HasPrefix(out, "0\nafter\n") || st != 0 {
			t.Errorf("%s = %q (status %d), want a quiet zero", src, out, st)
		}
	}
	// And the same name, once it exists, takes the refusal — in every store
	// a name can live in. An associative array declared and never written to
	// is the row a plain read of the name cannot answer, and an indexed one
	// declared empty is the other; both are *there*, and an existence test
	// that only asked the scalar reader would call them missing and hand back
	// a quiet zero.
	for _, src := range []string{
		`nodecl=; echo $(( nodecl[] )); echo after`,
		`typeset -A m; echo $(( m[] )); echo after`,
		`typeset -a a; echo $(( a[] )); echo after`,
		`s=hello; echo $(( s[] )); echo after`,
	} {
		out, st := run(t, src, setup)
		if out != "sh: invalid subscript\n" || st != 2 {
			t.Errorf("%s = %q (status %d), want the refusal at 2", src, out, st)
		}
	}
}

// The grammar carries the empty pair where a value is *written* too, and the
// axis answers it there in a sentence of its own.
//
// It used to be a parse error in this one position, on the grounds that the
// shells part over the write as well as over the read. They do — and each of
// the three writes is that shell's own answer to the read wearing a different
// sentence, which is what makes it one axis and two wordings rather than a
// refusal. Measured 2026-09-12, `-c`, with `a=(5 6 7)`:
//
//	(( a[] = 4 ))
//
//	zsh 5.9.2      `not an identifier: a[]`, the array untouched, (( )) at 2
//	bash 5.3.15    `` `a[]': not a valid identifier ``, untouched, and the
//	               (( )) at **0** — the expression keeps its value and only
//	               the store is dropped, which `(( a[] = 0 ))` at 1 and
//	               `x=$(( a[] = 4 ))` giving 4 are what show
//	ksh93u+        silent at 0, and the array is `4 6 7` — element zero,
//	               which is the element the empty expression names
//
// Checked by running rather than by parsing, because an expression inside
// `(( ))` is read after its parameters have gone in — which is the whole
// reason `a[$w]` can arrive here as `a[]` at all (#1764).
func TestAnEmptyArithmeticSubscriptAsAnAssignmentTarget(t *testing.T) {
	for _, tc := range []struct {
		name  string
		p     EmptyArithSubscriptPolicy
		src   string
		left  string
		says  string
		quiet bool
	}{
		{
			name: "the empty expression writes element zero",
			p:    EmptyArithSubscriptIsTheEmptyExpression,
			src:  `a=(5 6 7); (( a[] = 4 )); s=$?; printf "[%s]" "${a[*]}"; echo " st=$s"`,
			left: "[4 6 7] st=0", quiet: true,
		},
		{
			name: "and the compound operator through the same element",
			p:    EmptyArithSubscriptIsTheEmptyExpression,
			src:  `a=(5 6 7); (( a[] += 4 )); s=$?; printf "[%s]" "${a[*]}"; echo " st=$s"`,
			left: "[9 6 7] st=0", quiet: true,
		},
		{
			// The row the bare-name branch used to fail: writing through the
			// name would leave a one-element array holding the number.
			name: "and never through the bare name",
			p:    EmptyArithSubscriptIsTheEmptyExpression,
			src:  `a=(5 6 7); (( a[] = 4 )); printf "%d" "${#a[@]}"; echo`,
			left: "3", quiet: true,
		},
		{
			name: "reported, with the store dropped and the value kept",
			p:    EmptyArithSubscriptIsReported,
			src:  `a=(5 6 7); (( a[] = 4 )); s=$?; printf "[%s]" "${a[*]}"; echo " st=$s"`,
			left: "[5 6 7] st=0", says: "not a valid identifier",
		},
		{
			name: "reported, and a zero value still fails the command",
			p:    EmptyArithSubscriptIsReported,
			src:  `a=(5 6 7); (( a[] = 0 )); s=$?; printf "[%s]" "${a[*]}"; echo " st=$s"`,
			left: "[5 6 7] st=1", says: "not a valid identifier",
		},
		{
			name: "invalid, where the expression fails outright",
			p:    EmptyArithSubscriptIsInvalid,
			src:  `a=(5 6 7); (( a[] = 4 )); printf "[%s]" "${a[*]}"; echo " st=$?"`,
			says: "not an identifier: a[]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, emptySub(tc.p))
			if tc.left != "" && !strings.Contains(out, tc.left) {
				t.Errorf("%s = %q, want %q in it", tc.src, out, tc.left)
			}
			if tc.says != "" && !strings.Contains(out, tc.says) {
				t.Errorf("%s = %q, want %q in it", tc.src, out, tc.says)
			}
			if tc.quiet && strings.Contains(out, "identifier") {
				t.Errorf("%s = %q, want nothing said", tc.src, out)
			}
		})
	}
}

// The wording is what does not carry over from the read, which is why there
// is a second Diagnostics field: the same shell says one thing of a read and
// another of a write.
func TestTheEmptyTargetAndTheEmptyReadAreWordedApart(t *testing.T) {
	const src = `a=(5 6 7); echo "r=$(( a[] ))"; (( a[] = 4 )); echo after`
	out, _ := run(t, src, func(r *Runner) {
		sem := testSemantics()
		sem.EmptyArithSubscript = EmptyArithSubscriptIsReported
		r.Semantics = &sem
		d := CoreDiagnostics()
		d.ArithEmptySubscript = "%[1]s[]: bad array subscript"
		d.ArithEmptySubscriptTarget = "`%[1]s[]': not a valid identifier"
		r.Diagnostics = &d
	})
	if !strings.Contains(out, "a[]: bad array subscript") {
		t.Errorf("%s = %q, want the read's own sentence", src, out)
	}
	if !strings.Contains(out, "`a[]': not a valid identifier") {
		t.Errorf("%s = %q, want the write's own sentence", src, out)
	}
	// Both sides reported and the script ran on, which is what the reporting
	// answer means on either of them.
	if !strings.Contains(out, "after") {
		t.Errorf("%s = %q, want the script running on", src, out)
	}
}
