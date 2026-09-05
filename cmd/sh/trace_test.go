// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

func TestReadOwnFlagsReadsTheSeamFlags(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		trace bool
		deny  []string
		rest  []string
	}{
		{"neither", []string{"-c", "echo hi"}, false, nil, []string{"-c", "echo hi"}},
		{"trace alone", []string{"-trace-events", "-c", "echo hi"}, true, nil, []string{"-c", "echo hi"}},
		{"deny with the value next", []string{"-deny", "/etc", "x.sh"}, false, []string{"/etc"}, []string{"x.sh"}},
		{"deny attached with =", []string{"-deny=/etc", "x.sh"}, false, []string{"/etc"}, []string{"x.sh"}},
		{
			// Repeatable, because every separator a list could use is a
			// character a path may contain.
			"deny repeated",
			[]string{"-deny", "/etc", "-deny", "/var", "x.sh"},
			false,
			[]string{"/etc", "/var"},
			[]string{"x.sh"},
		},
		{
			"both, alongside a dialect",
			[]string{"-dialect", "bash", "-trace-events", "-deny", "/etc", "-c", "echo hi"},
			true,
			[]string{"/etc"},
			[]string{"-c", "echo hi"},
		},
		{
			// The front-anchoring rule these have to obey like the rest: the
			// scan stops at the script, so what follows is the script's.
			"after the script they are the script's",
			[]string{"x.sh", "-trace-events", "-deny", "/etc"},
			false, nil,
			[]string{"x.sh", "-trace-events", "-deny", "/etc"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			own, rest, err := readOwnFlags(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			if own.traceEvents != tc.trace {
				t.Errorf("traceEvents = %v, want %v", own.traceEvents, tc.trace)
			}
			if !slices.Equal(own.deny, tc.deny) {
				t.Errorf("deny = %q, want %q", own.deny, tc.deny)
			}
			if !slices.Equal(rest, tc.rest) {
				t.Errorf("rest = %q, want %q", rest, tc.rest)
			}
		})
	}
}

func TestReadOwnFlagsWantsAPathToDeny(t *testing.T) {
	if _, _, err := readOwnFlags([]string{"-deny"}); err == nil {
		t.Error("-deny with nothing after it must be refused, not read as an empty prefix")
	}
}

// A shell nobody pointed a policy at leaves both fields nil, and nil is the
// behavior every binary had before these flags existed: interp allows
// everything and discards every event, for one nil check.
func TestInstallSeamsLeavesAPlainShellAlone(t *testing.T) {
	sh := installSeams(driver.Shell{}, ownFlags{}, &bytes.Buffer{})
	if sh.Gate != nil {
		t.Error("a shell with no -deny must carry no gate")
	}
	if sh.Events != nil {
		t.Error("a shell with no -trace-events must carry no sink")
	}
}

func TestInstallSeamsWiresWhatWasAskedFor(t *testing.T) {
	var buf bytes.Buffer
	sh := installSeams(driver.Shell{}, ownFlags{deny: []string{"/etc"}}, &buf)
	if sh.Gate == nil {
		t.Error("-deny must reach the shell's gate")
	}
	if sh.Events != nil {
		t.Error("-deny alone must not install a sink")
	}

	sh = installSeams(driver.Shell{}, ownFlags{traceEvents: true}, &buf)
	if sh.Events == nil {
		t.Error("-trace-events must reach the shell's sink")
	}
	if sh.Gate != nil {
		t.Error("-trace-events alone must not install a gate")
	}
}

