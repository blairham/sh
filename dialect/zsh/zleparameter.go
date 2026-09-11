// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"github.com/blairham/sh/interp"
)

// The `zsh/zleparameter` module: the line editor's own tables, read as
// parameters.
//
// Measured 2026-09-09 against zsh 5.9.2 (Homebrew, aarch64) with a scratch
// HOME and no startup files. Two features, `p:widgets` and `p:keymaps`, and
// neither is a table this file keeps — `zle -N` already records the widgets in
// zle.go and `bindkey` already knows the keymaps in bindkey.go, so this is one
// more view over each and nothing else.
//
// # Its absence took a plugin down, which is why it is a P1
//
// #1618 records the consequence rather than the module. Two syntax
// highlighters on the maintainer's machine open with
//
//	zmodload zsh/zleparameter 2>/dev/null || {
//	  print -r -- >&2 'zsh-syntax-highlighting: failed loading zsh/zleparameter.'
//	  return 1
//	}
//
// and a third plugin does the same, so `failed to load module` was followed by
// `failed binding ZLE widgets, exiting` and the plugin did not load **at all**
// — no highlighting, and a startup carrying two error lines that named a
// module rather than anything a person could act on.
//
// # `$widgets` says what each widget *is*, and the three spellings are exact
//
//	builtin                  one the editor performs itself
//	user:fn                  one `zle -N name fn` defined, naming its function
//	completion:widget:fn     one `zle -C` defined
//
// Measured name by name: `zle -N mywidget` makes `$widgets[mywidget]` read
// `user:mywidget` — the function's name, which for that spelling is the
// widget's own — and `zle -N other f` makes `$widgets[other]` read `user:f`.
// A widget aliased with `zle -A accept-line aliased` reads `builtin`, the same
// as the widget it was aliased to, so an alias is not a fourth kind.
//
// All three are produced, the completion one included: `zle -C comp
// .complete-word _main_complete` makes `$widgets[comp]` read
// `completion:.complete-word:_main_complete`, which is the completer this
// shell will run and then the function it calls — in that order, measured.
//
// The *roster* is this shell's own actions and not zsh's 386, which is
// listedWidgets' reasoning followed exactly: a name in the answer is a claim
// that pressing a key bound to it does something, so `zle -la` prints this
// shell's list and so does this.
//
// # `$keymaps` is an array, not an association
//
// `typeset -p keymaps` writes `typeset -ar keymaps` where `typeset -p widgets`
// writes `typeset -Ar widgets`, so the keymaps are a list of names and the
// widgets are a table of name to kind. Both readonly, and `unset widgets` is
// `read-only variable` as well.
//
// The order is this shell's rather than zsh's, and deliberately: zsh's is its
// hash table's — `visual viopp command .safe vicmd main isearch viins emacs`,
// which is not sorted and not the definition order either — while `bindkey -l`
// in the same shell prints the same nine sorted. A list with no stated order
// is not one to imitate, so this is the sorted one, which is also the order
// this shell's own `bindkey -l` writes and cannot drift from it: both read
// keymapNames.
//
// # A name that is not a widget reads empty, and that is right
//
// `${widgets[nosuch]}` is empty at `${+widgets[nosuch]}` of 0 in zsh, and the
// same here — no [interp.Runner.SetAbsentElements] on this one, unlike
// `$terminfo` and `$langinfo`. The difference is that the table is *complete*:
// this shell knows every widget it has, so a name that is not in it is a name
// that is not a widget, which is a fact and not a gap. Nothing is being hidden
// by the empty string.

// registerZleParameterModule installs `$widgets` and `$keymaps`.
func registerZleParameterModule(r *interp.Runner) {
	r.SetDynamicAssoc("widgets", zshWidgetsView)
	// Readonly rather than given a writer, which is zsh's own answer and the
	// pair parameter.go and terminfo.go describe: a produced table with
	// neither would take an assignment into a stored table, and a stored
	// table is what a later read finds first — so one `widgets[x]=y` would
	// turn the view into a snapshot that never says it stopped tracking.
	r.MarkReadonly("widgets")
	r.MarkHidden("widgets")
	r.SetDynamicArray("keymaps", zshKeymapsView)
	r.MarkReadonly("keymaps")
	r.MarkHidden("keymaps")
}

// The values `$widgets` gives, which are the kinds of widget there are.
const (
	// zshWidgetBuiltin is one the editor performs itself.
	zshWidgetBuiltin = "builtin"
	// zshWidgetUserPrefix precedes the *function's* name for a widget `zle
	// -N` defined, which is the widget's own name where the definition gave
	// no other.
	zshWidgetUserPrefix = "user:"
	// zshWidgetCompletionPrefix opens the three-part spelling `zle -C`
	// defines: the completer this shell runs, then the function it calls,
	// separated by the same colon.
	zshWidgetCompletionPrefix = "completion:"
	zshWidgetSeparator        = ":"
)

// zshWidgetsView is `$widgets`: every widget this shell has, and what each one
// is.
//
// A view and not a snapshot, which this table needs more than most: `zle -N`
// is what a plugin runs, so a shell that read the parameter during startup and
// kept the answer would report the widgets that existed before the plugins
// loaded. Measured against zsh, defining one raises `${#widgets}` by one in
// the same shell.
//
// The editor's own actions are laid down first and a definition writes over
// one of the same name, which is the order zsh answers in: `zle -N
// accept-line myfn` makes `$widgets[accept-line]` read `user:myfn`, since a
// definition shadows the action it takes the name of.
func zshWidgetsView(r *interp.Runner) interp.AssocArray {
	out := make(interp.AssocArray, len(bindkeyWidgets))
	for name := range bindkeyWidgets {
		out[name] = zshWidgetBuiltin
	}
	for name, def := range readWidgets(r) {
		out[name] = zshWidgetSpelling(def)
	}
	return out
}

// zshWidgetSpelling is what one defined widget reads as: the completion form
// when `zle -C` named a completer, and the user form otherwise.
//
// The completer is what makes it the first: measured, that is the field `zle
// -C` fills and `zle -N` leaves empty, and it is the same field zle.go's own
// listing branches on — so the two spellings cannot disagree about which kind
// a widget is.
func zshWidgetSpelling(def widgetDefinition) string {
	if def.completer != "" {
		return zshWidgetCompletionPrefix + def.completer + zshWidgetSeparator + def.function
	}
	return zshWidgetUserPrefix + def.function
}

// zshKeymapsView is `$keymaps`: the keymaps this shell has, by name.
//
// keymapsNow rather than a list of its own, which is what keeps this and
// `bindkey -l` from ever disagreeing — including about the keymaps a module
// added, since that function derives the list from what is loaded rather than
// keeping a second copy. It sorts, so the order needs nothing done to it here.
func zshKeymapsView(r *interp.Runner) []string {
	return keymapsNow(r)
}
