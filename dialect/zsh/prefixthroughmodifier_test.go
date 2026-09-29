// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An assignment prefix stands in front of the command the **modifiers
// resolve to**, and which modifier was written still matters for exactly one
// word.
//
// Measured 2026-09-29 on zsh 5.9.2 with `posixbuiltins` on, the value read
// back outside a subshell through a file the command has not touched:
//
//	x=v builtin :      x=v     looked through to `:`, which is special
//	x=v noglob :       x=v     the same, and was already right here
//	x=v command :      x=43    `command` is the word that stops it
//	x=v builtin true   x=43    looked through to `true`, which is not
//
// `noglob` was already right because the front end takes it away before the
// prefix rules see it. **`builtin` stays in the words** — it is a builtin as
// well as a modifier — so the builtin lookup shadowed the look-through and
// the prefix was read as standing in front of `builtin` itself.
//
// That made the two halves of the decision disagree one line apart:
// prefixRosterName already looked through the modifiers, so `builtin :`
// asked the roster about `:` and the persistence axis about `builtin`, and
// neither answered for the command that was going to run.
func TestAPrefixStandsInFrontOfWhatTheModifiersResolveTo(t *testing.T) {
	for _, tc := range []struct{ name, cmd, want string }{
		{"a transparent modifier is looked through", "builtin :", "x=v"},
		{"and so is the one that is not a builtin", "noglob :", "x=v"},
		{"to a command that is not special, it is not kept", "builtin true", "x=43"},
		// The one word that stops the look-through, and the axis that says
		// so: interp.Semantics.CommandKeepsASpecialBuiltinsPrefix is No here.
		{"`command` stops it even in front of a special builtin", "command :", "x=43"},
		{"and in front of an ordinary one", "command true", "x=43"},
		// Chained modifiers resolve the same way.
		{"two modifiers in a row", "noglob builtin :", "x=v"},
		{"and one that resolves to nothing special", "noglob builtin true", "x=43"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, _ := runZshSplit(t, t.TempDir(),
				"setopt posixbuiltins\n( x=43\n  x=v "+tc.cmd+"\n"+
					"  print -r -- \"x=$x\" > result )\nprint -r -- \"$(<result)\"\n")
			if out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q", out, st, tc.want+"\n")
			}
		})
	}
}

// Without the option the same modifiers resolve the same way and the answer
// is the dialect's own, which is what keeps the rows above about the
// resolution rather than about the option.
//
// A plain zsh does not persist a prefix in front of a special builtin, so
// every row here is transient except the ones its own roster keeps — and
// `builtin` with nothing behind it is one of those, which is why the bare
// word is not in the table above.
func TestWithoutTheOptionTheResolutionIsUnchanged(t *testing.T) {
	for _, tc := range []struct{ name, cmd, want string }{
		{"a special builtin is not kept here", "builtin :", "x=43"},
		{"nor an ordinary one", "builtin true", "x=43"},
		{"nor through `command`", "command :", "x=43"},
		{"and the bare modifier is kept, as it is either way", "builtin", "x=v"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, _ := runZshSplit(t, t.TempDir(),
				"( x=43\n  x=v "+tc.cmd+"\n  print -r -- \"x=$x\" > result )\nprint -r -- \"$(<result)\"\n")
			if out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q", out, st, tc.want+"\n")
			}
		})
	}
}
