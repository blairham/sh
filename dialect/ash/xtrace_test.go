// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
)

// What `set -x` does here, and every expectation below is a cell of the
// committed record or a line of the sweep behind #2443 — BusyBox v1.37.0,
// measured 2026-09-13 from a script file under `env -i PATH=/usr/bin:/bin`.
//
// It is worth saying why this file exists at all rather than only the runtime
// assertions. `dialect/ash` wrote *nothing* about tracing for the whole life
// of the ash column, so it took the zero value and printed every traced word
// bare, like dash — while the record said, on the very first row anybody
// looks at, that this shell quotes. Nothing failed, because nothing compared
// a preset with the record.

func TestTraceAnswersAreThisShellsAndNotDashs(t *testing.T) {
	d := ash.Diagnostics()
	// The spelling. Not QuoteShell: the run of quotes goes inside double
	// quotes rather than being backslash-escaped, there is no `$'…'` to fall
	// back to, and an empty field is written as nothing.
	if got, want := d.TraceQuoting, interp.QuoteSingleOnly; got != want {
		t.Errorf("TraceQuoting = %v, want %v", got, want)
	}
	// The alphabet, and the two characters that make it nobody else's: `%`
	// is in it and no other panel member quotes that, and `]` is out of it
	// and every other quoting member has it.
	if got, want := d.TraceMetacharacters.Anywhere, "*?[{}~#!=%"; got != want {
		t.Errorf("TraceMetacharacters.Anywhere = %q, want %q", got, want)
	}
	if strings.ContainsAny(d.TraceMetacharacters.Anywhere, "]^") {
		t.Errorf("TraceMetacharacters.Anywhere = %q, want no `]` and no `^`", d.TraceMetacharacters.Anywhere)
	}
	// No position rule at all, which sides with ksh93 and zsh against bash:
	// `~a` and `a~b` are both quoted here.
	if got := d.TraceMetacharacters.Leading; got != "" {
		t.Errorf("TraceMetacharacters.Leading = %q, want none: this shell quotes `a~b` as well as `~a`", got)
	}
	// The bracket line reads a fourth way and needs no exemption to say it —
	// see the runtime assertion below for what that looks like.
	if got, want := d.TraceBareBracket, interp.TraceBracketQuotedLikeAnyWord; got != want {
		t.Errorf("TraceBareBracket = %v, want %v", got, want)
	}
}

// TestTheTraceQuotesWhereDashWouldNot is the answer above reached through the
// shell rather than read off the struct, on the row the record has held since
// this column existed.
func TestTheTraceQuotesWhereDashWouldNot(t *testing.T) {
	out, _ := run(t, `set -x; x="hello wor"; echo "$x"`)
	for _, want := range []string{"+ x='hello wor'\n", "+ echo 'hello wor'\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("run = %q, want %q", out, want)
		}
	}
}

// TestTheTracedWordsAreThisShellsAlphabet walks the rows that separate this
// shell's set from each of the other three at once.
func TestTheTracedWordsAreThisShellsAlphabet(t *testing.T) {
	for _, tc := range []struct{ src, want, why string }{
		// The unanimous half among the shells that quote at all.
		{
			`set -x; echo '[1]' 'a*b' 'a?b' '{a,b}'`, `+ echo '[1]' 'a*b' 'a?b' '{a,b}'`,
			"a pattern or brace metacharacter",
		},
		// No leading-only rule: bash leaves `a~b` and `a#b` bare.
		{
			`set -x; echo '~a' 'a~b' '#a' 'a#b'`, `+ echo '~a' 'a~b' '#a' 'a#b'`,
			"`~` and `#` anywhere, not only at the front",
		},
		// `!` and `=` anywhere, `^` nowhere — which is no other member's
		// combination of the three.
		{
			`set -x; echo '^ab' '!ab' '=ab' 'ab=' 'a=b'`, `+ echo ^ab '!ab' '=ab' 'ab=' 'a=b'`,
			"`!` and `=` anywhere and `^` nowhere",
		},
		// The two rows that are this shell alone in the whole panel.
		{
			`set -x; echo 'a%b' 'a]b' 'a^b'`, `+ echo 'a%b' a]b a^b`,
			"`%` quoted and `]` bare, which is the opposite of the other three",
		},
		// The characters no member quotes.
		{
			`set -x; echo 'a+b' 'a,b' 'a-b' 'a.b' 'a/b' 'a:b' 'a@b' 'a_b'`,
			`+ echo a+b a,b a-b a.b a/b a:b a@b a_b`,
			"the punctuation nothing quotes",
		},
	} {
		out, _ := run(t, tc.src)
		if !strings.Contains(out, tc.want+"\n") {
			t.Errorf("%s: run = %q, want a line %q", tc.why, out, tc.want)
		}
	}
}

// TestTheBracketsOfATestAreQuotedByTheAlphabet is the fourth reading of the
// bracket line. The opening `[` is quoted like any other word — so this shell
// is not ksh93 or zsh — and every `]` is bare, which is not bash either. The
// second is the alphabet and not an exemption, and the third row is what says
// so: a `]` handed to `echo`, nowhere near a test, is bare as well.
func TestTheBracketsOfATestAreQuotedByTheAlphabet(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`set -x; [ 1 -lt 2 ]`, `+ '[' 1 -lt 2 ]`},
		{`set -x; [ -n ']' ]`, `+ '[' -n ] ]`},
		{`set -x; echo ']' '[' 'a[b'`, `+ echo ] '[' 'a[b'`},
	} {
		out, _ := run(t, tc.src)
		if !strings.Contains(out, tc.want+"\n") {
			t.Errorf("run = %q, want a line %q", out, tc.want)
		}
	}
}

// TestTheTraceHasNoDollarQuote is the half of QuoteSingleOnly that the
// `hello wor` row cannot see, because a word with only a space in it looks
// the same under bash's answer and this one.
func TestTheTraceHasNoDollarQuote(t *testing.T) {
	// An embedded quote: closed, and the quote carried through inside double
	// quotes. bash writes `'it'\''s'` and ksh93 `$'it\'s'`.
	out, _ := run(t, `set -x; x="it's"; echo "$x"`)
	if !strings.Contains(out, `+ echo 'it'"'"'s'`+"\n") {
		t.Errorf("embedded quote: run = %q", out)
	}
	// A control character: the byte itself, inside plain single quotes.
	out, _ = run(t, "set -x; x=\"$(printf 'a\\tb')\"; echo \"$x\"")
	if !strings.Contains(out, "+ echo 'a\tb'\n") {
		t.Errorf("control character: run = %q", out)
	}
	if strings.Contains(out, "$'") {
		t.Errorf("control character: run = %q, want no $'…' anywhere", out)
	}
	// And an empty field leaves no mark; the `a` after it is what says the
	// field is still there to be counted.
	out, _ = run(t, `set -x; echo '' a`)
	if !strings.Contains(out, "+ echo  a\n") {
		t.Errorf("empty field: run = %q", out)
	}
}
