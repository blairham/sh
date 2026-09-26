// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// An assignment *prefix* — `name=value cmd`, the value a command is handed
// for the length of one command — and what its right-hand side is.
//
// **It is an assignment's value, and the noun is the assignment.** That is
// the whole of this file, and it is the reading the panel is unanimous on:
// the value is expanded the way the statement form's is, so it is not split,
// not brace-expanded and **not matched against the filesystem**. Measured
// 2026-09-26 in a directory holding `a.txt b.txt c.txt one.only plain`, with
// an `echo *.txt` beside every row printing the three names so the literal
// answers are falsifiable:
//
//	                    a=*.txt printenv a   a=one.* printenv a
//	zsh 5.9.2           *.txt                one.*
//	bash 5.3.20         *.txt                one.*
//	ksh93u+ 2012-08-01  *.txt                one.*
//	dash (/bin/dash)    *.txt                one.*
//	BusyBox ash v1.37.0 *.txt                one.*
//
// This shell matched all five, in every dialect, and handed the child the
// names (#4657). The road was Runner.expandWord — the *word* pipeline, which
// globs because an ordinary word does — where the statement form has always
// taken Runner.expandAssignValue.
//
// **The wrong noun is "an assignment that stores into the shell".** It agrees
// with the right one on every row a statement can produce, and the prefix is
// exactly where the two part: `a=*.txt cmd` leaves this shell's own `a`
// untouched — `a=kept; a=*.txt printenv a; typeset -p a` is `typeset a=kept`
// in zsh 5.9.2 with the option in either state — and its value is globbed
// under the option all the same. So the pair that keys the rule is
// `a=*.txt` against `a=*.txt cmd`, which hold the noun fixed and differ in
// whether anything is stored, against `typeset a=*.txt`, which keeps the
// characters. See [Semantics.ScalarAssignmentValueIsGlobbed].
//
// Under the option the prefix follows the statement form in every measured
// respect, and the child is where the consequence shows:
//
//	setopt globassign
//	a=one.*     printenv a    one.only          a scalar, and exported
//	a=*.txt     printenv a    (nothing, 1)      an array, and arrays do not export
//	a=*.nomatch printenv a    no matches found  the command never runs
//	setopt nullglob; a=*.nomatch printenv a     an empty scalar, exported as `a=`
//
// The missing name is not a rule of its own. zsh exports no array at all —
// `a=(x y); export a; printenv a` is status 1 — so a prefix whose pattern
// matched more than one name has nothing to hand over, and the name is
// **absent** from the child's environment rather than left showing what the
// shell had: `export a=old; setopt globassign; a=*.txt printenv a` is status
// 1 and not `old`. Inside a function, which forks nothing, the same prefix is
// visible as `typeset -g -a a=( a.txt b.txt c.txt )`.

// prefixGlobMatch is one prefix assignment's match, held for the length of
// the command it stands in front of.
//
// A slice rather than a map, the arrangement prefixTraceAssigns gives its
// reasons for: a prefix is one, two or three assignments, so the scan is
// shorter than hashing a pointer, and a map on the Runner is a *table* every
// clone then has to own.
type prefixGlobMatch struct {
	assign *syntax.Assign
	fields []string
}

// prefixAssignValue is a prefix assignment's own value, expanded once.
//
// The text, and — where this shell asked for the reading and the value turned
// out to be a pattern — the names it matched. The fields are recorded against
// the assignment rather than returned, because the four routes a prefixed
// command takes each reach the expansion through Runner.prefixExpansion and
// only three of them care what the match was.
func (r *Runner) prefixAssignValue(a *syntax.Assign) string {
	value, fields, globbed := r.expandScalarAssignValue(a.Value)
	if globbed {
		r.prefixGlobMatches = append(r.prefixGlobMatches, prefixGlobMatch{assign: a, fields: fields})
	}
	return value
}

