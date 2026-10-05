// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"slices"
	"testing"

	"github.com/blairham/sh/repl"
)

// `read-command` reads one key sequence and names the widget it is bound to
// without running it (#5880).
//
// Measured 2026-10-04 through a pseudo-terminal against zsh 5.9.2, a widget
// pushing keys with `zle -U` and calling `zle .read-command` once per key:
//
//	a        self-insert          ^A      beginning-of-line
//	\e[A     up-line-or-history   \eb     backward-word
//	^X q     undefined-key, $KEYS ^Xq — the prefix and the byte after it
//	^X ^U    undo
//	é        self-insert twice, a byte in $KEYS each time, with no LANG
//
// every one at status 0, and `$LASTWIDGET` reads `.read-command` afterwards.
// A read with nothing left to read is the failure.
func TestReadCommandNamesTheWidgetAKeyWouldRun(t *testing.T) {
	r, out := zleRunner(t, `w() {
  while zle .read-command; do
    print -r -- "$REPLY ${(q)KEYS}"
  done
  zle .read-command
  print -r -- "end=$? last=$LASTWIDGET"
}
zle -N w
`)
	ed := &stubEditor{input: []byte("a\x01\x1b[A\x1bb\x18q\x18\x15\xc3\xa9")}
	_, ok, printed, _ := runWidgetWatching(t, r, out, "w", repl.Line{}, ed)
	if !ok {
		t.Fatal("the widget did not run")
	}
	want := "self-insert a\n" +
		"beginning-of-line $'\\001'\n" +
		"up-line-or-history $'\\033'\\[A\n" +
		"backward-word $'\\033'b\n" +
		"undefined-key $'\\030'q\n" +
		"undo $'\\030'$'\\025'\n" +
		"self-insert $'\\303'\n" +
		"self-insert $'\\251'\n" +
		"end=1 last=.read-command\n"
	if printed != want {
		t.Errorf("printed\n%s\nwant\n%s", printed, want)
	}
}

// Under a UTF-8 locale a character of more than one byte is one key: `$KEYS`
// holds all of it and it types itself (#5949). Measured 2026-10-04 through a
// pseudo-terminal against zsh 5.9.2, a pushed `é` read once under
// LANG=en_US.UTF-8 and twice, a byte at a time, under LANG=C. A widget that
// strips each key off a copy of the text never gets past half a character,
// which is how bracketed-paste-magic's loop hung on a paste holding one.
func TestReadCommandReadsAWholeCharacterInAUTF8Locale(t *testing.T) {
	for _, tc := range []struct{ lang, want string }{
		{"en_US.UTF-8", "self-insert é\nself-insert z\nrest=\n"},
		{"C", "self-insert $'\\303'\nself-insert $'\\251'\nrest=\n"},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			r, out := zleRunner(t, "LANG="+tc.lang+`
w() {
  local text=$'\303\251z'
  zle .read-command; print -r -- "$REPLY ${(q)KEYS}"; text=${text#"$KEYS"}
  zle .read-command; print -r -- "$REPLY ${(q)KEYS}"; text=${text#"$KEYS"}
  print -r -- "rest=${text#z}"
}
zle -N w
`)
			ed := &stubEditor{input: []byte("\xc3\xa9z")}
			_, _, printed, _ := runWidgetWatching(t, r, out, "w", repl.Line{}, ed)
			if printed != tc.want {
				t.Errorf("printed %q, want %q", printed, tc.want)
			}
		})
	}
}

// A key that is complete and also the start of a longer binding is read on
// only where another byte is already in hand, and a byte read on that leads
// nowhere goes back for the next read.
func TestReadCommandGivesBackWhatItReadPastAKey(t *testing.T) {
	r, out := zleRunner(t, `bindkey '^Xa' beginning-of-line
bindkey '^Xab' end-of-line
w() {
  zle .read-command; print -r -- "$REPLY ${(q)KEYS}"
  zle .read-command; print -r -- "$REPLY ${(q)KEYS}"
}
zle -N w
`)
	ed := &stubEditor{input: []byte("\x18az")}
	_, _, printed, _ := runWidgetWatching(t, r, out, "w", repl.Line{}, ed)
	if want := "beginning-of-line $'\\030'a\nself-insert z\n"; printed != want {
		t.Errorf("printed %q, want %q", printed, want)
	}
}

// `split-undo` ends the change the line is in, which is the editor's to do,
// and is status 0 with or without an argument — measured against zsh 5.9.2.
func TestSplitUndoClosesTheEditorsChange(t *testing.T) {
	r, out := zleRunner(t, "w() { zle .split-undo; print -r -- st=$?; zle split-undo x; print -r -- st=$? }\nzle -N w\n")
	ed := &stubEditor{changes: 7}
	_, _, printed, _ := runWidgetWatching(t, r, out, "w", repl.Line{}, ed)
	if printed != "st=0\nst=0\n" {
		t.Errorf("printed %q, want two zero statuses", printed)
	}
	if ed.changesAsked != 2 {
		t.Errorf("the editor was asked to close a change %d times, want 2", ed.changesAsked)
	}
}

