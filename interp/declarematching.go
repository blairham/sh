// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// `typeset -m` and `typeset +m`, where the operands are *patterns* rather
// than names.
//
// One shell has the letter: zsh, whose `typeset` and `declare` take it and
// whose `local`, `export`, `readonly`, `integer` and `float` do not — measured
// a letter at a time, and each of the five says `bad option: -m` back. bash
// refuses `-m` under both spellings. ksh93 has an `m` and it is a wholly
// different thing — a *rename*, `typeset -m new=old`, which moves a parameter
// rather than selecting several — so there is no axis here either, only a
// letter one dialect has. See Semantics.DeclareOptions.
//
// What makes it worth its own file is that the letter decides nothing on its
// own. Every other letter on the line decides what matching *means*, and the
// four readings are genuinely four commands:
//
//	typeset -m 'p*'      each match's value      — a listing
//	typeset +m 'p*'      each match's attributes and name — a second listing
//	typeset -mx 'p*'     export every match      — a declaration
//	typeset +mx 'p*'     the exported matches, named — a third listing
//
// The sign that decides is the *letter's* and not the word's: `typeset -m +x`
// takes the export attribute off the matches where `typeset +mx` prints them,
// so a reading that asked which sign the last option word carried would have
// swapped the two. That is what declareFlags.added records.
//
// Measured 2026-09-10 against zsh 5.9.2 with a scrubbed environment and no
// startup files. `compdump` is why it exists: its `typeset +fm '_*'` is how
// the completion system collects the function names it is about to write, and
// refusing the letter left every interactive startup without a dump.
//
// **Order.** A pattern's matches are written in this engine's sorted order,
// for both signs. zsh sorts them under `+m` and writes them in its parameter
// table's *hash* order under `-m` — an order no script can predict and this
// engine has no way to reproduce, since it is a property of that shell's
// table rather than of the language. Sorted for both is the deterministic
// reading, and it agrees with zsh wherever a pattern matches one name, which
// is what a script that writes a name as its own pattern is doing.

// declareMatching is `typeset -m` with at least one pattern. See the file
// comment for why the letters around it are the whole question.
func (r *Runner) declareMatching(name string, patterns []string, f declareFlags) int {
	switch {
	case f.function || f.funcNames:
		// The function table rather than the parameters, and the sign of the
		// `f` letter picks the shape: `-f` writes bodies and `+f` writes
		// names. The same walk `functions -m` makes, and deliberately the
		// same code — a second one here is how the two would come to
		// disagree about a name defined twice or a pattern matching none.
		return r.functionsMatching(patterns, f.functionOff)
	case f.print && !anyAssignment(patterns):
		// `-p` outranks the *listings* wherever it is written, so the
		// matches come back in the `-p` form and the `m` decides only which
		// names are in it. Measured: `typeset -pm 'p*'` and `typeset +pm
		// 'p*'` both write `typeset p=1`.
		//
		// An attribute letter alongside is accepted and decides nothing,
		// which is the stance `typeset -p` already takes without the `m`:
		// the shell being modeled uses such a letter to *filter* the full
		// listing, and that filter is not built. See declareNames.
		return r.declarePrint(r.matchedNames(patterns))
	case withoutMatching(f) == declareFlags{} && !anyAssignment(patterns):
		// Nothing but the letter itself and nothing to assign, so this is
		// the listing it names. An assignment takes it out of this branch
		// however the letter was signed: `typeset -m 'p*'=9` sets every
		// match to 9 and `typeset +m 'p*'=9` does the same, which is
		// measured and is the one place the plus sign decides nothing.
		return r.matchedListing(patterns, f.matchNames)
	case !f.added && f.attributeFilter() != nil:
		// Every letter on the line was written with a plus, so none of them
		// is a declaration and all of them are a *filter*: the matching
		// names carrying those attributes, named and not valued.
		return r.matchedNameListing(patterns, f.attributeFilter())
	}
	// Whatever the letters ask for, applied to the parameters that already
	// exist. A pattern creates nothing: `typeset -mx 'nosuch*'` is a silent
	// 0 with no name behind it.
	f.matching, f.matchNames = false, false
	operands := r.matchedOperands(patterns)
	if len(operands) == 0 {
		return 0
	}
	if f.print {
		// An assignment is the command and `-p` is only the shape of the
		// answer: measured, `typeset -pm 'q*'=9` sets every match to 9 and
		// *then* lists them, where the same line without the `m` assigns
		// nothing at all. Reading `-p` first sent `q*=9` to the listing as
		// though it were a name, which matched nothing — and a listing
		// asked for no names writes the whole table, so one operand wrote
		// the entire parameter table out and assigned nothing.
		f.print = false
		if code := r.declareNames(name, operands, f); code != 0 {
			return code
		}
		return r.declarePrint(r.matchedNames(patternHalves(patterns)))
	}
	return r.declareNames(name, operands, f)
}

