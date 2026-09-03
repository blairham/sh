// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// Whether a prompt parameter's value is expanded is the dialect's answer, not
// this package's.
//
// Measured through a pty: bash, dash and ksh93 expand it, and zsh draws it as
// it stands unless asked with `setopt PROMPT_SUBST`.
func TestAPromptIsExpandedWhenTheDialectSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name  string
		style PromptStyle
		ps1   string
		want  string
	}{
		{"expanded", PromptStyle{Expand: true}, "<$who>@ ", "<someone>@ "},
		{"drawn as it stands", PromptStyle{}, "<$who>@ ", "<$who>@ "},
		{"arithmetic too", PromptStyle{Expand: true}, "<$((1+1))>", "<2>"},
		// The default is expanded like anything else, and a lone dollar
		// before a space is a dollar: the fallback has to survive its own
		// expansion or every shell without a PS1 draws a broken prompt.
		{"the default survives it", PromptStyle{Expand: true}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vars := map[string]string{"who": "someone"}
			if tc.ps1 != "" {
				vars["PS1"] = tc.ps1
			}
			s := Shell{Runner: newTestRunner(vars), Style: tc.style}
			want := tc.want
			if tc.ps1 == "" {
				want = "$ "
			}
			if got := s.prompt("PS1", "$ "); got != want {
				t.Errorf("prompt = %q, want %q", got, want)
			}
		})
	}
}

// And expanded every time it is drawn rather than once when it is read.
//
// That is the whole reason to expand it: a prompt holding `$PWD` is expected
// to follow the directory. Reading it once would make it follow nothing.
func TestAPromptIsExpandedAtEachDraw(t *testing.T) {
	var out, errs strings.Builder
	in := readerFile(t, "n=1\nn=2\n")
	r := newTestRunner(map[string]string{"PS1": "[$n]"})
	r.Stdout = &out
	s := Shell{
		Runner: r, In: in, Out: &out, Err: &errs,
		Style: PromptStyle{Expand: true},
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	// A prompt before each line: nothing assigned yet, then 1, then 2.
	if got := errs.String(); got != "[][1][2]" {
		t.Errorf("prompts = %q, want [][1][2] — one per line, expanded each time", got)
	}
}
