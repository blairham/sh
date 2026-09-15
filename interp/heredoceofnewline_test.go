// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Whether a here-document whose delimiter never arrived gains the newline its
// last line never had.
//
// Measured 2026-09-12 with `od -c` on the raw output, from a script file and
// again through `-c`:
//
//	bash 5.3, bash 3.2, bash-as-sh   body\n   5 bytes
//	dash, ksh93, zsh                 body     4 bytes
//
// It is reachable no other way. A here-document closed by its delimiter
// always has a body ending in a newline, so this is the only shape in which
// the question exists — which is also why the corpus cannot ask it: `$( )`
// strips trailing newlines and the harness trims them, so the row for this
// snippet records `body` in all six columns and could never show the byte
// (#1020).
//
// A Go test on the runner's own bytes is what is left, and the `run` helper
// returns them untrimmed.
const unterminatedHeredoc = "cat <<X\nbody"

func TestAnUnterminatedHeredocsLastLineIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name  string
		adds  Answer
		want  string
		which string
	}{
		{"the newline is supplied", Yes, "body\n", "bash"},
		{"the body is left as it was written", No, "body", "dash, ksh93 and zsh"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := permissive()
			sem.UnterminatedHeredocGainsATrailingNewline = tc.adds
			out, st := run(t, unterminatedHeredoc, withSem(sem))
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0 — %s", out, st, tc.want, tc.which)
			}
		})
	}
}

// An empty body is not the shape the axis is about, and neither answer may
// put a byte in it.
//
// Measured 2026-09-15 with `od -c` on the raw output of `cat <<X` with the
// input ending there: bash 5.3, dash, ksh93 and zsh all hand the command
// nothing. The axis asks whether the *last line* of a body gains the newline
// it never had, and a body with no lines has no last line — so a shell
// answering Yes was writing a bare newline nobody asked for, at status 0,
// which is exactly the silent-wrong-answer shape #2298 is a burndown of.
//
// Both answers are checked rather than only bash's: an axis that is not asked
// here must give the same bytes either way, and asserting one answer would
// leave the other free to differ.
func TestAnEmptyUnterminatedHeredocGainsNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		adds Answer
	}{
		{"where the last line would gain one", Yes},
		{"and where it would not", No},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := permissive()
			sem.UnterminatedHeredocGainsATrailingNewline = tc.adds
			out, st := run(t, "cat <<X\n", withSem(sem))
			if out != "" || st != 0 {
				t.Errorf("got %q (status %d), want no bytes at 0", out, st)
			}
		})
	}
}

// And with nothing to ask about, the axis is not asked: a Runner holding no
// answer runs the command rather than refusing it.
func TestAnEmptyUnterminatedHeredocNeverAsks(t *testing.T) {
	sem := permissive()
	sem.UnterminatedHeredocGainsATrailingNewline = Unspecified
	out, st := run(t, "cat <<X\n", withSem(sem))
	if out != "" || st != 0 {
		t.Errorf("got %q (status %d), want no bytes at 0", out, st)
	}
}

// A quoted delimiter changes what the body is *expanded* to and not whether
// it ends in a newline, so both spellings move together.
func TestTheQuotedDelimiterAsksTheSameQuestion(t *testing.T) {
	sem := permissive()
	sem.UnterminatedHeredocGainsATrailingNewline = Yes
	out, st := run(t, "cat <<'X'\nbody", withSem(sem))
	if want := "body\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// The axis is asked at the disagreement and nowhere else: a here-document
// that reached its delimiter has a body ending in a newline already, so a
// Runner with no answer still runs one.
func TestATerminatedHeredocNeverAsks(t *testing.T) {
	sem := permissive()
	sem.UnterminatedHeredocGainsATrailingNewline = Unspecified
	out, st := run(t, "cat <<X\nbody\nX\n", withSem(sem))
	if want := "body\n"; out != want || st != 0 {
		t.Errorf("terminated: got %q (status %d), want %q at 0", out, st, want)
	}

	out, st = run(t, unterminatedHeredoc, withSem(sem))
	if st == 0 {
		t.Errorf("unterminated: status 0 and %q, want the axis refused by name", out)
	}
	if !strings.Contains(out, "unterminated here-document") {
		t.Errorf("unterminated: %q does not name the axis", out)
	}
}

// The mark is the lexer's and it is a fact about the text: the body ran to the
// end of the input. Every other here-document leaves it off, which is what
// keeps the question off the common path.
func TestTheLexerMarksOnlyTheHeredocThatRanOut(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want bool
	}{
		{"the delimiter never arrived", unterminatedHeredoc, true},
		{"the delimiter arrived", "cat <<X\nbody\nX\n", false},
		{"an empty body that ran out", "cat <<X\n", true},
		{"a here-string", "cat <<<body\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := onlyRedirect(t, f).HeredocAtEOF; got != tc.want {
				t.Errorf("HeredocAtEOF = %v, want %v", got, tc.want)
			}
		})
	}
}

func onlyRedirect(t *testing.T, f *syntax.File) *syntax.Redirect {
	t.Helper()
	c, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd)
	if !ok || len(c.Redirs) != 1 {
		t.Fatalf("want one simple command with one redirection, got %#v", f.Stmts[0].Expr)
	}
	return c.Redirs[0]
}
