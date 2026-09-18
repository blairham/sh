// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A backslash-newline inside the older substitution form is removed before
// the command text is parsed, and it is removed whatever quoting it stands in
// *inside* that text.
//
// This is the one place the two spellings of a command substitution part
// company. `$( )` holds a program and hands it over as written, so a
// single-quoted `'a\⏎b'` in one is the four characters it looks like; the
// backquoted form takes the pair out first, so the same text is `ab`.
// Measured 2026-09-16 from script files and unanimous in bash 5.3, bash 3.2,
// zsh 5.9.2, ksh93u+ and dash. Before, the pair survived into the text and
// every dialect printed a backslash and a newline the panel does not (#3453).
//
// Asserted on the span's Value — the command text handed on — because both
// readings parse. The wrong one runs `printf %s 'a\⏎b'`, which prints four
// characters at status 0, so a test that asked only for a nil error would
// pass it.
func TestABackquotedBodyRemovesALineContinuationInAnyQuotingWithinIt(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src, want string
	}{
		// The rule, at the quoting that makes it visible: nothing inside a
		// single-quoted run is otherwise touched, and this is.
		{"inside single quotes", "echo \"`printf %s 'a\\\nb'`\"", "printf %s 'ab'"},
		{"inside double quotes", "echo \"`printf %s \"a\\\nb\"`\"", `printf %s "ab"`},
		{"unquoted in the text", "echo \"`printf %s a\\\nb`\"", "printf %s ab"},
		{"between a word's own characters", "echo \"`prin\\\ntf %s x`\"", "printf %s x"},

		// A quoted here-document body inside the substitution loses it too,
		// and that row is what says this is not a continuation the inner
		// parse removes for itself: nothing inside a quoted here-document
		// removes one.
		{"a quoted here-document body", "echo \"`cat <<'E'\na\\\nb\nE\n`\"", "cat <<'E'\nab\nE\n"},

		// The floor. An escaped backslash is unescaped first, so the newline
		// it leaves behind is an ordinary character and stays: `'a\\⏎b'` is
		// `a\⏎b` in all five.
		{"an escaped backslash", "echo \"`printf %s 'a\\\\\nb'`\"", "printf %s 'a\\\nb'"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := Parse(c.src, Core())
			if err != nil {
				t.Fatalf("%q: %v", c.src, err)
			}
			got, ok := firstCommandSubst(f)
			if !ok {
				t.Fatalf("%q: no backquoted substitution in the tree", c.src)
			}
			if got.Value != c.want {
				t.Errorf("%q: body is %q, want %q", c.src, got.Value, c.want)
			}
		})
	}
}

// The control that keeps the rule on the older spelling: `$( )` hands its
// program over with the pair where it was written. The panel prints a
// backslash and a newline for `"$(printf %s 'a\⏎b')"` in all five columns,
// and an implementation that removed the pair for every command substitution
// would pass the test above and break this line.
func TestADollarParenBodyKeepsALineContinuationWrittenInsideIt(t *testing.T) {
	t.Parallel()
	const src = "echo \"$(printf %s 'a\\\nb')\""
	f, err := Parse(src, Core())
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	sc, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if !ok {
		t.Fatalf("%q is not a simple command", src)
	}
	const want = "printf %s 'a\\\nb'"
	for _, w := range sc.Args {
		for _, s := range w.Spans {
			if s.Kind != CommandSubst {
				continue
			}
			if s.Value != want {
				t.Errorf("%q: body is %q, want %q", src, s.Value, want)
			}
			return
		}
	}
	t.Fatalf("%q: no command substitution in the tree", src)
}
