// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/repl"
)

// `bind` measured against bash 5.3.15 under a pseudo-terminal, 2026-09-07.
//
// A terminal on the other end is not a nicety here: `bind` in a shell without
// one remarks `warning: line editing not enabled` first, and every answer
// below was taken with the editor really loaded so that the answers are the
// builtin's and not the warning path's.

// bindRun runs a snippet and gives back everything it wrote, with the
// interactive flag set so the warning path is out of the way — the warning
// has its own test.
func bindRun(t *testing.T, src string) (string, int) {
	t.Helper()
	var buf strings.Builder
	r := preset.Runner(dialecttest.Base{Stdout: &buf, Stderr: &buf})
	r.Interactive = true
	f := preset.Parse(t, src)
	st, err := r.Run(t.Context(), f)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return buf.String(), st
}

// TestTheTwoArgumentFormsAreNotOneForm is the finding the file turns on: the
// left side of a bare `keyseq:function` is a *key name* and one key, and the
// left side of a quoted `"keyseq": function` is a whole sequence.
//
// Measured, and the difference is silent in bash: `bind "\C-x\C-a":x` binds
// nothing at status 0 because `\C-x\C-a` names no single key, while the same
// sequence in quotes binds for real. A shell that read the first as two bytes
// would be binding a key bash leaves alone.
func TestTheTwoArgumentFormsAreNotOneForm(t *testing.T) {
	// The key-name form, which is what a real rc file writes.
	out, st := bindRun(t, `bind "\C-l":clear-screen`+"\n"+`bind -q clear-screen`)
	if want := `clear-screen can be invoked via "\C-l".`; !strings.Contains(out, want) {
		t.Errorf("key-name form: out = %q, want it to contain %q", out, want)
	}
	if st != 0 {
		t.Errorf("key-name form: status %d, want 0", st)
	}
	// Two keys with no quotes is not a key name: nothing is bound, and
	// nothing is said. Asked of the *listing* and not only of the status —
	// without the query this passes for an implementation that binds the two
	// bytes, since binding is silent either way.
	out, st = bindRun(t, `bind "\C-x\C-a":beginning-of-line`+"\n"+`echo st=$?`+"\n"+`bind -q beginning-of-line`)
	if strings.Contains(out, `\C-x\C-a`) {
		t.Errorf("bare two-byte form bound something: %q", out)
	}
	if !strings.Contains(out, "st=0") || st != 0 {
		t.Errorf("bare two-byte form: out = %q status %d, want a silent 0", out, st)
	}
	// The control for that: the same sequence *is* reachable, so the absence
	// above is the form being refused and not the query being blind.
	if !strings.Contains(out, "beginning-of-line can be invoked via") {
		t.Errorf("the query said nothing at all: %q", out)
	}
	// And the same sequence in quotes does bind.
	out, _ = bindRun(t, `bind '"\C-x\C-a": beginning-of-line'`+"\n"+`bind -q beginning-of-line`)
	if !strings.Contains(out, `"\C-x\C-a"`) {
		t.Errorf("quoted form did not bind: %q", out)
	}
}

// TestAnUnknownFunctionNameIsNotAnErrorAndIsNotStored is the other measured
// finding, and it is the *opposite* of what the other shell with an editor
// does — which is why the answer lives in a dialect.
//
// A plugin's `bind` for something this shell has not got leaves the keyboard
// exactly as it was, rather than turning the key into one that does nothing.
func TestAnUnknownFunctionNameIsNotAnErrorAndIsNotStored(t *testing.T) {
	out, st := bindRun(t, `bind '"\C-x\C-t": no-such-widget'`+"\n"+`echo st=$?`+"\n"+`bind -p`)
	if !strings.Contains(out, "st=0") || st != 0 {
		t.Errorf("out = %q status %d, want a silent 0", out, st)
	}
	if strings.Contains(out, "no-such-widget") {
		t.Errorf("the unknown name was stored: %q", out)
	}
	if strings.Contains(out, `\C-x\C-t`) {
		t.Errorf("the key was bound: %q", out)
	}
	// **And the key still does what it did**, which the listing cannot say:
	// `-p` walks the names this editor has, so a stored unknown one would
	// never appear there however wrong the storing was. What it *would* do
	// is put the key in the table the editor reads, bound to nothing — which
	// is exactly the "the plugin believes its binding exists" failure, and
	// only KeyBindings can see it.
	if table := bindingsAfter(t, `bind '"\C-x\C-t": no-such-widget'`); len(table) != 0 {
		t.Errorf("the editor was handed %v, and nothing this editor has was bound", table)
	}
	// The control for both: a name this editor *has* is stored and does
	// reach the editor, so the two assertions above are about the name and
	// not about the whole path being dead.
	out, _ = bindRun(t, `bind '"\C-x\C-t": clear-screen'`+"\n"+`bind -p`)
	if !strings.Contains(out, `"\C-x\C-t": clear-screen`) {
		t.Errorf("a known name was not stored: %q", out)
	}
	table := bindingsAfter(t, `bind '"\C-x\C-t": clear-screen'`)
	if got, want := table["\x18\x14"], (repl.Binding{Widget: repl.WidgetClearScreen}); got != want {
		t.Errorf("^X^T reached the editor as %v, want %v", got, want)
	}
}

