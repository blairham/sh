// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/repl"
)

// TestTheUndoLimitIsTheEditorsInteger: `UNDO_LIMIT_NO` reads the editor's
// limit, takes an arithmetic assignment as the integer it is, hands what it
// was given to the editor, and is gone outside the widget (#5898).
func TestTheUndoLimitIsTheEditorsInteger(t *testing.T) {
	r, out := zleRunner(t, `w() { print -r -- "was=$UNDO_LIMIT_NO t=${(t)UNDO_LIMIT_NO}"; UNDO_LIMIT_NO=1+2 }
zle -N w
`)
	ed := &stubEditor{limit: 4}
	_, ok, said, _ := runWidgetWatching(t, r, out, "w", repl.Line{}, ed)
	if want := "was=4 t=integer-local-special\n"; !ok || said != want {
		t.Errorf("ok=%v said %q, want %q", ok, said, want)
	}
	if ed.limit != 3 {
		t.Errorf("the editor's limit is %d, want 3", ed.limit)
	}
	if got, _ := runZshVars(t, r, `print -r -- "${UNDO_LIMIT_NO-unset}"; UNDO_LIMIT_NO=1+2; print -r -- $UNDO_LIMIT_NO`); got != "unset\n1+2\n" {
		t.Errorf("outside a widget: %q, want %q", got, "unset\n1+2\n")
	}
}
