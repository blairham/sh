// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package parsecache

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/blairham/sh/internal/treecodec"
	"github.com/blairham/sh/syntax"
)

// Cache is where a Reader finds and keeps its entries: a Store, or anything
// with the same two methods. interp.ParseCache is the same interface, and is
// what a runner hands in.
type Cache interface {
	// Load returns exactly what was last stored under name, or nil.
	Load(name string) []byte
	// Create starts a new entry under name, to be written as it is made.
	// done(true) puts it in place of whatever was there, and done(false)
	// throws it away; a nil w is a cache that will not take one now.
	Create(name string) (w io.Writer, done func(keep bool))
}

// Budget is a cache that limits how much a process records; a Store is one.
// See DefaultBudget.
type Budget interface {
	// Spend asks to record a text of size bytes.
	Spend(size int) bool
	// Credit is a text of size bytes read out of the cache.
	Credit(size int)
}

// AliasKind is which of the three alias tables a parser asks.
type AliasKind byte

// The three tables, in syntax.Parser's field order.
const (
	AliasPlain AliasKind = iota
	AliasGlobal
	AliasSuffix
	aliasKinds
)

// Shell is the running shell, answering the alias questions a parser asks
// exactly as the parser's own alias fields would.
//
// A read is not recorded with each word it asked about — a start asks about
// a hundred thousand of them — but with a fingerprint of the table that
// answered: equal tables give every word the same answer.
type Shell interface {
	// Alias is the answer to one question.
	Alias(kind AliasKind, name string) (string, bool)
	// AliasTable is a fingerprint of everything an answer of kind depends
	// on now.
	AliasTable(kind AliasKind) uint64
	// AliasSnapshot answers kind's questions from the table as it is now,
	// whatever happens to it afterwards.
	AliasSnapshot(kind AliasKind) func(name string) (string, bool)
}

// MinText is the shortest text worth caching. Below it the entry costs more
// than the parse it saves: a read of the entry is a file opened and read
// whatever its size, about 40 µs on this machine, against roughly 17 µs a
// kilobyte saved by decoding rather than parsing — and writing one costs a
// file created and renamed, about 200 µs.
const MinText = 4096

// NewReader is the reader for the text src of the file at path, read in
// dialect d — the dialect a syntax.Parser for it would be built with — with
// sh answering the parser's questions. A nil sh is a parser with no alias
// tables at all, as a parse that expands none is built. path names the entry and nothing more:
// what an entry is checked against is the text.
func NewReader(cache Cache, path, src string, d syntax.Dialect, sh Shell) *Reader {
	return &Reader{sh: sh, cache: cache, path: path, src: src, d0: d, cur: d}
}

// Served and Recorded count, for this process, the reads a Reader answered
// from the cache and the reads it recorded into it. They exist for the tests
// and instruments that have to tell a tree the cache handed out from one the
// parser made — which no output can, by design.
var Served, Recorded atomic.Int64

// The modes an entry is written in, which are two different sequences of
// reads of the same text and are cached apart.
const (
	readWhole = 1
	readLines = 2
)

