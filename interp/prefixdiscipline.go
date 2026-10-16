// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

	"github.com/blairham/sh/syntax"
)

// An assignment *prefix* — the `s=5` in `s=5 cmd` — reaches this shell's
// variable table on its way to the command it stands in front of, and the
// table is where a `.set` discipline is fired. That made every prefix look
// like an assignment to a hook watching the name, which is not what the
// facility's own shell does.
//
// The rule ksh93u+ 2012-08-01 keeps, measured a command kind at a time on
// 2026-09-16, is that **the hook fires where the value is actually stored**:
//
//	s=5 true                 no hook; `true` is a regular builtin and the
//	                         prefix is an entry in the environment it is
//	                         handed rather than a store in the shell
//	s=5 command true         no hook, for the same reason one word along
//	s=5 :                    fires; a special builtin's prefix persists,
//	                         so there is a store for it to be about
//	s=5 pf                   fires; a POSIX-form function's prefix persists
//	s=5 /usr/bin/env         fires — in the forked child, whose store it is:
//	                         `${.sh.value}` reaches the child's environment
//	                         and a variable the hook writes is gone in the
//	                         parent. Ours does not fork, and the row it
//	                         answers differently is filed rather than fixed.
//
// And the event is the one the operator names. `s+=5 :` enters `s.append`
// with `5` — the part being appended, the same value the bare `s+=5`
// statement gives it — where this shell entered `s.set` with `base5`, the
// join it had already made. An `.append` hook that logs what was added
// logged the whole value, and a shell with only a `.set` hook ran it for an
// append that should have run nothing.
//
// None of this is an axis. A discipline is ksh93's alone — see
// Semantics.DisciplineFunctionIsAVariableHook — so a dialect without one has
// no hook to fire and every line below is inert in it.

// prefixChildForTheStores is the throwaway shell an **external**
// command's assignment prefix stores into, or nil where there is nothing for
// one to be about.
//
// The rule interp/prefixdiscipline.go records — the hook fires where the
// value is actually stored — leaves one row unexplained: an external command's
// prefix fires a hook in ksh93u+ and fired none here, because this shell hands
// the child an environment entry rather than making a store at all. The reason
// it fires there is that the store **is** made, in the process that has just
// forked. Measured 2026-09-18 from a script file under `env -i` with a scratch
// HOME:
//
//	s=5 /usr/bin/env             SET[5], and the child is shown s=5
//	s=5 /bin/nosuchfile          SET[5], then `not found`
//	s=5 nosuchcmd99              SET[5], then `not found`
//	s+=5 /usr/bin/env            APP[5], and the child is shown s=base5
//	function s.set { .sh.value=REPLACED; }
//	                             the child is shown s=REPLACED
//	function s.set { t=HOOKRAN; }
//	                             the parent's `t` is empty afterwards
//	function s.set { return 7; } the command's status is the command's
//	export E=parent
//	function s.set { E=child; }  the child is shown E=child
//	a=1 b=2 /usr/bin/env         b's hook sees what a's hook wrote
//
// The last two are what decide the shape. One fork serves the whole prefix
// list, so the hooks share a state; and that state is what the child's
// environment is built from, so a hook's write to a *second* exported name
// reaches the child. A per-name save-and-restore cannot say either, because a
// hook may write any name at all — so this is a copy of the variable tables,
// made with the same ownTables a subshell is made with and thrown away when
// the command has been started.
//
// It is nil unless a store in it could hand the child something other than
// the text the prefix expanded to, which is what keeps an ordinary `PATH=/x
// cmd` from copying every table in the shell. Two things can:
//
//   - a hook really watching one of the names, above;
//   - a name carrying a **type** letter — the integer or float letters, a
//     case letter, a field width — in the dialect whose prefix writes over
//     the binding it displaces rather than into a fresh cell. The store
//     folds the value through those letters, and the fold is what the child
//     is handed. Measured 2026-10-15 under `-c`, `typeset -i v=3; v=2+2
//     /usr/bin/env` shows `v=4` in zsh 5.9.2 and ksh93u+ and `v=2+2` in bash
//     5.3.20; `typeset -u`, `-F`, `-i 16` and ksh93's `-R5`/`-L5`/`-Z5` go
//     the same way, and a value that will not evaluate (`v=1+`) is the
//     arithmetic complaint and status 1 for the command alone, with the
//     entries behind it never expanded. The store is the child's, so the
//     base `v=0x10` teaches the name (zsh shows the child `16#10`) is gone
//     again afterwards — which a store into a copy keeps for free (#6698).
//
// Otherwise the copy is invisible: the stores in it and the entries they
// produce are what prefixValue already computed.
//
// Not an axis of its own. A discipline is one shell's alone — see
// Semantics.DisciplineFunctionIsAVariableHook — and the type letters are
// Semantics.AssignmentPrefixMakesAFreshCell's question, asked only of a name
// that carries one.
func (r *Runner) prefixChildForTheStores(assigns []*syntax.Assign) *Runner {
	typed := r.anyNameCarriesATypeAttribute()
	if r.disciplined == nil && !typed {
		// Neither can be true of any name, so the answer needs no word about
		// the names. A dialect that starts with a typed parameter of its own
		// never takes this, and the per-name test below answers there.
		return nil
	}
	watching := false
	for _, a := range assigns {
		if a.Operand || prefixIsSubscripted(a) {
			continue
		}
		if r.disciplined != nil && (r.disciplineIsWatching(a.Name, disciplineSet) ||
			(a.Append && r.disciplineIsWatching(a.Name, disciplineAppend))) {
			watching = true
			break
		}
		if typed && r.prefixStoreFoldsThroughTheLetters(a) {
			watching = true
			break
		}
	}
	if !watching {
		return nil
	}
	child := *r
	child.dialectMoves = nil
	child.bodyTexts = nil
	child.ownTables(r)
	return &child
}

