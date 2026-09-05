// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/secret"
	"github.com/blairham/sh/interp"
)

func at(sec int64) time.Time { return time.Unix(sec, 0).UTC() }

// newStore is a store in a directory the framework takes away again.
func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s := Open(dir, boundary.Boundary{}, "SESSION")
	t.Cleanup(func() { _ = s.Close() })
	return s, dir
}

func mustAppend(t *testing.T, s *Store, r Record) {
	t.Helper()
	if err := s.Append(t.Context(), r); err != nil {
		t.Fatal(err)
	}
}

// The store fills in the version and the session, so a caller cannot get
// either wrong, and reads back what it wrote.
func TestARecordSurvivesTheStore(t *testing.T) {
	s, _ := newStore(t)
	mustAppend(t, s, Record{
		ID: "A", Command: "make check", Cwd: "/src",
		Start: at(1000), DurationMs: 8421, Status: 2,
	})
	got := s.Load(t.Context(), 10)
	if len(got) != 1 {
		t.Fatalf("loaded %d records, want 1", len(got))
	}
	r := got[0]
	if r.V != Version {
		t.Errorf("version is %d, want %d", r.V, Version)
	}
	if r.Session != "SESSION" {
		t.Errorf("session is %q, want the store's", r.Session)
	}
	if r.Command != "make check" || r.Cwd != "/src" || r.Status != 2 || r.DurationMs != 8421 {
		t.Errorf("record came back as %+v", r)
	}
	if !r.Start.Equal(at(1000)) {
		t.Errorf("start is %v, want %v", r.Start, at(1000))
	}
}

// Two sessions writing the same file both keep what they recorded. Appending
// rather than rewriting is the whole of it: a store that rewrote at exit would
// make the last shell to close the only session that happened.
func TestTwoSessionsBothKeepTheirBlocks(t *testing.T) {
	dir := t.TempDir()
	one := Open(dir, boundary.Boundary{}, "ONE")
	two := Open(dir, boundary.Boundary{}, "TWO")
	// Interleaved on purpose: the handles are open at the same time, which is
	// what two terminals are.
	mustAppend(t, one, Record{ID: "A", Command: "one-first"})
	mustAppend(t, two, Record{ID: "B", Command: "two-first"})
	mustAppend(t, one, Record{ID: "C", Command: "one-second"})
	if err := errors.Join(one.Close(), two.Close()); err != nil {
		t.Fatal(err)
	}

	got := Open(dir, boundary.Boundary{}, "READER").Load(t.Context(), 10)
	if len(got) != 3 {
		t.Fatalf("loaded %d records, want 3 — a session lost its lines", len(got))
	}
	var sessions []string
	for _, r := range got {
		sessions = append(sessions, r.Session)
	}
	if strings.Join(sessions, ",") != "ONE,TWO,ONE" {
		t.Errorf("sessions read back as %v, want them interleaved as written", sessions)
	}
}

// A reader keeps the tail, exactly as the line file does with HISTFILESIZE.
// Nothing truncates the file, because nothing may: another shell has it open.
func TestLoadKeepsTheTailAndTheFileKeepsEverything(t *testing.T) {
	s, dir := newStore(t)
	for i := range 10 {
		mustAppend(t, s, Record{ID: "X", Command: string(rune('a' + i))})
	}
	got := s.Load(t.Context(), 3)
	if len(got) != 3 || got[0].Command != "h" || got[2].Command != "j" {
		t.Fatalf("loaded %d records ending %v, want the last three", len(got), got)
	}
	b, err := os.ReadFile(filepath.Join(dir, IndexName))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "\n"); n != 10 {
		t.Errorf("the file holds %d lines, want all 10 — the reader trimmed the file", n)
	}
}

// A torn or foreign line costs one record rather than the file. That is the
// property JSON Lines was chosen for.
func TestOneUnreadableLineDoesNotLoseTheRest(t *testing.T) {
	s, dir := newStore(t)
	mustAppend(t, s, Record{ID: "A", Command: "before"})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, IndexName), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{\"v\":1,\"command\":\"tor\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	after := Open(dir, boundary.Boundary{}, "AFTER")
	t.Cleanup(func() { _ = after.Close() })
	mustAppend(t, after, Record{ID: "B", Command: "after"})

	got := after.Load(t.Context(), 10)
	if len(got) != 2 || got[0].Command != "before" || got[1].Command != "after" {
		t.Fatalf("loaded %v, want the two good records with the torn one skipped", got)
	}
}

