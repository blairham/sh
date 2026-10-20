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

// **`setopt aliasfuncdef` lets an alias expand where a function is named**
// (#5235). With it on, `alias ga=isafunc` and then `ga() { … }` defines
// `isafunc`; with it off this dialect refuses the definition outright.
//
// The option was listed and answered `[[ -o ]]`, and moved nothing: the
// refusal came with it on exactly as with it off. Measured 2026-09-30
// against zsh 5.9.2, each row a script file. It is what `A02alias.ztst` stopped
// on, and the last of that file.
//
// **The timing row is the one that says how it reaches the grammar.** An option
// that moves the parser reaches the next line read, so `setopt aliasfuncdef;
// ga() { … }` on *one* line is still refused, in both shells — and the rows
// that show the option working are the ones where the definition is read after
// it: `eval`'d text, and the next line.
//
// Run through the front end with a script file rather than through runZsh,
// which runs without the alias expansion a script gets and so cannot see an
// alias at a function name at all.
func TestAliasFuncDefLetsAnAliasNameAFunction(t *testing.T) {
	dir := t.TempDir()
	const refused = "defining function based on alias `ga'\n"
	for _, c := range []struct {
		name, src, out, err string
		code                int
	}{
		{
			"on, in eval'd text",
			"alias ga=isafunc\nsetopt aliasfuncdef\neval 'ga() { print ran; }'\nisafunc\necho st=$?\n",
			"ran\nst=0\n", "", 0,
		},
		{
			"on, the definition on the next line",
			"alias ga=isafunc\nsetopt aliasfuncdef\nga() { print ran; }\nisafunc\necho st=$?\n",
			"ran\nst=0\n", "", 0,
		},
		{
			// The same line: read before the option took effect.
			"on, but set on the definition's own line",
			"alias ga=isafunc\nsetopt aliasfuncdef; ga() { print ran; }\nwhence -w isafunc\n",
			"", "<d>/case.sh:2: " + refused + "<d>/case.sh:2: parse error near `()'\n", 1,
		},
		{
			"on, then off again",
			"alias ga=isafunc\nsetopt aliasfuncdef\nunsetopt aliasfuncdef\neval 'ga() { print ran; }'\necho st=$?\n",
			"st=1\n", "(eval):1: " + refused + "(eval):1: parse error near `()'\n", 0,
		},
		{
			// `sh` emulation turns it on, through the same switch.
			"under emulate sh",
			"alias ga=isafunc\nemulate sh\neval 'ga() { echo ran; }'\nisafunc\necho st=$?\n",
			"ran\nst=0\n", "", 0,
		},
		{
			// The control: off by default, and refused.
			"off by default",
			"alias ga=isafunc\neval 'ga() { print ran; }'\necho st=$?\n",
			"st=1\n", "(eval):1: " + refused + "(eval):1: parse error near `()'\n", 0,
		},
		{
			// And the state was always right; only the effect was missing.
			"the option's state",
			"[[ -o aliasfuncdef ]]; echo st=$?\nsetopt aliasfuncdef; [[ -o aliasfuncdef ]]; echo st=$?\n",
			"st=1\nst=0\n", "", 0,
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