// prefixStoredForAChild is one prefix's store inside that child, and answers
// what the child should be handed for the name.
//
// The value is read back from the store rather than returned from the join,
// because a `.set` hook may replace it: `function s.set { .sh.value=REPLACED;
// }; s=5 /usr/bin/env` shows the child `s=REPLACED`. Read without the `.get`
// hook, which is a read of the store and not of the value's producer.
//
// The status the hook returned is left where it is. Measured: `function s.set
// { return 7; }; s=5 /usr/bin/env` reports the command's status and not 7,
// where the same hook in front of a *special builtin* does leave 7 — so the
// store's status belongs to the shell that made the store, and this one is
// about to be thrown away.
//
// And it is read the way the environment reads a name, not the way an
// expansion does: the case letters fold, and a field width does not pad.
// Measured 2026-10-15 on zsh 5.9.2, `typeset -R5 v; v=ab /usr/bin/env` shows
// the child `v=ab` where `$v` would be `   ab` — the same split
// Runner.exportedScalarEntry records for an exported name. The column that
// pads on the store (ksh93: `v=   ab`) has the padding in what is stored.
//
// failed is a store the type letters refused — a value that would not
// evaluate — which has said so and leaves the walk where an expansion that
// failed leaves it. See Runner.prefixWalkFailed.
func (r *Runner) prefixStoredForAChild(a *syntax.Assign, part string) (stored string, failed bool) {
	if a.Append {
		// The hook is given the part being appended and may rewrite it,
		// exactly as it is for the spellings that store in this shell.
		if v, ran := r.disciplineWrite(a.Name, disciplineAppend, "", part); ran {
			part = v
		}
		defer r.suppressDiscipline(a.Name, disciplineSet)()
	}
	before := r.expandErr || r.ctl != controlNone
	r.setVar(a.Name, r.prefixJoined(a, part))
	if !before && (r.expandErr || r.ctl != controlNone) {
		return "", true
	}
	stored, _ = r.storedValue(a.Name, false)
	return r.readCaseFolded(r.attributedName(a.Name), stored), false
}

// prefixStoreFoldsThroughTheLetters reports whether a prefix entry's store
// would make something other than its text of it, in the dialect whose prefix
// keeps the letters of the binding it lands on. See
// Runner.prefixChildForTheStores.
//
// The type letters are looked for first, so the question is never asked of a
// name that carries none — which is every prefix the common language writes.
//
// A positional parameter carries no letter, so it is never one; a readonly
// name is refused before any store, so it is never one either.
func (r *Runner) prefixStoreFoldsThroughTheLetters(a *syntax.Assign) bool {
	name := r.attributedName(a.Name)
	if !r.nameCarriesATypeAttribute(name) || r.attrHas(name, attrReadonly) {
		return false
	}
	return !r.prefixEntryIsFresh(name)
}

