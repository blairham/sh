// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

// The letters a history-style modifier list is written with.
//
// Here rather than beside the code that *applies* them, because one shell
// writes a modifier list without braces and the letters then decide where the
// **word** ends: `$p:t` is one expansion and `$p:zz` is an expansion followed
// by three characters, and nothing downstream can tell the two apart once the
// spans are cut. See Dialect.BareParamModifiers.
//
// One table and not two. The letter set was the lexer's question and the
// evaluator's answer, and a second copy of it is how a letter comes to end a
// word in one place and mean nothing in the other — this repository's
// standing failure, where a new helper omits what the old one carries. So the
// evaluator reads these and the letters are declared once.

// ModifierArg is what a modifier letter may carry after it inside its own
// segment.
type ModifierArg int

const (
	// ModifierNothing: the letter is the whole segment, and anything after
	// it is the complaint that names nothing.
	ModifierNothing ModifierArg = iota
	// ModifierCount: an optional decimal count, which `h` and `t` alone
	// take. `${x:h2}` is the head twice over and `${x:h:2}` is a `2` that
	// names no modifier — the digit belongs to the letter or to nothing.
	//
	// The **braced** spelling alone. A bare `$x:h2` is the head once with a
	// literal `2` after it, measured; see Dialect.BareParamModifiers.
	ModifierCount
	// ModifierSubst: a delimited pattern and replacement, `s/l/r/`, whose
	// delimiter is whatever byte follows the letter.
	ModifierSubst
)

// ModifierLetters is what the shell that has them accepts, measured a letter
// at a time. Fourteen, and the fourteenth is not a letter: `:&` repeats the
// last substitution.
//
// The value is what may follow the letter inside the same segment, because
// that is the only thing about a segment this table cannot say twice. Most
// take nothing at all; `h` and `t` take a count; `s` takes a whole
// substitution and `&` takes the one before it.
var ModifierLetters = map[byte]ModifierArg{
	'h': ModifierCount,   // head — everything before the last slash
	't': ModifierCount,   // tail — everything after it
	'r': ModifierNothing, // root — the value with its suffix taken off
	'e': ModifierNothing, // extension — the suffix, without its dot
	'l': ModifierNothing, // lowercase
	'u': ModifierNothing, // uppercase
	'a': ModifierNothing, // an absolute path, against the working directory
	'A': ModifierNothing, // and the same with the links resolved
	'P': ModifierNothing, // the real path, which applies `..` after resolving
	'c': ModifierNothing, // a command's path, from the command search
	'q': ModifierNothing, // quoted, in this shell's own quoting
	'Q': ModifierNothing, // and unquoted, read back
	's': ModifierSubst,   // substitution, which takes a delimited pair
	'&': ModifierNothing, // the substitution before it, again
}
