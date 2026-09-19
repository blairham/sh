// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// Where `set -x` writes a declaration utility's `name=value` operand.
//
// `typeset x=1` is one command with one operand, and the panel writes it in
// three shapes rather than one. Two of them put the operand somewhere other
// than the command's own line, and this shell wrote neither, so a traced
// script showed the utility and not the assignment it performed (#3567).
//
// Measured 2026-09-19, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> x.sh` over
// a script file with `set -x` on line 1 and the command on line 2, stdin from
// `/dev/null`:
//
//	written            	bash 5.3.20                  	ksh93u+ 2012-08-01        	zsh 5.9.2
//	`typeset x=1`      	`+ typeset x=1`              	`+ x=1` / `+ typeset x`   	one line
//	`export ev=1`      	`+ export ev=1` / `+ ev=1`   	`+ ev=1` / `+ export ev`  	one line
//	`readonly rv=5`    	`+ readonly rv=5` / `+ rv=5` 	`+ rv=5` / `+ readonly rv`	one line
//	`export e1=1 e2=2` 	the line / `+ e1=1` / `+ e2=2`	`+ e1=1` / `+ e2=2` / `+ export e1 e2`	one line
//
// dash 0.5.12 and BusyBox ash 1.37.0 have no declaration utility that takes
// an operand this question can be asked about — `export ev=1` is one line
// there — and bash 3.2.57 answers as bash 5.3.20 does.
//
// So two independent rules and not one exception to a rule:
//
//  1. ksh93 takes **every** operand off the command line and writes it as an
//     assignment in front, leaving the bare name behind. This type.
//  2. bash leaves the operand where it was written and writes it **again**
//     behind the command line, for two utilities and no others. See
//     Diagnostics.TraceRepeatsAScalarOperandAfter.
//
// Decoration, so both are on Diagnostics beside TraceStyle, TraceQuoting and
// TracePrefixAssignment rather than on Semantics: they decide what is
// *written* and not what happens.
type TraceDeclarationOperand int

const (
	// TraceOperandOnTheCommandLine leaves the operand where the script wrote
	// it, on the command's own line. bash, zsh, dash and BusyBox ash, and
	// the substrate's own. How it is *rendered* there is a second question —
	// see TraceAssignmentOperand.
	TraceOperandOnTheCommandLine TraceDeclarationOperand = iota

	// TraceOperandSplitBefore writes the operand as an assignment on a line
	// of its own ahead of the command, and leaves the bare name on the
	// command line: `typeset x=1` is `+ x=1` then `+ typeset x`. ksh93.
	//
	// The name and not the target: `typeset -a a; typeset a[1]=v` is
	// `+ a[1]=v` then `+ typeset a`, so the subscript goes with the
	// assignment line and not with the word that is left behind, and the cut
	// is at the `=` with a subscript dropped rather than an identifier kept.
	//
	// An **appending** operand splits the same way: ksh93 writes
	// `typeset x+=q` as `+ x+=q` then `+ typeset x+`, and so does this. It
	// did not until #3772, because the word was not read as an assignment
	// at all — `assignNameSplit` requires a plain name in front of the `=`
	// and `x+` is not one — so it split into fields and the position this
	// line is keyed on was never recorded. See interp.appendNameSplit.
	TraceOperandSplitBefore
)

// An array literal operand — `typeset a=(1 2)` — is a **third** answer that
// is measured and not modeled: bash writes `+ a=('1' '2')` ahead of the
// command line for it while leaving a scalar where it was, so its split is
// keyed on the operand's shape rather than on the utility. It is left out
// because writing that line needs the literal's elements expanded before the
// utility runs, and in this engine the utility is what decides how they are
// read: `typeset -A m=([k]=v)` reads `[k]` as a key only because `typeset -A`
// has already declared the name a table. Expanding the operand ahead of the
// trace would decide that question before the answer exists. #3567 carries
// the measurement and stays open for it.

// declarationOperandIndex reports whether the word at this position in argv
// is one the expansion read as a declaration utility's assignment.
//
// The positions are recorded by the expansion itself rather than re-derived
// from the expanded string, because the two readings come apart on exactly
// the words that discriminate. Measured 2026-09-19 on ksh93u+: `typeset x=1`
// is split in front and `typeset "x=a b"`, `typeset x"=a b"`, `typeset "x"=ab`
// and `n=x; typeset $n=1` are not — one line each, operand and all. Every one
// of those expands to a string with an `=` in it, and only the *word* says
// whether the name was written as a name. That is the same question
// Runner.assignShaped asked when it routed the word through
// Runner.expandAssignArg, so the answer is kept from there.
func (r *Runner) declarationOperandIndex(i int) bool {
	for _, at := range r.declarationOperands {
		if at == i {
			return true
		}
	}
	return false
}