// prefixGlobFields is what this prefix's value matched, and whether it was
// read as a pattern at all.
//
// The second return rather than a nil check, for the reason
// expandedAssign.fieldsSet exists: no fields is a real answer — `setopt
// globassign nullglob; a=*.nomatch cmd` hands the child an empty `a` — and it
// is not the same answer as "this prefix never globbed".
func (r *Runner) prefixGlobFields(a *syntax.Assign) ([]string, bool) {
	for _, m := range r.prefixGlobMatches {
		if m.assign == a {
			return m.fields, true
		}
	}
	return nil, false
}

// prefixGlobbedToAnArray reports whether this prefix's value matched more
// than one name, which is what makes the entry an array — and an array is
// what no child's environment can carry.
func (r *Runner) prefixGlobbedToAnArray(a *syntax.Assign) bool {
	fields, ok := r.prefixGlobFields(a)
	return ok && len(fields) > 1
}

// storeGlobbedPrefix puts a matched prefix where the name is, for the length
// of the command.
//
// The statement form's store in every respect but the cell it lands in — see
// Runner.storeGlobbedScalarAssign, which this is deliberately the twin of.
// **How many names matched decides the kind**, the numeric attributes go
// whichever it is, and the append is not an append: measured on zsh 5.9.2,
// 2026-09-26, `setopt globassign; a=x; a+=one.* printenv a` is `one.only`
// and not `xone.only`, so a match replaces the name whichever operator was
// written — which is why a.Append is not consulted here.
//
// A single match is a *scalar* and not a one-element array, and zero under
// `nullglob` is an **empty scalar**: `a=*.nomatch(N) f` shows the body an
// exported `a` holding nothing. The three-way split is the row an
// implementation that rewrote the prefix as an array gets wrong at status 0.
func (r *Runner) storeGlobbedPrefix(a *syntax.Assign, fields []string) {
	r.clearTypeAttributes(a.Name)
	if len(fields) > 1 {
		r.setArray(a.Name, fields)
		return
	}
	value := ""
	if len(fields) == 1 {
		value = fields[0]
	}
	r.setVarAs(a.Name, value, assignedAlone)
}

// prefixGlobTraced is the record `set -x` needs to render a matched prefix,
// and nil for every prefix that was not read as a pattern.
//
// The trace says what the command was handed rather than what the script
// typed: measured on zsh 5.9.2, 2026-09-26, `setopt globassign xtrace;
// a=*.txt true` writes `a=( a.txt b.txt c.txt )` — the element list a written
// literal gets, from a line with no parentheses in it — while one match
// writes the value bare and no match writes an empty quoted value. The same
// rendering the statement form reaches, and reached the same way. See
// Runner.traceAssign.
func (r *Runner) prefixGlobTraced(a *syntax.Assign) *expandedAssign {
	fields, ok := r.prefixGlobFields(a)
	if !ok {
		return nil
	}
	return &expandedAssign{assign: a, fields: fields, fieldsSet: true}
}

// withoutPrefixArrayNames takes the named entries out of the environment a
// child is about to be given.
//
// **An array reaches no child's environment**, and taking the name out is not
// the same as leaving it off: a prefix whose pattern matched more than one
// name has to *shadow* whatever the shell was exporting under it. Measured on
// zsh 5.9.2, 2026-09-26: `export a=old; setopt globassign; a=*.txt printenv
// a` prints nothing and exits 1, where the same line with the option off
// prints `*.txt` and the same line over `a=one.*` prints `one.only`. So the
// child is shown neither the match nor the old value.
//
// The mechanism is zsh's and not this construct's — `a=(x y); export a;
// printenv a` exits 1 there too — which is why the removal is written as
// "this name carries an array now" rather than as a rule about prefixes.
func withoutPrefixArrayNames(env, names []string) []string {
	if len(names) == 0 {
		return env
	}
	kept := env[:0:0]
	for _, entry := range env {
		drop := false
		for _, name := range names {
			if len(entry) > len(name) && entry[len(name)] == '=' && entry[:len(name)] == name {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, entry)
		}
	}
	return kept
}
