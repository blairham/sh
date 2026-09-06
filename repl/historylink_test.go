// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/policy"
)

// HISTFILE is the front end's most exposed open, and the reason #942 is a hole
// rather than hygiene.
//
// Everything else internal/boundary opens is chosen from the invocation or by
// the front end. HISTFILE is a shell *variable*: a line typed at the prompt or
// read from a startup file sets it, so a session can point the shell's own
// history at any path it likes — and, until this, at any path a symbolic link
// it makes reaches. The gate was asked about the name and the read and the
// append were done on the object.
//
// Both fixtures are built under t.TempDir(). Nothing here goes near a real
// home, and no link is ever made pointing into one; a history file is the
// thing this repository has already destroyed twice by being careless with it.

// deniedUnder parses a policy that refuses reads and writes beneath one
// directory. Parsed rather than hand-written, because a selector that does not
// mean what it looks like matches nothing and reads exactly like a rule being
// obeyed.
func deniedUnder(t *testing.T, dir string) *policy.Policy {
	t.Helper()
	p, err := policy.Parse(strings.NewReader(fmt.Sprintf(
		"version 1\ndefault allow\ndeny read %s/**\ndeny write %s/**\n", dir, dir)))
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	return p
}

// TestAHistoryFileThatIsALinkIsCheckedOnWhatItReached.
//
// The session is told to keep its history in a path the policy says nothing
// about, and that path is a link into a directory the policy hides. Neither
// half may happen: the earlier contents must not come back as this session's
// recall, and what was typed must not be appended to the file the policy
// refused.
func TestAHistoryFileThatIsALinkIsCheckedOnWhatItReached(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no way to ask the kernel what an open reached here")
	}
	dir := t.TempDir()
	hidden := filepath.Join(dir, "hidden")
	if err := os.Mkdir(hidden, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(hidden, "real_history")
	if err := os.WriteFile(target, []byte("echo earlier\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "innocent_history")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	h := historyFile{
		path: link, size: 10, file: 10,
		bound: boundary.Boundary{Gate: deniedUnder(t, hidden)},
	}

	if got := h.load(t.Context()); got != nil {
		t.Errorf("the session recalled %q through a link into a denied place", got)
	}
	if err := h.save(t.Context(), []string{"echo typed"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "echo earlier\n" {
		t.Errorf("the denied file now holds %q, want it unwritten", body)
	}
}

// TestAHistoryFileThroughAnAllowedLinkStillWorks, so the check above is a
// refusal of what the policy refuses rather than of links.
func TestAHistoryFileThroughAnAllowedLinkStillWorks(t *testing.T) {
	dir := t.TempDir()
	elsewhere := filepath.Join(dir, "elsewhere")
	if err := os.Mkdir(elsewhere, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(elsewhere, "real_history")
	if err := os.WriteFile(target, []byte("echo earlier\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "innocent_history")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	h := historyFile{
		path: link, size: 10, file: 10,
		// Denies a sibling that nothing here touches, so the policy is a real
		// one with a rule in it rather than a bare `default allow`.
		bound: boundary.Boundary{Gate: deniedUnder(t, filepath.Join(dir, "nowhere"))},
	}
	if got := h.load(t.Context()); len(got) != 1 || got[0] != "echo earlier" {
		t.Fatalf("loaded %q, want the earlier line", got)
	}
	if err := h.save(t.Context(), []string{"echo typed"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "echo earlier\necho typed\n" {
		t.Errorf("history = %q, want the new line appended through the link", body)
	}
}