// Whole path components. A prefix matched by characters would refuse a
// neighboring directory because its name starts the same way, which is never
// what a person naming a directory meant.
func TestDenyPrefixesMatchesWholeComponents(t *testing.T) {
	d := denyPrefixes{"/etc", "/var/lib/"}
	for _, tc := range []struct {
		path string
		want interp.Decision
	}{
		{"/etc", interp.Deny},
		{"/etc/passwd", interp.Deny},
		{"/etc/ssl/certs/ca.pem", interp.Deny},
		{"/etcetera", interp.Allow},
		{"/etcd/data", interp.Allow},
		{"/var/lib/thing", interp.Deny},
		{"/var/libexec", interp.Allow},
		{"/usr/bin/cat", interp.Allow},
		{"", interp.Allow},
	} {
		got := d.Allow(context.Background(), interp.Action{Kind: interp.ActionStat, Path: tc.path})
		if got != tc.want {
			t.Errorf("%q: decision = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// An empty prefix would otherwise deny the whole filesystem by matching every
// path, which is what an unset flag reaching here would look like.
func TestDenyPrefixesIgnoresAnEmptyPrefix(t *testing.T) {
	d := denyPrefixes{""}
	if got := d.Allow(context.Background(), interp.Action{Path: "/anything"}); got != interp.Allow {
		t.Errorf("decision = %v, want an empty prefix to name nothing", got)
	}
}

// Every kind of action, so a person can watch the difference between a refusal
// that stops a command and one that quietly answers "not there".
func TestDenyPrefixesCoversEveryKindOfAction(t *testing.T) {
	d := denyPrefixes{"/secret"}
	for _, k := range []interp.ActionKind{
		interp.ActionExec, interp.ActionOpen, interp.ActionStat, interp.ActionReadDir,
	} {
		if got := d.Allow(context.Background(), interp.Action{Kind: k, Path: "/secret/f"}); got != interp.Deny {
			t.Errorf("%v: decision = %v, want Deny", k, got)
		}
	}
}

func TestFormatEventShowsWhatTheEventCarries(t *testing.T) {
	for _, tc := range []struct {
		name string
		e    interp.Event
		want string
	}{
		{
			"a command starting",
			interp.Event{
				Kind:   interp.EventCommandStart,
				Action: interp.Action{Kind: interp.ActionExec, Path: "/bin/echo", Args: []string{"echo", "hi"}},
				Line:   3,
				File:   "x.sh",
			},
			`trace: command-start exec /bin/echo args=["echo" "hi"] line=3 file=x.sh`,
		},
		{
			"a command ending",
			interp.Event{
				Kind:   interp.EventCommandEnd,
				Action: interp.Action{Kind: interp.ActionExec, Path: "/bin/false"},
				Status: 1,
				Line:   1,
			},
			"trace: command-end exec /bin/false status=1 line=1",
		},
		{
			// The direction is on the record, because an audit trail that
			// could not tell a read from a write would be worthless.
			"an open for writing",
			interp.Event{
				Kind:   interp.EventAccess,
				Action: interp.Action{Kind: interp.ActionOpen, Path: "/tmp/out", Write: true},
				Line:   2,
			},
			"trace: access open /tmp/out write=true line=2",
		},
		{
			"a refusal",
			interp.Event{
				Kind:   interp.EventDenied,
				Action: interp.Action{Kind: interp.ActionStat, Path: "/secret/f"},
				Line:   4,
			},
			"trace: denied stat /secret/f line=4",
		},
		{
			"a failure that was not a status",
			interp.Event{
				Kind:   interp.EventError,
				Action: interp.Action{Kind: interp.ActionExec, Path: "nope"},
				Err:    errors.New("not found"),
				Line:   9,
			},
			`trace: error exec nope err="not found" line=9`,
		},
		{
			"a directory read",
			interp.Event{
				Kind:   interp.EventAccess,
				Action: interp.Action{Kind: interp.ActionReadDir, Path: "/tmp"},
				Line:   1,
			},
			"trace: access read-dir /tmp line=1",
		},
		{
			// The one kind with no path: a signal names a process, so the
			// target and the number are what there is to print.
			"a signal",
			interp.Event{
				Kind:   interp.EventAccess,
				Action: interp.Action{Kind: interp.ActionSignal, PID: 4321, Signal: syscall.SIGTERM},
				Line:   7,
			},
			"trace: access signal pid=4321 signal=" + strconv.Itoa(int(syscall.SIGTERM)) + " line=7",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatEvent(tc.e); got != tc.want {
				t.Errorf("formatEvent =\n  %s\nwant\n  %s", got, tc.want)
			}
		})
	}
}

// A Sink is called from more than one goroutine, so the writer behind it has
// to be guarded. Unguarded, two events interleave into one unreadable line —
// and under -race this is what says so.
func TestTheTraceSinkIsSafeUnderConcurrentEmits(t *testing.T) {
	var buf bytes.Buffer
	s := &traceSink{w: &buf}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Emit(context.Background(), interp.Event{
				Kind:   interp.EventAccess,
				Action: interp.Action{Kind: interp.ActionStat, Path: "/p"},
				Line:   i,
			})
		}()
	}
	wg.Wait()
	if got := strings.Count(buf.String(), "\n"); got != 8 {
		t.Errorf("wrote %d lines, want 8 whole ones", got)
	}
}

// The end-to-end path this binary's flags exist to create: a real script, run
// through the shared front end, with the gate and the sink the invocation
// asked for.
//
// This is what nothing had before. The seam was reachable only from interp's
// own tests, so the conformance harness graded a shell that never ran gated
// and a hole in the boundary would have looked exactly like a shell that
// works.
func TestAGatedRunDeniesAndReports(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	if err := os.MkdirAll(secret, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secret, "f"), []byte("hidden\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "case.sh")
	src := "if [ -f " + secret + "/f ]; then echo there; else echo missing; fi\n" +
		"/bin/echo ran\n"
	if err := os.WriteFile(script, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	base, err := pickDialect("core")
	if err != nil {
		t.Fatal(err)
	}
	base.Name = "sh"

	// The control first: ungated, the file is found and the trace is empty
	// because nothing is watching.
	var out, errs bytes.Buffer
	plain := base
	plain.Stdout, plain.Stderr = &out, &errs
	if code := driver.MainArgs(plain, []string{"sh", script}); code != 0 {
		t.Fatalf("ungated status %d: %s", code, errs.String())
	}
	if !strings.Contains(out.String(), "there") {
		t.Fatalf("ungated out = %q, want the file to be found", out.String())
	}

	// Now with both flags, the way `sh -trace-events -deny <dir> case.sh`
	// reaches them.
	own, rest, err := readOwnFlags([]string{"-trace-events", "-deny", secret, script})
	if err != nil {
		t.Fatal(err)
	}
	var trace bytes.Buffer
	out.Reset()
	errs.Reset()
	gated := base
	gated.Stdout, gated.Stderr = &out, &errs
	gated = installSeams(gated, own, &trace)
	if code := driver.MainArgs(gated, append([]string{"sh"}, rest...)); code != 0 {
		t.Fatalf("gated status %d: %s", code, errs.String())
	}

	// The denial was observed by the script, as a path that is not there —
	// quietly, with nothing on the error stream to tell it apart from a file
	// that was never installed.
	if !strings.Contains(out.String(), "missing") {
		t.Errorf("out = %q, want the denied path to test as absent", out.String())
	}
	if errs.String() != "" {
		t.Errorf("err = %q, want a refused probe to say nothing", errs.String())
	}
	// And the event stream was produced, with the refusal in it: what the
	// script cannot tell, the observer can.
	if !strings.Contains(trace.String(), "trace: denied stat "+secret+"/f") {
		t.Errorf("trace =\n%s\nwant a denied stat for %s/f", trace.String(), secret)
	}
	if !strings.Contains(trace.String(), "trace: command-start exec ") {
		t.Errorf("trace =\n%s\nwant the command that ran to be recorded", trace.String())
	}
	if !strings.Contains(trace.String(), "trace: command-end exec ") {
		t.Errorf("trace =\n%s\nwant the status of the command that ran", trace.String())
	}
	// The trace is a stream of its own and never the shell's output: a
	// consumer redirecting the script's stdout must not collect it.
	if strings.Contains(out.String(), "trace:") {
		t.Errorf("out = %q, want the trace kept off the shell's output stream", out.String())
	}
}

// A signal a script sends reaches the trace, through the flag a person types.
//
// The kind arrived last and by a different route from the file actions —
// `kill` is a builtin, so nothing about a path was ever going to show it — and
// a kind the shipped binary cannot be made to print is a kind nothing outside
// interp's own tests would ever notice was missing.
func TestATracedRunRecordsASignal(t *testing.T) {
	base, err := pickDialect("core")
	if err != nil {
		t.Fatal(err)
	}
	base.Name = "sh"

	own, rest, err := readOwnFlags([]string{"-trace-events", "-c", "kill -0 $$"})
	if err != nil {
		t.Fatal(err)
	}
	var out, errs, trace bytes.Buffer
	sh := base
	sh.Stdout, sh.Stderr = &out, &errs
	sh = installSeams(sh, own, &trace)
	if code := driver.MainArgs(sh, append([]string{"sh"}, rest...)); code != 0 {
		t.Fatalf("status %d: %s", code, errs.String())
	}
	// The probe delivers nothing and is still an act worth recording: `kill
	// -0` exists to learn whether a process is there.
	want := "trace: access signal pid=" + strconv.Itoa(os.Getpid()) + " signal=0"
	if !strings.Contains(trace.String(), want) {
		t.Errorf("trace =\n%s\nwant %s", trace.String(), want)
	}
}
