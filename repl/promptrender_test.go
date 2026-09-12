// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"testing"
	"time"

	"github.com/blairham/sh/interp"
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
			if got := s.render(tc.code); got != tc.want {
				t.Errorf("%s drew %q, want %q", tc.code, got, tc.want)
			}
		})
	}
	t.Run("the padded hour at ten", func(t *testing.T) {
		ten := s
		ten.Clock = at(t, 22, 9, 0)
		if got := ten.render(`\p`); got != "10:09PM" {
			t.Errorf("drew %q, want 10:09PM with no padding", got)
		}
	})
}

// The fields that tell two shells' answers apart.
//
// Each pair here is one thing a prompt draws that the panel draws two ways, so
// a dialect cannot be given the other one without a prompt being wrong every
// time it is drawn.
func TestTheFieldsThePanelDisagreesAbout(t *testing.T) {
	vars := map[string]string{"HOME": "/home/someone", "PWD": "/home/someone"}
	style := PromptStyle{
		Escape: '\\',
		Codes: map[rune]PromptField{
			'W': FieldCwdBase, 'C': FieldCwdBaseFull,
			't': FieldTime24, 'u': FieldTime24Unpadded,
			'A': FieldTime24HM, 'T': FieldTime24HMUnpadded,
			'D': FieldDateMonthDayYear, 'd': FieldDateYearMonthDay,
			'?': FieldExitStatus,
		},
	}
	r := newTestRunner(vars)
	r.SetExitStatus(3)
	s := Shell{Runner: r, Style: style, Clock: at(t, 6, 11, 43)}
	for _, tc := range []struct{ code, want string }{
		// In the home directory itself the two last-component codes part
		// company: measured, bash's `\W` and zsh's `%c` drew `~` there where
		// zsh's `%C` drew the directory's own name.
		{`\W`, "~"},
		{`\C`, "someone"},
		// bash pads the hour and zsh does not.
		{`\t`, "06:11:43"},
		{`\u`, "6:11:43"},
		{`\A`, "06:11"},
		{`\T`, "6:11"},
		{`\D`, "09/02/26"},
		{`\d`, "26-09-02"},
		{`\?`, "3"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			if got := s.render(tc.code); got != tc.want {
				t.Errorf("%s drew %q, want %q", tc.code, got, tc.want)
			}
		})
	}
	// One directory down they agree, which is why the home directory is where
	// this had to be measured.
	s.Runner.SetVar("PWD", "/home/someone/work")
	for _, code := range []string{`\W`, `\C`} {
		if got := s.render(code); got != "work" {
			t.Errorf("%s drew %q below home, want work", code, got)
		}
	}
	// With no directory to name, neither draws one. A shell whose PWD has not
	// been set has no last component, and `.` is a directory rather than the
	// absence of one.
	nowhere := Shell{Runner: newTestRunner(nil), Style: style}
	for _, code := range []string{`\W`, `\C`} {
		if got := nowhere.render(code); got != "" {
			t.Errorf("%s drew %q with no directory, want nothing", code, got)
		}
	}
	// Midnight keeps its digit: an unpadded hour has the zero taken off
	// rather than replaced. Measured, zsh drew 0:17:07.
	midnight := s
	midnight.Clock = at(t, 0, 17, 7)
	if got := midnight.render(`\u`); got != "0:17:07" {
		t.Errorf("drew %q at midnight, want 0:17:07", got)
	}
}

// The count in front of a directory code reaches the *drawn* prompt.
//
// It did not, and nothing said so. This reader had four cases of its own for
// the directory codes — reading the same `PWD` and `HOME` out of the same
// Runner, and abbreviating them with a copy of the same helper — and the copy
// had no count in it. So `${(%%):-%2~}` written in a script was right while
// `PS1='%2~> '` drew the whole path, which is about the most common thing
// anyone puts in a hand-made prompt (#1699).
//
// The fix was to delete the copy and ask the Runner, so this test is the
// guard on the *fold* rather than on an arithmetic that is asserted in interp:
// what it proves is that the drawer and the script reach one implementation.
// Every want here is what the same style answers on the script side.
func TestACountReachesTheDrawnPrompt(t *testing.T) {
	vars := map[string]string{"HOME": "/home/someone", "PWD": "/home/someone/work/deep"}
	style := PromptStyle{
		Escape:          '%',
		NumericArgument: true,
		Codes: map[rune]PromptField{
			'~': FieldCwd, 'd': FieldCwdFull,
			'c': FieldCwdCounted, 'C': FieldCwdCountedFull,
			'W': FieldCwdBase,
		},
	}
	s := Shell{Runner: newTestRunner(vars), Style: style}
	for _, tc := range []struct{ code, want string }{
		{"%~", "~/work/deep"},
		{"%1~", "deep"},
		{"%2~", "work/deep"},
		{"%3~", "~/work/deep"},
		{"%-1~", "~"},
		{"%-2~", "~/work"},
		{"%2d", "work/deep"},
		{"%-1d", "/home"},
		// The counting pair, whose absent count is one rather than no limit.
		{"%c", "deep"},
		{"%2c", "work/deep"},
		{"%C", "deep"},
		{"%-1C", "/home"},
		// And bash's `\W`, which has no count at all and keeps its own field.
		{"%W", "deep"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			if got := s.render(tc.code); got != tc.want {
				t.Errorf("%s drew %q, want %q", tc.code, got, tc.want)
			}
		})
	}
}

