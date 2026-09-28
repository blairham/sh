// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/blairham/sh/internal/boundary"
)

// The **mail check**: the look at a mailbox between prompts that says when
// something has arrived and nobody has read it.
//
// Measured 2026-09-28 through a pseudo-terminal against zsh 5.9.2 — `zsh -f
// -i` under `env -i PATH=/usr/bin:/bin TERM=dumb` with a scratch `HOME`,
// `MAILCHECK=1` and `MAIL` naming a file, driving a real session and setting
// the file's two times between commands. Each row is a fresh session.
//
// The announcement is a **conjunction of three**, and each row below holds
// two of them still and moves the third:
//
//	grow once, then four prompts          →  one report, then silence
//	grow again at the fifth               →  a second report
//	fresh mtime with a NEWER atime        →  silence
//	fresh mtime with atime EQUAL to it    →  a report
//	mail already in the box at startup    →  silence, however new its mtime
//	MAILCHECK=0                           →  silence, however the file grows
//
// So: **bytes in the file, an access time that is not past the modification
// time, and a modification since the previous check.** The third conjunct is
// the one a short grid misses, and the first row is what catches it — nothing
// read the file between those four prompts, and the reports stopped anyway.
//
// **An earlier reading of this had only the first two**, on a grid of `echo`,
// `echo`, `cat`, `echo`, `echo`: it reported at every prompt until the `cat`
// and then stopped, and the reference agreed on every row of it. It agreed
// because silence after a `cat` is equally what "nothing has changed since
// the last check" produces, so that grid could not tell the two rules apart —
// the `cat` never had to be the thing that stopped it. A **second growth** is
// what separates them, and against it the two-conjunct rule reports three
// times where the reference reports twice.
//
// The equality cell is measured rather than assumed, because the two spellings
// of "not read since it arrived" part exactly there: a box whose two times are
// the same instant **is** announced, so the test is `atime <= mtime`.
//
// The baseline is the session, not the epoch. Mail sitting unread in the box
// when the shell starts is never announced — the first check establishes what
// "since" means rather than reporting everything older than it.
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
//
// It is the interval's clock and the "since" of the change test at once,
// which is not a saving but the measured shape: a session announces a box
// that changed since it last **looked**, so one field answers both. Set to
// the session's start by Run, which is why mail already sitting in the box
// when the shell starts is never announced.
type mailState struct {
	last time.Time
}

// checkMail looks at the mailboxes if enough time has passed, and writes the
// message for each one that has grown since the last look and not been read.
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
func (s Shell) checkMail(ctx context.Context) {
	if s.Mail.Message == "" || s.Mail.Interval == "" || s.mail == nil {
		return
	}
	every, ok := s.mailInterval()
	if !ok {
		return
	}
	now := time.Now()
	if now.Sub(s.mail.last) < every {
		return
	}
	// What "since" means for this look, taken before the clock is moved on.
	// The clock moves whether or not anything is written, so a mailbox that
	// is not there costs one look per interval rather than one per prompt —
	// and a box that grew between two checks is reported about once, at the
	// first check past the growth, rather than at every prompt after it.
	since := s.mail.last
	s.mail.last = now
	bound := boundary.Boundary{Gate: s.Gate, Events: s.Events, Session: s.Session}
	for _, box := range s.mailboxes() {
		if !mailIsUnread(ctx, bound, box.path, since) {
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

// mailIsUnread reports a mailbox worth announcing at this look: bytes in it,
// an access time that is not past its modification time, and a modification
// since the previous look.
//
// All three are measured and all three are load-bearing. The size test is not
// decoration — an empty box something touched has a new modification time and
// no mail in it, and every shell that reports is quiet about one. The access
// test is `<=` rather than `<` because a box whose two times are the same
// instant is announced. And `since` is the conjunct an earlier reading of this
// left out, which made it report at every prompt rather than once per arrival;
// see the grid above.
//
// The probe goes through [boundary.Boundary.Stat] rather than to the os
// package, because `$MAIL` and `$MAILPATH` are variables a line at the prompt
// can set: the path is one whoever the policy is about chose, which is this
// tree's own rule for what is inside the boundary and is the same argument
// `$HISTFILE` already makes next door in history.go. A refused probe answers
// exactly as an absent mailbox does — silence — which is what that seam
// promises and is the answer a mail check wants anyway.
func mailIsUnread(ctx context.Context, bound boundary.Boundary, path string, since time.Time) bool {
	info, err := bound.Stat(ctx, path)
	if err != nil || info.Size() == 0 {
		return false
	}
	access, ok := accessTime(info)
	if !ok {
		return false
	}
	mod := info.ModTime()
	return !access.After(mod) && mod.After(since)
}
