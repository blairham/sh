// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package fswatch_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blairham/sh/internal/fswatch"
)

// open starts a watcher whose notifications land in a channel, closed on the
// way out so that nothing this test started outlives it.
func open(t *testing.T) (*fswatch.Watcher, chan struct{}) {
	t.Helper()
	changed := make(chan struct{}, 64)
	w, err := fswatch.Open(func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	if errors.Is(err, fswatch.ErrUnsupported) {
		t.Skip("no filesystem watch on this system")
	}
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := w.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return w, changed
}

// await waits for one notification, and fails rather than hanging.
func await(t *testing.T, changed chan struct{}, what string) {
	t.Helper()
	select {
	case <-changed:
	case <-time.After(10 * time.Second):
		t.Fatalf("nothing was reported for %s", what)
	}
}

func TestAFileArrivingIsAChange(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	w, changed := open(t)
	if err := w.Add(dir); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	await(t, changed, "a file arriving")
}

func TestAFileRenamedIntoPlaceIsAChange(t *testing.T) {
	t.Parallel()
	// This is the case the caller actually has. A repository moves its
	// branch and rewrites its index by writing a temporary file and renaming
	// it over the old one, so a watch that only heard about writes to a file
	// it already held would hear nothing at all.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, changed := open(t)
	if err := w.Add(dir); err != nil {
		t.Fatalf("Add: %v", err)
	}

	tmp := filepath.Join(dir, "HEAD.lock")
	if err := os.WriteFile(tmp, []byte("ref: two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, "HEAD")); err != nil {
		t.Fatal(err)
	}
	await(t, changed, "a file renamed into place")
}

func TestADirectoryThatIsNotThereIsNothingToWatchAndNotAFailure(t *testing.T) {
	t.Parallel()
	// A repository has paths that exist only while an operation is running,
	// and a caller naming one is saying "tell me if it turns up". This
	// cannot, and must not fail over being asked.
	w, _ := open(t)
	if err := w.Add(filepath.Join(t.TempDir(), "nowhere")); err != nil {
		t.Errorf("Add of an absent path: %v", err)
	}
}

func TestClosingTwiceIsNotAnError(t *testing.T) {
	t.Parallel()
	changed := make(chan struct{}, 1)
	w, err := fswatch.Open(func() { changed <- struct{}{} })
	if errors.Is(err, fswatch.ErrUnsupported) {
		t.Skip("no filesystem watch on this system")
	}
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestCloseReturnsOnlyOnceTheWatchHasStopped(t *testing.T) {
	t.Parallel()
	// The property that matters for a shell: nothing a session started is
	// still running when the session has gone. Close waits for its own
	// goroutine rather than signaling it and hoping.
	dir := t.TempDir()
	w, changed := open(t)
	if err := w.Add(dir); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "one"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	await(t, changed, "a file arriving")

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Drain whatever was already delivered, then write again: a closed
	// watcher reports nothing, because its goroutine has returned.
	for len(changed) > 0 {
		<-changed
	}
	if err := os.WriteFile(filepath.Join(dir, "two"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changed:
		t.Error("a closed watcher reported a change")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestAChangeIsReportedAndThenTheWatcherGoesQuiet(t *testing.T) {
	t.Parallel()
	// The failure this guards is not a missed event but the opposite: an
	// event left queued keeps the descriptor readable, so the wait returns
	// at once, for ever, and the watcher holds a core while reporting a
	// change that happened once. Measured on the shell being modeled, a
	// descriptor that is permanently ready costs a whole core — see
	// repl/watchfd.go — and a watcher doing it to itself is worse, because
	// nothing on the screen would say so.
	dir := t.TempDir()
	w, changed := open(t)
	if err := w.Add(dir); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "one"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	await(t, changed, "a file arriving")

	// Whatever else that one write produced, settled.
	time.Sleep(300 * time.Millisecond)
	for len(changed) > 0 {
		<-changed
	}
	// And now nothing is happening, so nothing should be reported.
	time.Sleep(300 * time.Millisecond)
	if n := len(changed); n != 0 {
		t.Errorf("%d changes reported with nothing happening", n)
	}
}
