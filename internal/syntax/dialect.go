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

	// FunctionKeyword enables `function name { ... }`. Absent from dash. The
	// hybrid `function name() { ... }` is accepted where this is on, and is
	// not core because ksh93 — where the keyword originated — rejects it.
	FunctionKeyword bool

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

	// ParamIndirection enables `${!x}`. bash alone means indirection by it.
	// ksh93 accepts it and means something else without erring, which is why
	// a dialect without this must *refuse* the construct rather than guess:
	// picking either meaning would be wrong for half the panel.
	ParamIndirection bool

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
		ParamSubstitution: true,
		ParamSubstring:    true,
	}
}

// POSIX is the specification's shell language and nothing else. It is
// deliberately narrower than any shell anyone actually runs, which makes it
// the right setting for a portability check and the wrong one for a runtime.
func POSIX() Dialect { return Dialect{} }

// Bash is Core plus what only bash has.
func Bash() Dialect {
	d := Core()
	d.CaseContinue = true
	d.ParamCaseChange = true
	d.ParamIndirection = true
	return d
}