// Reader answers a file's reads from the cache while what the cache
// recorded still holds, and from a parser — which it records — once it does
// not. It has the methods of syntax.Parser that a shell reading a file uses.
//
// # What an entry records, and why it is enough
//
// A parser is a function of its text, of the grammar it reads in, and of the
// answers it gets to the questions it asks the shell while it reads: whether
// a word is an alias and what it expands to, and, for a byte past ASCII, how
// the locale reads it. Nothing else reaches it. So an entry holds, for each
// read the shell made — each line, or the whole text — the questions that
// read asked and the answers it got, the remarks it made, and the tree it
// returned; and before a stored tree is handed out, every one of its
// questions is asked again of the shell as it stands now. Same answers, same
// tree. The grammar is checked the same way: by a fingerprint of the dialect
// at the first read, and again each time the shell replaced it.
//
// # When an answer has moved
//
// The text has been run up to that line already, so the reader cannot start
// over. It builds a parser and walks it through the lines already handed
// out, feeding it the answers those lines recorded — which are the answers a
// parser reading alongside would have been given, since each was checked
// before its tree was used — and then reads on live, recording as it goes,
// and the entry is written again with the new lines.
//
// A text that ran out partway (a `return` in the middle of a file) leaves an
// entry that ends there; a later run that reads further reads on live and
// extends it. A text with a parse failure, or a tree the codec will not
// carry, is marked as such and parsed every time.
type Reader struct {
	sh    Shell
	cache Cache
	path  string
	src   string
	d0    syntax.Dialect

	started bool
	mode    byte
	name    string
	sum     [32]byte
	summed  bool

	// cur is the dialect the next read is in, with the shell's own function
	// answers; set says SetDialect was called since the last read.
	cur syntax.Dialect
	set bool

	// The stored entry and how far into it the reads have got.
	old    *parseEntry
	served int
	// sets is the dialect handed in before each served read that had one,
	// by the read's number, kept for the walk a fallback makes.
	sets map[int]syntax.Dialect

	// The live parser, once there is one, and the questions the read in
	// progress is asking it.
	p   *syntax.Parser
	rec *readDeps
	// replaying, during the walk, is the recorded answers of the read the
	// parser is being walked through.
	replaying *readDeps
	remarks   int
	// rem is the remarks the served reads made, in order: what a parser
	// that had read them would hold.
	rem []syntax.Remark
	kit *kit
	enc *treecodec.Encoder
	dec *treecodec.Decoder
	// snaps holds the alias tables as they stood when served reads were
	// checked, by fingerprint, for the walk a fallback makes.
	snaps map[snapKey]func(string) (string, bool)

	// servedRaw holds the stored reads handed out, as the entry's bytes; w
	// is the next entry, written out a read at a time once a read goes live,
	// and done what puts it in place. scratch is where one read is put
	// together on its way out.
	servedRaw [][]byte
	w         io.Writer
	done      func(keep bool)
	scratch   []byte
	rbuf      []byte

	dirty    bool
	stopped  bool // a read the entry cannot hold: record no more
	failed   bool // and that read was a failure: mark the text as one
	finished bool // the whole-text read or the last line has been handed out
	plain    bool // the parser asks the shell directly: nothing is being kept
}

// begin loads the entry for this text in mode, on the first read.
func (c *Reader) begin(mode byte) {
	c.started, c.mode = true, mode
	fp, ok := treecodec.DialectFingerprint(&c.d0)
	if !ok {
		c.stopped = true
		return
	}
	// Named by the state the text is read from as well as by the text's
	// path, so that one file read twice from two different states — a theme
	// file sourced before the aliases are defined and again after — keeps an
	// entry for each rather than one entry rewritten twice a start.
	h := sha256.New()
	var b []byte
	b = append(b, entryVersion, mode)
	b = binary.LittleEndian.AppendUint64(b, fp)
	for k := range aliasKinds {
		var sum uint64
		if c.sh != nil {
			sum = c.sh.AliasTable(k)
		}
		b = binary.LittleEndian.AppendUint64(b, sum)
	}
	_, _ = h.Write(b)
	_, _ = h.Write([]byte(c.path))
	c.name = hex.EncodeToString(h.Sum(nil)[:20])
	data := c.cache.Load(c.name)
	if data == nil {
		return
	}
	e, err := decodeEntry(data)
	if err != nil || e.mode != mode || e.dialect != fp || e.sum != c.textSum() {
		return
	}
	switch {
	case e.flags&entryFailed != 0:
		// Parsed every time, and not recorded: the entry already says
		// everything there is to say about this text.
		c.stopped = true
		return
	}
	c.old = e
	// Every tree the entry holds, counted up front, so that the decoder
	// takes each type's nodes for the whole file out of one block.
	dec := c.decoder(nil)
	for i := range e.raw {
		ch, err := e.chunk(i)
		if err != nil {
			break
		}
		for _, part := range [][]byte{ch.remarks, ch.tree} {
			if len(part) > 0 {
				if needs, err := treecodec.Needs(part); err == nil {
					dec.Reserve(needs)
				}
			}
		}
	}
}

// textSum is the SHA-256 of the text, worked out the first time it is
// wanted: a text with no entry and nothing to record never needs it.
func (c *Reader) textSum() [32]byte {
	if !c.summed {
		c.sum, c.summed = sha256.Sum256(unsafe.Slice(unsafe.StringData(c.src), len(c.src))), true
	}
	return c.sum
}

// SetDialect is the shell replacing its grammar between two reads.
func (c *Reader) SetDialect(d syntax.Dialect) {
	c.cur, c.set = d, true
	if c.p != nil {
		c.p.SetDialect(c.wrap(d))
	}
}

// Parse reads the whole text at once.
func (c *Reader) Parse() *syntax.File {
	if !c.started {
		c.begin(readWhole)
	}
	set := c.set
	c.set = false
	if c.p == nil && c.old != nil && len(c.old.raw) == 1 {
		if f, ok := c.replay(0, set); ok {
			c.finished = true
			return f
		}
	}
	c.goLive(set)
	f := c.p.Parse()
	c.finished = true
	c.recordRead(f, true, set)
	return f
}

