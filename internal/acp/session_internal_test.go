// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp

import (
	"encoding/json"
	"io"
	"os"
	"testing"
	"unicode/utf8"

	"github.com/blairham/sh/driver"
)

// Two properties of a session that a client cannot see from outside, and that
// are catastrophic rather than wrong when they are missing. Both were left to
// a comment first and neither was graded: a mutation that removed them changed
// nothing any end-to-end test could reach, because the damage they prevent
// needs a real process to happen in.

func newTestSession(t *testing.T) *session {
	t.Helper()
	a := NewAgent(driver.Shell{Name: "sh"}, Implementation{Name: "sh", Version: "test"})
	params, err := json.Marshal(NewSessionRequest{Cwd: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	made, err := a.newSession(params)
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	id := made.(NewSessionResponse).SessionID
	s := a.session(id)
	if s == nil {
		t.Fatal("the session was not kept")
	}
	return s
}

// A session's standard input must not be the process's.
//
// The front end fills a nil standard input in with `os.Stdin`, which for this
// binary is the protocol stream: a `read` in a script would take the client's
// next message off the wire and the connection would be desynchronized with
// nothing to say why. It is checked here rather than through a session,
// because under `go test` the process's own standard input is usually empty
// too — so a script reading it looks exactly like a script reading the null
// device, and every end-to-end test passes while the shipped binary eats its
// own protocol.
func TestASessionsStandardInputIsNotTheProcessStream(t *testing.T) {
	t.Parallel()
	s := newTestSession(t)

	in := s.shell.Runner().Stdin
	if in == nil {
		t.Fatal("standard input is nil, which the front end fills in with the process's")
	}
	if f, ok := in.(*os.File); ok && f.Fd() == os.Stdin.Fd() {
		t.Fatal("standard input is the process's, which here is the protocol stream")
	}
	b, err := io.ReadAll(in)
	if err != nil {
		t.Fatalf("reading the session's input: %v", err)
	}
	if len(b) != 0 {
		t.Errorf("the session's input held %q, want nothing to read", b)
	}
}

// A shell that is a protocol server must not stop being one.
//
// `exec cmd` replacing this process would take the connection and every other
// session on it, and an untrapped fatal signal aimed at the shell is the same
// argument. KeepProcess is what declines both, and there is no way to observe
// its absence from a test that survives it.
func TestASessionMayNotReplaceTheProcess(t *testing.T) {
	t.Parallel()
	a := NewAgent(driver.Shell{Name: "sh"}, Implementation{Name: "sh", Version: "test"})
	if !a.Shell.KeepProcess {
		t.Error("a session would replace the process on `exec`")
	}
	s := newTestSession(t)
	if s.shell.Runner().ReplaceProcess != nil {
		t.Error("the runner was given a way to replace the process")
	}
	if s.shell.Runner().DieBySignal != nil {
		t.Error("the runner was given a way to end the process by signal")
	}
}

// Output is chunked by character, not by write.
//
// A Writer takes whatever size of write a command made, and a multi-byte
// character can straddle two of them. Sending the halves puts invalid UTF-8
// into a JSON string, where the encoder replaces it — so the text arrives as
// mojibake rather than as what was written. It is unit-tested here because a
// shell usually writes a line at a time and the split does not happen on
// demand: an end-to-end test of `printf 'café'` passes whether or not this
// works.
func TestOutputIsChunkedByCharacterRatherThanByWrite(t *testing.T) {
	t.Parallel()
	var pieces []string
	c := &chunker{emit: func(s string) { pieces = append(pieces, s) }}

	// "café ☕" split in the middle of both multi-byte characters.
	full := []byte("café ☕")
	for i := range full {
		if _, err := c.Write(full[i : i+1]); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	c.Flush()

	var joined string
	for _, p := range pieces {
		if !utf8.ValidString(p) {
			t.Errorf("a piece was not valid UTF-8: %q", p)
		}
		joined += p
	}
	if joined != string(full) {
		t.Errorf("joined = %q, want %q", joined, string(full))
	}
}

// What is held back at the end of a turn is a truncated character rather than
// an incomplete one, and is better shown than dropped.
func TestFlushSendsAnUnfinishedCharacter(t *testing.T) {
	t.Parallel()
	var pieces []string
	c := &chunker{emit: func(s string) { pieces = append(pieces, s) }}

	if _, err := c.Write([]byte{'a', 0xE2, 0x98}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if len(pieces) != 1 || pieces[0] != "a" {
		t.Fatalf("pieces = %q, want the complete part only", pieces)
	}
	c.Flush()
	if len(pieces) != 2 {
		t.Errorf("pieces = %q, want the held-back bytes to have been sent", pieces)
	}
}
