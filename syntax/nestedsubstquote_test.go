// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"testing"
)

// A `$( )` written inside a double-quoted run holds a *program*, and a
// program brings its own quoting with it. The single quotes in
// `"$( echo 'a"b' )"` quote that `"`, so it is not the one that ends the run
// — the run ends at the `"` after the `)`.
//
// The scanners that only need to find a delimiter used to read such a run as
// text: the inner `"` closed it, the cursor was left on the second `'`, and
// everything to the end of the file became a single-quoted string. Four
// dialects refused `${x:-"$( grep '"' )"}` in four wordings, which is one
// defect surfacing four times rather than four bugs (#1140).
//
// These assertions are on the *tree* rather than on "it parses". A scan that
// ended the run at the wrong quote can still balance — see the silent row in
// TestASwallowedQuoteDoesNotEndTheWordEarly — and a check for a nil error
// passes it.
func TestASubstitutionInsideAnExpansionBringsItsOwnQuoting(t *testing.T) {
	for _, c := range []struct {
		src string
		// body is the whole text between `${` and its `}`.
		body string
		// inner is the one span the operand word holds, and what kind it is.
		inner     string
		innerKind SpanKind
	}{
		// The shape `~/.zi/bin/lib/zsh/install.zsh` stops at on line 1389,
		// reduced: a single-quoted `"` inside a `$( )` inside a `${ }`.
		{`echo "${x:-"$( echo 'a"b' )"}"`, `x:-"$( echo 'a"b' )"`, ` echo 'a"b' `, CommandSubst},
		// The mirror, so neither quote character is the special one.
		{`echo "${x:-"$( echo "c'd" )"}"`, `x:-"$( echo "c'd" )"`, ` echo "c'd" `, CommandSubst},
		// The operator does not matter — the body scanner runs before any of
		// them is read.
		{`echo "${x-"$( echo 'a"b' )"}"`, `x-"$( echo 'a"b' )"`, ` echo 'a"b' `, CommandSubst},
		{`echo "${x:+"$( echo 'a"b' )"}"`, `x:+"$( echo 'a"b' )"`, ` echo 'a"b' `, CommandSubst},
		{`echo "${x%"$( echo 'a"b' )"}"`, `x%"$( echo 'a"b' )"`, ` echo 'a"b' `, CommandSubst},
		// The arithmetic spelling nests a `$( )` of its own, so the skip has
		// to recur rather than take the first `)`.
		{`echo "${x:-"$(( 1 + $( echo '"' | wc -c ) ))"}"`, `x:-"$(( 1 + $( echo '"' | wc -c ) ))"`, ` 1 + $( echo '"' | wc -c ) `, ArithSubst},
		// A `$( )` reached from the body's own level rather than from inside
		// a double quote: the run that misreads is the one *inside* it.
		{`echo "${x:-$( echo "$( echo 'a"b' )" )}"`, `x:-$( echo "$( echo 'a"b' )" )`, ` echo "$( echo 'a"b' )" `, CommandSubst},
	} {
		f, err := Parse(c.src, Core())
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
			continue
		}
		sc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
		if len(sc.Args) != 2 {
			t.Errorf("%s: %d words, want 2 — the expansion swallowed or split", c.src, len(sc.Args))
			continue
		}
		span := sc.Args[1].Spans[0]
		if span.Kind != ParamExp {
			t.Errorf("%s: first span is %v, want a parameter expansion", c.src, span.Kind)
			continue
		}
		if span.Value != c.body {
			t.Errorf("%s: body is %q, want %q", c.src, span.Value, c.body)
			continue
		}
		operand := span.Param.Arg
		if operand == nil || len(operand.Spans) != 1 {
			t.Errorf("%s: operand is %v, want one span", c.src, operand)
			continue
		}
		got := operand.Spans[0]
		if got.Kind != c.innerKind || got.Value != c.inner {
			t.Errorf("%s: operand span is %v %q, want %v %q", c.src, got.Kind, got.Value, c.innerKind, c.inner)
		}
		if got.Quoting != DoubleQuoted && got.Quoting != Unquoted {
			t.Errorf("%s: operand span quoting is %v", c.src, got.Quoting)
		}
	}
}

