// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/acp"
	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/policy"
	"github.com/blairham/sh/interp"
)

// The ACP pair is the front end's most interesting open, because the path is
// chosen live by a party outside this process.
//
// An agent that can write a file can make a symbolic link — `fs/write_text_file`
// cannot, but a command it runs through `terminal/create` can, and that command
// passed the exec gate rather than a rule about the link it then made. So an
// agent asked to read `work/notes` where `work/notes` points into a directory
// the policy hides used to get the contents: the gate was consulted about the
// name it sent and the read was performed on the object.
//
// Both fixtures are built under t.TempDir(), and no link is ever made pointing
// into a real home.

func linkFixture(t *testing.T, body string) (hidden, link, object string) {
	t.Helper()
	dir := t.TempDir()
	hidden = filepath.Join(dir, "hidden")
	if err := os.Mkdir(hidden, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(hidden, "classified")
	if err := os.WriteFile(target, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	link = filepath.Join(dir, "innocent")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	object, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	return hidden, link, object
}

// gateOf is a recorder driven by a parsed policy, so the rule under test is
// the one the policy language actually means.
func gateOf(t *testing.T, rules string) *recorder {
	t.Helper()
	p, err := policy.Parse(strings.NewReader("version 1\ndefault allow\n" + rules))
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	return &recorder{deny: func(a interp.Action) bool {
		return p.Allow(t.Context(), a) == interp.Deny
	}}
}

// TestAFileAnAgentNamesThroughALinkIsCheckedOnWhatItReached.
func TestAFileAnAgentNamesThroughALinkIsCheckedOnWhatItReached(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no way to ask the kernel what an open reached here")
	}
	hidden, link, object := linkFixture(t, "classified\n")
	seen := gateOf(t, fmt.Sprintf("deny read %s/**\n", hidden))
	c := &acp.Client{
		Info:     acp.Implementation{Name: "test-client", Version: "1"},
		Files:    true,
		Boundary: boundary.Boundary{Gate: seen, Events: seen},
	}
	fake := &agentSide{}
	against(t, c, fake)

	var resp acp.ReadTextFileResponse
	err := fake.conn.Call(t.Context(), acp.MethodReadTextFile,
		acp.ReadTextFileRequest{SessionID: "s1", Path: link}, &resp)
	if err == nil {
		t.Fatalf("the read through the link succeeded and returned %q", resp.Content)
	}
	if strings.Contains(err.Error(), "classified") {
		t.Errorf("the refusal leaked the content: %v", err)
	}
	// The agent is told what it is told for a path denied by name: that it is
	// not there. Saying where the link went would let an agent map a hidden
	// directory one link at a time by asking to be refused.
	if !strings.Contains(err.Error(), "no such file or directory") {
		t.Errorf("the refusal reads %q, want the ordinary missing-path answer", err)
	}
	if strings.Contains(err.Error(), object) {
		t.Errorf("the refusal names where the link went: %v", err)
	}
}

// TestAWriteAnAgentNamesThroughALinkDoesNotEmptyTheFileFirst.
//
// The protocol's write replaces a file's contents, so it is os.WriteFile's
// O_TRUNC — the flag that destroys as part of the open. An agent that pointed
// a link at a file the policy protects and asked to write it would, before
// #942, have emptied that file and then been told it could not write to it.
func TestAWriteAnAgentNamesThroughALinkDoesNotEmptyTheFileFirst(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no way to ask the kernel what an open reached here")
	}
	hidden, link, object := linkFixture(t, "KEEP THIS\n")
	seen := gateOf(t, fmt.Sprintf("deny write %s/**\n", hidden))
	c := &acp.Client{
		Info:     acp.Implementation{Name: "test-client", Version: "1"},
		Files:    true,
		Boundary: boundary.Boundary{Gate: seen, Events: seen},
	}
	fake := &agentSide{}
	against(t, c, fake)

	if err := fake.conn.Call(t.Context(), acp.MethodWriteTextFile,
		acp.WriteTextFileRequest{SessionID: "s1", Path: link, Content: "clobbered"}, nil); err == nil {
		t.Fatal("the write through the link was allowed")
	}
	body, err := os.ReadFile(object)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "KEEP THIS\n" {
		t.Errorf("the protected file holds %q, want it untouched by a refused write", body)
	}
}