// prefixStore performs one assignment prefix's store, firing the discipline
// the store deserves.
//
// `stores` is whether the prefix reaches a store at all. The value still
// lands in the table either way — a regular builtin has to *see* `IFS=: read
// x`, and this shell shows it the value by assigning it and taking it back —
// so what a prefix that does not store suppresses is the event and not the
// write.
//
// `fresh` is whether the entry is a cell of the prefix's own rather than a
// write over the binding the name already had, which the caller decides
// because it is the caller that knows the binding is being saved. It is
// applied between the join and the store, not before both: an append joins
// what the displaced binding was showing and lands in the fresh cell. See
// interp/prefixfreshcell.go.
//
// `landsOn` is the name that cell belongs to, which a **name reference** makes
// different from `a.Name`: the store below goes through the reference and so
// is written as the script wrote it, while the cell it lands in is the
// target's and has to be emptied by its own name. Emptying the reference
// instead took the `n` letter off and turned the store into an ordinary write
// to the reference, leaving the target holding the binding the prefix was
// supposed to displace. See interp/namerefprefix.go (#4110).
func (r *Runner) prefixStore(ctx context.Context, a *syntax.Assign, landsOn string, stores, fresh bool) {
	value := r.prefixExpansion(a)
	if !r.assignmentGaveUp() && r.refuseADottedPrefix(a, value, true, stores) {
		// A dotted name the command will not take. See
		// interp/dottednoparent.go.
		return
	}
	if r.assignmentGaveUp() {
		// The value would not expand, so the command is given up and there
		// is nothing to store — the same answer the scalar store in
		// Runner.assign gives a bare `x=$((1/0))`. It shows where a prefix
		// outlives its command: measured 2026-10-15, BusyBox ash 1.38 run
		// interactively leaves `x=4` after `x=$((1/0)) eval …`, where
		// storing the empty value left it empty (#6544).
		return
	}
	if prefixIsSubscripted(a) {
		// The subscript names where the value goes, so the store is the
		// element write the same word performs as a statement — the
		// subscript evaluated, a key read as a key, `a[1]+=v` appending to
		// the element — and not a scalar the name never had. Reached only in
		// the dialects that store it at all; see interp/prefixsubscript.go.
		//
		// Through Runner.assign and not a store of its own, because every
		// branch of that switch is a shape a subscript can be written in and
		// a second copy of the dispatch is how the two come to disagree. The
		// value is handed over already expanded, the way the trace hands one
		// over, so `a[1]=$(date) cmd` runs its substitution once.
		if !stores {
			defer r.suppressDiscipline(a.Name, disciplineSet)()
			defer r.suppressDiscipline(a.Name, disciplineAppend)()
		}
		r.withPreparedValue(ctx, &expandedAssign{assign: a, value: value})
		return
	}
	if elems, ok := r.prefixArrayLiteral(a); ok {
		// An array literal read as the array it spells, stored as the
		// statement form stores one — the append, a subscripted element and
		// a dialect's compound reading with it — from the elements already
		// expanded, so a substitution in one runs once. See
		// Semantics.PrefixArrayLiteralIsAnArray.
		if !stores {
			defer r.suppressDiscipline(a.Name, disciplineSet)()
			defer r.suppressDiscipline(a.Name, disciplineAppend)()
		}
		if fresh {
			r.emptyPrefixEntry(landsOn)
		}
		r.withPreparedValue(ctx, &expandedAssign{assign: a, elems: elems, elemsSet: true})
		return
	}
	if fields, ok := r.prefixGlobFields(a); ok {
		// The value was read as a pattern and matched — the one shell that
		// asks for that, and only where the value was a pattern at all. The
		// match replaces the name outright, the kind and the numeric
		// attributes with it, so neither the join below nor the scalar store
		// is on its road. See interp/prefixglob.go.
		if !stores {
			defer r.suppressDiscipline(a.Name, disciplineSet)()
			defer r.suppressDiscipline(a.Name, disciplineAppend)()
		}
		if fresh {
			r.emptyPrefixEntry(landsOn)
		}
		r.storeGlobbedPrefix(a, fields)
		return
	}
	if !stores {
		defer r.suppressDiscipline(a.Name, disciplineSet)()
		defer r.suppressDiscipline(a.Name, disciplineAppend)()
		joined := r.prefixJoined(a, value)
		if fresh {
			r.emptyPrefixEntry(landsOn)
		}
		r.setVar(a.Name, joined)
		return
	}
	if a.Append {
		// The hook is given only the part being appended and may rewrite it,
		// which is what makes `s=base; function s.append { .sh.value="<${.sh.value}>"; }; s+=5 :`
		// leave `base<5>`. And an append fires *only* that event: the store
		// below is the same scalar store a plain assignment ends at, so the
		// mark is what keeps the two apart — the same pair of lines
		// Runner.assign already keeps for the bare statement.
		if v, ran := r.disciplineWrite(a.Name, disciplineAppend, "", value); ran {
			value = v
		}
		defer r.suppressDiscipline(a.Name, disciplineSet)()
	}
	joined := r.prefixJoined(a, value)
	if fresh {
		r.emptyPrefixEntry(landsOn)
	}
	r.setVar(a.Name, joined)
}
