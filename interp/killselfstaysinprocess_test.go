// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A signal one Runner aims at itself never reaches another Runner in the same
// process — #7040, in the shape it was found in.
//
// Two Runners share this test binary. The *listener* traps every signal the
// rows send and records any it is handed; the *sender* aims one at `$$` and
// carries on. `$$` is the process, and the process is both of them, so a send
// that went through kill(2) is delivered to the listener by os/signal — which
// is how a row of #7032's tests that ignored USR1 ran two other rows' USR1
// handlers, three times in four, with nothing in either of them sending one.
//
// The order is the one those parallel rows happened to fall into when they
// failed: the sender ignores first and the listener traps second. It was the
// whole of the arrangement while `trap ” SIG` was signal.Ignore, which took
// *every* subscription in the process away with it, so a listener that
// subscribed first would have been deaf to the leak and every row would have
// passed on a tree that still had it. A Runner no front end has handed the
// process's dispositions keeps an ignore to itself now (#7044), and
// TestOneRunnersIgnoreLeavesAnotherRunnersHandlerAlone holds both orders.
// Each Runner waits on a builtin the test controls, so the sequence is the
// test's and not the scheduler's.
//
// The last row is the control, and the reason the others mean anything: the
// same sequence with the signal sent by a process that is *not* this one
// reaches the listener, as it should. A listener that could not hear that
// could not hear a leak either.
func TestASignalAimedAtThisShellReachesNoOtherRunner(t *testing.T) {
	// Every signal a row sends. URG is left out because this runtime raises
	// it for itself — see urgisthisruntimes.go — and a listener trapping it
	// would hear the scheduler; CHLD because a kernel CHLD is never an
	// arrival here at all — see signalState.forwarded. Both are rows of
	// TestTheGateIsNotAskedAboutASignalThatNeverLeaves, which reads the send
	// itself rather than a listener.
	heard := []string{"USR1", "USR2", "TERM", "WINCH", "CONT", "TSTP", "TTIN", "TTOU"}
	var listen strings.Builder
	for _, name := range heard {
		listen.WriteString("trap 'echo heard " + name + "' " + name + "\n")
	}
	listen.WriteString("step ready\necho done\n")
	usr1 := itoa(int(syscall.SIGUSR1))

	for _, tc := range []struct {
		name string
		// ignore is what the sender ignores before the listener starts, and
		// send is what it then does to itself.
		ignore, send string
		// want is what the sender writes, and heard is what the listener
		// must — the control's, and nobody else's.
		want, heard string
	}{
		{name: "ignored, by name", ignore: "USR1", send: "kill -USR1 $$", want: "st=0\n"},
		{name: "ignored, by number", ignore: "USR1", send: "kill -" + usr1 + " $$", want: "st=0\n"},
		{name: "ignored, through -s", ignore: "USR2", send: "kill -s USR2 $$", want: "st=0\n"},
		{name: "ignored, the default signal", ignore: "TERM", send: "kill $$", want: "st=0\n"},
		{name: "ignored, twice over", ignore: "USR1", send: "kill -USR1 $$ $$", want: "st=0\n"},
		{
			name: "ignored, from a subshell", ignore: "USR1",
			send: `(kill -USR1 $$; echo "in=$?")`, want: "in=0\nst=0\n",
		},
		{name: "ignored, a stop", ignore: "TSTP", send: "kill -TSTP $$", want: "st=0\n"},
		{name: "a default that discards", send: "kill -WINCH $$", want: "st=0\n"},
		{name: "a continue", send: "kill -CONT $$", want: "st=0\n"},
		{name: "a keyboard stop", send: "kill -TSTP $$", want: "st=0\n"},
		{name: "a terminal-read stop", send: "kill -TTIN $$", want: "st=0\n"},
		{name: "a terminal-write stop", send: "kill -TTOU $$", want: "st=0\n"},
		{name: "trapped by the sender", send: "trap 'echo mine' USR1; kill -USR1 $$", want: "mine\nst=0\n"},
		{
			name: "the control: sent from outside", ignore: "USR1",
			send: externalKill(t) + " -USR1 $$", want: "st=0\n", heard: "heard USR1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restoreDispositionsAfter(t, heard)
			var prep string
			if tc.ignore != "" {
				prep = "trap '' " + tc.ignore + "\n"
			}
			sender := startStepped(t, prep+"step ignoring\n"+tc.send+"\necho \"st=$?\"\nstep finish\n")
			sender.reach(t, "ignoring")
			listener := startStepped(t, listen.String())
			listener.reach(t, "ready")

			sender.release()
			sender.reach(t, "finish")
			// Long enough for os/signal to have forwarded anything the
			// kernel delivered: the control row is what says so.
			time.Sleep(200 * time.Millisecond)

			listener.release()
			listener.wait(t)
			sender.release()
			sender.wait(t)

			if got, want := sender.out.String(), tc.want; got != want {
				t.Errorf("the sender wrote %q, want %q", got, want)
			}
			if got, want := listener.out.String(), tc.heard+"done\n"; got != want {
				t.Errorf("the listener wrote %q, want %q — a signal left the sender's process", got, want)
			}
		})
	}
}

