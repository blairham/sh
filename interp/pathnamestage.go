// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "slices"

// The pathname stage of a word list runs once every word of the list has had
// its other expansions, word by word, and stops at the first word it refuses.
//
// A command's words, a `for` or `select` list, an array literal and a
// declaration's literal operand are each one list. Code in a glob qualifier is
// the one place the stage runs commands, so it is the one place the order can
// be seen. Measured 2026-10-27 on zsh 5.9.2 under -fc and from a file, in a
// directory holding one file `q`, both streams on one pipe (#6768):
//
//	echo q*(e:'print -u2 Q':) $(echo A >&2)          A, Q, q
//	echo q*(e:'…Q1':) $(echo A >&2) q*(e:'…Q2':) $(echo B >&2)
//	                                                 A, B, Q1, Q2
//	q*(e:'print -u2 Q':) $(echo A >&2)               A, Q: the name is a word
//	for w in …, a=( … ), typeset -a a=( … ), select  the same
//	echo q*(e:'X=set':) $X                           q: the later word was
//	                                                 expanded before the code
//	echo q*(e:'print -u2 Q':) > $(echo A >&2; …)     Q, A: the redirections
//	                                                 come after the words
//	echo zz* q*(e:'print -u2 Q':)                    the refusal alone: no Q
//	/bin/echo zz* q*(e:…)                            the same, at 1 — a
//	                                                 program's refusal stops
//	                                                 the stage as the shell's
//	echo *(Z) q*(e:…)                                the same, for a qualifier
//	                                                 that will not read
//	echo q*(e:'…Q1; false':) q*(e:'…Q2':)            Q1 alone: the first
//	                                                 word's own miss stops it
//
// So the stage is a pass of its own over the finished list, and a word is
// matched in its place in that pass. The words were matched one at a time as
// each was expanded, which put a qualifier's code ahead of the substitutions
// in the words after it, and ran a later word's code after an earlier word's
// refusal had already been held (#6677 held the refusal; this is the half it
// could not reach).
//
// No other shell can see the order: bash and ksh93 run nothing in the stage,
// so for them the pass is the same list in the same order with the same
// refusals, and nothing is asked.

// heldGlob is a run of a list's fields that are expanded and not yet matched:
// where they stand in the list, and what the stage is told about them.
type heldGlob struct {
	from, to int
	// noglob says a modifier switched the match off for these fields;
	// they are still unescaped by the stage, as every field is.
	noglob bool
	// armed says the fields stand behind the command's name, where a
	// refusal may be the program's. See Runner.armProgramGlob.
	armed bool
	// matched is how many fields the run came to, written by the stage.
	matched int
}

// holdGlob adds a run of fields to the list and records that the stage has
// them still to match.
func holdGlob(list []string, held []heldGlob, fields []string, noglob, armed bool) ([]string, []heldGlob) {
	from := len(list)
	list = append(list, fields...)
	return list, append(held, heldGlob{from: from, to: len(list), noglob: noglob, armed: armed})
}

// pathnameStage matches each held run of the list in order, and answers with
// the list as the matches left it. arm, where it is not nil, is told the list
// so far before an armed run is matched and put back after it; it is the
// command's, for the refusal that may be the program's.
//
// The stage stops at the first run whose match refused, or where an earlier
// failure has already ended the list: the runs after it are left as text,
// with their quoting marks taken off, and are never matched. held is written
// with what each run came to, for shiftedBy.
func (r *Runner) pathnameStage(list []string, held []heldGlob, arm func([]string)) []string {
	delta := 0
	for i := range held {
		g := &held[i]
		from, to := g.from+delta, g.to+delta
		stopped := r.pathnameStageStopped()
		if g.armed && arm != nil && !stopped {
			arm(list)
		}
		out := r.globFieldsUnlessSuppressed(list[from:to], g.noglob || stopped)
		if g.armed && arm != nil && !stopped {
			r.disarmProgramGlob()
		}
		g.matched = len(out)
		if len(out) == to-from && (len(out) == 0 || &out[0] == &list[from]) {
			// globFields handed the run back as it was.
			continue
		}
		list = slices.Replace(list, from, to, out...)
		delta += len(out) - (to - from)
	}
	return list
}

// pathnameStageStopped reports whether the stage has nothing more to match:
// an earlier run refused, or a failure has already ended the list. The runs
// after that point are left as text.
func (r *Runner) pathnameStageStopped() bool {
	return r.expandErr || r.unspecified || r.givingUpAlready() ||
		r.progGlob.raise != nil || r.progGlob.refused != ""
}

// shiftedBy is where a position in the list as it was built stands once the
// stage has matched it: a run that came to more or fewer fields moves every
// position behind it. A position inside a run is the run's own start.
func shiftedBy(held []heldGlob, at int) int {
	delta := 0
	for _, g := range held {
		if g.to <= at {
			delta += g.matched - (g.to - g.from)
			continue
		}
		if g.from <= at {
			return g.from + delta
		}
		break
	}
	return at + delta
}

// cameToOneField reports whether the run starting at this position in the
// list as it was built came to exactly one field.
func cameToOneField(held []heldGlob, from int) bool {
	for _, g := range held {
		if g.from == from {
			return g.matched == 1
		}
	}
	return false
}

// heldAt reports whether a run of held fields starts at this position in the
// list as it was built. A position no run starts at was put on the list
// already matched — a declaration's operand, a modifier — and stands as it is.
func heldAt(held []heldGlob, from int) bool {
	for _, g := range held {
		if g.from == from {
			return true
		}
	}
	return false
}
