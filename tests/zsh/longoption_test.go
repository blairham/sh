// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// `--name` on the command line is every `setopt` name, and it is not ksh93's
// facility with a different roster behind it.
//
// Three things differ, all measured against zsh 5.9.2 on 2026-09-16, and each
// has a test below that the other reading would fail:
//
//   - the namespace holds the `no` forms as names in their own right, so
//     `--nonomatch` is taken (`nomatch` is canonical) and `--nonoglob` is not
//     (`noglob` is already the negation of `glob`);
//   - the `--name` spelling ignores hyphens, which no other route to the same
//     namespace does — `zsh --no-glob` is `noglob` and `zsh -o no-glob` is a
//     refusal, in one shell in one run;
//   - a refused word is `no such option: …` at **1**, naming the whole word
//     the shell was invoked by, where ksh93 writes a usage block and exits 2.
//
// And the `set` builtin does not have the spelling at all: `set --zzz q` is
// status 0 with an empty standard error and `q` as `$1`. See
// Semantics.LongOptionNamesASetOption, Semantics.SetLongOptionWord and
// Semantics.LongOptionNameIgnoresHyphens — three axes because the panel
// splits three ways, and ksh93 answers the first two differently.
//
// These call Runner.SetLongOption, which is the resolver and not the route:
// whether the **front end** hands a `--word` to it is the first of those
// three axes, read in driver.Shell.optionWord, and no test in this package
// can see that. `share/suite/zsh/invocation.tests` is what covers it, by
// starting the shipped binary with the word on its command line — measured
// by mutation, dropping that answer takes the file from strict to 64% of its
// lines while everything here stays green.

// longOptionRunner builds a shell that has read nothing yet, with one buffer
// for both streams, and hands back the buffer so a caller can read what the
// invocation said before it runs anything on the same shell.
//
// The name is a path because these cases are about the exact sentence: a
// refusal here names the whole word the shell was invoked by.
func longOptionRunner(t *testing.T) (*interp.Runner, *strings.Builder) {
	t.Helper()
	said := &strings.Builder{}
	r := preset.Runner(dialecttest.Base{
		Stdout: said, Stderr: said, Name: "/opt/homebrew/bin/zsh",
		Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
	})
	// The word the process was executed as, which the shell binaries fill in
	// at startup and dialecttest.Base does not carry. It is what an option
	// refusal at an invocation names here, so a runner without one would be
	// asserting the short name these cases exist to say is wrong.
	r.Invocation = "/opt/homebrew/bin/zsh"
	return r, said
}

// runOn runs src on a shell an invocation has already spoken to, and returns
// only what the script itself wrote.
func runOn(t *testing.T, r *interp.Runner, said *strings.Builder, src string) (string, int) {
	t.Helper()
	before := said.Len()
	if _, err := r.Run(context.Background(), preset.Parse(t, src)); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return said.String()[before:], r.ExitStatus()
}

// setLongOption applies one `--name` word at an invocation and reports what
// reached standard error, which is the route `zsh --xtrace script` takes.
func setLongOption(t *testing.T, word string) (string, int) {
	t.Helper()
	r, said := longOptionRunner(t)
	// The status first: the two operands of a return are evaluated left to
	// right, so reading the buffer in the same statement reads it empty.
	st := r.SetLongOption(word)
	return said.String(), st
}

