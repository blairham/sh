// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// What the protocol costs, in ns/op that benchstat can read (#2038).
//
// `make acp` already prints these costs, and prints them better: in units a
// person reads, with an argument attached, next to what an editor pays
// instead. What it cannot do is be compared to itself. Its sample count is a
// constant in the source, its output is prose, and two runs a month apart can
// only be held against each other by eye. These say the same thing in a form a
// release can be diffed against the release before it.
//
// Not in the unit lane, for `make startup`'s reason: every row launches a real
// binary, speaks to a real child over a real pipe, and times it. Benchmarks do
// not run under `go test` without -bench, which is the whole of what keeps
// this out of everybody's build — `make acp-bench` is how to ask for it.
//
// An external test package on purpose. This file is a client of acpcheck the
// same way acpcheck is a client of the shell, and a benchmark that reached
// inside the package could end up timing a shortcut no real caller has.
package acpcheck_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/blairham/sh/internal/acpcheck"
)

// gatedCommand is the program the gated rows run, and the one the spawn rows
// run too.
//
// The same program in both, because the comparison is between two ways of
// running one thing — and taken from acpcheck rather than written out again,
// so the table `make acp` prints and the numbers here stay about the same
// work. Writing it out again is what put `/bin/true` in two places and made
// both of them wrong on macOS; see acpcheck.TrueCommand.
func gatedCommand(tb testing.TB) string {
	tb.Helper()
	prog := acpcheck.TrueCommand()
	if prog == "" {
		tb.Skip("no true(1) to run: nothing to time that is not the shell itself")
	}
	return prog
}

// opsPerConnection is how many timed operations a connection serves before it
// is replaced, untimed.
//
// Both ends accumulate. The agent holds a live shell for every session opened
// on a connection, and this client keeps every session update and every
// permission request it was sent, because grading needs them after the fact.
// Neither is a leak — no editor asks one connection for fifty thousand turns —
// but a benchmark pointed at a single connection drifts into measuring the
// accumulation rather than the operation, and drifts further the longer it is
// given. Rotating bounds it, outside the timer.
const opsPerConnection = 64

var (
	buildOnce sync.Once
	agentBin  string
	buildErr  error
	buildLog  string
)

// buildTheAgent builds the shipped binary into a directory of its own.
//
// Its own, rather than the tree's build/, because that is where `make acp`
// leaves an acp-sh as well. A benchmark that measured whichever of the two was
// written last would be reporting on the build system; #1403 lost several
// results to exactly that and they read as findings at the time.
func buildTheAgent() {
	dir, err := os.MkdirTemp("", "acpbench")
	if err != nil {
		buildErr = err
		return
	}
	bin := filepath.Join(dir, "acp-sh")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/sh")
	cmd.Dir = "../.."
	if out, err := cmd.CombinedOutput(); err != nil {
		buildErr, buildLog = err, string(out)
		return
	}
	// Warmed before anything is timed. The first execution of a freshly
	// linked binary on macOS pays a one-time validation the same binary never
	// pays again, and it lands in whichever sample happens to run first —
	// which is a fact about a build, not about a shell.
	if out, err := exec.Command(bin, "-c", ":").CombinedOutput(); err != nil {
		buildErr, buildLog = err, string(out)
		return
	}
	agentBin = bin
}

// agent is the binary to drive, built and warmed once for the whole run.
func agent(tb testing.TB) string {
	tb.Helper()
	buildOnce.Do(buildTheAgent)
	if buildErr != nil {
		tb.Skipf("cannot build the agent to measure: %v\n%s", buildErr, buildLog)
	}
	return agentBin
}

// dial launches the agent and completes the handshake: the state every row but
// Connect and Handshake starts from.
func dial(ctx context.Context, tb testing.TB, dir string) *acpcheck.Client {
	tb.Helper()
	c, err := acpcheck.Dial(agent(tb), acpcheck.Options{Args: []string{"-acp"}, Dir: dir})
	if err != nil {
		tb.Fatalf("dialing the agent: %v", err)
	}
	if _, err := c.Initialize(ctx); err != nil {
		_ = c.Close()
		tb.Fatalf("the handshake: %v", err)
	}
	return c
}

// rotating hands out a connection, replacing it every opsPerConnection calls.
//
// The replacement happens between timed regions, so what it costs is not
// attributed to the operation that triggered it.
type rotating struct {
	tb   testing.TB
	dir  string
	c    *acpcheck.Client
	sess string
	used int
}

// stale says whether the next call should start over on a fresh connection.
func (r *rotating) stale() bool { return r.c == nil || r.used >= opsPerConnection }

// conn returns a connection with no session opened on it, for the row that is
// timing the opening.
func (r *rotating) conn(ctx context.Context) *acpcheck.Client {
	if r.stale() {
		r.close()
		r.c, r.sess, r.used = dial(ctx, r.tb, r.dir), "", 0
	}
	r.used++
	return r.c
}

// session returns a connection with a session already open on it, for the rows
// that are timing what happens inside one.
func (r *rotating) session(ctx context.Context) (*acpcheck.Client, string) {
	if r.stale() {
		r.close()
		c := dial(ctx, r.tb, r.dir)
		s, err := c.NewSession(ctx, r.dir)
		if err != nil {
			_ = c.Close()
			r.tb.Fatalf("opening a session: %v", err)
		}
		r.c, r.sess, r.used = c, s, 0
	}
	r.used++
	return r.c, r.sess
}

func (r *rotating) close() {
	if r.c != nil {
		_ = r.c.Close()
		r.c = nil
	}
}

