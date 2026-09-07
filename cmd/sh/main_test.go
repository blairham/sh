// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
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
	err := dumpTokens(io.Discard, `"abc`, syntax.Core())
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

// The boundary is what this front end still owns: its flags are read from
// the front of the line and stop at the first word that is not one of them,
// so nothing a shell reads — options, operands, a script's parameters — can
// be eaten as a -dialect or a -tokens.
func TestReadOwnFlagsStopsAtTheShellsFirstWord(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		dialect string
		rest    []string
	}{
		{"dialect then a command", []string{"-dialect", "bash", "-c", "echo hi"}, "bash", []string{"-c", "echo hi"}},
		{"dialect attached with =", []string{"-dialect=zsh", "x.sh", "a"}, "zsh", []string{"x.sh", "a"}},
		{"two dashes, as the flag package read it", []string{"--dialect", "ksh", "-e", "x.sh"}, "ksh", []string{"-e", "x.sh"}},
		{"nothing of ours", []string{"-e", "-c", "echo hi"}, "core", []string{"-e", "-c", "echo hi"}},
		{
			// A script's own parameter spelled like our flag is the
			// script's: the scan ended at the path.
			"a parameter that looks like a flag",
			[]string{"x.sh", "-dialect"},
			"core",
			[]string{"x.sh", "-dialect"},
		},
		{"a lone dash is the shell's", []string{"-", "x.sh"}, "core", []string{"-", "x.sh"}},
		{"-- is the shell's", []string{"--", "-c"}, "core", []string{"--", "-c"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			own, rest, err := readOwnFlags(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			if own.dialect != tc.dialect {
				t.Errorf("dialect = %q, want %q", own.dialect, tc.dialect)
			}
			if !slices.Equal(rest, tc.rest) {
				t.Errorf("rest = %q, want %q", rest, tc.rest)
			}
		})
	}
}

func TestReadOwnFlagsWantsADialectName(t *testing.T) {
	if _, _, err := readOwnFlags([]string{"-dialect"}); err == nil {
		t.Error("-dialect with nothing after it must be refused, not defaulted")
	}
}

// The shell this binary hands the shared front end runs a script the way the
// dialect binaries do — parameters included, which is the half the fifth
// copy of the invocation logic dropped.
func TestTheSharedFrontEndCarriesTheScriptsParameters(t *testing.T) {
	sh, _ := pickDialect("core")
	var out bytes.Buffer
	sh.Stdout, sh.Stderr = &out, &out
	sh.Name = "testsh"

	path := filepath.Join(t.TempDir(), "args.sh")
	if err := os.WriteFile(path, []byte("echo \"n=$# 1=[$1] 2=[$2]\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := driver.MainArgs(sh, []string{"testsh", path, "a", "b"}); code != 0 {
		t.Fatalf("status %d: %s", code, out.String())
	}
	if got, want := strings.TrimSpace(out.String()), "n=2 1=[a] 2=[b]"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
}

// -acp is this binary's flag rather than a shell's, so the shared front end
// must never see it — and, like the others, the scan stops at the first word
// that is not ours, so a script's own `-acp` argument is the script's.
func TestACPFlag(t *testing.T) {
	own, rest, err := readOwnFlags([]string{"-acp"})
	if err != nil {
		t.Fatalf("readOwnFlags: %v", err)
	}
	if !own.acp {
		t.Error("-acp was not read")
	}
	if len(rest) != 0 {
		t.Errorf("rest = %v, want nothing left for the shell", rest)
	}

	own, rest, err = readOwnFlags([]string{"script.sh", "-acp"})
	if err != nil {
		t.Fatalf("readOwnFlags: %v", err)
	}
	if own.acp {
		t.Error("a script's own -acp argument was eaten")
	}
	if !slices.Equal(rest, []string{"script.sh", "-acp"}) {
		t.Errorf("rest = %v, want both words left for the shell", rest)
	}
}

// A session's directory and its program both arrive as messages, so there is
// no invocation to read. An operand here would be silently ignored, which is
// the failure where somebody's `-c` never runs and nothing says why.
func TestACPTakesNoOperands(t *testing.T) {
	sh, err := pickDialect("core")
	if err != nil {
		t.Fatal(err)
	}
	if code := serveACP(sh, []string{"-c", "echo hi"}, strings.NewReader(""), io.Discard); code == 0 {
		t.Error("operands were accepted; they would have been ignored")
	}
}

// -acp-connect takes the command that starts an agent, and the scan stops at
// it: the agent's own flags are the agent's, not ours. `npx pkg --acp` must
// reach the agent with its `--acp` intact.
func TestACPConnectFlag(t *testing.T) {
	own, rest, err := readOwnFlags([]string{"-acp-allow", "-acp-connect", "npx", "pkg", "--acp"})
	if err != nil {
		t.Fatalf("readOwnFlags: %v", err)
	}
	if !own.acpConnect || !own.acpAllow {
		t.Fatalf("flags = %+v, want both read", own)
	}
	if !slices.Equal(rest, []string{"npx", "pkg", "--acp"}) {
		t.Errorf("rest = %v, want the agent's whole command line", rest)
	}
}

// -acp-auth names one of the methods the agent advertised, and it is a value
// flag in both spellings the rest of them accept. It must not eat the agent's
// command line either: `-acp-auth id -acp-connect npx …` leaves `npx` alone.
func TestACPAuthFlag(t *testing.T) {
	for _, args := range [][]string{
		{"-acp-auth", "gemini-api-key", "-acp-connect", "npx", "pkg"},
		{"-acp-auth=gemini-api-key", "-acp-connect", "npx", "pkg"},
	} {
		own, rest, err := readOwnFlags(args)
		if err != nil {
			t.Fatalf("readOwnFlags(%v): %v", args, err)
		}
		if own.acpAuth != "gemini-api-key" {
			t.Errorf("acpAuth = %q, want the method id", own.acpAuth)
		}
		if !slices.Equal(rest, []string{"npx", "pkg"}) {
			t.Errorf("rest = %v, want the agent's command line", rest)
		}
	}
	if _, _, err := readOwnFlags([]string{"-acp-auth"}); err == nil {
		t.Error("-acp-auth with no method id was accepted")
	}
}

// With no command there is nothing to connect to, and saying so beats hanging
// on a pipe nobody is on the other end of.
func TestACPConnectNeedsACommand(t *testing.T) {
	sh, err := pickDialect("core")
	if err != nil {
		t.Fatal(err)
	}
	if code := connectACP(sh, false, "", nil); code == 0 {
		t.Error("connecting to nothing reported success")
	}
}
