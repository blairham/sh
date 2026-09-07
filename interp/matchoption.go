// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// MatchOption is a pattern-matching behavior a script can switch at run time.
//
// These are not axes. An axis is a fixed disagreement between dialects about
// one piece of syntax; these are states one shell lets a script move between
// while it runs, through a builtin the dialect registers. The core holds the
// state because the core is what consults it — pathname expansion and the
// matcher live here — while which names a script may use, and through which
// builtin, is entirely the dialect's.
//
// Each option is named for what it does rather than for any shell's spelling
// of it, because the spellings differ where the behaviors do not: two shells
// in the panel can empty an unmatched pattern, under different names.
type MatchOption int

const (
	// UnmatchedPatternIsEmpty expands a pattern that matches no file to
	// nothing at all, rather than leaving the pattern in place.
	UnmatchedPatternIsEmpty MatchOption = iota

	// PatternsMatchHidden lets metacharacters match a leading period, so `*`
	// sees names that begin with one. `.` and `..` are never matched; the
	// directory listing the expansion walks does not contain them.
	PatternsMatchHidden

	// GlobFoldsCase makes pathname expansion compare letters without case.
	// Only pathname expansion: `case` and `[[ ]]` have a switch of their own,
	// because the shell that has both keeps them independent.
	GlobFoldsCase

	// MatchFoldsCase makes `case` and `[[ ]]` patterns compare letters
	// without case. It does not reach pathname expansion or the pattern
	// operators of parameter expansion, which is measured: with it on,
	// `case A in a)` matches and `${x#a}` still leaves `ABC` alone.
	MatchFoldsCase

	// StarStarCrossesDirectories reads `**` standing alone as a pattern
	// component as the directory itself and everything beneath it, however
	// deep. Anything else about the component — `a**`, a quoted star — makes
	// it an ordinary pattern, where adjacent stars collapse to one.
	//
	// It governs a `**` that has another component behind it — `**/a*` — and
	// StarStarAloneCrossesDirectories decides the one that has not.
	StarStarCrossesDirectories

	// StarStarAloneCrossesDirectories extends the reading above to a `**`
	// with nothing after it, so `d/**` reaches every level rather than
	// listing the one directory. Consulted only where
	// StarStarCrossesDirectories is already on: a shell that does not read
	// `**` as a level-crossing component at all reads it as `*` in every
	// position.
	//
	// The two are separate because the panel splits on exactly this and on
	// nothing else about `**`. Measured 2026-09-07 in a directory holding
	// `ax`, `bx` and `cx/dx/ax`: `**/a*` is `ax cx/ax cx/dx/ax` in bash 5.3
	// under `shopt -s globstar`, in ksh93 under `set -o globstar`, and in zsh
	// with no option at all — so the slashed form is one behavior in three
	// shells. Bare `**` is not: bash and ksh93 list every level, and zsh
	// answers `ax bx cx`, which is what `*` answers. That is why turning the
	// crossing on for zsh needs this second question rather than one flag.
	StarStarAloneCrossesDirectories

	// ExtendedPatternOperators reads the pattern operators one shell keeps
	// behind an option of its own: the parenthesized flag groups `(#…)`, the
	// closures `#` and `##`, the negation `^pat` and the exclusion
	// `pat1~pat2`.
	//
	// It is an option rather than a dialect flag because the shell that has
	// them switches it while it runs, and because the same characters are
	// *ordinary text* with it off — measured, and the measurement is the
	// reason this is not simply always on: `[[ 'a#' == a# ]]` matches with
	// the option off and does not with it on, and `(#i)abc` off is a group
	// holding one alternative that matches the four characters `#iab` and a
	// `c`. Turning the reading on unconditionally would break every pattern
	// in the other direction, silently.
	//
	// Unlike QuantifiedGroupsEverywhere this reaches no grammar: the
	// constructs are read out of the pattern *text* by the matcher, so a
	// pattern held in a variable is read the same way as one written down,
	// which is what that shell does.
	ExtendedPatternOperators

	// QuantifiedGroupsEverywhere reads `@(a|b)` and the other quantified
	// groups in every pattern, not only where the dialect's grammar already
	// has them. This one reaches the parser: whether `(` belongs to a group
	// is decided during tokenization, so setting it moves the runner to a
	// dialect whose ExtendedPattern is on, and input parsed after that —
	// `eval`, a sourced file, and the front end's next line — follows. See
	// syntax.Parser.SetDialect for the front end's half.
	QuantifiedGroupsEverywhere
)

// SetMatchOption switches one of the behaviors on or off.
//
// The grammar-reaching option copies the dialect rather than writing through
// the shared pointer: a subshell is a cloned runner holding the same Dialect,
// and a script must not change the grammar of the shell that spawned it.
func (r *Runner) SetMatchOption(o MatchOption, on bool) {
	if o == QuantifiedGroupsEverywhere {
		d := r.dialect()
		if d.ExtendedPattern != on {
			d.ExtendedPattern = on
			r.Dialect = &d
		}
		return
	}
	if on {
		r.matchOptions |= 1 << uint(o)
		return
	}
	r.matchOptions &^= 1 << uint(o)
}

// MatchOption reports whether one of the behaviors is on, so the builtin that
// switches them can also answer questions about them.
func (r *Runner) MatchOption(o MatchOption) bool {
	if o == QuantifiedGroupsEverywhere {
		return r.dialect().ExtendedPattern
	}
	return r.matchOptions&(1<<uint(o)) != 0
}
