// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// globQuals is the core with the flag this file is about, plus the bare
// pattern group it rests on — a leading `(` becoming part of a word is only
// useful in a grammar where a group is pattern text at all.
func globQuals() Dialect {
	d := Core()
	d.PatternAlternation = true
	d.GlobQualifiers = true
	return d
}

// A `(` where an *argument* may stand belongs to the word, and where a
// command may begin it does not.
//
// Command position is the whole of the disambiguation, measured on zsh 5.9.2:
// `( x )` written first runs `x` in a subshell, and the same three characters
// after a word are one argument. So this cannot be a lexer rule on its own —
// the parser has to say which position it is — and every row below is a row
// about that rather than about parentheses.
func TestAParenWhereAnArgumentStandsBelongsToTheWord(t *testing.T) {
	on, off := globQuals(), Core()
	for _, src := range []string{
		`echo MY ( x )`,
		`echo ( x )`,
		`echo a ( b ) c`,
		`echo MY (x)`,
		`MY ( x )`,
		`echo *( x )`,
	} {
		mustParse(t, src, on, "a group where an argument stands")
		mustFail(t, src, off, "a paren after a word without the flag")
	}
	// And command position is untouched by the flag: these are the shapes
	// that would break if the lexer took every `(` it could reach.
	for _, src := range []string{
		`( x )`,
		`( echo hi )`,
		`echo hi | ( cat )`,
		`if ( true ); then echo y; fi`,
		`while ( false ); do :; done`,
		`x=$( ( echo hi ) )`,
		`case f1 in ( f1 ) echo m;; esac`,
		`echo a; ( echo b )`,
	} {
		mustParse(t, src, on, "a paren in command position")
	}
}

// The words it makes, which is the half a parse test can get wrong by only
// asking whether the input parsed.
//
// `MY ( x )` is **two** words there and not one: measured through
// `setopt no_glob; print -l MY ( x )`, which prints `MY` and `( x )` on
// separate lines, and through a function counting `$#`, which says 2.
func TestTheGroupIsOneWordAndDoesNotJoinThePrecedingOne(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want []string
	}{
		{`echo MY ( x )`, []string{"echo", "MY", "( x )"}},
		{`echo MY( x )`, []string{"echo", "MY( x )"}},
		{`echo ( x )`, []string{"echo", "( x )"}},
		{`echo a ( b ) c`, []string{"echo", "a", "( b )", "c"}},
		{`echo ( a|b )`, []string{"echo", "( a|b )"}},
		{`echo ((1))`, []string{"echo", "((1))"}},
		{`echo *(.)`, []string{"echo", "*(.)"}},
	} {
		f, err := Parse(tc.src, globQuals())
		if err != nil {
			t.Errorf("%s: %v", tc.src, err)
			continue
		}
		pipe, ok := f.Stmts[0].Expr.(*Pipeline)
		if !ok || len(pipe.Cmds) != 1 {
			t.Errorf("%s: not one command", tc.src)
			continue
		}
		cmd, ok := pipe.Cmds[0].(*SimpleCmd)
		if !ok {
			t.Errorf("%s: not a simple command", tc.src)
			continue
		}
		got := make([]string, 0, len(cmd.Args))
		for _, w := range cmd.Args {
			got = append(got, w.Literal())
		}
		if len(got) != len(tc.want) {
			t.Errorf("%s: %d words %q, want %d %q", tc.src, len(got), got, len(tc.want), tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: word %d = %q, want %q", tc.src, i, got[i], tc.want[i])
			}
		}
	}
}

// Argument position lasts as long as the command does and no longer, which a
// parse test alone cannot see: `echo a; ( echo b )` *parses* either way, and
// only what the second statement turned out to be says whether the flag was
// still set when the `(` was read.
func TestArgumentPositionEndsWithTheCommand(t *testing.T) {
	for _, src := range []string{
		`echo a; ( echo b )`,
		`x=1; ( echo b )`,
		`echo a
( echo b )`,
		`echo a | ( cat )`,
	} {
		f, err := Parse(src, globQuals())
		if err != nil {
			t.Errorf("%s: %v", src, err)
			continue
		}
		last := f.Stmts[len(f.Stmts)-1]
		pipe, ok := last.Expr.(*Pipeline)
		if !ok {
			t.Errorf("%s: last statement is not a pipeline", src)
			continue
		}
		if _, ok := pipe.Cmds[len(pipe.Cmds)-1].(*Subshell); !ok {
			t.Errorf("%s: the trailing `( … )` is %T, want a subshell", src, pipe.Cmds[len(pipe.Cmds)-1])
		}
	}
}

// The group ends the word at a shell operator, which is measured rather than
// assumed and is why it is not simply the balanced text.
//
// `echo ( a <b )` is `parse error near `)'` on zsh 5.9.2 — with globbing on
// *and* off, so it is the reading of the input rather than what became of it
// — because the `<` ended the word and left the `)` with nowhere to go. A `|`
// is the exception, a pattern group being allowed to hold an alternation.
func TestAnOperatorInTheGroupEndsTheWord(t *testing.T) {
	d := globQuals()
	for _, src := range []string{
		`echo ( a <b )`,
		`echo ( a;b )`,
		`echo ( a>b )`,
		`echo ( a&b )`,
		`coproc MY ( cat </dev/null )`,
	} {
		if _, err := Parse(src, d); err == nil {
			t.Errorf("%s: parsed, want the `)` left with nowhere to go", src)
		}
	}
	// The `)` is what is blamed, and not the `(` — which is the whole point
	// of ending the word at the operator rather than refusing the group.
	// zsh names the `)` here, and naming the `(` is what this answered
	// before the flag.
	if _, err := Parse(`echo ( a <b )`, d); err == nil || !containsText(err.Error(), `")" unexpected`) {
		t.Errorf("error = %v, want the `)` named", err)
	}
	// And a `|` does not end it.
	mustParse(t, `echo ( a|b )`, d, "an alternation inside the group")
}

func containsText(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
