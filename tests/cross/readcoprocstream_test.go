// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blairham/sh/internal/dialecttest"
)

// What a `read -p` that ends short leaves in a coprocess's output (#7025), and
// what `read -t 0 -p` says about it. Measured 2026-10-29 against ksh93u+
// 2012-08-01 and zsh 5.9.2, from -c in a directory of the run's own:
//
//	ksh: mkfifo p; { read x <p; echo xy; } |&
//	     v=old; read -t 0.01 -r -p v; echo "st=$? [$v]"
//	     echo go >p; read -r -p w; echo "next [$w]"
//
//	                                       ksh93            zsh
//	-t 0.01, nothing until it returned     st=1 [old]       st=1 [old]
//	                                       next [xy]        next [xy]
//	-t 0, nothing waiting                  none st=1 [old]  none st=1 [old]
//	                                       next [xy]        next [xy]
//	-t 0, `ab` waiting, `c` after          part st=1 [old]
//	                                       next [c]
//	-t 0.01, `ab` waiting, `c` after       drip st=1 [old]
//	                                       next [c]
//	INT trapped, nothing until given up    T, st=258 []
//	                                       next [late]
//
// zsh's rows are the same script with `coproc { … };` for `{ … } |&`.
//
// **A coprocess is a descriptor, and it was read as if it were not.** `read
// -p` reads through a wrapper that notices the end of the coprocess's output,
// and the wrapper hid the descriptor under it. So a timed read fell back to a
// reader goroutine whose abandoned read took the next byte the coprocess
// wrote — `next [y]`, `next []` — and `read -t 0 -p` was told input was waiting
// when none was, and blocked on a coprocess that was waiting for it: the `-t
// 0` rows never finished. ksh93's trapped-signal row is #7024 on the same
// route, `next [ate]`.
//
// The coprocess writes only once the script has said so through the fifo `p`,
// so a byte that must arrive late cannot arrive early, and `q` tells the
// script `ab` is in the pipe before the read asks. A shell that still blocks is
// released by a writer opened on each fifo (runReleasingFifos), and fails
// rather than hanging the test binary.
func TestAReadFromACoprocessLeavesItAsTheShellsDo(t *testing.T) {
	// The coprocess's body, the read, and the label its status is printed
	// under; then the late half, which is the same in every row.
	script := func(dialect, body, read, label string) string {
		start := `mkfifo p q; { ` + body + ` } |& `
		if dialect == "zsh" {
			start = `mkfifo p q; coproc { ` + body + ` }; `
		}
		return start + read + `; echo "` + label + `st=$? [$v]"; ` +
			`echo go >p; read -r -p w; echo "next [$w]"`
	}
	const nothing = `read x <p; echo xy;`
	const partly = `printf ab; echo r >q; read x <p; echo c;`
	const expired, zero = `v=old; read -t 0.01 -r -p v`, `v=old; read -t 0 -r -p v`
	type row struct{ name, dialect, src, want string }
	rows := []row{{
		"trapped", "ksh",
		`mkfifo p; { read x <p; echo late; } |& trap "echo T" INT; (sleep 0.3; kill -INT $$) & ` +
			`read -r -p l; echo "st=$? [$l]"; echo go >p; read -r -p m; echo "next [$m]"; wait`,
		"T\nst=258 []\nnext [late]\n",
	}}
	for _, d := range []string{"ksh", "zsh"} {
		rows = append(rows,
			row{"expired", d, script(d, nothing, expired, ""), "st=1 [old]\nnext [xy]\n"},
			row{"zero", d, script(d, nothing, zero, "none "), "none st=1 [old]\nnext [xy]\n"})
	}
	rows = append(rows,
		row{"zero partial", "ksh", script("ksh", partly, `read r <q; `+zero, "part "), "part st=1 [old]\nnext [c]\n"},
		row{"expired partial", "ksh", script("ksh", partly, `read r <q; `+expired, "drip "), "drip st=1 [old]\nnext [c]\n"})
	for _, r := range rows {
		t.Run(r.dialect+"/"+r.name, func(t *testing.T) {
			if got := runReleasingFifos(t, presets[r.dialect], r.src); got != r.want {
				t.Errorf("got %q, want %q", got, r.want)
			}
		})
	}
}

// runReleasingFifos runs src in a directory of its own and returns what it
// wrote. A run still going after releaseFifosAfter is waiting on one of its
// fifos for something a correct shell would already have done; each is opened
// for writing and handed a line, and held open so a later `>p` finds a reader,
// which lets the run end, and the test fails with what it wrote.
func runReleasingFifos(t *testing.T, p dialecttest.Preset, src string) string {
	t.Helper()
	dir := t.TempDir()
	var buf lockedOutput
	r := p.Runner(dialecttest.Base{
		Dir: dir, Env: []string{"PATH=/usr/bin:/bin"}, Stdout: &buf, Stderr: &buf,
	})
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
	var held []*os.File
	for _, name := range []string{"p", "q"} {
		if w, err := os.OpenFile(filepath.Join(dir, name), os.O_RDWR, 0); err == nil {
			_, _ = w.WriteString("x\n")
			held = append(held, w)
		}
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	for _, w := range held {
		_ = w.Close()
	}
	t.Fatalf("still running after %v, waiting on a fifo nothing wrote to. Output: %q", releaseFifosAfter, buf.String())
	return ""
}

// releaseFifosAfter is how long runReleasingFifos lets a run go before it
// takes it for one waiting on a fifo. It bounds a hang and asserts nothing
// about the shell, so it is set by the slowest correct run rather than by the
// rows' own margins, and kept well inside go test's own ten minutes. The
// slowest is a ksh93 row of TestATrappedSignalGivesUpAReadWhereTheShellsDo,
// reading a 262144-byte line a byte at a time with thirty rows beside it:
// 1.6 s on an idle machine, and up to 35 s under the race detector with
// GOMAXPROCS=2, background QoS and a load average of sixty, where ten seconds
// failed two runs of nine (#7042).
const releaseFifosAfter = 2 * time.Minute
