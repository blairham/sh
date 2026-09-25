// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
	"github.com/blairham/sh/syntax"
)

// The line editor's key table, measured against zsh 5.9.2 on 2026-09-05 —
// with `-c`, and one keystroke at a time under a pseudo-terminal for the
// questions `-c` cannot answer.

// TestAnUnknownWidgetIsAcceptedInSilence is the measured finding this builtin
// is built around, and the one that makes a real rc file work.
//
// Both `bindkey` lines in the config that opened #840 name a widget from a
// plugin. Real zsh takes them without a word, stores them, and says them back
// — under a terminal with the editor loaded as much as under `-c`. Refusing
// them would be *stricter than the shell being modeled*, which is the one way
// this could fail the file it exists to run.
func TestAnUnknownWidgetIsAcceptedInSilence(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"bindkey '^[[A' history-substring-search-up; echo \"st=$?\"\nbindkey '^[[A'\n")
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	want := "st=0\n\"^[[A\" history-substring-search-up\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestABindingIsSaidBack covers the plain form and the one-argument query
// together, since the second is how a person checks the first.
func TestABindingIsSaidBack(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"bindkey '^X^T' transpose-chars; echo \"st=$?\"\nbindkey '^X^T'\nbindkey '^X^Q'\n")
	want := "st=0\n\"^X^T\" transpose-chars\n\"^X^Q\" undefined-key\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestRemovingABindingLeavesItUndefined is `-r`, including that removing a key
// nobody bound is still 0 — measured; there is nothing to fail at.
func TestRemovingABindingLeavesItUndefined(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"bindkey '^A'\nbindkey -r '^A'; echo \"st=$?\"\nbindkey '^A'\nbindkey -r '^X^Q'; echo \"none=$?\"\n")
	want := "\"^A\" beginning-of-line\nst=0\n\"^A\" undefined-key\nnone=0\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestTheKeymapCanBeSelectedAndNamed covers `-v`, `-e`, `-l` and `-M`.
//
// `-v` and `-e` change which keymap is current and stay changed; `-M` names
// one for a single command and leaves the current one alone, measured.
func TestTheKeymapCanBeSelectedAndNamed(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"bindkey -M emacs '^X^Q' beginning-of-line\nbindkey '^X^Q'\n"+
			"bindkey -v; echo \"v=$?\"\nbindkey '^X^Q'\nbindkey -e; echo \"e=$?\"\nbindkey '^X^Q'\n")
	want := "\"^X^Q\" beginning-of-line\nv=0\n\"^X^Q\" undefined-key\ne=0\n\"^X^Q\" beginning-of-line\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	out, st := runZsh(t, t.TempDir(), "bindkey -l\n")
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	if want := ".safe\ncommand\nemacs\nisearch\nmain\nvicmd\nviins\nviopp\nvisual\n"; out != want {
		t.Errorf("keymaps = %q, want %q", out, want)
	}
}

