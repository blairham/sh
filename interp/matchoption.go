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

// matchOptionSet is the type Runner.matchOptions stores the switched-on
// options in, one bit each, and matchOptionBits is its width.
//
// Named rather than written inline at the field, because the width is a
// function of how many options there are and nothing said so: the set was a
// uint8 and the list had grown to exactly eight, so the next option's bit
// shifted off the end and the option read as off in every state — a dialect
// turning it on, `shopt -p` reporting it off, and the behavior behind it
// never running (#1862).
type matchOptionSet = uint16

const matchOptionBits = 16

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
	// without case, and the **substitution** operators of parameter
	// expansion with them.
	//
	// Not pathname expansion, which has GlobFoldsCase, and not the trims or
	// the case-change operator. All of that is measured, with it on: `case A
	// in a)` matches, `v=ABC; ${v//b/X}` is `AXC` and `${v/#a/Y}` is `YBC`,
	// while `${v#a}`, `${v%c}` and `${v^^b}` each leave `ABC` alone.
	//
	// The line about parameter expansion used to say it reached none of
	// those operators, cited `${x#a}` as the measurement, and was half wrong
	// for two releases: `${x#a}` cannot tell a trim from a substitution, and
	// the two do not agree (#1969). A probe that cannot separate the
	// hypotheses is not evidence for either.
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

	// StarStarSeesLinkedDirectories lets a symbolic link to a directory
	// count as one of the levels a `**` component stands for, so `**/` names
	// it with a trailing slash. It never makes the walk **enter** one: that
	// is not a dialect's choice and is refused everywhere, which is what
	// keeps `**` bounded on a tree holding a link to its own ancestor.
	//
	// On where `**` matches the entries beneath a directory and the trailing
	// slash then keeps the ones that are directories; off where the
	// component stands for the levels the walk crossed and nothing else.
	//
	// Measured 2026-09-12 in a directory holding `r/x` and a symlink `s` to
	// `r`: `echo **/` is `r/ s/` in bash 5.3.15 under `shopt -s globstar`
	// and in ksh93 under `set -o globstar`, and `r/` in zsh, which has the
	// crossing with no option asked for. The same split is why bare `**`
	// lists every entry in the first two and answers `*` in the third —
	// see StarStarAloneCrossesDirectories — but the two cannot be one
	// question here, because nothing in the panel holds one without the
	// other and a probe that cannot separate them is evidence for neither.
	//
	// It is asked only where a `**` component is the last real one, since
	// that is the only place the answer is visible: with a component behind
	// it the set is a set of directories to descend, and a link is not
	// descended whatever this says.
	StarStarSeesLinkedDirectories

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

	// TrailingGroupIsPartOfThePattern reads a parenthesized group at the end
	// of a pattern as *pattern text* rather than as the qualifier list that
	// narrows what the pattern matched. Off is the reading the one dialect
	// with glob qualifiers defaults to, which is why the bit names the other
	// state: `*.md(N)` is a pattern with a qualifier on it until a script
	// asks otherwise, and an ordinary pattern — one matching a name ending
	// `N` — afterwards.
	//
	// It reaches pathname expansion only. Measured on zsh 5.9.2, 2026-09-10:
	// with the option's own name turned off, `echo *.md(N)` in a directory
	// holding two `.md` files is `no matches found: *.md(N)` at 1 where the
	// default expands both, `echo x(N|y)` still reads the group as the
	// alternation it always was, and `[[ xN == x(N) ]]` is true either way —
	// a condition has no qualifier list to lose.
	//
	// The `(#q…)` spelling of the same list is **not** gated by it, which is
	// measured too and is the whole reason this is a check of its own rather
	// than a second reading of ExtendedPatternOperators: `echo *(#q.)` still
	// lists regular files with the bare reading turned off.
	TrailingGroupIsPartOfThePattern

	// ReplacementAmpersandIsTheMatch reads an unescaped `&` in a pattern
	// substitution's replacement as the text the pattern just matched, so
	// `${v/b/[&]}` on `abc` is `a[b]c` rather than `a[&]c`, and a global
	// `${v//pat/rep}` writes each match into its own copy of the
	// replacement.
	//
	// Only the *unquoted* `&` is read, which is the same channel quoting
	// uses everywhere else in this package: `"&"`, `'&'`, `$'&'` and `\&`
	// are all one ordinary character, and so is an `&` arriving from a
	// quoted expansion. Measured on bash 5.3.15, 2026-09-11, with `v=abc`
	// and `r='&'`: `${v/b/$r}` is `abc` and `${v/b/"$r"}` is `a&c`.
	//
	// It is an option rather than an axis because the shell that has it
	// switches it while it runs — bash 5.3's `shopt patsub_replacement`,
	// which is on with nothing said. Nothing else in the panel reads the
	// `&` at all: bash 3.2, ksh93 and zsh answer `a[&]c`, and bash 3.2 does
	// not have the option name either.
	//
	// Its escape half is the reason this changes what a backslash means and
	// not only what an `&` means. With the reading on, a `\&` that arrives
	// from an expansion loses its backslash and leaves a literal `&`, and a
	// `\\` leaves one backslash; a backslash before anything else stays —
	// `r='[\a]'` is `[\a]` with the option on as well as off. So the
	// replacement's backslashes are resolved in the same pass that reads
	// the ampersands, and only there.
	ReplacementAmpersandIsTheMatch

	// QuantifiedGroupsEverywhere reads `@(a|b)` and the other quantified
	// groups in every pattern, not only where the dialect's grammar already
	// has them. This one reaches the parser: whether `(` belongs to a group
	// is decided during tokenization, so setting it moves the runner to a
	// dialect whose ExtendedPattern is on, and input parsed after that —
	// `eval`, a sourced file, and the front end's next line — follows. See
	// syntax.Parser.SetDialect for the front end's half.
	QuantifiedGroupsEverywhere

	// lastMatchOption is the guard's subject and never a behavior. It has to
	// stay last.
	lastMatchOption
)

// Every MatchOption must have a bit in matchOptionSet. Where one does not the
// shift is taken modulo the width and the option lands on another option's
// bit, which is silent in the direction that matters — see matchOptionSet.
// This fails to compile instead.
const _ = uint(matchOptionBits - lastMatchOption)

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
		r.matchOptions |= matchOptionSet(1) << uint(o)
		return
	}
	r.matchOptions &^= matchOptionSet(1) << uint(o)
}

// MatchOption reports whether one of the behaviors is on, so the builtin that
// switches them can also answer questions about them.
func (r *Runner) MatchOption(o MatchOption) bool {
	if o == QuantifiedGroupsEverywhere {
		return r.dialect().ExtendedPattern
	}
	return r.matchOptions&(matchOptionSet(1)<<uint(o)) != 0
}
