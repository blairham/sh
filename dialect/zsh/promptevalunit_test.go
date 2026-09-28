// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// `%N` names the unit being read, and text an `eval` is running is a unit of
// its own called `(eval)` — #5017.
//
// Measured 2026-09-28 on zsh 5.9.2 with `-f`. It reaches two surfaces from
// one value: a written `%N`, and **the default trace prefix**, since this
// dialect's `PS4` is `+%N:%i> ` and is drawn through the same table.
//
// Four of the rows are controls and each rules out a wrong shape of the fix:
//
//   - a **named function** called from an eval is `f`, so entering a function
//     leaves the eval behind and this is not "anything below an eval";
//   - an **anonymous** function is `(anon)` and a top-level `%N` is the
//     shell's name, both of which were already right;
//   - `%i` inside the eval is unchanged, so the label moved and the line did
//     not;
//   - and **`unsetopt evallineno`** puts it back to the shell's name in the
//     reference, which is the row that says the label follows the option that
//     makes eval text a place of its own rather than following the construct.
//
// The two multi-line rows carry line 2 rather than line 1 and that is the
// snippet's own shape, not a second rule: measured in the harness's form,
// `zsh -f -c $'setopt xtrace\neval "true"'` is `+zsh:2> eval true` in the
// reference as well.
func TestThePromptUnitNameIsTheEvalText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{"an eval is a unit of its own", `eval "print -P %N"` + "\n", "(eval)\n"},
		{"and so is a nested one", `eval "eval \"print -P %N\""` + "\n", "(eval)\n"},
		{"an eval inside a function wins over the function", "f() { eval \"print -rP '[%N][%i]'\" }\nf\n", "[(eval)][1]\n"},
		{"a function called from an eval is the function", "f() { print -rP \"[%N][%i]\" }\neval \"f\"\n", "[f][0]\n"},
		{"an anonymous function is unchanged", "() { print -P %N }\n", "(anon)\n"},
		{"the top level is unchanged", "print -P %N\n", "zsh\n"},
		{"the line inside the eval is unchanged", `eval "print -P %i"` + "\n", "1\n"},
		{
			"and the option that makes eval text a place decides it",
			"unsetopt evallineno\neval \"print -rP '[%N][%i]'\"\n",
			"[zsh][2]\n",
		},
		{
			"the trace prefix is drawn from the same value",
			"setopt xtrace\neval \"true\"\n",
			"+zsh:2> eval true\n+(eval):1> true\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out = %q status = %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The same label on the routes that have a file, where the enclosing unit is
// a name rather than the shell's — which is where this was wrong in the way
// that mattered, since a script's `%N` answered the script's own path inside
// an eval and so never said `(eval)` at all.
func TestThePromptUnitNameIsTheEvalTextInAFileToo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := "print -rP \"plain:[%N]\"\neval \"print -rP 'in-eval:[%N]'\"\n"
	if err := os.WriteFile(filepath.Join(dir, "s.zsh"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, src, want string }{
		{"a script file", "source ./s.zsh\n", "plain:[./s.zsh]\nin-eval:[(eval)]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out = %q status = %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
