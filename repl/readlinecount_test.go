// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
	"time"
)

// readlineCounts is the readline dialect's answers to every question a count
// asks, as bash sets them (#6248).
var readlineCounts = EditorStyle{
	PrefixArgument:                       true,
	CountPrompt:                          "(arg: %d) ",
	CountReadAsReadline:                  true,
	NegativeCountTypesNothing:            true,
	CountStopsWhereItCannotAct:           true,
	NegativeCaseCountGoesBackward:        true,
	TransposeWordsCountAsReadline:        true,
	YankLastArgCountAsReadline:           true,
	KillLineReadsOnlyTheSign:             true,
	TransposeCharsTakesNoNegativeCount:   true,
	BellRingsWhenAnEditHasNothingToActOn: true,
	WordKeys:                             true,
	CapitalizeTakesTheFirstCharacter:     true,
	TransposeWordsReachesTheLineEnd:      true,
}

// A count under readline's rules, against the rows measured 2026-10-06
// through a pseudo-terminal against bash 5.3.20 — see the EditorStyle fields.
// Each row is the line, the cursor walked back from the end, the keys, and an
// `@` typed where the cursor ended up; the bells the keys rang are counted
// apart from the ones the setup rang.
func TestACountAsReadlineReadsAndSpendsIt(t *testing.T) {
	at := func(line string, cursor int) string {
		return line + strings.Repeat("\x02", len([]rune(line))-cursor)
	}
	for _, c := range []struct {
		line   string
		cursor int
		keys   string
		want   string
		rings  bool
	}{
		// The issue's four rows.
		{"echo ab", 7, "\x1b9\x02", "@echo ab", true},
		{"echo ab", 2, "\x1b3\x7f", "@ho ab", true},
		{"echo ab", 7, "\x1b2\x06", "echo ab@", true},
		{"echo ab", 0, "\x1b9\x06", "echo ab@", false},

		// Running out part-way: backward a character rings, the rest do not.
		{"echo ab", 1, "\x1b3\x02", "@echo ab", true},
		{"echo ab", 1, "\x1b3\x1b[D", "@echo ab", true},
		{"echo ab", 1, "\x1b3\x08", "@cho ab", true},
		{"echo ab", 5, "\x1b3\x1b[C", "echo ab@", false},
		{"echo ab", 5, "\x1b9\x04", "echo @", false},
		{"echo ab", 7, "\x1b2\x04", "echo ab@", true},
		{"echo ab", 0, "\x1b2\x17", "@echo ab", true},
		{"aa bb cc dd", 11, "\x1b9\x17", "@", false},
		{"echo ab", 1, "\x1b-3\x06", "@echo ab", true},
		{"echo ab", 6, "\x1b-3\x02", "echo ab@", false},
		{"abcd", 1, "\x1b9\x14", "bcda@", false},
		{"abcd", 4, "\x1b2\x14", "abdc@", false},
		{"abcd", 2, "\x1b2\x14", "acdb@", false},

		// How the count is typed.
		{"echo ab", 7, "\x1b12z", "echo ab" + strings.Repeat("z", 12) + "@", false},
		{"echo ab", 7, "\x1b1\x1b2z", "echo ab" + strings.Repeat("z", 12) + "@", false},
		{"echo ab", 3, "\x1b-2\x02", "echo @ab", false},
		{strings.Repeat("a", 30), 30, "\x1b-\x1b2\x06", strings.Repeat("a", 18) + "@" + strings.Repeat("a", 12), false},
		{strings.Repeat("a", 30), 30, "\x1b-2\x1b3\x06", strings.Repeat("a", 7) + "@" + strings.Repeat("a", 23), false},
		{"ab", 1, "\x1b3\x1b-z", "a---z@b", false},
		{"ab", 1, "\x1b1-\x02", "a@-b", false},
		{"ab", 1, "\x1b-\x1b-z", "az@b", false},
		{"ab", 1, "\x1b--z", "a@b", false},

		// A negative count types nothing, and ^V takes that many keys.
		{"ab", 1, "\x1b-z", "a@b", false},
		{"ab", 1, "\x1b-3z", "a@b", false},
		{"ab", 1, "\x1b0z", "a@b", false},
		{"ab", 1, "\x1b3\x16\x01", "a\x01\x01\x01@b", false},
		{"ab", 1, "\x1b-\x16\x01", "a\x01@b", false},
		{"ab", 1, "\x1b-3\x16\x01y\x01", "a\x01y\x01@b", false},

		// The case keys go backward for a negative count.
		{"aa bb cc", 8, "\x1b-\x1bu", "aa bb CC@", false},
		{"aa bb cc", 7, "\x1b-\x1bu", "aa bb C@c", false},
		{"aa bb cc", 6, "\x1b-\x1bu", "aa BB @cc", false},
		{"aa bb cc", 8, "\x1b-\x1bc", "aa bb Cc@", false},
		{"aa bb cc dd ee", 14, "\x1b-3\x1bu", "aa bb CC DD EE@", false},
		{"aa bb cc", 0, "\x1b2\x1bu", "AA BB@ cc", false},
		{"aa bb cc", 0, "\x1b9\x1bu", "AA BB CC@", false},

		// transpose-words with a count (#6265).
		{"aa bb cc dd ee", 0, "\x1b2\x1bt", "bb aa@ cc dd ee", false},
		{"aa bb cc dd ee", 0, "\x1b3\x1bt", "cc bb aa@ dd ee", false},
		{"aa bb cc dd ee", 3, "\x1b2\x1bt", "cc bb aa@ dd ee", false},
		{"aa bb cc dd ee", 3, "\x1b3\x1bt", "dd bb cc aa@ ee", false},
		{"aa bb cc dd ee", 6, "\x1b2\x1bt", "aa dd cc bb@ ee", false},
		{"aa bb cc dd ee", 6, "\x1b3\x1bt", "aa ee cc dd bb@", false},
		{"aa bb cc dd ee", 9, "\x1b2\x1bt", "aa bb ee dd cc@", false},
		{"aa bb cc dd ee", 9, "\x1b3\x1bt", "aa ee cc dd bb@", false},
		{"aa bb cc dd ee", 14, "\x1b2\x1bt", "aa bb ee dd cc@", false},
		{"aa bb cc dd", 11, "\x1b9\x1bt", "dd bb cc aa@", false},
		{"aa bb cc dd ee", 4, "\x1b0\x1bt", "aa b@b cc dd ee", false},
		{"aa bb cc dd ee", 6, "\x1b-\x1bt", "aa bb @cc dd ee", true},
		{"aa bb cc dd ee", 14, "\x1b-2\x1bt", "aa bb cc dd ee@", true},

		// kill-line reads only the count's sign.
		{"aa bb cc dd", 5, "\x1b-\x0b", "@ cc dd", false},
		{"aa bb cc dd", 5, "\x1b-3\x0b", "@ cc dd", false},
		{"aa bb cc dd", 5, "\x1b0\x0b", "aa bb@", false},
		{"aa bb cc dd", 5, "\x1b3\x0b", "aa bb@", false},
		{"aa bb cc dd", 0, "\x1b-\x0b", "@aa bb cc dd", true},

		// transpose-chars takes no count of nought or less, but at the end.
		{"abcd", 3, "\x1b-\x14", "abc@d", false},
		{"abcd", 1, "\x1b-\x14", "a@bcd", false},
		{"abcd", 2, "\x1b0\x14", "ab@cd", false},
		{"abcd", 4, "\x1b-\x14", "abdc@", false},
		{"abcdef", 5, "\x1b-3\x14", "abcde@f", false},
		{"abcd", 0, "\x1b2\x14", "@abcd", true},
	} {
		t.Run(c.line+" "+strings.ReplaceAll(c.keys, "\x1b", "ESC"), func(t *testing.T) {
			setup := at(c.line, c.cursor)
			before, _ := typedBells(t, readlineCounts, setup+"@\r")
			bells, got := typedBells(t, readlineCounts, setup+c.keys+"@\r")
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
			if rang := bells - before; rang != map[bool]int{true: 1}[c.rings] {
				t.Errorf("rang %d times, want rings=%v", rang, c.rings)
			}
		})
	}
}

