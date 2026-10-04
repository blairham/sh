// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// What a refused flag group names is decided by the word's *first* group, and
// the text inside a group is read as a list of commands. Measured 2026-10-04
// on ksh93u+ 2012-08-01 over `-c` (#5722). See syntax.Error.FlagGroupWordTail
// and docs/spec/grammar/substitutions.md.
func TestTheFirstFlagGroupDecidesTheRefusal(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// A later group is text in the first one's tail, not a tail of its
		// own.
		{`echo "${(U)x}${(U)y}"`, "syntax error at line 1: `x}${(U)y}\"\"' unexpected"},
		{`echo ${(U)x}${(U)y}`, "syntax error at line 1: `x}${(U)y}' unexpected"},
		// Including across an expansion whose operand is a word of its own.
		{`echo "${(U)x}${y#X*}${(U)z}"`, "syntax error at line 1: `x}${y#X*}${(U)z}\"\"' unexpected"},
		// And a later refusal is never reached.
		{`echo "${(S)w##(a|ab)}${v%%(bc|cbc)}"`, "syntax error at line 1: `w##' unexpected"},
		// A `(` after these ends the tail; after the pattern characters it
		// does not.
		{`echo "${(U)x}a[(i)]"`, "syntax error at line 1: `x}a[' unexpected"},
		{`echo "${(U)x}a.(b)c"`, "syntax error at line 1: `x}a.' unexpected"},
		{`echo "${(U)x}a#(b)c"`, "syntax error at line 1: `x}a#' unexpected"},
		{`echo "${(U)x}a%(b)c"`, "syntax error at line 1: `x}a%(b)c\"\"' unexpected"},
		// Outside quotes the same stop blames the parenthesis.
		{`echo ${(U)x:-ab(N)} q`, "syntax error at line 1: `(' unexpected"},
		{`echo "${(U)x:-ab(N)}"`, "syntax error at line 1: `x:-ab' unexpected"},
		// A `#` straight after the group opens a comment, and the input runs
		// out a line past the command.
		{`echo "${(U)#a}"`, "syntax error at line 2: `end of file' unexpected"},
		// The group's own text, read as a list.
		{`echo ${(j:|:)x}`, "syntax error at line 1: `|' unexpected"},
		{`echo ${(|a)x}y`, "syntax error at line 1: `|' unexpected"},
		{`echo ${(a.b:|c)x}y`, "syntax error at line 1: `|' unexpected"},
		{`echo ${(j:||:)x}y`, "syntax error at line 1: `||' unexpected"},
		{`echo ${(&&a)x}y`, "syntax error at line 1: `&&' unexpected"},
		{`echo ${(a|;b)x}y`, "syntax error at line 1: `;' unexpected"},
		{`echo ${(a;|b)x}y`, "syntax error at line 1: `|' unexpected"},
		{`echo ${()x}y`, "syntax error at line 1: `)' unexpected"},
		{`echo ${(a|)x}y`, "syntax error at line 1: `)' unexpected"},
		{`echo ${(a:)x}y`, "syntax error at line 1: `)' unexpected"},
		{`echo ${(&)x}y`, "syntax error at line 1: `)' unexpected"},
		{`echo ${(a:&)x}y`, "syntax error at line 1: `)' unexpected"},
		// The controls: lists that read, and words that are not labels.
		{`echo ${(a|b)x}y`, "syntax error at line 1: `x}y' unexpected"},
		{`echo ${(a | b)x}y`, "syntax error at line 1: `x}y' unexpected"},
		{`echo ${(:|a)x}y`, "syntax error at line 1: `x}y' unexpected"},
		{`echo ${(a:b|c)x}y`, "syntax error at line 1: `x}y' unexpected"},
		{`echo ${(a:b:|c)x}y`, "syntax error at line 1: `x}y' unexpected"},
		{`echo ${('a:'|b)x}y`, "syntax error at line 1: `x}y' unexpected"},
		{`echo ${(x a:|b)x}y`, "syntax error at line 1: `x}y' unexpected"},
		{`echo ${(a:&b)x}y`, "syntax error at line 1: `x}y' unexpected"},
		{`echo ${(a&)x}y`, "syntax error at line 1: `x}y' unexpected"},
		{`echo ${(;)x}y`, "syntax error at line 1: `x}y' unexpected"},
		{`echo ${(;|a)x}y`, "syntax error at line 1: `x}y' unexpected"},
	} {
		if got := refusal(t, c.src); got != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