// NextLine reads the next logical line.
func (c *Reader) NextLine() (*syntax.File, bool) {
	if !c.started {
		c.begin(readLines)
	}
	if c.mode == readWhole && c.finished {
		// After a whole-text read there is nothing left to read, and a
		// parser asked again says so; it is not a read of its own.
		if c.p != nil {
			return c.p.NextLine()
		}
		return nil, false
	}
	set := c.set
	c.set = false
	if c.p == nil && c.old != nil && c.served < len(c.old.raw) {
		if f, ok := c.replay(c.served, set); ok {
			if f == nil {
				c.finished = true
				return nil, false
			}
			return f, true
		}
	}
	c.goLive(set)
	f, ok := c.p.NextLine()
	if !ok {
		c.finished = true
	}
	c.recordRead(f, ok, set)
	return f, ok
}

// Err is the parser's, and nil while nothing has been parsed: a stored read
// is only ever one that parsed.
func (c *Reader) Err() error {
	if c.p == nil {
		return nil
	}
	return c.p.Err()
}

// Remarks is the parser's, or while the reads are served from the entry, the
// remarks those reads recorded — the same list a parser would hold.
func (c *Reader) Remarks() []syntax.Remark {
	if c.p == nil {
		return c.rem
	}
	return c.p.Remarks()
}

// replay hands out the stored read i if every question it asked is answered
// the same way now. A nil tree with ok is the end of the text.
func (c *Reader) replay(i int, set bool) (*syntax.File, bool) {
	ch, err := c.old.chunk(i)
	if err != nil || ch.set != set {
		return nil, false
	}
	if set {
		fp, ok := treecodec.DialectFingerprint(&c.cur)
		if !ok || fp != ch.dialect {
			return nil, false
		}
	}
	if !c.holds(ch.deps, true) {
		return nil, false
	}
	var made []syntax.Remark
	if len(ch.remarks) > 0 {
		made, err = c.decoder(ch.remarks).Remarks()
		if err != nil {
			return nil, false
		}
	}
	var f *syntax.File
	if !ch.end {
		f, err = c.decoder(ch.tree).File()
		if err != nil {
			return nil, false
		}
	}
	c.rem = append(c.rem, made...)
	if set {
		if c.sets == nil {
			c.sets = map[int]syntax.Dialect{}
		}
		c.sets[c.served] = c.cur
	}
	c.servedRaw = append(c.servedRaw, ch.withLen)
	c.served++
	Served.Add(1)
	return f, true
}

// goLive builds the parser, if there is none yet, and walks it through the
// reads already handed out from the entry.
//
// set says the shell replaced the dialect before this read. A parser that
// was already live was told at the time; one built now is told here, which
// is the same single call a parser reading all along would have had.
func (c *Reader) goLive(set bool) {
	if c.p != nil {
		return
	}
	if !c.stopped {
		c.startEntry()
	}
	switch {
	case c.served > 0:
		c.walkTo(c.served)
	case c.stopped:
		// Nothing of this text will be kept, so the parser is the one the
		// caller would have built, asking the shell directly.
		c.plain = true
		c.p = syntax.NewParser(c.src, c.d0)
		sh := c.sh
		if sh == nil {
			break
		}
		c.p.Aliases = func(name string) (string, bool) { return sh.Alias(AliasPlain, name) }
		c.p.GlobalAliases = func(name string) (string, bool) { return sh.Alias(AliasGlobal, name) }
		c.p.SuffixAliases = func(name string) (string, bool) { return sh.Alias(AliasSuffix, name) }
	default:
		// Built under the first read's record: the parser reads its first
		// token as it is built, and that can ask the locale a question.
		c.rec = &readDeps{}
		c.p = syntax.NewParser(c.src, c.wrap(c.d0))
		c.installAliases()
	}
	if set {
		c.p.SetDialect(c.wrap(c.cur))
	}
	// The remarks already handed out: those the served reads recorded,
	// which the walk has just made again. A parser built fresh has made none
	// that anybody has seen — its first token, read as it was built, may
	// have made one, and that one belongs to the first read.
	c.remarks = len(c.rem)
}

