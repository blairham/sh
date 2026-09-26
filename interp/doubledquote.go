// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Whether a doubled quote inside a single-quoted string is one literal quote
// is a question about how a *word is read*, so it lives on syntax.Dialect and
// not on Semantics — and one dialect spells it as an option a running script
// switches, which is why a runner has to be able to move it.
//
// That combination is what makes the *moment* the interesting part. The
// answer that decides is the one in force when the text is **lexed**, never
// when the word is expanded, and the two are far apart: a function body is
// read once at its definition and expanded at every call. Measured on zsh
// 5.9.2 (`/opt/homebrew/bin/zsh`, `-f`) from a script file, 2026-09-26, over
// `f() { print -r -- 'a''b' }`:
//
//	defined with the option on, called with it off    a'b
//	defined with it off, called with it on            ab
//
// Each row holds the state at the *call* fixed at the other's value, so the
// call cannot be what decides. The same pair through `eval "print -r --
// 'a''b'"` answers to the state at the `eval`, and an alias body — stored as
// text and read at its use — answers to the state at the use, both of which
// are the same rule: the reading is what is asked.
//
// The signature of a parse-time answer, and the reason a probe written with
// `-c` reports no difference and is wrong: `setopt rcquotes; print -r --
// 'a''b'` on **one line** prints `ab`, because the whole line was read before
// the option moved.
//
// The state is carried as a replaced Dialect rather than as a bit beside it,
// which is the arrangement [Runner.SetMatchOption] uses for the option that
// moves syntax.Dialect.ExtendedPattern. Two things follow and both are
// wanted: a subshell is a cloned runner holding the same Dialect pointer, so
// copying rather than writing through it keeps a subshell's change inside the
// subshell; and the front end watches the pointer, so the *next line of the
// program* is read the new way. See driver's run loop.

// DoubledQuoteInSingleQuotes reports whether this runner reads a doubled
// quote inside a single-quoted string as one literal quote.
func (r *Runner) DoubledQuoteInSingleQuotes() bool {
	return r.lang().DoubledQuoteInSingleQuotesIsALiteralQuote
}

// SetDoubledQuoteInSingleQuotes moves it, for a dialect whose option
// namespace has a name for the reading.
//
// The dialect is copied and replaced rather than written through: the pointer
// is shared with every subshell cloned from this runner, and a script must
// not change the grammar of the shell that spawned it.
func (r *Runner) SetDoubledQuoteInSingleQuotes(on bool) {
	d := r.dialect()
	if d.DoubledQuoteInSingleQuotesIsALiteralQuote == on {
		return
	}
	d.DoubledQuoteInSingleQuotesIsALiteralQuote = on
	r.Dialect = &d
}
