// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package parsecache is where the shell keeps the parse cache on disk: a
// directory of files under `$XDG_CACHE_HOME/sh/parse`, or `~/.cache/sh/parse`
// (#6424). It sits beside the completion dump from #6317 and keeps its rules:
// the format is this shell's own, it is written only by this shell, and
// nothing another shell keeps is read or written.
//
// The store knows nothing about trees. It keeps opaque entries by name and
// guarantees three things about them: that an entry comes back exactly as it
// was stored or not at all, that it was stored by this very build of the
// shell, and that no reader ever sees half of one. Whether an entry may be
// used is decided by interp, which checks it against the text and the shell
// on every read; see interp.ParseCache.
//
//   - Each entry is a file of its own, named by a hash of the build and the
//     entry's name, so two builds never read each other's and one file is one
//     entry.
//   - A file is the entry framed by a magic, the build that wrote it, its
//     length and a CRC-32C over all of it. A file that is short, damaged, or
//     another build's is not handed back, and the next write replaces it.
//   - A write goes to a file of this process's own, opened exclusively, and is
//     renamed over the entry. Two shells writing one entry at once each rename
//     a whole file, and the last rename wins.
//   - The directory is made 0700 and every file 0600.
//   - The directory is held under a size bound. Once per process, before its
//     first write, the oldest entries are removed until the rest fit in three
//     quarters of it. An entry that is read is touched now and then, so the
//     oldest are the ones nobody reads.
//   - The cache's parent directory is never created: a home with no `~/.cache`
//     gets no cache. That keeps a scratch home — a test's, a container's —
//     exactly as the shell found it.
package parsecache

import (
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
	"time"
)

// DefaultLimit is the size the directory is held under.
const DefaultLimit = 32 << 20

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

	mu      sync.Mutex
	queue   []job
	running bool
}

// Open is the store in dir for the build named build. Nothing is touched on
// disk until the first Load or Store.
func Open(dir, build string, limit int64) *Store {
	return &Store{dir: dir, build: build, limit: limit}
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

// Load is the entry stored under name, or nil.
func (s *Store) Load(name string) []byte {
	path := s.path(name)
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

func (s *Store) frame(payload []byte) []byte {
	b := make([]byte, 0, len(magic)+1+len(s.build)+len(payload)+24)
	b = append(b, magic...)
	b = append(b, version)
	b = binary.AppendUvarint(b, uint64(len(s.build)))
	b = append(b, s.build...)
	b = binary.AppendUvarint(b, uint64(len(payload)))
	b = append(b, payload...)
	return binary.LittleEndian.AppendUint32(b, crc32.Checksum(b, castagnoli))
}

func (s *Store) unframe(b []byte) ([]byte, bool) {
	if len(b) < len(magic)+1+4 {
		return nil, false
	}
	body, sum := b[:len(b)-4], binary.LittleEndian.Uint32(b[len(b)-4:])
	if crc32.Checksum(body, castagnoli) != sum {
		return nil, false
	}
	if !bytes.HasPrefix(body, []byte(magic)) || body[len(magic)] != version {
		return nil, false
	}
	body = body[len(magic)+1:]
	n, k := binary.Uvarint(body)
	if k <= 0 || n > uint64(len(body)-k) || string(body[k:k+int(n)]) != s.build {
		return nil, false
	}
	body = body[k+int(n):]
	n, k = binary.Uvarint(body)
	if k <= 0 || n != uint64(len(body)-k) {
		return nil, false
	}
	return body[k:], true
}

// Store keeps data under name. A failure is silent: the cache is an
// optimization, and a shell that cannot write one parses next time.
//
// The write happens on a goroutine of the store's own, in the order the
// entries were stored, so the shell reading its startup files does not wait
// on the disk. Wait is how a process that is about to end lets them land.
func (s *Store) Store(name string, data []byte) {
	pending.Add(1)
	s.mu.Lock()
	s.queue = append(s.queue, job{name, data})
	start := !s.running
	s.running = true
	s.mu.Unlock()
	if start {
		go s.drain()
	}
}

// Wait returns once every entry stored so far, by any store in the process,
// has been written or given up on.
func Wait() { pending.Wait() }

var pending sync.WaitGroup

type job struct {
	name string
	data []byte
}

func (s *Store) drain() {
	for {
		s.mu.Lock()
		if len(s.queue) == 0 {
			s.running = false
			s.mu.Unlock()
			return
		}
		j := s.queue[0]
		s.queue = s.queue[1:]
		s.mu.Unlock()
		s.write(j.name, j.data)
		pending.Done()
	}
}

func (s *Store) write(name string, data []byte) {
	s.prepare.Do(s.setUp)
	if !s.ready {
		return
	}
	path := s.path(name)
	// A name of this process's own, opened exclusively: two shells writing
	// the same entry each write a whole file of their own and rename it.
	tmp := fmt.Sprintf("%s.%d.%d.tmp", path, os.Getpid(), time.Now().UnixNano())
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return
	}
	_, werr := f.Write(s.frame(data))
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp, path)
	}
	if werr != nil {
		_ = os.Remove(tmp)
	}
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
