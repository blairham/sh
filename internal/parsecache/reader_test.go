// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package parsecache_test

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/oracle"
	"github.com/blairham/sh/internal/parsecache"
	"github.com/blairham/sh/internal/testenv"
	"github.com/blairham/sh/internal/treecodec"
	"github.com/blairham/sh/syntax"
)

// memCache is a cache in memory, counting what it was asked.
type memCache struct {
	m       map[string][]byte
	creates int
}

func newMem() *memCache { return &memCache{m: map[string][]byte{}} }

func (c *memCache) Load(name string) []byte { return c.m[name] }

func (c *memCache) Create(name string) (io.Writer, func(bool)) {
	c.creates++
	var b bytes.Buffer
	return &b, func(keep bool) {
		if keep {
			c.m[name] = bytes.Clone(b.Bytes())
		}
	}
}

// shell is a shell's alias tables, answering as interp's runner does —
// fingerprints kept until a table changes, as the runner keeps them.
type shell struct {
	tables [3]map[string]string
	sums   [3]uint64
	summed [3]bool
}

func (s *shell) Alias(k parsecache.AliasKind, name string) (string, bool) {
	v, ok := s.tables[k][name]
	return v, ok
}

func (s *shell) AliasTable(k parsecache.AliasKind) uint64 {
	if s.summed[k] {
		return s.sums[k]
	}
	h := fnv.New64a()
	names := slices.Sorted(maps.Keys(s.tables[k]))
	for _, n := range names {
		fmt.Fprintf(h, "%d:%s=%d:%s;", len(n), n, len(s.tables[k][n]), s.tables[k][n])
	}
	s.sums[k], s.summed[k] = h.Sum64(), true
	return s.sums[k]
}

func (s *shell) AliasSnapshot(k parsecache.AliasKind) func(string) (string, bool) {
	held := maps.Clone(s.tables[k])
	return func(name string) (string, bool) {
		v, ok := held[name]
		return v, ok
	}
}

func (s *shell) set(k parsecache.AliasKind, name, value string) {
	if s.tables[k] == nil {
		s.tables[k] = map[string]string{}
	}
	s.tables[k][name] = value
	s.summed[k] = false
}

// reader is what a shell reads a file through: a parser, or the cache's
// reader standing in for one.
type reader interface {
	SetDialect(syntax.Dialect)
	NextLine() (*syntax.File, bool)
	Parse() *syntax.File
	Err() error
	Remarks() []syntax.Remark
}

// plain is the parser a shell builds when it has no cache.
func plain(src string, d syntax.Dialect, sh *shell) reader {
	p := syntax.NewParser(src, d)
	p.Aliases = func(n string) (string, bool) { return sh.Alias(parsecache.AliasPlain, n) }
	p.GlobalAliases = func(n string) (string, bool) { return sh.Alias(parsecache.AliasGlobal, n) }
	p.SuffixAliases = func(n string) (string, bool) { return sh.Alias(parsecache.AliasSuffix, n) }
	return p
}

// schedule is what happens around the reads: the shell replacing its grammar
// before a read, and changing its aliases before one, as running the lines
// already read would.
type schedule struct {
	whole       bool
	setBefore   map[int]syntax.Dialect
	aliasBefore map[int]func(*shell)
}

// outcome is everything a shell sees of a text's reading.
type outcome struct {
	Reads   []*syntax.File
	Err     string
	Remarks []syntax.Remark
}

// read reads src through r the way interp's loops do.
func read(r reader, sh *shell, sch schedule) outcome {
	var o outcome
	if sch.whole {
		o.Reads = append(o.Reads, r.Parse())
		if f, ok := r.NextLine(); ok {
			o.Reads = append(o.Reads, f)
		}
	} else {
		for i := 0; ; i++ {
			if f := sch.aliasBefore[i]; f != nil {
				f(sh)
			}
			if d, ok := sch.setBefore[i]; ok {
				r.SetDialect(d)
			}
			f, ok := r.NextLine()
			if !ok {
				break
			}
			o.Reads = append(o.Reads, f)
		}
	}
	if err := r.Err(); err != nil {
		o.Err = err.Error()
	}
	o.Remarks = slices.Clone(r.Remarks())
	return o
}

