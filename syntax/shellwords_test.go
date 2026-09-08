// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// zshLike is the grammar the `(Z)` flag's shell has, which is what ShellWords
// is read under: the constructs a value split as a command line may hold.
func zshLike() Dialect {
	d := Core()
	d.ParamExpansionFlags = true
	d.ProcessSubstitution = true
	d.GlobQualifiers = true
	d.ArithCommand = true
	return d
}

// Every row is measured on zsh 5.9.2, the only shell in the panel whose
// grammar has `${(Z:opts:)v}` at all — so its answer is the specification
// here rather than one column of a comparison.
//
// The fields are asserted and not their number, deliberately. A splitter that
// hands back the whole value as one field and one that splits correctly have
// the same status and differ only in what they contain, and the count is the
// weakest thing they differ in: `a b  c` is three fields under both a correct
// split and a split that dropped the quotes off `'b c'`.
func TestShellWordsSplitsAValueTheWayTheShellWouldSplitALine(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		opt  ShellSplit
		want []string
	}{
		{"blanks separate", "a b  c", ShellSplit{}, []string{"a", "b", "c"}},
		{"a tab is a blank", "a\tb", ShellSplit{}, []string{"a", "b"}},
		{"one word is one word", "a", ShellSplit{}, []string{"a"}},
		{"nothing is nothing", "", ShellSplit{}, nil},
		{"blanks alone are nothing", "   ", ShellSplit{}, nil},

		// Quoting decides the boundary and is kept in the word: the caller
		// asked how the line splits, not what the words would come to.
		{"single quotes hold a blank", "a 'b c' d", ShellSplit{}, []string{"a", "'b c'", "d"}},
		{"double quotes hold one too", `a "b c" d`, ShellSplit{}, []string{"a", `"b c"`, "d"}},
		{"a backslash holds one", `a b\ c d`, ShellSplit{}, []string{"a", `b\ c`, "d"}},
		{"an empty quoted word is not an empty word", "a '' b", ShellSplit{}, []string{"a", "''", "b"}},
		{"a quote may open mid-word", `a"b c"d`, ShellSplit{}, []string{`a"b c"d`}},

		// Substitutions are one word each, unrun.
		{"a command substitution is one word", "a $(echo x y) b", ShellSplit{}, []string{"a", "$(echo x y)", "b"}},
		{"a backquoted one as well", "a `y z` b", ShellSplit{}, []string{"a", "`y z`", "b"}},
		{"and an expansion", "a ${x} b", ShellSplit{}, []string{"a", "${x}", "b"}},
		{"an arithmetic command keeps its parentheses", "((1+2)) x", ShellSplit{}, []string{"((1+2))", "x"}},

		// Operators are words of their own.
		{
			"a pipeline is words and operators", "echo a|b; c && d",
			ShellSplit{},
			[]string{"echo", "a", "|", "b", ";", "c", "&&", "d"},
		},
		{"a one-digit descriptor joins its operator", "a 2>&1", ShellSplit{}, []string{"a", "2>&", "1"}},
		{"two digits do not", "a 22>&1", ShellSplit{}, []string{"a", "22", ">&", "1"}},
		{"nor a digit written apart", "a 2 > f", ShellSplit{}, []string{"a", "2", ">", "f"}},

		// A here-document operator is an operator. Nothing reads a body:
		// what follows is more of the same value.
		{
			"no here-document body is read", "a <<EOF\nbody\nEOF\nb",
			ShellSplit{NewlineIsBlank: true},
			[]string{"a", "<<", "EOF", "body", "EOF", "b"},
		},

		// Input that ends inside something is not an error here.
		{"an unclosed quote is the rest of the text", "a 'b", ShellSplit{}, []string{"a", "'b"}},
		{"an unclosed double quote too", `a "unclosed`, ShellSplit{}, []string{"a", `"unclosed`}},
		{"and an unclosed substitution", "a $(unclosed", ShellSplit{}, []string{"a", "$(unclosed"}},
		{"a trailing backslash", `a b\`, ShellSplit{}, []string{"a", `b\`}},

		// Newlines. Without NewlineIsBlank each one is its own word, and the
		// word is `;` — the terminator's spelling, not the newline's.
		{"a newline is a semicolon", "a\nb", ShellSplit{}, []string{"a", ";", "b"}},
		{"one per newline", "a\n\n\nb", ShellSplit{}, []string{"a", ";", ";", ";", "b"}},
		{"a leading one", "\na", ShellSplit{}, []string{";", "a"}},
		{"a trailing one", "a\n", ShellSplit{}, []string{"a", ";"}},
		{"and blank where asked", "a\nb", ShellSplit{NewlineIsBlank: true}, []string{"a", "b"}},
		{"a line continuation is removed", "a \\\nb", ShellSplit{NewlineIsBlank: true}, []string{"a", "b"}},

		// Comments, three ways.
		{
			"with no comment rule a hash is a word", "a # hi\nb",
			ShellSplit{Comments: CommentsOrdinaryText, NewlineIsBlank: true},
			[]string{"a", "#", "hi", "b"},
		},
		{
			"and a tight one is one word", "a #hi\nb",
			ShellSplit{Comments: CommentsOrdinaryText, NewlineIsBlank: true},
			[]string{"a", "#hi", "b"},
		},
		{
			"kept, a comment is one word", "a # hi\nb",
			ShellSplit{Comments: CommentsKept, NewlineIsBlank: true},
			[]string{"a", "# hi", "b"},
		},
		{
			"skipped, it is no word at all", "a # hi\nb",
			ShellSplit{Comments: CommentsSkipped, NewlineIsBlank: true},
			[]string{"a", "b"},
		},
		{
			"the newline is not part of a kept comment", "a # hi\nb",
			ShellSplit{Comments: CommentsKept},
			[]string{"a", "# hi", ";", "b"},
		},
		{"a value that is only a comment", "#only", ShellSplit{Comments: CommentsSkipped}, nil},
		{
			"mid-word a hash is text under every rule", "a#b c",
			ShellSplit{Comments: CommentsKept},
			[]string{"a#b", "c"},
		},
		{
			"a hash inside quotes is not a comment", "a 'x #y' b",
			ShellSplit{Comments: CommentsSkipped},
			[]string{"a", "'x #y'", "b"},
		},
		// Inside a substitution a kept comment and a skipped one behave
		// alike, and both swallow the closing parenthesis on the same line —
		// measured, and the reason commentsExist has two modes answering yes.
		{
			"no comment rule keeps a substitution whole", "a $(b # c) d",
			ShellSplit{Comments: CommentsOrdinaryText},
			[]string{"a", "$(b # c)", "d"},
		},
		{
			"a kept comment eats the parenthesis", "a $(b # c) d",
			ShellSplit{Comments: CommentsKept},
			[]string{"a", "$(b # c) d"},
		},
		{
			"and a skipped one does the same", "a $(b # c) d",
			ShellSplit{Comments: CommentsSkipped},
			[]string{"a", "$(b # c) d"},
		},
		{
			"a newline lets it close", "a $(b\n# c\n) d",
			ShellSplit{Comments: CommentsSkipped, NewlineIsBlank: true},
			[]string{"a", "$(b\n# c\n)", "d"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ShellWords(tc.src, zshLike(), tc.opt)
			if !equalWords(got, tc.want) {
				t.Errorf("ShellWords(%q, %+v) = %q, want %q", tc.src, tc.opt, got, tc.want)
			}
		})
	}
}

// A value that would hang or panic a scanner is a value a plugin manager can
// hand this, so the property is asserted rather than assumed. It is the one
// the lexer's own doc comment promises, checked through the door ShellWords
// opens: an arbitrary *value* reaches it, where the lexer's other callers
// only ever hand it program text.
func TestShellWordsTerminatesOnAnythingAtAll(t *testing.T) {
	for _, src := range []string{
		"", " ", "\n", "\\", "'", `"`, "$(", "${", "`", "<<", "((", "$((",
		"$(((", "a$(b`c'd\"e", strings.Repeat("(", 200), strings.Repeat("'", 51),
		"<(", ">(", "#", "a #", "\x00 b", "\xff\xfe", "a\\\n", "${(", "<<<",
	} {
		for _, opt := range []ShellSplit{
			{}, {Comments: CommentsKept}, {Comments: CommentsSkipped, NewlineIsBlank: true},
		} {
			// A hang is the failure this guards, and `go test` reports it as
			// a timeout of the whole package rather than as this row — which
			// is still a report, and is why nothing more elaborate is here.
			ShellWords(src, zshLike(), opt)
		}
	}
}

func equalWords(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
