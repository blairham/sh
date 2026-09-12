// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// The visual state a rendering leaves behind belongs to the shell, so the next
// rendering restores from where the last one stopped.
//
// The mechanism, named as flags and never as a shell — the measured bytes are
// in dialect/zsh. [TestARestoringSequenceWritesBackWhatIsStillInEffect] is the
// same rule inside *one* rendering; this is the half that says the boundary
// between two of them is not a reset.
//
// It is not an edge case dressed up. A prompt theme that measures its own
// width renders the same text through the `%` flag many times before the
// prompt itself is drawn, so the state is never empty by the time it matters,
// and a walk that started empty wrote one escape too few between every
// segment (#2113).
func TestTheVisualStateOutlivesOneRendering(t *testing.T) {
	st := visualStyle()
	refuse := func(PromptField, string, bool) (string, bool) { return "", false }
	render := func(t *testing.T, visual *promptVisualState, text string) string {
		t.Helper()
		got, refused, ok := expandPromptStyle(st, text, refuse, nil, visual)
		if !ok {
			t.Fatalf("%q was refused at %q", text, refused)
		}
		return got
	}

	t.Run("a restore reaches back into the rendering before it", func(t *testing.T) {
		var visual promptVisualState
		if got := render(t, &visual, "%F{red}"); got != "\x1b[31m" {
			t.Fatalf("first rendering = %q", got)
		}
		// Alone, `%h` has nothing to write back — that row is in the
		// one-rendering test. Here the color of the rendering before it is
		// still in effect.
		if got, want := render(t, &visual, "%h"), "<h>\x1b[31m"; got != want {
			t.Errorf("second rendering = %q, want %q", got, want)
		}
	})

	t.Run("the state accumulates over every rendering", func(t *testing.T) {
		var visual promptVisualState
		render(t, &visual, "%F{red}")
		render(t, &visual, "%K{blue}")
		render(t, &visual, "%L")
		// One from each of three earlier renderings, in the order the
		// [PromptAttribute] constants are in.
		if got, want := render(t, &visual, "%h"), "<h><L>\x1b[31m\x1b[44m"; got != want {
			t.Errorf("restore = %q, want %q", got, want)
		}
	})

	t.Run("a clear in its own rendering leaves nothing to write back", func(t *testing.T) {
		var visual promptVisualState
		render(t, &visual, "%F{red}")
		render(t, &visual, "%l")
		// The underline was never set, so turning it off changes nothing and
		// the color is still there.
		if got, want := render(t, &visual, "%h"), "<h>\x1b[31m"; got != want {
			t.Errorf("restore = %q, want %q", got, want)
		}
	})

	t.Run("two states do not see each other", func(t *testing.T) {
		var one, two promptVisualState
		render(t, &one, "%F{red}")
		// What a subshell gets is a copy of this, and this is what says a
		// copy is enough: nothing is shared behind the value.
		if got, want := render(t, &two, "%h"), "<h>"; got != want {
			t.Errorf("restore in the second state = %q, want %q", got, want)
		}
	})

	t.Run("the exported walker keeps no state of its own", func(t *testing.T) {
		// ExpandPromptStyle is the entry for a caller with no shell behind
		// it, and it starts empty every time rather than sharing a package
		// global.
		if _, _, ok := ExpandPromptStyle(st, "%F{red}", refuse, nil); !ok {
			t.Fatal("first rendering was refused")
		}
		got, _, ok := ExpandPromptStyle(st, "%h", refuse, nil)
		if !ok {
			t.Fatal("second rendering was refused")
		}
		if want := "<h>"; got != want {
			t.Errorf("second rendering = %q, want %q", got, want)
		}
	})
}
