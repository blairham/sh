// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// What a color code writes, and the whole of what was measured.
//
// 91 arguments in both layers, against zsh 5.9.2 with `TERM=xterm-256color`,
// through `print -P` — which is the same expansion a drawn prompt runs, so a
// measurement of one is a measurement of both. The table is the measurement
// and not a sample of it: the rule these rows yield has four readings and
// three of them were surprises, so a representative row would have been the
// wrong one three times out of four.
//
// The rule, and where each part of it is visible here:
//
//   - **A name matches by prefix**, and an ambiguous prefix is the first of
//     the eight in the order the terminal numbers them: `re` is red and `b`
//     is *black* rather than being ambiguous with blue. The name ends at the
//     first character that is not a letter, so `red,`, `red bold` and
//     `red;bold` are all red.
//   - **A run of letters that is no prefix is the default**, which is what
//     separates `bogus`, `grey`, `bo` and `x9` from everything below.
//   - **Anything not starting with a letter is read as C's `strtol` would**:
//     whitespace skipped, a sign taken, trailing junk ignored, and *no digits
//     at all is nought* — so `-`, `,`, ` `, `0x9` and ` red ` all draw the
//     first color, where `256` and `-1` draw the default because they are
//     numbers out of range.
//   - **`#rrggbb` and `#rgb` are the terminal's direct-color form**, under
//     every TERM measured including `dumb`, and each short digit is doubled:
//     `#abc` is 170, 187, 204. A `#` whose hex run starts badly is no number
//     and comes to nought — `#`, `#g`, `#ggg` — where one that starts well
//     and is the wrong length or ends badly is malformed and takes the
//     default: `#0`, `#0000`, `#00g`.
//
// The background half is the same arithmetic ten higher, and the two ranges
// above the first eight are their own runs of parameters rather than a
// modifier: 8 is not 0 made brighter, it is 90.
//
// **What is deliberately not modeled** is the count of colors the terminal
// claims: `%F{9}` is `\e[91m` under `TERM=xterm-256color` and `\e[39m` under
// `TERM=xterm`, which reports eight. See the note on colorSequence.
func TestWhatAColorCodeWrites(t *testing.T) {
	for _, tc := range []struct {
		layer PromptColor
		arg   string
		want  string
	}{
		{Foreground, "", "\x1b[30m"},
		{Background, "", "\x1b[40m"},
		{Foreground, "red", "\x1b[31m"},
		{Background, "red", "\x1b[41m"},
		{Foreground, "black", "\x1b[30m"},
		{Background, "black", "\x1b[40m"},
		{Foreground, "green", "\x1b[32m"},
		{Background, "green", "\x1b[42m"},
		{Foreground, "yellow", "\x1b[33m"},
		{Background, "yellow", "\x1b[43m"},
		{Foreground, "blue", "\x1b[34m"},
		{Background, "blue", "\x1b[44m"},
		{Foreground, "magenta", "\x1b[35m"},
		{Background, "magenta", "\x1b[45m"},
		{Foreground, "cyan", "\x1b[36m"},
		{Background, "cyan", "\x1b[46m"},
		{Foreground, "white", "\x1b[37m"},
		{Background, "white", "\x1b[47m"},
		{Foreground, "r", "\x1b[31m"},
		{Background, "r", "\x1b[41m"},
		{Foreground, "re", "\x1b[31m"},
		{Background, "re", "\x1b[41m"},
		{Foreground, "b", "\x1b[30m"},
		{Background, "b", "\x1b[40m"},
		{Foreground, "bl", "\x1b[30m"},
		{Background, "bl", "\x1b[40m"},
		{Foreground, "bla", "\x1b[30m"},
		{Background, "bla", "\x1b[40m"},
		{Foreground, "blu", "\x1b[34m"},
		{Background, "blu", "\x1b[44m"},
		{Foreground, "blac", "\x1b[30m"},
		{Background, "blac", "\x1b[40m"},
		{Foreground, "g", "\x1b[32m"},
		{Background, "g", "\x1b[42m"},
		{Foreground, "gr", "\x1b[32m"},
		{Background, "gr", "\x1b[42m"},
		{Foreground, "y", "\x1b[33m"},
		{Background, "y", "\x1b[43m"},
		{Foreground, "w", "\x1b[37m"},
		{Background, "w", "\x1b[47m"},
		{Foreground, "m", "\x1b[35m"},
		{Background, "m", "\x1b[45m"},
		{Foreground, "c", "\x1b[36m"},
		{Background, "c", "\x1b[46m"},
		{Foreground, "k", "\x1b[39m"},
		{Background, "k", "\x1b[49m"},
		{Foreground, "kk", "\x1b[39m"},
		{Background, "kk", "\x1b[49m"},
		{Foreground, "bo", "\x1b[39m"},
		{Background, "bo", "\x1b[49m"},
		{Foreground, "bold", "\x1b[39m"},
		{Background, "bold", "\x1b[49m"},
		{Foreground, "bogus", "\x1b[39m"},
		{Background, "bogus", "\x1b[49m"},
		{Foreground, "grey", "\x1b[39m"},
		{Background, "grey", "\x1b[49m"},
		{Foreground, "gray", "\x1b[39m"},
		{Background, "gray", "\x1b[49m"},
		{Foreground, "default", "\x1b[39m"},
		{Background, "default", "\x1b[49m"},
		{Foreground, "none", "\x1b[39m"},
		{Background, "none", "\x1b[49m"},
		{Foreground, "RED", "\x1b[39m"},
		{Background, "RED", "\x1b[49m"},
		{Foreground, "Red", "\x1b[39m"},
		{Background, "Red", "\x1b[49m"},
		{Foreground, "redx", "\x1b[39m"},
		{Background, "redx", "\x1b[49m"},
		{Foreground, "0", "\x1b[30m"},
		{Background, "0", "\x1b[40m"},
		{Foreground, "1", "\x1b[31m"},
		{Background, "1", "\x1b[41m"},
		{Foreground, "2", "\x1b[32m"},
		{Background, "2", "\x1b[42m"},
		{Foreground, "7", "\x1b[37m"},
		{Background, "7", "\x1b[47m"},
		{Foreground, "8", "\x1b[90m"},
		{Background, "8", "\x1b[100m"},
		{Foreground, "9", "\x1b[91m"},
		{Background, "9", "\x1b[101m"},
		{Foreground, "15", "\x1b[97m"},
		{Background, "15", "\x1b[107m"},
		{Foreground, "16", "\x1b[38;5;16m"},
		{Background, "16", "\x1b[48;5;16m"},
		{Foreground, "200", "\x1b[38;5;200m"},
		{Background, "200", "\x1b[48;5;200m"},
		{Foreground, "254", "\x1b[38;5;254m"},
		{Background, "254", "\x1b[48;5;254m"},
		{Foreground, "255", "\x1b[38;5;255m"},
		{Background, "255", "\x1b[48;5;255m"},
		{Foreground, "256", "\x1b[39m"},
		{Background, "256", "\x1b[49m"},
		{Foreground, "-1", "\x1b[39m"},
		{Background, "-1", "\x1b[49m"},
		{Foreground, "-", "\x1b[30m"},
		{Background, "-", "\x1b[40m"},
		{Foreground, "+", "\x1b[30m"},
		{Background, "+", "\x1b[40m"},
		{Foreground, "09", "\x1b[91m"},
		{Background, "09", "\x1b[101m"},
		{Foreground, "+9", "\x1b[91m"},
		{Background, "+9", "\x1b[101m"},
		{Foreground, "0x9", "\x1b[30m"},
		{Background, "0x9", "\x1b[40m"},
		{Foreground, "0x", "\x1b[30m"},
		{Background, "0x", "\x1b[40m"},
		{Foreground, "9x", "\x1b[91m"},
		{Background, "9x", "\x1b[101m"},
		{Foreground, "x9", "\x1b[39m"},
		{Background, "x9", "\x1b[49m"},
		{Foreground, "1red", "\x1b[31m"},
		{Background, "1red", "\x1b[41m"},
		{Foreground, " 2", "\x1b[32m"},
		{Background, " 2", "\x1b[42m"},
		{Foreground, "  9", "\x1b[91m"},
		{Background, "  9", "\x1b[101m"},
		{Foreground, "2 ", "\x1b[32m"},
		{Background, "2 ", "\x1b[42m"},
		{Foreground, " ", "\x1b[30m"},
		{Background, " ", "\x1b[40m"},
		{Foreground, "  ", "\x1b[30m"},
		{Background, "  ", "\x1b[40m"},
		{Foreground, ",", "\x1b[30m"},
		{Background, ",", "\x1b[40m"},
		{Foreground, ",red", "\x1b[30m"},
		{Background, ",red", "\x1b[40m"},
		{Foreground, "red,", "\x1b[31m"},
		{Background, "red,", "\x1b[41m"},
		{Foreground, "blue,", "\x1b[34m"},
		{Background, "blue,", "\x1b[44m"},
		{Foreground, "2,3", "\x1b[32m"},
		{Background, "2,3", "\x1b[42m"},
		{Foreground, "red,bold", "\x1b[31m"},
		{Background, "red,bold", "\x1b[41m"},
		{Foreground, "red bold", "\x1b[31m"},
		{Background, "red bold", "\x1b[41m"},
		{Foreground, "red;bold", "\x1b[31m"},
		{Background, "red;bold", "\x1b[41m"},
		{Foreground, " red ", "\x1b[30m"},
		{Background, " red ", "\x1b[40m"},
		{Foreground, "#", "\x1b[30m"},
		{Background, "#", "\x1b[40m"},
		{Foreground, "#0", "\x1b[39m"},
		{Background, "#0", "\x1b[49m"},
		{Foreground, "#00", "\x1b[39m"},
		{Background, "#00", "\x1b[49m"},
		{Foreground, "#000", "\x1b[38;2;0;0;0m"},
		{Background, "#000", "\x1b[48;2;0;0;0m"},
		{Foreground, "#0000", "\x1b[39m"},
		{Background, "#0000", "\x1b[49m"},
		{Foreground, "#f", "\x1b[39m"},
		{Background, "#f", "\x1b[49m"},
		{Foreground, "#ff", "\x1b[39m"},
		{Background, "#ff", "\x1b[49m"},
		{Foreground, "#fff", "\x1b[38;2;255;255;255m"},
		{Background, "#fff", "\x1b[48;2;255;255;255m"},
		{Foreground, "#ffff", "\x1b[39m"},
		{Background, "#ffff", "\x1b[49m"},
		{Foreground, "#fffff", "\x1b[39m"},
		{Background, "#fffff", "\x1b[49m"},
		{Foreground, "#ffffff", "\x1b[38;2;255;255;255m"},
		{Background, "#ffffff", "\x1b[48;2;255;255;255m"},
		{Foreground, "#abc", "\x1b[38;2;170;187;204m"},
		{Background, "#abc", "\x1b[48;2;170;187;204m"},
		{Foreground, "#FF8800", "\x1b[38;2;255;136;0m"},
		{Background, "#FF8800", "\x1b[48;2;255;136;0m"},
		{Foreground, "#ff8800", "\x1b[38;2;255;136;0m"},
		{Background, "#ff8800", "\x1b[48;2;255;136;0m"},
		{Foreground, "#F0F", "\x1b[38;2;255;0;255m"},
		{Background, "#F0F", "\x1b[48;2;255;0;255m"},
		{Foreground, "#g", "\x1b[30m"},
		{Background, "#g", "\x1b[40m"},
		{Foreground, "#gg0", "\x1b[30m"},
		{Background, "#gg0", "\x1b[40m"},
		{Foreground, "#ggg", "\x1b[30m"},
		{Background, "#ggg", "\x1b[40m"},
		{Foreground, "#00g", "\x1b[39m"},
		{Background, "#00g", "\x1b[49m"},
		{Foreground, "#gggggg", "\x1b[30m"},
		{Background, "#gggggg", "\x1b[40m"},
		{Foreground, "#ggggggg", "\x1b[30m"},
		{Background, "#ggggggg", "\x1b[40m"},
	} {
		if got := colorSequence(tc.layer, tc.arg); got != tc.want {
			t.Errorf("colorSequence(%v, %q) = %q, want %q", tc.layer, tc.arg, got, tc.want)
		}
	}
}

