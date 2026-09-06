// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// toolOnPath drops an executable named `tool` into dir, which runKsh already
// has as the whole of PATH.
func toolOnPath(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "tool")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// Bare `whence` answers with the resolution and nothing more — a builtin,
// keyword or function as its own name, an external as its path, and a missing
// name as silence at 1. Measured 2026-09-04, ksh93u+.
func TestBareWhenceIsTheResolution(t *testing.T) {
	dir := t.TempDir()
	path := toolOnPath(t, dir)
	out, st := runKsh(t, dir, `whence echo; whence if; f(){ :; }; whence f; whence tool`)
	if st != 0 || out != "echo\nif\nf\n"+path+"\n" {
		t.Errorf("out %q status %d, want the four bare resolutions", out, st)
	}
	out, st = runKsh(t, dir, `whence nosuchzz`)
	if st != 1 || out != "" {
		t.Errorf("out %q status %d, want silence at 1", out, st)
	}
}

// `whence -v` is the sentence — ksh93's `type` is spelled `whence -v`, and
// the not-found complaint names whence.
func TestWhenceVIsTheSentence(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `whence -v echo; f(){ :; }; whence -v f; whence -v if`)
	if st != 0 || out != "echo is a shell builtin\nf is a function\nif is a keyword\n" {
		t.Errorf("out %q status %d, want type's sentences", out, st)
	}
	out, st = runKsh(t, t.TempDir(), `whence -v nosuchzz`)
	if st != 1 || !strings.Contains(out, "whence: nosuchzz: not found") {
		t.Errorf("out %q status %d, want the whence-prefixed not-found at 1", out, st)
	}
}

// An alias answers as its value, quoted when it needs it — the one resolution
// the core's lookup cannot see, measured as `'ls -l'` bare and `ll is an
// alias for 'ls -l'` under -v.
func TestWhenceSpeaksForAliases(t *testing.T) {
	out, st := runKsh(t, t.TempDir(),
		`alias ll="ls -l"; whence ll; whence -v ll; alias g=grep; whence g`)
	if st != 0 || out != "'ls -l'\nll is an alias for 'ls -l'\ngrep\n" {
		t.Errorf("out %q status %d, want the quoted value, the sentence, and the bare word", out, st)
	}
}

// `-p` is the PATH search alone: a function is invisible to it, a found path
// prints bare, and with -v the path gets type's tracked-alias sentence.
func TestWhencePSearchesPathAlone(t *testing.T) {
	dir := t.TempDir()
	path := toolOnPath(t, dir)
	out, st := runKsh(t, dir, `whence -p tool; f(){ :; }; whence -p f; echo st=$?; whence -pv tool`)
	want := path + "\nst=1\ntool is a tracked alias for " + path + "\n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// `-q` is the status with the words withheld.
func TestWhenceQIsQuiet(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `whence -q echo nosuchzz; echo st=$?; whence -q echo; echo st=$?`)
	if out != "st=1\nst=0\n" {
		t.Errorf("out %q status %d, want the statuses alone", out, st)
	}
}

// An unknown letter is `unknown option` with the usage line after it at 2; so
// is a whence with nothing to ask about, the bare line alone. The letters
// ksh93 has and this shell does not — -a, -f — are refused as not
// implemented rather than unknown, which would be a worse answer.
func TestWhenceRefusals(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `whence -z ls`)
	if st != 2 || !strings.Contains(out, "whence: -z: unknown option") ||
		!strings.Contains(out, "Usage: whence [-afpqv] name  ...") {
		t.Errorf("out %q status %d, want the complaint and the usage at 2", out, st)
	}
	out, st = runKsh(t, t.TempDir(), `whence`)
	if st != 2 || out != "Usage: whence [-afpqv] name  ...\n" {
		t.Errorf("out %q status %d, want the bare usage at 2", out, st)
	}
	// `-f` is the letter still missing; `-a` was on this list until #633 and
	// is built now — see TestWhenceAListsEveryResolution.
	out, st = runKsh(t, t.TempDir(), `whence -f echo`)
	if st != 2 || !strings.Contains(out, "whence: -f is not implemented yet") {
		t.Errorf("out %q status %d, want the not-implemented refusal at 2", out, st)
	}
}

