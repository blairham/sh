// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

func TestPickDialect(t *testing.T) {
	// A name resolves to a grammar *and* a semantics, because they answer
	// different questions about the same shell.
	for _, name := range []string{"core", "posix", "bash", "zsh", "ksh", "dash"} {
		if _, _, err := pickDialect(name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, _, err := pickDialect("nosuchshell"); err == nil {
		t.Error("an unknown dialect must be refused rather than defaulted")
	}

	// The pairings differ where the shells do, which is the point of having
	// two axes rather than one name.
	_, bash, _ := pickDialect("bash")
	_, zsh, _ := pickDialect("zsh")
	if bash.ArithLeadingZeroIsOctal == zsh.ArithLeadingZeroIsOctal {
		t.Error("bash and zsh should disagree about whether a leading zero is octal")
	}
}

func TestDetailShowsQuotingPerSpan(t *testing.T) {
	// The quoting is the part that decides what happens to a word later and
	// the part hardest to see by eye, so the dump has to make it visible.
	toks := syntax.NewLexer(`a"b c"d`, syntax.Core()).Tokens()
	got := detail(toks[0])
	for _, want := range []string{"plain(a)", "double(b c)", "plain(d)"} {
		if !strings.Contains(got, want) {
			t.Errorf("detail = %q, missing %q", got, want)
		}
	}
}

func TestDumpTokensReportsUnfinishedInputDistinctly(t *testing.T) {
	err := dumpTokens(`"abc`, syntax.Core())
	if err == nil {
		t.Fatal("want an error for an unterminated quote")
	}
	if !strings.Contains(err.Error(), "unfinished") {
		t.Errorf("error should say the input is unfinished, got %q", err)
	}
}

func TestDetailDistinguishesSubstitutionsFromText(t *testing.T) {
	// A substitution shown as "plain" reads as literal text, which is the
	// opposite of what this tool is for.
	toks := syntax.NewLexer(`"[$(echo hi)]"`, syntax.Core()).Tokens()
	got := detail(toks[0])
	if !strings.Contains(got, "quoted-cmd-subst(echo hi)") {
		t.Errorf("detail = %q, want a quoted-cmd-subst span", got)
	}
	if strings.Contains(got, "plain(echo hi)") {
		t.Errorf("detail = %q: a substitution must not render as literal text", got)
	}
}
