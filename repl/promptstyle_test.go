// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
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
		{"expanded", PromptStyle{Expand: interp.PromptExpandsAlways}, "<$who>@ ", "<someone>@ "},
		{"drawn as it stands", PromptStyle{}, "<$who>@ ", "<$who>@ "},
		{"arithmetic too", PromptStyle{Expand: interp.PromptExpandsAlways}, "<$((1+1))>", "<2>"},
		// The default is expanded like anything else, and a lone dollar
		// before a space is a dollar: the fallback has to survive its own
		// expansion or every shell without a PS1 draws a broken prompt.
		{"the default survives it", PromptStyle{Expand: interp.PromptExpandsAlways}, "", ""},
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
		Style:  PromptStyle{Expand: interp.PromptExpandsAlways},
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
		Style: PromptStyle{Expand: interp.PromptExpandsAlways},
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	// A prompt before each line: nothing assigned yet, then 1, then 2.
	if got := errs.String(); got != "[][1][2]" {
		t.Errorf("prompts = %q, want [][1][2] — one per line, expanded each time", got)
	}
}

// A command substitution is run again at every prompt, and not once when the
// prompt was assigned.
//
// This is the whole of how a prompt that says which branch you are on works,
// and a value read once would say the branch you were on when the shell
// started. Measured: with `PS2='@@$(echo re)##'` set, bash drew `re` at the
// continuation prompt, so the substitution is run for both prompts.
func TestACommandSubstitutionRunsAgainAtEachPrompt(t *testing.T) {
	var out, errs strings.Builder
	in := readerFile(t, "n=1\nn=2\n")
	r := newTestRunner(map[string]string{"PS1": "[$(echo $n)]"})
	r.Stdout = &out
	s := Shell{
		Runner: r, In: in, Out: &out, Err: &errs,
		Style: PromptStyle{Expand: interp.PromptExpandsAlways},
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := errs.String(); got != "[][1][2]" {
		t.Errorf("prompts = %q, want [][1][2] — the command run again each time", got)
	}
}

// The continuation prompt is read exactly as the first one is.
//
// Measured through a pty, with PS2 set to a prompt of codes: bash 5.3.15 and
// 3.2.57 drew the user name, the directory, the privilege character and a
// bracketed color at the continuation prompt, and zsh 5.9.2 drew the same from
// its own language. Nothing about the table is conditional on which of the two
// parameters the text came from.
func TestTheContinuationPromptIsReadLikeTheFirst(t *testing.T) {
	var out, errs strings.Builder
	in := readerFile(t, "for i in 1\ndo :; done\n")
	r := newTestRunner(map[string]string{
		"USER": "someone",
		"PS1":  `<1:\u>`,
		"PS2":  `<2:\u:$(echo sub):\[\e[31m\]>`,
	})
	r.Stdout = &out
	s := Shell{
		Runner: r, In: in, Out: &out, Err: &errs,
		Style: PromptStyle{
			Expand: interp.PromptExpandsAlways, Escape: '\\', Unknown: KeepBoth,
			Codes: map[rune]PromptField{
				'u': FieldUser,
				'[': FieldNonPrintingStart, ']': FieldNonPrintingEnd,
			},
			Sequences: map[rune]string{'e': "\x1b"},
		},
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	want := "<1:someone>" + "<2:someone:sub:\x1b[31m>" + "<1:someone>"
	if got := errs.String(); got != want {
		t.Errorf("prompts = %q, want %q", got, want)
	}
}

// A dialect's default prompt is its own, and is read the same way an assigned
// one is — so a default may hold codes.
//
// Measured with nothing assigned: real bash prompts `bash-5.3$ `, which is
// `\s-\v\$ ` drawn; real zsh prompts with the host and a percent sign; dash
// and ksh93 prompt `$ `.
func TestTheDialectsDefaultPrompt(t *testing.T) {
	bashish := Shell{
		Runner: newTestRunner(nil),
		Name:   "bash",
		Style: PromptStyle{
			Escape:  '\\',
			Version: "5.3", Privilege: "$",
			Codes: map[rune]PromptField{
				's': FieldShellName, 'v': FieldVersion, '$': FieldPrivilege,
			},
			Default:          `\s-\v\$ `,
			DefaultContinued: "> ",
		},
	}
	// Through beforeReading, which is where the choice between the two
	// prompts is made and so the only place the defaults are reached.
	if got := drawn(bashish, false); got != "bash-5.3$ " {
		t.Errorf("default = %q, want bash-5.3$ ", got)
	}
	if got := drawn(bashish, true); got != "> " {
		t.Errorf("continuation = %q, want > ", got)
	}

	// With a continuation that is not the substrate's own text, so that the
	// dialect being asked at all is what the answer depends on. bash's is
	// `> ` and so is the fallback, which makes bash the one dialect whose
	// continuation cannot grade this.
	distinct := bashish
	distinct.Style.DefaultContinued = `\s? `
	if got := drawn(distinct, true); got != "bash? " {
		t.Errorf("continuation = %q, want the dialect's own, drawn", got)
	}

	// An assignment still wins over it.
	assigned := bashish
	assigned.Runner = newTestRunner(map[string]string{"PS1": "mine> "})
	if got := drawn(assigned, false); got != "mine> " {
		t.Errorf("assigned = %q, want mine> ", got)
	}

	// A dialect that has not said anything gets the substrate's text. Empty
	// means unsaid here, not a prompt of nothing — only an assignment can ask
	// for that.
	silent := Shell{Runner: newTestRunner(nil)}
	if got := drawn(silent, false); got != "$ " {
		t.Errorf("unsaid = %q, want the substrate's $ ", got)
	}

	// A code the table does not have degrades rather than refusing: zsh's
	// default continuation is `%_> ` and `%_` is not drawable yet, so it
	// comes out as `> ` until it is.
	zshish := Shell{
		Runner: newTestRunner(nil),
		Style: PromptStyle{
			Escape: '%', Unknown: DropBoth,
			DefaultContinued: "%_> ",
		},
	}
	if got := drawn(zshish, true); got != "> " {
		t.Errorf("continuation = %q, want the missing code to drop out", got)
	}
}

// drawn is the prompt this shell would print next, through the path that
// chooses between the two of them.
func drawn(s Shell, continuing bool) string {
	var pending strings.Builder
	if continuing {
		pending.WriteString("for i in 1\n")
	}
	return s.beforeReading(context.Background(), nil, &pending).text
}

// The two orders a prompt can be drawn in, and the difference is whether an
// escape that came *out of a parameter* is one the table ever sees.
//
// Measured in both directions on real shells. bash, with `C='\u'` and
// `PS1='A${C}B\u C '`, draws `A\uBbhamilton C` — the escapes are read first
// and the parameter's `\u` survives as text. zsh, with `C='%F{red}'` under
// `setopt prompt_subst`, draws both colors — the expansion runs first and
// the table reads its result.
//
// Both orders are asserted here, from one style and one value, because a
// test of the zsh order alone passes for a shell that always expands first
// and would have taken bash's prompt with it.
func TestTheExpansionAndTheEscapeTableRunInTheDialectsOrder(t *testing.T) {
	base := PromptStyle{
		Expand: interp.PromptExpandsAlways,
		Escape: '%',
		Codes:  map[rune]interp.PromptField{'m': interp.FieldHost},
	}
	for _, tc := range []struct {
		name   string
		before bool
		want   string
	}{
		// What the host renders *to* is empty in this harness and is not
		// the question. The question is which occurrences the table
		// reached: with the table first, the `%m` the parameter produces
		// arrives too late and survives as text.
		{"escapes first", false, "[][%m]"},
		// With the expansion first both are in the string by the time the
		// table walks it, so neither survives.
		{"expansion first", true, "[][]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := base
			st.ExpandBeforeEscapes = tc.before
			s := Shell{Runner: newTestRunner(map[string]string{
				"C":   "%m",
				"PS1": "[%m][$C]",
			}), Style: st}
			if got := s.prompt("PS1", "$ "); got != tc.want {
				t.Errorf("prompt = %q, want %q", got, tc.want)
			}
		})
	}
}
