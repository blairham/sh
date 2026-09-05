// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/secret"
)

// IndexName is the file every session appends its records to.
const IndexName = "index.jsonl"

// bodyDir is where a block's output is kept, sharded by date underneath.
//
// By date rather than by a prefix of the id, and the reason is retention:
// nothing in the shell ever deletes a record, so a person's own `rm -rf
// body/2026/08` has to be the answer, and a date is the only sharding somebody
// can act on. Sharding on the id would be uniform and useless — the front of
// an id is a timestamp, so every block in an eighteen-minute window shares its
// first eight characters.
const bodyDir = "body"

// A Store is one session's view of the block store.
//
// The zero value is a store that is turned off: every method succeeds and
// writes nothing, which is what a session with no home, an empty
// SH_BLOCKS_DIR, or an empty HISTFILE has. That is a state to arrive at
// deliberately and often, so it costs a nil check rather than an error path at
// every call site.
type Store struct {
	// dir is the store's root. Empty is the off state.
	dir string
	// bound is the session's gate and event sink, because this store is inside
	// the boundary rather than beside it: SH_BLOCKS_DIR is a shell variable,
	// so a line typed at the prompt chooses the path, and an open a script can
	// aim is an open a policy is entitled to refuse.
	bound boundary.Boundary
	// session identifies the shell that wrote a record. Nothing in the event
	// stream carries one, and an append-only file that several shells write to
	// is unreadable without it.
	session string

	// mu guards the index handle and everything about writing a line to it. A
	// Store is reached from the prompt loop and, once output capture exists,
	// from whatever closes a block, so the file handle is shared state.
	mu sync.Mutex
	// index is opened on the first append rather than at session start. A
	// shell that is opened and closed without running anything leaves no file
	// behind, which is what makes turning the feature on cost nothing to
	// somebody who never uses it.
	index  *os.File
	opened bool
}

// Open prepares a store rooted at dir.
//
// An empty dir is the off switch and gives back a zero Store, which every
// method tolerates. Nothing is created here: the directory and the index are
// made when the first record is appended.
func Open(dir string, b boundary.Boundary, session string) *Store {
	if dir == "" {
		return &Store{}
	}
	return &Store{dir: dir, bound: b, session: session}
}

// Session is the id of the shell this store is writing as.
func (s *Store) Session() string {
	if s == nil {
		return ""
	}
	return s.session
}

// Dir is the store's root, and empty when the store is off.
func (s *Store) Dir() string {
	if s == nil {
		return ""
	}
	return s.dir
}

// Append writes one record.
//
// The record is filled in with the schema version and this session's id, so a
// caller cannot get either wrong, and it is written as a single Write under
// O_APPEND: that is what lets two shells share the file without either of them
// locking it or losing the other's lines. A record longer than the
// filesystem's atomic write could in principle be torn by a concurrent writer,
// and Load skips a line that will not parse — which is the property JSON Lines
// was chosen for.
//
// A command line carrying a credential is not written at all: no record and no
// body. The credential essentially *is* the command — `export TOKEN=…` is
// nothing else — so a record of it with the value taken out recalls nothing
// anyone wanted, and the block store must not become the copy of the history
// that the history file refused to keep. This is the same rule the line file
// applies, on the same table, at the same point.
//
// Nothing is reported to the caller about that refusal, because the prompt has
// already said so at the moment the line was typed, which is where a person
// can act on it.
func (s *Store) Append(ctx context.Context, r Record) error {
	if s == nil || s.dir == "" {
		return nil
	}
	if _, found := secret.Default().Match(r.Command); found {
		return nil
	}
	r.V, r.Session = Version, s.session
	line, err := encode(r)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.indexFile(ctx)
	if f == nil {
		return err
	}
	_, err = f.Write(line)
	return err
}

// indexFile is the open index, opened on first use.
//
// Opened once per session and kept, rather than opened per record: a record is
// written after every command, and a shell that opened and closed a file each
// time would be paying for this at the prompt. The append mode is what makes
// the shared handle safe anyway — the offset is taken at the write, not at the
// open, so a second shell appending in between does not overwrite anything.
//
// A refusal is remembered as an open that produced no file, so a policy that
// hides the store is asked once rather than at every command.
func (s *Store) indexFile(ctx context.Context) (*os.File, error) {
	if s.opened {
		return s.index, nil
	}
	s.opened = true
	path := filepath.Join(s.dir, IndexName)
	if !s.bound.Open(ctx, path, true) {
		// Refused, and silently: a policy that hid the store meant for it not
		// to be written, and the sink has the refusal.
		return nil, nil
	}
	// 0700 and 0600: a block store is a record of what someone typed and what
	// it printed, which is not something to leave readable by everyone on the
	// machine. The history file already sets that bar.
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	s.index = f
	return f, nil
}

// Close releases the index.
//
// No flush and no sync: every record was written with one Write, so there is
// nothing buffered in this process to lose. A shell that is killed keeps
// everything up to the last completed command, which is the case where the
// record is most wanted and is the reason a record is written when a block
// closes rather than when the session ends.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index == nil {
		return nil
	}
	f := s.index
	s.index, s.opened = nil, false
	return f.Close()
}

