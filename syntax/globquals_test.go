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

// arrayGlobQuals is globQuals with the array literal, which is the grammar
// the assignment cases below need and which Core() has not got.
func arrayGlobQuals() Dialect {
	d := globQuals()
	d.ArrayLiteral = true
	return d
}

// An element of an array literal stands where an argument does, so a `(` that
// begins one belongs to the element.
//
// The assignment used to find its closing `)` by counting, and a glob flag
// opens one that is part of a *word*: `files=( (#i)a )` was `expected ) to
// close an array assignment` where zsh accepts it, and so was every shape
// whose group stands at the *front* of an element. The three that stand after
// a pattern — `*(-.DN)`, `*~(*/*)` and `*.(zip|tgz)` — were already right,
// because mid-word the lexer folds a group without being told. Measured on
// zsh 5.9.2, `-n` over a script file under `env -i` (#1149).
func TestAParenWhereAnArrayElementBeginsBelongsToTheElement(t *testing.T) {
	on := arrayGlobQuals()
	off := Core()
	off.ArrayLiteral = true
	for _, src := range []string{
		`files=( (#i)a )`,
		`files=( x (#i)a )`,
		`files=( (#b)a )`,
		`files=( (#s)a )`,
		`files=( (#q-.) )`,
		`files=( (#i)a (#b)b )`,
		`a=( (a|b) )`,
		`a=( (echo x) )`,
		// The line `~/.zi/bin/lib/zsh/install.zsh` stopped on, whole: four
		// parenthesised shapes in one word, of which only the leading one
		// was the problem.
		`files=( (#i)**/*.(zip|rar|7z|tgz|tbz|tbz2|tar.gz|tar.bz2|tar.7z|txz|tar.xz|gz|xz|tar|dmg|exe)~(*/*|.(_backup|git))/*(-.DN) )`,
		// Across a newline, which is where the elements of a real array go.
		"files=( (#i)a\n(#b)b )",
		// And a declaration's operand, which reaches the same parser by
		// another route.
		`local files=( (#i)a )`,
		`typeset -a files=( (#i)a )`,
	} {
		mustParse(t, src, on, "a group where an array element begins")
		mustFail(t, src, off, "the same without the flag")
	}
	// The shapes that were already right, kept so a fix that reached them
	// through a different path would be noticed.
	for _, src := range []string{
		`files=( a(#i) )`,
		`files=( *(-.DN) )`,
		`files=( *~(*/*|.(_backup|git))/* )`,
		`files=( "(#i)a" )`,
		`files=( $(echo x) )`,
		`files=( )`,
		`files=()`,
		`files=( a b )`,
	} {
		mustParse(t, src, on, "an array literal that already parsed")
	}
}

// The elements it makes, which is the half a parse test cannot see: a group
// swallowed into the previous element, or into the assignment, still parses.
func TestAnArrayElementsGroupIsItsOwnElement(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want []string
	}{
		{`files=( (#i)a )`, []string{"(#i)a"}},
		{`files=( x (#i)a )`, []string{"x", "(#i)a"}},
		{`files=( (#i)a (#b)b )`, []string{"(#i)a", "(#b)b"}},
		{`files=( (#i)a* )`, []string{"(#i)a*"}},
		{`files=( a(#i) )`, []string{"a(#i)"}},
		{`files=( (a|b) )`, []string{"(a|b)"}},
		{`files=( )`, nil},
	} {
		f, err := Parse(tc.src, arrayGlobQuals())
		if err != nil {
			t.Errorf("%s: %v", tc.src, err)
			continue
		}
		cmd, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
		if !ok || len(cmd.Assigns) != 1 {
			t.Errorf("%s: not one assignment", tc.src)
			continue
		}
		a := cmd.Assigns[0]
		if !a.IsArray {
			t.Errorf("%s: not an array assignment", tc.src)
			continue
		}
		got := make([]string, 0, len(a.Elems))
		for _, w := range a.Elems {
			got = append(got, w.Literal())
		}
		if len(got) != len(tc.want) {
			t.Errorf("%s: %d elements %q, want %d %q", tc.src, len(got), got, len(tc.want), tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: element %d = %q, want %q", tc.src, i, got[i], tc.want[i])
			}
		}
	}
}

// Argument position is *restored* rather than turned off, and the difference
// is observable inside the same command.
//
// An assignment is read where a command may begin as well as after a word, so
// what the token after the array stands in is whatever it stood in before —
// and measured on zsh 5.9.2 the two answers differ: `a=( x ) (b)` written
// first is `parse error near '('`, where `local a=( x ) (b)` parses, the
// `local` having already made it argument position. Leaving the flag on after
// the array made the first of those parse.
//
// parseSimple's own defer clears the flag when the *command* ends, which is
// why `a=( x ); ( echo b )` cannot see this and the pair below can.
func TestTheArraysArgumentPositionIsRestoredAndNotCleared(t *testing.T) {
	d := arrayGlobQuals()
	d.DeclarationUtilities = map[string]bool{"local": true}
	for _, src := range []string{
		`a=( x ) (b)`,
		`a=( (#i)x ) (b)`,
		`a=( x ) ( echo b )`,
	} {
		mustFail(t, src, d, "a paren after an array at command position")
	}
	for _, src := range []string{
		`local a=( x ) (b)`,
		`local a=( (#i)x ) (b)`,
	} {
		mustParse(t, src, d, "a paren after an array in argument position")
	}
}

