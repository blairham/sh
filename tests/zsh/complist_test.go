// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `zsh/complist`, measured against zsh 5.9 on 2026-09-11 with a scratch HOME
// and no startup files, one question at a time. Whole outputs rather than
// substrings, because what these cases are about is a *list* changing — a
// Contains check for `menuselect` passes on a shell that added it to every
// keymap listing whether or not the module loaded.

// The eleven keymaps a loaded `zsh/complist` leaves behind, and the nine
// before it. Byte for byte real zsh's own two listings.
//
// The order is not incidental: zsh prints them sorted, dot-prefixed first, and
// the two new names land in the middle of the existing nine rather than at the
// end — `listscroll` after `isearch` and `menuselect` after `main`. A shell
// that appended them would pass a set comparison and fail this.
func TestComplistCreatesTheKeymapsItCreatesInZsh(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `bindkey -l
print -r -- "---"
zmodload zsh/complist
print -r -- "load=$?"
bindkey -l`)
	want := ".safe\ncommand\nemacs\nisearch\nmain\nvicmd\nviins\nviopp\nvisual\n" +
		"---\nload=0\n" +
		".safe\ncommand\nemacs\nisearch\nlistscroll\nmain\nmenuselect\nvicmd\nviins\nviopp\nvisual\n"
	if out != want || st != 0 {
		t.Errorf("bindkey -l around the load = %q (status %d), want %q", out, st, want)
	}
}

// Unloading takes them away again, measured. The list is derived from what is
// loaded rather than stored, and this is what says so: a shell that added the
// names to a table when the module loaded would keep them here.
func TestUnloadingComplistTakesItsKeymapsAway(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/complist
zmodload -u zsh/complist
print -r -- "unload=$?"
bindkey -l`)
	want := "unload=0\n" +
		".safe\ncommand\nemacs\nisearch\nmain\nvicmd\nviins\nviopp\nvisual\n"
	if out != want || st != 0 {
		t.Errorf("bindkey -l after the unload = %q (status %d), want %q", out, st, want)
	}
}

// **The pair a real startup file writes**, and the reason this issue existed:
// Oh My Zsh's completion library loads the module and binds a key in the
// keymap it creates two lines later. Both halves, because the second one's
// refusal was the honest consequence of the first one's, and only the first
// was ever a defect.
//
// The binding is read back rather than merely accepted. `bindkey -M menuselect
// '^o' …` answering 0 and storing nothing would pass a status check, and the
// widget it names is one this editor has not got — which zsh stores too,
// measured in bindkey.go — so what is being pinned is that an unknown widget
// in a module's keymap behaves as an unknown widget in any other.
func TestTheModuleIsWhatMakesItsKeymapBindable(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `bindkey -M menuselect '^o' accept-and-infer-next-history
print -r -- "before=$?"
zmodload -i zsh/complist
bindkey -M menuselect '^o' accept-and-infer-next-history
print -r -- "after=$?"
bindkey -M menuselect '^o'`)
	want := "zsh:bindkey:1: no such keymap `menuselect'\nbefore=1\nafter=0\n" +
		"\"^O\" accept-and-infer-next-history\n"
	if out != want || st != 0 {
		t.Errorf("the startup pair = %q (status %d), want %q", out, st, want)
	}
}

// **Supporting features and naming none is not the same as not supporting
// them**, and the difference is a status. Measured: `zmodload -lF
// zsh/complist` on a loaded one writes nothing at 0, where the same command
// about `zsh/main` is a refusal at 1.
//
// Both modules have an empty feature list, so the emptiness cannot tell them
// apart — which is what this case is for. A shell that went back to deciding
// on the length of the list gives complist zsh/main's sentence and 1.
func TestComplistSupportsFeaturesAndNamesNoneWhereMainSupportsNone(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/complist
zmodload -lF zsh/complist
print -r -- "complist=$?"
zmodload -lF zsh/main
print -r -- "main=$?"
zmodload -F zsh/complist
print -r -- "select=$?"
zmodload -F zsh/main
print -r -- "selectmain=$?"`)
	want := "complist=0\n" +
		"zsh:zmodload:4: module `zsh/main' does not support features\nmain=1\n" +
		"select=0\n" +
		"zsh:8: module `zsh/main' does not support features\nselectmain=1\n"
	if out != want || st != 0 {
		t.Errorf("the two empty feature lists = %q (status %d), want %q", out, st, want)
	}
}

// `$keymaps` and `bindkey -l` are one list asked twice, so the module moves
// both. Sorted here where zsh's association is in hash order, which is
// zleparameter.go's own recorded choice and not this module's.
func TestTheKeymapsParameterSeesTheModulesKeymaps(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zleparameter
print -r -- "n=${#keymaps}"
zmodload zsh/complist
print -r -- "n=${#keymaps}"
print -r -- ${keymaps[(r)menuselect]} ${keymaps[(r)listscroll]}`)
	want := "n=9\nn=11\nmenuselect listscroll\n"
	if out != want || st != 0 {
		t.Errorf("$keymaps around the load = %q (status %d), want %q", out, st, want)
	}
}

// **A recorded divergence, and the line this change deliberately did not
// cross.** zsh's `zsh/complist` defines the `menu-select` widget as well as
// the keymaps, so `${+widgets[menu-select]}` is 1 there. It is 0 here, and
// stays 0: a widget is a thing the editor performs and this editor cannot
// perform menu selection, so naming it would be the invention bindkey.go's
// rule exists to prevent — and the `$terminfo` trap in the same words, since a
// caller testing `${+widgets[menu-select]}` before wrapping it would be told
// yes and then find nothing behind it.
//
// `beginning-of-line` beside it, so the case cannot pass on a shell whose
// `$widgets` answers 0 to everything.
func TestComplistDoesNotClaimTheWidgetThisEditorCannotPerform(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zleparameter
zmodload zsh/complist
print -r -- "menu=${+widgets[menu-select]} bol=${+widgets[beginning-of-line]}"`)
	want := "menu=0 bol=1\n"
	if out != want || st != 0 {
		t.Errorf("$widgets after the load = %q (status %d), want %q", out, st, want)
	}
}
