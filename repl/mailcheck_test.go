// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/interp"
)

// A mailbox is announced when three things hold at once: bytes in it, an
// access time not past its modification time, and a modification since the
// session's previous look.
//
// **The third conjunct is the one a short grid misses.** Measured through a
// pseudo-terminal against zsh 5.9.2: grow the mailbox once and the reference
// reports at the next prompt and is silent at the four after it, with nothing
// having read the file — grow it a second time and a second report follows.
// An earlier reading of this had only the first two conjuncts and reported at
// every prompt until the box was read; it agreed with the reference on a grid
// of `echo`, `echo`, `cat`, `echo`, `echo` because silence after a `cat` is
// equally what "nothing changed since the last check" produces. A **second
// growth** is what tells the two rules apart.
//
// The rows here are that condition asked directly, because a pty grid cannot
// hold a file's two times apart: touching a file to make one probe changes the
// other.
func TestAMailboxIsAnnouncedWhenItGrewSinceTheLastLookAndNobodyReadIt(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string, mod, access time.Time) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, access, mod); err != nil {
			t.Fatal(err)
		}
		return path
	}
	var (
		ancient  = time.Now().Add(-2 * time.Hour)
		lastLook = time.Now().Add(-30 * time.Minute)
		old      = time.Now().Add(-time.Hour)
		recent   = time.Now().Add(-time.Minute)
	)
	for _, tc := range []struct {
		name  string
		path  string
		since time.Time
		want  bool
	}{
		{
			"mail arrived since the last look and nobody read it",
			write("unread", "mail\n", recent, old), lastLook, true,
		},
		{
			// The conjunct the pty grid's second growth settled: the same
			// unread box, looked at again with nothing having changed.
			"and the same box at the next look is not announced again",
			write("again", "mail\n", recent, old), time.Now(), false,
		},
		{
			// A fresh arrival read before the look comes round.
			"mail that arrived and was read is not announced",
			write("read", "mail\n", recent, time.Now()), lastLook, false,
		},
		{
			// Measured: a box whose two times are the same instant **is**
			// announced, so the access test is `<=` and not `<`.
			"a box read at the very instant it grew is still announced",
			write("equal", "mail\n", recent, recent), lastLook, true,
		},
		{
			// An empty mailbox something touched has the same two times in
			// the same order and no mail in it. Every shell that reports is
			// quiet about one, so the size is a test and not decoration.
			"an empty mailbox is not mail however new it is",
			write("empty", "", recent, old), lastLook, false,
		},
		{
			// Measured: mail already in the box when the shell starts is
			// never announced, because the first look is the baseline.
			"mail that was already there when the session began is not announced",
			write("preexisting", "mail\n", ancient, ancient.Add(-time.Minute)), lastLook, false,
		},
		{
			"and a mailbox that is not there is not mail either",
			filepath.Join(dir, "absent"), lastLook, false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mailIsUnread(context.Background(), boundary.Boundary{}, tc.path, tc.since)
			if got != tc.want {
				t.Errorf("mailIsUnread(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// Which mailboxes a session watches: the single name, and each entry of the
// list with the message it carries.
//
// `MAILPATH=/tmp/inbox?Custom mail here` writes that sentence in place of the
// default for that box alone — measured against zsh 5.9.2 through a
// pseudo-terminal, and re-run against this shell's own binary through the
// same driver.
func TestTheMailboxesASessionWatches(t *testing.T) {
	for _, tc := range []struct {
		name       string
		file, path string
		want       []mailbox
	}{
		{"the single name alone", "/m/inbox", "", []mailbox{{path: "/m/inbox"}}},
		{
			"a list of two",
			"", "/m/a:/m/b",
			[]mailbox{{path: "/m/a"}, {path: "/m/b"}},
		},
		{
			"an entry carrying its own message",
			"", "/m/a?Custom mail here",
			[]mailbox{{path: "/m/a", message: "Custom mail here"}},
		},
		{
			// The **last** `?` splits, so a path holding one keeps it:
			// there is no escape for it in either shell, and a path is more
			// likely to hold a `?` than a message is to want one.
			"a path holding a question mark keeps it",
			"", "/m/a?b?the message",
			[]mailbox{{path: "/m/a?b", message: "the message"}},
		},
		{
			"both together, the single name first",
			"/m/inbox", "/m/a",
			[]mailbox{{path: "/m/inbox"}, {path: "/m/a"}},
		},
		{"and neither is none at all", "", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Shell{Mail: MailStyle{File: "MAIL", Path: "MAILPATH"}}
			s.Runner = &interp.Runner{Vars: map[string]string{"MAIL": tc.file, "MAILPATH": tc.path}}
			got := s.mailboxes()
			if len(got) != len(tc.want) {
				t.Fatalf("mailboxes = %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("mailbox %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// How often the check is made, and the number that turns it off.
//
// **Zero is the off switch and it is measured**, not a guard: a session with
// `MAILCHECK=0` reports nothing however the file grows, driven through a
// pseudo-terminal against zsh 5.9.2 and re-run against this shell.
func TestTheMailCheckInterval(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		want        time.Duration
		ok          bool
	}{
		{"a count of seconds", "60", time.Minute, true},
		{"one second", "1", time.Second, true},
		{"zero turns the check off", "0", 0, false},
		{"and so does a negative", "-5", 0, false},
		{"a word is not a number", "soon", 0, false},
		{"and blanks around a number are ignored", " 30 ", 30 * time.Second, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Shell{Mail: MailStyle{Interval: "MAILCHECK"}}
			s.Runner = &interp.Runner{Vars: map[string]string{"MAILCHECK": tc.value}}
			got, ok := s.mailInterval()
			if got != tc.want || ok != tc.ok {
				t.Errorf("mailInterval(%q) = %v,%v want %v,%v", tc.value, got, ok, tc.want, tc.ok)
			}
		})
	}
	t.Run("and an unset parameter makes no check", func(t *testing.T) {
		s := Shell{Mail: MailStyle{Interval: "MAILCHECK"}}
		s.Runner = &interp.Runner{}
		if _, ok := s.mailInterval(); ok {
			t.Errorf("an unset interval asked for a check")
		}
	})
}
