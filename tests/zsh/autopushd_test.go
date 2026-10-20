// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// autopushdDir is a scratch directory with `sub` and `sub/deep` in it, made
// here rather than by a `mkdir` in the snippet: these cases run with `PATH`
// pointing at the scratch directory and nothing else on it, so there is no
// `mkdir` to reach.
func autopushdDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runAutopushd runs a snippet with the prelude installed — `pushd`, `popd`
// and `dirs` are prelude functions, so a case about the directory stack that
// used the plain helper would be asking about a shell that has none.
//
// `HOME` is pointed at a path that is not a prefix of the scratch directory,
// because `dirs` abbreviates `$HOME` to `~` and a run whose temporary
// directory happened to sit under the caller's home would print a different
// string for the same stack.
func runAutopushd(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	out, st, err := preset.CombinedWithPrelude(t, dialecttest.Base{
		Dir:  dir,
		Vars: map[string]string{"PATH": dir, "HOME": filepath.Join(dir, "nohome")},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// `AUTO_PUSHD` makes every `cd` a `pushd`, so the directory stack grows as
// the shell moves.
//
// It was accepted and then ignored until #4592 — the name went into the
// recorded store, `[[ -o autopushd ]]`, `$options[autopushd]` and the `setopt`
// listing all reported it faithfully, its letter `-N` was wired, and `cd`
// pushed nothing. `pushd`, `popd` and `dirs` kept a stack the whole time,
// which is what made the gap a routing one rather than a missing feature.
//
// **Every case is run in both states of the option**, so a shell that ignores
// it fails one half of every pair rather than passing a row that only ever
// asked it one question. Two of the rows are deliberately the same on both
// sides — a `cd` that failed and the outer shell after a `cd` in a subshell —
// and they are here because each is a way the push could have been written
// that is wrong in one state only.
//
// Measured against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), which `go version -m` calls *not a Go
// executable* — run `-f` from a scratch directory, 2026-09-26.
func TestAutoPushdMakesEveryCdAPushd(t *testing.T) {
	// %s is where the one word the two states differ by goes.
	for _, c := range []struct{ name, src, on, off string }{
		{
			// The issue's own reduction, counted the way its suite chunk
			// counts it: `dirs` puts `$PWD` in front of the array, so one
			// `cd` under the option is two printed rows and one element.
			name: "one cd, counted both ways",
			src:  "setopt %s\ncd sub\ndirs -v\nprint \"n=$#dirstack\"\n",
			on:   "0\t%[1]s/sub\n1\t%[1]s\nn=1\n",
			off:  "0\t%[1]s/sub\nn=0\n",
		},
		{
			name: "two cds stack up",
			src:  "setopt %s\ncd sub\ncd deep\nprint \"n=$#dirstack\"\n",
			on:   "n=2\n",
			off:  "n=0\n",
		},
		{
			// `cd -` is a `cd` like any other and pushes like one, so going
			// back and forth grows the stack rather than canceling out.
			name: "cd - pushes too",
			src:  "setopt %s\ncd sub\ncd -\nprint \"n=$#dirstack\"\n",
			on:   "n=2\n",
			off:  "n=0\n",
		},
		{
			// **Not conditional on the directory changing.** `cd .` arrives
			// where it already was and pushes that.
			name: "cd to where the shell already is",
			src:  "setopt %s\ncd .\nprint \"n=$#dirstack\"\n",
			on:   "n=1\n",
			off:  "n=0\n",
		},
		{
			// **Conditional on arriving.** Same on both sides, and the row
			// that says the push belongs after the move rather than before
			// it: pushing at the top of the builtin would have left an entry
			// behind for a `cd` that went nowhere.
			name: "a cd that failed pushes nothing",
			src:  "setopt %s\ncd nope 2>/dev/null\nprint \"st=$? n=$#dirstack\"\n",
			on:   "st=1 n=0\n",
			off:  "st=1 n=0\n",
		},
		{
			// A `cd` anywhere is a `cd`: the option is not about the top
			// level.
			name: "a cd inside a function",
			src:  "setopt %s\nf() { cd sub }\nf\nprint \"n=$#dirstack\"\n",
			on:   "n=1\n",
			off:  "n=0\n",
		},
		{
			// The stack a subshell grows is the subshell's, which is the
			// outer half being the same on both sides. It is here because
			// the stack is a shell parameter and a push that reached the
			// parent would be a push written somewhere other than the
			// parameter.
			name: "a cd inside a subshell",
			src:  "setopt %s\n(cd sub; print \"inner=$#dirstack\")\nprint \"outer=$#dirstack\"\n",
			on:   "inner=1\nouter=0\n",
			off:  "inner=0\nouter=0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct{ name, word, want string }{
				{"on", "autopushd", c.on},
				{"off", "noautopushd", c.off},
			} {
				t.Run(state.name, func(t *testing.T) {
					dir := autopushdDir(t)
					want := state.want
					if strings.Contains(want, "%[1]s") {
						want = fmt.Sprintf(want, dir)
					}
					out, st := runAutopushd(t, dir, fmt.Sprintf(c.src, state.word))
					if out != want || st != 0 {
						t.Errorf("got %q status %d, want %q at 0", out, st, want)
					}
				})
			}
		})
	}
}

// **The state at the moment `cd` runs is what decides, not the state when it
// finishes**, and the two can be told apart because `cd` ends by calling
// `chpwd`, which is a shell function and can move the option.
//
// This is the grid #4562 said to build: the option is varied *between* the
// two candidate moments rather than around the whole `cd`. A conversion that
// read the axis at the end of the builtin passes every row of the test above
// and gets both rows here backwards.
//
// Measured 2026-09-26 on zsh 5.9.2, and the third row is why the answer is
// "when it starts" rather than "before the hook by luck": `chpwd` can already
// see the pushed entry, so the push is done before the hook is told anything.
func TestTheOptionIsReadWhenCdStartsAndNotWhenItFinishes(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			// On at the `cd`, off by the time it ends: pushed anyway.
			name: "a chpwd that turns it off does not take the push back",
			src:  "setopt autopushd\nchpwd() { unsetopt autopushd }\ncd sub\nprint \"n=$#dirstack\"\n",
			want: "n=1\n",
		},
		{
			// Off at the `cd`, on by the time it ends: nothing pushed.
			name: "a chpwd that turns it on causes no push",
			src:  "unsetopt autopushd\nchpwd() { setopt autopushd }\ncd sub\nprint \"n=$#dirstack\"\n",
			want: "n=0\n",
		},
		{
			name: "the hook can already see the pushed entry",
			src:  "setopt autopushd\nchpwd() { print \"in chpwd: n=$#dirstack\" }\ncd sub\n",
			want: "in chpwd: n=1\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := autopushdDir(t)
			out, st := runAutopushd(t, dir, c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// **And the noun is the `cd`, not a directory change.**
//
// `pushd` and `popd` are builtins in the shell being modeled and are prelude
// *functions* that call `cd` in this one, so the reading that says "a
// directory change pushes" makes `pushd sub` push twice and `popd` push
// instead of popping. Measured 2026-09-26: real zsh's `pushd sub` under the
// option leaves one entry and its `popd` leaves none, exactly as with the
// option off.
//
// This is the test the prelude's snapshot is for — see dialect/zsh/prelude.go,
// where each of those functions reads the stack into the positional
// parameters before its `cd` and assigns the whole array afterwards. The
// mutation that kills it is moving either read back after the `cd`: with the
// option on, `pushd` then reports two entries and `popd` one.
func TestPushdAndPopdAreUnmovedByTheOption(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			name: "pushd pushes one entry and popd takes it away",
			src:  "setopt %s\npushd sub\nprint \"a: n=$#dirstack pwd=${PWD:t}\"\npopd\nprint \"b: n=$#dirstack pwd=${PWD:t}\"\n",
			want: "a: n=1 pwd=sub\nb: n=0 pwd=nowhere\n",
		},
		{
			name: "pushd with no operand swaps the top two",
			src:  "setopt %s\npushd sub\npushd\nprint \"n=$#dirstack pwd=${PWD:t}\"\n",
			want: "n=1 pwd=nowhere\n",
		},
		{
			name: "a rotation turns the stack and does not lengthen it",
			src:  "setopt %s\npushd sub >/dev/null\npushd deep >/dev/null\nprint \"a: n=$#dirstack pwd=${PWD:t}\"\npushd +1 >/dev/null\nprint \"b: n=$#dirstack pwd=${PWD:t}\"\n",
			want: "a: n=2 pwd=deep\nb: n=2 pwd=sub\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, word := range []string{"autopushd", "noautopushd"} {
				t.Run(word, func(t *testing.T) {
					dir := autopushdDir(t)
					// The scratch directory's own base name, which the
					// snippets print through `${PWD:t}`.
					want := strings.ReplaceAll(c.want, "nowhere", filepath.Base(dir))
					out, st := runAutopushd(t, dir, fmt.Sprintf(c.src, word))
					if out != want || st != 0 {
						t.Errorf("got %q status %d, want %q at 0", out, st, want)
					}
				})
			}
		})
	}
}

