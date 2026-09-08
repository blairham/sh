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
		{`noglob command echo a[b]c`, "a[b]c"},
		{`builtin noglob echo a[b]c`, "a[b]c"},
		// The scratch PATH holds no `echo` program, so this one is graded
		// on *which* diagnostic: `command not found: echo` says the scan
		// read past `exec` and took the modifier, where a match that had
		// fired would have said `no matches found: a[b]c` instead.
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
		{`noglob eval 'echo a[b]c'`, "zsh:1: no matches found: a[b]c"},
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
