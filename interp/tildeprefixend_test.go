// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Where a tilde prefix **ends** is two questions, and this file holds both
// because getting either wrong produces the same symptom — a `~` that moved
// when it should not have, or stayed when it should have gone — from two
// different places.
//
// The first is unanimous and is core behavior. The second splits the panel
// six to one and is Semantics.TildePrefixStopsAtAQuoteOrAnExpansion, whose no
// answer lives beside the one dialect that takes it.

// A colon closes a tilde prefix inside an assignment's value, exactly as a
// slash does. Measured 2026-09-22, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with `HOME=/usr/xyz`:
//
//	foo=~:~     bash 5.3, bash as sh, bash 3.2, dash, zsh, ksh93 and
//	foo=~:x     BusyBox ash all expand the leading tilde; here it stayed
//	foo=~:      as written
//
// All seven columns, so there is nothing to answer — and it was wrong here
// anyway, because the leading tilde looked for a slash, found none, and asked
// the user database for a name of `:~`. The colon after it was expanded by
// the *other* road, which is what made the symptom a value one home short
// rather than a value with no homes in it: `foo=~:~` came to `~:/usr/xyz`.
//
// The two roads are the reason this is worth a test of its own. An assignment
// reaches a tilde through Runner.expandTildeIn for the one in front and
// Runner.expandColonTildes for the ones after each colon, and a rule written
// into one of them and not the other is a shell no column has (#4156).
//
// Runner.expandAssignValue names the colon set twice over — once itself and
// once through the wordTextUnsplit it ends with, which expands a leading tilde
// again for the callers that are not assignments. That redundancy predates
// this and is why a mutation has to move **both** to make these rows fail:
// either site alone carries the value, and the pair is the change.
//
// It is the assignment's value and not every word: whether a colon closes a
// prefix in an **ordinary** word is a separate question the panel splits three
// ways on — `echo ~:x` is the home in bash and ksh93 and the characters as
// written in dash, zsh and BusyBox ash, and bash alone declines it where the
// word holds a quote. That is Semantics.TildeColonEndsAnOrdinaryWordsPrefix and
// tildecolonword_test.go, answered in #4251; it is not this one.
func TestAColonClosesATildePrefixInAnAssignmentsValue(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a colon straight after the tilde", `foo=~:~; echo "$foo"`, "/h:/h"},
		{"a colon then ordinary text", `foo=~:x; echo "$foo"`, "/h:x"},
		{"a colon ending the value", `foo=~:; echo "$foo"`, "/h:"},
		{"a slash first, then a colon", `foo=~/a:~; echo "$foo"`, "/h/a:/h"},
		// The controls. A slash still closes it, a tilde with neither still
		// expands, and — the row that says this is the assignment road and
		// not a new rule about colons — an ordinary word is untouched.
		{"a slash closes it", `foo=~/a; echo "$foo"`, "/h/a"},
		{"nothing closes it", `foo=~; echo "$foo"`, "/h"},
		{"an ordinary word keeps its colon tilde", `echo x:~/m`, "x:~/m"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src+"\n", func(r *Runner) {
				sem := CoreSemantics()
				r.Semantics = &sem
				r.Vars = map[string]string{"HOME": "/h"}
			})
			if st != 0 {
				t.Fatalf("status = %d; out = %q", st, out)
			}
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// A tilde prefix carrying a quote or an expansion stops the expansion, and the
// word is the characters it was written as. Six of the seven columns — the
// standard says as much for the quoting half, XCU 2.6.1 giving the prefix to a
// login name only "if none of the characters in the tilde-prefix are quoted" —
// so it is the core's answer and the axis exists for the seventh.
//
// Measured 2026-09-22 against the panel; the rows and the one dissenter are in
// Semantics.TildePrefixStopsAtAQuoteOrAnExpansion, and dialect/zsh holds the
// no answer.
//
// Every case here is a word whose prefix **has not ended** by the time
// something that is not plain text arrives. That is the whole of the rule, and
// it is why the controls matter as much as the subjects: a prefix that ended
// at a slash first is past this and expands however the rest of the word is
// spelled (#4156).
func TestAQuoteOrAnExpansionInsideATildePrefixStopsIt(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a backslash straight after the tilde", `echo ~\chet/bar`, "~chet/bar"},
		{"a backslash inside the name", `echo ~ch\et/bar`, "~chet/bar"},
		{"a backslash on the closing slash", `echo ~\/bar`, "~/bar"},
		{"a quoted span inside the name", `echo ~"chet"/bar`, "~chet/bar"},
		{"a quoted span where the slash would be", `echo ~"/bar"`, "~/bar"},
		{"an expansion after the tilde", `u=root; echo ~$u`, "~root"},
		{"an expansion that begins with a slash", `x=/y; echo ~$x`, "~/y"},
		{"an expansion before the slash", `x=; echo ~$x/y`, "~/y"},
		{"a backslash on a named tilde", `echo ~\-`, "~-"},
		// The controls. Each of these has a prefix that ended in plain text
		// before anything else in the word, so the quoting past it is
		// somebody else's business.
		{"a quoted span past the slash", `echo ~/"bar"`, "/h/bar"},
		{"an expansion past the slash", `x=z; echo ~/$x`, "/h/z"},
		{"a plain prefix", `echo ~/bar`, "/h/bar"},
		{"a prefix that is the whole word", `echo ~`, "/h"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src+"\n", func(r *Runner) {
				sem := CoreSemantics()
				r.Semantics = &sem
				r.Vars = map[string]string{"HOME": "/h"}
			})
			if st != 0 {
				t.Fatalf("status = %d; out = %q", st, out)
			}
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// And a word merely *shaped* like an assignment is not a tilde context here.
// Six of the seven columns leave `make FOO=~/x` alone and the seventh does it
// only outside POSIX mode, so the core keeps the `~` — see
// Semantics.AnAssignmentShapedArgumentIsATildeContextOutsidePosixMode, whose
// yes answer lives beside the dialect that takes it (#4213).
func TestAnArgumentShapedLikeAnAssignmentKeepsItsTilde(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an argument", `echo make -k FOO=~/mumble`, "make -k FOO=~/mumble"},
		{"after a colon in one", `echo foo=~:~`, "foo=~:~"},
		// The control: a real assignment statement *is* a tilde context in
		// every column, and this test must not be read as saying otherwise.
		{"a real assignment", `FOO=~/mumble; echo "$FOO"`, "/h/mumble"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src+"\n", func(r *Runner) {
				sem := CoreSemantics()
				r.Semantics = &sem
				r.Vars = map[string]string{"HOME": "/h"}
			})
			if st != 0 {
				t.Fatalf("status = %d; out = %q", st, out)
			}
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