// externalKill is a kill(1) that is not this shell's builtin, for the control:
// a real sender in another process.
func externalKill(t *testing.T) string {
	t.Helper()
	for _, p := range []string{"/bin/kill", "/usr/bin/kill"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Fatal("no external kill to send the control with: without it nothing proves the listener can hear")
	return ""
}

// restoreDispositionsAfter is the safety net under the reverse-order finish
// above: should a row fail half way, the process is not left ignoring a
// signal for every later test and every child they start (#2446).
func restoreDispositionsAfter(t *testing.T, names []string) {
	t.Helper()
	t.Cleanup(func() {
		for _, name := range names {
			sig := signalByName(name)
			if signal.Ignored(sig) {
				signal.Notify(make(chan os.Signal, 1), sig)
			}
		}
	})
}

func signalByName(name string) syscall.Signal {
	switch name {
	case "USR1":
		return syscall.SIGUSR1
	case "USR2":
		return syscall.SIGUSR2
	case "TERM":
		return syscall.SIGTERM
	case "WINCH":
		return syscall.SIGWINCH
	case "CONT":
		return syscall.SIGCONT
	case "TSTP":
		return syscall.SIGTSTP
	case "TTIN":
		return syscall.SIGTTIN
	case "TTOU":
		return syscall.SIGTTOU
	}
	panic("no signal named " + name)
}

// stepped is a Runner running on a goroutine of its own, which stops at every
// `step NAME` until the test lets it go.
type stepped struct {
	out     output
	at      chan string
	proceed chan struct{}
	done    chan error
	stop    sync.Once
}

func startStepped(t *testing.T, src string) *stepped {
	t.Helper()
	return startSteppedWith(t, src, nil)
}

// startSteppedWith is startStepped with the Runner handed to setup before it
// starts, for a test about a field the helper leaves alone.
func startSteppedWith(t *testing.T, src string, setup func(*Runner)) *stepped {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	s := &stepped{at: make(chan string), proceed: make(chan struct{}), done: make(chan error, 1)}
	sem := testSemantics()
	r := newTestRunner(t, &Runner{Stdout: &s.out, Stderr: &s.out, Semantics: &sem, Env: testPATH()})
	if setup != nil {
		setup(r)
	}
	r.Register("step", func(_ *Runner, ctx context.Context, args []string) int {
		select {
		case s.at <- strings.Join(args, " "):
		case <-ctx.Done():
			return 1
		}
		<-s.proceed
		return 0
	})
	// A row that fails half way leaves a Runner parked on a step; letting
	// every step through is what lets it finish before the next row starts.
	t.Cleanup(func() {
		s.stop.Do(func() { close(s.proceed) })
		<-s.done
	})
	go func() {
		_, err := r.Run(context.Background(), f)
		s.done <- err
		close(s.done)
	}()
	return s
}

func (s *stepped) reach(t *testing.T, want string) {
	t.Helper()
	select {
	case got := <-s.at:
		if got != want {
			t.Fatalf("reached step %q, want %q", got, want)
		}
	case err := <-s.done:
		t.Fatalf("finished (%v) before step %q, having written %q", err, want, s.out.String())
	case <-time.After(30 * time.Second):
		t.Fatalf("never reached step %q, having written %q", want, s.out.String())
	}
}

func (s *stepped) release() { s.proceed <- struct{}{} }

func (s *stepped) wait(t *testing.T) {
	t.Helper()
	select {
	case err, ok := <-s.done:
		if ok && err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("never finished, having written %q", s.out.String())
	}
}
