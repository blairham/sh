// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A `{name}` redirection and an assignment prefix naming the same variable
// on one command this shell runs itself — a function or a builtin — and
// which of the two the command sees. Semantics.FdVariableStoredAfterThePrefix
// holds the measurements (#6569).

// fdVarStore is one `{name}` store a command's redirections made in this
// shell: the name as written and the number it received.
type fdVarStore struct{ ref, value string }

// fdVarStoresThroughThePrefix stores again, through the prefix, each number
// this command's redirections stored into a name its prefix also assigns, in
// the dialects where the store comes after the prefix.
//
// The redirections are opened first here (Runner.runSimple), so the first
// store went to the name the prefix then displaced or overwrote; storing the
// number again into what the prefix made is that order's answer without
// moving the redirections. Where the prefix is the call's own binding the
// first store is what is left once it is taken back — which is bash's
// `[10]` afterwards — and where the prefix persists the second store is.
//
// Asked only where a prefix entry names a stored name, which is rare: a
// command with no prefix, or no `{name}`, or the two over different names,
// asks nothing.
func (r *Runner) fdVarStoresThroughThePrefix(assigns []*syntax.Assign, stores []fdVarStore) {
	if len(stores) == 0 || len(assigns) == 0 {
		return
	}
	for _, st := range stores {
		if !prefixNames(assigns, st.ref) {
			continue
		}
		if !r.ask(r.sem().FdVariableStoredAfterThePrefix,
			"whether a `{name}` store on a command is seen through the command's prefix assignment of the same name") {
			return
		}
		r.setFdVar(st.ref, st.value, "")
	}
}

// prefixNames reports whether an entry of the prefix assigns ref, written
// the same way.
func prefixNames(assigns []*syntax.Assign, ref string) bool {
	for _, a := range assigns {
		if a.Name == ref {
			return true
		}
	}
	return false
}