// Several names answer in turn, and one that resolves to nothing marks the
// whole command's status without stopping the rest.
func TestWhenceCarriesOnPastAMiss(t *testing.T) {
	dir := t.TempDir()
	path := toolOnPath(t, dir)
	out, st := runKsh(t, dir, `whence echo nosuchzz tool`)
	if st != 1 || out != "echo\n"+path+"\n" {
		t.Errorf("out %q status %d, want both hits and the miss in the status", out, st)
	}
}

// `whence -a` is every resolution rather than the first, always in `-v`'s
// sentences, in the order ksh93 lists them. Measured 2026-09-05, ksh93u+.
//
// The last line is the one #633 recorded as needing FPATH machinery, and the
// measurement says otherwise: `N is an undefined function` appears exactly
// when the name had a builtin or function resolution *and* a PATH hit, with
// FPATH unset, empty or exported alike. A builtin with no file on PATH does
// not get it.
func TestWhenceAListsEveryResolution(t *testing.T) {
	dir := t.TempDir()
	path := toolOnPath(t, dir)
	for _, c := range []struct {
		name, src, want string
		st              int
	}{
		{
			"a builtin with no file on PATH is one line",
			"whence -a whence", "whence is a shell builtin\n", 0,
		},
		{
			"a PATH hit standing alone keeps the tracked-alias sentence",
			"whence -a tool", "tool is a tracked alias for " + path + "\n", 0,
		},
		{
			"a keyword", "whence -a if", "if is a keyword\n", 0,
		},
		{
			"an alias", "alias al='ls -l'; whence -a al", "al is an alias for 'ls -l'\n", 0,
		},
		{
			"a function", "f() { :; }; whence -a f", "f is a function\n", 0,
		},
		{
			// A function and a PATH hit: the function line, the plain path
			// rather than the tracked alias, and the undefined-function line
			// the pair earns.
			"a function that is also on PATH",
			"tool() { :; }; whence -a tool",
			"tool is a function\ntool is " + path + "\ntool is an undefined function\n", 0,
		},
		{
			// An alias hides nothing, so the function line is still there —
			// and the alias alone would not have earned the last line.
			"an alias over a function that is also on PATH",
			"alias tool=x; tool() { :; }; whence -a tool",
			"tool is an alias for x\ntool is a function\ntool is " + path +
				"\ntool is an undefined function\n", 0,
		},
		{
			// The alias without the function: a PATH hit that is not the
			// only line, and no undefined-function line.
			"an alias over a PATH hit alone",
			"alias tool=x; whence -a tool",
			"tool is an alias for x\ntool is " + path + "\n", 0,
		},
		{
			// The complaint is the same whence-prefixed line every other
			// mode gives, on standard error, which runKsh folds in.
			"a name that is nothing", "whence -a nosuchzz",
			"ksh: whence: nosuchzz: not found\n", 1,
		},
		{
			"every operand is answered", "whence -a if whence",
			"if is a keyword\nwhence is a shell builtin\n", 0,
		},
		{
			// -a already speaks in sentences, so -v adds nothing.
			"-v adds nothing", "whence -av if", "if is a keyword\n", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, dir, c.src)
			if out != c.want || st != c.st {
				t.Errorf("out %q status %d, want %q and %d", out, st, c.want, c.st)
			}
		})
	}
}

// `-aq` is the status with the words withheld, both ways.
func TestWhenceAQIsQuiet(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `whence -aq if`)
	if out != "" || st != 0 {
		t.Errorf("out %q status %d, want silence at 0", out, st)
	}
	out, st = runKsh(t, t.TempDir(), `whence -aq nosuchzz`)
	if out != "" || st != 1 {
		t.Errorf("out %q status %d, want silence at 1", out, st)
	}
}