// parsed is what a shell with no cache reads of src: one shell, both
// answering the parser and changed by the schedule.
func parsed(src string, d syntax.Dialect, sh *shell, sch schedule) outcome {
	return read(plain(src, d, sh), sh, sch)
}

// cached reads src through a cache reader and finishes it, reporting how
// many of its reads the cache served.
func cached(c parsecache.Cache, path, src string, d syntax.Dialect, sh *shell, sch schedule) (outcome, int64) {
	before := parsecache.Served.Load()
	r := parsecache.NewReader(c, path, src, d, sh)
	o := read(r, sh, sch)
	r.Finish()
	// Bodies the cache deferred are read now, so that the trees compare
	// field by field with the parser's (#6483).
	treecodec.Settle(o.Reads)
	return o, parsecache.Served.Load() - before
}

// fresh is a shell with no aliases, or a few: words the corpora use as
// commands, so that expansion happens in the trees being compared.
func fresh(aliases bool) *shell {
	sh := &shell{}
	if aliases {
		sh.set(parsecache.AliasPlain, "ll", "ls -l")
		sh.set(parsecache.AliasPlain, "e", "echo")
		sh.set(parsecache.AliasPlain, "x", "y=1;")
	}
	return sh
}

func dialects() map[string]syntax.Dialect {
	return map[string]syntax.Dialect{
		"core": syntax.Core(), "bash": bash.Dialect(), "zsh": zsh.Dialect(),
		"ksh": ksh.Dialect(), "dash": dash.Dialect(),
	}
}