// TestAKeymapThatIsNotThereIsRefused, in this shell's own wording and with its
// own backtick-and-quote spelling.
func TestAKeymapThatIsNotThereIsRefused(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "bindkey -M nosuchmap '^A' beginning-of-line\n")
	if !strings.Contains(out, "no such keymap `nosuchmap'") {
		t.Errorf("output = %q, want the keymap refused", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

// TestBindkeyRefusesInItsOwnWords keeps this builtin's wording apart from
// `zstyle`'s. This one says `bad option` where that one says `invalid option`,
// and its usage complaint names the letter that was short where that one names
// nothing — measured in both, and a shared helper would quietly make them
// agree.
func TestBindkeyRefusesInItsOwnWords(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "bindkey -q\n")
	if !strings.Contains(out, "bad option: -q") || st != 1 {
		t.Errorf("output = %q at %d, want the option refused this builtin's way", out, st)
	}
	if !strings.Contains(out, ":bindkey:") {
		t.Errorf("output = %q, want the builtin named in the location", out)
	}
	for _, c := range []struct{ src, want string }{
		{"bindkey -r\n", "not enough arguments for -r"},
		{"bindkey -s\n", "not enough arguments for -s"},
	} {
		out, st := runZsh(t, t.TempDir(), c.src)
		if !strings.Contains(out, c.want) || st != 1 {
			t.Errorf("%q gave %q at %d, want %q at 1", c.src, out, st, c.want)
		}
	}
}

// TestSequenceNotationRoundTrips pins the spellings a key can be written in
// and the one spelling it is printed in.
//
// Every pair was measured by binding the left and reading the right back. `^ `
// is the one that looks like a typo and is not: the caret clears the top three
// bits of whatever follows, and a space is 0x20, so `^ ` is NUL.
func TestSequenceNotationRoundTrips(t *testing.T) {
	for _, c := range []struct{ written, printed string }{
		{`\C-x\C-y`, "^X^Y"},
		{`\e[A`, "^[[A"},
		{`^[[A`, "^[[A"},
		{`\C-a`, "^A"},
		{`\e\eq`, "^[^[q"},
		{`^ `, "^@"},
		{`\t`, "^I"},
		{`\x41x`, "Ax"},
		{`\100`, "@"},
		{`\M-q`, `\M-q`},
		{`^?`, "^?"},
	} {
		out, _ := runZsh(t, t.TempDir(), "bindkey '"+c.written+"' aWidget\nbindkey '"+c.written+"'\n")
		if want := "\"" + c.printed + "\" aWidget\n"; out != want {
			t.Errorf("%q printed %q, want %q", c.written, out, want)
		}
	}
}

// TestAStringBindingIsShownAsAString is `-s`, whose target is text to type
// rather than a widget — and which the listing tells apart by quoting it.
func TestAStringBindingIsShownAsAString(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "bindkey -s '^X^Z' 'echo hi'; echo \"st=$?\"\nbindkey '^X^Z'\n")
	if want := "st=0\n\"^X^Z\" \"echo hi\"\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestSeveralPairsBindAtOnce, which an rc file does and which a loop over
// argument pairs is the only way to get right.
func TestSeveralPairsBindAtOnce(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"bindkey '^X^Q' beginning-of-line '^X^W' end-of-line; echo \"st=$?\"\nbindkey '^X^Q'\nbindkey '^X^W'\n")
	want := "st=0\n\"^X^Q\" beginning-of-line\n\"^X^W\" end-of-line\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestTheListingIsSortedByTheBytesTheKeySends, which is why `^_` comes before
// a space and `^?` after a tilde in this shell's own output — and `-L` writes
// the same list as the commands that would set it.
func TestTheListingIsSortedByTheBytesTheKeySends(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "bindkey -r '^A'\nbindkey\n")
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) < 10 {
		t.Fatalf("listing = %q, want the editor's keys", out)
	}
	// The order is the bytes the key sends and not the text that is printed,
	// and the two disagree: `^[^?` is ESC then DEL, which sorts *after* `^[f`
	// by its bytes and before it by the caret spelling of them. Sorting the
	// printed lines is the mistake this pins.
	at := func(prefix string) int {
		for i, line := range lines {
			if strings.HasPrefix(line, prefix) {
				return i
			}
		}
		t.Fatalf("listing = %q, want a line for %s", out, prefix)
		return -1
	}
	if at(`"^[f"`) > at(`"^[^?"`) {
		t.Errorf("listing has ESC-DEL before ESC-f, which is the printed order and not the byte order")
	}
	if at(`"^B"`) > at(`"^[B"`) {
		t.Errorf("listing is not in byte order")
	}
	// And a key just removed is not in it, which is what makes the listing a
	// statement about what the keys do rather than about what was ever set.
	for _, line := range lines {
		if strings.HasPrefix(line, `"^A" `) {
			t.Errorf("listing still holds the removed key: %q", line)
		}
	}
	out, _ = runZsh(t, t.TempDir(), "bindkey -L\n")
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if !strings.HasPrefix(line, "bindkey \"") {
			t.Errorf("-L line = %q, want a command", line)
		}
	}
}

