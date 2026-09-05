// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/acp"
	"github.com/blairham/sh/internal/pty"
)

// Terminal authentication is claimed only where it can be honored, and here
// that means a terminal on both streams. A shell whose prompts arrive on a
// pipe cannot show anybody a login, and an agent told otherwise offers one
// that draws nothing.
func TestTerminalAuthNeedsATerminalOnBothStreams(t *testing.T) {
	control, term, err := pty.Open()
	if err != nil {
		if errors.Is(err, pty.ErrUnsupported) {
			t.Skip("no pseudo-terminal on this platform")
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close(); _ = term.Close() })
	// The negative half needs something that is definitely not a terminal, and
	// the null device is the one every platform here has.
	pipe, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pipe.Close() })

	for _, tc := range []struct {
		name     string
		in, out  *os.File
		wantHook bool
	}{
		{"both a terminal", term, term, true},
		{"input is not", pipe, term, false},
		{"output is not", term, pipe, false},
		{"neither", pipe, pipe, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hook := terminalAuth([]string{"npx", "pkg"}, tc.in, tc.out)
			if (hook != nil) != tc.wantHook {
				t.Errorf("relaunch hook present = %v, want %v", hook != nil, tc.wantHook)
			}
		})
	}
}

// The login is the agent's own invocation with the method's arguments after
// it, not instead of it: the method describes what to *add*, and an agent
// relaunched without its own flags is a different program.
func TestALoginRepeatsTheAgentsOwnInvocation(t *testing.T) {
	t.Setenv("SH_ACP_TEST_VAR", "inherited")
	login := loginCommand(t.Context(),
		[]string{"npx", "pkg", "--acp"},
		[]string{"/login", "--force"},
		map[string]string{"SH_ACP_TEST_VAR": "overridden"})

	want := []string{"npx", "pkg", "--acp", "/login", "--force"}
	if !slices.Equal(login.Args, want) {
		t.Errorf("argv = %v, want %v", login.Args, want)
	}
	// The environment is inherited and then written over, and os/exec keeps
	// the last of a repeated name — which is what "these values override
	// same-named variables" asks for. Read from the end for that reason.
	got := ""
	for _, kv := range login.Env {
		if v, ok := strings.CutPrefix(kv, "SH_ACP_TEST_VAR="); ok {
			got = v
		}
	}
	if got != "overridden" {
		t.Errorf("SH_ACP_TEST_VAR = %q, want the method's value to win", got)
	}
}

// The method a person named has to reach the client, and a name the agent
// never advertised has to end the run rather than be dropped on the way. This
// shell's own agent side advertises none, which makes it exactly the agent
// that must refuse.
func TestTalkCarriesTheNamedAuthMethod(t *testing.T) {
	a, b := net.Pipe()
	agent := acp.NewAgent(driver.Shell{Name: "sh"}, acp.Implementation{Name: "sh", Version: "test"})
	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = agent.Serve(ctx, a, a) }()
	client := &acp.Client{Info: acp.Implementation{Name: "sh", Version: "test"}}
	client.Connect(b, b)
	go func() { defer wg.Done(); _ = client.Serve(ctx) }()
	t.Cleanup(func() {
		agent.Close(context.Background())
		cancel()
		_ = a.Close()
		_ = b.Close()
		wg.Wait()
	})

	if code := talk(ctx, client, "no-such-method"); code == 0 {
		t.Error("a method the agent never advertised was reported as a success")
	}
}
