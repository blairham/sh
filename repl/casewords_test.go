// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// The case keys and transpose-words, against the table in casewords.go.
//
// Each row is typed as the line, the cursor walked back from the end, the
// key, and an `X` typed where the cursor ended up — so the accepted line
// carries the cursor as well as the text.
func TestTheCaseKeysAndTransposeWords(t *testing.T) {
	style := EditorStyle{
		WordCharacters:  "*?_-.[]~=/&;!#$%^(){}<>",
		WideEmacsKeymap: true,
		PrefixArgument:  true,
	}
	at := func(line string, cursor int) string {
		return line + strings.Repeat("\x02", len([]rune(line))-cursor)
	}
	for _, c := range []struct {
		name, line string
		cursor     int
		keys, want string
	}{
		{"M-u from the c", "echo abc def", 7, "\x1bu", "echo abCX def"},
		{"M-u from a blank takes the next word", "echo abc def", 4, "\x1bu", "echo ABCX def"},
		{"M-u at the end does nothing", "echo abc def", 12, "\x1bu", "echo abc defX"},
		{"M-U is M-u", "echo abc def", 7, "\x1bU", "echo abCX def"},
		{"a word character is part of the word", "foo-bar/baz.q x", 0, "\x1bu", "FOO-BAR/BAZ.QX x"},
		{"M-l", "echo ABC DEF", 7, "\x1bl", "echo ABcX DEF"},
		{"M-c from the B", "echo aBC dEF", 6, "\x1bc", "echo aBcX dEF"},
		{"M-c from a blank", "echo aBC dEF", 8, "\x1bc", "echo aBC DefX"},
		{"M-c passes a digit to the first letter", "3ab x", 0, "\x1bc", "3AbX x"},
		{"M-c does not restart after a word character", "foo-bar", 0, "\x1bc", "Foo-barX"},
		{"M-c in another script", "éCOLE x", 0, "\x1bc", "ÉcoleX x"},
		{"a count is that many words", "ab cd ef", 0, "\x1b2\x1bu", "AB CDX ef"},
		{"a count past the end stops there", "ab cd ef", 0, "\x1b5\x1bu", "AB CD EFX"},
		{"a negative count leaves the cursor", "ab cD eF", 3, "\x1b-\x1b2\x1bc", "ab XCd Ef"},
		{"and a count of nought does nothing", "ab cd", 0, "\x1b0\x1bc", "Xab cd"},

		{"M-t from the c", "echo abc def", 7, "\x1bt", "abc echoX def"},
		{"M-t with no word before does nothing", "aa bb cc", 1, "\x1bt", "aXa bb cc"},
		{"M-t on a blank takes the next word", "aa bb cc", 5, "\x1bt", "aa cc bbX"},
		{"M-t at the end swaps the last two", "aa bb cc", 8, "\x1bt", "aa cc bbX"},
		{"M-t after the last word takes it", "aa bb  ", 7, "\x1bt", "bb aaX  "},
		{"the separator stays", "aa, bb", 4, "\x1bt", "bb, aaX"},
		{"a count reaches further back", "aa bb cc dd", 11, "\x1b2\x1bt", "aa dd cc bbX"},
		{"a count too far does nothing", "aa bb cc dd", 11, "\x1b9\x1bt", "aa bb cc ddX"},
		{"a count of nought moves to the word's end", "aa bb cc dd", 4, "\x1b0\x1bt", "aa bbX cc dd"},
		{"a negative count leaves the cursor", "aa bb cc dd", 6, "\x1b-\x1bt", "aa cc Xbb dd"},
		{"M-T is M-t", "echo abc def", 7, "\x1bT", "abc echoX def"},

		{"one undo takes the case back", "echo abc def", 7, "\x1bu\x1f", "echo abXc def"},
		{"and the swap", "echo abc def", 7, "\x1bt\x1f", "echo abXc def"},
	} {
		t.Run(c.name, func(t *testing.T) {
			style := style
			style.UndoRestoresTheCursorToWhereItWas = true
			got := typedStyled(t, style, at(c.line, c.cursor)+c.keys+"X\r")
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// Without the field the keys do what they always did here, which is
// nothing, and in vi editing they are not the insert keymap's either.
func TestTheCaseKeysBelongToTheEmacsKeymapThatHasThem(t *testing.T) {
	if got := typedStyled(t, EditorStyle{}, "echo abc\x02\x1bu\x1btX\r"); got != "echo abXc" {
		t.Errorf("without the field: got %q", got)
	}
}
