// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A dialect asked to read a `#` as an ordinary character does, wherever the
// `#` stands.
//
// The flag is the front end's — it says the text about to be parsed was typed
// at a prompt by a shell whose option for that is off — and the parser has no
// other way to be told, because the first token is read in the constructor and
// a line that *is* a `#` has no second token to correct.
func TestADialectCanReadAHashAsAnOrdinaryCharacter(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		skipped   []string
		ordinary  []string
	}{
		{
			name:     "a trailing comment is an argument",
			src:      "echo a #b",
			skipped:  []string{"echo", "a"},
			ordinary: []string{"echo", "a", "#b"},
		},
		{
			// The whole line, which is the case the constructor's timing is
			// about: with comments skipped there is no command at all.
			name:     "a line that is only a comment is a command",
			src:      "# hi",
			skipped:  nil,
			ordinary: []string{"#", "hi"},
		},
		{
			name:     "a hash after a separator",
			src:      "true; #b",
			skipped:  []string{"true"},
			ordinary: []string{"true", "#b"},
		},
		{
			// Not a rule about every `#`: one that does not begin a word is
			// an ordinary character in every shell and in both readings.
			name:     "a hash inside a word is never a comment",
			src:      "echo a#b",
			skipped:  []string{"echo", "a#b"},
			ordinary: []string{"echo", "a#b"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := commandWords(t, tc.src, syntax.CommentsSkipped); !sameWords(got, tc.skipped) {
				t.Errorf("with comments skipped %q parsed to %q, want %q", tc.src, got, tc.skipped)
			}
			if got := commandWords(t, tc.src, syntax.CommentsOrdinaryText); !sameWords(got, tc.ordinary) {
				t.Errorf("with a hash ordinary %q parsed to %q, want %q", tc.src, got, tc.ordinary)
			}
		})
	}
}

// A substitution's body records the rule it was read under, because the body
// is kept as text and parsed a second time by whatever runs it.
//
// No `#` is written in these bodies on purpose. What is recorded is the rule
// the *re-parse* is to follow, and a body holding no comment at all has to
// carry it just the same — a field set only where a `#` was seen would be a
// fact about this text rather than about how to read the next one.
//
// A command substitution carries the rule and a process substitution does not,
// which is measured on the shell that has the option — see Lexer.bodyComments
// for the table. Nothing here can observe what the re-parse does with it; what
// this asserts is that the fact survives the trip, which is the half that was
// missing and the half a re-parse cannot recover.
func TestASubstitutionBodyRecordsHowItsHashWasRead(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      syntax.CommentMode
	}{
		{name: "a command substitution", src: "echo $(f a)", want: syntax.CommentsOrdinaryText},
		{name: "its backquoted spelling", src: "echo `f a`", want: syntax.CommentsOrdinaryText},
		{name: "a process substitution", src: "cat <(f a)", want: syntax.CommentsSkipped},
		{name: "the writing one", src: "f > >(g a)", want: syntax.CommentsSkipped},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := syntax.Core()
			d.Comments = syntax.CommentsOrdinaryText
			if got := bodyMode(t, tc.src, d); got != tc.want {
				t.Errorf("%q recorded %v on the body, want %v", tc.src, got, tc.want)
			}
			// And a parse that said nothing records nothing, whichever kind
			// it is: the field is the front end's statement and not a fact
			// about the construct.
			if got := bodyMode(t, tc.src, syntax.Core()); got != syntax.CommentsSkipped {
				t.Errorf("%q recorded %v with nothing asked, want %v", tc.src, got, syntax.CommentsSkipped)
			}
		})
	}
}

// commandWords is the words of the first command in src, parsed under mode.
func commandWords(t *testing.T, src string, mode syntax.CommentMode) []string {
	t.Helper()
	d := syntax.Core()
	d.Comments = mode
	var words []string
	for _, c := range simpleCommands(t, src, d) {
		for _, w := range c.Args {
			words = append(words, w.Literal())
		}
	}
	return words
}

// bodyMode is the comment rule recorded on the first substitution span in src.
func bodyMode(t *testing.T, src string, d syntax.Dialect) syntax.CommentMode {
	t.Helper()
	for _, c := range simpleCommands(t, src, d) {
		for _, w := range append(append([]*syntax.Word(nil), c.Args...), redirectWords(c)...) {
			for _, sp := range w.Spans {
				switch sp.Kind {
				case syntax.CommandSubst, syntax.ProcSubstIn, syntax.ProcSubstOut, syntax.ProcSubstFile:
					return sp.Comments
				}
			}
		}
	}
	t.Fatalf("%q holds no substitution span", src)
	return syntax.CommentsSkipped
}

// redirectWords are the targets of a command's redirections, which is where
// `f > >(g)` keeps its substitution.
func redirectWords(c *syntax.SimpleCmd) []*syntax.Word {
	var out []*syntax.Word
	for _, r := range c.Redirs {
		if r.Word != nil {
			out = append(out, r.Word)
		}
	}
	return out
}

// simpleCommands are the top-level simple commands of src, in order.
func simpleCommands(t *testing.T, src string, d syntax.Dialect) []*syntax.SimpleCmd {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out []*syntax.SimpleCmd
	for _, stmt := range f.Stmts {
		p, ok := stmt.Expr.(*syntax.Pipeline)
		if !ok {
			continue
		}
		for _, cmd := range p.Cmds {
			if c, ok := cmd.(*syntax.SimpleCmd); ok {
				out = append(out, c)
			}
		}
	}
	return out
}

func sameWords(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
