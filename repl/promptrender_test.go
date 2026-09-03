// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"testing"
	"time"
)

func at(t *testing.T, hour, min, sec int) func() time.Time {
	t.Helper()
	return func() time.Time {
		return time.Date(2026, time.September, 2, hour, min, sec, 0, time.UTC)
	}
}

// What each code draws, with everything it reads supplied.
func TestWhatACodeDraws(t *testing.T) {
	vars := map[string]string{
		"USER":     "someone",
		"HOSTNAME": "machine.example.com",
		"HOME":     "/home/someone",
		"PWD":      "/home/someone/work/deep",
	}
	style := PromptStyle{
		Escape:    '\\',
		Privilege: "$",
		Codes: map[rune]PromptField{
			'u': FieldUser, 'h': FieldHost, 'H': FieldHostFull,
			'w': FieldCwd, 'W': FieldCwdBase, 'D': FieldCwdFull,
			's': FieldShellName, '$': FieldPrivilege,
			'n': FieldNewline, 'r': FieldReturn, 'i': FieldTab,
			'\\': FieldEscape,
			't':  FieldTime24, 'T': FieldTime12, 'A': FieldTime24HM,
			'@': FieldTime12AMPM, 'p': FieldTime12Padded,
			'd': FieldDate, 'e': FieldDateShort,
		},
	}
	s := Shell{
		Runner: newTestRunner(vars),
		Style:  style,
		Name:   "/usr/local/bin/testsh",
		Clock:  at(t, 21, 5, 9),
	}
	for _, tc := range []struct{ code, want string }{
		{`\u`, "someone"},
		{`\h`, "machine"},
		{`\H`, "machine.example.com"},
		{`\w`, "~/work/deep"},
		{`\W`, "deep"},
		{`\D`, "/home/someone/work/deep"},
		// The basename: a shell invoked by a path calls itself by its name.
		{`\s`, "testsh"},
		{`\$`, "$"},
		{`\n`, "\r\n"},
		{`\r`, "\r"},
		{`\i`, "\t"},
		{`\\`, `\`},
		{`\t`, "21:05:09"},
		{`\T`, "09:05:09"},
		{`\A`, "21:05"},
		{`\@`, "09:05 PM"},
		// zsh pads the hour with a space below ten and not at or above it.
		{`\p`, " 9:05PM"},
		{`\d`, "Wed Sep 02"},
		{`\e`, "Wed 2"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			if got := s.escapes(tc.code); got != tc.want {
				t.Errorf("%s drew %q, want %q", tc.code, got, tc.want)
			}
		})
	}
	t.Run("the padded hour at ten", func(t *testing.T) {
		ten := s
		ten.Clock = at(t, 22, 9, 0)
		if got := ten.escapes(`\p`); got != "10:09PM" {
			t.Errorf("drew %q, want 10:09PM with no padding", got)
		}
	})
}

// A code with no entry in the table gets one of three answers, and the panel
// gives all three: bash draws `\q` for `\q`, ksh93 draws `q`, zsh draws
// nothing at all for `%q`.
func TestACodeThatIsNotInTheTable(t *testing.T) {
	for _, tc := range []struct {
		name string
		unk  UnknownCode
		want string
	}{
		{"kept whole", KeepBoth, `<\q>`},
		{"escape dropped", DropEscape, "<q>"},
		{"both dropped", DropBoth, "<>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Shell{Style: PromptStyle{Escape: '\\', Unknown: tc.unk}}
			if got := s.escapes(`<\q>`); got != tc.want {
				t.Errorf("drew %q, want %q", got, tc.want)
			}
		})
	}
}

// An escape with nothing after it is the character itself: there is no code to
// look up, so none of the three answers applies.
func TestAnEscapeAtTheEnd(t *testing.T) {
	for _, unk := range []UnknownCode{KeepBoth, DropEscape, DropBoth} {
		s := Shell{Style: PromptStyle{Escape: '\\', Unknown: unk}}
		if got := s.escapes(`x\`); got != `x\` {
			t.Errorf("Unknown %v drew %q, want the character itself", unk, got)
		}
	}
}

// With no escape language the text is the text.
func TestNoEscapeLanguage(t *testing.T) {
	s := Shell{Style: PromptStyle{}}
	if got := s.escapes(`<\u %n>`); got != `<\u %n>` {
		t.Errorf("drew %q, want it untouched", got)
	}
}

// The table is read before expansion, which is measurable rather than a
// detail: with `x='\u'` set, bash draws `$x` as the two characters and not as
// the user name. A code that arrives through expansion is text.
func TestTheTableIsReadBeforeExpansion(t *testing.T) {
	s := Shell{
		Runner: newTestRunner(map[string]string{"USER": "someone", "x": `\u`}),
		Style: PromptStyle{
			Expand: true, Escape: '\\',
			Codes: map[rune]PromptField{'u': FieldUser},
		},
	}
	if got := s.render("<$x>"); got != `<\u>` {
		t.Errorf("drew %q, want the escape to have arrived too late to count", got)
	}
	// And one written in the prompt itself is a code.
	if got := s.render(`<\u>`); got != "<someone>" {
		t.Errorf("drew %q, want <someone>", got)
	}
}

// The home directory is written `~` when the directory is it or is inside it,
// and not when it merely starts with the same letters.
func TestAbbreviatingTheHomeDirectory(t *testing.T) {
	for _, tc := range []struct{ dir, home, want string }{
		{"/home/someone", "/home/someone", "~"},
		{"/home/someone/work", "/home/someone", "~/work"},
		{"/home/someone2", "/home/someone", "/home/someone2"},
		{"/elsewhere", "/home/someone", "/elsewhere"},
		{"/home/someone", "", "/home/someone"},
		{"/", "/home/someone", "/"},
	} {
		if got := abbreviate(tc.dir, tc.home); got != tc.want {
			t.Errorf("abbreviate(%q, %q) = %q, want %q", tc.dir, tc.home, got, tc.want)
		}
	}
}

// A character that stands for the history number on its own.
//
// ksh93 alone has one, spelled `!`. Measured against it: `<!>` drew 1, 2 and
// 3 on successive prompts, `<!!>` drew `<!>`, and `<a!b>` drew `<a1b>`. bash,
// dash and zsh draw a bare `!` as a bare `!`.
func TestTheCharacterThatStandsForTheHistoryNumber(t *testing.T) {
	s := Shell{
		Runner: newTestRunner(nil),
		Style:  PromptStyle{History: '!', Escape: '\\', Unknown: DropEscape},
		counts: &counts{history: 6},
	}
	for _, tc := range []struct{ in, want string }{
		{"<!>", "<7>"},
		{"<a!b>", "<a7b>"},
		// Doubled is one of itself, which is the only way to put one in a
		// prompt that reads them.
		{"<!!>", "<!>"},
		{"<!!!>", "<!7>"},
		// Read after the table, not with it: ksh93 draws `\!` as the number,
		// which is the backslash being dropped and the `!` left behind being
		// read in the pass that follows. One pass would leave it alone.
		{`<\!>`, "<7>"},
		{`<\!!>`, "<!>"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			if got := s.escapes(tc.in); got != tc.want {
				t.Errorf("drew %q, want %q", got, tc.want)
			}
		})
	}
}

// And with no such character the text is the text.
func TestABareBangWithoutTheHabit(t *testing.T) {
	s := Shell{Runner: newTestRunner(nil), counts: &counts{history: 6}}
	if got := s.escapes("<!> <!!>"); got != "<!> <!!>" {
		t.Errorf("drew %q, want it untouched", got)
	}
}