// TestWhatIsNotBuiltIsRefusedAsMissing keeps a gap distinguishable from a
// typo, which is the split `whence` makes for the same reason.
func TestWhatIsNotBuiltIsRefusedAsMissing(t *testing.T) {
	for _, letter := range []string{"p", "R", "N", "A", "D", "d"} {
		out, st := runZsh(t, t.TempDir(), "bindkey -"+letter+" x y\n")
		if !strings.Contains(out, "-"+letter+" is not implemented yet") || st != 1 {
			t.Errorf("-%s gave %q at %d, want it refused as missing", letter, out, st)
		}
	}
}

// TestBindingsAreASubshellsOwn, for the reason the styles are: the table is in
// the Runner and not in a package variable, so a clone cannot write through.
func TestBindingsAreASubshellsOwn(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"bindkey '^X^Q' beginning-of-line\n(bindkey -r '^X^Q')\nbindkey '^X^Q'\n")
	if want := "\"^X^Q\" beginning-of-line\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestOnlyTheChangesReachTheEditor is the contract between this builtin and
// the front end, and it is the half no output can show.
//
// The table handed to repl is an *override layer*: a key nobody mentioned must
// be absent from it, so that the editor's own dispatch keeps it. A binding to
// a widget this editor has not got must be present and map to nothing, because
// that is what stops the key doing what it used to.
func TestOnlyTheChangesReachTheEditor(t *testing.T) {
	r := bindkeyRunner(t, "")
	if got := zsh.KeyBindings(r, repl.KeymapMain); len(got) != 0 {
		t.Errorf("with nothing rebound the table = %v, want empty", got)
	}

	r = bindkeyRunner(t, "bindkey '^G' beginning-of-line\n")
	got := zsh.KeyBindings(r, repl.KeymapMain)
	if want := (map[string]repl.Binding{"\a": {Widget: repl.WidgetBeginningOfLine}}); len(got) != 1 || got["\a"] != want["\a"] {
		t.Errorf("table = %v, want %v", got, want)
	}

	r = bindkeyRunner(t, "bindkey '^G' history-substring-search-up\n")
	got = zsh.KeyBindings(r, repl.KeymapMain)
	if b, present := got["\a"]; !present || b != (repl.Binding{}) {
		t.Errorf("table = %v, want the unknown widget present and doing nothing", got)
	}

	// A key removed is present and does nothing, which is different from
	// absent: absent would leave the editor's own default running.
	r = bindkeyRunner(t, "bindkey -r '^A'\n")
	got = zsh.KeyBindings(r, repl.KeymapMain)
	if b, present := got["\x01"]; !present || b != (repl.Binding{}) {
		t.Errorf("table = %v, want the removed key present and doing nothing", got)
	}

	// And a key rebound back to what it already did is not a change at all.
	r = bindkeyRunner(t, "bindkey '^A' beginning-of-line\n")
	if got := zsh.KeyBindings(r, repl.KeymapMain); len(got) != 0 {
		t.Errorf("table = %v, want nothing for a key rebound to its own default", got)
	}
}

