// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

// escapes walks the prompt's text and draws each code in it.
//
// Before expansion rather than after, which is measurable and not a detail:
// with `x='\u'` set, bash draws `$x` as the two characters and not as the user
// name, so a code that arrives *through* expansion is text and not a code. The
// order also explains a thing that looks like a prompt feature and is not —
// `\$` drawing a bare dollar in dash, which has no table at all, is what a
// backslash does to a dollar during the expansion that follows.
func (s Shell) escapes(text string) string {
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
		cwd := s.varOr("PWD", "")
		if cwd == "/" {
			return "/"
		}
		return path.Base(cwd)
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
		return terminalName(s.In)
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
	case FieldDate:
		return s.now().Format("Mon Jan 02")
	case FieldDateShort:
		return s.now().Format("Mon 2")
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
	// history is how many lines are behind the one about to be typed,
	// including those that came from the history file — the numbering
	// carries across sessions, which is what makes it a *history* number.
	history int
	// command is how many commands this session has run, which does not.
	command int
}
