// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package parsecache_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blairham/sh/internal/parsecache"
)

func envOf(m map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		v, ok := m[name]
		return v, ok
	}
}

// TestTheCacheLivesWhereXDGSays is the location rule #6317 set for the
// completion dump, and the one this cache adds: its parent is never made.
func TestTheCacheLivesWhereXDGSays(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".cache"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"XDG_CACHE_HOME", map[string]string{"XDG_CACHE_HOME": xdg, "HOME": home}, filepath.Join(xdg, "sh", "parse")},
		{"a relative XDG_CACHE_HOME is ignored", map[string]string{"XDG_CACHE_HOME": "rel", "HOME": home}, filepath.Join(home, ".cache", "sh", "parse")},
		{"HOME", map[string]string{"HOME": home}, filepath.Join(home, ".cache", "sh", "parse")},
		{"nothing", map[string]string{}, ""},
		{"a relative HOME", map[string]string{"HOME": "rel"}, ""},
		{"a parent that is not there", map[string]string{"XDG_CACHE_HOME": filepath.Join(xdg, "nope")}, ""},
	} {
		got, ok := parsecache.Dir(envOf(c.env))
		if got != c.want || ok != (c.want != "") {
			t.Errorf("%s: %q %v, want %q", c.name, got, ok, c.want)
		}
	}
	// A home with no ~/.cache has no cache, and is left as it was found.
	bare := t.TempDir()
	if _, ok := parsecache.Dir(envOf(map[string]string{"HOME": bare})); ok {
		t.Fatal("a home with no .cache was given a cache")
	}
	if entries, _ := os.ReadDir(bare); len(entries) != 0 {
		t.Fatalf("the home was written to: %v", entries)
	}
}

func store(t *testing.T, dir, build string) *parsecache.Store {
	t.Helper()
	return parsecache.Open(dir, build, parsecache.DefaultLimit, 1<<30)
}

func put(t *testing.T, s *parsecache.Store, name string, data []byte) {
	t.Helper()
	w, done := s.Create(name)
	if w == nil {
		t.Fatal("the store would not take an entry")
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	done(true)
}

func entryFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out
}

// TestAnEntryComesBackWholeOrNotAtAll damages a stored entry every way a
// file can be damaged — every byte changed, every length cut, another
// build's, empty — and requires each to come back as nothing. A tree read
// from damaged bytes could be a different tree, so this is what stands
// between a disk fault and a wrong program.
func TestAnEntryComesBackWholeOrNotAtAll(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "parse")
	s := store(t, dir, "build-1")
	data := bytes.Repeat([]byte("an entry's bytes "), 40)
	put(t, s, "e", data)
	files := entryFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("%d files", len(files))
	}
	path := files[0]
	good, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	load := func() []byte { return store(t, dir, "build-1").Load("e") }
	if !bytes.Equal(load(), data) {
		t.Fatal("the entry did not come back as stored: the checks below prove nothing")
	}
	write := func(b []byte) {
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for i := range len(good) {
		bad := bytes.Clone(good)
		bad[i] ^= 0x01
		write(bad)
		if got := load(); got != nil {
			t.Fatalf("an entry with byte %d changed came back", i)
		}
	}
	for n := range len(good) {
		write(good[:n])
		if got := load(); got != nil {
			t.Fatalf("an entry cut to %d bytes came back", n)
		}
	}
	// Another build's entry, under the name this build would read: the
	// name is made from the build as well, so this is the frame's own check.
	other := filepath.Join(t.TempDir(), "parse")
	put(t, store(t, other, "build-2"), "e", []byte("x"))
	theirs := entryFiles(t, other)[0]
	if err := os.WriteFile(theirs, good, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := store(t, other, "build-2").Load("e"); got != nil {
		t.Fatal("another build's entry came back")
	}
	write(good)
	if !bytes.Equal(load(), data) {
		t.Fatal("the entry put back did not come back")
	}
}

// TestTheDirectoryIsPrivate is 0700 for the directory and 0600 for every
// entry, whatever the umask leaves.
func TestTheDirectoryIsPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sh", "parse")
	put(t, store(t, dir, "b"), "e", []byte("x"))
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("the directory is %v", fi.Mode().Perm())
	}
	for _, f := range entryFiles(t, dir) {
		fi, err := os.Stat(f)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("%s is %v", f, fi.Mode().Perm())
		}
	}
}

