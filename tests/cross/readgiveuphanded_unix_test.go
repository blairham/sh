// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package cross_test

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/internal/dialecttest"
)

// The descriptors a shell is handed, in the dialect a trapped signal gives a
// `read` up in, where how the stream was opened decides when (#7030). Every
// row was measured 2026-10-31 against ksh93u+ 2012-08-01 run as `ksh -c
// script <>p 3<>p`: a fifo handed over open both ways as standard input and
// again as descriptor 3, and the wanted output is what it printed.
//
// The standard input a shell is handed counts as open for reading alone,
// whatever it was opened as, and so does every copy of it; a descriptor 3
// handed over the same way does not, and nor does a standard input the
// script opened both ways.
func TestTheStandardInputAShellIsHandedReadsOnThroughASignal(t *testing.T) {
	t.Parallel()
	const (
		readOn      = "T\nst=0 l=[abcd]\nnext [end]\n"
		gaveUpAB258 = "T\nst=258 l=[ab]\nnext [cd]\n"
		gaveUpFirst = "T\nst=258 l=[]\nnext [abcd]\n"
	)
	// part has `ab` waiting, INT 0.3 s into the read and `cd` and `end`
	// half a second after; pending has the signal already waiting as the
	// read begins, which only a descriptor open both ways gives up on.
	part := func(read, next string) string {
		return `trap "echo T" INT; printf ab >p; (sleep 0.3; kill -INT $$; sleep 0.5; printf 'cd\nend\n' >p) & ` +
			read + `; echo "st=$? l=[$l]"; ` + next + `; echo "next [$m]"; wait`
	}
	pending := func(read, next string) string {
		return `trap "echo T" INT; printf ab >p; (sleep 0.5; printf 'cd\nend\n' >p) & ` +
			read + ` 2>"$(kill -INT $$)/dev/null"; echo "st=$? l=[$l]"; ` + next + `; echo "next [$m]"; wait`
	}
	rows := []struct{ name, src, want string }{
		{"part", part(`read -r l`, `read -r m`), readOn},
		{"part, -u0", part(`read -r -u0 l`, `read -r m`), readOn},
		{"part, a copy", part(`exec 4<&0; read -r -u4 l`, `read -r -u4 m`), readOn},
		// A move is what its source was: opened at a number, so a `-u`
		// read of it is given up at the next byte — but not open both ways.
		{"part, a move", part(`exec 4<&0-; read -r -u4 l`, `read -r -u4 m`), gaveUpAB258},
		{"pending", pending(`read -r l`, `read -r m`), readOn},
		{"pending, -u0", pending(`read -r -u0 l`, `read -r m`), readOn},
		{"pending, a copy", pending(`exec 4<&0; read -r -u4 l`, `read -r -u4 m`), readOn},
		{"pending, a move", pending(`exec 4<&0-; read -r -u4 l`, `read -r -u4 m`), readOn},
		// The controls: the script's own both-ways standard input, and the
		// descriptor 3 handed over beside it.
		{"part, exec 0<>p", part(`exec 0<>p; read -r l`, `read -r m`), gaveUpAB258},
		{"pending, exec 0<>p", pending(`exec 0<>p; read -r l`, `read -r m`), gaveUpFirst},
		{"part, 3", part(`read -r l <&3`, `read -r m <&3`), gaveUpAB258},
		{"pending, 3", pending(`read -r l <&3`, `read -r m <&3`), gaveUpFirst},
	}
	for _, row := range rows {
		t.Run("ksh/"+row.name, func(t *testing.T) {
			t.Parallel()
			if got := runOnAHandedFifo(t, presets["ksh"], row.src); got != row.want {
				t.Errorf("got %q, want %q", got, row.want)
			}
		})
	}
}

// runOnAHandedFifo runs src in a runner handed a fifo `p`, opened both ways,
// as its standard input and as descriptor 3, as `ksh -c src <>p 3<>p` would
// be. Each open holds a writer, so no read of the fifo ends for want of one.
//
// A run still going after releaseFifosAfter is waiting on a line nothing
// wrote; it is handed one and failed, as runReleasingFifos does.
func runOnAHandedFifo(t *testing.T, p dialecttest.Preset, src string) string {
	t.Helper()
	dir := t.TempDir()
	fifo := filepath.Join(dir, "p")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	var handed [2]*os.File
	for i := range handed {
		f, err := os.OpenFile(fifo, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Close() })
		handed[i] = f
	}
	var buf lockedOutput
	r := p.Runner(dialecttest.Base{
		Dir: dir, Env: []string{"PATH=/usr/bin:/bin"}, Stdout: &buf, Stderr: &buf,
	})
	r.Stdin = handed[0]
	r.InheritedFiles = []*os.File{handed[1]}
	f := p.ParseThrough(t, r, src)
	done := make(chan error, 1)
	go func() {
		_, err := r.Run(context.Background(), f)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("err %v: %s", err, buf.String())
		}
		return buf.String()
	case <-time.After(releaseFifosAfter):
	}
	_, _ = handed[0].WriteString("x\nx\n")
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	t.Fatalf("still running after %v, waiting on a fifo nothing wrote to. Output: %q", releaseFifosAfter, buf.String())
	return ""
}
