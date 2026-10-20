// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeInc puts a file beside the run for the script to source, and answers
// the directory both live in.
func writeInc(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "inc"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// This shell's name for the call stack is absent while nothing but sourced
// files is on it, and holds the whole stack — the `source` frames and the
// script's own `main` — the moment a function is anywhere in it.
//
// Measured 2026-09-22 on bash 5.3.20, from a script file, over a script
// sourcing `a`, `a` sourcing `b`, and `b` calling a function:
//
//	in a        ${FUNCNAME[@]} empty,  ${#BASH_SOURCE[@]} 2
//	in b        empty,                 3
//	in the fn   `q source source main` 4
//	sourced from inside a call         `source g h main`
//
// The companion arrays do not follow it: BASH_SOURCE and BASH_LINENO count
// every frame in all four rows, which is why only this one asks.
//
// The rows below are the same questions on the `-c` route these tests take,
// re-measured there in the same run, where there is no script for a `main`
// frame to name and the bottom of the stack is one entry shorter throughout.
func TestTheCallStackNameIsAbsentUntilAFunctionIsOnIt(t *testing.T) {
	for _, tc := range []struct{ name, inc, src, want string }{
		{
			"a file sourced at the top level",
			`echo "in inc: (${FUNCNAME[@]-}) n=${#BASH_SOURCE[@]}"` + "\n",
			`. ./inc`,
			"in inc: () n=1\n",
		},
		{
			"a function called from one",
			"q() { echo \"in q: (${FUNCNAME[@]-})\"; }\nq\n",
			`. ./inc`,
			"in q: (q source)\n",
		},
		{
			"a file sourced from inside a call",
			`echo "in inc: (${FUNCNAME[@]-})"` + "\n",
			`g() { . ./inc; }
h() { g; }
h`,
			"in inc: (source g h)\n",
		},
	} {
		dir := writeInc(t, tc.inc)
		out, st := runBash(t, dir, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s: = %q status %d, want %q at 0", tc.name, out, st, tc.want)
		}
	}
}

// And the RETURN trap a sourced file fires runs in the frame it returned to,
// which is what the names above are read through. Measured in the same run on
// the same route: both firings — the file's end and the function's own return
// — report `(fn)` and the shell's own name, never the sourced file's frame or
// its path.
func TestASourcedFilesReturnTrapRunsInTheCallersFrame(t *testing.T) {
	dir := writeInc(t, "echo inc\n")
	out, st := runBash(t, dir, `trap 'echo "R (${FUNCNAME[@]-}) ${BASH_SOURCE[0]}"' RETURN
fn() { . ./inc; }
set -o functrace
fn`)
	// Two firings — the file's end and the function's own return — and the
	// first of them is already outside the file.
	if got := strings.Count(out, "R ("); got != 2 {
		t.Errorf("the trap fired %d times, want 2: %q", got, out)
	}
	if st != 0 {
		t.Fatalf("status = %d, output %q", st, out)
	}
	if !strings.Contains(out, "R (fn) ") {
		t.Errorf("the file's own frame stood through its RETURN trap: %q", out)
	}
	if strings.Contains(out, "R (source") {
		t.Errorf("the trap named the sourced file's frame: %q", out)
	}
}
