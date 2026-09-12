// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A qualifier list may end in the same history-style modifiers `${x:t}` takes,
// and they apply to every name the pattern reported.
//
// Measured on zsh 5.9.2, 2026-09-10, against `gq4/x/b.txt` and `gq4/x/c.md`:
//
//	gq4/*/*(N:t)        b.txt c.md      the tail of each name
//	gq4/*/*(N:t:r)      b c             a chain, applied left to right
//	gq4/*/*(N:e)        md txt          and the answer is *re-sorted*
//	gq4/*/*(N:h1)       gq4 gq4         a count reaches the letter here too
//	gq4/*/*(N:gs/b/Q/)  …/Q.txt …/c.md  and so does a substitution
//
// **The list is applied to the words as reported**, not to the paths the walk
// held: `gq/*(N:h)` answers `gq` once per name, which is the head of `gq/a`
// and not of the absolute path underneath it.
//
// **Re-sorting is the measurement's own finding.** `gq4/*/*(N:e)` answers
// `md txt` where the names it came from are in the order `b.txt c.md`, so the
// order is taken after the modifiers rather than carried through them. It
// falls out of applying them where the sort already happens.
//
// **An unrecognized modifier stops the chain and says nothing**, which is not
// what the parameter surface does with the same text — `${x:z}` is refused by
// name there. Measured: `*(N:z)` and `*(N:X)` list the names unchanged,
// `*(N:zt)` does *not* apply the `t` behind the `z`, and `*(N:t:X:u)` applies
// the `t` and not the `u`. So the run stops at the first segment it cannot
// read, and what was applied before it stands.
//
// **Text after a letter inside one segment is ignored** rather than being the
// failure it is in `${x:ha}`: `*(N:tr)` is the tail alone and `*(N:t:r)` is
// the pair, so the second letter is only a modifier when it has a `:` of its
// own.
//
// A substitution is the exception to the silence, and it is the shell's:
// `*(N:s)` is `bad substitution` and `*(N:s//Q/)` is `no previous
// substitution`, the same two sentences the parameter surface gives, and both
// abandon the script.

// applyGlobModifiers runs a qualifier list's modifier text over one name.
//
// ok is false where a substitution failed, having already reported itself —
// the only way this can fail, every other unreadable segment being a stop
// rather than an error.
func (r *Runner) applyGlobModifiers(value, text string) (string, bool) {
	for {
		seg, rest, more := scanOneModifier(text)
		next, applied, ok := r.applyGlobModifier(value, seg)
		if !ok {
			return "", false
		}
		if !applied {
			// A segment that is not a modifier ends the run, and the value
			// as it stands is the answer.
			return value, true
		}
		value = next
		if !more {
			return value, true
		}
		text = rest
	}
}

// applyGlobModifier performs one segment. applied is false where the segment
// names no modifier, which stops the chain without a word.
func (r *Runner) applyGlobModifier(value, seg string) (result string, applied, ok bool) {
	global := false
	if len(seg) > 1 && seg[0] == 'g' {
		global, seg = true, seg[1:]
	}
	if seg == "" {
		return value, false, true
	}
	arg, known := modifierLetters[seg[0]]
	if !known {
		return value, false, true
	}
	letter, rest := seg[0], seg[1:]
	switch arg {
	case modifierNothing:
		if letter == '&' {
			out, ok := r.repeatSubstitution(value, global, nil)
			return out, true, ok
		}
		// The trailing text a parameter's modifier would be refused for is
		// simply not read here; see the measurements above.
		out, ok := r.applyModifier(value, letter, nil)
		return out, true, ok
	case modifierCount:
		n, ok := modifierCountOf(rest)
		if !ok {
			// `:tX` is the tail: what follows the letter is not a count and
			// is not an error either, so the letter stands on its own.
			n = 0
		}
		if n == 0 {
			return r.applyPureModifier(value, letter), true, true
		}
		if letter == 'h' {
			return modifierHeadCount(value, n), true, true
		}
		return modifierTailCount(value, n), true, true
	default:
		// A substitution reports its own failures, and it needs a node to
		// name the subject of one. There is no parameter here and the
		// subject is empty in the shell too — `*(N:s)` is `bad substitution`
		// with nothing in front of it — so an empty node is the honest
		// answer rather than a missing one.
		out, ok := r.substituteModifier(value, rest, global, &syntax.ParamExpr{})
		return out, true, ok
	}
}
