// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"maps"
	"path/filepath"
	"strings"

	"github.com/blairham/sh/internal/parsecache"
	"github.com/blairham/sh/syntax"
)

// ParseCache keeps the trees of files this shell has parsed, from one run of
// it to the next (#6424).
//
// A start on a real configuration sources over a megabyte of shell, and
// parsing it was the largest single item left in the time to a usable line.
// The shell this one is measured against avoids most of that with compiled
// copies of the same files, kept beside them by the plugin managers; their
// format is that shell's own, so this shell keeps a cache of its own instead.
//
// Nil — the default — is a runner that parses every file it reads, which is
// every embedder: deciding to keep files between runs, and where, is the
// front end's to make. driver fills it in.
//
// What a store holds is opaque to it. Everything that decides whether a
// stored tree may be used is checked here, on every read — the text it was
// parsed from, by its hash; the grammar it was read in; each alias and each
// locale question the parser asked — so a store that hands back the wrong
// bytes costs a parse and never a wrong tree.
type ParseCache interface {
	// Load returns what was last stored under name, or nil.
	Load(name string) []byte
	// Store keeps data under name, replacing whatever was there. It may
	// decline without saying so.
	Store(name string, data []byte)
}

// textReader is what reads a file a line at a time, or whole: a
// syntax.Parser, or a parsecache.Reader answering in its place.
type textReader interface {
	SetDialect(d syntax.Dialect)
	NextLine() (*syntax.File, bool)
	Parse() *syntax.File
	Err() error
	Remarks() []syntax.Remark
}

// fileReader is the reader for the text of a file at path, read in dialect
// d — exactly what ParseWithAliases(src, d) would be, from the cache where
// the runner has one. done is called once the caller has finished reading,
// and is where what was read is stored for next time.
//
// path is how the file was named. A relative one is taken from the runner's
// directory; it names the entry and nothing more, since what an entry is
// checked against is the text itself.
func (r *Runner) fileReader(path, src string, d syntax.Dialect) (p textReader, done func()) {
	if r.ParseCache == nil || path == "" || len(src) < parsecache.MinText ||
		strings.HasPrefix(path, "/dev/") || strings.HasPrefix(path, "/proc/") || r.parseCacheOff() {
		// A descriptor's name is a stream and not a file: what it holds
		// next time is somebody else's text. And a short text is cheaper
		// to parse than to look up.
		return r.ParseWithAliases(src, d), func() {}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.dirNow(), path)
	}
	c := parsecache.NewReader(r.ParseCache, path, src, r.ParsingDialect(d), parseAsker{r})
	return c, c.Finish
}

// parseCacheOff is the per-run switch: SH_NO_PARSE_CACHE set to anything but
// empty or 0, as the shell holds it. The front end reads the environment for
// the same switch before it builds a cache at all; this is the answer for a
// script that sets it partway through.
func (r *Runner) parseCacheOff() bool {
	v, ok := r.getVar("SH_NO_PARSE_CACHE")
	return ok && v != "" && v != "0"
}

// parseAsker is the runner answering a parse-cache reader's questions, on
// behalf of the parser it stands in for.
type parseAsker struct{ r *Runner }

// Alias is the answer the parser's own alias field would have given.
func (a parseAsker) Alias(kind parsecache.AliasKind, name string) (string, bool) {
	switch kind {
	case parsecache.AliasGlobal:
		return a.r.ExpandingGlobalAlias(name)
	case parsecache.AliasSuffix:
		return a.r.ExpandingSuffixAlias(name)
	}
	return a.r.ExpandingAlias(name)
}

// AliasTable is a fingerprint of everything the answers to kind depend on:
// the switch that turns expansion off, and every entry a lookup of that kind
// can find. Two runs whose fingerprints agree give every name the same
// answer, which is what lets a stored read stand on one number rather than
// on each word it asked about.
//
// Kept per table as of a count of changes to the tables, so a startup that
// reads a thousand lines between two `alias` commands works it out once.
func (a parseAsker) AliasTable(kind parsecache.AliasKind) uint64 {
	r := a.r

	if !r.aliasExpansion {
		return aliasSumOff
	}
	m := &r.aliasSums[kind]
	if m.valid && m.gen == r.aliasGen {
		return m.sum
	}
	var sum uint64
	switch kind {
	case parsecache.AliasSuffix:
		for name, value := range r.suffixAliases {
			sum += aliasEntrySum(name, value, false)
		}
	default:
		for name, d := range r.aliases {
			if kind == parsecache.AliasGlobal && !d.global {
				continue
			}
			sum += aliasEntrySum(name, d.value, d.global)
		}
	}
	// A sum, because a map has no order; the count goes in so that no two
	// entries can cancel to an empty table.
	*m = aliasSum{valid: true, gen: r.aliasGen, sum: fnvMix(sum, uint64(r.aliasCount(kind)))}
	return m.sum
}

// AliasSnapshot answers kind's questions from the tables as they stand now,
// however they change afterwards — for a reader that has to put a parser
// back where it stood several lines ago.
func (a parseAsker) AliasSnapshot(kind parsecache.AliasKind) func(name string) (string, bool) {
	r := a.r
	if !r.aliasExpansion {
		return func(string) (string, bool) { return "", false }
	}
	switch kind {
	case parsecache.AliasSuffix:
		held := maps.Clone(r.suffixAliases)
		return func(name string) (string, bool) {
			v, ok := held[name]
			return v, ok
		}
	case parsecache.AliasGlobal:
		held := map[string]string{}
		for name, d := range r.aliases {
			if d.global {
				held[name] = d.value
			}
		}
		return func(name string) (string, bool) {
			v, ok := held[name]
			return v, ok
		}
	}
	held := make(map[string]string, len(r.aliases))
	for name, d := range r.aliases {
		held[name] = d.value
	}
	return func(name string) (string, bool) {
		v, ok := held[name]
		return v, ok
	}
}

func (r *Runner) aliasCount(kind parsecache.AliasKind) int {
	switch kind {
	case parsecache.AliasSuffix:
		return len(r.suffixAliases)
	case parsecache.AliasGlobal:
		n := 0
		for _, d := range r.aliases {
			if d.global {
				n++
			}
		}
		return n
	}
	return len(r.aliases)
}

// aliasSum is one table's fingerprint as of a count of changes.
type aliasSum struct {
	valid bool
	gen   uint64
	sum   uint64
}

// aliasSumOff stands for expansion turned off, which answers every name the
// same way whatever the tables hold.
const aliasSumOff = 0x6f66662d616c6961

func aliasEntrySum(name, value string, global bool) uint64 {
	h := uint64(14695981039346656037)
	h = fnvString(h, name)
	h = fnvString(h, value)
	if global {
		h = fnvMix(h, 1)
	}
	return h
}

func fnvString(h uint64, s string) uint64 {
	h = fnvMix(h, uint64(len(s)))
	for i := range len(s) {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

func fnvMix(h, x uint64) uint64 {
	for range 8 {
		h ^= x & 0xff
		h *= 1099511628211
		x >>= 8
	}
	return h
}
