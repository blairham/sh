// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// `sh_option_letters` re-points the single-letter options at sh's meanings.
//
// Measured 2026-09-26 on zsh 5.9.2 (aarch64-apple-darwin25.4.0) a letter at a
// time, in both routes — `zsh [--emulate sh] -L script` and `zsh -c
// '[emulate sh;] set -L; setopt'` — each against the same shell with no
// letter. The two routes agree on every row, and the rows here are the ones
// where the two sets disagree; a letter the sets spell alike is left to the
// table the panel shares (#4518).
func TestShOptionLettersMoveTheLettersThatDisagree(t *testing.T) {
	for _, tc := range []struct{ name, letter, zsh, sh string }{
		{"f is norcs here and globbing there", "f", "rcs=off glob=on", "rcs=on glob=off"},
		{
			"T is cdablevars here and trapsasync there", "T",
			"cdablevars=on trapsasync=off", "cdablevars=off trapsasync=on",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			names := strings.FieldsFunc(tc.zsh+" "+tc.sh, func(r rune) bool { return r == ' ' })
			var read strings.Builder
			seen := map[string]bool{}
			for _, n := range names {
				name, _, _ := strings.Cut(n, "=")
				if seen[name] {
					continue
				}
				seen[name] = true
				read.WriteString("if [[ -o " + name + " ]]; then printf '" + name +
					"=on '; else printf '" + name + "=off '; fi\n")
			}
			out, st := runZsh(t, t.TempDir(), "set -"+tc.letter+"\n"+read.String()+"print")
			if want := tc.zsh + " \n"; st != 0 || out != want {
				t.Errorf("own letters: out %q status %d, want %q", out, st, want)
			}
			out, st = runZsh(t, t.TempDir(),
				"setopt shoptionletters\nset -"+tc.letter+"\n"+read.String()+"print")
			if want := tc.sh + " \n"; st != 0 || out != want {
				t.Errorf("sh letters: out %q status %d, want %q", out, st, want)
			}
		})
	}
}

// `-X` is the third row that moves, and it moves from *nothing*: this shell's
// set takes it and changes no option a listing shows, where sh's set spells
// `markdirs` with it.
func TestShOptionLettersGiveXAMeaning(t *testing.T) {
	const read = "if [[ -o markdirs ]]; then print md=on; else print md=off; fi"
	out, st := runZsh(t, t.TempDir(), "set -X\n"+read)
	if want := "md=off\n"; st != 0 || out != want {
		t.Errorf("own letters: out %q status %d, want %q", out, st, want)
	}
	out, st = runZsh(t, t.TempDir(), "setopt shoptionletters\nset -X\n"+read)
	if want := "md=on\n"; st != 0 || out != want {
		t.Errorf("sh letters: out %q status %d, want %q", out, st, want)
	}
}

// And the rest of the difference is an absence: sh's set is the smaller of
// the two, so every letter this shell spends on an option of its own is a
// `bad option` there.
//
// The three at the front are the ones the *substrate* would otherwise answer
// for — `-H` outright and `-h` and `-E` through an axis this dialect has
// never had to answer, because its own table reached them first — so they are
// refused by name rather than by absence. See shRefusedLetters.
func TestShOptionLettersRefuseTheLettersOnlyThisShellHas(t *testing.T) {
	for _, letter := range []string{"h", "H", "E", "d", "g", "k", "w", "y", "B", "G", "Q", "3"} {
		t.Run(letter, func(t *testing.T) {
			out, st, errs := runZshSplit(t, t.TempDir(),
				"setopt shoptionletters\nset -"+letter+"\nprint ran")
			if want := ":set:2: bad option: -" + letter + "\n"; !strings.HasSuffix(errs, want) {
				t.Errorf("stderr %q, want it to end %q", errs, want)
			}
			if st == 0 {
				t.Errorf("status 0 for a letter this set does not have")
			}
			if strings.Contains(out, "ran") {
				t.Errorf("out %q, want the refusal to have stopped the script", out)
			}
		})
	}
}

// The control every row above rests on: with the option off the same letters
// are this shell's own, and one of them is a letter sh's set refuses.
func TestThisShellsOwnLettersStandWithTheOptionOff(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"unsetopt shoptionletters\nset -g\nif [[ -o histignorespace ]]; then print g=on; else print g=off; fi")
	if want := "g=on\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// An emulation turns the option on by its own default, and the letters move
// with it — which is what the invocation route needs, since `--emulate` is
// applied before the letters are.
func TestAnEmulationMovesTheLetterSet(t *testing.T) {
	const read = "if [[ -o glob ]]; then print glob=on; else print glob=off; fi"
	out, st := runZsh(t, t.TempDir(), "emulate sh\nset -f\n"+read)
	if want := "glob=off\n"; st != 0 || out != want {
		t.Errorf("emulate sh: out %q status %d, want %q", out, st, want)
	}
	out, st = runZsh(t, t.TempDir(), "emulate sh\nemulate zsh\nset -f\n"+read)
	if want := "glob=on\n"; st != 0 || out != want {
		t.Errorf("emulate zsh puts them back: out %q status %d, want %q", out, st, want)
	}
}