// `$UNDO_CHANGE_NO` is the editor's number, `integer-local-readonly-special`
// as zsh 5.9.2 calls it, and `zle undo N` hands N back to the editor —
// read as a number is read here, so a word that is not one is 0, which
// measured is what `zle .undo abc` does.
func TestTheUndoNumberRoundTripsThroughTheEditor(t *testing.T) {
	r, out := zleRunner(t, `w() {
  print -r -- "n=$UNDO_CHANGE_NO t=${(t)UNDO_CHANGE_NO}"
  zle .undo $UNDO_CHANGE_NO; print -r -- st=$?
  zle .undo abc
}
zle -N w
`)
	ed := &stubEditor{changes: 5}
	_, _, printed, _ := runWidgetWatching(t, r, out, "w", repl.Line{}, ed)
	if want := "n=5 t=integer-local-readonly-special\nst=0\n"; printed != want {
		t.Errorf("printed %q, want %q", printed, want)
	}
	if want := []int{5, 0}; !slices.Equal(ed.undoneTo, want) {
		t.Errorf("undone to %v, want %v", ed.undoneTo, want)
	}
	if len(ed.performed) != 0 {
		t.Errorf("a numbered undo was performed as a plain one: %v", ed.performed)
	}
}

// `$KEYS` is the keystroke the widget is running for: measured against zsh
// 5.9.2, `scalar-local-readonly-special`, and `$'\024'` from a widget on `^T`.
func TestKeysIsTheKeystrokeTheWidgetIsFor(t *testing.T) {
	r, out := zleRunner(t, "w() { print -r -- \"${(t)KEYS} ${(q)KEYS}\" }\nzle -N w\n")
	_, _, printed, _ := runWidgetWatching(t, r, out, "w", repl.Line{Keys: "\x14"}, &stubEditor{})
	if want := "scalar-local-readonly-special $'\\024'\n"; printed != want {
		t.Errorf("printed %q, want %q", printed, want)
	}
}

// `zle -K` names the keymap the keys after it are read in. Measured against
// zsh 5.9.2 inside a widget: `-K main` leaves `$KEYMAP` reading `main` at
// status 0, a keymap that is not there is status 1 and not a word, and the
// count is refused in two sentences. Moving the editor between its two
// dispatches is not built, and says so rather than reporting it done.
func TestSelectingAKeymapFromAWidget(t *testing.T) {
	r, out := zleRunner(t, `w() {
  zle -K main; print -r -- "st=$? $KEYMAP"
  zle -K emacs; print -r -- "st=$? $KEYMAP"
  zle -K nosuch; print -r -- "st=$? $KEYMAP"
  zle -K; print -r -- "st=$?"
  zle -K main emacs; print -r -- "st=$?"
  zle -K vicmd; print -r -- "st=$? $KEYMAP"
}
zle -N w
`)
	_, _, printed, _ := runWidgetWatching(t, r, out, "w", repl.Line{}, &stubEditor{})
	want := "st=0 main\n" +
		"st=0 emacs\n" +
		"st=1 emacs\n" +
		"w:zle:4: not enough arguments for -K\nst=1\n" +
		"w:zle:5: too many arguments for -K\nst=1\n" +
		"w:zle:6: -K: changing to the vicmd keymap is not implemented yet\nst=1 emacs\n"
	if printed != want {
		t.Errorf("printed\n%s\nwant\n%s", printed, want)
	}
	if out, st := runZsh(t, t.TempDir(), "zle -K main\n"); out != "zsh:zle:1: can only be called from widget function\n" || st != 1 {
		t.Errorf("outside a widget: %q status %d", out, st)
	}
}

// A lone `-` ends the options and is taken off with them, and `zle` with
// nothing to do asks whether widgets can be called from here. Measured
// against zsh 5.9.2: inside a widget `zle -U - abc` pushes `abc`, and `zle`,
// `zle --` and `zle -` are each status 0; under `zsh -c` each is 1.
func TestALoneDashEndsZlesOptions(t *testing.T) {
	r, out := zleRunner(t, "w() { zle -U - abc; print -r -- st=$?; zle; print -r -- $?; zle -; print -r -- $? }\nzle -N w\n")
	ed := &stubEditor{}
	_, _, printed, _ := runWidgetWatching(t, r, out, "w", repl.Line{}, ed)
	if printed != "st=0\n0\n0\n" {
		t.Errorf("printed %q", printed)
	}
	if want := []string{"abc"}; !slices.Equal(ed.pushed, want) {
		t.Errorf("pushed %v, want %v", ed.pushed, want)
	}
	if out, _ := runZsh(t, t.TempDir(), "zle; print -r -- $?; zle -; print -r -- $?\n"); out != "1\n1\n" {
		t.Errorf("outside a widget: %q", out)
	}
}

// Both are built-in widgets, so a listing has them and the dotted spelling
// reaches them past a redefinition.
func TestReadCommandAndSplitUndoAreListed(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "zle -la read-command && zle -la split-undo && print -r -- both\n")
	if out != "both\n" {
		t.Errorf("got %q", out)
	}
}
