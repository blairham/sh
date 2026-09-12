// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path"
	"strings"
	"time"

	"github.com/blairham/sh/interp"
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
		b.WriteString(s.field(FieldHistoryNumber, "", false))
	}
	return b.String()
}

// The table half of the drawing — the walker and the resolvers — is
// [interp.RenderPromptValue], called from render above. The table is the
// dialect's, which is the same table the interpreter's `${(%)…}`, `print -P`
// and bash's `${v@P}` read; see repl/promptstyle.go for why that is one type
// and not two. What this file supplies is the *resolver* half, and the
// drawer's resolver answers everything: a session knows its own history
// number, its terminal's name and what the parser is still inside, so nothing
// here is ever refused and the walker's refusal path is unreachable from this
// side. The escapes it would refuse for a script are the ones this Shell
// answers.
//
// A code the table lists as Unsupported is the one exception, and it is the
// one place the two readers deliberately differ: an expansion refuses it by
// name, and a prompt has to draw something and falls through to whatever
// Unknown says — the same answer it had when the code was in no table at all.
//
// The escapes run before the expansion where the dialect says so, which is
// measurable and not a detail: with `x='\u'` set, bash draws `$x` as the two
// characters and not as the user name, so a code that arrives *through*
// expansion is text and not a code. The order also explains a thing that looks
// like a prompt feature and is not — `\$` drawing a bare dollar in dash, which
// has no table at all, is what a backslash does to a dollar during the
// expansion that follows.

// promptField is the drawer's resolver: every code, always answered.
//
// FieldNone is a code in no part of the table. A prompt has to draw
// something, so it draws what this dialect's Unknown says: bash keeps both
// characters, ksh93 drops the escape and zsh drops the pair. A script's
// expansion refuses the same code by name instead, which is the split the
// walker's note explains — the policy is read here because this is the reader
// that has to obey it.
func (s Shell) promptField(f PromptField, arg string, braced bool) (string, bool) {
	if f == FieldNone {
		switch s.Style.Unknown {
		case DropEscape:
			return arg, true
		case DropBoth:
			return "", true
		default: // KeepBoth
			return string(s.Style.Escape) + arg, true
		}
	}
	return s.field(f, arg, braced), true
}

// promptQuantity is the drawer's half of the conditional's split, and like
// promptField it answers everything: a session knows its own terminal's
// width and what the parser is still inside, so the walker's refusal path is
// unreachable from this side too.
//
// Everything else is asked of the Runner, for the reason the fields are: a
// conditional written *in* a prompt and the same one written in a script have
// to agree about the shell they are both asking about.
//
// Two answers are this reader's own and each is measured against what a
// prompt is:
//
//	%(e.…)  the eval depth, which is nought while a prompt is drawn — nothing
//	        is running — where the interpreter refuses it rather than read a
//	        frame count that is not the same question.
//	%(_.…)  how many constructs are still open, which is what the
//	        continuation prompt is *for* and which a script has none of.
//
// The width `%(l.…)` wraps at is *not* one of them, and is asked of the
// Runner with the rest: this session assigns COLUMNS before every prompt (see
// trackWindowSize), so the terminal's size and the shell's variable are one
// answer, and a startup file that narrows COLUMNS narrows what the prompt
// measures itself against — which is what powerlevel10k relies on.
func (s Shell) promptQuantity(c PromptCondition, n int) (int, bool) {
	switch c {
	case ConditionEvalDepth:
		return 0, true
	case ConditionOpenConstructs:
		return len(s.counted().open), true
	}
	if s.Runner == nil {
		// No interpreter to ask, and a drawer still has to answer. Nought is
		// the count of everything a Runner would have held.
		return 0, true
	}
	v, ok := s.Runner.PromptQuantity(c, n)
	if !ok {
		return 0, true
	}
	return v, true
}

