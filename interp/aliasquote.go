// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"strings"
)

// How a shell spells a value it lists back — an alias's replacement, a
// trap's action.
//
// Four engines, and no two agree. The listing is meant to be text the shell
// could read again, so each one quotes whatever its own parser would need —
// which is why this is a policy rather than one function with flags.
//
// The style is shared; which style a builtin uses is not. zsh spells an alias
// holding a tab as `$'a\tb'` and a trap holding one as `'a<tab>b'`, so the
// two are separate fields over the same vocabulary.
//
//	value        bash              dash                ksh93          zsh
//	ls           'ls'              'ls'                ls             ls
//	echo x       'echo x'          'echo x'            'echo x'       'echo x'
//	it's         'it'\''s'         'it'"'"'s'          $'it\'s'       'it'\''s'
//	a<tab>b      'a<tab>b'         'a<tab>b'           $'a\tb'        $'a\tb'
//
// Two of them quote always and two only when the value needs it; two escape an
// embedded quote by closing and reopening with a backslash and one by closing
// and reopening with a double-quoted quote; and two reach for `$'...'` when the
// value holds a control character.

// ListingQuotingStyle is how a dialect spells a value it lists back.
type ListingQuotingStyle int

const (
	// ListingQuotingUnspecified is no answer, and is refused like any other.
	ListingQuotingUnspecified ListingQuotingStyle = iota
	// ListingQuoteAlwaysEscaped always single-quotes and writes an embedded
	// quote as `'\''`: bash.
	ListingQuoteAlwaysEscaped
	// ListingQuoteAlwaysDoubled always single-quotes and writes an embedded
	// quote as `'"'"'`: dash.
	ListingQuoteAlwaysDoubled
	// ListingQuoteWhenNeededDollar leaves a plain value bare and reaches for
	// `$'...'` where a quote or a control character appears: ksh93.
	ListingQuoteWhenNeededDollar
	// ListingQuoteWhenNeededEscaped leaves a plain value bare, wraps
	// everything else in one pair of single quotes with each embedded quote
	// written `'\''`, and reaches for `$'...'` only for a control character:
	// bash's bare `set`.
	ListingQuoteWhenNeededEscaped
	// ListingQuoteWhenNeededRuns is the same decision spelled the other way:
	// the value is cut at each quote, every non-empty run gets its own pair
	// of single quotes, and each quote is written outside them with a
	// backslash in front. zsh.
	//
	// The two part only where a quote stands at an *end* of the value, which
	// is why one style stood for both until it was measured there. With a
	// variable holding `x'`, a bare `set` writes six characters in bash —
	// the run quoted, the escaped quote, then an empty pair — and four in
	// zsh, which writes no empty pair. Holding `'x` the empty pair is at the
	// front instead, and bash writes it and zsh does not.
	//
	// Both spellings read back as the value, so nothing downstream notices —
	// which is what let a spelling belonging to neither shell survive here.
	// We wrapped the whole value like bash and then dropped the trailing
	// empty pair like zsh, so a value of one quote came out with a leading
	// empty pair that neither shell writes (#2299). Measured 2026-09-12 on
	// bash 5.3.15 and zsh 5.9.2, over `set`, `alias` and `typeset -p`.
	ListingQuoteWhenNeededRuns
	// ListingQuoteAlwaysDouble always double-quotes, escaping an embedded
	// backslash, backquote, dollar or double quote, and replaces the double
	// quotes with `$'...'` when the value holds a control character: bash's
	// `declare -p`, which is not the single-quoting style its `alias` and
	// `trap` use.
	ListingQuoteAlwaysDouble
	// ListingQuoteWhenNeededPlain leaves a plain value bare and single-quotes
	// everything else, reaching for `$'...'` never: zsh's `trap`.
	//
	// zsh arrives at that output a different way — it parses the action when
	// the trap is set, refuses one it cannot parse, and lists the parse back
	// rather than the text it was given, so `a<tab>b` lists as `a b`. For an
	// action that is an ordinary command the two agree, which is every
	// action in the corpus. Where they do not, this is the closer of the two
	// answers available, and re-printing the parse is its own question.
	ListingQuoteWhenNeededPlain
)

