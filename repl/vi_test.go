// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// vi command mode, against the three shells that have one.
//
// Measured under a pseudo-terminal on 2026-09-12 against bash 5.3.15, bash
// 3.2.57 and zsh 5.9.2, started with no startup files and put into vi editing
// at the first prompt — `set -o vi` in bash, `bindkey -v` in zsh.
//
// **The cursor is read back rather than reasoned about.** Every row is the
// same shape: type a line, press Escape, press the keys under test, then `i`
// and an `X`, then Return. `fc -ln -1` in the real shell prints the line it
// accepted, so the `X` says exactly which character the cursor was on and the
// rest of the line says exactly what the edit did. The `want` columns below
// are those lines, byte for byte. docs/spec/editing.md holds the table in
// prose and the four places the shells disagree.

// b1 and b2 are the lines the table is measured on: four alphabetic words, and
// one with punctuation inside a word, which is where `w` and `W` part company.
const (
	b1 = "true alpha beta gamma"
	b2 = "true a-b.c def"
)

// viTyped runs one line through an editor that edits the vi way.
//
// Through newEditor and the Shell, the way bindings_test does, so that the
// wiring is exercised rather than the mode alone: a session gets a command
// mode by the front end answering ViEditing and by nothing else.
func viTyped(t *testing.T, style EditorStyle, keys string) string {
	t.Helper()
	var out strings.Builder
	e := Shell{
		ViEditing: func() bool { return true },
		Editor:    style,
	}.newEditor(t.Context(), nil)
	e.in, e.out = typing(keys), &out
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("%q: %v", keys, err)
	}
	return line
}

// vi is one measured row: a line typed, Escape, the keys under test, and then
// `i` and an `X` unless the keys have already reached insert mode.
type vi struct {
	name string
	base string
	keys string
	// mark is what reveals the cursor. Empty is `iX`, which is what a row
	// that ends in command mode needs; a row that ends in insert mode marks
	// with `X` alone.
	mark string
	want string
}

func (c vi) run(t *testing.T, style EditorStyle) string {
	t.Helper()
	mark := c.mark
	if mark == "" {
		mark = "iX"
	}
	return viTyped(t, style, c.base+"\x1b"+c.keys+mark+"\n")
}

