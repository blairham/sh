// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A pattern that matched nothing, in the words of a command that runs as a
// program, is that program's failure rather than the script's, in the one
// dialect that refuses an unmatched pattern by default. See
// Semantics.UnmatchedPatternFailsOnlyTheProgram for the measurement.
//
// Not only a miss: a pattern that will not compile and a qualifier list that
// will not read are the program's failure in the same place, so the three
// places those are written — refuseUnmatchedPattern, fatalPattern and fatal —
// each ask holdForTheProgram first, and nothing else decides it (#6576).
//
// The refusal is held rather than written while the command's words are
// expanded, and written once they all have been: every other expansion in the
// line runs first and a failure among them wins, the redirections are never
// opened, and the command does not run. Held for the first pattern only,
// which is the one the reference names.

// programGlob is where a command's words are, for the refusal that may be
// held: words is set only while the pathname stage of a word after the command's name
// runs, so nothing that runs a command — a substitution in a later word — is
// ever reached while it is.
type programGlob struct {
	// words is the command line as far as it has been built, the kept
	// modifiers first; modifiers is how many of them there are.
	words     []string
	modifiers int
	// viaCommand says the scan stopped at the modifier that names a program
	// rather than a function.
	viaCommand bool
	// refused is the sentence held for the first refusal, empty while there
	// is none.
	refused string

	// listOpen says a word list is being expanded — a command's words, an
	// array literal's elements, a `for` loop's list — and globbing that the
	// pathname stage of one of its words is running. While both hold, a
	// refusal that is not a program's is held too, in held and raise, and
	// raised once every word has been expanded. See holdPathnameRefusal.
	listOpen, globbing bool
	held               string
	raise              func()
}

// armProgramGlob says the pathname stage about to run is for a word behind
// the command's name, and answers with what disarms it.
func (r *Runner) armProgramGlob(words []string, modifiers int, viaCommand bool) {
	r.progGlob.words, r.progGlob.modifiers, r.progGlob.viaCommand = words, modifiers, viaCommand
}

func (r *Runner) disarmProgramGlob() { r.progGlob.words = nil }

// holdForTheProgram reports whether a refusal worded msg is the program's
// and has been held for it, rather than written and acted on now.
//
// The axis is asked here and nowhere else: only a pattern that missed, in a
// word behind the name of a command that runs as a program, is where the
// dialects disagree, so every other word and every matching pattern reaches no
// question.
func (r *Runner) holdForTheProgram(msg string) bool {
	if !r.runsAsAProgram() {
		return false
	}
	if !r.ask(r.sem().UnmatchedPatternFailsOnlyTheProgram,
		"an unmatched pattern in a program's words failing only the program") {
		return false
	}
	if r.progGlob.refused == "" {
		r.progGlob.refused = msg
	}
	return true
}

// runsAsAProgram reports whether the command whose words are armed is started
// as a program: neither a function nor a builtin, and not behind a modifier
// that runs one of those or replaces the shell.
func (r *Runner) runsAsAProgram() bool {
	g := r.progGlob
	if len(g.words) <= g.modifiers {
		// Disarmed, or nothing behind the modifiers to be a name.
		return false
	}
	for _, m := range g.words[:g.modifiers] {
		// `builtin` runs a builtin and `exec` starts nothing in a child —
		// both measured fatal. Only the word that looks for a program is
		// left standing.
		if kind, ok := r.precommand(m); !ok || kind != PrecommandStopsTheScan {
			return false
		}
	}
	name := g.words[g.modifiers]
	if g.viaCommand {
		return !r.presentsAsBuiltin(name) || r.sem().CommandReachesABuiltin != Yes
	}
	if r.HasFunction(name) {
		return false
	}
	if _, ok := r.undefinedFunction(name); ok {
		return false
	}
	return !r.presentsAsBuiltin(name)
}

// holdPathnameRefusal holds a refusal the pathname stage raised in a word of a
// list, and reports whether it did: as the program's failure, where the
// command runs as one (see holdForTheProgram), and otherwise until every word
// of the list has been expanded, when raiseHeldRefusal performs raise.
//
// Every word is expanded before any refusal is raised, and only the first
// refusal is named. Measured 2026-10-15 on bash 5.3.20 under `shopt -s
// failglob` and on zsh 5.9.2, -c and from a file, in a directory where nothing
// matches (#6677):
//
//	echo $(echo S >&2) nomatch*zz $(echo A >&2)   S, A, then the refusal
//	nomatch*zz $(echo A >&2)                      A, then the refusal
//	f nomatch*zz / builtin echo / local / export  the same
//	a=(5 $(echo S >&2) nomatch*zz $(echo A >&2))  the same, and so a `for` list
//	                                              and `declare -a a=(…)`
//	echo nomatch*zz other*qq $(echo A >&2)        A, then nomatch*zz alone
//	echo nomatch*zz $((1/0)) $(echo A >&2)        the division alone: an
//	                                              expansion's failure stops the
//	                                              list where it stands, and wins
//	echo nomatch*zz $(…) >$(echo R >&2)f          no R: the redirections are
//	                                              never expanded, nor is a
//	x=$(echo X >&2) echo nomatch*zz               prefix assignment's value
//
// zsh's refusals of a pattern that will not compile or a qualifier that will
// not read are held the same way: `echo *(Z) $(echo A >&2)` writes A first.
func (r *Runner) holdPathnameRefusal(msg string, raise func()) bool {
	if r.holdForTheProgram(msg) {
		return true
	}
	g := &r.progGlob
	if !g.listOpen || !g.globbing {
		return false
	}
	if g.raise == nil && g.refused == "" {
		g.held, g.raise = msg, raise
	}
	return true
}

// openWordList gives a word list a hold of its own, and answers with what
// puts the outer one back. A substitution in one of its words runs commands
// whose lists are their own.
func (r *Runner) openWordList() func() {
	outer := r.progGlob
	r.progGlob = programGlob{listOpen: true}
	return func() { r.progGlob = outer }
}

// raiseHeldRefusal raises the refusal held while the list's words were
// expanded, unless a failure since has already ended the list, and closes the
// hold: what is expanded after the words — a redirection's target — raises
// its own.
func (r *Runner) raiseHeldRefusal() {
	g := &r.progGlob
	raise := g.raise
	g.listOpen, g.held, g.raise = false, "", nil
	if raise == nil || r.expansionHasFailed() {
		return
	}
	raise()
}

// globbingAListWord marks the pathname stage of a list's word as running,
// and answers whether it did, so that the caller clears only its own mark.
func (r *Runner) globbingAListWord() bool {
	if !r.progGlob.listOpen || r.progGlob.globbing {
		return false
	}
	r.progGlob.globbing = true
	return true
}

func (r *Runner) doneGlobbingAListWord() { r.progGlob.globbing = false }

// takeProgramGlobRefusal answers the sentence held for this command, if any,
// and forgets it.
func (r *Runner) takeProgramGlobRefusal() string {
	msg := r.progGlob.refused
	r.progGlob.refused = ""
	return msg
}

// enterProgramGlob gives a command a hold of its own and answers with what
// puts the outer command's back: a substitution in a later word runs commands
// of its own between a held refusal and the line that writes it.
func (r *Runner) enterProgramGlob() func() {
	outer := r.progGlob
	r.progGlob = programGlob{}
	return func() { r.progGlob = outer }
}
