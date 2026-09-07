// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// An operand a builtin was given where it wanted a name.
//
// `export -` used to export a variable called `-` and report success. That
// hides a typo — `export -n FOO` with a stray space is `export - n FOO` — and
// it hides a shell whose options are not the ones the script was written for,
// which is the same reason a bad option is refused.
//
// Three of the four refuse every operand that is not a name, differing only in
// wording and in whether the script survives it. zsh is the fourth and refuses
// fewer: a special parameter is a name there, so `export -` is not a complaint
// at all.

// specialParameterNames are the one-character parameters a dialect may accept
// where a builtin wants a name.
//
// `$0` is in the set and the other digits are not: zsh takes `export 0` and
// refuses `export 1`, which is the difference between a special parameter and
// a positional one.
const specialParameterNames = "?*@#!-$0"

// isBuiltinName reports whether an operand can stand where a builtin wants a
// name.
//
// Measured on the operand *before* any `=`, because that is the part every
// shell judges: `export 1x=v` complains about the name in all four, and the
// two that quote the whole word back are quoting what they were given rather
// than what they rejected.
func (r *Runner) isBuiltinName(name string, takes NameOperands) bool {
	if takes == AnythingIsAName {
		return true
	}
	if name == "" {
		return false
	}
	if isPlainName(name) {
		return true
	}
	switch takes {
	case NamesAndSpecialParameters:
		return len(name) == 1 && strings.IndexByte(specialParameterNames, name[0]) >= 0
	case NamesAndPositionals:
		return allDigits(name)
	case PlainNamesOnly:
		return false
	}
	r.diagf("%s\n", r.unanswered("what may stand where a name is wanted"))
	r.status = 2
	r.unspecified = true
	return false
}

// isPlainName reports whether s is a name: a letter or `_`, then letters,
// digits and `_`.
//
// Not isNameLike, which trims first — that is right where it is used, for a
// word being judged as an assignment, and wrong here: all four shells refuse
// `export " a "`, and trimming would have made it a name.
func isPlainName(s string) bool {
	// The one caller has already refused the empty operand — it has to, since
	// an "every byte is a digit" loop accepts it — so this is belt and braces
	// rather than the check that does the work.
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || isLetter(c) || (i > 0 && isDigit(c)) {
			continue
		}
		return false
	}
	return true
}

// allDigits reports whether every byte is one, which is what tells a
// positional parameter from a name that starts badly: `12` is one and `1x` is
// not.
func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// nameRules gives the two answers that split by builtin rather than by
// dialect, in one place rather than at each of the three call sites.
// takesASubscript answers whether `a[0]` is a name to this builtin.
//
// Two questions rather than one, because the answer is per builtin as much
// as per dialect: bash takes it for `unset` and refuses it for `export` and
// `readonly`, and the two builtins sit on different name strictnesses in
// every shell, so no rule over that strictness gives all four.
func (r *Runner) takesASubscript(builtin string) bool {
	switch builtin {
	case "unset":
		return r.ask(r.sem().UnsetTakesASubscript, "`unset a[0]` naming an array element")
	case "typeset", "declare", "integer", "local":
		// A third answer rather than the declaration's, because bash gives a
		// third answer: it refuses `export a[1]=v` and takes
		// `typeset a[1]=v`. See Semantics.TypesetTakesASubscript.
		//
		// `local` is on this side of the split rather than with `export`:
		// measured 2026-09-07, bash 5.3 takes `local a[1]=v` and creates a
		// local array holding the element, and zsh 5.9.2 takes the operand
		// too — it refuses it afterwards, for a reason of its own about
		// elements rather than about names. Reading it through the export
		// question made this shell refuse a line bash has always accepted.
		return r.ask(r.sem().TypesetTakesASubscript, "`typeset a[0]=v` naming an array element")
	}
	return r.ask(r.sem().DeclarationTakesASubscript, "`export a[0]` naming an array element")
}

