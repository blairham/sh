// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"slices"
	"strconv"

	"github.com/blairham/sh/interp"
)

// A keymap name and a keymap are two things, and `bindkey -N` and `bindkey
// -A` are what pull them apart (#5969).
//
// `-N new [old]` makes a keymap and gives it a name, empty or a copy of old's
// bindings; `-A old new` makes new one more name for the keymap old names. The
// names have equal standing, so a key bound through either is bound in both,
// and `main` is one of them: `bindkey -A mymap main` is what selects a keymap
// a script made. Measured 2026-10-05 on zsh 5.9.2 under `zsh -f`:
//
//	bindkey -N mymap emacs          status 0, `mymap` listed by `-l`, and
//	                                `-M mymap '^A'` is beginning-of-line —
//	                                117 bindings, the same as emacs
//	bindkey -N e2                   empty: `-M e2 '^A'` is undefined-key
//	bindkey -A emacs foo            `-M foo '^Xq' undo` binds it in emacs too;
//	                                `-lL foo` is `bindkey -A emacs foo`
//	bindkey -N foo emacs; bindkey -M foo '^Xq' undo; bindkey -N foo
//	                                `-M foo '^Xq'` is undefined-key again
//	bindkey -A mymap main           `-lL main` is `bindkey -A mymap main`, and
//	                                a plain `bindkey '^Xz' undo` binds in mymap
//	bindkey -A emacs mymap          main stays on the keymap mymap made, and
//	                                `-lL main` still says `-A mymap main`
//
// That last row is why the listing names a keymap by the name it was *made*
// with rather than by whichever names reach it now. The refusals, each status
// 1: `-N foo nosuch` and `-A nosuch foo` are "no such keymap `nosuch'", `-N`
// and `-A x` are "not enough arguments for -N/-A", `-N a b c` and `-A e a b`
// are "too many arguments", and `.safe` as the new name is "keymap name
// `.safe' is protected".
//
// The keymap `main` names is still bindkeyMap and a binding is still stored
// against the keymap it is in, so the two built-in keymaps keep their own
// names as their identity and nothing written before this changes. A made
// keymap gets an identity no name can spell, so that renaming over it cannot
// reach it by accident.
const (
	// keymapNamesStore is the names laid over the built-in ones: flat
	// pairs of name and keymap.
	keymapNamesStore = ".zsh.keymap.names"
	// keymapMadeStore is the keymaps `-N` made: flat triples of keymap, the
	// name it was made with, and the built-in keymap it descends from (empty
	// for one made empty).
	keymapMadeStore = ".zsh.keymap.made"
)

// keymapTable is every keymap name this shell has and the keymap each names,
// with `main` resolved to the selected one.
func keymapTable(r *interp.Runner) map[string]string {
	out := map[string]string{}
	for _, name := range builtinKeymapNames(r) {
		out[name] = name
	}
	flat, _ := r.GetArray(keymapNamesStore)
	for i := 0; i+2 <= len(flat); i += 2 {
		out[flat[i]] = flat[i+1]
	}
	out["main"] = currentKeymap(r)
	return out
}

// keymapID is the keymap a name names, and whether it names one.
func keymapID(r *interp.Runner, name string) (string, bool) {
	id, ok := keymapTable(r)[name]
	return id, ok
}

// madeKeymap is what `-N` recorded about a keymap it made.
func madeKeymap(r *interp.Runner, id string) (primary, base string, ok bool) {
	flat, _ := r.GetArray(keymapMadeStore)
	for i := 0; i+3 <= len(flat); i += 3 {
		if flat[i] == id {
			return flat[i+1], flat[i+2], true
		}
	}
	return "", "", false
}

// keymapBase is the built-in keymap a keymap descends from: itself for a
// built-in one, and for a made one the one it was copied from. It is what
// decides the editor's mode — this editor's vi editing is a mode and not a
// table, so a copy of viins selected as main has to be told it is one.
func keymapBase(r *interp.Runner, id string) string {
	if _, base, ok := madeKeymap(r, id); ok {
		return base
	}
	return id
}

// keymapPrimary is the name a keymap is listed under: the one it was made
// with, or its own for a built-in one.
func keymapPrimary(r *interp.Runner, id string) string {
	if primary, _, ok := madeKeymap(r, id); ok {
		return primary
	}
	return id
}

// nameKeymap points a name at a keymap, which is `-A` and the second half of
// `-N`, and for `main` is selecting it.
func nameKeymap(r *interp.Runner, name, id string) {
	if name == "main" {
		selectKeymap(r, id)
		return
	}
	flat, _ := r.GetArray(keymapNamesStore)
	for i := 0; i+2 <= len(flat); i += 2 {
		if flat[i] == name {
			flat[i+1] = id
			r.SetArray(keymapNamesStore, flat)
			return
		}
	}
	r.SetArray(keymapNamesStore, append(flat, name, id))
}

// bindkeyNew is `bindkey -N new [old]`.
func bindkeyNew(r *interp.Runner, args []string) int {
	switch {
	case len(args) < 1:
		return bindkeyShort(r, "-N")
	case len(args) > 2:
		r.Diagnosef("too many arguments for -N\n")
		return 1
	}
	name := args[0]
	if protectedKeymap(r, name) {
		return 1
	}
	var from, base string
	if len(args) == 2 {
		id, ok := keymapID(r, args[1])
		if !ok {
			r.Diagnosef("no such keymap `%s'\n", args[1])
			return 1
		}
		from, base = id, keymapBase(r, id)
	}
	made, _ := r.GetArray(keymapMadeStore)
	// `+` and a number: no keymap name a script writes is spelled like this
	// and reaches here, because every name goes through keymapTable first.
	id := "+" + strconv.Itoa(len(made)/3+1)
	r.SetArray(keymapMadeStore, append(made, id, name, base))
	if from != "" {
		// A duplicate: every binding the old keymap answers with, the
		// defaults included, stored against the new one, so that a later
		// change to either is that one's alone.
		flat, _ := r.GetArray(bindkeyStore)
		table := readBindings(r, from)
		seqs := make([]string, 0, len(table))
		for seq := range table {
			seqs = append(seqs, seq)
		}
		slices.Sort(seqs)
		for _, seq := range seqs {
			flat = append(flat, id, seq, table[seq])
		}
		r.SetArray(bindkeyStore, flat)
	}
	nameKeymap(r, name, id)
	return 0
}

// bindkeyLink is `bindkey -A old new`.
func bindkeyLink(r *interp.Runner, args []string) int {
	switch {
	case len(args) < 2:
		return bindkeyShort(r, "-A")
	case len(args) > 2:
		r.Diagnosef("too many arguments for -A\n")
		return 1
	}
	id, ok := keymapID(r, args[0])
	if !ok {
		r.Diagnosef("no such keymap `%s'\n", args[0])
		return 1
	}
	if protectedKeymap(r, args[1]) {
		return 1
	}
	nameKeymap(r, args[1], id)
	return 0
}

// protectedKeymap refuses `.safe` as a name to make or point elsewhere: it is
// the keymap the editor falls back on and no script may take it away.
func protectedKeymap(r *interp.Runner, name string) bool {
	if name != ".safe" {
		return false
	}
	r.Diagnosef("keymap name `%s' is protected\n", name)
	return true
}