// The reason a test for this cannot ask whether the source parses.
//
// Two of the shape in one line put the stray quotes back in balance: nothing
// is ever unterminated, no diagnostic is raised, and the program that runs is
// a different one. This is the row that would survive a fix graded by exit
// status — it exited 0 before the fix and it exits 0 after.
//
// The word count is the assertion, because that is what the misreading
// changes: the `}` of the first expansion became text, the two expansions
// became one word, and `[a"b} c'd]` is what the shell printed.
func TestASwallowedQuoteDoesNotEndTheWordEarly(t *testing.T) {
	const src = `printf '[%s]' "${x:-"$( echo 'a"b' )"}" "${y:-"$( echo "c'd" )"}"`
	f, err := Parse(src, Core())
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	sc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	if len(sc.Args) != 4 {
		t.Fatalf("%d words, want 4 — printf, the format and one word per expansion", len(sc.Args))
	}
	for i, want := range []string{`x:-"$( echo 'a"b' )"`, `y:-"$( echo "c'd" )"`} {
		w := sc.Args[i+2]
		if len(w.Spans) != 1 || w.Spans[0].Kind != ParamExp {
			t.Errorf("word %d is %q, want one parameter expansion", i+2, w.Literal())
			continue
		}
		if w.Spans[0].Value != want {
			t.Errorf("word %d body is %q, want %q", i+2, w.Spans[0].Value, want)
		}
	}
}

// The neighbors the change must not move.
//
// Skipping a nested substitution whole is a way of *finding* a delimiter, so
// what it must not do is find one that is not there. Input that really does
// run out inside one of these still runs out.
//
// Which delimiter the refusal blames is deliberately not asserted for the
// nested rows, and the reason is a measurement rather than an omission (#1151):
// the
// panel gives four different answers to `echo "${x:-"$( echo 'a"b'` — bash
// names the `)`, ksh93 names the `(`, zsh names the `"`, dash names the `)`
// in one of these two shapes and the `'` in the other — and this shell names
// the `"` in all four dialects. That is a pre-existing gap in the wording,
// unchanged by this file's subject: main answers these two rows byte for
// byte as they are answered now. Pinning the current wording here would
// record it as settled when it is not.
//
// What is asserted is the part this change owns — that the input is still
// refused, and refused as unmatched rather than accepted or reported as
// something else.
func TestANestedSubstitutionThatNeverClosesStillRunsOut(t *testing.T) {
	for _, src := range []string{
		`echo "${x:-"$( echo 'a"b'`,
		"echo \"${x:-\"$( echo 'a\"b'\necho ok",
		// A single quote inside the substitution that never closes either.
		`echo "${x:-"$( echo 'a"b )"}"`,
		// The arithmetic spelling, whose two closers the skip counts as one
		// depth of two — a skip that stopped at the first `)` would accept
		// this.
		`echo "${x:-"$(( 1 + $( echo 2 )`,
	} {
		_, err := Parse(src, Core())
		var se *Error
		if !errors.As(err, &se) || se.Kind != ErrUnmatched {
			t.Errorf("%q: got %v, want an ErrUnmatched", src, err)
		}
	}
}

// The rows from TestUnmatchedDelimitersCarryTheirState that run through the
// same scanners, kept here as well because this file is what would break
// them: an expansion with nothing nested inside it still carries the opener,
// the closer that never came, and the word it was written in (#1022, #1023).
func TestAnExpansionWithNothingNestedIsUnchanged(t *testing.T) {
	for _, c := range []struct{ src, opener, closer, near string }{
		{`echo "${x`, `"`, `"`, `"${x`},
		{`echo ${x`, `${`, `}`, `${x`},
		{`echo $(echo`, `$(`, `)`, `$(echo`},
		{`echo "$(echo`, `$(`, `)`, `"$(echo`},
	} {
		_, err := Parse(c.src, Core())
		var se *Error
		if !errors.As(err, &se) || se.Kind != ErrUnmatched {
			t.Errorf("%q: got %v, want an ErrUnmatched", c.src, err)
			continue
		}
		if se.Token != c.opener || se.Expected != c.closer {
			t.Errorf("%q: opener %q closer %q, want %q %q", c.src, se.Token, se.Expected, c.opener, c.closer)
		}
		if se.LastToken != c.near {
			t.Errorf("%q: near %q, want %q", c.src, se.LastToken, c.near)
		}
	}
}