// bindingsAfter runs a snippet and hands back the table the editor would read,
// which is the only place a binding to a name this editor has not got is
// visible — the listings walk the names it *has*.
func bindingsAfter(t *testing.T, src string) map[string]repl.Binding {
	t.Helper()
	var buf strings.Builder
	r := preset.Runner(dialecttest.Base{Stdout: &buf, Stderr: &buf})
	r.Interactive = true
	if _, err := r.Run(t.Context(), preset.Parse(t, src)); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return bash.KeyBindings(r, repl.KeymapMain)
}

// TestBindRefusesByNameRatherThanAcceptingSilently is the rule the whole
// builtin is written to: an rc file's `bind` must not fail for something this
// shell has, and must not quietly succeed for something it has not.
//
// The letters bash has and this does not are `not implemented yet`, which is
// a different sentence from `invalid option` on purpose — a script can tell a
// gap from a typo.
func TestBindRefusesByNameRatherThanAcceptingSilently(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{"a readline variable", "bind -v", "bind: -v is not implemented yet", 2},
		{"reading an inputrc", "bind -f /dev/null", "bind: -f is not implemented yet", 2},
		{
			// A keymap bash really has: readline reaches it through a prefix
			// and this editor reads a sequence whole, so a binding there
			// could never fire. Missing, not invalid.
			"a prefix keymap", `bind -m emacs-ctlx '"\C-a": undo'`,
			"bind: -m emacs-ctlx is not implemented yet", 2,
		},
		{
			"a keymap that is no keymap", `bind -m nosuch '"\C-a": undo'`,
			"bind: `nosuch': invalid keymap name", 1,
		},
		{"a letter that is no letter", "bind -Z", "bind: -Z: invalid option", 2},
		{"an unknown function queried", "bind -q no-such", "bind: `no-such': unknown function name", 1},
		{
			// A name this editor has with no key on it is a *different*
			// answer to a different question, and it is also 1 — measured.
			// Asserted here rather than only in the listing test, where the
			// wording was pinned and the status was not.
			"a known function on no key", "bind -u forward-char\nbind -q forward-char",
			"forward-char is not bound to any keys.", 1,
		},
		{"an unknown function unbound", "bind -u no-such", "bind: `no-such': unknown function name", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := bindRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("out = %q, want it to contain %q", out, tc.want)
			}
			if st != tc.status {
				t.Errorf("status = %d, want %d", st, tc.status)
			}
		})
	}
	// And the usage line follows a bad letter, which is bash's shape for
	// this builtin and not every builtin's.
	if out, _ := bindRun(t, "bind -Z"); !strings.Contains(out, "bind: usage: bind [-lpsvPSVX]") {
		t.Errorf("a bad letter printed no usage line: %q", out)
	}
}

// TestBindListingsTakeTheirMeasuredShapes pins the four listings, each of
// which has wording of its own.
//
// The two "not bound" sentences differ by a full stop — `-P` has none and `-q`
// has one — measured in bash 5.3. It reads like a typo and is not, which is
// exactly why it is asserted.
func TestBindListingsTakeTheirMeasuredShapes(t *testing.T) {
	out, _ := bindRun(t, "bind -u forward-char\nbind -P\nbind -q forward-char")
	if want := "forward-char is not bound to any keys\n"; !strings.Contains(out, want) {
		t.Errorf("-P said %q, want a line %q with no full stop", out, want)
	}
	if want := "forward-char is not bound to any keys.\n"; !strings.Contains(out, want) {
		t.Errorf("-q said %q, want %q with a full stop", out, want)
	}
	// -p prints one row per key, and a function on no key as a comment.
	out, _ = bindRun(t, "bind -u forward-char\nbind -p")
	if want := "# forward-char (not bound)"; !strings.Contains(out, want) {
		t.Errorf("-p said %q, want it to contain %q", out, want)
	}
	if want := `"\C-b": backward-char`; !strings.Contains(out, want) {
		t.Errorf("-p said %q, want it to contain %q", out, want)
	}
	// -s prints only the keys that type text, and nothing where none were
	// made.
	out, _ = bindRun(t, "bind -s")
	if strings.TrimSpace(out) != "" {
		t.Errorf("-s with no macros said %q, want nothing", out)
	}
	out, _ = bindRun(t, `bind '"\C-x\C-y": "hello"'`+"\nbind -s")
	if want := `"\C-x\C-y": "hello"`; !strings.Contains(out, want) {
		t.Errorf("-s said %q, want it to contain %q", out, want)
	}
}

