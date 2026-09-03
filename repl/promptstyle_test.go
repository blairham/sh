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
		{"and is drawn as it stands when nothing expands", PromptStyle{}, "", ""},
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

// The default text goes through the same treatment as a value read from the
// parameter.
//
// `$ ` survives its own expansion, so today the two are indistinguishable and
// a test written with it grades nothing. The decision still has to be graded,
// because the defaults are on their way to being the dialect's — bash's is
// `\s-\v\$ `, which is text that means something — and the moment they are,
// a default that skipped expansion would be drawn wrong.
func TestTheDefaultIsRenderedTheSameWay(t *testing.T) {
	s := Shell{
		Runner: newTestRunner(map[string]string{"who": "someone"}),
		Style:  PromptStyle{Expand: true},
	}
	if got := s.prompt("PS1", "<$who> "); got != "<someone> " {
		t.Errorf("default = %q, want it expanded like any other prompt", got)
	}
	plain := Shell{Runner: newTestRunner(map[string]string{"who": "someone"})}
	if got := plain.prompt("PS1", "<$who> "); got != "<$who> " {
		t.Errorf("default = %q, want it as it stands", got)
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
