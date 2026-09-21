// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Reading and writing variables in the frame a script has *selected*.
//
// One dialect lets a script point the shell at a frame further down the call
// stack — see callstack.go, where the selection itself lives — and the
// selection does not only decide which frame the two location parameters
// answer about. It moves the **variable scope** there too: a function that
// selects its caller reads the caller's locals, writes into the caller's
// cells, and cannot see its own. That is the other half of what a debug trap
// is for, and it is why the selection is a mechanism here rather than a
// wording in the dialect.
//
// Measured on AT&T ksh93u+ 2012-08-01 (`/bin/ksh`), 2026-09-20, with two
// nested functions each holding a local `v` and a local of its own:
//
//   - At the caller's level the callee's `v` is gone and the caller's is
//     there, and so is a name only the caller declared. A name only the
//     *callee* declared reads unset — so the move is a different scope and
//     not the callee's scope with the caller's names added to it.
//   - Level 0 is the globals. A name every frame declared local reads unset
//     there while a genuinely global name still reads, which is the
//     discriminating pair: "no locals in the way" would have answered the
//     innermost local instead.
//   - A write lands in the selected frame and stays there. Stepping back to
//     the callee's own level reads what the callee always held, and the
//     caller sees the write after the callee returns — so it is the caller's
//     cell that was written and not a copy that the unwind puts back.
//   - A write at level 0 to a name no frame has shadowed is an ordinary
//     global.
//
// **How it is done matters, because the scopes here are not a chain of
// tables.** A scope is a save-and-restore record: the one map of variables is
// written in place and each scope remembers what the names it shadowed held
// before it did. So the value a frame sees is not stored anywhere as such —
// it is the current value unless something inner to that frame displaced it,
// and then it is what the *innermost-but-outermost-of-those* scope saved.
// Walking up from the frame's own window and stopping at the first scope that
// shadowed the name is exactly that value, and writing into that scope's save
// is exactly a write to the frame's cell: the unwind will hand it back.
//
// A name nothing inner has shadowed therefore needs no diversion at all — the
// live tables already *are* the selected frame's view of it — which is what
// keeps this off every read of every variable in every other shell.
//
// **Narrowed on purpose**: only the scalar view moves. A name an inner frame
// shadowed while it held an array or a keyed table is left reading the way it
// read before, because those are two further tables with their own saves and
// answering half of one would be worse than answering none of it. `unset` and
// a declaration at a selected level are the same kind of unfinished and are
// left alone too. See #3115, which records the rest.

// markFrameScopeBase records where the innermost frame's scope window starts,
// which the caller does once the call's own scope has been pushed.
func (r *Runner) markFrameScopeBase() {
	if n := len(r.frames); n > 0 {
		r.frames[n-1].scopeBase = len(r.scopes)
	}
}

// divertingSelection is the level a script has selected, where the selection
// is live and names a frame other than the one the shell is standing in.
//
// False for the overwhelmingly common case of no selection, and false as well
// where a selection names the frame the shell is already standing in: that is
// the live state, so there is nothing to divert and no walk to pay for.
//
// One function rather than a guard per reader, because everything that moves
// with a selection — the variable scope here, the positional parameters and
// `$0` in interp/frameparams.go — has to agree about *when* it moves. Two
// copies of these four tests are two chances for one of them to keep
// answering after the other has stopped.
func (r *Runner) divertingSelection() (int, bool) {
	if !r.frameSelected {
		return 0, false
	}
	if r.frameSelectedAt != r.innermostFrameSerial() {
		// The selection belongs to a frame that is no longer the one
		// running, which is the same staleness SelectedCallFrame reads: a
		// callee sees its own answers, and the stamp is left for the frame
		// that made it to find when the callee returns.
		return 0, false
	}
	depth := r.FunctionDepth()
	if r.selectedFrame == depth {
		return 0, false
	}
	if r.selectedFrame > depth {
		// A level the stack has no frame at. The selection was refused when
		// it was made, so reaching here means the stack has shrunk under it;
		// the live state is the honest answer.
		return 0, false
	}
	if r.selectedFrame < 0 {
		// Only reachable from the top level, where any number is kept — see
		// SelectCallFrame. Read as the top level, which is what the number
		// is below.
		return 0, true
	}
	return r.selectedFrame, true
}

