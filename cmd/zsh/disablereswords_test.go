// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDisableRSwitchesAGrammarWord is #5267. `disable -r` on a word of the
// grammar makes the text read after it read the word as an ordinary word, and
// `enable -r` puts it back. A function parsed before the change keeps its
// reading, and a subshell's change is its own. The text is read a line at a
// time, so these run as script files.
//
// Every row measured 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`),
// from a script file.
func TestDisableRSwitchesAGrammarWord(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			"disable -r foreach\nwhence -w foreach\nforeach() { echo F; }\nforeach x\ndisable -r if\nif() { echo IF; }\nif\nenable -r if\nif true; then echo back; fi\nwhence -w if\nprint -r -- ${(o)dis_reswords}",
			"foreach: none\nF\nIF\nback\nif: reserved\nforeach\n",
		},
		{"f() { if true; then echo inF; fi; }\ndisable -r if\nf", "inF\n"},
		{"disable -r fi\neval \"if true; then echo a; fi\"\necho st=$?", "(eval):1: parse error near `fi'\nst=1\n"},
		{"(disable -r if)\nif true; then echo ok; fi", "ok\n"},
		{"disable -r while foreach\ndisable -r\nprint -r -- $dis_reswords\nenable -r while\nprint -r -- $dis_reswords", "foreach\nwhile\nwhile foreach\nforeach\n"},
		{"disable -r select\nselect() { echo S; }\nselect", "S\n"},
		{"disable -r '!'\n! true; echo st=$?", "dd.zsh:2: command not found: !\nst=127\n"},
		// A word that ends a list is an ordinary command once it is off.
		{"disable -r fi done\nfi() { echo FI; }\nfi\n{ done; }", "FI\ndd.zsh:4: command not found: done\n"},
	} {
		dir := t.TempDir()
		t.Chdir(dir)
		if err := os.WriteFile(filepath.Join(dir, "dd.zsh"), []byte(tc.src+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := runZshMerged(t, "-f", "dd.zsh"); got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
