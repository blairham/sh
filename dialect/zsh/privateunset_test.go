// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An `unset` in a callee takes the enclosing `private` declaration with it
// only when what that private displaced was the **shell's own** name (#5093).
//
// It took it in both cases here. The rule was measured at the script's own
// level, where the shell's own name and the innermost enclosing binding are
// the same thing and nothing could tell the two readings apart; one frame out
// they part, and the reference keeps the private.
//
// Measured 2026-09-28 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*, so the reference is that shell and not
// another build of this one), script files under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME and standard input on the null device.
const privUnset = `(){ private v=inner; (){ print -r -- "X [$v]"; unset v }; ` +
	`print -r -- "Y [$v]" }`

func TestAnUnsetInACalleeTakesThePrivateOnlyFromTheShellsOwnName(t *testing.T) {
	const mod = "zmodload zsh/param/private\n"
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		// **The two rows that decide it, and they are the same depth.** Only
		// what stands over the private moves: the shell's own name, or the
		// enclosing function's local. A rule written as "one frame in keeps
		// it" passes the second and fails the first.
		{
			"the shell's own name, one frame in",
			mod + "typeset v=top\nw(){ " + privUnset + " }\nw", "X [top]\nY []",
		},
		{
			"an enclosing function's local, same depth",
			mod + "w(){ typeset v=top; " + privUnset + " }\nw", "X [top]\nY [inner]",
		},
		// And the level the rule was first measured at, which is right and
		// stays right.
		{
			"the shell's own name at its own level",
			mod + "typeset v=top\n" + privUnset, "X [top]\nY []",
		},
		// A `local` in the enclosing function is the same displacement under
		// the other word.
		{
			"the enclosing local under the other word",
			mod + "w(){ local v=top; " + privUnset + " }\nw", "X [top]\nY [inner]",
		},
		// Deeper again: what matters is the first binding below the
		// declaration, not how many frames there are.
		{
			"two enclosing frames, the outer one holding it",
			mod + "w2(){ typeset v=top; w3(){ " + privUnset + " }; w3 }\nw2",
			"X [top]\nY [inner]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src+"\n"); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
}

// The controls, and they are what bound the rule to the **unset**.
//
// With nothing displaced there is no parameter for the callee to unset and
// the private stands — which was already right and is the branch's other
// guard. A callee that *writes* rather than unsets leaves the private alone
// in both. And the same shapes with no `private` anywhere answer identically,
// so none of this is about `unset` in a callee generally.
func TestWhatTheRuleDoesNotReach(t *testing.T) {
	const mod = "zmodload zsh/param/private\n"
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"nothing displaced, one frame in",
			mod + "w(){ " + privUnset + " }\nw", "X []\nY [inner]",
		},
		{
			"nothing displaced, at the shell's level",
			mod + privUnset, "X []\nY [inner]",
		},
		{
			"the callee writes instead of unsetting",
			mod + "w(){ typeset v=top; (){ private v=inner; (){ v=written }; " +
				`print -r -- "Y [$v]" } }` + "\nw", "Y [inner]",
		},
		{
			"and with no private anywhere at all",
			`w(){ typeset v=top; (){ unset v }; print -r -- "Y [$v] ${+v}" }` + "\nw",
			"Y [] 0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src+"\n"); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
}

// And it is neither about the callee's spelling nor about the private's kind,
// which is worth rows of its own because the suite test this was found in is
// named for one of them.
//
// That file calls it `privates are not visible in anonymous functions`, and a
// rule read off the name would be keyed on the wrong noun: a **named** callee
// does the same, and an **association** does the same. All four combinations
// answer alike, and what moves the answer is what the private displaced.
func TestItIsNeitherTheCalleesSpellingNorThePrivatesKind(t *testing.T) {
	const mod = "zmodload zsh/param/private\n"
	const named = "g(){ print -r -- \"X [$v]\"; unset v }\n" +
		`(){ private v=inner; g; print -r -- "Y [$v]" }`
	const assoc = `(){ local -PA v=(k inner); (){ print -r -- "X ${(kv)v}"; unset v }; ` +
		`print -r -- "Y ${(kv)v}" }`
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"a named callee, over an enclosing local",
			mod + "w(){ typeset v=top; " + named + " }\nw", "X [top]\nY [inner]",
		},
		{
			"a named callee, over the shell's own",
			mod + "typeset v=top\nw(){ " + named + " }\nw", "X [top]\nY []",
		},
		{
			"an association, over an enclosing local",
			mod + "w(){ typeset -A v=(k top); " + assoc + " }\nw", "X k top\nY k inner",
		},
		{
			"an association, over the shell's own",
			mod + "typeset -A v=(k top)\nw(){ " + assoc + " }\nw", "X k top\nY ",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src+"\n"); out != tc.want+"\n" || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want+"\n")
			}
		})
	}
}