// typedBells is typedStyled with the bells the read rang.
func typedBells(t *testing.T, style EditorStyle, keys string) (int, string) {
	t.Helper()
	var out strings.Builder
	e := Shell{Editor: style}.newEditor(t.Context(), nil)
	e.in, e.out = typing(keys), &out
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("%q: %v", keys, err)
	}
	return strings.Count(out.String(), bell), line
}

// The count is drawn in place of the prompt's last row while it is typed, and
// the prompt comes back with the key that spends it — through a terminal,
// because a draw is only answerable there.
//
// And the presses after the first are played at once rather than when the next
// key arrives: the wait for a descriptor sat between the first press and the
// rest, so `ESC 3 ^B` moved the cursor one place and left the other two for
// whatever was typed next (#6248). zsh's way of reading a count had the same
// wait in front of it, so both are asked.
func TestACountIsDrawnAndPlayedThroughATerminal(t *testing.T) {
	for _, c := range []struct {
		name  string
		style EditorStyle
		shown string
	}{
		{"readline", readlineCounts, "(arg: 3) abcdef"},
		{"zsh", EditorStyle{PrefixArgument: true}, "[1]abcdef"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := newSessionWith(t, func(sh *Shell) { sh.Editor = c.style })
			s.typeLine("abcdef\x1b3")
			waitForRow(t, s, c.shown)
			s.typeKeys("\x02")
			waitForRow(t, s, "[1]abcdef")
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, col := s.shown().at(); col == len("[1]abc") {
					break
				}
				if time.Now().After(deadline) {
					_, col := s.shown().at()
					t.Fatalf("the cursor is at column %d after ESC 3 ^B, want %d:\n%q", col, len("[1]abc"), s.screen.String())
				}
				time.Sleep(5 * time.Millisecond)
			}
		})
	}
}