// walkTo builds the parser and has it read the first n lines again, given
// the answers those lines recorded, so that it stands where a parser that
// had read them all along would stand.
//
// Each of those lines' answers was checked against the shell before its tree
// was handed out, so the answers fed here are the ones a parser reading
// alongside would have been given at the same moments.
func (c *Reader) walkTo(n int) {
	deps := make([]readDeps, n)
	for i := range n {
		ch, err := c.old.chunk(i)
		if err == nil {
			deps[i], err = c.decodeDeps(ch.deps)
		}
		if err != nil {
			// Not reachable: the same bytes were read once already, to check
			// them. If it ever were, the parser below would stand in the wrong
			// place, so the only safe thing is to say so loudly.
			panic("parse cache: an entry changed while it was being read")
		}
	}
	c.rec = nil
	c.replaying = &deps[0]
	c.p = syntax.NewParser(c.src, c.wrap(c.d0))
	c.installAliases()
	for i := range n {
		c.replaying = &deps[i]
		if c.chunkSet(i) {
			c.p.SetDialect(c.wrap(c.sets[i]))
		}
		c.p.NextLine()
	}
	c.replaying = nil
	c.rec = &readDeps{}
}

// chunkSet is whether served read i had the dialect replaced before it.
func (c *Reader) chunkSet(i int) bool {
	return len(c.servedRaw[i]) > 4 && c.servedRaw[i][4]&1 != 0
}

func (c *Reader) installAliases() {
	if c.sh == nil {
		return
	}
	c.p.Aliases = func(name string) (string, bool) { return c.alias(AliasPlain, name) }
	c.p.GlobalAliases = func(name string) (string, bool) { return c.alias(AliasGlobal, name) }
	c.p.SuffixAliases = func(name string) (string, bool) { return c.alias(AliasSuffix, name) }
}

// recordRead keeps what a live read returned, for the next entry. It runs
// before the caller runs any of the tree, so what is kept is the tree as the
// parser made it.
func (c *Reader) recordRead(f *syntax.File, ok, set bool) {
	if c.stopped {
		c.rec = nil
		return
	}
	rec := c.rec
	c.rec = &readDeps{}
	all := c.p.Remarks()
	made := all[min(c.remarks, len(all)):]
	c.remarks = len(all)
	if c.p.Err() != nil || (ok && f.Refused != nil) {
		c.abandon(true)
		return
	}
	var fp uint64
	if set {
		var fok bool
		if fp, fok = treecodec.DialectFingerprint(&c.cur); !fok {
			c.abandon(false)
			return
		}
	}
	if c.w == nil {
		return
	}
	c.recBufs()
	b := append(c.scratch[:0], 0, 0, 0, 0)
	b = appendChunkHead(b, set, !ok, fp)
	b = c.appendDeps(b, rec)
	c.rbuf = c.rbuf[:0]
	var err error
	if len(made) > 0 {
		if c.rbuf, err = c.enc.AppendRemarks(c.rbuf, made); err != nil {
			c.abandon(false)
			return
		}
	}
	b = binary.AppendUvarint(b, uint64(len(c.rbuf)))
	b = append(b, c.rbuf...)
	if ok {
		if b, err = c.enc.AppendFile(b, f); err != nil {
			c.scratch = b
			c.abandon(true)
			return
		}
	}
	binary.LittleEndian.PutUint32(b, uint32(len(b)-4))
	c.scratch = b
	c.write(b)
	c.dirty = true
	Recorded.Add(1)
}

// startEntry begins the next entry: its head, and then the reads already
// served, as the stored entry's own bytes.
//
// It is written out as it is made rather than held, because a start that
// fills the cache is a start that also parses everything, and every byte it
// holds on the way to the disk is allocation it pays on top. Measured on the
// real configuration: an interactive session there allocates within a few
// megabytes of its second collection, and a cache that kept its entries in
// memory until the end tipped a cold start into it — about 60 ms of CPU for
// a collection, against a few milliseconds of encoding.
func (c *Reader) startEntry() {
	if b, ok := c.cache.(Budget); ok && !b.Spend(len(c.src)) {
		// Put off to a later start; see DefaultBudget.
		c.stopped = true
		return
	}
	c.w, c.done = c.cache.Create(c.name)
	if c.w == nil {
		c.stopped = true
		return
	}
	fp, _ := treecodec.DialectFingerprint(&c.d0)
	e := parseEntry{mode: c.mode, sum: c.textSum(), dialect: fp}
	c.recBufs()
	c.write(e.appendHead(c.scratch[:0]))
	for _, raw := range c.servedRaw {
		c.write(raw)
	}
}

func (c *Reader) write(b []byte) {
	if c.w == nil {
		return
	}
	if _, err := c.w.Write(b); err != nil {
		c.done(false)
		c.w, c.done, c.stopped = nil, nil, true
	}
}

// abandon gives up the entry being written. failed marks the text as one
// whose reads are not kept at all, which is a small entry of its own.
func (c *Reader) abandon(failed bool) {
	if c.w != nil {
		c.done(false)
		c.w, c.done = nil, nil
	}
	c.stopped = true
	if failed {
		c.failed, c.dirty = true, true
	}
}