// declarationOperandsFrom is where this command's first declaration operand
// stood, or -1 where it has none.
//
// *Which* words are rendered as an assignment rather than as a word is a
// wider question than which were read as one, and the panel says so: measured
// 2026-09-19 on zsh 5.9.2, `typeset "y=a b"` is `typeset 'y=a b'` and
// `typeset "y=a b" x=1` is `typeset 'y=a b' x=1`, while `typeset x=1 "y=a b"`
// is `typeset x=1 y='a b'` — the same quoted word, written both ways, in the
// same shell. ksh93 splits the same line the same way and leaves `y='a b'` on
// the command line behind it. So the rendering runs from the first operand to
// the end of the line, where the *split* takes only the words that were read
// as assignments.
func (r *Runner) declarationOperandsFrom() int {
	if len(r.declarationOperands) == 0 {
		return -1
	}
	return r.declarationOperands[0]
}

// traceOperandSplitsBefore reports whether this dialect takes a declaration
// utility's operands off the command line and writes them in front of it.
//
// The utility has to be one this shell has. `local` is in the substrate's
// list of declaration utilities and ksh93 has no such command at all, so
// `f(){ local lv=1; }; f` is `+ local lv=1` and then `local: not found`
// there — one line, because nothing was declared. Splitting on the list
// alone wrote an assignment line for a command that does not exist.
func (r *Runner) traceOperandSplitsBefore(words []string, d Diagnostics) bool {
	if d.TraceDeclarationOperand != TraceOperandSplitBefore ||
		len(r.declarationOperands) == 0 {
		return false
	}
	k := traceDeclarationUtilityWord(words)
	if k >= len(words) {
		return false
	}
	_, ok := r.lookupBuiltin(words[k])
	return ok
}

// traceOperandCommandWord is what is left of a split operand on the command
// line: the target up to the `=`, with any subscript dropped.
func traceOperandCommandWord(w string) string {
	name, _, ok := strings.Cut(w, "=")
	if !ok {
		return w
	}
	if at := strings.IndexByte(name, '['); at >= 0 {
		return name[:at]
	}
	return name
}

// traceOperandsBefore is the assignment lines this dialect writes ahead of a
// declaration utility's own line, in the order the operands were written.
func (r *Runner) traceOperandsBefore(words []string, d Diagnostics) []string {
	if !r.traceOperandSplitsBefore(words, d) {
		return nil
	}
	lines := make([]string, 0, len(r.declarationOperands))
	for _, at := range r.declarationOperands {
		if at >= len(words) {
			continue
		}
		line, ok := traceAssignmentOperand(words[at], d)
		if !ok {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// traceOperandsAfter is the assignment lines the dialect that *repeats* a
// scalar operand writes behind the command's own line, one per operand in the
// order they were written.
//
// Gated on the utility that ran and not on the dialect's reading of how a
// declaration has to be spelled, which is measured and is the difference
// between this and TraceOperandSplitBefore. On bash 5.3.20, where the
// declaration reading is DeclarationByUnquotedLiteralWord:
//
//	`c=export; $c ev=1`    	`+ export ev=1` / `+ ev=1`
//	`builtin export ev=1`  	the line       / `+ ev=1`
//	`command -p export ev=1`	the line       / `+ ev=1`
//	`export -- ev=1`       	the line       / `+ ev=1`
//	`export f ev=1`        	the line       / `+ ev=1`  — only the operand
//
// The first is the discriminator: that spelling is *not* a declaration to
// bash, so the operand is an ordinary word there, and the line is still
// written. What the shell is reporting is what `export` is about to be given,
// which is also why a refused one is still written — `readonly z=1; export
// z=2` traces `+ export z=2`, `+ z=2` and then the complaint.
func (r *Runner) traceOperandsAfter(words []string, d Diagnostics) []string {
	if len(d.TraceRepeatsAScalarOperandAfter) == 0 || len(words) == 0 {
		return nil
	}
	k := traceDeclarationUtilityWord(words)
	if k >= len(words) {
		return nil
	}
	found := false
	for _, name := range d.TraceRepeatsAScalarOperandAfter {
		if words[k] == name {
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	var lines []string
	for _, w := range words[k+1:] {
		line, ok := traceAssignmentOperand(w, d)
		if !ok {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// traceDeclarationUtilityWord is where in argv the utility itself stands,
// past the words that only say where to look for it.
//
// `command`, `command -p` and `builtin` all keep bash's repeat — measured
// above — so the scan steps over them. Anything else is the utility, whether
// or not this shell has one by that name.
func traceDeclarationUtilityWord(words []string) int {
	k := 0
	for k < len(words) {
		switch {
		case words[k] == "command", words[k] == "builtin":
			k++
		case k > 0 && words[k] == "-p" && words[k-1] == "command":
			k++
		default:
			return k
		}
	}
	return k
}
