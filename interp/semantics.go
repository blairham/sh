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

// Semantics is where implementations disagree about what identical syntax
// *means*.
//
// This is the structure docs/spec/semantics.md argues for, and the argument is
// worth restating because the obvious alternative looks fine. Grammar
// differences are additive — a construct either parses or it does not — and
// [syntax.Dialect] models those. Semantic differences are conflicts: the same
// text means different things, and no amount of adding or removing features
// produces one from another. They need switches.
//
// Every field is named for the behavior rather than for the implementation
// that wants it. A name-shaped field could not be given a value in the first
// place: one implementation accepts `&>` or refuses it depending on which
// build is installed, twelve years apart under the same name, so the behavior
// is the only thing stable enough to name.
//
// The axes produce groupings that overlap and contradict — no ordering of the
// implementations explains the data, which is why this is a vector and not a
// level.
//
// This package defines the questions and never the answers. A comment here
// says what an axis decides, what each value means, where it is asked and what
// keeps it from being asked elsewhere. Which preset answers it which way, and
// the measurement behind that answer, are recorded in docs/spec/semantics.md
// and held in the preset itself.
type Semantics struct {
	// SplitParamExpansion field-splits the result of an unquoted parameter
	// expansion.
	//
	// Narrower than "word splitting", and paired with
	// SplitCommandSubstitution: an implementation may split an unquoted
	// command substitution while leaving a parameter expansion whole, so this
	// is two axes and not one.
	SplitParamExpansion Answer
	// SplitCommandSubstitution field-splits an unquoted command substitution.
	//
	// The content of this axis is the *contrast* with SplitParamExpansion
	// rather than a split of its own: every preset answers it the same way,
	// and it exists so that the one axis they differ on cannot be read as
	// "this implementation does not word-split at all". Fold the two together
	// and that misreading is the only one left.
	//
	// So this axis is asked rather than read. An implementation that did
	// leave an unquoted substitution unsplit has somewhere to say so, and
	// until one does, No is the other side of a binary [Answer] rather than a
	// value any preset holds.
	SplitCommandSubstitution Answer
	// UnquotedListJoinsOnIFS makes an unquoted list expansion one string —
	// the elements joined on the first character of IFS — before the split
	// above runs on it, rather than splitting each element on its own.
	//
	// One question wearing two faces, and the reason it is a question at all
	// is that both faces were being answered without asking: `$@` and
	// `${a[@]}` never joined, and `$*` and `${a[*]}` always did, each of which
	// is right for some presets and wrong for the rest.
	//
	// The join is what decides the fate of an *empty* element, which is where
	// it shows. Under a non-whitespace IFS, joining makes `set -- x "" y` into
	// `x::y`, which splits back to three fields; splitting each element on its
	// own drops the empty one and leaves two. The same join is why joining
	// then *loses* a trailing empty element — `set -- x y ""` is `x:y:`, and a
	// trailing separator makes no field — where not joining keeps it. No
	// arrangement of the splitting answer alone reaches either reading, which
	// is why this is an axis of its own.
	//
	// Asked only at the disagreement: the two readings coincide under a
	// whitespace IFS, which is why `a=("" x)` is `[x]` under every answer and
	// needs none, and there is nothing to join with when IFS is set and empty.
	// See Runner.elementFields, which computes both and asks only when they
	// differ.
	//
	// The *quoted* spellings are not this question and must not reach it:
	// `"$*"` and `"${a[*]}"` join on the first character of IFS, and `"$@"`
	// and `"${a[@]}"` keep one field per element. Both are unanimous, so both
	// are core.
	UnquotedListJoinsOnIFS Answer

	// UnsplitAtListJoinsOnIFS decides the character an unquoted list spelled
	// `@` is joined with when it reaches a context that keeps no fields: the
	// first character of IFS, or a hard space.
	//
	// The four contexts that never split — an assignment's value, a `case`
	// subject, a `[[ ]]` operand and a here-document body — answer this alike
	// within any one preset, which is what makes it one axis rather than one
	// per context.
	//
	// The `*` spelling is **not** this question and must not reach it. `$*`,
	// `${a[*]}` and a range subscript join on the first character of IFS under
	// every answer, so that half is core — see Runner.unsplitJoinSeparator,
	// which answers the star before it asks.
	//
	// Asked only at the disagreement, and the guard is not the one
	// UnquotedListJoinsOnIFS uses. Here the join happens either way and only
	// its character is in question, so an IFS that is *set and empty* is a
	// live answer rather than a reason not to ask: joining with nothing is a
	// real answer, and `IFS=""; a=(x y); v=$a` separates the two readings as
	// `xy` against `x y`. What does make them coincide is an IFS whose first
	// character is already a space — which is every script that leaves IFS
	// alone, and the reason this is silent — and a list of fewer than two
	// elements, which uses no separator at all.
	UnsplitAtListJoinsOnIFS Answer

	// TrailingSeparatorEndsAField makes the non-whitespace IFS separator that
	// closes a value open one last empty field, rather than being absorbed.
	//
	// A *leading* separator opens a field under every answer, so the asymmetry
	// is at the tail alone and this axis is the whole of it. POSIX.1-2024
	// 2.6.5 settles it for the standard — "once the input is empty, the
	// candidate shall become an output field if and only if it is not empty" —
	// which is the absorbing reading, so PosixSemantics says No.
	//
	// Asked only at the disagreement, and the guard is what keeps it off every
	// ordinary script: it is the closing *run* of separators that decides, and
	// only a non-whitespace one in that run makes the two readings differ.
	// `' a '` under the default IFS is one field either way, because
	// whitespace is absorbed at both ends under both answers; `'a: '` with
	// `IFS=' :'` is the case that shows it is the run rather than the last
	// byte, since the trailing space does not hide the colon in front of it.
	// An escaped separator is data and not part of the run at all, which is
	// why the mask `read` carries has to reach this question: `read -A` on
	// `a\:` is the one field `a:` where `a:` unescaped is two.
	//
	// It is not [Semantics] alone that answers a split — an unquoted
	// `${=spec}` keeps the fields at *both* edges unconditionally, which is a
	// different rule and reaches the splitter as its own parameter. Where
	// that rule is in force this question is never asked, because the field
	// behind the last separator is already there.
	TrailingSeparatorEndsAField Answer

	// ReadTrailingWhitespaceEndsAField is the same question about a closing run
	// of IFS *whitespace*, and it is asked of `read` alone because the two can
	// be answered differently.
	//
	// It cannot ride on TrailingSeparatorEndsAField: that one is measured on an
	// expansion, and an implementation that opens a field on a closing
	// whitespace run in `read` may still give `x=' a '; set -- ${=x}` one
	// field. Folding them would make `${=x}` grow a field it does not have.
	//
	// A *leading* run of whitespace is still absorbed, so the asymmetry is at
	// the tail here as it is above, and the run rather than the byte decides:
	// `a::` with `IFS=:` is three elements and not four.
	//
	// Only the array target can see it. With a list of names the last name
	// takes the remainder of the *line* and the closing whitespace comes off
	// that remainder anyway, so `read x <<< 'a  '` is `a` whichever way this is
	// answered — which is why it is asked in `read`'s splitting rather than at
	// the array store, and observed there.
	ReadTrailingWhitespaceEndsAField Answer

	// ReadNoFieldsIsOneEmptyElement leaves an array `read` filled from a line
	// that split into nothing at all holding one empty element rather than
	// none. A line of nothing but IFS whitespace answers the same way.
	//
	// No is the ordinary field split — no text, no fields — which is what the
	// POSIX preset takes, since the letter is not in the standard at all.
	//
	// Asked after ReadTrailingWhitespaceEndsAField and only where nothing is
	// left: an implementation that opens a field on a closing whitespace run
	// already has one by then, and asking before it would give that
	// implementation two elements for a line of spaces where it gives one.
	ReadNoFieldsIsOneEmptyElement Answer

	// ReadTrailingEscapedSeparator is what `read` does with an IFS
	// *whitespace* character the line escaped at the very end of the value its
	// last name takes. Without `-r` a backslash makes the character after it
	// data, and `read` carries that as a mask into the splitter; whether that
	// mask reaches the trim at the tail is the question, and it takes three
	// values rather than two.
	//
	// The third value is needed because one reading trims the escaped
	// character only from a value that took a *remainder* — the last name
	// swallowing more fields than it was given — and keeps it when the line
	// held exactly one field per name. A line whose escape joins two fields
	// into one, `a b\\ c\\ ` read into two names, is what separates that
	// reading from the one that trims however the value was reached.
	//
	// Two questions this axis does *not* have to carry, both unanimous:
	//
	//   - a **non-whitespace** separator. The trim only ever takes whitespace,
	//     so the mask cannot be seen through it: with `IFS=:`, an escaped `\\:`
	//     at the tail lands exactly where an unescaped one does.
	//   - a non-default **whitespace** IFS. With `IFS` a tab and tabs for
	//     separators the readings split as they do under the default, so the
	//     answer is about the trim and not about which character it trims.
	//
	// Asked where the readings land differently and nowhere else: a value whose
	// closing IFS whitespace was escaped. `-r` never reaches it, because there
	// is no mask for the trim to disagree about.
	ReadTrailingEscapedSeparator ReadTrailingEscapedSeparatorPolicy

	// GlobExpansionResults matches the *result* of an expansion against the
	// filesystem, rather than expanding only a pattern written literally in the
	// source. The same rule decides whether `[[ abc == $p ]]` treats `$p` as a
	// pattern, which is one behavior observed twice rather than two quirks.
	//
	// It is the one axis in this vector with a *run-time* name over it: where
	// the answer is No a script can say otherwise through a shell option, so
	// the preset moves this answer rather than carrying a bit of its own. The
	// per-expansion spelling `${~spec}` overrides it for one expansion and is
	// not an option — see interp/tildeflag.go, where the two meet.
	GlobExpansionResults Answer
	// ValueBackslashInAPattern is what a backslash that arrived in a **value**
	// does to the character behind it when the field is then matched as a
	// pattern. Three readings, and no two of them can stand in for each other:
	//
	//	quoting   the backslash quotes the next character and is **not itself
	//	          matched**, so a value `a\\b*` matches as the pattern `ab*`
	//	data      the backslash is data and the character behind it stays live,
	//	          so `a\\b*` matches a name with a backslash in it and globs
	//	disarmed  the backslash is data and the character behind it is
	//	          **disarmed**, the same match a wholly literal `a\\b*` makes
	//
	// It takes a pattern with a live metacharacter *behind* the backslash to
	// tell the three apart. A value whose only backslash disarms a `*` cannot
	// separate quoting from disarmed, because a quoted `*` and a disarmed one
	// both leave nothing to glob.
	//
	// No live reading removes the backslash from the *text*: no quote removal
	// is performed on the result of an expansion, so a failed match restores
	// the word with the backslash still in it.
	//
	// A **doubled** backslash is the case that says a quoting backslash needs a
	// symbol of its own rather than the marked-backslash-plus-marked-character
	// an escaped form already had. Under the quoting reading the first
	// backslash quotes the second and vanishes from the pattern; under the
	// other two both survive. The same characters written *literally* match
	// alike under all three, so the two provenances must be told apart and one
	// alphabet cannot carry both.
	//
	// **Asked only where the three encodings put different fields on the
	// wire**, and only where the result of an expansion is globbed at all.
	// valueBackslashReadingsDiffer computes that rather than describing it: a
	// value with a backslash but nothing live beside it encodes three ways and
	// restores one text, which is why `v='a\\b'; echo $v` demands no answer.
	// GlobExpansionResults is *read* rather than asked for the same reason —
	// where nothing is globbed the three readings agree — and answering it No
	// is not the same as having no reading here: `${~spec}` globs one expansion
	// regardless, and it takes the disarmed reading.
	ValueBackslashInAPattern ValueBackslashPolicy

	// GlobNoMatchIsError makes a pattern matching nothing an error instead of
	// passing the pattern through unchanged.
	GlobNoMatchIsError Answer

	// AssignmentPrefixPersistsOnSpecialBuiltin keeps `x=1 shift` set
	// afterwards. POSIX requires it, and the presets part over whether they
	// follow the standard here.
	AssignmentPrefixPersistsOnSpecialBuiltin Answer

	// PrefixToARegularBuiltinIsRefused applies the readonly refusal to an
	// assignment written in front of a *regular builtin* — `readonly x=1;
	// x=2 true`. Answering No says nothing at all, runs the builtin and
	// reports 0, and does the same for an alias naming a regular builtin and
	// for `command` naming one.
	//
	// Asked only in front of a regular builtin, which is the only position the
	// answers part at: an external command, a special builtin and a function
	// are refused under every answer.
	PrefixToARegularBuiltinIsRefused Answer
	// PrefixRefusalFatality is what a *reported* refusal of an assignment
	// prefix costs the script. Four answers, two of them keyed on the kind of
	// command the prefix stood in front of and keyed on different lines. See
	// PrefixRefusalFatalityPolicy.
	PrefixRefusalFatality PrefixRefusalFatalityPolicy
	// PrefixRefusalCostsTheCommand leaves the command the prefix stood in front
	// of unrun, at status 1. Answering No reports the refusal, runs the command
	// with the name still holding its old value, and reports 0.
	//
	// Asked only where the refusal is reported and is not fatal, which is the
	// only place the two readings differ: a script that ends has not run the
	// command either.
	//
	// A separate axis from the fatality because the two are independent in both
	// directions — an implementation may be fatal on a function and not on an
	// external while skipping the command in both non-fatal positions, or fatal
	// nowhere and skipping nothing.
	PrefixRefusalCostsTheCommand Answer

	// PrefixToAFrozenNameIsCheckedFirst refuses the prefix **before** the
	// command's values are expanded and before its redirections are opened.
	// `No` does the other things first, so a value that will not expand and a
	// file that will not open each report on their own and the frozen name is
	// never mentioned.
	//
	// Two independent probes agree on the one boundary, which is what makes it
	// a boundary rather than a quirk of arithmetic: a prefix whose value cannot
	// be evaluated, and a redirection that cannot be opened, reach the same
	// order with no expression involved in the second. The first also says the
	// value is not merely reported later but **never evaluated** — where the
	// check comes first, the evaluation is silent.
	//
	// The other three prefix axes are asked at the same point and for the same
	// reason: a command with a prefix is a minority of a script's lines and one
	// with a frozen name in the prefix is a minority of those, so four
	// questions sit off the common path entirely. This one is asked there too,
	// once a name in the prefix is actually frozen.
	//
	// It is the order and not the refusal. What a reported refusal costs is
	// PrefixRefusalCostsTheCommand and PrefixRefusalFatality, and those are
	// answered the same way whichever order the two happen in.
	PrefixToAFrozenNameIsCheckedFirst Answer

	// EchoOptions is the set of letters `echo` reads as options. Empty means
	// `n` alone.
	//
	// A word carrying any letter outside the set is not an option at all — the
	// whole word becomes an operand, which is unanimous and is why `echo -nq
	// hi` prints `-nq hi` under every answer.
	EchoOptions string
	// EchoLastEscapeFlagWins decides `echo -e -E`: the last flag wins and the
	// backslashes are printed, rather than `-e` winning whatever the order.
	//
	// Reached only when `-e` came first — the other order agrees under both
	// answers — and only where EchoOptions holds both letters.
	EchoLastEscapeFlagWins Answer
	// EchoExpandsHexEscapes admits `\xHH` alongside the XSI set, rather than
	// printing it as written.
	EchoExpandsHexEscapes Answer
	// EchoExpandsEscEscape admits `\e` for the escape character in an `echo`
	// argument, rather than writing the two characters.
	//
	// A separate axis from EchoExpandsCapitalEscEscape below because the
	// implementations that split the two letters split them in *opposite*
	// directions, so no single answer describes either one. It is the same
	// asymmetry the `%b` site has, and it is asked separately there: see
	// PrintfBEscEscape.
	//
	// Asked only where an `echo` argument actually carries a `\e`.
	EchoExpandsEscEscape Answer
	// EchoExpandsCapitalEscEscape admits `\E` in an `echo` argument, rather
	// than writing the two characters. See EchoExpandsEscEscape for why the two
	// letters are two questions.
	//
	// Asked only where an `echo` argument actually carries a `\E`.
	EchoExpandsCapitalEscEscape Answer
	// EchoExpandsUnicodeEscapes admits `\uHHHH` and `\UHHHHHHHH` in an `echo`
	// argument, each read as a code point and written in UTF-8, rather than
	// writing the characters as they stand.
	//
	// One axis for both letters, unlike `\e` and `\E` above, because the
	// answers do not split them: an implementation that reads one reads the
	// other, and with the same rules — at most four hex digits after `\u` and
	// at most eight after `\U`, fewer accepted (`\u41` is `A`), and the value
	// written as UTF-8 rather than as a byte, so `\u00e9` is two bytes and
	// `\u20ac` three.
	//
	// It is the *original* UTF-8 and not the range it was later narrowed to,
	// which is measured rather than assumed: a surrogate and a value past the
	// last code point are encoded rather than replaced — `\ud800` is three
	// bytes and `\U110000` four — and the five- and six-byte forms are
	// reachable, `\U200000` being five and `\U4000000` six.
	//
	// Asked only where an `echo` argument actually carries one.
	EchoExpandsUnicodeEscapes Answer
	// UnicodeEscapeOutsideTheLocale is what becomes of a `\u` or `\U` escape
	// naming a code point the locale's encoding cannot hold — see
	// OutsideLocaleEscapePolicy, and interp/localeescape.go for what "cannot
	// hold" is read off.
	//
	// One axis for every site that reads the escape — `echo`, `print`, a
	// `printf` format, a `%b` argument, `$'...'`, and the `(g)` and `(p)`
	// expansion flags — because an implementation answers the same at every
	// site it reads the escape at. It is a question *after* the escape has been
	// read, so it is separate from EchoExpandsUnicodeEscapes above and from the
	// printf policies: an implementation that does not read the escape at a
	// site never reaches this axis there, so how many sites can reach it varies
	// with the answers above.
	//
	// `$'...'` is a **core** construct, and the axis is three-valued because of
	// it. An implementation may read the escape at only some of those sites and
	// write the character regardless of the locale where it does, which is
	// neither of the two straight answers. So a core script with `$'\u00e9'`
	// in it under a non-UTF-8 locale is an unanswered axis rather than a value
	// — the core refusing what is disagreed about, in the one place that
	// disagreement reaches the common denominator.
	//
	// Asked only where such an escape actually names a code point the locale
	// refuses, so an ASCII one needs no answer from anybody and neither does
	// any escape at all in a UTF-8 locale.
	UnicodeEscapeOutsideTheLocale OutsideLocaleEscapePolicy
	// EchoEmptyHexDigitRunIsNul reads a hexadecimal escape with no digit after
	// it as a zero rather than leaving it as written, so `echo '\xZ'`,
	// `echo '\uZ'` and `echo '\x'` are a NUL byte followed by whatever was
	// there.
	//
	// One question for `\x`, `\u` and `\U` together, because an
	// implementation that answers it one way answers the same for all three. It
	// is the same split PrintfHexEscapePolicy records at the two `printf`
	// sites, where it is one of the three details that made a policy out of a
	// bool.
	//
	// Asked only where such an escape actually runs out of digits, so
	// `echo '\x41'` needs no answer to it.
	EchoEmptyHexDigitRunIsNul Answer
	// EchoInterpretsEscapes expands backslash escapes in `echo` without `-e`.
	// The answers group in a way no other axis reproduces, which is why it
	// cannot ride on any of them.
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
	// DollarSingleNulTruncates ends the decoded text at the first NUL an escape
	// produces, which is C-string semantics: `$'a\0b'` is `a` rather than the
	// three bytes `a`, NUL, `b`.
	//
	// The truncation is the *span's*, not the word's: `$'a\0b'ccc` is `accc`
	// where it truncates, so what is lost is the remainder of the quoted text
	// and nothing else. Reached only where a decoded escape actually yields a
	// zero byte — `\0`, an octal or hex escape that comes to zero, and `\c@`,
	// which is the same zero by another road.
	DollarSingleNulTruncates Answer
	// DollarSingleCaretMeta reads `\C-X` inside `$'…'` as a control character
	// and `\M-X` as the same byte with the high bit set. The separating `-` is
	// optional in both, so `\CA` and `\C-A` are one byte apiece, and either may
	// take the other as its argument.
	//
	// Three-valued rather than two, because an implementation may give `\C` a
	// different meaning of its own — a control escape taking no separator,
	// which reads `$'\C-A'` as control-`-` followed by `A` — and that is a
	// different reading rather than this one turned off. Refusing it there is
	// the point: answering `$'\C-A'` as `C-A` would be off by a byte and
	// silent about it.
	//
	// Asked only for a `$'…'` that has a `\C` or an `\M` in it. This is the
	// reading side of what the `q+` expansion flag writes, and the two are the
	// same table seen from its two ends: a `q+` whose spelling cannot be read
	// back is not a quoting flag at all.
	DollarSingleCaretMeta Answer

	// ReadOptions is the set of letters `read` takes, a `:` after a letter
	// marking one whose argument follows it — the getopts convention, the same
	// one the shared option reader speaks. Empty means `r`, the one letter
	// POSIX gives the builtin.
	//
	// The letters belong to the preset, and so does their *shape*: the same
	// capability is spelled with different letters, and a letter that takes an
	// argument in one preset is a bare flag in another. The array option is
	// spelled one way with the array's name as the option's argument and
	// another with the name as the first operand; `-n` is a flag in one preset
	// and a count in others; `-p` takes a prompt for the terminal in some and
	// names the coprocess as the source in others. That is why this is a string
	// of letters rather than a set of booleans.
	ReadOptions string
	// UnsetOptions is the same question asked of `unset`, spelled the same way.
	// Empty means `vf`, which is what POSIX gives the builtin.
	//
	// The letters split three ways and no two presets have the same set: `-v`
	// and `-f` are unanimous, `-n` is held by some and refused by the rest, and
	// `-m`, which reads its operands as *patterns* and unsets every parameter
	// whose name matches one, is held by a single preset.
	UnsetOptions string
	// ReadZeroTimeout is what `read -t 0` asks of the stream — a poll, a read
	// of what is already waiting, or a read that commits once it has begun.
	// Asked only where `-t 0` is actually written; every other timeout is a
	// deadline and needs no answer. See ReadZeroTimeoutStyle.
	ReadZeroTimeout ReadZeroTimeoutStyle
	// ReadPartialCountSucceeds decides `read -n N` when the input ends after
	// some but fewer than N characters: the read is a success rather than a
	// report of 1. What arrived is kept under either answer.
	//
	// Asked only there — a full count, a delimiter, or a wholly empty input
	// answers the same way under both.
	ReadPartialCountSucceeds Answer
	// ReadExactCountKeepsPartial decides what `read -N N` leaves behind when
	// the input ends short: the partial text is assigned rather than nothing.
	// The status is 1 under either answer. Asked only on that partial text.
	ReadExactCountKeepsPartial Answer
	// ReadTimeoutKeepsWhatArrived decides what an expired `read -t` leaves
	// behind: whatever had arrived before the deadline is assigned, rather than
	// no name being touched and the variable keeping its earlier value. Asked
	// only on the timeout, never at end of input, where both answers assign.
	//
	// The distinction the wording is careful about is that Yes does not
	// *clear* the variable — it assigns a short read, and clearing is only what
	// that looks like when nothing had arrived.
	//
	// The axis is worded around the timeout rather than around the partial text
	// because of ReadTimeoutBoundsReadability below. Where `-t` bounds only the
	// wait for the stream to become readable, a timeout can only ever happen
	// with nothing to assign, which is the same observable as leaving the name
	// alone; the two are told apart by how long the read takes, not by what it
	// assigns.
	ReadTimeoutKeepsWhatArrived Answer

	// ReadTimeoutBoundsReadability makes `read -t` bound the wait for the
	// stream to become *readable* rather than the whole read: once a byte has
	// arrived the line is read to its end however long that takes, and the
	// answer is 0.
	//
	// The two answers agree for every timeout a normal script writes and
	// disagree about *when* it expires, which is why this is a divergence to be
	// recorded rather than a bug to be noticed: under a byte dripping slower
	// than the deadline, one answer returns 0 with the whole line and the other
	// returns 1 with a partial one.
	//
	// It is also the axis behind a fifo that holds an unterminated line: under
	// Yes the read never returns there, because the first byte arrived and the
	// delimiter never does. That is the same rule and not a second one.
	//
	// Asked only where a timeout was written and is not zero — a `read` with no
	// `-t` has no deadline to place, and `-t 0` is a question about the stream
	// rather than a deadline at all (ReadZeroTimeout).
	ReadTimeoutBoundsReadability Answer

	// BuiltinWriteErrorFailsTheCommand makes a builtin whose output write
	// failed — into a descriptor closed with `>&-`, most plainly — report
	// status 1, rather than keeping the builtin's own status and quietly losing
	// the text.
	//
	// Whether anything is *said* about it is a matter of wording —
	// Diagnostics.BuiltinWriteError — and not a second axis: an implementation
	// may complain or fail silently, and one that answers No has nothing to
	// word because it does not fail. Asked only when a write has actually
	// failed, so `echo hi` on an open stream needs no answer.
	BuiltinWriteErrorFailsTheCommand Answer

	// LengthOfSpecialIsCount makes `${#@}` the number of positional parameters
	// rather than the length of the joined string.
	//
	// A silent axis: both answers are plausible numbers and nothing says which
	// rule produced one.
	LengthOfSpecialIsCount Answer

	// TransformLetterCheckedOnlyWhenValued delays the check of a `@` operator's
	// letter until the name has a value. Yes makes `${u@QQ}` on an unset name
	// empty at status 0 while the identical spelling on a set one is a bad
	// substitution — the same word meaning two different things depending on
	// what a variable happens to hold.
	//
	// Reached only by a grammar that *has* the family; without it `${u@QQ}` is
	// an unknown operator whatever the value and nothing here is asked. So this
	// is an axis with a single reachable answer, deliberately: making an
	// operator's validity depend on a value is not a rule anything should
	// inherit by having a `@` family, and an implementation that does it should
	// have to say so. An empty array counts as no value — `a=(); ${a[@]@Z}` is
	// quiet where `a=(x); ${a[@]@Z}` is not.
	//
	// No is the null hypothesis this axis exists to make the odd answer argue
	// against: it is what an implementation that checked the letter whatever
	// the value would hold, which is what having a `@` family would otherwise
	// imply. It stays unheld rather than unwritten.
	TransformLetterCheckedOnlyWhenValued Answer

	// ArithLeadingZeroIsOctal reads `0100` as sixty-four rather than as one
	// hundred.
	//
	// The quietest divergence in the vector — nothing warns, both are plausible
	// numbers, and file modes are written this way.
	ArithLeadingZeroIsOctal Answer
	// HeredocExpandsInTheCommandsProcess confines what a here-document body's
	// expansion writes to the command the body feeds, where that command is one
	// the shell runs as a process of its own. Answering No lets the write
	// escape into the shell, so a body of `${u:=zz}` leaves `u` set afterwards.
	//
	// It is one axis rather than one per construct because the split is a
	// property of *where a body is expanded*, and every construct downstream of
	// that follows: a builtin, a function, a compound command, `exec`, `eval`
	// and `.` all leave the write behind under either answer, because the shell
	// runs them itself and there is no other process for it to land in. Nothing
	// is asked for those.
	//
	// Silent either way, which is the reason it is here: a counter advanced
	// inside a template's here-document reads one too high on the next line
	// under the wrong answer, and nothing about the output says so.
	//
	// Asked only when the body actually wrote something. A here-document with
	// no side effect is every other here-document and is unanimous, so an
	// unanswered preset must still be able to run one.
	HeredocExpandsInTheCommandsProcess Answer

	// RedirectTargetExpandsInTheCommandsProcess is the same claim for a
	// redirection's *target*: `> "${u:=made}"` on a command the shell runs
	// as a process of its own leaves `u` unset afterwards, and `> "$NOPE"`
	// under `set -u` costs that command rather than the script.
	//
	// One answer for both consequences, because they are one fact about where
	// the word was expanded — and a second axis rather than a widening of the
	// body's, because an implementation may split the other way there: a
	// here-document body's failed expansion may cost the command and not the
	// script where a target's ends it.
	//
	// Asked only where the command is one the shell runs as a process of its
	// own *and* the expansion either wrote something or failed. A target that
	// expands to a name is every other redirection, is unanimous, and an
	// unanswered preset must still be able to open a file.
	//
	// What is not asked anywhere is whether the open happens: a target whose
	// expansion failed is not opened, unanimously. Diagnosing the unset name
	// and then reporting that `` could not be created is two complaints for one
	// mistake, and the second names a file nobody wrote.
	RedirectTargetExpandsInTheCommandsProcess Answer
	// ForNameWhenTheLoopRuns is what a `for` or `select` does when it is
	// reached and the word standing where its variable belongs is not a name.
	//
	// Asked only where the grammar carried the word this far —
	// syntax.Dialect.ForNameCheckedWhenTheLoopRuns. A grammar that refuses the
	// word while parsing builds no clause to run and never reaches this. See
	// ForNameRunForm for the three answers.
	//
	// One of those answers is reachable only through [Runner.SetPosixMode]
	// rather than through any vector. A mode entered and left at run time is
	// not a preset, so no preset holds that value and it stays unheld on
	// purpose.
	ForNameWhenTheLoopRuns ForNameRunForm

	// FunctionNameWhenTheDefinitionRuns is what a `function` definition does
	// when it is reached and the word standing where its name belongs is not a
	// name.
	//
	// Asked only where the grammar carried the word this far —
	// syntax.Dialect.FunctionNameCheckedWhenTheDefinitionRuns. A grammar that
	// reads such a name as a word and defines what it comes to never reaches
	// the question, and neither does one with no keyword to reach it with. See
	// FuncNameRunForm for the three answers.
	//
	// As with ForNameWhenTheLoopRuns, one answer is reachable only through
	// [Runner.SetPosixMode] and is held by no preset.
	FunctionNameWhenTheDefinitionRuns FuncNameRunForm

	// FatalErrorStatusIsOne is the status a fatal shell error carries, as
	// against a 2.
	//
	// It is one axis rather than one per error, on evidence: a failed
	// arithmetic expansion, a readonly reassignment and a `shift` past the end
	// are three unrelated errors, and any one implementation gives all three
	// the same status. The split is a property of the implementation, not of
	// the error.
	//
	// *Which* errors are fatal is a separate question and stays per-error —
	// ReadonlyReassignmentFatal and ShiftPastEndFatal answer it, and the
	// answers genuinely disagree there. A silent axis either way: scripts that
	// branch on `$?` rather than on truthiness read the failure correctly under
	// one answer and misread it under the other.
	FatalErrorStatusIsOne Answer
	// ArithNameValueRecurses re-evaluates a name-shaped value as an expression:
	// with `y=5; x=y`, `$((x+1))` is 6 because `y` is looked up in turn, and it
	// recurses as far as the values lead — `y=z; z=7; x=y` is 8. Answering No
	// reads the value as a literal and refuses it as an illegal number.
	//
	// Asked rather than assumed, and arithValueOf is where the ask is made.
	//
	// What an implementation does one step further in is a different question:
	// see ArithRecursedNameMustBeSet, which recursion reaching an unset name
	// answers and this axis does not.
	ArithNameValueRecurses Answer
	// ArithRecursedNameMustBeSet makes an unset name *reached through another
	// name's value* an error rather than a zero.
	//
	// Asked only where ArithNameValueRecurses says the lookup happens at all,
	// and only below the top: a name written in the expression itself is zero
	// when it is unset under every answer, `$((nosuch+1))` being 1 throughout.
	//
	// Where it is Yes the refusal is the `set -u` sentence word for word, with
	// nounset off — a name arrived at this way is read as a *parameter
	// reference* rather than as text that might be a number — and it is fatal
	// the way an unset parameter under nounset is: `||` does not catch it, and
	// a subshell dies alone.
	//
	// Silent when it is wrong, which is why it is worth an axis rather than a
	// wording: answering `x=abc; $((x+1))` as 1 and carrying on gives a script
	// whose variable held a stale name a plausible number where it should have
	// stopped.
	ArithRecursedNameMustBeSet Answer
	// ArithSubscriptSkippedWhenNameUnset looks the name up before it reads the
	// brackets, and answers zero for a name that is not there without
	// evaluating the subscript at all: `$(( nodecl[1/0] ))` is a quiet 0 rather
	// than a division by zero, and `i=0; $(( nodecl[i++] ))` leaves `i` at 0.
	//
	// Not a rule about *empty* subscripts, though it is what answers one:
	// `$(( m[$w] ))` with `$w` empty reaches the expression as the literal
	// `m[]`, and where the name has never been set the brackets are never
	// looked at, so the operand is the plain unset 0 that `$(( nosuchvar ))`
	// is. Modeling that as a special case for the empty subscript would have
	// been a rule no probe could tell from this one — the two agree on every
	// empty-subscript row and part only on a subscript that errors or assigns.
	//
	// What "not there" means is set-ness and not emptiness: `e=` then
	// `$(( e[1/0] ))` divides by zero under both answers, and so does an array
	// declared with nothing in it.
	//
	// An unanswered axis reads as No, which is the harmless side: a subscript
	// with no error and no side effect gives the same zero either way, so an
	// unanswered preset is not refused over `$(( a[0] ))`. A grammar with no
	// subscript in arithmetic never reaches the question at all. The axis is
	// read (`== Yes`) rather than asked, so silence and No reach the same code
	// and a preset writing No down would add a line and no fact.
	ArithSubscriptSkippedWhenNameUnset Answer
	// ArithInvalidOctalDigitIsError rejects `08` once a leading zero has been
	// read as octal, rather than falling back to decimal and yielding 8.
	//
	// The second axis the vector could not express with one field.
	// ArithLeadingZeroIsOctal was doing two jobs: an implementation can be
	// octal *and tolerant*, and one boolean cannot say that. The `${!x}` note
	// in docs/spec/semantics.md records the same failure mode, which makes it a
	// limit of the model rather than a quirk.
	//
	// Never reached where nothing made the zero octal in the first place.
	ArithInvalidOctalDigitIsError Answer
	// IntegerAssignmentReadsALeadingZeroAsDecimal makes `typeset -i d=010` ten
	// rather than eight, where the *arithmetic* reader still makes `$((010))`
	// eight.
	//
	// The question is one implementation having two readers, only one of which
	// applies the octal rule. An implementation that applies octal in both, and
	// one that has no octal-by-leading-zero at all, each agree with themselves
	// for opposite reasons — which is why this is a field and not a rule.
	//
	// Whether a *standing* value is re-read when the attribute arrives is a
	// third fact and not this axis: see AttributeRereadsTheValueItFinds.
	//
	// Asked only where the text is a signed digit string with a leading zero in
	// front of another digit, and only where ArithLeadingZeroIsOctal is Yes.
	// Outside that shape the two readers agree — `$((010+1))` and
	// `typeset -i d=010+1` are both nine even where they split — and where
	// nothing made the zero octal there is nothing to choose between. See
	// Runner.zeroPaddedInteger for the edges.
	//
	// Silent and arithmetically wrong when it is answered wrongly: a
	// zero-padded date field or counter comes out eight where it should be ten,
	// with nothing said about it.
	IntegerAssignmentReadsALeadingZeroAsDecimal Answer
	// ArithStoredValueReadsALeadingZeroAsDecimal reads `010` out of a
	// *variable* as ten inside an expression, where the lexer still reads the
	// identical literal as eight.
	//
	// The other half of the split IntegerAssignmentReadsALeadingZeroAsDecimal
	// records, at the other reader. That one is the assignment to an integer
	// name; this one is `$(( k ))`, with no attribute anywhere in it. An
	// implementation reaches it by having an arithmetic lexer that makes a
	// leading zero octal while a value dereferenced into the expression does
	// not go through that lexer — so it is asked only where
	// ArithLeadingZeroIsOctal is Yes.
	//
	// It is the *leading numeral* of the value and not the whole of it, which
	// is what tells this apart from a value read as a number: where they split,
	// `k=010+1` is 11 and the same literal is 9, so the ten is read decimal and
	// the rest of the expression stays octal. See Runner.decimalLeadingNumeral
	// for the edges — a sign, leading space, and an `0x` prefix each leave the
	// value alone.
	//
	// Silent and arithmetically wrong: a zero-padded field read out of a
	// variable — a month, a padded counter — comes out short with nothing said,
	// and under the octal reading `09` is an invalid digit as well as a wrong
	// number.
	ArithStoredValueReadsALeadingZeroAsDecimal Answer
	// LetReadsALeadingZeroAsDecimal gives the `let` builtin a reader of its
	// own, in which `010` is ten and not eight.
	//
	// A third site rather than a reuse of either axis above.
	// ArithStoredValueReadsALeadingZeroAsDecimal rewrites the *leading* numeral
	// of a value and leaves the rest octal, so `k=1+010; $((k))` is 9 there —
	// where `let "x=1+010"` is 11, every numeral in the word having been read
	// in decimal. So `let`'s words are evaluated with the octal rule off rather
	// than with one numeral rewritten, and `(( y=010 ))` beside `let "x=010"`
	// is what separates the site from the builtin.
	//
	// Answered No where nothing makes a leading zero octal in the first place:
	// the two readings coincide there.
	//
	// Silent and arithmetically wrong, the way its two neighbors are:
	// `let "n=010"` is a plausible number, eight where it should be ten.
	LetReadsALeadingZeroAsDecimal Answer
	// ArithmeticAssignmentDeclaresAnInteger gives a name assigned inside an
	// arithmetic context the integer attribute, which outlives the expression.
	//
	// It is not only a listing difference, which is the reason it is an axis
	// rather than a note: the attribute changes what a *later* assignment
	// means. Where it is Yes, `(( x = 5 )); x=2+3` leaves `x` holding 5 rather
	// than the three characters `2+3`, and with the attribute comes the output
	// base, so a name that learned 16 renders a later plain `5` as `16#5`.
	//
	// Every construct that assigns inside arithmetic is the same answer —
	// `(( ))`, `let` and a C-style `for` header alike — so it is asked where
	// the assignment operator is applied rather than at each of them.
	ArithmeticAssignmentDeclaresAnInteger Answer
	// IndirectionYieldsName makes `${!x}` the *name* rather than the value it
	// names: with `x=y`, Yes gives `x` and No gives the value of `y`.
	//
	// Only reachable where the grammar parses `${!x}` at all, and that is the
	// point: a three-way divergence became a grammar flag plus a binary axis,
	// and neither half needed a third state. docs/spec/semantics.md records
	// `${!x}` as the axis a binary table could not express; this is the shape
	// that expresses it.
	IndirectionYieldsName Answer
	// BraceExpansion expands `{a,b}` and `{1..3}`, rather than leaving the word
	// a literal.
	//
	// It lives here rather than in [syntax.Dialect] even though it is additive,
	// because the token stream is identical either way: the parser produces the
	// same word, and only expansion differs. It is also silent in the `&>`
	// sense — `echo {1..3}` prints something under either answer, and nothing
	// reports that one of them is not what was meant.
	BraceExpansion Answer
	// BraceRangePadsToEndpointWidth keeps the leading zeros of a range endpoint
	// and pads every element to the widest endpoint, zeros after the sign:
	// `{01..3}` is `01 02 03` and `{-03..3..3}` is `-03 000 003`. Answering No
	// strips the padding and gives `1 2 3` and `-3 0 3`.
	//
	// Asked only when an endpoint is written with leading zeros, and only where
	// BraceExpansion is Yes.
	BraceRangePadsToEndpointWidth Answer
	// BraceRangeStepSignHonored takes a written step's sign at its word: the
	// walk leaves the first endpoint in the direction the sign says, so a sign
	// pointing away from the far endpoint ends the range after one element —
	// `{10..1..3}` is `10`, `{1..10..-3}` is `1`, and letters answer the same
	// way, `{a..e..-1}` being `a`. Answering No lets the endpoints decide the
	// direction and the step contribute magnitude alone.
	//
	// Asked only when the sign and the endpoints disagree.
	BraceRangeStepSignHonored Answer
	// BraceRangeNegativeStepReverses hands a negative step's sign to the order
	// of the result rather than to the walk: the range is walked endpoint to
	// endpoint and then reversed, so `{3..1..-1}` is `1 2 3` and `{1..10..-4}`
	// is `9 5 1` — the forward `1 5 9` backwards, not the `10 6 2` that
	// swapping the endpoints would give. Answering No leaves `{3..1..-1}` as
	// `3 2 1`.
	//
	// Asked only for a written negative step whose sign was not already
	// honored, so an implementation that reaches the question only when the
	// sign agrees with the endpoints keeps their order too.
	BraceRangeNegativeStepReverses Answer
	// BraceRangeEndpointsExpanded reads a range's endpoints *after* the
	// expansions written in them, rather than before: with `n=3`,
	// `echo {1..$n}` is `1 2 3` rather than the literal `{1..3}` that brace
	// expansion finishing before `$n` exists produces. Quoting hides an
	// endpoint from the brace scanner and not from the range, so `{1..'3'}`
	// counts the same way.
	//
	// This is an ordering axis, and the ordering is the whole of it. Asked only
	// where a range is written with something to expand in it: a literal
	// `{1..3}` is unanimous and must not be turned into a question.
	//
	// What a failed range leaves is not a second axis. An implementation that
	// expands endpoints runs those expansions once and puts the text back
	// unsplit and unmatched, so the ordering decides that too.
	BraceRangeEndpointsExpanded Answer
	// EqualsExpansion replaces an unquoted word beginning with `=` by the path
	// of the command named after it: `echo =ls` prints /bin/ls.
	//
	// Silent in the `&>` sense — answering No takes the word literally and
	// reports nothing, so the same script prints two different things and
	// neither answer complains.
	//
	// Its failure is not silent: a name that resolves to nothing is fatal to
	// the script, like any other failed expansion.
	EqualsExpansion Answer
	// UnterminatedBracket is what `[` without a closing `]` means in a pattern,
	// and it is the axis that does not fit [Answer]. Three readings:
	//
	//	case "[" in [) hit;; *) miss;; esac
	//	a literal `[`                     → hit
	//	a class that can never match      → miss
	//	an error                          → a bad pattern
	//
	// Load-bearing rather than exotic: `[` is the name of the test builtin.
	//
	// It gets its own type rather than a wider [Answer]. Growing [Answer] a
	// third state would be worse, because every other axis is genuinely binary
	// and a wider [Answer] would let a bad-pattern value be assigned to any of
	// them and still compile. An axis with three answers gets a type with three
	// values; the binary ones keep the type that says so.
	UnterminatedBracket BracketPolicy
	// TraceAssignmentsSeparately gives each assignment of `a=1 b=2` its own
	// trace line, rather than putting them on one.
	TraceAssignmentsSeparately Answer
	// TraceShowsItsOwnDisabling prints `set +x` before acting on it. Answering
	// No applies the change first, so the command that stops tracing leaves no
	// trace of itself.
	TraceShowsItsOwnDisabling Answer
	// UnsetPositionalIsAllowed lets `$1` expand to nothing under `set -u`
	// rather than being an error.
	//
	// Quiet where it differs: a script that reads an argument it was not given
	// carries on under Yes and stops under No.
	UnsetPositionalIsAllowed Answer
	// BackgroundJobInput is the standard input a job started with `&` reads,
	// and it is three answers rather than a switch.
	//
	// POSIX XCU 2.9.3, Asynchronous Lists, says a background command's standard
	// input "shall be assigned to an empty file or /dev/null" while job control
	// is disabled. The divergence is to hand the job the shell's own input
	// instead, and it is the direction that *steals*: the job and the script
	// read the same descriptor, so every byte the job consumes is one the
	// script's own `read` never sees —
	//
	//	while read -r line; do process "$line" & done < input.txt
	//
	// silently loses lines, at status 0, with nothing said.
	//
	// The third answer is what a *closed* descriptor does. Substituting the
	// empty input even there is silent at 0; substituting only what can be
	// duplicated leaves a closed fd 0 closed and the job reports a bad file
	// descriptor. It is a sub-answer rather than a second axis: the same
	// decision, asked of an input that is not there.
	//
	// Only while job control is off. That is the condition XCU 2.9.3 states,
	// and it is measured rather than inherited: with job control on, the job is
	// handed the *terminal* and the kernel stops it with SIGTTIN, which an
	// empty input can never produce — so the kind of standard input is an axis
	// of the measurement rather than a detail of it, and an implementation with
	// someone to tell substitutes nothing.
	//
	// Read without asking, for the reason the field below gives: an unanswered
	// axis here would have to refuse `&` itself, and backgrounding a command is
	// ordinary where a background job that reads standard input is rare.
	// Unanswered is the POSIX answer.
	BackgroundJobInput BackgroundJobInputPolicy
	// LastBackgroundPidIsZeroBeforeAnyJob makes `$!` read `0` before a
	// background command has been started, where the alternative is that it
	// expands to nothing at all.
	//
	// Zero is not the same answer as nothing, which is why this is a switch and
	// not a rendering: a background builtin runs in this process and its job
	// carries no pid, so an implementation really can hold a *recorded* zero,
	// and a script cannot tell that apart from the before-any-job zero if the
	// two are spelled alike.
	//
	// Read without asking. A preset that answers nothing answers with nothing,
	// and refusing a `$!` expansion over an unanswered field would break the
	// `p=$!` of every script running under a preset that has not chosen —
	// including before its first job, where the read is exactly the ordinary
	// one.
	LastBackgroundPidIsZeroBeforeAnyJob Answer
	// LastBackgroundPidIsUnsetBeforeAnyJob makes `$!` an *unset* parameter
	// before a background command has been started, so `set -u` is fatal about
	// it.
	//
	// A different split from the field above, and the more useful one: neither
	// answer predicts the other, because an implementation can hold a zero that
	// is *set* and another can hold an empty value that is set too, for
	// different reasons.
	//
	// It is the half a script relies on, since `set -u` exists to stop exactly
	// this read. The wording and the status come from the same [Diagnostics]
	// fields an unset *name* uses, because they are the same two lines: an
	// implementation that writes the `$` back for `$!` writes it back for `$1`
	// too, which is Diagnostics.UnboundPositional, and one that does not uses
	// its ordinary `parameter not set`.
	//
	// Read without asking, for the reason above. Unanswered means the parameter
	// is set and empty, which is what carrying on requires.
	LastBackgroundPidIsUnsetBeforeAnyJob Answer
	// ExitInTrapReportsEarlierStatus makes a bare `exit` in an EXIT trap report
	// the status the shell had when the trap began, rather than that of the
	// trap's own last command.
	//
	//	trap "false; exit" 0; true
	//
	// is 0 under Yes and 1 under No. Only the bare form: `exit 7` is 7 under
	// both, and a trap that does not exit at all leaves the script's status
	// alone under both.
	//
	// Found on an installed script rather than by construction — a script that
	// traps `stty …; exit` on EXIT exits 1 under the wrong answer where it
	// should exit 0. A wrong exit status is what a caller branches on, so this
	// is the quiet kind of difference.
	ExitInTrapReportsEarlierStatus Answer

	// SignalHandlerSeesEarlierStatus shows a signal handler the status from
	// before the command that triggered it rather than that command's own:
	// after `false; kill -INT $$`, Yes reads 1 where No reads the 0 that `kill`
	// succeeding left.
	SignalHandlerSeesEarlierStatus Answer
	// ExitTrapIsFunctionLocal fires an EXIT trap set inside a function when
	// that function returns, rather than when the script ends. A trap set at
	// the top level behaves the same under either answer.
	ExitTrapIsFunctionLocal Answer

	// FunctionLocalTraps is whether a trap a function *sets* is undone when
	// that function returns — the displaced disposition coming back, and the
	// signal going back to its default where nothing was displaced. See
	// TrapLocality for the answers and for why it is a form rather than a flag.
	//
	// EXIT is not this question. Where both exist it is already function-scoped
	// by a rule of its own that asks no option — see ExitTrapIsFunctionLocal —
	// and the switch this axis carries changes nothing about it in either
	// direction.
	//
	// Every preset answers this the same way, because the other reading is
	// reached through a run-time option rather than a default. That is the
	// standing example of a value no vector holds and an implementation still
	// exhibits: a preset is not the whole of an implementation, and the option
	// is wired where the option lives.
	FunctionLocalTraps TrapLocality
	// SIGPrefixAccepted reads `SIGINT` as a name for the same signal `INT`
	// names, wherever a signal can be named. Answering No means the prefix is
	// simply not part of a signal's name, and it shows in three places for two
	// different reasons — `trap 'x' SIGINT` is a bad trap, `kill -SIGINT $$` is
	// an illegal option, and `kill -s SIGINT $$` is an invalid name.
	//
	// It is one axis rather than one per builtin because it is a property of
	// how a signal name is read, and an implementation that refuses the prefix
	// refuses it everywhere. Asked only where the prefix is actually present
	// and stripping it would name a signal: `trap 'x' INT` needs no answer from
	// anyone, and neither does `SIGNOPE`, which names nothing either way.
	SIGPrefixAccepted Answer

	// KillListAcceptsName lets `kill -l` translate a name into a number, as the
	// reverse of what it does with one.
	//
	// Answering No gives `-l` an *exit status* operand instead, so `kill -l 9`
	// agrees with Yes by arriving there another way and `kill -l INT` is an
	// illegal number. One question with two answers rather than a feature that
	// is missing, which is why it is an axis and not a gap.
	KillListAcceptsName Answer

	// ExitTrapRunsOnSignalDeath fires the EXIT trap when the shell is ending
	// because a signal it had no handler for killed it, rather than because it
	// reached the end or ran `exit`.
	//
	//	trap 'echo bye' EXIT; kill -INT $$
	//
	// prints bye under Yes and nothing under No, and reports 130 either way.
	// The axis is whether dying counts as exiting.
	ExitTrapRunsOnSignalDeath Answer

	// QuitIgnoredWhenNotInteractive makes an untrapped SIGQUIT do nothing at
	// all rather than end the shell.
	//
	//	kill -QUIT $$; echo after
	//
	// prints after and exits 0 under Yes, and kills the shell with SIGQUIT
	// under No. Asked only where those disagree — an untrapped QUIT in a shell
	// that is not interactive — because every other case is unanimous: it is
	// ignored with `-i` under both answers, and a QUIT with a trap runs the
	// handler under both.
	//
	// Two things are worth recording beside the split. The first is that this
	// is a *version* divergence as much as an implementation one: builds of one
	// implementation answer it differently, so a claim about a name that does
	// not say which build is incomplete. The second is that where it is ignored
	// it is ignored properly rather than deferred — a signal sent from another
	// process is survived too.
	//
	// What `trap - QUIT` then means is a further question this does not answer,
	// and the answers split differently on it: after a handler is installed and
	// removed again, the signal may still be ignored or may become fatal.
	QuitIgnoredWhenNotInteractive Answer

	// SubshellRunsOnAfterSignalingTheShell lets the rest of a subshell's body
	// run after something inside it has sent the whole shell a fatal signal —
	// `(kill -TERM $$; echo inner)`.
	//
	// The shell ends either way, and that half is unanimous: `outer` is never
	// printed, and the answer holds under load, so this is not a delivery race.
	// What splits is `inner`.
	//
	// The reason is the opposite of the obvious one. An implementation that
	// gives the subshell a **process of its own** has `$$` name the parent, so
	// the child never receives the signal and finishes its body while the
	// parent dies. One that runs the subshell **in the shell's own process**
	// makes `kill -TERM $$` a self-signal landing on the very process that was
	// about to run `echo inner`, and there is nothing left to run it. So the
	// one that keeps going is the one that forked.
	//
	// Nothing here forks for a subshell either, which is what makes this an
	// axis rather than a consequence: the answer has to be chosen rather than
	// inherited from the architecture, and choosing Yes is choosing to behave
	// like an implementation that forks.
	//
	// Only a subshell. The same signal at the top level, in a brace group, in a
	// function body and in a `while` body stops at once and prints nothing
	// under every answer, so there is no question to ask anywhere but here.
	//
	// The POSIX preset says yes: the standard has `( )` execute "in a subshell
	// environment" and describes that environment as a copy, which is the
	// forking reading.
	SubshellRunsOnAfterSignalingTheShell Answer

	// HangupIsAnOrderlyExit makes an untrapped SIGHUP end the shell the way
	// `exit 1` would rather than by the signal's default action.
	//
	//	kill -HUP $$; echo after
	//
	// reports 1 under Yes and 129 under No, and prints nothing after it either
	// way. The number is the visible half; the discipline behind it is the
	// whole answer, and three further observations say so: under Yes the
	// shell's caller sees an ordinary exit rather than a death by SIGHUP, the
	// EXIT trap runs, and an `exit 5` inside that trap wins the status the way
	// it would after any other ending.
	//
	// That the EXIT trap runs is what makes this an axis of its own rather than
	// a number to special-case. ExitTrapRunsOnSignalDeath asks whether dying
	// counts as exiting; an implementation can answer that No — `trap 'echo
	// bye' EXIT; kill -TERM $$` printing nothing — and still print bye for
	// `kill -HUP $$`, which is only consistent if SIGHUP never produced a death
	// to ask the question about.
	//
	// It is one signal, and only this one. Across the nineteen signals whose
	// default action ends a process — HUP, INT, QUIT, ILL, TRAP, ABRT, FPE,
	// BUS, SEGV, SYS, PIPE, ALRM, TERM, USR1, USR2, XCPU, XFSZ, VTALRM and
	// PROF — the answers are unanimous on every one except QUIT, which
	// QuitIgnoredWhenNotInteractive covers, and this. An external SIGHUP is
	// answered the same way, so it is a disposition rather than something the
	// `kill` builtin does on its way past.
	HangupIsAnOrderlyExit Answer

	// StatusArgument is how a *status operand* is read — the word after `exit`
	// and the word after `return` — and it is an ordering rather than a side.
	// The questions it bundles are whether text that is not a number is
	// refused, whether a sign is, whether the value is masked to eight bits,
	// and whether the refusal ends the script.
	//
	// One axis for two builtins because the two operands are read identically:
	// every row was measured on `exit` and on `return` and the two never
	// parted. A second field for `return` is the shape that has cost this tree
	// seven bugs — the copy omits what the original learned — and here it would
	// have left `exit r` and `return r` disagreeing.
	//
	// Four readings, so a policy rather than a bool — the same shape as
	// UnterminatedBracket, and for the same reason. What each value carries is
	// a bundle rather than four axes, because the four questions were measured
	// together and never crossed: an implementation that refuses text is one
	// that refuses a sign or does not, masks to eight bits or does not, and
	// ends the script over the refusal or does not. Some readings refuse
	// nothing at all, so splitting the refusal out would have needed an answer
	// from presets that cannot reach the question.
	StatusArgument StatusArgumentPolicy
	// BracketCaretNegates reads `[^abc]` as a negated class, rather than
	// treating `^` as an ordinary character.
	//
	// Both answers are matches, on different inputs, with nothing to warn on:
	// under No `[^abc]` matches a caret, and under Yes it matches
	// everything-but.
	BracketCaretNegates Answer
	// GetoptsAssignmentRestartsWord makes assigning OPTIND begin the word
	// again, dropping any position inside a cluster.
	//
	// It is the *assignment* that does it rather than the value: `set -- -ab;
	// getopts ab o; OPTIND=1` writes the number OPTIND already held, and Yes
	// still restarts and reads `a` a second time where No carries on to `b`.
	GetoptsAssignmentRestartsWord Answer

	// GetoptsPositionIsFunctionLocal gives every shell function call its own
	// `getopts` cursor: OPTIND starts the call at 1 whatever the caller had
	// reached, and the caller's position comes back when the call returns.
	//
	// Under Yes it is the parameter itself that is local rather than only the
	// builtin's bookkeeping — an explicit assignment inside the function does
	// not escape either:
	//
	//	g() { echo "entry=$OPTIND"; OPTIND=7; }
	//	OPTIND=3; g; echo "after=$OPTIND"
	//
	// answers entry=1 after=3 under Yes and entry=3 after=7 under No. The
	// position *inside* a clustered word is saved with it, which a shared
	// cursor cannot express: with `-ab` half read, a function scanning `-cd` of
	// its own reads both `c` and `d` under Yes and only `d` under No, and on
	// return the caller still finds its `b`.
	//
	// A `getopts` that resets its cursor when it runs out of options looks
	// close and is not: that leaves OPTIND at 2 on the way out and starts the
	// *next* scan at 1, where Yes shows 2 inside the function and 1 outside,
	// which is a restore rather than a reset.
	//
	// What this does not cover is `unset OPTIND`, which takes the parameter
	// away rather than giving the call a value of its own: the name stays gone
	// after the function returns under every answer, so there is nothing for
	// the return to put back. A function entered with OPTIND already unset is
	// not handed a cursor at 1 either.
	//
	// Why it earns an axis rather than a note: a shell function that parses
	// options is only reusable if the second call starts over, so a function
	// library written under Yes omits the `local OPTIND=1` the other answer
	// needs — and under the wrong answer its second call reads its arguments
	// from index 2 and prints its usage.
	GetoptsPositionIsFunctionLocal Answer

	// GetoptsLocalOptindRestoresTheCursor hands the caller back its position
	// *inside* a clustered word when a call that declared a local `OPTIND`
	// returns — not only the number the parameter held.
	//
	// The scan position has two halves: `OPTIND`, which counts words, and how
	// far into a clustered word the letters have been read. Only the first is a
	// parameter, so an implementation that shadows the parameter and stops
	// there hands the caller a cursor pointing at the start of a word it had
	// already part-read. That is measurable with no `getopts` in the callee at
	// all:
	//
	//	g() { local OPTIND=1; :; }
	//	set -- -ab; getopts ab o; g; getopts ab o
	//
	// The second `getopts` reads `b` under Yes and reads `a` a second time
	// under No. Reading `a` again is not a wrong letter so much as a scan that
	// cannot finish: a loop whose body calls a function that declares
	// `local OPTIND` starts the same word over every time round and never runs
	// out of options.
	//
	// Entering the call is *not* the disagreement and is not asked here: any
	// implementation with a local scope at all hands the callee a cursor at the
	// start of a word, so a callee scanning its own `-cd` reads both letters
	// whether or not the declaration carried a value. What splits the answers
	// is only the way back.
	//
	// Yes is also reachable by a different route — a cursor local to every call
	// whether or not anything was declared, which is
	// GetoptsPositionIsFunctionLocal — and an implementation with no `local`
	// may reach it through its own declaration keyword instead.
	GetoptsLocalOptindRestoresTheCursor Answer

	// GetoptsClearsOptarg empties OPTARG when `getopts` reports a bad option
	// rather than leaving it unset. A script testing `${OPTARG-}` can tell the
	// two apart.
	GetoptsClearsOptarg Answer

	// CdWithoutHomeIsAnError makes `cd` with no operand and no HOME a failure,
	// rather than staying put and reporting success — which is the quieter
	// answer and the surprising one. The same axis answers `cd -` with no
	// OLDPWD.
	CdWithoutHomeIsAnError Answer
	// CdEmptyOperandIsAnError refuses `cd ""` instead of taking it as the
	// directory the shell is already in.
	//
	// An empty operand is not the same thing as no operand, and it is not
	// nothing either: under No, `cd /tmp; OLDPWD=MARK; cd ""` leaves OLDPWD at
	// `/tmp` and any change-of-directory hook fires — so it is a real move to
	// the same place, which joining an empty operand against the working
	// directory already is. Under Yes it is a refusal at status 1, staying put.
	CdEmptyOperandIsAnError Answer

	// CdEmptyHomeIsAnError refuses `cd` with HOME set to the empty string,
	// rather than going where the shell already is.
	//
	// A separate question from CdWithoutHomeIsAnError, which is about a HOME
	// that is *absent*, and separate from the axis above, which is about an
	// operand. An implementation can answer yes to the first, no to this one
	// and yes to the third, so no two of them can be one field.
	CdEmptyHomeIsAnError Answer

	// CdSubstitutesTheOperands reads `cd old new` as a rewrite of the current
	// directory — the first occurrence of old in `$PWD` replaced by new —
	// rather than as too many operands.
	//
	// It is the first occurrence in the *string* rather than the first path
	// component: from `…/a/q/a/w`, `cd a Z` lands in `…/Z/q/a/w`. Answering No
	// either refuses the shape outright or ignores everything after the first
	// operand, which CdRefusesExtraOperands decides.
	CdSubstitutesTheOperands Answer

	// CdSubstitutionPrintsTheDirectory writes where a `cd old new` went, the
	// way `cd -` writes where it went, rather than moving in silence.
	//
	// Asked only on a substitution that arrived somewhere: a rewrite naming a
	// directory that is not there prints nothing under either answer.
	CdSubstitutionPrintsTheDirectory Answer

	// CdRefusesExtraOperands refuses operands after the first instead of taking
	// the first and saying nothing about the rest.
	//
	// Asked only where CdSubstitutesTheOperands said no, which is the point an
	// implementation that has that form is no longer in the conversation.
	CdRefusesExtraOperands Answer

	// CdDashPrintsTheDirectory writes the new directory when `cd -` moves,
	// rather than moving in silence.
	CdDashPrintsTheDirectory Answer

	// PrintfReportsBadNumber complains when a numeric conversion is given
	// something that is not a number.
	//
	// The zero is printed under either answer, so the complaint sits beside the
	// output rather than instead of it.
	PrintfReportsBadNumber Answer
	// PrintfBackslashC is what `\c` means in a printf format, and it is three
	// different things rather than a switch:
	//
	//	printf "a\cbZ"   a\cbZ      two literal characters
	//	                 a<0x02>Z   `\cX` is control-X
	//	                 a          the output stops there
	//
	// Told apart by the bytes rather than by the display, which is the only way
	// to separate the middle reading from the last: a control character *looks*
	// like truncation until it is read as a byte.
	PrintfBackslashC PrintfBackslashCPolicy
	// PrintfUnfinishedConversionIsAPercent writes a bare `%` for a format that
	// ended before its conversion character, and reports success, rather than
	// complaining about a conversion it could not read.
	//
	// The whole unfinished conversion becomes the one character: `a%5` and
	// `a%ll` are both `a%`, so the prefix that was scanned is dropped rather
	// than written back. Answering No reports the unreadable conversion and
	// fails.
	//
	// Asked only where a format actually ends inside a conversion.
	PrintfUnfinishedConversionIsAPercent Answer
	// PrintfHexEscape is how a printf format reads `\x`, and it is four answers
	// rather than a presence. The three questions that separate them are
	// whether it is an escape at all, how many digits it takes, and what an
	// empty digit run means:
	//
	//	printf 'a\x80Z'   a<0x80>Z        one raw byte
	//	                  a\x80Z          not an escape at all
	//	printf 'a\x0ffZ'  a<0x0f>ffZ      two digits, then text
	//	                  a<0xc3><0xbf>Z  every digit, read as a code point
	//	printf 'a\xZ'     a\xZ            and a complaint
	//	                  a<0x00>Z        an empty digit run is zero
	//
	// Asked only where a `\x` is actually in the format. It is a question about
	// the *format*, and never about a `%b` argument: that site has its own axis,
	// PrintfBHexEscape below.
	PrintfHexEscape PrintfHexEscapePolicy
	// PrintfBHexEscape is how a `%b` argument reads `\x`, which is a different
	// question from the one PrintfHexEscape answers: an implementation can read
	// `\x41` in a format and write the four characters as they stand in a `%b`,
	// so the site decides as much as the implementation does.
	//
	// The readings themselves are the format's four, which is why this shares
	// that enumeration: an implementation that has the escape here reads its
	// digits the way it reads a format's. Not every reading is reachable at
	// this site — the code-point reading is held at the format and nowhere
	// answers it here — which is exactly why these are two fields.
	//
	// Asked only where a `%b` argument actually carries a `\x`.
	PrintfBHexEscape PrintfHexEscapePolicy
	// PrintfUnicodeEscape is how a printf format reads `\uHHHH` and
	// `\UHHHHHHHH`, and it is four answers rather than a presence — the same
	// shape PrintfHexEscape has, arrived at from the same three questions and
	// splitting in a different place:
	//
	//	printf 'a\u0041Z'  aAZ          the code point
	//	                   a\u0041Z     not an escape at all
	//	printf 'a\uZ'      a\uZ         and a complaint, at status 0
	//	                   a<0x00>Z     an empty digit run is zero
	//	                   a            the rest of the format pass is dropped,
	//	                                and the loop over the operands goes on,
	//	                                so `printf '[%s]\uZ' x y` is `[x][y]`
	//
	// That fourth reading is why this is not PrintfHexEscapePolicy under
	// another name: no `\x` reading drops what follows it.
	//
	// Four digits after `\u` and eight after `\U`, and fewer are accepted:
	// `a\u41Z` is `aAZ` wherever the escape is read, and `a\u00410` is an `A`
	// followed by a zero. The value is a code point written in UTF-8 rather
	// than a byte, and it is the *original* UTF-8 rather than the range Unicode
	// later kept — see EncodeCodePoint, which is the one encoder both escape
	// sites and `echo` share.
	//
	// Asked only where a `\u` or a `\U` is actually in the format. It is a
	// question about the *format*, and never about a `%b` argument: that site
	// has its own axis, PrintfBUnicodeEscape below.
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
	// Visible only where both streams arrive at one place, which is exactly how
	// the corpus reads them: `printf "[%z]"` is `[` and then the complaint under
	// Yes, and the complaint and then `[` under No, where the output is still
	// sitting in a buffer when the complaint goes out.
	PrintfOutputPrecedesComplaint Answer
	// PrintfEmptyIsNotANumber complains about a numeric conversion given an
	// operand that is present and empty. The zero is printed under either
	// answer, and an argument that is *missing* is never an error under either.
	PrintfEmptyIsNotANumber Answer

	// PidListingFinishesWithAJob makes `jobs -p` forget a finished job the way
	// a listing of states does.
	//
	// The two readings part on a background command that has already ended: Yes
	// writes the process id and then shows nothing on the *next* `jobs`, where
	// No writes the id and then reports it Done. So a pid listing is a listing
	// that finishes with the job under one answer and a peek that leaves it
	// under the other.
	//
	// Asked only where a pid listing met a finished job, so an ordinary `jobs`
	// never raises it: that form finishes with the job under both answers.
	PidListingFinishesWithAJob Answer

	// PrintfTimeConversion gives `printf` a `%(fmt)T`: an epoch through a date
	// format, with the format written inside the conversion.
	//
	// The operand is seconds since the epoch, and two numbers are not times: -1
	// is now and -2 is when the shell started. A missing operand is now as well,
	// and an empty format is the C locale's time of day.
	//
	// A `%T` whose operand is a date *string* is **not** this conversion, and
	// answering No is what an implementation with that one holds: see
	// PrintfTimeOperandIsADateString, and docs/spec/semantics.md for why reading
	// a date that way is its own feature rather than this one configured.
	//
	// Asked only where a format actually carries a `%(`, so an implementation
	// without the conversion is never questioned about `%s`.
	PrintfTimeConversion Answer

	// PrintfTimeOperandIsADateString makes that conversion's operand a date
	// *string* rather than a number of seconds, and gives the shell a plain `%T`
	// with no parentheses as well.
	//
	// Asked only where PrintfTimeConversion already said yes — an implementation
	// without the conversion is never questioned about its operand. See
	// interp/printfdate.go for the strings, the subset taken, and why the rest
	// meet a warning rather than a guess.
	//
	// It moves the empty format too: `%()T` is the time of day where the operand
	// is an epoch and the full `date` line where it is a string, which is the
	// same default the bare `%T` writes.
	PrintfTimeOperandIsADateString Answer

	// PrintfQuote is how `%q` quotes, which is three answers and an absence
	// rather than a switch — see PrintfQuoteStyle.
	PrintfQuote PrintfQuoteStyle

	// RedirectsUseEveryTarget makes a stream redirected more than once use
	// *every* file it names rather than only the last, in both directions:
	// output goes to all of them and input arrives as all of them in the order
	// written. Under Yes `echo x >a >b` fills both and `cat <a <b` is both
	// files; under No `a` is left empty and only `b` is read.
	//
	// One axis and not two, because it is one switch where it exists: turning
	// that switch off takes the fan-out and the concatenation together, and two
	// fields would be two places to forget one of them. Named for a target
	// rather than for a direction for the same reason.
	//
	// Silent either way — using the last target alone reports no error, and the
	// script looks like it worked — which is the `&>` failure mode in a
	// redirection. Asked only where a command redirects one stream twice,
	// because that is the only place it decides anything.
	RedirectsUseEveryTarget Answer

	// NullCommandVariable names the parameter holding the command that a
	// command consisting only of redirections runs.
	//
	// Empty is the core's answer: `<f` opens the file, runs nothing and writes
	// nothing, and `>g` truncates `g` the same way. A non-empty name instead
	// treats the redirections as arguments to a command named by that
	// parameter, so `<f` at a prompt pages the file — observed by pointing the
	// parameter at a function that prints a marker and watching the marker come
	// out.
	//
	// A name rather than a value, because the parameter is a script's to
	// reassign at any moment and the answer has to be read when the command
	// runs, not when the preset is built.
	//
	// The hook is off entirely while this is empty, which is what keeps the two
	// readings apart: without the hook the command runs nothing and *succeeds*,
	// where the hook with an empty parameter refuses the command by name — see
	// Diagnostics.RedirectionWithNoCommand. So "no hook" and "a hook with
	// nothing in it" are different observable behaviors, and this field is the
	// first of them.
	//
	// What it is not: the `$(<file)` form, whose whole body is one input
	// redirection. That form does not consult this parameter — see
	// readfilesubst.go — so a substitution reads the file even where the hook is
	// pointed somewhere else. Two operands, two paths.
	NullCommandVariable string

	// ReadNullCommandVariable names the parameter consulted in place of
	// NullCommandVariable when the command's *only* redirection is a plain input
	// file redirection.
	//
	// One redirection and one operator: `<f` and `3<f` both take this route —
	// the descriptor number does not matter — where `<f <g`, `<>f`, `<<<x`,
	// `<&0`, `2>e` and `<f 2>e` all take the other. Established a spelling at a
	// time with the two parameters pointed at two different marker functions,
	// which is the only probe that can tell them apart: with both left at their
	// defaults the two routes print the same file and the reading is
	// unfalsifiable.
	//
	// Empty — the parameter unset, or set to nothing — falls back to
	// NullCommandVariable rather than refusing, so a script that clears the
	// reader still pages nothing and concatenates instead.
	ReadNullCommandVariable string

	// NoclobberBlocksAppendCreate makes `set -C` stop `>>` from *creating* a
	// file, so appending to a name that is not there is a refusal rather than a
	// new file. Asked only under noclobber, which is the only place it decides
	// anything.
	//
	// POSIX puts noclobber on `>` alone — 2.7.2 makes `>` fail when the file
	// exists and says nothing about `>>` — so the standard's answer is No, and
	// Yes is the departure, refusing at 1 with `no such file or directory`.
	// Appending to a file that *does* exist is the control, and appends and
	// reports 0 under both answers.
	//
	// It is the reason `>>|` exists. Where this is No the override has nothing
	// to override, so a preset that answers Yes here is the only one for which
	// the append half of syntax.Dialect.ClobberOverrideMarker is observable —
	// which is why the two belong together.
	NoclobberBlocksAppendCreate Answer

	// KillStatus is what `kill` reports when it was given several targets and
	// they did not all agree. Three answers, and no two of them are the
	// majority:
	//
	//	kill -0 $$ 999999    0 · 1 · 1
	//	kill 999998 999999   1 · 1 · 2
	//
	// One reading reports success if it signaled anything at all; another
	// reports the number that failed — a status carrying a count rather than a
	// verdict, and the reason this is a policy rather than a bool.
	KillStatus KillStatusPolicy
	// CommandNotFoundStatusIsNotFound makes `command -v` answer 127 for a name
	// that is nothing, rather than a plain 1. Answering No reports a failure and
	// leaves 127 to mean a command that was looked for and run.
	CommandNotFoundStatusIsNotFound Answer

	// SubshellJobTable is what a subshell sees of the jobs its parent started.
	// Three answers, and neither of the two-way splits it contains is the same
	// pair:
	//
	//	sleep 1 & jobs -p | cat; echo T    the pid · the pid · nothing
	//	sleep 1 & (jobs -p); echo T        the pid · nothing · nothing
	//
	// so no single yes-or-no can hold both rows. See
	// SubshellJobsKeptOutsideACompound for the reading that separates them and
	// for the part of it that is measured and not modeled.
	SubshellJobTable SubshellJobTable

	// InteractiveSelectsEmacs turns the `emacs` editing mode on when the shell
	// becomes interactive, and leaves both mode names off otherwise. Answering
	// No selects neither mode at any point, reporting both off even in an
	// interactive session at a real terminal.
	//
	// The trigger is interactivity rather than a terminal, which is what makes
	// it a question about *when a mode is chosen* rather than about which mode.
	// A script that selects one is unaffected either way: this only says what an
	// unasked shell reads.
	//
	// Read rather than `ask`ed, as DefaultOptionLetters is: reporting an option
	// is not the place to refuse a script over a disagreement, and a preset that
	// answers nothing gets No.
	InteractiveSelectsEmacs Answer

	// SetFTurnsOffGlobbing makes `set -f` the short spelling of `set -o noglob`.
	// Answering No spells that option the long way only, and gives `-f` a
	// different meaning that leaves globbing alone, so `set -f; echo *.txt`
	// lists the files.
	SetFTurnsOffGlobbing Answer

	// SetBTurnsOffBraceExpansion makes `-B` the short spelling of the
	// `braceexpand` option, so `set +B` stops `{a,b}` expanding and `set -B`
	// puts it back. Answering No either has no such letter or means something
	// else by it, in which case the letter belongs among the ones that preset
	// refuses.
	//
	// Asked only where the letter is written, like SetFTurnsOffGlobbing: the
	// long name `braceexpand` raises no question, because an implementation
	// either declares it or has never heard of it.
	SetBTurnsOffBraceExpansion Answer

	// NoglobLetterIsF puts `f` in `$-` while noglob is on, which is the letter
	// POSIX gives it. Answering No reports the capital instead, `-F` being the
	// short option that means noglob there — the same split
	// SetFTurnsOffGlobbing records, seen from the reading side.
	NoglobLetterIsF Answer

	// DefaultOptionLetters is what `$-` starts with before the script has set
	// anything: the single-letter options a shell turns on at startup. The
	// answer is the same under `-c`, a script file and standard input.
	//
	// The letters that describe the invocation *route* rather than an option a
	// script could set — `c` and `s` — are not here, for the reason `i` is not:
	// they are facts about the invocation that the front end carries in, read
	// off Runner.Route. Where the answers split over them they have axes of
	// their own, below.
	//
	// The letters a shell turns on only *when* it is interactive are a second
	// vector of their own; see InteractiveOptionLetters, which replaces this one
	// rather than adding to it.
	DefaultOptionLetters string

	// InteractiveOptionLetters is DefaultOptionLetters for a shell that is
	// interactive. It **replaces** the other rather than being appended to it,
	// and that is the whole reason it is a second string instead of a field of
	// letters to add.
	//
	// A preset forces that shape by *dropping* a letter when it becomes
	// interactive — a command-tracking option that is on for a script and off at
	// a prompt. A "letters to add" field could not have said that, and would
	// have recorded most presets correctly and one wrongly.
	//
	// Empty means there is no separate answer and DefaultOptionLetters stands
	// for both, which is what a preset whose `$-` does not change leaves it at.
	//
	// Two letters are deliberately *not* in it, and both for the reason the
	// route letters are not in DefaultOptionLetters — they are facts the runner
	// holds rather than a string it prints:
	//
	//   - `i` itself, which is unanimous and comes from Runner.Interactive.
	//   - `m`, the monitor. Writing `m` into this string would report a monitor
	//     that is not running. The letter comes from Runner.monitor or it does
	//     not come at all; that a preset may turn job control on for
	//     `-i script.sh` where the front end does not is recorded in
	//     docs/spec/invocation.md, and is a separate question from this one.
	//
	// Read rather than `ask`ed, exactly as DefaultOptionLetters is: a preset
	// that answers nothing shows the letters it shows for a script, and refusing
	// a whole `$-` expansion over an unanswered field would break
	// `case $- in *e*)` in every script running under a preset that has not
	// chosen.
	InteractiveOptionLetters string

	// CommandStringShowsCInDollarDash puts `c` in `$-` when the program came
	// from `-c`. The answers split evenly, so there is no majority to follow and
	// this is a switch.
	//
	// The POSIX preset says yes, from the text rather than from a vote: `$-` is
	// defined as the option flags specified on invocation, and `-c` is one of
	// them.
	//
	// Read without asking, unlike most axes. A preset that answers nothing shows
	// no letter, which is the same thing an unanswered DefaultOptionLetters
	// does; refusing a whole `$-` expansion over it would break
	// `case $- in *e*)`, the ordinary errexit check.
	CommandStringShowsCInDollarDash Answer

	// CommandStringShowsSInDollarDash also puts `s` there under `-c`.
	//
	// The shape of the disagreement is worth stating: `s` itself is unanimous
	// for the standard-input route — with `-s` written or not, and at a prompt —
	// so what splits the answers is only whether a command string counts. Yes is
	// the rule "no script file was named" and No is "the program came from
	// standard input"; the two agree everywhere except here.
	//
	// Read without asking, for the reason above.
	CommandStringShowsSInDollarDash Answer

	// LoginShowsLInDollarDash puts `l` in `$-` when the shell was started as a
	// login shell. No majority to follow, so it is a switch.
	//
	// A No can be deliberate rather than an omission: an implementation may keep
	// the fact in a shell option that reads `on` for exactly the invocations
	// this letter would mark, answering the question somewhere else.
	//
	// The spelling of the invocation does not decide it. Bundled (`-lc`),
	// unbundled (`-l -c`), long (`--login -c`) and inferred from a dashed
	// `argv[0]` with no option at all all answer the same way, so the split
	// belongs to the implementation and not to how the caller said it. It is
	// membership rather than order: no two presets order `$-` alike.
	//
	// The fact itself is Runner.LoginShell, carried in from the front end: no
	// `set` letter turns login-ness on in most presets, so there is no option
	// field for the table to write. That an implementation may make `l` a
	// genuine `set` option — `set +l` taking it back out of `$-` and `set -l`
	// putting it in — is recorded in docs/spec/semantics.md rather than
	// implemented here.
	//
	// The POSIX preset leaves it unanswered, which shows no letter: POSIX names
	// no login option at all, so there is nothing for `$-` to report as one —
	// unlike CommandStringShowsCInDollarDash, where `-c` *is* an invocation flag
	// the text defines.
	//
	// Read without asking, for the reason above.
	LoginShowsLInDollarDash Answer

	// ArithIntegerOperatorRefusesFloat rejects a float where only an integer
	// will do — `7 % 2.5`, `1.5 & 1`, a shift — rather than truncating it. It
	// does not arise without floats, which is why a preset with no float
	// arithmetic leaves it unanswered.
	ArithIntegerOperatorRefusesFloat Answer

	// ArithNegativeExponentIsError refuses `2**-1` and stops the expression,
	// rather than answering 0.5. It does not arise where the grammar has no
	// `**`, which is why such a preset leaves it unanswered.
	ArithNegativeExponentIsError Answer

	// ProcessSubstitutionInCondition lets `<(cmd)` stand as a condition's
	// operand — `[[ $v == <(cmd) ]]` — and be performed there.
	//
	// A No may be reached at different moments: the word can be read and then
	// refused in a sentence of its own, or refused earlier still while reading,
	// and a grammar with no `[[ ]]` never reaches the question. What differs
	// between those is only when and in what words — which is exactly the split
	// between this axis and [Diagnostics].
	//
	// It is asked *before* the substitution is performed. Refusing the word must
	// not have started the command first, and that is observable: the command
	// has side effects.
	ProcessSubstitutionInCondition Answer

	// ProcessSubstitutionBodyReadsTheShellsInput hands a process substitution's
	// body the standard input the *shell* has, rather than the standard input of
	// the command whose word the substitution stands in.
	//
	// The two are the same stream almost everywhere, which is what makes the
	// axis narrow and is why the obvious control row cannot see it: `cat <(cat)`
	// reads the shell's input under either answer, because the command's input
	// *is* the shell's. They part inside a pipeline element, whose input is the
	// pipe:
	//
	//	printf "PIPE\n" | cat <(cat)      with the shell's input a file
	//	                                  holding OUTER
	//
	// Yes answers OUTER and No answers PIPE.
	//
	// The reading behind Yes is that a pipeline element's pipe is one of that
	// element's *redirections*, and a redirection is applied after the command's
	// words have been expanded — so a substitution performed while expanding
	// them is still looking at the shell's own input. That reading is what
	// bounds the axis, and every boundary below is measured rather than
	// inferred, because both answers agree at each of them:
	//
	//	printf "PIPE\n" | { cat <(cat); }        PIPE either way
	//	f() { cat <(cat); }; printf … | f        PIPE either way
	//	printf "PIPE\n" | eval "cat <(cat)"      PIPE either way
	//	printf "PIPE\n" | cat < <(cat)           PIPE either way
	//
	// A compound command's body, a function's body and an `eval`'s program all
	// run after the element's redirections are in place, and a substitution
	// written as a *redirection operand* is expanded with them rather than
	// before them. So the answer reaches one simple command's words and stops
	// there.
	//
	// `>(cmd)` does not observe it: that spelling gives the body the reading end
	// of its own pipe, which replaces whatever it would otherwise have read. The
	// axis is still asked for it through the one place all three spellings are
	// prepared, so the file form `=(cmd)` cannot drift away from `<(cmd)`.
	ProcessSubstitutionBodyReadsTheShellsInput Answer

	// ConditionArithmeticErrorIsFatal abandons the input when an operand of a
	// word-spelled comparison — `[[ 1+ -eq 0 ]]` — is not an expression the
	// arithmetic parser can read.
	//
	// The operands themselves are core: a grammar with `[[ ]]` evaluates them as
	// arithmetic, so `n=5; [[ n -eq 5 ]]` holds throughout and there is nothing
	// to switch on. What is disagreed about is the *failure*, from
	// `echo one; [[ 1+ -eq 0 ]]; echo two`:
	//
	//	Yes  complains, `two` never runs, exit 1
	//	No   complains, the condition is false, `two` runs at status 1
	//
	// A conflict and not a wording difference — a script that guards with
	// `[[ n -eq 0 ]]` over a name it did not set runs to the end under one
	// answer and stops at that line under the other.
	//
	// Asked only on the error path. A condition whose operands read cleanly
	// never reaches it.
	ConditionArithmeticErrorIsFatal Answer

	// ArithCommandErrorStatusIsTwo is what `(( expr ))` leaves behind when the
	// expression could not be evaluated: 2 where this is Yes, 1 where it is No.
	//
	// The sentence is not the question — that is [Diagnostics]. What differs
	// here is what the construct leaves for the next line to read.
	//
	// It is the *construct* and not the evaluator: `let "1+"` is 1 under both
	// answers, so a status hung on the arithmetic error itself would have moved
	// `let` with it. That is the discriminating pair, and it is why this is
	// asked here and nowhere else.
	//
	// The value is a status and not a truth, so it is reached the same way
	// through `if (( 1+ ))` — the condition is false, and the status behind it
	// is this answer.
	//
	// Asked only on the error path. An expression that reads cleanly leaves 0 or
	// 1 for its own value, which is unanimous and not a question.
	ArithCommandErrorStatusIsTwo Answer

	// ArithCommandErrorIsFatal abandons the input when `(( expr ))` could not be
	// evaluated, instead of leaving the status above for the next line to read.
	//
	// A conflict rather than a wording difference — under Yes the construct is
	// fatal, under No it is a reporting statement, and a script that tests a
	// counter with `(( n ))` over a name it did not set runs to the end under
	// one answer and stops at that line under the other.
	//
	// The same question for *both* ways the expression can fail, which is what
	// ArithCommandErrorStatusIsTwo already found: `(( 1+ ))` never reaches the
	// evaluator and `(( 1/0 ))` does, and Yes abandons the input for both.
	//
	// It is not the same question as the construct standing as a condition:
	// `(( 1+ )) && echo yes` and `if (( 1+ ))` are fatal under Yes too, so this
	// is about the expression and not about what the status is read for.
	//
	// Nor does it group with the word-spelled comparison. This is the other half
	// of the question ConditionArithmeticErrorIsFatal asks about
	// `[[ 1+ -eq 0 ]]`, and the two do not cut the same way: an implementation
	// may abandon the condition and stay for `(( ))`, which is why a single
	// field could not carry both.
	//
	// The reach is the ordinary one for a fatal error rather than anything of
	// this construct's: a *sourced file* alone is given up — `. ./s.sh; echo
	// after` still prints `after` — and a subshell alone, which is the same
	// boundary the `[[ ]]` failure stops at.
	//
	// Asked only on the error path. An expression that reads cleanly never
	// reaches it.
	ArithCommandErrorIsFatal Answer

	// LetKeepsTheValueBeforeAnIllegalByte leaves `let` with the value its
	// expression had reached when the arithmetic reader met a byte it refuses,
	// instead of leaving it with nothing.
	//
	// `let` reports *false* for an expression that came out zero, which is
	// unanimous and not a question — `let "x=5"` is 0 and `let "x=0"` is 1. What
	// this decides is the value that rule is then applied to:
	//
	//	let '1 @'  	0 under Yes, 1 under No
	//	let '0 @'  	1 either way
	//	let '1+2 @'	0 under Yes, 1 under No
	//	let '@'    	1 either way
	//
	// So Yes is not "a failure is success": the value before the byte is what
	// decides, and where nothing stood before it both answers agree.
	//
	// Only the byte the reader refuses outright, which is the discriminating
	// half and the reason this is not a statement about arithmetic failure at
	// large: `let '1+'` and `let '5 5'` are 1 under both answers, though a value
	// stood before those failures as well. The reader gave up mid-stream in one
	// case and the grammar rejected the whole expression in the others.
	//
	// Asked in `let` and nowhere else, because nowhere else can it be seen: the
	// same text inside `$(( ))` or `(( ))` abandons the line under Yes whatever
	// value stood, at 1 and at 2 respectively. Those three statuses are
	// deliberately not shared, and this is the narrow field that keeps them
	// apart.
	LetKeepsTheValueBeforeAnIllegalByte Answer

	// RegexQuotingMakesLiteral treats a quoted right operand of `=~` as a
	// literal string rather than keeping it a regex, so quoting a regex is
	// unportable in either direction.
	RegexQuotingMakesLiteral Answer

	// LastPipelineElementInCurrentShell runs the last command of a pipeline in
	// this shell, so `echo x | read v` sets v.
	LastPipelineElementInCurrentShell Answer

	// RedirectTargetIsAnOrdinaryWord expands a redirection's target the way an
	// argument is expanded — split into fields and matched as a pattern — and
	// requires the result to be exactly one word:
	//
	//	e="a b"; echo hi > $e      Yes refuses; No writes to `a b`
	//	e="x*";  echo hi > $e      Yes refuses where two files match and writes
	//	                           to the match where one does; No creates a
	//	                           file named `x*`
	//
	// No expands it and stops there: no splitting, no matching, whatever it came
	// to is the name. A tilde expands under both.
	//
	// Doing the Yes expansion and then quietly taking the first field is an
	// answer nothing gives, and it is the trap this axis exists to keep out:
	// `> $e` writing to `a`, and `> $e` with a pattern truncating whichever file
	// happened to match.
	RedirectTargetIsAnOrdinaryWord Answer

	// TypePrintsFunctionBody makes `type name` follow "name is a function" with
	// the function itself, reformatted, rather than stopping at the sentence.
	TypePrintsFunctionBody Answer

	// TypeEndsOptionsWithDashDash makes `type -- name` skip the `--`. Answering
	// No gives the builtin no options at all, so `--` is a name and gets
	// answered as one before the real names are.
	TypeEndsOptionsWithDashDash Answer

	// TypeNamesTheKindWithDashT gives `type` its `-t`, which answers one bare
	// word per name — keyword, function, builtin or file — and prints nothing at
	// all for a name it cannot account for, only the failing status. The
	// scripted form of the question: a word to compare against rather than a
	// sentence to parse.
	//
	// Answering No either refuses the letter as an option that does not exist or
	// reads it as a name like the rest of the operands.
	TypeNamesTheKindWithDashT Answer

	// TypeOptions is the rest of `type`'s letters, in the getopts spelling the
	// other optstrings use — `-a` for every resolution a name has, `-p` and `-P`
	// for the path alone, `-f` to leave the functions out.
	//
	// Empty means none beyond what the two axes above already give, which is
	// what a `type` with no options at all holds;
	// TypeEndsOptionsWithDashDash already says so.
	TypeOptions string

	// TypePSearchesPathPastTheShell is what `type -p` does about a name the
	// shell would answer itself: Yes searches PATH anyway and names the file,
	// No prints nothing at all and reports 0, so its `-p` speaks only when the
	// plain answer would have been a file.
	//
	// Asked only with the letter, so a preset without it never meets the
	// question.
	TypePSearchesPathPastTheShell Answer

	// TypePathAnswerIsASentence is the shape of `-p`'s answer: Yes words it the
	// way the plain `type` does — `echo is /bin/echo`, and the not-found
	// complaint for a miss — where No prints the bare path and meets a miss with
	// silence and the failing status.
	TypePathAnswerIsASentence Answer

	// TypeFSaysTheFunctionBack turns `-f` around: under Yes the letter *prints*
	// a function — the definition, laid out, nothing else — where under No it
	// leaves functions out of the search.
	TypeFSaysTheFunctionBack Answer

	// ArraysAreSparse makes an unassigned subscript no element at all, so
	// `a=(x); a[5]=y` is an array of two. Answering No reads the whole extent
	// and finds the gap empty, giving five.
	//
	// The store is sparse either way — only the reading differs — so this is
	// asked when an array *has* a gap and never otherwise, which is almost every
	// array there is.
	ArraysAreSparse Answer

	// OperatorDistributesOverStarSubscript applies an operator written on
	// `${a[*]}` — a trim, a replacement, a case change — to each element before
	// the join, so `${a[*]#a}` on `(aa ab)` is `a b`. Answering No joins first
	// and applies the operator to the joined string once, giving `a ab`.
	//
	// Only the star form is an axis. On `${a[@]}` the operator is applied to
	// each element wherever arrays exist, and the two readings of `[*]` often
	// agree — a suffix trim that stops at the last element, most patterns that
	// match nothing — so this is asked only when they differ.
	OperatorDistributesOverStarSubscript Answer

	// ExportCarriesFunctions gives `export` its `-f`, which writes a function
	// into a child's environment. Answering No has no way to carry a function at
	// all, and rejects the option as an option.
	ExportCarriesFunctions Answer

	// ExportTakesTheAttributeOff gives `export` its `-n`, which takes the export
	// attribute off a name and leaves the name itself alone.
	//
	// The same shape as ExportCarriesFunctions and for the same reason: what the
	// letter *means* is not in question anywhere it exists — the name stays set
	// and stops reaching a child — only whether the preset has it at all. So
	// there is no wording here, and a preset that says no sends `-n` down the
	// ordinary unknown-option path to collect its own refusal, which may or may
	// not be fatal.
	//
	// A wording field would be the wrong tool even where the refusal is not
	// fatal: unlike `-f`, which an implementation may know and refuse in words
	// of its own, `-n` is simply not a letter a preset that says no has.
	ExportTakesTheAttributeOff Answer

	// AnnouncesBackgroundJob prints the job number and the process id when a job
	// is backgrounded, before the next prompt.
	//
	// Only ever at a prompt: the announcement never reaches a script.
	AnnouncesBackgroundJob Answer

	// AnnouncesBackgroundJobWithoutTheMonitor keeps that announcement when the
	// monitor has been turned *off* — `set +m`, `unsetopt monitor` — at a prompt
	// where there is still somebody to tell.
	//
	// A *second* question and not a consequence of the first: an implementation
	// may announce nothing either way, announce with the monitor on and stop
	// when it is off, or carry on announcing — which is announcing something the
	// option says it is not managing. Asked only when the monitor is off and
	// there is somebody to tell, which is the one place the two answers differ.
	//
	// **The other end of the job is not an axis.** With the monitor off nothing
	// is said when the job *finishes*, so that is shared ground and
	// FinishedJobNotices simply stays quiet.
	AnnouncesBackgroundJobWithoutTheMonitor Answer

	// UnsetFunctionChecksTheName judges the operand `unset -f` was given as a
	// name, and refuses one that could not be a function name.
	//
	// Not the same question as the one below, and the two are independent: an
	// implementation may refuse `1x` while staying quiet about a well formed
	// name that is not defined, or the other way round.
	UnsetFunctionChecksTheName Answer

	// UnsetFunctionReportsMissing complains when `unset -f` names a function
	// that is not defined — about any name it does not hold, well formed or not.
	//
	// Unsetting a function that *is* there is quiet under both answers.
	UnsetFunctionReportsMissing Answer

	// LoneDashIsAnOption eats a `-` given to a builtin on its own instead of
	// passing it on as an operand.
	//
	// Only visible once something looks at the operands. `unset -` is quiet
	// under No because its bare form validates nothing, not because the dash was
	// eaten — `unset -v -`, which does validate, names the dash there. Under Yes
	// it is `not enough arguments` instead, because after the dash is eaten
	// there is nothing left to unset. Recorded as
	// `name/a-lone-dash-given-to-a-builtin` and
	// `name/unset-v-validates-the-lone-dash`.
	LoneDashIsAnOption Answer

	// LoopControlOutsideALoopIsFatal ends the script when `break` or `continue`
	// is run with no loop around it, instead of reporting it (or not) and
	// running the next command.
	//
	// With `echo t; break; echo after`, No prints `after` and ends at 0, and Yes
	// prints neither `after` nor anything on a later *line* either — so it is
	// the script that stops and not the line, which is why this reaches
	// fatalQuiet rather than controlAbandon. The status is then the preset's own
	// for a fatal error.
	//
	// The question is only about the misuse. A `break` with a loop around it is
	// ordinary control flow, and the count it is asked against is the dynamic
	// one a cloned Runner carries with it — which of those loops the word can
	// actually see is the pair of axes below.
	//
	// Separate from the wording, because the two questions cut differently: an
	// implementation may report and carry on, report and stop, or say nothing
	// and carry on. One field could not express the first of those three — see
	// Diagnostics.LoopControlOutsideALoop.
	LoopControlOutsideALoopIsFatal Answer

	// FunctionCallIsALoopControlBoundary stops a `break` or `continue` in a
	// function body from reaching the loops the *caller* is inside.
	//
	// With `f(){ break; }; for i in 1 2; do f; echo body; done; echo after`, Yes
	// prints `after` — the loop ended — and No prints `body body after`.
	//
	// A complaint alongside the No behavior is not a third answer: a boundary
	// leaves the word with no loop at all, which is the misuse above, and
	// implementations differ over saying so exactly as
	// Diagnostics.LoopControlOutsideALoop does. That the same family of
	// implementations answers this both ways is what says it is a decision and
	// not a consequence of something else.
	//
	// The reach is a count and not a flag: `break 2` from a body with one loop
	// in it stops at that loop under Yes, and reaches the caller's under No.
	//
	// Asked only where the answer decides something — a `break` that can see a
	// loop inside the call never reaches this.
	FunctionCallIsALoopControlBoundary Answer

	// SubshellIsALoopControlBoundary is the same question for `( )`, and it is a
	// second field because the two are not answered together.
	//
	// With `for i in 1 2; do ( break; echo insub ); echo body; done; echo
	// after`, No prints `body body after` — the subshell was left, quietly —
	// and Yes prints `insub body insub body after`.
	//
	// An implementation may make both boundaries or only the call one, and one
	// may part from a sibling on the *call* while agreeing with it here. A
	// single field would have to give one group the other's answer.
	//
	// The parentheses and nothing else. A command substitution and a pipeline
	// element are subshells too, and making `( )` a boundary makes neither of
	// them one: `for i in 1 2; do x=$( break ); done` and `do break | cat; done`
	// draw no complaint where the parenthesized form draws one per pass.
	SubshellIsALoopControlBoundary Answer

	// ReturnOutsideAFunctionIsRefused reports a `return` that has nothing to
	// return from and carries on, instead of ending the script with the status
	// it was given.
	//
	// Asked only where there is nothing to return from. Inside a function and
	// inside a sourced file it is obeyed under both answers, so the question is
	// about the one case they split on.
	ReturnOutsideAFunctionIsRefused Answer

	// StartupFileReturnCarriesItsArgument makes `return 3` at the top of a
	// startup file leave `$?` as 3, instead of leaving whatever the command
	// before it left.
	//
	// A startup file *is* a sourced script — a `return` in one is accepted, the
	// file stops being read there and nothing is said — so the question is only
	// what the argument does. Reading `$?` at the first prompt, with the rc file
	// as the whole probe:
	//
	//	rc                 Yes  No
	//	return 3             3   0
	//	false; return 3      3   1
	//	false; return        1   1
	//	(exit 5)             5   5
	//
	// The last two rows are what make this about the argument and nothing else.
	// No does carry a startup file's status out, and a `return` with no argument
	// means the last command's status under both, so the only thing No discards
	// is the number written on the `return` itself.
	//
	// Asked only of a `return` at the top level of the startup file. A `return`
	// inside a function the file calls, or inside a file the file sources,
	// carries its argument under both answers. So this is a property of the
	// outermost frame rather than of `return`.
	StartupFileReturnCarriesItsArgument Answer

	// UnknownConditionOptionIsAStatus makes `[[ -o name ]]` with a name this
	// shell does not have a status of its own with a complaint, instead of the
	// plain false that a name it has but has not set would give.
	//
	// Asked only at a name nothing would recognize. A name the shell has is read
	// the same way under both answers and nothing is asked.
	//
	// Not the same question as BadSetOptionNameFatal, and measured rather than
	// assumed to be: a name that ends the script when `set -o` is given it can
	// still leave `[[ ]]` running, with the complaint said and the next command
	// reached. One construct's refusal is not the other's.
	//
	// The status is a third value rather than a false, which the combining
	// operators show: `[[ ! -o zzz ]]` is 3 and not 0, so `!` leaves it alone,
	// and `[[ -o zzz || 1 == 1 ]]` is 0, so `||` goes on past it the way it
	// would past a false.
	UnknownConditionOptionIsAStatus Answer

	// BadSetOptionNameFatal ends the script when `set -o` is given a name this
	// shell does not have.
	//
	// Not the same question as BadOptionToSpecialBuiltinFatal, and measured
	// rather than assumed to be: a bad option *letter* to the same builtin is
	// fatal in fewer presets, and one of them does not so much as complain about
	// it. So an unknown name can be treated as worse than an unknown letter,
	// which is why this is a field of its own.
	BadSetOptionNameFatal Answer

	// CdLastPathOptionWins lets the last of `cd -L` and `cd -P` decide, so
	// `cd -P -L` is logical. Answering No gives `-P` the answer wherever it
	// appears, so both orders resolve.
	//
	// Asked only when both were given, because that is the only time the two
	// rules differ.
	CdLastPathOptionWins Answer

	// CdRefusesUnknownOption refuses a letter `cd` does not have rather than
	// reading the word as a directory. Answering No looks for somewhere called
	// `-Q` instead, which is what a `cd` taking two operands — `cd old new` —
	// does with a leading dash word.
	//
	// Only about an *unknown* letter. `-L` and `-P` are options under both
	// answers and are not asked about.
	CdRefusesUnknownOption Answer

	// CdHasQuietOption gives `cd` a `-q`, the one letter beyond `-L` and `-P`
	// that is in question at all.
	//
	// What the letter means is *hook suppression*: a change-of-directory
	// function and a member of its list both run on a plain `cd` and neither
	// runs on `cd -q`. It is not about printing — `cd -q -` still writes the
	// directory at an interactive prompt, and a CDPATH move stays silent with
	// the letter and without it — so an implementation that fires no such hook
	// has already done everything `-q` asks for.
	//
	// So the letter is carried to that site rather than swallowed at the option
	// loop: `cd` fires DirectoryChangeHook and `cd -q` does not, for the named
	// function and for a list member alike, and for `pushd -q` and `popd -q`,
	// which move through `cd` and are quiet for the same reason. It is a
	// *letter* and not an operand, which is the other half of why it is here —
	// without that, `cd -q /tmp` goes looking for a directory called `-q`.
	//
	// Asked only when a `q` is actually seen, so a preset without the letter
	// never reaches the question and answers the word the way it answers any
	// other letter it does not have — see CdRefusesUnknownOption, which is the
	// next question when this one says no.
	CdHasQuietOption Answer

	// HookListSuffix is what a hook's list of *extra* function names is spelled
	// by: the hook's own name plus this. With `_functions`, `precmd` reads
	// `precmd_functions` as well and `chpwd` reads `chpwd_functions`.
	//
	// Empty is an implementation whose hooks are the named function and nothing
	// else — which, where there are no hooks at all, is "no hooks" said once.
	//
	// Not decoration: a registration helper may define no function of the hook's
	// name at all and only append to the list, so reading the named function
	// alone would find a correctly registered hook and run nothing.
	//
	// Here rather than on repl.HookStyle, where it began, because the hook
	// *sites* are on both sides of that line: a pre-prompt hook fires in a
	// prompt loop and a change-of-directory hook fires inside `cd`, which is a
	// builtin and cannot reach up into a front end. One home for the suffix, one
	// [Runner.HookChain] that applies it, and no way for the two sites to come
	// to disagree about what a hook's list is called.
	HookListSuffix string

	// DirectoryChangeHook names the function this shell runs after `cd` has
	// moved it. Empty is an implementation without one.
	//
	// It is the *last* thing `cd` does, after the directory has moved and after
	// anything `cd` itself prints: `cd -` writes the old directory and *then*
	// the hook runs, and a CDPATH move writes the directory it found and then
	// runs it. So a hook cannot land in the middle of `cd`'s own output.
	//
	// **On the move, not on the change.** `cd` to the directory the shell is
	// already in fires it — twice in a row fires it twice, with `$PWD` and
	// `$OLDPWD` both the same. A `cd` that *fails* does not: it reports its
	// error, leaves `$OLDPWD` alone and runs nothing.
	//
	// `$PWD` is where the shell now is and `$OLDPWD` where it was, both already
	// set when the hook runs, and the hook is told **no arguments** — `$#` is 0
	// in the named function and in every member of the list.
	//
	// Everything that moves through `cd` fires it and nothing else does.
	// `pushd`, `popd` and a bare directory name under `autocd` are `cd` here,
	// and all three fire it; assigning to `PWD` is not a move and fires nothing.
	// A `cd` inside a function fires it at the `cd`, and a `cd` inside a
	// subshell or a command substitution fires it in there, where the move is.
	//
	// A hook that itself calls `cd` fires the hook again, and there is no guard
	// against that beyond the ordinary recursion limit — a pair of hooks moving
	// back and forth ends at it. Nothing special is done here either: the call
	// goes through [Runner.CallFunction] and meets whatever limit an ordinary
	// function call meets.
	DirectoryChangeHook string

	// ExitHook names the function this shell runs on the way out. Empty is an
	// implementation without one.
	//
	// **After the EXIT trap, not before it.** A script with both writes the
	// trap's line and then the hook's, and an interactive session left with
	// `exit` or with end-of-input writes them in that same order. So the trap is
	// the script's last word and the hook is the shell's, which is the order a
	// plugin's teardown is written against — a status daemon stopping itself, an
	// async worker cleaning up.
	//
	// The hook is told **no arguments** — `$#` is 0 in the named function and in
	// every member of the list — and every one of them is told the status the
	// shell is exiting with. That is the entry status and not a running one:
	// with the shell exiting 4, a named hook that returned 5 and a member that
	// ran `false` are both followed by a member reading `$?` as 4.
	//
	// **`return` cannot change the status and `exit` can.** Returning 5 leaves a
	// shell exiting 4 exiting 4. `exit 9` in the named hook and `exit 11` in a
	// member leave it exiting 11 — the *last* `exit` wins — and, unlike every
	// other chain here, an item that exited does **not** stop the ones after it.
	// There is no session left for `exit` to end, so all it can do is record a
	// status. See Runner.runExitHook, which is where that one difference from
	// FireChain's rules lives.
	//
	// **Not on a signal death.** A script killed by SIGTERM runs neither its
	// EXIT trap nor this hook, which is the same split ExitTrapRunsOnSignalDeath
	// records for the trap.
	//
	// **The subshell case is not this site.** A subshell that calls `exit`
	// explicitly fires the hook in there, while a subshell that merely falls off
	// its end does not. That is a firing at a subshell's own exit and not at the
	// shell's, and subshells here do not pass through Finish at all, so it is
	// written down rather than modeled: nothing reaches it, and a guess about it
	// would be a plausible wrong answer.
	ExitHook string

	// ChildInterruptEndsTheScript stops the script when a child was ended by an
	// interrupt, instead of carrying on with the next command. For SIGINT alone
	// — QUIT, TERM, HUP, USR1 and PIPE are all carried on from under either
	// answer.
	//
	// It ends the whole script rather than the construct around it: from inside
	// a loop, the loop and everything after it are abandoned too. The status is
	// 128 plus the signal, which is not the same answer as for a command killed
	// by one — that is 256 plus it.
	ChildInterruptEndsTheScript Answer

	// ReportsAnyKilledPipelineElement remarks on a signal that ended an element
	// of a pipeline other than the last.
	//
	// Answering No reports only the element whose status the pipeline takes, so
	// `sh -c 'kill -ABRT $$' | cat` is silent while the same command as the
	// *last* element is not. Yes says the same thing wherever the element
	// stands.
	//
	// Unreachable where nothing is said about a killed command at all, so the
	// question never arises for such a preset.
	ReportsAnyKilledPipelineElement Answer

	// ReportsACommandKilledBySignal says out loud that a signal ended a command,
	// rather than leaving the status to carry it alone. The answer is the same
	// with a terminal and without one, so it is not the prompt-only rule that
	// governs a background job's announcement.
	//
	// Not asked for the two signals nothing reports. ^C and a broken pipe are
	// how a command is meant to end, and both answers stay quiet about those, so
	// there is no disagreement there to put to a preset.
	ReportsACommandKilledBySignal Answer

	// JobsShowBackgroundCommand puts the command of a `&` job in a `jobs`
	// listing, rather than an empty column or a placeholder.
	//
	// Only for a `&` job, which is the whole reason this is not a question about
	// rendering a command at all: an implementation that leaves it out here
	// still prints the command of a job it stopped itself. It kept nothing for
	// this kind of job, and the listing is where that shows.
	JobsShowBackgroundCommand Answer

	// JobsListNewestFirst puts the most recent job at the top of a `jobs`
	// listing, rather than listing oldest first.
	//
	// An even split, which is the usual shape here and the reason this is a
	// field rather than a choice: there is no ordering of the implementations
	// that explains it.
	JobsListNewestFirst Answer

	// JobsListFinishedJobs includes a job that has already ended in a `jobs`
	// listing, once, before forgetting it. Answering No drops a finished job
	// without ever mentioning it.
	//
	// The forgetting is not the axis and is not optional: a finished job is
	// reported at most once under either answer, so a second `jobs` shows
	// nothing. Keeping them would grow a listing for the length of the session.
	JobsListFinishedJobs Answer

	// SetReportsEveryBadOption makes `set` report every option word it cannot
	// use before it gives up, rather than stopping at the first.
	//
	// Easy to miss, because a single line is what No produces for two different
	// reasons: the refusal may be *fatal*, so the loop never reaches the second
	// word, or the loop may simply stop. Yes is fatal as well and still prints
	// every line first, which is what separates it from both.
	//
	//	set -q -z             two lines, then one usage block, status 2
	//	set -q -z -y          three lines, then one usage block
	//	set -qz               two lines — a bundle is one word, several letters
	//	set -q -o nosuch -z   three lines, in the order written
	//
	// So the unit is the **letter**, not the word: a bundle is not one refusal,
	// which was the open question. Long names count too and interleave with
	// letters in argument order.
	//
	// The usage block is printed **once**, after all of them, and not one per
	// word — which is what makes this more than "keep looping", since the block
	// is written by the same helper that writes each sentence. The fatality is
	// applied after them as well, so it ends the script *after* the reports
	// rather than instead of them.
	//
	// This is the rule Runner.builtinNames already follows for bad *operands*.
	// The two questions are answered by different presets, which is what keeps
	// them separate fields: one may report every bad name to `export` and stop
	// at the first bad option to `set`, and another do the reverse.
	//
	// Asked for the builtin only. The front end's own option parse is a
	// different surface with its own quirks, so it keeps stopping at the first
	// until those are settled.
	SetReportsEveryBadOption Answer

	// ShiftPastEndFatal ends a non-interactive shell when `shift` runs off the
	// end.
	ShiftPastEndFatal Answer
	// ReadonlyReassignmentByDeclarationFatal ends the script when a declaration
	// utility assigns to a readonly name — `export x=2`, `typeset x=2` — rather
	// than reporting it and carrying on.
	//
	// A different set of answers from the plain assignment above, which is what
	// makes it a question of its own: an implementation can stop for `x=2` given
	// as an argument and never stop for this one.
	ReadonlyReassignmentByDeclarationFatal Answer

	// SetArrayBadNameLeavesZeroFromCommandString makes `set -A` refuse a name
	// that is not one and leave the shell exiting **0**, where the same refusal
	// from a script file leaves 1.
	//
	// Unreachable without the letter — SetArrayLetter is read rather than asked,
	// so a preset whose `set` has no `-A` never arrives here.
	//
	// The complaint is byte-identical on both routes and the next command runs
	// on neither, so the refusal is fatal either way and only the status moves.
	//
	// It is this refusal and no other, which is what makes it a field of its own
	// rather than a route rule about bad names or about `set`. Every neighbor
	// leaves 1 from the same route:
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

	// FailedExpansionAbandonsTheLine ends the *line* a failed expansion happened
	// on and carries on at the next one, rather than ending the shell. A bad
	// substitution, a division by zero, a bad subscript and an arithmetic
	// expression the parser refused are all this failure.
	//
	// It has to be measured over both statement separators, which is the pairing
	// that makes it visible and the reason it was missed:
	//
	//	echo pre; echo "${bad}"; echo after     no `after`, status 1
	//	echo pre / echo "${bad}" / echo after   `after` runs, status 0
	//
	// The same 2x2 by `-c` and by a script file, so the *route* has nothing to
	// do with it. What ends is the command list, and a list ends at a newline —
	// so `;` between the two commands puts them in the same unit and a newline
	// does not. Every enclosing shape gives up the same way and the shell
	// carries on at the next top-level statement: in a loop, a function body, an
	// `if`, a group and a sourced file, where the commands after the `.` still
	// run.
	//
	// It is controlAbandon, which is exactly this and is what a readonly
	// reassignment already uses. Treating the failure as controlExit instead
	// ends the whole file over one unreadable expansion — the shape that makes a
	// diagnostic useless, since the point of naming a construct is that the next
	// line still runs and the next gate becomes visible.
	//
	// Not the two parameter failures that look like it. `set -u` on an unset
	// name and `${x?word}` end the *shell* under every answer, by both routes
	// and with either separator, so they are fatalExpansion's and stay there.
	//
	// The core leaves it unanswered: this is a genuine disagreement, and the
	// path already asks an unanswered axis there — FatalErrorStatusIsOne — so a
	// core run says which preset it needs rather than picking one.
	FailedExpansionAbandonsTheLine Answer

	// AssignThroughExpansionMayNameAPositional lets `${1:=word}` assign to a
	// positional parameter, rather than refusing the expansion fatally. A real
	// disagreement and not a wording one.
	//
	// `${@:=word}` and `${*:=word}` are refused unanimously and are **not** this
	// axis; only the positional splits. `${2:=abc}` and `${10:=abc}` answer with
	// their own row, so it is the *shape* of the name and not the number.
	//
	// The core leaves it unanswered: a preset that has chosen nothing is told
	// which one it needs rather than being given one reading of an operator
	// every preset has.
	//
	// A run-time question, asked only when the operator fires: `set -- p;
	// ${@:=abc}` is `p` at status 0 under every answer, and
	// `if false; then echo ${@:=abc}; fi` is silent under every answer.
	AssignThroughExpansionMayNameAPositional Answer

	// ReadonlyReassignmentFatal ends the script when a readonly variable is
	// assigned.
	//
	// Measured with a plain assignment in a script file — adding a redirect
	// makes it a command and reverses the answer, which is the
	// contaminated-probe trap docs/spec/oracle.md records.
	ReadonlyReassignmentFatal Answer

	// DeclarationMayShadowAReadonly lets a declaration inside a function make a
	// local of a name the shell has frozen:
	//
	//	typeset -r x=1
	//	f() { local x=2; echo "in=[$x]"; echo running; }
	//	f; echo "st=$? out=[$x]"
	//
	// Yes answers `in=[2]`, `running`, `st=0 out=[1]` — the local shadows the
	// frozen name, the shadow is an ordinary local, and the outer value is
	// untouched when the function returns. No refuses the declaration, leaves
	// the *outer* value in view, and **carries on**.
	//
	// Every preset answers, each asked in the words it has, which is what makes
	// this a question for all of them rather than the two-way one it looks like
	// from `typeset -r` and `local` alone. An implementation with no `local`
	// answers through its declaration keyword in a keyword function — see
	// TypesetLocalNeedsKeywordFunction — and asked through `f() { … }` instead
	// it has no scope to shadow into, so the declaration is the ordinary
	// refusal, which is that field and ReadonlyReassignmentFatal rather than
	// this one. Reading that fatality as this axis's answer is the mistake to
	// avoid. One with no `typeset -r` answers through `readonly`, the freeze
	// POSIX spells.
	//
	// A different split from ReadonlyAttributeCanBeRemoved below. That the two
	// questions divide the presets differently is what makes them two questions.
	//
	// It is one field for the whole family and not one per spelling: `local
	// x=2`, `local x`, `typeset x=3`, `local -r x=4` and `local y=1 x=5 z=2` are
	// all taken alike under Yes and all refused under No, with the builtin
	// reporting 1 each time. Splitting them would have been five fields whose
	// answers can only ever agree.
	//
	// Where the answer is **no**, three things follow. The refusal names the
	// builtin — `local: x: readonly variable`, which is
	// ReadonlyVariableInDeclaration and the reason ReadonlyRefusalNamesBuiltin
	// has entries for the declaration words. The builtin reports 1 and the
	// *function* runs on, so `local x=2 || …` fires its right-hand side and the
	// next line still runs. And the remaining operands are still declared:
	// `local y=1 x=5 z=2` leaves `y` and `z` local and only `x` refused.
	//
	// Where it is **yes** the shadow takes the attribute with it: the local cell
	// is writable and the outer name is frozen again when the function returns.
	// Asked only when a declaration meets a name that is already frozen, so
	// nothing else reaches the question.
	DeclarationMayShadowAReadonly Answer

	// ReadonlyAttributeCanBeRemoved lets a plus form take the readonly attribute
	// off a name — `typeset +r x` — leaving it writable again:
	//
	//	typeset -r s=1; typeset +r s; s=9; echo "st=$? s=[$s]"
	//
	// Yes is silent at status 0 and then `s=[9]`, the attribute gone. No refuses
	// through the declaration, and whether that ends the script is a separate
	// question.
	//
	// `declare +r` is the same word under its other spelling wherever both
	// exist, and so is an `export +r` where `export` is the declaration builtin
	// with an export flag. `readonly +r` is not: that builtin takes no `r`,
	// since the attribute is the whole of what it means. An implementation with
	// no declaration builtin at all cannot ask the question.
	//
	// This splits the presets differently from DeclarationMayShadowAReadonly
	// above. Two questions rather than one, and that difference is what proves
	// it.
	//
	// A preset that says no still has to say *which* no, and it already does:
	// the refusal is the ordinary readonly refusal through a declaration, so the
	// wording and the fatality come from ReadonlyReassignmentByDeclarationFatal
	// and the diagnostics beside it rather than from anything of this field's
	// own. That is measured and not an economy — an implementation that ends the
	// script over `typeset +r` ends one over `export x=2`, and one that carries
	// on carries on from both.
	//
	// Asked only for a plus form on a name that is *already* frozen. A
	// `typeset +r` on a free name reports 0 and says nothing wherever the word
	// is spelled at all, which is the shape a script actually writes — making
	// sure a name is writable — and it must not reach an axis.
	//
	// A Yes has one limit that is not an axis: a *special* parameter refuses the
	// change whatever this says, `typeset +r EPOCHSECONDS` being `can't change
	// type of a special parameter`. That is a fact about specials rather than
	// about the attribute.
	ReadonlyAttributeCanBeRemoved Answer

	// DeclaredNameWithoutValueIsEmpty gives a name a value when it is declared
	// without one: `local u` or `typeset u`. Under Yes `${u-UNSET}` is empty and
	// under No it is UNSET — the name exists either way, and only Yes considers
	// it set.
	DeclaredNameWithoutValueIsEmpty Answer
	// ExportLetterDeclaresAGlobal makes the `x` letter on a declaration ask for
	// `-g` as well, so `typeset -x v=1` written inside a function declares no
	// local and the name outlives the call:
	//
	//	f(){ typeset -x lxx=1; }; f; echo "[$lxx]"
	//
	// is `[1]` under Yes and `[]` under No.
	//
	// `local -x` is the control and both answers agree on it — `[]` — so the
	// question is about the letter under the *other* words and not about `-x` in
	// general. The same holds for `declare`, and for the words that carry an
	// attribute in their own name: `readonly -x`, `integer -x` and `float -x`
	// all reach past the function under Yes.
	//
	// The exemption is measured too: a name this scope has *already* made local
	// stays local, so `f(){ local m=1; typeset -x m; }` leaves the caller's m
	// alone. So the letter decides where a declaration lands rather than what it
	// does to a name that is already here.
	//
	// Asked only inside a function, only under a word that is not `local`, and
	// only where the name is not already local — outside that shape the two
	// answers do the same thing.
	//
	// Silent: a script that exports a working name inside a function leaves it
	// behind under one answer and not the other, and nothing is said either way.
	ExportLetterDeclaresAGlobal Answer
	// ValuelessDeclarationOfAHeldNameListsIt writes the name back when a
	// declaration names it, assigns nothing, and carries no letters at all —
	// provided the name already holds something in the cell being declared:
	//
	//	a=(x y); typeset a; s=str; typeset s; unset u; typeset u; echo done
	//
	// is `a=( x y )`, `s=str`, `done` under Yes and `done` alone under No.
	//
	// Three facts in the one line, and each is a limit on the rule rather than a
	// special case. A name holding **nothing** prints nothing, which is why this
	// is not "a declaration with one operand lists". The value is unchanged
	// either way, so the listing is all that happens. And the spelling is the
	// bare assignment — `a=( x y )`, not `typeset -a a=…` — which is
	// BareDeclarationListing's row and not `-p`'s.
	//
	// A **letter** suppresses it: `n=5; typeset -i n` and `typeset -g s` are
	// both silent under Yes, which is what makes the rule "no options at all"
	// and also why `readonly`, `export`, `integer` and `float` never do it —
	// each of those words is an attribute already. `local` does, on a name its
	// own scope has already declared: `f(){ local s=1; local s; }` writes `s=1`.
	//
	// Inside a function a declaration that takes a *fresh* cell finds nothing
	// standing in it, so nothing is listed — which is the answer that keeps a
	// shell from narrating every `local` in every function.
	//
	// The quiet answer is the one to get wrong carefully: a script whose output
	// matches under No diverges the moment it is run under Yes.
	ValuelessDeclarationOfAHeldNameListsIt Answer
	// ScalarOverACompoundIsAnInconsistentType refuses a declaration that assigns
	// a plain word to a name whose cell is really holding an array or a keyed
	// table, and ends the script over it:
	//
	//	b=(x y); typeset b=q; echo "st=$? [${b[*]}]"; echo tail
	//
	// Yes is `typeset: b: inconsistent type for assignment` at status 1 and the
	// script ends; No is `st=0 [q]` and `tail`.
	//
	// It is the **declaration** that refuses and not the store: `b=(x y); b=q`
	// is taken under both answers and leaves a scalar, which is
	// ScalarAssignedOverACompoundReplacesTheName's question and a different one.
	// Nor is it the fresh cell — `f(){ local b=q; }` over a caller's array is
	// taken under Yes, because the cell that declaration writes is new and holds
	// nothing. What is refused is a declaration reaching a cell that is *really*
	// compound, which is what the top level and `-g` have in common.
	//
	// The refusal has a direction, and that asymmetry is the measurement that
	// pins it: the mirror image is **taken**. `b=1; typeset -a b` is a silent
	// empty array under Yes too, so a name is not simply frozen in its kind.
	//
	// `readonly b=q` and `export b=q` refuse it in the same words with their own
	// name in the sentence, so the rule belongs to the declaration utilities
	// rather than to one word.
	//
	// Silent where it is answered wrongly, and worse than a wrong value: the
	// script that should have stopped carries on, so everything downstream of
	// the line runs under one answer and never runs under the other.
	ScalarOverACompoundIsAnInconsistentType Answer
	// ReadonlyRecordsTheCompoundAttribute makes `readonly -a` and `readonly -A`
	// declare an array and a table the way `typeset -a` and `typeset -A` do,
	// rather than freezing a name and saying nothing about its kind:
	//
	//	f() { readonly -a a; typeset -p a; }; f
	//
	// is `typeset -ar a=(  )` under Yes and `declare -r a` under No.
	//
	// The keyed half moves with it, so it is one question and not two. An
	// implementation with no such letter on the word refuses the option and
	// cannot ask the question at all.
	//
	// Only the listing observes it under No: a frozen name cannot then be
	// assigned an array to tell the two apart.
	ReadonlyRecordsTheCompoundAttribute Answer
	// TypeLetterAndAnArrayLiteralIsAnInconsistentType refuses a declaration that
	// names a *type* — the integer or the float letter — and assigns an array
	// literal to the same name, and ends the script over it:
	//
	//	typeset -ia z=(1 2); echo "st=$?"; typeset -p z; echo tail
	//
	// Yes is `typeset: z: inconsistent type for assignment` at status 1 and the
	// script ends; No takes it and lists the array.
	//
	// The array letter is not what triggers it, which is the measurement that
	// says this is about the *type* and not about a pairing: `typeset -i z=(1
	// 2)` with no `-a` is the same refusal, `typeset -F 3 z=(1 2)` and
	// `typeset -E 3 z=(1 2)` are too, and `typeset -ua q=(ab cd)` is taken. The
	// case letters name what happens *to* a value and the numeric ones name what
	// the value *is*, so only the second kind is two things at once with an
	// array. The width letters side with the case ones.
	//
	// It is the **letter on this line** and not the attribute the name is
	// carrying, which is the second discriminating row: `typeset -i z; typeset
	// z=(1 2)` is taken under Yes and leaves `typeset -a z=( 1 2 )`, the integer
	// letter simply lost. So the question is asked of a declaration and never of
	// a store.
	//
	// All four declaration utilities refuse it, each naming itself, and the
	// wording is Diagnostics.InconsistentType, shared with
	// ScalarOverACompoundIsAnInconsistentType — one sentence, two questions that
	// reach it.
	//
	// Silent where it is answered wrongly, and in the worse direction: the
	// script that should have stopped carries on holding an array of the type it
	// was refused.
	TypeLetterAndAnArrayLiteralIsAnInconsistentType Answer
	// NumericTypeWithNoValueReachesAChildAsZero hands a child `0` for an
	// exported name whose declaration named a numeric type — the integer or the
	// float letter — and which holds no value at all:
	//
	//	typeset -ix Z; env | grep '^Z='
	//
	// is `Z=0` under Yes and nothing under No. The name really is unset under
	// Yes: `${Z+set}` is empty and `typeset -p Z` writes the declaration with no
	// value.
	//
	// It is the numeric letters and no others. `typeset -u U; export U`,
	// `typeset -a A; export A` and a plain `typeset P; export P` tell that child
	// nothing.
	//
	// A No for the one-command form still tells a child about the name when a
	// *second* declaration names it — `typeset -i Z; export Z` — but that is the
	// ordinary store being exported once the name owns its value, and not this.
	// See Runner.declarationOwnsTheStandingEmpty, which is where the two part.
	//
	// Silent where it is answered wrongly, and only a real child can see it.
	NumericTypeWithNoValueReachesAChildAsZero Answer
	// NumericAttributeReplacesTheArrayAttribute makes the integer and float
	// letters take the *array* letter off a declaration that writes both, so the
	// name is a scalar of that type rather than an array of it: `typeset -ia z`
	// lists as `typeset -i z=0` under Yes and keeps both letters under No.
	//
	// Within one word the numeric letter wins whichever order it is written in,
	// so this is not the last-one-speaks rule the case letters follow. Across
	// two words it is: `typeset -a z; typeset -i z` collapses and
	// `typeset -i z; typeset -a z` is an array, which the compound axes already
	// answer from the other side.
	//
	// The *valued* form of the same combination is a refusal rather than a
	// collapse — see TypeLetterAndAnArrayLiteralIsAnInconsistentType — so the
	// two together are the whole of what Yes does with the pairing.
	//
	// It is also what makes the array letter worth recording at all: a name this
	// answer left a scalar must not count as declared-an-array when a later
	// array literal decides whether to start it over.
	NumericAttributeReplacesTheArrayAttribute Answer
	// ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver makes `a=(x y)`
	// re-create a name whose declaration never wrote the array letter, dropping
	// the letters that say what its values are — where a name the letter *was*
	// written for keeps them and the literal simply fills it.
	//
	// The `-l` pair is the discriminating one, because it is the same two lines
	// differing only in the array letter:
	//
	//	typeset -l e;    e=(AB Cd)     the letter goes
	//	typeset -la f;   f=(AB Cd)     the letter is kept
	//
	// So whether the name was *declared* an array is the whole of what it turns
	// on, and neither the value the name holds nor the kind it currently is can
	// answer it. That is why the letter has to be recorded — and it already is:
	// markIndexed puts an empty array under the name, which is exactly the state
	// compoundNameHolds documents its `len(a) > 0` guard against. See
	// Runner.nameIsAnArray.
	//
	// Distinct from ArrayLiteralAssignmentStartsTheNameOver, which asks the same
	// thing of a name that is *already holding* an array. A name may be one and
	// not the other in either direction, and the two are answered differently.
	ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver Answer
	// AppendedArrayLiteralOverANameNotDeclaredAnArrayStartsItOver is the same
	// question asked of `a+=(x y)`, and it is a second axis because the two can
	// be answered differently:
	//
	//	typeset -i p=3; p+=(5+5); typeset -p p
	//
	// keeps the letter and evaluates the append under one answer, and lets the
	// letter go with the store under the other. `typeset -l t=A; t+=(B)` is the
	// same split, so it is the operator and not the letter that parts them.
	//
	// Where the assign form is unanimous and the append form is not, that is the
	// whole point of the second field: an attribute's answer on the way in is
	// not its answer on a join.
	AppendedArrayLiteralOverANameNotDeclaredAnArrayStartsItOver Answer
	// NumericAttributeReplacesTheCaseAttribute makes the integer and float
	// letters take a case attribute off the name they are given, rather than
	// standing beside it: `typeset z=1; typeset -l z; typeset -i z` lists
	// without the `-l` under Yes and with both under No.
	//
	// `-F` is the same letter's family and behaves the same way.
	//
	// The converse is a separate question and is answered differently — see
	// CaseAttributeReplacesTheNumericAttribute, which is what makes this a
	// direction rather than a set.
	//
	// Only a listing observes it, so the whole cost of the wrong answer is a
	// `typeset -p` that says more than the shell would.
	NumericAttributeReplacesTheCaseAttribute Answer
	// CaseAttributeReplacesTheNumericAttribute is the other direction: `-l` and
	// `-u` take the integer or float letter off the name they are given.
	//
	// Yes holds one family — a name carries one letter saying what its values
	// are, and the last one written speaks. No may replace in the other
	// direction only, or in neither. Two axes rather than one three-valued
	// answer because the two directions were measured separately and can be
	// answered differently.
	//
	// The value the earlier letter produced stays: `typeset -F z; typeset -l z`
	// keeps the rendered float, so what goes is the rendering of what comes next
	// and not what is already there — the same reading `+i` and `+F` have.
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
	// An implementation with no declaration builtin carrying a type letter never
	// arrives at the question.
	InheritedValueSurvivesADeclaredType Answer
	// CompoundElementsGoThroughTheAttribute folds what is written to one element
	// of an array or a keyed table through the attribute the *name* carries, the
	// way a scalar assignment already does.
	//
	// For a scalar this needs no answer: `typeset -i n; n=3+4` is 7 and
	// `typeset -u d; d=again` is AGAIN wherever the letter is spelled. An
	// element is where the answers split:
	//
	//	typeset -ia a=(1 2); a[1]=3+4     Yes `1 7`
	//	typeset -ua q=(ab cd); q[1]=ef    Yes `AB EF`   No `ef cd`
	//	typeset -A m; typeset -i m
	//	m[k]=7+7                          Yes `14`
	//
	// A No may not fold an array's elements at all: its case letters reach a
	// scalar's expansion and stop there, and its integer letter never meets an
	// array in the first place — see CompoundMeetingANewAttribute, where that
	// letter replaces the array with a scalar. So for such a preset the answer
	// is read off the case letters, which are the only ones it can be asked
	// about here.
	//
	// Asked only where a name with one of these attributes has an element
	// written to it, so an array with no attribute needs no answer.
	CompoundElementsGoThroughTheAttribute Answer

	// CaseAttributeFoldsWhenRead decides *when* the case attributes act: once,
	// on the value being stored, or on every read of it.
	//
	// The difference is invisible in the value — `$v` is `ab` under both — and
	// shows in the two places that see the store itself. Over `typeset -l lo=AB`:
	//
	//	        $lo   typeset -p lo        typeset +l lo; $lo
	//	store   ab    typeset -l lo=ab     ab
	//	read    ab    typeset -l lo=AB     AB
	//
	// The third column is the discriminating one: taking the attribute off
	// reveals what the store really holds, and only the folding-on-read answer
	// has anything left to reveal. The listing is the other half — folding on
	// the way in leaves no way back to the text the assignment carried, so `-p`
	// cannot write the declaration it read.
	//
	// Two consequences of folding on the read, neither derivable from the row
	// above:
	//
	//   - An append joins the *stored* text. `typeset -l lo=AB; lo+=CD` lists as
	//     `ABCD` and reads as `abcd`.
	//   - A pattern operator matches the *folded* text, because it is a read
	//     like any other: `${v/A/x}` leaves `ab` alone, where the storing answer
	//     never had an `A` to match either. The two agree there and differ on
	//     `${v/a/x}`, which is `xb` under the storing answer.
	//
	// Arrays are outside this on both answers and for different reasons, so it
	// is not asked of one: folding on the read does not fold an array's elements
	// at all, and folding on the way in has
	// CompoundElementsGoThroughTheAttribute for the same question.
	//
	// Asked only where a name carries `-l` or `-u`, which is where the two
	// answers can be told apart.
	CaseAttributeFoldsWhenRead Answer
	// ArrayLiteralAssignmentStartsTheNameOver makes `a=(x y)` *re-create* the
	// name — the attributes it carries and all — rather than replacing only its
	// elements.
	//
	//	typeset -ia z=(1); z=(5+5 6+6); typeset -p z; z[0]=3+4
	//	  No    the letter stands, values `10 12`, then `7 12`
	//	  Yes   the letter is gone, values `5+5 6+6`, then `3+4 6+6`
	//
	// Yes keeps neither the fold nor the letter: the listing has lost the `-i`,
	// and the element write after it is not folded either, which is what says
	// the attribute is *gone* rather than merely bypassed by that one
	// assignment. The case letters may be answered the other way by the same
	// preset, which is why this is read off whichever letter a preset can be
	// asked about.
	//
	// The same idea `unset` is a rule about, and that
	// InheritedValueSurvivesADeclaredType is the other side of: a name whose
	// whole value is replaced may be a *new* name.
	//
	// Asked only for the plain assignment spelling, and this is where the
	// spelling earns its own question: a declaration's own operand —
	// `typeset -ia d=(5+5 6+6)` — folds under **both** answers, so the letters
	// cannot have gone there. It is `syntax.Assign.Operand` that tells the two
	// apart, and an append is not it either: `f+=(8+8)` folds under both.
	//
	// And asked only where the name has something to start over: the *first*
	// array literal a declared name receives keeps the letter and folds under
	// both, so what re-creates the name is replacing a value it is already
	// holding.
	//
	// The shape that reading leaves out has an axis of its own: `typeset -i a;
	// a=(5+5 6+6)`, where the declaration named no array letter at all. What
	// that turns on is whether `-a` was *written*, so it is a different question
	// and the two are answered differently — see
	// ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver and its append half.
	//
	// And asked only for the *indexed* literal. A keyed one keeps the attribute
	// under both, where the indexed spelling on the same line loses it. So it is
	// this spelling and not "replacing a compound value" in general — a wider
	// reading would take the attribute off a table nothing takes it off.
	ArrayLiteralAssignmentStartsTheNameOver Answer
	// ScalarAppendedToAnArrayBecomesANewElement decides where `a+=x` puts the
	// value when the name is holding an *array*: after the last element, or
	// joined onto the first one.
	//
	// With `a=(1 2); a+=x`, Yes leaves three elements `1 2 x` and No leaves two,
	// `1x 2`. The count is what tells the two apart from the outside; the
	// listing is what says which element moved. An implementation with no arrays
	// reports the parenthesis, which is the absence rather than a third answer.
	//
	// The join is at the *base* rather than at the lowest subscript standing,
	// which a sparse array shows: under No, `a=([5]=q); a+=x` puts `x` at
	// element 0 whether or not there is one there, and `q` is left where it was.
	// The empty string is a value on both sides of the axis — `a+=""` leaves No
	// alone and gives Yes a third element that is empty — and the value joins
	// whole however many words it looks like: `a+="p q"` is one element under
	// both.
	//
	// Asked only where the name is holding an array. An *unset* name and a name
	// holding a scalar are the string append, which is unanimous and core:
	// `unset a; a+=x` leaves a plain scalar. The array-literal spelling `a+=(x)`
	// is not this question either — it adds an element wherever arrays exist,
	// which is why that one has no field. See Runner.appendScalarToArray.
	ScalarAppendedToAnArrayBecomesANewElement Answer
	// ScalarAssignedOverACompoundReplacesTheName decides what a plain `a=x` does
	// to a name that is already holding an array or a table: the value becomes
	// the whole of the name, or it lands on the compound's first element and the
	// rest stays where it is.
	//
	// With `a=(1 2 3); a=x`, Yes leaves a scalar `x` — the type reads back
	// `scalar` and the array is gone rather than merely hidden — and No leaves
	// three elements `x 2 3`. An implementation with no arrays refuses the
	// parenthesis, which is the absence rather than a third answer.
	//
	// A **table** answers the same way, so it is this one field and not two:
	// `typeset -A m; m=([k]=v); m=x` splits exactly as the array does. Where the
	// two kinds of compound part is a *declaration* adding an attribute, which
	// is ScalarUnderAnArrayDeclaration against ScalarUnderATableDeclaration;
	// nothing parts them here.
	//
	// The element written is the compound's *first* — the array base, and the
	// key `0` — whether or not there is anything there already: under No,
	// `a=([5]=q); a=x` leaves `q` where it is and the array grows. The same
	// place `a+=x` joins, which is why the two axes read one arrayBase between
	// them.
	//
	// Asked wherever a scalar is *stored* and not only at an assignment
	// statement, which is the whole point of the field: `for a in x y z`,
	// `read a`, `select`, `getopts`, `printf -v` and `${a::=x}` all set a name,
	// and leaving an array standing at any of them makes the name read back as
	// the array on every pass. A completion system that reuses one name as an
	// array and then as a loop variable is where that shows.
	//
	// Not asked for the store that keeps `$a` answering for an array `a` — see
	// assignedAsTheCompoundView, which is that write and no other.
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
	// A second field and not a widening of the one above, because the two
	// letters can be answered differently: an implementation may leave
	// `b=1; typeset -a b` a plain scalar and take `a=1; typeset -A a` to a table
	// holding the old value. Presets that promote under both letters, or discard
	// under both, would not need the split — the one that does is the whole of
	// why this is two questions, and one field would have had to give it an
	// answer that is wrong for one of its letters whichever way it was set.
	ScalarUnderATableDeclaration ScalarUnderACompoundPolicy

	// ValuelessDeclarationHidesTheOuterValue makes `local u` in a function hide
	// any outer `u` — the local exists unset, so `${u-UNSET}` fires the default
	// even when the caller had a value.
	//
	// Reached only when DeclaredNameWithoutValueIsEmpty said no: a name declared
	// *empty* hides the outer value by having one of its own.
	//
	// Answering No leaves the caller's value showing through until the first
	// assignment. This is the shape used to declare a local before assigning it
	// conditionally, so the difference is silent: the function reads the
	// caller's value where it expected nothing.
	ValuelessDeclarationHidesTheOuterValue Answer

	// DeclarationAssignmentClearsTheExportAttribute takes the export attribute
	// off a name a declaration utility assigns to:
	//
	//	export FOO=bar; typeset FOO=baz; env | grep '^FOO='
	//
	// Yes tells the child nothing and goes on telling it nothing: the name keeps
	// its value and is simply no longer exported, which `export -p` and
	// `typeset -p` both confirm. `export FOO` afterwards puts the attribute
	// back, so it is a reset rather than a refusal. An implementation with no
	// declaration utility cannot be asked.
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
	// FOO=baz` clears it, where `readonly` is the declaration builtin with the
	// readonly letter; `export FOO=baz` does not, because it names the
	// attribute. A plain `FOO=baz` does not either, under any answer — this is a
	// declaration utility's doing and not an assignment's.
	//
	// The preset is no: POSIX has an exported name keep the attribute for the
	// life of the shell.
	DeclarationAssignmentClearsTheExportAttribute Answer

	// LocalInheritsTheExportAttribute gives a local declaration the export
	// attribute of the name it shadows, so a child sees the local's value under
	// the shadowed name. Answering No hands the child nothing at all under that
	// name for as long as the function runs.
	//
	// Asked only where the shadowed name is exported — explicitly or by having
	// been inherited — and only where a scope was actually taken. Declaring a
	// name nothing has exported asks nothing, and a local declared `-x` says so
	// outright and asks nothing either.
	//
	// The value is not the question: under Yes the local's own value is what a
	// child is told, and whether a valueless declaration still shows the outer
	// value is ValuelessDeclarationHidesTheOuterValue rather than this.
	//
	// An implementation with no `local` reaches the question only through its
	// declaration keyword in a keyword-defined function. Where that keyword
	// takes the attribute off any name it assigns — at the top level as well as
	// in a function — the child is told nothing by that route instead, which is
	// the same answer arrived at another way. Only the local half is modeled.
	LocalInheritsTheExportAttribute Answer
	// TypesetLocalNeedsKeywordFunction restricts `typeset`'s local scope to
	// functions defined with the `function` word. Under Yes, `f() { typeset x=1;
	// }` reaches the caller's `x` and `function f { typeset x=1; }` does not;
	// under No the two definition forms are interchangeable. An implementation
	// with no `typeset` at all leaves the axis absent rather than false.
	//
	// It asks about `typeset` and not about `local` because `local` is
	TypesetLocalNeedsKeywordFunction Answer

	// DeclareListing is the shape of what `declare -p` and `typeset -p`
	// write back. Three engines rather than two answers — see
	// DeclarationListingForm.
	DeclareListing DeclarationListingForm

	// DeclareValueQuoting is how a listed declaration spells its value. A field
	// of its own over the shared vocabulary because it does not follow a
	// preset's other listings: an engine that single-quotes its aliases and
	// traps may double-quote its declarations.
	DeclareValueQuoting ListingQuotingStyle

	// ExportListing is the shape `export -p` writes: each name as a clustered
	// declaration (`declare -x V="1"`), or the command word repeated
	// (`export V='1'`).
	ExportListing DeclarationListingForm
	// ReadonlyListing is the same question from `readonly -p`, where a preset
	// may part ways with its own export listing and write `typeset -r R=2`.
	ReadonlyListing DeclarationListingForm

	// CoprocEndsInAnArray publishes a started coprocess's near ends as the two
	// elements of an array — `${COPROC[0]}` to read and `${COPROC[1]}` to write,
	// with the process in `COPROC_PID` — which is the model whose `coproc` takes
	// a name.
	//
	// Answering No has no name for a coprocess and no array, and a script
	// reaches the ends with `print -p` and `read -p` instead. Asked only when a
	// coprocess is started, so a preset without the word never meets it.
	CoprocEndsInAnArray Answer

	// BareDeclarationListing is the shape `export` and `readonly` write with no
	// operands and no `-p` — which is not always the shape `-p` writes. A preset
	// may answer the bare form exactly as it answers `-p`, or drop the command
	// word for the bare form alone and write a plain `V='a b'`, which no `-p`
	// anywhere writes because it could not be read back as a declaration.
	//
	// One field for both builtins, because nothing splits them: where the bare
	// form differs from `-p` it differs for both, and by the same rule.
	//
	// It is the *filtered* listing's row as well — `declare -x` and `typeset -a`
	// with no names — which is measured and not assumed: a preset writes the
	// same row for both, whichever row that is. So the filter chooses the names
	// and this chooses the row, and neither builtin needs a form of its own.
	BareDeclarationListing DeclarationListingForm

	// DeclarationListingFilter is how a `declare` or `typeset` with attribute
	// letters and no names combines them when more than one is written —
	// `declare -ir`, `typeset -ax`. See DeclarationFilterForm for the readings.
	//
	// Asked only where two letters were written: every reading agrees on one
	// letter, so a preset that has not answered still lists `declare -x`.
	DeclarationListingFilter DeclarationFilterForm

	// DeclarePrintReportsAMissingName makes `typeset -p nosuch` say so and fail
	// — with its own wording, see Diagnostics.DeclareNoSuchVariable — and answer
	// 1 even when other names listed fine. Answering No prints nothing for the
	// missing name and answers 0.
	DeclarePrintReportsAMissingName Answer

	// DeclareOptions is the set of letters `declare` and `typeset` take, spelled
	// the way ReadOptions is. The letters belong to the preset, and so do their
	// meanings: `-g` declares a global where the letter exists and is an unknown
	// option elsewhere — fatally, where bad `typeset` options are fatal — and
	// `-F` names functions under one preset while it sets a float's precision
	// under another.
	//
	// Empty means `aAiprx`, the set the substrate implemented before the letters
	// were a question.
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
	// The letter this was written for is one whose meaning under some preset is
	// a *float's* precision rather than a function listing, and silence was the
	// smaller lie only for as long as there was no float attribute to record the
	// precision in. There is one now — see DeclareOptionsTakingANumber — so that
	// letter graduated out of here, and the field stands for the next letter of
	// that shape rather than being deleted with its one user.
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
	// The detached spelling is not universal: an implementation may have the
	// same letters and spell their number *attached only* — `-F[n]` in its own
	// usage — so `typeset -Fx 3 a=1.5` is a bad identifier there where the
	// detached reading takes the precision. The field has no per-spelling half,
	// because only the detached arrangement is modeled.
	//
	// `-i` is not named here and is not an omission: whether it takes a base is
	// IntegerAttributeTakesABase, which the `integer` builtin asks too and which
	// also decides whether the base is *recorded*. Two fields both claiming `-i`
	// takes a number could disagree, so the parse asks one helper —
	// declareOptionTakesANumber — and that helper reads the axis for `i` and
	// this list for every other letter.
	//
	// Only letters this engine both spells and acts on belong here. A letter
	// that takes a number in the real shell but is refused by name before it is
	// ever read would have an entry nothing consults — see
	// Diagnostics.UnimplementedOptionLetters.
	DeclareOptionsTakingANumber string

	// TypesetBadOptionFatal ends the script over an option `typeset` does not
	// have, which is what counting `typeset` among the special builtins gets.
	// Answering No reports it and carries on.
	//
	// Asked only when the refusal has happened, so a preset with no `typeset`
	// never meets it.
	TypesetBadOptionFatal Answer

	// SignAloneIsAnOptionWord reads a declaration's `-` or `+` written with no
	// letters after it as an option word rather than as an operand.
	//
	// A real disagreement and not a missing feature, which is why it is a field:
	// under Yes, `typeset +` writes a listing — the whole parameter table's
	// attribute words and names, or a name listing — and under No the sign is a
	// *name*, and not one a script may declare, so the line is `not a valid
	// identifier` at 1. Both readings are complete and neither is a superset of
	// the other, so the sign cannot simply be swallowed.
	//
	// The sign means what it means everywhere else on the builtin once it is
	// read: a minus adds nothing to the bare listing and a plus turns it into
	// the names-only shape. `functions +` is the same word reaching the function
	// table, and is how a shell snapshot asks for a name list.
	//
	// Asked only where a script really wrote one, so a preset that never meets
	// the shape is never asked for an answer.
	SignAloneIsAnOptionWord Answer

	// SignAloneIsAnOptionWordToExport is the same reading asked of `export` and
	// `readonly`, which have an option parse of their own.
	//
	// A second field rather than the one above because the two questions can
	// have different answers in the same implementation: one may list for
	// `typeset +` and refuse `export +` and `readonly +` as names. A preset
	// reading one field for both would have to be wrong about one of them.
	//
	// The listing it reaches is the *builtin's own* attribute, names only:
	// `export +` writes the exported names and `readonly +` the frozen ones,
	// which is `typeset +x` and `typeset +r` under a second word. The minus
	// spelling needs no field — a lone `-` is already LoneDashIsAnOption's
	// question, and once it is eaten `export -` is the bare listing.
	//
	// One field for the two builtins because nothing separates them: `export +`
	// and `readonly +` are answered the same way. Asked only where a script
	// really wrote the sign.
	SignAloneIsAnOptionWordToExport Answer

	// FunctionNamesUnderPlus reads the *plus* spelling of the function letter as
	// a request for the names alone — `typeset +f` against `typeset -f`.
	//
	// The implementations that spell the letter disagree about what the sign
	// means, which is why this is a field rather than an assumption:
	//
	//   - Yes writes one bare name a line, with an operand and without one
	//     alike, and the last `f` letter's sign decides — `typeset -f +f f` is
	//     the name and `typeset +f -f f` is the body.
	//   - No has no names-only spelling under this sign at all: `+f` takes the
	//     function attribute *off*, leaving the bare `declare`, which writes
	//     every variable and then every function. That listing is
	//     BareDeclarationListing's row, so No here leaves the letter writing
	//     bodies — as wrong as it was rather than wrong in a new way.
	//
	// A third reading exists and is not asked for: a name listing that renders
	// the definition form as well. That belongs to an implementation which
	// refuses the `f` letter outright, and whose body listing is a verbatim copy
	// of the source text this engine does not keep.
	//
	// Asked only where a plus-signed `f` was really written.
	FunctionNamesUnderPlus Answer

	// FunctionLettersThatMarkUndefined names the letters that turn a `-f`
	// declaration with operands from a *listing* into a marking: the name
	// becomes a function at once whose body is read the first time it is called.
	//
	// An implementation with the notion spells it on `typeset` as well as under
	// its own word, and they are not spelled alike, which is why this is a
	// letter set rather than a fixed reading. One may take `u` and `U`, where a
	// further letter rides along and is recorded without being one of these —
	// a letter that only *decorates* the marking cannot start one. Another may
	// take `u` alone, listing the result straight back as its whole rendering of
	// an undefined function.
	//
	// Empty where there is no such notion and every `-f` line is a listing. What
	// the letters *do* is [Runner.SetFunctionMarkedUndefined], which is where a
	// preset's own vocabulary lives; this field only says which lines are not
	// listings.
	FunctionLettersThatMarkUndefined string

	// IntegerOptions is the set of letters the `integer` builtin takes, spelled
	// the way DeclareOptions is.
	//
	// A separate field rather than DeclareOptions over again because an
	// implementation with the word gives it a *narrower* set than its own
	// `typeset`, and they narrow it differently — one dropping the array and
	// function letters, another the float and justification ones. A preset whose
	// `integer` simply reused the declaration's letters would accept
	// `integer -A m`, which is an associative array under neither.
	//
	// Empty means the builtin is not registered at all, which is what a preset
	// where the word is simply a command that was not found holds.
	IntegerOptions string

	// FunctionsOptions is the set of letters the `functions` builtin takes,
	// spelled the way DeclareOptions is.
	//
	// A separate field rather than DeclareOptions over again, because the name
	// that means `typeset -f` does not take `typeset`'s letters: it may refuse
	// letters its own `typeset` spells and take a dozen its `typeset` does not.
	// A preset that reused the declaration's set would accept an array attribute
	// here and refuse a listing letter that really exists.
	//
	// Only the letters this engine both spells and acts on belong here; the rest
	// are Diagnostics.UnimplementedOptionLetters, so a script meets "not
	// implemented yet" for a letter the real shell has and "bad option" for one
	// it does not. Empty means the builtin takes no letters at all, which is a
	// real answer — whether the word exists is Register's, not this field's.
	FunctionsOptions string

	// UnfunctionOptions is the same for `unfunction`, whose set is one letter:
	// `-m`, with every other letter of the alphabet refused in both cases as a
	// bad option — including the `-f` that is the option this name stands for.
	UnfunctionOptions string

	// IntegerAttributeTakesABase is `-i` reading an output base — `typeset -i 16
	// n=255` and its attached spelling `-i16` — so that the name prints in that
	// base afterwards rather than in decimal. Answering No makes `-i16` an
	// invalid option and a bare `16` an invalid identifier.
	//
	// The base is a property of the *name* and not of the assignment that met
	// it, so it is recorded like the attribute itself and consulted by every
	// store afterwards: `typeset -i8 c; c=64` is `8#100`, and a second
	// declaration with a different base re-renders what the name already holds.
	//
	// What is stored is the *rendered text*, which is what every read sees:
	// `${#h}` is 5 for `16#ff`, `g=$h` copies those five characters, a child is
	// told `h=16#ff`, and arithmetic parses it back — `$(( h + 1 ))` is 256. So
	// it is a change to the value and not a way of printing it, and modeling it
	// as a rendering would answer every one of those rows wrong.
	//
	// Base 10 and any base outside what the preset can spell render plain.
	// Which bases it *can* spell is IntegerBaseDigits, whose length is the
	// largest, and whether it complains about the rest is
	// Diagnostics.IntegerBadBase.
	//
	// Asked only when a base is actually written, so the ordinary `-i` never
	// reaches it and a preset with no `typeset` never meets the question at all.
	IntegerAttributeTakesABase Answer
	// IntegerBaseDigits is the alphabet a preset renders an output base in, and
	// its length is the largest base it can spell.
	//
	// Two facts in one string, because they are one fact about the
	// implementation: counting in lower case and carrying on into upper gives an
	// alphabet 62 long — `16#ff`, `36#2s`, `64#1A` for 100 — where counting in
	// upper case stops at 36, `16#FF` and `36#2S`. An empty alphabet renders
	// every base plain, which is what a preset without the feature does.
	IntegerBaseDigits string
	// IntegerBaseComesFromTheValueAssigned learns a name's output base from the
	// radix prefix of the text assigned to it, where no base was named:
	//
	//	typeset -i b; b=0x10; echo "$b"     Yes `16#10`   No `16`
	//	typeset -i d; d=8#7;  d=99          Yes `8#143`   No `99`
	//	a=0x10; typeset -i a                Yes `16#10`   No `16`
	//
	// The base is remembered on the name under either answer, and what differs
	// is only where the default comes from. It sticks — a later plain `5` under
	// a name that learned 16 is `16#5` — which is what makes it the name's and
	// not the assignment's.
	//
	// Only a *radix* prefix teaches it. A leading zero does not (`016` is `16`
	// under both), and neither does a value that arrived already evaluated:
	// `f=$((0x10))` is `16` under both, the expansion having handed the
	// assignment the four decimal characters.
	IntegerBaseComesFromTheValueAssigned Answer
	// IntegerBaseNegativeIsTwosComplement renders a negative integer in its
	// output base as the bit pattern rather than as a sign and a magnitude:
	//
	//	typeset -i16 h; h=-255    Yes `16#ffffffffffffff01`   No `-16#FF`
	//	typeset -i2 c=-5          Yes sixty-four binary digits  No `-2#101`
	//
	// Asked only for a negative value in a base that renders at all, so nothing
	// else meets it.
	IntegerBaseNegativeIsTwosComplement Answer
	// IntegerBaseTenIsNoBase makes ten the *default* of the integer letter
	// rather than a base like any other, so that naming it records nothing and
	// writing the letter with no base at all takes off the base a name already
	// has:
	//
	//	typeset -i10 d=255; typeset -p d      Yes `typeset -i d=255`
	//	                                      No  `typeset -i10 d=255`
	//	typeset -i16 a=255; typeset -i a      Yes `255`   No `16#FF`
	//	typeset -i16 b=255; integer b         Yes `255`   No `16#FF`
	//	typeset -i16 c=255; typeset -x c      `16#ff` either way
	//
	// The rows are one answer. Under Yes the letter always names a base and ten
	// is what it names when nothing is written, so a bare `-i` is `-i10` and ten
	// is the absence of one. Under No ten is a state: it is recorded, its
	// listing says `-i10` back, and a later bare `-i` leaves it alone. Whether
	// "no base" is a state or just base ten is exactly what the two answer
	// differently.
	//
	// The value reads the same either way — `typeset -i10 e=255` is `255` under
	// both, ten being the base nothing is written in — so this is not
	// IntegerBaseDigits asked twice. What it changes is the *listing* and what a
	// second declaration does to a base already there.
	//
	// Asked only where the answer changes something: where ten is written, and
	// where the letter arrives bare over a name that has a base. An ordinary
	// `typeset -i n` on a name with no base never meets it, which is nearly
	// every declaration there is.
	//
	// The other letter is the control and needs no answer: `typeset -x` over a
	// based name leaves the base alone under both.
	IntegerBaseTenIsNoBase Answer

	// IntegerPlusFormTakesAttributesOff decides whether a plus word on `integer`
	// removes anything at all.
	//
	// The implementations that have the word disagree about what the word *is*,
	// and the disagreement is not confined to `+i`:
	//
	//	integer n=5; integer +i n; n=3+4    Yes `3+4`   No `7`
	//	integer -x e=1; integer +x e        Yes gone    No still exported
	//
	// Yes prepends the letter to an ordinary declaration, so every plus form
	// means what it means on `typeset`. No has a declaration command of its own
	// whose type is fixed, and a plus form on it removes nothing — its listing
	// keeps the letters where Yes lists a plain name, which is the same finding
	// read from the value side. That its own `typeset +x` does unexport is what
	// says this is about the second name and not about the letter.
	//
	// It is not spelled per letter: one reading of the word covers `+i` and `+x`
	// alike, and a field per letter would have been two questions whose answers
	// can only ever agree.
	//
	// Asked only for a plus word on `integer`, which is the only place the two
	// readings differ — every other spelling is parsed identically.
	IntegerPlusFormTakesAttributesOff Answer

	// SetArrayLetter is `set -A name value …`, which assigns an array through
	// a name a variable holds — the thing `name=(…)` cannot do, because the
	// name is a literal there.
	//
	// Which makes it a preset's answer rather than an axis, and it is **read
	// rather than asked**: where the answer is not yes the letter is somebody
	// else's invalid option, and the refusal already in place is that preset's
	// own real words. Asking an axis there would replace a correct answer with a
	// complaint about a missing preset.
	SetArrayLetter Answer

	// SetArrayOptionsContinuePastTheName decides whether the words behind
	// `set -A name` are more options or the array's values:
	//
	//	set -A ff -x -y     Yes `-y: unknown option`   No `[-x -y]`
	//	set -A dd -- 1 2    Yes `[1 2]`                No `[-- 1 2]`
	//
	// Yes keeps parsing, so the values are whatever the option parse does not
	// claim — exactly the words that would have become the positional parameters
	// — and a `--` among them still ends the options. No stops at the name and
	// every word behind it is a value, dash words and `--` included.
	//
	// **One question, not two.** Both rows move together, because whether `--`
	// is an operand *is* whether options are still being read. Asked only once
	// the letter is taken, so a preset without it never meets the question.
	SetArrayOptionsContinuePastTheName Answer

	// SetArrayWithNoValuesUnsetsTheName is `set -A name` with nothing after the
	// name: Yes unsets it and No leaves an array with no elements.
	//
	//	set -A a 1 2 3; set -A a; typeset -p a
	//	  Yes  nothing at all — the name is gone
	//	  No   typeset -a a=(  )
	//
	// Both answer `${#a[@]}` as 0, so the difference shows only through `${a+x}`
	// and a listing — which is exactly what makes it worth a field: a script
	// that tests whether the name is set gets opposite answers.
	//
	// The *plus* form is not this question and needs no field: `set +A a` with
	// no values leaves the array exactly as it was under both, which is
	// unanimous and is a different operation.
	SetArrayWithNoValuesUnsetsTheName Answer

	// JobSpecsByName resolves `%name` — the job whose command begins with the
	// text — and `%?text`, the one whose command contains it. POSIX gives both
	// spellings; answering No says "no such job" to every spec that is not a
	// number, `%%`, `%+` or `%-`.
	JobSpecsByName Answer
	// AmbiguousJobNameIsRefused is `%name` matching more than one job: Yes
	// refuses it as an ambiguous job spec, No takes the most recent match. Asked
	// only on a second match.
	AmbiguousJobNameIsRefused Answer
	// WaitReportsAMissingJob says a job spec `wait` cannot resolve earns a
	// complaint — see Diagnostics.WaitNoSuchJob — and a failing status.
	// Answering No says nothing at all and reports 0.
	WaitReportsAMissingJob Answer
	// WaitNWaitsForTheNextJob gives `wait` a `-n`: block until whichever job
	// finishes first and report its status, 127 with no jobs at all. Answering
	// No refuses or misreads the letter.
	WaitNWaitsForTheNextJob Answer
	// WaitForAJobFailsWhenInterrupted has a `wait` that names a job report a
	// plain 1 when a trapped signal cuts it short, rather than the status that
	// signal encodes.
	//
	// Only with an operand: under Yes, `wait $!` and `wait %1` both report 1
	// where the *bare* `wait` reports that preset's own encoding for a command a
	// signal killed. Answering No makes no distinction between the two forms.
	//
	// The preset follows the answer the majority agree on. Measured with a
	// background job outliving the signal, so the answer is about the
	// interruption and not about the job's own status.
	WaitForAJobFailsWhenInterrupted Answer
	// DisownRemovesTheJob makes `disown` take the job out of the table, so a
	// later `jobs` no longer lists it.
	//
	// Answering No gives `disown` a narrower meaning — shielding the job from
	// the HUP an exiting shell would send, a signal this engine never forwards —
	// and goes on listing the job.
	DisownRemovesTheJob Answer

	// JobsOptions is the set of letters `jobs` takes, spelled the way
	// ReadOptions is. The letters belong to the preset and the sets are not
	// nested: some add state filters, some a listing letter, some several of
	// their own. Empty means `lp`, which is POSIX's pair and the only one
	// everything has.
	//
	// It is a semantics field rather than a constant because a letter one
	// implementation has and another has never heard of is a *refusal* in the
	// second one: `jobs -r` lists the running jobs under one preset and is an
	// illegal option under another, and a shared letter set would have this
	// engine accept it everywhere and answer those scripts differently from the
	// shell they were written for.
	JobsOptions string
	// JobsPidsOnlyOption makes `jobs -p` print one process id per line and
	// nothing else — no number, no marker, no state, no command.
	//
	// Answering No reads the same letter as "put the job's process *group* id in
	// the listing" and prints the ordinary rows, so a `kill $(jobs -p)` written
	// against Yes kills nothing there.
	//
	// Asked only where the letter was given, and only in a preset that has it,
	// so a listing with no `-p` never reaches it.
	JobsPidsOnlyOption Answer
	// JobsStateFiltersAccumulate decides `jobs -r -s`, where both of the state
	// filters are named at once: Yes lists a job matching *either* state, No
	// lets the last letter given decide and lists only the jobs in that state,
	// so `jobs -rs` is `jobs -s`.
	//
	// Asked only when both letters arrive together. One of them alone means the
	// same thing under both answers, and a preset without the letters cannot
	// reach the question at all.
	JobsStateFiltersAccumulate Answer

	// DeclareGlobalReachesPastALocal is `declare -g x=new` with a `local x`
	// standing in front of the name: Yes writes the global cell and leaves the
	// local untouched, No assigns the visible cell — the local — and leaves the
	// global alone.
	//
	// Asked only there: with no local in front, both write the global, which is
	// what the letter is for.
	DeclareGlobalReachesPastALocal Answer

	// LocalOptions is the same question asked of `local`, whose answers do not
	// follow `typeset`'s: an implementation may have `local` and give it no
	// options at all, so `local -r x` declares a variable named `-r` there — and
	// then refuses it as a bad name. Empty means none.
	LocalOptions string

	// BareLocalListing is what `local` with no operands writes — several shapes
	// among the presets that can reach it, so it is a form rather than a flag.
	// See BareLocalListingForm.
	BareLocalListing BareLocalListingForm

	// BareTypesetListing is what `typeset` or `declare` with no operands and no
	// letters writes.
	//
	// An axis of its own even though it shares the form type with
	// BareLocalListing, because an implementation with both words need not
	// answer the two the same: one writes the identical parameter table either
	// way, another's bare `declare` is every variable the shell holds rather
	// than the running function's locals. Only some of those answers are values
	// this form already carries; the rest stay unanswered and are refused by
	// name rather than guessed at.
	BareTypesetListing BareLocalListingForm

	// SetListing is what `set` with no arguments writes — see SetListingForm.
	// Every preset lists, but not the same things: one follows the variables
	// with every defined function, and one lists special parameters and tied
	// arrays no other has.
	SetListing SetListingForm

	// ListingControlEscape is how a `$'...'` listing spells a control byte — see
	// ControlEscapeStyle. A field of its own rather than a part of the quoting
	// style, because two presets that quote the same way may spell a control
	// byte differently.
	ListingControlEscape ControlEscapeStyle

	// SetListingQuoting is how that listing spells a value, over the shared
	// listing vocabulary: quoting only where it must and closing-reopening with
	// a backslash, single-quoting everything, or reaching for `$'...'`.
	SetListingQuoting ListingQuotingStyle

	// SelectLayout is how `select` draws its menu. Three engines rather than
	// two answers, which is why it has its own type.
	SelectLayout SelectMenuLayout

	// AliasParsesOptions lets `alias` read leading `-` words as options.
	// Answering No reads none, so `alias -p` is a name and the answer is
	// "-p not found" rather than a refusal.
	AliasParsesOptions Answer

	// AliasHasPrintOption gives `alias` a `-p`, which prints the listing with
	// `alias ` in front of every line.
	//
	// It is only visible where the plain listing does not already look like
	// that. A preset that parses no options for `alias` at all makes `-p` a
	// *name* and answers "not found"; one that has options may refuse it.
	AliasHasPrintOption Answer

	// GlobalAliases gives this preset the second kind of alias: `alias -g
	// name=value` defines one, and a word naming one is expanded *wherever it
	// stands* rather than only where a command word does — in an argument, a
	// `for` list, a `case` pattern, a redirection target, a heredoc delimiter, a
	// `[[ ]]` word. The value is spliced as tokens like any other alias body, so
	// `alias -g UP="| tr a-z A-Z"` puts a pipeline in the middle of a line.
	//
	// It shares a table with the regular kind — `alias -g` over a regular name
	// replaces it — and the plain listing shows both, which is why the two are
	// one field rather than one table each.
	//
	// The letter is the visible half; the expansion is the feature. A preset
	// answering Yes and expanding nothing would list an alias it never uses.
	//
	// `make axis-sweep` cannot pin this where `alias` reads no options at all —
	// AliasParsesOptions is No there, so the accepted set is never consulted,
	// and nothing has an `unalias -g` for it to be consulted from either. The
	// answer has no reachable consequence in such a preset, which is the third
	// of the four triages docs/spec/semantics.md lists. SuffixAliases is pinned
	// everywhere, because `unalias -s` reads the axis whatever `alias` does with
	// its operands.
	GlobalAliases Answer

	// SuffixAliases gives this preset the third kind, which is a second
	// *namespace*: `alias -s ext=value` keys on a command word's extension, and
	// a command word `text.ext` — text non-empty, ext the run after the last dot
	// — is replaced by the text `value text.ext`. So `alias -s txt=cat` makes
	// `./x.txt` into `cat ./x.txt`.
	//
	// It is a parse-time substitution rather than a fallback for a command that
	// was not found: it beats an executable of that name on PATH and a function
	// of that name, and loses to a regular alias of that name, which is exactly
	// the order a substitution done while reading the line produces. A value
	// holding a pipeline splices one in.
	//
	// The namespace is the half a single flag could not say: the two sets are
	// never listed together, `unalias -a` empties the other table and leaves
	// this one, and `unalias -s` is the only way to remove one — which is also
	// why this field is read by `unalias` as well.
	SuffixAliases Answer

	// AliasListsAsDefinitions gives `alias` a `-L`, which writes every line as a
	// command that would define the alias back: `alias ` in front, and the
	// kind's own letter where the entry is not the regular kind — `alias -g
	// UP='| tr a-z A-Z'`, `alias -s txt=cat`.
	//
	// It is what a startup file wants and what a plain `name=value` listing
	// cannot be, since that says nothing about which kind an entry is. The
	// letter is a listing form and not a filter: `alias -L`, `alias -g -L` and
	// `alias -s -L` each list what the kind letter alone would have listed.
	AliasListsAsDefinitions Answer

	// AliasRestrictsToRegularKind gives `alias` a `-r`, the kind letter for
	// "neither global nor suffix".
	//
	// It exists because the other two kind letters leave no way to ask for the
	// plain ones: the shared table holds the regular and the global aliases
	// together and the plain listing shows both. It is a kind like `-g` and `-s`
	// rather than a modifier on them, so `alias -r -g` and `alias -rs` are
	// `illegal combination of options` exactly as `alias -gs` is.
	AliasRestrictsToRegularKind Answer

	// AliasOperandsCanBePatterns gives `alias` and `unalias` a `-m`, which reads
	// every operand as a pattern rather than as a name.
	//
	// Read by both builtins because one letter serves both, and they differ in
	// what an absent operand means: `alias -m` with nothing after it is the
	// plain listing at 0, and `unalias -m` with nothing after it is `not enough
	// arguments` at 1 — a removal with no pattern would be a removal of
	// everything, which is what `-a` is for.
	//
	// A pattern that matches nothing is 0 for `alias` and 1 for `unalias`, which
	// is the same shape as a name that is not there.
	AliasOperandsCanBePatterns Answer

	// AliasPlusPrintsNamesOnly makes `+g`, `+r`, `+s` and a bare `+` list the
	// names without the values.
	//
	// The plus words are not option letters in the ordinary sense: a bare `+`
	// also *ends* the option list, so `alias + -L` looks up an alias called `-L`
	// where `alias -L +` lists everything in the `-L` form. `-L` wins over the
	// names-only reading when both are written: `alias -L +g` is the full
	// definition line.
	//
	// `unalias` has none of them — `unalias +m x` looks for hash table elements
	// called `+m` and `x` — so this is read by `alias` alone.
	AliasPlusPrintsNamesOnly Answer

	// TypeNamesAnAliasOnlyWhenExpanded holds `type`, `command -v` and
	// `command -V` silent about an alias while alias expansion is off.
	//
	// A real difference rather than a detail of how a program arrived: under Yes
	// `alias a='echo hi'; type a` in a `-c` string is `type: a: not found` while
	// `alias` lists the entry one line earlier, and turning expansion on in
	// front of it makes the same call answer. Under No the table answers
	// whatever the switch says.
	//
	// So the three builtins report what *would run* under Yes, where an alias
	// that cannot expand would not run, and report what the table holds under
	// No, where the two are never apart.
	TypeNamesAnAliasOnlyWhenExpanded Answer

	// AliasReportsNotFound says something when `alias` is given a name the
	// table does not hold. Answering No reports 1 and prints nothing.
	AliasReportsNotFound Answer

	// UnaliasReportsNotFound is that question for `unalias`, and the panel
	// does not pair the two: ksh93 complains about `alias nope` and is silent
	// about `unalias nope`, and zsh does exactly the reverse. One field could
	// not say that.
	UnaliasReportsNotFound Answer

	// AliasNotFoundStatusCounts makes `alias` report how many names it could not
	// find rather than a plain 1, so `alias n1 n2 n3` is 3.
	//
	// About `alias` alone — the same preset's `unalias` answers 1 however many
	// were missing — so it is asked where the count is known and not where the
	// complaint is printed.
	AliasNotFoundStatusCounts Answer

	// UnaliasAllRefusesOperands makes `unalias -a name` an error that clears
	// nothing: "-a: too many arguments", status 1, table intact. Answering No
	// takes the `-a`, ignores the names and empties the table.
	UnaliasAllRefusesOperands Answer

	// AliasQuoting is how a value is spelled in a listing — several engines, no
	// two alike. See ListingQuotingStyle.
	AliasQuoting ListingQuotingStyle

	// TrapQuoting is that same question asked of `trap`, and it is a separate
	// field because a preset may answer the two differently — writing an alias
	// holding a tab as `$'a\tb'` and a trap holding one as a plainly quoted
	// `'a<tab>b'`.
	TrapQuoting ListingQuotingStyle

	// TrapActionIsParsedWhenSet reads a trap's action when the trap is set
	// rather than when it fires, and refuses a trap whose action will not parse.
	//
	// Answering No stores the text: `trap "if" EXIT` is taken and complains at
	// the end, and `trap "if" INT` is taken and never complains at all, because
	// the trap never fires.
	TrapActionIsParsedWhenSet Answer

	// TrapBodyRunsWhatParsed runs each line of a trap's body as it parses, so
	// the part before a syntax error has already run by the time the error is
	// reported: `trap "echo a\nif" EXIT` prints `a` and then complains.
	// Answering No reads the whole body first and prints nothing.
	//
	// Where TrapActionIsParsedWhenSet is Yes this answers No by construction
	// rather than by measurement: the whole body has parsed before the trap can
	// fire, so there is no partial run to have and the two answers cannot be
	// told apart.
	TrapBodyRunsWhatParsed Answer

	// TrapParseFailureNamesWhereItFired puts the runtime location in front of a
	// trap body's parse failure — where the trap fired — rather than the line
	// the parse gave out on.
	//
	// The two are different numbers: a body set on line 2 and fired from line 5
	// reports `w5.sh: line 5: syntax error at line 6` under Yes, and names the
	// parse position in both places under No.
	//
	// Not asked where the action is read when the trap is set, since a parse
	// failure is never reached at fire time there.
	TrapParseFailureNamesWhereItFired Answer

	// SymbolicMaskTakesMoreThanOneOperator lets one `umask` clause turn on
	// several: `umask u+rw-x` is 0122 from 022. Answering No takes a single
	// operator per clause and names the second one.
	SymbolicMaskTakesMoreThanOneOperator Answer

	// SymbolicMaskWhoAloneSetsIt reads `umask g` as `umask g=`, denying that
	// group everything. Answering No refuses it, or answers it with the
	// complaint a number that could not be read draws.
	SymbolicMaskWhoAloneSetsIt Answer

	// SymbolicMaskTakesTheSetuidLetter accepts `s` in a clause, which changes no
	// bits — a umask has no setuid bit to deny — and is accepted all the same.
	SymbolicMaskTakesTheSetuidLetter Answer

	// SymbolicMaskTakesTheStickyLetter is the same question about `t`, and a
	// different set of answers. Two fields because the two letters are not
	// answered together.
	SymbolicMaskTakesTheStickyLetter Answer

	// ShiftOptionWords is which leading-`-` words `shift` reads as options
	// rather than as its count, and it is three answers rather than a presence —
	// see ShiftOptionWordPolicy.
	//
	//	shift -x   the count, and not a number
	//	           an option, and not one this preset has
	//	shift -1   the count
	//	           an option, and not one this preset has
	//
	// The third reading is what makes it three: refusing `-x` as an option while
	// reading `-1` as a count that is out of range, so "reads options" and
	// "reads every dash word as an option" are not the same answer.
	//
	// A lone `-` is not a dash word in any reading here and reaches the count.
	// Other readings of that one word exist and are not modeled — see
	// docs/spec/semantics.md. Nor is `--` a dash word, which is asked about
	// separately — see ShiftDoubleDashEndsOptions.
	//
	// Asked only for a word that actually begins with a `-`.
	ShiftOptionWords ShiftOptionWordPolicy
	// ShiftDoubleDashEndsOptions takes `--` as the end-of-options marker and
	// reads what follows as the count. Answering No calls `--` an illegal
	// number, having no option parsing here for a marker to end.
	//
	// It is not ShiftOptionWords: a preset may read no dash word as an option
	// and still honor the marker, so the two questions have different answers in
	// the same implementation. Only the *first* `--` is the marker —
	// `shift -- --` complains about the second wherever the marker is taken.
	//
	// Asked only where the operand actually is `--`.
	ShiftDoubleDashEndsOptions Answer
	// ShiftNamesAreArrays reads `shift`'s operands as the names of arrays to
	// shift, instead of the positional parameters — the synopsis
	// `shift [ n ] [ name ... ]`. Answering No calls a name a `numeric argument
	// required` and a second operand `too many arguments`, or evaluates the word
	// arithmetically.
	//
	// The count stays optional in front of them, so the first word is ambiguous
	// — and Yes settles it by *type*:
	//
	//	q=5;   a=(1 2 3 4 5 6); shift q a   # a=(6)          — q is a count
	//	q=(5); a=(1 2 3 4 5 6); shift q a   # q=() a=(2 … 6) — q is a name
	//
	// So a word that names an array is a name and every other word is an
	// arithmetic count, which is why `shift a a` shifts `a` twice rather than
	// once by its first element: arithmetic on an array name is a bad math
	// expression, so an array can only ever have been a name.
	//
	// A name that is not an array — unset, a scalar, an association — is left
	// alone and not complained about, at status 0. What protects the positional
	// parameters is giving *any* operand, not the operand turning out to name an
	// array: `set -- x y z; shift 1 nosuch` leaves `$@` where it is. A first
	// word that is a scalar is the count, though, and shifts them like a literal
	// one. A count past the end of one array is reported
	// (Diagnostics.ShiftTooMany) and the remaining names are still shifted, so
	// `shift 2 a b` with a one-element `a` is status 1 with `b` shifted. That
	// carrying-on is why this may only be answered where ShiftPastEndFatal is
	// No.
	ShiftNamesAreArrays Answer
	// ShiftNegativeIsOutOfRange reads a negative count as a number that is out
	// of range rather than as a word that is not a number, at status 1 and in
	// each preset's own wording (Diagnostics.ShiftNegativeCount). Answering No
	// calls `-1` an illegal number, which is the same complaint it makes about
	// `-x`.
	//
	// It is the other end of ShiftTooMany — one count, out of range in two
	// directions — so the same ShiftPastEndFatal decides whether it ends the
	// script, with `$#` untouched either way.
	//
	// Asked only where the count really is negative, which where a bare `-1` is
	// an option means only after a `--`.
	ShiftNegativeIsOutOfRange Answer

	// WaitReadsOptions reads a leading `-` word as an option rather than as a
	// job to wait for. Answering No has none, and answers `wait -x` with the job
	// it could not find.
	WaitReadsOptions Answer

	// CommandRejectsUnknownOption refuses a leading `-` word that is not one of
	// `command`'s own options, rather than taking it as the command.
	//
	// `-v` and `-p` are read everywhere. Answering No stops reading options
	// there, so `command -q ls` is `command not found: -q` rather than a refused
	// option. The same shape `printf` already has. Recorded as
	// `cmd/command-with-an-option-nobody-has`, with a letter no preset owns: a
	// first probe using a letter that is a real option somewhere read that
	// implementation as tolerant off its own feature.
	CommandRejectsUnknownOption Answer

	// GetoptsRejectsUnknownOption is the same question for `getopts`, which has
	// no options at all here — so any leading `-` word is the one being asked
	// about, and it would otherwise be the optstring.
	//
	// Yes makes `getopts -q o` an unknown option; No makes `-q` the optstring.
	// Recorded as `getopts/a-dash-word-where-the-optstring-belongs`, with a
	// letter no preset owns: a first probe using a letter that is a real option
	// somewhere left that implementation's answer standing wrongly at No.
	GetoptsRejectsUnknownOption Answer

	// ShiftCountIsArithmetic reads `shift`'s operand as an expression rather
	// than as a plain number: `shift 1+1` moves two and `shift n` moves whatever
	// n holds.
	//
	// An unset name is zero in an expression, so under Yes `shift abc` shifts
	// nothing and succeeds, where No calls it a number it cannot read.
	ShiftCountIsArithmetic Answer

	// ReportsAKilledCommandInACommandSubstitution remarks on a command that a
	// signal ended inside `$(…)`.
	//
	// An implementation may answer No here and still remark on the same command
	// inside `( … )`, so this is not the subshell question in another spelling.
	//
	// Asked only inside a substitution, so a preset that answers the wider
	// question the same way everywhere is not asked twice, and one that remarks
	// on no killed command at all never reaches it.
	ReportsAKilledCommandInACommandSubstitution Answer

	// TrapBodyLine is which lines a diagnostic from inside a trap's body
	// names. See TrapBodyLineStyle.
	TrapBodyLine TrapBodyLineStyle

	// ExitTrapFiresPastTheEnd counts the EXIT trap as having fired on the line
	// after the script's last, rather than on its first.
	//
	// Asked only by a preset whose TrapBodyLine needs a firing line at all, and
	// only for EXIT, which has no line of its own. Yes reports the line the
	// parser stopped at; No makes an EXIT body read like a small script of its
	// own.
	ExitTrapFiresPastTheEnd Answer
	// SelectPromptNeedsTerminal withholds PS3 unless the input is a terminal,
	// which is why a transcript can have the menu in it and no prompt.
	SelectPromptNeedsTerminal Answer
	// SelectTakesUnterminatedReply counts a final reply that has no trailing
	// newline, so `printf 2 | sh -c 'select x in a b; do ...'` picks `b`.
	// Answering No ignores the line and ends the loop with 1.
	//
	// The same question `read` answers, and the opposite outcome — `read` is
	// unanimous and this is split, so that one is the core's behavior and this
	// one is an axis. Reachable only from a pipe or a file, since a terminal
	// ends every line.
	SelectTakesUnterminatedReply Answer

	// SelectEofIsSuccess makes the input running out a success rather than a
	// report of 1.
	SelectEofIsSuccess Answer
	// SelectAssumesUnboundedWidth treats an unset COLUMNS as no limit rather
	// than as 80 — with no terminal to ask, Yes puts forty items on one line. It
	// does not arise for a menu that is always vertical, which leaves such a
	// preset unanswered.
	SelectAssumesUnboundedWidth Answer
	// SelectEofEndsPromptLine writes a newline to standard error when the input
	// runs out, closing the line the prompt left open. Closing the line on
	// standard *output* instead is a different question and the field below.
	SelectEofEndsPromptLine Answer
	// SelectEofPrintsNewline writes a newline to standard *output* when the
	// input runs out — the one thing this loop prints that does not go to
	// standard error.
	SelectEofPrintsNewline Answer

	// AssignmentUpdatesPipelineStatus counts a bare assignment as a command for
	// the pipeline-status record, so `false | true; x=1` replaces the two
	// elements with one holding 0. Answering No leaves them.
	//
	// A bare assignment is one with no command name *and no redirection*:
	// `false | true; x=1 >/dev/null` replaces the elements under both answers,
	// because the redirection is what makes it a job. Negation does the same,
	// and for the same reason — see recordSingleStatus.
	AssignmentUpdatesPipelineStatus Answer
	// TestAndArithmeticUpdatePipelineStatus counts `[[ … ]]` and `(( … ))` as
	// commands for the pipeline-status record. Answering No leaves the elements
	// the last pipeline left, which is what makes the shape real code uses work:
	//
	//	cmd | filter
	//	if (( pipestatus[1] == 141 )); then …
	//	elif (( pipestatus[1] )); then print "failed ($pipestatus[1])"
	//	fi
	//
	// Under Yes the first `(( … ))` overwrites the array it just read, the
	// `elif` reads that instead, and the message reports the status of the test
	// rather than of the pipeline — a genuine failure printed as 0.
	//
	// One axis for the two constructs because nothing separates them: a preset
	// with the record answers both the same way. The two are grouped with the
	// bare assignment above by what they are not — running all three without
	// making a job, and a job is what writes the record.
	TestAndArithmeticUpdatePipelineStatus Answer
	// NegatedTestRecordsThePostNegationStatus writes the status a `!` in front
	// of `[[ … ]]` or `(( … ))` produced, rather than the one the construct
	// itself reported. After `false | true`, so a replaced record is visible:
	//
	//	                 ! [[ a = a ]]   ! [[ a = b ]]   ! false
	//	Yes              1               0               1
	//	No               0               1               1
	//
	// The first two columns are the axis and the third is why it is confined to
	// these two constructs: an ordinary command records what *it* reported under
	// both answers, so `!` is not a rule about negation in general.
	// `! { [[ a = a ]]; }` and `! ( [[ a = a ]] )` record 0 under Yes as well —
	// the compound reports its own status and the `!` does not reach the record
	// — which is what says this is about the construct and not about the shape
	// of the line.
	//
	// A redirection does not move it: `! [[ a = a ]] >/dev/null` is the same as
	// without one, where a redirection *does* move the axis above. So the two
	// are asked separately even though they name the same two constructs.
	//
	// Silent when it is wrong: a plausible one-element record, no diagnostic,
	// and a script branching on `${PIPESTATUS[0]}` after a negated test reads
	// the opposite of what it was written against.
	NegatedTestRecordsThePostNegationStatus Answer
	// PromptAsksAgainAfterARefusedToken draws the continuation prompt for a
	// construct the parser has **refused**, rather than refusing it where it
	// stands.
	//
	// Read by the front end rather than by the interpreter: it is about what a
	// prompt does with a line, which is `repl`'s to do and `driver`'s to carry —
	// the same shape as PlusSignedCommandStringIsDollarZero, and a plain bool
	// for the same reason, since a prompt has no way to refuse to run over an
	// unanswered axis.
	//
	// Feeding `echo one`, `if; then`, `echo three` to an interactive shell, No
	// refuses at once, draws no PS2, and then runs `echo three`; Yes draws PS2
	// and waits. `while; do` splits the same way, and `for do` and `case in`
	// split *neither* way — every reading prompts for those, because they are
	// input that has not finished rather than input that is wrong.
	//
	// The cost of answering yes where the shell answers no is a command
	// disappearing: the next line typed is read as part of the construct already
	// refused, so `echo three` never runs.
	PromptAsksAgainAfterARefusedToken bool

	// CompoundPipelineStatusRecord is what a compound command does to the
	// pipeline-status record — see CompoundPipelineStatusPolicy, whose two
	// answers are two *mechanisms* rather than two values for one rule.
	//
	// The first is a question about the **parse** and not about what ran, which
	// is what makes it worth an axis at all. Each line after `false | true`, so
	// a replaced record is visible:
	//
	//	false | true; if [[ a = b ]]; then :; fi              0
	//	false | true; if [[ a = b ]]; then [[ b = b ]]; fi    1 0
	//
	// Neither body runs — the condition is false both times — and the only
	// difference is the text inside `then`. An **unexecuted** `:` is enough to
	// make the compound count as a command.
	//
	// So the rule composes: a compound counts where its body holds anything that
	// would count on its own, by the two axes above, and nothing else.
	//
	//	{ :; }                       0      { [[ a = a ]]; }        1 0
	//	{ [[ a = a ]]; :; }          0      { x=1; }                1 0
	//	{ { :; } }                   0      { { [[ a = a ]]; } }    1 0
	//	while false; do [[ a=a ]]; done  0  while [[ a = b ]]; do [[ a=a ]]; done  1 0
	//
	// The `while` pair is the one that says the condition is body too, and the
	// nested pair is the recursion. Four shapes answer for themselves whatever
	// they hold:
	//
	//	( [[ a = a ]] )       0     a subshell is a job however it ends
	//	{ coproc cat; }       0     and so is a coprocess
	//	{ [[ a = a ]] & }     0     and so is anything backgrounded
	//	{ time [[ a=a ]]; }   0     and the timed pipeline reports
	//	{ f() { :; }; }       1 0   a *definition* runs nothing
	//	select … do :; done   0     counts with a body that would not
	//
	// A redirection on the compound writes the record whatever the body says —
	// `{ [[ a = a ]]; } >/dev/null` and `if … fi >/dev/null` are both one
	// element — which is the same rule the two axes above follow, and for the
	// same reason: the redirection is what makes the job. That is this
	// mechanism's rule and not the other's.
	//
	// The second mechanism is not this one's opposite: the compound writes
	// **nothing**, and the record it leaves is whatever the last pipeline that
	// actually ran inside it wrote. Each line after `false | true`:
	//
	//	if false; then :; fi              1     the condition ran and wrote it
	//	if [[ a = b ]]; then :; fi        1     and so did this one
	//	while false; do :; done           1
	//	case a in b) :;; esac             1 0   nothing ran: the record stands
	//	for i in ; do :; done             1 0
	//	{ [[ a = a ]] & }                 1 0   a background job writes nothing
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
	// the pipeline's elements should still be there.
	CompoundPipelineStatusRecord CompoundPipelineStatusPolicy
	// UnsetEndsTheProducedPipelineStatus makes `unset` permanent, so the name
	// never fills again; answering No lets the producer outlive it.
	//
	// It is the opposite of what a produced *scalar* does, where unset ends it
	// under both answers — `unset RANDOM` leaves an ordinary empty name.
	UnsetEndsTheProducedPipelineStatus Answer

	// ArrayScalarIsTheWholeArray decides what a plain `$a` gives when `a` is an
	// array: every element joined by a space, or the first element alone. A
	// preset with no arrays leaves the axis absent rather than false.
	ArrayScalarIsTheWholeArray Answer

	// KeyedTableScalarIsTheFirstValue says *which* element a plain `$m` gives
	// when `m` is a keyed table and the axis above has answered "one element".
	//
	// Two readings, and they are the same disagreement about whether such a
	// table has an order at all: looking up the key `0` and handing back nothing
	// when there is no such key, or reading the table as an ordered list and
	// handing back the first value in whatever order it lists.
	//
	// A second axis rather than a widening of the first, because presets that
	// share the first answer need not share this one, and because it is
	// reachable only after the first has been answered — where a bare name is
	// the whole table it is never asked.
	//
	// With the table `m=(a 1 b 2)`, one reading gives `1` and the other gives
	// nothing. `m=(z 9 a 1)` giving `9` is what says it is the order and not a
	// sort, and `m=(a 1 0 x)` giving `1` is what says it is not the key `0`
	// under another name.
	//
	// "First" is whatever order `${m[@]}` yields, which is a separate question
	// from this one — the axis says which end of the order to read and not what
	// the order is. That order is **this implementation's own**, and
	// deliberately: see KeyedTableOrder in docs/spec/semantics.md. It is not
	// insertion order anywhere measured, so this axis agrees with what it was
	// taken from exactly when the two orders happen to coincide — a table of
	// one, and a table whose keys hash into their sorted order.
	KeyedTableScalarIsTheFirstValue Answer

	// ArrayBaseIsZero indexes arrays from 0 rather than counting from 1. A
	// preset with no arrays at all leaves the axis absent rather than false.
	ArrayBaseIsZero Answer

	// BareSubscriptIsASubscript reads the `[…]` an *unbraced* `$name` carries as
	// a subscript, rather than as three ordinary characters behind the
	// parameter. `$a[1]` is an element where it says yes and `${a[0]}` followed
	// by `[1]` where it says no; `${a[1]}` is unaffected either way, because the
	// braces settle where the expansion ends.
	//
	// An axis rather than a grammar flag, and the difference from
	// syntax.Dialect.BareSubscript is the whole point. That flag decides whether
	// a grammar has the construct at all — whether the brackets belong to the
	// expansion or are the next thing in the word — and it is answered when the
	// word is read. This decides what the construct *means*, and it is answered
	// when the word is expanded: the answer moves at run time, and a function
	// body written under one answer and called under the other takes the
	// caller's. Deciding it while reading gives a shell that is right in a
	// script and wrong in `eval`, or the reverse.
	//
	// Only reachable where the grammar flag is on, which is why a preset with no
	// such construct leaves it unanswered rather than false: turning the grammar
	// on without answering this is a gap, and should say so out loud rather than
	// pick a side.
	//
	// The two halves of the no answer are one answer. The parameter loses the
	// subscript *and* the brackets become text, and the text is the word's like
	// any other — expanded, split and read as a pattern where the word around it
	// would be. With `a=(x y z)`, No gives `x[1]` for `$a[1]`, `b=2` makes
	// `$a[$b]` into `x[2]`, and an unquoted `$a[1]` is the pattern `x[1]` —
	// which is the point of saying no at all, since it is what leaves a
	// `$dir[0-9]*` written in a script for another shell the glob its author
	// meant.
	BareSubscriptIsASubscript Answer

	// SubscriptCommaIsARange reads the comma in `${a[1,3]}` as the separator of
	// a range — elements 1 through 3 — rather than as the arithmetic comma
	// operator, whose value is its right operand and names element 3 alone.
	//
	// The same characters with two meanings, which is what puts it here rather
	// than in a grammar flag: `${a[1,3]}` is one subscript wherever subscripts
	// exist, and what it says is disagreed about. On `a=(w x y z)`, Yes gives
	// `w x y` and No gives `z`.
	//
	// Asked only where the two readings differ, which is what keeps `${a[2,2]}`
	// — one element under either — from needing an answer.
	SubscriptCommaIsARange Answer

	// SubscriptIsAQuotingContext runs an associative array's subscript through
	// quote removal, so the key is the text *inside* its quotes and escapes.
	// Answering No takes the subscript exactly as written — substitutions
	// performed, and every other character, quotes and backslashes included,
	// kept.
	//
	// Storing under one spelling and reading with the other is what makes it
	// visible, because a key that is one string under both answers hides it:
	//
	//	m["k"]=W; kk='"k"'   ${m[$kk]}  Yes ""    No W
	//	                     ${m[k]}    Yes W     No ""
	//	v=k; q[k]=K          ${q["$v"]} Yes K     No ""
	//	                     ${q[$v]}   Yes K     No K
	//
	// The last pair is the crisp form of it: the substitution is performed under
	// both answers and only the quote characters around it differ, so this is a
	// rule about *quoting* and not about expansion.
	//
	// It is one rule with the search operand, reached from the other side —
	// `${b[(r)"beta"]}` finds an element whose value is the six characters
	// `"beta"` — which is why the two share Runner.searchOperand rather than
	// reconstructing the text twice.
	//
	// Two things stay unanimous and must not move with it. A bare `@` or `*` is
	// still the whole array; *quoted*, it is a key, so `${n["@"]}` looks one up
	// and finds nothing. And an associative key is never space-trimmed:
	// `${p[ s ]}` looks up three characters, so a key stored under `s` is not
	// found by it. A preset with no arrays leaves the axis absent rather than
	// false.
	SubscriptIsAQuotingContext Answer

	// PatternEscapeReaches is the set of characters a backslash escapes inside a
	// pattern. Empty means **every** character, so `bet\a` matches `beta`, the
	// backslash spent on a character that needed none.
	//
	// A preset may name a set instead, and where it does the set is exactly its
	// pattern metacharacters — a backslash before anything else is a literal
	// backslash *and* the character after it, so `bet\a` matches the five
	// characters `bet\a` and matches `beta` not at all.
	//
	// Measured by handing the matcher a raw backslash, which is the only way to
	// ask: quote removal takes an escape off a pattern written in the source
	// before the matcher ever sees it, so a `case` pattern spelled `bet\a` is
	// `beta` everywhere and says nothing about this. What does ask it is a
	// *substituted* pattern — `p='bet\a'; case beta in $p)` — wherever the
	// result of an expansion is matched at all.
	//
	//	a named set is typically:  - = ! * ? [ ] ( ) | ^ ~ # < >
	//	and typically excludes:    letters, digits, _ . / + : % & @ , " ' space { } $
	//
	// The set is written down rather than derived from the other pattern answers
	// because it is not the same set: `-`, `=`, `!`, `^`, `~` and `#` are in it,
	// and this matcher gives none of the six a meaning of its own in a pattern.
	//
	// One row is deliberately not modeled: escaping every character *but* `^`,
	// so a pattern `x\^y` does not match `x^y` where `x\.y` matches `x.y`. One
	// character of one implementation, recorded in the corpus and filed rather
	// than given a value here.
	PatternEscapeReaches string

	// PatternClasses is the character-class names a dialect answers **beyond
	// the twelve POSIX ones**, space separated. Empty is the common ground:
	// alnum, alpha, blank, cntrl, digit, graph, lower, print, punct, space,
	// upper and xdigit, which every shell in the panel answers alike.
	//
	// A roster rather than a flag per name, and a roster rather than an
	// enumeration, for the reason EchoOptions and ReadOptions are strings: what
	// differs is **which names exist**, and that is data. What each name *means*
	// is not in dispute — nothing disagrees about `ascii` — so there is no axis
	// to switch, only a set to declare. A preset that leaves it empty is not
	// unanswered; it is saying the twelve and nothing else, which is a measured
	// answer.
	//
	// Measured with `[[ $c = [[:NAME:]] ]]` a character at a time. Beyond the
	// twelve, `ascii` is the one several presets share; the rest — `IDENT`,
	// `IFS`, `IFSSPACE`, `INCOMPLETE`, `INVALID`, `WORD` — belong to a single
	// preset.
	//
	// A name outside the twelve **and** outside this roster matches nothing,
	// silently, at status 0 — and that is not a gap: it is what every preset
	// does with `[[:nosuchclass:]]`, measured rather than assumed. So the defect
	// this was written for was never generic. A class came back empty because
	// the *name* was missing, and only the names go here.
	//
	// The names are case-sensitive where they exist: `[[:ident:]]` and
	// `[[:ASCII:]]` both match nothing.
	//
	// Two of them read shell state rather than a fixed set of characters —
	// `IFS` is the field separators as they stand and `WORD` is the letters and
	// digits together with `$WORDCHARS` — which is why they are resolved where a
	// Runner can be asked and not in a table.
	PatternClasses string

	// BracketEscapeIsAlsoAMember says a backslash that protects a member of a
	// bracket expression is a member of the set itself.
	//
	// Under False, `[\)]` handed to the matcher with the backslash still in it
	// is the one-character set `)`. Under True the same set holds the backslash
	// as well.
	//
	// Measured through `${~p}`, which is the only construct that hands this
	// matcher a bracket expression holding a raw backslash — a pattern *written*
	// in the source has had its escapes spent by quote removal long before,
	// which is why the two routes can disagree at all and why the source route
	// is unanimous. Four values, four exact hits:
	//
	//	p='[\)]'   matches `)` and `\`, not `a`
	//	p='[\-z]'  matches `-`, `z` and `\`, not `y` — no range is formed
	//	p='[\a]'   matches `a` and `\`
	//	p='[\]]'   matches `]` and `\`
	//
	// The second row is what says the answer is *also a member* rather than *not
	// an escape*: the `-` behind the backslash stays a member instead of
	// becoming the range operator, so the protection happens there too. Both
	// halves are true at once, which is exactly what this field turns on.
	//
	// It reaches only the results of expansions, in expansionPattern, because
	// that is the only place a backslash arrives inside a bracket expression
	// without having been put there to say "the source quoted this". Reading it
	// in the matcher instead would take the source route with it and break the
	// unanimous half.
	BracketEscapeIsAlsoAMember bool

	// LongestMatchTakesTheWrittenArm decides which match `${x##pat}` removes,
	// and which one `${x//pat/rep}` replaces, when `pat` holds an alternation
	// whose arms take different lengths: the arm that was written first, or the
	// longest of them.
	//
	// With `x=abc`, `${x##(a|ab)}` is `bc` under Yes and `c` under No, so the
	// two spellings disagree about which of `a` and `ab` came off. The group's
	// spelling differs by preset — a bare `(a|ab)` under one, `@(a|ab)` with
	// extended patterns switched on under another — and the answer does not.
	//
	// Yes is not "the shortest arm", and that is what makes it a search order
	// rather than a second length rule: `${x##(a*|ab)}` empties the value under
	// Yes, because the first arm is tried first and then matches as much as it
	// can. `${x##(a|ab)c}` empties it too — the first arm is a preference and
	// not a refusal, so the search falls back to a later arm where the rest of
	// the pattern needs it. An empty arm is an arm: `${x##(|a)}` removes nothing
	// under Yes and removes `a` under No.
	//
	// **Asked where the longest match is wanted and the end of that match is
	// free to move**, which is where the answers actually split, and which is
	// two operators rather than one. The single `#` takes the shortest match
	// under both, arms or no arms — `${x#(ab|a)}` is `bc` either way — and the
	// unflagged suffix trims take the longest: `${x%%(|bc)}` is `a` under Yes,
	// where a written-arm search would have taken the empty arm and removed
	// nothing. So an axis worded for trims in general would have moved three
	// rows that are agreed about.
	//
	// The *substitution* is the second operator. `${x//(a|ab)/X}` on `abc` is
	// `Xbc` under Yes and `Xc` under No, exactly as the trim splits, and every
	// unanchored spelling goes the same way: `${x/(|a)/X}` is `Xabc` under Yes
	// and `Xbc` under No, and `/#` follows because its end is still free. `/%`
	// does not, because pinning the end leaves the arms no length to disagree
	// about — `${x/%(c|bc)/X}` and `${x/%(bc|c)/X}` are both `aX` either way.
	//
	// Under a shortest-match flag the minimum over every arm is wanted, so the
	// arms cannot disagree there either.
	//
	// It reaches the match-returning flag and the capture flags with the same
	// answer, because they are the same match seen from the other side:
	// `${(M)x##(a|ab)}` is `a` where the trim leaves `bc`, and
	// `${x##(#b)(a|ab)}` reports `a` in the first capture.
	//
	// Asked only where the two readings land in different places, which is why
	// an ordinary pattern never reaches it: a pattern with no alternation has
	// one reading, and `(ab|a)` — the arms in decreasing length — has two that
	// agree. See armEnd, which is the one place both operators ask it, and
	// interp/trimarm.go for the search.
	LongestMatchTakesTheWrittenArm Answer

	// EmptyReplacementPattern is what `${v//\/X}` — a span replacement whose
	// pattern is empty — matches in the unanchored spellings. With `v=abc` and
	// `e=`:
	//
	//	                  decline  take-when-empty  match-everywhere
	//	${v///X}          abc      abc              XaXbXc
	//	${v/$e/X}         abc      abc              Xabc
	//	${e///X}          (empty)  X                X
	//	${e//x/X}         (empty)  (empty)          (empty)
	//
	// Three answers and not two, which the empty *value* row is the whole of:
	// one reading declines the pattern outright and another takes it where there
	// is nothing to scan. The last row is the control that says the `X` is a
	// match and not something an empty value produces on its own.
	//
	// It is a question about the pattern's *text* rather than about empty
	// matches in general, and the discriminating probe is a non-empty pattern
	// that matches only the empty string: with extended patterns on,
	// `${v//@(|)/<>}` replaces at every position under the readings that decline
	// here, so neither is refusing empty matches — see
	// ReplacementEmptyMatchDeclined, which is where those two then part.
	//
	// Asked only where the pattern is empty and the spelling unanchored. A
	// pattern with anything in it never reaches it, and the anchored forms are
	// their own row: `${v/#/X}` may be taken or declined independently, which is
	// a preset declining an *anchor* it takes nowhere else.
	EmptyReplacementPattern EmptyReplacementPatternPolicy

	// ReplacementEmptyMatchDeclined is which empty match a global replacement
	// refuses to take, once the pattern is one that can match empty at all.
	//
	// Every reading agrees that a match reaching the end of the value ends the
	// scan — `${v//*/X}` is one `X` — and they part over an empty match that
	// does not. With `v=abc`, the replacement written `<>` so each match shows,
	// and the pattern "empty or one letter":
	//
	//	          after-a-match      at-the-end
	//	(b|)      <>a<><>c           <>a<>c<>
	//	(x|)      <>a<>b<>c          <>a<>b<>c<>
	//	(c|)      <>a<>b<>           <>a<>b<>
	//	(a|)      <><>b<>c           <>b<>c<>
	//
	// Row one is the discriminator: one reading has no `<>` between `b` and `c`
	// where the other does, and has one after `c` where the other does not.
	//
	// Row three is the control both readings answer the same way and both must
	// keep, because it is the rule nobody disputes reached from a third
	// direction — `c` matches at the last unit, so the scan ends there under
	// either policy.
	//
	// Asked at the two positions the readings land differently on, and nowhere
	// else: an empty match where the match before it ended, and the end of the
	// value stepped onto after an empty match. A pattern that cannot match empty
	// reaches neither.
	ReplacementEmptyMatchDeclined EmptyMatchDeclinedPolicy

	// ParameterIsSetSeesPositionals lets `-v 1` ask about a positional
	// parameter, and `-v 0` about the shell's name.
	//
	// With `set -- p q`, `[[ -v 1 ]]` is set under true and **unset** under
	// false, and `[[ -v 0 ]]` splits the same way. It is the operator declining
	// to treat a digit as a name rather than a lookup coming back empty —
	// `${1+s}` is `s` under both — which is why the answer is here rather than
	// in the parameter table.
	//
	// A positional past `$#` is unset under both and needs no axis: with two
	// parameters set, `[[ -v 3 ]]` is unset either way.
	ParameterIsSetSeesPositionals bool

	// ParameterIsSetSeesSpecials lets `-v ?` and its fellows — `#`, `$`, `!`,
	// `*` and `-` — ask about a parameter spelled as one punctuation character.
	//
	// True answers set for every one of them and false answers unset for every
	// one. Again the parameters are there under both and it is the operator that
	// does not look: `[[ -n ${?+s} ]]` and `[[ -n ${#+s} ]]` are set either way.
	//
	// `@` is not one of them and is unset under both — with positional
	// parameters set, and under the answer that sees every other character — so
	// it is excluded outright rather than by this axis. See isSetNameKind.
	ParameterIsSetSeesSpecials bool

	// ScalarSubscriptIsACharacter reads `${s[2]}` on a plain string as its
	// second character, rather than as an element of the one-element array a
	// scalar reads as.
	//
	// On `s=hello`, Yes gives `h` for `${s[1]}` and `e` for `${s[2]}`; No gives
	// `hello` for `${s[0]}` and nothing for either of the others. Both readings
	// answer, neither reports, and an empty string is a plausible element — so a
	// script cannot tell which it is on except by the value it gets, which is
	// the definition of a conflict rather than an addition.
	//
	// A range and a character go together: `${s[2,4]}` is the substring `ell`
	// under Yes, and the arithmetic comma's element 4 — nothing — under No. But
	// they are two axes, because `${a[1,3]}` on an *array* is a range without a
	// character anywhere in it.
	//
	// Asked only where the two readings differ: a one-character string at the
	// preset's first subscript is itself under either reading.
	ScalarSubscriptIsACharacter Answer

	// MultibyteEncodingIsHonored decodes the locale's character encoding, so
	// that `${#s}`, `${s:off:len}` and a subscript on a scalar count characters
	// rather than bytes.
	//
	// With `s=héllo; echo ${#s}` under a UTF-8 locale, Yes answers 5 and No
	// answers 6. Under `LC_ALL=C` every answer is 6 — so this is not "some count
	// characters", it is "some honor the encoding the locale names and one has
	// no multibyte decoder at all". `s=日本語; echo ${#s}` separates them
	// further: 3 against 9.
	//
	// Which encoding is in force is **not** a second axis. It is state read off
	// the runner's own variables, exactly as PATH and IFS are, and it moves
	// inside a running shell: `LC_ALL=C; s=héllo; echo ${#s}` gives 6 under
	// every answer with nothing exported. See interp/multibyte.go for the
	// precedence and the codesets, and driver/startup.go for the same reasoning
	// applied to POSIX mode.
	//
	// Silent either way, which is why it is an axis and not a bug in one place:
	// both answers are plausible numbers and nothing is reported.
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
	// `LC_CTYPE` and `LANG` are all unset — which is what `env -i`, a cron job
	// and a container have, and where a person's terminal never is.
	//
	// Three operators read the same state — a length, a case mapping, and a
	// `\u` escape — and an implementation gives the *same* reading in all three,
	// which is what makes this one axis rather than one per operator. Yes reads
	// an unset locale as UTF-8-capable; No reads it as C.
	//
	// Silent either way: a length is a plausible number and a case-mapped word
	// is a plausible word, so a script carried from a terminal into a container
	// changes answer with nothing reported.
	//
	// Asked only where the two readings differ — a value whose bytes are all
	// ASCII, an ASCII code point, and case mapping below 0x80 are the same under
	// both — and only after the operator's own axis has said the question can
	// matter. See interp/multibyte.go for the order.
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
	// So it is one preset's operand rather than a core one. A reading that
	// refuses it names **`a+`** — the text in front of the `=` — and not the
	// whole operand, which is what a bad operand is otherwise quoted back as.
	// That is the tell that the `+=` was read as an operator too, and the name
	// it left then refused.
	//
	// The value joins through the name's attributes, which is the same join the
	// bare statement performs and not a second rule: `declare -i a=1;
	// declare a+=2` is 3, `declare -a arr=(p q); declare arr+=x` is `px q`, and
	// a declared table joins its `0` key.
	//
	// Asked only where an operand's name ends in `+` and a value follows it. A
	// `+` with no `=` is not this spelling — `declare a+` is refused as a name
	// under every answer, the one that takes the operator included — so nothing
	// well formed ever reaches the question.
	DeclarationTakesAnAppendOperand Answer

	// NegativeSubscriptCountsOverAPromotedScalar resolves a negative subscript
	// on the left of `=` against the array a held *scalar* is about to become,
	// rather than against the elements the name already has — of which a scalar
	// has none.
	//
	// An element write over a name holding a string keeps the string as the
	// first element, which is core and unanimous: `a=abc; a[1]=x` leaves `abc`
	// beside the `x` wherever arrays exist. When that happens relative to
	// reading the subscript is not unanimous, and a subscript counting back from
	// the end is the only spelling that can tell. With `a=abc; a[-1]=x`, Yes
	// promotes and then counts back over the one element it made, leaving a
	// single `x`; No counts back first and refuses as out of range.
	//
	// A preset with no negative subscripts at all refuses the spelling outright,
	// which is its absence rather than a third answer.
	//
	// Asked only where there is a scalar to promote *and* the subscript is
	// negative. A non-negative one lands at the number it names whether the
	// promotion happened before or after it, and an unset name has nothing to
	// promote, so `unset a; a[-1]=x` is refused under both and needs no answer.
	//
	// A preset where a subscript on a string names a character never arrives
	// here at all: there is no array to promote into on that side.
	NegativeSubscriptCountsOverAPromotedScalar Answer

	// NegativeSubscriptPastTheStartInserts places a new element in front of
	// every other when a negative subscript counts back past the first one:
	// `a=(p q); a[-3]=x` leaves three elements with `x` at the head, however far
	// past the start the subscript reached. Answering No refuses the subscript
	// and ends the script.
	//
	// Asked only for a *negative* subscript that lands before the first element,
	// which is the only spelling that can. A non-negative one below the base —
	// `a[0]` where the first element is 1 — is refused under every answer, so it
	// needs none.
	NegativeSubscriptPastTheStartInserts Answer

	// ArrayLiteralSubscriptIsAKey reads a subscript written inside an array
	// literal as the text between the brackets rather than as an arithmetic
	// expression — and, because the two go together, makes such a literal
	// declare a keyed array rather than an indexed one.
	//
	// One concept with two consequences, like whether an assignment prefix
	// survives a special builtin. Under Yes, `a=([1+1]=c)` stores under the
	// three characters and `${a[2]}` finds nothing; under No the subscript is
	// evaluated and the value lands at 2.
	//
	// Asked only where the two readings differ. A plain decimal numeral
	// evaluates to itself, so `a=([2]=c)` fills the same slot either way and
	// never reaches the question — which is what keeps the ordinary way to build
	// a sparse array available in a core that has chosen no preset.
	//
	// A preset with no array literal at all leaves the axis absent rather than
	// false.
	ArrayLiteralSubscriptIsAKey Answer

	// DollarZeroNamesTheInnermostCall makes `$0` the innermost thing the shell
	// has been called into rather than the shell's own name: the function being
	// run, or the file being sourced.
	//
	// One concept with two consequences, and one field because nothing splits
	// them: where both exist they sit under a single option, and turning that
	// option off takes both away together — `$0` inside a function goes back to
	// the script's name in the same breath as `$0` inside a sourced file does.
	// Under No there is neither: `$0` is the script's name inside a function,
	// inside a file it sourced, inside a file that file sourced, and inside a
	// function defined by one of them.
	//
	// Innermost is the whole of the rule and is measured rather than assumed: a
	// function that sources a file reports the *file* while that file runs and
	// the function's name again afterwards, and a function defined in a sourced
	// file reports its own name and not the file it came from. So this is a
	// question about the top of the call stack and not about whether a function
	// is anywhere on it.
	//
	// The file is named as the operand was written — `. ./inc.sh` reports
	// `./inc.sh` and a bare name found on PATH reports the bare name — which is
	// the same spelling the call stack and the diagnostics use.
	//
	// A shell's startup files are outside this. They are read by the shell
	// rather than sourced by a script, and `$0` inside one is the shell's own
	// name even under Yes.
	DollarZeroNamesTheInnermostCall Answer

	// BuiltinSyntaxErrorFatal ends a non-interactive shell when text handed to a
	// special builtin does not parse — `eval "if"`, or a sourced file with an
	// unterminated `if` in it.
	//
	// Yes is the POSIX rule that a special builtin's failure is fatal; No
	// reports it and carries on. One axis covers both callers because they are
	// answered the same way, where the *status* is not — that is two fields on
	// [Diagnostics].
	BuiltinSyntaxErrorFatal Answer

	// EvalRunsWhatItParsed runs the commands `eval` has already read when a
	// later line of its text will not parse, instead of reading the text through
	// and running none of it.
	//
	// Measured by counting a side effect rather than reading a transcript,
	// because the transcript is what buffering can reorder:
	//
	//	$ <shell> -c 'eval "printf x >> f
	//	if; then"'
	//
	// leaves `f` holding `x` under Yes and no `f` at all under No.
	//
	// Not the same question as the wording or the status of the complaint, both
	// of which are [Diagnostics]' and both of which are written either way. What
	// this decides is whether the *work* before the offending line happened.
	//
	// A separate field from the sourced-file one because an implementation may
	// split them — reading a file a command at a time and reading `eval`'s text
	// through first.
	EvalRunsWhatItParsed Answer

	// SourcedFileRunsWhatItParsed is the same question for `.`, and the answer
	// is not always the same one.
	//
	// Measured the same way, with a file holding `printf y >> f` and then
	// `if; then`: `f` holds `y` under Yes and does not exist under No.
	//
	// It is worth more here than for `eval`: a file that sets six names and has
	// a typo on the last line leaves six names set under Yes and none under No.
	SourcedFileRunsWhatItParsed Answer

	// FatalErrorEndsBorrowedTextOnly makes an error that would end a script end
	// only the text a special builtin is running — a file `.` read, or `eval`'s
	// argument — handing the builtin a status and letting the script around it
	// carry on. Answering No ends the shell: nothing after the `.` runs, in the
	// sourcing file or any file above it.
	//
	// Measured with an error worded identically under every answer, so that the
	// row is about the abandonment and not about the operator — a file whose
	// third line reads an unset name under `set -u`, sourced by a file that
	// prints afterwards.
	//
	// Four measured facts make this one axis rather than several:
	//
	//	*Every* error that would end a script behaves this way wherever
	//	anything is caught at all — a readonly assignment, a division by zero,
	//	a bad substitution, an unset parameter. So the axis is about what a
	//	fatal error costs and not about expansion.
	//
	//	Only one file is given up. A file sourced from a file sourced from a
	//	script loses the innermost file alone, and the middle one prints the
	//	line after its own `.`.
	//
	//	It is the *running* `.` and not the file the text came from: a function
	//	defined in a sourced file and called later from the script ends the
	//	shell under every answer. A `.` inside a function is the boundary, and
	//	the function body resumes after it.
	//
	//	`exit`, and errexit firing, are not errors and are never caught —
	//	unanimous. That is what separates this from the neighboring rule that
	//	`exit` in a startup file ends the shell and the files after it are not
	//	read.
	//
	// One axis covers `eval` and `.` because they are answered the same way —
	// the same errors fatal, the same ones caught, `exit` uncaught — which is
	// the arrangement BuiltinSyntaxErrorFatal already has for the same pair. The
	// *status* is not the same for both, and that is
	// Diagnostics.SourcedFatalStatus, which the file route passes and `eval`
	// does not.
	FatalErrorEndsBorrowedTextOnly Answer

	// ParamErrorIsAnExitRequest makes `${x?word}` and `${x:?word}` a request to
	// stop rather than an error, so no boundary catches it.
	//
	// Asked only where the answers differ, which is at a boundary that gives up
	// one file: the same `${NOPE?msg}` inside a file `.` read reports and lets
	// the sourcing file carry on under No and ends the whole shell under Yes,
	// where an unset parameter under `set -u` two lines away is caught by both.
	// The startup-file boundary splits the same way. At the top level of a
	// script both operators end the shell under every answer, so nothing there
	// has a question to ask.
	//
	// Yes is a documented reading rather than an inconsistency: the `?` form is
	// specified to print the word and *exit the shell*, which is the same family
	// as the `exit` builtin and not the family of a diagnostic.
	ParamErrorIsAnExitRequest Answer

	// DotWithNoOperandIsAnError decides whether `.` with no filename is a
	// failure at all. Answering No does nothing and reports success.
	//
	// Separate from the status and from the fatality because `.` alone splits
	// four ways — 0; 2 surviving; 2 fatal; 1 surviving — and one field with four
	// answers would have to invent a type to hold what is really three
	// independent questions.
	DotWithNoOperandIsAnError Answer

	// DotDirectoryOperandIsAnError decides whether `.` naming a **directory** is
	// a failure at all, with `. ./` from a script file in a scratch directory:
	//
	//	No   silent, status 0
	//	Yes  `.: ./: is a directory`, status 1, and the script carries on —
	//	     or `cannot open [Is a directory]`, and the script ends
	//
	// A No opens the directory, reads no commands out of it and calls that a
	// script that did nothing. So it is an axis and not a wording fix — a fix
	// that answered only the sentence would leave the status wrong and invent a
	// diagnostic for the presets that report nothing.
	//
	// Separate from DotMissingFileFatal, which decides what an error here
	// *costs*. Separate from DotWithNoOperandIsAnError for the same reason that
	// one is separate from the status — three independent questions about one
	// builtin.
	//
	// A path that does not exist is not this axis: that one is reported under
	// every answer, and the tell that this was a different question was
	// answering `no such file or directory` at 127 for a path that does exist.
	DotDirectoryOperandIsAnError Answer

	// DotMissingFileFatal ends the script when `.` cannot read its file — the
	// same split as ShiftPastEndFatal, and for the same POSIX reason.
	DotMissingFileFatal Answer

	// DotPassesArguments gives a sourced file its own positional parameters from
	// the words after the filename, restoring the caller's afterwards. Answering
	// No ignores them, so `. f.sh ARG` leaves `$1` as the caller's.
	//
	// With no words after the filename the parameters are left alone under both,
	// so the axis only speaks when there are some.
	DotPassesArguments Answer

	// ExecFailureRunsExitTrap runs a `trap … EXIT` handler when `exec` could not
	// run the command it was given.
	//
	// A *successful* exec runs no handler anywhere, and that is not an axis: the
	// trap died with the process the exec replaced. Only the failure has a shell
	// left to decide anything.
	ExecFailureRunsExitTrap Answer

	// TimesRejectsArguments makes `times` refuse an argument rather than ignore
	// it.
	//
	// A preset where `times` is a reserved word answers neither: `times foo` is
	// a *syntax* error there and no builtin ever runs. That is a grammar
	// question rather than this one, and it is recorded in the corpus rather
	// than modeled here.
	TimesRejectsArguments Answer

	// EmptyPathIsTheCurrentDirectory searches the current directory when PATH is
	// set and empty.
	//
	// `PATH=` reads like "nowhere" and is not: an empty PATH is one *empty
	// element*, and an empty element means the current directory, so Yes will
	// still run a command sitting next to the script. Measured with the command
	// in the current directory, which is the only arrangement that tells the two
	// answers apart — with it anywhere else both report not-found and the axis
	// is invisible.
	//
	// `PATH=:` is not this question. Two empty elements is unanimous: every
	// shell searches the current directory for it.
	EmptyPathIsTheCurrentDirectory Answer

	// HashReportsAMissingName has `hash name` complain and answer 1 when the
	// name resolves to nothing. Answering No — which is what a `hash` that is
	// really an alias table under another name does — says nothing and reports
	// success.
	HashReportsAMissingName Answer

	// HashSearchesPathAlone counts only what PATH holds, answering `hash shift`
	// with "no such command" where No accepts a builtin or a function as
	// hashable.
	//
	// Measured with `shift`, which no PATH carries — `cd` is a contaminated
	// probe, since some systems ship a `/usr/bin/cd`. Recorded as
	// `hash/a-builtin-counts-except-in-zsh`.
	HashSearchesPathAlone Answer

	// UnderscoreTracksTheLastArgument moves `$_` to the previous simple
	// command's last expanded argument — the command word itself when it had
	// none, and empty after a bare assignment. Answering No keeps no such
	// parameter at all.
	//
	// What a No holds instead is whatever the environment brought, and nothing
	// when it brought nothing, because `_` is an ordinary name there. That is
	// measured rather than assumed: a claim that it holds the shell's own path
	// survives any probe whose cells are empty either way, so it has to be asked
	// with `_` scrubbed from the environment and again with `_=X` in it, on both
	// the `-c` and the script route.
	UnderscoreTracksTheLastArgument Answer

	// UnderscoreStartsAtTheInvocation writes argv[0] into `$_` before the first
	// command runs, so a script reading it at the top finds how the shell was
	// started. Answering No leaves it as it was.
	//
	// Not the same question as UnderscoreTracksTheLastArgument, which is why it
	// is its own axis rather than a consequence of that one: a preset can answer
	// yes to tracking and still start empty, so a startup write gated on
	// tracking would give it a value it does not have.
	//
	// The value is the *invocation* rather than the executable — the same binary
	// reached through a symlink named `sh` writes `sh` — and rather than `$0`,
	// which a `-c` invocation takes from its first operand.
	UnderscoreStartsAtTheInvocation Answer

	// UnderscoreInheritsFromTheEnvironment lets an `_` the shell was handed in
	// its environment show through. Answering No discards it and starts empty
	// however the shell was invoked.
	//
	// It is the other half of the startup value and it decides what the half
	// above means: argv[0] is written only when the environment said nothing, so
	// an exported `_` wins over the invocation wherever one is read at all. Only
	// reachable with an environment, which is why the case that pins it carries
	// one — no snippet can put a name in the environment of the shell already
	// running it.
	//
	// `_` is exported by some shells as the command they are about to run, so
	// this is not a hypothetical: it is what a shell started by another shell
	// actually finds.
	UnderscoreInheritsFromTheEnvironment Answer

	// FdVariableOutlivesTheCommand keeps a `{name}>f` descriptor open past the
	// simple command that carried it. Answering No takes it back with the
	// command's other redirections, so the number the variable holds is already
	// dead.
	FdVariableOutlivesTheCommand Answer

	// FirstAllocatedDescriptor is the number the shell counts up from when it
	// picks a descriptor for itself — `exec {fd}< file`, and the builtins that
	// hand a number back the same way.
	//
	// The same number is what a socket builtin reports in `$REPLY` and what an
	// open-by-name builtin writes, so a corpus row about either either avoids
	// printing the number or is wrong under some preset.
	//
	// **The one axis with no unanswered state**, and that is measured rather
	// than an omission: there is no shape in which a shell declines to pick a
	// number. A `{name}<file` that reached the allocation is going to be given
	// one, and so is an embedder calling Runner.OpenDescriptor, so there is
	// nothing for a refusal to protect and a zero value that refused would
	// refuse a construct every implementation performs. The zero value is
	// therefore an answer — the common one — and every preset states it anyway.
	FirstAllocatedDescriptor DescriptorAllocationBase

	// UnterminatedHeredocGainsATrailingNewline adds the newline a here-document
	// body never got, where the delimiter never arrived and the input ended
	// mid-line. With `printf 'cat <<X\nbody'`, Yes is `body\n` at five bytes
	// and No is `body` at four.
	//
	// It is reachable no other way, which is why it is worth a field at all: a
	// here-document closed by its delimiter always has a body ending in a
	// newline, so this is the only shape in which the question exists. The
	// corpus cannot see it either — `$( )` strips trailing newlines and the
	// harness trims them — so it is checked by a Go test on the runner's bytes.
	//
	// Asked only where the two answers differ, which is what
	// syntax.Redirect.HeredocAtEOF marks: an ordinary here-document never
	// reaches the question.
	UnterminatedHeredocGainsATrailingNewline Answer

	// ReadFailureInAFileSubstitutionFailsIt is `$(<file)` where the *read* fails
	// after the open worked — a directory is the shape that reaches it. Yes is
	// status 1, No is status 0.
	//
	// The status and the sentence are separate questions, and that separation is
	// measured: an implementation may fail the substitution and say nothing, so
	// a preset could hold either answer with either wording. The sentence is
	// Diagnostics.FileSubstitutionReadError.
	//
	// An open that fails is a different event and is already answered by
	// redirectFailureStatus. This one is the read after a successful open, which
	// is why it cannot ride on that: `$(<nosuch)` and `$(<dir)` can be status 1
	// and status 0 in the same implementation.
	ReadFailureInAFileSubstitutionFailsIt Answer

	// ExecOpenedFdReachesACommand hands a descriptor that `exec`'s own
	// redirection list opened to whatever the shell runs next — the flock and
	// shared-log idioms, and every script that gives a child a logging
	// descriptor. Answering No closes anything above 2 that `exec` opened when
	// another program is invoked, which an implementation that does it states in
	// its manual rather than leaving it to be discovered.
	//
	// POSIX decides nothing here: the Shell Command Language says whether
	// standard input, output and error are open for a utility and is silent
	// about the rest, so both answers conform and there is no majority to defer
	// to on the standard's authority.
	//
	// It is narrower than "the shell hands nothing over", and the boundary is
	// measured. A descriptor the *caller* opened crosses under both answers, and
	// closing it closes it for the child under both. A command's own redirection
	// crosses under both too — `sh -c '… >&3' 3>f` writes, and so does
	// `exec 3>f; sh -c '… >&3' 3>&3`, where restating the number on the command
	// brings it back. What a No withholds is what `exec` opened: the numbered
	// form, a `{v}>f` the shell numbered itself, and a `9<&3` duplicated from an
	// inherited descriptor, while the inherited 3 it was copied from still
	// crosses.
	//
	// Read where the outbound table is built, so an external command and a
	// process replacement get the same answer — measured the same in both, which
	// is one divergence rather than two.
	ExecOpenedFdReachesACommand Answer

	// FdVariableBadCloseIsAnError refuses `exec {name}>&-` when the name holds
	// no descriptor number. Answering No says nothing and reports success.
	FdVariableBadCloseIsAnError Answer

	// FdNumberBoundedByOpenFileLimit refuses a redirection whose descriptor
	// number is at or above the process's soft limit on open files. Answering No
	// accepts the number and lets whatever comes next fail on it, or not at all.
	//
	// There is no *language* bound anywhere — no implementation has a ceiling of
	// its own, and the one that bites is the kernel's `ulimit -n`. A Yes reports
	// the errno it gets: with the limit at 20, `exec 20>f` is
	// `20: Bad file descriptor` and status 1, `exec 19>f` is silent, and
	// lowering the limit lowers the ceiling exactly. A No answers 0 for
	// `exec 8>f` under a limit of 6 and leaves the descriptor unusable, which is
	// the shape of not asking rather than of a different answer.
	//
	// It is asked at the disagreement rather than on every redirection: a number
	// below the limit is nobody's question, and a Runner with no GetRlimit has
	// no limit to be asked about. Reached most often through MultiDigitFdNumber,
	// which is what lets a script write a number that large at all — where only
	// one digit is read, the only way to a descriptor above nine is to let the
	// shell pick it.
	FdNumberBoundedByOpenFileLimit Answer

	// JobControlAbsenceIsReportedFirst refuses `bg` and `fg` before reading the
	// operand when there is no job control. Answering No reads the operands and
	// options first and complains about those.
	JobControlAbsenceIsReportedFirst Answer

	// StoppedJobsHoldTheExit keeps an interactive shell alive when leaving would
	// abandon a job that is stopped: the shell says so and stays, and the
	// attempt has to be made a second time. Answering No leaves at once and the
	// job is left stopped with nothing able to name it.
	//
	// Measured through a pseudo-terminal — a long sleep, ^Z, `exit` — for `exit`
	// and for the end of input alike, which behave the same under Yes.
	//
	// What counts as having been told is measured too, and it is not simply
	// "warned once": a `jobs` listing counts, so `exit` straight after one
	// leaves; any other command does not, so `echo hi` between the ^Z and the
	// `exit` still warns; and a job stopping afterwards starts it over.
	StoppedJobsHoldTheExit Answer

	// HeldExitListsTheJobs follows that warning with the job table — the same
	// rows `jobs` writes. Answering No writes the sentence and nothing else,
	// whatever its own options are set to.
	//
	// Reached only in a shell that holds an exit at all. Asked *with*
	// Runner.ChecksRunningJobsAtExit rather than instead of it, because under
	// Yes the listing is the other half of what that option buys: with the
	// option off the sentence still appears for a stopped job and the table
	// under it does not.
	HeldExitListsTheJobs Answer

	// CdpathAnnouncesTheDirectory prints where CDPATH sent a `cd`, when the
	// winning entry was not a plain dot. Answering No moves in silence.
	CdpathAnnouncesTheDirectory Answer

	// AutoCdAnnouncesTheSubstitution writes the `cd` that a bare directory name
	// was read as, before moving. Answering No moves in silence.
	//
	// Only a preset that has the option at all reaches this, which is why it is
	// unanswered in the base rather than given the quieter default: where
	// nothing can turn the capability on, nothing can ask. See
	// Runner.autoCdInstead.
	//
	// Measured through a pseudo-terminal, since the name is interactive-only:
	// Yes writes `cd -- subdir` and then moves, and the line survives a
	// `2>/dev/null` on the word itself while `exec 2>file` captures it.
	//
	// An axis rather than one answer with an exception, because the two are a
	// conflict and not a subset: there is no ordering in which one derives the
	// other's silence.
	AutoCdAnnouncesTheSubstitution Answer

	// FcEmptyHistoryIsAnError has `fc` report the event it cannot find.
	// Answering No answers a script with silence at 0.
	FcEmptyHistoryIsAnError Answer

	// TestIntegerRefusalIsSilent has `[ a -eq 1 ]` fail with no sentence at
	// status 1. Answering No complains at 2.
	TestIntegerRefusalIsSilent Answer

	// MissingFileIsOlder has `-nt` and `-ot` count a path that does not exist as
	// older than any file that does, so `f -nt missing` and `missing -ot f` hold
	// whenever f exists. Answering No is false unless both files exist.
	//
	// Asked only there: with both files present the comparison is unanimous, and
	// the mirrored cases — a missing file being *newer* — are false under both.
	// One axis for `test`, `[` and `[[ ]]` alike, because the two constructs are
	// answered together.
	MissingFileIsOlder Answer

	// TerminalTestRequiresANumber has `-t` refuse an operand that is not a
	// number, at status 2 with the preset's integer wording. Answering No is a
	// silent false at 1.
	//
	// Asked only for such an operand: a numeric descriptor is answered false the
	// same way wherever a terminal is absent.
	TerminalTestRequiresANumber Answer

	// BareTerminalTestIsDescriptorOne reads a lone `-t` — `[ -t ]` and
	// `test -t`, where the one-argument rule would make it a non-empty string
	// and so unconditionally true — as `-t 1` instead. Answering No keeps the
	// string rule.
	//
	// Measured with `[ -t ] >/dev/null`, which pins the answer to a descriptor
	// that is certainly not a terminal rather than to whatever the run was
	// handed: 0 under No and 1 under Yes. The same line on a pseudo-terminal
	// with no redirection is 0 under both, which is what says Yes is answering
	// about descriptor 1 and not refusing the word.
	//
	// `-t` alone among the unary operators: `[ -f ]`, `[ -n ]`, `[ -z ]` and
	// `[ -e ]` are true under both, so this is not a general "an operator with
	// no operand is an operator" rule and is not written as one. The negated
	// two-word form follows it — `[ ! -t ] >/dev/null` splits the same way —
	// because that is the same one-argument rule with a `!` in front.
	//
	// Nothing is asked for `[[ -t ]]`: a grammar with the construct refuses it
	// as a syntax error.
	BareTerminalTestIsDescriptorOne Answer

	// ReadRequiresAVariableName refuses a bare `read` as an argument-count error
	// at 2, where answering No reads into REPLY.
	ReadRequiresAVariableName Answer

	// ReadRefusesABadNameBeforeReading judges `read`'s first operand as a name
	// before it goes to the stream, rather than after.
	//
	// Observable, and only through the input: a refusal that comes first leaves
	// the line for the next reader, and one that comes after has eaten it. With
	// `printf 'AAA\nBBB\n' | { read 1bad; cat; }`, Yes prints both lines and No
	// prints only BBB. Builds of one implementation answer it differently, so a
	// corpus case here has columns that disagree on purpose.
	//
	// It is the *first* operand and not the whole list. The names in front of a
	// bad one are assigned and then the refusal comes, under every answer:
	// `printf 'X Y Z\n' | { c=keep; read a 1bad c; }` leaves a as X and c as
	// keep, and the line is consumed, including under the answer that would not
	// have read it had `1bad` come first. So checking the whole list up front
	// would answer a=[] where every answer gives a=[X].
	ReadRefusesABadNameBeforeReading Answer

	// ReadCountJudgesTheNamesAfterTheFirst keeps judging `read`'s operands as
	// names when `-n` or `-N` gave it a count. Answering No stops at the first.
	//
	// With `printf 'XYZW\n' | { read -n 3 a 1bad; }`, Yes refuses `1bad` and
	// fills a with XYZ, and No fills a with XYZ and says nothing. Without a
	// count the two agree — `read a 1bad` is refused either way — so it is the
	// count that moves it and not the operand.
	//
	// Left unanswered by a preset that cannot reach it: with no count letter at
	// all, or with one that reads from the terminal rather than from the stream,
	// the question is never asked. An answer there would be a claim nothing
	// measured.
	ReadCountJudgesTheNamesAfterTheFirst Answer

	// ReadPromptOperand says whether `read`'s first operand may carry a prompt
	// after a `?`, and what an operand that is nothing else names.
	//
	// The form is `read "v?Name: "`, which reads into v and writes `Name: ` at a
	// terminal — `read -p` in one word, and what scripts written for the presets
	// that have it use. It is the *first* operand alone — `read v "w?p"` is a
	// bad name `w?p` — and the prompt is written for a terminal only, so a piped
	// `read "v?p"` is a plain read into v.
	//
	// It has to be answered wherever the name check is, not beside it: the word
	// being judged is the part in front of the `?`, so a check that did not know
	// the form would refuse `read "v?Name: "` wherever it is spelled that way.
	ReadPromptOperand ReadPromptOperand

	// StdinProgramReadInBlocks takes a program arriving on standard input as
	// much at a time as the descriptor will give, rather than a line at a time.
	// Whatever the block swallowed has left the descriptor, so a `read`, an
	// external command, or anything else the script points at standard input
	// finds only what had not arrived yet.
	//
	// With `printf 'read x\necho "[$x]"\nDATA\n' | sh`, true prints `[]` and
	// then runs `DATA` as a command, where false hands the second line to `read`
	// and never parses it. docs/spec/invocation.md has the grid, including the
	// case that shows what the difference really is — `exec 0< file` mid-program
	// replaces the *rest of the program* under false, and only what follows the
	// block under true.
	//
	// A bool rather than an [Answer], and deliberately: a common denominator
	// exists, and "refuse to read a piped script at all" is not an answer
	// anything could ship. False is reading by the line, which is what the
	// substrate does.
	//
	// It is the standard-input route's question alone. A script named as an
	// operand is opened separately from standard input, so nothing is shared and
	// the answers agree; `-c` reads no descriptor at all. The sibling question
	// for a command string is Diagnostics.CommandStringParsedWhole.
	StdinProgramReadInBlocks bool

	// StdinOptionNamesTheOperands lets the standard-input option name the
	// operands of an invocation that also carries a command string — `sh -sc CMD
	// name a`. The stdin option's rule is that no operand is `$0`: the shell
	// keeps its own name and every operand is a positional parameter, so `$0` is
	// the shell and `$#` is 2. The command string's rule is that the first
	// operand is `$0` and only the rest are parameters, so `$0` is `name` and
	// `$#` is 1.
	//
	// Measured with `-sc`, `-s -c` and `-c -s` alike, since order and bundling
	// change nothing.
	//
	// Asked only when both are given, which is the only place the answers part.
	// Where the program comes from is not this question: the command string is
	// run under both, and the corpus pins that separately. Either option alone
	// is unanimous too — the command string names the first operand `$0`, and
	// standard input leaves `$0` as the shell — and with no operands at all the
	// two rules agree by having nothing to name.
	//
	// It has no answer in PosixSemantics, and that is the honest zero rather
	// than an omission: the standard gives `-c` and `-s` separate synopses and
	// says the second is assumed only when the first is absent, so it never
	// describes an invocation carrying both. An even split with no standard to
	// break it is refused until a preset chooses.
	StdinOptionNamesTheOperands Answer

	// PlusSignedCommandStringIsDollarZero gives a plus-signed command string
	// `$0` for itself: `sh +c CMD name a` leaves `$0` as CMD and makes every
	// operand a positional parameter, where the minus spelling would have made
	// `name` `$0` and only `a` a parameter.
	//
	// False reads `+c` as `-c` in every respect, and the command string is *run*
	// either way — the sign changes nothing about where the program comes from,
	// which the corpus pins separately.
	//
	// A bool rather than an [Answer], for the reason StdinProgramReadInBlocks is
	// one: a common denominator exists, and refusing an invocation everything
	// runs is not an answer anything could ship. False is the majority answer
	// and the standard's own — POSIX has no plus spelling of the option at all,
	// so reading it as the option it spells invents nothing.
	//
	// Two further things a true answer has been measured doing are deliberately
	// not modeled, because they do not agree with each other and read as defects
	// of one build rather than as a rule: the operands also reaching the program
	// as literal words appended to its last command, and a command string of a
	// single word being looked up on PATH and run as a file.
	// docs/spec/invocation.md records both.
	PlusSignedCommandStringIsDollarZero bool

	// LoginProfileWhenNonInteractive has a login shell read its login profile
	// even when there is a script to run rather than a person to prompt. A shell
	// is a login shell when argv[0] begins with a dash, which is what `login`
	// and every terminal emulator's "run as a login shell" does, and the
	// question is only what that then means for a shell that is not going to
	// prompt.
	//
	// Measured with a scratch HOME, on all four of the script-operand, `-c`,
	// standard-input and `-s` routes, and the answer is the same on every one of
	// them — this is a fact about the implementation rather than about the
	// route. docs/spec/invocation.md has the grid, including the two
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
	// makes the profile read even with a script to run, so the option overrides
	// this rather than setting the same bit. See StartupFileOptions.Login.
	LoginProfileWhenNonInteractive bool

	// NonInteractiveStartupVariable names a variable whose value is expanded and
	// sourced by a shell that is *not* going to prompt. Empty means there is no
	// such file.
	//
	// A name rather than a bool, for the reason the profile's own filename is
	// not modeled as an axis: what a shell calls the thing is a per-preset fact
	// and not a disagreement about behavior. Where the name exists nothing else
	// does anything at all with it — measured on a script operand, `-c` and a
	// program on standard input alike.
	//
	// It is the exact counterpart of `$ENV`, which the front end reads for every
	// preset and reads *only* when interactive. The two never overlap: a preset
	// with this reads this and not `$ENV` when it is not interactive, and reads
	// neither at a prompt, where it has a file of its own name instead.
	//
	// Two more measured properties, both shared with the profile and both the
	// reason it is sourced where it is. The value is expanded before it is
	// opened, since `$HOME/…` is the usual spelling; and the file is run *by*
	// the shell that is about to run the script, so it sees that shell's `$0`,
	// `$#`, positional parameters and options, and an `exit 3` in it exits 3
	// with the script never run. A file that is not there is not a failure.
	//
	// **POSIX mode suppresses it**, which is measured and is why this is one
	// field rather than two. Nothing is read under the standard's own posix
	// option, and nothing when invoked as `sh` — the two spellings of the same
	// mode. So the absence in the `sh` column is the mode again rather than a
	// second fact about a second name.
	NonInteractiveStartupVariable string

	// StartupDirectoryVariable names a variable whose value replaces the home
	// directory as the place the startup files below are looked for. Empty means
	// the home directory.
	//
	// Where one exists it redirects *all* of the files rather than one of them —
	// every file under the named directory read and none of the same names under
	// `$HOME`. It is read afresh for each file rather than once, which is also
	// measured and is the reason a person's first startup file setting it works
	// at all: the file that sets it is found under the home directory and every
	// file after it under the directory it named.
	//
	// A variable name rather than a path, for the reason
	// NonInteractiveStartupVariable is one: what the shell calls the thing is
	// the preset's, and the value is the person's.
	StartupDirectoryVariable string

	// UnconditionalStartupFile names a file read on *every* invocation — login
	// or not, prompting or not, `-c` and a script alike. Empty means there is no
	// such file.
	//
	// Where one exists it is the only startup file read for a plain `sh -c cmd`.
	// Measured with a scratch home directory on all four routes.
	//
	// First of the files, before the profile: an interactive login shell reads
	// this, then the profile, then the interactive file, then the late login
	// file, in that order.
	UnconditionalStartupFile string

	// LoginStartupFiles names the profile a login shell reads, most preferred
	// first, as whitespace-separated names. **The first one that can be read is
	// the only one read**, which reduces to "the file" for a preset with one
	// name for it.
	//
	// Measured with a scratch home directory holding a marker for every name: a
	// preset naming three reads the first, falls back to the second when that is
	// absent and to the third when both are, and reads exactly one of them.
	//
	// Empty means no profile is read, which is what a Semantics nobody has
	// filled in should do — see LoginProfileWhenNonInteractive for why a default
	// must not reach into a home directory.
	//
	// *Whether* a login shell reads it when there is a script to run rather than
	// a person to prompt is the separate question LoginProfileWhenNonInteractive
	// asks; this is only which file.
	//
	// One string rather than a slice, which is how EchoOptions, ReadOptions and
	// JobsOptions already spell a list and is not only consistency: a slice
	// anywhere in this struct makes the whole vector uncomparable, and `==`
	// against another vector is something a test — and an embedder — may already
	// be doing. No startup file is named with a space in it, so nothing is lost
	// by the separator.
	LoginStartupFiles string

	// LateLoginStartupFile names a login file read *after* the interactive file
	// rather than before it. Empty where there is none.
	//
	// The position is the whole of why it is a second field: an interactive
	// login shell reads the profile, then the interactive file, then this, so a
	// person's late file sees what their interactive one did. It is read for a
	// non-interactive login shell too, wherever a profile is read there at all.
	LateLoginStartupFile string

	// InteractiveStartupFile names the file read when the shell is interactive,
	// in the startup directory. Empty means there is no file of the shell's own
	// name and `$ENV` is read instead, which is what the standard specifies.
	//
	// The two are alternatives rather than a sequence: a preset with a file of
	// its own reads that and not `$ENV`, and one without reads `$ENV` and
	// nothing of its own name.
	//
	// **POSIX mode replaces it with `$ENV`**, which is the interactive half of
	// what NonInteractiveStartupVariable records and is measured the same way:
	// invoked as `sh`, the shell reads `$ENV` at a prompt and not its own file.
	// So this is not suppressed in the mode the way the non-interactive file is
	// — the standard has a file here and the shell reads the standard's one
	// instead of its own.
	InteractiveStartupFile string

	// InteractiveStartupFileWhenLogin has an interactive *login* shell read the
	// interactive file as well as its profile.
	//
	// The one disagreement about startup ordering, and it is why the four
	// combinations of login and interactive are not four independent facts.
	// Measured through a pseudo-terminal: Yes reads the interactive file for a
	// login shell and No does not — under which a person's interactive file is
	// reached from a login shell only because their profile sources it by hand,
	// which is why so much documentation tells them to.
	//
	// Asked only where InteractiveStartupFile names something. A preset whose
	// interactive file is `$ENV` reads it in both cases — a login interactive
	// shell reads the profile and then `$ENV` — so there is nothing here to
	// answer.
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

	// VersionOption is how this shell answers an invocation option asking it to
	// name its version — `--version`. The zero value is a shell with no such
	// option.
	//
	// It is an invocation input like StartupFileOptions above, read by the front
	// end rather than by the interpreter, and it is here for the same reason:
	// the spelling belongs to the preset, and a front end with an opinion about
	// it would have to hold every preset's at once.
	VersionOption VersionOption

	// FunctionSearchVariable names the scalar this shell searches for *function
	// definition files* — the parameter an `autoload`d name is looked up on.
	// Empty means there is no such search.
	//
	// It is here rather than in the builtin that reads it because the value is
	// not the builtin's to invent. Measured under `env -i` with a scratch HOME
	// and no startup file speaking: a shell with the parameter arrives with that
	// *installation's* directories on it — its own function library, plus the
	// site directories third-party packages install into. A value in the
	// environment **replaces** the lot rather than adding to it, and does so
	// even when it is the empty string, so the default is a fallback for a name
	// the environment does not mention rather than for one it leaves blank.
	//
	// Which directories is a fact about where a shell was installed, and no
	// preset can hold one: a value written here would be the recording
	// machine's. So this names the parameter and the front end supplies the
	// value — the same split StartupDirectoryVariable above already makes, and
	// the same one `$PATH` has. See driver.
	FunctionSearchVariable string

	// ArrayLengthWithoutSubscriptIsCount makes `${#a}` of an array the number of
	// elements, rather than measuring the element the bare name yields. Asked
	// only where the two answers differ.
	ArrayLengthWithoutSubscriptIsCount Answer

	// WholeSubscriptOnAScalarMeasuresIt makes `${#s[@]}` on a name holding one
	// string the *width of that string* rather than the count of a list of one:
	//
	//	          on a=""  on b=x  on h="a b"
	//	Yes       0        1       3
	//	No        1        1       1
	//
	// The three-character row is what says which reading it is: an empty scalar
	// answering 0 alone would be "no elements", and `a b` answering 3 says the
	// whole-array subscript on a scalar reaches the *value*. A grammar with no
	// such expansion refuses it outright, which is the absence rather than a
	// third answer.
	//
	// Asked of the length alone, because the length is where *this* split falls.
	// The plain value is unanimous — `set -- "${h[@]}"` leaves one parameter
	// holding `a b` under both — so the fields ask nobody. The slice is not
	// unanimous and is not this question either: it splits a different way and
	// has an axis of its own, immediately below.
	//
	// It is how a script asks "did I get anything?" after a parse:
	// `local -a opts; zparseopts …; (( ${#opts[@]} ))` reads 1 under No for a
	// name that never became an array, which is a count agreeing with the wrong
	// answer.
	WholeSubscriptOnAScalarMeasuresIt Answer

	// WholeSubscriptOnAScalarSlicesIt makes `${s[@]:off:len}` on a name holding
	// one string a slice of *that string's characters* rather than of a list
	// whose only element is the whole value. On `h="a b"` and `h=abcdef`:
	//
	//	      ${h[@]:0:1}  ${h[*]:0:1}  ${h[@]:1}  ${h[@]:2:3}
	//	Yes   a            a            ` b`       cde
	//	No    a b          a b          (no field) (empty)
	//
	// A different split from the length above, which is why it is a second
	// question and not a second reading of the first: an implementation can
	// count a list of one for `${#h[@]}` and slice the characters here, so no
	// single answer about "what a whole subscript on a scalar reaches" fits it.
	// The offsets index the value exactly as `${h:off:len}` does — `${h:0:1}` is
	// `a` under both, which is the control that says the character reading is
	// not new — and No is the list reading.
	//
	// Asked of the slice alone, and only of a name that is set and holds one
	// string: an unset name is empty under both readings, and a real array is a
	// list under both.
	//
	// Its silence is the reason it is worth an axis rather than a default.
	// `${line[@]:0:1}` reads as the first character to anyone writing it and
	// comes back as the whole line under No, and `${h[@]:1}` — drop the first
	// character — comes back as nothing at all, both at status 0.
	WholeSubscriptOnAScalarSlicesIt Answer

	// ArrayNameWithoutSubscriptIsTheList makes an unquoted bare array name the
	// array itself — one field per element, a slice slicing the list and an
	// element-wise operator applying to each — exactly as `${a[@]}` is.
	// Answering No reads the bare name as `${a[0]}`, one field.
	//
	// The field-count half of what ArrayScalarIsTheWholeArray answers for the
	// value, and separate from it because the two can be answered differently by
	// quoting: `"$a"` is one joined field under both, so the divergence is
	// exactly the unquoted spelling in a context that splits.
	//
	// Its silence is the reason it is a P1 — `for f in $files` runs once over a
	// joined string instead of once per element, every command inside gets one
	// argument where it expected several, and the status is 0. The same join
	// makes an element-wise operator a quiet no-op: `${a:#pattern}` matches the
	// joined string, fails, and hands the whole array back looking like a filter
	// that found nothing.
	//
	// Asked only where the two readings differ, which is more than one element —
	// or exactly one under an operator that reads the list even then, a slice or
	// one of the element-selecting three.
	ArrayNameWithoutSubscriptIsTheList Answer

	// UnsetNameAtIsOneEmptyField hands a quoted `"${a[@]}"` written on a name
	// that holds nothing at all one empty field — the reading under which a name
	// that is not a declared array is a scalar, and an unset scalar under quotes
	// is one empty field, the same field `"$a"` gives.
	//
	// It is a question about **existence** and not about emptiness. An array
	// that exists and has no elements is no field under every answer, so that
	// half is core and asks nothing:
	//
	//	f() { printf '%s\n' "$#"; }
	//
	//	      unset a   declared, no elements   one element
	//	No    0         0                       1
	//	Yes   1         0                       1
	//
	// Measured from files, with a *function* rather than `set --` so the
	// positional-parameter builtin is not a confound, and with each preset's own
	// way of declaring an empty array.
	//
	// That last clause is the whole reason this axis is spelled this way. An
	// earlier reading had it as a question about an *empty* array, on the
	// strength of `a=(); set -- "${a[@]}"; echo "n=$#"` answering `n=1`
	// somewhere. That implementation does not read `a=()` as an array literal at
	// all: it makes a *compound variable* whose value is the two-line text
	// `(\n)`, so the one field that row counted held those three bytes rather
	// than nothing. A count-only snippet cannot tell one empty field from one
	// field holding `(`, newline, `)`, and the corpus row recorded the agreement
	// of a coincidence. Asked with that preset's real empty-array spelling it
	// gives no field — and that spelling on an existing array leaves the name
	// *unset*, so it has no declared-and-empty state to ask about.
	//
	// The column that really splits the two does it in the opposite direction
	// from the one that story predicted: an empty literal is a set, empty array
	// and no field, while a name nothing declared is one field.
	UnsetNameAtIsOneEmptyField Answer

	// SubstringNegativeLengthIsEmpty answers `${x:1:-2}` with nothing at all,
	// rather than counting the negative length from the end.
	SubstringNegativeLengthIsEmpty Answer

	// SubstringRangeReadsModifiers makes `${x:h}` a *modifier* rather than an
	// arithmetic offset, where the range is also a history-modifier syntax.
	// Answering No reads it as the expression it looks like everywhere else.
	//
	// The two spellings share every byte of their punctuation, so the reading is
	// decided before either is evaluated, and it is decided by the first byte: a
	// range segment that begins with an unquoted letter is a modifier.
	// `${x:_q:2}`, `${x: i:2}`, `${x:(i):2}`, `${x:$i:2}` and `${x:"h"}` are all
	// substrings under Yes for that reason.
	//
	// Asked only where a segment does begin with one, so `${x:1:2}` needs no
	// answer from anyone.
	SubstringRangeReadsModifiers Answer

	// ReplacementOperandTakesTheEnclosingQuoting reads the replacement half of
	// `${v/pat/repl}` as *content* of the quoting around the expansion rather
	// than as a word of its own.
	//
	// Under Yes, `"${s/a/'$v'}"` on `s=xay` and `v=VAL` is `x'VAL'y` — the
	// quotes are two characters of the result and what stands between them is
	// still substituted. Under No the quotes quote and are removed, giving
	// `x$vy`.
	//
	// The third of three readings a quote in a `${ }` operand can take, and the
	// only one that is disagreed about. A *word* operand takes the enclosing
	// quoting unanimously — `"${u:-'$v'}"` is `'VAL'` throughout — and a
	// *pattern* operand's quotes quote, also unanimously. So neither of those is
	// an axis, and the replacement cannot borrow either one's answer.
	//
	// Builds of one implementation moved between the two, which is what says a
	// field named for a shell could not carry it.
	//
	// Asked only at the disagreement: the two readings coincide unless the
	// expansion is double-quoted *and* the operand holds one of the three
	// characters they part on — a single quote, a backslash, or a tilde at the
	// front. Unquoted, every preset with the operator agrees with the word
	// reading, which is what says the disagreement belongs to the enclosing
	// context and not to the operator. The parser decides whether it can arise
	// at all and keeps both readings when it can; see
	// syntax.ParamExpr.Arg2Enclosed.
	ReplacementOperandTakesTheEnclosingQuoting Answer

	// LinenoCountsFromTheFunction numbers `$LINENO` inside a function from the
	// line the function was written on, rather than from the file.
	//
	// Inside means the line is one the body holds. A file the function sourced
	// counts from its own top, because the line is the file's — the same
	// innermost-frame rule Diagnostics.LocationNamesTheFunction is read by, and
	// measured the same way.
	LinenoCountsFromTheFunction Answer

	// ArithBaseAbove36 admits `37#…` through `64#…`, whose letters split into
	// cases and whose last two digits are `@` and `_`. Answering No stops at 36
	// and says so.
	ArithBaseAbove36 Answer
	// ArithBaseMayHaveALeadingZero lets `010#5` name base ten. The base is read
	// in decimal either way; what this decides is whether a zero in front of it
	// is padding or the start of an octal constant.
	//
	// Measured with the digit chosen so the two readings could not agree —
	// `010#5` is five under both base eight and base ten, which is the probe
	// that cannot discriminate:
	//
	//	          010#5   010#9   010#11   08#7   0010#5
	//	Yes       5       9       11       7      5
	//	No        refuse  refuse  refuse   refuse refuse
	//
	// Yes reads the base in plain decimal, padding and all. One No refuses every
	// row for a single reason that is not about bases at all: a leading zero
	// makes the text an octal constant, so the `#` is never a base marker and
	// the literal fails as a number. Another says yes to the zero and no to the
	// length, which is ArithBaseIsAtMostTwoDigits beside this one.
	//
	// A grammar with no `base#digits` at all gives the same refusal it gives
	// `10#5`.
	ArithBaseMayHaveALeadingZero Answer
	// ArithBaseIsAtMostTwoDigits stops the base after two characters, which is
	// as many as a base up to 64 needs.
	//
	// The reading that survived three hypotheses — an octal base, a decimal
	// base, and a two-character cap:
	//
	//	02#11    3        base two, so the cap took `02`
	//	0002#11  refuse   four characters, so the cap took `00`
	//	64#10    64       two characters is enough for the widest base
	//	012#11   refuse   the cap took `01`, which is no base
	//	020#11   refuse   the cap took `02` and left `0#11`
	//
	// A decimal reading answers 13 and 21 to the last two and an octal one 11
	// and 17; the cap explains all five and neither of the others does.
	ArithBaseIsAtMostTwoDigits Answer
	// ArithBaseZeroReadsTheDigitsAsWritten answers `0#5` with 5 rather than
	// refusing a base of zero: the digits are read as an ordinary constant,
	// prefix and all, so `$(( 0#0x10 ))` is 16.
	//
	// Not the same question as a base below two: `$(( 1#0 ))` is an invalid base
	// under Yes while `$(( 0#5 ))` is 5 — so zero is a base read through rather
	// than one refused. A preset where a leading zero has already made the text
	// an octal constant never sees a base in either.
	ArithBaseZeroReadsTheDigitsAsWritten Answer
	// ArithEmptyRadixDigitsAreZero reads `0x` — a radix prefix with no digits
	// after it — as a complete number worth zero, rather than refusing it.
	//
	// The control is `$(( 0x+1 ))`, which is 1 under Yes: the prefix is a
	// *finished* number and the `+1` goes on from it, rather than the `+` being
	// swallowed by a digit scan that found nothing.
	//
	// Only a radix prefix. `$(( 8# ))` — a named base with no digits — is a
	// separate row, answered differently again, and is not this axis.
	ArithEmptyRadixDigitsAreZero Answer
	// ArithOverflowSaturates clamps integer overflow at the edge, holding max+1
	// at the maximum rather than wrapping. Asked only when an overflow actually
	// happened.
	ArithOverflowSaturates Answer
	// EmptyArithExpressionIsAnError refuses `$(( ))`, wanting a primary and
	// stopping the script, rather than answering zero.
	EmptyArithExpressionIsAnError Answer
	// TildePlusMinusExpands turns `~+` into $PWD and `~-` into $OLDPWD, only
	// while the variable is set — a fresh shell's `~-` stays literal. Answering
	// No keeps both as written.
	//
	// An implementation may still answer `~-` after `unset OLDPWD`, from
	// directory state of its own this runner does not keep — recorded, not
	// reproduced.
	TildePlusMinusExpands Answer

	// SetHasTraceLetters gives `set` the -E and -T letters, which carry the ERR
	// trap (and DEBUG with RETURN) into functions and subshells the preset
	// otherwise bounds them out of.
	//
	// Answering No either refuses the letters or spells different options with
	// them, so only a refusal is honest there. Recorded as
	// `opt/set-e-carries-the-err-trap`.
	SetHasTraceLetters Answer

	// SetHasTheTLetter gives `set` the -t letter: the shell reads and runs one
	// more line and then stops — the option some presets list as `onecmd` and
	// others spell with the letter alone.
	//
	// Answering No either has the letter and refuses to move it, the way a
	// fixed option is refused, or has never heard of it and calls it illegal.
	// Recorded as `opt/set-t-stops-after-one-command`.
	SetHasTheTLetter Answer

	// ImmovableOptionsSetAtInvocation lets the command line that started the
	// shell move an option a *running script* may not — a route split inside one
	// implementation rather than a disagreement between two, which is why it is
	// asked where the route is known instead of where the option is.
	//
	// Under Yes, `-t plain.sh` runs the first line of a three-line script and
	// stops, and the long spellings of the same option do the same, while
	// `set -t` and the option's own set/unset words inside that script are all
	// `can't change option` at 1 and fatal. So the names such a preset calls
	// fixed are not states it cannot reach; they are states a script may not
	// change.
	//
	// It governs the refusal and not the applying: a preset that answers Yes
	// still has to say what each such name *would* move, which for the letter is
	// the substrate's `onecmd` and for a name is the preset's own table. A name
	// with nothing to apply is refused at the invocation exactly as it is
	// refused in a script.
	ImmovableOptionsSetAtInvocation Answer

	// OneCommandStopsACommandString extends `set -t` to `-c`, and it is the one
	// route the presets that have the option disagree about.
	//
	// With a two-line command string that sets it and then echoes, No writes the
	// echo — the option is on and `$-` says so, and the rest of the string is
	// read anyway — where Yes writes nothing. Both stop a script file and both
	// stop standard input, so the question is this route and no other. Asked
	// only where the option is on; see Runner.OneCommand.
	OneCommandStopsACommandString Answer

	// SetHasTheHLetter gives `set` the -h letter at all. Which option it
	// abbreviates is SetHLetterTracksCommands; answering No refuses the letter
	// outright, fatally, the way any letter that does not exist is refused.
	SetHasTheHLetter Answer

	// SetHLetterTracksCommands makes `set -h` the short spelling of command
	// tracking — permission to remember where commands were found, listed as
	// `hashall` or `trackall` depending on the preset. Answering No abbreviates
	// a history option with the letter instead and leaves command hashing alone.
	//
	// Asked only where the letter is written, like SetFTurnsOffGlobbing: the
	// long names raise no question.
	SetHLetterTracksCommands Answer

	// MonitorNeedsATerminal ties turning `set -m` on to having a terminal.
	//
	// Measured in shells run with none, which is what a script has: answering No
	// grants the option silently; answering Yes either remarks that no terminal
	// could be reached and reports success with the option left off, or refuses
	// at 1, fatally. Those two refusal shapes are the preset's own wording and
	// status — Diagnostics.MonitorDenied and MonitorDeniedStatus.
	//
	// A runner whose front end gave it a person to report jobs to (JobControl)
	// has a terminal, so the question is asked only without one. Turning the
	// option *off* is granted under every answer.
	MonitorNeedsATerminal Answer

	// InteractiveMonitorNeedsATerminal ties the monitor an *interactive* shell
	// turns on for itself to having a terminal, which is a different question
	// from the one above: that one is a script asking with `set -m`, and this
	// one is nobody asking at all.
	//
	// The rule the answer qualifies is unanimous and is not an axis. On
	// `-i script.sh` with a pseudo-terminal, every preset reports `monitor on`
	// and puts `m` in `$-`. So an interactive shell runs the monitor, and a
	// front end that leaves it off is wrong on every route rather than under one
	// preset.
	//
	// What splits is the same invocation with no terminal anywhere: Yes reports
	// it off and leaves `m` out, No reports it on regardless.
	//
	// The terminal that counts is a terminal on any of the three standard
	// streams, and that is measured rather than assumed. A controlling terminal
	// with all three redirected elsewhere is *not* enough, and a pseudo-terminal
	// on any one of the three alone is enough. So the question the front end has
	// to answer is about the descriptors it was handed, which is the one it can
	// answer.
	//
	// It is not MonitorNeedsATerminal read a second time: one implementation
	// turns the monitor on for `-c 'set -m'` with no terminal and leaves it off
	// for `-i script.sh` with no terminal. One shell, two answers, so an
	// explicit request and an automatic one are two questions.
	//
	// The preset says a terminal *is* needed, and this is the rarer case where
	// the text does not decide. XCU says of `-m` that it "shall be enabled by
	// default for interactive shells" and puts no terminal in that sentence, but
	// it also defines job control throughout in terms of a controlling terminal,
	// so the sentence is silent about having none rather than permissive about
	// it. Silent text gets the answer that claims less — a shell with no
	// terminal does not report a monitor.
	//
	// Read rather than `ask`ed, exactly as InteractiveOptionLetters is: the
	// answer is wanted once at startup, before the program has run a line, so
	// refusing over an unanswered field would put "this is disagreed about"
	// ahead of every `-i script.sh` under a preset that has not chosen. An
	// unanswered field reads as Yes — a terminal is needed and the monitor stays
	// off, which is the quiet answer.
	//
	// One further state is worth knowing about and is not this axis: an
	// implementation may put `m` in `$-` and announce its jobs while its own
	// `set -o` still lists `monitor off` — disagreeing with itself. What is
	// recorded here is the state the other two readers report.
	InteractiveMonitorNeedsATerminal Answer

	// InteractiveScriptAnnouncesJobs gives an interactive shell running a
	// *named script file* somebody to tell about its jobs: the job number and
	// pid as one starts, and the `Done` row as one ends.
	//
	// A different question from AnnouncesBackgroundJob, which asks whether the
	// *start* is announced at all. Both are read on this route, and they cannot
	// be one field: an implementation may announce the end of a job here and
	// never the beginning.
	//
	// Measured through a pseudo-terminal, scratch HOME and scratch HISTFILE, on
	// `sh -i script.sh` running `sleep 0.3 &` between two echoes.
	//
	// It is not the monitor asked a second time. The monitor is unanimous on
	// this route with a terminal — InteractiveMonitorNeedsATerminal records that
	// — and this is not, so a front end that turned both on together would give
	// a quiet preset an announcement it does not make.
	//
	// It is however *gated* on the monitor, which is measured: with no terminal
	// anywhere, a preset that leaves the monitor off says nothing about the job
	// either, and one that runs a monitor without a terminal announces both
	// ends. So the notice rides on the monitor, and this axis is what the preset
	// that runs a monitor and stays quiet anyway is for.
	//
	// And the quiet answer is not about where the commands come from, which is
	// the reading the grid rules out: the same interactive shell with the
	// program on a *pipe* announces both, and so does `-i -c`. The silence falls
	// on exactly one interactive route, the one whose program is a named file —
	// which is why this axis names the route rather than the terminal.
	//
	// The preset says no. XCU has nothing to say about a notice on this route,
	// and where the text is silent the preset takes the answer that claims less:
	// a shell that has not been asked for a job report does not write one. It is
	// also the intersection, and the core is the intersection rather than the
	// majority.
	//
	// Read rather than `ask`ed, exactly as InteractiveMonitorNeedsATerminal is
	// and for the same reason: the answer is wanted once at startup, so an
	// unanswered field would put a disagreement ahead of every `-i script.sh`
	// under a preset that has not chosen — including scripts that never mention
	// a job.
	//
	// `-i -c` is a separate question and is deliberately not this one. That
	// route splits differently and is therefore a different axis;
	// docs/spec/invocation.md has the grid.
	InteractiveScriptAnnouncesJobs Answer

	// PunctuatedFunctionNameIsRefused stops the script when a function whose
	// name carries `-` or `.` is defined. Answering No defines and runs it; a
	// grammar that never parses the definition at all does not reach the
	// question.
	PunctuatedFunctionNameIsRefused Answer

	// DirectoryOnPathIsACandidate keeps a directory the PATH search found as the
	// failed candidate when no later entry runs, so the report names the
	// directory rather than saying the command was never found.
	//
	// The search continues past the directory under every answer — that is
	// unanimous, and is what makes a shim directory early on PATH work at all.
	// The answers part only when nothing later matches: No reports the name as
	// not found at all, Yes reports the directory it could not run. What status
	// that carries is DirectoryOnPathStatus's question.
	DirectoryOnPathIsACandidate Answer

	// ExecTakesOptions lets `exec` read options of its own, such as `-a name` to
	// choose the argv[0] the command sees. Answering No makes a leading `-a` the
	// name of a command, reported as not found.
	//
	// The answer has to come before the command is looked up, because it decides
	// which word the command is.
	ExecTakesOptions Answer

	// DotFallsBackToCurrentDirectory looks in the current directory for a `.`
	// operand with no slash in it, after PATH has missed.
	//
	// PATH is searched first under every answer, and wins over an identically
	// named file in the current directory; this is only about what happens when
	// PATH does not have it.
	DotFallsBackToCurrentDirectory Answer

	// TestAcceptsDoubleEqual makes `==` a synonym for `=` in `test` and `[`, so
	// `test a == a` is a string comparison.
	//
	// No does not mean "compares unequal": it means the word is not an operator
	// at all, so `test a == b` is three words with no operator among them and is
	// reported as one. The answer therefore has to come before the comparison,
	// not after it.
	//
	// This is only about `test` and `[`. Inside `[[ ]]` the same spelling is a
	// pattern match, which is a different question entirely.
	TestAcceptsDoubleEqual Answer

	// SignalDeathStatusIsTwoFiftySix encodes a command killed by a signal as
	// 256 + the signal rather than 128 + the signal, so KILL is 265 and TERM 271
	// instead of 137 and 143.
	//
	// Measured across eight signals; it is not a special case for any one of
	// them. POSIX requires only "greater than 128", which decides nothing, so
	// the preset follows the more common encoding.
	SignalDeathStatusIsTwoFiftySix Answer

	// PipefailOption is whether `set -o pipefail` exists, making a pipeline
	// report its last failing element rather than its last element. Absent from
	// POSIX, where a pipeline is defined to report its last command and nothing
	// offers to change it.
	//
	// Not a wording difference: where it is absent the name is not an option at
	// all, so `set -o pipefail` fails and the pipeline goes on reporting its
	// last element — which is the answer a script guarding against a failure
	// upstream is specifically trying not to get.
	PipefailOption Answer

	// ErrexitSeesPipefailFailure lets `set -e` stop for a failure that only
	// pipefail produced — a pipeline whose last element succeeded and whose
	// earlier one did not. Answering No runs on.
	//
	// Absent rather than false where there is no pipefail, so the question
	// cannot arise and is never asked.
	//
	// Narrower than it looks: an ordinary failing pipeline — `true | false` —
	// stops under both answers, and this is only about the failure the option
	// adds.
	ErrexitSeesPipefailFailure Answer

	// PipefailSubstitutesTheBareSignal reports an element pipefail chose over
	// the pipeline's last one, and which died of a signal, as the signal's
	// *number* rather than as the status a command killed by that signal
	// reports.
	//
	// Not the same question as SignalDeathStatusIsTwoFiftySix. That axis is
	// about every status a signal death produces, and an implementation answers
	// it consistently everywhere — a foreground command, a subshell, a command
	// substitution, a `wait`, the shell dying by its own hand as its parent sees
	// it, and the *last* element of a pipeline. This is the one place the
	// convention stops. With `pipefail` set:
	//
	//	kill-me-with-TERM | cat      Yes  15    No  143
	//	kill-me-with-PIPE | head -1   Yes  13    No  141
	//	( exit 42 )       | cat       Yes  42    No   42
	//	cat </dev/null | kill-me      Yes 271    No  143
	//
	// The last row is why this is about the *substitution* and not about the
	// pipeline: an element that fails in the position the pipeline reports
	// anyway keeps the ordinary encoding, and only the status pipefail went
	// looking for is bare. An ordinary non-zero exit is unchanged either way, so
	// a signal is the whole of the difference.
	//
	// Measured builtin and external, first and middle, in pipelines of two and
	// of three, with SIGPIPE and SIGTERM. Absent rather than false where there
	// is no pipefail, and asked only where a substitution actually happened and
	// actually was a signal death.
	PipefailSubstitutesTheBareSignal Answer

	// PrintfAssignsWithV makes `printf -v name fmt args` put the formatted text
	// in a variable and print nothing. Answering No has no such option and
	// rejects it as an unknown one.
	//
	// It is how a script formats a value without a command substitution, so
	// without it the text goes to stdout and the variable stays empty — two
	// wrongs at once, and both silent.
	PrintfAssignsWithV Answer

	// PrintfRejectsUnknownOption treats any leading word starting with `-` as an
	// option and refuses one it does not know, so even `printf "-%s\n" x` is an
	// error because the format itself begins with a dash.
	//
	// Answering No recognizes the options it has and takes anything else as the
	// format — so `printf -q x` prints `-q` there and is an error under Yes.
	PrintfRejectsUnknownOption Answer

	// TrapParsesOptions reads a leading `-` word as an option rather than as the
	// action to run.
	//
	// Answering No makes `trap -p` set a trap whose action is the word `-p`, and
	// the failure surfaces later, when it fires.
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
	DeclarationNameOperands NameOperands

	// UnsetNameOperands is that question for `unset`, and is a separate field
	// because two dialects answer it differently from the declarations. zsh
	// answers the two with *disjoint* sets — `export ?` is fine there and
	// `unset ?` is not, while `unset 12` is fine and `export 12` is not — and
	// bash 5.3 checks a name for `export` and nothing at all for `unset`. One
	// field could not say either.
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

	// EmptyAssociativeKeyIsReportedWhenRead reports a *read* whose key came
	// out empty — `typeset -A m; w=; ${m[$w]}` — and answers with the empty
	// string anyway.
	//
	// The other face of EmptyAssociativeKeyIsAnError, and a different
	// question rather than the same one seen from the read side: the store
	// refuses and the read does not, the subject is the name alone rather
	// than the subscript as written, and the status stays 0.
	//
	// Measured 2026-09-12, `-c`, with `typeset -A m; m[k]=v` and `w=`:
	//
	//	probe             bash 5.3.15                      ksh93u+   zsh 5.9.2
	//	${m[$w]}          `m: bad array subscript`, ``, 0  ``, 0     ``, 0
	//	${m[""]}          the same sentence                ``, 0     a two-character key
	//	${m[$w]-none}     the same sentence, then `none`   `none`    `none`
	//	${m[ ]}           nothing at all                   ``, 0     ``, 0
	//
	// So one column says the subscript is bad and two say nothing, and all
	// three answer the same empty string at the same status — which is why
	// this is a report and not a value. bash 3.2 has no such attribute to
	// ask about, and dash reaches no subscript in an expansion at all.
	//
	// The blank row is the control that says this is emptiness and not
	// whitespace: `${m[ ]}` looks up a one-space key and finds nothing,
	// silently, in every column. `${m[]}` — nothing between the brackets as
	// written — is refused one construct earlier by
	// EmptyParamSubscriptIsAnError and never reaches this.
	//
	// It is the **key** and not the subscript, so an indexed name asks
	// nothing here: `a=(1 2); ${a[$w]}` is the first element and silent
	// everywhere, and the refusal an indexed name can earn from the same
	// emptiness is EmptySubscriptTextIsAMathError, in another column again.
	//
	// Reported once per read, and the expansion carries on: measured,
	// `"[${m[$w]}]${m[$w]}"` writes the sentence twice and prints `[]`, and
	// a word that goes on to a conditional still gets its empty value. So
	// nothing here sets the failed-expansion flag.
	//
	// The wording is Diagnostics.EmptyAssociativeKeyRead, whose one verb is
	// the name (#1972).
	EmptyAssociativeKeyIsReportedWhenRead Answer

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

	// ArithWholeArraySubscriptIsReportedAsBad reports a `*` or `@` subscript
	// inside an expression and answers **zero** for it, where the dialect
	// does not read it as the slice — so the expression survives instead of
	// being abandoned.
	//
	// Measured 2026-09-11 and again 2026-09-12, `-c`, on an *indexed* name:
	//
	//	probe                       bash 5.3.15 / as-sh / 3.2      ksh93u+
	//	a=(3 4 5); $(( a[*] ))      `a[*]: bad array subscript`, 0, st 0   `*: arithmetic syntax error`, st 1
	//	a=(3); $(( a[*] + 1 ))      the same sentence, then 1              the same refusal
	//	a=(3); $(( a[@] + 1 ))      `a[@]: …`, then 1                      the same refusal
	//	$(( nodecl[*] ))            `nodecl[*]: …`, then 0                 the same refusal
	//	s=7; $(( s[*] ))            `s[*]: …`, then 0                      the same refusal
	//	(( a[*] = 5 ))              the sentence, nothing written, st 0    the same refusal
	//
	// So the two columns that agree about the *value* disagree about the
	// report and about whether the expression survives, which is why this is
	// an axis of its own rather than a wording. The refusing answer is the
	// ordinary one: the brackets hold a text that is no expression and the
	// arithmetic says so, which is what happens with no answer here at all.
	//
	// It is the **indexed** reading alone. An association reads the brackets
	// as the key `*`, finds nothing under it and answers zero silently in
	// both columns, so the table is consulted first and this is never asked
	// there — the same ordering ArithWholeArraySubscriptIsTheSlice keeps, and
	// asked one step behind it: a dialect that reads the slice never reaches
	// this at all.
	//
	// **The spelling has to be exact.** `$(( a[ * ] ))` is an arithmetic
	// syntax error in every column measured, bash and zsh alike, so the
	// brackets are the whole-array spelling only when they hold the one
	// character and nothing else. A blank subscript is a question of its own
	// — BlankArithSubscriptIsTheEmptyExpression — and trimming here answered
	// it wrongly for both.
	//
	// Reported once per evaluation, and the write reports too: `(( a[*]++ ))`
	// writes the sentence twice in the column that reports, once for the read
	// and once for the store, and leaves every element as it was.
	//
	// The wording is Diagnostics.ArithWholeArraySubscript, whose two verbs are
	// the name and the subscript (#1978).
	ArithWholeArraySubscriptIsReportedAsBad Answer

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
