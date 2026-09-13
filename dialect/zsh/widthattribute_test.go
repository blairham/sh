// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
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
