// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// Where a refused `$( … )` body's two lines say they happened, when the text
// that failed is not the script's own line (#3354), and what the sentence
// adds while the closer is still being looked for (#3467).
//
// Measured 2026-09-17, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> s.sh` with
// stdin from /dev/null, in a fresh directory, on bash 5.3.20. Every row holds
// `v=$(echo hi; for)` or `$(for)` somewhere, and every row's two lines carry
// the same prefix.
//
// The three columns that are not here are not missing: zsh, ksh93 and dash
// each already name their own route on these shapes — `(eval):2:`,
// `s.sh[2]: eval:` and `s.sh: 2: eval:` — through the borrowed-text naming
// they already had, and none of them tags the construct at all.
func TestARefusedSubstitutionBodyNamesTheRouteItArrivedBy(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			// The control, and the one that must not move: written on the
			// script's own line, the location names no route.
			"the script's own line",
			"echo a\nv=$(echo hi; for)\n",
			"bash: line 2: ",
		},
		{
			"an eval argument",
			"echo a\neval 'q=1; v=$(echo hi; for); z=2'\n",
			"bash: eval: line 2: ",
		},
		{
			"an EXIT trap's body",
			"trap 'v=$(echo hi; for)' EXIT\n",
			"bash: exit trap: line 1: ",
		},
		{
			"an ERR trap's body",
			"trap 'v=$(echo hi; for)' ERR\nfalse\n",
			"bash: error trap: line 2: ",
		},
		{
			"a DEBUG trap's body",
			"trap 'v=$(echo hi; for)' DEBUG\necho x\n",
			"bash: debug trap: line 2: ",
		},
		{
			// Every signal but one falls back to the builtin's own name,
			// which is measured rather than assumed from the four
			// pseudo-conditions each having a word.
			"a signal trap's body",
			"trap 'v=$(echo hi; for)' USR1\nkill -USR1 $$\n",
			"bash: trap: line 1: ",
		},
		{
			// The older spelling's body is read at expansion time, so a
			// `$( … )` written inside one is the construct's.
			"a backquoted body",
			"v=`echo $(for)`\n",
			"bash: command substitution: line 1: ",
		},
		{
			// And so is a here-document body, whatever command the
			// redirection is for.
			"a here-document body",
			"echo a\necho b\ncat <<E\nbefore $(echo hi; for) after\nE\n",
			"bash: command substitution: line 4: ",
		},
		{
			// The second control: a `$( … )` inside a `$( … )` names no
			// route, because that dialect reads both bodies while it is
			// reading the script's line.
			"a substitution inside a substitution",
			"v=$(echo $(echo hi; for))\n",
			"bash: line 1: ",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs, _ := locRun(t, tc.src)
			for _, line := range strings.Split(strings.TrimSuffix(errs, "\n"), "\n") {
				if !strings.HasPrefix(line, tc.want) {
					t.Errorf("wrote %q, want every line to start %q", errs, tc.want)
					break
				}
			}
			if errs == "" {
				t.Error("nothing was reported")
			}
		})
	}
}

// The text a here-document body's refusal quotes back is a line of the
// *substitution*, which is the program that dialect was reading — so it starts
// where that program did, just past the opener, and is the line as written
// once the opener is behind it.
//
// Measured the same way on bash 5.3.20.
func TestAHereDocumentBodysRefusalQuotesTheTextItWasReading(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the opener's own line",
			"echo a\necho b\ncat <<E\nbefore $(echo hi; for) after\nE\n",
			"bash: command substitution: line 4: `echo hi; for) after'\n",
		},
		{
			"a line the opener is not on",
			"echo a\ncat <<E\nx $(echo hi\nfor) y\nE\n",
			"bash: command substitution: line 4: `for) y'\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs, _ := locRun(t, tc.src)
			if !strings.HasSuffix(errs, tc.want) {
				t.Errorf("wrote %q, want it to end %q", errs, tc.want)
			}
		})
	}
}

// And what the sentence adds while the shell is still looking for the closing
// parenthesis (#3467).
func TestATokenRefusedInsideASubstitutionNamesTheCloserStillWanted(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"v=$(echo hi; ;)\n", "syntax error near unexpected token `;' while looking for matching `)'"},
		{"v=$(for z in 1 2 3; done)\n", "syntax error near unexpected token `done' while looking for matching `)'"},
		{"v=$(} )\n", "syntax error near unexpected token `}' while looking for matching `)'"},
		{"v=$(esac)\n", "syntax error near unexpected token `esac' while looking for matching `)'"},
		// The closer itself is what the shell was looking for, so nothing is
		// added: the row that says this is not simply every refusal.
		{"v=$(echo hi; for)\n", "syntax error near unexpected token `)'"},
		// And the older spelling's body is read as text, so it never says
		// what it is looking for.
		{"v=`echo hi; ;`\n", "syntax error near unexpected token `;'"},
	} {
		t.Run(strings.TrimSuffix(tc.src, "\n"), func(t *testing.T) {
			_, errs, _ := locRun(t, tc.src)
			first, _, _ := strings.Cut(errs, "\n")
			if _, msg, _ := strings.Cut(first, ": line 1: "); msg != tc.want {
				t.Errorf("wrote %q, want the sentence to be %q", first, tc.want)
			}
		})
	}
}

// A trap body that will not parse *itself* names the same word, which is the
// half that was already written and knew only one of the six conditions.
//
// Measured on bash 5.3.20 with a two-line body whose second line is `if`: the
// same words the refused substitution above carries, from the message the
// trap's own parse failure writes.
func TestATrapBodysOwnParseFailureNamesTheCondition(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"EXIT", "trap 'echo a\nif' EXIT\n", "bash: exit trap: line "},
		{"ERR", "trap 'echo a\nif' ERR\nfalse\n", "bash: error trap: line "},
		{"DEBUG", "trap 'echo a\nif' DEBUG\necho x\n", "bash: debug trap: line "},
		{"INT", "trap 'echo a\nif' INT\nkill -INT $$\n", "bash: interrupt trap: line "},
		{"a signal with no word of its own", "trap 'echo a\nif' USR1\nkill -USR1 $$\n", "bash: trap: line "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs, _ := locRun(t, tc.src)
			if !strings.Contains(errs, tc.want) {
				t.Errorf("wrote %q, want a line starting %q", errs, tc.want)
			}
		})
	}
}

// locRun is splitRun with the program's text handed to the runner, which is
// what a front end does before each line and what the second message quotes
// from. Without it the quote is the body alone, which is a different
// question's answer.
func locRun(t *testing.T, src string) (out, errs string, status int) {
	t.Helper()
	p := presets["bash"]
	f := p.Parse(t, src)
	var o, e strings.Builder
	r := p.Runner(dialecttest.Base{Stdout: &o, Stderr: &e, Env: []string{"PATH=/usr/bin:/bin"}})
	r.SetProgramText(src)
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return o.String(), e.String(), st
}
