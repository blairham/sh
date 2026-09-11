// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A backslash before the `}` that would close a `${ }` escapes it and is
// removed, and the operand's text is what carries the freed brace (#1966).
//
// This is the parse half. The operand of an expansion written inside double
// quotes is read as double-quoted *content*, where a backslash escapes only
// `$`, a backtick, a `"` and itself — and, here alone, the closing brace. The
// brace is the whole of the addition: an opening one keeps its backslash, and
// so does an ordinary character, both of them in every shell in the panel.
//
// Measured 2026-09-10 across dash, bash 5.3.15, that build as `sh`, bash
// 3.2.57, ksh93u+ and zsh 5.9.2, with `u` unset. Five of the six write `A}B`
// for the first row and bash 3.2 writes `A\}B`; the other rows are unanimous.
// See the corpus rows core/backslash-before-a-brace-in-a-quoted-operand and
// core/backslash-before-a-brace-in-a-replacement-operand.
func TestABackslashEscapesTheBraceThatWouldCloseAQuotedOperand(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the closing brace is escaped and the backslash goes", `printf "%s" "${u-A\}B}"`, "A}B"},
		{"an opening brace keeps its backslash", `printf "%s" "${u-A\{B}"`, `A\{B`},
		{"and so does an ordinary character", `printf "%s" "${u-A\qB}"`, `A\qB`},

		// The four the double-quote rule already escaped, which must not
		// have moved: the brace joined that set rather than replacing it.
		{"a dollar is still escaped", `printf "%s" "${u-A\$B}"`, `A$B`},
		{"a backslash is still escaped", `printf "%s" "${u-A\\B}"`, `A\B`},
		{"a double quote is still escaped", `printf "%s" "${u-A\"B}"`, `A"B`},

		// A single quote is an ordinary character in a quoted operand, so it
		// opens nothing and the brace inside it is escaped like any other.
		{"single quotes do not protect it", `printf "%s" "${u-A'\}'B}"`, `A'}'B`},

		// Outside an expansion the same text is a plain double-quoted run,
		// where nothing has to be escaped to be written — the row that says
		// the rule belongs to the operand and not to double quotes.
		{"but not in an ordinary double-quoted run", `printf "%s" "A\}B"`, `A\}B`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := operandLiteral(t, tc.src); got != tc.want {
				t.Errorf("%s: operand text = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// A `"` written inside the operand opens a run of its own, and the brace is
// not escapable in one. That is zsh's answer and the other five shells escape
// it there too, which is a split with no axis for it yet (#1971) — this row
// pins what the parser does today rather than endorsing it.
func TestTheBraceIsNotEscapableInsideANestedQuotedRun(t *testing.T) {
	const src = `printf "%s" "${u-"A\}B"}"`
	if got, want := operandLiteral(t, src), `A\}B`; got != want {
		t.Errorf("%s: operand text = %q, want %q", src, got, want)
	}
}

// The same escape in both halves of a substitution, which is the route
// powerlevel10k takes: it builds `${NAME-<sep>\}` text and re-reads it under
// `${(e)}`, and a kept backslash makes the built text unparseable.
func TestTheBraceIsEscapedInBothHalvesOfASubstitution(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		pattern         bool
	}{
		{name: "the replacement half", src: `printf "%s" "${v/x/A\}B}"`, want: "A}B"},
		{name: "the replacement half of the global form", src: `printf "%s" "${v//x/A\}B}"`, want: "A}B"},
		{name: "and the pattern half", src: `printf "%s" "${v/a\}b/Z}"`, want: "a}b", pattern: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := substitutionOf(t, tc.src)
			arg := e.Arg2
			if tc.pattern {
				arg = e.Arg
			}
			// Both readings of a replacement are built where they could
			// differ, and both have to carry the freed brace: a dialect that
			// takes the enclosing one must not get the backslash back.
			if got := wordText(arg); got != tc.want {
				t.Errorf("%s: operand text = %q, want %q", tc.src, got, tc.want)
			}
			if !tc.pattern && e.Arg2Enclosed != nil {
				if got := wordText(e.Arg2Enclosed); got != tc.want {
					t.Errorf("%s: enclosed reading = %q, want %q", tc.src, got, tc.want)
				}
			}
		})
	}
}

// operandLiteral is the text of the one operand in src, which every row above
// writes as a single literal run.
func operandLiteral(t *testing.T, src string) string {
	t.Helper()
	f, err := Parse(src, Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	// The last argument, so the format string every row shares is not what
	// gets read back — an assertion that matched the first literal it found
	// answered `%s` for every row and passed none of them.
	w := sc.Args[len(sc.Args)-1]
	for _, s := range w.Spans {
		if s.Kind == ParamExp && s.Param != nil && s.Param.Arg != nil {
			return wordText(s.Param.Arg)
		}
	}
	return wordText(w)
}

// substitutionOf is the one `${v/…/…}` in src.
func substitutionOf(t *testing.T, src string) *ParamExpr {
	t.Helper()
	f, err := Parse(src, Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	for _, s := range sc.Args[len(sc.Args)-1].Spans {
		if s.Kind == ParamExp && s.Param != nil && s.Param.Op == ParamReplace {
			return s.Param
		}
	}
	t.Fatalf("no substitution in %q", src)
	return nil
}
