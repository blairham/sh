// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A `;` inside a `case` header (#3040).
//
// `case x; in` and `case x in;` are the two spellings, and one dialect takes
// both wherever a newline may stand: between the subject and the `in`, and
// after the `in`. Nine of the completion functions that shell ships open a
// `case` the first way, so a parser without this cannot read them at all.
//
// Measured 2026-09-15, each probe in a script file of its own, against a
// `case` with one arm that echoes:
//
//	| probe           | zsh 5.9.2 | bash 5.3 | bash-as-sh | bash 3.2 | ksh93u+ | dash | ash |
//	| `case x; in`    | ran       | refused  | refused    | refused  | refused | refused | refused |
//	| `case x ; ; in` | ran       | refused  | refused    | refused  | refused | refused | refused |
//	| `case x;⏎in`    | ran       | refused  | refused    | refused  | refused | refused | refused |
//	| `case x in;`    | ran       | refused  | refused    | refused  | refused | refused | refused |
//	| `case x;; in`   | refused   | refused  | refused    | refused  | refused | refused | refused |
//	| `case x in ;;`  | refused   | refused  | refused    | refused  | refused | refused | refused |
//	| `case x & in`   | refused   | refused  | refused    | refused  | refused | refused | refused |
//
// One column against six, so this is a dialect's grammar rather than a bug
// against an agreeing panel. The last three rows are the half that keeps the
// flag narrow: it is the `;` and not the separator family, since the shell
// that takes this refuses `;;` and `&` in the same two places.
//
// The rows below assert the arm *and its body* rather than "it parsed",
// because parsing is the cheap half: a reading that stepped over the `;` by
// swallowing the token after it would parse every row here and lose an arm.

func caseHeaderSeparatorGrammar(d *Dialect) {
	d.CaseHeaderSpansSeparators = true
}

func TestASemicolonMayStandInACaseHeader(t *testing.T) {
	t.Parallel()
	d := Core()
	caseHeaderSeparatorGrammar(&d)
	for _, tc := range []struct {
		name, src string
	}{
		{"before the in", "case x; in\nx) :;;\nesac"},
		{"a run of them before the in", "case x ; ; in\nx) :;;\nesac"},
		{"one before a newline before the in", "case x;\nin\nx) :;;\nesac"},
		{"after the in", "case x in;\nx) :;;\nesac"},
		{"on both sides of the in", "case x; in;\nx) :;;\nesac"},
		{"and with the arm on the same line", "case x; in x) :;; esac"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse(tc.src, d)
			if err != nil {
				t.Fatalf("%q: %v", tc.src, err)
			}
			c, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*CaseClause)
			if !ok {
				t.Fatalf("%q: first command is %T, want a case clause", tc.src, f.Stmts[0].Expr.(*Pipeline).Cmds[0])
			}
			if len(c.Items) != 1 {
				t.Fatalf("%q: %d arms, want 1", tc.src, len(c.Items))
			}
			if got := len(c.Items[0].Patterns); got != 1 {
				t.Fatalf("%q: %d patterns in the arm, want 1", tc.src, got)
			}
			if got := c.Items[0].Patterns[0].Literal(); got != "x" {
				t.Errorf("%q: pattern %q, want %q", tc.src, got, "x")
			}
			if got := len(c.Items[0].Body); got != 1 {
				t.Errorf("%q: %d statements in the arm's body, want 1", tc.src, got)
			}
		})
	}
}

// TestACaseHeaderSeparatorIsASemicolonAndNothingElse is the half that keeps
// the flag from being read as "any separator": the shell that takes a `;`
// here refuses `;;` and `&` in the same two positions, so the dialect with
// the flag on must refuse them too.
func TestACaseHeaderSeparatorIsASemicolonAndNothingElse(t *testing.T) {
	t.Parallel()
	d := Core()
	caseHeaderSeparatorGrammar(&d)
	for _, tc := range []struct{ name, src string }{
		{"a double semicolon before the in", "case x;; in\nx) :;;\nesac"},
		{"a double semicolon after the in", "case x in ;;\nx) :;;\nesac"},
		{"an ampersand before the in", "case x & in\nx) :;;\nesac"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.src, d); err == nil {
				t.Errorf("%q parsed, and the shell this flag is read from refuses it", tc.src)
			}
		})
	}
}

// TestACaseHeaderSeparatorIsOneDialectsAndNotEveryShells is the other half of
// the panel: six columns refuse the line, so the core grammar must too.
func TestACaseHeaderSeparatorIsOneDialectsAndNotEveryShells(t *testing.T) {
	t.Parallel()
	d := Core()
	for _, src := range []string{"case x; in\nx) :;;\nesac", "case x in;\nx) :;;\nesac"} {
		if _, err := Parse(src, d); err == nil {
			t.Errorf("%q parsed without the flag that reads a `;` in a case header", src)
		}
	}
}
