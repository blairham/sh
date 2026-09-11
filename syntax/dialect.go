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

	// OneSeparatorExceptAfterABar steps over a single `;`, and not at all
	// after a `|`. ksh93, and both halves are measured rather than assumed:
	// `a || ; ; b` is `` `;' unexpected `` there where `a || ; b` runs, and
	// `a | ; b` is refused where `a || ; b` and `a |& ; b` are taken — the
	// same asymmetry #1115 found for that shell's `|&`, and the probe that
	// says the bar is a separate question from the and-or.
	OneSeparatorExceptAfterABar

	// AnySeparatorWhereACommandBelongs steps over as many as are written,
	// anywhere, the bar included. zsh: `echo one | ; ; cat -n` numbers the
	// line, and so does `echo one | ; ⏎ ; cat -n`.
	AnySeparatorWhereACommandBelongs
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
	CaseFallthrough bool

	// CStyleFor enables `for ((init; cond; post))`. Absent from dash, where
	// the parenthesis after `for` is a syntax error.
	CStyleFor bool

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
	// `end` is the whole of what it adds: the list is the parenthesized one
	// ShortForm already reads, and `for name (a b); …; end` is refused —
	// measured — so the terminator belongs to the opening word rather than
	// to the list.
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

	// CurrentShellSubstitution reads `${ cmd;}` as a command substitution
	// that runs in the current shell. bash 5.3 and ksh93 have it; dash and
	// zsh call it a bad substitution.
	//
	// The space after the brace is load-bearing and is the whole of the
	// grammar: `${x}` is a parameter and `${ x}` is a command.
	CurrentShellSubstitution bool

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
	// The emptiness is read off the *separator* rather than off the
	// position, and every place a separator can put one is allowed:
	// `(|a|b)`, `(a||b)`, `(a|b|)`, `(|)` and `(||)` all parse there, with
	// or without the arm's optional open paren. `()` does not — the shell
	// that accepts every line above calls it a parse error — so this is not
	// "the list may be empty": with no separator there is nothing to read
	// the emptiness off.
	//
	// It is a grammar flag and not a matching rule. A group with an arm that
	// matches nothing already stands for nothing in every shell that has the
	// construct at all — `@(|a)b` matches `b` in bash and ksh93 alike — so
	// what divides the panel here is only whether the pattern *list* may
	// have such an alternative written into it.
	CasePatternMayBeEmpty bool

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
	// simple command as a one-command body and run it. The keyword form is
	// not this flag's question — its shapes differ per shell in ways the
	// POSIX form's do not.
	FuncBodyMustBeCompound bool

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
	// The third row is a definition there and is **not** read here: the
	// parentheses follow the redirection's target rather than a name, and the
	// redirection sits inside the header text a formatter copies from the
	// source, so the body would write it a second time. See #1838.
	FunctionMultipleNames bool

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
	// Measured alongside it, and **not** modeled: with a body that is not a
	// brace group, that shell reads the whole and-or list as the body —
	// `function a; echo X && echo Y` prints `X` then `Y` from a call, where
	// `function a { echo X; } && echo Y` prints `Y` then `X`. The body here
	// is one command either way; see #1832.
	FunctionKeywordBodyIsOptional bool

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

	// ArithCommand enables `(( expr ))` as a command. Consumed by the lexer,
	// which scans the expression as raw text: what is inside is an arithmetic
	// expression rather than a command list, so the token stream would lose
	// it. Where this is off, `(( 1+1 ))` is two nested subshells running
	// `1+1` as a command name, which is what dash does — not an error, a
	// different program.
	ArithCommand bool

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

	// ParamSubstitution enables `${x/pat/rep}` and its anchored forms.
	// Absent from dash.
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

	// ExpandAliases is the set of *non-interactive* routes this dialect
	// expands an alias on. dash and ksh93 answer every route; bash answers
	// none without `shopt -s expand_aliases`. All four expand interactively,
	// which is the front end's to know rather than this — it is what decides
	// there is a person at the keyboard.
	//
	// A set rather than a boolean because one answer is not a property of
	// the shell at all. Measured 2026-09-05, the same two lines by all three
	// routes:
	//
	//	shell   -c    script file   standard input
	//	bash    no    no            no
	//	dash    yes   yes           yes
	//	ksh93   yes   yes           yes
	//	zsh     no    yes           yes
	//
	// A boolean gets one of zsh's three right and the two it gets wrong are
	// the ones a real script uses. The same measurement found bash in POSIX
	// mode splitting the other way — `sh -c` expands and `sh script.sh` does
	// not — so the route is a dimension of the question rather than one
	// shell's quirk.
	//
	// Whether a word *is* expanded, and into what, is not a dialect question:
	// every shell that expands agrees on the whole algorithm, so that is the
	// core's behavior and lives in alias.go.
	ExpandAliases ProgramRoutes

	// AliasBodyCountsLines counts the newlines inside a substituted alias
	// body as lines of the input, so that every later line shifts by one per
	// newline and a command written on the body's second line is reported
	// there.
	//
	// It is the one place the two substitution models are visible from
	// outside. dash, ksh93 and zsh splice the body's *text*, so its newlines
	// are input lines; bash splices tokens and the whole body sits on the
	// line the alias word was written on. Measured 2026-09-05 with `$LINENO`
	// after a two-line body physically on line 5 — bash 5, the other three 6
	// — and again with a three-line body, which shifts by two; and with a
	// command that fails inside the body, reported on the body's own line by
	// the three and on the alias word's line by bash. The shift is per
	// *expansion*: using the alias twice shifts twice, and defining it and
	// never using it shifts nothing.
	//
	// True in the core, which is the majority of the panel and of the
	// dialects that expand at all. Unreachable where ExpandAliases is empty,
	// since nothing is ever spliced.
	AliasBodyCountsLines bool

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
	// reading takes text the other three read as belonging to the word, and
	// a core that swallowed it would be reading a word nobody wrote.
	//
	// Only unquoted. In double quotes all six stop at the first `}` and the
	// flag is not consulted — see the note in scanBraces, and #1586, which
	// settled that half. The `${ cmd;}` command form keeps its own rule for
	// a third reason again: its body is a program, so a `{ … }` block
	// written in one has to balance the way the program's braces do.
	BareBraceNestsInExpansion bool

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

	// ArithExplicitBase enables the `base#digits` form. Absent from dash.
	ArithExplicitBase bool

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
	// here: three of the four have no such sentence, and for them this is
	// empty and every failure keeps the wording it already had. The bytes are
	// measured, not derived — every other punctuation byte tried is either a
	// math token in that shell or can begin a value.
	//
	// It does not decide on its own. The same byte gets the operand sentence
	// where an operator has just been consumed and a value is wanted, so the
	// parser asks this only at the two positions where the expression could
	// legally have stopped: before anything has been read, and where an
	// operator belonged.
	ArithBytesRefusedOutright string

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

	// PatternAlternation enables a bare `(a|b)` inside a pattern word, which
	// zsh has and the others do not: `a(b|c)` matches `ab` there. It is why
	// `@(abc|xyz)` is a literal `@` followed by a group in zsh rather than an
	// extended pattern — the same text, read by a different rule.
	PatternAlternation bool

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

	// CloseBraceAlwaysReserved makes `}` a reserved word wherever a word may
	// stand, not only where a command may begin.
	//
	// It is what lets zsh write `{ echo hi }` with no terminator before the
	// brace: the `}` cannot be an argument, so it can only be closing the
	// group. The same rule is why `echo }` is a syntax error there and prints
	// a brace in the other three, which is the half that shows it is one rule
	// rather than a special case inside brace groups.
	CloseBraceAlwaysReserved bool

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
	// Two neighbors are deliberately not this flag. A `case` with no arms —
	// `case x in esac` — is a list of *arms* rather than a command list, and
	// the panel splits the other way there: dash, bash and zsh take it and
	// ksh93 alone refuses. A command substitution's body — `x=$( )` — is a
	// whole program rather than a compound command's body, and every shell
	// in the panel takes an empty one.
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
	// One shell in the panel has it. Measured 2026-09-08 on zsh 5.9.2 and on
	// zsh 5.9, where `g(){ REPLY=$(($1+100)); }; functions -M mf 1 1 g;
	// echo $(( mf(5) ))` prints 105 — against bash 5.3, bash 3.2, bash as
	// `sh`, ksh93 and dash, none of which has the registration and all of
	// which read `mf(5)` as a name followed by a leftover `(`. So the five
	// columns without it record the grammar's absence rather than a different
	// meaning for the same text, which is what makes this additive.
	//
	// The `(` has to touch the name. A space between them is not a call in
	// the shell that has one either — `$(( mf ( 5 ) ))` is
	// `operator expected` there — so turning this on does not change what
	// `$(( a (b) ))` means anywhere.
	ArithFunctionCall bool

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

	// ProcessSubstitution is `<(cmd)` and `>(cmd)`: a command run with one end
	// of a pipe, expanding to a path the other end can be opened by.
	//
	// It is the sharpest evidence that a dialect is a runtime switch rather
	// than a build-time identity, and the reason is measured: bash 3.2 has it
	// as `bash` and loses it as `sh`, from the same binary — see
	// docs/spec/shell-matrix.md.
	ProcessSubstitution bool

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
	// bash alone reads it that way. To dash, ksh93 and zsh the digits are an
	// ordinary word and the operator is a redirection of its own, so
	// `exec 10>f` is `exec 10 >f` and reports that no command called `10` was
	// found — measured on all five panel members, and the same answer for
	// every width from two digits up. Those three still hold descriptors
	// above nine perfectly well; they simply have no way to *write* one, and
	// `exec {v}>f` is what puts them there — the shell picks 10 or 11 and the
	// variable says which.
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
