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

// prefixChildForTheDisciplines is the throwaway shell an **external**
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
// It is nil unless a hook is really watching one of the names, which is what
// keeps an ordinary `PATH=/x cmd` from copying every table in the shell. The
// copy is otherwise invisible: with no hook the stores in it and the entries
// they produce are what prefixValue already computed.
//
// Not an axis. A discipline is one shell's alone — see
// Semantics.DisciplineFunctionIsAVariableHook — so nothing here is reachable
// in a dialect that has none.
func (r *Runner) prefixChildForTheDisciplines(assigns []*syntax.Assign) *Runner {
	if r.disciplined == nil {
		return nil
	}
	watching := false
	for _, a := range assigns {
		if a.Operand || prefixIsSubscripted(a) {
			continue
		}
		if r.disciplineIsWatching(a.Name, disciplineSet) ||
			(a.Append && r.disciplineIsWatching(a.Name, disciplineAppend)) {
			watching = true
			break
		}
	}
	if !watching {
		return nil
	}
	child := *r
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
func (r *Runner) prefixStoredForAChild(a *syntax.Assign, part string) string {
	if a.Append {
		// The hook is given the part being appended and may rewrite it,
		// exactly as it is for the spellings that store in this shell.
		if v, ran := r.disciplineWrite(a.Name, disciplineAppend, "", part); ran {
			part = v
		}
		defer r.suppressDiscipline(a.Name, disciplineSet)()
	}
	r.setVar(a.Name, r.prefixJoined(a, part))
	stored, _ := r.storedValue(a.Name, true)
	return stored
}

// prefixStore performs one assignment prefix's store, firing the discipline
// the store deserves.
//
// `stores` is whether the prefix reaches a store at all. The value still
// lands in the table either way — a regular builtin has to *see* `IFS=: read
// x`, and this shell shows it the value by assigning it and taking it back —
// so what a prefix that does not store suppresses is the event and not the
// write.
func (r *Runner) prefixStore(ctx context.Context, a *syntax.Assign, stores bool) {
	value := r.prefixExpansion(a)
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
	if !stores {
		defer r.suppressDiscipline(a.Name, disciplineSet)()
		defer r.suppressDiscipline(a.Name, disciplineAppend)()
		r.setVar(a.Name, r.prefixJoined(a, value))
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
	r.setVar(a.Name, r.prefixJoined(a, value))
}
