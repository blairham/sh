// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// #4640: this shell did not enter another shell's emulation when that is what
// it was called, so a binary copied to a file called `sh` split no words,
// based its arrays at one, and answered `cd` with no HOME at 0.
//
// Against the *binary's* shell value, which is the half a package test cannot
// reach: the name arrives through `argv[0]`, and a shebang, a `chsh` entry
// and `ARGV0=` are the three things that ever set it.
//
// Every row below is real zsh 5.9.2 (aarch64-apple-darwin25.4.0), measured
// 2026-09-26 by copying `/opt/homebrew/bin/zsh` to a file with each name and
// running it under `env -u HOME` with `-f`. `go version -m` says *not a Go
// executable* for the reference, which is the check that says the two columns
// are two programs; a symlink under the same name answers as the copy does.
func TestTheNameStartsTheEmulation(t *testing.T) {
	for _, c := range []struct {
		argv0 string
		out   string
	}{
		{"zsh", "zsh\n1\n"},
		{"sh", "sh\n2\n"},
		{"ksh", "ksh\n2\n"},
		{"csh", "csh\n1\n"},
		// The first letter, which is the whole rule. No grid of plausible
		// shell names reaches either of these two, and they are what say the
		// rule is not "the word is `sh`" and not "the word begins with `sh`".
		{"b", "sh\n2\n"},
		{"xsh", "zsh\n1\n"},
		// The login spelling is the name too, and the restricted prefix is
		// dropped before the letter is read.
		{"-sh", "sh\n2\n"},
		{"rsh", "sh\n2\n"},
		{"rzsh", "zsh\n1\n"},
		// Case-sensitively, and a path is reduced to its last element.
		{"SH", "zsh\n1\n"},
		{"/usr/bin/sh", "sh\n2\n"},
	} {
		t.Run(c.argv0, func(t *testing.T) {
			var out, errs strings.Builder
			sh := scratchShell(t)
			sh.Stdout, sh.Stderr = &out, &errs
			argv := []string{c.argv0, "-f", "-c", `emulate; v="a b"; set -- $v; echo $#`}
			if got := driver.MainArgs(sh, argv); got != 0 {
				t.Fatalf("status %d, want 0 (err %q)", got, errs.String())
			}
			if out.String() != c.out {
				t.Errorf("stdout %q, want %q", out.String(), c.out)
			}
		})
	}
}

// `cd` with no operand and no HOME is the chunk `B01cd.ztst` stops on, and it
// is the one axis this shell moves with the name that has no option name over
// it. Measured in the same run, `env -u HOME`:
//
//	sh ksh csh  ->  zsh:cd:1: HOME not set, status 1
//	zsh         ->  nothing, status 0
//
// The diagnostic keeps the shell's own name whatever the binary was called,
// which is the reference's answer and not a slip: it writes `zsh:cd:1:` under
// every one of those names.
func TestCdWithNoHomeFollowsTheNamesEmulation(t *testing.T) {
	for _, c := range []struct {
		argv0  string
		errs   string
		status int
	}{
		{"sh", "zsh:cd:1: HOME not set\n", 1},
		{"ksh", "zsh:cd:1: HOME not set\n", 1},
		{"csh", "zsh:cd:1: HOME not set\n", 1},
		{"zsh", "", 0},
		{"xsh", "", 0},
	} {
		t.Run(c.argv0, func(t *testing.T) {
			var out, errs strings.Builder
			sh := scratchShell(t)
			sh.Stdout, sh.Stderr = &out, &errs
			// No HOME at all rather than an empty one: they are two states
			// and only the absent one is "HOME not set", measured across the
			// panel and pinned in interp.Semantics.CdEmptyHomeIsAnError.
			sh.Env = withoutHome(os.Environ())
			if got := driver.MainArgs(sh, []string{c.argv0, "-f", "-c", "cd"}); got != c.status {
				t.Errorf("status %d, want %d (out %q, err %q)", got, c.status, out.String(), errs.String())
			}
			if errs.String() != c.errs {
				t.Errorf("stderr %q, want %q", errs.String(), c.errs)
			}
		})
	}
}

