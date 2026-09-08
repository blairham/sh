// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"errors"
	"strings"
	"testing"
)

// The override layer a shell's key-rebinding command drives.
//
// Nothing here names a shell, for the reason editkeys_test.go gives: a dialect
// package imports this one, so a test here asking what zsh calls a widget
// would be an import cycle as well as the wrong place. This pins what an
// override *does*; dialect/zsh pins which names reach it.

// typedBound runs a line through an editor with a table of overrides in front
// of it, built the way the front end builds one so the wiring is exercised
// rather than the editor alone.
func typedBound(t *testing.T, table map[string]Binding, keys string) string {
	t.Helper()
	var out strings.Builder
	e := Shell{KeyBindings: func() map[string]Binding { return table }}.newEditor(t.Context(), nil)
	e.in, e.out = strings.NewReader(keys), &out
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("%q: %v", keys, err)
	}
	return line
}

// TestAnOverriddenKeyRunsItsWidget is the whole point: a key nobody would
// otherwise act on moves the cursor because the table says so.
//
// ^G is the key, because the editor does nothing with it by itself — so a pass
// here cannot be the default dispatch having done the work.
func TestAnOverriddenKeyRunsItsWidget(t *testing.T) {
	table := map[string]Binding{"\a": {Widget: WidgetBeginningOfLine}}
	if got, want := typedBound(t, table, "world\aecho hello \n"), "echo hello world"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
	// And with no table the same keys leave the ^G doing nothing, so the
	// insert lands where it was typed. Without this the test above would pass
	// for an editor that ignored the table and happened to treat ^G as a
	// motion of its own.
	if got, want := typedBound(t, nil, "world\aecho hello \n"), "worldecho hello "; got != want {
		t.Errorf("unbound line = %q, want %q", got, want)
	}
}

