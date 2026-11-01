// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// posixDeclarationOptionStates is each way into or out of `posixbuiltins` the
// rows below are read under, with whether the option ends up on. The two
// `emulate` rows are the deciding pair: they hold the emulation fixed and move
// only the option, so a rule keyed on sh-ness rather than on the option fails
// one of them.
var posixDeclarationOptionStates = []struct {
	set string
	on  bool
}{
	{"", false},
	{"setopt posixbuiltins; ", true},
	{"setopt posixbuiltins; unsetopt posixbuiltins; ", false},
	{"emulate sh; unsetopt posixbuiltins; ", false},
	{"emulate csh; setopt posixbuiltins; ", true},
}

// posixDeclarationWords is the declaration family the option makes special,
// then `readonly`, which POSIX already makes special and so moves with the
// option exactly as they do, then `true`, a regular builtin that moves with
// nothing.
var posixDeclarationWords = []struct {
	word    string
	special bool
}{
	{"typeset", true},
	{"declare", true},
	{"integer", true},
	{"float", true},
	{"local", true},
	{"readonly", true},
	{"true", false},
}

// withoutRedirectionComplaints drops the line a failed redirection writes, so
// a row reads what ran after it and not the sentence, which is the same in
// every state.
func withoutRedirectionComplaints(out string) string {
	var kept []string
	for line := range strings.SplitSeq(out, "\n") {
		if line != "" && !strings.Contains(line, "no such file or directory") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, " ")
}

// **Under `posixbuiltins` a prefix in front of `typeset`, `declare`,
// `integer`, `float` or `local` persists**, as one in front of POSIX's own
// special builtins does (#7009). Measured 2026-10-30 on zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f -c` under `env -i PATH=/usr/bin:/bin`), every
// row of this table in every state.
//
// The second snippet is the one that says what the kept prefix *is*: inside a
// function it is an ordinary assignment, seen by a function called after it
// and still there when the function returns, and not the function's local.
// See interp.Semantics.PrefixToALocalDeclarationIsALocal.
func TestPosixBuiltinsKeepsAPrefixInFrontOfADeclaration(t *testing.T) {
	t.Parallel()
	for _, o := range posixDeclarationOptionStates {
		for _, w := range posixDeclarationWords {
			kept := o.on && w.special
			for _, c := range []struct{ src, off, on string }{
				{
					"v=old; v=new " + w.word + " y=1; print v=$v",
					"v=old", "v=new",
				},
				{
					"g() { print g=$v; }; f() { v=new " + w.word + " y=1; g; print in=$v; }; v=old; f; print out=$v",
					"g=old in=old out=old", "g=new in=new out=new",
				},
			} {
				want := c.off
				if kept {
					want = c.on
				}
				src := o.set + c.src
				t.Run(src, func(t *testing.T) {
					out, _ := runZsh(t, t.TempDir(), src)
					if got := withoutRedirectionComplaints(out); got != want {
						t.Errorf("%s\ngot  %q\nwant %q", src, got, want)
					}
				})
			}
		}
	}
}

// **And a redirection on one that cannot be made ends the script**, the
// other consequence of being special that this shell reaches: measured the
// same way on the same day. Every builtin name zsh has was swept for this row
// under the option, and these five are the only names beyond POSIX's list and
// the precommand words that end the script. Inside an `eval` in a subshell it
// ends the subshell, which is what the second snippet reads.
func TestPosixBuiltinsMakesARedirectionFailureOnADeclarationFatal(t *testing.T) {
	t.Parallel()
	for _, o := range posixDeclarationOptionStates {
		for _, w := range posixDeclarationWords {
			fatal := o.on && w.special
			for _, c := range []struct {
				src, off, on string
				offStatus    int
				onStatus     int
			}{
				{
					src: w.word + " y=1 >./no/x; print after",
					off: "after", on: "", offStatus: 0, onStatus: 1,
				},
				{
					src: "( eval '" + w.word + " y=1 >./no/x'; print alive ); print status=$?",
					off: "alive status=0", on: "status=1",
				},
			} {
				want, wantStatus := c.off, c.offStatus
				if fatal {
					want, wantStatus = c.on, c.onStatus
				}
				src := o.set + c.src
				t.Run(src, func(t *testing.T) {
					out, st := runZsh(t, t.TempDir(), src)
					if got := withoutRedirectionComplaints(out); got != want || st != wantStatus {
						t.Errorf("%s\ngot  %q at %d\nwant %q at %d", src, got, st, want, wantStatus)
					}
				})
			}
		}
	}
}
