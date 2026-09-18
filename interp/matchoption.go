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
//
// Widened from uint16 when the two `**` options below took the fifteenth and
// sixteenth bits. The guard under lastMatchOption would have caught the
// seventeenth at compile time, which is what it is for, but a width with no
// room left in it is a trap set for whoever adds the next option rather than
// a bound anybody chose (#3152).
type matchOptionSet = uint32

const matchOptionBits = 32

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
	//
	// It reaches the *pattern* operators of `[[ ]]` and not the regular
	// expression one: `=~` has RegexFoldsCase, because the panel keeps the
	// two apart under one name — see there.
	MatchFoldsCase

	// RegexFoldsCase makes the `=~` operator compare letters without case,
	// which is a different mechanism from MatchFoldsCase and not a second
	// name for it: `=~` is a regular expression, so the fold is a property
	// of the compiled expression rather than a comparison the matcher makes
	// a character at a time.
	//
	// It is separate because **the two shells that have the option bundle it
	// differently, under the same spelling.** Measured 2026-09-13 on bash
	// 5.3.15, bash-as-`sh`, bash 3.2.57 and zsh 5.9.2, with each shell's own
	// `nocasematch` turned on:
	//
	//	                       bash    zsh
	//	[[ ABC =~ ^abc$ ]]     yes     yes
	//	[[ ABC == abc ]]       yes     no
	//	case A in a)           hit     exact
	//	v=ABC; ${v//b/X}       AXC     ABC
	//
	// So bash's name turns both of these on and zsh's turns only this one
	// on, which is the trap a shared spelling sets: the name is the same and
	// the feature behind it is not. `nocaseglob`/`caseglob` is a third
	// thing again and reaches neither — it is GlobFoldsCase, pathname
	// expansion alone.
	//
	// ksh93, dash and BusyBox ash have no option of the kind at all: dash
	// and ash have neither `[[ ]]` nor `=~`, and ksh93 has `=~` and refuses
	// both `set -o nocasematch` and `shopt`. Its `~(i)` pattern flag is an
	// inline spelling rather than a switch, the way zsh's `(#i)` is. So the
	// panel does not split on the cell this governs — every member that can
	// ask the question answers it the same way — and it is a correction
	// rather than an axis.
	//
	// The fold reaches the whole expression and not only its literals, which
	// is measured rather than assumed and is what makes `(?i)` the right
	// mechanism: with bash's `nocasematch` on, `[[ ABC =~ ^[[:lower:]]+$ ]]`
	// and `[[ abc =~ ^[[:upper:]]+$ ]]` and `[[ ABC =~ ^[a-c]+$ ]]` all
	// match, and the negation folds with them — `[[ A =~ ^[^a]$ ]]` does
	// *not* match, so the fold happens before the class is complemented.
	RegexFoldsCase

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
	// It is asked only where a `**` component is the last real one. With a
	// component behind it the question is a different one, because the set
	// the next component looks in need not be the set the walk entered —
	// see ComponentBehindStarStarSeesLinkedLevels, which one column answers
	// the other way round from this.
	StarStarSeesLinkedDirectories

	// ComponentBehindStarStarSeesLinkedLevels lets the component after a
	// `**` look inside a level the walk **listed and did not enter**, which
	// a symbolic link to a directory always is.
	//
	// The walk itself is unchanged and stays bounded: nothing descends
	// through such a link, so a tree holding a link to its own ancestor is
	// still finite. What moves is only the set the next component is offered
	// — everything the `**` matched, rather than the directories it read.
	//
	// It is the other half of StarStarSeesLinkedDirectories rather than the
	// same answer, and the panel proves they are two questions: the column
	// that names a linked level for `**/` and the column that looks inside
	// one behind a `**` are not the same column.
	//
	// Measured 2026-09-18 in a tree holding `r/x`, a symlink `s` to `r`, and
	// `a/b/sl` where `sl` is a second symlink to `r`:
	//
	//	            **/       a/**/x      ./**/x        a/**/[x]
	//	bash 5.3.20 r/ s/ …   [a/b/sl/x]  [./a/b/sl/x]  [a/b/sl/x]
	//	                                  [./r/x]
	//	                                  [./s/x]
	//	ksh93u+     r/ s/ …   no match    [./r/x]       no match
	//	zsh 5.9.2   r/ …      no match    [./r/x]       no match
	//
	// So ksh93 names a linked level and never looks inside one, which is the
	// pair that made the older option's scope note wrong: it said a link is
	// not descended whatever the option says, which is true, and concluded
	// that nothing behind a `**` could see one, which is not.
	//
	// **It is not asked of a `**` that begins the word.** That is measured
	// rather than chosen, and it is why the same tree answers two ways in
	// the one column that has this on: `**/x` is `[r/x]` there and `./**/x`
	// is `[./r/x][./s/x][./a/b/sl/x]`, over the same files, with the leading
	// `.` the only difference between the patterns. An absolute spelling of
	// the same walk looks inside, `**//x` looks inside, and `a/**/**/x`
	// looks inside — the run of `**` having collapsed to one component that
	// is no longer the first. The carve-out is that narrow: the word's very
	// first component, with one separator and a real component behind it.
	// Modeling the shell's answer means modeling that too, since a pattern
	// written either way is a pattern scripts write.
	//
	// Consulted only where StarStarCrossesDirectories is already on.
	ComponentBehindStarStarSeesLinkedLevels

	// RepeatedStarStarIsOneComponent reads a run of `**` components with
	// nothing but separators between them as a single one, so `**/**` is
	// `**` and `**//**` is `**` as well — the empty component goes with the
	// run rather than surviving it.
	//
	// Without it each `**` is an alternative of its own and the run is a
	// cross product, which is visible because **no shell takes duplicates
	// out of a pathname expansion**. Measured 2026-09-13 in a tree of
	// directories `a` and `b` nested three deep: `echo **/**/` is 14 names
	// in bash 5.3.15 under `shopt -s globstar` and in ksh93 under `set -o
	// globstar`, and 48 in zsh — the same 14 with each name repeated once
	// per way of splitting it between the two components, `a/` twice and
	// `a/a/a/` four times.
	//
	// Consulted only where StarStarCrossesDirectories is already on, since a
	// shell that reads `**` as `*` has no run to collapse.
	RepeatedStarStarIsOneComponent

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

	// UnmatchedPatternIsError refuses a pattern that matched no file: the
	// shell reports it and the command does not run.
	//
	// The other half of UnmatchedPatternIsEmpty and not its negation, which
	// is why it is a second option rather than a third state of the first.
	// Three answers are reachable and a script picks between them a name at
	// a time: leave the pattern in place, delete the word, or refuse.
	//
	// **Where it and UnmatchedPatternIsEmpty are both on, the refusal wins.**
	// That is measured rather than chosen, and it is the opposite of the
	// order Semantics.GlobNoMatchIsError keeps with the same emptying option
	// one field along — the shell that has these two as `shopt` names refuses
	// with both set (`shopt -s nullglob failglob; echo nosuch*` is the
	// complaint at 1, in either order), and the shell that has the axis
	// deletes the word (`setopt nullglob; echo zz*` is an empty line at 0
	// with nomatch still on). Two shells, two orders; a single rule for both
	// would have to be wrong for one of them.
	//
	// How far the refusal reaches is not this option's to say. It ends what
	// Semantics.FailedExpansionAbandonsTheLine ends — the statement in the
	// shell that answers yes and the script in the shells that do not — so
	// the two dialects that can reach this path get their own measured scope
	// without either naming the other.
	UnmatchedPatternIsError

	// StarStarZeroLevelIsTheDirectoryItStartsFrom reports a `**` that
	// matched **zero** levels as the directory the walk stood in when it
	// reached the component, so `d/**` names `d/` ahead of what is inside
	// it. Off, the zero-level match is whatever the component ahead of the
	// `**` **listed** instead — which is nothing at all where the path was
	// spelled out, and includes plain files where it was not.
	//
	// The one question about `**` the panel splits three ways rather than
	// two, and the split is ksh93 against the other two. Measured 2026-09-16
	// in a tree holding `p/pf`, `p/q/qf` and `p/q/w/leaf`, rendered one word
	// per `[…]` because `echo` joins with spaces and hides exactly this:
	//
	//	p/**      ksh93  [p/pf][p/q][p/q/qf][p/q/w][p/q/w/leaf]
	//	          bash   [p/][p/pf][p/q][p/q/qf][p/q/w][p/q/w/leaf]
	//	*/**      ksh93  [p][p/pf][p/q][p/q/qf][p/q/w][p/q/w/leaf][topf]
	//	          bash   [p][p/pf][p/q][p/q/qf][p/q/w][p/q/w/leaf]
	//	p/*/**    both   [p/pf][p/q][p/q/qf][p/q/w][p/q/w/leaf]
	//	*/q/**    ksh93  [p/q/qf][p/q/w][p/q/w/leaf]
	//	          bash   [p/q][p/q/qf][p/q/w][p/q/w/leaf]
	//	**/q/**   both   [p/q][p/q/qf][p/q/w][p/q/w/leaf]
	//
	// Two things fall out of that table and neither is the symmetric guess.
	// `topf` is a **file** and ksh93 names it for `*/**`, so the zero-level
	// match there is not a directory the walk stood in — it is the `*`
	// component's own listing, before anything filtered it down to what
	// could be descended into. And `*/q/**` and `**/q/**` differ while
	// spelling the same `q`, so it is not the whole prefix that decides it
	// either.
	//
	// What fits every row is where the starting directory's **name** came
	// from. A literal component is joined onto the path and stat'd; a
	// pattern is resolved by reading the directory it sits in, and so is a
	// literal that follows a `**`, because that walk is already listing
	// every level it crosses. So the zero-level match exists exactly where
	// the start came out of a listing, and the listing is what it reports.
	// See globZeroLevelSource, which is that rule and nothing else.
	//
	// Consulted only where StarStarCrossesDirectories is already on, and
	// reachable only where a `**` is the last real component: with something
	// behind it the set is a set of directories to descend and a zero-level
	// match is a place to carry on from rather than an answer.
	StarStarZeroLevelIsTheDirectoryItStartsFrom

	// StarStarPatternsReadLinkedDirectories lets a pattern holding a
	// level-crossing `**` list a directory it reached through a symbolic
	// link, so `s/**` with `s` a link to `r` lists what is under `r`.
	//
	// The whole pattern and not the `**` component alone, which is measured
	// and is the reason this is one option rather than two. One shell in the
	// panel answers a `**` pattern with a **physical** walk and reads no
	// directory through a link anywhere in it — not the one the walk starts
	// in, and not one an earlier pattern component would have listed — while
	// the same shell reads straight through the same link for a pattern with
	// no `**` in it.
	//
	// It is not about following a link the walk *meets* on the way down: no
	// dialect does that, which is what keeps `**` bounded on a tree holding
	// a link to its own ancestor, and StarStarSeesLinkedDirectories decides
	// only whether such a link is *named*.
	//
	// Measured 2026-09-16 in a tree holding `r/x`, a symlink `s` to `r`, a
	// three-deep `tree/` holding `tree/a/b/c` and `tree/afile`, and a
	// symlink `t` to `tree`:
	//
	//	s/**       bash [s/][s/x]   zsh [s/x]   ksh93 [s/**] — no match
	//	s/**/x     bash [s/x]       zsh [s/x]   ksh93 [s/**/x] — no match
	//	t/**       bash [t/][t/a]…  zsh [t/a]…  ksh93 [t/**] — no match
	//	t/*/**     bash [t/a/b]…                ksh93 [t/*/**] — no match
	//	*/*/**     bash …[t/a]…                 ksh93 no `t/` name at all
	//	**/s/**    bash [s][s/x]    zsh [s/x]   ksh93 [s]
	//
	// Three rows pull the rule away from the guesses next to it. `s/./**`
	// is `[s/./x]` in **every** column, so it is the directory a listing is
	// read from that matters and not the text of the path. `*/a/**` is
	// `…[t/a/b]…` in ksh93 too, because a *literal* component is joined onto
	// the path and stat'd rather than listed — the same split
	// globZeroLevelSource turns on. And `*/*` without a `**` anywhere lists
	// `[t/a][t/afile][t/b]` in that shell with the option on, so the
	// physical reading belongs to the pattern that holds a `**` rather than
	// to the option being set.
	//
	// Consulted only where StarStarCrossesDirectories is already on.
	StarStarPatternsReadLinkedDirectories

	// PeriodPatternListsDotAndDotDot puts `.` and `..` among the names a
	// pattern component may match, where **that component begins with a
	// literal period**.
	//
	// The other half of the question Semantics.GlobListsDotAndDotDot asks,
	// and the two are separate because the shells reach the same two names by
	// different rules. The axis holds the shells whose directory listing
	// simply contains them, where the leading-period rule is what keeps them
	// out of an ordinary `*` — turn that rule off there and `echo *` lists
	// them. This option's shell never has them in the listing at all, and
	// turning the leading-period rule off does *not* bring them back: with
	// both the dot-matching option on and this one on, `*` is still only the
	// entries. So one rule cannot serve both, and a second reading of the
	// axis would have made the wrong shell list `.` for `*`.
	//
	// Measured 2026-09-17 on bash 5.3.20 under `LC_ALL=C`, in a directory
	// holding `.a`, `.b`, `vis` and a subdirectory `sub`, with the option on
	// — which is the state `shopt -u globskipdots` puts that shell in:
	//
	//	.*                     . .. .a .b
	//	*                      sub vis
	//	dotglob on, then *     .a .b sub vis
	//	*/.*                   sub/. sub/.. sub/.x
	//	.*/                    ../ ./
	//	globstar on, then **   sub sub/y vis
	//
	// Four things fall out of that table. It is the *component* and not the
	// pattern, so `*/.*` sees them one level down. The listing is sorted with
	// them in it. The trailing-slash form keeps only what is a directory,
	// which is what leaves `.a` out of the fifth row. And a `**` descent does
	// not list them at all, so the walk is untouched — the same gap
	// globListingNames already records for the axis, and here it is the
	// measured answer rather than a gap.
	//
	// Off is the reading every shell in the panel that has no such option
	// gives, which is why the bit names the state a script has to ask for:
	// the two names are absent until something says otherwise.
	PeriodPatternListsDotAndDotDot

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
