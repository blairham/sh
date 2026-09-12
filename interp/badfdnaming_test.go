// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a duplication says when nothing is open at the number it names, and
// which spelling of the target it quotes.
//
// Measured 2026-09-12, `cat <&10`:
//
//	bash 5.3, bash 3.2   10: Bad file descriptor
//	ksh93                10: cannot open [Bad file descriptor]
//	zsh 5.9.2            10: bad file descriptor
//
// And with the number behind a variable, `n=10; cat <&$n`, bash quotes `$n`
// where ksh93 and zsh both print `10` — verbatim, so `${n}`, `"$n"` and
// `$((5+5))` each come back as themselves (#734).
//
// The sentence is not CannotOpen's. This is a duplication rather than an
// open, and one dialect's CannotOpen names the reason *first*, so sharing the
// field would answer `bad file descriptor: 10` there.
func runBadFd(t *testing.T, src string, d Diagnostics) string {
	t.Helper()
	out, _ := run(t, src, func(r *Runner) {
		s := permissive()
		r.Semantics = &s
		r.Diagnostics = &d
	})
	return out
}

func TestADuplicationWhoseSourceIsNotOpenTakesTheDialectsSentence(t *testing.T) {
	for _, tc := range []struct {
		name, wording, want string
	}{
		{"the substrate's shape", "", "10: bad file descriptor"},
		{"a verb and a bracket", "%[1]s: cannot open [%[2]s]", "10: cannot open [bad file descriptor]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := runBadFd(t, `cat <&10`, Diagnostics{
				DuplicationSourceNotOpen: tc.wording,
				LowercaseReason:          true,
			})
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q", out, tc.want)
			}
		})
	}
}

func TestTheDuplicationTargetIsNamedAsWrittenOrAsExpanded(t *testing.T) {
	for _, tc := range []struct {
		name     string
		asWrite  bool
		src      string
		want     string
		wantNot  string
		wantNote string
	}{
		{
			name: "as written", asWrite: true,
			src: `n=10; cat <&$n`, want: "$n:", wantNot: "10:",
			wantNote: "bash quotes the word the script wrote",
		},
		{
			name: "as expanded", asWrite: false,
			src: `n=10; cat <&$n`, want: "10:", wantNot: "$n:",
			wantNote: "ksh93 and zsh print the number it came to",
		},
		{
			// A target written as a number is the same word either way, so
			// this row must not move: an implementation that quoted the word
			// only sometimes would still pass the two above.
			name: "a literal number is the same word under both", asWrite: true,
			src: `cat <&10`, want: "10:", wantNot: "$",
			wantNote: "nothing expanded, so there is nothing to differ",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := runBadFd(t, tc.src, Diagnostics{
				NamesTheDuplicationTargetAsWritten: tc.asWrite,
			})
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q — %s", out, tc.want, tc.wantNote)
			}
			if strings.Contains(out, tc.wantNot) {
				t.Errorf("got %q, want it not to contain %q", out, tc.wantNot)
			}
		})
	}
}

// The naming is the duplication's alone. The same dialect names an *open* by
// what the word expanded to, so a change that read the written text for every
// redirection target would pass the rows above and be wrong here.
func TestAnOpenIsStillNamedByWhatItExpandedTo(t *testing.T) {
	out := runBadFd(t, `n=nosuchfile-xyz; cat <$n`, Diagnostics{
		NamesTheDuplicationTargetAsWritten: true,
	})
	if !strings.Contains(out, "nosuchfile-xyz") {
		t.Errorf("got %q, want the expanded name", out)
	}
	if strings.Contains(out, "$n") {
		t.Errorf("got %q, want the written word left out of an open", out)
	}
}