// A command line carrying a credential is not recorded at all. The store must
// not become the copy of the history that the history file refused to keep.
func TestACommandWithACredentialIsNotRecorded(t *testing.T) {
	s, dir := newStore(t)
	mustAppend(t, s, Record{ID: "A", Command: "export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE"})
	mustAppend(t, s, Record{ID: "B", Command: "echo fine"})
	if got := s.Load(t.Context(), 10); len(got) != 1 || got[0].Command != "echo fine" {
		t.Fatalf("loaded %v, want the credential line dropped and the other kept", got)
	}
	// And it is not in the bytes either, which is the property that matters:
	// the file never held it, rather than a reader declining to show it.
	b, err := os.ReadFile(filepath.Join(dir, IndexName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "AKIAIOSFODNN7EXAMPLE") {
		t.Error("the credential reached the file")
	}
}

// Output is redacted rather than dropped: losing a build log because one line
// of it echoed a token destroys what the person wanted to keep.
func TestABodyIsRedactedAndKept(t *testing.T) {
	s, _ := newStore(t)
	text := "building\nAKIAIOSFODNN7EXAMPLE\ndone\n"
	rel, err := s.WriteBody(t.Context(), "BLOCK", at(1000), text)
	if err != nil {
		t.Fatal(err)
	}
	body, ok := s.Body(t.Context(), Record{Output: rel})
	if !ok {
		t.Fatal("the body did not read back")
	}
	if strings.Contains(body, "AKIAIOSFODNN7EXAMPLE") {
		t.Error("the credential reached the body")
	}
	if !strings.Contains(body, "building") || !strings.Contains(body, "done") {
		t.Errorf("body is %q, want everything but the credential", body)
	}
	if !strings.Contains(body, secret.Placeholder) {
		t.Errorf("body is %q, want a placeholder saying something was taken out", body)
	}
}

// The recorded path is relative and date-sharded, which is what makes `rm -rf`
// over a month the retention story.
func TestABodyIsFiledUnderItsDate(t *testing.T) {
	s, dir := newStore(t)
	rel, err := s.WriteBody(t.Context(), "BLOCK", time.Date(2026, 9, 5, 11, 2, 3, 0, time.UTC), "out")
	if err != nil {
		t.Fatal(err)
	}
	if rel != "body/2026/09/05/BLOCK.out" {
		t.Fatalf("path is %q, want it sharded by date", rel)
	}
	if filepath.IsAbs(rel) {
		t.Error("the recorded path is absolute — a store that moved would not resolve")
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
		t.Error(err)
	}
}

// A record whose body has been removed reads as a block with no output, which
// is the same state a block recorded without capture is in. One state, not two.
func TestADeletedBodyReadsAsNoOutput(t *testing.T) {
	s, dir := newStore(t)
	rel, err := s.WriteBody(t.Context(), "BLOCK", at(1000), "out")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
	if body, ok := s.Body(t.Context(), Record{Output: rel}); ok {
		t.Errorf("a removed body read back as %q", body)
	}
}

// A block is named by its id or by how far back it is, and the two forms
// cannot be confused.
func TestFindTakesAnIDOrARecencyNumber(t *testing.T) {
	s, _ := newStore(t)
	ids := []string{NewID(at(1000)), NewID(at(2000)), NewID(at(3000))}
	for i, id := range ids {
		mustAppend(t, s, Record{ID: id, Command: string(rune('a' + i))})
	}
	r, err := s.Find(t.Context(), "1", 100)
	if err != nil || r.Command != "c" {
		t.Fatalf("1 gave %+v, %v — want the most recent", r, err)
	}
	if r, err := s.Find(t.Context(), "3", 100); err != nil || r.Command != "a" {
		t.Fatalf("3 gave %+v, %v — want the third most recent", r, err)
	}
	if r, err := s.Find(t.Context(), ids[1], 100); err != nil || r.Command != "b" {
		t.Fatalf("an id gave %+v, %v", r, err)
	}
	for _, name := range []string{"0", "4", "NOTANID", ""} {
		if _, err := s.Find(t.Context(), name, 100); !errors.Is(err, ErrNoSuchBlock) {
			t.Errorf("%q gave %v, want ErrNoSuchBlock", name, err)
		}
	}
}

// base32hex's alphabet begins with the ten digits, so an id can be all digits.
// Length is what tells the two forms apart, and a rule that read only the
// characters would one day resolve a real id as a recency number.
func TestAnAllDigitIDIsStillAnID(t *testing.T) {
	s, _ := newStore(t)
	id := strings.Repeat("7", idLength)
	mustAppend(t, s, Record{ID: id, Command: "the all-digit one"})
	r, err := s.Find(t.Context(), id, 100)
	if err != nil || r.Command != "the all-digit one" {
		t.Fatalf("an all-digit id gave %+v, %v — it was read as a recency number", r, err)
	}
}

// The off state is a store every method tolerates, because it is a state a
// session arrives at deliberately and often.
func TestAStoreThatIsOffWritesNothingAndComplainsAboutNothing(t *testing.T) {
	s := Open("", boundary.Boundary{}, "SESSION")
	if err := s.Append(t.Context(), Record{ID: "A", Command: "x"}); err != nil {
		t.Errorf("append to an off store gave %v", err)
	}
	if rel, err := s.WriteBody(t.Context(), "A", at(1000), "out"); err != nil || rel != "" {
		t.Errorf("body on an off store gave %q, %v", rel, err)
	}
	if got := s.Load(t.Context(), 10); got != nil {
		t.Errorf("an off store loaded %v", got)
	}
	if err := s.Close(); err != nil {
		t.Errorf("closing an off store gave %v", err)
	}
	if s.Dir() != "" || s.Session() != "" {
		t.Errorf("an off store reports dir %q session %q", s.Dir(), s.Session())
	}
}

// The store is inside the boundary. SH_BLOCKS_DIR is a shell variable, so a
// typed line chooses the path, and an open a script can aim is an open a
// policy is entitled to refuse.
func TestADenyingPolicyGetsNoStore(t *testing.T) {
	dir := t.TempDir()
	var asked []string
	deny := interp.GateFunc(func(_ context.Context, a interp.Action) interp.Decision {
		asked = append(asked, a.Path)
		return interp.Deny
	})
	s := Open(dir, boundary.Boundary{Gate: deny}, "SESSION")
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Append(t.Context(), Record{ID: "A", Command: "x"}); err != nil {
		t.Errorf("a refused append gave %v, want silence", err)
	}
	if rel, err := s.WriteBody(t.Context(), "A", at(1000), "out"); err != nil || rel != "" {
		t.Errorf("a refused body gave %q, %v", rel, err)
	}
	if len(asked) == 0 {
		t.Fatal("the gate was never asked, so the store is outside the boundary")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("a refused store still wrote %v", entries)
	}
}

// The refusal is recorded, so an audit trail says the store was hidden rather
// than that nothing happened.
func TestARefusedStoreIsRecordedOnTheSink(t *testing.T) {
	var kinds []interp.EventKind
	sink := interp.SinkFunc(func(_ context.Context, e interp.Event) { kinds = append(kinds, e.Kind) })
	deny := interp.GateFunc(func(context.Context, interp.Action) interp.Decision { return interp.Deny })
	s := Open(t.TempDir(), boundary.Boundary{Gate: deny, Events: sink}, "SESSION")
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Append(t.Context(), Record{ID: "A", Command: "x"}); err != nil {
		t.Fatal(err)
	}
	if len(kinds) != 1 || kinds[0] != interp.EventDenied {
		t.Errorf("the sink saw %v, want one denial", kinds)
	}
}

// Asked once rather than per command: a policy that hides the store should not
// cost a gate call at every prompt.
func TestARefusedIndexIsAskedAboutOnce(t *testing.T) {
	n := 0
	deny := interp.GateFunc(func(context.Context, interp.Action) interp.Decision {
		n++
		return interp.Deny
	})
	s := Open(t.TempDir(), boundary.Boundary{Gate: deny}, "SESSION")
	t.Cleanup(func() { _ = s.Close() })
	for range 5 {
		if err := s.Append(t.Context(), Record{ID: "A", Command: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	if n != 1 {
		t.Errorf("the gate was asked %d times about the index, want 1", n)
	}
}
