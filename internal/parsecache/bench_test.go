// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package parsecache_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/parsecache"
)

// library is this repository's own zsh function library, read as the files a
// start sources: real zsh, and the same bytes on every machine.
func library(tb testing.TB) []string {
	tb.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "share", "sh", "functions", "*"))
	if err != nil || len(paths) < 20 {
		tb.Fatalf("the function library is not where it was: %v", err)
	}
	var out []string
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			tb.Fatal(err)
		}
		out = append(out, string(b))
	}
	return out
}

func readAll(r reader) {
	for {
		if _, ok := r.NextLine(); !ok {
			return
		}
	}
}

// BenchmarkParse, BenchmarkRecord and BenchmarkServe are the three costs the
// cache trades: a parse, a parse that also records, and a read served from
// what was recorded. Measured 2026-10-12 on the plugin files of a real
// configuration, per kilobyte of text: about 21 µs to parse, 7 µs more to
// record, and 6 µs to serve.
func BenchmarkParse(b *testing.B) {
	texts, d := library(b), zsh.Dialect()
	b.ReportAllocs()
	for b.Loop() {
		for _, src := range texts {
			readAll(plain(src, d, fresh(true)))
		}
	}
}

func BenchmarkRecord(b *testing.B) {
	texts, d := library(b), zsh.Dialect()
	b.ReportAllocs()
	for b.Loop() {
		for i, src := range texts {
			r := parsecache.NewReader(newMem(), filepath.Join("/b", string(rune('a'+i%26))), src, d, fresh(true))
			readAll(r)
			r.Finish()
		}
	}
}

func BenchmarkServe(b *testing.B) {
	texts, d := library(b), zsh.Dialect()
	mems := make([]*memCache, len(texts))
	for i, src := range texts {
		mems[i] = newMem()
		r := parsecache.NewReader(mems[i], "/b", src, d, fresh(true))
		readAll(r)
		r.Finish()
	}
	b.ReportAllocs()
	for b.Loop() {
		for i, src := range texts {
			r := parsecache.NewReader(mems[i], "/b", src, d, fresh(true))
			readAll(r)
			r.Finish()
		}
	}
}

// TestAServedReadAllocatesFarLessThanAParse is the shape the decoder was
// built for: a file's nodes of one type taken out of one block rather than
// allocated one at a time. Counted rather than timed, so it says the same on
// a loaded machine. Measured 2026-10-12 over the function library: parsed,
// 55,400 allocations; served with a block per type per line, 22,800; with a
// block per type per file, 11,400.
func TestAServedReadAllocatesFarLessThanAParse(t *testing.T) {
	texts, d := library(t), zsh.Dialect()
	mems := make([]*memCache, len(texts))
	for i, src := range texts {
		mems[i] = newMem()
		r := parsecache.NewReader(mems[i], "/b", src, d, fresh(true))
		readAll(r)
		r.Finish()
	}
	parse := testing.AllocsPerRun(3, func() {
		for _, src := range texts {
			readAll(plain(src, d, fresh(true)))
		}
	})
	served := parsecache.Served.Load()
	serve := testing.AllocsPerRun(3, func() {
		for i, src := range texts {
			r := parsecache.NewReader(mems[i], "/b", src, d, fresh(true))
			readAll(r)
			r.Finish()
		}
	})
	if parsecache.Served.Load() == served {
		t.Fatal("nothing was served: the count below measures a parse")
	}
	if serve*4 > parse {
		t.Fatalf("serving the library made %.0f allocations against a parse's %.0f", serve, parse)
	}
	t.Logf("%.0f allocations served, %.0f parsed", serve, parse)
}
