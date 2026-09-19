// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// ProgramRoutes is a set of the ways a non-interactive program reaches a
// shell, for the grammar questions whose answer depends on which of them it
// was.
//
// The routes are the front end's — a command string, a file, standard input —
// and this package names them anyway, because the questions have to be
// askable where they are answered. Two ask it:
//
//	[Dialect.ExpandAliases]      read by the front end, which passes the
//	                             table of aliases in or leaves it nil.
//	[Dialect.CloseQuotesAtEOF]   read by the lexer, which is told the route
//	                             through [Dialect.ProgramRoute].
//
// `eval` is a command string. It is not an invocation route at all, but it is
// a program handed over as a *string* rather than read from a file or a
// descriptor, and that is the distinction ksh93 draws — measured 2026-09-07,
// `eval "echo 'abc"` inside a script file prints abc there where the same
// text as the script itself is refused.
type ProgramRoutes uint8

const (
	// RouteFromCommandString is a program given as an argument: `-c`. Also
	// a string handed to `eval`, which is the same kind of program by the
	// one measurement that separates the kinds.
	RouteFromCommandString ProgramRoutes = 1 << iota
	// RouteFromScriptFile is a program read from a path named as an operand.
	// Also a file read by `.`, and a startup file.
	RouteFromScriptFile
	// RouteOnStandardInput is a program read from the descriptor, whether
	// by `-s` or by there being no operand and no terminal.
	RouteOnStandardInput
	// RouteOnEveryRoute is what a dialect that does not distinguish them
	// answers.
	RouteOnEveryRoute = RouteFromCommandString | RouteFromScriptFile | RouteOnStandardInput
	// RouteOnNoRoute is the empty set, spelled so a dialect can say it
	// deliberately rather than by leaving a field out.
	RouteOnNoRoute ProgramRoutes = 0
)

// Has reports whether route is in the set. A caller asks with exactly one
// route, which is what it knows.
//
// The empty route is in no set, which is what makes the strict answer the
// default: a parse that was never told how its program arrived — a function
// body being re-read, a prelude, a tree dump — gets the grammar every shell
// agrees on rather than one shell's leniency.
func (a ProgramRoutes) Has(route ProgramRoutes) bool { return a&route != 0 }

// SeparatorSkip is how far a dialect will step over a `;` written where the
// grammar wants a command. See [Dialect.SeparatorWhereACommandBelongs].
//
// A count and a place rather than a bool, because the two shells that allow
// this draw it differently and a single yes/no could not be given a value for
// either of them without accepting lines the other refuses.
type SeparatorSkip uint8

const (
	// NoSeparatorWhereACommandBelongs is the core answer: a `;` where a
	// command belongs is a syntax error. dash, bash 5.3, bash 3.2 and
	// bash-as-`sh`.
	NoSeparatorWhereACommandBelongs SeparatorSkip = iota

	// OneSeparatorExceptAfterABarOrBeforeACondition steps over a single `;`,
	// and not at all after a `|` or where a *condition* begins. ksh93, and
	// every part is measured rather than assumed: `a || ; ; b` is
	// `` `;' unexpected `` there where `a || ; b` runs, and `a | ; b` is
	// refused where `a || ; b` and `a |& ; b` are taken — the same asymmetry
	// #1115 found for that shell's `|&`, and the probe that says the bar is a
	// separate question from the and-or.
	//
	// The condition is the third exception and was missing, so `if; then`
	// parsed here and was blamed on the `then` where ksh93 blames the `;`.
	// Measured 2026-09-12 over a script file:
	//
	//	if; then :; fi                 `;' unexpected
	//	while; do :; done              `;' unexpected
	//	until; do :; done              `;' unexpected
	//	if :; then :; elif; then :; fi `;' unexpected
	//	if :; ; then :; fi             `;' unexpected — the whole list, not
	//	                               only the position after the keyword
	//	if : ; :; then :; fi           runs — a `;` *terminating* a statement
	//	                               of the condition is not this
	//	if false || ; then :; fi       runs — where an and-or's right-hand
	//	                               side belongs is still the and-or's
	//	if :; then : ; ; fi            runs — a body is not a condition
	//
	// So it is the position where a *statement of a condition list* begins,
	// and neither "anywhere in the header" nor "the token after the keyword".
	// The wider value takes all nine: zsh parses every line above (#2023).
	OneSeparatorExceptAfterABarOrBeforeACondition

	// AnySeparatorWhereACommandBelongs steps over as many as are written,
	// anywhere, the bar included. zsh: `echo one | ; ; cat -n` numbers the
	// line, and so does `echo one | ; ⏎ ; cat -n`.
	AnySeparatorWhereACommandBelongs

	// SeparatorOnlyWhereAnAndOrWantsOne steps over a single `;` where an
	// and-or's right-hand side belongs and nowhere else at all.
	//
	// No dialect holds this as its own answer: it is what
	// OneSeparatorExceptAfterABarOrBeforeACondition becomes inside the body
	// of a `$( … )` or a `${ …;}` in the shell that holds that value — see
	// [Dialect.SubstitutionBodyRefusesASteppedOverSeparator], which is where
	// the measurement is.
	SeparatorOnlyWhereAnAndOrWantsOne
)

// ArraySemicolon is how far a dialect will take a `;` inside an array
// literal's parentheses. See [Dialect.SemicolonInAnArrayLiteral].
//
// A place rather than a bool, for the reason [SeparatorSkip] is one: the two
// shells that take a `;` there draw it in two different sets, and a single
// yes/no could not be given a value for either without accepting lines the
// other refuses.
//
// Measured 2026-09-12, `env -i PATH=/usr/bin:/bin` with a scratch HOME and
// ZDOTDIR, `-n` over a script file and then a run printing `${#a[@]}` and the
// elements. dash and BusyBox ash have no array literal at all, so the `(` is
// already their error and the question does not reach them.
//
//	probe            bash 5.3 / 3.2 / as sh   ksh93            zsh 5.9.2
//	a=( x; )         `;' unexpected           runs, 1 element  runs, 1 element
//	a=( x y; )       `;' unexpected           runs, 2 elements runs, 2 elements
//	a=( x ⏎ ; )      `;' unexpected           runs             runs
//	a=( x; ⏎ )       `;' unexpected           runs             runs
//	a=( x; y )       `;' unexpected           `y' unexpected   runs, 2 elements
//	a=( x; y; )      `;' unexpected           `y' unexpected   runs, 2 elements
//	a=( x; ⏎ y )     `;' unexpected           `y' unexpected   runs, 2 elements
//	a=( ; )          `;' unexpected           `;' unexpected   runs, 0 elements
//	a=( x; ; )       `;' unexpected           `;' unexpected   runs, 1 element
//	a=( x;; y )      `;;' unexpected          `;;' unexpected  `;;' error
//	a=( x & )        `&' unexpected           `&' unexpected   `&' error
//	a=( x && y )     `&&' unexpected          `&&' unexpected  `&&' error
//
// The last two rows are what say the `;` is specifically a separator rather
// than the parser being lenient about operators, and the `;;` row says the
// two-character token stays its own token in the shell that takes one.
type ArraySemicolon uint8

const (
	// NoSemicolonInAnArrayLiteral is the core answer: a `;` between the
	// parentheses is a syntax error. Every bash column, and the two shells
	// with no array literal never reach the question.
	NoSemicolonInAnArrayLiteral ArraySemicolon = iota

	// OneSemicolonEndsTheArrayElements takes a single `;` after the last
	// element, where nothing but newlines and the closing `)` may follow it.
	// ksh93.
	//
	// It is a terminator and not a separator, which is the correction the
	// measurement above made to the filing: `a=( x; y )` is `` `y' unexpected ``
	// there, so the `;` does not stand between two elements. It also needs an
	// element in front of it — `a=( ; )` is `` `;' unexpected `` — and it may
	// be written once — `a=( x; ; )` is `` `;' unexpected `` on the second.
	OneSemicolonEndsTheArrayElements

	// SemicolonSeparatesArrayElementsLikeANewline takes as many as are
	// written, anywhere between the parentheses, exactly where a newline
	// already stands. zsh 5.9.2, where `a=( ; )` is the empty array and
	// `a=( x; y )` holds two elements.
	SemicolonSeparatesArrayElementsLikeANewline
)

// HeredocDelimiterTabs is what `<<-` does with tabs the *delimiter* was
// written with. See [Dialect.StrippedHeredocDelimiter].
//
// The operator strips leading tabs from every body line, and that part is
// unanimous. A delimiter can only begin with a tab if it was quoted — `<<-
// '<tab>EOF'` — and then the stripped lines can never be spelled like it, so
// each shell has had to decide what such a document ends at. Three answers,
// measured 2026-09-16 from script files under `env -i`, standard input the
// null device, the body printed by `cat`:
//
//	delimiter written  <tab>EOF   <tab>EOF   <tab>EOF        <tab><tab>EOF
//	end line           <tab>EOF   EOF        <tab><tab>EOF   <tab><tab>EOF
//	bash 5.3.20        ends       runs out   runs out        ends
//	zsh 5.9.2          ends       ends       ends            ends
//	ksh93u+ 2012       ends       ends       ends            ends
//	dash 0.5.12        runs out   runs out   runs out        runs out
//	BusyBox ash 1.37   runs out   runs out   runs out        runs out
//
// So bash compares the line as written as well as the stripped line, zsh and
// ksh93 strip the delimiter's tabs as they strip the line's, and dash compares
// the stripped line against the delimiter as written, which nothing can equal.
// A tab anywhere but the front of the delimiter is unanimous and is not this
// question, and neither is `<<` without the dash, which strips nothing in any
// column. BusyBox ash was measured the same day through the pinned Alpine
// image and answers as dash does.
type HeredocDelimiterTabs uint8

const (
	// HeredocDelimiterTabsAreKept compares the stripped line against the
	// delimiter as it was written, so a delimiter that opens with a tab is
	// never met and the body runs to the end of the input. dash, BusyBox
	// ash, and the core.
	HeredocDelimiterTabsAreKept HeredocDelimiterTabs = iota

	// HeredocLineAsWrittenMeetsTheDelimiter also compares the line as it was
	// written, before its tabs were stripped: `<tab>EOF` ends a `<tab>EOF`
	// document and `EOF` does not. bash 5.3.
	HeredocLineAsWrittenMeetsTheDelimiter

	// HeredocDelimiterTabsAreStrippedToo strips the delimiter's leading tabs
	// as the lines' are stripped, so any line reading `EOF` once its tabs are
	// gone ends a `<tab>EOF` document. zsh 5.9.2 and ksh93u+.
	HeredocDelimiterTabsAreStrippedToo
)

// ContinuedHeredocDelimiter is how far a here-document body line assembled
// across a backslash-newline may go toward being the delimiter. See
// [Dialect.HeredocDelimiterAcrossAContinuation].
//
// A place rather than a bool, for the reason [SeparatorSkip] and
// [ArraySemicolon] are: the panel draws this in three sets, not two, and a
// single yes/no could not be given a value for any of them without taking a
// line one of the others reads as body.
//
// That a body line ends in a backslash and continues at all is unanimous and
// is not this question — every column joins `A\` to the line under it and
// looks for the delimiter afterwards, which is what #2430 was. This is the
// narrower one: the *joined* text is spelled exactly like the delimiter, and
// the columns part over whether that ends the document.
//
// Measured 2026-09-12 with `-c` and the null device on standard input, the
// body printed by `cat`:
//
//	delimiter ABC, body line    A\ ⏎ BC        \ ⏎ ABC
//	                            joins to ABC   joins to ABC
//	                            after text     before any
//	bash 5.3 / 3.2 / as `sh`    delimiter      delimiter
//	zsh 5.9.2                   delimiter      delimiter
//	dash                        body           delimiter
//	BusyBox ash                 body           delimiter
//	ksh93u+ 2012                body           body
//
// So the second column is what separates dash and BusyBox ash from ksh93: a
// continuation standing *before* any text of the line still leaves the
// delimiter reachable there, and one standing after text does not.
//
// One corner below this is measured and deliberately not modeled, because the
// panel parts three ways again and over tab-stripping rather than over the
// delimiter. `<<-EOF` with a body line of one tab and a backslash, the
// delimiter written under it with its own tab:
//
//	cat <<-EOF ⏎ →\ ⏎ →EOF ⏎ X ⏎ →EOF
//
// bash strips the tabs of the *joined* text and so reads the line as `EOF`
// and ends the document there; zsh and ksh93 strip only the tabs the logical
// line opens with, leaving `<tab>EOF` as body, which is what this
// implementation does; dash and BusyBox ash keep the backslash-newline
// outright and join nothing. Two of the five columns agree with what is here
// and a rule for the other three would be three rules.
type ContinuedHeredocDelimiter uint8

const (
	// NoHeredocDelimiterAcrossAContinuation is the core answer and the safe
	// one: a line that took a continuation is never the delimiter. ksh93u+.
	//
	// Safe because of which way the two readings fail. Reading a line as the
	// delimiter that the writer meant as body ends the document early and
	// hands the rest of the body to the parser as *commands*; reading a
	// delimiter as body only runs the document on, which is unfinished input
	// and says so.
	NoHeredocDelimiterAcrossAContinuation ContinuedHeredocDelimiter = iota

	// HeredocDelimiterAfterALeadingContinuation lets the delimiter be found
	// after continuations that stand before any text of the line, and not
	// after one that stands after text. dash and BusyBox ash.
	HeredocDelimiterAfterALeadingContinuation

	// HeredocDelimiterOnTheJoinedLine compares the whole joined text, however
	// many physical lines went into it. bash 5.3, bash 3.2, bash as `sh`, and
	// zsh.
	HeredocDelimiterOnTheJoinedLine
)

// CaseBraceSpelling is how a dialect writes a `case` header's second
// spelling, `case x { … }`. See [Dialect.CaseBraceBody], where the rows are.
//
// A spelling rather than a bool because the two shells that have the
// construct disagree about whether the opener and the closer are paired, and
// a single yes/no could not be given a value for either without accepting
// lines the other refuses.
type CaseBraceSpelling uint8

const (
	// NoCaseBraceBody is the core answer: a `case` header is `in` and the
	// clause ends at `esac`. dash and every bash column.
	NoCaseBraceBody CaseBraceSpelling = iota

	// CaseBraceBodyPairsWithItsOpener takes `case x { … }` and `case x in …
	// esac` and neither of the mixtures. ksh93u+, where `case x { … esac`
	// and `case x in … }` are both `` `case' unmatched ``.
	CaseBraceBodyPairsWithItsOpener

	// CaseBraceBodyMixesWithTheKeyword takes all four combinations: either
	// opener closes with either word. zsh 5.9.2.
	CaseBraceBodyMixesWithTheKeyword
)

// BareNegationReach is how far a `!` written with no pipeline after it may
// stand. See [Dialect.BareNegationReach].
//
// A place rather than a bool, for the reason [SeparatorSkip] is one: the three
// shells that take a bare `!` draw its boundary in three different sets, and a
// single yes/no could not be given a value for any of them without accepting
// lines another refuses.
//
// Measured 2026-09-12, `env -i PATH=/usr/bin:/bin` with a scratch HOME, over
// `-c` and a script file alike. `st=` is what `echo "st=$?"` printed after it.
//
//	after the `!`      dash   bash 3.2   bash 5.3   bash as sh   ksh93   zsh
//	`;`                error  error      st=1       st=1         st=1    st=1
//	a newline          error  error      st=1       st=1         st=1    st=1
//	the end of input   error  error      st=1       st=1         st=1    st=1
//	`&`                error  error      st=0       st=0         runs    error
//	`)` of a subshell  error  error      error      error        st=1    st=1
//	`}` of a group     error  error      error      error        runs    runs
//	`;;` of a case arm error  error      error      error        st=1    st=1
//	`&&`               error  error      error      error        runs    runs
//	`||`               error  error      error      error        runs    runs
//	`|`                error  error      error      error        error   error
//
// bash-as-`sh` follows bash 5.3 and not bash 3.2, which #948 suspected might
// be POSIX mode rather than the version. It is the version: `bash --posix -c
// '!'` answers 1 on the same binary.
type BareNegationReach uint8

const (
	// NoBareNegation is the core answer: a `!` needs a pipeline after it.
	// dash and bash 3.2.
	NoBareNegation BareNegationReach = iota

	// BareNegationBeforeATerminator takes a `!` that a statement terminator
	// or the end of input follows — `;`, a newline, `&`, EOF — and nothing
	// else. bash 5.3 and that binary as `sh`: `{ ! ; echo "st=$?"; }` prints
	// `st=1` there where `{ ! }`, `( ! )` and `! && echo two` are all
	// refused.
	BareNegationBeforeATerminator

	// BareNegationWhereAListEnds takes it where the *list* ends instead —
	// a closer, a `case` terminator, or an and-or operator whose right-hand
	// side is where the `!` stood — as well as before `;`, a newline and the
	// end of input. Not before `&`, which is the one row that separates this
	// from the value below: zsh refuses `! & echo x` and takes `( ! )`,
	// where bash 5.3 does the opposite of both.
	//
	// The `&` exception is the same boundary [Dialect.OpenEndedAndOr] has in
	// that shell, where `true || & b` is a parse error and `( true || )`
	// runs.
	BareNegationWhereAListEnds

	// BareNegationAtEitherPlace takes both sets, which is ksh93: it is the
	// union rather than a third rule, and every row above says so.
	BareNegationAtEitherPlace
)

// EndOfInputBackslash is what an unquoted backslash the input ends
// immediately after becomes — a line continuation with no line to continue.
// See [Dialect.BackslashAtEndOfInput].
//
// No shell refuses it, which is the half of this with no axis in it: all
// eight columns run the line and exit 0. What the backslash *becomes* is
// where they part, and they part three ways among the dialects, so a bool
// could be given a value for neither ksh93 nor the pair it sits between.
//
// Measured 2026-09-13 over `-c`, and over a script file with no trailing
// newline, which answer alike. A file that *ends* in a newline is a
// different question with one answer: the backslash is then an ordinary
// continuation, and every column drops it and prints `[x]` and `[]`.
//
//	written            bash 5.3  bash as sh  bash 3.2  dash    ksh93   zsh    zsh as sh  ash
//	printf "[%s]" x\   [x\]      [x\]        [x]       [x\]    [x]     [x]    [x]        [x\]
//	printf "[%s]" \    [\]       [\]         []        [\]     [\]     []     []         [\]
//	printf "[%s]" a \  [a][\]    [a][\]      [a]       [a][\]  [a][\]  [a][]  [a][]      [a][\]
//
// The third row reuses one conversion rather than writing two, and that is
// the only spelling that can see the fourth reading: `printf "[%s][%s]" a \`
// prints `[a][]` in bash 3.2 as well as in zsh, because a format with two
// conversions fills a missing operand with the empty string and so cannot
// tell a word that is empty from a word that is gone. Reused, the format is
// printed once per operand, so `[a]` alone says the operand is not there.
//
// bash 3.2 holds that fourth reading and it is recorded rather than given a
// value: it drops the *word* along with the backslash, where zsh keeps an
// empty one — `[a]` against `[a][]` on the third row. No dialect preset is
// bash 3.2, so inventing a value for it would put a reading in the vector
// that nothing could ask for. The row it is visible in is in the corpus, so
// docs/spec/measurements.md carries the column.
type EndOfInputBackslash uint8

const (
	// EndOfInputBackslashIsLiteral keeps it: the word ends with a protected
	// backslash, wherever in the word it stood. bash 5.3, bash as `sh`,
	// dash and BusyBox ash, and the core answer because it is what four of
	// the six columns that are a dialect do and because it is the reading
	// that loses nothing — the text the writer typed is still in the field.
	EndOfInputBackslashIsLiteral EndOfInputBackslash = iota

	// EndOfInputBackslashIsDropped removes it and keeps the word, which is
	// an empty field when the backslash was all of it. zsh 5.9.2, as itself
	// and as `sh`.
	EndOfInputBackslashIsDropped

	// EndOfInputBackslashIsLiteralOnlyAtAWordStart keeps it where the
	// backslash is the first thing in the word and drops it anywhere else.
	// ksh93u+, and measured rather than guessed at: `printf "[%s]" "x"\`
	// and `printf "[%s]" 'q'\` are both `[x]`/`[q]` there, so it is having
	// read *anything* into the word that decides it and not the character
	// in front of the backslash. `printf "[%s]" \\\` is `[\]` — the escaped
	// pair is read first, so the trailing one is no longer at a start.
	EndOfInputBackslashIsLiteralOnlyAtAWordStart
)

// BraceProgramBodyEnd says where the body of `${ cmd;}` stops.
//
// The body holds a command list, and the two shells that have the construct
// disagree about which `}` ends it. Measured 2026-09-13 over `-c`, ksh93u+
// 2012-08-01 and bash 5.3.15 — the four columns without the construct are not
// in the table because they refuse every row of it:
//
//	written                      ksh93            bash 5.3
//	${ echo a}b;}                a}b              a}b
//	${ echo hi}                  `{' unmatched    unexpected EOF
//	${ echo } ;}                 `}' unexpected   }
//	${ echo a } b;}              `}' unexpected   a } b
//	A${ echo B }C                ABC              unexpected EOF
//	${ echo {a,b};}              `{' unmatched    a b
//	${ echo a;}                  a                a
//
// Both readings refuse `${ echo hi}`, which is the row that matters most:
// a body with no terminator in front of the brace is not a closed
// substitution in either column, and reading it as one runs a command the
// author never wrote (#2711). The rest of the table is where the two part
// company (#2724).
//
// The values carry the two rules; see [BraceProgramBodyEnd]'s constants.
type BraceProgramBodyEnd uint8

const (
	// BraceProgramBodyEndsWhereAListEnds ends the body exactly where the
	// list of a `{ …; }` group ends: at the reserved word `}`, which stands
	// only where a command may begin. bash 5.3, and the core, because it is
	// the rule the construct's own braces already have everywhere else in
	// the grammar.
	//
	// It is why `echo }` passes a literal brace to `echo` from inside a body
	// — rows three and four above — and why a body with no terminator runs
	// off the end of the input rather than closing at the first `}` it meets.
	BraceProgramBodyEndsWhereAListEnds BraceProgramBodyEnd = iota

	// BraceProgramBodyEndsAtATokenStart ends it at a `}` that *begins a
	// token*, which is argument position as well as command position, and
	// lets a `{` that begins one open a nested level that a token-start `}`
	// closes. ksh93u+.
	//
	// Rows three and four are the visible consequence: a `}` written as an
	// argument ends the body there rather than reaching `echo`, so what is
	// left over is a stray brace and the line is refused. Row six is the
	// other half — `{a,b}` opens a level whose `}` is mid-word and closes
	// nothing, so the body runs past the end of the input.
	//
	// Quotes and the older substitution shield a brace and the newer one
	// does not, which is measured rather than assumed: a body whose `}` sits
	// inside a backquoted substitution keeps it — `}x` comes back — and the
	// same body written `${ echo $(echo x)}` is `` `{' unmatched ``, so the
	// `)` that closes a `$( )` leaves the cursor inside a word while the
	// `)` of a subshell — `${ (echo q)}` — begins the next token.
	BraceProgramBodyEndsAtATokenStart
)

// DollarForms is a set of the forms a `$` introduces — what the character
// behind it says the construct is.
//
// A set rather than a count, because the two shells that narrow it narrow it
// to sets neither of which contains the other: see
// [Dialect.ContinuationStopsADollarAt], which is the one question that asks
// it.
type DollarForms uint8

const (
	// DollarBareParameter is `$x`, `$1`, `$#`, `$@` — a name, a positional
	// or a special parameter written with no delimiter of its own.
	DollarBareParameter DollarForms = 1 << iota
	// DollarBraces is `${…}`, every operator inside it included.
	DollarBraces
	// DollarParens is `$(…)` and `$((…))`, which the character after the
	// first parenthesis tells apart long after this question is settled.
	DollarParens
	// DollarBrackets is `$[…]`, the older arithmetic spelling, where
	// [Dialect.DollarBracketArith] gives the dialect the form at all.
	DollarBrackets
	// DollarQuotes is `$'…'` and `$"…"`, which are quoting rather than
	// expansion but are introduced by the same character.
	DollarQuotes

	// NoDollarForm is the empty set, spelled so a dialect can say it
	// deliberately rather than by leaving a field out.
	NoDollarForm DollarForms = 0
	// EveryDollarForm is all five.
	EveryDollarForm = DollarBareParameter | DollarBraces | DollarParens | DollarBrackets | DollarQuotes
)

// Has reports whether form is in the set. A caller asks with exactly one
// form, which is what it knows.
func (f DollarForms) Has(form DollarForms) bool { return f&form != 0 }

// DeclarationArrayWord is how a declaration utility's name must be *written*
// for a `name=( … )` operand behind it to be read as an array literal.
//
// The grammar half of what [interp.Semantics.DeclarationCommandWord] answers
// for expansion, and it is a separate question because it decides where the
// *word ends* rather than how the operand expands: under one reading
// `'typeset' a=(x y)` is a syntax error and under the other it is an array.
// See [Dialect.DeclarationArrayFromTheCommandWord].
type DeclarationArrayWord uint8

const (
	// DeclarationArrayFromAnUnquotedLiteralWord reads the operand as an
	// array only where the command word is one unquoted literal with no
	// quoting of any kind in it. `'typeset'`, `\typeset`, `type"set"` and
	// `$cmd` all take the reading away. bash 5.3 and zsh 5.9.2, and the
	// core.
	DeclarationArrayFromAnUnquotedLiteralWord DeclarationArrayWord = iota

	// DeclarationArrayFromAWrittenWord keeps the reading through quoting —
	// `'typeset'`, `\typeset` and `type"set"` all still open an array — and
	// loses it only where part of the word is an expansion. ksh93u+.
	DeclarationArrayFromAWrittenWord
)

// Dialect says which constructs the lexer accepts.
//
// Fields are named for the construct rather than for the shell that wants it,
// which docs/spec/semantics.md requires and which the measurements insist on:
// ksh93 accepts `&>` or does not depending on which build is installed, twelve
// years apart under the same name, so a field called `Ksh` could not be given
// a value. A field called [Dialect.AmpersandRedirect] can.
//
// Grammar differences are additive — a construct either parses or it does not
// — which is why this is a set of flags. Semantic differences, where the same
// syntax means different things, are not additive and do not belong here; they
// are the interpreter's problem and get their own vector.
type Dialect struct {
	// AmpersandRedirect enables `&>` and `&>>`, which redirect both streams.
	//
	// This is the one to be careful with. Where it is off, `echo hi &>b` is
	// not an error: it is `echo hi &` — a background command — followed by
	// `>b`, which truncates the file. The command runs, its output goes
	// elsewhere, and nothing is reported. Accepting the union of dialects
	// here would silently pick one meaning for text that legitimately has
	// two.
	AmpersandRedirect bool

	// BareNegationReach says where a `!` written with no pipeline after it
	// may stand, and how far the shell will look for one.
	//
	// It is a *pipeline with no commands*, which is the reading the status
	// settles: `true; !` and `false; !` both answer 1 wherever the line is
	// taken, so nothing ran and a success was inverted. It is not "the `!`
	// negates the next line's pipeline", which #948 read it as — that would
	// make `! ⏎ echo x; echo "st=$?"` print `st=1`, and it prints `st=0` in
	// all three shells that take the line.
	//
	// The values carry the measurements; see [BareNegationReach].
	BareNegationReach BareNegationReach

	// RepeatedNegationToggles lets a pipeline carry more than one `!`, each
	// inverting the one before it.
	//
	// bash 5.3, that binary as `sh`, and ksh93. Measured 2026-09-12, and it
	// really is a toggle rather than an idempotent mark:
	//
	//	! ! true    st=0     ! ! false   st=1
	//	! ! !       st=1     ! ! ! true  st=1
	//	! !         st=0
	//
	// dash and zsh refuse a second `!` outright, which is what makes this a
	// question of its own rather than part of [Dialect.BareNegationReach]:
	// zsh takes a bare `!` and refuses `! !`, so a dialect answering one
	// answers nothing about the other.
	//
	// The tree carries one flag and not a count, which the toggle is what
	// permits: an even number of them is no negation and an odd number is
	// one, so `! ! !` and `!` are the same program and print back the same.
	RepeatedNegationToggles bool

	// PipeBothStreams enables `|&`, a pipe that carries the left command's
	// standard error along with its standard output. Measured identical to
	// writing `2>&1` as the left command's *last* redirection — not its
	// first: `e 2>/dev/null |& cat` shows the error and `e 2>&1 2>/dev/null
	// | cat` does not, and `e >/dev/null |& cat` shows nothing where `e 2>&1
	// >/dev/null | cat` shows the error. Both discriminating shapes agree in
	// both shells that have it, so the parser writes the redirection out
	// rather than the interpreter growing a second way to point a stream.
	//
	// Two of the six panel columns accept this reading — bash 5.3 and zsh —
	// which is why it is not core. bash 3.2 has no `|&` at all, so this is a
	// version fact as much as a dialect one; dash has none either. ksh93
	// spells a *coprocess* with the same two characters, and it is not this
	// construct with another meaning but another slot in the grammar: ksh93
	// refuses `a | ; b` and accepts `a |& ; b`, so its `|&` terminates a
	// command the way `&` does rather than joining two. That reading is
	// [Dialect.CoprocPipeOperator], a flag of its own beside Coproc and
	// never a second value of this one.
	//
	// Where it is off, `a |& b` is not silently something else: the operator
	// table falls back to `|` and then `&`, which is exactly what bash 3.2
	// and dash lex, so the refusal lands on the `&` where theirs does.
	PipeBothStreams bool

	// CaseFallthrough enables `;&`, which runs the next case body. Absent
	// from dash, and from bash before 4.0 — so it cannot be reached through
	// macOS's /bin/sh.
	//
	// Where it is off the two characters lex apart, exactly as
	// PipeBothStreams' do, and the consequence is visible wherever the pair
	// stands somewhere the grammar refuses it: `;& echo hi` names `;` in
	// bash 3.2 and `;&` in bash 5.3. Turning this flag and PipeBothStreams
	// off the bash preset is the whole of what "bash 3.2 has neither" needs
	// the grammar to say, which is measured against that build in
	// dialect/bash's TestTheGrammarSaysWhenAShellHasNeitherPipeAmpersandNorCaseFallthrough
	// (#2406).
	CaseFallthrough bool

	// CStyleFor enables `for ((init; cond; post))`. Absent from dash, where
	// the parenthesis after `for` is a syntax error.
	CStyleFor bool

	// ForArithExtraSeparators lets a C-style `for` header hold more than the
	// two separators its three expressions need: `for ((;;;))`,
	// `for ((1;2;3;4))`.
	//
	// The header is still three expressions there — everything past the
	// second `;` is part of the third, semicolons and all — so the loop
	// parses and the leftover text is refused as arithmetic, at run time and
	// only if that expression is ever reached. `for ((;;;)); do break; done`
	// therefore runs and prints nothing, where `for ((;;;)); do :; done`
	// reaches the third expression on its first pass and stops.
	//
	// Measured 2026-09-12 on `for ((;;;)); do echo body; break; done`: ksh93u+
	// and zsh 5.9.2 print `body`, and bash 5.3.15, bash 3.2.57 and the same
	// bash under argv[0] of `sh` all refuse the script with `` `;'
	// unexpected `` and run none of it. So this is an acceptance the two add
	// rather than a refusal bash has, and it is off in Core for that reason
	// (#2225).
	//
	// *Fewer* than two separators is not this flag and has no flag: every
	// shell in the panel refuses `for (())`, `for ((;))` and `for ((i=0))`,
	// which is [ErrForArithHeader]. The two were one over-acceptance here
	// until they were measured apart, and the missing refusal was unbounded
	// rather than wrong — a header with no separators has no condition, an
	// absent condition is true, and `for (()); do echo x; done` printed for
	// as long as it was left alone.
	ForArithExtraSeparators bool

	// Select enables `select name [in words] do … done`, the menu loop.
	// Absent from dash, where `select` is an ordinary word and the `do` that
	// follows it is a syntax error.
	Select bool

	// ForMultipleNames lets a `for` or `foreach` name more than one variable,
	// so the loop takes that many words from its list on every pass:
	// `for key value ( a 1 b 2 ) { … }` runs twice with the pairs. zsh alone
	// has it, and it is how a script walks a serialized key/value table.
	//
	// Separate from ForBraceBody and from ShortForm, and measured that way
	// rather than assumed: all four combinations of the name count and the
	// body spelling parse in zsh independently. `for a b in x 1 y 2; do … done`
	// works, so does `for a b ( … ) { … }`, so does `for a b ( … ); do … done`,
	// and so does `for a b` with no list at all, which walks the positional
	// parameters in groups. This flag is the name count and nothing else.
	//
	// `select` does **not** take it — measured, `select a b (x y) { … }` is a
	// parse error in the shell that has every other spelling — so the loop
	// whose header is a `for`'s parts company here. `foreach` does take it.
	//
	// **Names are taken greedily, and that is subtractive.** Every word after
	// the first is another name until the header ends: at `in`, at `(`, at a
	// separator, at `do` or at `{`. So a short body may not follow the names
	// directly — `for a print -r -- "[$a]"` is a parse error near `-r` in
	// zsh, because `print` was read as a second name — where a header that
	// ended itself still takes one: `for a b ( 1 2 ) print "$a$b"` runs. A
	// dialect with ShortForm and not this flag keeps the older, wider
	// reading, in which a word after the name begins the body.
	//
	// A name must be a plain unquoted name. `for a "b" ( … )` and
	// `for a $n ( … )` are parse errors in zsh, so the words are not
	// expanded and not unquoted before being read as names.
	ForMultipleNames bool

	// ForNameMayBeQuoted lets a loop's variable be written with quoting or an
	// escape in it, the quoting being removed before the word is read as a
	// name: `for "i" in a b` binds `i`.
	//
	// ksh93u+ alone accepts it. Measured 2026-09-06, `env -i
	// PATH=/usr/bin:/bin` with a scratch HOME, from a script file and through
	// `-c`: `for "i"`, `for 'i'`, `for i""`, `for "i"x` and `for \i` all run
	// there and all five are refused by bash 5.3.15, the same binary as `sh`,
	// bash 3.2.57, dash and zsh 5.9.2. So the escape travels with the quotes
	// rather than being a question of its own, and the flag is one bit.
	//
	// It is the *quoting* half only. A name coming out of an expansion —
	// `for $n`, `for ${n}`, `for "$n"`, `for $(echo n)` — is refused by all
	// six columns including this one, so that half is core and lives in
	// [Parser.forName] rather than here (#1076).
	ForNameMayBeQuoted bool

	// ForNameMayBeAPositionalParameter lets a loop's variable be a run of
	// digits — `for 1 in a b` — which sets the positional parameter of that
	// number on each pass rather than a variable with a digit for a name.
	//
	// One shell in the panel has it, and seven of the completion functions it
	// ships are written with it. Measured 2026-09-15, each probe in a script
	// file of its own:
	//
	//	| probe                    | zsh 5.9.2 | bash 5.3 | bash-as-sh | bash 3.2 | ksh93u+ | dash | ash |
	//	| `for 1 in a b`           | `a` `b`   | refused  | refused    | refused  | refused | refused | refused |
	//	| `for 1 2 in a b c d`     | `a-b` `c-d` | refused | refused   | refused  | refused | refused | refused |
	//	| `set -- p q r; for 1;`   | `p` `q` `r` | refused | refused   | refused  | refused | refused | refused |
	//
	// The three bash columns call it `` `1': not a valid identifier ``, ksh93
	// `invalid variable name`, dash and BusyBox ash `bad for loop variable`.
	// So no column but one has it and this is a dialect's grammar.
	//
	// **Digits and nothing else**, measured in the same run: `for 0`, `for
	// 12` and `for 01` are all taken, `for 1x` is `` parse error near `1x' ``
	// and `for @` is `` parse error near `@' ``. So it is a positional
	// parameter's *number* rather than "a name the core would refuse", which
	// is why the flag admits one shape and not a class.
	//
	// The parameter it sets is the real one: after `set -- p q; for 1 in a b;
	// do :; done`, `$1` is `b` and `$2` is still `q`.
	ForNameMayBeAPositionalParameter bool

	// ForNameCheckedWhenTheLoopRuns makes a `for` or `select` whose variable
	// is not a name **parse**, with the word carried on the clause and the
	// complaint raised when the loop is reached.
	//
	// A stage and not a wording, which is the point: `bash -n` accepts a
	// script this refused. Measured 2026-09-06 and re-measured 2026-09-07,
	// `env -i PATH=/usr/bin:/bin` with a scratch HOME, over a script file
	// holding `n=x`, `for $n in a b; do echo body; done` and
	// `echo "reached-after st=$?"`:
	//
	//	shell        -n over that file   a run
	//	bash 5.3.15  accepts, silent, 0  the complaint, then reached-after st=1
	//	bash-as-sh   accepts, silent, 0  the complaint, and stops at 2
	//	bash 3.2.57  accepts, silent, 0  the complaint, then reached-after st=1
	//	dash         refuses, 2          the complaint, and stops
	//	ksh93u+      accepts, silent, 0  the complaint, and stops at 1
	//	zsh 5.9.2    refuses, 1          the complaint, and stops
	//
	// So four of the six parse it and `for 1x` behaves identically in every
	// column — the expansion is not what makes the difference, which is why
	// this is one flag and not one per spelling.
	//
	// A syntax check is what a CI job runs, so a script that works reported
	// as broken is the visible cost; and stopping where bash carries on is
	// the worse of the two remaining directions, because the output goes
	// missing rather than coming out wrong (#1110).
	//
	// What happens when the loop *is* reached is not this flag —
	// interp.Semantics.ForNameWhenTheLoopRuns — because three answers among
	// the two shells that get here is a behavior question and not a grammar
	// one.
	ForNameCheckedWhenTheLoopRuns bool

	// ForNonWordIsANameError judges whatever stands in the loop-variable
	// position as a *name*, even when it is a token that could never be a
	// word at all — the end of the input, a newline, a `;`.
	//
	// dash alone does that, and it is the whole of the difference: it answers
	// `for`, `for` with a newline after it, `for ;` and `for ; in a b` with
	// the one sentence it gives every bad loop variable. The other three ask
	// the grammar first, so a token that is not a word is refused as a token
	// and never reaches the name check:
	//
	//	              dash                    bash            ksh93            zsh
	//	`for`         Bad for loop variable   `newline'       `for' unmatched  near `for'
	//	`for` NL do   Bad for loop variable   `newline'       `newline'        near `\n'
	//	`for ;`       Bad for loop variable   `;'             `;'              near `;'
	//	`for 1x in a` Bad for loop variable   not a valid identifier — the name check
	//
	// Measured 2026-09-07, `env -i PATH=/usr/bin:/bin` with a scratch HOME,
	// through `-c`, from a script file and on standard input; the panel gives
	// the same answer on all three routes, so this is not a route question
	// (#1319). The last row is the control: a word that is present and is not
	// a name is the [ErrForName] case in every column including this one, and
	// stays there.
	//
	// The status follows the classification rather than being set beside it:
	// a grammar failure carries the dialect's parse-error status — bash 2,
	// ksh93 3, zsh 1 — where ErrForName carries ForNameStatus, so calling a
	// bare `for` a bad name answered 1 in three dialects that answer 2, 3 and
	// 1 for every other refused line.
	ForNonWordIsANameError bool

	// ForNameEndOfInputIsANewline makes the input running out where a loop's
	// variable belonged read as a newline standing there, rather than as a
	// construct left unfinished.
	//
	// It is the other half of the three-way split [ForNonWordIsANameError]
	// begins, and the two together carry the three answers the panel gives a
	// bare `for`: dash calls it a bad loop variable, bash calls it an
	// unexpected `newline`, and ksh93 and zsh give the same unterminated
	// wording they give a bare `while` — ``for' unmatched`` and ``parse error
	// near `for'``.
	//
	// bash's answer is a fact about where its input ends. It terminates what
	// it reads with a newline, so a position that *accepts* newlines swallows
	// that one and reports the end of the file — a bare `while` is `unexpected
	// end of file from `while' command on line 1` there, the same shape as
	// every other unfinished construct. The loop-variable position accepts no
	// newline, so the newline is what is left over and the newline is what it
	// names. The same fact is why it puts the end of input on the line *after*
	// the text; see Error.EndLine, which is the half of it already recorded.
	//
	// Consulted only where a newline could not have stood, which is why it is
	// one loop's flag rather than a claim about every failure: everywhere else
	// the newline is taken and the question never arises.
	ForNameEndOfInputIsANewline bool

	// ForBraceBody lets a `for` or `select` loop take a brace group where
	// `do … done` stands: `for ((;;)) { echo hi; break; }`, and equally
	// `for i in a b; { echo "$i"; }`. Absent from dash, which is the only
	// panel shell that refuses it.
	//
	// It belongs to those two loops and to nothing else. `while cond { … }`,
	// `until cond { … }` and `if cond { … }` are refused by every shell in
	// the panel, which is what makes this a production of the loop rather
	// than a general rule about bodies — and what makes it easy to miss.
	//
	// The separator before the brace is not optional in the list form, and
	// not for a reason about this flag: `for i in a b { … }` reads `{` as
	// another *item*, so the loop then meets `}` where `do` belongs. Only
	// the C-style header ends itself, which is why that one form takes the
	// brace with nothing between.
	//
	// A dialect with ShortForm widens the *body* half of this to any single
	// command, and adds `while` and `until` to the loops that may take one.
	ForBraceBody bool

	// ShortForm is the family of compound commands whose body may be written
	// without the words that ordinarily open and close it. One shell in the
	// panel has it; the other four refuse every shape below.
	//
	// It was called ShortLoop, and the name was the reason its coverage
	// stopped where it did: `if` is not a loop, and the rule the flag stands
	// for has nothing to do with looping (#827).
	//
	// Two productions, and they are one flag because they are one feature —
	// a loop header that has ended may be followed by its body directly:
	//
	//	while (( i < 2 )) echo $i        a body of one command
	//	while (( i < 2 )) { … }          which may be a brace group
	//	until [[ -n $x ]] { … }          and `until` equally
	//	for i (a b) { echo $i }          a parenthesized word list
	//	for i (a b) echo $i              with the same short body
	//	select x (a b) { … }             and `select`, whose header is a for's
	//	while false                      the body omitted altogether
	//
	// **The header has to end itself, and a separator is the opposite of
	// help.** `while true { … }` is a syntax error because `true` is a simple
	// command and `{` is another of its words; `while (( i < 2 )) { … }`
	// works because `(( … ))` closes. And `while true; { … }` is not this
	// production at all — the `;` continues the *condition list*, so the
	// brace group becomes the last command tested and the body is empty.
	// That is measurable rather than a reading: `i=0; while (( i<2 )); {
	// echo $i; i=$((i+1)) }` counts up forever, where the body reading
	// would print 0 and 1 and stop. The one-command body is what makes the
	// omitted body reachable, so the two are not separable.
	//
	// For a `for`, "ended" means the parenthesized list, or no `in` clause
	// at all. `for i in a b` still needs a separator or `do`, which is the
	// same rule that makes `for i in a b { … }` read `{` as another item.
	//
	// **An `if`'s arms choose the form one at a time.** The `{` is what
	// makes an arm short, and the first arm not written that way puts the
	// rest of the construct in the long form — where there is a `fi`, and it
	// is required. So `if (( 0 )) { echo A } else echo B; fi` runs and
	// `if (( 0 )) { echo A } else echo B` is refused for want of the `fi`,
	// while `else { echo B }` owes none and refuses one written anyway. A
	// newline before that brace decides nothing, an `else` having no
	// condition for one to end; a newline after an `elif`'s condition
	// decides everything, that being the long form's own rule. #1372 read
	// the refusal of an `else` with nothing after it as a rule about empty
	// arms, which would have refused the first shape here too.
	//
	// **And the end of the chain is not an arm.** A short `if` whose last
	// arm is a brace body and which has no `else` may be closed with one
	// `fi` written anyway: `if (( 1 )) { echo A } fi` runs, and so does the
	// same line with an `elif` chain in front of it. It is the one place the
	// production's two body spellings part — `if (( 1 )) (( 2 )) fi` is
	// refused — and it is `if` alone, the short loops taking no `done`. See
	// [Parser.redundantFi], which has the whole measured table. #2242.
	//
	// **A short body that took its separator took the construct's.** `if
	// (( 1 )) echo A; else echo B` is refused with or without a `fi`: the
	// `;` belongs to `echo A` and a short body's separator is the whole
	// statement's, so the `if` ended and nothing is left for the `else`. A
	// brace body takes no separator — not its own, and not one an inner
	// short form took inside it.
	//
	// A production of the grammar and not of the printer: a body that was
	// written short is printed as `do … done`, which parses to the same tree
	// under any dialect. Only an *omitted* body has no long spelling, so
	// that one is printed back short.
	ShortForm bool

	// Repeat is `repeat N`, a loop over a count rather than over a list or a
	// condition. One shell in the panel has it; the other four read the word
	// as an ordinary command name.
	//
	// It is a separate flag from ShortForm because the two are separate
	// questions — a shell could have the construct and spell its body only
	// as `do … done` — and because the word is a keyword only where a
	// command may begin: `repeat=5` is an ordinary assignment there.
	Repeat bool

	// Foreach is `foreach name (a b) … end`, the same loop a `for` is under
	// a different pair of words. One shell in the panel has it.
	//
	// The list is the parenthesized one ShortForm already reads, or an `in`
	// list, and `for name (a b); …; end` is refused — measured — so the
	// terminator belongs to the opening word rather than to the list.
	//
	// **`end` is not the whole of what it adds**, which this said until it
	// was measured against a shipped function that writes the other
	// spelling. All of `foreach c (a b); do … done`, `foreach c (a b) do …
	// done`, `foreach c (a b) { … }` and `foreach c in a b; do … done` run
	// on zsh 5.9.2 (2026-09-15). The closers pair rather than mixing:
	// `foreach c (a b); do … end` is refused there, as `for` closed by
	// `end` is.
	Foreach bool

	// TryAlways is `{ … } always { … }`: a brace group whose second half runs
	// however the first half ended. One shell in the panel has it; the other
	// five call the word a syntax error where it stands.
	//
	// **It is positional and not a reserved word, which is the whole of the
	// grammar.** Measured 2026-09-07 against zsh 5.9.2: `always` alone is
	// `command not found`, `always() { :; }` defines a function, `echo
	// always` prints it and `x=always` assigns it — so the word may not join
	// stopWords or reservedWords. It is read only where a brace group has
	// just closed, and nothing else there will do:
	//
	//	{ echo t; } always { echo a; }     the construct
	//	{ echo t; }; always { … }          `always` is a command name again
	//	{ echo t; }
	//	always { … }                       a newline is a separator too
	//	{ echo t; } "always" { … }         quoting takes the keyword away
	//	{ echo t; } > /dev/null always {}  a redirection ends the try half
	//	{ echo t; } always echo a          the second half must be a group
	//	{ echo t; } always {} always {}    and there is exactly one of them
	//	if true; then :; fi always { … }   no other compound command takes it
	//	for i in a; { :; } always { … }    including a loop's brace body
	//	f() { :; } always { … }            nor a function definition's
	//
	// Every one of those is a parse error in the shell that has the
	// construct, and the first two run there as two commands. So the flag
	// gates one production hanging off the brace group and never the lexer.
	//
	// Redirections belong to the whole construct rather than to either half:
	// `{ echo t; } always { echo a; } > /dev/null` prints nothing at all.
	// Nesting works in both halves.
	TryAlways bool

	// AnonymousFunction is `() { … }` and `function { … }`: a function with
	// no name, defined and run where it stands, with the words after it as
	// its positional parameters. One shell in the panel has it; in the other
	// four a `(` where a command begins opens a subshell and `()` is a
	// syntax error.
	AnonymousFunction bool

	// AppendAssign enables `name+=value`, which appends rather than
	// replacing. Absent from dash, where `x+=b` is a command called `x+=b`.
	AppendAssign bool

	// PositionalAssignment lets an assignment's *name* half be a run of
	// decimal digits, so `1=abc` writes the first positional parameter.
	//
	// A grammar flag rather than an axis, because the difference is in what
	// the word *is* and not in what is then done with it. Measured
	// 2026-09-07 with `set -- x; 1=abc; echo $1`: bash 5.3.15, that binary
	// as `sh`, bash 3.2.57, dash and ksh93 all take the word as a command
	// name and answer `1=abc: command not found` at 127, and zsh 5.9.2
	// prints `abc`. The two readings are told apart by what the word is
	// *subjected to*, which is the discriminating probe: `set -- x; 1=*`
	// leaves `$1` holding a literal `*` in zsh — an assignment's value is
	// neither globbed nor split — where bash globs the whole word and
	// complains about `1=*`, and `v="a b"; 1=$v` leaves one parameter
	// holding `a b` in zsh where bash splits and complains about `1=a`. A
	// word that is expanded one way in one shell and another way in another
	// is a difference in the parse, so it belongs here.
	//
	// Only a bare run of digits. A subscript on one is not this construct:
	// `1[0]=v` is a command name in zsh too, and globs as one. And the
	// grammar is where it ends — a *declaration* still refuses the digits it
	// admits, because `local 1=abc`, `typeset 1=(a b)`, `export 1=x` and
	// `readonly 1` are each `not an identifier: 1` in the same shell. So the
	// declaration path reads the operand and the utility refuses the name,
	// which is where that complaint is already worded.
	//
	// Off, `1=abc` is a command name exactly as it was, which is what keeps
	// the five refusing columns byte-identical.
	//
	// The plugin manager in `~/.zi` writes it: `.zi-any-to-user-plugin` and
	// `.zi-formatter-pid` both assign their own positionals, so a startup
	// that reads them printed twelve `no such file or directory:
	// 1=username/reponame` lines and never reached a prompt (#1438).
	PositionalAssignment bool

	// DottedName makes `.` a name character, so `${.sh.version}` reads and
	// `.foo=1` is an assignment rather than a command name.
	//
	// **ksh93 alone.** It is one lexical rule and not two, which is the
	// question #2620 asked and the probes answered. Measured on ksh93u+
	// 2012-08-01, 2026-09-13, `-c`:
	//
	//	${.sh.version}   Version AJM 93u+ 2012-08-01
	//	${.foo}          empty, status 0 — no `.sh` about it
	//	${.}             empty, status 0 — the dot alone is a name
	//	${x.y}           empty, status 0, with `x=1` set
	//	.foo=1; ${.foo}  1 — an assignment, and it reads back
	//	typeset .x=3     accepted, and `${.x}` is 3
	//	for .x in 1 2    accepted, and the body sees `${.x}`
	//	$.foo            the four characters, with `.foo` set — *not* an
	//	                 expansion, so the dot is a rule about names and
	//	                 not about what follows a `$`
	//
	// So a dot is an ordinary name byte wherever a name is read, it may
	// *begin* one, and the `.sh` namespace and a compound variable's member
	// are the same grammar reached twice rather than two constructs. What
	// distinguishes them is only what the interpreter has stored under the
	// name.
	//
	// The other five columns call `${.sh.version}` a bad substitution at
	// *run* time — bash 5.3.15, that binary as `sh`, bash 3.2.57, zsh 5.9.2
	// and BusyBox ash all run the command before it — so this is a grammar
	// flag and not an axis: nobody else has a reading of the construct to
	// disagree with.
	DottedName bool

	// TildeGroup makes a `(` that stands immediately after a `~` part of the
	// word rather than the operator it otherwise is, which is what lets
	// ksh93's `~(…)` pattern-modifier prefix be written at all.
	//
	// **ksh93 alone**, and it is a rule about the *lexer* rather than about
	// patterns: the group belongs to the word wherever the word stands, and
	// only a word being matched as a pattern then reads the letters.
	// Measured on ksh93u+ 2012-08-01, 2026-09-13, `-c`:
	//
	//	echo ~(E)abc                 ~(E)abc     an ordinary word, literal
	//	x=~(Z)abc; echo "$x"         ~(Z)abc     and so is a value
	//	echo a~(x)b                  a~(x)b      mid-word too
	//	[[ abc == ~(E)a.c ]]         matches     a pattern reads them
	//	[[ abc == a~(E)b.? ]]        matches     mid-pattern as well
	//	case abc in ~(E)^a.c$)       matches
	//	s=aXbXc; ${s//~(E)X/-}       a-b-c
	//
	// Without it the `(` ends the word and the shell reports a syntax error
	// at the paren — which is what the other five columns do, unanimously
	// and at parse time: bash 5.3.15, that binary as `sh`, bash 3.2.57, dash
	// and BusyBox ash all refuse the file. zsh is the one that neither
	// refuses nor honors: there `~` is the exclusion operator and `~(E)abc`
	// is a pattern that reads and does not match, which is a reading of its
	// own and not this construct.
	//
	// So a grammar flag, and gated: a `cmd/bash` that took the `(` would
	// accept what real bash refuses. See interp's tildeModifier for which
	// letters are honored once the group has been read (#2621).
	TildeGroup bool

	// CurrentShellSubstitution reads `${ cmd;}` as a command substitution
	// that runs in the current shell. bash 5.3 and ksh93 have it; dash and
	// zsh call it a bad substitution.
	//
	// The space after the brace is load-bearing and is the whole of the
	// grammar: `${x}` is a parameter and `${ x}` is a command.
	CurrentShellSubstitution bool

	// BraceProgramBodyEnd says which `}` ends that body. The two shells that
	// have the construct disagree, and both of them refuse a body with no
	// terminator in front of the brace. The values carry the measurements;
	// see [BraceProgramBodyEnd].
	//
	// Read only where CurrentShellSubstitution is on — where it is off there
	// is no such body to end — and it governs [Dialect.ReplySubstitution]'s
	// body too, which is the same list with a `|` in front of it.
	BraceProgramBodyEnd BraceProgramBodyEnd

	// ReplySubstitution reads `${| cmd;}` as a command substitution that runs
	// in the current shell and expands to whatever the body left in `$REPLY`
	// rather than to what it printed. bash 5.3 has it and nothing else in the
	// panel does.
	//
	// **Not the same lexical rule as the form above, and measured rather than
	// assumed.** That one turns on a *blank* after the brace and this one
	// turns on a `|` adjacent to it, and the two do not mix — measured
	// 2026-09-13 on bash 5.3.15:
	//
	//	${|REPLY=hi; }    hi             no blank needed after the `|`
	//	${| REPLY=hi; }   hi             a blank after it is ordinary body
	//	${ | REPLY=hi; }  syntax error   `|' unexpected, looking for `}'
	//	${echo hi; }      bad substitution
	//	${|}              empty, status 0
	//
	// So the third row is the discriminating one: with a blank first the body
	// has already begun and a `|` opening it is a pipeline with nothing on its
	// left. A single flag reading "brace, then a blank *or* a pipe" would
	// accept it.
	//
	// Separate from CurrentShellSubstitution because the panel separates them:
	// ksh93 has the blank form and answers `` `|' unexpected `` to this one
	// (#2656).
	ReplySubstitution bool

	// SubshellSubstitution reads `${(list)}` as a command substitution whose
	// body is the parenthesized subshell. ksh93 has it and nothing else in
	// the panel does: bash 5.3 and bash 3.2 call it a bad substitution, dash
	// and BusyBox ash the same, and zsh reads the parenthesis as its
	// expansion flags and complains about the letters inside.
	//
	// **Its extent is the parenthesis and not a command list, which is
	// measured rather than inferred from the four lines that work.** #2615
	// filed it as the blank form above with a subshell for a body — the
	// reading the isolation invites, since `${(cd /tmp; pwd)}` leaves the
	// caller's directory alone exactly as a subshell would. A list would go
	// on past the `)`, and it does not; measured 2026-09-13 on ksh93u+
	// 2012-08-01, in a script so that `ksh -n` can say which stage refused:
	//
	//	${(echo a)}              a
	//	${(echo a);}             `}' unexpected      refused while reading
	//	${(echo a) ;}            `}' unexpected      refused while reading
	//	${(echo a); echo b;}     `}' unexpected      refused while reading
	//	${(echo a)b}             `b}' unexpected     refused at the run
	//	${(echo a}               `(' unmatched       refused while reading
	//
	// So the `}` has to sit directly behind the matching `)` with nothing
	// between them, not even a blank, and the read-time refusals are what
	// say so: a body that were a list would take `;` and a second command
	// the way the blank form does. The blank form is the contrast, and it is
	// a list — `${ (echo a); echo b;}` runs both there.
	//
	// A separate flag from CurrentShellSubstitution for the reason
	// ReplySubstitution is one: the panel separates them (bash 5.3 has the
	// blank form and refuses this) and so does the grammar (that body ends
	// where a list ends, this one where its parenthesis does).
	//
	// `${((` is **not** this construct and is deliberately left out: it is
	// ksh93's braced arithmetic, `${((1+2))}` is 3 there and `${((echo hi))}`
	// an arithmetic syntax error, so the two adjacent parens are the whole of
	// the discriminator — `${( (1+2) )}` runs `1+2` as a command and reports
	// it not found. That spelling is refused here as it was before this flag,
	// and it is a construct of its own rather than a corner of this one.
	SubshellSubstitution bool

	// FlagGroupRefusedAtExpansion defers the refusal of a `${(…)…}` an
	// expansion flag group was written in to the moment the word expands,
	// in a grammar that has no such group and refuses every other unreadable
	// expansion while reading.
	//
	// The two halves are what make it a flag of its own rather than a
	// reading of BadSubstitutionAtParseTime. Measured 2026-09-18 on ksh93u+
	// 2012-08-01, a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`
	// with standard input on /dev/null, each body written as
	// `echo one; echo "[<body>]"; echo two`:
	//
	//	${(f)x}     one, then `syntax error at line 2: `x}]' unexpected`, 3
	//	${(qq)x}    one, then the same shape
	//	${(j:|:)x}  one, then a refusal naming a token of its own
	//	${%%%}      refused before `one` runs
	//	${~x}       refused before `one` runs
	//	${=x}       refused before `one` runs
	//	${+x}       refused before `one` runs
	//
	// So the deferral is the *group* and not the brace: four other
	// unreadable expansions in the same shell stop the input where it is
	// read, and only the parenthesis waits. The control that says it is the
	// expansion rather than the line is a group in a branch never taken —
	// `if false; then echo "${(f)x}"; fi` writes nothing and exits 0 there,
	// where `if false; then echo "${%%%}"; fi` is still refused.
	//
	// The refusal itself does not move: the same failure the parser builds
	// today, kind, token and word tail alike, is carried on the node and
	// written when the word expands. See ParamExpr.RefusedAtExpansion, and
	// Error.FlagGroupWordTail for the text that shell quotes back.
	//
	// Read only where ParamExpansionFlags is off and
	// BadSubstitutionAtParseTime is on, which is the one state that refuses
	// a group while reading at all.
	FlagGroupRefusedAtExpansion bool

	// BracedArithmeticExpansion reads `${((expr))}` as an arithmetic
	// expansion. ksh93 has it and nothing else in the panel does: the three
	// bash columns and dash call it a bad substitution, BusyBox ash words
	// that as a syntax error, and zsh reads the parenthesis as its expansion
	// flags and complains about the letters inside.
	//
	// **Arithmetic and not [Dialect.SubshellSubstitution] with a subshell in
	// it**, which is measured rather than assumed. The discriminator is one
	// space: `${((echo hi))}` is `echo hi: arithmetic syntax error` there,
	// where a subshell body would have run the command and printed `hi`, and
	// `${( (1+2) )}` — the same characters with the parens apart — reports
	// `1+2: not found`, a command by that name. Measured 2026-09-13 on
	// ksh93u+ 2012-08-01:
	//
	//	${((1+2))}        3
	//	${(( 1+2 ))}      3
	//	${(((1+2)))}      3
	//	${((1+2))}x       3x
	//	${((echo hi))}    echo hi: arithmetic syntax error
	//	${((1+2)) }       `end of file' unexpected
	//
	// The last row is why the `}` is required directly behind the `))` with
	// nothing between them, which is the same shape [Dialect.SubshellSubstitution]
	// has and the reason the two adjacent parens can settle the spelling
	// before anything else is read (#2725).
	BracedArithmeticExpansion bool

	// ConditionOperandMayOpenWithAGroup lets a `(` at the front of a
	// condition's **third word** belong to that word rather than being an
	// operator. `[[ 9 -gt ( 1 + 2 ) ]]` and `[[ -prefix 1 (f|ht)tp:// ]]`
	// are the two shapes, and four of the completion functions one shell
	// ships are written with them.
	//
	// Measured 2026-09-15, each probe in a script file of its own:
	//
	//	| probe                 | zsh 5.9.2 | bash 5.3 | bash-as-sh | bash 3.2 | ksh93u+ | dash | ash |
	//	| `[[ 9 -gt ( 1 ) ]]`   | true      | refused  | refused    | refused  | refused | refused | refused |
	//	| `[[ 2 -gt ( 1 + 2 ) ]]` | false   | refused  | refused    | refused  | refused | refused | refused |
	//	| `[[ -pfx 1 (a\|b)c ]]` | parsed   | refused  | refused    | refused  | refused | refused | refused |
	//
	// The three bash columns say `` unexpected argument `(' to conditional
	// binary operator ``, ksh93 `` `(' unexpected ``, and dash and BusyBox
	// ash — which have no `[[ ]]` at all — name the `(` where they wanted a
	// `then`. One column against six, so this is a dialect's grammar.
	//
	// **The position is the whole of the rule**, measured in the same run:
	// `[[ ( 1 -gt 0 ) ]]` is the condition's own grouping paren at the first
	// word, `[[ -n ( a ) ]]` and `[[ -pfx ( a ) ]]` are `` parse error near
	// `(' `` at the second, `[[ -pfx 1 ( a ) ]]` parses at the third, and
	// `[[ -pfx 1 2 ( a ) ]]` is refused again at the fourth. A `!` or a
	// connective starts the count over.
	//
	// The group is not a *pattern* — that is [Lexer.inPattern], which is set
	// for `==`, `=` and `!=` and was already right for them. What this adds
	// is the same lexing for every other operator, and what becomes of the
	// word afterwards is the operator's business: `[[ 9 -gt ( 1 -gt 0 ) ]]`
	// hands `( 1 -gt 0 )` to the arithmetic evaluator, which complains about
	// it.
	ConditionOperandMayOpenWithAGroup bool

	// PidBraceGroupIsText makes a `{ … }` written immediately after `$$` a
	// run of characters: a blank, a newline or an operator inside it is text
	// rather than a separator, and the braces are a brace *list* nowhere.
	//
	// One shell in the panel has it, and one of the completion functions it
	// ships is written with it — by accident, on the evidence, since the line
	// carries `$${(s<,>)…}` where `${(s<,>)…}` was plainly meant and "works"
	// only because the stray `$` turns the rest into text. Measured
	// 2026-09-15, each probe in a script file of its own, printed one
	// argument per `[%s]` so the word count is visible:
	//
	//	| probe            | zsh 5.9.2    | bash 5.3 | bash-as-sh | bash 3.2 | ksh93u+ | dash | ash |
	//	| `$${a b}`        | `{a b}`      | `{a` `b}` | `{a` `b}` | `{a` `b}` | `{a` `b}` | `{a` `b}` | `{a` `b}` |
	//	| `$${a;b}`        | `{a;b}`      | refused  | refused    | refused  | refused | refused | refused |
	//	| `$${a>b}`        | `{a>b}`      | redirects | redirects | redirects | redirects | redirects | redirects |
	//	| `$${a{b,c}d}`    | `{abd}` `{acd}` | `{a{b,c}d}` | `{a{b,c}d}` | `{a{b,c}d}` | `{a{b,c}d}` | `{a{b,c}d}` | `{a{b,c}d}` |
	//
	// Six columns against one, so this is a dialect's grammar rather than a
	// bug. The `>` row is what says the run is lexical and not a quoting
	// rule: every other column writes the word into a file called `b}`.
	//
	// **The outer pair alone is text**, which the fourth row is there to
	// pin: a brace pair *inside* the run still expands, so a reading that
	// emitted the whole run as one literal span would be wrong in the other
	// direction. Everything else inside expands as it always did — `$${a$(echo
	// X)b}` is `{aXb}` and `v=V; $${x${v}y}` is `{xVy}` — so this is about
	// the braces and the separators, not about expansion.
	//
	// **And the outer pair is text for the list reading only.** A range
	// written straight into it still fires: `$${1..3}`, `$${a..c}` and
	// `$${1..5..2}` all expand there, where `$${a,b}`, `$${1,2}`,
	// `$${a,1..3}` and `$${1..2 3}` are all one word. That is [Span.PidBrace]
	// and the brace expander rather than anything here, but it belongs beside
	// the rest of the rule: a reading that made the whole pair inert passes
	// the table above and loses the ranges, and did.
	//
	// **`$$` and nothing else, touching the brace.** `$!{a,b}`, `$?{a,b}`,
	// `$-{a,b}` and `$#{a,b}` all brace-expand there, and so do `x{a,b}`,
	// `$x{a,b}` and `${x}{a,b}`; `$$x{a b}` and `$$ {a b}` are refused along
	// with the rest of the panel, because the `}` left over is the reserved
	// word. It survives `emulate sh` and `emulate ksh` and the `sh` argv0,
	// so it is the lexer rather than an option.
	//
	// An unmatched one is refused rather than left as text — `closing brace
	// expected`, reported at the end of the input, because the run swallows
	// every line after it looking for the match.
	PidBraceGroupIsText bool

	// CaseHeaderSpansSeparators lets a `;` stand in a `case` header wherever
	// a newline may: between the subject and the `in`, and after the `in`.
	//
	// One shell in the panel takes it, and nine of the completion functions
	// zsh ships write the first spelling. Measured 2026-09-15, each probe in
	// a script file of its own, against a `case` with one arm:
	//
	//	| probe          | zsh 5.9.2 | bash 5.3 | bash-as-sh | bash 3.2 | ksh93u+ | dash | ash |
	//	| `case x; in`   | runs      | refused  | refused    | refused  | refused | refused | refused |
	//	| `case x ; ; in`| runs      | refused  | refused    | refused  | refused | refused | refused |
	//	| `case x;⏎in`   | runs      | refused  | refused    | refused  | refused | refused | refused |
	//	| `case x in;`   | runs      | refused  | refused    | refused  | refused | refused | refused |
	//	| `case x;; in`  | refused   | refused  | refused    | refused  | refused | refused | refused |
	//	| `case x in ;;` | refused   | refused  | refused    | refused  | refused | refused | refused |
	//	| `case x & in`  | refused   | refused  | refused    | refused  | refused | refused | refused |
	//
	// So it is the `;` and not the whole separator family: `;;` is the arm
	// terminator and `&` is a job-control operator, and neither becomes a
	// newline here. The last three rows are why this is one flag and not
	// "anything that ends a statement".
	//
	// Separate from [Dialect.SeparatorWhereACommandBelongs], which is about a
	// `;` where a *command* belongs: no command belongs in a `case` header,
	// so that flag says nothing about this and a shell could have either
	// without the other.
	CaseHeaderSpansSeparators bool

	// CasePatternAcceptsOperator lets an operator stand where a case pattern
	// belongs, which produces an arm with no patterns at all.
	//
	// Measured rather than inferred from the error it causes, because the
	// error is not the whole of it: `case a in & ) echo hit;; *) echo miss;;
	// esac` *parses and runs* in dash, and prints miss — the `&` is consumed
	// and the arm it opens matches nothing, not even `&` and not even the
	// empty string. `&a )` then fails at the word and `a& )` at the `&`, so
	// what dash accepts is one operator where the pattern list would start
	// and nothing else.
	//
	// The other three reject it outright, which is why `;;&` in a dialect
	// without that terminator reaches a different diagnosis there: dash is
	// already past the `&` and complaining about the `esac` where the `)`
	// should be.
	CasePatternAcceptsOperator bool

	// CasePatternMayBeEmpty lets a `case` arm's pattern list carry a pattern
	// written as nothing, which then matches only the empty string:
	// `(|https|git|ftp)` is the idiom for "one of these schemes, or none",
	// and it is what a widely installed zsh library's own startup path is
	// written with.
	//
	// Measured 2026-09-06 with `-n` over a script file, because the question
	// is whether it parses. zsh 5.9.2 accepts it; bash 5.3.15, the same
	// binary as `sh`, bash 3.2.57, ksh93u+ and dash all refuse, at three
	// statuses and in three wordings, and each blames a different token
	// depending on where the emptiness is.
	//
	// Every place a separator can put one is allowed: `(|a|b)`, `(a||b)`,
	// `(a|b|)`, `(|)` and `(||)` all parse there, with or without the arm's
	// optional open paren.
	//
	// So is the whole list, where the arm's parentheses are there to hold
	// it. `case "" in ( ) echo em;; (*) echo star;; esac` prints `em` on zsh
	// 5.9.2 and prints `star` for a subject of one blank, measured
	// 2026-09-12 — the empty string, not the blank, that dialect trimming
	// blanks either side of a list. Written *without* the parentheses there
	// is nowhere for an empty list to be and `case a in ) …` is refused.
	//
	// `()` is refused as well, and this file said for a while that the
	// reason was the missing separator. It is not: the pair is one token to
	// the same dialect — see [Dialect.EmptyParensAreOneToken] — so the `(`
	// never opens an arm, and the two characters with a blank between them
	// are the line above (#1111).
	//
	// It is a grammar flag and not a matching rule. A group with an arm that
	// matches nothing already stands for nothing in every shell that has the
	// construct at all — `@(|a)b` matches `b` in bash and ksh93 alike — so
	// what divides the panel here is only whether the pattern *list* may
	// have such an alternative written into it.
	CasePatternMayBeEmpty bool

	// CaseTerminatorIsAPatternAfterTheHeader makes `esac` an ordinary word
	// where the *first* arm's pattern list begins, so a `case` whose subject
	// list is written on one line has no terminator until a newline has been
	// read.
	//
	// "The header" is either opener: this shell writes `case x { … }` as well
	// as `case x in … esac` — see [Dialect.CaseBraceBody] — and the reading
	// follows the position rather than the word. `case esac { esac) echo
	// hit;; }` prints `hit` there too.
	//
	// ksh93u+ alone, and it is not the rule #773 filed it as. That issue read
	// `case x in esac` being refused there as "a case must have an arm", and
	// the discriminator says otherwise. Measured 2026-09-12, `env -i
	// PATH=/usr/bin:/bin` with a scratch HOME, over `-c` and a script file
	// alike:
	//
	//	case x in esac                    `case' unmatched     — a pattern,
	//	                                  and then no `)`
	//	case x in esac; echo done         `;' unexpected       — the same
	//	case x in⏎esac                    runs                 — a newline
	//	                                  makes it the terminator again
	//	case esac in esac) echo hit;; esac
	//	                                  prints `hit`         — so it really
	//	                                  is being read as a pattern
	//	case x in y) ;; esac) echo hit;; esac
	//	                                  `)' unexpected       — only the
	//	                                  first arm's position, not after a
	//	                                  `;;`
	//	case x in \⏎esac                  `newline' unexpected — a line
	//	                                  continuation is not a newline
	//	case x in # c⏎esac                runs                 — a comment
	//	                                  ends the line and the newline counts
	//	case esac in (esac) echo hit;; esac
	//	                                  prints `hit` in all six — a paren in
	//	                                  front takes the reservation away
	//	                                  everywhere and needs no flag
	//
	// The fourth row is what makes this additive rather than a refusal: the
	// shell *accepts* a program the other five refuse, and the one-line
	// `case x in esac` is refused as a consequence of that acceptance rather
	// than as a rule of its own. Writing it the other way round — a flag that
	// simply refused an armless `case` — would have refused `case x in⏎esac`
	// too, which ksh93 runs.
	CaseTerminatorIsAPatternAfterTheHeader bool

	// CaseBraceBody lets a `case` be written with braces in place of `in` …
	// `esac`: `case x { x) echo hit;; }`.
	//
	// **Two shells have it and they draw it differently**, which is why this
	// is a spelling rather than a bool. Measured 2026-09-12 under `env -i
	// PATH=/usr/bin:/bin` with a scratch HOME, ksh93u+ and zsh 5.9.2; dash,
	// bash 5.3, that binary as `sh` and bash 3.2 refuse every row.
	//
	//	probe                              ksh93            zsh
	//	case x { x) echo hit;; }           hit              hit
	//	case x { }                         runs             runs
	//	case x { (x) echo hit;; }          hit              hit
	//	case x { x) echo hit;; esac        `case' unmatched hit
	//	case x in x) echo hit;; }          `case' unmatched hit
	//	case esac { esac) echo hit;; }     hit              parse error at `)`
	//	case x { x) echo hit }             `case' unmatched hit
	//	case x {x) echo hit;; }            `{x' unexpected  hit
	//
	// Rows 4 and 5 are the split this field records: ksh93 **pairs** the two
	// words and zsh takes either closer after either opener. Rows 6, 7 and 8
	// are the other three flags showing through rather than anything of this
	// one's — [Dialect.CaseTerminatorIsAPatternAfterTheHeader],
	// [Dialect.CloseBraceAlwaysReserved] and [Dialect.OpenBraceNeedsNoBlank],
	// each of which one of the two shells has and the other does not.
	CaseBraceBody CaseBraceSpelling

	// CasePatternListSpansNewlines makes a newline inside a `case` arm's
	// **parenthesized** pattern list an ordinary character of the pattern
	// rather than the end of a word or a statement.
	//
	// zsh alone. Measured 2026-09-07 over a script file, with a scratch HOME
	// and ZDOTDIR:
	//
	//	case a in (a|      zsh: `m`, status 0
	//	b) echo m;; *) echo no;; esac
	//
	// and every other shell in the panel refuses the line — bash 5.3.15, the
	// same binary as `sh`, and bash 3.2.57 blame the newline, dash blames the
	// word on the next line, ksh93u+ blames the newline at line 2. Three
	// wordings and three statuses.
	//
	// What zsh matched is the *first* alternative, and the second is two
	// characters — a newline and a `b`. Four probes say so and no fewer will:
	// subject `a` gives `m`, subject `b` gives `no`, subject `""` gives `no`,
	// and a subject holding a newline before the `b` gives `m`. `functions`
	// on a function carrying the arm prints the newline back inside the
	// pattern, which is the same fact from the writing side.
	//
	// It is not a rule about the `|`. A newline anywhere inside the list is
	// text: `(a` newline `)` is the two-character pattern, so subject `a`
	// does *not* match it, and `(a` newline `|b)` puts the newline on the end
	// of the first alternative. And it needs the arm's **paren**: `case a in
	// a|` newline `b)` is a parse error in zsh too, so what opens this is the
	// parenthesis and not the position.
	//
	// This is #1083's `|` seen from the other side and a bigger claim than
	// that one. `CasePatternMayBeEmpty` lets an alternative be *written* as
	// nothing, which is a rule about the list; this says the separator does
	// not end the word at all, which reaches the lexer.
	CasePatternListSpansNewlines bool

	// CasePatternListPipeIsOnlyASeparator keeps a `|` inside a `case` arm's
	// **parenthesized** pattern list from joining the character after it into
	// a two-byte operator: the pipe is the alternation separator there and
	// nothing else.
	//
	// zsh alone, and only one operator can be written where it shows —
	// `|&`, which every other member of the panel that has the operator
	// lexes whole. Measured 2026-09-12, `-n` over a script file:
	//
	//	case a in (a|&b) …    zsh `&`      bash 5.3 / ksh93 `|&`
	//	case a in (a|&|b) …   zsh `&|`     bash 5.3 / ksh93 `|&`
	//	case a in (a|&&b) …   zsh `&&`     — the discriminator
	//	case a in a|&|b) …    zsh `|&`     — the control
	//
	// The third row is what says the rule belongs to the `|` and not to the
	// `&`: an `&&` after the separator is still one token, so it is the pipe
	// that stops reading rather than the ampersand that starts. The fourth is
	// what confines it to the parentheses — written without the arm's paren,
	// the same characters lex as `|&` in zsh too, which is the answer every
	// other column gives everywhere.
	//
	// It is a lexical rule and shows only as the token a refusal names: no
	// line that parses is read differently, because `|&` cannot stand in a
	// pattern list under either reading.
	CasePatternListPipeIsOnlyASeparator bool

	// CasePatternListSpansBlanks makes a blank inside a `case` arm's
	// **parenthesized** pattern list an ordinary character of the pattern
	// rather than the end of a word — so `(a b)` is the three-character
	// pattern and not two words, one of which nothing can be done with.
	//
	// zsh alone, and the line it is needed for is `VCS_INFO_get_data_git`,
	// which every prompt drawing a git segment autoloads: line 234 of it is
	// `(''(x|exec) *)`, a group, a blank and more pattern.
	//
	// Measured on zsh 5.9.2, 2026-09-10, `-c` under `env -i`, against
	// bash 5.3.15 (which is also `sh`), bash 3.2.57, ksh93u+ and dash:
	//
	//	case 'a b' in (a b) echo hit;; (*) echo no;; esac
	//
	//	zsh 5.9.2   `hit`, status 0
	//	bash 5.3    `` syntax error near unexpected token `b' ``, status 2
	//	bash 3.2    the same sentence, status 2
	//	ksh93       `` syntax error at line 1: `b' unexpected ``, status 3
	//	dash        `Syntax error: word unexpected (expecting ")")`, 2
	//
	// The paren is what licenses it, exactly as it licenses the newline
	// above: `case 'a b' in a b) …` is `` parse error near `b' `` in zsh too,
	// so this is a rule about the parenthesized form and not about the
	// position. That control is what separates it from "a word may follow a
	// pattern".
	//
	// The blanks are **taken verbatim**, which four probes say and no fewer
	// will: `(a b)` misses `a  b` and misses `ab`, `(a  b)` matches `a  b`,
	// and `(a<tab>b)` misses `a b` while matching `a<tab>b`. So a run is not
	// collapsed and a tab is not a space — the pattern is the source text.
	//
	// And they are text only where the pattern *continues* after them. A run
	// of blanks in front of the `|` that separates two alternatives, or in
	// front of the `)` that closes the list, still separates nothing and is
	// dropped: `( a b )` matches `a b` and misses ` a b ` and `a b `, and
	// `(a b |z)` matches `a b`. That is why this is not `isBlank` losing its
	// meaning inside the list — `(a | b)` is still two alternatives, and it
	// is two in every shell in the panel.
	//
	// The two flags compose the way the shell does. Where a newline is text
	// as well, a blank beside one is text too: `(a` blank newline blank `b)`
	// matches exactly that subject and misses `a` newline `b`.
	//
	// An operator is still an operator. `(a >b)` is `` parse error near `>' ``
	// in zsh and the blank before it is no part of any pattern, so this
	// admits the words a pattern can hold and nothing else.
	CasePatternListSpansBlanks bool

	// CasePatternRunsOutAsANewline reads the end of the input at a `case`
	// arm's pattern as the **newline** that would have ended the line, rather
	// than as the input running out inside an unfinished `case`.
	//
	// It decides only the wording of a failure, and the failure is the same
	// one either way: a pattern was begun and its `)` never arrived. What
	// differs is whether the shell blames the construct that never closed or
	// the token that stood where the `)` belonged.
	//
	// bash alone, and the way to see that it is bash's own reading rather
	// than an accident of where the text stopped is to write the same script
	// twice, once with a trailing newline and once without. Measured
	// 2026-09-13, `env -i PATH=/usr/bin:/bin` with a scratch HOME, over `-c`,
	// on `case x in x) :;; zzz` — the pattern of a second arm, begun and
	// unfinished:
	//
	//	shell        without a trailing newline              with one
	//	bash 5.3     `newline' unexpected, line 1            the same, line 1
	//	bash 3.2     the same                                the same
	//	bash as sh   the same                                the same
	//	dash         end of file unexpected (expecting ")")  newline unexpected …
	//	ash          unexpected end of file (expecting ")")  unexpected newline …
	//	ksh93        `case' unmatched                        `newline' unexpected
	//	zsh          parse error near `zzz'                  parse error near `\n'
	//
	// Five of the six columns answer differently in the two spellings, which
	// is what says they read the end of the input as the end of the input.
	// bash gives one answer to both, echoes the same source line for both,
	// and numbers both at the line the pattern is on rather than at the line
	// after — so to bash the run-out *is* the newline.
	//
	// This shell already agrees with bash where the newline is really there:
	// the token stands, the pattern position refuses it, and the bytes match.
	// So the flag is not a second wording but the same one reached from the
	// other spelling, which is why it is a token substitution rather than a
	// message.
	//
	// **It is the pattern position and nothing wider.** `for i in a b`, `if
	// true`, `while true`, `{ echo a`, `select i in a` and an arm's *body*
	// running out are all the unterminated shape in bash too, measured in the
	// same run. A flag that made the end of input a newline generally would
	// encode a rule bash does not have.
	CasePatternRunsOutAsANewline bool

	// FuncDefAtParen commits to a function definition as soon as a name is
	// followed by `(`, rather than requiring the `()` pair.
	//
	// It decides *which token* a malformed one is blamed on, which is why it
	// is a grammar flag and not a wording: `f ( x )` is "x" in bash and dash,
	// which are already inside a definition looking for `)`, and "(" in
	// ksh93, which never entered one. Reached most often through a construct
	// a dialect does not have — `[[ ( -n x ) ]]` is a definition of a
	// function called `[[` to a shell without `[[`.
	FuncDefAtParen bool

	// FuncBodyMustBeCompound refuses `f() echo hi`: bash alone wants a
	// compound command after the parens, where dash, ksh93 and zsh take a
	// simple command as a one-command body and run it.
	//
	// The `function` keyword's body is held to the same rule where the
	// dialect has both — measured 2026-09-11 over a file holding `function a`
	// and `echo B`, bash 5.3.15, bash 3.2.57 and bash-as-sh all answer
	// ``syntax error near unexpected token `echo' `` at status 2, which is
	// the sentence and the status the parenthesized form gets from them. It
	// is not the whole of the keyword form's question, though, because the
	// shell that originated the keyword is stricter still: see
	// [Dialect.FunctionKeywordBodyMustBeBraceGroup].
	FuncBodyMustBeCompound bool

	// FunctionKeywordBodyMustBeBraceGroup refuses every body after the
	// `function` keyword but a brace group — ksh93's rule, and narrower than
	// [Dialect.FuncBodyMustBeCompound] rather than a spelling of it: a
	// compound command is enough for the dialect that wants one, and this one
	// wants the braces.
	//
	// Measured 2026-09-11 over a file holding `function a`, the body, and a
	// call, ksh93u+ against the three bash columns:
	//
	//	body            bash              ksh93
	//	echo B          at `echo', st 2   at `echo', st 3
	//	(( 1 ))         runs              at `((', st 3
	//	( echo B )      runs              at `(', st 3
	//	for … done      runs              at `for', st 3
	//	{ echo B; }     runs              runs
	//
	// It is the keyword's question alone. The same shell takes `f() echo hi`
	// through the parenthesized form and refuses only what that body
	// redirects, which is [Dialect.FuncBodyTakesNoRedirection] — so the two
	// spellings of a definition are not one rule there, and a flag that tried
	// to be both would have to pick one of the two answers and be wrong about
	// the other.
	FunctionKeywordBodyMustBeBraceGroup bool

	// FuncBodyTakesNoRedirection refuses a redirection in a function body
	// that is not compound: ksh93 takes `f() echo hi` and refuses `f() >out`,
	// `f() echo hi >out` and `f() x=1 >out`, blaming the operator itself —
	// `` `>' unexpected ``, and `` `>&' `` for `2>&1`. A braced body is not
	// this rule and `f() { :; } >out` is accepted there.
	//
	// It is narrower than FuncBodyMustBeCompound rather than a weaker form of
	// it, which is what makes it a second flag: the shell that wants a
	// compound body refuses the simple command outright, and this one takes
	// the command and refuses only what it redirects. Two of the four accept
	// both, so the panel is three ways here and not two.
	FuncBodyTakesNoRedirection bool

	// EmptyParensAreOneToken lexes `()` as a single token where the rest of
	// the panel reads two, which is visible only when a diagnostic names the
	// last token it read: zsh answers `f()` with ``parse error near `()' ``
	// where naming the closing paren alone would say `` `)' ``.
	//
	// A granularity fact rather than a wording one, which is why it is here
	// and not in Diagnostics — the two halves of `f( )` are not this token in
	// that shell either, and it declines to read that as a definition at all.
	//
	// **It is the pair wherever a refusal falls on the first of them**, and
	// not only inside the production that consumes them. An assignment in
	// front of the name puts the parenthesis outside the definition path —
	// the word list is read as a command's arguments and the `(` after them
	// is refused by the ordinary rule — and the pair is still named there.
	// Measured 2026-09-12 on zsh 5.9.2:
	//
	//	x=1 f () { echo X; }     parse error near `()'
	//	x=1 a b () { echo X; }   parse error near `()'
	//	x=1 f ( ) { echo X; }    parse error near `}'    — a blank inside,
	//	                                                   so two tokens and
	//	                                                   a subshell
	//
	// The third row is why the join reads the source for an *adjacent* `)`
	// rather than skipping blanks the way the definition path's lookahead
	// does: one blank and the two characters are not this token (#1846).
	EmptyParensAreOneToken bool

	// FunctionNamePunctuation lets a POSIX-form or keyword-form function
	// name carry punctuation — `f-g()`, `a.b()`, `:zi-reload-and-run()` —
	// which every panel shell but dash parses. dash refuses the name
	// outright (`Bad function name`), and what a shell that parsed one
	// *does* with it is the interpreter's question: ksh93 parses every name
	// below and stops the script at the definition.
	//
	// The characters are `!#%+,-./:@]^`, plus every byte above ASCII, and
	// the set is measured rather than chosen. Each of the ninety-three
	// printable ASCII punctuation marks was put at the front, the middle and
	// the end of a name, in both definition forms, in a *file* read with
	// `-n` — a command string is the wrong instrument here, because a `-c`
	// argument that begins with `-` never reaches the grammar at all. Those
	// twelve are the ones all five of bash 5.3, bash 3.2, bash-as-sh, ksh93
	// and zsh accept in every position.
	//
	// What was left out, and why, because the omissions are the interesting
	// half:
	//
	//   - `=` is excluded everywhere and by everyone, above.
	//   - `$`, `\`, `'`, `"` and a backquote are quoting or expansion
	//     operators, so the word they appear in is not one literal span and
	//     never reaches this test.
	//   - `*`, `?`, `[`, `{` and `~` split the panel: bash and zsh parse
	//     them and ksh93 refuses. `]` does not split, which is the shape of
	//     the disagreement — it is the *opening* of a pattern that ksh93
	//     will not have in a name. They are not in the corpus either,
	//     because the case that would record them cannot be run: zsh parses
	//     `f*g(){ :; }` and then expands the word, so what it reports is
	//     `no matches found` rather than anything about a name.
	//   - `}` splits the other way: zsh alone refuses `f}`, because a close
	//     brace is reserved wherever a word may stand there — the same fact
	//     [Dialect.CloseBraceAlwaysReserved] records.
	//   - A leading `#` never arrives, being a comment, though `f#g` is
	//     unanimous and is in the set.
	//
	// Position turned out not to matter to any shell once the invocation
	// artifact above was removed, so this is a character class and not a
	// grammar of names.
	FunctionNamePunctuation bool

	// FunctionKeywordNameIsAnyWord makes the word after the `function`
	// keyword a name whatever its text is: `function '' { … }`,
	// `function 'a b' { … }`, `function 'a;b' { … }`, `function '@#%' { … }`
	// are all definitions, callable by those names, listed by `functions`
	// and `typeset -f` and removed by `unfunction`.
	//
	// One shell in the panel does this and no other, and the split is not
	// only over whether it works — it is over *when* they say so, which is
	// why the answer is here rather than in Diagnostics. Measured 2026-09-08,
	// `sh -c "function '' { echo b; }; echo done"`, and the same six answers
	// come back for every name below:
	//
	//	zsh 5.9.2   done, status 0, nothing on stderr, and `functions`
	//	            lists the definition under that name
	//	bash 5.3    `not a valid identifier` on stderr naming the quoted
	//	            word, then `done` — refused where it runs, and the
	//	            script carries on at status 0
	//	bash 3.2    the same two lines
	//	bash-as-sh  the same diagnostic, then nothing: fatal, status 2
	//	ksh93       `: invalid function name`, status 1, nothing after
	//	dash        `Syntax error: "}" unexpected` — no `function` keyword
	//	            at all, so the refusal is about the brace
	//
	// So four of the six parse the construct and refuse the *name*, and only
	// dash refuses to parse it. This parser has no separate definition-time
	// name check, and building one for a construct four shells refuse and one
	// accepts would be a lot of machinery to arrive at the same diagnostic
	// those four already print. The flag says whether the word is a name; the
	// four that refuse it keep refusing it here, at their own wording, which
	// is where they refused it before this flag existed.
	//
	// The rule is that there is no rule, which is what makes this a flag
	// rather than a wider [Dialect.FunctionNamePunctuation]: the shell reads
	// a word and the word is the name. Measured over the thirty-two printable
	// ASCII punctuation marks in `function a<c>b { :; }`, both quoted and
	// bare — quoted, every one of them defines; bare, the ones that define
	// are `! # $ % + , - . / : < = > @ ] ^ _ \ { } ~`, and the rest fail for
	// reasons that are not about names at all. `& ( ) ; |` are operators, so
	// the word ended before them; `"`, `'` and a backquote open quoting that
	// never closes; and `* ? [` are **matched against the filesystem** —
	// `no matches found: a*b`, `no matches found: a?b`, `bad pattern: a[b`.
	//
	// That last group is the one exception this parser keeps. A name whose
	// unquoted literal text holds `*`, `?` or `[` is still refused, because
	// matching a function name against the filesystem is not implemented and
	// a definition of a function literally called `a*b` would be a plausible
	// wrong answer where a refusal is a visible one. The wording is the
	// keyword's own rather than that shell's, which is a diagnostics gap and
	// not a semantic one. Quoted, the same characters are ordinary text and
	// are taken: `function 'a*b' { :; }` defines it and `functions` writes it
	// back as `'a*b'`.
	//
	// The neighboring readings, all measured on zsh 5.9.2, because they are
	// what says this is a *name* rather than a hole in a check:
	//
	//	function "" { echo b; }   the same definition, either spelling
	//	function '' () { … }      the hybrid form takes it too
	//	n=''; function "$n" { … } an expanded empty name defines it too
	//	n=''; function $n { … }   defines nothing at all, silently, at
	//	                          status 0 — an unquoted empty expansion is
	//	                          no word, and a keyword with no name words
	//	                          left defines no functions
	//	e=(a b); function $e {…}  defines `a` and `b`, both with that body
	//
	// The last two are why this is about the name and not about the *word
	// list* being empty: `function` with no name words at all is
	// [Dialect.AnonymousFunction], and a name list that expands to nothing is
	// a third thing again, which this parser does not read.
	//
	// The keyword form only. The POSIX `name()` form refuses a quoted word
	// before any name test is reached, and its panel is a different one —
	// ksh93 reads `'q'() { … }` as a definition too and then refuses the
	// empty name where zsh takes it (#1561).
	FunctionKeywordNameIsAnyWord bool

	// FunctionNameIsAnyWord is that flag for the POSIX `name()` form:
	// `'a b'() { … }`, `a\ b() { … }`, `''() { … }`, `'a;b'() { … }` are
	// definitions, callable by those names, listed by `functions` and
	// `typeset -f` and removed by `unfunction`.
	//
	// Separate from the keyword flag because the panel is a different one and
	// the two came apart in this parser: the keyword form has taken any word
	// since #1548, and `name()` refused a quoted word before any name test
	// was reached — so `function a\ b { … }` defined a function here and
	// `a\ b() { … }`, the same name, was `` parse error near `(' ``. One
	// shell writing both spellings could not be read.
	//
	// Measured 2026-09-10 from a script file under `env -i`, `a\ b() { echo
	// b; }; echo after`:
	//
	//	zsh 5.9.2   `after`, status 0, nothing on stderr, and `typeset -f`
	//	            lists the body under `'a b'`
	//	bash 5.3    `` `a\ b': not a valid identifier `` on stderr, then
	//	            `after` — refused where it runs, script carries on at 0
	//	bash 3.2    the same two lines
	//	bash-as-sh  the same diagnostic and nothing after: fatal, status 2
	//	ksh93       `a b: invalid function name`, status 1, nothing after
	//	dash        `Syntax error: Bad function name`, status 2 — the only
	//	            column that refuses to *parse* it
	//
	// So five of the six read the definition and four of those refuse the
	// *name* where it runs. This parser has no definition-time name check and
	// [Dialect.FunctionKeywordNameIsAnyWord] records why one is not being
	// built for the sake of a diagnostic those four already print: the flag
	// says whether the word is a name, and the five that decline it keep
	// declining it at their own wording, where they declined it before.
	//
	// The quoting is the whole of it, which one control says and no character
	// class could: `'a;b'()` and `'a|b'()` are names here, and bare those
	// characters would have ended the word before the parenthesis. The
	// neighboring control is `'q'()` — an ordinary name in quotes — where
	// the panel splits *two against four* rather than one against five,
	// because ksh93 removes the quotes and defines `q`. That is what says the
	// quoted rows measure the quoting rather than a wider set of characters,
	// and it is the same #1566 gap the keyword form has.
	//
	// The one exception the keyword flag keeps, this one keeps for the same
	// reason: a name whose *unquoted* literal text holds `*`, `?` or `[` is
	// still refused, because that shell matches such a word against the
	// filesystem — `a*b() { :; }` defines nothing and is `no matches found:
	// a*b` — and a function literally called `a*b` would be a plausible wrong
	// answer where a refusal is a visible one. Quoted, they are ordinary
	// text: `'a*b'() { :; }` defines it.
	//
	// An assignment is still an assignment. `a=()` is an empty array and not
	// a definition of a function called `a=`, and the reading is lexical, so
	// the `=` has to be *bare* to make one: `'a=b'()` and `a\=b()` both
	// define `a=b` there, and `a[$i]=()` empties an element. That is the same
	// test [Dialect.FunctionNameExpands] already makes and this shares it.
	FunctionNameIsAnyWord bool

	// FunctionKeywordNameIsAnyBareWord makes the word after the `function`
	// keyword a name whenever it was written **bare**, whatever its
	// characters — and leaves a word carrying quoting or an expansion
	// refused, by the dialect's own route and in its own words.
	//
	// The keyword form alone, and narrower than both flags around it. It is
	// not [Dialect.FunctionKeywordNameIsAnyWord], which takes a quoted word
	// too and then has to except the three characters that shell matches
	// against the filesystem; and it is not
	// [Dialect.FunctionNameIsAnyBareWord], which answers the `name()`
	// spelling by the same rule *and* makes the refused word define nothing
	// in silence. The column this is for refuses a quoted word out loud and
	// carries on.
	//
	// Measured 2026-09-16 on bash 5.3.20 and bash 3.2.57, `eval "function $n
	// { echo r; }"` a name at a time from a script file. Every one of these
	// defines, at status 0:
	//
	//	a=2   f=    [     a~b   a?b   a*b   a{b   a}b   x[y
	//
	// and `a!b`, `a#b`, `a-b`, `a.b`, `a@b`, `a]b`, `a^b`, `a%b`, `a,b`,
	// `a:b`, `a/b` and `a+b` define there as they already did here. The
	// refusals are the words that were not written bare — `x$y`, `x${y}`,
	// `x$(y)`, `$x`, `'f'`, `"f"`, `\f`, `a\*b`, `a"b"c` — each `` `…': not
	// a valid identifier `` at 1 with the script carrying on, which is what
	// the source-text reading already writes. `a;b`, `a&b`, `a|b`, `a<b`,
	// `a>b`, `a(b`, `a)b` and `a b` are 2 in both shells and are not about
	// names at all: the word ended at the operator.
	//
	// `a*b` is the row that says the filesystem is never consulted — it
	// defines, and calling `a*b` runs it — which is where the shell this is
	// for parts from the one [Dialect.FunctionKeywordNameIsAnyWord] is for.
	//
	// The `name()` spelling is not this flag's and does not need one here:
	// that route already takes the same characters, since the parentheses
	// are the announcement and no name test stands in front of them.
	FunctionKeywordNameIsAnyBareWord bool

	// FunctionNameIsAnyBareWord is the third answer to the same question, and
	// the one that turns on **how the word was written** rather than on what
	// it says: a name written bare is a name whatever its characters, and a
	// name written with any quoting or any expansion in it is read, defines
	// nothing, and is not complained about.
	//
	// BusyBox ash alone, and it is the seventh column rather than a variant
	// of one of the six. Measured 2026-09-13 in the digest-pinned alpine
	// image, BusyBox v1.37.0, each line its own script file:
	//
	//	'f'() { echo p; }; f          `f: not found`, 127 — and `f` is a
	//	                              name nobody could object to, so this
	//	                              is not the characters
	//	a.b() { echo hi; }; a.b       `hi` — bare punctuation defines, which
	//	                              is [Dialect.FunctionNamePunctuation]
	//	a*b() { echo d; }; a*b        `d`, and `a?b`, `a[b`, `a{b`, `a}b`
	//	                              and `a~b` the same: a bare word is not
	//	                              matched against the filesystem here,
	//	                              which is where this parts company with
	//	                              [Dialect.FunctionNameIsAnyWord]
	//	\f() { echo p; }; f           `f: not found` — one backslash over an
	//	                              ordinary letter is enough
	//	a"b"() { echo p; }; ab        `ab: not found` — so it is the word and
	//	                              not the span the quotes are on
	//	''() { echo x; }; echo $?     `0`, nothing said
	//	w=foo; _p_${w}() { … }        `_p_foo: not found`
	//
	// And the same eight answers come back for the `function` keyword's
	// spelling, which is why this is one flag where the two above are two:
	// the panels differ for those and coincide here. `function 'f' { … }`,
	// `function a\*b { … }` and `function $(echo n) { … }` all define
	// nothing, and `function a.c { … }` and `function a*b { … }` both
	// define.
	//
	// **Nothing is defined, rather than something being defined under
	// another name.** Three probes say so and no one of them would have on
	// its own: `command -v` and `type` both answer 127 for the word's text
	// *and* for its source text, so it is not hiding under `'g'`; and
	// `g() { echo old; }; 'g'() { echo new; }; g` prints `old`, so an
	// existing definition is not replaced either. The definition's own
	// status is 0 and its body never runs.
	//
	// The body is still **parsed**: `'h'() { if; }` is `syntax error:
	// unexpected ";"` at status 2, so this is a definition the grammar reads
	// whole and not a line it skips. That is what makes this the same shape
	// [Dialect.FunctionNameCheckedWhenTheDefinitionRuns] records — the word
	// reaches [FuncDecl.RefusedName] as source text and the answer is the
	// interpreter's, which for this shell is
	// interp.FuncNameDefinesNothing: no wording, nothing bound, status 0.
	//
	// An assignment is still an assignment, and lexically, exactly as the
	// two flags above have it: `a=()` is `syntax error: unexpected "("`
	// here, and `'a=b'() { echo x; }` is a definition of nothing at status
	// 0 — the `=` has to be bare to make one.
	FunctionNameIsAnyBareWord bool

	// FunctionMultipleNames lets the `function` keyword take more than one
	// name for one body: `function clipcopy clippaste { … }` defines both,
	// and `$0` inside the body is the name that was called, which is what
	// makes the construct more than two definitions written once. zsh alone
	// in the panel — measured 2026-09-10, `function f1 f2 f3 { echo "$0"; }`:
	//
	//	zsh 5.9.2   all three defined, each `$0` its own name, status 0
	//	bash 5.3    `syntax error near unexpected token `f2'`, status 2
	//	bash 3.2    the same sentence, status 2
	//	bash-as-sh  the same sentence, status 2
	//	dash        no keyword at all: `function: not found`, then the
	//	            brace group runs and the closing `}` is unexpected
	//	ksh93       parses it and defines **only the first** — `f2` and
	//	            `f3` are `not found`, and `functions f1` says the whole
	//	            header, names and all, back
	//
	// So five of the six part company with zsh and the sixth reads the same
	// text as a different program. ksh93's reading is not modeled: it defines
	// a function whose extra names went nowhere, which is a lenience rather
	// than a construct, and the ksh dialect keeps refusing the line here.
	//
	// **Names are taken greedily**, exactly as [Dialect.ForMultipleNames]
	// takes a loop's: every word after the first is another name until the
	// body begins at `{` or at the `()` of the hybrid form, and a reserved
	// word is a name like any other. Measured: `function a while { … }`
	// defines `a` *and* `while` in zsh, so calling `while` afterwards runs
	// the body rather than opening a loop. A stop word is not taken —
	// `function a } { … }` is a parse error on the `}` — and neither is a
	// compound command: `function a b if true; then …` is a parse error at
	// `then` there, because `if` and `true` were read as two more names and
	// the `;` ended a definition with no body at all.
	//
	// That last reading is [Dialect.FunctionKeywordBodyIsOptional], which
	// says what a name list with no body after it declares. What the list
	// replaces is a worse answer rather than a better one: without it,
	// `function a b` read `b` as the body and defined `a` alone, silently,
	// at status 0.
	//
	// Each name is read by the rule the first one is read by, so
	// [Dialect.FunctionKeywordNameIsAnyWord] and
	// [Dialect.FunctionNameExpands] apply to every name in the list:
	// `function a "b c" d { … }` defines three, the middle one holding a
	// space. What the names then share is one body — measured, a redirection
	// on the definition is shared too, `function a b { echo "$0"; } > out`
	// sending both calls to the file.
	//
	// **The parenthesis spelling takes a name list too**, and this flag is
	// read there as well — `clipcopy clippaste() { … }` defines both, and so
	// does `echo hi () { … }`, which makes *any* word list followed by `()` a
	// definition rather than only a word the grammar already liked. Measured
	// 2026-09-10, all three at status 0 in zsh 5.9.2 and a syntax error at
	// the `(` in the other five. Two shapes bound it, measured with them:
	//
	//	x=1 a b () { … }    `parse error near `()`` — an assignment ends it
	//	a b ()              `parse error near `()`` — the body is not optional
	//	                    in this spelling, unlike the keyword one above
	//	a b >out () { … }   defines both, and the redirection is the body's
	//
	// So the names are read where a command's *arguments* are read, and the
	// two things that are not names — an assignment before them, and a
	// missing body after them — are refusals rather than readings (#1685).
	//
	// **The redirection is the body's**, which is where a definition's
	// written one goes everywhere else. Measured 2026-09-12 on zsh 5.9.2,
	// each in a scratch directory:
	//
	//	a b >o1 () { echo "[$0]"; }; a; b; cat o1     `[b]`, nothing on the
	//	                                              terminal — one body,
	//	                                              two names, one file
	//	>o1 a b () { echo "[$0]"; }; a; cat o1        `[a]` — a leading one
	//	                                              is taken too
	//	a b >o1 >o2 () { echo "[$0]"; }; a            both files written
	//	x=1 a b >o1 () { … }                          still `parse error
	//	                                              near `()'` — the
	//	                                              assignment still ends
	//	                                              the reading
	//
	// It reaches the parser by a second route, because the `(` then follows
	// the redirection's target rather than a name and there is no word in
	// hand to announce the reading — see
	// [Parser.parseFuncPosixNamesAtParen]. And it reaches the *formatter*,
	// which copies a declaration's header from the source: the redirection
	// lies inside that span and the body writes it again, so the header
	// leaves it out. Printed twice, a formatted file redirects twice (#1838).
	FunctionMultipleNames bool

	// FunctionKeywordReferenceList lets the `function` keyword's name be
	// followed by more words, which are **taken and discarded**: only the
	// first word is a function, and the rest name nothing.
	//
	// ksh93 alone, where they are a list of name references the body may
	// bind. Measured 2026-09-11 and 2026-09-12 from a script file on
	// ksh93u+, because the blame lands on a later line and `-c` has none:
	//
	//	function a b { print hi; } ⏎ a ⏎ b
	//	    `hi`, then `b: not found` at 127 — `a` is defined and `b` is not
	//	function a b c d { print hi; } ⏎ a      `hi`
	//	function a "b" { print hi; } ⏎ a        `hi` — the quotes come off
	//	function a 1b { print hi; }             invalid reference list
	//	function a b=c { print hi; }            invalid reference list
	//	function a $foo { print hi; }           invalid reference list
	//	function a b; { print hi; }             `;' unexpected
	//	function a b > out { print hi; }        `>' unexpected
	//
	// So the words are names and nothing else is: an expansion, an
	// assignment and a word that is not an identifier are all the same
	// refusal, and an operator is refused as the operator it is. Quoting is
	// removed before the test, which is what parts this from
	// [Dialect.FunctionNameIsSourceText].
	//
	// It is **not** [Dialect.FunctionMultipleNames], and the pair of them is
	// what says so: zsh defines every name in its list and answers each call
	// with its own `$0`, where here `b` is not found. No dialect sets both.
	//
	// The list stops at the end of the line, and that is where the rule is
	// visible from the outside. This shell wants a brace group after the
	// keyword ([Dialect.FunctionKeywordBodyMustBeBraceGroup]), so
	// `function a echo B` ⏎ `a` blames the `a` on **line 2** — `echo` and `B`
	// were eaten as header words and the body never started — where
	// `function a echo` ⏎ `{ print hi; }` is status 0.
	//
	// Discarded by the *grammar* and not quite by the shell: `typeset -f`
	// writes the declaration back with its list, `function a b { print hi;
	// }`. That is a listing question rather than a parsing one (#1494), and
	// nothing about `b` is reachable from a script.
	FunctionKeywordReferenceList bool

	// FunctionKeywordBodyIsOptional lets a `function` keyword's name list be
	// followed by a separator, and lets it end with no body at all. Each name
	// is then defined with an **empty** body, which is what the shell reports
	// for it — measured 2026-09-10 on zsh 5.9.2, `eval "function a b"` leaves
	// `typeset +f` listing `a` and `b`, `functions a` printing `a () { }`,
	// and a call to either printing nothing at status 0.
	//
	// It is not an autoload stub, which #1686 recorded it as and which the
	// same run disproves: `fpath=(dir); autoload af1; functions af1` prints
	// `# undefined` and `builtin autoload -X`, and a call to it reads the
	// file, where `fpath=(dir); eval "function af1"; af1` prints nothing.
	// The transcript that suggested otherwise was `function af1; af1`, and
	// the `af1` after the `;` is the *body* rather than a call — which is
	// the other half of this flag.
	//
	// **The separator half is why the two are one flag.** A `;` between the
	// names and the body is taken there — `function a; echo B` defines `a`
	// with body `echo B`, so `echo B` never runs where it stands — and
	// without reading it, a bodyless declaration would swallow the separator
	// and run the next command instead of binding it. Newlines already stand
	// there in every dialect, and the `;` joins them. The body is absent
	// exactly when no command follows: `function a b` at the end of the
	// input, before a `}`, a `fi` or a `done`, before `&&` and before a `|`.
	//
	// How far a body that is not a brace group reaches is a flag of its own:
	// [Dialect.FunctionKeywordBodyIsAnAndOrList], below.
	FunctionKeywordBodyIsOptional bool

	// FunctionKeywordBodyIsAnAndOrList makes a `function` keyword's body
	// reach to the end of the and-or list where the body is not a brace
	// group. The brace group is the one shape that ends the declaration at
	// its `}`; everything else takes the `&&` and `||` after it.
	//
	// zsh alone, and measured 2026-09-12 on zsh 5.9.2 by the **order** the
	// two commands come out in, which is the only thing that parts the two
	// readings — both print `X` and `Y` at status 0:
	//
	//	function a; echo X && echo Y  ⏎ a     X then Y — one body
	//	function a { echo X; } && echo Y ⏎ a  Y then X — `&&` is the
	//	                                      declaration's continuation
	//
	// A pipeline is inside the body too — `function a; echo X | cat && echo
	// Y` prints X then Y — and so is every other compound, which is what
	// says the brace group is special rather than "compound" being: an `if`
	// takes the `&&` after it, `function a; if true; then echo X; fi &&
	// echo Y` printing X then Y from the call.
	//
	// The hybrid form goes with the keyword and not with the parentheses:
	// `function a() echo X && echo Y` prints X then Y, where the bare
	// `a() echo X && echo Y` prints Y then X. So this is asked where the
	// keyword was written, and [Parser.parseFuncParensAndBody] never asks it.
	//
	// A `&` ends the list as it ends any and-or, and it then backgrounds the
	// whole declaration: `function a; echo X &` leaves `a` undefined in the
	// shell that ran it, the definition having happened in the subshell.
	//
	// [syntax.FuncDecl.Body] is a Command and an and-or list is an Expr, so
	// a body of more than one pipeline is wrapped in a [Group]. That is the
	// same program written back — `function a { echo X && echo Y; }` reads to
	// the same tree — which is what [SameProgram] asks of a printer. A body
	// of exactly one pipeline of one command is left bare, because it already
	// was one before this flag and wrapping it would change what every
	// existing definition prints back as.
	FunctionKeywordBodyIsAnAndOrList bool

	// TimeKeyword makes `time` a reserved word at the start of a pipeline,
	// timing the whole pipeline — `time true | wc -l` measures both elements
	// — with the report going to the shell's own standard error. Absent from
	// dash, where `time` is an ordinary name resolved from PATH.
	//
	// Only at the front: `echo hi | time wc -c` keeps `time` an ordinary
	// word, which is what bash and dash do there. It sits on either side of
	// `!`, and a bare `time` with no pipeline parses too.
	TimeKeyword bool

	// TimePosixFlag lets that keyword read `-p`, which switches the report
	// to the POSIX line format. bash and ksh93 read it; zsh does not — there
	// `-p` is the first word of the timed pipeline, a command that is not
	// found — so it is not core, and where it is off the word is left to the
	// pipeline exactly as zsh leaves it.
	TimePosixFlag bool

	// TimesIsReserved makes `times` a reserved word rather than a builtin,
	// so a word after it is a syntax error rather than an argument it
	// ignores. ksh93 alone, and the only place in the panel where *which*
	// builtin a shell has changes what parses.
	TimesIsReserved bool

	// CaseContinue enables `;;&`, which keeps testing later patterns. bash
	// only: ksh93 and zsh both reject it, so it is not core.
	CaseContinue bool

	// CaseContinuePipe enables `;|`, which is zsh's spelling of `;;&` — the
	// same terminator, measured to the same output: an arm runs and the
	// *later patterns keep being tested*, which a five-arm program with a
	// `;&` in it confirms letter for letter against bash's `;;&`.
	//
	// A second flag beside CaseContinue rather than a second value of it,
	// for the reason PipeBothStreams already records about `|&`: the two
	// spellings are mutually exclusive, so no single flag could be given a
	// value. zsh takes `;|` and refuses `;;&` with ``parse error near `&'``;
	// bash 4-and-later takes `;;&` and refuses `;|`; dash, bash 3.2 and
	// ksh93 have neither.
	//
	// The token is not confined to a `case` arm, because zsh's is not:
	// measured, `echo a ;| echo b` is ``parse error near `;|'`` there and
	// ``near `|'`` in the other four, so the two bytes are one operator
	// wherever they stand. Where the flag is off the operator table falls
	// back to `;` and then `|`, which is what those four lex — so the
	// refusal lands on the `|` where theirs does, and the diagnostic follows
	// from the lexing rather than being written twice.
	CaseContinuePipe bool

	// DollarSingleQuote enables `$'...'`, where backslash escapes are
	// interpreted. Absent from dash.
	DollarSingleQuote bool

	// DollarDoubleQuote enables `$"..."`, the locale-translatable string.
	// With no message catalog — the only condition the panel can measure —
	// bash and ksh93 strip the `$` and read a plain double-quoted string,
	// same escapes and same expansions. It is not core because dash and zsh
	// are on the other side: there the `$` stays a literal character in
	// front of an ordinary double-quoted string — no error, an extra byte in
	// the word, which is the `&>` failure mode again and the reason this is
	// a flag rather than always on.
	DollarDoubleQuote bool

	// Herestring enables `<<<`. Absent from dash.
	Herestring bool

	// ClobberOverrideMarker generalizes the clobber-override marker. Core
	// takes `|` after `>` alone, which is `>|`; this makes the marker `|`
	// *or* `!` and lets it follow any of the four write operators, so it
	// enables all seven of `>!`, `>>|`, `>>!`, `&>|`, `&>!`, `&>>|` and
	// `&>>!`. The last four need AmpersandRedirect as well, since a marker
	// cannot attach to an operator the dialect does not read.
	//
	// Measured 2026-09-07 across the panel, and it moves as one thing: zsh
	// 5.9.2 accepts all seven, and dash, bash 5.3, bash 5.3 as sh, bash 3.2
	// and ksh93 accept none of them — they have `>|` and nothing else. That
	// is why it is one flag rather than one per spelling; there is no column
	// that takes some and refuses others.
	//
	// The two fallbacks are different and both are what the shells do, which
	// is the reason to be careful here. A `|` marker falls back to a pipe
	// with nothing on its left, so `echo hi >>| f` is a *refusal* in the
	// five — a syntax error at the `|`, each in its own words. A `!` marker
	// falls back to a word, and that one is silent: `echo hi >! f` in all
	// five writes a file whose name is the single character `!`, holding
	// `hi f`, and reports 0. This is the AmpersandRedirect hazard exactly —
	// one spelling, two meanings, no diagnostic — so accepting the union
	// here would quietly pick zsh's reading for text that legitimately has
	// the other one.
	ClobberOverrideMarker bool

	// RenameOnSuccessRedirect reads `>;`, one dialect's write that lands
	// only if the command succeeded. The output goes to a temporary file in
	// the target's own directory and is renamed over the target when the
	// command ends at status 0; at any other status the target is left
	// exactly as it was, and a target that did not exist is not created.
	//
	// Measured 2026-09-14. ksh93u+ alone has it: `echo new >; f` replaces
	// `f`, and `{ printf X; false; } >; f` leaves the old contents and
	// status 1. bash 5.3.15, zsh 5.9.2 and dash all refuse the text with a
	// syntax error at the `;`, which is the fallback this flag being off
	// leaves in place — `>` then `;`, and a `>` with no target.
	//
	// The `;` is part of the operator and must be tight: `echo x > ; f` is a
	// syntax error in ksh93 too. There is no `>>;` and no `<;`; both are
	// syntax errors there, so this is one operator rather than a marker that
	// generalizes.
	//
	// It is also where `<->` comes from, which is what #918 set out to
	// explain. `echo <->; echo done` in ksh93 reports that it cannot open
	// `-` and then does *not* run `done` — because the text is `echo` with
	// `<-` and `>;`, whose target is the word `echo` and whose argument is
	// `done`. `echo <->x` puts an `x` where the `;` was, so there is no `>;`
	// at all, and `done` runs. The diagnostic naming `-` rather than `->`
	// was the clue: the `<` had already taken its operand.
	RenameOnSuccessRedirect bool

	// SeekRedirect reads `<#` and `>#`, one dialect's **file-position**
	// redirections. They move where a descriptor next reads or writes rather
	// than deciding what it is aimed at, so `exec 3<#((0))` rewinds
	// descriptor 3 and nothing is opened, closed or duplicated.
	//
	// The operand is a word. The one this grammar reads is an arithmetic
	// command — `((expr))`, whose expression may name parameters and assign
	// to them — so the parser takes a TokArithCmd where every other
	// redirection takes a target. ksh93 also reads a *pattern* there and
	// seeks to the line matching it; that half is not claimed here, and the
	// operand is refused rather than misread (#3034).
	//
	// Measured 2026-09-16 on ksh93u+ 2012-08-01, script files under
	// `env -i PATH=/usr/bin:/bin LC_ALL=C`, stdin on /dev/null:
	//
	//	printf abcdefghij > f; exec 3< f
	//	read -n4 v <&3       [abcd]
	//	exec 3<#((0)); read -n2 v <&3    [ab]
	//	exec 3<#((6)); read -n2 v <&3    [gh]
	//
	// bash 5.3.20, zsh 5.9 and dash all refuse the text, each in its own
	// words, and none has the operator at all — so with the flag off the two
	// bytes fall back to `<` or `>` followed by a `#`, which begins a
	// comment and leaves the redirection with no target. That is the syntax
	// error those three report and the one this shell reported before the
	// operator existed.
	SeekRedirect bool

	// HeredocEndsAtClosingParen lets a here-document's body end at the
	// closing parenthesis of the construct it sits inside, so that the
	// delimiter is a delimiter even with the `)` written onto its line:
	//
	//	v=$(cat <<EOF
	//	a
	//	EOF)
	//
	// Where it is set — bash and ksh93 — the parentheses are found first and
	// the body is read from what is between them, so `EOF)` is the last line
	// the body could have had and the document ends there. Where it is not —
	// dash and zsh, and so the core — a body is read from the whole input,
	// takes the `)` with it, and the construct is left unclosed. Neither of
	// those two needs new wording for that: the complaint is the one each
	// already makes for a plainly unterminated `(`, word for word, which is
	// what the control `v=$(echo hi` shows.
	//
	// It is not a question about `$( )`. Any parentheses holding a program
	// are the same shape, and real zsh refuses `cat <(cat <<EOF` … `EOF)`
	// with the same complaint pointing at the `<(`. Backquotes are not: a
	// body cannot contain the mark that closes them, so all six shells take
	// `` v=`cat <<E ... E` `` and there is nothing to ask.
	//
	// The one place it is *not* additive is where the body's delimiter also
	// appears further down the file. The document then ends there rather
	// than at end of input — but the `)` was inside the body either way, so
	// the construct is unclosed all the same and the answer does not change.
	HeredocEndsAtClosingParen bool

	// HeredocLastLineIsADelimiterPrefix reads the last line of the text
	// inside `$( )` as a delimiter *prefix*, where a here-document in that
	// text reached the end of it without ever seeing the delimiter on a line
	// of its own: the delimiter is consumed and the rest of that line is
	// parsed as more of the substitution.
	//
	// The looser half of HeredocEndsAtClosingParen above, and one shell has
	// it. Measured 2026-09-12 and re-measured 2026-09-18 from a script file
	// under `env -i PATH=/usr/bin:/bin LC_ALL=C`, bash 5.3.20 against bash
	// 3.2.57, ksh93u+, dash and zsh 5.9.2, over
	//
	//	v=$(cat <<E
	//	w
	//	E <tail>
	//	echo "[$v]"
	//
	//	tail          bash 5.3                          bash 3.2 and the rest
	//	)             [w]                               [w⏎E ] or a refusal
	//	x)            `x: command not found`, [w]        the same
	//	x y)          the same with an argument          the same
	//	x) (no blank) `x: command not found`, [w]        the same
	//	E)            `E: command not found`, [w]        the same
	//	; echo hi)    syntax error at `;`, echoing       the same
	//	              `` ` ; echo hi)' ``
	//
	// **The match is a bare prefix and needs no blank**, which is what the
	// fourth and fifth rows say: `Ex)` consumes the `E` and runs `x`.
	//
	// **It is a recovery at the end of the text and not a prefix match on
	// every body line**, which is the control that makes it safe. A body line
	// beginning with the delimiter, with the document then closed properly —
	//
	//	v=$(cat <<E
	//	EXTRA
	//	E
	//	)
	//
	// — answers `[EXTRA]`: `EXTRA` begins with `E` and is not taken as the
	// delimiter. Move the closer up onto the `E` line and the answer is
	// `[EXTRA]` again, with the `E ` line consumed. So a well-formed document
	// cannot be terminated early by this, which was the regression it looked
	// like it might carry into a very common construct.
	//
	// **And it is `$( )` and nothing else.** The discriminating shape puts
	// the prefix line last in a plain file — `cat <<E` / `w` / `E x` with
	// nothing after it — and bash gives cat the body `w⏎E x`, so the rule is
	// not a general here-document rule. The backquoted spelling of the same
	// substitution answers `[w⏎E ]`, body, exactly as bash 3.2 has it.
	//
	// The warning that goes with it is not a cost: RemarkHeredocAtEOF fires
	// in exactly these cases already and both shells write
	// `warning: here-document at line 1 delimited by end-of-file (wanted
	// `E')` character for character. The well-formed control is what says the
	// remark must *not* fire for a document closed on its own line.
	//
	// Not ksh93, which refuses this shape with `` syntax error at line 1:
	// `(' unmatched `` — it runs the document to end of input and then cannot
	// find the closer, which is a question about how the substitution's
	// extent is found rather than about this rule (#1021).
	HeredocLastLineIsADelimiterPrefix bool

	// HeredocDelimiterAcrossAContinuation says whether a body line built out
	// of two or more physical lines may itself be the delimiter, and how far.
	// See [ContinuedHeredocDelimiter], which carries the measurement.
	//
	// The *joining* is not this flag's to decide and happens in every
	// dialect: an unquoted here-document's body line that ends in an odd
	// number of backslashes continues onto the line under it, and the
	// delimiter is looked for on the result. Without that, a body holding a
	// continued line ends at the first line that merely looks like the
	// delimiter and everything under it is run as commands — which, where the
	// real delimiter never arrives, reads on to the end of the input and
	// under a terminal is a hang rather than a diagnostic (#2430).
	//
	// A quoted delimiter — `<<'EOF'` or `<<\EOF` — makes the body literal
	// throughout, continuation included, and never reaches this.
	HeredocDelimiterAcrossAContinuation ContinuedHeredocDelimiter

	// StrippedHeredocDelimiter is what `<<-` does with a delimiter written
	// with leading tabs, which only a quoted delimiter can be. See
	// [HeredocDelimiterTabs], which carries the measurement.
	StrippedHeredocDelimiter HeredocDelimiterTabs

	// ArithCommand enables `(( expr ))` as a command. Consumed by the lexer,
	// which scans the expression as raw text: what is inside is an arithmetic
	// expression rather than a command list, so the token stream would lose
	// it. Where this is off, `(( 1+1 ))` is two nested subshells running
	// `1+1` as a command name, which is what dash does — not an error, a
	// different program.
	ArithCommand bool

	// ArithCommandScanIgnoresQuoting makes the scan that looks for an
	// arithmetic command's closing `))` blind to quoting, so a `)` written
	// inside quotes closes the expression's nesting like any other — and,
	// arriving where nothing closes it twice, gives the arithmetic reading up
	// and leaves two groupings behind.
	//
	// `((` is ambiguous and ArithCommand's own doc has the rule that settles
	// it: the reading holds where the expression's nesting is closed by two
	// *adjacent* `)`. This is the narrower question of whether the scan
	// looking for them sees a `)` inside a quoted run at all, and the panel
	// splits on it where the rule itself is unanimous. Measured 2026-09-15,
	// each probe in a script file of its own:
	//
	//	probe                 bash 5.3, 3.2, as sh  zsh 5.9.2   ksh93u+
	//	((echo "a)b"))        arithmetic error, 1   prints a)b  prints a)b
	//	((echo 'a)b'))        arithmetic error, 1   prints a)b  prints a)b
	//	((echo "(" ))         arithmetic error, 1   prints (    `"' unmatched, 3
	//	((echo a\) ))         arithmetic error, 1   arith error arith error
	//	((echo $(echo a) ))   arithmetic error, 1   arith error arith error
	//
	// dash and BusyBox ash have no `((` at all and print the text in every
	// row. The last two rows are what keeps the flag narrow: a backslash and a
	// command substitution are stepped over by every column, so this is about
	// *quoting* and not about skipping in general.
	//
	// It is not shared with the `$((` fallback, which asks the same-shaped
	// question at a different construct: bash tracks quoting there too and
	// zsh and ksh93 do not, but the two were measured separately and nothing
	// here assumes they move together. See
	// ArithSubstFallsBackToCommandSubst and `substitutions.md`.
	//
	// ksh93's third row is its own third answer and is a fact about that
	// shell's *fallback* rather than about this scan: having given the
	// reading up at the `(` inside the quotes it meets a quote it cannot
	// close, where the flag alone leaves two groupings that print `(`. It is
	// measured and left (#3069).
	ArithCommandScanIgnoresQuoting bool

	// ArithSubstScanIgnoresQuoting is the same question at the `$((`
	// fallback: whether the scan that decides between arithmetic and a
	// command substitution holding a subshell sees a `)` written inside
	// quotes.
	//
	// Measured 2026-09-17 and again 2026-09-18, each probe in a script file
	// of its own under `env -i PATH=/usr/bin:/bin LC_ALL=C`:
	//
	//	probe                   bash 5.3, 3.2, as sh  zsh 5.9.2        ksh93u+
	//	echo $(( '0)' + 1 ))    arithmetic error, 1   `0)` not found   the same
	//	echo $(( "0)" + 1 ))    arithmetic error, 1   `0)` not found   the same
	//
	// So the three bash columns track quoting in the deciding scan and the
	// other two do not — the same split ArithCommandScanIgnoresQuoting
	// records for `((`, at the other construct.
	//
	// **Two fields and not one**, for the reason #3069 gives: the two scans
	// were measured separately and a single flag would assert that they can
	// never part. They agree today; that is a measurement rather than a
	// guarantee.
	//
	// Meaningless where ArithSubstFallsBackToCommandSubst is off, which is
	// dash and BusyBox ash — neither has the fallback reading at all (#3530).
	ArithSubstScanIgnoresQuoting bool

	// PatternCharacterUndoesTheContinuationStop takes away the stop
	// [Dialect.ContinuationStopsADollarAt] puts on a `$` where an unquoted
	// `*`, `?`, `[`, `{` or `~` stands earlier in the same word.
	//
	// One column has a stop at all and this is that column's exception to it.
	// Measured 2026-09-16 and again 2026-09-18 on ksh93u+ 2012-08-01, `x=5`,
	// each probe a script file of its own; the word is `<prefix>$\⏎x`, written
	// as the operand of a `printf` whose format wraps it in angle brackets:
	//
	//	prefix   ksh93u+     prefix   ksh93u+
	//	a        `<a$x>`     [        `<[5>`
	//	!        `<!$x>`     {        `<{5>`
	//	}        `<}$x>`     *        `<*5>`
	//	]        `<]$x>`     ?        `<?5>`
	//	'*'      `<*$x>`     ~        `<~5>`
	//	"*"      `<*$x>`     a[b]     `<a[b]5>`
	//	\*       `<*$x>`
	//
	// So the left column keeps the stop and the right loses it, and the three
	// quoted spellings are what say the character has to be unquoted. The
	// list is neither the glob metacharacters nor the expansion characters:
	// `~` is in it and the closing `}` and `]` are not.
	//
	// Whether the rule is about globbing or about that shell taking a second
	// pass over a word it has marked as a pattern is not decidable from
	// outside, and the field says what was seen rather than which of those it
	// is. The other four columns have no stop for it to take away, so there
	// is no panel split here — this is one shell against our reading of it
	// (#3523).
	PatternCharacterUndoesTheContinuationStop bool

	// FunctionKeyword enables `function name { ... }`. Absent from dash,
	// present in bash, ksh93 and zsh.
	FunctionKeyword bool

	// FunctionKeywordParens enables the hybrid `function name() { ... }`.
	// bash and zsh accept it; ksh93 — where the keyword originated — rejects
	// it, so it is not core.
	//
	// It is a second flag rather than part of FunctionKeyword because the
	// two forms are accepted by different sets of shells, and a flag that
	// answers for both cannot be given a value for ksh93. The distinction
	// was documented on FunctionKeyword before anything enforced it, which
	// is how ksh93 came to accept a form it rejects.
	FunctionKeywordParens bool

	// ParamSubstitution enables `${x/pat/rep}` and the spellings that put a
	// `#` or a `%` after the `/`. Absent from dash.
	//
	// It used to say "and its anchored forms", which read the two as one
	// feature and they are not: BusyBox ash accepts every spelling and reads
	// no anchor in any of them — `${w/#a/Q}` on `x#ay%bz` is `xQy%bz` there,
	// the `#a` found where it really stands. Acceptance is what this flag
	// answers and it is unanimous among the shells that have the construct;
	// what the `#` *means* is interp.Semantics.ReplacementAnchors, which is
	// where the divergence lives (#3272).
	//
	// The same division one slash further along: a `#` or a `%` after the
	// **global** `//` is accepted by every column too and is an anchor in
	// zsh alone — `${v//#a/X}` on `abcabc` is `Xbcabc` there and `abcabc`
	// in bash, bash 3.2, bash-as-`sh` and ksh93. The grammar reads the
	// anchor in both spellings and
	// interp.Semantics.GlobalReplacementAnchors decides whether it counts
	// (#3307).
	ParamSubstitution bool

	// BadSubstitutionAtParseTime refuses a `${...}` with an unrecognized
	// operator while reading the script, rather than when the expansion is
	// reached. ksh93 alone diagnoses it at parse time; bash, dash and zsh
	// treat a bad substitution as a runtime error, so one inside a branch
	// that is never taken is never diagnosed at all. The zero value is the
	// majority: defer to runtime.
	//
	// This is not a corner case in the wild — Terraform templates carry
	// `${name ~}` interpolations bash never evaluates, and refusing them at
	// parse time refuses the whole file.
	BadSubstitutionAtParseTime bool

	// ParamSubstring enables `${x:off:len}`. Absent from dash.
	ParamSubstring bool

	// ParamSubstringOffsetTakesALeadingColon puts a `:` written immediately
	// after the substring's own into the **offset expression**, so the range
	// has no separator there and `${x::2}` is an expression reading `:2`.
	//
	// The other reading — the zero value, and bash's — is that the offset is
	// simply empty and the range is `0:2`. Measured 2026-09-15 under `env -i
	// PATH=/usr/bin:/bin` with `v=oldoldold`:
	//
	//	                bash 5.3.15   ksh93u+
	//	${v::2}         `ol`          `:2: arithmetic syntax error`
	//	${v::}          empty         `:: arithmetic syntax error`
	//	${v::-D}        empty         `:-D: arithmetic syntax error`
	//	${v::1:2}       `o`           `:1:2: arithmetic syntax error`
	//
	// zsh reads `${v::…}` as a modifier list and dash has no substring at
	// all, so the panel is one column against one and two that are asked
	// something else.
	//
	// A grammar flag and not a wording, because it decides where the range
	// is *cut*: the same characters are one expression under it and two
	// under the other reading, and what the shell then says about them
	// follows from that. `${v::=A}` is the shape that made it worth having —
	// it is the always-assign operator in zsh, an offset of nothing and a
	// length of `=A` in bash's reading, and the single expression `:=A` here
	// (#2818).
	//
	// It cannot collide with the colon-prefixed conditionals: `${v:-D}` has
	// one colon and is read as the default-value operator before any of this.
	ParamSubstringOffsetTakesALeadingColon bool

	// ParamCaseChange enables `${x^^}` and `${x,,}`. **bash alone**: ksh93
	// reports a syntax error and zsh a bad substitution, so a construct one
	// panel shell supports is not a common denominator and this is off for
	// the core.
	ParamCaseChange bool

	// ParamTransformations enables `${x@Q}` and the rest of the letter
	// family — Q E P A a K k L U u — which transform the value rather than
	// test or edit it. **bash alone**: dash and zsh call the construct a bad
	// substitution when the expansion is reached, and so does ksh93, whose
	// refusal of every *other* unrecognized operator while reading makes the
	// `@` family its one deferred bad substitution. Off for the core.
	//
	// The letter set is fixed. `${x@}`, `${x@QQ}` and `${x@Z}` are bad
	// substitutions in the shell that has the form, so where the flag is on
	// they stay exactly what they are where it is off.
	ParamTransformations bool

	// AliasesExpandUnlessTold is whether this shell expands aliases with
	// nobody having asked it to. bash is the holdout and needs `shopt -s
	// expand_aliases`; every other shell in the panel expands out of the box,
	// and turns it off with an option of its own — zsh's `unsetopt aliases`.
	//
	// It is the **whole** gate on every text the shell reads that is not its
	// own program: `eval`'s string, a command substitution's, a sourced
	// file's, a trap body's. Measured 2026-09-12 under `-c`, with the alias
	// defined on a line of its own so that the probe is not measuring the
	// rule that an alias never expands on the line that defines it:
	//
	//	shell        eval  $( )  .  trap
	//	bash          no    no   no  no
	//	bash +shopt  yes   yes   —   —
	//	bash as sh   yes   yes  yes   —
	//	dash         yes   yes  yes  yes
	//	ksh93        yes   yes  yes  yes
	//	zsh          yes   yes  yes  yes
	//
	// A one-liner is the trap here, and it caught the earlier measurement of
	// this and the one behind ash's route set (#2338): `sh -c 'alias t=echo;
	// t X'` answers `t: not found` in *every* shell, because the alias has
	// not run when the line holding its use is parsed. Two lines, or nothing
	// is being measured.
	//
	// **Not a route set.** That is what ExpandAliasesInProgramText below is,
	// and separating them is #2109: one field holding both made bash's "off
	// until asked" indistinguishable from zsh's "not under `-c`", and the
	// front end derived the nested texts' answer from the same value — so a
	// zsh `-c` string, which really does not expand its own text, turned the
	// alias table off for every `eval` and `$( )` inside it as well.
	AliasesExpandUnlessTold bool

	// ExpandAliasesInProgramText is the set of *non-interactive* routes on
	// which the shell's **own program text** expands an alias, when
	// AliasesExpandUnlessTold has said there is anything to expand. All the
	// shells expand interactively, which is the front end's to know rather
	// than this — it is what decides there is a person at the keyboard.
	//
	// Measured 2026-09-12, the same two lines by all three routes, with the
	// option turned on where the shell has it off by default:
	//
	//	shell        -c    script file   standard input
	//	bash        yes    yes           yes
	//	bash as sh  yes    yes           yes
	//	dash        yes    yes           yes
	//	ksh93       yes    yes           yes
	//	ash         yes    yes           yes
	//	zsh         *no*   yes           yes
	//
	// The ash row is measured 2026-09-13, BusyBox v1.37.0 in the pinned
	// alpine image, and it moved: see below.
	//
	// **One cell, and it is zsh's.** The row is not really about aliases:
	// zsh reads a `-c` string *whole* before running any of it — see
	// Diagnostics.CommandStringParsedWhole — so the `alias` on line 1 has not
	// run when line 2 is parsed, and nothing on the string can expand. The
	// route set stands in for that here because this front end parses a
	// command string a line at a time and would otherwise expand where zsh
	// does not.
	//
	// ash held that value too until #2338, and it never should have: the
	// probe behind it was a one-liner, which dash — expanding on every route
	// — answers identically. Asked with two lines, BusyBox expands under
	// `-c`, so the cell is dash's and zsh's is the only one in the column.
	// That is worth saying out loud, because the reading above depends on
	// it: the route set stands in for a reading strategy, and exactly one
	// shell in the panel has that strategy.
	//
	// Whether a word *is* expanded, and into what, is not a dialect question:
	// every shell that expands agrees on the whole algorithm, so that is the
	// core's behavior and lives in alias.go.
	ExpandAliasesInProgramText ProgramRoutes

	// AliasesExpandReservedWords lets an alias whose *name* is one of the
	// words this grammar reserves stand in for it, so `alias for=echo` on
	// one line turns `for x in 1` on the next into a command rather than
	// into a loop. Where it is off the name is still stored and still
	// listed — only the substitution is declined, and the word goes on
	// meaning what the grammar says it means.
	//
	// The standard puts the name out of bounds and two shells take it
	// anyway. Measured 2026-09-13 from a script file, the alias defined on a
	// line of its own and used on the next, with the option turned on where
	// the shell has it off:
	//
	//	shell         for  if  while  case  {  !  select  function  time
	//	bash 5.3      yes yes  yes    yes  yes yes  yes     yes      yes
	//	bash 3.2      yes yes  yes    yes  yes yes  yes     yes      yes
	//	zsh 5.9       yes yes  yes    yes  yes yes  yes     yes      yes
	//	bash as sh     no  no   no     no   no  no   no      no       no
	//	dash           no  no   no     no   no  no  yes     yes      yes
	//	ksh93          no  no   no     no   —   no   no      no       no
	//	ash            no  no   no     no   no  no  yes      no      yes
	//
	// Two rows are what make this one field and not a table of names. The
	// protected set is **the words that dialect reserves** and nothing else:
	// dash has no `select`, no `function` keyword and no `time` keyword, so
	// an alias takes all three; BusyBox ash has `function` and neither of
	// the others, and protects exactly `function`. ksh93 has all three and
	// protects all three. So the question is per dialect and the set is
	// read off the grammar — see [Parser.reservedInDialect] — rather than
	// written down a second time here. (ksh93's cell for `{` is a dash
	// because it refuses the alias *name*, which is a different axis.)
	//
	// Two of the words are read one level out from a command — a pipeline's
	// leading `!` and the `time` in front of it — and that is where this
	// reached nothing at first: parsePipeline answered both before a
	// command existed to consult a table for, so `alias '!'='echo took'`
	// with `! true` behind it printed nothing here and `took true` in all
	// three shells that expand (#2638). The same field decides them, at the
	// same three-against-four split, from [Parser.expandPipelineHead]. The
	// `time` cell above is measured in that position and the `!` cell in
	// both it and the one after a value ending in a blank.
	//
	// Both shells with a POSIX mode move it, in both directions, and
	// interp.Runner.SetPosixMode is what moves it: `set -o posix` protects
	// the words in bash 5.3 and in the 3.2 macOS ships, and `set +o posix`
	// hands them back — measured with `shopt -s expand_aliases` re-issued
	// after leaving, since leaving the mode turns that switch off on its own.
	// Invoking either bash or zsh as `sh` protects them for the whole run.
	//
	// Off in the core, which refuses what the panel disagrees about.
	AliasesExpandReservedWords bool

	// AliasedReservedWordStandsBehindAnAssignmentPrefix keeps a reserved
	// word an alias supplied reserved where the command word stands behind
	// an assignment prefix, which is the one position a *written* one loses
	// the reading in.
	//
	// The two spellings part company only there, which is what makes this a
	// question about the expansion rather than about the construct. Measured
	// 2026-09-18 on ksh93u+ 2012-08-01 from script files under `env -i
	// PATH=/usr/bin:/bin LC_ALL=C` with stdin `/dev/null`:
	//
	//	written out          alias g="…" then `v=x g`
	//	v=x { :; }      `}'  `{'
	//	v=x while …     `do' `while'
	//	v=x if …        `then' `if'
	//	v=x case …      `)'  `case'
	//	v=x ( : )       `('  `('
	//
	// Written out, `{` behind the prefix is an ordinary word and the list
	// runs on until the `}` that closes nothing — which is why the token
	// quoted there is the far end of the group. Supplied by an alias it is
	// the reserved word, a compound command has nowhere to stand behind an
	// assignment, and the complaint is at the word itself. `(` is an
	// operator in both spellings and so agrees in both, which is the control
	// row: this is about *words* the lexer would otherwise hand on as names.
	//
	// bash never reaches the question — it expands no alias in a
	// non-interactive shell without `shopt -s expand_aliases` — and dash
	// quotes the far end for both spellings. zsh quotes the near word for
	// both, because it keeps the reading for a written-out reserved word
	// too; that is a separate answer this flag does not carry, and is
	// measured in #3560.
	//
	// The prefix is an **assignment** one. A redirection alone does not take
	// the reading away from a written reserved word in the shell this is
	// for — `>/dev/null { echo hi; }` is `` `}' unexpected `` there and
	// `>/dev/null g` with the same alias runs the group — so that row is
	// measured and left as it is, being a compound command to parse rather
	// than a token to name (#3560).
	AliasedReservedWordStandsBehindAnAssignmentPrefix bool

	// AliasBodyCountsLines counts the newlines inside a substituted alias
	// body as lines of the input, so that every later line shifts by one per
	// newline and a command written on the body's second line is reported
	// there.
	//
	// It is the one place the two substitution models are visible from
	// outside. dash, ksh93, zsh and BusyBox ash splice the body's *text*, so
	// its newlines are input lines; bash splices tokens and the whole body
	// sits on the line the alias word was written on. Measured 2026-09-05
	// with `$LINENO` after a two-line body physically on line 5 — bash 5,
	// the other four 6 — and again with a three-line body, which shifts by
	// two; and with a command that fails inside the body, reported on the
	// body's own line by the four and on the alias word's line by bash.
	//
	// The bash column is only reachable with the aliases on. Re-measured
	// 2026-09-16, a script that never asks for them expands nothing in bash
	// 5.3 or bash 3.2 and the word is `command not found`; the same file
	// under that binary called `sh` expands it and answers 5. So this is one
	// of the rows where argv[0] decides whether there is an answer at all. The shift is per
	// *expansion*: using the alias twice shifts twice, and defining it and
	// never using it shifts nothing.
	//
	// True in the core, which is the majority of the panel and of the
	// dialects that expand at all. Unreachable where ExpandAliases is empty,
	// since nothing is ever spliced.
	AliasBodyCountsLines bool

	// AliasTrailingBlankReachesPastAnOpenConstruct keeps the value's
	// trailing blank working when the blank is *inside* a construct the
	// body opened, so the word after that construct closes is offered to
	// the table in turn.
	//
	// A value ending in a blank makes the next word eligible — the rule
	// behind `alias sudo='sudo '` — and it is unanimous while the blank is
	// a blank. Where the body also opens a quote, the blank is inside the
	// quote and the panel parts:
	//
	//	alias c='CEE'
	//	alias q='echo "x '
	//	q b" c
	//
	//	bash 5.3, bash as sh, bash 3.2   x  b CEE
	//	dash, ksh93, zsh, BusyBox ash    x  b c
	//
	// Measured 2026-09-13 from a script file, since zsh expands no alias
	// under `-c`. bash offers the next *word* of the resulting line, which
	// is the one after the quote closes; the other four offer the text
	// immediately after the value, which is inside the quote and is no word
	// at all, so nothing is offered.
	//
	// The half that does not split is measured with it: `q b"` alone, with
	// `b` an alias, is `x  b` in all seven — the word the quote swallows is
	// never a candidate anywhere. So this is about the word *past* the
	// construct and not about the one inside it.
	//
	// Off in the core, which refuses what the panel disagrees about, and it
	// is reachable only through [Parser.carryOpenWord] — there is no open
	// construct for a blank to be inside of otherwise (#2685).
	AliasTrailingBlankReachesPastAnOpenConstruct bool

	// AliasBodyBackslashJoinsTheNextLine lets a backslash an alias body ends
	// with reach the *newline* that follows the alias word, where it is an
	// ordinary line continuation and joins the next line to the word.
	//
	// The backslash reaching the input at all is not this axis and is not
	// split: with anything else after the alias word the panel is six to one
	// that the backslash escapes the character it meets. Measured 2026-09-15
	// from a script file, because zsh expands no alias under `-c`, with
	// `alias q='printf "[%s]" a'` and one conversion reused so an empty
	// field is visible as a field:
	//
	//	written  dash       bash 5.3   bash as sh  bash 3.2   ksh93      zsh      ash
	//	q b      [a b]      [a b]      [a b]       [a ][b]    [a b]      [a b]    [a b]
	//	q        [aecho]    [aecho]    [aecho]     [a ]two    [aecho]    [a ]two  [aecho]
	//	         [two]      [two]      [two]                  [two]
	//
	// The first row is #2710's defect and needs no flag — the body's last
	// token ends at the body's edge with a backslash still in it, and the
	// blank in the input is what it escapes. The second is the split this
	// answers: five columns join `echo` to the word and print two fields,
	// and zsh and bash 3.2 do not.
	//
	// Off in the core, which is the reading that joins nothing: the line
	// after the alias word is left as a line of its own, which is what the
	// text says without the continuation. bash 3.2's `[a ]` — a word with a
	// blank in it — is a difference of its own that this does not carry; it
	// is the extra blank that whole family shows and no dialect preset is
	// that build.
	AliasBodyBackslashJoinsTheNextLine bool

	// ParamExpansionFlags enables the parenthesized flag group that may open
	// an expansion: `${(U)x}`, `${(s.:.)x}`, `${(%):-%x}`. One shell in the
	// panel parses it; to the rest the whole expansion is a bad substitution,
	// which BadSubstitutionAtParseTime already splits into a parse-time
	// refusal for one dialect and a deferred runtime error for the others.
	//
	// The flag also relaxes the name: `${(%):-%x}` — found in the wild as
	// the idiom for "the path of the file being sourced" — has no parameter
	// at all, only flags and an operator, so an empty name is legal exactly
	// when a flag group was read.
	ParamExpansionFlags bool

	// ParamTildeFlag enables a `~` written between the `${` and the
	// parameter: `${~name}`, which makes the *result* of the substitution
	// eligible for tilde expansion and filename generation whatever the
	// `GLOB_SUBST` option says. zsh alone has it; to the other four a
	// leading `~` is not a name and the whole expansion is unreadable, which
	// BadSubstitutionAtParseTime already splits into a parse-time refusal
	// for ksh93 (`` `~' unexpected ``) and a deferred runtime error for the
	// rest.
	//
	// A grammar flag rather than a semantics axis, for the reason
	// NestedParamExpansion is one: it decides where the word is cut. Without
	// it there is no parameter at the front of `${~name}` at all, so the
	// expansion is unreadable rather than differently read, and there is
	// nothing for a value to switch between.
	//
	// The node carries the *count* of tildes rather than a bool, because
	// parity is the whole of the meaning and it is not a toggle of the
	// option: measured under `GLOB_SUBST` both ways, one tilde is yes and
	// two are no from either starting point, and only a spec with no tilde
	// consults the option. Counting in the parser keeps that arithmetic in
	// one place and lets the printer write the span back as written.
	//
	// The flag also relaxes the name, the way ParamExpansionFlags does:
	// `${~}` is the empty string in the shell that has the construct.
	//
	// It cannot collide with ParamCaseChange, and not only because no
	// dialect has both: bash's case-toggle `~` follows the name — `${x~}` —
	// and this one precedes it.
	ParamTildeFlag bool

	// ParamSplitFlag enables an `=` written between the `${` and the
	// parameter: `${=name}`, which splits the *result* of the substitution
	// into words on `IFS` whatever the `SH_WORD_SPLIT` option says. zsh
	// alone has it; to the other four a leading `=` is not a name and the
	// whole expansion is unreadable, which BadSubstitutionAtParseTime
	// already splits into a parse-time refusal for ksh93 (`` `=' unexpected
	// ``) and a deferred runtime error for the rest.
	//
	// The sibling of ParamTildeFlag in every structural respect, and a
	// grammar flag for the same reason: without it there is no parameter at
	// the front of `${=name}` at all, so the expansion is unreadable rather
	// than differently read.
	//
	// The node carries the *count*, because parity is the meaning here too:
	// measured under `SH_WORD_SPLIT` both ways, one `=` splits and two do
	// not, from either starting point.
	//
	// It cannot collide with ParamAssign, the `${x=word}` that assigns a
	// default: that `=` follows a name and this one precedes it, so the
	// two are told apart by position before either is read. `${#=word}` is
	// the assignment on `$#` in zsh, and it stays one here, because the
	// run is read in front of the `#` and not behind it.
	//
	// The flag also relaxes the name, the way ParamExpansionFlags and
	// ParamTildeFlag do: `${=}` is the empty string in the shell that has
	// the construct.
	ParamSplitFlag bool

	// ParamRcExpandFlag enables a `^` written between the `${` and the
	// parameter: `${^name}`, which distributes the word the expansion stands
	// in over the elements it came to — `a=(1 2); x${^a}y` is the two words
	// `x1y` and `x2y`, where `x${a}y` is `x1` and `2y` — whatever the
	// `RC_EXPAND_PARAM` option says. zsh alone has it; to the other four a
	// leading `^` is not a name and the whole expansion is unreadable, which
	// BadSubstitutionAtParseTime already splits into a parse-time refusal
	// for ksh93 (`` `^' unexpected ``) and a deferred runtime error for the
	// rest.
	//
	// The third occupant of ParamTildeFlag's slot and a grammar flag for the
	// same reason: without it there is no parameter at the front of
	// `${^name}` at all, so the expansion is unreadable rather than
	// differently read.
	//
	// The node carries the *count*, because parity is the meaning here too:
	// measured under `RC_EXPAND_PARAM` both ways, one `^` distributes and
	// two do not, from either starting point.
	//
	// It cannot collide with anything else this grammar spells with a `^`.
	// bash's case-conversion `^` follows the name — `${x^}` — and this one
	// precedes it, and the pattern-negation `^` of `EXTENDED_GLOB` is not
	// inside a `${`.
	ParamRcExpandFlag bool

	// ParamSetTestFlag enables a `+` written between the `${` and the
	// parameter: `${+name}`, which substitutes `1` when the parameter is set
	// and `0` when it is not, and never fails. zsh alone has it; to the
	// other four a leading `+` is not a name and the whole expansion is
	// unreadable, which BadSubstitutionAtParseTime already splits into a
	// parse-time refusal for ksh93 (`` `+' unexpected ``) and a deferred
	// runtime error for the rest.
	//
	// A grammar flag rather than a semantics axis, for the reason
	// ParamTildeFlag is one: without it there is no parameter at the front
	// of `${+name}` at all, so the expansion is unreadable rather than
	// differently read, and there is nothing for a value to switch between.
	//
	// A bool rather than a count, which is the one place this differs from
	// ParamTildeFlag and is measured rather than assumed: `${++x}` is a bad
	// substitution, so there is no second `+` for a parity to be about. It
	// asks a question rather than setting a mode, and asking it twice is not
	// a spelling the grammar has.
	//
	// Unlike ParamTildeFlag it does *not* relax the name: `${+}` is a bad
	// substitution where `${~}` is the empty string, and so are `${(U)+}`
	// and `${~+}`. The name it takes is a name or a positional — `${+@}`,
	// `${+?}`, `${+#}`, `${+$}`, `${+!}`, `${+-}` and `${+*}` are all bad
	// substitutions, while `${+0}` and `${+10}` read.
	ParamSetTestFlag bool

	// ParamElementSelection enables the three operators that choose which
	// *elements* of a value survive: `${a:#pattern}` drops the ones a
	// pattern matches, `${a:|other}` the ones another array holds, and
	// `${a:*other}` keeps only those. One shell in the panel has them.
	//
	// A grammar flag rather than a semantics axis, for the reason
	// BareSubscript is one: the panel does not disagree about what these
	// characters *mean*, it cuts the word in different places. `${a:#two}`
	// is an exclusion in zsh, an offset whose arithmetic begins `#two` in
	// bash — which is a refusal, `operand expected` — and in ksh93 it is
	// `${a#two}`, prefix removal, the colon simply ignored. Three readings,
	// no shared syntax for a value to switch between, so the flag decides
	// which grammar is being read and nothing downstream has to ask.
	//
	// It is only about the colon. `${a:1}` is still an offset with the flag
	// on, because the disambiguation is the single character after it, and
	// `${a:-x}` is still a default for the same reason.
	ParamElementSelection bool

	// ParamArrayZip is `${a:^b}` and `${a:^^b}`: interleave a parameter with
	// the array the operand names, one element from each in turn. `:^` stops
	// when the shorter runs out, `:^^` cycles it.
	//
	// zsh alone. Without the flag the colon opens a substring and `^b` is
	// handed to arithmetic, which refuses it — that refusal is what a real
	// startup file produced, from `FG=( ${codes:^fg} )` in Oh My Zsh's
	// spectrum library (#2112).
	ParamArrayZip bool

	// ParamWholeElementReplace enables `${a:/pattern/replacement}`, the
	// fourth operator of that family: the elements the pattern matches
	// **whole** become the replacement and the rest are left alone, and an
	// empty replacement drops them the way ParamExclude does.
	//
	// A flag of its own rather than a fourth character in the set above,
	// because it is a different *shape* — two operands split on an unquoted
	// slash where those three take one — and a dialect may have the family
	// without it. Set together in the one shell that has either, which is
	// where the sameness ends.
	//
	// The disambiguation is the same one character, and it can be read
	// before the substring without taking anything from it: no arithmetic
	// expression begins with a division, so `${x:/p/r}` is unreadable as an
	// offset in the grammars without the flag — `bad math expression:
	// operand expected at \`/p/r'` — where `${x:3/2}` keeps its offset of
	// `3/2` under the flag. Measured on zsh 5.9.2 and bash 5.3.
	//
	// It is not ParamReplace with a colon in front. That one substitutes a
	// matching *span* inside the value; this one tests the whole of it.
	// `${x:/foo/Z}` on `foobar` is `foobar` where `${x/foo/Z}` is `Zbar`,
	// so a grammar that folded the two would replace where the shell leaves
	// the value alone.
	ParamWholeElementReplace bool

	// ParamColonBeforeTrimIsIgnored reads a colon written before a trim as
	// nothing at all: `${v:#p}` is `${v#p}`, and so are `${v:##p}`,
	// `${v:%p}` and `${v:%%p}`. One shell in the panel does this.
	//
	// The third of the three readings ParamElementSelection's note names,
	// and the one that had no answer behind it: those six characters are an
	// exclusion in zsh, an arithmetic offset beginning `#p` in bash — a
	// refusal — and the plain trim in ksh93. So this is the flag that says
	// which grammar is being read, exactly as that one is, and for the same
	// reason it is a grammar flag rather than a semantics axis: the shells
	// do not disagree about what the characters mean, they cut the word in
	// different places.
	//
	// The colon is *ignored* rather than recorded, so the node is the trim
	// it would have been without it and everything downstream — the
	// per-element mapping under `[@]`, the pattern coming out of a word —
	// works because it is the same node. ParamExpr.Colon stays false: a
	// trim has no unset-or-empty test for it to extend, and setting it
	// would put a flag on the node that nothing reads and that a later
	// reader would have to guess the meaning of.
	//
	// Only the four trims. Measured on ksh93u+ 2012-08-01: `${v:/l/L}`,
	// `${v:^^}` and a colon with a space after it stay arithmetic errors
	// there, `${v:2}` is still an offset, and `${v:#}` is `${v#}` — an
	// empty pattern that trims nothing rather than a special case.
	//
	// No shell in the panel sets this and ParamElementSelection together,
	// and the parser takes them in that order so the pair has a defined
	// reading rather than depending on which case came first in the file.
	ParamColonBeforeTrimIsIgnored bool

	// NestedQuoteResetsOperandEscapes says the escaping context of a `${ }`
	// operand stops at a `"` written inside it, so the brace that would close
	// the expansion is no longer escapable there. One shell in the panel says
	// so and five say the whole body is the context, nested quotes included.
	//
	// Measured 2026-09-10, `u` unset:
	//
	//	printf '[%s]' "${u-"A\}B"}"
	//
	//	dash                      [A}B]
	//	bash 5.3.15               [A}B]
	//	that build as `sh`        [A}B]
	//	bash 3.2.57               [A}B]
	//	ksh93u+                   [A}B]
	//	BusyBox ash               [A}B]
	//	zsh 5.9.2                 [A\}B]
	//
	// Note that bash 3.2 joins the majority *here*, which is the reverse of
	// its position on the bare-operand row `operandEscapes` records — the two
	// rows are independent and neither predicts the other.
	//
	// A grammar flag rather than a semantics axis, for the reason
	// ParamColonBeforeTrimIsIgnored is one: it decides what text the operand
	// *is*, at the stage that reads it, and by the time a value could switch
	// on it the backslash is either in the word or gone. The lexer is also
	// the only thing that knows a `"` was written inside an operand at all.
	//
	// False is the majority and the core's answer, which is a change of
	// answer rather than a new construct: this engine gave zsh's reading in
	// every dialect, arrived at without anyone choosing it — a `"` inside an
	// operand calls the plain double-quote scanner, and that scanner's escape
	// set has never had the brace in it (#2001).
	//
	// The single-quoted spelling is not this question and is unanimous apart
	// from bash 3.2: `"${u-A'\}'B}"` is `[A'}'B]` in six of the seven
	// columns, single quotes opening no run at all inside a quoted operand.
	NestedQuoteResetsOperandEscapes bool

	// ParamLengthTakesAnOperator lets `${#name}` carry an operator as well:
	// `${#v#a}` is the length of what the trim leaves. One shell in the
	// panel accepts it; the other four call the whole expansion a bad
	// substitution.
	//
	// A grammar flag, because the disagreement is over whether the text is a
	// construct at all rather than over what it means — `${#v#a}` is not a
	// number bash computes differently, it is a refusal there. Modeling it
	// as a semantics answer would mean building a node four grammars cannot
	// read and then declining to evaluate it, which puts the refusal a stage
	// later than the shells put it.
	//
	// Only with an operator. `${#v}`, `${#a[@]}`, `${#@}` and `${#*}` are
	// unanimous and are not this flag's business.
	//
	// The refusal is *deferred* in every dialect, including the one that
	// otherwise refuses an unknown operator while reading. Measured
	// 2026-09-06: `if false; then echo ${#v#a}; fi` runs clean in bash 5.3,
	// bash 3.2, bash as `sh`, dash and ksh93 alike, so none of them decides
	// this before the expansion is reached — which is why the node is marked
	// rather than failed here. The issue this came from expected dash to
	// refuse while reading; it does not.
	ParamLengthTakesAnOperator bool

	// ParamAssignAlways enables `${name::=word}`, the assignment that runs
	// every time: the word is stored and substituted whatever the parameter
	// held, where `${name:=word}` stores it only when the parameter is unset
	// or empty and `${name=word}` only when it is unset.
	//
	// One shell in the panel has it. The other four read the same characters
	// as `${name:off:len}` with an empty offset and a length of `=word`, and
	// fail in arithmetic there — `operand expected at \`=word'` — or, in
	// dash, which has no substring at all, call it a bad substitution.
	//
	// A grammar flag rather than a semantics axis, for the reason
	// ParamLengthTakesAnOperator is one: the disagreement is over whether
	// the text is a third assignment operator or a substring, which is a
	// question about what was written and not about what it means. A shell
	// without the flag must keep reading it as a substring, because that is
	// what its own arithmetic error is evidence of.
	//
	// The flag is what tells the two readings apart *before* either is
	// evaluated, and reading it the substring way is not a quiet
	// mis-answer — it is the arithmetic error that stood between a real
	// plugin manager and its own colored output for eighteen lines of one
	// startup (#1369).
	ParamAssignAlways bool

	// NestedParamExpansion enables an expansion to stand where a parameter
	// name would: `${${v}}` applies one expansion to the result of another,
	// and `${${v}#a}` applies the outer operator to what the inner came to.
	// One shell in the panel has it; bash and ksh93 refuse the same
	// characters, in their own words and at their own moment.
	//
	// A grammar flag rather than a semantics axis, for the reason
	// BareSubscript is one: it decides where the word is cut. Without it
	// there is no parameter name at the front of `${${v}#a}` at all, so the
	// expansion is unreadable rather than differently read — there is
	// nothing for a value to switch between.
	//
	// The inner expansion is the *whole* of the name position. Measured:
	// `${x${v}}` and `${${v}x}` are both a bad substitution in the shell that
	// has the construct, so text either side of it is not a shape at all and
	// an implementation that appended it would be inventing one.
	NestedParamExpansion bool

	// NamelessParamExpansion enables an expansion with no parameter name at
	// all: `${}` is the empty string, `${:-abc}` is `abc` because the name
	// that is not there is never set, and `${%x}` trims a suffix off the
	// nothing in front of it. One shell in the panel has it; the other five
	// call every one of those texts a bad substitution.
	//
	// A grammar flag rather than a semantics axis, for the reason
	// NestedParamExpansion is one: without it there is no parameter at the
	// front of `${:-abc}`, so the expansion is unreadable rather than
	// differently read, and there is nothing for a value to switch between.
	// Measured 2026-09-08 on the six-column panel:
	//
	//	${:-abc}   abc   bash, bash-as-sh, bash 3.2 and dash: bad
	//	                 substitution; ksh93: `:' unexpected while reading
	//	${}        ``    the same five refusals
	//	${%x}      ``    and again
	//
	// What the flag does *not* relax is a character that is a parameter or a
	// prefix in its own right, because those are taken before the name scan
	// ever runs and the nameless reading never competes: `${-x}` is a bad
	// substitution in all six — `-` is the name `$-` and `x` is a stray word
	// after it — `${?x}` is the same with `$?`, and `${#}` is `$#` in all
	// six rather than a length over nothing. Widening the guard leaves every
	// one of those exactly where it was, which is the measured answer.
	//
	// The flag also decides `${#:-w}`, which is the one place the two
	// readings of `#` are separated by nothing else: with the nameless form
	// the `#` is a length over `${:-w}` and zsh answers `1`, and without it
	// the `#` is the parameter `$#` and the other five answer `2` — see
	// hashIsTheParameter.
	//
	// `~/.zi/bin/zi.zsh` writes `${:-…}` inside the nested expansion that
	// builds the argv of every non-zsh plugin, which is why the empty name
	// is a daily-driver blocker rather than a corner (#1529).
	NamelessParamExpansion bool

	// ParamLengthOverASpecialNameIsFinal keeps the length reading of `${#-…}`
	// and `${#?…}` when text is left over behind the name, instead of
	// re-reading the `#` as the parameter `$#`.
	//
	// zsh alone. All six shells read a *bare* `${#-}` and `${#?}` as a length
	// over the special name — which is measured rather than assumed, and it
	// takes a probe where the two readings differ, since `$-` happens to be
	// two characters in bash and ksh93 and `$#` was two in the run that filed
	// this. With `set -- p q r`, so `$#` is 3:
	//
	//	probe        bash 5.3   dash   ksh93   zsh 5.9.2
	//	${#-}        2          0      2       4          the length of `$-`
	//	${#?}        1          1      1       1          the length of `$?`
	//	${#-w}       3          3      3       bad substitution
	//	${#?w}       3          3      3       bad substitution
	//	${#-:-x}     3          3      3       4
	//	${#?:-x}     3          3      3       1
	//
	// So the divergence is not which reading is *reached* — every shell reads
	// the `-` as a name — but what happens when that reading cannot use the
	// whole expansion. Five of them fall back and take the `#` as the
	// parameter, so `-w` and `?:-x` become an operator on `$#` and answer 3;
	// zsh keeps the name and either applies a real operator to it (`${#-:-x}`
	// is the length of `${-:-x}`, which is `$-`, so 4) or refuses the stray
	// word.
	//
	// **It is `-` and `?` and no other special name**, which is what makes it
	// a rule about operators rather than about specials: `${#$w}` and
	// `${#!w}` are bad substitutions in all six, because `$` and `!` are not
	// operators and there is no second reading to fall back to. That is the
	// same test the `%`, `/` and `#` rows in hashIsTheParameter already make
	// — an operator with an operand takes the `#` — and these two were the
	// entries missing from it because they can begin a name as well (#1242).
	ParamLengthOverASpecialNameIsFinal bool

	// ParamLengthRefusesTheBangName makes a length over the special name `!`
	// unreadable, so `${#!}` is a bad substitution rather than the length of
	// `$!`.
	//
	// zsh alone, and it is a rule about that one name behind a `${#` rather
	// than about `$!` or about lengths. Measured 2026-09-12 from a script
	// file under `env -i` with a scratch HOME and ZDOTDIR, `set -- p q r`,
	// across the seven-column panel:
	//
	//	probe     bash 5.3   bash 3.2   bash-as-sh   dash   ksh93   ash   zsh 5.9.2
	//	${!}      (empty)    (empty)    (empty)      ""     ""      ""    0
	//	${#!}     0          0          0            0      0       0     bad substitution
	//	${#$}     5          5          5            5      5       5     5
	//	${#?}     1          1          1            1      1       1     1
	//
	// The first row is what makes the second a fact about the shape rather
	// than about the value: `$!` reads perfectly well in zsh — with no
	// background job it is `0` there where the other six leave it empty — so
	// the refusal is not "there is nothing to measure". The third and fourth
	// rows are the boundary: `$` and `?` behind the same `${#` are lengths in
	// all seven, so it is not a rule about special names either.
	//
	// Deferred rather than fatal at parse time, which is measured too: with
	// `${#!}` inside an `if false` branch, zsh runs the script to the end and
	// exits 0, and `zsh -n` accepts the file.
	//
	// Distinct from ParamIndirection, which is about a `!` at the *front* of
	// an expansion. Here the `!` stands behind a length prefix, where no
	// dialect reads it as indirection: the length case is taken first and the
	// `!` is scanned as an ordinary name (#2415).
	ParamLengthRefusesTheBangName bool

	// ParamContinuationNeedsAName keeps a line continuation inside `${ }` —
	// so the expansion is refused — where no name has begun: directly behind
	// the `${`, or its `#` or `!` prefix, or behind a positional or special
	// parameter.
	//
	// A backslash-newline anywhere else in the expansion is removed before it
	// is read, and that is unanimous; this is the one shape the panel splits
	// on. Measured 2026-09-16 from script files under `env -i`, stdin on
	// /dev/null, fresh directory:
	//
	//	probe                   bash 5.3  bash 3.2  zsh 5.9.2  dash  ksh93u+
	//	xy=5 ${x\⏎y}            5         5         5          5     5
	//	${x\⏎}  ${x:\⏎-d}        value     value     value      value value
	//	${#x\⏎}  ${@:\⏎-d}       value     value     value      value value
	//	${\⏎x}  ${#\⏎x}          value     value     value      value refused
	//	${1\⏎}  ${12\⏎}  ${@\⏎}  value     value     value      value refused
	//	${?\⏎}  ${#\⏎}  ${$\⏎}   value     value     value      value refused
	//
	// "refused" is ``syntax error at line 1: `\' unexpected``, status 3, which
	// is what the kept pair already reads as there. The last two rows are
	// the boundary: the continuation is kept until a name has begun, and a
	// special or positional parameter never begins one — `${1:-a\⏎b}` and
	// `${@:\⏎-d}` read, because an operator stands before the pair.
	//
	// ksh93 alone, and not in an unquoted here-document body, where its
	// continuations are gone before the expansion is scanned: `${\⏎x}` is
	// the value there.
	ParamContinuationNeedsAName bool

	// ContinuationStopsADollarAt names the forms a `$` does **not** reach
	// across a line continuation written directly behind it, outside quotes,
	// and ContinuationStopsADollarAtInDoubleQuotes asks the same inside `"`.
	// The empty set is the core answer: the pair goes and the `$` introduces
	// whatever stands behind it, which is what every shape in bash 5.3, bash
	// 3.2 and dash does.
	//
	// This is the construct's *delimiter* and not its inside, so it is not
	// ParamContinuationNeedsAName's question nor the arithmetic one: the pair
	// stands between the `$` and the character that says which construct this
	// is. Measured 2026-09-16 from script files, `env -i PATH=/usr/bin:/bin
	// LC_ALL=C`, stdin on /dev/null, fresh directory, with `x=5` and
	// `set -- a b`:
	//
	//	probe            bash 5.3/3.2  dash       zsh 5.9.2  ksh93u+
	//	$\⏎x             5             5          5          $x
	//	$\⏎{x}           5             5          5          5
	//	$\⏎(echo hi)     hi            hi         hi         `(' unexpected
	//	$\⏎[1+2]         3             —          3          —
	//	$\⏎'a\tb'        a<tab>b       —          a<tab>b    a<tab>b
	//	"$\⏎x"           5             5          5          $x
	//	"$\⏎{x}"         5             5          {x}        ${x}
	//	"$\⏎(echo hi)"   hi            hi         $(echo hi) $(echo hi)
	//	"$\⏎[1+2]"       3             —          $[1+2]     —
	//	"$\⏎1"           a             a          a          $1
	//
	// A `—` is a shell that has no such form at all, so nothing there is a
	// measurement of this question. BusyBox ash was not measured. `$\⏎` with
	// a blank or the end of the word behind it is a literal `$` in bash 5.3,
	// bash 3.2, dash, zsh 5.9.2 and ksh93u+ alike, which is the row that says
	// the `$` reaches a *form* rather than the pair being removed and
	// forgotten.
	//
	// So zsh stops at everything but a bare parameter inside double quotes
	// and at nothing outside them, and ksh93 stops at a bare parameter and a
	// parenthesis outside quotes and at everything inside them. Neither is a
	// subset of the other, which is why this is a named set per quoting
	// rather than a count (#3457).
	ContinuationStopsADollarAt               DollarForms
	ContinuationStopsADollarAtInDoubleQuotes DollarForms

	// DollarGoesWhenAContinuationStopsItAtABrace drops the `$` where the set
	// above stopped it at a `${`, instead of leaving it as text.
	//
	// zsh's alone, and only where it stops at one: `"$\⏎{x}"` is `{x}` there
	// and `${x}` in ksh93, which is the same stop with the two opposite
	// answers about the character that was already read. It is asked at the
	// brace and nowhere else, because zsh keeps the `$` at every other form
	// it stops at — `"$\⏎(echo hi)"` is `$(echo hi)` and `"$\⏎[1+2]"` is
	// `$[1+2]` there.
	DollarGoesWhenAContinuationStopsItAtABrace bool

	// ContinuationPartsTheArithmeticOpener reads a line continuation written
	// between the `$(` and the second `(` as *parting* them, so `$(\⏎( 1 + 2
	// ))` is a command substitution whose first command is a subshell rather
	// than an arithmetic expansion.
	//
	// ContinuationPartsTheArithmeticCloser is the same question at the other
	// end: whether a pair standing between the two `)` still closes an
	// arithmetic expansion.
	//
	// Two fields, because the panel splits differently at the two ends and a
	// single answer could not be given a value for the shell that joins one
	// and parts the other. Measured 2026-09-16 from script files, `env -i
	// PATH=/usr/bin:/bin LC_ALL=C`, stdin on /dev/null, fresh directory;
	// BusyBox ash has no arithmetic expansion of this shape and was not
	// measured:
	//
	//	probe                bash 5.3, 3.2  dash  zsh 5.9.2  ksh93u+
	//	echo "[$(\⏎( 1 + 2 ))]"  3          3     3          runs `1`
	//	echo "[$(( 1 + 2 )\⏎)]"  3          3     runs `1`   runs `1`
	//
	// "runs `1`" is the command-substitution reading: the body is the
	// subshell `( 1 + 2 )`, whose first word is a command nobody has, so the
	// diagnostic names `1` and the expansion is empty.
	//
	// A blank on either side of the pair takes the question away and is the
	// control: `$( \⏎( 1 + 2 ))` and `$(( 1 + 2 ) \⏎)` are a command
	// substitution in every column, because the parentheses are no longer
	// adjacent whatever becomes of the continuation.
	//
	// The arithmetic *command*'s closer is not this question and needs no
	// field: `(( 1 + 2 )\⏎)` is two groupings running `1` in bash 5.3, zsh
	// 5.9.2, ksh93u+ and dash alike (#3454).
	ContinuationPartsTheArithmeticOpener bool
	ContinuationPartsTheArithmeticCloser bool

	// ContinuationEndsTheArithmeticCommandOpener makes `((` followed
	// *directly* by a line continuation open nothing at all: the two
	// parentheses and the pair are consumed, no command is produced, and the
	// next token is read as though the construct had never been written.
	//
	// ksh93u+ alone, measured 2026-09-16 from script files under `env -i`,
	// stdin on /dev/null. The visible answer is a syntax error, but the error
	// is at the *leftovers* rather than at the continuation, which is what
	// says the two characters were consumed and discarded:
	//
	//	((\⏎x = 5 )); echo "[$x]"      `)' unexpected at line 2, status 3
	//	if ((\⏎1 )); then …            `)' unexpected
	//	for ((\⏎i=0; i<1; i++)); do …  `i' unexpected
	//	((\⏎)); echo st=$?             `)' unexpected
	//	echo pre⏎((\⏎echo hi⏎echo b    pre, hi and b, status 0
	//	false⏎((\⏎echo "rc=$?"         rc=1 — not even the status moved
	//
	// The last two rows are the whole of it: the construct runs nothing and
	// leaves nothing behind, and every refusal above is the text that
	// followed being read on its own. bash 5.3, bash 3.2 and zsh read all six
	// as an ordinary arithmetic command; dash has no `((` and reads two
	// groupings.
	//
	// A blank in front of the backslash takes the rule away — `(( \⏎x = 5 ))`
	// is 5 there — and so does anything at all: `((x\⏎ = 5 ))` is 5 too. It
	// is the pair standing *directly* behind the second parenthesis (#3455).
	ContinuationEndsTheArithmeticCommandOpener bool

	// BareBraceNestsInExpansion makes an unquoted `{` inside `${…}` open a
	// nesting level, so the expansion ends at the brace that *balances* it
	// rather than at the first `}`.
	//
	// A grammar flag rather than a semantics axis, for the reason
	// NestedParamExpansion is one: it decides where the word is cut. With it
	// `${u:-{a,q}.z}` is one expansion whose operand runs to `.z`; without
	// it the expansion stops at the first `}` and `.z}` is two more
	// characters of the enclosing word. Those are different words, not one
	// word two values could disagree about.
	//
	// The panel splits two against three. Measured 2026-09-10 with
	// `unset u; printf "[%s]" ${u:-{a,q}.z}`:
	//
	//	zsh 5.9.2   [a.z][q.z]   the operand ran to `.z` and the group was
	//	ksh93       [a.z][q.z]   then expanded — two fields
	//	bash 5.3    [{a,q.z}]    the operand stopped at the first `}` and
	//	bash 3.2    [{a,q.z}]    `.z}` arrived as literal text — one field
	//	dash        [{a,q.z}]
	//
	// Brace *expansion* is not what separates them: `${u:-a{b}c}` has no
	// comma in it and splits the panel exactly the same way, `a{b}c` in the
	// two that balance and `a{bc}` in the three that do not. So the flag is
	// about the scan and not about what the group would have produced —
	// which is why dash, which has no brace expansion at all, still has an
	// answer here.
	//
	// Off in the core, which is what the common denominator means: the wider
	// reading takes text the other five columns read as belonging to the
	// word, and a core that swallowed it would be reading a word nobody
	// wrote.
	//
	// Only unquoted. In double quotes all seven stop at the first `}` and the
	// flag is not consulted — see the note in scanBraces, and #1586, which
	// settled that half. The `${ cmd;}` command form keeps its own rule for
	// a third reason again: its body is a program, so a `{ … }` block
	// written in one has to balance the way the program's braces do.
	BareBraceNestsInExpansion bool

	// QuoteProtectsTheClosingBrace says which of a `${ … }` operand's kinds a
	// single quote written inside **double quotes** protects the closing brace
	// in, so that the expansion ends at the brace *after* the quoted text
	// rather than at the one inside it.
	//
	// A grammar flag rather than a semantics axis for the reason
	// BareBraceNestsInExpansion is one: it decides where the word is cut.
	// `"[${v-'}'}]"` is one expansion whose operand is `'}'` where the quote
	// protects, and the expansion `${v-'}` followed by two more characters of
	// the enclosing word where it does not. Those are different words, not one
	// word two values could disagree about.
	//
	// Measured 2026-09-12 with `v=Vx}y; echo "[${v<op>'a}b'}]"`, where the
	// leaked `b'}` is the scan having stopped at the quoted brace:
	//
	//	                   `-` `:-` `=` `+` `?` `:1:`   `#` `##` `%` `%%` `/`
	//	bash 5.3.15        [Vx}y]                       [Vx}y]
	//	bash 3.2.57        [Vx}y]                       [Vx}y]
	//	bash 5.3.15 as sh  [Vx}yb'}]                    [Vx}y]
	//	ksh93u+            [Vx}yb'}]                    [Vx}y]
	//	dash               [Vx}yb'}]                    [Vx}y]
	//	BusyBox ash        [Vx}yb'}]                    [Vx}y]
	//	zsh 5.9.2          [Vx}yb'}]                    [Vx}yb'}]
	//
	// So the panel splits three ways rather than two, and the line it splits
	// on is the **kind of the operand** rather than the shell: a word operand
	// inherits the enclosing double quoting, where a single quote stands for
	// itself and cannot quote anything, and a pattern operand does not, so its
	// quotes are quotes. Only bash reads a word operand's quote as quoting,
	// and only zsh declines to read a pattern's that way.
	//
	// Three boundaries, all measured and none of them this:
	//
	//   - **Unquoted, every column protects**, in both kinds — the divergence
	//     is inside double quotes only. A here-document body answers with the
	//     double-quoted columns, which is the reading its spans already carry.
	//   - **A double quote protects in every column**, in both kinds:
	//     `v=SET; echo "[${v-"a}b"}]"` is `[SET]` in all seven. So this is
	//     about the single quote, and the `"` case of the scan is unanimous.
	//   - **The `${ cmd;}` command form keeps its own rule.** Its body is a
	//     program, so its quotes are that program's for the same reason its
	//     braces are.
	//
	// A dialect without `${x/pat/rep}` has no `/` operator to introduce a
	// pattern with, so `/` is read as a pattern only where ParamSubstitution
	// says the form exists — which is what keeps dash, the one panel member
	// without it, refusing `"${v/'$('/z}"` where ksh93 and ash accept it.
	//
	// The `sh` row is modeled, and by the *mode* rather than by the name:
	// QuoteProtectsTheClosingBraceInPosixMode below is where a dialect says
	// what POSIX mode makes of this axis, and driver enters that mode for
	// every dialect invoked as `sh`.
	//
	// It was recorded and not modeled until #2604, on the grounds that "POSIX
	// mode is a Semantics question here, and `set -o posix` cannot reach a
	// grammar flag through it (#2399)". The second half of that is false and
	// was already false when it was written: SetPosixMode replaces the
	// Runner's Dialect to move AliasesExpandReservedWords, the front end
	// notices the replacement and hands it to Parser.SetDialect, and the
	// parser re-reads what it has not tokenized yet. A grammar flag is
	// reachable at run time, and this is the second one through that door.
	//
	// **And it is decided when the word expands rather than when it is read**,
	// which says a front end handing the parser a different reading at startup
	// would agree with bash only on the inputs it was built for. Measured
	// 2026-09-13 on one function body, parsed once, before the mode was on:
	//
	//	$ bash -c "v=Vx}y; f(){ printf '[%s]' \"\${v-'a}b'}\"; echo; }; f;
	//	          set -o posix; f"
	//	[Vx}y]
	//	[Vx}yb'}]
	//
	// The same already-parsed word answers both ways as the mode moves under
	// it. Under the name `sh` the move is from BraceQuoteProtectsEveryOperand
	// to BraceQuoteProtectsAPatternOnly — the core's own reading — and only
	// for the *word* operand: `"${v#'a}'}"` is `[Vx}y]` under either name. zsh
	// under the same name does not move at all, which is the shape
	// Semantics.BadOptionToSpecialBuiltinFatalInPosixMode has, so whatever
	// mechanism reaches this the answer has to come from the dialect, which
	// is what the field below is.
	//
	// So the mode reaches this axis twice, and the two halves are different
	// things. The **parse-time** half is the dialect swap itself: text the
	// parser has not read yet is read under the new value, exactly as an
	// alias-expanded reserved word is. The **expansion-time** half is
	// ParamExpr.RawTail, which keeps the source from a `${` to the end of the
	// word it stands in so that an already-cut word can be divided again when
	// it expands. Measured 2026-09-14, and it is the measurement that bounds
	// the whole design: the re-read never moves a *word* or a *statement*
	// boundary —
	//
	//	$ bash -c 'v=V; f(){ printf "[%s]" "${v-'"'"'a}"; echo MIDDLE;
	//	          :"'"'"'}"; echo END; }; f; set -o posix; f'
	//	[V]END
	//	[V; echo MIDDLE; :'}]END
	//
	// where `MIDDLE` never prints in either mode and the `printf` takes one
	// argument in both. The tree's shape stays a fact about the parse; what
	// the run re-decides is one already-cut word's internal division.
	QuoteProtectsTheClosingBrace BraceQuotePolicy

	// QuoteProtectsTheClosingBraceInPosixMode is where POSIX mode puts the
	// reading above, and the zero value is *unmoved*.
	//
	// bash moves and zsh does not. Measured 2026-09-14 over `v=Vx}y; printf
	// '[%s]' "${v-'a}b'}"` with `s=a}b; printf '[%s]' "${s#'a}'}"` beside it
	// as the pattern-operand control: bash 5.3.15 answers `[Vx}y]` and moves
	// to `[Vx}yb'}]` under `set -o posix`, `--posix`, `POSIXLY_CORRECT=1` and
	// the name `sh` alike, while the pattern operand is unmoved under every
	// one of them; zsh 5.9.2 answers `[Vx}yb'}]` and `-o posixbuiltins` and
	// the name both leave it there. bash 3.2.57 does not move either, which
	// is recorded and not modeled for the reason every 3.2 divergence is.
	//
	// A type of its own rather than a second BraceQuotePolicy field, because
	// BraceQuotePolicy's zero value is BraceQuoteProtectsAPatternOnly — a
	// real reading — and driver calls SetPosixMode(true) for *every* dialect
	// named `sh`. A dialect that never took a position on the mode would
	// acquire one, and zsh-as-`sh` is measured above not to have it. That is
	// the hazard #2659 names one axis over, and the zero value is what
	// answers it here: unanswered stays unanswered, as it does for
	// ForNameRunForm and for the three listing axes (#2604).
	QuoteProtectsTheClosingBraceInPosixMode BraceQuotePosixMove

	// CompoundAssignmentErrorGivesUpTheLine makes a syntax error inside
	// `a=( … )` end the line it was written on rather than the file, so the
	// shell reports it, throws that line away unrun, and reads on.
	//
	// The parentheses of a compound assignment are part of a *word*, and what
	// stands between them is a list read on its own. That is what separates
	// this from every other syntax error: the file around it parsed, and only
	// the list did not.
	//
	// Measured 2026-09-12 from a script file, `echo one` above and `echo two`
	// below, with `a=(p & q)` between them:
	//
	//	bash 5.3.15           one · the complaint · two, status 0
	//	bash 3.2.57           one · the complaint · two, status 0
	//	bash 5.3.15 as `sh`   one · the complaint, status 1
	//	zsh 5.9.2             one · parse error near `&', status 1
	//	ksh93u+               one · `&' unexpected, status 3
	//	dash                  one · "(" unexpected, status 2 — it has no arrays
	//
	// So two columns carry on and four stop, and `bash -n` reports it in
	// every one of them: this is not about *when* the error is found but
	// about how much it ends. `$?` on the line after is 1 in the two that
	// carry on, and a script whose last line is the bad one exits 1 — the
	// status a failed command leaves, not the status a refused file leaves.
	//
	// It is not a property of nested parsing in general. `echo $(if)` is
	// fatal at status 2 in bash, so a substitution's contents are the file's
	// and an assignment's elements are not.
	//
	// The input running out between the parentheses is this too, and only
	// the status is left of it: there is no next line to read on to, and
	// `a=( x` is 1 where `echo $(` — the same message, outside an array
	// literal — is 2 (#2404).
	//
	// A grammar flag rather than a semantics axis because the parser is what
	// recovers: it has to read past the construct for there to be a next line
	// at all. See Parser.giveUpOnTheArray and File.Refused.
	CompoundAssignmentErrorGivesUpTheLine bool

	// ParamIndirection enables `${!x}` to *parse*. bash and ksh93 accept it;
	// dash and zsh reject it outright.
	//
	// What it then means is not this file's question, and separating the two
	// is what finally made the construct expressible. It is the panel's
	// clearest three-way divergence — bash indirects, ksh93 yields the name,
	// dash and zsh error — and a single flag could not carry three answers.
	// Split into "does it parse" here and "what does it mean" in the
	// semantics vector, each half is binary. That is the grammar/semantics
	// split doing the work it was introduced for, on the case that motivated
	// it.
	ParamIndirection bool

	// ArithIncDec enables `++` and `--`. Not POSIX; dash rejects them.
	ArithIncDec bool

	// ArithComma enables the sequence operator. Not POSIX; dash rejects it.
	ArithComma bool

	// ArithExponent enables `**`, exponentiation. Not POSIX — the operator
	// is not in ISO C either — and dash rejects it; bash, ksh93 and zsh all
	// have it. What a negative exponent means is not this flag's question:
	// the shells that parse it disagree, and the semantics vector answers.
	ArithExponent bool

	// ArithLogicalXor enables `^^`, the logical exclusive-or of an arithmetic
	// expression, and its assignment spelling `^^=`. One shell in the panel
	// has them and no script that runs under bash can contain one.
	//
	// It yields 1 or 0 like every other logical operator there, evaluates
	// both operands — there is nothing to short-circuit, since neither side
	// can decide the answer alone — and sits on the `||` rung of that
	// shell's own ladder, left-associative, while [ArithPrecedence]'s C order
	// gives it a rung of its own between `||` and `&&`. Both are measured;
	// see docs/spec/grammar/arithmetic.md.
	//
	// The spelling is already *taken* where the flag is off: `^` is bitwise
	// xor, so `a ^^ b` there is an xor whose right operand is missing, which
	// is what every column without the operator reports (#2991).
	ArithLogicalXor bool

	// ArithExplicitBase enables the `base#digits` form. Absent from dash.
	ArithExplicitBase bool

	// GroupOpeningAPatternOperandIsRefused stops the parse at a `(` standing
	// first in an expansion's *pattern* operand — `${v#(a)}`, `${v%(b)}`,
	// `${v/(a)/Z}` — rather than reading it as part of the pattern.
	//
	// A refusal rather than a construct, which is the one shape a grammar
	// flag here takes the other way round, and it is what the panel says:
	// four columns accept the text and one refuses it. Measured 2026-09-12
	// with `v=aXb`:
	//
	//	                dash   bash 5.3, 3.2, as sh   ksh93        zsh
	//	${v#(a)}        aXb    aXb                    syntax error Xb
	//	${v%(b)}        aXb    aXb                    syntax error aX
	//	${v/(a)/Z}      —      aXb                    syntax error ZXb
	//	${v#@(a)}       aXb    aXb                    Xb           aXb
	//	${v#\(a\)}      aXb    aXb                    aXb          aXb
	//
	// The fourth row is the control that makes it the *bare* spelling: `@(`
	// is that shell's own group and is read. The fifth is the other one: an
	// escaped parenthesis is an ordinary character and passes.
	//
	// A **word** operand is not this question and is measured to be so:
	// `${u:-(a)}`, `${u-(a)}` and `${u:=(a)}` all come to `(a)` in that
	// shell. So it is the pattern's reader that refuses and not the brace.
	//
	// The refusal is a syntax error at the operand's position, naming `(`,
	// and it takes the script with it — status 3 in the one shell that has
	// it.
	GroupOpeningAPatternOperandIsRefused bool

	// ArithColonIsAToken reads `:` as a math token wherever it stands rather
	// than as text no operator could be. zsh alone.
	//
	// Measured 2026-09-12: `$(( 1 : ))` there is `operand expected at end of
	// string` — the reader took the colon and then wanted a value — and
	// `$(( 1 : 2 ))` is `':' without '?'`, which is a complaint only a reader
	// that got as far as the second value can make. The other shells stop at
	// the byte: bash blames `:` and `: 2` with the sentence it gives any
	// leftover text, and ksh93 calls it an invalid character.
	//
	// A grammar flag rather than a wording, because what differs is how far
	// the reader gets before it complains and not what it says when it does.
	ArithColonIsAToken bool

	// ArithNumeralEndsAtABadDigit stops a numeral at the first character its
	// own base cannot use, rather than reading every character the base-64
	// alphabet knows and refusing the lot. zsh alone.
	//
	// Measured 2026-09-12. The two readings differ in *how many tokens* the
	// text is, which is why it is a grammar flag and not a wording:
	//
	//	           1abc                        0y
	//	bash 5.3   1abc: value too great …     0y: value too great …
	//	zsh 5.9.2  operator expected at `abc'  operator expected at `y '
	//
	// The base is known from the text — a radix prefix names it, a `base#`
	// names it, ten otherwise — so the reader can stop where that shell
	// stops. It is the same rule behind three rows that look unrelated:
	// `$(( 2#12 ))` is `operator expected at `2'` there, `$(( 08#9 ))` at
	// `9`, and `$(( 0b2 ))` at `2`, each being a digit run that ended early
	// with the rest left standing.
	ArithNumeralEndsAtABadDigit bool

	// ArithDigitSeparator makes an underscore inside a numeral a **digit
	// separator**: it is skipped, and the numeral reads as though it were
	// not there. zsh alone.
	//
	// Measured 2026-09-13 against zsh 5.9.2 and bash 5.3.15. `$(( 1_ ))` was
	// the row that started this and it cannot decide it — 1 is what both a
	// separator and a discarded byte would give — so the question is `1_0`:
	//
	//	            1_    1_0   1_0_0  0x1_f  2#1_0  16#f_f  1_0.5  1_abc
	//	zsh 5.9.2   1     10    100    31     2      255     10.5   operator
	//	                                                             expected
	//	                                                             at `abc'
	//	bash 5.3    value too great for base, every one of them
	//
	// Ten is what a separator gives and nothing else does: the byte is
	// neither a digit — bash reads it as digit 63 of the base-64 alphabet and
	// then refuses a digit base ten has no room for, which is what
	// ArithNumeralEndsAtABadDigit's sibling rows pin — nor a leftover token,
	// `1` followed by an unset name being an `operator expected` rather than
	// a 1.
	//
	// **Removed, and then the ordinary rules apply to what is left.** That is
	// the whole rule and it is worth stating that way round, because every
	// other row follows from it: `setopt octalzeroes; $(( 0_10 ))` is 8, so
	// the leading zero the separator uncovers is an octal prefix; `$(( 1_#5
	// ))` is `invalid base … : 1`, so the base is read from the cleaned text;
	// `$(( 0x_1 ))` and `$(( 2#_10 ))` are 1 and 2, so a separator may stand
	// where the first digit would; `$(( 1__0 ))` is 10, so a run of them is
	// one; and `$(( 1_ ))` is 1, so a trailing one belongs to the numeral it
	// follows rather than being left standing.
	//
	// **It is only a separator inside a numeral.** A numeral begins with a
	// digit, so a leading underscore is a name as it always was: `$(( _ ))`
	// and `$(( _1 ))` are both 0, an unset name being zero.
	//
	// A grammar flag rather than a semantics axis, and asked in the two
	// places that need it: the parser, where it decides how far the numeral
	// reaches, and the conversion, where a value a *variable* was holding is
	// read by the same rule — `x=1_0; $(( x ))` is 10 there, and no parser
	// saw that text.
	//
	// One shape is knowingly short of the shell, and it is a wording rather
	// than a value. zsh cleans the token and then reports what is left of the
	// *cleaned* text, so `$(( 1e_foo ))` blames `efoo`; this reader leaves the
	// separator standing in the leftover and blames `e_foo`. Both stop in the
	// same place and both are an `operator expected`. #2223.
	ArithDigitSeparator bool

	// ArithBinaryLiteral enables `0b101`, the binary radix prefix. zsh alone
	// among the panel: measured 2026-09-12, `$(( 0b101 ))` is 5 there and
	// `0b101: value too great for base` in bash 5.3, bash 3.2 and
	// bash-as-sh, which read it as an octal constant carrying a `b`. ksh93
	// and dash refuse it too.
	//
	// A grammar flag rather than an axis for the reason ArithFloat is one:
	// the question is whether the dialect has the literal at all, not what
	// it means where both have it.
	ArithBinaryLiteral bool

	// ArithHexFloat enables C's hexadecimal float spelling inside `$(( ))`:
	// a hexadecimal literal carrying a point or a `p` exponent is a float.
	// ksh93 alone among the panel — measured 2026-09-16, `$(( 0x1p4 ))` is
	// 16 and `$(( 0x1.8 ))` is 1.5 there, while bash 5.3, bash 3.2,
	// bash-as-sh, zsh 5.9.2, dash 0.5.12 and BusyBox ash 1.37.0 all refuse
	// every shape of it.
	//
	// The `e` of the decimal spelling is not this: `0x1e5` is the integer
	// 485 in every column, `e` being a hexadecimal digit. Only a point or a
	// `p` makes the literal a float.
	//
	// A grammar flag rather than an axis for the reason ArithFloat and
	// ArithBinaryLiteral are: the question is whether the dialect has the
	// literal at all, and the reader and the evaluator must not be able to
	// disagree about it. See interp's hexFloatNumeral for the measured
	// table, the two shapes ksh93 refuses and what an exponent with no
	// digits comes to.
	ArithHexFloat bool

	// DollarBracketArith enables `$[expr]`, the older spelling of `$((expr))`.
	//
	// Measured 2026-09-06: bash 5.3.15, bash 3.2.57, bash invoked as `sh` and
	// zsh 5.9.2 all read it as arithmetic — `echo $[1+1]` is 2, `$[2**10]` is
	// 1024, and it expands inside double quotes and in a here-document body
	// exactly as `$((…))` does. ksh93u+ and dash do not read it at all: the
	// `$` stays literal and the brackets are a pattern, so `echo $[1+1]`
	// prints `$[1+1]` when nothing on the filesystem matches.
	//
	// The additive kind of difference, so a grammar flag: where it is off the
	// text takes the route it takes today and nobody means something else by
	// it. bash has *documented* it as deprecated for years, which is a fact
	// about its manual rather than about its parser — both builds in the
	// panel still take it, which is why they are separate members.
	//
	// The construct is arithmetic and nothing else: the expression inside is
	// the same grammar `$((…))` holds, the diagnostics for a bad expression
	// or a division by zero are word for word the ones `$((…))` gives, and
	// what is produced is an ArithSubst span. Only the spelling differs,
	// which Span.Bracketed carries for anything writing one back.
	DollarBracketArith bool

	// ArithSubstFallsBackToCommandSubst decides what `$((` opens where the
	// parentheses do not close as `))`.
	//
	// `$(( … ))` is arithmetic and `$( ( … ) )` is a command substitution
	// whose first command is a subshell, and the second may be written with
	// the parentheses touching. POSIX tells the author to separate them and
	// says nothing about the shell that meets them together, so this is
	// measured.
	//
	// Measured 2026-09-12: `echo $((echo ab cde) )` prints `ab cde` in bash
	// 5.3.15, in the 3.2.57 macOS ships, in that build invoked as `sh`, in
	// ksh93u+ and in zsh 5.9.2; dash 0.5.12 and BusyBox ash 1.37.0 refuse it
	// for a missing `))`. Every shell in the panel but the two minimal ones,
	// which is the head count that put ProcessSubstitution in the core, so
	// this is on in [Core] and off in [POSIX].
	//
	// Additive rather than a conflict: where it is off the text is arithmetic
	// and nothing else, which is what the two say by refusing it. Where it is
	// on, the rule the five agree on is positional — counting from one after
	// the `$((`, the `)` that brings the count to zero is arithmetic only
	// when another `)` follows it immediately. So `$(( 1 ) + (2 ))` is a
	// command substitution even though `(1) + (2)` is good arithmetic, and
	// `$(( (1+2)) )` runs `1+2` as a command where `$(( (1+2) ))` is 3.
	ArithSubstFallsBackToCommandSubst bool

	// Whether `0100` is sixty-four or one hundred is deliberately *not* a
	// field here. A literal is kept as written, so the tree bakes in no
	// answer and nothing in the parser has the question to ask; the answer
	// is `interp.Semantics.ArithLeadingZeroIsOctal`, which evaluation reads.
	//
	// A `syntax.Dialect` field of the same name stood here and was removed
	// (#564). Nothing read it, and being unread it was also *wrong*: it was
	// documented as "true everywhere but zsh" while the `zsh` preset — which
	// starts from Core, where it was set — carried true, the opposite of
	// zsh's own answer. A flag no parser consults cannot be corrected by
	// anything failing, so it drifts, and it is indistinguishable from one
	// whose consumer was lost in a refactor.

	// ArithFloat enables floating point, which ksh93 and zsh have and POSIX
	// does not.
	ArithFloat bool

	// ArithBytesRefusedOutright are the bytes this shell's arithmetic reader
	// refuses as part of no token at all, and reports at the byte rather than
	// as a missing operand: `$((@))` is `illegal character: @` in the one
	// shell in the panel that has the sentence.
	//
	// A table of bytes and not a code path, which is the whole reason it is
	// here: the other four dialects have no such sentence, and for them this
	// is empty and every failure keeps the wording it already had. Measured
	// 2026-09-16, `x=$((@))` is `operand expected` in the three bash builds,
	// `arithmetic syntax error` in ksh93 and in BusyBox ash, `expecting
	// primary` in dash, and `illegal character: @` in zsh alone. The bytes are
	// measured, not derived — every other punctuation byte tried is either a
	// math token in that shell or can begin a value.
	//
	// It does not decide on its own. The same byte gets the operand sentence
	// where an operator has just been consumed and a value is wanted, so the
	// parser asks this only at the two positions where the expression could
	// legally have stopped: before anything has been read, and where an
	// operator belonged.
	ArithBytesRefusedOutright string

	// ArithDoubleQuote is what a `"` standing inside an arithmetic expression
	// is: a byte the reader passes over, a byte it removes from the text
	// before reading it at all, or no part of any token. See
	// [ArithDoubleQuotePolicy].
	//
	// Measured 2026-09-07 and re-measured 2026-09-10, from a script file with
	// `n=5`: `$(( "1" + 1 ))` is 2, `$(( "n" + 1 ))` is 6 and `$(( 1 + "2" ))`
	// is 3 in bash 5.3.15, that build as `sh`, ksh93u+ and zsh 5.9.2; bash
	// 3.2.57, dash and BusyBox ash refuse all three, ash with the bare
	// `arithmetic syntax error` it gives every reader failure. So four
	// columns read what the quote held and three refuse it — additive, which
	// is why it is here and not on the semantics vector, exactly as
	// [ArithCharacterCode] is.
	//
	// It matters more than the spelling suggests: `$(( "$n" + 1 ))` looks
	// defensive and is common, and refusing it fails under three of the five
	// dialects graded here (#1223).
	ArithDoubleQuote ArithDoubleQuotePolicy

	// ArithSubscriptQuoting says a quotation inside an arithmetic subscript
	// holds its brackets: the `]` that ends the subscript is one written
	// outside the quotes, so a key spelled with a bracket in it is
	// reachable.
	//
	// Measured 2026-09-16 from a script file with `typeset -A a; a[']']=5`:
	// `$(( a[']'] ))` is 5 in bash 5.3.20, under that build as `sh` and in
	// ksh93u+ 2012-08-01, and so is `(( a[']'] ))`. bash 3.2.57 has no
	// associative arrays to ask, and dash and BusyBox ash have no arrays at
	// all.
	//
	// The same subscript inside `[[ ]]` is deliberately *not* this flag, and
	// that is measured rather than assumed: ksh93u+ answers `[[ a[']'] -eq 5
	// ]]` with `a[]]: arithmetic syntax error` in the same run that answers
	// the `(( ))` line with 5. A condition's operand reaches its arithmetic
	// already expanded, so what a shell does there is a question about which
	// reading it takes rather than about what its reader sees — see
	// interp.Semantics.ConditionArithmeticReadsTheWrittenSubscript.
	//
	// zsh 5.9.2 is measured *off* rather than left out: it will not store
	// such a key from an assignment at all, and with the element put there
	// by `a=( "]" 5 )` instead, `e="a[']']"; $(( $e ))` is `bad math
	// expression: illegal character: '` — the quote reaching its reader as
	// a byte and not as a quotation. Which is the same shell that reads a
	// subscript's expanded text back as syntax, and for a reader that never
	// sees quoting the two are one behavior (#3302).
	//
	// Additive rather than a semantics axis, for the reason
	// ArithDoubleQuote is: it is what the *reader* does with a byte, and a
	// dialect without subscripts never reaches it.
	ArithSubscriptQuoting bool

	// ArithPrecedence is the order the binary operators bind in. See
	// [ArithPrecedencePolicy]: one shell in the panel does not use C's, and
	// says so in its own manual.
	//
	// It is a grammar question and not a semantics one, which is what puts
	// it here: nothing about what `<<` *means* changes, only which operands
	// it is given, so the disagreement is entirely about the tree the text
	// parses to.
	//
	// Measured 2026-09-15 from a script file, `env -i` with a scratch HOME:
	//
	//	                 zsh 5.9.2   bash 5.3   ksh93   dash
	//	1 << 2 + 1           5           8        8       8
	//	1 + 2 << 1           5           6        6       6
	//	1 << 2 * 2           8          16       16      16
	//	1 < 2 & 1            0           1        1       1
	//	2 ** 1 | 3           8           3        3       -
	//	6 | 1 + 1            8           6        6       6
	//
	// Parenthesized, every column agrees, which is what says this is
	// precedence and not a broken operator.
	ArithPrecedence ArithPrecedencePolicy

	// ArithCharacterConstant enables `'c'` inside an arithmetic expression:
	// the code of the character between the quotes, the way C reads one.
	//
	// One shell in the panel. Measured 2026-09-10 on ksh93u+, where
	// `$(( '1' + 1 ))` is 50 — the code of `1` is 49 — `$(( 'a' ))` is 97 and
	// `$(( '\101' ))` is 65, against bash 5.3.15, bash 3.2.57, bash as `sh`
	// and dash, which call it an arithmetic syntax error, and zsh 5.9.2,
	// which refuses the byte outright as an illegal character. A literal one
	// shell has and five refuse is the additive kind of split (#1223).
	//
	// The closing quote is optional, which is measured rather than assumed:
	// `$(( 'a ))` is 97 and `$(( '' ))` is 39 — the second quote read as the
	// character, with nothing left to close it. That also says why `$(( 'ab' ))`
	// is a syntax error rather than a multi-character constant: the reading
	// stops after `a`, and the `b` is left standing where an operator belongs.
	//
	// Kept apart from the double-quote question above because no shell's
	// answer to one predicts its answer to the other: the four that read
	// through a double quote include three that refuse this.
	ArithCharacterConstant bool

	// ExtendedPattern enables `@(a|b)`, `?(a)`, `+(a)`, `*(a)` and `!(a)` in
	// a pattern: a group with a quantifier in front of it. ksh93 has them
	// wherever a pattern may stand.
	ExtendedPattern bool

	// ExtendedPatternInCondition enables the same groups inside `[[ ]]` and
	// nowhere else, which is bash's answer: `[[ abc == @(abc|xyz) ]]` matches
	// there while `case abc in @(abc|xyz))` is a syntax error, because bash
	// reads the condition's operand under rules a `case` pattern does not
	// get. ksh93 answers yes to this as well as to the field above; a shell
	// with neither leaves both false.
	ExtendedPatternInCondition bool

	// CompletionConditions enables `[[ -prefix … ]]` and `[[ -suffix … ]]`,
	// the two completion-context tests, as one-operand conditions.
	//
	// zsh alone, and the other four have no such operator at all — `-prefix`
	// is the completion system's, and dash has no `[[ ]]` — so this is
	// additive grammar for one dialect rather than a divergence.
	//
	// They are in the **grammar** unconditionally there and the restriction
	// is on where they may *run*. Measured 2026-09-11 and again 2026-09-12
	// with `-n` against a script file, zsh 5.9.2: `[[ -prefix : ]]`,
	// `[[ -prefix 'ab' ]]`, `[[ -prefix //(a|b)/ ]]` and `[[ -suffix : ]]`
	// all parse, and running any of them answers `condition can only be used
	// in completion function` at status 1. That split is the one this
	// substrate draws everywhere, and it is what makes the gap a parser's: a
	// completion function is a file, and a file that will not parse never
	// gets as far as the restriction. Two files in an ordinary plugin tree
	// reach it, both completions from `zsh-users/zsh-completions` (#1879).
	//
	// The operand is read the way a pattern operand is rather than the way
	// `-o`'s option name is: `//(127.0.0.1|localhost)/` is one of the two
	// real occurrences, and it is a pattern with a group in it.
	//
	// Deliberately **only these two**. The same shell parses `[[ -nosuch x
	// ]]` as well and refuses it at run time with a different sentence, which
	// is a decision to change the rule that an operator is either implemented
	// or refused while reading — see docs/spec/grammar/conditions.md, and
	// #965, which is that question and not this one.
	CompletionConditions bool

	// ConditionArityIsCheckedWhenItRuns lets `[[ … ]]` accept a known
	// conditional operator standing with the wrong number of operands, and
	// leaves the refusal to the interpreter.
	//
	// The grammar question #965 is about, and it is the one place in this
	// parser where a condition is *accepted* and then refused. Measured
	// 2026-09-14 on zsh 5.9.2, which is the only column that does it:
	//
	//	[[ -n ]]            unknown condition: -n     at evaluation, status 2
	//	[[ -n x y ]]        unknown condition: -n     the same
	//	[[ -n x -z "" ]]    unknown condition: -n     the same
	//
	// against bash 5.3 and ksh93, which refuse all three while reading and
	// name the offending token. `echo pre; [[ -n x y ]]` prints `pre` in zsh
	// and prints nothing in the other two, which is what says where the
	// refusal happens rather than only how it is worded.
	//
	// **A known operator only**, which is measured and is the line between
	// this and CompletionConditions above: `[[ -bogus ]]` is 0 in zsh — a
	// bare word is a test for non-emptiness and `-bogus` is not empty — so a
	// word that is not an operator here is not an operator with a bad arity
	// either.
	//
	// **And not where the operand is itself an operator**, which is the row
	// that stops this being "everything after the operand is surplus":
	// `[[ -n -z x ]]` is `parse error near `x'` in zsh, where `[[ -n x y ]]`
	// is the run-time refusal — so an operator-shaped operand starts a
	// reading of its own and the word after it is simply unexpected.
	// `[[ -n -n ]]` is 0 in the same shell, which is that reading finishing.
	ConditionArityIsCheckedWhenItRuns bool

	// ParameterIsSetTest enables `[[ -v name ]]`, which asks whether a
	// parameter is set rather than anything about its value.
	//
	// A grammar flag rather than core, and bash 3.2 is why: it is the one
	// shell in the panel with `[[ ]]` that does not have the operator, and it
	// does not merely answer differently — it cannot read the line at all,
	// `conditional binary operator expected` followed by `syntax error near
	// `x''. dash has no `[[ ]]` to put it in. So the head count that made `-o`
	// core fails here by exactly one column.
	//
	// The operand is an ordinary word, read the same way `-o`'s is: `[[ -v
	// 'x' ]]` and `n=x; [[ -v $n ]]` both ask about `x`, and a subscript
	// belongs to it — `[[ -v 'a[2]' ]]`. What the name then *means* is the
	// interpreter's, and the three shells that have the operator disagree
	// about two classes of name; see interp.Semantics.
	ParameterIsSetTest bool

	// FunctionNameExpands reads a function definition's name as a *word*
	// rather than as literal text, so an expansion in one names the function
	// the expansion produces: `w=foo; _p_${w}() { … }` defines `_p_foo`.
	//
	// One dialect's, and it is the only one — dash, bash 5.3, bash-as-`sh`,
	// bash 3.2 and ksh93 all refuse both spellings, and their wordings say
	// they are refusing a *name*: `` `_p_${w}': not a valid identifier `` and
	// `_p_${w}: invalid function name`. So this is additive grammar rather
	// than an axis.
	//
	// The flag also decides what happens without it. The keyword form parsed
	// either way and took the token's literal text, which for `_p_${w}` is
	// `_p_w` — a different function, defined at status 0, with the one the
	// script asked for missing. Off, a name holding an expansion is refused
	// rather than flattened — while parsing, or where the definition runs;
	// see [Dialect.FunctionNameCheckedWhenTheDefinitionRuns], which is the
	// stage those two shells answer at.
	FunctionNameExpands bool

	// FunctionNameCheckedWhenTheDefinitionRuns makes a `function` keyword
	// whose name is not a name **parse**, with the word carried on the
	// declaration as source text and the complaint raised when the definition
	// is reached.
	//
	// The same stage question [Dialect.ForNameCheckedWhenTheLoopRuns] asks of
	// a loop variable, and the same two shells answer it that way. Measured
	// 2026-09-10 through `-c`, over `w=foo; function _p_${w} { echo HI; };
	// echo st=$?; echo after`:
	//
	//	bash 5.3.15  `` `_p_${w}': not a valid identifier ``, then
	//	             `st=1` and `after` — the script carries on
	//	bash 3.2.57  the same two lines
	//	bash-as-sh   the same sentence and nothing after: fatal at 2
	//	ksh93u+      `_p_${w}: invalid function name`, fatal at 1
	//	zsh 5.9.2    defines `_p_foo` — FunctionNameExpands, above
	//	dash         no keyword at all, and the brace group's `}` is
	//	             where it stops
	//
	// **Both of the four name the offending text, and they name it as it was
	// written.** `_p_${w}` and `_p_$@`, not what the word would come to and
	// not its literal spelling — which is why the word is kept on
	// [FuncDecl.RefusedName] as source text rather than as a name or a
	// [Word]. A refusal that named neither left a script with several such
	// definitions no way to tell which one was disliked, which is half of
	// what #1296 is about.
	//
	// The other half is the stage, and it matters more: ours refused this
	// while *parsing*, so the script stopped where bash reports and carries
	// on. dash is a syntax error there, so the split is real rather than a
	// simplification — and it is the reason this is a grammar flag and the
	// three answers behind it are not. What happens when the definition is
	// reached is interp.Semantics.FunctionNameWhenTheDefinitionRuns.
	//
	// Only the *keyword* form is carried this far. The `name()` spelling is
	// its own question with its own panel — see
	// [Dialect.FunctionNameIsAnyWord] — and the two shells here read a
	// quoted word there where ksh93 refuses an expansion outright:
	// `_p_${w}() { … }` is ``syntax error … `}' unexpected`` in ksh93 and
	// the run-time complaint in bash, which is a different split again.
	FunctionNameCheckedWhenTheDefinitionRuns bool

	// ReservedWordStandsBehindAnAssignmentPrefix keeps a written-out reserved
	// word's reading where an assignment prefix stands in front of it, so the
	// complaint lands on the word itself rather than on whatever token closes
	// the construct it opened.
	//
	// One column, and it is every word the grammar reserves rather than a
	// handful. Measured 2026-09-18, script files under `env -i
	// PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME and no startup files:
	//
	//	written                     zsh 5.9.2   bash 5.3, bash 3.2, ksh93,
	//	                                        dash and BusyBox 1.37.0 ash
	//	v=x { :; }                       names `{`     names `}`
	//	v=x while :; do :; done          names `while` names `do`
	//	v=x if :; then :; fi             names `if`    names `then`
	//	v=x for i in a; do :; done       names `for`   names `do`
	//	v=x case a in a) :;; esac        names `case`  names `)`
	//	v=x function f { :; }            names `function`  names `}`
	//	v=x select i in a; do :; done    names `select`    —
	//	v=x until false; do :; done      names `until` names `do`
	//	v=x repeat 2 do :; done          names `repeat`    —
	//	v=x foreach i (a); :; end        names `foreach`   —
	//	v=x [[ -n a ]]                   names `[[`    a command that is not found
	//	v=x time :                       names `time`  runs /usr/bin/time
	//	v=x !                            names `!`     —
	//	v=x then / do / done / fi / esac / else / elif   names each one
	//	v=x end                          names `end`   —
	//	v=x coproc cat                   names `coproc`    —
	//	v=x in                           command not found: in
	//	v=x ( : )                        names `(` everywhere
	//
	// The last two rows are the controls. `in` is special inside `for` and
	// `case` and an ordinary command name where a command begins, so it is
	// not in the set even in that column; `(` is an operator rather than a
	// word, so no reading was ever taken away from it and every column
	// agrees.
	//
	// Three of the rows are worse than a wording without this: `v=x time :`
	// ran `/usr/bin/time`, `v=x !` was a command that could not be found, and
	// `v=x [[ -n a ]]` reached the pattern matcher (#3560).
	//
	// Not the same question as
	// [Dialect.AliasedReservedWordStandsBehindAnAssignmentPrefix], which is
	// ksh93's and is asked where the word arrived **from an alias**: there a
	// written-out `v=x { :; }` names `}` and only the aliased spelling keeps
	// the reading. This column needs no alias for any row above.
	ReservedWordStandsBehindAnAssignmentPrefix bool

	// RedirectionBeforeACompound says whether a redirection may stand in
	// front of a compound command, and before which of them — see
	// [RedirectionBeforeACompoundPolicy] for the three values and the panel.
	RedirectionBeforeACompound RedirectionBeforeACompoundPolicy

	// InputDuplicateOperandIsAFileNumber requires the operand of `<&` to be a
	// run of digits, a `-`, or the coprocess letter `p`, and refuses anything
	// else **while reading**.
	//
	// One column, and it is `<&` alone. Measured 2026-09-18 on zsh 5.9.2
	// under `-f`, script files under `env -i PATH=/usr/bin:/bin LC_ALL=C`:
	//
	//	cat <&5, cat <&55, cat <&-, cat <&p, cat 3<&4              accepted
	//	cat <&"5", cat <&$'5'                                      accepted
	//	cat <&5$v, cat <&${v}5, cat <&"5""$v"                      accepted
	//	cat <&$(echo 5)5, cat <&5$(echo x)                         accepted
	//	cat <&$v, cat <&"$v", cat <&${v}, cat <&$(echo 5)          refused
	//	cat <&x, cat <&5x, cat <&{fd}, cat <&`echo 5`              refused
	//	cat <&$v5, cat <&$v$w, cat <&x$v, cat <&"x"$v              refused
	//	cat <&${v}x, cat <&""$v, exec {fd}<&$v                     refused
	//	cat >&$v, cat >&x, cat >&p, cat 2>&$v                      accepted
	//
	// It is the **literal text** the parser already holds rather than what
	// the word would come to: the spans an expansion fills are passed over,
	// and what is left has to be a non-empty run of digits. Two pairs say so
	// twice over. `5$v` against `$v5`: a digit the parser can see is enough
	// and the expansion beside it is not looked into — and `$v5` is the
	// *parameter* `v5`, one span with no literal text at all, which is why it
	// goes the other way. `5x` against `5$v`: literal text that is not a
	// digit refuses whatever stands beside it. The quoted rows are the same
	// fact once more, `<&"5"` being the digits and `<&"$v"` not.
	//
	// The `>&` rows are the control and are why this is not "a duplicating
	// redirection's operand": that operator also spells "send both streams to
	// this file", so a word there is a path.
	//
	// It is refused while reading and reported where the line stands: `zsh -n`
	// says so with the script never run, and without `-n` the line before it
	// prints, the command is skipped at status 1, and the line after it
	// prints. So the refusal gives up the **line** — [File.Refused] — rather
	// than the file, which is the same shape a syntax error inside a compound
	// assignment's parentheses already has.
	//
	// One row is measured and not modeled: on a line holding more than one
	// command, `cat <&$v; echo hi` runs the `echo` there and gives up only
	// the command. Giving up the line is as far as File.Refused reaches, and
	// a refusal carried on the *statement* is a wider change than this rule.
	//
	// bash 5.3, bash 3.2, ksh93, dash and BusyBox ash read the word and open
	// whatever it expands to, so every refused row above parsed here (#3144).
	InputDuplicateOperandIsAFileNumber bool

	// ConditionCloserIsAWordWhereATermBegins reads a `]]` standing where a
	// condition **term** belongs as an ordinary word, so the closer is only a
	// closer once the condition has something to close over.
	//
	// The five places a term may begin are the `[[` itself, a `!`, a `&&`, a
	// `||` and a group's `(`. Measured 2026-09-18, `env -i PATH=/usr/bin:/bin
	// LC_ALL=C`, `-c` and script files:
	//
	//	                   [[ ]] ]]   [[ ]] == x ]]   [[ ]] && x ]]
	//	ksh93u+ 2012-08-01  0          1               0
	//	bash 5.3.20         refuses    refuses         refuses
	//	zsh 5.9.2           refuses    refuses         refuses
	//
	// So one column takes the text as the two-character word `]]` and runs
	// the condition over it — true for being non-empty, compared with `x` as
	// a string, joined by the connective — and the other two refuse the token
	// wherever it stands. That is the whole of the flag, and the reason it is
	// the grammar's rather than a wording: what the parser **consumes** moves.
	//
	// It reaches a term's position only. An operand is a separate question
	// and every column answers it the same way: `[[ -n ]]` and `[[ x == ]]`
	// are refusals in that column too — ``]]' unexpected`` — so the closer is
	// still a closer behind an operator.
	//
	// zsh is **not** this, which is worth writing down because it looks like
	// it from one probe. That shell refuses the token as well; what it does
	// differently is report the refusal at the token *after* the closer, and
	// [Dialect.ConditionTermMissingBlamesTheTokenAfterTheCloser] is that.
	//
	// Where a newline follows the word, the newline itself is what is named,
	// and that is not this flag's — it is
	// [Dialect.ConditionNewlineMayFollowATermsFirstWord], which every column
	// but one answers the same way. `[[ ]]` reaches it here because the
	// closer is *read as the word* and a word standing alone is where that
	// question is asked (#2964, #3627).
	ConditionCloserIsAWordWhereATermBegins bool

	// ConditionNewlineMayFollowATermsFirstWord lets a condition term's first
	// word be the last thing on its line, with whatever decides the term —
	// a binary operator, the `]]`, a connective, a group's `)` — written on
	// the next.
	//
	// Additive, and one column adds it. Measured 2026-09-18, script files
	// under `env -i PATH=/usr/bin:/bin LC_ALL=C`, stdin `/dev/null`:
	//
	//	written                zsh 5.9.2  bash 5.3.20 and 3.2      ksh93u+
	//	[[ y \n ]]              runs       `newline', cond. binary  `newline'
	//	[[ y \n && -n z ]]      runs       the same                 the same
	//	[[ y \n || -n z ]]      runs       the same                 the same
	//	[[ ( y \n ) ]]          runs       the same                 the same
	//	[[ y \n == z ]]         runs       the same                 the same
	//	[[ y \n echo after      refuses    the same                 the same
	//	[[ -n x \n ]]           runs       runs                     runs
	//	[[ y == z \n ]]         runs       runs                     runs
	//	[[ ( -n x ) \n ]]       runs       runs                     runs
	//
	// So the question is asked at exactly one position — a term whose first
	// word has been read and whose **shape is not yet decided**, since a
	// binary operator may still follow it — and nowhere else. The last three
	// rows are the control: a term that is *finished* takes a newline in
	// every column, which is what says this is not a rule about newlines
	// inside conditions.
	//
	// Off is the core's answer because two of the three columns refuse, and
	// because what moves is what the parser **accepts**: on, `[[ y` and a
	// `]]` on the next line is a condition; off, it is a refusal naming the
	// newline. Reading it as core-accepts made this shell run a line bash
	// and ksh93 both reject.
	//
	// The newline is named where the parser stands **after** the blank lines
	// rather than at the first of them: three of them before an `echo` put
	// ksh93's complaint on the `echo`'s line, with `newline` still the word
	// quoted. That is the same skip
	// [Dialect.ConditionTermMissingBlamesTheTokenAfterTheCloser] measured one
	// construct over.
	//
	// Not the same question as a newline behind a *binary operator*, which
	// every column refuses and which the core already refused — `[[ 1 ==`
	// and a newline is `newline' unexpected` in ksh93 and `unexpected
	// argument `newline'` in bash. That one is an operator with no operand;
	// this one is a term with no verdict (#3627).
	ConditionNewlineMayFollowATermsFirstWord bool

	// ConditionTermMissingBlamesTheTokenAfterTheCloser reports a condition
	// with no term in it at the token **behind** the `]]` rather than at the
	// `]]` itself — the closer is consumed and whatever stands after it is
	// what the complaint names, at that token's own line.
	//
	// Measured 2026-09-18 over `[[ ]]` in every route, with `env -i`:
	//
	//	route                  bash 5.3.20         zsh 5.9.2
	//	-c                     `]]', line 1        `]]', line 1
	//	a file with a last NL  `]]', line 1        a newline, line 2
	//	a file without one     `]]', line 1        `]]', line 1
	//	standard input         `]]', line 1        a newline
	//	`[[ ]]` then `echo`    `]]', line 1        `echo', line 2
	//	`[[ ]]; echo after`    `]]', line 1        `;', line 1
	//	`[[ ]] echo after`     `]]', line 1        `echo', line 1
	//	`[[ ]] ]]`             `]]', line 1        `]]', line 1
	//	`[[ ]] == x ]]`        `]]', line 1        `==', line 1
	//
	// One column refuses the `]]` **as a token**, at the same place on every
	// route; the other consumes it and blames what follows, which is a
	// newline where there is one, the next word where there is one, and the
	// `]]` itself where the input simply ends. Blank lines between are
	// skipped: three of them before an `echo` put the complaint on the
	// `echo`'s line.
	//
	// One shape it does not cover, and it is measured rather than forgotten:
	// `[[ ]] && x ]]` is `condition expected: x` in that column — a run-time
	// complaint about a word, not a parse failure at a token — so the `&&`
	// there is read as the list operator it also is (#2964).
	ConditionTermMissingBlamesTheTokenAfterTheCloser bool

	// FunctionNamesRefused are the words this dialect will not let a function
	// definition bind, whatever else is true of them. Every one of them is a
	// perfectly good name, so nothing about the *spelling* is what refuses
	// it — the shell keeps the word for itself.
	//
	// Two columns have such a set and both draw it around the **special**
	// builtins rather than around the builtins: `set`, `eval`, `export`,
	// `readonly`, `shift`, `trap` and `unset` are refused in both, while
	// `true`, `read` and `cd` — regular builtins — are accepted by every
	// column including these two. The two sets are close and not equal, and
	// each difference is a fact about which builtins that shell makes
	// special, so the set is data here rather than one list with exceptions.
	// Measured 2026-09-15 on the `-c` route, a script file and standard
	// input, by defining a function of each name:
	//
	//	bash 5.3.15  defines every one of them, status 0
	//	bash 3.2.57  the same
	//	zsh 5.9.2    the same
	//	BusyBox ash  the same, in the digest-pinned alpine image
	//	dash 0.5.12  refuses 16 — the 15 POSIX marks special, and `local`
	//	ksh93u+      refuses 21 — those 15 less `times`, which is a preset
	//	             alias there rather than a builtin, plus `alias`, `enum`,
	//	             `hash`, `login`, `newgrp`, `typeset` and `unalias`
	//
	// `.` and `:` are in both of those counts and in neither set, because
	// both dialects already refuse them for their spelling — dash by the
	// name check its parentheses commit to, ksh93 by
	// interp.Semantics.PunctuatedFunctionNameIsRefused, which has a wording
	// of its own for a dot. A set that repeated them would take that
	// wording away.
	//
	// The stage splits the two, and it is the split
	// [Dialect.FunctionNameCheckedWhenTheDefinitionRuns] already records for
	// a name that is not a name: dash is a **syntax error while reading**, so
	// `printf a; export() { :; }; printf b` prints neither word and a
	// definition in a branch nothing takes is refused all the same; ksh93
	// parses the definition and complains where it **runs**, so the `printf`
	// in front of it runs, a definition inside a false branch is never
	// reached, and one inside a subshell ends the subshell alone. Which is
	// why this field carries no stage of its own.
	//
	// Nil for the four columns that refuse nothing, which is also what a
	// hand-built dialect has.
	FunctionNamesRefused map[string]bool

	// FunctionNameIsSourceText makes a definition's name the word as it was
	// **written** — quotes, backslashes and all — rather than the text the
	// word comes to. A name nobody could object to is refused when it is
	// spelled with quotes around it, because the quotes are in the name.
	//
	// bash alone, in all three of its spellings. Measured 2026-09-12 over
	// `sh -c "function 'f' { echo p; }; f; echo st=\$?"`, and again with the
	// `'f'() { … }` spelling, which answers the same:
	//
	//	bash 5.3.15  `` `'f'': not a valid identifier ``, then
	//	             `f: command not found` and st 127
	//	bash 3.2.57  the same two lines
	//	bash-as-sh   the same diagnostic, fatal at 2
	//	ksh93u+      `p`, st 0 — the quotes come off and `f` is defined
	//	zsh 5.9.2    the same
	//	dash         no keyword at all
	//
	// The name is `f`. Nothing about it is unusual except the quotes, which
	// is what makes this the *control* for the two flags either side of it:
	// [Dialect.FunctionKeywordNameIsAnyWord] and
	// [Dialect.FunctionNameIsAnyWord] are about a **wider set of names**, and
	// no set of names could exclude `f`. Every other spelling of the same
	// fact answers the same way there — `function "f"`, `function f""` and
	// `function \f` are all `not a valid identifier` naming the source text.
	//
	// It pairs with [Dialect.FunctionNameCheckedWhenTheDefinitionRuns] and is
	// not the same question: that one says *when* a bad name is refused, and
	// this one says what the name is. Without it the quotes came off here in
	// every dialect and `f` was defined — right for two columns and wrong for
	// three (#1566). With it the refused word reaches
	// [FuncDecl.RefusedName] as it always did, which is why the wording is
	// already what those three print.
	FunctionNameIsSourceText bool

	// FunctionDefinitionIsSourceText keeps a definition's own source text on
	// the declaration — see [FuncDecl.SourceText] — for the dialect that
	// writes a function back as it was **written** rather than as a tree.
	//
	// ksh93 alone. `typeset -f` there does not pretty-print: it reproduces
	// the characters the definition was spelled with, comments and odd
	// spacing and all, and ends with the character that ended the statement.
	// Measured 2026-09-13 on ksh93u+ 2012-08-01 through `cat -A`:
	//
	//	ksh -c 'f(){    echo     a   ;   }; typeset -f f'
	//	                     f(){    echo     a   ;   };
	//	ksh -c 'f() { :; }   ; typeset -f f'    f() { :; }   ;
	//	ksh -c 'eval "f() { :; }"; typeset -f f'    f() { :; }
	//	printf 'g() { :; }\ntypeset -f g' | file    g() { :; }\n
	//
	// So the span is the name through the terminator inclusive, and a
	// definition with no terminator after it — the last thing an `eval`
	// string holds — ends at its body. The other five dialects print from
	// the tree and never read this, which is why keeping the text is a
	// dialect's answer and not something every parse pays for.
	//
	// The parser is where it has to happen: the interpreter is handed a tree
	// and the source is gone by then. [Diagnostics.FunctionListingIsSourceText]
	// is the other half — this one keeps the text and that one writes it —
	// and both are needed, because a tree parsed by one dialect may be run by
	// another's vector and neither half may assume the other (#2610).
	FunctionDefinitionIsSourceText bool

	// PatternAlternation enables a bare `(a|b)` inside a pattern word, which
	// zsh has and the others do not: `a(b|c)` matches `ab` there. It is why
	// `@(abc|xyz)` is a literal `@` followed by a group in zsh rather than an
	// extended pattern — the same text, read by a different rule.
	PatternAlternation bool

	// PatternTopLevelAlternation reads a `|` standing outside every group and
	// bracket as an alternation of the whole pattern — `a|b` matching `a` or
	// `b` rather than the three characters.
	//
	// Read only from a bar a value supplied — `L='a|b'; [[ a = ${~L} ]]`, the
	// same value under `setopt globsubst`, or a `case` arm expanded from one.
	// Measured on zsh 5.9.2: all three match, and `[[ a = a|b ]]` is
	// `parse error near '|'` there and here alike.
	//
	// That last row used to be the whole argument — "the written spelling is
	// a parse error, so only a value can put one here" — and it is false
	// inside a `${…}`, where the braces keep the bar out of the command
	// grammar and it reaches the matcher as pattern text. zsh reads a written
	// bar there as an ordinary character: `v=abc; ${v#a|ab}` is `abc` while
	// `${v#${~L}}` with the same three characters in a value is `bc`. So the
	// provenance is arranged rather than implied, by interp's markWrittenBars
	// (#2168), and this flag still means what it says.
	//
	// Separate from PatternAlternation because the two are answered
	// independently: a dialect with bare groups need not read a bar outside
	// one, and #1331 fixed the group while leaving this — the two were
	// measured together and only one of them was about groups (#1497).
	//
	// It is a three-way rather than a bool because **two shells have the
	// reading and they do not have the same one** (#2528). See
	// [TopLevelAlternation] for the two, and for the contexts each reaches.
	PatternTopLevelAlternation TopLevelAlternation

	// BackgroundAndDisown reads `&!` and `&|` as terminators that start a
	// statement in the background and then let go of the job: nothing lists
	// it and nothing waits for it by number. zsh's, and the two spellings
	// are one operator — every probe below answers alike for both.
	//
	// Measured 2026-09-06 with `-n` over a *script file*, which is the only
	// instrument that answers this: a `-c` string reads `&!` differently,
	// and "did it parse" is not the question anyway. The panel does not
	// split the way a first look suggests:
	//
	//	`echo hi &!`   zsh disowns. bash 5.3 and ksh93 *parse* it — as `&`
	//	               followed by the `!` that negates a pipeline — and
	//	               leave the job in the table, which `jobs` then lists.
	//	               bash 3.2, bash-as-sh and dash refuse it outright.
	//	`echo hi &|`   zsh disowns. ksh93 parses it and means something
	//	               else — `echo hi &| echo done` prints only `done`
	//	               there. bash 5.3, bash 3.2, bash-as-sh and dash all
	//	               refuse it.
	//
	// So what is zsh's alone is the *disowning*, and the flag carries the
	// grammar half. What the other shells do with the same text is their
	// own grammar answering, and this flag does not reach them.
	//
	// The job is otherwise an ordinary background job, measured: `$!` is
	// still set to its process and `wait` still reports 0. Only the *table*
	// differs, which is why a later `&` job is `[1]` and not `[2]`.
	//
	// There is no corpus row for any of this, and the reason is worth
	// stating because it is not "nobody wrote one". Every snippet that puts
	// a `&!` where it can *run* reaches a second gap on the way: `!` in the
	// other shells is the pipeline negation, and what they do with a bare
	// one differs from what this shell does in both directions. `echo a &!`
	// alone is `a` in bash 5.3 and ksh93 and a syntax error in bash 3.2,
	// bash-as-sh and dash; ours accepts it everywhere. `sleep 0.4 &!` with a
	// line after it is accepted by bash 5.3 and ksh93 — the `!` negating the
	// *next* line's pipeline — and ours refuses it. So a row recording the
	// disowning would record four unrelated divergences beside it, and the
	// bare `!` is its own issue. The behavior is asserted in
	// syntax/disown_test.go and interp/disown_test.go instead.
	//
	// It does not change what happens at exit, and the first measurement
	// that said it did was an artifact: `zsh m.sh | tr …` holds the script
	// open for the whole `sleep 0.5` because the *pipe* is waiting for the
	// background job that inherited its standard output, not because the
	// shell is. Timed without a pipe, `sleep 0.5 &` and `sleep 0.5 &!` both
	// return in six milliseconds.
	BackgroundAndDisown bool

	// NumericRangePattern reads `<n-m>` in a word as a pattern matching a
	// run of digits whose *value* falls in the range, rather than as a
	// redirection: `<->` is any number, `<1-9>` a bounded one, and `<2->`
	// and `<-9>` are bounded on one side. zsh alone has it, and it is what
	// makes `[[ $1 = <-> ]]` a test rather than a parse error there.
	//
	// It reaches the lexer because `<` is a redirection operator everywhere
	// else, and the shape is the whole of the disambiguation — measured
	// 2026-09-05 on zsh 5.9.2, `<` then digits then `-` then digits then
	// `>`, and nothing else. `echo <1` reads the file `1`, `echo <a-b>` is a
	// redirection followed by a parse error at the `>`, and `<1-2-3>` and
	// `<-->` are parse errors too. Only the exact shape is a pattern.
	//
	// The digits in front of a redirection stop being a descriptor when the
	// operator turns out to be one of these: `echo 2<->` is one word there
	// and not a redirection of descriptor 2, and `{a}<->` is a word rather
	// than a descriptor the shell would pick.
	//
	// The matcher's half of the flag is read from here as well, the way
	// [Dialect.PatternAlternation] is: what a `<n-m>` matches is not a
	// grammar question, but which dialects have one to match is.
	NumericRangePattern bool

	// GlobQualifiers reads a parenthesized group where an *argument* may
	// stand as part of the word rather than as anything of the shell's:
	// `echo MY ( x )` is two words there, `MY` and `( x )`, and the second
	// is a pattern carrying a list of qualifiers. zsh alone has it, and in
	// the other four a `(` after a word is a syntax error.
	//
	// **Command position is the whole of the disambiguation**, measured
	// 2026-09-06 on zsh 5.9.2: `( x )` written where a command begins is a
	// subshell running `x`, and the same three characters after a word are
	// one argument. So the parser has to say which it is — the flag alone
	// cannot — exactly as it does for [Lexer.inPattern] and a condition.
	//
	// `setopt no_glob` is what proves the split is lexical rather than
	// interpretive: with globbing off, `echo MY ( x )` *prints* `MY ( x )`,
	// so the words are the same words and only what becomes of the group
	// has changed. The grammar half is therefore unconditional under the
	// flag, and the qualifier reading lives in the expansion, where whether
	// a word is a pattern at all is already decided.
	//
	// The group ends the word at a shell operator, which is measured rather
	// than assumed and is the reason it is not simply the balanced text:
	// `echo ( a <b )` is `parse error near `)'` there, with globbing on or
	// off, because the `<` ended the word and left the `)` with nowhere to
	// go. A `|` is the exception — `echo ( a|b )` is one word — because a
	// pattern group may hold an alternation.
	//
	// That is a rule about a group **starting a word** rather than about
	// this flag: a pattern operand's leading group ends at the same four
	// characters — `[[ $k == (a<b) ]]` is `parse error near `<'` there and
	// needs only [Dialect.PatternAlternation] — where a *regular
	// expression's* operand keeps them, in all four shells that have `=~`.
	// See Lexer.scanGroupSpans, which holds both measurements (#1175).
	//
	// The matcher's half is read from here too, the way
	// [Dialect.PatternAlternation] and [Dialect.NumericRangePattern] are:
	// which dialects read a trailing group as qualifiers is a grammar
	// question even though applying them is not.
	GlobQualifiers bool

	// DeclarationUtilities are the commands that may be given an array
	// assignment as an operand: `local a=(x y)`, `typeset -a b=()`.
	//
	// A grammar question rather than a runtime one, and name-sensitive:
	// `echo a=(x)` is a syntax error in bash and ksh93, so the parser has to
	// know which names take the form. It differs by dialect because the
	// utilities do — ksh93 has no `local` and no `declare`, and there
	// `local a=(x)` is the same syntax error `echo a=(x)` is.
	//
	// Empty means none, which is dash: it has no array literal at all.
	DeclarationUtilities map[string]bool

	// DeclarationArrayFromTheCommandWord is how the command word must be
	// written for a `name=( … )` operand behind it to be an array literal.
	//
	// Measured 2026-09-16 from script files, `env -i PATH=/usr/bin:/bin
	// LC_ALL=C`, `'typeset' a=(x y); echo "[${a[1]}]"` and the spellings
	// below, each in a script file of its own:
	//
	//	probe                   bash 5.3        zsh 5.9.2        ksh93u+
	//	typeset a=(x y)         [y]             [x]              [y]
	//	'typeset' a=(x y)       syntax error    a glob qualifier [y]
	//	\typeset a=(x y)        syntax error    a glob qualifier [y]
	//	type"set" a=(x y)       syntax error    a glob qualifier [y]
	//	cmd=typeset; $cmd a=(…) syntax error    a glob qualifier syntax error
	//
	// "a glob qualifier" is `unknown file attribute:` at 1 with the next line
	// still running: that shell reads the parenthesis as a qualifier on the
	// word `a=`, which is what it does with any argument that is not a
	// declaration's operand. bash has no such reading, so the `(` is left
	// standing behind a word and the file ends there. dash and BusyBox ash
	// have no array literal at all and the question does not reach them.
	//
	// The array route used to key on the command word with its quotes
	// removed, so every dialect took the array form from every spelling —
	// which is ksh93's answer given to two columns that refuse it (#3351).
	DeclarationArrayFromTheCommandWord DeclarationArrayWord

	// ReservedPrecommands are words that may stand in front of a simple
	// command, are taken away before it runs, and change nothing the command
	// can see. zsh's `nocorrect` is the only one measured: it turns off
	// spelling correction, which a non-interactive shell never does anyway,
	// so what is left is a word to be consumed.
	//
	// A *grammar* question rather than a runtime one, which is measured
	// rather than assumed and is the whole reason this is not a builtin like
	// `noglob` beside it (see interp.PrecommandModifier):
	//
	//	nocorrect x=1 echo hi     prints `hi` — `x=1` is still an assignment
	//	                          prefix, so the word was gone before the
	//	                          command was read
	//	noglob x=1 echo a[b]c     `command not found: x=1` — that one is a
	//	                          builtin, so what follows it is a command
	//	                          word and no longer an assignment
	//	x=nocorrect; $x echo hi   `command not found: nocorrect` — an
	//	                          expansion cannot produce it
	//	x=noglob; $x echo a[b]c   prints `a[b]c` — an expansion can produce
	//	                          that one
	//	\nocorrect echo hi        `command not found` — quoting removes the
	//	                          reservation, as it does for `if`
	//
	// It is recognized where a command word may first stand, which is after
	// a redirection or an assignment prefix as well as at the very start:
	// `>/dev/null nocorrect echo hi` and `x=1 nocorrect echo hi` both run
	// the command. And the word after it stands in command position, so an
	// alias there expands — `alias e=echo; nocorrect e hi` prints `hi`.
	//
	// Empty means none, which is the other four shells: `nocorrect` is an
	// ordinary command name there and `command not found` is the right
	// answer.
	ReservedPrecommands map[string]bool

	// ArrayLiteral enables `a=(x y)`. Absent from dash, where the `(` is a
	// syntax error rather than a different construct — so unlike `&>`, this
	// one is safe to be wrong about loudly.
	ArrayLiteral bool

	// CompoundVariableDeclarators are the command words that, standing first
	// inside `name=( … )`, make the parentheses a **compound variable's body**
	// rather than a list of array elements — and their presence at all is what
	// says the dialect has compound variables.
	//
	// Two constructs share one spelling, and what tells them apart is the
	// first word as it was *written*. ksh93u+ 2012-08-01, measured 2026-09-13
	// with `env -i PATH=/usr/bin:/bin` and a scratch HOME:
	//
	//	written                  typeset -p says
	//	c=(x y)                  typeset -a c=(x y)
	//	c=(a=1 b=2)              typeset -C c=(a=1;b=2)
	//	c=()                     typeset -C c=()            and so is an empty one
	//	c=(x b=2)                typeset -a c=(x b\=2)      the first word decides
	//	c=("a=1")                typeset -a c=(a\=1)        quoted is not an assignment
	//	w=a=1; c=($w)            typeset -a c=(a\=1)        nor is an expansion
	//	c=(typeset -i n=5)       typeset -C c=(typeset -i n=5)
	//	c=(integer n=1)          typeset -C c=(typeset -l -i n=1)
	//	c=(float n=1)            typeset -C c=(typeset -l -E n=1)
	//	c=(readonly x=1)         typeset -C c=(typeset -r x=1)
	//	c=(export x=1)           typeset -C c=(typeset -x x=1)
	//	c=(declare x=1)          typeset -a c=(declare x\=1)   not a word of this shell
	//	c=(local x=1)            typeset -a c=(local x\=1)     nor is this
	//	c=(set x=1)              typeset -a c=(set x\=1)       nor an ordinary builtin
	//
	// So the set is closed and small, and it is a *grammar* question rather
	// than a runtime one for the reason DeclarationUtilities is: the two
	// readings take a `;` differently, which is a parse and not a value.
	// Under SemicolonInAnArrayLiteral's ksh93 value one `;` *ends* the element
	// list — `a=( x; y )` is `` `y' unexpected `` — where the compound body
	// takes as many as there are members:
	//
	//	a=( x; y )               `y' unexpected
	//	c=(a=1; b=2)             typeset -C c=(a=1;b=2)
	//	c=(a=1; echo mid; b=2)   `echo' unexpected      a body holds declarations
	//	c=(a=1 && b=2)           `&&' unexpected        and nothing else
	//	c=(a=1; ; b=2)           `;' unexpected         each one needs a member
	//	c=(; a=1)                `;' unexpected
	//
	// Empty means the dialect has no compound variable, which is every column
	// but ksh93: there `c=(a=1 b=2)` is an indexed array of the two strings
	// `a=1` and `b=2`, which is also what this shell stored for every dialect
	// before #2620.
	//
	// **The empty literal is a compound too**, and the set's own emptiness is
	// what says so — there is no word in `c=()` to decide with, so what
	// decides is whether the dialect has the construct at all. Measured, and
	// the knock-ons are the reason it is worth stating: `${#c[@]}` answers 1,
	// `${c[0]}` is the tree's rendering, `${!c[@]}` is `0`, a later
	// `c+=(x y)` starts at subscript 1, and `[[ -v c ]]` is true. A prior
	// `typeset -a c` does not change any of it. The spelling is the ordinary
	// way a script starts an array, so nothing outside ksh may move — which
	// is what the emptiness of the set guarantees rather than promises.
	//
	// A **declaration's own letters** settle it ahead of either rule, which
	// is why [compoundLiteralReading] exists: `-a` and `-A` take the compound
	// reading off the table and `-C` puts it on, so `typeset -a c=(a=1 b=2)`
	// is an array of two strings and `typeset -C c=(x y)` is
	// `` `x' unexpected ``. That is a parse and not a store, because the
	// readings take a `;` differently: `typeset -a c=(a=1; b=2)` is
	// `` `b=2' unexpected `` there.
	//
	// One measured row is *not* modeled and is recorded rather than
	// reproduced: `typeset -A c=(a=1 b=2)` nests the compound under the key
	// `0` — `typeset -A c=([0]=(a=1;b=2))` — where the array reading this
	// takes reaches ksh93's own `cannot append index array to associative
	// array c`, which is what it answers for `typeset -A c=(x y)`. A value
	// under a key with a kind of its own is [SubscriptedArrayLiteralNests]'s
	// question rather than this one.
	CompoundVariableDeclarators map[string]bool

	// SemicolonInAnArrayLiteral is how far a `;` between the parentheses of
	// an array literal is taken. See [ArraySemicolon], where the rows are.
	//
	// It is a grammar flag rather than a semantics axis because the three
	// answers are three different *parses* of the same characters: one
	// refuses, one ends the element list, and one produces a different number
	// of elements.
	SemicolonInAnArrayLiteral ArraySemicolon

	// CloseBraceAlwaysReserved makes `}` a reserved word wherever a word may
	// stand, not only where a command may begin.
	//
	// It is what lets zsh write `{ echo hi }` with no terminator before the
	// brace: the `}` cannot be an argument, so it can only be closing the
	// group. The same rule is why `echo }` is a syntax error there and prints
	// a brace in the other six columns, which is the half that shows it is
	// one rule rather than a special case inside brace groups.
	//
	// **Reserved reaches into the word, which is the half that was missing.**
	// A `}` that ends a word is the reserved word and not the word's last
	// character, so no blank is needed in front of it either. Measured
	// 2026-09-12 on zsh 5.9.2, each line its own `zsh -c`:
	//
	//	echo A}          parse error near `}'
	//	echo A} B        parse error near `}'
	//	echo A}|cat      parse error near `}'
	//	echo "A"}        parse error near `}'
	//	{ echo A}        A            — the group closes with no `;` and no blank
	//	echo a}b         a}b          — not at the end of the word
	//	echo }a          }a           — nor is this
	//	echo A\}         A}           — quoting takes the reserved reading away
	//	echo A"}"        A}
	//
	// A `{` earlier in the same word matches it and takes the reading away
	// too, which is brace expansion's pairing seen from the lexer: `echo {a}`
	// and `echo a{b}` both print their braces, `echo {a}}` and `echo a{b}c}`
	// are both the parse error. Only bare text pairs — `echo ${x}}` is the
	// parse error, the expansion's braces counting for nothing.
	//
	// The one carve-out is an assignment's value, where the brace is text to
	// the end: `x=a}` and `x=}` both assign, where the same words as
	// arguments — `echo x=}` — are the parse error.
	CloseBraceAlwaysReserved bool

	// OpenBraceNeedsNoBlank makes a bare `{` where a command may begin the
	// reserved word on its own, however the text runs on after it.
	//
	// zsh alone, and it is the shortest spelling of a one-line function:
	// `a(){print A}` defines `a` there and is `{print: command not found` in
	// dash and ksh93 and `syntax error near unexpected token `{print'` in
	// bash 5.3, bash 3.2 and bash-as-sh. Measured 2026-09-12 on zsh 5.9.2:
	//
	//	{print A}        A
	//	{echo A; echo B} A then B
	//	{a,b}            command not found: a,b   — no brace expansion: the
	//	                                            `{` was the reserved word
	//	echo {print A}   parse error near `}'     — argument position keeps
	//	                                            the brace in the word
	//	'{'print A}      parse error near `}'     — quoted, so no group opens
	//	\{print A}       the same
	//
	// Command position is the whole of it, so a redirection's target, a
	// `case` subject, a `for` list and a pattern all keep the brace: `echo hi
	// > {a}` writes a file called `{a}` there, and `for i in {a,b}` expands
	// to two words.
	//
	// The closing half is [Dialect.CloseBraceAlwaysReserved], which the same
	// shell has and which the one-line spelling needs as well — `{print A}`
	// is three tokens and this flag reads only the first of them.
	OpenBraceNeedsNoBlank bool

	// EmptyCompoundBody lets a compound command stand with nothing in it:
	// `{ }`, `( )`, `while cond; do done`, `if cond; then fi`, and the
	// condition as well as the body — `if ; then :; fi`.
	//
	// zsh alone, and it is every shape rather than a rule about braces:
	// measured with `-n`, so that a parse is told apart from a loop that
	// never ends, dash, bash 5.3, bash 3.2 and ksh93 refuse all of them and
	// zsh takes all of them.
	//
	// Off in the core, which is what the common denominator means here: the
	// core is the language every panel shell accepts, and `{ }` is outside
	// that set because four of the five refuse it. A dialect adds it back,
	// which is the additive direction a grammar flag is for.
	//
	// Three neighbors are deliberately not this flag. A `case` with no arms —
	// `case x in esac` — is a list of *arms* rather than a command list, and
	// the one shell that refuses it on one line runs the same `case` with a
	// newline in front of the `esac`, which is
	// [Dialect.CaseTerminatorIsAPatternAfterTheHeader] and not an emptiness rule at
	// all. A command substitution's body — `x=$( )` — is a whole program
	// rather than a compound command's body, and every shell in the panel
	// takes an empty one. And a body written as a single stepped-over `;` —
	// `{ ; }` — is [Dialect.SteppedOverSeparatorIsABody]: ksh93 takes that
	// one and refuses `{ }`, so the two are measurably apart.
	EmptyCompoundBody bool

	// OpenEndedAndOr lets an and-or list end with its operator: the
	// right-hand side of a `&&` or a `||` may be absent where the list it is
	// in closes.
	//
	// zsh alone, measured 2026-09-07 over a script file with a scratch
	// `HOME`, `ZDOTDIR` and `HISTFILE`. `{ : || ⏎ }` runs there and is a
	// syntax error in bash 5.3, bash 3.2, bash-as-`sh`, dash and ksh93, each
	// of them naming the `}`. It reaches every closing context and is not a
	// rule about braces: `( : || )`, `if x; then : || fi`, `while false; do
	// : || done`, `until true; do : || done`, `case x in x) : || ;; esac`,
	// `case x in x) : || esac`, `if x; then : || else … fi`, `… elif …`, a
	// function body's `}` and a command substitution's `)` all take it.
	//
	// **The operator is dropped, not stood in for.** The status is the
	// left-hand side's: `false ||` answers 1 and `true &&` answers 0, so an
	// absent operand is neither an implicit `true` — which would make the
	// first 0 — nor an implicit `false`, which would make the second
	// non-zero. So nothing in the interpreter needs a value for this; the
	// parser returns the left-hand side and there is no operator left.
	//
	// **The pipeline does not take it, in any shell.** The leniency belongs
	// to the and-or list alone, which is the discriminating half of the
	// measurement: zsh refuses `true |` at the end of input, `{ true | ; }`,
	// `( true | )` and `true | )`, and names the token it found in each. A
	// flag that covered every joining operator would accept four lines zsh
	// rejects.
	//
	// **A terminator does not close the list for this purpose.** `true || &
	// b` and `true || ;;` with no `case` open are parse errors in zsh, so
	// `&` and a bare `case` terminator are not among the tokens that may
	// stand there. The set that does is the one every enclosing construct
	// stops on, which the parser's own atListEnd spells out.
	//
	// **The end of input is a separate question and not this flag.** zsh
	// takes `true &&` with nothing after it at all when it reads a script or
	// a `-c` string, and still draws a continuation prompt for the same text
	// typed at a terminal — measured through a pty, where zsh, bash and
	// ksh93 all prompt. So whether input that ran out ends the list depends
	// on the route it arrived by, the way [Dialect.ExpandAliases] does, and
	// it cannot be answered by a flag the parser reads on its own. Until it
	// is asked where it is answered, input ending on `&&` stays unfinished
	// in every dialect, which is right for the terminal in every column and
	// right for four of the five dialects everywhere else.
	OpenEndedAndOr bool

	// SeparatorWhereACommandBelongs lets a `;` stand where the grammar wants
	// a command, and steps over it.
	//
	// One rule rather than several, which is what the measurement says and is
	// the whole finding. Every shape two of the panel shells accept and the
	// other four refuse is the same thing seen in a different position:
	//
	//	; b                a list beginning with one
	//	a ; ; b            one between two statements
	//	a & ; b            the same after a `&` rather than a `;`
	//	a |& ; b           and after ksh93's coprocess terminator (#1141)
	//	a && ; b           where an and-or's right-hand side belongs
	//	a || ; b           the same for the other operator
	//	a | ; b            where a pipeline's right-hand command belongs
	//
	// **The `;` is skipped, not stood in for**, which `false` makes visible
	// and `true` hides: `false || ; echo two` prints `two` in both shells and
	// `true || ; echo two` prints nothing, so `echo two` really is the `||`'s
	// right-hand side and the `;` was absorbed the way the newline in
	// `a || ⏎ b` already is everywhere. Nothing runs for the separator itself
	// — `false ; ; echo $?` answers 1 in both, so it does not even set a
	// status.
	//
	// Measured 2026-09-07 over a script file with a scratch `HOME`, `ZDOTDIR`
	// and `HISTFILE`, `-n` and then a run. dash, bash 5.3, bash 3.2 and
	// bash-as-`sh` refuse every line above; ksh93 and zsh differ from each
	// other in exactly two ways, which is what the values below record.
	SeparatorWhereACommandBelongs SeparatorSkip

	// SubstitutionBodyRefusesASteppedOverSeparator takes that reading away
	// inside the body of a `$( … )` or a `${ …;}`, leaving only the position
	// where an and-or's right-hand side belongs.
	//
	// ksh93 alone, and it is the one place that shell's own answer does not
	// reach. Measured 2026-09-17 over a script file, `env -i
	// PATH=/usr/bin:/bin LC_ALL=C ksh s.sh` with stdin from /dev/null:
	//
	//	echo a; ;                 runs — the top level takes it
	//	v=`echo a; ;`             runs — and so does the older spelling
	//	v=`; echo a`              runs
	//	v=`false || ; echo b`     runs
	//	v=$(echo a; ;)            `;' unexpected
	//	v=$(; echo a)             `;' unexpected
	//	v=$(echo a; ; echo b)     `;' unexpected
	//	v=$(echo a & ; echo b)    `;' unexpected
	//	v=$(echo a | ; cat)       `;' unexpected
	//	v=$(if ; then echo a; fi) `;' unexpected
	//	v=$( ; )                  `;' unexpected
	//	v=${ echo a; ;}           `;' unexpected
	//	v=$(false || ; echo b)    runs, and the value is `b`
	//	v=$(echo a; )             runs — a separator *terminating* a
	//	                          statement is not this question
	//
	// So the two spellings of one construct part company, exactly as they do
	// for where a refusal is located and for whether a line continuation at
	// the front is removed — and the last two rows are what keep the rule
	// from being "the body refuses a `;`": an and-or still takes one, and a
	// terminator was never this.
	//
	// zsh takes every row, so the wider value is unaffected; bash 5.3, bash
	// 3.2, bash-as-`sh`, dash and BusyBox ash step over no separator anywhere
	// and so have nothing here to narrow.
	//
	// Read by the interpreter where the body is parsed rather than by the
	// parser, because only the caller knows it is reading a body and which
	// spelling opened it. See interp's bodyDialect.
	SubstitutionBodyRefusesASteppedOverSeparator bool

	// AbsentAndOrOperandIsAnEmptyCommand supplies a command that does nothing
	// and succeeds where an and-or's right-hand side is missing after a
	// separator was stepped over.
	//
	// ksh93's answer, and it is not [Dialect.OpenEndedAndOr]'s: the two
	// shells disagree about the *meaning* of the same text, which the
	// operator's short-circuit makes visible from one side only.
	//
	//	false || ;   ⏎ echo $?   →  0   ksh93     1   zsh
	//	false && ;   ⏎ echo $?   →  1   ksh93     1   zsh
	//
	// So it is an implicit success in ksh93 — `false || :` — and a dropped
	// operator in zsh, where the status is the left-hand side's. The `&&` row
	// agrees in both because the left-hand side failed and nothing on the
	// right was going to run either way, which is why the `||` row is the
	// only one that separates them.
	//
	// It is spelled here rather than in the interpreter's vector because the
	// difference is what *stands* in the tree and not what the tree means: an
	// empty command already runs and already answers 0, so this puts one
	// there and nothing downstream needs to know why.
	//
	// Only reachable where a separator was stepped over. ksh93 refuses
	// `false ||` and `{ false || ⏎ }` outright — an and-or may not simply end
	// with its operator there — so this is not that shell's spelling of
	// OpenEndedAndOr, and setting both would accept lines neither shell does.
	AbsentAndOrOperandIsAnEmptyCommand bool

	// SteppedOverSeparatorIsABody lets a `;` the dialect stepped over stand
	// for a compound command's whole body, so `{ ; }` is a brace group that
	// runs nothing.
	//
	// ksh93u+ alone, and it is measurably not [Dialect.EmptyCompoundBody] in
	// another spelling: that shell takes `{ ; }` and refuses `{ }`, where the
	// shell with the other flag takes both. Measured 2026-09-12 with `-n`,
	// `env -i PATH=/usr/bin:/bin` and a scratch HOME:
	//
	//	{ }                        `}' unexpected  — the control
	//	{ ; }                      runs
	//	{ ; ; }                    `;' unexpected  — only one is stepped over
	//	( ; )                      runs
	//	if :; then ; fi            runs
	//	if :; then :; else ; fi    runs
	//	if :; then :; elif :; then ; fi   runs
	//	while :; do ; done         runs
	//	until :; do ; done         runs
	//	for i in a; do ; done      runs
	//	select i in a; do ; done   runs
	//
	// The third row is why this reads the step-over rather than the token:
	// how many separators may stand there is
	// [Dialect.SeparatorWhereACommandBelongs]'s count, which for this shell
	// is one, and a body written as two `;` is refused by the same rule that
	// refuses `a ; ; ; b`.
	//
	// It runs nothing and sets no status of its own — `{ ; }; echo $?` is 0
	// and `false; { ; }; echo $?` is 0 as well — so the parser leaves the
	// body empty and nothing downstream needs a value for it.
	//
	// **One neighbor is deliberately not modeled.** After such a `;` that
	// shell takes a simple command and refuses a *compound* one: `{ ; :` runs
	// where `{ ; { :`, `{ ; ( :`, `{ ; if :; then :; fi`, `{ ; while …` and
	// `{ ; ! :` each name their own first keyword. That is a fact about what
	// may follow a stepped-over separator rather than about a body written as
	// one, it predates this flag — the same lines were accepted here before
	// it — and it looks like a parser's internal state rather than a grammar
	// anybody would write down. A reader comparing against ksh93 will find
	// it, so it is written down here rather than left to be rediscovered
	// (#2231).
	SteppedOverSeparatorIsABody bool

	// EmptyBodyBlame is which token a refusal names when a terminator stands
	// where a body or a condition must have something in it.
	//
	// Every shell in the panel refuses `if & then :; fi`, at the same line,
	// and only the quoted token differs — so this is a wording and not a
	// grammar. Measured 2026-09-14, `-n` over a script file under `env -i`
	// with a scratch `HOME` and `ZDOTDIR`:
	//
	//	                        dash   bash 5.3  ksh93   zsh
	//	if & then :; fi          `&`     `&`      `then`  `&`
	//	while & do :; done       `&`     `&`      `do`    `&`
	//	if & fi                  `&`     `&`      `fi`    `&`
	//	{ & }                    `&`     `&`      `}`     `&`
	//	( & )                    `&`     `&`      `)`     `&`
	//	if & ; then :; fi        `&`     `&`      `;`     `&`
	//	if | then :; fi          `|`     `|`      `|`     `then`
	//	while | do :; done       `|`     `|`      `|`     `do`
	//	if && then :; fi         `&&`    `&&`     `&&`    `then`
	//	if | ; then :; fi        `|`     `|`      `|`     `then`
	//	if | :; then :; fi       `|`     `|`      `|`     `then`
	//	{ | }                    `|`     `|`      `|`     `|`
	//	( | )                    `|`     `|`      `|`     `|`
	//	if ;; then :; fi         `;;`    `;;`     `;;`    `;;`
	//
	// Two shells step over the terminator and name what is behind it, over
	// **different sets of terminators** — ksh93 for `&` and zsh for the
	// pipeline and and-or operators — and they do not step the same way,
	// which is what makes this an enum rather than a bool. See the values.
	//
	// The terminators each one applies it to are [Dialect.EmptyBodyBlamed].
	// A `;` is in neither set and is [Dialect.SteppedOverSeparatorIsABody]'s
	// and #2023's question instead; `;;`, `;&` and `;;&` are in neither and
	// every shell names them where they stand.
	//
	// **This models the refusal and not the acceptance.** Both shells also
	// *take* such a terminator in a position where the list may be empty —
	// ksh93 runs `if :; then & :; fi` and `{ & :; }`, zsh runs nothing of
	// the kind — and taking it is a grammar question that changes what a
	// script does rather than what a refusal says. So where the token after
	// the terminator could begin a command this names the terminator, which
	// is what every column that refuses the line writes and what this shell
	// already wrote (#2235).
	EmptyBodyBlame EmptyBodyBlame

	// EmptyBodyBlamed is the set of terminators [Dialect.EmptyBodyBlame]
	// applies to. Empty means none, which is dash's and bash's answer and
	// the substrate's own.
	EmptyBodyBlamed map[Kind]bool

	// RegexTakesAlternation makes a bare `|` part of a `=~` operand rather
	// than the end of the word. bash and ksh93 say yes, so `[[ ab =~ a|b ]]`
	// matches there; zsh says no and reports a parse error. Parentheses are
	// taken by all three and so need no flag — a dialect without `[[ ]]`
	// never reaches the question.
	RegexTakesAlternation bool

	// ArraySubscript enables `${a[i]}`, `${a[@]}` and `${a[*]}`, `a[i]`
	// inside an arithmetic expression, and the element assignment `a[i]=v`
	// and `a[i]+=v`. Absent from dash, which has no arrays at all and calls
	// the subscript a bad substitution rather than reading it — a separate
	// flag from ArrayLiteral because the two halves are separately
	// reachable: a subscript can be written for a variable that was never an
	// array.
	//
	// The assignment shape is this flag's rather than a fourth one, and that
	// is measured rather than assumed: reading a subscript and writing
	// through one split the panel the same way, with bash 3.2, bash 5.3,
	// ksh93 and zsh on one side and dash alone on the other. zsh's refusal
	// of `a[0]=x` is not a third answer — it parses the assignment and
	// rejects the *subscript*, which is the array-base axis and belongs to
	// the semantics vector. A flag no dialect can be given a different value
	// for is a field nothing reads, which is what #564 is about.
	//
	// Where it is off, `a[0]=x` is a command name and not an assignment —
	// the shell without arrays reports `a[0]=x: not found` and carries on.
	// Getting that wrong is silent on the permissive side: the element is
	// stored, nothing is reported, and a script written against the
	// no-array dialect on purpose is told it is portable when it is not.
	ArraySubscript bool

	// SubscriptQuoteProtectsTheClosingBracket says which quoting constructs
	// written inside a `${name[ … ]}` subscript hold a `]` back from ending
	// it, so that the subscript runs to the bracket *after* the quoted text.
	//
	// The read side only. The write side is already the lexer's and is
	// already unanimous — see SubscriptSpansSeparators, whose measured table
	// ends on `m['a]b']=v`, the key `a]b` in every column that has arrays.
	// Reading `${m['a]b']}` back is the same question put to a different
	// scanner, and the panel does not answer it the same way, which is why
	// it is a field rather than a constant.
	//
	// Measured 2026-09-15 with an indexed array, so that the answer is a
	// *diagnostic* rather than a value and the two readings cannot be
	// confused. `a=(9 8 7); echo "[${a[<q>0]<q>+1]}]"`, where <q> is the
	// quoting under test: where the quote protects, the subscript is the
	// whole of `0]+1` and the shell reports an arithmetic error naming it;
	// where it does not, the subscript ends at the quoted `]`, the `+` that
	// follows is read as the alternate-value operator and the expansion is
	// the text `1]` — the answer `[1]]`, with nothing reported.
	//
	//	                      \]      '…]…'   "…]…"   $'…]…'
	//	bash 5.3.20           yes     yes     yes     no
	//	bash 5.3.20 as sh     yes     yes     yes     no
	//	bash 3.2.57           no      no      no      no
	//	ksh93u+ 2012          yes     yes     yes     yes
	//	zsh 5.9.2             yes     no      no      no
	//	dash, BusyBox ash     no subscripts at all, so the question is never put
	//
	// POSIX mode does not move it: `--posix` and the name `sh` both answer
	// with the bash row above, measured the same day. That is why there is no
	// companion field here, where QuoteProtectsTheClosingBrace needed one.
	//
	// The `$'…'` column is measured and *not* modeled, and the reason is in
	// closingBracket: bash decodes the escapes before it looks for the
	// bracket, so a `]` inside `$'…'` is a bare one by the time the scan
	// runs. Reading the run whole, as this scan does, keeps the ordinary
	// `${a[$'k']}` working in every dialect that has the construct and
	// leaves `${a[$'x]y']}` disagreeing with bash — in a different word, at
	// the same size, as it did before any of this quoted anything.
	//
	// Whether the quote's own bytes then reach the subscript is a *separate*
	// question and is not this flag's: bash keeps a single quote's, so
	// `${a['0]'+1]}` is an arithmetic error against `'0]'+1` with the quotes
	// still in it, and removes a double quote's, reporting `0]+1`. ksh93
	// removes both. This flag decides only where the subscript ends.
	SubscriptQuoteProtectsTheClosingBracket SubscriptQuoting

	// SubscriptSpansSeparators lets a subscript written at command-word
	// position hold a separator: `m[foo bar]=v` assigns the element keyed
	// `foo bar`, where a grammar without the flag ends the word at the blank
	// and runs `m[foo` as a command.
	//
	// **It is the word boundary and so it is the lexer's**, the way
	// BareSubscript is. Once a name at command position is followed by `[`,
	// the text runs to the *matching* `]` and everything in between is
	// ordinary characters of the subscript — which is more than blanks.
	// Measured 2026-09-12 on bash 5.3.15, bash 3.2.57, bash as `sh` and
	// ksh93, each row read back with `typeset -p m`:
	//
	//	m[foo bar]=v      the key `foo bar`
	//	m[foo	bar]=v    a tab the same way
	//	m[foo\nbar]=v     and a newline: the word spans the line
	//	m[a; b]=v         and a `;`, a `|`, a `>` and an `&&`
	//	m[#c]=v           a `#` is not a comment in there
	//	m[a [b] c]=v      brackets nest, so the *matching* `]` ends it
	//	m['a]b']=v        a quoted `]` closes nothing
	//
	// A name is required in front of the bracket and it is required to be a
	// name: `1m[foo bar]=v`, `m-n[foo bar]=v` and `[foo bar]=v` all still
	// end at the blank in the same four shells. So does an argument —
	// `printf '<%s>' m[foo bar]=v` prints two fields everywhere — which is
	// why this asks atCommandWord rather than applying to every word.
	//
	// The `=` is not part of the condition, because the shells do not make
	// it one: `m[foo bar]` alone is `m[foo bar]: command not found` in all
	// four, quoting back the whole word. An assignment prefix is command
	// position too, so `a=1 m[foo bar]=v` writes the element.
	//
	// Additive and not a semantics axis: zsh 5.9.2 is the panel's holdout
	// and it does not mean something else by the text, it refuses it —
	// `bad pattern: m[foo`, exit 1 — and dash and BusyBox ash have no
	// arrays to subscript at all. So four columns read the text one way,
	// one refuses it outright, and two never reach a subscript: there is no
	// second *meaning* for a semantics axis to switch between.
	//
	// Where the bracket has **no** matching `]` the word is left exactly as
	// a grammar without the flag reads it, and that is deliberate. The two
	// shells that span separators diverge there — bash refuses with
	// `unexpected EOF while looking for matching ']'` and ksh93 swallows the
	// rest of the input into the word — so there is no common answer to
	// implement, and falling back means no input that parses today parses
	// differently tomorrow.
	SubscriptSpansSeparators bool

	// ArrayLiteralShapeFollowsTheFirstElement makes a compound literal one
	// shape or the other rather than a mixture: **either** every element
	// names a subscript — `[sub]=value`, or `[sub]+=value` — **or** none of
	// them does and a bracket in front of an element is text. The first
	// element decides which, and a bare element after a subscripted one is a
	// *parse* error rather than a word.
	//
	// Measured 2026-09-14 on ksh93u+ 2012-08-01 (`/bin/ksh`), against bash
	// 5.3.15, bash 3.2.57 and zsh 5.9.2, each row read back with
	// `typeset -p a`:
	//
	//	a=([1]=A [2]=B)   typeset -A a=([1]=A [2]=B)      subscripted throughout
	//	a=(p [1]=A)       typeset -a a=(p '[1]=A')        a word list, brackets and all
	//	a=(p q [1]=A)     typeset -a a=(p q '[1]=A')      and however far along it stands
	//	a=("[1]=A" p)     typeset -a a=('[1]=A' p)        a quoted head was never a subscript
	//	a=([1]=A p)       syntax error at line 1: `p' unexpected
	//	a=([1]=A "b")     syntax error at line 1: `b' unexpected
	//	a=([1]=A $x)      syntax error at line 1: `$x' unexpected
	//
	// The refusal is the grammar's and not the store's, which the last two
	// rows are what say: the offending element is named as it was *written*,
	// with `$x` unexpanded and the quotes off `"b"`, and it is refused
	// whatever `x` holds and whatever the name held before the assignment.
	// `a=([1]=A` + newline + `p)` is refused too, at the line the bare
	// element stands on, so the literal's own newlines do not end the rule.
	//
	// **The appending spelling is shaped by the same rule and not exempt**,
	// which is where #2505 as filed was wrong. `a+=([1]=A p)` is the same
	// syntax error, and `unset a; a+=([1]=Z [2]=Y)` is
	// `typeset -A a=([1]=Z [2]=Y)` — a subscripted literal, read as one.
	// What is separately true of `+=` is a *store* rule rather than a
	// grammar one: an append whose name is already holding an indexed array
	// cannot become a keyed one, and there the subscripted reading is given
	// up and the elements go in as the words they were written as. That half
	// is [interp.Runner.literalReadsSubscripts]'s and is measured there.
	//
	// Nothing else in the panel reads a literal this way. bash and zsh place
	// a subscripted element wherever it stands and continue the bare ones
	// from it, so `a=(x [3]=y z)` is three elements there and three *words*
	// here; dash and BusyBox ash have no array literal to shape.
	//
	// The word-list shape reaches the lexer too, because the blanks inside a
	// bracket are only characters while a subscript is being read:
	// `a=(p [1 2]=A)` is the three words `p`, `[1` and `2]=A` here, where
	// `a=([1 2]=A)` is the single key `1 2`. See
	// [Dialect.SubscriptSpansSeparators], which is the flag that spans them,
	// and Parser.arrayLiteral, which stops asking it once a bare first
	// element has settled the shape.
	//
	// It is a grammar flag and not a semantics axis because the two readings
	// are not two meanings for one parse: one of them is a parse error, and
	// the other changes where the word boundaries fall.
	ArrayLiteralShapeFollowsTheFirstElement bool

	// SubscriptSpansSeparatorsInRedirect extends the reading above to a
	// redirection's target, so `> m[foo bar] echo hi` writes one file named
	// `m[foo bar]` where a grammar without the flag writes `m[foo` and then
	// runs `bar]`.
	//
	// Separate from SubscriptSpansSeparators because the panel splits them.
	// Measured 2026-09-13 on ksh93u+ 2012-08-01 against bash 5.3.15, bash
	// 3.2.57 and bash as `sh`, each run in an empty directory and read back
	// with `ls`:
	//
	//	> m[foo bar] echo hi      ksh93 `m[foo bar]`; every bash `m[foo`
	//
	// So the four columns that span at command position are three-to-one
	// against spanning here, and this is one shell's answer rather than the
	// common rule — which is why #2410 followed bash and stopped, and why
	// this is a flag of its own rather than a widening of that one.
	//
	// **The position is "a redirection whose target stands where a command
	// may begin", not "a redirection".** Measured the same day, and this is
	// the half that is easy to get wrong:
	//
	//	> m[foo bar] echo hi      spans — the redirection is a prefix
	//	2> m[foo bar] echo hi     spans — an IO number changes nothing
	//	{ :; } > m[foo bar]       spans — a compound command's redirection
	//	for i in x; do :; done > m[a b]   spans, the same way
	//	echo hi > m[foo bar]      **splits** — an argument stood first
	//	echo hi 3> m[foo bar]     splits, the same way
	//
	// The condition in front of the bracket is the one
	// SubscriptSpansSeparators already carries and is not restated: a name
	// and nothing else, so `> 1m[foo bar]`, `> m-n[foo bar]` and
	// `> [foo bar]` all still end at the blank in ksh93. Text *after* the
	// matching `]` stays part of the word — `> pre[1 2]post` names one file
	// — because the bracket ends the subscript and not the word.
	//
	// A `case` subject is the near miss and it is why the lexer has a flag
	// of its own for this position: it is read where a command may begin and
	// takes no assignment, exactly as a redirection prefix does, and ksh93
	// **splits** there — `case m[foo bar] in *) ;; esac` is
	// the message `bar]' unexpected.
	//
	// Additive on the same terms as SubscriptSpansSeparators: with no
	// matching `]` the word is left exactly as a grammar without the flag
	// reads it. ksh93 swallows the rest of the input there
	// (`> m[foo bar echo hi` writes a file called `m[foo bar echo hi[`) and
	// bash refuses, so there is no common answer to reach for and nothing
	// that parses today parses differently with the flag on.
	SubscriptSpansSeparatorsInRedirect bool

	// SpecialParamSubscript lets a parameter that is *not* a name carry a
	// subscript: `${@[1]}` and `${*[2]}` name one of the positional
	// parameters, and `${1[2]}`, `${0[1]}`, `${?[1]}`, `${-[1]}` and
	// `${$[1]}` reach into the value of a special one.
	//
	// One shell in the panel. Measured on zsh 5.9.2 against bash 5.3.15,
	// bash 3.2.57, bash as `sh`, ksh93 and dash: `set -- a b; echo ${@[1]}`
	// prints `a` there, where bash calls the whole expansion
	// `${@[1]}: bad substitution` when it is reached, ksh93 refuses `[' at
	// parse time and dash says `Bad substitution`. Every one of those five
	// refuses; none of them means something else by it. So this is the same
	// kind of split ArraySubscript is — a construct one grammar has and the
	// rest do not — rather than a conflict for the semantics vector.
	//
	// It is the *name* that this flag is about and not the subscript, which
	// is why it is separate from ArraySubscript: `${a[1]}` is read by four
	// of the six and `${@[1]}` by one, so a single flag could not say both.
	//
	// Where it is off the bracket is simply not consumed, and the leftover
	// text takes the ordinary route an unreadable expansion takes — deferred
	// to the run in most of the panel, refused while reading by the one
	// grammar that refuses everything else while reading. Nothing here has
	// to word a diagnostic of its own.
	//
	// `#` and `!` are unreachable in the braced spelling for the reason
	// BareSubscript records: `${#[1]}` is a length and `${![1]}` an
	// indirection, so there is nowhere to write them down.
	SpecialParamSubscript bool

	// ArraySubscriptFlags enables a parenthesized flag group at the front of
	// a *subscript* — `${a[(re)value]}`, the first element equal to the
	// operand — which is a different construct from the flag group
	// ParamExpansionFlags enables and not the same one moved. The three
	// differences are set out at the head of subscriptflags.go, and the
	// character sets are separate for the first of them.
	//
	// Additive rather than a semantics axis, and measured rather than
	// argued: the grammar that has the group falls back to arithmetic for
	// any group it cannot read, which is exactly what the four grammars
	// without it do with every group — `${a[(z)2]}` is an arithmetic failure
	// in all six shells on the panel and only `${a[(r)beta]}` divides them.
	// So there is no text this flag gives a *second* reading to; it gives a
	// reading to text that had none.
	//
	// It rides on ArraySubscript, which is what makes the brackets a
	// subscript in the first place, and on BareSubscript for the unbraced
	// form. A grammar with this and neither of those has nowhere to put a
	// group.
	ArraySubscriptFlags bool

	// BareSubscript lets a parameter written without braces carry a
	// subscript, and lets `$#name` mean that parameter's length: `$a[1]` is
	// an element and `$#a` is a count, where a grammar without the flag
	// reads `$a` followed by the three characters `[1]`, and `$#` followed
	// by the letter `a`.
	//
	// One shell in the panel, measured on zsh 5.9.2 against bash 3.2, bash
	// 5.3 and dash: `a=(x y z); echo $a[1]` prints `x` there and `x[1]`
	// everywhere else, and `echo $#a` prints `3` there and `0a` everywhere
	// else. So this is a *grammar* question and not a value on the semantics
	// vector — the same characters are two different words, not one word two
	// shells disagree about the meaning of — which is why it sits here beside
	// ShortLoop rather than in a dialect package.
	//
	// It is the lexer's, because the word boundary is: `$a[1]` is one
	// expansion where the flag is on and an expansion plus a glob pattern
	// where it is off, and nothing downstream can tell them apart once the
	// spans are cut. Getting it wrong is loud in one direction and silent in
	// the other — a shell without the flag globs `x y z[1]` and reports no
	// matches, while a shell with it applied everywhere would quietly turn
	// every `$dir[0-9]*` in a bash script into an element lookup.
	//
	// The subscript follows a name, `@` or `*`. Not the positional digits:
	// measured, `set -- abcd; echo $1[2]` prints `abcd[2]` there, so the
	// parameters that carry one are not simply all of them. The remaining
	// specials do take one — `$?[1]`, `$-[2]`, `$$[1]` and `$0[2]` are all
	// subscripted — and are left out here because a span has no way to write
	// two of them down: `${#[1]}` is a length and `${![1]}` an indirection,
	// so the inner text of `$#[1]` and `$![1]` would say something else.
	//
	// `$#` takes a name, a digit, `@` or `*` after it. Not `#` and not `!`:
	// `$##` prints `2#` and `$#!` prints `2!` on the same shell, so the
	// length form stops at exactly two of the specials rather than at all of
	// them.
	BareSubscript bool

	// BareParamFlags enables zsh's unbraced flag sigils: a flag character
	// between the `$` and the name, without braces. `$+v` is `${+v}`, `$=v`
	// is `${=v}`, `$~v` is `${~v}` and `$^a` is `${^a}` — the same four
	// flags the braced group already takes, written the short way.
	//
	// One shell in the panel. Measured on zsh 5.9.2 against bash 5.3.15,
	// bash 3.2.57, bash as `sh`, ksh93 and dash: `v=1; echo $+v` prints `1`
	// there and the literal `$+v` everywhere else, and the same for the
	// other three.
	//
	// It is the lexer's for the reason BareSubscript is: the word boundary
	// moves. `$=v` is one expansion where the flag is on and a literal `$`
	// followed by `=v` where it is off, and nothing downstream can tell them
	// apart once the spans are cut. Separate from BareSubscript because it
	// is a separate question — that one decides whether `[` after a name
	// belongs to the expansion, and a shell could sensibly answer the two
	// differently.
	//
	// The four do not all take the same target, which is why the lexer asks
	// what follows. `$+` takes a name or a digit and nothing else: measured,
	// `$+@`, `$+*`, `$+?` and `$+$` all print themselves, and so does `$++v`
	// — the sigil does not repeat. The other three take every target a bare
	// `$` does, specials included, and are flags even with no target at all,
	// since a bare `$=` expands to nothing there rather than printing.
	BareParamFlags bool

	// BareParamModifiers lets a parameter written without braces carry a
	// history-style modifier list: `$p:t` is the tail of `$p`, and `$b:q` is
	// each element of `$b` quoted.
	//
	// One shell in the panel. Measured on zsh 5.9.2 against bash 5.3.20,
	// bash 3.2.57, bash as `sh`, ksh93u+, dash and BusyBox ash, 2026-09-18
	// with `s='p q'`: `$s:q` is `p\ q` there and the six characters `p q:q`
	// in all six of the others — which is what this grammar already did, so
	// the flag is what one shell adds rather than what six of them lose.
	//
	// It is the lexer's for the reason BareSubscript and BareParamFlags are:
	// the word boundary moves. `$s:q` is one expansion where the flag is on
	// and an expansion followed by two literal characters where it is off,
	// and nothing downstream can tell the two apart once the spans are cut.
	//
	// **The braced spelling is not this**, which is the measurement that
	// makes the flag about the *bare* form alone: `${b}:q` leaves the `:q` as
	// text in that shell too, and `${b:q}` is the modifier written inside the
	// braces, which is ParamSubstring and needs no flag. So a grammar with
	// modifiers in braces and without this one is a coherent grammar and not
	// a half-finished one.
	//
	// **The bare form takes the letter and nothing after it**, where the
	// braced form takes a count. Measured with `p=/a/bb/c.txt`: `${p:h2}` is
	// `/a` — the head twice — and `$p:h2` is `/a/bb2`, the head once with a
	// literal `2` after it. `$p:t2` is `c.txt2` the same way, and `$s:qX` is
	// `p\ qX`. The one exception is `:s`, which takes its whole delimited
	// substitution: `$p:s/a/Z/x` is `/Z/bb/c.txtx`.
	//
	// **A letter that names no modifier is not one**, and there is no
	// complaint: `$s:zz` is `p q:zz` and `$s:` is `p q:`, where the braced
	// `${s:zz}` is refused. So the scan gives the colon back rather than
	// failing on it, which is what lets a bare `:` after an expansion go on
	// meaning whatever it meant.
	BareParamModifiers bool

	// MultiDigitPositional makes a run of digits after an unbraced `$` one
	// positional parameter: `$10` is the tenth, not `$1` followed by a `0`.
	//
	// One shell in the panel. Measured 2026-09-15 with
	// `set -- 1 2 3 4 5 6 7 8 9 ten eleven twelve` on zsh 5.9.2, bash 5.3,
	// bash 3.2, bash as `sh`, ksh93, dash and BusyBox ash: `$10` is `ten` in
	// zsh and `10` — `$1` then the character — in the other six, and `$11` is
	// `eleven` against `11` the same way. The **braced** `${10}` is the tenth
	// in every one of the seven, which is what says this is about where the
	// unbraced token ends and not about what a positional name means.
	//
	// It is the lexer's for the reason BareSubscript and MultiDigitFdNumber
	// are: the word boundary moves. `$10` is one expansion where the flag is
	// on and an expansion plus a literal `0` where it is off, and once the
	// spans are cut nothing downstream can tell the two readings apart. And
	// it is a grammar flag rather than a semantics axis for the same reason —
	// the two shells are not disagreeing about the value of a parameter, they
	// are reading different tokens.
	//
	// The run is read as a *number*, which is what decides the leading-zero
	// spellings rather than a rule about the digits: measured on the same
	// binary, `$01` is the first parameter, `$09` is the ninth, `$010` is the
	// tenth, and `$00` is the shell's own name. So the name the span carries
	// is whatever digits were written and whoever resolves it reads the
	// number — the same division FdVariablePositional makes, and the reason
	// the lexer does not normalize the text it took.
	//
	// Neighboring forms are unaffected and were measured beside it. A digit
	// run still stops at the first non-digit, so `$1a` is `$1` then `a` in
	// every column; a run still takes no bare subscript, so `$1[2]` is
	// unchanged; and `$#10` is the length of the tenth parameter in the shell
	// that has both this and BareSubscript, which falls out of the two
	// without a rule of its own.
	//
	// POSIX is the reason this is a flag rather than a fault in six shells:
	// XCU makes `$10` the first positional parameter followed by a `0` and
	// says the multi-digit form has to be braced, so the majority conforms
	// and the one shell that reads further is the extension.
	MultiDigitPositional bool

	// ChainedSubscript lets a braced expansion carry more than one subscript,
	// each reading what the one before it named: `${m[k][2]}` is the second
	// *character* of the value under `k`, and `${a[2,4][1]}` the first
	// *element* of the three the range named. Which of the two a subscript
	// counts is decided by what it is handed rather than by where it stands,
	// so a chain is the same rule applied twice and not a new one.
	//
	// One shell in the panel, measured 2026-09-08 on zsh 5.9.2 with
	// `a=(one two three); ${a[1][2]}`: this shell answers `n`, bash 5.3.15
	// and that binary as `sh` both answer `${a[1][2]}: bad substitution`,
	// dash has no arrays to subscript, ksh93 answers empty, and bash 3.2.57
	// answers `two` — the first subscript read and the second ignored, which
	// is the reading none of the others has and is not the one this enables.
	// So the text divides the panel, and this is the grammar that has it.
	//
	// Braced only. Measured on the same binary: `$m[k][2]` unbraced is one
	// subscript and the rest is a pattern — `no matches found: abc[2]` — so
	// the bare spelling keeps its single bracket, which is what
	// Lexer.bareSubscript already reads.
	//
	// It rides on ArraySubscript, which is what makes the first bracket a
	// subscript at all. `~/.zi/bin/zi.zsh` writes `${ICE[atload][1]}` ten
	// times, in the function that decides whether an `atload'!…'` ice needs
	// tracking (#1516).
	ChainedSubscript bool

	// SubscriptDotRange reads a braced expansion's subscript written
	// `lo..hi` as a range of elements rather than as one arithmetic
	// expression: `${a[1..3]}` is the elements 1 through 3, as separate
	// fields, where every other grammar hands `1..3` to the arithmetic and
	// refuses it.
	//
	// Measured 2026-09-16 with `a=(a b c d e)`, script files under `env -i`:
	// ksh93u+ 2012-08-01 answers `b c d` and `"${a[1..3]}"` is three fields;
	// bash 5.3 calls it an arithmetic syntax error, and zsh 5.9.2 a bad
	// floating point constant. An additive split, so a grammar flag.
	//
	// The `..` must be written, in a literal of the subscript — quoted or not
	// — and not produced: `x=1..3; ${a[$x]}` is the arithmetic refusal in the
	// shell with the construct, and so is `${a[1\..3]}`. The first one
	// written is the separator, which is why `(1..3)` splits inside its
	// parentheses there and is refused as unbalanced. What the range then
	// names is the run's — see interp/dotrange.go.
	SubscriptDotRange bool

	// ChainedAssignSubscript lets an assignment's name carry more than one
	// subscript, where the later ones reach *into* the value the earlier one
	// named rather than being part of its text: `a[1][2]=v` puts `v` at 2 of
	// the array held in element 1.
	//
	// One shell in the panel, and it is not the one ChainedSubscript is for.
	// Measured 2026-09-14 on ksh93u+ 2012-08-01, `env -i` with a scratch
	// HOME, read back with `typeset -p a`:
	//
	//	a[1][2]=v         typeset -a a=([1]=([2]=v) )   and ${#a[@]} is 1
	//	typeset a[1][2]=v the same
	//	a[1]=([2]=v)      the same, which is the spelling this engine had
	//
	// bash 5.3 and zsh both refuse the operand — `not a valid identifier`
	// and `no matches found`, the brackets being a pattern there — and bash
	// 3.2 declares an empty array under the base name at status 0. So four
	// answers among the shells that reach it and this is the grammar for the
	// one that nests (#2491).
	//
	// It is the *write* half alone. What `${a[1][2]}` reads in that shell is
	// a second question and a different reading from the one
	// ChainedSubscript enables — see #2830.
	ChainedAssignSubscript bool

	// ArithCharacterCode enables `#name` and `##c` inside an arithmetic
	// expression: the code of the first character of a parameter's value, and
	// the code of a character written out.
	//
	// One shell in the panel. Measured 2026-09-05 on zsh 5.9.2, where
	// `b=zebra; echo $((#b))` prints 122 and `echo $((##a))` prints 97
	// — against bash 5.3.15, bash 3.2.57, bash as `sh`, ksh93 and dash, every
	// one of which calls the same text an arithmetic syntax error naming the
	// operand it could not read. An operator five shells refuse and one has
	// is the additive kind of split, so it is a flag here rather than a value
	// on the semantics vector.
	//
	// It does not disturb the `base#digits` literal, which every shell with
	// arithmetic has: that `#` follows digits and is read by the number, and
	// this one stands where an operand belongs.
	//
	// Getting it backwards is the hazard the operator is worth recording for:
	// `$((#a))` looks like a length and is not one. On `a=(1 2)` it is 49,
	// the code of the `1`, where the count is `$(( $#a ))`.
	ArithCharacterCode bool

	// ArithFunctionCall enables `name(args)` inside an arithmetic expression:
	// a call to a *math function*, which is a name the shell has been told
	// arithmetic may call. See [ArithCall].
	//
	// Two things register one and the grammar cannot tell them apart, which is
	// why this says "a name" rather than "a shell function": `functions -M`
	// names a shell function to run, and a module may bring a table of them
	// implemented in the interpreter itself. The measurement below is of the
	// first, because it is the one a script writes.
	//
	// **Two** shells in the panel have it, and this comment said one until
	// 2026-09-12. Measured 2026-09-08 on zsh 5.9.2 and on zsh 5.9, where
	// `g(){ REPLY=$(($1+100)); }; functions -M mf 1 1 g; echo $(( mf(5) ))`
	// prints 105. Re-measured 2026-09-12 on ksh93u+ 2012-08-01, `env -i` with
	// a scratch HOME, and that shell has the construct too — with a library
	// of them built in and nothing to register:
	//
	//	echo $(( sqrt(4) ))        2
	//	echo $(( pow(2,10) ))      1024
	//	echo $(( fmod(7,3) ))      1
	//	echo $(( int(3.7) ))       3
	//	echo $(( nosuchmf(1) ))    nosuchmf(1) : unknown function
	//	echo $(( atan(1,2) ))       atan(1,2) : function has wrong number of
	//	                           arguments
	//
	// So it has the grammar *and* both sentences, and the flag is on for it
	// since 2026-09-13, when the built-in table arrived beside it — see
	// dialect/ksh/mathfunc.go, which is that shell's sixty-one. Turning the
	// grammar on without the table would have been worse than the syntax
	// error it replaced, because a name that shell *knows* would have come
	// back `unknown function`; the two go together and landed together.
	// What the flag is **not** is a fact about that shell's grammar, and it
	// was read as one: #2420 grouped five corpus rows as a wording difference
	// over a syntax error when they are a shell that parsed a call we did
	// not.
	//
	// bash 5.3, bash 3.2, bash as `sh`, dash and ash have neither the
	// registration nor a table, and read `mf(5)` as a name followed by a
	// leftover `(`. So those columns record the grammar's absence rather than
	// a different meaning for the same text, which is what makes this
	// additive.
	//
	// The `(` has to touch the name. A space between them is not a call in
	// the shell that has one either — `$(( mf ( 5 ) ))` is
	// `operator expected` there — so turning this on does not change what
	// `$(( a (b) ))` means anywhere.
	ArithFunctionCall bool

	// ArithOutputFormat enables the bracketed output-format specifier inside
	// an arithmetic expression: `$(( [#16] 255 ))` is `16#FF` and
	// `$(( [##16] 255 ))` is `FF`. It says the base the *result is written
	// in*, and a second `#` drops the `base#` in front of the digits.
	//
	// The rest of the specifier is measured rather than guessed: an `_`
	// after the base groups the digits — `[#16_4] 1048575` is `16#F_FFFF`,
	// and a bare `_` groups decimal in threes — and `_0` turns grouping off.
	// What a base *means* is not here, because it decides nothing about the
	// tree: see the ArithOutput node, and interp's IntegerBaseDigits for the
	// alphabet and the range a base is checked against.
	//
	// It is **lexical rather than positional**, which is the part worth
	// recording because the obvious reading is wrong. The specifier is not a
	// prefix operator over the expression that follows it: it may stand
	// anywhere a token may, including after a value — `$(( 2[#8] ))` is
	// `8#2` — and it takes effect even where the expression it stands in is
	// never evaluated: `$(( 0 ? [#16] 1 : 2 ))` is `16#2`. Several may
	// appear, and the *textually last* one decides, which is what
	// `$(( [#16] 255 + [#8] 1 ))` being `8#400` says. So it is read while
	// skipping blanks and lifted to the top of the tree, rather than being a
	// node where it was written.
	//
	// One shell in the panel. Measured 2026-09-12 on zsh 5.9.2, against
	// bash 5.3.15, bash 3.2.57, bash as `sh`, ksh93u+ and dash — every one of
	// which reads the `[` as an operand it cannot have and says so. An
	// operator five shells refuse and one has is the additive kind of split,
	// so it is a flag here rather than a value on the semantics vector.
	//
	// It does not disturb a subscript, and could not: a subscript's bracket
	// touches the name in front of it and is read by the name, where this one
	// stands where a token begins. `a[#8]` is the element of `a` under the
	// subscript `#8`, measured in the same shell.
	ArithOutputFormat bool

	// DoubleBracket enables `[[ ... ]]`.
	//
	// Consumed by the *parser*, not the lexer, and the reason is worth
	// stating because the opposite is the obvious guess. Inside `[[ ]]` the
	// `<` and `>` are comparisons rather than redirections, which sounds like
	// a lexer mode — but `[[` is only special in command position (`echo [[ a
	// ]]` prints `[[ a ]]`), and the lexer does not know where commands
	// begin. Lexing `<` as an operator loses nothing: the parser knows it is
	// inside `[[ ]]` and reinterprets the token. A lexer mode keyed on seeing
	// the word `[[` would break `echo`.
	DoubleBracket bool

	// DoubleBracketIsACommand reads `[[` as the *name of a command* — `test`
	// with a closing word — rather than as the conditional's keyword. Meant
	// for a dialect with DoubleBracket off; with both on, the keyword wins
	// where a command begins and this still governs every other `[[` word.
	//
	// Measured 2026-09-16 on BusyBox v1.37.0 in the pinned alpine image, the
	// one shell in the panel that answers this way. `type '[['` is `[[ is a
	// shell builtin`; `v='[['; $v -n x ]]` runs it; its operands are split
	// and globbed (`s="two words"; [[ $s == "two words" ]]` is `words:
	// unknown operand` at 2); `[[ a < b ]]` opens `b` for reading; and `[[ (
	// -n x ) ]]` is a syntax error at the `(`, because every operator the
	// shell has is still an operator.
	//
	// All but two. After an unquoted `[[` word of a simple command, `&&` and
	// `||` are *words* until an unquoted `]]` word, so the builtin receives
	// them as its connectives: `[[ -n x && -z "" ]]` is 0. The rule is about
	// the word and not about command position — `echo [[ a && b ]] && echo
	// c` prints `[[ a && b ]]` and then `c` — and it is the parser's rather
	// than the lexer's, since `for w in [[ a && b ]]` is `unexpected "&&"`
	// there. A quoted `"[["`, `\[[` or `x[[` opens nothing, `]]x` closes
	// nothing, and `;`, `|`, `&`, a newline and the redirections keep their
	// meaning inside: `[[ -n x &&` at the end of a line is `missing ]]`.
	DoubleBracketIsACommand bool

	// ProcessSubstitution is `<(cmd)` and `>(cmd)`: a command run with one end
	// of a pipe, expanding to a path the other end can be opened by.
	//
	// It is the sharpest evidence that a dialect is a runtime switch rather
	// than a build-time identity, and the reason is measured: bash 3.2 has it
	// as `bash` and loses it as `sh`, from the same binary — see
	// docs/spec/shell-matrix.md.
	ProcessSubstitution bool

	// ProcessSubstitutionToFile is `=(cmd)`: the same construct with a
	// regular file where the other two spellings have a pipe.
	//
	// Additive and to one dialect. Measured 2026-09-11: `echo =(echo hi)`
	// writes a path in zsh 5.9.2 and the other five columns refuse the `(`,
	// so there is nothing for a semantics axis to switch between — the
	// construct is either in the grammar or it is not.
	//
	// **Position is the grammar, and it is narrow.** The `=` has to begin a
	// word: `echo =(echo hi)x` appends the `x` to the path, and `echo
	// x=(echo hi)`, `echo a=b=(echo hi)`, `echo \=(echo hi)` and `echo
	// =(echo hi)=(echo hi)` are all `missing end of string` there. Quoting
	// takes it away like any other operator — `"=(echo hi)"` is its own ten
	// characters — and the one position that is not the front of a word is
	// the front of an assignment's *value*: `a==(echo hi)` assigns the path,
	// and so do `a[1]==(…)`, `a+==(…)` and `typeset a==(…)`, while the same
	// word written as an argument is the refusal above. An array literal's
	// element needs no rule of its own, `a=(=(echo hi))` being the front of
	// a word again.
	//
	// The flag is the lexer's: whether `=(` opens a substitution decides
	// where the word ends, and that is settled before any parser sees a
	// token. See Lexer.startsProcSubstFile, and
	// docs/spec/grammar/substitutions.md for the measurements.
	ProcessSubstitutionToFile bool

	// ReadFileSubstitution makes `$(<file)` the file's contents.
	//
	// A command substitution whose whole body is one input redirection and
	// nothing else — no command word, no assignment, no second redirection —
	// is a *special form* rather than a program: the file is opened and its
	// bytes become the substitution's result, with no command run and no
	// process started. `` `<file` `` is the same form in the older spelling,
	// and an explicit `0` before the operator is still it.
	//
	// It is additive because the shell without it reads the same text as an
	// ordinary command: a redirection with no command name opens the file,
	// runs nothing, and writes nothing, so the substitution is empty.
	// Measured 2026-09-10 with `printf x > f` and `printf "[%s]" "$(<f)"`:
	// `[x]` in zsh 5.9.2, bash 5.3, bash 3.2, bash as `sh`, bash `--posix`
	// and ksh93, and `[]` in dash — the sole holdout, which is what puts the
	// form in [Core].
	//
	// It is not the shell's null-command hook, and that was the fork worth
	// settling before any of this was written. zsh runs a bare redirection
	// with no command through `READNULLCMD`, so `<f` at a prompt pages the
	// file — but the special form is measurably not that route: with
	// `READNULLCMD` pointing at a function that prints `CHANGED`, `<f` prints
	// `CHANGED` and `$(<f)` still prints the file, while `$(:; <f)` and
	// `$(<f; :)` — the same redirection with company, so no longer the whole
	// body — print `CHANGED` again. A hook the form does not consult is a
	// different mechanism, and implementing this as "a null command copies
	// its input" would have made `<f` print the file in bash and ksh93, where
	// measurably it prints nothing.
	//
	// The flag is read by the interpreter rather than the parser. The body of
	// a command substitution is kept as source and parsed when it is
	// expanded, so the tree is the same either way and only the meaning
	// differs — which is why the question is asked where the substitution
	// runs.
	ReadFileSubstitution bool

	// ProcessSubstitutionInParamOperand says a `${…}` operand may carry one.
	//
	// Separate from ProcessSubstitution because the panel separates them:
	// measured, `${u:-<(:)}` is a path in bash and the five characters
	// `<(:)` in ksh93, zsh and dash — and ksh93 and zsh both have process
	// substitution everywhere else. So whether `<(` opens one is a question
	// about *where the word stands*, not only about the dialect, and every
	// operand of an expansion is on the same side of it: the pattern of
	// `${v#…}`, the replacement of `${v/…/…}` and the word of `${v:-…}` all
	// answer alike.
	//
	// It matters most where the operand is a pattern, because there the
	// wrong answer is silent. The text is pattern text in the shells that
	// say no, so `${v#<(x)}` is a `<` and — where the grammar has bare
	// groups — the group `(x)`, which matches `<x`; a reading that took the
	// substitution's *inner* text instead matched a bare `x`, which no shell
	// in the panel does, and answered with the subject quietly trimmed.
	ProcessSubstitutionInParamOperand bool

	// ProcessSubstitutionOnlyWhereACommandTakesAWord refuses `<(cmd)` and
	// `>(cmd)` **while reading** anywhere but the two places a command takes
	// a word: an argument of a simple command, and the target of a file
	// redirection. The refusal names the opener — `` `<(' unexpected `` — and
	// abandons the input, the way any other token in the wrong place does.
	//
	// ksh93 alone, of the three panel members that have the construct.
	// Measured 2026-09-13, `env -i PATH=/usr/bin:/bin` with a scratch HOME,
	// ksh93u+ 2012-08-01 over `-c`, stdin on the null device:
	//
	//	[[ x == <(:) ]]                   `<(' unexpected, status 3
	//	[[ -f <(:) ]]                     the same
	//	[[ <(:) ]]                        the same
	//	case <(:) in *) :;; esac          the same
	//	case x in <(:)) :;; esac          the same
	//	for i in <(:); do :; done         the same
	//	select i in <(:); do break; done  the same
	//	a=( <(:) )                        the same
	//	cat <<< <(:)                      the same
	//	[[ x == >(:) ]]                   `>(' unexpected, status 3
	//
	//	echo <(:)                         /dev/fd/3
	//	cat <(:)                          runs
	//	: <(:)                            runs
	//	set -- <(:)                       /dev/fd/3
	//	cat < <(:)                        runs — a redirection target
	//	for i in a; do echo <(:); done    /dev/fd/3 — the body is commands
	//
	// **The refusal is at the parse and not at the run**, which the last row
	// of the first block is not enough to show. `false && [[ x == <(:) ]];
	// echo reached` prints nothing and exits 3: the condition is in a branch
	// never taken, so a shell refusing it at the run would have printed
	// `reached`.
	//
	// **This is why the flag is not named for the condition.** #930 was filed
	// from the `[[ ]]` row alone, and a flag spelled "a condition's operand
	// may carry one" would have made this shell accept the other eight —
	// encoding a rule ksh93 does not have. The measured rule is about where a
	// *word* stands, and the condition is one of five positions that are not
	// it.
	//
	// It is a grammar flag rather than the
	// interp.Semantics.ProcessSubstitutionInCondition axis because an axis
	// cannot carry a moment. That axis says whether the substitution may
	// stand as a condition's operand and is still the right question for zsh,
	// which reads the word and then refuses it at the run, in a sentence of
	// its own and at status 2. Both shells say no; only this one says it
	// before anything has run.
	//
	// Two boundaries the measurement draws that this flag deliberately does
	// not reach, each recorded so that a later reading does not mistake them
	// for oversights:
	//
	//   - At the *start* of a command ksh93 never lexes the opener at all,
	//     `<` being a redirection operator there: `if <(:); then :; fi`,
	//     `echo | <(:)` and `! <(:)` are `` `)' unexpected ``, naming the
	//     paren rather than the pair. That is the lexer's rule about `<` and
	//     not the grammar's about `<(`, and this shell reads the opener there
	//     as a word.
	//   - The opener also *ends the word before it* in ksh93 — `echo a<(:)b`
	//     writes three fields, `a`, the path and `b` — where this shell keeps
	//     one. A separate fact about word boundaries, measured here so it is
	//     not rediscovered as this rule.
	ProcessSubstitutionOnlyWhereACommandTakesAWord bool

	// FdVariableRedirections is `{name}>file` and its family: the shell
	// picks the descriptor and the variable receives its number. Consumed
	// by the lexer, because the adjacency to the operator is the whole
	// grammar — `{fd}>f` names a descriptor and `{fd} >f` is a word.
	//
	// Three of the four have it; to dash the braces are part of an
	// ordinary word.
	FdVariableRedirections bool

	// FdVariablePositional lets the name inside those braces begin with a
	// digit, which is how a *positional parameter* names a descriptor:
	// `exec {1}>&-` closes the one whose number is in `$1`.
	//
	// zsh alone. Measured 2026-09-10: `f(){ exec {1}</dev/null; }` opens the
	// descriptor and puts its number in `$1` there, where bash 5.3 answers
	// `exec: {1}: not found` — it read the braces as a word — and ksh93
	// answers `1: invalid variable name`. So two of the three shells with
	// the parent feature refuse it, which is the same shape
	// FdVariableSubscript has and the reason this is a flag of its own
	// rather than a consequence of having the parent.
	//
	// A digit-leading name must be *all* digits, and that rule is the
	// runtime's rather than the lexer's: measured, `{1a}` and `{1_}` are
	// taken as the token and then refused with `not an identifier: 1a`,
	// where `{01}` and `{99}` work. So the lexer admits the run and
	// whoever resolves the name decides, which is also what keeps the two
	// halves from disagreeing about what a name is.
	//
	// It is needed by a real prompt theme: a scheduler handed a descriptor
	// as its first argument closes it exactly this way.
	FdVariablePositional bool

	// MultiDigitFdNumber lets a redirection's descriptor number have more
	// than one digit: `exec 10>file` opens the file on ten.
	//
	// bash and BusyBox ash read it that way. To dash, ksh93 and zsh the
	// digits are an ordinary word and the operator is a redirection of its
	// own, so `exec 10>f` is `exec 10 >f` and reports that no command called
	// `10` was found — the same answer for every width from two digits up.
	// Those three still hold descriptors above nine perfectly well; they
	// simply have no way to *write* one, and `exec {v}>f` is what puts them
	// there — the shell picks 10 or 11 and the variable says which.
	//
	// The ash column was measured late and reads the other way from what
	// this comment used to claim: 2026-09-16, BusyBox v1.37.0, `exec
	// 10>f10; echo hi >&10` leaves three bytes in `f10`, while a bare `10`
	// is still a command that is not found. That dialect sets the flag
	// since #3238.
	//
	// It is the lexer's because the token is: digits immediately before `<`
	// or `>` are either a number or the tail of a word, and nothing later can
	// tell them apart. This is the case AmpersandRedirect's comment warns
	// about, in a milder form — where the flag is off, `echo x 10>f` is not
	// an error but a different command, printing `x 10` into the file rather
	// than `x` to the terminal.
	//
	// POSIX is the reason it is a flag rather than a fact: the standard's
	// IO_NUMBER is one *or more* digits, so a shell taking `10>f` conforms
	// and so does the majority that does not. Where the panel and the
	// standard disagree the panel decides the core, which leaves this to the
	// one shell that wants it.
	MultiDigitFdNumber bool

	// FdVariableSubscript lets the name inside those braces carry a
	// subscript: `exec {a[1]}>&-` closes the descriptor that element holds,
	// which is how a coprocess's feed is closed by the array the shell put
	// its near ends in.
	//
	// A flag of its own rather than a consequence of having both features,
	// because the shell that has both still refuses this one. Measured with
	// a descriptor parked on 3 and the element holding 3: bash 5.3 and ksh93
	// close it, and zsh reads `{a[1]}` as an ordinary word — as a *pattern*,
	// in fact, so it reports no matches, and with globbing turned off it
	// reports a command not found. Either way it never reached a redirection,
	// which is what makes this the construct's absence rather than a glob
	// getting in first. bash 3.2 and dash have no `{name}` token at all.
	//
	// So two of the three shells that have the parent feature have this one,
	// and the core is what all three agree on — which leaves it to the two
	// that answer yes.
	//
	// The subscript is taken as written and never expanded, because the
	// token is one literal: `{a[i]}` and `{a[i+1]}` are read where `{a[$i]}`
	// stays a word. bash takes that last spelling and ksh93 refuses it in
	// the arithmetic, so no answer here is everyone's.
	FdVariableSubscript bool

	// CloseQuotesAtEOF is the set of routes on which an unterminated `'`,
	// `"` or backquote ends at the end of input as if the closing mark were
	// there, instead of the parse being refused: `echo "abc` prints abc.
	// `$(` and `${` are not quotes and still refuse on every route.
	//
	// A set rather than a boolean because the one shell that answers yes
	// does not answer it for the whole shell. Measured 2026-09-07, ksh93u+
	// 2012-08-01, the same five lines by every route, `echo one` first so
	// that what ran before the quote is visible:
	//
	//	route                     ksh93                 other five
	//	-c string                 runs, status 0        refuse
	//	eval string               runs, status 0        refuse
	//	script file operand       `'' unmatched`, 3     refuse
	//	`.` on a file             `'' unmatched`, 3     refuse
	//	standard input, or a pipe `'' unmatched`, 3     refuse
	//
	// So a boolean gets one of ksh93's routes right and four wrong, and the
	// four it gets wrong are the ones a script arrives by. A truncated file
	// then ran under our ksh and was refused by the real one, silently and
	// with status 0, and everything after the opening quote was discarded
	// without a word (#1424).
	//
	// It is not the trailing newline telling the routes apart, which is the
	// reading a `-c` string invites: measured the same day, `-c` closes the
	// quote whether or not the string ends in one, and a file refuses
	// whether or not it does. The route is the whole of it.
	//
	// A quote left open at a *prompt* is neither: every shell in the panel,
	// this one included, asks for more input with PS2 rather than closing
	// or refusing. The lexer marks the input incomplete before it asks this
	// at all, which is what leaves that answer to the front end.
	CloseQuotesAtEOF ProgramRoutes

	// BackslashAtEndOfInput is what an unquoted backslash the input ends
	// immediately after becomes. See [EndOfInputBackslash], where the rows
	// are.
	//
	// It is a word rule rather than a route rule, which is the difference
	// from [Dialect.CloseQuotesAtEOF] just above: a script file with no
	// trailing newline and a `-c` string answer identically in every column,
	// so nothing here asks how the program arrived. It is also not a
	// *refusal* — nothing in the panel refuses, and this shell did, which
	// ended the script at the line and discarded everything after it
	// (#2680). The older substitution is where that was found: its body is
	// unescaped and re-lexed, so `` `echo \\` `` hands the inner parse the
	// text `echo \`, and the refusal came back out as the whole line's.
	BackslashAtEndOfInput EndOfInputBackslash

	// ProgramRoute is which of those ways the program *now being parsed*
	// arrived, for the rules above that ask.
	//
	// Not a grammar rule and the only field here that is not: a dialect is
	// a shell's language and this is one program's provenance. It lives on
	// the dialect because the lexer is where the question is answered and
	// the dialect is the only thing the lexer is handed. The front end sets
	// it — see [Dialect.On] — and the zero value is no route at all, so a
	// parse that never said gets the strict answer.
	ProgramRoute ProgramRoutes

	// Comments is what a `#` where a word could begin means in the program
	// *now being parsed*. The zero value is [CommentsSkipped], the rule
	// every shell reads a script by.
	//
	// The second field here that is not a grammar rule, and it is here for
	// the reason ProgramRoute is: the lexer answers the question and the
	// dialect is the only thing the lexer is handed. Where ProgramRoute is
	// one program's provenance, this is one program's *reader* — the front
	// end setting it says that the text about to be parsed was typed at a
	// prompt by a shell whose option for that is off.
	//
	// No preset writes it, and it would be wrong for one to: the shell this
	// exists for reads a `#` in a script exactly as the others do, and
	// differs only on the line a person typed. Measured on zsh 5.9.2,
	// 2026-09-12, `-f -i` on a pipe with a scratch HOME, beside the same
	// shell's script routes:
	//
	//	route                          `echo a #b`
	//	typed at the prompt            a #b
	//	`-c`, with `-i` or without     a
	//	`eval` typed at the prompt     a
	//	`.` on a file, at the prompt   a
	//	standard input, not `-i`       a
	//
	// So it is neither the route nor the shell's interactivity on its own:
	// zsh reading the same descriptor answers both ways depending on which,
	// and a `-c` string is a comment in a shell that is interactive. What
	// decides it is the front end, which is where both facts meet, and
	// which is why this is set per parse rather than chosen by a preset.
	// See repl.Shell.CommentsNeedTheOption.
	//
	// [CommentsKept] is not a mode a program can be parsed under — a
	// comment arrives as a word and the grammar has nowhere to put it — so
	// a front end has the other two. See [ShellWords], which is where the
	// third belongs.
	Comments CommentMode

	// UnmatchedBlamesTheOutermost names the *enclosing* construct when the
	// input runs out inside nested ones, where the default names the
	// innermost.
	//
	// Measured 2026-09-07, `-n` over a script file, with constructs nested
	// both ways round so that neither reading can pass for the other:
	//
	//	echo $( echo "hi          `"` in bash, dash and ksh93
	//	echo "${x:-"$( echo hi    `$(` in the same three
	//	echo $(( 1 + `echo 2      the backquote in the same three
	//	echo "$( echo hi          `$(` in the same three
	//
	// Three of the four name the innermost in every arrangement. zsh names
	// the outermost in all of them — `echo "$( echo hi` is `unmatched "`
	// there, about a quote the script did write, where bash is looking for a
	// `)`.
	//
	// It is answered by the *order* the reports arrive in rather than by
	// anything looking around: the scanners recurse, so the innermost to run
	// out reports first and the enclosing ones follow it outwards. Keeping
	// the first report is the default; this makes each replace the last.
	UnmatchedBlamesTheOutermost bool

	// Coproc is `coproc command`: the command runs in the background with a
	// pipe on each of its named streams and the shell keeps the near ends.
	// bash and zsh both have the word; ksh93 spells a coprocess `cmd |&`,
	// which is a different construct and is CoprocPipeOperator, and dash has
	// none.
	//
	// How the shell reaches the ends is not this flag — bash puts them in an
	// array and zsh speaks to them with `print -p` and `read -p` — because
	// that is what a *running* coprocess offers rather than what the parser
	// reads. It is interp.Semantics.CoprocEndsInAnArray.
	Coproc bool

	// CoprocPipeOperator is ksh93's `cmd |&`: the same coprocess Coproc
	// starts, spelled as an **operator that terminates a command** rather
	// than as a word in front of one. There is no `coproc` keyword in that
	// shell and no name to give — the two bytes are the whole of the syntax.
	//
	// It is not a second reading of PipeBothStreams, and the measurement
	// that separates them is what may *follow* the operator rather than what
	// the command prints. Measured 2026-09-07, ksh93u+ 2012-08-01, `-n` over
	// a script file under `env -i`:
	//
	//	probe                  bash 5.3   ksh93     zsh
	//	echo one | ; echo two  error `;`  error `;` accepts
	//	echo one |& ; echo two error `;`  accepts   accepts
	//	echo one & ; echo two  error `;`  accepts   accepts
	//
	// A `|` needs a command after it and ksh93's `|&` does not, exactly as a
	// bare `&` does not. zsh accepts all three because it is lenient about
	// `;` after any control operator, so the ksh column alone decides it.
	// ksh93 also takes `echo one | & echo two` with a blank between, where
	// the other five refuse — another sign the two bytes are not that token.
	//
	// It terminates the whole **and-or**, the way `&` does rather than the
	// way a pipe binds: `echo A && cat |&` puts `echo A`'s output into the
	// coprocess pipe, which a later `read -p` answers with `A`. Measured the
	// same day, and the same for `echo A | cat |&`.
	//
	// Where a dialect had both this and PipeBothStreams the pipe reading
	// would win, because parsePipeline asks first. No preset does: the two
	// are the two readings of one spelling and a dialect has one of them.
	CoprocPipeOperator bool

	// CoprocName lets a *name* stand between `coproc` and a compound
	// command, and it is bash's alone: `coproc MY { cat; }` puts the near
	// ends in MY there, and is a parse error in zsh, whose coprocess has no
	// name to give. Additive over Coproc, and asked only where that is set.
	//
	// The name is a name only before a *compound* command in the shell that
	// has it: before a simple one the first word is the command, so
	// `coproc MY cat` runs `MY cat` in both shells, and both leave whatever
	// the reader asks for afterwards unset.
	CoprocName bool
}

// On returns this dialect set to parse a program that arrived by route.
//
// A copy, so that the shell's own dialect keeps saying what the *language*
// is and each parse says how its text got here. One expression at the call
// site is the whole point: the route has to be attached at every place a
// program is read, and a step that is easy to leave out is one that gets
// left out.
func (d Dialect) On(route ProgramRoutes) Dialect {
	d.ProgramRoute = route
	return d
}

// Core is the common denominator of real shells: what dash, bash, ksh93 and
// zsh agree on, minus dash, whose absence is the decision recorded in
// docs/spec/core.md.
//
// CaseContinue is off because it is bash-only. Everything else here is
// something every non-dash shell in the reference panel accepts.
func Core() Dialect {
	return Dialect{
		AppendAssign:      true,
		CStyleFor:         true,
		Select:            true,
		ForBraceBody:      true,
		AmpersandRedirect: true,
		CaseFallthrough:   true,
		DollarSingleQuote: true,
		Herestring:        true,
		ArithCommand:      true,
		DoubleBracket:     true,
		FunctionKeyword:   true,
		// Every shell in the panel but dash times a pipeline with it. The
		// `-p` flag is not here: zsh reads `-p` as a word of the pipeline,
		// so the flag is bash's and ksh93's to add.
		TimeKeyword: true,
		// Every shell in the panel but dash has it, which is what puts it in
		// the core — and docs/spec/core.md has named it as core since before
		// there was code to refuse it.
		ProcessSubstitution: true,
		// The same head count once more: bash, bash 3.2, bash-as-sh, ksh93
		// and zsh all read `$((echo hi) )` as a command substitution holding
		// a subshell, where dash and BusyBox ash refuse it for a missing
		// `))`. Measured 2026-09-12.
		ArithSubstFallsBackToCommandSubst: true,
		// dash is the only shell in the panel that leaves `$(<f)` empty,
		// and it leaves it empty by not having the form rather than by
		// meaning something else by it — the same head count that put
		// process substitution here.
		ReadFileSubstitution: true,
		// The same head count: bash, ksh93 and zsh all pick a descriptor
		// for `exec {fd}>f` and set the variable; dash reads a word.
		FdVariableRedirections: true,
		ArrayLiteral:           true,
		// The utilities that take an array assignment as an operand. The
		// same four the interpreter treats as declarations for the same
		// reason: every shell in the panel that has them takes the form.
		// `declare` is bash's and zsh's to add, and ksh93 takes `local`
		// away, because the rule follows the name into the shell that has
		// it.
		DeclarationUtilities: map[string]bool{
			"export": true, "readonly": true, "local": true, "typeset": true,
		},
		ArraySubscript: true,
		// Three of the four count an alias body's newlines as input lines,
		// and so do all three of the dialects that expand aliases at all.
		// Unreachable until a dialect says it expands them.
		AliasBodyCountsLines:    true,
		FunctionNamePunctuation: true,
		ParamSubstitution:       true,
		ParamSubstring:          true,

		ArithIncDec:       true,
		ArithComma:        true,
		ArithExponent:     true,
		ArithExplicitBase: true,
	}
}

// POSIX is the specification's shell language and nothing else. It is
// deliberately narrower than any shell anyone actually runs, which makes it
// the right setting for a portability check and the wrong one for a runtime.
func POSIX() Dialect { return Dialect{} }

// ArithPrecedencePolicy is the order the binary arithmetic operators bind in.
//
// Two orders, and the second is not a quirk to be worked around: one shell
// documents both of them and ships an option — `c_precedences` — that picks
// between them, which is as explicit as a disagreement gets.
//
// The two differ in where the shifts and the bitwise operators sit, and in
// nothing else. Everything from `&&` down and everything from `*` up is the
// same ladder either way.
type ArithPrecedencePolicy int

const (
	// ArithPrecedenceAsInC is ISO C's order, which POSIX defers to and which
	// five of the six panel columns use. Tightest first, after the unary
	// operators:
	//
	//	**                     exponentiation
	//	* / %                  multiplication
	//	+ -                    addition
	//	<< >>                  shifts
	//	< > <= >=              comparison
	//	== !=                  equality
	//	&                      bitwise and
	//	^                      bitwise xor
	//	|                      bitwise or
	//	&&                     logical and
	//	||                     logical or
	ArithPrecedenceAsInC ArithPrecedencePolicy = iota

	// ArithPrecedenceShiftsAndBitwiseBindTighter is the other order, which
	// one shell calls its native mode. The shifts move to the *tightest*
	// binary level and the three bitwise operators move above `**`:
	//
	//	<< >>                  shifts
	//	&                      bitwise and
	//	^                      bitwise xor
	//	|                      bitwise or
	//	**                     exponentiation
	//	* / %                  multiplication
	//	+ -                    addition
	//	< > <= >=              comparison
	//	== !=                  equality
	//	&&                     logical and
	//	||                     logical or
	//
	// Note what that does to `**`, which is the part a reading of the four
	// rows in #2883 would miss: exponentiation is *looser* than the bitwise
	// operators here, so `2 ** 1 | 3` is `2 ** (1 | 3)` and answers 8 where
	// C's order answers 3. The relative order of the three bitwise
	// operators, and of everything from `*` down, is unchanged.
	//
	// The associativity is not part of this. `**` is right-associative and
	// everything else left-associative under both orders, measured: `2 ** 3
	// ** 2` is 512 in every column that has the operator.
	ArithPrecedenceShiftsAndBitwiseBindTighter
)

func (p ArithPrecedencePolicy) String() string {
	if p == ArithPrecedenceShiftsAndBitwiseBindTighter {
		return "shifts and bitwise operators bind tighter"
	}
	return "as in C"
}

// ArithDoubleQuotePolicy is what a `"` inside an arithmetic expression is.
//
// Three readings rather than two, and the third is measured rather than
// invented: the four shells that read *through* a double quote do not agree
// about a quote standing in the middle of a token. `$(( 1"0" ))` is 10 in
// bash 5.3 — the quotes are gone before anything reads the text, so the two
// digits are one number — and an arithmetic syntax error in ksh93u+ and zsh
// 5.9.2, where the quote ends the number and leaves a second operand behind.
// Measured 2026-09-10 (#1223).
type ArithDoubleQuotePolicy int

const (
	// ArithDoubleQuoteRefused is no part of any token: the quote is where a
	// value should be, and the expression is refused for wanting an operand.
	// bash 3.2.57 and dash, and the core, which refuses what the panel
	// disagrees about.
	ArithDoubleQuoteRefused ArithDoubleQuotePolicy = iota
	// ArithDoubleQuoteSkipped passes over the byte wherever a token may
	// begin, so `"1" + 1` is 2 and `"n" + 1` reads the parameter n — but a
	// quote inside a token still ends it, and `1"0"` is two operands running
	// together. ksh93 and zsh.
	ArithDoubleQuoteSkipped
	// ArithDoubleQuoteRemoved takes the byte out of the text before the
	// expression is read, so `1"0"` is the number 10 and `n"a"me` is the
	// parameter `name`. bash 5.3, and the same build invoked as `sh`.
	//
	// It is also what that shell quotes back when the expression fails —
	// `$(( "1" "2" ))` is reported against `1 2` there — which falls out of
	// removing the bytes rather than having to be arranged.
	ArithDoubleQuoteRemoved
)

func (p ArithDoubleQuotePolicy) String() string {
	switch p {
	case ArithDoubleQuoteSkipped:
		return "skipped"
	case ArithDoubleQuoteRemoved:
		return "removed"
	}
	return "refused"
}

// SubscriptQuoting is the set of quoting constructs that hold a `]` back from
// ending a `${name[ … ]}` subscript.
//
// A set rather than an ordered policy, because the panel does not nest: bash
// takes the backslash and both quotes and leaves `$'…'` out, zsh takes the
// backslash alone, and ksh93 takes all four. See
// Dialect.SubscriptQuoteProtectsTheClosingBracket for the measurement.
type SubscriptQuoting uint8

const (
	// SubscriptBackslashQuotes reads `\]` as the character and not as the end
	// of the subscript. bash 5.3, ksh93 and zsh.
	SubscriptBackslashQuotes SubscriptQuoting = 1 << iota
	// SubscriptSingleQuotes reads `'…'` as a quoted run. bash 5.3 and ksh93;
	// not zsh, where `${a['0]'+1]}` stops at the quoted bracket and reports a
	// math error against `'0`.
	SubscriptSingleQuotes
	// SubscriptDoubleQuotes reads `"…"` as a quoted run, with a backslash
	// inside it quoting the next byte. bash 5.3 and ksh93.
	SubscriptDoubleQuotes
	// SubscriptDollarSingleQuotes reads `$'…'` as a quoted run. ksh93 alone —
	// bash 5.3 stops at the bracket inside it, which is the one cell where
	// bash and ksh93 part.
	SubscriptDollarSingleQuotes
)

// Has says q holds the given construct.
func (q SubscriptQuoting) Has(c SubscriptQuoting) bool { return q&c != 0 }

func (q SubscriptQuoting) String() string {
	if q == 0 {
		return "nothing"
	}
	out := ""
	for _, c := range []struct {
		bit  SubscriptQuoting
		name string
	}{
		{SubscriptBackslashQuotes, `\`},
		{SubscriptSingleQuotes, `'`},
		{SubscriptDoubleQuotes, `"`},
		{SubscriptDollarSingleQuotes, `$'`},
	} {
		if !q.Has(c.bit) {
			continue
		}
		if out != "" {
			out += " "
		}
		out += c.name
	}
	return out
}

// BraceQuotePolicy is what a single quote written inside a double-quoted
// `${ … }` does to the scan for the closing brace.
//
// Three readings rather than two, because the panel splits three ways and the
// line runs through the *operand* rather than through the shell. See
// Dialect.QuoteProtectsTheClosingBrace for the measurement.
type BraceQuotePolicy int

const (
	// BraceQuoteProtectsAPatternOnly reads the quote as quoting in a `#`,
	// `##`, `%`, `%%` or `/` operand and as an ordinary character in the
	// word operands, which is where a `}` it stands in front of closes the
	// expansion. Five of the seven columns, and the core: a pattern's quotes
	// are its own, and a word operand's belong to the double quote around
	// the whole expansion, where a single quote quotes nothing.
	BraceQuoteProtectsAPatternOnly BraceQuotePolicy = iota
	// BraceQuoteProtectsNothing reads it as an ordinary character in every
	// operand, so the expansion always ends at the first `}`. zsh 5.9.2.
	BraceQuoteProtectsNothing
	// BraceQuoteProtectsEveryOperand reads it as quoting wherever it is
	// written, so `"${v-'}'}"` runs to the second brace. bash 5.3.15 and
	// 3.2.57 — and not the same build invoked as `sh`, which answers with
	// the other five.
	BraceQuoteProtectsEveryOperand
)

func (p BraceQuotePolicy) String() string {
	switch p {
	case BraceQuoteProtectsNothing:
		return "nothing"
	case BraceQuoteProtectsEveryOperand:
		return "every operand"
	}
	return "a pattern only"
}

// BraceQuotePosixMove is where POSIX mode puts [Dialect.QuoteProtectsTheClosingBrace].
//
// Four values rather than three, because the first of them is *unmoved* and
// has to be the zero one: [BraceQuotePolicy]'s zero is a real reading, and
// every dialect invoked as `sh` enters POSIX mode, so a plain second
// BraceQuotePolicy field would hand the standard's answer to a shell that
// never took a position on the question. See
// [Dialect.QuoteProtectsTheClosingBraceInPosixMode] for the panel.
type BraceQuotePosixMove uint8

const (
	// BraceQuoteUnmovedInPosixMode leaves the dialect's own reading in place.
	// zsh, whose `posixbuiltins` and whose `sh` name both leave the axis
	// alone, and the three shells with no POSIX mode to move it with.
	BraceQuoteUnmovedInPosixMode BraceQuotePosixMove = iota
	// BraceQuoteMovesToAPatternOnly is bash: the mode takes the word
	// operand's quote away and leaves a pattern operand's where it was.
	BraceQuoteMovesToAPatternOnly
	// BraceQuoteMovesToNothing and BraceQuoteMovesToEveryOperand are the
	// other two destinations. Nothing in the panel takes either, and they are
	// declared because the axis they move is a three-valued one: a move that
	// could only ever name one of the three readings would be recording the
	// shell that happens to have it rather than the question.
	BraceQuoteMovesToNothing
	BraceQuoteMovesToEveryOperand
)

// Policy is the reading this move names, or base where it names none.
func (m BraceQuotePosixMove) Policy(base BraceQuotePolicy) BraceQuotePolicy {
	switch m {
	case BraceQuoteMovesToAPatternOnly:
		return BraceQuoteProtectsAPatternOnly
	case BraceQuoteMovesToNothing:
		return BraceQuoteProtectsNothing
	case BraceQuoteMovesToEveryOperand:
		return BraceQuoteProtectsEveryOperand
	}
	return base
}

func (m BraceQuotePosixMove) String() string {
	if m == BraceQuoteUnmovedInPosixMode {
		return "unmoved"
	}
	return "moves to " + m.Policy(BraceQuoteProtectsAPatternOnly).String()
}

// TopLevelAlternation is how a dialect reads a `|` standing outside every
// group and bracket — see [Dialect.PatternTopLevelAlternation].
//
// Two shells have the reading and they do not have the same one, which is why
// this is not a bool. Measured 2026-09-13 on ksh93u+ 2012-08-01 and zsh 5.9.2,
// `env -i PATH=/usr/bin:/bin` over a script file, with `v=abc` and `L='a|ab'`:
//
//	probe                        ksh93        zsh
//	${v#a|ab}   written bar      bc           abc
//	${v#$L}     value bar        bc           abc  (bc under ${~L})
//	case via a value             matches      does not
//	$P with P='a|b', files a b   one field    two fields under globsubst
//	[[ ab == $L ]]               no           no
//
// Two rows carry the argument. The **fourth** is what keeps the readings
// apart in the same direction they differ everywhere else, and it is measured
// against a control: in that same run ksh93 expands `a*` and `a?` out of a
// value to two fields each, so it is the *bar* that does not reach pathname
// expansion there and not the value failing to be a pattern. And `w='a|b';
// ${w#a|b}` answers `|b` in ksh93 where zsh answers empty — the value stops
// matching its own text, which is what says the bar is syntax there rather
// than one more character.
type TopLevelAlternation uint8

const (
	// NoTopLevelAlternation reads the bar as an ordinary character. The
	// zero value, and what four of the panel's seven columns do.
	NoTopLevelAlternation TopLevelAlternation = iota

	// TopLevelAlternationFromAValue is zsh's: the bar is an alternation
	// only where a **value** supplied it, so a written one is an ordinary
	// character and the provenance has to be arranged — see
	// interp's markWrittenBars (#2168). It reaches pathname expansion,
	// where a value under `globsubst` becomes a pattern in its own right.
	TopLevelAlternationFromAValue

	// TopLevelAlternationWhereverWritten is ksh93's, and is wider in one
	// direction and narrower in another. Wider: provenance is not asked, so
	// a bar the script wrote is an alternation exactly as one out of a value
	// is. Narrower: it does **not** reach pathname expansion, and it does
	// not reach `[[ ]]`.
	//
	// So neither reading contains the other, which is the shape this whole
	// vector exists for — turning the old bool on for ksh would have given
	// it zsh's provenance rule, which is the wrong one (#2528).
	TopLevelAlternationWhereverWritten
)

// ReadsATopLevelBar reports whether a bar outside every group is an
// alternation, in the contexts that ask about pattern text rather than about
// file names. condition says the pattern stands inside `[[ ]]`.
//
// The two readings part company there, and **the simpler rule is wrong**:
// both shells answer `n` to `L='a|ab'; [[ ab == $L ]]`, which reads as "no
// dialect reads a bar in a condition" — but turning it off for both takes
// zsh's `setopt globsubst; [[ ab == $L ]]` from matching to not, because
// there the value has become a pattern in its own right and the bar inside it
// is live. ksh93 has no such option and answers `n` either way. Measured
// 2026-09-13; the globsubst row is what distinguishes the two rules, and a
// table without it would have justified the wrong one.
func (t TopLevelAlternation) ReadsATopLevelBar(condition bool) bool {
	if condition {
		return t == TopLevelAlternationFromAValue
	}
	return t != NoTopLevelAlternation
}

// ReachesPathnameExpansion reports whether a top-level bar makes a field a
// pattern against the filesystem.
//
// Only zsh's reading does. Stated as a method rather than derived at the call
// site because the two facts are independent — a dialect could have ksh's
// provenance rule and zsh's reach — and because the one caller is a hundred
// lines from the flag.
func (t TopLevelAlternation) ReachesPathnameExpansion() bool {
	return t == TopLevelAlternationFromAValue
}

// ReadsAWrittenBar reports whether the bar is an alternation however it
// arrived, which is the question markWrittenBars asks: where it is false and
// the reading is on, a written bar has to be escaped so only a value's
// survives.
func (t TopLevelAlternation) ReadsAWrittenBar() bool {
	return t == TopLevelAlternationWhereverWritten
}

// EmptyBodyBlame is which token a refusal names when a terminator stands
// where a body or a condition must have something in it. See
// [Dialect.EmptyBodyBlame] for the measured table.
type EmptyBodyBlame uint8

const (
	// BlameTheTerminatorItself names the terminator where it stands, which
	// is what dash and bash do and what the substrate answers with nothing
	// said.
	BlameTheTerminatorItself EmptyBodyBlame = iota
	// BlameTheTokenAfterIt names whatever stands next, one token on, with no
	// separators stepped over. ksh93 for `&`: `if & then` is `then`, `if & ;
	// then` is `;` and `{ & }` is `}` — the token after it whatever kind it
	// is.
	//
	// A newline is the exception and it is an acceptance rather than a
	// wording: `if &` with the `then` on the next line *runs* there, so
	// there is no refusal to name. This names the terminator, which is what
	// this shell already wrote.
	BlameTheTokenAfterIt
	// BlameTheKeywordAfterIt names a reserved word, and how far it looks for
	// one depends on which list the terminator stood in. zsh for `|`, `|&`,
	// `&&` and `||`, measured 2026-09-14:
	//
	//	if | :; then :; fi        `then`   a condition, read to its keyword
	//	if | :; :; then :; fi     `then`   however much stands in between
	//	if | { :; }; then :; fi   `then`   a `}` on the way is not it
	//	if | ; then :; fi         `then`
	//	if | :; fi                `fi`     the construct's own closer
	//	while | :; done           `done`
	//	until | :; do :; done     `do`
	//	if :; then | fi           `fi`     a body, and the keyword is next
	//	for i in 1; do | :; done  `|`      a body, and a command is next
	//	{ | }                     `|`      the closing brace is not named
	//	( | )                     `|`
	//	case x in x) | ;; esac    `|`
	//
	// So a **condition** is named at the reserved word that ends its header,
	// read forward to; a **body** is named at the reserved word standing
	// next, and at the terminator otherwise. The closing brace is the one
	// reserved word never named — measured rather than assumed, and what
	// parts this from naming any stop word.
	//
	// One measured row this does not reach: `if | || :; then :; fi` is `||`
	// there, which is neither a reserved word nor the terminator. It is the
	// shell's own recovery showing through, it is what this shell already
	// wrote, and a rule built to catch it would have to name an operator in
	// a position where `if | ; then :; fi` names the keyword.
	BlameTheKeywordAfterIt
)

// Reserves reports whether name is part of *this* dialect's grammar rather
// than a name a command could have.
//
// It exists because the question is asked from outside the parser. A shell's
// `command -v` and `type` answer "reserved word" for a name the grammar
// claims, and an interpreter holding a written-out list of them answers for a
// grammar that is not the one it is running: ours told a dash script that
// `[[`, `]]`, `select` and `function` were runnable words, and answered
// `time` with the keyword where dash has none and a script reaching for
// `command -v time` wants the path of `/usr/bin/time` (#2918). The list is
// here, beside the flags that decide it, so there is one of it.
//
// Measured 2026-09-15 with `command -v` and with `type`, on bash 5.3.15,
// zsh 5.9.2, ksh93 93u+, dash 0.5.12 and BusyBox ash: every shell claims
// exactly the constructs it has, and a word it does not have falls through to
// the ordinary search — which is what makes `command -v time` a path in dash
// and a keyword everywhere else.
//
// Two rows of that measurement are **not** the grammar, and are deliberately
// not here: `]]` is named by bash alone among the four shells that have the
// construct, and `in` is named by everyone but zsh. Both shells *have* what
// the word is part of and decline to report it, so the answer is a fact about
// the report rather than about what parses. See #2981.
//
// Two of its members are spelled as operators rather than words and so are
// not in [reservedWords] at all. `[[` and `]]` are the conditional command's
// two ends, and `coproc` opens a construct a dialect either has or does not;
// each is claimed by the same flag the grammar reads.
func (d Dialect) Reserves(name string) bool {
	switch name {
	case "[[", "]]":
		return d.DoubleBracket
	case "coproc":
		return d.Coproc
	}
	return d.reservesWord(name)
}

// reservesWord is [Dialect.Reserves] over the words the *lexer* classes as
// reserved, which is the narrower question an alias expansion asks.
//
// The two part company on the three spellings above: they are constructs a
// dialect has, so a report about what is runnable must name them, and they
// are not words the lexer reserves, so the rule about which words an alias
// may shadow has never been about them. See [Parser.reservedInDialect].
// RedirectionBeforeACompoundPolicy says whether a redirection may stand in
// **front** of a compound command, and before which of them.
//
// POSIX puts a compound command's redirections after it and every shell takes
// them there; two of the panel take them in front as well, and they take them
// before different things. Measured 2026-09-18, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`:
//
//	written                                zsh 5.9.2  ksh93u+  bash, dash, ash
//	>/dev/null ( echo hi )                 runs       runs     refuses
//	>/dev/null (( 1 ))                     runs       runs     refuses
//	>/dev/null { echo hi; }                runs       refuses  refuses
//	>/dev/null while false; do :; done     runs       refuses  refuses
//	>/dev/null if true; then echo hi; fi   runs       refuses  refuses
//	>/dev/null for i in a; do echo hi; done  runs     refuses  refuses
//	>/dev/null case a in a) echo hi;; esac   runs     refuses  refuses
//	>/dev/null until true; do :; done      runs       refuses  refuses
//	>/dev/null select i in a; do break; done runs     refuses  refuses
//	>/dev/null repeat 2 do echo hi; done   runs       —        —
//	>/dev/null foreach i (a); echo hi; end runs       —        —
//	2>/dev/null [[ -n a ]]                 runs       refuses  refuses
//	>/dev/null ! false                     refuses    —        —
//	>/dev/null coproc cat                  refuses    —        —
//
// So one column takes it before a **parenthesized** command alone and the
// other before every compound it has. The two rows at the bottom are that
// column's own boundary: `!` and `coproc` are not compound commands there and
// the redirection does not reach them.
//
// The **assignment prefix** takes it away in both: `v=x >/dev/null ( echo hi
// )` and `>/dev/null v=x { echo hi; }` are refused in zsh and in ksh93 alike,
// which is why this is asked only where nothing but redirections has been
// read (#3560).
type RedirectionBeforeACompoundPolicy uint8

const (
	// RedirectionBeforeACompoundIsRefused is POSIX's own shape, and the
	// answer bash 5.3, bash 3.2, dash and BusyBox 1.37.0 ash give: a
	// compound command's redirections follow it.
	RedirectionBeforeACompoundIsRefused RedirectionBeforeACompoundPolicy = iota
	// RedirectionMayPrecedeAParenthesizedCommand takes one in front of `(
	// … )` and `(( … ))` and nowhere else: ksh93.
	RedirectionMayPrecedeAParenthesizedCommand
	// RedirectionMayPrecedeAnyCompoundCommand takes one in front of every
	// compound command the dialect has: zsh.
	RedirectionMayPrecedeAnyCompoundCommand
)

func (p RedirectionBeforeACompoundPolicy) String() string {
	switch p {
	case RedirectionMayPrecedeAParenthesizedCommand:
		return "before a parenthesized command"
	case RedirectionMayPrecedeAnyCompoundCommand:
		return "before any compound command"
	}
	return "refused"
}

// reservedAtACommandStart reports whether name is a word this dialect reads as
// reserved where a **command** begins.
//
// reservesWord with two differences, and both are measured rather than
// reasoned. The constructs a preset adds bring words with them — `[[`,
// `repeat`, `foreach` and its `end`, `coproc` — and none of those is in the
// union reservesWord reads. And `in` is **not** one: it is special inside
// `for` and `case` and an ordinary command name anywhere else, so `v=x in` is
// `command not found: in` in the column that refuses every other reserved
// word behind a prefix. See
// [Dialect.ReservedWordStandsBehindAnAssignmentPrefix] for the rows.
func (d Dialect) reservedAtACommandStart(name string) bool {
	switch name {
	case "in":
		return false
	case "[[":
		return d.DoubleBracket
	case "repeat":
		return d.Repeat
	case "foreach", "end":
		return d.Foreach
	case "coproc":
		return d.Coproc
	}
	return d.reservesWord(name)
}

func (d Dialect) reservesWord(name string) bool {
	switch name {
	case "select":
		return d.Select
	case "function":
		return d.FunctionKeyword
	case "time":
		return d.TimeKeyword
	}
	return reservedWords[name]
}