// corpus is every text the repository holds: the oracle's cases, our own
// suite, the function library and the preludes.
func corpus(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, c := range oracle.Corpus {
		out = append(out, c.Snippet)
	}
	err := filepath.WalkDir(filepath.Join("..", "..", "share"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err == nil {
			out = append(out, string(b))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	out = append(out, bash.Prelude(), zsh.Prelude(), ksh.Prelude())
	if len(out) < 4000 {
		t.Fatalf("only %d texts: the corpora were not found", len(out))
	}
	return out
}

// equivalent reads src three ways — with no cache, through an empty cache,
// and through the cache that filled — and requires the three outcomes to be
// the same and the third to have come from the cache. It returns whether
// the text read cleanly, which is when the third must have been served.
func equivalent(t *testing.T, label, src string, d syntax.Dialect, aliases bool, sch schedule) bool {
	t.Helper()
	want := parsed(src, d, fresh(aliases), sch)
	mem := newMem()
	cold, _ := cached(mem, "/t/"+label, src, d, fresh(aliases), sch)
	creates := mem.creates
	warm, served := cached(mem, "/t/"+label, src, d, fresh(aliases), sch)
	if !reflect.DeepEqual(cold, want) {
		t.Fatalf("%s: recording %q changed what was read", label, src)
	}
	if !reflect.DeepEqual(warm, want) {
		t.Fatalf("%s: the cache gave back something other than %q", label, src)
	}
	clean := want.Err == "" && carried(src, want)
	switch {
	case !clean:
		if served != 0 {
			t.Fatalf("%s: a text that does not parse was served from the cache: %q", label, src)
		}
	case sch.whole:
		if served != 1 {
			t.Fatalf("%s: the cache did not serve the whole of %q", label, src)
		}
	default:
		// Every read, and the end of the text, which is a read of its own.
		if served != int64(len(want.Reads))+1 {
			t.Fatalf("%s: the cache served %d of %d reads of %q", label, served, len(want.Reads)+1, src)
		}
	}
	if mem.creates != creates {
		t.Fatalf("%s: a text the cache had already recorded was recorded again: %q", label, src)
	}
	return clean
}

// carried reports whether every tree of an outcome is one the codec writes:
// a tree holding a refusal it defers to run time is not, and a text with one
// is parsed every time rather than served.
func carried(src string, o outcome) bool {
	for _, f := range o.Reads {
		if f == nil || f.Refused != nil {
			return false
		}
		if err := treecodec.NewEncoder(src).File(f); err != nil {
			return false
		}
	}
	return true
}

// TestACachedReadIsTheParse is the cache's claim over everything the
// repository holds, in every grammar, read a line at a time and whole, with
// aliases and without: a read the cache serves is the read a parser makes.
//
// Under the race detector a grammar reads every raceStride-th text rather than
// all of them. The sweep runs in one goroutine, so the detector has nothing to
// compare and can find nothing in it, and it cost ten times the work: 427s of
// this package's time on CI's Linux leg against 39s for the same package on the
// uninstrumented macOS leg (#7290). That leg, `make test`, and any `go test`
// without -race still read every text in every grammar, so the claim is proved
// whole on every pull request; the instrumented run keeps a sample, offset per
// grammar, so the code under the detector is still the code the claim is about.
func TestACachedReadIsTheParse(t *testing.T) {
	texts := corpus(t)
	ds := dialects()
	clean, failing, want := 0, 0, 50000
	if testenv.RaceDetector {
		want /= raceStride
	}
	for k, name := range slices.Sorted(maps.Keys(ds)) {
		d := ds[name]
		for i, src := range texts {
			if testenv.RaceDetector && i%raceStride != k {
				continue
			}
			for _, aliases := range []bool{false, true} {
				for _, whole := range []bool{false, true} {
					label := fmt.Sprintf("%s/%d/%v/%v", name, i, aliases, whole)
					if equivalent(t, label, src, d, aliases, schedule{whole: whole}) {
						clean++
					} else {
						failing++
					}
				}
			}
		}
	}
	// Both kinds have to be there: a corpus that never failed to parse would
	// never have checked that a failure is not kept.
	if clean < want || failing == 0 {
		t.Fatalf("%d clean readings and %d that failed", clean, failing)
	}
	t.Logf("%d clean readings and %d that failed", clean, failing)
}

// raceStride is how far apart the texts are that a grammar reads under the
// race detector — see TestACachedReadIsTheParse. Wider than the number of
// grammars, so that each reads texts none of the others does.
const raceStride = 40

// TestARealPluginTree is the same claim over a tree of real plugins, which is
// what the cache is for. The tree is named by SH_TEST_PLUGIN_TREE, since it
// is one person's and has no place in this repository.
func TestARealPluginTree(t *testing.T) {
	root := os.Getenv("SH_TEST_PLUGIN_TREE")
	if root == "" {
		t.Skip("SH_TEST_PLUGIN_TREE names no plugin tree")
	}
	n := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || (!strings.HasSuffix(path, ".zsh") && !strings.HasSuffix(path, ".sh") &&
			!strings.HasSuffix(path, ".zsh-theme")) {
			return nil //nolint:nilerr // an unreadable entry is skipped; the walk goes on
		}
		b, err := os.ReadFile(path)
		if err != nil || len(b) > 1<<20 {
			return nil //nolint:nilerr // as above
		}
		for _, whole := range []bool{false, true} {
			equivalent(t, path, string(b), zsh.Dialect(), true, schedule{whole: whole})
		}
		n++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatalf("no plugin file under %s", root)
	}
	t.Logf("%d files", n)
}

// TestAnEditedTextIsReadAgain changes one byte of a text and keeps its
// length, which is the edit a stat-keyed cache cannot see: the entry must
// not be used, and the new text must read as itself.
func TestAnEditedTextIsReadAgain(t *testing.T) {
	src := padded("f() { echo one; }\nf\n")
	edited := strings.Replace(src, "one", "two", 1)
	mem := newMem()
	cached(mem, "/t/f", src, zsh.Dialect(), fresh(false), schedule{})
	got, served := cached(mem, "/t/f", edited, zsh.Dialect(), fresh(false), schedule{})
	if served != 0 {
		t.Fatalf("an edited text was served %d reads from the old one's entry", served)
	}
	want := parsed(edited, zsh.Dialect(), fresh(false), schedule{})
	if !reflect.DeepEqual(got, want) {
		t.Fatal("the edited text did not read as itself")
	}
	// The control: the same text is served.
	if _, served := cached(mem, "/t/f", edited, zsh.Dialect(), fresh(false), schedule{}); served == 0 {
		t.Fatal("the text was not served even unedited: the check above proves nothing")
	}
}

// padded puts a text after enough comment lines to be worth a cache.
func padded(body string) string {
	return strings.Repeat("# a line of commentary to make the file worth caching\n", 80) + body
}

// TestAnAliasChangeIsSeen defines the alias a later line expands differently
// from the run that recorded it: the read that asked about it must not be
// served, and what is read must be the new expansion.
func TestAnAliasChangeIsSeen(t *testing.T) {
	src := padded("greet hello\n")
	recorded := fresh(false)
	recorded.set(parsecache.AliasPlain, "greet", "echo")
	mem := newMem()
	cached(mem, "/t/a", src, zsh.Dialect(), recorded, schedule{})

	now := fresh(false)
	now.set(parsecache.AliasPlain, "greet", "printf %s")
	got, served := cached(mem, "/t/a", src, zsh.Dialect(), now, schedule{})
	if served != 0 {
		t.Fatalf("%d reads were served from an entry recorded under another alias table", served)
	}
	again := fresh(false)
	again.set(parsecache.AliasPlain, "greet", "printf %s")
	want := read(plain(src, zsh.Dialect(), again), again, schedule{})
	if !reflect.DeepEqual(got, want) {
		t.Fatal("the line did not read with the alias as it is now")
	}
}

// TestALineDefinesWhatTheNextReads is the shape a startup file has: an
// earlier line defines an alias a later line uses. Recorded that way and read
// that way it is served; read with the alias defined at another line, the
// reads before the change are served, the reader picks up from there, and
// everything reads as a parser reading alongside would have read it.
func TestALineDefinesWhatTheNextReads(t *testing.T) {
	src := padded("echo one\necho two\nhi there\necho three\nhi again\n")
	define := func(at int) schedule {
		return schedule{aliasBefore: map[int]func(*shell){at: func(s *shell) {
			s.set(parsecache.AliasPlain, "hi", "echo hi:")
		}}}
	}
	mem := newMem()
	cached(mem, "/t/l", src, zsh.Dialect(), fresh(false), define(2))
	got, served := cached(mem, "/t/l", src, zsh.Dialect(), fresh(false), define(2))
	if served != 6 {
		t.Fatalf("the same run was served %d reads of 6", served)
	}
	want := parsed(src, zsh.Dialect(), fresh(false), define(2))
	if !reflect.DeepEqual(got, want) {
		t.Fatal("the same run read differently")
	}

	// The alias now arrives a line later, so the third read asks about a
	// word the table answers differently than it did when it was recorded.
	got, served = cached(mem, "/t/l", src, zsh.Dialect(), fresh(false), define(3))
	want = parsed(src, zsh.Dialect(), fresh(false), define(3))
	if !reflect.DeepEqual(got, want) {
		t.Fatal("a different run read differently from a parser reading it")
	}
	if served == 0 || served >= 6 {
		t.Fatalf("a run that parts from the recorded one at its third read was served %d reads", served)
	}
}

// TestAWalkUsesTheAnswersItWasGiven parts from the recorded run after an
// alias that opens a quote has been read, so the reader has to put a parser
// back after that line before reading on. It must be put back with the alias
// as it was when the line was read: asked now, the alias no longer opens the
// quote, the parser stops a line short, and every line after reads wrong.
func TestAWalkUsesTheAnswersItWasGiven(t *testing.T) {
	src := padded("echo zero\nq\nrest'\necho three\necho four\n")
	open := func(s *shell) { s.set(parsecache.AliasPlain, "q", "echo '") }
	recorded := schedule{aliasBefore: map[int]func(*shell){1: open}}
	parted := schedule{aliasBefore: map[int]func(*shell){1: open, 2: func(s *shell) {
		s.set(parsecache.AliasPlain, "q", "echo plain")
	}}}
	mem := newMem()
	cached(mem, "/t/w", src, zsh.Dialect(), fresh(false), recorded)
	got, served := cached(mem, "/t/w", src, zsh.Dialect(), fresh(false), parted)
	want := parsed(src, zsh.Dialect(), fresh(false), parted)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("the reads after the walk differ from a parser's")
	}
	if served != 2 {
		t.Fatalf("%d reads were served, want the 2 before the alias changed", served)
	}
	if want.Err != "" || len(want.Reads) != 4 {
		t.Fatalf("the text did not read as four lines (%d, %q): the case tests nothing", len(want.Reads), want.Err)
	}
}

// TestAReplacedGrammarIsSeen replaces the dialect between reads, as `setopt`
// does: recorded and replayed alike it is served, and replaced differently
// it is read again — each time as a parser would have read it.
func TestAReplacedGrammarIsSeen(t *testing.T) {
	src := padded("echo one\nprint -r -- 'a''b'\necho @(x|y)\n")
	other := zsh.Dialect()
	other.DoubledQuoteInSingleQuotesIsALiteralQuote = !other.DoubledQuoteInSingleQuotesIsALiteralQuote
	at1 := schedule{setBefore: map[int]syntax.Dialect{1: other}}
	at2 := schedule{setBefore: map[int]syntax.Dialect{2: other}}
	mem := newMem()
	cached(mem, "/t/g", src, zsh.Dialect(), fresh(false), at1)
	if _, served := cached(mem, "/t/g", src, zsh.Dialect(), fresh(false), at1); served != 4 {
		t.Fatalf("the same run was served %d reads of 4", served)
	}
	third := zsh.Dialect()
	third.ExtendedPattern = !third.ExtendedPattern
	other1 := schedule{setBefore: map[int]syntax.Dialect{1: third}}
	for name, sch := range map[string]schedule{"later": at2, "never": {}, "another grammar at the same line": other1} {
		got, served := cached(mem, "/t/g", src, zsh.Dialect(), fresh(false), sch)
		want := parsed(src, zsh.Dialect(), fresh(false), sch)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: the grammar replaced differently read differently", name)
		}
		if served >= 4 {
			t.Fatalf("%s: every read was served under another grammar", name)
		}
		// Put the recorded run back for the next case.
		cached(mem, "/t/g", src, zsh.Dialect(), fresh(false), at1)
	}
	// And a different grammar from the first read names a different entry.
	if _, served := cached(mem, "/t/g", src, other, fresh(false), schedule{}); served != 0 {
		t.Fatalf("%d reads were served from an entry recorded in another grammar", served)
	}
}

