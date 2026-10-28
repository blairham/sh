// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// Behind the `}` of a short `if` whose last arm is a brace group and which
// has no `else`, what stands there is refused one token later: the token
// after it, past any `;` and newlines, or the last one read where the input
// ends first (#6553). Measured 2026-10-15 on zsh 5.9.2 at
// /opt/homebrew/bin/zsh with `-fc '<program>'`, a script file and standard
// input; the rows are on syntax.Parser.refuseAfterABraceArm.
func TestARefusalBehindABraceArmNamesTheTokenAfter(t *testing.T) {
	for _, tc := range []struct {
		src  string
		line uint
		want string
	}{
		{"if [[ x ]] { :; } a b c", 1, "b"},
		{"if [[ x ]] { :; } a\necho ok", 2, "echo"},
		{"if [[ x ]] { :; } a; b", 1, "b"},
		{"if [[ x ]] { :; } foo; bar", 1, "bar"},
		{"if [[ x ]] { :; } | cat", 1, "cat"},
		{"if [[ x ]] { :; } && echo y", 1, "echo"},
		{"if [[ x ]] { :; } & b", 1, "b"},
		{"if [[ x ]] { :; } ;; b", 1, "b"},
		{"if [[ x ]] { :; } >/dev/null foo", 1, "/dev/null"},
		{"if [[ x ]] { :; } foo >/dev/null", 1, ">"},
		{"if [[ x ]] { :; } then :; fi", 1, ":"},
		{"if [[ x ]] { :; } x then", 1, "then"},
		{"if [[ x ]] { :; } a fi", 1, "fi"},
		{"if [[ x ]] { :; } a; else :; fi", 1, "else"},
		{"if [[ x ]] { :; } (echo x)", 1, "echo"},
		{"if [[ x ]] { :; } \"a b\" c", 1, "c"},
		{"if [[ x ]] { :; } {foo}", 1, "foo"},
		{"if [[ x ]] { :; } {echo hi}", 1, "echo"},
		{"if [[ x ]] { :; } {v}</dev/null", 1, "v"},
		{"if [[ x ]] { :; } {}", 1, "}"},
		{"if (( 1 )) { echo a; } { echo b; }", 1, "echo"},
		{"if ! [[ x ]] { :; } a b", 1, "b"},
		{"if [[ x ]] && [[ y ]] { :; } a b", 1, "b"},
		{"if [[ x ]] { :; } elif [[ y ]] { :; } foo bar", 1, "bar"},
		{"{ if [[ x ]] { echo in; } }\necho ok", 2, "echo"},
		{"f() { if [[ x ]] { echo in; } }; f", 1, "f"},
		// The input running out names the last token read.
		{"if [[ x ]] { :; } a", 1, "a"},
		{"if [[ x ]] { :; } a;", 1, ";"},
		{"if [[ x ]] { :; } a\n", 2, "\\n"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		want := "parse error near `" + tc.want + "'"
		if got := diagTable(zsh.Diagnostics()).ParseFailure(err); got != want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, want)
		}
		var se *syntax.Error
		if errors.As(err, &se) && tc.want != `\n` && uint(se.Pos.Line) != tc.line {
			// A refused newline is numbered on the line after it, which is
			// the location's rule and not this one's.
			t.Errorf("%q: line %d, want %d", tc.src, se.Pos.Line, tc.line)
		}
	}
	// The controls: a separator straight behind the `}`, a `fi`, an `else`
	// arm, and a condition ended by a `;`.
	for _, src := range []string{
		"if [[ x ]] { echo a; }\necho ok",
		"if [[ x ]] { :; } ; echo ok",
		"if [[ x ]] { :; } # c",
		"if [[ x ]] { echo a; } fi",
		"if [[ x ]] { :; } else { :; }",
		"if :; { :; } { echo hi; }",
		"if [[ x ]]; { :; } a b",
	} {
		if _, err := syntax.Parse(src, zsh.Dialect()); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
}
