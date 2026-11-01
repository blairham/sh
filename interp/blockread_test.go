// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// countingFile is a regular file that counts the reads made of it, which is
// the number of read(2) calls the shell makes on the descriptor: *os.File
// keeps no buffer of its own, so one Read is one system call.
type countingFile struct {
	*os.File
	reads int
}

func (f *countingFile) Read(p []byte) (int, error) {
	f.reads++
	return f.File.Read(p)
}

func lineFile(t *testing.T, text string) *countingFile {
	t.Helper()
	p := filepath.Join(t.TempDir(), "data")
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return &countingFile{File: f}
}

func offsetOf(t *testing.T, f *countingFile) int64 {
	t.Helper()
	at, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

// The line readers read a regular file in blocks and put back what they did
// not take, so a read costs a couple of system calls rather than one per byte
// — #6355 counted 3.9 million read(2) calls for a 100,000-line file — and the
// offset afterwards is still exactly past the delimiter, which is what the
// next reader of the descriptor sees: measured, `{ read -r l; cat; } < f`
// hands `cat` the second line onward in every shell in the panel.
func TestLineReadersReadAFileInBlocksAndPutBackTheRest(t *testing.T) {
	t.Parallel()
	var text strings.Builder
	for i := range 10 {
		text.WriteString("line ")
		text.WriteByte(byte('0' + i))
		text.WriteByte('\n')
	}
	long := strings.Repeat("x", 1000)
	for _, tc := range []struct {
		name, text, src, want string
		offset                int64
		mostReads             int
	}{
		{
			"two reads take two lines and leave the third",
			text.String(), `read -r a; read -r b; echo "$a|$b"`, "line 0|line 1\n",
			14, 2,
		},
		{
			"a delimiter other than the newline stops inside the line",
			text.String(), `mapfile -t -d ' ' -n 1 arr; echo "${arr[0]}"`, "line\n",
			5, 1,
		},
		{
			"a line longer than a block is read whole and nothing past it",
			long + "\nrest\n", `read -r a; echo "${#a}"`, "1000\n",
			1001, 6,
		},
		{
			"mapfile with a count leaves the rest of the file",
			text.String(), `mapfile -n 3 arr; echo "${#arr[@]}"`, "3\n",
			21, 1,
		},
		{
			"mapfile with no count reads to the end",
			text.String(), `mapfile -t arr; echo "${#arr[@]}:${arr[9]}"`, "10:line 9\n",
			70, 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := lineFile(t, tc.text)
			out, _ := mapfileRun(t, tc.src, func(r *Runner) { r.Stdin = f })
			if out != tc.want {
				t.Errorf("said %q, want %q", out, tc.want)
			}
			if at := offsetOf(t, f); at != tc.offset {
				t.Errorf("left the offset at %d, want %d — the next reader starts there", at, tc.offset)
			}
			if f.reads > tc.mostReads {
				t.Errorf("made %d reads of a %d-byte file, want at most %d", f.reads, len(tc.text), tc.mostReads)
			}
		})
	}
}

// A mapfile callback is shell code and may read the stream mapfile is
// reading, and what it reads is the line after the element it was handed —
// measured on bash 5.3.20, `mapfile -c 1 -C 'read x; …'` over one..four gives
// the callback two and four, on a file and on a pipe alike. Read ahead in a
// block, that is only true if the block is put back before the callback runs.
func TestAMapfileCallbackReadsFromWhereTheElementEnded(t *testing.T) {
	t.Parallel()
	f := lineFile(t, "one\ntwo\nthree\nfour\n")
	out, _ := mapfileRun(t, `mapfile -t -c 1 -C 'read x; echo "cb=$x";:' arr; echo "${arr[*]}"`,
		func(r *Runner) { r.Stdin = f })
	if want := "cb=two\ncb=four\none three\n"; out != want {
		t.Errorf("said %q, want %q", out, want)
	}
}

// The control: a pipe cannot be put back, so a read from one still takes a
// byte at a time and leaves the rest in the pipe for whoever reads next.
func TestALineReaderLeavesTheRestOfAPipe(t *testing.T) {
	t.Parallel()
	for _, src := range []string{`read -r a; echo "$a"`, `mapfile -n 1 arr; echo "${arr[0]}"`} {
		t.Run(src, func(t *testing.T) {
			pr, pw, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = pr.Close() })
			if _, err := pw.WriteString("first\nsecond\nthird\n"); err != nil {
				t.Fatal(err)
			}
			_ = pw.Close()
			out, _ := mapfileRun(t, src, func(r *Runner) { r.Stdin = pr })
			if !strings.HasPrefix(out, "first") {
				t.Errorf("said %q, want the first line", out)
			}
			rest, err := io.ReadAll(pr)
			if err != nil {
				t.Fatal(err)
			}
			if string(rest) != "second\nthird\n" {
				t.Errorf("left %q in the pipe, want the second line onward", rest)
			}
		})
	}
}