// decoder is the reader's one decoder, pointed at data.
func (c *Reader) decoder(data []byte) *treecodec.Decoder {
	if c.dec == nil {
		c.dec = treecodec.NewDecoder(data, c.src)
	} else {
		c.dec.Reset(data)
	}
	return c.dec
}

// recBufs takes the encoder and the buffers a recording read writes through.
func (c *Reader) recBufs() {
	if c.enc != nil {
		return
	}
	k, ok := kits.Get().(*kit)
	if !ok {
		k = &kit{enc: treecodec.NewEncoder("")}
	}
	c.kit, c.enc, c.scratch, c.rbuf = k, k.enc, k.scratch, k.rbuf
	c.enc.SetSource(c.src)
}

// kit is what recording reads write through, kept between files: its
// buffers grow to the largest line written, and a fresh kit per file grew
// them again every time.
type kit struct {
	enc           *treecodec.Encoder
	scratch, rbuf []byte
}

var kits = sync.Pool{New: func() any { return &kit{enc: treecodec.NewEncoder("")} }}

// Finish writes the entry, where this run learned something the stored one
// did not hold. It is called once, after the last read.
func (c *Reader) Finish() {
	if c.p == nil && c.served > 0 {
		// Every read was served: the parse this text would have cost was not
		// paid, and that pays for recording more.
		if b, ok := c.cache.(Budget); ok {
			b.Credit(len(c.src))
		}
	}
	if c.enc != nil {
		c.enc.SetSource("")
		c.kit.scratch, c.kit.rbuf = c.scratch[:0], c.rbuf[:0]
		kits.Put(c.kit)
		c.kit, c.enc, c.scratch, c.rbuf = nil, nil, nil, nil
	}
	if !c.started {
		return
	}
	if c.failed {
		// A text that does not parse, or a tree the codec will not carry:
		// an entry that says so and holds no reads, so the next start does
		// not record it again.
		c.mark(entryFailed)
		return
	}
	if c.w != nil {
		c.done(c.dirty)
		c.w, c.done = nil, nil
	}
}

// mark writes an entry holding no reads and saying why.
func (c *Reader) mark(flags byte) {
	fp, ok := treecodec.DialectFingerprint(&c.d0)
	if !ok {
		return
	}
	if w, done := c.cache.Create(c.name); w != nil {
		e := parseEntry{mode: c.mode, sum: c.textSum(), dialect: fp, flags: flags}
		_, err := w.Write(e.appendHead(nil))
		done(err == nil)
	}
}

// alias asks the shell and notes which table answered, or — while a parser
// is being walked through lines already handed out — answers from the table
// as it stood when that line was checked.
func (c *Reader) alias(kind AliasKind, name string) (string, bool) {
	if d := c.replaying; d != nil && d.asked[kind] {
		if f, ok := c.snaps[snapKey{kind, d.table[kind]}]; ok {
			return f(name)
		}
	}
	if d := c.rec; d != nil && !d.asked[kind] {
		d.asked[kind] = true
		d.table[kind] = c.sh.AliasTable(kind)
	}
	return c.sh.Alias(kind, name)
}

type snapKey struct {
	kind AliasKind
	sum  uint64
}

// wrap is d with each function answer it carries routed through this reader,
// so that every question is recorded or answered from a record. A nil answer
// stays nil: the reader takes nil as an answer of its own, and the dialect's
// fingerprint holds which are nil.
func (c *Reader) wrap(d syntax.Dialect) syntax.Dialect {
	if c.plain {
		return d
	}
	if d.CharacterWidth != nil {
		d.CharacterWidth = c.characterWidth
	}
	if d.NameTakesALetterPastASCII != nil {
		d.NameTakesALetterPastASCII = c.nameLetter
	}
	if d.CharacterLength != nil {
		d.CharacterLength = c.characterLength
	}
	if d.HeredocDelimiterValue != nil {
		d.HeredocDelimiterValue = c.heredocValue
	}
	return d
}

func (c *Reader) characterWidth(b, next byte) int {
	k := [2]byte{b, next}
	if c.replaying != nil {
		if n, ok := c.replaying.width[k]; ok {
			return n
		}
	}
	n := c.cur.CharacterWidth(b, next)
	if c.rec != nil {
		if c.rec.width == nil {
			c.rec.width = map[[2]byte]int{}
		}
		c.rec.width[k] = n
	}
	return n
}

