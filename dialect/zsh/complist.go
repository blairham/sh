// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"sort"

	"github.com/blairham/sh/interp"
)

// `zsh/complist` is the module a real startup file loads two lines before it
// binds a key in the keymap the module creates.
//
// Measured 2026-09-11 against zsh 5.9 on a scratch HOME with no startup files,
// one question at a time. `zmodload zsh/complist` is status 0 and silence, and
// what it leaves behind is exactly three things:
//
//	bindkey -l          gains `listscroll` and `menuselect`
//	zle -la             gains `menu-select` (and the internal `.menu-select`)
//	zmodload -lF …      writes nothing, at status 0
//
// The third is the one that decides how this file is written. **The module
// names no features at all** — not a builtin, not a parameter, not a condition
// — which is not the same as `zsh/main`, whose `-lF` is `module `zsh/main'
// does not support features` at status 1. So complist supports features and has
// none, and zmodload.go's own rule — *a module loads when everything it names
// is either implemented or refuses by name on access* — loads it vacuously,
// with nothing forgiven and nothing claimed. The two cases had been one line in
// that file because only `zsh/main` had ever reached it; they are two now, and
// zmodloadFeatureless is which of them a module is in.
//
// # What this shell provides of it, and what it does not
//
// The keymaps, and not the widget. That split is the whole of the decision here
// and it is the same one bindkey.go already argues for the nine keymaps this
// shell starts with:
//
//   - **A keymap is a place bindings live**, and `bindkey -M menuselect '^o'
//     accept-and-infer-next-history` is a line from a real startup file
//     (Oh My Zsh's completion library, loaded here through a plugin manager).
//     zsh stores that binding against a widget name it may not have either —
//     measured in bindkey.go: an unknown widget is not an error, it is stored,
//     and the key does nothing. A keymap that exists and holds what was put in
//     it is therefore the *entire* truth of what zsh does with that line, and
//     `no such keymap` was this shell's only remaining reason for refusing it.
//   - **A widget is a thing the editor performs**, and this editor cannot
//     perform menu selection: there is no scrolling highlighted list for a
//     keymap to move a cursor around in. So `menu-select` is deliberately not
//     in bindkeyWidgets and not in `$widgets`, and `zle menu-select` goes on
//     refusing by name. bindkey.go states the rule this follows: what a listing
//     answers is what this editor will actually do, and naming a widget nothing
//     here performs is the invention that rule exists to prevent.
//
// The trap that shape is measured against is `$terminfo`'s (#2076): a theme
// tests whether a capability is *there* and builds something different when it
// is not, so a plausible-but-wrong answer is worse than an absent one. Checked
// by hand against the configuration this was found in — nothing tests for
// `menuselect`, for `$widgets[menu-select]` or for `zmodload -e zsh/complist`
// before binding; the one real caller is the unconditional `bindkey -M` above,
// two lines after an unconditional `zmodload -i zsh/complist`. A stub that
// loaded and created nothing would have been the lie; a keymap that holds the
// binding is what the caller asked for and all this shell can honestly give.
//
// # Two recorded divergences
//
//   - **zsh's `menuselect` and `listscroll` arrive with default bindings** —
//     eleven and six of them, `^I` to `complete-word` and the four arrow
//     spellings among them. They are not reproduced, for the reason
//     bindkey.go gives for not reproducing zsh's 117: this editor never enters
//     either keymap, so every one of those lines would name a key doing
//     something that will not happen. What `bindkey -M menuselect` lists here
//     is what it lists for `viopp`, `visual`, `command` and `isearch` — this
//     editor's own keys — which is a pre-existing shape and one question about
//     all eleven keymaps rather than a new one about two.
//   - **zsh loads `zsh/complete` and `zsh/zle` along with it**, so its
//     `zmodload -L` writes four names where this shell's writes two. This
//     shell has no dependency table — `zmodload -d` is refused by name — and
//     inventing one for a single module would be a table with one row in it
//     that nothing else could ever consult.
//
// Unloading takes the keymaps away again, measured, and it does so here
// because the list is derived from what is loaded rather than stored: see
// keymapsNow.

// moduleComplist is the module's name, written once.
const moduleComplist = "zsh/complist"

// moduleKeymaps is which keymaps each module creates when it loads.
//
// A table rather than a check for one name, because it is the shape the next
// module needs and because `bindkey -l` should not have to know which module
// is which. Measured with `bindkey -l` before and after the load; the two
// names are in the order zsh's own listing puts them, which is plain sorted
// order and is imposed by keymapsNow rather than by this table.
var moduleKeymaps = map[string][]string{
	moduleComplist: {"listscroll", "menuselect"},
}

// keymapsNow is the keymaps this shell has right now: the ones it starts with
// and the ones a loaded module added.
//
// Derived on every call rather than stored, which is what makes `zmodload -u
// zsh/complist` take `menuselect` away again with no code of its own — and
// what keeps `bindkey -l`, `bindkey -M` and `$keymaps` from ever disagreeing,
// since all three ask this.
//
// Sorted, which is the order zsh's `bindkey -l` prints and is plain byte order
// with the dot-prefixed name first: `.safe command emacs isearch listscroll
// main menuselect vicmd viins viopp visual`, measured after the load.
func keymapsNow(r *interp.Runner) []string {
	out := append([]string(nil), keymapNames...)
	for _, module := range zmodloadLoaded(r) {
		out = append(out, moduleKeymaps[module]...)
	}
	sort.Strings(out)
	return out
}