// TestTheCommandModeTheThreeShellsAgreeAbout is the core of the mode: every
// row here is the same in bash 5.3.15, bash 3.2.57 and zsh 5.9.2, so a
// disagreement with it is a disagreement with all three at once.
func TestTheCommandModeTheThreeShellsAgreeAbout(t *testing.T) {
	for _, c := range []vi{
		// Leaving insert mode steps the cursor back one, and at the start of
		// the line there is nowhere to step back to.
		{name: "escape steps back", base: b1, want: "true alpha beta gammXa"},
		{name: "escape at the start", base: "t", want: "Xt"},
		{name: "escape twice", base: b1, keys: "\x1b", want: "true alpha beta gammXa"},

		// Character motions.
		{name: "0", base: b1, keys: "0", want: "Xtrue alpha beta gamma"},
		{name: "^", base: "   " + b1, keys: "^", want: "   Xtrue alpha beta gamma"},
		{name: "^ with no indent", base: b1, keys: "$^", want: "Xtrue alpha beta gamma"},
		{name: "$", base: b1, keys: "0$", want: "true alpha beta gammXa"},
		{name: "l", base: b1, keys: "0l", want: "tXrue alpha beta gamma"},
		{name: "3l", base: b1, keys: "03l", want: "truXe alpha beta gamma"},
		{name: "10l", base: b1, keys: "010l", want: "true alphaX beta gamma"},
		{name: "space is l", base: b1, keys: "0 ", want: "tXrue alpha beta gamma"},
		{name: "h", base: b1, keys: "$h", want: "true alpha beta gamXma"},
		{name: "2h", base: b1, keys: "$2h", want: "true alpha beta gaXmma"},
		{name: "l stops at the last character", base: b1, keys: "$lll", want: "true alpha beta gammXa"},
		{name: "h stops at the first", base: b1, keys: "0hhh", want: "Xtrue alpha beta gamma"},
		{name: "3|", base: b1, keys: "$3|", want: "trXue alpha beta gamma"},

		// Word motions. A word is a run of letters, digits and `_`, or a run
		// of anything else that is not a blank; `W` and its fellows know only
		// blanks.
		{name: "w", base: b1, keys: "0w", want: "true Xalpha beta gamma"},
		{name: "3w", base: b1, keys: "03w", want: "true alpha beta Xgamma"},
		{name: "w over the blanks", base: "true   alpha", keys: "0w", want: "true   Xalpha"},
		{name: "w stops at punctuation", base: b2, keys: "0ww", want: "true aX-b.c def"},
		{name: "W does not", base: b2, keys: "0WW", want: "true a-b.c Xdef"},
		{name: "_ is inside a word", base: "true a_b cd", keys: "0ww", want: "true a_b Xcd"},
		{name: "$ is not", base: "true a$b cd", keys: "0ww", want: "true aX$b cd"},
		{name: "w at the last word", base: b1, keys: "$w", want: "true alpha beta gammXa"},
		{name: "b", base: b1, keys: "$b", want: "true alpha beta Xgamma"},
		{name: "2b", base: b1, keys: "$2b", want: "true alpha Xbeta gamma"},
		{name: "b from a word start", base: b1, keys: "0wwb", want: "true Xalpha beta gamma"},
		{name: "e", base: b1, keys: "0e", want: "truXe alpha beta gamma"},
		{name: "2e", base: b1, keys: "02e", want: "true alphXa beta gamma"},

		// Find, and repeating one.
		{name: "fa", base: b1, keys: "0fa", want: "true Xalpha beta gamma"},
		{name: "2fa", base: b1, keys: "02fa", want: "true alphXa beta gamma"},
		{name: "Fa", base: b1, keys: "$Fa", want: "true alpha beta gXamma"},
		{name: "ta", base: b1, keys: "0ta", want: "trueX alpha beta gamma"},
		{name: "Ta", base: b1, keys: "$Ta", want: "true alpha beta gaXmma"},
		{name: "; repeats", base: b1, keys: "0fa;", want: "true alphXa beta gamma"},
		{name: ", reverses", base: b1, keys: "0fa;;,", want: "true alphXa beta gamma"},
		{name: "a find of nothing does nothing", base: b1, keys: "0fz", want: "Xtrue alpha beta gamma"},

		// Edits that are one key.
		{name: "x", base: b1, keys: "0x", want: "Xrue alpha beta gamma"},
		{name: "3x", base: b1, keys: "03x", want: "Xe alpha beta gamma"},
		{name: "x at the end", base: b1, keys: "$x", want: "true alpha beta gamXm"},
		{name: "x past the end", base: "abc", keys: "0l9x", want: "Xa"},
		{name: "X", base: b1, keys: "$X", want: "true alpha beta gamXa"},
		{name: "r", base: b1, keys: "0rZ", want: "XZrue alpha beta gamma"},
		{name: "2r", base: b1, keys: "02rZ", want: "ZXZue alpha beta gamma"},
		{name: "r then Escape changes nothing", base: b1, keys: "0r\x1b", want: "Xtrue alpha beta gamma"},
		{name: "~", base: b1, keys: "0~", want: "TXrue alpha beta gamma"},
		{name: "3~", base: b1, keys: "03~", want: "TRUXe alpha beta gamma"},

		// An operator and a motion.
		{name: "dw", base: b1, keys: "0dw", want: "Xalpha beta gamma"},
		{name: "d2w", base: b1, keys: "0d2w", want: "Xbeta gamma"},
		{name: "2dw is the same", base: b1, keys: "02dw", want: "Xbeta gamma"},
		{name: "dw over the blanks", base: "true   alpha", keys: "0dw", want: "Xalpha"},
		{name: "dw at the last word takes the rest", base: b1, keys: "$dw", want: "true alpha beta gamXm"},
		{name: "dW", base: b2, keys: "0dW", want: "Xa-b.c def"},
		{name: "db", base: b1, keys: "$db", want: "true alpha beta Xa"},
		{name: "de", base: b1, keys: "0de", want: "X alpha beta gamma"},
		{name: "d$", base: b1, keys: "0ld$", want: "Xt"},
		{name: "d0", base: b1, keys: "$d0", want: "Xa"},
		{name: "df", base: b1, keys: "0dfa", want: "Xlpha beta gamma"},
		{name: "dd", base: b1, keys: "0dd", want: "X"},
		{name: "D", base: b1, keys: "0lD", want: "Xt"},
		{name: "d past the end", base: "abc", keys: "0l9dw", want: "Xa"},
		{name: "d then a key that is not a motion", base: b1, keys: "0dz", want: "Xtrue alpha beta gamma"},
		{name: "d then Escape", base: b1, keys: "0d\x1b", want: "Xtrue alpha beta gamma"},

		// `c` is `d` and then insert mode, and `cw` is `ce`.
		{name: "cw is ce", base: b1, keys: "0cw", mark: "X", want: "X alpha beta gamma"},
		{name: "cw inside a word", base: b1, keys: "0lcw", mark: "X", want: "tX alpha beta gamma"},
		{name: "cw on the last character", base: b1, keys: "$cw", mark: "X", want: "true alpha beta gammX"},
		{name: "c$", base: b1, keys: "0lc$", mark: "X", want: "tX"},
		{name: "cc", base: b1, keys: "0cc", mark: "X", want: "X"},
		{name: "C", base: b1, keys: "0lC", mark: "X", want: "tX"},
		{name: "S", base: b1, keys: "0S", mark: "X", want: "X"},

		// Yank, and putting back.
		{name: "yw then p", base: b1, keys: "0yw$p", want: "true alpha beta gammatrueX "},
		{name: "yw then P", base: b1, keys: "0yw$P", want: "true alpha beta gammtrueX a"},
		{name: "yw then P at the start", base: b1, keys: "0yw0P", want: "trueX true alpha beta gamma"},
		{name: "dw then p", base: b1, keys: "0dw$p", want: "alpha beta gammatrueX "},
		{name: "x then p swaps", base: b1, keys: "0xp", want: "rXtue alpha beta gamma"},

		// Getting back to insert mode.
		{name: "i", base: b1, keys: "0", want: "Xtrue alpha beta gamma"},
		{name: "a", base: b1, keys: "0", mark: "aX", want: "tXrue alpha beta gamma"},
		{name: "A", base: b1, keys: "0", mark: "AX", want: "true alpha beta gammaX"},
		{name: "a at the end", base: b1, keys: "$", mark: "aX", want: "true alpha beta gammaX"},

		// A key command mode has nothing on does nothing at all, and above all
		// does not type itself into the line.
		{name: "an unbound letter", base: b1, keys: "0z", want: "Xtrue alpha beta gamma"},
		{name: "another", base: b1, keys: "0q", want: "Xtrue alpha beta gamma"},

		// An empty line has one cursor position and every motion stays on it.
		{name: "w on an empty line", base: "", keys: "w", want: "X"},
		{name: "x on an empty line", base: "", keys: "x", want: "X"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, style := range []struct {
				shell string
				EditorStyle
			}{
				{shell: "bash", EditorStyle: EditorStyle{}},
				{shell: "zsh", EditorStyle: zshAnswers()},
			} {
				if got := c.run(t, style.EditorStyle); got != c.want {
					t.Errorf("%s: line = %q, want %q", style.shell, got, c.want)
				}
			}
		})
	}
}

