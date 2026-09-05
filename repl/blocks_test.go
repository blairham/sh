// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/blocks"
	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/syntax"
)

// blockShell is a session that reads its lines from a pipe and records its
// blocks into a directory the framework takes away again.
//
// Nothing here touches the machine's own home or history: HISTFILE is set to a
// path under the temporary directory, SH_BLOCKS_DIR to another, and the Runner
// is given an explicit Dir. A test that read the real store would be a
// different answer on every machine and would be the user's own file besides.
func blockShell(t *testing.T, script string, vars map[string]string) (Shell, string, func()) {
	t.Helper()
	dir := t.TempDir()
	store := filepath.Join(dir, "blocks")
	if vars == nil {
		vars = map[string]string{}
	}
	if _, ok := vars["HISTFILE"]; !ok {
		vars["HISTFILE"] = filepath.Join(dir, "hist")
	}
	if _, ok := vars[blocks.DirVar]; !ok {
		vars[blocks.DirVar] = store
	}
	r := newTestRunner(vars)
	r.Dir = dir

	in := filepath.Join(dir, "script")
	if err := os.WriteFile(in, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	// A fixed clock, so a duration is a number a test can assert on rather
	// than whatever the machine happened to take.
	tick := time.Unix(1_757_000_000, 0).UTC()
	sh := Shell{
		Runner: r, Dialect: syntax.Core(), In: f,
		Out: &strings.Builder{}, Err: &strings.Builder{},
		Clock: func() time.Time {
			tick = tick.Add(250 * time.Millisecond)
			return tick
		},
	}
	return sh, store, func() { _ = f.Close() }
}

// read is what the store holds afterwards, read back through a store of its
// own so the test goes the same way a later session would.
func read(t *testing.T, dir string) []blocks.Record {
	t.Helper()
	return blocks.Open(dir, boundary.Boundary{}, "READER").Load(t.Context(), 100)
}

// A session records what it ran: the line as typed, where, when, for how long
// and what it exited with.
func TestASessionRecordsItsBlocks(t *testing.T) {
	sh, store, done := blockShell(t, "true\nfalse\n", nil)
	defer done()
	if _, err := sh.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := read(t, store)
	if len(got) != 2 {
		t.Fatalf("recorded %d blocks, want 2: %+v", len(got), got)
	}
	if got[0].Command != "true" || got[1].Command != "false" {
		t.Errorf("commands are %q and %q", got[0].Command, got[1].Command)
	}
	if got[0].Status != 0 || got[1].Status != 1 {
		t.Errorf("statuses are %d and %d, want 0 and 1", got[0].Status, got[1].Status)
	}
	if got[0].Cwd != sh.Runner.Dir {
		t.Errorf("cwd is %q, want the Runner's %q", got[0].Cwd, sh.Runner.Dir)
	}
	if got[0].DurationMs != 250 {
		t.Errorf("duration is %d, want the 250ms the clock advanced", got[0].DurationMs)
	}
	if got[0].Session == "" || got[0].Session != got[1].Session {
		t.Errorf("sessions are %q and %q, want one non-empty id for the session",
			got[0].Session, got[1].Session)
	}
	if got[0].ID == got[1].ID {
		t.Errorf("both blocks have id %q", got[0].ID)
	}
}

// The line is recorded as typed, before expansion. That is the field no event
// can supply — an event carries argv after expansion — and it is why a block
// is the repl's to record.
func TestABlockKeepsTheLineAsTyped(t *testing.T) {
	sh, store, done := blockShell(t, "echo $HOME\n", map[string]string{"HOME": "/somewhere"})
	defer done()
	if _, err := sh.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := read(t, store)
	if len(got) != 1 || got[0].Command != "echo $HOME" {
		t.Fatalf("recorded %+v, want the line before expansion", got)
	}
}

// A construct typed over several lines is one block, exactly as it is one
// history entry and one command.
func TestAMultiLineConstructIsOneBlock(t *testing.T) {
	sh, store, done := blockShell(t, "for i in 1 2\ndo\n:\ndone\necho after\n", nil)
	defer done()
	if _, err := sh.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := read(t, store)
	if len(got) != 2 {
		t.Fatalf("recorded %d blocks, want 2: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Command, "for i in 1 2") ||
		!strings.Contains(got[0].Command, "done") {
		t.Errorf("the first block is %q, want the whole construct", got[0].Command)
	}
	if got[1].Command != "echo after" {
		t.Errorf("the second block is %q", got[1].Command)
	}
}

// A blank line is not a block. Recording one would fill the store with records
// of somebody pressing return.
func TestABlankLineIsNotABlock(t *testing.T) {
	sh, store, done := blockShell(t, "\n   \n:\n", nil)
	defer done()
	if _, err := sh.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := read(t, store); len(got) != 1 || got[0].Command != ":" {
		t.Fatalf("recorded %+v, want the one command that ran", got)
	}
}

// A line that would not parse never became a command, so it is not a block.
func TestALineThatWouldNotParseIsNotABlock(t *testing.T) {
	sh, store, done := blockShell(t, "for\n:\n", nil)
	defer done()
	if _, err := sh.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := read(t, store); len(got) != 1 || got[0].Command != ":" {
		t.Fatalf("recorded %+v, want only the command that ran", got)
	}
}

// An empty HISTFILE means *do not remember this session*, and that has to
// govern both files. A shell that honored it for the line file and went on
// writing a richer record would be doing the opposite of what was asked.
func TestAnEmptyHISTFILETurnsBlocksOffToo(t *testing.T) {
	dir := t.TempDir()
	store := filepath.Join(dir, "blocks")
	sh, _, done := blockShell(t, ":\n", map[string]string{
		"HISTFILE": "", blocks.DirVar: store,
	})
	defer done()
	if _, err := sh.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store); !os.IsNotExist(err) {
		t.Errorf("the store exists at %q with HISTFILE empty", store)
	}
}

// And the store's own off switch is the same idiom, without taking the
// ordinary history with it.
func TestAnEmptyBlocksDirTurnsOnlyBlocksOff(t *testing.T) {
	dir := t.TempDir()
	hist := filepath.Join(dir, "hist")
	sh, _, done := blockShell(t, ":\n", map[string]string{
		"HISTFILE": hist, blocks.DirVar: "",
	})
	defer done()
	if _, err := sh.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if sh.blocksDir() != "" {
		t.Errorf("blocksDir is %q with the variable empty", sh.blocksDir())
	}
}

// Where the store goes when nothing named it: a state directory rather than a
// dotfile in the home, because the bodies make it a directory of arbitrary
// size.
func TestWhereTheStoreLives(t *testing.T) {
	sh := Shell{Runner: newTestRunner(map[string]string{
		"HISTFILE": "/tmp/hist", "HOME": "/home/someone",
	})}
	want := filepath.Join("/home/someone", ".local", "state", "sh", "blocks")
	if got := sh.blocksDir(); got != want {
		t.Errorf("default store is %q, want %q", got, want)
	}
	sh = Shell{Runner: newTestRunner(map[string]string{
		"HISTFILE": "/tmp/hist", "HOME": "/home/someone", "XDG_STATE_HOME": "/state",
	})}
	if got := sh.blocksDir(); got != filepath.Join("/state", "sh", "blocks") {
		t.Errorf("with XDG_STATE_HOME the store is %q", got)
	}
	// With no home there is nowhere to put it, which is the same answer the
	// history file gives.
	sh = Shell{Runner: newTestRunner(map[string]string{"HISTFILE": "/tmp/hist"})}
	if got := sh.blocksDir(); got != "" {
		t.Errorf("with no home the store is %q, want none", got)
	}
}

// The two files are independent: the line file keeps its format and the index
// is a second file beside it, never a replacement.
func TestTheLineFileIsUntouched(t *testing.T) {
	sh, store, done := blockShell(t, ":\n", nil)
	defer done()
	hist, _ := sh.Runner.GetVar("HISTFILE")
	if err := os.WriteFile(hist, []byte("earlier one\nearlier two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := sh.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(hist)
	if err != nil {
		t.Fatal(err)
	}
	// Still plain lines, and still the ones that were there. This session has
	// no editor — it is reading a file — so it adds nothing, which is exactly
	// the property being checked: the block store did not write here.
	if string(b) != "earlier one\nearlier two\n" {
		t.Errorf("the line file is now %q, want it untouched and still plain lines", b)
	}
	if got := read(t, store); len(got) != 1 {
		t.Errorf("the index holds %d records, want the one block", len(got))
	}
}

// With capture on, a block keeps what it printed — and the person still sees
// it, because the copy is the addition rather than a diversion.
func TestABlockKeepsWhatItPrinted(t *testing.T) {
	sh, store, done := blockShell(t, "echo one\necho two\n", map[string]string{
		blocks.OutputVar: "1",
	})
	defer done()
	var out strings.Builder
	sh.Runner.Stdout = &out
	if _, err := sh.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if out.String() != "one\ntwo\n" {
		t.Errorf("the session printed %q, want both lines on the stream", out.String())
	}
	got := read(t, store)
	if len(got) != 2 {
		t.Fatalf("recorded %d blocks", len(got))
	}
	reader := blocks.Open(store, boundary.Boundary{}, "READER")
	for i, want := range []string{"one\n", "two\n"} {
		if got[i].Output == "" {
			t.Fatalf("block %d kept no output", i)
		}
		if got[i].OutputBytes != int64(len(want)) || got[i].Truncated {
			t.Errorf("block %d reports %d bytes, truncated %v",
				i, got[i].OutputBytes, got[i].Truncated)
		}
		if got[i].Streams != blocks.StreamsMerged {
			t.Errorf("block %d says streams %q", i, got[i].Streams)
		}
		body, ok := reader.Body(t.Context(), got[i])
		if !ok || body != want {
			t.Errorf("block %d body is %q, %v — want %q", i, body, ok, want)
		}
	}
}

// One block's output does not leak into the next.
func TestOutputDoesNotLeakBetweenBlocks(t *testing.T) {
	sh, store, done := blockShell(t, "echo one\n:\n", map[string]string{
		blocks.OutputVar: "1",
	})
	defer done()
	sh.Runner.Stdout = &strings.Builder{}
	if _, err := sh.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := read(t, store)
	if len(got) != 2 {
		t.Fatalf("recorded %d blocks", len(got))
	}
	if got[1].Output != "" || got[1].OutputBytes != 0 {
		t.Errorf("the silent block kept %q, %d bytes", got[1].Output, got[1].OutputBytes)
	}
}

// Both of a command's streams reach one body, in write order. Two files would
// lose the interleaving, which is often the information.
func TestBothStreamsReachOneBody(t *testing.T) {
	sh, store, done := blockShell(t, "{ echo out; echo err >&2; }\n", map[string]string{
		blocks.OutputVar: "1",
	})
	defer done()
	sh.Runner.Stdout, sh.Runner.Stderr = &strings.Builder{}, &strings.Builder{}
	if _, err := sh.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := read(t, store)
	if len(got) != 1 {
		t.Fatalf("recorded %d blocks", len(got))
	}
	body, ok := blocks.Open(store, boundary.Boundary{}, "READER").Body(t.Context(), got[0])
	if !ok || body != "out\nerr\n" {
		t.Errorf("body is %q, %v — want both streams in write order", body, ok)
	}
}

// Off by default, because a captured stream is not a terminal to the child on
// the other end of it. The command half still runs.
func TestOutputIsNotKeptUnlessAsked(t *testing.T) {
	sh, store, done := blockShell(t, "echo one\n", nil)
	defer done()
	if sh.captureOutput() != nil {
		t.Fatal("a session with nothing set is capturing")
	}
	if _, err := sh.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := read(t, store)
	if len(got) != 1 {
		t.Fatalf("recorded %d blocks", len(got))
	}
	if got[0].Output != "" || got[0].OutputBytes != 0 || got[0].Streams != "" {
		t.Errorf("a session that was not asked kept %+v", got[0])
	}
}

// With capture off the Runner keeps the streams it was given, which is the
// whole reason it is off: an *os.File is inherited by a child as a descriptor
// and anything else makes os/exec build a pipe.
func TestWithoutCaptureTheStreamsAreLeftAlone(t *testing.T) {
	sh, _, done := blockShell(t, ":\n", nil)
	defer done()
	var out strings.Builder
	sh.Runner.Stdout = &out
	if c := sh.captureOutput(); c != nil {
		t.Fatal("a capture was installed without being asked for")
	}
	if sh.Runner.Stdout != io.Writer(&out) {
		t.Error("the Runner's stream was replaced anyway")
	}
}
