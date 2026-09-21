// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Restoring a `local` is an assignment moment, and the names whose assignment
// *does* something have to hear it.
//
// SetAssignmentAction is delivered from the store, so every **written**
// assignment reaches it: plain, `export`, `declare`, `typeset -i`, a command
// prefix, `((name=2))`, `printf -v`, and `local` itself on the way in. What no
// route reached was the frame being popped, where the value a name resolves to
// changes with nothing being written — and two of bash's history parameters
// act on the value moving, so both were wrong at exactly that moment (#4045).
//
// Measured 2026-09-21 on bash 5.3.20, `env -i` with a scratch HOME and
// HISTIGNORE keeping the reader's own lines out of the list, so that what the
// return did is what the listing shows:
//
//	HISTSIZE=2, three entries, then
//	fn() { local HISTSIZE=9; three more; history; }; fn; history
//	    inside fn     2 b · 3 c · 4 d · 5 e · 6 g
//	    after it      3 e · 4 g
//
//	HISTFILESIZE=2, then
//	fn() { local HISTFILESIZE=9; five lines > $HISTFILE; }; fn
//	    inside fn     a b c d e
//	    after it      d e
//
// Nothing ran between the return and the listing in the first, so the trim is
// the restore and not a later entry; and the file **grows** after the inner
// assignment in the second, so the truncation cannot be the one
// `HISTFILESIZE=9` did on the way in. The alternative reading — catching up at
// the next entry — is the lazy one this repository has already rejected for
// HISTFILESIZE (#3423): a `history` that looks first sees the longer list.
//
// Three rules, each measured rather than chosen:
//
//   - Only where the **visible value actually changed**. A `local HISTSIZE=2`
//     under an outer `HISTSIZE=2` restores to the same text, and bash's list
//     is untouched across the return.
//   - A name the restore leaves **unset** sends nothing. Measured: with
//     HISTSIZE unset outside and `local HISTSIZE=2` inside, the list grows
//     again after the return, which is what an absent HISTSIZE already means
//     to the reader — there is no message to deliver, and an `unset` is not
//     what happened.
//   - A name the restore leaves **set** sends the value, whether it was set
//     before or not: `local HISTSIZE` with no value under an outer
//     `HISTSIZE=9` lifts the cap inside the call and puts nine back at the
//     return, and bash keeps the six entries the body made.
//
// Per name and not per scope: the map is consulted only for names this scope
// actually shadowed, so a call that declared nothing a dialect registered
// pays a length check.

// restoredActionValues is what the names this scope shadowed resolve to
// before any of them is put back, for the ones whose assignment does
// something. Nil where no dialect registered an action, which is every shell
// that does not have one.
func (r *Runner) restoredActionValues(names []string) map[string]string {
	if len(r.assignmentActions) == 0 {
		return nil
	}
	var before map[string]string
	for _, name := range names {
		if _, ok := r.assignmentActions[name]; !ok {
			continue
		}
		if before == nil {
			before = map[string]string{}
		}
		// The absent value spelled as the empty string would be a third
		// name for it: a name that was unset and comes back holding "" has
		// not moved, and one that held "" and comes back unset sends
		// nothing either way. The sentinel keeps the two apart.
		if value, ok := r.getVar(name); ok {
			before[name] = value
		}
	}
	return before
}

// speakRestoredAssignments delivers the message for every name whose visible
// value the unwind moved. Run once the scope is fully closed, so an action
// reading the name back sees what the caller sees.
func (r *Runner) speakRestoredAssignments(before map[string]string, names []string) {
	if before == nil {
		return
	}
	for _, name := range names {
		act, ok := r.assignmentActions[name]
		if !ok {
			continue
		}
		value, ok := r.getVar(name)
		if !ok {
			// Gone rather than moved: see the third rule above.
			continue
		}
		if was, had := before[name]; had && was == value {
			continue
		}
		act(r, value)
	}
}
