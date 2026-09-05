// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// The keys a day at a prompt is spent on.
//
// Measured under a pty against bash 5.3.15 and zsh 5.9.2, a keystroke at a
// time. That last part is not a detail: written as one burst, bash looks as
// though it joins a kill onto the kill before it across an insert between
// them, and typed at human speed it does not. The burst is read out of pending
// input by a path that never sees the keystroke in between, so the first
// reading measures the harness.

// The two answers each of the word axes has, named for the answer and not for
// a shell.
//
// This package cannot name one: a dialect package imports it, so a test here
// that asked bash what it does would be an import cycle. It is also the rule —
// a test in the core names an axis, and the test that a given shell answers a
// given way lives beside that shell. `dialect/bash` and `dialect/zsh` pin the
// answers; this pins what each answer does.
var (
	// zeroAnswers is the whole of EditorStyle left alone, which is what a
	// front end that has said nothing gets.
	zeroAnswers = EditorStyle{}

	// otherAnswers is every one of them at its other value.
	otherAnswers = EditorStyle{
		WordCharacters:                         "*?_-.[]~=/&;!#$%^(){}<>",
		KillToStartOfLineTakesTheWholeLine:     true,
		KillWordBeforeCursorUsesWordCharacters: true,
		ForwardWordStopsBeforeTheNextWord:      true,
		TransposeAtTheStartSwapsTheFirstTwo:    true,
	}
)

// typedStyled runs a line through an editor built the way a dialect's front
// end builds one, so that what the style says has to travel the same route it
// travels in a session.
func typedStyled(t *testing.T, style EditorStyle, keys string) string {
	t.Helper()
	var out strings.Builder
	e := Shell{Editor: style}.newEditor()
	e.in, e.out = strings.NewReader(keys), &out
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("%q: %v", keys, err)
	}
	return line
}

// A realistic command, a realistic mistake, and whether it can be repaired
// without retyping.
//
// This is the test the key list is for. Each case is one command someone
// meant to type, mistyped in the way people mistype, and put right with the
// keys their fingers already know.
func TestFixingAMistypedCommandWithoutRetypingIt(t *testing.T) {
	for _, c := range []struct{ name, keys, want string }{
		{
			// A word in the middle is wrong: walk back to it, kill the word
			// rather than the rest of the line, and type the right one.
			"the wrong word in the middle",
			"git comit -m fix\x1bb\x1bb\x1bb\x1bdcommit\r",
			"git commit -m fix",
		},
		{
			// The classic: the path at the end is wrong, ^W takes it off.
			"the last path is wrong",
			"cp report.txt /tpm/backups\x17/tmp/backups\r",
			"cp report.txt /tmp/backups",
		},
		{
			// ^W once too often, and ^Y puts back what it should not have
			// taken. A kill that cannot be undone is one nobody presses.
			"one word kill too many",
			"cp report.txt /tmp/backups\x17\x17\x19\r",
			"cp report.txt /tmp/backups",
		},
		{
			// Two letters swapped, which is what ^T is for.
			"two letters the wrong way round",
			"grep -r pattren .\x1bb\x06\x06\x06\x06\x06\x14\r",
			"grep -r pattern .",
		},
		{
			// The whole front of the line is wrong.
			"start again from the cursor",
			"sudo apt install vim\x1bb\x15git clone \r",
			"git clone vim",
		},
		{
			// A word typed into the middle of the line, reached with Home
			// and the arrows rather than by retyping.
			"a word missing at the front",
			"cat /etc/hosts\x1b[1~sudo \r",
			"sudo cat /etc/hosts",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := typedStyled(t, zeroAnswers, c.keys); got != c.want {
				t.Errorf("gave %q, want %q", got, c.want)
			}
		})
	}
}