func (a ListingQuotingStyle) String() string {
	switch a {
	case ListingQuoteAlwaysEscaped:
		return "ListingQuoteAlwaysEscaped"
	case ListingQuoteAlwaysDoubled:
		return "ListingQuoteAlwaysDoubled"
	case ListingQuoteWhenNeededDollar:
		return "ListingQuoteWhenNeededDollar"
	case ListingQuoteWhenNeededEscaped:
		return "ListingQuoteWhenNeededEscaped"
	case ListingQuoteWhenNeededRuns:
		return "ListingQuoteWhenNeededRuns"
	case ListingQuoteAlwaysDouble:
		return "ListingQuoteAlwaysDouble"
	case ListingQuoteWhenNeededPlain:
		return "ListingQuoteWhenNeededPlain"
	}
	return "ListingQuotingUnspecified"
}

// ListedValuePlace says where a listed value stands, because the dialect that
// writes a bare `name=` head writes that head's `=` differently inside a
// parenthesized body than outside one. Every other question a listing asks is
// about the value; this one is about the position, so it is a parameter rather
// than a field of the value.
type ListedValuePlace int

const (
	// ListedValueAlone is a value with nothing around it that could read it
	// as an assignment: a scalar's own `=`, a key inside brackets, the value
	// after a subscripted element's `]=`.
	ListedValueAlone ListedValuePlace = iota

	// ListedValueInAList is a value written as a **bare word inside
	// parentheses** — an index array literal's element, a nested array's
	// element, and the value half of a compound body's member. There a bare
	// `name=` head would re-read as an element of its own, so the dialect
	// that writes it bare backslashes the `=`.
	ListedValueInAList
)

func (p ListedValuePlace) String() string {
	if p == ListedValueInAList {
		return "ListedValueInAList"
	}
	return "ListedValueAlone"
}

// quoteListedValue spells a value the way this dialect lists it back, in the
// style the calling builtin uses. What is being listed is named so the
// refusal can say which question went unanswered, and where it stands decides
// the one thing the style does not — see ListedValuePlace.
func (r *Runner) quoteListedValue(style ListingQuotingStyle, what, v string, place ListedValuePlace) string {
	// A value that opens with `name=` is written with that much bare and the
	// rest quoted on its own, in the one dialect that does it. Ahead of the
	// styles rather than inside one, because the split is about the *value*
	// and the tail then takes whichever style the caller asked for — measured
	// on ksh93u+, `a=b` lists as `a=b`, `a=b c` as `a='b c'`, and a tail with
	// a tab in it as `a=$'b\tc'`, which is the same three answers the style
	// gives a whole value. See Semantics.ListedAssignmentPrefixIsBare.
	if head, tail, split := r.listedAssignmentHead(v, place); split {
		if tail == "" {
			return head
		}
		return head + r.quoteListedValueBody(style, what, tail)
	}
	return r.quoteListedValueBody(style, what, v)
}

// listedAssignmentHead splits a listed value after a leading `name=`, where the
// dialect writes that much without quotes, and reports whether it did.
//
// Once, at the front, and never again on what is left: measured, ksh93u+
// writes `a=b=c` as `a='b=c'` and not as `a=b=c`, so the second `=` is inside
// the quoted tail rather than starting another bare head.
//
// Two shapes it does not fire on, both measured: a value beginning with `=`,
// which has no name in front of it — `=x` lists as `'=x'` — and one whose
// text before the first `=` is not a name, so `1=2` lists as `'1=2'` and
// `a.b=c` as `'a.b=c'`.
//
// **A doubled `=` is measured and not reproduced.** ksh93u+ writes `a==b`
// bare and `a==` as `a==”`, where this gives `a='=b'` and `a='='`. Both read
// back as the value, which is what the listing is for; the rule that produces
// the shell's own spelling there is not one three probes could state, and the
// shapes it covers are keys and values no script writes.
func (r *Runner) listedAssignmentHead(v string, place ListedValuePlace) (head, tail string, split bool) {
	if r.sem().ListedAssignmentPrefixIsBare != Yes {
		return "", "", false
	}
	eq := strings.IndexByte(v, '=')
	if eq <= 0 || !isNameLike(v[:eq]) {
		return "", "", false
	}
	sep := "="
	if place == ListedValueInAList {
		// Inside parentheses the head's `=` carries a backslash, and only
		// there. Measured 2026-09-20 on ksh93u+ 2012-08-01, a script file
		// under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on
		// /dev/null, read through `sed -n l`:
		//
		//	typeset -a c=(a=1 b=2)       typeset -a c=(a\=1 b\=2)
		//	typeset -a g=("a=1 b")       typeset -a g=(a\='1 b')
		//	n1[0]=('a=1' x)              typeset -a n1=((a\=1 x) )
		//	typeset -C co=(q='a=1')      typeset -C co=(q=a\=1)
		//	typeset -a y=('a=1'); set    y=(a\=1)
		//
		// and the positions that do **not** take it, which are what says
		// this is the bare-word position rather than the `=`:
		//
		//	v=a=1                        v=a=1
		//	export v9='a=1'; export -p   export v9=a=1
		//	typeset -A m=([k]='a=1')     typeset -A m=([k]=a=1)
		//	typeset -A t=(['j=2']=v)     typeset -A t=([j=2]=v)
		//	e=(x y z); unset 'e[1]'      typeset -a e=([0]=x [2]=z [5]=a=1)
		//	  e[5]='a=1'
		//
		// The rule has a reason and the reason is the position: an index
		// array's elements are written as bare words, so an unescaped `a=1`
		// among them re-reads as the keyed element `[a]=1` — or, under the
		// compound reading, as a body. A key inside brackets and a value
		// after `]=` are already unambiguous and take no backslash (#3863).
		sep = `\=`
	}
	return v[:eq] + sep, v[eq+1:], true
}

