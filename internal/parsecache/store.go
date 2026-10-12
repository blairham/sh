// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package parsecache keeps the trees of the files a shell sources from one
// run of it to the next (#6424): a Reader that answers a file's reads from
// the cache while what the cache recorded still holds, and a Store that keeps
// the entries on disk, under `$XDG_CACHE_HOME/sh/parse` or
// `~/.cache/sh/parse`. It sits beside the completion dump from #6317 and
// keeps its rules: the format is this shell's own, it is written only by
// this shell, and nothing another shell keeps is read or written.
//
// Whether an entry may be used is the Reader's to decide, on every read,
// against the text and the running shell; see Reader. The Store knows
// nothing about trees. It keeps opaque entries by name and guarantees three
// things about them: that an entry comes back exactly as it was stored or not
// at all, that it was stored by this very build of the shell, and that no
// reader ever sees half of one.
//
//   - Each entry is a file of its own, named by a hash of the build and the
//     entry's name, so two builds never read each other's and one file is one
//     entry.
//   - A file is the entry framed by a magic and the build that wrote it in
//     front, and its length and a CRC-32C over all of it behind. A file that
//     is short, damaged, or another build's is not handed back, and the next
//     write replaces it.
//   - An entry is written as it is made into a file of this process's own,
//     opened exclusively, and renamed over the entry once it is whole. Two
//     shells writing one entry at once each rename a whole file, and the
//     last rename wins.
//   - The directory is made 0700 and every file 0600.
//   - The directory is held under a size bound. Once per process, before its
//     first write, the oldest entries are removed until the rest fit in three
//     quarters of it. An entry that is read is touched now and then, so the
//     oldest are the ones nobody reads.
//   - A process records no more than the cache has saved it, beyond a small
//     allowance; see DefaultBudget.
//   - The cache's parent directory is never created: a home with no `~/.cache`
//     gets no cache. That keeps a scratch home — a test's, a container's —
//     exactly as the shell found it.
package parsecache

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultLimit is the size the directory is held under.
const DefaultLimit = 32 << 20

// DefaultBudget is how much text a process may record into the cache before
// the cache has saved it anything, as bytes of source.
//
// Recording a text costs its encoding on top of its parse — about a quarter
// of a parse, measured — and the start that pays it is a start somebody is
// waiting for. Unbounded, the first start after a change recorded the whole
// of a real configuration's 1.2 MB and was about 10 ms slower to a usable
// line than a shell with no cache. So a process starts with this much, and
// every text it reads out of the cache rather than parsing earns it twice
// that text's size more (see Credit): reading a kilobyte back saves two to
// three times what recording one costs. A start therefore never spends on
// the cache more than the cache saved it, beyond this, and a configuration
// fills in a handful of starts — each recording more than the last, because
// each reads more back.
const DefaultBudget = 64 << 10

// Credit is a text of size bytes read out of the cache rather than parsed,
// which pays for recording twice as much. See DefaultBudget.
func (s *Store) Credit(size int) { s.budget.Add(2 * int64(size)) }

// Dir is where the cache lives for an environment: `$XDG_CACHE_HOME/sh/parse`,
// or `$HOME/.cache/sh/parse` where XDG_CACHE_HOME is unset or not absolute —
// the XDG base directory specification says a relative value is ignored. ok is
// false when there is no such parent directory already: the cache never
// creates one.
func Dir(getenv func(string) (string, bool)) (dir string, ok bool) {
	base := ""
	if x, set := getenv("XDG_CACHE_HOME"); set && filepath.IsAbs(x) {
		base = x
	} else if h, set := getenv("HOME"); set && filepath.IsAbs(h) {
		base = filepath.Join(h, ".cache")
	}
	if base == "" {
		return "", false
	}
	if fi, err := os.Stat(base); err != nil || !fi.IsDir() {
		return "", false
	}
	return filepath.Join(base, "sh", "parse"), true
}

