// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// What the `-p` letter on `export` and `readonly` does once the line carries
// operands — `export -p s=5`, `readonly -p t`.
//
// `typeset -p name` is a listing in every column of the panel, and the
// operand narrows it. These two words are *not* that, and the split is not
// the one interp/declareprintoperand.go records: that axis is what a listing
// does with an operand carrying a value, and it presupposes a listing. Here
// four columns have no listing at all.
//
// Measured 2026-09-20, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> z.sh` over
// a script file, standard input on the null device: dash 0.5.12 (`/bin/dash`),
// bash 5.3.20 and zsh 5.9.2 at `/opt/homebrew`, bash 3.2.57 (`/bin/bash`),
// AT&T ksh93u+ 2012-08-01 (`/bin/ksh`), BusyBox ash v1.37.0 in the pinned
// alpine image.
//
//	written                     dash             bash 5.3 / 3.2     ksh93u+            ash              zsh 5.9.2
//	export s=5; export -p s     every export     nothing            nothing            nothing          `export s=5`
//	export -p w=8               every export,    nothing, and `w`   nothing, and `w`   nothing, `w`=8   `no such variable: w`
//	                            `w` unset        is 8 and exported  is 8 and exported  and exported     at 1, `w` unset
//	readonly t=6; readonly -p t every readonly   nothing            nothing            nothing          `typeset -r t=6`
//	readonly -p u=9             every readonly,  nothing, `u` is    nothing, `u` is    nothing, `u`     `no such variable: u`
//	                            `u` unset        9 and frozen       9 and frozen       is 9, frozen     at 1, `u` unset
//
// So three readings, and every one of them is a complete answer:
//
//   - **the letter is inert** in bash (both builds), ksh93 and ash. Nothing
//     is listed and the operands are declared exactly as the same line
//     without the letter would declare them.
//   - **the listing narrows to the operands** in zsh, which is what every
//     column does for `typeset -p`, and nothing is declared.
//   - **the operands are dropped** in dash: the whole-table listing runs and
//     the names are not looked at, not listed and not declared.
//
// **The letter is read and then does the ordinary thing — it is not
// ignored.** Two controls, measured the same day and both needed, because
// "inert" and "not an option at all" predict the same bytes on the rows
// above:
//
//	export -q z=1     bash: `-q: invalid option` at 2, ksh93 and ash refuse it too
//	export -p nosuch  then `export -p`: `declare -x nosuch` in bash, `export nosuch` in ksh93 and ash
//	export k=1; export -pn k   silent 0, `k` is 1 and **no longer exported** in bash
//
// The first says option parsing happens, so `-p` is a letter this word knows
// rather than a word it would have tried to export. The second says the
// operand is *performed* rather than discarded — which is the row that parts
// bash and ksh93 from dash, whose `export -p nosuch` declares nothing. And
// the third says the line's other letters keep working while `-p` does not,
// so what is inert is that one letter and not the option word.
//
// **`readonly` was already right here and `export` was not**, which is what
// #3904 is: biReadonly took its listing branch only with no operands, and
// biExport took it whenever `-p` was written. Two builtins with one behavior
// in every column of the panel were written two ways, so the engine agreed
// with bash and ksh93 about `readonly -p t=6` and disagreed about `export -p
// w=8` on the same line of reasoning. One helper answers for both now.
type ExportPrintOperandPolicy uint8

const (
	// ExportPrintOperandUnspecified is no answer, and it is refused rather
	// than guessed at. The three readings differ over whether the name is
	// set at all once the line has run — inert stores it, the other two do
	// not — and a script reading it back afterwards cannot tell a shell that
	// declined to store from one that never had the name.
	ExportPrintOperandUnspecified ExportPrintOperandPolicy = iota

	// ExportPrintLetterIsInert lists nothing and declares the operands as
	// the same line without `-p` would declare them. bash 5.3.20, bash
	// 3.2.57, ksh93u+ and BusyBox ash.
	ExportPrintLetterIsInert

	// ExportPrintNarrowsToTheOperands runs the listing over the named
	// operands and declares nothing, which is what `typeset -p name` does
	// everywhere. zsh 5.9.2.
	ExportPrintNarrowsToTheOperands

	// ExportPrintDropsTheOperands runs the whole-table listing and does not
	// look at the operands at all — they are neither listed nor declared.
	// dash 0.5.12.
	ExportPrintDropsTheOperands
)

func (p ExportPrintOperandPolicy) String() string {
	switch p {
	case ExportPrintLetterIsInert:
		return "the print letter is inert once operands are written"
	case ExportPrintNarrowsToTheOperands:
		return "the print listing narrows to its operands"
	case ExportPrintDropsTheOperands:
		return "the print listing drops its operands"
	}
	return "unspecified"
}

// exportPrintWithOperands answers what a `-p` listing on `export` or
// `readonly` does with the operands beside it.
//
// The first result is the names the listing walks and the second is whether
// there is a listing at all: a false says the letter is inert and the
// caller's own declaration loop runs, which is the one branch where these two
// builtins declare under a `-p`.
//
// One function for both words rather than a copy in each, because the two
// were already two spellings of one rule and the divergence #3904 records is
// exactly what two copies of it cost.
func (r *Runner) exportPrintWithOperands(args []string) ([]string, bool) {
	if len(args) == 0 {
		// No operand, so the question does not arise: every column writes
		// the whole listing and no dialect is asked an axis it cannot
		// answer.
		return nil, true
	}
	switch r.sem().ExportOrReadonlyPrintWithOperands {
	case ExportPrintLetterIsInert:
		return nil, false
	case ExportPrintNarrowsToTheOperands:
		// The operand's name and not the whole word, which is the same
		// reading `typeset -p` takes in this column and the same stripper —
		// see declarePrintOperandNames. `export e1=1; export -p e1=9` writes
		// the row `export e1=1` rather than reporting `e1=9` missing (#3922).
		return declarePrintOperandNames(args), true
	case ExportPrintDropsTheOperands:
		return nil, true
	}
	r.diagf("%s\n", r.unanswered("`export -p` or `readonly -p` given an operand"))
	r.status = 2
	r.unspecified = true
	return nil, true
}
