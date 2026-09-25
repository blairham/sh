// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The third question about OPTIND, and the only one of the three that is
// about a *refusal* rather than about when an accepted word is counted
// (#4474).
//
// TestGetoptsClusterOptindAxis and
// TestGetoptsCountsTheWordOnTheNextCallIsAnAxis ask when a word the scan
// accepted is counted. This asks what a letter the spec string does not name
// does to the rest of the word it was in: one answer reports the letters
// after it, the other never does.

func endsTheWordSem(ends, lags Answer) Semantics {
	s := getoptsSem()
	s.GetoptsErrorEndsTheWord = ends
	s.GetoptsCountsTheWordOnTheNextCall = lags
	return s
}

// A refused letter in the middle of a cluster, which is the shape the two
// answers part on: the letter after it is reported under one and lost under
// the other. Reading OPTIND alone would not show that — the count moves too,
// and an implementation that moved the count while going on scanning would
// pass a test that read only numbers.
//
// Both answers are asked under both answers to the counting axis beside
// them, because the two move the same parameter and a test that varied only
// one could not say which of them the number came from. Three of the four
// cells are a real column; the fourth is the composition and is marked.
func TestGetoptsErrorEndsTheWordIsAnAxis(t *testing.T) {
	const src = `set -- -axb -b
OPTIND=1
while getopts 'ab' o 2>/dev/null; do printf '[%s %s]' "$o" "$OPTIND"; done
printf ' end=%s\n' "$OPTIND"`
	for _, tc := range []struct {
		name       string
		ends, lags Answer
		want       string
	}{
		// bash 5.3.20, bash 3.2.57 and ksh93u+ 2012-08-01: the `x` costs
		// the letter and the `b` beside it is still reported.
		{"the letter alone is refused", No, No, "[a 1][? 1][b 2][b 3] end=3\n"},
		// zsh 5.9.2 without POSIX_BUILTINS: the same four letters, with the
		// count a word behind all the way along.
		{"refused and lagging", No, Yes, "[a 1][? 1][b 1][b 2] end=3\n"},
		// zsh 5.9.2 with POSIX_BUILTINS: three calls, not four. The `b`
		// inside `-axb` is never reported, and OPTIND names the word after
		// it on the call that refused.
		{"the refusal ends the word", Yes, Yes, "[a 1][? 2][b 2] end=3\n"},
		// No column holds this pair — it is the composition, recorded so a
		// change to either axis has to say what it did to the other.
		{"ended, counted at the last letter", Yes, No, "[a 1][? 2][b 3] end=3\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := endsTheWordSem(tc.ends, tc.lags)
			out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
	// No redirection here: the diagnostic is what this subtest reads, and
	// the `2>/dev/null` the rows above carry would swallow it.
	t.Run("unanswered", func(t *testing.T) {
		sem := endsTheWordSem(Unspecified, No)
		const src = `set -- -axb
OPTIND=1
getopts 'ab' o; getopts 'ab' o`
		out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
		const axis = "a refused `getopts` letter giving up the rest of its word"
		if !strings.Contains(out, axis) {
			t.Fatalf("got %q, want it to carry %q", out, axis)
		}
	})
}

// A missing argument is the same refusal at a word that is already spent, so
// only the count can show it — and it is the half that would be lost if the
// axis were read at the unknown-option branch alone. The counting axis is on
// here, which is what makes the `No` row 1 rather than 2, so the row says
// which of the two axes moved the number.
func TestGetoptsErrorEndsTheWordAtAMissingArgument(t *testing.T) {
	const src = `set -- -ba
OPTIND=1
while getopts 'ba:' o 2>/dev/null; do printf '[%s %s]' "$o" "$OPTIND"; done
printf ' end=%s\n' "$OPTIND"`
	for _, tc := range []struct {
		name string
		ends Answer
		want string
	}{
		// zsh 5.9.2 without POSIX_BUILTINS, and with it.
		{"the word is spent and the count lags", No, "[b 1][? 1] end=2\n"},
		{"the refusal counts the word", Yes, "[b 1][? 2] end=2\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := endsTheWordSem(tc.ends, Yes)
			out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// And the control the two above need: a letter the spec string *does* name
// still lags under both answers. Without it, an implementation that turned
// the lagging off wholesale — rather than at the refusal — would pass every
// row above, because a refusal is all those rows read.
func TestGetoptsErrorEndsTheWordLeavesAnAcceptedLetterAlone(t *testing.T) {
	const src = `set -- -b -c
OPTIND=1
while getopts 'bc' o 2>/dev/null; do printf '[%s %s]' "$o" "$OPTIND"; done
printf ' end=%s\n' "$OPTIND"`
	for _, ends := range []Answer{No, Yes} {
		sem := endsTheWordSem(ends, Yes)
		out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
		const want = "[b 1][c 2] end=3\n"
		if out != want {
			t.Errorf("answer %v: got %q, want %q", ends, out, want)
		}
	}
}