// The invocation route, which is what #4518 is about: `--emulate sh -f` is
// sh's `-f` and not this shell's.
//
// Through driver.MainArgs rather than through the option table, because the
// front end is the half that was missing: it reads the startup-file letters
// before there is a runner to ask, so the letter never reached the dialect's
// table at all. See Semantics.StartupFileOptions.LettersBorrowedUnderEmulation.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0), 2026-09-26, with an
// empty HOME: `--emulate sh -f` leaves `rcs` on and turns `glob` off, where
// plain `-f` does the opposite; `--emulate csh -f` and `--emulate zsh -f`
// keep this shell's reading; `-d` is `bad option` under the two borrowed
// modes and `noglobalrcs` under the other two; `-l` is login in every mode;
// and the long spelling `--no-rcs` means what it always meant.
func TestShOptionLettersReachTheInvocationsLetters(t *testing.T) {
	const read = `if [[ -o rcs ]]; then printf "rcs=on "; else printf "rcs=off "; fi
if [[ -o glob ]]; then printf "glob=on "; else printf "glob=off "; fi
print`
	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		{"the borrowed set reads f as globbing", []string{"zsh", "--emulate", "sh", "-f", "-c", read}, "rcs=on glob=off \n"},
		{"and ksh borrows the same set", []string{"zsh", "--emulate", "ksh", "-f", "-c", read}, "rcs=on glob=off \n"},
		{"csh does not borrow it", []string{"zsh", "--emulate", "csh", "-f", "-c", read}, "rcs=off glob=on \n"},
		{"nor does zsh's own mode", []string{"zsh", "--emulate", "zsh", "-f", "-c", read}, "rcs=off glob=on \n"},
		{"and with no emulation at all", []string{"zsh", "-f", "-c", read}, "rcs=off glob=on \n"},
		{"the long spelling is untouched", []string{"zsh", "--emulate", "sh", "--no-rcs", "-c", read}, "rcs=off glob=on \n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			code := driver.MainArgs(zshWriting(&out, &errs), tc.argv)
			if out.String() != tc.want || errs.String() != "" || code != 0 {
				t.Errorf("%v ran %q / said %q status %d, want %q and nothing said",
					tc.argv[1:], out.String(), errs.String(), code, tc.want)
			}
		})
	}
}

// `-l` is spelled `l` in both sets, so it survives the borrowing — which is
// what says the rule is about the *letters* and not about the options.
func TestTheLoginLetterSurvivesTheBorrowedSet(t *testing.T) {
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs),
		[]string{"zsh", "--emulate", "sh", "-l", "-c", `[[ -o login ]] && echo login`})
	if out.String() != "login\n" || code != 0 {
		t.Errorf("ran %q / said %q status %d, want login at 0", out.String(), errs.String(), code)
	}
}

// And `-d`, this shell's other startup letter, is a letter the borrowed set
// does not have at all.
func TestTheSystemStartupLetterIsRefusedInTheBorrowedSet(t *testing.T) {
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs),
		[]string{"zsh", "--emulate", "sh", "-d", "-c", "print ran"})
	if want := "bad option: -d\n"; !strings.HasSuffix(errs.String(), want) || code == 0 {
		t.Errorf("said %q status %d, want it to end %q at nonzero", errs.String(), code, want)
	}
	if strings.Contains(out.String(), "ran") {
		t.Errorf("ran %q, want the refusal to have stopped the shell", out.String())
	}
}

// The letter set goes back with the option table, which the loop that puts
// the table back cannot do on its own: the option keeps its state in the
// recorded store, so the wholesale write reaches it and the loop then finds
// the name where it is being put and calls nothing.
//
// Measured on zsh 5.9.2, 2026-09-26: after `emulate sh -c ':'` and after a
// function whose body ran `emulate -L sh`, `set -f` is this shell's `-f`
// again. Each row reads the state *inside* as well as after, so a row cannot
// pass on a shell that never borrowed the letters at all.
func TestTheBorrowedLetterSetGoesBackWithTheOptionTable(t *testing.T) {
	const read = "if [[ -o glob ]]; then printf 'glob=on '; else printf 'glob=off '; fi\n" +
		"if [[ -o rcs ]]; then print rcs=on; else print rcs=off; fi"
	for _, tc := range []struct{ name, src, want string }{
		{
			"after an emulate -c",
			"emulate sh -c ':'\nset -f\n" + read,
			"glob=on rcs=off\n",
		},
		{
			"after a function that localized one",
			"f() { emulate -L sh }\nf\nset -f\n" + read,
			"glob=on rcs=off\n",
		},
		{
			"and inside that function the letters really are borrowed",
			"f() { emulate -L sh; set -f; " + read + " }\nf",
			"glob=off rcs=on\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if st != 0 || out != tc.want {
				t.Errorf("out %q status %d, want %q", out, st, tc.want)
			}
		})
	}
}
