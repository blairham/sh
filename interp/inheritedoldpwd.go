// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// InheritedOldpwdPolicy is what a shell does with an `OLDPWD` it was handed in
// its environment.
//
// It looks like a question about `cd -` and is not. `cd -` reads OLDPWD in
// every shell measured and reports the same way about a value it cannot use,
// so the branch inside `cd` is unanimous; what splits the panel is whether the
// value is still there to read by the time a script runs. Measured 2026-09-12
// in a directory that exists, with `${OLDPWD-UNSET}` as the whole probe:
//
//	                       OLDPWD=/nonexistent   OLDPWD=/usr   no OLDPWD
//	dash, ksh93, ash       /nonexistent          /usr          UNSET
//	bash 5.3, bash-as-sh   UNSET                 /usr          UNSET
//	bash 3.2               UNSET                 UNSET         UNSET
//	zsh                    $PWD                  $PWD          $PWD
//
// That is what #1490 saw from the far end and read as a branch: `OLDPWD=/nope
// bash -c 'cd -'` says `cd: OLDPWD not set` — the sentence for a genuinely
// unset OLDPWD — because by then it genuinely is unset. Set the same value
// *inside* the shell and bash names the path like everyone else:
// `cd /usr; OLDPWD=/nope; cd -` is `cd: /nope: No such file or directory` in
// bash 5.3, bash-as-`sh` and bash 3.2 alike. A fix inside `cd` would have made
// the two spellings disagree in bash and matched neither.
//
// zsh's silence is the same misreading twice over. It reports nothing because
// there is nothing to report: an inherited value never arrives, the name
// starts life holding the directory the shell started in, and `cd -` there
// goes where it already is.
//
// bash 3.2 is a fourth answer — no inherited OLDPWD at all, usable or not —
// and has no constant here because no dialect in this tree targets it. It is a
// column in the golden record and a dated answer rather than a disputed one.
type InheritedOldpwdPolicy int

const (
	// InheritedOldpwdUnspecified is no answer, and is refused like any other.
	InheritedOldpwdUnspecified InheritedOldpwdPolicy = iota
	// InheritedOldpwdTaken reads the environment's value as it stands, and
	// leaves whether it can be used to whatever tries. dash, ksh93 and ash.
	InheritedOldpwdTaken
	// InheritedOldpwdTakenIfADirectory keeps the environment's value only
	// when it names a directory, and otherwise starts with no OLDPWD at all.
	// bash 5.3.
	//
	// A directory rather than an enterable one: measured, a mode-000
	// directory is kept and a plain file is dropped, so this is a stat and
	// not the question `cd` itself asks. A relative value is resolved
	// against the directory the shell started in and kept if that names a
	// directory — `OLDPWD=olp` survives in a shell started in the parent of
	// `olp`, and `OLDPWD=relative` with nothing of that name does not.
	InheritedOldpwdTakenIfADirectory
	// InheritedOldpwdIgnored does not read the environment's value at all:
	// OLDPWD is the shell's own record of where it has been, and it starts
	// at the directory the shell started in. zsh, which also exports the
	// name from the first command — `env | grep OLDPWD` in a zsh started
	// without one answers `OLDPWD=$PWD`, where dash, bash and ksh93 answer
	// nothing.
	InheritedOldpwdIgnored
)

func (p InheritedOldpwdPolicy) String() string {
	switch p {
	case InheritedOldpwdTaken:
		return "InheritedOldpwdTaken"
	case InheritedOldpwdTakenIfADirectory:
		return "InheritedOldpwdTakenIfADirectory"
	case InheritedOldpwdIgnored:
		return "InheritedOldpwdIgnored"
	}
	return "InheritedOldpwdUnspecified"
}

// settleInheritedOldpwd applies the policy above, once, before the first
// command runs.
//
// Runs at most once per session, which is what keeps it off a `cd` that has
// already written an OLDPWD of the shell's own — a dialect's prelude runs
// before the script, and a shell reading a person's input runs a chunk a line.
// A dialect with no answer is left alone rather than refused: the
// name is simply read from the environment as any other name is, which is what
// every route through here did before the axis existed.
func (r *Runner) settleInheritedOldpwd() {
	if r.oldpwdSettled {
		return
	}
	r.oldpwdSettled = true
	if _, ok := r.Vars["OLDPWD"]; ok || r.removed["OLDPWD"] {
		// A caller set one, or a `cd` already has. Either is a value of this
		// shell's own and the environment has nothing to say about it.
		return
	}
	switch r.sem().InheritedOldpwd {
	case InheritedOldpwdIgnored:
		r.setVarQuietly("OLDPWD", r.workDir())
		if r.exported == nil {
			r.exported = map[string]bool{}
		}
		r.exported["OLDPWD"] = true
	case InheritedOldpwdTakenIfADirectory:
		v, ok := r.inheritedValue("OLDPWD")
		if !ok {
			return
		}
		// Through the gate like every other stat, so a policy sees the one
		// probe the shell makes on the caller's behalf at startup.
		if fi, err := r.stat(r.absolute(v)); err == nil && fi.IsDir() {
			return
		}
		r.unsetOneName("OLDPWD")
	case InheritedOldpwdTaken, InheritedOldpwdUnspecified:
	}
}
