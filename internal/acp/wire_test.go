// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp_test

import (
	"encoding/json"
	"testing"

	"github.com/blairham/sh/internal/acp"
)

// The shapes here are the protocol's, not ours, so what these tests pin down
// is that the Go types encode to what the published schema describes. A field
// name that drifts is a message a client silently ignores.

func encode(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

// A tool call update is a *flat* object carrying its discriminator beside the
// tool call's own fields, which is why the tool call is embedded anonymously.
// Nesting it under a key would be a message no client reads.
func TestNoticeInlinesTheToolCallBesideItsDiscriminator(t *testing.T) {
	t.Parallel()
	got := encode(t, acp.Notice{
		SessionUpdate: acp.UpdateToolCall,
		ToolCall: acp.ToolCall{
			ToolCallID: "call-3",
			Title:      "ls -l",
			Kind:       acp.KindExecute,
			Status:     acp.StatusInProgress,
			Locations:  []acp.ToolCallLocation{{Path: "/tmp/x"}},
		},
	})
	for _, key := range []string{"sessionUpdate", "toolCallId", "title", "kind", "status", "locations"} {
		if _, ok := got[key]; !ok {
			t.Errorf("no %q at the top level: %v", key, got)
		}
	}
	if got["sessionUpdate"] != acp.UpdateToolCall {
		t.Errorf("sessionUpdate = %v, want %q", got["sessionUpdate"], acp.UpdateToolCall)
	}
	if _, nested := got["ToolCall"]; nested {
		t.Error("the tool call was nested rather than inlined")
	}
}

// The same tool call, used in a permission request rather than as an update,
// must not carry a sessionUpdate: it is a tool call there, not a notification
// about one. That is the whole reason the discriminator lives on the wrapper.
func TestAPermissionRequestCarriesNoSessionUpdateField(t *testing.T) {
	t.Parallel()
	got := encode(t, acp.RequestPermissionRequest{
		SessionID: "s1",
		ToolCall:  acp.ToolCall{ToolCallID: "call-3", Title: "rm -rf /"},
		Options:   acp.PermissionOptions(),
	})
	call, ok := got["toolCall"].(map[string]any)
	if !ok {
		t.Fatalf("no toolCall object: %v", got)
	}
	if _, stray := call["sessionUpdate"]; stray {
		t.Error("the tool call carried a sessionUpdate field")
	}
	if call["toolCallId"] != "call-3" {
		t.Errorf("toolCallId = %v", call["toolCallId"])
	}
	if len(got["options"].([]any)) != 4 {
		t.Errorf("options = %v, want all four kinds", got["options"])
	}
}

// Every escalated action offers all four kinds with distinct ids, so that a
// person can settle a repeated question once.
func TestPermissionOptionsOfferAllFourKinds(t *testing.T) {
	t.Parallel()
	want := map[string]string{
		acp.OptionAllowOnce:    acp.KindAllowOnce,
		acp.OptionAllowAlways:  acp.KindAllowAlways,
		acp.OptionRejectOnce:   acp.KindRejectOnce,
		acp.OptionRejectAlways: acp.KindRejectAlway,
	}
	got := map[string]string{}
	for _, o := range acp.PermissionOptions() {
		if o.Name == "" {
			t.Errorf("option %q has no name to show a person", o.OptionID)
		}
		got[o.OptionID] = o.Kind
	}
	if len(got) != len(want) {
		t.Fatalf("options = %v, want %v", got, want)
	}
	for id, kind := range want {
		if got[id] != kind {
			t.Errorf("option %q has kind %q, want %q", id, got[id], kind)
		}
	}
}

// Which of the shell's streams a chunk came from is carried in _meta, because
// v1 has no update kind that means "diagnostics" and agent_thought_chunk means
// something else entirely.
func TestChunkNamesItsStreamInMeta(t *testing.T) {
	t.Parallel()
	got := encode(t, acp.Chunk("oops\n", acp.StreamStderr))
	if got["sessionUpdate"] != acp.UpdateAgentMessageChunk {
		t.Errorf("sessionUpdate = %v", got["sessionUpdate"])
	}
	content, ok := got["content"].(map[string]any)
	if !ok {
		t.Fatalf("no content object: %v", got)
	}
	if content["type"] != acp.ContentText || content["text"] != "oops\n" {
		t.Errorf("content = %v", content)
	}
	meta, ok := got["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("no _meta object: %v", got)
	}
	if meta[acp.MetaStream] != acp.StreamStderr {
		t.Errorf("_meta stream = %v, want %q", meta[acp.MetaStream], acp.StreamStderr)
	}
}

// A text block carries nothing it does not have. The union is wider than the
// struct, and an empty uri on a text block would claim a variant this agent
// does not advertise.
func TestATextBlockCarriesOnlyText(t *testing.T) {
	t.Parallel()
	got := encode(t, acp.TextBlock("hi"))
	if len(got) != 2 || got["type"] != acp.ContentText || got["text"] != "hi" {
		t.Errorf("block = %v, want just a type and a text", got)
	}
}

// Both outcome variants read into one struct, and an outcome that is neither
// leaves the option id empty — which the gate treats as a refusal, because an
// answer that names no option is not an answer.
func TestPermissionOutcomeReadsBothVariants(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body, outcome, option string
	}{
		{"selected", `{"outcome":"selected","optionId":"allow-once"}`, acp.OutcomeSelected, acp.OptionAllowOnce},
		//nolint:misspell // the protocol's own spelling of the outcome
		{"the turn ended first", `{"outcome":"cancelled"}`, acp.OutcomeCancelled, ""},
		{"unknown", `{"outcome":"something-new"}`, "something-new", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got acp.PermissionOutcome
			if err := json.Unmarshal([]byte(tc.body), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.Outcome != tc.outcome || got.OptionID != tc.option {
				t.Errorf("outcome = %+v, want %q/%q", got, tc.outcome, tc.option)
			}
		})
	}
}

