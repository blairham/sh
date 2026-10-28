// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp_test

import (
	"testing"

	"github.com/blairham/sh/internal/acp"
	"github.com/blairham/sh/internal/jsonrpc"
	"github.com/blairham/sh/interp"
)

// #786 measured that two of the three published adapters run their commands in
// their own process and call no client method at all — so a shell's policy on
// them covers file access and not the commands they run. Nothing here can force
// an agent to ask; what a client can do is *know*, and say so.
//
// The two facts are counted separately and neither is inferred from the other:
// there is no id joining an agent's tool call to a terminal it asked us for.
func TestAnAgentThatRunsItsOwnCommandsIsCountedApartFromOneThatAsks(t *testing.T) {
	t.Parallel()
	conn, client := withTerminals(t, nil)

	if asked, announced := client.Commands(); asked != 0 || announced != 0 {
		t.Fatalf("a client that has done nothing counts %d/%d", asked, announced)
	}

	// The agent announces two commands it ran in its own process. This is
	// exactly the shape measured off the published adapters: a tool call of
	// kind execute, and no client method behind it.
	announceID(t, conn, "call-1", acp.KindExecute)
	announceID(t, conn, "call-2", acp.KindExecute)
	if asked, announced := client.Commands(); asked != 0 || announced != 2 {
		t.Errorf("asked %d and announced %d, want 0 and 2", asked, announced)
	}

	// And one it asks for, which is a command *we* start.
	create(t, conn, acp.CreateTerminalRequest{SessionID: "s1", Command: "/bin/echo"})
	announceID(t, conn, "call-3", acp.KindExecute)
	if asked, announced := client.Commands(); asked != 1 || announced != 3 {
		t.Errorf("asked %d and announced %d, want 1 and 3", asked, announced)
	}
}

// A tool call that is not a command is not a command, and an update to one
// already announced is not a second: a command whose status changes three
// times has still been run once.
func TestOnlyTheAnnouncementOfAnExecutedCommandIsCounted(t *testing.T) {
	t.Parallel()
	conn, client := withTerminals(t, nil)
	announce(t, conn, acp.KindRead)
	announce(t, conn, acp.KindEdit)
	announce(t, conn, acp.KindOther)
	update(t, conn, acp.KindExecute)
	if _, announced := client.Commands(); announced != 0 {
		t.Errorf("announced %d, want none: nothing here was a command being announced", announced)
	}
}

// #6936. claude-code-acp 0.16.2 announces every Bash call twice under one
// toolCallId — once titled "Terminal" while its input streams, again with the
// command once it is whole — and then asks for it through terminal/create.
// Counted by message, every gated command read as one run ungated, and the
// notice told a person their policy was leaking when it was not.
func TestACommandAnnouncedTwiceAndAskedForIsOneCommandThatAsked(t *testing.T) {
	t.Parallel()
	conn, client := withTerminals(t, nil)
	announceID(t, conn, "toolu_1", acp.KindExecute)
	announceID(t, conn, "toolu_1", acp.KindExecute)
	create(t, conn, acp.CreateTerminalRequest{SessionID: "s1", Command: "/bin/echo"})
	asked, announced := client.Commands()
	if asked != 1 || announced != 1 {
		t.Errorf("asked %d and announced %d, want 1 and 1: one command, announced twice, asked for once", asked, announced)
	}
}

// The other direction, and the property the notice exists for: a command the
// agent announced — however many times — and never asked for is still one it
// ran in its own process, and must still be counted as such. Deduplicating
// must not make an ungated command disappear.
func TestACommandAnnouncedTwiceAndNeverAskedForIsStillReported(t *testing.T) {
	t.Parallel()
	conn, client := withTerminals(t, nil)
	announceID(t, conn, "toolu_1", acp.KindExecute)
	announceID(t, conn, "toolu_1", acp.KindExecute)
	// A second, ungated command in the same turn is a second command.
	announceID(t, conn, "toolu_2", acp.KindExecute)
	asked, announced := client.Commands()
	if asked != 0 || announced != 2 {
		t.Errorf("asked %d and announced %d, want 0 and 2: two commands ran ungated", asked, announced)
	}
}

// The count must not depend on whether the front end wired an Update hook, or
// what a person is told would change with which binary they ran.
func TestTheCountIsKeptWhetherOrNotAnybodyIsWatchingTheUpdates(t *testing.T) {
	t.Parallel()
	for _, watching := range []bool{false, true} {
		c := &acp.Client{Info: acp.Implementation{Name: "test-client", Version: "1"}, Terminals: true}
		if watching {
			c.Update = func(acp.SessionNotification) {}
		}
		conn := against(t, c, &agentSide{})
		announce(t, conn, acp.KindExecute)
		if _, announced := c.Commands(); announced != 1 {
			t.Errorf("with Update wired = %v: announced %d, want 1", watching, announced)
		}
	}
}

// A create the gate refuses still took the route, and the count is about the
// route: an agent that asks and is told no is not an agent that went around us.
func TestARefusedCreateStillCountsAsHavingAsked(t *testing.T) {
	t.Parallel()
	conn, client := withTerminals(t, &recorder{deny: func(interp.Action) bool { return true }})
	var resp acp.CreateTerminalResponse
	if err := conn.Call(t.Context(), acp.MethodCreateTerminal,
		acp.CreateTerminalRequest{SessionID: "s1", Command: "/bin/echo"}, &resp); err == nil {
		t.Fatal("a denied create was allowed")
	}
	if asked, _ := client.Commands(); asked != 1 {
		t.Errorf("asked %d, want 1: the agent took the route and was refused on it", asked)
	}
}

// announce sends the update an agent sends when it has run something itself.
func announce(t *testing.T, conn *jsonrpc.Conn, kind string) {
	t.Helper()
	notifyUpdate(t, conn, acp.UpdateToolCall, "call-1", kind)
}

// announceID is announce for a tool call with an id of the caller's choosing:
// the count is per tool call, so two commands need two ids.
func announceID(t *testing.T, conn *jsonrpc.Conn, id, kind string) {
	t.Helper()
	notifyUpdate(t, conn, acp.UpdateToolCall, id, kind)
}

// update sends a change to a tool call already announced.
func update(t *testing.T, conn *jsonrpc.Conn, kind string) {
	t.Helper()
	notifyUpdate(t, conn, acp.UpdateToolCallUpdate, "call-1", kind)
}

func notifyUpdate(t *testing.T, conn *jsonrpc.Conn, sessionUpdate, id, kind string) {
	t.Helper()
	err := conn.Notify(acp.MethodSessionUpdate, map[string]any{
		"sessionId": "s1",
		"update": map[string]any{
			"sessionUpdate": sessionUpdate,
			"toolCallId":    id,
			"kind":          kind,
			"status":        acp.StatusCompleted,
			"rawInput":      map[string]any{"command": "ls -la"},
		},
	})
	if err != nil {
		t.Fatalf("session/update: %v", err)
	}
	// A notification has no answer, so there is nothing to wait for on the
	// wire. One request that *does* answer puts this goroutine behind the
	// notification in the connection's own ordering, which is what makes the
	// count readable rather than racy.
	var out acp.TerminalOutputResponse
	_ = conn.Call(t.Context(), acp.MethodTerminalOutput,
		acp.TerminalRequest{SessionID: "s1", TerminalID: "no-such-terminal"}, &out)
}
