// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The cached home: one column's answer to a bare `~`.
//
// Six of the seven columns replace a written `~` with the value `HOME` holds
// at the moment the word is expanded, which is what the standard describes and
// what this package does. The seventh keeps a copy, and answers the copy. See
// Semantics.TildeReadsACachedHome for the rows, and
// interp/tildecurrenthome_test.go for what the other six are pinned to.
//
// Two things about the copy decide how it is modeled here, and both were
// measured rather than assumed — #3484 and #4039 were each filed on the
// reading "the home the shell started with", and that reading is wrong:
//
//   - It is refreshed by **building an environment for a child**. An external
//     command, a pipeline, a background job and a command substitution all
//     move it; a builtin, a function, `eval`, `.`, `export` and a subshell do
//     not. So the refresh is hung on Runner.environ, which is the one function
//     in this package whose whole job is to build that environment, and not on
//     "a process was started" — a subshell starts one and does not move it.
//   - It is the **exported** environment that is read. A name the shell has
//     taken the export attribute off is not in what a child is handed, and the
//     copy goes with it; a command's own assignment prefix is appended after
//     the environment is built and is not what the copy is taken from. Both
//     fall out of reading the value back from the slice environ returns rather
//     than from Runner.Vars.
//
// A clone takes the copy with it and moves its own from there, which is what
// makes `( /usr/bin/true; echo ~ )` the new home inside the parentheses and
// the old one after them.

// ensureCachedHome seeds the copy from the environment the shell started with.
//
// Called at the head of every chunk, and it writes once: the value a script
// assigns must not become the copy, so a lazy seed taken at the first `~`
// would read `/h` for `HOME=/h; echo ~` and answer the question this axis
// exists to separate. Seeded whichever way the axis is answered, because the
// axis is a dialect's and a dialect can be swapped onto a runner that has
// already read a line.
func (r *Runner) ensureCachedHome() {
	if r.cachedHomeSeeded {
		return
	}
	r.cachedHomeSeeded = true
	r.cachedHome, r.cachedHomeSet = r.getVar("HOME")
}

// noteChildEnvironment refreshes the copy from an environment just built for a
// child.
//
// env is what the child will be handed, and the value is read back out of it
// rather than off Runner.Vars for the reason above: `export -n HOME` leaves
// the name out of the slice, and the copy is then absent too.
func (r *Runner) noteChildEnvironment(env []string) {
	r.cachedHomeSeeded = true
	r.cachedHome, r.cachedHomeSet = "", false
	// Backwards, because a slice built by environ can carry a name twice and
	// it is the **last** entry that a child resolves to — the same rule
	// execve applies to the block it is given.
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := afterEquals(env[i], "HOME"); ok {
			r.cachedHome, r.cachedHomeSet = v, true
			return
		}
	}
}

// afterEquals reads an environment entry's value if it is the named one.
func afterEquals(entry, name string) (string, bool) {
	if len(entry) <= len(name) || entry[len(name)] != '=' || entry[:len(name)] != name {
		return "", false
	}
	return entry[len(name)+1:], true
}

// tildeHome is the home a written `~` comes to, and the one place the axis is
// asked.
//
// The copy is never *consulted* by a dialect that answers No, so a shell on
// the standard's reading pays nothing for the field being there and cannot
// drift onto it by accident.
func (r *Runner) tildeHome() (string, bool) {
	if !r.ask(r.sem().TildeReadsACachedHome, "a `~` answered from a cached home") {
		return r.getVar("HOME")
	}
	r.ensureCachedHome()
	return r.cachedHome, r.cachedHomeSet
}
