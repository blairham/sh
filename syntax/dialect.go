// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

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

	// ExpandAliases expands an alias in a *non-interactive* shell. True in
	// dash and ksh93, which expand by every route; bash needs
	// `shopt -s expand_aliases` and expands by none without it. All four
	// expand interactively, which is the front end's to know rather than
	// this — it is what decides there is a person at the keyboard.
	//
	// zsh does not fit the boolean, measured 2026-09-05: it declines under
	// `-c` and expands from a script file and from standard input. This flag
	// is false for zsh, which is right for `-c` and wrong for the other two
	// routes; the answer wants to be route-aware the way "is this
	// interactive" already is, and that is not built. Issue #583.
	//
	// Whether a word *is* expanded, and into what, is not a dialect question:
	// every shell that expands agrees on the whole algorithm, so that is the
	// core's behavior and lives in alias.go.
	ExpandAliases bool

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

	// ArithLeadingZeroIsOctal decides whether `0100` is sixty-four or one
	// hundred. It is true everywhere but zsh, and it is the quietest
	// divergence measured: nothing warns, both answers are plausible
	// numbers, and file modes are written with leading zeros.
	//
	// Nothing in the parser reads this — a literal is kept as written, so
	// the tree does not bake in an answer — but the field belongs with the
	// others, and evaluation needs it.
	ArithLeadingZeroIsOctal bool

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

	// RegexTakesAlternation makes a bare `|` part of a `=~` operand rather
	// than the end of the word. bash and ksh93 say yes, so `[[ ab =~ a|b ]]`
	// matches there; zsh says no and reports a parse error. Parentheses are
	// taken by all three and so need no flag — a dialect without `[[ ]]`
	// never reaches the question.
	RegexTakesAlternation bool

	// ArraySubscript enables `${a[i]}`, `${a[@]}` and `${a[*]}`, and `a[i]`
	// inside an arithmetic expression. Absent from dash, which has no arrays
	// at all and calls the subscript a bad substitution rather than reading
	// it — a separate flag from ArrayLiteral because the two halves are
	// separately reachable: a subscript can be written for a variable that
	// was never an array.
	ArraySubscript bool

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
		ArraySubscript:          true,
		FunctionNamePunctuation: true,
		ParamSubstitution:       true,
		ParamSubstring:          true,

		ArithIncDec:             true,
		ArithComma:              true,
		ArithExponent:           true,
		ArithExplicitBase:       true,
		ArithLeadingZeroIsOctal: true,
	}
}

// POSIX is the specification's shell language and nothing else. It is
// deliberately narrower than any shell anyone actually runs, which makes it
// the right setting for a portability check and the wrong one for a runtime.
func POSIX() Dialect { return Dialect{} }
