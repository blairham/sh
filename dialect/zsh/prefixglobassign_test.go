// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"strings"
	"testing"
)

// `GLOB_ASSIGN` and an assignment **prefix** — `name=value cmd`, the value a
// command is handed for the length of one command.
//
// **The prefix follows the statement form and not the declaration.** That is
// the finding, and it is the third shape of the question #4638 answered for
// the first two: `a=*.txt` globs under the option and `typeset a=*.txt` keeps
// the characters, and the prefix is an assignment word like the first rather
// than a utility's operand like the second. So the noun the rule is keyed on
// is **the assignment**, and not "an assignment that stores into the shell" —
// the readings agree on every row a statement can produce and part exactly
// here, because `a=*.txt cmd` stores nothing into this shell and globs all
// the same.
//
// Before this the prefix globbed in **both** states, in every dialect, which
// is a reading no shell in the panel has (#4657).
//
// Measured against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — with `-f`, over `-c`, 2026-09-26, in the
// directory globTree builds. `go version -m` on that binary says *not a Go
// executable*, and each grid runs a `print -r -- *.txt` control that prints
// the three names, so a row reporting `*.txt` is reporting an unglobbed value
// rather than an empty directory.

// The instrument: the pattern really does reach three names here, in either
// state of the option.
func TestThePrefixGlobDirectoryReallyHoldsTheNames(t *testing.T) {
	for _, setopt := range []string{"unsetopt globassign\n", "setopt globassign\n"} {
		root := globTree(t)
		out, _ := runZsh(t, root, setopt+"print -r -- *.txt\n")
		if out != "a.txt b.txt c.txt\n" {
			t.Errorf("%q: control = %q, want the three names", setopt, out)
		}
	}
}

// The grid, in both states, read through a **function** — which forks
// nothing, so what the body sees is the entry itself and not a copy of it
// that crossed an environment. `$#a` and `${a[2]}` rather than `typeset -p`
// because the kind is half the answer and an echo of `$a` cannot tell a
// three-element array from a scalar holding three names.
func TestAPrefixAssignmentGlobsOnlyUnderTheOption(t *testing.T) {
	for _, c := range []struct{ name, src, off, on string }{
		{
			// The issue's own shape, and the row that says the value is
			// matched at all.
			"several matches", `a=*.txt f`,
			"scalar[*.txt]\n", "array[a.txt|b.txt|c.txt]\n",
		},
		{
			// **One** match is a scalar and not a one-element array, which
			// is the row an implementation that rewrote the prefix as a list
			// gets wrong at status 0.
			"one match", `a=one.* f`,
			"scalar[one.*]\n", "scalar[one.only]\n",
		},
		{
			// `nullglob` leaves the name an **empty scalar**, exactly as the
			// statement form's miss does.
			"no match under nullglob", "setopt nullglob\na=*.nomatch f",
			"scalar[*.nomatch]\n", "scalar[]\n",
		},
		{
			// And `unsetopt nomatch` leaves the characters standing, so the
			// miss is the ordinary miss in both readings.
			"no match with nomatch off", "unsetopt nomatch\na=*.nomatch f",
			"scalar[*.nomatch]\n", "scalar[*.nomatch]\n",
		},
		{"a slash in the pattern", `a=dir/* f`, "scalar[dir/*]\n", "array[dir/d1|dir/d2]\n"},
		{
			// The control that must not move: no metacharacter, nothing to
			// glob, and the option has nothing to do.
			"no metacharacter", `a=plain f`,
			"scalar[plain]\n", "scalar[plain]\n",
		},
		{
			// The second control: the metacharacter is written and quoted,
			// so it is not a pattern in either state.
			"a quoted pattern", `a='*.txt' f`,
			"scalar[*.txt]\n", "scalar[*.txt]\n",
		},
		{
			// A value a parameter carries is not a written pattern, and this
			// shell's `GLOB_SUBST` default says so in either state — the
			// gate on the *value*, which this option is not.
			"a value carrying a pattern", "v='*.txt'\na=$v f",
			"scalar[*.txt]\n", "scalar[*.txt]\n",
		},
		{
			// And the two ways a script asks for that gate to open, which
			// reach this road as they reach the statement form's.
			"the tilde flag asks for it", "v='*.txt'\na=${~v} f",
			"scalar[*.txt]\n", "array[a.txt|b.txt|c.txt]\n",
		},
		{
			"globsubst asks for it", "setopt globsubst\nv='*.txt'\na=$v f",
			"scalar[*.txt]\n", "array[a.txt|b.txt|c.txt]\n",
		},
		{
			// `unsetopt glob` takes the filesystem away from every pattern,
			// this one included.
			"glob off", "unsetopt glob\na=*.txt f",
			"scalar[*.txt]\n", "scalar[*.txt]\n",
		},
		{
			// An `(N)` qualifier is `nullglob` for one pattern and reaches
			// this road too.
			"an N qualifier", `a=*.nomatch(N) f`,
			"scalar[*.nomatch(N)]\n", "scalar[]\n",
		},
		{
			// **The append is not an append.** A match replaces the name
			// whichever operator was written, exactly as it does for the
			// statement form.
			"an append a match replaces", "a=x\na+=one.* f",
			"scalar[xone.*]\n", "scalar[one.only]\n",
		},
		{
			// And the joining `+=` is still there for a value that is not a
			// pattern, which keeps the row above from reading as "the option
			// breaks append".
			"an append with no pattern", "a=x\na+=plain f",
			"scalar[xplain]\n", "scalar[xplain]\n",
		},
		{
			// The numeric attributes go whichever kind the match produced,
			// and a value with no pattern in it still reaches the arithmetic
			// reader with the attribute intact — the control that keeps this
			// from reading as "the option switches arithmetic off".
			"an integer name taking matches", "integer a\na=*.txt f",
			"", "array[a.txt|b.txt|c.txt]\n",
		},
		{
			"an integer name taking an expression", "integer a\na=2+2 f",
			"integer[4]\n", "integer[4]\n",
		},
		{
			"the second of two prefixes", `a=plain b=*.txt f`,
			"scalar[plain]\n", "scalar[plain]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct{ name, setopt, want string }{
				{"off", "unsetopt globassign\n", c.off},
				{"on", "setopt globassign\n", c.on},
			} {
				t.Run(state.name, func(t *testing.T) {
					root := globTree(t)
					src := state.setopt +
						"f() { print -r -- \"${${(t)a}%%-*}[${(j:|:)a[@]}]\"; }\n" + c.src + "\n"
					out, st := runZsh(t, root, src)
					if state.want == "" {
						// With the option off the pattern reaches the
						// arithmetic reader, which cannot read it.
						if st == 0 || !strings.Contains(out, "bad math expression") {
							t.Errorf("got %q at %d, want the arithmetic refusal", out, st)
						}
						return
					}
					if out != state.want {
						t.Errorf("got %q, want %q", out, state.want)
					}
				})
			}
		})
	}
}