// zshAnswers is the other shell's side of every editing question, so that a
// row above is checked against both dialects' answers rather than against the
// zero value alone. A key whose behaviour one of these fields changes belongs
// in the disagreement test below and not in the table.
func zshAnswers() EditorStyle {
	return EditorStyle{
		WordCharacters:                          "*?_-.[]~=/&;!#$%^(){}<>",
		KillToStartOfLineTakesTheWholeLine:      true,
		KillWordBeforeCursorUsesWordCharacters:  true,
		ForwardWordStopsBeforeTheNextWord:       true,
		TransposeAtTheStartSwapsTheFirstTwo:     true,
		UndoTakesBackOneKeystrokeAtATime:        true,
		UndoRestoresTheCursorToWhereItWas:       true,
		LastArgumentStaysOnTheOldestLine:        true,
		ViInsertAtStartOfLineSkipsLeadingBlanks: true,
	}
}

// TestWhereTheVWordMotionsIgnoreTheDialect is the one rule in the mode that is
// deliberately not a dialect's answer.
//
// Every other word key in this editor asks a dialect what a word is, because
// the two shells disagree and one of them disagrees with itself. The vi
// motions ask nobody: measured, `w` lands on the `-` of `a-b.c` in both
// shells, although zsh's WORDCHARS holds `-` and its own `M-f` walks straight
// past it.
func TestTheViWordMotionsIgnoreTheDialectsWordCharacters(t *testing.T) {
	// `true a-b.c`, where the whole of `a-b.c` is one word to zsh and three to
	// vi. From the start, two `w` land on the `-` under either dialect's
	// answers.
	const line = "true a-b.c"
	const want = "true aX-b.c"
	for _, style := range []struct {
		shell string
		EditorStyle
	}{
		{shell: "bash", EditorStyle: EditorStyle{}},
		{shell: "zsh", EditorStyle: zshAnswers()},
	} {
		if got := viTyped(t, style.EditorStyle, line+"\x1b0wwiX\n"); got != want {
			t.Errorf("%s: vi ww = %q, want %q", style.shell, got, want)
		}
	}
	// And `M-b` on the same line still reads the field, or the rows above are
	// passing because nothing reads it at all: zsh's WORDCHARS put the whole
	// of `a-b.c` in one word and bash's answer stops at the `c`.
	if got, want := typedStyle(t, zshAnswers(), line+"\x1bbX\n"), "true Xa-b.c"; got != want {
		t.Errorf("M-b under zsh's answers = %q, want %q", got, want)
	}
	if got, want := typedStyle(t, EditorStyle{}, line+"\x1bbX\n"), "true a-b.Xc"; got != want {
		t.Errorf("M-b under bash's answers = %q, want %q", got, want)
	}
}

