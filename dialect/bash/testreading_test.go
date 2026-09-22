// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"strings"
	"syscall"
	"testing"
)

// What this shell's `test` makes of five shapes it was getting wrong, all
// measured on bash 5.3.20, 2026-09-22.
//
// One table, because each row is a whole `test` invocation and what it asserts
// is the *status* — the expression's own answer — or the one sentence written
// for it. A row that asserted only that something was refused would pass for a
// shell that refused everything.
func TestTestReadsFiveShapesTheWayThisShellDoes(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name, src, want string
	}{
		// `-R name` is whether the name is a reference. The `[[ -R r ]]`
		// spelling is the same operator and is grammar rather than an
		// operand, so it is #4228 rather than this change.
		{"a reference", "v=1\ndeclare -n r=v\ntest -R r; echo $?", "0"},
		{"its target", "v=1\ndeclare -n r=v\ntest -R v; echo $?", "1"},
		{"a name that is nothing", "test -R zz; echo $?", "1"},

		// An operator with nothing behind it at the end of a long expression
		// is the word it is spelled with.
		{"a trailing operator", "test -n xx -a -f; echo $?", "0"},
		{"a trailing -t", "test -n xx -a -t; echo $?", "0"},
		{"with a paren behind it", `test "(" -n xx -a -t ")"; echo $?`, "2"},

		// A group holding up to three words is read by the counts.
		{"a group of one", `test true -a "(" -n ")"; echo $?`, "0"},
		{"a group of two", `test true -a "(" ! -a ")"; echo $?`, "1"},
		{"a group of three", `[ \( x y z \) ]; echo $?`, "2"},
		{"a group of four", `test "(" -n xx -a -n ")"; echo $?`, "2"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBashPrelude(t, dir, "\n"+c.src)
			if got := strings.TrimSpace(lastLine(out)); got != c.want {
				t.Errorf("%s: status %q, want %q (output %q)", c.src, got, c.want, out)
			}
		})
	}
}

// And the four sentences those shapes write, which are what a person reads.
func TestTestWritesThisShellsRefusals(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ src, want string }{
		// A group the reading never closed names the parenthesis, and names
		// the word it found where there was one — for `[` that word is the
		// `]` the builtin took off the end.
		{`test "(" 1 = 2`, "bash: line 2: test: `)' expected"},
		{`[ "(" 1 = 2 ]`, "bash: line 2: [: `)' expected, found ]"},
		{`test "(" 1 = 2 junk`, "bash: line 2: test: `)' expected, found junk"},
		{`test x -a "(" y`, "bash: line 2: test: `)' expected"},
		// A leftover word spelled like an operator is a second sentence, and
		// one that is not spelled like one is the ordinary count.
		{`test 1 -ne 2 -ne 3`, "bash: line 2: test: syntax error: `-ne' unexpected"},
		{`test 1 -ne 2 -Q 3`, "bash: line 2: test: syntax error: `-Q' unexpected"},
		{`test a = b = c`, "bash: line 2: test: too many arguments"},
		{`test 1 -ne 2 ! 3`, "bash: line 2: test: too many arguments"},
		// The inner reading still wins where the group's content is short
		// enough for the counts to read it.
		{`[ \( -n x y \) ]`, "bash: line 2: [: x: binary operator expected"},
		{`[ \( x y \) ]`, "bash: line 2: [: x: unary operator expected"},
	} {
		out, st := runBashPrelude(t, dir, "\n"+c.src)
		if st == 0 {
			t.Errorf("%s: status 0, want a refusal", c.src)
		}
		wantWholeLines(t, out, c.want)
	}
}

// A path naming one of this shell's own descriptors is the file the shell has
// open there, not the process's Nth.
//
// The pair is two **different files at the same number**, which is what makes
// the row a discriminator without depending on what the process happens to
// have open: a named pipe answers `-p` and a regular file does not, and only
// the shell's own table can tell them apart at `/dev/fd/6`. Asking about a
// number the shell has nothing at would not be a claim about this shell —
// measured on a Linux runner, the test process really does hold a pipe at 6.
func TestAFileTestOnADescriptorPathReadsThisShellsTable(t *testing.T) {
	dir := t.TempDir()
	fifo := dir + "/pipe"
	// Made here rather than by the script: the run has no PATH to find
	// `mkfifo` on, and the fixture has to be a real named pipe for the row
	// to mean anything.
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("no named pipe on this machine: %v", err)
	}
	if err := os.WriteFile(dir+"/plain", []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st := runBashPrelude(t, dir, `
test -p `+fifo+`; echo "fifo=$?"
test -p `+dir+`/plain; echo "plain=$?"
exec 6<>`+fifo+`
test -p /dev/fd/6; echo "pipe-at-6=$?"
exec 6>&-
exec 6<`+dir+`/plain
test -p /dev/fd/6; echo "file-at-6=$?"
exec 6<&-`)
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	wantWholeLines(t, out, "fifo=0", "plain=1", "pipe-at-6=0", "file-at-6=1")
}
