// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// Which words a shell classes as *reserved* is the dialect's table and not
// one grammar's (#3291).
//
// zsh's `$reswords` holds the seven declaration commands — `declare`,
// `export`, `float`, `integer`, `local`, `readonly`, `typeset` — and four
// words the rest of the panel has no construct for, and it does *not* hold
// `in` or `]]`, which the POSIX grammar's table does. So one name is a
// reserved word in one column and a builtin in four, with no shell able to
// see the disagreement from inside its own tier.
//
// Measured 2026-09-16, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> case.sh`,
// stdin from /dev/null, in a fresh directory:
//
//	type export           type if            type echo
//	bash 5.3.20      shell builtin      shell keyword      shell builtin
//	that as `sh`     SPECIAL builtin    shell keyword      shell builtin
//	bash 3.2.57      shell builtin      shell keyword      shell builtin
//	zsh 5.9.2        RESERVED WORD      reserved word      shell builtin
//	ksh93u+          SPECIAL builtin    keyword            shell builtin
//	dash 0.5.12      SPECIAL builtin    shell keyword      shell builtin
//	BusyBox ash      SPECIAL builtin    shell keyword      shell builtin
//
// `if` is the control on the *wording*: every column has a sentence for a
// reserved word and they differ only in how it is spelled, so a shell that
// could not say the phrase at all fails there rather than here. `echo` is
// the control on the *class*: unanimous, so a shell that had started calling
// every name reserved fails it in all six presets.
//
// # Why this is a Go row
//
// `zsh/` can pin zsh's side and `bash/` can pin bash's, and with both green
// the split is unpinned: the table this reads from is per dialect, so a
// preset handed the wrong one leaves every tier at 100% and only the column
// that moved is wrong. Here the six presets sit together with the controls
// beside the discriminator.
func TestReservedWordClassIsPerDialect(t *testing.T) {
	for _, c := range []struct {
		preset string
		// declaration is what this preset writes for `export`, whole.
		declaration string
		// keyword is what it writes for `if`, which is the wording control.
		keyword string
	}{
		{"bash", "export is a shell builtin", "if is a shell keyword"},
		{"zsh", "export is a reserved word", "if is a reserved word"},
		{"ksh", "export is a special shell builtin", "if is a keyword"},
		{"dash", "export is a special shell builtin", "if is a shell keyword"},
		{"ash", "export is a special shell builtin", "if is a shell keyword"},
		{"posix", "export is a special shell builtin", "if is a shell keyword"},
	} {
		t.Run(c.preset, func(t *testing.T) {
			p := presets[c.preset]
			out, st, err := p.Combined(t, dialecttest.Base{}, "type export; type if; type echo")
			if err != nil {
				t.Fatal(err)
			}
			want := c.declaration + "\n" + c.keyword + "\necho is a shell builtin\n"
			if out != want || st != 0 {
				t.Errorf("type said %q status %d, want %q at 0", out, st, want)
			}
		})
	}
}

// TestZshDoesNotReserveWhatThePosixGrammarDoes is the half a table read as a
// *union* would get wrong, and the reason the dialect's list replaces the
// grammar's rather than adding to it.
//
// `in` is a word the POSIX grammar reserves and zsh's `$reswords` does not:
// measured 2026-09-16 on zsh 5.9.2 under `-f`, `whence -w in` is `none` at 1
// where `whence -w '[['` is `reserved`. `command -v` is the same fact a
// script can act on — a reserved word is a name the shell would run, so it
// answers with the word, and a name reserved nowhere, with no builtin and not
// on PATH, fails.
//
// `[[` is the control, in the three columns whose grammar has it: zsh's table
// holds that one, so a shell that had simply stopped reserving anything fails
// there rather than passing this test.
//
// The POSIX preset is not in the table. It refuses
// `CommandNotFoundStatusIsNotFound` — the panel disagrees about the status a
// missing name answers with — so the not-found half of the question cannot be
// asked there at all, and a row written over it would be pinning that refusal.
func TestZshDoesNotReserveWhatThePosixGrammarDoes(t *testing.T) {
	for _, c := range []struct {
		preset string
		// found is whether `command -v in` answers with the word.
		found bool
	}{
		{"bash", true},
		{"ksh", true},
		{"dash", true},
		{"ash", true},
		{"zsh", false},
	} {
		t.Run("in/"+c.preset, func(t *testing.T) {
			out, st, err := presets[c.preset].Combined(t, dialecttest.Base{}, "command -v -- in")
			if err != nil {
				t.Fatal(err)
			}
			if c.found {
				if out != "in\n" || st != 0 {
					t.Errorf("command -v in in %s = %q (status %d), want `in` at 0", c.preset, out, st)
				}
				return
			}
			if out != "" || st == 0 {
				t.Errorf("command -v in in %s = %q (status %d), want nothing and a failure", c.preset, out, st)
			}
		})
	}
	for _, preset := range []string{"bash", "ksh", "zsh"} {
		t.Run("[[/"+preset, func(t *testing.T) {
			out, st, err := presets[preset].Combined(t, dialecttest.Base{}, "command -v -- '[['")
			if err != nil {
				t.Fatal(err)
			}
			if out != "[[\n" || st != 0 {
				t.Errorf("command -v [[ in %s = %q (status %d), want `[[` at 0", preset, out, st)
			}
		})
	}
}
