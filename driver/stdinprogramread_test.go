// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// fileWith writes a program to a regular file and opens it for reading, which
// is the `sh < script` route: a descriptor that can be put back where it was.
func fileWith(t testing.TB, s string) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "program.sh")
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// The two ways a program reaches standard input differ in one thing only: a
// regular file can be rewound and a pipe cannot, so the shell may read ahead
// on the first and must not on the second. Nothing about that is the script's
// business, and it must not be able to tell.
//
// Measured before it was implemented, because it is the whole license for the
// optimisation: bash, dash, ksh93 and zsh each produce byte-identical output
// for every one of these programs whether it arrives on a pipe or on a file.
// The split cases are the sharp ones — what the script's own `read` finds, and
// what a command inheriting descriptor 0 finds, is exactly the question of how
// far ahead the shell read.
func TestAProgramReadsTheSameFromAFileAsFromAPipe(t *testing.T) {
	dir := t.TempDir()
	rest := filepath.Join(dir, "rest.sh")
	if err := os.WriteFile(rest, []byte("echo from the file\necho and again\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name    string
		program string
	}{
		{
			name:    "a read takes the next line of the program",
			program: "read x\necho \"[$x]\"\nDATA-LINE\necho end\n",
		},
		{
			name:    "a while read loop eats the rest of the program",
			program: "while read -r l; do echo \"got:$l\"; done\nA\nB\nC\n",
		},
		{
			name:    "a command inherits the rest of the program",
			program: "echo one\ncat\nNOT-A-COMMAND\necho end\n",
		},
		{
			name:    "exec repoints the rest of the program",
			program: "echo one\nexec 0< " + rest + "\necho never reached\n",
		},
		{
			name:    "a here-document body belongs to its command",
			program: "cat <<EOF\nbody\nEOF\necho after\n",
		},
		{
			name:    "a continued line joins the one after it",
			program: "echo one \\\ntwo\necho three\n",
		},
		{
			name:    "a construct is read until it finishes",
			program: "if true\nthen\n\techo yes\nfi\necho done\n",
		},
		{
			name: "a program longer than one buffer",
			// Longer than the reader's buffer, so a rewind happens on a
			// boundary that falls inside a line rather than tidily between
			// two, and happens many times.
			program: strings.Repeat("echo pad\n", 4000) + "read x\necho \"[$x]\"\nLAST-LINE\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, blocks := range []bool{false, true} {
				run := func(in *os.File) (string, string, int) {
					sh := shell()
					sh.Semantics = interp.PosixSemantics()
					sh.Semantics.StdinProgramReadInBlocks = blocks
					sh.Diagnostics = interp.Diagnostics{Location: interp.LocationLineWord}
					sh.Stdin = in
					var o, e bytes.Buffer
					sh.Stdout, sh.Stderr = &o, &e
					code := driver.MainArgs(sh, []string{"testsh"})
					return o.String(), e.String(), code
				}
				pipeOut, pipeErr, pipeCode := run(pipeWith(t, c.program))
				fileOut, fileErr, fileCode := run(fileWith(t, c.program))
				if fileOut != pipeOut {
					t.Errorf("blocks=%v: from a file %q, from a pipe %q", blocks, fileOut, pipeOut)
				}
				if fileErr != pipeErr {
					t.Errorf("blocks=%v: stderr from a file %q, from a pipe %q", blocks, fileErr, pipeErr)
				}
				if fileCode != pipeCode {
					t.Errorf("blocks=%v: status from a file %d, from a pipe %d", blocks, fileCode, pipeCode)
				}
			}
		})
	}
}

// The rewind, stated as the position rather than as the output: a line taken
// off a file must leave the descriptor on the byte after that line's newline
// and not on the byte after whatever the reader happened to fetch, which is up
// to a whole buffer further on.
//
// `cat` proves this from the script's side and the case above does that. This
// proves it from the shell's, which is what every later reader depends on and
// what the byte-at-a-time loop got right by never over-reading at all.
func TestALineReadFromAFileLeavesTheDescriptorAfterTheLine(t *testing.T) {
	// Far more than one buffer behind the first line, so a reader that kept
	// what it fetched would be thousands of bytes out rather than a few.
	src := "echo one\n" + strings.Repeat("padding that must still be there\n", 1000)
	f := fileWith(t, src)
	buf := make([]byte, 8192)
	// One reader across every line, which is how the front end holds it: the
	// descriptor is asked whether it seeks once and the answer is kept, so a
	// reader that kept the wrong one would go wrong on the second line and not
	// on the first.
	var lr driver.LineReaderForTest
	at := 0
	for _, want := range strings.SplitAfter(src, "\n") {
		if want == "" {
			break
		}
		if got := lr.Read(f, buf); got != want {
			t.Fatalf("at byte %d: read %q, want %q", at, got, want)
		}
		at += len(want)
		pos, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			t.Fatal(err)
		}
		if pos != int64(at) {
			t.Fatalf("after %q the descriptor is at %d, want %d", want, pos, at)
		}
	}
	if got := lr.Read(f, buf); got != "" {
		t.Errorf("read %q past the end of the file, want nothing", got)
	}
}

