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

	// ParamSubstring enables `${x:off:len}`. Absent from dash.
	ParamSubstring bool

	// ParamCaseChange enables `${x^^}` and `${x,,}`. **bash alone**: ksh93
	// reports a syntax error and zsh a bad substitution, so a construct one
	// panel shell supports is not a common denominator and this is off for
	// the core.
	ParamCaseChange bool

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

	// ArrayLiteral enables `a=(x y)`. Absent from dash, where the `(` is a
	// syntax error rather than a different construct — so unlike `&>`, this
	// one is safe to be wrong about loudly.
	ArrayLiteral bool

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
}

// Core is the common denominator of real shells: what dash, bash, ksh93 and
// zsh agree on, minus dash, whose absence is the decision recorded in
// docs/spec/core.md.
//
// CaseContinue is off because it is bash-only. Everything else here is
// something every non-dash shell in the reference panel accepts.
func Core() Dialect {
	return Dialect{
		AmpersandRedirect: true,
		CaseFallthrough:   true,
		DollarSingleQuote: true,
		Herestring:        true,
		ArithCommand:      true,
		DoubleBracket:     true,
		FunctionKeyword:   true,
		ArrayLiteral:      true,
		ParamSubstitution: true,
		ParamSubstring:    true,

		ArithIncDec:             true,
		ArithComma:              true,
		ArithExplicitBase:       true,
		ArithLeadingZeroIsOctal: true,
	}
}

// POSIX is the specification's shell language and nothing else. It is
// deliberately narrower than any shell anyone actually runs, which makes it
// the right setting for a portability check and the wrong one for a runtime.
func POSIX() Dialect { return Dialect{} }
