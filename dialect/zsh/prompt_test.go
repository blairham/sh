// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/repl"
)

// // Measured: with PS1='<$LOGNAME>@ ' exported, real zsh draws <$LOGNAME>@ —
// the four characters as they stand. It wants `setopt PROMPT_SUBST` before it
// will expand a prompt, which is why this is the one dialect here that says
// no.

func TestPromptStyle(t *testing.T) {
	if got := zsh.PromptStyle().Expand; got != false {
		t.Errorf("Expand = %v, want %v", got, false)
	}
}

// The table, as measured against real zsh.
func TestPromptCodes(t *testing.T) {
	st := zsh.PromptStyle()
	if st.Escape != '%' {
		t.Errorf("Escape = %q, want a percent sign", st.Escape)
	}
	if st.Unknown != repl.DropBoth {
		t.Errorf("Unknown = %v, want an unknown code removed", st.Unknown)
	}
	if st.Privilege != "%" {
		t.Errorf("Privilege = %q, want %%", st.Privilege)
	}
	for code, want := range map[rune]repl.PromptField{
		'n': repl.FieldUser, 'm': repl.FieldHost, 'M': repl.FieldHostFull,
		'~': repl.FieldCwd, 'd': repl.FieldCwdFull, '/': repl.FieldCwdFull,
		'c': repl.FieldCwdBase, '.': repl.FieldCwdBase, 'C': repl.FieldCwdBaseFull,
		'#': repl.FieldPrivilege, '%': repl.FieldEscape,
		't': repl.FieldTime12Padded, '@': repl.FieldTime12Padded,
		'*': repl.FieldTime24Unpadded, 'T': repl.FieldTime24HMUnpadded,
		'w': repl.FieldDateShort, 'W': repl.FieldDateMonthDayYear,
		'D': repl.FieldDateYearMonthDay, '?': repl.FieldExitStatus,
		'j': repl.FieldJobCount, '!': repl.FieldHistoryNumber,
		'h': repl.FieldHistoryNumber, 'y': repl.FieldTerminalName,
		'{': repl.FieldNonPrintingStart, '}': repl.FieldNonPrintingEnd,
	} {
		if got := st.Codes[code]; got != want {
			t.Errorf("%%%c drew field %v, want %v", code, got, want)
		}
	}
	// The clock is zsh's own and not bash's: measured at six in the morning,
	// `%*` drew 6:11:43 where bash's `\t` drew 06:11:40.
	if st.Codes['*'] == repl.FieldTime24 {
		t.Error("%* pads the hour, and zsh does not")
	}
	// `%c` and `%C` are two codes of one shell that disagree, which is what
	// says the abbreviated base and the plain one are two fields: in the home
	// directory itself the first drew `~` and the second drew its own name.
	if st.Codes['c'] == st.Codes['C'] {
		t.Error("the two directory-base codes draw the same field, and they differ at $HOME")
	}
}

// The visual codes, measured as the exact bytes each put on the wire.
func TestPromptSequences(t *testing.T) {
	st := zsh.PromptStyle()
	for code, want := range map[rune]string{
		'B': "\x1b[1m", 'b': "\x1b[0m", 'U': "\x1b[4m", 'u': "\x1b[24m",
		'S': "\x1b[7m", 's': "\x1b[27m", 'f': "\x1b[39m", 'k': "\x1b[49m",
		'E': "\x1b[K",
	} {
		if got := st.Sequences[code]; got != want {
			t.Errorf("%%%c drew %q, want %q", code, got, want)
		}
	}
	// `%b` turns everything off rather than only bold — measured `\e[0m`,
	// where `%u` was the matching `\e[24m` for underline.
	if st.Sequences['b'] == "\x1b[22m" {
		t.Error("the bold-off code turns off bold alone, and zsh turns off everything")
	}
	// zsh has no backslash language: the escape is a percent sign and the
	// backslash codes bash draws are text here.
	if _, ok := st.Sequences['e']; ok {
		t.Error("zsh has an entry for bash's \\e")
	}
	if st.Octal {
		t.Error("zsh reads three octal digits, and only bash does")
	}
}

// The two color codes, and which half of the screen each paints.
func TestPromptColors(t *testing.T) {
	c := zsh.PromptStyle().Colors
	if got, ok := c['F']; !ok || got != repl.Foreground {
		t.Errorf("%%F paints %v (present %v), want the foreground", got, ok)
	}
	if got, ok := c['K']; !ok || got != repl.Background {
		t.Errorf("%%K paints %v (present %v), want the background", got, ok)
	}
}

// zsh's defaults, as measured — including a continuation written the way zsh
// writes it, with a code that is not drawable yet.
func TestPromptDefaults(t *testing.T) {
	st := zsh.PromptStyle()
	if st.Default != "%m%# " {
		t.Errorf("default = %q, want %%m%%# — host and privilege", st.Default)
	}
	if st.DefaultContinued != "%_> " {
		t.Errorf("continuation = %q, want %%_> ", st.DefaultContinued)
	}
	// `%_` draws what the line is still inside, which is what makes the
	// default continuation say `for> ` inside a for loop.
	if st.Codes['_'] != repl.FieldOpenState {
		t.Errorf("%%_ draws %v, want the open state", st.Codes['_'])
	}
}

// A bare `!` is a bare `!` here: measured, only ksh93 reads it.
func TestNoHistoryCharacter(t *testing.T) {
	if got := zsh.PromptStyle().History; got != 0 {
		t.Errorf("History = %q, want none", got)
	}
}

// What zsh calls each thing a line can still be inside.
//
// Measured one construct at a time with PS2='[%_]'. The two properties that
// are not just a word: a clause stands in place of the construct it is inside
// — `if true` draws `if` and then `then` once `then` is typed, not `if then`
// — and an operator follows it, since `true &&` inside a `then` draws
// `then cmdand`.
func TestPromptOpenWords(t *testing.T) {
	w := zsh.PromptStyle().OpenWords
	for word, want := range map[string]string{
		"for": "for", "while": "while", "until": "until", "select": "select",
		"case": "case", "if": "if", "then": "then", "else": "else",
		"elif": "elif", "{": "cursh", "function": "function", "(": "subsh",
		"$(": "cmdsubst", "`": "bquote", "${": "braceparam", "<<": "heredoc",
		"'": "quote", `"`: "dquote", "|": "pipe", "&&": "cmdand", "||": "cmdor",
		// The two spellings of a bar are two words here, which is the only
		// surface on which they are distinguishable to the person typing:
		//
		//	% PS2='[%_]'
		//	% echo a |
		//	[pipe]cat
		//	% echo b |&
		//	[errpipe]cat
		"|&": "errpipe",
	} {
		if got := w[word].Text; got != want {
			t.Errorf("%q draws %q, want %q", word, got, want)
		}
	}
	for _, clause := range []string{"then", "else", "elif"} {
		if !w[clause].Replaces {
			t.Errorf("%q follows its construct, want it to stand in place of it", clause)
		}
	}
	for _, op := range []string{"|", "|&", "&&", "||"} {
		if w[op].Replaces {
			t.Errorf("%q stands in place of what it is inside, want it to follow", op)
		}
	}
	// A loop's `do` is drawn as nothing at all, and the loop stays: zsh draws
	// `while false` and then `do` alike as `while`.
	if _, drawn := w["do"]; drawn {
		t.Error("`do` has an entry, and zsh draws nothing for it")
	}
}
