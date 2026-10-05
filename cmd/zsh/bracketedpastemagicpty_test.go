// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path/filepath"
	"testing"
)

// The shipped bracketed-paste-magic, in the session the paste-key-by-key
// fixture was written to stand in for (#5880): the paste is drawn in the line
// as a paste, runs nothing, and runs when Return is pressed. Measured
// 2026-10-04 against zsh 5.9.2 with its own bracketed-paste-magic, the same
// rc and the same paste.
func TestAPasteThroughTheShippedBracketedPasteMagicIsTextInTheLine(t *testing.T) {
	shipped, err := filepath.Abs(filepath.Join("..", "..", "share", "sh", "functions"))
	if err != nil {
		t.Fatal(err)
	}
	pasteIsTextInTheLineUntilReturn(t, "fpath=("+shipped+" $fpath)\n"+
		"autoload -Uz bracketed-paste-magic\n"+
		"zle -N bracketed-paste bracketed-paste-magic\n", "true ")
}

// What it is for: a paste goes through the widget a typed key would, so with
// url-quote-magic as self-insert a pasted URL is quoted as a typed one is, and
// with the backward-extend-paste helper the URL already begun on the line is
// what the widget sees. Measured 2026-10-04 against zsh 5.9.2 with its own
// copies, the same startup file and the same keys.
func TestBracketedPasteMagicRunsAPasteThroughUrlQuoteMagic(t *testing.T) {
	control, screen, home := contribSession(t, `autoload -Uz bracketed-paste-magic url-quote-magic
zle -N bracketed-paste bracketed-paste-magic
zle -N self-insert url-quote-magic
`)
	contribSend(t, control, "x \x1b[200~curl http://x/?a=1&b\x1b[201~")
	if got, want := contribRecorded(t, control, screen, home, 1), `$'x curl http://x/\\?a\\=1\\&b'|25`; got != want {
		t.Errorf("after the paste: %q, want %q", got, want)
	}
}

// And with the backward-extend-paste helper, the start of a URL already on the
// line is handed to the widgets with the paste, so the part pasted after it is
// quoted too. Measured the same way.
func TestBackwardExtendPasteGivesTheWidgetsTheWholeWord(t *testing.T) {
	control, screen, home := contribSession(t, `autoload -Uz bracketed-paste-magic url-quote-magic
zle -N bracketed-paste bracketed-paste-magic
zle -N self-insert url-quote-magic
zstyle :bracketed-paste-magic paste-init backward-extend-paste
`)
	contribSend(t, control, "curl http://x\x1b[200~/?a\x1b[201~")
	if got, want := contribRecorded(t, control, screen, home, 1), `$'curl http://x/\\?a'|17`; got != want {
		t.Errorf("after the paste: %q, want %q", got, want)
	}
}