// The three shapes a code can have that are not a field.
func TestACodeThatDrawsWhatTheDialectSays(t *testing.T) {
	s := Shell{
		Runner: newTestRunner(nil),
		Style: PromptStyle{
			Escape:    '\\',
			Sequences: map[rune]string{'e': "\x1b", 'a': "\a"},
			Colors:    map[rune]PromptColor{'F': Foreground, 'K': Background},
			Octal:     true,
			Unknown:   KeepBoth,
		},
	}
	for _, tc := range []struct{ in, want string }{
		// A fixed string the dialect named.
		{`\e[32m`, "\x1b[32m"},
		{`\a`, "\a"},
		// A color, and the argument it takes.
		{`\F{red}`, "\x1b[31m"},
		{`\K{blue}`, "\x1b[44m"},
		{`\F{9}`, "\x1b[91m"},
		{`\F{200}`, "\x1b[38;5;200m"},
		{`\F{bogus}`, "\x1b[39m"},
		// No braces is the empty argument, and the letters after it are text:
		// measured, zsh drew `%Fred` as black and then `red`.
		{`\Fred`, "\x1b[30mred"},
		{`\F`, "\x1b[30m"},
		{`\F{}`, "\x1b[30m"},
		// Three octal digits are the byte they name, and any shorter run is
		// not a number at all.
		{`\007`, "\a"},
		{`\101`, "A"},
		{`\1011`, "A1"},
		{`\10`, `\10`},
		{`\1`, `\1`},
		{`\00`, `\00`},
		{`\8`, `\8`},
		{`\400`, "\x00"},
		// Octal digits, so a run of three that holds an 8 or a 9 is not a
		// number: measured, bash drew `\189`, `\888` and `\099` as written,
		// and `\1234` as `S4` — the first three digits and then a `4`.
		{`\189`, `\189`},
		{`\888`, `\888`},
		{`\099`, `\099`},
		{`\1234`, "S4"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			if got := s.render(tc.in); got != tc.want {
				t.Errorf("drew %q, want %q", got, tc.want)
			}
		})
	}
}

