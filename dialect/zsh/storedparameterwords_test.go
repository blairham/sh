// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// The word a type query writes for the names this shell stores for itself.
//
// #4488 marked the names `SetSpecial` and `Tie` make; a sweep of all 123 names
// a fresh `zsh -f` lists under `typeset +` found twelve more that this shell
// *has* and described with a word missing, and three of those with the wrong
// kind besides (#4857). See dialect/zsh/shellownparameters.go for the
// measurement and for what the reference answers.
//
// The rows run with the prelude installed, because that is where four of these
// names get their value and a snippet run without it would be asking about a
// shell nobody runs.
func runZshOwnParameters(t *testing.T, src string) (string, int) {
	t.Helper()
	dir := t.TempDir()
	out, st, err := preset.CombinedWithPrelude(t, dialecttest.Base{
		Dir: dir,
		// A scratch home rather than the machine's, and handed over rather
		// than assigned, since the export attribute the first row is about
		// is the one an inherited name carries.
		Vars: map[string]string{"PATH": dir},
		Env:  []string{"HOME=" + dir, "PATH=" + dir},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

func TestTheStoredParametersThisShellOwnsSaySo(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the home directory", `print -r -- ${(t)HOME}`, "scalar-export-special\n"},
		{"the history size", `print -r -- ${(t)HISTSIZE}`, "integer-special\n"},
		{"the null command", `print -r -- ${(t)NULLCMD}`, "scalar-special\n"},
		{"and the reading one", `print -r -- ${(t)READNULLCMD}`, "scalar-special\n"},
		{"the word characters", `print -r -- ${(t)WORDCHARS}`, "scalar-special\n"},
		{"the three prompts this shell stores", `print -r -- ${(t)PS1} ${(t)PS2} ${(t)PS4}`, "scalar-special scalar-special scalar-special\n"},
		{"the history characters", `print -r -- ${(t)histchars}`, "scalar-special\n"},
		{"the working directory", `print -r -- ${(t)PWD}`, "scalar-export\n"},
		{"the option index", `print -r -- ${(t)OPTIND}`, "integer-special\n"},
		{"the parent's process id", `print -r -- ${(t)PPID}`, "integer-readonly-special\n"},
		{"the shell's depth", `print -r -- ${(t)SHLVL}`, "integer-export-special\n"},

		// The table `${(t)}` reads is the one `$parameters` renders, so the
		// two spellings of the question cannot answer it differently.
		{"and the table agrees", `print -r -- $parameters[PPID] $parameters[HOME]`, "integer-readonly-special scalar-export-special\n"},

		// The controls, and they are what make the rows above a finding
		// rather than "this shell never says special". Every one of these
		// agreed with the reference in the same sweep, in the same run.
		{"a produced parameter was already right", `print -r -- ${(t)RANDOM} ${(t)SECONDS}`, "integer-special integer-special\n"},
		{"the sharp pair: the other produced integer", `print -r -- ${(t)LINENO}`, "integer-readonly-special\n"},
		{"a tie's two halves", `print -r -- ${(t)path} ${(t)PATH}`, "array-tied-special scalar-tied-export-special\n"},
		{"the field separator", `print -r -- ${(t)IFS}`, "scalar-special\n"},
		// The row that says the mark may not be moved into the hook that
		// stores the value: `$HOST` goes through the same one and is an
		// ordinary scalar in the reference.
		{"a stored name the reference calls ordinary", `print -r -- ${(t)HOST}`, "scalar\n"},
		{"an ordinary variable a script makes", `ord=1; print -r -- ${(t)ord}`, "scalar\n"},
		// The upper-case halves of the two pairs whose stores are in the
		// list above. They are produced over those stores and have been
		// answering `special` all along, which is the asymmetry the sweep
		// found.
		{"the produced half of each pair", `print -r -- ${(t)PROMPT} ${(t)HISTCHARS}`, "scalar-special scalar-special\n"},

		// The integer letter on these three is the attribute table and not a
		// listing's spelling, which is what an assignment consults.
		{"an integer name evaluates what is assigned to it", `SHLVL=1+2; print -r -- $SHLVL`, "3\n"},
		{"and a word that is no number at all is zero", `SHLVL=abc; print -r -- $SHLVL`, "0\n"},
		{"the option index too", `OPTIND=3+4; print -r -- $OPTIND`, "7\n"},
		{"and the listing writes the letter and the base", `typeset -p OPTIND`, "typeset -i10 OPTIND=1\n"},

		// And the mark is a parameter's rather than a record's: `unset`
		// takes the name away and takes the word with it.
		{"unset takes the mark too", `unset NULLCMD; print -r -- "[${(t)NULLCMD}][${+NULLCMD}]"`, "[][0]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshOwnParameters(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The freeze on `$PPID` is the half of #4857 that refuses something, and it is
// on that name alone: `getopts` writes `$OPTIND` and a nested shell increments
// `$SHLVL`, so freezing either would refuse a write this shell itself makes.
func TestTheParentProcessIdIsFrozenAndItsNeighborsAreNot(t *testing.T) {
	out, st := runZshOwnParameters(t, `PPID=7; print -r -- "rc=$?"`)
	if st == 0 {
		t.Errorf("PPID=7 = %q at status %d, want a refusal", out, st)
	}
	wantWholeLines(t, out, "zsh:1: read-only variable: PPID")

	for _, src := range []string{
		`OPTIND=5; print -r -- $OPTIND`,
		`SHLVL=5; print -r -- $SHLVL`,
	} {
		out, st := runZshOwnParameters(t, src)
		if out != "5\n" || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", src, out, st, "5\n")
		}
	}
}
