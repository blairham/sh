// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"path/filepath"
	"strings"
	"testing"
)

// The style a colored prompt is written in: brackets around what the terminal
// reads, and an escape character to write the color with.
func bracketing() PromptStyle {
	return PromptStyle{
		Escape: '\\',
		Codes: map[rune]PromptField{
			'[': FieldNonPrintingStart,
			']': FieldNonPrintingEnd,
			'u': FieldUser,
		},
		Sequences: map[rune]string{'e': "\x1b", 'a': "\a"},
		Unknown:   KeepBoth,
	}
}

// What the brackets are for: the bytes go to the terminal and none of them is
// a column.
func TestWhatIsBracketedIsWrittenAndNotCounted(t *testing.T) {
	s := Shell{Runner: newTestRunner(map[string]string{"USER": "someone"}), Style: bracketing()}
	got := drawPrompt(s.render(`\[\e[32m\]\u\[\e[0m\]$ `))
	if want := "\x1b[32msomeone\x1b[0m$ "; got.text != want {
		t.Errorf("drew %q, want %q", got.text, want)
	}
	if want := len("someone$ "); got.cells != want {
		t.Errorf("counted %d cells, want %d", got.cells, want)
	}
}

// And the markers themselves never reach the terminal, whichever loop drew
// them.
//
// A control character on the screen is what a prompt that said nothing about
// one would not have. bash writes a stray one for an unmatched `\]`, measured;
// this drops it instead.
func TestTheMarkersAreNotWritten(t *testing.T) {
	for _, tc := range []struct{ name, in string }{
		{"a matched pair", `\[\e[32m\]x`},
		{"an end with no start", `x\]y`},
		{"a start with no end", `x\[y`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Shell{Runner: newTestRunner(nil), Style: bracketing()}
			if got := drawPrompt(s.render(tc.in)); strings.ContainsAny(got.text, markStart+markEnd) {
				t.Errorf("a marker reached the terminal in %q", got.text)
			}
		})
	}
}

// A start with no end hides what follows it, and an end with no start is only
// dropped.
func TestAnUnmatchedMarker(t *testing.T) {
	if got := drawPrompt("ab" + markStart + "cde"); got.cells != 2 {
		t.Errorf("counted %d cells, want the 2 before the marker", got.cells)
	}
	if got := drawPrompt("ab" + markEnd + "cde"); got.cells != 5 {
		t.Errorf("counted %d cells, want all 5", got.cells)
	}
}

// The bytes between the markers are not read for anything, which is the whole
// point of being told about them.
//
// A prompt that sets the terminal's title is the case that cannot be worked
// out by looking: `\e]0;...\a` is not a sequence the ordinary reckoning knows,
// so every letter of the title is charged to the prompt as a column. With the
// brackets it is charged nothing, and the arithmetic is right whatever is in
// there.
func TestTitleSettingIsCountedOnlyWhenItIsBracketed(t *testing.T) {
	s := Shell{Runner: newTestRunner(nil), Style: bracketing()}
	const title = `\e]0;a long window title\a`
	bracketed := drawPrompt(s.render(`\[` + title + `\]$ `))
	if bracketed.cells != 2 {
		t.Errorf("counted %d cells, want the 2 of `$ `", bracketed.cells)
	}
	// The same prompt written without them, to show the markers are doing the
	// work and the escape-skipping alone is not.
	bare := drawPrompt(s.render(title + `$ `))
	if bare.cells == 2 {
		t.Error("an unbracketed title cost nothing, so this test proves nothing")
	}
	if bracketed.text != bare.text {
		t.Errorf("the two wrote different bytes: %q and %q", bracketed.text, bare.text)
	}
}

// The proof that the width is what the editor uses: a colored prompt is drawn
// exactly as the plain one of the same visible width is.
//
// Byte for byte, with the color taken back out. Every cursor movement in a
// redraw is counted from the prompt's width — which row the line ends on, how
// far back up to come, whether it wrapped at all — so if the color were
// counted, a wrapped line would be drawn over itself on every keystroke.
func TestAColoredPromptWrapsWhereThePlainOneDoes(t *testing.T) {
	const (
		cols  = 20
		color = "\x1b[32m"
		typed = "0123456789012345678\r"
	)
	plain := typedAtWith(t, cols, "abc> ", typed)
	for _, tc := range []struct{ name, prompt string }{
		// Bracketed, which is how a prompt says so.
		{"bracketed", markStart + color + markEnd + "abc> "},
		// And unbracketed, because a color written without the brackets is
		// still a sequence the reckoning knows: bash counts this one and draws
		// a long line over itself, and being right is worth more than being
		// bug-compatible about where the cursor goes.
		{"unbracketed", color + "abc> "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			colored := typedAtWith(t, cols, tc.prompt, typed)
			if got := strings.ReplaceAll(colored, color, ""); got != plain {
				t.Errorf("drew\n%q\nwant the plain prompt's drawing\n%q", got, plain)
			}
			// And the drawing is one that wrapped, or the two agreeing says
			// nothing.
			if !strings.Contains(plain, "\x1b[1A") {
				t.Fatal("the line did not wrap, so this test proves nothing")
			}
		})
	}
}

// A prompt that is bracketed and one that is not are drawn differently when
// the terminal cannot be told what the sequence is.
//
// The same comparison as above, with a title sequence instead of a color: this
// one *does* need the brackets, and the test says so by failing to match
// without them.
func TestAnUnbracketedTitleIsCountedAndDrawsDifferently(t *testing.T) {
	const (
		cols  = 20
		title = "\x1b]0;window\a"
		typed = "0123456789012345678\r"
	)
	plain := typedAtWith(t, cols, "abc> ", typed)
	bracketed := typedAtWith(t, cols, markStart+title+markEnd+"abc> ", typed)
	if got := strings.ReplaceAll(bracketed, title, ""); got != plain {
		t.Errorf("bracketed drew\n%q\nwant\n%q", got, plain)
	}
	bare := typedAtWith(t, cols, title+"abc> ", typed)
	if got := strings.ReplaceAll(bare, title, ""); got == plain {
		t.Error("an unbracketed title was drawn correctly, so the brackets are not what fixed it")
	}
}

// Both loops write the prompt, so both have to take the markers out of it.
//
// The one without a terminal is the one nothing else exercises: it writes the
// prompt itself rather than handing it to the editor, and a rendering that
// went straight to the error stream would put the two control characters on
// the screen at every prompt.
func TestThePlainLoopWritesTheStrippedPrompt(t *testing.T) {
	var out, errs strings.Builder
	r := newTestRunner(map[string]string{
		"USER": "someone", "PS1": `\[\e[32m\]\u$ `,
		"HISTFILE": filepath.Join(t.TempDir(), "history"),
	})
	r.Stdout = &out
	s := Shell{
		Runner: r, In: readerFile(t, ": one\n"), Out: &out, Err: &errs,
		Style: bracketing(),
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if want := "\x1b[32msomeone$ \x1b[32msomeone$ "; errs.String() != want {
		t.Errorf("prompted %q, want %q", errs.String(), want)
	}
}
