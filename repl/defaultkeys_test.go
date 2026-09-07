// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// DefaultBindings has to be what the dispatch does, and this is what makes
// the two of them one thing rather than two that agree today.
//
// The table is read by a dialect's key-listing command, so an entry that is
// wrong is a shell answering "this key does nothing" about a key that works,
// or the reverse. Nothing but a test can hold the two together: the dispatch
// is a switch and the table is a map, and neither is generated from the
// other.
//
// **This is not a test that hopes.** Each key is typed for real and the line
// it produces is compared against the line the *same widget* produces when it
// is reached through the override layer instead. A table entry naming the
// wrong widget therefore fails, and so does an entry for a key the dispatch
// ignores — the key would leave the line alone where the widget would not.
// The second assertion is what makes the first mean anything: it insists the
// widget changed the line, so "both did nothing" cannot pass as agreement.

// keyProbe is a line to type with one key left out of it, and whether the
// widget that key runs is one that changes the accepted line.
type keyProbe struct {
	// typed puts the key in the middle of a line and returns the whole of
	// what to type, the accepting Return included. It takes the key so the
	// same shape serves the real key and the stand-in.
	typed func(key string) string

	// changesTheLine is whether this widget's effect is visible in the
	// accepted line. False for the three that need something the editor has
	// not got in a test — a history to walk, a directory to complete
	// against — and for the clear, which moves no text.
	changesTheLine bool
}

// TestEveryDefaultKeyRunsTheWidgetTheTableNames types each key in the table
// and compares it against that widget reached by another key entirely.
func TestEveryDefaultKeyRunsTheWidgetTheTableNames(t *testing.T) {
	for seq, want := range DefaultBindings() {
		t.Run(keyLabel(seq), func(t *testing.T) {
			probe := widgetProbe(want)
			// ^G is the control byte the dispatch does nothing with, which is
			// what makes it usable as the stand-in: a pass cannot be the
			// dispatch having acted on the stand-in as well.
			viaOverride := typedBound(t, map[string]Binding{"\a": {Widget: want}}, probe.typed("\a"))
			viaKey := typedBound(t, nil, probe.typed(seq))
			if viaKey != viaOverride {
				t.Errorf("%s gave %q; the same widget through the override layer gave %q",
					keyLabel(seq), viaKey, viaOverride)
			}
			if !probe.changesTheLine {
				return
			}
			// And the widget really did something, or the comparison above
			// would hold for any two keys the editor ignores.
			if plain := typedBound(t, nil, probe.typed("")); viaKey == plain {
				t.Errorf("%s left the line at %q, which is what the same keys without it give, "+
					"so the probe cannot tell whether the widget ran", keyLabel(seq), plain)
			}
		})
	}
}

// widgetProbe is how each action is made visible in an accepted line.
//
// One probe per action, because the actions are not interchangeable: a motion
// shows up only when something is typed after it, and a kill shows up on its
// own.
func widgetProbe(w Widget) keyProbe {
	switch w {
	case WidgetBeginningOfLine, WidgetBackwardChar, WidgetBackwardWord:
		// Type the tail, go left, type the head: where the cursor landed is
		// where the head appears.
		return keyProbe{func(key string) string { return "cd" + key + "ab\n" }, true}
	case WidgetEndOfLine, WidgetForwardChar, WidgetForwardWord:
		// Go left first, so there is somewhere to come back from.
		return keyProbe{func(key string) string { return "ab\x02\x02" + key + "cd\n" }, true}
	case WidgetKillLine, WidgetDeleteChar:
		return keyProbe{func(key string) string { return "abcd\x02\x02" + key + "\n" }, true}
	case WidgetKillWholeLine, WidgetBackwardDeleteChar, WidgetKillWordBefore:
		return keyProbe{func(key string) string { return "ab cd" + key + "\n" }, true}
	case WidgetKillWordAfter:
		return keyProbe{func(key string) string { return "ab cd\x01" + key + "\n" }, true}
	case WidgetYank:
		// Kill a word, go to the front, put it back there.
		return keyProbe{func(key string) string { return "ab cd\x17\x01" + key + "\n" }, true}
	case WidgetTransposeChars:
		return keyProbe{func(key string) string { return "ab" + key + "\n" }, true}
	case WidgetUndo:
		return keyProbe{func(key string) string { return "ab cd\x17" + key + "\n" }, true}
	default:
		// The clear, the two history keys and completion. Each is still
		// compared against the same widget through the override layer, which
		// is the assertion that catches a wrong name; what is not claimed is
		// that the accepted line moved, because none of these four moves it
		// in an editor with no screen, no history and nothing to complete.
		return keyProbe{func(key string) string { return "ab" + key + "cd\n" }, false}
	}
}

// keyLabel names a sequence for a subtest, since the bytes are not printable.
func keyLabel(seq string) string {
	var out strings.Builder
	for i := 0; i < len(seq); i++ {
		switch c := seq[i]; {
		case c == 0x1b:
			out.WriteString("ESC-")
		case c == 0x7f:
			out.WriteString("^?")
		case c < 0x20:
			out.WriteByte('^')
			out.WriteByte(c + '@')
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}
