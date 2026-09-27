// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `private` over one of the shell's *own* parameters, which this shell
// refuses and which was taken here until #4802.
//
// The rule is the substrate's and names a word rather than a shell — see
// interp/parameterscopefixed.go and its test. What is this dialect's is the
// **set**, and it is a list because nothing a script can see derives it: see
// scopefixedparameters.go for the 177-name sweep that produced it.
//
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh` — `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, `go version -m` says *not a Go executable*
// for it — run `-f` under `env -i PATH=/usr/bin:/bin` with a scratch `HOME`.

// TestPrivateRefusesTheShellsOwnParameters is the refusal itself, in the
// sentence and at the status the reference gives it, with the script carrying
// on and the parameter untouched.
func TestPrivateRefusesTheShellsOwnParameters(t *testing.T) {
	out, st := answersRun(t, `zmodload zsh/param/private
f() { private path; print "st=$?"; print "p=$path" }
f
print "after"`)
	want := "f:private: can't change scope of existing param: path\nst=1\np=/usr/bin /bin\nafter\n"
	if out != want || st != 0 {
		t.Errorf("private over the shell's own = %q (status %d), want %q", out, st, want)
	}
}

// And a name-at-a-time row for each shape the sweep found, because a set this
// large is where a rule gets guessed from a handful of examples. Each of the
// refused names is one the sweep measured at 1 and each of the taken ones is
// one it measured at 0 — a produced parameter beside a stored one, a built-in
// tie beside a script's own, a name with no value at all beside one with a
// value, and a module's parameter beside the shell's.
func TestWhichOfTheShellsOwnParametersPrivateRefuses(t *testing.T) {
	refused := []string{
		"path", "PATH", "fpath", "psvar", "argv", "status", "pipestatus",
		"RANDOM", "SECONDS", "LINENO", "UID", "IFS", "HISTSIZE", "COLUMNS",
		"PROMPT", "HOME", "SHLVL", "OPTIND", "_", "TERM", "LANG", "LC_ALL",
		"RPROMPT", "TTYIDLE", "ERRNO", "ZSH_SUBSHELL", "zsh_eval_context",
	}
	for _, name := range refused {
		out, st := answersRun(t, `zmodload zsh/param/private
f() { private `+name+`; print "st=$?" }
f`)
		want := "f:private: can't change scope of existing param: " + name + "\nst=1\n"
		if out != want || st != 0 {
			t.Errorf("private %s = %q (status %d), want %q", name, out, st, want)
		}
	}
	// The other column, which is what makes the first one evidence rather
	// than a word that refuses everything. `PWD` and `OLDPWD` are the shell's
	// and are taken; `ZSH_VERSION`, `HOST` and `TMPPREFIX` likewise; `WATCH`
	// and `watch` are the two the reference calls `special` and takes anyway;
	// `options` and `functions` are a module's; and `nosuchname` is a name no
	// shell owns.
	taken := []string{
		"PWD", "OLDPWD", "ZSH_VERSION", "HOST", "TMPPREFIX", "MAILCHECK",
		"WATCH", "watch", "options", "functions", "dirstack", "HISTFILE",
		"nosuchname",
	}
	for _, name := range taken {
		out, st := answersRun(t, `zmodload zsh/param/private
f() { private `+name+`; print "st=$?" }
f`)
		if out != "st=0\n" || st != 0 {
			t.Errorf("private %s = %q (status %d), want st=0", name, out, st)
		}
	}
	// And a tie a **script** makes, which is the row that says the refusal is
	// about which names the shell hardwired rather than about the mechanism.
	out, st := answersRun(t, `zmodload zsh/param/private
typeset -T TT tt
f() { private tt; print "st=$?"; private TT; print "st=$?" }
f`)
	if out != "st=0\nst=0\n" || st != 0 {
		t.Errorf("private over a script's own tie = %q (status %d), want both 0", out, st)
	}
}

// The top level takes every one of them, which is the boundary the refusal
// stands on: a declaration that takes no scope is not moving a binding
// anywhere. `private path` there is the listing `typeset path` is, and
// `private HOME=/x` assigns.
func TestTheShellsOwnParametersAreTakenOutsideAFunction(t *testing.T) {
	out, st := answersRun(t, `zmodload zsh/param/private
private path; print "st=$?"
private HOME=/x; print "st=$? home=$HOME"`)
	want := "path=( /usr/bin /bin )\nst=0\nst=0 home=/x\n"
	if out != want || st != 0 {
		t.Errorf("private at the top level = %q (status %d), want %q", out, st, want)
	}
	// An anonymous function is a function, and its location says so.
	out, st = answersRun(t, `zmodload zsh/param/private
() { private path; print "st=$?" }`)
	want = "(anon):private: can't change scope of existing param: path\nst=1\n"
	if out != want || st != 0 {
		t.Errorf("private in an anonymous function = %q (status %d), want %q", out, st, want)
	}
}