// Every name the shell has is a command-line word of its own, and the state
// it asks for is the state the shell is then in.
//
// The listing is read on the *same* runner the word was applied to, which is
// the whole point of the route: a fresh shell would report the defaults
// however faithfully the word had been read.
func TestALongOptionWordIsAnyOptionName(t *testing.T) {
	for _, c := range []struct {
		word string
		// listed is the spelling a bare `setopt` prints once the request has
		// landed; empty means the listing must *not* hold a name for it.
		listed string
	}{
		{"xtrace", "xtrace"},
		{"noxtrace", ""},
		{"globsubst", "globsubst"},
		{"extendedglob", "extendedglob"},
		{"errexit", "errexit"},
		{"noerrexit", ""},
		{"nomatch", ""},
		{"nonomatch", "nonomatch"},
		{"glob_dots", "globdots"},
		{"nullglob", "nullglob"},
		// Case and underscores are the option table's own fold, which every
		// route to it gets; hyphens are this spelling's, and no other route
		// has them. All three spellings of the same name are here so the two
		// folds are exercised together, which is how they were measured.
		{"no_glob", "noglob"},
		{"NO_GLOB", "noglob"},
		{"no-glob", "noglob"},
		{"extended-glob", "extendedglob"},
		{"e-x-t-endedglob", "extendedglob"},
	} {
		t.Run(c.word, func(t *testing.T) {
			r, said := longOptionRunner(t)
			if st := r.SetLongOption(c.word); st != 0 || said.String() != "" {
				t.Fatalf("--%s said %q at %d, want it taken in silence", c.word, said, st)
			}
			out, st := runOn(t, r, said, "setopt\n")
			if st != 0 {
				t.Fatalf("setopt after --%s gave %d: %q", c.word, st, out)
			}
			has := false
			for _, line := range strings.Split(out, "\n") {
				if line == c.listed {
					has = true
				}
			}
			switch {
			case c.listed != "" && !has:
				t.Errorf("--%s left the listing %q, want a %q row in it", c.word, out, c.listed)
			case c.listed == "" && strings.Contains(out, strings.TrimPrefix(c.word, "no")):
				t.Errorf("--%s left the listing %q, want no row for it", c.word, out)
			}
		})
	}
}

// The state really moves, which a status of 0 and a listing row cannot say on
// their own: an option recorded and not acted on would pass both.
func TestALongOptionWordMovesTheState(t *testing.T) {
	for _, c := range []struct{ word, src, want string }{
		{"shwordsplit", `v="a b"; set -- $v; echo $#`, "2"},
		{"noshwordsplit", `v="a b"; set -- $v; echo $#`, "1"},
		{"no-glob", `printf "[%s]" /dev/nul*`, "[/dev/nul*]"},
		{"NO_GLOB", `printf "[%s]" /dev/nul*`, "[/dev/nul*]"},
		{"noexec", `echo unreachable`, ""},
	} {
		t.Run(c.word, func(t *testing.T) {
			r, said := longOptionRunner(t)
			if st := r.SetLongOption(c.word); st != 0 || said.String() != "" {
				t.Fatalf("--%s said %q at %d", c.word, said, st)
			}
			out, st := runOn(t, r, said, c.src+"\n")
			if st != 0 || strings.TrimSpace(out) != c.want {
				t.Errorf("--%s then %q gave %q at %d, want %q", c.word, c.src, out, st, c.want)
			}
		})
	}
}

// A word the namespace has not got is `no such option`, status 1, naming the
// word as it was written and the shell by what it was invoked by.
//
// The four words are four different ways of not being a name, and each one
// would be *taken* under some reading this shell must not have: `nonoglob`
// and `no_no_glob` under a second `no` strip, `xtrace=1` under ksh93's
// `=value`, and `no` under a strip that leaves nothing behind.
func TestALongOptionWordThatIsNoNameIsRefused(t *testing.T) {
	for _, word := range []string{"zzznosuch", "nonoglob", "no_no_glob", "xtrace=1", "no", "Zzz_No"} {
		t.Run(word, func(t *testing.T) {
			said, st := setLongOption(t, word)
			want := "/opt/homebrew/bin/zsh: no such option: " + word + "\n"
			if said != want || st != 1 {
				t.Errorf("--%s said %q at %d, want %q at 1", word, said, st, want)
			}
		})
	}
}

// And the `set` builtin swallows such a word whole: nothing moves, nothing is
// said, and the word does not fall through to become a positional parameter.
//
// The positional parameters are what part "swallowed" from "declined": an
// option loop that merely stopped at the word would have left it as `$1`.
func TestTheSetBuiltinDiscardsALongOptionWord(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`set --xtrace q; case $- in *x*) echo traced;; *) echo untraced;; esac; echo "[$*]"`, "untraced\n[q]\n"},
		{`set --zzznosuch q; echo "$? [$*]"`, "0 [q]\n"},
		{`set --noglob q; printf "[%s]" /dev/nul*; echo " [$*]"`, "[/dev/null] [q]\n"},
		// The terminator is still the terminator, which is the word this
		// reading must not eat.
		{`set -- --xtrace q; echo "[$*]"`, "[--xtrace q]\n"},
	} {
		out, st := answersRun(t, c.src+"\n")
		if st != 0 || out != c.want {
			t.Errorf("%q gave %q at %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}