// The same thing said where it can be measured directly: the script asks what
// is left, one line at a time, and every line of the rest of the program has
// to be there. A reader that took a buffer and did not give the remainder back
// would answer with a hole the size of the buffer.
func TestNoBytesAreLostAheadOfWhatTheScriptReads(t *testing.T) {
	// Wider than the reader's buffer so that several rewinds happen, and long
	// enough that a lost buffer could not be mistaken for a lost line.
	const lines = 3000
	var sb strings.Builder
	sb.WriteString("while read -r l; do echo \"$l\"; done\n")
	for i := range lines {
		fmt.Fprintf(&sb, "data-%04d\n", i)
	}
	sh := shell()
	sh.Semantics = interp.PosixSemantics()
	sh.Stdin = fileWith(t, sb.String())
	var o, e bytes.Buffer
	sh.Stdout, sh.Stderr = &o, &e
	if code := driver.MainArgs(sh, []string{"testsh"}); code != 0 {
		t.Fatalf("status %d, stderr %q", code, e.String())
	}
	var want strings.Builder
	for i := range lines {
		fmt.Fprintf(&want, "data-%04d\n", i)
	}
	if o.String() != want.String() {
		t.Errorf("the loop read %d bytes, want %d", o.Len(), want.Len())
	}
}

// A descriptor that reports a position and then refuses to move is the one
// case where the reader has already taken bytes it cannot hand back. It hands
// them forward instead — everything read becomes program text — because that
// is the only answer that loses none of them.
func TestAnOverReadThatCannotBeGivenBackIsRunAsTheProgram(t *testing.T) {
	// Not reachable through Shell.Stdin, which is an *os.File and either seeks
	// or does not: this is the reader's own contract, exercised where a
	// descriptor that changes its mind can be built.
	in := &seekOnceReader{r: strings.NewReader("echo one\necho two\n")}
	var lr driver.LineReaderForTest
	got := lr.Read(in, make([]byte, 64))
	if want := "echo one\necho two\n"; got != want {
		t.Errorf("read %q, want %q — the bytes past the line were dropped", got, want)
	}
}

// seekOnceReader answers the reader's question about where it is and then
// fails the rewind, which is the only way to reach that arm.
type seekOnceReader struct {
	r      *strings.Reader
	probed bool
}

func (s *seekOnceReader) Read(p []byte) (int, error) { return s.r.Read(p) }

func (s *seekOnceReader) Seek(int64, int) (int64, error) {
	if !s.probed {
		s.probed = true
		return 0, nil
	}
	return 0, io.ErrUnexpectedEOF
}

// benchProgram is a program long enough for the cost of fetching it to show
// over the cost of running it: assignments, which the interpreter finishes
// immediately, so what is measured is the reading.
func benchProgram(lines int) string {
	var sb strings.Builder
	for i := range lines {
		fmt.Fprintf(&sb, "x%d=%d\n", i, i)
	}
	return sb.String()
}

// The reason #567 exists, as a number. A program on standard input is fetched
// one buffer at a time when the descriptor can be rewound and one byte at a
// time when it cannot, and this is the difference between the two on the same
// program — plus the same program as an operand, which is the floor.
func BenchmarkAProgramOnStandardInput(b *testing.B) {
	const lines = 20000
	src := benchProgram(lines)
	path := filepath.Join(b.TempDir(), "program.sh")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		b.Fatal(err)
	}
	run := func(b *testing.B, in *os.File) {
		b.Helper()
		sh := shell()
		sh.Semantics = interp.PosixSemantics()
		sh.Stdin = in
		sh.Stdout, sh.Stderr = io.Discard, io.Discard
		if code := driver.MainArgs(sh, []string{"testsh"}); code != 0 {
			b.Fatalf("status %d", code)
		}
	}
	b.Run("a file, which can be rewound", func(b *testing.B) {
		for b.Loop() {
			b.StopTimer()
			f, err := os.Open(path)
			if err != nil {
				b.Fatal(err)
			}
			b.StartTimer()
			run(b, f)
			b.StopTimer()
			_ = f.Close()
			b.StartTimer()
		}
	})
	b.Run("a pipe, which cannot", func(b *testing.B) {
		for b.Loop() {
			b.StopTimer()
			r, w, err := os.Pipe()
			if err != nil {
				b.Fatal(err)
			}
			go func() {
				_, _ = w.WriteString(src)
				_ = w.Close()
			}()
			b.StartTimer()
			run(b, r)
			b.StopTimer()
			_ = r.Close()
			b.StartTimer()
		}
	})
	b.Run("an operand, which is read whole", func(b *testing.B) {
		for b.Loop() {
			sh := shell()
			sh.Semantics = interp.PosixSemantics()
			sh.Stdout, sh.Stderr = io.Discard, io.Discard
			if code := driver.MainArgs(sh, []string{"testsh", path}); code != 0 {
				b.Fatalf("status %d", code)
			}
		}
	})
}
