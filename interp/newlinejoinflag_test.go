// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// The `(F)` flag: a join whose separator is a newline.
//
// The vendor manual states it as shorthand for `pj:\n:`, and it is carried as
// exactly that — one slot for the separator, filled by whichever of `j` and
// `F` was written last — rather than as a second join with rules of its own.
// So the rows below are as much about *which* join ran as about the
// characters that came back: every one of them has a `j` twin that answers
// the same shape with a different separator.
//
// Measured on zsh 5.9.2, 2026-09-25, the one grammar in the panel with the
// construct. #4452.
func TestTheNewlineJoinFlagJoinsOnNewlines(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The two shapes the issue names. The positional one is the shape a
		// script actually walks into and goes through a different base than
		// the array does, so neither stands in for the other.
		{"an array", `a=(x y z); printf "[%s]" "${(F)a}"`, "[x\ny\nz]"},
		{"the positionals", `set -- a b c; printf "[%s]" "${(F)@}"`, "[a\nb\nc]"},

		// Unquoted it joins too, which is what says it is `j`'s join and not
		// something that only happens under quotes. Without it these are
		// three fields and two brackets more.
		{"an array unquoted", `a=(x y z); printf "[%s]" ${(F)a}`, "[x\ny\nz]"},
		{"the positionals unquoted", `set -- a b c; printf "[%s]" ${(F)@}`, "[a\nb\nc]"},
		{"where without the flag the fields stand", `a=(x y z); printf "[%s]" "${a[@]}"`, "[x][y][z]"},

		// The `@` letter keeps one field per element and the join overrides
		// it, exactly as it overrides it for `j`.
		{"the @ letter does not exempt it", `a=(x y z); printf "[%s]" "${(@F)a}"`, "[x\ny\nz]"},
		{"where @ alone keeps the fields", `a=(x y z); printf "[%s]" "${(@)a}"`, "[x][y][z]"},
		{"and @ with a named separator joins as well", `a=(x y z); printf "[%s]" "${(@j:-:)a}"`, "[x-y-z]"},

		// The edges.
		{"a hole is kept", `a=(x "" z); printf "[%s]" "${(F)a}"`, "[x\n\nz]"},
		{"one element is not joined to anything", `a=(x); printf "[%s]" "${(F)a}"`, "[x]"},
		{"an empty array is empty", `a=(); printf "[%s]" "${(F)a}"`, "[]"},
		{"a scalar is unchanged", `v=hello; printf "[%s]" "${(F)v}"`, "[hello]"},
		{"an unset name is empty", `unset u; printf "[%s]" "${(F)u}"`, "[]"},
		{"and the default a substitution supplies is reached", `a=(); printf "[%s]" "${(F)a:-empty}"`, "[empty]"},

		// `$IFS` is what a group with *no* join letter falls back to, so a
		// row that changes it separates "this flag named a separator" from
		// "the join happened to find a newline".
		{"IFS does not reach it", `a=(x y z); IFS=-; printf "[%s]" "${(F)a}"`, "[x\ny\nz]"},
		{"where with no join letter IFS is the separator", `a=(x y z); IFS=-; printf "[%s]" "${a[*]}"`, "[x-y-z]"},

		// The inverse flag composes with it rather than racing it: the join
		// runs at rule 10 and the split at rule 11, so a value that already
		// held a newline comes apart along with the ones the join inserted.
		{"a newline split undoes it", `a=("a${nl}b" c); printf "[%s]" "${(Ff)a}"`, "[a][b][c]"},
		{"written the other way round, the same", `a=("a${nl}b" c); printf "[%s]" "${(fF)a}"`, "[a][b][c]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := flagsRun(t, newlineFixture+tc.src)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%q: out=%q errs=%q st=%d, want %q clean", tc.src, out, errs, st, tc.want)
			}
		})
	}
}

// newlineFixture gives the rows a newline they can write inside a Go string
// without depending on how this grammar spells one.
const newlineFixture = "nl='\n'; "

// `j` and `F` fill one separator slot, and the last one written wins.
//
// This is the discriminating grid for the whole implementation: a `(F)` read
// as a join of its own would have to decide which of the two ran, and every
// order it could pick is wrong in one of these four rows. Measured on zsh
// 5.9.2, 2026-09-25.
func TestTheLastJoinLetterWrittenNamesTheSeparator(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a j behind an F takes the newline away", `a=(x y z); printf "[%s]" "${(Fj:-:)a}"`, "[x-y-z]"},
		{"an F behind a j puts it back", `a=(x y z); printf "[%s]" "${(j:-:F)a}"`, "[x\ny\nz]"},
		{"and the last of three still decides", `a=(x y z); printf "[%s]" "${(Fj:-:F)a}"`, "[x\ny\nz]"},
		{"whichever letter it is", `a=(x y z); printf "[%s]" "${(j:-:Fj:+:)a}"`, "[x+y+z]"},
		{"a repeated F is one F", `a=(x y z); printf "[%s]" "${(FF)a}"`, "[x\ny\nz]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := flagsRun(t, tc.src)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%q: out=%q errs=%q st=%d, want %q clean", tc.src, out, errs, st, tc.want)
			}
		})
	}
}

// The character count under `${#…}` reads the same separator slot.
//
// `${(cF)#a}` alone proves nothing — a newline is one character, exactly like
// the space `(c)` counts without any join letter at all, so both readings
// answer 8. The pair that discriminates holds the flags fixed and moves only
// which letter was written last. Measured on zsh 5.9.2, 2026-09-25, with
// `a=(abc de f)`: six characters of word and two separators.
func TestTheCharacterCountReadsTheNewlineJoinSeparator(t *testing.T) {
	const a = `a=(abc de f); `
	for _, tc := range []struct{ name, src, want string }{
		{"no join letter counts a space", a + `printf "[%s]" "${(c)#a}"`, "[8]"},
		{"a named separator replaces it", a + `printf "[%s]" "${(cj.--.)#a}"`, "[10]"},
		{"an F behind that j puts the newline back", a + `printf "[%s]" "${(cj.--.F)#a}"`, "[8]"},
		{"and a j behind an F takes it away again", a + `printf "[%s]" "${(cFj.--.)#a}"`, "[10]"},
		{"the flag alone agrees with the space, as a newline must", a + `printf "[%s]" "${(cF)#a}"`, "[8]"},
		{"and a plain length still counts elements", a + `printf "[%s]" "${(F)#a}"`, "[3]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := flagsRun(t, tc.src)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%q: out=%q errs=%q st=%d, want %q clean", tc.src, out, errs, st, tc.want)
			}
		})
	}
}