func (c *Reader) nameLetter(ch rune) bool {
	if c.replaying != nil {
		if v, ok := c.replaying.letter[ch]; ok {
			return v
		}
	}
	v := c.cur.NameTakesALetterPastASCII(ch)
	if c.rec != nil {
		if c.rec.letter == nil {
			c.rec.letter = map[rune]bool{}
		}
		c.rec.letter[ch] = v
	}
	return v
}

func (c *Reader) characterLength(s string) int {
	if c.replaying != nil {
		if n, ok := c.replaying.length[s]; ok {
			return n
		}
	}
	n := c.cur.CharacterLength(s)
	if c.rec != nil {
		if c.rec.length == nil {
			c.rec.length = map[string]int{}
		}
		c.rec.length[s] = n
	}
	return n
}

func (c *Reader) heredocValue(s string) string {
	if c.replaying != nil {
		if v, ok := c.replaying.heredoc[s]; ok {
			return v
		}
	}
	v := c.cur.HeredocDelimiterValue(s)
	if c.rec != nil {
		if c.rec.heredoc == nil {
			c.rec.heredoc = map[string]string{}
		}
		c.rec.heredoc[s] = v
	}
	return v
}

// readDeps is every question one read asked the shell, with its answer.
type readDeps struct {
	// asked and table are, per alias table, whether the read asked it
	// anything and the table's fingerprint when it did.
	asked   [aliasKinds]bool
	table   [aliasKinds]uint64
	width   map[[2]byte]int
	letter  map[rune]bool
	length  map[string]int
	heredoc map[string]string
}

// The questions are written as: a byte saying which alias tables were asked
// and each one's fingerprint, then each kind of locale question as a count
// and its entries. A string the parser asked about is nearly always a slice
// of the text, and is written as where it is in the text rather than spelled
// out, which keeps entries small and lets the check read it back without
// making a string.
func (c *Reader) appendDeps(b []byte, d *readDeps) []byte {
	var flags byte
	for k := range aliasKinds {
		if d.asked[k] {
			flags |= 1 << k
		}
	}
	b = append(b, flags)
	for k := range aliasKinds {
		if d.asked[k] {
			b = binary.LittleEndian.AppendUint64(b, d.table[k])
		}
	}
	b = binary.AppendUvarint(b, uint64(len(d.width)))
	for k, n := range d.width {
		b = append(b, k[0], k[1])
		b = binary.AppendUvarint(b, uint64(n))
	}
	b = binary.AppendUvarint(b, uint64(len(d.letter)))
	for ch, v := range d.letter {
		b = binary.AppendUvarint(b, uint64(ch))
		b = appendBool(b, v)
	}
	b = binary.AppendUvarint(b, uint64(len(d.length)))
	for s, n := range d.length {
		b = c.appendText(b, s)
		b = binary.AppendUvarint(b, uint64(n))
	}
	b = binary.AppendUvarint(b, uint64(len(d.heredoc)))
	for s, v := range d.heredoc {
		b = c.appendText(b, s)
		b = appendString(b, v)
	}
	return b
}

// appendText writes s as an offset into the text where it is a slice of it,
// and spelled out where it is not.
func (c *Reader) appendText(b []byte, s string) []byte {
	if len(s) > 0 && len(c.src) > 0 {
		lo := uintptr(unsafe.Pointer(unsafe.StringData(c.src)))
		at := uintptr(unsafe.Pointer(unsafe.StringData(s)))
		if at >= lo && at+uintptr(len(s)) <= lo+uintptr(len(c.src)) {
			b = append(b, 0)
			b = binary.AppendUvarint(b, uint64(len(s)))
			return binary.AppendUvarint(b, uint64(at-lo))
		}
	}
	b = append(b, 1)
	return appendString(b, s)
}

func appendBool(b []byte, v bool) []byte {
	if v {
		return append(b, 1)
	}
	return append(b, 0)
}

