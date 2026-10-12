// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// readCountingFile is a regular file that counts the reads made of it: one
// Read of an *os.File is one read(2).
type readCountingFile struct {
	*os.File
	reads int
}

func (f *readCountingFile) Read(p []byte) (int, error) {
	f.reads++
	return f.File.Read(p)
}

// A timed `read` of a regular file reads it in blocks, as an untimed one
// does, and still leaves the offset just past the line.
//
// A regular file never makes a reader wait, so no deadline can pass while
// one is read. The timed route read it a byte per system call all the same,
// each byte a round trip to a goroutine — about 20 µs a line where the same
// read without `-t` costs nearly nothing. F-Sy-H reads its theme files with
// `read -r -t 1`, three of them at every interactive start (#6363).
func TestATimedReadOfAFileReadsItInBlocks(t *testing.T) {
	long := strings.Repeat("x", 1000)
	p := filepath.Join(t.TempDir(), "data")
	if err := os.WriteFile(p, []byte("a b\n"+long+"\nrest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	osf, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = osf.Close() })
	f := &readCountingFile{File: osf}
	var out strings.Builder
	sem, diag, dl := Semantics(), Diagnostics(), Dialect()
	r := &interp.Runner{
		Stdin: f, Stdout: &out, Stderr: &out,
		Semantics: &sem, Diagnostics: &diag, Dialect: &dl,
		Dir: t.TempDir(), Name: "zsh",
	}
	Apply(r)
	prog, err := syntax.Parse(`read -r -t 5 a b; read -r -t 5 c; print -r -- "$a|$b|${#c}"`, Dialect())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), prog); err != nil {
		t.Fatal(err)
	}
	if want := "a|b|1000\n"; out.String() != want {
		t.Errorf("said %q, want %q", out.String(), want)
	}
	at, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(4 + 1001); at != want {
		t.Errorf("left the offset at %d, want %d — the next reader starts there", at, want)
	}
	// Two short reads and a doubling block over a 1,000-byte line, as the
	// untimed reader makes; the byte-at-a-time route made 1,005.
	if f.reads > 10 {
		t.Errorf("made %d reads for two lines of 1,005 bytes, want at most 10", f.reads)
	}
}
