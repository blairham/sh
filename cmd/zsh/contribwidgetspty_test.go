// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/smoke"
)

// The contrib widgets this shell ships, driven through a real session on a
// pseudo-terminal with the shipped files on `$fpath`: the keys are the keys a
// person presses, and what the widget left is read back through a widget that
// writes the line, quoted as `$'…'` so that one record is one line of the
// file, and the cursor, so the assertion is on the line and not on how it was
// drawn.
//
// Every expected line below was measured 2026-10-04 through a pseudo-terminal
// against /opt/homebrew/bin/zsh (zsh 5.9.2), with its own copies of these
// functions reached through its default `$fpath`, the same keys and the same
// history. docs/spec/functions.md has the tables.

// contribSession starts a session whose startup file puts the shipped
// functions first on `$fpath`, runs rc, and binds `^G` to the widget that
// records the line.
func contribSession(t *testing.T, rc string) (control *os.File, screen *smoke.Screen, home string) {
	t.Helper()
	shipped, err := filepath.Abs(filepath.Join("..", "..", "share", "sh", "functions"))
	if err != nil {
		t.Fatal(err)
	}
	return jobNoticeSessionRC(t, "fpath=("+shipped+")\n"+
		`record() { print -r -- "${(qqqq)BUFFER}|$CURSOR" >> $HOME/recorded }`+"\n"+
		"zle -N record\nbindkey '^G' record\n"+rc, "zsh", "-i")
}

// contribRecorded presses the recording key and answers the line it wrote:
// the n-th line of the file, counted from 1, waited for.
func contribRecorded(t *testing.T, control *os.File, screen *smoke.Screen, home string, n int) string {
	t.Helper()
	if _, err := control.WriteString("\a"); err != nil {
		t.Fatalf("pressing the recording key: %v", err)
	}
	for deadline := time.Now().Add(jobNoticeBudget); ; time.Sleep(10 * time.Millisecond) {
		b, _ := os.ReadFile(filepath.Join(home, "recorded"))
		lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
		if len(b) > 0 && len(lines) >= n {
			return lines[n-1]
		}
		if time.Now().After(deadline) {
			t.Fatalf("no recording %d; recorded %q\n%s", n, b, smoke.Readable(smoke.LastLines(screen.Text(), 6)))
		}
	}
}

// contribAwaitRow waits for the prompt's own row to read want, which is how a
// key that changed the line is known to have been read before the next one is
// sent.
func contribAwaitRow(t *testing.T, screen *smoke.Screen, want string) {
	t.Helper()
	for deadline := time.Now().Add(jobNoticeBudget); ; time.Sleep(10 * time.Millisecond) {
		g := grid(screen)
		if p := promptRow(g); p >= 0 && g.Text(p) == jobNoticeMark+want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the line never read %q:\n%s", want, grid(screen))
		}
	}
}

func contribSend(t *testing.T, control *os.File, keys string) {
	t.Helper()
	if _, err := control.WriteString(keys); err != nil {
		t.Fatalf("typing %q: %v", keys, err)
	}
}

// up-line-or-beginning-search and its partner: a run of presses walks every
// entry beginning with what was before the cursor when the run started, and
// walking forward past the newest brings back the typed line with the cursor
// where it was.
func TestTheBeginningSearchWidgetsWalkTheMatches(t *testing.T) {
	control, screen, home := contribSession(t, `autoload -Uz up-line-or-beginning-search down-line-or-beginning-search
zle -N up-line-or-beginning-search
zle -N down-line-or-beginning-search
bindkey '^P' up-line-or-beginning-search
bindkey '^N' down-line-or-beginning-search
setb() { BUFFER=$'echo abc\ndef ghi'; CURSOR=13 }
zle -N setb
bindkey '^T' setb
`)
	for _, line := range []string{": echo apple", ": ls one", ": echo banana", ": echo apple", ": ls two", ": echo cherry"} {
		jobNoticeType(t, control, screen, line)
	}
	contribSend(t, control, ": ec")
	contribAwaitRow(t, screen, ": ec")
	for _, step := range []struct{ key, want string }{
		{"\x10", ": echo cherry"},
		{"\x10", ": echo apple"},
		{"\x10", ": echo banana"},
		{"\x0e", ": echo apple"},
		{"\x0e", ": echo cherry"},
		{"\x0e", ": ec"},
	} {
		contribSend(t, control, step.key)
		contribAwaitRow(t, screen, step.want)
	}
	if got, want := contribRecorded(t, control, screen, home, 1), "$': ec'|4"; got != want {
		t.Errorf("back at the typed line: %q, want %q", got, want)
	}

	// Nothing matching: up leaves the line and puts the cursor at its end,
	// down leaves the cursor where it was.
	contribSend(t, control, "\x15zzz\x02\x02")
	contribAwaitRow(t, screen, "zzz")
	contribSend(t, control, "\x10")
	if got, want := contribRecorded(t, control, screen, home, 2), "$'zzz'|3"; got != want {
		t.Errorf("up with no match: %q, want %q", got, want)
	}
	contribSend(t, control, "\x02\x02\x0e")
	if got, want := contribRecorded(t, control, screen, home, 3), "$'zzz'|1"; got != want {
		t.Errorf("down with no match: %q, want %q", got, want)
	}

	// A line holding two: up moves to the first, keeping the column; up again
	// searches for `echo`, finds nothing, and goes to the end of that line.
	contribSend(t, control, "\x15\x14\x10")
	if got, want := contribRecorded(t, control, screen, home, 4), `$'echo abc\ndef ghi'|4`; got != want {
		t.Errorf("up inside two lines: %q, want %q", got, want)
	}
	contribSend(t, control, "\x10")
	if got, want := contribRecorded(t, control, screen, home, 5), `$'echo abc\ndef ghi'|8`; got != want {
		t.Errorf("up from the first of two lines: %q, want %q", got, want)
	}
}

