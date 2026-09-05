// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/blairham/sh/syntax"
)

// escapes walks the prompt's text and draws each code in it.
//
// Before expansion rather than after, which is measurable and not a detail:
// with `x='\u'` set, bash draws `$x` as the two characters and not as the user
// name, so a code that arrives *through* expansion is text and not a code. The
// order also explains a thing that looks like a prompt feature and is not —
// `\$` drawing a bare dollar in dash, which has no table at all, is what a
// backslash does to a dollar during the expansion that follows.
// history reads the character that stands for the history number.
//
// Last of the three passes, which is measurable rather than chosen. ksh93
// draws `\!` as the number — its table drops the backslash and this reads the
// `!` left behind, where one pass would have left it alone. And with `x='!'`
// set, ksh93 draws `$x` as the number too, so this happens after expansion as
// well: a `!` is read wherever it has come from, unlike a code in the table,
// which is only read where it was typed.
func (s Shell) history(text string) string {
	if s.Style.History == 0 || text == "" {
		return text
	}
	var b strings.Builder
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		if runes[i] != s.Style.History {
			b.WriteRune(runes[i])
			continue
		}
		if i+1 < len(runes) && runes[i+1] == s.Style.History {
			// Doubled is one of itself, which is the only way to put one in
			// a prompt that reads them.
			b.WriteRune(runes[i])
			i++
			continue
		}
		b.WriteString(s.field(FieldHistoryNumber))
	}
	return b.String()
}

// table draws each code in the prompt's text.
func (s Shell) table(text string) string {
	if s.Style.Escape == 0 || text == "" {
		return text
	}
	var b strings.Builder
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		if runes[i] != s.Style.Escape {
			b.WriteRune(runes[i])
			continue
		}
		if i+1 >= len(runes) {
			// An escape with nothing after it is the character itself. There
			// is no code to look up, so none of the three answers below
			// applies.
			b.WriteRune(runes[i])
			break
		}
		code := runes[i+1]
		i++
		if field, ok := s.Style.Codes[code]; ok {
			b.WriteString(s.field(field))
			continue
		}
		if seq, ok := s.Style.Sequences[code]; ok {
			b.WriteString(seq)
			continue
		}
		if layer, ok := s.Style.Colors[code]; ok {
			arg, next := colorArgument(runes, i+1)
			i = next
			b.WriteString(colorSequence(layer, arg))
			continue
		}
		if v, ok := octalByte(s.Style.Octal, runes, i); ok {
			b.WriteByte(v)
			i += 2
			continue
		}
		switch s.Style.Unknown {
		case DropEscape:
			b.WriteRune(code)
		case DropBoth:
		default: // KeepBoth
			b.WriteRune(s.Style.Escape)
			b.WriteRune(code)
		}
	}
	return b.String()
}

// colorArgument reads the braces after a color code, and says where the code
// ended.
//
// No braces is the empty argument and the code ends where it was, which is
// measured: zsh drew `%Fred` as the color for an empty argument and then the
// three letters, so the letters after an unbraced code are text.
//
// An opening brace with no closing one is the rest of the prompt, for the
// reason an unterminated anything is: there is no later text for it to be
// text of.
func colorArgument(runes []rune, i int) (string, int) {
	if i >= len(runes) || runes[i] != '{' {
		return "", i - 1
	}
	for j := i + 1; j < len(runes); j++ {
		if runes[j] == '}' {
			return string(runes[i+1 : j]), j
		}
	}
	return string(runes[i+1:]), len(runes) - 1
}

// octalByte reads three octal digits as the byte they name.
//
// Three exactly. Measured against bash: `\007` drew the bell and `\101` drew
// `A`, while `\0`, `\1`, `\10`, `\00` and `\8` were each drawn as the two
// characters written — so a shorter run is not a shorter number, it is not a
// number at all and falls through to whatever the dialect does with a code it
// does not know.
//
// The low byte of the value, so `\400` is a NUL, which is what bash drew.
func octalByte(enabled bool, runes []rune, i int) (byte, bool) {
	if !enabled || i+2 >= len(runes) {
		return 0, false
	}
	v := 0
	for _, r := range runes[i : i+3] {
		if r < '0' || r > '7' {
			return 0, false
		}
		v = v*8 + int(r-'0')
	}
	return byte(v), true
}