// TestTheLocalesAnswersAreChecked records a text whose bytes past ASCII the
// dialect's locale reads, and reads it again under a locale that reads them
// differently.
func TestTheLocalesAnswersAreChecked(t *testing.T) {
	// Big5's U+03B1 is a3 5c: a second byte that is a backslash. Read whole,
	// it is a letter; read a byte at a time, the backslash escapes the quote
	// after it and the word runs on.
	src := padded("echo \"\xa3\x5c\" done\n")
	whole := zsh.Dialect()
	whole.CharacterWidth = func(b, next byte) int {
		if b >= 0x81 && b <= 0xfe {
			return 2
		}
		return 1
	}
	bytewise := zsh.Dialect()
	bytewise.CharacterWidth = func(byte, byte) int { return 1 }
	mem := newMem()
	cached(mem, "/t/w", src, whole, fresh(false), schedule{})
	if _, served := cached(mem, "/t/w", src, whole, fresh(false), schedule{}); served == 0 {
		t.Fatal("the same locale was not served")
	}
	got, served := cached(mem, "/t/w", src, bytewise, fresh(false), schedule{})
	want := parsed(src, bytewise, fresh(false), schedule{})
	if !reflect.DeepEqual(got, want) {
		t.Fatal("another locale read differently from a parser")
	}
	if served > 0 {
		// The first lines are ASCII and were served; the line with the
		// character in it must not have been.
		if int(served) > len(want.Reads)-1 {
			t.Fatalf("the line read under another locale was served (%d reads served)", served)
		}
	}
	if reflect.DeepEqual(parsed(src, whole, fresh(false), schedule{}), want) {
		t.Fatal("the two locales read the text alike: the case tests nothing")
	}
}