// `$dirstack` **is** the stack rather than a view over one, which is what
// real zsh's is: a script may assign to it, and the assignment is the new
// stack.
//
// It was on the absent roster until #4592 and refused by name — `$#dirstack`
// was `dirstack: parameter not implemented yet` at status 1, which ended the
// script. The stack it reports on was already there, under a private name
// (`DIRSTACK`) that real zsh does not have at all, so the parameter was what
// was missing and not the fact.
//
// **One stack and not two**: the rows below write through the parameter and
// read through `dirs`, and write through `pushd` and read through the
// parameter, so a second copy behind either would show up as a disagreement.
func TestTheDirstackParameterIsTheStack(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			// The refusal that used to be, in the shape the issue reported.
			name: "reading it is not refused",
			src:  "print \"n=$#dirstack [${dirstack[*]}]\"\n",
			want: "n=0 []\n",
		},
		{
			name: "what pushd pushed is what the parameter holds",
			src:  "pushd sub >/dev/null\nprint \"n=$#dirstack base=${dirstack[1]:t}\"\n",
			want: "n=1 base=nowhere\n",
		},
		{
			// Written through the parameter, read through `dirs`: the
			// current directory in front, then the array.
			name: "an assignment is the new stack",
			src:  "dirstack=(/one /two)\ndirs\n",
			want: "%[1]s /one /two\n",
		},
		{
			// And the builtins act on what the assignment left, which is
			// what says there is no second store behind them.
			name: "popd follows an assigned stack",
			src:  "dirstack=(/ /one)\npopd\nprint \"pwd=$PWD n=$#dirstack [${dirstack[*]}]\"\n",
			want: "pwd=/ n=1 [/one]\n",
		},
		{
			// And `dirs` writing the stack is visible through the
			// parameter, which is the same check in the other direction.
			name: "an operand to dirs replaces what the parameter holds",
			src:  "dirs /a /b\nprint \"n=$#dirstack [${dirstack[*]}]\"\n",
			want: "n=2 [/a /b]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := autopushdDir(t)
			want := strings.ReplaceAll(c.want, "nowhere", filepath.Base(dir))
			if strings.Contains(want, "%[1]s") {
				want = fmt.Sprintf(want, dir)
			}
			out, st := runAutopushd(t, dir, c.src)
			if out != want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, want)
			}
		})
	}
}