// The nesting the skip has to count rather than take the first `)` of.
//
// These are the rows a first pass of the fix passed by luck. A skip that
// stopped one parenthesis early left the leftover `)` for the enclosing
// double-quoted run, which advanced over it and closed in the right place —
// so `"${x:-"$(( 1 + 2 ))"}"` reads the same either way and measures
// nothing. What tells them apart is text *after* the arithmetic that the
// substitution still owns: with the skip one short, the `"` in `'a"b'` is
// reached by the run rather than by the program, and the word ends there.
func TestTheSkipCountsTheNestingRatherThanTakingTheFirstCloser(t *testing.T) {
	for _, c := range []struct{ src, body, inner string }{
		// `$(( ))` closes with two parentheses, and the single-quoted `"`
		// after it is inside the substitution, not after it.
		{
			`echo "${x:-"$( echo $(( 1 + 2 )) 'a"b' )"}"`,
			`x:-"$( echo $(( 1 + 2 )) 'a"b' )"`,
			` echo $(( 1 + 2 )) 'a"b' `,
		},
		// A subshell's parentheses are nesting too, and nothing marks them
		// as belonging to a substitution.
		{
			`echo "${x:-"$( ( echo a ) ; echo 'p"q' )"}"`,
			`x:-"$( ( echo a ) ; echo 'p"q' )"`,
			` ( echo a ) ; echo 'p"q' `,
		},
		// The older spelling, which all six of the panel accept in this
		// position — including the one that refuses it when the word stands
		// on its own.
		{
			"echo \"${x:-\"`echo 'e\"f'`\"}\"",
			"x:-\"`echo 'e\"f'`\"",
			"echo 'e\"f'",
		},
		// And the older spelling is a *program* too, so the parentheses in
		// it are the program's. A `case` arm's `)` closes nothing, and
		// counting past it is what scanParens learned the hard way — the
		// same lesson one level further in, where the backquotes are what
		// hides it.
		{
			"echo \"${x:-\"$( echo `case a in a) echo y;; esac` )\"}\"",
			"x:-\"$( echo `case a in a) echo y;; esac` )\"",
			" echo `case a in a) echo y;; esac` ",
		},
	} {
		f, err := Parse(c.src, Core())
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
			continue
		}
		sc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
		if len(sc.Args) != 2 {
			t.Errorf("%s: %d words, want 2", c.src, len(sc.Args))
			continue
		}
		span := sc.Args[1].Spans[0]
		if span.Kind != ParamExp || span.Value != c.body {
			t.Errorf("%s: body is %v %q, want a parameter expansion %q", c.src, span.Kind, span.Value, c.body)
			continue
		}
		operand := span.Param.Arg
		if operand == nil || len(operand.Spans) != 1 {
			t.Errorf("%s: operand is %v, want one span", c.src, operand)
			continue
		}
		if got := operand.Spans[0]; got.Kind != CommandSubst || got.Value != c.inner {
			t.Errorf("%s: operand span is %v %q, want a command substitution %q", c.src, got.Kind, got.Value, c.inner)
		}
	}
}

