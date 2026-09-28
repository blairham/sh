// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blairham/sh/interp"
)

// A mailbox holds unread mail when its modification time is past its access
// time and there is something in it.
//
// **The condition is what the pty grid settled**, and it is the one thing a
// simpler rule gets wrong: "the file changed since the last check" reports
// once and then stops whether or not anybody read it, where the reference
// reports at every check until the file is **read**. `cat` — which moves
// nothing but the access time — is what ends the reports, measured against
// zsh 5.9.2 through a pseudo-terminal.
//
// The rows here are that condition asked directly, because a pty grid cannot
// hold a file's two times apart: touching a file to make one probe changes
// the other.
func TestAMailboxIsUnreadWhenItsChangeIsNewerThanItsRead(t *testing.T) {
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
	old := time.Now().Add(-time.Hour)
	recent := time.Now().Add(-time.Minute)
	for _, tc := range []struct {
		name string
		path string
		want bool
	}{
		{
			"mail arrived and nobody read it",
			write("unread", "mail\n", recent, old), true,
		},
		{
			// The read is what stops it, which is the row the whole
			// condition turns on.
			"and once it is read it is not reported",
			write("read", "mail\n", old, recent), false,
		},
		{
			// An empty mailbox something touched has the same two times in
			// the same order and no mail in it. Every shell that reports is
			// quiet about one, so the size is a test and not decoration.
			"an empty mailbox is not mail however new it is",
			write("empty", "", recent, old), false,
		},
		{
			"and a mailbox that is not there is not mail either",
			filepath.Join(dir, "absent"), false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := mailIsUnread(tc.path); got != tc.want {
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