// Where the code ends, which is what decides whether the text after it is text.
func TestWhereACodesArgumentEnds(t *testing.T) {
	for _, tc := range []struct {
		name, in   string
		want       string
		wantEnd    int
		wantBraced bool
	}{
		// The braces and everything in them belong to the code.
		{"braced", "{red}x", "red", 4, true},
		// Empty braces are braces, and the third result is the only thing
		// that says so. It is a measured difference and not tidiness: zsh
		// draws `%D` as the plain date and `%D{}` as nothing at all, so a
		// code reading a `strftime` format has to tell an empty format from
		// the absence of one. A color code reads the two the same way round —
		// `%F` and `%F{}` both draw `\e[30m` — which is why the distinction
		// is reported rather than acted on here.
		{"empty braces", "{}x", "", 1, true},
		// No braces at all: the argument is empty and the code ended before
		// the letters, which is measured — zsh drew `%Fred` as a color and
		// then `red`.
		{"unbraced", "red", "", -1, false},
		{"nothing after it", "", "", -1, false},
		// An opening brace with no closing one is the rest of the prompt,
		// because there is no later text for the rest to be text of.
		{"unterminated", "{red", "red", 3, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			arg, end, braced := promptArgument([]rune(tc.in), 0)
			if arg != tc.want || end != tc.wantEnd || braced != tc.wantBraced {
				t.Errorf("promptArgument(%q) = %q, %d, %v, want %q, %d, %v",
					tc.in, arg, end, braced, tc.want, tc.wantEnd, tc.wantBraced)
			}
		})
	}
}