// The other quote character, and it does not follow the same rule.
//
// A `${ }` body written inside double quotes is double-quoted *content*, so a
// single quote in it is an ordinary character: it quotes nothing, and what
// stands between two of them is still substituted. Measured 2026-09-07,
// unanimous across bash 5.3.15, bash 3.2.57, that build invoked as `sh`, dash,
// ksh93u+ and zsh 5.9.2:
//
//	v=VAL; printf '[%s]' "${u:-'$v'}"     ['VAL']
//	printf '[%s]' "${u:-'$(echo hi)'}"    ['hi']
//
// So the substitution inside the quotes is recognized and *performed*, which
// is the row that separates that reading from merely scanning past it for a
// delimiter — and with the `'` an ordinary character there is nothing to end
// an unbalanced `$(`, which is why every shell in the panel refuses the first
// two rows below (#1150).
//
// The body the lexer captures is unchanged by any of this: where a `}` inside
// a quoted run closes the expansion is a separate question, the panel divides
// on it, and nothing here answers it.
func TestASingleQuoteInAQuotedBodyIsAnOrdinaryCharacter(t *testing.T) {
	for _, src := range []string{
		`echo "${x:-'a$(b'}"`,
		"echo \"${x:-'a`b'}\"",
	} {
		if _, err := Parse(src, Core()); err == nil {
			t.Errorf("%s: parsed, want the substitution the quote no longer closes to be refused", src)
		}
	}

	// The balanced spelling parses, and the spans say which reading it got:
	// the quotes are literal text around a substitution rather than a run
	// holding two ordinary bytes.
	f, err := Parse(`echo "${x:-'a$(b)c'}"`, Core())
	if err != nil {
		t.Fatalf("balanced: %v", err)
	}
	sc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	span := sc.Args[1].Spans[0]
	if span.Kind != ParamExp || span.Value != `x:-'a$(b)c'` {
		t.Fatalf("body is %v %q, want the whole of it captured", span.Kind, span.Value)
	}
	spans := span.Param.Arg.Spans
	want := []struct {
		kind  SpanKind
		value string
	}{{Literal, `'a`}, {CommandSubst, `b`}, {Literal, `c'`}}
	if len(spans) != len(want) {
		t.Fatalf("operand is %d spans, want %d: %v", len(spans), len(want), spans)
	}
	for i, w := range want {
		if spans[i].Kind != w.kind || spans[i].Value != w.value {
			t.Errorf("span %d is %v %q, want %v %q", i, spans[i].Kind, spans[i].Value, w.kind, w.value)
		}
	}

	// And unquoted the single quote is a quote again, which is what says the
	// rule belongs to the enclosing context rather than to the body: the same
	// characters are one literal run there, and the unbalanced spelling
	// parses because there is no substitution in it to leave open.
	for _, tc := range []struct{ src, value string }{
		{`echo ${x:-'a$(b)c'}`, `a$(b)c`},
		{`echo ${x:-'a$(b'}`, `a$(b`},
	} {
		f, err := Parse(tc.src, Core())
		if err != nil {
			t.Errorf("%s: %v", tc.src, err)
			continue
		}
		sc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
		spans := sc.Args[1].Spans[0].Param.Arg.Spans
		if len(spans) != 1 || spans[0].Kind != Literal || spans[0].Value != tc.value {
			t.Errorf("%s: operand is %v, want the one literal %q", tc.src, spans, tc.value)
		}
	}
}

// What the skip has to keep tracking once it is inside the substitution.
//
// The delimiter it is looking for can be written inside the program without
// closing anything, and every way of writing it that way has to hold: in a
// double-quoted string, behind a backslash, and in the single quotes this
// file's subject is about. All four rows are unanimous across the panel.
func TestTheSkipTracksQuotingInsideTheSubstitutionToo(t *testing.T) {
	for _, c := range []struct{ src, inner string }{
		// A `)` and a `}` inside a double-quoted string of the program.
		{`echo "${x:-"$( echo "a)b" )"}"`, ` echo "a)b" `},
		{`echo "${x:-"$( echo "a}b" )"}"`, ` echo "a}b" `},
		// The same two behind a backslash rather than inside a quote.
		{`echo "${x:-"$( echo \) )"}"`, ` echo \) `},
		{`echo "${x:-"$( echo \" )"}"`, ` echo \" `},
	} {
		f, err := Parse(c.src, Core())
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
			continue
		}
		sc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
		if len(sc.Args) != 2 {
			t.Errorf("%s: %d words, want 2", c.src, len(sc.Args))
			continue
		}
		span := sc.Args[1].Spans[0]
		if span.Kind != ParamExp || span.Param.Arg == nil || len(span.Param.Arg.Spans) != 1 {
			t.Errorf("%s: got %v, want an expansion whose operand is one span", c.src, span.Kind)
			continue
		}
		if got := span.Param.Arg.Spans[0]; got.Kind != CommandSubst || got.Value != c.inner {
			t.Errorf("%s: operand span is %v %q, want a command substitution %q", c.src, got.Kind, got.Value, c.inner)
		}
	}
}

