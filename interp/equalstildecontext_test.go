// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.TheFirstUnquotedEqualsInAWordOpensATildeContext is the widest of
// the three readings of `something=~` a word can get, and the reason it is an
// axis of its own rather than a second Yes on the assignment-shape one is
// that it **subsumes** that rule: where this is Yes, every word the shape
// would have claimed is claimed already, at the same `=`.
//
// **The noun is that one `=`.** Not "a word shaped like an assignment", and
// not "an `=` in the word" — the *first* one, written plainly. Two obvious
// misreadings agree with it nearly everywhere and the rows below are chosen to
// part them:
//
//   - reading the shape agrees on every `FOO=~` anybody writes, and differs
//     on `--opt=~`, `1abc=~` and `f.g=~`;
//   - reading the **last** `=` agrees on every word with one `=` in it, and
//     differs on `a=b=~`, which stays as written because the value the first
//     `=` opens is `b=~` and its tilde stands at neither the head nor a colon.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// `/opt/homebrew/bin/zsh` — `go version -m` reports *not a Go executable*, so
// the reference is not this program — under `-f` with `HOME=/Users/testhome`,
// 2026-09-25, `setopt magicequalsubst` against `unsetopt magicequalsubst`. No
// column answers this from its startup state; it is zsh's MAGIC_EQUAL_SUBST
// and nothing else, which is why the answer is read at the word.
func TestTheFirstUnquotedEqualsOpensATildeContext(t *testing.T) {
	for _, tc := range []struct{ name, src, wide, off string }{
		// The shape's own rows. The core answers the shape No, so these move
		// on the wide answer alone — which is the subsumption written down
		// on the axis: a Yes here does not go on to consult the shape.
		{"a name", `echo FOO=~/m`, "FOO=/h/m", "FOO=~/m"},
		{"an append", `echo FOO+=~/m`, "FOO+=/h/m", "FOO+=~/m"},
		// And the rows the shape could never have claimed. These are what say
		// the shape is not the noun.
		{"a long option", `echo --opt=~/m`, "--opt=/h/m", "--opt=~/m"},
		{"a name starting with a digit", `echo 1abc=~`, "1abc=/h", "1abc=~"},
		{"a dot in the name", `echo f.g=~`, "f.g=/h", "f.g=~"},
		{"a colon in the name", `echo a:b=~`, "a:b=/h", "a:b=~"},
		{"a slash in the name", `echo a/b=~`, "a/b=/h", "a/b=~"},
		// **The first `=` and not the last.** Held still while everything
		// around it moves: the same word with a colon in front of the second
		// tilde does expand, through that same first `=`.
		{"a second equals in the value", `echo a=b=~`, "a=b=~", "a=b=~"},
		{"a colon past the second equals", `echo a=b=~:~`, "a=b=~:/h", "a=b=~:~"},
		// The value is the assignment's value: head, colons, and a `~` that
		// opens neither is left alone.
		{"a colon segment", `echo PATH=a:~/b`, "PATH=a:/h/b", "PATH=a:~/b"},
		{"two segments", `echo a=~:~`, "a=/h:/h", "a=~:~"},
		{"a tilde that opens nothing", `echo a=x~`, "a=x~", "a=x~"},
		// A colon-tilde in front of the `=` moves once the word qualifies and
		// never on its own — the pair, not either row alone.
		{"a colon tilde before the equals", `echo a:~/b=~`, "a:/h/b=/h", "a:~/b=~"},
		{"the same word with nothing to qualify it", `echo a:~/b=c`, "a:~/b=c", "a:~/b=c"},
		// **The `=`'s own quoting decides, not the word's and not the
		// name's.** Three rows holding the `=` in place and moving quotes
		// around it.
		{"a quoted equals", `echo a'='~`, "a=~", "a=~"},
		{"a quoted name", `echo 'x-y'=~`, "x-y=/h", "x-y=~"},
		{"a quoted whole word", `echo 'a=~'`, "a=~", "a=~"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, w := range []struct {
				label  string
				answer Answer
				want   string
			}{{"wide", Yes, tc.wide}, {"off", No, tc.off}} {
				out, _ := run(t, tc.src+"\n", func(r *Runner) {
					sem := CoreSemantics()
					sem.TheFirstUnquotedEqualsInAWordOpensATildeContext = w.answer
					r.Semantics = &sem
					r.Vars = map[string]string{"HOME": "/h"}
				})
				if got := strings.TrimSpace(out); got != w.want {
					t.Errorf("%s under %s = %q, want %q", tc.src, w.label, got, w.want)
				}
			}
		})
	}
}

// The wide answer does not reach into an assignment statement or change what
// one already does, which is the control the rows above cannot carry: every
// one of them is an argument, and an argument moving while the statement
// stayed put would look the same as both moving.
func TestTheWideEqualsAnswerLeavesAnAssignmentStatementWhereItWas(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a plain assignment", `v=~; echo "$v"`, "/h"},
		{"a colon segment", `v=a:~/b; echo "$v"`, "a:/h/b"},
		{"a second equals", `v=b=~; echo "$v"`, "b=~"},
		{"a quoted value", `v='~'; echo "$v"`, "~"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, a := range []struct {
				label  string
				answer Answer
			}{{"wide", Yes}, {"narrow", No}} {
				out, st := run(t, tc.src+"\n", func(r *Runner) {
					sem := CoreSemantics()
					sem.TheFirstUnquotedEqualsInAWordOpensATildeContext = a.answer
					r.Semantics = &sem
					r.Vars = map[string]string{"HOME": "/h"}
				})
				if st != 0 {
					t.Fatalf("status = %d; out = %q", st, out)
				}
				if got := strings.TrimSpace(out); got != tc.want {
					t.Errorf("%s under %s = %q, want %q", tc.src, a.label, got, tc.want)
				}
			}
		})
	}
}

// The axis is asked **at the disagreement and nowhere else**: only once the
// word carries a tilde the answer could move. A question put to every `=` on
// a command line would be this axis refusing `cc -DX=1`, which is a shape
// every build in the world writes.
//
// Left unanswered on purpose, so that an ask is visible as a refusal rather
// than having to be inferred from an output that looks the same either way.
// That is also what makes this discriminating: with the gate removed the
// first row refuses, and with the gate widened to any `=` at all the second
// row would still pass, so both are needed.
func TestTheWideEqualsAxisIsAskedOnlyWhereATildeCouldMove(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		asks      bool
	}{
		{"an equals with no tilde behind it", `echo cc -DX=1`, false},
		{"an equals with a quoted tilde behind it", `echo cc -DX='~'`, false},
		{"a tilde that opens nothing", `echo cc -DX=a~b`, false},
		{"a tilde straight after the equals", `echo cc -DX=~/m`, true},
		{"a tilde after one of its colons", `echo cc -DX=a:~/m`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src+"\n", func(r *Runner) {
				sem := CoreSemantics()
				sem.TheFirstUnquotedEqualsInAWordOpensATildeContext = Answer(0)
				r.Semantics = &sem
				r.Vars = map[string]string{"HOME": "/h"}
			})
			asked := strings.Contains(out, "every word with an unquoted `=` in it being a tilde context")
			if asked != tc.asks {
				t.Errorf("%s: asked = %v, want %v; status %d, out %q", tc.src, asked, tc.asks, st, out)
			}
		})
	}
}
