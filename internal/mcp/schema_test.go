// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package mcp_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/blairham/sh/internal/mcp"
)

// Every modern result, validated against the revision's own published schema
// rather than against this package's types.
//
// The other tests in this package decode a result into mcp's own structs, or
// read fields out of it by name, and both sides of that agree by construction:
// a field the types never had is a field no test looks for. That is how #6934
// shipped. `2026-07-28` makes every list result a CacheableResult with
// `ttlMs` and `cacheScope` required, this server sent neither, its tests
// passed, and Claude Code 2.1.292 rejected the tool list outright, so a model
// connected to this shell had no tools at all.
//
// So the reference here is not ours. testdata/schema-2026-07-28.json is
// `schema/2026-07-28/schema.json` from github.com/modelcontextprotocol/
// modelcontextprotocol at 271ecc9accafdd9b83a3c869fa67c22953b2af80 (blob
// 213c58f6), unchanged except for the trailing blank line the
// end-of-file hook removes. It is the same argument internal/acpcheck makes
// for ACP: a protocol tested only against itself is not tested.
//
// A revision bump replaces that file and Version together; a schema that no
// longer names a definition this table asks for fails here rather than
// passing over nothing.
func TestEveryModernResultValidatesAgainstThePublishedSchema(t *testing.T) {
	t.Parallel()
	defs := publishedSchema(t)
	p, _ := connect(t, nil)
	modern := map[string]any{"_meta": map[string]any{mcp.MetaProtocolVersion: mcp.Version}}
	withMeta := func(params map[string]any) map[string]any {
		params["_meta"] = modern["_meta"]
		return params
	}

	created := p.rawBytes(t, mcp.MethodToolsCall, withMeta(map[string]any{
		"name": mcp.ToolCreate, "arguments": map[string]any{"command": "echo schema"},
	}))
	var call struct {
		StructuredContent mcp.CreateResult `json:"structuredContent"`
	}
	if err := json.Unmarshal(created, &call); err != nil || call.StructuredContent.TerminalID == "" {
		t.Fatalf("terminal_create answered %s (%v); the rows below need a terminal", created, err)
	}
	id := call.StructuredContent.TerminalID

	for _, row := range []struct {
		name, def string
		result    json.RawMessage
	}{
		{"server/discover", "DiscoverResult", p.rawBytes(t, mcp.MethodDiscover, modern)},
		{"tools/list", "ListToolsResult", p.rawBytes(t, mcp.MethodToolsList, modern)},
		{"tools/call (create)", "CallToolResult", created},
		{"tools/call (wait)", "CallToolResult", p.rawBytes(t, mcp.MethodToolsCall, withMeta(map[string]any{
			"name": mcp.ToolWait, "arguments": map[string]any{"terminalId": id},
		}))},
		{"tools/call (output)", "CallToolResult", p.rawBytes(t, mcp.MethodToolsCall, withMeta(map[string]any{
			"name": mcp.ToolOutput, "arguments": map[string]any{"terminalId": id},
		}))},
		{"tools/call (release)", "CallToolResult", p.rawBytes(t, mcp.MethodToolsCall, withMeta(map[string]any{
			"name": mcp.ToolRelease, "arguments": map[string]any{"terminalId": id},
		}))},
		// A tool execution error is a result too, and the one most easily
		// built by hand somewhere else.
		{"tools/call (isError)", "CallToolResult", p.rawBytes(t, mcp.MethodToolsCall, withMeta(map[string]any{
			"name": mcp.ToolKill, "arguments": map[string]any{"terminalId": "term-nope"},
		}))},
	} {
		if err := defs.validate(row.def, row.result); err != nil {
			t.Errorf("%s does not validate as the revision's %s:\n%v\n\nresult: %s",
				row.name, row.def, err, row.result)
		}
	}
}

// The instrument above, shown to fire: a result missing what the revision
// requires is refused, and one with a cache scope the revision does not name
// is refused. Without this a validator that compiled nothing, or a definition
// lookup that matched a permissive schema, would read as every row passing.
func TestThePublishedSchemaRefusesAResultWithoutTheCachingHint(t *testing.T) {
	t.Parallel()
	defs := publishedSchema(t)
	for _, bad := range []string{
		`{"resultType":"complete","tools":[]}`,
		`{"resultType":"complete","ttlMs":0,"tools":[]}`,
		`{"resultType":"complete","ttlMs":1,"cacheScope":"shared","tools":[]}`,
		`{"resultType":"complete","ttlMs":-1,"cacheScope":"public","tools":[]}`,
	} {
		if err := defs.validate("ListToolsResult", json.RawMessage(bad)); err == nil {
			t.Errorf("the published ListToolsResult accepted %s; the instrument cannot see the defect it exists for", bad)
		}
	}
	good := `{"resultType":"complete","ttlMs":0,"cacheScope":"private","tools":[]}`
	if err := defs.validate("ListToolsResult", json.RawMessage(good)); err != nil {
		t.Errorf("the published ListToolsResult refused %s: %v", good, err)
	}
}

// The legacy era is answered without the caching hint, because `2025-11-25`
// has no such fields, and the hint is per request like the rest of the era.
func TestOnlyAModernListCarriesTheCachingHint(t *testing.T) {
	t.Parallel()
	p, _ := connect(t, nil)
	modern := p.raw(t, mcp.MethodToolsList, map[string]any{
		"_meta": map[string]any{mcp.MetaProtocolVersion: mcp.Version},
	})
	if modern["cacheScope"] != mcp.CacheScopePublic {
		t.Errorf("a modern tools/list answered cacheScope %v, want %q", modern["cacheScope"], mcp.CacheScopePublic)
	}
	if ttl, _ := modern["ttlMs"].(float64); ttl <= 0 {
		t.Errorf(
			"a modern tools/list answered ttlMs %v, want a positive lifetime: the list is fixed by the binary",
			modern["ttlMs"],
		)
	}
	legacy := p.raw(t, mcp.MethodToolsList, map[string]any{})
	for _, field := range []string{"ttlMs", "cacheScope"} {
		if v, ok := legacy[field]; ok {
			t.Errorf("a legacy tools/list answered %s %v; that field is the modern era's", field, v)
		}
	}
}

// rawBytes is one call whose result is kept as the bytes a client received.
// Not decoded through encoding/json's map, which would turn every number into
// a float64 and lose the integer-ness the schema asks about.
func (p *peer) rawBytes(t *testing.T, method string, params any) json.RawMessage {
	t.Helper()
	var out json.RawMessage
	if err := p.conn.Call(t.Context(), method, params, &out); err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	return out
}

// schemaDefs is the published schema, compiled once per test.
type schemaDefs struct {
	c *jsonschema.Compiler
}

const schemaURL = "https://modelcontextprotocol.io/schema/2026-07-28/schema.json"

func publishedSchema(t *testing.T) schemaDefs {
	t.Helper()
	raw, err := os.ReadFile("testdata/schema-" + mcp.Version + ".json")
	if err != nil {
		t.Fatalf("the published schema for %s is not in testdata: %v", mcp.Version, err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(schemaURL, doc); err != nil {
		t.Fatal(err)
	}
	return schemaDefs{c: c}
}

func (d schemaDefs) validate(def string, result json.RawMessage) error {
	sch, err := d.c.Compile(schemaURL + "#/$defs/" + def)
	if err != nil {
		return err
	}
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(string(result)))
	if err != nil {
		return err
	}
	return sch.Validate(inst)
}