// The same axis through the option, which is what says it belongs to the
// *mode* rather than to the name: two routes into one emulation agree.
// Measured on the reference under its own name with `env -u HOME`,
// `--emulate sh|ksh|csh -c cd` all write `zsh:cd:1: HOME not set` at 1 and
// `--emulate zsh -c cd` is silent at 0.
//
// csh parts from zsh here where it agrees with it on every option name this
// shell models, which is why the mode carries two booleans and not one.
func TestCdWithNoHomeFollowsTheEmulationOption(t *testing.T) {
	for _, c := range []struct {
		mode   string
		errs   string
		status int
	}{
		{"sh", "zsh:cd:1: HOME not set\n", 1},
		{"ksh", "zsh:cd:1: HOME not set\n", 1},
		{"csh", "zsh:cd:1: HOME not set\n", 1},
		{"zsh", "", 0},
		{"fish", "", 0},
	} {
		t.Run(c.mode, func(t *testing.T) {
			var out, errs strings.Builder
			sh := scratchShell(t)
			sh.Stdout, sh.Stderr = &out, &errs
			sh.Env = withoutHome(os.Environ())
			argv := []string{"zsh", "--emulate", c.mode, "-f", "-c", "cd"}
			if got := driver.MainArgs(sh, argv); got != c.status {
				t.Errorf("status %d, want %d (out %q, err %q)", got, c.status, out.String(), errs.String())
			}
			if errs.String() != c.errs {
				t.Errorf("stderr %q, want %q", errs.String(), c.errs)
			}
		})
	}
}

// And the option still wins over the name, in both directions — measured on
// the reference under both spellings. It is written first because it has to
// be: the option must precede every other option word, which is the dialect's
// own rule and is not loosened by the name having already answered.
func TestTheEmulateOptionWinsOverTheName(t *testing.T) {
	for _, c := range []struct {
		name string
		argv []string
		out  string
	}{
		{"the option replaces the name's mode", []string{"sh", "--emulate", "zsh", "-f", "-c", "emulate"}, "zsh\n"},
		{"and the other way round", []string{"zsh", "--emulate", "sh", "-f", "-c", "emulate"}, "sh\n"},
		// A mode no shell knows is the builtin's silence, and the name is
		// not a fallback for it: the reference called `sh` reports `zsh`.
		{"a word that names nothing still puts the name aside", []string{"sh", "--emulate", "fish", "-f", "-c", "emulate"}, "zsh\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errs strings.Builder
			sh := scratchShell(t)
			sh.Stdout, sh.Stderr = &out, &errs
			if got := driver.MainArgs(sh, c.argv); got != 0 {
				t.Fatalf("status %d, want 0 (err %q)", got, errs.String())
			}
			if out.String() != c.out {
				t.Errorf("stdout %q, want %q", out.String(), c.out)
			}
		})
	}
}

// The name is read before the invocation's own options, which is the ordering
// an emulation gets wrong invisibly: the mode resets the option table, so a
// name applied second would undo what the command line asked for. Measured on
// the reference copied to `sh`: `+o shwordsplit` turns the splitting the name
// brought back off again, so the option is read after the name.
func TestTheInvocationsOptionsAreReadAfterTheName(t *testing.T) {
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	argv := []string{"sh", "-f", "+o", "shwordsplit", "-c", `v="a b"; set -- $v; echo $#`}
	if got := driver.MainArgs(sh, argv); got != 0 {
		t.Fatalf("status %d, want 0 (err %q)", got, errs.String())
	}
	if out.String() != "1\n" {
		t.Errorf("stdout %q, want %q — the option is read after the name", out.String(), "1\n")
	}
}

func withoutHome(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(kv, "HOME=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}
