// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The precommand modifiers: words that stand in front of a command, are taken
// away before it runs, and change what happens to the words behind them.
// Measured on zsh 5.9.2, 2026-09-08, under `zsh -f` in a UTF-8 locale — see
// interp/precommand.go for the whole family and docs/spec/grammar/commands.md.
//
// `noglob` was read as an ordinary command name before this, so every line
// using it reported `command not found` *after* the pattern behind it had
// already been matched and failed — and an unmatched pattern is fatal in this
// shell, so the enclosing function ended. The rc file this shell has to run
// reaches `.zi-set-m-func`, whose whole body is `noglob unset functions[m]`
// (#1526).
//
// Every row asserts the *value* on the output rather than a status: a status
// of 1 is what a refusal and a failed match both produce, so a status is the
// one thing that cannot tell the fix from the bug.

func TestNoglobSwitchesTheMatchOffForTheWordsBehindIt(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The pair that is the whole of it: without the modifier the
		// unmatched pattern is fatal here, with it the word stands.
		{`echo a[b]c`, "zsh:1: no matches found: a[b]c"},
		{`noglob echo a[b]c`, "a[b]c"},
		{`noglob print -r -- *.nosuchthing`, "*.nosuchthing"},
		// It reaches every word, not only the first.
		{`noglob echo a[b]c d[e]f`, "a[b]c d[e]f"},
		// And the word that carries it is itself never matched, which is
		// what makes the command word behind it the command word.
		{`noglob a[b]c`, "zsh:1: command not found: a[b]c"},
		// Repeats and combinations. `command` stops the scan and the other
		// two do not — measured, and the reason the three are not one rule.
		{`noglob noglob echo a[b]c`, "a[b]c"},
		{`builtin noglob echo a[b]c`, "a[b]c"},
		// The scratch PATH holds no `echo` program, so these two are graded
		// on *which* diagnostic: `command not found: echo` says the scan
		// read past the word ahead and took the modifier, where a match that
		// had fired would have said `no matches found: a[b]c` instead.
		//
		// `command` reaches only an external here — see
		// Semantics.CommandReachesABuiltin — which is why it is `echo` that
		// is missing rather than `echo` that ran.
		{`noglob command echo a[b]c`, "zsh:1: command not found: echo"},
		{`exec noglob echo a[b]c`, "zsh:1: command not found: echo"},
		{`command noglob echo a[b]c`, "zsh:1: no matches found: a[b]c"},
		// A builtin rather than grammar: quoting does not take it away and
		// an expansion can produce it. Both are the opposite of `nocorrect`
		// below, and the pair is what says the two are different mechanisms.
		{`\noglob echo a[b]c`, "a[b]c"},
		{`"noglob" echo a[b]c`, "a[b]c"},
		{`c=noglob; $c echo a[b]c`, "a[b]c"},
		// And the scan is over fields rather than over words: a list can
		// carry it, an unsplit scalar holding both words cannot, and the
		// pair is what says which of the two the rule is about.
		{`c=(noglob echo); $c a[b]c`, "a[b]c"},
		{`c="noglob echo"; ${=c} a[b]c`, "a[b]c"},
		{`c="noglob echo"; $c a[b]c`, "zsh:1: no matches found: a[b]c"},
		// What follows it is a command word and no longer an assignment,
		// which is the same fact read from the other side.
		{`noglob x=1 echo a[b]c`, "zsh:1: command not found: x=1"},
		// It ends with the command. A function called through it globs in
		// its own body, and an `eval` globs in what it reads.
		{`f() { echo a[b]c; }; noglob f`, "f: no matches found: a[b]c"},
		// Located at the evaluated text and not at the shell, which is this
		// shell's answer for anything `eval` reads (#2133).
		{`noglob eval 'echo a[b]c'`, "(eval):1: no matches found: a[b]c"},
		// A substitution inside the word is its own command and globs.
		{`noglob echo "$(echo a[b]c)"`, "zsh:1: no matches found: a[b]c"},
		// The words alone: a redirection target is expanded by another
		// route and the modifier does not reach it.
		{`noglob echo x >out[1].txt`, "zsh:1: no matches found: out[1].txt"},
		// Nothing behind it is nothing to do, and it succeeds.
		{`noglob; echo done`, "done"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// TestNoglobLeavesTheOtherHalvesOfMatchingAlone: only the filesystem pass is
// switched off, which is the same boundary `set -o noglob` keeps.
func TestNoglobLeavesTheOtherHalvesOfMatchingAlone(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ src, want string }{
		{`noglob echo *.txt`, "*.txt"},
		{`echo *.txt`, "a.txt"},
		// A tilde is not a pattern, so it still expands.
		{`HOME=/tmp; noglob echo ~`, "/tmp"},
	} {
		out, _ := runZsh(t, dir, tc.src)
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// TestNoglobDoesNotMoveTheCommandOutOfThisShell: the modifier is taken away
// before anything asks what the command is, so a builtin behind it is still
// the builtin the last element of a pipeline may run in this shell.
//
// The sibling caller this exists for: the question is asked a second time
// from the written words rather than the expanded ones, and the copy that did
// not know about the modifier read into a subshell and left the name empty.
func TestNoglobDoesNotMoveTheCommandOutOfThisShell(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo a | read x; echo "x=[$x]"`, "x=[a]"},
		{`echo a | noglob read x; echo "x=[$x]"`, "x=[a]"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// TestNocorrectIsGrammarAndNoglobIsNot. The two words look alike and are not:
// `whence -w` calls one reserved and the other a builtin, and every row here
// is a shape that tells them apart. See syntax.Dialect.ReservedPrecommands.
func TestNocorrectIsGrammarAndNoglobIsNot(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`nocorrect echo hi`, "hi"},
		{`nocorrect nocorrect echo hi`, "hi"},
		// Taken away before the command is read, so what follows is still
		// an assignment prefix — where `noglob` leaves a command word.
		{`nocorrect x=1 echo hi; echo "x=[$x]"`, "hi\nx=[]"},
		// Recognized after a redirection or an assignment prefix too.
		{`x=1 nocorrect echo hi`, "hi"},
		{`>/dev/null nocorrect echo hi; echo done`, "done"},
		// Quoting removes the reservation, as it does for `if`.
		{`\nocorrect echo hi`, "zsh:1: command not found: nocorrect"},
		// And an expansion cannot produce it.
		{`x=nocorrect; $x echo hi`, "zsh:1: command not found: nocorrect"},
		// It is not the correction it names: the words behind it still glob.
		{`nocorrect echo a[b]c`, "zsh:1: no matches found: a[b]c"},
		{`nocorrect noglob echo a[b]c`, "a[b]c"},
		// The other way round it is an ordinary word again, because the
		// grammar has finished by the time the builtin is read.
		{`noglob nocorrect echo a[b]c`, "zsh:1: command not found: nocorrect"},
		// An ordinary word everywhere else.
		{`echo nocorrect`, "nocorrect"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// TestNocorrectRefusesWhatMayNotFollowIt: a reserved word has nowhere to go
// behind it, which is what says the word was consumed by the grammar rather
// than passed on as an argument.
func TestNocorrectRefusesWhatMayNotFollowIt(t *testing.T) {
	if _, err := parseZsh("nocorrect if true; then echo hi; fi"); err == nil {
		t.Error("nocorrect if … parsed, want a refusal")
	}
	if _, err := parseZsh("nocorrect echo hi"); err != nil {
		t.Errorf("nocorrect echo hi: %v", err)
	}
}

// The `-` modifier: the command runs with a dash on the front of its argv[0],
// and every modifier behind it stops reading its own options.
//
// Measured 2026-09-28 against /opt/homebrew/bin/zsh — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m`: *not a Go executable* — from
// script files under `env -i PATH=/usr/bin:/bin` with a scratch HOME. It was
// read as a word to **throw away** until then, and every row that reached the
// command through `echo` agreed with that reading, because `echo hi` prints
// `hi` under either one (#5018, #5028).
//
// The rows here run `/bin/sh`, which the scratch PATH does not hold and does
// not need to: the name is written with a slash in it.
func TestTheDashModifierNamesTheCommandWithADash(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The control, and then the one line that separates a modifier from
		// a discard.
		{`/bin/sh -c 'echo "[$0]"'`, "[/bin/sh]"},
		{`- /bin/sh -c 'echo "[$0]"'`, "[-/bin/sh]"},
		// One dash however many words were written, which says it is a
		// property of the invocation rather than a character per modifier.
		{`- - /bin/sh -c 'echo "[$0]"'`, "[-/bin/sh]"},
		{`- - - /bin/sh -c 'echo "[$0]"'`, "[-/bin/sh]"},
		// A modifier may stand on either side of it, and `exec` is the
		// builtin that stands in front of a command.
		{`noglob - /bin/sh -c 'echo "[$0]"'`, "[-/bin/sh]"},
		{`- noglob /bin/sh -c 'echo "[$0]"'`, "[-/bin/sh]"},
		{`- exec /bin/sh -c 'echo "[$0]"'`, "[-/bin/sh]"},
		{`exec - /bin/sh -c 'echo "[$0]"'`, "[-/bin/sh]"},
		{`noglob exec - /bin/sh -c 'echo "[$0]"'`, "[-/bin/sh]"},
		// `command` stops the scan and the dash still reaches the command
		// behind it.
		{`- command /bin/sh -c 'echo "[$0]"'`, "[-/bin/sh]"},
		// It reaches an external command alone: a function is named by
		// itself, and a builtin has nothing to dash.
		{`f() { echo "[$0]"; }; - f`, "[f]"},
		{`- :; echo "st=$?"`, "st=0"},
		// And an ordinary builtin behind it reads its own options.
		{`- echo -n hi; echo "|"`, "hi|"},
		{`- print -n hi; echo "|"`, "hi|"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// TestTheDashModifierStopsTheOptionScanBehindIt: a dash-word behind it becomes
// the **command name**, which `command not found: -l` says and a usage error
// would not.
//
// The controls are what make this one rule and not four: each letter reads
// perfectly well on its own, and the same `exec -l` behind a *different*
// modifier still reads it — so it is this word and not "a modifier in front of
// a modifier".
func TestTheDashModifierStopsTheOptionScanBehindIt(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`exec -l /bin/sh -c 'echo "[$0]"'`, "[-/bin/sh]"},
		{`exec -a zz /bin/sh -c 'echo "[$0]"'`, "[zz]"},
		{`command -p /bin/sh -c 'echo "[$0]"'`, "[/bin/sh]"},
		{`noglob exec -l /bin/sh -c 'echo "[$0]"'`, "[-/bin/sh]"},
		{`- exec -l /bin/sh -c 'echo "[$0]"'`, "zsh:1: command not found: -l"},
		{`- exec -a zz /bin/sh -c 'echo "[$0]"'`, "zsh:1: command not found: -a"},
		{`- exec -l -a zz /bin/sh -c 'echo "[$0]"'`, "zsh:1: command not found: -l"},
		{`- exec -c /bin/sh -c 'echo "[$0]"'`, "zsh:1: command not found: -c"},
		{`- exec -- /bin/sh -c 'echo "[$0]"'`, "zsh:1: command not found: --"},
		{`- command -p /bin/sh -c 'echo "[$0]"'`, "zsh:1: command not found: -p"},
		{`- command -v ls`, "zsh:1: command not found: -v"},
		// Wherever in the scan the word stood, and however far behind it the
		// modifier is.
		{`- - exec -l /bin/sh -c 'echo "[$0]"'`, "zsh:1: command not found: -l"},
		{`noglob - exec -l /bin/sh -c 'echo "[$0]"'`, "zsh:1: command not found: -l"},
		{`- noglob exec -l /bin/sh -c 'echo "[$0]"'`, "zsh:1: command not found: -l"},
		{`- builtin exec -l /bin/sh -c 'echo "[$0]"'`, "zsh:1: command not found: -l"},
		{`builtin - exec -l /bin/sh -c 'echo "[$0]"'`, "zsh:1: command not found: -l"},
		// The discriminating pair for the noun: `exec -l` asks for exactly
		// the dash the modifier asks for and the letters behind it are still
		// read, and the identical letters behind a `-` are not read at all.
		{`exec -l -a zz /bin/sh -c 'echo "[$0]"'`, "[zz]"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// TestAModifierWithNothingToRunLeavesARedirectionWithNoCommand: a word that
// was written is not the same state as no word at all, and the redirection is
// what tells them apart.
//
// Measured 2026-09-28 against /opt/homebrew/bin/zsh — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m`: *not a Go executable*.
// Whether the modifier was **taken away or kept** is not the question, which
// is where this was first written too narrowly: `noglob` is taken and
// `builtin` stays, and both leave nothing to run.
//
// Two rows are not this rule and are the discriminating ones. `nocorrect` is
// grammar rather than a builtin, so the parser takes it and a bare
// redirection is what reaches the command — the control's own case. And an
// assignment prefix takes a command off the route in every dialect, which is
// the same clause the null command already carries.
func TestAModifierWithNothingToRunLeavesARedirectionWithNoCommand(t *testing.T) {
	const refused = "zsh:1: redirection with no command"
	for _, tc := range []struct{ src, want string }{
		{`NULLCMD=:; noglob >f; print after`, refused},
		{`NULLCMD=:; - >f; print after`, refused},
		{`NULLCMD=:; builtin >f; print after`, refused},
		{`NULLCMD=:; command >f; print after`, refused},
		{`NULLCMD=:; builtin command >f; print after`, refused},
		{`NULLCMD=:; noglob builtin >f; print after`, refused},
		{`NULLCMD=:; builtin - >f; print after`, refused},
		// Any redirection, not only an output one.
		{`NULLCMD=:; builtin <f; print after`, refused},
		// The control: with no word at all the null command runs and the
		// script carries on. The hook is named because the harness starts
		// with none, and with none the two sides are the same refusal —
		// which is a control that cannot fail.
		{`NULLCMD=:; >f; print after`, "after"},
		// `nocorrect` is the grammar's, so it never reaches the command.
		{`NULLCMD=:; nocorrect >f; print after`, "after"},
		// An assignment prefix takes it off the route, and persists.
		{`NULLCMD=:; v=1 noglob >f; print "after v=$v"`, "after v=1"},
		{`NULLCMD=:; v=1 - >f; print "after v=$v"`, "after v=1"},
		// And with nothing to redirect there is nothing to refuse — the
		// assignments in front of it persist, as they do for `v=1` alone.
		{`v=1 -; print "st=$? v=[$v]"`, "st=0 v=[1]"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}

// TestTheRedirectionFormIsNotCommandless: `exec` is the one word in the family
// whose own no-command form is defined, so a chain holding it leaves the
// redirection something to belong to — wherever in the chain it stands.
func TestTheRedirectionFormIsNotCommandless(t *testing.T) {
	for _, src := range []string{
		`NULLCMD=:; exec >f; print after; exec >&2; read -r x <f; print -r -- $x`,
		`NULLCMD=:; builtin exec >f; print after; exec >&2; read -r x <f; print -r -- $x`,
	} {
		out, _ := runZsh(t, t.TempDir(), src)
		if strings.TrimSpace(out) != "after" {
			t.Errorf("%s = %q, want the redirection kept and %q read back", src, out, "after")
		}
	}
}

// TestCommandStopsTheScan: the word behind `command` is a command name and not
// a modifier, which naming it in the table must not change.
func TestCommandStopsTheScan(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// Looked up on the filesystem rather than read as the modifier it
		// spells — the scratch PATH holds neither.
		{`NULLCMD=:; command builtin >f`, "zsh:1: command not found: builtin"},
		{`command noglob echo a[b]c`, "zsh:1: no matches found: a[b]c"},
		// And the ordinary uses are untouched. `command` reaches only an
		// external here — see Semantics.CommandReachesABuiltin — so the
		// scratch PATH not holding `echo` is what this row reads.
		{`command echo hi`, "zsh:1: command not found: echo"},
		{`command -v echo`, "echo"},
		{`command; echo st=$?`, "st=0"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}
