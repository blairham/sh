// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// tildeFoldGroupLetters is the body a `~(…)` group may hold and still be read
// where it stands rather than only at the front of the pattern: the sense
// signs, and the letters whose whole meaning is a request about the match
// rather than about the language it is written in.
//
// The ones left out are the *flavors* — `E X P G V F L` choose a
// regular-expression language for the whole pattern rather than setting
// something on a branch, and interp/tildeflavorhere.go reads those before any
// of this runs — and the letters ksh93 has that this shell does not answer,
// which stay the ordinary characters they were rather than being taken and
// dropped.
//
// What each of the letters here asks for, measured on ksh93u+ 2012-08-01,
// 2026-09-27, `-c` under `env -i` with a scratch `HOME`:
//
//   - `i` folds case for the rest of the branch, which is what
//     splitTildeHereGroup's own rows are about;
//   - `K p s` choose ksh's own glob, which is the language the walk is
//     already in, so they are consumed and change nothing. That is what
//     `[[ zab == z~(K)a* ]]` measures: it matches there, so the group is
//     read and the `a*` behind it is an ordinary glob;
//   - `l r` pin the match to an end of the **subject**, which is the same
//     question matchTilde answers for a group at the front — and a request
//     rather than a flag on a branch, so tildeHereAnchors asks it once for
//     the whole pattern;
//   - `g` makes a prefix trim take the longest piece its pattern will match,
//     which reaches tildeGreedyTrim, a caller outside the matcher;
//   - `N` deletes the word where the pattern names nothing — **at the front
//     only**, and that is measured rather than an omission. With `za` and
//     `zb` on disk, `f z~(N)a` is `za` there, so the group is consumed; and
//     `f z~(N)z*` is the word `z~(N)z*`, where a `N` that reached the word
//     would have deleted it. So the letter is consumed here and asks for
//     nothing, and Runner.tildeGlobPattern goes on reading a front group's.
//
// This used to say `g`, `l` and `r` were about "which span of the subject the
// match takes, and a branch cannot change that halfway along", which is right
// about the mechanism and was **wrong about the shell**: `v=aXbXc;
// ${v#a~(g)*X}` is `c` there and `[[ zab == z~(l)ab ]]` matches. #4893 has
// the rows.
//
// A group holding a letter this shell does not answer is still left exactly
// as it was — four or more ordinary characters — and that is a silent wrong
// answer rather than a reading: `[[ zab == z~(M)ab ]]` matches in ksh93u+ and
// does not here. It is its own row rather than this one's, because taking the
// letter and dropping it is the shape this repository minds most and refusing
// it by name is a change to Runner.tildeModifierOpts.
const tildeFoldGroupLetters = "+-iKpsglrN"

// tildeHereGroup is what a `~(…)` group the walk can consume asks of the
// pattern it stands in.
//
// foldSet is separate from fold because `~(-i)` is a real answer and not the
// absence of one: a group with no `i` in it asks nothing about case, and one
// with `-i` asks for the fold to stop. The other three need no such pairing —
// an anchor nobody asked for and an anchor turned off are the same absence of
// a requirement, and so are the two spellings of not being greedy — except
// that greedy has to be able to *unset* an earlier group's request, which is
// what greedySet carries.
type tildeHereGroup struct {
	foldSet   bool
	fold      bool
	left      bool
	right     bool
	greedySet bool
	greedy    bool
}

// splitTildeHereGroup peels a `~(…)` group off the front of p when the group
// is one this position can honor, reporting what it asks for.
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
//
// The other letters are consumed on the same rows, `-c` this time so that the
// question is the match rather than the filesystem:
//
//	[[ zab == z~(l)ab ]]   yes   [[ zab == z~(r)ab ]]   yes
//	[[ zab == z~(g)ab ]]   yes   [[ zab == z~(N)ab ]]   yes
//	[[ zab == z~()ab  ]]   yes   [[ zab == z~(Z)ab ]]   **no**
//
// The last one is the control that says this is a group being read rather
// than every `~(…)` being stepped over: `Z` is a letter no ksh93 has, and the
// pattern that holds one cannot match.
func splitTildeHereGroup(p string) (g tildeHereGroup, ok bool) {
	body, _, split := splitTildeModifier(p)
	if !split {
		return tildeHereGroup{}, false
	}
	for i := 0; i < len(body); i++ {
		if strings.IndexByte(tildeFoldGroupLetters, body[i]) < 0 {
			return tildeHereGroup{}, false
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
			g.foldSet, g.fold = true, sense
		case 'l':
			g.left = sense
		case 'r':
			g.right = sense
		case 'g':
			g.greedySet, g.greedy = true, sense
		}
	}
	return g, true
}

// tildeHereAnchors reads `l` and `r` out of every `~(…)` group the walk can
// consume, and is how a mid-pattern one reaches the comparison matchTilde
// makes for a group at the front.
//
// They are asked **once for the whole pattern** rather than where the group
// stands, and that is the mechanism rather than the reading: an anchor is a
// question about where the *piece* a surface handed over sits in the subject,
// which matchPatternIn knows and the walk does not — the walk's memo is keyed
// on a position and would alias two trials of a suffix trim that reach the
// same position from different starts. The cost is that a group inside an arm
// the match never takes still asks, which is a limit and is not a row anybody
// has measured.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-27, with `v=abcd`:
//
//	${v#a~(l)bc}    d       so `l` holds where the piece starts the subject
//	${v#a~(r)bcd}   empty   and `r` where it ends it
//	${v#a~(r)bc}    abcd    and refuses where it does not
//	${v/a~(r)bc/X}  abcd    the same refusal in a substitution
//
// **A shortest suffix trim does not read `l` at all**, and the caller is what
// knows that: `${v%~(l)d}` is `abc` there where `${v%%~(l)d}` leaves `abcd`
// alone, so the reading is the operator's. patternOpts.tildeLeftUnread
// carries it and trimLeavesTildeLeftUnread has the rows. The check is made in
// one place for a front group and a mid-pattern one alike, which is the point
// of asking it here: the two cannot drift.
func tildeHereAnchors(pattern string) (left, right bool) {
	if strings.IndexByte(pattern, '~') < 0 {
		return false, false
	}
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' {
			i++
			continue
		}
		if pattern[i] != '~' {
			continue
		}
		if g, ok := splitTildeHereGroup(pattern[i:]); ok {
			left, right = left || g.left, right || g.right
		}
	}
	return left, right
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
		if _, ok := splitTildeHereGroup(field[i:]); !ok {
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