// Every spelling a terminal has for Home, End, the arrows and Delete.
//
// The same key is not the same bytes twice: a terminal in application cursor
// mode sends `\eOH` where it otherwise sends `\e[H`, and `\e[1~`, `\e[7~`,
// `\e[4~` and `\e[8~` are what several others send instead. A shell that knows
// one spelling has a Home key on some terminals and a stray `~` on the rest.
func TestEveryWayATerminalSpellsAKey(t *testing.T) {
	for _, c := range []struct{ name, keys, want string }{
		{"home csi", "echo abc\x1b[HX\r", "Xecho abc"},
		{"home ss3", "echo abc\x1bOHX\r", "Xecho abc"},
		{"home tilde 1", "echo abc\x1b[1~X\r", "Xecho abc"},
		{"home tilde 7", "echo abc\x1b[7~X\r", "Xecho abc"},
		{"end csi", "echo abc\x1b[H\x1b[FX\r", "echo abcX"},
		{"end ss3", "echo abc\x1b[H\x1bOFX\r", "echo abcX"},
		{"end tilde 4", "echo abc\x1b[H\x1b[4~X\r", "echo abcX"},
		{"end tilde 8", "echo abc\x1b[H\x1b[8~X\r", "echo abcX"},
		{"left csi", "echo abc\x1b[DX\r", "echo abXc"},
		{"left ss3", "echo abc\x1bODX\r", "echo abXc"},
		{"right csi", "echo abc\x1b[H\x1b[CX\r", "eXcho abc"},
		{"right ss3", "echo abc\x1b[H\x1bOCX\r", "eXcho abc"},
		{"delete", "echo abc\x1b[H\x1b[3~X\r", "Xcho abc"},
		{"delete at the end does nothing", "echo abc\x1b[3~X\r", "echo abcX"},
		// The modifier forms, which is what a terminal sends for Ctrl-Left
		// and Alt-Left. Measured: bash moves by a word for both.
		{"ctrl left", "echo one two\x1b[1;5DX\r", "echo one Xtwo"},
		{"alt left", "echo one two\x1b[1;3DX\r", "echo one Xtwo"},
		{"ctrl right", "echo one two\x1b[H\x1b[1;5CX\r", "echoX one two"},
		// Shift alone is not a word: it moves by one, like the bare arrow.
		{"shift right", "echo abc\x1b[H\x1b[1;2CX\r", "eXcho abc"},
		// A terminal sends the upper-case letter when Shift is held with the
		// Meta key, and both shells act on it the same as the lower-case one.
		{"meta b in upper case", "echo one two\x1bBX\r", "echo one Xtwo"},
		{"meta f in upper case", "echo one two\x1b[H\x1bFX\r", "echoX one two"},
		{"meta d in upper case", "echo one two\x1b[H\x1bDX\r", "X one two"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := typedStyled(t, zeroAnswers, c.keys); got != c.want {
				t.Errorf("gave %q, want %q", got, c.want)
			}
		})
	}
}

// A key this does not act on must not end up in the line.
//
// This is the failure that makes a shell feel broken rather than incomplete.
// An editor that recognizes some escape sequences and abandons the rest
// half-read hands the bytes it did not read back to the reader, which types
// them: Home becomes `~`, Ctrl-Right becomes `;5C`, and a mouse click becomes
// three characters of nonsense in the middle of a command.
//
// Both real shells do this. Measured, zsh 5.9 with no startup files puts `~`
// in the line for `\e[1~` and `;5C` for `\e[1;5C`; bash 5.3 leaves a `~` for
// `\e[4~`, `\e[7~` and `\e[8~`, and a `C` for `\e[1;2C`. Reading a sequence by
// its shape rather than by looking a spelling up is what makes that
// impossible here rather than merely unlikely.
func TestAnUnknownKeyIsNeverTypedIntoTheLine(t *testing.T) {
	for _, c := range []struct{ name, keys string }{
		{"insert", "\x1b[2~"},
		{"page up", "\x1b[5~"},
		{"page down", "\x1b[6~"},
		{"F5", "\x1b[15~"},
		{"F1 as ss3", "\x1bOP"},
		{"shift tab", "\x1b[Z"},
		// A mouse click in the old encoding, which is the one sequence that is
		// not shaped like a control sequence — three bytes follow the final
		// byte and have to be counted off. Measured: both real shells type
		// them into the line, and in bash the `!!` a click can land there is
		// then expanded into the previous command.
		{"a mouse click", "\x1b[M !!"},
		{"a mouse press, SGR", "\x1b[<0;12;30M"},
		{"a mouse release, SGR", "\x1b[<0;12;30m"},
		{"a device attributes reply", "\x1b[?1;2c"},
		// A private-use sequence is not the key its number would name: the
		// leading `?` makes it something else entirely, and reading the
		// number out of it would turn a terminal's own chatter into a Home.
		{"a private-use sequence numbered like Home", "\x1b[?1~"},
		{"a bracketed paste opening", "\x1b[200~"},
		{"a bracketed paste closing", "\x1b[201~"},
		{"a cursor position report", "\x1b[24;80R"},
		{"an unbound meta letter", "\x1bz"},
		{"a meta control character", "\x1b\x01"},
		{"a bare escape before a key it does not name", "\x1b\x1b"},
		{"a sequence with an intermediate byte", "\x1b[1 q"},
		{"a sequence with more parameters than any key has", "\x1b[" + strings.Repeat("9", 200) + "~"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, style := range []EditorStyle{zeroAnswers, otherAnswers} {
				const want = "echo hi"
				if got := typedStyled(t, style, "echo "+c.keys+"hi\r"); got != want {
					t.Errorf("gave %q, want %q — the key was typed into the line", got, want)
				}
			}
		})
	}
}