// The two ways past the refusal, and the letters that are not.
//
// `-h` asks for an ordinary local that hides the shell's parameter for the
// call, which is the one way to have a binding of one's own over such a name;
// a bare `+` is an option word rather than a declaration, so there is nothing
// to refuse — until an operand carries a value, when both reasons come back.
func TestTheTwoWaysPastTheRefusalOverTheShellsOwn(t *testing.T) {
	out, st := answersRun(t, `zmodload zsh/param/private
f() { private -h path; print "st=$? p=[$path]" }
f
print "after=[$path]"`)
	want := "st=0 p=[]\nafter=[/usr/bin /bin]\n"
	if out != want || st != 0 {
		t.Errorf("private -h over the shell's own = %q (status %d), want %q", out, st, want)
	}
	// The status, the empty binding and the restore, and deliberately not the
	// word a type query writes for it: the reference says
	// `scalar-local-hide-special` where this shell says
	// `scalar-local-tied-hide-special`, a tie attribute the hidden local
	// should not have inherited. That row stands on `main` as it does here —
	// it was reachable there because the declaration was taken outright — and
	// it belongs to the letter rather than to this name set.
	out, st = answersRun(t, `zmodload zsh/param/private
f() { private + path; print "st=$?"; private -a + HOME; print "st=$?" }
f`)
	if out != "st=0\nst=0\n" || st != 0 {
		t.Errorf("private + over the shell's own = %q (status %d), want both 0", out, st)
	}
	// With a value the sign is a declaration again, and the parameter is left
	// exactly as it was.
	out, st = answersRun(t, `zmodload zsh/param/private
f() { private + path=(/x); print "st=$?" }
f
print "after=[$path]"`)
	want = "f:private: can't change scope of existing param: path\nst=1\nafter=[/usr/bin /bin]\n"
	if out != want || st != 0 {
		t.Errorf("private + with a value = %q (status %d), want %q", out, st, want)
	}
	// And the letters that are not a way past, `+h` being the one that looks
	// like `-h` and is the opposite request.
	for _, letter := range []string{"+h", "-a", "-x", "-r", "-U", "-P"} {
		out, st := answersRun(t, `zmodload zsh/param/private
f() { private `+letter+` path; print "st=$?" }
f`)
		if !strings.Contains(out, "can't change scope of existing param: path") || st != 0 {
			t.Errorf("private %s path = %q (status %d), want the refusal", letter, out, st)
		}
	}
}

// A bare `+` is the *listing* `typeset` writes, and it reaches the second
// word too: `private + v` over a name this call has already declared writes
// the value back at 0 where `private v` there is the redeclaration refusal.
//
// The `typeset` row beside it is the control that says this is the sign and
// not the word — `-` reached this already because it leaves every flag at its
// zero, and `+` did not because it sets one.
func TestABareSignListsRatherThanDeclares(t *testing.T) {
	out, st := answersRun(t, `v=hi
typeset + v; typeset - v; typeset v`)
	want := "v=hi\nv=hi\nv=hi\n"
	if out != want || st != 0 {
		t.Errorf("a bare sign over a standing name = %q (status %d), want %q", out, st, want)
	}
	out, st = answersRun(t, `zmodload zsh/param/private
f() { private v=1; private + v; print "st=$? v=[$v]" }
f`)
	want = "v=1\nst=0 v=[1]\n"
	if out != want || st != 0 {
		t.Errorf("private + over its own declaration = %q (status %d), want %q", out, st, want)
	}
	// And with a value it is the refusal again, which is the pair that says
	// the sign is not a free pass.
	out, st = answersRun(t, `zmodload zsh/param/private
f() { private v=1; private + v=2; print "st=$? v=[$v]" }
f`)
	want = "f:private: can't change scope of existing param: v\nst=1 v=[1]\n"
	if out != want || st != 0 {
		t.Errorf("private + with a value over its own = %q (status %d), want %q", out, st, want)
	}
}
