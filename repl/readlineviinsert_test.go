// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// readlineViInsert is the readline dialect's vi insert mode, as bash sets it
// (#6304).
var readlineViInsert = EditorStyle{
	ReadlineViInsertKeymap:               true,
	MenuReturnsToTheWord:                 true,
	ViUndoAsReadline:                     true,
	UndoRingsWithNothingToUndo:           true,
	BellRingsWhenAnEditHasNothingToActOn: true,
}

// readlineVi runs keys through a vi editor in the readline style, with a
// completer offering the words given, and hands back the line and how many
// times it rang.
func readlineVi(t *testing.T, style EditorStyle, words []string, keys string) (string, int) {
	t.Helper()
	var out strings.Builder
	e := Shell{
		ViEditing: func() bool { return true },
		Editor:    style,
		Completers: []Completer{CompleterFunc(func(c Completion) []Candidate {
			var got []string
			for _, w := range words {
				if strings.HasPrefix(w, c.Word) {
					got = append(got, w)
				}
			}
			return Words(got...)
		})},
	}.newEditor(t.Context(), nil)
	// In one read, as a terminal hands over a burst: an Escape followed by
	// `[` is then an arrow, and by anything else the mode switch (#6283).
	e.in, e.out = strings.NewReader(keys), &out
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("%q: %v", keys, err)
	}
	return line, strings.Count(out.String(), bell)
}

// `^W`, against bash 5.3.20 with the cursor at every place in `aa bb..cc  dd`
// and `a.. ..b  .a` — see viUnixWordRubout. Each row is the line, the cursor
// walked back from the end with Left, `^W` and a `Z` where the cursor ended.
func TestViUnixWordRuboutAsReadline(t *testing.T) {
	for _, c := range []struct {
		line string
		back int
		want string
	}{
		{"aa bb..cc  dd", 0, "aa bb..cc  Z"},
		{"aa bb..cc  dd", 1, "aa bb..cc  Zd"},
		{"aa bb..cc  dd", 2, "aa bb..ccZdd"},
		{"aa bb..cc  dd", 3, "aa bb..ccZ dd"},
		{"aa bb..cc  dd", 4, "aa bb..ccZ  dd"},
		{"aa bb..cc  dd", 6, "aa bbZcc  dd"},
		{"aa bb..cc  dd", 8, "aa bbZ..cc  dd"},
		{"aa bb..cc  dd", 10, "aaZbb..cc  dd"},
		{"aa bb..cc  dd", 11, "aaZ bb..cc  dd"},
		{"a.. ..b  .a", 1, "a.. ..bZa"},
		{"a.. ..b  .a", 5, "aZb  .a"},
		{"a.. ..b  .a", 7, "aZ..b  .a"},
		{"a.. ..b  .a", 10, "aZ.. ..b  .a"},
		{"aa bb  ", 0, "aa Z"},
		{"aa .;", 0, "aaZ"},
		{"x a_b", 0, "x a_Z"},
		{"x éa", 0, "x Z"},
	} {
		t.Run(c.line, func(t *testing.T) {
			keys := c.line + strings.Repeat("\x1b[D", c.back) + "\x17Z\r"
			if got, _ := readlineVi(t, readlineViInsert, nil, keys); got != c.want {
				t.Errorf("back %d: got %q, want %q", c.back, got, c.want)
			}
		})
	}
	if _, bells := readlineVi(t, readlineViInsert, nil, "\x17\r"); bells != 1 {
		t.Errorf("^W at the start rang %d times, want 1", bells)
	}
	// And two in a row are two kills: `^Y` puts back the second alone.
	if got, _ := readlineVi(t, readlineViInsert, nil, "aa bb\x17\x17\x19\r"); got != "aa " {
		t.Errorf("^W ^W ^Y: got %q, want %q", got, "aa ")
	}
}

// `^_` and `u` over a line that has been in command mode, against the rows in
// EditorStyle.ViUndoAsReadline, and the bell for nothing to undo.
func TestViUndoAsReadline(t *testing.T) {
	for _, c := range []struct {
		keys  string
		want  string
		bells int
	}{
		{"abc\x1f\x1f\r", "", 1},
		{"ab\x1bu\x1bu\r", "ab", 2},
		{"abc\x1b0xaZ\x1f\x1f\r", "bc", 1},
		{"a\x1bab\x1bac\x1f\x1f\x1f\r", "a", 1},
		{"a\x1bab\x1bac\x1buu\r", "a", 0},
		{"a\x1bab\x1bac\x1buuuu\r", "a", 2},
		{"ab\x1baX\x1fY\x1f\r", "ab", 0},
	} {
		t.Run(strings.ReplaceAll(c.keys, "\x1b", "ESC"), func(t *testing.T) {
			got, bells := readlineVi(t, readlineViInsert, nil, c.keys)
			if got != c.want || bells != c.bells {
				t.Errorf("got %q and %d bells, want %q and %d", got, bells, c.want, c.bells)
			}
		})
	}
}

// `^D` with something typed accepts the line wherever the cursor is.
func TestViEOFMaybeAcceptsALine(t *testing.T) {
	for _, keys := range []string{"echo ab\x04", "echo ab\x1b[D\x04"} {
		if got, _ := readlineVi(t, readlineViInsert, nil, keys+"Z\r"); got != "echo ab" {
			t.Errorf("%q: got %q, want the line accepted as it was", keys, got)
		}
	}
}

// `^N` and `^P` walk the matches and back to the word, against the rows in
// EditorStyle.MenuReturnsToTheWord.
func TestMenuCompletionReturnsToTheWord(t *testing.T) {
	words := []string{"aa1", "aa2", "aa3", "bb", "ddir/"}
	for _, c := range []struct {
		keys  string
		want  string
		bells int
	}{
		{"x aa\x0e\r", "x aa1 ", 0},
		{"x aa\x0e\x0e\r", "x aa2 ", 0},
		{"x aa\x0e\x0e\x0e\x0e\r", "x aa", 1},
		{"x aa\x0e\x0e\x0e\x0e\x0e\r", "x aa1 ", 1},
		{"x aa\x10\r", "x aa3 ", 0},
		{"x aa\x10\x10\r", "x aa2 ", 0},
		{"x aa\x0e\x10\r", "x aa", 1},
		{"x b\x0e\r", "x bb ", 0},
		{"x d\x0e\r", "x ddir/", 0},
		{"x zz\x0e\r", "x zz", 1},
	} {
		t.Run(c.keys, func(t *testing.T) {
			got, bells := readlineVi(t, readlineViInsert, words, c.keys)
			if got != c.want || bells != c.bells {
				t.Errorf("got %q and %d bells, want %q and %d", got, bells, c.want, c.bells)
			}
		})
	}
	// And without the keymap the keys are the shared table's.
	if got, _ := readlineVi(t, EditorStyle{}, words, "x aa\x0e\r"); got != "x aa" {
		t.Errorf("without the field ^N completed: %q", got)
	}
}
