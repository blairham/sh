// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package mcp_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/jsonrpc"
	"github.com/blairham/sh/internal/mcp"
)

// The two eras, and what each one is owed.
//
// `2026-07-28` is what this server implements: no handshake, a version in
// every request's `_meta`, `server/discover` as the probe, `resultType` on
// every result. `2025-11-25` is the handshake revision served beside it, for
// the clients that still open with one — which is the measured majority. The
// tests below hold both, because the way a dual-era server fails is by
// answering one of them in the other's shape.

// raw is one call whose result is read as the JSON a client would see, rather
// than through this package's own types. A field that is present or absent by
// era is a question about the bytes.
func (p *peer) raw(t *testing.T, method string, params any) map[string]any {
	t.Helper()
	var out map[string]any
	if err := p.conn.Call(t.Context(), method, params, &out); err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	return out
}

// The modern probe, which every server in this revision must answer — and
// which is also the stdio backward-compatibility probe, so a server that
// refused it would be telling every dual-era client that it is a legacy
// server and getting an `initialize` it need never have seen.
func TestServerDiscoverAnswersWithTheRevisionAndTheServerInfo(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, nil)
	got := p.raw(t, mcp.MethodDiscover, map[string]any{
		"_meta": map[string]any{mcp.MetaProtocolVersion: mcp.Version},
	})
	if got["resultType"] != mcp.ResultComplete {
		t.Errorf("resultType is %v, want %q", got["resultType"], mcp.ResultComplete)
	}
	versions, _ := got["supportedVersions"].([]any)
	if len(versions) != 1 || versions[0] != mcp.Version {
		t.Errorf("supportedVersions is %v, want [%s]", got["supportedVersions"], mcp.Version)
	}
	caps, _ := got["capabilities"].(map[string]any)
	if _, ok := caps["tools"]; !ok {
		t.Errorf("capabilities is %v, want the tools capability: a server that serves tools must declare them", caps)
	}
	meta, _ := got["_meta"].(map[string]any)
	info, _ := meta[mcp.MetaServerInfo].(map[string]any)
	if info["name"] != "sh" {
		t.Errorf("serverInfo is %v, want this shell named in it", meta[mcp.MetaServerInfo])
	}
}

// A modern client naming a revision this server does not speak is answered
// with the revision's own error *and the list*, because its next move is to
// pick one off the list and retry. An ordinary method-not-found would make it
// fall back to the handshake instead, which is a different conversation.
func TestAModernRequestNamingAnotherRevisionIsRefusedWithTheSupportedList(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, nil)
	err := p.conn.Call(t.Context(), mcp.MethodToolsList, map[string]any{
		"_meta": map[string]any{mcp.MetaProtocolVersion: "1900-01-01"},
	}, nil)
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) {
		t.Fatalf("a request naming an unknown revision answered %v, want an error", err)
	}
	if rpcErr.Code != mcp.CodeUnsupportedProtocolVersion {
		t.Errorf("the code is %d, want %d", rpcErr.Code, mcp.CodeUnsupportedProtocolVersion)
	}
	var data struct {
		Supported []string `json:"supported"`
		Requested string   `json:"requested"`
	}
	if err := json.Unmarshal(rpcErr.Data, &data); err != nil {
		t.Fatalf("the error carried no data a client can act on: %v", err)
	}
	if len(data.Supported) != 1 || data.Supported[0] != mcp.Version {
		t.Errorf("supported is %v, want [%s]: a client picks its retry off this", data.Supported, mcp.Version)
	}
	if data.Requested != "1900-01-01" {
		t.Errorf("requested is %q, want what the client asked for", data.Requested)
	}
}