// patternHalves is each operand with any `=value` cut away, so that the names
// an assigning line touched can be looked up again once it has run.
func patternHalves(operands []string) []string {
	out := make([]string, 0, len(operands))
	for _, operand := range operands {
		pattern, _, _ := strings.Cut(operand, "=")
		out = append(out, pattern)
	}
	return out
}

// withoutMatching is one declaration's letters with the matching letter and
// the signs it carries taken out, so that "nothing else was written" can be
// asked as one comparison rather than as a list of fields that a new
// attribute would silently fall off the end of.
func withoutMatching(f declareFlags) declareFlags {
	f.matching, f.matchNames, f.remove, f.added = false, false, false, false
	return f
}

// attributeFilter is the test a plus-signed attribute letter makes of a
// declaration, or nil where no such letter was written. It is the `+mx`
// reading — the matching names that are exported — and it is a filter rather
// than a request because a plus letter takes an attribute *off*, which no
// listing does.
//
// `-g` is deliberately absent: it says where a declaration lands rather than
// what a name carries, so there is nothing for it to filter on and a line
// that writes only `+g` is a declaration and not a listing.
func (f declareFlags) attributeFilter() func(declaration) bool {
	var tests []func(declaration) bool
	add := func(on bool, test func(declaration) bool) {
		if on {
			tests = append(tests, test)
		}
	}
	add(f.integer, func(d declaration) bool { return d.integer })
	add(f.float, func(d declaration) bool { return d.float })
	add(f.readonly, func(d declaration) bool { return d.readonly })
	add(f.export, func(d declaration) bool { return d.exported })
	add(f.array, func(d declaration) bool { return d.isArr })
	add(f.assoc, func(d declaration) bool { return d.isAssoc })
	add(f.lower, func(d declaration) bool { return d.lower })
	add(f.upper, func(d declaration) bool { return d.upper })
	add(f.unique, func(d declaration) bool { return d.unique })
	add(f.hidden, func(d declaration) bool { return d.hidden })
	if len(tests) == 0 {
		return nil
	}
	// Any of them and not all of them, which is measured rather than the
	// reading a filter invites: `typeset +mxi '[ipq]'` over an exported `q`,
	// an integer `i` and a plain `p` writes `i` and `q` in zsh 5.9.2 — so a
	// name carrying *either* attribute is in, and `p` is out for carrying
	// neither. The same answer without a pattern: `typeset +xr` writes every
	// exported name and every read-only one. An intersection agrees with it
	// on one letter and disagrees on every line that writes two, which is
	// why one letter is not enough to pin it (#1576).
	return func(d declaration) bool {
		for _, test := range tests {
			if test(d) {
				return true
			}
		}
		return false
	}
}

// matchedNames is every declarable name a pattern picks out, pattern by
// pattern and sorted within each.
//
// A name reached by two patterns is written twice, which is measured rather
// than an oversight: `typeset +fm 'p*' pa` names `pa` once per pattern in zsh
// 5.9.2, exactly as `functions -m` does — see functionsMatching, where the
// same finding was made from the other side.
func (r *Runner) matchedNames(patterns []string) []string {
	var out []string
	all := r.declarableNames()
	for _, pattern := range patterns {
		o := r.patternOpts(pattern)
		for _, name := range all {
			if matchPattern(pattern, name, o) {
				out = append(out, name)
			}
		}
	}
	return out
}