// typedStyle runs a line through an editor with a dialect's answers and no vi
// editing, which is what every session was before this.
func typedStyle(t *testing.T, style EditorStyle, keys string) string {
	t.Helper()
	var out strings.Builder
	e := Shell{Editor: style}.newEditor(t.Context(), nil)
	e.in, e.out = typing(keys), &out
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("%q: %v", keys, err)
	}
	return line
}

// TestWhereTheCommandModeDependsOnTheDialect is the one key the two shells
// answer differently, and the undo they answer differently for the same reason
// their `^_` does.
func TestWhereTheCommandModeDependsOnTheDialect(t *testing.T) {
	for _, c := range []struct {
		name, keys, bash, zsh string
	}{
		// `I` inserts at column 0 in bash and at the first character that is
		// not a blank in zsh. Both agree about `^`, which is the same place
		// zsh's `I` goes.
		{
			name: "I and the indent", keys: "   ab\x1b$IX\n",
			bash: "X   ab", zsh: "   Xab",
		},
		// And where the cursor lands after `u`, which is the same
		// disagreement `^_` has: zsh puts it back where the change was made
		// and bash puts it after the text the undo restored.
		{
			name: "u after x", keys: b1 + "\x1b0xuiX\n",
			bash: "tXrue alpha beta gamma", zsh: "Xtrue alpha beta gamma",
		},
		{
			name: "u after dw", keys: b1 + "\x1b0dwuiX\n",
			bash: "true Xalpha beta gamma", zsh: "Xtrue alpha beta gamma",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := viTyped(t, EditorStyle{}, c.keys); got != c.bash {
				t.Errorf("bash: line = %q, want %q", got, c.bash)
			}
			if got := viTyped(t, zshAnswers(), c.keys); got != c.zsh {
				t.Errorf("zsh: line = %q, want %q", got, c.zsh)
			}
		})
	}
}

