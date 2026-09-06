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

// The boundary of the change, and it is a boundary rather than a claim.
//
// Only a *double-quoted* run steps over a nested substitution. A single
// quote has no specification beyond "nothing inside is special", so the run
// it opens is taken literally to the next `'` and a `$(` inside it is two
// ordinary bytes.
//
// That is what this shell does on both sides of the change, and it is not
// what the panel does: all four of bash, dash, ksh93 and zsh refuse
// `"${x:-'a$(b'}"` and `"${x:-'a` + "`" + `b'}"` — the `${ }` body is where they
// look inside single quotes and this shell does not. The divergence is
// pre-existing and unanimous, so it is a bug of its own (#1150) rather than a
// part of this one; the assertion here is that removing the guard is a change,
// so that it cannot be removed as tidying while the answer to that bug is
// still open.
func TestASingleQuotedRunIsTakenLiterally(t *testing.T) {
	for _, c := range []struct{ src, body string }{
		{`echo "${x:-'a$(b'}"`, `x:-'a$(b'`},
		{"echo \"${x:-'a`b'}\"", "x:-'a`b'"},
		// And the balanced spellings, where taking it literally and looking
		// inside it happen to end in the same place.
		{`echo "${x:-'a$(b)c'}"`, `x:-'a$(b)c'`},
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
