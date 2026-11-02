// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/testenv"
)

// collectorRoute names the entry point a re-executed copy of this test binary
// should go through before it reports the collector it was left with. Set, it
// means "you are the shell"; unset, "you are the test".
const collectorRoute = "SH_TEST_COLLECTOR_ROUTE"

// collectorProbe turns this process into a shell when it was re-executed as
// one, and does nothing otherwise. It runs `-c :` through one entry point and
// prints the collector's percent and soft limit as main left them.
//
// A second process is the only way to ask, for two reasons. The runtime reads
// GOGC and GOMEMLIMIT from the environment once, at start, so a case about an
// explicit setting has to be a process that started with it rather than a
// t.Setenv after the fact. And the routes under test change process-wide
// state, which a test binary must not be left holding.
func collectorProbe() {
	route := os.Getenv(collectorRoute)
	if route == "" {
		return
	}
	report := func(code int) {
		// Read, not restored: the process ends on the next line.
		percent := debug.SetGCPercent(-1)
		limit := debug.SetMemoryLimit(-1)
		fmt.Printf("%d %d %d\n", code, percent, limit)
		os.Exit(0)
	}
	switch route {
	case "sh":
		// main itself, with only its exit replaced so the report runs.
		os.Args = []string{"sh", "-c", ":"}
		exit = report
		main()
	case "run":
		// What every test in this package calls instead of main.
		report(run([]string{"sh", "-c", ":"}, nil, os.Stdout, os.Stderr))
	case "dialect":
		// A dialect binary's whole main, driver.Main, for the comparison.
		sh, err := pickDialect("bash")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Args = []string{"bash", "-c", ":"}
		report(driver.Main(sh))
	}
	fmt.Fprintf(os.Stderr, "unknown route %q\n", route)
	os.Exit(1)
}

// collectorAfter re-executes this test binary as a shell through route, with
// the environment's own GOGC and GOMEMLIMIT replaced by env, and returns the
// percent and the limit it reported.
func collectorAfter(t *testing.T, route string, env []string) (percent, limit int64) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if name != "GOGC" && name != "GOMEMLIMIT" && name != collectorRoute {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, collectorRoute+"="+route)
	cmd.Env = testenv.WithoutRaceExitPause(append(cmd.Env, env...))
	var errs strings.Builder
	cmd.Stderr = &errs
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("route %s %v: %v (stderr %q)", route, env, err, errs.String())
	}
	var code int
	if _, err := fmt.Sscan(string(out), &code, &percent, &limit); err != nil {
		t.Fatalf("route %s %v: output %q: %v", route, env, out, err)
	}
	if code != 0 {
		t.Fatalf("route %s %v: `-c :` exited %d (stderr %q)", route, env, code, errs.String())
	}
	return percent, limit
}

// #6393: this binary tunes the collector exactly as a dialect binary does,
// and respects GOGC and GOMEMLIMIT by the same rules — the driver's
// collector_test.go holds the rules; this holds that main reaches them. It
// read its own flags and called driver.MainArgs, which tunes nothing, so it
// ran at GOGC=100 with no ceiling while every dialect binary ran at 1600 with
// one, and its loop rows cost what the dash binary's did and more.
//
// And run, which every other test here calls, leaves the process alone: the
// tuning is main's and not run's for the reason driver's is Main's and not
// MainArgs'.
func TestTheCoreBinaryTunesTheCollectorAsADialectBinaryDoes(t *testing.T) {
	t.Parallel()
	const (
		tuned     = 1600
		ceiling   = 384 << 20
		unlimited = math.MaxInt64
		runtimes  = 100 // the runtime's own percent when GOGC is unset
	)
	cases := []struct {
		name       string
		env        []string
		percent    int64
		limit      int64
		runPercent int64 // what run leaves: the runtime's own reading
		runLimit   int64
	}{
		{"unset", nil, tuned, ceiling, runtimes, unlimited},
		{"GOGC=50", []string{"GOGC=50"}, 50, unlimited, 50, unlimited},
		{"GOGC=off", []string{"GOGC=off"}, -1, unlimited, -1, unlimited},
		{"GOMEMLIMIT=1GiB", []string{"GOMEMLIMIT=1GiB"}, tuned, 1 << 30, runtimes, 1 << 30},
		{"both", []string{"GOGC=200", "GOMEMLIMIT=200MiB"}, 200, 200 << 20, 200, 200 << 20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			for _, route := range []string{"sh", "dialect"} {
				p, l := collectorAfter(t, route, c.env)
				if p != c.percent || l != c.limit {
					t.Errorf("%s binary: percent %d limit %d, want %d and %d", route, p, l, c.percent, c.limit)
				}
			}
			p, l := collectorAfter(t, "run", c.env)
			if p != c.runPercent || l != c.runLimit {
				t.Errorf("run: percent %d limit %d, want %d and %d, the runtime's own", p, l, c.runPercent, c.runLimit)
			}
		})
	}
}
