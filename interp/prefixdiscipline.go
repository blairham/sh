// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

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

// prefixStore performs one assignment prefix's store, firing the discipline
// the store deserves.
//
// `stores` is whether the prefix reaches a store at all. The value still
// lands in the table either way — a regular builtin has to *see* `IFS=: read
// x`, and this shell shows it the value by assigning it and taking it back —
// so what a prefix that does not store suppresses is the event and not the
// write.
func (r *Runner) prefixStore(a *syntax.Assign, stores bool) {
	value := r.prefixExpansion(a)
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
