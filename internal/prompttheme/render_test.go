// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package prompttheme_test

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/blairham/sh/internal/prompttheme"
)

func TestWithNothingConfiguredThePromptIsBare(t *testing.T) {
	// A default prompt that is fast and plain is a better first minute than a
	// rich one that needs a file.
	engine := newEngine(t, nil)
	got := engine.Render(&prompttheme.Context{})

	if got.Text != "$ " || got.Cont != "> " {
		t.Errorf("Render = %q / %q, want the caller's bare prompt", got.Text, got.Cont)
	}
	if got.Right != "" {
		t.Errorf("a prompt with no elements drew a right prompt: %q", got.Right)
	}
}

func TestOneLoopDrawsALeanPromptAndAFramedOne(t *testing.T) {
	// The single most important structural decision in the engine: the same
	// loop, driven from two configurations, is both looks.
	lean := newEngine(t, assignments(t, "preset",
		"LEFT_ELEMENTS", "one two",
	))
	leanText := plain(lean.Render(&prompttheme.Context{}).Text)
	if leanText != "ONE TWO " {
		t.Errorf("the lean prompt drew %q", leanText)
	}
	if escapes(lean.Render(&prompttheme.Context{}).Text) != 0 {
		t.Error("a prompt that configured no colors still instructed the terminal")
	}

	framed := newEngine(t, assignments(t, "preset",
		"LEFT_ELEMENTS", "one two",
		"ONE_BACKGROUND", "4",
		"TWO_BACKGROUND", "5",
		"LEFT_SEGMENT_SEPARATOR", "",
		"LEFT_END_SYMBOL", "",
		"WHITESPACE", " ",
	))
	out := framed.Render(&prompttheme.Context{}).Text
	if got := plain(out); got != " ONE  TWO  " {
		t.Errorf("the framed prompt drew %q", got)
	}
	// The separator between two differently-backgrounded segments is drawn in
	// the previous background over the next one.
	if !strings.Contains(out, sgr("0;38;5;4;48;5;5")+"") {
		t.Errorf("the separator was not colored from one background into the other: %q", out)
	}
	// And the side's end symbol trails the last background off into the
	// terminal's own.
	if !strings.Contains(out, sgr("0;38;5;5")+"") {
		t.Errorf("the end symbol did not close the last background: %q", out)
	}
}

func TestASegmentThatDeclinesCostsNoSpace(t *testing.T) {
	engine := newEngine(t, assignments(t, "preset", "LEFT_ELEMENTS", "one absent two"))
	if got := plain(engine.Render(&prompttheme.Context{}).Text); got != "ONE TWO " {
		t.Errorf("an absent tool left a box behind: %q", got)
	}
}

func TestAnElementWithNoSegmentDrawsNothingAndIsNamed(t *testing.T) {
	// The third state of a setting — set and ignored — is the one that is
	// invisible by default, and the one this repository treats as its worst.
	settings := assignments(t, "preset", "LEFT_ELEMENTS", "one kubernetes two")
	engine := newEngine(t, settings)

	if got := plain(engine.Render(&prompttheme.Context{}).Text); got != "ONE TWO " {
		t.Errorf("an element with no segment took space: %q", got)
	}
	if got := engine.Roster.NotYet(); !slices.Equal(got, []string{"kubernetes"}) {
		t.Errorf("NotYet = %#v, want kubernetes", got)
	}
}

func TestNewlineSplitsASideAndTheLongerSideDecidesTheLines(t *testing.T) {
	engine := newEngine(t, assignments(t, "preset",
		"LEFT_ELEMENTS", "one newline two",
		"RIGHT_ELEMENTS", "three",
	))
	got := engine.Render(&prompttheme.Context{Columns: 40})

	rows := strings.Split(plain(got.Text), "\n")
	if len(rows) != 2 {
		t.Fatalf("the prompt drew %d rows: %#v", len(rows), rows)
	}
	if !strings.HasPrefix(rows[0], "ONE") {
		t.Errorf("the first row was %q", rows[0])
	}
	if rows[1] != "TWO " {
		t.Errorf("the last row was %q", rows[1])
	}
	// The right side of a banner line is placed by filling the gap.
	if !strings.HasSuffix(rows[0], "THREE") {
		t.Errorf("the banner's right side was not placed: %q", rows[0])
	}
	if utf8.RuneCountInString(rows[0]) != 40 {
		t.Errorf("the banner row was %d cells, want 40", utf8.RuneCountInString(rows[0]))
	}
	if got.Right != "" {
		t.Errorf("a banner line's right side became a right prompt: %q", got.Right)
	}
}