// A count in front of a color code is the index it paints.
//
// Measured 2026-09-12 against zsh 5.9.2 with `TERM=xterm-256color`, through
// `${(%%)…}`. The count uses the same reading `%F{N}` does, so the arithmetic
// was already right and all that was missing was the count reaching it: `%2F`
// is the second of the eight, `%9F` is in the bright run, `%30F` and `%200F`
// are the extended one, and 256 and above take the default exactly as
// `%F{256}` does.
//
// Braces win over a count, including empty ones, which is what says the two
// are one argument arriving two ways rather than two things to combine. And
// the letters after an unbraced count are still text — `%30Fx` paints and then
// draws `x` — so reading the count did not take the rest of the prompt with it.
//
// This branch dropped the count until #2087, so every unbraced numeric color
// painted the empty argument, which colorIndex reads as nought: black, at
// status 0, with nothing said.
func TestACountInFrontOfAColorCodeIsTheIndex(t *testing.T) {
	style := PromptStyle{
		Escape:          '%',
		NumericArgument: true,
		Colors:          map[rune]PromptColor{'F': Foreground, 'K': Background},
	}
	for _, tc := range []struct{ text, want string }{
		{"%F", "\x1b[30m"},
		{"%0F", "\x1b[30m"},
		{"%1F", "\x1b[31m"},
		{"%2F", "\x1b[32m"},
		{"%9F", "\x1b[91m"},
		{"%15F", "\x1b[97m"},
		{"%16F", "\x1b[38;5;16m"},
		{"%30F", "\x1b[38;5;30m"},
		{"%200F", "\x1b[38;5;200m"},
		{"%255F", "\x1b[38;5;255m"},
		{"%256F", "\x1b[39m"},
		{"%300F", "\x1b[39m"},
		{"%2K", "\x1b[42m"},
		{"%30K", "\x1b[48;5;30m"},
		// The braces win, and an empty pair of them is still braces.
		{"%2F{red}", "\x1b[31m"},
		{"%2F{}", "\x1b[30m"},
		// What follows an unbraced count is text.
		{"%30Fx", "\x1b[38;5;30mx"},
		{"%2Fabc", "\x1b[32mabc"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			got, refused, ok := ExpandPromptStyle(style, tc.text, func(PromptField, string, bool) (string, bool) {
				return "", false
			}, nil)
			if !ok {
				t.Fatalf("%q was refused at %q", tc.text, refused)
			}
			if got != tc.want {
				t.Errorf("%q drew %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

// And a dialect without the count does not reach the color code at all, which
// is what keeps this a row of the table rather than a rule of the walker.
//
// bash spells its colors by writing the escape sequence out by hand and has no
// numeric argument anywhere, so with NumericArgument off the digit *is* the
// code: `%30F` is an escape in front of `3`, which no table has, and the rest
// is text. The count is read where the dialect says it is and nowhere else.
func TestWithoutANumericArgumentTheDigitIsTheCode(t *testing.T) {
	style := PromptStyle{
		Escape: '%',
		Colors: map[rune]PromptColor{'F': Foreground},
	}
	got, refused, ok := ExpandPromptStyle(style, "%30F", func(PromptField, string, bool) (string, bool) {
		return "", false
	}, nil)
	if ok {
		t.Fatalf("drew %q, want the digit refused as a code of its own", got)
	}
	if refused != "3" {
		t.Errorf("refused %q, want %q — the digit is the code where nothing reads it as a count", refused, "3")
	}
}

// A negative count in front of a color is no color at all.
//
// Measured 2026-09-12 against zsh 5.9.2 with `TERM=xterm-256color`, and it is
// the one place the two readings of a color's argument part company: `%F{-1}`
// writes the terminal's default, because `-1` is a number out of range, while
// `%-1F` writes **nothing whatever**. A bare `%-F` is minus one and writes
// nothing too, and `%-0F` is nought and writes the first color — so the sign
// decides and not the presence of the minus.
//
// It still clears the layer, which is the half that cannot be inferred from
// "it writes nothing" and is what the last three rows are for: after `%-2F`
// the reset has no foreground to write back, and after `%-2K` it has the
// foreground and not the background. A shell that skipped the code entirely
// would pass every row above and fail those.
func TestANegativeCountInFrontOfAColorIsNoColor(t *testing.T) {
	style := PromptStyle{
		Escape:          '%',
		NumericArgument: true,
		Colors:          map[rune]PromptColor{'F': Foreground, 'K': Background},
		Sequences:       map[rune]string{'b': "\x1b[0m"},
		Visual:          map[rune]PromptVisual{'b': {Attribute: AttributeBold, Off: true, Restores: true}},
	}
	for _, tc := range []struct{ text, want string }{
		{"%-1F", ""},
		{"%-2F", ""},
		{"%-9F", ""},
		{"%-F", ""},
		{"%-2K", ""},
		// The text after it is still text, so the code was read and not left
		// half-consumed.
		{"%-2Fx", "x"},
		// Nought is not negative, and braces beat the count as they do
		// everywhere else.
		{"%-0F", "\x1b[30m"},
		{"%-2F{red}", "\x1b[31m"},
		// It clears the layer it names, and only that one.
		{"%F{red}a%-2Fb%b", "\x1b[31mab\x1b[0m"},
		{"%K{blue}%F{red}%-2Fa%b", "\x1b[44m\x1b[31ma\x1b[0m\x1b[44m"},
		{"%K{blue}%F{red}%-2Ka%b", "\x1b[44m\x1b[31ma\x1b[0m\x1b[31m"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			got, refused, ok := ExpandPromptStyle(style, tc.text, func(PromptField, string, bool) (string, bool) {
				return "", false
			}, nil)
			if !ok {
				t.Fatalf("%q was refused at %q", tc.text, refused)
			}
			if got != tc.want {
				t.Errorf("%q drew %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}
