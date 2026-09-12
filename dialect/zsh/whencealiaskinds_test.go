// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// What the name-reporting builtins say about the two kinds of alias this
// dialect has, in each of the four shapes its letters ask for (#2097).
//
// Measured 2026-09-12 on zsh 5.9.2 with
//
//	alias -g UP='| tr a-z A-Z'; alias cmd='echo hi'; alias -s txt='cat -n'
//
// A suffix alias is the one that is not looked up by the word it was asked
// about: `whence -v p.txt` answers about `txt`, because the table is keyed on
// the extension. It also needs a word in front of the dot — `whence -v .txt`
// is not found — which is the row that keeps the keying from swallowing every
// dotfile.
func TestWhenceNamesEachKindOfAlias(t *testing.T) {
	const defs = `alias -g UP='| tr a-z A-Z'; alias cmd='echo hi'; alias -s txt='cat -n'; `
	for _, c := range []struct {
		src, want string
		status    int
	}{
		// The regular kind, which already worked, kept as the control the
		// other two are compared against.
		{`whence cmd`, "echo hi\n", 0},
		{`whence -w cmd`, "cmd: alias\n", 0},
		{`whence -c cmd`, "cmd: aliased to echo hi\n", 0},
		{`whence -v cmd`, "cmd is an alias for echo hi\n", 0},

		{`whence UP`, "| tr a-z A-Z\n", 0},
		{`whence -w UP`, "UP: global alias\n", 0},
		{`whence -c UP`, "UP: globally aliased to | tr a-z A-Z\n", 0},
		{`whence -v UP`, "UP is a global alias for | tr a-z A-Z\n", 0},

		{`whence p.txt`, "cat -n\n", 0},
		{`whence -w p.txt`, "txt: suffix alias\n", 0},
		{`whence -c p.txt`, "txt: suffix aliased to cat -n\n", 0},
		{`whence -v p.txt`, "txt is a suffix alias for cat -n\n", 0},
		// The extension and not the whole word, twice over.
		{`whence -v p.q.txt`, "txt is a suffix alias for cat -n\n", 0},
		{`whence -v .txt`, ".txt not found\n", 1},
		// And the bare extension is not a command word, so it is nobody.
		{`whence txt`, "", 1},

		// `type` is `whence -v` in this shell, so it must not have a second
		// answer of its own.
		{`type p.txt`, "txt is a suffix alias for cat -n\n", 0},
		{`type UP`, "UP is a global alias for | tr a-z A-Z\n", 0},

		// `command -v` writes a line that would define the alias back, kind
		// letter and all — except for the suffix kind, whose line is the
		// body alone. `command -V` is the sentence for all three.
		{`command -v cmd`, "alias cmd='echo hi'\n", 0},
		{`command -v UP`, "alias -g UP='| tr a-z A-Z'\n", 0},
		{`command -v p.txt`, "cat -n\n", 0},
		{`command -V p.txt`, "txt is a suffix alias for cat -n\n", 0},
	} {
		out, st := runZsh(t, t.TempDir(), defs+c.src)
		if out != c.want || st != c.status {
			t.Errorf("%s: out %q status %d, want %q at %d", c.src, out, st, c.want, c.status)
		}
	}
}

// The three letters `alias` gained, against the shell they were measured
// from. The exhaustive table is interp's, where the axes can be moved; this
// is the live dialect, which is what says the axes are actually set.
func TestTheAliasLettersAreOnInThisDialect(t *testing.T) {
	const defs = `alias -g UP='| tr a-z A-Z'; alias cmd='echo hi'; alias -s txt=cat; `
	for _, c := range []struct{ src, want string }{
		{`alias -L`, "alias -g UP='| tr a-z A-Z'\nalias cmd='echo hi'\n"},
		{`alias -s -L`, "alias -s txt=cat\n"},
		{`alias -r`, "cmd='echo hi'\n"},
		{`alias -m 'U*'`, "UP='| tr a-z A-Z'\n"},
		{`unalias -m 'U*'; alias`, "cmd='echo hi'\n"},
		{`alias +`, "UP\ncmd\n"},
		{`alias +g`, "UP\n"},
	} {
		out, st := runZsh(t, t.TempDir(), defs+c.src)
		if out != c.want || st != 0 {
			t.Errorf("%s: out %q status %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// The refusals, which are the half that says the letters are the shell's own
// rather than accepted-and-ignored.
func TestTheAliasLetterRefusals(t *testing.T) {
	for _, c := range []struct {
		src, want string
		status    int
	}{
		{`alias -r -g`, "zsh:alias:1: illegal combination of options\n", 1},
		{`alias -rs`, "zsh:alias:1: illegal combination of options\n", 1},
		{`unalias -m`, "zsh:unalias:1: not enough arguments\n", 1},
		{`alias +q`, "zsh:alias:1: bad option: +q\n", 1},
		{`alias a=1; unalias -m 'z*'`, "", 1},
	} {
		out, st := runZsh(t, t.TempDir(), c.src)
		if out != c.want || st != c.status {
			t.Errorf("%s: out %q status %d, want %q at %d", c.src, out, st, c.want, c.status)
		}
	}
}
