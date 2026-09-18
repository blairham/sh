// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A history-style modifier list written straight after an **unbraced**
// expansion, which is one shell's and is the last thing in front of
// `git checkout <branch>` completing (#3127).
//
// Measured against zsh 5.9.2 on 2026-09-18 from a script file, and against
// bash 5.3.20, bash 3.2.57, bash as `sh`, ksh93u+, dash and BusyBox ash for
// the other side of it: `$s:q` is `p\ q` in zsh and the six characters
// `p q:q` in every one of the other six — which is what this grammar did
// before, so the flag is what one shell adds rather than what six lose.
//
// The failure was loud rather than silent, and that is why it mattered: the
// shipped `__git_commits` writes `sopts+=( $ropt:q )` with `$ropt` empty, so
// a literal `:q` traveled into a `compadd` argument list, ended the options
// there, and offered `-J`, `-a`, `-o`, `numeric`, `-default-` and `tags` as
// branch names.

// The rows are counted as fields rather than compared as text: `:q` puts a
// backslash in front of a blank, and a test that printed the words joined
// could not tell a quoted word from two words.
func TestAModifierAfterABareExpansion(t *testing.T) {
	dir := t.TempDir()
	const setup = "a=(); b=(x 'y z'); s='p q'; p=/a/bb/c.txt; " +
		"f() { print -r -- \"n=$# args=(${(j:|:)@})\" }; "
	for _, tc := range []struct{ name, src, want string }{
		// The three rows of the issue's own table.
		{"an empty array", "f $a:q", "n=0 args=()\n"},
		{"an array, one element with a blank in it", "f $b:q", "n=2 args=(x|y\\ z)\n"},
		{"a scalar", "f $s:q", "n=1 args=(p\\ q)\n"},
		// And the row that says this is the *bare* form and not modifiers in
		// general: braced, the `:q` is text in that shell too.
		{"braced, the colon is text", "f ${b}:q", "n=2 args=(x|y z:q)\n"},
		// The path modifiers, which are what the letters are mostly used for.
		{"tail", "f $p:t", "n=1 args=(c.txt)\n"},
		{"head", "f $p:h", "n=1 args=(/a/bb)\n"},
		{"root", "f $p:r", "n=1 args=(/a/bb/c)\n"},
		{"extension", "f $p:e", "n=1 args=(txt)\n"},
		{"chained", "f $p:t:r", "n=1 args=(c)\n"},
		{"a substitution", "f $p:s/a/Z/", "n=1 args=(/Z/bb/c.txt)\n"},
		// Case, which is the one pair that is also an attribute elsewhere.
		{"upper, per element", "f $b:u", "n=2 args=(X|Y Z)\n"},
		{"lower", "f ${${p:u}:l}", "n=1 args=(/a/bb/c.txt)\n"},
		// The bare form takes the **letter** and nothing after it, where the
		// braced form takes a count. The pair is the measurement; either row
		// alone reads as an accident.
		{"no count in the bare form", "f $p:h2", "n=1 args=(/a/bb2)\n"},
		{"a count in the braced form", "f ${p:h2}", "n=1 args=(/a)\n"},
		{"and nothing else after the letter", "f $s:qX", "n=1 args=(p\\ qX)\n"},
		// A colon that begins no modifier is left where it is, with no
		// complaint — which is what lets every other use of a colon after an
		// expansion go on working.
		{"a letter that names nothing", "f $s:zz", "n=1 args=(p q:zz)\n"},
		{"a bare colon", "f $s:", "n=1 args=(p q:)\n"},
		// A positional and a special take one too.
		{"a positional", "set -- 'A B'; f $1:q", "n=1 args=(A\\ B)\n"},
		{"the parameter count", "set -- 'A B'; f $#:q", "n=1 args=(1)\n"},
		// And a subscripted element, which is the expansion the shipped
		// completions write it after.
		{"after a subscript", "f $b[2]:q", "n=1 args=(y\\ z)\n"},
		// In an assignment, where the word is not a command's argument at
		// all.
		{"in an assignment", "x=$s:q; f $x", "n=1 args=(p\\ q)\n"},
		// The word still ends where it would have ended: an unterminated
		// substitution takes the rest of the *word*, not the rest of the
		// line.
		{"an unterminated substitution", "f $p:s/a/Z; f x", "n=1 args=(/Z/bb/c.txt)\nn=1 args=(x)\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := runZsh(t, dir, setup+tc.src)
			if got != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, got, st, tc.want)
			}
		})
	}
}

// What a modifier list is applied to is the fields the expansion has where it
// stands, and nothing the modifier decides.
//
// Measured with `b=(/a/x.c /b/y.c 'p q')`: `${b:t}` is three words with a
// tail each and `"${b:t}"` is `y.c p q` — one word, because the array joined
// before anything asked, and the tail of `/a/x.c /b/y.c p q` is everything
// after its last slash. `"${b[@]:t}"` keeps its three, which is what `[@]`
// means, and `"${b[*]:t}"` does not, which is what `[*]` means.
//
// The four rows together are the claim. Any one of them passes against an
// implementation that joins always or splits always.
func TestWhatAModifierListIsAppliedTo(t *testing.T) {
	dir := t.TempDir()
	const setup = "b=(/a/x.c /b/y.c 'p q'); " +
		"f() { print -r -- \"n=$# [${(j:|:)@}]\" }; "
	for _, tc := range []struct{ name, src, want string }{
		{"unquoted, a tail each", "f ${b:t}", "n=3 [x.c|y.c|p q]\n"},
		{"unquoted and bare, the same", "f $b:t", "n=3 [x.c|y.c|p q]\n"},
		{"quoted, one word and one tail", `f "${b:t}"`, "n=1 [y.c p q]\n"},
		{"quoted with [@], the fields survive", `f "${b[@]:t}"`, "n=3 [x.c|y.c|p q]\n"},
		{"quoted with [*], they do not", `f "${b[*]:t}"`, "n=1 [y.c p q]\n"},
		// An offset is still an offset, and a length after it still a
		// length: the reading is decided before any element is touched.
		{"an offset slices the list", "f ${b:1}", "n=2 [/b/y.c|p q]\n"},
		{"an offset and a length", "f ${b:1:2}", "n=2 [/b/y.c|p q]\n"},
		{"an offset and then a modifier", "f ${b:1:t}", "n=2 [y.c|p q]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := runZsh(t, dir, setup+tc.src)
			if got != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, got, st, tc.want)
			}
		})
	}
}