// quoteListedValueBody is the style's own answer, with no assignment head
// taken off the front.
func (r *Runner) quoteListedValueBody(style ListingQuotingStyle, what, v string) string {
	switch style {
	case ListingQuoteAlwaysEscaped:
		return singleQuotedEscaped(v)
	case ListingQuoteAlwaysDoubled:
		return singleQuoted(v, `'"'"'`, true)
	case ListingQuoteWhenNeededDollar:
		switch {
		case r.listedNeedsDollar(v):
			return r.dollarQuoted(v)
		case strings.ContainsRune(v, '\''):
			return r.dollarQuoted(v)
		case r.valueListsBare(v):
			return v
		}
		return singleQuoted(v, `'\''`, true)
	case ListingQuoteWhenNeededEscaped:
		switch {
		case r.listedNeedsDollar(v):
			return r.dollarQuoted(v)
		case r.valueListsBare(v):
			return v
		}
		return singleQuotedEscaped(v)
	case ListingQuoteWhenNeededRuns:
		switch {
		case r.listedNeedsDollar(v):
			return r.dollarQuoted(v)
		case r.valueListsBare(v):
			return v
		}
		return singleQuotedInRuns(v)
	case ListingQuoteWhenNeededPlain:
		if r.valueListsBare(v) {
			return v
		}
		return singleQuoted(v, `'\''`, true)
	case ListingQuoteAlwaysDouble:
		if r.listedNeedsDollar(v) {
			return r.dollarQuoted(v)
		}
		return doubleQuoted(v)
	}
	r.diagf("%s\n", r.unanswered("how "+what+" spells a value"))
	r.status = 2
	r.unspecified = true
	return v
}

// listedValueIsBare reports whether a value can be listed with no quotes at
// all, which the styles that ask only do for a value made of ordinary
// characters.
func (r *Runner) listedValueIsBare(v string) bool {
	if v == "" {
		return false
	}
	for i := 0; i < len(v); i++ {
		if !r.listedByteIsOrdinary(v[i]) {
			return false
		}
	}
	return true
}

// listedByteIsOrdinary is one character a listing may leave unquoted whatever
// stands around it.
//
// Most of the set is unanimous — letters, digits, `_ - . / : @ + , %`, and
// every shell metacharacter quoted — and three bytes are not. Measured
// 2026-09-14, `env -i` with a scratch HOME, over `set`, a keyed `typeset -p`
// and an alias listing alike, which agree within each column:
//
//	              ! bare   ^ bare   = bare   a non-ASCII byte bare
//	bash 5.3.15   no       no       yes      yes
//	ksh93u+       yes      yes      *        no — `$'\xNN'`, byte by byte
//	zsh 5.9.2     yes      no       no       yes
//
// No two columns group the same way, and no two *bytes* group the same way
// either — which is why these are four questions and not one. ksh93's `=` is a fourth answer rather than the other side of a
// switch — a leading `name=` is bare and the rest is quoted on its own — and
// is ListedAssignmentPrefixIsBare, asked before the style (#2820).
//
// dash and BusyBox ash are not in the table because they are not asked: every
// listing style either of them uses quotes whatever it is given, so `'^'`,
// `'a=b'` and `'é'` there say nothing about this set. Their answers are left
// unanswered rather than guessed from a neighbor.
//
// **bash's non-ASCII answer is the locale's.** Measured 2026-09-15 on the
// same binary: with `LC_ALL=C` it writes `$'\303\251'` and the key
// `[$'\303\251']`, and with any other locale set — or with none at all —
// it writes the character. zsh writes the character either way and ksh93
// spells it out either way, so the two of them answer a question about the
// byte and bash answers one about the encoding. This axis carries the
// character reading, which is what a person's terminal sees; the corpus runs
// `LC_ALL=C` and records bash's other spelling there rather than here.
//
// Read rather than asked, and the answer a dialect has not given is the one
// that quotes: a listing that quotes more than it must still reads back, and
// a listing that stopped to say the shells disagree would be a `set` with a
// complaint in the middle of it.
func (r *Runner) listedByteIsOrdinary(c byte) bool {
	switch {
	case isLetter(c) || isDigit(c) || strings.IndexByte("_-./:@+,%", c) >= 0:
		return true
	case c == '!':
		return r.sem().ListedBangIsOrdinary == Yes
	case c == '^':
		return r.sem().ListedCaretIsOrdinary == Yes
	case c == '=':
		return r.sem().ListedEqualsIsOrdinary == Yes
	case c >= 0x80:
		return r.sem().ListedNonAsciiIsOrdinary == Yes
	}
	return false
}

