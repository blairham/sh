// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// answeringCountedReads sets the two axes a `read -n` asks in #7179, leaving
// the rest of the test vector alone.
func answeringCountedReads(findsItsEnd, sticks Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.ReadCountOfAPipeExecOpenedFindsItsEnd = findsItsEnd
		s.ReadCountAtTheEndSticksTheDescriptor = sticks
		r.Semantics = &s
	}
}

// TestACountedReadOfAPipeAsksTheDialect: a `read -n` of a pipe at a number
// `exec` put there asks ReadCountOfAPipeExecOpenedFindsItsEnd. Yes answers 1
// with the name empty and leaves the stream as it was; no reads the count.
// The controls are the same read of the pipe by another route — the read's
// own redirection, standard input, `-N` — and of a regular file, and answer
// the same under both values, so it is the pipe at `exec`'s number that asks.
// The other axis is held at no, so what follows the 1 is a plain read.
func TestACountedReadOfAPipeAsksTheDialect(t *testing.T) {
	t.Parallel()
	const line = `printf 'abcdef\nxyz\n' | `
	for _, tc := range []struct{ name, src, no, yes string }{
		{
			"exec's copy of a pipe",
			line + `{ exec 3<&0; read -r -n 4 -u3 l; echo "st=$? l=[$l]"; read -r -u3 m; echo "m=[$m]"; }`,
			"st=0 l=[abcd]\nm=[ef]\n", "st=1 l=[]\nm=[abcdef]\n",
		},
		{
			"a count past the line",
			line + `{ exec 4<&0; l=x; read -r -n 10 -u4 l; echo "st=$? l=[$l]"; read -r -u4 m; echo "m=[$m]"; }`,
			"st=0 l=[abcdef]\nm=[xyz]\n", "st=1 l=[]\nm=[abcdef]\n",
		},

		{
			"the read's own redirection",
			line + `{ read -r -n 4 -u3 l 3<&0; echo "st=$? l=[$l]"; }`,
			"st=0 l=[abcd]\n", "st=0 l=[abcd]\n",
		},
		{
			"exec's number restated by the read",
			line + `{ exec 3<&0; read -r -n 4 -u3 l 3<&3; echo "st=$? l=[$l]"; }`,
			"st=0 l=[abcd]\n", "st=0 l=[abcd]\n",
		},
		{
			"standard input",
			line + `{ read -r -n 4 -u0 l; echo "st=$? l=[$l]"; }`,
			"st=0 l=[abcd]\n", "st=0 l=[abcd]\n",
		},
		{
			"an exact count",
			line + `{ exec 3<&0; read -r -N 4 -u3 l; echo "st=$? l=[$l]"; }`,
			"st=0 l=[abcd]\n", "st=0 l=[abcd]\n",
		},
		{
			"a regular file",
			`printf 'abcdef\n' >f; exec 3<f; read -r -n 4 -u3 l; echo "st=$? l=[$l]"`,
			"st=0 l=[abcd]\n", "st=0 l=[abcd]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := run(t, tc.src, answeringCountedReads(No, No)); out != tc.no || st != 0 {
				t.Errorf("no: got %q (status %d), want %q", out, st, tc.no)
			}
			if out, st := run(t, tc.src, answeringCountedReads(Yes, No)); out != tc.yes || st != 0 {
				t.Errorf("yes: got %q (status %d), want %q", out, st, tc.yes)
			}
		})
	}
	src := line + `{ exec 3<&0; l=x; read -r -n 4 -u3 l; echo "st=$? [$l]"; read -r -u3 m; echo "[$m]"; }`
	out, _ := run(t, src, answeringCountedReads(Unspecified, No))
	if !strings.Contains(out, "a `read -n` of a pipe `exec` opened finding it at its end: "+shellsDisagree) ||
		!strings.HasSuffix(out, "st=2 [x]\n[abcdef]\n") {
		t.Errorf("got %q, want the refusal naming the question, l untouched and the line still there", out)
	}
}