// TestAMultiByteSequenceIsMatchedWhole covers the reason the lookup reads
// ahead at all: the sequences people bind are `^X` plus something, and a
// two-byte binding must not act on the first byte of it.
func TestAMultiByteSequenceIsMatchedWhole(t *testing.T) {
	table := map[string]Binding{"\x18\x01": {Widget: WidgetBeginningOfLine}}
	if got, want := typedBound(t, table, "world\x18\x01echo hello \n"), "echo hello world"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

// TestAnOverrideBeatsTheEditorsOwnKey is what rebinding *means*: a key the
// editor already acts on does the new thing instead of the old one.
//
// ^A is beginning-of-line by default here; bound to end-of-line it must move
// the other way, and a test that only checked the cursor moved would pass
// either way. So the line is composed to be wrong in a visible way if the
// default won.
func TestAnOverrideBeatsTheEditorsOwnKey(t *testing.T) {
	table := map[string]Binding{"\x01": {Widget: WidgetEndOfLine}}
	// Type `ab`, go to the start with ^B^B, then ^A. With the override ^A ends
	// at the far end and `!` lands after `ab`; with the default it would land
	// before it.
	if got, want := typedBound(t, table, "ab\x02\x02\x01!\n"), "ab!"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
	if got, want := typedBound(t, nil, "ab\x02\x02\x01!\n"), "!ab"; got != want {
		t.Errorf("unbound line = %q, want %q", got, want)
	}
}

// TestAKeyBoundToNothingStopsDoingWhatItDid is how a removed binding differs
// from an absent one. WidgetNone is present in the table and does nothing,
// which must not fall through to the default the key used to have.
func TestAKeyBoundToNothingStopsDoingWhatItDid(t *testing.T) {
	table := map[string]Binding{"\x01": {Widget: WidgetNone}}
	if got, want := typedBound(t, table, "ab\x01!\n"), "ab!"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

// TestAnUnboundByteIsNotReadPast is the hang this design exists to avoid: a
// key that starts nothing in the table must reach the editor with the bytes
// after it untouched.
//
// If the lookup read ahead on every key, the `b` would be swallowed looking
// for a sequence and the line would come back short.
func TestAnUnboundByteIsNotReadPast(t *testing.T) {
	table := map[string]Binding{"\x18\x01": {Widget: WidgetBeginningOfLine}}
	if got, want := typedBound(t, table, "ab\n"), "ab"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

// TestAStartedSequenceThatGoesNowhereIsDroppedWhole is the other half of
// reading by shape: `^X` followed by a byte no binding continues to takes both
// with it rather than typing the stray byte into the line.
func TestAStartedSequenceThatGoesNowhereIsDroppedWhole(t *testing.T) {
	table := map[string]Binding{"\x18\x01": {Widget: WidgetBeginningOfLine}}
	if got, want := typedBound(t, table, "a\x18zb\n"), "ab"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

// TestAnAbandonedSequenceDoesNotRunItsFirstKey is the other half of dropping a
// started sequence whole, and the half a mutant slipped through: the bytes
// consumed looking for a binding must not be handed back to the editor's own
// dispatch either.
//
// `^A` is the first byte here because the editor acts on it by itself. Reading
// `^A` and then a byte no binding continues to must leave the line alone — not
// jump the cursor to the start, which is what `^A` would have done had the
// lookup declined the key after eating the byte behind it.
func TestAnAbandonedSequenceDoesNotRunItsFirstKey(t *testing.T) {
	table := map[string]Binding{"\x01\x02": {Widget: WidgetEndOfLine}}
	if got, want := typedBound(t, table, "ab\x01z!\n"), "ab!"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

// TestAnExactMatchWinsOverALongerOne pins the choice made instead of a timer.
// Both `^X` and `^X^A` are bound; `^X` acts at once rather than waiting to
// find out whether the second byte was coming.
func TestAnExactMatchWinsOverALongerOne(t *testing.T) {
	table := map[string]Binding{
		"\x18":     {Widget: WidgetBeginningOfLine},
		"\x18\x01": {Widget: WidgetEndOfLine},
	}
	if got, want := typedBound(t, table, "ab\x18!\n"), "!ab"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

// TestInterruptPartWayThroughABindingAbandonsTheLine is the wedge escape.go
// names, reached through this layer instead: a sequence that has begun and
// cannot finish is the one failure that makes an editor look broken rather
// than incomplete, and ^C between the bytes of a binding has to mean the line
// rather than the key.
func TestInterruptPartWayThroughABindingAbandonsTheLine(t *testing.T) {
	var out strings.Builder
	table := map[string]Binding{"\x18\x01": {Widget: WidgetBeginningOfLine}}
	e := Shell{KeyBindings: func() map[string]Binding { return table }}.newEditor(t.Context(), nil)
	e.in, e.out = strings.NewReader("ab\x18\x03"), &out
	if _, err := e.readLine(drawPrompt("$ ")); !errors.Is(err, ErrInterrupted) {
		t.Errorf("err = %v, want the line abandoned", err)
	}
}

// TestTheLastWordWidgetNeedsAHistoryToShowItsWork is the one action a bare
// editor cannot be tested for: with nothing recalled there is no last word,
// and doing nothing is what a widget wired to nothing also does. So this one
// gets a history, and asserts the word actually arrives.
func TestTheLastWordWidgetNeedsAHistoryToShowItsWork(t *testing.T) {
	var out strings.Builder
	table := map[string]Binding{"\a": {Widget: WidgetInsertLastWord}}
	e := Shell{KeyBindings: func() map[string]Binding { return table }}.newEditor(t.Context(), nil)
	e.history = []string{"echo one two"}
	e.in, e.out = strings.NewReader("ls \a\n"), &out
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("readLine: %v", err)
	}
	if want := "ls two"; line != want {
		t.Errorf("line = %q, want %q", line, want)
	}
}

// TestEveryWidgetIsReachable walks the whole vocabulary through the dispatch.
//
// It exists because a constant added to the list and forgotten in runWidget is
// a name a dialect can bind and a person can press to no effect — the exact
// failure widgets.go's comment says the list is kept short to avoid, and one
// no other test would see.
func TestEveryWidgetIsReachable(t *testing.T) {
	// The line typed before the widget runs, and what the line must be after —
	// chosen so that no two widgets could pass each other's case.
	cases := []struct {
		widget Widget
		typed  string
		want   string
	}{
		{WidgetBeginningOfLine, "ab\x07!", "!ab"},
		{WidgetEndOfLine, "ab\x02\x02\x07!", "ab!"},
		{WidgetBackwardChar, "ab\x07!", "a!b"},
		{WidgetForwardChar, "ab\x02\x02\x07!", "a!b"},
		{WidgetBackwardWord, "one two\x07!", "one !two"},
		{WidgetForwardWord, "one two\x02\x02\x02\x02\x02\x02\x02\x07!", "one! two"},
		{WidgetKillLine, "ab\x02\x07", "a"},
		{WidgetKillWholeLine, "ab\x07", ""},
		// `a+b` rather than a plain word, because the kill before the cursor
		// and the backward *motion* disagree about punctuation and agree
		// about everything else: with the style left alone this key is
		// delimited by whitespace, so it takes the `+` with it. A word
		// without punctuation in it would pass for either.
		{WidgetKillWordBefore, "echo a+b\x07", "echo "},
		{WidgetKillWordAfter, "one two\x02\x02\x02\x07", "one "},
		{WidgetTransposeChars, "ab\x07", "ba"},
		// The two the undo work brought with it. Undo takes back the typing
		// before it, and the last word comes off a history this editor has
		// none of — so it is asserted for reaching its action and leaving the
		// line alone, which is what an empty history means.
		{WidgetUndo, "ab\x07", ""},
		{WidgetDeleteChar, "ab\x02\x02\x07", "b"},
		{WidgetBackwardDeleteChar, "ab\x07", "a"},
		// The yank has to have something to put back, so a kill comes first —
		// which also proves the two widgets share the one buffer.
		{WidgetYank, "ab\x0b\x0b\x07", "ab"},
	}
	for _, c := range cases {
		table := map[string]Binding{"\a": {Widget: c.widget}}
		if got := typedBound(t, table, c.typed+"\n"); got != c.want {
			t.Errorf("widget %d: line = %q, want %q", c.widget, got, c.want)
		}
	}
	// The four that change no text are exercised for reachability rather than
	// for an effect on the line: history browsing on an empty history, a
	// search that is immediately accepted, a screen clear, and a completion
	// with no completer. Each must leave the line alone and must not hang.
	for _, w := range []Widget{
		WidgetPreviousHistory, WidgetNextHistory, WidgetClearScreen, WidgetComplete, WidgetSearchHistoryBackward,
	} {
		table := map[string]Binding{"\a": {Widget: w}}
		if got, want := typedBound(t, table, "ab\a\n"), "ab"; got != want {
			t.Errorf("widget %d: line = %q, want %q", w, got, want)
		}
	}
}