// TestReturnFromCommandModeAcceptsTheLine — measured, and it is the key that
// makes the mode usable at all: a person who has just fixed a word does not
// press `A` first.
func TestReturnFromCommandModeAcceptsTheLine(t *testing.T) {
	if got, want := viTyped(t, EditorStyle{}, b1+"\x1b0\r"), b1; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

// TestCommandModeLastsOneLine — a prompt takes typing, whatever state the line
// before it ended in.
func TestCommandModeLastsOneLine(t *testing.T) {
	var out strings.Builder
	e := Shell{ViEditing: func() bool { return true }}.newEditor(t.Context(), nil)
	e.in, e.out = typing("one\x1b0\rtwo\n"), &out
	if got, err := e.readLine(drawPrompt("$ ")); err != nil || got != "one" {
		t.Fatalf("first line = %q, %v", got, err)
	}
	if got, err := e.readLine(drawPrompt("$ ")); err != nil || got != "two" {
		t.Errorf("second line = %q, %v, want it to take typing", got, err)
	}
}

// TestWithoutViEditingEscapeIsStillTheMetaPrefix is the regression this whole
// change had to avoid: a session that never asked for vi editing must read
// Escape exactly as it did before.
func TestWithoutViEditingEscapeIsStillTheMetaPrefix(t *testing.T) {
	// `M-b` on `echo one two`, then `X`. In emacs mode that is a word back.
	line, _, err := typed(t, "echo one two\x1bbX\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := "echo one Xtwo"; line != want {
		t.Errorf("line = %q, want %q", line, want)
	}
	// And with vi editing the same bytes leave insert mode, walk a word back
	// and then delete the character before the cursor — which is what bash
	// does with them, measured: `X` in command mode is a backward delete.
	if got, want := viTyped(t, EditorStyle{}, "echo one two\x1bbX\n"), "echo onetwo"; got != want {
		t.Errorf("vi: line = %q, want %q", got, want)
	}
}

// TestAnEscapeSequenceIsStillReadWholeInViMode is the half of the mode with
// nothing to copy from: Escape is the mode switch and also the first byte of
// every arrow key.
//
// Both real shells tell the two apart with a timer — measured, `\e[D` typed as
// one burst moves the cursor left in both, and the same three bytes with 1.2
// seconds after the Escape leave insert mode and then read `[` and `D` as two
// command-mode keys, `D` deleting to the end of the line. This editor asks
// whether a byte is *there* instead; see escapeIsTheModeSwitch.
//
// So the two readers below stand for the two cases. A reader that delivers the
// whole burst is a terminal writing a key sequence, and one byte per read with
// no descriptor to ask is a person pressing Escape on its own.
func TestAnEscapeSequenceIsStillReadWholeInViMode(t *testing.T) {
	var out strings.Builder
	e := Shell{ViEditing: func() bool { return true }}.newEditor(t.Context(), nil)
	e.in, e.out = strings.NewReader("abcd\x1b[DX\n"), &out
	got, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatal(err)
	}
	if want := "abcXd"; got != want {
		t.Errorf("in one burst: line = %q, want %q — the arrow was read as a mode switch", got, want)
	}
	// The same bytes a keystroke at a time, which is the Escape a person
	// pressed: command mode, `[` on nothing, `D` to the end of the line, and
	// `X` deleting the character before the cursor.
	if got, want := viTyped(t, EditorStyle{}, "abcd\x1b[DX\n"), "ac"; got != want {
		t.Errorf("a byte at a time: line = %q, want %q", got, want)
	}
}