// listedNeedsDollar reports whether a value has to be written in `$'...'`
// rather than in quotes the shell can carry as themselves.
//
// A control byte, always — every style that reaches for the form does it for
// one. And a non-ASCII byte in the one dialect that spells those out too:
// measured, ksh93u+ writes `$'\xc3\xa9'` for a value, for a key and for an
// alias body, where bash and zsh write the character itself and leave it
// inside `$'...'` when a control byte put them there — `$'a\téb'` in both.
// So the same answer decides whether such a byte is bare and whether it is
// escaped, and there is one question rather than two.
func (r *Runner) listedNeedsDollar(v string) bool {
	if hasControl(v) {
		return true
	}
	if r.sem().ListedNonAsciiIsOrdinary == Yes {
		return false
	}
	for i := 0; i < len(v); i++ {
		if v[i] >= 0x80 {
			return true
		}
	}
	return false
}

// valueListsBare is listedValueIsBare for the value of a *declaration* and
// for a listed key, where one dialect leaves a `#` unquoted as well — see
// Semantics.ListedHashIsBareAfterANonName.
//
// Asked only where the two answers differ: a value with no `#` in it, and one
// whose `#` follows a name, are the same either way.
func (r *Runner) valueListsBare(v string) bool {
	if r.listedValueIsBare(v) {
		return true
	}
	if r.positionallyBareValue(v) {
		return true
	}
	if !r.hashIsAllThatNeedsQuoting(v) {
		return false
	}
	return r.ask(r.sem().ListedHashIsBareAfterANonName,
		"a `#` in a listed value with no name in front of it")
}

// positionallyBareValue is the position rule: the only bytes in this value a
// listing would otherwise quote are a `#` or a `~`, neither of them where it
// would start something, and the dialect leaves each of them alone there.
//
// One predicate for the two characters because it is one rule — a byte is
// quoted at the position where re-reading the value would *act* on it, a
// comment for `#` and a tilde expansion for `~`, and nowhere else — and
// because the two compose. Measured on bash 5.3.20, 2026-09-17: `a#~b` and
// `a~b#c` are bare and `~a#b` is quoted, so a value carrying both is bare
// exactly when neither is at such a position. Two separate passes would have
// quoted the mixed value, which no column does.
//
// False is "this rule does not settle it" rather than "quote it": the ksh93
// reading, which judges the text in front of the first `#` instead of the
// offset, is asked after this returns false. So the `#` question is asked here
// only where the position rule could carry the whole value, which is what
// hashIsAllThatNeedsQuoting then narrows the other way.
func (r *Runner) positionallyBareValue(v string) bool {
	hash, tilde := false, false
	for i := 0; i < len(v); i++ {
		switch {
		case r.listedByteIsOrdinary(v[i]):
		case v[i] == '#' && i > 0:
			hash = true
		case v[i] == '~' && !tildeWouldExpandAt(v, i):
			tilde = true
		default:
			return false
		}
	}
	if hash && !r.ask(r.sem().ListedHashIsBareUnlessItOpensTheValue,
		"a `#` in a listed value that does not open it") {
		return false
	}
	if tilde && !r.ask(r.sem().ListedTildeIsBareWhereItCannotExpand,
		"a `~` in a listed value where no tilde expansion could start") {
		return false
	}
	// Both false is a value listedValueIsBare has already answered, and this
	// is only ever reached from there.
	return hash || tilde
}

