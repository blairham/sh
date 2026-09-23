// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
)

// commentOptionAfter runs src and hands back the runner it ran on, so the
// option can be asked for the way the front end asks for it — by name,
// through interp.Runner.DialectOption — rather than through the listing.
func commentOptionAfter(t *testing.T, src string) (on, known bool) {
	t.Helper()
	var buf strings.Builder
	r := preset.Runner(dialecttest.Base{Name: "sh", Stdout: &buf, Stderr: &buf, Env: []string{"PATH=/usr/bin:/bin"}})
	st, err := r.Run(t.Context(), preset.Parse(t, src))
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	if st != 0 {
		t.Fatalf("run %q: status %d: %s", src, st, buf.String())
	}
	return r.DialectOption("interactive_comments")
}

// `interactive_comments` decides whether a `#` typed at this shell's prompt
// opens a comment. It is on with nothing said, and a script can turn it off.
//
// Measured 2026-09-23 on bash 5.3.15, `printf … | bash --norc --noprofile -i`
// on a pipe with a scratch HOME:
//
//	echo a #b                                  a
//	shopt -u interactive_comments; echo a #b    a #b
//	…then shopt -s again; echo c #d             c
//
// It sat in shoptStates reading `on`, which was honest about the default and
// wrong about everything the name exists for: `shopt -s` was a silent grant
// and `shopt -u` was refused at 1, so the one state a script asks for was the
// one it could not have (#4149).

// TestTheCommentOptionNamesItselfToThePrompt is the wiring, and it is the
// assertion the rest of this file cannot make: the option only reaches a `#`
// because the axis carries its *name*. An empty axis reads as "nothing has to
// be on", which is the state this preset was in and is indistinguishable from
// correct at the default.
func TestTheCommentOptionNamesItselfToThePrompt(t *testing.T) {
	if got := bash.Semantics().PromptCommentsNeedTheOption; got != "interactive_comments" {
		t.Errorf("PromptCommentsNeedTheOption = %q, want %q", got, "interactive_comments")
	}
}

// TestTheCommentOptionIsReadableByName: the front end asks for it per line
// through interp.Runner.DialectOption, and the name lives in the `shopt`
// namespace rather than in `set -o`'s — so this is the route that has to
// answer.
func TestTheCommentOptionIsReadableByName(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{`:`, true},
		{`shopt -u interactive_comments`, false},
		{`shopt -u interactive_comments; shopt -s interactive_comments`, true},
	} {
		on, known := commentOptionAfter(t, tc.src)
		if !known {
			t.Errorf("%s: the name is not readable through DialectOption", tc.src)
			continue
		}
		if on != tc.want {
			t.Errorf("%s: DialectOption says on=%v, want %v", tc.src, on, tc.want)
		}
	}
}

// TestTheCommentOptionIsNotASetOName is the boundary the reader must not
// cross. Measured on bash 5.3.15: `[[ -o interactive_comments ]]` is **1** and
// `shopt -q interactive_comments` is **0** on the same shell, so making the
// second namespace answer the condition operator would be a widening bash
// does not have — and one that no test of the prompt would notice.
func TestTheCommentOptionIsNotASetOName(t *testing.T) {
	out, _ := answersRun(t, `[[ -o interactive_comments ]]; echo "dashO=$?"; shopt -q interactive_comments; echo "shoptQ=$?"`)
	if want := "dashO=1\nshoptQ=0\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestTheCommentOptionMovesBothWaysAndIsListed: the two states, their
// statuses, and the listing. A refusal is what this used to be, so the status
// is asserted beside the listing rather than left to the reader.
func TestTheCommentOptionMovesBothWaysAndIsListed(t *testing.T) {
	out, st := answersRun(t, `shopt interactive_comments
shopt -u interactive_comments; echo "u=$?"
shopt interactive_comments
shopt -s interactive_comments; echo "s=$?"
shopt interactive_comments`)
	want := "interactive_comments\ton\nu=0\ninteractive_comments\toff\ns=0\ninteractive_comments\ton\n"
	if out != want || st != 0 {
		t.Errorf("status %d, output %q; want status 0 and %q", st, out, want)
	}
}

// TestTheCommentOptionLeavesScriptInputAlone: non-interactive input is not
// this option's business in bash either, so a `#` in a script is a comment
// whichever way the option is set. Measured the same day:
// `bash -c 'shopt -u interactive_comments; echo a #b'` is `a`.
//
// This is the case that says the change is about the *prompt*: an
// implementation that reached the lexer for every route would pass every
// assertion above and fail this one.
func TestTheCommentOptionLeavesScriptInputAlone(t *testing.T) {
	for _, src := range []string{`echo a #b`, `shopt -u interactive_comments; echo a #b`} {
		out, st := answersRun(t, src)
		if strings.TrimSpace(out) != "a" || st != 0 {
			t.Errorf("%s: %q at %d, want \"a\" at 0", src, out, st)
		}
	}
}
