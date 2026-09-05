// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `compgen` writes the completions a word would have. What is here is the
// part this shell can answer from what it knows — the names of its own
// commands, and of the functions defined — and everything else is refused
// rather than guessed at.
func TestCompgenGeneratesWhatThisShellKnows(t *testing.T) {
	for _, c := range []struct {
		name, src, want, gone string
		status                int
	}{
		{"a builtin by letter", "compgen -b cd\n", "cd\n", "", 0},
		{"and by action name", "compgen -A builtin cd\n", "cd\n", "", 0},
		{"the word is a prefix", "compgen -b unse\n", "unset\n", "unalias", 0},
		{
			// Nothing to offer is a failure, because a completer is asking
			// whether there is anything at all.
			"nothing matching", "compgen -b zzzz\n", "", "zzz", 1,
		},
		{"a function", "f() { :; }\ncompgen -A function f\n", "f\n", "", 0},
		{
			// No action asked for, so nothing was generated — and that is
			// success rather than failure, which is the one place the two
			// come apart.
			"no action at all", "compgen\n", "", "compgen", 0,
		},
		{
			// An action bash has and this shell cannot generate.
			"an action we do not generate", "compgen -A file\n", "not implemented", "invalid action", 2,
		},
		{
			// And one that is not an action anywhere, which is a different
			// thing to tell a script: the first is a shell that is missing
			// something and this is a typo.
			"a name that is no action", "compgen -A nosuch\n", "invalid action name", "not implemented", 2,
		},
		{"an option letter we do not generate", "compgen -d\n", "not implemented", "", 2},
		{
			// bash has no short letter for the function action at all — `-u`
			// is user names there — so a function is not what this answers.
			// It said otherwise once, which is the wrong direction: an answer
			// where the real shell gives a different one.
			"there is no short letter for function",
			"f1() { :; }\ncompgen -u f1\n", "not implemented", "f1\n", 2,
		},
		{
			// A letter bash does not have either, which is a typo rather than
			// a shell that is missing something — the same split the action
			// names get.
			"a letter that is no letter", "compgen -z\n", "invalid option", "not implemented", 2,
		},
		{
			// The first non-option word is the one matched against and the
			// rest are ignored. Written with the narrow prefix first, so a
			// last-wins reading would answer with the wider one and show.
			"the first word is the one matched",
			"compgen -b unse read\n", "unset\n", "readonly", 0,
		},
		{"an action name with nothing after it", "compgen -A\n", "requires an argument", "", 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := compgenRun(t, c.src)
			if c.want != "" && !strings.Contains(out, c.want) {
				t.Errorf("said %q, want %q in it", out, c.want)
			}
			if c.gone != "" && strings.Contains(out, c.gone) {
				t.Errorf("said %q, want %q not in it", out, c.gone)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d (%q)", st, c.status, out)
			}
		})
	}
}

// The listing is the same one `enable` prints, so a name switched off is not
// offered as a completion either — it is not what running the word would
// find.
func TestCompgenDoesNotOfferABuiltinThatIsSwitchedOff(t *testing.T) {
	out, st := compgenRun(t, "enable -n cd\ncompgen -b cd\n")
	if strings.Contains(out, "cd") {
		t.Errorf("said %q, want a switched-off builtin left out", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1 — nothing left to offer", st)
	}
}

func compgenRun(t *testing.T, src string) (string, int) {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	r := &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh"}
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return buf.String(), st
}