// matchedOperands is matchedNames with each operand's `=value` half carried
// over onto every name its pattern picked out — `typeset -m 'p*'=9` sets each
// matching parameter to 9, which is measured.
func (r *Runner) matchedOperands(operands []string) []string {
	var out []string
	for _, operand := range operands {
		pattern, value, assigned := strings.Cut(operand, "=")
		for _, name := range r.matchedNames([]string{pattern}) {
			if r.producedParameter(name) {
				continue
			}
			if assigned {
				name += "=" + value
			}
			out = append(out, name)
		}
	}
	return out
}

// producedParameter reports whether a name reads as a table this shell makes
// up on each read rather than as state a script put there — `keymaps`,
// `parameters`, `terminfo`.
//
// A pattern reaches them, because they are listable, and a *declaration* must
// not: there is nothing behind them for an attribute to change, and this
// engine marks them readonly, so `typeset -m +x '*a*'` swept six of them up
// and answered with six read-only refusals where zsh writes one line. zsh's
// counterparts are its unloaded module parameters, which its own listing
// calls `undefined` and which take a letter in silence.
//
// A stored value wins, the same order every read follows: a script that has
// assigned to the name is declaring its own parameter and not the view.
func (r *Runner) producedParameter(name string) bool {
	if _, ok := r.Vars[name]; ok {
		return false
	}
	if _, ok := r.Arrays[name]; ok {
		return false
	}
	if _, ok := r.AssocArrays[name]; ok {
		return false
	}
	if _, ok := r.DynamicArrays[name]; ok {
		return true
	}
	_, ok := r.DynamicAssocs[name]
	return ok
}

// matchedListing is the listing `-m` and `+m` are when no other letter was
// written: the value under a minus, the attributes and the bare name under a
// plus.
//
// Both halves come from the bare listing's engine, which writes the attribute
// words *and* the value — `integer n=5`. Each half takes one of the two:
// `+m` is `integer n` and `-m` is `n=5`, and neither is the whole row.
// Measured 2026-09-10 on zsh 5.9.2 over a scalar, an integer, an array, an
// association, a readonly and a local.
//
// The hiding attribute is the one thing `-m` does not inherit: `typeset -H
// h=v; typeset -m h` writes `h=v` there, where the bare listing and `+m`
// write the name with no value at all.
func (r *Runner) matchedListing(patterns []string, namesOnly bool) int {
	names := r.matchedNames(patterns)
	if namesOnly {
		return r.declarationNameListing(names)
	}
	for _, name := range names {
		d, _ := r.declarationOf(name)
		r.printf("%s\n", name+"="+r.listedDeclarationValue(d))
	}
	return 0
}

// declarationNameListing writes each name with its attribute words and no
// value — `integer n`, `array tied FPATH fpath`.
//
// The shape two commands share, which is why it is a function rather than a
// branch: `typeset +m PAT` names the matches and a bare `typeset +` names the
// whole table. Those reached the same rows by two routes before, and only one
// of the routes existed — the plus form with no pattern wrote nothing at all
// (#1576). Choosing the names is the caller's; writing them is here.
func (r *Runner) declarationNameListing(names []string) int {
	locals := r.innermostLocalNames()
	for _, name := range names {
		d, _ := r.declarationOf(name)
		r.printf("%s\n", r.attributeWordHead(d, locals[name])+name)
	}
	return 0
}

// matchedNameListing is the `+mx` reading: the matching names that carry
// every plus-signed attribute, named and nothing else. No attribute words —
// the letters already said which attributes these are.
func (r *Runner) matchedNameListing(patterns []string, keep func(declaration) bool) int {
	return r.declarationFilteredNameListing(r.matchedNames(patterns), keep)
}

// declarationFilteredNameListing is that reading over names already chosen —
// `typeset +x` with no pattern at all, where the names are the whole table.
// The same fold declarationNameListing is: the letters decide the test and
// the caller decides the candidates.
func (r *Runner) declarationFilteredNameListing(names []string, keep func(declaration) bool) int {
	for _, name := range names {
		d, known := r.declarationOf(name)
		if !known || !keep(d) {
			continue
		}
		r.printf("%s\n", name)
	}
	return 0
}

// anyAssignment reports whether any operand carries a value — the `=9` of
// `typeset -m 'p*'=9`. A pattern is not a name, so the split is at the first
// `=` for the same reason a name's is.
func anyAssignment(operands []string) bool {
	for _, operand := range operands {
		if strings.Contains(operand, "=") {
			return true
		}
	}
	return false
}
