// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A function body is the same parsed words on every call, so whatever a call
// expands has to be expanded on a copy. `=cmd` rewrote the tree's own word,
// and the second call ran the path the first one resolved after PATH had
// changed under it (#6433). Measured 2026-10-15 on zsh 5.9.2 under `-f`, with
// a `tool` in each of two directories: `f() { echo =tool; }` prints the first
// directory's path, then the second's once PATH is pointed at it, and
// `functions f` still lists `echo =tool`. Every road `=cmd` takes is a row,
// because each of them wrote into the tree on its own.
func TestEqualsInABodyIsResolvedOnEveryCall(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		setup      func(*Runner)
	}{
		{"a command's word", `echo =zzq`, nil},
		{"a for loop's word", `for x in =zzq; do echo $x; done`, nil},
		{"an assignment's value", `x==zzq; echo $x`, nil},
		{"after a colon in a value", `x=a:=zzq; echo ${x#a:}`, nil},
		{"an array literal's element", `a=(=zzq); echo ${a[0]}`, nil},
		{"after a word's first equals", `echo y==zzq`, nil},
		{"under shfileexpansion", `echo =zzq`, func(r *Runner) { r.SetShFileExpansion(true) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := assignEqualsSem(Yes, Yes)
			out, st := withDirs(t,
				`f() { `+tc.body+`; }; f; PATH=$PWD/d2:$PATH; f`,
				func(d1, d2 string) {
					hashable(t, d1, "zzq", ":")
					hashable(t, d2, "zzq", ":")
				},
				func(r *Runner) {
					r.Semantics = &sem
					if tc.setup != nil {
						tc.setup(r)
					}
				})
			lines := strings.Split(strings.TrimSpace(out), "\n")
			if st != 0 || len(lines) != 2 ||
				!strings.HasSuffix(lines[0], "/d1/zzq") || !strings.HasSuffix(lines[1], "/d2/zzq") {
				t.Errorf("out=%q st=%d, want d1's path from the first call and d2's from the second", out, st)
			}
		})
	}
}

// The tilde an assignment's value holds is the other head that was written
// into the tree: a word's own tilde had been given a copy in #5155, and the
// value's road had not. Measured beside the rows above on zsh 5.9.2:
// `h() { x=~/q; y=a:~/r; }` called with HOME at /a and then /b reads /b the
// second time, and in bash 5.3 a subscript's `~` does the same.
func TestATildeInABodyIsExpandedOnEveryCall(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, before, body, want string }{
		{"a value's head", "", `x=~/q; echo $x`, "/a/q\n/b/q\n"},
		{"after a colon in a value", "", `x=c:~/q; echo $x`, "c:/a/q\nc:/b/q\n"},
		{"a subscript", "typeset -A m; ", `m[~]=1; echo "[${m[$HOME]}]"`, "[1]\n[1]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := permissive()
			sem.SubscriptKeyExpandsALeadingTilde = Yes
			out, st := run(t, tc.before+`f() { `+tc.body+`; }; HOME=/a; f; HOME=/b; f`, withSem(sem))
			if st != 0 || out != tc.want {
				t.Errorf("out=%q st=%d, want %q", out, st, tc.want)
			}
		})
	}
}