// BenchmarkConnect is what an editor pays once to attach: the process start
// and the handshake, together.
//
// Together, because apart they report a number nobody pays. The agent cannot
// answer `initialize` before it has started, so the handshake's round trip
// already contains most of a process start — timing the two separately invites
// a reader to add them up and count the start twice.
func BenchmarkConnect(b *testing.B) {
	bin, dir, ctx := agent(b), b.TempDir(), b.Context()
	var total time.Duration
	n := 0
	for b.Loop() {
		start := time.Now()
		c, err := acpcheck.Dial(bin, acpcheck.Options{Args: []string{"-acp"}, Dir: dir})
		if err != nil {
			b.Fatalf("dialing the agent: %v", err)
		}
		if _, err := c.Initialize(ctx); err != nil {
			_ = c.Close()
			b.Fatalf("the handshake: %v", err)
		}
		total, n = total+time.Since(start), n+1
		_ = c.Close()
	}
	report(b, total, n)
}

// BenchmarkHandshake is the `initialize` round trip alone, against an agent
// that has already started.
//
// The row `make acp` calls "handshake, once per connection", measured the way
// that instrument measures it so the two can be held against each other. On
// its own it flatters the protocol, since nobody is handed a started agent for
// free, which is why Connect is the one above it.
func BenchmarkHandshake(b *testing.B) {
	bin, dir, ctx := agent(b), b.TempDir(), b.Context()
	var total time.Duration
	n := 0
	for b.Loop() {
		c, err := acpcheck.Dial(bin, acpcheck.Options{Args: []string{"-acp"}, Dir: dir})
		if err != nil {
			b.Fatalf("dialing the agent: %v", err)
		}
		start := time.Now()
		if _, err := c.Initialize(ctx); err != nil {
			_ = c.Close()
			b.Fatalf("the handshake: %v", err)
		}
		total, n = total+time.Since(start), n+1
		_ = c.Close()
	}
	report(b, total, n)
}

// BenchmarkSessionNew is `session/new` on a connection already open: what a
// second shell costs an editor that already has one.
func BenchmarkSessionNew(b *testing.B) {
	ctx := b.Context()
	r := &rotating{tb: b, dir: b.TempDir()}
	defer r.close()
	var total time.Duration
	n := 0
	for b.Loop() {
		c := r.conn(ctx)
		start := time.Now()
		if _, err := c.NewSession(ctx, r.dir); err != nil {
			b.Fatalf("opening a session: %v", err)
		}
		total, n = total+time.Since(start), n+1
	}
	report(b, total, n)
}

// BenchmarkTurn is a turn in an open session that asks nobody anything: the
// floor, and the part of a turn that is protocol rather than work.
func BenchmarkTurn(b *testing.B) { benchmarkTurn(b, ":") }

// BenchmarkTurnGated is the row to read: a real command, announced to the
// client, approved, and run.
//
// It is the honest unit of the comparison against a spawn, because it is the
// whole of what the protocol is for. A turn that asked nobody anything would
// win the comparison by not doing the thing being paid for.
func BenchmarkTurnGated(b *testing.B) { benchmarkTurn(b, gatedCommand(b)) }

func benchmarkTurn(b *testing.B, line string) {
	b.Helper()
	ctx := b.Context()
	r := &rotating{tb: b, dir: b.TempDir()}
	defer r.close()
	var total time.Duration
	n := 0
	for b.Loop() {
		c, session := r.session(ctx)
		start := time.Now()
		if _, err := c.Prompt(ctx, session, line); err != nil {
			b.Fatalf("the turn: %v", err)
		}
		total, n = total+time.Since(start), n+1
	}
	report(b, total, n)
}

// BenchmarkSpawn is the arrangement the protocol replaces: a process per
// command, watched by nobody.
//
// In this file rather than a table of its own because the claim is a ratio,
// and a ratio whose halves were measured in different runs is not one. This
// machine is rarely quiet — the numbers in #2038 were taken at load 9 — and a
// spawn timed this morning divided by a turn timed last night says more about
// the two mornings than about either arrangement. Measured in the same run,
// under the same load, the division means something.
func BenchmarkSpawn(b *testing.B) {
	prog := gatedCommand(b)
	type subject struct{ name, path string }
	subjects := []subject{{"ours", agent(b)}}
	if bash, err := exec.LookPath("bash"); err == nil {
		subjects = append(subjects, subject{"bash", bash})
	} else {
		b.Logf("bash is not installed here, so not compared against: %v", err)
	}
	for _, s := range subjects {
		b.Run(s.name, func(b *testing.B) {
			ctx := b.Context()
			var total time.Duration
			n := 0
			for b.Loop() {
				start := time.Now()
				if err := exec.CommandContext(ctx, s.path, "-c", prog).Run(); err != nil {
					b.Fatalf("%s -c %s: %v", s.name, prog, err)
				}
				total, n = total+time.Since(start), n+1
			}
			report(b, total, n)
		})
	}
}

// report replaces the benchmark's own ns/op with the time the operation took.
//
// Necessary because several loop bodies here contain setup the timer should
// not see: a connection dialed so that a handshake can be timed without it, a
// rotation that falls due mid-loop. Timing the region by hand and reporting the
// mean is how those stay out of the number.
func report(b *testing.B, total time.Duration, n int) {
	b.Helper()
	if n == 0 {
		return
	}
	b.ReportMetric(float64(total.Nanoseconds())/float64(n), "ns/op")
}