// TestACountedReadAtTheEndAsksTheDialect: a `read -n` that finds the end of
// its input before a character asks ReadCountAtTheEndSticksTheDescriptor.
// Yes leaves the number stuck, so a later read of it answers 0 with its names
// empty and takes nothing, even with a line appended since; no leaves the
// later read to read on. The controls — the number opened again, a copy of
// it, `-N`, a plain read and a short count at the end — answer the same
// under both values.
func TestACountedReadAtTheEndAsksTheDialect(t *testing.T) {
	t.Parallel()
	const stuck = `: >f; exec 3<f; read -r -n 4 -u3 l; echo 'a b' >>f; `
	for _, tc := range []struct{ name, src, no, yes string }{
		{
			"standard input at its end",
			`: | { read -r -n 4 l; echo "st=$?"; read -r m; echo "st=$? m=[$m]"; }`,
			"st=1\nst=1 m=[]\n", "st=1\nst=0 m=[]\n",
		},
		{
			"a line written since",
			stuck + `read -r -u3 m; echo "st=$? m=[$m]"; read -r -u3 m; echo "st=$? m=[$m]"`,
			"st=0 m=[a b]\nst=1 m=[]\n", "st=0 m=[]\nst=0 m=[]\n",
		},
		{
			"every name",
			stuck + `x=X y=Y; read -r -u3 x y; echo "st=$? [$x][$y]"`,
			"st=0 [a][b]\n", "st=0 [][]\n",
		},
		{
			"a counted read after it",
			stuck + `read -r -N 2 -u3 m; echo "st=$? m=[$m]"`,
			"st=0 m=[a ]\n", "st=0 m=[]\n",
		},

		{
			"the number opened again",
			stuck + `exec 3<f; read -r -u3 m; echo "st=$? m=[$m]"`,
			"st=0 m=[a b]\n", "st=0 m=[a b]\n",
		},
		{
			"a copy of the number",
			stuck + `exec 4<&3; read -r -u4 m; echo "st=$? m=[$m]"`,
			"st=0 m=[a b]\n", "st=0 m=[a b]\n",
		},
		{
			"an exact count at the end",
			`: >f; exec 3<f; read -r -N 4 -u3 l; echo abc >>f; read -r -u3 m; echo "st=$? m=[$m]"`,
			"st=0 m=[abc]\n", "st=0 m=[abc]\n",
		},
		{
			"a plain read at the end",
			`: >f; exec 3<f; read -r -u3 l; echo abc >>f; read -r -u3 m; echo "st=$? m=[$m]"`,
			"st=0 m=[abc]\n", "st=0 m=[abc]\n",
		},
		{
			"a short count",
			`printf ab >f; exec 3<f; read -r -n 4 -u3 l; read -r -u3 m; echo "st=$? m=[$m]"`,
			"st=1 m=[]\n", "st=1 m=[]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := run(t, tc.src, answeringCountedReads(No, No)); out != tc.no || st != 0 {
				t.Errorf("no: got %q (status %d), want %q", out, st, tc.no)
			}
			if out, st := run(t, tc.src, answeringCountedReads(No, Yes)); out != tc.yes || st != 0 {
				t.Errorf("yes: got %q (status %d), want %q", out, st, tc.yes)
			}
		})
	}
	src := `: | { read -r -n 4 l; echo "st=$?"; read -r m; echo "st=$?"; }`
	out, _ := run(t, src, answeringCountedReads(No, Unspecified))
	if !strings.Contains(out, "a `read -n` that found the end sticking the descriptor it read: "+shellsDisagree) ||
		!strings.HasSuffix(out, "st=2\nst=1\n") {
		t.Errorf("got %q, want the refusal naming the question at 2 and the next read at the end", out)
	}
}

// TestBothCountedReadAxesTogether is the shape #7179 was found in: the pipe
// found at its end under a count, and the number stuck after it, so the line
// is in the pipe and no read of 3 reaches it — but one of standard input does.
func TestBothCountedReadAxesTogether(t *testing.T) {
	t.Parallel()
	src := `printf 'abcdef\nxyz\n' | { exec 3<&0; read -r -n 4 -u3 l; echo "st=$? l=[$l]"; ` +
		`read -r -u3 m; echo "st=$? m=[$m]"; read -r m <&3; echo "st=$? m=[$m]"; }`
	want := "st=1 l=[]\nst=0 m=[]\nst=0 m=[abcdef]\n"
	if out, st := run(t, src, answeringCountedReads(Yes, Yes)); out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
