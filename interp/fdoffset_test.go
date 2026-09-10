// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Where an open descriptor's file is positioned: the seam under a shell
// facility that reports it, and the honest false where there is no position
// to report.

// TestADescriptorsOffsetIsWhereReadingLeftIt.
//
// The whole point is that the number *moves*: a seam that asked the kernel
// once, or that answered the file's size, or that answered nought, would pass
// a case that only ever read it before anything happened.
//
// The offsets are five and ten for two lines of four bytes each, which is the
// line and its newline both times — so this also says the shell's `read` takes
// exactly what it consumed off the descriptor and no more.
//
// `read x <&3` rather than a letter naming the descriptor: the letter is a
// dialect's and the duplication is the core's, and this package's cases name
// neither a shell nor a shell's spelling.
func TestADescriptorsOffsetIsWhereReadingLeftIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lines")
	if err := os.WriteFile(path, []byte("abcd\nefgh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := syntax.Core()
	f, err := syntax.Parse("exec 3< "+path+"\nread x <&3\n", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Dir: dir})
	if off, ok := r.DescriptorOffset(3); ok {
		t.Fatalf("offset of an unopened descriptor = %d, %v; want not one at all", off, ok)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	off, ok := r.DescriptorOffset(3)
	if !ok || off != 5 {
		t.Errorf("offset after one line = %d, %v; want 5, true", off, ok)
	}
	f, err = syntax.Parse("read x <&3\n", d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if off, ok := r.DescriptorOffset(3); !ok || off != 10 {
		t.Errorf("offset after the second line = %d, %v; want 10, true", off, ok)
	}
}

// TestANumberWithNoPositionBehindItIsNotOne.
//
// Three cases that must all come back false, and they are different reasons
// that deserve the one answer: nothing is open there, the number is one of the
// named streams held by an in-memory buffer with no descriptor at all, and the
// stream is a pipe, which has no position however open it is.
func TestANumberWithNoPositionBehindItIsNotOne(t *testing.T) {
	d := syntax.Core()
	var out bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Dialect: &d})
	for _, fd := range []int{1, 2, 7, 99} {
		if off, ok := r.DescriptorOffset(fd); ok {
			t.Errorf("DescriptorOffset(%d) = %d, true; want not one", fd, off)
		}
	}
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rd.Close(); _ = wr.Close() })
	r2 := newTestRunner(t, &Runner{Stdin: rd, Stdout: &out, Stderr: &out, Dialect: &d})
	if off, ok := r2.DescriptorOffset(0); ok {
		t.Errorf("DescriptorOffset of a pipe = %d, true; want not one", off)
	}
}