// TestABindingInTheCommandKeymapFires is the structural half of this change,
// and the thing that was broken before it.
//
// Both shells already stored a binding written into their command keymap and
// neither could ever run it, because the editor had one keymap and it was the
// other one. Measured under a pty, in both shells: `^Xz` bound to
// `beginning-of-line` in the command map moves the cursor in command mode, and
// in insert mode the same key does nothing.
func TestABindingInTheCommandKeymapFires(t *testing.T) {
	table := func(km Keymap) map[string]Binding {
		if km == KeymapViCommand {
			return map[string]Binding{"\x18z": {Widget: WidgetBeginningOfLine}}
		}
		return nil
	}
	var out strings.Builder
	e := Shell{
		ViEditing:   func() bool { return true },
		KeyBindings: table,
	}.newEditor(t.Context(), nil)
	e.in, e.out = typing(b1+"\x1b\x18ziX\n"), &out
	got, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatal(err)
	}
	if want := "Xtrue alpha beta gamma"; got != want {
		t.Errorf("in command mode: line = %q, want %q", got, want)
	}
	// And the same key while typing reaches the insert map, which has nothing
	// on it — so the cursor is where typing left it.
	var out2 strings.Builder
	e2 := Shell{ViEditing: func() bool { return true }, KeyBindings: table}.newEditor(t.Context(), nil)
	e2.in, e2.out = typing(b1+"\x18z\x1biX\n"), &out2
	got, err = e2.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatal(err)
	}
	if want := "true alpha beta gammXa"; got != want {
		t.Errorf("in insert mode: line = %q, want %q", got, want)
	}
}

// TestTheThreeModeWidgets pins the vi-only actions repl names, which are how a
// person rebinds their way between the two states — `bindkey -M viins jk
// vi-cmd-mode` and its counterparts. See widgets.go for why these three and
// not the motions.
func TestTheThreeModeWidgets(t *testing.T) {
	for _, c := range []struct {
		name string
		w    Widget
		keys string
		want string
	}{
		// ^G leaves insert mode, and then `0` and `i` are command-mode keys.
		{name: "command mode", w: WidgetViCommandMode, keys: b1 + "\a0iX\n", want: "Xtrue alpha beta gamma"},
		// From command mode, ^G puts the editor back into insert at the
		// cursor, so the `0` after it is typed rather than a motion.
		{name: "insert mode", w: WidgetViInsertMode, keys: b1 + "\x1b0\a0X\n", want: "0Xtrue alpha beta gamma"},
		{name: "append mode", w: WidgetViAppendMode, keys: b1 + "\x1b0\a0X\n", want: "t0Xrue alpha beta gamma"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out strings.Builder
			e := Shell{
				ViEditing:   func() bool { return true },
				KeyBindings: func(Keymap) map[string]Binding { return map[string]Binding{"\a": {Widget: c.w}} },
			}.newEditor(t.Context(), nil)
			e.in, e.out = typing(c.keys), &out
			got, err := e.readLine(drawPrompt("$ "))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("line = %q, want %q", got, c.want)
			}
		})
	}
}

// TestCommandModeAbandonsOnInterrupt — ^C gives the line up wherever the
// editor is, which is what makes a half-typed command mode escapable.
func TestCommandModeAbandonsOnInterrupt(t *testing.T) {
	var out strings.Builder
	e := Shell{ViEditing: func() bool { return true }}.newEditor(t.Context(), nil)
	e.in, e.out = typing(b1+"\x1b0\x03"), &out
	if _, err := e.readLine(drawPrompt("$ ")); err != ErrInterrupted {
		t.Errorf("err = %v, want %v", err, ErrInterrupted)
	}
	// And part-way through a command, which is the wedge escape.go describes.
	var out2 strings.Builder
	e2 := Shell{ViEditing: func() bool { return true }}.newEditor(t.Context(), nil)
	e2.in, e2.out = typing(b1+"\x1b0d\x03"), &out2
	if _, err := e2.readLine(drawPrompt("$ ")); err != ErrInterrupted {
		t.Errorf("mid-command: err = %v, want %v", err, ErrInterrupted)
	}
}
