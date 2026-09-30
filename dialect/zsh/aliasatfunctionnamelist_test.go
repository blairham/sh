// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// **A function definition is refused wherever alias expansion would rewrite
// one of its names** (#5236) — not only where the alias stands next to `()`.
//
// For a *global* alias that is any name in the list, since a global alias
// expands wherever a word stands. For a *regular* one it is the first name
// only, since a regular alias expands only where a command begins: `aa ga ()
// { … }` defines both, in both shells, because nothing rewrites that `ga`.
//
// Two things the code had wrong, one of them measured only by tracing it:
//
//   - A global alias was expanded as its word was **read**, before the
//     command-start check looked, so the check saw the expansion and never
//     the alias. `GA () { … }` defined `isafunc` in silence. The refusal is
//     asked in the global pass now, before it expands.
//   - The regular check asked for `()` directly after the word, so `ga aa ()
//     { … }` — the `()` two words away — was defined.
//
// Measured 2026-09-30 against zsh 5.9.2, each row a script file. The remark
// names the **first** alias in the list, once: `GA HB ()` draws one, on `GA`.
// And the question is whether a `()` follows, not whether what stands before
// it could make a real definition — `echo $x GA ()` is refused naming `GA`.
//
// With MULTI_FUNC_DEF off a name after the first is just an argument: `bb GA
// ()` is a plain parse error with no remark, while `GA ()` keeps its remark.
//
// Through the front end with a script file, because runZsh runs without the
// alias expansion a script gets and so cannot see any of this.
func TestAnAliasAnywhereInAFunctionNameListIsRefused(t *testing.T) {
	dir := t.TempDir()
	refused := func(name string) string {
		return "(eval):1: defining function based on alias `" + name + "'\n(eval):1: parse error near `()'\n"
	}
	for _, c := range []struct {
		name, src, out, err string
		code                int
	}{
		{
			"a global alone", "alias -g GA=isafunc\neval 'GA () { :; }'\nwhence -w isafunc\n",
			"isafunc: none\n", refused("GA"), 1,
		},
		// Not next to the `()`: the first of two names.
		{
			"a global first of two", "alias -g GA=isafunc\neval 'GA aa () { :; }'\nwhence -w isafunc\n",
			"isafunc: none\n", refused("GA"), 1,
		},
		{
			"a global in the middle of three", "alias -g GA=isafunc\neval 'aa GA bb () { :; }'\nwhence -w isafunc\n",
			"isafunc: none\n", refused("GA"), 1,
		},
		{
			// A later name with its parentheses attached, no blank before them.
			"a global before a name with attached ()", "alias -g GA=isafunc\neval 'GA aa() { :; }'\nwhence -w isafunc\n",
			"isafunc: none\n", refused("GA"), 1,
		},
		// The regular kind, first of two — the same gap in the older check.
		{
			"a regular alias first of two", "alias ga=isafunc\neval 'ga aa () { :; }'\nwhence -w isafunc\n",
			"isafunc: none\n", refused("ga"), 1,
		},
		// And second of two, where it is not rewritten and so not refused.
		{
			"a regular alias second of two", "alias ga=isafunc\neval 'aa ga () { :; }'\nwhence -w aa ga\n",
			"aa: function\nga: alias\n", "", 0,
		},
		// One remark to a definition, naming the first alias.
		{
			"two aliases, one remark", "alias -g GA=x; alias -g HB=y\neval 'GA HB () { :; }'\necho st=$?\n",
			"st=1\n", refused("GA"), 0,
		},
		{
			"a $word before it", "alias -g GA=x\neval 'echo $x GA () { :; }'\necho st=$?\n",
			"st=1\n", refused("GA"), 0,
		},
		{
			"MULTI_FUNC_DEF off, second name", "alias -g GA=isafunc\nsetopt nomultifuncdef\neval 'bb GA () { :; }'\necho st=$?\n",
			"st=1\n", "(eval):1: parse error near `()'\n", 0,
		},
		{
			"MULTI_FUNC_DEF off, alone", "alias -g GA=isafunc\nsetopt nomultifuncdef\neval 'GA () { :; }'\necho st=$?\n",
			"st=1\n", refused("GA"), 0,
		},
		// With MULTI_FUNC_DEF off only the command word can be a name, so the
		// list is not crossed from it either: `GA bb ()` is a plain parse
		// error there, and so is `ga bb ()`, where `GA ()` alone is refused.
		{
			"MULTI_FUNC_DEF off, a global first of two", "alias -g GA=isafunc\nsetopt nomultifuncdef\neval 'GA bb () { :; }'\necho st=$?\n",
			"st=1\n", "(eval):1: parse error near `()'\n", 0,
		},
		{
			"MULTI_FUNC_DEF off, a regular alias first of two", "alias ga=isafunc\nsetopt nomultifuncdef\neval 'ga bb () { :; }'\necho st=$?\n",
			"st=1\n", "(eval):1: parse error near `()'\n", 0,
		},
		// Whatever the scanner calls a word stands in the list, and a
		// redirection with its target: a byte scan that stopped at a `$`, a
		// quote or an operator gave all three up, and the reference refuses
		// each one.
		{
			"a $word in the list", "alias -g GA=x\neval 'GA $x () { :; }'\necho st=$?\n",
			"st=1\n", refused("GA"), 0,
		},
		{
			"a quoted ( in the list", "alias -g GA=x\neval \"GA 'a ( b' () { :; }\"\necho st=$?\n",
			"st=1\n", refused("GA"), 0,
		},
		{
			"a redirection in the list", "alias -g GA=x\neval 'GA aa >out () { :; }'\necho st=$?\n",
			"st=1\n", refused("GA"), 0,
		},
		// The controls: an argument still expands, and ALIAS_FUNC_DEF still
		// lets the definition through.
		{"a global as an argument", "alias -g GA=isafunc\necho GA\n", "isafunc\n", "", 0},
		{
			"ALIAS_FUNC_DEF on", "alias -g GA=isafunc\nsetopt aliasfuncdef\neval 'GA () { :; }'\nwhence -w isafunc\n",
			"isafunc: function\n", "", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, "case.sh")
			if err := os.WriteFile(path, []byte(c.src), 0o600); err != nil {
				t.Fatal(err)
			}
			var out, errs bytes.Buffer
			sh := zshShell()
			sh.Stdout, sh.Stderr = &out, &errs
			code := driver.MainArgs(sh, []string{"zsh", "-f", path})
			got := strings.ReplaceAll(errs.String(), dir, "<d>")
			if out.String() != c.out || got != c.err || code != c.code {
				t.Errorf("out %q err %q code %d, want out %q err %q code %d",
					out.String(), got, code, c.out, c.err, c.code)
			}
		})
	}
}