// field is what one code draws.
//
// The values come from the shell's own variables wherever the shell has an
// answer of its own: a session that has assigned PWD or HOME means it, and
// asking the operating system instead would draw a prompt describing a
// different shell than the one being typed into.
//
// Where it has none, the answer is asked of the Runner rather than of the
// process — see askRunner. Which of the two a code takes is measured per code
// and not assumed: a directory is the session's, and a login name is not,
// because every shell in the panel draws the password database's answer for it
// however `$USER` is set.
func (s Shell) field(f PromptField, arg string, braced bool) string {
	switch f {
	case FieldUser, FieldHost, FieldHostFull:
		// Who is typing and where, which only the shell that was built knows
		// — asked of the Runner for the reason `%x` below is, so that `\u`
		// drawn in a prompt and `${(%):-%n}` written in a script are one
		// answer rather than two. See interp.LoginName, which is the one
		// question all of them ask.
		//
		// This was two answers, and #1446 is what that cost. The lookup here
		// read `$USER` and `$LOGNAME` and fell through to nothing, where the
		// one beside it fell through to the *system*; so `\h` was right and
		// `\u` was empty, and a prompt short of only its user still looks
		// like a prompt. Reading the variables was wrong on its own terms
		// too, and measured: bash 5.3.15, bash 3.2.57 and zsh 5.9.2 all draw
		// the password database's answer with `USER=someone-else` injected
		// before the shell starts and assigned inside it, and all three draw
		// the system's host name with `HOSTNAME=elsewhere` set the same two
		// ways. A prompt that followed either variable named the wrong
		// person, or the wrong machine, at exactly the moment somebody had
		// said which one they meant.
		//
		// The cost is paid once. SetPromptUserFunc keeps the first answer,
		// which a prompt needs: the line editor redraws the whole prompt on
		// every keystroke, and `user.Current` is 0.83-1.10 ms on darwin
		// (#1423). Resolving per draw would put a millisecond of Directory
		// Services behind every character typed; resolving eagerly at startup
		// would put it in front of every `-c` run that can never draw one.
		return s.askRunner(f, arg, braced)
	case FieldCwd, FieldCwdFull, FieldCwdBase, FieldCwdBaseFull,
		FieldCwdCounted, FieldCwdCountedFull:
		// Asked of the Runner, not answered here — even though the values are
		// the session's own `PWD` and `HOME`, which this reader has in hand.
		//
		// It had four cases of its own until #1699, reading the same two
		// variables out of the same Runner and abbreviating them with a copy
		// of the same helper. What the copy did not have was the **count**, so
		// a drawn `PS1='%2~> '` showed the whole path where zsh showed `b/c`,
		// and the script-side `${(%%):-%2~}` beside it was right the whole
		// time. Nothing said so, because the two answers only differ when
		// somebody writes a count and every test here wrote none.
		//
		// That is the shape a second helper always takes: it is written to
		// avoid a dependency, it is correct on the day, and the next thing the
		// first one learns is the thing it does not. So the dependency is
		// taken instead. See askRunner.
		return s.askRunner(f, arg, braced)
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
	case FieldCountedColumn:
		// Nothing drawn, and a column counted — which the walker does, not
		// this. A prompt writes it to say that bytes hidden between the two
		// markers above do reach the screen after all.
		return ""
	case FieldSourceFile, FieldUnitName:
		// The file being read, which only the interpreter knows — it is
		// reading nothing at a prompt, so both come to what the shell calls
		// itself. Asked of the Runner rather than answered here, so that
		// `${(%):-%x}` typed at the prompt and `%x` written *in* the prompt
		// are the same answer.
		return s.askRunner(f, arg, braced)
	case FieldExitStatus:
		if s.Runner == nil {
			return "0"
		}
		return itoa(s.Runner.ExitStatus())
	}
	if braced && isClockField(f) {
		// The braces after a Formats code hold a `strftime` format and
		// replace the shape the code would otherwise draw: measured, `%D` is
		// `26-09-07` and `%D{%H:%M}` is the clock through that format.
		//
		// interp.Strftime rather than a formatter of this package's own,
		// because a second implementation of one format language is the thing
		// that would drift — it is the same one `printf '%(fmt)T'` writes
		// through. The *clock* is still this reader's, which is the resolver
		// split doing what it is for: a prompt is tested with an injected
		// Clock and a script's expansion with the Runner's.
		return interp.Strftime(arg, s.now())
	}
	switch f {
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

// isClockField reports whether a code is drawn from the clock, and so whether
// a `strftime` argument in braces has anything to replace.
func isClockField(f PromptField) bool {
	return f >= FieldTime24 && f <= FieldDateYearMonthDay
}

// askRunner is one code answered by the interpreter rather than here.
//
// The seam the whole resolver split rests on: a shell is *told* who it is
// running as, what machine it is on and what file it is reading, and the front
// end reads back the same answer a script would get for the same escape. A
// Shell with no Runner has not been told, and draws nothing rather than
// inventing something — which is also how a prompt is tested without process
// state, since a test builds a Runner and tells it what to say.
func (s Shell) askRunner(f PromptField, arg string, braced bool) string {
	if s.Runner == nil {
		return ""
	}
	v, _ := s.Runner.PromptField(f, arg, braced)
	return v
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

// counts is the session state a prompt is drawn from, held by pointer because
// the prompt is drawn from a Shell taken by value and this changes under it.
//
// Named for what it started as. It is now the two running totals, what the
// parser is still inside, the terminal's name, and what the last command was —
// all of it the same thing: facts about the session that only the loops know
// and that the next prompt needs.
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

	// last is what the previous command was, for a prompt provider that draws
	// something about it. Recorded by closeBlock, which is also where the
	// block store's record of the same command is written — one place and one
	// rule, so the two accounts of "the last command" cannot part company.
	last lastCommand
}

// lastCommand is what a prompt provider is told about the command before this
// prompt. The zero value is a session in which nothing has run yet.
type lastCommand struct {
	// command is the line as typed, before expansion, which is the same text
	// a block record keeps and for the same reason: it is what the person
	// wrote, and an event carries argv after expansion instead.
	command string
	// duration is the wall clock around the whole typed line.
	duration time.Duration
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
		c.tty, c.looked = lookupTerminal(s.inFile()), true
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