// waitForRow waits for the screen's last row to read want.
func waitForRow(t *testing.T, s *session, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		rows := strings.Split(strings.TrimRight(s.shown().text(), "\n"), "\n")
		if strings.TrimRight(rows[len(rows)-1], " ") == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the last row is %q, want %q", rows[len(rows)-1], want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// yank-last-arg with a count (#6265), with `: x1 x2 x3` and then
// `: w1 w2 w3 w4` behind the prompt. See
// EditorStyle.YankLastArgCountAsReadline for the rows.
func TestYankLastArgReadsACountAsReadline(t *testing.T) {
	for _, c := range []struct {
		keys, want string
		rings      bool
	}{
		{"\x1b0\x1b.", ":", false},
		{"\x1b2\x1b.", "w2", false},
		{"\x1b4\x1b.", "w4", false},
		{"\x1b5\x1b.", "", true},
		{"\x1b-\x1b.", "w3", false},
		{"\x1b-2\x1b.", "w2", false},
		{"\x1b-4\x1b.", ":", false},
		{"\x1b-5\x1b.", "", true},
		{"\x1b1\x1b.\x1b.", "x1", false},
		{"\x1b-\x1b.\x1b.", "x2", false},
		{"\x1b.\x1b2\x1b.", "x3", false},
		{"\x1b.\x1b.\x1b-\x1b.", "w4", false},
		{"\x1b4\x1b.\x1b.", "", true},
		{"\x1b2\x1b_", "w2", false},
	} {
		t.Run(strings.ReplaceAll(c.keys, "\x1b", "ESC"), func(t *testing.T) {
			var out strings.Builder
			e := Shell{Editor: readlineCounts}.newEditor(t.Context(), nil)
			e.remember(": x1 x2 x3")
			e.remember(": w1 w2 w3 w4")
			e.in, e.out = typing(c.keys+"\r"), &out
			got, err := e.readLine(drawPrompt("$ "))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
			if rang := strings.Count(out.String(), bell) > 0; rang != c.rings {
				t.Errorf("rang %v, want %v", rang, c.rings)
			}
		})
	}
}
