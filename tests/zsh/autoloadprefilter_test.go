// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAFunctionFileThatIsOneDefinitionIsStillFound pins the shapes a look at
// a function file's text must not wave past before its parse (#6363): a file
// that is nothing but a definition of its own name runs that definition's
// body on the first call (#1704), and the look now decides from the first
// line that is neither blank nor a comment whether the parse is worth taking.
//
// So each row puts the definition somewhere that line could miss it — behind
// comments and blank lines, behind a shebang, behind a backslash that carries
// the statement on, spelled with `function` — and the last row is the control:
// a file whose first command is something else is not one definition, and
// its first call defines the function and prints nothing.
//
// Measured 2026-10-15 on zsh 5.9.2 (/opt/homebrew/bin/zsh, -f), each file in
// its own `$fpath` directory and called once under `autoload -Uz`:
// `ka:[A] kb:[B] kc:[C] kd:[D] ke:[] kf:[F] kg:[G]`.
func TestAFunctionFileThatIsOneDefinitionIsStillFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fns := filepath.Join(dir, "fns")
	if err := os.Mkdir(fns, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"ka": "# comment\n\nka() { print A }\n",
		"kb": "function kb {\n print B\n}\n",
		"kc": "function \\\nkc { print C }\n",
		"kd": "  kd () {\n print D\n}\n",
		"ke": "emulate -L zsh\nke() { print E }\n",
		"kf": "\\\nkf() { print F }\n",
		"kg": "#!/bin/zsh\n  # x\nkg() { print G; }\n",
	} {
		if err := os.WriteFile(filepath.Join(fns, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src := "fpath=(" + fns + " $fpath)\nautoload -Uz ka kb kc kd ke kf kg\n" +
		`for f in ka kb kc kd ke kf kg; do print -n "$f:[$($f)] "; done; print` + "\n"
	out, st := runZsh(t, dir, src)
	if want := "ka:[A] kb:[B] kc:[C] kd:[D] ke:[] kf:[F] kg:[G] \n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
