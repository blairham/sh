// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// AliasRoutes is a set of the ways a non-interactive program reaches a shell,
// for the one grammar question whose answer depends on which of them it was.
//
// The routes are the front end's — a command string, a file, standard input —
// and this package names them anyway, because [Dialect.ExpandAliases] is the
// question and a question has to be askable where it is answered. The parser
// never reads the set; whoever knows how the program arrived does, and passes
// the table of aliases in or leaves it nil.
type AliasRoutes uint8

const (
	// AliasFromCommandString is a program given as an argument: `-c`.
	AliasFromCommandString AliasRoutes = 1 << iota
	// AliasFromScriptFile is a program read from a path named as an operand.
	AliasFromScriptFile
	// AliasFromStandardInput is a program read from the descriptor, whether
	// by `-s` or by there being no operand and no terminal.
	AliasOnStandardInput
	// AliasOnEveryRoute is what a shell that does not distinguish them
	// answers, which is two of the four.
	AliasOnEveryRoute = AliasFromCommandString | AliasFromScriptFile | AliasOnStandardInput
	// AliasOnNoRoute is the empty set, spelled so a dialect can say it
	// deliberately rather than by leaving a field out.
	AliasOnNoRoute AliasRoutes = 0
)

// Has reports whether route is in the set. A caller asks with exactly one
// route, which is what it knows.
func (a AliasRoutes) Has(route AliasRoutes) bool { return a&route != 0 }

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

	// AnonymousFunction is `() { … }` and `function { … }`: a function with
	// no name, defined and run where it stands, with the words after it as
	// its positional parameters. One shell in the panel has it; in the other
	// four a `(` where a command begins opens a subshell and `()` is a
	// syntax error.
	AnonymousFunction bool

	// AppendAssign enables `name+=value`, which appends rather than
	// replacing. Absent from dash, where `x+=b` is a command called `x+=b`.
	AppendAssign bool

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
	// name carry `-` and `.` — `f-g()` and `a.b()` — which bash, ksh93 and
	// zsh all parse. dash refuses the name outright (`Bad function name`),
	// and what a shell that parsed one *does* with it is the interpreter's
	// question: ksh93 refuses at definition time. bash's commit-at-paren
	// reading accepts still more (`f+x()`, `@weird()`), which that flag
	// already covers without this one.
	FunctionNamePunctuation bool

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
	ExpandAliases AliasRoutes

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

	// PatternAlternation enables a bare `(a|b)` inside a pattern word, which
	// zsh has and the others do not: `a(b|c)` matches `ab` there. It is why
	// `@(abc|xyz)` is a literal `@` followed by a group in zsh rather than an
	// extended pattern — the same text, read by a different rule.
	PatternAlternation bool

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

	// FdVariableRedirections is `{name}>file` and its family: the shell
	// picks the descriptor and the variable receives its number. Consumed
	// by the lexer, because the adjacency to the operator is the whole
	// grammar — `{fd}>f` names a descriptor and `{fd} >f` is a word.
	//
	// Three of the four have it; to dash the braces are part of an
	// ordinary word.
	FdVariableRedirections bool

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

	// CloseQuotesAtEOF ends an unterminated `'`, `"` or backquote at the
	// end of input as if the closing mark were there, instead of refusing
	// to parse: `echo "abc` prints abc in the one shell that answers this
	// way. `$(` and `${` are not quotes and still refuse.
	CloseQuotesAtEOF bool

	// Coproc is bash's `coproc [NAME] command`: the command runs in the
	// background with a pipe on each of its named streams, and the shell
	// keeps the near ends in an array. zsh spells a coprocess the same
	// way and plumbs it differently — through `print -p` and `read -p`
	// rather than an array — and that model is not this flag.
	Coproc bool
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
