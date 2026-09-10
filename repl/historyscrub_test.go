// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/syntax"
)

// Assembled rather than written out: this repository's commit hook scans for
// exactly these shapes, and a test file holding a literal one — invented or
// not — is a file nobody can commit. Neither is a real credential.
var (
	fakeKeyID = "AKIA" + strings.Repeat("Q", 16)
	fakeToken = "ghp" + "_" + strings.Repeat("a", 36)
)

// The property the whole feature exists for: a credential does not reach the
// file, and everything else does.
//
// Asserted against the bytes on disk rather than against what load gives
// back. A history file is read by other things than this shell — a person
// with a pager, a backup, whatever ends up with the machine — so "not in the
// file" is the claim, and "not returned by our own reader" would be a
// weaker one that a scrubbing step on the read side could satisfy while the
// token sat there.
func TestACredentialIsNotWrittenToTheHistoryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	h := historyFile{path: path, size: 100, file: 100}
	added := []string{
		"echo one",
		"export AWS_ACCESS_KEY_ID=" + fakeKeyID,
		"git push origin main",
		"curl -H \"Authorization: Bearer " + strings.Repeat("z", 40) + "\" https://api.example.com",
		"echo two",
	}
	if err := h.save(t.Context(), added); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{fakeKeyID, strings.Repeat("z", 40)} {
		if strings.Contains(string(raw), gone) {
			t.Errorf("the file holds a credential:\n%s", raw)
		}
	}
	got := h.load(t.Context())
	want := []string{"echo one", "git push origin main", "echo two"}
	if len(got) != len(want) {
		t.Fatalf("loaded %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d is %q, want %q", i, got[i], want[i])
		}
	}
	// The slice the caller handed over is the session's live history, and
	// the shell is still using it. Filtering it in place would delete lines
	// out from under the editor as the shell exits.
	if len(added) != 5 {
		t.Errorf("save edited the caller's slice, leaving %q", added)
	}
}

// A session that typed nothing but credentials leaves no file behind, rather
// than an empty one that says a shell was here and reveals when.
func TestNothingButCredentialsWritesNoFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist")
	if err := (historyFile{path: path, size: 100, file: 100}).save(t.Context(), []string{"export GH_TOKEN=" + fakeToken}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("a file was created for a history with nothing keepable in it")
	}
}

// The omission is visible. A history that quietly drops lines is its own
// confusion — the next person to look for the command finds nothing and
// concludes the shell lost it.
func TestTheNoticeSaysWhyAndWhich(t *testing.T) {
	var out bytes.Buffer
	e := &editor{}
	sh := Shell{Runner: newTestRunner(nil), Dialect: syntax.Core(), Err: &out, Name: "sh"}
	var pending strings.Builder
	var added []string
	record := sh.recording(e, &added)

	line := "export AWS_ACCESS_KEY_ID=" + fakeKeyID
	if _, _, _, ready := sh.take(&pending, record, line); !ready {
		t.Fatal("a complete line was not ready")
	}
	notice := out.String()
	if strings.Count(notice, "\n") != 1 {
		t.Errorf("the notice is %q, want one line", notice)
	}
	if !strings.Contains(notice, "sh: history:") {
		t.Errorf("the notice is %q, want it to say what it is about", notice)
	}
	if !strings.Contains(notice, "aws-access-key-id") {
		t.Errorf("the notice is %q, want the rule named — it is the only way to report a wrong one", notice)
	}
	// The credential is not in the notice. Printing the line back would put
	// it in a terminal log and in a `sh 2> log` file, which is the class of
	// thing this feature is about.
	if strings.Contains(notice, fakeKeyID) {
		t.Errorf("the notice repeated the credential: %q", notice)
	}

	// It is still recallable this session: the line is most often about to
	// be retyped, and taking the up arrow away is how a person ends up
	// turning the feature off.
	if len(e.history) != 1 || e.history[0] != line {
		t.Fatalf("the session history is %q, want the line still there", e.history)
	}
	// And it still does not reach the file, which is the part that outlives
	// the session.
	path := filepath.Join(t.TempDir(), "hist")
	if err := (historyFile{path: path, size: 100, file: 100}).save(t.Context(), e.history); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("the remembered line reached the file after all")
	}

	// An ordinary line says nothing at all. A notice on a line that is fine
	// is the same failure as a missing one, in the other direction.
	out.Reset()
	if _, _, _, ready := sh.take(&pending, record, "grep -r token ."); !ready {
		t.Fatal("a complete line was not ready")
	}
	if out.Len() != 0 {
		t.Errorf("an ordinary line printed %q", out.String())
	}
	if len(e.history) != 2 {
		t.Errorf("the session history is %q, want both lines", e.history)
	}
}

// A session without an editor has nothing to record into, and the wrapper
// says so rather than manufacturing one.
func TestNoHistoryMeansNoRecorder(t *testing.T) {
	if got := (Shell{}).recording(nil, nil); got != nil {
		t.Error("a shell with nowhere to remember produced a recorder")
	}
}

