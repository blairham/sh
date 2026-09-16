// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A `case` subject that opens with a `(` (#3040).
//
// A `case` whose subject is a parenthesized subscripted parameter is what one
// shipped completion function stops this shell at, and that function is the
// one everything downstream of the completion system wanted. The subject
// stands where an argument does in one dialect, so
// the `(` there is two characters of the word rather than a grouping paren or
// a subshell — which is a *lexical* question, because `(` is in the operator
// table and a token beginning with one never reaches the word scanner.
//
// Measured 2026-09-15, each probe in a script file of its own:
//
//	| probe                              | zsh 5.9.2 | bash 5.3 | bash-as-sh | bash 3.2 | ksh93u+ | dash | ash |
//	| `case (x) in "(x)") echo lit;; …`  | `lit`     | refused  | refused    | refused  | refused | refused | refused |
//	| `case (x) in x) echo strip;; …`    | no arm    | refused  | refused    | refused  | refused | refused | refused |
//
// So the parentheses reach the matcher: the arm that matches is the one
// spelling them, and the arm spelling the subject without them matches
// nothing. The six refusing columns all name the `(` — `syntax error near
// unexpected token '('` in the three bash columns, `'(' unexpected` in ksh93,
// `"(" unexpected (expecting word)` in dash and BusyBox ash — so this is one
// dialect's grammar and not a bug against a panel that agrees. It is read
// only where [Dialect.GlobQualifiers] already says a leading `(` may belong
// to a word.
//
// The rows are written as the *subject text* rather than as "it parsed",
// because parsing is the cheap half: a reading that admitted the line and
// dropped the parentheses would parse every row here and match the wrong arm.

// caseSubjectParenGrammar is the grammar this construct needs.
func caseSubjectParenGrammar(d *Dialect) {
	// A `(` at the front of a word belongs to the word. The subject is such
	// a position; the flag is not about the subject, which is why the test
	// names it rather than a shell that happens to have it.
	d.GlobQualifiers = true
	// One row below is a group carrying an alternation.
	d.PatternAlternation = true
}

// caseSubject is the subject text of the first `case` in src.
func caseSubject(t *testing.T, src string, d Dialect) string {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	c, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*CaseClause)
	if !ok {
		t.Fatalf("%s: first command is %T, want a case clause", src, f.Stmts[0].Expr.(*Pipeline).Cmds[0])
	}
	if c.Word == nil {
		t.Fatalf("%s: the case has no subject", src)
	}
	// The source text rather than the literal: a `$` is what the word is
	// made of here and the expansion's spelling is part of what is being
	// pinned.
	return src[c.Word.Pos().Offset:c.Word.End().Offset]
}

func TestACaseSubjectMayOpenWithAParen(t *testing.T) {
	d := Core()
	caseSubjectParenGrammar(&d)
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			// The shape the issue is about, reduced.
			"a bare word in parentheses",
			"case (x) in (y) :;; esac",
			"(x)",
		},
		{
			// The shape the failing function writes: a subscripted
			// parameter with the parentheses around the whole of it.
			"a subscripted parameter in them",
			"case ($v[i]) in (y) :;; esac",
			"($v[i])",
		},
		{
			// Blanks inside the parentheses stay inside the word, which is
			// what says the `(` opened a group in the word rather than
			// ending it.
			"blanks inside them",
			"case ( a b ) in (y) :;; esac",
			"( a b )",
		},
		{
			// A group carrying an alternation, which is the shape that would
			// otherwise end the word at the `|`.
			"an alternation inside them",
			"case (a|b) in (y) :;; esac",
			"(a|b)",
		},
		{
			// Text after the group is the same word: the group is part of
			// the subject, not the whole of it.
			"text behind the group",
			"case (a|b)c in (y) :;; esac",
			"(a|b)c",
		},
		{
			// Nesting, so a hand-written scan for the first `)` would be
			// caught.
			"a nested group",
			"case ((a)b) in (y) :;; esac",
			"((a)b)",
		},
		{
			// The control that says nothing here moved for ordinary
			// subjects: a word with no paren in it is what it always was.
			"no paren at all",
			"case $v in (y) :;; esac",
			"$v",
		},
		{
			// And a group that opens *inside* the word already worked, so
			// its row is here to say the change did not cost it.
			"a group opening mid-word",
			"case x(a|b) in (y) :;; esac",
			"x(a|b)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := caseSubject(t, tc.src, d); got != tc.want {
				t.Errorf("%s: subject %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// TestACaseSubjectParenIsTheDialectsAndNotEveryShells is the other half: the
// six columns that refuse the line are refused here too, so the reading is
// not handed to a dialect whose reference shell says no.
func TestACaseSubjectParenIsTheDialectsAndNotEveryShells(t *testing.T) {
	d := Core()
	if _, err := Parse("case (x) in (y) :;; esac", d); err == nil {
		t.Error("`case (x) in` parsed without the grammar that reads a leading `(` as a word's")
	}
}
