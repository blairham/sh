// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// Answer is one axis's value, and it has three states rather than two.
//
// Unspecified is the point of the type. The vector contains *only* the places
// the shells disagree — everything they agree about never became an axis and
// is simply the implementation — so leaving one unset is not an oversight, it
// is saying "no shell has been chosen here". A script that depends on such an
// axis is refused, naming it, rather than silently getting one shell's answer.
type Answer uint8

const (
	// Unspecified refuses the behavior rather than guessing at it.
	Unspecified Answer = iota
	Yes
	No
)

func (a Answer) String() string {
	switch a {
	case Yes:
		return "yes"
	case No:
		return "no"
	}
	return "unspecified"
}

// Semantics is where the shells disagree about what identical syntax *means*.
//
// This is the structure docs/spec/semantics.md argued for, and the argument
// is worth restating because the obvious alternative looks fine. Grammar
// differences are additive — a construct either parses or it does not — and
// syntax.Dialect models those. Semantic differences are conflicts: the same
// text means different things, and no amount of adding or removing features
// produces one from another. They need switches.
//
// Every field is named for the behavior rather than for the shell that wants
// it, which the spec requires and the measurements insist on: ksh93 accepts
// `&>` or does not depending on which build is installed, twelve years apart
// under the same name, so a field called `Ksh` could not be given a value.
//
// The axes were measured across four shells and produced groupings that
// overlap and contradict — no ordering of the shells explains the data,
// which is why this is a vector and not a level.
type Semantics struct {
	// SplitParamExpansion field-splits the result of an unquoted parameter
	// expansion. False in zsh, and narrower than "word splitting": zsh still
	// splits an unquoted *command* substitution, so this is two fields and
	// not one.
	SplitParamExpansion Answer
	// SplitCommandSubstitution field-splits an unquoted command
	// substitution. True everywhere measured, including zsh.
	//
	// unanimous: the content of this axis is the *contrast* with
	// SplitParamExpansion, not a split of its own, so the four answering
	// alike is the measurement rather than the absence of one. Re-measured
	// 2026-09-12 with `IFS` a space and a value of three blank-separated
	// words, `set -- $v` beside `f() { echo "$v"; }; set -- $(f)`: the
	// command substitution gives three fields in dash, bash 5.3.15,
	// bash-as-`sh`, bash 3.2.57, ksh93u+ and zsh 5.9.2, and the parameter
	// gives one in zsh alone. Delete this and
	// that single measurement reads as "zsh does not word-split", which is
	// the misreading the pair exists to prevent (#2060).
	//
	// unexhibited No: nobody, and nothing reaches it. The probe above is
	// unanimous across all six columns and zsh has no option that turns
	// the splitting off, so `No` is the other side of a binary Answer
	// rather than a claim about a shell. It stays because this axis is
	// asked rather than read: a dialect that did leave an unquoted
	// substitution unsplit has somewhere to say so, and until one does the
	// value is unheld on purpose (#2060).
	SplitCommandSubstitution Answer
	// UnquotedListJoinsOnIFS makes an unquoted list expansion one string —
	// the elements joined on the first character of IFS — before the split
	// above runs on it, rather than splitting each element on its own.
	//
	// True in bash, bash 3.2 and bash as `sh`; false in zsh, ksh93 and dash.
	// It is one question wearing two faces, and both were being answered
	// without asking: `$@` and `${a[@]}` never joined, which is right for
	// three of the six, and `$*` and `${a[*]}` always did, which is right for
	// the other three.
	//
	// The join is what decides the fate of an *empty* element, which is where
	// it shows. Under a non-whitespace IFS, `set -- x "" y` is `x::y` joined
	// and splits back to three fields in bash, where splitting each element
	// on its own drops the empty one and leaves two. The same join is why
	// bash then *loses* a trailing empty element — `set -- x y ""` is `x:y:`,
	// and a trailing separator makes no field — while zsh, which does not
	// join, keeps it. No arrangement of the splitting answer alone reaches
	// either reading, which is why this is a question of its own.
	//
	// Asked only at the disagreement: the two readings coincide under a
	// whitespace IFS, which is why `a=("" x)` is `[x]` in every shell
	// measured and needs no answer, and there is nothing to join with when
	// IFS is set and empty. See Runner.elementFields, which computes both and
	// asks only when they differ.
	//
	// The *quoted* spellings are not this question and must not reach it:
	// `"$*"` and `"${a[*]}"` join on the first character of IFS in every
	// shell measured, and `"$@"` and `"${a[@]}"` keep one field per element
	// in every shell measured. Both are core.
	UnquotedListJoinsOnIFS Answer

	// UnsplitAtListJoinsOnIFS decides the character an unquoted list spelled
	// `@` is joined with when it reaches a context that keeps no fields: the
	// first character of IFS, or a hard space.
	//
	// True in zsh and dash; false in bash, bash 3.2, bash as `sh` and ksh93.
	// Measured 2026-09-06 in the four contexts that never split — an
	// assignment's value, a `case` subject, a `[[ ]]` operand and a
	// here-document body — with `IFS=-` and a three-element list:
	//
	//	IFS=-; a=(x y z); v=${a[@]}   zsh x-y-z · bash, ksh93 x y z
	//	IFS=-; set -- x y z; v=${@}   zsh, dash x-y-z · bash, ksh93 x y z
	//
	// All four contexts answer alike within each shell, which is what makes
	// this one axis rather than one per context.
	//
	// The `*` spelling is **not** this question and must not reach it. `$*`,
	// `${a[*]}` and a range subscript join on the first character of IFS in
	// every graded dialect's shell, so that half is core — see
	// Runner.unsplitJoinSeparator, which answers the star before it asks.
	//
	// Asked only at the disagreement, and the guard is not the one
	// UnquotedListJoinsOnIFS uses. Here the join happens either way and only
	// its character is in question, so an IFS that is *set and empty* is a
	// live answer rather than a reason not to ask: `IFS=""; a=(x y); v=$a`
	// is `xy` in zsh against `x y` in bash, and joining with nothing is
	// exactly what zsh does. What does make the readings coincide is an IFS
	// whose first character is already a space — which is every script that
	// leaves IFS alone, and the reason this is silent — and a list of fewer
	// than two elements, which uses no separator at all.
	UnsplitAtListJoinsOnIFS Answer

	// TrailingSeparatorEndsAField makes the non-whitespace IFS separator that
	// closes a value open one last empty field, rather than being absorbed.
	//
	// False in bash, bash 3.2, bash as `sh`, dash and ksh93, and true in zsh
	// — the one axis in this neighborhood where the panel splits five to one.
	// Measured 2026-09-07 with `IFS=:` and an unquoted `$v` (zsh under
	// `setopt shwordsplit`, the only way to ask it there):
	//
	//	v='a:'    five shells [a]        · zsh [a][]
	//	v='a::'   five shells [a][]      · zsh [a][][]
	//	v='a:b:'  five shells [a][b]     · zsh [a][b][]
	//	v=':a:'   five shells [][a]      · zsh [][a][]
	//	v=':'     five shells []         · zsh [][]
	//
	// A *leading* separator opens a field in all six, so the asymmetry is at
	// the tail alone and this axis is the whole of it. POSIX.1-2024 2.6.5
	// answers it too — "once the input is empty, the candidate shall become
	// an output field if and only if it is not empty" — which is the
	// absorbing reading, so PosixSemantics says No and zsh is the departure.
	//
	// Asked only at the disagreement, and the guard is what keeps it off
	// every ordinary script: it is the closing *run* of separators that
	// decides, and only a non-whitespace one in that run makes the two
	// readings differ. `' a '` under the default IFS is one field in all six,
	// because whitespace is absorbed at both ends in zsh as well; `'a: '`
	// with `IFS=' :'` is the case that shows it is the run rather than the
	// last byte, since the trailing space does not hide the colon in front of
	// it. An escaped separator is data and not part of the run at all, which
	// is why the mask `read` carries has to reach this question: `read -A` on
	// `a\:` is one field `a:` in zsh where `a:` is two.
	//
	// It is not [Semantics] alone that answers a split — an unquoted
	// `${=spec}` keeps the fields at *both* edges unconditionally, which is a
	// different rule and reaches the splitter as its own parameter. Where
	// that rule is in force this question is never asked, because the field
	// behind the last separator is already there.
	TrailingSeparatorEndsAField Answer

	// ReadTrailingWhitespaceEndsAField is the same question about a closing
	// run of IFS *whitespace*, and it is asked of `read` alone because one
	// shell answers the two differently.
	//
	// Measured 2026-09-07 with a here-string into `read -A`/`-a` and the
	// element count read back:
	//
	//	read -A r <<< 'a  '        bash, ksh93 [a]      · zsh [a][]
	//	read -A r <<< ' a '        bash, ksh93 [a]      · zsh [a][]
	//	read -A r <<< "$(printf 'a\t')"   bash, ksh93 [a] · zsh [a][]
	//	read -A r <<< 'a: ', IFS=': '      bash, ksh93 [a] · zsh [a][]
	//
	// And the same shell, same IFS, through an expansion instead:
	// `x=' a '; set -- ${=x}` is **one** field in zsh. So this cannot ride on
	// TrailingSeparatorEndsAField — that one is measured on an expansion and
	// is right there, and folding them would make `${=x}` grow a field zsh
	// does not give it.
	//
	// A *leading* run of whitespace is still absorbed — `read -A r <<< ' a'`
	// is one element in all three — so the asymmetry is at the tail here as
	// it is above, and the run rather than the byte decides: `a::` with
	// `IFS=:` is three elements in zsh and not four.
	//
	// Only the array target can see it. With a list of names the last name
	// takes the remainder of the *line* and the closing whitespace comes off
	// that remainder anyway, so `read x <<< 'a  '` is `a` in every column
	// whichever way this is answered — which is why it is asked in `read`'s
	// splitting rather than at the array store, and observed there.
	ReadTrailingWhitespaceEndsAField Answer

	// ReadNoFieldsIsOneEmptyElement leaves an array `read` filled from a line
	// that split into nothing at all holding one empty element rather than
	// none.
	//
	// Measured 2026-09-07: `read -A r <<< ''` is one empty element in ksh93
	// and zsh and no elements in bash, and a line of nothing but IFS
	// whitespace answers the same way in each. bash's is the ordinary field
	// split — no text, no fields — which is what the POSIX preset takes,
	// since the letter is not in the standard at all.
	//
	// Asked after ReadTrailingWhitespaceEndsAField and only where nothing is
	// left: the shell that opens a field on a closing whitespace run already
	// has one by then, and asking before it would give that shell two
	// elements for a line of spaces where it gives one.
	ReadNoFieldsIsOneEmptyElement Answer

	// ReadTrailingEscapedSeparator is what `read` does with an IFS
	// *whitespace* character the line escaped at the very end of the value
	// its last name takes. Without `-r` a backslash makes the character
	// after it data, and `read` carries that as a mask into the splitter;
	// the trim at the tail does not agree across the panel about whether the
	// mask reaches it.
	//
	// Measured 2026-09-12 from a script file, default IFS, each row a `read`
	// of its own over one printed line:
	//
	//	          `a b c\ `   `a b\ `   `a\ `    `a b\ c\ `
	//	          read x y    read x y  read x   read x y
	//	          the last name's value
	//	dash      b c         b         a        b c
	//	                      ^ keeps          all four keep the space
	//	bash      b c         b         a        b c
	//	ksh93     b c         b         a        b c
	//
	// — with the trailing space shown by the brackets in the corpus row
	// rather than here. Three columns and three answers:
	//
	//	dash    keeps the escaped space everywhere: the mask reaches the trim
	//	bash    trims it, but only from a value that took a *remainder*
	//	ksh93   trims it from the last name's value however it was reached
	//	zsh     ksh93's answer
	//
	// The bash column is the one that needs the third value, and the row
	// that says so is `a b\ c\ `: the escaped space in the middle joins `b`
	// and `c` into one field, so the line holds exactly one field per name
	// and the last name takes its own field rather than a remainder — bash
	// keeps the closing space there and trims it in the first column, where
	// there are three fields for two names.
	//
	// Two questions the axis does *not* have to carry, both measured the
	// same day and both unanimous:
	//
	//   - a **non-whitespace** separator. The trim only ever takes
	//     whitespace, so the mask cannot be seen through it: with `IFS=:`,
	//     `a:b:c\:` gives `b:c:` in all six, and so does the unescaped
	//     `a:b:c:`.
	//   - a non-default **whitespace** IFS. With `IFS` a tab and tabs for
	//     separators the rows split exactly as above, so the answer is about
	//     the trim and not about which character it is trimming.
	//
	// Asked where the readings land differently and nowhere else: a value
	// whose closing IFS whitespace was escaped. `-r` never reaches it,
	// because there is no mask for the trim to disagree about (#1360).
	ReadTrailingEscapedSeparator ReadTrailingEscapedSeparatorPolicy

	// GlobExpansionResults matches the *result* of an expansion against the
	// filesystem. False in zsh, where only a pattern written literally in the
	// source is expanded. The same rule decides whether `[[ abc == $p ]]`
	// treats $p as a pattern, which is one behavior observed twice rather
	// than two quirks.
	//
	// It is the one axis in this vector with a *run-time* name over it: the
	// shell that answers `No` gives a script `setopt globsubst` to say
	// otherwise, so the dialect moves this answer rather than carrying a bit
	// of its own. The per-expansion spelling `${~spec}` overrides it for one
	// expansion and is not an option — see interp/tildeflag.go, where the two
	// meet.
	GlobExpansionResults Answer
	// ValueBackslashInAPattern is what a backslash that arrived in a **value**
	// does to the character behind it when the field is then matched as a
	// pattern. Three readings, and no two of them can stand in for each other.
	//
	// Measured 2026-09-12 from a script file, in directories holding exactly
	// the names either reading would find -- a directory holding neither
	// prints the same word whichever rule is in force, which is what makes
	// these arrangements discriminating rather than merely plausible.
	//
	// With `a\\b` and `a*` present:
	//
	//	v='a\\*'; set -- $v
	//
	//	dash, bash 5.3.15, bash-as-`sh`, bash 3.2.57, zsh 5.9.2   [a\\*]
	//	ksh93u+                                                  [a\\b]
	//
	// With `a\\bc` and `ab` present:
	//
	//	v='a\\b*'; set -- $v
	//
	//	dash, bash 5.3.15, bash-as-`sh`, bash 3.2.57   [ab]
	//	ksh93u+                                       [a\\bc]
	//	zsh 5.9.2                                     [a\\b*]
	//
	// The second probe is what makes it three answers. bash reads the
	// backslash as a quote that is **not itself matched**, so the pattern is
	// `ab*`; ksh93 reads it as data with the `b` live; and zsh -- reached
	// through `${~spec}`, since it globs no expansion result otherwise --
	// reads it as data with the `b` *disarmed*, which is the same match a
	// literal `a\\b*` makes. The first probe cannot tell bash from zsh, because a
	// disarmed `*` and a quoted one both leave nothing to glob.
	//
	// Neither live reading removes the backslash from the *text*: a shell
	// performs no quote removal on the result of an expansion, so a failed
	// match restores the word with the backslash in it. `v='a\\b[c]'` is
	// `a\\b[c]` in bash and dash for that reason and `a\\bc` in ksh93, which
	// found a name.
	//
	// The doubled backslash is the row that says a quoting backslash needs a
	// symbol of its own rather than the marked-backslash-plus-marked-character
	// the escaped form already had: `v='a\\\\*'` is `a\\b` in bash -- the first
	// backslash quoting the second and vanishing from the pattern -- and
	// `a\\\\*` in ksh93 and zsh. The same four characters written *literally*,
	// `'a\\\\b'*`, match the two-backslash name in every column, so the two
	// provenances are told apart and one alphabet cannot carry both.
	//
	// **Asked only where the three encodings put different fields on the
	// wire**, and only in a dialect that globs the result of an expansion at
	// all. valueBackslashReadingsDiffer computes that rather than describing
	// it: a value with a backslash but nothing live beside it encodes three
	// ways and restores one text, which is why `v='a\\b'; echo $v` demands no
	// dialect. GlobExpansionResults is *read* rather than asked for the same
	// reason -- where nothing is globbed the three readings agree -- and that
	// is not the shell without it having no answer: `${~spec}` globs one
	// expansion there and has the third reading (#1367, #1370).
	ValueBackslashInAPattern ValueBackslashPolicy

	// GlobNoMatchIsError makes a pattern matching nothing an error instead of
	// passing it through. True only in zsh.
	GlobNoMatchIsError Answer

	// AssignmentPrefixPersistsOnSpecialBuiltin keeps `x=1 shift` set
	// afterwards. POSIX requires it; dash and ksh93 comply and bash and zsh
	// do not.
	AssignmentPrefixPersistsOnSpecialBuiltin Answer

	// PrefixToARegularBuiltinIsRefused applies the readonly refusal to an
	// assignment written in front of a *regular builtin* — `readonly x=1;
	// x=2 true`.
	//
	// No in ksh93 alone, where that line says nothing at all, runs the
	// builtin and reports 0 — and so do `x=2 echo E`, an alias naming a
	// regular builtin, and `command` naming one. Yes everywhere else.
	// Measured 2026-09-11 from a script file.
	//
	// Asked only in front of a regular builtin, which is the only position
	// the panel parts at: an external command, a special builtin and a
	// function are all refused in all six columns. See #1219.
	PrefixToARegularBuiltinIsRefused Answer
	// PrefixRefusalFatality is what a *reported* refusal of an assignment
	// prefix costs the script. Four answers, two of them keyed on the kind of
	// command the prefix stood in front of and keyed on different lines. See
	// PrefixRefusalFatalityPolicy.
	PrefixRefusalFatality PrefixRefusalFatalityPolicy
	// PrefixRefusalCostsTheCommand leaves the command the prefix stood in
	// front of unrun, at status 1. Yes in ksh93 and zsh — `readonly x=1;
	// x=2 /bin/echo RAN; echo after` prints the complaint and `after` and
	// never `RAN` — and No in bash, which reports the refusal, runs the
	// command with the name still holding its old value, and reports 0.
	//
	// Asked only where the refusal is reported and is not fatal, which is
	// the only place the two readings differ: a script that ends has not run
	// the command either.
	//
	// A separate axis from the fatality because the two are independent in
	// both directions across the panel — ksh93 is fatal on a function and
	// not on an external and skips the command in both non-fatal positions,
	// where bash is fatal nowhere and skips nothing. See #1219.
	PrefixRefusalCostsTheCommand Answer

	// PrefixToAFrozenNameIsCheckedFirst refuses the prefix **before** the
	// command's values are expanded and before its redirections are opened.
	// `No` does the other things first, so a value that will not expand and
	// a file that will not open each report on their own and the frozen name
	// is never mentioned.
	//
	// Measured 2026-09-07 and again 2026-09-12, script file, `readonly x=1`
	// in front of each line:
	//
	//	                              bash 5.3, as-`sh`, 3.2   dash, ksh93u+, zsh
	//	x=$((1/0)) /bin/echo RAN      `x: readonly variable`,   the division, and
	//	                              `RAN`, and no division    no `RAN`
	//	x=$((1/0)) f                  the same, and the body    the division, and
	//	                              runs                      no body
	//	x=2 /bin/echo RAN >/nope/f    `x: readonly variable`,   the file only
	//	                              then the file
	//
	// Two probes agreeing on one boundary is what makes it a boundary rather
	// than a quirk of arithmetic: a redirection that cannot be opened reaches
	// the same order with no expression in it at all. The first row also says
	// the value is not merely reported later but **never evaluated** — the
	// division is silent in the column that checks first.
	//
	// The other three prefix axes are asked at the same point and for the
	// same reason: a command with a prefix is a minority of a script's lines
	// and one with a frozen name in the prefix is a minority of those, so
	// four questions sit off the common path entirely. This one is asked
	// there too, once a name in the prefix is actually frozen (#1943).
	//
	// It is the order and not the refusal. What a reported refusal costs is
	// PrefixRefusalCostsTheCommand and PrefixRefusalFatality, and those are
	// answered the same way whichever order the two happen in — bash runs
	// the command in both rows above, exactly as it does with a prefix that
	// expands.
	PrefixToAFrozenNameIsCheckedFirst Answer

	// EchoOptions is the set of letters `echo` reads as options: `n` for
	// every shell measured, `e` everywhere but dash, `E` in bash and zsh
	// alone. A word carrying any other letter is not an option at all — the
	// whole word becomes an operand, which is unanimous and is why `echo
	// -nq hi` prints `-nq hi` in all four. Empty means `n`.
	EchoOptions string
	// EchoLastEscapeFlagWins decides `echo -e -E`: bash lets the last flag
	// win and prints the backslashes, zsh lets -e win whatever the order.
	// Reached only when -e came first — the other order agrees everywhere —
	// and only in a dialect whose EchoOptions has both letters.
	EchoLastEscapeFlagWins Answer
	// EchoExpandsHexEscapes admits `\xHH` alongside the XSI set: bash and
	// zsh do, dash and ksh93 print it as written.
	EchoExpandsHexEscapes Answer
	// EchoExpandsEscEscape admits `\e` for the escape character in an `echo`
	// argument: bash 5.3 and zsh do, dash bash 3.2 and ksh93 write the two
	// characters.
	//
	// It is a separate axis from EchoExpandsCapitalEscEscape below because
	// the two shells that split the letters split them in *opposite*
	// directions, so no single answer describes either one — ksh93 has `\E`
	// and not `\e`, zsh has `\e` and not `\E` (#908). It is the same
	// asymmetry the `%b` site has, and it is asked separately there: see
	// PrintfBEscEscape.
	//
	// Asked only where an `echo` argument actually carries a `\e`.
	EchoExpandsEscEscape Answer
	// EchoExpandsCapitalEscEscape admits `\E` in an `echo` argument: bash 5.3
	// and ksh93 do, dash bash 3.2 and zsh write the two characters. See
	// EchoExpandsEscEscape for why the two letters are two questions.
	//
	// Asked only where an `echo` argument actually carries a `\E`.
	EchoExpandsCapitalEscEscape Answer
	// EchoExpandsUnicodeEscapes admits `\uHHHH` and `\UHHHHHHHH` in an `echo`
	// argument, each read as a code point and written in UTF-8: bash 5.3 and
	// zsh do, bash 3.2, that binary as `sh`, dash and ksh93 write the
	// characters as they stand.
	//
	// One axis for both letters, unlike `\e` and `\E` above, because the
	// panel does not split them: measured 2026-09-10, every shell that reads
	// one reads the other, and with the same rules — at most four hex digits
	// after `\u` and at most eight after `\U`, fewer accepted (`\u41` is `A`
	// in both), and the value written as UTF-8 rather than as a byte, so
	// `\u00e9` is two bytes and `\u20ac` three.
	//
	// It is the *original* UTF-8 and not the range it was later narrowed to,
	// which is measured rather than assumed: a surrogate and a value past the
	// last code point are encoded rather than replaced — `\ud800` is three
	// bytes and `\U110000` four — and the five- and six-byte forms are
	// reachable, `\U200000` being five and `\U4000000` six, in both shells
	// that have the escape.
	//
	// Asked only where an `echo` argument actually carries one.
	EchoExpandsUnicodeEscapes Answer
	// UnicodeEscapeOutsideTheLocale is what becomes of a `\u` or `\U` escape
	// naming a code point the locale's encoding cannot hold — see
	// OutsideLocaleEscapePolicy, and interp/localeescape.go for the
	// measurements and for what "cannot hold" is read off.
	//
	// One axis for every site that reads the escape — `echo`, `print`, a
	// `printf` format, a `%b` argument, `$'...'`, and the `(g)` and `(p)`
	// expansion flags — because each shell answers the same at every site it
	// reads the escape at, measured one site at a time. It is a question
	// *after* the escape has been read, so it is separate from
	// EchoExpandsUnicodeEscapes above and from the printf policies: a dialect
	// that does not read the escape at a site never reaches it there, which is
	// why ksh93's answer is reachable at two sites and not at five.
	//
	// `$'...'` is a **core** construct, and the axis is three-valued because
	// of it: ksh93 has no `\u` in `echo`, in `print` or in a `%b`, and writes
	// the character regardless of the locale in the two places it does read
	// one. So a core script with `$'\u00e9'` in it under a non-UTF-8 locale
	// is an unanswered axis rather than a value — which is the core refusing
	// what the panel disagrees about, in the one place that disagreement
	// reaches the common denominator (#2021).
	//
	// Asked only where such an escape actually names a code point the locale
	// refuses, so an ASCII one needs no answer from anybody and neither does
	// any escape at all in a UTF-8 locale.
	UnicodeEscapeOutsideTheLocale OutsideLocaleEscapePolicy
	// EchoEmptyHexDigitRunIsNul reads a hexadecimal escape with no digit
	// after it as a zero rather than leaving it as written: `echo '\xZ'`,
	// `echo '\uZ'` and `echo '\x'` are a NUL byte followed by what was
	// there in zsh, and the two characters as written in bash 5.3.
	//
	// One question for `\x`, `\u` and `\U` together, because the shell that
	// answers it answers the same for all three and the shells that leave one
	// as written leave all of them. It is the same split
	// PrintfHexEscapePolicy records at the two `printf` sites, where it is one
	// of the three details that made a policy out of a bool.
	//
	// Asked only where such an escape actually runs out of digits, so
	// `echo '\x41'` needs no answer to it.
	EchoEmptyHexDigitRunIsNul Answer
	// EchoInterpretsEscapes expands backslash escapes in `echo` without -e.
	// True in dash and zsh, false in bash and ksh93 — a grouping no other
	// axis produces.
	EchoInterpretsEscapes Answer

	// DollarSingleBackslashC is what `\c` means inside `$'…'`, and like the
	// `\c` of a printf format it is three different things rather than a
	// switch — see DollarSingleControlPolicy. Asked only for a `$'…'` that
	// has a `\c` in it.
	DollarSingleBackslashC DollarSingleControlPolicy
	// DollarSingleUnknownEscape is what becomes of a backslash before a
	// character no escape claims — `$'\q'` — see DollarSingleUnknownPolicy.
	// Asked only when such an escape is actually there.
	DollarSingleUnknownEscape DollarSingleUnknownPolicy
	// DollarSingleNulTruncates ends the decoded text at the first NUL an
	// escape produces, which is C-string semantics: `$'a\0b'` is `a` in
	// bash and ksh93 and the three bytes `a`, NUL, `b` in zsh.
	//
	// The truncation is the *span's*, not the word's: `$'a\0b'ccc` is `accc`
	// in the shells that truncate, so what is lost is the remainder of the
	// quoted text and nothing else. Reached only where a decoded escape
	// actually yields a zero byte — `\0`, an octal or hex escape that comes
	// to zero, and `\c@`, which is the same zero by another road.
	DollarSingleNulTruncates Answer
	// DollarSingleCaretMeta reads `\C-X` inside `$'…'` as a control
	// character and `\M-X` as the same byte with the high bit set. The
	// separating `-` is optional in both, so `\CA` and `\C-A` are one byte
	// apiece, and either may take the other as its argument.
	//
	// Yes in zsh alone. Measured 2026-09-08 from `$'\C-A'`, `$'\M-x'`,
	// `$'\M-\C-?'` and `$'\cA'` in each shell:
	//
	//	bash 5.3, bash 3.2, bash as sh   \C-A and \M-x kept as written,
	//	                                 and `\cA` is the control escape
	//	dash                             no `$'…'` at all
	//	ksh93                            `\C` is a control escape of its own,
	//	                                 spelled without the dash, so
	//	                                 `$'\C-A'` is control-`-` then `A`,
	//	                                 and `\M-x` is ESC then `x`
	//	zsh                              01, f8, ff, and `\c` is nothing
	//
	// So it is No in bash and unspecified in ksh93, whose two escapes are a
	// different reading rather than this one turned off — and refusing it
	// there is the point: `$'\C-A'` answered as `C-A` would be off by a
	// byte and silent about it.
	//
	// Asked only for a `$'…'` that has a `\C` or an `\M` in it. This is the
	// reading side of what the `q+` expansion flag writes, and the two are
	// the same table seen from its two ends: a `q+` whose spelling the shell
	// cannot read back is not a quoting flag at all.
	DollarSingleCaretMeta Answer

	// ReadOptions is the set of letters `read` takes, a `:` after a letter
	// marking one whose argument follows it — the getopts convention, the
	// same one the shared option reader speaks. The letters are the
	// dialect's own: bash spells the array option `-a` and takes the array's
	// name as the option's argument, ksh93 and zsh spell it `-A` and take
	// the name as the first operand, and zsh reads `-n` as a flag where bash
	// and ksh93 read a count after it. `-p` splits the same way: `p:` takes
	// a prompt for the terminal in bash and dash, a bare `p` names the
	// coprocess as the source in ksh93 and zsh. Empty means `r`, the one
	// letter POSIX gives the builtin.
	ReadOptions string
	// UnsetOptions is the same question asked of `unset`, spelled the same
	// way. The letters split three ways and no two dialects have the same
	// set: `-v` and `-f` are unanimous, `-n` is bash 5.3's and ksh93's — and
	// is refused by bash 3.2, dash and zsh — and `-m`, which reads its
	// operands as *patterns* and unsets every parameter whose name matches
	// one, is zsh's alone. Measured 2026-09-05 across the panel. Empty means
	// `vf`, which is what POSIX gives the builtin.
	UnsetOptions string
	// ReadZeroTimeout is what `read -t 0` asks of the stream — a poll, a
	// read of what is already waiting, or a read that commits once it has
	// begun. Asked only where `-t 0` is actually written; every other
	// timeout is a deadline and needs no answer. See ReadZeroTimeoutStyle
	// for the measurements.
	ReadZeroTimeout ReadZeroTimeoutStyle
	// ReadPartialCountSucceeds decides `read -n N` when the input ends
	// after some but fewer than N characters: ksh93 calls the read a
	// success and bash reports 1, both keeping what arrived. Asked only
	// there — a full count, a delimiter, or a wholly empty input answers
	// the same way everywhere.
	ReadPartialCountSucceeds Answer
	// ReadExactCountKeepsPartial decides what `read -N N` leaves behind
	// when the input ends short: bash assigns the partial text and ksh93
	// assigns nothing, both reporting 1. Asked only on that partial text.
	ReadExactCountKeepsPartial Answer
	// ReadTimeoutKeepsWhatArrived decides what an expired `read -t` leaves
	// behind: bash assigns whatever had arrived before the deadline and
	// ksh93 and zsh touch no name at all, leaving the variable's earlier
	// value. Asked only on the timeout, never at end of input, where all
	// three assign.
	//
	// The distinction the wording is careful about is that bash does not
	// *clear* the variable — it assigns a short read, and clearing is only
	// what that looks like when nothing had arrived. Measured with a stream
	// that delivers half a line and then stalls:
	//
	//	{ printf part; sleep 0.5; printf 'ial\n'; } |
	//	  sh -c 'v=old; read -t 0.2 v; echo "$? [$v]"'
	//
	//	bash 5.3  142 [part]      ksh93  1 [old]      zsh  0 [partial]
	//
	// zsh's row is not this axis and is why the axis is worded around the
	// timeout rather than around the partial text: its `-t` bounds the wait
	// for the stream to become readable and nothing after that, so once a
	// byte has arrived it reads the line to the end however long that takes
	// and reports success. A zsh timeout therefore only ever happens with
	// nothing to assign, which is the same observable as leaving the name
	// alone; the two are told apart by how long the read takes, not by what
	// it assigns.
	ReadTimeoutKeepsWhatArrived Answer

	// BuiltinWriteErrorFailsTheCommand makes a builtin whose output write
	// failed — into a descriptor closed with `>&-`, most plainly — report
	// status 1. True in bash, dash and ksh93; zsh keeps the builtin's own
	// status and quietly loses the text.
	//
	// Whether anything is *said* about it is the dialect's wording —
	// Diagnostics.BuiltinWriteError — not a second axis: bash and dash
	// complain, ksh93 fails silently, and zsh has nothing to word because it
	// does not fail. Asked only when a write has actually failed, so `echo
	// hi` on an open stream needs no dialect.
	BuiltinWriteErrorFailsTheCommand Answer

	// LengthOfSpecialIsCount makes `${#@}` the number of positional
	// parameters. False in dash, which gives the length of the joined
	// string. The first axis measured where dash stands alone, and a silent
	// one: both answers are plausible numbers.
	LengthOfSpecialIsCount Answer

	// TransformLetterCheckedOnlyWhenValued delays the check of a `@`
	// operator's letter until the name has a value. Yes makes `${u@QQ}` on
	// an unset name empty at status 0 while the identical spelling on a set
	// one is a bad substitution — the same word meaning two different things
	// depending on what a variable happens to hold.
	//
	// Reached only by a grammar that *has* the family, which is one shell;
	// to the rest `${u@QQ}` is an unknown operator whatever the value, and
	// nothing here is asked. So this is an axis with one measured answer,
	// deliberately: making an operator's validity depend on a value is not a
	// rule anything should inherit by having a `@` family, and the shell
	// that does it should have to say so. An empty array counts as no value,
	// measured — `a=(); ${a[@]@Z}` is quiet and `a=(x); ${a[@]@Z}` is not.
	//
	// unexhibited No: nobody in the panel, and kept deliberately — it is
	// the null hypothesis this axis exists to make the odd shell argue
	// against. Measured 2026-09-12: `${s@Q}` is a bad substitution in
	// dash, bash 3.2.57, ksh93u+ and zsh 5.9.2, so bash 5.x is the only
	// column whose grammar reaches the question, and it answers Yes. `No`
	// is what a shell that checked the letter whatever the value would
	// hold, which is what having a `@` family would otherwise imply
	// (#2060).
	TransformLetterCheckedOnlyWhenValued Answer

	// ArithLeadingZeroIsOctal reads `0100` as sixty-four. False in zsh, where
	// it is one hundred. The quietest divergence measured — nothing warns,
	// both are plausible numbers, and file modes are written this way.
	ArithLeadingZeroIsOctal Answer
	// HeredocExpandsInTheCommandsProcess confines what a here-document body's
	// expansion writes to the command the body feeds, where that command is
	// one the shell runs as a process of its own.
	//
	// True in bash, ksh93 and zsh; dash alone lets the write escape.
	// Measured with `unset u` and a body of `${u:=zz}` fed to `cat`: `u` is
	// unset afterwards in the three, and holds `zz` in dash. With `$(( n++ ))`
	// instead — which dash has not got — the three are unanimous again.
	//
	// It is one axis rather than one per construct because the split is a
	// property of *where the shell expands a body*, and every construct
	// downstream of that follows: a builtin, a function, a compound command,
	// `exec`, `eval` and `.` all leave the write behind in all four shells,
	// because the shell runs them itself and there is no other process for
	// it to land in. Nothing is asked for those.
	//
	// Silent either way, which is the reason it is here: a counter advanced
	// inside a template's here-document reads one too high on the next line
	// under the wrong answer, and nothing about the output says so.
	//
	// Asked only when the body actually wrote something. A here-document
	// with no side effect is every other here-document and the shells agree
	// about it, so an unanswered dialect must still be able to run one.
	HeredocExpandsInTheCommandsProcess Answer

	// RedirectTargetExpandsInTheCommandsProcess is the same claim for a
	// redirection's *target*: `> "${u:=made}"` on a command the shell runs
	// as a process of its own leaves `u` unset afterwards, and `> "$NOPE"`
	// under `set -u` costs that command rather than the script.
	//
	// True in bash, ksh93 and zsh; dash alone keeps the write and stops the
	// script. One answer for both consequences, because they are one fact
	// about where the word was expanded — and a second axis rather than a
	// widening of the body's, because dash splits the other way there: a
	// here-document body's failed expansion costs dash the command and not
	// the script, where a target's ends it (#1228).
	//
	// Asked only where the command is one the shell runs as a process of
	// its own *and* the expansion either wrote something or failed. A target
	// that expands to a name is every other redirection, the shells agree
	// about it, and an unanswered dialect must still be able to open a file.
	//
	// What is not asked anywhere is whether the open happens: a target whose
	// expansion failed is not opened, unanimously. Diagnosing the unset name
	// and then reporting that `` could not be created is two complaints for
	// one mistake, and the second names a file nobody wrote.
	RedirectTargetExpandsInTheCommandsProcess Answer
	// ForNameWhenTheLoopRuns is what a `for` or `select` does when it is
	// reached and the word standing where its variable belongs is not a
	// name. Asked only where the grammar carried the word this far —
	// syntax.Dialect.ForNameCheckedWhenTheLoopRuns — which is bash and
	// ksh93; the other four refuse it while parsing and build no clause to
	// run. Three answers among those two, and POSIX mode is the third; see
	// ForNameRunForm (#1110).
	//
	// unexhibited ForNameEndsTheScriptAsASyntaxError: bash in POSIX mode,
	// which is a panel column with no dialect preset — the reading is
	// reached through [Runner.SetPosixMode] rather than by any vector,
	// exactly as the grid above records. A mode a shell enters and leaves
	// is not a fifth dialect, so no preset can exhibit it and the sweep's
	// list cannot shrink here (#2060).
	ForNameWhenTheLoopRuns ForNameRunForm

	// FunctionNameWhenTheDefinitionRuns is what a `function` definition does
	// when it is reached and the word standing where its name belongs is not
	// a name. Asked only where the grammar carried the word this far —
	// syntax.Dialect.FunctionNameCheckedWhenTheDefinitionRuns — which is bash
	// and ksh93; zsh reads such a name as a word and defines what it comes
	// to, and dash has no keyword to reach the question with. Three answers
	// among those two, and POSIX mode is the third; see FuncNameRunForm
	// (#1296).
	//
	// unexhibited FuncNameEndsTheScriptAsASyntaxError: bash in POSIX mode,
	// reached through [Runner.SetPosixMode] and held by no preset, for the
	// reason ForNameWhenTheLoopRuns records (#2060).
	FunctionNameWhenTheDefinitionRuns FuncNameRunForm

	// FatalErrorStatusIsOne is the status a fatal shell error carries.
	// True in bash, ksh93 and zsh; dash alone exits 2.
	//
	// It began as an arithmetic-only axis and was generalized on evidence:
	// a failed arithmetic expansion, a readonly reassignment and a `shift`
	// past the end are three unrelated errors, and every shell gives all
	// three the same status. The split is a property of the shell, not of
	// the error, so it is one axis rather than three.
	//
	// *Which* errors are fatal is a separate question and stays per-error —
	// ReadonlyReassignmentFatal and ShiftPastEndFatal answer it, and the
	// shells genuinely disagree there. A silent axis either way: scripts
	// that branch on `$?` rather than on truthiness read the failure
	// correctly under one group and misread it under the other.
	FatalErrorStatusIsOne Answer
	// ArithNameValueRecurses re-evaluates a name-shaped value as an
	// expression: with `y=5; x=y`, `$((x+1))` is 6 because `y` is looked up
	// in turn, and it recurses as far as the values lead — `y=z; z=7; x=y`
	// is 8. Yes in bash, zsh and ksh93; **dash alone** reads the value as a
	// literal and refuses it, `Illegal number: y`.
	//
	// The interpreter once followed the shells that agree and said so in a
	// comment, which is the shape of a guess rather than a measurement; it
	// asks here now, and arithValueOf is where the ask is made.
	//
	// This comment said "dash and ksh93 error instead" until #1629, and the
	// preset it contradicted was the right one: measured 2026-09-11, ksh93
	// recurses exactly as bash and zsh do. What it does differently is one
	// step further in — see ArithRecursedNameMustBeSet, which is the axis
	// that mistake was really about.
	ArithNameValueRecurses Answer
	// ArithRecursedNameMustBeSet makes an unset name *reached through another
	// name's value* an error rather than a zero. Asked only where
	// ArithNameValueRecurses says the lookup happens at all, and only below
	// the top: a name written in the expression itself is zero when it is
	// unset in every shell in the panel, `$((nosuch+1))` being 1 everywhere.
	//
	// Measured 2026-09-11, `x=abc` with `abc` unset:
	//
	//	bash 5.3.15   $((x+1)) is 1, the script runs on
	//	bash 3.2.57   $((x+1)) is 1, the script runs on
	//	zsh 5.9.2     $((x+1)) is 1, the script runs on
	//	ksh93u+       abc: parameter not set, status 1, the script stops
	//	dash          never gets here: it does not recurse
	//
	// The refusal is the shell's `set -u` sentence word for word, with
	// nounset off — ksh93 reads a name arrived at this way as a *parameter
	// reference* rather than as text that might be a number. It is fatal the
	// way an unset parameter under nounset is: `||` does not catch it, and a
	// subshell dies alone.
	//
	// Silent when it is wrong, which is why it is worth an axis rather than
	// a wording: `x=abc; $((x+1))` answered 1 and carried on, so a ksh script
	// whose variable held a stale name got a plausible number where the real
	// shell had stopped.
	ArithRecursedNameMustBeSet Answer
	// ArithSubscriptSkippedWhenNameUnset looks the name up before it reads
	// the brackets, and answers zero for a name that is not there without
	// evaluating the subscript at all. Yes in zsh alone: measured 2026-09-10,
	// `$(( nodecl[1/0] ))` is a quiet 0 there and a division by zero in bash
	// 5.3, bash 3.2 and ksh93, and `i=0; $(( nodecl[i++] ))` leaves i at 0 in
	// zsh and at 1 in the other three.
	//
	// Not a rule about *empty* subscripts, though it is what answers one:
	// `$(( m[$w] ))` with `$w` empty reaches the expression as the literal
	// `m[]`, and where the name has never been set the brackets are never
	// looked at, so the operand is the plain unset 0 that `$(( nosuchvar ))`
	// is. Modeling that as a special case for the empty subscript would have
	// been a rule the probe for it could not tell from this one — the two
	// agree on every empty-subscript row and part only on a subscript that
	// errors or assigns.
	//
	// What "not there" means is set-ness and not emptiness: `e=` then
	// `$(( e[1/0] ))` divides by zero in zsh too, and so does an array
	// declared with nothing in it.
	//
	// No answer reads as No, which is the majority and the harmless side: a
	// subscript that has no error and no side effect gives the same zero
	// either way, so an unanswered preset is not refused over `$(( a[0] ))`.
	//
	// unexhibited No: bash 5.3.15, bash-as-`sh`, bash 3.2.57 and ksh93u+,
	// re-measured 2026-09-12: `unset nodecl; $(( nodecl[1/0] ))` is a
	// division by zero in all four and a quiet 0 in zsh 5.9.2, and `i=0;
	// $(( nodecl2[i++] ))` leaves i at 1 in the four and 0 in zsh. dash
	// has no subscript in arithmetic, so the question does not arise
	// there. No preset writes it because the axis is read (`== Yes`) and
	// not asked — silence and `No` reach the same code, so writing it down
	// would add a line and no fact (#2060).
	ArithSubscriptSkippedWhenNameUnset Answer
	// ArithInvalidOctalDigitIsError rejects `08` once a leading zero has
	// been read as octal. True in dash and bash; ksh93 falls back to decimal
	// and yields 8.
	//
	// This is the second axis the vector could not express with one field.
	// `ArithLeadingZeroIsOctal` was doing two jobs: `0100` is 64 in dash,
	// bash and ksh93 and 100 in zsh, so ksh93 *is* octal — but it is octal
	// and tolerant, and one boolean cannot say that. The `${!x}` note in
	// semantics.md records the same failure mode; this is it happening a
	// second time, which makes it a limit of the model rather than a quirk.
	//
	// zsh never reaches this: nothing there made the zero octal.
	ArithInvalidOctalDigitIsError Answer
	// IntegerAssignmentReadsALeadingZeroAsDecimal makes `typeset -i d=010`
	// ten rather than eight, in a shell whose *arithmetic* still reads
	// `$((010))` as eight.
	//
	// Measured 2026-09-10 from a script file under `env -i`:
	//
	//	                        $((010))   typeset -i d=010   e=010; typeset -i e
	//	bash 5.3.15, bash 3.2      8              8                  010
	//	ksh93u+                    8             10                   10
	//	zsh 5.9.2                 10             10                   10
	//
	// The middle row is the whole of the question: one shell has two
	// readers, and the one an *assignment* to an integer name goes through
	// does not apply the octal rule the expression reader does. The other
	// two rows agree with themselves for two different reasons — bash
	// applies octal in both, and zsh has no octal-by-leading-zero at all —
	// which is why this is a field and not a rule. bash's third column is a
	// third fact and not this axis: it never re-reads a standing value, and
	// AttributeRereadsTheValueItFinds is where that lives.
	//
	// Asked only where the text is a signed digit string with a leading
	// zero in front of another digit, and only where
	// ArithLeadingZeroIsOctal is Yes. Outside that shape the two readers
	// agree — `$((010+1))` and `typeset -i d=010+1` are both nine in the
	// shell that splits — and where nothing made the zero octal there is
	// nothing to choose between. See Runner.zeroPaddedInteger for the
	// measured edges.
	//
	// Silent and arithmetically wrong either way it is answered wrongly: a
	// zero-padded date field or counter comes out eight where the shell
	// says ten, with nothing said about it.
	IntegerAssignmentReadsALeadingZeroAsDecimal Answer
	// ArithStoredValueReadsALeadingZeroAsDecimal reads `010` out of a
	// *variable* as ten inside an expression, in a shell whose lexer still
	// reads the identical literal as eight.
	//
	// The other half of the split IntegerAssignmentReadsALeadingZeroAsDecimal
	// records, at the other reader. That one is the assignment to an integer
	// name; this one is `$(( k ))`, with no attribute anywhere in it.
	//
	// Measured 2026-09-11, `k=010; echo "$((k)) $((010))"`:
	//
	//	                    a name holding 010   the literal 010
	//	dash                        8                   8
	//	bash 5.3.15, bash 3.2       8                   8
	//	ksh93u+                    10                   8
	//	zsh 5.9.2                  10                  10
	//
	// ksh93 is the only column whose two answers differ: its arithmetic lexer
	// reads a leading zero as octal, and a value dereferenced into the
	// expression does not go through that lexer. zsh's two agree because
	// nothing there makes a leading zero octal at all, and the rest because
	// both readers are octal — which is why this is asked only where
	// ArithLeadingZeroIsOctal is Yes, and never of zsh.
	//
	// It is the *leading numeral* of the value and not the whole of it, which
	// is what tells this apart from a value read as a number: `k=010+1` is 11
	// on the shell that splits, where the same literal is 9, so the ten is
	// read decimal and the rest of the expression stays octal. See
	// Runner.decimalLeadingNumeral for the measured edges — a sign, leading
	// space, and an `0x` prefix each leave the value alone.
	//
	// Silent and arithmetically wrong: a zero-padded field read out of a
	// variable — a month, a padded counter — comes out short with nothing
	// said, and under the octal reading `09` is an invalid digit as well as
	// a wrong number.
	ArithStoredValueReadsALeadingZeroAsDecimal Answer
	// LetReadsALeadingZeroAsDecimal gives the `let` builtin a reader of its
	// own, in which `010` is ten and not eight.
	//
	// ksh93 alone, and it is a third site rather than either of the two
	// above. Measured 2026-09-12:
	//
	//	                    let "x=010"  (( y=010 ))  let "x=1+010"  k=1+010; $((k))
	//	bash 5.3.15              8            8             9               9
	//	ksh93u+                 10            8            11               9
	//	zsh 5.9.2               10           10            11              11
	//
	// The third and fourth columns are what make it a site and not a reuse
	// of ArithStoredValueReadsALeadingZeroAsDecimal: that one rewrites the
	// *leading* numeral of a value and leaves the rest octal, so `k=1+010`
	// is 9 there — where `let "x=1+010"` is 11, every numeral in the word
	// having been read in decimal. So `let`'s words are evaluated with the
	// octal rule off rather than with one numeral rewritten.
	//
	// Answered No where nothing makes a leading zero octal in the first
	// place, which is zsh: the two readings coincide there and the column
	// above says so.
	//
	// Silent and arithmetically wrong, the way its two neighbors are:
	// `let "n=010"` is a plausible number, eight where the shell says ten.
	LetReadsALeadingZeroAsDecimal Answer
	// ArithmeticAssignmentDeclaresAnInteger gives a name assigned inside an
	// arithmetic context the integer attribute, which outlives the
	// expression. zsh alone.
	//
	// Measured 2026-09-12 against `typeset -p`, with bash as the control:
	//
	//	                                  zsh 5.9.2         bash 5.3.15   ksh93u+
	//	(( x = 5 ))                       typeset -i x=5    declare -- x  x=5
	//	(( x = 5 )); x=7                  typeset -i x=7    declare -- x  x=7
	//	(( y = 0x1f ))                    typeset -i16 y=31 declare -- y  y=31
	//	for (( i=0; i<2; i++ )); do :; done  typeset -i i=2 declare -- i  i=2
	//	let "z = 3"                       typeset -i z=3    declare -- z  z=3
	//
	// It is not only a listing difference, which is the reason it is an axis
	// rather than a note: the attribute changes what a *later* assignment
	// means. `(( x = 5 )); x=2+3` is 5 in zsh and the three characters
	// `2+3` everywhere else, and with the attribute comes the output base,
	// so a name that learned 16 renders a later plain `5` as `16#5`.
	//
	// Every construct that assigns inside arithmetic is the same answer —
	// `(( ))`, `let` and a C-style `for` header alike — so it is asked where
	// the assignment operator is applied rather than at each of them.
	ArithmeticAssignmentDeclaresAnInteger Answer
	// IndirectionYieldsName makes `${!x}` the *name* rather than the value it
	// names: with `x=y`, ksh93 gives `x` and bash gives the value of `y`.
	//
	// Only reachable where the grammar parses `${!x}` at all, which is bash
	// and ksh93 — dash and zsh reject it. That is the point: a three-way
	// divergence became a grammar flag plus a binary axis, and neither half
	// needed a third state. `semantics.md` records `${!x}` as the axis the
	// binary table could not express; this is the shape that expresses it.
	IndirectionYieldsName Answer
	// BraceExpansion expands `{a,b}` and `{1..3}`. Absent from dash, where
	// the word is a literal.
	//
	// It lives here rather than in syntax.Dialect even though it is
	// additive, because the token stream is identical either way: the
	// parser produces the same word, and only expansion differs. It is also
	// silent in the `&>` sense — `echo {1..3}` prints something either way,
	// and nothing reports that one of them is not what was meant.
	BraceExpansion Answer
	// BraceRangePadsToEndpointWidth keeps the leading zeros of a range
	// endpoint and pads every element to the widest endpoint, zeros after
	// the sign: `{01..3}` is `01 02 03` and `{-03..3..3}` is `-03 000 003`.
	// True in bash and zsh; ksh93 strips the padding and prints `1 2 3` and
	// `-3 0 3`. Asked only when an endpoint is written with leading zeros,
	// and only in a dialect whose braces expand at all — dash never reaches
	// it.
	BraceRangePadsToEndpointWidth Answer
	// BraceRangeStepSignHonored takes a written step's sign at its word:
	// the walk leaves the first endpoint in the direction the sign says, so
	// a sign pointing away from the far endpoint ends the range after one
	// element. ksh93's `{10..1..3}` is `10`, its `{1..10..-3}` is `1`, and
	// letters answer the same way — `{a..e..-1}` is `a`. False in bash and
	// zsh, where the endpoints decide the direction and the step
	// contributes magnitude alone. Asked only when the sign and the
	// endpoints disagree.
	BraceRangeStepSignHonored Answer
	// BraceRangeNegativeStepReverses hands a negative step's sign to the
	// order of the result rather than to the walk: the range is walked
	// endpoint to endpoint and then reversed, so zsh's `{3..1..-1}` is
	// `1 2 3` and its `{1..10..-4}` is `9 5 1` — bash's `1 5 9` backwards,
	// not the `10 6 2` that swapping the endpoints would give. True in
	// zsh; false in bash, whose `{3..1..-1}` stays `3 2 1`, and in ksh93,
	// which reaches the question only when the sign agrees with the
	// endpoints and then keeps their order too. Asked only for a written
	// negative step whose sign was not already honored.
	BraceRangeNegativeStepReverses Answer
	// BraceRangeEndpointsExpanded reads a range's endpoints *after* the
	// expansions written in them, rather than before: with `n=3`,
	// `echo {1..$n}` is `1 2 3` in zsh and ksh93 and the literal `{1..3}` in
	// bash, bash 3.2 and bash-as-sh, where brace expansion has finished
	// before `$n` exists. Quoting hides an endpoint from the brace scanner
	// and not from the range, so `{1..'3'}` counts the same way.
	//
	// This is the ordering axis the vector had no field for, and the comment
	// in `interp/brace.go` asserted flatly that no shell could do it — a
	// claim two of the six panel columns disprove (#1679). It is asked only
	// where a range is written with something to expand in it: a literal
	// `{1..3}` is unanimous and must not be turned into a question.
	//
	// What a failed range leaves is not a second axis. Both shells that
	// expand endpoints run those expansions once and put the text back
	// unsplit and unmatched — ksh93 splits `$sp` in a word of its own and
	// leaves `{1..$sp}` a single field — so the ordering decides that too.
	BraceRangeEndpointsExpanded Answer
	// EqualsExpansion replaces an unquoted word beginning with `=` by the
	// path of the command named after it: `echo =ls` prints /bin/ls. zsh
	// alone, and silent in the `&>` sense — the other three take the word
	// literally and report nothing, so the same script prints two different
	// things and neither shell complains.
	//
	// Its failure is not silent: a name that resolves to nothing is fatal to
	// the script, like any other failed expansion.
	EqualsExpansion Answer
	// UnterminatedBracket is what `[` without a closing `]` means in a
	// pattern, and it is the axis that does not fit Answer.
	//
	//	case "[" in [) hit;; *) miss;; esac
	//	bash  → hit          a literal `[`
	//	ksh93 → hit          a literal `[`
	//	dash  → miss         a class that can never match
	//	zsh   → bad pattern  an error
	//
	// Three answers, and it is load-bearing rather than exotic: `[` is the
	// name of the test builtin.
	//
	// It gets its own type rather than a wider Answer. The prediction in
	// semantics.md was that Answer would have to grow a third state; writing
	// it showed that would be worse, because every other axis is genuinely
	// binary and a wider Answer would let `BracketBadPattern` be assigned to
	// any of them and still compile. An axis with three answers gets a type
	// with three values; the binary ones keep the type that says so.
	UnterminatedBracket BracketPolicy
	// TraceAssignmentsSeparately gives each assignment of `a=1 b=2` its own
	// trace line. True in bash and ksh93; dash and zsh put them on one.
	TraceAssignmentsSeparately Answer
	// TraceShowsItsOwnDisabling prints `set +x` before acting on it. True in
	// dash, bash and zsh; ksh93 applies the change first, so the command
	// that stops tracing leaves no trace of itself.
	TraceShowsItsOwnDisabling Answer
	// UnsetPositionalIsAllowed lets `$1` expand to nothing under `set -u`
	// rather than being an error. ksh93 alone, and quiet where it differs:
	// a script that reads an argument it was not given carries on there and
	// stops everywhere else.
	UnsetPositionalIsAllowed Answer
	// BackgroundJobInput is the standard input a job started with `&` reads,
	// and it is three answers rather than a switch.
	//
	// The probe reads one descriptor twice, so the answers come out in
	// opposite orders and neither can be mistaken for the other:
	//
	//	printf 'DATA\n' > f
	//	<shell> -c '/bin/cat & wait; echo ---; /bin/cat' < f
	//
	// Measured 2026-09-07 it writes `---` and then `DATA` in dash, bash
	// 5.3.15, bash 5.3.15 as `sh`, bash 3.2.57 and ksh93u+ — the job read
	// nothing and the script kept its line — and `DATA` then `---` in zsh
	// 5.9.2, where the job ate it. `ls -l /dev/fd/0` inside the job names
	// what the five handed it: a character device with `/dev/null`'s rdev in
	// the four, and the file itself in zsh. Same answer whether the shell's
	// input is a file or a pipe; `/dev/null` cannot tell the two apart, which
	// is why the probe uses neither.
	//
	// POSIX XCU 2.9.3, Asynchronous Lists, says a background command's
	// standard input "shall be assigned to an empty file or /dev/null" while
	// job control is disabled, so the majority is the specified answer and
	// zsh is the divergence. It is also the direction that *steals*: the job
	// and the script read the same descriptor, so every byte the job consumes
	// is one the script's own `read` never sees —
	//
	//	while read -r line; do process "$line" & done < input.txt
	//
	// silently loses lines, at status 0, with nothing said.
	//
	// The third answer is what a *closed* descriptor does, and it splits the
	// five: `exec 0<&-; /bin/cat & wait` is silent at 0 in dash and bash,
	// which substitute the empty input even there, and
	// `cat: stdin: Bad file descriptor` in ksh93u+, which substitutes only
	// what it can dup and leaves a closed fd 0 closed. zsh says the same as
	// ksh93 there, for the different reason that it never substitutes at all.
	// Four columns against one, and it is the sub-answer rather than a second
	// axis: it is the same decision, asked of an input that is not there.
	//
	// Only while job control is off. That is the condition XCU 2.9.3 states,
	// and it is measured rather than inherited: on a pty,
	// `bash -i -c '/bin/cat & sleep 0.3; jobs'` lists the job `Stopped` and
	// zsh lists it `suspended (tty input)`. Both handed it the *terminal* and
	// let the kernel stop it with SIGTTIN, which an empty input can never
	// produce — so stdin's kind is an axis of the measurement rather than a
	// detail of it, and a shell with someone to tell substitutes nothing.
	//
	// Read without asking, for the reason the field below gives: an
	// unanswered axis here would have to refuse `&` itself, and backgrounding
	// a command is ordinary where a background job that reads standard input
	// is rare. Unanswered is the POSIX answer, which puts a preset that has
	// chosen nothing in the column five of the six shells are in.
	BackgroundJobInput BackgroundJobInputPolicy
	// LastBackgroundPidIsZeroBeforeAnyJob makes `$!` read `0` before a
	// background command has been started. zsh alone, and it is a number
	// nothing ever had: `sh -c 'echo "[$!]"'` writes `[0]` there and `[]` in
	// bash 5.3, bash 3.2, bash 3.2 as `sh`, dash and ksh93u+.
	//
	// Zero is not the same answer as nothing, which is why this is a switch
	// and not a rendering: a background builtin runs in this process and its
	// job carries no pid, so a shell really can hold a *recorded* zero, and a
	// script cannot tell that apart from zsh's if the two are spelled alike.
	//
	// Read without asking. A dialect that answers nothing answers with
	// nothing, which is what five of the six columns do, and refusing a `$!`
	// expansion over an unanswered field would break the `p=$!` of every
	// script that runs under a preset which has not chosen — including
	// before its first job, where the read is exactly the ordinary one.
	LastBackgroundPidIsZeroBeforeAnyJob Answer
	// LastBackgroundPidIsUnsetBeforeAnyJob makes `$!` an *unset* parameter
	// before a background command has been started, so `set -u` is fatal
	// about it.
	//
	// A different split from the field above, and the more useful one: two
	// shells against two. `set -u; echo "[$!]"` stops bash — `$!: unbound
	// variable`, status 127 — and dash — `!: parameter not set`, status 2 —
	// and is silently empty in ksh93 and zero in zsh, both carrying on at 0.
	// Neither answer predicts the other: zsh's zero is set and ksh93's empty
	// is set too, for different reasons.
	//
	// It is the half a script relies on, since `set -u` exists to stop
	// exactly this read. The wording and the status come from the same
	// Diagnostics fields an unset *name* uses, because measured they are the
	// same two lines — bash writes the `$` back for `$!` as it does for `$1`,
	// which is Diagnostics.UnboundPositional, and dash's is its ordinary
	// `parameter not set`.
	//
	// Read without asking, for the reason above. Unanswered means the
	// parameter is set and empty, which is what this shell did before the
	// axis existed and what the two shells that carry on do.
	LastBackgroundPidIsUnsetBeforeAnyJob Answer
	// ExitInTrapReportsEarlierStatus makes a bare `exit` in an EXIT trap
	// report the status the shell had when the trap began, rather than that
	// of the trap's own last command.
	//
	//	trap "false; exit" 0; true
	//
	// is 0 in bash, dash and ksh93 and 1 in zsh. Only the bare form: `exit 7`
	// is 7 everywhere, and a trap that does not exit at all leaves the
	// script's status alone in all four.
	//
	// Found on an installed script — /usr/bin/bzless traps `stty …; exit` on
	// EXIT, and the `stty` failing made the script exit 1 where every shell
	// exits 0. A wrong exit status is what a caller branches on, so this is
	// the quiet kind of difference.
	ExitInTrapReportsEarlierStatus Answer

	// SignalHandlerSeesEarlierStatus shows a signal handler the status from
	// before the command that triggered it rather than that command's own.
	// zsh alone: after `false; kill -INT $$`, zsh's handler reads 1 where
	// the others read 0, because `kill` succeeded.
	SignalHandlerSeesEarlierStatus Answer
	// ExitTrapIsFunctionLocal fires an EXIT trap set inside a function when
	// that function returns, rather than when the script ends. zsh alone; a
	// trap set at the top level behaves the same everywhere.
	ExitTrapIsFunctionLocal Answer

	// FunctionLocalTraps is whether a trap a function *sets* is undone when
	// that function returns — the displaced disposition coming back, and the
	// signal going back to its default where nothing was displaced. See
	// TrapLocality for the answers and for why it is a form rather than a
	// flag.
	//
	// EXIT is not this question. It is already function-scoped in the one
	// shell that has both, by a rule of its own that does not ask any
	// option — see ExitTrapIsFunctionLocal — and measured, the switch this
	// axis carries changes nothing about it in either direction.
	//
	// unanimous: the four answer alike because zsh's other answer is an
	// option and not a default. Re-measured 2026-09-12: `trap 'echo OUTER'
	// USR1; f() { trap 'echo INNER' USR1; }; f; kill -USR1 $$` prints
	// INNER in all six columns, and the same script under `setopt
	// localtraps` prints OUTER in zsh 5.9.2. A preset is not the whole of
	// a dialect (#2060).
	//
	// unexhibited TrapsGoBackAtTheReturn: zsh under `setopt localtraps`,
	// at run time — the measurement above, wired in
	// dialect/zsh/localtraps.go rather than in the preset. This is the
	// standing example of a value no vector holds and a real shell
	// exhibits (#2060).
	FunctionLocalTraps TrapLocality
	// SIGPrefixAccepted reads `SIGINT` as a name for the same signal `INT`
	// names, wherever a signal can be named.
	//
	// dash alone says no, and says it in three places for two different
	// reasons — the prefix is simply not part of a signal's name there:
	//
	//	trap 'x' SIGINT   trap: SIGINT: bad trap
	//	kill -SIGINT $$   kill: Illegal option -S
	//	kill -s SIGINT $$ kill: invalid signal number or name: SIGINT
	//
	// It is one axis rather than one per builtin because it is a property of
	// how the shell reads a signal name, and the shell that refuses it
	// refuses it everywhere. Asked only where the prefix is actually present
	// and stripping it would name a signal: `trap 'x' INT` needs no answer
	// from anyone, and neither does `SIGNOPE`, which names nothing either way.
	SIGPrefixAccepted Answer

	// KillListAcceptsName lets `kill -l` translate a name into a number, as
	// the reverse of what it does with one. True in bash, ksh93 and zsh.
	//
	// dash's `-l` takes an *exit status* rather than a signal, so `kill -l 9`
	// agrees with everyone by arriving there another way and `kill -l INT` is
	// an illegal number. One question with two answers rather than a feature
	// dash is missing, which is why it is an axis and not a gap.
	KillListAcceptsName Answer

	// ExitTrapRunsOnSignalDeath fires the EXIT trap when the shell is ending
	// because a signal it had no handler for killed it, rather than because
	// it reached the end or ran `exit`.
	//
	//	trap 'echo bye' EXIT; kill -INT $$
	//
	// prints bye in bash and ksh93 and prints nothing in dash and zsh, and
	// all four report 130. A two-two split on whether dying counts as
	// exiting.
	ExitTrapRunsOnSignalDeath Answer

	// QuitIgnoredWhenNotInteractive makes an untrapped SIGQUIT do nothing at
	// all rather than end the shell.
	//
	//	kill -QUIT $$; echo after
	//
	// prints after and exits 0 in bash 5.3 and zsh, and kills the shell with
	// SIGQUIT in dash and ksh93. It is asked only where those disagree — an
	// untrapped QUIT in a shell that is not interactive — because every other
	// case is unanimous: all five in the panel ignore it with `-i`, and a QUIT
	// with a trap runs the handler everywhere.
	//
	// Two things are worth recording beside the split. The first is that this
	// is a *version* divergence as much as a shell one: bash 3.2 dies by
	// SIGQUIT where bash 5.3 ignores it, so the two bash columns of the corpus
	// differ here and a claim about "bash" that does not say which build is
	// incomplete. The second is that where it is ignored it is ignored
	// properly rather than deferred — measured with a signal sent from another
	// process, bash 5.3 and zsh survive that too.
	//
	// What `trap - QUIT` then means is a further question this does not
	// answer, and the panel splits differently on it: after a handler is
	// installed and removed again, bash 5.3 still ignores the signal and zsh
	// dies by it.
	QuitIgnoredWhenNotInteractive Answer

	// SubshellRunsOnAfterSignalingTheShell lets the rest of a subshell's
	// body run after something inside it has sent the whole shell a fatal
	// signal — `(kill -TERM $$; echo inner)`. True in bash, dash and zsh;
	// false in ksh93.
	//
	// The shell ends either way, and that half is unanimous. Measured
	// 2026-09-05, `(kill -TERM $$; echo inner); echo outer` ends the shell by
	// the signal in all six panel members, `outer` is printed by none of
	// them, and the answer is the same on twenty-five runs of each under
	// load — this is not a delivery race. What splits is `inner`: bash
	// 5.3.15, bash 3.2.57, bash 3.2 run as `sh`, dash and zsh 5.9.2 print it
	// and ksh93u+ does not.
	//
	// The reason is the opposite of the obvious one. Measured with a child
	// started inside the subshell and its parent process id read back:
	// bash, dash and zsh give it a **process of its own**, so `$$` names the
	// parent, the child never receives the signal, and it finishes its body
	// while the parent dies. ksh93 runs the subshell **in the shell's own
	// process**, so `kill -TERM $$` is a self-signal landing on the very
	// process that was about to run `echo inner`, and there is nothing left
	// to run it. So the shell that keeps going is the one that forked, and
	// ksh93 is the panel's only member here that does not.
	//
	// Nothing in this implementation forks for a subshell either, which is
	// what makes this an axis rather than a consequence: the answer has to be
	// chosen rather than inherited from the architecture, and choosing the
	// majority is choosing to behave like the shells that fork.
	//
	// Only a subshell. Measured on the same signal at the top level, in a
	// brace group, in a function body and in a `while` body: all six shells
	// stop at once and print nothing, so there is no question to ask
	// anywhere but here.
	//
	// The preset says yes. POSIX has `( )` execute "in a subshell
	// environment" and describes that environment as a copy, which is the
	// forking reading, and it is five of the six.
	SubshellRunsOnAfterSignalingTheShell Answer

	// HangupIsAnOrderlyExit makes an untrapped SIGHUP end the shell the way
	// `exit 1` would rather than by the signal's default action.
	//
	//	kill -HUP $$; echo after
	//
	// reports 1 in zsh and 129 in bash, dash and ksh93, and prints nothing
	// after it anywhere. The number is the visible half; the discipline
	// behind it is the whole answer, and three further measurements say so:
	// the shell's caller sees an ordinary exit rather than a death by
	// SIGHUP, the EXIT trap runs, and an `exit 5` inside that trap wins the
	// status the way it would after any other ending.
	//
	// That the EXIT trap runs is what makes this an axis of its own rather
	// than a number to special-case. ExitTrapRunsOnSignalDeath asks whether
	// dying counts as exiting, and zsh answers no — `trap 'echo bye' EXIT;
	// kill -TERM $$` prints nothing there. `kill -HUP $$` prints bye in the
	// same shell, which is only consistent if SIGHUP never produced a death
	// to ask the question about.
	//
	// It is one signal, and only this one. Measured across the nineteen
	// signals whose default action ends a process — HUP, INT, QUIT, ILL,
	// TRAP, ABRT, FPE, BUS, SEGV, SYS, PIPE, ALRM, TERM, USR1, USR2, XCPU,
	// XFSZ, VTALRM and PROF — the panel is unanimous on every one except
	// QUIT, which QuitIgnoredWhenNotInteractive covers, and this. An
	// external SIGHUP is answered the same way, so it is a disposition
	// rather than something the `kill` builtin does on its way past.
	HangupIsAnOrderlyExit Answer

	// StatusArgument is how a *status operand* is read — the word after
	// `exit` and the word after `return` — and it is an ordering rather
	// than a side:
	//
	//	                    dash   bash 5.3   ksh93   zsh
	//	exit -1 / return -1 error  255        255     -1
	//	exit abc            error  error      0       0
	//	return r  (r=3)     error  error      0       3
	//	return r+1  (r=2)   error  error      0       3
	//	return 3abc         error  error      3       math error
	//	return 300          300    44         44      300
	//
	// One axis for two builtins because the panel reads the two operands
	// identically — every row above was measured on `exit` and on `return`
	// and the two never parted. A second field for `return` is the shape
	// that has cost this tree seven bugs: the copy omits what the original
	// learned, and here it would have left `exit r` in zsh at 0 while
	// `return r` answered 3.
	//
	// Four readings, so a policy rather than a bool — the same shape as
	// UnterminatedBracket, and for the same reason. What each value carries
	// is a bundle rather than three axes, because the four questions were
	// measured together and never crossed: a shell that refuses text is a
	// shell that refuses a sign or does not, masks to eight bits or does
	// not, and ends the script over the refusal or does not. Two of the four
	// refuse nothing at all, so splitting the refusal out would have needed
	// an answer from columns that cannot reach the question.
	StatusArgument StatusArgumentPolicy
	// BracketCaretNegates reads `[^abc]` as a negated class. dash alone
	// treats `^` as an ordinary character, so `[^abc]` matches a caret there
	// and everything-but there elsewhere: the two answers are both matches,
	// on different inputs, with nothing to warn on.
	BracketCaretNegates Answer
	// GetoptsAssignmentRestartsWord makes assigning OPTIND begin the word
	// again, dropping any position inside a cluster.
	//
	// True in bash, dash and ksh93, and it is the *assignment* that does it
	// rather than the value: `set -- -ab; getopts ab o; OPTIND=1` writes the
	// number OPTIND already held, and those three still restart and read `a`
	// a second time where zsh carries on to `b`.
	GetoptsAssignmentRestartsWord Answer

	// GetoptsPositionIsFunctionLocal gives every shell function call its own
	// `getopts` cursor: OPTIND starts the call at 1 whatever the caller had
	// reached, and the caller's position comes back when the call returns.
	//
	// zsh alone, and it is the parameter itself that is local rather than
	// only the builtin's bookkeeping — an explicit assignment inside the
	// function does not escape either:
	//
	//	g() { echo "entry=$OPTIND"; OPTIND=7; }
	//	OPTIND=3; g; echo "after=$OPTIND"
	//
	// answers entry=1 after=3 in zsh and entry=3 after=7 in bash, bash 3.2,
	// dash and ksh93. The position *inside* a clustered word is saved with
	// it, which a shared cursor cannot express: with `-ab` half read, a
	// function scanning `-cd` of its own reads both `c` and `d` in zsh and
	// only `d` everywhere else, and on return the caller still finds its
	// `b`.
	//
	// dash looks close and is not: it leaves OPTIND at 2 on the way out and
	// starts the *next* scan at 1 because its `getopts` resets the cursor
	// when it runs out of options. zsh shows 2 inside the function and 1
	// outside, which is a restore rather than a reset.
	//
	// What this does not cover is `unset OPTIND`, which takes the parameter
	// away rather than giving the call a value of its own: the name stays
	// gone after the function returns in every shell measured, so there is
	// nothing for the return to put back. A function entered with OPTIND
	// already unset is not handed a cursor at 1 either.
	//
	// Why it earns an axis rather than a note: a shell function that parses
	// options is only reusable if the second call starts over, so zsh's own
	// function library is written *without* the `local OPTIND=1` the others
	// need. `add-zsh-hook -Uz precmd f` followed by any second
	// `add-zsh-hook` had the second call reading its arguments from index 2
	// and printing its usage — see #1392.
	GetoptsPositionIsFunctionLocal Answer

	// GetoptsLocalOptindRestoresTheCursor hands the caller back its position
	// *inside* a clustered word when a call that declared a local `OPTIND`
	// returns — not only the number the parameter held.
	//
	// The scan position has two halves: `OPTIND`, which counts words, and
	// how far into a clustered word the letters have been read. Only the
	// first is a parameter, so a shell that shadows the parameter and stops
	// there hands the caller a cursor pointing at the start of a word it had
	// already part-read. That is measurable with no `getopts` in the callee
	// at all:
	//
	//	g() { local OPTIND=1; :; }
	//	set -- -ab; getopts ab o; g; getopts ab o
	//
	// The second `getopts` reads `b` in bash, ksh93 and zsh, and reads `a` a
	// second time in bash 3.2 and dash. Reading `a` again is not a wrong
	// letter so much as a scan that cannot finish: a loop whose body calls a
	// function that declares `local OPTIND` starts the same word over every
	// time round and never runs out of options (#2226).
	//
	// Entering the call is *not* the disagreement and is not asked here:
	// every shell in the panel that has a local scope at all hands the callee
	// a cursor at the start of a word, so a callee scanning its own `-cd`
	// reads both letters whether or not the declaration carried a value.
	// What splits the panel is only the way back.
	//
	// zsh answers yes and reaches it by a different route — its cursor is
	// local to every call whether or not anything was declared, which is
	// GetoptsPositionIsFunctionLocal. ksh93 has no `local`, and answers this
	// through `typeset` in a function defined with the `function` word.
	GetoptsLocalOptindRestoresTheCursor Answer

	// GetoptsClearsOptarg empties OPTARG when `getopts` reports a bad option
	// rather than leaving it unset. zsh alone, and a script testing
	// `${OPTARG-}` can tell the two apart.
	GetoptsClearsOptarg Answer

	// CdWithoutHomeIsAnError makes `cd` with no operand and no HOME a
	// failure. True in bash and ksh93; dash and zsh stay where they are and
	// report success, which is the quieter answer and the surprising one.
	// The same axis answers `cd -` with no OLDPWD.
	CdWithoutHomeIsAnError Answer
	// CdEmptyOperandIsAnError refuses `cd ""` instead of taking it as the
	// directory the shell is already in. True in bash and ksh93.
	//
	// An empty operand is not the same thing as no operand, and it is not
	// nothing either: measured 2026-09-10, `cd /tmp; OLDPWD=MARK; cd ""`
	// leaves OLDPWD as `/tmp` in dash, bash 3.2 and zsh, and zsh's `chpwd`
	// fires — so it is a real move to the same place, which joining an empty
	// operand against the working directory already is. bash 5.3 and bash
	// called as `sh` say `cd: null directory` and ksh93 `cd: bad directory`,
	// both at 1 and both staying put.
	CdEmptyOperandIsAnError Answer

	// CdEmptyHomeIsAnError refuses `cd` with HOME set to the empty string,
	// rather than going where the shell already is. True in ksh93 alone.
	//
	// A separate question from CdWithoutHomeIsAnError, which is about a HOME
	// that is *absent*, and separate from the axis above, which is about an
	// operand: bash answers yes to the first, no to this one and yes to the
	// third, so no two of them can be one field. Measured: `HOME= cd` says
	// nothing and reports 0 in dash, both bashes and zsh, and `cd: bad
	// directory` at 1 in ksh93 — which is the same sentence it refuses an
	// empty operand with.
	CdEmptyHomeIsAnError Answer

	// CdSubstitutesTheOperands reads `cd old new` as a rewrite of the
	// current directory — the first occurrence of old in `$PWD` replaced by
	// new — rather than as too many operands. True in ksh93 and zsh.
	//
	// Measured from `…/x/alpha`: `cd alpha beta` lands in `…/x/beta` in both,
	// and `cd a Z` from `…/a/q/a/w` lands in `…/Z/q/a/w`, so it is the first
	// occurrence in the string rather than the first path component. bash
	// refuses the shape outright and dash and bash 3.2 ignore everything
	// after the first operand.
	CdSubstitutesTheOperands Answer

	// CdSubstitutionPrintsTheDirectory writes where a `cd old new` went, the
	// way `cd -` writes where it went. True in ksh93; zsh moves in silence.
	//
	// Asked only on a substitution that arrived somewhere: a rewrite naming
	// a directory that is not there prints nothing in either shell.
	CdSubstitutionPrintsTheDirectory Answer

	// CdRefusesExtraOperands refuses operands after the first instead of
	// ignoring them, in a dialect that does not read two as a substitution.
	// True in bash; false in dash and bash 3.2, which take the first and say
	// nothing about the rest.
	//
	// Asked only where CdSubstitutesTheOperands said no, which is the point
	// the two shells that have the form are no longer in the conversation.
	CdRefusesExtraOperands Answer

	// CdDashPrintsTheDirectory writes the new directory when `cd -` moves.
	// True in bash, dash and ksh93; zsh alone is silent.
	CdDashPrintsTheDirectory Answer

	// PrintfReportsBadNumber complains when a numeric conversion is given
	// something that is not a number. True in bash and dash, false in ksh93
	// and zsh — and all four print the zero either way, so the complaint sits
	// beside the output rather than instead of it.
	PrintfReportsBadNumber Answer
	// PrintfBackslashC is what `\c` means in a printf format, and it is three
	// different things rather than a switch:
	//
	//	printf "a\cbZ"   bash, dash  a\cbZ      two literal characters
	//	                 ksh93       a<0x02>Z   \cX is control-X
	//	                 zsh         a          the output stops there
	//
	// Measured by the bytes rather than by the display, which is the only way
	// to tell the middle one from the last: ksh93's output *looks* truncated
	// next to zsh's until the control character is read as a byte.
	PrintfBackslashC PrintfBackslashCPolicy
	// PrintfUnfinishedConversionIsAPercent writes a bare `%` for a format
	// that ended before its conversion character, and reports success,
	// rather than complaining about a conversion it could not read.
	//
	//	printf 'a%'    bash  a, and `%': missing format character   st=1
	//	printf 'a%5'   zsh   a, and %5: invalid directive           st=1
	//	printf 'a%ll'  dash  a, and missing format character        st=2
	//	printf 'a%5'   ksh93 a%                                     st=0
	//
	// The whole unfinished conversion becomes the one character: `a%5` and
	// `a%ll` are both `a%` there, so the prefix that was scanned is dropped
	// rather than written back.
	//
	// Asked only where a format actually ends inside a conversion.
	PrintfUnfinishedConversionIsAPercent Answer
	// PrintfHexEscape is how a printf format reads `\x`, and it is four
	// answers rather than a presence:
	//
	//	printf 'a\x80Z'   bash, ksh93, zsh  a<0x80>Z    one raw byte
	//	                  dash              a\x80Z      not an escape at all
	//	printf 'a\x0ffZ'  bash, zsh         a<0x0f>ffZ  two digits, then text
	//	                  ksh93             a<0xc3><0xbf>Z  every digit, a code point
	//	printf 'a\xZ'     bash              a\xZ        and a complaint
	//	                  ksh93, zsh        a<0x00>Z    an empty digit run is zero
	//	                  dash              a\xZ        not an escape at all
	//
	// Asked only where a `\x` is actually in the format. It is a question
	// about the *format*, and never about a `%b` argument: that site has its
	// own table and its own axis, PrintfBHexEscape below.
	PrintfHexEscape PrintfHexEscapePolicy
	// PrintfBHexEscape is how a `%b` argument reads `\x`, which is a
	// different question from the one PrintfHexEscape answers: ksh93 reads
	// `\x41` in a format and writes the four characters as they stand in a
	// `%b`, so the site decides as much as the shell does.
	//
	//	printf '%b' 'a\x41Z'  bash, zsh    aAZ
	//	                      dash, ksh93  a\x41Z
	//	printf '%b' 'a\xZ'    bash         a\xZ, and the same complaint a
	//	                                   format's empty digit run draws
	//	                      zsh          a<0x00>Z
	//	                      dash, ksh93  a\xZ
	//
	// The readings themselves are the format's four, which is why this is
	// the same enumeration: a shell that has the escape here reads its
	// digits the way it reads a format's.
	//
	// Asked only where a `%b` argument actually carries a `\x`.
	//
	// unexhibited PrintfHexEscapeCodePoint: PrintfHexEscape holds it, for
	// ksh93 — which is why the two fields share an enumeration and why
	// they are two fields. Re-measured 2026-09-12: `printf '%b\n'
	// 'a\x41Z'` writes `a\x41Z` in ksh93u+ and dash, so this site's ksh93
	// answer is Absent while the format's is the code point. Nothing in
	// the panel reads a `%b` argument's `\x` as a code point (#2060).
	PrintfBHexEscape PrintfHexEscapePolicy
	// PrintfUnicodeEscape is how a printf format reads `\uHHHH` and
	// `\UHHHHHHHH`, and it is four answers rather than a presence — the same
	// shape PrintfHexEscape has, arrived at from the same three questions and
	// splitting in a different place:
	//
	//	printf 'a\u0041Z'  bash 5.3, ksh93, zsh  aAZ
	//	                    bash 3.2, dash  a\u0041Z
	//	printf 'a\uZ'      bash 5.3  a\uZ, and `printf: missing unicode digit
	//	                              for \u` on standard error, status 0
	//	                    zsh       a<0x00>Z
	//	                    ksh93     a  — the rest of the format pass is
	//	                              dropped, and the loop over the operands
	//	                              goes on, so `printf '[%s]\uZ' x y` is
	//	                              `[x][y]`
	//
	// That fourth reading is why this is not PrintfHexEscapePolicy under
	// another name: no `\x` anywhere in the panel drops what follows it.
	//
	// Four digits after `\u` and eight after `\U`, and fewer are accepted:
	// `a\u41Z` is `aAZ` in all three that have the escape, and `a\u00410` is
	// an `A` followed by a zero. The value is a code point written in UTF-8
	// rather than a byte, and it is the *original* UTF-8 rather than the
	// range Unicode later kept — see EncodeCodePoint, which is the one
	// encoder both escape sites and `echo` share.
	//
	// Asked only where a `\u` or a `\U` is actually in the format. It is a
	// question about the *format*, and never about a `%b` argument: that
	// site has its own axis, PrintfBUnicodeEscape below.
	PrintfUnicodeEscape PrintfUnicodeEscapePolicy
	// PrintfBUnicodeEscape is how a `%b` argument reads `\u` and `\U`, which
	// is a different question from the one PrintfUnicodeEscape answers, and
	// ksh93 is again the shell that separates them:
	//
	//	printf '%b' 'a\u0041Z'  bash 5.3, zsh  aAZ
	//	                         bash 3.2, dash, ksh93  as written
	//
	// So ksh93 reads the escape in a format and writes the characters as
	// they stand in a `%b`, exactly as it does for `\x`. No dialect in the
	// panel answers this site with the truncating reading; the enumeration is
	// shared because a shell that has the escape here reads its digits the
	// way it reads a format's.
	//
	// Asked only where a `%b` argument actually carries a `\u` or a `\U`.
	//
	// unexhibited PrintfUnicodeEscapeCodePointOrTruncate:
	// PrintfUnicodeEscape holds it, for ksh93; at this site ksh93u+ is
	// Absent. Re-measured 2026-09-12: `printf '%b\n' 'a\u0041Z'` writes
	// the escape as written in dash, bash 3.2.57 and ksh93u+, and `aAZ` in
	// bash 5.3.15 and zsh 5.9.2. Nothing truncates here (#2060).
	PrintfBUnicodeEscape PrintfUnicodeEscapePolicy
	// PrintfBEscEscape admits `\e` in a `%b` argument for the escape
	// character: bash and zsh do, dash and ksh93 write the two characters.
	//
	// It is a separate axis from PrintfBCapitalEscEscape below because the
	// two shells that split them split them in opposite directions, so no
	// single answer describes either one: ksh93 has `\E` and not `\e`, and
	// zsh has `\e` and not `\E`.
	//
	// Asked only where a `%b` argument actually carries a `\e`.
	PrintfBEscEscape Answer
	// PrintfBCapitalEscEscape admits `\E` in a `%b` argument: bash and ksh93
	// do, dash and zsh write the two characters. See PrintfBEscEscape for
	// why the two letters are two questions.
	//
	// Asked only where a `%b` argument actually carries a `\E`.
	PrintfBCapitalEscEscape Answer
	// PrintfBStopIsPadded puts what a `\c` left of a `%b` argument through the
	// conversion's field all the same — the width, the precision and the
	// left-justifying flag. bash, dash and zsh do; ksh93 alone writes the
	// partial text as it stands:
	//
	//	printf '[%5b]'   'a\cb'   five  [    a      ksh93  [a
	//	printf '[%-5b]'  'a\cb'   five  [a          ksh93  [a
	//	printf '[%.1b]'  'ab\cc'  five  [a          ksh93  [ab
	//
	// It is a property of the *stop* and not of the conversion: with nothing
	// stopping it ksh93 pads and truncates like the rest, so `printf '[%5b]'
	// 'ab'` is `[   ab` in all six.
	//
	// Asked only where a `\c` actually stopped a `%b` *and* the field would
	// change the text, so an ordinary `printf '%b' 'a\cb'` needs no dialect.
	PrintfBStopIsPadded Answer
	// PrintfBOctalWithoutZero reads a `%b` argument's `\nnn` as octal with no
	// leading zero to introduce it. bash and dash do; ksh93 and zsh want the
	// `\0` and write `\101` as the four characters it is.
	//
	// The `\0nnn` form itself is unanimous and asks nothing — it is the XSI
	// escape `echo` expands, and reading it as a format's octal is what put
	// a backspace and a `1` where every shell writes an `A` (#798).
	//
	// This one is not `echo`'s answer at the other site: an `echo` argument's
	// `\101` is an escape in dash alone, where a `%b` argument's is an escape
	// in bash too.
	//
	// Asked only where a `%b` argument actually carries such an escape.
	PrintfBOctalWithoutZero Answer
	// PrintfLengthModifiers is which C length modifiers a conversion may
	// carry between its precision and its verb — `%zX`, `%ld`, `%jd`.
	//
	// Three answers, and every one of them *ignores* the modifier rather
	// than acting on it: `%hhd` with 300 is 300 and not 44, and `%lld` with
	// the largest signed 64-bit value is that value, in every shell that
	// takes the modifier at all. A shell's arithmetic is one width and the
	// modifier cannot change it, so this is about what a format may say and
	// never about what it means.
	//
	// Asked only where a conversion actually carries one of the letters, so
	// a dialect is never questioned about `%s`.
	PrintfLengthModifiers PrintfLengthModifierSet
	// PrintfOutputPrecedesComplaint writes what `printf` produced before it
	// complains about the rest, rather than after.
	//
	// True in ksh93 alone. It is visible only where both streams arrive at
	// one place, which is exactly how the corpus reads them: `printf "[%z]"`
	// is `[` then the complaint there and the complaint then `[` in the other
	// three, whose output is still sitting in a buffer when the complaint
	// goes out.
	//
	// unexhibited No: five of the six columns. Re-measured 2026-09-12 with
	// both streams arriving at one place: `printf "[%z]"` writes the
	// complaint and then `[` in dash, bash 5.3.15, bash-as-`sh`, bash
	// 3.2.57 and zsh 5.9.2, and `[` and then the complaint in ksh93u+. No
	// preset writes it because the axis is read (`== Yes`) and not asked,
	// so `No` and silence reach the same code (#2060).
	PrintfOutputPrecedesComplaint Answer
	// PrintfEmptyIsNotANumber complains about a numeric conversion given an
	// operand that is present and empty. bash alone: `printf '%d' ""` is an
	// error there and a zero in the other three, all of which print the zero
	// anyway. An argument that is *missing* is never an error in any of them.
	//
	// unpinned zsh: reached, and both answers print the same thing there.
	// Measured 2026-09-12: moving it to `Unspecified` in the zsh dialect
	// *does* change `printf '%d' ""`, so the axis is consulted — but `Yes`
	// only sends the empty operand on to the bad-number complaint, and
	// zsh's `PrintfReportsBadNumber` is `No`, so the complaint is
	// swallowed and the bare `0` comes out either way. `printf '%d' abc`
	// is a silent `0` in real zsh too, which is the same fact from the
	// other side. No row can separate the two until that second axis moves
	// (#2057).
	PrintfEmptyIsNotANumber Answer

	// PidListingFinishesWithAJob makes `jobs -p` forget a finished job the
	// way a listing of states does.
	//
	// ksh93 alone. Measured 2026-09-12 with a background command that has
	// already ended: `jobs -p` writes the process id in dash, bash and
	// ksh93 — zsh writes nothing for a job that is over — and the *next*
	// `jobs` reports it as Done in dash and bash and shows nothing in
	// ksh93. So the pid listing is a listing that finishes with the job in
	// one column and a peek that leaves it in two.
	//
	// Asked only where a pid listing met a finished job, so an ordinary
	// `jobs` never raises it: that form finishes with the job everywhere and
	// needs no dialect.
	PidListingFinishesWithAJob Answer

	// PrintfTimeConversion gives `printf` a `%(fmt)T`: an epoch through a
	// date format, with the format written inside the conversion. bash 5.3's
	// alone among the panel — dash and zsh call `%(` a directive they do not
	// have, and bash 3.2 an invalid format character.
	//
	// The operand is seconds since the epoch, and two numbers are not times:
	// -1 is now and -2 is when the shell started. A missing operand is now
	// as well, and an empty format is the C locale's time of day.
	//
	// ksh93 also has a `%T`, and it is not this one: its operand is a date
	// *string* — `now`, `tomorrow` — and a number earns a warning and the
	// current time instead. Answered No there and recorded in
	// docs/spec/semantics.md rather than modeled, because reading a date the
	// way ksh93 reads one is its own feature.
	//
	// Asked only where a format actually carries a `%(`, so a dialect
	// without the conversion is never questioned about `%s`.
	PrintfTimeConversion Answer

	// PrintfTimeOperandIsADateString makes that conversion's operand a date
	// *string* rather than a number of seconds, and gives the shell a plain
	// `%T` with no parentheses as well.
	//
	// ksh93 alone, and asked only where PrintfTimeConversion already said
	// yes — a dialect without the conversion is never questioned about its
	// operand. See interp/printfdate.go for the strings, the subset taken
	// and why the rest meet that shell's own warning rather than a guess.
	//
	// It moves the empty format too: `%()T` is the time of day where the
	// operand is an epoch and the full `date` line where it is a string,
	// which is the same default the bare `%T` writes.
	PrintfTimeOperandIsADateString Answer

	// PrintfQuote is how `%q` quotes, which is three answers and an absence
	// rather than a switch — see PrintfQuoteStyle.
	PrintfQuote PrintfQuoteStyle

	// RedirectsUseEveryTarget makes a stream redirected more than once use
	// *every* file it names rather than only the last, in both directions:
	// output goes to all of them and input arrives as all of them in the
	// order written. `echo x >a >b` fills both in zsh and leaves `a` empty in
	// the other five; `cat <a <b` is both files there and only `b` elsewhere.
	//
	// One axis and not two, because it is one switch in the one shell that
	// has it: turning that shell's option off takes the fan-out and the
	// concatenation together, and two fields would be two places to forget
	// one of them. Named for a target rather than for a direction for the
	// same reason.
	//
	// Silent either way — the shells that use the last target alone report no
	// error, and the script looks like it worked — which is the `&>` failure
	// mode in a redirection. Asked only where a command redirects one stream
	// twice, because that is the only place it decides anything.
	RedirectsUseEveryTarget Answer

	// NullCommandVariable names the parameter holding the command that a
	// command consisting only of redirections runs.
	//
	// Empty is the core's answer and five of the six panel columns': `<f`
	// opens the file, runs nothing and writes nothing, and `>g` truncates
	// `g` the same way. One shell instead treats the redirections as
	// arguments to a command named by this parameter, so `<f` at a prompt
	// pages the file — measured by pointing the parameter at a function that
	// prints a marker and watching the marker come out. Its own name for
	// this is `NULLCMD` and it defaults to `cat`.
	//
	// A name rather than a value, because the parameter is a script's to
	// reassign at any moment and the answer has to be read when the command
	// runs, not when the dialect is built.
	//
	// The hook is off entirely while this is empty, which is what keeps the
	// two readings apart: a shell without it runs nothing and *succeeds*,
	// where a shell with it and an empty parameter refuses the command by
	// name — see Diagnostics.RedirectionWithNoCommand. So "no hook" and "a
	// hook with nothing in it" are different observable shells, and this
	// field is the first of them.
	//
	// What it is not: the `$(<file)` form, whose whole body is one input
	// redirection. That form does not consult this parameter — measured, and
	// see readfilesubst.go — so a substitution reads the file even where the
	// hook is pointed somewhere else. Two operands, two paths.
	NullCommandVariable string

	// ReadNullCommandVariable names the parameter consulted in place of
	// NullCommandVariable when the command's *only* redirection is a plain
	// input file redirection.
	//
	// One redirection and one operator: `<f` and `3<f` both take this route
	// — the descriptor number does not matter — where `<f <g`, `<>f`, `<<<x`,
	// `<&0`, `2>e` and `<f 2>e` all take the other. Measured a spelling at a
	// time with the two parameters pointed at two different marker functions,
	// which is the only probe that can tell them apart: with both left at
	// their defaults the two routes print the same file and the reading is
	// unfalsifiable.
	//
	// Empty — the parameter unset, or set to nothing — falls back to
	// NullCommandVariable rather than refusing, so a script that clears the
	// reader still pages nothing and cats instead. The shell that has it
	// calls it `READNULLCMD` and defaults it to `more`.
	ReadNullCommandVariable string

	// NoclobberBlocksAppendCreate makes `set -C` stop `>>` from *creating* a
	// file, so appending to a name that is not there is a refusal rather
	// than a new file. Asked only under noclobber, which is the only place
	// it decides anything.
	//
	// POSIX puts noclobber on `>` alone — 2.7.2 makes `>` fail when the file
	// exists and says nothing about `>>` — so the standard's answer is No,
	// and it is dash's, bash 5.3's, bash 3.2's and ksh93's: `set -C; echo hi
	// >> f` on a missing `f` creates it and reports 0 in all five, bash 5.3
	// under an argv[0] of `sh` included. zsh 5.9.2 is the departure, and
	// refuses at 1 with `no such file or directory`. Measured 2026-09-07,
	// with appending to a file that *does* exist as the control: all six
	// append and report 0.
	//
	// It is the reason `>>|` exists. Where this is No the override has
	// nothing to override, so a dialect that answers Yes here is the only
	// one for which the append half of syntax.Dialect.ClobberOverrideMarker
	// is observable — which is why the two were measured and added together.
	NoclobberBlocksAppendCreate Answer

	// KillStatus is what `kill` reports when it was given several targets
	// and they did not all agree. Three answers, and no two of them are the
	// majority:
	//
	//	kill -0 $$ 999999    bash → 0   dash, ksh93 → 1   zsh → 1
	//	kill 999998 999999   bash → 1   dash, ksh93 → 1   zsh → 2
	//
	// bash reports success if it signaled anything at all, and zsh reports
	// the number that failed — which is a status carrying a count rather
	// than a verdict, and the reason this is a policy rather than a bool.
	KillStatus KillStatusPolicy
	// CommandNotFoundStatusIsNotFound makes `command -v` answer 127 for a
	// name that is nothing, rather than a plain 1. dash alone says yes; the
	// other three report a failure and leave 127 to mean a command that was
	// looked for and run.
	CommandNotFoundStatusIsNotFound Answer

	// SubshellJobTable is what a subshell sees of the jobs its parent
	// started. Three answers, and neither of the two-way splits it contains
	// is the same pair:
	//
	//	sleep 1 & jobs -p | cat; echo T    bash, ksh93 → the pid   dash, zsh → nothing
	//	sleep 1 & (jobs -p); echo T        ksh93 → the pid         bash, dash, zsh → nothing
	//
	// so no single yes-or-no can hold both rows for bash. See
	// SubshellJobsKeptOutsideACompound for what bash is doing and for the
	// part of it that is measured and not modeled.
	SubshellJobTable SubshellJobTable

	// InteractiveSelectsEmacs turns the `emacs` editing mode on when the
	// shell becomes interactive, and leaves both mode names off otherwise.
	// True in bash alone. Measured 2026-09-11, both without a terminal and
	// at one: `bash -c 'set -o'` reports `emacs off` and `vi off`,
	// `bash -i -c 'set -o'` reports `emacs on`, and bash 3.2 and bash
	// invoked as `sh` agree. dash, ksh93 and zsh select neither at any point
	// — ksh93 reports `emacs off` and `vi off` in an interactive session at
	// a real terminal, and zsh answers 1 to both `[[ -o emacs ]]` and
	// `[[ -o vi ]]` there.
	//
	// So the trigger is interactivity rather than a terminal, which is what
	// makes it a question about *when a mode is chosen* rather than about
	// which mode. A script that selects one is unaffected either way: this
	// only says what an unasked shell reads.
	//
	// Read rather than `ask`ed, as DefaultOptionLetters is: reporting an
	// option is not the place to refuse a script over a disagreement, and a
	// dialect that answers nothing gets the majority's no.
	InteractiveSelectsEmacs Answer

	// SetFTurnsOffGlobbing makes `set -f` the short spelling of `set -o
	// noglob`. True in bash, dash and ksh93. zsh spells that option the long
	// way only: there `-f` is about startup files and leaves globbing alone,
	// so `set -f; echo *.txt` lists the files.
	SetFTurnsOffGlobbing Answer

	// SetBTurnsOffBraceExpansion makes `-B` the short spelling of the
	// `braceexpand` option, so `set +B` stops `{a,b}` expanding and `set -B`
	// puts it back. True in bash and ksh93. zsh has the letter and means
	// something else by it — measured 2026-09-11, `set -B` there turns the
	// terminal bell off and leaves braces alone, so that dialect keeps `B`
	// among the letters it refuses. dash has no such letter at all.
	//
	// Asked only where the letter is written, like SetFTurnsOffGlobbing: the
	// long name `braceexpand` raises no question, because a shell either
	// declares it or has never heard of it.
	//
	// unpinned zsh: every row that reaches it fails at baseline, so none
	// can pin it. This dialect refuses the `B` letter as unimplemented —
	// the bell it means there is not built (#1856) — while real zsh takes
	// `set +B` silently and goes on expanding braces. Measured 2026-09-12:
	// `set +B; echo {a,b}` is `a b` at 0 in zsh 5.9.2 and `+B is not
	// implemented yet` at 1 here, so a corpus row would record a
	// divergence this axis is not about. It pins in bash and ksh, where
	// the letter is real (#2057).
	SetBTurnsOffBraceExpansion Answer

	// NoglobLetterIsF puts `f` in `$-` while noglob is on, which is the
	// letter POSIX gives it and what bash, dash and ksh93 report. False in
	// zsh, which reports the capital: `-F` is the short option that means
	// noglob there, `-f` being about startup files — the same split
	// SetFTurnsOffGlobbing records, seen from the reading side.
	NoglobLetterIsF Answer

	// DefaultOptionLetters is what `$-` starts with before the script has
	// set anything: the single-letter options a shell turns on at startup.
	// Measured identical under `-c`, a script file and standard input —
	// bash and ksh93 report `hB`, zsh `569X`, dash nothing at all.
	//
	// The letters that describe the invocation *route* rather than an option
	// a script could set — `c` and `s` — are not here either, for the reason
	// `i` is not: they are facts about the invocation that the front end
	// carries in, read off Runner.Route. Where the panel splits over them
	// they have axes of their own, below.
	//
	// The letters a shell turns on only *when* it is interactive are a
	// second vector of their own; see InteractiveOptionLetters, which
	// replaces this one rather than adding to it.
	DefaultOptionLetters string

	// InteractiveOptionLetters is DefaultOptionLetters for a shell that is
	// interactive. It **replaces** the other rather than being appended to
	// it, and that is the whole reason it is a second string instead of a
	// field of letters to add.
	//
	// ksh93 is what forces the shape: measured, its `$-` goes from `hB` for
	// a script to `imBE` for `-i script.sh`, so it *drops* `h` — its
	// command-tracking option, which is on for a script and off at a prompt.
	// A "letters to add" field could not have said that, and a field that
	// could only add would have recorded three shells correctly and one
	// wrongly. bash goes `hB` to `hiBH` and zsh `569X` to `569XZi`, both of
	// which a replacement expresses just as well.
	//
	// Empty means the shell has no separate answer and DefaultOptionLetters
	// stands for both. dash is the panel member that leaves it so: its `$-`
	// is empty either way, so there is nothing for a second string to say.
	//
	// Two letters are deliberately *not* in it, and both for the same reason
	// the route letters are not in DefaultOptionLetters — they are facts the
	// runner holds rather than a string it prints:
	//
	//   - `i` itself, which is unanimous and comes from Runner.Interactive.
	//   - `m`, the monitor. ksh93 is the only shell in the panel that turns
	//     job control on for `-i script.sh` — measured, `set -o` reports
	//     `monitor on` there, and off in bash and zsh, which is exactly why
	//     the letter appears in its row and no other. Writing `m` into this
	//     string would report a monitor that is not running. The letter comes
	//     from Runner.monitor or it does not come at all; that ksh93 turns it
	//     on for `-i script.sh` where the front end does not is measured and
	//     recorded in docs/spec/invocation.md, and is a separate question
	//     from this one.
	//
	// Read rather than `ask`ed, exactly as DefaultOptionLetters is: a dialect
	// that answers nothing shows the letters it shows for a script, and
	// refusing a whole `$-` expansion over an unanswered field would break
	// `case $- in *e*)` in every script running under a preset that has not
	// chosen.
	InteractiveOptionLetters string

	// CommandStringShowsCInDollarDash puts `c` in `$-` when the program came
	// from `-c`. Two against two: bash and ksh93 do, dash and zsh do not, so
	// there is no majority to follow and this is a switch.
	//
	// The POSIX preset says yes, from the text rather than from a vote: `$-`
	// is defined as the option flags specified on invocation, and `-c` is
	// one of them.
	//
	// Read without asking, unlike most axes. A dialect that answers nothing
	// shows no letter, which is the same thing an unanswered
	// DefaultOptionLetters does; refusing a whole `$-` expansion over it
	// would break `case $- in *e*)`, the ordinary errexit check, in every
	// script that runs under a preset which has not chosen.
	CommandStringShowsCInDollarDash Answer

	// CommandStringShowsSInDollarDash also puts `s` there under `-c`.
	//
	// ksh93 alone, and the shape of the disagreement is worth stating: `s`
	// itself is unanimous for the standard-input route — with `-s` written
	// or not, and at a prompt — so what splits the panel is only whether a
	// command string counts. Read down ksh93's rows and its rule is "no
	// script file was named" where the other three's is "the program came
	// from standard input"; the two agree everywhere except here.
	//
	// Read without asking, for the reason above.
	CommandStringShowsSInDollarDash Answer

	// LoginShowsLInDollarDash puts `l` in `$-` when the shell was started as
	// a login shell.
	//
	// Four against two, so there is no majority to follow and this is a
	// switch: ksh93 and zsh say yes, and dash and all three bash columns say
	// no. bash's no is a deliberate one rather than an omission — it keeps
	// the fact in `shopt login_shell`, which reads `on` for exactly the
	// invocations this letter would mark, so the shell answers the question
	// and answers it somewhere else.
	//
	// Measured 2026-09-06 with the letters bundled (`-lc`), unbundled
	// (`-l -c`), spelled long (`--login -c`) and inferred from a dashed
	// `argv[0]` with no option at all: every column answers the same way on
	// all four, so the split belongs to the shell and not to how the caller
	// said it. Membership rather than the spelling in the cases that pin it
	// — ksh93 writes `chsBl` and zsh `569Xl`, and no two shells in the panel
	// order the string alike.
	//
	// The fact itself is Runner.LoginShell, carried in from the front end:
	// no `set` letter turns login-ness on in three of the four dialects, so
	// there is no option field for the table to write. zsh is the exception
	// and is recorded rather than implemented — see docs/spec/semantics.md,
	// "the login letter": there `l` is a genuine `set` option, `set +l`
	// takes it back out of `$-` and `set -l` puts it in, which this shell
	// does not do.
	//
	// The POSIX preset leaves it unanswered, which shows no letter: POSIX
	// names no login option at all, so there is nothing for `$-` to report
	// as one — unlike CommandStringShowsCInDollarDash, where `-c` *is* an
	// invocation flag the text defines.
	//
	// Read without asking, for the reason above.
	LoginShowsLInDollarDash Answer

	// ArithIntegerOperatorRefusesFloat rejects a float where only an integer
	// will do — `7 % 2.5`, `1.5 & 1`, a shift. ksh93 says yes and refuses;
	// zsh says no and truncates. It does not arise in a shell without floats,
	// which is why bash and dash leave it unanswered.
	ArithIntegerOperatorRefusesFloat Answer

	// ArithNegativeExponentIsError refuses `2**-1` rather than answering
	// with a float. bash says yes and stops the expression; ksh93 and zsh
	// say no and answer 0.5. It does not arise where the grammar has no
	// `**`, which is why dash leaves it unanswered.
	ArithNegativeExponentIsError Answer

	// ProcessSubstitutionInCondition lets `<(cmd)` stand as a condition's
	// operand — `[[ $v == <(cmd) ]]` — and be performed there.
	//
	// bash alone. zsh reads the word and then refuses it, at status 2 and in
	// a sentence of its own; ksh93 refuses earlier still, while reading, and
	// dash has no `[[ ]]` to refuse it in. So the answer is no for three of
	// the four, and what differs between them is only when and in what words
	// — which is exactly the split between this axis and Diagnostics.
	//
	// It is asked *before* the substitution is performed. A shell that
	// refuses the word must not have started the command first, and that is
	// observable: the command has side effects.
	ProcessSubstitutionInCondition Answer

	// ProcessSubstitutionBodyReadsTheShellsInput hands a process
	// substitution's body the standard input the *shell* has, rather than
	// the standard input of the command whose word the substitution stands
	// in.
	//
	// The two are the same stream almost everywhere, which is what makes the
	// axis narrow and is why the obvious control row cannot see it: `cat
	// <(cat)` reads the shell's input in every shell that has the construct,
	// because the command's input *is* the shell's. They part inside a
	// pipeline element, whose input is the pipe:
	//
	//	printf "PIPE\n" | cat <(cat)      with the shell's input a file
	//	                                  holding OUTER
	//
	// zsh answers OUTER and is alone in it; bash 5.3, bash 3.2, bash as `sh`
	// and ksh93 all answer PIPE, and dash has no such construct. Measured
	// 2026-09-11 and again on the panel for #1933.
	//
	// The reading behind zsh's answer is that a pipeline element's pipe is
	// one of that element's *redirections*, and a redirection is applied
	// after the command's words have been expanded — so a substitution
	// performed while expanding them is still looking at the shell's own
	// input. That reading is what bounds the axis, and every boundary below
	// is measured rather than inferred, because zsh agrees with the rest of
	// the panel at each of them:
	//
	//	printf "PIPE\n" | { cat <(cat); }        PIPE everywhere
	//	f() { cat <(cat); }; printf … | f        PIPE everywhere
	//	printf "PIPE\n" | eval "cat <(cat)"      PIPE everywhere
	//	printf "PIPE\n" | cat < <(cat)           PIPE everywhere
	//
	// A compound command's body, a function's body and an `eval`'s program
	// all run after the element's redirections are in place, and a
	// substitution written as a *redirection operand* is expanded with them
	// rather than before them. So the answer reaches one simple command's
	// words and stops there.
	//
	// `>(cmd)` does not observe it: that spelling gives the body the reading
	// end of its own pipe, which replaces whatever it would otherwise have
	// read. The axis is still asked for it through the one place all three
	// spellings are prepared, so the file form `=(cmd)` — which only zsh has
	// — cannot drift away from `<(cmd)`.
	ProcessSubstitutionBodyReadsTheShellsInput Answer

	// ConditionArithmeticErrorIsFatal abandons the input when an operand of a
	// word-spelled comparison — `[[ 1+ -eq 0 ]]` — is not an expression the
	// arithmetic parser can read.
	//
	// The operands themselves are core: every shell in the panel that has
	// `[[ ]]` evaluates them as arithmetic, so `n=5; [[ n -eq 5 ]]` holds in
	// all three and there is nothing to switch on. What they disagree about
	// is the *failure*. Measured from a script file, `echo one; [[ 1+ -eq 0
	// ]]; echo two`:
	//
	//	zsh   	complains, `two` never runs, exit 1
	//	ksh93 	complains, `two` never runs, exit 1
	//	bash  	complains, the condition is false, `two` runs at status 1
	//
	// So two abandon and one carries on, which is a conflict and not a
	// wording difference — a script that guards with `[[ n -eq 0 ]]` over a
	// name it did not set runs to the end under one group and stops at that
	// line under the other.
	//
	// It is asked only on the error path. A condition whose operands read
	// cleanly never reaches it.
	ConditionArithmeticErrorIsFatal Answer

	// ArithCommandErrorStatusIsTwo is what `(( expr ))` leaves behind when
	// the expression could not be evaluated: 2 where this is Yes, 1 where it
	// is No.
	//
	// The sentence is not the question — that is Diagnostics, and the two
	// shells measured here already word it their own way. What differs is
	// what the construct leaves for the next line to read. Measured
	// 2026-09-10, `(( 1+ )); echo $?`:
	//
	//	zsh   	bad math expression: operand expected at end of string, 2
	//	bash  	arithmetic syntax error: operand expected, 1
	//
	// It is the *construct* and not the evaluator: `let "1+"` is 1 in both,
	// and in ksh93 too, so a status hung on the arithmetic error itself
	// would have moved `let` with it. That is the discriminating pair, and
	// it is why this is asked here and nowhere else.
	//
	// The value is a status and not a truth, so it is reached the same way
	// through `if (( 1+ ))` — the condition is false, and the status behind
	// it is the dialect's.
	//
	// Asked only on the error path. An expression that reads cleanly leaves
	// 0 or 1 for its own value, which is unanimous and not a question.
	ArithCommandErrorStatusIsTwo Answer

	// ArithCommandErrorIsFatal abandons the input when `(( expr ))` could not
	// be evaluated, instead of leaving the status above for the next line to
	// read. True in ksh93 alone.
	//
	// Measured 2026-09-10 and again 2026-09-11, `echo one; (( 1+ )); echo
	// two`: bash 5.3, bash 3.2 and zsh all print `two` and end at 0, and
	// ksh93 prints the complaint and nothing after it, ending at 1. So the
	// construct is fatal there and a reporting statement everywhere else,
	// which is a conflict rather than a wording difference — a script that
	// tests a counter with `(( n ))` over a name it did not set runs to the
	// end under one group and stops at that line under the other.
	//
	// The same question for *both* ways the expression can fail, which is
	// what ArithCommandErrorStatusIsTwo already found: `(( 1+ ))` never
	// reaches the evaluator and `(( 1/0 ))` does, and ksh93 abandons the
	// input for both.
	//
	// It is not the same question as the construct standing as a condition:
	// `(( 1+ )) && echo yes` and `if (( 1+ ))` are fatal there too, so this
	// is about the expression and not about what the status is read for.
	//
	// Nor does it group with the word-spelled comparison. This is the other
	// half of the question ConditionArithmeticErrorIsFatal asks about
	// `[[ 1+ -eq 0 ]]`, and the two do not cut the panel the same way: zsh
	// abandons the condition and stays for `(( ))`, which is why a single
	// field could not carry both. See that one for the condition's measured
	// answers.
	//
	// The reach is the ordinary one for a fatal error rather than anything of
	// this construct's: measured, ksh93 gives up a *sourced file* alone —
	// `. ./s.sh; echo after` still prints `after` — and a subshell alone,
	// which is the same boundary its `[[ ]]` failure stops at.
	//
	// Asked only on the error path. An expression that reads cleanly never
	// reaches it.
	ArithCommandErrorIsFatal Answer

	// LetKeepsTheValueBeforeAnIllegalByte leaves `let` with the value its
	// expression had reached when the arithmetic reader met a byte it refuses,
	// instead of leaving it with nothing. True in zsh alone.
	//
	// `let` reports *false* for an expression that came out zero, which is
	// unanimous and not a question — `let "x=5"` is 0 and `let "x=0"` is 1
	// everywhere. What this decides is the value that rule is then applied to.
	// Measured 2026-09-11, zsh 5.9.2 against bash 5.3, bash 3.2 and ksh93:
	//
	//	let '1 @'  	zsh 0, the others 1
	//	let '0 @'  	1 everywhere
	//	let '1+2 @'	zsh 0, the others 1
	//	let '@'    	1 everywhere
	//
	// So it is not "a failure is success there": the value before the byte is
	// what decides, and where nothing stood before it the answer is the same
	// as everybody's.
	//
	// Only the byte the reader refuses outright, which is the discriminating
	// half and the reason this is not a statement about arithmetic failure at
	// large: `let '1+'` and `let '5 5'` are 1 in that shell too, though a
	// value stood before those failures as well. The reader gave up
	// mid-stream in one case and the grammar rejected the whole expression in
	// the others.
	//
	// Asked in `let` and nowhere else, because nowhere else can it be seen:
	// the same text inside `$(( ))` or `(( ))` abandons the line in that shell
	// whatever value stood, at 1 and at 2 respectively. #1191 recorded those
	// three statuses and warned against copying one of them to the others;
	// this is the narrow field that does not.
	LetKeepsTheValueBeforeAnIllegalByte Answer

	// RegexQuotingMakesLiteral treats a quoted right operand of `=~` as a
	// literal string. True in bash alone; ksh93 and zsh keep it a regex, so
	// quoting a regex is unportable in either direction.
	RegexQuotingMakesLiteral Answer

	// LastPipelineElementInCurrentShell runs the last command of a pipeline
	// in this shell, so `echo x | read v` sets v. True in ksh93 and zsh.
	LastPipelineElementInCurrentShell Answer

	// RedirectTargetIsAnOrdinaryWord expands a redirection's target the way
	// an argument is expanded — split into fields and matched as a pattern —
	// and requires the result to be exactly one word. True in bash alone:
	//
	//	e="a b"; echo hi > $e      bash refuses; the rest write to `a b`
	//	e="x*";  echo hi > $e      bash refuses where two files match, and
	//	                           writes to the match where one does; the
	//	                           rest create a file named `x*`
	//
	// The other three expand it and stop there: no splitting, no matching,
	// whatever it came to is the name. A tilde expands either way.
	//
	// Doing bash's expansion and then quietly taking the first field is the
	// answer no shell gives, and it is the one this had: `> $e` wrote to `a`,
	// and `> $e` with a pattern truncated whichever file happened to match.
	RedirectTargetIsAnOrdinaryWord Answer

	// TypePrintsFunctionBody makes `type name` follow "name is a function"
	// with the function itself, reformatted. True in bash alone; the other
	// three stop at the sentence.
	TypePrintsFunctionBody Answer

	// TypeEndsOptionsWithDashDash makes `type -- name` skip the `--`. True
	// in bash, ksh93 and zsh; dash has no options for it at all, so `--` is
	// a name there and gets answered as one before the real names are.
	TypeEndsOptionsWithDashDash Answer

	// TypeNamesTheKindWithDashT gives `type` its `-t`, which answers one
	// bare word per name — keyword, function, builtin or file — and prints
	// nothing at all for a name it cannot account for, only the failing
	// status. The scripted form of the question: a word to compare against
	// rather than a sentence to parse. True in bash alone; ksh93 and zsh
	// refuse the letter the way they refuse any option they do not have,
	// and dash reads it as a name like the rest of its operands.
	//
	// unpinned bash: never reached, so no row could catch it however it
	// was written. The letter is already in bash's `TypeOptions`
	// (`afpPt`), and this axis — which predates the optstring — is
	// consulted only where the optstring does *not* carry a `t`. Measured
	// 2026-09-12 by moving it in the bash dialect: `type -t f`, `type -t
	// while` and `type -t nosuch` answer the same under `Yes`, under `No`
	// and under `Unspecified` alike. The three dialects that do reach it
	// all answer `No`, so the `Yes` this one holds is a value nothing
	// consults — see #2180 (#2057).
	TypeNamesTheKindWithDashT Answer

	// TypeOptions is the rest of `type`'s letters, in the getopts spelling
	// the other optstrings use — `-a` for every resolution a name has, `-p`
	// and `-P` for the path alone, `-f` to leave the functions out. Empty
	// means none beyond what the two axes above already give, which is
	// dash's answer: its `type` has no options at all, and
	// TypeEndsOptionsWithDashDash already says so.
	TypeOptions string

	// TypePSearchesPathPastTheShell is what `type -p` does about a name the
	// shell would answer itself: ksh93 and zsh search PATH anyway and name
	// the file, bash prints nothing at all and reports 0 — its `-p` speaks
	// only when the plain answer would have been a file. Asked only with
	// the letter, so a dialect without it never meets the question.
	TypePSearchesPathPastTheShell Answer

	// TypePathAnswerIsASentence is the shape of `-p`'s answer: zsh words it
	// the way its plain `type` does — `echo is /bin/echo`, and the not-found
	// complaint for a miss — where bash and ksh93 print the bare path and
	// meet a miss with silence and the failing status.
	TypePathAnswerIsASentence Answer

	// TypeFSaysTheFunctionBack turns `-f` around: in zsh the letter *prints*
	// a function — the definition, laid out, nothing else — where bash and
	// ksh93 use it to leave functions out of the search.
	TypeFSaysTheFunctionBack Answer

	// ArraysAreSparse makes an unassigned subscript no element at all, so
	// `a=(x); a[5]=y` is an array of two. True in bash and ksh93; zsh reads
	// the whole extent and finds the gap empty, giving five.
	//
	// The store is sparse either way — only the reading differs — so this is
	// asked when an array *has* a gap and never otherwise, which is almost
	// every array there is.
	ArraysAreSparse Answer

	// OperatorDistributesOverStarSubscript applies an operator written on
	// `${a[*]}` — a trim, a replacement, a case change — to each element
	// before the join, so `${a[*]#a}` on `(aa ab)` is `a b`. True in bash and
	// ksh93; zsh joins first and applies the operator to the joined string
	// once, giving `a ab`.
	//
	// Only the star form is an axis. On `${a[@]}` every shell with arrays
	// applies the operator to each element, and the two readings of `[*]`
	// often agree — a suffix trim that stops at the last element, most
	// patterns that match nothing — so this is asked only when they differ.
	OperatorDistributesOverStarSubscript Answer

	// ExportCarriesFunctions gives `export` its `-f`, which writes a
	// function into a child's environment. True in bash alone: the other
	// three have no way to carry a function at all, and each rejects the
	// option as an option — two of them fatally.
	ExportCarriesFunctions Answer

	// ExportTakesTheAttributeOff gives `export` its `-n`, which takes the
	// export attribute off a name and leaves the name itself alone. True in
	// bash alone; the other three refuse the letter as an option, two of
	// them fatally.
	//
	// The same shape as ExportCarriesFunctions and for the same reason: what
	// the letter *means* is not in question anywhere it exists — the name
	// stays set and stops reaching a child — only whether the dialect has it
	// at all. So there is no wording here, and a dialect that says no sends
	// `-n` down the ordinary unknown-option path to collect its own refusal.
	// Measured 2026-09-05: `dash: 1: export: Illegal option -n` and the
	// script ends, `ksh: export: -n: unknown option` with a usage line and
	// the script ends, `zsh:export:1: bad option: -n` with `export` failing
	// at 1 and the script carrying on.
	//
	// A wording field would be the wrong tool even for the one shell that
	// carries on: unlike `-f`, which zsh knows and refuses in words of its
	// own, `-n` is simply not a letter any of the three has.
	ExportTakesTheAttributeOff Answer

	// AnnouncesBackgroundJob prints the job number and the process id when a
	// job is backgrounded, before the next prompt. True in bash, ksh93 and
	// zsh; dash says nothing at all.
	//
	// Only ever at a prompt: no shell announces one to a script.
	AnnouncesBackgroundJob Answer

	// AnnouncesBackgroundJobWithoutTheMonitor keeps that announcement when
	// the monitor has been turned *off* — `set +m`, `unsetopt monitor` — at a
	// prompt where there is still somebody to tell.
	//
	// Measured 2026-09-10 on a pseudo-terminal, with the monitor off:
	//
	//	bash 5.3.15   [1] <pid>      bash 3.2.57   [1] <pid>
	//	ksh93u+       [1]	<pid>     zsh 5.9.2     nothing
	//	dash          nothing
	//
	// So the start notice is a *second* question and not a consequence of the
	// first: dash answers no to both, zsh announces a job with the monitor on
	// and stops when it is off, and the two shells that carry on announcing
	// are announcing something the option says they are not managing. It is
	// asked only when the monitor is off and there is somebody to tell, which
	// is the one place the two answers differ.
	//
	// **The other end of the job is not an axis.** With the monitor off no
	// shell in the panel says anything when the job *finishes* — measured the
	// same day, against the same jobs — so that is shared ground and
	// FinishedJobNotices simply stays quiet. dash's late report of a finished
	// job with an empty command is its own oddity, measured and not
	// reproduced (#1738).
	AnnouncesBackgroundJobWithoutTheMonitor Answer

	// UnsetFunctionChecksTheName judges the operand `unset -f` was given as
	// a name, and refuses one that could not be a function name. True in
	// ksh93 alone.
	//
	// Not the same question as the one below, and measured to be: ksh93
	// refuses `1x` and is quiet about a well formed name that is not
	// defined, where zsh is the other way round.
	UnsetFunctionChecksTheName Answer

	// UnsetFunctionReportsMissing complains when `unset -f` names a function
	// that is not defined. True in zsh alone, which reports it about any
	// name it does not hold, well formed or not.
	//
	// Unsetting a function that *is* there is quiet in all four.
	UnsetFunctionReportsMissing Answer

	// LoneDashIsAnOption eats a `-` given to a builtin on its own instead of
	// passing it on as an operand. True in zsh alone.
	//
	// Only visible once something looks at the operands. `unset -` is quiet
	// in bash because its bare form validates nothing, not because the dash
	// was eaten — `unset -v -`, which does validate, names the dash there.
	// zsh reports `not enough arguments` instead, because after the dash is
	// eaten there is nothing left to unset. Recorded as
	// `name/a-lone-dash-given-to-a-builtin` and
	// `name/unset-v-validates-the-lone-dash`.
	LoneDashIsAnOption Answer

	// LoopControlOutsideALoopIsFatal ends the script when `break` or
	// `continue` is run with no loop around it, instead of reporting it (or
	// not) and running the next command. True in zsh alone.
	//
	// Measured 2026-09-10, `-c`, with `echo t; break; echo after`: dash,
	// bash 5.3, bash called as `sh` and bash 3.2 all print `after` and end
	// at 0, and zsh prints neither `after` nor anything after it on a later
	// *line* either — so it is the script that stops and not the line, which
	// is why this reaches fatalQuiet rather than controlAbandon. The status
	// is then the dialect's own for a fatal error, which is 1 there.
	//
	// The question is only about the misuse. A `break` with a loop around it
	// is ordinary control flow in every shell in the panel, and the count it
	// is asked against is the dynamic one a cloned Runner carries with it —
	// which of those loops the word can actually see is the pair of axes
	// below.
	//
	// Separate from the wording, because the two questions cut the panel
	// differently: bash reports and carries on, zsh reports and stops, and
	// dash says nothing and carries on. One field could not express the
	// first of those three — see Diagnostics.LoopControlOutsideALoop.
	LoopControlOutsideALoopIsFatal Answer

	// FunctionCallIsALoopControlBoundary stops a `break` or `continue` in a
	// function body from reaching the loops the *caller* is inside.
	//
	// Measured 2026-09-11, `f(){ break; }; for i in 1 2; do f; echo body;
	// done; echo after`:
	//
	//	bash 3.2, zsh                  	`after` — the loop ended
	//	dash, ksh93, bash called as sh 	`body body after` — it did not
	//	bash 5.3                       	`body body after`, and the
	//	                               	complaint twice
	//
	// bash 5.3's complaint is not a third answer: a boundary leaves the word
	// with no loop at all, which is the misuse above, and the two rows differ
	// over saying so exactly as Diagnostics.LoopControlOutsideALoop does.
	// bash 3.2 is the same family answering the opposite way, which is what
	// says this is a decision and not a consequence of something else.
	//
	// The reach is a count and not a flag: `break 2` from a body with one
	// loop in it stops at that loop wherever this is Yes, and reaches the
	// caller's wherever it is No.
	//
	// Asked only where the answer decides something — a `break` that can see
	// a loop inside the call never reaches this.
	FunctionCallIsALoopControlBoundary Answer

	// SubshellIsALoopControlBoundary is the same question for `( )`, and it
	// is a second field because the panel does not group the two.
	//
	// Measured 2026-09-11, `for i in 1 2; do ( break; echo insub ); echo
	// body; done; echo after`:
	//
	//	bash 3.2, dash, ksh93, zsh	`body body after` — the subshell was
	//	                          	left, quietly
	//	bash 5.3, bash as sh      	`insub body insub body after`
	//
	// So bash 5.3 makes both boundaries and dash and ksh93 make only the
	// call one; a single field would have to give one group the other's
	// answer. Note bash called as `sh` parts from bash 5.3 on the *call* and
	// agrees with it here, which is a second reason the two cannot share.
	//
	// The parentheses and nothing else. A command substitution and a
	// pipeline element are subshells too, and the column that makes `( )` a
	// boundary makes neither of them one: `for i in 1 2; do x=$( break );
	// done` and `do break | cat; done` draw no complaint from bash 5.3 where
	// the parenthesized form draws one per pass.
	SubshellIsALoopControlBoundary Answer

	// ReturnOutsideAFunctionIsRefused reports a `return` that has nothing to
	// return from and carries on, instead of ending the script with the
	// status it was given. True in bash alone.
	//
	// Asked only where there is nothing to return from. Inside a function
	// and inside a sourced file all four obey it, so the question is about
	// the one case they split on.
	ReturnOutsideAFunctionIsRefused Answer

	// StartupFileReturnCarriesItsArgument makes `return 3` at the top of a
	// startup file leave `$?` as 3, instead of leaving whatever the command
	// before it left. True in dash, ksh93 and zsh; false in bash.
	//
	// A startup file *is* a sourced script — every shell in the panel accepts
	// a `return` in one, stops reading the file there and says nothing — so
	// the question is only what the argument does. Measured through a pty
	// with the rc file as the whole probe, reading `$?` at the first prompt:
	//
	//	rc              bash  bash32  dash  ksh93  zsh
	//	return 3           0       0     3      3    3
	//	false; return 3    1       1     3      3    3
	//	false; return      1       1     1      1    1
	//	(exit 5)           5       5     5      5    5
	//
	// The last two rows are what make this about the argument and nothing
	// else. bash does carry a startup file's status out — `(exit 5)` leaves
	// 5 — and a `return` with no argument means the last command's status
	// everywhere, so the only thing bash discards is the number written on
	// the `return` itself.
	//
	// Asked only of a `return` at the top level of the startup file. A
	// `return` inside a function the file calls, or inside a file the file
	// sources, carries its argument in bash too: measured, an rc running
	// `f(){ return 3; }; f` or `. inner.sh` where inner returns 3 leaves 3 at
	// the prompt in bash 5.3.15. So this is a property of the outermost
	// frame rather than of `return`.
	StartupFileReturnCarriesItsArgument Answer

	// UnknownConditionOptionIsAStatus makes `[[ -o name ]]` with a name this
	// shell does not have a status of its own with a complaint, instead of
	// the plain false that a name it has but has not set would give. True in
	// zsh alone; bash and ksh93 answer 1 and say nothing, and dash has no
	// `[[ ]]` to ask it in.
	//
	// Asked only where the shells disagree, which is at a name none of them
	// would recognize. A name this shell has is read the same way in all
	// three and nothing is asked.
	//
	// Not the same question as BadSetOptionNameFatal, and measured rather
	// than assumed to be: the name that ends a zsh script when `set -o` is
	// given it leaves `[[ ]]` running, with the complaint said and the next
	// command reached. One construct's refusal is not the other's.
	//
	// The status is a third value rather than a false, which the combining
	// operators show: `[[ ! -o zzz ]]` is 3 and not 0, so `!` leaves it
	// alone, and `[[ -o zzz || 1 == 1 ]]` is 0, so `||` goes on past it the
	// way it would past a false. Measured across the whole truth table on
	// zsh 5.9.2.
	UnknownConditionOptionIsAStatus Answer

	// BadSetOptionNameFatal ends the script when `set -o` is given a name
	// this shell does not have. True in dash, ksh93 and zsh.
	//
	// Not the same question as BadOptionToSpecialBuiltinFatal, and measured
	// rather than assumed to be: a bad option *letter* to the same builtin
	// is fatal in only two of them, and zsh does not so much as complain
	// about `set -Q`. So one shell treats an unknown name as worse than an
	// unknown letter, which is why this is a field of its own.
	BadSetOptionNameFatal Answer

	// CdLastPathOptionWins lets the last of `cd -L` and `cd -P` decide.
	// True in bash, dash and ksh93 — `cd -P -L` is logical there. zsh gives
	// `-P` the answer wherever it appears, so both orders resolve.
	//
	// Asked only when both were given, because that is the only time the
	// two rules differ.
	CdLastPathOptionWins Answer

	// CdRefusesUnknownOption refuses a letter `cd` does not have rather than
	// reading the word as a directory. True in bash, dash and ksh93; zsh
	// looks for somewhere called `-Q` instead, because its `cd` takes two
	// operands — `cd old new` — and a leading dash word is the first of
	// them there.
	//
	// Only about an *unknown* letter. `-L` and `-P` are options in all four
	// and are not asked about.
	CdRefusesUnknownOption Answer

	// CdHasQuietOption gives `cd` the `-q` of zsh, which is the one letter
	// beyond `-L` and `-P` that any of the panel has. True in zsh alone.
	//
	// What the letter means there is *hook suppression*: measured 2026-09-08,
	// a `chpwd` function and a `chpwd_functions` entry both ran on a plain
	// `cd` and neither ran on `cd -q`. It is not about printing — `cd -q -`
	// still wrote the directory at an interactive prompt, and a CDPATH move
	// stayed silent with the letter and without it — so a shell that fires no
	// `chpwd` has already done everything `-q` asks for.
	//
	// So the letter is carried to that site rather than swallowed at the
	// option loop: `cd` fires DirectoryChangeHook and `cd -q` does not,
	// measured for the named function and for a `chpwd_functions` member
	// alike, and measured again for `pushd -q` and `popd -q`, which move
	// through `cd` and are quiet for the same reason. It is a *letter* and
	// not an operand, which is the other half of why it is here — without it
	// `cd -q /tmp` went looking for a directory called `-q`, which is #1558.
	//
	// Asked only when a `q` is actually seen, so the three shells without the
	// letter never reach the question and answer the word the way they answer
	// any other letter they do not have — see CdRefusesUnknownOption, which
	// is the next question when this one says no.
	CdHasQuietOption Answer

	// HookListSuffix is what a hook's list of *extra* function names is
	// spelled by: the hook's own name plus this. zsh's is `_functions`, so
	// `precmd` reads `precmd_functions` as well and `chpwd` reads
	// `chpwd_functions`. Empty is a shell whose hooks are the named function
	// and nothing else, which is three of the four — and, since those three
	// have no hooks at all, is really "no hooks" said once.
	//
	// Not decoration: `add-zsh-hook precmd f` defines no function called
	// `precmd`, it appends `f` to `precmd_functions`, so a shell that read
	// only the named function would find a correctly registered hook and run
	// nothing. That was #1281.
	//
	// Here rather than on repl.HookStyle, where it began, because the hook
	// *sites* are on both sides of that line: `precmd` fires in a prompt loop
	// and `chpwd` fires inside `cd`, which is a builtin and cannot reach up
	// into a front end. One home for the suffix, one [Runner.HookChain] that
	// applies it, and no way for the two sites to come to disagree about what
	// a hook's list is called.
	HookListSuffix string

	// DirectoryChangeHook names the function this shell runs after `cd` has
	// moved it — zsh's `chpwd`. Empty is a shell without one, which is three
	// of the four: measured 2026-09-10, a `chpwd` function defined in bash
	// 5.3.15, bash 3.2.57, bash-as-sh, dash and ksh93 ran on none of their
	// `cd`s and none of them said anything about it.
	//
	// It is the *last* thing `cd` does, after the directory has moved and
	// after anything `cd` itself prints. Measured at a zsh prompt: `cd -`
	// wrote `/usr` and *then* the hook's marker, and a CDPATH move wrote the
	// directory it found and then the marker. So a hook cannot land in the
	// middle of `cd`'s own output.
	//
	// **On the move, not on the change.** `cd` to the directory the shell is
	// already in fires it — measured, `cd /usr` twice in a row fired it
	// twice, with `$PWD` and `$OLDPWD` both `/usr`. A `cd` that *fails* does
	// not: `cd /nope-nope` reported its error, left `$OLDPWD` alone and ran
	// nothing.
	//
	// `$PWD` is where the shell now is and `$OLDPWD` where it was, both
	// already set when the hook runs, and the hook is told **no arguments** —
	// `$#` is 0 in the named function and in every member of the list.
	//
	// Everything that moves through `cd` fires it and nothing else does.
	// `pushd`, `popd` and a bare directory name under `autocd` are `cd` here
	// and in zsh both, and all three fired it; assigning to `PWD` is not a
	// move and fired nothing. A `cd` inside a function fires it at the `cd`,
	// and a `cd` inside a subshell or a command substitution fires it in
	// there, where the move is.
	//
	// A hook that itself calls `cd` fires the hook again, and zsh has no
	// guard against that beyond its ordinary recursion limit: a pair of
	// hooks moving back and forth ended with `chpwd: job table full or
	// recursion limit exceeded`. Nothing special is done here either — the
	// call goes through [Runner.CallFunction] and meets whatever limit an
	// ordinary function call meets.
	DirectoryChangeHook string

	// ExitHook names the function this shell runs on the way out — zsh's
	// `zshexit`. Empty is a shell without one, which is three of the four:
	// measured 2026-09-12, a `zshexit` function defined in bash 5.3.15,
	// bash-as-sh, bash 3.2.57, dash and ksh93 ran on none of their exits and
	// none of them said anything about it.
	//
	// **After the EXIT trap, not before it.** Measured against zsh 5.9.2, a
	// script with both wrote the trap's line and then the hook's — and an
	// interactive session left with `exit` or with end-of-input wrote them
	// in that same order. So the trap is the script's last word and the hook
	// is the shell's, which is the order a plugin's teardown is written
	// against: gitstatus registers `_gitstatus_cleanup_…` here to stop the
	// daemon it started, and powerlevel10k's async worker registers
	// `_p9k_worker_cleanup`.
	//
	// The hook is told **no arguments** — `$#` is 0 in the named function and
	// in every member of the list — and every one of them is told the status
	// the shell is exiting with. That is the entry status and not a running
	// one: with the shell exiting 4, a named hook that returned 5 and a
	// member that ran `false` were both followed by a member reading `$?` as
	// 4.
	//
	// **`return` cannot change the status and `exit` can.** `zshexit`
	// returning 5 left a shell exiting 4 exiting 4. `exit 9` in the named
	// hook and `exit 11` in a member left it exiting 11 — the *last* `exit`
	// wins — and, unlike every other chain in this shell, an item that exited
	// did **not** stop the ones after it: the member after `exit 9` still
	// ran. There is no session left for `exit` to end, so all it can do is
	// record a status. See Runner.runExitHook, which is where that one
	// difference from FireChain's rules lives.
	//
	// **Not on a signal death.** A script killed by SIGTERM ran neither its
	// EXIT trap nor its `zshexit`, which is the same two-two split
	// ExitTrapRunsOnSignalDeath records for the trap — and since zsh is the
	// only shell in the panel with the hook at all, there is no disagreement
	// to make an axis of.
	//
	// **The subshell case is not this site.** A subshell that calls `exit`
	// explicitly fires the hook in there — measured, `(exit 7)` ran
	// `zshexit` with `$ZSH_SUBSHELL` of 1 — while a subshell that merely
	// falls off its end does not. That is a firing at a subshell's own exit
	// and not at the shell's, and this shell's subshells do not pass through
	// Finish at all, so it is written down here rather than modeled: nothing
	// reaches it, and a guess about it would be a plausible wrong answer.
	ExitHook string

	// ChildInterruptEndsTheScript stops the script when a child was ended by
	// an interrupt, instead of carrying on with the next command. True in
	// ksh93 alone, and for SIGINT alone — measured across QUIT, TERM, HUP,
	// USR1 and PIPE, every one of which it carries on from.
	//
	// It ends the whole script rather than the construct around it: from
	// inside a loop, the loop and everything after it are abandoned too.
	// The status is 128 plus the signal, which is not the same shell's
	// answer for a command killed by one — that is 256 plus it.
	ChildInterruptEndsTheScript Answer

	// ReportsAnyKilledPipelineElement remarks on a signal that ended an
	// element of a pipeline other than the last. True in dash alone.
	//
	// bash and ksh93 report only the element whose status the pipeline
	// takes: `sh -c 'kill -ABRT $$' | cat` is silent in both, and the same
	// command as the *last* element is not. dash says the same thing
	// wherever the element stands.
	//
	// Unreachable in zsh, which says nothing about a killed command at all,
	// so the question never arises there.
	ReportsAnyKilledPipelineElement Answer

	// ReportsACommandKilledBySignal says out loud that a signal ended a
	// command, rather than leaving the status to carry it alone. True in
	// bash, dash and ksh93; zsh says nothing — measured with a terminal as
	// well as without one, so it is not the prompt-only rule that governs a
	// background job's announcement.
	//
	// Not asked for the two signals nothing reports. ^C and a broken pipe
	// are how a command is meant to end, and all four stay quiet about
	// those, so there is no disagreement there to put to a dialect.
	ReportsACommandKilledBySignal Answer

	// JobsShowBackgroundCommand puts the command of a `&` job in a `jobs`
	// listing. True in bash and zsh; dash prints an empty column there and
	// ksh93 a placeholder.
	//
	// Only for a `&` job, which is the whole reason this is not a question
	// about rendering a command at all: both of the shells that leave it out
	// here *do* print the command of a job they stopped themselves. They
	// kept nothing for this kind of job, and the listing is where that shows.
	JobsShowBackgroundCommand Answer

	// JobsListNewestFirst puts the most recent job at the top of a `jobs`
	// listing. True in dash and ksh93; bash and zsh list oldest first.
	//
	// A two-two split, which is the usual shape here and the reason this is
	// a field rather than a choice: there is no ordering of the shells that
	// explains it.
	JobsListNewestFirst Answer

	// JobsListFinishedJobs includes a job that has already ended in a `jobs`
	// listing, once, before forgetting it. True in bash, dash and ksh93; zsh
	// drops a finished job without ever mentioning it.
	//
	// The forgetting is not the axis and is not optional: every shell in the
	// panel reports a finished job at most once, so a second `jobs` shows
	// nothing. A shell that kept them would grow a listing for the length of
	// the session.
	JobsListFinishedJobs Answer

	// SetReportsEveryBadOption makes `set` report every option word it
	// cannot use before it gives up, rather than stopping at the first.
	//
	// True in ksh93 alone, which is why it went unnoticed: three of the five
	// columns print one line because the refusal is *fatal* there and the
	// loop never reaches the second word, and the fourth prints one because
	// it stops too. ksh93's refusal is fatal as well and it still prints
	// every line first.
	//
	// Measured 2026-09-06 and again 2026-09-07, over a script file and over
	// `-c`, which answer alike:
	//
	//	set -q -z        two lines, then one usage block, status 2
	//	set -q -z -y     three lines, then one usage block
	//	set -qz          two lines — a bundle is one word and several letters
	//	set -q -o nosuch -z   three lines, in the order written
	//
	// So the unit is the **letter**, not the word: a bundle is not one
	// refusal, which was the open question. Long names count too and
	// interleave with letters in argument order.
	//
	// The usage block is printed **once**, after all of them, and not one
	// per word — which is what makes this more than "keep looping", since
	// the block is written by the same helper that writes each sentence.
	// The fatality is applied after them as well, so it ends the script
	// *after* the reports rather than instead of them.
	//
	// This is the rule Runner.builtinNames already follows for bad
	// *operands*, where bash is the shell that reports each one. Two
	// different shells answer yes to the two questions, which is what keeps
	// them separate fields: bash reports every bad name to `export` and
	// stops at the first bad option to `set`, and ksh93 does the reverse.
	//
	// Asked for the builtin only. The front end's own option parse is a
	// different surface with its own measured quirks — ksh93 answers
	// `ksh -q -z` with a spurious `- : unknown option` between the two, and
	// folds the whole of the rest of argv into a `-o` complaint — so it
	// keeps stopping at the first until those are settled.
	SetReportsEveryBadOption Answer

	// ShiftPastEndFatal ends a non-interactive shell when `shift` runs off
	// the end. True in dash and ksh93.
	ShiftPastEndFatal Answer
	// ReadonlyReassignmentByDeclarationFatal ends the script when a
	// declaration utility assigns to a readonly name — `export x=2`,
	// `typeset x=2`. True in dash, ksh93 and zsh; bash reports it and
	// carries on.
	//
	// A different set of shells from the plain assignment above, which is
	// what makes it a question of its own: bash stops for `x=2` given as an
	// argument and never stops for this one.
	ReadonlyReassignmentByDeclarationFatal Answer

	// SetArrayBadNameLeavesZeroFromCommandString makes `set -A` refuse a
	// name that is not one and leave the shell exiting **0**, where the same
	// refusal from a script file leaves 1.
	//
	// True in zsh, false in ksh93, and unreachable in the two shells without
	// the letter — SetArrayLetter is read rather than asked, so a dialect
	// whose `set` has no `-A` never arrives here.
	//
	// Measured 2026-09-06 and again 2026-09-07 with `>/dev/null 2>&1`, so
	// the number is the shell's own. The stderr is byte-identical on both
	// routes — `<shell>:1: not an identifier: 1bad` — and `after` runs on
	// neither, so the refusal is fatal either way and only the status moves.
	//
	// It is this refusal and no other, which is what makes it a field of its
	// own rather than a route rule about bad names or about `set`. Every
	// neighbor was measured from `-c` in zsh and every one of them leaves 1:
	//
	//	set -A 1bad v      0      set -q             1
	//	set +A 1bad v      0      set -o nosuch      1
	//	set -A 1bad        0      unset 1x           1
	//	set -A a-b v       0      export 1bad        1
	//	set -eA 1bad v     0      typeset 1bad       1
	//
	// And it is a flat 0 rather than the previous command's status:
	// `-c 'false; set -A 1bad v'` exits 0 too.
	SetArrayBadNameLeavesZeroFromCommandString Answer

	// FailedExpansionAbandonsTheLine ends the *line* a failed expansion
	// happened on and carries on at the next one, rather than ending the
	// shell. A bad substitution, a division by zero, a bad subscript and an
	// arithmetic expression the parser refused are all this failure.
	//
	// True in bash and false in dash, ksh93 and zsh. Measured 2026-09-07
	// over both invocation routes and both statement separators, which is
	// the pairing this axis has to be measured on and the reason it was
	// missed:
	//
	//	echo pre; echo "${(P)x}"; echo after     no `after`, status 1
	//	echo pre / echo "${(P)x}" / echo after   `after` runs, status 0
	//
	// The same 2x2 by `-c` and by a script file, so the *route* has nothing
	// to do with it. What ends is the command list, and a list ends at a
	// newline — so `;` between the two commands puts them in the same unit
	// and a newline does not. Every enclosing shape gives up the same way and
	// the shell carries on at the next top-level statement: measured in a
	// loop, a function body, an `if`, a group and a sourced file, where the
	// commands after the `.` still run.
	//
	// It is controlAbandon, which is exactly this and was already in the
	// tree for a readonly reassignment. Before this axis existed the failure
	// was controlExit in every dialect, so one unreadable expansion ended
	// the whole file — which is the shape that makes a diagnostic useless,
	// since the point of naming a construct is that the next line still runs
	// and the next gate becomes visible.
	//
	// Not the two parameter failures that look like it. `set -u` on an unset
	// name and `${x?word}` end the *shell* in all four, by both routes and
	// with either separator, so they are fatalExpansion's and stay there.
	//
	// The core leaves it unanswered: one shell against three is a
	// disagreement, and this path already asks an unanswered axis there —
	// FatalErrorStatusIsOne — so a core run says which dialect it needs
	// rather than picking one.
	FailedExpansionAbandonsTheLine Answer

	// AssignThroughExpansionMayNameAPositional lets `${1:=word}` assign to a
	// positional parameter. zsh alone, and it is a real disagreement rather
	// than a wording one — the other five refuse the expansion fatally.
	//
	// Measured 2026-09-11 on the six columns, after `set --` so the
	// conditional fires:
	//
	//	probe          bash 5.3 / as-sh / 3.2      dash                 ksh93                        zsh 5.9.2
	//	${@:=abc}      $@: cannot assign this way  @: bad variable name ${@:=abc}: bad substitution  not an identifier: @
	//	${*:=abc}      the same with *             the same with *      the same                     not an identifier: *
	//	${1:=abc}      $1: cannot assign this way  1: bad variable name ${1:=abc}: bad substitution  assigns, `abc`, status 0
	//
	// So `@` and `*` are refused unanimously and are not this axis; only the
	// positional splits, and it splits five to one. `${2:=abc}` and
	// `${10:=abc}` answer with their own row, so it is the *shape* of the
	// name and not the number.
	//
	// Yes in zsh, No everywhere else, and the core leaves it unanswered: a
	// shell that has chosen nothing is told which dialect it needs rather
	// than being given one shell's reading of an operator every dialect has.
	//
	// It is a run-time question and is asked only when the operator fires:
	// `set -- p; ${@:=abc}` is `p` at status 0 in all six, and
	// `if false; then echo ${@:=abc}; fi` is silent in all six.
	AssignThroughExpansionMayNameAPositional Answer

	// ReadonlyReassignmentFatal ends the script when a readonly variable is
	// assigned. True everywhere but bash, measured with a plain assignment in
	// a script file — adding a redirect makes it a command and reverses the
	// answer, which is the contaminated-probe trap docs/spec/oracle.md
	// records.
	ReadonlyReassignmentFatal Answer

	// DeclarationMayShadowAReadonly lets a declaration inside a function
	// make a local of a name the shell has frozen.
	//
	// Measured 2026-09-06, `env -i PATH=/usr/bin:/bin` with a scratch HOME,
	// ZDOTDIR and HISTFILE, over a script file:
	//
	//	typeset -r x=1
	//	f() { local x=2; echo "in=[$x]"; echo running; }
	//	f; echo "st=$? out=[$x]"
	//
	// zsh answers `in=[2]`, `running`, `st=0 out=[1]` — the local shadows
	// the frozen name, the shadow is an ordinary local, and the outer value
	// is untouched when the function returns. bash answers `local: x:
	// readonly variable`, then `in=[1]` and `running` — it refuses the
	// declaration, leaves the *outer* value in view, and **carries on**.
	// All four members answer, each asked in the words it has — which is
	// what makes this a four-shell question rather than the two-shell one it
	// looks like from `typeset -r` and `local` alone (#1168):
	//
	//	zsh    typeset -r x=1; f() { local x=2; …; }; f      → in=[2]
	//	bash   the same three words                          → refused
	//	ksh93  typeset -r x=1; function f { typeset x=2; }   → in=[2]
	//	dash   readonly x=1; f() { local x=2; …; }; f        → refused
	//
	// ksh93 has no `local` and answers through `typeset` in a keyword
	// function, which is a local there — see
	// TypesetLocalNeedsKeywordFunction. Asked through `f() { … }` instead it
	// has no scope to shadow into and the declaration is the ordinary
	// refusal, which is that field and ReadonlyReassignmentFatal rather than
	// this one; reading that fatality as this axis's answer had ksh93 down
	// as a `No`. dash has no `typeset -r` and answers through `readonly`,
	// which is the freeze POSIX spells.
	//
	// Two each way, so it stays an axis — and a different split from
	// ReadonlyAttributeCanBeRemoved below, where ksh93 crosses to bash's
	// side. That the two questions divide the panel differently is what
	// makes them two questions.
	//
	// It is one field for the whole family and not one per spelling: zsh
	// takes `local x=2`, `local x`, `typeset x=3`, `local -r x=4` and
	// `local y=1 x=5 z=2` alike, and bash refuses every one of them and
	// reports 1 from the builtin each time. Splitting them would have been
	// five fields whose answers can only ever agree.
	//
	// Where the answer is **no**, three things follow and all three were
	// wrong here. The refusal names the builtin — `local: x: readonly
	// variable`, which is ReadonlyVariableInDeclaration and the reason
	// ReadonlyRefusalNamesBuiltin has entries for the declaration words. The
	// builtin reports 1 and the *function* runs on, so `local x=2 || …`
	// fires its right-hand side and the next line still runs. And the
	// remaining operands are still declared: bash's `local y=1 x=5 z=2`
	// leaves `y` and `z` local and only `x` refused.
	//
	// Where it is **yes** the shadow takes the attribute with it: the local
	// cell is writable and the outer name is frozen again when the function
	// returns. Asked only when a declaration meets a name that is already
	// frozen, so nothing else reaches the question.
	DeclarationMayShadowAReadonly Answer

	// ReadonlyAttributeCanBeRemoved lets a plus form take the readonly
	// attribute off a name — `typeset +r x` — leaving it writable again.
	//
	// Measured 2026-09-07, `env -i PATH=/usr/bin:/bin` with a scratch HOME,
	// ZDOTDIR and HISTFILE, over a script file:
	//
	//	typeset -r s=1; typeset +r s; s=9; echo "st=$? s=[$s]"
	//
	//	zsh    silent, status 0, then s=[9] — the attribute is gone
	//	bash   typeset: s: readonly variable, status 1, and carries on
	//	ksh93  typeset: s: is read only, and the script ends
	//	dash   no `typeset` or `declare`, so nothing here can ask
	//
	// `declare +r` is the same word under its other spelling wherever both
	// exist, and zsh's `export +r` is too — `export` is `typeset -gx` there.
	// `readonly +r` is not: that builtin takes no `r` in any shell, since
	// the attribute is the whole of what it means.
	//
	// zsh alone allows it, so this splits the panel differently from
	// DeclarationMayShadowAReadonly above, where ksh93 is on zsh's side.
	// Two questions rather than one, and the ksh93 row is what proves it.
	//
	// A shell that says no still has to say *which* no, and it already
	// does: the refusal is the ordinary readonly refusal through a
	// declaration, so the wording and the fatality come from
	// ReadonlyReassignmentByDeclarationFatal and the diagnostics beside it
	// rather than from anything of this field's own. That is measured and
	// not an economy — ksh93 ends the script over `typeset +r` exactly as it
	// ends one over `export x=2`, and bash carries on from both.
	//
	// Asked only for a plus form on a name that is *already* frozen. A
	// `typeset +r` on a free name reports 0 and says nothing in all three
	// shells that spell it, which is the shape a script actually writes —
	// making sure a name is writable — and it must not reach an axis.
	//
	// zsh's yes has one limit that is not an axis: a *special* parameter
	// refuses the change there whatever this says — `typeset +r
	// EPOCHSECONDS` is `can't change type of a special parameter` — which is
	// a fact about specials rather than about the attribute.
	ReadonlyAttributeCanBeRemoved Answer

	// DeclaredNameWithoutValueIsEmpty gives a name a value when it is
	// declared without one: `local u` or `typeset u`. zsh alone says yes, so
	// `${u-UNSET}` is empty there and UNSET in bash and ksh93 — the name
	// exists in all three, but only zsh considers it set.
	DeclaredNameWithoutValueIsEmpty Answer
	// ExportLetterDeclaresAGlobal makes the `x` letter on a declaration ask
	// for `-g` as well, so `typeset -x v=1` written inside a function
	// declares no local and the name outlives the call.
	//
	// Measured 2026-09-10, `-f` / `--norc --noprofile`:
	//
	//	f(){ typeset -x lxx=1; }; f; echo "[$lxx]"
	//
	//	zsh 5.9.2    [1]   the name is global and exported
	//	bash 5.3.15  []    an ordinary local
	//
	// `local -x` is the control and both shells agree on it — `[]` — so the
	// question is about the letter under the *other* words and not about
	// `-x` in general. The same holds for `declare`, and for the words that
	// carry an attribute in their own name: `readonly -x`, `integer -x` and
	// `float -x` all reach past the function in the shell that answers yes.
	//
	// The exemption is measured too: a name this scope has *already* made
	// local stays local, so `f(){ local m=1; typeset -x m; }` leaves the
	// caller's m alone. So the letter decides where a declaration lands
	// rather than what it does to a name that is already here.
	//
	// Asked only inside a function, only under a word that is not `local`,
	// and only where the name is not already local — outside that shape the
	// two answers do the same thing.
	//
	// Silent: a script that exports a working name inside a function leaves
	// it behind in one shell and not in the other, and nothing is said
	// either way.
	ExportLetterDeclaresAGlobal Answer
	// ValuelessDeclarationOfAHeldNameListsIt writes the name back when a
	// declaration names it, assigns nothing, and carries no letters at all —
	// provided the name already holds something in the cell being declared.
	//
	// Measured 2026-09-10, `-f` / `--norc --noprofile`:
	//
	//	a=(x y); typeset a; s=str; typeset s; unset u; typeset u; echo done
	//
	//	zsh 5.9.2    a=( x y ) · s=str · done
	//	bash 5.3.15  done
	//
	// Three facts in the one line, and each is a limit on the rule rather
	// than a special case. A name holding **nothing** prints nothing, which
	// is why this is not "a declaration with one operand lists". The value
	// is unchanged either way, so the listing is all that happens. And the
	// spelling is the bare assignment — `a=( x y )`, not `typeset -a a=…` —
	// which is BareDeclarationListing's row and not `-p`'s.
	//
	// A **letter** suppresses it: `n=5; typeset -i n` and `typeset -g s` are
	// both silent in the shell that lists, which is what makes the rule "no
	// options at all" and also why `readonly`, `export`, `integer` and
	// `float` never do it — each of those words is an attribute already.
	// `local` does, on a name its own scope has already declared:
	// `f(){ local s=1; local s; }` writes `s=1` there.
	//
	// Inside a function a declaration that takes a *fresh* cell finds
	// nothing standing in it, so nothing is listed — which is the answer
	// that keeps a shell from narrating every `local` in every function.
	//
	// We are the quiet one where this is answered wrongly, so a script whose
	// output matches today diverges the moment it is run under the shell
	// that speaks.
	ValuelessDeclarationOfAHeldNameListsIt Answer
	// ScalarOverACompoundIsAnInconsistentType refuses a declaration that
	// assigns a plain word to a name whose cell is really holding an array
	// or a keyed table, and ends the script over it.
	//
	// Measured 2026-09-10, `-f` / `--norc --noprofile`:
	//
	//	b=(x y); typeset b=q; echo "st=$? [${b[*]}]"; echo tail
	//
	//	zsh 5.9.2    typeset: b: inconsistent type for assignment, status 1,
	//	             and the script ends
	//	bash 5.3.15  st=0 [q] · tail
	//
	// It is the **declaration** that refuses and not the store: `b=(x y);
	// b=q` is taken in both shells and leaves a scalar, which is
	// ScalarAssignedOverACompoundReplacesTheName's question and a different
	// one. Nor is it the fresh cell — `f(){ local b=q; }` over a caller's
	// array is taken in the shell that refuses, because the cell that
	// declaration writes is new and holds nothing. What is refused is a
	// declaration reaching a cell that is *really* compound, which is what
	// the top level and `-g` have in common.
	//
	// The refusal has a direction, and that asymmetry is the measurement
	// that pins it: the mirror image is **taken**. `b=1; typeset -a b` is a
	// silent empty array in the same shell, so a name is not simply frozen
	// in its kind.
	//
	// `readonly b=q` and `export b=q` refuse it in the same words with their
	// own name in the sentence, so the rule belongs to the declaration
	// utilities rather than to one word.
	//
	// Silent where it is answered wrongly, and worse than a wrong value: the
	// script that should have stopped carries on, so everything downstream
	// of the line runs here and never runs there.
	ScalarOverACompoundIsAnInconsistentType Answer
	// ReadonlyRecordsTheCompoundAttribute makes `readonly -a` and
	// `readonly -A` declare an array and a table the way `typeset -a` and
	// `typeset -A` do, rather than freezing a name and saying nothing about
	// its kind.
	//
	// Measured 2026-09-10:
	//
	//	f() { readonly -a a; typeset -p a; }; f
	//
	//	zsh 5.9.2    typeset -ar a=(  )
	//	bash 5.3.15  declare -r a
	//
	// The keyed half moves with it — `typeset -Ar m=( )` against
	// `declare -r m` — so it is one question and not two. ksh93 has no such
	// letter on the word at all and refuses the option, so the panel that
	// answers this is two shells and they disagree.
	//
	// Only the listing observes it in the shell that says no: a frozen name
	// cannot then be assigned an array to tell the two apart.
	ReadonlyRecordsTheCompoundAttribute Answer
	// TypeLetterAndAnArrayLiteralIsAnInconsistentType refuses a declaration
	// that names a *type* — the integer or the float letter — and assigns an
	// array literal to the same name, and ends the script over it.
	//
	// Measured 2026-09-12, from a script file with `env -i` and a scratch
	// HOME:
	//
	//	typeset -ia z=(1 2); echo "st=$?"; typeset -p z; echo tail
	//
	//	zsh 5.9.2    typeset: z: inconsistent type for assignment, status 1,
	//	             and the script ends
	//	bash 5.3.15  st=0 · declare -ai z=([0]="1" [1]="2") · tail
	//	ksh93u+      st=0 · typeset -a -i z=(1 2) · tail
	//
	// The array letter is not what triggers it, which is the measurement
	// that says this is about the *type* and not about a pairing: `typeset
	// -i z=(1 2)` with no `-a` is the same refusal, `typeset -F 3 z=(1 2)`
	// and `typeset -E 3 z=(1 2)` are too, and `typeset -ua q=(ab cd)` is
	// taken and lists as `typeset -au q=( ab cd )`. The case letters name
	// what happens *to* a value and the numeric ones name what the value
	// *is*, so only the second kind is two things at once with an array.
	// The width letters side with the case ones — `typeset -Z 4 z=(1 2)`
	// lists as `typeset -aZ4 z=( 1 2 )` and `typeset -L 4 z=(ab cd)` as
	// `typeset -aL4 z=( ab cd )`.
	//
	// It is the **letter on this line** and not the attribute the name is
	// carrying, which is the second discriminating row: `typeset -i z;
	// typeset z=(1 2)` is taken in the same shell and leaves `typeset -a z=(
	// 1 2 )`, the integer letter simply lost. So the question is asked of a
	// declaration and never of a store.
	//
	// All four declaration utilities refuse it, each naming itself:
	// `readonly -i z=(1 2)` is `readonly: z: inconsistent type for
	// assignment`, `export -i z=(1 2)` is `export:`, and `local -i z=(1 2)`
	// inside a function is `f:local:`. The wording is
	// Diagnostics.InconsistentType, shared with
	// ScalarOverACompoundIsAnInconsistentType — one sentence, two questions
	// that reach it.
	//
	// Silent where it is answered wrongly, and in the worse direction: the
	// script that should have stopped carries on holding an array of the
	// type it was refused.
	TypeLetterAndAnArrayLiteralIsAnInconsistentType Answer
	// NumericTypeWithNoValueReachesAChildAsZero hands a child `0` for an
	// exported name whose declaration named a numeric type — the integer or
	// the float letter — and which holds no value at all.
	//
	// Measured 2026-09-12 from a script file, `env -i` with a scratch
	// `HOME`, reading a real child's environment:
	//
	//	typeset -ix Z; env | grep '^Z='
	//
	//	ksh93u+      Z=0
	//	bash 5.3.15  nothing
	//	zsh 5.9.2    nothing
	//
	// The name really is unset in the shell that answers yes: `${Z+set}` is
	// empty there and `typeset -p Z` writes `typeset -x -i Z` with no value.
	// So the zero is not a value the store holds and cannot come from it —
	// it is what the *type* makes of nothing, produced for the child alone.
	// The float letter does the same and does not carry its precision:
	// `typeset -F 3 F; export F` hands over `F=0` and not `0.000`.
	//
	// It is the numeric letters and no others. `typeset -u U; export U`,
	// `typeset -a A; export A` and a plain `typeset P; export P` tell that
	// child nothing.
	//
	// The shell that answers no for the one-command form still tells a child
	// about the name when a *second* declaration names it — `typeset -i Z;
	// export Z` is `Z=0` in zsh — but that is the ordinary store being
	// exported once the name owns its value, and not this. See
	// Runner.declarationOwnsTheStandingEmpty, which is where the two part.
	//
	// Silent where it is answered wrongly, and only a real child can see it.
	NumericTypeWithNoValueReachesAChildAsZero Answer
	// NumericAttributeReplacesTheArrayAttribute makes the integer and float
	// letters take the *array* letter off a declaration that writes both, so
	// the name is a scalar of that type rather than an array of it.
	//
	// Measured 2026-09-12 from a script file:
	//
	//	typeset -ia z; typeset -p z
	//
	//	zsh 5.9.2    typeset -i z=0        the array letter is gone
	//	ksh93u+      typeset -a -i z       both stand
	//	bash 5.3.15  declare -ai z         both stand
	//
	// Within one word the numeric letter wins whichever order it is written
	// in — `typeset -ai z` is the same `typeset -i z=0` — so this is not the
	// last-one-speaks rule the case letters follow. Across two words it is:
	// `typeset -a z; typeset -i z` is `typeset -i z=0` there and `typeset -i
	// z; typeset -a z` is `typeset -a z=(  )`, which the compound axes
	// already answer from the other side.
	//
	// The *valued* form of the same combination is a refusal rather than a
	// collapse — see TypeLetterAndAnArrayLiteralIsAnInconsistentType — so the
	// two together are the whole of what the shell that says yes does with
	// the pairing.
	//
	// It is also what makes the array letter worth recording at all: a name
	// this answer left a scalar must not count as declared-an-array when a
	// later array literal decides whether to start it over.
	NumericAttributeReplacesTheArrayAttribute Answer
	// ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver makes `a=(x y)`
	// re-create a name whose declaration never wrote the array letter,
	// dropping the letters that say what its values are — where a name the
	// letter *was* written for keeps them and the literal simply fills it.
	//
	// Measured 2026-09-12 from a script file, with `typeset -p` after each:
	//
	//	                                        ksh93u+           zsh 5.9.2
	//	typeset -i a;    a=(5+5 6+6)   the letter goes       the letter goes
	//	typeset -ia b;   b=(5+5 6+6)   -a -i, values 10 12   (b is a scalar
	//	                                                     there; see
	//	                                                     the axis above)
	//	typeset -a -i c; c=(5+5 6+6)   -a -i, values 10 12   the letter goes
	//	typeset -l e;    e=(AB Cd)     the letter goes       the letter goes
	//	typeset -la f;   f=(AB Cd)     -a -l, folded         -al, kept
	//
	// bash keeps the letter under every one of those and evaluates through
	// it: `declare -ai a=([0]="10" [1]="12")`.
	//
	// The `-l` pair is the discriminating one, because it is the same two
	// lines differing only in the array letter: whether the name was
	// *declared* an array is the whole of what it turns on, and neither the
	// value the name holds nor the kind it currently is can answer it. That
	// is why the letter has to be recorded — and it already is: markIndexed
	// puts an empty array under the name, which is exactly the state
	// compoundNameHolds documents its `len(a) > 0` guard against. See
	// Runner.nameIsAnArray and #1264, whose "recorded nowhere" is out of
	// date rather than wrong.
	//
	// Distinct from ArrayLiteralAssignmentStartsTheNameOver, which asks the
	// same thing of a name that is *already holding* an array. A name may be
	// one and not the other in either direction, and the panel answers them
	// differently: zsh says yes here and no there.
	ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver Answer
	// AppendedArrayLiteralOverANameNotDeclaredAnArrayStartsItOver is the same
	// question asked of `a+=(x y)`, and it is a second axis because one shell
	// answers the two differently.
	//
	// Measured 2026-09-12 from a script file:
	//
	//	typeset -i p=3; p+=(5+5); typeset -p p
	//
	//	ksh93u+      typeset -a -i p=(3 10)   kept, and the append evaluated
	//	zsh 5.9.2    typeset -a p=( 3 5+5 )   the letter goes with the store
	//	bash 5.3.15  declare -ai p=([0]="3" [1]="10")
	//
	// `typeset -l t=A; t+=(B)` is the same split — `typeset -a -l t=(a b)` in
	// ksh93 — so it is the operator and not the letter that parts them. The
	// assign form is unanimous between those two shells and the append form
	// is not, which is exactly the shape #1755 warned about: an attribute's
	// answer on the way in is not its answer on a join.
	AppendedArrayLiteralOverANameNotDeclaredAnArrayStartsItOver Answer
	// NumericAttributeReplacesTheCaseAttribute makes the integer and float
	// letters take a case attribute off the name they are given, rather than
	// standing beside it.
	//
	// Measured 2026-09-12, from a script file:
	//
	//	typeset z=1; typeset -l z; typeset -i z; typeset -p z
	//
	//	ksh93u+      typeset -i z=1     the `-l` is gone
	//	zsh 5.9.2    typeset -i z=1     the same
	//	bash 5.3.15  declare -il z="1"  both stand
	//
	// `-F` is the same letter's family and behaves the same way: `typeset
	// -l z; typeset -F z` is `typeset -F z=1.0000000000` in both shells that
	// answer yes.
	//
	// The converse is a separate question and the panel splits differently
	// on it — see CaseAttributeReplacesTheNumericAttribute, which is what
	// makes this a direction rather than a set.
	//
	// Only a listing observes it, so the whole cost of the wrong answer is a
	// `typeset -p` that says more than the shell would.
	NumericAttributeReplacesTheCaseAttribute Answer
	// CaseAttributeReplacesTheNumericAttribute is the other direction: `-l`
	// and `-u` take the integer or float letter off the name they are given.
	//
	// Measured 2026-09-12, from a script file:
	//
	//	typeset y=1; typeset -i y; typeset -l y; typeset -p y
	//
	//	ksh93u+      typeset -l y=1      the `-i` is gone
	//	zsh 5.9.2    typeset -il y=1     both stand
	//	bash 5.3.15  declare -il y="1"   both stand
	//
	// So ksh93 holds one family — a name carries one letter saying what its
	// values are, and the last one written speaks — where zsh replaces in
	// one direction only and bash in neither. Two axes rather than one
	// three-valued answer because the two directions were measured
	// separately and one shell answers them differently.
	//
	// The value the earlier letter produced stays: `typeset -F z; typeset -l
	// z` is `typeset -l z=1.0000000000` in ksh93, so what goes is the
	// rendering of what comes next and not what is already there — the same
	// reading `+i` and `+F` already have.
	CaseAttributeReplacesTheNumericAttribute Answer
	// AttributeRereadsTheValueItFinds makes an attribute a declaration adds
	// re-read the value the name already holds, on the spot, rather than
	// waiting for the next assignment. `typeset -i FOO` on a `FOO=bar`
	// stores 0, and `typeset -u d` on a `d=MiXeD` stores MIXED.
	//
	//	FOO=bar; typeset -i FOO; echo "[$FOO]"
	//
	//	bash 5.3, bash as sh, bash 3.2   [bar]   the text stands
	//	ksh93u+, zsh 5.9.2               [0]     re-read as an expression
	//
	// **One of the two answers loses data whichever way it is chosen**, and
	// that is the reason this is a field and not a rule: the shells that
	// re-read destroy `bar` — an expression made of an unset name is 0, and
	// 0 is what is left — and the shells that do not leave a name declared
	// integer holding text that is not a number. There is no reading under
	// which both are satisfied, so a dialect has to say which shell it is.
	//
	// It is one question over every attribute that has something to say
	// about a value, not one per letter: the shells that re-read `-i` also
	// fold `-u` and `-l` on the spot, and the shells that do not, do not.
	// `-x`, `-r` and `-a` say nothing about a value and reach this nowhere.
	//
	// Asked only where a name is *already* holding something in the cell
	// being declared. A declaration that creates the name has nothing to
	// re-read, and inside a function the cell a shadow just made is new
	// whatever the caller held — measured, `v=5; function f { typeset -i v;
	// echo "[${v-UNSET}]"; }` reads UNSET in bash and ksh93 and `0` in zsh,
	// which is DeclaredNameWithoutValueIsEmpty and not this. Nor is it asked
	// where the two readings agree, so `a=7; typeset -i a` is 7 without a
	// dialect.
	//
	// The *next* assignment is unanimous and is no part of this: `typeset -i
	// a; a=3+4` is 7 and `typeset -u d; d=again` is AGAIN in every shell
	// that spells the letter. What is being asked is only whether the
	// attribute reaches backwards.
	//
	// Not an assignment, so it does not meet the readonly refusal: measured,
	// `typeset -r r=1; typeset -i r` is 1 at status 0 in the shells that
	// re-read.
	AttributeRereadsTheValueItFinds Answer
	// InheritedValueSurvivesADeclaredType keeps the value a name was born
	// with when a declaration gives it a *type* — `-i`, `-u` or `-l` — that
	// the same declaration does not also export or freeze.
	//
	// The third answer to the question above, on the one input where the two
	// shells that agree about a scalar part company. bash and zsh both keep
	// the value and go on to answer that question: bash leaves `bar` alone
	// and zsh re-reads it to 0, and both still hand it to a child. ksh93u+
	// **discards it** — the name comes back *unset* and *unexported*, so
	// `INHERITED=bar sh -c 'typeset -i INHERITED; env'` tells the child
	// nothing at all where the other two tell it something.
	//
	// It is not the re-read seen from another angle, and modeling it as one
	// gives the wrong answer twice over: a re-read produces 0 and keeps the
	// export, and the shell that discards produces neither. It is also not
	// "a declaration empties what it touches" — `${name+SET}` is empty
	// afterwards, so the name has no value rather than an empty one, which
	// is exactly what that dialect's `DeclaredNameWithoutValueIsEmpty` = no
	// leaves a *fresh* name holding. The declaration is starting the name
	// over.
	//
	// Asked only where every part of the shape holds, because each part is
	// what a narrower or wider reading gets wrong:
	//
	//   - The value is the one the shell was **started with** and the script
	//     has never assigned. `D=$D; typeset -i D` re-reads to 0 and keeps
	//     the export in all three, and so does `export FOO=bar; typeset -i
	//     FOO` — so this is about where the value lives, not about the
	//     export attribute.
	//   - A **type** letter arrived. A declaration that says nothing about a
	//     value — a bare `typeset`, `-x`, `-r` — leaves an inherited name
	//     entirely alone in every shell.
	//   - Whether the fold would change anything is **not** part of it, and
	//     this is where the question parts company with the re-read above:
	//     an inherited `7` meeting `-i`, and an inherited `UPPER` meeting
	//     `-u`, are discarded too. So it is asked ahead of
	//     attributeWouldChange rather than behind it.
	//   - The **same command** must not also name `-x` or `-r`. `typeset -ix
	//     G` and `typeset -ir K` keep the value and re-read it; splitting
	//     them in two — `typeset -x P; typeset -i P` — discards it, and so
	//     does any *other* extra letter, measured with `-t`. One command's
	//     letters, not the name's standing attributes.
	//
	// bash yes (both builds), ksh93 no, zsh yes. dash has no declaration
	// builtin with a type letter and never arrives.
	InheritedValueSurvivesADeclaredType Answer
	// CompoundElementsGoThroughTheAttribute folds what is written to one
	// element of an array or a keyed table through the attribute the *name*
	// carries, the way a scalar assignment already does everywhere.
	//
	// For a scalar this needs no dialect: `typeset -i n; n=3+4` is 7 and
	// `typeset -u d; d=again` is AGAIN in every shell that spells the letter.
	// An element is where the panel splits.
	//
	//	typeset -ia a=(1 2); a[1]=3+4     bash, ksh93 `1 7`
	//	typeset -ua q=(ab cd); q[1]=ef    bash, ksh93 `AB EF`   zsh `ef cd`
	//	typeset -A m; typeset -i m
	//	m[k]=7+7                          bash, ksh93 `14`
	//
	// bash and ksh93 fold every element write. zsh does not fold an array's
	// elements at all: its case letters reach a scalar's expansion and stop
	// there, and its integer letter never meets an array in the first place —
	// see CompoundMeetingANewAttribute, where that letter replaces the array
	// with a scalar. So the answer is read off the case letters, which are
	// the only ones that dialect can be asked about here.
	//
	// Asked only where a name with one of these attributes has an element
	// written to it, so an array with no attribute needs no dialect.
	CompoundElementsGoThroughTheAttribute Answer

	// CaseAttributeFoldsWhenRead decides *when* the case attributes act:
	// once, on the value being stored, or on every read of it.
	//
	// The difference is invisible in the value — `$v` is `ab` under both —
	// and shows in the two places that see the store itself. Measured
	// 2026-09-12, `env -i` with a scratch HOME and no startup files, over
	// `typeset -l lo=AB`:
	//
	//	           $lo   typeset -p lo        typeset +l lo; $lo
	//	bash 5.3   ab    declare -l lo="ab"   ab
	//	ksh93u+    ab    typeset -l lo=ab     ab
	//	zsh 5.9.2  ab    typeset -l lo=AB     AB
	//
	// So the third column is the discriminating one: taking the attribute
	// off reveals what the store really holds, and only one of these shells
	// has anything left to reveal. The listing is what #1755 is about — a
	// shell that folds on the way in has no way back to the text the
	// assignment carried, and its `-p` cannot write the declaration it read.
	//
	// Two consequences of folding on the read, both measured on the same
	// day and neither derivable from the row above:
	//
	//   - An append joins the *stored* text. `typeset -l lo=AB; lo+=CD`
	//     lists as `ABCD` and reads as `abcd`.
	//   - A pattern operator matches the *folded* text, because it is a read
	//     like any other: `${v/A/x}` leaves `ab` alone where the shell that
	//     stores `ab` never had an `A` to match either. The two answers agree
	//     here and differ on `${v/a/x}`, which is `xb` in the storing shell.
	//
	// Arrays are outside this on both answers and for different reasons, so
	// it is not asked of one: the shell that folds on the read does not fold
	// an array's elements at all — `typeset -al arr=(AB Cd)` reads back `AB
	// Cd` — and the shell that folds on the way in has
	// CompoundElementsGoThroughTheAttribute for the same question.
	//
	// Asked only where a name carries `-l` or `-u`, which is where the two
	// answers can be told apart.
	CaseAttributeFoldsWhenRead Answer
	// ArrayLiteralAssignmentStartsTheNameOver makes `a=(x y)` *re-create* the
	// name — the attributes it carries and all — rather than replacing only
	// its elements.
	//
	//	typeset -ia z=(1); z=(5+5 6+6); typeset -p z; z[0]=3+4
	//	  bash    declare -ai z=([0]="10" [1]="12")   then `7 12`
	//	  ksh93   typeset -a z=(5+5 6+6)              then `3+4 6+6`
	//
	// ksh93 keeps neither the fold nor the letter: the listing has lost the
	// `-i`, and the element write after it is not folded either, which is
	// what says the attribute is *gone* rather than merely bypassed by that
	// one assignment. The case letters answer the same way in both — bash
	// keeps `-u` and folds, ksh93 lists `typeset -a q=(gh ij)` — and zsh,
	// which can only be asked through a case letter, keeps it: `typeset -au
	// q=( gh ij )`.
	//
	// The same idea `unset` is a rule about and that
	// InheritedValueSurvivesADeclaredType is the other side of: a name whose
	// whole value is replaced may be a *new* name in one of these shells.
	//
	// Asked only for the plain assignment spelling, and this is where the
	// spelling earns its own question: a declaration's own operand —
	// `typeset -ia d=(5+5 6+6)` — folds to `10 12` in **both**, so the
	// letters cannot have gone there. It is `syntax.Assign.Operand` that
	// tells the two apart, and an append is not it either: `f+=(8+8)` is 16
	// in both.
	//
	// And asked only where the name has something to start over: the
	// *first* array literal a declared name receives keeps the letter and
	// folds in both — `typeset -ia b; b=(5+5 6+6)` is `10 12` and lists as
	// `typeset -a -i b=(10 12)` — so what re-creates the name is replacing a
	// value it is already holding.
	//
	// The shape that reading leaves out has an axis of its own now: `typeset
	// -i a; a=(5+5 6+6)`, where the declaration named no array letter at
	// all, drops the attribute in ksh93 and in zsh even though `a` was
	// holding nothing. What that turns on is whether `-a` was *written*, so
	// it is a different question from this one and the panel answers the two
	// differently — see
	// ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver and its append
	// half (#1264).
	//
	// And asked only for the *indexed* literal. A keyed one keeps the
	// attribute in both: `typeset -A m; typeset -i m; m[k]=1; m=([j]=2+2)`
	// lists as `typeset -A -i m=(…)` in ksh93 with the `2+2` folded to 4,
	// where the indexed spelling on the same line loses the letter. So it is
	// this spelling and not "replacing a compound value" in general — a
	// wider reading would take the attribute off a table no shell takes it
	// off.
	ArrayLiteralAssignmentStartsTheNameOver Answer
	// ScalarAppendedToAnArrayBecomesANewElement decides where `a+=x` puts
	// the value when the name is holding an *array*: after the last element,
	// or joined onto the first one.
	//
	// Measured 2026-09-08, panel and machine as docs/spec/oracle.md, with
	// `a=(1 2); a+=x; typeset -p a`:
	//
	//	bash 5.3.15         declare -a a=([0]="1x" [1]="2")   n=2
	//	bash 5.3.15 as sh   declare -a a=([0]="1x" [1]="2")   n=2
	//	bash 3.2.57         declare -a a=([0]="1x" [1]="2")   n=2
	//	ksh93               typeset -a a=(1x 2)               n=2
	//	zsh 5.9.2           typeset -a a=( 1 2 x )            n=3
	//
	// So No in bash, bash as `sh`, bash 3.2 and ksh93, and Yes in zsh. dash
	// has no arrays and reports the parenthesis, which is the absence rather
	// than a sixth answer. The count is what tells the two apart from the
	// outside; the listing is what says which element moved.
	//
	// The join is at the *base* rather than at the lowest subscript standing,
	// which a sparse array is what shows: `a=([5]=q); a+=x` is
	// `declare -a a=([0]="x" [5]="q")` in bash, so the value lands at the
	// first element whether or not there is one there, and `q` is left where
	// it was. The empty string is a value on both sides of the axis —
	// `a=(1 2); a+=""` leaves bash's two elements alone and gives zsh a third
	// that is empty — and the value joins whole however many words it looks
	// like: `a+="p q"` is one element in every column.
	//
	// Asked only where the name is holding an array. An *unset* name and a
	// name holding a scalar are the string append, which is unanimous and
	// core: `unset a; a+=x` leaves a plain scalar in every column. The
	// array-literal spelling `a+=(x)` is not this question either — it adds
	// an element in every shell that has arrays, which is why that one has no
	// field. See Runner.appendScalarToArray.
	ScalarAppendedToAnArrayBecomesANewElement Answer
	// ScalarAssignedOverACompoundReplacesTheName decides what a plain `a=x`
	// does to a name that is already holding an array or a table: the value
	// becomes the whole of the name, or it lands on the compound's first
	// element and the rest stays where it is.
	//
	// Measured 2026-09-09, panel and machine as docs/spec/oracle.md, with
	// `a=(1 2 3); a=x; typeset -p a`:
	//
	//	bash 5.3.15         declare -a a=([0]="x" [1]="2" [2]="3")   n=3
	//	bash 5.3.15 as sh   declare -a a=([0]="x" [1]="2" [2]="3")   n=3
	//	bash 3.2.57         declare -a a=([0]="x" [1]="2" [2]="3")   n=3
	//	ksh93               typeset -a a=(x 2 3)                     n=3
	//	zsh 5.9.2           typeset a=x                              n=1
	//
	// So No in bash, bash as `sh`, bash 3.2 and ksh93, and Yes in zsh, where
	// `${(t)a}` reads `scalar` afterwards and the array is gone rather than
	// merely hidden. dash has no arrays and refuses the parenthesis, which is
	// the absence rather than a sixth answer.
	//
	// A **table** answers the same way in every column, so it is this one
	// field and not two: `typeset -A m; m=([k]=v); m=x` is
	// `declare -A m=([0]="x" [k]="v" )` in bash and `typeset -A m=([0]=x
	// [k]=v)` in ksh93, and `typeset m=x` in zsh. Where the two kinds of
	// compound part is a *declaration* adding an attribute, which is
	// ScalarUnderAnArrayDeclaration against ScalarUnderATableDeclaration;
	// nothing parts them here.
	//
	// The element written is the compound's *first* — the array base, and the
	// key `0` — whether or not there is anything there already:
	// `a=([5]=q); a=x` is `declare -a a=([0]="x" [5]="q")` in bash, so `q`
	// does not move and the array grows. The same place `a+=x` joins, which
	// is why the two axes read one arrayBase between them.
	//
	// Asked wherever a scalar is *stored* and not only at an assignment
	// statement, which is the whole point of the field: `for a in x y z`,
	// `read a`, `select`, `getopts`, `printf -v` and `${a::=x}` all set a
	// name, and every one of them was leaving an array standing so the name
	// read back as the array on every pass. zsh's own compinit reuses
	// `_i_line` as an array and then as a loop variable, and all eight passes
	// of its widget-rebinding loop saw the last file it had read (#1645).
	//
	// Not asked for the store that keeps `$a` answering for an array `a` —
	// see assignedAsTheCompoundView, which is that write and no other.
	ScalarAssignedOverACompoundReplacesTheName Answer
	// CompoundAttribute is what an attribute a declaration has just added
	// makes of a compound value the name is already holding — see
	// CompoundAttributePolicy, where the three answers are.
	CompoundAttribute CompoundAttributePolicy
	// ScalarUnderAnArrayDeclaration is what `typeset -a` makes of a scalar
	// the name is already holding — see ScalarUnderACompoundPolicy, where the
	// three answers are. The converse of CompoundAttribute above, which asks
	// what an attribute makes of an array.
	ScalarUnderAnArrayDeclaration ScalarUnderACompoundPolicy
	// ScalarUnderATableDeclaration is the same question asked of `typeset -A`.
	//
	// A second field and not a widening of the one above, because one shell
	// answers the two letters differently: measured 2026-09-08, ksh93 leaves
	// `b=1; typeset -a b` a plain scalar and takes `a=1; typeset -A a` to
	// `typeset -A a=([0]=1)`. bash promotes under both letters and zsh
	// discards under both, so ksh93 is the whole of why this is two questions
	// — and one field would have had to give it an answer that is wrong for
	// one of its letters whichever way it was set.
	//
	// unexhibited ScalarUnderACompoundStaysAScalar:
	// ScalarUnderAnArrayDeclaration holds it, for ksh93 — which is the
	// whole reason these are two fields. Re-measured 2026-09-12: `a=1;
	// typeset -A a` lists as `typeset -A a=([0]=1)` in ksh93u+ while `b=1;
	// typeset -a b` leaves `b=1`. bash promotes under both letters and zsh
	// discards under both; dash has no `typeset` and bash 3.2.57 has no
	// `-A`, so no column answers *this* letter with the scalar (#2060).
	ScalarUnderATableDeclaration ScalarUnderACompoundPolicy

	// ValuelessDeclarationHidesTheOuterValue makes `local u` in a function
	// hide any outer `u` — the local exists unset, so `${u-UNSET}` fires the
	// default even when the caller had a value. Reached only when
	// DeclaredNameWithoutValueIsEmpty said no: a name declared *empty* hides
	// the outer value by having one of its own.
	//
	// bash hides it, and so does ksh93's `typeset` in a keyword function;
	// dash leaves the caller's value showing through until the first
	// assignment. This is the shape used to declare a local before
	// assigning it conditionally, so the difference is silent: the function
	// reads the caller's value where it expected nothing.
	//
	// unpinned zsh: never reached. It is asked only where
	// `DeclaredNameWithoutValueIsEmpty` said no, and this dialect says yes
	// — a `local u` there exists holding the empty string, so there is no
	// outer value left to hide. Measured 2026-09-12: `u=out; f() { local
	// u; echo "[${u-UNSET}]"; }; f` is `[]` in zsh 5.9.2 and here, and
	// moving this axis to `Yes` or to `Unspecified` changes neither
	// (#2057).
	ValuelessDeclarationHidesTheOuterValue Answer

	// DeclarationAssignmentClearsTheExportAttribute takes the export
	// attribute off a name a declaration utility assigns to. ksh93 does;
	// bash 5.3, bash 3.2, bash as `sh` and zsh keep it, and dash has no
	// declaration utility to ask with.
	//
	//	export FOO=bar; typeset FOO=baz; env | grep '^FOO='
	//
	// One shell tells the child nothing and goes on telling it nothing: the
	// name keeps its value and is simply no longer exported, which
	// `export -p` and `typeset -p` both confirm. `export FOO` afterwards
	// puts the attribute back, so it is a reset rather than a refusal.
	//
	// Asked only where the name being assigned was already exported, where
	// the declaration does not name the attribute itself, and where the
	// declaration did *not* take a scope. The scoped half is
	// LocalInheritsTheExportAttribute, which is the same shell's answer
	// arrived at from the other side and already takes the attribute off for
	// the function's duration — the two must not both fire, or a keyword
	// function would leave the caller's name unexported, which it does not.
	//
	// The value on the line is what asks it. A valueless `typeset FOO`
	// leaves the attribute alone, and so do the valueless declarations that
	// change the value anyway — `typeset -i FOO` stores 0 and `typeset -u
	// FOO` folds what is there, and a child is told about both. `readonly
	// FOO=baz` clears it, because in the shell that does this `readonly` is
	// that shell's `typeset -r`; `export FOO=baz` does not, because it names
	// the attribute. A plain `FOO=baz` does not either, in any shell — this
	// is a declaration utility's doing and not an assignment's.
	//
	// The preset is no: POSIX has an exported name keep the attribute for
	// the life of the shell, and the two other shells with the builtin
	// agree.
	DeclarationAssignmentClearsTheExportAttribute Answer

	// LocalInheritsTheExportAttribute gives a local declaration the export
	// attribute of the name it shadows, so a child sees the local's value
	// under the shadowed name. bash and dash say yes; zsh says no and hands
	// the child nothing at all under that name for as long as the function
	// runs.
	//
	// Asked only where the shadowed name is exported — explicitly or by
	// having been inherited — and only where a scope was actually taken.
	// Declaring a name nothing has exported asks nothing, and a local
	// declared `-x` says so outright and asks nothing either.
	//
	// The value is not the question: the local's own value is what a child
	// is told in the dialects that answer yes, and whether a valueless
	// declaration still shows the outer value is
	// ValuelessDeclarationHidesTheOuterValue rather than this.
	//
	// ksh93 has no `local`, so the question reaches it only through
	// `typeset` in a keyword-defined function, where a child is told
	// nothing — the same answer as zsh by a different road, because that
	// shell's `typeset` takes the attribute off any name it assigns, at the
	// top level as well as in a function. Only the local half is modeled.
	LocalInheritsTheExportAttribute Answer
	// TypesetLocalNeedsKeywordFunction restricts `typeset`'s local scope to
	// functions defined with the `function` word. ksh93 says yes: in
	// `f() { typeset x=1; }` the assignment reaches the caller's `x`, and in
	// `function f { typeset x=1; }` it does not. bash and zsh make no such
	// distinction, which is why the two definition forms are interchangeable
	// there and are not in ksh93. dash has no `typeset` at all, which is why
	// the axis is absent rather than false there.
	//
	// It asks about `typeset` and not about `local` because `local` is
	//
	// unanimous: every shell that has it — all but ksh93, which does not —
	// makes it local in a function defined either way.
	TypesetLocalNeedsKeywordFunction Answer

	// DeclareListing is the shape of what `declare -p` and `typeset -p`
	// write back. Three engines rather than two answers — see
	// DeclarationListingForm.
	//
	// unexhibited DeclareListingCommandWord: ExportListing and
	// ReadonlyListing hold it, for dash, ksh93 and zsh. No `declare -p` or
	// `typeset -p` in the panel repeats its own command word: re-measured
	// 2026-09-12, the three shells that have the builtin write `declare -x
	// V="a b"`, `typeset -x V=E` and `export V='a b'`, and bash keeps the
	// clustered form even under `set -o posix` where its `export -p` does
	// not (#2154, #2060).
	//
	// unexhibited DeclareListingPlainAssignment: BareDeclarationListing
	// holds it, for ksh93 and zsh. The value's own comment is the
	// measurement: no `-p` anywhere writes a bare assignment, because a
	// listing with no command word could not be read back as a declaration
	// (#2060).
	DeclareListing DeclarationListingForm

	// DeclareValueQuoting is how a listed declaration spells its value. A
	// field of its own over the shared vocabulary because it does not follow
	// the dialect's other listings: the engine that single-quotes its
	// aliases and traps double-quotes its declarations.
	//
	// unexhibited ListingQuoteAlwaysDoubled: SetListingQuoting and
	// AliasQuoting hold it, for dash — which has no `typeset` at all, so
	// it never answers this field. Re-measured 2026-09-12: a bare `set`
	// writes `Q='a'"'"'b'` there (#2060).
	//
	// unexhibited ListingQuoteWhenNeededPlain: TrapQuoting holds it, for
	// zsh. zsh's `typeset -p` reaches for `$'...'` where its `trap`
	// listing never does, which is why the two are separate fields
	// (#2060).
	DeclareValueQuoting ListingQuotingStyle

	// ExportListing is the shape `export -p` writes: bash spells each name
	// as a clustered declaration (`declare -x V="1"`), and the other three
	// repeat the command word (`export V='1'`).
	//
	// unexhibited DeclareListingExportSpelled: ReadonlyListing holds it,
	// for zsh — measured 2026-09-12, `readonly -p` writes `typeset -r R=2`
	// there where `export -p` writes `export V='a b'`, which is why the
	// two are separate fields (#2060).
	//
	// unexhibited DeclareListingBareAssignments: DeclareListing holds it,
	// for ksh93. `export -p` in ksh93u+ repeats the command word (`export
	// V='a b'`) where `typeset -p` drops it for an unattributed name
	// (#2060).
	//
	// unexhibited DeclareListingPlainAssignment: BareDeclarationListing
	// holds it, for ksh93 and zsh, which drop the command word for the
	// bare form alone. Note what is *not* a fifth reading: bash in POSIX
	// mode writes `export V="a b"`, which is DeclareListingCommandWord — a
	// value this axis already carries, reached by a mode no preset holds
	// and [Runner.SetPosixMode] does not move. Measured 2026-09-12 on both
	// bash builds and filed as #2154 (#2060).
	ExportListing DeclarationListingForm
	// ReadonlyListing is the same question from `readonly -p`, where zsh
	// parts ways with its own export listing and writes `typeset -r R=2`.
	//
	// unexhibited DeclareListingBareAssignments: DeclareListing holds it,
	// for ksh93, whose `readonly -p` repeats the command word instead —
	// `readonly R=2`, measured 2026-09-12 (#2060).
	//
	// unexhibited DeclareListingPlainAssignment: BareDeclarationListing
	// holds it, for ksh93 and zsh. bash in POSIX mode writes `readonly
	// R="2"` here, which is DeclareListingCommandWord and already carried;
	// see #2154 (#2060).
	ReadonlyListing DeclarationListingForm

	// CoprocEndsInAnArray publishes a started coprocess's near ends as the
	// two elements of an array — `${COPROC[0]}` to read and `${COPROC[1]}` to
	// write, with the process in `COPROC_PID` — which is bash's model and the
	// reason its `coproc` takes a name. zsh answers no: it has no name for a
	// coprocess and no array, and a script reaches the ends with `print -p`
	// and `read -p` instead. Asked only when a coprocess is started, so a
	// dialect without the word never meets it.
	CoprocEndsInAnArray Answer

	// BareDeclarationListing is the shape `export` and `readonly` write with
	// no operands and no `-p` — which is not always the shape `-p` writes.
	// dash and both bash builds answer the bare form exactly as they answer
	// `-p`; ksh93 and zsh drop the command word for the bare form alone and
	// write a plain `V='a b'`, which no `-p` anywhere writes because it could
	// not be read back as a declaration. Measured across the panel from one
	// exported and one readonly name.
	//
	// One field for both builtins, because no shell in the panel splits them:
	// where the bare form differs from `-p` it differs for both, and by the
	// same rule.
	//
	// It is the *filtered* listing's row as well — `declare -x` and
	// `typeset -a` with no names — which is measured and not assumed: bash
	// writes `declare -x e="1"` for both, and ksh93 and zsh drop the command
	// word for both, `e=1`. So the filter chooses the names and this chooses
	// the row, and neither builtin needs a form of its own.
	//
	// unexhibited DeclareListingExportSpelled: ReadonlyListing holds it,
	// for zsh. zsh spells the bare form as a plain assignment and the `-p`
	// form as `typeset -r`, which is the split this field records (#2060).
	//
	// unexhibited DeclareListingBareAssignments: DeclareListing holds it,
	// for ksh93. Re-measured 2026-09-12 over one exported and one readonly
	// name: the bare `export` writes `V='a b'` in ksh93u+ and zsh 5.9.2,
	// `export V='a b'` in dash, and `declare -x V="a b"` in both bash
	// builds — and `export V="a b"` in bash under `set -o posix`, which is
	// DeclareListingCommandWord and filed as #2154 (#2060).
	BareDeclarationListing DeclarationListingForm

	// DeclarationListingFilter is how a `declare` or `typeset` with attribute
	// letters and no names combines them when more than one is written —
	// `declare -ir`, `typeset -ax`. See DeclarationFilterForm, which carries
	// the three shells' three answers and the measurements.
	//
	// Asked only where two letters were written: all three readings agree on
	// one letter, so a dialect that has not answered still lists `declare -x`.
	DeclarationListingFilter DeclarationFilterForm

	// DeclarePrintReportsAMissingName makes `typeset -p nosuch` say so and
	// fail. bash and zsh report it (with their own wording — see
	// Diagnostics.DeclareNoSuchVariable) and answer 1 even when other names
	// listed fine; ksh93 prints nothing for the missing name and answers 0.
	DeclarePrintReportsAMissingName Answer

	// DeclareOptions is the set of letters `declare` and `typeset` take,
	// spelled the way ReadOptions is. The letters are the dialect's own:
	// `-g` declares a global in the two shells that have the letter and is
	// an unknown option in ksh93, whose bad typeset options are fatal, and
	// `-F` names functions in bash while it sets a float's precision in the
	// other two. Empty means `aAiprx`, the set the substrate implemented
	// before the letters were a question.
	DeclareOptions string

	// DeclareOptionsWithoutEffect names letters out of DeclareOptions that
	// this engine models as doing nothing: accepted, silent, and 0.
	//
	// What it is for is a letter whose meaning *here* is not the meaning the
	// substrate gives it. A letter the substrate has never heard of is
	// already silent once DeclareOptions names it — that is what `-a` is —
	// so this field earns its place only where the substrate would otherwise
	// act, and taking the substrate's meaning away is the whole of what it
	// does.
	//
	// It is the quiet counterpart of Diagnostics.UnimplementedOptionLetters,
	// and the choice between the two is which lie is smaller. A letter that
	// is refused puts a sentence into output a caller shows a person and
	// ends a script under `set -e`; a letter accepted here changes what a
	// value looks like when it is printed back and nothing else.
	//
	// zsh's `-F` is the letter this was written for and no dialect sets it
	// today: the letter is a *float's* precision there rather than bash's
	// function listing, and silence was the smaller lie only for as long as
	// there was no float attribute to record the precision in. There is one
	// now — see DeclareOptionsTakingANumber — so `-F` graduated out of here
	// and the field stands for the next letter of that shape rather than
	// being deleted with its one user.
	//
	// A letter listed here that is not in DeclareOptions does nothing: the
	// dialect has to have the letter before this can decide what it means,
	// which keeps the two fields from disagreeing about whether it exists.
	DeclareOptionsWithoutEffect string

	// DeclareOptionsTakingANumber names letters out of DeclareOptions whose
	// *argument* is a number rather than the first operand: `typeset -F 3 x`
	// declares one name with three digits of precision, and the `3` was
	// never a name. Both spellings, measured 2026-09-07 in zsh 5.9.2 and
	// ksh93 alike: the detached `-F 3 x` and the attached `-F3 x`.
	//
	// Empty is bash's answer and the substrate's: measured, bash reads the
	// same line as two operands and says `8: not a valid identifier` for
	// `typeset -i 8 n=64`, so the word after the letter is a name there and
	// nothing is consumed. dash has neither builtin.
	//
	// Three rules come with the letters, each measured rather than assumed:
	//
	//   - Only a run of decimal digits is the argument. `typeset -F abc x=1`
	//     declares *both* `abc` and `x` as floats in zsh and ksh93 alike, so
	//     a word that is not a number was never the letter's argument.
	//   - One number, not a list. `typeset -F 3 4 x` is `not an identifier:
	//     4` in zsh and `4: invalid variable name` in ksh93: the letter is
	//     satisfied by the first number and the second is an operand again.
	//   - The number belongs to the *first* number-taking letter of its
	//     option word, wherever in the word that letter stands, and taking
	//     it discards whatever else that word carried. `typeset -ix 16
	//     n=255` reads 16 as a base and leaves `n` unexported, `-gF 3` and
	//     `-Fg 3` both read 3 as a precision, and the two letters written
	//     together settle it the same way — `-Fi 3 x=1.5` lists back as
	//     `typeset -F x=1.500` and `-iF 3 x=1.5` as `typeset -i3 x=1`.
	//     Only when a number is really taken, which is the half a rule
	//     spelled "the letter ends its word" gets wrong: `typeset -ix
	//     n=255` with no number behind it exports like any other word. See
	//     declarebuiltin.go's numberEndsTheWord, which is where that lives.
	//
	// This is zsh's arrangement throughout. ksh93 has the same letters and
	// spells their number *attached only* — `-F[n]` in its own usage — so
	// `typeset -Fx 3 a=1.5` is `3: is not an identifier` there where zsh
	// reads the precision. The field has no per-spelling half because only
	// zsh sets it; #1461 is where that would be needed.
	//
	// `-i` is not named here and is not an omission: whether it takes a base
	// is IntegerAttributeTakesABase, which the `integer` builtin asks too and
	// which also decides whether the base is *recorded*. Two fields both
	// claiming `-i` takes a number could disagree, so the parse asks one
	// helper — declareOptionTakesANumber — and that helper reads the axis for
	// `i` and this list for every other letter.
	//
	// Only letters this engine both spells and acts on belong here. zsh's
	// `-E`, `-L`, `-R` and `-Z` take a number in the real shell and are
	// refused by name before it is ever read, so an entry for them would be
	// consulted by nothing — see Diagnostics.UnimplementedOptionLetters, and
	// #1461 for implementing them.
	DeclareOptionsTakingANumber string

	// TypesetBadOptionFatal ends the script over an option `typeset` does
	// not have. ksh93 counts `typeset` among its special builtins and stops
	// there; bash and zsh report it and carry on. Asked only when the
	// refusal has happened, so a shell with no `typeset` never meets it.
	TypesetBadOptionFatal Answer

	// SignAloneIsAnOptionWord reads a declaration's `-` or `+` written with
	// no letters after it as an option word rather than as an operand.
	//
	// A real disagreement and not a missing feature, which is why it is a
	// field: measured 2026-09-10, `typeset +` writes the whole parameter
	// table's attribute words and names in zsh 5.9.2 and its own name
	// listing in ksh93, while bash 5.3 answers ``typeset: `+': not a valid
	// identifier`` at 1 — the sign is a *name* there, and not one a script
	// may declare. Both readings are complete and neither is a superset of
	// the other, so the sign cannot simply be swallowed.
	//
	// The sign means what it means everywhere else on the builtin once it is
	// read: a minus adds nothing to the bare listing and a plus turns it
	// into the names-only shape. `functions +` is the same word reaching the
	// function table, and is how a shell snapshot asks for a name list
	// (#1576).
	//
	// Asked only where a script really wrote one, so a dialect that never
	// meets the shape is never asked for an answer.
	SignAloneIsAnOptionWord Answer

	// SignAloneIsAnOptionWordToExport is the same reading asked of `export`
	// and `readonly`, which have an option parse of their own.
	//
	// A second field rather than the one above because the two questions
	// have different answers in the same shell. Measured 2026-09-12 with
	// `env -i` and no startup files: ksh93u+ lists for `typeset +` and
	// answers `export: +: is not an identifier` at 1 for `export +`, and
	// `readonly: +: invalid variable name` for `readonly +` — so a dialect
	// reading one field for both would have to be wrong about one of them.
	// zsh 5.9.2 takes the sign in all three; bash 5.3, bash 3.2 and dash
	// refuse it as a name in all three.
	//
	// The listing it reaches is the *builtin's own* attribute, names only:
	// `export +` writes the exported names and `readonly +` the frozen ones,
	// which is `typeset +x` and `typeset +r` under a second word. The minus
	// spelling needs no field — a lone `-` is already LoneDashIsAnOption's
	// question, and once it is eaten `export -` is the bare listing, which
	// is what zsh writes for it (#1756).
	//
	// One field for the two builtins because no column separates them: each
	// shell answers `export +` and `readonly +` the same way. Asked only
	// where a script really wrote the sign.
	SignAloneIsAnOptionWordToExport Answer

	// FunctionNamesUnderPlus reads the *plus* spelling of the function
	// letter as a request for the names alone — `typeset +f` against
	// `typeset -f`.
	//
	// The two shells that spell the letter disagree about what the sign
	// means, which is why this is a field rather than the substrate's
	// assumption. Measured 2026-09-10:
	//
	//   - zsh 5.9.2 writes one bare name a line, with an operand and
	//     without one alike, and the last `f` letter's sign decides —
	//     `typeset -f +f f` is the name and `typeset +f -f f` is the body.
	//   - bash 5.3 has no names-only spelling under this sign at all. Its
	//     `+f` takes the function attribute *off*, leaving the bare
	//     `declare`, which writes every variable and then every function.
	//     That listing is BareDeclarationListing's unanswered row for that
	//     shell, so `No` here leaves the letter writing bodies: the row is
	//     as wrong as it was rather than wrong in a new way (#1754).
	//
	// ksh93 is a third answer again — `f()` for a function declared with
	// parentheses and the bare name for one declared with the keyword — and
	// is not asked, because that dialect refuses the `f` letter outright
	// while its body listing is a verbatim copy of the source text this
	// engine does not keep (#1494).
	//
	// Asked only where a plus-signed `f` was really written.
	FunctionNamesUnderPlus Answer

	// FunctionLettersThatMarkUndefined names the letters that turn a `-f`
	// declaration with operands from a *listing* into a marking: the name
	// becomes a function at once whose body is read the first time it is
	// called.
	//
	// Both shells with the notion spell it on `typeset` as well as under
	// their own word, and they do not spell it alike, which is why this is a
	// letter set rather than a fixed reading. Measured 2026-09-12, `env -i`
	// and no startup files:
	//
	//   - zsh 5.9.2 takes `u` and `U`. `typeset -fu nm` leaves the stub
	//     `autoload nm` leaves and `typeset -fUz nm` the one `autoload -Uz
	//     nm` leaves, letters and all — the `z` rides along and is recorded
	//     without being one of these, because a letter that only *decorates*
	//     the marking cannot start one: `typeset -fz nm` marks nothing.
	//   - ksh93u+ takes `u` alone and has no `U`. `typeset -fu nm` there
	//     lists back as `typeset -fu nm`, which is that shell's whole
	//     rendering of an undefined function.
	//
	// Empty in a shell with no such notion, where every `-f` line is a
	// listing. What the letters *do* is [Runner.SetFunctionMarkedUndefined],
	// which is where the dialect's own vocabulary lives; this field only
	// says which lines are not listings.
	FunctionLettersThatMarkUndefined string

	// IntegerOptions is the set of letters the `integer` builtin takes,
	// spelled the way DeclareOptions is. It is a separate field rather than
	// DeclareOptions over again because the two shells that have the word
	// give it a *narrower* set than their own `typeset`, and they narrow it
	// differently: measured 2026-09-06, zsh's `integer` refuses `-a`, `-A`,
	// `-f`, `-F`, `-T` and `-U` as bad options where its `typeset` takes
	// every one of them, and ksh93's refuses `-E`, `-H`, `-L`, `-R`, `-T`
	// and `-Z` — the float and justification letters — where its `typeset`
	// spells them all. So a shell whose `integer` simply reused the
	// declaration's letters would accept `integer -A m`, which is an
	// associative array in neither shell.
	//
	// Empty means the builtin is not registered at all, which is the answer
	// for bash and dash: the word is a command that was not found there,
	// which is what those two really do with it.
	IntegerOptions string

	// FunctionsOptions is the set of letters the `functions` builtin takes,
	// spelled the way DeclareOptions is.
	//
	// A separate field rather than DeclareOptions over again, because the
	// name that means `typeset -f` does not take `typeset`'s letters:
	// measured 2026-09-08, zsh 5.9.2 refuses `functions -f`, `-F` and `-p`
	// as bad options though its `typeset` spells all three, and takes `-c`,
	// `-k`, `-m`, `-s`, `-t`, `-u`, `-x`, `-z`, `-M`, `-T`, `-U` and `-W`,
	// which its `typeset` does not. So a shell that reused the declaration's
	// set would accept `functions -a`, which is an array attribute in
	// neither shell, and refuse `functions -m`, which is a listing in one.
	//
	// Only the letters this engine both spells and acts on belong here; the
	// rest are Diagnostics.UnimplementedOptionLetters, so a script meets
	// "not implemented yet" for a letter zsh really has and "bad option" for
	// one it does not. Empty means the builtin takes no letters at all,
	// which is a real answer — whether the word exists is Register's, not
	// this field's.
	FunctionsOptions string

	// UnfunctionOptions is the same for `unfunction`, whose set is one
	// letter: measured, zsh takes `-m` and refuses every other letter of the
	// alphabet in both cases as a bad option — including the `-f` that is
	// the option this name stands for.
	UnfunctionOptions string

	// IntegerAttributeTakesABase is `-i` reading an output base — `typeset
	// -i 16 n=255` and its attached spelling `-i16` — so that the name
	// prints in that base afterwards rather than in decimal.
	//
	// ksh93 and zsh answer yes and store `16#ff` and `16#FF`; bash answers
	// no, where `-i16` is an invalid option and a bare `16` is not a valid
	// identifier.
	//
	// The base is a property of the *name* and not of the assignment that
	// met it, so it is recorded like the attribute itself and consulted by
	// every store afterwards: `typeset -i8 c; c=64` is `8#100`, and a second
	// declaration with a different base re-renders what the name already
	// holds — `typeset -i16 h=255; typeset -i8 h` is `8#377`.
	//
	// What is stored is the *rendered text*, which is what every read sees:
	// `${#h}` is 5 for `16#ff`, `g=$h` copies those five characters, a child
	// is told `h=16#ff`, and arithmetic parses it back — `$(( h + 1 ))` is
	// 256. So it is a change to the value and not a way of printing it, and
	// modeling it as a rendering would answer every one of those rows wrong.
	//
	// Base 10 and any base outside what the dialect can spell render plain —
	// measured, ksh93 takes `-i1` and `-i0` in silence and prints `5` for
	// both. Which bases a dialect *can* spell is IntegerBaseDigits, whose
	// length is the largest, and whether it complains about the rest is
	// Diagnostics.IntegerBadBase.
	//
	// Asked only when a base is actually written, so the ordinary `-i` never
	// reaches it and a dialect that has no `typeset` never meets the
	// question at all.
	IntegerAttributeTakesABase Answer
	// IntegerBaseDigits is the alphabet a dialect renders an output base in,
	// and its length is the largest base it can spell.
	//
	// Two facts in one string, because they are one fact about the shell:
	// ksh93 counts in lower case and carries on into upper — `16#ff`,
	// `36#2s`, `64#1A` for 100 — so its alphabet is 62 long, and zsh counts
	// in upper case and stops at 36, `16#FF` and `36#2S`. A dialect with no
	// alphabet renders every base plain, which is what a shell without the
	// feature does.
	IntegerBaseDigits string
	// IntegerBaseComesFromTheValueAssigned learns a name's output base from
	// the radix prefix of the text assigned to it, where no base was named.
	//
	//	typeset -i b; b=0x10; echo "$b"     ksh93 `16`   zsh `16#10`
	//	typeset -i d; d=8#7;  d=99          ksh93 `99`   zsh `8#143`
	//	a=0x10; typeset -i a                ksh93 `16`   zsh `16#10`
	//
	// zsh alone, and it is the half of #1130 that is not about the letter:
	// the base is remembered on the name either way, and what differs is
	// only where the default comes from. It sticks — a later plain `5` under
	// a name that learned 16 is `16#5` — which is what makes it the name's
	// and not the assignment's.
	//
	// Only a *radix* prefix teaches it. A leading zero does not (`016` is
	// `16` in both), and neither does a value that arrived already evaluated:
	// `f=$((0x10))` is `16` in zsh too, the expansion having handed the
	// assignment the four decimal characters.
	IntegerBaseComesFromTheValueAssigned Answer
	// IntegerBaseNegativeIsTwosComplement renders a negative integer in its
	// output base as the bit pattern rather than as a sign and a magnitude.
	//
	//	typeset -i16 h; h=-255    ksh93 `16#ffffffffffffff01`   zsh `-16#FF`
	//	typeset -i2 c=-5          ksh93 sixty-four binary digits   zsh `-2#101`
	//
	// ksh93 prints the sixty-four-bit two's complement and zsh puts the sign
	// in front of the magnitude. Asked only for a negative value in a base
	// that renders at all, so nothing else meets it.
	IntegerBaseNegativeIsTwosComplement Answer
	// IntegerBaseTenIsNoBase makes ten the *default* of the integer letter
	// rather than a base like any other, so that naming it records nothing
	// and writing the letter with no base at all takes off the base a name
	// already has. Measured 2026-09-07:
	//
	//	typeset -i10 d=255; typeset -p d      ksh93 `typeset -i d=255`
	//	                                      zsh   `typeset -i10 d=255`
	//	typeset -i16 a=255; typeset -i a      ksh93 `255`   zsh `16#FF`
	//	typeset -i16 b=255; integer b         ksh93 `255`   zsh `16#FF`
	//	typeset -i16 c=255; typeset -x c      ksh93 `16#ff` zsh `16#FF`
	//
	// The two rows are one answer. ksh93's letter always names a base and
	// ten is what it names when nothing is written, so a bare `-i` is `-i10`
	// and ten is the absence of one — which is what #1130 asked when it
	// asked whether "no base" is a state or just base ten, and the two
	// shells answer it differently. In zsh ten is a state: it is recorded,
	// its listing says `-i10` back, and a later bare `-i` leaves it alone.
	//
	// The value reads the same either way — `typeset -i10 e=255` is `255` in
	// both, ten being the base nothing is written in — so this is not
	// IntegerBaseDigits asked twice. What it changes is the *listing* and
	// what a second declaration does to a base already there.
	//
	// Asked only where the answer changes something: where ten is written,
	// and where the letter arrives bare over a name that has a base. An
	// ordinary `typeset -i n` on a name with no base never meets it, which
	// is nearly every declaration there is.
	//
	// The other letter is the control and needs no answer: `typeset -x` over
	// a based name leaves the base alone in both.
	IntegerBaseTenIsNoBase Answer

	// IntegerPlusFormTakesAttributesOff decides whether a plus word on
	// `integer` removes anything at all.
	//
	// The two shells that have the word disagree about what the word *is*,
	// and the disagreement is not confined to `+i`. Measured 2026-09-06:
	//
	//	integer n=5; integer +i n; n=3+4    zsh 3+4    ksh93 7
	//	integer -x e=1; integer +x e        zsh gone   ksh93 still exported
	//
	// zsh prepends the letter to an ordinary declaration, so every plus form
	// means there what it means on `typeset`. ksh93 has a declaration
	// command of its own whose type is fixed, and a plus form on it removes
	// nothing — `typeset -p n` says `typeset -l -i n=5` there against zsh's
	// plain `typeset n=5`, which is the same finding read from the value
	// side. Its own `typeset +x` does unexport, so this is about the second
	// name and not about the letter.
	//
	// Yes is zsh's answer. No is ksh93's, and it is why the field is not
	// spelled per letter: one reading of the word covers `+i` and `+x`
	// alike, and a field per letter would have been two questions whose
	// answers can only ever agree.
	//
	// Asked only for a plus word on `integer`, which is the only place the
	// two readings differ — every other spelling is parsed identically.
	IntegerPlusFormTakesAttributesOff Answer

	// SetArrayLetter is `set -A name value …`, which assigns an array through
	// a name a variable holds — the thing `name=(…)` cannot do, because the
	// name is a literal there.
	//
	// ksh93 and zsh have the letter; bash refuses it as an invalid option and
	// dash as an illegal one. Which makes it a dialect's answer rather than
	// an axis, and it is **read rather than asked**: where the answer is not
	// yes the letter is somebody else's invalid option, and the refusal
	// already in place is that shell's own real words. Asking an axis there
	// would replace a correct answer with a complaint about a missing
	// dialect.
	//
	// unexhibited No: nobody writes it, and the paragraph above is why:
	// the axis is read (`== Yes`) rather than asked, so where the answer
	// is not yes the letter is somebody else's invalid option and that
	// shell's own refusal already stands. Measured 2026-09-12: `set -A arr
	// x y` is `set: -A: invalid option` in bash 5.3.15, bash-as-`sh` and
	// bash 3.2.57 and `set: Illegal option -A` in dash, each in the
	// shell's own words, and is taken by ksh93u+ and zsh 5.9.2. Writing
	// `No` into those two presets would record no fact the refusal does
	// not already carry (#2060).
	SetArrayLetter Answer

	// SetArrayOptionsContinuePastTheName decides whether the words behind
	// `set -A name` are more options or the array's values.
	//
	// Measured 2026-09-06, and the two shells that have the letter answer
	// opposite ways:
	//
	//	set -A ff -x -y     ksh93 -y: unknown option    zsh [-x -y]
	//	set -A dd -- 1 2    ksh93 [1 2]                 zsh [-- 1 2]
	//
	// ksh93 keeps parsing, so the values are whatever the option parse does
	// not claim — exactly the words that would have become the positional
	// parameters — and a `--` among them still ends the options. zsh stops at
	// the name and every word behind it is a value, dash words and `--`
	// included.
	//
	// **One question, not two.** Both rows above move together, because
	// whether `--` is an operand *is* whether options are still being read.
	// Asked only once the letter is taken, so a shell without it never meets
	// the question.
	SetArrayOptionsContinuePastTheName Answer

	// SetArrayWithNoValuesUnsetsTheName is `set -A name` with nothing after
	// the name: ksh93 unsets it and zsh leaves an array with no elements.
	//
	//	set -A a 1 2 3; set -A a; typeset -p a
	//	  ksh93  nothing at all — the name is gone
	//	  zsh    typeset -a a=(  )
	//
	// Both answer `${#a[@]}` as 0, so the difference shows only through
	// `${a+x}` and a listing — which is exactly what makes it worth a field:
	// a script that tests whether the name is set gets opposite answers.
	//
	// The *plus* form is not this question and needs no field: `set +A a`
	// with no values leaves the array exactly as it was in both, which is
	// unanimous and is a different operation.
	SetArrayWithNoValuesUnsetsTheName Answer

	// JobSpecsByName resolves `%name` — the job whose command begins with
	// the text — and `%?text`, the one whose command contains it. POSIX
	// gives both spellings; dash answers "no such job" to every spec that
	// is not a number, `%%`, `%+` or `%-`.
	JobSpecsByName Answer
	// AmbiguousJobNameIsRefused is `%name` matching more than one job: bash
	// refuses it as an ambiguous job spec where ksh93 and zsh take the most
	// recent match. Asked only on a second match.
	AmbiguousJobNameIsRefused Answer
	// WaitReportsAMissingJob says a job spec `wait` cannot resolve earns a
	// complaint — see Diagnostics.WaitNoSuchJob — and a failing status.
	// ksh93 says nothing at all and reports 0.
	WaitReportsAMissingJob Answer
	// WaitNWaitsForTheNextJob gives `wait` a `-n`: block until whichever
	// job finishes first and report its status, 127 with no jobs at all.
	// bash's letter alone; the other three refuse or misread it.
	WaitNWaitsForTheNextJob Answer
	// WaitForAJobFailsWhenInterrupted has a `wait` that names a job report a
	// plain 1 when a trapped signal cuts it short, rather than the status
	// that signal encodes. True in ksh93 alone, and only with an operand:
	// `wait $!` and `wait %1` both report 1 there where its *bare* `wait`
	// reports 286 for USR1 — 256 plus the signal, its own encoding for a
	// command a signal killed.
	//
	// bash 3.2, bash 5.3, dash and zsh make no distinction between the two
	// forms and report 158 for either, so the preset follows the four that
	// agree. Measured with a background job outliving the signal, so the
	// answer is about the interruption and not about the job's own status.
	WaitForAJobFailsWhenInterrupted Answer
	// DisownRemovesTheJob makes `disown` take the job out of the table, so
	// a later `jobs` no longer lists it: bash and zsh. ksh93's disown only
	// shields the job from the HUP an exiting shell would send — a signal
	// this engine never forwards — and its `jobs` goes on listing the job.
	DisownRemovesTheJob Answer

	// JobsOptions is the set of letters `jobs` takes, spelled the way
	// ReadOptions is. The letters are the dialect's own and the sets are
	// not nested: POSIX and dash have `-l` and `-p` alone, bash adds
	// `-n -r -s -x`, ksh93 adds only `-n`, and zsh adds `-r -s` plus three
	// of its own. Empty means `lp`, which is POSIX's pair and the only one
	// every shell in the panel has.
	//
	// It is a semantics field rather than a constant because a letter one
	// shell has and another has never heard of is a *refusal* in the second
	// one: `jobs -r` lists the running jobs in bash and is an illegal
	// option in dash, and a shared letter set would have this engine accept
	// it everywhere and answer dash's scripts differently from dash.
	JobsOptions string
	// JobsPidsOnlyOption makes `jobs -p` print one process id per line and
	// nothing else — no number, no marker, no state, no command. dash, bash
	// and ksh93 all do; zsh reads the same letter as "put the job's process
	// *group* id in the listing" and prints its ordinary rows, so a
	// `kill $(jobs -p)` written for one of the first three kills nothing
	// there.
	//
	// Asked only where the letter was given, and only in a dialect that has
	// it, so a listing with no `-p` never reaches it.
	JobsPidsOnlyOption Answer
	// JobsStateFiltersAccumulate decides `jobs -r -s`, where both of the
	// state filters are named at once: zsh lists a job matching *either*
	// state, bash lets the last letter given decide and lists only the jobs
	// in that state — so `jobs -rs` there is `jobs -s`.
	//
	// Asked only when both letters arrive together. One of them alone means
	// the same thing in both shells, and the two dialects without the
	// letters cannot reach the question at all.
	JobsStateFiltersAccumulate Answer

	// DeclareGlobalReachesPastALocal is `declare -g x=new` with a `local x`
	// standing in front of the name: bash writes the global cell and leaves
	// the local untouched, zsh assigns the visible cell — the local — and
	// leaves the global alone. Asked only there: with no local in front,
	// both write the global, which is what the letter is for.
	DeclareGlobalReachesPastALocal Answer

	// LocalOptions is the same question asked of `local`, whose answers do
	// not follow `typeset`'s: dash has `local` and gives it no options at
	// all, so `local -r x` declares a variable named `-r` there — and then
	// refuses it as a bad name. Empty means none, dash's answer and the
	// substrate's old behavior.
	LocalOptions string

	// BareLocalListing is what `local` with no operands writes — three
	// shapes from the three shells that can reach it, so it is a form
	// rather than a flag. See BareLocalListingForm.
	BareLocalListing BareLocalListingForm

	// BareTypesetListing is what `typeset` or `declare` with no operands
	// and no letters writes.
	//
	// An axis of its own even though it shares the form type with
	// BareLocalListing, because the
	// shells that have both words do not answer the two the same: zsh writes
	// the identical parameter table either way, and bash's bare `declare` is
	// every variable the shell holds rather than the running function's
	// locals. Only zsh's answer is a value this form already carries, so the
	// others stay unanswered and are refused by name rather than guessed at.
	//
	// unexhibited BareLocalListsLocals: BareLocalListing holds it, for
	// bash's bare `local`. It is not this builtin's answer anywhere:
	// measured 2026-09-12, bash's bare `declare` is every variable the
	// shell holds *and then every function* — `f(){ :; }` lists as `f ()`
	// — which is a fourth reading this form does not carry, and is why
	// bash is unanswered here rather than approximated. Recorded as #1754
	// (#2060).
	//
	// unexhibited BareLocalListsNothing: BareLocalListing holds it, for
	// dash's and ksh93's bare `local`. Neither shell reaches this field:
	// dash has no `typeset` and ksh93u+ has no `local`, so both are
	// unanswered rather than silent-by-measurement (#2060).
	BareTypesetListing BareLocalListingForm

	// SetListing is what `set` with no arguments writes — see
	// SetListingForm. All four list, but not the same things: one follows
	// the variables with every defined function, and one lists special
	// parameters and tied arrays no other shell has.
	SetListing SetListingForm

	// ListingControlEscape is how a `$'...'` listing spells a control byte —
	// see ControlEscapeStyle. A field of its own rather than a part of the
	// quoting style, because two dialects that quote the same way spell a
	// control byte differently.
	ListingControlEscape ControlEscapeStyle

	// SetListingQuoting is how that listing spells a value. The styles are
	// the shared listing vocabulary: bash quotes only where it must and
	// closes-reopens with a backslash, dash single-quotes everything, and
	// ksh93 reaches for `$'...'`.
	//
	// unexhibited ListingQuoteAlwaysEscaped: AliasQuoting holds it, for
	// bash — whose bare `set` quotes only where it must, which is the
	// split these two fields record. Re-measured 2026-09-12 over
	// `Q="a'b"`: a bare `set` writes `Q='a'\''b'` in bash 5.3.15, bash-
	// as-`sh`, bash 3.2.57 and zsh 5.9.2 (#2060).
	//
	// unexhibited ListingQuoteAlwaysDouble: DeclareValueQuoting holds it,
	// for bash's `declare -p`, which is not the style its own `set`
	// listing uses (#2060).
	//
	// unexhibited ListingQuoteWhenNeededPlain: TrapQuoting holds it, for
	// zsh's `trap`. The bare `set` in zsh 5.9.2 reaches for the escaped
	// style instead, measured with the same probe (#2060).
	SetListingQuoting ListingQuotingStyle

	// SelectLayout is how `select` draws its menu. Three engines rather than
	// two answers, which is why it has its own type.
	SelectLayout SelectMenuLayout

	// AliasParsesOptions lets `alias` read leading `-` words as options. True
	// in bash, ksh93 and zsh; dash reads none, so `alias -p` is a name there
	// and the answer is "-p not found" rather than a refusal.
	AliasParsesOptions Answer

	// AliasHasPrintOption gives `alias` a `-p`, which prints the listing with
	// `alias ` in front of every line. bash and ksh93 have it — and it is
	// what bash's plain listing already looks like, so it is only visible in
	// ksh93. dash parses no options for `alias` at all, so `-p` is a *name*
	// there and the answer is "not found"; zsh has options and refuses it.
	AliasHasPrintOption Answer

	// GlobalAliases gives this dialect the second kind of alias: `alias -g
	// name=value` defines one, and a word naming one is expanded *wherever
	// it stands* rather than only where a command word does — in an
	// argument, a `for` list, a `case` pattern, a redirection target, a
	// heredoc delimiter, a `[[ ]]` word. The value is spliced as tokens like
	// any other alias body, so `alias -g UP="| tr a-z A-Z"` puts a pipeline
	// in the middle of a line.
	//
	// zsh alone, measured: bash calls `-g` an invalid option, ksh93 an
	// unknown one, and dash reads no options at all and looks for an alias
	// called `-g`. It shares a table with the regular kind there — `alias -g`
	// over a regular name replaces it — and the plain listing shows both,
	// which is why the two are one field rather than one table each.
	//
	// The letter is the visible half; the expansion is the feature. A
	// dialect answering Yes and expanding nothing would list an alias it
	// never uses, which is the shape #2081 was filed against in reverse.
	//
	// `make axis-sweep` pins this in three of the four dialects and cannot
	// in dash, which is a fact about dash rather than a gap: `alias` there
	// reads no options at all — AliasParsesOptions is No — so the accepted
	// set is never consulted, and no shell in the panel has an `unalias -g`
	// for it to be consulted from either. The answer has no reachable
	// consequence in that dialect, which is the third of the four triages
	// docs/spec/semantics.md lists. SuffixAliases is pinned in all four,
	// because `unalias -s` reads the axis whatever `alias` does with its
	// operands.
	GlobalAliases Answer

	// SuffixAliases gives this dialect the third kind, which is a second
	// *namespace*: `alias -s ext=value` keys on a command word's extension,
	// and a command word `text.ext` — text non-empty, ext the run after the
	// last dot — is replaced by the text `value text.ext`. So `alias -s
	// txt=cat` makes `./x.txt` into `cat ./x.txt`.
	//
	// zsh alone, measured, and it is a parse-time substitution there rather
	// than a fallback for a command that was not found: it beats an
	// executable of that name on PATH and a function of that name, and loses
	// to a regular alias of that name, which is exactly the order a
	// substitution done while reading the line produces. A value holding a
	// pipeline splices one in.
	//
	// The namespace is the half a single flag could not say: the two sets
	// are never listed together, `unalias -a` empties the other table and
	// leaves this one, and `unalias -s` is the only way to remove one —
	// which is also why this field is read by `unalias` as well.
	SuffixAliases Answer

	// AliasListsAsDefinitions gives `alias` a `-L`, which writes every line
	// as a command that would define the alias back: `alias ` in front, and
	// the kind's own letter where the entry is not the regular kind — `alias
	// -g UP='| tr a-z A-Z'`, `alias -s txt=cat`.
	//
	// zsh alone, measured 2026-09-12. It is what a startup file wants and
	// what the plain listing cannot be, since the plain listing is `name=value`
	// there and says nothing about which kind an entry is. The letter is a
	// listing form and not a filter: `alias -L`, `alias -g -L` and `alias -s
	// -L` each list what the kind letter alone would have listed.
	AliasListsAsDefinitions Answer

	// AliasRestrictsToRegularKind gives `alias` a `-r`, the kind letter for
	// "neither global nor suffix".
	//
	// zsh alone, and it exists there because the other two kind letters
	// leave no way to ask for the plain ones: the shared table holds the
	// regular and the global aliases together and the plain listing shows
	// both. It is a kind like `-g` and `-s` rather than a modifier on them,
	// so `alias -r -g` and `alias -rs` are `illegal combination of options`
	// exactly as `alias -gs` is.
	AliasRestrictsToRegularKind Answer

	// AliasOperandsCanBePatterns gives `alias` and `unalias` a `-m`, which
	// reads every operand as a pattern rather than as a name.
	//
	// zsh alone. Read by both builtins because one letter serves both there,
	// and they differ in what an absent operand means: `alias -m` with
	// nothing after it is the plain listing at 0, and `unalias -m` with
	// nothing after it is `not enough arguments` at 1 — a removal with no
	// pattern would be a removal of everything, which is what `-a` is for.
	//
	// A pattern that matches nothing is 0 for `alias` and 1 for `unalias`,
	// which is the same shape as a name that is not there.
	AliasOperandsCanBePatterns Answer

	// AliasPlusPrintsNamesOnly makes `+g`, `+r`, `+s` and a bare `+` list the
	// names without the values.
	//
	// zsh alone, and the plus words are not option letters in the ordinary
	// sense: a bare `+` also *ends* the option list, so `alias + -L` looks up
	// an alias called `-L` where `alias -L +` lists everything in the `-L`
	// form. `-L` wins over the names-only reading when both are written,
	// measured: `alias -L +g` is the full definition line.
	//
	// `unalias` has none of them — `unalias +m x` looks for hash table
	// elements called `+m` and `x` — so this is read by `alias` alone.
	AliasPlusPrintsNamesOnly Answer

	// TypeNamesAnAliasOnlyWhenExpanded holds `type`, `command -v` and
	// `command -V` silent about an alias while alias expansion is off.
	//
	// bash alone, and it is a real difference rather than a detail of how a
	// program arrived: `alias a='echo hi'; type a` in a `-c` string is
	// `type: a: not found` there while `alias` lists the entry one line
	// earlier, and `shopt -s expand_aliases` in front of it makes the same
	// call answer. The other three answer from the table whatever the switch
	// says — zsh names one after `unsetopt aliases`.
	//
	// So the three builtins report what *would run*, in the dialect where an
	// alias that cannot expand would not run, and report what the table
	// holds in the dialects where the two are never apart.
	TypeNamesAnAliasOnlyWhenExpanded Answer

	// AliasReportsNotFound says something when `alias` is given a name the
	// table does not hold. True in bash, dash and ksh93; zsh reports 1 and
	// prints nothing.
	AliasReportsNotFound Answer

	// UnaliasReportsNotFound is that question for `unalias`, and the panel
	// does not pair the two: ksh93 complains about `alias nope` and is silent
	// about `unalias nope`, and zsh does exactly the reverse. One field could
	// not say that.
	UnaliasReportsNotFound Answer

	// AliasNotFoundStatusCounts makes `alias` report how many names it could
	// not find rather than a plain 1: `alias n1 n2 n3` is 3 in ksh93 and 1 in
	// the other three.
	//
	// About `alias` alone — ksh93's own `unalias` answers 1 however many were
	// missing — so it is asked where the count is known and not where the
	// complaint is printed.
	AliasNotFoundStatusCounts Answer

	// UnaliasAllRefusesOperands makes `unalias -a name` an error that clears
	// nothing. zsh alone: "-a: too many arguments", status 1, table intact.
	// The other three take the `-a`, ignore the names and empty the table.
	UnaliasAllRefusesOperands Answer

	// AliasQuoting is how a value is spelled in a listing — four engines, no
	// two alike. See ListingQuotingStyle.
	//
	// unexhibited ListingQuoteAlwaysDouble: DeclareValueQuoting holds it,
	// for bash's `declare -p` — the engine that double-quotes its
	// declarations single-quotes its aliases, which is the whole reason
	// that field is separate. Re-measured 2026-09-12: `alias al="echo
	// a'b"` lists as `al='echo a'\''b'` in the three bash columns and zsh
	// 5.9.2, `al='echo a'"'"'b'` in dash and `al=$'echo a\'b'` in ksh93u+
	// (#2060).
	//
	// unexhibited ListingQuoteWhenNeededPlain: TrapQuoting holds it, for
	// zsh, whose trap listing never reaches for `$'...'` where its alias
	// listing does — the paragraph on TrapQuoting is the measurement
	// (#2060).
	AliasQuoting ListingQuotingStyle

	// TrapQuoting is that same question asked of `trap`, and it is a
	// separate field because one dialect answers the two differently: zsh
	// writes an alias holding a tab as `$'a\tb'` and a trap holding one as
	// a plainly quoted `'a<tab>b'`.
	//
	// unexhibited ListingQuoteWhenNeededEscaped: AliasQuoting and
	// DeclareValueQuoting hold it, for zsh, which is the split this field
	// exists to record: the same shell writes an alias holding a tab as
	// `$'a\tb'` and a trap holding one plainly (#2060).
	//
	// unexhibited ListingQuoteAlwaysDouble: DeclareValueQuoting holds it,
	// for bash's `declare -p`. bash's own `trap -p` uses the escaped
	// single-quote style instead — measured 2026-09-12, `trap -- 'echo
	// a'\''b' SIGUSR2` (#2060).
	TrapQuoting ListingQuotingStyle

	// TrapActionIsParsedWhenSet reads a trap's action when the trap is set
	// rather than when it fires, and refuses a trap whose action will not
	// parse.
	//
	// zsh alone. The other three store the text: `trap "if" EXIT` is taken
	// and complains at the end, and `trap "if" INT` is taken and never
	// complains at all, because the trap never fires.
	TrapActionIsParsedWhenSet Answer

	// TrapBodyRunsWhatParsed runs each line of a trap's body as it parses,
	// so the part before a syntax error has already run by the time the
	// error is reported.
	//
	// bash and dash do — `trap "echo a
	// if" EXIT` prints `a` and then complains. ksh93 reads the whole body
	// first and prints nothing. zsh answers no by construction rather than
	// by measurement: it reads the action when the trap is set, so by the
	// time a trap fires the whole body has parsed and there is no partial
	// run to have. The two answers cannot be told apart there.
	TrapBodyRunsWhatParsed Answer

	// TrapParseFailureNamesWhereItFired puts the runtime location in front
	// of a trap body's parse failure — where the trap fired — rather than
	// the line the parse gave out on.
	//
	// ksh93 alone, and the two are different numbers: a body set on line 2
	// and fired from line 5 reports `w5.sh: line 5: syntax error at line 6`.
	// bash and dash name the parse position in both places. zsh is not
	// asked, because it reads the action when the trap is set and never
	// reaches a parse failure at fire time.
	TrapParseFailureNamesWhereItFired Answer

	// SymbolicMaskTakesMoreThanOneOperator lets one `umask` clause turn on
	// several: `umask u+rw-x` is 0122 from 022 in three of the four. zsh
	// takes a single operator per clause and names the second one.
	SymbolicMaskTakesMoreThanOneOperator Answer

	// SymbolicMaskWhoAloneSetsIt reads `umask g` as `umask g=`, denying that
	// group everything. ksh93 alone. bash and dash refuse it, and zsh
	// answers it with the complaint it gives a number it could not read.
	SymbolicMaskWhoAloneSetsIt Answer

	// SymbolicMaskTakesTheSetuidLetter accepts `s` in a clause, which
	// changes no bits — a umask has no setuid bit to deny — and is accepted
	// by three of the four all the same. zsh refuses it.
	SymbolicMaskTakesTheSetuidLetter Answer

	// SymbolicMaskTakesTheStickyLetter is the same question about `t`, and a
	// different set of shells: bash and ksh93 take it, dash and zsh do not.
	// Two fields because the two letters are not answered together.
	SymbolicMaskTakesTheStickyLetter Answer

	// ShiftOptionWords is which leading-`-` words `shift` reads as options
	// rather than as its count, and it is three answers rather than a
	// presence — see ShiftOptionWordPolicy.
	//
	//	shift -x   bash, dash  -x: the count, and not a number
	//	           ksh93, zsh  -x: an option, and not one they have
	//	shift -1   bash, dash, zsh  -1: the count
	//	           ksh93            -1: an option, and not one it has
	//
	// zsh is what makes this three: it refuses `-x` as an option and reads
	// `-1` as a count that is out of range, so "reads options" and "reads
	// every dash word as an option" are not the same answer.
	//
	// A lone `-` is not a dash word in any reading here and reaches the
	// count, which is bash's and dash's answer for it; ksh93 and zsh each
	// do something else with that one word and neither is modeled — see
	// docs/spec/semantics.md. Nor is `--` a dash word, which is asked about
	// separately — see ShiftDoubleDashEndsOptions.
	//
	// Asked only for a word that actually begins with a `-`.
	ShiftOptionWords ShiftOptionWordPolicy
	// ShiftDoubleDashEndsOptions takes `--` as the end-of-options marker and
	// reads what follows as the count. bash, ksh93 and zsh do; dash calls
	// `--` an illegal number, having no option parsing here for a marker to
	// end.
	//
	// It is not ShiftOptionWords: bash reads no dash word as an option and
	// still honors the marker, so the two questions have different answers
	// in the same shell. Only the *first* `--` is the marker —
	// `shift -- --` complains about the second in all three that take it.
	//
	// Asked only where the operand actually is `--`.
	ShiftDoubleDashEndsOptions Answer
	// ShiftNamesAreArrays reads `shift`'s operands as the names of arrays to
	// shift, instead of the positional parameters. Only zsh, whose synopsis
	// is `shift [ n ] [ name ... ]`; bash calls a name a `numeric argument
	// required` and a second operand `too many arguments`, and ksh93
	// evaluates the word arithmetically, reaching either the array's first
	// element or a `bad number`.
	//
	// The count stays optional in front of them, so the first word is
	// ambiguous — and zsh settles it by *type*, measured:
	//
	//	q=5;   a=(1 2 3 4 5 6); shift q a   # a=(6)          — q is a count
	//	q=(5); a=(1 2 3 4 5 6); shift q a   # q=() a=(2 … 6) — q is a name
	//
	// So a word that names an array is a name and every other word is an
	// arithmetic count, which is why `shift a a` shifts `a` twice rather
	// than once by its first element: arithmetic on an array name is a `bad
	// math expression` in zsh, so an array can only ever have been a name.
	//
	// A name that is not an array — unset, a scalar, an association — is
	// left alone and not complained about, at status 0. What protects the
	// positional parameters is giving *any* operand, not the operand turning
	// out to name an array: `set -- x y z; shift 1 nosuch` leaves `$@` where
	// it is. A first word that is a scalar is the count, though, and shifts
	// them like a literal one — `s=9; shift s` overruns three positional
	// parameters and says so. A count past the end
	// of one array is reported (Diagnostics.ShiftTooMany) and the remaining
	// names are still shifted, so `shift 2 a b` with a one-element `a` is
	// status 1 with `b` shifted. That carrying-on is why this may only be
	// answered where ShiftPastEndFatal is No, which is zsh.
	ShiftNamesAreArrays Answer
	// ShiftNegativeIsOutOfRange reads a negative count as a number that is
	// out of range rather than as a word that is not a number. bash, ksh93
	// and zsh do, at status 1 and in three different wordings
	// (Diagnostics.ShiftNegativeCount); dash calls `-1` an illegal number,
	// which is the same complaint it makes about `-x`.
	//
	// It is the other end of ShiftTooMany — one count, out of range in two
	// directions — so the same ShiftPastEndFatal decides whether it ends the
	// script, and it does: fatal in dash and ksh93, survivable in bash and
	// zsh, with `$#` untouched either way.
	//
	// Asked only where the count really is negative, which in ksh93 means
	// only after a `--`: a bare `-1` is an option there.
	ShiftNegativeIsOutOfRange Answer

	// WaitReadsOptions reads a leading `-` word as an option rather than as
	// a job to wait for. Three of the four do; zsh has none, and answers
	// `wait -x` with the job it could not find.
	WaitReadsOptions Answer

	// CommandRejectsUnknownOption refuses a leading `-` word that is not one
	// of `command`'s own options, rather than taking it as the command.
	//
	// All four read `-v` and `-p`. bash, dash and ksh93 refuse anything
	// else; zsh alone stops reading options there, so `command -q ls` is
	// `command not found: -q` in zsh and a refused option in the other
	// three. The same shape printf already has. Recorded as
	// `cmd/command-with-an-option-nobody-has`, with a letter no panel shell
	// owns: the first probe used -x, which is a real ksh93 option, and read
	// ksh93 as tolerant off ksh93's own feature.
	CommandRejectsUnknownOption Answer

	// GetoptsRejectsUnknownOption is the same question for `getopts`, which
	// has no options at all here — so any leading `-` word is the one being
	// asked about, and it would otherwise be the optstring.
	//
	// bash and ksh93 refuse it: `getopts -q o` is an unknown option there
	// and an optstring of `-q` in dash and zsh. Recorded as
	// `getopts/a-dash-word-where-the-optstring-belongs`, with a letter no
	// panel shell owns: ksh93 has `-a` for real, and the first probe used
	// it — ksh93's answer stood, wrongly, at No until the probe was rerun
	// with -q.
	GetoptsRejectsUnknownOption Answer

	// ShiftCountIsArithmetic reads `shift`'s operand as an expression rather
	// than as a plain number: `shift 1+1` moves two and `shift n` moves
	// whatever n holds.
	//
	// ksh93 and zsh do. An unset name is zero in an expression, so
	// `shift abc` shifts nothing and succeeds there, where bash and dash
	// call it a number they cannot read.
	ShiftCountIsArithmetic Answer

	// ReportsAKilledCommandInACommandSubstitution remarks on a command that
	// a signal ended inside `$(…)`.
	//
	// bash does not, and does remark on the same command inside `( … )`, so
	// this is not the subshell question in another spelling. dash and ksh93
	// report it wherever it happened; zsh remarks on none of them and never
	// reaches this.
	//
	// Asked only inside a substitution, so the three dialects that answer
	// the wider question the same way everywhere are not asked twice.
	ReportsAKilledCommandInACommandSubstitution Answer

	// TrapBodyLine is which lines a diagnostic from inside a trap's body
	// names. See TrapBodyLineStyle.
	TrapBodyLine TrapBodyLineStyle

	// ExitTrapFiresPastTheEnd counts the EXIT trap as having fired on the
	// line after the script's last, rather than on its first.
	//
	// Only asked by a dialect whose TrapBodyLine needs a firing line at all,
	// and only for EXIT, which has no line of its own. zsh says yes: its
	// EXIT trap reports the line the parser stopped at. ksh93 says no, which
	// makes an EXIT body read like a small script of its own.
	ExitTrapFiresPastTheEnd Answer
	// SelectPromptNeedsTerminal withholds PS3 unless the input is a terminal.
	// ksh93 alone says yes, which is why a ksh93 script's transcript has the
	// menu in it and no prompt.
	SelectPromptNeedsTerminal Answer
	// SelectTakesUnterminatedReply counts a final reply that has no trailing
	// newline. zsh alone: `printf 2 | sh -c 'select x in a b; do ...'` picks
	// `b` there, and bash and ksh93 ignore the line and end the loop with 1.
	//
	// The same question `read` answers, and the opposite outcome — the panel
	// is unanimous for `read` and split here, so that one is the core's
	// behavior and this one is an axis. Reachable only from a pipe or a file,
	// since a terminal ends every line.
	SelectTakesUnterminatedReply Answer

	// SelectEofIsSuccess makes the input running out a success. zsh alone
	// says yes; the other two report 1.
	SelectEofIsSuccess Answer
	// SelectAssumesUnboundedWidth treats an unset COLUMNS as no limit rather
	// than as 80. zsh says yes — with no terminal to ask it puts forty items
	// on one line — and bash says no. It does not arise for a menu that is
	// always vertical, which is why ksh93 leaves it unanswered.
	SelectAssumesUnboundedWidth Answer
	// SelectEofEndsPromptLine writes a newline to standard error when the
	// input runs out, closing the line the prompt left open. zsh alone does;
	// bash closes the line on standard *output* instead, which is a different
	// question and the field below.
	SelectEofEndsPromptLine Answer
	// SelectEofPrintsNewline writes a newline to standard *output* when the
	// input runs out — the one thing this loop prints that does not go to
	// standard error. bash alone does it.
	SelectEofPrintsNewline Answer

	// AssignmentUpdatesPipelineStatus counts a bare assignment as a command
	// for the pipeline-status record. bash says yes, so `false | true; x=1`
	// replaces the two elements with one holding 0; zsh says no and leaves
	// them.
	//
	// A bare assignment is one with no command name *and no redirection*:
	// `false | true; x=1 >/dev/null` replaces the elements in zsh too,
	// because the redirection is what makes it a job. Negation does the
	// same, and for the same reason — see recordSingleStatus.
	AssignmentUpdatesPipelineStatus Answer
	// TestAndArithmeticUpdatePipelineStatus counts `[[ … ]]` and `(( … ))`
	// as commands for the pipeline-status record. bash says yes; zsh says no
	// and leaves the elements the last pipeline left, which is what makes
	// the shape real code uses work:
	//
	//	cmd | filter
	//	if (( pipestatus[1] == 141 )); then …
	//	elif (( pipestatus[1] )); then print "failed ($pipestatus[1])"
	//	fi
	//
	// With yes, the first `(( … ))` overwrites the array it just read, the
	// `elif` reads that instead, and the message reports the status of the
	// test rather than of the pipeline — a genuine failure printed as 0.
	//
	// One axis for the two constructs because no shell separates them: every
	// panel member that has the record answers both the same way. The two
	// are grouped with the bare assignment above by what they are not — zsh
	// runs all three without making a job, and a job is what writes the
	// record.
	TestAndArithmeticUpdatePipelineStatus Answer
	// NegatedTestRecordsThePostNegationStatus writes the status a `!` in
	// front of `[[ … ]]` or `(( … ))` produced, rather than the one the
	// construct itself reported.
	//
	// Measured 2026-09-11, after `false | true` so the record is visibly
	// replaced:
	//
	//	                       ! [[ a = a ]]   ! [[ a = b ]]   ! false
	//	bash 5.3.15            1               0               1
	//	bash 3.2.57            1               0               1
	//	zsh 5.9.2              0               1               1
	//
	// The first two columns are the axis and the third is why it is confined
	// to these two constructs: an ordinary command records what *it*
	// reported in both shells, so `!` is not a rule about negation in
	// general. `! { [[ a = a ]]; }` and `! ( [[ a = a ]] )` record 0 in bash
	// as well — the compound reports its own status and the `!` does not
	// reach the record — which is what says this is about the construct and
	// not about the shape of the line.
	//
	// A redirection does not move it: `! [[ a = a ]] >/dev/null` is 1 in
	// bash, the same as without one, where a redirection *does* move the
	// axis above. So the two are asked separately even though they name the
	// same two constructs.
	//
	// Silent when it is wrong: a plausible one-element record, no diagnostic,
	// and a script branching on `${PIPESTATUS[0]}` after a negated test reads
	// the opposite of what the shell it was written for reports (#1513).
	NegatedTestRecordsThePostNegationStatus Answer
	// PromptAsksAgainAfterARefusedToken draws the continuation prompt for a
	// construct the parser has **refused**, rather than refusing it where it
	// stands.
	//
	// Read by the front end rather than by the interpreter: it is about what a
	// prompt does with a line, which is `repl`'s to do and `driver`'s to carry
	// — the same shape as PlusSignedCommandStringIsDollarZero, and a plain
	// bool for the same reason, since a prompt has no way to refuse to run
	// over an unanswered axis.
	//
	// Measured 2026-09-11, `printf 'echo one\nif; then\necho three\n'` into
	// each shell under `-i` with PS1 and PS2 set:
	//
	//	bash 5.3.15  refuses at once, no PS2, and then runs `echo three`
	//	ksh93u+      the same
	//	dash         the same
	//	zsh 5.9.2    draws PS2 and waits
	//
	// `while; do` splits the panel the same way, and `for do` and `case in`
	// split it *neither* way — every shell prompts for those, because they
	// are input that has not finished rather than input that is wrong.
	//
	// The cost of answering yes where the shell answers no is a command
	// disappearing: the next line typed is read as part of the construct
	// already refused, so `echo three` above never runs (#1893).
	PromptAsksAgainAfterARefusedToken bool

	// CompoundPipelineStatusRecord is what a compound command does to the
	// pipeline-status record — see CompoundPipelineStatusPolicy, whose two
	// answers are two *mechanisms* rather than two values for one rule.
	//
	// The first is a question about the **parse** and not about what ran,
	// which is what makes it worth an axis at all. Measured 2026-09-11,
	// zsh 5.9.2,
	// each line after `false | true` so a replaced record is visible:
	//
	//	false | true; if [[ a = b ]]; then :; fi              0
	//	false | true; if [[ a = b ]]; then [[ b = b ]]; fi    1 0
	//
	// Neither body runs — the condition is false both times — and the only
	// difference is the text inside `then`. An **unexecuted** `:` is enough
	// to make the compound count as a command.
	//
	// So the rule composes: a compound counts where its body holds anything
	// that would count on its own, by the two axes above, and nothing else.
	//
	//	{ :; }                       0      { [[ a = a ]]; }        1 0
	//	{ [[ a = a ]]; :; }          0      { x=1; }                1 0
	//	{ { :; } }                   0      { { [[ a = a ]]; } }    1 0
	//	while false; do [[ a=a ]]; done  0  while [[ a = b ]]; do [[ a=a ]]; done  1 0
	//
	// The `while` pair is the one that says the condition is body too, and
	// the nested pair is the recursion. Four shapes answer for themselves
	// whatever they hold, and each is measured:
	//
	//	( [[ a = a ]] )       0     a subshell is a job however it ends
	//	{ coproc cat; }       0     and so is a coprocess
	//	{ [[ a = a ]] & }     0     and so is anything backgrounded
	//	{ time [[ a=a ]]; }   0     and the timed pipeline reports
	//	{ f() { :; }; }       1 0   a *definition* runs nothing
	//	select … do :; done   0     counts with a body that would not
	//
	// A redirection on the compound writes the record whatever the body says
	// — `{ [[ a = a ]]; } >/dev/null` and `if … fi >/dev/null` are both one
	// element — which is the same rule the two axes above follow, and for the
	// same reason: the redirection is what makes the job. That is this
	// mechanism's rule and not the other's; see the table below.
	//
	// The second mechanism is bash's, and it is not this one's opposite: the
	// compound writes **nothing**, and the record it leaves is whatever the
	// last pipeline that actually ran inside it wrote. Measured 2026-09-11 on
	// bash 5.3.15 and 3.2.57 alike, each line after `false | true`:
	//
	//	if false; then :; fi              1     the condition ran and wrote it
	//	if [[ a = b ]]; then :; fi        1     and so did this one
	//	while false; do :; done           1
	//	case a in b) :;; esac             1 0   nothing ran: the record stands
	//	for i in ; do :; done             1 0
	//	{ [[ a = a ]] & }                 1 0   a background job writes nothing here
	//	{ [[ a = a ]] | [[ b = b ]]; }    0 0   two elements survive the braces
	//	{ :; }                            0     the `:` inside wrote it
	//	if false; then :; fi >/dev/null   1     a redirection does not change it
	//
	// The two-element rows are what say it is the inner *pipeline* rather than
	// the compound's last command, and the redirection row is where the two
	// mechanisms part company about the rule the neighboring axes follow.
	//
	// A subshell is not a compound for this purpose in either of them: it
	// reports its own status, one element, `( false | true | false )` leaving
	// `1` after a record of `1 0`.
	//
	// Silent when it is wrong, both ways: a plausible one-element record where
	// the pipeline's elements should still be there (#1931, #2016).
	CompoundPipelineStatusRecord CompoundPipelineStatusPolicy
	// UnsetEndsTheProducedPipelineStatus makes `unset` permanent. zsh says
	// yes and the name never fills again; in bash the producer outlives it.
	// It is the opposite of what a produced *scalar* does, where unset ends
	// it in both — `unset RANDOM` leaves an ordinary empty name everywhere.
	UnsetEndsTheProducedPipelineStatus Answer

	// ArrayScalarIsTheWholeArray decides what a plain `$a` gives when `a` is
	// an array: zsh says every element joined by a space, and bash and ksh93
	// say the first element alone. dash has no arrays, which is why the axis
	// is absent rather than false there.
	ArrayScalarIsTheWholeArray Answer

	// KeyedTableScalarIsTheFirstValue says *which* element a plain `$m`
	// gives when `m` is a keyed table and the axis above has answered "one
	// element". Two readings, and they are the same disagreement about
	// whether such a table has an order at all: bash and ksh93 look up the
	// key `0` and hand back nothing when there is no such key, while the
	// shell that reads a table as an ordered list hands back the first value
	// in whatever order it lists.
	//
	// A second axis rather than a widening of the first, because the shells
	// that share the first answer do not share this one, and because it is
	// reachable only after the first has been answered — a dialect where a
	// bare name is the whole table never asks it.
	//
	// Measured 2026-09-10 on zsh 5.9.2 under the option that moves the array
	// axes, against bash 5.3.15 and ksh93u+ with the same table:
	//
	//	m=(a 1 b 2)   $m   zsh 1      bash, ksh93 empty
	//	m=(z 9 a 1)   $m   zsh 9      so it is the order and not the sort
	//	m=(a 1 0 x)   $m   zsh 1      and not the key `0` under another name
	//
	// "First" is whatever order `${m[@]}` yields, which is a separate
	// question from this one — the axis says which end of the order to read
	// and not what the order is. That order is **this implementation's own**,
	// and deliberately: see KeyedTableOrder in docs/spec/semantics.md, where
	// the panel is measured. It is not insertion order in any shell measured,
	// so this axis agrees with the shell it was taken from exactly when the
	// two orders happen to coincide — which is a table of one, and a table
	// whose keys hash into their sorted order (#1758).
	KeyedTableScalarIsTheFirstValue Answer

	// ArrayBaseIsZero indexes arrays from 0. True in bash and ksh93, false in
	// zsh, which counts from 1. dash has no arrays at all, which is why the
	// axis is absent rather than false there.
	ArrayBaseIsZero Answer

	// BareSubscriptIsASubscript reads the `[…]` an *unbraced* `$name`
	// carries as a subscript, rather than as three ordinary characters
	// behind the parameter. `$a[1]` is an element where it says yes and
	// `${a[0]}` followed by `[1]` where it says no; `${a[1]}` is unaffected
	// either way, because the braces settle where the expansion ends.
	//
	// An axis rather than a grammar flag, and the difference from
	// syntax.Dialect.BareSubscript is the whole point. That flag decides
	// whether a grammar has the construct at all — whether the brackets
	// belong to the expansion or are the next thing in the word — and it is
	// answered when the word is read. This decides what the construct
	// *means*, and it is answered when the word is expanded: the one shell
	// with the grammar moves this at run time, and a function body written
	// under one answer and called under the other takes the caller's.
	// Deciding it while reading gives a shell that is right in a script and
	// wrong in `eval`, or the reverse.
	//
	// Only reachable where the grammar flag is on, which is why the presets
	// that have no such construct leave it unanswered rather than false: a
	// dialect that turns the grammar on and does not answer this is a gap,
	// and should say so out loud rather than pick a side.
	//
	// The two halves of the no answer are one answer. The parameter loses
	// the subscript *and* the brackets become text, and the text is the
	// word's like any other — expanded, split and read as a pattern where
	// the word around it would be. Measured on the shell with the
	// construct: `a=(x y z)` and the option that says no gives `x[1]` for
	// `$a[1]`, `b=2` makes `$a[$b]` into `x[2]`, and an unquoted `$a[1]` is
	// the pattern `x[1]` — which is the point of saying no at all, since it
	// is what leaves a `$dir[0-9]*` written in a script for another shell
	// the glob its author meant.
	//
	// unexhibited No: zsh under `setopt ksharrays`, at run time — the same
	// shape FunctionLocalTraps has, and wired in dialect/zsh/ksharrays.go
	// rather than in the preset. Measured 2026-09-12: `a=(x y z); echo
	// "$a[1]"` is `x` in zsh 5.9.2 and `x[1]` under `ksharrays`. The other
	// five columns print `x[1]` too and do *not* hold this value: with the
	// grammar flag off the brackets were never part of the expansion, so
	// they never answer the axis at all (#2060).
	BareSubscriptIsASubscript Answer

	// SubscriptCommaIsARange reads the comma in `${a[1,3]}` as the separator
	// of a range — elements 1 through 3 — rather than as the arithmetic comma
	// operator, whose value is its right operand and names element 3 alone.
	//
	// The same characters with two meanings, which is what puts it here
	// rather than in a grammar flag: `${a[1,3]}` is one subscript in every
	// shell that has subscripts at all, and they disagree about what it
	// says. Measured on `a=(w x y z)`: zsh 5.9.2 gives `w x y`, and bash
	// 5.3.15, bash 3.2.57, bash as `sh` and ksh93 all give `z`. dash has no
	// subscript to read.
	//
	// Asked only where the two readings differ, which is what keeps
	// `${a[2,2]}` — one element under either — from needing an answer.
	SubscriptCommaIsARange Answer

	// SubscriptIsAQuotingContext runs an associative array's subscript
	// through quote removal, so the key is the text *inside* its quotes and
	// escapes. True in bash and ksh93; false in zsh, where the subscript is
	// taken exactly as written — substitutions performed, and every other
	// character, quotes and backslashes included, kept.
	//
	// Measured 2026-09-07. Storing under one spelling and reading with the
	// other is what makes it visible, because a key that is one string in
	// both shells hides it:
	//
	//	m["k"]=W; kk='"k"'   ${m[$kk]}  bash, ksh93 ""    zsh W
	//	                     ${m[k]}    bash, ksh93 W     zsh ""
	//	v=k; q[k]=K          ${q["$v"]} bash, ksh93 K     zsh ""
	//	                     ${q[$v]}   bash, ksh93 K     zsh K
	//
	// The last pair is the crisp form of it: the substitution is performed
	// under both answers and only the quote characters around it differ, so
	// this is a rule about *quoting* and not about expansion.
	//
	// It is one rule with the search operand PR #1101 landed, reached from
	// the other side — `${b[(r)"beta"]}` finds an element whose value is the
	// six characters `"beta"` — which is why the two share
	// Runner.searchOperand rather than reconstructing the text twice.
	//
	// Two things stay unanimous and must not move with it. A bare `@` or `*`
	// is still the whole array in all three; *quoted*, it is a key, so
	// `${n["@"]}` looks one up and finds nothing. And no shell in the panel
	// space-trims an associative key: `${p[ s ]}` looks up three characters
	// in all three, so a key stored under `s` is not found by it. dash has no
	// arrays, which is why the axis is absent there rather than false.
	SubscriptIsAQuotingContext Answer

	// PatternEscapeReaches is the set of characters a backslash escapes
	// inside a pattern. Empty means **every** character, which is bash's
	// answer, bash 3.2's, bash as `sh`'s, dash's and ksh93's: `bet\a` matches
	// `beta` there, the backslash spent on a character that needed none.
	//
	// zsh names a set instead, and it is exactly its pattern
	// metacharacters — a backslash before anything else is a literal
	// backslash *and* the character after it, so `bet\a` matches the five
	// characters `bet\a` and matches `beta` not at all.
	//
	// Measured 2026-09-07 by handing the matcher a raw backslash, which is
	// the only way to ask: quote removal takes an escape off a pattern
	// written in the source before the matcher ever sees it, so a `case`
	// pattern spelled `bet\a` is `beta` in all six and says nothing about
	// this. What does ask it is a *substituted* pattern — `p='bet\a'; case
	// beta in $p)` in the five shells that match the result of an expansion,
	// and `setopt globsubst` with the same two lines in zsh, which does not.
	//
	//	escaped in zsh:      - = ! * ? [ ] ( ) | ^ ~ # < >
	//	not escaped in zsh:  letters, digits, _ . / + : % & @ , " ' space { } $
	//
	// The set is written down rather than derived from the other pattern
	// answers because it is not the same set: `-`, `=`, `!`, `^`, `~` and `#`
	// are in it, and this matcher gives none of the six a meaning of its own
	// in a pattern.
	//
	// One row is deliberately not modeled. dash escapes every character but
	// `^`: a pattern `x\^y` does not match `x^y` there, where `x\.y` matches
	// `x.y`. One character of one shell, recorded in the corpus and filed
	// rather than given a value here.
	PatternEscapeReaches string

	// PatternClasses is the character-class names a dialect answers **beyond
	// the twelve POSIX ones**, space separated. Empty is the common ground:
	// alnum, alpha, blank, cntrl, digit, graph, lower, print, punct, space,
	// upper and xdigit, which every shell in the panel answers alike.
	//
	// A roster rather than a flag per name, and a roster rather than an
	// enumeration, for the reason EchoOptions and ReadOptions are strings:
	// what differs between shells is **which names exist**, and that is data.
	// What each name *means* is not in dispute — no shell disagrees with
	// another about `ascii` — so there is no axis to switch, only a set to
	// declare. A dialect that leaves it empty is not unanswered; it is saying
	// the twelve and nothing else, which is the measured answer for two of
	// the five.
	//
	// Measured 2026-09-10, `[[ $c = [[:NAME:]] ]]` a character at a time:
	//
	//	ascii       zsh, bash 5.3, bash 3.2 — not ksh93, not dash
	//	IDENT       zsh alone
	//	IFS         zsh alone
	//	IFSSPACE    zsh alone
	//	INCOMPLETE  zsh alone
	//	INVALID     zsh alone
	//	WORD        zsh alone
	//
	// A name outside the twelve **and** outside this roster matches nothing,
	// silently, at status 0 — and that is not a gap: it is what every shell
	// in the panel does with `[[:nosuchclass:]]`, bash 3.2 included, and it
	// is measured rather than assumed. So the defect this was written for was
	// never generic. `[[:IDENT:]]` came back empty because the *name* was
	// missing, and only the names go here.
	//
	// The names are case-sensitive in the shell that has them: `[[:ident:]]`
	// and `[[:ASCII:]]` both match nothing.
	//
	// Two of the seven read shell state rather than a fixed set of
	// characters — `IFS` is the field separators as they stand and `WORD` is
	// the letters and digits together with `$WORDCHARS` — which is why they
	// are resolved where a Runner can be asked and not in a table.
	PatternClasses string

	// BracketEscapeIsAlsoAMember says a backslash that protects a member of
	// a bracket expression is a member of the set itself.
	//
	// False is five columns' answer: `[\)]` handed to the matcher with the
	// backslash still in it is the one-character set `)` in bash, bash 3.2,
	// bash as `sh`, dash and ksh93. True is zsh's, where the same set holds
	// the backslash as well.
	//
	// Measured 2026-09-07 through `${~p}`, which is the only construct that
	// hands this matcher a bracket expression holding a raw backslash — a
	// pattern *written* in the source has had its escapes spent by quote
	// removal long before, which is why the two routes can disagree at all
	// and why the source route is unanimous. Four values, four exact hits:
	//
	//	p='[\)]'   matches `)` and `\`, not `a`
	//	p='[\-z]'  matches `-`, `z` and `\`, not `y` — no range is formed
	//	p='[\a]'   matches `a` and `\`
	//	p='[\]]'   matches `]` and `\`
	//
	// The second row is what says the answer is *also a member* rather than
	// *not an escape*: the `-` behind the backslash stays a member instead of
	// becoming the range operator, so the protection happens there too. Both
	// halves are true at once, which is exactly what this field turns on.
	//
	// It reaches only the results of expansions, in expansionPattern, because
	// that is the only place a backslash arrives inside a bracket expression
	// without having been put there to say "the source quoted this". Reading
	// it in the matcher instead would have taken the source route with it and
	// broken the unanimous half (#1407).
	BracketEscapeIsAlsoAMember bool

	// LongestMatchTakesTheWrittenArm decides which match `${x##pat}` removes,
	// and which one `${x//pat/rep}` replaces, when `pat` holds an alternation
	// whose arms take different lengths: the arm that was written first, or
	// the longest of them.
	//
	// Measured 2026-09-11, `x=abc`, each probe in a `-c` of its own. The
	// spelling differs by column because the group does — one shell reads a
	// bare `(a|ab)` and the others want `@(a|ab)` with their extended
	// patterns switched on — and the answer does not:
	//
	//	${x##(a|ab)}    zsh 5.9.2   `bc`, the first arm
	//	${x##(ab|a)}    zsh 5.9.2   `c`
	//	${x##@(a|ab)}   bash 5.3    `c`, the longest arm
	//	${x##@(ab|a)}   bash 5.3    `c`
	//	${x##@(a|ab)}   bash 3.2    `c`
	//	${x##@(a|ab)}   ksh93u+     `c`
	//
	// Yes is one shell's and No is the rest of the panel's, so the two
	// spellings of `abc` disagree about which of `a` and `ab` came off.
	//
	// Yes is not "the shortest arm", and that is what makes it a search
	// order rather than a second length rule: `${x##(a*|ab)}` empties the
	// value in the shell that answers Yes, because the first arm is tried
	// first and then matches as much as it can. `${x##(a|ab)c}` empties it
	// too — the first arm is a preference and not a refusal, so the search
	// falls back to a later arm where the rest of the pattern needs it. An
	// empty arm is an arm: `${x##(|a)}` removes nothing there and removes
	// `a` under No.
	//
	// **Asked where the longest match is wanted and the end of that match is
	// free to move**, which is where the panel actually splits, and which is
	// two operators rather than one. The single `#` takes the shortest match
	// in every column, arms or no arms — `${x#(ab|a)}` is `bc` in all of them
	// — and the unflagged suffix trims take the longest: `${x%%(|bc)}` is `a`
	// in the shell that answers Yes, where a written-arm search would have
	// taken the empty arm and removed nothing. So an axis worded for trims in
	// general would have moved three rows the panel agrees about.
	//
	// The *substitution* is the second operator and was missing for a long
	// time, which is what the field's old name — `LongestPrefixTrimTakes…` —
	// recorded rather than caused. `${x//(a|ab)/X}` on `abc` is `Xbc` in the
	// shell that answers Yes and `Xc` in the rest, exactly as the trim
	// splits, and every unanchored spelling goes the same way: `${x/(|a)/X}`
	// is `Xabc` there and `Xbc` elsewhere, and `/#` follows because its end
	// is still free. `/%` does not, because pinning the end leaves the arms
	// no length to disagree about — measured, `${x/%(c|bc)/X}` and
	// `${x/%(bc|c)/X}` are both `aX` in every column (#2152).
	//
	// Under zsh's `(S)` flag the shortest match is wanted, which is the
	// minimum over every arm, so the arms cannot disagree there either.
	//
	// It reaches the `(M)` flag and the `(#b)` captures with the same
	// answer, because they are the same match seen from the other side:
	// `${(M)x##(a|ab)}` is `a` where the trim leaves `bc`, and
	// `${x##(#b)(a|ab)}` reports `a` in `$match[1]`.
	//
	// Asked only where the two readings land in different places, which is
	// why an ordinary pattern never reaches it: a pattern with no
	// alternation has one reading, and `(ab|a)` — the arms in decreasing
	// length — has two that agree. See armEnd, which is the one place both
	// operators ask it, and interp/trimarm.go for the search.
	LongestMatchTakesTheWrittenArm Answer

	// EmptyReplacementPattern is what `${v//\/X}` — a span replacement whose
	// pattern is empty — matches in the unanchored spellings.
	//
	// Measured 2026-09-12 from a script file, `v=abc` and `e=`:
	//
	//	                  bash 5.3.15  ksh93u+  zsh 5.9.2
	//	${v///X}          abc          abc      XaXbXc
	//	${v/$e/X}         abc          abc      Xabc
	//	${e///X}          (empty)      X        X
	//	${e//x/X}         (empty)      (empty)  (empty)
	//
	// Three answers and not two, which the empty *value* row is the whole of:
	// bash declines the pattern outright and ksh93 takes it where there is
	// nothing to scan. The last row is the control that says the `X` is a
	// match and not something an empty value produces on its own.
	//
	// It is a question about the pattern's *text* rather than about empty
	// matches in general, and the discriminating probe is a non-empty pattern
	// that matches only the empty string: with extglob on, `${v//@(|)/<>}` is
	// `<>a<>b<>c` in bash and `<>a<>b<>c<>` in ksh93, so neither shell is
	// refusing empty matches — see ReplacementEmptyMatchDeclined, which is
	// where those two columns then part.
	//
	// Asked only where the pattern is empty and the spelling unanchored. A
	// pattern with anything in it never reaches it, and the anchored forms
	// are their own row: `${v/#/X}` is `Xabc` in bash and zsh and `abc` in
	// ksh93, which is that shell declining an *anchor* it takes nowhere else
	// (#1857).
	EmptyReplacementPattern EmptyReplacementPatternPolicy

	// ReplacementEmptyMatchDeclined is which empty match a global replacement
	// refuses to take, once the pattern is one that can match empty at all.
	//
	// Every column agrees that a match reaching the end of the value ends the
	// scan — `${v//*/X}` is one `X` — and they part over an empty match that
	// does not. Measured 2026-09-12, `v=abc`, the replacement written `<>` so
	// each match shows, the pattern "empty or one letter" spelled `@(b|)`
	// under extglob and `(b|)` under extendedglob:
	//
	//	          bash 5.3.15  zsh 5.9.2  ksh93u+
	//	@(b|)     <>a<><>c     <>a<><>c   <>a<>c<>
	//	@(x|)     <>a<>b<>c    <>a<>b<>c  <>a<>b<>c<>
	//	@(c|)     <>a<>b<>     <>a<>b<>   <>a<>b<>
	//	@(a|)     <><>b<>c     <><>b<>c   <>b<>c<>
	//
	// Row one is the discriminator: ksh93 has no `<>` between `b` and `c`,
	// where the other two do, and has one after `c`, where they do not.
	//
	// Row three is the control both readings answer the same way and both
	// must keep, because it is the rule nobody disputes reached from a third
	// direction — `c` matches at the last unit, so the scan ends there under
	// either policy.
	//
	// Asked at the two positions the readings land differently on, and
	// nowhere else: an empty match where the match before it ended, and the
	// end of the value stepped onto after an empty match. A pattern that
	// cannot match empty reaches neither.
	ReplacementEmptyMatchDeclined EmptyMatchDeclinedPolicy

	// ParameterIsSetSeesPositionals lets `-v 1` ask about a positional
	// parameter, and `-v 0` about the shell's name.
	//
	// Measured 2026-09-07 on the three shells that have the operator, under
	// `-c`: `set -- p q; [[ -v 1 ]]` is set in bash 5.3.15 and zsh 5.9.2 and
	// **unset** in ksh93u+, and `[[ -v 0 ]]` splits the same way. It is the
	// operator declining to treat a digit as a name rather than a lookup
	// coming back empty — `${1+s}` is `s` in all three — which is why the
	// answer is here rather than in the parameter table.
	//
	// A positional past `$#` is unset everywhere and needs no axis: with two
	// parameters set, `[[ -v 3 ]]` is unset in all three.
	ParameterIsSetSeesPositionals bool

	// ParameterIsSetSeesSpecials lets `-v ?` and its fellows — `#`, `$`,
	// `!`, `*` and `-` — ask about a parameter spelled as one punctuation
	// character.
	//
	// zsh 5.9.2 alone answers set for every one of them; bash 5.3.15 and
	// ksh93u+ answer unset for every one. Again the parameters are there in
	// all three and it is the operator that does not look: `[[ -n ${?+s} ]]`
	// and `[[ -n ${#+s} ]]` are set in bash and ksh93 as well.
	//
	// `@` is not one of them and is unset in all three — with positional
	// parameters set, and in the shell that answers for every other
	// character — so it is excluded outright rather than by this axis. See
	// isSetNameKind.
	ParameterIsSetSeesSpecials bool

	// ScalarSubscriptIsACharacter reads `${s[2]}` on a plain string as its
	// second character, rather than as an element of the one-element array a
	// scalar reads as.
	//
	// Measured on `s=hello`: zsh 5.9.2 gives `h` for `${s[1]}` and `e` for
	// `${s[2]}`, where bash 5.3.15, bash 3.2.57, bash as `sh` and ksh93 all
	// give `hello` for `${s[0]}` and nothing for either of the others. Both
	// readings answer, neither reports, and an empty string is a plausible
	// element — so a script cannot tell which shell it is on except by the
	// value it gets, which is the definition of a conflict rather than an
	// addition.
	//
	// A range and a character go together: `${s[2,4]}` is the substring
	// `ell` in the shell that reads characters, and the arithmetic comma's
	// element 4 — nothing — in the shells that do not. But they are two
	// axes, because `${a[1,3]}` on an *array* is a range without a character
	// anywhere in it.
	//
	// Asked only where the two readings differ: a one-character string at
	// the dialect's first subscript is itself under either reading.
	ScalarSubscriptIsACharacter Answer

	// MultibyteEncodingIsHonored decodes the locale's character encoding, so
	// that `${#s}`, `${s:off:len}` and a subscript on a scalar count
	// characters rather than bytes.
	//
	// Measured 2026-09-05 with `s=héllo; echo ${#s}` under
	// `LC_ALL=en_US.UTF-8`: bash 5.3.15, bash 3.2.57, bash as `sh`, ksh93u+
	// and zsh 5.9.2 all answer 5, and dash answers 6. Under `LC_ALL=C` every
	// one of them answers 6, dash included — so this is not "four shells
	// count characters", it is "four shells honor the encoding the locale
	// names and one has no multibyte decoder at all". `s=日本語; echo ${#s}`
	// separates them further: 3 against 9.
	//
	// Which encoding is in force is **not** a second axis. It is state read
	// off the runner's own variables, exactly as PATH and IFS are, and it
	// moves inside a running shell: `LC_ALL=C; s=héllo; echo ${#s}` gives 6
	// in every panel member with nothing exported. See interp/multibyte.go
	// for the precedence and the codesets, and driver/startup.go for the
	// same reasoning applied to POSIX mode (#691, #733).
	//
	// Silent either way, which is why it is an axis and not a bug in one
	// place: both answers are plausible numbers and neither shell reports
	// anything.
	//
	// Asked only where the two readings differ — a value whose bytes are all
	// ASCII is the same length and has the same positions under both — so a
	// shell that never sees a non-ASCII byte never needs an answer.
	MultibyteEncodingIsHonored Answer

	// UnsetLocaleIsUnicodeAware is what a locale *nothing names* is: the
	// encoding the environment would have chosen, or the C locale.
	//
	// A second question to MultibyteEncodingIsHonored above rather than a
	// restatement of it. That one is whether this shell decodes the locale's
	// encoding at all; this one is which locale is in force when `LC_ALL`,
	// `LC_CTYPE` and `LANG` are all unset — which is what `env -i`, a cron
	// job and a container have, and where a person's terminal never is.
	//
	// Measured 2026-09-11 under `env -i`, with no locale variable set
	// anywhere, on three operators that read the same state:
	//
	//	                          bash 5.3.15  bash 3.2.57  ksh93u+  zsh 5.9.2  dash
	//	s=héllo; echo ${#s}       5            6            6        6          6
	//	s=café; upper-case it     CAFÉ         n/a          CAFé     CAFé       n/a
	//	echo -e 'a\u00e9Z'       61 c3 a9 5a  n/a          n/a      refused    n/a
	//
	// So one shell reads an unset locale as UTF-8-capable and the rest read
	// it as C, and it is the *same* reading in each of them across all three
	// operators — which is what makes this one axis rather than one per
	// operator. The case-mapping row is spelled per shell (`${s^^}`,
	// `typeset -u`, `${(U)s}`), and bash 3.2.57 has none of the three
	// spellings, which is why its cell is empty rather than measured.
	//
	// Silent either way: a length is a plausible number and a case-mapped
	// word is a plausible word, so a script carried from a terminal into a
	// container changes answer with nothing reported.
	//
	// Asked only where the two readings differ — a value whose bytes are all
	// ASCII, an ASCII code point, and case mapping below 0x80 are the same
	// under both — and only after the operator's own axis has said the
	// question can matter. See interp/multibyte.go for the order.
	UnsetLocaleIsUnicodeAware Answer

	// DeclarationTakesAnAppendOperand reads a declaration builtin's
	// `name+=value` operand as the append operator rather than as a name
	// with a `+` on the end of it: `declare a=1; declare a+=2` leaves `12`.
	//
	// Measured 2026-09-09 with `declare a=1; declare a+=2; echo "$a"`, and
	// the same three lines under `export`, `readonly` and `local`:
	//
	//	bash 5.3.15         12
	//	bash 5.3.15 as sh   12
	//	bash 3.2.57         12
	//	ksh93               typeset: a+: invalid variable name
	//	zsh 5.9.2           not valid in this context: a+
	//	dash                export: a+: bad variable name
	//
	// So it is one shell's operand rather than a core one. The three that
	// refuse it all name **`a+`** — the text in front of the `=` — and not
	// the whole operand, which is what the two of them that otherwise quote
	// a bad operand back do for `typeset 1x=v`. That is the tell that they
	// read the `+=` as an operator too and then refuse the name it left.
	//
	// The value joins through the name's attributes, which is the same join
	// the bare statement performs and not a second rule: `declare -i a=1;
	// declare a+=2` is 3, `declare -a arr=(p q); declare arr+=x` is `px q`,
	// and a declared table joins its `0` key.
	//
	// Asked only where an operand's name ends in `+` and a value follows it.
	// A `+` with no `=` is not this spelling — `declare a+` is refused as a
	// name in every column, the one that takes the operator included — so
	// nothing well formed ever reaches the question.
	DeclarationTakesAnAppendOperand Answer

	// NegativeSubscriptCountsOverAPromotedScalar resolves a negative
	// subscript on the left of `=` against the array a held *scalar* is
	// about to become, rather than against the elements the name already
	// has — of which a scalar has none.
	//
	// An element write over a name holding a string keeps the string as the
	// first element, which is core and unanimous: `a=abc; a[1]=x` leaves
	// `abc` beside the `x` in every shell in the panel that has arrays. When
	// that happens relative to reading the subscript is not unanimous, and a
	// subscript counting back from the end is the only spelling that can
	// tell. Measured 2026-09-09 with `a=abc; a[-1]=x; typeset -p a`:
	//
	//	bash 5.3.15         declare -a a=([0]="x")
	//	bash 5.3.15 as sh   declare -a a=([0]="x")
	//	bash 3.2.57         a[-1]: bad array subscript
	//	ksh93               a: subscript out of range
	//
	// bash promotes and then counts back over the one element it made;
	// ksh93 counts back first and refuses. bash 3.2 has no negative
	// subscripts at all — `a=(p q); a[-1]=x` is the same complaint there —
	// which is the absence of the spelling rather than a third answer.
	//
	// Asked only where there is a scalar to promote *and* the subscript is
	// negative. A non-negative one lands at the number it names whether the
	// promotion happened before or after it, and an unset name has nothing
	// to promote, so `unset a; a[-1]=x` is refused in both columns and needs
	// no answer from either.
	//
	// The dialect where a subscript on a string names a character never
	// arrives here at all: there is no array to promote into on that side.
	NegativeSubscriptCountsOverAPromotedScalar Answer

	// NegativeSubscriptPastTheStartInserts places a new element in front of
	// every other when a negative subscript counts back past the first one:
	// `a=(p q); a[-3]=x` leaves three elements with `x` at the head, however
	// far past the start the subscript reached. True in zsh alone; bash and
	// ksh93 refuse the subscript and end the script.
	//
	// Asked only for a *negative* subscript that lands before the first
	// element, which is the only spelling that can. A non-negative one below
	// the base — `a[0]` where the first element is 1 — is refused by every
	// shell measured, zsh included, so it needs no answer from anyone.
	NegativeSubscriptPastTheStartInserts Answer

	// ArrayLiteralSubscriptIsAKey reads a subscript written inside an array
	// literal as the text between the brackets rather than as an arithmetic
	// expression — and, because the two go together, makes such a literal
	// declare a keyed array rather than an indexed one.
	//
	// One concept with two consequences, like whether an assignment prefix
	// survives a special builtin. True in ksh93, where `a=([1+1]=c)` stores
	// under the three characters and `${a[2]}` finds nothing; false in bash
	// and zsh, where the subscript is evaluated and the value lands at 2.
	//
	// Asked only where the two readings differ. A plain decimal numeral
	// evaluates to itself, so `a=([2]=c)` fills the same slot either way and
	// never reaches the question — which is what keeps the ordinary way to
	// build a sparse array available in a core that has chosen no shell.
	//
	// dash has no array literal at all, so the axis is absent there rather
	// than false.
	ArrayLiteralSubscriptIsAKey Answer

	// DollarZeroNamesTheInnermostCall makes `$0` the innermost thing the
	// shell has been called into rather than the shell's own name: the
	// function being run, or the file being sourced.
	//
	// One concept with two consequences, and one field because no shell
	// splits them. zsh has both under a single option, and turning that
	// option off takes both away together — `$0` inside a function goes back
	// to the script's name in the same breath as `$0` inside a sourced file
	// does. Every other member of the panel has neither: measured across
	// dash, bash 5.3, bash-as-sh, bash 3.2 and ksh93, `$0` is the script's
	// name inside a function, inside a file it sourced, inside a file that
	// file sourced, and inside a function defined by one of them.
	//
	// Innermost is the whole of the rule and is measured rather than
	// assumed: a function that sources a file reports the *file* while that
	// file runs and the function's name again afterwards, and a function
	// defined in a sourced file reports its own name and not the file it
	// came from. So this is a question about the top of the call stack and
	// not about whether a function is anywhere on it.
	//
	// The file is named as the operand was written — `. ./inc.sh` reports
	// `./inc.sh` and a bare name found on PATH reports the bare name —
	// which is the same spelling the call stack and the diagnostics use.
	//
	// A shell's startup files are outside this. They are read by the shell
	// rather than sourced by a script, and `$0` inside one is the shell's
	// own name in the shell that has this: measured, a `~/.zshrc` printing
	// `$0` under `zsh -i` prints the path of the zsh binary.
	DollarZeroNamesTheInnermostCall Answer

	// BuiltinSyntaxErrorFatal ends a non-interactive shell when text handed
	// to a special builtin does not parse — `eval "if"`, or a sourced file
	// with an unterminated `if` in it.
	//
	// True only in dash, which is the POSIX rule that a special builtin's
	// failure is fatal; bash, ksh93 and zsh report it and carry on. One axis
	// covers both callers because the answers are the same for both in every
	// shell measured, where the *status* is not — that is two fields on
	// Diagnostics.
	BuiltinSyntaxErrorFatal Answer

	// EvalRunsWhatItParsed runs the commands `eval` has already read when a
	// later line of its text will not parse, instead of reading the text
	// through and running none of it.
	//
	// Measured 2026-09-11, counting a side effect rather than reading a
	// transcript, because the transcript is what a shell's buffering can
	// reorder:
	//
	//	$ <shell> -c 'eval "printf x >> f
	//	if; then"'
	//
	//	bash 5.3, bash 3.2, dash	f holds x
	//	zsh, ksh93              	f does not exist
	//
	// Not the same question as the wording or the status of the complaint,
	// both of which are Diagnostics' and both of which are written either
	// way. What this decides is whether the *work* before the offending line
	// happened.
	//
	// A separate field from the sourced-file one because zsh splits them: it
	// reads a file a command at a time and reads `eval`'s text through first.
	EvalRunsWhatItParsed Answer

	// SourcedFileRunsWhatItParsed is the same question for `.`, and the
	// answer is not always the same one.
	//
	// Measured the same day and the same way, with a file holding `printf y
	// >> f` and then `if; then`:
	//
	//	bash 5.3, bash 3.2, dash, zsh	f holds y
	//	ksh93                        	f does not exist
	//
	// It is worth more here than for `eval`: a file that sets six names and
	// has a typo on the last line leaves six names set in five of the six
	// columns, and a shell reading it through first leaves none.
	SourcedFileRunsWhatItParsed Answer

	// FatalErrorEndsBorrowedTextOnly makes an error that would end a script
	// end only the text a special builtin is running — a file `.` read, or
	// `eval`'s argument — handing the builtin a status and letting the script
	// around it carry on.
	//
	// Measured with an error every shell in the panel words identically, so
	// that the row is about the abandonment and not about the operator — a
	// file whose third line is `echo X${NOPE}` under `set -u`, sourced by a
	// file that prints afterwards:
	//
	//	dash, bash, bash-as-sh, bash32   the shell ends; nothing after the `.`
	//	                                 runs, in the sourcing file or any
	//	                                 file above it
	//	ksh93                            `.` reports 1 and the sourcing file
	//	                                 carries on
	//	zsh                              `.` reports 126 and the sourcing file
	//	                                 carries on
	//
	// Four measured facts make this one axis rather than several:
	//
	//	*Every* error that would end a script behaves this way in the two
	//	shells that catch anything — a readonly assignment they call fatal, a
	//	division by zero, a bad substitution, an unset parameter. So the axis
	//	is about what a fatal error costs and not about expansion.
	//
	//	Only one file is given up. A file sourced from a file sourced from a
	//	script loses the innermost file alone, and the middle one prints the
	//	line after its own `.`.
	//
	//	It is the *running* `.` and not the file the text came from: a
	//	function defined in a sourced file and called later from the script
	//	ends the shell in every member of the panel, ksh93 and zsh included.
	//	A `.` inside a function is the boundary, and the function body
	//	resumes after it.
	//
	//	`exit`, and errexit firing, are not errors and are never caught —
	//	unanimous. That is what separates this from the neighboring rule
	//	that `exit` in a startup file ends the shell and the files after it
	//	are not read.
	//
	// One axis covers `eval` and `.` because the answers are the same for
	// both in every shell measured — the same four fatal, the same two
	// catching, the same `exit` uncaught — which is the arrangement
	// BuiltinSyntaxErrorFatal already has for the same pair. The *status* is
	// not the same for both, and that is Diagnostics.SourcedFatalStatus,
	// which the file route passes and `eval` does not: measured, an error
	// caught at an `eval` reports 1 in ksh93 and in zsh, where the same
	// failure caught at a `.` reports 1 in ksh93 and 126 in zsh.
	FatalErrorEndsBorrowedTextOnly Answer

	// ParamErrorIsAnExitRequest makes `${x?word}` and `${x:?word}` a request
	// to stop rather than an error, so no boundary catches it.
	//
	// Asked only where the answers differ, which is at a boundary that gives
	// up one file: measured, the same `${NOPE?msg}` inside a file `.` read
	// reports and lets the sourcing file carry on in ksh93 and ends the whole
	// shell in zsh, where an unset parameter under `set -u` two lines away is
	// caught by both. The startup-file boundary splits the same way — at the
	// top of a `$BASH_ENV` the operator stops that file and the script still
	// runs, and at the top of a `~/.zshenv` it ends the shell before the
	// script. At the top level of a script both operators end the shell in
	// every member of the panel, so nothing there has a question to ask.
	//
	// zsh's own manual is the reason it reads as a request rather than as an
	// inconsistency: the `?` form is documented to print the word and *exit
	// the shell*, which is the same family as the `exit` builtin and not the
	// family of a diagnostic.
	ParamErrorIsAnExitRequest Answer

	// DotWithNoOperandIsAnError decides whether `.` with no filename is a
	// failure at all. False in dash, which does nothing and reports success;
	// true in bash, ksh93 and zsh.
	//
	// Separate from the status and from the fatality because the panel splits
	// four ways on `.` alone — dash 0, bash 2 surviving, ksh93 2 fatal, zsh 1
	// surviving — and one field with four answers would have to invent a type
	// to hold what is really three independent questions.
	DotWithNoOperandIsAnError Answer

	// DotDirectoryOperandIsAnError decides whether `.` naming a **directory**
	// is a failure at all, which the panel is split down the middle on.
	// Measured 2026-09-08, `. ./` from a script file in a scratch directory:
	//
	//	zsh 5.9.2   silent, status 0
	//	dash        silent, status 0
	//	bash 5.3.15 `.: ./: is a directory`, status 1, and the script carries on
	//	bash as sh  the same
	//	bash 3.2.57 the same
	//	ksh93u+     `.: ./: cannot open [Is a directory]`, and the script ends
	//
	// Two shells open the directory, read no commands out of it and call that
	// a script that did nothing; four call it an error. So it is an axis and
	// not a wording fix — a fix that answered only the sentence would leave the
	// status wrong for two columns and invent a diagnostic for them.
	//
	// Separate from DotMissingFileFatal, which decides what an error here
	// *costs* and already splits the four that report one the right way: ksh93
	// ends the script and bash reports and carries on. Separate from
	// DotWithNoOperandIsAnError for the same reason that one is separate from
	// the status — three independent questions about one builtin.
	//
	// A path that does not exist is not this axis. Every column reports that
	// one, and we already match each of them; the tell that this was a
	// different question was our answering `no such file or directory` at 127
	// for a path that does exist (#1577).
	DotDirectoryOperandIsAnError Answer

	// DotMissingFileFatal ends the script when `.` cannot read its file.
	// True in dash and ksh93, false in bash and zsh — the same split as
	// ShiftPastEndFatal, and for the same POSIX reason.
	DotMissingFileFatal Answer

	// DotPassesArguments gives a sourced file its own positional parameters
	// from the words after the filename, restoring the caller's afterwards.
	//
	// False in dash, which ignores them, so `. f.sh ARG` leaves `$1` as the
	// caller's; true in bash, ksh93 and zsh. With no words after the filename
	// every shell leaves the parameters alone, so the axis only speaks when
	// there are some.
	DotPassesArguments Answer

	// ExecFailureRunsExitTrap runs a `trap … EXIT` handler when `exec` could
	// not run the command it was given. True in dash and bash, false in ksh93
	// and zsh.
	//
	// A *successful* exec runs no handler anywhere, and that is not an axis:
	// the trap died with the process the exec replaced. Only the failure has
	// a shell left to decide anything, and the panel splits on it.
	ExecFailureRunsExitTrap Answer

	// TimesRejectsArguments makes `times` refuse an argument rather than
	// ignore it. True in zsh, false in dash and bash.
	//
	// ksh93 answers neither: `times` is a reserved word there, so `times foo`
	// is a *syntax* error and no builtin ever runs. That is a grammar question
	// rather than this one, and it is recorded in the corpus rather than
	// modeled here.
	TimesRejectsArguments Answer

	// EmptyPathIsTheCurrentDirectory searches the current directory when PATH
	// is set and empty.
	//
	// True in dash, bash and zsh; false in ksh93. `PATH=` reads like "nowhere"
	// and is not: an empty PATH is one *empty element*, and an empty element
	// means the current directory, so three of the four will still run a
	// command sitting next to the script. Measured with the command in the
	// current directory, which is the only arrangement that tells the two
	// answers apart — with it anywhere else all four report not-found and the
	// axis is invisible.
	//
	// `PATH=:` is not this question. Two empty elements is unanimous: every
	// shell searches the current directory for it.
	EmptyPathIsTheCurrentDirectory Answer

	// HashReportsAMissingName has `hash name` complain and answer 1 when
	// the name resolves to nothing. bash, dash and zsh do; ksh93 — whose
	// hash is an alias for `alias -t` — says nothing and reports success.
	HashReportsAMissingName Answer

	// HashSearchesPathAlone counts only what PATH holds: zsh answers
	// `hash shift` with "no such command" where the other three accept a
	// builtin or a function as hashable. Measured with `shift`, which no
	// PATH carries — `cd` was the contaminated probe, macOS ships
	// /usr/bin/cd. Recorded as `hash/a-builtin-counts-except-in-zsh`.
	HashSearchesPathAlone Answer

	// UnderscoreTracksTheLastArgument moves `$_` to the previous simple
	// command's last expanded argument — the command word itself when it
	// had none, and empty after a bare assignment. bash and zsh; dash and
	// ksh93 keep no such parameter at all.
	//
	// What the two that do not keep it hold instead was recorded here as
	// the shell's own path, forever, and that was measured false: they
	// hold whatever the environment brought and nothing when it brought
	// nothing, because `_` is an ordinary name there. The claim survived
	// because the harness writes a shell's path as `<shell>` and the cells
	// were empty either way — a real path would have shown. Re-measured
	// with `_` scrubbed from the environment and again with `_=X` in it,
	// on both the `-c` and the script route.
	UnderscoreTracksTheLastArgument Answer

	// UnderscoreStartsAtTheInvocation writes argv[0] into `$_` before the
	// first command runs, so a script reading it at the top finds how the
	// shell was started. bash alone, in both builds and under either
	// argv[0]; dash, ksh93 and zsh leave it as it was.
	//
	// Not the same question as UnderscoreTracksTheLastArgument, which is
	// why it is its own axis rather than a consequence of that one: zsh
	// answers yes to tracking and still starts empty, so a startup write
	// gated on tracking would give zsh a value no zsh has.
	//
	// The value is the *invocation* rather than the executable — the same
	// binary reached through a symlink named `sh` writes `sh` — and rather
	// than `$0`, which a `-c` invocation takes from its first operand.
	UnderscoreStartsAtTheInvocation Answer

	// UnderscoreInheritsFromTheEnvironment lets an `_` the shell was handed
	// in its environment show through. Everywhere but zsh, which discards
	// it and starts empty however it was invoked.
	//
	// It is the other half of the startup value and it decides what the
	// half above means: bash writes argv[0] only when the environment said
	// nothing, so an exported `_` wins over the invocation in every shell
	// that reads one. Only reachable with an environment, which is why the
	// case that pins it carries one — no snippet can put a name in the
	// environment of the shell already running it.
	//
	// `_` is exported by some shells as the command they are about to run,
	// so this is not a hypothetical: it is what a shell started by another
	// shell actually finds.
	UnderscoreInheritsFromTheEnvironment Answer

	// FdVariableOutlivesTheCommand keeps a `{name}>f` descriptor open past
	// the simple command that carried it — two of the three that have the
	// grammar; ksh93 takes it back with the command's other redirections,
	// so the number the variable holds is already dead.
	FdVariableOutlivesTheCommand Answer

	// FirstAllocatedDescriptor is the number the shell counts up from when it
	// picks a descriptor for itself — `exec {fd}< file`, and the two zsh
	// builtins that hand a number back the same way.
	//
	// Measured 2026-09-12, `<shell> -c 'exec {fd}< /etc/hosts; echo $fd'`:
	// bash 5.3.15 and ksh93 (AJM 93u+) say 10, zsh 5.9.2 says 11. dash and
	// bash 3.2 have no such grammar. The same number is what `zsocket`
	// reports in `$REPLY` and what `sysopen -u name` writes, so a corpus row
	// about either builtin either avoids printing the number or is wrong in
	// one column (#1752).
	//
	// **The one axis with no unanswered state**, and that is measured rather
	// than an omission: there is no shape in which a shell declines to pick a
	// number. A `{name}<file` that reached the allocation is going to be
	// given one, and so is an embedder calling Runner.OpenDescriptor, so
	// there is nothing for a refusal to protect and a zero value that refused
	// would refuse a construct every shell performs. The zero value is
	// therefore an answer — the one four of the five shells with the
	// construct give — and every preset states it anyway.
	FirstAllocatedDescriptor DescriptorAllocationBase

	// UnterminatedHeredocGainsATrailingNewline adds the newline a
	// here-document body never got, where the delimiter never arrived and
	// the input ended mid-line.
	//
	// Measured 2026-09-12, `printf 'cat <<X\nbody' > u.sh` read back with
	// `od -c`, and again through `-c` with the same two lines:
	//
	//	bash 5.3, bash 3.2, bash-as-sh   body\n   5 bytes
	//	dash, ksh93, zsh                 body     4 bytes
	//
	// An earlier reading of #1020 put bash 3.2 with the four; it is not, and
	// the two bash builds agree.
	//
	// It is reachable no other way, which is why it is worth a field at all:
	// a here-document closed by its delimiter always has a body ending in a
	// newline, so this is the only shape in which the question exists. The
	// corpus cannot see it either — `$( )` strips trailing newlines and the
	// harness trims them — so it is checked by a Go test on the runner's
	// bytes.
	//
	// Asked only where the two answers differ, which is what
	// syntax.Redirect.HeredocAtEOF marks: an ordinary here-document never
	// reaches the question.
	UnterminatedHeredocGainsATrailingNewline Answer

	// ReadFailureInAFileSubstitutionFailsIt is `$(<file)` where the *read*
	// fails after the open worked — a directory is the shape that reaches it.
	//
	// Measured 2026-09-12, `mkdir dir; v=$(<dir); echo "st=$? v=[$v]"`:
	//
	//	zsh 5.9.2   st=1, and `error when reading dir: is a directory`
	//	bash 3.2    st=1, silent
	//	bash 5.3    st=0, silent
	//	ksh93       st=0, silent
	//	dash        no such form
	//
	// The status and the sentence are separate questions and the panel is
	// what separates them: bash 3.2 fails the substitution and says nothing,
	// so a dialect could hold either answer with either wording. The sentence
	// is Diagnostics.FileSubstitutionReadError.
	//
	// An open that fails is a different event and is already answered by
	// redirectFailureStatus. This one is the read after a successful open,
	// which is why it cannot ride on that: `$(<nosuch)` and `$(<dir)` are
	// status 1 and status 0 in the same shell (#1778).
	ReadFailureInAFileSubstitutionFailsIt Answer

	// ExecOpenedFdReachesACommand hands a descriptor that `exec`'s own
	// redirection list opened to whatever the shell runs next — the flock
	// and shared-log idioms, and every script that gives a child a logging
	// descriptor. Four of the five say yes; ksh93 alone closes anything
	// above 2 that `exec` opened when it invokes another program, which its
	// manual states as the rule rather than leaving it to be discovered.
	//
	// POSIX decides nothing here: the Shell Command Language says whether
	// standard input, output and error are open for a utility and is silent
	// about the rest, so both answers conform and there is no majority to
	// defer to on the standard's authority.
	//
	// It is narrower than "that shell hands nothing over", and the boundary
	// is measured. A descriptor the *caller* opened crosses in every shell,
	// this one included, and closing it closes it for the child everywhere.
	// A command's own redirection crosses everywhere too — `sh -c '… >&3'
	// 3>f` writes, and so does `exec 3>f; sh -c '… >&3' 3>&3`, where
	// restating the number on the command brings it back. What is withheld
	// is what `exec` opened: the numbered form, a `{v}>f` the shell numbered
	// itself, and a `9<&3` duplicated from an inherited descriptor, while
	// the inherited 3 it was copied from still crosses.
	//
	// Read where the outbound table is built, so an external command and a
	// process replacement get the same answer — measured the same in both,
	// which is one divergence rather than two.
	ExecOpenedFdReachesACommand Answer

	// FdVariableBadCloseIsAnError refuses `exec {name}>&-` when the name
	// holds no descriptor number. ksh93 says nothing and reports success.
	FdVariableBadCloseIsAnError Answer

	// FdNumberBoundedByOpenFileLimit refuses a redirection whose descriptor
	// number is at or above the process's soft limit on open files. bash and
	// ksh93 do; dash and zsh accept the number and let whatever comes next
	// fail on it, or not at all.
	//
	// There is no *language* bound anywhere in the panel — no shell has a
	// ceiling of its own, and the one that bites is the kernel's `ulimit -n`.
	// bash reports the errno it gets: with the limit at 20, `exec 20>f` is
	// `20: Bad file descriptor` and status 1, `exec 19>f` is silent, and
	// lowering the limit lowers the ceiling exactly. ksh93 refuses the same
	// numbers in its own words. dash and zsh answer 0 for `exec 8>f` under a
	// limit of 6 and leave the descriptor unusable, which is the shape of not
	// asking rather than of a different answer.
	//
	// It is asked at the disagreement rather than on every redirection: a
	// number below the limit is nobody's question, and a Runner with no
	// GetRlimit has no limit to be asked about. Reached most often through
	// MultiDigitFdNumber, which is what lets a script write a number that
	// large at all — under the three shells that read one digit, the only way
	// to a descriptor above nine is to let the shell pick it.
	FdNumberBoundedByOpenFileLimit Answer

	// JobControlAbsenceIsReportedFirst refuses `bg` and `fg` before
	// reading the operand when there is no job control — bash and zsh; dash
	// and ksh93 read their operands and options first and complain about
	// those.
	JobControlAbsenceIsReportedFirst Answer

	// StoppedJobsHoldTheExit keeps an interactive shell alive when leaving
	// would abandon a job that is stopped: the shell says so and stays, and
	// the attempt has to be made a second time.
	//
	// bash and zsh; dash and ksh93 leave at once and the job is left stopped
	// with nothing able to name it. Measured through a pseudo-terminal —
	// `sleep 40`, ^Z, `exit` — for `exit` and for the end of input alike,
	// which behave the same in both shells that hold.
	//
	// What counts as having been told is measured too, and it is not simply
	// "warned once": a `jobs` listing counts, so `exit` straight after one
	// leaves; any other command does not, so `echo hi` between the ^Z and the
	// `exit` still warns; and a job stopping afterwards starts it over.
	StoppedJobsHoldTheExit Answer

	// HeldExitListsTheJobs follows that warning with the job table — the
	// same rows `jobs` writes. bash does and zsh does not: measured through a
	// pseudo-terminal, `shopt -s checkjobs` then `exit` writes
	// `There are running jobs.` and `[1]+  Running   sleep 40 &` under it,
	// where zsh writes `you have running jobs.` and nothing else, whatever
	// its own options are set to.
	//
	// Reached only in a shell that holds an exit at all, so dash and ksh93
	// never answer it. Asked *with* Runner.ChecksRunningJobsAtExit rather
	// than instead of it, because in bash the listing is the other half of
	// what one `checkjobs` buys: with the option off the sentence still
	// appears for a stopped job and the table under it does not.
	HeldExitListsTheJobs Answer

	// CdpathAnnouncesTheDirectory prints where CDPATH sent a `cd`, when
	// the winning entry was not a plain dot — three of the four; zsh moves
	// in silence.
	CdpathAnnouncesTheDirectory Answer

	// AutoCdAnnouncesTheSubstitution writes the `cd` that a bare directory
	// name was read as, before moving — bash; zsh moves in silence.
	//
	// Only the two shells that have the option at all reach this, which is
	// why it is unanswered in the base rather than given the quieter default:
	// dash and ksh93 have no `autocd`, so nothing can turn the capability on
	// in them and nothing can ask. See Runner.autoCdInstead.
	//
	// Measured 2026-09-08 through a pseudo-terminal, since the name is
	// interactive-only in both: bash 5.3.15 with `shopt -s autocd` writes
	// `cd -- subdir` and then moves, and the line survives a `2>/dev/null` on
	// the word itself while `exec 2>file` captures it. zsh 5.9.2 with `setopt
	// autocd` writes nothing at all and moves.
	//
	// An axis rather than a bash-shaped default with a zsh exception,
	// because the two answers are a conflict and not a subset: there is no
	// ordering of the shells in which one derives the other's silence.
	AutoCdAnnouncesTheSubstitution Answer

	// FcEmptyHistoryIsAnError has `fc` report the event it cannot find —
	// zsh; bash and dash answer a script with silence at 0.
	FcEmptyHistoryIsAnError Answer

	// TestIntegerRefusalIsSilent has `[ a -eq 1 ]` fail with no sentence at
	// status 1 — ksh93; the other three complain at 2.
	TestIntegerRefusalIsSilent Answer

	// MissingFileIsOlder has `-nt` and `-ot` count a path that does not
	// exist as older than any file that does, so `f -nt missing` and
	// `missing -ot f` hold whenever f exists. bash and ksh93; dash and zsh
	// answer false unless both files exist. Asked only there: with both
	// files present the comparison is unanimous, and the mirrored cases —
	// a missing file being *newer* — are false in every shell measured.
	// One axis for `test`, `[` and `[[ ]]` alike, because every shell
	// answers its two constructs the same way.
	MissingFileIsOlder Answer

	// TerminalTestRequiresANumber has `-t` refuse an operand that is not a
	// number, at status 2 with the dialect's integer wording — bash and
	// dash; ksh93 and zsh answer a silent false at 1, and so does the bash
	// 3.2 that macOS ships. Asked only for such an operand: a numeric
	// descriptor is answered false the same way everywhere a terminal is
	// absent.
	TerminalTestRequiresANumber Answer

	// BareTerminalTestIsDescriptorOne reads a lone `-t` — `[ -t ]` and
	// `test -t`, where the one-argument rule would make it a non-empty
	// string and so unconditionally true — as `-t 1` instead. ksh93 and zsh;
	// dash, bash 5.3, bash-as-sh and bash 3.2 keep the string rule.
	//
	// Measured 2026-09-10 with `[ -t ] >/dev/null`, which pins the answer to
	// a descriptor that is certainly not a terminal rather than to whatever
	// the run was handed: 0 in dash, bash, bash-as-sh and bash 3.2, and 1 in
	// ksh93 and zsh. The same line on a pseudo-terminal with no redirection
	// is 0 in all six, which is what says the two shells are answering about
	// descriptor 1 and not refusing the word.
	//
	// `-t` alone among the unary operators: `[ -f ]`, `[ -n ]`, `[ -z ]` and
	// `[ -e ]` are true in all six columns, so this is not a general "an
	// operator with no operand is an operator" rule and is not written as
	// one. The negated two-word form follows it — `[ ! -t ] >/dev/null` is 1
	// in the four and 0 in the two — because that is the same one-argument
	// rule with a `!` in front.
	//
	// Nothing is asked for `[[ -t ]]`: every shell in the panel that has the
	// construct refuses it as a syntax error.
	BareTerminalTestIsDescriptorOne Answer

	// ReadRequiresAVariableName refuses a bare `read`: dash's "arg count"
	// at 2, where the other three read into REPLY.
	ReadRequiresAVariableName Answer

	// ReadRefusesABadNameBeforeReading judges `read`'s first operand as a
	// name before it goes to the stream, rather than after.
	//
	// Observable, and only through the input: a refusal that comes first
	// leaves the line for the next reader, and one that comes after has
	// eaten it. Measured 2026-09-07 with `printf 'AAA\nBBB\n' | { read 1bad;
	// cat; }` — bash 5.3 and ksh93 print both lines, dash and zsh print only
	// BBB. bash 3.2 is on dash's side, which makes this a change within bash
	// rather than a difference between shells, so the `bash32` column of a
	// corpus case here disagrees with `bash` on purpose.
	//
	// It is the *first* operand and not the whole list. Every shell in the
	// panel assigns the names in front of a bad one and then refuses:
	// `printf 'X Y Z\n' | { c=keep; read a 1bad c; }` leaves a as X and c as
	// keep in all six, and the line is consumed in all six, including the two
	// that would not have read it had `1bad` come first. So a shell that
	// checked the whole list up front would answer a=[] where every shell in
	// the panel answers a=[X].
	ReadRefusesABadNameBeforeReading Answer

	// ReadCountJudgesTheNamesAfterTheFirst keeps judging `read`'s operands
	// as names when `-n` or `-N` gave it a count.
	//
	// bash does; ksh93 stops at the first. Measured 2026-09-07 with
	// `printf 'XYZW\n' | { read -n 3 a 1bad; }`: bash refuses `1bad` and
	// fills a with XYZ, and ksh93 fills a with XYZ and says nothing. Without
	// a count the two agree — `read a 1bad` is refused in both — so it is
	// the count that moves it and not the operand.
	//
	// Left unanswered in the two dialects that cannot reach it: dash has no
	// count letter at all, and zsh's `-k` reads from the terminal rather
	// than from the stream, so neither ever asks. An answer there would be a
	// claim nothing measured.
	ReadCountJudgesTheNamesAfterTheFirst Answer

	// ReadPromptOperand says whether `read`'s first operand may carry a
	// prompt after a `?`, and what an operand that is nothing else names.
	//
	// The form is ksh93's and zsh inherited it: `read "v?Name: "` reads into
	// v and writes `Name: ` at a terminal, which is `read -p` in one word and
	// is what scripts written for either shell use. It is the *first* operand
	// alone — `read v "w?p"` is a bad name `w?p` in both — and the prompt is
	// written for a terminal only, so a piped `read "v?p"` is a plain read
	// into v.
	//
	// It has to be answered wherever the name check is, not beside it: the
	// word a shell judges is the part in front of the `?`, so a check that
	// did not know the form would refuse `read "v?Name: "` in the two shells
	// that spell it that way.
	ReadPromptOperand ReadPromptOperand

	// StdinProgramReadInBlocks takes a program arriving on standard input
	// as much at a time as the descriptor will give, rather than a line at
	// a time. Whatever the block swallowed has left the descriptor, so a
	// `read`, an external command, or anything else the script points at
	// standard input finds only what had not arrived yet.
	//
	// dash alone, measured: `printf 'read x\necho "[$x]"\nDATA\n' | sh`
	// prints `[]` there and then runs `DATA` as a command, where bash,
	// ksh93 and zsh hand the second line to `read` and never parse it.
	// docs/spec/invocation.md has the grid, including the case that shows
	// what the difference really is — `exec 0< file` mid-program replaces
	// the *rest of the program* in the three, and only what follows the
	// block in dash.
	//
	// A bool rather than an Answer, and deliberately: the panel is four to
	// one, so a common denominator exists, and "refuse to read a piped
	// script at all" is not an answer any shell could ship. False is
	// reading by the line, which is what the substrate does.
	//
	// It is the standard-input route's question alone. A script named as an
	// operand is opened separately from standard input, so nothing is
	// shared and all four behave the same way; `-c` reads no descriptor at
	// all. The sibling question for a command string is
	// Diagnostics.CommandStringParsedWhole.
	StdinProgramReadInBlocks bool

	// StdinOptionNamesTheOperands lets the standard-input option name the
	// operands of an invocation that also carries a command string — `sh -sc
	// CMD name a`. The stdin option's rule is that no operand is `$0`: the
	// shell keeps its own name and every operand is a positional parameter,
	// so `$0` is the shell and `$#` is 2. The command string's rule is that
	// the first operand is `$0` and only the rest are parameters, so `$0` is
	// `name` and `$#` is 1.
	//
	// Yes in ksh93 and zsh, no in bash and dash — measured with `-sc`, `-s
	// -c` and `-c -s` alike, since order and bundling change nothing.
	//
	// Asked only when both are given, which is the only place the panel
	// disagrees. Where the program comes from is not this question: all four
	// run the command string, and the corpus pins that separately. Either
	// option alone is unanimous too — the command string names the first
	// operand `$0`, and standard input leaves `$0` as the shell — and with
	// no operands at all the two rules agree by having nothing to name.
	//
	// It has no answer in PosixSemantics, and that is the honest zero
	// rather than an omission: the standard gives `-c` and `-s` separate
	// synopses and says the second is assumed only when the first is absent,
	// so it never describes an invocation carrying both. A 2-2 split with no
	// standard to break it is refused until a dialect chooses.
	StdinOptionNamesTheOperands Answer

	// PlusSignedCommandStringIsDollarZero gives a plus-signed command string
	// `$0` for itself: `sh +c CMD name a` leaves `$0` as CMD and makes every
	// operand a positional parameter, where the minus spelling would have
	// made `name` `$0` and only `a` a parameter.
	//
	// True in ksh93 alone. bash, dash and zsh read `+c` as `-c` in every
	// respect, and all four *run* the command string either way — the sign
	// changes nothing about where the program comes from, which the corpus
	// pins separately.
	//
	// A bool rather than an Answer, for the reason
	// StdinProgramReadInBlocks is one: the panel is three to one, so a
	// common denominator exists, and refusing an invocation every shell
	// runs is not an answer any shell could ship. False is the majority
	// answer and the standard's own — POSIX has no plus spelling of the
	// option at all, so reading it as the option it spells invents nothing.
	//
	// Two further things ksh93 does with `+c` are measured and deliberately
	// not modeled, because they do not agree with each other and read as
	// defects of the 2012 build rather than as a rule: the operands also
	// reach the program as literal words appended to its last command, and
	// a command string of a single word is looked up on PATH and run as a
	// file. docs/spec/invocation.md records both.
	PlusSignedCommandStringIsDollarZero bool

	// LoginProfileWhenNonInteractive has a login shell read its login
	// profile even when there is a script to run rather than a person to
	// prompt. A shell is a login shell when argv[0] begins with a dash,
	// which is what `login` and every terminal emulator's "run as a login
	// shell" does, and the question is only what that then means for a
	// shell that is not going to prompt.
	//
	// dash, ksh93 and zsh read theirs; bash alone reads nothing. Measured
	// 2026-09-05 with a scratch HOME, on all four of the script-operand,
	// `-c`, standard-input and `-s` routes, and the answer is the same on
	// every one of them — this is a fact about the shell rather than about
	// the route. docs/spec/invocation.md has the grid, including the two
	// facts that keep the axis from being wider than it is: an interactive
	// login shell reads its profile in all four, so the interactive route
	// asks nothing, and an explicit `--login` makes bash read it too, so
	// this is about login-ness *inferred from argv[0]* and not about being
	// a login shell as such.
	//
	// A bool rather than an Answer: there is no third thing to do, and
	// "refuse to start" is not an answer any shell could ship.
	//
	// False is the zero value and it is the minority answer, which is the
	// opposite of the way StdinProgramReadInBlocks is named, and on purpose.
	// The majority behavior here is to *read a file out of the invoking
	// person's home directory*, and a Semantics nobody has filled in belongs
	// to a library embedder or a test rather than to a shell — neither of
	// which should touch a home directory because a field was left at its
	// default. A dialect that wants it says so, and PosixSemantics does.
	//
	// Which file a login shell reads is a separate question and is
	// LoginStartupFiles below. The system-wide /etc/profile is still not
	// read at all, on any route; see docs/spec/invocation.md.
	//
	// It is about login-ness **inferred from argv[0]** and not about being a
	// login shell as such, which is measured: an explicit `-l` or `--login`
	// makes bash read its profile with a script to run, so the option
	// overrides this rather than setting the same bit. See
	// StartupFileOptions.Login.
	LoginProfileWhenNonInteractive bool

	// NonInteractiveStartupVariable names a variable whose value is expanded
	// and sourced by a shell that is *not* going to prompt. Empty means the
	// shell has no such file, which is three of the four.
	//
	// A name rather than a bool, for the reason the profile's own filename is
	// not modeled as an axis: what a shell calls the thing is a per-dialect
	// fact and not a disagreement about behavior. One shell in the panel has
	// it, under a name of its own, and the other three do nothing at all with
	// that name — measured 2026-09-05 on a script operand, `-c` and a program
	// on standard input alike.
	//
	// It is the exact counterpart of `$ENV`, which the front end reads for
	// every dialect and reads *only* when interactive. The two never overlap:
	// the shell that has this reads this and not `$ENV` when it is not
	// interactive, and reads neither at a prompt, where it has a file of its
	// own name instead.
	//
	// Two more measured properties, both shared with the profile and both the
	// reason it is sourced where it is. The value is expanded before it is
	// opened, since `$HOME/…` is the usual spelling; and the file is run *by*
	// the shell that is about to run the script, so it sees that shell's `$0`,
	// `$#`, positional parameters and options, and an `exit 3` in it exits 3
	// with the script never run. A file that is not there is not a failure.
	//
	// **POSIX mode suppresses it**, which is measured and is why this is one
	// field rather than two. The shell that has it reads nothing when started
	// with the standard's own posix option, and nothing when invoked as `sh` —
	// the two spellings of the same mode. So the absence in the `sh` column is
	// the mode again rather than a second fact about a second name.
	NonInteractiveStartupVariable string

	// StartupDirectoryVariable names a variable whose value replaces the home
	// directory as the place the startup files below are looked for. Empty
	// means the home directory, which is three of the four.
	//
	// zsh alone has one, `ZDOTDIR`, and it redirects *all* of its files
	// rather than one of them — measured 2026-09-05, with every file under
	// the named directory read and none of the same names under `$HOME`. It
	// is read afresh for each file rather than once, which is also measured
	// and is the reason a person's `~/.zshenv` setting `ZDOTDIR` works at
	// all: the file that sets it is found under the home directory and every
	// file after it under the directory it named.
	//
	// A variable name rather than a path, for the reason
	// NonInteractiveStartupVariable is one: what the shell calls the thing is
	// the dialect's, and the value is the person's.
	StartupDirectoryVariable string

	// UnconditionalStartupFile names a file read on *every* invocation —
	// login or not, prompting or not, `-c` and a script alike. Empty means
	// the shell has no such file, which is three of the four.
	//
	// zsh alone has one, `.zshenv`, and it is the only startup file any shell
	// in the panel reads for a plain `sh -c cmd`. Measured 2026-09-05 with a
	// scratch home directory on all four routes.
	//
	// First of the files, before the profile: measured, `zsh -l -i` reads
	// `.zshenv`, `.zprofile`, `.zshrc` and `.zlogin`, in that order.
	UnconditionalStartupFile string

	// LoginStartupFiles names the profile a login shell reads, most preferred
	// first, as whitespace-separated names. **The first one that can be read
	// is the only one read**, which is bash's rule and reduces to "the file"
	// for every shell with one name for it.
	//
	// Measured 2026-09-05 with a scratch home directory holding a marker for
	// every name: bash reads `.bash_profile`, falls back to `.bash_login` when
	// that is absent and to `.profile` when both are, and reads exactly one of
	// the three. dash, ksh93 and the POSIX preset name `.profile`; zsh names
	// `.zprofile`. Empty means the shell reads no profile, which is what a
	// Semantics nobody has filled in should do — see
	// LoginProfileWhenNonInteractive for why a default must not reach into a
	// home directory.
	//
	// *Whether* a login shell reads it when there is a script to run rather
	// than a person to prompt is the separate question
	// LoginProfileWhenNonInteractive asks; this is only which file.
	//
	// One string rather than a slice, which is how EchoOptions, ReadOptions
	// and JobsOptions already spell a list and is not only consistency: a
	// slice anywhere in this struct makes the whole vector uncomparable, and
	// `==` against another vector is something a test — and an embedder —
	// may already be doing. No shell in the panel names a startup file with
	// a space in it, so nothing is lost by the separator.
	LoginStartupFiles string

	// LateLoginStartupFile names a login file read *after* the interactive
	// file rather than before it. Empty for three of the four.
	//
	// zsh alone has one, `.zlogin`, and the position is the whole of why it
	// is a second field: measured, an interactive login zsh reads `.zprofile`,
	// then `.zshrc`, then `.zlogin`, so a person's `.zlogin` sees what their
	// `.zshrc` did. It is read for a non-interactive login shell too, in the
	// dialects that read a profile there at all.
	LateLoginStartupFile string

	// InteractiveStartupFile names the file read when the shell is
	// interactive, in the startup directory. Empty means the shell has no
	// file of its own name and reads `$ENV` instead, which is dash, ksh93 and
	// the standard.
	//
	// bash names `.bashrc` and zsh names `.zshrc`. Measured 2026-09-05: both
	// read theirs and neither reads `$ENV`, and the shell that reads `$ENV`
	// reads nothing of its own name — the two are alternatives rather than a
	// sequence.
	//
	// **POSIX mode replaces it with `$ENV`**, which is the interactive half of
	// what NonInteractiveStartupVariable records and is measured the same way:
	// bash invoked as `sh` reads `$ENV` at a prompt and does not read
	// `.bashrc`, and so does zsh invoked as `sh`. So this is not suppressed in
	// the mode the way the non-interactive file is — the standard has a file
	// here and the shell reads the standard's one instead of its own.
	InteractiveStartupFile string

	// InteractiveStartupFileWhenLogin has an interactive *login* shell read
	// the interactive file as well as its profile.
	//
	// The panel's one disagreement about startup ordering, and it is why the
	// four combinations of login and interactive are not four independent
	// facts. Measured 2026-09-05 through a pseudo-terminal: zsh reads
	// `.zshrc` for `zsh -l -i` and bash does *not* read `.bashrc` for `bash
	// -l -i` — a person's `.bashrc` is reached from a login bash only because
	// their `.bash_profile` sources it by hand, which is why every bash
	// tutorial tells them to.
	//
	// Asked only where InteractiveStartupFile names something. A shell whose
	// interactive file is `$ENV` reads it in both cases — measured, `-sh -i`
	// reads `.profile` and then `$ENV` in dash, ksh93 and bash-as-`sh` alike —
	// so there is nothing here to answer.
	InteractiveStartupFileWhenLogin Answer

	// StartupFileOptions names the invocation options that say which of the
	// files above to skip, and which file to read in place of the interactive
	// one. The zero value is a shell with no way to skip them.
	//
	// It is a startup input like the files themselves, and the reason it is
	// modeled at all is that **a broken startup file has to be escapable**: a
	// shell whose only `.zshrc` raises an error every time it starts is a
	// shell a person cannot repair from.
	StartupFileOptions StartupFileOptions

	// VersionOption is how this shell answers an invocation option asking it
	// to name its version — `--version`. The zero value is a shell with no
	// such option, which is one of the four.
	//
	// It is an invocation input like StartupFileOptions above, read by the
	// front end rather than by the interpreter, and it is here for the same
	// reason: the spelling is the dialect's, and a front end with an opinion
	// about it would have to hold every dialect's at once.
	VersionOption VersionOption

	// FunctionSearchVariable names the scalar this shell searches for
	// *function definition files* — the parameter an `autoload`d name is
	// looked up on. Empty means the shell has no such search, which is three
	// of the four; zsh names `FPATH`.
	//
	// It is here rather than in the builtin that reads it because the value is
	// not the builtin's to invent. Measured 2026-09-07 against zsh 5.9.2 under
	// `env -i` with a scratch HOME and `-f`, so no startup file is speaking:
	// the shell arrives with three directories on it, all three of them that
	// *installation's* — its own function library, plus the two site
	// directories third-party packages install into. An `FPATH` in the
	// environment **replaces** the lot rather than adding to it, and does so
	// even when it is the empty string, so the default is a fallback for a
	// name the environment does not mention rather than for one it leaves
	// blank.
	//
	// Which directories is a fact about where a shell was installed, and no
	// dialect can hold one: a value written here would be the recording
	// machine's. So this names the parameter and the front end supplies the
	// value — the same split `StartupDirectoryVariable` above already makes,
	// and the same one `$PATH` has. See driver.
	FunctionSearchVariable string

	// ArrayLengthWithoutSubscriptIsCount makes `${#a}` of an array the
	// number of elements, which is zsh's reading; bash and ksh93 measure
	// the element the bare name yields. Asked only where the two answers
	// differ.
	ArrayLengthWithoutSubscriptIsCount Answer

	// WholeSubscriptOnAScalarMeasuresIt makes `${#s[@]}` on a name holding
	// one string the *width of that string* rather than the count of a list
	// of one. Measured 2026-09-10:
	//
	//	                  ${#a[@]} on a=""  on b=x  on h="a b"
	//	bash 5.3.15       1                 1       1
	//	bash as `sh`      1                 1       1
	//	bash 3.2.57       1                 1       1
	//	ksh93             1                 1       1
	//	dash              bad substitution  —       —
	//	zsh 5.9.2         0                 1       3
	//
	// The three-character row is what says which reading it is: an empty
	// scalar answering 0 alone would be "no elements", and `a b` answering 3
	// says the whole-array subscript on a scalar reaches the *value*. The
	// panel is one answer against one, so it is a disagreement and not a
	// majority.
	//
	// Asked of the length alone, because the length is where *this* split
	// falls. The plain value is unanimous — `set -- "${h[@]}"` leaves one
	// parameter holding `a b` in every column — so the fields ask nobody.
	// The slice is not unanimous and is not this question either: it splits
	// the panel a different way and has an axis of its own, immediately
	// below. This comment used to say everything but the length agreed,
	// which is what let the slice keep the count's reading (#1850).
	//
	// It is how a script asks "did I get anything?" after a parse:
	// `local -a opts; zparseopts …; (( ${#opts[@]} ))` reads 1 here for a
	// name that never became an array, which is a count agreeing with the
	// wrong answer (#1553).
	WholeSubscriptOnAScalarMeasuresIt Answer

	// WholeSubscriptOnAScalarSlicesIt makes `${s[@]:off:len}` on a name
	// holding one string a slice of *that string's characters* rather than
	// of a list whose only element is the whole value. Measured 2026-09-11,
	// on `h="a b"` and `h=abcdef`:
	//
	//	              ${h[@]:0:1}  ${h[*]:0:1}  ${h[@]:1}  ${h[@]:2:3}
	//	bash 5.3.15   a            a            ` b`       cde
	//	bash as `sh`  a            a            ` b`       cde
	//	bash 3.2.57   a            a            ` b`       cde
	//	zsh 5.9.2     a            a            ` b`       cde
	//	ksh93         a b          a b          (no field) (empty)
	//	dash          bad substitution
	//
	// A different split from the length above, which is why it is a second
	// question and not a second reading of the first: bash counts a list of
	// one for `${#h[@]}` and slices the characters here, so no single answer
	// about "what a whole subscript on a scalar reaches" fits it. The
	// offsets index the value exactly as `${h:off:len}` does — `${h:0:1}` is
	// `a` in every column, which is the control that says the character
	// reading is not new — and the list reading is what ksh93 keeps.
	//
	// Asked of the slice alone, and only of a name that is set and holds one
	// string: an unset name is empty under both readings, and a real array
	// is a list in every column.
	//
	// Its silence is the reason it is worth an axis rather than a default.
	// `${line[@]:0:1}` reads as the first character to anyone writing it and
	// came back as the whole line, and `${h[@]:1}` — drop the first
	// character — came back as nothing at all, both at status 0 (#1850).
	WholeSubscriptOnAScalarSlicesIt Answer

	// ArrayNameWithoutSubscriptIsTheList makes an unquoted bare array name
	// the array itself — one field per element, a slice slicing the list and
	// an element-wise operator applying to each — exactly as `${a[@]}` is:
	// zsh. bash and ksh93 read the bare name as `${a[0]}`, one field, and
	// dash has no arrays at all.
	//
	// The field-count half of what ArrayScalarIsTheWholeArray answers for the
	// value, and separate from it because the same shell answers the two
	// differently by quoting: `"$a"` is one joined field in zsh as well, so
	// the divergence is exactly the unquoted spelling in a context that
	// splits. Its silence is the reason it is a P1 — `for f in $files` runs
	// once over a joined string instead of once per element, every command
	// inside gets one argument where it expected several, and the status is
	// 0. The same join makes an element-wise operator a quiet no-op:
	// `${a:#pattern}` matches the joined string, fails, and hands the whole
	// array back looking like a filter that found nothing.
	//
	// Asked only where the two readings differ, which is more than one
	// element — or exactly one under an operator that reads the list even
	// then, a slice or one of the element-selecting three.
	ArrayNameWithoutSubscriptIsTheList Answer

	// UnsetNameAtIsOneEmptyField hands a quoted `"${a[@]}"` written on a name
	// that holds nothing at all one empty field. zsh alone, where a name
	// that is not a declared array reads as a scalar and an unset scalar
	// under quotes is one empty field — the same field `"$a"` gives.
	//
	// It is a question about **existence** and not about emptiness. An array
	// that exists and has no elements is no field in every column measured,
	// so that half is core and asks nothing:
	//
	//	f() { printf '%s\n' "$#"; }
	//
	//	                       unset a   declared, no elements   one element
	//	dash                   n/a       n/a                     n/a
	//	bash 5.3, as sh, 3.2   0         0                       1
	//	ksh93u+                0         0                       1
	//	zsh 5.9.2              1         0                       1
	//
	// Measured 2026-09-07 from files, with a *function* rather than `set --`
	// so the positional-parameter builtin is not a confound, and with each
	// shell's own way of declaring an empty array — `a=()` for bash and zsh,
	// `set -A a` for ksh93, which has no such literal.
	//
	// That last clause is the whole reason this axis is spelled this way. It
	// was `EmptyArrayAtIsOneEmptyField`, ksh93 yes, on the strength of
	// `a=(); set -- "${a[@]}"; echo "n=$#"` answering `n=1` there. ksh93 does
	// not read `a=()` as an array literal: it makes a *compound variable*
	// whose value is the two-line text `(\n)`, which `typeset -p a` reports
	// as `typeset -C a=()`, so the one field that row counted held those
	// three bytes rather than nothing. A count-only snippet cannot tell one
	// empty field from one field holding `(`, newline, `)`, and the corpus
	// row recorded the agreement of a coincidence. Asked with `set -A a`,
	// ksh93 gives no field — and `set -A a` on an existing array leaves the
	// name *unset*, so ksh93 has no declared-and-empty state to ask about.
	//
	// zsh is the column that really splits the two, and in the opposite
	// direction from the one that story predicted: `a=()` there is a set,
	// empty array and no field, while a name nothing declared is one field.
	UnsetNameAtIsOneEmptyField Answer

	// SubstringNegativeLengthIsEmpty answers `${x:1:-2}` with nothing at
	// all: ksh93; bash and zsh count the negative length from the end.
	SubstringNegativeLengthIsEmpty Answer

	// SubstringRangeReadsModifiers makes `${x:h}` a *modifier* rather than
	// an arithmetic offset: zsh, where the range is also that shell's
	// history-modifier syntax; bash, ksh93 and dash read it as the
	// expression it looks like everywhere else.
	//
	// The two spellings share every byte of their punctuation, so the reading
	// is decided before either is evaluated, and it is decided by the first
	// byte: a range segment that begins with an unquoted letter is a
	// modifier. `${x:_q:2}`, `${x: i:2}`, `${x:(i):2}`, `${x:$i:2}` and
	// `${x:"h"}` are all substrings in that shell for that reason.
	//
	// Asked only where a segment does begin with one, so `${x:1:2}` needs no
	// answer from anyone.
	SubstringRangeReadsModifiers Answer

	// ReplacementOperandTakesTheEnclosingQuoting reads the replacement half
	// of `${v/pat/repl}` as *content* of the quoting around the expansion
	// rather than as a word of its own. Yes in zsh and in bash 3.2, where
	// `"${s/a/'$v'}"` on `s=xay` and `v=VAL` is `x'VAL'y` — the quotes are
	// two characters of the result and what stands between them is still
	// substituted; No in bash 5.3, that build as `sh` and ksh93, where the
	// quotes quote and are removed, giving `x$vy`.
	//
	// The third of three readings a quote in a `${ }` operand can take, and
	// the only one the panel divides on. A *word* operand takes the enclosing
	// quoting unanimously — `"${u:-'$v'}"` is `'VAL'` in all six — and a
	// *pattern* operand's quotes quote, also unanimously. So neither of those
	// is an axis, and the replacement cannot borrow either one's answer.
	//
	// bash moved between its two builds, which is what says a field named for
	// a shell could not carry it.
	//
	// Asked only at the disagreement: the two readings coincide unless the
	// expansion is double-quoted *and* the operand holds one of the three
	// characters they part on — a single quote, a backslash, or a tilde at
	// the front. Unquoted, all five shells with the operator agree with the
	// word reading, which is what says the disagreement belongs to the
	// enclosing context and not to the operator. The parser decides whether
	// it can arise at all and keeps both readings when it can; see
	// syntax.ParamExpr.Arg2Enclosed (#1209).
	ReplacementOperandTakesTheEnclosingQuoting Answer

	// LinenoCountsFromTheFunction numbers `$LINENO` inside a function from
	// the line the function was written on: zsh; the other three count from
	// the file.
	//
	// Inside means the line is one the body holds. A file the function
	// sourced counts from its own top, because the line is the file's — the
	// same innermost-frame rule Diagnostics.LocationNamesTheFunction is read
	// by, and measured the same way (#2037).
	LinenoCountsFromTheFunction Answer

	// ArithBaseAbove36 admits `37#…` through `64#…`, whose letters split
	// into cases and whose last two digits are `@` and `_`. bash and ksh93
	// take the full 64; zsh stops at 36 and says so.
	ArithBaseAbove36 Answer
	// ArithBaseMayHaveALeadingZero lets `010#5` name base ten. The base is
	// read in decimal either way; what this decides is whether a zero in
	// front of it is padding or the start of an octal constant.
	//
	// Measured 2026-09-12 over the panel, with the digit chosen so the two
	// readings could not agree — `010#5` is five under both base eight and
	// base ten, which is the probe the issue was filed from:
	//
	//	          010#5   010#9   010#11   08#7   0010#5
	//	bash 5.3  refuse  refuse  refuse   refuse refuse
	//	ksh93u+   refuse  refuse  refuse   7      refuse
	//	zsh 5.9.2 5       9       11       7      5
	//
	// So zsh reads the base in plain decimal, padding and all. bash refuses
	// every one of them for a single reason that is not about bases at all:
	// a leading zero makes the text an octal constant there, so the `#` is
	// never a base marker and the literal fails as a number — which is why
	// `08#5` is "value too great for base" and `010#5` "invalid number".
	// ksh93 says yes to the zero and no to the length, which is
	// ArithBaseIsAtMostTwoDigits beside this one.
	//
	// dash has no `base#digits` at all, so its column is the same refusal it
	// gives `10#5`.
	ArithBaseMayHaveALeadingZero Answer
	// ArithBaseIsAtMostTwoDigits stops the base after two characters, which
	// is as many as a base up to 64 needs. ksh93 alone.
	//
	// Measured 2026-09-12, and it is the reading that survived three
	// hypotheses — an octal base, a decimal base, and a two-character cap:
	//
	//	02#11    3        base two, so the cap took `02`
	//	0002#11  refuse   four characters, so the cap took `00`
	//	64#10    64       two characters is enough for the widest base
	//	012#11   refuse   the cap took `01`, which is no base
	//	020#11   refuse   the cap took `02` and left `0#11`
	//
	// A decimal reading answers 13 and 21 to the last two and an octal one
	// 11 and 17; ksh93 answers neither, and the cap explains all five.
	ArithBaseIsAtMostTwoDigits Answer
	// ArithBaseZeroReadsTheDigitsAsWritten answers `0#5` with 5 rather than
	// refusing a base of zero: the digits are read as an ordinary constant,
	// prefix and all, so `$(( 0#0x10 ))` is 16.
	//
	// zsh alone, and it is not the same question as a base below two:
	// measured 2026-09-12, `$(( 1#0 ))` there is `invalid base (must be 2 to
	// 36 inclusive): 1` while `$(( 0#5 ))` is 5 — so zero is a base it reads
	// through rather than one it refuses. bash never sees a base at all in
	// either, the leading zero having made the text an octal constant, and
	// ksh93 refuses both.
	ArithBaseZeroReadsTheDigitsAsWritten Answer
	// ArithEmptyRadixDigitsAreZero reads `0x` — a radix prefix with no digits
	// after it — as a complete number worth zero, rather than refusing it.
	//
	// Measured 2026-09-12. `$(( 0x ))` and `$(( 0X ))` are 0 in bash 5.3,
	// bash 3.2, bash-as-sh and zsh, and refused by ksh93 and dash. The
	// control is `$(( 0x+1 ))`, which is 1 in the four that accept it: the
	// prefix is a *finished* number and the `+1` goes on from it, rather
	// than the `+` being swallowed by a digit scan that found nothing.
	//
	// Only a radix prefix. `$(( 8# ))` — a named base with no digits — is a
	// separate row the panel answers differently again (bash 5.3 refuses it
	// where bash 3.2 answers zero), and is not this axis.
	ArithEmptyRadixDigitsAreZero Answer
	// ArithOverflowSaturates clamps integer overflow at the edge: ksh93
	// holds max+1 at the maximum where the other shells wrap. Asked only
	// when an overflow actually happened.
	ArithOverflowSaturates Answer
	// EmptyArithExpressionIsAnError refuses `$(( ))`: dash wants a primary
	// and stops the script; the other three answer zero.
	EmptyArithExpressionIsAnError Answer
	// TildePlusMinusExpands turns `~+` into $PWD and `~-` into $OLDPWD,
	// only while the variable is set — a fresh shell's `~-` stays literal.
	// bash, ksh93 and zsh have the pair; dash keeps both as written. zsh
	// alone still answers `~-` after `unset OLDPWD`, from directory state
	// of its own this runner does not keep — recorded, not reproduced.
	TildePlusMinusExpands Answer

	// SetHasTraceLetters gives `set` the -E and -T letters, which carry
	// the ERR trap (and DEBUG with RETURN) into functions and subshells the
	// dialect otherwise bounds them out of. bash alone: dash and ksh93
	// refuse the letters, and zsh spells different options with them, so
	// only a refusal is honest elsewhere. Recorded as
	// `opt/set-e-carries-the-err-trap`.
	SetHasTraceLetters Answer

	// SetHasTheTLetter gives `set` the -t letter: the shell reads and runs
	// one more line and then stops, which is what bash lists as `onecmd` and
	// what ksh93 spells with the letter alone. Two of the panel have it and
	// mean this by it; zsh has the letter and refuses to move it, the way it
	// refuses the `onecmd` name it borrowed for `singlecommand`; dash has
	// never heard of it and answers `Illegal option -t`. Recorded as
	// `opt/set-t-stops-after-one-command`.
	SetHasTheTLetter Answer

	// ImmovableOptionsSetAtInvocation lets the command line that started the
	// shell move an option a *running script* may not — a route split inside
	// one shell rather than a disagreement between two, which is why it is
	// asked where the route is known instead of where the option is.
	//
	// One of the panel has it. Measured on zsh 5.9.2, 2026-09-10: `zsh -t
	// plain.sh` runs the first line of a three-line script and stops, and
	// `-o singlecommand` and the borrowed `-o onecmd` do the same, while
	// `set -t`, `setopt singlecommand` and `unsetopt singlecommand` inside
	// that script are all `can't change option` at 1 and fatal. So the five
	// names that shell calls fixed are not five states it cannot reach; they
	// are five a script may not change.
	//
	// It governs the refusal and not the applying: a dialect that answers
	// `Yes` still has to say what each such name *would* move, which for the
	// letter is the substrate's `onecmd` and for a name is the dialect's own
	// table (see the zsh dialect's singleCommandOption). A name with nothing
	// to apply is refused at the invocation exactly as it is refused in a
	// script.
	//
	// unexhibited No: nobody, and nothing in the panel can reach it.
	// Measured 2026-09-12: zsh 5.9.2 is the only column with an option a
	// running script may not change, so it is the only one the axis is
	// consulted in, and it answers Yes. bash's `-r` is not this shape —
	// `set -r` is taken inside a script in all three bash columns and in
	// ksh93u+, and only `set +r` is refused, which is a latch and not a
	// fixed option. `No` is the other side of a binary Answer, read (`==
	// Yes`) rather than asked (#2060).
	ImmovableOptionsSetAtInvocation Answer

	// OneCommandStopsACommandString extends `set -t` to `-c`, and it is the
	// one route the two shells that have the option disagree about.
	// Measured: a two-line command string that sets it and then echoes —
	// bash writes the echo, the option is on and `$-` says so, and the shell
	// reads the rest of the string anyway — where ksh93 given the same
	// string writes nothing. Both stop a script file and both stop standard
	// input, so the question is this route and no other. Asked only where
	// the option is on; see Runner.OneCommand.
	OneCommandStopsACommandString Answer

	// SetHasTheHLetter gives `set` the -h letter at all. Three of the four
	// have it and no two mean quite the same thing by it — which option it
	// abbreviates is SetHLetterTracksCommands — while dash refuses the
	// letter outright, fatally, the way it refuses any letter it does not
	// have.
	SetHasTheHLetter Answer

	// SetHLetterTracksCommands makes `set -h` the short spelling of command
	// tracking — the option bash lists as hashall and ksh93 as trackall,
	// permission to remember where commands were found. zsh answers no: its
	// -h abbreviates histignoredups, a history option, and leaves command
	// hashing alone. Asked only where the letter is written, like
	// SetFTurnsOffGlobbing: the long names raise no question.
	SetHLetterTracksCommands Answer

	// MonitorNeedsATerminal ties turning `set -m` on to having a terminal.
	// Measured in shells run with none, which is what a script has: bash
	// and ksh93 grant the option silently; dash remarks `can't access tty;
	// job control turned off` and reports success with the option left off;
	// zsh refuses it at 1, fatally. The two refusal shapes are the
	// dialect's own wording and status — Diagnostics.MonitorDenied and
	// MonitorDeniedStatus. A runner whose front end gave it a person to
	// report jobs to (JobControl) has a terminal, so the question is asked
	// only without one. Turning the option *off* is granted everywhere.
	MonitorNeedsATerminal Answer

	// InteractiveMonitorNeedsATerminal ties the monitor an *interactive*
	// shell turns on for itself to having a terminal, which is a different
	// question from the one above: that one is a script asking with `set -m`,
	// and this one is nobody asking at all.
	//
	// The rule the answer qualifies is unanimous and is not an axis. Measured
	// 2026-09-05 on `-i script.sh` with a scratch HOME and a pseudo-terminal:
	// bash 5.3.15, dash, ksh93u+ and zsh 5.9.2 all report `monitor on` and
	// all four put `m` in `$-`. So an interactive shell runs the monitor, and
	// a front end that leaves it off is wrong on every route rather than in
	// one dialect.
	//
	// What splits is the same invocation with no terminal anywhere: ksh93
	// still reports `monitor on` and `imBE`, and bash, dash and zsh all
	// report it off and leave `m` out. True in bash, dash and zsh; false in
	// ksh93.
	//
	// The terminal that counts is a terminal on any of the three standard
	// streams, and that is measured rather than assumed. A controlling
	// terminal with all three redirected elsewhere is *not* enough — bash,
	// dash and zsh all report the monitor off there — and a pseudo-terminal
	// on any one of the three alone is enough for all three of them. So the
	// question the front end has to answer is about the descriptors it was
	// handed, which is the one it can answer.
	//
	// It is not MonitorNeedsATerminal read a second time, and bash is what
	// separates them: `bash -c 'set -m'` with no terminal turns the monitor
	// on, and `bash -i script.sh` with no terminal leaves it off. One shell,
	// two answers, so an explicit request and an automatic one are two
	// questions.
	//
	// The preset says a terminal *is* needed, and this is the rarer case
	// where the text does not decide. XCU says of `-m` that it "shall be
	// enabled by default for interactive shells" and puts no terminal in
	// that sentence, but it also defines job control throughout in terms of
	// a controlling terminal, so the sentence is silent about having none
	// rather than permissive about it. Silent text gets the answer that
	// claims less — a shell with no terminal does not report a monitor —
	// which is three of the four as well.
	//
	// Read rather than `ask`ed, exactly as InteractiveOptionLetters is: the
	// answer is wanted once at startup, before the program has run a line, so
	// refusing over an unanswered field would put "the shells disagree here"
	// ahead of every `-i script.sh` under a preset that has not chosen. An
	// unanswered field reads as Yes — a terminal is needed and the monitor
	// stays off, which is the majority and the quiet answer.
	//
	// zsh is worth knowing about and is not this axis. With a terminal it
	// puts `m` in `$-` and announces its jobs while its own `set -o` still
	// lists `monitor off` — it disagrees with itself, and what is recorded
	// here is the state the other two readers report.
	InteractiveMonitorNeedsATerminal Answer

	// InteractiveScriptAnnouncesJobs gives an interactive shell running a
	// *named script file* somebody to tell about its jobs: the job number
	// and pid as one starts, and the `Done` row as one ends. True in dash,
	// ksh93 and zsh; false in bash.
	//
	// A different question from AnnouncesBackgroundJob, which asks whether
	// the *start* is announced at all and is answered No by dash alone. Both
	// are read on this route, and dash is why they cannot be one field: it
	// announces the end of a job here and never the beginning.
	//
	// Measured 2026-09-05 through a pseudo-terminal, scratch HOME and scratch
	// HISTFILE, on `sh -i script.sh` running `sleep 0.3 &` between two
	// echoes:
	//
	//	bash 5.3.15   nothing         bash 3.2.57  nothing
	//	bash as `sh`  nothing         dash         the `Done` row, no start
	//	ksh93u+       both            zsh 5.9.2    both
	//
	// It is not the monitor asked a second time. The monitor is unanimous on
	// this route with a terminal — InteractiveMonitorNeedsATerminal records
	// that — and this is not, so a front end that turned both on together
	// would give bash an announcement no bash makes.
	//
	// It is however *gated* on the monitor, which is measured: with no
	// terminal anywhere, dash and zsh leave the monitor off and say nothing
	// about the job either, and ksh93 runs the monitor without one and
	// announces both ends. So the notice rides on the monitor and this axis
	// is what the one dialect that runs a monitor and stays quiet anyway is
	// for.
	//
	// And bash's silence is not about where the commands come from, which is
	// the reading the grid rules out: `bash -i < script` with the program on
	// a *pipe* announces both, and so does `bash -i -c`. Measured, bash is
	// silent on exactly one interactive route, the one whose program is a
	// named file — which is why this axis names the route rather than the
	// terminal.
	//
	// The preset says no. XCU has nothing to say about a notice on this
	// route, and where the text is silent the preset takes the answer that
	// claims less: a shell that has not been asked for a job report does not
	// write one. It is also the intersection — the whole panel is quiet on
	// this route only if bash is — and the core is the intersection rather
	// than the majority.
	//
	// Read rather than `ask`ed, exactly as InteractiveMonitorNeedsATerminal
	// is and for the same reason: the answer is wanted once at startup,
	// before the program has run a line, so an unanswered field would put
	// "the shells disagree here" ahead of every `-i script.sh` under a preset
	// that has not chosen — including scripts that never mention a job.
	//
	// `-i -c` is a separate question and is deliberately not this one. On
	// that route bash, ksh93 and zsh announce and dash does not, which is a
	// different split and therefore a different axis; `docs/spec/invocation.md`
	// has the grid.
	InteractiveScriptAnnouncesJobs Answer

	// PunctuatedFunctionNameIsRefused stops the script when a function
	// whose name carries `-` or `.` is defined. ksh93 alone: bash and zsh
	// define and run it, and dash never parses the definition at all.
	PunctuatedFunctionNameIsRefused Answer

	// DirectoryOnPathIsACandidate keeps a directory the PATH search found as
	// the failed candidate when no later entry runs, so the report names the
	// directory rather than saying the command was never found.
	//
	// Every shell measured continues the search past the directory — that is
	// unanimous, and is what makes a shim directory early on PATH work at
	// all. They part ways only when nothing later matches: bash reports the
	// name as not found at all (status 127), where dash, ksh93 and zsh
	// report the directory they could not run. dash alone keeps 127 for the
	// status even then, which is DirectoryOnPathStatus's question.
	DirectoryOnPathIsACandidate Answer

	// ExecTakesOptions lets `exec` read options of its own, such as
	// `-a name` to choose the argv[0] the command sees. True in bash, ksh93
	// and zsh; false in dash, where a leading `-a` is the name of a command
	// and is reported as not found.
	//
	// The answer has to come before the command is looked up, because it
	// decides which word the command is.
	ExecTakesOptions Answer

	// DotFallsBackToCurrentDirectory looks in the current directory for a
	// `.` operand with no slash in it, after PATH has missed.
	//
	// True only in bash. PATH is searched first everywhere, and wins over an
	// identically named file in the current directory in all four — this is
	// only about what happens when PATH does not have it.
	DotFallsBackToCurrentDirectory Answer

	// TestAcceptsDoubleEqual makes `==` a synonym for `=` in `test` and `[`,
	// so `test a == a` is a string comparison. True in bash, ksh93 and zsh.
	//
	// False in dash, and false does not mean "compares unequal": it means the
	// word is not an operator at all, so `test a == b` is three words with no
	// operator among them and is reported as one. The answer therefore has to
	// come before the comparison, not after it.
	//
	// This is only about `test` and `[`. Inside `[[ ]]` the same spelling is
	// a pattern match, which is a different question entirely.
	TestAcceptsDoubleEqual Answer

	// SignalDeathStatusIsTwoFiftySix encodes a command killed by a signal as
	// 256 + the signal rather than 128 + the signal. True only in ksh93,
	// which reports 265 for KILL and 271 for TERM where the other three
	// report 137 and 143.
	//
	// Measured across eight signals; it is not a special case for any one of
	// them. POSIX requires only "greater than 128", which decides nothing,
	// so the preset follows the three that agree.
	SignalDeathStatusIsTwoFiftySix Answer

	// PipefailOption is whether `set -o pipefail` exists, making a pipeline
	// report its last failing element rather than its last element. True in
	// bash, ksh93 and zsh; absent from dash and from POSIX, where a pipeline
	// is defined to report its last command and nothing offers to change it.
	//
	// Not a wording difference: where it is absent the name is not an option
	// at all, so `set -o pipefail` fails and the pipeline goes on reporting
	// its last element — which is the answer a script guarding against a
	// failure upstream is specifically trying not to get.
	PipefailOption Answer

	// ErrexitSeesPipefailFailure lets `set -e` stop for a failure that only
	// pipefail produced — a pipeline whose last element succeeded and whose
	// earlier one did not. True in bash and zsh; false in ksh93, which runs
	// on.
	//
	// Absent rather than false in dash, which has no pipefail, so the
	// question cannot arise there and is never asked.
	//
	// Narrower than it looks: an ordinary failing pipeline — `true | false` —
	// stops all three, and this is only about the failure the option adds.
	ErrexitSeesPipefailFailure Answer

	// PipefailSubstitutesTheBareSignal reports an element pipefail chose over
	// the pipeline's last one, and which died of a signal, as the signal's
	// *number* rather than as the status a command killed by that signal
	// reports.
	//
	// True in ksh93 alone, and it is not the same question as
	// SignalDeathStatusIsTwoFiftySix. That axis is about every status a
	// signal death produces, and ksh93 answers it consistently everywhere it
	// was measured — a foreground command, a subshell, a command
	// substitution, a `wait`, the shell dying by its own hand as its parent
	// sees it, and the *last* element of a pipeline are all 256 + n there.
	// This is the one place the convention stops: with `pipefail` set,
	//
	//	kill-me-with-TERM | cat     ksh93  15    bash/zsh  143
	//	kill-me-with-PIPE | head -1  ksh93  13    bash/zsh  141
	//	( exit 42 )       | cat      ksh93  42    bash/zsh   42
	//	cat </dev/null | kill-me     ksh93 271    bash/zsh  143
	//
	// The last row is why this is about the *substitution* and not about the
	// pipeline: an element that fails in the position the pipeline reports
	// anyway keeps the ordinary encoding, and only the status pipefail went
	// looking for is bare. An ordinary non-zero exit is unchanged either way,
	// so a signal is the whole of the difference.
	//
	// Measured builtin and external, first and middle, in pipelines of two
	// and of three, with SIGPIPE and SIGTERM. Absent rather than false in
	// dash, which has no pipefail, and asked only where a substitution
	// actually happened and actually was a signal death.
	PipefailSubstitutesTheBareSignal Answer

	// PrintfAssignsWithV makes `printf -v name fmt args` put the formatted
	// text in a variable and print nothing. True in bash and zsh; dash and
	// ksh93 have no such option and reject it as an unknown one.
	//
	// It is how a script formats a value without a command substitution, so
	// without it the text goes to stdout and the variable stays empty — two
	// wrongs at once, and both silent.
	PrintfAssignsWithV Answer

	// PrintfRejectsUnknownOption treats any leading word starting with `-` as
	// an option and refuses one it does not know. True in bash, dash and
	// ksh93, where even `printf "-%s\n" x` is an error because the format
	// itself begins with a dash.
	//
	// False in zsh, which recognizes the options it has and takes anything
	// else as the format — so `printf -q x` prints `-q` there and is an error
	// in the other three.
	PrintfRejectsUnknownOption Answer

	// TrapParsesOptions reads a leading `-` word as an option rather than as
	// the action to run.
	//
	// Three of the four do. zsh does not, so `trap -p` sets a trap whose
	// action is the word `-p` and the failure surfaces later, when it fires
	// — which is what this shell did for every dialect before there were
	// options here at all.
	//
	// Asked of the letters a dialect knows as much as of the ones it does
	// not, because zsh takes `-p` as the action just as it takes `-Q`. A
	// lone `-` is trap's own word for "put it back" and is never an option,
	// and `--` ends them in all four.
	TrapParsesOptions Answer

	// TrapPrintsWithP makes `trap -p` write the traps currently set, and
	// `trap -p condition ...` only the named ones. bash and ksh93 have it;
	// dash rejects the letter along with every other.
	TrapPrintsWithP Answer

	// TrapPrintsBareWithP makes `trap -P condition ...` write the action
	// alone, with no `trap --` around it. bash only, and it is the one option
	// that insists on an operand: printing all of them is `-p`'s job.
	TrapPrintsBareWithP Answer

	// TrapListsSignalsWithL makes `trap -l` list the signal names, the way
	// `kill -l` does. bash only — ksh93 refuses the letter.
	TrapListsSignalsWithL Answer

	// TrapPrintsBareWithConditions makes `trap -p condition ...` write the
	// action alone rather than the whole `trap -- action condition` line.
	//
	// ksh93 only, and it is why `-p` and bash's `-P` are two questions and
	// not one: ksh93 reaches bash's `-P` output through `-p` with an operand,
	// and has no `-P` at all.
	TrapPrintsBareWithConditions Answer

	// TrapOneArgumentIsACondition reads `trap EXIT` as "put EXIT back"
	// rather than as an action with no condition to attach it to.
	//
	// Three of the four do, which makes `trap EXIT` the short spelling of
	// `trap - EXIT`. ksh93 refuses the form and the refusal ends the script.
	TrapOneArgumentIsACondition Answer

	// TrapReportsAnUnknownSingleCondition complains when that one word turns
	// out not to name a condition. zsh says nothing — and does complain about
	// `trap : foo`, so this is the single-word form's own answer rather than
	// zsh declining to check at all.
	TrapReportsAnUnknownSingleCondition Answer

	// TrapSingleUnknownConditionIsUsage prints the usage line rather than
	// naming the word. bash does: with one word it cannot tell a misspelled
	// condition from an action someone forgot to give a condition to. dash
	// names the word the same way it does anywhere else.
	TrapSingleUnknownConditionIsUsage Answer

	// TrapHasErrCondition makes `trap … ERR` a condition rather than a
	// misspelled signal: the action runs after every command that fails
	// where `set -e` would judge it — with or without `set -e` on, which is
	// measured rather than assumed. dash alone refuses the name, with the
	// same words it refuses any other word that names no signal.
	TrapHasErrCondition Answer

	// TrapHasDebugCondition makes `trap … DEBUG` run the action before each
	// simple command. dash alone refuses the name.
	TrapHasDebugCondition Answer

	// TrapHasReturnCondition makes `trap … RETURN` a condition that fires
	// when a sourced file finishes, and when a function whose own body set
	// the trap returns. bash alone; the other three refuse the name the way
	// they refuse any word that names no signal.
	TrapHasReturnCondition Answer

	// ErrTrapRunsInsideFunctions fires the ERR trap for a failure inside a
	// function the trap was not set in. bash does not — there a function
	// does not inherit the ERR trap, so only the call itself is judged
	// where the trap can see it. ksh93 and zsh fire it inside too.
	//
	// The suppression is per *frame*, not per depth, which is measured: a
	// trap set inside a function fires in that function and at the top
	// level after it returns, and does not fire inside a sibling function
	// entered afterwards, though the sibling's own failing call still does.
	ErrTrapRunsInsideFunctions Answer

	// ErrTrapRunsInSubshells fires the ERR trap for a failure inside a
	// subshell or a command substitution. zsh alone: `trap 'echo E' ERR;
	// x=$(false; echo hi)` captures an E there and nowhere else. bash and
	// ksh93 reset the trap on the way into the child, the way they reset
	// every trap that is not ignored.
	ErrTrapRunsInSubshells Answer

	// DebugTrapRunsInsideCalls fires the DEBUG trap before commands inside
	// a function or a sourced file the trap was not set in. bash does not;
	// ksh93 and zsh do. Not the ERR axis under another name, and not only
	// because bash controls the two with different options: a sourced file
	// bounds DEBUG there and does not bound ERR — measured, with a
	// top-level trap of each, `false` inside a dotted file fires ERR and
	// the commands of the same file fire no DEBUG.
	DebugTrapRunsInsideCalls Answer

	// DebugTrapRunsInSubshells fires the DEBUG trap inside a subshell or a
	// command substitution. ksh93 and zsh do — a command substitution there
	// captures the handler's output into the variable — and bash does not,
	// which is a grouping ErrTrapRunsInSubshells does not have: ksh93
	// carries DEBUG into the child and not ERR.
	DebugTrapRunsInSubshells Answer

	// A subshell starts with the parent's handled traps back at their
	// defaults and only an ignored signal still ignored — POSIX, and
	// unanimous in the working state. What `trap` *lists* in the child is
	// where the panel splits, and it splits by the kind of boundary, so the
	// question is asked once per kind rather than once. The listing survives
	// until the child modifies a trap, at which point every shell that kept
	// it shows the child's own state instead — `(trap '' USR2; trap)` lists
	// USR2 and nothing the parent had.

	// SubshellKeepsTrapListing makes `trap` inside `( … )` or `$( … )` still
	// list the traps the parent had, though a handled one no longer fires —
	// the save=$(trap) idiom POSIX carves out, extended to the compound.
	// bash and ksh93; dash and zsh list only what survived the entry.
	SubshellKeepsTrapListing Answer

	// PipelineElementKeepsTrapListing is the same question asked of a
	// pipeline element that runs in a subshell environment, and the panel
	// pairs off the other way: bash and zsh keep the listing there, ksh93
	// and dash do not. `trap 'echo x' USR1; trap | cat` prints the trap in
	// bash and zsh and nothing in the other two — the shape issue #339
	// measured.
	PipelineElementKeepsTrapListing Answer

	// BackgroundJobKeepsTrapListing asks it of `… &`. bash alone: the other
	// three list nothing the parent had there.
	BackgroundJobKeepsTrapListing Answer

	// KeptTrapListingIncludesExit says a kept listing shows the parent's
	// EXIT trap alongside the signals. bash and ksh93 list it; zsh keeps a
	// pipeline element's listing and still drops EXIT from it. Unanswerable
	// where nothing is kept, so dash never reaches the question.
	KeptTrapListingIncludesExit Answer

	// SubshellHidesInheritedIgnoredTraps drops an *inherited* ignore from
	// the child's listing while the signal stays ignored in fact: zsh, where
	// `trap '' INT; (trap)` prints nothing and `(kill -INT $$; echo alive)`
	// still prints alive. The other three list what POSIX says is still a
	// current trap. An ignore the child sets itself is listed everywhere.
	SubshellHidesInheritedIgnoredTraps Answer

	// LocalOutsideAFunctionIsAnError refuses `local x=2` written where there
	// is no function to be local to.
	//
	// bash and dash refuse it, zsh takes it and sets a global instead. ksh93
	// has no `local` at all, so it never reaches the question.
	LocalOutsideAFunctionIsAnError Answer

	// LocalOutsideAFunctionIsFatal ends the script rather than carrying on
	// after that refusal. dash does; bash says the same thing and runs the
	// next command.
	LocalOutsideAFunctionIsFatal Answer

	// UmaskPrintsFourDigits writes the mask as four digits, always — `0022`
	// against zsh's `022`. True in bash, dash and ksh93.
	//
	// False is not "three digits". zsh writes a C octal literal with a
	// minimum of three, so the leading zero comes back as soon as the owner
	// group denies anything: `022` and `077`, but `0333` and `0777`. Reading
	// this as a flat three printed `333` where zsh prints `0333`.
	//
	// Only about printing: all four read `022` and `0022` alike, and the
	// symbolic form `umask -S` is identical in every one of them.
	UmaskPrintsFourDigits Answer

	// UmaskSetWithSPrints echoes the new mask when `umask -S mask` both sets
	// and is asked for the symbolic form. True only in bash, which prints
	// `u=rwx,g=,o=` after setting; the other three set and say nothing.
	//
	// Only for that combination: `umask mask` is silent in all four, and
	// `umask -S` with no mask prints in all four.
	UmaskSetWithSPrints Answer

	// UlimitBlockIsKilobyte counts `ulimit -c` and `-f` in 1024-byte blocks
	// rather than POSIX's 512. True only in bash.
	//
	// Measured rather than read: `ulimit -f 1` then writing until the kernel
	// objected. bash allowed 1000 bytes and refused 1200; dash, ksh93 and zsh
	// refused 600. The probe lives in docs/spec/semantics.md rather than in
	// the corpus — bash and ksh93 announce the killed writer by process id,
	// which no golden record can hold.
	UlimitBlockIsKilobyte Answer

	// UlimitHasResidentSet is `ulimit -m`. True in bash, dash and ksh93; zsh
	// has no such letter and reports it as a bad option. Recorded as
	// `ulimit/a-letter-zsh-does-not-have`.
	UlimitHasResidentSet Answer

	// UlimitHasProcessCount is `ulimit -u`. True in bash, ksh93 and zsh; dash
	// has no such letter. Recorded as `ulimit/a-letter-dash-does-not-have`.
	UlimitHasProcessCount Answer

	// UlimitSetsBothLimits lowers the hard limit along with the soft one when
	// neither -H nor -S was given — which is what makes `ulimit -t 3600`
	// irreversible. True in bash, dash and ksh93.
	//
	// False in zsh, which sets only the soft limit and leaves the hard one
	// where it was, so the same line there can be undone.
	UlimitSetsBothLimits Answer

	// BadOptionToSpecialBuiltinFatal ends the script when a special builtin is
	// given an option it does not have. True in dash and ksh93, which is the
	// POSIX rule that a special builtin's failure is fatal; bash and zsh
	// report it and carry on.
	//
	// A different question from BuiltinSyntaxErrorFatal, which is about text
	// that would not *parse* and is true for dash alone. Measured across
	// `export`, `readonly` and `unset`.
	BadOptionToSpecialBuiltinFatal Answer

	// MultiDigitDuplicationTargetIsAnError refuses `>&10` — a duplication
	// whose *target* is written with more than one digit. True in dash
	// alone; the other four read the number and fail at run time with `10:
	// Bad file descriptor` if nothing is open there, at status 1, and the
	// script carries on.
	//
	// The companion question, how many digits may stand *before* the
	// operator, is the grammar's and has the opposite dissenter: bash alone
	// reads `exec 10>f` as a redirection where the other three run a command
	// called `10` (Dialect.MultiDigitFdNumber). Two questions that split the
	// panel the other way round cannot be one flag read from both ends.
	//
	// It is not a parse refusal, though the shell that has it words it as a
	// syntax error: `sh -n -c 'echo hi >&10'` accepts the input and exits 0,
	// and a script prints its earlier lines before stopping on this one. The
	// grammar takes the construct everywhere, so this is a semantics axis.
	//
	// The width is what is refused and not the value — `>&08` names
	// descriptor 8 and is refused too — which is what keeps this separate
	// from FdNumberBoundedByOpenFileLimit. And it is the *expanded* word:
	// `n=10; echo hi >&$n` is refused where `n=9` is not.
	//
	// The refusal ends the script, which travels with the answer rather than
	// being an axis of its own — one shell refuses and that shell stops. The
	// status is FatalErrorStatusIsOne's, as every fatal error's is.
	//
	// Asked only where a target really is wider than one digit; `>&2` is
	// nobody's question.
	MultiDigitDuplicationTargetIsAnError Answer

	// GreatAmpTarget is what `>&word` does with a word that is not a
	// descriptor number — refuse it, or open it as a file for both output
	// streams, which is the csh spelling of `&>word`. A form rather than a
	// flag because two of the shells that keep the form disagree about a
	// word that expanded to nothing; see GreatAmpTargetForm.
	GreatAmpTarget GreatAmpTargetForm

	// DuplicationTargetErrorOnABuiltinIsFatal ends a non-interactive shell
	// when `<&word` names something that is not a descriptor and the command
	// it is written on runs *in* the shell.
	//
	// zsh alone, and the boundary is the command rather than the redirection.
	// Measured 2026-09-06: `cat <&""` and `/bin/echo hi <&""` complain and
	// carry on, while `read x <&""`, `echo hi <&""`, `true <&""` and `: <&""`
	// end the shell — the same word, the same complaint, and a builtin on the
	// left.
	//
	// Not RedirectErrorOnSpecialBuiltinFatal, which zsh answers No and which
	// would not reach `read` or `echo` in any case. Nor is it redirection
	// failure in general: an ordinary one on a zsh builtin — `read x
	// 3>/nope/x`, `read x <&9` — complains and carries on there too. It is
	// this refusal, on a builtin.
	DuplicationTargetErrorOnABuiltinIsFatal Answer

	// RedirectErrorOnSpecialBuiltinFatal ends a non-interactive shell when a
	// redirection written on a *special* builtin cannot be made — a file that
	// will not open, a descriptor that is not there, a number the open-file
	// limit refuses. POSIX states it outright, and it is one rule with a wide
	// reach: `exec 3>/nope/x`, `: 3>/nope/x`, `eval : 3>/nope/x` and
	// `. /dev/null 3>/nope/x` all stop where any of them does.
	//
	// The failure is the *redirection's*, so the builtin never runs and its
	// own status is never reached; the status of the shell that stops is
	// FatalErrorStatusIsOne's, which is why dash exits 2 here and the rest
	// exit 1 without this needing a status of its own.
	//
	// The boundary is measured rather than assumed. A regular builtin
	// (`true 3>/nope/x`) and an external command are unaffected everywhere,
	// and so is a *compound* command's own redirection — `{ echo x; } 3>/f`
	// carries on in all five, because the redirection is the group's and not
	// a special builtin's. Inside a subshell it ends the subshell alone and
	// the parent runs on; inside a function it ends the shell.
	//
	// **This axis is POSIX mode, not a shell.** The panel splits three to
	// two — dash, ksh93 and bash-as-`sh` stop; bash and zsh carry on — and
	// the bash column and the bash-as-`sh` column are the same binary. The
	// flip is reachable at runtime in both shells that have such a mode, and
	// that is what makes this an axis rather than a quirk of an invocation:
	// `set -o posix` makes bash 5.3 and bash 3.2 stop, `set +o posix` makes
	// bash invoked as `sh` carry on, and `emulate sh` or `emulate ksh` makes
	// zsh stop where `emulate zsh` does not. zsh invoked as `sh` stops too,
	// so the same argv[0] moves two different binaries the same way.
	//
	// So a dialect's field here is where the shell *starts*, and the shell's
	// own posix knob moves it — see dialect/bash's `posix` option and
	// dialect/zsh's `emulate`. Nothing is attached to argv[0]: naming the
	// invocation would record the accident and lose the rule.
	RedirectErrorOnSpecialBuiltinFatal Answer

	// BadNameToDeclarationFatal ends the script when `export` or `readonly` is
	// given an operand that is not a name. True in dash, ksh93 and zsh; bash
	// reports every bad operand, exports the well-formed ones and carries on
	// with a status of 1.
	BadNameToDeclarationFatal Answer

	// BadNameToUnsetFatal is that question again for `unset`, and is a
	// separate field because ksh93 answers the two differently: `export 1x`
	// ends the script there where `unset 1x` prints the same kind of
	// complaint, returns 1 and carries on.
	//
	// Not a question about `unset` being less special than the other two — a
	// bad *option* to ksh93's `unset` is fatal, which is what makes the split
	// about the kind of failure rather than about the builtin.
	BadNameToUnsetFatal Answer

	// BadNameToReadFatal ends the script when `read` is given an operand that
	// is not a name. zsh alone: `read 1bad; echo after` prints the refusal
	// and nothing else there, and prints `after` in the other five.
	//
	// A third field rather than either of the two above, and not because the
	// panel splits differently — it does, but that alone would only make it a
	// separate *value*. `read` is not a special builtin in any shell, so no
	// dialect's rule about special builtins reaches it: dash and ksh93 stop
	// the script for `export 1x` and carry on past `read 1x`, which is the
	// same shell answering the same kind of failure two ways depending on the
	// builtin. zsh is the one that stops here, and it stops for `export` too.
	BadNameToReadFatal Answer

	// UnsetReadonlyFatal ends a non-interactive shell when `unset` is asked
	// to remove a readonly name. True in dash and zsh; bash and ksh93 report
	// it, leave the value standing and carry on with a status of 1.
	//
	// The refusal itself is not the axis. Every shell in the panel refuses,
	// keeps the value, and says so — `readonly x=1; unset x` leaves `x` as 1
	// in all six — so *that* is the core answer and only what follows the
	// refusal splits.
	//
	// A field of its own rather than BadNameToUnsetFatal, which it agrees
	// with on all four dialect defaults. They are separable because bash's
	// POSIX mode moves this one and not that one: `set -o posix` makes bash
	// 5.3 stop here, and it makes no difference to `unset 1x` — a name bash
	// 5.3 accepts in silence whatever the mode. That is the shape
	// FatalErrorStatusIsOne's note describes, where *which* errors are fatal
	// stays per-error even when two errors happen to split the panel alike.
	//
	// Like RedirectErrorOnSpecialBuiltinFatal, a dialect's field is where the
	// shell *starts* and its own posix knob moves it — see SetPosixMode. zsh
	// is the difference between the two: it carries on past a failed
	// redirection on a special builtin and stops here, under every
	// `emulate`, so the two axes cannot be one flag. ksh93 is the same
	// difference the other way round.
	//
	// bash 3.2 is fatal in neither mode, so this is bash 5's rule and not
	// bash's; the preset carries the version that measured it.
	UnsetReadonlyFatal Answer

	// DeclarationNameOperands says what may stand where `export` and
	// `readonly` want a name, beyond a plain name itself.
	//
	// zsh is the only one that takes anything more: the special parameters
	// are names to it, which is why `export -` is a complaint in three of the
	// four and not in the fourth.
	//
	// unexhibited NamesAndPositionals: UnsetNameOperands holds it, for
	// zsh. Re-measured 2026-09-12 **with the operands quoted**, because an
	// unquoted `?` is a glob in zsh and fails as `no matches found` before
	// the builtin sees it: `export '1'` and `export '12'` are refused in
	// all six columns, so nothing takes a positional where a declaration
	// wants a name (#2060).
	//
	// unexhibited AnythingIsAName: UnsetNameOperands holds it, for bash
	// 5.3. The same quoted probe: `export 'a-b'` is refused everywhere,
	// and zsh reaches no further than its special parameters — `export
	// '?'` and `export '-'` are 0 there and 1 in the other five (#2060).
	DeclarationNameOperands NameOperands

	// UnsetNameOperands is that question for `unset`, and is a separate field
	// because two dialects answer it differently from the declarations. zsh
	// answers the two with *disjoint* sets — `export ?` is fine there and
	// `unset ?` is not, while `unset 12` is fine and `export 12` is not — and
	// bash 5.3 checks a name for `export` and nothing at all for `unset`. One
	// field could not say either.
	//
	// unexhibited NamesAndSpecialParameters: DeclarationNameOperands holds
	// it, for zsh — and the disjointness is this field's reason to exist.
	// Re-measured 2026-09-12 with the operands quoted (an unquoted `?` is
	// a glob in zsh): `unset '?'` and `unset '-'` are refused in zsh 5.9.2
	// while `unset '1'` and `unset '12'` are taken, the mirror image of
	// `export`. Worth knowing about the value this preset *does* hold:
	// bash 3.2.57 refuses all four where bash 5.3.15 takes all four, so
	// AnythingIsAName is bash 5's reading and the older build's column is
	// PlainNamesOnly (#2060).
	UnsetNameOperands NameOperands

	// ReadNameOperands is that question for `read`, and is a third field
	// because `read` answers it differently again from the declarations:
	// `read 1` fills `$1` in zsh, where `export 1` is refused.
	//
	// **`read ?` is not evidence either way, and this comment used to cite
	// it.** Measured 2026-09-12: a leading `?` argument is a *prompt* in
	// that shell, so `echo hi | zsh -c "read '?'; print -r -- $REPLY"`
	// prints `hi` — the word never stood where a name belongs and the line
	// landed in REPLY. What discriminates is `read 'a-b'`, which is `not an
	// identifier` there, against `read '1'`, which is taken. The quoting is
	// load-bearing too: an unquoted `?` is a glob in zsh and fails as `no
	// matches found` before any builtin sees the word, which makes every
	// unquoted probe of this axis a measurement of globbing.
	//
	// Every shell in the panel refuses a word that is not a name — this is
	// not the axis, and the refusal itself is the core's (#1440). What splits
	// them is only how far the set reaches past a plain name.
	//
	// unexhibited NamesAndSpecialParameters: DeclarationNameOperands holds
	// it, for zsh's `export`. `read` does not reach it: measured
	// 2026-09-12, `read 'a-b'` is `not an identifier` in zsh 5.9.2
	// (#2060).
	//
	// unexhibited AnythingIsAName: UnsetNameOperands holds it, for bash
	// 5.3. `read` reaches positionals and stops: `echo hello | read '1'`
	// fills `$1` in zsh 5.9.2 and is refused in the other five columns,
	// measured 2026-09-12 (#2060).
	ReadNameOperands NameOperands

	// DeclarationTakesASubscript accepts `export a[0]` and `readonly a[0]`,
	// naming an element rather than a variable. ksh93 does; bash and dash
	// refuse it in the words they give any other bad name.
	//
	// A separate question from the name strictness above, because the answer
	// is per builtin: bash refuses it here and takes it for `unset`, and the
	// two builtins sit on different strictnesses in every shell, so no rule
	// over that strictness gives all four.
	DeclarationTakesASubscript Answer

	// TypesetTakesASubscript is that same question for `typeset`, `declare`
	// and `integer`, and it is a third field because the answer is per
	// builtin here as well: measured, bash refuses `export a[1]=v` as a bad
	// name and *takes* `typeset a[1]=v`, creating the element. ksh93 and zsh
	// take both. So the declaration builtins are not one strictness with two
	// spellings, and reading `typeset a[1]=v` through
	// DeclarationTakesASubscript would have made bash start refusing a line
	// it has always accepted.
	TypesetTakesASubscript Answer

	// UnsetTakesASubscript is the same question asked of `unset`, where the
	// answers are not the same: bash, ksh93 and zsh take it and dash refuses
	// it.
	UnsetTakesASubscript Answer

	// BadNameDeclaresTheOperandsAfterIt keeps declaring past an operand the
	// builtin refused, where the refusal is fatal.
	//
	// Measured 2026-09-07 with the bad name first, in the middle and last,
	// over `export`, `readonly`, `typeset` and `unset`, reading the names
	// back from an EXIT trap because the fatality otherwise hides the answer:
	// ksh93 and zsh declare every well-formed operand wherever the bad one
	// stood, and dash and bash-as-`sh` declare only the ones in front of it.
	//
	// Asked only on the fatal path. bash proper reports each bad operand and
	// carries on, so its whole list is declared by the loop rather than by
	// this — the answer recorded for it is the one bash-as-`sh` gives, which
	// is the same shell with the fatality turned on.
	BadNameDeclaresTheOperandsAfterIt Answer

	// SubscriptedOperandTakesTheIntegerAttribute lets `typeset -i a[1]=0x10`
	// give the array the integer attribute and write the element with it.
	//
	// Measured 2026-09-07: bash 5.3 and ksh93u+ both store 16 and list the
	// array back with the attribute on it; zsh 5.9.2 refuses the operand
	// outright, because an element is not a name and the attribute belongs to
	// the name. Asked only when the letter was written, so the plain
	// declaration needs no answer from anyone.
	SubscriptedOperandTakesTheIntegerAttribute Answer

	// SubscriptedOperandTakesALocalDeclaration lets `typeset a[1]=v` inside a
	// function make the array local and write the element into the local one.
	//
	// The same split, and a separate field because it is a different thing
	// being done to the variable: bash 5.3 declares a local array holding the
	// element and the caller's array comes back on return; ksh93u+ has no
	// scope for it to take and writes the caller's; zsh 5.9.2 refuses. A
	// dialect could answer one of the two and not the other, and folding them
	// would give zsh's refusal to whichever the other shell was measured for.
	//
	// Asked only where there is a scope to take, so a declaration at the top
	// level never reaches it.
	SubscriptedOperandTakesALocalDeclaration Answer

	// ReadonlyElement is what a declaration does when it would freeze the
	// array whose element its operand names — `readonly a[1]=v` and
	// `typeset -r a[1]=v`.
	//
	// Three answers rather than two, which is why it is not an Answer:
	// ksh93u+ writes the element and freezes the array over it, zsh 5.9.2
	// refuses the operand, and bash 5.3 does a third thing — it creates the
	// array frozen and *empty* and then reports the element write it has just
	// made impossible, at status 0. The third is measured and recorded and
	// deliberately not implemented here; bash reaches this by `typeset -r`
	// alone, since it refuses `readonly a[1]=v` as a bad name long before.
	ReadonlyElement ReadonlyElementPolicy

	// BadSubscriptToUnsetFatal ends the script when an `unset` operand's
	// subscript will not evaluate. True in bash, where a bad expression ends
	// it wherever one is written; false in ksh93 and zsh, which leave a failed
	// builtin behind and go on. dash has no subscript to evaluate.
	//
	// Asked only for an operand whose subscript actually failed, so `unset
	// a[1]` needs no answer from anyone.
	BadSubscriptToUnsetFatal Answer

	// UnsetSubscriptOnAScalarIsAnError refuses `unset "a[1]"` where `a`
	// holds a string, rather than leaving the name alone without a word.
	// bash says `unset: a: not an array variable` and fails; ksh93 says
	// nothing and succeeds.
	//
	// Reached only through the *element* reading of a subscripted name — the
	// shell that reads `a[1]` as a character of the string takes that
	// character out and never gets here, so this is ScalarSubscriptIsACharacter's
	// consequence rather than a second decision about the same shape.
	//
	// Asked only where the subscript names *no* element. A scalar is the one
	// element at the base, so `unset "a[0]"` where the base is 0 takes the
	// whole name away in both shells and asks nothing; and a name holding
	// nothing at all has no element for any subscript to name and is left
	// alone everywhere, which is why `unset "b[0]"` on an unset `b` is quiet
	// in all four.
	//
	// Nor is it about arrays with a gap: `a=(x y z); unset "a[9]"` is silent
	// and succeeds in every shell measured. What the refusing shell objects
	// to is the *name* not being an array, which is what its wording says.
	//
	// The preset is no. POSIX has `unset` remove what is there and say
	// nothing about what is not — `unset nosuchname` is a success everywhere
	// — and the silent reading is that sentence applied to a subscript.
	//
	// bash 3.2 refuses the base subscript too, so a corpus case here splits
	// the `bash` and `bash32` columns on purpose: that build reads `${a[0]}`
	// as the whole string for an *expansion* and still refuses to unset
	// through it, which is a disagreement within one shell rather than
	// between two.
	UnsetSubscriptOnAScalarIsAnError Answer

	// UnsetArraySpan is what `unset` does to the elements a subscript names,
	// and the panel gives three answers rather than two — see
	// UnsetArraySpanPolicy.
	//
	// One field for `unset a[@]` and for `unset a[3]`, because in the shell
	// that parts from the rest they are one rule: `unset` of a span replaces
	// that span with a single empty element, so `a[3]` is a span of one and
	// comes back blank in place while `[@]` is the whole array and comes back
	// as one empty element. Measured across spans of one, two and all — see
	// docs/spec/measurements.md.
	UnsetArraySpan UnsetArraySpanPolicy
	// EmptyArithSubscript is what `a[]` means where an expression wants a
	// value — the shape `a[$w]` takes when `$w` is empty, because the
	// parameters go in before the expression is read. See
	// EmptyArithSubscriptPolicy.
	EmptyArithSubscript EmptyArithSubscriptPolicy
	// PositionalListWithNoneIsSet calls `$@` — and `$*` — a **set** parameter
	// when there are no positional parameters at all. `No` says the list is
	// unset until something is in it, so a colon-less conditional fires.
	//
	// Measured 2026-09-11 and again 2026-09-12, `-c` and a script file,
	// after `set --`:
	//
	//	                       dash, zsh 5.9.2   bash 5.3/as-sh/3.2, ksh93u+
	//	"${@-word}"            (empty)           word
	//	"${@+word}"            word              (empty)
	//	"${*-word}"            (empty)           word
	//	"${@?}"                (empty), 0        `@: parameter not set`
	//	${@=abc}               (empty), 0        the operator fires, and is
	//	                                         refused: `$@: cannot assign
	//	                                         in this way` in bash,
	//	                                         `${@=abc}: bad substitution`
	//	                                         in ksh93
	//
	// Two columns say set and four say unset, and `$*` splits exactly as
	// `$@` does, so one axis answers the family rather than one operator.
	// The refusal in the last row is not this question — #1541 put that in
	// and every dialect reaches it the moment the operator fires — this is
	// the step before it, which decides whether it fires at all.
	//
	// The **colon** form is unanimous and asks nothing: `${@:-word}` is
	// `word` and `${@:+word}` is empty in all six, because an empty value
	// fires the test whichever way the set-ness reads. `${1-word}` is
	// `word` everywhere too, so this is about the list and not about a
	// positional parameter that is not there.
	//
	// Asked at the disagreement: a colon-less conditional, on `$@` or `$*`
	// itself, with no positional parameters. With any parameter at all every
	// column calls the list set, and a subscripted name that happens to be
	// spelled `@` is an array and answers elsewhere (#1941).
	PositionalListWithNoneIsSet Answer

	// EmptyAssociativeKeyIsAnError refuses to *store* under a key that is
	// empty once the subscript has been read — `typeset -A m; m[""]=4`, and
	// `m[$w]=4` with an empty `$w` beside it.
	//
	// Measured 2026-09-12 from a script file, with the name declared:
	//
	//	                   m[""]=4    m[$w]=4    m[ ]=7
	//	bash 5.3.15        refused    refused    stored under one space
	//	ksh93u+            stored     stored     stored
	//	zsh 5.9.2          stored     stored     a bad pattern
	//
	// bash names the subscript as it was written — `m[""]: bad array
	// subscript` and `m[$w]: bad array subscript` — leaves the table
	// untouched, reports 1 and carries on; the same build under argv[0] `sh`
	// ends the script instead. The blank column is the control that says
	// this is emptiness and not whitespace: one space is a key everywhere
	// that reads the brackets as a key at all.
	//
	// The two columns that store are not storing the same key, and that is
	// SubscriptIsAQuotingContext rather than this axis: ksh93 reads the
	// quotes off and holds the empty key, zsh keeps them and holds a
	// two-character one, so `typeset -A m; m[""]=4; m[$w]=4` leaves ksh93
	// with one element and zsh with two. Both are measured and both are
	// right for their column.
	//
	// Asked only where an association is about to be stored under a key that
	// came out empty. A key with anything in it asks nothing, an indexed name
	// reads its subscript as an expression and asks EmptySubscriptText…
	// instead, and *reading* an empty key is a third question again — bash
	// writes `m: bad array subscript` there, with a different subject, and
	// that is #1972 (#1938).
	EmptyAssociativeKeyIsAnError Answer

	// EmptyParamSubscriptIsAnError refuses `${a[]}` — a subscript written
	// with nothing at all between the brackets — where a *parameter
	// expansion* reads it. The same text one level over from
	// EmptyArithSubscript, and a different answer.
	//
	// Measured 2026-09-11, `-c`, with `a=(5 6 7)` so no other axis answers
	// first:
	//
	//	bash 5.3 / as-sh / 3.2   `[${a[]}]: bad substitution`, and the shell ends
	//	dash                     `Bad substitution`, and the shell ends
	//	ksh93u+                  `5` — the element the empty expression names
	//	zsh 5.9.2                `invalid subscript`, and the shell ends
	//
	// Five columns refuse and one reads it, which is what makes this an axis
	// and not a diagnostic: ksh93 takes the brackets as holding an expression
	// that happens to be empty, so `${a[]}` is element zero, `${s[]}` on a
	// scalar is the scalar, and `${m[]}` is the value under the empty key.
	// That is the same reading EmptyArithSubscriptIsTheEmptyExpression
	// records for the same shell one construct over.
	//
	// **It is the *written* brackets and not an empty subscript that arrived
	// through one.** `w=; ${m[$w]}` is `[]` at status 0 in zsh, because the
	// subscript text is `$w` and the expansion happens inside the subscript
	// rather than before it — which is the opposite of the arithmetic case,
	// where the parameters go in first and `m[$w]` really does become `m[]`.
	// So the question is asked of the node and answered before anything is
	// expanded.
	//
	// The operator makes no difference in any column: `${#a[]}`, `${a[]-d}`,
	// `${a[]:-d}`, `${a[]/x/y}` and `${a[]%x}` all answer as the bare shape
	// does, which is why this is asked where the subscript is read rather
	// than once per operator.
	//
	// dash is in the table for completeness and does not answer it: no
	// subscript reaches a parameter expansion there at all — `${a[0]}` is
	// `Bad substitution` too — so the grammar has already refused before this
	// could be asked.
	//
	// The wording is Diagnostics.EmptyParamSubscript, which is empty for
	// every column whose sentence is its ordinary bad-substitution one.
	EmptyParamSubscriptIsAnError Answer

	// ArithWholeArraySubscriptIsTheSlice reads a `*` or `@` subscript inside
	// an arithmetic expression as the *slice* the same subscript takes in an
	// expansion — the elements joined on the first character of IFS — rather
	// than as an ordinary subscript.
	//
	// Measured 2026-09-11, `-c`:
	//
	//	probe                                    bash 5.3   ksh93u+   zsh 5.9.2
	//	typeset -A m; m[k]=9; $(( m[*] ))        0          0         9
	//	typeset -A m; m[k]=9; $(( m[@] ))        0          0         9
	//	a=(3); $(( a[*] + 1 ))                   1, and `a[*]: bad array subscript`   syntax error   4
	//	a=(3 4 5); $(( a[*] ))                   0, reported   syntax error   `operator expected at `4 5''
	//
	// So one column expands the slice first and the other two read the
	// brackets as arithmetic, fail to make a subscript of `*`, and answer
	// zero. The expansion route already agrees everywhere — `"${m[*]}"` is
	// `9` in all three — which is what says this is a missing *reading* here
	// rather than a missing feature.
	//
	// The joined text is then read as an expression, not as a numeral, which
	// is the same reading an element's value gets: `a=(1+1); $(( a[*] * 3 ))`
	// is 6 where `$(( a[1] * 3 ))` is 6. A slice of more than one element is
	// therefore usually a failure rather than a number, and an empty array
	// joins to nothing and is zero.
	//
	// `@` joins on IFS here exactly as `*` does — measured, `a=(3 4);
	// IFS=:` gives both spellings `3:4` to read — so this is not the
	// unquoted-`@` question, which is about field splitting and has none to
	// be about inside an expression.
	//
	// Asked **before** the association is consulted, because the key `*` is
	// precisely what the other answer makes of the same text: a table read
	// first would answer the key and the two readings would collapse into
	// one. That is the ordering #1875 established for a subscript that is
	// not a number, with this question inserted at its front.
	//
	// The two columns that answer no *report* the subscript on an indexed
	// name — `a[*]: bad array subscript` in bash, a syntax error in ksh93 —
	// and are silent on an associative one. That is a diagnostic of its own
	// and is #1978 rather than this axis, which is about the value.
	//
	// The joined text is read as an expression by the same route an
	// element's value takes, so it inherits that route's own gap: a value
	// that is an expression rather than a numeral is refused here where
	// every column re-reads it (#1977). `$(( a[*] ))` over `a=(1+1)` is 6
	// there and a refusal here for that reason and not for this one.
	ArithWholeArraySubscriptIsTheSlice Answer

	// EmptySubscriptTextIsAMathError refuses a subscript whose text is
	// *empty or blank once it has been expanded* — `${a[$w]}` with an empty
	// `$w`, and `${a[ ]}` beside it — where an expression reads it.
	//
	// Measured 2026-09-11, `-c`, with `a=(5 6 7)` and `w=`:
	//
	//	probe             bash 5.3   ksh93u+   zsh 5.9.2
	//	${a[$w]}          `5`, 0     `5`, 0    bad math expression: empty string, and the shell ends
	//	${a[ ]}           `5`, 0     `5`, 0    bad math expression: operand expected at end of string
	//	${a[$w]} on a scalar  —      —         the same as the first row
	//	a[$w]=z           assigns    assigns   the same as the first row
	//	${#a[$w]}         `1`, 0     `1`, 0    the same as the first row
	//
	// So two columns read the empty text as the expression that is zero and
	// one will not read it at all. dash has no subscript in a parameter
	// expansion to ask about.
	//
	// **It is the expanded text and not the written brackets.** `${a[]}` —
	// nothing between them as written — is EmptyParamSubscriptIsAnError, and
	// the shell that refuses both gives them *different* sentences: `invalid
	// subscript` for the written pair, the arithmetic reader's complaint
	// here. One axis for both would have had to give one of those answers to
	// the other.
	//
	// **And it is the subscript and not every number a parameter expansion
	// reads.** A substring's offset is the neighbor that says so: measured,
	// `x=abcdef; ${x:$w:2}` with the same empty `$w` is `ab` in every column,
	// the refusing one included. So the question is asked where a subscript
	// is evaluated and not in the arithmetic the two sites share.
	//
	// **Nor is it an association's subscript**, which is a key and never an
	// expression: `typeset -A m; ${m[$w]}` is the empty string at status 0 in
	// every column that has the attribute, so the key path is reached before
	// this is asked — the same ordering arithElement keeps for the same
	// reason (#1875). bash writes `m: bad array subscript` beside that empty
	// string and still answers it, which is a diagnostic of its own and is
	// #1972 rather than this axis.
	//
	// The arithmetic site is a different partition of the panel again and
	// has axes of its own: EmptyArithSubscript for `$(( a[] ))` and
	// BlankArithSubscriptIsTheEmptyExpression for the blank one.
	//
	// The wording of the empty half is Diagnostics.EmptySubscriptTextExpanded;
	// the blank half is the expression running out and is worded by
	// Diagnostics.ArithExpressionRanOut, which already words `$(( a[ ] ))`.
	EmptySubscriptTextIsAMathError Answer

	// BlankArithSubscriptIsTheEmptyExpression reads a subscript holding
	// whitespace and nothing else — `$(( a[ ] ))` — as the blank expression,
	// which is zero, so the operand is the *element that subscript names*
	// rather than a flat zero. Yes in bash and ksh93; No in zsh, where the
	// subscript's reader wants a value and reports that it reached the end of
	// the text instead.
	//
	// A separate axis from EmptyArithSubscript, and asked one text further
	// along, because bash answers the two apart: `a[]` is `bad array
	// subscript` and a flat zero there, while `a[ ]` is silently element
	// zero. One axis worded for both would have had to give one of those
	// answers to the other.
	//
	// Asked at the subscript and not at the expression, which is where the
	// panel actually splits: `$((   ))` with nothing but spaces in it is zero
	// in all three of those shells, so an axis placed on the blank
	// *expression* would move a row the panel agrees about.
	//
	// Not reached on an associative name, where a subscript is a key and
	// never an expression — `$(( m[ ] ))` looks up the one-space key — and
	// not reached on a name that is not set where
	// ArithSubscriptSkippedWhenNameUnset answers first. See #1762.
	//
	// Measured 2026-09-10, `a=(1 2 3)` then `echo $(( a[ ] ))`:
	//
	//	bash 5.3, bash 3.2   1, the first element, silently
	//	ksh93u+              1, the same
	//	zsh 5.9.2            bad math expression: operand expected at end
	//	                     of string, and no value at all
	//
	// The same split, unchanged, with the subscript standing as an assignment
	// target: `(( a[ ] = 9 ))` writes element zero in bash and ksh93 and is
	// refused with that sentence in zsh.
	BlankArithSubscriptIsTheEmptyExpression Answer

	// SubscriptedArrayLiteral is what `a[i]=(p q)` does — an array literal
	// standing where one element's value goes. The panel disagrees about it
	// completely; see SubscriptedArrayLiteralPolicy.
	SubscriptedArrayLiteral SubscriptedArrayLiteralPolicy
}

// StartupFileOptions are the invocation options that change which startup
// files a shell reads: the escape hatches from a startup file that is wrong.
//
// Each field holds the spellings the dialect accepts, whitespace-separated and
// exactly as they are written on a command line — `--norc`, `-f`. A
// single-dash entry of one letter also matches inside a bundle, so `-if` is
// `-i` and `-f`; a double-dash entry matches a whole word and nothing else.
// Empty means a shell with no such option, and the zero value is a shell with
// none at all.
//
// Strings rather than slices, for the reason LoginStartupFiles is one: a slice
// reached from Semantics makes the whole vector uncomparable, and an option
// spelling has no whitespace in it to lose.
//
// Measured 2026-09-05 across the panel with a scratch home directory. bash has
// three of the four and spells them long; zsh has only the first and spells it
// both ways; dash and ksh93 have none, so a startup file that breaks them is
// escaped by moving the file. The shell that has no escape is the reason the
// other two are worth carrying.
type StartupFileOptions struct {
	// SuppressAll names the options that suppress every startup file. zsh's
	// `-f` and `--no-rcs`, and measured they mean *every* one: `zsh -f -l -i`
	// reads no `.zshenv`, no `.zprofile`, no `.zshrc` and no `.zlogin`.
	//
	// It is not the POSIX `-f`, which turns globbing off. zsh gives the
	// letter this meaning instead — measured, `zsh -f -c 'echo /etc/pas*'`
	// still expands the pattern — which is why the letter is a per-dialect
	// spelling here rather than a set option every shell shares.
	SuppressAll string

	// Login names the options that make this a login shell whatever argv[0]
	// said. `-l` in all four, and `--login` in three of them — dash refuses
	// the long spelling outright, with `Illegal option --` at status 2.
	//
	// It is not simply a second way to set the same bit, and that is why it
	// belongs here rather than beside LoginShell: the option reads the
	// profile **even with a script to run**, where login-ness inferred from
	// argv[0] does not in every dialect. Measured 2026-09-05: `bash --login
	// -c cmd` reads its profile and `exec -a -bash bash -c cmd` reads
	// nothing, so an explicit option makes the panel unanimous where
	// LoginProfileWhenNonInteractive says it is not.
	//
	// Without it a login shell can only be started by exec'ing with a
	// dashed argv[0], which is what `login` does and what a person at a
	// terminal cannot.
	Login string

	// SuppressLogin names the options that suppress the login profile and
	// leave the rest. bash's `--noprofile`, and nobody else's.
	//
	// It beats Login above, which is measured: `bash --noprofile --login -i`
	// reads no profile.
	SuppressLogin string

	// SuppressInteractive names the options that suppress the interactive
	// file and leave the rest. bash's `--norc`, and nobody else's.
	//
	// It suppresses the file the shell reads *of its own name* and not
	// `$ENV`: measured, `bash --posix --norc -i` still reads `$ENV`, because
	// in that mode the standard's file is the one it was going to read.
	SuppressInteractive string

	// NameInteractive names the options whose operand — the next word — is
	// read in place of the interactive file. bash's `--rcfile` and its
	// synonym `--init-file`.
	//
	// It replaces rather than adds, and it loses to everything that suppresses
	// the file: measured, `bash --norc --rcfile f -i` reads neither, and so
	// does `bash --rcfile f -l -i`, where a login shell was not going to read
	// an interactive file at all.
	NameInteractive string
}

// VersionOption is what a shell does when its invocation asks for a version.
//
// Measured 2026-09-11 with `env -i <shell> --version`:
//
//	                  writes                                       on      exit
//	bash 5.3.15       GNU bash, version 5.3.15(1)-release (…)      stdout  0
//	bash 3.2.57       GNU bash, version 3.2.57(1)-release (…)      stdout  0
//	zsh 5.9.2         zsh 5.9.2 (…)                                stdout  0
//	ksh93u+           "  version         sh (AT&T Research) …"     stderr  2
//	dash              /bin/dash: 0: Illegal option --              stderr  2
//
// Three facts, and none of them follows from the others: whether the shell
// knows the option at all, which stream the answer goes to, and what it exits.
// ksh93 is the reason all three are fields — it answers with its version and
// still exits a failure, because the option reaches its generic option reader
// rather than a case of its own.
//
// A dialect that names no spelling refuses the word the way any unknown long
// option is refused, which is what dash does.
//
// Measured too: the answer *ends* the invocation. `--version -c 'echo hi'`
// prints the version and does not run the command in bash, zsh and ksh93
// alike, and a second `--version` changes nothing.
type VersionOption struct {
	// Spellings names the option, whitespace-separated and written exactly
	// as a command line writes it — `--version`. Empty means the shell has
	// no such option.
	//
	// A string rather than a slice for the reason StartupFileOptions' fields
	// are strings: a slice reached from Semantics makes the whole vector
	// uncomparable, and an option spelling has no whitespace to lose.
	Spellings string

	// Text is the line the shell writes, without its newline. It is the
	// dialect's own — the version *this* shell implements, not the one a
	// panel member on this machine reports.
	Text string

	// ToStandardError writes the answer on standard error rather than
	// standard output, which one shell in the panel does.
	ToStandardError bool

	// Status is what the shell exits after answering. Zero for the three
	// that treat the option as a request; ksh93 exits 2.
	Status int
}

// NameOperands is what a builtin takes where it wants a name.
type NameOperands int

const (
	// NameOperandsUnspecified is no answer, and is refused like any other.
	NameOperandsUnspecified NameOperands = iota
	// PlainNamesOnly takes a name and nothing else: bash, dash and ksh93,
	// for all three builtins.
	PlainNamesOnly
	// NamesAndSpecialParameters also takes `?`, `*`, `@`, `#`, `!`, `-`, `$`
	// and `0`: zsh's `export` and `readonly`. Not the other digits — `export
	// 0` is quiet there and `export 1` is "not an identifier", which is the
	// difference between a special parameter and a positional one.
	NamesAndSpecialParameters
	// NamesAndPositionals also takes any all-digit operand: zsh's `unset`,
	// where `unset 12` is quiet. `0` falls in here too, so both of zsh's
	// answers take it and they agree on nothing else.
	NamesAndPositionals
	// AnythingIsAName checks nothing at all: bash 5.3's bare `unset`, which
	// is quiet about `unset 1x`, `unset "a b"` and `unset -- -` alike while
	// its `export` refuses every one of them.
	//
	// A change within bash rather than a difference between shells — bash
	// 3.2 refuses all three — so the `bash32` and `bash` columns of a corpus
	// case here disagree on purpose.
	AnythingIsAName
)

func (n NameOperands) String() string {
	switch n {
	case PlainNamesOnly:
		return "PlainNamesOnly"
	case NamesAndSpecialParameters:
		return "NamesAndSpecialParameters"
	case NamesAndPositionals:
		return "NamesAndPositionals"
	case AnythingIsAName:
		return "AnythingIsAName"
	}
	return "NameOperandsUnspecified"
}

// ReadPromptOperand is what `read` makes of a `?` in its first operand.
type ReadPromptOperand int

const (
	// ReadPromptOperandUnspecified is no answer, and is refused like any
	// other.
	ReadPromptOperandUnspecified ReadPromptOperand = iota
	// ReadOperandIsAllName reads the whole word as the name: bash and dash,
	// where `read "v?p"` is the bad name `v?p` and nothing else.
	ReadOperandIsAllName
	// ReadPromptNeedsANameBeforeIt takes the part in front of the `?` as the
	// name and the rest as a prompt, and still wants a name there: ksh93,
	// where `read "?p"` is refused for the empty name it leaves.
	ReadPromptNeedsANameBeforeIt
	// ReadPromptAloneNamesTheDefault is the same split with nothing in front
	// of the `?` meaning the default name: zsh, where `read "?Press enter"`
	// prompts and reads into REPLY. Measured 2026-09-07 — it is the shape
	// the idiom is usually written in, and it is the one ksh93 refuses.
	ReadPromptAloneNamesTheDefault
)

func (p ReadPromptOperand) String() string {
	switch p {
	case ReadOperandIsAllName:
		return "ReadOperandIsAllName"
	case ReadPromptNeedsANameBeforeIt:
		return "ReadPromptNeedsANameBeforeIt"
	case ReadPromptAloneNamesTheDefault:
		return "ReadPromptAloneNamesTheDefault"
	}
	return "ReadPromptOperandUnspecified"
}

// SelectMenuLayout is how a shell draws a `select` menu. The engines differ
// enough that the same nine items are nine lines in two shells and one line in
// the third, so this is a named choice rather than a flag.
type SelectMenuLayout int

const (
	// SelectMenuVertical is one item per line, always. ksh93's, whose column
	// mode is reached on the terminal's height rather than its width.
	SelectMenuVertical SelectMenuLayout = iota
	// SelectMenuVerticalThenColumns is bash's: one item per line while the
	// list would fit on one line, and tab-separated columns once it would
	// not — which is the opposite way round from how it sounds.
	SelectMenuVerticalThenColumns
	// SelectMenuColumns is zsh's: always packed into columns padded with
	// spaces, so even three items share one line.
	SelectMenuColumns
)

// PosixSemantics is what the specification requires, which is not what any
// shell does in full — it is the right target for a portability check and the
// wrong one for a runtime.
func PosixSemantics() Semantics {
	return Semantics{
		SplitParamExpansion: Yes,
		// 2.11 has the shell read its input and execute commands as it goes,
		// and 2.14's `eval` "shall be read and executed by the shell" in the
		// same way. So text that will not parse further stops the reading
		// rather than unwinding what has already run, and dash — the shell in
		// the panel that targets this text — complies for both. zsh's `eval`
		// and ksh93 are the departures.
		EvalRunsWhatItParsed:        Yes,
		SourcedFileRunsWhatItParsed: Yes,
		SplitCommandSubstitution:    Yes,
		// 2.7.2 puts noclobber on `>` and says nothing about `>>`, so
		// appending still creates. Four of the panel comply; zsh departs.
		NoclobberBlocksAppendCreate: No,
		// A null field from an unquoted expansion is removed, which is the
		// reading that takes the elements one at a time — and it is dash's,
		// the shell in the panel that targets this text. bash's join is the
		// departure from it.
		UnquotedListJoinsOnIFS: No,
		// POSIX makes an unquoted `$@` in a context that does not split
		// behave as `$*` does, which is the join on IFS; dash, the shell in
		// the panel that targets this text, complies. bash and ksh93 are the
		// departure.
		UnsplitAtListJoinsOnIFS: Yes,
		// 2.6.5 spells the tail out — "once the input is empty, the
		// candidate shall become an output field if and only if it is not
		// empty" — so a trailing separator is absorbed and opens nothing.
		// dash, the shell in the panel that targets this text, complies, and
		// so do bash and ksh93; zsh is the departure.
		TrailingSeparatorEndsAField:              No,
		GlobExpansionResults:                     Yes,
		GlobNoMatchIsError:                       No,
		AssignmentPrefixPersistsOnSpecialBuiltin: Yes,
		EchoInterpretsEscapes:                    No,
		// POSIX has `echo` and `printf` exit greater than zero when "an
		// error occurred", and a write that went nowhere is one; dash
		// complies. zsh is the holdout, keeping status 0.
		BuiltinWriteErrorFailsTheCommand: Yes,
		// The XSI echo: -n alone, no \x, no \e. The letters the dialects
		// add are theirs to add.
		EchoOptions:                 "n",
		EchoExpandsHexEscapes:       No,
		EchoExpandsUnicodeEscapes:   No,
		EchoEmptyHexDigitRunIsNul:   No,
		EchoExpandsEscEscape:        No,
		EchoExpandsCapitalEscEscape: No,
		// The POSIX read: -r alone. The counts, delimiters and descriptors
		// the dialects add are theirs to add, and the two count axes are
		// unreachable without the letters that raise them.
		ReadOptions: "r",
		// POSIX gives `unset` both letters and no others.
		UnsetOptions: "vf",
		// The POSIX jobs: -l and -p, and `-p` means the process ids alone.
		// The state filters and the rest are the dialects' additions, and
		// the two axes their letters raise are unreachable without them.
		JobsOptions:             "lp",
		JobsPidsOnlyOption:      Yes,
		LengthOfSpecialIsCount:  Yes,
		ArithLeadingZeroIsOctal: Yes,
		// One reader: the value a name holds goes through the same octal
		// rule the literal does, so `k=010; $((k))` is eight. ksh93 is the
		// one shell whose two readers part.
		ArithStoredValueReadsALeadingZeroAsDecimal: No,
		// One reader for `let` too: the standard has no `let` and no integer
		// attribute, so the preset keeps the arithmetic it does describe.
		LetReadsALeadingZeroAsDecimal:         No,
		ArithmeticAssignmentDeclaresAnInteger: No,
		// dash is the panel's POSIX-faithful member and the only one
		// exiting 2, so the POSIX preset follows it. The standard itself
		// requires only "greater than zero", which decides nothing.
		FatalErrorStatusIsOne:  No,
		ArithNameValueRecurses: No,
		// ArithRecursedNameMustBeSet is deliberately left unanswered: with
		// no recursion there is no name below the top for it to be asked
		// about, so an answer here would be a value nothing can measure.
		// A login shell reads ~/.profile whether or not it is going to
		// prompt: dash, ksh93 and zsh, with bash the holdout. POSIX names
		// ~/.profile as the file a login shell reads and does not make it
		// conditional on being interactive, so the standard and the
		// majority agree here.
		LoginProfileWhenNonInteractive: true,
		// And the file it reads: the standard names ~/.profile, which is
		// dash's and ksh93's name for it too. One entry rather than a
		// fallback chain — bash is the only shell in the panel that tries
		// more than one name.
		//
		// InteractiveStartupFile is deliberately left empty here, which is
		// not an omission: the standard's interactive file is `$ENV`, and an
		// empty name is how a dialect says so.
		LoginStartupFiles: ".profile",
		// The four brace-range axes are left unanswered: a brace that
		// never expands never asks them.
		BraceExpansion:                 No,
		BracketCaretNegates:            No,
		ExitTrapIsFunctionLocal:        No,
		FunctionLocalTraps:             TrapsSurviveTheFunction,
		GetoptsPositionIsFunctionLocal: No,
		// dash is the panel's POSIX-faithful member and it loses the
		// intra-word half at the return, so the preset that follows it
		// loses it too. The standard has nothing to say — `local` is not
		// in it — so the measured member decides.
		GetoptsLocalOptindRestoresTheCursor: No,
		SignalHandlerSeesEarlierStatus:      No,
		// POSIX says a bare `exit` reports the status of the last command,
		// and in an EXIT trap it names the value `$?` had when the trap was
		// entered — which is what three of the four do.
		ExitInTrapReportsEarlierStatus: Yes,
		UnsetPositionalIsAllowed:       No,
		TraceShowsItsOwnDisabling:      Yes,
		TraceAssignmentsSeparately:     No,
		StatusArgument:                 StatusArgStrict,
		EqualsExpansion:                No,
		// POSIX gives `test` one spelling of string equality, so `==` is not
		// an operator; the three shells that accept it added it.
		TestAcceptsDoubleEqual: No,
		// POSIX requires only "greater than 128" for a command killed by a
		// signal, which decides nothing; three of the four use 128.
		SignalDeathStatusIsTwoFiftySix: No,
		// POSIX gives printf no options at all, so there is nothing to
		// assign with and a leading `-` word is not one.
		PrintfAssignsWithV:         No,
		PrintfRejectsUnknownOption: Yes,
		// POSIX shows the mask in a form that can be read back; three of the
		// four write four octal digits.
		UmaskPrintsFourDigits: Yes,
		// POSIX says setting the mask writes nothing.
		UmaskSetWithSPrints: No,
		// POSIX counts these in 512-byte blocks, and names neither -m nor -u.
		UlimitBlockIsKilobyte: No,
		UlimitHasResidentSet:  No,
		UlimitHasProcessCount: No,
		// POSIX sets both when neither is named.
		UlimitSetsBothLimits: Yes,
		// POSIX defines a pipeline's status as its last command's, and
		// offers nothing to change it.
		PipefailOption: No,
		// POSIX spells `export` with one option, `-p`. So the standard's
		// answer about `-n` is that there is no such letter, and a preset
		// that said nothing would refuse `export -n` as an unchosen axis
		// rather than as the unknown option the standard makes it.
		ExportTakesTheAttributeOff: No,
		// POSIX names the noglob letter itself: `set -f`, reported in `$-`
		// as `f`. Only zsh answers otherwise.
		NoglobLetterIsF: Yes,
		// POSIX defines `$-` as the option flags specified on invocation,
		// and `-c` is one of them; `-s` is not, when it was not written.
		// The preset takes the text rather than a vote, which is the rule
		// everywhere here, and the panel splits two against two anyway.
		CommandStringShowsCInDollarDash: Yes,
		CommandStringShowsSInDollarDash: No,
		ArithInvalidOctalDigitIsError:   Yes,
		RegexQuotingMakesLiteral:        No,
		ProcessSubstitutionInCondition:  No,
		// POSIX has no process substitution, so there is no text to read
		// here either: the base takes the answer every panel member but one
		// gives, which is that the body reads the input of the command the
		// word stands in like any other child of it.
		ProcessSubstitutionBodyReadsTheShellsInput: No,
		// POSIX has no `[[ ]]` to fail in, so this is the substrate's floor
		// rather than a reading of the text: an error is diagnosed and the
		// shell goes on, which is what POSIX asks of every failure that is
		// not a special builtin's.
		ConditionArithmeticErrorIsFatal: No,
		// The standard has no `(( ))` at all, so nothing here is POSIX's to
		// say; 1 is what the shells that do have it say, bar one.
		ArithCommandErrorStatusIsTwo: No,
		// POSIX has no `(( ))` at all — it is an extension every shell but
		// dash carries — so there is no text to read here and the base takes
		// the answer three of the four give: the status is left for the next
		// line, which runs.
		ArithCommandErrorIsFatal: No,
		// POSIX has no `let` either, and the answer three of the four give is
		// that a failed expression leaves nothing behind: the status is 1.
		LetKeepsTheValueBeforeAnIllegalByte: No,
		LastPipelineElementInCurrentShell:   No,
		ShiftPastEndFatal:                   Yes,
		// The standard describes one refusal and says nothing about a
		// second, so the preset stops at the first the way three of the
		// panel do.
		SetReportsEveryBadOption:  No,
		ReadonlyReassignmentFatal: Yes,
		// XCU makes an assignment to a readonly name an error, and an error
		// in a special builtin or in an assignment ends a non-interactive
		// shell — which is what a prefix assignment's refusal is. dash is
		// the panel member that follows it here, and it follows it for every
		// kind of command. The command's fate is not asked once the script
		// is over, so PrefixRefusalCostsTheCommand stays unanswered (#1219).
		PrefixToARegularBuiltinIsRefused: Yes,
		PrefixRefusalFatality:            PrefixRefusalAlwaysFatal,
		// XCU makes an expansion error fatal to a non-interactive shell, so
		// the standard's preset does not survive one.
		FailedExpansionAbandonsTheLine:         No,
		ReadonlyReassignmentByDeclarationFatal: Yes,
		ArrayBaseIsZero:                        Yes,
		// The standard has no subscript, and the nearest reading it does
		// have is its arithmetic: a comma there is the operator whose value
		// is its right operand, and a string is not a sequence a subscript
		// reaches into. Both are also what every panel member but one does.
		SubscriptCommaIsARange:      No,
		ScalarSubscriptIsACharacter: No,
		// XCU defines ${#parameter} as the length of the value "in
		// characters", and defines a character as what the locale's
		// LC_CTYPE category says one is. So the standard's answer is yes,
		// and it is also what every panel member but dash does.
		MultibyteEncodingIsHonored: Yes,
		// XBD ranks the locale variables and then leaves the case where none
		// of them is set to the implementation: what applies is the
		// implementation-defined default locale. The default a C program
		// starts in is the C locale, and asking for the environment's when
		// the environment names none leaves it there — so the standard's
		// preset reads an unset locale as C. It is also what every panel
		// member but one does.
		UnsetLocaleIsUnicodeAware:       No,
		DollarZeroNamesTheInnermostCall: No,
		// A special builtin's failure is fatal to a non-interactive shell,
		// which the standard states outright. dash is the only member of the
		// panel that still does it, and the preset follows the standard
		// rather than the majority.
		BuiltinSyntaxErrorFatal: Yes,
		// An expansion error ends a non-interactive shell, which XCU states
		// outright, and it says the same of an error in a special builtin —
		// `.` is one. So the standard's answer is that nothing is caught at
		// a `.`, and dash, the panel member that targets this text,
		// complies. ksh93 and zsh are the departure.
		FatalErrorEndsBorrowedTextOnly: No,
		// XCU's `${parameter?word}` says the shell writes the word and
		// *exits*, in those words, so the standard reads the operator as a
		// request to stop rather than as one more error. Nothing in the
		// preset can observe the difference — a preset whose `.` catches
		// nothing has no boundary to catch it at — but the answer is the
		// standard's and is written down rather than left to a refusal.
		ParamErrorIsAnExitRequest: Yes,
		// POSIX makes a special builtin's failure fatal, and a bad option is
		// one.
		BadOptionToSpecialBuiltinFatal: Yes,
		// XCU's `[n]>&word` takes "one or more digits", so the standard
		// admits `>&10` and the preset follows it. The shell that refuses is
		// the dissenter here, which is worth noting because it is usually
		// the panel's POSIX-faithful member.
		MultiDigitDuplicationTargetIsAnError: No,
		// A redirection that cannot be made is a special builtin's failure
		// as well, and the standard names it in so many words. Three of the
		// five follow it, and the two that do not both reach this answer as
		// soon as their own posix mode is on.
		RedirectErrorOnSpecialBuiltinFatal:      Yes,
		GreatAmpTarget:                          GreatAmpTargetIsADescriptor,
		DuplicationTargetErrorOnABuiltinIsFatal: No,
		// A bad name is a special builtin's failure too, and the standard
		// makes no exception for `unset`.
		BadNameToDeclarationFatal: Yes,
		BadNameToUnsetFatal:       Yes,
		// And so is a refusal to unset a readonly name: the standard makes a
		// special builtin's failure fatal and names no exception for this
		// one either.
		UnsetReadonlyFatal: Yes,
		// `local` needs a function to be local to, and saying so is what
		// three of the four do — the substrate keeps the answer it had
		// before the question was one.
		LocalOutsideAFunctionIsAnError: Yes,
		// The standard has no `local`; dash is the closest reading, and it
		// leaves the outer value visible until the first assignment.
		ValuelessDeclarationHidesTheOuterValue: No,
		// The standard has no `local` either, and it does have the export
		// attribute belong to the *name* for the life of the shell — so a
		// declaration of that name keeps it, which is what both shells with
		// a `local` worth the reading do.
		LocalInheritsTheExportAttribute: Yes,
		// The same reading of the same sentence, for a declaration that
		// assigns rather than one that shadows: the attribute belongs to
		// the name, so an assignment through a declaration utility leaves
		// it where it was. Both other shells with the builtin agree.
		DeclarationAssignmentClearsTheExportAttribute: No,
		// POSIX has `trap` save the action and execute it when the
		// condition arises, so the text is not read until then. Three of
		// the four agree; zsh reads it as the trap is set.
		TrapActionIsParsedWhenSet: No,
		// A shell runs what it has read rather than reading everything
		// first, which is unanimous for a script and is the same reading
		// applied to a trap's body. ksh93 is the one that reads a trap
		// body whole.
		TrapBodyRunsWhatParsed: Yes,
		// And it names where the failure was, not where the trap fired,
		// which is what three of the four do.
		TrapParseFailureNamesWhereItFired: No,
		// POSIX gives `trap` the signals and EXIT, and nothing else — ERR,
		// DEBUG and RETURN are conditions the shells added. dash still
		// refuses all three.
		TrapHasErrCondition:    No,
		TrapHasDebugCondition:  No,
		TrapHasReturnCondition: No,
		// POSIX resets a subshell's handled traps to their defaults, so the
		// listing shows what survived: the ignored signals, which the
		// standard still counts as traps in effect. The allowance for
		// save=$(trap) is a may, not a shall, and the preset follows the
		// rule rather than the allowance. KeptTrapListingIncludesExit is
		// left unanswered because a listing that is never kept never asks.
		SubshellKeepsTrapListing:           No,
		PipelineElementKeepsTrapListing:    No,
		BackgroundJobKeepsTrapListing:      No,
		SubshellHidesInheritedIgnoredTraps: No,
		// POSIX gives `command` -p and -v and gives `getopts` none, so a
		// leading `-` word is an option to the first and the optstring to
		// the second.
		CommandRejectsUnknownOption: Yes,
		GetoptsRejectsUnknownOption: No,
		// POSIX gives `umask` chmod's symbolic mode: a who list, then one
		// or more actions, each an operator and its permissions. So several
		// operators in a clause are allowed, an omitted who means all three,
		// `s` and `t` are permission characters like any other — and a who
		// with no action at all, or a letter that is neither, is not a
		// symbolic mode. The preset follows the grammar.
		SymbolicMaskTakesMoreThanOneOperator: Yes,
		SymbolicMaskWhoAloneSetsIt:           No,
		SymbolicMaskTakesTheSetuidLetter:     Yes,
		SymbolicMaskTakesTheStickyLetter:     Yes,
		// POSIX gives all three a *name*, and neither a special parameter
		// nor a positional one is a name — a positional has `shift` to
		// remove it.
		DeclarationNameOperands: PlainNamesOnly,
		UnsetNameOperands:       PlainNamesOnly,
		// `read` is given the same reading: the standard's synopsis is
		// `read var...`, and a positional parameter is not a var.
		ReadNameOperands: PlainNamesOnly,
		// And the standard has no prompt operand — `read` takes names and
		// nothing else — so the whole word is the name.
		ReadPromptOperand: ReadOperandIsAllName,
		// `read` is not in the standard's list of special builtins, so a
		// failure of it does not end the script. Five of the six agree.
		BadNameToReadFatal: No,
		// The standard has the utility check its operands and diagnose the
		// ones that are not names, which puts the check on the arguments
		// rather than on the line: nothing has been read when it fails.
		ReadRefusesABadNameBeforeReading: Yes,
		// ReadCountJudgesTheNamesAfterTheFirst is left unanswered on
		// purpose: the standard has no `-n` and no `-N`, so a core with no
		// count never asks, and an answer here would be read as a reading of
		// a sentence that does not exist.
		// The core has arrays — they are in the common denominator even
		// though POSIX has none — so `unset a[0]` names an element and
		// removes it, which is what three of the four do and the only part
		// of this anybody writes. A *declaration* still names a variable
		// rather than an element, which is bash's and dash's answer.
		DeclarationTakesASubscript: No,
		UnsetTakesASubscript:       Yes,
		// A fatal refusal takes the rest of the operand list with it, which
		// is the standard's reading — the builtin stops where it failed —
		// and dash's and bash-as-`sh`'s measured answer.
		BadNameDeclaresTheOperandsAfterIt: No,
		// `local` reads the declaration question rather than the export one,
		// and the standard gives it to nobody, so the core answers it the
		// same way it answers the neighboring one: a declaration names a
		// variable. Left unset it would refuse `local a[1]=v` as an
		// unanswered axis, which is a refusal about a construct the core
		// already had an answer for.
		TypesetTakesASubscript: No,
		// And `unset` says nothing about what is not there: the standard has
		// it remove what it finds and succeed either way, which read over a
		// subscript is the quiet answer.
		UnsetSubscriptOnAScalarIsAnError: No,
		// POSIX has `set` write each variable as an assignment "in a format
		// that can be reused as input", and dash — its closest reading —
		// single-quotes every value and lists no functions. The standard
		// gives `local` to nobody and `typeset` no options, so those two
		// keep their zero values: no option letters, and a bad one reported
		// rather than fatal — `typeset` is not one of the builtins POSIX
		// marks special.
		SetListing:        SetListingAssignments,
		SetListingQuoting: ListingQuoteAlwaysDoubled,
		// dash quotes every listed value and never reaches `$'...'`, so the
		// numeric fallback is never asked for there; the octal one is what
		// POSIX's own `printf` writes, and is the reading to start from.
		ListingControlEscape: ControlEscapeOctal,
		// POSIX has the operand-less `export` and `readonly` write output
		// "in a form that may be reused as input", which is the command word
		// and the assignment — the same thing `-p` writes.
		ExportListing:          DeclareListingCommandWord,
		ReadonlyListing:        DeclareListingCommandWord,
		BareDeclarationListing: DeclareListingCommandWord,
		BareLocalListing:       BareLocalListsNothing,
		TypesetBadOptionFatal:  No,
		// POSIX has no `typeset`, so nothing in the standard reads a lone
		// sign as an option word and an operand is what is left. It is also
		// the answer that declares nothing behind a script's back.
		SignAloneIsAnOptionWord: No,
		// POSIX has neither `typeset` nor `declare`, so it has no `f`
		// letter to sign and nothing to say about it.
		// POSIX gives `%string` and `%?string` outright, has `wait` answer
		// for a job that is not there, and calls a string matching more
		// than one job unspecified — refusing is the reading that invents
		// nothing. It has no `wait -n` and no `disown` at all, so the
		// second stays unanswered and the letter is refused.
		JobSpecsByName:            Yes,
		AmbiguousJobNameIsRefused: Yes,
		WaitReportsAMissingJob:    Yes,
		WaitNWaitsForTheNextJob:   No,
		// POSIX has an interrupted `wait` report a status above 128 and does
		// not carve out the form that names a job; four of the five measured
		// builds agree.
		WaitForAJobFailsWhenInterrupted: No,
		DotMissingFileFatal:             Yes,
		DotWithNoOperandIsAnError:       Yes,
		// The standard gives `.` a file to read commands from and says nothing
		// about a directory. dash, the panel's POSIX-faithful member, reads it
		// to its end, finds no commands and reports success — so the preset
		// follows the shell rather than the silence, the same way the `exec`
		// exit-trap answer below does.
		DotDirectoryOperandIsAnError: No,
		// The standard gives `.` a filename and nothing else; passing
		// positional parameters to a sourced file is an extension three of
		// the four grew. And it reads the file from PATH, with no mention of
		// the current directory as a fallback.
		DotPassesArguments:             No,
		DotFallsBackToCurrentDirectory: No,
		// The standard says a special builtin's failure is fatal and says
		// nothing about a trap on the way out; dash, the panel's
		// POSIX-faithful member, runs it, so the preset follows the shell
		// rather than the silence. `exec` takes no options in the standard —
		// -a is an extension three of the four grew.
		ExecFailureRunsExitTrap: Yes,
		ExecTakesOptions:        No,
		// The standard says an empty element is the current directory and
		// makes no exception for the whole variable being empty, so the
		// preset follows the text and the majority together.
		EmptyPathIsTheCurrentDirectory: Yes,
		// The standard's 126 is for a command that was found and cannot be
		// executed; a directory qualifies, and three of the four report it.
		DirectoryOnPathIsACandidate: Yes,
		// POSIX's hash concerns utilities, and dash — its closest reading —
		// counts builtins and functions too, and reports a missing name.
		HashReportsAMissingName: Yes,
		HashSearchesPathAlone:   No,
		// POSIX has no such names; refusal is one shell's own answer.
		PunctuatedFunctionNameIsRefused: No,
		SetHasTraceLetters:              No,
		// The standard's `set` has no -t and neither does its `sh`, so the
		// preset follows the text; the two shells that grew the letter
		// override. dash — the closest reading of the standard here — is the
		// one that refuses it outright, which is the same answer.
		SetHasTheTLetter: No,
		// POSIX names -h itself, as command tracking: "locate and remember
		// utilities invoked by functions as those functions are defined".
		// dash is the one shell that refuses the letter, and overrides.
		SetHasTheHLetter:         Yes,
		SetHLetterTracksCommands: Yes,
		// POSIX ties -m to process groups and job notices, not to a
		// terminal; the two shells that want one override.
		MonitorNeedsATerminal: No,
		// The standard does not answer this one. XCU says `-m` "shall be
		// enabled by default for interactive shells" and names no terminal
		// in that sentence, but it also defines job control throughout in
		// terms of a controlling terminal — so the sentence is silent about
		// the case where there is none rather than permissive about it.
		// Where the text is silent the preset takes the answer that claims
		// less: a shell with no terminal does not say it is running a
		// monitor. It is also three of the four.
		InteractiveMonitorNeedsATerminal: Yes,
		// The standard says nothing about announcing a job to a shell that
		// was handed a script to run, so the preset claims less and says
		// nothing. It is the intersection as well: bash is silent here and
		// the other three are not, and a core made of what they all do is
		// the quiet one.
		InteractiveScriptAnnouncesJobs:  No,
		TildePlusMinusExpands:           No,
		UnderscoreTracksTheLastArgument: No,
		// POSIX has no `$_`, so nothing is written at startup and a name
		// the environment carried is an ordinary variable that shows
		// through — which is also the majority, five of the six.
		UnderscoreStartsAtTheInvocation:      No,
		UnderscoreInheritsFromTheEnvironment: Yes,
		// The majority answers: full bases, wrapping overflow, zero for an
		// empty expression.
		TestIntegerRefusalIsSilent: No,
		// POSIX has no -nt or -ot at all; dash, its closest reading, wants
		// both files to exist.
		MissingFileIsOlder: No,
		// POSIX gives -t a file descriptor, and dash refuses anything that
		// is not a number.
		TerminalTestRequiresANumber: Yes,
		// POSIX gives the one-argument form of `test` to the string rule
		// with no exception in it, which is dash's reading and bash's.
		BareTerminalTestIsDescriptorOne:  No,
		FcEmptyHistoryIsAnError:          No,
		JobControlAbsenceIsReportedFirst: No,
		// POSIX has `( )` run "in a subshell environment" and describes that
		// environment as a copy, which is the forking reading: the copy is
		// not the process the signal was aimed at, so it finishes its body.
		// Five of the six as well.
		SubshellRunsOnAfterSignalingTheShell: Yes,
		// The standard describes `exit` as exiting and says nothing about a
		// job left stopped, so the base leaves; bash and zsh, which stay and
		// warn, override.
		StoppedJobsHoldTheExit:       No,
		CdpathAnnouncesTheDirectory:  Yes,
		FdVariableOutlivesTheCommand: Yes,
		// The standard has the here-document end at the delimiter and says
		// nothing about a body the input cut short, so this follows the
		// panel: three of the five leave the last line as it was written and
		// only bash supplies the newline (#1020).
		UnterminatedHeredocGainsATrailingNewline: No,
		// The standard has no `$(<file)` form at all, so this follows the
		// panel: bash 5.3 and ksh93 read a directory to status 0, and only
		// zsh and bash 3.2 fail (#1778).
		ReadFailureInAFileSubstitutionFailsIt: No,
		FdVariableBadCloseIsAnError:           Yes,
		// The standard says nothing about a ceiling, so this follows the
		// panel: bash and ksh93 hand the kernel's refusal back, dash and zsh
		// report success on a number the process cannot hold.
		FdNumberBoundedByOpenFileLimit: Yes,
		// The standard is silent and four of the five hand the descriptor
		// over, which is what the flock and shared-log idioms are built on.
		ExecOpenedFdReachesACommand:        Yes,
		ReadRequiresAVariableName:          No,
		ArrayLengthWithoutSubscriptIsCount: No,
		// An empty scalar is one empty element to every shell in the panel
		// but the one that spells the construct as a list of its own.
		WholeSubscriptOnAScalarMeasuresIt: No,
		// The other way round: four of the five shells that have the
		// construct slice the value's characters, and only ksh93 slices a
		// list of one. POSIX has neither arrays nor substrings, so the
		// panel is all there is to follow.
		WholeSubscriptOnAScalarSlicesIt: Yes,
		UnsetNameAtIsOneEmptyField:      No,
		SubstringNegativeLengthIsEmpty:  No,
		// The standard has no modifiers and no history syntax, so a range is
		// the arithmetic it looks like — which is also what three of the four
		// do with it.
		SubstringRangeReadsModifiers: No,
		LinenoCountsFromTheFunction:  No,
		ArithBaseAbove36:             Yes,
		// The standard's numeral is C's, where a leading zero opens an octal
		// constant — so a base cannot be written with one, and a radix prefix
		// needs at least one digit after it. Neither is a base spelling the
		// standard describes, since `base#digits` is not in it at all; what
		// the preset follows is the constant syntax it does describe.
		ArithBaseMayHaveALeadingZero:         No,
		ArithBaseIsAtMostTwoDigits:           No,
		ArithBaseZeroReadsTheDigitsAsWritten: No,
		ArithEmptyRadixDigitsAreZero:         No,
		ArithOverflowSaturates:               No,
		EmptyArithExpressionIsAnError:        No,
		// The standard says `times` takes no operands and does not say what to
		// do with one; the two shells that follow it most closely ignore it.
		TimesRejectsArguments: No,
	}
}

// CoreSemantics fixes the axes every shell in the core panel agrees on and
// leaves the rest unspecified.
//
// It is the counterpart of syntax.Core(), built the same way: that refuses
// constructs not every shell has, and this refuses *behaviors* not every
// shell shares. A script that runs under it depends on nothing the panel
// disagrees about, which makes it a portability check rather than a runtime —
// the same role docs/spec/core.md gave strict POSIX.
//
// Almost no axis survives — the two fields below are the whole of it — and
// that is not a defect of the panel. The axes exist because they diverge;
// everything shells agree about never became one.
//
// So it is *not* the counterpart in the sense of being an equally complete
// preset, and the asymmetry is the sharpest consequence of the whole
// specification rather than an oversight in this function. syntax.Core() gives
// every flag in its set a value because grammar differences are *additive*: a
// construct either parses or it does not, so "what every shell accepts" is a
// well-defined intersection. Semantic differences are conflicts. There is no
// intersection of "an unquoted expansion is split" and "it is not", and no
// value of a boolean means both, which is why Answer has three states and why
// nearly every axis comes back Unspecified here and is then refused by name.
//
// Nothing in this package therefore has a "default" answer to an axis to
// document. A spec entry that says otherwise is wrong; docs/spec/README.md
// spells out how an entry is required to name the field that governs it.
//
// The pairing this implies is not an inconsistency: accept the constructs
// every real shell accepts, and mean what the shell most scripts were written
// against means — syntax.Core() with the Semantics() of dialect/bash. Which
// pairing to use is not this package's decision. This is the substrate: it
// owns the *mechanism*, and a shell built on it owns the policy, the same way
// the gate and the event stream are defined here and the sandbox backends and
// protocols are not. The presets exist so that choice can be spelled in one
// line rather than one per axis.
func CoreSemantics() Semantics {
	return Semantics{
		SplitCommandSubstitution: Yes,
		LengthOfSpecialIsCount:   Yes,
		// Every shell in the panel reads a profile for a login shell, so the
		// core reads one too; the disagreement is only over what it is
		// called. `.profile` is the standard's name and nobody's brand,
		// which is the same choice `$ENV` is for the interactive file and
		// made for the same reason — this binary is not bash and must not
		// claim `.bashrc`.
		//
		// The zero Semantics still names nothing, and that is the split
		// worth keeping: a vector nobody filled in belongs to a library
		// embedder or a test, neither of which should touch a home
		// directory because a field was left at its default.
		LoginStartupFiles: ".profile",
		// And a way to say so. All four shells in the panel take `-l`, so
		// the common denominator has it even though the standard does not
		// — which is the one respect in which this differs from
		// PosixSemantics here. Without it the only way to start a login
		// shell is to exec with a dashed argv[0], which is what `login`
		// does and what a person at a keyboard cannot.
		StartupFileOptions: StartupFileOptions{Login: "-l"},
	}
}

// sem returns the runner's semantics, defaulting to the core.
//
// Defaulting to a *shell* would be the substrate answering a question that is
// not its to answer. Defaulting to the core answers it honestly: what every
// shell agrees on is done, and anything else is refused until something above
// chooses. A shell built on this package sets the field; that is its job.
// The axis sweep hooks the return rather than any caller, because this is the
// one place the interpreter reads the vector: a hook here covers a dialect
// binary and a test that builds its own Semantics alike. axisMutate is the
// identity in every build but the sweep's — see interp/axissweep_off.go.
func (r *Runner) sem() Semantics {
	if r.Semantics != nil {
		return axisMutate(*r.Semantics)
	}
	return axisMutate(CoreSemantics())
}

// swapSemantics moves an axis at run time, copy-on-write.
//
// A subshell clone shares the vector by pointer, so the change goes on a
// fresh copy and stays this runner's own — which is also what keeps a mode
// entered inside a subshell inside it. The same shape a dialect uses from
// outside the package, kept here because the core has a mode of its own to
// switch: `set -o posix`.
func (r *Runner) swapSemantics(change func(*Semantics)) {
	s := r.sem()
	change(&s)
	r.Semantics = &s
}

// BackgroundJobInputPolicy is what a job started with `&` reads for standard
// input while job control is off.
type BackgroundJobInputPolicy int

const (
	// BackgroundJobInputUnspecified is no answer, and reads as
	// BackgroundJobInputEmpty rather than being refused — see the field.
	BackgroundJobInputUnspecified BackgroundJobInputPolicy = iota
	// BackgroundJobInputEmpty hands the job an empty stream whatever the
	// shell's own input was, a closed descriptor included: dash and bash.
	BackgroundJobInputEmpty
	// BackgroundJobInputEmptyUnlessClosed hands the job an empty stream, but
	// leaves a closed descriptor closed so the job reports EBADF: ksh93.
	BackgroundJobInputEmptyUnlessClosed
	// BackgroundJobInputIsTheShells hands the job the shell's own standard
	// input, which is the descriptor the script goes on reading: zsh.
	BackgroundJobInputIsTheShells
)

func (b BackgroundJobInputPolicy) String() string {
	switch b {
	case BackgroundJobInputEmpty:
		return "empty"
	case BackgroundJobInputEmptyUnlessClosed:
		return "empty unless closed"
	case BackgroundJobInputIsTheShells:
		return "the shell's"
	}
	return "unspecified"
}

// TrapLocality is what happens to a trap a function set when that function
// returns. See Semantics.FunctionLocalTraps.
//
// A form rather than a flag, because the shells that scope a function's traps
// at all do not agree on what *asks* for the scoping. One has an option that
// turns it on for every call while it is set. The other keys it on the
// definition style: measured against ksh93 (AT&T 93u+ 2012-08-01), `function
// g { trap "echo I" USR1; }` puts the caller's trap back at the return and
// `g() { trap "echo I" USR1; }`, the same body written the other way, leaves
// the new one installed. That second answer is a third value here rather than
// a rewrite of a boolean, and it is left for its own issue — the shell that
// has it does not answer this axis yet.
type TrapLocality uint8

const (
	// TrapLocalityUnspecified reads as TrapsSurviveTheFunction rather than
	// being refused, which is not the usual bargain and is deliberate: there
	// is no disagreement here to refuse over. Every shell in the panel
	// leaves a function's trap installed unless something in that function
	// asked otherwise, so this is a question asked only where a dialect has
	// raised it, and a vector nobody filled in gets the answer they share.
	TrapLocalityUnspecified TrapLocality = iota
	// TrapsSurviveTheFunction leaves the modification standing: the trap the
	// function set is the trap the caller has afterwards.
	TrapsSurviveTheFunction
	// TrapsGoBackAtTheReturn puts the displaced disposition back as the
	// function returns.
	//
	// The save is per condition and it is taken when the trap is *modified*,
	// not when the body starts: measured, a trap set before the line that
	// asks for the scoping is not restored. First save wins, so a body that
	// sets the same condition twice goes back to what it displaced rather
	// than to what it set first. And the restore is unconditional at the
	// return — a body that stops asking for the scoping after it has moved a
	// trap still has that trap put back.
	TrapsGoBackAtTheReturn
)

func (t TrapLocality) String() string {
	switch t {
	case TrapsSurviveTheFunction:
		return "traps survive the function"
	case TrapsGoBackAtTheReturn:
		return "traps go back at the return"
	}
	return "unspecified"
}

// StatusArgumentPolicy is how the status operand of `exit` and `return` is
// read. See Semantics.StatusArgument for the measurements behind the four.
type StatusArgumentPolicy int

const (
	// StatusArgUnspecified is no answer, and is refused like any other.
	StatusArgUnspecified StatusArgumentPolicy = iota
	// StatusArgStrict takes decimal digits and refuses everything else —
	// no sign and no text — and the refusal ends the script, because a
	// special builtin's failure is fatal there: dash. It does not mask, so
	// `return 300` leaves 300 behind rather than 44.
	StatusArgStrict
	// StatusArgNumeric refuses text but takes a sign, and masks to eight
	// bits: bash. `return -1` is 255 and `return 300` is 44. The refusal is
	// reported and the function still returns, leaving 2; the script carries
	// on. Called as `sh` the same binary ends the script instead, which is
	// the POSIX rule rather than a second reading of the operand.
	StatusArgNumeric
	// StatusArgLeadingDigits reads the number the operand begins with and
	// ignores whatever follows, masking to eight bits: ksh93. `return 3abc`
	// is 3, `return r` is 0 whatever `r` holds, `return " -5x"` is 251, and
	// nothing is ever refused. Not arithmetic: `return 010` is 10 there
	// while `$((010))` is 8, so the operand is not going through the
	// arithmetic reader.
	StatusArgLeadingDigits
	// StatusArgArithmetic evaluates the operand as an arithmetic expression
	// and does not mask: zsh. `return r` is the value of `r`, `return r+1`
	// is one more, `return "(r+1)*2"` is six for r=2, `return 0x10` is 16,
	// and `return 300` leaves 300 behind. Nothing is refused either, but an
	// expression that will not parse is a math error rather than a status.
	//
	// This is the one that made `return r` a silent wrong answer: the
	// operand was discarded and `$?` handed back in its place, so a function
	// meaning to return 3 returned whatever ran last.
	StatusArgArithmetic
)

func (e StatusArgumentPolicy) String() string {
	switch e {
	case StatusArgStrict:
		return "strict"
	case StatusArgNumeric:
		return "numeric"
	case StatusArgLeadingDigits:
		return "leading digits"
	case StatusArgArithmetic:
		return "arithmetic"
	}
	return "unspecified"
}

// statusArgument resolves the axis, and only for an operand that is actually
// questionable — `exit 3` and `return 3` need no answer from anyone.
//
// The builtin is named rather than assumed, because the axis now answers for
// two of them and a complaint that always said `exit` would send a reader
// looking at the wrong line.
func (r *Runner) statusArgument(builtin string) StatusArgumentPolicy {
	p := r.sem().StatusArgument
	if p == StatusArgUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered(builtin+": this argument")))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// PrintfBackslashCPolicy is what `\c` means in a printf format.
type PrintfBackslashCPolicy int

const (
	// PrintfBackslashCUnspecified is no answer, and is refused like any other.
	PrintfBackslashCUnspecified PrintfBackslashCPolicy = iota
	// PrintfBackslashCLiteral writes the two characters: bash, dash.
	PrintfBackslashCLiteral
	// PrintfBackslashCControl reads `\cX` as control-X: ksh93.
	PrintfBackslashCControl
	// PrintfBackslashCStops ends the output there: zsh.
	PrintfBackslashCStops
)

func (p PrintfBackslashCPolicy) String() string {
	switch p {
	case PrintfBackslashCLiteral:
		return "literal"
	case PrintfBackslashCControl:
		return "control character"
	case PrintfBackslashCStops:
		return "stops the output"
	}
	return "unspecified"
}

// backslashC resolves the axis, and only for a format that has a `\c` in it.
func (r *Runner) backslashC() PrintfBackslashCPolicy {
	p := r.sem().PrintfBackslashC
	if p == PrintfBackslashCUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered(`printf: \c`)))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// PrintfHexEscapePolicy is how a printf format reads `\x`.
//
// Four answers, measured digit by digit rather than assumed from the two the
// escape is usually described with. The panel splits on three separate
// details and each split falls in a different place, which is why this is one
// enumeration and not a bool:
//
//   - Whether the escape exists. dash has no `\x`, so `printf 'a\x41Z'` is
//     the four characters as written.
//   - How wide the digit run is. bash and zsh stop at two and the value is a
//     byte, so `\x0ff` is 0x0f followed by an `f`. ksh93 takes every digit
//     that follows and the value is a *code point* once there are more than
//     two of them: `\xff` is one byte and `\x0ff` is U+00FF in UTF-8.
//   - What an empty digit run means. bash leaves `\x` standing and says so
//     on standard error; ksh93 and zsh read it as zero and write a NUL.
type PrintfHexEscapePolicy int

const (
	// PrintfHexEscapeUnspecified is no answer, and is refused like any other.
	PrintfHexEscapeUnspecified PrintfHexEscapePolicy = iota
	// PrintfHexEscapeAbsent has no `\x` at all, so the backslash and the
	// letter stand as written: dash.
	PrintfHexEscapeAbsent
	// PrintfHexEscapeByte reads at most two digits as one byte, and leaves
	// `\x` with no digit after it as written: bash 3.2 and bash 5.3.
	PrintfHexEscapeByte
	// PrintfHexEscapeByteOrNul reads the same two digits, and an empty digit
	// run as a zero: zsh.
	PrintfHexEscapeByteOrNul
	// PrintfHexEscapeCodePoint reads every digit that follows. Up to two of
	// them is a byte and three or more is a code point written in UTF-8, and
	// an empty run is a zero: ksh93.
	PrintfHexEscapeCodePoint
)

func (p PrintfHexEscapePolicy) String() string {
	switch p {
	case PrintfHexEscapeAbsent:
		return "absent"
	case PrintfHexEscapeByte:
		return "two digits, one byte"
	case PrintfHexEscapeByteOrNul:
		return "two digits, one byte, and no digits is a NUL"
	case PrintfHexEscapeCodePoint:
		return "every digit, a code point"
	}
	return "unspecified"
}

// hexEscape resolves the axis, and only for a format that has a `\x` in it.
func (r *Runner) hexEscape() PrintfHexEscapePolicy {
	p := r.sem().PrintfHexEscape
	if p == PrintfHexEscapeUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered(`printf: \x`)))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// bHexEscape resolves the `%b` site's `\x` reading, and only for an argument
// that has a `\x` in it.
//
// Separate from hexEscape because the site is half the question: ksh93 reads
// every digit of a format's `\x41` and writes the four characters as they
// stand in a `%b`.
func (r *Runner) bHexEscape() PrintfHexEscapePolicy {
	p := r.sem().PrintfBHexEscape
	if p == PrintfHexEscapeUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered(`printf: \x in a %b argument`)))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// PrintfUnicodeEscapePolicy is how a printf format reads `\uHHHH` and
// `\UHHHHHHHH`.
//
// Four answers, and they are not PrintfHexEscapePolicy's four. The two escapes
// ask the same three questions and the panel answers them in different places:
//
//   - Whether the escape exists. bash 3.2 and dash have no `\u` at all, so
//     `printf 'a\u0041Z'` is the ten characters as written. bash 5.3 has it
//     under either argv[0], so the panel's `bash` and `bash-as-sh` columns
//     agree here and it is the *version* that decides rather than the name.
//   - How wide the digit run is. Unanimous among the three that have it, and
//     the one question `\x` splits on that this one does not: four digits
//     after `\u` and eight after `\U`, with fewer accepted and the run ending
//     at the first character that is not a digit.
//   - What an empty digit run means. bash 5.3 leaves the escape standing and
//     says so on standard error; zsh reads it as a zero and writes a NUL;
//     ksh93 drops the rest of that pass over the format.
//
// The last of those is the reading no `\x` has anywhere in the panel, and it
// is why this is its own enumeration rather than the hexadecimal one reused.
// It is a *pass* that ends and not the builtin: the operands go on being
// consumed, so `printf '[%s]\uZ' x y` is `[x][y]` there, where a `\c` that
// stops — see PrintfBackslashCStops — would write `[x]` and end.
type PrintfUnicodeEscapePolicy int

const (
	// PrintfUnicodeEscapeUnspecified is no answer, and is refused like any
	// other.
	PrintfUnicodeEscapeUnspecified PrintfUnicodeEscapePolicy = iota
	// PrintfUnicodeEscapeAbsent has no `\u` or `\U` at all, so the backslash
	// and the letter stand as written: bash 3.2 and dash.
	PrintfUnicodeEscapeAbsent
	// PrintfUnicodeEscapeCodePoint reads the digits and leaves an escape with
	// no digit after it as written, with a complaint that does not change the
	// status: bash 5.3.
	PrintfUnicodeEscapeCodePoint
	// PrintfUnicodeEscapeCodePointOrNul reads the same digits, and an empty
	// digit run as a zero: zsh.
	PrintfUnicodeEscapeCodePointOrNul
	// PrintfUnicodeEscapeCodePointOrTruncate reads the same digits, and an
	// empty digit run ends this pass over the format: ksh93.
	PrintfUnicodeEscapeCodePointOrTruncate
)

func (p PrintfUnicodeEscapePolicy) String() string {
	switch p {
	case PrintfUnicodeEscapeAbsent:
		return "absent"
	case PrintfUnicodeEscapeCodePoint:
		return "a code point, and no digits stands as written"
	case PrintfUnicodeEscapeCodePointOrNul:
		return "a code point, and no digits is a NUL"
	case PrintfUnicodeEscapeCodePointOrTruncate:
		return "a code point, and no digits ends the pass"
	}
	return "unspecified"
}

// unicodeEscape resolves the axis, and only for a format that has a `\u` or a
// `\U` in it.
func (r *Runner) unicodeEscape() PrintfUnicodeEscapePolicy {
	p := r.sem().PrintfUnicodeEscape
	if p == PrintfUnicodeEscapeUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered(`printf: \u`)))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// bUnicodeEscape resolves the `%b` site's reading, and only for an argument
// that has a `\u` or a `\U` in it.
//
// Separate from unicodeEscape because the site is half the question: ksh93
// reads a format's `\u0041` and writes the ten characters as they stand in a
// `%b`.
func (r *Runner) bUnicodeEscape() PrintfUnicodeEscapePolicy {
	p := r.sem().PrintfBUnicodeEscape
	if p == PrintfUnicodeEscapeUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered(`printf: \u in a %b argument`)))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// PrintfLengthModifierSet is which C length modifiers a printf conversion may
// carry, which is a set rather than a switch: the panel splits three ways and
// two of the three take a different number of letters.
type PrintfLengthModifierSet int

const (
	// PrintfLengthModifiersUnspecified is no answer, and is refused like any
	// other.
	PrintfLengthModifiersUnspecified PrintfLengthModifierSet = iota
	// PrintfLengthModifiersAbsent is a printf with no modifiers at all, so
	// `%ld` is a conversion `l` that does not exist: dash.
	PrintfLengthModifiersAbsent
	// PrintfLengthModifiersC89 is `h`, `l` and `L`, and exactly one of them:
	// zsh, which takes `%ld` and calls `%lld` an invalid directive. The set
	// is C89's, which is the reading that explains why `hh`, `ll`, `j`, `z`
	// and `t` — every one of them a C99 addition — are the ones refused.
	PrintfLengthModifiersC89
	// PrintfLengthModifiersC99 adds `hh`, `ll`, `j`, `z` and `t`, and takes
	// any run of the letters rather than one: bash and ksh93 read `%lll` and
	// `%zz` as happily as `%ll`, which is what makes this a skipped run and
	// not a list of spellings.
	PrintfLengthModifiersC99
)

func (p PrintfLengthModifierSet) String() string {
	switch p {
	case PrintfLengthModifiersAbsent:
		return "none"
	case PrintfLengthModifiersC89:
		return "h, l and L"
	case PrintfLengthModifiersC99:
		return "h, hh, l, ll, j, z, t and L"
	}
	return "unspecified"
}

// lengthModifiers resolves the axis, and only for a conversion that carries a
// letter one of the answers would take.
func (r *Runner) lengthModifiers() PrintfLengthModifierSet {
	p := r.sem().PrintfLengthModifiers
	if p == PrintfLengthModifiersUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered(`printf: a length modifier`)))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// PrintfQuoteStyle is how `%q` quotes a word so the shell can read it back.
type PrintfQuoteStyle int

const (
	// PrintfQuoteUnspecified is no answer, and is refused like any other.
	PrintfQuoteUnspecified PrintfQuoteStyle = iota
	// PrintfQuoteAnsiCWord moves the whole word into one `$'…'` as soon as a
	// byte cannot be written as itself, and backslash-escapes otherwise:
	// bash.
	PrintfQuoteAnsiCWord
	// PrintfQuoteAnsiCCharacter wraps each such byte in a `$'…'` of its own
	// and leaves the rest backslash-escaped: zsh. It is the same answer that
	// shell's `${(q)…}` gives, measured byte for byte.
	PrintfQuoteAnsiCCharacter
	// PrintfQuoteSingle has three shapes — bare, `'…'`, and `$'…'` with hex
	// escapes — and picks by what the value holds: ksh93.
	PrintfQuoteSingle
	// PrintfQuoteAbsent is a dialect without `%q` at all: dash, which calls
	// it an invalid directive like any other conversion it does not have.
	PrintfQuoteAbsent
)

func (p PrintfQuoteStyle) String() string {
	switch p {
	case PrintfQuoteAnsiCWord:
		return "the whole word in $'…'"
	case PrintfQuoteAnsiCCharacter:
		return "each byte in its own $'…'"
	case PrintfQuoteSingle:
		return "single quoted"
	case PrintfQuoteAbsent:
		return "absent"
	}
	return "unspecified"
}

// quoteStyle resolves the axis, and only for a `%q` that is actually there.
func (r *Runner) quoteStyle() PrintfQuoteStyle {
	p := r.sem().PrintfQuote
	if p == PrintfQuoteUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("printf: %q")))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// DollarSingleControlPolicy is what `\c` means inside `$'…'`.
//
// The three answers were measured character by character rather than assumed
// from the usual "XOR 0x40" rule, and the measurement is why there are two
// decoding answers instead of one: the rules agree on every letter and on
// `@ [ \ ] ^ _` — the range where masking to five bits and toggling bit 6 are
// the same arithmetic — and part company everywhere else. `$'\c1'` is 0x11 in
// bash and `q` in ksh93.
type DollarSingleControlPolicy int

const (
	// DollarSingleControlUnspecified is no answer, and is refused like any
	// other.
	DollarSingleControlUnspecified DollarSingleControlPolicy = iota
	// DollarSingleControlMasked uppercases the character and keeps its low
	// five bits, with `?` reading as DEL: bash.
	//
	// The `?` is bash 5.3's answer. bash 3.2 has no special case and gives
	// 0x1f, which is what masking alone produces — dated rather than vetoed,
	// per docs/spec/core.md.
	DollarSingleControlMasked
	// DollarSingleControlToggled uppercases the character and toggles bit 6:
	// ksh93, where `\c?` is DEL because 0x3f toggles to 0x7f rather than
	// because anything special was said about it.
	DollarSingleControlToggled
	// DollarSingleControlAbsent is a dialect with no `\c` escape at all: zsh,
	// where the backslash falls to DollarSingleUnknownEscape like any other
	// character no escape claims.
	DollarSingleControlAbsent
)

func (p DollarSingleControlPolicy) String() string {
	switch p {
	case DollarSingleControlMasked:
		return "masked to five bits"
	case DollarSingleControlToggled:
		return "toggled by 0x40"
	case DollarSingleControlAbsent:
		return "absent"
	}
	return "unspecified"
}

// dollarSingleControl resolves the axis, and only for a `$'…'` that has a `\c`
// in it.
func (r *Runner) dollarSingleControl() DollarSingleControlPolicy {
	p := r.sem().DollarSingleBackslashC
	if p == DollarSingleControlUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered(`$'\c'`)))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// DollarSingleUnknownPolicy is what a backslash does before a character no
// escape claims.
type DollarSingleUnknownPolicy int

const (
	// DollarSingleUnknownUnspecified is no answer, and is refused like any
	// other.
	DollarSingleUnknownUnspecified DollarSingleUnknownPolicy = iota
	// DollarSingleUnknownKeepsBackslash keeps both characters, so `$'\q'` is
	// a backslash and a `q`: bash.
	DollarSingleUnknownKeepsBackslash
	// DollarSingleUnknownDropsBackslash keeps the character alone, so `$'\q'`
	// is a `q`: ksh93 and zsh.
	DollarSingleUnknownDropsBackslash
)

func (p DollarSingleUnknownPolicy) String() string {
	switch p {
	case DollarSingleUnknownKeepsBackslash:
		return "keeps the backslash"
	case DollarSingleUnknownDropsBackslash:
		return "drops the backslash"
	}
	return "unspecified"
}

// dollarSingleUnknown resolves the axis, and only for an escape that really
// has no meaning.
func (r *Runner) dollarSingleUnknown() DollarSingleUnknownPolicy {
	p := r.sem().DollarSingleUnknownEscape
	if p == DollarSingleUnknownUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered(`$'\': an escape with no meaning`)))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// KillStatusPolicy is what `kill` reports when its targets disagreed.
type KillStatusPolicy int

const (
	// KillStatusUnspecified is no answer, and is refused like any other.
	KillStatusUnspecified KillStatusPolicy = iota
	// KillStatusAnyFailure reports 1 if any target failed: dash, ksh93.
	KillStatusAnyFailure
	// KillStatusAnySuccess reports 0 if any target was signaled: bash.
	KillStatusAnySuccess
	// KillStatusFailureCount reports how many failed: zsh.
	KillStatusFailureCount
)

func (k KillStatusPolicy) String() string {
	switch k {
	case KillStatusAnyFailure:
		return "any failure"
	case KillStatusAnySuccess:
		return "any success"
	case KillStatusFailureCount:
		return "failure count"
	}
	return "unspecified"
}

// killStatusPolicy resolves the axis, and only where the targets actually
// disagreed — `kill $$` needs no answer from anyone, and neither does a
// command whose every target failed for the same reason.
func (r *Runner) killStatusPolicy() KillStatusPolicy {
	p := r.sem().KillStatus
	if p == KillStatusUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("kill: some of these targets")))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// killListAcceptsName resolves the axis, and only for an argument that is
// actually a name — `kill -l 9` needs no answer from anyone.
func (r *Runner) killListAcceptsName() Answer {
	a := r.sem().KillListAcceptsName
	if a == Unspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("kill -l: a signal name")))
		r.status = 2
		r.unspecified = true
	}
	return a
}

// BracketPolicy is what an unterminated bracket expression means.
type BracketPolicy int

const (
	// BracketUnspecified is no answer, and is refused like any other.
	BracketUnspecified BracketPolicy = iota
	// BracketLiteral treats the `[` as an ordinary character: bash, ksh93.
	BracketLiteral
	// BracketNoMatch treats it as a class that matches nothing: dash.
	BracketNoMatch
	// BracketBadPattern rejects the pattern: zsh.
	BracketBadPattern
)

func (b BracketPolicy) String() string {
	switch b {
	case BracketLiteral:
		return "literal"
	case BracketNoMatch:
		return "no match"
	case BracketBadPattern:
		return "bad pattern"
	}
	return "unspecified"
}

// bracketPolicy resolves the axis, refusing when no dialect answered — and
// only for a pattern that actually has an unterminated bracket, which is the
// rule the caret axis uses for the same reason.
func (r *Runner) bracketPolicy() BracketPolicy {
	p := r.sem().UnterminatedBracket
	if p == BracketUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("an unterminated bracket expression")))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// UnsetArraySpanPolicy is what `unset` does to the span of elements a
// subscript names — `a[@]` and `a[*]`, which every shell measured treats
// identically, and `a[3]`, which names a span of one.
//
// Three answers rather than a switch, and the third is not a variation on the
// other two: one shell does not read `@` as a spelling for "every element" at
// all, so the brackets hold an arithmetic expression like any other and `@` is
// not one. That is a different question from what is left behind, and folding
// it into a boolean would have had to call it "does not clear", which says
// nothing about why.
//
// The two readings of the whole-array spelling reach the single subscript
// unchanged, which is why there is one field and not two. Removing every
// element the subscript names removes the one `a[3]` names; replacing the span
// with a single empty element replaces a span of one with a blank in the same
// place, so the array keeps its length. The third answer parts from the other
// two only over whether `@` is an expression, and `3` is one in every reading,
// so at a single subscript it removes like the first.
// CompoundAttributePolicy is what an attribute a declaration has just added
// makes of a value the name is already holding when that value is *compound*
// — an array or a keyed table.
//
// The scalar question is AttributeRereadsTheValueItFinds and it splits the
// panel two ways: bash waits for the next assignment, ksh93 and zsh re-read.
// For a compound value it splits **three** ways, and the two shells that share
// the scalar answer disagree with each other about what reaching back into an
// array even means. So it is a second question with its own answer per shell
// rather than a widening of the first, in the family ArraysAreSparse and
// ArrayBaseIsZero already belong to.
//
//	arr=(a b); typeset -i arr      bash `a b`   ksh93 `0 0`   zsh `0`, one element
//	brr=(a b); typeset -u brr      bash `a b`   ksh93 `A B`   zsh `a b`
//	typeset -A m; m[k]=v
//	typeset -i m                   bash `v`     ksh93 `0`     zsh empty, and
//	                                                          the child is told `m=0`
//
// Measured 2026-09-07, `env -i PATH=/usr/bin:/bin` with a scratch `HOME`,
// `ZDOTDIR` and `HISTFILE`, from a script file.
type CompoundAttributePolicy int

const (
	// CompoundAttributeUnspecified is no answer, and it is refused rather
	// than guessed: the three readings leave three different names behind,
	// and one of them leaves no array at all.
	CompoundAttributeUnspecified CompoundAttributePolicy = iota
	// CompoundAttributeKeepsTheElements leaves the compound value exactly as
	// it stands and waits for the next write: bash, in both builds measured,
	// where `arr=(a b); typeset -i arr` still reads `a b` and the listing
	// carries the letter over untouched elements. It is the same answer that
	// shell gives for a scalar, which is what makes bash the one column
	// where the two questions cannot be told apart.
	CompoundAttributeKeepsTheElements
	// CompoundAttributeFoldsEveryElement re-reads each element through the
	// attribute in place, keeping the shape: ksh93, where `(a b)` under `-i`
	// becomes `0 0` and under `-u` becomes `A B`, `(0x10 9)` becomes `16 9`,
	// and a keyed table's `5+5` becomes 10 under its own key. The array
	// keeps its length and the table keeps its keys.
	CompoundAttributeFoldsEveryElement
	// CompoundAttributeReplacesItWithAScalar discards the compound value
	// outright and leaves the name a *fresh* scalar of the declared type:
	// zsh, where `arr=(a b); typeset -i arr` leaves one element and
	// `${#arr[@]}` is 1, a keyed table comes back empty, and a later
	// `arr[0]=3+4` is refused because the name is no longer an array.
	//
	// A fresh one and not a fold of anything: `(7 8)`, `(x y)` and
	// `(0x10 9)` all leave `0`, which is exactly what that dialect's
	// DeclaredNameWithoutValueIsEmpty = yes gives a name it has never held.
	// The child is told `0` too, so it is the value and not a rendering.
	//
	// Only the letter that names a **type** does this, because only that
	// letter changes what kind of name it is. The case letters change no
	// kind and leave the compound alone in that shell — which is
	// CompoundElementsGoThroughTheAttribute answering no there, and is why
	// this policy is read for the integer letter and that field for the
	// others.
	CompoundAttributeReplacesItWithAScalar
)

func (p CompoundAttributePolicy) String() string {
	switch p {
	case CompoundAttributeKeepsTheElements:
		return "keeps the elements"
	case CompoundAttributeFoldsEveryElement:
		return "folds every element"
	case CompoundAttributeReplacesItWithAScalar:
		return "replaces it with a scalar"
	}
	return "unspecified"
}

// compoundAttribute resolves the axis, refusing an unanswered dialect rather
// than guessing at it: one answer leaves the array alone, one rewrites every
// element and one leaves no array at all.
func (r *Runner) compoundAttribute() CompoundAttributePolicy {
	p := r.sem().CompoundAttribute
	if p == CompoundAttributeUnspecified {
		r.diagf("%s\n", r.unanswered("an attribute added to a name already holding an array"))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// ScalarUnderACompoundPolicy is what a declaration that gives a name the
// array or the table attribute does with a *scalar* the name is already
// holding. The converse of CompoundAttributePolicy, and it splits the panel
// three ways as well.
//
// Measured 2026-09-08, panel and machine as docs/spec/oracle.md:
//
//	b=1; typeset -a b; typeset -p b
//	  bash 5.3.15   declare -a b=([0]="1")   n=1   $b is `1`
//	  bash 3.2.57   declare -a b=([0]="1")   n=1   $b is `1`
//	  ksh93         b=1                      n=1   $b is `1`
//	  zsh 5.9.2     typeset -a b=( )         n=0   $b is empty
//
//	a=1; typeset -A a; typeset -p a
//	  bash 5.3.15   declare -A a=([0]="1" )  n=1
//	  ksh93         typeset -A a=([0]=1)     n=1
//	  zsh 5.9.2     typeset -A a=( )         n=0
//
// dash has neither letter and bash 3.2 has no `-A`. This implementation
// answered every column with zsh's, which matched one shell by accident and
// lost the value in the other two at status 0 (#1572).
//
// ksh93's two letters are the reason there are two fields rather than one,
// and its `-a` answer is a genuine third state rather than a rendering of
// bash's. Two probes say so: a one-element array of ksh93's own making lists
// *with* the letter — `b=(1); typeset -p b` is `typeset -a b=(1)` — and a
// bare `typeset -a`, which lists every name carrying the attribute, prints
// nothing at all after `b=1; typeset -a b`. So the declaration recorded
// nothing and converted nothing; the name is still the scalar it was, and
// ksh93 lets a scalar be subscripted, which is why `${b[0]}` and `${#b[@]}`
// cannot tell that apart from bash's promotion.
//
// Asked only where the name is holding a scalar *in the cell being declared*.
// An unset name has nothing to make anything of and is unanimous — `unset b;
// typeset -a b` is an array of no elements in all three — and so is a name
// already holding an array, which every column leaves exactly as it stands.
// A declaration carrying its own value is not this question either: `b=1;
// typeset -a b=(9)` is the one element `9` everywhere, because the operand
// replaces whatever the declaration left. See Runner.markDeclaredCompound.
type ScalarUnderACompoundPolicy int

const (
	// ScalarUnderACompoundUnspecified is no answer, and it is refused rather
	// than guessed at: the three readings differ over whether the script's
	// own value is still there, which is the kind of difference no later
	// command can report.
	ScalarUnderACompoundUnspecified ScalarUnderACompoundPolicy = iota
	// ScalarUnderACompoundBecomesTheFirstElement promotes: the value the name
	// was holding becomes the array's first element, or the table's entry
	// under the key `0`. bash for both letters, ksh93 for the table.
	//
	// The first element rather than any particular number, which is what the
	// store's positions already mean — no dialect that counts from 1 promotes
	// at all, so nothing here can be asked which subscript it is.
	ScalarUnderACompoundBecomesTheFirstElement
	// ScalarUnderACompoundStaysAScalar converts nothing and records nothing:
	// the name is the scalar it was, and the declaration is a no-op. ksh93
	// for `typeset -a`, where a bare `typeset -a` afterwards does not list
	// the name.
	//
	// Not the same as promoting, even though that shell reads `${b[0]}` as
	// the scalar and answers `${#b[@]}` with 1 either way: the listing is
	// what tells them apart, and it is the listing a script reads to find
	// out what a name is.
	ScalarUnderACompoundStaysAScalar
	// ScalarUnderACompoundDiscardsIt takes the value away and leaves the
	// name an empty array or table: zsh, for both letters, where `$b` reads
	// back empty and `${#b[@]}` is 0.
	ScalarUnderACompoundDiscardsIt
)

func (p ScalarUnderACompoundPolicy) String() string {
	switch p {
	case ScalarUnderACompoundBecomesTheFirstElement:
		return "becomes the first element"
	case ScalarUnderACompoundStaysAScalar:
		return "stays a scalar"
	case ScalarUnderACompoundDiscardsIt:
		return "discards it"
	}
	return "unspecified"
}

// scalarUnderACompound resolves one of the two axes, refusing an unanswered
// dialect rather than guessing: one answer keeps the script's value, one
// leaves the name a scalar and one throws the value away.
func (r *Runner) scalarUnderACompound(p ScalarUnderACompoundPolicy, what string) ScalarUnderACompoundPolicy {
	if p == ScalarUnderACompoundUnspecified {
		r.diagf("%s\n", r.unanswered(what))
		r.status = 2
		r.unspecified = true
	}
	return p
}

type UnsetArraySpanPolicy int

const (
	// UnsetArraySpanUnspecified is no answer. It is refused for the
	// whole-array spelling, where the panel genuinely disagrees about the
	// result. A single subscript takes it as removal, because a preset with
	// no arrays of its own has already committed to that reading — see
	// UnsetTakesASubscript, where the POSIX preset has `unset a[0]` name an
	// element and remove it.
	UnsetArraySpanUnspecified UnsetArraySpanPolicy = iota
	// UnsetArraySpanIsAnExpression reads the brackets as it reads any other
	// subscript: ksh93, where `@` is not an expression and the operand is
	// reported as a bad one. A subscript that *is* an expression names its
	// element and the element is removed.
	UnsetArraySpanIsAnExpression
	// UnsetArraySpanRemovesTheElements takes away every subscript the span
	// names: bash, in both builds measured, where `a[@]` leaves the array
	// with nothing in it and `a[3]` leaves a hole. A name that is not an
	// array is reported rather than emptied, and one that holds nothing at
	// all is quietly left alone.
	UnsetArraySpanRemovesTheElements
	// UnsetArraySpanLeavesOneEmptyElement replaces what the subscript names
	// with a single empty element: zsh, where `unset` of a span is the span
	// becoming one empty string rather than the subscripts going away. A
	// three-element array under `[@]` comes back holding one empty element, a
	// scalar comes back empty, and a single subscript comes back blank in
	// place with the array's length unchanged.
	UnsetArraySpanLeavesOneEmptyElement
)

func (p UnsetArraySpanPolicy) String() string {
	switch p {
	case UnsetArraySpanIsAnExpression:
		return "a subscript"
	case UnsetArraySpanRemovesTheElements:
		return "removes every element"
	case UnsetArraySpanLeavesOneEmptyElement:
		return "leaves one empty element"
	}
	return "unspecified"
}

// unsetArraySpan resolves the axis for the whole-array spelling, where an
// unanswered dialect is refused rather than guessed at: the three answers
// leave three different arrays behind.
func (r *Runner) unsetArraySpan() UnsetArraySpanPolicy {
	p := r.sem().UnsetArraySpan
	if p == UnsetArraySpanUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("`unset a[@]`")))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// unsetBlanksInPlace resolves the same axis for a single subscript, where
// there is nothing to refuse: two of the three answers remove the element and
// no answer at all means removal too.
func (r *Runner) unsetBlanksInPlace() bool {
	return r.sem().UnsetArraySpan == UnsetArraySpanLeavesOneEmptyElement
}

// EmptyReplacementPatternPolicy is what an empty pattern matches in an
// unanchored span replacement. See Semantics.EmptyReplacementPattern for the
// measurements.
type EmptyReplacementPatternPolicy int

const (
	// EmptyReplacementPatternUnspecified is no answer, and is refused: the
	// three below leave three different values behind.
	EmptyReplacementPatternUnspecified EmptyReplacementPatternPolicy = iota
	// EmptyReplacementPatternMatchesNothing declines the pattern outright,
	// so the operator is a no-op whatever the value holds: bash, in every
	// build measured and as `sh`.
	EmptyReplacementPatternMatchesNothing
	// EmptyReplacementPatternMatchesAnEmptyValue takes it only where there
	// is nothing to scan, so an empty value becomes the replacement and any
	// other value is left alone: ksh93.
	EmptyReplacementPatternMatchesAnEmptyValue
	// EmptyReplacementPatternMatchesEveryPosition treats it as the ordinary
	// pattern that matches the empty string, so it fires wherever any such
	// pattern would: zsh.
	EmptyReplacementPatternMatchesEveryPosition
)

func (p EmptyReplacementPatternPolicy) String() string {
	switch p {
	case EmptyReplacementPatternMatchesNothing:
		return "nothing"
	case EmptyReplacementPatternMatchesAnEmptyValue:
		return "an empty value"
	case EmptyReplacementPatternMatchesEveryPosition:
		return "every position"
	}
	return "unspecified"
}

// emptyReplacementPattern resolves the axis, and is reached only where the
// pattern of an unanchored span replacement is empty.
func (r *Runner) emptyReplacementPattern() EmptyReplacementPatternPolicy {
	p := r.sem().EmptyReplacementPattern
	if p == EmptyReplacementPatternUnspecified {
		r.diagf("%s\n", r.unanswered("what an empty pattern in `${v///X}` matches"))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// EmptyMatchDeclinedPolicy is which empty match a global replacement refuses.
// See Semantics.ReplacementEmptyMatchDeclined for the measurements.
type EmptyMatchDeclinedPolicy int

const (
	// EmptyMatchDeclinedUnspecified is no answer, and reads as
	// EmptyMatchDeclinedAtTheEnd after the refusal — the same shape ask()
	// has, where an unanswered axis is reported and then does not fire.
	EmptyMatchDeclinedUnspecified EmptyMatchDeclinedPolicy = iota
	// EmptyMatchDeclinedAtTheEnd refuses an empty match at the end of the
	// value reached by stepping over the last unit, and takes one adjacent
	// to the match before it: bash and zsh.
	EmptyMatchDeclinedAtTheEnd
	// EmptyMatchDeclinedAfterAMatch refuses an empty match at the position
	// the match before it ended, and takes one at the end: ksh93. The
	// classic global-replace rule, where a replacement never happens twice
	// in the same place.
	EmptyMatchDeclinedAfterAMatch
)

func (p EmptyMatchDeclinedPolicy) String() string {
	switch p {
	case EmptyMatchDeclinedAtTheEnd:
		return "at the end of the value"
	case EmptyMatchDeclinedAfterAMatch:
		return "where the match before it ended"
	}
	return "unspecified"
}

// emptyMatchDeclined resolves the axis, and is reached only at the two
// positions the two readings land differently on.
func (r *Runner) emptyMatchDeclined() EmptyMatchDeclinedPolicy {
	p := r.sem().ReplacementEmptyMatchDeclined
	if p == EmptyMatchDeclinedUnspecified {
		r.diagf("%s\n", r.unanswered("which empty match a replacement declines"))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// ReadTrailingEscapedSeparatorPolicy is what `read` does with an escaped IFS
// whitespace character closing the last name's value. See
// Semantics.ReadTrailingEscapedSeparator for the measurements.
// DescriptorAllocationBase is the number a shell counts up from when it picks
// a descriptor for itself. See Semantics.FirstAllocatedDescriptor, which is
// the only reader and which records why this type has no unanswered value.
type DescriptorAllocationBase int

const (
	// AllocateDescriptorsFromTen is bash 5.3 and ksh93, and is the zero value
	// because it is the answer this shell gave everywhere before the axis
	// existed.
	AllocateDescriptorsFromTen DescriptorAllocationBase = iota
	// AllocateDescriptorsFromEleven is zsh 5.9.2.
	AllocateDescriptorsFromEleven
)

// number is the descriptor the base stands for.
func (b DescriptorAllocationBase) number() int {
	if b == AllocateDescriptorsFromEleven {
		return 11
	}
	return 10
}

func (b DescriptorAllocationBase) String() string {
	return "from " + itoa(b.number())
}

type ReadTrailingEscapedSeparatorPolicy int

const (
	// ReadTrailingEscapedSeparatorUnspecified is no answer, and reads as
	// Kept after the refusal — the same shape ask() has, where an unanswered
	// axis is reported and then does not move the value.
	ReadTrailingEscapedSeparatorUnspecified ReadTrailingEscapedSeparatorPolicy = iota
	// ReadTrailingEscapedSeparatorKept lets the mask reach the trim, so an
	// escaped separator is data wherever it stands: dash.
	ReadTrailingEscapedSeparatorKept
	// ReadTrailingEscapedSeparatorTrimmedFromARemainder ignores the mask,
	// and only for a last name that took the *remainder* of the line — one
	// field per name leaves the field's own closing space alone: bash, in
	// all three builds measured.
	ReadTrailingEscapedSeparatorTrimmedFromARemainder
	// ReadTrailingEscapedSeparatorTrimmed ignores the mask for the last
	// name's value however it was reached, remainder or single field:
	// ksh93 and zsh.
	ReadTrailingEscapedSeparatorTrimmed
)

func (p ReadTrailingEscapedSeparatorPolicy) String() string {
	switch p {
	case ReadTrailingEscapedSeparatorKept:
		return "kept"
	case ReadTrailingEscapedSeparatorTrimmedFromARemainder:
		return "trimmed from a remainder"
	case ReadTrailingEscapedSeparatorTrimmed:
		return "trimmed"
	}
	return "unspecified"
}

// readTrailingEscapedSeparator resolves the axis, and is reached only where
// the readings would leave different values behind.
func (r *Runner) readTrailingEscapedSeparator() ReadTrailingEscapedSeparatorPolicy {
	p := r.sem().ReadTrailingEscapedSeparator
	if p == ReadTrailingEscapedSeparatorUnspecified {
		r.diagf("%s\n", r.unanswered(
			"an escaped IFS whitespace character closing a `read` value"))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// ValueBackslashPolicy is what a backslash that arrived in a value does to
// the character behind it when the field is matched as a pattern. See
// Semantics.ValueBackslashInAPattern for the measurements.
type ValueBackslashPolicy int

const (
	// ValueBackslashUnspecified is no answer, and is refused: the three
	// below match different names and restore different words.
	ValueBackslashUnspecified ValueBackslashPolicy = iota
	// ValueBackslashQuotesWhatFollows makes the backslash a quote: what
	// follows it is not a metacharacter, the backslash is not matched, and
	// it is still in the word a failed match restores. dash, bash 5.3, that
	// build as `sh`, and bash 3.2.
	ValueBackslashQuotesWhatFollows
	// ValueBackslashDisarmsWhatFollows keeps the backslash as a character of
	// the pattern and takes the metacharacter status off what follows it, so
	// the field matches what the same text written literally would. zsh,
	// through `${~spec}`.
	ValueBackslashDisarmsWhatFollows
	// ValueBackslashIsData keeps the backslash as a character and leaves
	// what follows it live. ksh93.
	ValueBackslashIsData
)

func (p ValueBackslashPolicy) String() string {
	switch p {
	case ValueBackslashQuotesWhatFollows:
		return "quotes what follows"
	case ValueBackslashDisarmsWhatFollows:
		return "disarms what follows"
	case ValueBackslashIsData:
		return "data"
	}
	return "unspecified"
}

// valueBackslashInAPattern resolves the axis, and is reached only where the
// three readings put different fields on the wire.
func (r *Runner) valueBackslashInAPattern() ValueBackslashPolicy {
	p := r.sem().ValueBackslashInAPattern
	if p == ValueBackslashUnspecified {
		r.diagf("%s\n", r.unanswered(
			"what a value's backslash does to the metacharacter behind it"))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// PrefixRefusalFatalityPolicy is what a refused assignment prefix — `x=2 cmd`
// where `x` is readonly — costs the script.
//
// Four answers rather than two, and two of them are not a property of the
// prefix at all: they are keyed on the *kind of command* the prefix stood in
// front of, and the two shells that do that draw the line in different places
// and read different words to find it.
//
// Measured 2026-09-11 from a script file, `env -i PATH=/usr/bin:/bin` with a
// scratch HOME and ZDOTDIR, with `readonly x=1` and then `x=2 <cmd>; echo
// after`:
//
//	                  bash 5.3   dash    ksh93u+        zsh 5.9.2
//	/bin/echo RAN     R, RAN, 0  fatal   R, skip, 1     R, skip, 1
//	true              R, run, 0  fatal   silent, 0      R, fatal 1
//	echo E            R, E, 0    fatal   silent, E, 0   R, fatal 1
//	command true      R, 0       fatal   silent, 0      R, skip, 1
//	command /bin/echo R, CE, 0   fatal   R, skip, 1     R, skip, 1
//	an alias for true R, 0       fatal   silent, 0      R, fatal 1
//	:                 R, 0       fatal   R, fatal 1     R, fatal 1
//	a function        R, run, 0  fatal   R, fatal 1     R, fatal 1
//
// An earlier reading of this issue held `true` fixed throughout and got
// ksh93's answer backwards for five of those rows, which is what the two
// kind-keyed values are here to prevent: `true` is one of the least
// representative command words available.
//
// One column does give up the rest of the *command list* without ending the
// script — bash invoked as `sh`, which reports and then reaches the next line
// — and there is no value for it, because no preset answers for that column:
// the `sh` invocation changes the grammar and not the semantics vector. The
// corpus records it. What made #1219 is that the same state was *ours* in
// bash and in ksh, which is what the `;` cells of that issue measured.
type PrefixRefusalFatalityPolicy int

const (
	// PrefixRefusalFatalityUnspecified is no answer, and it is refused by
	// name: one shell ends the script for every command kind, one for none,
	// and two for sets that do not contain one another.
	PrefixRefusalFatalityUnspecified PrefixRefusalFatalityPolicy = iota
	// PrefixRefusalNeverFatal reports and carries on whatever the command
	// was: bash, in both builds measured.
	PrefixRefusalNeverFatal
	// PrefixRefusalAlwaysFatal ends the script whatever the command was:
	// dash, which is uniform in the other direction.
	PrefixRefusalAlwaysFatal
	// PrefixRefusalFatalOnASpecialBuiltinOrFunction ends the script for the
	// two kinds that would have *kept* the assignment and carries on for the
	// rest: ksh93. `command` is transparent to it — what matters is the kind
	// the word resolves to, so `command /bin/echo` is the external answer and
	// `command true` the regular builtin's.
	PrefixRefusalFatalOnASpecialBuiltinOrFunction
	// PrefixRefusalFatalOnACommandThisShellRuns ends the script for every
	// kind that runs inside the shell and carries on for an external one:
	// zsh. `command` is *not* transparent there — it is read as the word that
	// was written, and a command written with it in front carries on whatever
	// it names, which is the row that separates this from the value above.
	PrefixRefusalFatalOnACommandThisShellRuns
)

func (p PrefixRefusalFatalityPolicy) String() string {
	switch p {
	case PrefixRefusalNeverFatal:
		return "never fatal"
	case PrefixRefusalAlwaysFatal:
		return "always fatal"
	case PrefixRefusalFatalOnASpecialBuiltinOrFunction:
		return "fatal on a special builtin or a function"
	case PrefixRefusalFatalOnACommandThisShellRuns:
		return "fatal on a command this shell runs"
	}
	return "unspecified"
}

// EmptyArithSubscriptPolicy is what a subscript written with nothing between
// the brackets means when an expression reads it: `$(( a[] ))`.
//
// It is reached far more often than it is written, which is why it earns an
// axis. An arithmetic expansion substitutes its parameters before it parses,
// so `$(( m[$w] ))` with `$w` unset or empty *is* `$(( m[] ))` by the time the
// expression exists — the line a widget-binding helper in a widely installed
// completion plugin runs, and the diagnostic a real startup stopped on.
//
// Measured 2026-09-10, `-c`, with the name already declared so that no other
// axis answers first:
//
//	bash 5.3, bash 3.2   `m[]: bad array subscript`, value 0, script goes on
//	ksh93u+              silent 0, script goes on
//	zsh 5.9.2            `invalid subscript`, the expression fails
//
// Three answers that part on all three of the wording, the value and whether
// the input survives, which is the definition of a conflict rather than of a
// feature one shell adds.
type EmptyArithSubscriptPolicy int

const (
	// EmptyArithSubscriptUnspecified is no answer, and it is refused by name
	// rather than guessed at: one shell prints and continues, one prints and
	// stops, one prints nothing, and no two of those can stand in for each
	// other.
	EmptyArithSubscriptUnspecified EmptyArithSubscriptPolicy = iota
	// EmptyArithSubscriptIsTheEmptyExpression reads the brackets as holding
	// an expression that happens to be empty, which is zero — so the operand
	// is the *element that subscript names* and not the number zero. ksh93,
	// where `a=(5 6 7); $(( a[] ))` is 5 rather than 0, `m[""]=4` then
	// `$(( m[] ))` is 4, and `(( a[]++ ))` steps element zero. The stream
	// stays clean and the status stays 0.
	//
	// The distinction is not pedantry: bash reports and then answers with a
	// flat zero — `a=(5 6 7); $(( a[] ))` is 0 there — so a policy worded as
	// "zero" would have given ksh93 bash's value and no probe written against
	// an unset name could have told the two apart.
	EmptyArithSubscriptIsTheEmptyExpression
	// EmptyArithSubscriptIsReported names the subscript and carries on with
	// zero: bash, in both builds measured, where `$(( m[] ))` writes
	// `m[]: bad array subscript` and still expands to 0. The real shell
	// writes that sentence twice for one subscript, which is an artifact of
	// how it evaluates rather than a fact about the construct; once is what
	// this produces.
	EmptyArithSubscriptIsReported
	// EmptyArithSubscriptIsInvalid fails the expression: zsh, where the
	// complaint is `invalid subscript` — the subscript machinery's own
	// sentence and not the expression parser's, with no `bad math
	// expression` in front of it and no name after it — and the arithmetic
	// produces no value at all.
	//
	// Only reached for a name that is already set there, because
	// ArithSubscriptSkippedWhenNameUnset answers first for one that is not.
	EmptyArithSubscriptIsInvalid
)

func (p EmptyArithSubscriptPolicy) String() string {
	switch p {
	case EmptyArithSubscriptIsTheEmptyExpression:
		return "the empty expression"
	case EmptyArithSubscriptIsReported:
		return "reported, and zero"
	case EmptyArithSubscriptIsInvalid:
		return "an invalid subscript"
	}
	return "unspecified"
}

// SubscriptedArrayLiteralPolicy is what an array literal means when a
// subscript names one element to put it in: `a[2]=(p q)`.
//
// Three shells with arrays, three answers, and none of them is a weakening of
// another — which is what makes this a semantics axis rather than a grammar
// flag. The syntax is identical in all three and only the meaning parts.
type SubscriptedArrayLiteralPolicy int

const (
	// SubscriptedArrayLiteralUnspecified is no answer, and it is refused by
	// name rather than guessed at. There is nothing to fall back on: the two
	// answers that exist leave arrays of different lengths, and the third
	// shell in the panel builds a nested value this interpreter has no
	// representation for at all.
	SubscriptedArrayLiteralUnspecified SubscriptedArrayLiteralPolicy = iota
	// SubscriptedArrayLiteralRefused reports the line and ends the script:
	// bash, in both builds measured, where `a[1]=(p q)` is `a[1]: cannot
	// assign list to array member` at status 1 and the rest of the command
	// string does not run. The refusal does not depend on what the name
	// holds — an array, a scalar, a declared table and an unset name are all
	// refused with the same sentence and the subscript quoted as written.
	SubscriptedArrayLiteralRefused
	// SubscriptedArrayLiteralSplices replaces the element with the words:
	// zsh, where the array's *length* changes by the literal's count less
	// one. `i=1; a=(x y); a[$i]=(p q)` reads back `p q y`, `a[$i]=()` removes
	// the element, and `a[$i]+=(p)` appends at the element rather than at the
	// end. A subscript past the last element pads with empties on the way, a
	// scalar name is refused as a non-array and a declared table is refused
	// as a slice.
	SubscriptedArrayLiteralSplices
)

func (p SubscriptedArrayLiteralPolicy) String() string {
	switch p {
	case SubscriptedArrayLiteralRefused:
		return "refused"
	case SubscriptedArrayLiteralSplices:
		return "splices the element"
	}
	return "unspecified"
}

// subscriptedArrayLiteral resolves the axis, refusing an unanswered dialect by
// name rather than picking one of the two answers: they disagree about the
// array's length, its contents and the exit status, so there is no reading
// that is nearly right.
func (r *Runner) subscriptedArrayLiteral() SubscriptedArrayLiteralPolicy {
	p := r.sem().SubscriptedArrayLiteral
	if p == SubscriptedArrayLiteralUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("`a[i]=(p q)`, an array literal through a subscript")))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// ask reads one axis.
//
// An unspecified axis is refused rather than guessed, and the refusal names
// it, because "this script depends on something the shells disagree about"
// is a useful thing to be told and a silent wrong answer is not.
//
// Callers consult an axis only when the input actually depends on it — `echo
// hi` does not ask about escapes and `echo 'a\tb'` does — which is what keeps
// the core usable rather than refusing everything.
// caretNegates resolves the `[^…]` axis, and only for a pattern that actually
// uses it — so a dialect is never questioned about syntax the pattern does not
// contain, and `[abc]` needs no answer from anyone.
func (r *Runner) caretNegates(pattern string) bool {
	if !strings.Contains(pattern, "[^") {
		return false
	}
	return r.ask(r.sem().BracketCaretNegates, "`^` negating a bracket class")
}

// matchPatternR is matchPattern with the caret axis resolved from the dialect.
// condition says the pattern stands inside `[[ ]]`, which one dialect reads by
// different rules from a `case` pattern.
func (r *Runner) matchPatternR(pattern, s string, condition bool) bool {
	o := patternOpts{
		caret:        r.caretNegates(pattern),
		group:        r.dialect().PatternAlternation,
		topGroup:     r.dialect().PatternTopLevelAlternation,
		quantified:   r.readsQuantifiedGroups(condition),
		numericRange: r.dialect().NumericRangePattern,
		// The run-time option folds exactly the two consumers this function
		// serves — `case` and `[[ ]]` — and neither of the others: pathname
		// expansion has a fold of its own, and parameter expansion stays
		// exact. Which is why the fold sits here and not in patternOpts.
		fold:    r.MatchOption(MatchFoldsCase),
		chars:   r.patternCountsCharacters(pattern, s),
		escapes: r.sem().PatternEscapeReaches,
		classes: r.patternClasses(pattern),
	}
	// The status a rejected pattern exits with is the surface's, and the two
	// this function serves do not agree: measured, `[[ x == (#Z)a ]]` exits 2
	// and the same pattern in a `case` exits 0.
	badStatus := 0
	if condition {
		badStatus = 2
	}
	o = r.extendedPatternOpts(o, pattern, badStatus)
	var bad bool
	if hasUnterminatedBracket(pattern) {
		o.bracket, o.bad = r.bracketPolicy(), &bad
	}
	matched, report := matchPatternIn(pattern, s, s, 0, o)
	if bad {
		// zsh abandons the script rather than failing the match.
		// Measured: zsh abandons the script here with status 0, and with 1
		// when the same pattern fails against the filesystem. Both are
		// zsh's, and neither is guessable from the other.
		r.fatalPattern(pattern, 0)
		return false
	}
	// The surfaces this function serves are the ones that report a match
	// into `$match` and `$MATCH` — a condition, a `case`, the element
	// filters and an `(r)` subscript. Pathname expansion is deliberately
	// not among them: measured, `print -rl -- (#b)(a)*` leaves `$match`
	// untouched, so the walk goes through matchPattern instead.
	if matched {
		r.publishMatch(report)
	}
	return matched
}

// fatalPattern reports a pattern the dialect rejects outright.
func (r *Runner) fatalPattern(pattern string, status int) {
	r.diagf("%s\n", Wording(r.diag().BadPattern, "bad pattern: %s", pattern))
	r.status = status
	r.ctl = controlExit
}

// OutsideLocaleEscapePolicy is what a `\u` or `\U` escape does when the code
// point it names cannot be represented in the locale's encoding.
//
// Three answers and not a bool, for the reason ReadonlyElementPolicy is not
// one: no answer is the negation of another, and a field named for one of
// them would read as `false` meaning another by accident. The third is
// ksh93's, which never consults a locale at all, and it is reachable only at
// the two sites ksh93 reads the escape at — a `printf` format and `$'…'`.
//
// The escape's reading is not in question here — see
// Semantics.EchoExpandsUnicodeEscapes and the printf policies for that. This
// is only what happens to a value the locale has no room for, and the shells
// that have the escape split on it. interp/localeescape.go holds the
// measurements.
type OutsideLocaleEscapePolicy uint8

const (
	// OutsideLocaleEscapeUnspecified is no answer, and is refused like any
	// other.
	OutsideLocaleEscapeUnspecified OutsideLocaleEscapePolicy = iota
	// OutsideLocaleEscapeWritten leaves the escape standing, normalized to
	// four or eight upper-case digits — `\ue9` and `\U000000e9` both stand as
	// `\u00E9` — and the command carries on: bash 5.3.
	OutsideLocaleEscapeWritten
	// OutsideLocaleEscapeRefused reports `character not in range`, writes
	// what came before the escape and nothing after it, and abandons the
	// script with status **0**: zsh. Measured, and the pair that says so is
	// `(exit 3); echo '...'`, which also exits 0 — so it is zero rather than
	// whatever was already there.
	OutsideLocaleEscapeRefused
	// OutsideLocaleEscapeEncoded writes the character whatever the locale
	// says, which is to say it never consults one: ksh93. Measured 2026-09-11
	// under `LC_ALL=C`, `printf 'a\u00e9Z'` and `$'a\u00e9Z'` both giving
	// `61 c3 a9 5a` there and in a UTF-8 locale alike — the answer this shell
	// gave everywhere before the axis existed, and the third answer that
	// makes this a policy with two shells to tell apart rather than one.
	OutsideLocaleEscapeEncoded
)

func (p OutsideLocaleEscapePolicy) String() string {
	switch p {
	case OutsideLocaleEscapeWritten:
		return "the escape stands as written"
	case OutsideLocaleEscapeRefused:
		return "refused, and the script is abandoned"
	case OutsideLocaleEscapeEncoded:
		return "the character is written, the locale unread"
	}
	return "unspecified"
}

// outsideLocaleEscape resolves the axis, and only for an escape that actually
// names a code point this locale cannot hold.
func (r *Runner) outsideLocaleEscape() OutsideLocaleEscapePolicy {
	p := r.sem().UnicodeEscapeOutsideTheLocale
	if p == OutsideLocaleEscapeUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered(`a \u escape outside the locale`)))
		r.status = 2
		r.ctl = controlExit
	}
	return p
}

// CompoundPipelineStatusPolicy is what a compound command does to the record
// of the last pipeline's statuses.
//
// Two answers and not a bool, for the reason ReadonlyElementPolicy is not one:
// they are two mechanisms rather than two values of one rule, and the third
// reading — a compound writing its own status for having run, whatever its
// body holds and whatever ran inside it — is what this shell used to do and
// what no panel member does. A field named for one of the two would read as
// `false` meaning the other by accident, and it would leave nowhere for that
// third reading to be ruled out.
//
// Only the two shells that keep such a record reach it: ksh93 and dash have no
// name for it, so the axis is never asked there. interp/pipestatus.go holds
// the mechanics and Semantics.CompoundPipelineStatusRecord the measurements.
type CompoundPipelineStatusPolicy uint8

const (
	// CompoundPipelineStatusUnspecified is no answer, and is refused like any
	// other.
	CompoundPipelineStatusUnspecified CompoundPipelineStatusPolicy = iota
	// CompoundPipelineStatusFromTheBody writes the compound's own status
	// where its **body** holds anything that would write the record standing
	// alone, and leaves the record alone otherwise — a question about the
	// parse, answered without running any of it: zsh.
	CompoundPipelineStatusFromTheBody
	// CompoundPipelineStatusFromWhatRan writes nothing for the compound at
	// all, so the record is whatever the last pipeline that actually ran
	// inside it left — and a compound that ran nothing leaves the record from
	// before it: bash.
	CompoundPipelineStatusFromWhatRan
)

func (p CompoundPipelineStatusPolicy) String() string {
	switch p {
	case CompoundPipelineStatusFromTheBody:
		return "the body decides, without being run"
	case CompoundPipelineStatusFromWhatRan:
		return "left to whatever ran inside it"
	}
	return "unspecified"
}

// compoundPipelineStatus resolves the axis, and reports it unanswered the way
// ask does for a two-valued one.
func (r *Runner) compoundPipelineStatus() CompoundPipelineStatusPolicy {
	p := r.sem().CompoundPipelineStatusRecord
	if p == CompoundPipelineStatusUnspecified {
		r.diagf("%s\n", r.unanswered("what a compound does to the pipeline status"))
		r.status = 2
		r.unspecified = true
	}
	return p
}

// ShiftOptionWordPolicy is which leading-`-` words `shift` reads as options.
//
// ReadonlyElementPolicy is what a declaration does to a subscripted operand it
// would also have to freeze.
//
// Three answers and not a bool, and only two of them modeled: see
// Semantics.ReadonlyElement for the third and for what is measured.
type ReadonlyElementPolicy uint8

const (
	// ReadonlyElementUnspecified is no answer, and is refused like any other.
	ReadonlyElementUnspecified ReadonlyElementPolicy = iota
	// ReadonlyElementWritten writes the element and freezes the array over
	// it, so `readonly a[1]=v` leaves `a` holding v and immutable: ksh93.
	ReadonlyElementWritten
	// ReadonlyElementRefused refuses the operand and ends the script,
	// because an element cannot carry the attribute a name carries: zsh.
	ReadonlyElementRefused
)

func (p ReadonlyElementPolicy) String() string {
	switch p {
	case ReadonlyElementWritten:
		return "the element is written and the array frozen over it"
	case ReadonlyElementRefused:
		return "the operand is refused"
	}
	return "unspecified"
}

// Three answers and not a bool, because the panel splits on *which* dash
// words rather than on whether there are any: zsh refuses `-x` as an option
// it does not have and reads `-1` as a count, so it is neither of the two
// answers a bool could give.
type ShiftOptionWordPolicy int

const (
	// ShiftOptionWordsUnspecified is no answer, and is refused like any
	// other.
	ShiftOptionWordsUnspecified ShiftOptionWordPolicy = iota
	// ShiftOptionWordsNone reads every dash word as the count, so `shift -x`
	// complains about a number: bash, dash.
	ShiftOptionWordsNone
	// ShiftOptionWordsNonNumeric reads a dash word as an option unless what
	// follows the dash is all digits, so `shift -x` is an option and
	// `shift -1` is a count: zsh.
	ShiftOptionWordsNonNumeric
	// ShiftOptionWordsAny reads every dash word as an option, digits and
	// all, so `shift -1` and `shift -0` are both refused as options: ksh93.
	ShiftOptionWordsAny
)

func (p ShiftOptionWordPolicy) String() string {
	switch p {
	case ShiftOptionWordsNone:
		return "none: a dash word is the count"
	case ShiftOptionWordsNonNumeric:
		return "an option unless it is all digits"
	case ShiftOptionWordsAny:
		return "every dash word is an option"
	}
	return "unspecified"
}

// shiftOptionWords resolves the axis, and only for a word that begins with a
// `-` and is neither a lone dash nor the end-of-options marker.
func (r *Runner) shiftOptionWords() ShiftOptionWordPolicy {
	p := r.sem().ShiftOptionWords
	if p == ShiftOptionWordsUnspecified {
		r.diagf("%s\n", r.unanswered("`shift -x` read as an option rather than as a count"))
		r.status = 2
		r.unspecified = true
	}
	return p
}

func (r *Runner) ask(a Answer, axis string) bool {
	switch a {
	case Yes:
		return true
	case No:
		return false
	}
	r.diagf("%s\n", r.unanswered(axis))
	r.status = 2
	r.unspecified = true
	return false
}