// Argument position ends with the command, which a parse test alone cannot
// see: `a=( x ); ( echo b )` parses either way, and only what the second
// statement turned out to be says which.
func TestTheArraysArgumentPositionEndsWithIt(t *testing.T) {
	for _, src := range []string{
		`a=( x ); ( echo b )`,
		`a=( (#i)x ); ( echo b )`,
		"a=( (#i)x )\n( echo b )",
	} {
		f, err := Parse(src, arrayGlobQuals())
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

// The group ends where it closes rather than swallowing the assignment's own
// `)`: zsh answers `files=( (#i)a) )` with a parse error at the stray `)`,
// which is only reachable if the group took one parenthesis and the array
// took the next.
func TestTheElementsGroupDoesNotSwallowTheAssignmentsParen(t *testing.T) {
	if _, err := Parse(`files=( (#i)a) )`, arrayGlobQuals()); err == nil {
		t.Error("parsed, want the stray `)` refused")
	}
}

// condGlobQuals is globQuals with `[[ ]]`, which is what the pattern
// operand's rows need and which Core() has but this file's dialect did not
// have to say until now.
func condGlobQuals() Dialect {
	d := globQuals()
	d.DoubleBracket = true
	return d
}

// A *pattern operand's* leading group ends at an operator too, which is the
// second route into scanArgumentGroup and was answered wrong until #1175.
//
// This is the route the `inArgument` guard used to divert: `[[ $k == (a<b) ]]`
// took the group whole and matched, where zsh refuses the `<` while parsing.
// Measured on zsh 5.9.2, 2026-09-07, from a script file under `env -i`, each
// probe in a file of its own so the first refusal does not hide the rest:
//
//	k='a<b'; [[ $k == (a<b) ]]    parse error near `<'
//	k='a>b'; [[ $k == (a>b) ]]    parse error near `>'
//	k='a;b'; [[ $k == (a;b) ]]    parse error near `;'
//	k='a&b'; [[ $k == (a&b) ]]    parse error near `&'
//	k=a;     [[ $k == (a|b) ]]    matches, status 0
//
// The whole rendered line is asserted, location included, because the
// position is the claim: the word ended *at the operator*, so the complaint
// has to land on the operator and not on the `(` in front of it or the `)`
// behind it.
func TestAPatternOperandsLeadingGroupEndsAtAnOperator(t *testing.T) {
	d := condGlobQuals()
	for _, tc := range []struct{ src, want string }{
		{`[[ $k == (a<b) ]]`, `1:12: "<" unexpected`},
		{`[[ $k == (a>b) ]]`, `1:12: ">" unexpected`},
		{`[[ $k == (a;b) ]]`, `1:12: ";" unexpected`},
		{`[[ $k == (a&b) ]]`, `1:12: "&" unexpected`},
	} {
		_, err := Parse(tc.src, d)
		if err == nil {
			t.Errorf("%s: parsed, want the operator refused", tc.src)
			continue
		}
		if got := err.Error(); got != tc.want {
			t.Errorf("%s: error = %q, want %q", tc.src, got, tc.want)
		}
	}
	// And the `|` still belongs to the group, which is what keeps this from
	// being "an operand may not hold a parenthesis".
	mustParse(t, `[[ $k == (a|b) ]]`, d, "an alternation in a pattern operand")
	// The same four are refused without the qualifier flag as well: this
	// route is `inPattern`, which rests on PatternAlternation alone, so the
	// rows above are not secretly about GlobQualifiers.
	bare := Core()
	bare.PatternAlternation = true
	if _, err := Parse(`[[ $k == (a<b) ]]`, bare); err == nil {
		t.Error("without GlobQualifiers: parsed, want the `<` refused")
	}
}

// A *regular expression's* operand is the other answer, and it does not reach
// scanArgumentGroup at all — scanWord takes a regex group whole at a case of
// its own, ahead of the one this file is about.
//
// The four shells with `=~` agree, measured 2026-09-07 over a script file
// under `env -i`. `[[ 'a<b' =~ (a<b) ]]` and `[[ 'a;b' =~ (a;b) ]]` both
// match in bash 5.3.15, bash 3.2.57, bash-as-`sh` and ksh93u+, and
// `[[ 'ab' =~ (a<b) ]]` does not — so the `<` is regex text there rather than
// a redirection. zsh is the dissenter and refuses the `<` while parsing,
// which is a wording question for that preset and not this rule.
//
// It is asserted here because #1175's whole complaint was that nothing did:
// a guard whose only live branch is untested reads as though it decided
// something.
func TestARegexOperandsLeadingGroupKeepsItsOperators(t *testing.T) {
	d := condGlobQuals()
	for _, src := range []string{
		`[[ $k =~ (a<b) ]]`,
		`[[ $k =~ (a>b) ]]`,
		`[[ $k =~ (a;b) ]]`,
		`[[ $k =~ (a&b) ]]`,
	} {
		mustParse(t, src, d, "a regex group holds its operators")
	}
	// Without the pattern-group flags either, because the regex route is
	// `inRegex` and rests on neither.
	for _, src := range []string{`[[ $k =~ (a<b) ]]`, `[[ $k =~ (a;b) ]]`} {
		mustParse(t, src, Core(), "a regex group holds its operators in the core")
	}
}