// BuildIdentity is which build of the shell is running: an entry one build
// wrote is never read by another, since what a tree is depends on the parser
// that made it. The module and VCS stamps, and the executable's own size and
// time, which is what tells two development builds of one revision apart.
var BuildIdentity = sync.OnceValue(func() string {
	var b strings.Builder
	if bi, ok := debug.ReadBuildInfo(); ok {
		b.WriteString(bi.GoVersion + " " + bi.Main.Path + " " + bi.Main.Version + " " + bi.Main.Sum)
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision", "vcs.time", "vcs.modified":
				b.WriteString(" " + s.Key + "=" + s.Value)
			}
		}
	}
	if exe, err := os.Executable(); err == nil {
		if fi, err := os.Stat(exe); err == nil {
			fmt.Fprintf(&b, " %s %d %d", exe, fi.Size(), fi.ModTime().UnixNano())
		}
	}
	return b.String()
})

// Store is a parse cache in one directory, for one build.
type Store struct {
	dir   string
	build string
	limit int64

	prepare sync.Once
	ready   bool

	budget atomic.Int64

	listed sync.Once
	held   map[string]bool
}

// Open is the store in dir for the build named build. Nothing is touched on
// disk until the first Load or Store.
func Open(dir, build string, limit, budget int64) *Store {
	s := &Store{dir: dir, build: build, limit: limit}
	s.budget.Store(budget)
	return s
}

const (
	magic   = "SHPF"
	version = 1
	// touchAfter is how stale an entry's time may get before a read
	// refreshes it, so the order eviction goes in is the order of use
	// without a write on every read.
	touchAfter = 24 * time.Hour
	// staleTemp is how old a temporary file must be before it is taken
	// for a dead process's and removed.
	staleTemp = time.Hour
)

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

func (s *Store) path(name string) string {
	h := sha256.Sum256([]byte(s.build + "\x00" + name))
	return filepath.Join(s.dir, hex.EncodeToString(h[:20]))
}

// mayHold reports whether the directory held a file of this name when the
// process first looked, which it does once: a start reads a score of files,
// and on the first start after a change most of them are not there — a
// listing is one call where a failed open each was twenty. A file written
// since is only ever this process's own, and its own readers do not look for
// it again.
func (s *Store) mayHold(file string) bool {
	s.listed.Do(func() {
		entries, err := os.ReadDir(s.dir)
		if err != nil {
			return
		}
		s.held = make(map[string]bool, len(entries))
		for _, e := range entries {
			s.held[e.Name()] = true
		}
	})
	return s.held[file]
}

// Load is the entry stored under name, or nil.
func (s *Store) Load(name string) []byte {
	path := s.path(name)
	if !s.mayHold(filepath.Base(path)) {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return nil
	}
	data := make([]byte, fi.Size())
	if _, err := io.ReadFull(f, data); err != nil {
		return nil
	}
	payload, ok := s.unframe(data)
	if !ok {
		return nil
	}
	if time.Since(fi.ModTime()) > touchAfter {
		now := time.Now()
		_ = os.Chtimes(path, now, now)
	}
	return payload
}

// head is the frame in front of an entry. The frame's tail — the entry's
// length and a checksum over everything before it — follows the entry, so
// that an entry can be written as it is made.
func (s *Store) head() []byte {
	b := make([]byte, 0, len(magic)+1+len(s.build)+binary.MaxVarintLen64)
	b = append(b, magic...)
	b = append(b, version)
	b = binary.AppendUvarint(b, uint64(len(s.build)))
	return append(b, s.build...)
}

func (s *Store) unframe(b []byte) ([]byte, bool) {
	const tail = 8 + 4
	if len(b) < len(magic)+1+tail {
		return nil, false
	}
	body, sum := b[:len(b)-4], binary.LittleEndian.Uint32(b[len(b)-4:])
	if crc32.Checksum(body, castagnoli) != sum {
		return nil, false
	}
	n := binary.LittleEndian.Uint64(body[len(body)-8:])
	body = body[:len(body)-8]
	if !bytes.HasPrefix(body, []byte(magic)) || body[len(magic)] != version {
		return nil, false
	}
	body = body[len(magic)+1:]
	k, w := binary.Uvarint(body)
	if w <= 0 || k > uint64(len(body)-w) || string(body[w:w+int(k)]) != s.build {
		return nil, false
	}
	body = body[w+int(k):]
	if n != uint64(len(body)) {
		return nil, false
	}
	return body, true
}

