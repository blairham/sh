// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// tildeFoldGroupLetters is the body a `~(…)` group may hold and still be read
// where it stands rather than only at the front of the pattern: the sense
// signs, and the one letter whose whole meaning is an option for the rest of
// the branch.
//
// `i` is that letter. The others are not left out for tidiness — each of them
// is a *flavor*, or a question about the whole word, and neither can be
// honored from the middle of a glob by turning a flag on:
//
//   - `E X P G V F L` choose a regular-expression flavor, which settles the
//     language of the whole pattern rather than setting a flag on a branch —
//     interp/tildeflavorhere.go reads those, before any of this runs;
//   - `K p s` choose ksh's own glob, which is the language the walk is
//     already in, so they are consumed here and change nothing. That is what
//     `[[ zab == z~(K)a* ]]` measures: it matches in ksh93u+, so the group
//     is read and the `a*` behind it is an ordinary glob;
//   - `N` deletes the *word* when the pattern names nothing, which is a
//     question the matcher is never asked — see Runner.tildeGlobPattern, and
//     measured: `f z~(N)qq*` is the word in ksh93u+ as it is here;
//   - `g l r` are about which span of the subject the match takes. This used
//     to say "a branch cannot change that halfway along", which is right
//     about the mechanism and **wrong about the shell** — `v=aXbXc;
//     ${v#a~(g)*X}` is `c` in ksh93u+ and `[[ zab == z~(l)ab ]]` matches
//     there, so it does change it halfway along. Each wants plumbing of its
//     own rather than a flag on a branch: the greed reaches a caller outside
//     the matcher and the two anchors reach a comparison the walk does not
//     make. #4893 has the rows.
//
// So a group holding any of them is left exactly as it was: four or more
// ordinary characters, which is what this shell did with every mid-pattern
// group before. ksh93u+ reads a mid-pattern flavor and this does not, which
// is #4883 — and **consuming the group without honoring it is not the fix**,
// because `.` is an ordinary character in a glob and any one character in an
// expression. Measured, and recorded in splitTildeFoldGroup.
const tildeFoldGroupLetters = "+-iKps"

// splitTildeFoldGroup peels a `~(…)` group off the front of p when the group
// is one this position can honor, reporting what it asks for.
//
// fold is whether the letter turns case-folding on or off — `~(-i)` is a real
// answer and not the absence of one, which is why it is reported beside ok
// rather than folded into it. A group with no `i` in it at all asks for
// nothing and is stepped over.
//
// Measured 2026-09-27 against /bin/ksh `Version AJM 93u+ 2012-08-01`, each
// case in a directory of its own holding `za` and `zb` — **created by the
// shell under test**, and `ls` inside the case confirms them. There is no
// `zA`: this filesystem folds case in a *name*, so a directory holding both
// spellings holds one file and the rows would be comparing a pattern against
// a name that is not there.
//
//	f zA          (control)  1 | [zA]    no fold, so the word stands
//	f ~(i)zA      (control)  1 | [za]    a group at the front folds
//	f z~(i)A                 1 | [za]    and one in the middle folds too
//	f z~(i)AB                1 | [zab]   with `zab` and `zqb` present
//	f @(z~(i)A)              1 | [za]    inside a group
//	f *~(i)A                 1 | [za]    behind a star
//	f z~(i)A~(i)B            1 | [zab]   twice over
//	f ~(i)z~(-i)A            1 | [~(i)z~(-i)A]
//	                                     and `-i` turns it off again, so the
//	                                     `A` has to match itself and nothing
//	                                     does
//
// The two controls are why the rest is readable: the first says the probe can
// see a pattern *not* fold and the second that it can see one fold, and the
// rows between them each have `za` on disk for the folding reading to reach.
// The last row is the one that says the sense sign is read: a group applied
// to the whole pattern rather than from where it stands would fold the `A`
// and match, which ksh93u+ does not.
func splitTildeFoldGroup(p string) (fold, on, ok bool) {
	body, _, split := splitTildeModifier(p)
	if !split {
		return false, false, false
	}
	for i := 0; i < len(body); i++ {
		if strings.IndexByte(tildeFoldGroupLetters, body[i]) < 0 {
			return false, false, false
		}
	}
	sense := true
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '+':
			sense = true
		case '-':
			sense = false
		case 'i':
			fold, on = true, sense
		}
	}
	return fold, on, true
}

// tildeFoldHere applies a mid-pattern `~(…)` group to the options carried
// down the branch, and is the tilde half of what applyPatternFlags does for
// the other dialect's `(#…)` groups.
//
// foldClass moves with fold, which is the difference between this fold and
// the run-time option's: measured 2026-09-14 on ksh93u+, `~(i)[[:lower:]]`
// matches `A` and `~(i)[a-z]` matches it too, so the group reaches a bracket
// and a class where `nocasematch` reaches only one of them. See matchTilde,
// which does the same thing for a group at the front.
func tildeFoldHere(o patternOpts, fold, on bool) patternOpts {
	if !fold {
		return o
	}
	o.fold, o.foldClass = on, on
	return o
}

// holdsTildeFoldGroup reports whether a field carries a `~(…)` fold group
// somewhere other than its front, which is what makes it describe a name
// rather than spell one out.
//
// The gate at the top of pathname expansion asks
// Runner.describesRatherThanSpells, and until this was there a field like
// `z~(i)A` was a spelled-out name: looked up as the characters it was written
// with, found missing, and passed back through as text. tildeGlobPattern
// answers the same question for a group at the **front**, and this is the
// same argument one character along.
//
// **A group that asks for nothing still counts here, and at the front it does
// not.** That is measured and not an oversight, with `za` and `zb` in the
// directory: `f ~()zb` is the single field `~()zb` in ksh93u+ — the group is
// at the front, says nothing, and the field spells its name out, which is the
// rule tildeGlobPattern already has — while `f z~()b` is `[zb]` there, and so
// are `f z~(+)b` and `f z~(-)b`. So the test is whether the walk will *consume*
// the group, not whether the group changes anything: a group it consumes is
// gone from the pattern, and a pattern with a piece taken out of it is not the
// name the field was written as.
//
// A remainder holding a `/` is left out, which is tildeGlobPattern's
// restriction rather than a new one: the group is the whole field's and the
// walk matches one component at a time, so honoring it would give the letters
// to one component and to no other, and the reference shell does not answer
// that shape consistently enough to copy (#3186).
//
// An escaped `\~` is stepped over rather than read, because a quoted tilde is
// marked on the way into the field and is text by the time it reaches here.
func holdsTildeFoldGroup(field string) bool {
	for i := 0; i < len(field); i++ {
		if field[i] == '\\' {
			i++
			continue
		}
		if field[i] != '~' || i == 0 {
			// A group at the very front is tildeGlobPattern's, and its rule
			// is not this one's: `f ~()zb` is the field `~()zb` in ksh93u+
			// where `f z~()b` is `[zb]`, so the empty body decides there and
			// does not decide here.
			continue
		}
		if _, _, ok := splitTildeFoldGroup(field[i:]); !ok {
			// A group the walk will not consume may still be one that
			// settles the language — `f z~(E)b` names `zb` in ksh93u+ — and
			// a field holding one describes a name just as much. See
			// findTildeFlavorGroup.
			if _, _, _, flavor := findTildeFlavorGroup(field[i:]); !flavor {
				continue
			}
		}
		_, rest, _ := splitTildeModifier(field[i:])
		if strings.Contains(rest, "/") {
			continue
		}
		return true
	}
	return false
}
