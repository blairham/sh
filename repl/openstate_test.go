// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// What the line is still inside, drawn in a dialect's words.
//
// The parser says `if then` where zsh says `then`, and `then &&` where zsh
// says `then cmdand`. The difference is not clause-versus-construct: a clause
// stands in place of what it is inside and an operator follows it, and which
// is which is the dialect's to say.
func TestDrawingWhatTheLineIsInside(t *testing.T) {
	words := map[string]OpenWord{
		"for":  {Text: "for"},
		"if":   {Text: "if"},
		"then": {Text: "then", Replaces: true},
		"&&":   {Text: "cmdand"},
		"'":    {Text: "quote"},
		// A loop's `do` is drawn as nothing, and the loop it belongs to
		// stays.
		"do": {},
	}
	for _, tc := range []struct {
		name string
		open []syntax.Open
		want string
	}{
		{"nothing", nil, ""},
		{"one construct", []syntax.Open{{Word: "for", Construct: true}}, "for"},
		{
			"a clause stands in place of its construct",
			[]syntax.Open{{Word: "if", Construct: true}, {Word: "then"}},
			"then",
		},
		{
			"an operator follows it",
			[]syntax.Open{{Word: "if", Construct: true}, {Word: "then"}, {Word: "&&"}},
			"then cmdand",
		},
		{
			"nesting keeps the outer one",
			[]syntax.Open{
				{Word: "for", Construct: true},
				{Word: "if", Construct: true},
				{Word: "then"},
			},
			"for then",
		},
		{
			"a word with no text is not drawn",
			[]syntax.Open{{Word: "while", Construct: true}, {Word: "do"}},
			"",
		},
		{
			"and a word with no entry is not drawn either",
			[]syntax.Open{{Word: "for", Construct: true}, {Word: "case", Construct: true}},
			"for",
		},
		{"the lexer's own", []syntax.Open{{Word: "'"}}, "quote"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Shell{Style: PromptStyle{OpenWords: words}, counts: &counts{open: tc.open}}
			if got := s.openState(); got != tc.want {
				t.Errorf("drew %q, want %q", got, tc.want)
			}
		})
	}
	// A dialect that says nothing draws nothing, whatever is open.
	silent := Shell{counts: &counts{open: []syntax.Open{{Word: "for", Construct: true}}}}
	if got := silent.openState(); got != "" {
		t.Errorf("drew %q with no table, want nothing", got)
	}
}

// The state is kept while the construct is unfinished and let go when it is
// whole, so a prompt that is not a continuation has nothing to say.
func TestTheOpenStateIsKeptOnlyWhileWaiting(t *testing.T) {
	var out, errs strings.Builder
	in := readerFile(t, "for i in 1\ndo\n:\ndone\necho after\n")
	r := newTestRunner(map[string]string{"PS1": "<>", "PS2": "<%_>"})
	r.Stdout = &out
	s := Shell{
		Runner: r, In: in, Out: &out, Err: &errs,
		Style: PromptStyle{
			Escape: '%',
			Codes:  map[rune]PromptField{'_': FieldOpenState},
			OpenWords: map[string]OpenWord{
				"for": {Text: "for"}, "do": {},
			},
		},
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Three continuations inside the loop, then a plain prompt again once it
	// is whole — and one more asking for the line after that.
	if got := errs.String(); got != "<><for><for><for><><>" {
		t.Errorf("prompts = %q, want the loop named while it is open and not after", got)
	}
}