// bindkeyRunner runs some source and hands back the Runner it ran in, which is
// what KeyBindings reads. runZsh cannot be used because it keeps the Runner to
// itself and the table is the thing under test.
func bindkeyRunner(t *testing.T, src string) *interp.Runner {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	dir := t.TempDir()
	var out strings.Builder
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{
		Stdout: &out, Stderr: &out, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "zsh", Vars: map[string]string{"PATH": dir},
		Dialect: presetDialect(),
	}
	zsh.Apply(r)
	if _, err := r.Run(t.Context(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return r
}

// TestTheCommandKeymapIsReadWhenTheEditorIsInIt closes the hole #1427 was
// filed for: `bindkey -M vicmd` was stored and `vicmd` was never current, so
// the binding could not fire.
//
// Measured under a pty against zsh 5.9.2, with `bindkey -v` and `bindkey -M
// vicmd '^Xz' beginning-of-line`: in command mode the key moves the cursor to
// the start of the line, and in insert mode the same key does nothing.
func TestTheCommandKeymapIsReadWhenTheEditorIsInIt(t *testing.T) {
	r := bindkeyRunner(t, "bindkey -v\n"+
		"bindkey -M vicmd '^Xz' beginning-of-line\n"+
		"bindkey -M viins '^Xy' end-of-line\n")
	command := zsh.KeyBindings(r, repl.KeymapViCommand)
	if got, want := command["\x18z"], (repl.Binding{Widget: repl.WidgetBeginningOfLine}); got != want {
		t.Errorf("^Xz in vicmd = %v, want %v", got, want)
	}
	if _, present := command["\x18y"]; present {
		t.Errorf("the viins key is in vicmd: %v", command)
	}
	typing := zsh.KeyBindings(r, repl.KeymapMain)
	if got, want := typing["\x18y"], (repl.Binding{Widget: repl.WidgetEndOfLine}); got != want {
		t.Errorf("^Xy in viins = %v, want %v", got, want)
	}
	if _, present := typing["\x18z"]; present {
		t.Errorf("the vicmd key is in viins: %v", typing)
	}
}

// TestACommandKeyBoundToItsEmacsDefaultIsStillReported is the narrow case the
// override filter used to swallow: the filter compares against what this
// editor does *while typing*, and in command mode the same key means something
// else, so dropping it as "already the default" would leave the key dead.
func TestACommandKeyBoundToItsEmacsDefaultIsStillReported(t *testing.T) {
	r := bindkeyRunner(t, "bindkey -M vicmd '^A' beginning-of-line\n")
	got, bound := zsh.KeyBindings(r, repl.KeymapViCommand)["\x01"]
	if !bound || got != (repl.Binding{Widget: repl.WidgetBeginningOfLine}) {
		t.Errorf("^A in vicmd = %v, %v, want the binding that was written", got, bound)
	}
}

// TestViEditingAndTheOptionAreReadTogether is the measurement that decided the
// seam's shape, and it reads *both* halves of every row on purpose.
//
// Under a pty against zsh 5.9.2: `bindkey -v` gives a working command mode and
// leaves `set -o` reporting `emacs off` and `vi off`, while `setopt vi` gives
// the same command mode and reports `vi on`. So the two are one piece of state
// written from two directions, and the option is a *report* of one of those
// writes rather than the state itself.
//
// **An earlier version of this test read only ViEditing, and it passed every
// day of #3140** — the bug where `setopt vi` was taken, reported itself on,
// and left the editor reading the emacs keymap. ViEditing answered from
// interp's editing mode, which the option really did move; nothing here asked
// what the *editor* would consult. The last three rows are the ones that tell
// the two apart, and each of them has the option and the keymap disagreeing.
func TestViEditingAndTheOptionAreReadTogether(t *testing.T) {
	for _, c := range []struct {
		src    string
		vi     bool // what the line editor does
		option bool // what `[[ -o vi ]]` says about it
	}{
		{src: ":\n"},
		{src: "bindkey -v\n", vi: true},
		{src: "bindkey -e\n"},
		{src: "bindkey -v\nbindkey -e\n"},
		{src: "setopt vi\n", vi: true, option: true},
		{src: "set -o vi\n", vi: true, option: true},
		{src: "setopt emacs\n"},

		// The rows that discriminate. Each is measured in zsh 5.9.2 by
		// reading `bindkey -lL main` and `[[ -o vi ]]` back together.
		//
		//   `setopt vi; bindkey -e` — the option stays on and the editor
		//   goes to emacs, so an answer read off the option is wrong here.
		//
		//   `setopt vi; unsetopt vi` — the option goes off and the keymap
		//   stays viins, so turning the option off is not a request to go
		//   back and an answer read off the option is wrong here too, in the
		//   other direction.
		{src: "setopt vi\nbindkey -e\n", option: true},
		{src: "setopt vi\nunsetopt vi\n", vi: true},
		{src: "bindkey -v\nset +o vi\n", vi: true},

		// And turning off the name that is *not* selected, which is the row
		// that tells "only on selects" from "either direction selects". The
		// two rows above cannot: turning `vi` off in a shell that turned `vi`
		// on would select `viins` again either way and read the same. This
		// one moves the other name, so a selection on the way off would put
		// the editor in emacs. Measured: `setopt vi; unsetopt emacs` leaves
		// `main` on `viins` with `vi` still on.
		{src: "setopt vi\nunsetopt emacs\n", vi: true, option: true},
		{src: "bindkey -v\nunsetopt emacs\n", vi: true},
		{src: "setopt emacs\nunsetopt vi\n"},
	} {
		r := bindkeyRunner(t, c.src)
		if got := zsh.ViEditing(r); got != c.vi {
			t.Errorf("after %q, ViEditing = %v, want %v", c.src, got, c.vi)
		}
		if got, known := r.DialectOption("vi"); !known || got != c.option {
			t.Errorf("after %q, [[ -o vi ]] = %v (known %v), want %v", c.src, got, known, c.option)
		}
	}
}

// TestTheKeymapListingSaysWhichOneIsCurrent is `bindkey -l` and `-lL`.
//
// `-lL` is the only way a script can read which keymap is current: `main` is
// an alias, and the command that would recreate it writes the target out.
// Measured on zsh 5.9.2 — `.safe` is named by `-l` and skipped by `-lL`
// because no `bindkey` command would make it, and a name no keymap has is
// refused with a colon that the same complaint from `-M` does not have.
func TestTheKeymapListingSaysWhichOneIsCurrent(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{src: "bindkey -lL main\n", want: "bindkey -A emacs main\n"},
		{src: "setopt vi\nbindkey -lL main\n", want: "bindkey -A viins main\n"},
		{src: "set -o vi\nbindkey -lL main\n", want: "bindkey -A viins main\n"},
		{src: "bindkey -v\nbindkey -lL main\n", want: "bindkey -A viins main\n"},
		{src: "setopt vi\nbindkey -e\nbindkey -lL main\n", want: "bindkey -A emacs main\n"},
		{src: "bindkey -l main\n", want: "main\n"},
		{src: "bindkey -l .safe\n", want: ".safe\n"},
		{src: "bindkey -lL .safe\n", want: ""},
		{src: "bindkey -lL emacs main\n", want: "bindkey -N emacs\nbindkey -A emacs main\n"},
		{src: "bindkey -lL nosuch_zz\n", want: "zsh:bindkey:1: no such keymap: `nosuch_zz'\n"},
	} {
		if out, _ := runZsh(t, t.TempDir(), c.src); out != c.want {
			t.Errorf("%q printed %q, want %q", c.src, out, c.want)
		}
	}
	if _, st := runZsh(t, t.TempDir(), "bindkey -lL nosuch_zz\n"); st != 1 {
		t.Errorf("a name no keymap has is status %d, want 1", st)
	}
}

// TestTheCommandKeymapIsTheChangesAndNothingElse is the guard on the bug that
// only the shipped binary showed.
//
// The table handed to the editor is an *override layer*, and `vicmd` has
// nothing to be an override of: what the editor does with a key there is the
// mode itself. Built from the same defaults as the typing map, it handed the
// editor an override for every key in that table — and Return stopped
// accepting the line, because `accept-line` is the editor's own control flow
// and has no widget for an override to carry.
func TestTheCommandKeymapIsTheChangesAndNothingElse(t *testing.T) {
	r := bindkeyRunner(t, "bindkey -v\n")
	if got := zsh.KeyBindings(r, repl.KeymapViCommand); len(got) != 0 {
		t.Errorf("with nothing bound into it, vicmd = %v, want empty", got)
	}
	if _, present := zsh.KeyBindings(r, repl.KeymapViCommand)["\r"]; present {
		t.Errorf("Return is in vicmd, and nobody put it there")
	}
	r = bindkeyRunner(t, "bindkey -v\nbindkey -M vicmd '^G' clear-screen\n")
	got := zsh.KeyBindings(r, repl.KeymapViCommand)
	if len(got) != 1 || got["\a"] != (repl.Binding{Widget: repl.WidgetClearScreen}) {
		t.Errorf("vicmd = %v, want only the key that was bound", got)
	}
}

// TestEndOfInputSurvivesARedefinedCompletionWidget is #4422, and it is the
// hole in TestOnlyTheChangesReachTheEditor above rather than a new question.
//
// `^D` is an editorControlKey and not a binding, deliberately: the key ends a
// session and the action it shares a name with offers to list. That separation
// held only while the key was dropped as a default, and the drop was guarded
// on the widget *not* being redefined. `compinit` ends by redefining the eight
// standard completion widgets, and `delete-char-or-list` is one of the eight —
// so on any ordinary rc the guard opened, `\x04` reached the editor as an
// override pointing at the completion widget, and the key loop's own `^D` was
// never asked. The session could not be ended from the keyboard.
func TestEndOfInputSurvivesARedefinedCompletionWidget(t *testing.T) {
	r := bindkeyRunner(t, "zle -C delete-char-or-list .delete-char-or-list _main_complete\n")
	if b, present := zsh.KeyBindings(r, repl.KeymapMain)["\x04"]; present {
		t.Errorf("^D reached the editor as %#v, want it left to the editor's own dispatch", b)
	}

	// The redefinition is real and must still reach the keys it is *bound*
	// to — this takes the key away from the action, not the action away.
	r = bindkeyRunner(t, "zle -C delete-char-or-list .delete-char-or-list _main_complete\nbindkey '^G' delete-char-or-list\n")
	if b, present := zsh.KeyBindings(r, repl.KeymapMain)["\a"]; !present || b == (repl.Binding{}) {
		t.Errorf("^G = %#v, want the redefined completion widget", b)
	}

	// And rebinding the key away is still a rebinding: what is dropped is the
	// key still holding its own default name, not the sequence.
	r = bindkeyRunner(t, "bindkey '^D' beginning-of-line\n")
	if b, present := zsh.KeyBindings(r, repl.KeymapMain)["\x04"]; !present || b.Widget != repl.WidgetBeginningOfLine {
		t.Errorf("^D rebound = %#v, want beginning-of-line", b)
	}
	r = bindkeyRunner(t, "bindkey -r '^D'\n")
	if b, present := zsh.KeyBindings(r, repl.KeymapMain)["\x04"]; !present || b != (repl.Binding{}) {
		t.Errorf("^D removed = %#v, want present and doing nothing", b)
	}

	// **Return is the other editorControlKey and it is deliberately not in
	// endOfInputKeys.** A plugin's wrapper around `accept-line` is exactly a
	// redefinition that leaves the key where it was, and repl runs the
	// wrapper and then ends the line for it (#2082) — so there, unlike here,
	// the redefinition must take the key. Pinned because the restriction to
	// one key is the whole of what keeps that true, and nothing else asked.
	r = bindkeyRunner(t, "f(){ :; }\nzle -N accept-line f\n")
	if got, want := zsh.KeyBindings(r, repl.KeymapMain)["\r"], (repl.Binding{Function: "accept-line"}); got != want {
		t.Errorf("Return = %#v, want %#v", got, want)
	}
}