// The prompt is read the way a client sends it: a session id and an array of
// content blocks whose text is the program.
func TestPromptRequestReadsTheSchemaShape(t *testing.T) {
	t.Parallel()
	const body = `{"sessionId":"sess_abc","prompt":[` +
		`{"type":"text","text":"echo hi"},` +
		`{"type":"resource_link","uri":"file:///tmp/x","name":"x"}]}`
	var got acp.PromptRequest
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SessionID != "sess_abc" {
		t.Errorf("sessionId = %q", got.SessionID)
	}
	if len(got.Prompt) != 2 {
		t.Fatalf("prompt = %v, want both blocks", got.Prompt)
	}
	if got.Prompt[0].Type != acp.ContentText || got.Prompt[0].Text != "echo hi" {
		t.Errorf("first block = %+v", got.Prompt[0])
	}
	if got.Prompt[1].Type != acp.ContentResourceLink || got.Prompt[1].URI != "file:///tmp/x" {
		t.Errorf("second block = %+v", got.Prompt[1])
	}
}

// A new-session request names a directory, and the MCP servers it must carry
// are kept unread rather than decoded into a shape this agent would not act
// on.
func TestNewSessionRequestKeepsMCPServersUnread(t *testing.T) {
	t.Parallel()
	const body = `{"cwd":"/work","mcpServers":[{"name":"x","command":"y","args":[]}]}`
	var got acp.NewSessionRequest
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Cwd != "/work" {
		t.Errorf("cwd = %q", got.Cwd)
	}
	if len(got.MCPServers) != 1 {
		t.Errorf("mcpServers = %v, want the one it was sent", got.MCPServers)
	}
}

// The version this agent speaks is the one the protocol calls 1, and it is
// bumped only for breaking changes.
func TestVersionIsOne(t *testing.T) {
	t.Parallel()
	if acp.Version != 1 {
		t.Errorf("Version = %d, want 1", acp.Version)
	}
}