// BodyPath is where a block's output goes, relative to the store.
//
// Relative because that is what the record carries: a store somebody moved,
// copied out of a backup, or mounted somewhere else still resolves.
func BodyPath(id string, t time.Time) string {
	return filepath.ToSlash(filepath.Join(bodyDir, t.UTC().Format("2006/01/02"), id+".out"))
}

// WriteBody writes a block's output and returns the path to record.
//
// The text is redacted rather than refused, which is the other half of the
// split the secret table is built for: dropping a two-hundred-kilobyte build
// log because one line of it echoed a token destroys exactly what the person
// wanted to keep. The command line is the case that gets dropped whole, and
// Append does that.
//
// An empty path comes back when the store is off or the write was refused, and
// the caller records a block with no output — which is a state it has to
// handle anyway, since a body file can be deleted after the fact.
func (s *Store) WriteBody(ctx context.Context, id string, t time.Time, text string) (string, error) {
	if s == nil || s.dir == "" || text == "" {
		return "", nil
	}
	rel := BodyPath(id, t)
	full := filepath.Join(s.dir, filepath.FromSlash(rel))
	if !s.bound.Open(ctx, full, true) {
		return "", nil
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return "", err
	}
	redacted, _ := secret.Default().Redact(text)
	if err := os.WriteFile(full, []byte(redacted), 0o600); err != nil {
		return "", err
	}
	return rel, nil
}

// Body reads back what a block printed.
//
// A missing file is not an error. Nothing in the shell deletes a body, so the
// only way one goes is that a person removed it — which the date-sharded
// layout is built to make easy — and a block whose output has been cleaned up
// is a block with no output, exactly like one recorded with capture off.
func (s *Store) Body(ctx context.Context, r Record) (string, bool) {
	if s == nil || s.dir == "" || r.Output == "" {
		return "", false
	}
	full := filepath.Join(s.dir, filepath.FromSlash(r.Output))
	if !s.bound.Open(ctx, full, false) {
		return "", false
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// Load reads the last n records, oldest first.
//
// The whole file is read and the tail is kept, which is what the line file
// already does with HISTFILESIZE and is right for the same reason: the file is
// append-only and is never rewritten, so bounding what a *reader* keeps is the
// only bounding there is. A store that had grown beyond what is comfortable to
// read is a store to run `rm` over, not one for the shell to quietly truncate.
//
// A line that will not parse is skipped rather than ending the read. That is
// the property JSON Lines was chosen for: a torn write costs one record, and a
// record from a future schema version is skipped by the caller rather than
// here — Load reports what it read and the caller decides what it understands.
func (s *Store) Load(ctx context.Context, n int) []Record {
	if s == nil || s.dir == "" || n <= 0 {
		return nil
	}
	path := filepath.Join(s.dir, IndexName)
	if !s.bound.Open(ctx, path, false) {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		// A store nothing has written to yet is the first session anyone runs,
		// and complaining would be the first thing they saw.
		return nil
	}
	defer func() { _ = f.Close() }()

	var recs []Record
	sc := bufio.NewScanner(f)
	// A record holds a command line, and a pasted command can be very long.
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue
		}
		recs = append(recs, r)
	}
	if len(recs) > n {
		recs = recs[len(recs)-n:]
	}
	return recs
}

// ErrNoSuchBlock is what Find reports when nothing matches.
var ErrNoSuchBlock = errors.New("no such block")

// Find resolves the way a person names a block: an id, or how far back it is.
//
// Two forms rather than one, because the obvious third does not work here.
// Prefix matching — natural for a content-addressed id — is useless when the
// front of an id is a timestamp: an eight-character prefix is shared by every
// block in an eighteen-minute window. So a *recency number* is the short form,
// `1` being the most recent, and it is told from an id by length, which means
// the two coexist without a flag to say which was meant.
//
// The search reads at most limit records, which bounds what naming a block
// costs on a store that has been accumulating for a year.
func (s *Store) Find(ctx context.Context, name string, limit int) (Record, error) {
	recs := s.Load(ctx, limit)
	if n, ok := recency(name); ok {
		if n <= 0 || n > len(recs) {
			return Record{}, ErrNoSuchBlock
		}
		return recs[len(recs)-n], nil
	}
	for i := len(recs) - 1; i >= 0; i-- {
		if recs[i].ID == name {
			return recs[i], nil
		}
	}
	return Record{}, ErrNoSuchBlock
}

// recency reads the "how far back" form: a run of decimal digits, shorter than
// an id.
//
// The length test is the whole of the disambiguation and it is not decoration.
// base32hex's alphabet begins with the ten digits, so an id *can* be all
// digits — vanishingly unlikely and not impossible — and a rule that read only
// the characters would one day resolve a real id as the twelfth-most-recent
// block. An id is always exactly idLength characters and a recency number is
// never that long, so the two sets do not meet.
func recency(name string) (int, bool) {
	if name == "" || len(name) >= idLength {
		return 0, false
	}
	n := 0
	for _, r := range name {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
		if n > 1<<30 {
			// Further digits cannot name a block and would overflow eventually.
			return 1 << 30, true
		}
	}
	return n, true
}