// **The prefix does not persist**, in either state, and that is the half of
// the pair that keys the rule: the value is globbed and this shell's own `a`
// is untouched, so "an assignment that stores into the shell" is not what the
// option is about. Measured on zsh 5.9.2, 2026-09-26.
func TestAGlobbedPrefixIsStillTakenBackAfterward(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a name that was holding something", "a=kept\na=*.txt true", "[kept]\n"},
		{"a name that was not", "a=*.txt true", "[UNSET]\n"},
		{"a single match", "a=kept\na=one.* true", "[kept]\n"},
		{
			// A miss the shell refused takes the *statement* form's name
			// with it — measured in #4638 — and leaves a prefix's alone,
			// because a prefix never stored into this shell at all.
			"a refused miss leaves the name",
			"a=kept\neval 'a=*.nomatch true'", "[kept]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, setopt := range []string{"unsetopt globassign\n", "setopt globassign\n"} {
				root := globTree(t)
				out, _ := runZsh(t, root, setopt+c.src+"\nprint -r -- \"[${a-UNSET}]\"\n")
				if !strings.HasSuffix(out, c.want) {
					t.Errorf("%q: got %q, want it to end %q", setopt, out, c.want)
				}
			}
		})
	}
}

// **A match of more than one name reaches no child's environment.** The entry
// is an array and zsh exports no array — `a=(x y); export a; printenv a` is
// status 1 there — so the name is *absent* from what the child is handed
// rather than left showing what the shell was exporting under it.
//
// Measured on zsh 5.9.2, 2026-09-26: `setopt globassign; export a=old;
// a=*.txt printenv a` prints nothing and exits 1, and the same line at one
// match prints `one.only`. The three rows that must still reach the child are
// beside it.
func TestAPrefixThatMatchedSeveralNamesReachesNoChildEnvironment(t *testing.T) {
	if _, err := os.Stat("/usr/bin/printenv"); err != nil {
		t.Skip("no /usr/bin/printenv")
	}
	for _, c := range []struct{ name, src, off, on string }{
		{"several matches", `a=*.txt /usr/bin/printenv a`, "*.txt\n", ""},
		{"one match", `a=one.* /usr/bin/printenv a`, "one.*\n", "one.only\n"},
		{"no metacharacter", `a=plain /usr/bin/printenv a`, "plain\n", "plain\n"},
		{
			"several matches over an exported name",
			"export a=old\na=*.txt /usr/bin/printenv a", "*.txt\n", "",
		},
		{
			"the other prefix still reaches it",
			`a=*.txt b=one.* /usr/bin/printenv b`, "one.*\n", "one.only\n",
		},
		{
			"an empty scalar still reaches it",
			"setopt nullglob\na=*.nomatch /usr/bin/printenv a", "*.nomatch\n", "\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct{ name, setopt, want string }{
				{"off", "unsetopt globassign\n", c.off},
				{"on", "setopt globassign\n", c.on},
			} {
				t.Run(state.name, func(t *testing.T) {
					root := globTree(t)
					if out, _ := runZsh(t, root, state.setopt+c.src+"\n"); out != state.want {
						t.Errorf("got %q, want %q", out, state.want)
					}
				})
			}
		})
	}
}