// holds asks every question in a read's record again, of the shell as it
// stands for this read, straight off the bytes. keep says to hold on to the
// alias tables that answered, for a walk that may come later.
func (c *Reader) holds(deps []byte, keep bool) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	r := entryReader{b: deps, src: c.src}
	flags := r.byte()
	for k := range aliasKinds {
		if flags&(1<<k) == 0 {
			continue
		}
		sum := r.uint64()
		if c.sh.AliasTable(k) != sum {
			return false
		}
		if keep {
			if _, ok := c.snaps[snapKey{k, sum}]; !ok {
				if c.snaps == nil {
					c.snaps = map[snapKey]func(string) (string, bool){}
				}
				c.snaps[snapKey{k, sum}] = c.sh.AliasSnapshot(k)
			}
		}
	}
	if n := r.uvarint(); n > 0 {
		if c.cur.CharacterWidth == nil {
			return false
		}
		for ; n > 0; n-- {
			b0, b1 := r.byte(), r.byte()
			if c.cur.CharacterWidth(b0, b1) != int(r.uvarint()) {
				return false
			}
		}
	}
	if n := r.uvarint(); n > 0 {
		if c.cur.NameTakesALetterPastASCII == nil {
			return false
		}
		for ; n > 0; n-- {
			ch := rune(r.uvarint())
			if c.cur.NameTakesALetterPastASCII(ch) != (r.byte() == 1) {
				return false
			}
		}
	}
	if n := r.uvarint(); n > 0 {
		if c.cur.CharacterLength == nil {
			return false
		}
		for ; n > 0; n-- {
			s := r.text()
			if c.cur.CharacterLength(s) != int(r.uvarint()) {
				return false
			}
		}
	}
	if n := r.uvarint(); n > 0 {
		if c.cur.HeredocDelimiterValue == nil {
			return false
		}
		for ; n > 0; n-- {
			s := r.text()
			if !r.sameString(c.cur.HeredocDelimiterValue(s)) {
				return false
			}
		}
	}
	return r.off == len(r.b)
}

// decodeDeps is a read's record as maps, for the walk.
func (c *Reader) decodeDeps(deps []byte) (d readDeps, err error) {
	defer func() {
		if recover() != nil {
			d, err = readDeps{}, errEntry
		}
	}()
	r := entryReader{b: deps, src: c.src}
	flags := r.byte()
	for k := range aliasKinds {
		if flags&(1<<k) != 0 {
			d.asked[k] = true
			d.table[k] = r.uint64()
		}
	}
	if n := r.uvarint(); n > 0 {
		d.width = make(map[[2]byte]int, n)
		for ; n > 0; n-- {
			b0, b1 := r.byte(), r.byte()
			d.width[[2]byte{b0, b1}] = int(r.uvarint())
		}
	}
	if n := r.uvarint(); n > 0 {
		d.letter = make(map[rune]bool, n)
		for ; n > 0; n-- {
			ch := rune(r.uvarint())
			d.letter[ch] = r.byte() == 1
		}
	}
	if n := r.uvarint(); n > 0 {
		d.length = make(map[string]int, n)
		for ; n > 0; n-- {
			s := r.text()
			d.length[s] = int(r.uvarint())
		}
	}
	if n := r.uvarint(); n > 0 {
		d.heredoc = make(map[string]string, n)
		for ; n > 0; n-- {
			s := r.text()
			d.heredoc[s] = r.string()
		}
	}
	return d, nil
}

// The entry, as this package lays it out. The store wraps it in a frame of
// its own — a checksum and the build that wrote it — and hands back only
// what it framed.
//
//	magic, version, codec layout, mode
//	the text's SHA-256 and the dialect's fingerprint at the first read
//	flags: failed
//	the reads, to the end: each as a four-byte length and its bytes
//
// A read is its flags (the dialect was replaced before it; it found the end
// of the text), the replacement dialect's fingerprint where there was one,
// its questions as a length and their bytes, its remarks as a length and
// theirs, and its tree.
const (
	entryMagic   = "SHPC"
	entryVersion = 1
)

type parseEntry struct {
	mode    byte
	sum     [32]byte
	dialect uint64
	flags   byte
	raw     [][]byte
}

// The flags of an entry that holds no reads.
const (
	entryFailed = 1 << 0 // the text does not parse, or holds a tree the codec refuses
)

type chunk struct {
	set     bool
	end     bool
	dialect uint64
	deps    []byte
	remarks []byte
	tree    []byte
	withLen []byte // the read as stored, its length in front
}

var errEntry = errors.New("parse cache: not an entry")

// entryHead is the fixed part of an entry, ahead of its reads.
const entryHead = len(entryMagic) + 1 + 8 + 1 + 32 + 8 + 1

// appendHead appends the fixed part of an entry.
func (e *parseEntry) appendHead(b []byte) []byte {
	layout, _ := treecodec.Layout()
	b = append(b, entryMagic...)
	b = append(b, entryVersion)
	b = binary.LittleEndian.AppendUint64(b, layout)
	b = append(b, e.mode)
	b = append(b, e.sum[:]...)
	b = binary.LittleEndian.AppendUint64(b, e.dialect)
	return append(b, e.flags)
}