// url-quote-magic: typed a character at a time, a URL's glob and separator
// characters go in behind a backslash; a word that is quoted, or has no
// scheme, is left alone; and for a command that does its own globbing only
// the separators of a local URL are quoted.
func TestURLQuoteMagicQuotesWhatTheShellWouldReadSpecially(t *testing.T) {
	control, screen, home := contribSession(t, `autoload -Uz url-quote-magic
zle -N self-insert url-quote-magic
`)
	for i, tc := range []struct{ typed, want string }{
		{"curl http://x.y/?a=1&b=2", `$'curl http://x.y/\\?a\\=1\\&b\\=2'|28`},
		{`curl "http://x.y/?a=1`, `$'curl "http://x.y/?a=1'|21`},
		{"curl x.y/?a=1", `$'curl x.y/?a=1'|13`},
		{"curl HTTP://x/?a", `$'curl HTTP://x/?a'|16`},
		{"noglob curl ftp://h/*?;", `$'noglob curl ftp://h/*?\\;'|24`},
		{"noglob curl http://x/*?;", `$'noglob curl http://x/\\*\\?\\;'|27`},
		{"ls; noglob curl ftp://h/*;", `$'ls; noglob curl ftp://h/\\*\\;'|28`},
		{"curl http://x/a%+,-.:@_", `$'curl http://x/a%+,-.:@_'|23`},
	} {
		contribSend(t, control, "\x15"+tc.typed)
		if got := contribRecorded(t, control, screen, home, i+1); got != tc.want {
			t.Errorf("typed %q: got %q, want %q", tc.typed, got, tc.want)
		}
	}
}

// edit-command-line: the editor gets a file holding the line, is told where
// the cursor is when its name says it is a vim, and what it leaves in the
// file is the line afterwards, with the cursor at the same offset and the
// file gone.
func TestEditCommandLineRunsTheEditorOnTheLine(t *testing.T) {
	control, screen, home := contribSession(t, `autoload -Uz edit-command-line
zle -N edit-command-line
bindkey '^X^E' edit-command-line
TMPPREFIX=$HOME/tmpq
mkdir -p $HOME/bin
print -r -- '#!/bin/sh
for a; do f=$a; done
printf "edited %s\n" "$(cat "$f")" > "$f"
printf "%s|" "$@" > "$HOME/argv"' > $HOME/bin/vim
chmod +x $HOME/bin/vim
VISUAL=$HOME/bin/vim
`)
	contribSend(t, control, "echo abc def\x02\x02")
	contribAwaitRow(t, screen, "echo abc def")
	contribSend(t, control, "\x18\x05")
	contribAwaitRow(t, screen, "edited echo abc def")
	if got, want := contribRecorded(t, control, screen, home, 1), `$'edited echo abc def'|10`; got != want {
		t.Errorf("after the editor: %q, want %q", got, want)
	}
	b, err := os.ReadFile(filepath.Join(home, "argv"))
	if err != nil {
		t.Fatalf("the editor did not run: %v", err)
	}
	args := strings.Split(strings.TrimSuffix(string(b), "|"), "|")
	if len(args) != 4 || args[0] != "-c" || args[1] != "normal! 11go" || args[2] != "--" {
		t.Fatalf("the editor was handed %q, want -c, normal! 11go, -- and the file", args)
	}
	file := args[3]
	if dir, name := filepath.Split(file); dir != home+"/" || !strings.HasPrefix(name, "tmpq") ||
		!strings.HasSuffix(name, ".zsh") || len(name) != len("tmpq")+6+len(".zsh") {
		t.Errorf("the file was %q, want $TMPPREFIX, six characters and .zsh", file)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("the file %s is still there afterwards (%v)", file, err)
	}
}