// TestRemarksComeBack records a text the parser remarks on and reads it back
// from the cache: the remarks are part of what the shell sees, and a served
// read has to make them as a parsed one does.
func TestRemarksComeBack(t *testing.T) {
	src := padded("cat <<EOF\nhello\n")
	d := bash.Dialect()
	want := parsed(src, d, fresh(false), schedule{})
	if len(want.Remarks) == 0 {
		t.Fatal("the text makes no remark: the case tests nothing")
	}
	equivalent(t, "remarks", src, d, false, schedule{})
}

// TestADamagedEntryIsNotUsed hands the reader an entry cut short at every
// length, and one with each of its bytes changed: a reader may serve what an
// entry still holds whole and must read the rest, and must never panic or
// read differently from a parser.
func TestADamagedEntryIsNotUsed(t *testing.T) {
	src := padded("f() { echo one; }\ng() { echo two; }\nf\ng\n")
	d := zsh.Dialect()
	mem := newMem()
	cached(mem, "/t/d", src, d, fresh(false), schedule{})
	if len(mem.m) != 1 {
		t.Fatalf("%d entries recorded", len(mem.m))
	}
	var name string
	var good []byte
	for name, good = range mem.m {
	}
	want := parsed(src, d, fresh(false), schedule{})
	cuts := 0
	for n := range len(good) {
		bad := &memCache{m: map[string][]byte{name: slices.Clone(good[:n])}}
		got, _ := cached(bad, "/t/d", src, d, fresh(false), schedule{})
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("an entry cut to %d bytes read differently", n)
		}
		cuts++
	}
	// A byte changed in the frame of the entry or of a read is caught by its
	// structure; one changed inside a tree may be read as another tree, which
	// is why the store keeps a checksum — this is only that nothing panics.
	for i := range len(good) {
		bad := slices.Clone(good)
		bad[i] ^= 0x5a
		cached(&memCache{m: map[string][]byte{name: bad}}, "/t/d", src, d, fresh(false), schedule{})
	}
	if cuts == 0 {
		t.Fatal("nothing was cut")
	}
}

