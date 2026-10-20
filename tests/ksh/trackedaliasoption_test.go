// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// ksh93 has no `hash` builtin. `hash` is the preset alias `alias -t --`, and
// the tracked-alias table it names is this shell's command cache — so the
// letter is answered by the builtin that already models the cache. What must
// not leak out of that reroute is the name `hash`, which is not a command a
// script can reach here, nor the cache builtin's idea of how a refusal ends.
//
// Every row measured 2026-09-15 on AT&T ksh93u+ 2012-08-01, in a script, with
// `echo after` on the line below (#3036).
func TestTheTrackedAliasRerouteRefusesAsAlias(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		// An operand that looks like an option, which is what `hash -t x`
		// and `hash -p x` come to after the preset alias expands. Two words
		// unlike `alias`'s own refusal a row below: `bad option(s)` rather
		// than `unknown option`, and no usage block under it. Fatal at 1.
		{"the tracked letter as an operand", "alias -t -- -t x", "alias: -t: bad option(s)\n", 1},
		{"the print letter as an operand", "alias -t -- -p x", "alias: -p: bad option(s)\n", 1},
		{"a letter no builtin here has", "alias -t -- -q x", "alias: -q: bad option(s)\n", 1},
		// Whole words, not letters: a bundle and a long word come back as
		// written, and a bare `-` counts.
		{"a bundle", "alias -t -- -tp x", "alias: -tp: bad option(s)\n", 1},
		{"a long word", "alias -t -- --help x", "alias: --help: bad option(s)\n", 1},
		{"a bare dash", "alias -t -- - x", "alias: -: bad option(s)\n", 1},
		// A bad letter *beside* the tracked one is `alias`'s ordinary
		// refusal instead, and it names the letter `alias` has not got
		// rather than the one it has.
		{"a bad letter beside it", "alias -tq x", "alias: -q: unknown option\nUsage: alias [-ptx] [name[=value]...]\n", 2},
		{"the same in the other order", "alias -qt x", "alias: -q: unknown option\nUsage: alias [-ptx] [name[=value]...]\n", 2},
		{"the same as two words", "alias -t -q x", "alias: -q: unknown option\nUsage: alias [-ptx] [name[=value]...]\n", 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKshWithPrelude(t, c.src+"\necho after\n")
			if out != c.want {
				t.Errorf("%s =\n%q\nwant\n%q", c.src, out, c.want)
			}
			if st != c.status {
				t.Errorf("%s: status = %d, want %d — and `after` must not be reached", c.src, st, c.status)
			}
		})
	}
}

// The words the same reading *accepts*, so the refusal above cannot be widened
// into one that swallows a working call. `-r` is the one dash-word it takes —
// `hash -r` is `alias -t -- -r` — and `--` ends the reading the way it does
// everywhere.
func TestTheTrackedAliasRerouteStillTakesItsOwnWords(t *testing.T) {
	for _, src := range []string{
		"alias -t -- -r",
		"alias -t -- -r x",
		"alias -t -- x",
		"alias -t -- -- x",
		"alias -t x",
		"alias -t",
	} {
		t.Run(src, func(t *testing.T) {
			out, st := runKshWithPrelude(t, src+"\necho after\n")
			if out != "after\n" || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", src, out, st, "after\n")
			}
		})
	}
}

// `-p` and `-x` are `alias`'s letters and not the cache's, so they used to
// reach a builtin that refused them. Beside the tracked letter and with no
// operand they are two shapes of the same listing: `alias -pt` writes the
// word that would define the entry back in front of it, and `alias -xt`
// writes nothing at all, because a tracked alias is never an exported one.
//
// Measured with one command hashed. The listing itself is the control: the
// same table under `-t` alone is the bare form, so a row that lost the prefix
// and a row that lost the listing cannot both pass.
func TestTheTrackedListingTakesAliasLetters(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, src, want string }{
		{"bare", "alias -t", "tool=" + tool + "\n"},
		{"printed", "alias -pt", "alias tool=" + tool + "\n"},
		{"exported", "alias -xt", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, dir, "tool\n"+c.src)
			if out != c.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}