func (r *Runner) nameRules(builtin string) (fatal Answer, takes NameOperands) {
	if builtin == "unset" {
		return r.sem().BadNameToUnsetFatal, r.sem().UnsetNameOperands
	}
	return r.sem().BadNameToDeclarationFatal, r.sem().DeclarationNameOperands
}

// builtinNames keeps the operands that are names and reports the rest.
//
// Every bad operand is reported, not just the first: bash prints a line for
// each of `export 1x 2y` and goes on to export whatever was well formed. The
// other three print one line because the first is fatal there and the loop
// never reaches the second, which falls out of the fatality rather than
// needing a rule of its own.
//
// **A fatal refusal does not throw the operands away.** Every shell in the
// panel declares the names that *were* names and only then stops, so the
// give-up is held back and reported as `ended` for the caller to raise once
// it has done what those operands ask (#1211).
//
// How much of the list goes with the refusal is where the panel splits, and
// the *position* of the bad name is what shows it. Measured 2026-09-07 from a
// script file over `export`, `readonly`, `typeset` and `unset`, reading the
// names back from an EXIT trap because the fatality ends everything written
// after it:
//
//   - ksh93 and zsh declare **every** well-formed operand, wherever the bad
//     one stood: `export ok1=1 ":" ok2=2` leaves both set in each.
//   - bash-as-`sh` and dash declare only the ones **in front of** it, so the
//     same line leaves ok1 set and ok2 unset, and the bad name first leaves
//     neither.
//
// With the bad name *last* the two answers agree, which is why a probe that
// only ever put it there could not have found the axis. bash proper never
// arrives: its refusal is not fatal, so it reports each bad operand and
// declares all the good ones by carrying on.
//
// Whether a subscripted operand is a name is its own question, and it is
// answered per builtin as well as per dialect: bash refuses `export a[0]`
// and takes `unset a[0]`, ksh93 takes both, dash refuses both. Asked only
// when an operand has a subscript, so nothing else is affected.
func (r *Runner) builtinNames(builtin string, args []string, explicitVariable bool) (rest []string, status int, ended bool) {
	fatal, takes := r.nameRules(builtin)
	// `unset -v` asks for a variable by name, and gets a name checked even
	// where the bare form checks nothing: bash 5.3 is quiet about `unset 1x`
	// and refuses `unset -v 1x`. Measured in all four; only bash has a bare
	// form loose enough for the difference to show.
	if explicitVariable && takes == AnythingIsAName {
		takes = PlainNamesOnly
	}
	for _, a := range args {
		name, _, _ := strings.Cut(a, "=")
		if base, _, subscripted := r.subscriptOperand(name); subscripted && isPlainName(base) {
			if r.takesASubscript(builtin) {
				rest = append(rest, a)
				continue
			}
			if r.unspecified {
				return nil, 2, false
			}
			if ended {
				// Judged in silence: one diagnostic is what every column
				// that stops writes, however many bad operands follow the
				// first — measured with `export ":" ok1=1 "1x" ok2=2`.
				continue
			}
			status = r.badSubscriptOperand(builtin, a, name, fatal)
		} else if r.isBuiltinName(name, takes) {
			rest = append(rest, a)
			continue
		} else {
			if r.unspecified {
				return nil, 2, false
			}
			if ended {
				continue
			}
			status = r.badBuiltinName(builtin, a, name, fatal)
		}
		if r.ctl != controlExit {
			continue
		}
		ended = true
		// Put the give-up aside rather than carrying it out of here: the
		// caller declares the operands that *were* names and raises it again
		// through endAfterABadName. Holding it is what lets the caller's loop
		// run at all — every step of it stops on controlExit, which is right
		// for a failure of its own and wrong for one that has already
		// happened.
		r.ctl, r.abandon = controlNone, abandonRequested
		if !r.ask(r.sem().BadNameDeclaresTheOperandsAfterIt,
			"a fatal bad name leaving the operands after it declared") {
			if r.unspecified {
				// Nothing was decided, so nothing is declared — and the
				// script still stops, which the refusal already asked for.
				return nil, status, true
			}
			return rest, status, true
		}
	}
	return rest, status, ended
}