func TestTheLastLinesRightSideIsARightPrompt(t *testing.T) {
	// Separate from the text so the editor can hide it when the typed line
	// grows into it, rather than baking it into text that would then wrap.
	engine := newEngine(t, assignments(t, "preset",
		"LEFT_ELEMENTS", "one",
		"RIGHT_ELEMENTS", "three",
	))
	got := engine.Render(&prompttheme.Context{Columns: 40})

	if plain(got.Text) != "ONE " {
		t.Errorf("Text = %q", plain(got.Text))
	}
	if plain(got.Right) != "THREE" {
		t.Errorf("Right = %q", plain(got.Right))
	}
}

func TestTheGapIsNotASetting(t *testing.T) {
	// A whitespace setting that emptied the gap would produce a prompt that
	// cannot be read, so it is a fixed suffix on the line being typed on.
	engine := newEngine(t, assignments(t, "preset",
		"LEFT_ELEMENTS", "one",
		"WHITESPACE", "",
	))
	if got := plain(engine.Render(&prompttheme.Context{}).Text); got != "ONE " {
		t.Errorf("Text = %q, want the gap kept", got)
	}
}

func TestABannerIsDroppedRatherThanWrapped(t *testing.T) {
	engine := newEngine(t, assignments(t, "preset",
		"LEFT_ELEMENTS", "one newline two",
		"RIGHT_ELEMENTS", "three",
	))

	// A width nobody knows is not a width of 80.
	unknown := plain(engine.Render(&prompttheme.Context{}).Text)
	if strings.Contains(unknown, "THREE") {
		t.Errorf("the right side was placed with no width to place it in: %q", unknown)
	}
	// And a terminal too narrow for both keeps the left.
	narrow := plain(engine.Render(&prompttheme.Context{Columns: 6}).Text)
	if strings.Contains(narrow, "THREE") {
		t.Errorf("the halves collided rather than the right side being dropped: %q", narrow)
	}
	if !strings.Contains(narrow, "ONE") {
		t.Errorf("the left side was lost: %q", narrow)
	}
}

func TestWhatASegmentProducedIsTextAndNotMarkup(t *testing.T) {
	// A directory holding a percent sign is drawn, not read: nothing a
	// segment produces is ever expanded, and the template is the
	// configuration's.
	engine := newEngine(t, assignments(t, "preset", "LEFT_ELEMENTS", "literal"))
	if got := plain(engine.Render(&prompttheme.Context{}).Text); got != "100%F{red} ${X} " {
		t.Errorf("a segment's text was read as markup: %q", got)
	}
}

func TestMarkupInASettingReturnsToTheSegmentsOwnAppearance(t *testing.T) {
	// %f is the segment's foreground and not the terminal's, so a value that
	// colors one word of itself does not knock out the background the layout
	// painted around it.
	engine := newEngine(t, assignments(t, "preset",
		"LEFT_ELEMENTS", "one",
		"ONE_BACKGROUND", "4",
		"ONE_FOREGROUND", "7",
		"ONE_PREFIX", "%F{1}!%f",
	))
	out := engine.Render(&prompttheme.Context{}).Text

	if !strings.Contains(out, sgr("0;38;5;1;48;5;4")+"!") {
		t.Errorf("the prefix did not take its own foreground over the segment's background: %q", out)
	}
	if !strings.Contains(out, sgr("0;38;5;7;48;5;4")+"ONE") {
		t.Errorf("the segment's own appearance did not come back after the markup: %q", out)
	}
}

