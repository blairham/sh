// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// The width attributes — `typeset -L`, `-R` and `-Z` — #1461.
//
// Every want below was measured on zsh 5.9.2 on 2026-09-12, run under `env -i`
// with a scratch HOME, ZDOTDIR and HISTFILE, from a script file; the rule and
// the wider measurement table are in interp/fieldwidth.go.
func TestTheWidthAttributePresentsTheValue(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`typeset -L 5 a=ab; print -r -- "[$a]"`, "[ab   ]\n"},
		{`typeset -R 5 a=ab; print -r -- "[$a]"`, "[   ab]\n"},
		{`typeset -Z 4 a=7; print -r -- "[$a]"`, "[0007]\n"},
		// `-Z` fills with zeros only where the value starts with a digit, so
		// a word and a sign are blank-filled. The first character decides it
		// and not whether the whole value is a number.
		{`typeset -Z 4 a=abc; print -r -- "[$a]"`, "[ abc]\n"},
		{`typeset -Z 5 a=-7; print -r -- "[$a]"`, "[   -7]\n"},
		{`typeset -Z 5 a=1.5; print -r -- "[$a]"`, "[001.5]\n"},
		// Truncation goes from the far side of the one that is kept, which
		// is the same rule as the padding.
		{`typeset -L 5 a=abcdefgh; print -r -- "[$a]"`, "[abcde]\n"},
		{`typeset -R 3 a=abcdefgh; print -r -- "[$a]"`, "[fgh]\n"},
		// Leading blanks come off before the pad, which is not the same as
		// trimming the result: padding `  x` as written would give `  x`.
		{`typeset -L 3 a="  x"; print -r -- "[$a]"`, "[x  ]\n"},
		// The attribute outlives the line that declared it.
		{`typeset -L 4 a; a=xy; print -r -- "[$a]"`, "[xy  ]\n"},
		// It is what the name *holds*, so its length agrees with it.
		{`typeset -L 5 a=ab; print -r -- "[${#a}]"`, "[5]\n"},
		// The attached spelling names the same width as the detached one.
		{`typeset -L5 a=ab; print -r -- "[$a]"`, "[ab   ]\n"},
		// The case fold runs first and the width is applied to what it left.
		{`typeset -lL 4 a=ABCD; print -r -- "[$a]"`, "[abcd]\n"},
		// The earlier of the three letters in an option word wins.
		{`typeset -RZ 5 a=7; print -r -- "[$a]"`, "[    7]\n"},
		{`typeset -LZ 5 a=7; print -r -- "[$a]"`, "[7    ]\n"},
		// `+L` reveals the text the name was holding all along, which is what
		// says this shell never stored the padded form.
		{`typeset -L 4 a=ab; typeset +L a; print -r -- "[$a]"`, "[ab]\n"},
		// And an append joins the raw text rather than the padding.
		{`typeset -L 4 a=ab; a+=zz; print -r -- "[$a]"`, "[abzz]\n"},
		// A local takes the letter too, and it is a local.
		{`f(){ typeset -L 5 a=ab; print -r -- "[$a]"; }; f; print -r -- "[$a]"`, "[ab   ]\n[]\n"},
		{`f(){ local -R 4 a=ab; print -r -- "[$a]"; }; f`, "[  ab]\n"},
		// And `export`, which is this word's declaration under another name
		// and takes the same letters bar six — these three are not among the
		// six. Its listing carries the letter, the width and the export at
		// once, so it is the row that says both halves landed.
		{`export -L 5 a=ab; print -r -- "[$a]"; typeset -p a`, "[ab   ]\nexport -L5 a=ab\n"},
		{`export -Z 4 c=7; typeset -p c`, "export -Z4 c=7\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The width is learned from the first value and becomes part of the
// declaration, so the listing writes it back whether or not it was written
// down.
//
// This is the half a shell that padded at print time would get wrong: it
// answers the listing right and every later read wrong, once the name holds
// something shorter than the value that taught it.
func TestTheWidthIsLearnedAndListsItselfBack(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`typeset -L f=xy; typeset -p f`, "typeset -L2 f=xy\n"},
		{`typeset -Z h=7; typeset -p h`, "typeset -Z1 h=7\n"},
		// A written zero is the same as none rather than a width of nothing.
		{`typeset -L 0 i=abcd; typeset -p i`, "typeset -L4 i=abcd\n"},
		// And a letter with a number and no value keeps the number.
		{`typeset -L 3 j; typeset -p j`, "typeset -L3 j=''\n"},
		// A learned width is a width: the name holds three characters after
		// it, not four.
		{`typeset -L f=xy; f=abcd; print -r -- "[$f]"`, "[ab]\n"},
		// The listing writes the *raw* text and not the padded one, because
		// this shell pads on the read.
		{`typeset -L 5 a=ab; typeset -p a`, "typeset -L5 a=ab\n"},
		// A number written down replaces one the name had learned.
		{`typeset -L 3 f=abcd; typeset -R 4 f; typeset -p f`, "typeset -R4 f=abcd\n"},
		// `+L` loses the letter and the width with it.
		{`typeset -L 4 e=ab; typeset +L e; typeset -p e`, "typeset e=ab\n"},
		// The earlier letter is the one recorded, and the width goes with it.
		{`typeset -RZ 5 l=7; typeset -p l`, "typeset -R5 l=7\n"},
		// `-i` swallows the number as a *base*, so the width letter learns
		// nothing and writes nothing.
		{`typeset -iL 3 k=12345; typeset -p k`, "typeset -i3 k=12345\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// A number-taking letter ends its option word in the listing, and this was
// already wrong for the integer letter before the width letters arrived:
// `typeset -i16r v=255` re-reads as the base `16r`, so the listing was not
// the declaration it claimed to be. One rule serves both letters.
func TestANumberedLetterEndsItsWordInTheListing(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`typeset -ri 16 v=255; typeset -p v`, "typeset -i16 -r v=255\n"},
		{`typeset -rL 3 a=abcd; typeset -p a`, "typeset -L3 -r a=abcd\n"},
		{`typeset -lL 4 d=ABCD; typeset -p d`, "typeset -L4 -l d=ABCD\n"},
		// The container letters come *before* the numbered one, so nothing
		// is broken off in front of it.
		{`typeset -aL 3 c; typeset -p c`, "typeset -aL3 c=(  )\n"},
		// The control: no number, no break.
		{`typeset -ri v=255; typeset -p v`, "typeset -ir v=255\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// A child is told the *raw* text, where the case letters do fold into the
// environment. The padding is a presentation of the parameter and the fold is
// a property of the value — this tree's environment loop carried a comment
// saying "the environment is a read like any other", which is false for this
// half and is corrected where the code is.
//
// The reader is a real child process rather than a `typeset -p`, because a
// listing reads the *parameter* and the whole question here is what crosses
// the boundary. Measured 2026-09-12 on zsh 5.9.2 through `env`: `a=AB`,
// `b=xy`, `c=7` — the case fold went and the two paddings did not.
func TestAChildIsToldTheRawTextButTheFoldedCase(t *testing.T) {
	dir := t.TempDir()
	show := filepath.Join(dir, "show")
	body := "#!/bin/sh\nprintf '%s\\n' \"[$a][$b][$c]\"\n"
	if err := os.WriteFile(show, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	src := `typeset -u a=ab; typeset -L 5 b=xy; typeset -Z 4 c=7
print -r -- "here[$a][$b][$c]"
export a b c
show`
	// The first line is the control: inside the shell all three attributes
	// act, so the child's line below is the boundary doing it and not the
	// attributes never having been applied.
	want := "here[AB][xy   ][0007]\n[AB][xy][7]\n"
	out, st := runZsh(t, dir, src)
	if out != want || st != 0 {
		t.Errorf("= %q (status %d), want %q", out, st, want)
	}
}

// An array literal over a name carrying one of these attributes re-creates the
// name, so the attribute goes — the same thing that was already measured for
// `-i`, `-l` and `-u`, and it has to go from the *store* and not only from the
// listing.
//
// The guard on asking that question named only those three letters, so the
// width attribute and the float precision survived a `c=(a bb)` that zsh drops
// them on. The `-aL` row is the control: declared an array with the letter, it
// keeps it, which is what says this is about the re-creation and not about
// arrays refusing the letter.
func TestAnArrayLiteralDropsTheWidthAttribute(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`typeset -L 5 s; s=(a bb); typeset -p s`, "typeset -a s=( a bb )\n"},
		{`typeset -L 5 t=x; t=(a bb); typeset -p t`, "typeset -a t=( a bb )\n"},
		{`typeset -F 3 b; b=(1 2); typeset -p b`, "typeset -a b=( 1 2 )\n"},
		// Gone from the store: a scalar assigned afterwards is not padded.
		{`typeset -L 3 d; d=(a bb); d=zz; print -r -- "[$d]"`, "[zz]\n"},
		// The control — declared an array with the letter, it stays, and the
		// elements are not padded either.
		{`typeset -aL 3 c=(ab cdefg); typeset -p c`, "typeset -aL3 c=( ab cdefg )\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// `Z` is exclusive with `R` here and a **combination** with `L`, where ksh93
// reads it as a fill riding on whichever justification it has and writes both
// letters back in either case (#2859, #4798).
//
// **Every row in the second group is a one-word spelling with a detached
// number**, and that is the reason they agree rather than a property of `Z`:
// the detached number ends the word and the rest of it is discarded, so the
// second letter is never read and every reading of `Z` produces these bytes.
// The `0012` row was chosen *as* the discriminator and is not one. They are
// kept because they are correct and because they are the control on #4766 and
// on the separate-word rows above them: an answer about two letters must not
// reach a spelling where only one was read.
//
// Measured 2026-09-27 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f` from a
// script file under `env -i PATH=/usr/bin:/bin`; `go version -m` says *not a
// Go executable* for it and `github.com/blairham/sh/cmd/zsh` for ours.
func TestTheZeroFillLetterCombinesWithTheLeftJustification(t *testing.T) {
	if got := zsh.Semantics().DeclareZeroFillLetter; got != interp.DeclareZeroFillLetterCombinesWithTheLeftJustification {
		t.Errorf("DeclareZeroFillLetter = %v, want a combination with the left "+
			"justification", got)
	}
	if got := zsh.Semantics().WidthJustificationPrecedence; got != interp.WidthJustificationConflictLeavesNoWidth {
		t.Errorf("WidthJustificationPrecedence = %v, want no width where the letters conflict", got)
	}
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		// Two option words, where both letters are really read: the pair
		// stands, the listing writes both, `${(t)}` names both, and the value
		// is left-justified with its leading zeros off.
		{
			`typeset -L5 -Z5 v=7; typeset -p v; print -r -- "${(t)v} [$v]"`,
			"typeset -L5 -Z5 v=7\nscalar-left-right_zeros [7    ]\n",
		},
		{
			`typeset -Z5 -L5 v=7; typeset -p v; print -r -- "${(t)v} [$v]"`,
			"typeset -L5 -Z5 v=7\nscalar-left-right_zeros [7    ]\n",
		},
		{`typeset -L -Z v=7; typeset -p v`, "typeset -L1 -Z1 v=7\n"},
		{`typeset -L5 -Z5 v=00700; print -r -- "[$v]"`, "[700  ]\n"},
		{`typeset -L5 -Z5 v=ab; print -r -- "[$v]"`, "[ab   ]\n"},
		{`typeset -L5 -Z5 v=0; print -r -- "[$v]"`, "[     ]\n"},
		{`typeset -L5 -Z5 v=-07; print -r -- "[$v]"`, "[-07  ]\n"},
		// The width is the justification's, whichever order and whichever
		// letter carried a number.
		{`typeset -L5 -Z3 v=7; typeset -p v`, "typeset -L5 -Z5 v=7\n"},
		{`typeset -Z3 -L5 v=7; typeset -p v`, "typeset -L5 -Z5 v=7\n"},
		{`typeset -L -Z5 v=7; typeset -p v`, "typeset -L5 -Z5 v=7\n"},
		{`typeset -Z5 -L v=7; typeset -p v`, "typeset -L5 -Z5 v=7\n"},
		{`typeset -Z5 -L3 -Z1 v=7; typeset -p v`, "typeset -L3 -Z3 v=7\n"},
		{`typeset -L5 -Z3 -L1 v=7; typeset -p v`, "typeset -L1 -Z1 v=7\n"},
		{`typeset -Z5 -Z3 v=7; typeset -p v`, "typeset -Z3 v=7\n"},
		// `R` is not part of the combination, in either order: that pair is
		// #4766's and leaves the name with no width at all. These are the
		// control that keeps the conflict rule keyed where it was measured.
		{
			`typeset -R5 -Z5 v=7; typeset -p v; print -r -- "${(t)v} [$v]"`,
			"typeset v=7\nscalar [7]\n",
		},
		{`typeset -Z5 -R5 v=7; typeset -p v`, "typeset v=7\n"},
		{`typeset -L5 -R5 v=7; typeset -p v`, "typeset v=7\n"},
		// And the pair beside other letters, which is what says the two
		// numbered letters each end their own option word.
		{`typeset -i -L5 -Z5 v=7; typeset -p v`, "typeset -iL5 -Z5 v=7\n"},
		{
			`typeset -u -L5 -Z5 v=ab; typeset -p v; print -r -- "${(t)v} [$v]"`,
			"typeset -L5 -Z5 -u v=ab\nscalar-left-right_zeros-upper [AB   ]\n",
		},
		// The one-word spellings with a detached number, where the second
		// letter is never read at all.
		{`typeset -ZL 5 c=7; print -r -- "[$c]"; typeset -p c`, "[00007]\ntypeset -Z5 c=7\n"},
		{`typeset -LZ 5 r=7; print -r -- "[$r]"; typeset -p r`, "[7    ]\ntypeset -L5 r=7\n"},
		{`typeset -ZRL 5 d=7; typeset -p d`, "typeset -Z5 d=7\n"},
		{`typeset -LRZ 5 e=7; typeset -p e`, "typeset -L5 e=7\n"},
		{`typeset -LR 5 t=7; print -r -- "[$t]"`, "[7    ]\n"},
		{`typeset -RL 5 u=7; print -r -- "[$u]"`, "[    7]\n"},
		// The leading zeros a left-hand ride would take off, kept.
		{`typeset -LZ 5 i=0012; print -r -- "[$i]"`, "[0012 ]\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// A width letter may stand beside the integer one here, the first written
// winning, where ksh93 answers the pair with typeset's usage block and ends
// the script. Measured 2026-09-18 (#2859).
func TestAWidthLetterSharesWithTheIntegerLetter(t *testing.T) {
	if got := zsh.Semantics().WidthLettersExcludeTheIntegerLetter; got != interp.No {
		t.Errorf("WidthLettersExcludeTheIntegerLetter = %v, want No", got)
	}
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		// The `5` becomes a *base* where the integer letter wins, which is
		// the half that says both letters were really read.
		{`typeset -iL 5 a=7; typeset -p a`, "typeset -i5 a=7\n"},
		{`typeset -Li 5 b=7; typeset -p b`, "typeset -L5 b=7\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// Two width letters that really are both read leave the name with **no**
// width attribute — #4766, and the row the five controls above could not
// reach.
//
// Measured 2026-09-27 on `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `-f` from a script file under `env -i
// PATH=/usr/bin:/bin`; `go version -m` says *not a Go executable* for it.
//
// The grid varies the two things the rule turns on — whether the `R` letter
// is in the pair, and whether the letters were really read or the second was
// discarded by a detached number. Varying only the letters, which is what the
// suite above does, agrees under every reading.
func TestTwoWidthLettersReallyReadLeaveNoWidth(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"L and R in separate words",
			`typeset -L5 -R5 v=7; typeset -p v; print -r -- "${(t)v}[$v]"`,
			"typeset v=7\nscalar[7]\n",
		},
		{
			"Z and R in separate words",
			`typeset -Z5 -R5 v=7; typeset -p v; print -r -- "${(t)v}[$v]"`,
			"typeset v=7\nscalar[7]\n",
		},
		{
			"R and L, the other order",
			`typeset -R5 -L5 v=7; typeset -p v; print -r -- "${(t)v}[$v]"`,
			"typeset v=7\nscalar[7]\n",
		},
		{
			"and in one word, where no number is detached",
			`typeset -LR5 v=7; typeset -p v`,
			"typeset v=7\n",
		},
		{
			"a third letter does not undo it",
			`typeset -L5 -R5 -L5 v=7; typeset -p v`,
			"typeset v=7\n",
		},
		// The three controls that say it reaches the width and nothing else.
		{
			"the integer letter is untouched",
			`typeset -i -L5 -R5 v=7; typeset -p v; print -r -- "[$v]"`,
			"typeset -i v=7\n[7]\n",
		},
		{
			"and so is a case letter",
			`typeset -u -L5 -R5 v=ab; typeset -p v; print -r -- "[$v]"`,
			// The listing writes the text the name is holding rather than
			// the presentation, which is what CaseAttributeFoldsWhenRead
			// records for this column: `v=ab` in the row and `AB` in the
			// expansion.
			"typeset -u v=ab\n[AB]\n",
		},
		{
			"an attribute the name already had is not removed",
			`typeset -L5 q; typeset -L5 -R5 q; q=7; typeset -p q`,
			"q=''\ntypeset -L5 q=7\n",
		},
		// And the pair that is a combination rather than a conflict: the
		// first letter, which is what this engine records until #4798.
		{
			"L beside Z is not a conflict",
			`typeset -L5 -Z5 v=7; print -r -- "[$v]"`,
			"[7    ]\n",
		},
		// One letter written twice is the last width, which is the row that
		// says this is about the letters differing and not about repetition.
		{
			"one letter twice is the last width",
			`typeset -L5 -L3 v=7; typeset -p v`,
			"typeset -L3 v=7\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A directory of its own per row: every row writes the same
			// name, so a shared one would let an earlier declaration answer
			// a later row.
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The **number** a declaration's width ends up with is the last one written
// here, where ksh93 keeps the first — which is #4827's row read from this
// side and is the reason that question is an axis of its own rather than a
// second reading of the letter rule.
//
// Measured 2026-09-27 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f` from a
// script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`. The zero rows are
// what say the answer is the order and not "a number already stored":
// `-L4 -L0` really does fall back to the width the value teaches.
func TestTheLastWidthNumberWritten(t *testing.T) {
	if got := zsh.Semantics().WidthNumberPrecedence; got != interp.WidthNumberLastWrittenWins {
		t.Errorf("WidthNumberPrecedence = %v, want the last number written", got)
	}
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`typeset -L5 -L3 v=7; typeset -p v`, "typeset -L3 v=7\n"},
		{`typeset -R5 -R3 v=7; typeset -p v`, "typeset -R3 v=7\n"},
		{`typeset -Z5 -Z3 v=7; typeset -p v`, "typeset -Z3 v=7\n"},
		{`typeset -L4 -L0 v=ab; typeset -p v`, "typeset -L2 v=ab\n"},
		{`typeset -L0 -L4 v=ab; typeset -p v`, "typeset -L4 v=ab\n"},
		{`typeset -R4 -R0 v=ab; typeset -p v`, "typeset -R2 v=ab\n"},
		{`typeset -L0 v=abcdef; typeset -p v`, "typeset -L6 v=abcdef\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
