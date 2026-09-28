// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// The **mail check**: the look at a mailbox between prompts that says when
// something has arrived and not been read.
//
// Measured 2026-09-28 through a pseudo-terminal against zsh 5.9.2 — `zsh -f
// -i` under `env -i PATH=/usr/bin:/bin TERM=xterm` with a scratch `HOME`,
// `MAILCHECK=1` and `MAIL` naming a file, driving a real session and growing
// the file between commands:
//
//	echo A                              →  You have new mail.
//	echo B                              →  You have new mail.
//	cat "$MAIL" >/dev/null               (nothing)
//	echo C                              →  (silence)
//	echo D                              →  (silence)
//
// **The read is what stops it**, and that is the row that settles the
// condition. A rule of "the file changed since the last check" reports once
// and then stops whether or not anybody read it; this reports at every check
// until the file is read, and `cat` — which moves nothing but the access
// time — is what ends it. So the question is the classic one: is the file's
// **modification time past its access time**, with something in it.
//
// Two more rows bound it. `MAILCHECK=0` reports nothing at all, however the
// file grows, so the interval is also the off switch. And with a `precmd`
// defined, `PRECMD` is written **before** the sentence at every prompt, so
// the check runs after the prompt hooks rather than in front of them.
type MailStyle struct {
	// File is the parameter naming one mailbox — zsh's and bash's `$MAIL`.
	// Empty is a dialect with no mail check, which is what every dialect had
	// before this and is what the zero value has to mean.
	File string

	// Path is the parameter naming several, each `file` or `file?message`.
	// Empty is a dialect that has only the single name above.
	Path string

	// Interval is the parameter holding how many seconds pass between
	// checks, and **zero in it turns the check off** — measured, a session
	// with `MAILCHECK=0` reports nothing however the file grows. Empty names
	// no parameter, and then no check is made at all: a front end that did
	// not say how often has not asked for one.
	Interval string

	// Message is what is written when a mailbox has unread mail in it, for a
	// `Path` entry that carries none of its own and for `File` always.
	//
	//	zsh   You have new mail.
	//
	// Empty writes nothing, which is the same "no mail check" the empty
	// File is.
	Message string
}

// mailState is what the session remembers between checks: when it last
// looked. A pointer for the reason hookState is one — Shell is copied by
// value and a check made once has to stay made.
type mailState struct {
	last time.Time
}

// checkMail looks at the mailboxes if enough time has passed, and writes the
// message for each one holding mail nobody has read.
//
// Called after the prompt hooks, which is measured rather than chosen: with a
// `precmd` defined, that function's output comes first at every prompt.
//
// **Which stream it goes to is not measured**, and is written as a notice —
// the same stream every other between-commands notice in this loop uses. The
// message is written between one command and the next, where a redirection a
// script can write reaches neither: `exec 2>file` at the prompt left the file
// empty and the terminal silent in the same run, so the probe that would have
// separated them did not separate them. It is recorded as unmeasured rather
// than asserted.
//
// The interval is read **live**, the way every other prompt-time parameter
// here is: `MAILCHECK=0` typed at the prompt turns the check off from the next
// prompt on, and a value read at startup would be the one an rc file left.
func (s Shell) checkMail() {
	if s.Mail.Message == "" || s.Mail.Interval == "" || s.mail == nil {
		return
	}
	every, ok := s.mailInterval()
	if !ok {
		return
	}
	now := time.Now()
	if !s.mail.last.IsZero() && now.Sub(s.mail.last) < every {
		return
	}
	// The clock moves whether or not anything is written, so a mailbox that
	// is not there costs one look per interval rather than one per prompt.
	s.mail.last = now
	for _, box := range s.mailboxes() {
		if !mailIsUnread(box.path) {
			continue
		}
		message := box.message
		if message == "" {
			message = s.Mail.Message
		}
		s.errf("%s\n", message)
	}
}

// mailInterval is how long between checks, and whether to check at all.
//
// False for a parameter that is unset, holds something that is not a number,
// or holds a number that is not positive — the last of which is the measured
// off switch rather than a guard: `MAILCHECK=0` reports nothing however the
// file grows.
func (s Shell) mailInterval() (time.Duration, bool) {
	if s.Runner == nil {
		return 0, false
	}
	value, ok := s.Runner.GetVar(s.Mail.Interval)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n <= 0 {
		return 0, false
	}
	return time.Duration(n) * time.Second, true
}

// mailbox is one place to look and what to say about it.
type mailbox struct {
	path    string
	message string
}

// mailboxes is every mailbox this session watches: the single name, and each
// entry of the list.
//
// A list entry is `file` or `file?message`, and the message it carries is
// used in place of the default for that box alone. Measured —
// `MAILPATH=/tmp/inbox?You got mail` writes `You got mail` where the same
// file under `MAIL` writes the ordinary sentence.
//
// **The `?` is the last one in the entry**, so a path holding one keeps it:
// there is no escape for it in either shell and a path is more likely to hold
// a `?` than a message is to want one.
func (s Shell) mailboxes() []mailbox {
	var out []mailbox
	if s.Runner == nil {
		return out
	}
	if s.Mail.File != "" {
		if v, ok := s.Runner.GetVar(s.Mail.File); ok && v != "" {
			out = append(out, mailbox{path: v})
		}
	}
	if s.Mail.Path == "" {
		return out
	}
	v, ok := s.Runner.GetVar(s.Mail.Path)
	if !ok || v == "" {
		return out
	}
	for _, entry := range strings.Split(v, ":") {
		if entry == "" {
			continue
		}
		if i := strings.LastIndexByte(entry, '?'); i >= 0 {
			out = append(out, mailbox{path: entry[:i], message: entry[i+1:]})
			continue
		}
		out = append(out, mailbox{path: entry})
	}
	return out
}

// mailIsUnread reports a mailbox holding something nobody has read since it
// arrived: the modification time past the access time, with bytes in it.
//
// The size test is not decoration. An empty mailbox that something touched
// has a modification time past its access time and no mail in it, and every
// shell that reports is quiet about one.
func mailIsUnread(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return false
	}
	access, ok := accessTime(info)
	if !ok {
		return false
	}
	return info.ModTime().After(access)
}