// field is what one code draws.
//
// The values come from the shell's own variables wherever the shell has an
// answer of its own, and from the process only where it cannot: a session that
// has assigned PWD or HOME means it, and asking the operating system instead
// would draw a prompt describing a different shell than the one being typed
// into.
func (s Shell) field(f PromptField) string {
	switch f {
	case FieldUser:
		return s.userName()
	case FieldHost:
		host, _, _ := strings.Cut(s.hostName(), ".")
		return host
	case FieldHostFull:
		return s.hostName()
	case FieldCwd:
		return abbreviate(s.varOr("PWD", ""), s.varOr("HOME", ""))
	case FieldCwdFull:
		return s.varOr("PWD", "")
	case FieldCwdBase:
		// The last component of the *abbreviated* path, so the home directory
		// itself is `~` and not its own name. Measured: in it, bash's `\W` and
		// zsh's `%c` both drew `~`, and one directory down both drew `sub`.
		return lastComponent(abbreviate(s.varOr("PWD", ""), s.varOr("HOME", "")))
	case FieldCwdBaseFull:
		return lastComponent(s.varOr("PWD", ""))
	case FieldPrivilege:
		if os.Geteuid() == 0 {
			return "#"
		}
		return s.Style.Privilege
	case FieldShellName:
		// The basename of it. A shell invoked as ./y-bash calls itself
		// bash, which is what real bash draws and is the point of the code.
		return path.Base(s.Name)
	case FieldNewline:
		// A carriage return with it: the terminal is in raw mode while the
		// prompt is drawn, so a newline alone moves down a row and leaves the
		// cursor where it was across.
		return "\r\n"
	case FieldReturn:
		return "\r"
	case FieldTab:
		return "\t"
	case FieldEscape:
		return string(s.Style.Escape)
	case FieldOpenState:
		return s.openState()
	case FieldVersion:
		return s.Style.Version
	case FieldVersionFull:
		return s.Style.VersionFull
	case FieldHistoryNumber:
		return itoa(s.counted().history + 1)
	case FieldCommandNumber:
		return itoa(s.counted().command + 1)
	case FieldJobCount:
		return itoa(s.liveJobs())
	case FieldTerminalName:
		return s.terminalName()
	case FieldNonPrintingStart:
		return markStart
	case FieldNonPrintingEnd:
		return markEnd
	case FieldExitStatus:
		if s.Runner == nil {
			return "0"
		}
		return itoa(s.Runner.ExitStatus())
	case FieldTime24:
		return s.now().Format("15:04:05")
	case FieldTime12:
		return s.now().Format("03:04:05")
	case FieldTime24HM:
		return s.now().Format("15:04")
	case FieldTime12AMPM:
		return s.now().Format("03:04 PM")
	case FieldTime12Padded:
		// zsh pads the hour with a space rather than a zero: " 9:56PM" at
		// nine and "10:08PM" at ten. Go's layouts have no space-padded hour
		// — `_` pads a day and nothing else — so it is done here.
		t := s.now().Format("3:04PM")
		if t[1] == ':' {
			t = " " + t
		}
		return t
	case FieldTime24Unpadded:
		return unpadHour(s.now().Format("15:04:05"))
	case FieldTime24HMUnpadded:
		return unpadHour(s.now().Format("15:04"))
	case FieldDate:
		return s.now().Format("Mon Jan 02")
	case FieldDateShort:
		return s.now().Format("Mon 2")
	case FieldDateMonthDayYear:
		return s.now().Format("01/02/06")
	case FieldDateYearMonthDay:
		return s.now().Format("06-01-02")
	}
	return ""
}

// userName is who the shell is running as.
//
// USER and LOGNAME first, because a session that set one has said who it is;
// the process's own answer only if neither is there.
func (s Shell) userName() string {
	if v := s.varOr("USER", ""); v != "" {
		return v
	}
	if v := s.varOr("LOGNAME", ""); v != "" {
		return v
	}
	return ""
}

// hostName is the machine's name, asked once and kept.
//
// A prompt is drawn on every keystroke that redraws the line, and the name
// does not change while a shell is running.
func (s Shell) hostName() string {
	if v := s.varOr("HOSTNAME", ""); v != "" {
		return v
	}
	hostOnce.Do(func() { host, _ = os.Hostname() })
	return host
}

// unpadHour takes the leading zero off an hour below ten.
//
// Go's 24-hour layouts pad and it has no form that does not, so it is done
// here. Measured against zsh through a pty at two hours it can be told apart
// at: `%*` drew 6:11:43 at six in the morning and 0:17:07 after midnight,
// where bash's `\t` drew 06:11:40 and 00:17:08. Midnight is the case that says
// this takes a zero off rather than replacing it with a space or with nothing:
// zero o'clock keeps a digit.
func unpadHour(clock string) string { return strings.TrimPrefix(clock, "0") }