// selectedScopeBase is the first scope index inner to the frame a script has
// selected, and whether a selection is moving the scope at all.
func (r *Runner) selectedScopeBase() (int, bool) {
	level, ok := r.divertingSelection()
	if !ok {
		return 0, false
	}
	if level == 0 {
		// The top level, where every scope on the stack was opened by
		// something the script called.
		return 0, true
	}
	// CallStack is innermost first, so the innermost function stands at the
	// current depth and each one further out is a level lower.
	at := r.FunctionDepth()
	for _, f := range r.CallStack() {
		if !f.IsFunction() {
			continue
		}
		if at == level {
			return f.scopeBase, true
		}
		at--
	}
	return 0, false
}

// frameCell is where the selected frame's value of one name is actually
// kept, which is not one place: a scope that *shadowed* the name holds it in
// its save, and a scope whose call was **sealed** holds it in the seal.
//
// Both are on the same walk because both are ways a frame's value got out of
// the live table, and which one applies is per scope and per name. Under the
// dialect that seals — the same one that has this selection, which is why the
// two meet at all — a call puts every caller local aside and installs what the
// name held before the caller declared it. So the callee's scope has *both*
// records for such a name, and the seal is the one that holds the caller's
// value: the save underneath it holds what the seal installed, which is a
// value from further out still. See interp/staticscope.go.
type frameCell struct {
	sc     *scope
	name   string
	sealed bool
}

// divertingCell is where the selected frame's value of one name lives: the
// first scope, from the frame's window up, that took the name out of the live
// table. Nil when nothing between here and there touched it, which means the
// live table holds the frame's own value and no diversion is needed.
func (r *Runner) divertingCell(name string) *frameCell {
	base, ok := r.selectedScopeBase()
	if !ok {
		return nil
	}
	for i := base; i < len(r.scopes); i++ {
		sc := r.scopes[i]
		if _, held := sc.sealed[name]; held {
			return &frameCell{sc: sc, name: name, sealed: true}
		}
		if _, saved := sc.saved[name]; !saved {
			continue
		}
		if sc.arrayExisted[name] || sc.assocExisted[name] {
			// The name held an array or a keyed table when it was displaced,
			// and this only moves the scalar view. Left alone rather than
			// answered from the scalar copy, which would report one element
			// as though it were the whole name.
			return nil
		}
		return &frameCell{sc: sc, name: name}
	}
	return nil
}

// valueInSelectedFrame answers a scalar read from the selected frame, and
// reports whether it answered at all.
//
// Set-ness comes from the same record, which is the half a probe turns on: a
// name the selected frame never had reads unset there even though the frame
// the shell is standing in declared it.
func (r *Runner) valueInSelectedFrame(name string) (value string, set, diverted bool) {
	cell := r.divertingCell(name)
	if cell == nil {
		return "", false, false
	}
	if cell.sealed {
		held := cell.sc.sealed[name].held
		if held.removed || !held.valueExists {
			return "", false, true
		}
		return held.value, true, true
	}
	if cell.sc.removedBefore[name] {
		// `unset` had already taken the name away in that frame, and the
		// save is what it held before that — putting it back here would
		// resurrect a value the frame had got rid of.
		return "", false, true
	}
	if !cell.sc.existed[name] {
		return "", false, true
	}
	return cell.sc.saved[name], true, true
}

// storeIntoSelectedFrame writes a scalar into the selected frame, and reports
// whether it did.
//
// Into the record rather than into the live table, which is what makes the
// write the selected frame's: the frame's own return is what hands the record
// back, and the frame the shell is standing in keeps the value it had.
func (r *Runner) storeIntoSelectedFrame(name, value string) bool {
	cell := r.divertingCell(name)
	if cell == nil {
		return false
	}
	if cell.sealed {
		sn := cell.sc.sealed[name]
		sn.held.value, sn.held.valueExists, sn.held.hasValue = value, true, true
		sn.held.removed, sn.held.hasRemoved = false, true
		cell.sc.sealed[name] = sn
		return true
	}
	cell.sc.saved[name] = value
	cell.sc.existed[name] = true
	if cell.sc.removedBefore != nil {
		// The name is there again in that frame, so the unwind must not
		// take it away as one `unset` had already removed.
		cell.sc.removedBefore[name] = false
	}
	return true
}
