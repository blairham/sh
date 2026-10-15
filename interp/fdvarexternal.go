// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "slices"

// A `{name}` redirection on a command that becomes a process of its own is
// made in that process, so the shell that ran the command keeps neither the
// name nor the descriptor.
//
// Measured 2026-10-15 with `-c` against bash 5.3.20, zsh 5.9.2 under `-f`
// and ksh93u+ 2012-08-01 (/bin/ksh), the program on one line and `echo
// "[$v]"` after it:
//
//	                                        bash   zsh    ksh93
//	/usr/bin/true {v}</dev/null             []     []     []
//	nosuch {v}</dev/null   (not found)      []     []     []
//	command /usr/bin/true {v}</dev/null     []     []     []
//	command true {v}</dev/null              [10]   []     [10]
//	true {v}</dev/null     (builtin)        [10]   [11]   [10]
//	f {v}</dev/null        (function)       [10]   [11]   [10]
//	{ :; } {v}</dev/null   (group)          [10]   refused [10]
//	v=keep; /usr/bin/true {v}</dev/null     [keep] [keep] [keep]
//	typeset -i v=3; …; typeset -p v         -i 3   -i 3   -i 3
//	/usr/bin/true {v}</dev/null; exec {x}…  x=10   x=11   x=10
//
// Unanimous wherever the command is a program, so a correction rather than
// an axis: the name is put back as it was — value, attributes and all — and
// the descriptor is not kept, which the last row is about. Ours left 10 open
// in the shell, so the next `{x}` picked 11. The `command true` row is zsh's
// `command` reaching only programs, which commandRunsInThisShell already
// answers; nothing here asks it again.
//
// The name is still *written* for the length of the command, because the
// command's own later redirections are made in the same process and read it:
// `/bin/cat {v}<f 0<&$v` reads f in all three, and putting the name back
// before the list was done made `$v` empty there.

// fdVarOutlivesItsCommand reports whether the descriptor a `{name}`
// redirection picks stays open once the command carrying it has ended.
//
// One question, asked from one place, because the three kinds of
// redirection that pick a descriptor — a file, a duplication, a here-document
// — each asked it on their own, and a fourth would be one more place to
// forget the external-command case below.
//
// A command that is a process of its own answers no without asking: every
// column closes it with the process. Asking there would also have put the
// axis on a path where the shells agree.
func (r *Runner) fdVarOutlivesItsCommand() bool {
	if r.redirOwner == redirOwnerTheCommand {
		return false
	}
	return r.ask(r.sem().FdVariableOutlivesTheCommand,
		"a variable-named descriptor outliving its command") &&
		r.FdVariableDescriptorOutlivesTheCommand()
}

// fdVarsInTheCommand holds what a `{name}` store will overwrite on a command
// that is a process of its own, and gives it back when the command ends.
//
// The whole of the expansion tables rather than the one name, for the reason
// saveExpansionTables exists: a name can be an element, a key, or a nameref
// to either, and the tables are what every one of those doors writes. The
// positional parameters go with them, which is the one dialect's
// `{1}<file`. Taken once per redirection list — a second copy would hold the
// first store, and putting that back is the leak this is here to stop.
type fdVarsInTheCommand struct {
	held   bool
	tables expansionTables
	params []string
}

// beforeStore takes the copy, once, for a command that is a process of its
// own, and returns the closer that puts it back — nil where there is nothing
// to put back.
func (h *fdVarsInTheCommand) beforeStore(r *Runner) closerFunc {
	if h.held || r.redirOwner != redirOwnerTheCommand {
		return nil
	}
	h.held = true
	h.tables = r.saveExpansionTables()
	h.params = slices.Clone(r.Params)
	return func() error {
		r.restoreExpansionTables(h.tables)
		r.Params = h.params
		return nil
	}
}