// Where the two dialects disagree, and they disagree about the keys a person
// presses most.
func TestTheDialectsDisagreeAboutWords(t *testing.T) {
	for _, c := range []struct{ name, keys, zero, other string }{
		// Punctuation is inside a word for zsh and outside it for bash.
		{"word motion over a path", "echo /usr/local/bin\x1bbX\r", "echo /usr/local/Xbin", "echo X/usr/local/bin"},
		{"word motion over an underscore", "echo foo_bar\x1bbX\r", "echo foo_Xbar", "echo Xfoo_bar"},
		{"a character in neither list", "echo a+b\x1bbX\r", "echo a+Xb", "echo a+Xb"},
		{"the word kill before the cursor", "echo a+b\x17X\r", "echo X", "echo a+X"},
		{"the word kill after the cursor", "echo /usr/bin x\x1b[H\x1bf\x06\x1bdX\r", "echo X/bin x", "echo /X x"},
		{"the word kill with meta delete", "echo /usr/local/bin\x1b\x7fX\r", "echo /usr/local/X", "echo X"},
		// A terminal whose erase character is ^H rather than Delete spells the
		// same key the other way, and both shells act on both.
		{"the word kill with meta backspace", "echo /usr/local/bin\x1b\x08X\r", "echo /usr/local/X", "echo X"},
		// Where the forward motion stops.
		{"forward a word", "echo one two\x1b[H\x1bfX\r", "echoX one two", "echo Xone two"},
		// What is left of the line after killing to the start of it.
		{"kill to the start from the middle", "echo one two\x1bb\x15X\r", "Xtwo", "X"},
		{"kill to the start from the start", "echo one two\x1b[H\x15X\r", "Xecho one two", "X"},
		// And transposing where there is nothing in front of the cursor.
		{"transpose at the start", "echo abc\x1b[H\x14X\r", "Xecho abc", "ceXho abc"},
		// And what that kill leaves for ^Y: a line one of them did not take
		// is a line the other can put back.
		{"what the kill to the start puts back", "echo one two\x1b[H\x15\x19X\r", "Xecho one two", "echo one twoX"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := typedStyled(t, zeroAnswers, c.keys); got != c.zero {
				t.Errorf("the zero answers gave %q, want %q", got, c.zero)
			}
			if got := typedStyled(t, otherAnswers, c.keys); got != c.other {
				t.Errorf("the other answers gave %q, want %q", got, c.other)
			}
		})
	}
}

// What the kills leave behind for ^Y, which is the half of a kill that makes
// it safe to press.
func TestWhatIsKilledCanBePutBack(t *testing.T) {
	for _, c := range []struct{ name, keys, want string }{
		{"a word", "echo one two\x17\x19\r", "echo one two"},
		{"to the end of the line", "echo one two\x1bb\x0b\x19\r", "echo one two"},
		{"to the start of the line", "echo one two\x1bb\x15\x19\r", "echo one two"},
		{"a word after the cursor", "echo one two\x1b[H\x1bd\x19\r", "echo one two"},
		// Two kills in a row are one piece of text, in the order the words
		// were on the line rather than the order they were killed in.
		{"two word kills in a row", "echo one two\x17\x17\x19\r", "echo one two"},
		{"a backward kill then a forward one", "echo one two\x1bb\x15\x0b\x19\r", "echo one two"},
		// A keystroke that is not a kill starts the next one afresh.
		{"a kill, a keystroke, then a kill", "echo one two\x17Z\x17\x19\r", "echo one Z"},
		{"a kill, a move, then a kill", "echo one two\x17\x02\x06\x17\x19\r", "echo one "},
		// Yanking does not empty it, and killing nothing does not either.
		{"yanked twice", "echo one two\x17\x19\x19\r", "echo one twotwo"},
		{"a kill of nothing leaves it alone", "echo one two\x17\x0b\x19\r", "echo one two"},
		// And does not join the kills either side of it. Measured: bash
		// starts afresh here and zsh joins; this is bash's answer, and the
		// one disagreement about editing that is not a field.
		{"a kill of nothing does not join", "echo one two\x17\x0b\x17\x19\r", "echo one "},
		// Nor does a word kill with nothing in front of the cursor throw away
		// what is there to put back.
		{"a word kill at the start of the line", "echo one two\x17\x01\x17\x19\r", "twoecho one "},
		// With nothing killed there is nothing to put back.
		{"nothing killed", "echo one two\x19\r", "echo one two"},
		// The cursor ends up after what was put back, not in front of it.
		{"the cursor follows the yank", "echo one two\x17\x19!\r", "echo one two!"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := typedStyled(t, zeroAnswers, c.keys); got != c.want {
				t.Errorf("gave %q, want %q", got, c.want)
			}
		})
	}
}

