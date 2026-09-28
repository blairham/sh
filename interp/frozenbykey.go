// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// MarkProducedTableFrozenByKey marks a produced association whose **elements**
// are frozen and whose table as a whole is not — a third state beside readonly
// and writable, and the only reason it is a state rather than a reading of
// those two is that a shell in the panel has a table in it.
//
// Measured 2026-09-28 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f` from a
// script file under `env -i PATH=/usr/bin:/bin TERM=dumb` with a scratch HOME,
// `zmodload zsh/langinfo` on the line above each row and one shell per row.
// The controls are the two neighboring shapes, measured in the same run, and
// they are what make this a third state rather than "langinfo is not
// readonly":
//
//	                        $sysparams, $parameters   $aliases, $commands   $langinfo
//	m[k]=v                  read-only variable: m     taken                 read-only variable: **k**
//	unset 'm[k]'            read-only variable: m     taken                 read-only variable: **k**
//	m=(A 1)                 read-only variable: m     taken                 taken and discarded, 0
//	m=([A]=1)               read-only variable: m     taken                 taken and discarded, 0
//	unset m                 read-only variable: m     taken, ${+m} is 0     taken and discarded, ${+m} is 1
//	${(t)m}                 …-readonly-hide-…         …                     association-hide-hideval-special
//
// So the refusal names the **key** and not the parameter, and the table-level
// writes are accepted, do nothing, and say nothing. `typeset -p langinfo`
// there is `typeset -A langinfo` — no `r` — which is the tell that the freeze
// is not the parameter's attribute, and is the row #4996 was filed for.
//
// **Two shapes have no answer to match, because the reference dies on them.**
// `langinfo+=(B 2)` aborts (status 134) and `langinfo=()` segfaults (status
// 139), both reproducibly, twice each, under the same harness that measures
// every row above at 0. A crash is not a behavior to model, so both take the
// table-level answer here — taken and discarded — as the nearest defined
// neighbor, and this paragraph is the record that it was chosen rather than
// measured.
//
// The mark is per name rather than a property of "a produced table with no
// writer", which is the reading that fits `langinfo` alone and would be wrong
// for the two frozen columns above: those are produced tables with no writer
// too, and they refuse everything by name.
//
// **`langinfo` is the only name in this state**, and that is swept rather than
// assumed. All forty-five module parameters the two shells both have were read
// back through `${(t)}` in one run each on 2026-09-28, under the harness above
// with `zmodload zsh/parameter zsh/langinfo zsh/mapfile zsh/system
// zsh/datetime zsh/terminfo zsh/termcap zsh/zleparameter zsh/sched` and
// `TERM=xterm-256color`; forty-five rows came back from each shell and
// `langinfo` was the one that differed. The control is the same sweep against
// this shell built at the commit before this one, which differs on `langinfo`
// and on nothing else — so the agreement is a measurement and not an empty
// instrument. `$terminfo` and `$termcap` are the rows that keep this from
// being "a module association is not readonly": both are
// `association-readonly-hide-hideval-special` there, and `terminfo[cols]=x` is
// `read-only variable: terminfo` — the parameter's name, not the key's.
func (r *Runner) MarkProducedTableFrozenByKey(name string) {
	if r.tableFrozenByKey == nil {
		r.tableFrozenByKey = map[string]bool{}
	}
	r.tableFrozenByKey[name] = true
}

// refuseFrozenTableElement refuses a write to one element of such a table,
// naming the key.
//
// The sentence, the status and what the refusal costs are the readonly path's
// — shared with it rather than written twice, because every one of those is an
// answer this tree has already measured and a second copy is a second place
// for them to drift. What this adds is the **name in the sentence**: the key
// rather than the parameter, which is the whole of the difference the
// reference has.
func (r *Runner) refuseFrozenTableElement(name, key string) bool {
	if !r.frozenByKeyAndStillProduced(name) {
		return false
	}
	return r.refuseWithTheReadonlySentence(key, assignedAlone,
		r.sem().ReadonlyReassignmentFatal)
}

// frozenByKeyAndStillProduced is the mark **and** the producer still standing
// behind the name, which is one question and not two.
//
// A hidden shadow suspends the producer for its scope — `suspendProducer`
// takes the name out of r.DynamicAssocs — and the name inside that scope is an
// ordinary association a script declared, so there is nothing of the shell's
// left to freeze. That is the same lifting the readonly path gets from
// `shadowStartsHiding`'s `delete(r.readonly, name)`, reached through the
// producer rather than through a second saved map, so `+h` putting the
// producer back puts the freeze back with it and neither can drift from the
// other.
//
// It is measured and not reasoned. zsh 5.9.2 takes
// `f(){ local -A langinfo; langinfo[A]=1; print $langinfo[A] }` as `1` and
// leaves `$langinfo[CODESET]` standing outside the function; asking the mark
// alone refused it as `read-only variable: A`, which is the shape #2586 had to
// fix for `local EPOCHSECONDS=5` and the reason the two questions are one.
func (r *Runner) frozenByKeyAndStillProduced(name string) bool {
	if !r.tableFrozenByKey[name] {
		return false
	}
	_, produced := r.DynamicAssocs[name]
	return produced
}

// producedTableDiscardsAWholeWrite reports whether a write to the whole of
// this table is taken, does nothing, and says nothing — which is the `m=(A 1)`
// and `unset m` half of the state MarkProducedTableFrozenByKey documents.
func (r *Runner) producedTableDiscardsAWholeWrite(name string) bool {
	return r.frozenByKeyAndStillProduced(name)
}
