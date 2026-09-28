// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `zmodload -P name` puts a feature listing into an array instead of writing
// it.
//
// It was `-P is not implemented yet` at 1 — the first failing chunk of
// `V04features.ztst` after #4599 landed the listing this redirects (#4969).
//
// Measured 2026-09-28 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* for
// it — under `-f` from a script file with `env -i PATH=/usr/bin:/bin
// TERM=dumb` and a scratch `HOME`.
func TestZmodloadFeatureParameter(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			// Two thirds of this letter's surface is refusals, and they are
			// **ordered**: `-F` is asked about first, so a line with `-L`
			// and `-P` and no `-F` reports the sentence about `-F`.
			"without -F it is refused by name",
			`zmodload -P p zsh/zutil`,
			"zsh:zmodload:1: -P is only allowed with -F\n", 1,
		},
		{
			"and the -F sentence comes first even with -L written",
			`zmodload -L -P p`,
			"zsh:zmodload:1: -P is only allowed with -F\n", 1,
		},
		{
			"with -F and neither listing letter it is refused again",
			`zmodload -F -P p zsh/zutil`,
			"zsh:zmodload:1: -P can only be used with -l or -L\n", 1,
		},
		{
			// The listing itself, which is what the letter redirects: the
			// signed feature names, exactly what `-F -l` writes one to a
			// line, and nothing on standard output.
			"with -l it fills the array with the signed names",
			`zmodload zsh/zutil
			 zmodload -F -l -P p zsh/zutil
			 print -r -- "st=$? n=${#p} p=[${(j:,:)p}]"`,
			"st=0 n=4 p=[+b:zformat,+b:zparseopts,+b:zregexparse,+b:zstyle]\n", 0,
		},
		{
			// **`-L` is a different list**, which is the row that says this
			// is a redirection of two listings rather than one format: the
			// words its command line carries after the module name, so the
			// signs are off.
			"with -L it fills it with the command's own words",
			`zmodload zsh/zutil
			 zmodload -F -L -P q zsh/zutil
			 print -r -- "st=$? n=${#q} q=[${(j:,:)q}]"`,
			"st=0 n=4 q=[b:zformat,b:zparseopts,b:zregexparse,b:zstyle]\n", 0,
		},
		{
			// And with a feature disabled the two lists part company for
			// real: `-l` has four elements with a `-` on one, `-L` has
			// three. A test on an all-enabled module cannot tell them apart
			// beyond the signs.
			"a disabled feature is signed by -l and absent from -L",
			`zmodload zsh/zutil
			 zmodload -F zsh/zutil -b:zstyle
			 zmodload -F -l -P p zsh/zutil
			 zmodload -F -L -P q zsh/zutil
			 print -r -- "l=[${(j:,:)p}]"
			 print -r -- "L=[${(j:,:)q}]"`,
			"l=[+b:zformat,+b:zparseopts,+b:zregexparse,-b:zstyle]\n" +
				"L=[b:zformat,b:zparseopts,b:zregexparse]\n", 0,
		},
		{
			// The name may ride the letter or be the next word.
			"the name may be attached to the letter",
			`zmodload zsh/zutil
			 zmodload -F -l -PA zsh/zutil
			 print -r -- "n=${#A}"`,
			"n=4\n", 0,
		},
		{
			// An operand after the module still filters, which is the
			// listing's own rule reaching through the array.
			"a feature operand still filters",
			`zmodload zsh/zutil
			 zmodload -F -l -P s zsh/zutil b:zstyle
			 print -r -- "n=${#s} s=[${(j:,:)s}]"`,
			"n=1 s=[+b:zstyle]\n", 0,
		},
		{
			// The array is replaced wholesale rather than appended to.
			"the array is replaced",
			`zmodload zsh/zutil
			 p=(old keep)
			 zmodload -F -l -P p zsh/zutil
			 print -r -- "n=${#p} first=[$p[1]]"`,
			"n=4 first=[+b:zformat]\n", 0,
		},
		{
			// A module that is not loaded is the listing's own refusal, and
			// the array is left alone — the letter adds no answer where the
			// listing has none.
			"an unloaded module is the listing's refusal and fills nothing",
			`zmodload -F -l -P q zsh/datetime
			 print -r -- "st=$? n=${#q}"`,
			"zsh:zmodload:1: module `zsh/datetime' is not yet loaded\nst=1 n=0\n", 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshPrelude(t, t.TempDir(), tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q (status %d), want %q at %d", tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}

// A name nothing may be stored under ends the script.
//
// Its own test because the fatality is the half a status cannot show: a script
// that wrote a bad name and carried on would go on to read an array nothing
// filled. **The location differs from the reference and is recorded here
// rather than only in the source**: that shell reports this from its
// assignment machinery, with no builtin name in it, where every complaint a
// registered builtin makes in this engine is located at the builtin.
func TestZmodloadFeatureParameterRefusesABadName(t *testing.T) {
	out, st := runZshPrelude(t, t.TempDir(),
		"zmodload zsh/zutil\nzmodload -F -l -P 1bad zsh/zutil\nprint -r -- unreached\n")
	want := "zsh:zmodload:2: not an identifier: 1bad\n"
	if out != want || st == 0 {
		t.Errorf("a bad -P name = %q (status %d), want %q and a failure", out, st, want)
	}
}