// The kill outlives the line it came off, and the joining does not.
//
// Measured: a word killed on one line yanks back on the next, and a kill on
// each of two lines is two pieces rather than one.
func TestAKillSurvivesTheLineItCameFrom(t *testing.T) {
	var out strings.Builder
	e := Shell{Editor: zeroAnswers}.newEditor()
	e.out = &out

	e.in = strings.NewReader("echo one two\x17\r")
	if line, err := e.readLine(drawPrompt("$ ")); err != nil || line != "echo one " {
		t.Fatalf("first line gave %q %v", line, err)
	}
	e.in = strings.NewReader("mv \x19\r")
	if line, err := e.readLine(drawPrompt("$ ")); err != nil || line != "mv two" {
		t.Errorf("second line gave %q %v, want the kill from the first line back", line, err)
	}
	// A kill on this line does not join onto the one from the last.
	e.in = strings.NewReader("cp here\x17\x19\r")
	if line, err := e.readLine(drawPrompt("$ ")); err != nil || line != "cp here" {
		t.Errorf("third line gave %q %v, want only its own kill back", line, err)
	}
}

// ^T swaps the two characters around the cursor and steps past them.
func TestTransposingCharacters(t *testing.T) {
	for _, c := range []struct{ name, keys, want string }{
		{"at the end of the line", "echo ab\x14\r", "echo ba"},
		{"in the middle", "echo abc\x02\x14\r", "echo acb"},
		{"and the cursor moves on", "echo abcd\x02\x02\x14\x14\r", "echo acdb"},
		{"a line of one character", "a\x14\r", "a"},
		// At the end the cursor stays where it was, which is the end.
		{"the cursor stays at the end", "echo ab\x14X\r", "echo baX"},
		{"an empty line", "\x14\r", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := typedStyled(t, zeroAnswers, c.keys); got != c.want {
				t.Errorf("gave %q, want %q", got, c.want)
			}
		})
	}
}

// The word keys count a wide character as one character, because it is one.
//
// The width machinery is about the screen; a word is about the text. Measured:
// `M-b` twice on `echo 日本語 x` puts the cursor in front of the whole of
// `日本語`, so the three of them are one word and not three.
func TestWordsMadeOfWideCharacters(t *testing.T) {
	for _, c := range []struct{ name, keys, want string }{
		{"a word of wide characters", "echo 日本語 x\x1bb\x1bbX\r", "echo X日本語 x"},
		{"killing one", "echo 日本語\x17X\r", "echo X"},
		{"stepping over one", "echo 日本語\x02X\r", "echo 日本X語"},
		{"transposing two", "echo 日本\x14\r", "echo 本日"},
		// One keystroke removes the whole character, not a byte of it.
		{"backspacing over one", "echo 日本\x7fX\r", "echo 日X"},
		{"backspacing over an emoji", "echo 👍a\x7f\x7fX\r", "echo X"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := typedStyled(t, zeroAnswers, c.keys); got != c.want {
				t.Errorf("gave %q, want %q", got, c.want)
			}
		})
	}
}

// A modified arrow is not the arrow.
//
// Measured: bash does nothing at all for Ctrl-Up, where the bare Up steps
// back through the history. A modifier that fell through to the plain key
// would jump the line out from under a finger that meant to select text.
func TestAModifiedUpArrowIsNotTheUpArrow(t *testing.T) {
	var out strings.Builder
	e := Shell{}.newEditor()
	e.out = &out
	e.remember("earlier")

	e.in = strings.NewReader("x\x1b[1;5A\r")
	if line, err := e.readLine(drawPrompt("$ ")); err != nil || line != "x" {
		t.Errorf("Ctrl-Up gave %q %v, want the line untouched", line, err)
	}
	e.in = strings.NewReader("x\x1b[A\r")
	if line, err := e.readLine(drawPrompt("$ ")); err != nil || line != "earlier" {
		t.Errorf("the bare arrow gave %q %v, want the line from the history", line, err)
	}
}
