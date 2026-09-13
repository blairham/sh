// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"
	"strings"
)

// The command hash: where PATH last found a name, kept so the next run of it
// does not walk PATH again.
//
// Every shell in the panel keeps one and every one of them exposes it through
// `hash`, so this is not an optimisation with a builtin bolted to it — the
// table is observable and scripts read it. Until #2554 this shell kept no
// table at all and `hash` said so in as many words, which made the builtin
// honest and left four letters of it unanswerable.
//
// What goes in is a **bare name that PATH resolved**, at the moment it is
// run. Measured 2026-09-13 across bash 5.3.15, zsh 5.9.2, ksh93 and dash, and
// unanimous in all four directions:
//
//	ls >/dev/null; hash            the name is in the table
//	type ls; command -v ls; hash   neither puts it there
//	command ls >/dev/null; hash    `command` does
//	(ls >/dev/null); hash          a subshell's entry is the subshell's
//	cd /; hash                     moving does not empty it
//	PATH=$PATH; hash               *assigning* PATH does
//
// The one thing they part on is what a **stale** entry means, and that is
// Semantics.CommandHashIsTrusted.
//
// One measurement here has no field and is prose in docs/spec/semantics.md
// instead: bash and dash empty the table *again* when a `PATH=… cmd` prefix
// is taken back, and zsh and ksh93 do not. It is a real 2-2 split and it
// cannot be asked yet, because a prefix does not reach this shell's own PATH
// for an external command at all (#2626) — the branch would be one no run
// could take.
type hashedCommand struct {
	// path is what the name resolved to.
	path string
	// hits is how many lookups this entry has answered since it was made —
	// a run, a `type`, a `command -v`, a `hash -t`, and not a listing. See
	// hashCommandHit. One dialect prints it; the rest keep no such column,
	// and `hash -p` and an explicit `hash name` start it at zero in the one
	// that does.
	hits int
}

// hashCommandRun records a name PATH has just resolved for a command about to
// run.
//
// **Only when it is not there already**, because the count is kept by the
// *lookup* and not by the run — see hashCommandHit. A name already in the
// table was counted on the way through lookPath, and counting it again here
// would make an execution worth two of anything else.
//
// Only a bare name: a word with a slash in it never went through PATH, so
// there is nothing for a table of PATH results to remember about it.
func (r *Runner) hashCommandRun(name, path string) {
	if strings.ContainsRune(name, '/') {
		return
	}
	if r.hashingIsOff() {
		return
	}
	if _, ok := r.cmdHash[name]; ok {
		return
	}
	r.putHashedCommand(name, path, 1)
}

// hashCommandHit counts a lookup that the table answered.
//
// Every lookup, and not only the ones that go on to run something. Measured
// 2026-09-13 against bash 5.3.15 with `ls` hashed at 1: `type ls`, `command
// -v ls` and `hash -t ls` each take it to 2, while `hash` and `hash -l` leave
// it where it is. So the count is of times the table saved a PATH walk, which
// is what a table exists to be judged on — a listing walks nothing.
func (r *Runner) hashCommandHit(name string) {
	e, ok := r.cmdHash[name]
	if !ok {
		return
	}
	e.hits++
	r.cmdHash[name] = e
}

// putHashedCommand writes one entry with the hit count given, adding the name
// to the listing order if it is new.
func (r *Runner) putHashedCommand(name, path string, hits int) {
	if r.cmdHash == nil {
		r.cmdHash = map[string]hashedCommand{}
	}
	if _, ok := r.cmdHash[name]; !ok {
		r.cmdHashOrder = append(r.cmdHashOrder, name)
	}
	r.cmdHash[name] = hashedCommand{path: path, hits: hits}
}

// hashedCommandPath is what the table holds for a name, if anything.
func (r *Runner) hashedCommandPath(name string) (string, bool) {
	e, ok := r.cmdHash[name]
	return e.path, ok
}

// forgetHashedCommand takes one name out, and says whether it was there.
func (r *Runner) forgetHashedCommand(name string) bool {
	if _, ok := r.cmdHash[name]; !ok {
		return false
	}
	delete(r.cmdHash, name)
	r.cmdHashOrder = slices.DeleteFunc(r.cmdHashOrder, func(n string) bool { return n == name })
	return true
}

// forgetEveryHashedCommand empties the table.
//
// `hash -r` is the spelling every shell has for it, and an assignment to PATH
// is the other way in — measured unanimous, see hashedCommand's comment. The
// slice is set to nil rather than truncated so a clone cannot share a backing
// array with the shell it came from.
func (r *Runner) forgetEveryHashedCommand() {
	r.cmdHash = nil
	r.cmdHashOrder = nil
}

// hashedCommandNames is the table in the order this dialect lists it.
//
// **Only zsh's order is reproducible.** bash, ksh93 and dash each print their
// table in its own bucket order, which is a fact about the hash function
// inside each of them rather than about the shell language: `awk`, `ls` and
// `sed`, hashed in that order, come out `ls awk sed` in bash 5.3.15 and `awk
// sed ls` in dash, and neither is insertion order or any ordering of the
// names or the paths. Reproducing one would mean reimplementing that shell's
// hashing, which CLEANROOM.md's red list covers and which would be a
// pointless thing to own besides. So the choice here is between an order we
// invented and one we can defend, and insertion order is the second: it is
// what a reader of a session would predict, and it is stable.
//
// zsh sorts, and that is reproducible and therefore asked for.
func (r *Runner) hashedCommandNames() []string {
	names := slices.Clone(r.cmdHashOrder)
	if len(names) < 2 {
		// Nothing an order could change, so nothing to ask: an empty or
		// one-entry listing is unanimous and must come out of a shell that
		// has chosen no dialect. See TestHashAsksNothingWhereThePanelAgrees.
		return names
	}
	if r.ask(r.sem().HashListingIsSorted, "`hash` listing its table in name order") {
		slices.Sort(names)
	}
	return names
}

// HashedCommandPath is what the shell's command hash holds for a name.
//
// Exported for the dialect that presents the table as a parameter — bash's
// `BASH_CMDS` — on the same footing as LookupAlias beside BASH_ALIASES.
func (r *Runner) HashedCommandPath(name string) (string, bool) {
	return r.hashedCommandPath(name)
}

// HashedCommandNames is every name the command hash holds, in this dialect's
// listing order.
func (r *Runner) HashedCommandNames() []string { return r.hashedCommandNames() }

// HashCommand puts an entry into the command hash, as `hash -p` does: the
// path is taken as given and the hit count starts at zero.
func (r *Runner) HashCommand(name, path string) { r.putHashedCommand(name, path, 0) }

// hashingIsOff reports whether the option behind `set +h` has been turned off
// *and* this dialect reads that as a stop rather than as a preference.
//
// The first half is what keeps this off the hot path of a shell nobody has
// told anything: a script that has not moved the option asks no axis, which
// matters here more than anywhere else — this runs in front of every external
// command, and an unanswered axis there would refuse every command a
// library embedder's Runner tried to start. See
// Semantics.HashObeysCommandTracking, and TestHashingIsAskedAboutOnlyWhenTheOptionMoved.
func (r *Runner) hashingIsOff() bool {
	if !r.tracksCommandsMoved || r.tracksCommands {
		return false
	}
	return r.ask(r.sem().HashObeysCommandTracking, "`set +h` stopping the command hash")
}