// The same sentence for the other construct, which was left out of it.
//
// A `${ }` written inside a double-quoted run brings its own quoting too. Its
// operand is a word rather than a program, but a word is quoted just as a
// program is, so the `"` in `"${y:-"Z"}"` belongs to that expansion and is not
// the one that ends the run.
//
// Reading the run character by character does not merely mispair the quotes,
// it moves the *braces*: the nested `${` is consumed inside the quote and
// never counted, and its `}` then lands outside and closes the enclosing
// expansion. So `"${u:-"${w:-"X{039}"}z"}"` ended at the `}` of `X{039}` and
// left `"}z"}` behind as literal text — which is the `"}`, `}+}` and `}}}+}`
// debris powerlevel10k's directory segment drew, one fragment per expansion
// closed early (#2092).
//
// Measured 2026-09-11 from a script file under `env -i`, unanimous across the
// six-shell panel — zsh 5.9.2, bash 5.3.15, that build as `sh`, bash 3.2.57,
// dash and ksh93:
//
//	unset u w; printf '[%s]' "${u:-"${w:-"X{039}"}z"}"    [X{039}z]
//	unset u w; printf '[%s]' "${u:-"${w:-"a}b"}z"}"       [a}bz]
//
// so it is the core's answer and not a dialect's.
//
// The assertions are on the tree for the reason the `$( )` half's are: a scan
// that ends the run at the wrong quote can still balance, and a check for a
// nil error passes it. The *quoting* of the operand's spans is asserted with
// their text, because the text alone would not part the readings — a run
// misread as ending early puts the same characters back as one raw literal.
func TestABracedExpansionInsideAQuotedRunBringsItsOwnQuoting(t *testing.T) {
	for _, c := range []struct {
		src string
		// body is the whole text between `${` and its `}`.
		body string
		// inner is the nested expansion the operand holds, and tail is the
		// literal text after it — the half that used to fall out of the word.
		inner, tail string
	}{
		// The shape powerlevel10k's directory segment is written in,
		// reduced: a `{ }` pair inside the nested expansion's own quotes.
		{`echo "${x:-"${y:-"X{039}"}z"}"`, `x:-"${y:-"X{039}"}z"`, `y:-"X{039}"`, `z`},
		// A bare `}` inside them, which is the character the enclosing scan
		// used to spend on itself.
		{`echo "${x:-"${y:-"a}b"}z"}"`, `x:-"${y:-"a}b"}z"`, `y:-"a}b"`, `z`},
		// Nothing unbalanced at all: the run still has to end after the
		// nested expansion rather than inside it.
		{`echo "${x:-"${y:-"a"}z"}"`, `x:-"${y:-"a"}z"`, `y:-"a"`, `z`},
		// Three levels, because the theme writes them: the middle one is
		// reached from inside a run that is itself inside a run.
		{`echo "${x:-"${y:-"${w:-"p{q}r"}s"}t"}"`, `x:-"${y:-"${w:-"p{q}r"}s"}t"`, `y:-"${w:-"p{q}r"}s"`, `t`},
		// The operator does not matter — the body scanner runs before any of
		// them is read.
		{`echo "${x-"${y:-"X{039}"}z"}"`, `x-"${y:-"X{039}"}z"`, `y:-"X{039}"`, `z`},
		{`echo "${x%"${y:-"X{039}"}z"}"`, `x%"${y:-"X{039}"}z"`, `y:-"X{039}"`, `z`},
	} {
		f, err := Parse(c.src, Core())
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
			continue
		}
		sc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
		if len(sc.Args) != 2 {
			t.Errorf("%s: %d words, want 2 — the expansion swallowed or split", c.src, len(sc.Args))
			continue
		}
		span := sc.Args[1].Spans[0]
		if span.Kind != ParamExp {
			t.Errorf("%s: first span is %v, want a parameter expansion", c.src, span.Kind)
			continue
		}
		if span.Value != c.body {
			t.Errorf("%s: body is %q, want %q", c.src, span.Value, c.body)
			continue
		}
		operand := span.Param.Arg
		if operand == nil || len(operand.Spans) != 2 {
			t.Errorf("%s: operand is %v, want the nested expansion and its tail", c.src, operand)
			continue
		}
		if got := operand.Spans[0]; got.Kind != ParamExp || got.Value != c.inner {
			t.Errorf("%s: operand span 0 is %v %q, want a parameter expansion %q", c.src, got.Kind, got.Value, c.inner)
		}
		if got := operand.Spans[1]; got.Kind != Literal || got.Value != c.tail {
			t.Errorf("%s: operand span 1 is %v %q, want the literal %q", c.src, got.Kind, got.Value, c.tail)
		}
		// The quoting is the attribute the text cannot stand in for. Both
		// spans are written inside the run, so both carry it — and a reading
		// that ended the run early would hand back what it had swallowed as
		// one raw literal with none.
		for i, got := range operand.Spans {
			if got.Quoting != DoubleQuoted {
				t.Errorf("%s: operand span %d quoting is %v, want double-quoted", c.src, i, got.Quoting)
			}
		}
	}
}