// endAfterABadName raises the give-up builtinNames held back.
//
// Called by every builtin on the shared name check, after its own loop: the
// operands that were names have been declared and the script stops here.
//
// No status is passed in and none is needed. fatalQuiet asks the dialect what
// a fatal error exits with — 1 in three columns and 2 in the fourth — and
// writes it over whatever the builtin was carrying, which is exactly what the
// refusal itself did before the give-up was held back.
func (r *Runner) endAfterABadName() int {
	r.fatalQuiet()
	return r.status
}

// badSubscriptOperand reports a subscripted operand the dialect will not take.
//
// Two of the three that refuse it say what they say about any bad name, so
// they need nothing here and fall through to badBuiltinName. The third has a
// complaint of its own per builtin — one about the subscript, naming the base,
// and one about array elements, naming the whole operand — and does not name
// the builtin in the location for the first of them where it does for the
// second. That is the same shape as which declarations name themselves in a
// readonly refusal, and it is a set for the same reason.
func (r *Runner) badSubscriptOperand(builtin, operand, name string, fatal Answer) int {
	d := r.diag()
	wording := d.BuiltinBadSubscript[builtin]
	if wording == "" {
		// The *name* rather than the whole operand, even in the dialect
		// whose bad-name complaint quotes what it was given: measured,
		// bash 5.3 answers `export 1x=v` with `` `1x=v' `` and
		// `export a[1]=v` with `` `a[1]' ``, so a subscript is where it
		// stops quoting the value back.
		return r.badBuiltinName(builtin, name, name, fatal)
	}
	base, _, _ := r.subscriptOperand(name)
	if !d.SubscriptRefusalNamesBuiltin[builtin] {
		// The location does not name the builtin for this one, where it does
		// for the other — put aside for the report and given back, the way a
		// readonly reassignment already does it.
		outer := r.inBuiltin
		r.inBuiltin = ""
		defer func() { r.inBuiltin = outer }()
	}
	r.diagf("%s\n", Wording(wording, "", builtin, operand, base))
	status := orDefault(d.BuiltinBadNameStatus, 1)
	if r.ask(fatal, "a bad name to a special builtin ending the script") {
		r.status = status
		r.fatalQuiet()
		return r.status
	}
	return status
}

// badBuiltinName reports it, and ends the script where the dialect says so.
//
// The wording is per builtin because two dialects word it per builtin: ksh93
// says "is not an identifier" for `export` and "invalid variable name" for
// `readonly` and `unset`, and zsh puts the operand before the reason for
// `unset` alone where it puts it after for the other two.
func (r *Runner) badBuiltinName(builtin, operand, name string, fatal Answer) int {
	d := r.diag()
	if d.BadNameRefusalHidesTheBuiltin[builtin] {
		// The location does not name the builtin for this one where it does
		// for the others — put aside for the report and given back, the way
		// badSubscriptOperand and the readonly refusal already do it.
		outer := r.inBuiltin
		r.inBuiltin = ""
		defer func() { r.inBuiltin = outer }()
	}
	shown := name
	if d.BuiltinBadNameKeepsValue {
		shown = operand
	}
	wording := d.BuiltinBadName[builtin]
	// A leading digit is a different complaint in the one dialect that tells
	// the two apart: zsh says "not an identifier: 1x" for `1x` and "not valid
	// in this context: a-b" for `a-b`. An empty entry means the dialect says
	// the same thing to both, which is three of the four.
	if len(name) > 0 && name[0] >= '0' && name[0] <= '9' {
		if numeric := d.BuiltinBadNameNumeric[builtin]; numeric != "" {
			wording = numeric
		}
	}
	r.diagf("%s\n", Wording(wording, "%[1]s: `%[2]s': not a valid identifier", builtin, shown))
	status := orDefault(d.BuiltinBadNameStatus, 1)
	if r.ask(fatal, "a bad name to a special builtin ending the script") {
		r.status = status
		r.fatalQuiet()
	}
	return status
}
