// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package cross_test

import (
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/internal/dialecttest"
)

// A trapped signal the shell sends a background body sitting in `wait` cuts
// the body's wait short: the handler runs at once and `wait` reports 128+N —
// ksh93 256+N for a bare `wait` and 1 for a named one — rather than waiting
// out the body's own job and reporting 0 (#7086). Measured 2026-11-02 under
// `-c`, each shell in a process group of its own: bash 5.3.20, bash 3.2.57,
// zsh 5.9.2, ksh93u+ and dash all answer at 0.3 s, where this shell answered
// as the inner job ended. An ignored signal waits the job out in all five.
// See interp/waitinabody.go.
func TestATrappedSignalCutsABodysWaitShort(t *testing.T) {
	const body = `( trap "echo T" TERM; /bin/sleep 2 & WAIT; echo after $? ) & ` +
		`/bin/sleep 0.2; kill $!; wait %1; echo st=$?`
	for _, dialect := range []string{"bash", "zsh", "dash", "ksh"} {
		bare, named := "143", "143"
		if dialect == "ksh" {
			bare, named = "271", "1"
		}
		for _, c := range []struct {
			src, want string
			atMost    time.Duration
		}{
			{strings.Replace(body, "WAIT", "wait", 1), "T after " + bare + " st=0", 1500 * time.Millisecond},
			{strings.Replace(body, "WAIT", "wait $!", 1), "T after " + named + " st=0", 1500 * time.Millisecond},
			// The control: an ignored signal queues nothing, so nothing
			// wakes the wait, which runs to the inner job's end.
			{
				`( trap "" USR1; /bin/sleep 0.6 & wait; echo after $? ) & ` +
					`/bin/sleep 0.2; kill -USR1 $!; wait %1; echo st=$?`,
				"after 0 st=0", 0,
			},
			// And a trapped one the body's own copy aims at the shell's
			// pid is the shell's, not the body's: the shell's handler
			// runs, and the body's wait is not cut short.
			{
				`trap "echo S" USR1; ( trap "echo U" USR1; ( kill -USR1 $$; /bin/sleep 0.3 ) & wait $!; echo w=$? ); echo st=$?`,
				"w=0 S st=0", 0,
			},
		} {
			began := time.Now()
			out, st, err := presets[dialect].Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
			took := time.Since(began)
			if err != nil {
				t.Fatalf("%s: %s: %v", dialect, c.src, err)
			}
			if got := strings.Join(strings.Fields(out), " "); got != c.want || st != 0 {
				t.Errorf("%s: %s\n got %q at %d\nwant %q at 0", dialect, c.src, got, st, c.want)
			}
			if c.atMost != 0 && took > c.atMost {
				t.Errorf("%s: %s\ntook %v, want under %v: the wait was not cut short", dialect, c.src, took, c.atMost)
			}
		}
	}
}

// A `&` job that aims a trapped signal at the shell before its `$!` has
// settled lands, as in a shell that forks, once the shell has gone on — so a
// `wait` that follows is cut short by it, and a script that ends there never
// runs the handler. Measured 2026-11-02 the same way, all five agreeing but
// for ksh93's status for a named wait (1) and a bare one (286); this shell
// answered `U` and then `w=0` after the job had ended, and ran the handler as
// the script ended. See interp/waitinabody.go.
func TestAJobsSignalToTheShellCutsTheWaitAfterIt(t *testing.T) {
	usr1 := 128 + int(syscall.SIGUSR1)
	for _, dialect := range []string{"bash", "zsh", "dash", "ksh"} {
		named, bare := strconv.Itoa(usr1), strconv.Itoa(usr1)
		if dialect == "ksh" {
			named, bare = "1", strconv.Itoa(256+int(syscall.SIGUSR1))
		}
		const trap = `trap "echo U" USR1; `
		for _, c := range []struct {
			src, want string
			atMost    time.Duration
		}{
			{trap + `kill -USR1 $$ & wait $!; echo w=$?`, "U w=" + named, 0},
			{trap + `( kill -USR1 $$; /bin/sleep 2 ) & wait; echo w=$?`, "U w=" + bare, time.Second},
			{trap + `( kill -USR1 $$; /bin/sleep 2 ) & wait %1; echo w=$?`, "U w=" + named, time.Second},
			{trap + `kill -USR1 $$ &`, "", 0},
		} {
			began := time.Now()
			out, st, err := presets[dialect].Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
			took := time.Since(began)
			if err != nil {
				t.Fatalf("%s: %s: %v", dialect, c.src, err)
			}
			if got := strings.Join(strings.Fields(out), " "); got != c.want || st != 0 {
				t.Errorf("%s: %s\n got %q at %d\nwant %q at 0", dialect, c.src, got, st, c.want)
			}
			if c.atMost != 0 && took > c.atMost {
				t.Errorf("%s: %s\ntook %v, want under %v: the wait was not cut short", dialect, c.src, took, c.atMost)
			}
		}
	}
}
