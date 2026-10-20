// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// forHeaderDiagnostic renders what ksh93 says about a header it will not parse.
func forHeaderDiagnostic(t *testing.T, src string) string {
	t.Helper()
	_, err := syntax.Parse(src, ksh.Dialect())
	if err == nil {
		t.Fatalf("%s: parsed, want a refusal", src)
	}
	return ksh.Diagnostics().ParseDiagnostic("ksh", "", err, src)
}

// A C-style `for` header with fewer than two separators is refused, and this
// shell names the **closer** whatever the header held — so the sentence is the
// same for an empty header and for one holding an assignment, where the other
// two columns say something different for each.
//
// Measured 2026-09-12 on ksh93u+ 2012-08-01. `for ((;))` and `for ((1;))` are
// not here: that build takes SIGSEGV on a one-separator header whose last
// section is empty, reproducibly, and a fault is not a wording to copy (#2225).
func TestAForHeaderWithTooFewSeparatorsNamesTheCloser(t *testing.T) {
	for _, header := range []string{"(())", "(( ))", "((1;2))", "((i=0))"} {
		src := "for " + header + "; do echo x; break; done\n"
		const want = "ksh: syntax error at line 1: `))' unexpected\n"
		if got := forHeaderDiagnostic(t, src); got != want {
			t.Errorf("%q\n got %q\nwant %q", src, got, want)
		}
	}
}

// More than two separators is **accepted** here as it is in zsh: everything
// past the second `;` belongs to the third expression, so the loop parses and
// a body that breaks on its first pass never reaches the leftover text.
func TestAForHeaderWithExtraSeparatorsIsTaken(t *testing.T) {
	for _, src := range []string{
		"for ((;;;)); do echo body; break; done",
		"for ((;;;;)); do echo body; break; done",
		"for ((1;2;3;4)); do echo body; break; done",
		"for (( ; ; ; )); do echo body; break; done",
	} {
		if _, err := syntax.Parse(src+"\n", ksh.Dialect()); err != nil {
			t.Fatalf("%q: refused while parsing: %v", src, err)
		}
		out, st := runKsh(t, t.TempDir(), src)
		if out != "body\n" || st != 0 {
			t.Errorf("%q\n got %q (status %d)\nwant \"body\\n\" at 0", src, out, st)
		}
	}
}

// And a loop that does reach the third expression finds text that is not an
// expression, because the fold left the separator in it: one pass runs and the
// arithmetic then fails, which is what bounds the acceptance.
func TestTheFoldedSectionFailsAsArithmetic(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), "for ((i=0;i<2;i++;i=9)); do echo b=$i; done")
	const want = "b=0\nksh: i++;i=9: arithmetic syntax error\n"
	if out != want || st != 1 {
		t.Errorf("\n got %q (status %d)\nwant %q at 1", out, st, want)
	}
}

// What this shell *keeps* of each header part, which is one string quoted
// back in two places: a trace writes it and a complaint blames it. The first
// two parts give up their trailing blanks and the third gives up its leading
// ones, and nothing is trimmed after that — so one header traces `((  i=0))`
// and `((i++  ))`. Measured 2026-09-13 on ksh93u+ 2012-08-01; see
// interp/arithforpart.go and Diagnostics.ArithForPartText.
func TestAForHeaderPartKeepsTheBlanksAtOneEnd(t *testing.T) {
	t.Run("the trace", func(t *testing.T) {
		out, st := runKsh(t, t.TempDir(), "set -x; for ((  i=0  ;  i<1  ;  i++  )); do :; done")
		want := "+ ((  i=0))\n+ ((  i<1))\n+ :\n+ ((i++  ))\n+ ((  i<1))\n"
		if out != want || st != 0 {
			t.Errorf("\n got %q (status %d)\nwant %q at 0", out, st, want)
		}
	})
	// The same string, arrived at through a complaint rather than a trace,
	// which is the whole reason the two are one answer: the initializer keeps
	// the blank in front of it and the step keeps the one behind.
	for _, tc := range []struct{ name, src, want string }{
		{"the initializer", "for (( i=1/0 ;; )); do :; done", "ksh:  i=1/0: divide by zero\n"},
		{"the condition", "for (( i=0; i=1/0 ; )); do :; done", "ksh:  i=1/0: divide by zero\n"},
		{"the step", "for (( i=0; i<3; i=1/0 )); do echo body; done", "body\nksh: i=1/0 : divide by zero\n"},
		// A part that will not parse at all takes the same text, which is
		// what says the trim is the part's and not the evaluator's.
		{"a part that is no expression", `x="echo hi"; for (( $x ;;)); do :; done`, "ksh:  echo hi: arithmetic syntax error\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 1 {
				t.Errorf("\n got %q (status %d)\nwant %q at 1", out, st, tc.want)
			}
		})
	}
	// And the trim is taken before the expansions, which is where a reading
	// could go the other way and lose a blank the *value* brought: the source
	// blank in front of the `;` is gone and the one inside `$x` is not.
	t.Run("before the expansions", func(t *testing.T) {
		for _, tc := range []struct{ name, src, want string }{
			{"the initializer", `x=" 1/0 "; for (( $x ;; )); do :; done`, "ksh:   1/0 : divide by zero\n"},
			{"the step", `x=" 1/0 "; for ((;; $x )); do echo b; done`, "b\nksh:  1/0  : divide by zero\n"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				out, st := runKsh(t, t.TempDir(), tc.src)
				if out != tc.want || st != 1 {
					t.Errorf("\n got %q (status %d)\nwant %q at 1", out, st, tc.want)
				}
			})
		}
	})
}
