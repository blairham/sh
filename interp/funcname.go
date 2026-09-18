// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// FuncNameRunForm is what a `function` definition does when it is reached and
// the word standing where its name belongs is not a name.
//
// A form rather than a flag, and for the reason ForNameRunForm is one: there
// are four answers among the three shells that get this far, the third differs
// from the second in its status alone, and the fourth says nothing at all.
// Only the dialects with
// [syntax.Dialect.FunctionNameCheckedWhenTheDefinitionRuns] ever ask — zsh
// reads such a name as a word and defines what it comes to, and dash has no
// keyword to reach the question with.
//
// Measured 2026-09-10 through `-c`, over `w=foo; function _p_${w} { echo HI;
// }; echo st=$?; echo after`, and ash on 2026-09-13 from a script file in the
// digest-pinned alpine image:
//
//	shell        what a run prints                       script status
//	bash 5.3.15  the complaint, then st=1 and after      0
//	bash-as-sh   the complaint, and stops                2
//	bash 3.2.57  the complaint, then st=1 and after      0
//	ksh93u+      the complaint, and stops                1
//	BusyBox ash  nothing, then st=0 and after            0
//
// The ash row reaches this by a different route and it matters to what the
// axis means: the other four are asked because the word's **text** is not a
// name, and ash is asked because of how the word was **written** — see
// [syntax.Dialect.FunctionNameIsAnyBareWord], where `'f'` is refused and a
// bare `a*b` is not.
//
// **The bash-as-`sh` row is POSIX mode and not the build**: `set -o posix`
// in bash 5.3 stops at 2 on the same line, exactly as the same binary invoked
// as `sh` does, and bash 3.2 and 5.3 agree under their own names. So it is a
// mode the shell can enter and leave, which is why it is a value of this axis
// and is swapped by [Runner.SetPosixMode] rather than being a second preset.
// The same grid ForNameRunForm records, asked of a different construct.
type FuncNameRunForm int

const (
	// FuncNameRunUnspecified is no answer, and is refused like any other. A
	// core run cannot get here at all without the grammar flag, so reaching
	// it means a dialect turned the flag on and left this unanswered.
	FuncNameRunUnspecified FuncNameRunForm = iota

	// FuncNameFailsTheDefinition reports the complaint, gives the definition
	// status 1, and lets the script carry on. bash under its own name, 3.2
	// and 5.3 alike.
	FuncNameFailsTheDefinition

	// FuncNameEndsTheScript reports the complaint and stops, at 1. ksh93.
	//
	// No redirection exception, which is measured rather than assumed by
	// symmetry with the loop's: `function _p_${w} { :; } > mf; echo after`
	// ends the script at 1 there too, where the same shape on a `for`
	// clause carries on. So the two constructs part company on exactly that
	// row and this value is the narrower of the two.
	FuncNameEndsTheScript

	// FuncNameEndsTheScriptAsASyntaxError reports the same complaint and
	// stops at the dialect's *syntax-error* status rather than at 1 — 2 for
	// bash, which is what POSIX mode answers. The wording does not change
	// with it, which is measured: bash invoked as `sh` writes ``line 1:
	// `_p_${w}': not a valid identifier`` word for word as bash does.
	FuncNameEndsTheScriptAsASyntaxError

	// FuncNameDefinesNothing says nothing, binds nothing, and gives the
	// definition status 0. BusyBox ash, which reaches the question by a
	// different route from the three above: there the word's *text* is not a
	// name, and here it is the **spelling** —
	// [syntax.Dialect.FunctionNameIsAnyBareWord] — so a name as ordinary as
	// `f` arrives here for having been written `'f'`.
	//
	// The fourth value rather than a silent variant of the first, because
	// the status differs too. Measured 2026-09-13 in the digest-pinned
	// alpine image, BusyBox v1.37.0: `'f'() { echo body; }` is status 0 with
	// nothing on stderr, and the call after it is `f: not found` at 127.
	//
	// **Nothing is bound, rather than something bound elsewhere.** `command
	// -v` and `type` answer 127 for both the word's text and its source
	// text, and `g() { echo old; }; 'g'() { echo new; }; g` prints `old`, so
	// a definition already standing under that name is not replaced either.
	FuncNameDefinesNothing
)

func (f FuncNameRunForm) String() string {
	switch f {
	case FuncNameFailsTheDefinition:
		return "FuncNameFailsTheDefinition"
	case FuncNameEndsTheScript:
		return "FuncNameEndsTheScript"
	case FuncNameEndsTheScriptAsASyntaxError:
		return "FuncNameEndsTheScriptAsASyntaxError"
	case FuncNameDefinesNothing:
		return "FuncNameDefinesNothing"
	}
	return "FuncNameRunUnspecified"
}

// refuseFuncName settles a definition whose name the dialect will not bind,
// where the dialect settles it when the definition is reached.
//
// Four of the five answers raise a complaint and one — ash's — is silence, so
// this is "what happens" rather than "what is said"; the name is kept in the
// signature because the four that speak all quote it.
//
// The wording is [Diagnostics.FunctionNameInvalid], which is the sentence the
// *expanded*-name refusal already uses — ksh93 says `%s: invalid function
// name` for `f-g()` and for this, word for word, so one field rather than two
// that could drift. What differs between the two routes is which text the
// sentence names: there, the name the definition would have bound; here, the
// word as it was written, because that is what the shells quote.
//
// The status both shells that report it as a failure give is 1, and it is
// written here rather than taken from a Diagnostics field because neither of
// them answers anything else — the third value carries the dialect's
// syntax-error status instead, which is the one number that does vary.
func (r *Runner) refuseFuncName(word string) {
	form := r.sem().FunctionNameWhenTheDefinitionRuns
	if form == FuncNameDefinesNothing {
		// The one answer with no complaint in it: the definition is over,
		// the table is untouched, and the script carries on at 0.
		r.status = 0
		return
	}
	if form == FuncNameRunUnspecified {
		r.errf("%s\n", r.diag().Report(r.name(), r.line,
			r.unanswered("a function name that is not a name, reached at run time")))
		r.status = 2
		r.unspecified = true
		return
	}
	r.diagf("%s\n", Wording(r.diag().FunctionNameInvalid, "%[1]s: invalid function name", word))
	switch form {
	case FuncNameEndsTheScriptAsASyntaxError:
		r.endTheScriptAt(r.diag().SyntaxStatus())
	case FuncNameEndsTheScript:
		r.endTheScriptAt(1)
	default:
		r.status = 1
	}
}

// AnsiCValue is what a `$'…'` word stands for, for a caller that has to write
// the *value* where the source wrote the escapes.
//
// Exported for exactly one caller and named for the thing rather than for it:
// a function listing in one dialect writes the characters back, which is
// syntax.Layout.AnsiCQuotedWordIsItsValue, and the decoder cannot live in the
// printer because three of the escapes are semantics axes and nothing under
// `syntax` may hold one. The argument is the text between the quotes, as the
// lexer kept it.
func (r *Runner) AnsiCValue(text string) string {
	return r.expandDollarSingle(text)
}