func TestEveryEscapeIsMarkedSoTheEditorIsNotCharged(t *testing.T) {
	engine := newEngine(t, assignments(t, "preset",
		"LEFT_ELEMENTS", "one two",
		"ONE_BACKGROUND", "4",
		"TWO_BACKGROUND", "5",
		"LEFT_SEGMENT_SEPARATOR", ">",
		"LEFT_END_SYMBOL", ">",
	))
	out := engine.Render(&prompttheme.Context{}).Text

	if strings.Contains(unmarked(out), "\x1b") {
		t.Errorf("an escape reached the editor's arithmetic: %q", out)
	}
	if got := unmarked(out); got != "ONE>TWO> " {
		t.Errorf("what the editor would measure is %q", got)
	}
}

func TestTheAddedNewlineIsASetting(t *testing.T) {
	engine := newEngine(t, assignments(t, "preset",
		"LEFT_ELEMENTS", "one",
		"ADD_NEWLINE", "true",
	))
	if got := plain(engine.Render(&prompttheme.Context{}).Text); got != "\nONE " {
		t.Errorf("Text = %q", got)
	}
}

func TestThePromptCharacterTakesTheLastStatus(t *testing.T) {
	engine := newEngine(t, assignments(t, "preset",
		"LEFT_ELEMENTS", "prompt_char",
		"PROMPT_CHAR_ERROR_SYMBOL", "✘",
		"PROMPT_CHAR_ERROR_FOREGROUND", "1",
	))

	if got := plain(engine.Render(&prompttheme.Context{}).Text); got != "$ " {
		t.Errorf("a prompt after a command that worked drew %q", got)
	}
	failed := engine.Render(&prompttheme.Context{Status: 1})
	if got := plain(failed.Text); got != "✘ " {
		t.Errorf("a prompt after a command that failed drew %q", got)
	}
	if !strings.Contains(failed.Text, sgr("0;38;5;1")+"✘") {
		t.Errorf("the failed state did not take its own color: %q", failed.Text)
	}
}

// newEngine builds an engine over a handful of segments that say their own
// names, so a layout test reads as layout rather than as a segment's output.
func newEngine(t *testing.T, settings *prompttheme.Store) *prompttheme.Engine {
	t.Helper()
	roster := prompttheme.NewRoster()
	for _, name := range []string{"one", "two", "three"} {
		roster.Compile(name, said(strings.ToUpper(name)))
	}
	roster.Compile("absent", prompttheme.SegmentFunc(
		func(*prompttheme.Settings, *prompttheme.Context) (prompttheme.Rendered, bool) {
			return prompttheme.Rendered{}, false
		}))
	roster.Compile("literal", said("100%F{red} ${X}"))
	roster.Compile("prompt_char", prompttheme.PromptChar("$"))

	layers := []prompttheme.Layer{}
	if settings != nil {
		layers = append(layers, settings)
	}
	return &prompttheme.Engine{
		Settings:  prompttheme.NewSettings(layers...),
		Roster:    roster,
		Screen:    prompttheme.Screen{Width: width, Mark: mark},
		Bare:      "$ ",
		Continued: "> ",
	}
}

func said(text string) prompttheme.Segment {
	return prompttheme.SegmentFunc(func(*prompttheme.Settings, *prompttheme.Context) (prompttheme.Rendered, bool) {
		return prompttheme.Rendered{Content: text}, true
	})
}

const (
	markStart = "\x01"
	markEnd   = "\x02"
)

func mark(escapes string) string { return markStart + escapes + markEnd }

// width counts what the editor would count: the cells, with everything
// between the markers discounted.
func width(s string) int { return utf8.RuneCountInString(unmarked(s)) }

// unmarked is what the editor measures — the text with every marked run gone.
func unmarked(s string) string {
	var b strings.Builder
	hidden := false
	for _, r := range s {
		switch r {
		case '\x01':
			hidden = true
		case '\x02':
			hidden = false
		default:
			if !hidden {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// plain is the drawn text with the markers and the escapes they hold taken
// out — what a person sees.
func plain(s string) string { return unmarked(s) }

func sgr(params string) string { return markStart + "\x1b[" + params + "m" + markEnd }

func escapes(s string) int { return strings.Count(s, "\x1b") }