// tildeWouldExpandAt reports whether a `~` at this offset is one the shell
// would expand if it read the value back unquoted.
//
// Three positions and not one, which is what the `#` rule beside it does not
// need: a tilde expands at the front of a word, and in an *assignment's* value
// it expands again after every `:` and after every `=` — the rule that makes
// `PATH=$PATH:~/bin` work. So those are the offsets a listing has to quote
// for, and nothing else is. Measured 2026-09-17 on bash 5.3.20 over a bare
// `set` and a keyed `declare -p`, which agree:
//
//	bare      a~b   b~   a:b~c   a,~b   a:x~b   a~~b   a/~b   a-~b   a.~b
//	quoted    ~b    ~    a:~b    a:~    :~b     a=~b   a::~b  a:~:b  ~~
//
// The character in front decides on its own: `a,~b` and `a@~b` are bare, so it
// is not "any punctuation", and `a:x~b` is bare, so it is not "anywhere after
// a colon" either.
func tildeWouldExpandAt(v string, i int) bool {
	return i == 0 || v[i-1] == ':' || v[i-1] == '='
}

// hashIsAllThatNeedsQuoting reports whether the only reason this value is not
// bare is a `#`, and that `#` is one the dialect above may leave alone: the
// text in front of the first one is there and is no name.
//
// The first `#` decides for the whole value, which is measured rather than
// convenient — `1#b#c` is bare in the shell that has this and `a#b#c` is
// quoted, so it is the leading text and not each occurrence that is judged.
//
// Measured 2026-09-12 on ksh93u+, listing a scalar with `typeset -p`:
//
//	16#ff  99#zz  16#gg  16#  1#0  1a#b  9x#y  /1#a  .1#a  a.b#c   bare
//	a#b  ab#  a1#  _#  e1#a  A1#a  a#b#c  #lead  tail#  #        quoted
//
// So the rule is not "a based number", which is what the issue that asked for
// this proposed: `99#zz` names no base and `16#gg` has no digits for the one
// it names, and both are bare. What is quoted is a `#` that a *name* stands
// in front of, or one that opens the value — where a comment would begin.
func (r *Runner) hashIsAllThatNeedsQuoting(v string) bool {
	hash := strings.IndexByte(v, '#')
	if hash <= 0 || isNameLike(v[:hash]) {
		return false
	}
	for i := 0; i < len(v); i++ {
		if v[i] != '#' && !r.listedByteIsOrdinary(v[i]) {
			return false
		}
	}
	return true
}

func hasControl(v string) bool {
	for i := 0; i < len(v); i++ {
		if v[i] < 0x20 || v[i] == 0x7f {
			return true
		}
	}
	return false
}

// singleQuoted wraps in single quotes, replacing each embedded quote with the
// dialect's way of writing one.
//
// Every escape here closes the quoted run, writes a quote somehow, and reopens
// it. When the value *ends* in a quote that reopened run is empty, and the
// panel splits on whether to leave it there: bash keeps the empty pair, and
// dash and zsh drop the reopen along with its closing quote.
//
// Only a value ending in a quote can show it, which is why it took a probe
// written to end in one.
// singleQuotedEscaped is bash's spelling: the whole value inside one pair of
// single quotes, with each embedded quote written as a closing quote, an
// escaped quote and a reopening one.
//
// The exception is the value that is *nothing but* one quote, which bash 5
// writes as an escaped quote alone — measured 2026-09-12 in 5.3.15, over a
// bare `set` and over `alias`, and it is the one-byte value alone: a value of
// two quotes comes back fully wrapped, empty pairs and all. The 3.2.57 macOS
// ships has no exception and wraps the single quote too, so this is a version
// line inside one lineage — the shape ArithDoubleQuote already has — and the
// preset follows the current build.
func singleQuotedEscaped(v string) string {
	if v == "'" {
		return `\'`
	}
	return singleQuoted(v, `'\''`, false)
}

// singleQuotedInRuns is zsh's spelling: the value cut at each quote, every
// non-empty run wrapped on its own, and each quote written with a backslash
// outside any quoting.
//
// It is the shape quoteInRuns already writes for the `q` modifiers, asked
// with the predicate pinned true: this value has already been found to need
// quoting, so the question those ask of each run separately is settled for
// all of them at once.
func singleQuotedInRuns(v string) string {
	return quoteInRuns(v, func(string, int) bool { return true })
}

