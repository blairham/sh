// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `vared`, which is the last of #1405's three: `zcompile` was built on that
// issue's own evidence (#2079) and `zregexparse` stays absent with the
// completion system it belongs to.
//
// **Everything a script outside a session sees is here**, and that is not a
// subset: measured 2026-09-15 against zsh 5.9.2 with standard input closed and
// again with a pipe on it, every run without a terminal ends at `can't access
// terminal`. So the rows below are the whole builtin for a script.
func TestVaredRefusesTheWayZshDoes(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"no operands", `vared`, "zsh:vared:1: not enough arguments\n"},
		{"two operands", `vared a b`, "zsh:vared:1: too many arguments\n"},
		{"a name that is not one", `vared 1bad`, "zsh:vared:1: not an identifier: `1bad'\n"},
		{"and with a subscript on it", `vared "1bad[1]"`, "zsh:vared:1: not an identifier: `1bad[1]'\n"},
		{"a name nothing has set", `vared nosuch`, "zsh:vared:1: no such variable: nosuch\n"},
		{"a bad option", `vared -Q v`, "zsh:vared:1: bad option: -Q\n"},
		// The bad option is found before the missing argument, measured.
		{"a bad option in front of one", `vared -Q -p`, "zsh:vared:1: bad option: -Q\n"},
		{"an option with nothing behind it", `vared -p`, "zsh:vared:1: argument expected: -p\n"},
		{"the two kind letters together", `vared -aA v`, "zsh:vared:1: specify only one of -a and -A\n"},
		// The terminal refusal is what every well-formed line reaches.
		{"a set name", `v=x; vared v`, "zsh:vared:1: can't access terminal\n"},
		{"with the create letter", `vared -c nosuch`, "zsh:vared:1: can't access terminal\n"},
		{"an array", `a=(1 2); vared a`, "zsh:vared:1: can't access terminal\n"},
		{"an association", `typeset -A m; vared m`, "zsh:vared:1: can't access terminal\n"},
		{"a prompt", `v=x; vared -p "P " v`, "zsh:vared:1: can't access terminal\n"},
		{"the letters clustered", `v=x; vared -ch v`, "zsh:vared:1: can't access terminal\n"},
		{"an ended option list", `v=x; vared -- v`, "zsh:vared:1: can't access terminal\n"},
		{"an attached argument", `v=x; vared -p"P " v`, "zsh:vared:1: can't access terminal\n"},
		// `-a` and `-A` say what kind of variable to *create*, so they mean
		// something only beside `-c`: without it they are a warning and the
		// line goes on.
		{"the array letter alone", `v=x; vared -a v`, "zsh:vared:1: -a ignored\nzsh:vared:1: can't access terminal\n"},
		{"the table letter alone", `v=x; vared -A v`, "zsh:vared:1: -A ignored\nzsh:vared:1: can't access terminal\n"},
		{"and the warning comes first", `vared -a 1bad`, "zsh:vared:1: -a ignored\nzsh:vared:1: not an identifier: `1bad'\n"},
		{"beside the create letter it is silent", `vared -c -a nosuch`, "zsh:vared:1: can't access terminal\n"},
		// `-f` takes an argument, which is easy to miss and is the row that
		// says so: the `v` went to the letter and `w` is the operand.
		{"the function letter takes one", `v=x; vared -f v w`, "zsh:vared:1: no such variable: w\n"},
		{"and so do -M, -t and -i", `v=x; vared -M v`, "zsh:vared:1: not enough arguments\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 1 {
				t.Errorf("%s = %q at %d, want %q at 1", tc.src, out, st, tc.want)
			}
		})
	}
}

// The name check is **far narrower** than the name it is about, which is
// measured rather than inferred from what a declaration would accept: only a
// word that starts with a digit and is not all digits is refused, and a
// parameter the shell answers for itself is set by being spellable.
//
// A tighter reading wrote `not an identifier` for `a[1]`, where that shell
// edits the element; asking `${+name}` instead wrote `bad substitution` for
// `a b` and called a positional nothing had set missing.
func TestVaredsNameCheckIsNarrowerThanAName(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"a subscript on a set array", `a=(1 2); vared "a[1]"`, "zsh:vared:1: can't access terminal\n"},
		{"and on a scalar", `v=x; vared "v[1]"`, "zsh:vared:1: can't access terminal\n"},
		{"a subscript on nothing", `vared "nosuch[1]"`, "zsh:vared:1: no such variable: nosuch[1]\n"},
		{"a word with a space", `vared "a b"`, "zsh:vared:1: no such variable: a b\n"},
		{"a word with a dash", `vared "a-b"`, "zsh:vared:1: no such variable: a-b\n"},
		{"an unclosed bracket", `vared "a["`, "zsh:vared:1: no such variable: a[\n"},
		{"the empty word", `vared ""`, "zsh:vared:1: no such variable: \n"},
		// A positional parameter is there whether or not anything set it.
		{"a positional nothing set", `vared 9`, "zsh:vared:1: can't access terminal\n"},
		{"a two-digit one", `vared 12`, "zsh:vared:1: can't access terminal\n"},
		{"and one that was set", `set -- q; vared 1`, "zsh:vared:1: can't access terminal\n"},
		// And so is a parameter that is a punctuation mark.
		{"the list", `vared "@"`, "zsh:vared:1: can't access terminal\n"},
		{"the status", `vared "?"`, "zsh:vared:1: can't access terminal\n"},
		{"the count", `vared "#"`, "zsh:vared:1: can't access terminal\n"},
		// The control: an ordinary name is looked up, and a digit in front
		// of letters is the one shape refused.
		{"an ordinary name the shell has", `vared PATH`, "zsh:vared:1: can't access terminal\n"},
		{"an ordinary name it has not", `vared _x`, "zsh:vared:1: no such variable: _x\n"},
		{"digits in front of letters", `vared "12abc"`, "zsh:vared:1: not an identifier: `12abc'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 1 {
				t.Errorf("%s = %q at %d, want %q at 1", tc.src, out, st, tc.want)
			}
		})
	}
}

// With a terminal the editing half is refused **by name**, and the sentence
// says which half is missing rather than claiming the terminal is out of
// reach: a shell at a prompt has one, and `can't access terminal` there would
// be a wrong sentence and not a missing feature.
func TestVaredNamesTheHalfItHasNotGot(t *testing.T) {
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: t.TempDir(), Terminal: true, Interactive: true,
	}, `v=x; vared v; echo "st=$?"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "zsh:vared:1: the line editor cannot be re-entered from a command yet\nst=1\n"; out != want || st != 0 {
		t.Errorf("with a terminal = %q at %d, want %q at 0", out, st, want)
	}
	// The control, and the pair that says the two answers are told apart by
	// the terminal rather than by the shell having given up: without one the
	// refusal is the shell's own sentence, which is what a script sees.
	out, st = runZsh(t, t.TempDir(), `v=x; vared v; echo "st=$?"`)
	if want := "zsh:vared:1: can't access terminal\nst=1\n"; out != want || st != 0 {
		t.Errorf("without one = %q at %d, want %q at 0", out, st, want)
	}
	// And the refusals in front of it do not depend on the terminal at all:
	// a bad option is a bad option at a prompt too.
	out, _, err = preset.Combined(t, dialecttest.Base{
		Dir: t.TempDir(), Terminal: true, Interactive: true,
	}, `vared -Q v`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "bad option: -Q") {
		t.Errorf("a bad option with a terminal = %q, want it refused as one", out)
	}
}