// TestAFailedTextIsNotRecordedAgain is the entry a text that does not parse
// leaves: it says so, and the next read of the text parses it without
// recording it again or serving any of it.
func TestAFailedTextIsNotRecordedAgain(t *testing.T) {
	src := padded("echo one\nif then\n")
	d := bash.Dialect()
	mem := newMem()
	cached(mem, "/t/x", src, d, fresh(false), schedule{})
	if len(mem.m) != 1 {
		t.Fatalf("a failed text left %d entries, want the one that says it failed", len(mem.m))
	}
	creates := mem.creates
	got, served := cached(mem, "/t/x", src, d, fresh(false), schedule{})
	if served != 0 || mem.creates != creates {
		t.Fatalf("a failed text was served %d reads and recorded %d more times", served, mem.creates-creates)
	}
	if !reflect.DeepEqual(got, parsed(src, d, fresh(false), schedule{})) {
		t.Fatal("a failed text read differently")
	}
	if got.Err == "" {
		t.Fatal("the text parsed: the case tests nothing")
	}
}

// TestAShortReadIsExtended stops reading partway, as a `return` in a sourced
// file does, then reads the whole text: the recorded reads are served, the
// rest is read and recorded, and the entry then serves all of it.
func TestAShortReadIsExtended(t *testing.T) {
	src := padded("echo one\necho two\necho three\necho four\n")
	d := zsh.Dialect()
	mem := newMem()
	r := parsecache.NewReader(mem, "/t/s", src, d, fresh(false))
	r.NextLine()
	r.NextLine()
	r.Finish()
	if _, served := cached(mem, "/t/s", src, d, fresh(false), schedule{}); served != 2 {
		t.Fatalf("a run reading further was served %d reads, want the 2 recorded", served)
	}
	got, served := cached(mem, "/t/s", src, d, fresh(false), schedule{})
	if served != 5 {
		t.Fatalf("after the longer run the entry served %d reads of 5", served)
	}
	if !reflect.DeepEqual(got, parsed(src, d, fresh(false), schedule{})) {
		t.Fatal("the extended entry read differently")
	}
}

// TestEntriesAreNamedByTheirState reads one file from two alias tables, as a
// theme file sourced before and after the aliases are defined is: each is
// kept, and neither run rewrites the other's entry.
func TestEntriesAreNamedByTheirState(t *testing.T) {
	src := padded("echo one\n")
	d := zsh.Dialect()
	before, after := fresh(false), fresh(true)
	mem := newMem()
	cached(mem, "/t/n", src, d, before, schedule{})
	cached(mem, "/t/n", src, d, after, schedule{})
	creates := mem.creates
	for _, sh := range []*shell{before, after, before} {
		if _, served := cached(mem, "/t/n", src, d, sh, schedule{}); served == 0 {
			t.Fatal("a state the file was read from before was not served")
		}
	}
	if mem.creates != creates || len(mem.m) != 2 {
		t.Fatalf("%d entries and %d more recordings", len(mem.m), mem.creates-creates)
	}
}