// TestBindPrintsSequencesInReadlineNotation pins the encoding, which is not
// the caret notation the other dialect prints — the same bytes, two shells,
// two spellings, and that is the reason each holds its own.
func TestBindPrintsSequencesInReadlineNotation(t *testing.T) {
	for _, tc := range []struct{ name, bound, want string }{
		{"a control byte", `\C-x\C-a`, `"\C-x\C-a"`},
		{"escape", `\e[Z`, `"\e[Z"`},
		{"delete", `\C-?`, `"\C-?"`},
		{"tab", `\t`, `"\C-i"`},
		{"NUL", `\C-@`, `"\C-@"`},
		{"the high half", `\M-z`, `"\372"`},
		{"a backslash", `\\`, `"\\"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := bindRun(t, `bind '"`+tc.bound+`": clear-screen'`+"\nbind -q clear-screen")
			if !strings.Contains(out, tc.want) {
				t.Errorf("out = %q, want it to contain %q", out, tc.want)
			}
		})
	}
}

// TestTheKeymapABindingLandsInIsTheEditingModes is what makes `set -o vi`
// mean something rather than being a flag nobody reads.
//
// The rc file that opened this writes `set -o vi` and then three `bind -m
// vi-insert` lines, and those bindings have to be the live ones afterwards.
// Measured: `set -o vi` makes `vi-insert` the map `bind` answers from.
func TestTheKeymapABindingLandsInIsTheEditingModes(t *testing.T) {
	// In vi mode the vi-insert binding is what `bind` answers with, and the
	// emacs one is not.
	src := "set -o vi\n" +
		`bind -m vi-insert '"\C-g": clear-screen'` + "\n" +
		`bind -m emacs '"\C-g": undo'` + "\n" +
		"bind -q clear-screen\nbind -q undo"
	out, _ := bindRun(t, src)
	if !strings.Contains(out, `clear-screen can be invoked via "\C-g", "\C-l".`) {
		t.Errorf("in vi mode: out = %q, want the vi-insert binding live", out)
	}
	if strings.Contains(out, `undo can be invoked via "\C-g"`) {
		t.Errorf("in vi mode: out = %q, want the emacs binding inert", out)
	}
	// And with no `set -o vi` the two swap over, which is what says the mode
	// is doing the work rather than the order of the two commands.
	out, _ = bindRun(t, strings.TrimPrefix(src, "set -o vi\n"))
	if !strings.Contains(out, `\C-g`) || !strings.Contains(out, "undo can be invoked via") {
		t.Errorf("in emacs mode: out = %q, want the emacs binding live", out)
	}
	if strings.Contains(out, `clear-screen can be invoked via "\C-g"`) {
		t.Errorf("in emacs mode: out = %q, want the vi-insert binding inert", out)
	}
}

// TestKeyBindingsReportsOnlyWhatChanged is the seam the editor reads, and the
// contract is that a key nobody mentioned is absent so it reaches the
// editor's own dispatch.
func TestKeyBindingsReportsOnlyWhatChanged(t *testing.T) {
	var buf strings.Builder
	r := preset.Runner(dialecttest.Base{Stdout: &buf, Stderr: &buf})
	r.Interactive = true
	if _, err := r.Run(t.Context(), preset.Parse(t, `bind '"\C-g": clear-screen'`)); err != nil {
		t.Fatalf("run: %v", err)
	}
	table := bash.KeyBindings(r, repl.KeymapMain)
	if got, want := table["\a"], (repl.Binding{Widget: repl.WidgetClearScreen}); got != want {
		t.Errorf("^G = %v, want %v", got, want)
	}
	// ^A is a default and must not be in the table: an override for it would
	// be the front end doing work the dispatch already does, and would hide a
	// change to the dispatch.
	if _, present := table["\x01"]; present {
		t.Errorf("^A is in the override table, and nobody rebound it: %v", table)
	}
	if len(table) != 1 {
		t.Errorf("table = %v, want only the one key that was rebound", table)
	}
}

// TestBindWarnsWhereThereIsNoLineEditorAndAnswersAnyway pins the warning path,
// which is measured to be a remark and not a refusal: `bash -c 'bind -p'`
// prints the whole keymap after it, and `bash -c 'bind -m nosuch -q x'` still
// refuses the keymap at 1.
func TestBindWarnsWhereThereIsNoLineEditorAndAnswersAnyway(t *testing.T) {
	var buf strings.Builder
	r := preset.Runner(dialecttest.Base{Stdout: &buf, Stderr: &buf})
	// Interactive left false, which is the whole of the case.
	st, err := r.Run(t.Context(), preset.Parse(t, "bind -q clear-screen"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	out := buf.String()
	if want := "bind: warning: line editing not enabled"; !strings.Contains(out, want) {
		t.Errorf("out = %q, want it to contain %q", out, want)
	}
	if want := `clear-screen can be invoked via "\C-l".`; !strings.Contains(out, want) {
		t.Errorf("out = %q, want the answer as well as the warning: %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0 — the warning is a remark", st)
	}
}

// TestEveryActionThisEditorPerformsHasANameAndAKey is the guard against the
// two halves drifting apart.
//
// A fresh shell's `bind -P` must have no "not bound" row at all: every action
// the editor performs is on a key by default — repl.DefaultBindings is where
// that is pinned against the dispatch — so a row saying otherwise means this
// dialect has no name for something the editor does, and `bind -p` would
// report a working key as unbound. The reverse, a name with no action behind
// it, is what repl/widgets.go says is worse than a name not offered.
//
// With one measured exception, which is the three actions that move between
// insert and command mode. They are reached from the command mode's own
// dispatch — `i`, `a` and `I` — rather than from a key in the map a person
// types in, so this listing has nothing to print for them, and real bash
// prints the same thing: `bind -P` in bash 5.3.15 says `vi-movement-mode is
// not bound to any keys` and the same for `vi-insertion-mode` and
// `vi-append-mode`, in emacs mode and in vi mode alike. (In vi mode it does
// find `vi-movement-mode` on `\e`; that row is viUnbound's open half — see
// the note there.)
func TestEveryActionThisEditorPerformsHasANameAndAKey(t *testing.T) {
	rows, _ := bindRun(t, "bind -P")
	offered := map[string]bool{}
	list, _ := bindRun(t, "bind -l")
	for _, name := range strings.Fields(list) {
		offered[name] = true
	}
	named := 0
	for _, line := range strings.Split(strings.TrimSpace(rows), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if strings.Contains(line, "is not bound to any keys") && !viUnbound[fields[0]] {
			t.Errorf("%q, in a shell where nothing was rebound — "+
				"either the editor has an action with no key or this dialect has no name for one", line)
		}
		// Every name a listing prints is a name `bind` will take back, or the
		// listing is telling a person to write something that gets ignored.
		// `accept-line` is the exception on purpose: it is the editor's own
		// control flow and has no Widget, so there is nothing to bind it to.
		if name := fields[0]; !offered[name] && name != "accept-line" {
			t.Errorf("the listing prints %q, which `bind -l` does not offer", name)
		}
		named++
	}
	// And the count is the actions plus the one control key, so a listing
	// that quietly stopped printing rows fails here rather than passing the
	// loop above by having nothing to check.
	if want := len(bindActionNames()) + 1 + len(viUnbound); named != want {
		t.Errorf("bind -P printed %d rows, want %d", named, want)
	}
}

// viUnbound are the three actions this listing has no key for, and the reason
// the test above has an exception at all.
//
// They are how a person gets between the editor's two states, and the keys
// that do it live in the command mode's own dispatch rather than in the map
// this listing prints. Measured, real bash prints them as unbound too.
//
// The open half: in vi mode real bash reports `vi-movement-mode can be found
// on "\e"`, because Escape is what selects command mode there. This listing
// does not say so, which is one row of one listing and not a key that fails to
// work — Escape leaves insert mode here whether or not `bind -P` mentions it.
var viUnbound = map[string]bool{
	"vi-movement-mode":  true,
	"vi-insertion-mode": true,
	"vi-append-mode":    true,
}

// bindActionNames is every action the editor performs, as the set of widgets
// the default table reaches — which is all of them, since a widget with no
// default key is one this shell could not name from a listing.
func bindActionNames() map[repl.Widget]bool {
	out := map[repl.Widget]bool{}
	for _, w := range repl.DefaultBindings() {
		out[w] = true
	}
	return out
}
