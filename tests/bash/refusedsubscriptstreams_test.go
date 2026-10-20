// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// An array literal whose *subscript* the shell refuses stores the elements
// placed before the refusal, and starts the name over first where the literal
// is not an append.
//
// A **stream** where the rest of that path is all-or-nothing, and the
// difference is the point: interp.Runner.failedHeading names three ways a
// heading can fail and only one of them is the element's own. Written here
// rather than in interp because the rows need the line to be given up and the
// script to carry on, which is Semantics.FailedExpansionAbandonsTheLine — the
// core ends the shell instead, so every row after the refusal would be unread.
//
// Measured 2026-09-23, one statement per line because the refusal ends the
// rest of a list, on bash 5.3.20, bash 5.3.15 and bash 3.2.57 — which agree on
// every row.
//
// **From a script file and not from `-c`**, which is the same distinction
// Runner.giveUpForABadSubscript is about: a bracketed expression gives a `-c`
// string up whole, so nothing after the refusal would run there and the rows
// would measure the give-up instead of the store. A file resumes at the next
// line, which is where the state the literal left behind is visible.
func TestARefusedSubscriptStoresWhatCameBeforeIt(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"an unset name becomes an empty array",
			"v=( [1+]=x )\n" + `declare -p v`,
			`declare -a v=()`,
		},
		{
			"an array the literal replaces is emptied",
			"v=(a b)\nv=( [1+]=x )\n" + `declare -p v`,
			`declare -a v=()`,
		},
		{
			"a scalar the literal replaces is emptied",
			"w=scalar\nw=( [1+]=x )\n" + `declare -p w`,
			`declare -a w=()`,
		},
		{
			// The row that says stream rather than clear: the element in
			// front of the refusal survives and the ones behind it do not.
			"the element in front of the refusal survives",
			"x=(a b)\nx=( [0]=z [1+]=bad [2]=q )\n" + `declare -p x`,
			`declare -a x=([0]="z")`,
		},
		{
			// Append keeps what the name held, because the stream starts
			// from the elements it is appending to.
			"an append keeps the elements it was adding to",
			"y=(a b)\ny+=( [1+]=bad )\n" + `declare -p y`,
			`declare -a y=([0]="a" [1]="b")`,
		},
		{
			// The load-bearing row for the *order*: the name is started over
			// after the words are expanded and not before, so the ordinary
			// spelling that reads its own old value still works.
			"the clear does not come before the words expand",
			"v=(a b)\n" + `v=( "${v[@]}" c )` + "\n" + `declare -p v`,
			`declare -a v=([0]="a" [1]="b" [2]="c")`,
		},
		{
			// And the guard #1568 put here is intact for a failure that is
			// not the subscript's: a fatal expansion stores nothing at all,
			// so the name keeps what it was holding.
			"a fatal expansion that is not a subscript stores nothing",
			"v=(a b)\n" + `v=( p $((1/0)) q )` + "\n" + `declare -p v`,
			`declare -a v=([0]="a" [1]="b")`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "case.sh")
			if err := os.WriteFile(path, []byte(c.src+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			var out, errs bytes.Buffer
			sh := bashShell(&out, &errs)
			sh.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
			_ = driver.MainArgs(sh, []string{"bash", path})
			// The diagnostic is the refusal's own and is not what this pins;
			// the last line of stdout is the state it left behind.
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			if got := lines[len(lines)-1]; got != c.want {
				t.Errorf("%s\nleft %q, want %q (stderr %q)", c.src, got, c.want, errs.String())
			}
		})
	}
}