// The route the prompt takes, and the one where the misreading said nothing.
//
// A here-document body is read by HeredocSpans rather than by a word scanner,
// which is also how a `${(%%)…}` prompt reaches this code. There is no
// enclosing word for the leftover quote to unbalance, so the mis-scan raised
// no diagnostic at all: it wrote `[X{039"}z"}]` and exited 0, where every
// shell in the panel writes `[X{039}z]`. That is the row a fix graded on exit
// status would pass in both directions.
func TestABracedExpansionInsideAQuotedRunInARawBody(t *testing.T) {
	// The flag is on for both rows, because it is what the second one is
	// about: a bare `{` opens a level in the dialects that have it, and only
	// there can a step-over that forgets the quoting be told from one that
	// keeps it. The core has it off, so the core reads both rows the same
	// whichever way the nested expansion is stepped over.
	nests := Core()
	nests.BareBraceNestsInExpansion = true
	for _, c := range []struct {
		body  string
		spans []struct {
			kind  SpanKind
			value string
		}
	}{
		{
			body: `[${u:-"${w:-"X{039}"}z"}]`,
			spans: []struct {
				kind  SpanKind
				value string
			}{{Literal, `[`}, {ParamExp, `u:-"${w:-"X{039}"}z"`}, {Literal, `]`}},
		},
		{
			// The run is still a run once the step-over is inside it, which
			// is what says the nested expansion is read *in double quotes*
			// rather than as a word standing on its own: a bare `{` opens no
			// level there, so `${w:-a{b}` ends at the first `}` and what
			// follows is the enclosing expansion's again. Read as bare, the
			// `{` would open one, the nested expansion would run past the
			// `}` that closes it, and the line would be refused as `closing
			// brace expected`.
			//
			// Measured 2026-09-11 in a here-document body under `env -i`:
			// zsh 5.9.2, bash 5.3.15, that build as `sh`, bash 3.2.57 and
			// dash all write `[a{bcz"}]`. ksh93 refuses the line, which is
			// its own divergence — it is the panel member that balances a
			// bare brace hardest — and no dialect here answers for it.
			body: `[${u:-"${w:-a{b}c"}z"}]`,
			spans: []struct {
				kind  SpanKind
				value string
			}{{Literal, `[`}, {ParamExp, `u:-"${w:-a{b}c"`}, {Literal, `z"}]`}},
		},
	} {
		spans, err := HeredocSpans(c.body, nests)
		if err != nil {
			t.Errorf("%s: %v", c.body, err)
			continue
		}
		if len(spans) != len(c.spans) {
			t.Errorf("%s: %d spans, want %d: %v", c.body, len(spans), len(c.spans), spans)
			continue
		}
		for i, w := range c.spans {
			if spans[i].Kind != w.kind || spans[i].Value != w.value {
				t.Errorf("%s: span %d is %v %q, want %v %q", c.body, i, spans[i].Kind, spans[i].Value, w.kind, w.value)
			}
		}
	}
}

// The neighbors this second half must not move.
//
// Stepping over a nested `${ }` is a way of *finding* a delimiter, so what it
// must not do is find one that is not there: input that really does run out
// inside one still runs out, and is still refused as unmatched rather than
// accepted. Which delimiter is blamed is deliberately not asserted, for the
// reason recorded on the `$( )` rows above.
func TestANestedBracedExpansionThatNeverClosesStillRunsOut(t *testing.T) {
	for _, src := range []string{
		`echo "${x:-"${y`,
		`echo "${x:-"${y:-"X{039}"`,
		"echo \"${x:-\"${y:-\"X{039}\"\necho ok",
	} {
		_, err := Parse(src, Core())
		var se *Error
		if !errors.As(err, &se) || se.Kind != ErrUnmatched {
			t.Errorf("%q: got %v, want an ErrUnmatched", src, err)
		}
	}
}
