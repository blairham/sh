// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// What this shell's own stored parameters say about themselves.
//
// `${(t)UID}` was a bare `scalar` here where the reference says
// `integer-special`, and so was every other name the shell stores for itself
// rather than producing — the seam, not the four names (#4488). Measured
// 2026-09-26 on zsh 5.9.2 under `-f`; see dialect/zsh/shellownparameters.go
// for the whole table and for the reference's own version.
func TestTheShellsOwnStoredParametersDescribeThemselves(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the real uid", `print -r -- ${(t)UID}`, "integer-special\n"},
		{"the effective uid", `print -r -- ${(t)EUID}`, "integer-special\n"},
		{"the real gid", `print -r -- ${(t)GID}`, "integer-special\n"},
		{"the effective gid", `print -r -- ${(t)EGID}`, "integer-special\n"},
		{"the field separator", `print -r -- ${(t)IFS}`, "scalar-special\n"},
		{"a tie's scalar half", `export PATH; print -r -- ${(t)PATH}`, "scalar-tied-export-special\n"},
		{"and its array half", `print -r -- ${(t)path}`, "array-tied-special\n"},
		{"an unexported tie", `print -r -- ${(t)FPATH} ${(t)fpath}`, "scalar-tied-special array-tied-special\n"},
		{"the last of the eight pairs", `print -r -- ${(t)FIGNORE} ${(t)fignore}`, "scalar-tied-special array-tied-special\n"},
		// The table `${(t)}` reads is the one `$parameters` renders, so the
		// two spellings of the question cannot answer it differently.
		{"and the table agrees", `print -r -- $parameters[UID] $parameters[IFS]`, "integer-special scalar-special\n"},
		// The integer letter rides on a *declaration* rather than on the
		// attribute table, so a listing writes it and an assignment is
		// untouched — which is the half #4476 measured and left alone.
		{
			// The letter and the base, without the *number*, which is the
			// machine's: a row spelling `501` passes on the laptop it was
			// written on and fails on a runner. The value is checked in the
			// same row, against `$UID` rather than against a constant.
			"the listing carries the letter and the base",
			`x=$(typeset -p UID); print -r -- "${x%=*}"; [[ ${x##*=} = $UID ]] && print -r -- "value ok"`,
			"typeset -i10 UID\nvalue ok\n",
		},
		{"and the separator's does not", `typeset -p IFS`, "typeset IFS=$' \\t\\n\\C-@'\n"},
		// An assignment of the id it already is is stored; one of another id
		// is refused, which identityassign_test.go grades (#5142).
		{"an assignment of the same id is stored", `x=$EGID; EGID=$x; [[ $EGID = $x ]] && print -r -- same`, "same\n"},
		// The controls. A produced parameter was right throughout — that is
		// what said the gap was the stored names' — and the one name this
		// hook stores that the reference does *not* call special is the row
		// that says the hook may not do the marking itself.
		{"a produced parameter was already right", `print -r -- ${(t)RANDOM}`, "integer-special\n"},
		{"a stored name the reference calls ordinary", `print -r -- ${(t)HOST}`, "scalar\n"},
		{"an ordinary exported name", `export ord=1; print -r -- ${(t)ord}`, "scalar-export\n"},
		// And the mark is a parameter's rather than a record's: `unset`
		// takes the name away and takes the word with it.
		{"unset takes the mark too", `unset UID; print -r -- "[${(t)UID}][${+UID}]"`, "[][0]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// A slot the shell holds keeps its **kind**, so a value of the wrong shape
// does not retype it: an array slot makes the value its one element and a
// slot that is not a container refuses an array literal outright (#4879).
//
// The same rule a `private` declaration's slot follows — see
// interp/privatekindfixed.go, which holds both — and the sentence parts by
// the *word*: a declaration utility names itself and says `special`, a bare
// assignment writes the ordinary one.
//
// Measured 2026-09-27 against zsh 5.9.2 from a script file under `env -i
// PATH=/usr/bin:/bin` with a scratch `HOME`.
func TestAShellHeldSlotKeepsItsKind(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an array slot takes the value as one element", `path=plain; print "${(t)path} n=${#path} [$path]"`, "array-tied-special n=1 [plain]\n"},
		{"and so do the other two", `fpath=plain; fignore=plain; print "${(t)fpath} ${#fpath} ${(t)fignore} ${#fignore}"`, "array-tied-special 1 array-tied-special 1\n"},
		// The controls on that half: an ordinary array *is* retyped by the
		// identical line, and the slot still takes a real literal.
		{"an ordinary array is retyped", `typeset -a q; q=plain; print "${(t)q} n=${#q}"`, "scalar n=5\n"},
		{"and the slot takes a literal", `path=(a b); print "${(t)path} n=${#path}"`, "array-tied-special n=2\n"},
		{"a slot that is not a container takes the value", `IFS=plain; print "${(t)IFS} [$IFS]"`, "scalar-special [plain]\n"},
		{"and an integer slot reads it as one", `SECONDS=plain; print "${(t)SECONDS} [$SECONDS]"`, "integer-special [0]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
	// An array literal over a slot that is not a container is refused, and
	// the two sentences are the pair that says the word in front decides.
	for _, name := range []string{"HOME", "IFS", "RANDOM", "SECONDS"} {
		out, st := runZsh(t, t.TempDir(), name+`=(p q); print reached`)
		if want := name + ": attempt to assign array value to non-array"; !strings.Contains(out, want) {
			t.Errorf("%s=(p q) = %q, want %q", name, out, want)
		}
		if strings.Contains(out, "reached") || st != 1 {
			t.Errorf("%s=(p q) = %q (status %d), want the script to end at 1", name, out, st)
		}
		out, st = runZsh(t, t.TempDir(), `typeset `+name+`=(p q); print reached`)
		if want := name + ": can't assign array value to non-array special"; !strings.Contains(out, want) || !strings.Contains(out, "typeset:") {
			t.Errorf("typeset %s=(p q) = %q, want %q", name, out, want)
		}
		if strings.Contains(out, "reached") || st != 1 {
			t.Errorf("typeset %s=(p q) = %q (status %d), want the script to end at 1", name, out, st)
		}
	}
	// And the letter is refused ahead of the value where the slot *would*
	// take the literal, which is the ordering interp/privatekindfixed.go
	// records: an array slot takes `(1 2)`, so only `-i` is left to refuse.
	out, st := runZsh(t, t.TempDir(), `typeset -i path=(1 2); print reached`)
	if want := "path: can't change type of a special parameter"; !strings.Contains(out, want) || !strings.Contains(out, "typeset:") {
		t.Errorf("typeset -i path=(1 2) = %q, want %q", out, want)
	}
	if strings.Contains(out, "reached") || st != 1 {
		t.Errorf("typeset -i path=(1 2) = %q (status %d), want the script to end at 1", out, st)
	}
}