// Create starts an entry under name, written as it is made into a file of
// this process's own beside the entry and renamed over it by done(true).
// Two shells writing one entry at once each write a whole file and rename
// it, and the last rename wins; a reader never sees half of one. A failure
// anywhere is silent: the cache is an optimization, and a shell that cannot
// write one parses next time.
func (s *Store) Create(name string, size int) (io.Writer, func(keep bool)) {
	// What is left of this process's allowance, which a text may overdraw
	// once but not start on when it is spent. See DefaultBudget.
	if left := s.budget.Add(-int64(size)); left+int64(size) <= 0 {
		return nil, nil
	}
	s.prepare.Do(s.setUp)
	if !s.ready {
		return nil, nil
	}
	path := s.path(name)
	// The process, a count within it — a shell's subshells write from one
	// process at once — and the time, so a name a dead process left is not
	// one this process will pick.
	tmp := fmt.Sprintf("%s.%d.%d.%d.tmp", path, os.Getpid(), tmpSeq.Add(1), time.Now().UnixNano())
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, nil
	}
	w := &entryWriter{f: f, buf: writers.Get().(*bufio.Writer)}
	w.buf.Reset(f)
	head := s.head()
	w.sum = crc32.Update(0, castagnoli, head)
	_, w.err = w.buf.Write(head)
	return w, func(keep bool) { w.finish(keep, tmp, path) }
}

var tmpSeq atomic.Int64

// writers are the buffers entries are written through, kept between entries
// so that a start writing twenty of them does not allocate twenty.
var writers = sync.Pool{New: func() any { return bufio.NewWriterSize(nil, 64<<10) }}

// entryWriter is one entry on its way to disk.
type entryWriter struct {
	f   *os.File
	buf *bufio.Writer
	sum uint32
	n   uint64
	err error
}

func (w *entryWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	w.sum = crc32.Update(w.sum, castagnoli, p)
	w.n += uint64(len(p))
	n, err := w.buf.Write(p)
	w.err = err
	return n, err
}

func (w *entryWriter) finish(keep bool, tmp, path string) {
	if keep && w.err == nil {
		tail := binary.LittleEndian.AppendUint64(nil, w.n)
		w.sum = crc32.Update(w.sum, castagnoli, tail)
		tail = binary.LittleEndian.AppendUint32(tail, w.sum)
		if _, err := w.buf.Write(tail); err == nil {
			w.err = w.buf.Flush()
		} else {
			w.err = err
		}
	}
	w.buf.Reset(nil)
	writers.Put(w.buf)
	err := w.f.Close()
	if keep && w.err == nil && err == nil && os.Rename(tmp, path) == nil {
		return
	}
	_ = os.Remove(tmp)
}

// setUp makes the directory and holds it under the limit, once per process
// and before its first write.
func (s *Store) setUp() {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return
	}
	// Ours, so ours to hold at 0700 whatever made it first.
	if err := os.Chmod(s.dir, 0o700); err != nil {
		return
	}
	s.ready = true
	s.evict()
}

// evict removes the oldest entries until the rest fit in three quarters of the
// limit, and any temporary file a process left behind.
func (s *Store) evict() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	type file struct {
		path string
		size int64
		mod  time.Time
	}
	var files []file
	var total int64
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		p := filepath.Join(s.dir, e.Name())
		if strings.HasSuffix(e.Name(), ".tmp") {
			if time.Since(fi.ModTime()) > staleTemp {
				_ = os.Remove(p)
			}
			continue
		}
		files = append(files, file{p, fi.Size(), fi.ModTime()})
		total += fi.Size()
	}
	if total <= s.limit {
		return
	}
	slices.SortFunc(files, func(a, b file) int { return a.mod.Compare(b.mod) })
	for _, f := range files {
		if total <= s.limit*3/4 {
			break
		}
		if err := os.Remove(f.path); err == nil || errors.Is(err, fs.ErrNotExist) {
			total -= f.size
		}
	}
}
