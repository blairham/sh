// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// FuncNameRunForm is what a `function` definition does when it is reached and
// the word standing where its name belongs is not a name.
//
// A form rather than a flag, and for the reason ForNameRunForm is one: there
// are three answers among the two shells that get this far, and the third
// differs from the second in its status alone. Only the dialects with
// [syntax.Dialect.FunctionNameCheckedWhenTheDefinitionRuns] ever ask — zsh
// reads such a name as a word and defines what it comes to, and dash has no
// keyword to reach the question with.
//
// Measured 2026-09-10 through `-c`, over `w=foo; function _p_${w} { echo HI;
// }; echo st=$?; echo after`:
//
//	shell        what a run prints                       script status
//	bash 5.3.15  the complaint, then st=1 and after      0
//	bash-as-sh   the complaint, and stops                2
//	bash 3.2.57  the complaint, then st=1 and after      0
//	ksh93u+      the complaint, and stops                1
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
)

func (f FuncNameRunForm) String() string {
	switch f {
	case FuncNameFailsTheDefinition:
		return "FuncNameFailsTheDefinition"
	case FuncNameEndsTheScript:
		return "FuncNameEndsTheScript"
	case FuncNameEndsTheScriptAsASyntaxError:
		return "FuncNameEndsTheScriptAsASyntaxError"
	}
	return "FuncNameRunUnspecified"
}

// refuseFuncName raises the complaint a definition's unusable name earns when
// the definition is reached.
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
