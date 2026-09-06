// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"os"
	"path/filepath"
	"testing"
)

// What the walk costs, as something that runs, because "O(depth) syscalls
// instead of one" is a shape rather than a number and the number is what a
// person deciding whether to turn a policy on actually wants.
//
// Three benchmarks rather than one, because the interesting comparison is not
// against an ungated open — a run with no gate does not enter this package at
// all — but against the arrangement the walk replaced: one open plus one read
// of the platform's answer.
//
// Measured at a twelve-to-sixteen component path, which is what a temporary
// directory already is on either platform:
//
//	                              Darwin 25.5.0    Linux 6.8
//	one plain open                     9.4 µs        2.3 µs
//	open, then ask the platform        9.7 µs        3.1 µs
//	the walk                          35.5 µs        8.5 µs
//
// So a gated open costs about three and a half times what it did, and the
// difference is tens of microseconds against a shell that spends milliseconds
// forking anything. `make conformance-gated` over 1734 cases takes the same
// 38 seconds it took before. The reason to keep this here is the other
// direction: a change that made the walk quadratic, or that stopped short-
// circuiting the ungated path, would show up as a number rather than as a
// feeling.
func benchFixture(b *testing.B, depth int) string {
	b.Helper()
	dir := b.TempDir()
	parts := make([]string, 0, depth)
	for i := range depth {
		parts = append(parts, string(rune('a'+i%26)))
	}
	deep := filepath.Join(dir, filepath.Join(parts...))
	if err := os.MkdirAll(deep, 0o755); err != nil {
		b.Fatal(err)
	}
	target := filepath.Join(deep, "file")
	if err := os.WriteFile(target, []byte("x\n"), 0o600); err != nil {
		b.Fatal(err)
	}
	return target
}

// BenchmarkPlainOpen is what an ungated run does, and what this package must
// not slow down: it is not entered at all when there is no gate.
func BenchmarkPlainOpen(b *testing.B) {
	p := benchFixture(b, 8)
	for b.Loop() {
		f, err := os.OpenFile(p, os.O_RDONLY, 0)
		if err != nil {
			b.Fatal(err)
		}
		_ = f.Close()
	}
}

// BenchmarkOpenThenAskThePlatform is the arrangement the walk replaced, kept as
// the honest baseline: the walk is not competing with a plain open, it is
// competing with this.
func BenchmarkOpenThenAskThePlatform(b *testing.B) {
	p := benchFixture(b, 8)
	for b.Loop() {
		f, err := os.OpenFile(p, os.O_RDONLY, 0)
		if err != nil {
			b.Fatal(err)
		}
		if _, ok := Path(f); !ok {
			b.Fatal("the platform had no answer for an ordinary file")
		}
		_ = f.Close()
	}
}

// BenchmarkWalk is what a gated open costs now.
func BenchmarkWalk(b *testing.B) {
	p := benchFixture(b, 8)
	for b.Loop() {
		r, err := Open(p, os.O_RDONLY, 0)
		if err != nil {
			b.Fatal(err)
		}
		_ = r.File.Close()
	}
}
