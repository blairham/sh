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

// push-line, quote-line, accept-and-hold and `^X u`, on the keys zsh's emacs
// keymap has them on (#6241). Each row reads two lines from one editor, so
// what the first key put aside is what the second read starts from.
func TestTheLineKeysOfTheWideEmacsKeymap(t *testing.T) {
	style := EditorStyle{WideEmacsKeymap: true, UndoRestoresTheCursorToWhereItWas: true, UndoTakesBackOneKeystrokeAtATime: true}
	for _, c := range []struct {
		name, keys    string
		first, second string
	}{
		{"M-q puts the line aside with its cursor", "echo abc def\x02\x02\x02\x02\x02\x1bqtrue\rX\r", "true", "echo abXc def"},
		{"M-Q too", "echo abc def\x02\x02\x02\x02\x02\x1bQtrue\rX\r", "true", "echo abXc def"},
		{"and the line that comes back is undone to nothing", "echo abc\x1bqtrue\r\x1fX\r", "true", "X"},
		{"M-a runs the line and hands it back", "echo abc def\x02\x02\x02\x02\x02\x1baX\r", "echo abc def", "echo abXc def"},
		{"M-A too", "echo abc def\x02\x02\x02\x02\x02\x1bAX\r", "echo abc def", "echo abXc def"},
		{"and undo empties what it handed back", "echo abc\x1ba\x1fX\r", "echo abc", "X"},
		{"M-' quotes the line", "echo it's x\x02\x02\x1b'X\r\r", "'echo it'\\''s x'X", ""},
		{"an empty line quotes to two quotes", "\x1b'X\r\r", "''X", ""},
		{"^X u takes back a keystroke", "echo abc\x18u\x18uX\r\r", "echo aX", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out strings.Builder
			e := Shell{Editor: style}.newEditor(t.Context(), nil)
			e.in, e.out = typing(c.keys), &out
			for i, want := range []string{c.first, c.second} {
				got, err := e.readLine(drawPrompt("$ "))
				if err != nil {
					t.Fatalf("read %d: %v", i+1, err)
				}
				if got != want {
					t.Errorf("read %d = %q, want %q", i+1, got, want)
				}
			}
		})
	}
}