// lastComponent is the final component of a path, and nothing at all for no
// path — where path.Base answers `.`, which is a directory and not the absence
// of one.
func lastComponent(dir string) string {
	if dir == "" {
		return ""
	}
	return path.Base(dir)
}

// abbreviate writes the home directory as `~`.
//
// The home directory exactly, or a path inside it — not any path that merely
// starts with the same letters, which is why the boundary is checked.
func abbreviate(dir, home string) string {
	if home == "" || dir == "" {
		return dir
	}
	if dir == home {
		return "~"
	}
	if strings.HasPrefix(dir, home) && (strings.HasSuffix(home, "/") || dir[len(home)] == '/') {
		return "~" + dir[len(home):]
	}
	return dir
}

func (s Shell) varOr(name, fallback string) string {
	if s.Runner == nil {
		return fallback
	}
	if v, ok := s.Runner.GetVar(name); ok && v != "" {
		return v
	}
	return fallback
}

// The host name is one syscall for the life of the shell.
var (
	hostOnce sync.Once
	host     string
)

// now is the clock a prompt with the time in it reads.
//
// Injectable because a prompt that draws the time cannot otherwise be tested
// twice with the same answer, and because this is the first thing here whose
// output changes with nothing typed.
func (s Shell) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

// counted is the pair of running totals a prompt can draw, and is never nil
// so that a Shell used without either loop still draws something.
func (s Shell) counted() *counts {
	if s.counts == nil {
		return &counts{}
	}
	return s.counts
}

// liveJobs is how many jobs the shell is still looking after.
//
// A finished job is not one: it stays in the table until its notice has been
// given, and counting it would say there is something running for exactly as
// long as it takes to say that there is not.
func (s Shell) liveJobs() int {
	if s.Runner == nil {
		return 0
	}
	n := 0
	for _, j := range s.Runner.Jobs() {
		if !j.Finished() {
			n++
		}
	}
	return n
}

// counts are the two running totals, held by pointer because the prompt is
// drawn from a Shell taken by value and these change under it.
type counts struct {
	// open is what the parser was still inside when the line so far ran out,
	// for a continuation prompt that says what it is waiting for.
	open []syntax.Open

	// tty is the terminal's name once it has been looked for, and looked
	// says it has been. Kept for the session rather than for the package: a
	// prompt is drawn on every keystroke and the search reads a directory,
	// and state that outlives one shell is state a test cannot arrange.
	tty    string
	looked bool

	// history is how many lines are behind the one about to be typed,
	// including those that came from the history file — the numbering
	// carries across sessions, which is what makes it a *history* number.
	history int
	// command is how many commands this session has run, which does not.
	command int
}

// accepted records what one accepted line was.
//
// One method for both loops rather than the same three lines in each. The
// note on beforeReading says why: the editor's copy of what happens between
// lines is the one nothing exercised, and counting written twice went the
// same way — a mutation of the terminal loop's copy survived every test,
// because every test drives the other one.
//
// A blank line is neither an entry nor a command. A line that will not parse
// is an entry and is not a command: measured, bash draws `!3 #2` at the
// prompt after one.
func (c *counts) accepted(blank, parsed bool) {
	if blank {
		return
	}
	c.history++
	if parsed {
		c.command++
	}
}

// terminalName is the terminal's name, asked once per session.
func (s Shell) terminalName() string {
	c := s.counted()
	if !c.looked {
		c.tty, c.looked = lookupTerminal(s.In), true
	}
	return c.tty
}

// openState is what the shell is still inside, in this dialect's words.
//
// The parser says `if then` where zsh says `then`, and `then &&` where zsh
// says `then cmdand`. The difference is not clause-versus-construct: a clause
// stands in place of what it is inside and an operator follows it, and which
// is which is what the dialect's table says.
func (s Shell) openState() string {
	open := s.counted().open
	if len(open) == 0 || s.Style.OpenWords == nil {
		return ""
	}
	var words []string
	for _, o := range open {
		w, ok := s.Style.OpenWords[o.Word]
		if !ok || w.Text == "" {
			// Not drawn at all, which is how a loop's `do` disappears while
			// the loop it belongs to stays.
			continue
		}
		if w.Replaces && len(words) > 0 {
			words[len(words)-1] = w.Text
			continue
		}
		words = append(words, w.Text)
	}
	return strings.Join(words, " ")
}
