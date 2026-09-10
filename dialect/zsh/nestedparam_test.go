// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// The dialect turns the grammar on, so the real lines that need it run here.
//
// The substrate's tests name the flag — `NestedParamExpansion` — and this one
// names the shell, which is the only place that is allowed. It is worth
// having for the reason the element-selection one is: a flag nothing turns on
// is a construct nobody can write, and nothing in `syntax` or `interp` can
// notice that.
func TestNestedExpansionsAreThisDialects(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			// The plugin loader's own idiom, whole rather than through a
			// variable: an exclusion whose result a default falls back over.
			// `${${0:#$ZSH_ARGZERO}:-…}` is how it finds the file being
			// sourced, and until the grammar landed it was a bad
			// substitution here.
			"a pattern exclusion under a default, as a loader writes it",
			`v=/abs/p; printf "[%s]" "${${v:#/*}:-WASABSOLUTE}"`,
			"[WASABSOLUTE]",
		},
		{
			"and the relative path survives its own exclusion",
			`v=rel/p; printf "[%s]" "${${v:#/*}:-WASABSOLUTE}"`,
			"[rel/p]",
		},
		{
			"an operator on the result of an expansion",
			`v=abc; printf "[%s]" "${${v}#a}"`,
			"[bc]",
		},
		{
			"a flag group on each of two levels",
			`v=abc; printf "[%s]" "${(U)${(L)v}}"`,
			"[ABC]",
		},
		{
			// An association's *values* are what a subscript over the result
			// counts through, where the reference route reads it by key —
			// and `typeset -A` is the dialect's, so this is the only place
			// the pair can be written. A one-key table has no second
			// element; its one value has a second character.
			"an association as a value rather than as a name",
			`typeset -A m=(k abc); h=m; printf "[%s]" ${${m}[2]}; printf "[%s]" ${${(P)h}[k]}`,
			"[][abc]",
		},
		{
			// A subscript on the result, which is the same construct one
			// bracket further on. The preset is what pairs the nesting with
			// a subscript that may carry a flag group; either flag alone
			// leaves this a bad substitution.
			"a subscript on the result of an expansion",
			`a=(x y z); h=a; printf "[%s]" "${${(P)h}[2]}"`,
			"[y]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The line a hook installer actually writes, in the dialect that has to run
// it.
//
// `add-zsh-hook` asks `(( ${${(P)hook}[(I)$fn]} == 0 ))` before adding a
// function to a hook array, and every plugin that installs a `precmd` or a
// `preexec` goes through it. Until the subscript on a nested expansion
// landed, that expansion was refused, the arithmetic was left with an empty
// operand, the function printed its usage and gave up — and in a real
// session the scheduler that follows it failed and the shell exited without
// drawing a prompt (#1381).
//
// Asserted through the arithmetic rather than through the expansion's text,
// because that is where the caller breaks: an implementation answering
// *empty* for a search that found nothing satisfies a text comparison in
// some spellings and still leaves `(( … == 0 ))` a bad math expression.
func TestTheHookInstallerIdiomRuns(t *testing.T) {
	const src = `typeset -ga precmd_functions
precmd_functions=(other_hook)
add_one() {
  local hook=$1 fn=$2
  if (( ${${(P)hook}[(I)$fn]} == 0 )); then
    eval "${hook}+=( $fn )"
    print -r -- "added $fn"
  else
    print -r -- "already there: $fn"
  fi
}
add_one precmd_functions mine
add_one precmd_functions mine
print -r -- "hooks=${precmd_functions[*]}"`
	out, st := runZsh(t, t.TempDir(), src)
	want := "added mine\nalready there: mine\nhooks=other_hook mine\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// An **unquoted** substitution standing where a name belongs is split on
// `$IFS` before the outer half sees it, and a quoted one is not. The two
// spellings are different programs and this shell gave both the quoted
// answer, which nothing noticed because `echo` cannot see the difference —
// two fields print as one line and one field holding a newline prints as
// two, so `printf "[%s]"` is the instrument.
//
// Measured 2026-09-10 against zsh 5.9.2 (#976, #1697).
func TestAnUnquotedSubstitutionInTheNamePositionIsFieldSplit(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			"a bare inner",
			`printf "[%s]" ${$(printf "a b")}; echo`,
			"[a][b]\n",
		},
		{
			"and quoted, which is a different program",
			`printf "[%s]" ${"$(printf "a b")"}; echo`,
			"[a b]\n",
		},
		{
			// The flag group works on the *fields*, which is the half a
			// shell handing it one string gets silently wrong: a sort and a
			// join are both no-ops on a single field, at status 0 and with
			// nothing on either stream.
			"a flag group over the fields it was handed",
			`f() { printf "b b\na a\nc\n"; }; a=( ${(oj:-:)$(f)} ); printf "[%s]" "${a[@]}"; echo`,
			"[b-b-a-a-c]\n",
		},
		{
			"the line-splitting flag, unquoted",
			`printf "[%s]" ${(@f)$(printf "a b\nc")}; echo`,
			"[a][b][c]\n",
		},
		{
			"and quoted, which is the spelling every configuration writes",
			`printf "[%s]" ${(@f)"$(printf "a b\nc")"}; echo`,
			"[a b][c]\n",
		},
		{
			// The one context that does not split it. The double space is
			// the discriminator: split-then-join would answer 9 here, which
			// is what a single-spaced payload answers either way.
			"a length measures the inner unsplit",
			`f() { printf "b  b\na a\nc\n"; }; echo "o=${#${(o)$(f)}} f=${#${(f)$(f)}} n=${#${$(f)}}"`,
			"o=10 f=3 n=10\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The whole shape a completion dump's `autoload` line is built out of: the
// names a listing produced, joined into an alternation and matched against
// every directory on the search path.
//
//	_d_als=($^fpath/(${(o~j.|.)$(typeset +fm '_*')})(N:t))
//
// One field holding newlines is a pattern that matches no file, so the line
// came out with no names on it and the dump it wrote cached nothing (#1697).
// `cc` is in the listing and not on disk, which is what says the glob is
// doing the filtering rather than the join alone.
func TestTheAlternationACompletionDumpBuilds(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"aa", "bb"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, st := runZsh(t, t.TempDir(), `f() { printf "bb\naa\ncc\n"; }
p=(`+dir+`)
a=($^p/(${(o~j.|.)$(f)})(N:t))
printf "[%s]" "${a[@]}"
echo`)
	if out != "[aa][bb]\n" || st != 0 {
		t.Errorf("the alternation = %q (status %d), want %q at 0", out, st, "[aa][bb]\n")
	}
}