// `^V`, against the table in quotedinsert.go.
func TestQuotedInsertTakesTheNextKeyAsItIs(t *testing.T) {
	style := EditorStyle{WideEmacsKeymap: true, PrefixArgument: true}
	for _, c := range []struct{ name, keys, want string }{
		{"a control character", "ab\x02\x16\x01\r", "a\x01b"},
		{"Return is not accepted", "ab\x02\x16\r\r", "a\rb"},
		{"Tab is not completed", "ab\x02\x16\t\r", "a\tb"},
		{"ESC is one byte, and the rest is typed", "ab\x02\x16\x1b[A\r", "a\x1b[Ab"},
		{"^V itself", "ab\x02\x16\x16\r", "a\x16b"},
		{"a character of more than one byte", "ab\x02\x16é\r", "aéb"},
		{"a count", "ab\x02\x1b3\x16\x01\r", "a\x01\x01\x01b"},
		{"a negative count leaves the cursor before it", "ab\x02\x1b-\x16\x01X\r", "aX\x01b"},
		{"a count of nought types nothing", "ab\x02\x1b0\x16\x01\r", "ab"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := typedStyled(t, style, c.keys); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
	if got := typedStyled(t, EditorStyle{}, "ab\x02\x16\x01X\r"); got != "Xab" {
		t.Errorf("without the field ^V is ignored and ^A moves: got %q", got)
	}
}

// The same keys under the readline dialect's fields, against the rows in
// EditorStyle.CapitalizeTakesTheFirstCharacter and
// TransposeWordsReachesTheLineEnd (#6250). WordKeys alone puts them on the
// keys, and none of the rest of the wide keymap comes with it.
func TestTheWordKeysAsReadlineHasThem(t *testing.T) {
	style := EditorStyle{
		WordKeys:                         true,
		QuotedInsertInViInsert:           true,
		CapitalizeTakesTheFirstCharacter: true,
		TransposeWordsReachesTheLineEnd:  true,
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
		{"M-l", "ABC DEF", 0, "\x1bl", "abcX DEF"},
		{"M-L is M-l", "ABC DEF", 1, "\x1bL", "AbcX DEF"},
		{"a word is letters and digits", "foo-bar", 0, "\x1bu", "FOOX-bar"},
		{"M-c raises a leading digit, which has no case", "3AB x", 0, "\x1bc", "3abX x"},
		{"and lowers the rest after a digit", "a3B x", 0, "\x1bc", "A3bX x"},
		{"M-c from the B", "echo aBC dEF", 6, "\x1bc", "echo aBcX dEF"},
		{"M-c across punctuation", "x 3ab", 1, "\x1bc", "x 3abX"},

		{"M-t from a blank takes the next word", "aa bb cc", 2, "\x1bt", "bb aaX cc"},
		{"M-t at the end of a word takes the next", "aa bb cc", 5, "\x1bt", "aa cc bbX"},
		{"M-t at the end swaps the last two", "aa bb cc", 8, "\x1bt", "aa cc bbX"},
		{"M-t after the last word takes the blanks along", "aa bb  ", 7, "\x1bt", "bb   aaX"},
		{"and from the end of the last word", "aa bb  ", 5, "\x1bt", "bb   aaX"},
		{"and punctuation", "aa bb;;", 7, "\x1bt", "bb;; aaX"},
		{"the separator stays", "aa, bb", 4, "\x1bt", "bb, aaX"},
		{"M-t in the first word does nothing", "aa bb cc", 1, "\x1bt", "aXa bb cc"},
		{"M-t on one word does nothing", "ab", 2, "\x1bt", "abX"},

		{"^V", "ab", 1, "\x16\x01", "a\x01Xb"},
		{"M-q is not one of them", "ab", 2, "\x1bq", "abX"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := typedStyled(t, style, at(c.line, c.cursor)+c.keys+"X\r")
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// `M-t` with no word before its own rings in the dialect whose edits ring
// when they have nothing to act on — measured on bash 5.3.20, and the rows
// are in EditorStyle.TransposeWordsReachesTheLineEnd — and a swap does not.
func TestTransposeWordsWithNothingToSwapRings(t *testing.T) {
	style := EditorStyle{WordKeys: true, TransposeWordsReachesTheLineEnd: true, BellRingsWhenAnEditHasNothingToActOn: true}
	for _, c := range []struct {
		name, keys string
		rings      bool
	}{
		{"in the first word", "aa bb cc\x01\x06\x1bt", true},
		{"on one word", "ab\x1bt", true},
		{"on an empty line", "\x1bt", true},
		{"a swap", "aa bb\x1bt", false},
		{"M-u at the end, which has nothing to act on", "ab\x1bu", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out strings.Builder
			e := Shell{Editor: style}.newEditor(t.Context(), nil)
			e.in, e.out = typing(c.keys+"\r"), &out
			if _, err := e.readLine(drawPrompt("$ ")); err != nil {
				t.Fatal(err)
			}
			want := 0
			if c.rings {
				want = 1
			}
			if got := strings.Count(out.String(), bell); got != want {
				t.Errorf("rang %d times, want %d", got, want)
			}
		})
	}
}

// `^V` in vi insert mode is the readline dialect's, and the other's viins
// has nothing there (#6250).
func TestQuotedInsertInViInsertIsTheDialectsToGive(t *testing.T) {
	for _, c := range []struct {
		name  string
		style EditorStyle
		want  string
	}{
		{"with the field", EditorStyle{WordKeys: true, QuotedInsertInViInsert: true}, "a\x01b"},
		// ^V is ignored and ^A is beginning-of-line.
		{"without it", EditorStyle{WideEmacsKeymap: true}, "ba"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := Shell{Editor: c.style}.newEditor(t.Context(), nil)
			e.vi = func() bool { return true }
			e.in, e.out = typing("a\x16\x01b\r"), &strings.Builder{}
			got, err := e.readLine(drawPrompt("$ "))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
