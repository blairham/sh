// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// An element written through a reference that is itself aimed at an element
// is refused here, naming the element as a name that is not one, and the
// array keeps nothing — the other column of
// interp.Semantics.ElementWriteThroughAnElementReferenceNests, where ksh93
// nests the element (#7113). Measured 2026-11-02 against bash 5.3.20 from `-c`
// under `env -i PATH=/usr/bin:/bin`, with the write on a line of its own,
// byte for byte.
func TestAnElementWriteThroughAnElementReferenceIsRefusedHere(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"a table's element",
			"declare -A a; declare -n e='a[x]'\ne[k]=1\ndeclare -p a e",
			"bash: line 2: `a[x]': not a valid identifier\ndeclare -A a\ndeclare -n e=\"a[x]\"\n",
		},
		{
			"an indexed array's element",
			"declare -a a; declare -n e='a[1]'\ne[2]=z\ndeclare -p a e",
			"bash: line 2: `a[1]': not a valid identifier\ndeclare -a a\ndeclare -n e=\"a[1]\"\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runBash(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// runBashCommandString runs src as `bash -c src` through the front end, both
// streams on one buffer, and returns what it wrote and the status it exited
// with — the command string's own, which is what a fatal refusal's 127 is.
func runBashCommandString(t *testing.T, src string) (string, int) {
	t.Helper()
	var buf bytes.Buffer
	sh := bashShell(&buf, &buf)
	sh.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	code := driver.MainArgs(sh, []string{"bash", "-c", src})
	return buf.String(), code
}

// And the refusal is the assignment statement's, so it gives up the rest of
// the line exactly as the bare `m=1` over an unaimed reference already did:
// the `echo` behind it on the same line never runs, the next line does, and
// on the last line the command string ends at the refusal's 1 (#7367). This
// shell wrote the sentence and carried on with the line.
//
// Measured 2026-11-02 against bash 5.3.20 (`/opt/homebrew/bin/bash`) from
// `-c` under `env -i PATH=/usr/bin:/bin` with a scratch HOME, byte for byte
// and status for status. The minimal pair is `e[k]+=1`, which gives the line
// up too, against `e+=1`, which has no subscript of the script's own, writes
// the element and runs on. The function row says the line given up is the
// one the call is on; the list row that the assignments behind it are given
// up with it; and the POSIX-mode row that the mode sharpens it into the
// shell's end, as it sharpens the control. The last two rows are routes that
// are not the statement and keep running on.
func TestARefusedElementWriteThroughAnElementReferenceGivesUpTheLine(t *testing.T) {
	t.Parallel()
	const table = `declare -A a; declare -n e='a[x]'; `
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			"the row: the rest of the line goes, the next line runs",
			table + "e[k]=1; echo \"after=$?\"\necho line2",
			"bash: line 1: `a[x]': not a valid identifier\nline2\n", 0,
		},
		{
			"on the last line the string ends at 1",
			table + `e[k]=1; echo "after=$?"`,
			"bash: line 1: `a[x]': not a valid identifier\n", 1,
		},
		{
			"an indexed array's element",
			"declare -a a; declare -n e='a[1]'; e[2]=z; echo \"after=$?\"\necho line2",
			"bash: line 1: `a[1]': not a valid identifier\nline2\n", 0,
		},
		{
			"the pair: an append with a subscript gives the line up too",
			table + "e[k]+=1; echo \"after=$?\"\necho line2; declare -p a",
			"bash: line 1: `a[x]': not a valid identifier\nline2\ndeclare -A a\n", 0,
		},
		{
			"the pair's other half: no subscript of its own writes the element",
			table + `e+=1; echo "after=$?"; declare -p a`,
			"after=0\ndeclare -A a=([x]=\"1\" )\n", 0,
		},
		{
			"through a second reference",
			table + "declare -n g=e; g[2]=1; echo \"after=$?\"\necho line2",
			"bash: line 1: `a[x]': not a valid identifier\nline2\n", 0,
		},
		{
			"in a function body, the line the call is on goes",
			table + "f() {\n e[k]=1; echo \"in=$?\"\n echo in2\n}\nf; echo \"afterf=$?\"\necho line2",
			"bash: line 2: `a[x]': not a valid identifier\nline2\n", 0,
		},
		{
			"the assignments behind it in the list are given up",
			table + "x=1 e[k]=1 y=2; echo \"after=$?\"\necho \"line2 x=$x y=$y\"",
			"bash: line 1: `a[x]': not a valid identifier\nline2 x=1 y=\n", 0,
		},
		{
			"the subscript's arithmetic has run first",
			table + "i=0; e[i++]=1; echo \"after=$?\"\necho \"line2 i=$i\"",
			"bash: line 1: `a[x]': not a valid identifier\nline2 i=1\n", 0,
		},
		{
			"POSIX mode ends the shell",
			"set -o posix; " + table + "e[k]=1; echo \"after=$?\"\necho line2",
			"bash: line 1: `a[x]': not a valid identifier\n", 127,
		},
		// The control, which already agreed: the bare reference-aim refusal.
		{
			"the control: a bare aim the reference refuses",
			"declare -n m; m=1; echo \"after=$?\"\necho line2",
			"bash: line 1: `1': not a valid identifier\nline2\n", 0,
		},
		// Routes that are not the statement, which run on.
		{
			"an arithmetic construct runs on",
			table + "(( e[k]=1 )); echo \"after=$?\"\necho line2",
			"bash: line 1: ((: `a[x]': not a valid identifier\nafter=0\nline2\n", 0,
		},
		{
			"an arithmetic write inside an assignment's value runs on",
			table + "x=$(( e[k]=1 )); echo \"x=$x\"\necho line2",
			"bash: line 1: `a[x]': not a valid identifier\nx=1\nline2\n", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, st := runBashCommandString(t, c.src)
			if out != c.want || st != c.status {
				t.Errorf("got %q at %d, want %q at %d", out, st, c.want, c.status)
			}
		})
	}
}

// The same give-up from a script file, where the line given up is the file's.
// Measured 2026-11-02 against bash 5.3.20 under `env -i PATH=/usr/bin:/bin`,
// byte for byte apart from the file's path (#7367). The second write is the
// reference aimed at an element of a name that does not exist, which the
// store refused in the same words before this.
func TestARefusedElementWriteThroughAnElementReferenceGivesUpTheFilesLine(t *testing.T) {
	t.Parallel()
	got := runBashFileCombined(t, "declare -A a; declare -n e='a[x]'; e[k]=1; echo \"after=$?\"\n"+
		"echo line2\n"+
		"typeset -n r='A[0]'; r[1]=v; echo \"after2=$?\"\n"+
		"echo line4\n")
	want := "f.sh: line 1: `a[x]': not a valid identifier\nline2\n" +
		"f.sh: line 3: `A[0]': not a valid identifier\nline4\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
