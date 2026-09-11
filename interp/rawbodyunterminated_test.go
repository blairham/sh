// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// Text that ran out inside an expansion is refused wherever it is read back,
// rather than handed on as the expansion the reader had to invent for it.
//
// The reader is `syntax.HeredocSpans`, and two roads reach it: a
// here-document body, and a value read again by the flag that re-evaluates
// one. It had no way to say a `${` never closed — it returned a
// parameter-expansion span whose name was whatever followed — so `x${` came
// back as the literal `x` plus an expansion over nothing.
//
// That is the failure mode this codebase minds most, and the here-document
// road is where it was reachable without any flag at all: measured 2026-09-10
// on `cat <<EOF` / `x${` / `EOF`, bash 5.3.15 answers `unexpected EOF while
// looking for matching }`, bash 3.2.57 and zsh 5.9.2 `bad substitution`, dash
// `Syntax error: Missing '}'` and ksh93 a syntax error — all five refuse, and
// this shell printed `x` at status 0 under two of its four dialects (#1653).
//
// The status is the command's rather than the shell's: every column in the
// panel runs the line after this one, so the refusal stops the redirection
// and not the script.
func TestABodyThatRanOutInsideAnExpansionIsRefused(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"an expansion with nothing after it", "cat <<EOF\nWROTE${\nEOF\necho after"},
		{"one that got as far as a name", "cat <<EOF\nWROTE${a\nEOF\necho after"},
		{"one that got as far as an operator", "cat <<EOF\nWROTE${a:\nEOF\necho after"},
		{"a command substitution", "cat <<EOF\nWROTE$(echo\nEOF\necho after"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, nil)
			// `after` is the line the shells go on to run, and `WROTE` is
			// the literal text a reader that swallowed the failure would
			// have handed the command in place of the body. A marker rather
			// than a single letter because the diagnostic is on the same
			// stream and has letters of its own.
			if !strings.HasSuffix(out, "after\n") {
				t.Errorf("output = %q, want the next line to have run", out)
			}
			if strings.Contains(out, "WROTE") {
				t.Errorf("output = %q, want the body refused rather than written", out)
			}
		})
	}
}

// The control: a body whose expansions *are* closed still expands, so the
// refusal above is about the text running out rather than about bodies.
func TestAClosedExpansionInABodyStillExpands(t *testing.T) {
	out, st := run(t, "v=VAL\ncat <<EOF\nx${v}y\nEOF\necho after", nil)
	if want := "xVALy\nafter\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}
