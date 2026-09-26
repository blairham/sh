// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// An `&&`/`||` list fires the DEBUG trap **once for the whole list** here,
// where bash and ksh93 fire it once per operand (#4556). This shell gave a
// list no rule of its own, so `print x && print y` wrote two firings against
// the reference's one, and every count in this file was over by the number of
// operators on the line.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` on it says *not a Go
// executable* — run `-f` with each snippet in a file of its own, so the
// `$LINENO` in the expectations is the file's own numbering.
//
// The action prints `$LINENO` alone rather than `$ZSH_DEBUG_CMD`, which is
// the parameter the reference's own single firing reads the whole list back
// in: nothing in this tree records it, so a row written around it would be
// asserting the absence of a feature rather than the count this is about.
func TestASublistFiresTheDebugTrapOnceForTheWholeList(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			// The issue's own reduction. Both operators, a three-operand
			// list, and a mixed one — one firing each, and the plain
			// command after them is the control that still fires once.
			name: "the operators",
			src: "trap 'print \"T@$LINENO\"' DEBUG\n" +
				"print x && print y\n" +
				"false || print z\n" +
				"print w && print v && print u\n" +
				"print p && false || print q\n" +
				"print tail\n",
			want: "T@2\nx\ny\nT@3\nz\nT@4\nw\nv\nu\nT@5\np\nq\nT@6\ntail\n",
		},
		{
			// The noun, which a count alone cannot pin down. Two lists on
			// **one line** write two firings and one list over **two lines**
			// writes one, at the first of them — so it is the list that
			// fires and not the line. A short-circuited list still fires
			// once, a negated operand does not change the count, and a list
			// whose operands are pipelines fires once for the list rather
			// than once per pipeline — two elements each, so the reading
			// that fires per pipeline would write two there.
			name: "the list and not the line",
			src: "trap 'print \"T@$LINENO\"' DEBUG\n" +
				"print a; print b\n" +
				"print c &&\n" +
				"  print d\n" +
				"false && print e\n" +
				"! print f && print g\n" +
				": | : && : | :\n" +
				"print end\n",
			want: "T@2\na\nT@2\nb\nT@3\nc\nd\nT@5\nT@6\nf\nT@7\nT@8\nend\n",
		},
		{
			// A list inside a compound is still a list: the head fires for
			// the construct, and the condition and the body each fire once
			// however many operands they hold. The loop's head repeats with
			// its passes and so does the body's list.
			name: "inside a compound",
			src: "trap 'print \"T@$LINENO\"' DEBUG\n" +
				"if print a && print b; then print c && print d; fi\n" +
				"for x in 1 2; do print $x && print y; done\n" +
				"case z in z) print n && print o;; esac\n",
			want: "T@2\nT@2\na\nb\nT@2\nc\nd\nT@3\nT@3\n1\ny\nT@3\n2\ny\nT@4\nT@4\nn\no\n",
		},
		{
			// And the two halves the count does not carry. An operand that
			// is a **compound** fires no head of its own — the group, the
			// subshell and the `if` on the last line each write nothing
			// where the same construct standing alone writes a head — while
			// commands **inside** one fire as they always do, the nested
			// lists on lines 2 and 3 included. A function called from an
			// operand fires its body's list at the body's own offset.
			name: "an operand that is a compound",
			src: "trap 'print \"T@$LINENO\"' DEBUG\n" +
				"print q && { print r && print s }\n" +
				"true && ( print t && print u )\n" +
				"f() { print v && print w }\n" +
				"true && f\n" +
				"print h && if true; then print i; fi\n",
			want: "T@2\nq\nT@2\nr\ns\nT@3\nT@3\nt\nu\nT@4\nT@5\nT@0\nv\nw\nT@6\nh\nT@6\nT@6\ni\n",
		},
		{
			// The count is orthogonal to `DEBUG_BEFORE_CMD`, which is
			// placement — so with every firing moved behind the command it
			// would have preceded, the list's single firing stands behind
			// the **whole list**, after everything its operands flushed.
			// Same number of firings as the same file run with the option
			// on, in the opposite order.
			name: "with the firings behind the command",
			src: "unsetopt DEBUG_BEFORE_CMD\n" +
				"trap 'print \"T@$LINENO\"' DEBUG\n" +
				"print q && { print r; print s }\n" +
				"if print a && print b; then print c; fi\n",
			want: "T@2\nq\nr\nT@3\ns\nT@3\nT@3\na\nb\nT@4\nc\nT@4\nT@4\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// A backgrounded list fires once, in the job — the route through
// interp/jobs.go rather than through a statement the shell runs itself.
//
// Counted through a file rather than read off the stream, because the job's
// own output races the `wait` that follows it and an ordering assertion would
// be a flake rather than a measurement. Three firings: the list's, made once
// in the job, and the ones for `wait` and for the `trap` that removes it.
//
// Measured on zsh 5.9.2, `-f`, 2026-09-25 — `n=3` there, and `n=4` from this
// shell before the list had a rule.
func TestABackgroundedSublistFiresOnceInTheJob(t *testing.T) {
	src := "trap 'print T >>log' DEBUG\n" +
		"print h && print i &\n" +
		"wait\n" +
		"trap - DEBUG\n" +
		"n=0\n" +
		"while read -r line; do n=$((n+1)); done < log\n" +
		"print \"n=$n\"\n"
	out, st := runZsh(t, t.TempDir(), src)
	if want := "h\ni\nn=3\n"; out != want || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, want)
	}
}

// And the axis says so, so a column flipped by hand fails here as well as in
// the rows above.
func TestSublistDebugAxis(t *testing.T) {
	if got, want := zsh.Semantics().DebugTrapSublists, interp.DebugTrapSublistOnceForTheList; got != want {
		t.Errorf("DebugTrapSublists = %v, want %v", got, want)
	}
}
