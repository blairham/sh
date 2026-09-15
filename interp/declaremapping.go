// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// `typeset -M`, where the letter names a *character mapping* applied to
// everything the name is later given.
//
// Two shells spell the letter on a declaration builtin and neither means what
// the other does, so this is an axis rather than a letter one dialect has:
// zsh registers a shell function as a **math** function callable from
// arithmetic — interp/mathfunc.go, reached through `functions -M` — and
// ksh93 names a mapping. See DeclareMappingLetterPolicy below.
//
// Measured 2026-09-13 on ksh93u+ 2012-08-01, scrubbed environment, no startup
// files.
//
// **There are exactly two mappings and each is an attribute letter under
// another name.** `typeset -M tolower v` lists back as `typeset -l v` and
// `typeset -M toupper v` as `typeset -u v`, so the mapping is not a third
// thing to store: it is the spelling. Every other word tried — `totitle`,
// `ascii`, `utf8`, `identity` — is `typeset: <name>: unknown mapping name` at
// 1, which ends the script there because these are special builtins.
//
// **The name rides on the letter or is the first operand.** `-Mtolower v` and
// `-M tolower v` are one command. A `--` takes the operands out of the
// letter's reach, so `typeset -M -- mf 1 1 g` has no name at all and is
// `typeset: -M requires argument when operands are specified`.
//
// **A `-f` line refuses the letter, after the name has been judged.** The
// order is the whole of what the corpus rows turn on: `functions -M a 1 1 g`
// — which is `typeset -f -M a …` — is `a: unknown mapping name` at 1, while
// `functions -M tolower v` and `functions -M -- mf 1 1 g` and a bare
// `functions -M` are the builtin's usage block at 2. So a name that is not a
// mapping is blamed before the company the letter is keeping is.

// DeclareMappingLetterPolicy is what the `M` letter of a declaration builtin
// means to a dialect that spells it.
//
// The two readings share nothing: one takes a function name and two counts
// and registers an arithmetic callback, the other takes a mapping name and a
// list of parameters. There is no reading that is nearly right, so the
// unanswered value is refused by name.
type DeclareMappingLetterPolicy int

const (
	// DeclareMappingLetterUnspecified is no answer, which bash and dash hold
	// and never reach: neither spells the letter.
	DeclareMappingLetterUnspecified DeclareMappingLetterPolicy = iota
	// DeclareMappingLetterRegistersAMathFunction is zsh's `functions -M`:
	// see interp/mathfunc.go, which is where that reading lives.
	DeclareMappingLetterRegistersAMathFunction
	// DeclareMappingLetterNamesACharacterMapping is ksh93's: the letter
	// names a mapping applied to the parameters after it.
	DeclareMappingLetterNamesACharacterMapping
)

func (p DeclareMappingLetterPolicy) String() string {
	switch p {
	case DeclareMappingLetterRegistersAMathFunction:
		return "registers a math function"
	case DeclareMappingLetterNamesACharacterMapping:
		return "names a character mapping"
	}
	return "unspecified"
}

// characterMappings is the whole of what the letter accepts, and each entry
// is the attribute letter that already does the job. Measured rather than
// guessed at: the listing a mapped name gives back is the letter's, so there
// is nothing here the attribute does not already record.
var characterMappings = map[string]byte{"tolower": 'l', "toupper": 'u'}

// declareMapping is `typeset -M` under the character-mapping reading. It
// answers the whole line: either the mapping is applied to the operands or
// the line is refused, and there is no path through it that goes on to
// declare anything the ordinary way.
func (r *Runner) declareMapping(name string, operands []string, f declareFlags) int {
	mapping, named := f.mappingName, f.mappingNamed
	if !named && !f.endedOptions && len(operands) > 0 {
		mapping, named, operands = operands[0], true, operands[1:]
	}
	if !named {
		switch {
		case f.function || f.funcNames:
			// A `-f` line with no mapping to name. The usage block, because
			// there is no word to blame.
			return r.refuseWithUsage(name)
		case len(operands) > 0:
			// The operands are there and the letter cannot reach them, which
			// is what a `--` in front of them does.
			r.diagf("%s\n", Wording(r.diag().DeclareMappingNeedsAName,
				"%[1]s: -M requires argument when operands are specified",
				r.builtinComplaintName(name)))
			return r.endDeclarationAfterARefusal(1)
		default:
			// A bare `-M` is a listing of the mapped names, and this engine
			// records no mapping apart from the attribute it stands for, so
			// there is never one to write.
			return 0
		}
	}
	letter, known := characterMappings[mapping]
	if !known {
		r.diagf("%s\n", Wording(r.diag().DeclareUnknownMapping,
			"%[1]s: %[2]s: unknown mapping name",
			r.builtinComplaintName(name), mapping))
		return r.endDeclarationAfterARefusal(1)
	}
	if f.function || f.funcNames {
		// A mapping is a property of a parameter, and this line asked for
		// the function table. Below the name check because that is the order
		// measured, not because it is the tidier one.
		return r.refuseWithUsage(name)
	}
	// The mapping *is* the attribute letter, so the declaration goes the
	// ordinary way with that letter written into it rather than through a
	// store of its own — which is what keeps `typeset -M tolower v` and
	// `typeset -l v` from being two answers to one question.
	f.mapping, f.mappingName, f.mappingNamed = false, "", false
	switch letter {
	case 'l':
		f.lower = true
	case 'u':
		f.upper = true
	}
	f.letters += string(letter)
	f.letterSigns += "-"
	return r.declareNames(name, operands, f)
}

// endDeclarationAfterARefusal is the status and the give-up a refused
// declaration leaves behind, where the dialect counts the builtin among its
// special ones — the same rule a bad name gets, since these refusals are a
// bad operand under another word.
func (r *Runner) endDeclarationAfterARefusal(status int) int {
	if r.ask(r.sem().BadNameToDeclarationFatal, "a bad name to a special builtin ending the script") {
		r.status = status
		r.fatalUsageQuiet()
	}
	return status
}
