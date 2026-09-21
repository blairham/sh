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
//	command ls >/dev/null; hash    `command` does too
//	(ls >/dev/null); hash          a subshell's entry is the subshell's
//	cd /; hash                     moving does not empty it
//	PATH=$PATH; hash               *assigning* PATH does
//	unset PATH; hash               so does unsetting it
//
// Two things they part on. What a **stale** entry means is
// Semantics.CommandHashIsTrusted, and whether a lookup that only *reports* a
// path also remembers it is Semantics.ALookupRemembersThePath — which this
// comment claimed was unanimous on the strength of a bash-only probe, and is
// not: `type ls >/dev/null; hash` leaves bash's table empty and fills the
// other three.
//
// A third is what a `PATH=… cmd` prefix leaves behind, which is a 4-3 split
// and is Semantics.APrefixedPathEmptiesTheCommandHash. It could not be asked
// until #2626, because a prefix did not reach this shell's own PATH for an
// external command at all and the branch was one no run could take. See
// interp/prefixedpath.go.
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

// retrackCommand points an entry at what a search has just found instead.
//
// The dialect whose table does not shadow an earlier directory keeps the
// table current rather than stale: measured 2026-09-15 on ksh93u+, a `hash`
// after a copy appears in front of the remembered one names the **new** path,
// and a `hash` after that copy is removed names the old one again. So the
// entry follows the search rather than being abandoned by it.
//
// The hit count is carried over rather than reset, and no hit is counted for
// this: the table answered nothing here — a walk found the program — and the
// count is of walks the table saved. See hashCommandHit.
func (r *Runner) retrackCommand(name, path string) {
	e, ok := r.cmdHash[name]
	if !ok || e.path == path {
		return
	}
	r.cmdHash[name] = hashedCommand{path: path, hits: e.hits}
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

// ForgetHashedCommand takes one name out of the command hash and says whether
// it was there.
//
// The other half of HashCommand, for the dialect that presents the table as a
// parameter a script can *remove* an entry from. It is the removing half of
// one shell's `$commands` and it is deliberately not on the other's
// `BASH_CMDS`: measured 2026-09-12, `hash -p /bin/ls q; unset "BASH_CMDS[q]"`
// leaves the entry alone in bash 5.3.15 and `unset "commands[ls]"` really
// takes it out in zsh 5.9.2. So the two dialects part here, and each says so
// in its own writer rather than in a shared rule.
func (r *Runner) ForgetHashedCommand(name string) bool { return r.forgetHashedCommand(name) }

// trackingIsOff reports whether a script has turned command tracking off.
//
// **Moved**, and not merely off: a shell nobody has told anything asks no axis
// at all, which matters here more than anywhere else — the two callers below
// run in front of every external command and in front of every `hash`, and an
// unanswered axis there would refuse every command a library embedder's
// Runner tried to start. See TestHashingIsAskedAboutOnlyWhenTheOptionMoved.
func (r *Runner) trackingIsOff() bool {
	return r.tracksCommandsMoved && !r.tracksCommands
}

// hashingIsOff reports whether tracking is off *and* this dialect reads that
// as a stop rather than as a preference.
//
// This is the automatic half — what a command that runs puts in the table.
// bash and zsh stop; ksh93 goes on hashing with `trackall` off. See
// Semantics.HashObeysCommandTracking.
func (r *Runner) hashingIsOff() bool {
	if !r.trackingIsOff() {
		return false
	}
	return r.ask(r.sem().HashObeysCommandTracking, "command tracking turned off stopping the command hash")
}

// hashBuiltinIsRefused reports whether the builtin itself declines while
// tracking is off, which is a second and narrower question.
//
// bash alone. Measured 2026-09-13: with the option off, bash answers every
// spelling — a bare listing, `hash -r`, `hash name` — with one sentence at 1,
// where zsh goes on answering all three at 0 and an explicit `hash ls` still
// puts `ls` in the table there. So the option stops the *automatic* hashing
// in both and only bash reads it as closing the builtin.
func (r *Runner) hashBuiltinIsRefused() bool {
	if !r.trackingIsOff() {
		return false
	}
	return r.ask(r.sem().HashRefusesWhileTrackingIsOff, "`hash` refusing while command tracking is off")
}

// lookPathReporting is lookPath for a builtin that was asked *where* a command
// is rather than to run it — `type`, `command -v`, `command -V`, `type -p`.
//
// Three of the four hash what they were only asked about; bash does not. See
// Semantics.ALookupRemembersThePath, which carries the measurement and the
// probe that got it wrong first.
//
// **The answer is read and not asked**, which is the rare shape and wants its
// reason. r.ask refuses the command outright where no dialect has answered,
// and that is right where the disagreement is something the command itself
// shows — but here it is not: `type ls` prints the same sentence either way,
// and the only difference is whether a later `hash` finds the name. Refusing
// a command whose own answer is unanimous, to settle a side effect nobody in
// that command can see, would be an over-refusal. Runner.commandTracking
// reads its axis directly for the same kind of reason.
//
// It is in a helper rather than inside lookPath because lookPath is the
// *execution* path too, where the answer is not this one: running a command
// hashes it in all four, so hashCommandRun stays there unconditionally. What it
// returns is still the path on disk: how a *pathname operand* is written back
// is Runner.reportedPath's question, and it is asked where the answer is
// printed rather than here, because `type -t ./x` writes `file` in every
// column and must not be refused for an axis it cannot show.
func (r *Runner) lookPathReporting(name string) (string, error) {
	// **The table is the answer here even when what it holds has gone**, in
	// the dialect that trusts it. Measured 2026-09-21 on bash 5.3.20:
	// `hash -p /nosuchfile cat; type cat` is `cat is hashed (/nosuchfile)`
	// at 0 and `command -v cat` is `/nosuchfile`, while *running* `cat` is
	// `/nosuchfile: No such file or directory` at 127 — so the report and
	// the run part company, and before this the report answered `not found`
	// about a name that had a perfectly good `/bin/cat` behind it (#4064).
	//
	// `shopt -s checkhash` does not move it: measured with the option on,
	// `type` still writes the remembered path. That is why this does not
	// read Runner.checksHashedCommand the way the execution path does — the
	// option is about the shell looking before it *runs*.
	//
	// Semantics.CommandHashIsTrusted is the axis because it is the same
	// disagreement: measured the same day, zsh hashes `zzc`, the file is
	// removed, and `whence -v zzc` is `zzc not found` at 1 where bash's
	// `type` names the path it remembered.
	//
	// Asked only once the remembered path has actually gone, which is the
	// one arrangement that tells the readings apart — an entry that still
	// runs is answered by the search below with nothing extra consulted, so
	// this adds no question to a report that was unanimous.
	if hashed, ok := r.hashedCommandPath(name); ok && r.rememberingLookups() {
		if r.runnable(r.absolute(hashed)) != nil &&
			r.ask(r.sem().CommandHashIsTrusted, "a hashed path reported without looking for it again") {
			// The entry **as it was written**, not absolutised: measured,
			// `hash -p relfile cat; type cat` is `cat is hashed (relfile)`
			// and `command -v cat` is `relfile`. The run of the same name
			// resolves it against the shell's directory and says so, which
			// is lookPath's job and not this one's.
			//
			// And it counts, the way a report of an entry that still runs
			// already did: measured, two reports of a `hash -p` entry leave
			// the count at 2 whether or not the file is there.
			r.hashCommandHit(name)
			return hashed, nil
		}
	}
	path, err := r.lookPath(name)
	if err != nil {
		return path, err
	}
	if r.rememberingLookups() && r.sem().ALookupRemembersThePath == Yes {
		r.hashCommandRun(name, path)
	}
	return path, err
}