// The loop actually runs the rules on what somebody types.
//
// Through a real pseudo-terminal, because the terminal loop is the only one
// with a history and every other test here reaches past it: a wrapper that
// works perfectly and is not the one `Run` installs looks exactly like a
// working feature until the day someone opens their history file. This is the
// test that would have caught that, and it is the only one that sees the
// notice arrive where a person would see it — between the line they typed and
// the output of the next command.
func TestTheTerminalLoopScrubsWhatIsTyped(t *testing.T) {
	control, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() {
		_ = tty.Close()
		_ = control.Close()
	}()

	path := filepath.Join(t.TempDir(), "hist")
	out, errs := &syncBuffer{}, &syncBuffer{}
	r := newTestRunner(map[string]string{"HISTFILE": path, "PS1": "$ "})
	r.Stdout, r.Stderr = out, errs
	s := Shell{Runner: r, In: tty, Out: out, Err: errs, Name: "sh"}

	done := make(chan error, 1)
	go func() {
		_, err := s.Run(t.Context())
		done <- err
	}()

	// Typed only once the prompt is on the screen. Input written before the
	// shell has the terminal in raw mode is read under the line discipline
	// that was there first, which is a different question from the one this
	// test is asking.
	waitFor(t, out, "$ ", "the first prompt")
	typed := "export AWS_ACCESS_KEY_ID=" + fakeKeyID + "\n"
	if _, err := control.WriteString(typed); err != nil {
		t.Fatal(err)
	}
	waitFor(t, errs, "history:", "the notice")
	if got := errs.String(); !strings.Contains(got, "aws-access-key-id") {
		t.Errorf("the notice is %q, want the rule named", got)
	}
	// ^D on an empty line ends the session, which is where the file is
	// written — but only once the shell is reading again. It puts the
	// terminal back in its own line discipline to run a line and takes it
	// into raw mode afterwards, and the kernel drops what is queued but
	// unread across that change, so a ^D sent any earlier is simply gone and
	// the session never ends. The next prompt is drawn after raw mode is
	// restored and immediately before the read, which makes it the mark to
	// wait on rather than a duration to guess at.
	waitFor(t, out, "$ ", "the prompt after the line ran")
	if _, err := control.WriteString("\x04"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the shell did not exit on ^D")
	}

	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), fakeKeyID) {
		t.Errorf("the credential reached the history file:\n%s", raw)
	}
}

// syncBuffer is a buffer the test reads while the shell is writing to it.
type syncBuffer struct {
	mu   sync.Mutex
	b    strings.Builder
	seen int
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// Len is how much has been written, for a caller that wants to know the screen
// changed without caring what it says — see session.typeLine, which uses it to
// wait for the draw its last keystroke caused.
func (s *syncBuffer) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Len()
}

// seek reports whether the text has arrived since the last thing waited for
// did, and moves the cursor past it if so.
//
// A cursor rather than a search of everything written, because a session
// writes the same prompt after every line: without one, a second wait for
// `$ ` is answered by the first prompt and waits for nothing. Nothing is
// discarded — the whole buffer is what a failure prints.
func (s *syncBuffer) seek(want string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := strings.Index(s.b.String()[s.seen:], want)
	if i < 0 {
		return false
	}
	s.seen += i + len(want)
	return true
}

// waitFor blocks until the text appears, or fails the test saying what it was
// waiting for and what it had instead.
func waitFor(t *testing.T, buf *syncBuffer, want, what string) {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		if buf.seek(want) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("waiting for %s: %q never arrived, and what did was %q", what, want, buf.String())
}

// TestAWaitIsNotSatisfiedByAPromptItAlreadySaw is why the buffer carries a
// cursor.
//
// The session writes the same prompt after every line, so a wait that
// searched everything written would be answered by the first prompt for the
// rest of the run — and a test that then types is typing at a shell that may
// still be between line disciplines, where the kernel drops what is queued
// but unread. That is how a ^D goes missing (#635).
func TestAWaitIsNotSatisfiedByAPromptItAlreadySaw(t *testing.T) {
	var buf syncBuffer
	if _, err := buf.Write([]byte("$ ")); err != nil {
		t.Fatal(err)
	}

	if !buf.seek("$ ") {
		t.Fatal("the first prompt was not found at all")
	}
	if buf.seek("$ ") {
		t.Error("the same prompt answered twice, so a later wait would not wait")
	}

	if _, err := buf.Write([]byte("history: aws-access-key-id\n$ ")); err != nil {
		t.Fatal(err)
	}
	if !buf.seek("history:") {
		t.Error("the notice was not found")
	}
	if !buf.seek("$ ") {
		t.Error("the prompt drawn after the line ran was not found")
	}
	if buf.seek("$ ") {
		t.Error("a third prompt was found where only two were written")
	}

	// And nothing is discarded: the whole buffer is what a failure prints.
	if got := buf.String(); !strings.Contains(got, "history:") {
		t.Errorf("the buffer is %q, want everything written kept for the diagnostic", got)
	}
}

// TestAWaitBlocksUntilThePromptIsWrittenAgain: waitFor is what the test
// actually calls, so it is checked rather than only the search under it.
//
// The second prompt is arranged to arrive late and the assertion is on the
// lower bound, so load can only lengthen the wait — the direction that keeps
// this from becoming a flake in its own right.
func TestAWaitBlocksUntilThePromptIsWrittenAgain(t *testing.T) {
	const late = 50 * time.Millisecond

	var buf syncBuffer
	if _, err := buf.Write([]byte("$ ")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, &buf, "$ ", "the first prompt")

	go func() {
		time.Sleep(late)
		_, _ = buf.Write([]byte("history: aws-access-key-id\n$ "))
	}()

	start := time.Now()
	waitFor(t, &buf, "$ ", "the prompt after the line ran")
	if waited := time.Since(start); waited < late {
		t.Errorf("the wait returned after %v, before the second prompt was written at %v", waited, late)
	}
}
