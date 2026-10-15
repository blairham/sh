// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A pattern that matched nothing, in the words of a command that runs as a
// program, is that program's failure rather than the script's, in the one
// dialect that refuses an unmatched pattern by default. See
// Semantics.UnmatchedPatternFailsOnlyTheProgram for the measurement.
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
		return !(r.presentsAsBuiltin(name) && r.sem().CommandReachesABuiltin == Yes)
	}
	if r.HasFunction(name) {
		return false
	}
	if _, ok := r.undefinedFunction(name); ok {
		return false
	}
	return !r.presentsAsBuiltin(name)
}

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
