// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// The older substitution form unescapes one layer of backslashes, and the set
// it unescapes is not fixed: it depends on where the substitution was
// written.
//
// Outside quotes it is the POSIX three — `$`, a backquote and another
// backslash. Inside double quotes a `\"` joins them, so the quote reaches the
// inner command as a quote and the command is lexed with it. Measured
// 2026-09-13 across bash 5.3, bash 3.2, bash as sh, dash, ksh93, zsh and zsh
// as sh: all seven print one field `a b` for `"`+"`"+`echo \"a b\"`+"`"+`"`.
//
// The assertion is on the span's Value — the command text handed on — rather
// than on whether the line parses, because both readings parse. The wrong one
// hands the inner shell `echo \"a b\"`, which runs and prints two words with
// quote marks in them, and a test that asked for a nil error would pass it.
func TestABackquotedBodyUnescapesAQuoteOnlyInsideDoubleQuotes(t *testing.T) {
	t.Parallel()
	const body = "echo " + `\"a b\"`
	for _, c := range []struct {
		src  string
		want string
	}{
		// The rule this exists for: inside double quotes the `\"` goes.
		{`echo "` + "`" + body + "`" + `"`, `echo "a b"`},
		// The same backquote outside them keeps it. This is the control that
		// tells a fix by position from a fix by character: one that unescaped
		// `\"` wherever it found it passes the row above and fails this.
		{`echo ` + "`" + body + "`", body},
		// A single-quoted context never reaches the scanner at all, so the
		// only two positions are the two above.
		{`echo "a` + "`" + body + "`" + `b"`, `echo "a b"`},
	} {
		f, err := Parse(c.src, Core())
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
			continue
		}
		got, ok := firstCommandSubst(f)
		if !ok {
			t.Errorf("%s: no command substitution in the tree", c.src)
			continue
		}
		if got.Value != c.want {
			t.Errorf("%s: body is %q, want %q", c.src, got.Value, c.want)
		}
	}
}

// The three the older spelling unescapes wherever it stands, and the one it
// never does. These are the guard on the widening above: a change that took
// the set from three to four by *position* leaves every one of them where it
// was, and a change that took it there by *character* would have moved none
// of them either — which is why the row above carries the discrimination and
// this one carries the floor.
func TestTheOlderSpellingAlwaysUnescapesTheSameThree(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		body string
		want string
	}{
		{`echo \$v`, `echo $v`},
		{`echo \\x`, `echo \x`},
		{"echo " + `\` + "`", "echo `"},
		// Before anything else the backslash is literal, in both positions.
		{`echo a\qb`, `echo a\qb`},
	} {
		for _, src := range []string{
			`echo "` + "`" + c.body + "`" + `"`,
			`echo ` + "`" + c.body + "`",
		} {
			f, err := Parse(src, Core())
			if err != nil {
				t.Errorf("%s: %v", src, err)
				continue
			}
			got, ok := firstCommandSubst(f)
			if !ok {
				t.Errorf("%s: no command substitution in the tree", src)
				continue
			}
			if got.Value != c.want {
				t.Errorf("%s: body is %q, want %q", src, got.Value, c.want)
			}
		}
	}
}

// firstCommandSubst is the first backquoted substitution anywhere in the
// first command's words, which is where every case above puts exactly one.
func firstCommandSubst(f *File) (Span, bool) {
	sc, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if !ok {
		return Span{}, false
	}
	for _, w := range sc.Args {
		for _, s := range w.Spans {
			if s.Kind == CommandSubst && s.Backquoted {
				return s, true
			}
		}
	}
	return Span{}, false
}