func singleQuoted(v, escape string, trimEmptyTail bool) string {
	quoted := "'" + strings.ReplaceAll(v, "'", escape) + "'"
	if trimEmptyTail && strings.HasSuffix(v, "'") {
		// Drop the closing quote and the reopening one the last escape
		// wrote, which between them wrap nothing.
		return quoted[:len(quoted)-2]
	}
	return quoted
}

// doubleQuoted wraps in double quotes, escaping the four characters that are
// live inside them: backslash, backquote, dollar and the quote itself.
func doubleQuoted(v string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(v); i++ {
		if c := v[i]; c == '\\' || c == '`' || c == '$' || c == '"' {
			b.WriteByte('\\')
		}
		b.WriteByte(v[i])
	}
	b.WriteByte('"')
	return b.String()
}

// dollarQuoted writes `$'...'`, the spelling that can carry a control
// character.
func (r *Runner) dollarQuoted(v string) string {
	style := r.sem().ListingControlEscape
	var b strings.Builder
	b.WriteString("$'")
	for i := 0; i < len(v); i++ {
		switch c := v[i]; {
		case c == '\'':
			b.WriteString(`\'`)
		case c == '\\':
			b.WriteString(`\\`)
		case c >= 0x20 && c != 0x7f && (c < 0x80 || r.sem().ListedNonAsciiIsOrdinary == Yes):
			b.WriteByte(c)
		default:
			b.WriteString(controlEscaped(style, c))
		}
	}
	b.WriteString("'")
	return b.String()
}

// ControlEscapeStyle is how a `$'...'` listing spells a byte the quotes cannot
// carry as itself. Every shell that lists with `$'...'` escapes *every*
// control byte — a listing is meant to be readable back, and a raw NUL makes
// the whole output binary, which is what `grep` says of it — and no two of
// them spell one the same way. Measured 2026-09-05 from
// `v=$'\a\b\t\n\v\f\r\e\001\037\177'` in each:
//
//	bash    \a \b \t \n \v \f \r \E then three octal digits: \001 \037 \177
//	ksh93   \a \b \t \n     \f \r \E then two hex digits:     \x0b \x01 \x7f
//	zsh             \t \n                 then a caret pair:      \C-G \C-A \C-?
//
// The named sets are not the same either — ksh93 has no `\v` and zsh has only
// the two — so the style names one whole vocabulary rather than a fallback.
// dash never reaches here: it single-quotes every value it lists and writes a
// control byte as itself.
type ControlEscapeStyle int

const (
	// ControlEscapeUnspecified is no answer. It is not refused, because the
	// question is only reachable from a quoting style that has already been
	// answered; it spells the numeric fallback the way POSIX's `printf` does.
	ControlEscapeUnspecified ControlEscapeStyle = iota
	// ControlEscapeOctal is bash's: the widest named set, then `\NNN`.
	ControlEscapeOctal
	// ControlEscapeHex is ksh93's: the same named set without `\v`, then
	// `\xNN` in lowercase.
	ControlEscapeHex
	// ControlEscapeCaret is zsh's: `\t` and `\n` alone, then `\C-X` — the
	// byte with bit 6 flipped, so NUL is `\C-@`, 1 is `\C-A` and delete is
	// `\C-?`.
	ControlEscapeCaret
)

func (c ControlEscapeStyle) String() string {
	switch c {
	case ControlEscapeOctal:
		return "ControlEscapeOctal"
	case ControlEscapeHex:
		return "ControlEscapeHex"
	case ControlEscapeCaret:
		return "ControlEscapeCaret"
	}
	return "ControlEscapeUnspecified"
}

// controlEscaped spells one control byte in the given style.
func controlEscaped(style ControlEscapeStyle, c byte) string {
	if style == ControlEscapeCaret {
		switch c {
		case '\t':
			return `\t`
		case '\n':
			return `\n`
		}
		return fmt.Sprintf(`\C-%c`, c^0x40)
	}
	switch c {
	case '\a':
		return `\a`
	case '\b':
		return `\b`
	case '\t':
		return `\t`
	case '\n':
		return `\n`
	case '\v':
		if style == ControlEscapeHex {
			// The one byte ksh93 leaves out of the named set.
			break
		}
		return `\v`
	case '\f':
		return `\f`
	case '\r':
		return `\r`
	case 0x1b:
		return `\E`
	}
	if style == ControlEscapeHex {
		return fmt.Sprintf(`\x%02x`, c)
	}
	return fmt.Sprintf(`\%03o`, c)
}
