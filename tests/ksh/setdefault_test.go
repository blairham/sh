// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// `set --default` and `set --state`, the two words this shell's own usage
// line advertises and the builtin declined.
//
//	$ ksh -c 'set --zzznope'
//	ksh: set: zzznope: bad option(s)
//	Usage: set [--default] [--state] [arg ...]
//
// Both were refused here, which is #3128's complaint one level down — a
// surface naming what the shell then declines — and it is why
// `eval "$(set +o)"` still did not round-trip with every roster name moving:
// the line that idiom saves **opens** with `--default` (#3153).
//
// Measured 2026-09-16 on ksh93u+ 2012-08-01.
func TestSetTakesTheStateAndDefaultWords(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The same line `set +o` writes, which is what the word is for.
			"state writes the plus-o line", "set --state\n",
			"set --default --braceexpand --multiline --trackall --viraw\n",
		},
		{
			"and it follows a moved row",
			"set -o globstar\nset --state\n",
			"set --default --braceexpand --globstar --multiline --trackall --viraw\n",
		},
		{
			// A negated row appears under the `no…` spelling and only when
			// it is off, which is the listing's rule and not this word's.
			"a negated row too",
			"set +o clobber\nset --state\n",
			"set --default --braceexpand --noclobber --multiline --trackall --viraw\n",
		},
		{
			// **The compiled default is not the startup state.** A stock
			// shell has braceexpand, multiline and trackall on; the reset
			// leaves only viraw.
			"default resets to the compiled state",
			"set --default\nset --state\n", "set --default --viraw\n",
		},
		{
			// It reaches behavior and not only the listing.
			"and the braces stop expanding",
			"set --default\nprintf '[%s]' {a,b}\n", "[{a,b}]",
		},
		{
			// `$-` follows it: the letters go with their options. From a
			// script file a stock shell's is `hB`, so the reset empties it
			// outright — measured on ksh93u+ under the same route this
			// helper uses, where `-c` would have shown `chsB` becoming `cs`.
			"the letters go with them",
			"set -o errexit\nset --default\nprintf '[%s]' \"$-\"\n", "[]",
		},
		{
			// The positional parameters are **not** touched, which is the
			// opposite of the obvious guess about a word named "default".
			"the positional parameters stand",
			"set 1 2 3\nset --default\nprintf '[%s]' \"$*\"\n", "[1 2 3]",
		},
		{
			"and operands behind it are still operands",
			"set 1 2 3\nset --default a b\nprintf '[%s]' \"$*\"\n", "[a b]",
		},
		{
			// `--default` moves the options where it stands and `--state`
			// asks for the listing this builtin defers to the end of the
			// parse, so a `-o` after the reset survives it and the line
			// names it however the two words were ordered.
			"default applies in place",
			"set --default -o errexit\nprintf '[%s]' \"$-\"\n", "[e]",
		},
		{
			"and state is written after the whole parse",
			"set --state -o errexit\n",
			"set --default --braceexpand --errexit --multiline --trackall --viraw\n",
		},
		{
			"however the two are ordered",
			"set --state --default\n", "set --default --viraw\n",
		},
		{
			"and twice is once",
			"set --state --state\n",
			"set --default --braceexpand --multiline --trackall --viraw\n",
		},
		{
			// Unique prefixes, all the way down to one letter.
			"the words abbreviate", "set --d\nprintf '[%s]' \"$-\"\n", "[]",
		},
		{
			"and so does the other one", "set --s\n",
			"set --default --braceexpand --multiline --trackall --viraw\n",
		},
		{
			// And the round trip the whole thing is for.
			"the save line feeds back in",
			"set -o globstar -o errexit\nsaved=$(set +o)\nset --default\neval \"$saved\"\nprintf '[%s]' \"$-\"\n",
			"[ehBG]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s\n = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// And the words that are not those two are still refused, word for word as a
// name this shell has never heard of — which is what the usage line under the
// complaint is naming the alternatives to.
func TestSetStillRefusesTheWordsThatAreNotThose(t *testing.T) {
	for _, word := range []string{"zzznope", "DEFAULT", "STATE", "defaults", "x"} {
		t.Run(word, func(t *testing.T) {
			out, st := answersRun(t, "set --"+word+"\n")
			if want := word + ": bad option(s)"; !strings.Contains(out, want) || st != 2 {
				t.Errorf("set --%s = %q at %d, want %q at 2", word, out, st, want)
			}
		})
	}
}