// A letter in more than one table is read from the first of them, which is
// what lets a dialect give a code of its own to a letter the substrate has a
// field for.
func TestTheTablesAreReadInOrder(t *testing.T) {
	s := Shell{
		Runner:  newTestRunner(map[string]string{"USER": "someone"}),
		Style:   PromptStyle{Escape: '\\', Codes: map[rune]PromptField{'u': FieldUser}},
		Clock:   at(t, 1, 2, 3),
		Session: "",
	}
	s.Style.Sequences = map[rune]string{'u': "sequence"}
	s.Style.Colors = map[rune]PromptColor{'u': Foreground}
	if got := s.render(`\u`); got != "someone" {
		t.Errorf("drew %q, want the field", got)
	}
	delete(s.Style.Codes, 'u')
	if got := s.render(`\u`); got != "sequence" {
		t.Errorf("drew %q, want the sequence", got)
	}
	delete(s.Style.Sequences, 'u')
	if got := s.render(`\u{red}`); got != "\x1b[31m" {
		t.Errorf("drew %q, want the color", got)
	}
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
			if got := s.render(`<\q>`); got != tc.want {
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
		if got := s.render(`x\`); got != `x\` {
			t.Errorf("Unknown %v drew %q, want the character itself", unk, got)
		}
	}
}

// With no escape language the text is the text.
func TestNoEscapeLanguage(t *testing.T) {
	s := Shell{Style: PromptStyle{}}
	if got := s.render(`<\u %n>`); got != `<\u %n>` {
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
			Expand: interp.PromptExpandsAlways, Escape: '\\',
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
			if got := s.render(tc.in); got != tc.want {
				t.Errorf("drew %q, want %q", got, tc.want)
			}
		})
	}
}

// And with no such character the text is the text.
func TestABareBangWithoutTheHabit(t *testing.T) {
	s := Shell{Runner: newTestRunner(nil), counts: &counts{history: 6}}
	if got := s.render("<!> <!!>"); got != "<!> <!!>" {
		t.Errorf("drew %q, want it untouched", got)
	}
}

// The bang is read after expansion, where a code in the table is read before
// it.
//
// Measured against ksh93, which does both: with `x='!'` set, `PS1='<$x>'`
// drew the history number, so a `!` is read wherever it has come from. A code
// is not — `x='\u'` drew the two characters in bash, because the table had
// already been read by the time the value arrived.
func TestTheBangIsReadAfterExpansionAndTheTableBeforeIt(t *testing.T) {
	s := Shell{
		Runner: newTestRunner(map[string]string{"x": "!", "USER": "someone"}),
		Style: PromptStyle{
			Expand: interp.PromptExpandsAlways, History: '!', Escape: '\\',
			Codes: map[rune]PromptField{'u': FieldUser},
		},
		counts: &counts{history: 6},
	}
	// A bang that arrives through expansion is still a bang.
	if got := s.render("<$x>"); got != "<7>" {
		t.Errorf("an expanded bang drew %q, want <7>", got)
	}
	// A code that arrives through expansion is text: the table has been and
	// gone.
	s.Runner.SetVar("y", `\u`)
	if got := s.render("<$y>"); got != `<\u>` {
		t.Errorf("an expanded code drew %q, want it left alone", got)
	}
	// And one written in the prompt is drawn.
	if got := s.render(`<\u>`); got != "<someone>" {
		t.Errorf("drew %q, want <someone>", got)
	}
}

// An escape with nothing after it, where the dialect says it is dropped.
//
// A row of the table rather than a rule above it, because the panel splits:
// measured through a pty, zsh drew `PS1='x%'` as `x` — and `print -P 'x%'` and
// `${(%):-x%}` as `x` too, which is what makes it the *dialect's* answer
// rather than a difference between the two readers — where bash and ksh93 draw
// the character. The pair of tests is the assertion; either alone passes with
// the flag ignored in one direction.
func TestATrailingEscapeTheDialectDrops(t *testing.T) {
	for _, unk := range []UnknownCode{KeepBoth, DropEscape, DropBoth} {
		s := Shell{Style: PromptStyle{Escape: '%', Unknown: unk, TrailingEscapeIsDropped: true}}
		if got := s.render(`x%`); got != `x` {
			t.Errorf("Unknown %v drew %q, want the escape dropped", unk, got)
		}
		if got := s.render(`%`); got != `` {
			t.Errorf("Unknown %v drew %q for a lone escape, want nothing", unk, got)
		}
	}
}

// The braces after a code the dialect lists in Formats hold a `strftime`
// format, and replace the shape the code would otherwise draw.
//
// Measured through a pty against zsh 5.9.2: `%D` drew `26-09-07`, `%D{%H:%M}`
// drew `04:25` and `%D{}` drew nothing at all. The last one is why the walker
// reports whether there were braces rather than only what was in them — an
// empty format is a format, and the absence of one is the plain date.
func TestACodeWhoseBracesAreATimeFormat(t *testing.T) {
	at := time.Date(2026, 9, 7, 4, 25, 13, 0, time.UTC)
	s := Shell{
		Clock: func() time.Time { return at },
		Style: PromptStyle{
			Escape:  '%',
			Codes:   map[rune]PromptField{'D': FieldDateYearMonthDay},
			Formats: map[rune]bool{'D': true},
		},
	}
	for _, tc := range []struct{ in, want string }{
		{`%D`, "26-09-07"},
		{`%D{%H:%M}`, "04:25"},
		{`%D{}`, ""},
		{`%D{%Y-%m-%dT%H:%M:%S}`, "2026-09-07T04:25:13"},
		// The letters after an unbraced code are text, exactly as they are
		// after an unbraced color code.
		{`%Dx`, "26-09-07x"},
	} {
		if got := s.render(tc.in); got != tc.want {
			t.Errorf("%s drew %q, want %q", tc.in, got, tc.want)
		}
	}
	// And a code *not* in Formats keeps its braces as the text they are,
	// which is what says the set is read and not the presence of braces.
	plain := Shell{
		Clock: func() time.Time { return at },
		Style: PromptStyle{Escape: '%', Codes: map[rune]PromptField{'D': FieldDateYearMonthDay}},
	}
	if got := plain.render(`%D{%H}`); got != "26-09-07{%H}" {
		t.Errorf("a code outside Formats drew %q, want the braces as text", got)
	}
}