// TestConcurrentWritersLeaveWholeEntries starts a dozen shells' stores on one
// directory at once, every one writing the same entries over and over while
// others read them. A reader may find an entry or not; it must never find
// one that is not exactly something a writer wrote, and when they are all
// done every entry is whole and nothing temporary is left.
func TestConcurrentWritersLeaveWholeEntries(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "parse")
	payload := func(writer, round int) []byte {
		return bytes.Repeat([]byte(fmt.Sprintf("<writer %d round %d>", writer, round)), 300+writer*7)
	}
	valid := func(b []byte) bool {
		var w, r int
		if _, err := fmt.Sscanf(string(b), "<writer %d round %d>", &w, &r); err != nil {
			return false
		}
		return bytes.Equal(b, payload(w, r))
	}
	const writers, rounds, names = 12, 20, 4
	var wg sync.WaitGroup
	errs := make(chan error, writers*2)
	for w := range writers {
		wg.Add(2)
		go func() {
			defer wg.Done()
			s := parsecache.Open(dir, "b", parsecache.DefaultLimit, 1<<30)
			for r := range rounds {
				data := payload(w, r)
				wr, done := s.Create(fmt.Sprintf("e%d", r%names))
				if wr == nil {
					errs <- fmt.Errorf("writer %d: no entry", w)
					return
				}
				// In pieces, as a reader writes an entry while it reads.
				for len(data) > 0 {
					n := min(len(data), 500)
					if _, err := wr.Write(data[:n]); err != nil {
						errs <- err
						return
					}
					data = data[n:]
				}
				done(true)
			}
		}()
		go func() {
			defer wg.Done()
			s := parsecache.Open(dir, "b", parsecache.DefaultLimit, 1<<30)
			for r := range rounds * 3 {
				if got := s.Load(fmt.Sprintf("e%d", r%names)); got != nil && !valid(got) {
					errs <- fmt.Errorf("a reader found a mixed entry of %d bytes", len(got))
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	s := parsecache.Open(dir, "b", parsecache.DefaultLimit, 1<<30)
	for i := range names {
		if got := s.Load(fmt.Sprintf("e%d", i)); !valid(got) {
			t.Fatalf("entry e%d is not whole after the writers finished", i)
		}
	}
	for _, f := range entryFiles(t, dir) {
		if strings.HasSuffix(f, ".tmp") {
			t.Fatalf("a temporary file was left: %s", f)
		}
	}
}

// TestADroppedEntryIsNotPutInPlace throws an entry away partway, as a reader
// does when its text turns out not to be one it can keep: the old entry
// stays, and nothing temporary is left.
func TestADroppedEntryIsNotPutInPlace(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "parse")
	s := store(t, dir, "b")
	put(t, s, "e", []byte("the old one"))
	w, done := s.Create("e")
	if _, err := w.Write([]byte("half of a new")); err != nil {
		t.Fatal(err)
	}
	done(false)
	if got := store(t, dir, "b").Load("e"); string(got) != "the old one" {
		t.Fatalf("got %q", got)
	}
	if n := len(entryFiles(t, dir)); n != 1 {
		t.Fatalf("%d files", n)
	}
}

// TestTheDirectoryIsBounded fills the directory past its limit and starts a
// new process's store: before its first write it removes the oldest entries
// until the rest fit in three quarters of the limit, and a temporary file a
// dead process left behind.
func TestTheDirectoryIsBounded(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "parse")
	const limit = 64 << 10
	s := parsecache.Open(dir, "b", limit, 1<<30)
	entry := bytes.Repeat([]byte("x"), 4<<10)
	old := time.Now().Add(-48 * time.Hour)
	for i := range 40 {
		put(t, s, fmt.Sprintf("e%02d", i), entry)
		files := entryFiles(t, dir)
		// Age them in order, so the first written is the oldest.
		for _, f := range files {
			if fi, _ := os.Stat(f); fi.ModTime().After(old.Add(time.Hour)) {
				ts := old.Add(time.Duration(i) * time.Minute)
				_ = os.Chtimes(f, ts, ts)
			}
		}
	}
	stale := filepath.Join(dir, "dead.123.456.tmp")
	if err := os.WriteFile(stale, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(stale, old, old)
	before := len(entryFiles(t, dir))
	put(t, parsecache.Open(dir, "b", limit, 1<<30), "new", entry)
	var total int64
	for _, f := range entryFiles(t, dir) {
		fi, err := os.Stat(f)
		if err != nil {
			t.Fatal(err)
		}
		total += fi.Size()
	}
	if total > limit {
		t.Fatalf("the directory holds %d bytes after eviction, limit %d (%d files before)", total, limit, before)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Fatal("a dead process's temporary file was left")
	}
	// The oldest went and the newest stayed.
	n := parsecache.Open(dir, "b", limit, 1<<30)
	if n.Load("e00") != nil || n.Load("e39") == nil || n.Load("new") == nil {
		t.Fatal("eviction did not take the oldest")
	}
}

// TestRecordingIsBudgeted spends a process's allowance. On the first start a
// text that fits is taken and one that does not is put off; a later start may
// overdraw for one, once, and then nothing more until reading from the cache
// earns more.
func TestRecordingIsBudgeted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "parse")
	first := parsecache.Open(dir, "b", parsecache.DefaultLimit, 1000)
	if !first.Spend(600) {
		t.Fatal("a text that fits was refused")
	}
	if first.Spend(600) {
		t.Fatal("a text that does not fit was taken on the first start")
	}
	if first.Spend(5000) {
		t.Fatal("a text bigger than the allowance was taken on the first start")
	}
	later := parsecache.Open(dir, "b", parsecache.DefaultLimit, 1000)
	if !later.Spend(5000) {
		t.Fatal("a later start could not overdraw for a text bigger than its allowance")
	}
	if later.Spend(1) {
		t.Fatal("a spent allowance was overdrawn again")
	}
	later.Credit(2500) // pays off the overdraft and leaves 1000
	if !later.Spend(100) {
		t.Fatal("a text read from the cache did not pay for recording another")
	}
	// Another build has not been here.
	if parsecache.Open(dir, "c", parsecache.DefaultLimit, 1000).Spend(5000) {
		t.Fatal("another build overdrew on this build's mark")
	}
}

// TestAnEntryReadIsKeptFresh reads an entry whose time is a day old and finds
// it touched, so eviction takes the entries nobody reads.
func TestAnEntryReadIsKeptFresh(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "parse")
	put(t, store(t, dir, "b"), "e", []byte("x"))
	f := entryFiles(t, dir)[0]
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(f, old, old); err != nil {
		t.Fatal(err)
	}
	if store(t, dir, "b").Load("e") == nil {
		t.Fatal("the entry did not come back")
	}
	fi, err := os.Stat(f)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(fi.ModTime()) > time.Hour {
		t.Fatalf("the entry read was left at %v", fi.ModTime())
	}
}
