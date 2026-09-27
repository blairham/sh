// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// This shell's `readonly` is the declaration builtin under a second name —
// interp.Semantics.ReadonlyWord — so every attribute letter `typeset` takes
// is one this word takes too, and reaches the same attribute (#4853).
//
// Measured 2026-09-27 a letter at a time, one run per cell, from a script
// file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME,
// against `/opt/homebrew/bin/zsh`, `zsh 5.9.2 (aarch64-apple-darwin25.4.0)`.
// Before it, thirteen of these rows were `readonly: bad option`.
//
// The table is deliberately not only the letters that *work*: the last four
// rows are letters this shell refuses, and they are what makes the rest
// evidence. A `readonly` that took every byte after a dash would pass a table
// of takes alone.
func TestReadonlyTakesTheDeclarationsLetters(t *testing.T) {
	for _, tc := range []struct{ letter, want string }{
		{"a", "array-readonly"},
		{"A", "association-readonly"},
		{"i", "integer-readonly"},
		{"F", "float-readonly"},
		{"E", "float-readonly"},
		{"x", "scalar-readonly-export"},
		{"U", "scalar-readonly-unique"},
		{"l", "scalar-lower-readonly"},
		{"u", "scalar-upper-readonly"},
		{"H", "scalar-readonly-hideval"},
		{"Z", "scalar-right_zeros-readonly"},
		{"h", "scalar-readonly-hide"},
		{"g", "scalar-readonly"},
		{"L", "scalar-left-readonly"},
		{"R", "scalar-right_blanks-readonly"},
		// The letters that shell refuses under this word, each for its own
		// reason: `r` is the letter the word already *is*, `m` and `z` are
		// `typeset`'s alone, and `k` is in neither set here.
		{"r", "bad option"},
		{"m", "bad option"},
		{"z", "bad option"},
		{"k", "bad option"},
	} {
		t.Run(tc.letter, func(t *testing.T) {
			out, _ := answersRun(t, "readonly -"+tc.letter+` vv 2>&1; print "t=${(t)vv}"`)
			if !strings.Contains(out, tc.want) {
				t.Errorf("`readonly -%s vv` then ${(t)vv} = %q, want %q in it",
					tc.letter, out, tc.want)
			}
		})
	}
}

// The `T` letter reads its operands as a scalar and an array rather than as a
// list of names, which is the one accepted letter above whose evidence is a
// *shape* rather than an attribute word. It rides the same route and is here
// so that the table's `-T` row is not the only thing standing for it.
func TestReadonlyTakesTheTieLetter(t *testing.T) {
	out, st := answersRun(t, `readonly -T TT=x:y tt; print "${#tt} [${tt[1]}] [$TT]"`)
	want := "2 [x] [x:y]\n"
	if out != want || st != 0 {
		t.Errorf("`readonly -T TT=x:y tt` = %q status %d, want %q", out, st, want)
	}
}

// And the operand order the tie makes visible: the word is the declaration,
// so its operands land in the declaration's order under every scope — not
// only inside a function, which is the only place the *scope* rule reaches.
//
// The two spellings on one line are the test: either order is defensible on
// its own and only their disagreement is a defect. With the freeze's order,
// `readonly` stored the array and then tied the name, which empties it, while
// `typeset -r` kept both elements.
func TestReadonlyAndTypesetTieAnArrayLiteralAlike(t *testing.T) {
	out, st := answersRun(t, `readonly -T AA aa=(a b); print "ro ${#aa} [$AA]"
typeset -rT BB bb=(a b); print "ts ${#bb} [$BB]"`)
	want := "ro 2 [a:b]\nts 2 [a:b]\n"
	if out != want || st != 0 {
		t.Errorf("the two spellings = %q status %d, want %q", out, st, want)
	}
}

// And the operands go through the declaration's loop, so the gate a kind
// letter over one of the shell's own parameters meets is this word's too.
//
// The `typeset` row beside it is the control: it is what says the gate itself
// is right and that only the route was missing it. Before #4853 the
// `readonly` half was silent at 0 while the `typeset` half refused and
// stopped the script.
func TestReadonlyMeetsTheKindGateTypesetMeets(t *testing.T) {
	for _, word := range []string{"readonly", "typeset"} {
		t.Run(word, func(t *testing.T) {
			out, st := answersRun(t, word+` -A path; print reached`)
			if !strings.Contains(out, "can't change type of a special parameter") {
				t.Errorf("`%s -A path` wrote %q, want the type refusal", word, out)
			}
			if strings.Contains(out, "reached") || st == 0 {
				t.Errorf("`%s -A path` = %q status %d, want the script ended at 1",
					word, out, st)
			}
		})
	}
}

// The preset's own answer, pinned where the rest of this dialect's are.
func TestReadonlyWordIsTheDeclaration(t *testing.T) {
	s := zsh.Semantics()
	if got, want := s.ReadonlyWord, interp.ReadonlyWordIsTheDeclaration; got != want {
		t.Errorf("ReadonlyWord = %v, want %v", got, want)
	}
	// And the letter set is DeclareOptions less the three the word has spent
	// or has not got, which is measured rather than derived — so it is
	// written out here and compared, not computed from the other field.
	if got, want := s.ReadonlyOptions, "aAEfFgHhiLlpRtuUTxZ"; got != want {
		t.Errorf("ReadonlyOptions = %q, want %q", got, want)
	}
}
