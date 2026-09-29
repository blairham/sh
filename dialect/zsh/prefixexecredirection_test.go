// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An assignment prefix in front of `exec`, which this shell keeps — but not
// once the `exec` is carrying a redirection.
//
// `A04redirect.ztst` stops on "Assignment with exec used for redirection: no
// POSIX_BUILTINS", where `x=43; x=$(…) exec >test.log` must leave `x` at 43.
// This shell was keeping the value, because `exec` is on
// interp.Semantics.BuiltinsKeepingAnAssignmentPrefix and the roster was read
// as being about the *name* alone.
//
// Measured 2026-09-29 on zsh 5.9.2, the value read back **outside** the
// subshell so that no probe writes through a descriptor the command has
// moved — `( x=43; x=v CMD; print "x=$x" > result )` and then `cat result`.
// That shape matters: an earlier probe wrote to standard error, which
// `exec 2>out` had just redirected, and the row came back empty rather than
// wrong.
//
// **The rows hold the name fixed and move only the operands**, which is what
// says the redirection is the key rather than "a roster entry with something
// written after it": `builtin` is on the same roster and a redirection
// changes nothing for it.
func TestAPrefixOnExecGoesAwayOnceExecIsRedirecting(t *testing.T) {
	for _, tc := range []struct{ name, cmd, want string }{
		// The roster keeps it: nothing is written after the word.
		{"a bare exec keeps it", "exec", "x=v"},
		// The redirection form doing its other job does not.
		{"an output redirection takes it away", "exec >out", "x=43"},
		{"standard error likewise", "exec 2>out", "x=43"},
		{"a descriptor of its own likewise", "exec 3>out", "x=43"},
		{"and reading, not only writing", "exec <in", "x=43"},
		// The contrast, and the reason this is keyed on the redirection
		// form rather than on the word: `builtin` is on the same roster,
		// and a redirection changes nothing there because its redirection
		// does not outlive the command.
		{"a bare builtin keeps it", "builtin", "x=v"},
		{"and keeps it with a redirection", "builtin 2>out", "x=v"},
		// What moves `builtin` is a command word, which is the look-through
		// rule and not this one.
		{"but not with a command word", "builtin :", "x=43"},
		// The other two roster names, where operands change nothing at all.
		{"alias keeps it with operands", "alias foo=bar", "x=v"},
		{"hash keeps it with a redirection", "hash 2>out", "x=v"},
		// Off the roster entirely.
		{"the null command does not keep it", ":", "x=43"},
		{"nor does an ordinary builtin", "true", "x=43"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			// `$(<file)` rather than `cat`: the runner's PATH is the
			// temp directory, and an external command here would fail for
			// a reason that has nothing to do with the rule.
			src := "( x=43\n  x=v " + tc.cmd + "\n  print -r -- \"x=$x\" > result )\nprint -r -- \"$(<result)\"\n"
			out, st, _ := runZshSplit(t, dir, ": > in\n"+src)
			if out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q", out, st, tc.want+"\n")
			}
		})
	}
}

// The chunk itself, with its command substitution, which is **not** what
// makes it work: the substitution is there to show that it runs before the
// redirection takes effect, so its standard error reaches the terminal.
// `x=plain exec >log` behaves identically.
func TestTheChunkWithItsSubstitution(t *testing.T) {
	dir := t.TempDir()
	out, st, errs := runZshSplit(t, dir,
		"( x=43\n"+
			"  x=$(print This should appear, really >&2; print Not used) exec >test.log\n"+
			"  print -r -- x=$x)\n"+
			"print -r -- \"$(<test.log)\"\n")
	if out != "x=43\n" || st != 0 {
		t.Errorf("out %q status %d, want x=43 — the prefix does not survive", out, st)
	}
	// The third field, and the point of the substitution: it ran before the
	// redirection was in place, so its complaint went to the terminal and
	// not into the log.
	if errs != "This should appear, really\n" {
		t.Errorf("stderr %q, want the substitution's own line on the terminal", errs)
	}
}