func decodeEntry(b []byte) (*parseEntry, error) {
	layout, err := treecodec.Layout()
	if err != nil {
		return nil, err
	}
	if len(b) < entryHead || !bytes.Equal(b[:4], []byte(entryMagic)) || b[4] != entryVersion ||
		binary.LittleEndian.Uint64(b[5:]) != layout {
		return nil, errEntry
	}
	e := &parseEntry{mode: b[13]}
	copy(e.sum[:], b[14:46])
	e.dialect = binary.LittleEndian.Uint64(b[46:])
	e.flags = b[54]
	b = b[entryHead:]
	if e.flags != 0 && len(b) != 0 {
		return nil, errEntry
	}
	e.raw = make([][]byte, 0, 64)
	for len(b) > 0 {
		if len(b) < 4 {
			return nil, errEntry
		}
		l := binary.LittleEndian.Uint32(b)
		if uint64(l) > uint64(len(b)-4) {
			return nil, errEntry
		}
		e.raw = append(e.raw, b[:4+l])
		b = b[4+l:]
	}
	if len(b) != 0 {
		return nil, errEntry
	}
	return e, nil
}

func appendChunkHead(b []byte, set, end bool, fp uint64) []byte {
	var flags byte
	if set {
		flags |= 1
	}
	if end {
		flags |= 2
	}
	b = append(b, flags)
	if set {
		b = binary.LittleEndian.AppendUint64(b, fp)
	}
	return b
}

// chunk splits read i into its parts; nothing in it is decoded yet.
func (e *parseEntry) chunk(i int) (ch chunk, err error) {
	defer func() {
		if recover() != nil {
			ch, err = chunk{}, errEntry
		}
	}()
	b := e.raw[i]
	ch.withLen = b
	b = b[4:]
	flags := b[0]
	b = b[1:]
	ch.set, ch.end = flags&1 != 0, flags&2 != 0
	if ch.set {
		ch.dialect = binary.LittleEndian.Uint64(b)
		b = b[8:]
	}
	r := entryReader{b: b}
	r.skipDeps()
	ch.deps = b[:r.off]
	n := int(r.uvarint())
	ch.remarks = r.b[r.off : r.off+n]
	r.off += n
	ch.tree = r.b[r.off:]
	if ch.end && len(ch.tree) != 0 {
		return chunk{}, errEntry
	}
	return ch, nil
}

func appendString(b []byte, s string) []byte {
	b = binary.AppendUvarint(b, uint64(len(s)))
	return append(b, s...)
}

// entryReader reads the fields an entry was written with, and panics past the
// end — which its callers turn into a damaged entry.
type entryReader struct {
	b   []byte
	off int
	src string
}

func (r *entryReader) uvarint() uint64 {
	x, n := binary.Uvarint(r.b[r.off:])
	if n <= 0 {
		panic(errEntry)
	}
	r.off += n
	return x
}

func (r *entryReader) uint64() uint64 {
	x := binary.LittleEndian.Uint64(r.b[r.off:])
	r.off += 8
	return x
}

func (r *entryReader) byte() byte {
	c := r.b[r.off]
	r.off++
	return c
}

func (r *entryReader) string() string {
	n := int(r.uvarint())
	s := string(r.b[r.off : r.off+n])
	r.off += n
	return s
}

// sameString reports whether the next string written is s, without making
// one.
func (r *entryReader) sameString(s string) bool {
	n := int(r.uvarint())
	b := r.b[r.off : r.off+n]
	r.off += n
	return string(b) == s
}

// text reads what appendText wrote: a slice of the text, which costs nothing,
// or the bytes spelled out.
func (r *entryReader) text() string {
	switch r.byte() {
	case 0:
		n := int(r.uvarint())
		at := int(r.uvarint())
		return r.src[at : at+n]
	case 1:
		return r.string()
	}
	panic(errEntry)
}

// skipDeps steps over a read's questions without asking any of them.
func (r *entryReader) skipDeps() {
	flags := r.byte()
	for k := range aliasKinds {
		if flags&(1<<k) != 0 {
			r.uint64()
		}
	}
	skipText := func() {
		if r.byte() == 0 {
			r.uvarint()
			r.uvarint()
			return
		}
		r.off += int(r.uvarint())
	}
	for n := r.uvarint(); n > 0; n-- {
		r.byte()
		r.byte()
		r.uvarint()
	}
	for n := r.uvarint(); n > 0; n-- {
		r.uvarint()
		r.byte()
	}
	for n := r.uvarint(); n > 0; n-- {
		skipText()
		r.uvarint()
	}
	for n := r.uvarint(); n > 0; n-- {
		skipText()
		r.off += int(r.uvarint())
	}
	if r.off > len(r.b) {
		panic(errEntry)
	}
}
