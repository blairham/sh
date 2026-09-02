// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

func TestPickDialect(t *testing.T) {
	// A name resolves to a grammar, a semantics *and* a diagnostics,
	// because they answer different questions about the same shell.
	for _, name := range []string{"core", "posix", "bash", "zsh", "ksh", "dash"} {
		if _, err := pickDialect(name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := pickDialect("nosuchshell"); err == nil {
		t.Error("an unknown dialect must be refused rather than defaulted")
	}

	// The three vectors differ independently, which is the point of having
	// three rather than one name. bash and zsh disagree on a semantics axis
	// and on a diagnostic; bash and dash agree on the diagnostic and
	// disagree on semantics.
	bashSh, _ := pickDialect("bash")
	zshSh, _ := pickDialect("zsh")
	dashSh, _ := pickDialect("dash")
	kshSh, _ := pickDialect("ksh")
	if bashSh.Semantics.ArithLeadingZeroIsOctal == zshSh.Semantics.ArithLeadingZeroIsOctal {
		t.Error("bash and zsh should disagree about whether a leading zero is octal")
	}
	if bashSh.Semantics.EchoInterpretsEscapes == dashSh.Semantics.EchoInterpretsEscapes {
		t.Error("bash and dash should disagree about echo and backslashes")
	}
	for _, tc := range []struct {
		name string
		got  int
		want int
	}{
		{"dash", dashSh.Diagnostics.SyntaxStatus(), 2},
		{"bash", bashSh.Diagnostics.SyntaxStatus(), 2},
		{"ksh93", kshSh.Diagnostics.SyntaxStatus(), 3},
		{"zsh", zshSh.Diagnostics.SyntaxStatus(), 1},
	} {
		if tc.got != tc.want {
			t.Errorf("%s syntax-error status = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

// TestEveryDialectResolvesToARegisteredShell pins the thing the extraction was
// for: this driver and each cmd/<shell> binary are built from the same value,
// so a dialect cannot behave one way here and another way there. The named
// shells all carry an Apply; core and posix are the substrate's own and carry
// none, which is what makes them a portability check rather than a shell.
func TestEveryDialectResolvesToARegisteredShell(t *testing.T) {
	for _, name := range []string{"bash", "zsh", "ksh", "dash"} {
		sh, err := pickDialect(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if sh.Register == nil {
			t.Errorf("%s: a named shell should carry its dialect's Apply", name)
		}
	}
	for _, name := range []string{"core", "posix"} {
		sh, err := pickDialect(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if sh.Register != nil {
			t.Errorf("%s: the substrate's own answers need no dialect Apply", name)
		}
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

// The wiring is what went wrong, so the wiring is what this tests: `-c`
// reached past driver's own and lost the operands with it, and nothing in
// this package could say so.
//
// `dispatch` returns a status rather than exiting for exactly that reason.
func TestDispatchGivesTheCommandStringItsOperands(t *testing.T) {
	sh, _ := pickDialect("bash")
	var out bytes.Buffer
	sh.Stdout, sh.Stderr = &out, &out
	sh.Name = "testsh"

	code := dispatch(sh, options{
		command: `echo "0=$0 1=$1 n=$#"`, commandGiven: true,
	}, "", []string{"zero", "one", "two"})
	if code != 0 {
		t.Fatalf("status %d: %s", code, out.String())
	}
	if got, want := strings.TrimSpace(out.String()), "0=zero 1=one n=2"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
}

// An empty `-c` is a command string and an empty one: running nothing is not
// the same as having named nothing to run, which is why the option carries
// whether it was given rather than whether it is empty.
func TestDispatchTellsAnEmptyCommandFromNone(t *testing.T) {
	sh, _ := pickDialect("bash")
	var out bytes.Buffer
	sh.Stdout, sh.Stderr = &out, &out
	sh.Name = "testsh"

	if code := dispatch(sh, options{command: "", commandGiven: true}, "", nil); code != 0 {
		t.Errorf("status %d for an empty command string, want it to run and do nothing", code)
	}
	if out.Len() != 0 {
		t.Errorf("out = %q, want nothing", out.String())
	}
}
