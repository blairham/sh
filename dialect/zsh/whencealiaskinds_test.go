// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"fmt"
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
		// **And not on the `-w` road either**, which is where it did have
		// one: `type -w p.txt` answered `p.txt: alias` — the asked word and
		// the plain kind — while every row above named `txt` a suffix alias
		// (#5219). These rows sat missing from this very table, which had
		// the plain road of each kind and the `-w` road of none, so the one
		// guard meant to keep `type` and `whence` together covered the half
		// that agreed.
		{`type -w p.txt`, "txt: suffix alias\n", 0},
		{`type -w UP`, "UP: global alias\n", 0},
		{`type -w cmd`, "cmd: alias\n", 0},
		// `-a` writes the same row and had the same second answer, fixed a
		// grid later than `-w` was: one call site of the shared lookup is
		// not the other.
		{`type -aw p.txt`, "txt: suffix alias\n", 0},
		{`type -aw UP`, "UP: global alias\n", 0},
		{`type -aw cmd`, "cmd: alias\n", 0},
		// The extension and not the whole word, on this road too.
		{`type -w p.q.txt`, "txt: suffix alias\n", 0},
		// And the negatives, so a rule that named the *suffix* whatever was
		// asked would not pass: a bare extension is nobody, and a word with
		// no alias behind it is `none` rather than a kind.
		// At 1, measured: a name with nothing behind it is `none` and a
		// failure, on this road as on `whence -w txt`.
		{`type -w txt`, "txt: none\n", 1},
		{`type -w p.zzz`, "p.zzz: none\n", 1},

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

// **An alias whose name holds `=` has no line under `-L`** (#5222): the entry
// is skipped and a warning goes to standard error instead.
//
// `aliases[x=y]=z` plants such a name through the tied parameter, the one
// route that can. `alias NAME=VALUE` splits at the first `=`, so a line for it
// would define something else if it were run — which is why only the listing
// that claims to write a command back refuses it. Measured 2026-09-30 against
// zsh 5.9.2; the table is on Diagnostics.AliasListingInvalidName.
//
// **Every row is one stream.** On a single pipe the reference writes all of an
// `alias` call's warnings before any of its lines — its listing is held until
// the builtin returns while standard error goes out at once, which a bad `-m`
// pattern shows just as well with no `-L` involved. That ordering is its own
// question and not this one's, so no row here depends on how the two streams
// interleave: the lines are read with standard error dropped, and the
// warnings with standard output dropped.
func TestAnAliasNameHoldingAnEqualsHasNoDefiningLine(t *testing.T) {
	const warn = "invalid alias '%s' encountered while printing aliases\n"
	three := `aliases[x=y]=z; aliases[a=b]=w; aliases[ok]=v; `
	for _, c := range []struct {
		name, src, want string
	}{
		{"the warning, alone", `aliases[x=y]=z; alias -L`, "zsh:1: " + fmt.Sprintf(warn, "x=y")},
		// The valid entry still gets its line, and the invalid ones none.
		{"the lines, with the warnings dropped", three + `alias -L 2>/dev/null`, "alias ok=v\n"},
		// One warning per skipped entry, in name order.
		{
			"the warnings, with the lines dropped", three + `alias -L >/dev/null`,
			"zsh:1: " + fmt.Sprintf(warn, "a=b") + "zsh:1: " + fmt.Sprintf(warn, "x=y"),
		},
		// The *defining form* decides and not the road to it: `-m` reaches it
		// too, and so do the other two kinds.
		{"-mL", `aliases[x=y]=z; aliases[ok]=v; alias -mL 'x*'`, "zsh:1: " + fmt.Sprintf(warn, "x=y")},
		{"-sL", `saliases[x=y]=z; alias -sL`, "zsh:1: " + fmt.Sprintf(warn, "x=y")},
		{"-gL", `galiases[x=y]=z; alias -gL`, "zsh:1: " + fmt.Sprintf(warn, "x=y")},
		// Located as the caller and not as the builtin — `f:` inside a
		// function, where `alias`'s own option errors carry `alias:`.
		{"inside a function", `aliases[x=y]=z; f() { alias -L; }; f`, "f: " + fmt.Sprintf(warn, "x=y")},
		// A warning, not a failure: the listing goes on and says 0.
		{"the status", `aliases[x=y]=z; alias -L >/dev/null 2>&1; echo st=$?`, "st=0\n"},
		// The controls. A listing that is not a command writes the entry out
		// without a word, which is what makes the rule about spelling rather
		// than about the entry being illegitimate.
		{"plain alias writes it out", `aliases[x=y]=z; alias`, "'x=y'=z\n"},
		// And a name that needs no `=` is listed under `-L` like any other.
		{"a slash is only a character", `aliases[a/b]=z; alias -L`, "alias a/b=z\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("%s: out %q status %d, want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}
