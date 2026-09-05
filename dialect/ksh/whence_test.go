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
	out, st = runKsh(t, t.TempDir(), `whence -a echo`)
	if st != 2 || !strings.Contains(out, "whence: -a is not implemented yet") {
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
