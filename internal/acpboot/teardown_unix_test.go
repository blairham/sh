// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package acpboot

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acpcheck"
)

// The test binary doubles as acpcheck's scripted agent, the same trick that
// package's own tests and `make acp` use, rather than a second agent written
// for this one test.
func TestMain(m *testing.M) {
	if status, ok := acpcheck.AgentFromEnv(); ok {
		os.Exit(status)
	}
	os.Exit(m.Run())
}

// TestAnAgentThatOutlivesItsInputIsEnded is #6935: `-acp-connect` waited for
// the agent with no deadline, and an agent that does not leave when its input
// closes — npx launching claude-code-acp is one — held the shell forever. The
// scripted agent here is worse than that one: it ignores SIGTERM, so does the
// child it started, and the child holds the connection's output open.
func TestAnAgentThatOutlivesItsInputIsEnded(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(t.TempDir(), "report.json")
	for _, kv := range acpcheck.AgentEnv("linger", report) {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	grace, delay := agentGrace, agentKillDelay
	agentGrace, agentKillDelay = 200*time.Millisecond, 300*time.Millisecond
	t.Cleanup(func() { agentGrace, agentKillDelay = grace, delay })

	// One prompt, then end of input — the ordinary end of a session.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.WriteString("do the thing\n")
	_ = w.Close()
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	stdin, stdout := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = r, devnull
	t.Cleanup(func() { os.Stdin, os.Stdout = stdin, stdout; _ = r.Close(); _ = devnull.Close() })

	done := make(chan int, 1)
	start := time.Now()
	go func() { done <- connectACP("sh", "-", driver.Shell{}, true, "", "", []string{self}) }()

	const bound = 10 * time.Second
	select {
	case status := <-done:
		if status != 0 {
			t.Errorf("status %d, want 0: the session itself went fine", status)
		}
		t.Logf("returned in %v", time.Since(start).Round(time.Millisecond))
	case <-time.After(bound):
		// Clean up by pid, never by group: unfixed, the agent shares this
		// test's group, and signaling that would end the test with it.
		for _, pid := range pids(t, report) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		t.Fatalf("-acp-connect was still waiting on the agent after %v", bound)
	}

	ps := pids(t, report)
	if len(ps) != 2 {
		t.Fatalf("the agent reported pids %v, want itself and its child", ps)
	}
	// Every process the agent was made of is gone. Polled, briefly, because
	// the orphaned child is reaped by init on init's schedule rather than ours
	// and a zombie still answers a signal.
	deadline := time.Now().Add(3 * time.Second)
	for _, pid := range ps {
		for syscall.Kill(pid, 0) == nil {
			if time.Now().After(deadline) {
				_ = syscall.Kill(pid, syscall.SIGKILL)
				t.Fatalf("pid %d from the agent outlived -acp-connect", pid)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	if err := syscall.Kill(-ps[0], 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("the agent's process group still has members: %v", err)
	}
}

func pids(t *testing.T, report string) []int {
	t.Helper()
	b, err := os.ReadFile(report)
	if err != nil {
		t.Fatalf("the agent left no report: %v", err)
	}
	var r acpcheck.AgentReport
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	if r.Error != "" {
		t.Fatalf("the agent could not start its child: %s", r.Error)
	}
	return r.Pids
}