// The handshake a real client sends, byte for byte as it was measured.
//
// Recorded 2026-09-21 against Claude Code 2.1.267 by standing a logging server
// up under `claude mcp add` and reading what arrived. It is the reason the
// legacy era is served at all: this client sends no `_meta`, never probes with
// `server/discover`, and would find a modern-only server unusable. A test
// written from the specification alone could not have found that.
func TestTheHandshakeAMeasuredClientSendsIsAnswered(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, nil)
	var params any
	measured := `{"protocolVersion":"2025-11-25",` +
		`"capabilities":{"roots":{"listChanged":true},"elicitation":{}},` +
		`"clientInfo":{"name":"claude-code","title":"Claude Code","version":"2.1.267",` +
		`"description":"Anthropic's agentic coding tool","websiteUrl":"https://claude.com/claude-code"}}`
	if err := json.Unmarshal([]byte(measured), &params); err != nil {
		t.Fatal(err)
	}
	got := p.raw(t, mcp.MethodInitialize, params)
	if got["protocolVersion"] != mcp.LegacyVersion {
		t.Errorf("the handshake answered %v, want %q", got["protocolVersion"], mcp.LegacyVersion)
	}
	caps, _ := got["capabilities"].(map[string]any)
	if _, ok := caps["tools"]; !ok {
		t.Errorf("the handshake declared %v, want the tools capability", caps)
	}
	info, _ := got["serverInfo"].(map[string]any)
	if info["name"] != "sh" {
		t.Errorf("serverInfo is %v, want this shell named in it", got["serverInfo"])
	}
	// And a command runs after it, which is the half that makes the handshake
	// worth answering: a server that shakes hands and then serves nothing is
	// the shape a legacy client cannot tell from a working one.
	if out, status := p.run(t, "echo after-the-handshake"); status != 0 ||
		!strings.Contains(out, "after-the-handshake") {
		t.Errorf("after the measured handshake a command wrote %q and exited %d", out, status)
	}
}

// A modern result carries the discriminator and a legacy one does not, and
// that is per *request* rather than per connection: the revision has no
// session, so the era is a property of what arrived.
func TestOnlyAModernResultCarriesResultType(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, nil)
	modern := p.raw(t, mcp.MethodToolsList, map[string]any{
		"_meta": map[string]any{mcp.MetaProtocolVersion: mcp.Version},
	})
	if modern["resultType"] != mcp.ResultComplete {
		t.Errorf("a modern tools/list answered resultType %v, want %q",
			modern["resultType"], mcp.ResultComplete)
	}
	legacy := p.raw(t, mcp.MethodToolsList, map[string]any{})
	if _, ok := legacy["resultType"]; ok {
		t.Errorf("a legacy tools/list answered resultType %v; that field is the modern era's and "+
			"a legacy client validating against its own schema has nowhere to put it", legacy["resultType"])
	}
	// The same connection answered both, one after the other, which is the
	// property being asserted: nothing here is remembered between requests.
	if _, ok := modern["tools"]; !ok {
		t.Error("the modern answer listed no tools")
	}
	if _, ok := legacy["tools"]; !ok {
		t.Error("the legacy answer listed no tools")
	}
}

// Every tool listed is one the dispatch serves, and the shapes clients are
// handed are the shapes they are answered in.
func TestEveryToolListedIsOneTheServerServes(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, nil)
	var listed mcp.ListToolsResult
	if err := p.conn.Call(t.Context(), mcp.MethodToolsList, map[string]any{}, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 5 {
		t.Fatalf("the server lists %d tools; the terminal model is five verbs", len(listed.Tools))
	}
	for _, tool := range listed.Tools {
		if tool.Description == "" {
			t.Errorf("%s is advertised with no description: a model chooses a tool by reading one", tool.Name)
		}
		if tool.InputSchema.Type != "object" {
			t.Errorf("%s has input schema type %q, want object", tool.Name, tool.InputSchema.Type)
		}
		// Called with arguments that cannot be right. What matters is that it
		// is *served* — an unknown tool is a JSON-RPC error and anything else
		// means the dispatch knows the name.
		err := p.conn.Call(t.Context(), mcp.MethodToolsCall, map[string]any{
			"name": tool.Name, "arguments": map[string]any{"terminalId": "term-nope"},
		}, nil)
		var rpcErr *jsonrpc.Error
		if errors.As(err, &rpcErr) && strings.Contains(rpcErr.Message, "unknown tool") {
			t.Errorf("%s is listed and the dispatch does not serve it", tool.Name)
		}
	}
}

// Ping, because a client that health-checks a server with one and gets
// "method not found" reports the server as down.
func TestPingIsAnswered(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, nil)
	if err := p.conn.Call(t.Context(), mcp.MethodPing, map[string]any{}, nil); err != nil {
		t.Errorf("ping: %v", err)
	}
}

// And a method this server does not serve is refused by name rather than
// answered emptily, which is the rule the ACP side follows: a server that
// answered a capability it never claimed would be telling the client
// something untrue about what it can rely on.
func TestAMethodThisServerDoesNotClaimIsRefused(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, nil)
	for _, method := range []string{"resources/list", "prompts/list", "logging/setLevel", "completion/complete"} {
		err := p.conn.Call(t.Context(), method, map[string]any{}, nil)
		var rpcErr *jsonrpc.Error
		if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeMethodNotFound {
			t.Errorf("%s answered %v, want method not found", method, err)
		}
	}
}
