// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "testing"

// What a widget does to the line is a change an undo can take back, and a
// numbered undo goes back to the change a number was taken at (#5880).
//
// Measured 2026-10-04 through a pseudo-terminal against zsh 5.9.2, a widget
// on the line `xy`, `UNDO_CHANGE_NO` read where the rows say:
//
//	a=$UNDO_CHANGE_NO               2, and 2 again read a second time
//	BUFFER=one; c=$UNDO_CHANGE_NO   3
//	LBUFFER+=two; d=$UNDO_CHANGE_NO 4
//	zle .undo $c                    0, `one`, cursor 2
//	zle .undo $a                    0, `xy`, cursor 2
//	zle .undo 0                     1, the empty line
//	zle .undo 99                    0, the line as it was
func TestANumberedUndoGoesBackToTheChangeItWasTakenAt(t *testing.T) {
	type step struct {
		buffer string
		cursor int
		ok     bool
	}
	var nums []int
	var steps []step
	// In zsh's style, where an undo puts the cursor back where it was.
	zsh := EditorStyle{UndoRestoresTheCursorToWhereItWas: true}
	typedReachingBackStyled(t, zsh, nil, nil, func(in Line, ed Actions) (Line, bool) {
		a := ed.ChangeNumber(in)
		nums = append(nums, a, ed.ChangeNumber(in))
		in.Buffer = "one"
		c := ed.ChangeNumber(in)
		in.Buffer, in.Cursor = "onetwo", 6
		nums = append(nums, c, ed.ChangeNumber(in))
		for _, n := range []int{c, a, 0, 99} {
			out, ok := ed.UndoTo(n, in)
			steps = append(steps, step{out.Buffer, out.Cursor, ok})
			in = out
		}
		return in, true
	}, "xy\a\n")
	if want := []int{2, 2, 3, 4}; !equalInts(nums, want) {
		t.Errorf("change numbers %v, want %v", nums, want)
	}
	want := []step{{"one", 2, true}, {"xy", 2, true}, {"", 0, false}, {"", 0, true}}
	if len(steps) != len(want) {
		t.Fatalf("undid %+v, want %+v", steps, want)
	}
	for i := range want {
		if steps[i] != want[i] {
			t.Errorf("undo %d = %+v, want %+v", i+1, steps[i], want[i])
		}
	}
}

// An edit a widget made and then an undo it asked for: the edit is what is
// taken back, and not the last key typed before the widget. Measured against
// zsh 5.9.2 on the line `xy`: `BUFFER=one; zle .split-undo; LBUFFER+=two;
// zle .undo` leaves `one`.
func TestAnUndoInAWidgetTakesBackTheWidgetsOwnEdit(t *testing.T) {
	var after Line
	typedReachingBack(t, nil, func(in Line, ed Actions) (Line, bool) {
		in.Buffer, in.Cursor = "one", 3
		ed.ChangeNumber(in)
		in.Buffer, in.Cursor = "onetwo", 6
		after, _ = ed.Perform(WidgetUndo, in)
		return after, true
	}, "xy\a\n")
	if after.Buffer != "one" {
		t.Errorf("undo left %q, want %q", after.Buffer, "one")
	}
}

// Keys an action says it read are what a self-insert after it types, a byte
// of a character at a time and a line ending as a newline — the shape a
// paste arrives in when a widget reads it back with `read-command`.
func TestASelfInsertTypesTheKeysTheActionRead(t *testing.T) {
	got, _ := typedReachingBack(t, nil, func(in Line, ed Actions) (Line, bool) {
		for _, keys := range []string{"a", "\xc3", "\xa9", "\n", "\t", "b"} {
			in.Keys = keys
			in, _ = ed.Perform(WidgetSelfInsert, in)
		}
		return in, true
	}, "\a\n")
	// The tab is typed too: a control character types itself, drawn as the
	// spaces to its stop — measured against zsh 5.9.2 (#5972).
	if want := "aé\n\tb"; got != want {
		t.Errorf("typed %q, want %q", got, want)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
